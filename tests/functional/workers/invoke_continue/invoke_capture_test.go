package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// M13 uses the real recording store except for the selected recipe write.
// Each attributed failure owns separate routes and Factory Sessions, so a
// parallel peer cannot supply the credential or mask a provider launch.
func TestRequesterRecipeFailureBeforeAdmission(t *testing.T) {
	t.Cleanup(func() {
		if !t.Failed() {
			functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.continue", "cli/you.worker-sessions.show")
		}
	})
	for _, operation := range []string{"start", "continue"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			runRequesterRecipeFailure(t, operation)
		})
	}
}

func runRequesterRecipeFailure(t *testing.T, operation string) {
	t.Helper()
	fixture := ensureInvokeContinuePackageFixture(t)
	parent := fixture.scenario(t, "requester-recipe-parent-"+operation)
	child := fixture.scenario(t, "requester-recipe-child-"+operation)
	defer parent.close(t)
	defer child.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := parent.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	parentID := scenarioScopedID(parent, "requester-recipe-parent")
	start := t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "invoke", "--execution", requesterExecutionPath(t, parent, parentID), "--async")
	if err := fixture.process.Execute(start.Input); err != nil {
		t.Fatal("recipe-failure requester did not start")
	}
	t19AwaitSignal(t, ctx, runner.started, "recipe-failure requester running")
	token := requesterSourceToken(t, runner, parentID)
	parentBefore := requesterObservation(t, fixture, parent, ctx, parentID)
	failedID := "requester-recipe-failed-" + scenarioScopedID(child, operation)
	args := []string{"invoke", "--execution", requesterExecutionPath(t, child, failedID), "--async"}
	expectedCode, expectedCalls := "WORKER_SESSION_START_OPENING_FAILED", 0
	if operation == "continue" {
		sourceID := scenarioScopedID(child, "requester-recipe-source")
		invoke := t7RemoteCLIInputs(child, ctx, fixture.baseURL, "invoke", "--execution", requesterExecutionPath(t, child, sourceID))
		invoke.Input.Env = append(invoke.Input.Env, "YOU_WORKER_SESSION_ID="+parentID, "YOU_WORKER_SESSION_TOKEN="+token)
		if err := fixture.process.Execute(invoke.Input); err != nil {
			t.Fatal("attributed recipe-failure source did not complete")
		}
		assertRequesterChild(t, fixture, child, ctx, sourceID, parentID, token)
		source := requesterObservation(t, fixture, child, ctx, sourceID)
		if source.ProviderSession == nil {
			t.Fatal("attributed source did not retain its provider identity")
		}
		awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, child.homeDirectory, child.workingDirectory, sourceID, source.ProviderSession.Id)
		defer assertRequesterRecipeSourceUnchanged(t, fixture, child, ctx, sourceID, source)
		args = []string{"continue", sourceID, "--request-id", failedID + "-request", "--successor-worker-session-id", failedID, "--user-message", "recipe failure follow-up", "--async"}
		expectedCode, expectedCalls = "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED", 1
	}
	for range 2 {
		input := t7RemoteCLIInputs(child, ctx, fixture.baseURL, args...)
		input.Input.Env = append(input.Input.Env, "YOU_WORKER_SESSION_ID="+parentID, "YOU_WORKER_SESSION_TOKEN="+token)
		if err := fixture.process.Execute(input.Input); err == nil {
			t.Fatal("failed recipe write admitted a provider execution")
		}
		assertDirectWorkerSessionCLIError(t, input, expectedCode)
		assertRequesterTokenAbsent(t, token, input.Stdout()+input.Stderr())
		if strings.Contains(input.Stdout()+input.Stderr(), "private-recipe-sync-detail") {
			t.Fatal("recipe failure disclosed private storage diagnostics")
		}
	}
	if child.providerRunner.CallCount() != expectedCalls || runner.CallCount() != 1 {
		t.Fatal("recipe failure or replay launched a provider or changed its peer")
	}
	failed := requesterObservation(t, fixture, child, ctx, failedID)
	if string(failed.State) != "FAILED" || failed.Revivable == nil || *failed.Revivable {
		t.Fatal("unadmitted recipe failure fabricated a runnable retained session")
	}
	parentAfter := requesterObservation(t, fixture, parent, ctx, parentID)
	parentBefore.DurationMillis, parentAfter.DurationMillis = nil, nil
	if !reflect.DeepEqual(parentBefore, parentAfter) {
		t.Fatal("recipe failure mutated the independent running requester")
	}
}

