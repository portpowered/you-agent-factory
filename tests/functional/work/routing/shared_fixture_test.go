package routing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

const (
	routingSharedFixtureReadyTimeout = 15 * time.Second
	routingProcessStopTimeout        = 10 * time.Second
	// Shared runtime requests contend on the same service-mode process. Four
	// fixed workers preserve package throughput while making scenario scheduling
	// deterministic for uncached elapsed-time samples.
	routingScenarioConcurrency = 4
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

func beginWorkRoutingScenarioRun(t *testing.T, fixture *workRoutingPackageFixture) {
	t.Helper()
	if err := fixture.lifecycle.resetScenarioRun(); err != nil {
		t.Fatalf("reset Work routing lifecycle census for test iteration: %v", err)
	}
	if err := fixture.provider.resetScenarioRun(); err != nil {
		t.Fatalf("reset Work routing provider census for test iteration: %v", err)
	}
	t.Cleanup(func() {
		if err := fixture.lifecycle.assertScenarioClean(); err != nil {
			t.Errorf("Work routing scenario cleanup census: %v", err)
		}
		if err := fixture.provider.assertClean(); err != nil {
			t.Errorf("Work routing provider cleanup census: %v", err)
		}
		fmt.Fprintf(os.Stderr, "work routing iteration cleanup census: %s; %s\n", fixture.lifecycle.summary(), fixture.provider.summary())
	})
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
			lifecycle.recordProcessStart()
			lifecycle.recordListenerStart()
			err := api.Start(ctx, request)
			lifecycle.recordListenerClose()
			apiStopOnce.Do(func() { close(apiStopped) })
			return err
		},
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(Work routing): %v", err)
	}
	lifecycle.recordProcessBuild()
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
	listenerClosed := false
	select {
	case <-fixture.apiStopped:
		listenerClosed = true
	case <-time.After(routingProcessStopTimeout):
		errs = append(errs, fmt.Errorf("shared Work routing API server did not close after process cleanup"))
	}
	if err := fixture.lifecycle.recordProcessClose(); err != nil {
		errs = append(errs, err)
	}
	if err := fixture.provider.assertClean(); err != nil {
		errs = append(errs, err)
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
	rootRemoved := false
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove shared Work routing package root %q: %w", fixture.rootDir, err))
	} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("CLEAN-001 shared Work routing package root %q remains: %v", fixture.rootDir, err))
	} else {
		rootRemoved = true
	}
	fmt.Fprintf(os.Stderr, "work routing cleanup census: %s; %s\n", fixture.lifecycle.summary(), fixture.provider.summary())
	if err := fixture.lifecycle.assertClean(listenerClosed, rootRemoved); err != nil {
		errs = append(errs, err)
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
	scenarioID        string
	sessionID         string
	rootDir           string
	factoryDir        string
	sessionCloseCount int
	sessionAbsent     bool
	rootCloseCount    int
	rootRemoved       bool
	factoryRemoved    bool
}

type workRoutingReaderResource struct {
	id         string
	owner      string
	closeCount int
}

type workRoutingLifecycleLedger struct {
	mu             sync.Mutex
	processBuilds  int
	processStarts  int
	processCloses  int
	listenerStarts int
	listenerCloses int
	nextReader     int
	resources      []workRoutingLifecycleResource
	readers        map[string]*workRoutingReaderResource
}

func newWorkRoutingLifecycleLedger() *workRoutingLifecycleLedger {
	return &workRoutingLifecycleLedger{readers: make(map[string]*workRoutingReaderResource)}
}

func (ledger *workRoutingLifecycleLedger) recordProcessBuild() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processBuilds++
}

func (ledger *workRoutingLifecycleLedger) recordProcessStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStarts++
}

func (ledger *workRoutingLifecycleLedger) recordProcessClose() error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.processBuilds == 0 {
		return fmt.Errorf("owner=%q resource=process close has no matching BuildProcess", "work-routing-package")
	}
	if ledger.processCloses >= ledger.processBuilds {
		return fmt.Errorf("owner=%q resource=process close count=%d want exactly once", "work-routing-package", ledger.processCloses+1)
	}
	ledger.processCloses++
	return nil
}

func (ledger *workRoutingLifecycleLedger) recordListenerStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.listenerStarts++
}

