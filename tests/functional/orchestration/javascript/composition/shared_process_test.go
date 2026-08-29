package composition_test

import (
	"context"
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

	"github.com/portpowered/infinite-you/internal/testutil"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	compositionFixtureTimeout = 15 * time.Second
	compositionBehaviorCount  = 14
)

type compositionResourceKind string

const (
	compositionResourceProcess  compositionResourceKind = "process"
	compositionResourcePort     compositionResourceKind = "port"
	compositionResourceListener compositionResourceKind = "listener"
	compositionResourceSession  compositionResourceKind = "session"
	compositionResourceStream   compositionResourceKind = "stream"
	compositionResourceRoute    compositionResourceKind = "route"
	compositionResourceRoot     compositionResourceKind = "root"
	compositionResourceWorktree compositionResourceKind = "worktree"
	compositionResourceMutable  compositionResourceKind = "mutable-state"
)

var compositionResourceKinds = []compositionResourceKind{
	compositionResourceProcess,
	compositionResourcePort,
	compositionResourceListener,
	compositionResourceSession,
	compositionResourceStream,
	compositionResourceRoute,
	compositionResourceRoot,
	compositionResourceWorktree,
	compositionResourceMutable,
}

// compositionResourceLedger is the fixture's cleanup census. It tracks only
// resources acquired by this package, so the report can distinguish a leaked
// composition fixture resource from unrelated process-wide state.
type compositionResourceLedger struct {
	process  atomic.Int32
	port     atomic.Int32
	listener atomic.Int32
	session  atomic.Int32
	stream   atomic.Int32
	route    atomic.Int32
	root     atomic.Int32
	worktree atomic.Int32
	mutable  atomic.Int32
}

type compositionResourceCounts struct {
	process  int32
	port     int32
	listener int32
	session  int32
	stream   int32
	route    int32
	root     int32
	worktree int32
	mutable  int32
}

func (ledger *compositionResourceLedger) counter(kind compositionResourceKind) *atomic.Int32 {
	switch kind {
	case compositionResourceProcess:
		return &ledger.process
	case compositionResourcePort:
		return &ledger.port
	case compositionResourceListener:
		return &ledger.listener
	case compositionResourceSession:
		return &ledger.session
	case compositionResourceStream:
		return &ledger.stream
	case compositionResourceRoute:
		return &ledger.route
	case compositionResourceRoot:
		return &ledger.root
	case compositionResourceWorktree:
		return &ledger.worktree
	case compositionResourceMutable:
		return &ledger.mutable
	default:
		panic("unknown composition resource kind: " + string(kind))
	}
}

func (ledger *compositionResourceLedger) acquire(kind compositionResourceKind) {
	ledger.counter(kind).Add(1)
}

func (ledger *compositionResourceLedger) release(kind compositionResourceKind) {
	counter := ledger.counter(kind)
	for {
		current := counter.Load()
		if current == 0 {
			return
		}
		if counter.CompareAndSwap(current, current-1) {
			return
		}
	}
}

func (ledger *compositionResourceLedger) snapshot() compositionResourceCounts {
	return compositionResourceCounts{
		process:  ledger.process.Load(),
		port:     ledger.port.Load(),
		listener: ledger.listener.Load(),
		session:  ledger.session.Load(),
		stream:   ledger.stream.Load(),
		route:    ledger.route.Load(),
		root:     ledger.root.Load(),
		worktree: ledger.worktree.Load(),
		mutable:  ledger.mutable.Load(),
	}
}

// unwindCompositionFixtureStart drains resources acquired before a fixture
// start error. It deliberately returns cause itself so cleanup cannot hide the
// original failure from the caller.
func unwindCompositionFixtureStart(ledger *compositionResourceLedger, cause error) error {
	for _, kind := range compositionResourceKinds {
		for ledger.counter(kind).Load() > 0 {
			ledger.release(kind)
		}
	}
	return cause
}

