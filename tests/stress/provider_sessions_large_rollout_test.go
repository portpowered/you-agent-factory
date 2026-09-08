package stress_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	largeStressRolloutProviderSessionID = "large-rollout-stress-provider-session"
	largeStressRolloutTargetBytes       = int64(256 << 20)
	largeStressRolloutMaxBytes          = int64(384 << 20)
	largeStressRolloutPaddingBytes      = 256 << 10
	largeStressRolloutChunkSize         = 64 << 10
	largeStressRolloutTimeout           = 5 * time.Minute
	largeStressRolloutDiskBudget        = int64(1 << 30)
)

type largeStressProcess interface {
	Execute(root.Input) error
}

type largeStressClosableProcess interface {
	largeStressProcess
	Close(context.Context) error
}

type largeStressInputs struct {
	Input  root.Input
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func newLargeStressInputs(ctx context.Context, args []string) *largeStressInputs {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return &largeStressInputs{
		Input: root.Input{
			Args: args, Context: ctx, Stdout: stdout, Stderr: stderr,
		},
		stdout: stdout,
		stderr: stderr,
	}
}

func (inputs *largeStressInputs) Stdout() string {
	return inputs.stdout.String()
}

func (inputs *largeStressInputs) Stderr() string {
	return inputs.stderr.String()
}

func buildLargeStressProcess(t *testing.T, edges serviceedges.Edges) largeStressClosableProcess {
	t.Helper()
	process, err := root.BuildProcess(context.Background(), edges)
	if err != nil {
		t.Fatalf("build large rollout stress process: %v", err)
	}
	t.Cleanup(func() {
		closeContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := process.Close(closeContext); err != nil {
			t.Errorf("close large rollout stress process: %v", err)
		}
	})
	return process
}

type largeStressProcessCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func startLargeStressProcessCommand(
	t *testing.T,
	process largeStressProcess,
	input root.Input,
) *largeStressProcessCommand {
	t.Helper()
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &largeStressProcessCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		command.mu.Lock()
		command.err = process.Execute(input)
		command.mu.Unlock()
		close(command.done)
	}()
	t.Cleanup(func() { command.Stop(t) })
	return command
}

func (command *largeStressProcessCommand) Stop(t *testing.T) {
	t.Helper()
	if command == nil {
		return
	}
	command.cancel()
	select {
	case <-command.done:
		command.mu.Lock()
		err := command.err
		command.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("large rollout stress Process.Execute shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Errorf("timed out waiting for large rollout stress Process.Execute shutdown")
	}
}

type largeStressAPIServer struct {
	ready chan struct{}
	once  sync.Once

	mu     sync.Mutex
	server *httptest.Server
}

func newLargeStressAPIServer() *largeStressAPIServer {
	return &largeStressAPIServer{ready: make(chan struct{})}
}

func (server *largeStressAPIServer) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	started := httptest.NewServer(request.Handler)
	server.mu.Lock()
	server.server = started
	server.mu.Unlock()
	server.once.Do(func() { close(server.ready) })
	if request.OnBound != nil {
		request.OnBound(platformhttpserver.Binding{Host: "127.0.0.1", Port: 0})
	}
	<-ctx.Done()
	started.Close()
	return ctx.Err()
}

func (server *largeStressAPIServer) WaitURL(t *testing.T) string {
	t.Helper()
	select {
	case <-server.ready:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for large rollout stress API server")
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.server == nil {
		t.Fatal("large rollout stress API server has no bound server")
	}
	return server.server.URL
}

func largeStressLegacyFixtureDir(t *testing.T) string {
	t.Helper()
	return testutil.MustRepoPath(t, filepath.Join("tests", "functional_test", "testdata", "executor_success"))
}

func writeLargeStressAgentConfig(t *testing.T, factoryDir string) {
	t.Helper()
	content := fmt.Sprintf(`---
type: MODEL_WORKER
model: fixture-model
modelProvider: %s
stopToken: COMPLETE
---
Process the input task.
`, modelprovider.ProviderCodex)
	path := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create large rollout stress Worker directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write large rollout stress Worker config: %v", err)
	}
}

func postLargeStressSession(
	t *testing.T,
	baseURL, factoryDir string,
) factoryapi.OpenFactorySessionResponse {
	t.Helper()
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: factoryDir})
	if err != nil {
		t.Fatalf("marshal large rollout stress session request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST large rollout stress Factory Session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST large rollout stress Factory Session status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(response.Body).Decode(&opened); err != nil {
		t.Fatalf("decode large rollout stress Factory Session: %v", err)
	}
	return opened
}