func (ledger *workRoutingLifecycleLedger) recordListenerClose() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.listenerCloses++
}

func (ledger *workRoutingLifecycleLedger) registerScenario(
	scenarioID, rootDir, factoryDir string,
) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if strings.TrimSpace(scenarioID) == "" {
		return fmt.Errorf("Work routing scenario ID is empty")
	}
	for _, resource := range ledger.resources {
		if resource.scenarioID == scenarioID {
			return fmt.Errorf("Work routing scenario ID %q is not unique", scenarioID)
		}
	}
	for label, path := range map[string]string{"scenario root": rootDir, "Factory": factoryDir} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("%s path %q is not absolute", label, path)
		}
	}
	ledger.resources = append(ledger.resources, workRoutingLifecycleResource{
		scenarioID: scenarioID,
		rootDir:    rootDir,
		factoryDir: factoryDir,
	})
	return nil
}

func (ledger *workRoutingLifecycleLedger) openSession(scenarioID, sessionID string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("owner=%q resource=session id is empty", scenarioID)
	}
	if sessionID == factorysessions.DefaultSessionID {
		return fmt.Errorf("owner=%q resource=session id is the default session %q", scenarioID, sessionID)
	}
	var owner *workRoutingLifecycleResource
	for index := range ledger.resources {
		resource := &ledger.resources[index]
		if resource.scenarioID == scenarioID {
			owner = resource
			break
		}
		if resource.sessionID == sessionID {
			return fmt.Errorf("owner=%q resource=session id=%q is not unique", scenarioID, sessionID)
		}
	}
	if owner == nil {
		return fmt.Errorf("owner=%q resource=session has no registered scenario", scenarioID)
	}
	if owner.sessionID != "" {
		return fmt.Errorf("owner=%q resource=session already opened as %q", scenarioID, owner.sessionID)
	}
	owner.sessionID = sessionID
	return nil
}

func (ledger *workRoutingLifecycleLedger) closeScenario(
	scenarioID string,
	sessionAbsent, rootRemoved, factoryRemoved bool,
) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	for index := range ledger.resources {
		resource := &ledger.resources[index]
		if resource.scenarioID != scenarioID {
			continue
		}
		if resource.rootCloseCount != 0 {
			return fmt.Errorf("owner=%q resource=scenario-root close count=%d want exactly once", scenarioID, resource.rootCloseCount+1)
		}
		resource.rootCloseCount = 1
		resource.rootRemoved = rootRemoved
		resource.factoryRemoved = factoryRemoved
		if resource.sessionID != "" {
			if resource.sessionCloseCount != 0 {
				return fmt.Errorf("owner=%q resource=session id=%q close count=%d want exactly once", scenarioID, resource.sessionID, resource.sessionCloseCount+1)
			}
			resource.sessionCloseCount = 1
			resource.sessionAbsent = sessionAbsent
		}
		return nil
	}
	return fmt.Errorf("owner=%q resource=scenario is not registered", scenarioID)
}

