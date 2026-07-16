package runner

import (
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestResolveRunnerSelectionPrecedenceAndCapabilities(t *testing.T) {
	t.Parallel()
	selection := ResolveRunnerSelection(" opencode ", workerexecution.RunnerIDGemini, "codex")
	if selection.RunnerID != workerexecution.RunnerIDOpenCode || selection.Source != workerexecution.RunnerSelectionSourceWorkstation {
		t.Fatalf("selection = %#v", selection)
	}
	metadata, ok := BuiltInRunnerMetadata(selection.RunnerID)
	if !ok || metadata.ID != workerexecution.RunnerIDOpenCode {
		t.Fatalf("metadata = %#v, %v", metadata, ok)
	}
	metadata.Capabilities.Optional[0].Detail = "changed"
	again, _ := BuiltInRunnerMetadata(selection.RunnerID)
	if again.Capabilities.Optional[0].Detail == "changed" {
		t.Fatal("runner metadata was not detached")
	}
}

func TestValidateOpenCodeAgentForRunnerSelection(t *testing.T) {
	t.Parallel()
	if err := ValidateOpenCodeAgentForRunnerSelection("reviewer", "", workerexecution.ResolvedRunnerSelection{RunnerID: workerexecution.RunnerIDCodex}); err == nil {
		t.Fatal("expected incompatible OpenCode agent error")
	}
	if err := ValidateOpenCodeAgentForRunnerSelection("reviewer", "", workerexecution.ResolvedRunnerSelection{RunnerID: workerexecution.RunnerIDOpenCode}); err != nil {
		t.Fatal(err)
	}
}
