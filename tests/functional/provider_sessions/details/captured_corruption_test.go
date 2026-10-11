package details

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	workerscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCapturedCredentialRedaction(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "credential-capture")
	support.ClearSeedInputs(t, dir)
	tokens := map[string]chan string{}
	cases := []struct {
		name  string
		fail  bool
		flags []string
	}{
		{"completed", false, []string{"--json"}},
		{"normal", true, nil},
		{"verbose", true, []string{"--verbose"}},
		{"debug", true, []string{"--debug"}},
		{"json", true, []string{"--json"}},
		{"output-json", true, []string{"--output", "json"}},
	}
	for _, tc := range cases {
		tokens[tc.name] = make(chan string, 1)
	}
	host := startCapturedCodexHost(t, home, dir, capturedCredentialRunner{tokens: tokens}, nil)
	for _, tc := range cases {
		// Complete profile bootstrap before concurrent calls share the process.
		clientHome := t.TempDir()
		clientEnv := []string{"HOME=" + clientHome, "USERPROFILE=" + clientHome, "VALIDATION_ENVIRONMENT=1"}
		bootstrap := support.FakeInputs(t.Context(), []string{"you", "run", "--factory", filepath.Join(dir, "missing-initialization-factory.json")})
		bootstrap.Env, bootstrap.WorkingDirectory = clientEnv, dir
		if err := host.Execute(t, bootstrap.Input); err == nil || !strings.Contains(err.Error(), "missing-initialization-factory.json") {
			t.Fatalf("bootstrap owned profile: %v", err)
		}
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := map[string]any{"requestId": "credential-request-" + tc.name, "workerSessionId": "credential-worker-" + tc.name,
				"execution": map[string]any{"workstationName": "direct", "workingDirectory": dir, "workerType": "direct-worker",
					"runnerId": "codex", "executorProvider": "codex", "modelProvider": "codex", "model": "functional-model",
					"userMessage": "private-request-input", "envVars": map[string]string{"API_KEY": "planted-api-credential", "ACCESS_TOKEN": "planted-token-credential", "CAPTURE_SCENARIO": tc.name},
					"dispatch": map[string]any{"dispatchId": "credential-attempt-" + tc.name, "workstationName": "direct", "workerType": "direct-worker"}}}
			path := filepath.Join(dir, "credential-"+tc.name+".json")
			if err := os.WriteFile(path, mustMarshalJSON(document), 0600); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"you", "worker-sessions", "invoke"}, tc.flags...)
			input := support.FakeInputs(t.Context(), append(args, "--execution", path))
			input.Env, input.WorkingDirectory = clientEnv, dir
			err := host.Execute(t, input.Input)
			if (err != nil) != tc.fail {
				t.Fatalf("failure classification changed: %v", err)
			}
			if tc.fail && strings.TrimSpace(input.Stdout()) != "" {
				t.Fatalf("failure emitted success stdout: %q", input.Stdout())
			}
			if tc.fail {
				var typed *workerscli.CLIError
				if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_FAILED" {
					t.Fatalf("typed failure lost: %v", err)
				}
			} else {
				var result struct {
					Accepted bool   `json:"accepted"`
					State    string `json:"state"`
				}
				if json.Unmarshal([]byte(input.Stdout()), &result) != nil || !result.Accepted || result.State != "COMPLETED" {
					t.Fatalf("completion did not return its typed success: %s", input.Stdout())
				}
			}
			detail := awaitCapturedCodexDetail(t, host, "credential-"+tc.name)
			encoded := string(mustMarshalJSON(detail)) + input.Stdout() + input.Stderr()
			if err != nil {
				encoded += err.Error()
			}
			workerToken := <-tokens[tc.name]
			if workerToken == "" {
				t.Fatal("controlled command received no Worker Session credential")
			}
			for _, secret := range []string{"planted-api-credential", "planted-token-credential", "private-request-input", workerToken} {
				if strings.Contains(encoded, secret) {
					t.Fatalf("public captured credential/failure leaked %s", secret)
				}
			}
			if !strings.Contains(encoded, "safe 8528a10f-6314-48ba-8230-542a4cf660f9") || !strings.Contains(encoded, "redacted") {
				t.Fatal("redaction lost useful neighboring captured content")
			}
			if !strings.Contains(encoded, "tool-safe peer["+tc.name+"]") {
				t.Fatal("redaction lost this peer's tool result")
			}
			for _, peer := range cases {
				if peer.name != tc.name && strings.Contains(encoded, "peer["+peer.name+"]") {
					t.Fatal("captured content leaked across concurrent peers")
				}
			}
		})
	}
}

