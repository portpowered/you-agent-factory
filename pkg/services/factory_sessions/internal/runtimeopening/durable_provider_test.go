package runtimeopening

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type stubWorkersCommandRunner struct{}

func (stubWorkersCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func recordingMockCommandRunnerFactory(
	built *workers.CommandRunner,
) factoryruntime.WorkersMockCommandRunnerFactory {
	return func(
		mockCfg *workers.MockWorkersConfig,
		_ interfaces.RuntimeDefinitionLookup,
		next workers.CommandRunner,
	) workers.CommandRunner {
		wrapped := &recordingMockCommandRunner{cfg: mockCfg, next: next}
		*built = wrapped
		return wrapped
	}
}

type recordingMockCommandRunner struct {
	cfg  *workers.MockWorkersConfig
	next workers.CommandRunner
}

func (runner *recordingMockCommandRunner) Run(
	ctx context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	if runner.next == nil {
		return workers.CommandResult{}, nil
	}
	return runner.next.Run(ctx, request)
}

func TestResolveDurableExecutionProvider_PrefersOverride(t *testing.T) {
	t.Parallel()

	override := testutil.NewMockProvider(workers.InferenceResponse{Content: "override"})
	provider, err := resolveDurableExecutionProvider(
		override,
		&workers.MockWorkersConfig{},
		nil,
		testutil.NewProviderCommandRunner(),
		func(platformprocess.CommandRunner) workers.CommandRunner { return stubWorkersCommandRunner{} },
		func(*workers.MockWorkersConfig, interfaces.RuntimeDefinitionLookup, workers.CommandRunner) workers.CommandRunner {
			t.Fatal("mock command runner factory should not run when override is present")
			return nil
		},
		func(workers.CommandRunner) (workers.Provider, error) {
			t.Fatal("build provider should not run when override is present")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("resolveDurableExecutionProvider: %v", err)
	}
	if provider != override {
		t.Fatalf("provider = %#v, want override %#v", provider, override)
	}
}

func TestResolveDurableExecutionProvider_BuildsFromMockWrappedRunner(t *testing.T) {
	t.Parallel()

	baseRunner := stubWorkersCommandRunner{}
	cfg := &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName: "worker-a",
			RunType:    workers.MockWorkerRunTypeAccept,
		}},
	}
	var builtRunner workers.CommandRunner
	provider, err := resolveDurableExecutionProvider(
		nil,
		cfg,
		nil,
		testutil.NewProviderCommandRunner(),
		func(platformprocess.CommandRunner) workers.CommandRunner { return baseRunner },
		recordingMockCommandRunnerFactory(&builtRunner),
		func(runner workers.CommandRunner) (workers.Provider, error) {
			if runner != builtRunner {
				t.Fatalf("build provider runner = %#v, want wrapped %#v", runner, builtRunner)
			}
			return testutil.NewMockProvider(workers.InferenceResponse{Content: "built"}), nil
		},
	)
	if err != nil {
		t.Fatalf("resolveDurableExecutionProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("provider = nil, want built provider")
	}
}

func TestResolveDurableExecutionProvider_ReturnsNilWithoutPassthroughPolicy(t *testing.T) {
	t.Parallel()

	provider, err := resolveDurableExecutionProvider(
		nil,
		workers.NewEmptyMockWorkersConfig(),
		nil,
		testutil.NewProviderCommandRunner(),
		func(platformprocess.CommandRunner) workers.CommandRunner { return stubWorkersCommandRunner{} },
		func(*workers.MockWorkersConfig, interfaces.RuntimeDefinitionLookup, workers.CommandRunner) workers.CommandRunner {
			t.Fatal("mock command runner factory should not run without passthrough unmatched policy")
			return nil
		},
		func(workers.CommandRunner) (workers.Provider, error) {
			t.Fatal("build provider should not run without passthrough unmatched policy")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("resolveDurableExecutionProvider: %v", err)
	}
	if provider != nil {
		t.Fatalf("provider = %#v, want nil", provider)
	}
}

func TestResolveDurableExecutionProvider_ReturnsNilWithoutMockWorkers(t *testing.T) {
	t.Parallel()

	provider, err := resolveDurableExecutionProvider(
		nil,
		nil,
		nil,
		testutil.NewProviderCommandRunner(),
		func(platformprocess.CommandRunner) workers.CommandRunner { return stubWorkersCommandRunner{} },
		func(*workers.MockWorkersConfig, interfaces.RuntimeDefinitionLookup, workers.CommandRunner) workers.CommandRunner {
			t.Fatal("mock command runner factory should not run without mock workers")
			return nil
		},
		func(workers.CommandRunner) (workers.Provider, error) {
			t.Fatal("build provider should not run without mock workers")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("resolveDurableExecutionProvider: %v", err)
	}
	if provider != nil {
		t.Fatalf("provider = %#v, want nil", provider)
	}
}
