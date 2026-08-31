package root_composition_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const rootCompositionRouteLeaseTimeout = 15 * time.Second

var (
	errRootCompositionRouteNotFound     = errors.New("root composition route not found")
	errRootCompositionRouteAmbiguous    = errors.New("root composition route is ambiguous")
	errRootCompositionRouteDuplicate    = errors.New("root composition route is already registered")
	errRootCompositionRouteOverlap      = errors.New("root composition route overlaps a registered route")
	errRootCompositionRouteNotActive    = errors.New("root composition route lease is not active")
	errRootCompositionRouteLeaseOwner   = errors.New("root composition route lease belongs to another route")
	errRootCompositionRouteClosed       = errors.New("root composition route is closed")
	errRootCompositionRouteRegistration = errors.New("root composition route registration is closed")
)

type rootCompositionFixtureState struct {
	once    sync.Once
	fixture *rootCompositionFixture
	err     error
}

var sharedRootCompositionFixture rootCompositionFixtureState

// TestMain owns the one package process after all parallel scenarios have
// finished. The process is not initialized here: inert tests must be able to
// prove that package setup does not activate a Factory Session.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeRootCompositionFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "root composition fixture cleanup: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

type rootCompositionFixture struct {
	process   support.ApplicationProcess
	api       *support.ProcessAPIServer
	routes    *rootCompositionRouteRegistry
	effects   *rootCompositionSessionEffects
	cleanup   *rootCompositionCleanupLedger
	apiStarts atomic.Int64
}

func ensureRootCompositionFixture(t testing.TB) *rootCompositionFixture {
	t.Helper()
	sharedRootCompositionFixture.once.Do(func() {
		sharedRootCompositionFixture.fixture, sharedRootCompositionFixture.err = newRootCompositionFixture()
	})
	if sharedRootCompositionFixture.err != nil {
		t.Fatalf("construct shared root composition fixture: %v", sharedRootCompositionFixture.err)
	}
	return sharedRootCompositionFixture.fixture
}

func newRootCompositionFixture() (*rootCompositionFixture, error) {
	routes := newRootCompositionRouteRegistry()
	fixture := &rootCompositionFixture{
		api:    support.NewProcessAPIServer(),
		routes: routes,
	}
	fixture.effects = newRootCompositionSessionEffects(routes, &fixture.apiStarts)

	process, err := support.BuildProcessWithContext(context.Background(), fixture.edges())
	if err != nil {
		return nil, err
	}
	fixture.process = process

	cleanup := newRootCompositionCleanupLedger()
	if _, err := cleanup.register("shared application process", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return process.Close(ctx)
	}); err != nil {
		return nil, errors.Join(err, process.Close(context.Background()))
	}
	fixture.cleanup = cleanup
	fixture.effects.captureConstructionSnapshot()
	return fixture, nil
}

func closeRootCompositionFixture() error {
	fixture := sharedRootCompositionFixture.fixture
	if fixture == nil {
		return sharedRootCompositionFixture.err
	}
	return errors.Join(fixture.routes.cleanup(), fixture.cleanup.cleanup())
}

func (fixture *rootCompositionFixture) edges() serviceedges.Edges {
	return serviceedges.Edges{
		APIServerStarter:                           fixture.startAPIServer,
		ProviderCommandRunner:                      fixture.routes.commandRunner("provider"),
		ScriptCommandRunner:                        fixture.routes.commandRunner("script"),
		FactorySessionsWorkingDirectory:            fixture.effects.workingDirectory(),
		FactorySessionExecutionOpeningFileSystem:   &rootCompositionExecutionOpeningFileSystem{effects: fixture.effects},
		FactorySessionDirectoryInspection:          &rootCompositionDirectoryInspection{effects: fixture.effects},
		FactorySessionResolveHomeDirectory:         fixture.effects.resolveHomeDirectory,
		FactorySessionResolveLogicalTargetSymlinks: fixture.effects.resolveLogicalTargetSymlinks,
		FactorySessionIDGenerator:                  fixture.effects.nextSessionID,
		FactorySessionRuntimeInstanceIDGenerator:   fixture.effects.nextRuntimeInstanceID,
		FactorySessionResponseEventIDGenerator:     fixture.effects.nextResponseEventID,
		FactorySessionCursorPersistenceFileSystem: &rootCompositionCursorPersistenceFileSystem{
			effects: fixture.effects,
		},
		FactorySessionCursorCreateTemporaryFile: fixture.effects.createCursorTemporaryFile,
		FactorySessionRuntimePersistenceFileSystem: &rootCompositionRuntimePersistenceFileSystem{
			effects: fixture.effects,
		},
		FactorySessionContractFixtureReader: fixture.effects.readContractFixture,
		FactorySessionInvocationInputReader: fixture.effects.readInvocationInput,
		FactorySessionReplayRecordingReader: fixture.effects.readReplayRecording,
		FactorySessionInitialWorkReader:     fixture.effects.readInitialWork,
		InvocationMetricsRecorder:           fixture.effects,
		RuntimeHostObserver:                 fixture.effects.observeRuntimeHost,
		WorkRequestIDGenerator:              fixture.effects.nextWorkRequestID,
	}
}

