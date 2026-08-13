package acceptance

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	s8WorkerASuccessorID = "s8-worker-a-replacement"
	s8InterruptRequestID = "s8-interrupt-request"
	s8ReplacementMessage = "replace repository A instruction only"
	s8ReplacementOutput  = "S8 repository A replacement provider output"
)

type s8InterruptScenario struct {
	ctx         context.Context
	manager     support.Process
	env         []string
	factoryDir  string
	serverURL   string
	repositoryA s8Repository
	repositoryB s8Repository
	runner      *s8InterruptProviderRunner
}

// TestDWROS8ManagerInterruptsOnlyOneRemoteWorker proves the second S8 story
// through the public manager boundary. A's source, A's resumed successor, and
// B each have an explicit provider-edge barrier. The only readiness signals
// used below are those edge barriers and public stream/list observations; the
// deadline is a bounded deadlock watchdog for a broken lifecycle.
func TestDWROS8ManagerInterruptsOnlyOneRemoteWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	factoryDir := support.ScaffoldSingleStepFactory(t, "s8-manager-interrupt-scenario")
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig("codex", "functional-model"))
	repositoryA := newS8Repository(t, "repository-a", "S8_REPOSITORY_A")
	repositoryB := newS8Repository(t, "repository-b", "S8_REPOSITORY_B")
	homeDir := t.TempDir()
	env := s8FunctionalEnvironment(homeDir)

	stdout := readS8ProviderFixture(t, "stdout.jsonl")
	rollout := readS8ProviderFixture(t, "rollout.jsonl")
	runner := newS8InterruptProviderRunner(stdout, repositoryA, repositoryB)
	defer runner.releaseAll()
	writeS8CodexRollout(t, homeDir, s8ProviderSessionA, rollout, s8ReplacementOutput)
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
	scenario := s8InterruptScenario{
		ctx: ctx, manager: manager, env: env, factoryDir: factoryDir, serverURL: server.URL(),
		repositoryA: repositoryA, repositoryB: repositoryB, runner: runner,
	}

	invokeS8RemoteWorker(t, ctx, manager, env, repositoryA.path, server.URL(), s8RemoteWorkerInvocation{
		requestID: s8RequestAID, workerSessionID: s8WorkerAID, dispatchID: s8DispatchAID,
		repository: repositoryA.path, message: s8MessageA,
	})
	runner.waitStarted(t, repositoryA.path, s8InterruptCallAInitial)

	invokeS8RemoteWorker(t, ctx, manager, env, repositoryB.path, server.URL(), s8RemoteWorkerInvocation{
		requestID: s8RequestBID, workerSessionID: s8WorkerBID, dispatchID: s8DispatchBID,
		repository: repositoryB.path, message: s8MessageB,
	})
	runner.waitStarted(t, repositoryB.path, s8InterruptCallBInitial)

	streamA := startS8LiveStream(t, ctx, manager, env, repositoryA.path, server.URL(), s8WorkerAID)
	streamA.writer.waitFirstWrite(t, s8WorkerAID)
	streamB := startS8LiveStream(t, ctx, manager, env, repositoryB.path, server.URL(), s8WorkerBID)
	streamB.writer.waitFirstWrite(t, s8WorkerBID)

	active := listS8RemoteWorkers(t, ctx, manager, env, factoryDir, server.URL())
	if len(active) != 2 {
		t.Fatalf("active direct Worker Sessions = %d, want two: %#v", len(active), active)
	}
	assertS8Observation(t, findS8Observation(t, active, s8WorkerAID), s8WorkerAID, "RUNNING", s8ProviderSessionA, s8DispatchAID)
	assertS8Observation(t, findS8Observation(t, active, s8WorkerBID), s8WorkerBID, "RUNNING", s8ProviderSessionB, s8DispatchBID)

	successor, streamSuccessor := assertS8InterruptOverlap(t, scenario, streamA, streamB)
	finishS8InterruptScenario(t, scenario, streamA, streamB, streamSuccessor, successor)
	server.Stop(t)
}

