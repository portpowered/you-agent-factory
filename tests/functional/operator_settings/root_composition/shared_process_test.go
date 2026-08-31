package root_composition_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedOperatorSettingsProcessTimeout = 15 * time.Second

var sharedOperatorSettingsFixtureState struct {
	sync.Once
	fixture *sharedOperatorSettingsFixture
	err     error
}

// sharedOperatorSettingsFixture owns the one production-composed process for
// this package. Invocation-local paths select a route; only the home, session
// ID, and settings ID callbacks use the bounded lease because their contracts
// do not carry a selector.
type sharedOperatorSettingsFixture struct {
	process support.ApplicationProcess
	router  *operatorSettingsEffectRouter

	construction operatorSettingsEffectSnapshot

	sessionMu     sync.Mutex
	activeSession map[string]string
	opened        atomic.Int64
	closed        atomic.Int64
}

type operatorSettingsEffectSnapshot struct {
	fileSystemCalls      int64
	createTemporaryCalls int64
	operatorIDCalls      int64
	sessionIDCalls       int64
	apiStartCalls        int64
	providerRunnerCalls  int64
	unmatchedRouteCalls  int64
}

type operatorSettingsEffectRouter struct {
	mu     sync.Mutex
	routes map[string]*operatorSettingsEffectRoute
	active *operatorSettingsEffectRoute
	lease  chan struct{}
	nextID atomic.Uint64

	fileSystemCalls      atomic.Int64
	createTemporaryCalls atomic.Int64
	operatorIDCalls      atomic.Int64
	sessionIDCalls       atomic.Int64
	apiStartCalls        atomic.Int64
	providerRunnerCalls  atomic.Int64
	unmatchedRouteCalls  atomic.Int64
}

type operatorSettingsProviderRunner func(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error)

func (runner operatorSettingsProviderRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return runner(ctx, request)
}

type operatorSettingsEffectRoute struct {
	label          string
	homeDir        string
	workingDir     string
	generatedID    string
	providerRunner platformprocess.CommandRunner
	apiServer      *support.ProcessAPIServer

	mu        sync.Mutex
	closed    bool
	temporary map[string]struct{}
}

type sharedOperatorSettingsSession struct {
	fixture *sharedOperatorSettingsFixture
	route   *operatorSettingsEffectRoute
	baseURL string
	session string
	command *support.ProcessCommand
	tracked bool

	closeOnce sync.Once
}

func ensureSharedOperatorSettingsFixture(t testing.TB) *sharedOperatorSettingsFixture {
	t.Helper()
	sharedOperatorSettingsFixtureState.Do(func() {
		sharedOperatorSettingsFixtureState.fixture, sharedOperatorSettingsFixtureState.err =
			newSharedOperatorSettingsFixture()
	})
	if sharedOperatorSettingsFixtureState.err != nil {
		t.Fatalf("initialize shared Operator Settings fixture: %v", sharedOperatorSettingsFixtureState.err)
	}
	return sharedOperatorSettingsFixtureState.fixture
}

func newSharedOperatorSettingsFixture() (_ *sharedOperatorSettingsFixture, err error) {
	router := newOperatorSettingsEffectRouter()
	process, buildErr := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		OperatorSettingsFileSystem:          router,
		OperatorSettingsCreateTemporaryFile: router.createTemporaryFile,
		OperatorSettingsIDGenerator:         router.generateOperatorID,

		FactorySessionResolveHomeDirectory: router.resolveHome,
		FactorySessionIDGenerator:          router.generateSessionID,
		APIServerStarter:                   router.startAPIServer,
		ProviderCommandRunner:              operatorSettingsProviderRunner(router.runProvider),
	})
	if buildErr != nil {
		return nil, fmt.Errorf("compose shared Operator Settings process: %w", buildErr)
	}
	if process == nil {
		return nil, errors.New("compose shared Operator Settings process: process is nil")
	}

	return &sharedOperatorSettingsFixture{
		process:       process,
		router:        router,
		construction:  router.snapshot(),
		activeSession: make(map[string]string),
	}, nil
}

func newOperatorSettingsEffectRouter() *operatorSettingsEffectRouter {
	return &operatorSettingsEffectRouter{
		routes: make(map[string]*operatorSettingsEffectRoute),
		lease:  make(chan struct{}, 1),
	}
}

