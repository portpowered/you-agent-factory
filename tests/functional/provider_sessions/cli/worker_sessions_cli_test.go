package cli_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const (
	workerSessionsCodexSuccessID = "session_fixture_codex_success"
	workerSessionsCodexFailureID = "session_fixture_codex_structured_failure"
)

// TestWorkerSessionsCLI proves the complete diagnosis path through one
// root-built application: accepted Work is executed through an injected
// provider command runner, and public CLI list/show/stream/read commands
// correlate successful and failed provider activity back to Work and attempt
// identity, timing, usage, transcript, and failure cause.
func TestWorkerSessionsCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	factoryDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.ClearSeedInputs(t, factoryDir)
	support.WriteAgentConfig(t, factoryDir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))

	homeDir := t.TempDir()
	successStdout := readProviderFixture(t, "codex", "success", "stdout.jsonl")
	successRollout := readProviderFixture(t, "codex", "success", "rollout.jsonl")
	failureStdout := readProviderFixture(t, "codex", "structured-failure", "stdout.jsonl")
	writeCodexRollout(t, homeDir, workerSessionsCodexSuccessID, successRollout)
	failureRollout := bytes.ReplaceAll(successRollout, []byte(workerSessionsCodexSuccessID), []byte(workerSessionsCodexFailureID))
	failureRollout = bytes.ReplaceAll(failureRollout, []byte("Codex fixture answer COMPLETE"), []byte("Codex authentication failed."))
	writeCodexRollout(t, homeDir, workerSessionsCodexFailureID, failureRollout)

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: successStdout},
		platformprocess.CommandResult{Stdout: failureStdout, ExitCode: 1},
	)
	api := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:                    api.Start,
		ProviderCommandRunner:               runner,
		ProviderSessionResolveHomeDirectory: func() (string, error) { return homeDir, nil },
	})
	support.CleanupProcess(t, process)

	env := functionalEnvironment(homeDir)
	recordPath := filepath.Join(t.TempDir(), "worker-session-recording.json")
	serverInputs := support.FakeInputs(ctx, []string{
		"you", "run", "--dir", factoryDir, "--continuously", "--with-server", "--quiet", "--record", recordPath,
	})
	serverInputs.Input.Env = env
	serverInputs.Input.WorkingDirectory = factoryDir
	server := support.StartProcessCommand(t, process, serverInputs.Input)
	baseURL := api.WaitForURL(t)

	helpInputs := executeCLI(t, ctx, process, env, factoryDir, "worker-sessions")
	for _, marker := range []string{
		"Usage:",
		"list        List Worker Sessions",
		"read        Read a finished Worker Session",
		"show        Show one Worker Session",
		"stream      Stream one Worker Session",
	} {
		if !strings.Contains(helpInputs.Stdout(), marker) {
			t.Fatalf("worker-sessions help omitted %q:\n%s", marker, helpInputs.Stdout())
		}
	}
	explicitHelpInputs := executeCLI(t, ctx, process, env, factoryDir, "worker-sessions", "--help")
	if !strings.Contains(explicitHelpInputs.Stdout(), "Usage:") {
		t.Fatalf("worker-sessions --help omitted usage:\n%s", explicitHelpInputs.Stdout())
	}
	unknownInputs, unknownErr := executeCLIExpectError(t, ctx, process, env, factoryDir, "worker-sessions", "--unknown")
	if unknownErr == nil {
		t.Fatal("worker-sessions unknown argument returned nil error")
	}
	if !strings.Contains(unknownErr.Error()+unknownInputs.Stderr(), "unknown command") {
		t.Fatalf("worker-sessions unknown argument omitted diagnostic: %v\nstderr:\n%s", unknownErr, unknownInputs.Stderr())
	}
	completionInputs := executeCLI(t, ctx, process, env, factoryDir, "__complete", "worker-sessions", "")
	for _, marker := range []string{"list", "read", "show", "stream"} {
		if !strings.Contains(completionInputs.Stdout(), marker) {
			t.Fatalf("worker-sessions completion omitted %q:\n%s", marker, completionInputs.Stdout())
		}
	}

	successWorkID := submitWork(t, ctx, process, env, factoryDir, baseURL, "worker-session-cli-success")
	waitForWorkerSession(t, ctx, process, env, factoryDir, baseURL, successWorkID)
	streamWorkerSession(t, ctx, process, env, factoryDir, baseURL, workerSessionsCodexSuccessID, "COMPLETED")
	assertSuccessfulWorkerSession(t, ctx, process, env, factoryDir, baseURL, successWorkID)

	failureWorkID := submitWork(t, ctx, process, env, factoryDir, baseURL, "worker-session-cli-failure")
	waitForWorkerSession(t, ctx, process, env, factoryDir, baseURL, failureWorkID)
	streamWorkerSession(t, ctx, process, env, factoryDir, baseURL, workerSessionsCodexFailureID, "FAILED")
	assertFailedWorkerSession(t, ctx, process, env, factoryDir, baseURL, failureWorkID)
	assertMissingWorkerSessionOutcomes(t, ctx, process, env, factoryDir, baseURL)
	assertMissingWorkerSessionInputs(t, ctx, process, env, factoryDir, baseURL)

	if runner.CallCount() != 2 {
		t.Fatalf("provider command calls = %d, want one success and one failure invocation", runner.CallCount())
	}
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("recorded worker activity missing at %s: %v", recordPath, err)
	}

	server.Stop(t)
	if err := server.Err(); err != nil {
		t.Fatalf("server Process.Execute: %v\nstdout:\n%s\nstderr:\n%s", err, serverInputs.Stdout(), serverInputs.Stderr())
	}

	functionalevidence.Covers(t,
		"cli/you.worker-sessions.list",
		"cli/you.worker-sessions.read",
		"cli/you.worker-sessions.show",
		"cli/you.worker-sessions.stream",
	)
}

