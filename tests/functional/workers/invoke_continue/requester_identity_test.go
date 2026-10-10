package acceptance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// The public CLI forwards credentials learned only at the controlled native
// provider edge. The running source and the child own independent routes and
// explicit Factory Sessions in the reusable process.
func TestRequesterDirectInvokeFromRunningCaller(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	parent := fixture.scenario(t, "requester-parent")
	child := fixture.scenario(t, "requester-child")
	defer parent.close(t)
	defer child.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := parent.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	parentID := scenarioScopedID(parent, "requester-source")
	parentPath := requesterExecutionPath(t, parent, parentID)
	start := t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "invoke", "--execution", parentPath, "--async")
	if err := fixture.process.Execute(start.Input); err != nil {
		t.Fatalf("start requester: %v: %s", err, start.Stderr())
	}
	t19AwaitSignal(t, ctx, runner.started, "requester native command running")
	token := requesterSourceToken(t, runner, parentID)
	childID := scenarioScopedID(child, "requester-child")
	path := requesterExecutionPath(t, child, childID)
	invoke := t7RemoteCLIInputs(child, ctx, fixture.baseURL, "invoke", "--execution", path)
	invoke.Input.Env = append(invoke.Input.Env, "YOU_WORKER_SESSION_ID="+parentID, "YOU_WORKER_SESSION_TOKEN="+token)
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatalf("attributed invoke: %v: %s", err, invoke.Stderr())
	}
	if child.providerRunner.CallCount() != 1 {
		t.Fatal("attributed child did not launch exactly once")
	}
	assertRequesterChild(t, fixture, child, ctx, childID, parentID, token)
	if strings.Contains(invoke.Stdout()+invoke.Stderr(), token) {
		t.Fatal("public invoke exposed the source credential")
	}
	for _, test := range []struct{ name, id, token string }{
		{"invalid", parentID, strings.Repeat("A", 43)},
		{"foreign", childID, token},
		{"partial", parentID, ""},
	} {
		assertRequesterRefusal(t, fixture, child, ctx, test.name, test.id, test.token)
	}
	stop := t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "cancel", parentID)
	if err := fixture.process.Execute(stop.Input); err != nil {
		t.Fatalf("end requester: %v: %s", err, stop.Stderr())
	}
	t19AwaitSignal(t, ctx, runner.stopped, "requester native command ended")
	assertRequesterRefusal(t, fixture, child, ctx, "ended", parentID, token)
	if child.providerRunner.CallCount() != 1 || runner.CallCount() != 1 {
		t.Fatal("refused callers launched or changed a provider attempt")
	}
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.show", "rest/startWorkerSession")
}

// M7 requires a separate root lifetime: both hosts use the same isolated
// profile and real capture storage; only the native provider process is fake.
func TestRequesterContinuationAfterHostRestart(t *testing.T) {
	t.Parallel()
	root, parentDir, childDir := t.TempDir(), t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	parentRunner := &t7GatedProviderRunner{}
	parentRunner.reset()
	result := platformprocess.CommandResult{Stdout: directCodexSessionOutput("t7-thread-requester-child", t7ObservationReport)}
	childRunner := testutil.NewProviderCommandRunner(result, result, result, result)
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{
		{workingDirectory: parentDir, runner: parentRunner}, {workingDirectory: childDir, runner: childRunner},
	}}
	first := startContinuationRestartHost(t, root, host, home, route)
	fixture := &invokeContinuePackageFixture{process: first.process, baseURL: first.baseURL, hostDir: host}
	parent := &invokeContinueScenario{fixture: fixture, name: "requester-restart-parent", runNumber: 1, workingDirectory: parentDir, homeDirectory: home, providerRunner: parentRunner, session: fixture.openSession(t)}
	child := &invokeContinueScenario{fixture: fixture, name: "requester-restart-child", runNumber: 1, workingDirectory: childDir, homeDirectory: home, providerRunner: childRunner, session: fixture.openSession(t)}
	defer t7ReleaseAndJoin(t, t.Context(), parentRunner)()
	parentID, sourceID := scenarioScopedID(parent, "requester-source"), scenarioScopedID(child, "requester-child")
	start := t7RemoteCLIInputs(parent, t.Context(), first.baseURL, "invoke", "--execution", requesterExecutionPath(t, parent, parentID), "--async")
	if err := first.process.Execute(start.Input); err != nil {
		t.Fatal(err)
	}
	t19AwaitSignal(t, t.Context(), parentRunner.started, "restart caller running")
	parentToken := requesterSourceToken(t, parentRunner, parentID)
	invoke := t7RemoteCLIInputs(child, t.Context(), first.baseURL, "invoke", "--execution", requesterExecutionPath(t, child, sourceID))
	invoke.Input.Env = append(invoke.Input.Env, "YOU_WORKER_SESSION_ID="+parentID, "YOU_WORKER_SESSION_TOKEN="+parentToken)
	if err := first.process.Execute(invoke.Input); err != nil {
		t.Fatal(err)
	}
	assertRequesterChild(t, fixture, child, t.Context(), sourceID, parentID, parentToken)
	assertRequesterContinuations(t, fixture, parent, child, t.Context(), sourceID, parentID)
	before := requesterObservation(t, fixture, child, t.Context(), sourceID)
	awaitContinuationRestartLogs(t, first, home, childDir, scenarioScopedID(child, "requester-head"), "t7-thread-requester-child")
	parent.close(t)
	child.close(t)
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	fixture.process, fixture.baseURL = fresh.process, fresh.baseURL
	child.session = fixture.openSession(t)
	defer child.close(t)
	assertRequesterRestoredChain(t, fixture, child, sourceID, parentID, parentToken, before)
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.show", "cli/you.worker-sessions.continue", "rest/startWorkerSession")
}

