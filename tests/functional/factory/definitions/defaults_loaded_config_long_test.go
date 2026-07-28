//go:build functionallong

package definitions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestLoadedFactoryConfigDrivesProviderEdgePromptAndStopToken proves loaded
// on-disk worker and workstation definition content drives provider-edge prompt
// and stop-token behavior through the public you run process boundary with
// asserted ProviderCommandRunner captures.
func TestLoadedFactoryConfigDrivesProviderEdgePromptAndStopToken(t *testing.T) {
	support.SkipLongFunctional(t, "slow loaded-config provider-edge sweep")

	originalDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	writeLoadedConfigWorker(t, originalDir, "test-model", "COMPLETE")
	testutil.WriteSeedFile(t, originalDir, "task", []byte(`{"item":"original-config"}`))

	originalRunner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Stage 1 done. COMPLETE")},
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Stage 2 done. COMPLETE")},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		originalDir,
		serviceedges.Edges{ProviderCommandRunner: originalRunner},
		15*time.Second,
	)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1; listed=%#v", got, listed)
	}
	originalRequests := originalRunner.Requests()
	if len(originalRequests) != 2 {
		t.Fatalf("provider command count = %d, want 2 for original config", len(originalRequests))
	}
	if !strings.Contains(providerCommandPrompt(originalRequests[1]), "Step 2 workstation.") {
		t.Fatalf(
			"original step2 prompt = %q, want Step 2 workstation content",
			providerCommandPrompt(originalRequests[1]),
		)
	}

	updatedDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	writeLoadedConfigWorker(t, updatedDir, "test-model", "APPROVED")
	writeDefinitionsFixtureFile(
		t,
		filepath.Join(updatedDir, "workstations", "step2", "AGENTS.md"),
		`---
type: MODEL_WORKSTATION
---
Updated Step 2 workstation.
`,
	)
	testutil.WriteSeedFile(t, updatedDir, "task", []byte(`{"item":"updated-config"}`))

	updatedRunner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Stage 1 approved. APPROVED")},
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Stage 2 approved. APPROVED")},
	)
	_, updatedListed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		updatedDir,
		serviceedges.Edges{ProviderCommandRunner: updatedRunner},
		15*time.Second,
	)
	if got := support.CountWorkAtCustomerState(updatedListed, "task:done"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1; listed=%#v", got, updatedListed)
	}
	updatedRequests := updatedRunner.Requests()
	if len(updatedRequests) != 2 {
		t.Fatalf("provider command count = %d, want 2 for updated config", len(updatedRequests))
	}
	if !strings.Contains(providerCommandPrompt(updatedRequests[0]), "Step 1 workstation.") {
		t.Fatalf(
			"updated step1 prompt = %q, want Step 1 workstation content",
			providerCommandPrompt(updatedRequests[0]),
		)
	}
	if !strings.Contains(providerCommandPrompt(updatedRequests[1]), "Updated Step 2 workstation.") {
		t.Fatalf(
			"updated step2 prompt = %q, want updated Step 2 workstation content",
			providerCommandPrompt(updatedRequests[1]),
		)
	}
}

func writeLoadedConfigWorker(t *testing.T, dir, model, stopToken string) {
	t.Helper()

	writeDefinitionsFixtureFile(
		t,
		filepath.Join(dir, "workers", "agent", "AGENTS.md"),
		`---
type: MODEL_WORKER
model: `+model+`
modelProvider: `+string(modelprovider.ProviderCodex)+`
stopToken: `+stopToken+`
---
Process the work item.
`,
	)
}

func writeDefinitionsFixtureFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func providerCommandPrompt(request platformprocess.CommandRequest) string {
	if len(request.Stdin) > 0 {
		return string(request.Stdin)
	}
	return strings.Join(request.Args, " ")
}
