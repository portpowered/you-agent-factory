package policy

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

	"github.com/portpowered/infinite-you/internal/testutil"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	policyFixtureTimeout       = 15 * time.Second
	policyBehaviorSessionCount = 3
	policyHostWorkflow         = `return "javascript-policy-host";`
)

type policyResourceKind string

const (
	policyResourceProcess  policyResourceKind = "process"
	policyResourcePort     policyResourceKind = "port"
	policyResourceListener policyResourceKind = "listener"
	policyResourceSession  policyResourceKind = "session"
	policyResourceStream   policyResourceKind = "stream"
	policyResourceRoute    policyResourceKind = "route"
	policyResourceRoot     policyResourceKind = "root"
	policyResourceWorktree policyResourceKind = "worktree"
	policyResourceMutable  policyResourceKind = "mutable-state"
)

var policyResourceKinds = []policyResourceKind{
	policyResourceProcess,
	policyResourcePort,
	policyResourceListener,
	policyResourceSession,
	policyResourceStream,
	policyResourceRoute,
	policyResourceRoot,
	policyResourceWorktree,
	policyResourceMutable,
}

// policyResourceLedger is scoped to the package fixture. It records the
// resources acquired by this matrix so cleanup evidence does not depend on a
// census of unrelated test-process state.
type policyResourceLedger struct {
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

type policyResourceCounts struct {
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

func (ledger *policyResourceLedger) counter(kind policyResourceKind) *atomic.Int32 {
	switch kind {
	case policyResourceProcess:
		return &ledger.process
	case policyResourcePort:
		return &ledger.port
	case policyResourceListener:
		return &ledger.listener
	case policyResourceSession:
		return &ledger.session
	case policyResourceStream:
		return &ledger.stream
	case policyResourceRoute:
		return &ledger.route
	case policyResourceRoot:
		return &ledger.root
	case policyResourceWorktree:
		return &ledger.worktree
	case policyResourceMutable:
		return &ledger.mutable
	default:
		panic("unknown policy resource kind: " + string(kind))
	}
}

func (ledger *policyResourceLedger) acquire(kind policyResourceKind) {
	ledger.counter(kind).Add(1)
}

func (ledger *policyResourceLedger) release(kind policyResourceKind) {
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

func (ledger *policyResourceLedger) snapshot() policyResourceCounts {
	return policyResourceCounts{
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

// unwindPolicyFixtureStart returns the original startup error after draining
// every resource cell acquired before a fixture-start failure.
func unwindPolicyFixtureStart(ledger *policyResourceLedger, cause error) error {
	for _, kind := range policyResourceKinds {
		for ledger.counter(kind).Load() > 0 {
			ledger.release(kind)
		}
	}
	return cause
}

// TestJavaScriptPolicyFixturePartialStartUnwinds proves the injected partial
// startup path leaves no process, session, route, root, or mutable fixture
// resource behind and preserves the original error.
func TestJavaScriptPolicyFixturePartialStartUnwinds(t *testing.T) {
	ledger := &policyResourceLedger{}
	for _, kind := range policyResourceKinds {
		ledger.acquire(kind)
	}
	original := errors.New("injected policy fixture start failure")

	if got := unwindPolicyFixtureStart(ledger, original); got != original {
		t.Fatalf("partial-start error = %v, want original error %v", got, original)
	}
	counts := ledger.snapshot()
	if counts != (policyResourceCounts{}) {
		t.Fatalf("partial-start resource counts = %#v, want all zero", counts)
	}
	t.Logf("policy partial-start lifecycle report: process=%d port=%d listener=%d session=%d stream=%d route=%d root=%d worktree=%d mutable-state=%d original_error=%q", counts.process, counts.port, counts.listener, counts.session, counts.stream, counts.route, counts.root, counts.worktree, counts.mutable, original)
}

// TestJavaScriptPolicyBehavior runs both policy rows through one package-owned
// process. The repeated denial row intentionally opens two fresh authored
// roots and sessions so diagnostic stability is proven without cross-session
// source or runtime state.
func TestJavaScriptPolicyBehavior(t *testing.T) {
	fixture := newPolicyFixture(t)
	cases := []struct {
		name string
		run  func(*testing.T, *policyFixture)
	}{
		{"denial/stable-diagnostic", runJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic},
		{"denial/no-external-dispatch", runJavaScriptPolicyFailureDoesNotDispatchExternalWork},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, fixture)
		})
	}
	fixture.startHostedProcess(t)
}

type policyFixture struct {
	owner          testing.TB
	process        *initializerapplication.Process
	api            *support.ProcessAPIServer
	apiStarter     *policyAPIServerStarter
	providerRunner *support.RecordingCommandRunner
	workerProvider *testutil.MockProvider
	resources      *policyResourceLedger
	baseURL        string
	hostDir        string
	homeDir        string

	rootBuilds    atomic.Int32
	processStarts atomic.Int32
	processStops  atomic.Int32

	sessionMu sync.Mutex
	sessions  map[string]policySession
	closed    map[string]struct{}
}

type policySession struct {
	requestID string
	rootDir   string
	homeDir   string
}

type policyAPIServerStarter struct {
	api       *support.ProcessAPIServer
	resources *policyResourceLedger
	starts    atomic.Int32
	stopped   chan struct{}
	stopOnce  sync.Once
}

func (starter *policyAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	starter.resources.acquire(policyResourcePort)
	starter.resources.acquire(policyResourceListener)
	defer starter.resources.release(policyResourcePort)
	defer starter.resources.release(policyResourceListener)
	err := starter.api.Start(ctx, request)
	starter.stopOnce.Do(func() { close(starter.stopped) })
	return err
}

func newPolicyFixture(t *testing.T) *policyFixture {
	t.Helper()

	homeDir := t.TempDir()
	hostDir := scaffoldPolicyHostFactory(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	workerProvider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: `{"text":"should not run"}`},
	)
	resources := &policyResourceLedger{}
	api := support.NewProcessAPIServer()
	apiStarter := &policyAPIServerStarter{
		api:       api,
		resources: resources,
		stopped:   make(chan struct{}),
	}
	fixture := &policyFixture{
		owner:          t,
		api:            api,
		apiStarter:     apiStarter,
		providerRunner: runner,
		workerProvider: workerProvider,
		resources:      resources,
		hostDir:        hostDir,
		homeDir:        homeDir,
		sessions:       make(map[string]policySession),
		closed:         make(map[string]struct{}),
	}

	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.apiStarter.Start,
		BrowserOpener:         func(context.Context, string) error { return nil },
		ProviderCommandRunner: runner,
		ProviderOverride:      workerProvider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(policy): %v", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)
	fixture.resources.acquire(policyResourceRoot)
	fixture.resources.acquire(policyResourceProcess)
	fixture.resources.acquire(policyResourceRoute)

	// Register the final report before process cleanup. LIFO cleanup then stops
	// the hosted command when the matrix has started it, releases static
	// resources, and emits the census.
	t.Cleanup(func() { fixture.assertCleanup(t) })
	t.Cleanup(func() { fixture.processStops.Add(1) })
	t.Cleanup(func() {
		fixture.resources.release(policyResourceRoute)
		fixture.resources.release(policyResourceProcess)
		fixture.resources.release(policyResourceRoot)
	})
	support.CleanupProcess(t, process)
	return fixture
}

func (fixture *policyFixture) startHostedProcess(t *testing.T) {
	t.Helper()
	// Local one-shot invocations own the compatibility runtime until Execute
	// returns. Start the long-lived host after those rows so it can provide the
	// package's single process/listener lifecycle witness without competing for
	// that runtime binding.
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", fixture.hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = policyCustomerEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostDir
	fixture.processStarts.Add(1)
	support.StartProcessCommand(fixture.owner, fixture.process, inputs.Input)

	baseURL, err := fixture.api.WaitForBaseURL(policyFixtureTimeout)
	if err != nil {
		t.Fatalf("wait for policy API: %v", err)
	}
	fixture.baseURL = baseURL
	support.WaitForStatus(t, baseURL, policyFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Fatalf("policy API server starts = %d, want one", got)
	}
}

func scaffoldPolicyHostFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-policy-host",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "policy-host.workflow.js",
			},
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "policy-host.workflow.js"), []byte(policyHostWorkflow), 0o600); err != nil {
		t.Fatalf("write policy host workflow: %v", err)
	}
	return dir
}