func (router *operatorSettingsEffectRouter) snapshot() operatorSettingsEffectSnapshot {
	return operatorSettingsEffectSnapshot{
		fileSystemCalls:      router.fileSystemCalls.Load(),
		createTemporaryCalls: router.createTemporaryCalls.Load(),
		operatorIDCalls:      router.operatorIDCalls.Load(),
		sessionIDCalls:       router.sessionIDCalls.Load(),
		apiStartCalls:        router.apiStartCalls.Load(),
		providerRunnerCalls:  router.providerRunnerCalls.Load(),
		unmatchedRouteCalls:  router.unmatchedRouteCalls.Load(),
	}
}

func (router *operatorSettingsEffectRouter) register(
	label, homeDir, workingDir, generatedID string,
	providerRunner platformprocess.CommandRunner,
) (*operatorSettingsEffectRoute, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, errors.New("register Operator Settings route: label is required")
	}
	homeDir, err := cleanOperatorSettingsPath(homeDir)
	if err != nil {
		return nil, fmt.Errorf("register Operator Settings route %q: home directory: %w", label, err)
	}
	workingDir, err = cleanOperatorSettingsPath(workingDir)
	if err != nil {
		return nil, fmt.Errorf("register Operator Settings route %q: working directory: %w", label, err)
	}
	generatedID = strings.TrimSpace(generatedID)
	if generatedID == "" {
		return nil, fmt.Errorf("register Operator Settings route %q: generated ID is required", label)
	}

	route := &operatorSettingsEffectRoute{
		label: label, homeDir: homeDir, workingDir: workingDir,
		generatedID:    generatedID,
		providerRunner: providerRunner,
		apiServer:      support.NewProcessAPIServer(),
		temporary:      make(map[string]struct{}),
	}

	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[label]; exists {
		return nil, fmt.Errorf("register Operator Settings route %q: duplicate label", label)
	}
	for _, existing := range router.routes {
		if operatorSettingsPathsOverlap(route, existing) {
			return nil, fmt.Errorf(
				"register Operator Settings route %q: selector overlaps route %q",
				label, existing.label,
			)
		}
	}
	router.routes[label] = route
	return route, nil
}

func (router *operatorSettingsEffectRouter) acquire(
	ctx context.Context,
	route *operatorSettingsEffectRoute,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("acquire Operator Settings route lease: %w", ctx.Err())
	default:
	}
	select {
	case router.lease <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("acquire Operator Settings route lease: %w", ctx.Err())
	}
	select {
	case <-ctx.Done():
		<-router.lease
		return fmt.Errorf("acquire Operator Settings route lease: %w", ctx.Err())
	default:
	}

	router.mu.Lock()
	if route == nil || route.isClosed() {
		router.mu.Unlock()
		<-router.lease
		return errors.New("acquire Operator Settings route lease: route is closed")
	}
	if router.active != nil {
		router.mu.Unlock()
		<-router.lease
		return errors.New("acquire Operator Settings route lease: another route is active")
	}
	router.active = route
	router.mu.Unlock()
	return nil
}

func (router *operatorSettingsEffectRouter) release(route *operatorSettingsEffectRoute) error {
	router.mu.Lock()
	if router.active == nil {
		router.mu.Unlock()
		return nil
	}
	if router.active != route {
		active := router.active.label
		router.mu.Unlock()
		return fmt.Errorf("release Operator Settings route lease: active route is %q", active)
	}
	router.active = nil
	router.mu.Unlock()
	<-router.lease
	return nil
}

func (router *operatorSettingsEffectRouter) unregister(route *operatorSettingsEffectRoute) error {
	if route == nil {
		return nil
	}
	router.mu.Lock()
	if route.isClosed() {
		router.mu.Unlock()
		return nil
	}
	if router.active == route {
		label := route.label
		router.mu.Unlock()
		return fmt.Errorf("unregister Operator Settings route %q: lease is still active", label)
	}
	delete(router.routes, route.label)
	router.mu.Unlock()
	route.mu.Lock()
	route.closed = true
	route.mu.Unlock()

	return route.removeTemporaryFiles()
}

