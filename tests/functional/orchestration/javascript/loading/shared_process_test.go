package loading_test

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

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	loadingFixtureTimeout = 15 * time.Second
	loadingBehaviorCount  = 11
	loadingHostWorkflow   = `return "javascript-loading-host";`
	loadingRecoveryResult = "<LOADING_RECOVERY>"
)

type loadingResourceKind string

const (
	loadingResourceProcess  loadingResourceKind = "process"
	loadingResourcePort     loadingResourceKind = "port"
	loadingResourceListener loadingResourceKind = "listener"
	loadingResourceSession  loadingResourceKind = "session"
	loadingResourceStream   loadingResourceKind = "stream"
	loadingResourceRoute    loadingResourceKind = "route"
	loadingResourceRoot     loadingResourceKind = "root"
	loadingResourceWorktree loadingResourceKind = "worktree"
	loadingResourceMutable  loadingResourceKind = "mutable-state"
)

var loadingResourceKinds = []loadingResourceKind{
	loadingResourceProcess,
	loadingResourcePort,
	loadingResourceListener,
	loadingResourceSession,
	loadingResourceStream,
	loadingResourceRoute,
	loadingResourceRoot,
	loadingResourceWorktree,
	loadingResourceMutable,
}

type loadingResourceLedger struct {
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

type loadingResourceCounts struct {
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

func (ledger *loadingResourceLedger) counter(kind loadingResourceKind) *atomic.Int32 {
	switch kind {
	case loadingResourceProcess:
		return &ledger.process
	case loadingResourcePort:
		return &ledger.port
	case loadingResourceListener:
		return &ledger.listener
	case loadingResourceSession:
		return &ledger.session
	case loadingResourceStream:
		return &ledger.stream
	case loadingResourceRoute:
		return &ledger.route
	case loadingResourceRoot:
		return &ledger.root
	case loadingResourceWorktree:
		return &ledger.worktree
	case loadingResourceMutable:
		return &ledger.mutable
	default:
		panic("unknown loading resource kind: " + string(kind))
	}
}

func (ledger *loadingResourceLedger) acquire(kind loadingResourceKind) {
	ledger.counter(kind).Add(1)
}

func (ledger *loadingResourceLedger) release(kind loadingResourceKind) {
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

func (ledger *loadingResourceLedger) snapshot() loadingResourceCounts {
	return loadingResourceCounts{
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

// unwindLoadingFixtureStart returns the injected cause unchanged after
// draining every resource cell acquired before a fixture start failure.
func unwindLoadingFixtureStart(ledger *loadingResourceLedger, cause error) error {
	for _, kind := range loadingResourceKinds {
		for ledger.counter(kind).Load() > 0 {
			ledger.release(kind)
		}
	}
	return cause
}

// TestJavaScriptLoadingFixturePartialStartUnwinds proves a partial package
// fixture start cannot strand process, listener, session, stream, routing,
// root, worktree, or mutable runner resources and preserves the original
// startup error.
func TestJavaScriptLoadingFixturePartialStartUnwinds(t *testing.T) {
	ledger := &loadingResourceLedger{}
	for _, kind := range loadingResourceKinds {
		ledger.acquire(kind)
	}
	original := errors.New("injected loading fixture start failure")

	if got := unwindLoadingFixtureStart(ledger, original); got != original {
		t.Fatalf("partial-start error = %v, want original error %v", got, original)
	}
	counts := ledger.snapshot()
	if counts != (loadingResourceCounts{}) {
		t.Fatalf("partial-start resource counts = %#v, want all zero", counts)
	}
	t.Logf("loading partial-start lifecycle report: process=%d port=%d listener=%d session=%d stream=%d route=%d root=%d worktree=%d mutable-state=%d original_error=%q", counts.process, counts.port, counts.listener, counts.session, counts.stream, counts.route, counts.root, counts.worktree, counts.mutable, original)
}

// TestJavaScriptLoadingBehavior runs the 11 loading rows sequentially through
// one process-owned host. Each row still receives an explicit, unique durable
// Factory Session and authored root; failure-before-session rows receive a
// fresh recovery invocation to prove the next source can load cleanly.
func TestJavaScriptLoadingBehavior(t *testing.T) {
	fixture := newLoadingFixture(t)

	cases := []struct {
		name string
		run  func(*testing.T, *loadingFixture)
	}{
		{"inline/success", runInlineJavaScriptFactoryRunsFromCLI},
		{"inline/ordered-pipeline", runInlineJavaScriptFactoryRunsOrderedTwoStagePipeline},
		{"inline/syntax-error", runInlineJavaScriptSyntaxErrorReturnsSourceLocation},
		{"file/relative-import", runJavaScriptFactoryFileRunsRelativeImportsFromFactoryRoot},
		{"file/missing-import", runJavaScriptFactoryMissingImportFailsActionably},
		{"typescript/success", runTypeScriptFactoryTranspilesAndRuns},
		{"typescript/syntax-error", runTypeScriptTypeOrSyntaxFailureReturnsCustomerDiagnostic},
		{"typescript/source-map", runTypeScriptSourceMapReportsAuthoredLocation},
		{"named/standard-cli", runNamedJavaScriptFactoryRunsThroughStandardCLI},
		{"named/api", runNamedJavaScriptFactoryRunsThroughAPIInvocation},
		{"named/session-controls", runNamedJavaScriptFactoryUsesSameFactorySessionControls},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, fixture)
		})
	}
}

type loadingFixture struct {
	owner         testing.TB
	process       *initializerapplication.Process
	api           *support.ProcessAPIServer
	apiStarter    *loadingAPIServerStarter
	resources     *loadingResourceLedger
	provider      *support.RecordingCommandRunner
	baseURL       string
	hostDir       string
	homeDir       string
	namedCLI      loadingNamedFactory
	namedAPI      loadingNamedFactory
	namedControl  loadingNamedFactory
	serverStarted atomic.Bool
	command       *support.ProcessCommand

	rootBuilds    atomic.Int32
	processStarts atomic.Int32
	processStops  atomic.Int32
	requestNumber atomic.Uint64

	sessionMu sync.Mutex
	sessions  map[string]loadingSession
	closed    map[string]struct{}
}

type loadingNamedFactory struct {
	name       string
	sourceDir  string
	factoryDir string
}

type loadingSession struct {
	mode      string
	requestID string
	rootDir   string
	homeDir   string
}

type loadingAPIServerStarter struct {
	api       *support.ProcessAPIServer
	resources *loadingResourceLedger
	starts    atomic.Int32
	stopped   chan struct{}
	stopOnce  sync.Once
}

func (starter *loadingAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	starter.resources.acquire(loadingResourcePort)
	starter.resources.acquire(loadingResourceListener)
	defer starter.resources.release(loadingResourcePort)
	defer starter.resources.release(loadingResourceListener)
	err := starter.api.Start(ctx, request)
	starter.stopOnce.Do(func() { close(starter.stopped) })
	return err
}

func newLoadingFixture(t *testing.T) *loadingFixture {
	t.Helper()

	homeDir := t.TempDir()
	hostDir := scaffoldLoadingHostFactory(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	resources := &loadingResourceLedger{}
	api := support.NewProcessAPIServer()
	apiStarter := &loadingAPIServerStarter{
		api:       api,
		resources: resources,
		stopped:   make(chan struct{}),
	}
	fixture := &loadingFixture{
		api:        api,
		owner:      t,
		apiStarter: apiStarter,
		resources:  resources,
		provider:   runner,
		hostDir:    hostDir,
		homeDir:    homeDir,
		sessions:   make(map[string]loadingSession),
		closed:     make(map[string]struct{}),
	}

	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.apiStarter.Start,
		ProviderCommandRunner: runner,
		FactoryRuntimeWorkflowHome: func() (string, error) {
			return homeDir, nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess(loading): %v", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)
	fixture.resources.acquire(loadingResourceRoot)
	fixture.resources.acquire(loadingResourceProcess)
	fixture.resources.acquire(loadingResourceRoute)

	// Register the lifecycle census before process cleanup so the final report
	// runs after the command, root, route, listener, and session cleanups.
	t.Cleanup(func() { fixture.assertCleanup(t) })
	t.Cleanup(func() { fixture.processStops.Add(1) })
	t.Cleanup(func() {
		fixture.resources.release(loadingResourceRoute)
		fixture.resources.release(loadingResourceProcess)
		fixture.resources.release(loadingResourceRoot)
	})
	support.CleanupProcess(t, process)

	fixture.namedCLI = fixture.prepareNamedFactory(t, "cli", false)
	fixture.namedAPI = fixture.prepareNamedFactory(t, "api", false)
	fixture.namedControl = fixture.prepareNamedFactory(t, "controls", true)
	return fixture
}

func (fixture *loadingFixture) prepareNamedFactory(t *testing.T, label string, busy bool) loadingNamedFactory {
	t.Helper()
	name := fixture.nextNamedFactoryName(label)
	sourceDir := scaffoldNamedInlineJavaScriptFactorySource(t, name)
	if busy {
		sourceDir = scaffoldNamedBusyLoopJavaScriptFactorySource(t, name)
	}
	factoryDir := support.CreateNamedFactoryWithProcess(
		t,
		fixture.process,
		fixture.homeDir,
		sourceDir,
		name,
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)
	return loadingNamedFactory{name: name, sourceDir: sourceDir, factoryDir: factoryDir}
}

func (fixture *loadingFixture) startAPIServer(t *testing.T) {
	t.Helper()
	if fixture.serverStarted.Load() {
		return
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", fixture.hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = loadingCustomerEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostDir
	fixture.processStarts.Add(1)
	fixture.command = support.StartProcessCommand(fixture.owner, fixture.process, inputs.Input)

	baseURL, err := fixture.api.WaitForBaseURL(loadingFixtureTimeout)
	if err != nil {
		t.Fatalf("wait for loading API: %v", err)
	}
	fixture.baseURL = baseURL
	support.WaitForStatus(t, baseURL, loadingFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Fatalf("loading API server starts = %d, want one", got)
	}
	fixture.serverStarted.Store(true)
}

func scaffoldLoadingHostFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-loading-host",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "loading-host.workflow.js",
			},
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "loading-host.workflow.js"), []byte(loadingHostWorkflow), 0o600); err != nil {
		t.Fatalf("write loading host workflow: %v", err)
	}
	return dir
}

func loadingCustomerEnvironment(homeDir string) []string {
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

func (fixture *loadingFixture) executeCLI(
	t testing.TB,
	args []string,
	workingDirectory string,
	homeDir string,
) (*support.CapturedInputs, error) {
	t.Helper()
	if fixture == nil || fixture.process == nil {
		t.Fatal("loading process is unavailable")
	}
	if strings.TrimSpace(homeDir) == "" {
		homeDir = fixture.homeDir
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = loadingCustomerEnvironment(homeDir)
	inputs.Input.WorkingDirectory = workingDirectory
	return inputs, fixture.process.Execute(inputs.Input)
}

func (fixture *loadingFixture) runCLIInvocation(
	t *testing.T,
	args []string,
	workingDirectory string,
	homeDir string,
) (factoryapi.InvocationResponse, *support.CapturedInputs) {
	return fixture.runCLIInvocationAtRoot(t, args, workingDirectory, homeDir, workingDirectory)
}

func (fixture *loadingFixture) runCLIInvocationAtRoot(
	t *testing.T,
	args []string,
	workingDirectory string,
	homeDir string,
	rootDir string,
) (factoryapi.InvocationResponse, *support.CapturedInputs) {
	t.Helper()
	inputs, err := fixture.executeCLI(t, args, workingDirectory, homeDir)
	if err != nil {
		t.Fatalf("Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful JSON invocation", inputs.Stderr())
	}
	result := decodeSingleInvocationResponse(t, inputs.Stdout())
	fixture.trackInvocationSession(t, result, rootDir, homeDir)
	return result, inputs
}

func (fixture *loadingFixture) trackInvocationSession(
	t testing.TB,
	result factoryapi.InvocationResponse,
	rootDir string,
	homeDir string,
) {
	t.Helper()
	if result.SessionId == nil || strings.TrimSpace(*result.SessionId) == "" {
		t.Fatalf("loading CLI invocation result = %#v, want explicit Factory Session ID", result)
	}
	fixture.trackSession(t, *result.SessionId, result.RequestId, rootDir, homeDir, "cli")
}

func (fixture *loadingFixture) nextRequestID(label string) string {
	return fmt.Sprintf("javascript-loading-%s-%d", label, fixture.requestNumber.Add(1))
}

func (fixture *loadingFixture) nextNamedFactoryName(label string) string {
	return fmt.Sprintf("%s-%s-%d", namedJavaScriptFactoryName, label, fixture.requestNumber.Add(1))
}

func (fixture *loadingFixture) trackSession(
	t testing.TB,
	sessionID string,
	requestID string,
	rootDir string,
	homeDir string,
	mode string,
) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("loading Factory Session ID is empty")
	}
	if strings.TrimSpace(rootDir) == "" {
		t.Fatal("loading Factory Session root is empty")
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.sessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("loading Factory Session ID %q was reused", sessionID)
	}
	for existingID, session := range fixture.sessions {
		if filepath.Clean(session.rootDir) == filepath.Clean(rootDir) {
			fixture.sessionMu.Unlock()
			t.Fatalf("loading Factory Session roots reused by %q and %q: %s", existingID, sessionID, rootDir)
		}
		if strings.TrimSpace(requestID) != "" && session.requestID == requestID {
			fixture.sessionMu.Unlock()
			t.Fatalf("loading request ID %q was reused", requestID)
		}
	}
	if strings.TrimSpace(homeDir) == "" {
		homeDir = fixture.homeDir
	}
	fixture.sessions[sessionID] = loadingSession{
		mode:      mode,
		requestID: requestID,
		rootDir:   rootDir,
		homeDir:   homeDir,
	}
	fixture.resources.acquire(loadingResourceSession)
	fixture.resources.acquire(loadingResourceWorktree)
	fixture.resources.acquire(loadingResourceMutable)
	fixture.sessionMu.Unlock()

	t.Cleanup(func() {
		fixture.closeSession(t, sessionID)
		fixture.markSessionClosed(sessionID)
	})
}

func (fixture *loadingFixture) closeSession(t testing.TB, sessionID string) {
	t.Helper()

	fixture.sessionMu.Lock()
	session, ok := fixture.sessions[sessionID]
	fixture.sessionMu.Unlock()
	if !ok {
		t.Errorf("loading Factory Session %q was not tracked during cleanup", sessionID)
		return
	}
	if session.mode == "api" {
		support.TerminateFactorySessionAt(t, fixture.baseURL, sessionID)
		return
	}

	// A completed local one-shot invocation releases its invocation-local
	// session service when Process.Execute returns. The local control probe
	// therefore reports not-found after the invocation, which is the expected
	// terminal observation; any other cleanup error remains actionable.
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "session", "terminate", sessionID,
	})
	inputs.Input.Env = loadingCustomerEnvironment(session.homeDir)
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