func (fixture *rootCompositionFixture) startAPIServer(ctx context.Context, request platformhttpserver.StartRequest) error {
	if _, err := fixture.routes.activeRoute(); err != nil {
		return err
	}
	fixture.apiStarts.Add(1)
	return fixture.api.Start(ctx, request)
}

func (fixture *rootCompositionFixture) constructionSnapshot() rootCompositionConstructionSnapshot {
	return fixture.effects.constructionSnapshot()
}

func (fixture *rootCompositionFixture) withRootCompositionRoute(
	t testing.TB,
	spec rootCompositionRouteSpec,
	run func(),
) {
	t.Helper()
	route, err := fixture.routes.register(spec)
	if err != nil {
		t.Fatalf("register root composition route: %v", err)
	}
	leaseContext, cancel := context.WithTimeout(context.Background(), rootCompositionRouteLeaseTimeout)
	defer cancel()
	if err := fixture.routes.acquire(leaseContext, route); err != nil {
		_ = fixture.routes.unregister(route)
		t.Fatalf("acquire root composition route %q: %v", route.label, err)
	}
	defer func() {
		if err := fixture.routes.release(route); err != nil {
			t.Errorf("release root composition route %q: %v", route.label, err)
		}
		if err := fixture.routes.unregister(route); err != nil {
			t.Errorf("unregister root composition route %q: %v", route.label, err)
		}
	}()
	run()
}

type rootCompositionRouteSpec struct {
	label          string
	homeDir        string
	workingDir     string
	extraPaths     []string
	providerRunner platformprocess.CommandRunner
	scriptRunner   platformprocess.CommandRunner
}

type rootCompositionRoute struct {
	label          string
	homeDir        string
	workingDir     string
	selectors      []string
	providerRunner platformprocess.CommandRunner
	scriptRunner   platformprocess.CommandRunner

	mu             sync.Mutex
	temporaryFiles map[string]struct{}
	closed         bool
}

type rootCompositionRouteRegistry struct {
	mu            sync.RWMutex
	routes        map[string]*rootCompositionRoute
	active        *rootCompositionRoute
	lease         chan struct{}
	closed        bool
	unmatched     atomic.Int64
	providerCalls atomic.Int64
	scriptCalls   atomic.Int64
}

func newRootCompositionRouteRegistry() *rootCompositionRouteRegistry {
	return &rootCompositionRouteRegistry{
		routes: make(map[string]*rootCompositionRoute),
		lease:  make(chan struct{}, 1),
	}
}

