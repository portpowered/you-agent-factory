package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
			functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.list", "cli/you.worker-sessions.read", "cli/you.worker-sessions.show", "rest/readWorkerSessionLogs", "rest/readWorkerSessionTranscriptByWorkerSessionId", "rest/getWorkerSessionObservationByWorkerSessionId", "rest/streamWorkerSessionEventsByTopLevelWorkerSessionId")
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
		if !strings.Contains(read.Stdout(), "public credential") || !strings.Contains(read.Stdout(), "redacted") {
			t.Fatalf("public privacy %s read lost the sanitized ordinary content", view)
		}
	}
	assertRequesterPrivacyListShow(t, fixture, scenario, ctx, observation, token)
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

type recordingPrivateProfile struct {
	host                         invokeContinueStartedProcess
	home, dir, id, marker, token string
	runner                       *testutil.ProviderCommandRunner
}

// FC6 needs two independently owned profiles: a shared host cannot prove that
// a cursor for the same Worker ID is fenced by the durable profile owner.
func TestRecordingContentPrivateProfilesFenceReadsAndCursors(t *testing.T) {
	t.Parallel()
	profiles := make([]recordingPrivateProfile, 2)
	for i, name := range []string{"private-alpha", "private-beta"} {
		root, dir := t.TempDir(), t.TempDir()
		host, home, err := prepareInvokeContinuePackageRoot(t, root)
		if err != nil {
			t.Fatal(err)
		}
		marker := name + "-captured-content"
		result := platformprocess.CommandResult{Stdout: directCodexSessionOutput(name+"-thread", marker)}
		runner := testutil.NewProviderCommandRunner(result, result)
		route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
		started := startContinuationRestartHost(t, root, host, home, route)
		for _, id := range []string{name, "same-worker-id"} {
			path := filepath.Join(dir, id+".json")
			writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt", workingDirectory: dir, userMessage: "ordinary private-profile prompt"})
			summaryRestartCLI(t, started, home, dir, "invoke", "--execution", path)
		}
		page := support.GetJSON[api.WorkerSessionLogPage](t, started.baseURL+"/worker-sessions/same-worker-id/logs?limit=1")
		if page.NextToken == nil {
			t.Fatal("private recording has no continuation cursor")
		}
		profiles[i] = recordingPrivateProfile{host: started, home: home, dir: dir, id: name, marker: marker, token: *page.NextToken, runner: runner}
	}
	for i, profile := range profiles {
		foreign := profiles[1-i]
		for _, view := range []string{"", "/logs", "/transcript"} {
			code := "NOT_FOUND"
			if view == "/logs" {
				code = "WORKER_SESSION_NOT_FOUND"
			}
			assertRecordingReadDenial(t, profile.host.baseURL+"/worker-sessions/"+foreign.id+view, http.StatusNotFound, code, foreign.marker, foreign.id)
		}
		// Same Worker ID, different profile: this isolates the profile fence.
		assertRecordingReadDenial(t, profile.host.baseURL+"/worker-sessions/same-worker-id/logs?nextToken="+url.QueryEscape(foreign.token), http.StatusBadRequest, "BAD_REQUEST", foreign.marker, foreign.id)
		// Same profile, different Worker ID: this isolates the session fence.
		assertRecordingReadDenial(t, profile.host.baseURL+"/worker-sessions/"+profile.id+"/logs?nextToken="+url.QueryEscape(profile.token), http.StatusBadRequest, "BAD_REQUEST", foreign.marker, foreign.id)
		for _, suffix := range []string{"", "/transcript"} {
			assertRecordingReadDenial(t, profile.host.baseURL+"/factory-sessions/foreign/worker-sessions/"+profile.id+suffix, http.StatusNotFound, "NOT_FOUND", profile.marker, profile.id)
		}
		assertRecordingPrivateProfileReadable(t, profile, foreign)
	}
}

func assertRecordingPrivateProfileReadable(t *testing.T, profile, foreign recordingPrivateProfile) {
	t.Helper()
	for _, id := range []string{profile.id, "same-worker-id"} {
		for _, view := range []string{"summary", "logs", "transcript"} {
			args := []string{"read", "--worker-session-id", id, "--view", view}
			if view == "summary" {
				args = []string{"show", "--worker-session-id", id}
			}
			body := summaryRestartCLI(t, profile.host, profile.home, profile.dir, args...)
			if strings.Contains(body, foreign.marker) || strings.Contains(body, foreign.id) || (view != "summary" && !strings.Contains(body, profile.marker)) {
				t.Fatalf("private peer read changed or leaked: %s", body)
			}
		}
	}
	if profile.runner.CallCount() != 2 {
		t.Fatal("read denials caused provider execution")
	}
}