type capturedCredentialRunner struct {
	tokens map[string]chan string
}

func (runner capturedCredentialRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("streaming command required")
}

func (runner capturedCredentialRunner) RunStreaming(_ context.Context, request platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	workerToken := ""
	scenario := ""
	for _, entry := range request.Env {
		if value, ok := strings.CutPrefix(entry, "YOU_WORKER_SESSION_TOKEN="); ok {
			workerToken = value
		}
		if value, ok := strings.CutPrefix(entry, "CAPTURE_SCENARIO="); ok {
			scenario = value
		}
	}
	text := "safe 8528a10f-6314-48ba-8230-542a4cf660f9 planted-api-credential planted-token-credential " + workerToken + " COMPLETE"
	if scenario != "completed" {
		// A partial message precedes auth failure; do not publish a completed
		// stop-token answer before the failure being exercised.
		text = strings.TrimSuffix(text, " COMPLETE")
	}
	runner.tokens[scenario] <- workerToken
	output := capturedCodexOutput("credential-" + scenario)
	output = bytes.ReplaceAll(output, []byte("captured answer COMPLETE"), []byte(text))
	output = bytes.ReplaceAll(output, []byte("public result"), []byte("tool-safe peer["+scenario+"] planted-api-credential planted-token-credential "+workerToken))
	if scenario != "completed" {
		output = bytes.ReplaceAll(output, []byte(`{"type":"item.completed","item":{"id":"answer"`), []byte(`{"type":"item.updated","item":{"id":"answer"`))
		output = bytes.ReplaceAll(output, []byte(`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":3,"output_tokens":4}}`), mustMarshalJSON(map[string]any{"type": "turn.failed", "error": map[string]string{"message": "controlled auth failure private-request-input planted-api-credential " + workerToken}}))
		observe(platformprocess.OutputStreamStdout, output)
		return platformprocess.CommandResult{Stdout: output, ExitCode: 1}, nil
	}
	observe(platformprocess.OutputStreamStdout, output)
	return platformprocess.CommandResult{Stdout: output}, nil
}

// Public detail reads must distinguish unreadable retained history from absence.
func TestCodexCapturedDetailsFailureMatrix(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "corruption-source")
	support.ClearSeedInputs(t, dir)
	runner := capturedCodexRunner{testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: capturedCodexOutput("captured-direct")})}
	host := startCapturedCodexHost(t, home, dir, runner, nil)
	invokeCapturedCodex(t, host, home, dir, "direct", "")
	assertCapturedCodexDetail(t, awaitCapturedCodexDetail(t, host, "captured-direct"), "captured-direct")
	host.Close(t)
	for name, damage := range map[string]func([]byte) []byte{
		"corrupt": func(data []byte) []byte { return append(data, []byte("{broken-corrupt-record}\n")...) },
		"torn":    func(data []byte) []byte { return append(data, []byte("{\"uncommitted\":")...) },
		"incomplete": func(data []byte) []byte {
			lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
			for i, line := range lines {
				if bytes.Contains(line, []byte(`"SourceEventID":"terminal"`)) {
					// Preserve the captured association but remove its terminal fact.
					lines[i] = bytes.Replace(line, []byte(`"phase":"COMPLETED"`), []byte(`"phase":"UPDATED"`), 1)
					lines[i] = bytes.Replace(lines[i], []byte(`"status":"COMPLETED"`), []byte(`"status":"RUNNING"`), 1)
					return append(bytes.Join(lines, []byte{'\n'}), '\n')
				}
			}
			panic("fixture has no terminal")
		},
		"truncated": func(data []byte) []byte { return data[:len(data)-10] },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertDamagedCodexReload(t, dir, damage)
		})
	}
	t.Run("unavailable", func(t *testing.T) {
		t.Parallel()
		faultHome, faultDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "capture-read-fault")
		support.ClearSeedInputs(t, faultDir)
		copyCapturedCodexStore(t, dir, faultDir)
		failed := startCapturedCodexHost(t, faultHome, faultDir, testutil.NewProviderCommandRunner(), func(string) ([]byte, error) {
			return nil, privateStorageFault()
		})
		body := getCodexProviderSessionDetailErrorBody(t, failed.URL(), "captured-direct", http.StatusInternalServerError)
		assertStorageFailureResponse(t, body, faultHome)
	})
}

