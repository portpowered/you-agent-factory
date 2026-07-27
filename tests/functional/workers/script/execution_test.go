package script_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const scriptPrimaryResultOutput = "script-primary-result-output"

// TestScriptWorkerCompletesWithPublicPrimaryResult proves a root-built script
// worker that exits successfully completes Work on the customer-visible surface
// and exposes the script stdout as the public dispatch primary result.
func TestScriptWorkerCompletesWithPublicPrimaryResult(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("success-input-payload"))

	runner := support.NewRecordingCommandRunner(scriptPrimaryResultOutput)
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ScriptCommandRunner: runner},
		10*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1 successful script dispatch", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("pending work tokens = %d, want 0 after successful completion", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("script command calls = %d, want exactly one external command effect", runner.CallCount())
	}

	assertDispatchOutput(t, events, scriptPrimaryResultOutput)
}