func assertRecordingReadDenial(t *testing.T, endpoint string, wantStatus int, wantCode string, privateValues ...string) {
	t.Helper()
	status, body := t7HTTP(t, t.Context(), http.MethodGet, endpoint, nil)
	var failure api.ErrorResponse
	if err := json.Unmarshal([]byte(body), &failure); err != nil || status != wantStatus || string(failure.Code) != wantCode {
		t.Fatalf("recording read denial: status=%d error=%v body=%s", status, err, body)
	}
	for _, value := range privateValues {
		if strings.Contains(body, value) {
			t.Fatalf("denied read exposed private content or identity: %s", body)
		}
	}
}

func TestRecordingContentDirectEchoAndNonEcho(t *testing.T) {
	for _, name := range []string{"recording-echo", "recording-other", "recording-equal", "recording-snapshot", "recording-final-only"} {
		t.Run(name, func(t *testing.T) { t.Parallel(); runRecordingContentDirect(t, name) })
	}
}

func runRecordingContentDirect(t *testing.T, name string) {
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, name)
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := scenario.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	id := scenarioScopedID(scenario, "content")
	prompt := "ordinary nonoverlapping prompt"
	if name == "recording-echo" {
		prompt = "DIRECT_CAPTURE_BETA"
	}
	path := filepath.Join(scenario.workingDirectory, "content.json")
	writeInvokeContinueJSON(t, path, invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
		workingDirectory: scenario.workingDirectory, userMessage: prompt,
	}))
	start := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--async", "--execution", path)
	if err := fixture.process.Execute(start.Input); err != nil {
		t.Fatalf("invoke: %v: %s", err, start.Stderr())
	}
	t19AwaitSignal(t, ctx, runner.started, "ordinary output")
	// The durable reader commits asynchronously after provider observation.
	// Await this session's public prefix rather than provider callback timing.
	liveMarker := "DIRECT_CAPTURE_BETA"
	if name == "recording-snapshot" {
		liveMarker = "DIRECT_CAPTURE_"
	}
	if name != "recording-final-only" {
		if body, err := support.WaitForObservation(60*time.Second, func() (string, error) {
			_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
			return body, nil
		}, func(body string) bool { return strings.Contains(body, liveMarker) }); err != nil {
			t.Fatalf("live content: %v: %s", err, body)
		}
	} else {
		// This command emits only session association before the release
		// gate. There is no message prefix for a reader to await.
	}
	close(runner.release)
	join := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(join.Input); err != nil {
		t.Fatalf("join: %v: %s", err, join.Stderr())
	}
	assertRecordingDirectReads(t, ctx, fixture, scenario, id, name)
}

func assertRecordingDirectReads(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id, name string) {
	t.Helper()
	_, summary := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id, nil)
	var observation api.WorkerSessionObservation
	if err := json.Unmarshal([]byte(summary), &observation); err != nil {
		t.Fatal(err)
	}
	if observation.TokenUsage != nil {
		t.Fatalf("unknown native-path usage became known: %+v", observation.TokenUsage)
	}
	page := recordingContentPages(t, ctx, fixture.baseURL, id)
	for _, frame := range page.Events {
		if frame.Event.Payload["kind"] == string(workers.KindUsage) {
			t.Fatal("unknown provider accounting invented a captured usage fact")
		}
	}
	assertRecordingMessageProvenance(t, page, name == "recording-final-only")
	for _, view := range []string{"logs", "transcript"} {
		read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--view", view, "--worker-session-id", id)
		if err := fixture.process.Execute(read.Input); err != nil {
			t.Fatalf("read %s: %v: %s", view, err, read.Stderr())
		}
		status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/"+view, nil)
		if status != http.StatusOK || !strings.Contains(read.Stdout(), "DIRECT_CAPTURE_BETA") || !strings.Contains(body, "DIRECT_CAPTURE_BETA") {
			t.Fatalf("completed %s lost ordinary output: %d %s / %s", view, status, read.Stdout(), body)
		}
		if view == "transcript" {
			assertRecordingDirectTranscript(t, read.Stdout(), body, name)
		}
	}
}