func (registry *rootCompositionRouteRegistry) register(spec rootCompositionRouteSpec) (*rootCompositionRoute, error) {
	label := strings.TrimSpace(spec.label)
	if label == "" {
		return nil, fmt.Errorf("%w: label is empty", errRootCompositionRouteDuplicate)
	}
	homeDir, err := normalizeRootCompositionPath(spec.homeDir)
	if err != nil {
		return nil, fmt.Errorf("route %q home directory: %w", label, err)
	}
	workingDir, err := normalizeRootCompositionPath(spec.workingDir)
	if err != nil {
		return nil, fmt.Errorf("route %q working directory: %w", label, err)
	}
	selectors := []string{homeDir, workingDir}
	for _, extraPath := range spec.extraPaths {
		normalized, err := normalizeRootCompositionPath(extraPath)
		if err != nil {
			return nil, fmt.Errorf("route %q extra path: %w", label, err)
		}
		selectors = append(selectors, normalized)
	}
	selectors = uniqueRootCompositionPaths(selectors)

	route := &rootCompositionRoute{
		label:          label,
		homeDir:        homeDir,
		workingDir:     workingDir,
		selectors:      selectors,
		providerRunner: spec.providerRunner,
		scriptRunner:   spec.scriptRunner,
		temporaryFiles: make(map[string]struct{}),
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed {
		return nil, errRootCompositionRouteRegistration
	}
	if _, exists := registry.routes[label]; exists {
		return nil, fmt.Errorf("%w: %q", errRootCompositionRouteDuplicate, label)
	}
	for _, existing := range registry.routes {
		if rootCompositionSelectorsOverlap(existing.selectors, route.selectors) {
			return nil, fmt.Errorf("%w: %q overlaps %q", errRootCompositionRouteOverlap, label, existing.label)
		}
	}
	registry.routes[label] = route
	return route, nil
}

func (registry *rootCompositionRouteRegistry) unregister(route *rootCompositionRoute) error {
	if route == nil {
		return nil
	}
	registry.mu.Lock()
	if current, exists := registry.routes[route.label]; !exists || current != route {
		registry.mu.Unlock()
		return nil
	}
	if registry.active == route {
		registry.mu.Unlock()
		return errRootCompositionRouteNotActive
	}
	delete(registry.routes, route.label)
	registry.mu.Unlock()

	route.mu.Lock()
	route.closed = true
	temporaryFiles := make([]string, 0, len(route.temporaryFiles))
	for path := range route.temporaryFiles {
		temporaryFiles = append(temporaryFiles, path)
	}
	route.temporaryFiles = make(map[string]struct{})
	route.mu.Unlock()

	var errs []error
	for _, path := range temporaryFiles {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove temporary file %q: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

func (registry *rootCompositionRouteRegistry) acquire(ctx context.Context, route *rootCompositionRoute) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := registry.requireRegistered(route); err != nil {
		return err
	}
	select {
	case registry.lease <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("acquire route lease: %w", ctx.Err())
	}

	registry.mu.Lock()
	if registry.closed {
		registry.mu.Unlock()
		<-registry.lease
		return errRootCompositionRouteRegistration
	}
	current, exists := registry.routes[route.label]
	if !exists || current != route {
		registry.mu.Unlock()
		<-registry.lease
		return errRootCompositionRouteClosed
	}
	if registry.active != nil {
		activeLabel := registry.active.label
		registry.mu.Unlock()
		<-registry.lease
		return fmt.Errorf("%w: %q", errRootCompositionRouteNotActive, activeLabel)
	}
	registry.active = route
	registry.mu.Unlock()
	return nil
}

func (registry *rootCompositionRouteRegistry) release(route *rootCompositionRoute) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.active == nil {
		return errRootCompositionRouteNotActive
	}
	if registry.active != route {
		return fmt.Errorf("%w: active route is %q", errRootCompositionRouteLeaseOwner, registry.active.label)
	}
	registry.active = nil
	<-registry.lease
	return nil
}

func (registry *rootCompositionRouteRegistry) requireRegistered(route *rootCompositionRoute) error {
	if route == nil {
		return errRootCompositionRouteNotFound
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if registry.closed {
		return errRootCompositionRouteRegistration
	}
	current, exists := registry.routes[route.label]
	if !exists || current != route {
		return errRootCompositionRouteNotFound
	}
	return nil
}

func (registry *rootCompositionRouteRegistry) routeForPath(path string) (*rootCompositionRoute, error) {
	normalized, err := normalizeRootCompositionPath(path)
	if err != nil {
		registry.unmatched.Add(1)
		return nil, fmt.Errorf("%w: %v", errRootCompositionRouteNotFound, err)
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	var matches []*rootCompositionRoute
	for _, route := range registry.routes {
		if route.closed {
			continue
		}
		for _, selector := range route.selectors {
			if rootCompositionPathWithin(selector, normalized) {
				matches = append(matches, route)
				break
			}
		}
	}
	if len(matches) == 0 {
		registry.unmatched.Add(1)
		return nil, fmt.Errorf("%w: %q", errRootCompositionRouteNotFound, normalized)
	}
	if len(matches) > 1 {
		registry.unmatched.Add(1)
		labels := make([]string, len(matches))
		for index, match := range matches {
			labels[index] = match.label
		}
		sort.Strings(labels)
		return nil, fmt.Errorf("%w: %s", errRootCompositionRouteAmbiguous, strings.Join(labels, ", "))
	}
	return matches[0], nil
}

func (registry *rootCompositionRouteRegistry) activeRoute() (*rootCompositionRoute, error) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if registry.active == nil {
		registry.unmatched.Add(1)
		return nil, errRootCompositionRouteNotActive
	}
	return registry.active, nil
}

func (registry *rootCompositionRouteRegistry) routeForEffectPath(path string) (*rootCompositionRoute, error) {
	route, err := registry.routeForPath(path)
	if err != nil {
		return nil, err
	}
	active, err := registry.activeRoute()
	if err != nil {
		return nil, err
	}
	if active != route {
		registry.unmatched.Add(1)
		return nil, fmt.Errorf("%w: active route is %q, requested route is %q", errRootCompositionRouteLeaseOwner, active.label, route.label)
	}
	return route, nil
}

func (registry *rootCompositionRouteRegistry) routeForCommand(path string) (*rootCompositionRoute, error) {
	route, err := registry.routeForPath(path)
	if err != nil {
		return nil, err
	}
	active, err := registry.activeRoute()
	if err != nil {
		return nil, err
	}
	if active != route {
		registry.unmatched.Add(1)
		return nil, fmt.Errorf("%w: active route is %q, command route is %q", errRootCompositionRouteLeaseOwner, active.label, route.label)
	}
	return route, nil
}

func (registry *rootCompositionRouteRegistry) commandRunner(kind string) platformprocess.CommandRunner {
	return &rootCompositionCommandRouter{registry: registry, kind: kind}
}

func (registry *rootCompositionRouteRegistry) unmatchedCount() int64 {
	return registry.unmatched.Load()
}

func (registry *rootCompositionRouteRegistry) count() int {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return len(registry.routes)
}

func (registry *rootCompositionRouteRegistry) cleanup() error {
	registry.mu.Lock()
	if registry.closed {
		registry.mu.Unlock()
		return nil
	}
	registry.closed = true
	active := registry.active
	registry.active = nil
	routes := make([]*rootCompositionRoute, 0, len(registry.routes))
	for _, route := range registry.routes {
		routes = append(routes, route)
	}
	registry.routes = make(map[string]*rootCompositionRoute)
	registry.mu.Unlock()

	var errs []error
	if active != nil {
		errs = append(errs, fmt.Errorf("active route %q still held at package cleanup", active.label))
		<-registry.lease
	}
	for _, route := range routes {
		route.mu.Lock()
		route.closed = true
		temporaryFiles := make([]string, 0, len(route.temporaryFiles))
		for path := range route.temporaryFiles {
			temporaryFiles = append(temporaryFiles, path)
		}
		route.temporaryFiles = make(map[string]struct{})
		route.mu.Unlock()
		for _, path := range temporaryFiles {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("remove temporary file %q: %w", path, err))
			}
		}
	}
	return errors.Join(errs...)
}

func normalizeRootCompositionPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path is empty")
	}
	absPath, err := filepath.Abs(filepath.Clean(trimmed))
	if err != nil {
		return "", err
	}
	return filepath.Clean(absPath), nil
}

func uniqueRootCompositionPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}
	return unique
}

func rootCompositionPathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func rootCompositionSelectorsOverlap(left, right []string) bool {
	for _, leftSelector := range left {
		for _, rightSelector := range right {
			if rootCompositionPathWithin(leftSelector, rightSelector) || rootCompositionPathWithin(rightSelector, leftSelector) {
				return true
			}
		}
	}
	return false
}

type rootCompositionCommandRouter struct {
	registry *rootCompositionRouteRegistry
	kind     string
}

func (router *rootCompositionCommandRouter) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if router.kind == "provider" {
		router.registry.providerCalls.Add(1)
	} else {
		router.registry.scriptCalls.Add(1)
	}
	route, err := router.registry.routeForCommand(request.WorkDir)
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("%s command route rejected: %w", router.kind, err)
	}
	var runner platformprocess.CommandRunner
	if router.kind == "provider" {
		runner = route.providerRunner
	} else {
		runner = route.scriptRunner
	}
	if runner == nil {
		router.registry.unmatched.Add(1)
		return platformprocess.CommandResult{}, fmt.Errorf("%s command route %q has no runner", router.kind, route.label)
	}
	return runner.Run(ctx, request)
}

type rootCompositionCleanupLedger struct {
	mu      sync.Mutex
	closed  bool
	next    uint64
	actions map[uint64]*rootCompositionCleanupAction
}

type rootCompositionCleanupAction struct {
	label string
	once  sync.Once
	fn    func() error
	err   error
}

func newRootCompositionCleanupLedger() *rootCompositionCleanupLedger {
	return &rootCompositionCleanupLedger{actions: make(map[uint64]*rootCompositionCleanupAction)}
}

func (ledger *rootCompositionCleanupLedger) register(label string, fn func() error) (func() error, error) {
	if strings.TrimSpace(label) == "" {
		return nil, errors.New("cleanup label is empty")
	}
	if fn == nil {
		return nil, errors.New("cleanup function is nil")
	}
	action := &rootCompositionCleanupAction{label: label, fn: fn}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.closed {
		return nil, errors.New("cleanup ledger is closed")
	}
	ledger.next++
	id := ledger.next
	ledger.actions[id] = action
	return func() error {
		ledger.mu.Lock()
		delete(ledger.actions, id)
		ledger.mu.Unlock()
		return action.run()
	}, nil
}

func (ledger *rootCompositionCleanupLedger) cleanup() error {
	ledger.mu.Lock()
	if ledger.closed && len(ledger.actions) == 0 {
		ledger.mu.Unlock()
		return nil
	}
	ledger.closed = true
	ids := make([]uint64, 0, len(ledger.actions))
	for id := range ledger.actions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	actions := make([]*rootCompositionCleanupAction, 0, len(ids))
	for _, id := range ids {
		actions = append(actions, ledger.actions[id])
	}
	ledger.actions = make(map[uint64]*rootCompositionCleanupAction)
	ledger.mu.Unlock()

	var errs []error
	for _, action := range actions {
		if err := action.run(); err != nil {
			errs = append(errs, fmt.Errorf("cleanup %s: %w", action.label, err))
		}
	}
	return errors.Join(errs...)
}

func (action *rootCompositionCleanupAction) run() error {
	action.once.Do(func() { action.err = action.fn() })
	return action.err
}

type rootCompositionEffectCounts struct {
	workingDirectory atomic.Int64
	executionGetwd   atomic.Int64
	executionStat    atomic.Int64
	directoryStat    atomic.Int64
	directoryReadDir atomic.Int64
	resolveHome      atomic.Int64
	resolveSymlinks  atomic.Int64
	sessionID        atomic.Int64
	runtimeID        atomic.Int64
	responseEventID  atomic.Int64
	cursorMkdirAll   atomic.Int64
	cursorReadFile   atomic.Int64
	cursorRemove     atomic.Int64
	cursorRename     atomic.Int64
	cursorTempFile   atomic.Int64
	runtimeMkdirAll  atomic.Int64
	runtimeReadFile  atomic.Int64
	runtimeWriteFile atomic.Int64
	contractFixture  atomic.Int64
	replayRecording  atomic.Int64
	invocationInput  atomic.Int64
	initialWork      atomic.Int64
	invocationMetric atomic.Int64
	runtimeHost      atomic.Int64
	workRequestID    atomic.Int64
}

