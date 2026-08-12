package http_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const remoteWorkerSessionProviderID = "session_fixture_codex_success"

func TestWorkerSessionRemoteInvokeObserveContinueUsesServerAfterDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	homeDir := t.TempDir()
	writeRemoteCodexRollout(t, homeDir, remoteWorkerSessionProviderID)
	providerOutput := readRemoteProviderFixture(t, "codex", "success", "stdout.jsonl")
	gate := make(chan struct{})
	runner := newRemoteInvokeContinueRunner(gate, providerOutput)

	factoryDir := support.ScaffoldSingleStepFactory(t, "remote-worker-session-invoke-continue")
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))
	env := remoteFunctionalEnvironment(homeDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       env,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
			ProviderSessionResolveHomeDirectory: func() (string, error) {
				return homeDir, nil
			},
		},
	})

	// Use the public HTTP admission boundary and close the actual submitting
	// connection after Workers admission. The server-owned execution must keep
	// running without a caller context to cancel it.
	connection := openDirectWorkerSessionConnection(t, server.URL(), "remote-invoke-request", "remote-source-session", "remote-source-dispatch")
	runner.waitStarted(t)
	if err := connection.Close(); err != nil {
		t.Fatalf("close submitting connection: %v", err)
	}
	if runner.wasCanceled() {
		t.Fatal("server-owned remote Worker Session was canceled by submitting disconnect")
	}
	close(gate)
	runner.waitFirstCompleted(t)

	clientRunner := testutil.NewProviderCommandRunner()
	client := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: clientRunner})
	support.CleanupProcess(t, client)

	serverURL := server.URL()
	assertRemoteSourceObservation(t, ctx, client, env, factoryDir, serverURL)
	continueRemoteWorkerSession(t, ctx, client, env, factoryDir, serverURL)
	assertRemoteContinuationUsesServer(t, clientRunner, runner)

	all := waitForRemoteWorkerSessionList(t, ctx, client, env, factoryDir, serverURL)
	if len(all) != 2 {
		t.Fatalf("top-level direct Worker Session count = %d, want source and distinct successor", len(all))
	}
}

func assertRemoteSourceObservation(t *testing.T, ctx context.Context, client support.Process, env []string, factoryDir, serverURL string) {
	t.Helper()
	listed := waitForRemoteWorkerSession(t, ctx, client, env, factoryDir, serverURL, "remote-source-session")
	if listed.State != "COMPLETED" || listed.WorkerSessionID != "remote-source-session" {
		t.Fatalf("remote source observation = %#v, want completed source", listed)
	}
	if listed.ProviderSession == nil || listed.ProviderSession.ID != remoteWorkerSessionProviderID {
		t.Fatalf("remote source provider session = %#v, want exact provider identity", listed.ProviderSession)
	}

	show := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "show", "--worker-session-id", "remote-source-session")
	var shown remoteWorkerSessionObservation
	decodeRemoteWorkerJSON(t, show.Stdout(), &shown)
	if shown.WorkerSessionID != "remote-source-session" || shown.State != "COMPLETED" {
		t.Fatalf("remote show = %#v, want completed source", shown)
	}

	read := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "read", "--worker-session-id", "remote-source-session")
	var transcript remoteWorkerSessionTranscript
	decodeRemoteWorkerJSON(t, read.Stdout(), &transcript)
	if transcript.WorkerSessionID != "remote-source-session" || transcript.ProviderSession.ID != remoteWorkerSessionProviderID || len(transcript.Entries) == 0 {
		t.Fatalf("remote transcript = %#v, want correlated entries", transcript)
	}
	transcriptBytes, _ := json.Marshal(transcript.Entries)
	if !strings.Contains(string(transcriptBytes), "Codex fixture answer COMPLETE") {
		t.Fatalf("remote transcript omitted provider answer: %s", transcriptBytes)
	}

	stream := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "stream", "--worker-session-id", "remote-source-session", "--replay-only")
	assertRemoteWorkerStreamTerminal(t, decodeRemoteWorkerNDJSON(t, stream.Stdout()), "remote-source-session")
}

func continueRemoteWorkerSession(t *testing.T, ctx context.Context, client support.Process, env []string, factoryDir, serverURL string) {
	t.Helper()
	continued := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "continue", "remote-source-session",
		"--request-id", "remote-continue-request", "--successor-worker-session-id", "remote-successor-session",
		"--user-message", "continue on the exact provider session", "--async")
	var continuation remoteWorkerSessionContinuation
	decodeRemoteWorkerJSON(t, continued.Stdout(), &continuation)
	if !continuation.Accepted || continuation.SourceWorkerSessionID != "remote-source-session" ||
		continuation.SuccessorWorkerSessionID != "remote-successor-session" {
		t.Fatalf("remote continuation admission = %#v, want accepted source/successor", continuation)
	}

	successorStream := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "stream", "--worker-session-id", "remote-successor-session", "--replay-only")
	assertRemoteWorkerStreamTerminal(t, decodeRemoteWorkerNDJSON(t, successorStream.Stdout()), "remote-successor-session")
	if !strings.Contains(successorStream.Stdout(), "Codex fixture answer COMPLETE") {
		t.Fatalf("remote successor stream omitted continued provider output:\n%s", successorStream.Stdout())
	}
}