func assertRequesterPrivacyListShow(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, ctx context.Context, observation api.WorkerSessionObservation, token string) {
	t.Helper()
	listed := awaitRequesterPrivacyList(t, fixture, scenario, ctx, token)
	var rows api.ListWorkerSessionsResponse
	decodeDirectWorkerSessionResult(t, listed, &rows)
	found := false
	for _, row := range rows.Sessions {
		if row.WorkerSessionId == observation.WorkerSessionId {
			found = true
			if row.State != observation.State || !reflect.DeepEqual(row.Requester, observation.Requester) ||
				!reflect.DeepEqual(row.Correlation, observation.Correlation) || !reflect.DeepEqual(row.Labels, observation.Labels) {
				t.Fatal("privacy list changed terminal identity or metadata")
			}
		}
	}
	if !found {
		t.Fatal("privacy list omitted the terminal execution")
	}
	show := support.FakeInputs(ctx, []string{"you", "--remote", "--server", fixture.baseURL, "worker-sessions", "show", "--worker-session-id", observation.WorkerSessionId})
	show.Input.Env, show.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
	if err := fixture.process.Execute(show.Input); err != nil {
		t.Fatal("human privacy show failed")
	}
	assertRequesterTokenAbsent(t, token, show.Stdout()+show.Stderr())
	if !strings.Contains(show.Stdout(), observation.WorkerSessionId) || !strings.Contains(show.Stdout(), string(observation.State)) {
		t.Fatal("human privacy show lost terminal identity")
	}
}

func awaitRequesterPrivacyList(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, ctx context.Context, token string) string {
	t.Helper()
	// Fleet history samples live owners before the durable catalog. A parallel
	// peer admitted between those reads can temporarily lack an owner in that
	// snapshot. Await a complete projection, without accepting other failures.
	body, err := support.WaitForObservation(60*time.Second, func() (string, error) {
		listed := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "list", "--history", "all")
		listErr := fixture.process.Execute(listed.Input)
		diagnostic := strings.ReplaceAll(listed.Stdout()+listed.Stderr(), token, "[REDACTED]")
		if listErr != nil {
			safeError := strings.ReplaceAll(listErr.Error(), token, "[REDACTED]")
			var failure *workersessionscli.CLIError
			if ctx.Err() != nil || !errors.As(listErr, &failure) || failure.Code != "PROJECTION_UNAVAILABLE" {
				t.Fatalf("privacy list failed: %s; stdout/stderr: %s", safeError, diagnostic)
			}
			assertRequesterTokenAbsent(t, token, listed.Stdout()+listed.Stderr())
			t.Logf("privacy list projection pending: %s; stdout/stderr: %s", safeError, diagnostic)
			return diagnostic, listErr
		}
		assertRequesterTokenAbsent(t, token, listed.Stdout()+listed.Stderr())
		return listed.Stdout(), nil
	}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("privacy list readiness failed: %s", strings.ReplaceAll(err.Error(), token, "[REDACTED]"))
	}
	return body
}

func assertRequesterTokenAbsent(t *testing.T, token, body string) {
	t.Helper()
	if token == "" || strings.Contains(body, token) {
		t.Fatal("public output exposed the admitted execution credential or no credential was tested")
	}
}

// Public archived reads prove capture completeness and credential exclusion.
// Private continuation-recipe envelopes are verified by their owning serializer
// in TestRestartRecipePersistsImmutableDetachedInputAcrossReopen.
func assertRequesterDurableTokenPrivacy(t *testing.T, fixture *invokeContinuePackageFixture, ctx context.Context, id, token string) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
	if status != http.StatusOK {
		t.Fatal("archived Worker logs unavailable")
	}
	assertRequesterTokenAbsent(t, token, body)
	var logs api.WorkerSessionLogPage
	if err := json.Unmarshal([]byte(body), &logs); err != nil || logs.WorkerSessionId != id || string(logs.Health) != "COMPLETE" || len(logs.Events) < 2 {
		t.Fatal("archived Worker logs omitted complete execution evidence")
	}
	terminal := false
	for _, event := range logs.Events {
		if event.WorkerSessionId != id {
			t.Fatal("archived Worker logs included another execution")
		}
		if event.Event.Payload["kind"] == "SESSION" {
			switch event.Event.Payload["phase"] {
			case "COMPLETED", "FAILED", "CANCELED":
				terminal = true
			}
		}
	}
	if !terminal {
		t.Fatal("archived Worker logs omitted the terminal execution event")
	}
	for _, suffix := range []string{"", "/transcript"} {
		status, body = t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+suffix, nil)
		if status != http.StatusOK {
			t.Fatal("archived Worker observation or transcript unavailable")
		}
		assertRequesterTokenAbsent(t, token, body)
	}
}