func assertS8InterruptOverlap(
	t *testing.T,
	scenario s8InterruptScenario,
	streamA, streamB s8StreamCapture,
) (s8WorkerObservation, s8StreamCapture) {
	t.Helper()
	interrupt := interruptS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL)
	assertS8InterruptAdmission(t, interrupt)
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	scenario.runner.assertOrder(t, "start:"+s8InterruptCallAInitial, "cancel:"+s8InterruptCallAInitial, "start:"+s8InterruptCallASuccessor)

	streamSuccessor := startS8LiveStream(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8WorkerASuccessorID)
	streamSuccessor.writer.waitFirstWrite(t, s8WorkerASuccessorID)
	overlap := listS8RemoteWorkers(t, scenario.ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL)
	if len(overlap) != 3 {
		t.Fatalf("overlap direct Worker Sessions = %d, want source, successor, and B: %#v", len(overlap), overlap)
	}
	assertS8Observation(t, findS8Observation(t, overlap, s8WorkerAID), s8WorkerAID, "CANCELED", s8ProviderSessionA, s8DispatchAID)
	successor := findS8Observation(t, overlap, s8WorkerASuccessorID)
	assertS8Observation(t, successor, s8WorkerASuccessorID, "RUNNING", s8ProviderSessionA, s8DispatchAID+"/continue/"+s8WorkerASuccessorID)
	assertS8Observation(t, findS8Observation(t, overlap, s8WorkerBID), s8WorkerBID, "RUNNING", s8ProviderSessionB, s8DispatchBID)

	showSource := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8WorkerAID)
	showSuccessor := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8WorkerASuccessorID)
	showB := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, s8WorkerBID)
	assertS8Observation(t, showSource, s8WorkerAID, "CANCELED", s8ProviderSessionA, s8DispatchAID)
	assertS8Observation(t, showSuccessor, s8WorkerASuccessorID, "RUNNING", s8ProviderSessionA, successor.AttemptID)
	assertS8Observation(t, showB, s8WorkerBID, "RUNNING", s8ProviderSessionB, s8DispatchBID)
	if scenario.runner.cancellationCount(s8InterruptCallBInitial) != 0 {
		t.Fatalf("Worker B provider cancellations = %d, want zero while B remains active", scenario.runner.cancellationCount(s8InterruptCallBInitial))
	}
	return successor, streamSuccessor
}

