package composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

	runner := newCompositionCommandRunner()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      failingStarter,
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess(composition partial start): %v", err)
	}
	defer func() {
		closeContext, cancel := context.WithTimeout(context.Background(), compositionFixtureTimeout)
		defer cancel()
		if closeErr := process.Close(closeContext); closeErr != nil {
			t.Errorf("close partial-start process: %v", closeErr)
		}
	}()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = compositionCustomerEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	got := process.Execute(inputs.Input)
	if !errors.Is(got, original) && (got == nil || !strings.Contains(got.Error(), original.Error())) {
		t.Fatalf("partial-start error = %v, want original error %q", got, original)
	}
	if got := starterCalls.Load(); got != 1 {
		t.Fatalf("partial-start API starter calls = %d, want one", got)
	}
	if got := listenerOpen.Load(); got != 0 {
		t.Fatalf("partial-start listeners still open = %d, want zero", got)
	}
	if got := runner.callCount(); got != 0 {
		t.Fatalf("partial-start provider command calls = %d, want zero", got)
	}

	if rawURL, ok := listenerURL.Load().(string); ok && strings.TrimSpace(rawURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, probeErr := client.Get(rawURL + "/status")
		if probeErr == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("partial-start listener remained available: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
		}
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
	requestNumber atomic.Uint64

	processCancel context.CancelFunc
	processDone   chan struct{}
	processMu     sync.Mutex
	processErr    error

	sessionMu sync.Mutex
	sessions  map[string]struct{}
	closed    map[string]struct{}
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
		sessions:    make(map[string]struct{}, compositionBehaviorCount),
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
	support.WaitForStatus(t, baseURL, compositionFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
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
	return fixture.api.Start(ctx, request)
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
	fixture.trackSession(t, started.SessionId)
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
	fixture.trackSession(t, sessionID)
}

func (fixture *compositionFixture) trackSession(t *testing.T, sessionID string) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("composition Factory Session ID is empty")
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.sessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("composition Factory Session ID %q was reused", sessionID)
	}
	fixture.sessions[sessionID] = struct{}{}
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
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("composition API listener remained available after shutdown: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body))))
		}
	}
	if err := os.RemoveAll(fixture.hostDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove composition factory: %w", err))
	}
	if err := os.RemoveAll(fixture.homeDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove composition home: %w", err))
	}
	fmt.Fprintf(os.Stderr, "composition lifecycle report: root_builds=%d process_starts=%d process_stops=%d api_server_starts=%d tracked_sessions=%d closed_sessions=%d\n", fixture.rootBuilds.Load(), fixture.processStarts.Load(), fixture.processStops.Load(), fixture.apiStarts.Load(), tracked, closed)
	return shutdownErr
}

var _ platformprocess.CommandRunner = (*compositionCommandRunner)(nil)
