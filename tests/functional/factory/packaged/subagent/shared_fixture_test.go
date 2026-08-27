package subagent

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

const subagentSharedFixtureTimeout = 15 * time.Second

const subagentExpectedSessions = 7

// subagentSharedFixture owns one root-built process and one continuous API
// host for the package's compatible child-result scenarios. Each child copies
// the packaged Factory into a private home and opens a non-default session.
type subagentSharedFixture struct {
	owner        testing.TB
	rootDir      string
	process      support.ApplicationProcess
	provider     *subagentProviderCommandRouter
	apiStarter   *subagentAPIServerStarter
	environment  []string
	host         *support.ProcessCommand
	baseURL      string
	factoryDir   string
	scenarioRoot string
	lifecycle    *subagentLifecycleLedger

	mu        sync.Mutex
	scenarios []*subagentScenario
}

type subagentLifecycleResource struct {
	sessionID     string
	rootDir       string
	factoryDir    string
	closed        bool
	sessionAbsent bool
	rootRemoved   bool
}

type subagentLifecycleLedger struct {
	mu            sync.Mutex
	expected      int
	processStarts int
	resources     []subagentLifecycleResource
}

func newSubagentLifecycleLedger(expected int) *subagentLifecycleLedger {
	return &subagentLifecycleLedger{expected: expected}
}

func (ledger *subagentLifecycleLedger) recordProcessStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStarts++
}

func (ledger *subagentLifecycleLedger) register(sessionID, rootDir, factoryDir string) error {
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
	ledger.resources = append(ledger.resources, subagentLifecycleResource{
		sessionID:  sessionID,
		rootDir:    rootDir,
		factoryDir: factoryDir,
	})
	return nil
}

func (ledger *subagentLifecycleLedger) close(sessionID string, sessionAbsent, rootRemoved bool) error {
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

func (ledger *subagentLifecycleLedger) assertClean(t testing.TB) {
	t.Helper()
	ledger.mu.Lock()
	processStarts := ledger.processStarts
	resources := append([]subagentLifecycleResource(nil), ledger.resources...)
	expected := ledger.expected
	ledger.mu.Unlock()

	if processStarts != 1 {
		t.Errorf("SUBAGENT-SPINE-001 process starts = %d, want 1 observed API host start", processStarts)
	}
	if len(resources) != expected {
		t.Errorf("SUBAGENT-SPINE-001 explicit sessions opened = %d, want %d", len(resources), expected)
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
		"subagent lifecycle: process_starts=%d explicit_sessions_opened=%d explicit_sessions_closed=%d unique_session_ids=%d scenario_roots_removed=%d runtime_artifacts=0 isolated_rows=0",
		processStarts, len(resources), closed, len(sessions), len(roots),
	)
}

// subagentAPIServerStarter records listener requests so the no-server CLI
// witness can prove that its invocation did not request a second listener.
type subagentAPIServerStarter struct {
	api   *support.ProcessAPIServer
	calls atomic.Int32
}

func (starter *subagentAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.calls.Add(1)
	return starter.api.Start(ctx, request)
}

// subagentProviderCommandRouter keeps the process edge immutable while
// selecting each scenario's synchronized runner by Factory or work path.
type subagentProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newSubagentProviderCommandRouter() *subagentProviderCommandRouter {
	return &subagentProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *subagentProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[subagentPathKey(path)] = runner
	}
}

func (router *subagentProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, subagentPathKey(path))
	}
}

func (router *subagentProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := subagentPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if subagentPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"subagent provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func subagentPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func subagentPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type subagentScenario struct {
	fixture          *subagentSharedFixture
	rootDir          string
	factoryDir       string
	environment      []string
	workingDirectory string
	selectorPaths    []string
	sessionID        string
}

func newSubagentSharedFixture(t *testing.T) *subagentSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared subagent home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared subagent working directory: %v", err)
	}
	scenarioRoot := filepath.Join(rootDir, "scenarios")
	if err := os.MkdirAll(scenarioRoot, 0o755); err != nil {
		t.Fatalf("create shared subagent scenario root: %v", err)
	}

	api := support.NewProcessAPIServer()
	apiStarter := &subagentAPIServerStarter{api: api}
	provider := newSubagentProviderCommandRouter()
	lifecycle := newSubagentLifecycleLedger(subagentExpectedSessions)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			lifecycle.recordProcessStart()
			return apiStarter.Start(ctx, request)
		},
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(subagent): %v", err)
	}

	fixture := &subagentSharedFixture{
		owner:        t,
		rootDir:      rootDir,
		process:      process,
		provider:     provider,
		apiStarter:   apiStarter,
		environment:  subagentCustomerEnvironment(homeDir),
		scenarioRoot: scenarioRoot,
		lifecycle:    lifecycle,
	}
	// Register the post-process probe before CleanupProcess so Go's LIFO cleanup
	// order stops the hosted command, closes the reusable process, and only then
	// verifies the listener and test-owned roots are gone.
	t.Cleanup(func() { fixture.cleanup(t) })
	support.CleanupProcess(t, process)

	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		fixture.environment,
		workingDirectory,
		"@you/subagent",
	)

	fixture.factoryDir = factoryDir
	return fixture
}