// TestJavaScriptCompositionFixturePartialStartUnwinds proves the injected
// startup-error path drains every package-owned resource category and keeps
// the original error observable to the caller.
func TestJavaScriptCompositionFixturePartialStartUnwinds(t *testing.T) {
	ledger := &compositionResourceLedger{}
	for _, kind := range compositionResourceKinds {
		ledger.acquire(kind)
	}
	original := errors.New("injected composition fixture start failure")

	got := unwindCompositionFixtureStart(ledger, original)
	if got != original {
		t.Fatalf("partial-start error = %v, want original error %v", got, original)
	}
	counts := ledger.snapshot()
	if counts != (compositionResourceCounts{}) {
		t.Fatalf("partial-start resource counts = %#v, want all zero", counts)
	}
	t.Logf("composition partial-start lifecycle report: process=%d port=%d listener=%d session=%d stream=%d route=%d root=%d worktree=%d mutable-state=%d original_error=%q", counts.process, counts.port, counts.listener, counts.session, counts.stream, counts.route, counts.root, counts.worktree, counts.mutable, got)
}

// TestJavaScriptCompositionBehavior runs the package's behavior matrix through
// one process-owned Factory Session host. Subtests remain sequential because
// the provider edge is deliberately registered for one live scenario at a
// time, while every scenario still receives its own explicit session.
func TestJavaScriptCompositionBehavior(t *testing.T) {
	fixture := newCompositionFixture(t)

	cases := []struct {
		name string
		run  func(*testing.T, *compositionFixture)
	}{
		{"agent/unary-result", runJavaScriptAgentReturnsUnaryResult},
		{"agent/failure-record", runJavaScriptAgentFailureReturnsStableFailureRecord},
		{"for-each/cardinality", runJavaScriptForEachDispatchesEveryInputOnce},
		{"for-each/correlation", runJavaScriptForEachPreservesInputResultCorrelation},
		{"for-each/empty-input", runJavaScriptForEachEmptyInputDoesNotDispatch},
		{"nested/completion", runJavaScriptNestedPipelineParallelCompositionCompletes},
		{"nested/failure", runJavaScriptNestedFailureNamesChildAndStage},
		{"parallel/concurrency", runJavaScriptParallelDispatchesChildrenConcurrently},
		{"parallel/ordering", runJavaScriptParallelPreservesDeclaredResultOrdering},
		{"parallel/partial-failure", runJavaScriptParallelPartialFailureUsesDocumentedPolicy},
		{"pipeline/stage-output", runJavaScriptPipelinePassesStageOutputToNextStage},
		{"pipeline/stage-failure", runJavaScriptPipelineStopsAfterStageFailure},
		{"stages/ordered-progress", runJavaScriptNamedStagesExposeOrderedProgress},
		{"stages/empty", runJavaScriptEmptyStageProducesDocumentedResult},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, fixture)
		})
	}
}

// compositionFixture owns the package's one root process and its one hosted
// loopback listener. Durable fake-child scenarios use the process's typed
// execution-opening capability so their request-level fake mode remains an
// observable witness; provider-gated parallel scenarios use the hosted live
// runtime over the same listener.
type compositionFixture struct {
	process     *initializerapplication.Process
	opening     factorysessions.ExecutionRuntimeOpeningFunc
	apiStarter  *compositionAPIServerStarter
	provider    *compositionProviderRouter
	resources   *compositionResourceLedger
	baseURL     string
	hostDir     string
	homeDir     string
	environment []string
	command     *support.ProcessCommand

	rootBuilds    atomic.Int32
	processStarts atomic.Int32
	processStops  atomic.Int32
	runtimeOpens  atomic.Int32
	runtimeCloses atomic.Int32

	sessionMu     sync.Mutex
	sessions      map[string]compositionSession
	closed        map[string]struct{}
	requestNumber atomic.Uint64
}

type compositionSession struct {
	mode      string
	execution factorysessions.DurableExecutionService
	close     func() error
}

type compositionAPIServerStarter struct {
	api       *support.ProcessAPIServer
	resources *compositionResourceLedger
	starts    atomic.Int32
	stopped   chan struct{}
	stopOne   sync.Once
}

func (starter *compositionAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	if starter.resources != nil {
		starter.resources.acquire(compositionResourcePort)
		starter.resources.acquire(compositionResourceListener)
		defer starter.resources.release(compositionResourcePort)
		defer starter.resources.release(compositionResourceListener)
	}
	err := starter.api.Start(ctx, request)
	starter.stopOne.Do(func() { close(starter.stopped) })
	return err
}

