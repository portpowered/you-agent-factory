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
		assertRequesterSuccessorEnvironment(t, environment, successorID, parentID, previousToken)
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
