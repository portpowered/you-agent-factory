package wire

import (
	"context"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRuntimeBridgeLocalRuntimeHooksConstructs(t *testing.T) {
	t.Parallel()

	_ = LocalRuntimeHooks()
}

func TestRuntimeBridgeNewMockCommandRunnerDecoratesNext(t *testing.T) {
	t.Parallel()

	next := stubRuntimeBridgeCommandRunner{}
	decorated := NewMockCommandRunner(nil, nil, next)
	if decorated == nil {
		t.Fatal("NewMockCommandRunner() returned nil")
	}
}

type stubRuntimeBridgeCommandRunner struct{}

func (stubRuntimeBridgeCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func (stubRuntimeBridgeCommandRunner) RunStreaming(
	context.Context,
	workers.CommandRequest,
	platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}
