package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F7-C1/C2 cross the real joined stop boundary through one shared production
// process. Direct admissions carry scenario-owned Factory Session correlation;
// a Factory-origin execution would change continuation eligibility.
func TestCapturedNativeProviderStopThenContinue(t *testing.T) {
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			runCapturedNativeProviderStopThenContinue(t, fixture, provider)
		})
	}
}

// F7-C4: a running provider command can be controlled before it emits a native
// identity. Refused redirection must preserve that source and its live sibling.
func TestCapturedProviderMissingReferencePreservesControls(t *testing.T) {
	t.Parallel()
	scenario := ensureInvokeContinuePackageFixture(t).scenario(t, "native-no-reference")
	runner := scenario.providerRunner.(*nativeContinuationRunner)
	source, sibling, successor := scenarioScopedID(scenario, "source"), scenarioScopedID(scenario, "sibling"), scenarioScopedID(scenario, "successor")
	invokeNativeContinuationWorker(t, scenario, "codex", source, "native source", runner.sourceReady)
	t.Cleanup(func() { stopNativeContinuationWorker(t, scenario, source, true) })
	invokeNativeContinuationWorker(t, scenario, "codex", sibling, "native sibling", runner.siblingReady)
	t.Cleanup(func() { stopNativeContinuationWorker(t, scenario, sibling, true) })
	for _, remote := range []bool{false, true} {
		request := missingReferenceRequest(t, scenario, remote, []string{"worker-sessions", "interrupt", source,
			"--request-id", scenarioScopedID(scenario, "interrupt"), "--successor-worker-session-id", successor,
			"--replacement-message", "native follow-up", "--async"})
		assertDirectWorkerSessionCLIError(t, request, "WORKER_SESSION_INTERRUPT_CONFLICT")
		var failure struct {
			Phase string `json:"phase"`
		}
		decodeDirectWorkerSessionResult(t, request.Stderr(), &failure)
		if failure.Phase != "VALIDATION" {
			t.Fatalf("missing-reference interrupt phase = %q", failure.Phase)
		}
		assertMissingReferenceObservation(t, scenario, source, "RUNNING")
		assertMissingReferenceObservation(t, scenario, sibling, "RUNNING")
	}
	stopNativeContinuationWorker(t, scenario, source, true)
	for _, remote := range []bool{false, true} {
		request := missingReferenceRequest(t, scenario, remote, []string{"worker-sessions", "continue", source,
			"--request-id", scenarioScopedID(scenario, "continue"), "--successor-worker-session-id", successor,
			"--user-message", "native follow-up", "--async"})
		assertDirectWorkerSessionCLIError(t, request, "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID")
	}
	assertMissingReferenceObservation(t, scenario, source, "TERMINATED")
	assertMissingReferenceObservation(t, scenario, sibling, "RUNNING")
	missing := missingReferenceRequest(t, scenario, true, []string{"worker-sessions", "show", "--worker-session-id", successor})
	assertDirectWorkerSessionCLIError(t, missing, "WORKER_SESSION_NOT_FOUND")
	if runner.CallCount() != 2 {
		t.Fatalf("missing-reference controls admitted provider: calls=%d", runner.CallCount())
	}
}

func missingReferenceRequest(t *testing.T, scenario *invokeContinueScenario, remote bool, args []string) *support.CapturedInputs {
	t.Helper()
	flags := []string{"you", "--json"}
	if remote {
		flags = append(flags, "--remote", "--server", scenario.fixture.baseURL)
	}
	request := support.FakeInputs(t.Context(), append(flags, args...))
	request.Input.Env, request.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
	if err := scenario.fixture.process.Execute(request.Input); err == nil {
		t.Fatalf("missing-reference command succeeded: %v stdout=%s", args, request.Stdout())
	}
	return request
}

func assertMissingReferenceObservation(t *testing.T, scenario *invokeContinueScenario, id, state string) {
	t.Helper()
	request := executeNativeContinuationCLI(t, scenario, []string{"worker-sessions", "show", "--worker-session-id", id}, true)
	var observation factoryapi.WorkerSessionObservation
	decodeDirectWorkerSessionResult(t, request.Stdout(), &observation)
	if string(observation.State) != state || observation.ProviderSession != nil || observation.SuccessorWorkerSessionId != nil {
		t.Fatalf("missing-reference observation %s: %#v", id, observation)
	}
}

func runCapturedNativeProviderStopThenContinue(t *testing.T, fixture *invokeContinuePackageFixture, provider string) {
	scenario := fixture.scenario(t, "native-stop-continue-"+provider)
	runner := scenario.providerRunner.(*nativeContinuationRunner)
	source, sibling, successor := scenarioScopedID(scenario, "source"), scenarioScopedID(scenario, "sibling"), scenarioScopedID(scenario, "successor")
	invokeNativeContinuationWorker(t, scenario, provider, source, "native source", runner.sourceReady)
	t.Cleanup(func() { stopNativeContinuationWorker(t, scenario, source, true) })
	invokeNativeContinuationWorker(t, scenario, provider, sibling, "native sibling", runner.siblingReady)
	t.Cleanup(func() { stopNativeContinuationWorker(t, scenario, sibling, true) })
	stopNativeContinuationWorker(t, scenario, source, provider == "claude")
	assertNativeContinuationObservation(t, scenario, source, "TERMINATED", "opaque-native-source", "", "")
	assertNativeContinuationObservation(t, scenario, sibling, "RUNNING", "opaque-native-sibling", "", "")
	args := []string{"worker-sessions", "continue", source, "--request-id", scenarioScopedID(scenario, "continue"),
		"--successor-worker-session-id", successor, "--user-message", "native follow-up"}
	result := raceNativeContinuationCLI(t, scenario, args)
	var continued directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, result.Stdout(), &continued)
	if !continued.Accepted || continued.State != "COMPLETED" || continued.SuccessorWorkerSessionID != successor || !strings.Contains(continued.Output, "native continued COMPLETE") {
		t.Fatalf("native continuation result: %#v", continued)
	}
	// The exact tuple crosses the other placement boundary without readmission.
	_ = executeNativeContinuationCLI(t, scenario, args, provider != "claude")
	assertNativeContinuationObservation(t, scenario, source, "TERMINATED", "opaque-native-source", "", successor)
	assertNativeContinuationObservation(t, scenario, successor, "COMPLETED", "opaque-native-source", source, "")
	assertNativeContinuationObservation(t, scenario, sibling, "RUNNING", "opaque-native-sibling", "", "")
	assertNativeContinuationCommand(t, runner.Requests(), provider, scenario.workingDirectory)
}