func (fixture *loadingFixture) markSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	if _, alreadyClosed := fixture.closed[sessionID]; alreadyClosed {
		fixture.sessionMu.Unlock()
		return
	}
	fixture.closed[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	fixture.resources.release(loadingResourceMutable)
	fixture.resources.release(loadingResourceWorktree)
	fixture.resources.release(loadingResourceSession)
}

func (fixture *loadingFixture) recoverAfterLoadFailure(t *testing.T, label string) {
	t.Helper()
	dir := scaffoldLoadingRecoveryFactory(t, label)
	result, inputs := fixture.runCLIInvocation(
		t,
		[]string{
			"you", "--json", "run",
			"--factory", filepath.Join(dir, "factory.json"),
			"--with-mock-workers", filepath.Join(dir, "mock-workers.json"),
			"--output", "primary",
			"--no-record",
			"recovery",
		},
		dir,
		t.TempDir(),
	)
	if fixture.provider.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 after %s recovery", fixture.provider.CallCount(), label)
	}
	assertLoadingRecoveryOutcome(t, result)
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
}

func scaffoldLoadingRecoveryFactory(t *testing.T, label string) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-loading-recovery-" + label,
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": false,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "recovery.workflow.js",
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "recovery.workflow.js"), []byte(`workflow.final("`+loadingRecoveryResult+`");`), 0o600); err != nil {
		t.Fatalf("write loading recovery workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mock-workers.json"), []byte(`{"mockWorkers":[]}`), 0o600); err != nil {
		t.Fatalf("write loading recovery mock-workers config: %v", err)
	}
	return dir
}

