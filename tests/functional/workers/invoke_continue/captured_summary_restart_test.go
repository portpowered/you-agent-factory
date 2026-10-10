package acceptance

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type summaryRestartSnapshot struct {
	observation api.WorkerSessionObservation
	transcript  api.WorkerSessionTranscriptResponse
	logs        api.WorkerSessionLogPage
}

// One production graph per joined profile epoch serves both attempts. The
// epochs must be sequential: reconstructing an idle host is the behavior.
// Independent profiles run in parallel; only the provider command is replaced.
func TestCapturedSummaryRestart(t *testing.T) {
	t.Parallel()
	root, dir := t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	failed := platformprocess.CommandResult{Stdout: []byte("{\"type\":\"thread.started\",\"thread_id\":\"failed-thread\"}\n{\"type\":\"turn.failed\",\"error\":{\"message\":\"controlled provider failure\"}}\n"), ExitCode: 1, Stderr: []byte("controlled provider failure")}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: directCodexSessionOutput("completed-thread", "completed answer")}, failed)
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
	first := startContinuationRestartHost(t, root, host, home, route)
	before := make(map[string]summaryRestartSnapshot)
	for _, id := range []string{"completed", "failed"} {
		path := filepath.Join(dir, id+".json")
		writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt", workingDirectory: dir, userMessage: id})
		request := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "invoke", "--execution", path})
		request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
		invokeErr := first.process.Execute(request.Input)
		if (invokeErr != nil) != (id == "failed") {
			t.Fatalf("%s invoke: %v %s %s", id, invokeErr, request.Stdout(), request.Stderr())
		}
		before[id] = readSummaryRestartSnapshot(t, first, home, dir, id)
		assertSummaryRestartCompletedFacts(t, id, before[id].observation)
	}
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	for id, want := range before {
		got := readSummaryRestartSnapshot(t, fresh, home, dir, id)
		// Live Runtime confirmation belongs to the canonical Factory ledger;
		// it is independent of durable Worker capture and is not a capture fact.
		want.observation.ConfirmationState = got.observation.ConfirmationState
		assertArchivedFailureUnknowns(t, got.observation)
		want.observation = recordedSummaryFailure(want.observation)
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("%s restart changed captured facts/content: before=%+v after=%+v", id, want, got)
		}
	}
	active := summaryRestartCLI(t, fresh, home, dir, "list", "--history", "active")
	var rows api.ListWorkerSessionsResponse
	decodeDirectWorkerSessionResult(t, active, &rows)
	if len(rows.Sessions) != 0 || runner.CallCount() != 2 {
		t.Fatalf("restart readmitted execution: %+v calls=%d", rows, runner.CallCount())
	}
}

func assertSummaryRestartCompletedFacts(t *testing.T, id string, row api.WorkerSessionObservation) {
	t.Helper()
	if !row.ProviderSessionAvailable || row.ProviderSession == nil || row.ProviderSession.Id != id+"-thread" || row.Transcript != "AVAILABLE" || row.EndedAt == nil || row.DurationMillis == nil {
		t.Fatalf("%s lost completed facts: %+v", id, row)
	}
	if id == "failed" && (row.State != "FAILED" || row.Failure == nil) {
		t.Fatalf("failure became completion: %+v", row)
	}
}

func summaryRestartCLI(t *testing.T, host invokeContinueStartedProcess, home, dir string, args ...string) string {
	t.Helper()
	request := support.FakeInputs(t.Context(), append([]string{"you", "--remote", "--server", host.baseURL, "--json", "worker-sessions"}, args...))
	request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := host.process.Execute(request.Input); err != nil {
		t.Fatalf("%v: %v %s %s", args, err, request.Stdout(), request.Stderr())
	}
	return request.Stdout()
}

// CLI/HTTP parity is explicit here: both must report the same captured facts.
func readSummaryRestartSnapshot(t *testing.T, host invokeContinueStartedProcess, home, dir, id string) summaryRestartSnapshot {
	t.Helper()
	var snapshot summaryRestartSnapshot
	decodeDirectWorkerSessionResult(t, summaryRestartCLI(t, host, home, dir, "show", "--worker-session-id", id), &snapshot.observation)
	http := support.GetJSON[api.WorkerSessionObservation](t, host.baseURL+"/worker-sessions/"+id)
	if !reflect.DeepEqual(snapshot.observation, http) {
		t.Fatalf("CLI/HTTP summary differs: %+v %+v", snapshot.observation, http)
	}
	for _, history := range []string{"archived", "all"} {
		var list api.ListWorkerSessionsResponse
		decodeDirectWorkerSessionResult(t, summaryRestartCLI(t, host, home, dir, "list", "--history", history), &list)
		found := false
		for _, row := range list.Sessions {
			if row.WorkerSessionId == id {
				found = true
				row.ConfirmationState = http.ConfirmationState
				if history == "archived" {
					assertArchivedFailureUnknowns(t, row)
				}
				if !reflect.DeepEqual(recordedSummaryFailure(row), recordedSummaryFailure(http)) {
					t.Fatalf("%s row differs: %+v %+v", history, row, http)
				}
			}
		}
		if !found {
			t.Fatalf("%s omitted %s", history, id)
		}
	}
	if err := json.Unmarshal([]byte(summaryRestartCLI(t, host, home, dir, "read", "--worker-session-id", id)), &snapshot.transcript); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(summaryRestartCLI(t, host, home, dir, "read", "--worker-session-id", id, "--view", "logs")), &snapshot.logs); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// The terminal schema records safe failure fields, not normalized provider
// classification. Compare durable facts while explicitly protecting unknowns.
func recordedSummaryFailure(row api.WorkerSessionObservation) api.WorkerSessionObservation {
	if row.Failure != nil {
		failure := *row.Failure
		failure.ProviderFailureKind = nil
		failure.ProviderContinuationFailureKind = nil
		failure.ProviderContinuationOutcome = nil
		row.Failure = &failure
	}
	return row
}

func assertArchivedFailureUnknowns(t *testing.T, row api.WorkerSessionObservation) {
	t.Helper()
	if row.Failure != nil && (row.Failure.ProviderFailureKind != nil || row.Failure.ProviderContinuationFailureKind != nil || row.Failure.ProviderContinuationOutcome != nil) {
		t.Fatalf("archive invented unrecorded provider classification: %+v", row.Failure)
	}
}