func (router *operatorSettingsEffectRouter) routeForPath(path string) (*operatorSettingsEffectRoute, error) {
	path, err := cleanOperatorSettingsPath(path)
	if err != nil {
		return nil, router.unmatchedRouteError(path, err)
	}

	router.mu.Lock()
	defer router.mu.Unlock()
	var matched *operatorSettingsEffectRoute
	for _, route := range router.routes {
		if route.isClosed() || !operatorSettingsPathMatches(route, path) {
			continue
		}
		if matched != nil && matched != route {
			return nil, router.unmatchedRouteError(path, errors.New("multiple routes match selector"))
		}
		matched = route
	}
	if matched == nil {
		return nil, router.unmatchedRouteError(path, errors.New("no route is registered"))
	}
	return matched, nil
}

func (router *operatorSettingsEffectRouter) activeRoute() (*operatorSettingsEffectRoute, error) {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.active == nil || router.active.isClosed() {
		return nil, errors.New("no route lease is active")
	}
	return router.active, nil
}

func (router *operatorSettingsEffectRouter) unmatchedRouteError(selector string, cause error) error {
	router.unmatchedRouteCalls.Add(1)
	return fmt.Errorf("Operator Settings route for %q is unavailable: %w", selector, cause)
}

func (router *operatorSettingsEffectRouter) resolveHome() (string, error) {
	route, err := router.activeRoute()
	if err != nil {
		return "", router.unmatchedRouteError("home directory", err)
	}
	return route.homeDir, nil
}

func (router *operatorSettingsEffectRouter) generateOperatorID() string {
	route, err := router.activeRoute()
	if err != nil {
		router.unmatchedRouteError("operator settings ID", err)
		return ""
	}
	router.operatorIDCalls.Add(1)
	return route.generatedID
}

func (router *operatorSettingsEffectRouter) generateSessionID() string {
	if _, err := router.activeRoute(); err != nil {
		router.unmatchedRouteError("Factory Session ID", err)
		return ""
	}
	router.sessionIDCalls.Add(1)
	return fmt.Sprintf("operator-settings-session-%d", router.nextID.Add(1))
}

func (router *operatorSettingsEffectRouter) routeForEffectPath(path string) (*operatorSettingsEffectRoute, error) {
	route, err := router.routeForPath(path)
	if err != nil {
		return nil, err
	}
	active, err := router.activeRoute()
	if err != nil {
		return nil, router.unmatchedRouteError(path, err)
	}
	if active != route {
		return nil, router.unmatchedRouteError(path, fmt.Errorf(
			"selector belongs to route %q while route %q is leased",
			route.label, active.label,
		))
	}
	return route, nil
}

func (router *operatorSettingsEffectRouter) ReadFile(path string) ([]byte, error) {
	_, err := router.routeForEffectPath(path)
	if err != nil {
		return nil, err
	}
	router.fileSystemCalls.Add(1)
	return os.ReadFile(filepath.Clean(path))
}

func (router *operatorSettingsEffectRouter) MkdirAll(path string, mode fs.FileMode) error {
	if _, err := router.routeForEffectPath(path); err != nil {
		return err
	}
	router.fileSystemCalls.Add(1)
	return os.MkdirAll(filepath.Clean(path), mode)
}

func (router *operatorSettingsEffectRouter) Remove(path string) error {
	if _, err := router.routeForEffectPath(path); err != nil {
		return err
	}
	router.fileSystemCalls.Add(1)
	return os.Remove(filepath.Clean(path))
}

func (router *operatorSettingsEffectRouter) Chmod(path string, mode fs.FileMode) error {
	if _, err := router.routeForEffectPath(path); err != nil {
		return err
	}
	router.fileSystemCalls.Add(1)
	return os.Chmod(filepath.Clean(path), mode)
}

func (router *operatorSettingsEffectRouter) Rename(oldPath, newPath string) error {
	oldRoute, err := router.routeForEffectPath(oldPath)
	if err != nil {
		return err
	}
	newRoute, err := router.routeForEffectPath(newPath)
	if err != nil {
		return err
	}
	if oldRoute != newRoute {
		return router.unmatchedRouteError(newPath, errors.New("cross-route rename is denied"))
	}
	router.fileSystemCalls.Add(1)
	return os.Rename(filepath.Clean(oldPath), filepath.Clean(newPath))
}

