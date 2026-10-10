package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const t7ObservationReport = "T7 controlled output COMPLETE\n" +
	"## Inspection status\nInspected: yes\n" +
	"## Chronological events\n00:00 synthetic fixture\n" +
	"## Temporal or transient defects\nNone observed\n" +
	"## Audio content and defects\nAudio content: silence\nNone observed\n" +
	"## Observed speech\nNone observed\n" +
	"## Overall recommendation\nRecommendation: pass\n"

// F6-05 uses the assembled Providers policy through each supported Start entry.
// Direct invocation deliberately has no Factory origin; each leaf owns its
// route, profile and identities in the package's existing shared process.
func TestT7UnsupportedEffortRejectsBeforeAdmission(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "rest/startWorkerSession")
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, mode := range []string{"local", "remote", "http"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			scenario := fixture.scenario(t, "t7-"+mode)
			defer scenario.close(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			id := scenarioScopedID(scenario, "t7-unsupported-"+mode)
			document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
				requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
				workingDirectory: scenario.workingDirectory, userMessage: "controlled prompt",
			})
			execution := document["execution"].(map[string]any)
			for _, key := range []string{"runnerId", "executorProvider", "modelProvider"} {
				execution[key] = "claude"
			}
			execution["reasoningEffort"] = "minimal"
			if mode == "http" {
				status, body := t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/worker-sessions", document)
				if status != http.StatusBadRequest || !strings.Contains(body, "BAD_REQUEST") {
					t.Fatalf("Start HTTP = %d %s", status, body)
				}
			} else {
				path := filepath.Join(scenario.workingDirectory, "unsupported.json")
				writeInvokeContinueJSON(t, path, document)
				args := []string{"you", "--json"}
				if mode == "remote" {
					args = append(args, "--remote", "--server", fixture.baseURL)
				}
				args = append(args, "worker-sessions", "invoke", "--execution", path)
				inputs := support.FakeInputs(ctx, args)
				inputs.Input.Env = scenario.environment()
				inputs.Input.WorkingDirectory = scenario.workingDirectory
				if err := fixture.process.Execute(inputs.Input); err == nil {
					t.Fatal("unsupported effort was accepted")
				}
				assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_INVOKE_INVALID")
				if inputs.Stdout() != "" {
					t.Fatalf("rejection stdout = %s", inputs.Stdout())
				}
			}
			if scenario.providerRunner.CallCount() != 0 {
				t.Fatal("unsupported settings reached provider command")
			}
			status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id, nil)
			if status != http.StatusNotFound {
				t.Fatalf("rejected identity was opened: %d %s", status, body)
			}
		})
	}
}

// F6-01 observes effective settings at the actual command adapter boundary.
func TestT7InvokeOverridesReachProviderCommandAndCapturedSession(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "rest/startWorkerSession", "rest/getWorkerSessionObservationByWorkerSessionId", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, cell := range []struct{ mode, source string }{
		{"local", "document"}, {"local", "overrides"}, {"local", "positional"}, {"local", "stdin"}, {"local", "document-stdin"},
		{"remote", "document"}, {"remote", "overrides"}, {"http", "document"},
	} {
		t.Run(cell.mode+"-"+cell.source, func(t *testing.T) {
			t.Parallel()
			t7AssertSettings(t, fixture, cell.mode, cell.source)
		})
	}
}

