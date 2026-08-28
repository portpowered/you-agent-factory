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

const factoryBuilderExpectedSessions = 6

// factoryBuilderSharedFixture owns one root-built process and one continuous
// API host for all six Factory Builder scenarios. Each child copies the
// packaged Factory into a private home and opens a non-default Factory Session.
type factoryBuilderSharedFixture struct {
	rootDir    string
	process    support.ApplicationProcess
	provider   *factoryBuilderProviderCommandRouter
	baseURL    string
	factoryDir string
	requestID  atomic.Uint64
	lifecycle  *factoryBuilderLifecycleLedger
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

func (ledger *factoryBuilderLifecycleLedger) assertClean(t testing.TB) {
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
		"Factory Builder lifecycle: process_starts=%d explicit_sessions_opened=%d explicit_sessions_closed=%d unique_session_ids=%d scenario_roots_removed=%d runtime_artifacts=0 isolated_rows=0",
		processStarts, len(resources), closed, len(sessions), len(roots),
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
		lifecycle: lifecycle,
	}
	// Register the post-process probe before CleanupProcess so Go's LIFO cleanup
	// order stops the hosted command, closes the reusable process, and only then
	// verifies the listener and test-owned roots are gone.
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
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--provider", "CODEX", "--model", "gpt-5",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = factoryDir
	support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	support.WaitForStatus(t, baseURL, factoryBuilderSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	fixture.baseURL = baseURL
	fixture.factoryDir = factoryDir
	return fixture
}

func (fixture *factoryBuilderSharedFixture) cleanup(t testing.TB) {
	t.Helper()
	fixture.lifecycle.assertClean(t)
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

func (fixture *factoryBuilderSharedFixture) nextRequestID() uint64 {
	return fixture.requestID.Add(1)
}

func (fixture *factoryBuilderSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *factoryBuilderScenario {
	t.Helper()
	rootDir := t.TempDir()
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
	return &factoryBuilderScenario{
		fixture:          fixture,
		rootDir:          rootDir,
		homeDir:          homeDir,
		factoryDir:       factoryDir,
		operatorRoot:     filepath.Join(homeDir, ".you-agent-factory", "factories"),
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
}

func (scenario *factoryBuilderScenario) open(t *testing.T) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	scenario.sessionID = opened.Session.Id
	if scenario.sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened Factory Builder Factory Session returned default session %q", scenario.sessionID)
	}
	t.Cleanup(func() {
		scenario.close(t)
	})
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
