package factory_builder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

const factoryBuilderExpectedSessions = 6

// factoryBuilderSharedFixture owns one root-built process and one continuous
// API host for all six Factory Builder scenarios. The ordinary cases execute
// through the local public CLI before the host is activated; the host then
// exercises the six private scenario sessions and their cleanup contract.
type factoryBuilderSharedFixture struct {
	rootDir    string
	process    support.ApplicationProcess
	provider   *factoryBuilderProviderCommandRouter
	api        *support.ProcessAPIServer
	baseURL    string
	factoryDir string
	lifecycle  *factoryBuilderLifecycleLedger
	scenarios  []*factoryBuilderScenario
	requestIDs struct {
		sync.Mutex
		values map[string]struct{}
	}
}

type factoryBuilderLifecycleResource struct {
	sessionID     string
	rootDir       string
	factoryDir    string
	closed        bool
	sessionAbsent bool
	rootRemoved   bool
}

type factoryBuilderLifecycleLedger struct {
	mu            sync.Mutex
	expected      int
	processStarts int
	resources     []factoryBuilderLifecycleResource
}

func newFactoryBuilderLifecycleLedger(expected int) *factoryBuilderLifecycleLedger {
	return &factoryBuilderLifecycleLedger{expected: expected}
}

func (ledger *factoryBuilderLifecycleLedger) recordProcessStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStarts++
}

func (ledger *factoryBuilderLifecycleLedger) register(
	sessionID, rootDir, factoryDir string,
) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("Factory Session ID is empty")
	}
	if sessionID == factorysessions.DefaultSessionID {
		return fmt.Errorf("Factory Session ID is the default session %q", sessionID)
	}
	for _, resource := range ledger.resources {
		if resource.sessionID == sessionID {
			return fmt.Errorf("Factory Session ID %q is not unique", sessionID)
		}
	}
	for label, path := range map[string]string{"scenario root": rootDir, "Factory": factoryDir} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("%s path %q is not absolute", label, path)
		}
	}
	ledger.resources = append(ledger.resources, factoryBuilderLifecycleResource{
		sessionID:  sessionID,
		rootDir:    rootDir,
		factoryDir: factoryDir,
	})
	return nil
}

func (ledger *factoryBuilderLifecycleLedger) close(
	sessionID string,
	sessionAbsent, rootRemoved bool,
) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	for index := range ledger.resources {
		resource := &ledger.resources[index]
		if resource.sessionID != sessionID {
			continue
		}
		if resource.closed {
			return fmt.Errorf("Factory Session %q was closed more than once", sessionID)
		}
		resource.closed = true
		resource.sessionAbsent = sessionAbsent
		resource.rootRemoved = rootRemoved
		return nil
	}
	return fmt.Errorf("Factory Session %q was not registered", sessionID)
}

func (ledger *factoryBuilderLifecycleLedger) assertClean(t testing.TB, runtimeArtifacts, isolatedRows int) {
	t.Helper()
	ledger.mu.Lock()
	processStarts := ledger.processStarts
	resources := append([]factoryBuilderLifecycleResource(nil), ledger.resources...)
	expected := ledger.expected
	ledger.mu.Unlock()

	if processStarts != 1 {
		t.Errorf("FACTORY-BUILDER-SPINE-001 process starts = %d, want 1 observed API host start", processStarts)
	}
	if len(resources) != expected {
		t.Errorf("FACTORY-BUILDER-SPINE-001 explicit sessions opened = %d, want %d", len(resources), expected)
	}
	sessions := make(map[string]struct{}, len(resources))
	roots := make(map[string]struct{}, len(resources))
	factories := make(map[string]struct{}, len(resources))
	closed := 0
	for _, resource := range resources {
		if _, exists := sessions[resource.sessionID]; exists {
			t.Errorf("Factory Session %q is not unique", resource.sessionID)
		}
		sessions[resource.sessionID] = struct{}{}
		if _, exists := roots[resource.rootDir]; exists {
			t.Errorf("scenario root %q is not unique", resource.rootDir)
		}
		roots[resource.rootDir] = struct{}{}
		if _, exists := factories[resource.factoryDir]; exists {
			t.Errorf("Factory definition %q is not unique", resource.factoryDir)
		}
		factories[resource.factoryDir] = struct{}{}
		if resource.closed {
			closed++
		} else {
			t.Errorf("Factory Session %q remains open", resource.sessionID)
		}
		if !resource.sessionAbsent {
			t.Errorf("Factory Session %q remained publicly readable after close", resource.sessionID)
		}
		if !resource.rootRemoved {
			t.Errorf("scenario root %q remains after cleanup", resource.rootDir)
		}
	}
	if closed != len(resources) {
		t.Errorf("explicit sessions closed = %d, want %d", closed, len(resources))
	}
	t.Logf(
		"Factory Builder lifecycle: process_starts=%d explicit_sessions_opened=%d explicit_sessions_closed=%d unique_session_ids=%d scenario_roots_removed=%d runtime_artifacts=%d isolated_rows=%d",
		processStarts, len(resources), closed, len(sessions), len(roots), runtimeArtifacts, isolatedRows,
	)
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
	lifecycle := newFactoryBuilderLifecycleLedger(factoryBuilderExpectedSessions)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			lifecycle.recordProcessStart()
			return api.Start(ctx, request)
		},
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(Factory Builder): %v", err)
	}
	fixture := &factoryBuilderSharedFixture{
		rootDir:   rootDir,
		process:   process,
		provider:  provider,
		api:       api,
		lifecycle: lifecycle,
	}
	// The process is closed after any later host/session cleanup callbacks. The
	// authoritative persisted-session probe is registered when the host starts,
	// after the host's own stop callback and before session close callbacks.
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
	support.StartProcessCommand(t, fixture.process, inputs.Input)
	fixture.baseURL = fixture.api.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, factoryBuilderSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
}

