package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	s8WorkerAID        = "s8-worker-a"
	s8WorkerBID        = "s8-worker-b"
	s8DispatchAID      = "s8-dispatch-a"
	s8DispatchBID      = "s8-dispatch-b"
	s8RequestAID       = "s8-request-a"
	s8RequestBID       = "s8-request-b"
	s8ProviderSessionA = "s8-provider-session-a"
	s8ProviderSessionB = "s8-provider-session-b"
	s8MessageA         = "inspect repository A marker"
	s8MessageB         = "inspect repository B marker"
	s8OutputA          = "S8 repository A provider output"
	s8OutputB          = "S8 repository B provider output"
)

// TestDWROS8ManagerInspectsTwoIsolatedRemoteWorkers proves the first S8
// story through the public manager boundary. The server and manager processes
// both come from root.BuildProcess; only the provider command edge is replaced.
// Each provider command publishes its own Provider Session identity before
// waiting on its repository-specific barrier, which makes overlap and public
// inspection deterministic without sleeps or polling.
func TestDWROS8ManagerInspectsTwoIsolatedRemoteWorkers(t *testing.T) {
	// The context is a bounded deadlock watchdog. Normal readiness comes from
	// provider-edge signals and public CLI stream observations below.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	factoryDir := support.ScaffoldSingleStepFactory(t, "s8-manager-scenario")
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig("codex", "functional-model"))
	repositoryA := newS8Repository(t, "repository-a", "S8_REPOSITORY_A")
	repositoryB := newS8Repository(t, "repository-b", "S8_REPOSITORY_B")
	homeDir := t.TempDir()
	env := s8FunctionalEnvironment(homeDir)

	stdout := readS8ProviderFixture(t, "stdout.jsonl")
	rollout := readS8ProviderFixture(t, "rollout.jsonl")
	runner := newS8RemoteProviderRunner(stdout,
		s8RemoteProviderCase{repository: repositoryA.path, sessionID: s8ProviderSessionA, output: s8OutputA, release: make(chan struct{}), started: make(chan struct{})},
		s8RemoteProviderCase{repository: repositoryB.path, sessionID: s8ProviderSessionB, output: s8OutputB, release: make(chan struct{}), started: make(chan struct{})},
	)
	writeS8CodexRollout(t, homeDir, s8ProviderSessionA, rollout, s8OutputA)
	writeS8CodexRollout(t, homeDir, s8ProviderSessionB, rollout, s8OutputB)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		Env:                       env,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
			ProviderSessionResolveHomeDirectory: func() (string, error) {
				return homeDir, nil
			},
		},
	})

	manager := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: testutil.NewProviderCommandRunner(),
	})
	support.CleanupProcess(t, manager)

	invokeS8RemoteWorker(t, ctx, manager, env, repositoryA.path, server.URL(), s8RemoteWorkerInvocation{
		requestID: s8RequestAID, workerSessionID: s8WorkerAID, dispatchID: s8DispatchAID,
		repository: repositoryA.path, message: s8MessageA,
	})
	runner.waitStarted(t, repositoryA.path)

	invokeS8RemoteWorker(t, ctx, manager, env, repositoryB.path, server.URL(), s8RemoteWorkerInvocation{
		requestID: s8RequestBID, workerSessionID: s8WorkerBID, dispatchID: s8DispatchBID,
		repository: repositoryB.path, message: s8MessageB,
	})
	runner.waitStarted(t, repositoryB.path)

	streamA := startS8LiveStream(t, ctx, manager, env, repositoryA.path, server.URL(), s8WorkerAID)
	streamA.writer.waitFirstWrite(t, s8WorkerAID)
	streamB := startS8LiveStream(t, ctx, manager, env, repositoryB.path, server.URL(), s8WorkerBID)
	streamB.writer.waitFirstWrite(t, s8WorkerBID)

	active := listS8RemoteWorkers(t, ctx, manager, env, factoryDir, server.URL())
	if len(active) != 2 {
		t.Fatalf("active direct Worker Sessions = %d, want two: %#v", len(active), active)
	}
	assertS8ActiveObservation(t, findS8Observation(t, active, s8WorkerAID), s8WorkerAID, s8ProviderSessionA)
	assertS8ActiveObservation(t, findS8Observation(t, active, s8WorkerBID), s8WorkerBID, s8ProviderSessionB)

	showA := showS8RemoteWorker(t, ctx, manager, env, repositoryA.path, server.URL(), s8WorkerAID)
	showB := showS8RemoteWorker(t, ctx, manager, env, repositoryB.path, server.URL(), s8WorkerBID)
	assertS8ActiveObservation(t, showA, s8WorkerAID, s8ProviderSessionA)
	assertS8ActiveObservation(t, showB, s8WorkerBID, s8ProviderSessionB)

	runner.release(t, repositoryA.path)
	runner.release(t, repositoryB.path)
	waitS8Stream(t, streamA, s8WorkerAID)
	waitS8Stream(t, streamB, s8WorkerBID)
	assertS8StreamIsolation(t, streamA.writer.bytes(), s8WorkerAID, s8ProviderSessionA, s8OutputA, s8WorkerBID, repositoryB.path, s8MessageB)
	assertS8StreamIsolation(t, streamB.writer.bytes(), s8WorkerBID, s8ProviderSessionB, s8OutputB, s8WorkerAID, repositoryA.path, s8MessageA)

	retainedA := replayS8RemoteWorker(t, ctx, manager, env, repositoryA.path, server.URL(), s8WorkerAID)
	retainedB := replayS8RemoteWorker(t, ctx, manager, env, repositoryB.path, server.URL(), s8WorkerBID)
	assertS8RetainedStream(t, retainedA, s8WorkerAID, s8ProviderSessionA, s8OutputA, s8WorkerBID, repositoryB.path, s8MessageB)
	assertS8RetainedStream(t, retainedB, s8WorkerBID, s8ProviderSessionB, s8OutputB, s8WorkerAID, repositoryA.path, s8MessageA)

	completed := listS8RemoteWorkers(t, ctx, manager, env, factoryDir, server.URL())
	if len(completed) != 2 {
		t.Fatalf("completed direct Worker Sessions = %d, want two: %#v", len(completed), completed)
	}
	assertS8CompletedObservation(t, findS8Observation(t, completed, s8WorkerAID), s8WorkerAID, s8ProviderSessionA)
	assertS8CompletedObservation(t, findS8Observation(t, completed, s8WorkerBID), s8WorkerBID, s8ProviderSessionB)

	assertS8ProviderRequests(t, runner.requests(), repositoryA, repositoryB)
	server.Stop(t)
}

