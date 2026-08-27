package fusion

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

const fusionSharedFixtureTimeout = 15 * time.Second

const fusionExpectedSessions = 3

// fusionSharedFixture owns one root-built process and one continuous API host
// for the package's compatible scenarios. Each child copies the packaged
// Factory into a private home and opens a non-default Factory Session.
type fusionSharedFixture struct {
	parent      testing.TB
	rootDir     string
	process     support.ApplicationProcess
	provider    *fusionProviderCommandRouter
	api         *support.ProcessAPIServer
	baseURL     string
	factoryDir  string
	environment []string
	lifecycle   *fusionLifecycleLedger

	hostMu      sync.Mutex
	hostStarted bool
}

type fusionLifecycleResource struct {
	sessionID     string
	rootDir       string
	factoryDir    string
	closed        bool
	sessionAbsent bool
	rootRemoved   bool
}

type fusionLifecycleLedger struct {
	mu            sync.Mutex
	expected      int
	processStarts int
	resources     []fusionLifecycleResource
}

func newFusionLifecycleLedger(expected int) *fusionLifecycleLedger {
	return &fusionLifecycleLedger{expected: expected}
}

func (ledger *fusionLifecycleLedger) recordProcessStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStarts++
}

func (ledger *fusionLifecycleLedger) register(
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
	ledger.resources = append(ledger.resources, fusionLifecycleResource{
		sessionID:  sessionID,
		rootDir:    rootDir,
		factoryDir: factoryDir,
	})
	return nil
}

func (ledger *fusionLifecycleLedger) close(
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

func (ledger *fusionLifecycleLedger) assertClean(t testing.TB) {
	t.Helper()
	ledger.mu.Lock()
	processStarts := ledger.processStarts
	resources := append([]fusionLifecycleResource(nil), ledger.resources...)
	expected := ledger.expected
	ledger.mu.Unlock()

	if processStarts != 1 {
		t.Errorf("FUSION-SPINE-001 process starts = %d, want 1 observed API host start", processStarts)
	}
	if len(resources) != expected {
		t.Errorf("FUSION-SPINE-001 explicit sessions opened = %d, want %d", len(resources), expected)
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
		"fusion lifecycle: process_starts=%d explicit_sessions_opened=%d explicit_sessions_closed=%d unique_session_ids=%d scenario_roots_removed=%d runtime_artifacts=0 isolated_rows=0",
		processStarts, len(resources), closed, len(sessions), len(roots),
	)
}

// fusionProviderCommandRouter keeps the process edge immutable while
// selecting the scenario-owned command runner by the request's Factory path.
// The path selector also covers child work directories below that Factory.
type fusionProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newFusionProviderCommandRouter() *fusionProviderCommandRouter {
	return &fusionProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *fusionProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[fusionPathKey(path)] = runner
	}
}

func (router *fusionProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, fusionPathKey(path))
	}
}

func (router *fusionProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := fusionPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if fusionPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"fusion provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func fusionPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func fusionPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type fusionScenario struct {
	fixture          *fusionSharedFixture
	rootDir          string
	factoryDir       string
	environment      []string
	workingDirectory string
	selectorPaths    []string
	sessionID        string
}

func newFusionSharedFixture(t *testing.T) *fusionSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared fusion home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared fusion working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newFusionProviderCommandRouter()
	lifecycle := newFusionLifecycleLedger(fusionExpectedSessions)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			lifecycle.recordProcessStart()
			return api.Start(ctx, request)
		},
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(fusion): %v", err)
	}
	environment := fusionCustomerEnvironment(homeDir)
	fixture := &fusionSharedFixture{
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
	fixture.factoryDir = support.InstallPackagedFactoryWithProcess(
		t,
		process,
		environment,
		workingDirectory,
		factorydefinitions.PackagedFusionFactoryName,
	)
	return fixture
}

func (fixture *fusionSharedFixture) startHost(t testing.TB) {
	t.Helper()
	fixture.hostMu.Lock()
	if fixture.hostStarted {
		fixture.hostMu.Unlock()
		return
	}
	fixture.hostStarted = true
	fixture.hostMu.Unlock()

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", fixture.factoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--provider", "CODEX", "--model", "gpt-5",
	})
	inputs.Input.Env = fixture.environment
	inputs.Input.WorkingDirectory = fixture.factoryDir
	support.StartProcessCommand(fixture.parent, fixture.process, inputs.Input)
	baseURL := fixture.api.WaitForURL(t)
	fixture.hostMu.Lock()
	fixture.baseURL = baseURL
	fixture.hostMu.Unlock()
	support.WaitForStatus(t, baseURL, fusionSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
}

func (fixture *fusionSharedFixture) cleanup(t testing.TB) {
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
			t.Errorf("FUSION-CLEANUP-001 listener still served /status after process close: %s", strings.TrimSpace(string(body)))
		}
	}
	if err := os.RemoveAll(rootDir); err != nil {
		t.Errorf("FUSION-CLEANUP-001 remove shared root %q: %v", rootDir, err)
	} else if _, err := os.Stat(rootDir); !os.IsNotExist(err) {
		t.Errorf("FUSION-CLEANUP-001 shared root %q remains after process close: %v", rootDir, err)
	}
}

func fusionCustomerEnvironment(homeDir string) []string {
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

func (fixture *fusionSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *fusionScenario {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create fusion scenario home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create fusion scenario working directory: %v", err)
	}
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.factoryDir,
		homeDir,
		factorydefinitions.PackagedFusionFactoryName,
	)
	environment := fusionCustomerEnvironment(homeDir)
	selectorPaths := []string{factoryDir, workingDirectory}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	return &fusionScenario{
		fixture:          fixture,
		rootDir:          rootDir,
		factoryDir:       factoryDir,
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
}

func (scenario *fusionScenario) open(t *testing.T) {
	t.Helper()
	scenario.fixture.startHost(t)
	opened := support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	scenario.sessionID = opened.Session.Id
	if scenario.sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened fusion Factory Session returned default session %q", scenario.sessionID)
	}
	t.Cleanup(func() {
		scenario.close(t)
	})
	if err := scenario.fixture.lifecycle.register(
		scenario.sessionID, scenario.rootDir, scenario.factoryDir,
	); err != nil {
		t.Fatalf("register fusion scenario lifecycle: %v", err)
	}
}

func (scenario *fusionScenario) close(t testing.TB) {
	t.Helper()
	if scenario == nil || scenario.sessionID == "" {
		return
	}
	support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	assertFusionSessionAbsent(t, scenario.fixture.baseURL, scenario.sessionID)
	sessionAbsent := true
	rootRemoved := false
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		t.Errorf("FUSION-CLEANUP-001 remove scenario root %q: %v", scenario.rootDir, err)
	} else if _, err := os.Stat(scenario.rootDir); !os.IsNotExist(err) {
		t.Errorf("FUSION-CLEANUP-001 scenario root %q remains: %v", scenario.rootDir, err)
	} else {
		rootRemoved = true
	}
	if err := scenario.fixture.lifecycle.close(scenario.sessionID, sessionAbsent, rootRemoved); err != nil {
		t.Errorf("record fusion scenario cleanup: %v", err)
	}
}

func assertFusionSessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted fusion Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET deleted fusion Factory Session %q status = %d, want 404: %s",
			sessionID, response.StatusCode, strings.TrimSpace(string(payload)),
		)
	}
}

var _ platformprocess.CommandRunner = (*fusionProviderCommandRouter)(nil)
