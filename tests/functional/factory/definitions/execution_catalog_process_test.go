package definitions

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestExecutionCatalogResolvesThroughPublicProcessRun keeps the supported
// customer-path proof separate from the owner-local ResolveExecutionCatalog
// integration contract. The run enters through root.BuildProcess and reaches
// the provider command edge with the authored Factory definition.
func TestExecutionCatalogResolvesThroughPublicProcessRun(t *testing.T) {
	dir := support.ScaffoldFactory(t, validAPIValidationFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"),
	)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"prompt":"hello from process"}`))

	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Done. COMPLETE")},
	)
	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)
	if session.Runtime.Progress.Categories.Terminal != 1 ||
		session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want one terminal and zero failed", session.Runtime.Progress.Categories)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("task:complete count = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command count = %d, want 1", runner.CallCount())
	}
	if got := runner.LastRequest().Command; got != string(modelprovider.ProviderCodex) {
		t.Fatalf("provider command = %q, want %q", got, modelprovider.ProviderCodex)
	}
}