func assertRecordingDirectTranscript(t *testing.T, cliBody, body, name string) {
	t.Helper()
	var cli, httpResult api.WorkerSessionTranscriptResponse
	if err := json.Unmarshal([]byte(cliBody), &cli); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(body), &httpResult); err != nil {
		t.Fatal(err)
	}
	wantEntries := 1
	if name == "recording-equal" {
		wantEntries = 2
	}
	if !reflect.DeepEqual(cli, httpResult) || len(cli.Entries) != wantEntries || cli.Entries[0].Text == nil || *cli.Entries[0].Text != "DIRECT_CAPTURE_BETA" {
		t.Fatalf("transcript parity/content = %+v / %+v", cli, httpResult)
	}
	for i, entry := range cli.Entries {
		if entry.Order != i+1 || entry.Text == nil || *entry.Text != "DIRECT_CAPTURE_BETA" || entry.Timestamp == nil {
			t.Fatalf("distinct ordered messages lost: %+v", cli.Entries)
		}
	}
}

// FC2 crosses actual Factory admission and the agent loop using the shared
// process. The command gate lets the observer read the first committed item.
func TestRecordingContentFactoryResultAppearsOnce(t *testing.T) {
	for _, name := range []string{"recording-factory", "recording-factory-retry"} {
		t.Run(name, func(t *testing.T) { t.Parallel(); runRecordingContentFactoryResult(t, name) })
	}
}

func runRecordingContentFactoryResult(t *testing.T, name string) {
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, name)
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner, gated := scenario.providerRunner.(*t7GatedProviderRunner)
	if gated {
		defer t7ReleaseAndJoin(t, ctx, runner)()
	}
	writeRecordingContentAgentFactory(t, scenario.workingDirectory)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, scenario.workingDirectory)
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	item := support.SubmitSessionWorkAt(t, fixture.baseURL, opened.Session.Id, api.SubmitWorkRequest{
		WorkTypeName: "task", Payload: "FACTORY_CAPTURE_ALPHA COMPLETE",
	})
	if item.WorkId == nil {
		t.Fatal("Factory Work has no identity")
	}
	if gated {
		t19AwaitSignal(t, ctx, runner.started, "Factory content")
	}
	if !gated {
		support.WaitForSessionTerminalStatus(t, fixture.baseURL, opened.Session.Id, 30*time.Second)
	}
	rows := support.GetJSON[api.ListWorkerSessionsResponse](t, fixture.baseURL+"/factory-sessions/"+opened.Session.Id+"/worker-sessions?workId="+*item.WorkId)
	if len(rows.Sessions) != 1 {
		t.Fatalf("Factory attempts = %+v", rows)
	}
	id := rows.Sessions[0].WorkerSessionId
	_, err := support.WaitForObservation(60*time.Second, func() (string, error) {
		_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
		return body, nil
	}, func(body string) bool { return strings.Contains(body, "FACTORY_CAPTURE_ALPHA COMPLETE") })
	if err != nil {
		t.Fatal(err)
	}
	if gated {
		close(runner.release)
	}
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, opened.Session.Id, 30*time.Second)
	read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--view", "transcript", "--worker-session-id", id)
	if err := fixture.process.Execute(read.Input); err != nil {
		t.Fatalf("Factory transcript: %v %s", err, read.Stderr())
	}
	assertRecordingFactoryTranscript(t, read.Stdout(), fixture.baseURL, id, gated)
	assertRecordingMessageProvenance(t, recordingContentPages(t, ctx, fixture.baseURL, id), false)
	if !gated && scenario.providerRunner.CallCount() != 2 {
		t.Fatal("retry witness did not execute two provider turns")
	}
}

