package server_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	c06SharedHTTPFixtureTimeout = 15 * time.Second
	c06SharedHTTPStopTimeout    = 10 * time.Second
)

// c06SharedHTTPFixture owns the one root-built process and HTTP transport used
// by the eligible HTTP behavior cells. Scenario roots, sessions, and command
// routes remain local to each test while immutable production wiring is reused.
type c06SharedHTTPFixture struct {
	rootDir    string
	factoryDir string
	baseURL    string
	process    support.ApplicationProcess
	command    *c06SharedHTTPProcessCommand
	api        *support.ProcessAPIServer
	apiStopped <-chan struct{}
	provider   *c06SharedHTTPProviderRouter
	ledger     *c06SharedHTTPLedger
}

var c06SharedHTTPFixtureState struct {
	sync.Mutex
	fixture *c06SharedHTTPFixture
}

// TestMain owns the package-scoped process because a test cleanup would stop
// the server when the first test using the fixture finished.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeC06SharedHTTPFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "c06 shared HTTP fixture cleanup failed: %v\n", err)
		exitCode = 1
	}
	if err := c06AssertIsolatedLifecycleClean(); err != nil {
		fmt.Fprintf(os.Stderr, "c06 isolated lifecycle cleanup failed: %v\n", err)
		exitCode = 1
	}
	fmt.Fprintf(os.Stderr, "%s\n", c06IsolatedLifecycle.summary())
	os.Exit(exitCode)
}

func c06SharedHTTPServer(t testing.TB) *c06SharedHTTPFixture {
	t.Helper()

	c06SharedHTTPFixtureState.Lock()
	defer c06SharedHTTPFixtureState.Unlock()
	if c06SharedHTTPFixtureState.fixture != nil {
		return c06SharedHTTPFixtureState.fixture
	}

	fixture, err := newC06SharedHTTPFixture(t)
	if err != nil {
		t.Fatalf("start c06 shared HTTP fixture: %v", err)
	}
	c06SharedHTTPFixtureState.fixture = fixture
	return fixture
}

func newC06SharedHTTPFixture(t testing.TB) (*c06SharedHTTPFixture, error) {
	t.Helper()

	rootDir, err := os.MkdirTemp("", "c06-http-server-package-")
	if err != nil {
		return nil, fmt.Errorf("create package root: %w", err)
	}
	keepRoot := false
	defer func() {
		if !keepRoot {
			_ = os.RemoveAll(rootDir)
		}
	}()

	test := t.(*testing.T)
	sourceDir := support.ScaffoldFactory(test, startupShutdownTestFactoryConfig())
	factoryDir := filepath.Join(rootDir, "host")
	if err := copyC06Directory(sourceDir, factoryDir); err != nil {
		return nil, fmt.Errorf("copy host Factory: %w", err)
	}
	installRoutingReachabilityModelWorker(test, factoryDir)
	support.WriteAgentConfig(
		test,
		factoryDir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	homeDir := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return nil, fmt.Errorf("create package home: %w", err)
	}

	api := support.NewProcessAPIServer()
	apiStopped := make(chan struct{})
	var apiStopOnce sync.Once
	provider := newC06SharedHTTPProviderRouter()
	ledger := newC06SharedHTTPLedger()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			ledger.recordProcessStart()
			startErr := api.Start(ctx, request)
			apiStopOnce.Do(func() { close(apiStopped) })
			return startErr
		},
		ProviderCommandRunner: provider,
	})
	if err != nil {
		return nil, fmt.Errorf("build root process: %w", err)
	}

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", factoryDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = c06SharedHTTPEnvironment(homeDir)
	inputs.Input.WorkingDirectory = factoryDir
	command := startC06SharedHTTPProcess(process, inputs.Input)

	baseURL, err := api.WaitForBaseURL(c06SharedHTTPFixtureTimeout)
	if err != nil {
		_ = command.stop()
		closeCtx, cancel := context.WithTimeout(context.Background(), c06SharedHTTPStopTimeout)
		_ = process.Close(closeCtx)
		cancel()
		return nil, fmt.Errorf("wait for API server: %w", err)
	}
	support.WaitForStatus(t, baseURL, c06SharedHTTPFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	keepRoot = true
	return &c06SharedHTTPFixture{
		rootDir:    rootDir,
		factoryDir: factoryDir,
		baseURL:    baseURL,
		process:    process,
		command:    command,
		api:        api,
		apiStopped: apiStopped,
		provider:   provider,
		ledger:     ledger,
	}, nil
}

