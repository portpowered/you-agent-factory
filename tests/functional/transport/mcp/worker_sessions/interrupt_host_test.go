package workersessions_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const interruptProviderID = "session_fixture_codex_success"

// Each invocation owns its host/profile and two provider barriers. The runner
// emits the association through the real streaming provider boundary, then
// waits for public control. No private Worker Session state is accessed.
type interruptHostRunner struct {
	output        []byte
	started       chan platformprocess.CommandRequest
	sourceStopped chan struct{}
	mu            sync.Mutex
	requests      []platformprocess.CommandRequest
}

func (r *interruptHostRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return r.RunStreaming(ctx, req, nil)
}

func (r *interruptHostRunner) RunStreaming(ctx context.Context, req platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	index := len(r.requests)
	r.requests = append(r.requests, req)
	r.mu.Unlock()
	if index > 1 {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected duplicate successor execution")
	}
	if index == 1 {
		select {
		case <-r.sourceStopped:
		default:
			return platformprocess.CommandResult{}, fmt.Errorf("successor started before source stopped")
		}
	}
	if observer != nil {
		end := bytes.IndexByte(r.output, '\n') + 1
		observer(platformprocess.OutputStreamStdout, r.output[:end])
	}
	r.started <- req
	<-ctx.Done()
	if index == 0 {
		close(r.sourceStopped)
	}
	return platformprocess.CommandResult{}, ctx.Err()
}

func runRealHostInterrupt(t *testing.T, process support.Process) {
	t.Helper()
	host, runner, dir := startInterruptHost(t, nil)
	session, ctx := startMCP(t, process, host.URL())
	admitInterruptSource(t, ctx, host.URL(), dir, runner)
	before := getHost(t, host.URL()+"/worker-sessions/source").(map[string]any)
	if before["state"] != "RUNNING" || before["providerSessionAvailable"] != true {
		t.Fatalf("streaming association not visible: %v", before)
	}
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.read", map[string]any{"workerSessionId": "source", "view": "transcript"}), "worker_session.conflict", false)
	args := map[string]any{"workerSessionId": "source", "operation": "INTERRUPT", "successorWorkerSessionId": "successor", "replacementMessage": "replace the initial instruction"}
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.control", args), "worker_session.invalid_request", false)
	assertRuntimeObservationParity(t, before, getHost(t, host.URL()+"/worker-sessions/source"))
	args["requestId"] = "interrupt-request"
	admitted := callWorker(t, ctx, session, "control", args)["result"].(map[string]any)
	assertInterruptAdmission(t, admitted)
	waitControlSignal(t, runner.sourceStopped)
	request := waitProviderRequest(t, ctx, runner)
	if !strings.Contains(strings.Join(request.Args, " "), "resume "+interruptProviderID) || !strings.Contains(string(request.Stdin), "replace the initial instruction") {
		t.Fatalf("successor provider request lost resume identity or replacement: args=%v stdin=%s", request.Args, request.Stdin)
	}
	assertJSONEqual(t, admitted, postHostJSON(t, ctx, host.URL()+"/worker-sessions/source/interrupt", interruptPayload("replace the initial instruction"), http.StatusAccepted))
	assertInterruptCLIParity(t, host, admitted)
	assertJSONEqual(t, admitted, callWorker(t, ctx, session, "control", args)["result"])
	args["replacementMessage"] = "changed immutable tuple"
	// Current HTTP mapping classifies validation-phase tuple conflicts as 400:
	// ErrInterruptValidation precedes ErrInterruptRequestIDConflict. Preserve
	// that public meaning in T9; do not change the host's control policy here.
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.control", args), "worker_session.invalid_request", false)
	postHostJSON(t, ctx, host.URL()+"/worker-sessions/source/interrupt", interruptPayload("changed immutable tuple"), http.StatusBadRequest)
	assertTerminalTranscriptParity(t, ctx, session, host)
	assertTerminalReplayParity(t, ctx, session, host, "source")
	listed := callWorker(t, ctx, session, "list", map[string]any{"scope": "direct"})["result"].(map[string]any)
	workers := listed["sessions"].([]any)
	if len(workers) != 2 {
		t.Fatalf("interrupt created unexpected fleet: %v", listed)
	}
	assertProviderCallCount(t, runner, 2)
	source := getHost(t, host.URL()+"/worker-sessions/source").(map[string]any)
	successor := getHost(t, host.URL()+"/worker-sessions/successor").(map[string]any)
	if source["successorWorkerSessionId"] != "successor" || successor["predecessorWorkerSessionId"] != "source" || source["provider"] != "codex" || successor["provider"] != "codex" {
		t.Fatalf("admitted lineage/provider lost: source=%v successor=%v", source, successor)
	}
	assertRuntimeObservationParity(t, source, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": "source"})["result"].(map[string]any)["session"])
	assertFactoryCLIParity(t, host, "successor", successor)
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "successor", "operation": "TERMINATE"})
	assertInterruptMetadataRecovery(t, process, host, runner, dir)
}

