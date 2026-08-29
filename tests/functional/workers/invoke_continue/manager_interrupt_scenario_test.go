package acceptance

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	s8ReplacementMessage = "replace repository A instruction only"
	s8ReplacementOutput  = "S8 repository A replacement provider output"
)

type s8InterruptScenario struct {
	ctx         context.Context
	fixture     *invokeContinuePackageFixture
	manager     support.Process
	env         []string
	factoryDir  string
	serverURL   string
	session     *invokeContinueFactorySession
	repositoryA s8Repository
	repositoryB s8Repository
	runner      *s8InterruptProviderRunner
	ids         s8ScenarioIdentities
}

// TestDWROS8ManagerInterruptsOnlyOneRemoteWorker proves the second S8 story
// through the public manager boundary. A's source, A's resumed successor, and
// B each have an explicit provider-edge barrier. The only readiness signals
// used below are those edge barriers and complete public stream/list
// observations; the deadline is a bounded deadlock watchdog for a broken
// lifecycle.
func TestDWROS8ManagerInterruptsOnlyOneRemoteWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx)
	defer scenario.runner.releaseAll()
	ids := scenario.ids

	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)

	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestB, workerSessionID: ids.workerB, dispatchID: ids.dispatchB,
		factorySessionID: scenario.session.id, repository: scenario.repositoryB.path, workID: ids.workB, message: s8MessageB,
	})
	scenario.runner.waitStarted(t, scenario.repositoryB.path, s8InterruptCallBInitial, scenario.fixture.router.requests)

	streamA := startS8LiveStream(t, scenario.fixture, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, scenario.session.id, ids.workerA, s8InterruptProviderSessionA)
	streamA.writer.waitWorkerSessionFrame(t, ids.workerA)
	streamB := startS8LiveStream(t, scenario.fixture, ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, scenario.session.id, ids.workerB, s8InterruptProviderSessionB)
	streamB.writer.waitWorkerSessionFrame(t, ids.workerB)

	active := listS8RemoteWorkers(t, ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL, "STARTING", "RUNNING")
	if len(active) != 2 {
		t.Fatalf("active direct Worker Sessions = %d, want two: %#v", len(active), active)
	}
	assertS8Observation(t, findS8Observation(t, active, ids.workerA), ids.workerA, scenario.session.id, ids.workA, "RUNNING", s8InterruptProviderSessionA, ids.dispatchA)
	assertS8Observation(t, findS8Observation(t, active, ids.workerB), ids.workerB, scenario.session.id, ids.workB, "RUNNING", s8InterruptProviderSessionB, ids.dispatchB)

	successor, streamSuccessor := assertS8InterruptOverlap(t, scenario, streamA, streamB)
	finishS8InterruptScenario(t, scenario, streamA, streamB, streamSuccessor, successor)
	scenario.close(t)
}

func newS8InterruptScenario(t *testing.T, ctx context.Context) s8InterruptScenario {
	t.Helper()
	fixture := ensureInvokeContinuePackageFixture(t)
	ownedScenario := fixture.scenario(t, "manager-interrupt")
	return s8InterruptScenario{
		ctx: ctx, fixture: fixture, manager: fixture.process,
		env: invokeContinueEnvironment(fixture.homeDir), factoryDir: fixture.hostDir,
		serverURL: fixture.baseURL, session: ownedScenario.session,
		repositoryA: fixture.interruptRepositoryA, repositoryB: fixture.interruptRepositoryB,
		runner: fixture.interruptRunner, ids: newS8ScenarioIdentities("interrupt", ownedScenario.runNumber),
	}
}

func (scenario s8InterruptScenario) close(t testing.TB) {
	t.Helper()
	scenario.session.close(t)
	scenario.session.assertDeleted(t)
}