func invokeNativeContinuationWorker(t *testing.T, scenario *invokeContinueScenario, provider, id, message string, ready <-chan struct{}) {
	t.Helper()
	path := filepath.Join(scenario.workingDirectory, id+".json")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt", factorySessionID: scenario.session.id,
		workingDirectory: scenario.workingDirectory, userMessage: message,
	})
	execution := document["execution"].(map[string]any)
	execution["runnerId"], execution["executorProvider"], execution["modelProvider"] = provider, provider, provider
	execution["reasoningEffort"] = "high"
	writeInvokeContinueJSON(t, path, document)
	_ = executeNativeContinuationCLI(t, scenario, []string{"worker-sessions", "invoke", "--execution", path, "--async"}, true)
	select {
	case <-ready:
	case <-t.Context().Done():
		t.Fatal("native source did not publish its provider identity")
	}
}

func executeNativeContinuationCLI(t *testing.T, scenario *invokeContinueScenario, args []string, remote bool) *support.CapturedInputs {
	t.Helper()
	flags := []string{"you", "--json"}
	if remote {
		flags = append(flags, "--remote", "--server", scenario.fixture.baseURL)
	}
	request := support.FakeInputs(t.Context(), append(flags, args...))
	request.Input.Env, request.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
	if err := scenario.fixture.process.Execute(request.Input); err != nil {
		t.Fatalf("native command %v: %v stdout=%s stderr=%s", args, err, request.Stdout(), request.Stderr())
	}
	return request
}

// F7-C8: local and HTTP-backed CLI callers overlap on one immutable tuple;
// both receive the same successor while the command edge admits it once.
func raceNativeContinuationCLI(t *testing.T, scenario *invokeContinueScenario, args []string) *support.CapturedInputs {
	t.Helper()
	start, done := make(chan struct{}), make(chan error, 2)
	requests := make([]*support.CapturedInputs, 0, 2)
	for _, flags := range [][]string{{"you", "--json"}, {"you", "--json", "--remote", "--server", scenario.fixture.baseURL}} {
		request := support.FakeInputs(t.Context(), append(flags, args...))
		request.Input.Env, request.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
		requests = append(requests, request)
		go func() { <-start; done <- scenario.fixture.process.Execute(request.Input) }()
	}
	close(start)
	for range requests {
		if err := <-done; err != nil {
			t.Fatalf("concurrent native continuation: %v", err)
		}
	}
	var local, remote directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, requests[0].Stdout(), &local)
	decodeDirectWorkerSessionResult(t, requests[1].Stdout(), &remote)
	if local != remote {
		t.Fatalf("concurrent placements disagreed: local=%#v remote=%#v", local, remote)
	}
	return requests[0]
}

func stopNativeContinuationWorker(t *testing.T, scenario *invokeContinueScenario, id string, remote bool) {
	t.Helper()
	// Go cancels t.Context before cleanup. The scenario still owns both Workers
	// until these joined stops complete, including on an earlier assertion failure.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	flags := []string{"you", "--json"}
	if remote {
		flags = append(flags, "--remote", "--server", scenario.fixture.baseURL)
	}
	result := support.FakeInputs(ctx, append(flags, "worker-sessions", "terminate", id))
	result.Input.Env, result.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
	if err := scenario.fixture.process.Execute(result.Input); err != nil {
		t.Fatalf("native stop: %v stdout=%s stderr=%s", err, result.Stdout(), result.Stderr())
	}
	var stopped struct {
		State   string `json:"state"`
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal([]byte(result.Stdout()), &stopped); err != nil || stopped.State != "TERMINATED" {
		t.Fatalf("native stop did not join: %v result=%#v stdout=%s", err, stopped, result.Stdout())
	}
}

func assertNativeContinuationObservation(t *testing.T, scenario *invokeContinueScenario, id, state, nativeID, predecessor, successor string) {
	t.Helper()
	result := executeNativeContinuationCLI(t, scenario, []string{"worker-sessions", "show", "--worker-session-id", id}, true)
	var observation factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(result.Stdout()), &observation); err != nil {
		t.Fatal(err)
	}
	if string(observation.State) != state || observation.ProviderSession == nil || observation.ProviderSession.Id != nativeID {
		t.Fatalf("native observation %s: %#v", id, observation)
	}
	if observation.FactorySessionId == nil || *observation.FactorySessionId != scenario.session.id {
		t.Fatalf("native continuation changed Factory Session scope: %#v", observation)
	}
	if predecessor != "" && (observation.PredecessorWorkerSessionId == nil || *observation.PredecessorWorkerSessionId != predecessor) {
		t.Fatalf("successor lost predecessor: %#v", observation)
	}
	if successor != "" && (observation.SuccessorWorkerSessionId == nil || *observation.SuccessorWorkerSessionId != successor) {
		t.Fatalf("source lost successor: %#v", observation)
	}
}

func assertNativeContinuationCommand(t *testing.T, requests []platformprocess.CommandRequest, provider, dir string) {
	t.Helper()
	if len(requests) != 3 {
		t.Fatalf("native continuation admitted duplicate: requests=%#v", requests)
	}
	request := requests[2]
	resume := "resume"
	if provider == "claude" {
		resume = "--resume"
	}
	args := strings.Join(request.Args, " ")
	if request.Command != provider || request.WorkDir != dir || !strings.Contains(args, resume+" opaque-native-source") ||
		!strings.Contains(args, "functional-model") || !strings.Contains(args, "high") ||
		!strings.Contains(args+string(request.Stdin), "native follow-up") {
		t.Fatalf("native continuation lost captured identity/settings/input: %#v", request)
	}
}

