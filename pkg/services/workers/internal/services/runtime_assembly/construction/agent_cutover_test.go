package construction

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	mockworker "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/testing"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

func TestWithRunnerSelectionPreservesRunnerSelectionWiring(t *testing.T) {
	service := New(
		nil, nil, nil, nil, testFactoryDocs, nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	).WithRunnerSelection(func(_, _, _ string) (workers.ResolvedRunnerSelection, error) {
		return workers.ResolvedRunnerSelection{RunnerID: "codex"}, nil
	})

	if service.resolveRunner == nil {
		t.Fatal("WithRunnerSelection() dropped runner selection wiring")
	}
}

func TestServiceBuildExposesDispatchAndDirect(t *testing.T) {
	factory := &cutoverProvidersFake{}
	scriptFactory, err := workerexecutor.NewScriptFactory(
		&mockworker.MockWorkerCommandRunner{},
		workers.ClockFunc(testClock),
		testFactoryDocs,
	)
	if err != nil {
		t.Fatalf("NewScriptFactory() error = %v", err)
	}

	service := New(
		factory,
		scriptFactory,
		nil,
		nil,
		testFactoryDocs,
		nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	)

	result, err := service.Build(
		runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"model": {Name: "model", Type: interfaces.WorkerTypeModel},
			},
		},
		"model",
		"",
		nil,
		logging.NoopLogger{},
		nil,
		nil,
		func(workers.ProgressFragment) {},
		nil,
		nil,
		nil,
		testClock,
		os.Environ,
		os.Getwd,
		nil,
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Dispatch == nil || result.Direct == nil {
		t.Fatalf("Build() = %#v, want dispatch and direct executors", result)
	}
}

func TestServiceBuildWithNilProgressPublisher(t *testing.T) {
	factory := &cutoverProvidersFake{}
	service := New(
		factory,
		nil,
		nil,
		nil,
		testFactoryDocs,
		nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	)

	_, err := service.Build(
		runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"model": {Name: "model", Type: interfaces.WorkerTypeModel},
			},
		},
		"model",
		"",
		nil,
		logging.NoopLogger{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		testClock,
		os.Environ,
		os.Getwd,
		nil,
	)
	if err != nil {
		t.Fatalf("Build() with nil progress publisher error = %v", err)
	}
}

func TestAgentRunnerProviderOverrideBuildsRunner(t *testing.T) {
	service := New(
		nil,
		nil,
		nil,
		nil,
		testFactoryDocs,
		nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	)

	runner, err := service.agentRunner(
		runtimefixtures.RuntimeConfigLookupFixture{},
		&interfaces.FactoryWorkerConfig{Type: interfaces.WorkerTypeModel},
		logging.NoopLogger{},
		false,
		providerStub{},
		nil,
	)
	if err != nil {
		t.Fatalf("agentRunner() error = %v", err)
	}
	if runner == nil {
		t.Fatal("provider override returned nil runner")
	}
}

func TestRegistryRunnerPreservesInferenceRequestContract(t *testing.T) {
	registry := &captureRunnerRegistry{}
	runner := registryRunner{registry: registry, identity: runners.InferenceIdentity}
	request := workers.RunnerExecutionRequest{
		RunnerID:       workers.RunnerIDCodex,
		ModelOperation: "transcribe",
		ModelBindings: []workers.ResolvedModelOperationBinding{{
			Slot:   "audio",
			Source: workers.ModelOperationBindingSourceInput,
		}},
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityWorkingDirectory,
		},
	}
	if _, err := runner.Execute(t.Context(), request); err != nil {
		t.Fatalf("registryRunner.Execute() error = %v", err)
	}
	if registry.request.Identity != runners.InferenceIdentity {
		t.Fatalf("registry identity = %q, want inference", registry.request.Identity)
	}
	attempt := registry.request.Attempt
	if attempt.RunnerID != request.RunnerID || attempt.ModelOperation != request.ModelOperation {
		t.Fatalf("inference selection fields = %#v, want runner %q and operation %q", attempt, request.RunnerID, request.ModelOperation)
	}
	if len(attempt.ModelBindings) != 1 || attempt.ModelBindings[0].Slot != request.ModelBindings[0].Slot {
		t.Fatalf("inference model bindings = %#v, want caller binding", attempt.ModelBindings)
	}
	if len(registry.request.RequiredCapabilities) != 1 ||
		registry.request.RequiredCapabilities[0] != request.RequiredOptionalCapabilities[0] {
		t.Fatalf("required capabilities = %#v, want caller capabilities", registry.request.RequiredCapabilities)
	}
}

type captureRunnerRegistry struct {
	request runners.ExecuteRequest
}

func (registry *captureRunnerRegistry) Resolve(runners.ResolutionRequest) (runners.Binding, error) {
	return runners.Binding{}, nil
}

func (registry *captureRunnerRegistry) Execute(
	_ context.Context,
	request runners.ExecuteRequest,
) (runners.ExecuteResult, error) {
	registry.request = request
	return runners.ExecuteResult{Content: "fixture"}, nil
}

func TestProvidersRootMapsThrottledFailure(t *testing.T) {
	_, err := (&cutoverProvidersFake{
		err: providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindThrottled,
			Message: "saturation",
		},
	}).Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "attempt-1",
		UserMessage: "hello",
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want throttled failure")
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindThrottled {
		t.Fatalf("Execute() error = %v, want throttled ExecuteFailure", err)
	}
}

type cutoverProvidersFake struct {
	providers.Service
	err error
}

func (fake *cutoverProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (fake *cutoverProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, providers.ErrUnknownProvider
}

func (fake *cutoverProvidersFake) Execute(
	_ context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if fake.err != nil {
		return providers.ExecuteResult{}, fake.err
	}
	return providers.ExecuteResult{Content: "ok"}, nil
}
