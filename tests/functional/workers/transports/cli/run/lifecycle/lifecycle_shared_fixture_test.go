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

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// lifecycleSharedProcessFixture owns only the compatible, finite one-shot
// invocations. Its mutex makes the process reuse explicit: every invocation
// still owns a fresh working directory, HOME, streams, route, and provider
// runner, while the immutable root wiring is built once for the package.
type lifecycleSharedProcessFixture struct {
	process support.ApplicationProcess
	router  *lifecycleCommandRouter

	builds     atomic.Int32
	executions atomic.Int32
	active     atomic.Int32
	closes     atomic.Int32

	mu       sync.Mutex
	closeErr error
}

var lifecycleSharedFixtureState struct {
	sync.Once
	fixture *lifecycleSharedProcessFixture
	err     error
}

// TestMain closes the package-owned process after all invocation cleanup has
// completed. Hosted, cancellation, timeout, and forced-cleanup tests do not
// use this fixture and retain their dedicated process ownership.
func TestMain(m *testing.M) {
	exitCode := m.Run()

	fixture := lifecycleSharedFixtureState.fixture
	if fixture != nil {
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
	lifecycleSharedFixtureState.Do(func() {
		router := newLifecycleCommandRouter()
		process, err := support.BuildProcessWithContext(
			context.Background(),
			serviceedges.Edges{ProviderCommandRunner: router},
		)
		if err != nil {
			lifecycleSharedFixtureState.err = fmt.Errorf("build shared lifecycle process: %w", err)
			return
		}
		fixture := &lifecycleSharedProcessFixture{process: process, router: router}
		fixture.builds.Store(1)
		lifecycleSharedFixtureState.fixture = fixture
	})
	if lifecycleSharedFixtureState.err != nil {
		t.Fatalf("initialize shared lifecycle process: %v", lifecycleSharedFixtureState.err)
	}
	if lifecycleSharedFixtureState.fixture == nil {
		t.Fatal("shared lifecycle process was not initialized")
	}
	return lifecycleSharedFixtureState.fixture
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
	inputs := support.FakeInputs(t.Context(), append([]string(nil), args...))
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
	return fixture.execute(t, inputs, runner)
}

func (fixture *lifecycleSharedProcessFixture) execute(
	t testing.TB,
	inputs *support.CapturedInputs,
	runner platformprocess.CommandRunner,
) error {
	t.Helper()
	if fixture == nil || fixture.process == nil || fixture.router == nil {
		return errors.New("shared lifecycle process is unavailable")
	}
	if inputs == nil {
		return errors.New("shared lifecycle invocation inputs are required")
	}
	if runner == nil {
		return errors.New("shared lifecycle provider runner is required")
	}
	workingDirectory := filepath.Clean(inputs.Input.WorkingDirectory)
	if workingDirectory == "." || workingDirectory == "" {
		return fmt.Errorf("shared lifecycle invocation working directory is invalid: %q", inputs.Input.WorkingDirectory)
	}

	// Process.Execute is reusable, but the finite one-shot cohort is deliberately
	// serialized so mutable invocation-owned runtime state never overlaps. The
	// route and its runner remain installed only for this one Execute call.
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if err := fixture.router.bind(workingDirectory, runner); err != nil {
		return err
	}
	fixture.active.Add(1)
	fixture.executions.Add(1)
	defer func() {
		fixture.active.Add(-1)
		fixture.router.unbind(workingDirectory)
	}()
	return fixture.process.Execute(inputs.Input)
}

func (fixture *lifecycleSharedProcessFixture) close(ctx context.Context) error {
	if fixture == nil {
		return nil
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
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
		"C14 lifecycle shared topology: root_builds=%d process_executions=%d process_closes=%d active_invocations=%d provider_route_registers=%d provider_route_unbinds=%d provider_routes=%d provider_calls=%d\n",
		fixture.builds.Load(), fixture.executions.Load(), fixture.closes.Load(), fixture.active.Load(),
		fixture.router.bindCount(), fixture.router.unbindCount(), fixture.router.routeCount(), fixture.router.callCount(),
	)
	return fixture.closeErr
}

type lifecycleCommandRoute struct {
	runner platformprocess.CommandRunner
}

type lifecycleCommandRouter struct {
	mu sync.Mutex

	routes  map[string]lifecycleCommandRoute
	binds   int
	unbinds int
	calls   int
}

func newLifecycleCommandRouter() *lifecycleCommandRouter {
	return &lifecycleCommandRouter{routes: make(map[string]lifecycleCommandRoute)}
}

func (router *lifecycleCommandRouter) bind(
	workingDirectory string,
	runner platformprocess.CommandRunner,
) error {
	key := filepath.Clean(workingDirectory)
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[key]; exists {
		return fmt.Errorf("shared lifecycle provider route %q is already bound", key)
	}
	router.routes[key] = lifecycleCommandRoute{runner: runner}
	router.binds++
	return nil
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

var _ platformprocess.CommandRunner = (*lifecycleCommandRouter)(nil)
