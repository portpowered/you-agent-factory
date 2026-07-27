package construction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/executor/agentrun"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	mockworker "github.com/portpowered/infinite-you/pkg/services/workers/services/testing"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

func TestWithAgentRunnerCutoverPreservesRunnerSelectionWiring(t *testing.T) {
	service := New(
		nil, nil, nil, nil, testFactoryDocs, nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	).WithRunnerSelection(func(_, _, _ string) (workers.ResolvedRunnerSelection, error) {
		return workers.ResolvedRunnerSelection{RunnerID: "codex"}, nil
	}).WithAgentRunnerCutover(true)

	if !service.agentDispatchUsesRegisteredRunner {
		t.Fatal("WithAgentRunnerCutover() did not enable registered agent dispatch")
	}
	if service.resolveRunner == nil {
		t.Fatal("WithAgentRunnerCutover() dropped runner selection wiring")
	}
}

func TestServiceBuildWithAgentRunnerCutoverExposesDispatchAndDirect(t *testing.T) {
	factory, err := workerprovider.NewFactory(
		&mockworker.MockWorkerCommandRunner{},
		workerprocess.ClockFunc(testClock),
		&agypty.MockAllocator{},
		filepath.EvalSymlinks,
		platformprocess.HostExecutableLocator{},
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		"linux",
		platformfilesystem.Local{},
	)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	scriptFactory, err := workerexecutor.NewScriptFactory(
		&mockworker.MockWorkerCommandRunner{},
		workerprocess.ClockFunc(testClock),
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
	).WithAgentRunnerCutover(true)

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

func TestAgentRunnerProviderOverrideBypassesRegisteredRunner(t *testing.T) {
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
	).WithAgentRunnerCutover(true)

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