func finishS8InterruptScenario(
	t *testing.T,
	scenario s8InterruptScenario,
	streamA, streamB, streamSuccessor s8StreamCapture,
	successor s8WorkerObservation,
) {
	t.Helper()
	scenario.runner.release(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	waitS8Stream(t, streamSuccessor, s8WorkerASuccessorID)
	stillActiveB := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, s8WorkerBID)
	assertS8Observation(t, stillActiveB, s8WorkerBID, "RUNNING", s8ProviderSessionB, s8DispatchBID)

	scenario.runner.release(t, scenario.repositoryB.path, s8InterruptCallBInitial)
	waitS8Stream(t, streamA, s8WorkerAID)
	waitS8Stream(t, streamB, s8WorkerBID)
	assertS8StreamIsolation(t, streamB.writer.bytes(), s8WorkerBID, s8ProviderSessionB, s8OutputB, s8WorkerASuccessorID, scenario.repositoryA.path, s8ReplacementMessage)
	assertS8CanceledStreamIsolation(t, streamA.writer.bytes(), s8WorkerAID, s8ProviderSessionA, s8WorkerASuccessorID, scenario.repositoryB.path, s8MessageB)

	retainedSource := replayS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8WorkerAID)
	retainedSuccessor := replayS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8WorkerASuccessorID)
	retainedB := replayS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, s8WorkerBID)
	assertS8CanceledRetainedStream(t, retainedSource, s8WorkerAID, s8ProviderSessionA, s8WorkerASuccessorID, scenario.repositoryB.path, s8MessageB)
	assertS8RetainedStream(t, retainedSuccessor, s8WorkerASuccessorID, s8ProviderSessionA, s8ReplacementOutput, s8WorkerBID, scenario.repositoryB.path, s8MessageB)
	assertS8RetainedStream(t, retainedB, s8WorkerBID, s8ProviderSessionB, s8OutputB, s8WorkerASuccessorID, scenario.repositoryA.path, s8ReplacementMessage)

	completed := listS8RemoteWorkers(t, scenario.ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL)
	if len(completed) != 3 {
		t.Fatalf("completed direct Worker Sessions = %d, want source, successor, and B: %#v", len(completed), completed)
	}
	assertS8Observation(t, findS8Observation(t, completed, s8WorkerAID), s8WorkerAID, "CANCELED", s8ProviderSessionA, s8DispatchAID)
	assertS8Observation(t, findS8Observation(t, completed, s8WorkerASuccessorID), s8WorkerASuccessorID, "COMPLETED", s8ProviderSessionA, successor.AttemptID)
	assertS8Observation(t, findS8Observation(t, completed, s8WorkerBID), s8WorkerBID, "COMPLETED", s8ProviderSessionB, s8DispatchBID)

	finalSource := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8WorkerAID)
	finalSuccessor := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8WorkerASuccessorID)
	finalB := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, s8WorkerBID)
	assertS8Observation(t, finalSource, s8WorkerAID, "CANCELED", s8ProviderSessionA, s8DispatchAID)
	assertS8Observation(t, finalSuccessor, s8WorkerASuccessorID, "COMPLETED", s8ProviderSessionA, successor.AttemptID)
	assertS8Observation(t, finalB, s8WorkerBID, "COMPLETED", s8ProviderSessionB, s8DispatchBID)

	assertS8InterruptProviderRequests(t, scenario.runner.requests(), scenario.repositoryA, scenario.repositoryB)
	if scenario.runner.cancellationCount(s8InterruptCallAInitial) != 1 || scenario.runner.cancellationCount(s8InterruptCallBInitial) != 0 || scenario.runner.cancellationCount(s8InterruptCallASuccessor) != 0 {
		t.Fatalf("provider cancellation counts = A initial:%d A successor:%d B:%d, want 1/0/0", scenario.runner.cancellationCount(s8InterruptCallAInitial), scenario.runner.cancellationCount(s8InterruptCallASuccessor), scenario.runner.cancellationCount(s8InterruptCallBInitial))
	}
}

type s8InterruptResult struct {
	RequestID                string              `json:"requestId"`
	SourceWorkerSessionID    string              `json:"sourceWorkerSessionId"`
	SuccessorWorkerSessionID string              `json:"successorWorkerSessionId"`
	Phase                    string              `json:"phase"`
	Accepted                 bool                `json:"accepted"`
	Source                   s8InterruptSnapshot `json:"source"`
	Successor                s8InterruptSnapshot `json:"successor"`
}

type s8InterruptSnapshot struct {
	WorkerSessionID string `json:"workerSessionId"`
	State           string `json:"state"`
	EventTopic      string `json:"eventTopic"`
}

func interruptS8RemoteWorker(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL string,
) s8InterruptResult {
	t.Helper()
	inputs := executeS8RemoteCLI(t, ctx, process, env, workingDirectory, serverURL,
		"--json", "worker-sessions", "interrupt", s8WorkerAID,
		"--request-id", s8InterruptRequestID,
		"--successor-worker-session-id", s8WorkerASuccessorID,
		"--replacement-message", s8ReplacementMessage, "--async")
	var result s8InterruptResult
	decodeS8JSON(t, inputs.Stdout(), &result)
	return result
}

func assertS8InterruptAdmission(t *testing.T, result s8InterruptResult) {
	t.Helper()
	if !result.Accepted || result.RequestID != s8InterruptRequestID ||
		result.SourceWorkerSessionID != s8WorkerAID || result.SuccessorWorkerSessionID != s8WorkerASuccessorID ||
		result.Phase != "SUCCESSOR_ADMISSION" ||
		result.Source.WorkerSessionID != s8WorkerAID || result.Source.State != "CANCELED" || result.Source.EventTopic == "" ||
		result.Successor.WorkerSessionID != s8WorkerASuccessorID || result.Successor.State != "RUNNING" || result.Successor.EventTopic == "" {
		t.Fatalf("interrupt result = %#v, want exact A cancellation and successor admission", result)
	}
}

