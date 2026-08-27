package javascript_families

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
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const javascriptFamiliesSharedFixtureTimeout = 15 * time.Second

const javascriptFamiliesExpectedSessions = 4

// javascriptFamiliesSharedFixture owns one root-built process and one
// continuous API host for the tournament and spawn scenarios. Each child
// copies its packaged Factory into a private home and opens a non-default
// Factory Session.
type javascriptFamiliesSharedFixture struct {
	parent      testing.TB
	rootDir     string
	process     support.ApplicationProcess
	provider    *javascriptFamiliesProviderCommandRouter
	api         *support.ProcessAPIServer
	baseURL     string
	factoryDirs map[string]string
	environment []string
	lifecycle   *javascriptFamiliesLifecycleLedger

	hostMu      sync.Mutex
	hostStarted bool
}

type javascriptFamiliesLifecycleResource struct {
	sessionID     string
	rootDir       string
	factoryDir    string
	closed        bool
	sessionAbsent bool
	rootRemoved   bool
}

type javascriptFamiliesLifecycleLedger struct {
	mu            sync.Mutex
	expected      int
	processStarts int
	resources     []javascriptFamiliesLifecycleResource
}

func newJavascriptFamiliesLifecycleLedger(expected int) *javascriptFamiliesLifecycleLedger {
	return &javascriptFamiliesLifecycleLedger{expected: expected}
}

func (ledger *javascriptFamiliesLifecycleLedger) recordProcessStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStarts++
}

func (ledger *javascriptFamiliesLifecycleLedger) register(
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
	ledger.resources = append(ledger.resources, javascriptFamiliesLifecycleResource{
		sessionID:  sessionID,
		rootDir:    rootDir,
		factoryDir: factoryDir,
	})
	return nil
}

func (ledger *javascriptFamiliesLifecycleLedger) close(
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

func (ledger *javascriptFamiliesLifecycleLedger) assertClean(t testing.TB) {
	t.Helper()
	ledger.mu.Lock()
	processStarts := ledger.processStarts
	resources := append([]javascriptFamiliesLifecycleResource(nil), ledger.resources...)
	expected := ledger.expected
	ledger.mu.Unlock()

	if processStarts != 1 {
		t.Errorf("JAVASCRIPT-FAMILIES-SPINE-001 process starts = %d, want 1 observed API host start", processStarts)
	}
	if len(resources) != expected {
		t.Errorf("JAVASCRIPT-FAMILIES-SPINE-001 explicit sessions opened = %d, want %d", len(resources), expected)
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
		"JavaScript families lifecycle: process_starts=%d explicit_sessions_opened=%d explicit_sessions_closed=%d unique_session_ids=%d scenario_roots_removed=%d runtime_artifacts=0 isolated_rows=0",
		processStarts, len(resources), closed, len(sessions), len(roots),
	)
}

// javascriptFamiliesProviderCommandRouter keeps the process edge immutable
// while selecting the scenario-owned command runner by Factory path. The path
// selector also covers child work directories below that Factory.
type javascriptFamiliesProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newJavascriptFamiliesProviderCommandRouter() *javascriptFamiliesProviderCommandRouter {
	return &javascriptFamiliesProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *javascriptFamiliesProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[javascriptFamiliesPathKey(path)] = runner
	}
}

func (router *javascriptFamiliesProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, javascriptFamiliesPathKey(path))
	}
}

func (router *javascriptFamiliesProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := javascriptFamiliesPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if javascriptFamiliesPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"JavaScript families provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func javascriptFamiliesPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func javascriptFamiliesPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type javascriptFamiliesScenario struct {
	fixture          *javascriptFamiliesSharedFixture
	rootDir          string
	factoryName      string
	factoryDir       string
	environment      []string
	workingDirectory string
	selectorPaths    []string
	sessionID        string
}

func newJavascriptFamiliesSharedFixture(t *testing.T) *javascriptFamiliesSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared JavaScript families home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared JavaScript families working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newJavascriptFamiliesProviderCommandRouter()
	lifecycle := newJavascriptFamiliesLifecycleLedger(javascriptFamiliesExpectedSessions)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			lifecycle.recordProcessStart()
			return api.Start(ctx, request)
		},
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(JavaScript families): %v", err)
	}
	environment := javascriptFamiliesCustomerEnvironment(homeDir)
	fixture := &javascriptFamiliesSharedFixture{
		parent:      t,
		rootDir:     rootDir,
		process:     process,
		provider:    provider,
		api:         api,
		environment: environment,
		lifecycle:   lifecycle,
	}
	// Register the post-process probe before CleanupProcess so Go's LIFO cleanup
	// order stops the hosted command, closes the reusable process, and only then
	// verifies the listener and test-owned roots are gone.
	t.Cleanup(func() { fixture.cleanup(t) })
	support.CleanupProcess(t, process)
	factoryDirs := make(map[string]string, 2)
	for _, factoryName := range []string{
		factorydefinitions.PackagedTournamentFactoryName,
		factorydefinitions.PackagedSpawnFactoryName,
	} {
		factoryDirs[factoryName] = support.InstallPackagedFactoryWithProcess(
			t,
			process,
			environment,
			workingDirectory,
			factoryName,
		)
	}
	fixture.factoryDirs = factoryDirs
	return fixture
}

