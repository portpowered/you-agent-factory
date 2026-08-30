package composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	compositionFixtureTimeout = 15 * time.Second
	compositionBehaviorCount  = 14
)

// compositionStreamLifecycle observes the actual public SSE handler lifetime
// at the injected HTTP edge. It is intentionally edge-local: no application
// resource registry is recreated by the test.
type compositionStreamLifecycle struct {
	active          atomic.Int32
	opened          atomic.Int32
	closed          atomic.Int32
	sessionRequests atomic.Int32
}

func (lifecycle *compositionStreamLifecycle) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/factory-sessions") {
			lifecycle.sessionRequests.Add(1)
		}
		isStream := strings.HasSuffix(request.URL.Path, "/events") ||
			strings.HasSuffix(request.URL.Path, "/response-events")
		if !isStream {
			next.ServeHTTP(writer, request)
			return
		}
		lifecycle.opened.Add(1)
		lifecycle.active.Add(1)
		defer func() {
			lifecycle.active.Add(-1)
			lifecycle.closed.Add(1)
		}()
		next.ServeHTTP(writer, request)
	})
}

// TestMain owns the one process and one HTTP listener for this package. The
// fixture intentionally outlives individual tests so every behavior receives
// a fresh explicit Factory Session without rebuilding the application graph.
func TestMain(m *testing.M) {
	code := m.Run()

	compositionFixtureMu.Lock()
	fixture := sharedCompositionFixture
	compositionFixtureMu.Unlock()
	if fixture != nil {
		if err := fixture.shutdown(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

// TestJavaScriptCompositionFixturePartialStartUnwinds proves a real process
// startup failure preserves the original error and closes the listener that
// was acquired by the injected HTTP transport edge.
func TestJavaScriptCompositionFixturePartialStartUnwinds(t *testing.T) {
	hostDir := support.ScaffoldFactory(t, parallelCompositionFactoryConfig())
	homeDir := t.TempDir()
	support.WriteAgentConfig(t, hostDir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	writeParallelCompositionGlobalConfig(t, homeDir)

	original := errors.New("injected composition fixture start failure")
	partialAPI := support.NewProcessAPIServer()
	partialStopped := make(chan struct{})
	partialStreams := &compositionStreamLifecycle{}
	var listenerURL string
	var starterCalls atomic.Int32
	failingStarter := newCompositionPartialStartStarter(original, partialAPI, partialStopped, partialStreams, &listenerURL, &starterCalls)

	runner := newCompositionCommandRunner()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      failingStarter,
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess(composition partial start): %v", err)
	}
	closed := false
	defer func() {
		if !closed {
			if closeErr := process.Close(context.Background()); closeErr != nil {
				t.Errorf("close partial-start process: %v", closeErr)
			}
		}
	}()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = compositionCustomerEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	got := process.Execute(inputs.Input)
	closeErr := process.Close(context.Background())
	if closeErr != nil {
		t.Errorf("close partial-start process: %v", closeErr)
	}
	closed = closeErr == nil
	if !errors.Is(got, original) && (got == nil || !strings.Contains(got.Error(), original.Error())) {
		t.Fatalf("partial-start error = %v, want original error %q", got, original)
	}
	if got := starterCalls.Load(); got != 1 {
		t.Fatalf("partial-start API starter calls = %d, want one", got)
	}
	if got := runner.callCount(); got != 0 {
		t.Fatalf("partial-start provider command calls = %d, want zero", got)
	}

	listenerClosed := false
	<-partialStopped
	if partialStreams.active.Load() != 0 || partialStreams.opened.Load() != partialStreams.closed.Load() {
		t.Fatalf("partial composition stream edge did not close: active=%d opened=%d closed=%d", partialStreams.active.Load(), partialStreams.opened.Load(), partialStreams.closed.Load())
	}
	listenerClosed = true
	if strings.TrimSpace(listenerURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, probeErr := client.Get(listenerURL + "/status")
		if probeErr == nil {
			listenerClosed = false
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("partial-start listener remained available: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
		}
	} else {
		t.Fatal("partial-start application listener URL was not recorded")
	}
	// The listener was intentionally never published, so the edge request
	// observation must show that no Factory Session route was admitted.
	sessionActive := int(partialStreams.sessionRequests.Load())
	if sessionActive != 0 {
		t.Fatalf("partial-start Factory Session requests = %d, want zero", sessionActive)
	}
	processActive := boolToInt(!closed)
	portActive := boolToInt(!listenerClosed)
	streamActive := int(partialStreams.active.Load())
	routeActive := runner.activeCount()
	rootActive := boolToInt(!closed)
	worktreeActive, err := removeAndObservePath(hostDir)
	if err != nil {
		t.Fatalf("remove composition partial-start factory: %v", err)
	}
	mutableStateActive := sessionActive + streamActive + routeActive + rootActive + worktreeActive
	if processActive != 0 || portActive != 0 || streamActive != 0 || routeActive != 0 || rootActive != 0 || worktreeActive != 0 || mutableStateActive != 0 {
		t.Fatalf("composition partial-start active resources process=%d port=%d listener=%d session=%d stream=%d route=%d root=%d worktree=%d mutable-state=%d", processActive, portActive, portActive, sessionActive, streamActive, routeActive, rootActive, worktreeActive, mutableStateActive)
	}
	t.Logf("composition partial-start lifecycle report: process_closed=%t api_starter_calls=%d active={process:%d port:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d} streams_opened=%d streams_closed=%d provider_calls=%d original_error=%q", closed, starterCalls.Load(), processActive, portActive, portActive, sessionActive, streamActive, routeActive, rootActive, worktreeActive, mutableStateActive, partialStreams.opened.Load(), partialStreams.closed.Load(), runner.callCount(), original)
}

func newCompositionPartialStartStarter(
	original error,
	api *support.ProcessAPIServer,
	stopped chan struct{},
	streams *compositionStreamLifecycle,
	listenerURL *string,
	starterCalls *atomic.Int32,
) func(context.Context, platformhttpserver.StartRequest) error {
	return func(ctx context.Context, request platformhttpserver.StartRequest) error {
		starterCalls.Add(1)
		partialContext, cancelPartial := context.WithCancel(ctx)
		startDone := make(chan error, 1)
		request.OnBound = nil
		request.Handler = streams.wrap(request.Handler)
		go func() {
			startDone <- api.Start(partialContext, request)
			close(stopped)
		}()
		baseURL, err := api.WaitForBaseURL(compositionFixtureTimeout)
		if err != nil {
			cancelPartial()
			<-startDone
			return err
		}
		*listenerURL = baseURL
		cancelPartial()
		if err := <-startDone; err != nil {
			return err
		}
		if streams.active.Load() != 0 || streams.opened.Load() != streams.closed.Load() {
			return fmt.Errorf("partial composition stream edge did not close: active=%d opened=%d closed=%d", streams.active.Load(), streams.opened.Load(), streams.closed.Load())
		}
		return original
	}
}

func TestJavaScriptAgentReturnsUnaryResult(t *testing.T) {
	runJavaScriptAgentReturnsUnaryResult(t, compositionFixtureForTest(t))
}

func TestJavaScriptAgentFailureReturnsStableFailureRecord(t *testing.T) {
	runJavaScriptAgentFailureReturnsStableFailureRecord(t, compositionFixtureForTest(t))
}

func TestJavaScriptForEachDispatchesEveryInputOnce(t *testing.T) {
	runJavaScriptForEachDispatchesEveryInputOnce(t, compositionFixtureForTest(t))
}

func TestJavaScriptForEachPreservesInputResultCorrelation(t *testing.T) {
	runJavaScriptForEachPreservesInputResultCorrelation(t, compositionFixtureForTest(t))
}

func TestJavaScriptForEachEmptyInputDoesNotDispatch(t *testing.T) {
	runJavaScriptForEachEmptyInputDoesNotDispatch(t, compositionFixtureForTest(t))
}

func TestJavaScriptNestedPipelineParallelCompositionCompletes(t *testing.T) {
	runJavaScriptNestedPipelineParallelCompositionCompletes(t, compositionFixtureForTest(t))
}

func TestJavaScriptNestedFailureNamesChildAndStage(t *testing.T) {
	runJavaScriptNestedFailureNamesChildAndStage(t, compositionFixtureForTest(t))
}

func TestJavaScriptParallelDispatchesChildrenConcurrently(t *testing.T) {
	runJavaScriptParallelDispatchesChildrenConcurrently(t, compositionFixtureForTest(t))
}

func TestJavaScriptParallelPreservesDeclaredResultOrdering(t *testing.T) {
	runJavaScriptParallelPreservesDeclaredResultOrdering(t, compositionFixtureForTest(t))
}

func TestJavaScriptParallelPartialFailureUsesDocumentedPolicy(t *testing.T) {
	runJavaScriptParallelPartialFailureUsesDocumentedPolicy(t, compositionFixtureForTest(t))
}

func TestJavaScriptPipelinePassesStageOutputToNextStage(t *testing.T) {
	runJavaScriptPipelinePassesStageOutputToNextStage(t, compositionFixtureForTest(t))
}

func TestJavaScriptPipelineStopsAfterStageFailure(t *testing.T) {
	runJavaScriptPipelineStopsAfterStageFailure(t, compositionFixtureForTest(t))
}

func TestJavaScriptNamedStagesExposeOrderedProgress(t *testing.T) {
	runJavaScriptNamedStagesExposeOrderedProgress(t, compositionFixtureForTest(t))
}

func TestJavaScriptEmptyStageProducesDocumentedResult(t *testing.T) {
	runJavaScriptEmptyStageProducesDocumentedResult(t, compositionFixtureForTest(t))
}

var (
	compositionFixtureMu     sync.Mutex
	sharedCompositionFixture *compositionFixture
)

type compositionFixture struct {
	process     support.ApplicationProcess
	api         *support.ProcessAPIServer
	runner      *compositionCommandRunner
	baseURL     string
	hostDir     string
	homeDir     string
	environment []string

	rootBuilds    atomic.Int32
	processStarts atomic.Int32
	processStops  atomic.Int32
	apiStarts     atomic.Int32
	apiStopped    chan struct{}
	apiStopOnce   sync.Once
	requestNumber atomic.Uint64

	processCancel context.CancelFunc
	processDone   chan struct{}
	processMu     sync.Mutex
	processErr    error

	sessionMu sync.Mutex
	sessions  map[string]compositionSession
	closed    map[string]struct{}
	stream    compositionStreamLifecycle
}

type compositionSession struct {
	rootDir string
}

func compositionFixtureForTest(t *testing.T) *compositionFixture {
	t.Helper()
	compositionFixtureMu.Lock()
	defer compositionFixtureMu.Unlock()
	if sharedCompositionFixture == nil {
		sharedCompositionFixture = newCompositionFixture(t)
	}
	return sharedCompositionFixture
}

func newCompositionFixture(t *testing.T) *compositionFixture {
	t.Helper()

	homeDir, err := os.MkdirTemp("", "you-functional-composition-home-")
	if err != nil {
		t.Fatalf("create composition home: %v", err)
	}
	hostDir, err := os.MkdirTemp("", "you-functional-composition-factory-")
	if err != nil {
		_ = os.RemoveAll(homeDir)
		t.Fatalf("create composition factory: %v", err)
	}
	writePersistentCompositionFactory(t, hostDir)
	writeParallelCompositionGlobalConfig(t, homeDir)

	api := support.NewProcessAPIServer()
	runner := newCompositionCommandRunner()
	fixture := &compositionFixture{
		api:         api,
		runner:      runner,
		hostDir:     hostDir,
		homeDir:     homeDir,
		processDone: make(chan struct{}),
		apiStopped:  make(chan struct{}),
		sessions:    make(map[string]compositionSession, compositionBehaviorCount),
		closed:      make(map[string]struct{}, compositionBehaviorCount),
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.startAPIServer,
		ProviderCommandRunner: runner,
	})
	if err != nil {
		_ = os.RemoveAll(hostDir)
		_ = os.RemoveAll(homeDir)
		t.Fatalf("BuildProcess(composition): %v", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)

	processContext, cancel := context.WithCancel(context.Background())
	fixture.processCancel = cancel
	inputs := support.FakeInputs(processContext, []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	fixture.environment = compositionCustomerEnvironment(homeDir)
	inputs.Input.Env = append([]string(nil), fixture.environment...)
	inputs.Input.WorkingDirectory = hostDir
	fixture.processStarts.Add(1)
	go func() {
		err := process.Execute(inputs.Input)
		fixture.processMu.Lock()
		fixture.processErr = err
		fixture.processMu.Unlock()
		fixture.processStops.Add(1)
		close(fixture.processDone)
	}()

	baseURL, err := api.WaitForBaseURL(compositionFixtureTimeout)
	if err != nil {
		cancel()
		<-fixture.processDone
		_ = process.Close(context.Background())
		_ = os.RemoveAll(hostDir)
		_ = os.RemoveAll(homeDir)
		t.Fatalf("wait for composition API: %v", err)
	}
	fixture.baseURL = baseURL
	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("composition API server starts = %d, want one", got)
	}
	return fixture
}

func writePersistentCompositionFactory(t *testing.T, dir string) {
	t.Helper()
	cfg := parallelCompositionFactoryConfig()
	cfg["name"] = "javascript-composition-shared"
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal composition factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), raw, 0o644); err != nil {
		t.Fatalf("write persistent composition factory: %v", err)
	}
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n")
}

func (fixture *compositionFixture) startAPIServer(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	fixture.apiStarts.Add(1)
	request.Handler = fixture.stream.wrap(request.Handler)
	err := fixture.api.Start(ctx, request)
	fixture.apiStopOnce.Do(func() { close(fixture.apiStopped) })
	return err
}

func compositionCustomerEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func (fixture *compositionFixture) nextRequestID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, fixture.requestNumber.Add(1))
}

