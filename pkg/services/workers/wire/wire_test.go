package wire

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root workers.Service = service
	if root == nil {
		t.Fatal("constructed root is nil")
	}
}

func TestNewServiceAssignsRuntimeRolesWithoutLifecycle(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}

	for _, runnerID := range []string{
		runners.AgentIdentity,
		runners.ScriptIdentity,
		runners.InferenceIdentity,
	} {
		t.Run(runnerID, func(t *testing.T) {
			t.Parallel()

			result, err := service.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
				RunnerID: runnerID,
				Roles: []workers.RuntimeBuildRoleRequest{
					{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
					{Name: "review", Kind: workers.RuntimeBuildRoleKindWorkstation},
				},
			})
			if err != nil {
				t.Fatalf("BuildRuntime(%q) error = %v", runnerID, err)
			}
			if result.RunnerSelection.RunnerID != runnerID {
				t.Fatalf(
					"BuildRuntime(%q) selection = %#v, want runner %q",
					runnerID,
					result.RunnerSelection,
					runnerID,
				)
			}
			if len(result.Bindings) != 2 {
				t.Fatalf("BuildRuntime(%q) bindings = %#v, want two", runnerID, result.Bindings)
			}
			for _, binding := range result.Bindings {
				if binding.RunnerSelection.RunnerID != runnerID {
					t.Fatalf("binding selection = %#v, want runner %q", binding.RunnerSelection, runnerID)
				}
			}
		})
	}
}

func TestWorkstationPoolAvailableThroughPublishedRootShim(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	started, err := service.StartWorkstationPool(ctx, workers.WorkstationPoolStartRequest{
		Bindings: []workers.AssembledRuntimeBinding{
			{RoleName: "review", RoleKind: workers.RuntimeBuildRoleKindWorkstation},
		},
	})
	if err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}
	if started.Outcome != workers.WorkstationPoolLifecycleOutcomeStarted {
		t.Fatalf("StartWorkstationPool() outcome = %q, want STARTED", started.Outcome)
	}

	boundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    workers.WorkstationExecutionServiceFromRoot(service),
		RouteNames: []string{"review"},
		Async:      true,
	})
	if err := boundary.Start(ctx); err != nil {
		t.Fatalf("WorkstationPoolBoundary.Start() error = %v", err)
	}
}