func writeRecordingContentAgentFactory(t *testing.T, directory string) {
	t.Helper()
	t7WriteFactorySibling(t, directory)
	// FC2 requires the Factory agent loop, which is selected by both kinds.
	for path, replacements := range map[string][2]string{
		filepath.Join("workers", "worker", "AGENTS.md"):       {"MODEL_WORKER", "AGENT_WORKER"},
		filepath.Join("workstations", "process", "AGENTS.md"): {"MODEL_WORKSTATION", "AGENT_RUN"},
	} {
		file := filepath.Join(directory, path)
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(strings.ReplaceAll(string(body), replacements[0], replacements[1])), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertRecordingFactoryTranscript(t *testing.T, body, baseURL, id string, gated bool) {
	t.Helper()
	var transcript api.WorkerSessionTranscriptResponse
	if err := json.Unmarshal([]byte(body), &transcript); err != nil {
		t.Fatal(err)
	}
	httpTranscript := support.GetJSON[api.WorkerSessionTranscriptResponse](t, baseURL+"/worker-sessions/"+id+"/transcript")
	wantEntries := 1
	if !gated {
		wantEntries = 2
	}
	if !reflect.DeepEqual(transcript, httpTranscript) || len(transcript.Entries) != wantEntries {
		t.Fatalf("Factory result identity/parity: %+v / %+v", transcript, httpTranscript)
	}
	for i, entry := range transcript.Entries {
		if entry.Order != i+1 || entry.Text == nil || *entry.Text != "FACTORY_CAPTURE_ALPHA COMPLETE" {
			t.Fatalf("equal text from separate provider turns collapsed: %+v", transcript)
		}
	}
}

// A failed inference turn still delivered its prefix through the command edge.
// Both physical commands reuse the native item ID; the loop must scope it.
type recordingStreamingRetryRunner struct {
	*invokeContinueResettableProviderCommandRunner
}

func (r *recordingStreamingRetryRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	result, err := r.Run(ctx, request)
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, result.Stdout)
	}
	return result, err
}

func recordingContentPages(t *testing.T, ctx context.Context, baseURL, id string) api.WorkerSessionLogPage {
	t.Helper()
	endpoint := baseURL + "/worker-sessions/" + id + "/logs?limit=2"
	var result api.WorkerSessionLogPage
	for pages := 0; pages < 100; pages++ {
		status, body := t7HTTP(t, ctx, http.MethodGet, endpoint, nil)
		var page api.WorkerSessionLogPage
		if err := json.Unmarshal([]byte(body), &page); err != nil || status != http.StatusOK || page.Health != "COMPLETE" {
			t.Fatalf("committed logs: status=%d error=%v body=%s", status, err, body)
		}
		if pages == 0 {
			result = page
			result.Events = nil
		} else if page.CommittedPosition != result.CommittedPosition || page.RecordingGenerationId != result.RecordingGenerationId {
			t.Fatal("capture watermark changed while paging completed recording")
		}
		for _, event := range page.Events {
			if event.Event.Position != int64(len(result.Events)+1) || event.Event.CapturedAt == nil || event.Event.SourceEventId == "" {
				t.Fatalf("committed order/identity/time: %+v", event)
			}
			result.Events = append(result.Events, event)
		}
		if page.NextToken == nil {
			if int64(len(result.Events)) != page.CommittedPosition {
				t.Fatal("paging lost committed records")
			}
			result.NextToken = nil
			return result
		}
		endpoint = baseURL + "/worker-sessions/" + id + "/logs?limit=2&nextToken=" + *page.NextToken
	}
	t.Fatal("completed recording did not finish paging")
	return result
}

