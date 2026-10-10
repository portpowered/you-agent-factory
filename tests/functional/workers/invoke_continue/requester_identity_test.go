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

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	if child.providerRunner.CallCount() != 1 {
		t.Fatal("refused requester launched a provider")
	}
	if token != "" && strings.Contains(input.Stdout()+input.Stderr(), token) {
		t.Fatal("caller refusal disclosed credentials")
	}
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
