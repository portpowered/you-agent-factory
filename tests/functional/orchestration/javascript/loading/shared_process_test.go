package loading_test

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

var (
	loadingFixtureMu     sync.Mutex
	sharedLoadingFixture *loadingFixture
)

// TestMain owns the one reusable process for this package. Individual tests
// retain their original top-level selectors, while the shared fixture keeps
// process construction and hosted-server startup out of every scenario.
func TestMain(m *testing.M) {
	code := m.Run()

	loadingFixtureMu.Lock()
	fixture := sharedLoadingFixture
	loadingFixtureMu.Unlock()
	if fixture != nil {
		if err := fixture.shutdown(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

// TestJavaScriptLoadingFixturePartialStartUnwinds proves a real process
// startup failure preserves the original error and closes the listener that
// was acquired by the injected HTTP transport edge.
func TestJavaScriptLoadingFixturePartialStartUnwinds(t *testing.T) {
	hostDir := scaffoldLoadingHostFactory(t)
	homeDir := t.TempDir()
	writeLoadingGlobalConfig(t, homeDir)

	original := errors.New("injected loading fixture start failure")
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

	runner := newLoadingCommandRunner()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      failingStarter,
		ProviderCommandRunner: runner,
		FactoryRuntimeWorkflowHome: func() (string, error) {
			return homeDir, nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess(loading partial start): %v", err)
	}

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = loadingCustomerEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	got := process.Execute(inputs.Input)
	closeContext, cancel := context.WithTimeout(context.Background(), loadingFixtureTimeout)
	closeErr := process.Close(closeContext)
	cancel()
	if closeErr != nil {
		t.Fatalf("close partial-start process: %v", closeErr)
	}
	if !errors.Is(got, original) && !strings.Contains(gotErrorText(got), original.Error()) {
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
		client := http.Client{Timeout: time.Second}
		response, probeErr := client.Get(rawURL + "/status")
		if probeErr == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("partial-start listener remained available: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
		}
	}
	t.Logf("loading partial-start lifecycle report: process_closed=true api_starter_calls=%d listener_open=%d provider_calls=%d original_error=%q", starterCalls.Load(), listenerOpen.Load(), runner.CallCount(), original)
}

func gotErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Keep the original top-level test identities so focused -run selectors and
// review tooling continue to address the same behavior rows as the baseline.
func TestInlineJavaScriptFactoryRunsFromCLI(t *testing.T) {
	runInlineJavaScriptFactoryRunsFromCLI(t, loadingFixtureForTest(t))
}

func TestInlineJavaScriptFactoryRunsOrderedTwoStagePipeline(t *testing.T) {
	runInlineJavaScriptFactoryRunsOrderedTwoStagePipeline(t, loadingFixtureForTest(t))
}

func TestInlineJavaScriptSyntaxErrorReturnsSourceLocation(t *testing.T) {
	runInlineJavaScriptSyntaxErrorReturnsSourceLocation(t, loadingFixtureForTest(t))
}

func TestJavaScriptFactoryFileRunsRelativeImportsFromFactoryRoot(t *testing.T) {
	runJavaScriptFactoryFileRunsRelativeImportsFromFactoryRoot(t, loadingFixtureForTest(t))
}

func TestJavaScriptFactoryMissingImportFailsActionably(t *testing.T) {
	runJavaScriptFactoryMissingImportFailsActionably(t, loadingFixtureForTest(t))
}

func TestTypeScriptFactoryTranspilesAndRuns(t *testing.T) {
	runTypeScriptFactoryTranspilesAndRuns(t, loadingFixtureForTest(t))
}

func TestTypeScriptTypeOrSyntaxFailureReturnsCustomerDiagnostic(t *testing.T) {
	runTypeScriptTypeOrSyntaxFailureReturnsCustomerDiagnostic(t, loadingFixtureForTest(t))
}

func TestTypeScriptSourceMapReportsAuthoredLocation(t *testing.T) {
	runTypeScriptSourceMapReportsAuthoredLocation(t, loadingFixtureForTest(t))
}

func TestNamedJavaScriptFactoryRunsThroughStandardCLI(t *testing.T) {
	runNamedJavaScriptFactoryRunsThroughStandardCLI(t, loadingFixtureForTest(t))
}

func TestNamedJavaScriptFactoryRunsThroughAPIInvocation(t *testing.T) {
	runNamedJavaScriptFactoryRunsThroughAPIInvocation(t, loadingFixtureForTest(t))
}

func TestNamedJavaScriptFactoryUsesSameFactorySessionControls(t *testing.T) {
	runNamedJavaScriptFactoryUsesSameFactorySessionControls(t, loadingFixtureForTest(t))
}

func loadingFixtureForTest(t *testing.T) *loadingFixture {
	t.Helper()

	loadingFixtureMu.Lock()
	defer loadingFixtureMu.Unlock()
	if sharedLoadingFixture == nil {
		sharedLoadingFixture = newLoadingFixture(t)
	}
	return sharedLoadingFixture
}

type loadingFixture struct {
	process      support.ApplicationProcess
	api          *support.ProcessAPIServer
	provider     *loadingCommandRunner
	baseURL      string
	hostDir      string
	homeDir      string
	namedCLI     loadingNamedFactory
	namedAPI     loadingNamedFactory
	namedControl loadingNamedFactory

	serverStarted atomic.Bool
	rootBuilds    atomic.Int32
	processStarts atomic.Int32
	processStops  atomic.Int32
	apiStarts     atomic.Int32
	requestNumber atomic.Uint64

	processCancel context.CancelFunc
	processDone   chan struct{}
	processMu     sync.Mutex
	processErr    error
	apiStopped    chan struct{}
	apiStopOnce   sync.Once

	sessionMu sync.Mutex
	sessions  map[string]loadingSession
	closed    map[string]struct{}
}

type loadingNamedFactory struct {
	name       string
	factoryDir string
}

type loadingSession struct {
	mode      string
	requestID string
	rootDir   string
	homeDir   string
}

// loadingCommandRunner is the provider boundary used by the real process.
// The ordered workflow gets deterministic provider-native output while all
// other rows can assert that no child dispatch was added to the call count.
type loadingCommandRunner struct {
	mu       sync.Mutex
	requests []platformprocess.CommandRequest
}

func newLoadingCommandRunner() *loadingCommandRunner {
	return &loadingCommandRunner{}
}

func (runner *loadingCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	runner.mu.Lock()
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	runner.requests = append(runner.requests, request)
	runner.mu.Unlock()

	prompt := strings.TrimSpace(string(request.Stdin))
	label := "child"
	if strings.Contains(prompt, "stage-two-after") {
		label = "stage-two"
	} else if strings.Contains(prompt, "stage-one-input") {
		label = "stage-one"
	}
	return platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("provider:" + label + ":" + prompt),
	}, nil
}

func (runner *loadingCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

var _ platformprocess.CommandRunner = (*loadingCommandRunner)(nil)

func newLoadingFixture(t *testing.T) *loadingFixture {
	t.Helper()

	homeDir, err := os.MkdirTemp("", "you-functional-loading-home-")
	if err != nil {
		t.Fatalf("create loading home: %v", err)
	}
	hostDir, err := os.MkdirTemp("", "you-functional-loading-factory-")
	if err != nil {
		_ = os.RemoveAll(homeDir)
		t.Fatalf("create loading factory: %v", err)
	}
	if err := writeLoadingHostFactory(hostDir); err != nil {
		_ = os.RemoveAll(hostDir)
		_ = os.RemoveAll(homeDir)
		t.Fatalf("write loading host factory: %v", err)
	}
	writeLoadingGlobalConfig(t, homeDir)

	api := support.NewProcessAPIServer()
	runner := newLoadingCommandRunner()
	fixture := &loadingFixture{
		api:         api,
		provider:    runner,
		hostDir:     hostDir,
		homeDir:     homeDir,
		processDone: make(chan struct{}),
		apiStopped:  make(chan struct{}),
		sessions:    make(map[string]loadingSession, loadingBehaviorCount),
		closed:      make(map[string]struct{}, loadingBehaviorCount),
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.serveAPIServer,
		ProviderCommandRunner: runner,
		FactoryRuntimeWorkflowHome: func() (string, error) {
			return homeDir, nil
		},
	})
	if err != nil {
		_ = os.RemoveAll(hostDir)
		_ = os.RemoveAll(homeDir)
		t.Fatalf("BuildProcess(loading): %v", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)

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
	return loadingNamedFactory{name: name, factoryDir: factoryDir}
}

func (fixture *loadingFixture) serveAPIServer(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	fixture.apiStarts.Add(1)
	err := fixture.api.Start(ctx, request)
	fixture.apiStopOnce.Do(func() { close(fixture.apiStopped) })
	return err
}

func (fixture *loadingFixture) startAPIServer(t *testing.T) {
	t.Helper()
	if fixture.serverStarted.Load() {
		return
	}
	processContext, cancel := context.WithCancel(context.Background())
	fixture.processCancel = cancel
	inputs := support.FakeInputs(processContext, []string{
		"you", "run", "--dir", fixture.hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = loadingCustomerEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostDir
	fixture.processStarts.Add(1)
	go func() {
		err := fixture.process.Execute(inputs.Input)
		fixture.processMu.Lock()
		fixture.processErr = err
		fixture.processMu.Unlock()
		fixture.processStops.Add(1)
		close(fixture.processDone)
	}()

	baseURL, err := fixture.api.WaitForBaseURL(loadingFixtureTimeout)
	if err != nil {
		cancel()
		<-fixture.processDone
		t.Fatalf("wait for loading API: %v", err)
	}
	fixture.baseURL = baseURL
	support.WaitForStatus(t, baseURL, loadingFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("loading API server starts = %d, want one", got)
	}
	fixture.serverStarted.Store(true)
}

func writeLoadingHostFactory(dir string) error {
	cfg := map[string]any{
		"name": "javascript-loading-host",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "loading-host.workflow.js",
			},
		},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), raw, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "loading-host.workflow.js"), []byte(loadingHostWorkflow), 0o600)
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
	if err := writeLoadingHostFactory(dir); err != nil {
		t.Fatalf("write loading host workflow: %v", err)
	}
	return dir
}

func writeLoadingGlobalConfig(t *testing.T, homeDir string) {
	t.Helper()
	dir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create loading global config directory: %v", err)
	}
	config := map[string]any{
		"defaults": map[string]any{
			"workerModelProvider": "openai",
			"workerModel":         "default-model",
		},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal loading global config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), raw, 0o600); err != nil {
		t.Fatalf("write loading global config: %v", err)
	}
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
	// session service when Process.Execute returns. Not-found is the expected
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
}

