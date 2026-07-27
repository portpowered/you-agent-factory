package runtimeopening

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	workersservice "github.com/portpowered/infinite-you/pkg/services/workers/service"
)

type stubWorkersCommandRunner struct{}

func (stubWorkersCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
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
		workersservice.NewMockCommandRunner,
		func(workers.CommandRunner) (workerprovider.Provider, error) {
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
		func(mockCfg *workers.MockWorkersConfig, _ interfaces.RuntimeDefinitionLookup, next workers.CommandRunner) workers.CommandRunner {
			if next != baseRunner {
				t.Fatalf("next runner = %#v, want base runner %#v", next, baseRunner)
			}
			builtRunner = workersservice.NewMockCommandRunner(mockCfg, nil, next)
			return builtRunner
		},
		func(runner workers.CommandRunner) (workerprovider.Provider, error) {
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
		workersservice.NewMockCommandRunner,
		func(workers.CommandRunner) (workerprovider.Provider, error) {
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
		workersservice.NewMockCommandRunner,
		func(workers.CommandRunner) (workerprovider.Provider, error) {
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
