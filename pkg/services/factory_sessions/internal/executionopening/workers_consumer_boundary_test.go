package executionopening

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const workersImportRoot = "github.com/portpowered/infinite-you/pkg/services/workers"

// TestExecutionOpeningPackagesImportWorkersOnlyThroughRoot seals execution-opening
// and durable-provider construction call sites to the Workers service root contract.
func TestExecutionOpeningPackagesImportWorkersOnlyThroughRoot(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-test",
		"-f",
		"{{.ImportPath}} {{join .Imports \" \"}}",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/executionopening/...",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list executionopening packages: %v\n%s", err, output)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		pkgPath := fields[0]
		for _, imp := range fields[1:] {
			if imp == workersImportRoot {
				continue
			}
			if strings.HasPrefix(imp, workersImportRoot+"/") {
				t.Fatalf(
					"%s must import Workers only through %s; found direct import %s",
					pkgPath,
					workersImportRoot,
					imp,
				)
			}
		}
	}
}

// TestExecutionOpeningFactoryRolesNameWorkersRootContracts proves execution-opening
// construction helpers type Workers-facing bindings only through the Workers
// service root.
func TestExecutionOpeningFactoryRolesNameWorkersRootContracts(t *testing.T) {
	t.Parallel()

	var (
		_ StandaloneSessionExecutionFactory
		_ WorkerInvocationFactory
		_ WorkerInvocationWithProgressFactory
	)

	var _ *Factory = &Factory{
		commandRunner: workersRootBindingProbeRunner{},
		allocator:     &workers.MockPTYAllocator{},
	}
}

type workersRootBindingProbeRunner struct{}

func (workersRootBindingProbeRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

type workersRootInvocationProbe struct{}

func (workersRootInvocationProbe) Execute(
	context.Context,
	workers.InvocationInput,
) (workers.InvocationResult, error) {
	return workers.InvocationResult{}, nil
}

func TestBuildWithWorkerEffectsForwardsWorkersRootInvocationBindings(t *testing.T) {
	t.Parallel()

	ptyAllocator := &workers.MockPTYAllocator{}
	commandRunner := workersRootBindingProbeRunner{}
	var gotRunner workers.CommandRunner
	var gotPTY workers.PTYAllocator

	factory := &Factory{
		standalone: func(
			_ factorysessions.ExecutionProvider,
			_ string,
			_ string,
			_ string,
			executor workers.InvocationExecutor,
			_ factory.Clock,
		) (factorysessions.ExecutionService, error) {
			if executor == nil {
				t.Fatal("worker invocation executor is required for live child execution")
			}
			return nil, nil
		},
		invocation: func(
			runner workers.CommandRunner,
			pty workers.PTYAllocator,
		) (workers.InvocationExecutor, error) {
			gotRunner = runner
			gotPTY = pty
			return workersRootInvocationProbe{}, nil
		},
		resolveClock: func(factory.Clock) factory.Clock { return nil },
		commandRunner: commandRunner,
		allocator:     ptyAllocator,
	}

	_, err := factory.buildWithWorkerEffects(
		context.Background(),
		string(factorysessions.ExecutionProviderJavaScriptRuntime),
		t.TempDir(),
		"",
		factorysessions.ChildExecutorModeLive,
	)
	if err != nil {
		t.Fatalf("buildWithWorkerEffects() error = %v, want nil", err)
	}
	if gotRunner != commandRunner {
		t.Fatalf("command runner = %#v, want %#v", gotRunner, commandRunner)
	}
	if gotPTY != ptyAllocator {
		t.Fatalf("PTY allocator = %#v, want %#v", gotPTY, ptyAllocator)
	}
}