type s8Repository struct {
	path   string
	marker string
}

func newS8Repository(t *testing.T, name, marker string) s8Repository {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create repository %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(path, "S8_MARKER"), []byte(marker+"\n"), 0o644); err != nil {
		t.Fatalf("write repository %s marker: %v", name, err)
	}
	return s8Repository{path: filepath.Clean(path), marker: marker}
}

type s8RemoteWorkerInvocation struct {
	requestID       string
	workerSessionID string
	dispatchID      string
	repository      string
	message         string
}

func invokeS8RemoteWorker(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL string,
	invocation s8RemoteWorkerInvocation,
) {
	t.Helper()
	document := s8ExecutionDocument(t, invocation)
	inputs := support.FakeInputs(ctx, []string{
		"you", "--remote", "--server", serverURL, "--json", "worker-sessions", "invoke",
		"--request-id", invocation.requestID,
		"--worker-session-id", invocation.workerSessionID,
		"--dispatch-id", invocation.dispatchID,
		"--execution", document,
		"--async",
	})
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("remote invoke %s: %v\nstdout:\n%s\nstderr:\n%s", invocation.workerSessionID, err, inputs.Stdout(), inputs.Stderr())
	}
	var result s8InvokeResult
	decodeS8JSON(t, inputs.Stdout(), &result)
	if !result.Accepted || result.RequestID != invocation.requestID || result.WorkerSessionID != invocation.workerSessionID {
		t.Fatalf("remote invoke %s result = %#v, want accepted exact identity", invocation.workerSessionID, result)
	}
}