func TestNewServiceRejectsUnknownRunnerSelection(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDCodex,
		Roles: []workers.RuntimeBuildRoleRequest{
			{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
		},
	})
	if !errors.Is(err, workers.ErrUnknownRunnerSelection) {
		t.Fatalf("BuildRuntime(codex) error = %v, want ErrUnknownRunnerSelection", err)
	}
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	valid := validNewServiceInputs()
	tests := []struct {
		name   string
		mutate func(*newServiceInputs)
		want   string
	}{
		{
			name: "agent Providers root",
			mutate: func(in *newServiceInputs) {
				in.agentDependencies.Providers = nil
			},
			want: "construct Workers: agent Providers service is required",
		},
		{
			name: "agent progress publisher",
			mutate: func(in *newServiceInputs) {
				in.agentDependencies.Publish = nil
			},
			want: "construct Workers: agent progress publisher is required",
		},
		{
			name: "script command",
			mutate: func(in *newServiceInputs) {
				in.scriptConfig = runners.ScriptConfig{}
			},
			want: "construct Workers: script command is required",
		},
		{
			name: "script command runner",
			mutate: func(in *newServiceInputs) {
				in.scriptDependencies.CommandRunner = nil
			},
			want: "construct Workers: script command runner is required",
		},
		{
			name: "script Factory docs loader",
			mutate: func(in *newServiceInputs) {
				in.scriptDependencies.FactoryDocs = nil
			},
			want: "construct Workers: script Factory docs loader is required",
		},
		{
			name: "script clock",
			mutate: func(in *newServiceInputs) {
				in.scriptDependencies.Now = nil
			},
			want: "construct Workers: script clock is required",
		},
		{
			name: "script progress publisher",
			mutate: func(in *newServiceInputs) {
				in.scriptDependencies.Publish = nil
			},
			want: "construct Workers: script progress publisher is required",
		},
		{
			name: "script event recorder",
			mutate: func(in *newServiceInputs) {
				in.scriptDependencies.Record = nil
			},
			want: "construct Workers: script event recorder is required",
		},
		{
			name: "inference worker",
			mutate: func(in *newServiceInputs) {
				in.inferenceConfig = runners.InferenceConfig{}
			},
			want: "construct Workers: inference worker name is required",
		},
		{
			name: "inference Models service",
			mutate: func(in *newServiceInputs) {
				in.inferenceDependencies.Models = nil
			},
			want: "construct Workers: inference Models service is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inputs := valid
			test.mutate(&inputs)
			service, err := inputs.callNewService()
			if err == nil {
				t.Fatalf("NewService() error = nil, want missing %s dependency", test.name)
			}
			if err.Error() != test.want {
				t.Fatalf("NewService() error = %q, want %q", err.Error(), test.want)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
	}
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	providers := &wireProvidersFake{}
	models := &wireInferenceInvoker{}
	command := &recordingWireCommandRunner{}
	agentPublishCalls := 0
	scriptPublishCalls := 0
	scriptRecordCalls := 0
	factoryDocsCalls := 0
	clockCalls := 0
	inputs := validNewServiceInputs()
	inputs.agentDependencies.Providers = providers
	inputs.agentDependencies.Publish = func(workers.ProgressFragment) {
		agentPublishCalls++
		panic("agent progress published during inert construction")
	}
	inputs.scriptDependencies.CommandRunner = command
	inputs.scriptDependencies.FactoryDocs = func(string) (map[string]string, error) {
		factoryDocsCalls++
		panic("Factory docs loaded during inert construction")
	}
	inputs.scriptDependencies.Now = func() time.Time {
		clockCalls++
		panic("script clock read during inert construction")
	}
	inputs.scriptDependencies.Publish = func(workers.ProgressFragment) {
		scriptPublishCalls++
		panic("script progress published during inert construction")
	}
	inputs.scriptDependencies.Record = func(workers.ScriptEvent) {
		scriptRecordCalls++
		panic("script event recorded during inert construction")
	}
	inputs.inferenceDependencies.Models = models

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := inputs.callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root workers.Service = service
	if root == nil {
		t.Fatal("constructed root is nil")
	}
	if providers.calls.Load() != 0 {
		t.Fatalf("Providers.Execute calls = %d, want no construction activity", providers.calls.Load())
	}
	if command.calls.Load() != 0 {
		t.Fatalf("command runner calls = %d, want no construction activity", command.calls.Load())
	}
	if models.calls.Load() != 0 {
		t.Fatalf("Models.InvokeLocal calls = %d, want no construction activity", models.calls.Load())
	}
	if agentPublishCalls != 0 || scriptPublishCalls != 0 || scriptRecordCalls != 0 ||
		factoryDocsCalls != 0 || clockCalls != 0 {
		t.Fatalf(
			"construction invoked process edges (agent publish=%d script publish=%d record=%d docs=%d clock=%d), want inert construction",
			agentPublishCalls, scriptPublishCalls, scriptRecordCalls, factoryDocsCalls, clockCalls,
		)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d",
			baseline, runtime.NumGoroutine(), leaked,
		)
	}

	ctx := context.Background()
	if _, err := service.WorkstationRoute(
		ctx,
		workers.WorkstationRouteRequest{WorkstationName: "review"},
	); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf(
			"WorkstationRoute() after construction error = %v, want ErrWorkstationPoolUnavailable",
			err,
		)
	}

	started, err := service.StartWorkstationPool(ctx, workers.WorkstationPoolStartRequest{
		Bindings: []workers.AssembledRuntimeBinding{
			{RoleName: "review", RoleKind: workers.RuntimeBuildRoleKindWorkstation},
		},
	})
	if err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}
	if started.Outcome != workers.WorkstationPoolLifecycleOutcomeStarted {
		t.Fatalf("StartWorkstationPool() outcome = %q, want STARTED", started.Outcome)
	}
	route, err := service.WorkstationRoute(
		ctx,
		workers.WorkstationRouteRequest{WorkstationName: "review"},
	)
	if err != nil {
		t.Fatalf("WorkstationRoute() after start error = %v", err)
	}
	if !route.Available || route.WorkstationName != "review" {
		t.Fatalf("WorkstationRoute() after start = %#v", route)
	}
}

func TestNewServiceFailedConstructionStartsNothing(t *testing.T) {
	t.Parallel()

	providers := &wireProvidersFake{}
	models := &wireInferenceInvoker{}
	command := &recordingWireCommandRunner{}
	inputs := validNewServiceInputs()
	inputs.agentDependencies.Providers = providers
	inputs.scriptDependencies.CommandRunner = command
	inputs.inferenceDependencies.Models = models
	inputs.inferenceConfig = runners.InferenceConfig{}

	service, err := inputs.callNewService()
	if err == nil {
		t.Fatal("NewService() error = nil, want missing inference worker")
	}
	if service != nil {
		t.Fatalf("NewService() = %#v, want nil service", service)
	}
	if providers.calls.Load() != 0 {
		t.Fatalf("Providers.Execute calls = %d, want no construction activity", providers.calls.Load())
	}
	if command.calls.Load() != 0 {
		t.Fatalf("command runner calls = %d, want no construction activity", command.calls.Load())
	}
	if models.calls.Load() != 0 {
		t.Fatalf("Models.InvokeLocal calls = %d, want no construction activity", models.calls.Load())
	}
}

func TestNewServiceDoesNotRegisterHostedRunner(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
		RunnerID: "hosted",
		Roles: []workers.RuntimeBuildRoleRequest{
			{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
		},
	})
	if !errors.Is(err, workers.ErrUnknownRunnerSelection) {
		t.Fatalf("BuildRuntime(hosted) error = %v, want ErrUnknownRunnerSelection", err)
	}
}

type newServiceInputs struct {
	agentDependencies     runners.AgentDependencies
	scriptConfig          runners.ScriptConfig
	scriptDependencies    runners.ScriptDependencies
	inferenceConfig       runners.InferenceConfig
	inferenceDependencies runners.InferenceDependencies
}

