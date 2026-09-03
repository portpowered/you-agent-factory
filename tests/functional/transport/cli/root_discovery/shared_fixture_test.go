package root_discovery_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const rootDiscoveryProcessCloseTimeout = 5 * time.Second

var rootDiscoverySharedState struct {
	sync.Once
	fixture *rootDiscoverySharedFixture
	err     error
}

// TestMain owns the one reusable root used by the validation and serverless
// run witnesses. Lifecycle-sensitive tests construct and close their own roots
// because their assertions depend on an invocation-owned listener or startup
// boundary.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if rootDiscoverySharedState.fixture != nil {
		if err := rootDiscoverySharedState.fixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared root-discovery fixture: %v\n", err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}
	os.Exit(exitCode)
}

type rootDiscoverySharedFixture struct {
	process support.ApplicationProcess
	router  *rootDiscoveryEdgeRouter

	rootBuilds atomic.Int32
}

type rootDiscoveryInvocation struct {
	workingDirectory string
	effects          *atomic.Int32
}

type rootDiscoveryEdgeRouter struct {
	mu     sync.Mutex
	routes map[string]*rootDiscoveryInvocation

	lateCalls     atomic.Int32
	nextSessionID atomic.Uint64
}

func rootDiscoverySharedFixtureForTest(t *testing.T) *rootDiscoverySharedFixture {
	t.Helper()
	rootDiscoverySharedState.Do(func() {
		rootDiscoverySharedState.fixture, rootDiscoverySharedState.err = newRootDiscoverySharedFixture()
	})
	if rootDiscoverySharedState.err != nil {
		t.Fatalf("start shared root-discovery fixture: %v", rootDiscoverySharedState.err)
	}
	if rootDiscoverySharedState.fixture == nil {
		t.Fatal("shared root-discovery fixture is unavailable")
	}
	return rootDiscoverySharedState.fixture
}

func newRootDiscoverySharedFixture() (*rootDiscoverySharedFixture, error) {
	router := &rootDiscoveryEdgeRouter{routes: make(map[string]*rootDiscoveryInvocation)}
	fixture := &rootDiscoverySharedFixture{router: router}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return router.server(ctx, request)
		},
		BrowserOpener: func(ctx context.Context, target string) error {
			return router.browser(ctx, target)
		},
		FactorySessionIDGenerator: func() string {
			return router.sessionID()
		},
		RuntimeHostObserver: func(binding factorysessions.RuntimeHostBinding) {
			router.runtimeHost(binding)
		},
		ProviderOverride: &rootDiscoveryProviderRouter{edgeRouter: router},
	})
	if err != nil {
		return nil, fmt.Errorf("build shared root-discovery process: %w", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)
	return fixture, nil
}

func (fixture *rootDiscoverySharedFixture) executeArgs(
	t *testing.T,
	workingDirectory string,
	args []string,
	stdoutIsTTY bool,
	ctx context.Context,
	effects *atomic.Int32,
) (string, string, error) {
	return fixture.executeArgsWithOptions(t, workingDirectory, args, stdoutIsTTY, ctx, effects)
}

func (fixture *rootDiscoverySharedFixture) executeArgsWithSessionID(
	t *testing.T,
	workingDirectory string,
	args []string,
	stdoutIsTTY bool,
	ctx context.Context,
	effects *atomic.Int32,
) (string, string, error) {
	return fixture.executeArgsWithOptions(t, workingDirectory, args, stdoutIsTTY, ctx, effects)
}

func (fixture *rootDiscoverySharedFixture) executeArgsWithOptions(
	t *testing.T,
	workingDirectory string,
	args []string,
	stdoutIsTTY bool,
	ctx context.Context,
	effects *atomic.Int32,
) (string, string, error) {
	t.Helper()

	if effects == nil {
		effects = &atomic.Int32{}
	}
	invocation := &rootDiscoveryInvocation{
		workingDirectory: workingDirectory,
		effects:          effects,
	}
	if err := fixture.router.begin(invocation); err != nil {
		t.Fatalf("begin shared root-discovery invocation: %v", err)
	}
	defer func() {
		if err := fixture.router.end(invocation); err != nil {
			t.Errorf("end shared root-discovery invocation: %v", err)
		}
	}()
	args = appendExplicitRootDiscoverySession(args, fixture.router.sessionID())
	return executeFactoryArgs(t, fixture.process, workingDirectory, args, stdoutIsTTY, ctx)
}