func (fixture *javascriptFamiliesSharedFixture) startHost(t testing.TB) {
	t.Helper()
	fixture.hostMu.Lock()
	if fixture.hostStarted {
		fixture.hostMu.Unlock()
		return
	}
	fixture.hostStarted = true
	fixture.hostMu.Unlock()

	baseFactoryDir := fixture.factoryDirs[factorydefinitions.PackagedTournamentFactoryName]
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", baseFactoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--provider", "CODEX", "--model", "operator-js-model",
	})
	inputs.Input.Env = fixture.environment
	inputs.Input.WorkingDirectory = baseFactoryDir
	support.StartProcessCommand(fixture.parent, fixture.process, inputs.Input)
	baseURL := fixture.api.WaitForURL(t)
	fixture.hostMu.Lock()
	fixture.baseURL = baseURL
	fixture.hostMu.Unlock()
	support.WaitForStatus(t, baseURL, javascriptFamiliesSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
}

func (fixture *javascriptFamiliesSharedFixture) cleanup(t testing.TB) {
	t.Helper()
	fixture.lifecycle.assertClean(t)
	fixture.hostMu.Lock()
	baseURL := fixture.baseURL
	rootDir := fixture.rootDir
	fixture.hostMu.Unlock()
	if baseURL != "" {
		// This is a single bounded shutdown probe, not synchronization: after the
		// reusable process closes, its injected listener must reject /status.
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Errorf("JAVASCRIPT-FAMILIES-CLEANUP-001 listener still served /status after process close: %s", strings.TrimSpace(string(body)))
		}
	}
	if err := os.RemoveAll(rootDir); err != nil {
		t.Errorf("JAVASCRIPT-FAMILIES-CLEANUP-001 remove shared root %q: %v", rootDir, err)
	} else if _, err := os.Stat(rootDir); !os.IsNotExist(err) {
		t.Errorf("JAVASCRIPT-FAMILIES-CLEANUP-001 shared root %q remains after process close: %v", rootDir, err)
	}
}

func javascriptFamiliesCustomerEnvironment(homeDir string) []string {
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
		"YOU_DEFAULT_WORKER_MODEL=operator-js-model",
	)
}

func (fixture *javascriptFamiliesSharedFixture) newScenario(
	t *testing.T,
	factoryName string,
	runner platformprocess.CommandRunner,
) *javascriptFamiliesScenario {
	t.Helper()
	sourceDir, ok := fixture.factoryDirs[factoryName]
	if !ok {
		t.Fatalf("JavaScript families Factory %q was not installed in the shared fixture", factoryName)
	}
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create JavaScript families scenario home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create JavaScript families scenario working directory: %v", err)
	}
	factoryDir := support.CopyFactoryAsNamed(t, sourceDir, homeDir, factoryName)
	environment := javascriptFamiliesCustomerEnvironment(homeDir)
	selectorPaths := []string{
		factoryDir,
		workingDirectory,
		fixture.factoryDirs[factorydefinitions.PackagedTournamentFactoryName],
	}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	return &javascriptFamiliesScenario{
		fixture:          fixture,
		rootDir:          rootDir,
		factoryName:      factoryName,
		factoryDir:       factoryDir,
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
}

func (scenario *javascriptFamiliesScenario) open(t *testing.T) {
	t.Helper()
	scenario.fixture.startHost(t)
	opened := support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	scenario.sessionID = opened.Session.Id
	if scenario.sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened JavaScript families Factory Session returned default session %q", scenario.sessionID)
	}
	t.Cleanup(func() {
		scenario.close(t)
	})
	if err := scenario.fixture.lifecycle.register(
		scenario.sessionID, scenario.rootDir, scenario.factoryDir,
	); err != nil {
		t.Fatalf("register JavaScript families scenario lifecycle: %v", err)
	}
}

func (scenario *javascriptFamiliesScenario) close(t testing.TB) {
	t.Helper()
	if scenario == nil || scenario.sessionID == "" {
		return
	}
	support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	assertJavascriptFamiliesSessionAbsent(t, scenario.fixture.baseURL, scenario.sessionID)
	sessionAbsent := true
	rootRemoved := false
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		t.Errorf("JAVASCRIPT-FAMILIES-CLEANUP-001 remove scenario root %q: %v", scenario.rootDir, err)
	} else if _, err := os.Stat(scenario.rootDir); !os.IsNotExist(err) {
		t.Errorf("JAVASCRIPT-FAMILIES-CLEANUP-001 scenario root %q remains: %v", scenario.rootDir, err)
	} else {
		rootRemoved = true
	}
	if err := scenario.fixture.lifecycle.close(scenario.sessionID, sessionAbsent, rootRemoved); err != nil {
		t.Errorf("record JavaScript families scenario cleanup: %v", err)
	}
}

func assertJavascriptFamiliesSessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted JavaScript families Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET deleted JavaScript families Factory Session %q status = %d, want 404: %s",
			sessionID, response.StatusCode, strings.TrimSpace(string(payload)),
		)
	}
}

var _ platformprocess.CommandRunner = (*javascriptFamiliesProviderCommandRouter)(nil)