func (ledger *workRoutingLifecycleLedger) resetScenarioRun() error {
	ledger.mu.Lock()
	hasResources := len(ledger.resources) != 0 || len(ledger.readers) != 0
	ledger.mu.Unlock()
	if hasResources {
		if err := ledger.assertScenarioClean(); err != nil {
			return fmt.Errorf("previous Work routing scenario iteration is not clean: %w", err)
		}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.resources = nil
	ledger.readers = make(map[string]*workRoutingReaderResource)
	ledger.nextReader = 0
	return nil
}

func (ledger *workRoutingLifecycleLedger) beginReader(owner string) string {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.nextReader++
	if strings.TrimSpace(owner) == "" {
		owner = "unknown"
	}
	id := fmt.Sprintf("work-routing-reader-%d", ledger.nextReader)
	ledger.readers[id] = &workRoutingReaderResource{id: id, owner: owner}
	return id
}

func (ledger *workRoutingLifecycleLedger) closeReader(id string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	resource, ok := ledger.readers[id]
	if !ok {
		return fmt.Errorf("owner=%q resource=reader id=%q close is unregistered", "work-routing-ledger", id)
	}
	if resource.closeCount != 0 {
		return fmt.Errorf("owner=%q resource=reader id=%q close count=%d want exactly once", resource.owner, id, resource.closeCount+1)
	}
	resource.closeCount = 1
	return nil
}

func (ledger *workRoutingLifecycleLedger) summary() string {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	processBuilds := ledger.processBuilds
	processCloses := ledger.processCloses
	processStarts := ledger.processStarts
	listenerStarts := ledger.listenerStarts
	listenerCloses := ledger.listenerCloses
	sessionsOpened := 0
	sessionsClosed := 0
	rootsClosed := 0
	factoriesRemoved := 0
	for _, resource := range ledger.resources {
		if resource.sessionID != "" {
			sessionsOpened++
		}
		if resource.sessionCloseCount == 1 {
			sessionsClosed++
		}
		if resource.rootCloseCount == 1 {
			rootsClosed++
		}
		if resource.factoryRemoved {
			factoriesRemoved++
		}
	}
	readersClosed := 0
	for _, resource := range ledger.readers {
		if resource.closeCount == 1 {
			readersClosed++
		}
	}
	return fmt.Sprintf(
		"process-builds=%d process-starts=%d process-closes=%d listener-starts=%d listener-closes=%d scenarios=%d scenario-roots-closed=%d factory-paths-removed=%d sessions-opened=%d sessions-closed=%d readers-opened=%d readers-closed=%d",
		processBuilds,
		processStarts,
		processCloses,
		listenerStarts,
		listenerCloses,
		len(ledger.resources),
		rootsClosed,
		factoriesRemoved,
		sessionsOpened,
		sessionsClosed,
		len(ledger.readers),
		readersClosed,
	)
}

func (ledger *workRoutingLifecycleLedger) assertScenarioClean() error {
	ledger.mu.Lock()
	resources := append([]workRoutingLifecycleResource(nil), ledger.resources...)
	readers := make([]workRoutingReaderResource, 0, len(ledger.readers))
	for _, resource := range ledger.readers {
		readers = append(readers, *resource)
	}
	ledger.mu.Unlock()

	var errs []error
	if len(resources) == 0 {
		errs = append(errs, fmt.Errorf("owner=%q resource=scenario count=0 want at least one scenario per test iteration", "work-routing-package"))
	}
	sessions := make(map[string]struct{}, len(resources))
	roots := make(map[string]struct{}, len(resources))
	factories := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if resource.sessionID != "" {
			if _, exists := sessions[resource.sessionID]; exists {
				errs = append(errs, fmt.Errorf("Factory Session %q is not unique", resource.sessionID))
			}
			sessions[resource.sessionID] = struct{}{}
		}
		if _, exists := roots[resource.rootDir]; exists {
			errs = append(errs, fmt.Errorf("scenario root %q is not unique", resource.rootDir))
		}
		roots[resource.rootDir] = struct{}{}
		if _, exists := factories[resource.factoryDir]; exists {
			errs = append(errs, fmt.Errorf("Factory definition %q is not unique", resource.factoryDir))
		}
		factories[resource.factoryDir] = struct{}{}
		if resource.rootCloseCount != 1 {
			errs = append(errs, fmt.Errorf("owner=%q resource=scenario-root close count=%d want exactly once", resource.scenarioID, resource.rootCloseCount))
		}
		if !resource.rootRemoved {
			errs = append(errs, fmt.Errorf("owner=%q resource=scenario-root path=%q remains after cleanup", resource.scenarioID, resource.rootDir))
		}
		if !resource.factoryRemoved {
			errs = append(errs, fmt.Errorf("owner=%q resource=factory-path path=%q remains after cleanup", resource.scenarioID, resource.factoryDir))
		}
		if resource.sessionID != "" {
			if resource.sessionCloseCount != 1 {
				errs = append(errs, fmt.Errorf("owner=%q resource=session id=%q close count=%d want exactly once", resource.scenarioID, resource.sessionID, resource.sessionCloseCount))
			}
			if !resource.sessionAbsent {
				errs = append(errs, fmt.Errorf("owner=%q resource=session id=%q remained publicly readable after close", resource.scenarioID, resource.sessionID))
			}
		}
	}
	for _, resource := range readers {
		if resource.closeCount != 1 {
			errs = append(errs, fmt.Errorf("owner=%q resource=reader id=%q close count=%d want exactly once", resource.owner, resource.id, resource.closeCount))
		}
	}
	return errors.Join(errs...)
}