func c06SharedHTTPEnvironment(homeDir string) []string {
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

func (fixture *c06SharedHTTPFixture) URL() string {
	if fixture == nil {
		return ""
	}
	return fixture.baseURL
}

func scaffoldC06HTTPFactory(t testing.TB, cfg map[string]any) string {
	t.Helper()
	test := t.(*testing.T)
	dir := support.ScaffoldFactory(test, cfg)
	support.WriteAgentConfig(
		test,
		dir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	return dir
}

func (fixture *c06SharedHTTPFixture) newScenario(
	t testing.TB,
	id string,
	factoryDirs ...string,
) *c06SharedHTTPScenario {
	t.Helper()
	if fixture == nil {
		t.Fatal("c06 shared HTTP fixture is unavailable")
	}
	if err := fixture.ledger.registerScenario(id, factoryDirs); err != nil {
		t.Fatal(err)
	}
	scenario := &c06SharedHTTPScenario{
		fixture:     fixture,
		id:          id,
		factoryDirs: append([]string(nil), factoryDirs...),
	}
	t.Cleanup(func() { scenario.cleanup(t) })
	return scenario
}

type c06SharedHTTPScenario struct {
	fixture     *c06SharedHTTPFixture
	id          string
	factoryDirs []string
	sessionIDs  []string
	route       *c06SharedHTTPRoute
	streams     atomic.Int32
	cleanupOnce sync.Once
}

func (scenario *c06SharedHTTPScenario) URL() string {
	if scenario == nil || scenario.fixture == nil {
		return ""
	}
	return scenario.fixture.URL()
}

func (scenario *c06SharedHTTPScenario) openSession(t testing.TB, factoryDir string) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, scenario.URL(), factoryDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatal("opened c06 Factory Session has no ID")
	}
	if opened.Session.IsDefault {
		t.Fatalf("opened c06 Factory Session %q is the default session", opened.Session.Id)
	}
	if err := scenario.fixture.ledger.registerSession(scenario.id, opened.Session.Id); err != nil {
		t.Fatal(err)
	}
	scenario.sessionIDs = append(scenario.sessionIDs, opened.Session.Id)
	return opened.Session.Id
}

func (scenario *c06SharedHTTPScenario) trackSession(t testing.TB, sessionID string) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("cannot track an empty c06 Factory Session ID")
	}
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("cannot track the default c06 Factory Session")
	}
	if err := scenario.fixture.ledger.registerSession(scenario.id, sessionID); err != nil {
		t.Fatal(err)
	}
	scenario.sessionIDs = append(scenario.sessionIDs, sessionID)
}

func (scenario *c06SharedHTTPScenario) registerRunner(
	t testing.TB,
	selectors []string,
	runner platformprocess.CommandRunner,
) {
	t.Helper()
	if scenario.route != nil {
		t.Fatal("c06 scenario command runner already registered")
	}
	route, err := scenario.fixture.provider.register(scenario.id, selectors, runner)
	if err != nil {
		t.Fatal(err)
	}
	scenario.route = route
}

func (scenario *c06SharedHTTPScenario) trackingTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return c06SharedHTTPTrackingRoundTripper{
		base:     base,
		streams:  &scenario.streams,
		streamed: c06IsStreamingEndpoint,
	}
}