func (fixture *factoryBuilderSharedFixture) openAllScenarios(t *testing.T) {
	t.Helper()
	for _, scenario := range fixture.scenarios {
		scenario.open(t)
	}
}

func (fixture *factoryBuilderSharedFixture) closeAllScenarios(t testing.TB) {
	t.Helper()
	for index := len(fixture.scenarios) - 1; index >= 0; index-- {
		fixture.scenarios[index].close(t)
	}
}

func (fixture *factoryBuilderSharedFixture) assertDurableSessionsClean(t testing.TB) {
	t.Helper()
	endpoint := strings.TrimSuffix(fixture.baseURL, "/") + "/factory-sessions?scope=persisted"
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(endpoint)
	if err != nil {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 list persisted Factory Sessions: %v", err)
		fixture.lifecycle.assertClean(t, 0, -1)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 list persisted Factory Sessions status = %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
		fixture.lifecycle.assertClean(t, 0, -1)
		return
	}
	var listing factoryapi.ListFactorySessionsResponse
	if err := json.NewDecoder(response.Body).Decode(&listing); err != nil {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 decode persisted Factory Sessions: %v", err)
		fixture.lifecycle.assertClean(t, 0, -1)
		return
	}
	if listing.Scope == nil || *listing.Scope != factoryapi.FactorySessionListScopePersisted {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 persisted Factory Session list scope = %#v, want persisted", listing.Scope)
	}
	isolatedRows := 0
	runtimeArtifacts := 0
	if listing.DurableSessions != nil {
		isolatedRows = len(*listing.DurableSessions)
		for _, session := range *listing.DurableSessions {
			if session.ArtifactCount != nil {
				runtimeArtifacts += *session.ArtifactCount
			}
		}
	}
	if isolatedRows != 0 {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 persisted Factory Session rows = %d, want 0", isolatedRows)
	}
	if runtimeArtifacts != 0 {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 persisted runtime artifacts = %d, want 0", runtimeArtifacts)
	}
	fixture.lifecycle.assertClean(t, runtimeArtifacts, isolatedRows)
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

func (fixture *factoryBuilderSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *factoryBuilderScenario {
	t.Helper()
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
	selectorPaths := []string{factoryDir, workingDirectory, fixture.factoryDir, filepath.Join(homeDir, ".you-agent-factory", "factories")}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	scenario := &factoryBuilderScenario{
		fixture:          fixture,
		rootDir:          rootDir,
		homeDir:          homeDir,
		factoryDir:       factoryDir,
		operatorRoot:     filepath.Join(homeDir, ".you-agent-factory", "factories"),
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
	fixture.scenarios = append(fixture.scenarios, scenario)
	return scenario
}

func (scenario *factoryBuilderScenario) open(t *testing.T) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	scenario.sessionID = opened.Session.Id
	if scenario.sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened Factory Builder Factory Session returned default session %q", scenario.sessionID)
	}
	if err := scenario.fixture.lifecycle.register(
		scenario.sessionID, scenario.rootDir, scenario.factoryDir,
	); err != nil {
		t.Fatalf("register Factory Builder scenario lifecycle: %v", err)
	}
}

func (scenario *factoryBuilderScenario) close(t testing.TB) {
	t.Helper()
	if scenario == nil || scenario.sessionID == "" {
		return
	}
	support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	assertFactoryBuilderSessionAbsent(t, scenario.fixture.baseURL, scenario.sessionID)
	sessionAbsent := true
	rootRemoved := false
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 remove scenario root %q: %v", scenario.rootDir, err)
	} else if _, err := os.Stat(scenario.rootDir); !os.IsNotExist(err) {
		t.Errorf("FACTORY-BUILDER-CLEANUP-001 scenario root %q remains: %v", scenario.rootDir, err)
	} else {
		rootRemoved = true
	}
	if err := scenario.fixture.lifecycle.close(scenario.sessionID, sessionAbsent, rootRemoved); err != nil {
		t.Errorf("record Factory Builder scenario cleanup: %v", err)
	}
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
