package routing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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

const (
	routingSharedFixtureReadyTimeout = 15 * time.Second
	routingProcessStopTimeout        = 10 * time.Second
)

// workRoutingPackageFixture owns the one root-built process and service-mode
// API host shared by this package. Scenario Factory directories and command
// result queues remain isolated; only immutable process wiring is reused.
type workRoutingPackageFixture struct {
	rootDir    string
	hostDir    string
	baseURL    string
	process    support.ApplicationProcess
	command    *workRoutingProcessCommand
	apiStopped <-chan struct{}
	api        *support.ProcessAPIServer
	provider   *workRoutingProviderCommandRunner
	lifecycle  *workRoutingLifecycleLedger
}

var workRoutingPackageFixtureState struct {
	sync.Mutex
	fixture *workRoutingPackageFixture
}

// TestMain closes the package-owned process after all scenario cleanups have
// run. A test's t.Cleanup cannot own this resource because the first test that
// initializes the fixture may finish while later package tests still need it.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeWorkRoutingPackageFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "work routing package fixture cleanup failed: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

type workRoutingProcessCommand struct {
	cancel context.CancelFunc
	done   chan error
}

func startWorkRoutingProcess(
	process support.ApplicationProcess,
	inputs *support.CapturedInputs,
) *workRoutingProcessCommand {
	parent := inputs.Input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input := inputs.Input
	input.Context = ctx
	command := &workRoutingProcessCommand{
		cancel: cancel,
		done:   make(chan error, 1),
	}
	go func() {
		command.done <- process.Execute(input)
	}()
	return command
}

func (command *workRoutingProcessCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	select {
	case err := <-command.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop shared Work routing process command: %w", err)
		}
		return nil
	case <-time.After(routingProcessStopTimeout):
		return fmt.Errorf("timed out waiting for shared Work routing process command shutdown")
	}
}

func ensureWorkRoutingPackageFixture(t *testing.T) *workRoutingPackageFixture {
	t.Helper()

	workRoutingPackageFixtureState.Lock()
	defer workRoutingPackageFixtureState.Unlock()
	if workRoutingPackageFixtureState.fixture != nil {
		return workRoutingPackageFixtureState.fixture
	}
	fixture := newWorkRoutingPackageFixture(t)
	workRoutingPackageFixtureState.fixture = fixture
	return fixture
}

func newWorkRoutingPackageFixture(t *testing.T) *workRoutingPackageFixture {
	t.Helper()

	rootDir, err := os.MkdirTemp("", "c05-work-routing-package-")
	if err != nil {
		t.Fatalf("create shared Work routing package root: %v", err)
	}
	keepRoot := false
	var process support.ApplicationProcess
	var command *workRoutingProcessCommand
	defer func() {
		if keepRoot {
			return
		}
		if command != nil {
			_ = command.stop()
		}
		if process != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), routingProcessStopTimeout)
			_ = process.Close(closeCtx)
			cancel()
		}
		_ = os.RemoveAll(rootDir)
	}()

	hostDir, err := copyWorkRoutingFixtureDir(
		support.LegacyFixtureDir(t, "logical_move_dir"),
		filepath.Join(rootDir, "host"),
	)
	if err != nil {
		t.Fatalf("copy shared Work routing host Factory: %v", err)
	}
	homeDir := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared Work routing home: %v", err)
	}

	api := support.NewProcessAPIServer()
	apiStopped := make(chan struct{})
	var apiStopOnce sync.Once
	provider := newWorkRoutingProviderCommandRunner()
	lifecycle := newWorkRoutingLifecycleLedger()
	process, err = support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			fixtureStart := lifecycle.recordProcessStart
			fixtureStart()
			err := api.Start(ctx, request)
			apiStopOnce.Do(func() { close(apiStopped) })
			return err
		},
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(Work routing): %v", err)
	}
	fixture := &workRoutingPackageFixture{
		rootDir:    rootDir,
		hostDir:    hostDir,
		api:        api,
		provider:   provider,
		lifecycle:  lifecycle,
		apiStopped: apiStopped,
		process:    process,
	}

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", hostDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = workRoutingEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	command = startWorkRoutingProcess(process, inputs)
	fixture.command = command

	baseURL := api.WaitForURL(t)
	fixture.baseURL = baseURL
	support.WaitForStatus(t, baseURL, routingSharedFixtureReadyTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	keepRoot = true
	return fixture
}