func (fixture *compositionFixture) startPublicSync(
	t *testing.T,
	requestID string,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	workflowFile := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: fixture.nextRequestID(requestID),
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowFile,
		},
	})
	if err != nil {
		t.Fatalf("marshal composition sync request %q: %v", requestID, err)
	}
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/sync",
		strings.NewReader(string(payload)),
	)
	if err != nil {
		t.Fatalf("build composition sync request %q: %v", requestID, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start composition sync workflow %q: %v", requestID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("start composition sync workflow %q status = %d: %s", requestID, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode composition sync workflow %q: %v", requestID, err)
	}
	fixture.trackSession(t, started.SessionId, dir)
	return started
}

func (fixture *compositionFixture) publicDispatches(
	t *testing.T,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+sessionID+"/dispatches",
	)
}

func (fixture *compositionFixture) publicEvents(t *testing.T, sessionID string) []factoryapi.FactoryEvent {
	t.Helper()
	return support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
}

func (fixture *compositionFixture) trackLiveSession(t *testing.T, sessionID string) {
	t.Helper()
	fixture.trackSession(t, sessionID, "")
}

func (fixture *compositionFixture) trackSession(t *testing.T, sessionID, rootDir string) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("composition Factory Session ID is empty")
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.sessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("composition Factory Session ID %q was reused", sessionID)
	}
	fixture.sessions[sessionID] = compositionSession{rootDir: rootDir}
	fixture.sessionMu.Unlock()

	t.Cleanup(func() {
		// Durable execution sessions are retained for public inspection after
		// reaching a terminal state; the public lifecycle control is the
		// authoritative cleanup operation for this fixture.
		support.TerminateFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.sessionMu.Lock()
		fixture.closed[sessionID] = struct{}{}
		fixture.sessionMu.Unlock()
	})
}

