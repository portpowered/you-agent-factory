package script_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

const scriptNonZeroExitMessage = "script-non-zero-exit-output"

// TestScriptWorkerNonZeroExitMapsToFailedOutcome proves a root-built script
// worker whose command exits non-zero routes Work to the failed customer state
// and reports a customer-readable dispatch failure instead of success.
func TestScriptWorkerNonZeroExitMapsToFailedOutcome(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("failure-input-payload"))

	runner := nonZeroExitCommandRunner{stderr: scriptNonZeroExitMessage, exitCode: 1}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ScriptCommandRunner: runner},
		10*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work tokens = %d, want 1 non-zero-exit script dispatch", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work tokens = %d, want 0 after script failure", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("pending work tokens = %d, want 0 after script failure", got)
	}

	assertScriptNonZeroExitDispatchFailure(t, events, scriptNonZeroExitMessage)
}

type nonZeroExitCommandRunner struct {
	stderr   string
	exitCode int
}

func (r nonZeroExitCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{
		Stderr:   []byte(r.stderr),
		ExitCode: r.exitCode,
	}, nil
}

func assertScriptNonZeroExitDispatchFailure(t *testing.T, events []factoryapi.FactoryEvent, wantMessage string) {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) == 0 {
		t.Fatal("factory events missing dispatch observations")
	}
	response := dispatches[len(dispatches)-1].Response
	if response == nil {
		t.Fatal("dispatch response missing for failed script execution")
	}
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want FAILED", response.Outcome)
	}
	if response.Output != nil {
		t.Fatalf("dispatch output = %#v, want no primary result on script failure", response.Output)
	}
	assertDispatchErrorContains(t, events, wantMessage)
}