func closeWorkRoutingPackageFixture() error {
	workRoutingPackageFixtureState.Lock()
	fixture := workRoutingPackageFixtureState.fixture
	workRoutingPackageFixtureState.Unlock()
	if fixture == nil {
		return nil
	}

	var errs []error
	if err := fixture.command.stop(); err != nil {
		errs = append(errs, err)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), routingProcessStopTimeout)
	if err := fixture.process.Close(closeCtx); err != nil {
		errs = append(errs, fmt.Errorf("close shared Work routing process: %w", err))
	}
	cancel()
	select {
	case <-fixture.apiStopped:
	case <-time.After(routingProcessStopTimeout):
		errs = append(errs, fmt.Errorf("shared Work routing API server did not close after process cleanup"))
	}
	if err := fixture.lifecycle.assertClean(); err != nil {
		errs = append(errs, err)
	}
	if active := fixture.provider.activeCallCount(); active != 0 {
		errs = append(errs, fmt.Errorf("active Work routing command calls after package cleanup = %d", active))
	}
	if fixture.baseURL != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			errs = append(errs, fmt.Errorf(
				"CLEAN-001 shared Work routing listener still served /status after process close: %s",
				strings.TrimSpace(string(body)),
			))
		}
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove shared Work routing package root %q: %w", fixture.rootDir, err))
	} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("CLEAN-001 shared Work routing package root %q remains: %v", fixture.rootDir, err))
	}
	return errors.Join(errs...)
}

func workRoutingEnvironment(homeDir string) []string {
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

type workRoutingLifecycleResource struct {
	sessionID     string
	rootDir       string
	factoryDir    string
	closed        bool
	sessionAbsent bool
	rootRemoved   bool
}

type workRoutingLifecycleLedger struct {
	mu            sync.Mutex
	processStarts int
	resources     []workRoutingLifecycleResource
}

func newWorkRoutingLifecycleLedger() *workRoutingLifecycleLedger {
	return &workRoutingLifecycleLedger{}
}

func (ledger *workRoutingLifecycleLedger) recordProcessStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStarts++
}

func (ledger *workRoutingLifecycleLedger) register(
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
	ledger.resources = append(ledger.resources, workRoutingLifecycleResource{
		sessionID:  sessionID,
		rootDir:    rootDir,
		factoryDir: factoryDir,
	})
	return nil
}