func assertInterruptMetadataRecovery(t *testing.T, process support.Process, host *support.FunctionalAPIServer, runner *interruptHostRunner, dir string) {
	t.Helper()
	// Join the original writer before constructing a new host over the same
	// isolated profile. Recovery must use captures with native reads denied.
	host.Close(t)
	home := t.TempDir()
	native := &deniedMetadataProviderFiles{}
	reopened := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir),
			ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil }, ProviderSessionFileSystem: native,
		},
	})
	session, ctx := startMCP(t, process, reopened.URL())
	page := historyParityPage(t, ctx, session, reopened, "archived", "direct", "")
	if len(page["sessions"].([]any)) != 2 {
		t.Fatalf("recovery lost source/successor membership: %v", page)
	}
	for _, value := range page["sessions"].([]any) {
		observation := value.(map[string]any)
		id := observation["workerSessionId"].(string)
		if observation["provider"] != "codex" || observation["model"] != "test-model" {
			t.Fatalf("recovery lost captured selection: %v", observation)
		}
		if (id == "source" && observation["successorWorkerSessionId"] != "successor") ||
			(id == "successor" && observation["predecessorWorkerSessionId"] != "source") {
			t.Fatalf("recovery lost committed lineage: %v", observation)
		}
		selected := getHost(t, reopened.URL()+"/worker-sessions/"+id)
		assertJSONEqual(t, observation, selected)
		assertRuntimeObservationParity(t, selected, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
		assertFactoryCLIParity(t, reopened, id, selected)
		assertArchivedHostTiming(t, reopened, id)
	}
	assertProviderCallCount(t, runner, 2)
	if native.calls.Load() != 0 {
		t.Fatal("archived metadata recovery attempted a provider-native file read")
	}
}

type deniedMetadataProviderFiles struct{ calls atomic.Int64 }

func (f *deniedMetadataProviderFiles) Open(string) (io.ReadCloser, error) {
	f.calls.Add(1)
	return nil, os.ErrPermission
}

func (f *deniedMetadataProviderFiles) Stat(string) (fs.FileInfo, error) {
	return nil, os.ErrPermission
}

func assertTerminalReplayParity(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer, id string) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL()+"/worker-sessions/"+id+"/events?replayOnly=true", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("replay HTTP status %d", response.StatusCode)
	}
	frames := []any{}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if !strings.HasPrefix(scanner.Text(), "data:") {
			continue
		}
		var frame any
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "data:"))), &frame); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(frames) < 2 || len(frames) >= 100 {
		t.Fatalf("controlled replay did not contain bounded terminal history: %v", frames)
	}
	for _, limit := range []int{0, 1, 1000} {
		args := map[string]any{"workerSessionId": id, "view": "events"}
		if limit != 0 {
			args["limit"] = limit
		}
		result := callWorker(t, ctx, session, "read", args)["result"].(map[string]any)
		if len(result) != 2 {
			t.Fatalf("events read has extra view: %v", result)
		}
		expected := frames
		if limit == 1 {
			expected = frames[:1]
		}
		assertJSONEqual(t, result["events"], map[string]any{"events": expected, "truncated": limit == 1})
	}
}

