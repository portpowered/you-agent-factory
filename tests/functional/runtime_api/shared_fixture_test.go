package runtime_api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const runtimeAPIPackageFixtureTimeout = 15 * time.Second

var (
	runtimeAPIFixtureOnce sync.Once
	runtimeAPIFixtureMu   sync.Mutex
	runtimeAPIFixtureVal  *runtimeAPIPackageFixture
	runtimeAPIFixtureErr  error
)

// TestMain gives the eligible runtime API cohort one package-scoped lifecycle.
// The fixture is lazy so isolated tests and short runs do not pay for a daemon
// they do not exercise.
func TestMain(m *testing.M) {
	code := m.Run()

	runtimeAPIFixtureMu.Lock()
	fixture := runtimeAPIFixtureVal
	runtimeAPIFixtureMu.Unlock()
	if fixture != nil {
		if err := fixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "runtime API package fixture cleanup: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}

	os.Exit(code)
}

type runtimeAPIPackageFixture struct {
	rootDir string
	hostDir string
	baseURL string

	process support.ApplicationProcess
	command *runtimeAPIProcessCommand

	apiStarts atomic.Int64

	providerRouter *runtimeAPIProviderRouter
	commandRouter  *runtimeAPICommandRouter
	scriptRouter   *runtimeAPICommandRouter
}

type runtimeAPIScenario struct {
	provider       any
	providerRunner platformprocess.CommandRunner
	scriptRunner   platformprocess.CommandRunner
	models         []string
}

func (fs *functionalAPIServer) URL() string {
	if fs == nil {
		return ""
	}
	if fs.shared != nil {
		return fs.shared.baseURL
	}
	if fs.FunctionalAPIServer != nil {
		return fs.FunctionalAPIServer.URL()
	}
	return ""
}

func (fs *functionalAPIServer) sessionURL(path string) string {
	if fs == nil || fs.sessionID == "" {
		return ""
	}
	return strings.TrimSuffix(fs.URL(), "/") + "/factory-sessions/" + url.PathEscape(fs.sessionID) + path
}

func (fs *functionalAPIServer) workURL(path string) string {
	if fs != nil && fs.shared != nil {
		return fs.sessionURL(path)
	}
	return support.DefaultSessionWorkURL(fs.URL(), path)
}

func (fs *functionalAPIServer) eventsURL() string {
	if fs != nil && fs.shared != nil {
		return support.SessionEventsURL(fs.URL(), fs.sessionID)
	}
	return support.DefaultSessionEventsURL(fs.URL())
}

func (fs *functionalAPIServer) responseEventsURL() string {
	if fs != nil && fs.shared != nil {
		return support.SessionResponseEventsURL(fs.URL(), fs.sessionID)
	}
	return support.SessionResponseEventsURL(fs.URL(), "~default")
}

func (fs *functionalAPIServer) statusURL() string {
	if fs != nil && fs.shared != nil {
		return fs.sessionURL("/status")
	}
	return strings.TrimSuffix(fs.URL(), "/") + "/status"
}

func (fs *functionalAPIServer) StatusURL() string {
	return fs.statusURL()
}

func (fs *functionalAPIServer) Session(t *testing.T) factoryapi.FactorySession {
	t.Helper()
	if fs != nil && fs.shared != nil {
		response := support.GetJSON[factoryapi.FactorySessionGetResponse](t, fs.sessionURL(""))
		session, err := response.AsFactorySession()
		if err != nil {
			t.Fatalf("decode shared Factory Session: %v", err)
		}
		return session
	}
	return support.GetDefaultSession(t, fs.URL())
}

func (fs *functionalAPIServer) GetFactoryEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	if fs != nil && fs.shared != nil {
		return support.GetFactoryEventsForSessionAt(t, fs.URL(), fs.sessionID)
	}
	return support.GetFactoryEventsAt(t, fs.URL())
}

