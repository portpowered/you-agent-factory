package acceptance

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
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

// Compatible command edges share one host in each joined profile epoch. The
// controls run concurrently against separate Worker IDs, directories and gates.
func TestCapturedControlledTerminalRestart(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	cells := make([]*controlledRestartCell, 0, 4)
	route := &invokeContinueStaticCommandRoute{}
	for _, action := range []string{"cancel", "terminate", "interrupt", "ownerlost"} {
		a, b := interruptRestartRepositories(t, filepath.Join(root, action))
		runner := newS8InterruptProviderRunner(directCodexSessionOutput("session_fixture_codex_success", "Codex fixture answer COMPLETE"), a, b)
		t.Cleanup(runner.releaseAll)
		id := action + "-source"
		if action == "ownerlost" {
			id = "terminal-owner-lost"
		}
		cells = append(cells, &controlledRestartCell{action: action, id: id, a: a, b: b, runner: runner, before: make(map[string]summaryRestartSnapshot)})
		route.routes = append(route.routes, invokeContinueStaticCommandRouteEntry{workingDirectory: a.path, runner: runner}, invokeContinueStaticCommandRouteEntry{workingDirectory: b.path, runner: runner})
	}
	scopeA, scopeB := interruptRestartRepositories(t, filepath.Join(root, "scope"))
	scopeRunner := newS8InterruptProviderRunner(directCodexSessionOutput("session_fixture_codex_success", "Codex fixture answer COMPLETE"), scopeA, scopeB)
	t.Cleanup(scopeRunner.releaseAll)
	route.routes = append(route.routes, invokeContinueStaticCommandRouteEntry{workingDirectory: scopeA.path, runner: scopeRunner})
	first := startContinuationRestartHost(t, root, host, home, route)
	t.Run("live", func(t *testing.T) {
		for _, c := range cells {
			if c.action == "ownerlost" {
				continue
			}
			t.Run(c.action, func(t *testing.T) {
				t.Parallel()
				runControlledRestartLive(t, first, home, c)
			})
		}
	})
	proveScopedCapturedTerminalAndNaturalWinner(t, first, home, scopeA.path, host, scopeRunner)
	// Losing all terminal writes makes the still-owned catalog unavailable.
	// Observe that isolated storage edge after the ordinary live-list witnesses.
	runControlledRestartLive(t, first, home, cells[len(cells)-1])
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	t.Run("archived", func(t *testing.T) {
		for _, c := range cells {
			t.Run(c.action, func(t *testing.T) {
				t.Parallel()
				runControlledRestartArchived(t, fresh, home, c)
			})
		}
	})
}

func invokeControlledRestartWorker(t *testing.T, host invokeContinueStartedProcess, home, dir, id string) {
	t.Helper()
	path := filepath.Join(dir, id+".json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt", workingDirectory: dir, userMessage: "initial input"})
	summaryRestartCLI(t, host, home, dir, "invoke", "--execution", path, "--async")
}

func assertControlledRestartFacts(t *testing.T, action string, row api.WorkerSessionObservation) {
	t.Helper()
	state, cause, terminal := "CANCELED", "OPERATOR_CANCELED", "OPERATOR_CANCEL"
	if action == "terminate" {
		state, cause, terminal = "TERMINATED", "OPERATOR_TERMINATED", "OPERATOR_TERMINATE"
	}
	if string(row.State) != state || row.Failure == nil || string(row.Failure.Kind) != cause || row.Failure.Detail == "" || row.StartedAt == nil || row.EndedAt == nil || row.DurationMillis == nil || row.TerminalCause == nil || string(*row.TerminalCause) != terminal {
		t.Fatalf("%s terminal facts: %+v", action, row)
	}
	if action == "interrupt" && (row.SuccessorWorkerSessionId == nil || *row.SuccessorWorkerSessionId != "recorded-successor") {
		t.Fatalf("lost lineage: %+v", row)
	}
}

func assertRestartOwnerLostPrefix(t *testing.T, host invokeContinueStartedProcess, id string) {
	t.Helper()
	row := support.GetJSON[api.WorkerSessionObservation](t, host.baseURL+"/worker-sessions/"+id)
	if row.State != "FAILED" || row.RecordingHealth == nil || *row.RecordingHealth != "INCOMPLETE" || row.TerminalCause == nil || *row.TerminalCause != "OWNER_LOST" || row.Failure == nil || row.Failure.Kind != "PROCESS_GONE" {
		t.Fatalf("owner loss invented complete facts: %+v", row)
	}
	assertRestartUnknownTerminalTiming(t, row)
	logs := support.GetJSON[api.WorkerSessionLogPage](t, host.baseURL+"/worker-sessions/"+id+"/logs")
	if logs.CommittedPosition != 1 || len(logs.Events) != 1 || logs.Health != "INCOMPLETE" || logs.Events[0].Event.Position != 1 {
		t.Fatalf("owner loss lost prefix: %+v", logs)
	}
}

