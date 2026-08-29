package loop

import (
	"context"
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

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	// Process startup includes production dependency construction and the first
	// loopback bind. This ceiling is intentionally scoped to fixture readiness;
	// it is not reused by scenario phase observations or cleanup.
	loopFixtureReadyTimeout = 15 * time.Second

	// These budgets preserve the old two-second public Work/dispatch bounds and
	// give only the direct runner/scheduler readiness edges a measured margin
	// over their former one-second scheduling opportunities.
	loopProviderPhaseBudget  = 3 * time.Second
	loopSchedulerPhaseBudget = 3 * time.Second
	loopWorkPhaseBudget      = 2 * time.Second
	loopDispatchPhaseBudget  = 2 * time.Second

	// A session cleanup has one bounded total budget. The four operation budgets
	// add to that total, so a stuck phase cannot multiply the old ceiling.
	loopSessionCleanupBudget   = 5 * time.Second
	loopSessionTerminateBudget = 1 * time.Second
	loopSessionStoppedBudget   = 2 * time.Second
	loopSessionDeleteBudget    = 1 * time.Second
	loopSessionAbsentBudget    = 1 * time.Second

	// The HTTP request carries a 20 ms product timeout. This client ceiling only
	// prevents a transport defect from hiding the requested terminal response.
	loopInvocationRequestBudget = 2 * time.Second
	loopStreamCloseBudget       = 1 * time.Second
)

const loopExpectedSessions = 8

// loopSharedFixture owns one root-built process and one continuous API host
// for the package's compatible scheduler scenarios. Each child copies the
// packaged Factory into a private home and opens a non-default Factory Session.
type loopSharedFixture struct {
	rootDir        string
	process        *loopCountingApplicationProcess
	processCommand *support.ProcessCommand
	provider       *loopProviderCommandRouter
	clock          *loopSchedulerClock
	baseURL        string
	factoryDir     string
	submissions    chan work.FactorySubmissionRecord
	lifecycle      *loopLifecycleLedger
}

type loopCountingApplicationProcess struct {
	support.ApplicationProcess
	closeCalls atomic.Uint32
}

func (process *loopCountingApplicationProcess) Close(ctx context.Context) error {
	process.closeCalls.Add(1)
	return process.ApplicationProcess.Close(ctx)
}

func (process *loopCountingApplicationProcess) closeCount() uint32 {
	if process == nil {
		return 0
	}
	return process.closeCalls.Load()
}

type loopLifecycleResource struct {
	sessionID     string
	rootDir       string
	factoryDir    string
	closed        bool
	sessionAbsent bool
	rootRemoved   bool
}

type loopLifecycleLedger struct {
	mu            sync.Mutex
	expected      int
	processStarts int
	processStops  int
	resources     []loopLifecycleResource
}

func newLoopLifecycleLedger(expected int) *loopLifecycleLedger {
	return &loopLifecycleLedger{expected: expected}
}

func (ledger *loopLifecycleLedger) setExpectedSessions(expected int) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.expected = expected
}

func (ledger *loopLifecycleLedger) recordProcessStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStarts++
}

func (ledger *loopLifecycleLedger) recordProcessStop() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStops++
}

func (ledger *loopLifecycleLedger) register(sessionID, rootDir, factoryDir string) error {
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
	ledger.resources = append(ledger.resources, loopLifecycleResource{
		sessionID:  sessionID,
		rootDir:    rootDir,
		factoryDir: factoryDir,
	})
	return nil
}

