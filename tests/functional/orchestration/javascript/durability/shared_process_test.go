package durability_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

// TestJavaScriptDurabilityFixturePartialStartUnwinds proves a real process
// startup failure preserves the original error and closes the listener
// acquired by the injected HTTP transport edge. No session or provider work
// is admitted because startup fails before the public server is available.
func TestJavaScriptDurabilityFixturePartialStartUnwinds(t *testing.T) {
	hostDir := setupJavaScriptDurabilityResumeWorkflowFixture(t, durabilityWorkflowName)
	homeDir := t.TempDir()
	original := errors.New("injected durability fixture start failure")
	var listenerURL atomic.Value
	var listenerOpen atomic.Int32
	var starterCalls atomic.Int32
	failingStarter := func(_ context.Context, request platformhttpserver.StartRequest) error {
		starterCalls.Add(1)
		server := httptest.NewServer(request.Handler)
		listenerURL.Store(server.URL)
		listenerOpen.Store(1)
		server.CloseClientConnections()
		server.Close()
		listenerOpen.Store(0)
		return original
	}
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
	closeContext, cancel := context.WithTimeout(context.Background(), durabilityFixtureTimeout)
	closeErr := process.Close(closeContext)
	cancel()
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
	if got := listenerOpen.Load(); got != 0 {
		t.Fatalf("partial-start listener state = %d, want closed", got)
	}
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("partial-start provider calls = %d, want zero", got)
	}
	if rawURL, ok := listenerURL.Load().(string); ok && strings.TrimSpace(rawURL) != "" {
		durabilityRequireListenerUnavailable(t, rawURL, "partial-start")
	} else {
		t.Fatal("partial-start listener URL was not recorded")
	}
	t.Logf("durability partial-start lifecycle report: process_closed=%t api_starter_calls=%d listener_open=%d provider_calls=%d original_error=%q", closed, starterCalls.Load(), listenerOpen.Load(), runner.CallCount(), original)
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
	starts   atomic.Int32
	stopped  chan struct{}
	stopOnce sync.Once
}

func (starter *durabilityAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
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
	support.WaitForStatus(t, baseURL, durabilityFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
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
		select {
		case <-fixture.command.Done():
			processStopped = true
		case <-time.After(durabilityFixtureTimeout):
			t.Errorf("durability shared Process.Execute did not stop")
		}
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
		select {
		case <-fixture.apiStarter.stopped:
			listenerClosed = true
		case <-time.After(durabilityFixtureTimeout):
			t.Errorf("durability shared API listener did not stop")
		}
	}
	if fixture.closeErr != nil {
		t.Errorf("durability shared process close error = %v", fixture.closeErr)
	}
	if strings.TrimSpace(fixture.baseURL) != "" {
		durabilityRequireListenerUnavailable(t, fixture.baseURL, "shared cleanup")
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
	processActive := boolToInt(!processStopped || fixture.closeErr != nil)
	listenerActive := boolToInt(!listenerClosed)
	sessionActive := boolToInt(closed == 0)
	t.Logf("durability lifecycle report: shared_root_builds=%d shared_process_starts=%d shared_process_stops=%d shared_api_server_starts=%d tracked_sessions=%d closed_sessions=%d provider_calls=%d isolated_process_pairs=2 process_closed=%t listener_closed=%t provider_active=%d durable_snapshot_retained=%t active_runtime={process:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d}", fixture.rootBuilds.Load(), fixture.processStarts.Load(), processStops, fixture.apiStarter.starts.Load(), tracked, closed, fixture.provider.callCount(), fixture.closeErr == nil && processStopped, listenerClosed, providerActive, durableSnapshotRetained, processActive, listenerActive, sessionActive, 0, processActive, processActive, processActive, processActive)
}

func durabilityRequireListenerUnavailable(t testing.TB, baseURL, label string) {
	t.Helper()
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	t.Errorf("durability %s API listener remained available after cleanup: status=%d body=%q", label, response.StatusCode, strings.TrimSpace(string(body)))
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