func (ledger *workRoutingLifecycleLedger) close(
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

func (ledger *workRoutingLifecycleLedger) assertClean() error {
	ledger.mu.Lock()
	processStarts := ledger.processStarts
	resources := append([]workRoutingLifecycleResource(nil), ledger.resources...)
	ledger.mu.Unlock()

	var errs []error
	if processStarts != 1 {
		errs = append(errs, fmt.Errorf("SPINE-001 process starts = %d, want exactly one eligible API host", processStarts))
	}
	if len(resources) == 0 {
		errs = append(errs, fmt.Errorf("SPINE-001 explicit Factory Sessions opened = 0, want at least one scenario"))
	}
	sessions := make(map[string]struct{}, len(resources))
	roots := make(map[string]struct{}, len(resources))
	factories := make(map[string]struct{}, len(resources))
	closed := 0
	for _, resource := range resources {
		if _, exists := sessions[resource.sessionID]; exists {
			errs = append(errs, fmt.Errorf("Factory Session %q is not unique", resource.sessionID))
		}
		sessions[resource.sessionID] = struct{}{}
		if _, exists := roots[resource.rootDir]; exists {
			errs = append(errs, fmt.Errorf("scenario root %q is not unique", resource.rootDir))
		}
		roots[resource.rootDir] = struct{}{}
		if _, exists := factories[resource.factoryDir]; exists {
			errs = append(errs, fmt.Errorf("Factory definition %q is not unique", resource.factoryDir))
		}
		factories[resource.factoryDir] = struct{}{}
		if resource.closed {
			closed++
		} else {
			errs = append(errs, fmt.Errorf("Factory Session %q remains open", resource.sessionID))
		}
		if !resource.sessionAbsent {
			errs = append(errs, fmt.Errorf("Factory Session %q remained publicly readable after close", resource.sessionID))
		}
		if !resource.rootRemoved {
			errs = append(errs, fmt.Errorf("scenario root %q remains after cleanup", resource.rootDir))
		}
	}
	if closed != len(resources) {
		errs = append(errs, fmt.Errorf("explicit Factory Sessions closed = %d, want %d", closed, len(resources)))
	}
	return errors.Join(errs...)
}

// workRoutingProviderCommandRunner is the single injected command edge. Its
// selector table is synchronized because registrations are scenario setup
// state, while every runtime call selects by path and never by global order.
type workRoutingProviderCommandRunner struct {
	mu     sync.RWMutex
	routes map[string]workRoutingProviderRoute
}

type workRoutingProviderRoute struct {
	scenarioID string
	selector   string
	runner     *workRoutingScenarioCommandRunner
}

func newWorkRoutingProviderCommandRunner() *workRoutingProviderCommandRunner {
	return &workRoutingProviderCommandRunner{
		routes: make(map[string]workRoutingProviderRoute),
	}
}

func (router *workRoutingProviderCommandRunner) register(
	scenarioID string,
	selectors []string,
	runner *workRoutingScenarioCommandRunner,
) error {
	if strings.TrimSpace(scenarioID) == "" {
		return fmt.Errorf("Work routing scenario selector ID is required")
	}
	if runner == nil {
		return fmt.Errorf("Work routing scenario %q has no command runner", scenarioID)
	}
	if len(selectors) == 0 {
		return fmt.Errorf("Work routing scenario %q has no selectors", scenarioID)
	}
	normalized := make([]string, 0, len(selectors))
	seen := make(map[string]struct{}, len(selectors))
	for _, rawSelector := range selectors {
		selector := workRoutingPathKey(rawSelector)
		if selector == "" {
			return fmt.Errorf("Work routing scenario %q has an empty selector", scenarioID)
		}
		if _, exists := seen[selector]; exists {
			continue
		}
		seen[selector] = struct{}{}
		normalized = append(normalized, selector)
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, selector := range normalized {
		if route, exists := router.routes[selector]; exists && route.scenarioID != scenarioID {
			return fmt.Errorf(
				"duplicate Work routing selector %q for scenarios %q and %q",
				selector, route.scenarioID, scenarioID,
			)
		}
	}
	for _, selector := range normalized {
		router.routes[selector] = workRoutingProviderRoute{
			scenarioID: scenarioID,
			selector:   selector,
			runner:     runner,
		}
	}
	return nil
}

func (router *workRoutingProviderCommandRunner) unregister(scenarioID string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for selector, route := range router.routes {
		if route.scenarioID == scenarioID {
			delete(router.routes, selector)
		}
	}
}

func (router *workRoutingProviderCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := workRoutingPathKey(request.WorkDir)
	router.mu.RLock()
	matched := make(map[string]workRoutingProviderRoute)
	for _, route := range router.routes {
		if workRoutingPathContains(route.selector, workDir) {
			matched[route.scenarioID] = route
		}
	}
	router.mu.RUnlock()
	if len(matched) != 1 {
		ids := make([]string, 0, len(matched))
		for scenarioID := range matched {
			ids = append(ids, scenarioID)
		}
		sort.Strings(ids)
		return platformprocess.CommandResult{}, fmt.Errorf(
			"Work routing provider selector matched %d scenarios %q for work directory %q; expected exactly one",
			len(matched), strings.Join(ids, ","), request.WorkDir,
		)
	}
	for _, route := range matched {
		return route.runner.Run(ctx, request)
	}
	return platformprocess.CommandResult{}, fmt.Errorf("Work routing provider selector resolution produced no route")
}

func (router *workRoutingProviderCommandRunner) activeCallCount() int {
	router.mu.RLock()
	runners := make(map[*workRoutingScenarioCommandRunner]struct{}, len(router.routes))
	for _, route := range router.routes {
		runners[route.runner] = struct{}{}
	}
	router.mu.RUnlock()
	active := 0
	for runner := range runners {
		active += runner.activeCallCount()
	}
	return active
}

type workRoutingScenarioCommandRunner struct {
	mu       sync.Mutex
	name     string
	results  []platformprocess.CommandResult
	errors   []error
	index    int
	requests []platformprocess.CommandRequest
	active   atomic.Int32
}

func newWorkRoutingScenarioCommandRunner(
	name string,
	results []platformprocess.CommandResult,
	errors []error,
) *workRoutingScenarioCommandRunner {
	clonedResults := make([]platformprocess.CommandResult, len(results))
	for index, result := range results {
		clonedResults[index] = cloneWorkRoutingCommandResult(result)
	}
	return &workRoutingScenarioCommandRunner{
		name:    name,
		results: clonedResults,
		errors:  append([]error(nil), errors...),
	}
}

func (runner *workRoutingScenarioCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.active.Add(1)
	defer runner.active.Add(-1)
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	default:
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	callIndex := runner.index
	runner.index++
	runner.requests = append(runner.requests, cloneWorkRoutingCommandRequest(request))
	if callIndex >= len(runner.results) {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"Work routing scenario %q command result queue exhausted at call %d",
			runner.name, callIndex+1,
		)
	}
	result := cloneWorkRoutingCommandResult(runner.results[callIndex])
	if callIndex < len(runner.errors) && runner.errors[callIndex] != nil {
		return result, runner.errors[callIndex]
	}
	return result, nil
}

func (runner *workRoutingScenarioCommandRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *workRoutingScenarioCommandRunner) requestsSnapshot() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = cloneWorkRoutingCommandRequest(request)
	}
	return requests
}

