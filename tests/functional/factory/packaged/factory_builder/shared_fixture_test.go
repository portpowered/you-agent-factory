package factory_builder

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const factoryBuilderSharedFixtureTimeout = 15 * time.Second

// factoryBuilderSharedFixture owns one root-built process and one continuous
// API host for all six Factory Builder scenarios. Each child invokes its
// behavior through one private explicit Factory Session on that host.
type factoryBuilderSharedFixture struct {
	rootDir                 string
	process                 support.ApplicationProcess
	provider                *factoryBuilderProviderCommandRouter
	api                     *support.ProcessAPIServer
	host                    *support.ProcessCommand
	baseURL                 string
	factoryDir              string
	requestID               atomic.Uint64
	scenariosMu             sync.Mutex
	scenarios               []*factoryBuilderScenario
	invalidExistingScenario *factoryBuilderScenario
	requestIDs              struct {
		sync.Mutex
		values map[string]struct{}
	}
}

// factoryBuilderProviderCommandRouter keeps process wiring immutable while
// selecting each scenario's synchronized command runner by its owned path.
type factoryBuilderProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newFactoryBuilderProviderCommandRouter() *factoryBuilderProviderCommandRouter {
	return &factoryBuilderProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *factoryBuilderProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[factoryBuilderPathKey(path)] = runner
	}
}

func (router *factoryBuilderProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, factoryBuilderPathKey(path))
	}
}

func (router *factoryBuilderProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := factoryBuilderPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if factoryBuilderPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"Factory Builder provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func factoryBuilderPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func factoryBuilderPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type factoryBuilderScenario struct {
	fixture          *factoryBuilderSharedFixture
	rootDir          string
	homeDir          string
	factoryDir       string
	operatorRoot     string
	environment      []string
	workingDirectory string
	selectorPaths    []string
	sessionID        string
	sessionClosed    bool
	rootRemoved      bool
	runner           *factoryBuilderCommandRunner
	installedKind    string
	installedPath    string
	existingBefore   []byte
}

func newFactoryBuilderSharedFixture(t *testing.T) *factoryBuilderSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared Factory Builder home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared Factory Builder working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newFactoryBuilderProviderCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return api.Start(ctx, request)
		},
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(Factory Builder): %v", err)
	}
	fixture := &factoryBuilderSharedFixture{
		rootDir:  rootDir,
		process:  process,
		provider: provider,
		api:      api,
	}
	// Register fixture cleanup before the process and host callbacks below. The
	// normal path performs session closure, the persisted-session probe, host
	// shutdown, installed-artifact checks, and root removal explicitly; this
	// callback remains the final filesystem/listener safety net.
	t.Cleanup(func() { fixture.cleanup(t) })
	support.CleanupProcess(t, process)

	environment := factoryBuilderCustomerEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		environment,
		workingDirectory,
		factoryBuilderName,
	)
	fixture.factoryDir = factoryDir
	return fixture
}

func (fixture *factoryBuilderSharedFixture) startServer(t *testing.T) {
	t.Helper()
	if fixture.api == nil {
		t.Fatal("Factory Builder API server is not configured")
	}
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", fixture.factoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--provider", "CODEX", "--model", "gpt-5",
	})
	inputs.Input.Env = factoryBuilderCustomerEnvironment(filepath.Join(fixture.rootDir, "home"))
	inputs.Input.WorkingDirectory = fixture.factoryDir
	fixture.host = support.StartProcessCommand(t, fixture.process, inputs.Input)
	fixture.baseURL = fixture.api.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, factoryBuilderSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
}

func (fixture *factoryBuilderSharedFixture) cleanup(t testing.TB) {
	t.Helper()
	if fixture.baseURL != "" {
		// This is a single bounded shutdown probe, not synchronization: after the
		// reusable process closes, its injected listener must reject /status.
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Errorf("FACTORY-BUILDER-CLEANUP-001 listener still served /status after process close: %s", strings.TrimSpace(string(body)))
		}
	}
	for _, scenario := range fixture.scenarioSnapshot() {
		scenario.close(t)
	}
	for _, scenario := range fixture.scenarioSnapshot() {
		fixture.provider.unregister(scenario.selectorPaths)
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 remove shared root %q: %v", fixture.rootDir, err)
	} else if _, err := os.Stat(fixture.rootDir); !os.IsNotExist(err) {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 shared root %q remains after process close: %v", fixture.rootDir, err)
	}
}

func factoryBuilderCustomerEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		switch {
		case strings.EqualFold(name, "HOME"), strings.EqualFold(name, "USERPROFILE"),
			strings.EqualFold(name, "YOU_DEFAULT_WORKER_MODEL_PROVIDER"), strings.EqualFold(name, "YOU_DEFAULT_WORKER_MODEL"):
			continue
		default:
			environment = append(environment, entry)
		}
	}
	return append(
		environment,
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER=CODEX",
		"YOU_DEFAULT_WORKER_MODEL=gpt-5",
	)
}