// F7-C3 uses sequential production root processes over one scenario-owned
// store. No native provider files exist; only the command edge is substituted.
func TestCapturedProviderContinueAfterHostRestart(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"completed", "failed", "lost-input-ack", "uncertain-opening", "unadmitted-recipe", "missing-recipe", "stale-recipe", "incomplete-capture"} {
		t.Run(name, func(t *testing.T) { t.Parallel(); runCapturedProviderContinueAfterHostRestart(t, name) })
	}
}

// Restart requires sequential processes over the same isolated profile. The
// runner is controlled at the public command edge; no executable is built.
func TestDurableRevivalFactorySourceAfterHostRestart(t *testing.T) {
	t.Parallel()
	for _, restart := range []bool{false, true} {
		name := "live-host"
		if restart {
			name = "rebuilt-host"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runDurableRevivalFactorySource(t, restart)
		})
	}
}

func runDurableRevivalFactorySource(t *testing.T, restart bool) {
	t.Helper()
	root, dir := t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("opaque-restart-thread", "initial COMPLETE")},
		continuationRestartCommandResult(false),
	)
	peerRunner := &t7GatedProviderRunner{}
	peerRunner.reset()
	commandRunner := &durableRevivalCommandRunner{source: runner, peer: peerRunner}
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: commandRunner}}}
	first := startContinuationRestartHost(t, root, host, home, route)
	t7WriteFactorySibling(t, dir)
	writeDurableRevivalCapacity(t, dir)
	opened := support.OpenFactorySessionAt(t, first.baseURL, dir)
	if !restart {
		defer support.CloseFactorySessionAt(t, first.baseURL, opened.Session.Id)
	}
	work := support.SubmitSessionWorkAt(t, first.baseURL, opened.Session.Id, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task", Payload: "durable Factory source input",
	})
	// Runtime result processing follows provider completion. Its public terminal
	// status is the only customer barrier covering both Work and Worker capture.
	support.WaitForSessionTerminalStatus(t, first.baseURL, opened.Session.Id, 30*time.Second)
	rows := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t,
		first.baseURL+"/factory-sessions/"+opened.Session.Id+"/worker-sessions?workId="+*work.WorkId)
	if len(rows.Sessions) != 1 || rows.Sessions[0].State != "COMPLETED" || rows.Sessions[0].Direct {
		t.Fatalf("Factory source: %#v", rows)
	}
	sourceID := rows.Sessions[0].WorkerSessionId
	awaitContinuationRestartLogs(t, first, home, dir, sourceID)
	fresh := first
	if restart {
		support.CloseFactorySessionAt(t, first.baseURL, opened.Session.Id)
		if err := first.command.stop(); err != nil {
			t.Fatal(err)
		}
		if err := first.process.Close(t.Context()); err != nil {
			t.Fatal(err)
		}
		fresh = startContinuationRestartHost(t, root, host, home, route)
	}
	peerSession := opened.Session.Id
	if restart {
		peerSession = support.OpenFactorySessionAt(t, fresh.baseURL, dir).Session.Id
		defer support.CloseFactorySessionAt(t, fresh.baseURL, peerSession)
	}
	defer t7ReleaseAndJoin(t, t.Context(), peerRunner)()
	peer := startDurableRevivalPeer(t, fresh, peerSession, peerRunner)
	peerEvents := support.GetFactoryEventsForSessionAt(t, fresh.baseURL, peerSession)
	peerWork := support.GetJSON[factoryapi.ListWorkResponse](t, fresh.baseURL+"/factory-sessions/"+peerSession+"/work")
	var factoryEvents []factoryapi.FactoryEvent
	var factoryWork factoryapi.ListWorkResponse
	if !restart {
		factoryEvents = support.GetFactoryEventsForSessionAt(t, first.baseURL, opened.Session.Id)
		factoryWork = support.GetJSON[factoryapi.ListWorkResponse](t, first.baseURL+"/factory-sessions/"+opened.Session.Id+"/work")
	}
	args := []string{"worker-sessions", "continue", sourceID,
		"--session", opened.Session.Id, "--request-id", "factory-revival-request",
		"--successor-worker-session-id", "factory-revival-successor", "--user-message", "fresh host follow-up"}
	requests := raceFactoryRevivalCLI(t, fresh, home, dir, args)
	for _, request := range requests {
		assertFactoryRevivalResult(t, fresh, runner, request.Stdout(), dir, sourceID)
	}
	replay := executeHeadRestartCLI(t, fresh, home, dir, args...)
	if replay.SuccessorWorkerSessionID != "factory-revival-successor" || runner.CallCount() != 2 {
		t.Fatalf("Factory revival retry launched another attempt: %+v calls=%d", replay, runner.CallCount())
	}
	if !restart {
		assertFactoryRevivalIndependence(t, first, opened.Session.Id, *work.WorkId, rows.Sessions[0], factoryEvents, factoryWork)
	}
	assertFactoryRevivalIndependence(t, fresh, peerSession, *peer.WorkId, peer.Observation, peerEvents, peerWork)
	assertDurableRevivalPeerCompletion(t, fresh, peerSession, peer.Observation.WorkerSessionId, peerRunner)
}

type durableRevivalPeer struct {
	WorkId      *string
	Observation factoryapi.WorkerSessionObservation
}

func startDurableRevivalPeer(t *testing.T, host invokeContinueStartedProcess, session string, runner *t7GatedProviderRunner) durableRevivalPeer {
	t.Helper()
	work := support.SubmitSessionWorkAt(t, host.baseURL, session, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task", Payload: "durable occupied peer input",
	})
	t19AwaitSignal(t, t.Context(), runner.started, "Factory peer occupies executor slot")
	rows := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, host.baseURL+"/factory-sessions/"+session+"/worker-sessions?workId="+*work.WorkId)
	if len(rows.Sessions) != 1 || rows.Sessions[0].State != "RUNNING" || rows.Sessions[0].Direct {
		t.Fatalf("occupied Factory peer: %+v", rows)
	}
	return durableRevivalPeer{WorkId: work.WorkId, Observation: rows.Sessions[0]}
}