type workerSessionJSON struct {
	AttemptID       string               `json:"attemptId"`
	DurationMillis  *int64               `json:"durationMillis"`
	DurationBasis   string               `json:"durationBasis"`
	Failure         json.RawMessage      `json:"failure"`
	ProviderSession *providerSessionJSON `json:"providerSession"`
	State           string               `json:"state"`
	TokenUsage      *tokenUsageJSON      `json:"tokenUsage"`
	Transcript      string               `json:"transcript"`
	WorkIDs         []string             `json:"workIds"`
	WorkerSessionID string               `json:"workerSessionId"`
}

type providerSessionJSON struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

type tokenUsageJSON struct {
	InputTokens  *int `json:"inputTokens"`
	OutputTokens *int `json:"outputTokens"`
	TotalTokens  *int `json:"totalTokens"`
}

type workerSessionListJSON struct {
	Sessions []workerSessionJSON `json:"sessions"`
}

type transcriptJSON struct {
	Entries         []transcriptEntryJSON `json:"entries"`
	ProviderSession providerSessionJSON   `json:"providerSession"`
	State           string                `json:"state"`
	WorkIDs         []string              `json:"workIds"`
	WorkerSessionID string                `json:"workerSessionId"`
}

type transcriptEntryJSON struct {
	Text    string `json:"text"`
	Summary string `json:"summary"`
	Type    string `json:"type"`
}

type streamFrameJSON struct {
	Delivery        string               `json:"delivery"`
	ErrorCode       *string              `json:"errorCode"`
	ErrorMessage    *string              `json:"errorMessage"`
	Event           json.RawMessage      `json:"event"`
	ProviderSession *providerSessionJSON `json:"providerSession"`
	WorkIDs         []string             `json:"workIds"`
}

func submitWork(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, name string) string {
	t.Helper()
	request := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":"task","payload":{"title":%q}}]}`,
		name, name, name,
	)
	inputs := executeCLI(t, ctx, process, env, factoryDir, "--server", baseURL, "--json", "submit", "batch", request)
	var response struct {
		WorkCount int `json:"workCount"`
		Works     []struct {
			WorkID string `json:"workId"`
		} `json:"works"`
	}
	decodeCLIJSON(t, inputs, &response)
	if response.WorkCount != 1 || len(response.Works) != 1 || strings.TrimSpace(response.Works[0].WorkID) == "" {
		t.Fatalf("submit batch response missing one accepted Work: %#v\noutput:\n%s", response, inputs.Stdout())
	}
	return response.Works[0].WorkID
}

