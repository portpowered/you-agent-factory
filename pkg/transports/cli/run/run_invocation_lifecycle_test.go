package run

import (
	"bytes"
	"context"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
	"io"
	"testing"
)

func TestOpenInvocationRetainsInjectedOperationWithoutOpeningRuntime(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan the sprint"
	buildCalls := 0
	lifecycleStarted := false
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		buildCalls++
		if lifecycleStarted {
			t.Fatal("invocation bootstrap constructed after lifecycle start")
		}
		return stubInvocationService{
			run: func(ctx context.Context) error {
				lifecycleStarted = true
				<-ctx.Done()
				return nil
			},
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "done",
					}},
				}, nil
			},
		}, nil
	}

	factory := testRunnerOpeners{invocation: openTestInvocationRunner}
	operation, err := Open(context.Background(), ensureTestRecordingsCLI(RunConfig{
		Dir:                      t.TempDir(),
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   io.Discard,
		DisableDefaultRecording:  true,
	}), factory.BuildRunner, factory.Invocation(), testResponsePresentation(), nil, testMockWorkersConfigLoader, testRuntimeOpeningRequestFactory)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if buildCalls != 0 || lifecycleStarted {
		t.Fatalf("after construction: build calls = %d, lifecycle started = %t; want 0, false", buildCalls, lifecycleStarted)
	}

	err = operation.Run(context.Background())
	if err != nil {
		t.Fatalf("Operation.Run() error = %v", err)
	}
	if buildCalls != 1 || !lifecycleStarted {
		t.Fatalf("after initialization: build calls = %d, lifecycle started = %t; want 1, true", buildCalls, lifecycleStarted)
	}
}

func TestInvocationTargetCarriesOnlyBoundedRuntimeSelection(t *testing.T) {
	t.Parallel()

	target := invocationTarget(RunConfig{
		Dir:                   "/tmp/factory",
		FactoryConfigPath:     "/tmp/factory/factory.yaml",
		Worktree:              "feature-login",
		Port:                  7437,
		WorkerReasoningEffort: "xhigh",
	}, zap.NewNop(), nil)
	if target.FactoryDir != "/tmp/factory" {
		t.Fatalf("FactoryDir = %q, want /tmp/factory", target.FactoryDir)
	}
	if target.FactorySourcePath != "/tmp/factory/factory.yaml" {
		t.Fatalf(
			"FactorySourcePath = %q, want /tmp/factory/factory.yaml",
			target.FactorySourcePath,
		)
	}
	if target.Worktree != "feature-login" {
		t.Fatalf("Worktree = %q, want feature-login", target.Worktree)
	}
	if target.WorkerReasoningEffort != "xhigh" {
		t.Fatalf("WorkerReasoningEffort = %q, want xhigh", target.WorkerReasoningEffort)
	}
}

func TestRun_FactoryInvocationUsesNoServerBootstrapConfig(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan the sprint"
	var captured *testRuntimeSelections
	var capturedEdges serviceedges.Edges
	openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, edges serviceedges.Edges) (sessionInvocationRunner, error) {
		cloned := *cfg
		captured = &cloned
		capturedEdges = edges
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "done",
					}},
				}, nil
			},
		}, nil
	}

	var output bytes.Buffer
	if err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if captured == nil {
		t.Fatal("expected factory invocation bootstrap config capture")
	}
	if captured.Port != 0 {
		t.Fatalf("captured Port = %d, want 0", captured.Port)
	}
	if capturedEdges.APIServerStarter != nil {
		t.Fatal("captured APIServerStarter = non-nil, want nil")
	}
}

func TestRun_FactoryInvocationReleasesSessionThroughFactoryServiceOwnership(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan the sprint"
	var closedSessionID string
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "done",
					}},
				}, nil
			},
			close: func(_ context.Context, sessionID string) error {
				closedSessionID = sessionID
				return nil
			},
		}, nil
	}

	var output bytes.Buffer
	if err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if closedSessionID == "" {
		t.Fatal("expected CloseFactorySession through bootstrap ownership path")
	}
}