func assertS8InterruptOverlap(
	t *testing.T,
	scenario s8InterruptScenario,
	streamA, streamB s8StreamCapture,
) (s8WorkerObservation, s8StreamCapture) {
	t.Helper()
	ids := scenario.ids
	interrupt := interruptS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor)
	assertS8InterruptAdmission(t, interrupt, ids)
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallASuccessor, scenario.fixture.router.requests)
	scenario.runner.assertOrder(t, "start:"+s8InterruptCallAInitial, "cancel:"+s8InterruptCallAInitial, "start:"+s8InterruptCallASuccessor)

	streamSuccessor := startS8LiveStream(t, scenario.fixture, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, scenario.session.id, ids.successor, s8InterruptProviderSessionA)
	streamSuccessor.writer.waitWorkerSessionFrame(t, ids.successor)
	overlap := listS8RemoteWorkers(t, scenario.ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL, "CANCELED", "STARTING", "RUNNING")
	assertS8NoUnexpectedActiveObservations(t, overlap, ids.workerA, ids.successor, ids.workerB)
	assertS8Observation(t, findS8Observation(t, overlap, ids.workerA), ids.workerA, scenario.session.id, ids.workA, "CANCELED", s8InterruptProviderSessionA, ids.dispatchA)
	successor := findS8Observation(t, overlap, ids.successor)
	assertS8Observation(t, successor, ids.successor, scenario.session.id, ids.workA, "RUNNING", s8InterruptProviderSessionA, ids.dispatchA+"/continue/"+ids.successor)
	assertS8Observation(t, findS8Observation(t, overlap, ids.workerB), ids.workerB, scenario.session.id, ids.workB, "RUNNING", s8InterruptProviderSessionB, ids.dispatchB)

	showSource := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA)
	showSuccessor := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.successor)
	showB := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, ids.workerB)
	assertS8Observation(t, showSource, ids.workerA, scenario.session.id, ids.workA, "CANCELED", s8InterruptProviderSessionA, ids.dispatchA)
	assertS8Observation(t, showSuccessor, ids.successor, scenario.session.id, ids.workA, "RUNNING", s8InterruptProviderSessionA, successor.AttemptID)
	assertS8Observation(t, showB, ids.workerB, scenario.session.id, ids.workB, "RUNNING", s8InterruptProviderSessionB, ids.dispatchB)
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
	ids := scenario.ids
	scenario.runner.release(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	waitS8Stream(t, streamSuccessor, ids.successor)
	stillActiveB := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, ids.workerB)
	assertS8Observation(t, stillActiveB, ids.workerB, scenario.session.id, ids.workB, "RUNNING", s8InterruptProviderSessionB, ids.dispatchB)

	scenario.runner.release(t, scenario.repositoryB.path, s8InterruptCallBInitial)
	waitS8Stream(t, streamA, ids.workerA)
	waitS8Stream(t, streamB, ids.workerB)
	sourceCorrelation := s8Correlation{
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, marker: scenario.repositoryA.marker, dispatchID: ids.dispatchA,
		workID: ids.workA, workerSessionID: ids.workerA, providerSessionID: s8InterruptProviderSessionA, message: s8MessageA, output: s8OutputA,
		successorWorkerSessionID: ids.successor,
	}
	successorCorrelation := s8Correlation{
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, marker: scenario.repositoryA.marker, dispatchID: successor.AttemptID,
		workID: ids.workA, workerSessionID: ids.successor, providerSessionID: s8InterruptProviderSessionA, message: s8ReplacementMessage, output: s8ReplacementOutput,
	}
	bCorrelation := s8Correlation{
		factorySessionID: scenario.session.id, repository: scenario.repositoryB.path, marker: scenario.repositoryB.marker, dispatchID: ids.dispatchB,
		workID: ids.workB, workerSessionID: ids.workerB, providerSessionID: s8InterruptProviderSessionB, message: s8MessageB, output: s8OutputB,
	}
	assertS8StreamIsolation(t, streamB.writer.bytes(), bCorrelation, sourceCorrelation, successorCorrelation)
	assertS8CanceledStreamIsolation(t, streamA.writer.bytes(), sourceCorrelation, successorCorrelation, bCorrelation)

	retainedSource := replayS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA)
	retainedSuccessor := replayS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.successor)
	retainedB := replayS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, ids.workerB)
	assertS8CanceledRetainedStream(t, retainedSource, sourceCorrelation, successorCorrelation, bCorrelation)
	assertS8RetainedStream(t, retainedSuccessor, successorCorrelation, sourceCorrelation, bCorrelation)
	assertS8RetainedStream(t, retainedB, bCorrelation, sourceCorrelation, successorCorrelation)

	completed := listS8RemoteWorkers(t, scenario.ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL, "COMPLETED", "CANCELED")
	assertS8Observation(t, findS8Observation(t, completed, ids.workerA), ids.workerA, scenario.session.id, ids.workA, "CANCELED", s8InterruptProviderSessionA, ids.dispatchA)
	assertS8Observation(t, findS8Observation(t, completed, ids.successor), ids.successor, scenario.session.id, ids.workA, "COMPLETED", s8InterruptProviderSessionA, successor.AttemptID)
	assertS8Observation(t, findS8Observation(t, completed, ids.workerB), ids.workerB, scenario.session.id, ids.workB, "COMPLETED", s8InterruptProviderSessionB, ids.dispatchB)

	finalSource := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA)
	finalSuccessor := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.successor)
	finalB := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, ids.workerB)
	assertS8Observation(t, finalSource, ids.workerA, scenario.session.id, ids.workA, "CANCELED", s8InterruptProviderSessionA, ids.dispatchA)
	assertS8Observation(t, finalSuccessor, ids.successor, scenario.session.id, ids.workA, "COMPLETED", s8InterruptProviderSessionA, successor.AttemptID)
	assertS8Observation(t, finalB, ids.workerB, scenario.session.id, ids.workB, "COMPLETED", s8InterruptProviderSessionB, ids.dispatchB)

	assertS8InterruptProviderRequests(t, scenario.runner.requests(), scenario.runner.markers(), sourceCorrelation, successorCorrelation, bCorrelation)
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
	workingDirectory, serverURL, sourceWorkerSessionID, requestID, successorWorkerSessionID string,
) s8InterruptResult {
	t.Helper()
	inputs := executeS8RemoteCLI(t, ctx, process, env, workingDirectory, serverURL,
		"--json", "worker-sessions", "interrupt", sourceWorkerSessionID,
		"--request-id", requestID,
		"--successor-worker-session-id", successorWorkerSessionID,
		"--replacement-message", s8ReplacementMessage, "--async")
	var result s8InterruptResult
	decodeS8JSON(t, inputs.Stdout(), &result)
	return result
}