func startSharedFunctionalServer(
	t *testing.T,
	factoryDir string,
	scenario runtimeAPIScenario,
) *functionalAPIServer {
	t.Helper()

	fixture := sharedRuntimeAPIFixture(t)
	provider := runtimeAPIProviderForScenario(t, scenario)
	if provider == nil && scenario.providerRunner == nil {
		// A mock-worker scenario does not call Providers, but registering a
		// fail-closed route keeps an accidental real invocation from escaping
		// the controlled package fixture.
		provider = testutil.NativeProvider{}
	}

	var unregisterProvider func()
	if provider != nil {
		unregisterProvider = fixture.providerRouter.register(factoryDir, scenario.models, provider)
	}
	var unregisterCommand func()
	if scenario.providerRunner != nil {
		unregisterCommand = fixture.commandRouter.register(factoryDir, scenario.providerRunner)
	}
	var unregisterScript func()
	if scenario.scriptRunner != nil {
		unregisterScript = fixture.scriptRouter.register(factoryDir, scenario.scriptRunner)
	}

	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if sessionID == "" || sessionID == "~default" {
		t.Fatalf("shared runtime API Factory Session ID = %q, want unique explicit session", sessionID)
	}
	server := &functionalAPIServer{
		shared:    fixture,
		sessionID: sessionID,
	}
	t.Cleanup(func() {
		// Close the live session before releasing its effect route. This lets
		// cancellation reach any in-flight controlled provider call.
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		if unregisterScript != nil {
			unregisterScript()
		}
		if unregisterCommand != nil {
			unregisterCommand()
		}
		if unregisterProvider != nil {
			unregisterProvider()
		}
	})
	return server
}

func runtimeAPIProviderForScenario(t *testing.T, scenario runtimeAPIScenario) providers.Service {
	t.Helper()
	if scenario.providerRunner != nil {
		return newRuntimeAPICommandProvider(scenario.providerRunner)
	}
	switch provider := scenario.provider.(type) {
	case nil:
		return nil
	case providers.Service:
		return provider
	case interface {
		Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error)
	}:
		return support.ProviderServiceFromInference(provider)
	default:
		t.Fatalf("unsupported shared runtime API provider %T", scenario.provider)
		return nil
	}
}

func sharedRuntimeAPIFixture(t *testing.T) *runtimeAPIPackageFixture {
	t.Helper()
	runtimeAPIFixtureOnce.Do(func() {
		fixture, err := newRuntimeAPIPackageFixture()
		runtimeAPIFixtureMu.Lock()
		runtimeAPIFixtureVal = fixture
		runtimeAPIFixtureErr = err
		runtimeAPIFixtureMu.Unlock()
	})
	runtimeAPIFixtureMu.Lock()
	fixture := runtimeAPIFixtureVal
	err := runtimeAPIFixtureErr
	runtimeAPIFixtureMu.Unlock()
	if err != nil {
		t.Fatalf("construct shared runtime API fixture: %v", err)
	}
	return fixture
}

func newRuntimeAPIPackageFixture() (*runtimeAPIPackageFixture, error) {
	rootDir, err := os.MkdirTemp("", "infinite-you-runtime-api-")
	if err != nil {
		return nil, err
	}
	cleanupOnError := func(cause error) (*runtimeAPIPackageFixture, error) {
		_ = os.RemoveAll(rootDir)
		return nil, cause
	}

	hostDir := filepath.Join(rootDir, "host")
	if err := writeRuntimeAPIFixtureFactory(hostDir, providerBackedModelTransportSmokeConfig()); err != nil {
		return cleanupOnError(err)
	}
	workerPath := filepath.Join(hostDir, "workers", "tts-worker", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workerPath), 0o755); err != nil {
		return cleanupOnError(fmt.Errorf("create fixture worker directory: %w", err))
	}
	if err := os.WriteFile(workerPath, []byte("---\ntype: MODEL_WORKER\nmodel: OMNIVOICE_Q4_K_M\nmodelProvider: CODEX\n---\nDo the work.\n"), 0o644); err != nil {
		return cleanupOnError(fmt.Errorf("write fixture worker configuration: %w", err))
	}
	homeDir := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return cleanupOnError(fmt.Errorf("create fixture home: %w", err))
	}

	fixture := &runtimeAPIPackageFixture{
		rootDir:        rootDir,
		hostDir:        hostDir,
		providerRouter: newRuntimeAPIProviderRouter(),
		commandRouter:  newRuntimeAPICommandRouter("provider"),
		scriptRouter:   newRuntimeAPICommandRouter("script"),
	}
	api := support.NewProcessAPIServer()
	edges := serviceedges.Edges{
		APIServerStarter: platformhttpserver.Starter(func(ctx context.Context, request platformhttpserver.StartRequest) error {
			fixture.apiStarts.Add(1)
			return api.Start(ctx, request)
		}),
		ProviderOverride:          fixture.providerRouter,
		ProviderCommandRunner:     fixture.commandRouter,
		ScriptCommandRunner:       fixture.scriptRouter,
		FactorySessionIDGenerator: uuid.NewString,
	}
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		return cleanupOnError(fmt.Errorf("build production process: %w", err))
	}
	fixture.process = process

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--continuously", "--with-server", "--quiet", "--dir", hostDir, "--no-record",
	})
	inputs.Input.WorkingDirectory = hostDir
	inputs.Input.Env = runtimeAPIEnvironment(inputs.Input.Env, homeDir)
	fixture.command = startRuntimeAPIProcessCommand(process, inputs.Input)
	baseURL, err := api.WaitForBaseURL(runtimeAPIPackageFixtureTimeout)
	if err != nil {
		_ = fixture.close()
		return nil, fmt.Errorf("wait for package API listener: %w", err)
	}
	fixture.baseURL = baseURL

	return fixture, nil
}