func getLargeStressJSON[T any](t *testing.T, endpoint string) T {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var value T
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatalf("decode GET %s: %v", endpoint, err)
	}
	return value
}

func largeStressSessionWorkURL(baseURL, sessionID, path string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + path
}

func largeStressSessionEventsURL(baseURL, sessionID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/events"
}

func countLargeStressWorkAtState(listed factoryapi.ListWorkResponse, workType, state string) int {
	count := 0
	for _, item := range listed.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != state {
			continue
		}
		count++
	}
	return count
}

func closeLargeStressSession(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	control := terminateLargeStressSession(t, baseURL, sessionID)
	if control.Outcome == factoryapi.FactorySessionLifecycleControlOutcomeAccepted &&
		control.Status != factoryapi.FactorySessionDurableLifecycleStatusTerminated {
		eventStream := openLargeStressEventStream(t, largeStressSessionEventsURL(baseURL, sessionID))
		defer eventStream.Close()
		waitLargeStressSessionTerminated(t, eventStream, sessionID)
	}

	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	request, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		t.Fatalf("build DELETE large rollout stress Factory Session: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("DELETE large rollout stress Factory Session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("DELETE large rollout stress Factory Session status = %d, want 204: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func terminateLargeStressSession(t *testing.T, baseURL, sessionID string) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	payload, err := json.Marshal(factoryapi.FactorySessionLifecycleControlRequest{})
	if err != nil {
		t.Fatalf("marshal terminate large rollout stress Factory Session: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/terminate"
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build terminate large rollout stress Factory Session: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST terminate large rollout stress Factory Session: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read terminate large rollout stress Factory Session response: %v", err)
	}
	var control factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(body, &control); err != nil {
		t.Fatalf("decode terminate large rollout stress Factory Session response: %v\nbody:\n%s", err, strings.TrimSpace(string(body)))
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return control
	}
	if response.StatusCode == http.StatusConflict && control.Outcome == factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		return control
	}
	t.Fatalf("terminate large rollout stress Factory Session status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	return factoryapi.FactorySessionLifecycleControlResponse{}
}

func waitLargeStressSessionTerminated(t *testing.T, eventStream *largeStressEventStream, sessionID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		// The public Factory Event stream is the synchronization primitive. The
		// deadline only bounds missing lifecycle evidence; it is not a poll or
		// quiet-period success path.
		event := eventStream.NextEventContext(ctx)
		if event.Type == factoryapi.FactoryEventTypeSessionCompleted {
			if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
				t.Fatalf("large rollout stress lifecycle event session = %#v, want %q", event.Context.SessionId, sessionID)
			}
			return
		}
		if event.Type != factoryapi.FactoryEventTypeSessionLifecycleControl {
			continue
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			t.Fatalf("large rollout stress lifecycle event session = %#v, want %q", event.Context.SessionId, sessionID)
		}
		payload, err := event.Payload.AsSessionLifecycleControlEventPayload()
		if err != nil {
			t.Fatalf("decode large rollout stress lifecycle-control event: %v", err)
		}
		if payload.Operation != factoryapi.FactorySessionLifecycleControlKindTerminate {
			continue
		}
		if payload.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
			payload.NewStatus != factoryapi.FactorySessionDurableLifecycleStatusTerminated {
			t.Fatalf(
				"large rollout stress termination event = operation:%q outcome:%q newStatus:%q, want accepted TERMINATED",
				payload.Operation,
				payload.Outcome,
				payload.NewStatus,
			)
		}
		return
	}
}

// TestLargeRolloutStress keeps the capacity-sensitive rollout witness in the
// stress layer. Its only subprocess is the test helper started by the command
// edge; application construction and customer actions still use the real root
// process and public Factory Session boundaries.
func TestLargeRolloutStress(t *testing.T) {
	started := time.Now()
	rolloutPath, expectedBytes := writeLargeStressRollout(t)
	assertLargeStressRolloutBounds(t, rolloutPath, expectedBytes)

	factoryDir := testutil.CopyFixtureDir(t, largeStressLegacyFixtureDir(t))
	homeDir := t.TempDir()
	writeLargeStressAgentConfig(t, factoryDir)
	runner := newLargeStressRolloutRunner(t, rolloutPath, expectedBytes)
	server := newLargeStressAPIServer()
	process := buildLargeStressProcess(t, serviceedges.Edges{
		APIServerStarter:                    server.Start,
		ProviderCommandRunner:               runner,
		FactorySessionResolveHomeDirectory:  func() (string, error) { return homeDir, nil },
		ProviderSessionResolveHomeDirectory: func() (string, error) { return homeDir, nil },
	})
	inputs := newLargeStressInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = largeStressRolloutEnvironment(homeDir)
	inputs.Input.WorkingDirectory = factoryDir
	daemon := startLargeStressProcessCommand(t, process, inputs.Input)
	baseURL := server.WaitURL(t)

	ctx, cancel := context.WithTimeout(t.Context(), largeStressRolloutTimeout)
	defer cancel()
	sessionID := openLargeStressSession(t, baseURL, factoryDir)
	sessionClosed := false
	t.Cleanup(func() {
		if !sessionClosed {
			closeLargeStressSession(t, baseURL, sessionID)
		}
	})
	eventStream := openLargeStressEventStream(t, largeStressSessionEventsURL(baseURL, sessionID))
	t.Cleanup(eventStream.Close)
	workID := submitLargeStressWork(t, ctx, process, inputs.Input.Env, factoryDir, baseURL, sessionID)
	events := readLargeStressEvents(t, ctx, eventStream, workID, sessionID)
	assertLargeStressPublicState(t, baseURL, sessionID, workID, events)

	closeLargeStressSession(t, baseURL, sessionID)
	sessionClosed = true
	assertLargeStressSessionAbsent(t, baseURL, sessionID)
	daemon.Stop(t)
	assertLargeStressRunner(t, runner, expectedBytes)
	removeLargeStressRollout(t, rolloutPath)

	elapsed := time.Since(started)
	t.Logf("large rollout stress generated=%d observed=%d helperCalls=%d elapsed=%s diskBudget=%d", expectedBytes, runner.observed.Load(), runner.calls.Load(), elapsed, largeStressRolloutDiskBudget)
	if elapsed > largeStressRolloutTimeout {
		t.Fatalf("large rollout stress elapsed time = %s, want <= %s", elapsed, largeStressRolloutTimeout)
	}
}

func writeLargeStressRollout(t *testing.T) (string, int64) {
	t.Helper()
	rolloutDir := t.TempDir()
	path := filepath.Join(rolloutDir, "large-rollout-stress.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create large rollout stress file: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriterSize(file, largeStressRolloutChunkSize)
	written := int64(0)
	write := func(record []byte) {
		t.Helper()
		if _, err := writer.Write(record); err != nil {
			t.Fatalf("write large rollout stress record: %v", err)
		}
		written += int64(len(record))
	}

	write([]byte(`{"type":"thread.started","thread_id":"` + largeStressRolloutProviderSessionID + `"}` + "\n"))
	filler, err := json.Marshal(map[string]any{
		"type": "item.updated",
		"item": map[string]any{
			"id":   "large-rollout-stress-progress",
			"type": "agent_message",
			"text": strings.Repeat("x", largeStressRolloutPaddingBytes),
		},
	})
	if err != nil {
		t.Fatalf("marshal large rollout stress record: %v", err)
	}
	filler = append(filler, '\n')
	for written < largeStressRolloutTargetBytes {
		write(filler)
	}
	write([]byte(`{"type":"item.completed","item":{"id":"large-rollout-stress-final","type":"agent_message","text":"large rollout completed with authoritative Codex evidence COMPLETE"}}` + "\n"))

	if err := writer.Flush(); err != nil {
		t.Fatalf("flush large rollout stress file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close large rollout stress file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat large rollout stress file: %v", err)
	}
	return path, info.Size()
}

func assertLargeStressRolloutBounds(t *testing.T, path string, size int64) {
	t.Helper()
	if size < largeStressRolloutTargetBytes || size > largeStressRolloutMaxBytes {
		t.Fatalf(
			"large rollout stress file size = %d, want between %d and %d bytes",
			size,
			largeStressRolloutTargetBytes,
			largeStressRolloutMaxBytes,
		)
	}
	if size > largeStressRolloutDiskBudget {
		t.Fatalf("large rollout stress file size = %d, exceeds %d-byte disk budget", size, largeStressRolloutDiskBudget)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("large rollout stress file disappeared before execution: %v", err)
	}
}

type largeStressRolloutRunner struct {
	path     string
	expected int64
	calls    atomic.Int32
	active   atomic.Int32
	observed atomic.Int64
}

func newLargeStressRolloutRunner(t *testing.T, path string, expected int64) *largeStressRolloutRunner {
	t.Helper()
	return &largeStressRolloutRunner{path: path, expected: expected}
}

func (runner *largeStressRolloutRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("large rollout stress runner requires streaming")
}

func (runner *largeStressRolloutRunner) RunStreaming(
	ctx context.Context,
	_ platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if observer == nil {
		return platformprocess.CommandResult{}, errors.New("large rollout stress runner requires an output observer")
	}
	runner.calls.Add(1)
	runner.active.Add(1)
	defer runner.active.Add(-1)

	execRunner, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil, nil)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	observing := func(stream string, chunk []byte) {
		if stream == platformprocess.OutputStreamStdout {
			runner.observed.Add(int64(len(chunk)))
		}
		observer(stream, chunk)
	}
	result, err := execRunner.RunStreaming(ctx, platformprocess.CommandRequest{
		Command: os.Args[0],
		Args: []string{
			"-test.run=TestLargeStressRolloutHelperProcess",
			"--",
			"stream",
		},
		Env: append(
			os.Environ(),
			"GO_WANT_LARGE_STRESS_ROLLOUT_HELPER=1",
			"LARGE_STRESS_ROLLOUT_PATH="+runner.path,
		),
	}, observing)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	return result, nil
}

func TestLargeStressRolloutHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_LARGE_STRESS_ROLLOUT_HELPER") != "1" {
		return
	}
	path := os.Getenv("LARGE_STRESS_ROLLOUT_PATH")
	if path == "" {
		t.Fatal("missing LARGE_STRESS_ROLLOUT_PATH")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open large rollout stress file: %v", err)
	}
	defer file.Close()
	if _, err := io.Copy(os.Stdout, file); err != nil {
		t.Fatalf("stream large rollout stress file: %v", err)
	}
	os.Exit(0)
}