func (fixture *factoryBuilderSharedFixture) recordInvocationRequestID(requestID string) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return fmt.Errorf("invocation request ID is empty")
	}
	fixture.requestIDs.Lock()
	defer fixture.requestIDs.Unlock()
	if fixture.requestIDs.values == nil {
		fixture.requestIDs.values = make(map[string]struct{})
	}
	if _, exists := fixture.requestIDs.values[requestID]; exists {
		return fmt.Errorf("invocation request ID %q is not unique", requestID)
	}
	fixture.requestIDs.values[requestID] = struct{}{}
	return nil
}

func (fixture *factoryBuilderSharedFixture) nextRequestID() uint64 {
	return fixture.requestID.Add(1)
}

func (fixture *factoryBuilderSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *factoryBuilderScenario {
	t.Helper()
	builderRunner, _ := runner.(*factoryBuilderCommandRunner)
	rootDir, err := os.MkdirTemp(fixture.rootDir, "scenario-")
	if err != nil {
		t.Fatalf("create Factory Builder scenario root: %v", err)
	}
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	for label, path := range map[string]string{"home": homeDir, "working directory": workingDirectory} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create Factory Builder scenario %s: %v", label, err)
		}
	}
	factoryDir := support.CopyFactoryAsNamed(t, fixture.factoryDir, homeDir, factoryBuilderName)
	environment := factoryBuilderCustomerEnvironment(homeDir)
	selectorPaths := []string{factoryDir, workingDirectory, filepath.Join(homeDir, ".you-agent-factory", "factories")}
	fixture.provider.register(selectorPaths, runner)
	scenario := &factoryBuilderScenario{
		fixture:          fixture,
		rootDir:          rootDir,
		homeDir:          homeDir,
		factoryDir:       factoryDir,
		operatorRoot:     filepath.Join(homeDir, ".you-agent-factory", "factories"),
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
		runner:           builderRunner,
	}
	fixture.scenariosMu.Lock()
	fixture.scenarios = append(fixture.scenarios, scenario)
	fixture.scenariosMu.Unlock()
	return scenario
}

func (fixture *factoryBuilderSharedFixture) scenarioSnapshot() []*factoryBuilderScenario {
	fixture.scenariosMu.Lock()
	defer fixture.scenariosMu.Unlock()
	return append([]*factoryBuilderScenario(nil), fixture.scenarios...)
}

func (scenario *factoryBuilderScenario) open(t *testing.T) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	scenario.sessionID = opened.Session.Id
	if scenario.sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened Factory Builder Factory Session returned default session %q", scenario.sessionID)
	}
}

func (fixture *factoryBuilderSharedFixture) closeAllScenarios(t testing.TB) {
	t.Helper()
	for _, scenario := range fixture.scenarioSnapshot() {
		scenario.closeSession(t)
	}
}

func (fixture *factoryBuilderSharedFixture) stopServer(t testing.TB) {
	t.Helper()
	if fixture.host != nil {
		fixture.host.Stop(t)
	}
}

func (fixture *factoryBuilderSharedFixture) removeAllScenarioRoots(t testing.TB) {
	t.Helper()
	for _, scenario := range fixture.scenarioSnapshot() {
		scenario.removeRoot(t)
	}
}

func (scenario *factoryBuilderScenario) closeSession(t testing.TB) {
	t.Helper()
	if scenario == nil || scenario.sessionID == "" || scenario.sessionClosed {
		return
	}
	support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	assertFactoryBuilderSessionAbsent(t, scenario.fixture.baseURL, scenario.sessionID)
	scenario.sessionClosed = true
}

func (scenario *factoryBuilderScenario) removeRoot(t testing.TB) {
	t.Helper()
	if scenario == nil || scenario.rootRemoved {
		return
	}
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 remove scenario root %q: %v", scenario.rootDir, err)
	} else if _, err := os.Stat(scenario.rootDir); !os.IsNotExist(err) {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 scenario root %q remains: %v", scenario.rootDir, err)
	} else {
		scenario.rootRemoved = true
	}
}

func (scenario *factoryBuilderScenario) close(t testing.TB) {
	t.Helper()
	scenario.closeSession(t)
	scenario.removeRoot(t)
}

func assertFactoryBuilderSessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted Factory Builder Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET deleted Factory Builder Factory Session %q status = %d, want 404: %s",
			sessionID, response.StatusCode, strings.TrimSpace(string(payload)),
		)
	}
}

var _ platformprocess.CommandRunner = (*factoryBuilderProviderCommandRouter)(nil)