func s8ExecutionDocument(t *testing.T, invocation s8RemoteWorkerInvocation) string {
	t.Helper()
	document := struct {
		RequestID       string `json:"requestId"`
		WorkerSessionID string `json:"workerSessionId"`
		Execution       struct {
			WorkstationName string `json:"workstationName"`
			Dispatch        struct {
				DispatchID      string `json:"dispatchId"`
				WorkstationName string `json:"workstationName"`
				WorkerType      string `json:"workerType"`
			} `json:"dispatch"`
			WorkerType               string `json:"workerType"`
			RunnerID                 string `json:"runnerId"`
			ModelProvider            string `json:"modelProvider"`
			Model                    string `json:"model"`
			WorkingDirectory         string `json:"workingDirectory"`
			WorkingDirectoryAuthored bool   `json:"workingDirectoryAuthored"`
			UserMessage              string `json:"userMessage"`
		} `json:"execution"`
	}{RequestID: invocation.requestID, WorkerSessionID: invocation.workerSessionID}
	document.Execution.WorkstationName = workers.ProviderInvocationRoute
	document.Execution.Dispatch.DispatchID = invocation.dispatchID
	document.Execution.Dispatch.WorkstationName = workers.ProviderInvocationRoute
	document.Execution.Dispatch.WorkerType = "processor"
	document.Execution.WorkerType = "processor"
	document.Execution.RunnerID = "codex"
	document.Execution.ModelProvider = "codex"
	document.Execution.Model = "functional-model"
	document.Execution.WorkingDirectory = invocation.repository
	document.Execution.WorkingDirectoryAuthored = true
	document.Execution.UserMessage = invocation.message
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode S8 execution document: %v", err)
	}
	return string(encoded)
}

type s8InvokeResult struct {
	RequestID       string `json:"requestId"`
	WorkerSessionID string `json:"workerSessionId"`
	Accepted        bool   `json:"accepted"`
	State           string `json:"state"`
}

type s8WorkerObservation struct {
	WorkerSessionID          string             `json:"workerSessionId"`
	Direct                   bool               `json:"direct"`
	ProviderSessionAvailable bool               `json:"providerSessionAvailable"`
	ProviderSession          *s8ProviderSession `json:"providerSession"`
	AttemptID                string             `json:"attemptId"`
	State                    string             `json:"state"`
	Transcript               string             `json:"transcript"`
	WorkIDs                  []string           `json:"workIds"`
}

type s8ProviderSession struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

type s8WorkerList struct {
	Sessions []s8WorkerObservation `json:"sessions"`
}

func listS8RemoteWorkers(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL string,
) []s8WorkerObservation {
	t.Helper()
	inputs := executeS8RemoteCLI(t, ctx, process, env, workingDirectory, serverURL,
		"--json", "worker-sessions", "list", "--scope", "direct")
	var result s8WorkerList
	decodeS8JSON(t, inputs.Stdout(), &result)
	return result.Sessions
}

func showS8RemoteWorker(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL, workerSessionID string,
) s8WorkerObservation {
	t.Helper()
	inputs := executeS8RemoteCLI(t, ctx, process, env, workingDirectory, serverURL,
		"--json", "worker-sessions", "show", "--worker-session-id", workerSessionID)
	var result s8WorkerObservation
	decodeS8JSON(t, inputs.Stdout(), &result)
	return result
}

func replayS8RemoteWorker(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL, workerSessionID string,
) []s8StreamFrame {
	t.Helper()
	inputs := executeS8RemoteCLI(t, ctx, process, env, workingDirectory, serverURL,
		"--json", "worker-sessions", "stream", "--worker-session-id", workerSessionID, "--replay-only")
	return decodeS8Stream(t, inputs.Stdout())
}

