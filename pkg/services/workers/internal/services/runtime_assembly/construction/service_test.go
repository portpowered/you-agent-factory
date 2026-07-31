package construction

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	mockworker "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/testing"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

func TestServiceBuildProviderBackedExposesDispatchAndDirectBoundaries(t *testing.T) {
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{Workers: map[string]*interfaces.FactoryWorkerConfig{
		"model": {Name: "model", Type: interfaces.WorkerTypeModel},
	}}
	decorated := false
	result, err := New(nil, nil, nil, nil, testFactoryDocs, nil, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}), testRetryRandom, platformfilesystem.Local{}).Build(
		runtimeConfig, "model", "", nil, logging.NoopLogger{}, nil,
		providerStub{}, nil, nil, nil, nil, testClock, os.Environ, os.Getwd,
		[]RunnerDecorator{func(runner workers.Runner, _ *interfaces.FactoryWorkerConfig) workers.Runner {
			decorated = true
			return runner
		}},
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Dispatch == nil || result.Direct == nil {
		t.Fatalf("Build() = %#v, want dispatch and direct executors", result)
	}
	if !decorated {
		t.Fatal("Build() did not apply the injected runner decorator")
	}
}

func TestEffectiveSkipPermissionsRunnerAppliesPolicyOutsideDecorators(t *testing.T) {
	t.Parallel()

	captured := workerexecution.ProviderInferenceRequest{}
	inner := runnerFunc(func(_ context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
		captured = request
		return workers.RunnerExecutionResult{}, nil
	})
	runner := effectiveSkipPermissionsRunner{next: inner, enabled: true}

	if _, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{UserMessage: "fixture"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !captured.SkipPermissions {
		t.Fatal("Execute() omitted invocation-effective skip-permissions policy")
	}
}

func TestServiceBuildLogicalWorkerHasDispatchOnly(t *testing.T) {
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{Workers: map[string]*interfaces.FactoryWorkerConfig{
		"logical": {Name: "logical", Type: interfaces.WorkstationTypeLogical},
	}}
	result, err := New(nil, nil, nil, nil, testFactoryDocs, nil, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}), testRetryRandom, platformfilesystem.Local{}).Build(
		runtimeConfig, "logical", "", nil, logging.NoopLogger{}, nil,
		nil, nil, nil, nil, nil, testClock, os.Environ, os.Getwd, nil,
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Dispatch == nil || result.Direct != nil {
		t.Fatalf("Build() = %#v, want dispatch-only logical executor", result)
	}
}

func TestServiceBuildScriptExposesDispatchAndDirectBoundaries(t *testing.T) {
	if _, err := workerexecutor.NewScriptFactory(&mockworker.MockWorkerCommandRunner{}, nil, testFactoryDocs); err == nil {
		t.Fatal("NewScriptFactory() succeeded without command clock")
	}
	scriptFactory, err := workerexecutor.NewScriptFactory(&mockworker.MockWorkerCommandRunner{}, workerprocess.ClockFunc(testClock), testFactoryDocs)
	if err != nil {
		t.Fatalf("NewScriptFactory() error = %v", err)
	}
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{Workers: map[string]*interfaces.FactoryWorkerConfig{
		"script": {Name: "script", Type: interfaces.WorkerTypeScript, Command: "script-tool"},
	}}
	result, err := New(nil, scriptFactory, nil, nil, testFactoryDocs, nil, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}), testRetryRandom, platformfilesystem.Local{}).Build(
		runtimeConfig, "script", "", nil, logging.NoopLogger{}, nil,
		nil, nil, nil, nil, nil, testClock, os.Environ, os.Getwd, nil,
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Dispatch == nil || result.Direct == nil {
		t.Fatalf("Build() = %#v, want dispatch and direct executors", result)
	}
}

func TestServiceBuildUnknownWorkerReturnsEmptyResult(t *testing.T) {
	result, err := New(nil, nil, nil, nil, testFactoryDocs, nil, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}), testRetryRandom, platformfilesystem.Local{}).Build(
		runtimefixtures.RuntimeConfigLookupFixture{}, "missing", "", nil,
		logging.NoopLogger{}, nil, nil, nil, nil, nil, nil, testClock, os.Environ, os.Getwd, nil,
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Dispatch != nil || result.Direct != nil {
		t.Fatalf("Build() = %#v, want empty result", result)
	}
}