type rootCompositionConstructionSnapshot struct {
	providerCalls    int64
	scriptCalls      int64
	apiStarts        int64
	routeRejections  int64
	workingDirectory int64
	executionGetwd   int64
	executionStat    int64
	directoryStat    int64
	directoryReadDir int64
	resolveHome      int64
	resolveSymlinks  int64
	sessionID        int64
	runtimeID        int64
	responseEventID  int64
	cursorMkdirAll   int64
	cursorReadFile   int64
	cursorRemove     int64
	cursorRename     int64
	cursorTempFile   int64
	runtimeMkdirAll  int64
	runtimeReadFile  int64
	runtimeWriteFile int64
	contractFixture  int64
	replayRecording  int64
	invocationInput  int64
	initialWork      int64
	invocationMetric int64
	runtimeHost      int64
	workRequestID    int64
}

func (snapshot rootCompositionConstructionSnapshot) totalLifecycle() int64 {
	return snapshot.resolveHome + snapshot.resolveSymlinks + snapshot.sessionID + snapshot.runtimeID +
		snapshot.directoryStat + snapshot.directoryReadDir + snapshot.cursorMkdirAll + snapshot.cursorReadFile +
		snapshot.cursorRemove + snapshot.cursorRename + snapshot.cursorTempFile + snapshot.runtimeMkdirAll +
		snapshot.runtimeReadFile + snapshot.runtimeWriteFile + snapshot.runtimeHost
}

func (snapshot rootCompositionConstructionSnapshot) totalRuntimeOpening() int64 {
	return snapshot.workingDirectory + snapshot.executionGetwd + snapshot.executionStat + snapshot.contractFixture + snapshot.replayRecording
}

func (snapshot rootCompositionConstructionSnapshot) totalWorkAdmission() int64 {
	return snapshot.invocationInput + snapshot.initialWork
}

func (snapshot rootCompositionConstructionSnapshot) totalResponseStream() int64 {
	return snapshot.responseEventID + snapshot.invocationMetric
}

func (snapshot rootCompositionConstructionSnapshot) total() int64 {
	return snapshot.providerCalls + snapshot.scriptCalls + snapshot.apiStarts + snapshot.routeRejections + snapshot.totalLifecycle() +
		snapshot.totalRuntimeOpening() + snapshot.totalWorkAdmission() + snapshot.totalResponseStream() + snapshot.workRequestID
}

type rootCompositionSessionEffects struct {
	routes    *rootCompositionRouteRegistry
	apiStarts *atomic.Int64
	counts    rootCompositionEffectCounts
	snapshot  atomic.Value
	sequence  atomic.Uint64
}

func newRootCompositionSessionEffects(routes *rootCompositionRouteRegistry, apiStarts *atomic.Int64) *rootCompositionSessionEffects {
	return &rootCompositionSessionEffects{routes: routes, apiStarts: apiStarts}
}

func (effects *rootCompositionSessionEffects) captureConstructionSnapshot() {
	effects.snapshot.Store(effects.constructionSnapshot())
}

func (effects *rootCompositionSessionEffects) constructionSnapshot() rootCompositionConstructionSnapshot {
	if snapshot := effects.snapshot.Load(); snapshot != nil {
		return snapshot.(rootCompositionConstructionSnapshot)
	}
	return rootCompositionConstructionSnapshot{
		providerCalls:    effects.routes.providerCalls.Load(),
		scriptCalls:      effects.routes.scriptCalls.Load(),
		apiStarts:        effects.apiStarts.Load(),
		routeRejections:  effects.routes.unmatchedCount(),
		workingDirectory: effects.counts.workingDirectory.Load(),
		executionGetwd:   effects.counts.executionGetwd.Load(),
		executionStat:    effects.counts.executionStat.Load(),
		directoryStat:    effects.counts.directoryStat.Load(),
		directoryReadDir: effects.counts.directoryReadDir.Load(),
		resolveHome:      effects.counts.resolveHome.Load(),
		resolveSymlinks:  effects.counts.resolveSymlinks.Load(),
		sessionID:        effects.counts.sessionID.Load(),
		runtimeID:        effects.counts.runtimeID.Load(),
		responseEventID:  effects.counts.responseEventID.Load(),
		cursorMkdirAll:   effects.counts.cursorMkdirAll.Load(),
		cursorReadFile:   effects.counts.cursorReadFile.Load(),
		cursorRemove:     effects.counts.cursorRemove.Load(),
		cursorRename:     effects.counts.cursorRename.Load(),
		cursorTempFile:   effects.counts.cursorTempFile.Load(),
		runtimeMkdirAll:  effects.counts.runtimeMkdirAll.Load(),
		runtimeReadFile:  effects.counts.runtimeReadFile.Load(),
		runtimeWriteFile: effects.counts.runtimeWriteFile.Load(),
		contractFixture:  effects.counts.contractFixture.Load(),
		replayRecording:  effects.counts.replayRecording.Load(),
		invocationInput:  effects.counts.invocationInput.Load(),
		initialWork:      effects.counts.initialWork.Load(),
		invocationMetric: effects.counts.invocationMetric.Load(),
		runtimeHost:      effects.counts.runtimeHost.Load(),
		workRequestID:    effects.counts.workRequestID.Load(),
	}
}

