package interfaces

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestRunnerMetadataFromDispatchRequestMetadata_MapsModelProviderFields(t *testing.T) {
	modelProvider := factoryapi.WorkerModelProviderGemini
	source := factoryapi.ModelProviderSelectionSourceFactory
	metadata := &factoryapi.DispatchRequestEventMetadata{
		ModelProvider:                &modelProvider,
		ModelProviderSelectionSource: &source,
	}

	runnerID, selectionSource := RunnerMetadataFromDispatchRequestMetadata(metadata)
	if runnerID != RunnerIDGemini {
		t.Fatalf("runnerID = %q, want %q", runnerID, RunnerIDGemini)
	}
	if selectionSource != RunnerSelectionSourceFactory {
		t.Fatalf("selection source = %q, want %q", selectionSource, RunnerSelectionSourceFactory)
	}
}

func TestPublicModelProviderFromLegacyRunnerID_MapsBuiltInRunnerIDs(t *testing.T) {
	public, err := PublicModelProviderFromLegacyRunnerID("cursor-cli")
	if err != nil {
		t.Fatalf("PublicModelProviderFromLegacyRunnerID: %v", err)
	}
	if public != factoryapi.WorkerModelProviderCursor {
		t.Fatalf("modelProvider = %q, want %q", public, factoryapi.WorkerModelProviderCursor)
	}
}

func TestPublicModelProviderFromLegacyRunnerID_RejectsUnknownValues(t *testing.T) {
	_, err := PublicModelProviderFromLegacyRunnerID("mystery-runner")
	if err == nil {
		t.Fatal("expected error for unknown legacy runnerId")
	}
	if !strings.Contains(err.Error(), `unknown legacy runnerId "mystery-runner"`) {
		t.Fatalf("error = %q, want legacy runnerId naming", err)
	}
}

func TestPublicModelProviderSelectionSourceFromLegacyRunnerSelectionSource_MapsLegacyAliases(t *testing.T) {
	got := PublicModelProviderSelectionSourceFromLegacyRunnerSelectionSource("legacy_provider")
	if got != factoryapi.ModelProviderSelectionSourceWorker {
		t.Fatalf("selection source = %q, want %q", got, factoryapi.ModelProviderSelectionSourceWorker)
	}
}