func policyCustomerEnvironment(homeDir string) []string {
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

func (fixture *policyFixture) trackInvocationSession(
	t testing.TB,
	result factoryapi.InvocationResponse,
	rootDir string,
	homeDir string,
) {
	t.Helper()
	if result.SessionId == nil || strings.TrimSpace(*result.SessionId) == "" {
		t.Fatalf("policy invocation result = %#v, want explicit Factory Session ID", result)
	}
	fixture.trackSession(t, *result.SessionId, result.RequestId, rootDir, homeDir)
}

func (fixture *policyFixture) trackSession(
	t testing.TB,
	sessionID string,
	requestID string,
	rootDir string,
	homeDir string,
) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("policy Factory Session ID is empty")
	}
	if strings.TrimSpace(rootDir) == "" {
		t.Fatal("policy Factory Session root is empty")
	}

	fixture.sessionMu.Lock()
	if _, exists := fixture.sessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("policy Factory Session ID %q was reused", sessionID)
	}
	for existingID, session := range fixture.sessions {
		if filepath.Clean(session.rootDir) == filepath.Clean(rootDir) {
			fixture.sessionMu.Unlock()
			t.Fatalf("policy Factory Session roots reused by %q and %q: %s", existingID, sessionID, rootDir)
		}
		if strings.TrimSpace(requestID) != "" && session.requestID == requestID {
			fixture.sessionMu.Unlock()
			t.Fatalf("policy request ID %q was reused", requestID)
		}
	}
	fixture.sessions[sessionID] = policySession{
		requestID: requestID,
		rootDir:   rootDir,
		homeDir:   homeDir,
	}
	fixture.resources.acquire(policyResourceSession)
	fixture.resources.acquire(policyResourceWorktree)
	fixture.resources.acquire(policyResourceMutable)
	fixture.sessionMu.Unlock()

	t.Cleanup(func() {
		fixture.closeSession(t, sessionID)
		fixture.markSessionClosed(sessionID)
	})
}

