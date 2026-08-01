package recordings_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type workersRootPortProbe struct{}

func (workersRootPortProbe) Execute(
	_ context.Context,
	_ workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	return workers.RunnerExecutionResult{}, nil
}

func (workersRootPortProbe) Run(
	_ context.Context,
	_ workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

// TestReplayBindingContractsAcceptWorkersRootPorts proves published replay
// binding contracts name Workers service root ports instead of nested Workers
// implementation packages.
func TestReplayBindingContractsAcceptWorkersRootPorts(t *testing.T) {
	t.Parallel()

	probe := workersRootPortProbe{}
	var provider workers.Runner = probe
	var runner workers.CommandRunner = probe

	binding := recordings.BindReplayExecutionResult{
		Provider:      provider,
		CommandRunner: runner,
	}
	if binding.Provider == nil || binding.CommandRunner == nil {
		t.Fatal("BindReplayExecutionResult must accept workers root ports")
	}

	var factory recordings.ReplayExecutionFactory = func(
		_ *recordings.ReplayArtifact,
	) (
		workers.Runner,
		workers.CommandRunner,
		[]recordings.ReplayHook,
		recordings.CompletionDeliveryPlanner,
		error,
	) {
		return provider, runner, nil, nil, nil
	}
	if factory == nil {
		t.Fatal("ReplayExecutionFactory must be constructible with workers root ports")
	}

	p, r, _, _, err := factory(&recordings.ReplayArtifact{SchemaVersion: "replay.v1"})
	if err != nil {
		t.Fatalf("ReplayExecutionFactory: %v", err)
	}
	if p == nil || r == nil {
		t.Fatalf("factory ports = (%v,%v), want non-nil provider and runner", p, r)
	}
}