func (router *operatorSettingsEffectRouter) createTemporaryFile(
	directory, pattern string,
) (operatorsettings.TemporaryFile, error) {
	route, err := router.routeForEffectPath(directory)
	if err != nil {
		return nil, err
	}
	temporary, err := os.CreateTemp(filepath.Clean(directory), pattern)
	if err != nil {
		return nil, err
	}
	route.mu.Lock()
	if route.closed {
		route.mu.Unlock()
		_ = temporary.Close()
		_ = os.Remove(temporary.Name())
		return nil, router.unmatchedRouteError(directory, errors.New("route closed while creating temporary file"))
	}
	route.temporary[temporary.Name()] = struct{}{}
	route.mu.Unlock()
	router.createTemporaryCalls.Add(1)
	return temporary, nil
}

func (router *operatorSettingsEffectRouter) startAPIServer(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	selector := processcontract.WorkingDirectory(ctx)
	route, err := router.routeForEffectPath(selector)
	if err != nil {
		return fmt.Errorf("start routed Operator Settings API server: %w", err)
	}
	router.apiStartCalls.Add(1)
	return route.apiServer.Start(ctx, request)
}

func (router *operatorSettingsEffectRouter) runProvider(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := request.WorkDir
	if strings.TrimSpace(selector) == "" {
		selector = operatorSettingsEnvironmentValue(request.Env, "HOME")
	}
	route, err := router.routeForEffectPath(selector)
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("run routed provider command: %w", err)
	}
	if route.providerRunner == nil {
		return platformprocess.CommandResult{}, router.unmatchedRouteError(
			selector, fmt.Errorf("provider runner is not configured for route %q", route.label),
		)
	}
	router.providerRunnerCalls.Add(1)
	return route.providerRunner.Run(ctx, request)
}