func t7AssertSettings(t *testing.T, fixture *invokeContinuePackageFixture, mode, source string) {
	t.Helper()
	scenario := fixture.scenario(t, "t7-settings-"+mode+"-"+source)
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	id := scenarioScopedID(scenario, "t7-settings-session")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
		workingDirectory: scenario.workingDirectory, userMessage: "document prompt",
	})
	execution := document["execution"].(map[string]any)
	execution["reasoningEffort"] = "low"
	execution["systemPrompt"] = "document system"
	execution["outputContract"] = "markdown-observation-report/v1"
	execution["dispatch"].(map[string]any)["execution"] = map[string]any{"requestId": id + "-request"}
	execution["envVars"] = map[string]string{"T7_SYNTHETIC_SETTING": "controlled-value"}
	model, effort, system, user := "functional-model", "low", "document system", "document prompt"
	args := []string{"you", "--json"}
	if mode != "local" {
		args = append(args, "--remote", "--server", fixture.baseURL)
	}
	path := filepath.Join(scenario.workingDirectory, "settings.json")
	args = append(args, "worker-sessions", "invoke", "--execution", path)
	switch source {
	case "overrides":
		model, effort, system, user = "opaque-override-model", "high", "override system\nquoted \"instruction\"", "override user"
		args = append(args, "--provider", "codex", "--model", model, "--reasoning-effort", "HIGH", "--system-prompt", system, "--user-message", user, "ignored positional prompt")
	case "positional":
		user = "positional prompt"
		args = append(args, "positional", "prompt")
	case "stdin":
		delete(execution, "userMessage")
		user = "stdin prompt"
	case "document-stdin":
		delete(execution, "reasoningEffort")
		effort = ""
		args[len(args)-1] = "-"
	}
	writeInvokeContinueJSON(t, path, document)
	if mode == "http" {
		t7AssertHTTPReplay(t, ctx, fixture.baseURL, id, document)
		// Replaying the exact HTTP tuple through a separate CLI observer joins
		// its terminal result without opening a second execution.
	}
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	stdinIsTTY := false
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("ignored stdin prompt")
	if source == "stdin" {
		inputs.Input.Stdin = strings.NewReader(user)
	}
	if source == "document-stdin" {
		inputs.Input.Stdin = t7DocumentStdin(t, document)
	}
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("invoke: %v\n%s", err, inputs.Stderr())
	}
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, inputs.Stdout(), &result)
	if !result.Accepted || result.WorkerSessionID != id || result.State != "COMPLETED" || !strings.Contains(result.Output, "T7 controlled output") {
		t.Fatalf("invoke result = %#v", result)
	}
	requests := scenario.providerRunner.Requests()
	if len(requests) != 1 {
		t.Fatalf("provider calls = %d", len(requests))
	}
	t7AssertProviderCommand(t, requests[0], scenario.workingDirectory, model, effort, system, user)
	t7AssertCapturedSettings(t, ctx, fixture.baseURL, id, model, effort)
}

func t7DocumentStdin(t *testing.T, document map[string]any) io.Reader {
	t.Helper()
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(encoded)
}

func t7AssertCapturedSettings(t *testing.T, ctx context.Context, baseURL, id, model, effort string) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+id, nil)
	var observation struct {
		WorkerSessionID string `json:"workerSessionId"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
		Provider        string `json:"provider"`
		State           string `json:"state"`
		Direct          bool   `json:"direct"`
	}
	if err := json.Unmarshal([]byte(body), &observation); err != nil {
		t.Fatalf("observation HTTP %d: %s: %v", status, body, err)
	}
	if status != http.StatusOK || observation.WorkerSessionID != id || observation.Model != model ||
		observation.ReasoningEffort != effort || observation.Provider != "codex" || !observation.Direct || observation.State != "COMPLETED" {
		t.Fatalf("captured settings HTTP %d: %s", status, body)
	}
	t7AssertCapturedLogs(t, ctx, baseURL, id)
}

func t7AssertHTTPReplay(t *testing.T, ctx context.Context, baseURL, id string, document map[string]any) {
	t.Helper()
	execution := document["execution"].(map[string]any)
	// Concurrent callers own the same immutable tuple. Each gets the same
	// identity; the command-edge assertion below proves one execution.
	t.Run("concurrent-replay", func(t *testing.T) {
		for range 4 {
			t.Run("caller", func(t *testing.T) {
				t.Parallel()
				status, body := t7HTTP(t, ctx, http.MethodPost, baseURL+"/worker-sessions", document)
				if status != http.StatusAccepted || !strings.Contains(body, id) {
					t.Fatalf("Start HTTP = %d %s", status, body)
				}
			})
		}
	})
	for _, field := range []string{"model", "userMessage"} {
		original := execution[field]
		execution[field] = "changed immutable input"
		status, body := t7HTTP(t, ctx, http.MethodPost, baseURL+"/worker-sessions", document)
		execution[field] = original
		if status != http.StatusConflict {
			t.Fatalf("changed %s replay HTTP = %d %s", field, status, body)
		}
	}
}

func t7AssertProviderCommand(t *testing.T, command platformprocess.CommandRequest, workingDirectory, model, effort, system, user string) {
	t.Helper()
	commandArgs := strings.Join(command.Args, " ")
	encodedSystem, err := json.Marshal(system)
	if err != nil {
		t.Fatal(err)
	}
	if command.Command != "codex" || !strings.Contains(commandArgs, "--model "+model) ||
		!strings.Contains(commandArgs, "developer_instructions="+string(encodedSystem)) ||
		string(command.Stdin) != user || command.WorkDir != workingDirectory {
		t.Fatalf("effective provider command = %#v", command)
	}
	if effort == "" {
		if strings.Contains(commandArgs, "model_reasoning_effort=") {
			t.Fatalf("omitted effort overrides provider default: %#v", command.Args)
		}
	} else if !strings.Contains(commandArgs, `model_reasoning_effort="`+effort+`"`) {
		t.Fatalf("effective reasoning effort = %#v", command.Args)
	}
	if !strings.Contains(strings.Join(command.Env, "\n"), "T7_SYNTHETIC_SETTING=controlled-value") {
		t.Fatalf("provider env = %#v", command.Env)
	}
}

func t7AssertCapturedLogs(t *testing.T, ctx context.Context, baseURL, id string) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+id+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(body, "T7 controlled output COMPLETE") {
		t.Fatalf("captured output HTTP %d: %s", status, body)
	}
	var logs struct {
		WorkerSessionID string `json:"workerSessionId"`
		Events          []struct {
			Event struct {
				Position   int64  `json:"position"`
				CapturedAt string `json:"capturedAt"`
			} `json:"event"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(body), &logs); err != nil || logs.WorkerSessionID != id || len(logs.Events) == 0 {
		t.Fatalf("captured logs = %s, error %v", body, err)
	}
	var previous int64
	for _, entry := range logs.Events {
		if entry.Event.Position <= previous || entry.Event.CapturedAt == "" {
			t.Fatalf("unordered or uncommitted capture = %s", body)
		}
		previous = entry.Event.Position
	}
}

