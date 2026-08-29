package durability_test

import (
	"context"
	"errors"
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
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	durabilityFixtureTimeout = 15 * time.Second
	durabilityBehaviorCount  = 1
	durabilityWorkflowName   = "resumable-two-step-fake-children"
)

type durabilityResourceKind string

const (
	durabilityResourceProcess  durabilityResourceKind = "process"
	durabilityResourcePort     durabilityResourceKind = "port"
	durabilityResourceListener durabilityResourceKind = "listener"
	durabilityResourceSession  durabilityResourceKind = "session"
	durabilityResourceStream   durabilityResourceKind = "stream"
	durabilityResourceRoute    durabilityResourceKind = "route"
	durabilityResourceRoot     durabilityResourceKind = "root"
	durabilityResourceWorktree durabilityResourceKind = "worktree"
	durabilityResourceMutable  durabilityResourceKind = "mutable-state"
)

var durabilityResourceKinds = []durabilityResourceKind{
	durabilityResourceProcess,
	durabilityResourcePort,
	durabilityResourceListener,
	durabilityResourceSession,
	durabilityResourceStream,
	durabilityResourceRoute,
	durabilityResourceRoot,
	durabilityResourceWorktree,
	durabilityResourceMutable,
}

// durabilityResourceLedger is scoped to the eligible shared fixture. It
// records only resources acquired by this package so the teardown report does
// not imply ownership of unrelated process-wide state.
type durabilityResourceLedger struct {
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

type durabilityResourceCounts struct {
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

func (ledger *durabilityResourceLedger) counter(kind durabilityResourceKind) *atomic.Int32 {
	switch kind {
	case durabilityResourceProcess:
		return &ledger.process
	case durabilityResourcePort:
		return &ledger.port
	case durabilityResourceListener:
		return &ledger.listener
	case durabilityResourceSession:
		return &ledger.session
	case durabilityResourceStream:
		return &ledger.stream
	case durabilityResourceRoute:
		return &ledger.route
	case durabilityResourceRoot:
		return &ledger.root
	case durabilityResourceWorktree:
		return &ledger.worktree
	case durabilityResourceMutable:
		return &ledger.mutable
	default:
		panic("unknown durability resource kind: " + string(kind))
	}
}

func (ledger *durabilityResourceLedger) acquire(kind durabilityResourceKind) {
	ledger.counter(kind).Add(1)
}

func (ledger *durabilityResourceLedger) release(kind durabilityResourceKind) {
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

func (ledger *durabilityResourceLedger) snapshot() durabilityResourceCounts {
	return durabilityResourceCounts{
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

// unwindDurabilityFixtureStart returns the injected cause unchanged after
// draining every resource cell acquired before a shared-fixture start error.
func unwindDurabilityFixtureStart(ledger *durabilityResourceLedger, cause error) error {
	for _, kind := range durabilityResourceKinds {
		for ledger.counter(kind).Load() > 0 {
			ledger.release(kind)
		}
	}
	return cause
}

// TestJavaScriptDurabilityFixturePartialStartUnwinds proves a partial shared
// fixture start cannot strand process, listener, session, stream, route,
// root, worktree, or mutable state and preserves the original error.
func TestJavaScriptDurabilityFixturePartialStartUnwinds(t *testing.T) {
	ledger := &durabilityResourceLedger{}
	for _, kind := range durabilityResourceKinds {
		ledger.acquire(kind)
	}
	original := errors.New("injected durability fixture start failure")

	if got := unwindDurabilityFixtureStart(ledger, original); got != original {
		t.Fatalf("partial-start error = %v, want original error %v", got, original)
	}
	counts := ledger.snapshot()
	if counts != (durabilityResourceCounts{}) {
		t.Fatalf("partial-start resource counts = %#v, want all zero", counts)
	}
	t.Logf("durability partial-start lifecycle report: process=%d port=%d listener=%d session=%d stream=%d route=%d root=%d worktree=%d mutable-state=%d original_error=%q", counts.process, counts.port, counts.listener, counts.session, counts.stream, counts.route, counts.root, counts.worktree, counts.mutable, original)
}

// TestJavaScriptDurabilityBehavior runs the eligible snapshot row through one
// package-owned process. The two interruption/resume rows remain top-level
// process-sensitive tests because their blocked cancellation and recovery
// lifecycles are explicitly isolated in resume_test.go.
func TestJavaScriptDurabilityBehavior(t *testing.T) {
	fixture := newDurabilityFixture(t)
	t.Run("snapshot/default-persistence", func(t *testing.T) {
		runJavaScriptDurabilityPersistsSnapshotsByDefault(t, fixture)
	})
}

type durabilityFixture struct {
	process     support.ApplicationProcess
	api         *support.ProcessAPIServer
	apiStarter  *durabilityAPIServerStarter
	provider    *javascriptDurabilityResumeBlockingCommandRunner
	resources   *durabilityResourceLedger
	baseURL     string
	projectRoot string
	homeDir     string
	command     *support.ProcessCommand

	rootBuilds    atomic.Int32
	processStarts atomic.Int32
	processStops  atomic.Int32

	stopOnce  sync.Once
	sessionMu sync.Mutex
	sessions  map[string]durabilitySession
	closed    map[string]struct{}
}

type durabilitySession struct {
	requestID string
	rootDir   string
}

type durabilityAPIServerStarter struct {
	api       *support.ProcessAPIServer
	resources *durabilityResourceLedger
	starts    atomic.Int32
	stopped   chan struct{}
	stopOnce  sync.Once
}

func (starter *durabilityAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	starter.resources.acquire(durabilityResourcePort)
	starter.resources.acquire(durabilityResourceListener)
	defer starter.resources.release(durabilityResourcePort)
	defer starter.resources.release(durabilityResourceListener)
	err := starter.api.Start(ctx, request)
	starter.stopOnce.Do(func() { close(starter.stopped) })
	return err
}

func newDurabilityFixture(t *testing.T) *durabilityFixture {
	t.Helper()

	projectRoot := setupJavaScriptDurabilityResumeWorkflowFixture(t, durabilityWorkflowName)
	provider := newJavaScriptDurabilityResumeBlockingCommandRunner(durabilityWorkflowName)
	homeDir := t.TempDir()
	resources := &durabilityResourceLedger{}
	api := support.NewProcessAPIServer()
	apiStarter := &durabilityAPIServerStarter{
		api:       api,
		resources: resources,
		stopped:   make(chan struct{}),
	}
	fixture := &durabilityFixture{
		api:         api,
		apiStarter:  apiStarter,
		provider:    provider,
		resources:   resources,
		projectRoot: projectRoot,
		homeDir:     homeDir,
		sessions:    make(map[string]durabilitySession),
		closed:      make(map[string]struct{}),
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.apiStarter.Start,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(durability): %v", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)
	fixture.resources.acquire(durabilityResourceRoot)
	fixture.resources.acquire(durabilityResourceProcess)
	fixture.resources.acquire(durabilityResourceRoute)

	// Register the report before process cleanup. LIFO cleanup waits for the
	// hosted command, closes the root, releases static cells, and reports the
	// final package-owned census.
	t.Cleanup(func() { fixture.assertCleanup(t) })
	t.Cleanup(func() { fixture.processStops.Add(1) })
	t.Cleanup(func() {
		fixture.resources.release(durabilityResourceRoute)
		fixture.resources.release(durabilityResourceProcess)
		fixture.resources.release(durabilityResourceRoot)
	})
	support.CleanupProcess(t, process)

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
	if got := fixture.apiStarter.starts.Load(); got != 1 {
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

	// The public interrupted projection is published before the canceled
	// provider command necessarily returns. Stop the shared hosted invocation
	// before inspecting project-local persistence so this assertion observes
	// the completed process cleanup boundary.
	fixture.stopHostedProcess(t)
	assertJavaScriptDurableSessionPersistence(t, fixture.projectRoot, sessionID)
}

func (fixture *durabilityFixture) stopHostedProcess(t testing.TB) {
	t.Helper()
	fixture.stopOnce.Do(func() {
		fixture.command.Stop(t)
	})
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
	if _, exists := fixture.sessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("durability Factory Session ID %q was reused", sessionID)
	}
	for existingID, session := range fixture.sessions {
		if filepath.Clean(session.rootDir) == filepath.Clean(rootDir) {
			fixture.sessionMu.Unlock()
			t.Fatalf("durability Factory Session roots reused by %q and %q: %s", existingID, sessionID, rootDir)
		}
		if session.requestID == requestID {
			fixture.sessionMu.Unlock()
			t.Fatalf("durability request ID %q was reused", requestID)
		}
	}
	fixture.sessions[sessionID] = durabilitySession{requestID: requestID, rootDir: rootDir}
	fixture.resources.acquire(durabilityResourceSession)
	fixture.resources.acquire(durabilityResourceWorktree)
	fixture.resources.acquire(durabilityResourceMutable)
	fixture.sessionMu.Unlock()

	t.Cleanup(func() { fixture.markSessionClosed(sessionID) })
}

func (fixture *durabilityFixture) markSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	if _, alreadyClosed := fixture.closed[sessionID]; alreadyClosed {
		fixture.sessionMu.Unlock()
		return
	}
	fixture.closed[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	fixture.resources.release(durabilityResourceMutable)
	fixture.resources.release(durabilityResourceWorktree)
	fixture.resources.release(durabilityResourceSession)
}

func (fixture *durabilityFixture) assertCleanup(t testing.TB) {
	t.Helper()
	if got := fixture.rootBuilds.Load(); got != 1 {
		t.Errorf("durability shared root builds = %d, want one", got)
	}
	if got := fixture.processStarts.Load(); got != 1 {
		t.Errorf("durability shared process starts = %d, want one", got)
	}
	if got := fixture.processStops.Load(); got != 1 {
		t.Errorf("durability shared process stops = %d, want one", got)
	}
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Errorf("durability shared API starts = %d, want one", got)
	}
	select {
	case <-fixture.apiStarter.stopped:
	case <-time.After(durabilityFixtureTimeout):
		t.Errorf("durability shared API listener did not stop")
	}
	if got := fixture.provider.callCount(); got != 2 {
		t.Errorf("durability shared provider calls = %d, want two interrupted-row calls", got)
	}

	counts := fixture.resources.snapshot()
	if counts != (durabilityResourceCounts{}) {
		t.Errorf("durability shared active resource counts = %#v, want all zero", counts)
	}
	fixture.sessionMu.Lock()
	tracked := len(fixture.sessions)
	closed := len(fixture.closed)
	fixture.sessionMu.Unlock()
	if tracked != durabilityBehaviorCount {
		t.Errorf("durability shared tracked top-level sessions = %d, want %d", tracked, durabilityBehaviorCount)
	}
	if tracked != closed {
		t.Errorf("durability shared sessions closed = %d/%d, want all tracked sessions closed", closed, tracked)
	}

	if strings.TrimSpace(fixture.baseURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Errorf("durability shared API listener remained available after cleanup: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
		}
	}
	t.Logf("durability lifecycle report: shared_root_builds=%d shared_process_starts=%d shared_process_stops=%d shared_api_server_starts=%d tracked_sessions=%d closed_sessions=%d provider_calls=%d isolated_process_pairs=2 active={process:%d port:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d}", fixture.rootBuilds.Load(), fixture.processStarts.Load(), fixture.processStops.Load(), fixture.apiStarter.starts.Load(), tracked, closed, fixture.provider.callCount(), counts.process, counts.port, counts.listener, counts.session, counts.stream, counts.route, counts.root, counts.worktree, counts.mutable)
}