func assertRequesterRecipeSourceUnchanged(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, id string, before api.WorkerSessionObservation) {
	t.Helper()
	after := requesterObservation(t, fixture, child, ctx, id)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("unadmitted successor changed source metadata, state or continuation head")
	}
}

// M11 observes the actual admitted credential echoed by the native command
// edge, through live capture and terminal public CLI/HTTP representations.
// Each parallel scenario owns its session and provider route on the shared host.
func TestRequesterExecutionTokenPrivacy(t *testing.T) {
	t.Cleanup(func() {
		if !t.Failed() {
			functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.read", "cli/you.worker-sessions.show", "rest/readWorkerSessionLogs", "rest/readWorkerSessionTranscriptByWorkerSessionId", "rest/getWorkerSessionObservationByWorkerSessionId", "rest/streamWorkerSessionEventsByTopLevelWorkerSessionId")
		}
	})
	for _, outcome := range []string{"success", "failure"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			fixture := ensureInvokeContinuePackageFixture(t)
			scenario := fixture.scenario(t, "requester-privacy-"+outcome)
			defer scenario.close(t)
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			runner := scenario.providerRunner.(*t7GatedProviderRunner)
			defer t7ReleaseAndJoin(t, ctx, runner)()
			id := scenarioScopedID(scenario, "requester-private")
			path := requesterExecutionPath(t, scenario, id)
			start := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path, "--async")
			if err := fixture.process.Execute(start.Input); err != nil {
				t.Fatal("privacy admission failed")
			}
			t19AwaitSignal(t, ctx, runner.started, "credential echo progress")
			token := requesterSourceToken(t, runner, id)
			awaitRequesterPrivacyLogs(t, fixture, ctx, id, "public credential progress", token)
			close(runner.release)
			join := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
			joinErr := fixture.process.Execute(join.Input)
			if (joinErr != nil) != (outcome == "failure") {
				t.Fatal("credential echo execution did not retain its expected outcome")
			}
			assertRequesterTokenAbsent(t, token, start.Stdout()+start.Stderr()+join.Stdout()+join.Stderr())
			awaitRequesterPrivacyLogs(t, fixture, ctx, id, `"health":"COMPLETE"`, token)
			assertRequesterPrivacyReads(t, fixture, scenario, ctx, id, token, outcome)
			assertRequesterDurableTokenPrivacy(t, fixture, ctx, id, token)
			assertRequesterRefusal(t, fixture, scenario, ctx, "terminal-privacy", id, token)
			if runner.CallCount() != 1 {
				t.Fatal("privacy replay or refused retired credential launched another attempt")
			}
		})
	}
}

func requesterPrivacyProgress(request platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) {
	if observe == nil || !strings.HasPrefix(filepath.Base(request.WorkDir), "requester-privacy-") {
		return
	}
	token := requesterEnvironment(request.Env)["YOU_WORKER_SESSION_TOKEN"]
	item, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{
		"id": "credential-progress", "type": "command_execution", "exit_code": 0,
		"command": "public credential command " + token, "aggregated_output": "public credential progress " + token,
	}})
	observe(platformprocess.OutputStreamStdout, append(item, '\n'))
	observe(platformprocess.OutputStreamStderr, []byte("public credential diagnostic "+token+"\n"))
}

func requesterPrivacyResult(request platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	token := requesterEnvironment(request.Env)["YOU_WORKER_SESSION_TOKEN"]
	output := directCodexSessionOutput("requester-private-thread", "public credential result "+token)
	diagnostic := []byte("public credential failure " + token + "\n")
	failed := filepath.Base(request.WorkDir) == "requester-privacy-failure"
	if failed {
		failure, _ := json.Marshal(map[string]any{"type": "turn.failed", "error": map[string]string{"message": "public credential failure " + token}})
		output = append([]byte("{\"type\":\"thread.started\",\"thread_id\":\"requester-private-thread\"}\n"), append(failure, '\n')...)
	}
	if observe != nil {
		observe(platformprocess.OutputStreamStdout, output)
		observe(platformprocess.OutputStreamStderr, diagnostic)
	}
	result := platformprocess.CommandResult{Stdout: output, Stderr: diagnostic}
	if failed {
		result.ExitCode = 1
		return result, errors.New("public credential runner failure " + token)
	}
	return result, nil
}