func writeRuntimeAPIFixtureFactory(dir string, cfg map[string]any) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create factory directory: %w", err)
	}
	if _, ok := cfg["name"]; !ok {
		cfg["name"] = "runtime-api-shared-host"
	}
	payload, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal factory configuration: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), payload, 0o644); err != nil {
		return fmt.Errorf("write factory configuration: %w", err)
	}
	workstations, _ := cfg["workstations"].([]map[string]any)
	for _, workstation := range workstations {
		name, _ := workstation["name"].(string)
		if name == "" {
			continue
		}
		path := filepath.Join(dir, "workstations", name, "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create workstation configuration directory: %w", err)
		}
		if err := os.WriteFile(path, []byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"), 0o644); err != nil {
			return fmt.Errorf("write workstation configuration: %w", err)
		}
	}
	return nil
}

func runtimeAPIEnvironment(environment []string, homeDir string) []string {
	result := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		name := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if name == "HOME" || name == "USERPROFILE" {
			continue
		}
		result = append(result, entry)
	}
	result = append(result, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return result
}

func (fixture *runtimeAPIPackageFixture) close() error {
	if fixture == nil {
		return nil
	}
	var result error
	if fixture.command != nil {
		fixture.command.stop()
		if err := fixture.command.terminalError(); err != nil && !errors.Is(err, context.Canceled) {
			result = errors.Join(result, fmt.Errorf("stop Process.Execute: %w", err))
		}
	}
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result = errors.Join(result, fixture.process.Close(ctx))
		cancel()
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		result = errors.Join(result, fmt.Errorf("API listener starts = %d, want 1", got))
	}
	if fixture.rootDir != "" {
		result = errors.Join(result, os.RemoveAll(fixture.rootDir))
	}
	return result
}

type runtimeAPIProcessCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu          sync.Mutex
	terminalErr error
}