func (scenario *c06SharedHTTPScenario) cleanup(t testing.TB) {
	t.Helper()
	scenario.cleanupOnce.Do(func() {
		for index := len(scenario.sessionIDs) - 1; index >= 0; index-- {
			sessionID := scenario.sessionIDs[index]
			support.CloseFactorySessionAt(t, scenario.URL(), sessionID)
			if err := assertC06FactorySessionAbsent(scenario.URL(), sessionID); err != nil {
				t.Errorf("c06 session %q cleanup: %v", sessionID, err)
			}
			if err := scenario.fixture.ledger.closeSession(sessionID); err != nil {
				t.Errorf("record c06 session %q cleanup: %v", sessionID, err)
			}
		}
		if scenario.route != nil {
			scenario.fixture.provider.unregister(scenario.id)
			if active := scenario.route.active.Load(); active != 0 {
				t.Errorf("c06 scenario %q active provider calls after cleanup = %d", scenario.id, active)
			}
		}
		if active := scenario.fixture.provider.activeCallCount(); active != 0 {
			t.Errorf("c06 scenario %q active provider calls after unregister = %d", scenario.id, active)
		}
		if routes := scenario.fixture.provider.routeCount(); routes != 0 {
			t.Errorf("c06 scenario %q provider registrations after cleanup = %d", scenario.id, routes)
		}
		if active := scenario.streams.Load(); active != 0 {
			t.Errorf("c06 scenario %q response streams after cleanup = %d", scenario.id, active)
		}
		for _, factoryDir := range scenario.factoryDirs {
			if err := os.RemoveAll(factoryDir); err != nil {
				t.Errorf("remove c06 scenario Factory root %q: %v", factoryDir, err)
				continue
			}
			if _, err := os.Stat(factoryDir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("c06 scenario Factory root %q remains after cleanup: %v", factoryDir, err)
			}
		}
		if err := scenario.fixture.ledger.closeScenario(scenario.id); err != nil {
			t.Errorf("record c06 scenario %q cleanup: %v", scenario.id, err)
		}
	})
}

func assertC06FactorySessionAbsent(baseURL, sessionID string) error {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build GET %s: %w", endpoint, err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("GET %s status = %d, want 404: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var notFound factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&notFound); err != nil {
		return fmt.Errorf("decode GET %s: %w", endpoint, err)
	}
	if notFound.Family != factoryapi.ErrorFamilyNotFound || notFound.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		return fmt.Errorf("GET %s response = %#v, want typed NOT_FOUND", endpoint, notFound)
	}
	return nil
}

func closeC06SharedHTTPFixture() error {
	c06SharedHTTPFixtureState.Lock()
	fixture := c06SharedHTTPFixtureState.fixture
	c06SharedHTTPFixtureState.Unlock()
	if fixture == nil {
		return nil
	}

	var errs []error
	if err := fixture.command.stop(); err != nil {
		errs = append(errs, err)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), c06SharedHTTPStopTimeout)
	if err := fixture.process.Close(closeCtx); err != nil {
		errs = append(errs, fmt.Errorf("close root process: %w", err))
	}
	cancel()
	select {
	case <-fixture.apiStopped:
	case <-time.After(c06SharedHTTPStopTimeout):
		errs = append(errs, errors.New("shared HTTP API server did not stop"))
	}
	if active := fixture.provider.activeCallCount(); active != 0 {
		errs = append(errs, fmt.Errorf("c06 active provider calls after package cleanup = %d", active))
	}
	if routes := fixture.provider.routeCount(); routes != 0 {
		errs = append(errs, fmt.Errorf("c06 provider registrations after package cleanup = %d", routes))
	}
	if err := fixture.ledger.assertClean(); err != nil {
		errs = append(errs, err)
	}
	if err := assertC06ListenerClosed(fixture.baseURL); err != nil {
		errs = append(errs, err)
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove c06 package root %q: %w", fixture.rootDir, err))
	} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("c06 package root %q remains: %v", fixture.rootDir, err))
	}
	return errors.Join(errs...)
}

func assertC06ListenerClosed(baseURL string) error {
	client := http.Client{Timeout: 500 * time.Millisecond}
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err == nil {
		response.Body.Close()
		return fmt.Errorf("shared HTTP listener still served /status")
	}
	return nil
}

type c06SharedHTTPProcessCommand struct {
	cancel context.CancelFunc
	done   chan error
}

func startC06SharedHTTPProcess(
	process support.ApplicationProcess,
	input root.Input,
) *c06SharedHTTPProcessCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &c06SharedHTTPProcessCommand{
		cancel: cancel,
		done:   make(chan error, 1),
	}
	go func() {
		command.done <- process.Execute(input)
	}()
	return command
}

func (command *c06SharedHTTPProcessCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	select {
	case err := <-command.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop shared HTTP process command: %w", err)
		}
		return nil
	case <-time.After(c06SharedHTTPStopTimeout):
		return errors.New("timed out waiting for shared HTTP process command shutdown")
	}
}

type c06SharedHTTPProviderRouter struct {
	mu      sync.RWMutex
	routes  map[string]*c06SharedHTTPRoute
	handles map[string]*c06SharedHTTPRoute
}

