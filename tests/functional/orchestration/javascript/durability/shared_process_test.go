package durability_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	durabilityFixtureTimeout = 15 * time.Second
	durabilityWorkflowName   = "resumable-two-step-fake-children"
)

// durabilityStreamLifecycle observes the actual public SSE handler lifetime
// at the injected HTTP edge. It is not an application-owned resource ledger.
type durabilityStreamLifecycle struct {
	active          atomic.Int32
	opened          atomic.Int32
	closed          atomic.Int32
	sessionRequests atomic.Int32
}

func (lifecycle *durabilityStreamLifecycle) wrap(next http.Handler) http.Handler {
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

// TestJavaScriptDurabilityFixturePartialStartUnwinds proves a real process
// startup failure preserves the original error and closes the listener
// acquired by the injected HTTP transport edge. No session or provider work
// is admitted because startup fails before the public server is available.
func TestJavaScriptDurabilityFixturePartialStartUnwinds(t *testing.T) {
	hostDir := setupJavaScriptDurabilityResumeWorkflowFixture(t, durabilityWorkflowName)
	homeDir := t.TempDir()
	original := errors.New("injected durability fixture start failure")
	partialAPI := support.NewProcessAPIServer()
	partialStopped := make(chan struct{})
	partialStreams := &durabilityStreamLifecycle{}
	var listenerURL string
	var starterCalls atomic.Int32
	failingStarter := newDurabilityPartialStartStarter(original, partialAPI, partialStopped, partialStreams, &listenerURL, &starterCalls)
	runner := support.NewRecordingCommandRunner("unexpected durability provider execution")
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      failingStarter,
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess(durability partial start): %v", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = process.Close(context.Background())
		}
	}()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = durabilityCustomerEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	got := process.Execute(inputs.Input)
	closeErr := process.Close(context.Background())
	if closeErr != nil {
		t.Fatalf("close durability partial-start process: %v", closeErr)
	}
	closed = true
	if !errors.Is(got, original) && !strings.Contains(durabilityErrorText(got), original.Error()) {
		t.Fatalf("partial-start error = %v, want original error %v", got, original)
	}
	if got := starterCalls.Load(); got != 1 {
		t.Fatalf("partial-start API starter calls = %d, want one", got)
	}
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("partial-start provider calls = %d, want zero", got)
	}
	listenerClosed := false
	<-partialStopped
	listenerClosed = true
	if partialStreams.active.Load() != 0 || partialStreams.opened.Load() != partialStreams.closed.Load() {
		t.Fatalf("partial durability stream edge did not close: active=%d opened=%d closed=%d", partialStreams.active.Load(), partialStreams.opened.Load(), partialStreams.closed.Load())
	}
	if strings.TrimSpace(listenerURL) != "" {
		listenerClosed = durabilityRequireListenerUnavailable(t, listenerURL, "partial-start")
	} else {
		t.Fatal("partial-start listener URL was not recorded")
	}
	sessionActive := int(partialStreams.sessionRequests.Load())
	if sessionActive != 0 {
		t.Fatalf("partial-start Factory Session requests = %d, want zero", sessionActive)
	}
	processActive := boolToInt(!closed)
	portActive := boolToInt(!listenerClosed)
	streamActive := int(partialStreams.active.Load())
	routeActive := boolToInt(runner.CallCount() != 0)
	rootActive := boolToInt(!closed)
	worktreeActive, err := removeAndObservePath(hostDir)
	if err != nil {
		t.Fatalf("remove durability partial-start factory: %v", err)
	}
	mutableStateActive := sessionActive + streamActive + routeActive + rootActive + worktreeActive
	if processActive != 0 || portActive != 0 || streamActive != 0 || routeActive != 0 || rootActive != 0 || worktreeActive != 0 || mutableStateActive != 0 {
		t.Fatalf("durability partial-start active resources process=%d port=%d listener=%d session=%d stream=%d route=%d root=%d worktree=%d mutable-state=%d", processActive, portActive, portActive, sessionActive, streamActive, routeActive, rootActive, worktreeActive, mutableStateActive)
	}
	t.Logf("durability partial-start lifecycle report: process_closed=%t api_starter_calls=%d active={process:%d port:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d} streams_opened=%d streams_closed=%d provider_calls=%d original_error=%q", closed, starterCalls.Load(), processActive, portActive, portActive, sessionActive, streamActive, routeActive, rootActive, worktreeActive, mutableStateActive, partialStreams.opened.Load(), partialStreams.closed.Load(), runner.CallCount(), original)
}

func newDurabilityPartialStartStarter(
	original error,
	api *support.ProcessAPIServer,
	stopped chan struct{},
	streams *durabilityStreamLifecycle,
	listenerURL *string,
	starterCalls *atomic.Int32,
) func(context.Context, platformhttpserver.StartRequest) error {
	return func(ctx context.Context, request platformhttpserver.StartRequest) error {
		starterCalls.Add(1)
		partialContext, cancelPartial := context.WithCancel(ctx)
		request.OnBound = nil
		request.Handler = streams.wrap(request.Handler)
		go func() {
			_ = api.Start(partialContext, request)
			close(stopped)
		}()
		baseURL, err := api.WaitForBaseURL(durabilityFixtureTimeout)
		if err != nil {
			cancelPartial()
			<-stopped
			return err
		}
		*listenerURL = baseURL
		cancelPartial()
		<-stopped
		return original
	}
}

func durabilityErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// TestJavaScriptDurabilityPersistsSnapshotsByDefault preserves the original
// top-level witness while routing its eligible snapshot row through one
// package-owned root process. The two interruption/resume witnesses remain
// separate top-level tests because their blocked recovery lifecycles are C01
// process-sensitive.
func TestJavaScriptDurabilityPersistsSnapshotsByDefault(t *testing.T) {
	runJavaScriptDurabilityPersistsSnapshotsByDefault(t, newDurabilityFixture(t))
}

type durabilityFixture struct {
	process     support.ApplicationProcess
	api         *support.ProcessAPIServer
	apiStarter  *durabilityAPIServerStarter
	provider    *javascriptDurabilityResumeBlockingCommandRunner
	baseURL     string
	projectRoot string
	homeDir     string
	command     *support.ProcessCommand

	rootBuilds    atomic.Int32
	processStarts atomic.Int32
	stopOnce      sync.Once

	sessionMu sync.Mutex
	session   durabilitySession
	closeErr  error
}

type durabilitySession struct {
	id        string
	requestID string
	rootDir   string
}

type durabilityAPIServerStarter struct {
	api      *support.ProcessAPIServer
	streams  *durabilityStreamLifecycle
	starts   atomic.Int32
	stopped  chan struct{}
	stopOnce sync.Once
}

func (starter *durabilityAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	request.Handler = starter.streams.wrap(request.Handler)
	err := starter.api.Start(ctx, request)
	starter.stopOnce.Do(func() { close(starter.stopped) })
	return err
}

func newDurabilityFixture(t *testing.T) *durabilityFixture {
	t.Helper()

	projectRoot := setupJavaScriptDurabilityResumeWorkflowFixture(t, durabilityWorkflowName)
	provider := newJavaScriptDurabilityResumeBlockingCommandRunner(durabilityWorkflowName)
	homeDir := t.TempDir()
	api := support.NewProcessAPIServer()
	apiStarter := &durabilityAPIServerStarter{
		api:     api,
		streams: &durabilityStreamLifecycle{},
		stopped: make(chan struct{}),
	}
	fixture := &durabilityFixture{
		api:         api,
		apiStarter:  apiStarter,
		provider:    provider,
		projectRoot: projectRoot,
		homeDir:     homeDir,
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      apiStarter.Start,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(durability): %v", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)

	// Cleanup is registered before the command so the command stops first,
	// then the root closes, and finally the report observes real terminal state.
	t.Cleanup(func() { fixture.assertCleanup(t) })
	t.Cleanup(func() { fixture.closeProcess(t) })
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", projectRoot, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = durabilityCustomerEnvironment(homeDir)
	inputs.Input.WorkingDirectory = projectRoot
	fixture.processStarts.Add(1)
	fixture.command = support.StartProcessCommand(t, process, inputs.Input)

	baseURL, err := api.WaitForBaseURL(durabilityFixtureTimeout)
	if err != nil {
		t.Fatalf("wait for durability API: %v", err)
	}
	fixture.baseURL = baseURL
	if got := apiStarter.starts.Load(); got != 1 {
		t.Fatalf("durability API server starts = %d, want one", got)
	}
	return fixture
}

func durabilityCustomerEnvironment(homeDir string) []string {
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

func runJavaScriptDurabilityPersistsSnapshotsByDefault(t *testing.T, fixture *durabilityFixture) {
	t.Helper()

	sessionID := startInterruptedJavaScriptDurabilitySession(
		t,
		fixture.baseURL,
		fixture.provider,
		durabilityWorkflowName,
	)
	fixture.trackSession(t, sessionID, javascriptDurabilityResumeRequestID, fixture.projectRoot)

	interrupted := readDurableJavaScriptSession(t, fixture.baseURL, sessionID)
	if interrupted.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("in-memory status = %q, want INTERRUPTED", interrupted.Status)
	}
	if interrupted.Lifecycle == nil || interrupted.Lifecycle.InterruptedAt == nil {
		t.Fatalf("in-memory lifecycle = %#v, want interruptedAt", interrupted.Lifecycle)
	}

	// The public interrupted projection is published before project-local
	// persistence is inspected. Stop the hosted command at this real process
	// boundary before checking the durable snapshot.
	fixture.stopHostedProcess(t)
	assertJavaScriptDurableSessionPersistence(t, fixture.projectRoot, sessionID)
}

func (fixture *durabilityFixture) stopHostedProcess(t testing.TB) {
	t.Helper()
	fixture.stopOnce.Do(func() { fixture.command.Stop(t) })
}

func (fixture *durabilityFixture) trackSession(
	t testing.TB,
	sessionID, requestID, rootDir string,
) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("durability Factory Session ID is empty")
	}
	if strings.TrimSpace(rootDir) == "" {
		t.Fatal("durability Factory Session root is empty")
	}
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if fixture.session.id != "" {
		t.Fatalf("durability Factory Session ID %q was already tracked", fixture.session.id)
	}
	fixture.session = durabilitySession{id: sessionID, requestID: requestID, rootDir: rootDir}
}