func t7HTTP(t testing.TB, ctx context.Context, method, endpoint string, document any) (int, string) {
	t.Helper()
	var data []byte
	if document != nil {
		var err error
		data, err = json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(body)
}

// F6-07 distinguishes rejected input from an admitted dependency failure.
// The real Codex adapter classifies the controlled timeout; Worker Sessions
// owns the bounded retry attempts and keeps their progress on one identity.
func TestT7AdmittedTimeoutRetainsHistoryAndRetryBudget(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "rest/getWorkerSessionObservationByWorkerSessionId", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, cell := range []struct {
		name     string
		attempts int
	}{{"one", 1}, {"two", 2}} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			scenario := fixture.scenario(t, "t7-failure-"+cell.name)
			defer scenario.close(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			id := scenarioScopedID(scenario, "t7-failure-session")
			document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
				requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
				workingDirectory: scenario.workingDirectory, userMessage: "controlled timeout prompt",
			})
			document["retry"] = map[string]any{"maxAttempts": cell.attempts}
			path := filepath.Join(scenario.workingDirectory, "timeout.json")
			writeInvokeContinueJSON(t, path, document)
			inputs := support.FakeInputs(ctx, []string{"you", "--json", "--remote", "--server", fixture.baseURL, "worker-sessions", "invoke", "--execution", path})
			inputs.Input.Env = scenario.environment()
			inputs.Input.WorkingDirectory = scenario.workingDirectory
			if err := fixture.process.Execute(inputs.Input); err == nil {
				status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id, nil)
				t.Fatalf("timed out provider reported successful invoke: %s, calls %d, observation %d %s", inputs.Stdout(), scenario.providerRunner.CallCount(), status, body)
			}
			assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_FAILED")
			if scenario.providerRunner.CallCount() != cell.attempts*3 {
				t.Fatalf("provider calls = %d, supervised budget = %d (three calls per attempt)", scenario.providerRunner.CallCount(), cell.attempts)
			}
			status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id, nil)
			var observation struct {
				WorkerSessionID string `json:"workerSessionId"`
				State           string `json:"state"`
				Failure         struct {
					ProviderFailureKind string `json:"providerFailureKind"`
				} `json:"failure"`
			}
			if err := json.Unmarshal([]byte(body), &observation); err != nil {
				t.Fatal(err)
			}
			if status != http.StatusOK || observation.WorkerSessionID != id || observation.State != "FAILED" || observation.Failure.ProviderFailureKind != "timeout" {
				t.Fatalf("admitted failure HTTP %d: %s", status, body)
			}
			status, body = t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
			if status != http.StatusOK || !strings.Contains(body, "T7 retained progress before timeout") {
				t.Fatalf("failed captured prefix HTTP %d: %s", status, body)
			}
			t7AssertRetryHistory(t, body, cell.attempts)
		})
	}
}

