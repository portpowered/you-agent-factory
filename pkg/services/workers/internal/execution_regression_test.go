package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

// TestNewRuntimeConstructsIndependentWorkstationLifecycle proves two runtimes
// constructed through the canonical Workers construction provider (NewRuntime)
// retain independent workstation pool lifecycle state: starting and stopping
// one runtime's pool has no observable effect on an independently constructed
// runtime's pool.
func TestNewRuntimeConstructsIndependentWorkstationLifecycle(t *testing.T) {
	t.Parallel()

	first := newTestFullRuntimeService(t, zap.NewNop())
	second := newTestFullRuntimeService(t, zap.NewNop())

	binding := []workers.AssembledRuntimeBinding{{
		RoleName: "review",
		RoleKind: workers.RuntimeBuildRoleKindWorkstation,
	}}
	if _, err := first.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{Bindings: binding}); err != nil {
		t.Fatalf("first.StartWorkstationPool() error = %v", err)
	}

	firstRoute, err := first.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "review"})
	if err != nil || !firstRoute.Available {
		t.Fatalf("first.WorkstationRoute() = %#v, err = %v, want available", firstRoute, err)
	}

	if _, err := second.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "review"}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("second.WorkstationRoute() error = %v, want ErrWorkstationPoolUnavailable (independent pool never started)", err)
	}

	if _, err := first.StopWorkstationPool(context.Background()); err != nil {
		t.Fatalf("first.StopWorkstationPool() error = %v", err)
	}

	if _, err := second.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "review"}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("second.WorkstationRoute() error = %v after first's pool stopped, want it to remain independently unstarted", err)
	}

	if _, err := second.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{Bindings: binding}); err != nil {
		t.Fatalf("second.StartWorkstationPool() error = %v", err)
	}
	secondRoute, err := second.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "review"})
	if err != nil || !secondRoute.Available {
		t.Fatalf("second.WorkstationRoute() = %#v, err = %v, want available after its own independent start", secondRoute, err)
	}
}

// TestNewRuntimeWorkstationDispatchLogsExcludePromptContent proves a fully
// constructed runtime (through NewRuntime, the canonical Workers construction
// provider) keeps forwarding the construction-injected logger to workstation
// dispatch lifecycle logs, and that those logs carry only safe structured
// fields, never dispatch prompt/output payload content.
func TestNewRuntimeWorkstationDispatchLogsExcludePromptContent(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.InfoLevel)
	service := newTestFullRuntimeService(t, zap.New(core))

	const sensitivePrompt = "TOP-SECRET-SYSTEM-PROMPT"
	const sensitiveOutput = "TOP-SECRET-MODEL-OUTPUT"
	executor := recordingSensitiveExecutor{output: sensitiveOutput}
	binding := []workers.AssembledRuntimeBinding{{
		RoleName: "review",
		RoleKind: workers.RuntimeBuildRoleKindWorkstation,
		Executor: executor,
	}}
	if _, err := service.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{Bindings: binding}); err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}

	result, err := service.DispatchWorkstation(context.Background(), workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:     work.WorkDispatch{DispatchID: "dispatch-1", TransitionID: "transition-1", WorkstationName: "review"},
			SystemPrompt: sensitivePrompt,
			UserMessage:  sensitivePrompt,
		},
	})
	if err != nil {
		t.Fatalf("DispatchWorkstation() error = %v", err)
	}
	if result.Result.Output != sensitiveOutput {
		t.Fatalf("DispatchWorkstation() output = %q, want executor output preserved", result.Result.Output)
	}

	accepted := logs.FilterMessage("workers workstation dispatch accepted").All()
	if len(accepted) != 1 {
		t.Fatalf("accepted logs = %#v, want exactly one entry surviving normal construction", accepted)
	}
	terminal := logs.FilterMessage("workers workstation dispatch terminal").All()
	if len(terminal) != 1 {
		t.Fatalf("terminal logs = %#v, want exactly one entry surviving normal construction", terminal)
	}

	for _, entry := range logs.All() {
		if entry.Message == sensitivePrompt || entry.Message == sensitiveOutput {
			t.Fatalf("log message leaked payload content: %q", entry.Message)
		}
		for _, field := range entry.Context {
			if field.String == sensitivePrompt || field.String == sensitiveOutput {
				t.Fatalf("log field %q leaked payload content in entry %q", field.Key, entry.Message)
			}
		}
	}
}

type recordingSensitiveExecutor struct {
	output string
}

func (e recordingSensitiveExecutor) Execute(
	context.Context,
	workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	return workers.WorkResult{Outcome: workers.OutcomeAccepted, Output: e.output}, nil
}

func newTestFullRuntimeService(t *testing.T, logger *zap.Logger) *Service {
	t.Helper()
	runtime, err := NewRuntime(
		testModelsService{},
		testProvidersService{},
		models.RuntimeScopeRef{},
		injectedProviderRunner{},
		injectedProviderRunner{},
		workers.ProgressPublisher(testProgressPublisher),
		&workers.MockPTYAllocator{},
		logger,
		false,
		"",
		"",
		"",
		nil,
		nil,
		time.Now,
		func() []string { return nil },
		func() (string, error) { return "", nil },
		nil,
		nil,
		nil,
		testFactoryDocsLoader,
		testResolveSymlinks,
		nil,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		"linux",
		testFactoryWorktreePreparer{},
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	service, ok := runtime.(*Service)
	if !ok {
		t.Fatalf("NewRuntime() returned %T, want *Service", runtime)
	}
	return service
}

func TestNewConfiguredRuntimeAcceptsDetachedStatelessService(t *testing.T) {
	t.Parallel()

	stateless := RootFrom(nil, nil)
	runtime, err := NewConfiguredRuntime(
		testModelsService{},
		testProvidersService{},
		models.RuntimeScopeRef{},
		injectedProviderRunner{},
		injectedProviderRunner{},
		workers.ProgressPublisher(testProgressPublisher),
		&workers.MockPTYAllocator{},
		zap.NewNop(),
		false,
		"",
		"",
		"",
		nil,
		nil,
		time.Now,
		func() []string { return nil },
		func() (string, error) { return "", nil },
		nil,
		nil,
		nil,
		testFactoryDocsLoader,
		testResolveSymlinks,
		nil,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		"linux",
		testFactoryWorktreePreparer{},
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		nil,
		true,
		true,
		false,
		nil,
		&stateless,
	)
	if err != nil {
		t.Fatalf("NewConfiguredRuntime() error = %v", err)
	}
	if runtime == nil {
		t.Fatal("NewConfiguredRuntime() returned nil runtime")
	}
}