func (fixture *durabilityFixture) closeProcess(t testing.TB) {
	t.Helper()
	closeContext, cancel := context.WithTimeout(context.Background(), durabilityFixtureTimeout)
	defer cancel()
	fixture.closeErr = fixture.process.Close(closeContext)
	if fixture.closeErr != nil {
		t.Errorf("close durability application process: %v", fixture.closeErr)
	}
}

func (fixture *durabilityFixture) assertCleanup(t testing.TB) {
	t.Helper()
	if got := fixture.rootBuilds.Load(); got != 1 {
		t.Errorf("durability shared root builds = %d, want one", got)
	}
	if got := fixture.processStarts.Load(); got != 1 {
		t.Errorf("durability shared process starts = %d, want one", got)
	}
	processStopped := false
	if fixture.command != nil {
		<-fixture.command.Done()
		processStopped = true
	}
	processStops := 0
	if processStopped {
		processStops = 1
	}
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Errorf("durability shared API starts = %d, want one", got)
	}
	listenerClosed := false
	if fixture.apiStarter.starts.Load() > 0 {
		<-fixture.apiStarter.stopped
		listenerClosed = true
	}
	if fixture.closeErr != nil {
		t.Errorf("durability shared process close error = %v", fixture.closeErr)
	}
	if strings.TrimSpace(fixture.baseURL) != "" {
		listenerClosed = durabilityRequireListenerUnavailable(t, fixture.baseURL, "shared cleanup")
	}
	providerActive := fixture.provider.activeCount()
	if providerActive != 0 {
		t.Errorf("durability shared provider commands still active = %d, want zero", providerActive)
	}
	fixture.sessionMu.Lock()
	tracked := 0
	if fixture.session.id != "" {
		tracked = 1
	}
	fixture.sessionMu.Unlock()
	closed := 0
	if tracked == 1 && processStopped && fixture.closeErr == nil {
		closed = 1
	}
	if tracked != closed {
		t.Errorf("durability shared sessions closed = %d/%d", closed, tracked)
	}
	durableSnapshotRetained := false
	fixture.sessionMu.Lock()
	if fixture.session.id != "" {
		_, err := os.Stat(javaScriptDurableSessionPersistencePath(fixture.projectRoot, fixture.session.id))
		durableSnapshotRetained = err == nil
	}
	fixture.sessionMu.Unlock()
	processClosed := processStopped && fixture.closeErr == nil
	processActive := boolToInt(!processClosed)
	listenerActive := boolToInt(!listenerClosed)
	sessionActive := boolToInt(closed != tracked)
	streamActive := fixture.apiStarter.streams.active.Load()
	if streamActive != 0 || fixture.apiStarter.streams.opened.Load() != fixture.apiStarter.streams.closed.Load() {
		t.Errorf("durability shared SSE streams active=%d opened=%d closed=%d", streamActive, fixture.apiStarter.streams.opened.Load(), fixture.apiStarter.streams.closed.Load())
	}
	routeActive := providerActive
	rootActive := maxInt(int(fixture.rootBuilds.Load())-boolToInt(processClosed), 0)
	worktreeActive, err := removeAndObservePath(fixture.projectRoot)
	if err != nil {
		t.Errorf("remove durability project root: %v", err)
	}
	mutableStateActive := sessionActive + int(streamActive) + routeActive + rootActive + worktreeActive
	if processActive != 0 || listenerActive != 0 || sessionActive != 0 || streamActive != 0 || routeActive != 0 || rootActive != 0 || worktreeActive != 0 || mutableStateActive != 0 {
		t.Errorf("durability active resources process=%d listener=%d session=%d stream=%d route=%d root=%d worktree=%d mutable-state=%d", processActive, listenerActive, sessionActive, streamActive, routeActive, rootActive, worktreeActive, mutableStateActive)
	}
	t.Logf("durability lifecycle report: shared_root_builds=%d shared_process_starts=%d shared_process_stops=%d shared_api_server_starts=%d tracked_sessions=%d closed_sessions=%d provider_calls=%d isolated_process_pairs=2 process_closed=%t listener_closed=%t provider_active=%d durable_snapshot_retained=%t active_runtime={process:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d}", fixture.rootBuilds.Load(), fixture.processStarts.Load(), processStops, fixture.apiStarter.starts.Load(), tracked, closed, fixture.provider.callCount(), processClosed, listenerClosed, providerActive, durableSnapshotRetained, processActive, listenerActive, sessionActive, streamActive, routeActive, rootActive, worktreeActive, mutableStateActive)
}

func durabilityRequireListenerUnavailable(t testing.TB, baseURL, label string) bool {
	t.Helper()
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return true
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	t.Errorf("durability %s API listener remained available after cleanup: status=%d body=%q", label, response.StatusCode, strings.TrimSpace(string(body)))
	return false
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

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
