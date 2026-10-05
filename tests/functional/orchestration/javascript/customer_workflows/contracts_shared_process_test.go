package customer_workflows_test

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

// contractStreamLifecycle observes active public Factory Event SSE handlers at
// the injected HTTP edge. It does not recreate an application resource map.
type contractStreamLifecycle struct {
	active          atomic.Int32
	opened          atomic.Int32
	closed          atomic.Int32
	sessionRequests atomic.Int32
}

// contractProviderCommandRunner keeps provider execution at the immutable
// external-effect edge while exposing its in-flight count for teardown proof.
type contractProviderCommandRunner struct {
	inner  *testutil.ProviderCommandRunner
	mu     sync.Mutex
	active int
}

func newContractProviderCommandRunner(results ...platformprocess.CommandResult) *contractProviderCommandRunner {
	return &contractProviderCommandRunner{inner: testutil.NewProviderCommandRunner(results...)}
}

func (runner *contractProviderCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.active++
	runner.mu.Unlock()
	defer func() {
		runner.mu.Lock()
		runner.active--
		runner.mu.Unlock()
	}()
	return runner.inner.Run(ctx, request)
}

func (runner *contractProviderCommandRunner) CallCount() int {
	return runner.inner.CallCount()
}

func (runner *contractProviderCommandRunner) ActiveCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.active
}