func waitForWorkerSession(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, workID string) {
	t.Helper()

	// Work admission and the first Worker Session opening record are separate
	// asynchronous runtime steps. Synchronize through the customer-facing list
	// projection so stream starts only after its session identity is observable;
	// a fixed sleep would make this coverage slower and still race under CI.
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	var lastOutput string
	for {
		inputs := support.FakeInputs(ctx, []string{
			"you", "--server", baseURL, "worker-sessions", "list", "--work-id", workID, "--output", "json",
		})
		inputs.Input.Env = append([]string(nil), env...)
		inputs.Input.WorkingDirectory = factoryDir
		if err := process.Execute(inputs.Input); err == nil {
			var listed workerSessionListJSON
			if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &listed); decodeErr == nil && len(listed.Sessions) > 0 {
				return
			}
			lastOutput = inputs.Stdout()
		} else {
			lastErr = err
			lastOutput = inputs.Stdout()
		}

		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for Worker Session for Work %s: err=%v stdout=%s", workID, lastErr, lastOutput)
		case <-ctx.Done():
			t.Fatalf("waiting for Worker Session for Work %s canceled: %v", workID, ctx.Err())
		}
	}
}

func assertSuccessfulWorkerSession(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, workID string) {
	t.Helper()
	listInputs := executeCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "list", "--work-id", workID, "--output", "json")
	var listed workerSessionListJSON
	decodeCLIJSON(t, listInputs, &listed)
	if len(listed.Sessions) != 1 {
		t.Fatalf("successful Work session count = %d, want 1: %#v", len(listed.Sessions), listed)
	}
	session := listed.Sessions[0]
	assertWorkerSessionIdentity(t, session, workerSessionsCodexSuccessID, workID)
	if session.State != "COMPLETED" || session.AttemptID == "" || session.DurationMillis == nil || *session.DurationMillis < 0 {
		t.Fatalf("successful session lifecycle projection = %#v", session)
	}
	if session.DurationBasis != "RECORDED_TIMESTAMPS" {
		t.Fatalf("successful duration basis = %q, want RECORDED_TIMESTAMPS", session.DurationBasis)
	}
	if session.TokenUsage == nil || session.TokenUsage.InputTokens == nil || *session.TokenUsage.InputTokens != 8 ||
		session.TokenUsage.OutputTokens == nil || *session.TokenUsage.OutputTokens != 12 ||
		session.TokenUsage.TotalTokens == nil || *session.TokenUsage.TotalTokens != 20 {
		t.Fatalf("successful token usage = %#v, want 8 input, 12 output, 20 total", session.TokenUsage)
	}

	showInputs := executeCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "show", "--provider", "codex", "--kind", "session_id", "--id", workerSessionsCodexSuccessID, "--output", "json")
	var shown workerSessionJSON
	decodeCLIJSON(t, showInputs, &shown)
	assertWorkerSessionIdentity(t, shown, workerSessionsCodexSuccessID, workID)
	if shown.DurationMillis == nil || shown.TokenUsage == nil || shown.TokenUsage.TotalTokens == nil || *shown.TokenUsage.TotalTokens != 20 {
		t.Fatalf("successful show omitted duration or token usage: %#v", shown)
	}

	readInputs := executeCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "read", "--provider", "codex", "--kind", "session_id", "--id", workerSessionsCodexSuccessID, "--output", "json")
	var transcript transcriptJSON
	decodeCLIJSON(t, readInputs, &transcript)
	if transcript.ProviderSession.ID != workerSessionsCodexSuccessID || !containsString(transcript.WorkIDs, workID) || len(transcript.Entries) == 0 {
		t.Fatalf("successful transcript correlation = %#v", transcript)
	}
	transcriptText, _ := json.Marshal(transcript.Entries)
	if !strings.Contains(string(transcriptText), "Codex fixture answer COMPLETE") {
		t.Fatalf("successful transcript omitted fixture answer:\n%s", transcriptText)
	}
}

func assertFailedWorkerSession(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, workID string) {
	t.Helper()
	listInputs := executeCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "list", "--work-id", workID, "--output", "json")
	var listed workerSessionListJSON
	decodeCLIJSON(t, listInputs, &listed)
	if len(listed.Sessions) != 1 {
		t.Fatalf("failed Work session count = %d, want 1: %#v", len(listed.Sessions), listed)
	}
	session := listed.Sessions[0]
	assertWorkerSessionIdentity(t, session, workerSessionsCodexFailureID, workID)
	if session.State != "FAILED" || session.AttemptID == "" || session.DurationMillis == nil || *session.DurationMillis < 0 {
		t.Fatalf("failed session lifecycle projection = %#v", session)
	}
	if !strings.Contains(strings.ToLower(string(session.Failure)), "auth") && !strings.Contains(strings.ToLower(string(session.Failure)), "401") {
		t.Fatalf("failed session omitted authentication diagnosis: %s", session.Failure)
	}

	showInputs := executeCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "show", "--provider", "codex", "--kind", "session_id", "--id", workerSessionsCodexFailureID, "--output", "json")
	var shown workerSessionJSON
	decodeCLIJSON(t, showInputs, &shown)
	assertWorkerSessionIdentity(t, shown, workerSessionsCodexFailureID, workID)
	if shown.State != "FAILED" || (!strings.Contains(strings.ToLower(string(shown.Failure)), "auth") && !strings.Contains(strings.ToLower(string(shown.Failure)), "401")) {
		t.Fatalf("failed show omitted recorded failure cause: %#v", shown)
	}
}