func assertRequesterRestoredChain(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, sourceID, parentID, parentToken string, before api.WorkerSessionObservation) {
	t.Helper()
	after := requesterObservation(t, fixture, child, t.Context(), sourceID)
	if !reflect.DeepEqual(before.Requester, after.Requester) || !reflect.DeepEqual(before.Correlation, after.Correlation) || !reflect.DeepEqual(before.Labels, after.Labels) || after.ContinuationHeadWorkerSessionId == nil || *after.ContinuationHeadWorkerSessionId != scenarioScopedID(child, "requester-head") {
		t.Fatal("reconstruction lost retained metadata or the exact chain head")
	}
	assertRequesterRefusal(t, fixture, child, t.Context(), "restart-parent", parentID, parentToken)
	for index, request := range child.providerRunner.Requests() {
		environment := requesterEnvironment(request.Env)
		assertRequesterRefusal(t, fixture, child, t.Context(), "restart-token-"+string(rune('a'+index)), environment["YOU_WORKER_SESSION_ID"], environment["YOU_WORKER_SESSION_TOKEN"])
		assertRequesterDurableTokenPrivacy(t, fixture, t.Context(), environment["YOU_WORKER_SESSION_ID"], environment["YOU_WORKER_SESSION_TOKEN"])
	}
	successorID := scenarioScopedID(child, "requester-rebuilt")
	input := t7RemoteCLIInputs(child, t.Context(), fixture.baseURL, "continue", sourceID, "--head", "--request-id", successorID+"-request", "--successor-worker-session-id", successorID, "--user-message", "requester rebuilt follow-up")
	if err := fixture.process.Execute(input.Input); err != nil {
		t.Fatalf("restored requester continuation: %v: %s", err, input.Stderr())
	}
	var continued directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, input.Stdout(), &continued)
	if continued.SourceWorkerSessionID != scenarioScopedID(child, "requester-head") || continued.State != "COMPLETED" || child.providerRunner.CallCount() != 4 {
		t.Fatal("reconstructed continuation did not advance the exact retained head once")
	}
	requests := child.providerRunner.Requests()
	environment := requesterEnvironment(requests[3].Env)
	assertRequesterEndpoint(t, environment, fixture.baseURL)
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, child.homeDirectory, child.workingDirectory, successorID, "t7-thread-requester-child")
	assertRequesterDurableTokenPrivacy(t, fixture, t.Context(), successorID, environment["YOU_WORKER_SESSION_TOKEN"])
	for _, request := range requests[:3] {
		assertRequesterSuccessorEnvironment(t, environment, successorID, parentID, requesterEnvironment(request.Env)["YOU_WORKER_SESSION_TOKEN"])
	}
	restored := requesterObservation(t, fixture, child, t.Context(), successorID)
	if !reflect.DeepEqual(before.Requester, restored.Requester) || !reflect.DeepEqual(before.Correlation, restored.Correlation) || !reflect.DeepEqual(before.Labels, restored.Labels) {
		t.Fatal("rebuilt successor lost retained requester metadata")
	}
}

