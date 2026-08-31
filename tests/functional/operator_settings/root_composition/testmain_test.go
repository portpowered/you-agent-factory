package root_composition_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedOperatorSettingsProcessTimeout = 15 * time.Second

var sharedOperatorSettingsFixtureState struct {
	sync.Once
	fixture *sharedOperatorSettingsFixture
	err     error
}

// sharedOperatorSettingsFixture owns the one production-composed process for
// this package. Invocation-local paths select a route; the home and settings
// ID callbacks use the bounded lease because their contracts do not carry a
// selector.
type sharedOperatorSettingsFixture struct {
	process support.ApplicationProcess
	router  *operatorSettingsEffectRouter

	construction operatorSettingsEffectSnapshot
}

type operatorSettingsEffectSnapshot struct {
	fileSystemCalls      int64
	readFileCalls        int64
	createTemporaryCalls int64
	operatorIDCalls      int64
	providerRunnerCalls  int64
	unmatchedRouteCalls  int64
}

type operatorSettingsEffectRouter struct {
	mu     sync.Mutex
	routes map[string]*operatorSettingsEffectRoute
	active *operatorSettingsEffectRoute
	lease  chan struct{}

	fileSystemCalls      atomic.Int64
	readFileCalls        atomic.Int64
	createTemporaryCalls atomic.Int64
	operatorIDCalls      atomic.Int64
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

	mu        sync.Mutex
	closed    bool
	temporary map[string]struct{}
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
		ProviderCommandRunner:               operatorSettingsProviderRunner(router.runProvider),
	})
	if buildErr != nil {
		return nil, fmt.Errorf("compose shared Operator Settings process: %w", buildErr)
	}
	if process == nil {
		return nil, errors.New("compose shared Operator Settings process: process is nil")
	}

	return &sharedOperatorSettingsFixture{
		process:      process,
		router:       router,
		construction: router.snapshot(),
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
		readFileCalls:        router.readFileCalls.Load(),
		createTemporaryCalls: router.createTemporaryCalls.Load(),
		operatorIDCalls:      router.operatorIDCalls.Load(),
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
	router.readFileCalls.Add(1)
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

// withOperatorSettingsRoute gives a test the synchronized external-effect
// route without starting a live Factory Session.
func (fixture *sharedOperatorSettingsFixture) withOperatorSettingsRoute(
	t *testing.T,
	label, homeDir, workingDir, generatedID string,
	providerRunner platformprocess.CommandRunner,
	run func(*operatorSettingsEffectRoute),
) {
	t.Helper()
	route, err := fixture.router.register(label, homeDir, workingDir, generatedID, providerRunner)
	if err != nil {
		t.Fatalf("register Operator Settings route: %v", err)
	}
	if err := fixture.router.acquire(t.Context(), route); err != nil {
		_ = fixture.router.unregister(route)
		t.Fatalf("acquire Operator Settings route: %v", err)
	}
	defer func() {
		if err := fixture.router.release(route); err != nil {
			t.Errorf("release Operator Settings route: %v", err)
		}
		if err := fixture.router.unregister(route); err != nil {
			t.Errorf("unregister Operator Settings route: %v", err)
		}
	}()
	run(route)
}

func closeSharedOperatorSettingsFixture() error {
	fixture := sharedOperatorSettingsFixtureState.fixture
	if fixture == nil {
		return nil
	}
	var errs []error
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

// TestMain registers Operator Settings composition hooks before functional proofs run.
func TestMain(m *testing.M) {
	settingswire.RegisterTestComposition()
	exitCode := m.Run()
	if err := closeSharedOperatorSettingsFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "shared Operator Settings fixture cleanup: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}