func awaitRequesterPrivacyLogs(t *testing.T, fixture *invokeContinuePackageFixture, ctx context.Context, id, marker, token string) {
	t.Helper()
	// Capture commits asynchronously after the command callback; only a public
	// read can establish that this session's echoed progress is durably readable.
	body, err := support.WaitForObservation(60*time.Second, func() (string, error) {
		_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
		assertRequesterTokenAbsent(t, token, body)
		return body, nil
	}, func(body string) bool { return strings.Contains(body, marker) })
	if err != nil || !strings.Contains(body, "redacted") {
		t.Fatal("public capture did not retain sanitized credential echo evidence")
	}
}

func assertRequesterPrivacyReads(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, ctx context.Context, id, token, outcome string) {
	t.Helper()
	state := "COMPLETED"
	if outcome == "failure" {
		state = "FAILED"
	}
	observation := requesterObservation(t, fixture, scenario, ctx, id)
	if string(observation.State) != state {
		t.Fatal("privacy observation lost the terminal execution outcome")
	}
	for _, view := range []string{"transcript", "logs"} {
		read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--view", view, "--worker-session-id", id)
		if err := fixture.process.Execute(read.Input); err != nil {
			t.Fatalf("public privacy %s read failed", view)
		}
		assertRequesterTokenAbsent(t, token, read.Stdout()+read.Stderr())
	}
	for _, suffix := range []string{"", "/transcript", "/logs", "/events?replayOnly=true"} {
		status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+suffix, nil)
		if status != http.StatusOK {
			t.Fatalf("public privacy HTTP %s read failed: %d", suffix, status)
		}
		assertRequesterTokenAbsent(t, token, body)
		if strings.HasPrefix(suffix, "/events") && !strings.Contains(body, "public credential progress") {
			t.Fatal("retained Events replay lost the observed credential echo")
		}
	}
}

func assertRequesterTokenAbsent(t *testing.T, token, body string) {
	t.Helper()
	if token == "" || strings.Contains(body, token) {
		t.Fatal("public output exposed the admitted execution credential or no credential was tested")
	}
}

// Inspect the persisted control-input contract as well as its decoded payload:
// input is base64 encoded on disk, so checking only raw bytes misses leaks.
// The exact session key excludes concurrent scenarios on the shared profile.
func assertRequesterDurableTokenPrivacy(t *testing.T, fixture *invokeContinuePackageFixture, ctx context.Context, id, token string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(fixture.hostDir, ".you-agent-factory", "worker-recordings", "worker-payloads", "*.control.json"))
	if err != nil {
		t.Fatal("durable control inputs unavailable")
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal("durable control input unreadable")
		}
		var artifact struct {
			Key   recordings.WorkerControlOperationKey `json:"key"`
			Input []byte                               `json:"input"`
		}
		if json.Unmarshal(data, &artifact) != nil {
			// Other parallel scenarios deliberately corrupt their inputs. The
			// exact recipe below must still be found and decoded for this session.
			continue
		}
		if artifact.Key.WorkerSessionID != id || !strings.HasPrefix(artifact.Key.RequestID, "restart-recipe/") {
			continue
		}
		assertRequesterTokenAbsent(t, token, string(data))
		assertRequesterTokenAbsent(t, token, string(artifact.Input))
		assertRequesterRecordedTokenPrivacy(t, fixture, ctx, artifact.Key.RecordingID, id, token)
		return
	}
	t.Fatal("execution has no persisted continuation recipe")
}

func assertRequesterRecordedTokenPrivacy(t *testing.T, fixture *invokeContinuePackageFixture, ctx context.Context, recordingID, id, token string) {
	t.Helper()
	snapshot, err := fixture.process.WorkerRecordingReader().LoadWorkerRecording(ctx, recordingID)
	if err != nil {
		t.Fatal("persisted Worker recording unavailable")
	}
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID != id {
			continue
		}
		data, err := json.Marshal(session)
		if err != nil || session.Status != recordings.WorkerRecordingStatusComplete || len(session.Records) < 2 || session.ExecutionTerminal == nil {
			t.Fatal("persisted Worker recording omitted complete execution evidence")
		}
		assertRequesterTokenAbsent(t, token, string(data))
		return
	}
	t.Fatal("persisted Worker recording omitted the exact execution")
}