func requesterSourceToken(t *testing.T, runner *t7GatedProviderRunner, parentID string) string {
	t.Helper()
	parentEnv := requesterEnvironment(runner.Requests()[0].Env)
	if parentEnv["YOU_WORKER_SESSION_ID"] != parentID || parentEnv["YOU_MESSAGE_TARGET"] != "" {
		t.Fatal("unattributed source identity was incorrect")
	}
	token := parentEnv["YOU_WORKER_SESSION_TOKEN"]
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(decoded) != 32 {
		t.Fatal("source did not receive a 32-byte caller credential")
	}
	return token
}

func assertRequesterChild(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, childID, parentID, token string) {
	t.Helper()
	childEnv := requesterEnvironment(child.providerRunner.Requests()[0].Env)
	assertRequesterEndpoint(t, childEnv, fixture.baseURL)
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(childEnv["YOU_WORKER_SESSION_TOKEN"])
	if err != nil || len(decoded) != 32 {
		t.Fatal("child did not receive a fresh 32-byte caller credential")
	}
	if childEnv["YOU_WORKER_SESSION_ID"] != childID || childEnv["YOU_MESSAGE_TARGET"] != parentID || childEnv["YOU_WORKER_SESSION_TOKEN"] == token {
		t.Fatal("child did not receive its own fresh identity and exact caller target")
	}
	show := t7RemoteCLIInputs(child, ctx, fixture.baseURL, "show", "--worker-session-id", childID)
	if err := fixture.process.Execute(show.Input); err != nil {
		t.Fatal(err)
	}
	var observation api.WorkerSessionObservation
	if err := json.Unmarshal([]byte(show.Stdout()), &observation); err != nil {
		t.Fatal(err)
	}
	if observation.Requester == nil || observation.Requester.WorkerSessionId != parentID || observation.Labels == nil || !reflect.DeepEqual(*observation.Labels, []string{"parent:" + parentID}) {
		t.Fatalf("public child metadata disagrees with verified caller identity: requester=%+v labels=%v state=%s", observation.Requester, observation.Labels, observation.State)
	}
	if strings.Contains(show.Stdout()+show.Stderr(), token) {
		t.Fatal("public output exposed the source credential")
	}
}

func assertRequesterRefusal(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, name, callerID, token string) {
	t.Helper()
	before := child.providerRunner.CallCount()
	id := scenarioScopedID(child, "requester-refusal-"+name)
	path := requesterExecutionPath(t, child, id)
	input := t7RemoteCLIInputs(child, ctx, fixture.baseURL, "invoke", "--execution", path, "--async")
	input.Input.Env = append(input.Input.Env, "YOU_WORKER_SESSION_ID="+callerID)
	if token != "" {
		input.Input.Env = append(input.Input.Env, "YOU_WORKER_SESSION_TOKEN="+token)
	}
	if err := fixture.process.Execute(input.Input); err == nil {
		t.Fatal("invalid requester was admitted")
	}
	assertDirectWorkerSessionCLIError(t, input, "WORKER_SESSION_CALLER_INVALID")
	if child.providerRunner.CallCount() != before {
		t.Fatal("refused requester launched a provider")
	}
	if token != "" && strings.Contains(input.Stdout()+input.Stderr(), token) {
		t.Fatal("caller refusal disclosed credentials")
	}
}