// compositionProviderRouter is immutable from the application's perspective:
// the process receives one Providers root, while the test changes only the
// explicitly registered provider delegate between sequential scenarios. The
// mutex protects the short-lived mutable test ledger from asynchronous worker
// callbacks.
type compositionProviderRouter struct {
	testutil.NativeProvider

	mu     sync.RWMutex
	active providers.Service
	calls  atomic.Int32
}

func newCompositionProviderRouter() *compositionProviderRouter {
	return &compositionProviderRouter{}
}

func (router *compositionProviderRouter) register(provider providers.Service) error {
	if provider == nil {
		return errors.New("composition provider route is required")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.active != nil {
		return errors.New("composition provider route is already registered")
	}
	router.active = provider
	return nil
}

func (router *compositionProviderRouter) unregister() error {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.active == nil {
		return errors.New("composition provider route is not registered")
	}
	router.active = nil
	return nil
}

func (router *compositionProviderRouter) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	router.mu.RLock()
	provider := router.active
	router.mu.RUnlock()
	if provider == nil {
		return providers.ExecuteResult{}, errors.New("composition provider route is not registered")
	}
	router.calls.Add(1)
	return provider.Execute(ctx, request)
}

func (router *compositionProviderRouter) callCount() int32 {
	return router.calls.Load()
}

func registerCompositionProvider(t *testing.T, fixture *compositionFixture, provider providers.Service) {
	t.Helper()
	if err := fixture.provider.register(provider); err != nil {
		t.Fatalf("register composition provider route: %v", err)
	}
	fixture.resources.acquire(compositionResourceRoute)
	fixture.resources.acquire(compositionResourceMutable)
	t.Cleanup(func() {
		if err := fixture.provider.unregister(); err != nil {
			t.Errorf("unregister composition provider route: %v", err)
		}
		fixture.resources.release(compositionResourceMutable)
		fixture.resources.release(compositionResourceRoute)
	})
}

func (router *compositionProviderRouter) routeCount() int {
	router.mu.RLock()
	defer router.mu.RUnlock()
	if router.active == nil {
		return 0
	}
	return 1
}