func (effects *rootCompositionSessionEffects) activeRoute() (*rootCompositionRoute, error) {
	return effects.routes.activeRoute()
}

func (effects *rootCompositionSessionEffects) effectRoute(path string) (*rootCompositionRoute, error) {
	return effects.routes.routeForEffectPath(path)
}

func (effects *rootCompositionSessionEffects) workingDirectory() platformfilesystem.WorkingDirectory {
	return rootCompositionWorkingDirectory{effects: effects}
}

type rootCompositionWorkingDirectory struct {
	effects *rootCompositionSessionEffects
}

func (directory rootCompositionWorkingDirectory) Getwd() (string, error) {
	route, err := directory.effects.activeRoute()
	if err != nil {
		return "", err
	}
	directory.effects.counts.workingDirectory.Add(1)
	return route.workingDir, nil
}

type rootCompositionExecutionOpeningFileSystem struct {
	effects *rootCompositionSessionEffects
}

func (filesystem *rootCompositionExecutionOpeningFileSystem) Getwd() (string, error) {
	route, err := filesystem.effects.activeRoute()
	if err != nil {
		return "", err
	}
	filesystem.effects.counts.executionGetwd.Add(1)
	return route.workingDir, nil
}

func (filesystem *rootCompositionExecutionOpeningFileSystem) Stat(path string) (fs.FileInfo, error) {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return nil, err
	}
	filesystem.effects.counts.executionStat.Add(1)
	return os.Stat(path)
}

type rootCompositionDirectoryInspection struct {
	effects *rootCompositionSessionEffects
}

func (filesystem *rootCompositionDirectoryInspection) Stat(path string) (fs.FileInfo, error) {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return nil, err
	}
	filesystem.effects.counts.directoryStat.Add(1)
	return os.Stat(path)
}

func (filesystem *rootCompositionDirectoryInspection) ReadDir(path string) ([]fs.DirEntry, error) {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return nil, err
	}
	filesystem.effects.counts.directoryReadDir.Add(1)
	return os.ReadDir(path)
}

type rootCompositionCursorPersistenceFileSystem struct {
	effects *rootCompositionSessionEffects
}

func (filesystem *rootCompositionCursorPersistenceFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return err
	}
	filesystem.effects.counts.cursorMkdirAll.Add(1)
	return os.MkdirAll(path, mode)
}

func (filesystem *rootCompositionCursorPersistenceFileSystem) ReadFile(path string) ([]byte, error) {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return nil, err
	}
	filesystem.effects.counts.cursorReadFile.Add(1)
	return os.ReadFile(path)
}

func (filesystem *rootCompositionCursorPersistenceFileSystem) Remove(path string) error {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return err
	}
	filesystem.effects.counts.cursorRemove.Add(1)
	return os.Remove(path)
}

func (filesystem *rootCompositionCursorPersistenceFileSystem) Rename(oldPath, newPath string) error {
	oldRoute, err := filesystem.effects.effectRoute(oldPath)
	if err != nil {
		return err
	}
	newRoute, err := filesystem.effects.effectRoute(newPath)
	if err != nil {
		return err
	}
	if oldRoute != newRoute {
		return fmt.Errorf("%w: rename crosses routes", errRootCompositionRouteLeaseOwner)
	}
	filesystem.effects.counts.cursorRename.Add(1)
	return os.Rename(oldPath, newPath)
}

type rootCompositionRuntimePersistenceFileSystem struct {
	effects *rootCompositionSessionEffects
}

func (filesystem *rootCompositionRuntimePersistenceFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return err
	}
	filesystem.effects.counts.runtimeMkdirAll.Add(1)
	return os.MkdirAll(path, mode)
}

func (filesystem *rootCompositionRuntimePersistenceFileSystem) ReadFile(path string) ([]byte, error) {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return nil, err
	}
	filesystem.effects.counts.runtimeReadFile.Add(1)
	return os.ReadFile(path)
}

func (filesystem *rootCompositionRuntimePersistenceFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return err
	}
	filesystem.effects.counts.runtimeWriteFile.Add(1)
	return os.WriteFile(path, data, mode)
}

func (effects *rootCompositionSessionEffects) createCursorTemporaryFile(dir, pattern string) (factorysessions.CursorPersistenceTemporaryFile, error) {
	route, err := effects.effectRoute(dir)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	route.mu.Lock()
	route.temporaryFiles[file.Name()] = struct{}{}
	route.mu.Unlock()
	effects.counts.cursorTempFile.Add(1)
	return file, nil
}

func (effects *rootCompositionSessionEffects) resolveHomeDirectory() (string, error) {
	route, err := effects.activeRoute()
	if err != nil {
		return "", err
	}
	effects.counts.resolveHome.Add(1)
	return route.homeDir, nil
}

func (effects *rootCompositionSessionEffects) resolveLogicalTargetSymlinks(path string) (string, error) {
	if _, err := effects.effectRoute(path); err != nil {
		return "", err
	}
	effects.counts.resolveSymlinks.Add(1)
	return filepath.EvalSymlinks(path)
}