type controlledRestartCell struct {
	action, id string
	a, b       s8Repository
	runner     *s8InterruptProviderRunner
	before     map[string]summaryRestartSnapshot
}

func runControlledRestartLive(t *testing.T, first invokeContinueStartedProcess, home string, c *controlledRestartCell) {
	t.Helper()
	invokeControlledRestartWorker(t, first, home, c.a.path, c.id)
	c.runner.waitStarted(t, c.a.path, s8InterruptCallAInitial)
	peerID := c.action + "-peer"
	invokeControlledRestartWorker(t, first, home, c.b.path, peerID)
	c.runner.waitStarted(t, c.b.path, s8InterruptCallBInitial)
	peer := support.GetJSON[api.WorkerSessionObservation](t, first.baseURL+"/worker-sessions/"+peerID)
	if c.action == "interrupt" {
		summaryRestartCLI(t, first, home, c.a.path, "interrupt", c.id, "--request-id", "recorded-interrupt", "--successor-worker-session-id", "recorded-successor", "--replacement-message", s8ReplacementMessage, "--resume-mode", "recorded", "--async")
		c.runner.waitStarted(t, c.a.path, s8InterruptCallASuccessor)
		c.runner.release(t, c.a.path, s8InterruptCallASuccessor)
		awaitContinuationRestartLogs(t, first, home, c.a.path, "recorded-successor", s8InterruptProviderSessionA)
		c.before["recorded-successor"] = readSummaryRestartSnapshot(t, first, home, c.a.path, "recorded-successor")
	} else if c.action == "ownerlost" {
		summaryRestartCLI(t, first, home, c.a.path, "cancel", c.id)
	} else {
		summaryRestartCLI(t, first, home, c.a.path, c.action, c.id)
	}
	c.runner.waitCanceled(t, c.a.path, s8InterruptCallAInitial)
	afterPeer := support.GetJSON[api.WorkerSessionObservation](t, first.baseURL+"/worker-sessions/"+peerID)
	if peer.State != "RUNNING" || afterPeer.State != "RUNNING" || peer.EndedAt != nil || afterPeer.EndedAt != nil {
		t.Fatalf("control ended peer: %+v %+v", peer, afterPeer)
	}
	// Active elapsed time advances while the target stops. Compare the
	// peer's identity, state and fixed facts, not that running clock.
	peer.DurationMillis, afterPeer.DurationMillis = nil, nil
	if !reflect.DeepEqual(peer, afterPeer) {
		t.Fatalf("control mutated peer: %+v %+v", peer, afterPeer)
	}
	if c.action != "ownerlost" {
		c.before[c.id] = readSummaryRestartSnapshot(t, first, home, c.a.path, c.id)
		assertControlledRestartFacts(t, c.action, c.before[c.id].observation)
	}
	c.runner.release(t, c.b.path, s8InterruptCallBInitial)
	awaitContinuationRestartLogs(t, first, home, c.b.path, peerID, s8InterruptProviderSessionB)
	if c.action != "ownerlost" {
		c.before[peerID] = readSummaryRestartSnapshot(t, first, home, c.b.path, peerID)
	}
}

func runControlledRestartArchived(t *testing.T, fresh invokeContinueStartedProcess, home string, c *controlledRestartCell) {
	t.Helper()
	for id, want := range c.before {
		got := readSummaryRestartSnapshot(t, fresh, home, c.a.path, id)
		want.observation.ConfirmationState = got.observation.ConfirmationState
		// Continuation capability depends on the current host, separately
		// from the immutable timing/failure/content tuple.
		want.observation.Revivable = got.observation.Revivable
		if !reflect.DeepEqual(want, got) {
			before, _ := json.Marshal(want.observation)
			after, _ := json.Marshal(got.observation)
			t.Fatalf("%s restart changed facts: before=%s after=%s transcriptEqual=%v logsEqual=%v", id, before, after, reflect.DeepEqual(want.transcript, got.transcript), reflect.DeepEqual(want.logs, got.logs))
		}
	}
	if c.action == "ownerlost" {
		assertRestartOwnerLostPrefix(t, fresh, c.id)
		peer := readSummaryRestartSnapshot(t, fresh, home, c.b.path, c.action+"-peer")
		if peer.observation.State != "COMPLETED" || peer.logs.Health != "COMPLETE" {
			t.Fatalf("owner loss hid completed sibling: %+v", peer.observation)
		}
	} else if c.action != "interrupt" {
		var repeat struct {
			Outcome string `json:"outcome"`
		}
		decodeDirectWorkerSessionResult(t, summaryRestartCLI(t, fresh, home, c.a.path, c.action, c.id), &repeat)
		if repeat.Outcome != "NOOP" {
			t.Fatalf("archive repeat=%+v", repeat)
		}
	}
	calls := 2
	if c.action == "interrupt" {
		calls++
	}
	if c.runner.CallCount() != calls {
		t.Fatalf("restart readmitted execution: %d", c.runner.CallCount())
	}
}