func assertS8Observation(
	t *testing.T,
	observation s8WorkerObservation,
	workerSessionID, state, providerSessionID, attemptID string,
) {
	t.Helper()
	if observation.WorkerSessionID != workerSessionID || !observation.Direct || observation.State != state || observation.AttemptID != attemptID {
		t.Fatalf("Worker Session observation = %#v, want %s/%s attempt %q", observation, workerSessionID, state, attemptID)
	}
	assertS8ProviderSession(t, observation.ProviderSession, observation.ProviderSessionAvailable, providerSessionID)
}

func assertS8CanceledStreamIsolation(
	t *testing.T,
	stdout []byte,
	workerSessionID, providerSessionID, foreignWorkerSessionID, foreignRepository, foreignMessage string,
) {
	t.Helper()
	frames := decodeS8Stream(t, string(stdout))
	if len(frames) == 0 {
		t.Fatalf("canceled stream for %q returned no frames", workerSessionID)
	}
	assertS8Frames(t, frames, workerSessionID, providerSessionID, foreignWorkerSessionID, foreignRepository, foreignMessage)
	for _, frame := range frames {
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			return
		}
	}
	t.Fatalf("canceled stream for %q omitted terminal delivery: %#v", workerSessionID, frames)
}

func assertS8CanceledRetainedStream(
	t *testing.T,
	frames []s8StreamFrame,
	workerSessionID, providerSessionID, foreignWorkerSessionID, foreignRepository, foreignMessage string,
) {
	t.Helper()
	if len(frames) == 0 {
		t.Fatalf("retained canceled stream for %q returned no frames", workerSessionID)
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
		t.Fatalf("retained canceled stream for %q = %#v, want terminal and complete replay summary", workerSessionID, frames)
	}
}

const (
	s8InterruptCallAInitial   = "a-initial"
	s8InterruptCallASuccessor = "a-successor"
	s8InterruptCallBInitial   = "b-initial"
)

type s8InterruptProviderCall struct {
	kind        string
	sessionID   string
	output      string
	started     chan struct{}
	release     chan struct{}
	canceled    chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
	cancelOnce  sync.Once
}

type s8InterruptProviderCase struct {
	repository string
	calls      []*s8InterruptProviderCall
	next       int
}

type s8InterruptProviderRunner struct {
	mu                 sync.Mutex
	stdout             []byte
	cases              map[string]*s8InterruptProviderCase
	requestLog         []platformprocess.CommandRequest
	order              []string
	invocationCounts   map[string]int
	cancellationCounts map[string]int
}

func newS8InterruptProviderRunner(stdout []byte, repositoryA, repositoryB s8Repository) *s8InterruptProviderRunner {
	return &s8InterruptProviderRunner{
		stdout: append([]byte(nil), stdout...),
		cases: map[string]*s8InterruptProviderCase{
			repositoryA.path: {
				repository: repositoryA.path,
				calls: []*s8InterruptProviderCall{
					newS8InterruptProviderCall(s8InterruptCallAInitial, s8ProviderSessionA, s8OutputA),
					newS8InterruptProviderCall(s8InterruptCallASuccessor, s8ProviderSessionA, s8ReplacementOutput),
				},
			},
			repositoryB.path: {
				repository: repositoryB.path,
				calls: []*s8InterruptProviderCall{
					newS8InterruptProviderCall(s8InterruptCallBInitial, s8ProviderSessionB, s8OutputB),
				},
			},
		},
		invocationCounts:   make(map[string]int),
		cancellationCounts: make(map[string]int),
	}
}

func newS8InterruptProviderCall(kind, sessionID, output string) *s8InterruptProviderCall {
	return &s8InterruptProviderCall{
		kind: kind, sessionID: sessionID, output: output,
		started: make(chan struct{}), release: make(chan struct{}), canceled: make(chan struct{}),
	}
}