func startRuntimeAPIProcessCommand(process support.ApplicationProcess, input root.Input) *runtimeAPIProcessCommand {
	ctx, cancel := context.WithCancel(input.Context)
	input.Context = ctx
	command := &runtimeAPIProcessCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.terminalErr = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func (command *runtimeAPIProcessCommand) stop() {
	if command == nil {
		return
	}
	command.cancel()
	select {
	case <-command.done:
	case <-time.After(5 * time.Second):
	}
}

func (command *runtimeAPIProcessCommand) terminalError() error {
	if command == nil {
		return nil
	}
	command.mu.Lock()
	defer command.mu.Unlock()
	return command.terminalErr
}

type runtimeAPIProviderRouter struct {
	mu     sync.RWMutex
	routes map[string]runtimeAPIProviderRoute
	models map[string]string
}

type runtimeAPIProviderRoute struct {
	factoryDir string
	models     []string
	provider   providers.Service
}

func newRuntimeAPIProviderRouter() *runtimeAPIProviderRouter {
	return &runtimeAPIProviderRouter{routes: make(map[string]runtimeAPIProviderRoute), models: make(map[string]string)}
}

func (router *runtimeAPIProviderRouter) register(factoryDir string, models []string, provider providers.Service) func() {
	key := runtimeAPINormalizeDir(factoryDir)
	route := runtimeAPIProviderRoute{factoryDir: key, models: append([]string(nil), models...), provider: provider}
	router.mu.Lock()
	router.routes[key] = route
	for _, model := range models {
		router.models[strings.ToLower(strings.TrimSpace(model))] = key
	}
	router.mu.Unlock()
	return func() {
		router.mu.Lock()
		if current, ok := router.routes[key]; ok && current.provider == provider {
			delete(router.routes, key)
			for _, model := range models {
				modelKey := strings.ToLower(strings.TrimSpace(model))
				if router.models[modelKey] == key {
					delete(router.models, modelKey)
				}
			}
		}
		router.mu.Unlock()
	}
}

func (router *runtimeAPIProviderRouter) providerFor(request providers.ExecuteRequest) providers.Service {
	factoryDir := runtimeAPINormalizeDir(request.FactoryDirectory)
	model := strings.ToLower(strings.TrimSpace(request.Model))
	router.mu.RLock()
	defer router.mu.RUnlock()
	if route, ok := router.routes[factoryDir]; ok {
		return route.provider
	}
	for _, route := range router.routes {
		if runtimeAPIDirContains(route.factoryDir, factoryDir) || runtimeAPIDirContains(route.factoryDir, runtimeAPINormalizeDir(request.WorkingDirectory)) {
			return route.provider
		}
	}
	if key := router.models[model]; key != "" {
		return router.routes[key].provider
	}
	return nil
}

func (router *runtimeAPIProviderRouter) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	provider := router.providerFor(request)
	if provider == nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindMisconfigured,
			Message: "no shared runtime API provider route for factory directory",
		}
	}
	return provider.Execute(ctx, request)
}

func (router *runtimeAPIProviderRouter) ListProviders(ctx context.Context, request providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return (testutil.NativeProvider{}).ListProviders(ctx, request)
}

func (router *runtimeAPIProviderRouter) GetProvider(ctx context.Context, request providers.GetProviderRequest) (providers.GetProviderResult, error) {
	return (testutil.NativeProvider{}).GetProvider(ctx, request)
}

func (router *runtimeAPIProviderRouter) ResolveIdentity(ctx context.Context, request providers.ResolveIdentityRequest) (providers.ResolveIdentityResult, error) {
	return (testutil.NativeProvider{}).ResolveIdentity(ctx, request)
}

func (router *runtimeAPIProviderRouter) ResolveSelection(ctx context.Context, request providers.ResolveSelectionRequest) (providers.ResolveSelectionResult, error) {
	return (testutil.NativeProvider{}).ResolveSelection(ctx, request)
}

func (router *runtimeAPIProviderRouter) ValidatePrerequisites(ctx context.Context, request providers.ValidatePrerequisitesRequest) error {
	return (testutil.NativeProvider{}).ValidatePrerequisites(ctx, request)
}

func (router *runtimeAPIProviderRouter) ControlAttempt(ctx context.Context, request providers.ControlAttemptRequest) (providers.ControlAttemptResult, error) {
	return (testutil.NativeProvider{}).ControlAttempt(ctx, request)
}

func (router *runtimeAPIProviderRouter) Continue(ctx context.Context, request providers.ContinueRequest) (providers.ContinueResult, error) {
	return (testutil.NativeProvider{}).Continue(ctx, request)
}

func (router *runtimeAPIProviderRouter) ContinueReference(ctx context.Context, request providers.ContinueReferenceRequest) (providers.ContinueReferenceResult, error) {
	return (testutil.NativeProvider{}).ContinueReference(ctx, request)
}

type runtimeAPICommandRouter struct {
	name   string
	mu     sync.RWMutex
	routes map[string]platformprocess.CommandRunner
}