func (ledger *loopLifecycleLedger) close(sessionID string, sessionAbsent, rootRemoved bool) error {
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

func (ledger *loopLifecycleLedger) assertClean(t testing.TB) {
	t.Helper()
	ledger.mu.Lock()
	processStarts := ledger.processStarts
	processStops := ledger.processStops
	resources := append([]loopLifecycleResource(nil), ledger.resources...)
	expected := ledger.expected
	ledger.mu.Unlock()

	if processStarts != 1 {
		t.Errorf("LOOP-SPINE-001 process starts = %d, want 1 observed API host start", processStarts)
	}
	if processStops != processStarts {
		t.Errorf("LOOP-CLEANUP-001 API listener stops = %d, want %d", processStops, processStarts)
	}
	if len(resources) != expected {
		t.Errorf("LOOP-SPINE-001 explicit sessions opened = %d, want %d", len(resources), expected)
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
		"loop lifecycle: process_starts=%d process_stops=%d explicit_sessions_opened=%d explicit_sessions_closed=%d unique_session_ids=%d scenario_roots_removed=%d runtime_artifacts=0 isolated_rows=0",
		processStarts, processStops, len(resources), closed, len(sessions), len(roots),
	)
}

// loopProviderCommandRouter keeps the process edge immutable while selecting
// each scenario's synchronized runner by Factory or work path.
type loopProviderCommandRouter struct {
	mu                 sync.RWMutex
	runners            map[string]platformprocess.CommandRunner
	unregisterAttempts uint64
}

func newLoopProviderCommandRouter() *loopProviderCommandRouter {
	return &loopProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *loopProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[loopPathKey(path)] = runner
	}
}

func (router *loopProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.unregisterAttempts++
	for _, path := range paths {
		delete(router.runners, loopPathKey(path))
	}
}

func (router *loopProviderCommandRouter) unregisterCount() uint64 {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return router.unregisterAttempts
}

func (router *loopProviderCommandRouter) registeredCount() int {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.runners)
}

func (router *loopProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := loopPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if loopPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"loop provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func loopPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func loopPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type loopScenario struct {
	fixture               *loopSharedFixture
	rootDir               string
	factoryDir            string
	environment           []string
	workingDirectory      string
	selectorPaths         []string
	routeUnregisterBefore uint64
	sessionID             string
	runner                platformprocess.CommandRunner
	cleanup               *loopCleanupStack
	sessionAbsent         bool
	lifecycleTracked      bool
	rootRemoved           bool
}

func newLoopSharedFixture(t *testing.T) *loopSharedFixture {
	return newLoopSharedFixtureWithExpected(t, loopExpectedSessions)
}

func newLoopSharedFixtureWithExpected(t *testing.T, expectedSessions int) *loopSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared loop home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared loop working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newLoopProviderCommandRouter()
	clock := newLoopSchedulerClockAt(time.Date(2026, time.July, 29, 20, 0, 0, 0, time.UTC))
	submissions := make(chan work.FactorySubmissionRecord, 16)
	lifecycle := newLoopLifecycleLedger(expectedSessions)
	builtProcess, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			lifecycle.recordProcessStart()
			err := api.Start(ctx, request)
			lifecycle.recordProcessStop()
			return err
		},
		Clock:                 clock,
		ProviderCommandRunner: provider,
		SubmissionRecorder:    func(record work.FactorySubmissionRecord) { submissions <- record },
	})
	if err != nil {
		t.Fatalf("BuildProcess(loop): %v", err)
	}
	process := &loopCountingApplicationProcess{ApplicationProcess: builtProcess}
	fixture := &loopSharedFixture{
		rootDir:     rootDir,
		process:     process,
		provider:    provider,
		clock:       clock,
		submissions: submissions,
		lifecycle:   lifecycle,
	}
	// Register the post-process probe before CleanupProcess so Go's LIFO cleanup
	// order stops the hosted command, closes the reusable process, and only then
	// verifies the listener and test-owned roots are gone.
	t.Cleanup(func() { fixture.cleanup(t) })
	support.CleanupProcess(t, process)

	environment := loopCustomerEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		environment,
		workingDirectory,
		factorydefinitions.PackagedLoopFactoryName,
	)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--provider", "CODEX", "--model", "operator-model",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = factoryDir
	fixture.processCommand = support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	support.WaitForStatus(t, baseURL, loopFixtureReadyTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	fixture.baseURL = baseURL
	fixture.factoryDir = factoryDir
	return fixture
}