type c06SharedHTTPRoute struct {
	scenarioID string
	runner     platformprocess.CommandRunner
	active     atomic.Int32
	calls      atomic.Int32
}

func newC06SharedHTTPProviderRouter() *c06SharedHTTPProviderRouter {
	return &c06SharedHTTPProviderRouter{
		routes:  make(map[string]*c06SharedHTTPRoute),
		handles: make(map[string]*c06SharedHTTPRoute),
	}
}

func (router *c06SharedHTTPProviderRouter) register(
	scenarioID string,
	selectors []string,
	runner platformprocess.CommandRunner,
) (*c06SharedHTTPRoute, error) {
	if strings.TrimSpace(scenarioID) == "" {
		return nil, errors.New("c06 provider route scenario ID is required")
	}
	if runner == nil {
		return nil, fmt.Errorf("c06 provider route %q has no runner", scenarioID)
	}
	normalized := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		cleaned, err := c06NormalizeRouteSelector(selector)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, cleaned)
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("c06 provider route %q has no selectors", scenarioID)
	}
	route := &c06SharedHTTPRoute{scenarioID: scenarioID, runner: runner}
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, selector := range normalized {
		if existing, ok := router.routes[selector]; ok && existing.scenarioID != scenarioID {
			return nil, fmt.Errorf("c06 provider selector %q is already registered", selector)
		}
	}
	for _, selector := range normalized {
		router.routes[selector] = route
	}
	router.handles[scenarioID] = route
	return route, nil
}

func (router *c06SharedHTTPProviderRouter) unregister(scenarioID string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for selector, route := range router.routes {
		if route.scenarioID == scenarioID {
			delete(router.routes, selector)
		}
	}
	delete(router.handles, scenarioID)
}

func (router *c06SharedHTTPProviderRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir, err := c06NormalizeRouteSelector(request.WorkDir)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.RLock()
	matched := make(map[*c06SharedHTTPRoute]struct{})
	for selector, route := range router.routes {
		if c06RouteContains(selector, workDir) {
			matched[route] = struct{}{}
		}
	}
	router.mu.RUnlock()
	if len(matched) != 1 {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"c06 provider selector matched %d scenarios for work directory %q",
			len(matched), filepath.Base(workDir),
		)
	}
	for route := range matched {
		route.active.Add(1)
		route.calls.Add(1)
		defer route.active.Add(-1)
		return route.runner.Run(ctx, request)
	}
	return platformprocess.CommandResult{}, errors.New("c06 provider selector resolution produced no route")
}

func (router *c06SharedHTTPProviderRouter) activeCallCount() int {
	router.mu.RLock()
	defer router.mu.RUnlock()
	active := 0
	for _, route := range router.handles {
		active += int(route.active.Load())
	}
	return active
}

func (router *c06SharedHTTPProviderRouter) routeCount() int {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.routes)
}

func c06NormalizeRouteSelector(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("c06 provider route selector is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("normalize c06 provider selector: %w", err)
	}
	cleaned := filepath.Clean(abs)
	if filepath.Separator == '\\' {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned, nil
}

func c06RouteContains(selector, workDir string) bool {
	if selector == workDir {
		return true
	}
	return strings.HasPrefix(workDir, selector+string(filepath.Separator))
}

type c06SharedHTTPLedger struct {
	mu        sync.Mutex
	processes int
	scenarios map[string]*c06ScenarioLedgerEntry
	sessions  map[string]*c06SessionLedgerEntry
}

type c06ScenarioLedgerEntry struct {
	closed      bool
	rootRemoved bool
}

type c06SessionLedgerEntry struct {
	scenarioID string
	closed     bool
	absent     bool
}

func newC06SharedHTTPLedger() *c06SharedHTTPLedger {
	return &c06SharedHTTPLedger{
		scenarios: make(map[string]*c06ScenarioLedgerEntry),
		sessions:  make(map[string]*c06SessionLedgerEntry),
	}
}

func (ledger *c06SharedHTTPLedger) recordProcessStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processes++
}