func assertRecordingMessageProvenance(t *testing.T, page api.WorkerSessionLogPage, finalOnly bool) {
	t.Helper()
	var native, synthesized []workers.Draft
	for _, event := range page.Events {
		encoded, err := json.Marshal(event.Event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		var draft workers.Draft
		if err := json.Unmarshal(encoded, &draft); err != nil {
			t.Fatal(err)
		}
		if draft.Kind != workers.KindMessage {
			continue
		}
		switch draft.Provenance.Delivery {
		case workers.DeliveryNativeStream:
			native = append(native, draft)
		case workers.DeliverySynthesized:
			synthesized = append(synthesized, draft)
		}
	}
	if (finalOnly && len(native) != 1) || (!finalOnly && len(native) < 2) || len(synthesized) != 0 {
		t.Fatalf("stream/final provenance: native=%+v synthesized=%+v", native, synthesized)
	}
	// Native completions are authoritative for direct and agent-loop turns.
	// Result-only synthesized FINAL_ONLY fallback is covered by FactoryFinalCapture.
	assertRecordingCompletedNativeIdentity(t, native[len(native)-1])
}

func assertRecordingCompletedNativeIdentity(t *testing.T, last workers.Draft) {
	t.Helper()
	if last.Phase != workers.PhaseCompleted || last.ItemID == "" || last.DispatchID == "" || last.Provenance.Fidelity != workers.FidelityNormalized {
		t.Fatalf("native stream lost completed message identity/provenance: %+v", last)
	}
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
	runRecordingContentSecrets(t, "t7-secrets")
}

func TestRecordingContentFailedPrefixRedactsSecrets(t *testing.T) {
	t.Parallel()
	runRecordingContentSecrets(t, "recording-failure")
}

func runRecordingContentSecrets(t *testing.T, name string) {
	t.Helper()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, name)
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
	// These prompt values are explicitly classified secrets in this cell.
	// Ordinary prompts are exercised separately by RecordingContent.
	execution["envVars"] = map[string]string{"T7_API_TOKEN": t7SecretToken, "T7_PROMPT_SECRET": t7SecretPrompt, "T7_SYSTEM_SECRET": t7SecretSystem}
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
	}, func(body string) bool {
		return strings.Contains(body, "t7-private-progress") && strings.Contains(body, "command_execution")
	})
	if err != nil {
		t.Fatalf("private capture: %v: %s", err, body)
	}
	t7AssertSecretsAbsent(t, body)
	if !strings.Contains(body, "redacted") {
		t.Fatalf("progress did not retain redaction marker: %s", body)
	}
	close(runner.release)
	join := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(join.Input); (err != nil) != (name == "recording-failure") {
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
	if name == "recording-failure" {
		assertRecordingFailedPrefix(t, ctx, fixture, scenario, id, observation)
	}
}

