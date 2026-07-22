package checkpointfixtures

import (
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestCheckpointSummariesFixtureReturnsConfiguredResults(t *testing.T) {
	t.Parallel()

	buildResult := &factoryruntime.JavaScriptCheckpointSummary{CheckpointID: "build-result"}
	latestResult := &factoryruntime.JavaScriptCheckpointSummary{
		CheckpointID:         "latest-result",
		CompletedDispatchIDs: []string{"dispatch-1"},
	}
	fixture := CheckpointSummariesFixture{BuildResult: buildResult, LatestResult: latestResult}

	built := fixture.Build(factoryruntime.JavaScriptCheckpointSummaryInput{CheckpointID: "ignored"})
	if built == nil || built.CheckpointID != "build-result" {
		t.Fatalf("Build = %#v, want configured result", built)
	}
	latest := fixture.Latest(factoryruntime.JavaScriptCheckpointSummaryInput{CheckpointID: "ignored"})
	if latest == nil || latest.CheckpointID != "latest-result" {
		t.Fatalf("Latest = %#v, want configured result", latest)
	}
	latest.CompletedDispatchIDs[0] = "mutated"
	if latestResult.CompletedDispatchIDs[0] != "dispatch-1" {
		t.Fatal("Latest mutated configured fixture result")
	}
}