func assertFactoryRevivalIndependence(t *testing.T, host invokeContinueStartedProcess, sessionID, workID string, source factoryapi.WorkerSessionObservation, events []factoryapi.FactoryEvent, work factoryapi.ListWorkResponse) {
	t.Helper()
	if after := support.GetFactoryEventsForSessionAt(t, host.baseURL, sessionID); !reflect.DeepEqual(events, after) {
		t.Fatal("direct Factory revival changed canonical Factory history")
	}
	if after := support.GetJSON[factoryapi.ListWorkResponse](t, host.baseURL+"/factory-sessions/"+sessionID+"/work"); !reflect.DeepEqual(work, after) {
		t.Fatal("direct Factory revival changed source Work")
	}
	rows := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, host.baseURL+"/factory-sessions/"+sessionID+"/worker-sessions?workId="+workID)
	// Correlation is retained by the independent direct successor. It does not
	// replace the source attempt or imply a new Runtime-owned Work attempt.
	for _, row := range rows.Sessions {
		if row.WorkerSessionId == source.WorkerSessionId {
			if row.AttemptId != source.AttemptId || row.State != source.State || row.Direct != source.Direct {
				t.Fatalf("direct revival changed the Factory source attempt: %+v", row)
			}
			return
		}
	}
	t.Fatal("Factory source attempt disappeared after direct revival")
}

// Local and remote callers race one exact Factory-origin address and immutable
// request tuple. The provider boundary must see only one direct revival.
func raceFactoryRevivalCLI(t *testing.T, host invokeContinueStartedProcess, home, dir string, args []string) []*support.CapturedInputs {
	t.Helper()
	start, done := make(chan struct{}), make(chan error, 2)
	requests := make([]*support.CapturedInputs, 0, 2)
	for _, flags := range [][]string{{"you", "--json"}, {"you", "--json", "--remote", "--server", host.baseURL}} {
		request := support.FakeInputs(t.Context(), append(flags, args...))
		request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
		requests = append(requests, request)
		go func() { <-start; done <- host.process.Execute(request.Input) }()
	}
	close(start)
	var failure error
	for range requests {
		if err := <-done; err != nil {
			failure = err
		}
	}
	if failure != nil {
		t.Fatalf("concurrent Factory revival: %v local=%s remote=%s", failure, requests[0].Stdout()+requests[0].Stderr(), requests[1].Stdout()+requests[1].Stderr())
	}
	return requests
}

func assertFactoryRevivalResult(t *testing.T, host invokeContinueStartedProcess, runner *testutil.ProviderCommandRunner, stdout, dir, sourceID string) {
	t.Helper()
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, stdout, &result)
	requests := runner.Requests()
	if !result.Accepted || result.State != "COMPLETED" || result.SuccessorWorkerSessionID != "factory-revival-successor" || len(requests) != 2 {
		t.Fatalf("Factory revival result=%#v calls=%d", result, len(requests))
	}
	command := requests[1]
	if command.WorkDir != dir || !strings.Contains(strings.Join(command.Args, " "), "resume opaque-restart-thread") ||
		!strings.Contains(strings.Join(command.Args, " "), "opaque-factory-model") ||
		!strings.Contains(strings.Join(command.Args, " ")+string(command.Stdin), "fresh host follow-up") {
		t.Fatalf("Factory revival lost exact execution: %#v", command)
	}
	observation := support.GetJSON[factoryapi.WorkerSessionObservation](t, host.baseURL+"/worker-sessions/factory-revival-successor")
	if !observation.Direct || observation.PredecessorWorkerSessionId == nil || *observation.PredecessorWorkerSessionId != sourceID {
		t.Fatalf("Factory revival did not create direct lineage: %#v", observation)
	}
}

func TestContinuationHeadSurvivesRepeatedContinuationAndHostRestart(t *testing.T) {
	t.Parallel()
	dir, root := t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("opaque-restart-thread", "initial COMPLETE")},
		continuationRestartCommandResult(false), continuationRestartCommandResult(false), continuationRestartCommandResult(false),
	)
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
	first := startContinuationRestartHost(t, root, host, home, route)
	path := filepath.Join(dir, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{requestID: "head-source-request", workerSessionID: "head-source", dispatchID: "head-source-attempt", workingDirectory: dir, userMessage: "initial input"})
	executeHeadRestartCLI(t, first, home, dir, "worker-sessions", "invoke", "--execution", path)
	args := []string{"worker-sessions", "continue", "head-source", "--request-id", "head-first", "--successor-worker-session-id", "head-first-successor", "--user-message", "first follow-up"}
	executeHeadRestartCLI(t, first, home, dir, args...)
	denied := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "continue", "head-source", "--request-id", "head-default-denied", "--successor-worker-session-id", "head-denied-successor", "--user-message", "default must conflict"})
	denied.Input.Env, denied.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := first.process.Execute(denied.Input); err == nil || runner.CallCount() != 2 {
		t.Fatalf("default continuation advanced the chain: %v calls=%d", err, runner.CallCount())
	}
	assertDirectWorkerSessionCLIError(t, denied, "WORKER_SESSION_CONTINUATION_CONFLICT")
	headArgs := []string{"worker-sessions", "continue", "head-source", "--head", "--request-id", "head-second", "--successor-worker-session-id", "head-second-successor", "--user-message", "second follow-up"}
	second := executeHeadRestartCLI(t, first, home, dir, append(headArgs, "--remote", "--server", first.baseURL)...)
	if second.SourceWorkerSessionID != "head-first-successor" || second.PredecessorWorkerSessionID != "head-first-successor" {
		t.Fatalf("head response lost resolved lineage: %+v", second)
	}
	awaitContinuationRestartLogs(t, first, home, dir, "head-second-successor")
	assertContinuationHeadCapability(t, first, home, dir, "head-second-successor")
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	assertContinuationHeadCapability(t, fresh, home, dir, "head-second-successor")
	replay := executeHeadRestartCLI(t, fresh, home, dir, headArgs...)
	if replay.SourceWorkerSessionID != second.SourceWorkerSessionID || runner.CallCount() != 3 {
		t.Fatalf("durable retry advanced head or repeated launch: %+v calls=%d", replay, runner.CallCount())
	}
	third := executeHeadRestartCLI(t, fresh, home, dir, "worker-sessions", "continue", "head-source", "--head", "--request-id", "head-third", "--successor-worker-session-id", "head-third-successor", "--user-message", "third follow-up")
	if third.SourceWorkerSessionID != "head-second-successor" || third.PredecessorWorkerSessionID != "head-second-successor" || runner.CallCount() != 4 {
		t.Fatalf("restart did not resolve newest head: %+v calls=%d", third, runner.CallCount())
	}
	awaitContinuationRestartLogs(t, fresh, home, dir, "head-third-successor")
	assertContinuationHeadCapability(t, fresh, home, dir, "head-third-successor")
	assertOlderHeadRequestReplay(t, fresh, runner, home, dir, headArgs, second)
	assertChangedHeadRequestRefusal(t, fresh, runner)
	assertHeadContinuationExecution(t, runner, dir)
}