// M6/M8: public continuation copies the attributed source's metadata rather
// than retargeting to its predecessor. Replaying the tuple cannot advance the
// chain or launch another provider; the original running caller stays intact.
func assertRequesterContinuations(t *testing.T, fixture *invokeContinuePackageFixture, parent, child *invokeContinueScenario, ctx context.Context, sourceID, parentID string) {
	t.Helper()
	source := requesterObservation(t, fixture, child, ctx, sourceID)
	if source.ProviderSession == nil {
		t.Fatal("attributed source did not retain its native provider identity")
	}
	host := invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}
	awaitContinuationRestartLogs(t, host, child.homeDirectory, child.workingDirectory, sourceID, source.ProviderSession.Id)
	parentBefore := requesterObservation(t, fixture, parent, ctx, parentID)
	previousID := sourceID
	previousToken := requesterEnvironment(child.providerRunner.Requests()[0].Env)["YOU_WORKER_SESSION_TOKEN"]
	for index, name := range []string{"first", "head"} {
		successorID := scenarioScopedID(child, "requester-"+name)
		args := []string{"continue", sourceID, "--request-id", successorID + "-request", "--successor-worker-session-id", successorID, "--user-message", "requester follow-up"}
		if index > 0 {
			args = append(args, "--head")
		}
		assertRequesterContinuationReplay(t, child, ctx, args, previousID, successorID, index+2)
		awaitContinuationRestartLogs(t, host, child.homeDirectory, child.workingDirectory, successorID, source.ProviderSession.Id)
		observation := requesterObservation(t, fixture, child, ctx, successorID)
		if !reflect.DeepEqual(observation.Requester, source.Requester) || !reflect.DeepEqual(observation.Correlation, source.Correlation) || !reflect.DeepEqual(observation.Labels, source.Labels) {
			t.Fatal("successor changed retained requester metadata")
		}
		command := child.providerRunner.Requests()[index+1]
		environment := requesterEnvironment(command.Env)
		assertRequesterEndpoint(t, environment, fixture.baseURL)
		assertRequesterSuccessorEnvironment(t, environment, successorID, parentID, previousToken)
		assertRequesterDurableTokenPrivacy(t, fixture, ctx, successorID, environment["YOU_WORKER_SESSION_TOKEN"])
		if !strings.Contains(strings.Join(command.Args, " "), "resume "+source.ProviderSession.Id) {
			t.Fatal("successor did not resume the exact captured provider session")
		}
		assertRequesterRefusal(t, fixture, child, ctx, "terminal-"+name, successorID, environment["YOU_WORKER_SESSION_TOKEN"])
		previousID, previousToken = successorID, environment["YOU_WORKER_SESSION_TOKEN"]
	}
	parentAfter := requesterObservation(t, fixture, parent, ctx, parentID)
	// Elapsed active time advances while the independent caller remains running.
	if parentBefore.DurationMillis == nil || parentAfter.DurationMillis == nil || *parentAfter.DurationMillis < *parentBefore.DurationMillis {
		t.Fatal("running caller lost its active-clock duration")
	}
	parentBefore.DurationMillis, parentAfter.DurationMillis = nil, nil
	if !reflect.DeepEqual(parentBefore, parentAfter) || parent.providerRunner.CallCount() != 1 {
		t.Fatalf("direct continuation mutated the independent running caller: before=%+v after=%+v", parentBefore, parentAfter)
	}
}

func assertRequesterContinuationReplay(t *testing.T, child *invokeContinueScenario, ctx context.Context, args []string, sourceID, successorID string, calls int) {
	t.Helper()
	for index := range 2 {
		input := t7RemoteCLIInputs(child, ctx, child.fixture.baseURL, args...)
		if index == 0 {
			input = support.FakeInputs(ctx, append([]string{"you", "--json", "worker-sessions"}, args...))
			input.Input.Env, input.Input.WorkingDirectory = child.environment(), child.workingDirectory
		}
		if err := child.fixture.process.Execute(input.Input); err != nil {
			t.Fatalf("attributed continuation: %v: %s", err, input.Stderr())
		}
		var result directWorkerSessionCLIResult
		decodeDirectWorkerSessionResult(t, input.Stdout(), &result)
		if result.SourceWorkerSessionID != sourceID || result.SuccessorWorkerSessionID != successorID || result.State != "COMPLETED" || child.providerRunner.CallCount() != calls {
			t.Fatal("continuation/replay changed the exact chain or launched a duplicate")
		}
	}
}

func assertRequesterSuccessorEnvironment(t *testing.T, environment map[string]string, id, requester, previousToken string) {
	t.Helper()
	token := environment["YOU_WORKER_SESSION_TOKEN"]
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(decoded) != 32 || token == previousToken || environment["YOU_WORKER_SESSION_ID"] != id || environment["YOU_MESSAGE_TARGET"] != requester {
		t.Fatal("successor did not receive a fresh credential and its retained exact target")
	}
}