func (runner *workRoutingScenarioCommandRunner) activeCallCount() int {
	return int(runner.active.Load())
}

func cloneWorkRoutingCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneWorkRoutingCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

type workRoutingScenario struct {
	fixture    *workRoutingPackageFixture
	id         string
	rootDir    string
	factoryDir string
	runner     *workRoutingScenarioCommandRunner
	sessionID  string
}

func (fixture *workRoutingPackageFixture) newScenario(
	t *testing.T,
	id string,
	sourceFixture string,
	runner *workRoutingScenarioCommandRunner,
) *workRoutingScenario {
	t.Helper()
	rootDir, err := os.MkdirTemp(fixture.rootDir, "scenario-")
	if err != nil {
		t.Fatalf("create Work routing scenario root: %v", err)
	}
	factoryDir, err := copyWorkRoutingFixtureDir(
		support.LegacyFixtureDir(t, sourceFixture),
		filepath.Join(rootDir, "factory"),
	)
	if err != nil {
		t.Fatalf("copy Work routing scenario Factory %q: %v", id, err)
	}
	selectors := []string{rootDir, factoryDir}
	if err := fixture.provider.register(id, selectors, runner); err != nil {
		t.Fatalf("register Work routing scenario %q: %v", id, err)
	}
	t.Cleanup(func() { fixture.provider.unregister(id) })
	return &workRoutingScenario{
		fixture:    fixture,
		id:         id,
		rootDir:    rootDir,
		factoryDir: factoryDir,
		runner:     runner,
	}
}

func (scenario *workRoutingScenario) open(t *testing.T) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	scenario.sessionID = opened.Session.Id
	t.Cleanup(func() { scenario.close(t) })
	if scenario.sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("Work routing scenario %q opened the default Factory Session", scenario.id)
	}
	if err := scenario.fixture.lifecycle.register(
		scenario.sessionID,
		scenario.rootDir,
		scenario.factoryDir,
	); err != nil {
		t.Fatalf("register Work routing scenario %q lifecycle: %v", scenario.id, err)
	}
}

func (scenario *workRoutingScenario) close(t testing.TB) {
	t.Helper()
	if scenario == nil || scenario.sessionID == "" {
		return
	}
	support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	assertWorkRoutingSessionAbsent(t, scenario.fixture.baseURL, scenario.sessionID)
	sessionAbsent := true
	rootRemoved := false
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		t.Errorf("CLEAN-001 remove Work routing scenario root %q: %v", scenario.rootDir, err)
	} else if _, err := os.Stat(scenario.rootDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("CLEAN-001 Work routing scenario root %q remains: %v", scenario.rootDir, err)
	} else {
		rootRemoved = true
	}
	if err := scenario.fixture.lifecycle.close(scenario.sessionID, sessionAbsent, rootRemoved); err != nil {
		t.Errorf("record Work routing scenario %q cleanup: %v", scenario.id, err)
	}
}

func (scenario *workRoutingScenario) observe(
	t *testing.T,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	support.WaitForSessionTerminalStatus(t, scenario.fixture.baseURL, scenario.sessionID, timeout)
	return getWorkRoutingSession(t, scenario.fixture.baseURL, scenario.sessionID),
		listWorkRoutingSession(t, scenario.fixture.baseURL, scenario.sessionID),
		support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
}

func workRoutingPathKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func workRoutingPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func copyWorkRoutingFixtureDir(sourceDir, targetDir string) (string, error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	err := filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(targetDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return "", err
	}
	return targetDir, nil
}

func getWorkRoutingSession(t testing.TB, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode Work routing Factory Session %q: %v", sessionID, err)
	}
	return session
}

func listWorkRoutingSession(
	t testing.TB,
	baseURL, sessionID string,
) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func getWorkRoutingWorkByID(
	t testing.TB,
	baseURL, sessionID, workID string,
) factoryapi.Work {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) + "/work/" + url.PathEscape(workID)
	return support.GetJSON[factoryapi.Work](t, endpoint)
}

func assertWorkRoutingSessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted Work routing Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET deleted Work routing Factory Session %q status = %d, want 404: %s",
			sessionID, response.StatusCode, strings.TrimSpace(string(body)),
		)
	}
}

var _ platformprocess.CommandRunner = (*workRoutingProviderCommandRunner)(nil)
var _ platformprocess.CommandRunner = (*workRoutingScenarioCommandRunner)(nil)