func (runner *s8InterruptProviderRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.run(ctx, request, nil)
}

func (runner *s8InterruptProviderRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	return runner.run(ctx, request, observer)
}

func (runner *s8InterruptProviderRunner) run(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	call, err := runner.nextCall(request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	output := bytes.ReplaceAll(runner.stdout, []byte("session_fixture_codex_success"), []byte(call.sessionID))
	output = bytes.ReplaceAll(output, []byte("Codex fixture answer COMPLETE"), []byte(call.output))
	lineEnd := bytes.IndexByte(output, '\n')
	if lineEnd < 0 {
		lineEnd = len(output)
	} else {
		lineEnd++
	}
	if observer != nil && lineEnd > 0 {
		observer(platformprocess.OutputStreamStdout, append([]byte(nil), output[:lineEnd]...))
	}
	runner.recordStarted(call)

	select {
	case <-call.release:
	case <-ctx.Done():
		runner.recordCanceled(call)
		return platformprocess.CommandResult{}, ctx.Err()
	}
	if observer != nil && lineEnd < len(output) {
		observer(platformprocess.OutputStreamStdout, append([]byte(nil), output[lineEnd:]...))
	}
	return platformprocess.CommandResult{Stdout: output}, nil
}

func (runner *s8InterruptProviderRunner) nextCall(request platformprocess.CommandRequest) (*s8InterruptProviderCall, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	providerCase := runner.cases[request.WorkDir]
	if providerCase == nil {
		return nil, fmt.Errorf("unexpected S8 interrupt provider working directory %q", request.WorkDir)
	}
	if providerCase.next >= len(providerCase.calls) {
		return nil, fmt.Errorf("unexpected duplicate S8 interrupt provider call in %q", request.WorkDir)
	}
	call := providerCase.calls[providerCase.next]
	providerCase.next++
	runner.requestLog = append(runner.requestLog, cloneS8CommandRequest(request))
	runner.invocationCounts[call.kind]++
	return call, nil
}

func (runner *s8InterruptProviderRunner) recordStarted(call *s8InterruptProviderCall) {
	runner.mu.Lock()
	runner.order = append(runner.order, "start:"+call.kind)
	runner.mu.Unlock()
	call.startOnce.Do(func() { close(call.started) })
}

func (runner *s8InterruptProviderRunner) recordCanceled(call *s8InterruptProviderCall) {
	call.cancelOnce.Do(func() {
		runner.mu.Lock()
		runner.order = append(runner.order, "cancel:"+call.kind)
		runner.cancellationCounts[call.kind]++
		runner.mu.Unlock()
		close(call.canceled)
	})
}

func (runner *s8InterruptProviderRunner) callFor(repository, kind string) *s8InterruptProviderCall {
	providerCase := runner.cases[repository]
	if providerCase == nil {
		return nil
	}
	for _, call := range providerCase.calls {
		if call.kind == kind {
			return call
		}
	}
	return nil
}

func (runner *s8InterruptProviderRunner) waitStarted(t *testing.T, repository, kind string) {
	t.Helper()
	call := runner.callFor(repository, kind)
	if call == nil {
		t.Fatalf("S8 interrupt provider call %q for %q is not configured", kind, repository)
	}
	runner.waitSignal(t, call.started, "provider start", kind)
}

func (runner *s8InterruptProviderRunner) waitCanceled(t *testing.T, repository, kind string) {
	t.Helper()
	call := runner.callFor(repository, kind)
	if call == nil {
		t.Fatalf("S8 interrupt provider call %q for %q is not configured", kind, repository)
	}
	runner.waitSignal(t, call.canceled, "provider cancellation", kind)
}

func (runner *s8InterruptProviderRunner) waitSignal(t *testing.T, signal <-chan struct{}, label, kind string) {
	t.Helper()
	watchdog := time.NewTimer(20 * time.Second)
	defer watchdog.Stop()
	select {
	case <-signal:
	case <-watchdog.C:
		t.Fatalf("deadlock watchdog expired waiting for S8 %s %q", label, kind)
	}
}

func (runner *s8InterruptProviderRunner) release(t *testing.T, repository, kind string) {
	t.Helper()
	call := runner.callFor(repository, kind)
	if call == nil {
		t.Fatalf("S8 interrupt provider call %q for %q is not configured", kind, repository)
	}
	call.releaseOnce.Do(func() { close(call.release) })
}

func (runner *s8InterruptProviderRunner) releaseAll() {
	for _, providerCase := range runner.cases {
		for _, call := range providerCase.calls {
			call.releaseOnce.Do(func() { close(call.release) })
		}
	}
}

func (runner *s8InterruptProviderRunner) requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requestLog))
	for index, request := range runner.requestLog {
		requests[index] = cloneS8CommandRequest(request)
	}
	return requests
}