const (
	t7SecretPrompt = "synthetic-private-t7-user-message"
	t7SecretSystem = "synthetic-private-t7-system-prompt"
	t7SecretToken  = "synthetic-private-t7-api-token"
)

// F6-10 observes the delivered redaction policy after the native adapter
// echoes classified request values in otherwise public tool progress.
func TestT7CapturedProgressRedactsRequestSecrets(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.read", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "t7-secrets")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runner := scenario.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	id := scenarioScopedID(scenario, "private-session")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
		workingDirectory: scenario.workingDirectory, userMessage: t7SecretPrompt,
	})
	execution := document["execution"].(map[string]any)
	execution["systemPrompt"] = t7SecretSystem
	execution["envVars"] = map[string]string{"T7_API_TOKEN": t7SecretToken}
	path := filepath.Join(scenario.workingDirectory, "private.json")
	writeInvokeContinueJSON(t, path, document)
	start := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--async", "--execution", path)
	if err := fixture.process.Execute(start.Input); err != nil {
		t.Fatalf("private invoke: %v: %s", err, start.Stderr())
	}
	t19AwaitSignal(t, ctx, runner.started, "private progress")
	// Capture is asynchronous; await only this session's committed progress.
	body, err := support.WaitForObservation(60*time.Second, func() (string, error) {
		_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
		return body, nil
	}, func(body string) bool { return strings.Contains(body, "public progress") })
	if err != nil {
		t.Fatalf("private capture: %v: %s", err, body)
	}
	t7AssertSecretsAbsent(t, body)
	if !strings.Contains(body, "redacted") {
		t.Fatalf("progress did not retain redaction marker: %s", body)
	}
	close(runner.release)
	join := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(join.Input); err != nil {
		t.Fatalf("private completion: %v: %s", err, join.Stderr())
	}
	logs := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--view", "logs", "--worker-session-id", id)
	if err := fixture.process.Execute(logs.Input); err != nil {
		t.Fatalf("private archived logs: %v", err)
	}
	t7AssertSecretsAbsent(t, logs.Stdout()+logs.Stderr()+start.Stdout()+start.Stderr()+join.Stdout()+join.Stderr())
	status, observation := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id, nil)
	if status != http.StatusOK || runner.CallCount() != 1 {
		t.Fatalf("private observation = %d, calls = %d", status, runner.CallCount())
	}
	t7AssertSecretsAbsent(t, observation)
}

func t7AssertSecretsAbsent(t *testing.T, body string) {
	t.Helper()
	for _, secret := range []string{t7SecretPrompt, t7SecretSystem, t7SecretToken} {
		if strings.Contains(body, secret) {
			t.Fatal("public capture or diagnostics exposed a classified synthetic secret")
		}
	}
}

// F6-10 injects one selected session's append failure at the existing durable
// writer edge. Its opening remains real and stopping still joins the attempt.
func TestT7CaptureLossRetainsPrefixAndLiveStop(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.terminate", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "t7-degraded")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runner := scenario.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	id, _ := t7StartGatedAsync(t, ctx, fixture, scenario)
	t19AwaitSignal(t, ctx, runner.started, "capture-loss attempt progress")
	// Capture consumes the Events topic asynchronously. Polling the public
	// projection is required to observe its committed prefix independently
	// of the provider command callback; no shared state is inspected.
	if body, err := support.WaitForObservation(60*time.Second, func() (string, error) {
		status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
		if status != http.StatusOK {
			return "", nil
		}
		return body, nil
	}, func(body string) bool {
		return strings.Contains(body, `"health":"INCOMPLETE"`) && strings.Contains(body, `"committedPosition":2`)
	}); err != nil {
		t.Fatalf("direct capture retains live prefix: %v: %s", err, body)
	}
	stop := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "terminate", id)
	if err := fixture.process.Execute(stop.Input); err != nil {
		t.Fatalf("degraded attempt stop: %v: %s", err, stop.Stderr())
	}
	t7AssertStopResult(t, stop.Stdout(), id, "terminate")
	t19AwaitSignal(t, ctx, runner.stopped, "degraded attempt joined")
	status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(body, `"health":"DEGRADED"`) || !strings.Contains(body, `"position":1`) || strings.Contains(body, "private-t7-capture-failure") {
		t.Fatalf("degraded prefix lost or health fabricated: %d %s", status, body)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("degraded attempt provider calls = %d", runner.CallCount())
	}
}