func appendExplicitRootDiscoverySession(args []string, sessionID string) []string {
	if len(args) < 2 || args[0] != "you" || args[1] != "run" {
		return args
	}
	for _, arg := range args[2:] {
		if arg == "--session" {
			return args
		}
	}
	result := append([]string(nil), args...)
	return append(result, "--session", sessionID)
}

func (fixture *rootDiscoverySharedFixture) executeCommand(
	t *testing.T,
	workingDirectory string,
	command string,
	stdoutIsTTY bool,
	ctx context.Context,
	effects *atomic.Int32,
) (string, string, error) {
	t.Helper()
	args := []string{"you", command}
	if command == "run" {
		args = append(args, "--no-record")
	}
	return fixture.executeArgs(t, workingDirectory, args, stdoutIsTTY, ctx, effects)
}

func (fixture *rootDiscoverySharedFixture) executeCommandWithSessionID(
	t *testing.T,
	workingDirectory string,
	command string,
	stdoutIsTTY bool,
	ctx context.Context,
	effects *atomic.Int32,
) (string, string, error) {
	t.Helper()
	args := []string{"you", command}
	if command == "run" {
		args = append(args, "--no-record")
	}
	return fixture.executeArgsWithSessionID(t, workingDirectory, args, stdoutIsTTY, ctx, effects)
}

func (fixture *rootDiscoverySharedFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.router != nil {
		fixture.router.mu.Lock()
		for workingDirectory := range fixture.router.routes {
			closeErr = errors.Join(closeErr, fmt.Errorf(
				"shared root-discovery route %q remained active", workingDirectory,
			))
		}
		fixture.router.mu.Unlock()
		if lateCalls := fixture.router.lateCalls.Load(); lateCalls != 0 {
			closeErr = errors.Join(closeErr, fmt.Errorf(
				"shared root-discovery edge calls without an active route = %d",
				lateCalls,
			))
		}
	}
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), rootDiscoveryProcessCloseTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(ctx))
		cancel()
	}
	if got := fixture.rootBuilds.Load(); got != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"shared root-discovery root builds = %d, want 1",
			got,
		))
	}
	return closeErr
}

func (router *rootDiscoveryEdgeRouter) begin(invocation *rootDiscoveryInvocation) error {
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[invocation.workingDirectory]; exists {
		return fmt.Errorf("invocation %q is already active", invocation.workingDirectory)
	}
	router.routes[invocation.workingDirectory] = invocation
	return nil
}

func (router *rootDiscoveryEdgeRouter) end(invocation *rootDiscoveryInvocation) error {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.routes[invocation.workingDirectory] != invocation {
		return fmt.Errorf("invocation %q is not the active route", invocation.workingDirectory)
	}
	delete(router.routes, invocation.workingDirectory)
	return nil
}

func (router *rootDiscoveryEdgeRouter) count(workingDirectory string) error {
	router.mu.Lock()
	defer router.mu.Unlock()
	invocation := router.routes[workingDirectory]
	if invocation == nil {
		router.lateCalls.Add(1)
		return fmt.Errorf("no active root-discovery invocation for %q", workingDirectory)
	}
	invocation.effects.Add(1)
	return nil
}

func (router *rootDiscoveryEdgeRouter) server(
	_ context.Context,
	_ platformhttpserver.StartRequest,
) error {
	router.lateCalls.Add(1)
	return errors.New("shared root-discovery fixture unexpectedly started an API server")
}

func (router *rootDiscoveryEdgeRouter) browser(context.Context, string) error {
	router.lateCalls.Add(1)
	return errors.New("shared root-discovery fixture unexpectedly opened a browser")
}

func (router *rootDiscoveryEdgeRouter) sessionID() string {
	return fmt.Sprintf("root-discovery-session-%d", router.nextSessionID.Add(1))
}

func (router *rootDiscoveryEdgeRouter) runtimeHost(factorysessions.RuntimeHostBinding) {
	router.lateCalls.Add(1)
}

type rootDiscoveryProviderRouter struct {
	testutil.NativeProvider
	edgeRouter *rootDiscoveryEdgeRouter
}

func (router *rootDiscoveryProviderRouter) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if err := router.edgeRouter.count(request.WorkingDirectory); err != nil {
		return providers.ExecuteResult{}, err
	}
	return providers.ExecuteResult{}, nil
}