func startInterruptHost(t *testing.T, writer recordings.WorkerRecordingWriter) (*support.FunctionalAPIServer, *interruptHostRunner, string) {
	t.Helper()
	home := t.TempDir()
	output := installNativeTranscript(t, home)
	runner := &interruptHostRunner{output: output, started: make(chan platformprocess.CommandRequest, 3), sourceStopped: make(chan struct{})}
	dir := support.ScaffoldSingleStepFactory(t, "mcp-interrupt-host")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir), WorkerRecordingWriter: writer, ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil }},
	})
	return host, runner, dir
}

func admitInterruptSource(t *testing.T, ctx context.Context, host, dir string, runner *interruptHostRunner) {
	t.Helper()
	payload := map[string]any{"requestId": "source-request", "workerSessionId": "source", "execution": map[string]any{
		"workstationName": "direct", "workerType": "direct-worker", "workingDirectory": dir,
		"runnerId": "codex", "executorProvider": "codex", "modelProvider": "codex", "model": "test-model", "userMessage": "initial instruction",
		"dispatch": map[string]any{
			"dispatchId": "source-attempt", "workstationName": "direct", "workerType": "direct-worker",
		},
	}}
	postHostJSON(t, ctx, host+"/worker-sessions", payload, http.StatusAccepted)
	waitProviderRequest(t, ctx, runner)
}

func assertProviderCallCount(t *testing.T, runner *interruptHostRunner, expected int) {
	t.Helper()
	runner.mu.Lock()
	calls := len(runner.requests)
	runner.mu.Unlock()
	if calls != expected {
		t.Fatalf("interrupt retries executed %d provider calls, want %d", calls, expected)
	}
}

func interruptPayload(message string) map[string]any {
	return map[string]any{"requestId": "interrupt-request", "successorWorkerSessionId": "successor", "replacementMessage": message}
}