func (fixture *loopSharedFixture) cleanup(t testing.TB) {
	t.Helper()
	fixture.lifecycle.assertClean(t)
	if fixture.processCommand != nil {
		select {
		case <-fixture.processCommand.Done():
		default:
			t.Errorf("LOOP-CLEANUP-001 Process.Execute goroutine remains after cleanup")
		}
	}
	if got := fixture.process.closeCount(); got != 1 {
		t.Errorf("LOOP-CLEANUP-001 reusable process close calls = %d, want 1", got)
	}
	if registered := fixture.provider.registeredCount(); registered != 0 {
		t.Errorf("LOOP-CLEANUP-001 provider route registrations remaining = %d", registered)
	}
	if fixture.baseURL != "" {
		// This is a single bounded shutdown probe, not synchronization: after the
		// reusable process closes, its injected listener must reject /status.
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Errorf("LOOP-CLEANUP-001 listener still served /status after process close: %s", strings.TrimSpace(string(body)))
		}
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		t.Errorf("LOOP-CLEANUP-001 remove shared root %q: %v", fixture.rootDir, err)
	} else if _, err := os.Stat(fixture.rootDir); !os.IsNotExist(err) {
		t.Errorf("LOOP-CLEANUP-001 shared root %q remains after process close: %v", fixture.rootDir, err)
	}
}

func loopCustomerEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
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
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func (fixture *loopSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *loopScenario {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create loop scenario home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create loop scenario working directory: %v", err)
	}
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.factoryDir,
		homeDir,
		factorydefinitions.PackagedLoopFactoryName,
	)
	environment := loopCustomerEnvironment(homeDir)
	selectorPaths := []string{factoryDir, workingDirectory, fixture.factoryDir}
	routeUnregisterBefore := fixture.provider.unregisterCount()
	fixture.provider.register(selectorPaths, runner)
	scenario := &loopScenario{
		fixture:               fixture,
		rootDir:               rootDir,
		factoryDir:            factoryDir,
		environment:           environment,
		workingDirectory:      workingDirectory,
		selectorPaths:         selectorPaths,
		routeUnregisterBefore: routeUnregisterBefore,
		runner:                runner,
		cleanup:               newLoopCleanupStack(),
	}
	scenario.cleanup.add("scenario root", scenario.removeRoot)
	scenario.cleanup.add("provider route registrations", func() error {
		fixture.provider.unregister(selectorPaths)
		return nil
	})
	t.Cleanup(func() { scenario.close(t) })
	return scenario
}

func (scenario *loopScenario) open(t *testing.T) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	scenario.sessionID = opened.Session.Id
	if scenario.sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened loop Factory Session returned default session %q", scenario.sessionID)
	}
	scenario.cleanup.add("Factory Session", func() error {
		absent, err := closeLoopSession(scenario.fixture.baseURL, scenario.sessionID)
		scenario.sessionAbsent = absent
		return err
	})
	scenario.cleanup.add("controlled provider runner", func() error {
		return releaseLoopRunner(scenario.runner)
	})
	if err := scenario.fixture.lifecycle.register(
		scenario.sessionID, scenario.rootDir, scenario.factoryDir,
	); err != nil {
		t.Fatalf("register loop scenario lifecycle: %v", err)
	}
	scenario.lifecycleTracked = true
}

func (scenario *loopScenario) close(t testing.TB) {
	t.Helper()
	if scenario == nil || scenario.cleanup == nil {
		return
	}
	alreadyRun := scenario.cleanup.hasRun()
	if err := scenario.cleanup.run(); err != nil && !alreadyRun {
		t.Errorf("LOOP-CLEANUP-001 scenario cleanup: %v", err)
	}
}

var _ platformprocess.CommandRunner = (*loopProviderCommandRouter)(nil)