func validNewServiceInputs() newServiceInputs {
	return newServiceInputs{
		agentDependencies: runners.AgentDependencies{
			Providers: &wireProvidersFake{},
			Publish:   func(workers.ProgressFragment) {},
		},
		scriptConfig: runners.ScriptConfig{
			Command:          "fixture",
			Args:             []string{"arg"},
			FactoryDirectory: "factory-root",
		},
		scriptDependencies: runners.ScriptDependencies{
			CommandRunner: &wireStreamingCommandRunner{},
			FactoryDocs:   func(string) (map[string]string, error) { return map[string]string{}, nil },
			Now:           func() time.Time { return time.Unix(1, 0) },
			Publish:       func(workers.ProgressFragment) {},
			Record:        func(workers.ScriptEvent) {},
		},
		inferenceConfig: runners.InferenceConfig{
			Worker: models.LocalWorker{
				Name:  "inference-worker",
				Type:  interfaces.WorkerTypeInference,
				Model: "WHISPER",
			},
			Resources: []models.LocalResource{{
				Name: "gpu",
				Type: "gpu",
			}},
		},
		inferenceDependencies: runners.InferenceDependencies{
			Models: &wireInferenceInvoker{},
		},
	}
}

func (in newServiceInputs) callNewService() (workers.Service, error) {
	return NewService(
		in.agentDependencies,
		in.scriptConfig,
		in.scriptDependencies,
		in.inferenceConfig,
		in.inferenceDependencies,
	)
}

type wireProvidersFake struct {
	providers.Service
	calls atomic.Int32
}

func (*wireProvidersFake) ResolveIdentity(
	_ context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	switch strings.ToLower(strings.TrimSpace(request.Identity)) {
	case "codex", "openai":
		return providers.ResolveIdentityResult{ID: providers.IDCodex}, nil
	default:
		return providers.ResolveIdentityResult{}, providers.ErrUnknownProvider
	}
}

func (*wireProvidersFake) ResolveSelection(
	_ context.Context,
	request providers.ResolveSelectionRequest,
) (providers.ResolveSelectionResult, error) {
	for _, candidate := range []struct {
		identity string
		source   providers.SelectionSource
	}{
		{request.Workstation, providers.SelectionSourceWorkstation},
		{request.Factory, providers.SelectionSourceFactory},
		{request.ModelProvider, providers.SelectionSourceLegacyProvider},
	} {
		if strings.TrimSpace(candidate.identity) == "" {
			continue
		}
		resolved, err := (&wireProvidersFake{}).ResolveIdentity(context.Background(), providers.ResolveIdentityRequest{Identity: candidate.identity})
		if err != nil {
			return providers.ResolveSelectionResult{}, err
		}
		return providers.ResolveSelectionResult{Provider: resolved.ID, Source: candidate.source}, nil
	}
	return providers.ResolveSelectionResult{Provider: providers.IDCodex, Source: providers.SelectionSourceDefault}, nil
}

func (*wireProvidersFake) ValidatePrerequisites(
	_ context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	if request.ID != providers.IDCodex {
		return providers.ErrUnknownProvider
	}
	return nil
}

func (fake *wireProvidersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.calls.Add(1)
	result := providers.ExecuteResult{Content: "fixture"}
	if request.ResumeSession != nil {
		session := request.ResumeSession.Clone()
		result.SessionRef = &session
	} else {
		result.SessionRef = &providers.SessionRef{
			Provider: request.Provider,
			Kind:     providers.SessionIDKind,
			ID:       "session-" + request.AttemptID,
		}
	}
	return result, nil
}

func (*wireProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{
		Providers: []providers.Descriptor{selectableCodexDescriptor()},
	}, nil
}

func (*wireProvidersFake) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	if request.ID != providers.IDCodex {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	return providers.GetProviderResult{Provider: selectableCodexDescriptor()}, nil
}

func selectableCodexDescriptor() providers.Descriptor {
	return providers.Descriptor{
		ID:           providers.IDCodex,
		Aliases:      []string{"openai"},
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
	}
}

type wireStreamingCommandRunner struct{}

type recordingWireCommandRunner struct {
	calls atomic.Int32
}

func (runner *recordingWireCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	runner.calls.Add(1)
	panic("command runner invoked during failed construction")
}

func (runner *recordingWireCommandRunner) RunStreaming(
	context.Context,
	workers.CommandRequest,
	platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	runner.calls.Add(1)
	panic("command runner invoked during failed construction")
}

func (*wireStreamingCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func (*wireStreamingCommandRunner) RunStreaming(
	context.Context,
	workers.CommandRequest,
	platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	return workers.CommandResult{ExitCode: 0}, nil
}

type wireInferenceInvoker struct {
	calls atomic.Int32
}

func (invoker *wireInferenceInvoker) InvokeLocal(
	context.Context,
	models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	invoker.calls.Add(1)
	return models.LocalInvocationResult{}, nil
}

var _ providers.Service = (*wireProvidersFake)(nil)