func t7AssertRetryHistory(t *testing.T, body string, attempts int) {
	t.Helper()
	var logs struct {
		Health string `json:"health"`
		Events []struct {
			Event struct {
				Position int64 `json:"position"`
				Payload  struct {
					Kind    string `json:"kind"`
					Payload struct {
						AttemptID string `json:"attemptId"`
						Attempt   int    `json:"attempt"`
						Status    string `json:"status"`
					} `json:"payload"`
				} `json:"payload"`
			} `json:"event"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(body), &logs); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	var previous int64
	for _, entry := range logs.Events {
		if entry.Event.Position <= previous {
			t.Fatalf("retry history is unordered: %s", body)
		}
		previous = entry.Event.Position
		payload := entry.Event.Payload.Payload
		if payload.AttemptID == "" {
			continue
		}
		if seen[payload.AttemptID] || payload.Attempt != len(seen)+1 {
			t.Fatalf("retry identity repeated or attempt skipped: %s", body)
		}
		seen[payload.AttemptID] = true
	}
	if len(seen) != attempts || logs.Health != "COMPLETE" || len(logs.Events) == 0 {
		t.Fatalf("retry history budget/completeness: %s", body)
	}
	terminal := logs.Events[len(logs.Events)-1].Event.Payload.Payload.Status
	if terminal != "FAILED" {
		t.Fatalf("last captured state = %s, want FAILED", terminal)
	}
}

// HTTP parity: a caller learned at the controlled native edge opens and invokes
// a JavaScript Factory through the same public process used by CLI scenarios.
func TestRequesterFactoryHTTPCaller(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	parent, child := fixture.scenario(t, "requester-http-parent"), fixture.scenario(t, "requester-http-child")
	defer parent.close(t)
	defer child.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := parent.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	parentID := scenarioScopedID(parent, "http-parent")
	start := t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "invoke", "--execution", requesterExecutionPath(t, parent, parentID), "--async")
	if err := fixture.process.Execute(start.Input); err != nil {
		t.Fatal(err)
	}
	t19AwaitSignal(t, ctx, runner.started, "HTTP caller running")
	token := requesterSourceToken(t, runner, parentID)
	writeInvokeContinueJSON(t, filepath.Join(child.workingDirectory, "factory.json"), map[string]any{
		"name": "requester-http-factory", "orchestrator": map[string]any{"kind": "JAVASCRIPT", "javascript": map[string]any{"inlineSource": map[string]any{"encoding": "utf-8", "inline": `return (async function () { await agent.run({prompt: "HTTP requester child", executorProvider: "codex", modelProvider: "codex"}); return "factory caller finished"; })();`}}},
	})
	status, body := requesterFactoryHTTP(t, ctx, fixture.baseURL+"/factory-sessions", map[string]any{"folderPath": child.workingDirectory}, parentID, token)
	if status != http.StatusOK {
		t.Fatalf("caller open: %d %s", status, body)
	}
	var opened api.OpenFactorySessionResponse
	if err := json.Unmarshal([]byte(body), &opened); err != nil || opened.Session == nil {
		t.Fatal("open did not return a live session")
	}
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	status, body = requesterFactoryHTTP(t, ctx, fixture.baseURL+"/factory-sessions/"+opened.Session.Id+"/invocations", map[string]any{"requestId": scenarioScopedID(child, "http-invoke"), "sourceKind": "text", "content": []map[string]any{{"type": "TEXT", "text": "HTTP invocation input"}}}, parentID, token)
	if status != http.StatusOK {
		t.Fatalf("caller invocation: %d %s", status, body)
	}
	var result api.InvocationResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil || result.Status != api.InvocationTerminalStatusCompleted || result.SessionId == nil {
		t.Fatalf("Factory result not completed: %s", body)
	}
	if strings.Contains(body, token) || child.providerRunner.CallCount() != 1 {
		t.Fatal("Factory invocation leaked caller or duplicated provider")
	}
	assertRequesterFactoryHTTPChild(t, fixture, child, ctx, result, parentID, token)
	assertRequesterFactoryHTTPRefusals(t, fixture, parent, child, ctx, opened.Session.Id, parentID, token)
	stop := t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "cancel", parentID)
	if err := fixture.process.Execute(stop.Input); err != nil {
		t.Fatal(err)
	}
	t19AwaitSignal(t, ctx, runner.stopped, "Factory caller ended")
	assertRequesterFactoryHTTPEnded(t, fixture, child, ctx, opened.Session.Id, parentID, token)

	functionalevidence.Covers(t, "rest/openFactorySession", "rest/invokeFactorySessionBySessionId", "rest/startDurableFactorySessionAsync", "rest/startDurableFactorySessionSync")
}

func requesterFactoryHTTP(t testing.TB, ctx context.Context, endpoint string, document any, id, token string) (int, string) {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if id != "" {
		req.Header.Set("X-You-Worker-Session-Id", id)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(body)
}

func assertRequesterFactoryHTTPRefusals(t *testing.T, fixture *invokeContinuePackageFixture, parent, child *invokeContinueScenario, ctx context.Context, sessionID, callerID, token string) {
	t.Helper()
	for _, pair := range []struct{ id, token string }{{callerID, strings.Repeat("A", 43)}, {callerID, ""}, {"foreign-worker", token}} {
		for _, route := range []string{"", "/async", "/sync", "/" + sessionID + "/invocations"} {
			status, body := requesterFactoryHTTP(t, ctx, fixture.baseURL+"/factory-sessions"+route, map[string]any{"folderPath": child.workingDirectory, "requestId": scenarioScopedID(child, "refused"), "source": map[string]any{"kind": "INLINE_WORKFLOW", "inlineWorkflow": map[string]any{"inlineSource": map[string]any{"encoding": "utf-8", "inline": "return 1;"}}}, "args": map[string]any{}}, pair.id, pair.token)
			if status != http.StatusForbidden || !strings.Contains(body, `"code":"WORKER_SESSION_CALLER_INVALID"`) {
				t.Fatalf("Factory caller refusal %s: %d %s", route, status, body)
			}
			assertRequesterTokenAbsent(t, token, body)
		}
	}
	if child.providerRunner.CallCount() != 1 || parent.providerRunner.CallCount() != 1 {
		t.Fatal("Factory caller refusal launched or changed a provider attempt")
	}
}

func assertRequesterFactoryHTTPChild(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, result api.InvocationResponse, parentID, token string) {
	t.Helper()

	environment := requesterEnvironment(child.providerRunner.Requests()[0].Env)
	show := t7RemoteCLIInputs(child, ctx, fixture.baseURL, "show", "--session", *result.SessionId, "--worker-session-id", environment["YOU_WORKER_SESSION_ID"])
	if err := fixture.process.Execute(show.Input); err != nil {
		t.Fatal(err)
	}
	var observation api.WorkerSessionObservation
	decodeDirectWorkerSessionResult(t, show.Stdout(), &observation)
	assertRequesterSuccessorEnvironment(t, environment, observation.WorkerSessionId, parentID, token)
	if observation.Requester == nil || observation.Requester.WorkerSessionId != parentID || observation.Correlation == nil || observation.Correlation.FactorySessionId == nil || *observation.Correlation.FactorySessionId != *result.SessionId {
		t.Fatal("Factory child lost requester or actual child session correlation")
	}
	assertRequesterEndpoint(t, environment, fixture.baseURL)
	assertRequesterTokenAbsent(t, token, show.Stdout()+show.Stderr())
	assertRequesterTokenAbsent(t, environment["YOU_WORKER_SESSION_TOKEN"], show.Stdout()+show.Stderr())
}

func assertRequesterFactoryHTTPEnded(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, sessionID, callerID, token string) {
	t.Helper()
	for _, route := range []string{"", "/async", "/sync", "/" + sessionID + "/invocations"} {
		document := map[string]any{"folderPath": child.workingDirectory, "requestId": scenarioScopedID(child, "ended"), "source": map[string]any{"kind": "INLINE_WORKFLOW", "inlineWorkflow": map[string]any{"inlineSource": map[string]any{"encoding": "utf-8", "inline": "return 1;"}}}, "args": map[string]any{}}
		status, body := requesterFactoryHTTP(t, ctx, fixture.baseURL+"/factory-sessions"+route, document, callerID, token)
		if status != http.StatusForbidden || !strings.Contains(body, `"code":"WORKER_SESSION_CALLER_INVALID"`) {
			t.Fatalf("ended Factory caller %s: %d %s", route, status, body)
		}
		assertRequesterTokenAbsent(t, token, body)
	}
	if child.providerRunner.CallCount() != 1 {
		t.Fatal("ended Factory caller launched a provider")
	}
}