func requesterObservation(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, ctx context.Context, id string) api.WorkerSessionObservation {
	t.Helper()
	show := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "show", "--worker-session-id", id)
	if err := fixture.process.Execute(show.Input); err != nil {
		t.Fatal(err)
	}
	var observation api.WorkerSessionObservation
	decodeDirectWorkerSessionResult(t, show.Stdout(), &observation)
	for _, request := range scenario.providerRunner.Requests() {
		token := requesterEnvironment(request.Env)["YOU_WORKER_SESSION_TOKEN"]
		if token != "" && strings.Contains(show.Stdout()+show.Stderr(), token) {
			t.Fatal("public observation disclosed an execution credential")
		}
	}
	return observation
}

func requesterExecutionPath(t *testing.T, scenario *invokeContinueScenario, id string) string {
	t.Helper()
	path := filepath.Join(scenario.workingDirectory, id+".json")
	writeInvokeContinueJSON(t, path, invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-dispatch",
		workingDirectory: scenario.workingDirectory, userMessage: "requester controlled input",
	}))
	return path
}

func requesterEnvironment(environment []string) map[string]string {
	facts := make(map[string]string)
	for _, entry := range environment {
		if key, value, found := strings.Cut(entry, "="); found && strings.HasPrefix(key, "YOU_") {
			facts[key] = value
		}
	}
	return facts
}

func assertRequesterEndpoint(t *testing.T, environment map[string]string, endpoint string) {
	t.Helper()
	if environment["YOU_SERVER"] != endpoint {
		t.Fatalf("execution endpoint = %q, want bound host %q", environment["YOU_SERVER"], endpoint)
	}
}

// M10: both supported interruption modes preserve the admitted requester and
// issue a fresh execution credential. The requester remains an active peer.
func TestRequesterInterruptedSuccessor(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct{ name, mode string }{
		{"provider", "provider"}, {"recorded", "recorded"}, {"ack-input", "provider"}, {"ack-source", "recorded"},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			runRequesterInterruptedSuccessor(t, cell.name, cell.mode)
		})
	}
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.show", "cli/you.worker-sessions.list", "cli/you.worker-sessions.interrupt", "rest/interruptWorkerSession")
}

func runRequesterInterruptedSuccessor(t *testing.T, name, mode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx, "requester-interrupt-"+name)
	defer scenario.runner.releaseAll()
	if strings.HasPrefix(name, "ack-") {
		// The real journal commits, then the existing storage edge loses only
		// this operation's response. Admission must recover the stored metadata.
		scenario.ids.interruptRequest = "interrupt-" + name + "-" + scenario.ids.interruptRequest
	}
	ids := scenario.ids
	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestB, workerSessionID: ids.workerB, dispatchID: ids.dispatchB,
		factorySessionID: scenario.session.id, repository: scenario.repositoryB.path, workID: ids.workB, message: s8MessageB,
	})
	scenario.runner.waitStarted(t, scenario.repositoryB.path, s8InterruptCallBInitial)
	parentEnv := requesterEnvironment(scenario.runner.requests()[0].Env)
	parentBefore := requesterInterruptObservation(t, scenario, ids.workerB)
	callerEnv := append(append([]string(nil), scenario.env...), "YOU_WORKER_SESSION_ID="+ids.workerB, "YOU_WORKER_SESSION_TOKEN="+parentEnv["YOU_WORKER_SESSION_TOKEN"])
	invokeS8RemoteWorker(t, ctx, scenario.manager, callerEnv, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	source := requesterInterruptObservation(t, scenario, ids.workerA)
	sourceEnv := requesterEnvironment(scenario.runner.requests()[1].Env)
	assertRequesterSuccessorEnvironment(t, sourceEnv, ids.workerA, ids.workerB, parentEnv["YOU_WORKER_SESSION_TOKEN"])
	if source.Requester == nil || source.Requester.WorkerSessionId != ids.workerB || source.Labels == nil || !reflect.DeepEqual(*source.Labels, []string{"parent:" + ids.workerB}) {
		t.Fatal("interruption source did not retain its verified requester")
	}
	first := interruptRequesterSource(t, scenario, mode)
	assertRequesterInterruptReplay(t, scenario, mode, first)
	assertRequesterInterruptedMetadata(t, scenario, source, sourceEnv, parentBefore, parentEnv)
	assertRequesterInterruptCallerRefused(t, scenario, ids.workerA, sourceEnv["YOU_WORKER_SESSION_TOKEN"])
	scenario.runner.release(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	_ = replayS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.successor)
	assertRequesterInterruptCallerRefused(t, scenario, ids.successor, requesterEnvironment(scenario.runner.requests()[2].Env)["YOU_WORKER_SESSION_TOKEN"])
	assertS8WorkNotAdvanced(t, scenario.fixture, scenario.session.id, ids.workA, ids.workB)
	scenario.close(t)
}