func (ledger *workRoutingLifecycleLedger) assertClean(listenerClosed, sharedRootRemoved bool) error {
	ledger.mu.Lock()
	processBuilds := ledger.processBuilds
	processStarts := ledger.processStarts
	processCloses := ledger.processCloses
	listenerStarts := ledger.listenerStarts
	listenerCloses := ledger.listenerCloses
	ledger.mu.Unlock()

	var errs []error
	if processBuilds != 1 {
		errs = append(errs, fmt.Errorf("owner=%q resource=process BuildProcess count=%d want exactly one eligible API host", "work-routing-package", processBuilds))
	}
	if processStarts != 1 {
		errs = append(errs, fmt.Errorf("owner=%q resource=process starts=%d want exactly once", "work-routing-package", processStarts))
	}
	if processCloses != 1 {
		errs = append(errs, fmt.Errorf("owner=%q resource=process closes=%d want exactly once", "work-routing-package", processCloses))
	}
	if listenerStarts != 1 || listenerCloses != 1 || !listenerClosed {
		errs = append(errs, fmt.Errorf("owner=%q resource=http-listener starts=%d closes=%d probe-closed=%t want one close and an unreachable listener", "work-routing-package", listenerStarts, listenerCloses, listenerClosed))
	}
	if err := ledger.assertScenarioClean(); err != nil {
		errs = append(errs, err)
	}
	if !sharedRootRemoved {
		errs = append(errs, fmt.Errorf("owner=%q resource=shared-fixture-root removed=false", "work-routing-package"))
	}
	return errors.Join(errs...)
}

func workRoutingRead(t testing.TB, fixture *workRoutingPackageFixture, owner string, read func()) {
	t.Helper()
	readerID := fixture.lifecycle.beginReader(owner)
	defer func() {
		if err := fixture.lifecycle.closeReader(readerID); err != nil {
			t.Errorf("close Work routing reader %q: %v", owner, err)
		}
	}()
	read()
}

func workRoutingReadValue[T any](
	t testing.TB,
	fixture *workRoutingPackageFixture,
	owner string,
	read func() T,
) (value T) {
	t.Helper()
	readerID := fixture.lifecycle.beginReader(owner)
	defer func() {
		if err := fixture.lifecycle.closeReader(readerID); err != nil {
			t.Errorf("close Work routing reader %q: %v", owner, err)
		}
	}()
	return read()
}

// workRoutingProviderCommandRunner is the single injected command edge. Its
// selector table is synchronized because registrations are scenario setup
// state, while every runtime call selects by path and never by global order.
type workRoutingProviderCommandRunner struct {
	mu               sync.RWMutex
	routes           map[string]workRoutingProviderRoute
	routeResources   map[string]*workRoutingRouteResource
	routeRegisters   int
	routeUnregisters int
}

type workRoutingProviderRoute struct {
	scenarioID string
	selector   string
	runner     *workRoutingScenarioCommandRunner
}

type workRoutingRouteResource struct {
	scenarioID      string
	selectors       []string
	runner          *workRoutingScenarioCommandRunner
	unregisterCount int
}