func executeS8RemoteCLI(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL string,
	args ...string,
) *support.CapturedInputs {
	t.Helper()
	command := append([]string{"you", "--remote", "--server", serverURL}, args...)
	inputs := support.FakeInputs(ctx, command)
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("remote CLI %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(command, " "), err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs
}

func findS8Observation(t *testing.T, observations []s8WorkerObservation, workerSessionID string) s8WorkerObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.WorkerSessionID == workerSessionID {
			return observation
		}
	}
	t.Fatalf("Worker Session %q missing from public list: %#v", workerSessionID, observations)
	return s8WorkerObservation{}
}

func assertS8ActiveObservation(t *testing.T, observation s8WorkerObservation, workerSessionID, providerSessionID string) {
	t.Helper()
	if observation.WorkerSessionID != workerSessionID || !observation.Direct || observation.AttemptID == "" {
		t.Fatalf("active observation = %#v, want direct exact attempt %q", observation, workerSessionID)
	}
	if observation.State != "STARTING" && observation.State != "RUNNING" {
		t.Fatalf("Worker Session %q state = %q, want active state", workerSessionID, observation.State)
	}
	assertS8ProviderSession(t, observation.ProviderSession, observation.ProviderSessionAvailable, providerSessionID)
}

func assertS8CompletedObservation(t *testing.T, observation s8WorkerObservation, workerSessionID, providerSessionID string) {
	t.Helper()
	if observation.WorkerSessionID != workerSessionID || !observation.Direct || observation.State != "COMPLETED" || observation.AttemptID == "" {
		t.Fatalf("completed observation = %#v, want direct completed exact attempt %q", observation, workerSessionID)
	}
	assertS8ProviderSession(t, observation.ProviderSession, observation.ProviderSessionAvailable, providerSessionID)
}

func assertS8ProviderSession(t *testing.T, session *s8ProviderSession, available bool, expectedID string) {
	t.Helper()
	if !available || session == nil || session.Provider != "codex" || session.Kind == "" || session.ID != expectedID {
		t.Fatalf("Provider Session = %#v available=%t, want codex exact %q", session, available, expectedID)
	}
}

type s8StreamFrame struct {
	Delivery        string             `json:"delivery"`
	WorkerSessionID string             `json:"workerSessionId"`
	ProviderSession *s8ProviderSession `json:"providerSession"`
	WorkIDs         []string           `json:"workIds"`
	Event           *s8StreamEvent     `json:"event"`
	ReplaySummary   *s8ReplaySummary   `json:"replaySummary"`
}

type s8StreamEvent struct {
	Position   uint64          `json:"position"`
	SourceType string          `json:"sourceType"`
	Payload    json.RawMessage `json:"payload"`
}

type s8ReplaySummary struct {
	Kind          string `json:"kind"`
	Complete      bool   `json:"complete"`
	Reason        string `json:"reason"`
	EventsEmitted int64  `json:"eventsEmitted"`
}

func decodeS8Stream(t *testing.T, stdout string) []s8StreamFrame {
	t.Helper()
	var frames []s8StreamFrame
	for index, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var frame s8StreamFrame
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode S8 stream frame %d: %v\nline:%s", index+1, err, line)
		}
		if frame.Delivery == "" {
			var summary s8ReplaySummary
			if err := json.Unmarshal([]byte(line), &summary); err != nil {
				t.Fatalf("decode S8 replay summary %d: %v\nline:%s", index+1, err, line)
			}
			frame.ReplaySummary = &summary
		}
		frames = append(frames, frame)
	}
	return frames
}

func assertS8StreamIsolation(
	t *testing.T,
	stdout []byte,
	workerSessionID, providerSessionID, output, foreignWorkerSessionID, foreignRepository, foreignMessage string,
) {
	t.Helper()
	frames := decodeS8Stream(t, string(stdout))
	if len(frames) == 0 {
		t.Fatalf("live stream for %q returned no frames", workerSessionID)
	}
	if !bytes.Contains(stdout, []byte(output)) {
		t.Fatalf("live stream for %q omitted its provider output %q:\n%s", workerSessionID, output, stdout)
	}
	assertS8Frames(t, frames, workerSessionID, providerSessionID, foreignWorkerSessionID, foreignRepository, foreignMessage)
	var terminal bool
	for _, frame := range frames {
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			terminal = true
		}
	}
	if !terminal {
		t.Fatalf("live stream for %q omitted terminal delivery: %#v", workerSessionID, frames)
	}
}

