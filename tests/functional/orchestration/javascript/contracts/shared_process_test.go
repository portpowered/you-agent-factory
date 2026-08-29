package contracts

import (
	"bytes"
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
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	contractFixtureTimeout = 15 * time.Second
	contractBehaviorCount  = 8
	contractHostWorkflow   = "return \"contract-host\";"
)

type contractResourceKind string

const (
	contractResourceProcess  contractResourceKind = "process"
	contractResourcePort     contractResourceKind = "port"
	contractResourceListener contractResourceKind = "listener"
	contractResourceSession  contractResourceKind = "session"
	contractResourceStream   contractResourceKind = "stream"
	contractResourceRoute    contractResourceKind = "route"
	contractResourceRoot     contractResourceKind = "root"
	contractResourceWorktree contractResourceKind = "worktree"
	contractResourceMutable  contractResourceKind = "mutable-state"
)

var contractResourceKinds = []contractResourceKind{
	contractResourceProcess,
	contractResourcePort,
	contractResourceListener,
	contractResourceSession,
	contractResourceStream,
	contractResourceRoute,
	contractResourceRoot,
	contractResourceWorktree,
	contractResourceMutable,
}

// contractResourceLedger is scoped to the reusable fixture. It records the
// package-owned lifecycle cells rather than attempting to census unrelated
// process-wide resources.
type contractResourceLedger struct {
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

type contractResourceCounts struct {
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

func (ledger *contractResourceLedger) counter(kind contractResourceKind) *atomic.Int32 {
	switch kind {
	case contractResourceProcess:
		return &ledger.process
	case contractResourcePort:
		return &ledger.port
	case contractResourceListener:
		return &ledger.listener
	case contractResourceSession:
		return &ledger.session
	case contractResourceStream:
		return &ledger.stream
	case contractResourceRoute:
		return &ledger.route
	case contractResourceRoot:
		return &ledger.root
	case contractResourceWorktree:
		return &ledger.worktree
	case contractResourceMutable:
		return &ledger.mutable
	default:
		panic("unknown contract resource kind: " + string(kind))
	}
}

func (ledger *contractResourceLedger) acquire(kind contractResourceKind) {
	ledger.counter(kind).Add(1)
}

func (ledger *contractResourceLedger) release(kind contractResourceKind) {
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

func (ledger *contractResourceLedger) snapshot() contractResourceCounts {
	return contractResourceCounts{
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

// unwindContractFixtureStart returns the injected cause unchanged after
// draining every resource cell acquired before a fixture start failure.
func unwindContractFixtureStart(ledger *contractResourceLedger, cause error) error {
	for _, kind := range contractResourceKinds {
		for ledger.counter(kind).Load() > 0 {
			ledger.release(kind)
		}
	}
	return cause
}

// TestJavaScriptContractFixturePartialStartUnwinds proves an injected fixture
// start error cannot strand package-owned process, listener, session, stream,
// routing, root, worktree, or mutable-state resources.
func TestJavaScriptContractFixturePartialStartUnwinds(t *testing.T) {
	ledger := &contractResourceLedger{}
	for _, kind := range contractResourceKinds {
		ledger.acquire(kind)
	}
	original := errors.New("injected contract fixture start failure")

	if got := unwindContractFixtureStart(ledger, original); got != original {
		t.Fatalf("partial-start error = %v, want original error %v", got, original)
	}
	counts := ledger.snapshot()
	if counts != (contractResourceCounts{}) {
		t.Fatalf("partial-start resource counts = %#v, want all zero", counts)
	}
	t.Logf("contract partial-start lifecycle report: process=%d port=%d listener=%d session=%d stream=%d route=%d root=%d worktree=%d mutable-state=%d original_error=%q", counts.process, counts.port, counts.listener, counts.session, counts.stream, counts.route, counts.root, counts.worktree, counts.mutable, original)
}

// TestJavaScriptContractBehavior runs CASE-15 through CASE-22 sequentially
// through one process-owned host. Each row still gets a fresh explicit
// Factory Session and source root; the single provider edge is immutable after
// process construction and only supplies the two live child responses.
func TestJavaScriptContractBehavior(t *testing.T) {
	fixture := newContractFixture(t)

	cases := []struct {
		name string
		run  func(*testing.T, *contractFixture)
	}{
		{"input/typed", runJavaScriptInvocationReceivesStringNumberBooleanObjectAndArrayInputs},
		{"input/missing-required", runJavaScriptMissingRequiredInputFailsBeforeChildDispatch},
		{"output/return-value", runJavaScriptReturnValueMapsToPrimaryInvocationResult},
		{"output/structured-artifact", runJavaScriptStructuredArtifactsMapToPublicResult},
		{"output/unsupported-return", runJavaScriptUnsupportedReturnValueFailsWithoutPrivateVMDetails},
		{"response/child-progress", runJavaScriptChildProgressPublishesCanonicalResponseEvents},
		{"response/terminal-order", runJavaScriptTerminalResultFollowsFinalResponseEvent},
		{"events/phase-checkpoint", runJavaScriptPhaseCheckpointLifecyclePublishesCanonicalFactoryEvents},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, fixture)
		})
	}
}

type contractFixture struct {
	process    support.ApplicationProcess
	api        *support.ProcessAPIServer
	apiStarter *contractAPIServerStarter
	resources  *contractResourceLedger
	provider   *testutil.ProviderCommandRunner
	baseURL    string
	hostDir    string
	homeDir    string

	rootBuilds    atomic.Int32
	processStarts atomic.Int32
	processStops  atomic.Int32
	requestNumber atomic.Uint64

	sessionMu sync.Mutex
	sessions  map[string]contractSession
	closed    map[string]struct{}
}

type contractSession struct {
	mode      string
	requestID string
	rootDir   string
}

type contractAPIServerStarter struct {
	api       *support.ProcessAPIServer
	resources *contractResourceLedger
	starts    atomic.Int32
	stopped   chan struct{}
	stopOnce  sync.Once
}

func (starter *contractAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	starter.resources.acquire(contractResourcePort)
	starter.resources.acquire(contractResourceListener)
	defer starter.resources.release(contractResourcePort)
	defer starter.resources.release(contractResourceListener)
	err := starter.api.Start(ctx, request)
	starter.stopOnce.Do(func() { close(starter.stopped) })
	return err
}

func newContractFixture(t *testing.T) *contractFixture {
	t.Helper()

	homeDir := t.TempDir()
	hostDir := scaffoldContractHostFactory(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{
			Stdout: codexChildProgressStream(codexChildSessionID, "Child summary COMPLETE"),
		},
		platformprocess.CommandResult{
			Stdout: codexChildProgressStream(codexChildSessionID, "Child summary COMPLETE"),
		},
	)
	resources := &contractResourceLedger{}
	api := support.NewProcessAPIServer()
	apiStarter := &contractAPIServerStarter{
		api:       api,
		resources: resources,
		stopped:   make(chan struct{}),
	}
	fixture := &contractFixture{
		api:        api,
		apiStarter: apiStarter,
		resources:  resources,
		provider:   runner,
		hostDir:    hostDir,
		homeDir:    homeDir,
		sessions:   make(map[string]contractSession),
		closed:     make(map[string]struct{}),
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.apiStarter.Start,
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess(contracts): %v", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)
	fixture.resources.acquire(contractResourceRoot)
	fixture.resources.acquire(contractResourceProcess)
	fixture.resources.acquire(contractResourceRoute)

	// Register the lifecycle census before process cleanup so the final report
	// runs after the command, root, route, listener, and session cleanups.
	t.Cleanup(func() { fixture.assertCleanup(t) })
	t.Cleanup(func() { fixture.processStops.Add(1) })
	t.Cleanup(func() {
		fixture.resources.release(contractResourceRoute)
		fixture.resources.release(contractResourceProcess)
		fixture.resources.release(contractResourceRoot)
	})
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = contractCustomerEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	fixture.processStarts.Add(1)
	support.StartProcessCommand(t, process, inputs.Input)

	baseURL, err := api.WaitForBaseURL(contractFixtureTimeout)
	if err != nil {
		t.Fatalf("wait for contracts API: %v", err)
	}
	fixture.baseURL = baseURL
	support.WaitForStatus(t, baseURL, contractFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Fatalf("contracts API server starts = %d, want one", got)
	}
	return fixture
}

func scaffoldContractHostFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-contract-host",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "contract-host.workflow.js",
			},
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "contract-host.workflow.js"), []byte(contractHostWorkflow), 0o600); err != nil {
		t.Fatalf("write contract host workflow: %v", err)
	}
	return dir
}