func proveScopedCapturedTerminalAndNaturalWinner(t *testing.T, host invokeContinueStartedProcess, home, dir, factoryDir string, runner *s8InterruptProviderRunner) {
	t.Helper()
	selected := support.OpenFactorySessionAt(t, host.baseURL, factoryDir).Session.Id
	foreign := support.OpenFactorySessionAt(t, host.baseURL, factoryDir).Session.Id
	defer support.CloseFactorySessionAt(t, host.baseURL, selected)
	defer support.CloseFactorySessionAt(t, host.baseURL, foreign)
	const id = "scoped-natural-winner"
	path := filepath.Join(dir, "scoped.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt", factorySessionID: selected, workingDirectory: dir, userMessage: "scoped completion"})
	summaryRestartCLI(t, host, home, dir, "invoke", "--execution", path, "--async")
	runner.waitStarted(t, dir, s8InterruptCallAInitial)
	runner.release(t, dir, s8InterruptCallAInitial)
	awaitContinuationRestartLogs(t, host, home, dir, id, s8InterruptProviderSessionA)
	endpoint := host.baseURL + "/factory-sessions/" + selected + "/worker-sessions/" + id
	before := support.GetJSON[api.WorkerSessionObservation](t, endpoint)
	if before.State != "COMPLETED" || before.Failure != nil || before.TerminalCause == nil || *before.TerminalCause != "COMPLETED" || before.EndedAt == nil {
		t.Fatalf("natural winner: %+v", before)
	}

	var repeat struct {
		Outcome string `json:"outcome"`
	}
	decodeDirectWorkerSessionResult(t, summaryRestartCLI(t, host, home, dir, "terminate", id), &repeat)
	after := support.GetJSON[api.WorkerSessionObservation](t, endpoint)
	if repeat.Outcome != "NOOP" || !reflect.DeepEqual(before, after) || runner.CallCount() != 1 {
		t.Fatalf("late stop replaced natural winner: %+v %+v repeat=%+v calls=%d", before, after, repeat, runner.CallCount())
	}
	assertScopedCapturedReads(t, host, selected, foreign, id, before)
}

func assertRestartUnknownTerminalTiming(t *testing.T, row api.WorkerSessionObservation) {
	t.Helper()
	if row.EndedAt != nil || row.DurationMillis != nil || row.Transcript != "UNAVAILABLE" || (row.Revivable != nil && *row.Revivable) {
		t.Fatalf("incomplete capture invented timing/authority: %+v", row)
	}
}

func assertScopedCapturedReads(t *testing.T, host invokeContinueStartedProcess, selected, foreign, id string, before api.WorkerSessionObservation) {
	t.Helper()
	status, body := t7HTTP(t, t.Context(), http.MethodGet, host.baseURL+"/factory-sessions/"+foreign+"/worker-sessions/"+id, nil)
	if status != http.StatusNotFound || strings.Contains(body, id+"-attempt") || strings.Contains(body, "startedAt") {
		t.Fatalf("foreign scope leaked facts: %d %s", status, body)
	}
	archived := support.GetJSON[api.ListWorkerSessionsResponse](t, host.baseURL+"/worker-sessions?history=archived")
	for _, row := range archived.Sessions {
		if row.WorkerSessionId != id {
			continue
		}
		if row.FactorySessionId == nil || *row.FactorySessionId != selected || row.Failure != nil || !reflect.DeepEqual(row.EndedAt, before.EndedAt) || !reflect.DeepEqual(row.DurationMillis, before.DurationMillis) {
			t.Fatalf("scoped archived facts: %+v", row)
		}
		return
	}
	t.Fatal("archive omitted scoped terminal")
}
