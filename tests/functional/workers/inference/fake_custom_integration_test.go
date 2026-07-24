package inference_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFakeCustomIntegrationCompletesFactoryDispatchThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	writeExternalProviderWorker(t, dir, "customer.provider")
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"fake custom integration"}`))

	integration := inference.ProgressingExternalIntegration("customer.provider", "structured progress COMPLETE")
	manifest := externalProviderManifest(t, "customer.provider", "customer")

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{{
			Manifest:    manifest,
			Integration: integration,
		}},
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}

	stats := integration.Stats()
	if stats.InvokeCalls != 1 {
		t.Fatalf("fake integration invoke calls = %d, want 1", stats.InvokeCalls)
	}
	if stats.ProgressWrites < 1 {
		t.Fatalf("structured progress writes = %d, want at least 1 through the conductor response writer", stats.ProgressWrites)
	}
	if stats.TerminalCloses != 1 {
		t.Fatalf("terminal closes = %d, want exactly one terminal outcome", stats.TerminalCloses)
	}
	if stats.DiscoverBeforeInvoke != 0 || stats.CapabilitiesBeforeInvoke != 0 {
		t.Fatalf(
			"provider I/O before invoke = discover:%d capabilities:%d, want zero until dispatch",
			stats.DiscoverBeforeInvoke,
			stats.CapabilitiesBeforeInvoke,
		)
	}
}

func TestFakeCustomIntegrationRemainsInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	integration := inference.ProgressingExternalIntegration("customer.provider", "structured progress COMPLETE")
	manifest := externalProviderManifest(t, "customer.provider", "customer")
	_ = support.BuildProcess(t, serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{{
			Manifest:    manifest,
			Integration: integration,
		}},
	})

	stats := integration.Stats()
	if stats.InvokeCalls != 0 || stats.ProgressWrites != 0 || stats.TerminalCloses != 0 ||
		stats.DiscoverCalls != 0 || stats.CapabilityCalls != 0 {
		t.Fatalf("construction side effects = %#v, want inert registry composition", stats)
	}
}

func writeExternalProviderWorker(t *testing.T, factoryDir, provider string) {
	t.Helper()
	workerPath := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	worker := strings.Join([]string{
		"---",
		"model: test-model",
		"modelProvider: " + provider,
		"stopToken: COMPLETE",
		"type: MODEL_WORKER",
		"---",
		"",
		"Test worker.",
		"",
	}, "\n")
	if err := os.WriteFile(workerPath, []byte(worker), 0o600); err != nil {
		t.Fatalf("write external provider worker: %v", err)
	}
}

func externalProviderManifest(t *testing.T, identity, alias string) inference.Manifest {
	t.Helper()
	var catalog struct {
		Providers []inference.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode embedded provider catalog: %v", err)
	}
	manifest := catalog.Providers[0]
	manifest.ID = identity
	manifest.Aliases = []string{alias}
	manifest.ImplementationAvailability = inference.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = inference.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities = inference.ExecutionCapabilities{
		PromptSubmission: true,
	}
	manifest.MaximumResponseFidelityCapabilities = inference.ResponseFidelityCapabilities{}
	return manifest
}