func contractCustomerEnvironment(homeDir string) []string {
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

func (fixture *contractFixture) nextRequestID(label string) string {
	return fmt.Sprintf("javascript-contract-%s-%d", label, fixture.requestNumber.Add(1))
}

func (fixture *contractFixture) providerCallCount() int {
	if fixture == nil || fixture.provider == nil {
		return 0
	}
	return fixture.provider.CallCount()
}

func (fixture *contractFixture) startSync(
	t *testing.T,
	request factoryapi.FactorySessionExecutionRequest,
	rootDir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal contract sync request: %v", err)
	}
	response := fixture.doRequest(t, http.MethodPost, "/factory-sessions/sync", payload)
	defer response.Body.Close()
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode contract sync response: %v", err)
	}
	fixture.trackSession(t, started.SessionId, request.RequestId, rootDir, "api")
	return started
}

func (fixture *contractFixture) startAsync(
	t *testing.T,
	request factoryapi.FactorySessionExecutionRequest,
	rootDir string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal contract async request: %v", err)
	}
	response := fixture.doRequest(t, http.MethodPost, "/factory-sessions/async", payload)
	defer response.Body.Close()
	var started factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode contract async response: %v", err)
	}
	fixture.trackSession(t, started.SessionId, request.RequestId, rootDir, "api")
	return started
}