func assertChangedHeadRequestRefusal(t *testing.T, host invokeContinueStartedProcess, runner *testutil.ProviderCommandRunner) {
	t.Helper()
	for _, cell := range []struct {
		source, successor, input string
		head                     bool
	}{
		{"head-source", "head-second-successor", "changed follow-up", true},
		{"head-source", "changed-successor", "second follow-up", true},
		{"head-first-successor", "head-second-successor", "second follow-up", true},
		{"head-source", "head-second-successor", "second follow-up", false},
	} {
		status, body := t7HTTP(t, t.Context(), http.MethodPost, host.baseURL+"/worker-sessions/"+cell.source+"/continue", map[string]any{
			"resolveHead": cell.head, "requestId": "head-second", "successorWorkerSessionId": cell.successor, "followUpInput": cell.input,
		})
		if status != http.StatusConflict || !strings.Contains(body, "WORKER_SESSION_CONTINUATION_REQUEST_ID_CONFLICT") || runner.CallCount() != 4 {
			t.Fatalf("changed head tuple admitted/rebound request: %d %s calls=%d", status, body, runner.CallCount())
		}
	}
}

func assertHeadContinuationExecution(t *testing.T, runner *testutil.ProviderCommandRunner, dir string) {
	t.Helper()
	for index, request := range runner.Requests()[1:] {
		command := strings.Join(request.Args, " ")
		input := []string{"first follow-up", "second follow-up", "third follow-up"}[index]
		if request.WorkDir != dir || !strings.Contains(command, "resume opaque-restart-thread") || !strings.Contains(command, "functional-model") || !strings.Contains(command+string(request.Stdin), input) {
			t.Fatalf("head continuation replaced exact execution: %+v", request)
		}
	}
}

func assertOlderHeadRequestReplay(t *testing.T, fresh invokeContinueStartedProcess, runner *testutil.ProviderCommandRunner, home, dir string, headArgs []string, second directWorkerSessionCLIResult) {
	t.Helper()
	for _, flags := range [][]string{nil, {"--remote", "--server", fresh.baseURL}} {
		replayed := executeHeadRestartCLI(t, fresh, home, dir, append(append([]string(nil), headArgs...), flags...)...)
		if replayed.SourceWorkerSessionID != second.SourceWorkerSessionID || replayed.SuccessorWorkerSessionID != second.SuccessorWorkerSessionID || runner.CallCount() != 4 {
			t.Fatalf("older request retry followed the advanced head: %+v calls=%d", replayed, runner.CallCount())
		}
	}
}

func executeHeadRestartCLI(t *testing.T, host invokeContinueStartedProcess, home, dir string, args ...string) directWorkerSessionCLIResult {
	t.Helper()
	request := support.FakeInputs(t.Context(), append([]string{"you", "--json"}, args...))
	request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := host.process.Execute(request.Input); err != nil {
		t.Fatalf("head command %v: %v stdout=%s stderr=%s", args, err, request.Stdout(), request.Stderr())
	}
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, request.Stdout(), &result)
	return result
}