func assertLoadingRecoveryOutcome(t *testing.T, result factoryapi.InvocationResponse) {
	t.Helper()
	if result.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("loading recovery status = %q, want COMPLETED", result.Status)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("loading recovery primary result = %#v, want one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode loading recovery result: %v", err)
	}
	if part.Text != loadingRecoveryResult {
		t.Fatalf("loading recovery result = %q, want %q", part.Text, loadingRecoveryResult)
	}
}

func (fixture *loadingFixture) assertCleanup(t testing.TB) {
	t.Helper()
	if got := fixture.rootBuilds.Load(); got != 1 {
		t.Errorf("loading root builds = %d, want one", got)
	}
	if got := fixture.processStarts.Load(); got != 1 {
		t.Errorf("loading process starts = %d, want one", got)
	}
	if got := fixture.processStops.Load(); got != 1 {
		t.Errorf("loading process stops = %d, want one", got)
	}
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Errorf("loading API starts = %d, want one", got)
	}
	select {
	case <-fixture.apiStarter.stopped:
	case <-time.After(loadingFixtureTimeout):
		t.Errorf("loading API listener did not stop")
	}

	counts := fixture.resources.snapshot()
	if counts != (loadingResourceCounts{}) {
		t.Errorf("loading active resource counts = %#v, want all zero", counts)
	}
	fixture.sessionMu.Lock()
	tracked := len(fixture.sessions)
	closed := len(fixture.closed)
	fixture.sessionMu.Unlock()
	if tracked != loadingBehaviorCount {
		t.Errorf("loading tracked top-level sessions = %d, want %d", tracked, loadingBehaviorCount)
	}
	if tracked != closed {
		t.Errorf("loading sessions closed = %d/%d, want all tracked sessions closed", closed, tracked)
	}
	if got := fixture.provider.CallCount(); got != 0 {
		t.Errorf("loading provider calls = %d, want zero because every row uses mock workers or no child dispatch", got)
	}

	if strings.TrimSpace(fixture.baseURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Errorf("loading API listener remained available after cleanup: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
		}
	}
	t.Logf("loading lifecycle report: root_builds=%d process_starts=%d process_stops=%d api_server_starts=%d tracked_sessions=%d closed_sessions=%d provider_calls=%d active={process:%d port:%d listener:%d session:%d stream:%d route:%d root:%d worktree:%d mutable-state:%d}", fixture.rootBuilds.Load(), fixture.processStarts.Load(), fixture.processStops.Load(), fixture.apiStarter.starts.Load(), tracked, closed, fixture.provider.CallCount(), counts.process, counts.port, counts.listener, counts.session, counts.stream, counts.route, counts.root, counts.worktree, counts.mutable)
}