func (fixture *contractFixture) doRequest(
	t *testing.T,
	method, path string,
	payload []byte,
) *http.Response {
	t.Helper()
	endpoint := strings.TrimSuffix(fixture.baseURL, "/") + path
	request, err := http.NewRequestWithContext(t.Context(), method, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build contract request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("contract request %s %s: %v", method, path, err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("contract request %s %s status = %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return response
}

func (fixture *contractFixture) factoryEvents(
	t *testing.T,
	sessionID string,
) []factoryapi.FactoryEvent {
	t.Helper()
	fixture.resources.acquire(contractResourceStream)
	defer fixture.resources.release(contractResourceStream)
	return getFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
}

func (fixture *contractFixture) responseEvents(
	t *testing.T,
	sessionID string,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	fixture.resources.acquire(contractResourceStream)
	defer fixture.resources.release(contractResourceStream)
	return support.GetFactoryResponseEventsAt(t, fixture.baseURL, sessionID)
}

func (fixture *contractFixture) observeChildProgressExecutionOrdering(
	ctx context.Context,
	t *testing.T,
	sessionID string,
) ([]factoryapi.FactoryResponseEvent, factoryapi.FactorySessionResult, []executionObservation) {
	t.Helper()
	fixture.resources.acquire(contractResourceStream)
	defer fixture.resources.release(contractResourceStream)
	return observeChildProgressExecutionOrdering(ctx, t, fixture.baseURL, sessionID)
}

func (fixture *contractFixture) trackSession(
	t *testing.T,
	sessionID, requestID, rootDir, mode string,
) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("contract Factory Session ID is empty")
	}
	if strings.TrimSpace(rootDir) == "" {
		t.Fatal("contract Factory Session root is empty")
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.sessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("contract Factory Session ID %q was reused", sessionID)
	}
	for existingID, session := range fixture.sessions {
		if session.rootDir == rootDir {
			fixture.sessionMu.Unlock()
			t.Fatalf("contract Factory Session roots reused by %q and %q: %s", existingID, sessionID, rootDir)
		}
		if strings.TrimSpace(requestID) != "" && session.requestID == requestID {
			fixture.sessionMu.Unlock()
			t.Fatalf("contract request ID %q was reused", requestID)
		}
	}
	fixture.sessions[sessionID] = contractSession{mode: mode, requestID: requestID, rootDir: rootDir}
	fixture.resources.acquire(contractResourceSession)
	fixture.resources.acquire(contractResourceWorktree)
	fixture.resources.acquire(contractResourceMutable)
	fixture.sessionMu.Unlock()

	t.Cleanup(func() {
		if mode == "api" {
			support.TerminateFactorySessionAt(t, fixture.baseURL, sessionID)
		}
		fixture.markSessionClosed(sessionID)
	})
}

func (fixture *contractFixture) trackLocalSession(
	t *testing.T,
	sessionID, requestID, rootDir string,
) {
	t.Helper()
	fixture.trackSession(t, sessionID, requestID, rootDir, "local")
}

func (fixture *contractFixture) markSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	if _, alreadyClosed := fixture.closed[sessionID]; alreadyClosed {
		fixture.sessionMu.Unlock()
		return
	}
	fixture.closed[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	fixture.resources.release(contractResourceMutable)
	fixture.resources.release(contractResourceWorktree)
	fixture.resources.release(contractResourceSession)
}

func (fixture *contractFixture) assertCleanup(t testing.TB) {
	t.Helper()
	if got := fixture.rootBuilds.Load(); got != 1 {
		t.Errorf("contracts root builds = %d, want one", got)
	}
	if got := fixture.processStarts.Load(); got != 1 {
		t.Errorf("contracts process starts = %d, want one", got)
	}
	if got := fixture.processStops.Load(); got != 1 {
		t.Errorf("contracts process stops = %d, want one", got)
	}
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Errorf("contracts API starts = %d, want one", got)
	}
	select {
	case <-fixture.apiStarter.stopped:
	case <-time.After(contractFixtureTimeout):
		t.Errorf("contracts API listener did not stop")
	}

	counts := fixture.resources.snapshot()
	if counts != (contractResourceCounts{}) {
		t.Errorf("contracts active resource counts = %#v, want all zero", counts)
	}
	fixture.sessionMu.Lock()
	tracked := len(fixture.sessions)
	closed := len(fixture.closed)
	fixture.sessionMu.Unlock()
	if tracked != contractBehaviorCount {
		t.Errorf("contracts tracked top-level sessions = %d, want %d", tracked, contractBehaviorCount)
	}
	if tracked != closed {
		t.Errorf("contracts sessions closed = %d/%d, want all tracked sessions closed", closed, tracked)
	}
	if got := fixture.providerCallCount(); got != 2 {
		t.Errorf("contracts provider calls = %d, want two live child calls", got)
	}

	if strings.TrimSpace(fixture.baseURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Errorf("contracts API listener remained available after cleanup: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
		}
	}
	t.Logf("contracts lifecycle report: root_builds=%d process_starts=%d process_stops=%d api_server_starts=%d tracked_sessions=%d closed_sessions=%d provider_calls=%d active={process:%d port:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d}", fixture.rootBuilds.Load(), fixture.processStarts.Load(), fixture.processStops.Load(), fixture.apiStarter.starts.Load(), tracked, closed, fixture.providerCallCount(), counts.process, counts.port, counts.listener, counts.session, counts.stream, counts.route, counts.root, counts.worktree, counts.mutable)
}
