package runner

import (
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestResolveRunnerSelectionPrecedenceAndCapabilities(t *testing.T) {
	t.Parallel()
	selection := ResolveRunnerSelection(" antigravity ", workerexecution.RunnerIDClaude, "codex")
	if selection.RunnerID != workerexecution.RunnerIDAntigravity || selection.Source != workerexecution.RunnerSelectionSourceWorkstation {
		t.Fatalf("selection = %#v", selection)
	}
	metadata, ok := BuiltInRunnerMetadata(selection.RunnerID)
	if !ok || metadata.ID != workerexecution.RunnerIDAntigravity {
		t.Fatalf("metadata = %#v, %v", metadata, ok)
	}
	metadata.Capabilities.Optional[0].Detail = "changed"
	again, _ := BuiltInRunnerMetadata(selection.RunnerID)
	if again.Capabilities.Optional[0].Detail == "changed" {
		t.Fatal("runner metadata was not detached")
	}
}

func TestAntigravityRunnerAdvertisesStructuredOutput(t *testing.T) {
	metadata, ok := BuiltInRunnerMetadata(workerexecution.RunnerIDAntigravity)
	if !ok {
		t.Fatal("Antigravity metadata is unavailable")
	}
	for _, capability := range metadata.Capabilities.Optional {
		if capability.Capability == workerexecution.RunnerOptionalCapabilityStructuredOutput {
			if capability.Status != workerexecution.RunnerOptionalCapabilityStatusSupported {
				t.Fatalf("structured output status = %q, want supported", capability.Status)
			}
			return
		}
	}
	t.Fatal("Antigravity metadata does not declare structured output")
}