func assertS8InterruptAdmission(t *testing.T, result s8InterruptResult, ids s8ScenarioIdentities) {
	t.Helper()
	if !result.Accepted || result.RequestID != ids.interruptRequest ||
		result.SourceWorkerSessionID != ids.workerA || result.SuccessorWorkerSessionID != ids.successor ||
		result.Phase != "SUCCESSOR_ADMISSION" ||
		result.Source.WorkerSessionID != ids.workerA || result.Source.State != "CANCELED" || result.Source.EventTopic == "" ||
		result.Successor.WorkerSessionID != ids.successor || result.Successor.State != "RUNNING" || result.Successor.EventTopic == "" {
		t.Fatalf("interrupt result = %#v, want exact A cancellation and successor admission", result)
	}
}

func assertS8Observation(
	t *testing.T,
	observation s8WorkerObservation,
	workerSessionID, factorySessionID, workID, state, providerSessionID, attemptID string,
) {
	t.Helper()
	if observation.WorkerSessionID != workerSessionID || !observation.Direct || observation.State != state || observation.AttemptID != attemptID {
		t.Fatalf("Worker Session observation = %#v, want %s/%s attempt %q", observation, workerSessionID, state, attemptID)
	}
	if observation.FactorySessionID != nil {
		assertS8FactorySessionIfPresent(t, observation.FactorySessionID, factorySessionID)
	}
	assertS8WorkIDs(t, observation.WorkIDs, workID)
	assertS8ProviderSession(t, observation.ProviderSession, observation.ProviderSessionAvailable, providerSessionID)
}