func assertS8RetainedStream(
	t *testing.T,
	frames []s8StreamFrame,
	workerSessionID, providerSessionID, output, foreignWorkerSessionID, foreignRepository, foreignMessage string,
) {
	t.Helper()
	if len(frames) == 0 {
		t.Fatalf("retained stream for %q returned no frames", workerSessionID)
	}
	encoded, err := json.Marshal(frames)
	if err != nil {
		t.Fatalf("encode retained stream for %q: %v", workerSessionID, err)
	}
	if !bytes.Contains(encoded, []byte(output)) {
		t.Fatalf("retained stream for %q omitted provider output %q: %s", workerSessionID, output, encoded)
	}
	assertS8Frames(t, frames, workerSessionID, providerSessionID, foreignWorkerSessionID, foreignRepository, foreignMessage)
	var terminal, complete bool
	for _, frame := range frames {
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			terminal = true
		}
		if frame.ReplaySummary != nil && frame.ReplaySummary.Complete {
			complete = true
		}
	}
	if !terminal || !complete {
		t.Fatalf("retained stream for %q = %#v, want terminal and complete replay summary", workerSessionID, frames)
	}
}

func assertS8Frames(
	t *testing.T,
	frames []s8StreamFrame,
	workerSessionID, providerSessionID, foreignWorkerSessionID, foreignRepository, foreignMessage string,
) {
	t.Helper()
	var previousPosition uint64
	for index, frame := range frames {
		if frame.ReplaySummary != nil {
			continue
		}
		if frame.WorkerSessionID != "" && frame.WorkerSessionID != workerSessionID {
			t.Fatalf("stream frame %d Worker Session = %q, want %q", index, frame.WorkerSessionID, workerSessionID)
		}
		if frame.ProviderSession != nil && (frame.ProviderSession.Provider != "codex" || frame.ProviderSession.ID != providerSessionID) {
			t.Fatalf("stream frame %d Provider Session = %#v, want codex/%q", index, frame.ProviderSession, providerSessionID)
		}
		if frame.Event != nil {
			if frame.Event.Position <= previousPosition {
				t.Fatalf("stream frame positions for %q are not increasing: previous=%d current=%d", workerSessionID, previousPosition, frame.Event.Position)
			}
			previousPosition = frame.Event.Position
			payload := string(frame.Event.Payload)
			for _, foreign := range []string{foreignWorkerSessionID, foreignRepository, foreignMessage} {
				if strings.Contains(payload, foreign) {
					t.Fatalf("stream frame %d for %q contains foreign correlation %q: %s", index, workerSessionID, foreign, payload)
				}
			}
		}
	}
}

type s8StreamCapture struct {
	inputs  *support.CapturedInputs
	writer  *s8SignalWriter
	command *support.ProcessCommand
}

func startS8LiveStream(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL, workerSessionID string,
) s8StreamCapture {
	t.Helper()
	inputs := support.FakeInputs(ctx, []string{
		"you", "--remote", "--server", serverURL, "--json", "worker-sessions", "stream",
		"--worker-session-id", workerSessionID, "--follow",
	})
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	writer := newS8SignalWriter()
	inputs.Input.Stdout = writer
	return s8StreamCapture{inputs: inputs, writer: writer, command: support.StartProcessCommand(t, process, inputs.Input)}
}

func waitS8Stream(t *testing.T, stream s8StreamCapture, workerSessionID string) {
	t.Helper()
	watchdog := time.NewTimer(20 * time.Second)
	defer watchdog.Stop()
	select {
	case <-stream.command.Done():
		if err := stream.command.Err(); err != nil {
			t.Fatalf("live stream for %q: %v\nstdout:\n%s\nstderr:\n%s", workerSessionID, err, stream.writer.bytes(), stream.inputs.Stderr())
		}
	case <-watchdog.C:
		t.Fatalf("deadlock watchdog expired waiting for live stream %q", workerSessionID)
	}
}