func (lifecycle *contractStreamLifecycle) wrap(next http.Handler) http.Handler {
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

var (
	contractFixtureMu     sync.Mutex
	sharedContractFixture *contractFixture
)

// initializeContractsFixture keeps its process alive until all customer journeys finish.
func initializeContractsFixture(t *testing.T) {
	sharedContractFixture = nil
	t.Cleanup(func() {
		fixture := sharedContractFixture
		if fixture != nil {
			if err := fixture.shutdown(); err != nil {
				t.Errorf("close workflow fixture: %v", err)
			}
		}
	})
}

// The original top-level behavior names remain the selectable witnesses. Each
// wrapper obtains the same lazily created package fixture and owns only its
// scenario's authored root and explicit Factory Session.
func testContractsJavaScriptInvocationReceivesStringNumberBooleanObjectAndArrayInputs(t *testing.T) {
	runJavaScriptInvocationReceivesStringNumberBooleanObjectAndArrayInputs(t, contractFixtureForTest(t))
}

func testContractsJavaScriptMissingRequiredInputFailsBeforeChildDispatch(t *testing.T) {
	runJavaScriptMissingRequiredInputFailsBeforeChildDispatch(t, contractFixtureForTest(t))
}

func testContractsJavaScriptReturnValueMapsToPrimaryInvocationResult(t *testing.T) {
	runJavaScriptReturnValueMapsToPrimaryInvocationResult(t, contractFixtureForTest(t))
}

func testContractsJavaScriptStructuredArtifactsMapToPublicResult(t *testing.T) {
	runJavaScriptStructuredArtifactsMapToPublicResult(t, contractFixtureForTest(t))
}

func testContractsJavaScriptUnsupportedReturnValueFailsWithoutPrivateVMDetails(t *testing.T) {
	runJavaScriptUnsupportedReturnValueFailsWithoutPrivateVMDetails(t, contractFixtureForTest(t))
}

func testContractsJavaScriptChildProgressPublishesCanonicalResponseEvents(t *testing.T) {
	runJavaScriptChildProgressPublishesCanonicalResponseEvents(t, contractFixtureForTest(t))
}

func testContractsJavaScriptTerminalResultFollowsFinalResponseEvent(t *testing.T) {
	runJavaScriptTerminalResultFollowsFinalResponseEvent(t, contractFixtureForTest(t))
}

func testContractsJavaScriptPhaseCheckpointLifecyclePublishesCanonicalFactoryEvents(t *testing.T) {
	runJavaScriptPhaseCheckpointLifecyclePublishesCanonicalFactoryEvents(t, contractFixtureForTest(t))
}

type contractFixture struct {
	process    support.ApplicationProcess
	api        *support.ProcessAPIServer
	apiStarter *contractAPIServerStarter
	provider   *contractProviderCommandRunner
	baseURL    string
	hostDir    string
	homeDir    string

	rootBuilds    atomic.Int32
	processStarts atomic.Int32
	processStops  atomic.Int32
	requestNumber atomic.Uint64

	processCancel context.CancelFunc
	processDone   chan struct{}
	processMu     sync.Mutex
	processErr    error

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
	api      *support.ProcessAPIServer
	streams  *contractStreamLifecycle
	starts   atomic.Int32
	stopped  chan struct{}
	stopOnce sync.Once
}

func (starter *contractAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	request.Handler = starter.streams.wrap(request.Handler)
	err := starter.api.Start(ctx, request)
	starter.stopOnce.Do(func() { close(starter.stopped) })
	return err
}

func contractFixtureForTest(t *testing.T) *contractFixture {
	t.Helper()
	contractFixtureMu.Lock()
	defer contractFixtureMu.Unlock()
	if sharedContractFixture == nil {
		sharedContractFixture = newContractFixture(t)
	}
	return sharedContractFixture
}

func newContractFixture(t *testing.T) *contractFixture {
	t.Helper()

	homeDir, err := os.MkdirTemp("", "you-functional-contract-home-")
	if err != nil {
		t.Fatalf("create contract home: %v", err)
	}
	hostDir, err := os.MkdirTemp("", "you-functional-contract-factory-")
	if err != nil {
		_ = os.RemoveAll(homeDir)
		t.Fatalf("create contract factory: %v", err)
	}
	if err := writeContractHostFactory(t, hostDir); err != nil {
		_ = os.RemoveAll(hostDir)
		_ = os.RemoveAll(homeDir)
		t.Fatalf("write contract host factory: %v", err)
	}
	childResult := platformprocess.CommandResult{
		Stdout: codexChildProgressStream(codexChildSessionID, "Child summary COMPLETE"),
	}
	// Go's -count flag repeats m.Run inside one test process, so TestMain's
	// package fixture serves every repetition. Queue one valid child response
	// for each response-event scenario in the aggregate -count=3 gate.
	runner := newContractProviderCommandRunner(
		childResult,
		childResult,
		childResult,
		childResult,
		childResult,
		childResult,
	)
	api := support.NewProcessAPIServer()
	apiStarter := &contractAPIServerStarter{
		api:     api,
		streams: &contractStreamLifecycle{},
		stopped: make(chan struct{}),
	}
	fixture := &contractFixture{
		api:         api,
		apiStarter:  apiStarter,
		provider:    runner,
		hostDir:     hostDir,
		homeDir:     homeDir,
		processDone: make(chan struct{}),
		sessions:    make(map[string]contractSession, contractBehaviorCount),
		closed:      make(map[string]struct{}, contractBehaviorCount),
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.apiStarter.Start,
		ProviderCommandRunner: runner,
	})
	if err != nil {
		_ = os.RemoveAll(hostDir)
		_ = os.RemoveAll(homeDir)
		t.Fatalf("BuildProcess(contracts): %v", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)

	processContext, cancel := context.WithCancel(context.Background())
	fixture.processCancel = cancel
	inputs := support.FakeInputs(processContext, []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = contractCustomerEnvironment(homeDir)
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

	baseURL, err := api.WaitForBaseURL(contractFixtureTimeout)
	if err != nil {
		cancel()
		<-fixture.processDone
		_ = process.Close(context.Background())
		_ = os.RemoveAll(hostDir)
		_ = os.RemoveAll(homeDir)
		t.Fatalf("wait for contracts API: %v", err)
	}
	fixture.baseURL = baseURL
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Fatalf("contracts API server starts = %d, want one", got)
	}
	return fixture
}

func contractHostFactoryConfig() map[string]any {
	return map[string]any{
		"name": "javascript-contract-host",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "contract-host.workflow.js",
			},
		},
	}
}

func scaffoldContractHostFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, contractHostFactoryConfig())
	if err := writeContractHostWorkflow(dir); err != nil {
		t.Fatalf("write contract host workflow: %v", err)
	}
	return dir
}

func writeContractHostFactory(t *testing.T, dir string) error {
	t.Helper()
	raw, err := json.Marshal(contractHostFactoryConfig())
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), raw, 0o600); err != nil {
		return err
	}
	return writeContractHostWorkflow(dir)
}

func writeContractHostWorkflow(dir string) error {
	return os.WriteFile(filepath.Join(dir, "contract-host.workflow.js"), []byte(contractHostWorkflow), 0o600)
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
	return getFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
}

func (fixture *contractFixture) responseEvents(
	t *testing.T,
	sessionID string,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	return readFactoryResponseEventsUntilClosed(t, fixture.baseURL, sessionID)
}

func (fixture *contractFixture) observeChildProgressExecutionOrdering(
	t *testing.T,
	sessionID string,
) ([]factoryapi.FactoryResponseEvent, factoryapi.FactorySessionResult, []executionObservation) {
	t.Helper()
	return observeChildProgressExecutionOrdering(t, fixture.baseURL, sessionID)
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
}