func assertMissingWorkerSessionOutcomes(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string) {
	t.Helper()
	cases := []struct {
		name string
		args []string
		code string
	}{
		{
			name: "list missing Work",
			args: []string{"worker-sessions", "list", "--work-id", "work-missing-from-cli", "--output", "json"},
			code: "WORK_NOT_FOUND",
		},
		{
			name: "show missing session",
			args: []string{"worker-sessions", "show", "--provider", "codex", "--kind", "session_id", "--id", "provider-session-missing-from-cli", "--output", "json"},
			code: "WORKER_SESSION_NOT_FOUND",
		},
		{
			name: "read missing session",
			args: []string{"worker-sessions", "read", "--provider", "codex", "--kind", "session_id", "--id", "provider-session-missing-from-cli", "--output", "json"},
			code: "WORKER_SESSION_NOT_FOUND",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			inputs, err := executeCLIExpectError(t, ctx, process, env, factoryDir, append([]string{"--server", baseURL}, test.args...)...)
			if err == nil {
				t.Fatal("missing Worker Session operation returned nil error")
			}
			output := inputs.Stdout() + inputs.Stderr() + err.Error()
			if !strings.Contains(output, test.code) {
				t.Fatalf("missing Worker Session operation omitted %s: %s", test.code, output)
			}
		})
	}
}

func assertMissingWorkerSessionInputs(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string) {
	t.Helper()
	cases := []struct {
		name string
		args []string
		code string
	}{
		{
			name: "list local JSON output",
			args: []string{"--server", baseURL, "worker-sessions", "list", "--output", "json"},
			code: "WORK_ID_REQUIRED",
		},
		{
			name: "list global JSON output",
			args: []string{"--json", "worker-sessions", "list"},
			code: "WORK_ID_REQUIRED",
		},
		{
			name: "show local provider validation",
			args: []string{"--server", baseURL, "worker-sessions", "show", "--output", "json"},
			code: "PROVIDER_REQUIRED",
		},
		{
			name: "show global kind validation",
			args: []string{"--json", "worker-sessions", "show", "--provider", "codex"},
			code: "SESSION_KIND_REQUIRED",
		},
		{
			name: "show local ID validation",
			args: []string{"--server", baseURL, "worker-sessions", "show", "--provider", "codex", "--kind", "session_id", "--output", "json"},
			code: "SESSION_ID_REQUIRED",
		},
		{
			name: "read local provider validation",
			args: []string{"--server", baseURL, "worker-sessions", "read", "--output", "json"},
			code: "PROVIDER_REQUIRED",
		},
		{
			name: "read global kind validation",
			args: []string{"--json", "worker-sessions", "read", "--provider", "codex"},
			code: "SESSION_KIND_REQUIRED",
		},
		{
			name: "read local ID validation",
			args: []string{"--server", baseURL, "worker-sessions", "read", "--provider", "codex", "--kind", "session_id", "--output", "json"},
			code: "SESSION_ID_REQUIRED",
		},
		{
			name: "stream local provider validation",
			args: []string{"--server", baseURL, "worker-sessions", "stream", "--output", "json"},
			code: "PROVIDER_REQUIRED",
		},
		{
			name: "stream global kind validation",
			args: []string{"--json", "worker-sessions", "stream", "--provider", "codex"},
			code: "SESSION_KIND_REQUIRED",
		},
		{
			name: "stream local ID validation",
			args: []string{"--server", baseURL, "worker-sessions", "stream", "--provider", "codex", "--kind", "session_id", "--output", "json"},
			code: "SESSION_ID_REQUIRED",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			inputs, err := executeCLIExpectError(t, ctx, process, env, factoryDir, test.args...)
			if err == nil {
				t.Fatal("missing required Worker Session input returned nil error")
			}
			var payload struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &payload); decodeErr != nil {
				t.Fatalf("decode required-input JSON: %v\nstdout:\n%s\nstderr:\n%s", decodeErr, inputs.Stdout(), inputs.Stderr())
			}
			if payload.Code != test.code || strings.TrimSpace(payload.Message) == "" {
				t.Fatalf("required-input payload = %#v, want code %s and message", payload, test.code)
			}
			if strings.Contains(inputs.Stdout(), "required flag(s)") || strings.Contains(inputs.Stderr(), "required flag(s)") {
				t.Fatalf("Cobra required-flag prose leaked into machine-readable failure:\nstdout:\n%s\nstderr:\n%s", inputs.Stdout(), inputs.Stderr())
			}
		})
	}
}

