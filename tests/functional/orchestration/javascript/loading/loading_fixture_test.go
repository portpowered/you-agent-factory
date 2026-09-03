package loading_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	loadingFixtureTimeout = 15 * time.Second
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

// Keep the original top-level test identities so focused -run selectors and
// review tooling continue to address the same behavior rows as the baseline.
func TestInlineJavaScriptFactoryRunsFromCLI(t *testing.T) {
	t.Parallel()
	runInlineJavaScriptFactoryRunsFromCLI(t, loadingFixtureForTest(t))
}

func TestInlineJavaScriptFactoryRunsOrderedTwoStagePipeline(t *testing.T) {
	t.Parallel()
	runInlineJavaScriptFactoryRunsOrderedTwoStagePipeline(t, loadingFixtureForTest(t))
}

func TestInlineJavaScriptFactoryRunsThroughAPIInvocation(t *testing.T) {
	t.Parallel()
	runInlineJavaScriptFactoryRunsThroughAPIInvocation(t, loadingFixtureForTest(t))
}

func TestInlineJavaScriptSyntaxErrorReturnsSourceLocation(t *testing.T) {
	t.Parallel()
	runInlineJavaScriptSyntaxErrorReturnsSourceLocation(t, loadingFixtureForTest(t))
}

func TestJavaScriptFactoryFileRunsRelativeImportsFromFactoryRoot(t *testing.T) {
	t.Parallel()
	runJavaScriptFactoryFileRunsRelativeImportsFromFactoryRoot(t, loadingFixtureForTest(t))
}

func TestJavaScriptFactoryMissingImportFailsActionably(t *testing.T) {
	t.Parallel()
	runJavaScriptFactoryMissingImportFailsActionably(t, loadingFixtureForTest(t))
}

func TestTypeScriptFactoryTranspilesAndRuns(t *testing.T) {
	t.Parallel()
	runTypeScriptFactoryTranspilesAndRuns(t, loadingFixtureForTest(t))
}

func TestTypeScriptSourceMapReportsAuthoredLocation(t *testing.T) {
	t.Parallel()
	runTypeScriptSourceMapReportsAuthoredLocation(t, loadingFixtureForTest(t))
}

func TestNamedJavaScriptFactoryRunsThroughStandardCLI(t *testing.T) {
	t.Parallel()
	runNamedJavaScriptFactoryRunsThroughStandardCLI(t, loadingFixtureForTest(t))
}

func TestNamedJavaScriptFactoryRunsThroughAPIInvocation(t *testing.T) {
	t.Parallel()
	runNamedJavaScriptFactoryRunsThroughAPIInvocation(t, loadingFixtureForTest(t))
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

	serverOnce    sync.Once
	serverErr     error
	requestNumber atomic.Uint64

	processCancel context.CancelFunc
	processDone   chan struct{}
	processMu     sync.Mutex
	processErr    error

	sessionMu sync.Mutex
	sessions  map[string]loadingSession
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
// The ordered workflow gets deterministic provider-native output.
type loadingCommandRunner struct{}

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
		sessions:    make(map[string]loadingSession),
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

	fixture.namedCLI = fixture.prepareNamedFactory(t, "cli", false)
	// CLI and API prove two public entry paths to the same installed customer
	// Factory. Installing an identical second copy added setup cost without
	// protecting a distinct identity or persistence guarantee.
	fixture.namedAPI = fixture.namedCLI
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
	return fixture.api.Start(ctx, request)
}

func (fixture *loadingFixture) startAPIServer(t *testing.T) {
	t.Helper()
	fixture.serverOnce.Do(func() {
		processContext, cancel := context.WithCancel(context.Background())
		fixture.processCancel = cancel
		inputs := support.FakeInputs(processContext, []string{
			"you", "run", "--session", uuid.NewString(), "--dir", fixture.hostDir,
			"--continuously", "--with-server", "--quiet", "--no-record",
		})
		inputs.Input.Env = loadingCustomerEnvironment(fixture.homeDir)
		inputs.Input.WorkingDirectory = fixture.hostDir
		go func() {
			err := fixture.process.Execute(inputs.Input)
			fixture.processMu.Lock()
			fixture.processErr = err
			fixture.processMu.Unlock()
			close(fixture.processDone)
		}()

		fixture.baseURL, fixture.serverErr = fixture.api.WaitForBaseURL(loadingFixtureTimeout)
		if fixture.serverErr != nil {
			cancel()
			<-fixture.processDone
		}
	})
	if fixture.serverErr != nil {
		t.Fatalf("wait for loading API: %v", fixture.serverErr)
	}
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
	args = loadingArgsWithIsolatedSession(args)
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = loadingCustomerEnvironment(homeDir)
	inputs.Input.WorkingDirectory = workingDirectory
	return inputs, fixture.process.Execute(inputs.Input)
}

// loadingArgsWithIsolatedSession makes each local customer invocation own its
// Factory Session. A shared process is construction reuse, not permission for
// independent commands to fall back to the process-wide ~default identity.
func loadingArgsWithIsolatedSession(args []string) []string {
	isolated := append([]string(nil), args...)
	runIndex := -1
	for index, arg := range isolated {
		if arg == "--session" || strings.HasPrefix(arg, "--session=") {
			return isolated
		}
		if arg == "run" && runIndex < 0 {
			runIndex = index
		}
	}
	if runIndex < 0 {
		return isolated
	}
	withSession := make([]string, 0, len(isolated)+2)
	withSession = append(withSession, isolated[:runIndex+1]...)
	withSession = append(withSession, "--session", uuid.NewString())
	withSession = append(withSession, isolated[runIndex+1:]...)
	return withSession
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
	for _, session := range fixture.sessions {
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
	})
}

func (fixture *loadingFixture) closeSession(t testing.TB, sessionID string) {
	t.Helper()

	fixture.sessionMu.Lock()
	session, ok := fixture.sessions[sessionID]
	delete(fixture.sessions, sessionID)
	fixture.sessionMu.Unlock()
	if !ok {
		t.Errorf("loading Factory Session %q was not tracked during cleanup", sessionID)
		return
	}
	if session.mode == "api" {
		support.TerminateFactorySessionAt(t, fixture.baseURL, sessionID)
	}
}

func (fixture *loadingFixture) recoverAfterLoadFailure(t *testing.T, label string) {
	t.Helper()
	dir := scaffoldLoadingRecoveryFactory(t, label)
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
		fixture.homeDir,
	)
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
	if err := os.RemoveAll(fixture.hostDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove loading factory: %w", err))
	}
	if err := os.RemoveAll(fixture.homeDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove loading home: %w", err))
	}
	return shutdownErr
}