func (fixture *loadingFixture) recoverAfterLoadFailure(t *testing.T, label string) {
	t.Helper()
	dir := scaffoldLoadingRecoveryFactory(t, label)
	providerCalls := fixture.provider.CallCount()
	result, inputs := fixture.runCLIInvocation(
		t,
		[]string{
			"you", "--json", "run",
			"--factory", filepath.Join(dir, "factory.json"),
			"--output", "primary",
			"--no-record",
			"recovery",
		},
		dir,
		t.TempDir(),
	)
	if got := fixture.provider.CallCount(); got != providerCalls {
		t.Fatalf("provider command runner call count = %d, want unchanged at %d after %s recovery", got, providerCalls, label)
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

func (fixture *loadingFixture) shutdown() error {
	if fixture.processCancel != nil {
		fixture.processCancel()
		<-fixture.processDone
	}

	closeContext, cancel := context.WithTimeout(context.Background(), loadingFixtureTimeout)
	defer cancel()
	closeErr := fixture.process.Close(closeContext)

	fixture.processMu.Lock()
	processErr := fixture.processErr
	fixture.processMu.Unlock()
	var shutdownErr error
	if processErr != nil && !errors.Is(processErr, context.Canceled) {
		shutdownErr = fmt.Errorf("loading Process.Execute shutdown: %w", processErr)
	}
	if closeErr != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close loading process: %w", closeErr))
	}
	if got := fixture.rootBuilds.Load(); got != 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("loading root builds = %d, want one", got))
	}
	if got := fixture.processStarts.Load(); got > 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("loading process starts = %d, want at most one", got))
	}
	if got := fixture.processStops.Load(); got != fixture.processStarts.Load() {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("loading process stops = %d, starts = %d", got, fixture.processStarts.Load()))
	}
	if got := fixture.apiStarts.Load(); got > 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("loading API server starts = %d, want at most one", got))
	}

	fixture.sessionMu.Lock()
	tracked := len(fixture.sessions)
	closed := len(fixture.closed)
	fixture.sessionMu.Unlock()
	if tracked != closed {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("loading sessions closed = %d/%d", closed, tracked))
	}

	listenerClosed := fixture.apiStarts.Load() == 0
	if fixture.apiStarts.Load() > 0 {
		// Process.Execute has returned before shutdown reaches this point, so
		// the API edge's terminal callback is the deterministic close signal.
		<-fixture.apiStopped
		listenerClosed = true
	}
	if strings.TrimSpace(fixture.baseURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			listenerClosed = false
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("loading API listener remained available after shutdown: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body))))
		}
	}
	if err := os.RemoveAll(fixture.hostDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove loading factory: %w", err))
	}
	if err := os.RemoveAll(fixture.homeDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove loading home: %w", err))
	}
	fmt.Fprintf(os.Stderr, "loading lifecycle report: root_builds=%d process_starts=%d process_stops=%d api_server_starts=%d tracked_sessions=%d closed_sessions=%d provider_calls=%d listener_closed=%t\n", fixture.rootBuilds.Load(), fixture.processStarts.Load(), fixture.processStops.Load(), fixture.apiStarts.Load(), tracked, closed, fixture.provider.CallCount(), listenerClosed)
	return shutdownErr
}