func (runner *s8InterruptProviderRunner) cancellationCount(kind string) int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.cancellationCounts[kind]
}

func (runner *s8InterruptProviderRunner) assertOrder(t *testing.T, required ...string) {
	t.Helper()
	runner.mu.Lock()
	order := append([]string(nil), runner.order...)
	runner.mu.Unlock()
	position := 0
	for _, want := range required {
		for position < len(order) && order[position] != want {
			position++
		}
		if position == len(order) {
			t.Fatalf("provider edge order = %#v, missing %q after required prefix %#v", order, want, required)
		}
		position++
	}
}

func assertS8InterruptProviderRequests(
	t *testing.T,
	requests []platformprocess.CommandRequest,
	repositoryA, repositoryB s8Repository,
) {
	t.Helper()
	if len(requests) != 3 {
		t.Fatalf("provider command requests = %d, want A initial, A successor, and B initial: %#v", len(requests), requests)
	}
	byRepository := map[string][]platformprocess.CommandRequest{}
	for _, request := range requests {
		if request.Command != "codex" {
			t.Fatalf("provider command = %q, want codex", request.Command)
		}
		byRepository[request.WorkDir] = append(byRepository[request.WorkDir], request)
	}
	if len(byRepository[repositoryA.path]) != 2 || len(byRepository[repositoryB.path]) != 1 {
		t.Fatalf("provider requests by repository = %#v, want A:2 B:1", byRepository)
	}

	initialA := byRepository[repositoryA.path][0]
	successorA := byRepository[repositoryA.path][1]
	initialB := byRepository[repositoryB.path][0]
	assertS8ProviderRequest(t, initialA, repositoryA.path, s8MessageA, false, s8ReplacementMessage, s8MessageB)
	assertS8ProviderRequest(t, successorA, repositoryA.path, s8ReplacementMessage, true, s8MessageA, s8MessageB)
	assertS8ProviderRequest(t, initialB, repositoryB.path, s8MessageB, false, s8MessageA, s8ReplacementMessage)
	if !strings.Contains(strings.Join(successorA.Args, " "), s8ProviderSessionA) {
		t.Fatalf("successor A provider args = %#v, want resumed Provider Session %q", successorA.Args, s8ProviderSessionA)
	}
}

func assertS8ProviderRequest(
	t *testing.T,
	request platformprocess.CommandRequest,
	repository, required string,
	resumeExpected bool,
	forbidden ...string,
) {
	t.Helper()
	requestText := strings.Join([]string{request.Command, strings.Join(request.Args, " "), string(request.Stdin), strings.Join(request.Env, "\n"), request.WorkDir}, "\n")
	if request.WorkDir != repository || !strings.Contains(requestText, required) {
		t.Fatalf("provider request %s = %#v, want repository %q and input %q", required, request, repository, required)
	}
	for _, value := range forbidden {
		if strings.Contains(requestText, value) {
			t.Fatalf("provider request %s contains forbidden input %q: %#v", required, value, request)
		}
	}
	if strings.Contains(strings.Join(request.Args, " "), "resume") != resumeExpected {
		t.Fatalf("provider request %s resume flag = %t, want %t: %#v", required, strings.Contains(strings.Join(request.Args, " "), "resume"), resumeExpected, request.Args)
	}
}

var _ platformprocess.CommandRunner = (*s8InterruptProviderRunner)(nil)