func openLargeStressSession(t *testing.T, baseURL, factoryDir string) string {
	t.Helper()
	opened := postLargeStressSession(t, baseURL, factoryDir)
	if opened.Session == nil || opened.Session.Id == "" || opened.Session.IsDefault {
		t.Fatalf("large rollout stress Factory Session = %#v, want explicit non-default identity", opened.Session)
	}
	return opened.Session.Id
}

func submitLargeStressWork(
	t *testing.T,
	ctx context.Context,
	process largeStressProcess,
	env []string,
	factoryDir, baseURL, sessionID string,
) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"requestId": "large-rollout-stress-request",
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []any{map[string]any{
			"name":         "large-rollout-stress-work",
			"workTypeName": "task",
			"payload":      map[string]string{"title": "large rollout stress"},
		}},
	})
	if err != nil {
		t.Fatalf("marshal large rollout stress Work: %v", err)
	}
	inputs := executeLargeStressCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "--json", "submit", "batch", "--session", sessionID, string(payload))
	var response struct {
		WorkCount int `json:"workCount"`
		Works     []struct {
			WorkID string `json:"workId"`
		} `json:"works"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); err != nil {
		t.Fatalf("decode large rollout stress submit response: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if response.WorkCount != 1 || len(response.Works) != 1 || strings.TrimSpace(response.Works[0].WorkID) == "" {
		t.Fatalf("large rollout stress submit response = %#v, want one Work\nstdout:\n%s", response, inputs.Stdout())
	}
	return response.Works[0].WorkID
}

func executeLargeStressCLI(
	t *testing.T,
	ctx context.Context,
	process largeStressProcess,
	env []string,
	workingDirectory string,
	args ...string,
) *largeStressInputs {
	t.Helper()
	inputs := newLargeStressInputs(ctx, append([]string{"you"}, args...))
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("you %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs
}

type largeStressEventStream struct {
	t      *testing.T
	cancel context.CancelFunc
	done   chan struct{}
	events chan factoryapi.FactoryEvent
	errs   chan error
}

func openLargeStressEventStream(t *testing.T, endpoint string) *largeStressEventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("build large rollout stress event stream request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		t.Fatalf("GET large rollout stress event stream: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		cancel()
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET large rollout stress event stream status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	stream := &largeStressEventStream{
		t:      t,
		cancel: cancel,
		done:   make(chan struct{}),
		events: make(chan factoryapi.FactoryEvent, 4096),
		errs:   make(chan error, 1),
	}
	go stream.read(response)
	return stream
}

func (stream *largeStressEventStream) read(response *http.Response) {
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	// The terminal dispatch event carries the deliberately large rollout
	// output, so the SSE data line is bounded by the rollout fixture rather
	// than the Scanner default or a small fixed token cap.
	scanner.Buffer(make([]byte, 64<<10), int(largeStressRolloutMaxBytes+16<<20))
	var dataLines []string
	defer close(stream.done)
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
			select {
			case stream.errs <- fmt.Errorf("decode large rollout stress Factory Event: %w", err):
			default:
			}
			dataLines = nil
			return
		}
		select {
		case stream.events <- event:
		default:
			select {
			case stream.errs <- errors.New("large rollout stress Factory Event stream buffer overflow"):
			default:
			}
		}
		dataLines = nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		select {
		case stream.errs <- err:
		default:
		}
	}
}

func (stream *largeStressEventStream) NextEventContext(ctx context.Context) factoryapi.FactoryEvent {
	// Prefer events already accepted from the public stream before observing its
	// terminal read state. The server can close after publishing the final
	// frames, and those buffered events remain the authoritative evidence.
	select {
	case event := <-stream.events:
		return event
	default:
	}

	select {
	case event := <-stream.events:
		return event
	case err := <-stream.errs:
		stream.t.Fatalf("large rollout stress Factory Event stream: %v", err)
	case <-stream.done:
		select {
		case event := <-stream.events:
			return event
		case err := <-stream.errs:
			stream.t.Fatalf("large rollout stress Factory Event stream: %v", err)
		default:
		}
		stream.t.Fatal("large rollout stress Factory Event stream closed before terminal response")
	case <-ctx.Done():
		stream.t.Fatalf("waiting for large rollout stress Factory Event: %v", ctx.Err())
	}
	return factoryapi.FactoryEvent{}
}

func (stream *largeStressEventStream) Close() {
	if stream == nil {
		return
	}
	stream.cancel()
	select {
	case <-stream.done:
	case <-time.After(2 * time.Second):
	}
}

func readLargeStressEvents(
	t *testing.T,
	ctx context.Context,
	stream *largeStressEventStream,
	workID, sessionID string,
) []factoryapi.FactoryEvent {
	t.Helper()
	events := make([]factoryapi.FactoryEvent, 0, 16)
	for {
		event := stream.NextEventContext(ctx)
		if event.Id == "" {
			t.Fatalf("large rollout stress emitted Factory Event without an ID: %#v", event)
		}
		if largeStressEventIncludesWork(event, workID) && (event.Context.SessionId == nil || *event.Context.SessionId != sessionID) {
			t.Fatalf("large rollout stress event session = %#v, want %q", event.Context.SessionId, sessionID)
		}
		events = append(events, event)
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse && largeStressEventIncludesWork(event, workID) {
			return events
		}
	}
}

func largeStressEventIncludesWork(event factoryapi.FactoryEvent, workID string) bool {
	if event.Context.WorkIds == nil {
		return false
	}
	for _, candidate := range *event.Context.WorkIds {
		if candidate == workID {
			return true
		}
	}
	return false
}

func assertLargeStressPublicState(
	t *testing.T,
	baseURL, sessionID, workID string,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	work := getLargeStressJSON[factoryapi.ListWorkResponse](t, largeStressSessionWorkURL(baseURL, sessionID, "/work"))
	if len(work.Results) != 1 || work.Results[0].WorkId == nil || *work.Results[0].WorkId != workID {
		t.Fatalf("large rollout stress public Work = %#v, want exactly %q", work.Results, workID)
	}
	if got := countLargeStressWorkAtState(work, "task", "done"); got != 1 {
		t.Fatalf("large rollout stress done Work count = %d, want 1", got)
	}
	if got := countLargeStressWorkAtState(work, "task", "failed"); got != 0 {
		t.Fatalf("large rollout stress failed Work count = %d, want 0", got)
	}
	session := getLargeStressSession(t, baseURL, sessionID)
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 ||
		session.Runtime.Progress.Categories.Initial != 0 || session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf("large rollout stress session progress = %+v, want one terminal and no active or failed Work", session.Runtime.Progress.Categories)
	}
	assertLargeStressEvents(t, events, workID)
}

func getLargeStressSession(t *testing.T, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	response := getLargeStressJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode large rollout stress Factory Session: %v", err)
	}
	return session
}

func largeStressInferenceObservation(event factoryapi.FactoryEvent) (factoryapi.InferenceResponseEventPayload, error) {
	if event.Type == factoryapi.FactoryEventTypeInferenceResponse {
		return event.Payload.AsInferenceResponseEventPayload()
	}
	if event.Type != factoryapi.FactoryEventTypeModelResponse {
		return factoryapi.InferenceResponseEventPayload{}, fmt.Errorf("event type %q is not a provider response", event.Type)
	}
	modelResponse, err := event.Payload.AsModelResponseEventPayload()
	if err != nil {
		return factoryapi.InferenceResponseEventPayload{}, err
	}
	response := modelResponse.OutputPreview
	if response == nil && modelResponse.OutputContent != nil {
		var textParts []string
		for _, part := range *modelResponse.OutputContent {
			textPart, textErr := part.AsWorkTextContentPart()
			if textErr == nil {
				textParts = append(textParts, textPart.Text)
			}
		}
		if len(textParts) > 0 {
			joined := strings.Join(textParts, "")
			response = &joined
		}
	}
	return factoryapi.InferenceResponseEventPayload{
		Attempt:            modelResponse.Attempt,
		Diagnostics:        modelResponse.Diagnostics,
		DurationMillis:     modelResponse.DurationMillis,
		FailureDetail:      modelResponse.FailureDetail,
		InferenceRequestId: modelResponse.ModelRequestId,
		Outcome:            modelResponse.Outcome,
		ProviderSession:    modelResponse.ProviderSession,
		Response:           response,
	}, nil
}

func assertLargeStressEvents(t *testing.T, events []factoryapi.FactoryEvent, workID string) {
	t.Helper()
	seenEventIDs := make(map[string]struct{}, len(events))
	var dispatch *factoryapi.DispatchResponseEventPayload
	var inference *factoryapi.InferenceResponseEventPayload
	for _, event := range events {
		if _, exists := seenEventIDs[event.Id]; exists {
			t.Fatalf("large rollout stress repeated Factory Event ID %q", event.Id)
		}
		seenEventIDs[event.Id] = struct{}{}
		if !largeStressEventIncludesWork(event, workID) {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			if dispatch != nil {
				t.Fatalf("large rollout stress emitted multiple dispatch responses")
			}
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode large rollout stress dispatch response: %v", err)
			}
			dispatch = &payload
		case factoryapi.FactoryEventTypeInferenceResponse, factoryapi.FactoryEventTypeModelResponse:
			if inference != nil {
				t.Fatalf("large rollout stress emitted multiple inference responses")
			}
			payload, err := largeStressInferenceObservation(event)
			if err != nil {
				t.Fatalf("decode large rollout stress inference response: %v", err)
			}
			inference = &payload
		}
	}
	if dispatch == nil || inference == nil {
		t.Fatalf("large rollout stress terminal events = dispatch:%t inference:%t, want both", dispatch != nil, inference != nil)
	}
	if dispatch.Outcome != factoryapi.WorkOutcomeAccepted || dispatch.Output == nil ||
		!strings.Contains(*dispatch.Output, "authoritative Codex evidence COMPLETE") || dispatch.FailureDetail != nil {
		t.Fatalf("large rollout stress dispatch response = %#v, want accepted output", dispatch)
	}
	if inference.Outcome != factoryapi.InferenceOutcomeSucceeded || inference.ProviderSession == nil ||
		inference.ProviderSession.Id == nil || *inference.ProviderSession.Id != largeStressRolloutProviderSessionID ||
		inference.Response == nil || !strings.Contains(*inference.Response, "authoritative Codex evidence COMPLETE") {
		t.Fatalf("large rollout stress inference response = %#v, want succeeded provider identity and final response", inference)
	}
}

func assertLargeStressSessionAbsent(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("probe closed large rollout stress Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("closed large rollout stress Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func assertLargeStressRunner(t *testing.T, runner *largeStressRolloutRunner, expectedBytes int64) {
	t.Helper()
	if got := runner.calls.Load(); got != 1 {
		t.Fatalf("large rollout stress helper calls = %d, want exactly one", got)
	}
	if got := runner.active.Load(); got != 0 {
		t.Fatalf("large rollout stress active helper calls = %d, want zero", got)
	}
	if got := runner.observed.Load(); got != expectedBytes {
		t.Fatalf("large rollout stress observed stdout bytes = %d, want %d", got, expectedBytes)
	}
}

func removeLargeStressRollout(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove large rollout stress file: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("large rollout stress file stat after cleanup = %v, want not exist", err)
	}
}

func largeStressRolloutEnvironment(homeDir string) []string {
	env := append([]string(nil), os.Environ()...)
	return append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

var _ platformprocess.CommandRunner = (*largeStressRolloutRunner)(nil)