type s8SignalWriter struct {
	mu    sync.Mutex
	data  bytes.Buffer
	first chan struct{}
	once  sync.Once
}

func newS8SignalWriter() *s8SignalWriter {
	return &s8SignalWriter{first: make(chan struct{})}
}

func (writer *s8SignalWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	_, _ = writer.data.Write(data)
	writer.mu.Unlock()
	writer.once.Do(func() { close(writer.first) })
	return len(data), nil
}

func (writer *s8SignalWriter) waitFirstWrite(t *testing.T, workerSessionID string) {
	t.Helper()
	watchdog := time.NewTimer(20 * time.Second)
	defer watchdog.Stop()
	select {
	case <-writer.first:
	case <-watchdog.C:
		t.Fatalf("deadlock watchdog expired waiting for public live stream %q", workerSessionID)
	}
}

func (writer *s8SignalWriter) bytes() []byte {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return append([]byte(nil), writer.data.Bytes()...)
}

type s8RemoteProviderCase struct {
	repository  string
	sessionID   string
	output      string
	release     chan struct{}
	started     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
}

type s8RemoteProviderRunner struct {
	mu         sync.Mutex
	cases      []s8RemoteProviderCase
	stdout     []byte
	requestLog []platformprocess.CommandRequest
}

func newS8RemoteProviderRunner(stdout []byte, cases ...s8RemoteProviderCase) *s8RemoteProviderRunner {
	return &s8RemoteProviderRunner{
		cases:  append([]s8RemoteProviderCase(nil), cases...),
		stdout: append([]byte(nil), stdout...),
	}
}

func (runner *s8RemoteProviderRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.run(ctx, request, nil)
}

func (runner *s8RemoteProviderRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	return runner.run(ctx, request, observer)
}

func (runner *s8RemoteProviderRunner) run(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	caseForRequest, err := runner.caseFor(request.WorkDir)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	runner.mu.Lock()
	runner.requestLog = append(runner.requestLog, cloneS8CommandRequest(request))
	runner.mu.Unlock()

	output := bytes.ReplaceAll(runner.stdout, []byte("session_fixture_codex_success"), []byte(caseForRequest.sessionID))
	output = bytes.ReplaceAll(output, []byte("Codex fixture answer COMPLETE"), []byte(caseForRequest.output))
	lineEnd := bytes.IndexByte(output, '\n')
	if lineEnd < 0 {
		lineEnd = len(output)
	} else {
		lineEnd++
	}
	if observer != nil && lineEnd > 0 {
		observer(platformprocess.OutputStreamStdout, append([]byte(nil), output[:lineEnd]...))
	}
	caseForRequest.startOnce.Do(func() { close(caseForRequest.started) })

	select {
	case <-caseForRequest.release:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	if observer != nil && lineEnd < len(output) {
		observer(platformprocess.OutputStreamStdout, append([]byte(nil), output[lineEnd:]...))
	}
	return platformprocess.CommandResult{Stdout: output}, nil
}

func (runner *s8RemoteProviderRunner) caseFor(repository string) (*s8RemoteProviderCase, error) {
	for index := range runner.cases {
		if runner.cases[index].repository == repository {
			return &runner.cases[index], nil
		}
	}
	return nil, fmt.Errorf("unexpected S8 provider working directory %q", repository)
}

func (runner *s8RemoteProviderRunner) waitStarted(t *testing.T, repository string) {
	t.Helper()
	caseForRequest, err := runner.caseFor(repository)
	if err != nil {
		t.Fatal(err)
	}
	watchdog := time.NewTimer(20 * time.Second)
	defer watchdog.Stop()
	select {
	case <-caseForRequest.started:
	case <-watchdog.C:
		t.Fatalf("deadlock watchdog expired waiting for provider command in %q", repository)
	}
}

func (runner *s8RemoteProviderRunner) release(t *testing.T, repository string) {
	t.Helper()
	caseForRequest, err := runner.caseFor(repository)
	if err != nil {
		t.Fatal(err)
	}
	caseForRequest.releaseOnce.Do(func() { close(caseForRequest.release) })
}

func (runner *s8RemoteProviderRunner) requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requestLog))
	for index, request := range runner.requestLog {
		requests[index] = cloneS8CommandRequest(request)
	}
	return requests
}