func runCapturedProviderContinueAfterHostRestart(t *testing.T, name string) {
	failed := name == "failed"
	requestID := continuationRestartRequestID(name)
	sourceID := "restart-source"
	if name == "missing-recipe" || name == "stale-recipe" || name == "incomplete-capture" {
		sourceID = "revival-" + name
	}
	dir, root := t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("opaque-restart-thread", "initial COMPLETE")},
		continuationRestartCommandResult(failed),
	)
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
	first := startContinuationRestartHost(t, root, host, home, route)
	path := filepath.Join(dir, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{
		requestID: "restart-source-request", workerSessionID: sourceID, dispatchID: "restart-source-attempt",
		workingDirectory: dir, userMessage: "initial input",
	})
	invoke := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "invoke", "--execution", path})
	invoke.Input.Env, invoke.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := first.process.Execute(invoke.Input); err != nil {
		t.Fatalf("source invoke: %v stdout=%s stderr=%s", err, invoke.Stdout(), invoke.Stderr())
	}
	var source directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, invoke.Stdout(), &source)
	if !source.Accepted || source.State != "COMPLETED" || runner.CallCount() != 1 {
		t.Fatalf("source was not joined: %#v calls=%d", source, runner.CallCount())
	}
	if name != "incomplete-capture" {
		awaitContinuationRestartLogs(t, first, home, dir, sourceID)
	}
	var transcript *factoryapi.WorkerSessionTranscriptResponse
	if name == "completed" {
		captured := readCapturedRestartTranscript(t, first, home, dir)
		transcript = &captured
	}

	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	if transcript != nil {
		captured := readCapturedRestartTranscript(t, fresh, home, dir)
		if !reflect.DeepEqual(*transcript, captured) {
			t.Fatalf("restart changed committed transcript: %+v / %+v", *transcript, captured)
		}
		assertCapturedRestartScopeDenial(t, fresh)
	}

	if sourceID != "restart-source" {
		if name == "incomplete-capture" {
			assertDurableRevivalRecipeRefusal(t, fresh, home, dir, sourceID, runner, "DEGRADED")
		} else {
			assertDurableRevivalRecipeRefusal(t, fresh, home, dir, sourceID, runner)
		}
		return
	}
	if successor := continuationRestartUnadmittedSuccessor(name); successor != "" {
		assertUncertainContinuationAfterRestart(t, fresh, root, host, home, dir, route, runner, successor)
		return
	}
	continued := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "continue", sourceID,
		"--request-id", requestID, "--successor-worker-session-id", "restart-successor", "--user-message", "fresh host follow-up"})
	continued.Input.Env, continued.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := fresh.process.Execute(continued.Input); !failed && err != nil {
		t.Fatalf("fresh-host continuation: %v stdout=%s stderr=%s", err, continued.Stdout(), continued.Stderr())
	}
	if failed {
		assertDirectWorkerSessionCLIError(t, continued, "WORKER_SESSION_FAILED")
	} else {
		assertContinuationRestartResult(t, continued.Stdout(), runner.Requests(), dir)
	}
	logs := awaitContinuationRestartLogs(t, fresh, home, dir, "restart-successor")
	if !strings.Contains(logs, sourceID) {
		t.Fatalf("successor logs omitted predecessor: %s", logs)
	}
	show := support.FakeInputs(t.Context(), []string{"you", "--server", fresh.baseURL, "--json", "worker-sessions", "show", "--worker-session-id", sourceID})
	show.Input.Env, show.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := fresh.process.Execute(show.Input); err != nil || !strings.Contains(show.Stdout(), "restart-successor") {
		t.Fatalf("archived source lost public successor link: %v stdout=%s stderr=%s", err, show.Stdout(), show.Stderr())
	}
	assertCompletedContinuationReplayAfterRestart(t, fresh, root, host, home, dir, route, runner, failed, requestID)
}

func continuationRestartCommandResult(failed bool) platformprocess.CommandResult {
	if failed {
		return platformprocess.CommandResult{Stderr: []byte("Error: thread/resume failed: no rollout found for thread id opaque-restart-thread"), ExitCode: 1}
	}
	return platformprocess.CommandResult{Stdout: directCodexSessionOutput("opaque-restart-thread", "continued COMPLETE")}
}

func continuationRestartUnadmittedSuccessor(name string) string {
	switch name {
	case "uncertain-opening":
		return "continuation-opening-ack-lost-successor"
	case "unadmitted-recipe":
		return "continuation-unadmitted-recipe-successor"
	default:
		return ""
	}
}

// F7-C9/C10: a synced input and persisted opening with a lost acknowledgement
// do not prove Workers admission. Sequential hosts share the actual store;
// neither the exact retry nor changed tuples may execute the provider again.
func assertUncertainContinuationAfterRestart(t *testing.T, previous invokeContinueStartedProcess, root, host, home, dir string, route *invokeContinueStaticCommandRoute, runner *testutil.ProviderCommandRunner, successor string) {
	t.Helper()
	assertUncertainContinuationRequests(t, previous, home, dir, runner, successor)
	if err := previous.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := previous.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	assertUncertainContinuationRequests(t, fresh, home, dir, runner, successor)
	show := support.FakeInputs(t.Context(), []string{"you", "--server", fresh.baseURL, "--json", "worker-sessions", "show", "--worker-session-id", "restart-source"})
	show.Input.Env, show.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := fresh.process.Execute(show.Input); err != nil {
		t.Fatalf("uncertain continuation lost source: %v %s %s", err, show.Stdout(), show.Stderr())
	}
	var observation factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(show.Stdout()), &observation); err != nil {
		t.Fatal(err)
	}
	// The captured read model exposes the reserved identity from the committed
	// successor opening. That historical link does not prove external admission.
	if observation.State != "COMPLETED" || observation.SuccessorWorkerSessionId == nil ||
		*observation.SuccessorWorkerSessionId != successor {
		t.Fatalf("uncertain opening lost terminal source or reserved identity: %#v", observation)
	}
}

func assertUncertainContinuationRequests(t *testing.T, host invokeContinueStartedProcess, home, dir string, runner *testutil.ProviderCommandRunner, successor string) {
	t.Helper()
	for _, cell := range []struct {
		message, successor, code string
		remote                   bool
	}{
		{"fresh host follow-up", successor, "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED", false},
		{"fresh host follow-up", successor, "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED", true},
		{"changed follow-up", successor, "WORKER_SESSION_CONTINUATION_REQUEST_ID_CONFLICT", false},
		{"fresh host follow-up", "changed-successor", "WORKER_SESSION_CONTINUATION_REQUEST_ID_CONFLICT", true},
	} {
		flags := []string{"you", "--json"}
		if cell.remote {
			flags = append(flags, "--remote", "--server", host.baseURL)
		}
		request := support.FakeInputs(t.Context(), append(flags, "worker-sessions", "continue", "restart-source",
			"--request-id", "uncertain-opening-request", "--successor-worker-session-id", cell.successor, "--user-message", cell.message, "--async"))
		request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
		if err := host.process.Execute(request.Input); err == nil {
			t.Fatal("uncertain opening admitted continuation")
		}
		assertDirectWorkerSessionCLIError(t, request, cell.code)
		if strings.Contains(request.Stdout()+request.Stderr(), "private-continuation") || runner.CallCount() != 1 {
			t.Fatalf("uncertain retry leaked diagnostic or admitted provider: calls=%d %s %s", runner.CallCount(), request.Stdout(), request.Stderr())
		}
	}
}

