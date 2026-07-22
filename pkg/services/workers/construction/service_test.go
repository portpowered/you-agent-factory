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
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/executor/agentrun"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	mockworker "github.com/portpowered/infinite-you/pkg/services/workers/services/testing"
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
		"script": {Name: "script", Type: interfaces.WorkerTypeScript},
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

type providerStub struct{}

func testClock() time.Time { return time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC) }

var testRetryRandom = platformrandom.SourceFunc(func(int64) (int64, error) {
	return 0, nil
})

func testFactoryDocs(string) (map[string]string, error) { return map[string]string{}, nil }

func (providerStub) Infer(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{}, nil
}