func (ledger *c06SharedHTTPLedger) registerScenario(id string, factoryDirs []string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if strings.TrimSpace(id) == "" {
		return errors.New("c06 scenario ID is required")
	}
	if existing, exists := ledger.scenarios[id]; exists {
		if !existing.closed || !existing.rootRemoved {
			return fmt.Errorf("c06 scenario %q is already registered", id)
		}
		// go test -count=N repeats the package in one test process. Reuse an
		// ID only after the prior iteration proved its scenario cleanup.
		delete(ledger.scenarios, id)
	}
	if len(factoryDirs) == 0 {
		return fmt.Errorf("c06 scenario %q has no Factory roots", id)
	}
	for _, factoryDir := range factoryDirs {
		if !filepath.IsAbs(factoryDir) {
			return fmt.Errorf("c06 scenario Factory root %q is not absolute", factoryDir)
		}
	}
	ledger.scenarios[id] = &c06ScenarioLedgerEntry{
		closed:      false,
		rootRemoved: false,
	}
	return nil
}

func (ledger *c06SharedHTTPLedger) registerSession(scenarioID, sessionID string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if sessionID == factorysessions.DefaultSessionID || strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("c06 session %q is empty or default", sessionID)
	}
	if _, exists := ledger.sessions[sessionID]; exists {
		return fmt.Errorf("c06 session %q is not unique", sessionID)
	}
	if _, exists := ledger.scenarios[scenarioID]; !exists {
		return fmt.Errorf("c06 session %q belongs to unknown scenario %q", sessionID, scenarioID)
	}
	ledger.sessions[sessionID] = &c06SessionLedgerEntry{scenarioID: scenarioID}
	return nil
}

func (ledger *c06SharedHTTPLedger) closeSession(sessionID string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	session, exists := ledger.sessions[sessionID]
	if !exists {
		return fmt.Errorf("c06 session %q was not registered", sessionID)
	}
	if session.closed {
		return fmt.Errorf("c06 session %q was closed more than once", sessionID)
	}
	session.closed = true
	session.absent = true
	return nil
}

func (ledger *c06SharedHTTPLedger) closeScenario(id string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	scenario, exists := ledger.scenarios[id]
	if !exists {
		return fmt.Errorf("c06 scenario %q was not registered", id)
	}
	if scenario.closed {
		return fmt.Errorf("c06 scenario %q was closed more than once", id)
	}
	scenario.closed = true
	scenario.rootRemoved = true
	return nil
}

func (ledger *c06SharedHTTPLedger) assertClean() error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	var errs []error
	if ledger.processes != 1 {
		errs = append(errs, fmt.Errorf("c06 process starts = %d, want exactly one shared API process", ledger.processes))
	}
	if len(ledger.scenarios) == 0 {
		errs = append(errs, errors.New("c06 shared HTTP ledger has no scenarios"))
	}
	for id, scenario := range ledger.scenarios {
		if !scenario.closed || !scenario.rootRemoved {
			errs = append(errs, fmt.Errorf("c06 scenario %q cleanup = closed:%t root_removed:%t", id, scenario.closed, scenario.rootRemoved))
		}
	}
	for id, session := range ledger.sessions {
		if !session.closed || !session.absent {
			errs = append(errs, fmt.Errorf("c06 session %q cleanup = closed:%t absent:%t", id, session.closed, session.absent))
		}
	}
	return errors.Join(errs...)
}

type c06SharedHTTPTrackingRoundTripper struct {
	base     http.RoundTripper
	streams  *atomic.Int32
	streamed func(string) bool
}

func (transport c06SharedHTTPTrackingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil || response == nil || !transport.streamed(request.URL.Path) {
		return response, err
	}
	transport.streams.Add(1)
	response.Body = &c06SharedHTTPTrackedBody{
		ReadCloser: response.Body,
		streams:    transport.streams,
	}
	return response, nil
}

type c06SharedHTTPTrackedBody struct {
	io.ReadCloser
	streams   *atomic.Int32
	closeOnce sync.Once
}

func (body *c06SharedHTTPTrackedBody) Read(p []byte) (int, error) {
	count, err := body.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) {
		body.release()
	}
	return count, err
}

func (body *c06SharedHTTPTrackedBody) Close() error {
	err := body.ReadCloser.Close()
	body.release()
	return err
}

func (body *c06SharedHTTPTrackedBody) release() {
	body.closeOnce.Do(func() { body.streams.Add(-1) })
}

func c06IsStreamingEndpoint(path string) bool {
	return strings.HasSuffix(path, "/events") || strings.HasSuffix(path, "/response-events")
}

func copyC06Directory(sourceDir, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(targetDir, 0o755)
		}
		targetPath := filepath.Join(targetDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0o644)
	})
}