func assertRecordingFailedPrefix(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id, observation string) {
	t.Helper()
	var summary api.WorkerSessionObservation
	if err := json.Unmarshal([]byte(observation), &summary); err != nil || summary.State != "FAILED" || summary.Failure == nil {
		t.Fatalf("failed prefix fabricated completion: error=%v summary=%s", err, observation)
	}
	page := recordingContentPages(t, ctx, fixture.baseURL, id)
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	t7AssertSecretsAbsent(t, string(encoded))
	if !strings.Contains(string(encoded), "ordinary prefix") || !strings.Contains(string(encoded), "t7-private-progress") || !strings.Contains(string(encoded), "command_execution") {
		t.Fatal("failed attempt lost ordinary captured neighbors")
	}
	read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--view", "transcript", "--worker-session-id", id)
	if err := fixture.process.Execute(read.Input); err != nil {
		t.Fatalf("failed prefix transcript: %v %s", err, read.Stderr())
	}
	httpTranscript := support.GetJSON[api.WorkerSessionTranscriptResponse](t, fixture.baseURL+"/worker-sessions/"+id+"/transcript")
	var transcript api.WorkerSessionTranscriptResponse
	if err := json.Unmarshal([]byte(read.Stdout()), &transcript); err != nil || !reflect.DeepEqual(transcript, httpTranscript) || transcript.State != "FAILED" {
		t.Fatalf("failed transcript parity: error=%v transcript=%s", err, read.Stdout())
	}
	t7AssertSecretsAbsent(t, read.Stdout())
	if !strings.Contains(read.Stdout(), "ordinary prefix") || !strings.Contains(read.Stdout(), "t7-private-progress") || !strings.Contains(read.Stdout(), "command_execution") {
		t.Fatal("failed transcript lost committed prefix")
	}
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

// Native-shaped facts use the command-runner edge on the shared root process;
// this protects captured public values without claiming real provider emission.
func capturedCodexToolOutput() []byte {
	return []byte(`{"type":"thread.started","thread_id":"captured-tools-thread"}
{"type":"item.started","item":{"id":"success","type":"command_execution","command":"private-command","status":"in_progress"}}
{"type":"item.completed","item":{"id":"success","type":"command_execution","aggregated_output":"private-output","exit_code":0,"status":"completed"}}
{"type":"item.completed","item":{"id":"failure","type":"command_execution","aggregated_output":"private-denial","exit_code":1,"status":"failed"}}
{"type":"item.started","item":{"id":"mcp","type":"mcp_tool_call","server":"docs","tool":"lookup","arguments":{"query":"visible"},"status":"in_progress"}}
{"type":"item.completed","item":{"id":"mcp","type":"mcp_tool_call","result":{"value":"visible"},"status":"completed"}}
{"type":"item.completed","item":{"id":"collab","type":"collab_tool_call"}}
{"type":"item.completed","item":{"id":"answer","type":"agent_message","text":"captured tools COMPLETE"}}
{"type":"turn.completed","usage":{"input_tokens":5,"output_tokens":2}}
`)
}

func TestCapturedCodexToolPublicFacts(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "captured-codex-tools")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	id := scenarioScopedID(scenario, "captured-tools")
	path := requesterExecutionPath(t, scenario, id)
	invoke := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatal("tool fixture invocation failed")
	}
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, scenario.homeDirectory, scenario.workingDirectory, id, "captured-tools-thread")
	for _, view := range []string{"logs", "transcript"} {
		read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", id, "--view", view)
		if err := fixture.process.Execute(read.Input); err != nil {
			t.Fatal("tool capture read failed")
		}
		status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/"+view, nil)
		var cli, httpValue any
		if view == "logs" {
			cli, httpValue = &api.WorkerSessionLogPage{}, &api.WorkerSessionLogPage{}
		} else {
			cli, httpValue = &api.WorkerSessionTranscriptResponse{}, &api.WorkerSessionTranscriptResponse{}
		}
		// Compare the public contract values; CLI re-encodes HTTP timestamps.
		if status != http.StatusOK || json.Unmarshal([]byte(read.Stdout()), cli) != nil || json.Unmarshal([]byte(body), httpValue) != nil || !reflect.DeepEqual(cli, httpValue) {
			t.Fatalf("CLI/HTTP tool facts differ for %s (HTTP %d)", view, status)
		}
		for _, private := range []string{"private-command", "private-output", "private-denial"} {
			if strings.Contains(body, private) {
				t.Fatal("sensitive tool summary leaked")
			}
		}
		if view == "logs" {
			var logs api.WorkerSessionLogPage
			if json.Unmarshal([]byte(body), &logs) != nil {
				t.Fatal("invalid captured logs")
			}
			count := 0
			for _, record := range logs.Events {
				if record.Event.Payload["kind"] == "TOOL" {
					count++
				}
			}
			if count != 6 {
				t.Fatalf("emitted tool lifecycle count = %d, want 6", count)
			}
		} else {
			assertCapturedCodexToolTranscript(t, body)
		}
	}
	if scenario.providerRunner.CallCount() != 1 {
		t.Fatal("captured reads reran provider")
	}
}

func assertCapturedCodexToolTranscript(t *testing.T, body string) {
	t.Helper()
	var transcript api.WorkerSessionTranscriptResponse
	if json.Unmarshal([]byte(body), &transcript) != nil || len(transcript.Entries) != 8 {
		t.Fatal("tool transcript lost or duplicated facts")
	}
	names := map[string]string{"success": "command_execution", "failure": "command_execution", "mcp": "mcp_tool_call:docs/lookup", "collab": "collab_tool_call"}
	for _, entry := range transcript.Entries {
		if entry.Type == "tool_call" {
			if entry.CallId == nil || entry.Name == nil || *entry.Name != names[*entry.CallId] {
				if entry.CallId == nil || entry.Name == nil {
					t.Fatal("native tool identity/name unavailable")
				}
				t.Fatalf("native tool identity/name lost: id=%q name=%q", *entry.CallId, *entry.Name)
			}
			if *entry.CallId == "mcp" && (entry.Arguments == nil || !strings.Contains(*entry.Arguments, "visible")) {
				t.Fatal("opening arguments lost on completion")
			}
		}
		if entry.Type == "tool_output" && entry.CallId != nil && (*entry.CallId == "success" || *entry.CallId == "failure") {
			var result map[string]any
			if entry.Output == nil || json.Unmarshal([]byte(*entry.Output), &result) != nil {
				t.Fatal("tool result is not valid JSON")
			}
			code, status := float64(0), "completed"
			if *entry.CallId == "failure" {
				code, status = 1, "failed"
			}
			if result["exit_code"] != code || result["status"] != status || entry.Status == nil || *entry.Status != status {
				t.Fatal("tool outcome lost or misreported")
			}
		}
	}
}