func assertRemoteContinuationUsesServer(t *testing.T, clientRunner *testutil.ProviderCommandRunner, runner *remoteInvokeContinueRunner) {
	t.Helper()
	if clientRunner.CallCount() != 0 {
		t.Fatalf("remote CLI caused local provider fallback: %d calls", clientRunner.CallCount())
	}
	requests := runner.requestsSnapshot()
	if len(requests) != 2 {
		t.Fatalf("server provider requests = %d, want initial and continuation only", len(requests))
	}
	if strings.Contains(strings.Join(requests[0].Args, " "), "resume") {
		t.Fatalf("initial server provider command resumed unexpectedly: %#v", requests[0].Args)
	}
	if !containsRemoteArgSequence(requests[1].Args, []string{"resume", remoteWorkerSessionProviderID}) {
		t.Fatalf("continuation server provider command = %#v, want exact resume identity", requests[1].Args)
	}
}

type remoteWorkerSessionObservation struct {
	WorkerSessionID string                          `json:"workerSessionId"`
	State           string                          `json:"state"`
	ProviderSession *remoteWorkerSessionProviderRef `json:"providerSession"`
}

type remoteWorkerSessionProviderRef struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

type remoteWorkerSessionTranscript struct {
	WorkerSessionID string                         `json:"workerSessionId"`
	ProviderSession remoteWorkerSessionProviderRef `json:"providerSession"`
	Entries         []map[string]any               `json:"entries"`
}

type remoteWorkerSessionContinuation struct {
	RequestID                string `json:"requestId"`
	SourceWorkerSessionID    string `json:"sourceWorkerSessionId"`
	SuccessorWorkerSessionID string `json:"successorWorkerSessionId"`
	Accepted                 bool   `json:"accepted"`
	State                    string `json:"state"`
}

type remoteWorkerSessionListResponse struct {
	Sessions []remoteWorkerSessionObservation `json:"sessions"`
}

type remoteWorkerSessionStreamFrame struct {
	Delivery        string                     `json:"delivery"`
	WorkerSessionID string                     `json:"workerSessionId"`
	Event           *remoteWorkerSessionEvent  `json:"event"`
	ReplaySummary   *remoteWorkerSessionReplay `json:"replaySummary"`
}

type remoteWorkerSessionEvent struct {
	Payload json.RawMessage `json:"payload"`
}

type remoteWorkerSessionReplay struct {
	Complete bool `json:"complete"`
}

func waitForRemoteWorkerSession(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	factoryDir, serverURL, workerSessionID string,
) remoteWorkerSessionObservation {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	// The provider edge has completed, but the server commits the public
	// Worker Session projection asynchronously. Poll the public list operation
	// until that projection is observable; a fixed sleep would not establish
	// the rediscovery boundary this scenario is proving.
	for {
		for _, session := range waitForRemoteWorkerSessionList(t, ctx, process, env, factoryDir, serverURL) {
			if session.WorkerSessionID == workerSessionID && session.State == "COMPLETED" {
				return session
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for completed remote Worker Session %q", workerSessionID)
		case <-ctx.Done():
			t.Fatalf("waiting for remote Worker Session %q canceled: %v", workerSessionID, ctx.Err())
		}
	}
}

func waitForRemoteWorkerSessionList(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	factoryDir, serverURL string,
) []remoteWorkerSessionObservation {
	t.Helper()
	inputs := executeRemoteWorkerCLI(t, ctx, process, env, factoryDir, serverURL,
		"--json", "worker-sessions", "list", "--scope", "direct")
	var listed remoteWorkerSessionListResponse
	decodeRemoteWorkerJSON(t, inputs.Stdout(), &listed)
	return listed.Sessions
}

func executeRemoteWorkerCLI(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	factoryDir, serverURL string,
	args ...string,
) *support.CapturedInputs {
	t.Helper()
	command := append([]string{"you", "--remote", "--server", serverURL}, args...)
	inputs := support.FakeInputs(ctx, command)
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("remote CLI %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(command, " "), err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs
}

func decodeRemoteWorkerJSON(t *testing.T, stdout string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), target); err != nil {
		t.Fatalf("decode remote Worker Session JSON: %v\nstdout:\n%s", err, stdout)
	}
}