func continuationRestartRequestID(name string) string {
	if name == "lost-input-ack" {
		return "continuation-input-ack-lost-restart"
	}
	return "restart-continue-request"
}

func assertCompletedContinuationReplayAfterRestart(t *testing.T, previous invokeContinueStartedProcess, root, host, home, dir string, route *invokeContinueStaticCommandRoute, runner *testutil.ProviderCommandRunner, failed bool, requestID string) {
	t.Helper()
	if err := previous.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := previous.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	for _, cell := range []struct {
		message, successor string
		flags              []string
	}{
		{"fresh host follow-up", "restart-successor", []string{"--async"}},
		{"fresh host follow-up", "restart-successor", nil},
		{"fresh host follow-up", "restart-successor", []string{"--remote", "--server", fresh.baseURL}},
		{"changed follow-up", "restart-successor", []string{"--async"}},
		{"fresh host follow-up", "changed-successor", []string{"--async"}},
	} {
		args := []string{"you", "--json", "worker-sessions", "continue", "restart-source",
			"--request-id", requestID, "--successor-worker-session-id", cell.successor, "--user-message", cell.message}
		request := support.FakeInputs(t.Context(), append(args, cell.flags...))
		request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
		err := fresh.process.Execute(request.Input)
		if cell.message == "fresh host follow-up" && cell.successor == "restart-successor" {
			if failed {
				assertFailedContinuationReplayResult(t, err, request.Stdout(), request.Stderr(), cell.flags)
			} else {
				assertCompletedContinuationReplayResult(t, err, request.Stdout(), request.Stderr(), cell.flags)
			}
		} else if err == nil || !strings.Contains(request.Stdout()+request.Stderr(), "CONFLICT") {
			t.Fatalf("changed replay was not refused: %v stdout=%s stderr=%s", err, request.Stdout(), request.Stderr())
		}
		if runner.CallCount() != 2 {
			t.Fatalf("replay repeated provider admission: calls=%d", runner.CallCount())
		}
	}
}

func assertCompletedContinuationReplayResult(t *testing.T, err error, stdout, stderr string, flags []string) {
	t.Helper()
	if err != nil {
		t.Fatalf("completed replay flags=%v: %v stdout=%s stderr=%s", flags, err, stdout, stderr)
	}
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, stdout, &result)
	if !result.Accepted || result.State != "COMPLETED" || result.SuccessorWorkerSessionID != "restart-successor" {
		t.Fatalf("completed replay: result=%#v stderr=%s", result, stderr)
	}
	if (len(flags) == 0 || flags[0] == "--remote") && !strings.Contains(result.Output, "continued COMPLETE") {
		t.Fatalf("synchronous replay lost captured output: %#v", result)
	}
}

func assertContinuationRestartResult(t *testing.T, stdout string, requests []platformprocess.CommandRequest, dir string) {
	t.Helper()
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, stdout, &result)
	if !result.Accepted || result.State != "COMPLETED" || result.SuccessorWorkerSessionID != "restart-successor" || !strings.Contains(result.Output, "continued COMPLETE") {
		t.Fatalf("fresh-host result = %#v", result)
	}
	if len(requests) != 2 || requests[1].WorkDir != dir || !strings.Contains(strings.Join(requests[1].Args, " "), "resume opaque-restart-thread") ||
		!strings.Contains(strings.Join(requests[1].Args, " ")+string(requests[1].Stdin), "fresh host follow-up") ||
		!strings.Contains(strings.Join(requests[1].Args, " "), "functional-model") {
		t.Fatalf("fresh-host native continuation = %#v", requests)
	}
}