func assertRequesterInterruptReplay(t *testing.T, scenario s8InterruptScenario, mode string, first s8InterruptResult) {
	t.Helper()
	ids := scenario.ids
	replay := postS8Interrupt(t, scenario.ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage, mode)
	if !reflect.DeepEqual(s8InterruptResultFromAPI(replay), first) {
		t.Fatal("attributed interrupt replay changed its admission snapshot")
	}
	otherMode := "provider"
	if mode == "provider" {
		otherMode = "recorded"
	}
	status, body, _, err := sendS8InterruptHTTP(scenario.ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage, otherMode)
	if err != nil || status != 409 || !strings.Contains(body, "WORKER_SESSION_INTERRUPT_REQUEST_ID_CONFLICT") || scenario.runner.CallCount() != 3 {
		t.Fatal("changed attributed interrupt replay was not refused before execution")
	}
}

func interruptRequesterSource(t *testing.T, scenario s8InterruptScenario, mode string) s8InterruptResult {
	t.Helper()
	ids := scenario.ids
	input := support.FakeInputs(scenario.ctx, []string{"you", "--remote", "--server", scenario.serverURL, "--json", "worker-sessions", "interrupt", ids.workerA,
		"--request-id", ids.interruptRequest, "--successor-worker-session-id", ids.successor,
		"--replacement-message", s8ReplacementMessage, "--resume-mode", mode, "--async"})
	input.Input.Env, input.Input.WorkingDirectory = scenario.env, scenario.repositoryA.path
	if err := scenario.manager.Execute(input.Input); err != nil {
		t.Fatal("attributed interruption failed")
	}
	var first s8InterruptResult
	decodeS8JSON(t, input.Stdout(), &first)
	assertS8InterruptAdmission(t, first, ids)
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	scenario.runner.assertOrder(t, "start:"+s8InterruptCallAInitial, "cancel:"+s8InterruptCallAInitial, "start:"+s8InterruptCallASuccessor)
	for _, request := range scenario.runner.requests() {
		token := requesterEnvironment(request.Env)["YOU_WORKER_SESSION_TOKEN"]
		if token != "" && strings.Contains(input.Stdout()+input.Stderr(), token) {
			t.Fatal("interruption response disclosed execution credentials")
		}
	}
	return first
}

func assertRequesterInterruptedMetadata(t *testing.T, scenario s8InterruptScenario, source api.WorkerSessionObservation, sourceEnv map[string]string, parentBefore api.WorkerSessionObservation, parentEnv map[string]string) {
	t.Helper()
	ids := scenario.ids
	successor := requesterInterruptObservation(t, scenario, ids.successor)
	if !reflect.DeepEqual(source.Requester, successor.Requester) || !reflect.DeepEqual(source.Correlation, successor.Correlation) || !reflect.DeepEqual(source.Labels, successor.Labels) {
		t.Fatal("interrupted successor changed admitted requester metadata")
	}
	assertInterruptModeLineage(t, scenario.ctx, scenario)
	requests := scenario.runner.requests()
	if len(requests) != 3 || scenario.runner.cancellationCount(s8InterruptCallBInitial) != 0 {
		t.Fatal("interruption replay launched a duplicate or stopped the requester")
	}
	environment := requesterEnvironment(requests[2].Env)
	assertRequesterSuccessorEnvironment(t, environment, ids.successor, ids.workerB, sourceEnv["YOU_WORKER_SESSION_TOKEN"])
	if environment["YOU_WORKER_SESSION_TOKEN"] == parentEnv["YOU_WORKER_SESSION_TOKEN"] || environment["YOU_WORK_ID"] != sourceEnv["YOU_WORK_ID"] || environment["YOU_FACTORY_SESSION_ID"] != sourceEnv["YOU_FACTORY_SESSION_ID"] {
		t.Fatal("successor reused requester authority or changed retained correlation")
	}
	parentAfter := requesterInterruptObservation(t, scenario, ids.workerB)
	if parentBefore.DurationMillis == nil || parentAfter.DurationMillis == nil || *parentAfter.DurationMillis < *parentBefore.DurationMillis {
		t.Fatal("active requester lost its elapsed duration")
	}
	parentBefore.DurationMillis, parentAfter.DurationMillis = nil, nil
	if !reflect.DeepEqual(parentBefore, parentAfter) {
		t.Fatal("interruption changed the active requester observation")
	}
	assertRequesterInterruptListedMetadata(t, scenario, successor)
}