func operatorSettingsEnvironmentValue(environment []string, name string) string {
	for index := len(environment) - 1; index >= 0; index-- {
		key, value, found := strings.Cut(environment[index], "=")
		if found && strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func cleanOperatorSettingsPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func operatorSettingsPathsOverlap(first, second *operatorSettingsEffectRoute) bool {
	for _, firstPath := range []string{first.homeDir, first.workingDir} {
		for _, secondPath := range []string{second.homeDir, second.workingDir} {
			if operatorSettingsPathWithin(firstPath, secondPath) || operatorSettingsPathWithin(secondPath, firstPath) {
				return true
			}
		}
	}
	return false
}

func operatorSettingsPathMatches(route *operatorSettingsEffectRoute, path string) bool {
	return operatorSettingsPathWithin(route.homeDir, path) || operatorSettingsPathWithin(route.workingDir, path)
}

func operatorSettingsPathWithin(rootPath, candidate string) bool {
	relative, err := filepath.Rel(rootPath, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func (route *operatorSettingsEffectRoute) removeTemporaryFiles() error {
	route.mu.Lock()
	temporary := make([]string, 0, len(route.temporary))
	for path := range route.temporary {
		temporary = append(temporary, path)
	}
	route.temporary = make(map[string]struct{})
	route.mu.Unlock()

	var errs []error
	for _, path := range temporary {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove routed temporary file %q: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

func (route *operatorSettingsEffectRoute) isClosed() bool {
	route.mu.Lock()
	defer route.mu.Unlock()
	return route.closed
}

func (fixture *sharedOperatorSettingsFixture) withFactorySession(
	t *testing.T,
	label, homeDir, workingDir, generatedID string,
	providerRunner platformprocess.CommandRunner,
	run func(string),
) {
	t.Helper()
	route, err := fixture.router.register(label, homeDir, workingDir, generatedID, providerRunner)
	if err != nil {
		t.Fatalf("register Operator Settings Factory Session route: %v", err)
	}
	if err := fixture.router.acquire(t.Context(), route); err != nil {
		_ = fixture.router.unregister(route)
		t.Fatalf("acquire Operator Settings Factory Session route: %v", err)
	}
	var command *support.ProcessCommand
	cleanupRoute := true
	defer func() {
		if cleanupRoute {
			if command != nil {
				command.Stop(t)
			}
			if err := fixture.router.release(route); err != nil {
				t.Errorf("release Operator Settings route after setup failure: %v", err)
			}
			if err := fixture.router.unregister(route); err != nil {
				t.Errorf("unregister Operator Settings route after setup failure: %v", err)
			}
		}
	}()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", workingDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = workingDir
	command = support.StartProcessCommand(t, fixture.process, inputs.Input)
	baseURL := route.apiServer.WaitForURL(t)
	support.WaitForStatus(t, baseURL, sharedOperatorSettingsProcessTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	opened := support.OpenFactorySessionAt(t, baseURL, workingDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" ||
		opened.Session.Id == factorysessions.DefaultSessionID {
		t.Fatalf("open routed Factory Session %q returned invalid session %#v", label, opened.Session)
	}

	session := &sharedOperatorSettingsSession{
		fixture: fixture,
		route:   route,
		baseURL: baseURL,
		session: opened.Session.Id,
		command: command,
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.activeSession[opened.Session.Id]; exists {
		fixture.sessionMu.Unlock()
		session.close(t)
		t.Fatalf("open routed Factory Session %q reused session ID %q", label, opened.Session.Id)
	}
	fixture.activeSession[opened.Session.Id] = label
	fixture.opened.Add(1)
	session.tracked = true
	fixture.sessionMu.Unlock()
	cleanupRoute = false

	defer func() {
		session.close(t)
	}()
	run(opened.Session.Id)
}

func (session *sharedOperatorSettingsSession) close(t testing.TB) {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		t.Helper()
		defer func() {
			if session.command != nil {
				session.command.Stop(t)
			}
			if err := session.fixture.router.release(session.route); err != nil {
				t.Errorf("release Operator Settings route for session %q: %v", session.session, err)
			}
			if err := session.fixture.router.unregister(session.route); err != nil {
				t.Errorf("unregister Operator Settings route for session %q: %v", session.session, err)
			}
			if session.tracked {
				session.fixture.sessionMu.Lock()
				delete(session.fixture.activeSession, session.session)
				session.fixture.closed.Add(1)
				session.fixture.sessionMu.Unlock()
			}
		}()
		support.CloseFactorySessionAt(t, session.baseURL, session.session)
		verifyOperatorSettingsSessionDeleted(t, session.baseURL, session.session)
	})
}

func verifyOperatorSettingsSessionDeleted(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted routed Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted routed Factory Session %q status = %d, want 404", sessionID, response.StatusCode)
	}
}

func closeSharedOperatorSettingsFixture() error {
	fixture := sharedOperatorSettingsFixtureState.fixture
	if fixture == nil {
		return nil
	}
	var errs []error
	fixture.sessionMu.Lock()
	if len(fixture.activeSession) != 0 {
		errs = append(errs, fmt.Errorf("active routed Factory Sessions after cleanup = %d", len(fixture.activeSession)))
	}
	fixture.sessionMu.Unlock()
	if got, want := fixture.opened.Load(), fixture.closed.Load(); got != want {
		errs = append(errs, fmt.Errorf("routed Factory Session opens = %d, closes = %d", got, want))
	}
	if got := fixture.router.routeCount(); got != 0 {
		errs = append(errs, fmt.Errorf("routed Operator Settings routes after cleanup = %d", got))
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), sharedOperatorSettingsProcessTimeout)
	defer cancel()
	if err := fixture.process.Close(closeCtx); err != nil {
		errs = append(errs, fmt.Errorf("close shared Operator Settings process: %w", err))
	}
	if route, err := fixture.router.activeRoute(); err == nil {
		errs = append(errs, fmt.Errorf("active routed Operator Settings route after cleanup = %q", route.label))
	}
	return errors.Join(errs...)
}

func (fixture *sharedOperatorSettingsFixture) constructionEffectSnapshot() operatorSettingsEffectSnapshot {
	if fixture == nil {
		return operatorSettingsEffectSnapshot{}
	}
	return fixture.construction
}

func (router *operatorSettingsEffectRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func assertOperatorSettingsRouteFailures(t *testing.T) {
	t.Helper()

	router := newOperatorSettingsEffectRouter()
	if _, err := router.register("", t.TempDir(), t.TempDir(), "id", nil); err == nil ||
		!strings.Contains(err.Error(), "label is required") {
		t.Fatalf("blank route registration error = %v, want deterministic label diagnostic", err)
	}
	if _, err := router.register("blank-id", t.TempDir(), t.TempDir(), "", nil); err == nil ||
		!strings.Contains(err.Error(), "generated ID is required") {
		t.Fatalf("blank generated ID registration error = %v, want deterministic ID diagnostic", err)
	}

	firstHome, firstWorking := t.TempDir(), t.TempDir()
	first, err := router.register("duplicate", firstHome, firstWorking, "id-first", nil)
	if err != nil {
		t.Fatalf("register first route: %v", err)
	}
	if _, err := router.register("duplicate", t.TempDir(), t.TempDir(), "id-second", nil); err == nil ||
		!strings.Contains(err.Error(), "duplicate label") {
		t.Fatalf("duplicate route registration error = %v, want deterministic duplicate diagnostic", err)
	}
	if err := router.unregister(first); err != nil {
		t.Fatalf("cleanup first route: %v", err)
	}

	unmatchedPath := filepath.Join(t.TempDir(), "config.json")
	firstErr := routeFailure(router, unmatchedPath)
	secondErr := routeFailure(router, unmatchedPath)
	if firstErr == nil || secondErr == nil || firstErr.Error() != secondErr.Error() {
		t.Fatalf("unmatched route diagnostics = (%v, %v), want identical failures", firstErr, secondErr)
	}

	firstHome, firstWorking = t.TempDir(), t.TempDir()
	secondHome, secondWorking := t.TempDir(), t.TempDir()
	first, err = router.register("first", firstHome, firstWorking, "id-first", nil)
	if err != nil {
		t.Fatalf("register first selector route: %v", err)
	}
	second, err := router.register("second", secondHome, secondWorking, "id-second", nil)
	if err != nil {
		_ = router.unregister(first)
		t.Fatalf("register second selector route: %v", err)
	}
	if err := router.acquire(context.Background(), first); err != nil {
		_ = router.unregister(first)
		_ = router.unregister(second)
		t.Fatalf("acquire first selector route: %v", err)
	}
	filesystemCalls := router.fileSystemCalls.Load()
	if _, err := router.ReadFile(filepath.Join(secondHome, "config.json")); err == nil ||
		!strings.Contains(err.Error(), "selector belongs to route") {
		t.Fatalf("cross-route filesystem access error = %v, want fail-closed diagnostic", err)
	}
	if got := router.fileSystemCalls.Load(); got != filesystemCalls {
		t.Fatalf("cross-route filesystem access calls = %d, want unchanged at %d", got, filesystemCalls)
	}
	temporary, err := router.createTemporaryFile(firstHome, "route-cleanup-*")
	if err != nil {
		t.Fatalf("create routed temporary file: %v", err)
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		t.Fatalf("close routed temporary file: %v", err)
	}
	if err := router.release(first); err != nil {
		t.Fatalf("release first selector route: %v", err)
	}
	if err := router.unregister(first); err != nil {
		t.Fatalf("cleanup first selector route: %v", err)
	}
	if err := router.unregister(second); err != nil {
		t.Fatalf("cleanup second selector route: %v", err)
	}
	if _, err := os.Stat(temporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("routed temporary file after route cleanup = %v, want not found", err)
	}

	partial := newOperatorSettingsEffectRouter()
	partialRoute, err := partial.register("partial", t.TempDir(), t.TempDir(), "id-partial", nil)
	if err != nil {
		t.Fatalf("register partial route: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := partial.acquire(ctx, partialRoute); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("partial route lease error = %v, want context cancellation", err)
	}
	if err := partial.unregister(partialRoute); err != nil {
		t.Fatalf("cleanup partial route: %v", err)
	}
	if got := partial.routeCount(); got != 0 {
		t.Fatalf("partial setup routes after cleanup = %d, want 0", got)
	}
}

func routeFailure(router *operatorSettingsEffectRouter, path string) error {
	_, err := router.ReadFile(path)
	return err
}

var _ operatorsettings.FileSystem = (*operatorSettingsEffectRouter)(nil)
