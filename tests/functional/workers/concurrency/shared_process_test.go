package root_composition_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	concurrencySharedProcessTimeout   = 20 * time.Second
	concurrencyForcedCleanupChildEnv  = "YOU_CONCURRENCY_FORCED_CLEANUP_CHILD"
	concurrencyForcedCleanupReportEnv = "YOU_CONCURRENCY_FORCED_CLEANUP_REPORT"
	concurrencyFailureMessage         = "concurrency controlled authentication failure"
)

type concurrencyRunnerBehavior string

const (
	concurrencyRunnerSuccess       concurrencyRunnerBehavior = "success"
	concurrencyRunnerHold          concurrencyRunnerBehavior = "hold"
	concurrencyRunnerFailureHold   concurrencyRunnerBehavior = "failure-hold"
	concurrencyRunnerTimeoutMarker concurrencyRunnerBehavior = "timeout-marker"
)

// TestConcurrencySharedProcess keeps the retained two-session witness and the
// concurrency matrix on one root-built process. Each scenario owns a distinct
// explicit Factory Session and an immutable factory-directory route; barriers
// live at the controlled command edge so the scheduler remains genuinely
// concurrent and no fixture-wide scenario lock can hide capacity behavior.
func TestConcurrencySharedProcess(t *testing.T) {
	if os.Getenv(concurrencyForcedCleanupChildEnv) == "1" {
		runConcurrencyForcedCleanupChild(t)
		return
	}
	t.Parallel()

	fixture := newConcurrencySharedProcessFixture(t)
	fixture.start(t)

	t.Run("Capacity", func(t *testing.T) {
		t.Parallel()
		t.Run("CC-01", func(t *testing.T) { t.Parallel(); fixture.runCapacityOne(t) })
		t.Run("CC-02", func(t *testing.T) { t.Parallel(); fixture.runCapacityTwo(t) })
		t.Run("CC-06", func(t *testing.T) { t.Parallel(); fixture.runIdempotentRequest(t) })
		t.Run("CC-07", func(t *testing.T) { t.Parallel(); fixture.runDuplicateConflict(t) })
		t.Run("CC-08", func(t *testing.T) { t.Parallel(); fixture.runEmptyRequest(t) })
	})
	t.Run("Concurrent", func(t *testing.T) {
		t.Parallel()
		t.Run("CC-03", func(t *testing.T) { t.Parallel(); fixture.runConcurrentSessionIsolation(t) })
		t.Run("CC-10", func(t *testing.T) { t.Parallel(); fixture.runPartialFailure(t) })
		t.Run("CC-12", func(t *testing.T) { t.Parallel(); fixture.runSessionOrdering(t) })
	})
	t.Run("Cancel", func(t *testing.T) {
		t.Parallel()
		t.Run("CC-04", func(t *testing.T) { t.Parallel(); fixture.runSessionCancellationIsolation(t) })
		t.Run("CC-05", func(t *testing.T) { t.Parallel(); fixture.runWorkerSessionCancellation(t) })
		t.Run("CC-13", func(t *testing.T) { t.Parallel(); fixture.runRecovery(t) })
	})
	t.Run("Timeout", func(t *testing.T) { t.Parallel(); fixture.runTimeoutRecovery(t) })
	t.Run("Cleanup", func(t *testing.T) { t.Parallel(); runConcurrencyForcedCleanupParent(t) })
}

type concurrencySharedProcessFixture struct {
	process    support.ApplicationProcess
	command    *support.ProcessCommand
	api        *support.ProcessAPIServer
	apiClosed  chan struct{}
	apiClose   sync.Once
	baseURL    string
	hostDir    string
	homeDir    string
	router     *concurrencyCommandRouter
	identities *concurrencyIdentityGenerator

	processBuilds   atomic.Int32
	apiStarts       atomic.Int32
	processClosed   atomic.Bool
	processCloseMu  sync.Mutex
	processCloseErr string

	sessionsMu sync.Mutex
	opened     map[string]string
	closed     map[string]struct{}
	ownedDirs  []string
	sessions   map[string]*concurrencySession
}

type concurrencySession struct {
	fixture   *concurrencySharedProcessFixture
	name      string
	dir       string
	marker    string
	runner    *concurrencyScenarioRunner
	id        string
	stream    factoryapi.FactorySessionStreamIdentity
	closeOnce sync.Once
}