func newRuntimeAPICommandRouter(name string) *runtimeAPICommandRouter {
	return &runtimeAPICommandRouter{name: name, routes: make(map[string]platformprocess.CommandRunner)}
}

func (router *runtimeAPICommandRouter) register(factoryDir string, runner platformprocess.CommandRunner) func() {
	key := runtimeAPINormalizeDir(factoryDir)
	router.mu.Lock()
	router.routes[key] = runner
	router.mu.Unlock()
	return func() {
		router.mu.Lock()
		if current, ok := router.routes[key]; ok && current == runner {
			delete(router.routes, key)
		}
		router.mu.Unlock()
	}
}

func (router *runtimeAPICommandRouter) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	key := runtimeAPINormalizeDir(request.WorkDir)
	router.mu.RLock()
	runner := router.routes[key]
	if runner == nil {
		for routeDir, candidate := range router.routes {
			if runtimeAPIDirContains(routeDir, key) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("no shared runtime API %s command route for %q", router.name, request.WorkDir)
	}
	return runner.Run(ctx, request)
}

type runtimeAPICommandProvider struct {
	testutil.NativeProvider
	runner platformprocess.CommandRunner
}

func newRuntimeAPICommandProvider(runner platformprocess.CommandRunner) providers.Service {
	provider := &runtimeAPICommandProvider{runner: runner}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (provider *runtimeAPICommandProvider) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	command := strings.TrimSpace(request.Command)
	if command == "" {
		command = request.Provider.CanonicalSessionProvider()
	}
	commandRequest := platformprocess.CommandRequest{
		Command:                  command,
		Args:                     append([]string(nil), request.Args...),
		Stdin:                    []byte(request.UserMessage),
		Env:                      append([]string(nil), request.ProcessEnvironment...),
		WorkDir:                  request.WorkingDirectory,
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	}
	result, err := provider.runner.Run(ctx, commandRequest)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	if failure := runtimeAPICommandFailure(result); failure != nil {
		return providers.ExecuteResult{}, failure
	}
	return providers.ExecuteResult{Content: runtimeAPICommandContent(command, result.Stdout)}, nil
}

func runtimeAPICommandFailure(result platformprocess.CommandResult) error {
	if result.ExitCode == 0 && len(result.Stderr) == 0 {
		return nil
	}
	message := strings.TrimSpace(string(result.Stderr))
	lower := strings.ToLower(message)
	kind := providers.ExecuteFailureKindUnknown
	switch {
	case strings.Contains(lower, "rate_limit"), strings.Contains(lower, "429"), strings.Contains(lower, "thrott"):
		kind = providers.ExecuteFailureKindThrottled
	case strings.Contains(lower, "authentication"), strings.Contains(lower, "401"):
		kind = providers.ExecuteFailureKindAuthentication
	case strings.Contains(lower, "invalid"):
		kind = providers.ExecuteFailureKindInvalidRequest
	}
	if message == "" {
		message = "provider command failed"
	}
	return providers.ExecuteFailure{Kind: kind, Message: message}
}

func runtimeAPICommandContent(command string, stdout []byte) string {
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed == "" {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) != nil {
			continue
		}
		typeName, _ := record["type"].(string)
		switch strings.ToLower(strings.TrimSpace(command)) {
		case "codex":
			if typeName != "item.completed" {
				continue
			}
			item, _ := record["item"].(map[string]any)
			text, _ := item["text"].(string)
			if text != "" {
				return text
			}
		case "claude":
			if typeName != "result" {
				continue
			}
			text, _ := record["result"].(string)
			if text != "" {
				return text
			}
		}
	}
	return trimmed
}

func runtimeAPINormalizeDir(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func runtimeAPIDirContains(parent, child string) bool {
	parent = runtimeAPINormalizeDir(parent)
	child = runtimeAPINormalizeDir(child)
	if parent == "" || child == "" {
		return false
	}
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)))
}

var _ providers.Service = (*runtimeAPIProviderRouter)(nil)
var _ platformprocess.CommandRunner = (*runtimeAPICommandRouter)(nil)
var _ providers.Service = (*runtimeAPICommandProvider)(nil)
