package lifecycle_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// lifecycleSharedProcessFixture owns a package-local concurrent cohort. Every
// invocation owns a fresh
// working directory, HOME, streams, route, provider runner, and (when used)
// API server, while immutable root wiring is built once for the cohort.
type lifecycleSharedProcessFixture struct {
	process support.ApplicationProcess
	router  *lifecycleCommandRouter
	label   string

	builds     atomic.Int32
	executions atomic.Int32
	active     atomic.Int32
	closes     atomic.Int32

	closeErr error
}

type lifecycleSharedFixtureRegistry struct {
	sync.Once
	fixture *lifecycleSharedProcessFixture
	err     error
}

var lifecycleSharedFixtureState lifecycleSharedFixtureRegistry
var lifecycleAdverseFixtureState lifecycleSharedFixtureRegistry

// TestMain closes package-owned processes after all invocation cleanup has
// completed. Cancellation/recovery and forced-cleanup retain dedicated
// process ownership because their lifecycle state cannot be reset safely.
func TestMain(m *testing.M) {
	exitCode := m.Run()

	for _, fixture := range []*lifecycleSharedProcessFixture{
		lifecycleSharedFixtureState.fixture,
		lifecycleAdverseFixtureState.fixture,
	} {
		if fixture == nil {
			continue
		}
		closeContext, cancel := context.WithTimeout(context.Background(), lifecycleProcessCloseTimeout)
		closeErr := fixture.close(closeContext)
		cancel()
		if closeErr != nil && exitCode == 0 {
			fmt.Fprintf(os.Stderr, "close shared worker CLI lifecycle process: %v\n", closeErr)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func sharedLifecycleProcess(t testing.TB) *lifecycleSharedProcessFixture {
	t.Helper()
	return initializeSharedLifecycleProcess(t, &lifecycleSharedFixtureState, "finite one-shot")
}

func sharedAdverseLifecycleProcess(t testing.TB) *lifecycleSharedProcessFixture {
	t.Helper()
	return initializeSharedLifecycleProcess(t, &lifecycleAdverseFixtureState, "hosted adverse")
}

func initializeSharedLifecycleProcess(
	t testing.TB,
	state *lifecycleSharedFixtureRegistry,
	label string,
) *lifecycleSharedProcessFixture {
	t.Helper()
	state.Do(func() {
		router := newLifecycleCommandRouter()
		process, err := support.BuildProcessWithContext(
			context.Background(),
			serviceedges.Edges{
				APIServerStarter:      router.Start,
				ProviderCommandRunner: router,
			},
		)
		if err != nil {
			state.err = fmt.Errorf("build %s shared lifecycle process: %w", label, err)
			return
		}
		fixture := &lifecycleSharedProcessFixture{process: process, router: router, label: label}
		fixture.builds.Store(1)
		state.fixture = fixture
	})
	if state.err != nil {
		t.Fatalf("initialize %s shared lifecycle process: %v", label, state.err)
	}
	if state.fixture == nil {
		t.Fatalf("%s shared lifecycle process was not initialized", label)
	}
	return state.fixture
}

func executeSharedLifecycleInvocation(
	t testing.TB,
	args []string,
	runner platformprocess.CommandRunner,
) (*support.CapturedInputs, error) {
	t.Helper()
	inputs := newSharedLifecycleInputs(t, args)
	return inputs, executeSharedLifecycleInputs(t, inputs, runner)
}

func newSharedLifecycleInputs(t testing.TB, args []string) *support.CapturedInputs {
	t.Helper()
	args, _ = ensureLifecycleSessionArg(args)
	inputs := support.FakeInputs(t.Context(), args)
	// The factory path in these candidates is absolute. A fresh working
	// directory therefore preserves the customer-facing invocation contract
	// while proving that route selection and cwd state cannot leak between runs.
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Env = isolatedLifecycleEnvironment(inputs.Input.Env, t.TempDir())
	return inputs
}

func executeSharedLifecycleInputs(
	t testing.TB,
	inputs *support.CapturedInputs,
	runner platformprocess.CommandRunner,
) error {
	t.Helper()
	fixture := sharedLifecycleProcess(t)
	return fixture.execute(t, inputs, runner, nil)
}

func executeSharedLifecyclePreRuntimeInputs(
	t testing.TB,
	inputs *support.CapturedInputs,
	runner platformprocess.CommandRunner,
) error {
	t.Helper()
	if inputs == nil {
		return errors.New("shared lifecycle invocation inputs are required")
	}
	return sharedLifecycleProcess(t).executeInputWithoutRuntimeLease(inputs.Input, runner)
}

func sharedLifecycleInvocationProcess(
	t testing.TB,
	runner platformprocess.CommandRunner,
	apiStarter platformhttpserver.Starter,
) support.Process {
	t.Helper()
	return &lifecycleRoutedInvocationProcess{
		fixture: sharedLifecycleProcess(t),
		runner:  runner,
		api:     apiStarter,
	}
}

func sharedAdverseLifecycleInvocationProcess(
	t testing.TB,
	runner platformprocess.CommandRunner,
	apiStarter platformhttpserver.Starter,
) support.Process {
	t.Helper()
	return &lifecycleRoutedInvocationProcess{
		fixture: sharedAdverseLifecycleProcess(t),
		runner:  runner,
		api:     apiStarter,
	}
}

func buildSharedAdverseLifecycleProcess(
	t *testing.T,
	runner platformprocess.CommandRunner,
	apiStarter platformhttpserver.Starter,
) *lifecycleCoordinator {
	t.Helper()
	return newLifecycleCoordinator(t, sharedAdverseLifecycleInvocationProcess(t, runner, apiStarter))
}

type lifecycleRoutedInvocationProcess struct {
	fixture *lifecycleSharedProcessFixture
	runner  platformprocess.CommandRunner
	api     platformhttpserver.Starter
}

func (process *lifecycleRoutedInvocationProcess) Execute(input root.Input) error {
	if process == nil || process.fixture == nil {
		return errors.New("shared lifecycle invocation process is unavailable")
	}
	return process.fixture.executeInput(input, process.runner, process.api)
}

func (fixture *lifecycleSharedProcessFixture) execute(
	t testing.TB,
	inputs *support.CapturedInputs,
	runner platformprocess.CommandRunner,
	apiStarter platformhttpserver.Starter,
) error {
	t.Helper()
	if inputs == nil {
		return errors.New("shared lifecycle invocation inputs are required")
	}
	return fixture.executeInput(inputs.Input, runner, apiStarter)
}

func (fixture *lifecycleSharedProcessFixture) executeInput(
	input root.Input,
	runner platformprocess.CommandRunner,
	apiStarter platformhttpserver.Starter,
) error {
	if fixture == nil || fixture.process == nil || fixture.router == nil {
		return errors.New("shared lifecycle process is unavailable")
	}
	if runner == nil {
		return errors.New("shared lifecycle provider runner is required")
	}
	workingDirectory := filepath.Clean(input.WorkingDirectory)
	if workingDirectory == "." || workingDirectory == "" {
		return fmt.Errorf("shared lifecycle invocation working directory is invalid: %q", input.WorkingDirectory)
	}

	if err := fixture.router.bind(workingDirectory, runner, apiStarter); err != nil {
		return err
	}
	fixture.active.Add(1)
	fixture.executions.Add(1)
	defer func() {
		fixture.active.Add(-1)
		fixture.router.unbind(workingDirectory)
	}()
	return fixture.process.Execute(input)
}

// executeInputWithoutRuntimeLease is reserved for customer input validation
// that is guaranteed to fail before runtime/session activation. Such cases can
// share the immutable process graph without joining the runtime serialization
// lane used by successful invocations.
func (fixture *lifecycleSharedProcessFixture) executeInputWithoutRuntimeLease(
	input root.Input,
	runner platformprocess.CommandRunner,
) error {
	if fixture == nil || fixture.process == nil || fixture.router == nil {
		return errors.New("shared lifecycle process is unavailable")
	}
	if runner == nil {
		return errors.New("shared lifecycle provider runner is required")
	}
	workingDirectory := filepath.Clean(input.WorkingDirectory)
	if workingDirectory == "." || workingDirectory == "" {
		return fmt.Errorf("shared lifecycle invocation working directory is invalid: %q", input.WorkingDirectory)
	}
	if err := fixture.router.bind(workingDirectory, runner, nil); err != nil {
		return err
	}
	fixture.active.Add(1)
	fixture.executions.Add(1)
	defer func() {
		fixture.active.Add(-1)
		fixture.router.unbind(workingDirectory)
	}()
	return fixture.process.Execute(input)
}

func (fixture *lifecycleSharedProcessFixture) close(ctx context.Context) error {
	if fixture == nil {
		return nil
	}
	if fixture.closeErr != nil {
		return fixture.closeErr
	}

	var closeErrors []error
	if fixture.active.Load() != 0 {
		closeErrors = append(closeErrors, fmt.Errorf("active shared lifecycle invocations = %d, want 0", fixture.active.Load()))
	}
	if routes := fixture.router.routeCount(); routes != 0 {
		closeErrors = append(closeErrors, fmt.Errorf("shared lifecycle provider routes = %d, want 0", routes))
	}
	if fixture.builds.Load() != 1 {
		closeErrors = append(closeErrors, fmt.Errorf("shared lifecycle root builds = %d, want 1", fixture.builds.Load()))
	}
	if err := fixture.process.Close(ctx); err != nil {
		closeErrors = append(closeErrors, fmt.Errorf("close shared lifecycle process: %w", err))
	} else {
		fixture.closes.Add(1)
	}
	if routes := fixture.router.routeCount(); routes != 0 {
		closeErrors = append(closeErrors, fmt.Errorf("shared lifecycle provider routes after close = %d, want 0", routes))
	}
	fixture.closeErr = errors.Join(closeErrors...)
	fmt.Fprintf(
		os.Stderr,
		"C14 lifecycle %s shared topology: root_builds=%d process_executions=%d process_closes=%d active_invocations=%d provider_route_registers=%d provider_route_unbinds=%d provider_routes=%d provider_calls=%d api_starts=%d\n",
		fixture.label,
		fixture.builds.Load(), fixture.executions.Load(), fixture.closes.Load(), fixture.active.Load(),
		fixture.router.bindCount(), fixture.router.unbindCount(), fixture.router.routeCount(), fixture.router.callCount(), fixture.router.apiStartCount(),
	)
	return fixture.closeErr
}

type lifecycleCommandRoute struct {
	runner     platformprocess.CommandRunner
	apiStarter platformhttpserver.Starter
}

type lifecycleCommandRouter struct {
	mu sync.Mutex

	routes    map[string]lifecycleCommandRoute
	binds     int
	unbinds   int
	calls     int
	apiStarts int
}

func newLifecycleCommandRouter() *lifecycleCommandRouter {
	return &lifecycleCommandRouter{routes: make(map[string]lifecycleCommandRoute)}
}

func (router *lifecycleCommandRouter) bind(
	workingDirectory string,
	runner platformprocess.CommandRunner,
	apiStarter platformhttpserver.Starter,
) error {
	key := filepath.Clean(workingDirectory)
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[key]; exists {
		return fmt.Errorf("shared lifecycle provider route %q is already bound", key)
	}
	router.routes[key] = lifecycleCommandRoute{runner: runner, apiStarter: apiStarter}
	router.binds++
	return nil
}

func (router *lifecycleCommandRouter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	key := filepath.Clean(startupcli.WorkingDirectory(ctx))
	router.mu.Lock()
	route, ok := router.routes[key]
	if route.apiStarter != nil {
		router.apiStarts++
	}
	router.mu.Unlock()
	if !ok || route.apiStarter == nil {
		return fmt.Errorf("shared lifecycle API server route is not bound for working directory %q", key)
	}
	return route.apiStarter(ctx, request)
}

func (router *lifecycleCommandRouter) unbind(workingDirectory string) {
	key := filepath.Clean(workingDirectory)
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[key]; exists {
		delete(router.routes, key)
		router.unbinds++
	}
}

func (router *lifecycleCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	key := filepath.Clean(request.WorkDir)
	router.mu.Lock()
	route, ok := router.routes[key]
	if ok {
		router.calls++
	}
	router.mu.Unlock()
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("shared lifecycle provider route not found for working directory %q", request.WorkDir)
	}
	return route.runner.Run(ctx, request)
}

func (router *lifecycleCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *lifecycleCommandRouter) bindCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return router.binds
}

func (router *lifecycleCommandRouter) unbindCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return router.unbinds
}

func (router *lifecycleCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return router.calls
}

func (router *lifecycleCommandRouter) apiStartCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return router.apiStarts
}

var _ platformprocess.CommandRunner = (*lifecycleCommandRouter)(nil)
var _ platformhttpserver.Starter = func(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	return (&lifecycleCommandRouter{}).Start(ctx, request)
}