func runRealHostPartialInterrupt(t *testing.T, process support.Process) {
	t.Helper()
	// A separate immutable recording edge rejects successor opening. This
	// proves the real admission failure path, not durability of accepted data.
	host, runner, dir := startInterruptHost(t, failingSuccessorStore{})
	session, ctx := startMCP(t, process, host.URL())
	admitInterruptSource(t, ctx, host.URL(), dir, runner)
	args := interruptPayload("replace the initial instruction")
	args["workerSessionId"], args["operation"] = "source", "INTERRUPT"
	first := callTool(t, ctx, session, "you.worker_session.control", args)
	assertToolError(t, first, "worker_session.unavailable", true)
	waitControlSignal(t, runner.sourceStopped)
	encoded, err := json.Marshal(first.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	details := envelope["error"].(map[string]any)["details"].(map[string]any)
	if details["phase"] != "SUCCESSOR_ADMISSION" || details["sourceWorkerSessionId"] != "source" || details["successorWorkerSessionId"] != "successor" || details["upstreamCode"] != "WORKER_SESSION_INTERRUPT_SUCCESSOR_ADMISSION_FAILED" {
		t.Fatalf("partial interrupt identity/code: %v", details)
	}
	retry := callTool(t, ctx, session, "you.worker_session.control", args)
	assertToolError(t, retry, "worker_session.unavailable", true)
	assertJSONEqual(t, first.StructuredContent, retry.StructuredContent)
	httpFailure := postHostJSON(t, ctx, host.URL()+"/worker-sessions/source/interrupt", interruptPayload("replace the initial instruction"), http.StatusServiceUnavailable).(map[string]any)
	if httpFailure["phase"] != details["phase"] || httpFailure["code"] != details["upstreamCode"] {
		t.Fatalf("HTTP partial error differs: %v", httpFailure)
	}
	source := getHost(t, host.URL()+"/worker-sessions/source").(map[string]any)
	if source["state"] != "CANCELED" {
		t.Fatalf("partial interrupt source is not stopped: %v", source)
	}
	assertProviderCallCount(t, runner, 1)
	select {
	case req := <-runner.started:
		t.Fatalf("failed successor reached provider: %v", req.Args)
	default:
	}
}

type failingSuccessorStore struct{}

func (failingSuccessorStore) PersistWorkerRecord(_ context.Context, record recordings.WorkerRecordingRecord) error {
	if record.WorkerSessionID == "successor" {
		return fmt.Errorf("controlled successor storage failure")
	}
	return nil
}

func (failingSuccessorStore) LoadWorkerRecording(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
	return recordings.WorkerRecordingSnapshot{}, recordings.ErrWorkerRecordingIncomplete
}

func installNativeTranscript(t *testing.T, home string) []byte {
	t.Helper()
	fixture := filepath.Join(testutil.MustRepoRoot(t), filepath.FromSlash(support.ProviderSessionFixturePath("codex", "success")))
	rollout, err := os.ReadFile(filepath.Join(fixture, "rollout.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-"+interruptProviderID+".jsonl"), rollout, 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(filepath.Join(fixture, "stdout.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func waitProviderRequest(t *testing.T, ctx context.Context, runner *interruptHostRunner) platformprocess.CommandRequest {
	t.Helper()
	select {
	case request := <-runner.started:
		return request
	case <-ctx.Done():
		t.Fatal("provider request barrier not reached")
		return platformprocess.CommandRequest{}
	}
}

func postHostJSON(t *testing.T, ctx context.Context, url string, payload any, status int) any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status %d, want %d: %s", url, response.StatusCode, status, body)
	}
	var result any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertInterruptAdmission(t *testing.T, result map[string]any) {
	t.Helper()
	if result["accepted"] != true || result["requestId"] != "interrupt-request" || result["sourceWorkerSessionId"] != "source" || result["successorWorkerSessionId"] != "successor" || result["phase"] != "SUCCESSOR_ADMISSION" {
		t.Fatalf("interrupt admission: %v", result)
	}
	source := result["source"].(map[string]any)
	successor := result["successor"].(map[string]any)
	if source["workerSessionId"] != "source" || source["state"] != "CANCELED" || successor["workerSessionId"] != "successor" || successor["state"] != "RUNNING" {
		t.Fatalf("interrupt snapshots: %v", result)
	}
}

func assertInterruptCLIParity(t *testing.T, host *support.FunctionalAPIServer, expected any) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "interrupt", "source", "--server", host.URL(), "--json", "--remote", "--async", "--request-id", "interrupt-request", "--successor-worker-session-id", "successor", "--replacement-message", "replace the initial instruction"})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI interrupt: %v stderr=%s", err, inputs.Stderr())
	}
	var actual map[string]any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &actual); err != nil {
		t.Fatal(err)
	}
	// The CLI adds its documented next-command guidance to the same admission.
	if actual["observation"] != "you worker-sessions show --worker-session-id successor" {
		t.Fatalf("CLI observation guidance: %v", actual)
	}
	delete(actual, "observation")
	assertJSONEqual(t, actual, expected)
}

func assertTerminalTranscriptParity(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer) {
	t.Helper()
	result := callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": "source", "view": "transcript"})["result"].(map[string]any)
	assertJSONEqual(t, result["session"], getHost(t, host.URL()+"/worker-sessions/source"))
	transcript := result["transcript"].(map[string]any)
	if transcript["workerSessionId"] != "source" || len(transcript["entries"].([]any)) == 0 {
		t.Fatalf("terminal transcript omitted correlated entries: %v", transcript)
	}
	assertTranscriptEqual(t, transcript, getHost(t, host.URL()+"/worker-sessions/source/transcript"))
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", "source", "--server", host.URL(), "--json"})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI transcript: %v stderr=%s", err, inputs.Stderr())
	}
	var actual any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &actual); err != nil {
		t.Fatal(err)
	}
	assertTranscriptEqual(t, actual, transcript)
}

// Generated optional transcript-entry members permit omitted and null forms.
// Compare their contract values without removing identities, entry order, text
// or errors. Summary and control responses still use exact JSON comparisons.
func assertTranscriptEqual(t *testing.T, actual, expected any) {
	t.Helper()
	values := make([]factoryapi.WorkerSessionTranscriptResponse, 2)
	for index, value := range []any{actual, expected} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &values[index]); err != nil {
			t.Fatal(err)
		}
	}
	assertJSONEqual(t, values[0], values[1])
}