func (fixture *contractFixture) shutdown() error {
	if fixture.processCancel != nil {
		fixture.processCancel()
	}
	if fixture.processDone != nil {
		<-fixture.processDone
	}

	closeContext, cancel := context.WithTimeout(context.Background(), contractFixtureTimeout)
	defer cancel()
	closeErr := fixture.process.Close(closeContext)

	fixture.processMu.Lock()
	processErr := fixture.processErr
	fixture.processMu.Unlock()
	var shutdownErr error
	if processErr != nil && !errors.Is(processErr, context.Canceled) {
		shutdownErr = fmt.Errorf("contracts Process.Execute shutdown: %w", processErr)
	}
	if closeErr != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close contracts process: %w", closeErr))
	}
	if got := fixture.rootBuilds.Load(); got != 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("contracts root builds = %d, want one", got))
	}
	if got := fixture.processStarts.Load(); got != 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("contracts process starts = %d, want one", got))
	}
	if got := fixture.processStops.Load(); got != 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("contracts process stops = %d, want one", got))
	}
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("contracts API starts = %d, want one", got))
	}
	// Process.Execute has returned only after the injected listener starter has
	// observed cancellation. This channel is the actual transport lifecycle
	// boundary, not a timeout or a manually released resource cell.
	<-fixture.apiStarter.stopped

	fixture.sessionMu.Lock()
	tracked := len(fixture.sessions)
	closed := len(fixture.closed)
	fixture.sessionMu.Unlock()
	if tracked != closed {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("contracts sessions closed = %d/%d", closed, tracked))
	}

	listenerClosed := fixture.apiStarter.starts.Load() == 0
	if fixture.apiStarter.starts.Load() > 0 {
		listenerClosed = true
	}
	if strings.TrimSpace(fixture.baseURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			listenerClosed = false
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("contracts API listener remained available after shutdown: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body))))
		}
	}
	if err := os.RemoveAll(fixture.hostDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove contracts factory: %w", err))
	}
	if err := os.RemoveAll(fixture.homeDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove contracts home: %w", err))
	}
	streamActive := fixture.apiStarter.streams.active.Load()
	if streamActive != 0 || fixture.apiStarter.streams.opened.Load() != fixture.apiStarter.streams.closed.Load() {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("contracts SSE streams active=%d opened=%d closed=%d", streamActive, fixture.apiStarter.streams.opened.Load(), fixture.apiStarter.streams.closed.Load()))
	}
	processClosed := fixture.processStarts.Load() == fixture.processStops.Load() && closeErr == nil
	processActive := contractsBoolToInt(!processClosed)
	portActive := contractsBoolToInt(!listenerClosed)
	sessionActive := contractMaxInt(tracked-closed, 0)
	streamActiveCount := int(streamActive)
	routeActive := fixture.provider.ActiveCount()
	rootActive := contractMaxInt(int(fixture.rootBuilds.Load())-contractsBoolToInt(processClosed), 0)
	worktreeActive := fixture.activeContractRoots()
	mutableStateActive := sessionActive + streamActiveCount + routeActive + rootActive + worktreeActive
	if processActive != 0 || portActive != 0 || sessionActive != 0 || streamActiveCount != 0 || routeActive != 0 || rootActive != 0 || worktreeActive != 0 || mutableStateActive != 0 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("contracts active resources process=%d port=%d listener=%d session=%d stream=%d route=%d root=%d worktree=%d mutable-state=%d", processActive, portActive, portActive, sessionActive, streamActiveCount, routeActive, rootActive, worktreeActive, mutableStateActive))
	}
	fmt.Fprintf(os.Stderr, "contracts lifecycle report: root_builds=%d process_starts=%d process_stops=%d api_server_starts=%d tracked_sessions=%d closed_sessions=%d provider_calls=%d active={process:%d port:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d}\n", fixture.rootBuilds.Load(), fixture.processStarts.Load(), fixture.processStops.Load(), fixture.apiStarter.starts.Load(), tracked, closed, fixture.providerCallCount(), processActive, portActive, portActive, sessionActive, streamActiveCount, routeActive, rootActive, worktreeActive, mutableStateActive)
	return shutdownErr
}

func (fixture *contractFixture) activeContractRoots() int {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	active := 0
	seen := make(map[string]struct{}, len(fixture.sessions))
	for _, session := range fixture.sessions {
		rootDir := filepath.Clean(session.rootDir)
		if rootDir == "." {
			continue
		}
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

func contractsBoolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func contractMaxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func contractsRemoveAndObservePath(path string) (int, error) {
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
