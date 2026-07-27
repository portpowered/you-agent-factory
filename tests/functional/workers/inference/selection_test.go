package inference_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestExplicitProviderAndModelReachSelectedProviderEdge proves that when a worker
// declares an explicit provider and model, root.BuildProcess dispatch invokes the
// matching registered provider-process edge, completes factory dispatch through
// that edge, and does not invoke a different registered provider edge for the
// same work.
func TestExplicitProviderAndModelReachSelectedProviderEdge(t *testing.T) {
	const (
		selectedProviderID    = "selected.provider"
		selectedProviderAlias = "selected"
		alternateProviderID   = "alternate.provider"
		alternateProviderAlias = "alternate"
		explicitModel         = "explicit-selection-model"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	writeExplicitSelectionWorker(t, dir, selectedProviderAlias, explicitModel)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"explicit provider selection"}`))

	selectedIntegration := inference.ProgressingExternalIntegration(
		selectedProviderID,
		"structured progress COMPLETE",
	)
	alternateIntegration := inference.ProgressingExternalIntegration(
		alternateProviderID,
		"alternate provider must not run",
	)

	selectedManifest := externalProviderManifest(t, selectedProviderID, selectedProviderAlias)
	alternateManifest := externalProviderManifest(t, alternateProviderID, alternateProviderAlias)

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{
			{Manifest: selectedManifest, Integration: selectedIntegration},
			{Manifest: alternateManifest, Integration: alternateIntegration},
		},
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}

	selectedStats := selectedIntegration.Stats()
	if selectedStats.InvokeCalls != 1 {
		t.Fatalf("selected provider invoke calls = %d, want 1", selectedStats.InvokeCalls)
	}
	if selectedStats.ProgressWrites < 1 {
		t.Fatalf(
			"selected provider progress writes = %d, want at least 1 through the conductor response writer",
			selectedStats.ProgressWrites,
		)
	}
	if selectedStats.TerminalCloses != 1 {
		t.Fatalf("selected provider terminal closes = %d, want exactly one terminal outcome", selectedStats.TerminalCloses)
	}
	if selectedStats.DiscoverBeforeInvoke != 0 || selectedStats.CapabilitiesBeforeInvoke != 0 {
		t.Fatalf(
			"selected provider I/O before invoke = discover:%d capabilities:%d, want zero until dispatch",
			selectedStats.DiscoverBeforeInvoke,
			selectedStats.CapabilitiesBeforeInvoke,
		)
	}

	alternateStats := alternateIntegration.Stats()
	if alternateStats.InvokeCalls != 0 {
		t.Fatalf(
			"alternate provider invoke calls = %d, want 0 when worker selected %q",
			alternateStats.InvokeCalls,
			selectedProviderAlias,
		)
	}
	if alternateStats.ProgressWrites != 0 || alternateStats.TerminalCloses != 0 {
		t.Fatalf(
			"alternate provider side effects = progress:%d terminal:%d, want inert when not selected",
			alternateStats.ProgressWrites,
			alternateStats.TerminalCloses,
		)
	}
}

func writeExplicitSelectionWorker(t *testing.T, factoryDir, provider, model string) {
	t.Helper()
	workerPath := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	worker := strings.Join([]string{
		"---",
		"model: " + model,
		"modelProvider: " + provider,
		"stopToken: COMPLETE",
		"type: MODEL_WORKER",
		"---",
		"",
		"Test worker for explicit provider and model selection.",
		"",
	}, "\n")
	if err := os.WriteFile(workerPath, []byte(worker), 0o600); err != nil {
		t.Fatalf("write explicit selection worker: %v", err)
	}
}