func TestServiceBuildRequiresRuntimeConfig(t *testing.T) {
	if _, err := New(nil, nil, nil, nil, testFactoryDocs, nil, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}), testRetryRandom, platformfilesystem.Local{}).Build(
		nil, "", "", nil, logging.NoopLogger{}, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
	); err == nil {
		t.Fatal("Build() succeeded without runtime config")
	}
}

func TestServiceWithRunnerSelectionReturnsConfiguredCopy(t *testing.T) {
	t.Parallel()

	if configured := (*Service)(nil).WithRunnerSelection(nil); configured != nil {
		t.Fatalf("nil Service.WithRunnerSelection() = %#v, want nil", configured)
	}

	service := New(
		nil, nil, nil, nil, testFactoryDocs, nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	)
	resolver := func(workstation, factory, worker string) (workers.ResolvedRunnerSelection, error) {
		return workers.ResolvedRunnerSelection{
			RunnerID: workstation + factory + worker,
			Source:   workers.RunnerSelectionSourceWorkstation,
		}, nil
	}
	configured := service.WithRunnerSelection(resolver)
	if configured == service {
		t.Fatal("WithRunnerSelection() mutated the original service")
	}
	if service.resolveRunner != nil {
		t.Fatal("WithRunnerSelection() configured the original service")
	}
	selection, err := configured.resolveRunner("workstation", "factory", "worker")
	if err != nil {
		t.Fatalf("configured resolver error = %v", err)
	}
	if selection.RunnerID != "workstationfactoryworker" {
		t.Fatalf("configured resolver selection = %#v", selection)
	}
}

func TestServiceWithExecutionFactoriesPreservesRunnerAndProviderWiring(t *testing.T) {
	t.Parallel()

	if configured := (*Service)(nil).WithExecutionFactories(nil, nil); configured != nil {
		t.Fatalf("nil Service.WithExecutionFactories() = %#v, want nil", configured)
	}

	service := New(
		nil, nil, nil, nil, testFactoryDocs, nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	)
	resolveRunner := func(workstation, factory, worker string) (workers.ResolvedRunnerSelection, error) {
		return workers.ResolvedRunnerSelection{
			RunnerID: "customer.provider",
			Source:   workers.RunnerSelectionSourceLegacyProvider,
		}, nil
	}
	resolveProvider := func(identity string) (string, error) {
		return "customer.provider", nil
	}
	configured := service.
		WithRunnerSelection(resolveRunner).
		WithProviderIdentityResolution(resolveProvider)
	rebuilt := configured.WithExecutionFactories(nil, nil)
	if rebuilt == configured {
		t.Fatal("WithExecutionFactories() mutated the configured service")
	}
	if rebuilt.resolveRunner == nil || rebuilt.resolveProvider == nil {
		t.Fatal("WithExecutionFactories() dropped registry selection wiring")
	}
	selection, err := rebuilt.resolveRunner("", "", "customer.provider")
	if err != nil {
		t.Fatalf("preserved resolver error = %v", err)
	}
	if selection.RunnerID != "customer.provider" {
		t.Fatalf("preserved resolver selection = %#v", selection)
	}
	canonical, err := rebuilt.resolveProvider("customer")
	if err != nil {
		t.Fatalf("preserved identity resolver error = %v", err)
	}
	if canonical != "customer.provider" {
		t.Fatalf("preserved identity resolver = %q, want customer.provider", canonical)
	}
}

type providerStub struct{}

type runnerFunc func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error)

func (fn runnerFunc) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	return fn(ctx, request)
}

func testClock() time.Time { return time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC) }

var testRetryRandom = platformrandom.SourceFunc(func(int64) (int64, error) {
	return 0, nil
})

func testFactoryDocs(string) (map[string]string, error) { return map[string]string{}, nil }

func (providerStub) Infer(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{}, nil
}