func awaitContinuationRestartLogs(t *testing.T, host invokeContinueStartedProcess, home, dir, id string, nativeIDs ...string) string {
	t.Helper()
	// Following the capture joins its durable terminal before host shutdown,
	// rather than relying on a delay after the live session terminal result.
	logs := support.FakeInputs(t.Context(), []string{"you", "--server", host.baseURL, "--json", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--follow"})
	logs.Input.Env, logs.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := host.process.Execute(logs.Input); err != nil {
		t.Fatalf("joined captured logs: %v %s", err, logs.Stderr())
	}
	nativeID := "opaque-restart-thread"
	if len(nativeIDs) != 0 {
		nativeID = nativeIDs[0]
	}
	if !strings.Contains(logs.Stdout(), nativeID) {
		t.Fatalf("captured logs omitted native identity: %s", logs.Stdout())
	}
	return logs.Stdout()
}

func startContinuationRestartHost(t *testing.T, root, host, home string, route *invokeContinueStaticCommandRoute) invokeContinueStartedProcess {
	t.Helper()
	started, err := startInvokeContinuePackageProcess(t, root, host, home, route, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := started.command.stop(); err != nil {
			t.Error(err)
		}
		if err := started.process.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	return started
}

func assertFailedContinuationReplayResult(t *testing.T, err error, stdout, stderr string, flags []string) {
	t.Helper()
	if len(flags) == 1 && flags[0] == "--async" {
		var result directWorkerSessionCLIResult
		decodeDirectWorkerSessionResult(t, stdout, &result)
		if err != nil || !result.Accepted || result.State != "FAILED" || result.SuccessorWorkerSessionID != "restart-successor" {
			t.Fatalf("failed async replay: err=%v result=%#v stderr=%s", err, result, stderr)
		}
	} else if err == nil || !strings.Contains(stdout+stderr, "WORKER_SESSION_FAILED") {
		t.Fatalf("failed sync replay lost failure: err=%v stdout=%s stderr=%s", err, stdout, stderr)
	}
}

// F7-C7: a real Providers policy refusal must precede the joined stop boundary.
// The incapable root is a different immutable edge shape; all control cells
// share it, with an explicit Factory Session and test-owned command routes.
func TestCapturedProviderUnsupportedInterruptPreservesControls(t *testing.T) {
	t.Parallel()
	root, dir := t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := newNativeContinuationRunner("codex")
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
	started, err := startInvokeContinuePackageProcessWithCapabilities(t, root, host, home, route, []providerswire.CatalogCapabilityOverride{{Provider: providers.IDCodex, Capabilities: []providers.Capability{providers.CapabilityPromptSubmission, providers.CapabilityNativeStreaming, providers.CapabilityMessageDeltas, providers.CapabilityUsage}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := started.command.stop(); err != nil {
			t.Error(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := started.process.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	fixture := &invokeContinuePackageFixture{process: started.process, baseURL: started.baseURL, hostDir: host}
	scenario := &invokeContinueScenario{fixture: fixture, name: "native-policy", runNumber: 1, workingDirectory: dir, homeDirectory: home, providerRunner: runner, session: fixture.openSession(t)}
	assertUnsupportedProviderInterrupt(t, scenario, runner)
}

func assertUnsupportedProviderInterrupt(t *testing.T, scenario *invokeContinueScenario, runner *nativeContinuationRunner) {
	t.Helper()
	source, sibling, successor := scenarioScopedID(scenario, "source"), scenarioScopedID(scenario, "sibling"), scenarioScopedID(scenario, "successor")
	invokeNativeContinuationWorker(t, scenario, "codex", source, "native source", runner.sourceReady)
	t.Cleanup(func() { stopNativeContinuationWorker(t, scenario, source, true) })
	invokeNativeContinuationWorker(t, scenario, "codex", sibling, "native sibling", runner.siblingReady)
	t.Cleanup(func() { stopNativeContinuationWorker(t, scenario, sibling, true) })
	for _, remote := range []bool{false, true} {
		request := missingReferenceRequest(t, scenario, remote, []string{"worker-sessions", "interrupt", source,
			"--request-id", scenarioScopedID(scenario, "interrupt"), "--successor-worker-session-id", successor,
			"--replacement-message", "native follow-up", "--async"})
		assertDirectWorkerSessionCLIError(t, request, "PROVIDER_UNSUPPORTED")
		var failure struct {
			Phase string `json:"phase"`
		}
		decodeDirectWorkerSessionResult(t, request.Stderr(), &failure)
		if failure.Phase != "VALIDATION" {
			t.Fatalf("unsupported phase = %q", failure.Phase)
		}
		assertNativeContinuationObservation(t, scenario, source, "RUNNING", "opaque-native-source", "", "")
		assertNativeContinuationObservation(t, scenario, sibling, "RUNNING", "opaque-native-sibling", "", "")
	}
	absent := missingReferenceRequest(t, scenario, true, []string{"worker-sessions", "show", "--worker-session-id", successor})
	assertDirectWorkerSessionCLIError(t, absent, "WORKER_SESSION_NOT_FOUND")
	if runner.CallCount() != 2 {
		t.Fatalf("unsupported policy admitted provider: calls=%d", runner.CallCount())
	}
	stopNativeContinuationWorker(t, scenario, source, true)
	assertNativeContinuationObservation(t, scenario, source, "TERMINATED", "opaque-native-source", "", "")
	assertNativeContinuationObservation(t, scenario, sibling, "RUNNING", "opaque-native-sibling", "", "")
	assertUnsupportedTerminalContinuation(t, scenario, runner, source, sibling, successor)
}

func assertUnsupportedTerminalContinuation(t *testing.T, scenario *invokeContinueScenario, runner *nativeContinuationRunner, source, sibling, successor string) {
	t.Helper()
	for _, remote := range []bool{false, true} {
		request := missingReferenceRequest(t, scenario, remote, []string{"worker-sessions", "continue", source,
			"--request-id", scenarioScopedID(scenario, "continue"), "--successor-worker-session-id", successor,
			"--user-message", "native follow-up", "--async"})
		assertDirectWorkerSessionCLIError(t, request, "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID")
		assertNativeContinuationObservation(t, scenario, source, "TERMINATED", "opaque-native-source", "", "")
		assertNativeContinuationObservation(t, scenario, sibling, "RUNNING", "opaque-native-sibling", "", "")
		// Show is remote-only; the source control's local placement must not
		// route this absence check to the unrelated default host.
		absent := missingReferenceRequest(t, scenario, true, []string{"worker-sessions", "show", "--worker-session-id", successor})
		assertDirectWorkerSessionCLIError(t, absent, "WORKER_SESSION_NOT_FOUND")
	}
	if runner.CallCount() != 2 {
		t.Fatalf("terminal policy refusal admitted provider: calls=%d", runner.CallCount())
	}
}

// CLI/HTTP parity observes the durable original address after chain advancement
// and process reconstruction. Metadata reads must never launch a provider.
func assertContinuationHeadCapability(t *testing.T, host invokeContinueStartedProcess, home, dir, head string) {
	t.Helper()
	for _, flags := range [][]string{nil, {"--remote", "--server", host.baseURL}} {
		args := append([]string{"you", "--json", "--server", host.baseURL, "worker-sessions", "show", "--worker-session-id", "head-source"}, flags...)
		request := support.FakeInputs(t.Context(), args)
		request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
		if err := host.process.Execute(request.Input); err != nil {
			t.Fatalf("head observation: %v %s %s", err, request.Stdout(), request.Stderr())
		}
		var row factoryapi.WorkerSessionObservation
		decodeDirectWorkerSessionResult(t, request.Stdout(), &row)
		if row.Revivable == nil || !*row.Revivable || row.ContinuationHeadWorkerSessionId == nil || *row.ContinuationHeadWorkerSessionId != head {
			t.Fatalf("head capability lost after continuation/restart: %+v", row)
		}
	}
	rows := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, host.baseURL+"/worker-sessions?history=archived")
	for _, row := range rows.Sessions {
		if row.WorkerSessionId == "head-source" {
			if row.Revivable == nil || !*row.Revivable || row.ContinuationHeadWorkerSessionId == nil || *row.ContinuationHeadWorkerSessionId != head {
				t.Fatalf("archived list capability disagrees with show: %+v", row)
			}
			return
		}
	}
	t.Fatal("archived list omitted original address")
}