func (runner *s8RemoteProviderRunner) releaseAll() {
	for index := range runner.cases {
		runner.cases[index].releaseOnce.Do(func() { close(runner.cases[index].release) })
	}
}

func cloneS8CommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func assertS8ProviderRequests(t *testing.T, requests []platformprocess.CommandRequest, repositoryA, repositoryB s8Repository) {
	t.Helper()
	if len(requests) != 2 {
		t.Fatalf("provider command requests = %d, want two: %#v", len(requests), requests)
	}
	want := map[string]struct {
		repository string
		message    string
		foreign    string
	}{
		s8RequestAID: {repository: repositoryA.path, message: s8MessageA, foreign: s8MessageB},
		s8RequestBID: {repository: repositoryB.path, message: s8MessageB, foreign: s8MessageA},
	}
	seenRepositories := make(map[string]bool, len(requests))
	for index, request := range requests {
		if request.Command != "codex" {
			t.Fatalf("provider request %d command = %q, want codex", index, request.Command)
		}
		if request.WorkDir != repositoryA.path && request.WorkDir != repositoryB.path {
			t.Fatalf("provider request %d working directory = %q, want one of the two repositories", index, request.WorkDir)
		}
		seenRepositories[request.WorkDir] = true
		requestText := strings.Join([]string{request.Command, strings.Join(request.Args, " "), string(request.Stdin), strings.Join(request.Env, "\n"), request.WorkDir}, "\n")
		var expected struct {
			repository string
			message    string
			foreign    string
		}
		if request.WorkDir == repositoryA.path {
			expected = want[s8RequestAID]
		} else {
			expected = want[s8RequestBID]
		}
		if !strings.Contains(string(request.Stdin), expected.message) || !strings.Contains(requestText, expected.repository) {
			t.Fatalf("provider request %d = %#v, want repository %q and message %q", index, request, expected.repository, expected.message)
		}
		if strings.Contains(requestText, expected.foreign) {
			t.Fatalf("provider request %d contains foreign message %q: %#v", index, expected.foreign, request)
		}
		foreignRepository := repositoryB.path
		if request.WorkDir == repositoryB.path {
			foreignRepository = repositoryA.path
		}
		if strings.Contains(requestText, foreignRepository) && request.WorkDir != foreignRepository {
			t.Fatalf("provider request %d contains foreign repository %q: %#v", index, foreignRepository, request)
		}
	}
	if !seenRepositories[repositoryA.path] || !seenRepositories[repositoryB.path] {
		t.Fatalf("provider request repositories = %#v, want both distinct repositories", seenRepositories)
	}
}

func readS8ProviderFixture(t *testing.T, fileName string) []byte {
	t.Helper()
	path := filepath.Join(testutil.MustRepoRoot(t), filepath.FromSlash(support.ProviderSessionFixturePath("codex", "success", fileName)))
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read S8 provider fixture %s: %v", fileName, err)
	}
	return contents
}

func writeS8CodexRollout(t *testing.T, homeDir, sessionID string, contents []byte, output string) {
	t.Helper()
	directory := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create S8 Codex session directory: %v", err)
	}
	contents = bytes.ReplaceAll(contents, []byte("Codex fixture answer COMPLETE"), []byte(output))
	path := filepath.Join(directory, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write S8 Codex rollout fixture: %v", err)
	}
}

func s8FunctionalEnvironment(homeDir string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return env
}

func decodeS8JSON(t *testing.T, stdout string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), target); err != nil {
		t.Fatalf("decode S8 public CLI JSON: %v\nstdout:\n%s", err, stdout)
	}
}

var _ platformprocess.CommandRunner = (*s8RemoteProviderRunner)(nil)