func assertS8CanceledStreamIsolation(
	t *testing.T,
	stdout []byte,
	own s8Correlation,
	foreign ...s8Correlation,
) {
	t.Helper()
	frames := decodeS8Stream(t, string(stdout))
	if len(frames) == 0 {
		t.Fatalf("canceled stream for %q returned no frames", own.workerSessionID)
	}
	assertS8Frames(t, frames, own, false, foreign...)
	for _, frame := range frames {
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			return
		}
	}
	t.Fatalf("canceled stream for %q omitted terminal delivery: %#v", own.workerSessionID, frames)
}

func assertS8CanceledRetainedStream(
	t *testing.T,
	frames []s8StreamFrame,
	own s8Correlation,
	foreign ...s8Correlation,
) {
	t.Helper()
	if len(frames) == 0 {
		t.Fatalf("retained canceled stream for %q returned no frames", own.workerSessionID)
	}
	assertS8Frames(t, frames, own, false, foreign...)
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
		t.Fatalf("retained canceled stream for %q = %#v, want terminal and complete replay summary", own.workerSessionID, frames)
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
	marker     string
	calls      []*s8InterruptProviderCall
	next       int
}

type s8InterruptProviderRunner struct {
	mu                 sync.Mutex
	stdout             []byte
	cases              map[string]*s8InterruptProviderCase
	requestLog         []platformprocess.CommandRequest
	markerLog          map[string][]string
	order              []string
	invocationCounts   map[string]int
	cancellationCounts map[string]int
	active             atomic.Int32
}

func newS8InterruptProviderRunner(stdout []byte, repositoryA, repositoryB s8Repository) *s8InterruptProviderRunner {
	runner := &s8InterruptProviderRunner{
		stdout: append([]byte(nil), stdout...),
		cases: map[string]*s8InterruptProviderCase{
			repositoryA.path: {
				repository: repositoryA.path,
				marker:     repositoryA.marker,
				calls: []*s8InterruptProviderCall{
					newS8InterruptProviderCall(s8InterruptCallAInitial, s8InterruptProviderSessionA, s8OutputA),
					newS8InterruptProviderCall(s8InterruptCallASuccessor, s8InterruptProviderSessionA, s8ReplacementOutput),
				},
			},
			repositoryB.path: {
				repository: repositoryB.path,
				marker:     repositoryB.marker,
				calls: []*s8InterruptProviderCall{
					newS8InterruptProviderCall(s8InterruptCallBInitial, s8InterruptProviderSessionB, s8OutputB),
				},
			},
		},
		invocationCounts:   make(map[string]int),
		cancellationCounts: make(map[string]int),
		markerLog:          make(map[string][]string),
	}
	runner.reset()
	return runner
}

func (runner *s8InterruptProviderRunner) reset() {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	for _, providerCase := range runner.cases {
		providerCase.next = 0
		for _, call := range providerCase.calls {
			call.started = make(chan struct{})
			call.release = make(chan struct{})
			call.canceled = make(chan struct{})
			call.startOnce = sync.Once{}
			call.releaseOnce = sync.Once{}
			call.cancelOnce = sync.Once{}
		}
	}
	runner.requestLog = nil
	runner.markerLog = make(map[string][]string)
	runner.order = nil
	runner.invocationCounts = make(map[string]int)
	runner.cancellationCounts = make(map[string]int)
	runner.active.Store(0)
}

