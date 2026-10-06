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

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

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
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke")
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "t7-settings")
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
	execution["envVars"] = map[string]string{"T7_SYNTHETIC_SETTING": "controlled-value"}
	path := filepath.Join(scenario.workingDirectory, "settings.json")
	writeInvokeContinueJSON(t, path, document)
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", path,
		"--model", "opaque-override-model", "--reasoning-effort", "HIGH", "--system-prompt", "override system", "--user-message", "override user"})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
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
	command := requests[0]
	args := strings.Join(command.Args, " ")
	if !strings.Contains(args, "--model opaque-override-model") || !strings.Contains(args, `model_reasoning_effort="high"`) ||
		!strings.Contains(args+string(command.Stdin), "override system") || !strings.Contains(string(command.Stdin), "override user") ||
		strings.Contains(string(command.Stdin), "document prompt") || command.WorkDir != scenario.workingDirectory {
		t.Fatalf("effective provider command = %#v", command)
	}
	if !strings.Contains(strings.Join(command.Env, "\n"), "T7_SYNTHETIC_SETTING=controlled-value") {
		t.Fatalf("provider env = %#v", command.Env)
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