func (effects *rootCompositionSessionEffects) nextSessionID() string {
	route, err := effects.activeRoute()
	if err != nil {
		return ""
	}
	effects.counts.sessionID.Add(1)
	return fmt.Sprintf("%s-session-%d", route.label, effects.sequence.Add(1))
}

func (effects *rootCompositionSessionEffects) nextRuntimeInstanceID() string {
	route, err := effects.activeRoute()
	if err != nil {
		return ""
	}
	effects.counts.runtimeID.Add(1)
	return fmt.Sprintf("%s-runtime-%d", route.label, effects.sequence.Add(1))
}

func (effects *rootCompositionSessionEffects) nextResponseEventID() string {
	route, err := effects.activeRoute()
	if err != nil {
		return ""
	}
	effects.counts.responseEventID.Add(1)
	return fmt.Sprintf("%s-response-%d", route.label, effects.sequence.Add(1))
}

func (effects *rootCompositionSessionEffects) nextWorkRequestID() string {
	route, err := effects.activeRoute()
	if err != nil {
		return ""
	}
	effects.counts.workRequestID.Add(1)
	return fmt.Sprintf("%s-request-%d", route.label, effects.sequence.Add(1))
}

func (effects *rootCompositionSessionEffects) readContractFixture(path string) ([]byte, error) {
	if _, err := effects.effectRoute(path); err != nil {
		return nil, err
	}
	effects.counts.contractFixture.Add(1)
	return os.ReadFile(path)
}

func (effects *rootCompositionSessionEffects) readInvocationInput(path string) ([]byte, error) {
	if _, err := effects.effectRoute(path); err != nil {
		return nil, err
	}
	effects.counts.invocationInput.Add(1)
	return os.ReadFile(path)
}

func (effects *rootCompositionSessionEffects) readReplayRecording(path string) ([]byte, error) {
	if _, err := effects.effectRoute(path); err != nil {
		return nil, err
	}
	effects.counts.replayRecording.Add(1)
	return os.ReadFile(path)
}

func (effects *rootCompositionSessionEffects) readInitialWork(path string) ([]byte, error) {
	if _, err := effects.effectRoute(path); err != nil {
		return nil, err
	}
	effects.counts.initialWork.Add(1)
	return os.ReadFile(path)
}

func (effects *rootCompositionSessionEffects) RecordInvocationMetric(factorysessions.InvocationMetric) {
	if _, err := effects.activeRoute(); err != nil {
		return
	}
	effects.counts.invocationMetric.Add(1)
}

func (effects *rootCompositionSessionEffects) observeRuntimeHost(factorysessions.RuntimeHostBinding) {
	if _, err := effects.activeRoute(); err != nil {
		return
	}
	effects.counts.runtimeHost.Add(1)
}

var _ platformfilesystem.WorkingDirectory = rootCompositionWorkingDirectory{}
var _ factorysessions.ExecutionOpeningFileSystem = (*rootCompositionExecutionOpeningFileSystem)(nil)
var _ factorysessions.DirectoryInspection = (*rootCompositionDirectoryInspection)(nil)
var _ factorysessions.CursorPersistenceFileSystem = (*rootCompositionCursorPersistenceFileSystem)(nil)
var _ factorysessions.RuntimePersistenceFileSystem = (*rootCompositionRuntimePersistenceFileSystem)(nil)
var _ factorysessions.InvocationMetricsRecorder = (*rootCompositionSessionEffects)(nil)
var _ platformprocess.CommandRunner = (*rootCompositionCommandRouter)(nil)
var _ io.Writer = (*os.File)(nil)

type rootCompositionCountingCommandRunner struct {
	calls  atomic.Int64
	result platformprocess.CommandResult
}

func (runner *rootCompositionCountingCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return runner.result, nil
}

var _ platformprocess.CommandRunner = (*rootCompositionCountingCommandRunner)(nil)