func (fixture *policyFixture) closeSession(t testing.TB, sessionID string) {
	t.Helper()

	fixture.sessionMu.Lock()
	session, ok := fixture.sessions[sessionID]
	fixture.sessionMu.Unlock()
	if !ok {
		t.Errorf("policy Factory Session %q was not tracked during cleanup", sessionID)
		return
	}

	// Local one-shot invocation sessions release their session service when
	// Process.Execute returns. The control probe therefore accepts the public
	// not-found terminal observation while still surfacing other cleanup errors.
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "session", "terminate", sessionID,
	})
	inputs.Input.Env = policyCustomerEnvironment(session.homeDir)
	inputs.Input.WorkingDirectory = session.rootDir
	err := fixture.process.Execute(inputs.Input)
	missing := fmt.Sprintf(`factory session %q not found`, sessionID)
	terminal := string(factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession)
	if err != nil && !strings.Contains(err.Error(), missing) && !strings.Contains(inputs.Stdout(), terminal) {
		t.Errorf(
			"Process.Execute(session terminate %q) error = %v\nstdout:\n%s\nstderr:\n%s",
			sessionID,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if err == nil {
		var response factoryapi.FactorySessionLifecycleControlResponse
		if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); decodeErr != nil {
			t.Errorf("decode local session terminate %q: %v\nstdout:\n%s", sessionID, decodeErr, inputs.Stdout())
		} else if response.SessionId != sessionID {
			t.Errorf("local session terminate id = %q, want %q", response.SessionId, sessionID)
		}
	}
}

func (fixture *policyFixture) markSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	if _, alreadyClosed := fixture.closed[sessionID]; alreadyClosed {
		fixture.sessionMu.Unlock()
		return
	}
	fixture.closed[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	fixture.resources.release(policyResourceMutable)
	fixture.resources.release(policyResourceWorktree)
	fixture.resources.release(policyResourceSession)
}

func (fixture *policyFixture) assertCleanup(t testing.TB) {
	t.Helper()
	if got := fixture.rootBuilds.Load(); got != 1 {
		t.Errorf("policy root builds = %d, want one", got)
	}
	if got := fixture.processStarts.Load(); got != 1 {
		t.Errorf("policy process starts = %d, want one", got)
	}
	if got := fixture.processStops.Load(); got != 1 {
		t.Errorf("policy process stops = %d, want one", got)
	}
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Errorf("policy API starts = %d, want one", got)
	}
	select {
	case <-fixture.apiStarter.stopped:
	case <-time.After(policyFixtureTimeout):
		t.Errorf("policy API listener did not stop")
	}

	if got := fixture.providerRunner.CallCount(); got != 0 {
		t.Errorf("policy provider command calls = %d, want zero", got)
	}
	if got := fixture.workerProvider.CallCount(); got != 0 {
		t.Errorf("policy worker provider calls = %d, want zero", got)
	}
	counts := fixture.resources.snapshot()
	if counts != (policyResourceCounts{}) {
		t.Errorf("policy active resource counts = %#v, want all zero", counts)
	}
	fixture.sessionMu.Lock()
	tracked := len(fixture.sessions)
	closed := len(fixture.closed)
	fixture.sessionMu.Unlock()
	if tracked != policyBehaviorSessionCount {
		t.Errorf("policy tracked top-level sessions = %d, want %d", tracked, policyBehaviorSessionCount)
	}
	if tracked != closed {
		t.Errorf("policy sessions closed = %d/%d, want all tracked sessions closed", closed, tracked)
	}

	if strings.TrimSpace(fixture.baseURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Errorf("policy API listener remained available after cleanup: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
		}
	}
	t.Logf("policy lifecycle report: root_builds=%d process_starts=%d process_stops=%d api_server_starts=%d tracked_sessions=%d closed_sessions=%d provider_calls=%d worker_calls=%d active={process:%d port:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d}", fixture.rootBuilds.Load(), fixture.processStarts.Load(), fixture.processStops.Load(), fixture.apiStarter.starts.Load(), tracked, closed, fixture.providerRunner.CallCount(), fixture.workerProvider.CallCount(), counts.process, counts.port, counts.listener, counts.session, counts.stream, counts.route, counts.root, counts.worktree, counts.mutable)
}