func (runner *s8InterruptProviderRunner) ActiveCallCount() int {
	return int(runner.active.Load())
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
	runner.active.Add(1)
	defer runner.active.Add(-1)
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
	providerCase := runner.cases[request.WorkDir]
	if providerCase == nil {
		runner.mu.Unlock()
		return nil, fmt.Errorf("unexpected S8 interrupt provider working directory %q", request.WorkDir)
	}
	if providerCase.next >= len(providerCase.calls) {
		runner.mu.Unlock()
		return nil, fmt.Errorf("unexpected duplicate S8 interrupt provider call in %q", request.WorkDir)
	}
	call := providerCase.calls[providerCase.next]
	providerCase.next++
	marker := providerCase.marker
	foreignMarkers := make([]string, 0, len(runner.cases)-1)
	for _, other := range runner.cases {
		if other.repository != request.WorkDir {
			foreignMarkers = append(foreignMarkers, other.marker)
		}
	}
	runner.mu.Unlock()
	observedMarker, err := readS8RepositoryMarker(request.WorkDir)
	if err != nil {
		return nil, err
	}
	if observedMarker != marker {
		return nil, fmt.Errorf("S8 interrupt provider working directory %q marker = %q, want %q", request.WorkDir, observedMarker, marker)
	}
	for _, foreignMarker := range foreignMarkers {
		if observedMarker == foreignMarker {
			return nil, fmt.Errorf("S8 interrupt provider working directory %q observed foreign marker %q", request.WorkDir, observedMarker)
		}
	}
	runner.mu.Lock()
	runner.requestLog = append(runner.requestLog, cloneS8CommandRequest(request))
	runner.markerLog[request.WorkDir] = append(runner.markerLog[request.WorkDir], observedMarker)
	runner.invocationCounts[call.kind]++
	runner.mu.Unlock()
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

func (runner *s8InterruptProviderRunner) waitStarted(
	t *testing.T,
	repository, kind string,
	routeRequests ...func() []platformprocess.CommandRequest,
) {
	t.Helper()
	call := runner.callFor(repository, kind)
	if call == nil {
		t.Fatalf("S8 interrupt provider call %q for %q is not configured", kind, repository)
	}
	watchdog := time.NewTimer(20 * time.Second)
	defer watchdog.Stop()
	select {
	case <-call.started:
	case <-watchdog.C:
		var routed []platformprocess.CommandRequest
		if len(routeRequests) > 0 && routeRequests[0] != nil {
			routed = routeRequests[0]()
		}
		runner.mu.Lock()
		requests := make([]platformprocess.CommandRequest, len(runner.requestLog))
		for index, request := range runner.requestLog {
			requests[index] = cloneS8CommandRequest(request)
		}
		runner.mu.Unlock()
		t.Fatalf("deadlock watchdog expired waiting for S8 provider start %q in %q; runner requests=%#v route requests=%#v", kind, repository, requests, routed)
	}
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

func (runner *s8InterruptProviderRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requestLog)
}

func (runner *s8InterruptProviderRunner) Requests() []platformprocess.CommandRequest {
	return runner.requests()
}

func (runner *s8InterruptProviderRunner) markers() map[string][]string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	markers := make(map[string][]string, len(runner.markerLog))
	for repository, values := range runner.markerLog {
		markers[repository] = append([]string(nil), values...)
	}
	return markers
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
	markers map[string][]string,
	correlations ...s8Correlation,
) {
	t.Helper()
	if len(requests) != 3 || len(correlations) != 3 {
		t.Fatalf("provider command requests = %d, want A initial, A successor, and B initial: %#v", len(requests), requests)
	}
	assertS8ProviderMarkers(t, markers, correlations...)
	byRepository := map[string][]platformprocess.CommandRequest{}
	for _, request := range requests {
		byRepository[request.WorkDir] = append(byRepository[request.WorkDir], request)
	}
	if len(byRepository[correlations[0].repository]) != 2 || len(byRepository[correlations[2].repository]) != 1 {
		t.Fatalf("provider requests by repository = %#v, want A:2 B:1", byRepository)
	}

	initialA := byRepository[correlations[0].repository][0]
	successorA := byRepository[correlations[0].repository][1]
	initialB := byRepository[correlations[2].repository][0]
	assertS8ProviderRequest(t, initialA, correlations[0], correlations[1], correlations[2])
	assertS8ProviderRequest(t, successorA, correlations[1], correlations[0], correlations[2])
	assertS8ProviderRequest(t, initialB, correlations[2], correlations[0], correlations[1])
	if !strings.Contains(strings.Join(successorA.Args, " "), correlations[1].providerSessionID) {
		t.Fatalf("successor A provider args = %#v, want resumed Provider Session %q", successorA.Args, correlations[1].providerSessionID)
	}
	if !strings.Contains(strings.Join(successorA.Args, " "), "resume") {
		t.Fatalf("successor A provider args = %#v, want resume", successorA.Args)
	}
}

var _ platformprocess.CommandRunner = (*s8InterruptProviderRunner)(nil)