func assertRequesterInterruptCallerRefused(t *testing.T, scenario s8InterruptScenario, id, token string) {
	t.Helper()
	invocation := s8RemoteWorkerInvocation{requestID: id + "-refused-request", workerSessionID: id + "-refused-child", dispatchID: id + "-refused-dispatch",
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: scenario.ids.workA, message: s8MessageA}
	path := s8ExecutionDocument(t, invocation)
	input := support.FakeInputs(scenario.ctx, []string{"you", "--remote", "--server", scenario.serverURL, "--json", "worker-sessions", "invoke", "--execution", path, "--async"})
	input.Input.Env = append(append([]string(nil), scenario.env...), "YOU_WORKER_SESSION_ID="+id, "YOU_WORKER_SESSION_TOKEN="+token)
	input.Input.WorkingDirectory = scenario.repositoryA.path
	if err := scenario.manager.Execute(input.Input); err == nil {
		t.Fatal("interrupted or terminal credential retained requester authority")
	}
	assertDirectWorkerSessionCLIError(t, input, "WORKER_SESSION_CALLER_INVALID")
	if strings.Contains(input.Stdout()+input.Stderr(), token) || scenario.runner.CallCount() != 3 {
		t.Fatal("retired credential refusal disclosed authority or launched a provider")
	}
}

func assertRequesterInterruptListedMetadata(t *testing.T, scenario s8InterruptScenario, expected api.WorkerSessionObservation) {
	t.Helper()
	args := []string{"--json", "worker-sessions", "list", "--scope", "direct"}
	seen := make(map[string]bool)
	for {
		input := executeS8RemoteCLI(t, scenario.ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL, args...)
		var page api.ListWorkerSessionsResponse
		decodeS8JSON(t, input.Stdout(), &page)
		for _, row := range page.Sessions {
			if row.WorkerSessionId != expected.WorkerSessionId {
				continue
			}
			if !reflect.DeepEqual(row.Requester, expected.Requester) || !reflect.DeepEqual(row.Correlation, expected.Correlation) || !reflect.DeepEqual(row.Labels, expected.Labels) {
				t.Fatal("interrupted successor show/list metadata disagreed")
			}
			return
		}
		if page.PaginationContext == nil || page.PaginationContext.NextToken == nil || *page.PaginationContext.NextToken == "" || seen[*page.PaginationContext.NextToken] {
			t.Fatal("interrupted successor absent from public list or cursor repeated")
		}
		next := *page.PaginationContext.NextToken
		seen[next] = true
		args = []string{"--json", "worker-sessions", "list", "--scope", "direct", "--next-token", next}
	}
}

func requesterInterruptObservation(t *testing.T, scenario s8InterruptScenario, id string) api.WorkerSessionObservation {
	t.Helper()
	input := support.FakeInputs(scenario.ctx, []string{"you", "--remote", "--server", scenario.serverURL, "--json", "worker-sessions", "show", "--worker-session-id", id})
	input.Input.Env, input.Input.WorkingDirectory = scenario.env, scenario.factoryDir
	if err := scenario.manager.Execute(input.Input); err != nil {
		t.Fatal("interrupted requester observation unavailable")
	}
	var observation api.WorkerSessionObservation
	decodeDirectWorkerSessionResult(t, input.Stdout(), &observation)
	for _, request := range scenario.runner.requests() {
		token := requesterEnvironment(request.Env)["YOU_WORKER_SESSION_TOKEN"]
		if token != "" && strings.Contains(input.Stdout()+input.Stderr(), token) {
			t.Fatal("interrupted requester observation disclosed execution credentials")
		}
	}
	return observation
}