func newConcurrencySharedProcessFixture(t *testing.T) *concurrencySharedProcessFixture {
	t.Helper()

	hostDir := scaffoldConcurrencyFactory(t, "concurrency-host", 1, 1)
	support.ClearSeedInputs(t, hostDir)
	support.WriteAgentConfig(t, hostDir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "concurrency-host-model"))
	support.WriteWorkstationConfig(t, hostDir, "process", concurrencyWorkstationConfig(1))

	fixture := &concurrencySharedProcessFixture{
		api:        support.NewProcessAPIServer(),
		apiClosed:  make(chan struct{}),
		hostDir:    hostDir,
		homeDir:    t.TempDir(),
		router:     newConcurrencyCommandRouter(),
		identities: &concurrencyIdentityGenerator{},
		opened:     make(map[string]string),
		closed:     make(map[string]struct{}),
		sessions:   make(map[string]*concurrencySession),
		ownedDirs:  []string{hostDir},
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			fixture.apiStarts.Add(1)
			err := fixture.api.Start(ctx, request)
			fixture.apiClose.Do(func() { close(fixture.apiClosed) })
			return err
		},
		ProviderCommandRunner:                  fixture.router,
		FactorySessionIDGenerator:              fixture.identities.nextSessionID,
		FactorySessionResponseEventIDGenerator: fixture.identities.nextResponseEventID,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	fixture.process = process
	fixture.processBuilds.Add(1)
	t.Cleanup(func() { fixture.close(t) })
	return fixture
}

func (fixture *concurrencySharedProcessFixture) start(t *testing.T) {
	t.Helper()
	if fixture.command != nil {
		t.Fatal("shared concurrency process started more than once")
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", fixture.hostDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+fixture.homeDir, "USERPROFILE="+fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostDir
	fixture.command = support.StartProcessCommand(t, fixture.process, inputs.Input)
	fixture.baseURL = fixture.api.WaitForURL(t)
	defaultSession := support.GetDefaultSession(t, fixture.baseURL)
	if !defaultSession.IsDefault || strings.TrimSpace(defaultSession.Id) == "" {
		t.Fatalf("default Factory Session = %#v, want default identity", defaultSession)
	}
}

func (fixture *concurrencySharedProcessFixture) openCase(
	t *testing.T,
	name string,
	capacity int,
	behavior concurrencyRunnerBehavior,
	marker string,
	failMarker string,
	maxRetries int,
) *concurrencySession {
	t.Helper()
	dir := scaffoldConcurrencyFactory(t, name, capacity, maxRetries)
	support.ClearSeedInputs(t, dir)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "concurrency-"+strings.ToLower(name)))
	support.WriteWorkstationConfig(t, dir, "process", concurrencyWorkstationConfig(maxRetries))
	runner := newConcurrencyScenarioRunner(behavior, marker, failMarker)
	fixture.router.register(t, dir, runner)
	fixture.addOwnedDir(dir)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, dir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("opened Factory Session for %q = %#v, want identity", dir, opened)
	}
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened Factory Session for %q = %q, want explicit session", dir, sessionID)
	}
	publicSession := getConcurrencyFactorySession(t, fixture.baseURL, sessionID)
	if publicSession.Runtime.StreamIdentity == nil {
		t.Fatalf("opened Factory Session for %q runtime stream identity = nil, want public identity", dir)
	}
	streamIdentity := *publicSession.Runtime.StreamIdentity
	for label, value := range map[string]string{
		"backend scope":     streamIdentity.BackendScopeID,
		"logical session":   streamIdentity.LogicalSessionKeyID,
		"factory session":   streamIdentity.FactorySessionID,
		"stream generation": streamIdentity.StreamGenerationID,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("opened Factory Session %q %s stream identity = %#v, want non-empty public identity", dir, label, streamIdentity)
		}
	}
	if streamIdentity.FactorySessionID != sessionID {
		t.Fatalf("opened Factory Session %q stream identity session = %q, want %q", dir, streamIdentity.FactorySessionID, sessionID)
	}
	fixture.sessionsMu.Lock()
	if _, exists := fixture.opened[sessionID]; exists {
		fixture.sessionsMu.Unlock()
		t.Fatalf("Factory Session id %q was reused", sessionID)
	}
	session := &concurrencySession{fixture: fixture, name: name, dir: dir, marker: marker, runner: runner, id: sessionID, stream: streamIdentity}
	fixture.opened[sessionID] = dir
	fixture.sessions[sessionID] = session
	fixture.sessionsMu.Unlock()
	t.Cleanup(func() { session.close(t) })
	return session
}

func (fixture *concurrencySharedProcessFixture) addOwnedDir(dir string) {
	fixture.sessionsMu.Lock()
	defer fixture.sessionsMu.Unlock()
	fixture.ownedDirs = append(fixture.ownedDirs, dir)
}

func (session *concurrencySession) close(t testing.TB) {
	t.Helper()
	session.closeOnce.Do(func() {
		support.CloseFactorySessionAt(t, session.fixture.baseURL, session.id)
		session.fixture.sessionsMu.Lock()
		session.fixture.closed[session.id] = struct{}{}
		session.fixture.sessionsMu.Unlock()
	})
}

func (session *concurrencySession) closeAndAssertGone(t *testing.T) {
	t.Helper()
	session.close(t)
	assertConcurrencySessionDeleted(t, session.fixture.baseURL, session.id)
}