func TestRootCompositionRoutesFailClosed(t *testing.T) {
	t.Parallel()

	t.Run("missing route rejects the command without invoking the runner", func(t *testing.T) {
		registry := newRootCompositionRouteRegistry()
		runner := &rootCompositionCountingCommandRunner{}
		commandRouter := registry.commandRunner("provider")
		registryRoute, err := registry.register(rootCompositionRouteSpec{
			label:          "owned",
			homeDir:        filepath.Join(t.TempDir(), "home"),
			workingDir:     filepath.Join(t.TempDir(), "factory"),
			providerRunner: runner,
		})
		if err != nil {
			t.Fatalf("register route: %v", err)
		}
		if err := registry.unregister(registryRoute); err != nil {
			t.Fatalf("unregister route: %v", err)
		}

		_, err = commandRouter.Run(context.Background(), platformprocess.CommandRequest{WorkDir: filepath.Join(t.TempDir(), "not-owned")})
		if !errors.Is(err, errRootCompositionRouteNotFound) {
			t.Fatalf("missing route error = %v, want route-not-found", err)
		}
		if got := runner.calls.Load(); got != 0 {
			t.Fatalf("missing route invoked runner %d times, want 0", got)
		}
	})

	t.Run("duplicate and overlapping registration preserve the first route", func(t *testing.T) {
		registry := newRootCompositionRouteRegistry()
		root := t.TempDir()
		first, err := registry.register(rootCompositionRouteSpec{
			label:      "first",
			homeDir:    filepath.Join(root, "home"),
			workingDir: filepath.Join(root, "factory"),
		})
		if err != nil {
			t.Fatalf("register first route: %v", err)
		}
		if _, err := registry.register(rootCompositionRouteSpec{
			label:      "first",
			homeDir:    filepath.Join(root, "other-home"),
			workingDir: filepath.Join(root, "other-factory"),
		}); !errors.Is(err, errRootCompositionRouteDuplicate) {
			t.Fatalf("duplicate route error = %v, want duplicate", err)
		}
		if _, err := registry.register(rootCompositionRouteSpec{
			label:      "nested",
			homeDir:    filepath.Join(root, "nested-home"),
			workingDir: filepath.Join(root, "factory", "nested"),
		}); !errors.Is(err, errRootCompositionRouteOverlap) {
			t.Fatalf("overlapping route error = %v, want overlap", err)
		}
		if got := registry.count(); got != 1 {
			t.Fatalf("registered route count = %d, want 1", got)
		}
		selected, err := registry.routeForPath(filepath.Join(first.workingDir, "input.json"))
		if err != nil || selected != first {
			t.Fatalf("selected route = %v, error = %v; want first route", selected, err)
		}
		if err := registry.unregister(first); err != nil {
			t.Fatalf("unregister first route: %v", err)
		}
	})

	t.Run("canceled lease and cross-route effect reject without fallback", func(t *testing.T) {
		registry := newRootCompositionRouteRegistry()
		root := t.TempDir()
		first, err := registry.register(rootCompositionRouteSpec{
			label:      "first",
			homeDir:    filepath.Join(root, "home-first"),
			workingDir: filepath.Join(root, "factory-first"),
		})
		if err != nil {
			t.Fatalf("register first route: %v", err)
		}
		second, err := registry.register(rootCompositionRouteSpec{
			label:      "second",
			homeDir:    filepath.Join(root, "home-second"),
			workingDir: filepath.Join(root, "factory-second"),
		})
		if err != nil {
			t.Fatalf("register second route: %v", err)
		}
		if err := registry.acquire(context.Background(), first); err != nil {
			t.Fatalf("acquire first route: %v", err)
		}

		canceled, cancel := context.WithCancel(context.Background())
		cancel()
		if err := registry.acquire(canceled, second); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled lease error = %v, want context.Canceled", err)
		}
		if err := registry.release(second); !errors.Is(err, errRootCompositionRouteLeaseOwner) {
			t.Fatalf("mismatched release error = %v, want lease-owner error", err)
		}
		if _, err := registry.routeForEffectPath(second.workingDir); !errors.Is(err, errRootCompositionRouteLeaseOwner) {
			t.Fatalf("cross-route effect error = %v, want lease-owner error", err)
		}
		if err := registry.release(first); err != nil {
			t.Fatalf("release first route: %v", err)
		}
		if err := registry.unregister(first); err != nil {
			t.Fatalf("unregister first route: %v", err)
		}
		if err := registry.unregister(second); err != nil {
			t.Fatalf("unregister second route: %v", err)
		}
		closed, err := registry.register(rootCompositionRouteSpec{
			label:      "closed",
			homeDir:    filepath.Join(root, "home-closed"),
			workingDir: filepath.Join(root, "factory-closed"),
		})
		if err != nil {
			t.Fatalf("register closed-route witness: %v", err)
		}
		if err := registry.unregister(closed); err != nil {
			t.Fatalf("unregister closed-route witness: %v", err)
		}
		if err := registry.acquire(context.Background(), closed); !errors.Is(err, errRootCompositionRouteNotFound) {
			t.Fatalf("closed route acquisition error = %v, want route-not-found", err)
		}
	})
}

func TestRootCompositionCleanupLedgerJoinsFailures(t *testing.T) {
	t.Parallel()
	ledger := newRootCompositionCleanupLedger()
	firstErr := errors.New("first cleanup failed")
	secondErr := errors.New("second cleanup failed")
	firstRan := false
	secondRan := false
	thirdRan := false
	if _, err := ledger.register("first", func() error {
		firstRan = true
		return firstErr
	}); err != nil {
		t.Fatalf("register first cleanup: %v", err)
	}
	if _, err := ledger.register("second", func() error {
		secondRan = true
		return secondErr
	}); err != nil {
		t.Fatalf("register second cleanup: %v", err)
	}
	if _, err := ledger.register("third", func() error {
		thirdRan = true
		return nil
	}); err != nil {
		t.Fatalf("register third cleanup: %v", err)
	}

	cleanupErr := ledger.cleanup()
	if !errors.Is(cleanupErr, firstErr) || !errors.Is(cleanupErr, secondErr) {
		t.Fatalf("cleanup error = %v, want both joined failures", cleanupErr)
	}
	if !firstRan || !secondRan || !thirdRan {
		t.Fatalf("cleanup execution = first:%t second:%t third:%t, want all actions", firstRan, secondRan, thirdRan)
	}
	if err := ledger.cleanup(); err != nil {
		t.Fatalf("idempotent second cleanup = %v, want nil", err)
	}
}