func assertDamagedCodexReload(t *testing.T, dir string, damage func([]byte) []byte) {
	t.Helper()
	damagedHome, damagedDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "captured-corrupt")
	support.ClearSeedInputs(t, damagedDir)
	copyCapturedCodexStore(t, dir, damagedDir)
	mutateCapturedCodexJournals(t, damagedDir, damage)
	damaged := startCapturedCodexHost(t, damagedHome, damagedDir, testutil.NewProviderCommandRunner(), nil)
	body := getCodexProviderSessionDetailErrorBody(t, damaged.URL(), "captured-direct", http.StatusInternalServerError)
	assertCapturedCodexFailure(t, body, factoryapi.ErrorResponseCodeINTERNALERROR, damagedHome)
}

func mutateCapturedCodexJournals(t *testing.T, dir string, change func([]byte) []byte) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, ".you-agent-factory", "worker-recordings", "*.worker.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("journal fixture: %v %v", files, err)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, change(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCodexCapturedDetailsAmbiguousAndEmpty(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "captured-boundaries")
	support.ClearSeedInputs(t, dir)
	output := capturedCodexOutput("shared-provider-id")
	empty := []byte("{\"type\":\"thread.started\",\"thread_id\":\"empty-provider-id\"}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"empty\",\"type\":\"agent_message\",\"text\":\"fixture content\"}}\n")
	runner := capturedCodexRunner{testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: empty},
		platformprocess.CommandResult{Stdout: output},
		platformprocess.CommandResult{Stdout: output})}
	host := startCapturedCodexHost(t, home, dir, runner, nil)
	invokeCapturedCodex(t, host, home, dir, "empty-worker", "")
	awaitCapturedCodexDetail(t, host, "empty-provider-id")
	invokeCapturedCodex(t, host, home, dir, "first-worker", "")
	assertCapturedCodexDetail(t, awaitCapturedCodexDetail(t, host, "shared-provider-id"), "shared-provider-id")
	invokeCapturedCodex(t, host, home, dir, "second-worker", "")
	// Reload after joining capture removes transient ordering from the ambiguity assertion.
	host.Close(t)
	freshHome, freshDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "ambiguous-reload")
	support.ClearSeedInputs(t, freshDir)
	copyCapturedCodexStore(t, dir, freshDir)
	// Codex execution requires a final message. Model a valid retained capture
	// with no message content by clearing only text in a stopped, test-owned copy.
	mutateCapturedCodexJournals(t, freshDir, func(data []byte) []byte {
		lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
		for i, line := range lines {
			var record any
			if err := json.Unmarshal(line, &record); err != nil {
				t.Fatal(err)
			}
			clearCapturedFixtureText(record)
			lines[i], _ = json.Marshal(record)
		}
		return append(bytes.Join(lines, []byte{'\n'}), '\n')
	})
	fresh := startCapturedCodexHost(t, freshHome, freshDir, testutil.NewProviderCommandRunner(), nil)
	detail := awaitCapturedCodexDetail(t, fresh, "empty-provider-id")
	if len(detail.Transcript) != 0 || detail.Parse.TokenUsage != nil || detail.Parse.FunctionCalls == nil || detail.Parse.ParseErrors == nil || detail.Parse.Reasoning == nil || detail.Parse.Turns == nil || detail.Parse.UnknownEvents == nil {
		t.Fatalf("complete empty capture fabricated content or omitted required arrays: %+v", detail)
	}
	body := getCodexProviderSessionDetailErrorBody(t, fresh.URL(), "shared-provider-id", http.StatusInternalServerError)
	assertCapturedCodexFailure(t, body, factoryapi.ErrorResponseCodeINTERNALERROR, freshHome)
}

func clearCapturedFixtureText(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if strings.EqualFold(key, "text") || strings.EqualFold(key, "textDelta") {
				value[key] = ""
			} else {
				clearCapturedFixtureText(child)
			}
		}
	case []any:
		for _, child := range value {
			clearCapturedFixtureText(child)
		}
	}
}
