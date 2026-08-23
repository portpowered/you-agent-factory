package runtimeopening

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type stubWorkersCommandRunner struct{}

func (stubWorkersCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

func recordingMockCommandRunnerFactory(
	built *platformprocess.CommandRunner,
) factoryruntime.WorkersMockCommandRunnerFactory {
	return func(
		mockCfg *workers.MockWorkersConfig,
		_ interfaces.RuntimeDefinitionLookup,
		next platformprocess.CommandRunner,
	) platformprocess.CommandRunner {
		wrapped := &recordingMockCommandRunner{cfg: mockCfg, next: next}
		*built = wrapped
		return wrapped
	}
}

type recordingMockCommandRunner struct {
	cfg  *workers.MockWorkersConfig
	next platformprocess.CommandRunner
}

func (runner *recordingMockCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if runner.next == nil {
		return platformprocess.CommandResult{}, nil
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
		stubWorkersCommandRunner{},
		func(*workers.MockWorkersConfig, interfaces.RuntimeDefinitionLookup, platformprocess.CommandRunner) platformprocess.CommandRunner {
			t.Fatal("mock command runner factory should not run when override is present")
			return nil
		},
		func(platformprocess.CommandRunner) (providers.Service, error) {
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
	var builtRunner platformprocess.CommandRunner
	provider, err := resolveDurableExecutionProvider(
		nil,
		cfg,
		nil,
		baseRunner,
		recordingMockCommandRunnerFactory(&builtRunner),
		func(runner platformprocess.CommandRunner) (providers.Service, error) {
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
		stubWorkersCommandRunner{},
		func(*workers.MockWorkersConfig, interfaces.RuntimeDefinitionLookup, platformprocess.CommandRunner) platformprocess.CommandRunner {
			t.Fatal("mock command runner factory should not run without passthrough unmatched policy")
			return nil
		},
		func(platformprocess.CommandRunner) (providers.Service, error) {
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
		stubWorkersCommandRunner{},
		func(*workers.MockWorkersConfig, interfaces.RuntimeDefinitionLookup, platformprocess.CommandRunner) platformprocess.CommandRunner {
			t.Fatal("mock command runner factory should not run without mock workers")
			return nil
		},
		func(platformprocess.CommandRunner) (providers.Service, error) {
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