func assertWorkerSessionIdentity(t *testing.T, session workerSessionJSON, providerID, workID string) {
	t.Helper()
	if session.WorkerSessionID == "" || session.ProviderSession == nil ||
		session.ProviderSession.Provider != "codex" || session.ProviderSession.Kind != "session_id" ||
		session.ProviderSession.ID != providerID || !containsString(session.WorkIDs, workID) {
		t.Fatalf("worker session identity = %#v, want provider %s and Work %s", session, providerID, workID)
	}
}

func streamWorkerSession(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, providerID, terminalState string) {
	t.Helper()
	inputs := support.FakeInputs(ctx, []string{
		"you", "--server", baseURL, "worker-sessions", "stream",
		"--provider", "codex", "--kind", "session_id", "--id", providerID, "--output", "json",
	})
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	command := support.StartProcessCommand(t, process, inputs.Input)
	select {
	case <-command.Done():
	case <-ctx.Done():
		command.Stop(t)
		t.Fatalf("worker-sessions stream %s timed out: %v", providerID, ctx.Err())
	}
	if err := command.Err(); err != nil {
		t.Fatalf("worker-sessions stream %s: %v\nstdout:\n%s\nstderr:\n%s", providerID, err, inputs.Stdout(), inputs.Stderr())
	}

	scanner := bufio.NewScanner(strings.NewReader(inputs.Stdout()))
	var deliveries []string
	for scanner.Scan() {
		var frame streamFrameJSON
		if err := json.Unmarshal([]byte(scanner.Text()), &frame); err != nil {
			t.Fatalf("decode worker-sessions stream frame: %v\nframe:%s", err, scanner.Text())
		}
		deliveries = append(deliveries, frame.Delivery)
		if frame.ProviderSession == nil || frame.ProviderSession.ID != providerID {
			t.Fatalf("stream frame provider session = %#v, want %s", frame.ProviderSession, providerID)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan worker-sessions stream output: %v", err)
	}
	if !containsString(deliveries, "RECORD") && !containsString(deliveries, "RECORD_REPLAY") {
		t.Fatalf("worker-sessions stream omitted activity frame: %v\noutput:\n%s", deliveries, inputs.Stdout())
	}
	if !containsString(deliveries, "TERMINAL") && !containsString(deliveries, "TERMINAL_REPLAY") {
		t.Fatalf("worker-sessions stream omitted terminal frame: %v\noutput:\n%s", deliveries, inputs.Stdout())
	}
	streamOutput := inputs.Stdout()
	if !strings.Contains(streamOutput, `"phase":"STARTED"`) || !strings.Contains(streamOutput, `"status":"`+terminalState+`"`) {
		t.Fatalf("worker-sessions stream omitted active or terminal state %s: %s", terminalState, streamOutput)
	}
}

func executeCLI(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir string, args ...string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(ctx, append([]string{"you"}, args...))
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("you %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs
}

func executeCLIExpectError(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir string, args ...string) (*support.CapturedInputs, error) {
	t.Helper()
	inputs := support.FakeInputs(ctx, append([]string{"you"}, args...))
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	return inputs, process.Execute(inputs.Input)
}

func decodeCLIJSON(t *testing.T, inputs *support.CapturedInputs, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), target); err != nil {
		t.Fatalf("decode CLI JSON: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
}

func readProviderFixture(t *testing.T, provider, caseName, fileName string) []byte {
	t.Helper()
	path := filepath.Join(testutil.MustRepoRoot(t), filepath.FromSlash(support.ProviderSessionFixturePath(provider, caseName, fileName)))
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read provider fixture %s: %v", path, err)
	}
	return contents
}

func writeCodexRollout(t *testing.T, homeDir, sessionID string, contents []byte) {
	t.Helper()
	directory := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create Codex session directory: %v", err)
	}
	path := filepath.Join(directory, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write Codex rollout fixture: %v", err)
	}
}

func functionalEnvironment(homeDir string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return env
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