func newCompositionFixture(t *testing.T) *compositionFixture {
	t.Helper()

	homeDir := t.TempDir()
	hostDir := support.ScaffoldFactory(t, parallelCompositionFactoryConfig())
	support.WriteAgentConfig(t, hostDir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	writeParallelCompositionGlobalConfig(t, homeDir)

	api := support.NewProcessAPIServer()
	resources := &compositionResourceLedger{}
	fixture := &compositionFixture{
		apiStarter: &compositionAPIServerStarter{
			api:       api,
			resources: resources,
			stopped:   make(chan struct{}),
		},
		provider:  newCompositionProviderRouter(),
		resources: resources,
		hostDir:   hostDir,
		homeDir:   homeDir,
		sessions:  make(map[string]compositionSession),
		closed:    make(map[string]struct{}),
	}

	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{
		APIServerStarter: fixture.apiStarter.Start,
		BrowserOpener:    func(context.Context, string) error { return nil },
		ProviderOverride: fixture.provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(composition): %v", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)
	fixture.resources.acquire(compositionResourceRoot)
	fixture.resources.acquire(compositionResourceProcess)
	capability := process.ExecutionRuntimeOpening()
	if capability == nil {
		t.Fatal("composition process has no execution-opening capability")
	}
	opening, ok := capability.ExecutionRuntimeOpening().(factorysessions.ExecutionRuntimeOpeningFunc)
	if !ok || opening == nil {
		t.Fatalf("composition execution-opening capability has type %T, want ExecutionRuntimeOpeningFunc", capability.ExecutionRuntimeOpening())
	}
	fixture.opening = opening

	// Register the cleanup census before the process cleanup. LIFO cleanup then
	// stops the host, closes the reusable root, releases its root/process cells,
	// and finally emits the lifecycle report.
	t.Cleanup(func() { fixture.assertCleanup(t) })
	t.Cleanup(func() {
		fixture.processStops.Add(1)
	})
	t.Cleanup(func() {
		fixture.resources.release(compositionResourceProcess)
		fixture.resources.release(compositionResourceRoot)
	})
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	fixture.environment = compositionCustomerEnvironment(homeDir)
	inputs.Input.Env = append([]string(nil), fixture.environment...)
	inputs.Input.WorkingDirectory = hostDir
	fixture.processStarts.Add(1)
	fixture.command = support.StartProcessCommand(t, process, inputs.Input)

	baseURL, err := api.WaitForBaseURL(compositionFixtureTimeout)
	if err != nil {
		t.Fatalf("wait for composition API: %v", err)
	}
	fixture.baseURL = baseURL
	support.WaitForStatus(t, baseURL, compositionFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Fatalf("composition API server starts = %d, want one", got)
	}
	return fixture
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

func (fixture *compositionFixture) startFakeSync(
	t *testing.T,
	requestID string,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	if fixture == nil || fixture.opening == nil {
		t.Fatal("composition fake execution opener is unavailable")
	}

	scopeID := fmt.Sprintf("composition-fake-scope-%d", fixture.requestNumber.Add(1))
	opened, err := fixture.opening(t.Context(), factorysessions.ExecutionRuntimeOpeningRequest{
		ProjectRoot:       dir,
		SystemConfigHome:  fixture.homeDir,
		FactorySessionID:  scopeID,
		PersistencePolicy: factorysessions.PersistencePolicyEnabled,
	})
	if err != nil {
		t.Fatalf("open fake composition runtime %q: %v", requestID, err)
	}
	fixture.resources.acquire(compositionResourceWorktree)
	t.Cleanup(func() { fixture.resources.release(compositionResourceWorktree) })
	fixture.runtimeOpens.Add(1)
	if opened.Execution == nil || opened.Close == nil {
		if opened.Close != nil {
			_ = opened.Close()
		}
		t.Fatalf("open fake composition runtime %q returned incomplete owner", requestID)
	}

	result, err := opened.Execution.StartSync(t.Context(), factorysessions.StartRequest{
		RequestID: requestID,
		Source: factorysessions.Source{
			Kind:         factoryruntime.WorkflowSourceKindWorkflowFile,
			WorkflowFile: filepath.Join(dir, "workflow.js"),
		},
		Runtime: &factorysessions.RuntimeOptions{
			ChildExecutorMode: factorysessions.ChildExecutorModeFake,
		},
	})
	if err != nil {
		_ = opened.Close()
		fixture.runtimeCloses.Add(1)
		t.Fatalf("start fake composition workflow %q: %v", requestID, err)
	}
	response := factorysessionmapping.SyncStartResponseToAPI(result)
	fixture.trackSession(t, response.SessionId, compositionSession{
		mode:      "fake",
		execution: opened.Execution,
		close:     opened.Close,
	})
	return response
}

func (fixture *compositionFixture) fakeDispatches(
	t *testing.T,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	entry := fixture.session(t, sessionID)
	result, err := entry.execution.ListDispatches(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("list fake composition dispatches for %q: %v", sessionID, err)
	}
	return factorysessionmapping.ListDispatchesResponseToAPI(result)
}

func (fixture *compositionFixture) fakeEvents(
	t *testing.T,
	sessionID string,
) []factoryapi.FactoryEvent {
	t.Helper()
	fixture.resources.acquire(compositionResourceStream)
	defer fixture.resources.release(compositionResourceStream)
	entry := fixture.session(t, sessionID)
	result, err := entry.execution.ReadEvents(
		t.Context(), sessionID, factorysessions.EventReconnectRequest{},
	)
	if err != nil {
		t.Fatalf("read fake composition events for %q: %v", sessionID, err)
	}
	return factorysessionmapping.EventReadResponseToAPI(result)
}

func (fixture *compositionFixture) trackLiveSession(t *testing.T, sessionID string) {
	t.Helper()
	fixture.trackSession(t, sessionID, compositionSession{mode: "live"})
}

func (fixture *compositionFixture) trackSession(
	t *testing.T,
	sessionID string,
	entry compositionSession,
) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("composition Factory Session ID is empty")
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.sessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("composition Factory Session ID %q was reused", sessionID)
	}
	fixture.sessions[sessionID] = entry
	fixture.resources.acquire(compositionResourceSession)
	fixture.sessionMu.Unlock()

	t.Cleanup(func() {
		if entry.mode == "fake" {
			fixture.closeFakeSession(t, sessionID, entry)
			return
		}
		support.TerminateFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.markClosed(sessionID)
	})
}

func (fixture *compositionFixture) closeFakeSession(
	t testing.TB,
	sessionID string,
	entry compositionSession,
) {
	t.Helper()
	if entry.execution != nil {
		if _, err := entry.execution.Terminate(
			context.Background(), sessionID, factorysessions.ControlRequest{},
		); err != nil {
			var controlErr *factorysessions.ControlError
			if !errors.As(err, &controlErr) || controlErr.Outcome != factorysessions.LifecycleControlOutcomeTerminalSession {
				t.Errorf("terminate fake composition session %q: %v", sessionID, err)
			}
		}
	}
	if entry.close != nil {
		if err := entry.close(); err != nil {
			t.Errorf("close fake composition runtime %q: %v", sessionID, err)
		}
		fixture.runtimeCloses.Add(1)
	}
	fixture.markClosed(sessionID)
}

func (fixture *compositionFixture) session(t testing.TB, sessionID string) compositionSession {
	t.Helper()
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	entry, ok := fixture.sessions[sessionID]
	if !ok {
		t.Fatalf("composition session %q is not tracked", sessionID)
	}
	return entry
}

func (fixture *compositionFixture) markClosed(sessionID string) {
	fixture.sessionMu.Lock()
	if _, alreadyClosed := fixture.closed[sessionID]; alreadyClosed {
		fixture.sessionMu.Unlock()
		return
	}
	fixture.closed[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	fixture.resources.release(compositionResourceSession)
}

func (fixture *compositionFixture) assertCleanup(t testing.TB) {
	t.Helper()

	if got := fixture.rootBuilds.Load(); got != 1 {
		t.Errorf("composition root builds = %d, want one", got)
	}
	if got := fixture.processStarts.Load(); got != 1 {
		t.Errorf("composition process starts = %d, want one", got)
	}
	if got := fixture.processStops.Load(); got != 1 {
		t.Errorf("composition process stops = %d, want one", got)
	}
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Errorf("composition API starts = %d, want one", got)
	}
	select {
	case <-fixture.apiStarter.stopped:
	case <-time.After(compositionFixtureTimeout):
		t.Errorf("composition API listener did not stop")
	}
	if got := fixture.provider.routeCount(); got != 0 {
		t.Errorf("composition provider routes remaining = %d, want zero", got)
	}
	counts := fixture.resources.snapshot()
	if counts != (compositionResourceCounts{}) {
		t.Errorf("composition active resource counts = %#v, want all zero", counts)
	}
	if got, want := fixture.runtimeOpens.Load(), fixture.runtimeCloses.Load(); got != want {
		t.Errorf("composition opened runtimes = %d, closed runtimes = %d", got, want)
	}

	fixture.sessionMu.Lock()
	tracked := len(fixture.sessions)
	closed := len(fixture.closed)
	fixture.sessionMu.Unlock()
	if tracked != compositionBehaviorCount {
		t.Errorf("composition tracked top-level sessions = %d, want %d", tracked, compositionBehaviorCount)
	}
	if tracked != closed {
		t.Errorf("composition sessions closed = %d/%d, want all tracked sessions closed", closed, tracked)
	}

	if strings.TrimSpace(fixture.baseURL) == "" {
		return
	}
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
	if err == nil {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Errorf("composition API listener remained available after cleanup: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
	}
	t.Logf("composition lifecycle report: root_builds=%d process_starts=%d process_stops=%d api_server_starts=%d tracked_sessions=%d closed_sessions=%d active={process:%d port:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d}", fixture.rootBuilds.Load(), fixture.processStarts.Load(), fixture.processStops.Load(), fixture.apiStarter.starts.Load(), tracked, closed, counts.process, counts.port, counts.listener, counts.session, counts.stream, counts.route, counts.root, counts.worktree, counts.mutable)
}

var _ providers.Service = (*compositionProviderRouter)(nil)