func newWorkRoutingProviderCommandRunner() *workRoutingProviderCommandRunner {
	return &workRoutingProviderCommandRunner{
		routes:         make(map[string]workRoutingProviderRoute),
		routeResources: make(map[string]*workRoutingRouteResource),
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
	if _, exists := router.routeResources[scenarioID]; exists {
		return fmt.Errorf("Work routing scenario %q registered a provider route more than once", scenarioID)
	}
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
	router.routeResources[scenarioID] = &workRoutingRouteResource{
		scenarioID: scenarioID,
		selectors:  append([]string(nil), normalized...),
		runner:     runner,
	}
	router.routeRegisters++
	return nil
}

func (router *workRoutingProviderCommandRunner) unregister(scenarioID string) error {
	router.mu.Lock()
	defer router.mu.Unlock()
	resource, exists := router.routeResources[scenarioID]
	if !exists {
		return fmt.Errorf("Work routing scenario %q provider route is not registered", scenarioID)
	}
	if resource.unregisterCount != 0 {
		return fmt.Errorf("Work routing scenario %q provider route was unregistered more than once", scenarioID)
	}
	removed := 0
	for selector, route := range router.routes {
		if route.scenarioID == scenarioID {
			delete(router.routes, selector)
			removed++
		}
	}
	resource.unregisterCount = 1
	router.routeUnregisters++
	if removed != len(resource.selectors) {
		return fmt.Errorf(
			"Work routing scenario %q removed %d provider selectors, want %d",
			scenarioID,
			removed,
			len(resource.selectors),
		)
	}
	return nil
}

func (router *workRoutingProviderCommandRunner) resetScenarioRun() error {
	router.mu.RLock()
	hasResources := len(router.routeResources) != 0 || len(router.routes) != 0 || router.routeRegisters != 0 || router.routeUnregisters != 0
	router.mu.RUnlock()
	if hasResources {
		if err := router.assertClean(); err != nil {
			return fmt.Errorf("previous Work routing provider iteration is not clean: %w", err)
		}
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	router.routes = make(map[string]workRoutingProviderRoute)
	router.routeResources = make(map[string]*workRoutingRouteResource)
	router.routeRegisters = 0
	router.routeUnregisters = 0
	return nil
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
	runners := make(map[*workRoutingScenarioCommandRunner]struct{}, len(router.routeResources))
	for _, resource := range router.routeResources {
		runners[resource.runner] = struct{}{}
	}
	router.mu.RUnlock()
	active := 0
	for runner := range runners {
		active += runner.activeCallCount()
	}
	return active
}

func (router *workRoutingProviderCommandRunner) assertClean() error {
	router.mu.RLock()
	routeResources := make([]workRoutingRouteResource, 0, len(router.routeResources))
	for _, resource := range router.routeResources {
		routeResources = append(routeResources, workRoutingRouteResource{
			scenarioID:      resource.scenarioID,
			selectors:       append([]string(nil), resource.selectors...),
			runner:          resource.runner,
			unregisterCount: resource.unregisterCount,
		})
	}
	routeRegisters := router.routeRegisters
	routeUnregisters := router.routeUnregisters
	activeSelectors := len(router.routes)
	router.mu.RUnlock()

	var errs []error
	if len(routeResources) == 0 {
		errs = append(errs, fmt.Errorf("owner=%q resource=provider-route count=0 want at least one route", "work-routing-package"))
	}
	if routeRegisters != len(routeResources) {
		errs = append(errs, fmt.Errorf("owner=%q resource=provider-route registrations=%d resources=%d", "work-routing-package", routeRegisters, len(routeResources)))
	}
	if routeUnregisters != len(routeResources) {
		errs = append(errs, fmt.Errorf("owner=%q resource=provider-route unregistrations=%d registrations=%d", "work-routing-package", routeUnregisters, routeRegisters))
	}
	if activeSelectors != 0 {
		errs = append(errs, fmt.Errorf("owner=%q resource=provider-route active selectors=%d want zero", "work-routing-package", activeSelectors))
	}
	runners := make(map[*workRoutingScenarioCommandRunner]struct{}, len(routeResources))
	for _, resource := range routeResources {
		if resource.unregisterCount != 1 {
			errs = append(errs, fmt.Errorf("owner=%q resource=provider-route unregister count=%d want exactly once", resource.scenarioID, resource.unregisterCount))
		}
		if resource.runner == nil {
			errs = append(errs, fmt.Errorf("owner=%q resource=provider-route has no command runner", resource.scenarioID))
			continue
		}
		runners[resource.runner] = struct{}{}
	}
	for runner := range runners {
		starts, closes, active := runner.callStats()
		if starts != closes {
			errs = append(errs, fmt.Errorf("owner=%q resource=controlled-call starts=%d closes=%d", runner.name, starts, closes))
		}
		if active != 0 {
			errs = append(errs, fmt.Errorf("owner=%q resource=controlled-call active=%d want zero", runner.name, active))
		}
	}
	return errors.Join(errs...)
}

func (router *workRoutingProviderCommandRunner) summary() string {
	router.mu.RLock()
	routeRegisters := router.routeRegisters
	routeUnregisters := router.routeUnregisters
	activeSelectors := len(router.routes)
	router.mu.RUnlock()
	return fmt.Sprintf(
		"routes-registered=%d routes-unregistered=%d active-route-selectors=%d active-controlled-calls=%d",
		routeRegisters,
		routeUnregisters,
		activeSelectors,
		router.activeCallCount(),
	)
}