func (fixture *subagentSharedFixture) cleanup(t testing.TB) {
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
			t.Errorf("SUBAGENT-CLEANUP-001 listener still served /status after process close: %s", strings.TrimSpace(string(body)))
		}
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		t.Errorf("SUBAGENT-CLEANUP-001 remove shared root %q: %v", fixture.rootDir, err)
	} else if _, err := os.Stat(fixture.rootDir); !os.IsNotExist(err) {
		t.Errorf("SUBAGENT-CLEANUP-001 shared root %q remains after process close: %v", fixture.rootDir, err)
	}
}

func (fixture *subagentSharedFixture) startHost(t testing.TB) {
	t.Helper()
	if fixture.host != nil {
		return
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", fixture.factoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = fixture.environment
	inputs.Input.WorkingDirectory = fixture.factoryDir
	fixture.host = support.StartProcessCommand(fixture.owner, fixture.process, inputs.Input)
	fixture.baseURL = fixture.apiStarter.api.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, subagentSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	// The continuous host is the one expected listener. Reset the observation
	// baseline so a no-server invocation can assert that it requested none.
	fixture.apiStarter.calls.Store(0)
}

func subagentCustomerEnvironment(homeDir string) []string {
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
		"YOU_DEFAULT_WORKER_MODEL=operator-model",
	)
}

func (fixture *subagentSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *subagentScenario {
	t.Helper()
	scenarioDir, err := os.MkdirTemp(fixture.scenarioRoot, "scenario-")
	if err != nil {
		t.Fatalf("create subagent scenario directory: %v", err)
	}
	homeDir := filepath.Join(scenarioDir, "home")
	workingDirectory := filepath.Join(scenarioDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create subagent scenario home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create subagent scenario working directory: %v", err)
	}
	factoryDir := support.CopyFactoryAsNamed(t, fixture.factoryDir, homeDir, "@you/subagent")
	environment := subagentCustomerEnvironment(homeDir)
	selectorPaths := []string{factoryDir, workingDirectory, fixture.factoryDir}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	scenario := &subagentScenario{
		fixture:          fixture,
		rootDir:          scenarioDir,
		factoryDir:       factoryDir,
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
	fixture.mu.Lock()
	fixture.scenarios = append(fixture.scenarios, scenario)
	fixture.mu.Unlock()
	return scenario
}

// openPendingScenarioSessions accounts for local CLI rows that deliberately
// run before the shared HTTP host. Their customer-owned Factory copies remain
// alive under the parent fixture until these explicit session lifecycles are
// observed and cleaned up.
func (fixture *subagentSharedFixture) openPendingScenarioSessions(t *testing.T) {
	t.Helper()
	fixture.startHost(t)
	fixture.mu.Lock()
	scenarios := append([]*subagentScenario(nil), fixture.scenarios...)
	fixture.mu.Unlock()
	for _, scenario := range scenarios {
		if scenario.sessionID == "" {
			scenario.open(t)
		}
	}
}

func (scenario *subagentScenario) open(t *testing.T) {
	t.Helper()
	scenario.fixture.startHost(t)
	opened := support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	scenario.sessionID = opened.Session.Id
	if scenario.sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened subagent Factory Session returned default session %q", scenario.sessionID)
	}
	t.Cleanup(func() {
		scenario.close(t)
	})
	if err := scenario.fixture.lifecycle.register(
		scenario.sessionID, scenario.rootDir, scenario.factoryDir,
	); err != nil {
		t.Fatalf("register subagent scenario lifecycle: %v", err)
	}
}

func (scenario *subagentScenario) close(t testing.TB) {
	t.Helper()
	if scenario == nil || scenario.sessionID == "" {
		return
	}
	support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	assertSubagentSessionAbsent(t, scenario.fixture.baseURL, scenario.sessionID)
	sessionAbsent := true
	rootRemoved := false
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		t.Errorf("SUBAGENT-CLEANUP-001 remove scenario root %q: %v", scenario.rootDir, err)
	} else if _, err := os.Stat(scenario.rootDir); !os.IsNotExist(err) {
		t.Errorf("SUBAGENT-CLEANUP-001 scenario root %q remains: %v", scenario.rootDir, err)
	} else {
		rootRemoved = true
	}
	if err := scenario.fixture.lifecycle.close(scenario.sessionID, sessionAbsent, rootRemoved); err != nil {
		t.Errorf("record subagent scenario cleanup: %v", err)
	}
}

func assertSubagentSessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted subagent Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET deleted subagent Factory Session %q status = %d, want 404: %s",
			sessionID, response.StatusCode, strings.TrimSpace(string(payload)),
		)
	}
}

var _ platformprocess.CommandRunner = (*subagentProviderCommandRouter)(nil)