func (fixture *compositionFixture) shutdown() error {
	if fixture.processCancel != nil {
		fixture.processCancel()
	}
	if fixture.processDone != nil {
		<-fixture.processDone
	}

	closeContext, cancel := context.WithTimeout(context.Background(), compositionFixtureTimeout)
	defer cancel()
	closeErr := fixture.process.Close(closeContext)

	fixture.processMu.Lock()
	processErr := fixture.processErr
	fixture.processMu.Unlock()
	var shutdownErr error
	if processErr != nil && !errors.Is(processErr, context.Canceled) {
		shutdownErr = fmt.Errorf("composition Process.Execute shutdown: %w", processErr)
	}
	if closeErr != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close composition process: %w", closeErr))
	}
	if got := fixture.rootBuilds.Load(); got != 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition root builds = %d, want one", got))
	}
	if got := fixture.processStarts.Load(); got != 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition process starts = %d, want one", got))
	}
	if got := fixture.processStops.Load(); got != 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition process stops = %d, want one", got))
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition API starts = %d, want one", got))
	}
	if got := fixture.runner.activeCount(); got != 0 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition provider command calls still active = %d", got))
	}
	listenerClosed := false
	if fixture.apiStarts.Load() > 0 {
		<-fixture.apiStopped
		listenerClosed = true
	}

	fixture.sessionMu.Lock()
	tracked := len(fixture.sessions)
	closed := len(fixture.closed)
	fixture.sessionMu.Unlock()
	if tracked != closed {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition sessions closed = %d/%d", closed, tracked))
	}

	if strings.TrimSpace(fixture.baseURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			listenerClosed = false
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition API listener remained available after shutdown: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body))))
		}
	}
	streamActive := fixture.stream.active.Load()
	if streamActive != 0 || fixture.stream.opened.Load() != fixture.stream.closed.Load() {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition SSE streams active=%d opened=%d closed=%d", streamActive, fixture.stream.opened.Load(), fixture.stream.closed.Load()))
	}
	processClosed := fixture.processStops.Load() == fixture.processStarts.Load() && closeErr == nil
	processActive := boolToInt(!processClosed)
	portActive := boolToInt(!listenerClosed)
	sessionActive := maxInt(tracked-closed, 0)
	streamActiveCount := int(streamActive)
	routeActive := fixture.runner.activeCount()
	rootActive := maxInt(int(fixture.rootBuilds.Load())-boolToInt(processClosed), 0)
	if err := os.RemoveAll(fixture.hostDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove composition factory: %w", err))
	}
	if err := os.RemoveAll(fixture.homeDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove composition home: %w", err))
	}
	worktreeActive := fixture.activeCompositionRoots()
	if worktreeActive != 0 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition authored roots still active = %d", worktreeActive))
	}
	mutableStateActive := sessionActive + streamActiveCount + routeActive + rootActive + worktreeActive
	if processActive != 0 || portActive != 0 || sessionActive != 0 || streamActiveCount != 0 || routeActive != 0 || rootActive != 0 || worktreeActive != 0 || mutableStateActive != 0 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition active resources process=%d port=%d listener=%d session=%d stream=%d route=%d root=%d worktree=%d mutable-state=%d", processActive, portActive, portActive, sessionActive, streamActiveCount, routeActive, rootActive, worktreeActive, mutableStateActive))
	}
	fmt.Fprintf(os.Stderr, "composition lifecycle report: root_builds=%d process_starts=%d process_stops=%d api_server_starts=%d tracked_sessions=%d closed_sessions=%d active={process:%d port:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d}\n", fixture.rootBuilds.Load(), fixture.processStarts.Load(), fixture.processStops.Load(), fixture.apiStarts.Load(), tracked, closed, processActive, portActive, portActive, sessionActive, streamActiveCount, routeActive, rootActive, worktreeActive, mutableStateActive)
	return shutdownErr
}

func (fixture *compositionFixture) activeCompositionRoots() int {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	seen := make(map[string]struct{}, len(fixture.sessions))
	active := 0
	for _, session := range fixture.sessions {
		rootDir := strings.TrimSpace(session.rootDir)
		if rootDir == "" {
			continue
		}
		rootDir = filepath.Clean(rootDir)
		if _, ok := seen[rootDir]; ok {
			continue
		}
		seen[rootDir] = struct{}{}
		if _, err := os.Stat(rootDir); err == nil || !errors.Is(err, os.ErrNotExist) {
			active++
		}
	}
	return active
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func removeAndObservePath(path string) (int, error) {
	if err := os.RemoveAll(path); err != nil {
		return 1, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 1, err
	}
	return 1, nil
}

var _ platformprocess.CommandRunner = (*compositionCommandRunner)(nil)
