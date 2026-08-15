package replay

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestReplaySideEffectsSatisfyProvidersRootPorts proves replay side effects
// satisfy the Providers root and the Workers command effect at the
// Recordings/Workers boundary.
func TestReplaySideEffectsSatisfyProvidersRootPorts(t *testing.T) {
	t.Parallel()

	sideEffects, err := NewSideEffects(
		testFactorySnapshotDecoder,
		testRuntimeConfigDecoder,
		replaySideEffectArtifact(t),
	)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	var provider providers.Service = sideEffects
	var runner workers.CommandRunner = sideEffects
	if provider == nil || runner == nil {
		t.Fatal("replay side effects must satisfy workers root ports")
	}

	providerRequest := providers.ExecuteRequest{
		AttemptID:       "provider-dispatch",
		Provider:        providers.ID("claude"),
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Model:           "claude-3-5-haiku-20241022",
		SystemPrompt:    "system prompt",
		UserMessage:     "user prompt",
		Correlation: providers.ExecuteCorrelation{
			ReplayKey: "process/trace-1/work-1",
			TraceID:   "trace-1",
			WorkIDs:   []string{"work-1"},
		},
	}
	resp, err := provider.Execute(context.Background(), providerRequest)
	if err != nil {
		t.Fatalf("Execute through providers.Service: %v", err)
	}
	if resp.Content != "recorded provider output" {
		t.Fatalf("provider content = %q, want recorded provider output", resp.Content)
	}

	commandRequest := workers.CommandRequest{
		Command: "echo",
		Args:    []string{"ok"},
		Execution: work.ExecutionMetadata{
			ReplayKey: "process/trace-2/work-2",
			TraceID:   "trace-2",
			WorkIDs:   []string{"work-2"},
		},
	}
	result, err := runner.Run(context.Background(), commandRequest)
	if err != nil {
		t.Fatalf("Run through workers.CommandRunner: %v", err)
	}
	if string(result.Stdout) != "recorded script output\n" {
		t.Fatalf("stdout = %q, want recorded script output", result.Stdout)
	}
}