func decodeRemoteWorkerNDJSON(t *testing.T, stdout string) []remoteWorkerSessionStreamFrame {
	t.Helper()
	var frames []remoteWorkerSessionStreamFrame
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame remoteWorkerSessionStreamFrame
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode Worker Session stream frame: %v\nline:%s", err, line)
		}
		if frame.Delivery == "" {
			var summary remoteWorkerSessionReplay
			if err := json.Unmarshal([]byte(line), &summary); err != nil {
				t.Fatalf("decode Worker Session replay summary: %v\nline:%s", err, line)
			}
			frame.Delivery = "REPLAY_SUMMARY"
			frame.ReplaySummary = &summary
		}
		frames = append(frames, frame)
	}
	return frames
}

func assertRemoteWorkerStreamTerminal(t *testing.T, frames []remoteWorkerSessionStreamFrame, workerSessionID string) {
	t.Helper()
	if len(frames) == 0 {
		t.Fatalf("Worker Session stream for %q returned no frames", workerSessionID)
	}
	terminal := false
	completeSummary := false
	for _, frame := range frames {
		if frame.WorkerSessionID != "" && frame.WorkerSessionID != workerSessionID {
			t.Fatalf("Worker Session stream frame identity = %q, want %q", frame.WorkerSessionID, workerSessionID)
		}
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			terminal = true
		}
		if frame.Delivery == "REPLAY_SUMMARY" && frame.ReplaySummary != nil && frame.ReplaySummary.Complete {
			completeSummary = true
		}
	}
	if !terminal || !completeSummary {
		t.Fatalf("Worker Session stream frames = %#v, want terminal and complete replay summary", frames)
	}
}

func readRemoteProviderFixture(t *testing.T, provider, caseName, fileName string) []byte {
	t.Helper()
	path := filepath.Join(testutil.MustRepoRoot(t), filepath.FromSlash(support.ProviderSessionFixturePath(provider, caseName, fileName)))
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read provider fixture %s: %v", path, err)
	}
	return contents
}

func writeRemoteCodexRollout(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	directory := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create Codex rollout directory: %v", err)
	}
	contents := readRemoteProviderFixture(t, "codex", "success", "rollout.jsonl")
	path := filepath.Join(directory, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write Codex rollout fixture: %v", err)
	}
}

func remoteFunctionalEnvironment(homeDir string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return env
}

func containsRemoteArgSequence(args, want []string) bool {
	for index := 0; index <= len(args)-len(want); index++ {
		match := true
		for offset := range want {
			if args[index+offset] != want[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

type remoteInvokeContinueRunner struct {
	gate      <-chan struct{}
	outputs   [][]byte
	started   chan struct{}
	firstDone chan struct{}
	startOnce sync.Once
	doneOnce  sync.Once

	mu       sync.Mutex
	calls    int
	canceled bool
	requests []platformprocess.CommandRequest
}

func newRemoteInvokeContinueRunner(gate <-chan struct{}, output []byte) *remoteInvokeContinueRunner {
	return &remoteInvokeContinueRunner{
		gate: gate, outputs: [][]byte{append([]byte(nil), output...), append([]byte(nil), output...)},
		started: make(chan struct{}), firstDone: make(chan struct{}),
	}
}

func (r *remoteInvokeContinueRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	index := r.calls
	r.calls++
	request.Args = append([]string(nil), request.Args...)
	r.requests = append(r.requests, request)
	r.mu.Unlock()
	if index == 0 {
		r.startOnce.Do(func() { close(r.started) })
		select {
		case <-r.gate:
		case <-ctx.Done():
			r.mu.Lock()
			r.canceled = true
			r.mu.Unlock()
			return platformprocess.CommandResult{}, ctx.Err()
		}
		r.doneOnce.Do(func() { close(r.firstDone) })
	}
	return platformprocess.CommandResult{Stdout: append([]byte(nil), r.outputs[minRemoteRunnerIndex(index, len(r.outputs))]...)}, nil
}

func minRemoteRunnerIndex(index, length int) int {
	if index < length {
		return index
	}
	return length - 1
}

func (r *remoteInvokeContinueRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for remote Worker Session provider admission")
	}
}

func (r *remoteInvokeContinueRunner) waitFirstCompleted(t *testing.T) {
	t.Helper()
	select {
	case <-r.firstDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for remote Worker Session provider completion")
	}
}

func (r *remoteInvokeContinueRunner) wasCanceled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.canceled
}

func (r *remoteInvokeContinueRunner) requestsSnapshot() []platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(r.requests))
	for index, request := range r.requests {
		requests[index] = request
		requests[index].Args = append([]string(nil), request.Args...)
	}
	return requests
}

var _ platformprocess.CommandRunner = (*remoteInvokeContinueRunner)(nil)
