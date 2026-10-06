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
		{"local", "document"}, {"local", "overrides"}, {"local", "positional"}, {"local", "stdin"},
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
		!strings.Contains(commandArgs, `model_reasoning_effort="`+effort+`"`) ||
		!strings.Contains(commandArgs, "developer_instructions="+string(encodedSystem)) ||
		string(command.Stdin) != user || command.WorkDir != workingDirectory {
		t.Fatalf("effective provider command = %#v", command)
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
