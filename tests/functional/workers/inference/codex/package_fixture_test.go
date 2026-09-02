package codex

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// codexPackageFixture owns the one root-built process used by every eligible
// Codex scenario in this package. Scenario-specific Factory directories and
// command results remain independent; only immutable process wiring is shared.
type codexPackageFixture struct {
	rootDir    string
	hostDir    string
	baseURL    string
	process    support.ApplicationProcess
	command    *codexPackageProcessCommand
	apiStopped <-chan struct{}
	apiStarts  *atomic.Int32
	router     *codexCommandRouter
	identities *codexIdentityGenerator

	conductorScenarios []codexConductorScenario
	goldenScenarios    []codexGoldenScenario
	worktreeScenarios  []codexWorktreeScenario
	runners            []*codexScenarioCommandRunner
	groupsSeen         map[string]bool
}

type codexPackageProcess struct {
	process    support.ApplicationProcess
	command    *codexPackageProcessCommand
	apiStopped <-chan struct{}
	apiStarts  *atomic.Int32
	baseURL    string
}

// The test runner invokes top-level tests serially. The mutex protects the
// package fixture from future parallel test registration and lets TestMain
// close the fixture after m.Run without relying on a test's t.Cleanup order.
var codexPackageFixtureState struct {
	sync.Mutex
	fixture *codexPackageFixture
}

// TestMain is the package-level lifecycle boundary for the shared process.
// Individual tests stop only their invocation command; this finalizer closes
// the one process after every scenario has released its session and stream.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeCodexPackageFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "Codex package fixture cleanup failed: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

type codexPackageProcessCommand struct {
	cancel context.CancelFunc
	done   chan error
}

func startCodexPackageProcess(
	process support.ApplicationProcess,
	inputs *support.CapturedInputs,
) *codexPackageProcessCommand {
	parent := inputs.Input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input := inputs.Input
	input.Context = ctx
	command := &codexPackageProcessCommand{
		cancel: cancel,
		done:   make(chan error, 1),
	}
	go func() {
		command.done <- process.Execute(input)
	}()
	return command
}

func (command *codexPackageProcessCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	select {
	case err := <-command.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop shared Codex process command: %w", err)
		}
		return nil
	case <-time.After(codexConductorRunTimeout):
		return fmt.Errorf("timed out waiting for shared Codex process command shutdown")
	}
}

func ensureCodexPackageFixture(t *testing.T) *codexPackageFixture {
	t.Helper()

	codexPackageFixtureState.Lock()
	fixture := codexPackageFixtureState.fixture
	codexPackageFixtureState.Unlock()
	if fixture != nil {
		return fixture
	}

	// Top-level Go tests are sequential, so the first eligible group can create
	// the package fixture. The state is published only after all routes and the
	// continuous process are ready.
	fixture = newCodexPackageFixture(t)
	codexPackageFixtureState.Lock()
	if codexPackageFixtureState.fixture == nil {
		codexPackageFixtureState.fixture = fixture
	} else {
		fixture = codexPackageFixtureState.fixture
	}
	codexPackageFixtureState.Unlock()
	return fixture
}

func newCodexPackageFixture(t *testing.T) *codexPackageFixture {
	t.Helper()

	rootDir, err := os.MkdirTemp("", "c04-codex-package-")
	if err != nil {
		t.Fatalf("create shared Codex package root: %v", err)
	}
	keepRoot := false
	defer func() {
		if !keepRoot {
			_ = os.RemoveAll(rootDir)
		}
	}()

	conductorScenarios := newCodexScenariosAt(t, rootDir)
	goldenScenarios := newCodexGoldenScenariosAt(t, rootDir)
	worktreeScenarios := newCodexWorktreeScenariosAt(t, rootDir)
	hostDir := newCodexHostDirAt(t, rootDir)
	routes, runners := newCodexPackageRoutes(
		t,
		conductorScenarios,
		goldenScenarios,
		worktreeScenarios,
	)
	router, err := newCodexCommandRouter(routes)
	if err != nil {
		t.Fatalf("new shared Codex command router: %v", err)
	}
	identities := &codexIdentityGenerator{}
	running := newCodexPackageProcess(t, rootDir, hostDir, router, identities)

	keepRoot = true
	return &codexPackageFixture{
		rootDir:            rootDir,
		hostDir:            hostDir,
		baseURL:            running.baseURL,
		process:            running.process,
		command:            running.command,
		apiStopped:         running.apiStopped,
		apiStarts:          running.apiStarts,
		router:             router,
		identities:         identities,
		conductorScenarios: conductorScenarios,
		goldenScenarios:    goldenScenarios,
		worktreeScenarios:  worktreeScenarios,
		runners:            runners,
		groupsSeen:         make(map[string]bool, 3),
	}
}

func newCodexPackageRoutes(
	t *testing.T,
	conductorScenarios []codexConductorScenario,
	goldenScenarios []codexGoldenScenario,
	worktreeScenarios []codexWorktreeScenario,
) ([]codexCommandRoute, []*codexScenarioCommandRunner) {
	t.Helper()

	routes := make([]codexCommandRoute, 0,
		len(conductorScenarios)+len(goldenScenarios)+len(worktreeScenarios),
	)
	runners := make([]*codexScenarioCommandRunner, 0, cap(routes))
	for _, scenario := range conductorScenarios {
		routes = append(routes, codexCommandRoute{
			selector: scenario.factoryDir,
			label:    scenario.name,
			runner:   scenario.runner,
		})
		runners = append(runners, scenario.runner)
	}
	for _, scenario := range goldenScenarios {
		routes = append(routes, codexCommandRoute{
			selector: scenario.factoryDir,
			label:    scenario.name,
			runner:   scenario.runner,
		})
		runners = append(runners, scenario.runner)
	}
	for _, scenario := range worktreeScenarios {
		routes = append(routes, codexCommandRoute{
			selector: scenario.checkoutPath,
			label:    scenario.name,
			runner:   scenario.runner,
		})
		runners = append(runners, scenario.runner)
	}
	return routes, runners
}

func newCodexPackageProcess(
	t *testing.T,
	rootDir string,
	hostDir string,
	router *codexCommandRouter,
	identities *codexIdentityGenerator,
) codexPackageProcess {
	t.Helper()

	api := support.NewProcessAPIServer()
	apiStopped := make(chan struct{})
	var apiStopOnce sync.Once
	apiStarts := &atomic.Int32{}
	// TestMain owns this package-wide process. Do not register its cleanup on
	// the first top-level test that happens to initialize the fixture.
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: router,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			apiStarts.Add(1)
			err := api.Start(ctx, request)
			apiStopOnce.Do(func() { close(apiStopped) })
			return err
		},
		FactorySessionIDGenerator:                identities.nextSessionID,
		FactorySessionRuntimeInstanceIDGenerator: identities.nextRuntimeID,
		FactorySessionResponseEventIDGenerator:   identities.nextResponseEventID,
	})
	if err != nil {
		t.Fatalf("build shared Codex package process: %v", err)
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
	homeDir := filepath.Join(rootDir, "home")
	configureCodexProcessInputs(t, inputs, hostDir, homeDir)
	command := startCodexPackageProcess(process, inputs)
	baseURL := api.WaitForURL(t)
	assertCodexHostDefaultSession(t, baseURL)
	return codexPackageProcess{
		process:    process,
		command:    command,
		apiStopped: apiStopped,
		apiStarts:  apiStarts,
		baseURL:    baseURL,
	}
}

func (fixture *codexPackageFixture) beginGroup(t *testing.T, group string) {
	t.Helper()
	if fixture.groupsSeen[group] {
		fixture.resetScenarioState(t)
		fixture.groupsSeen = make(map[string]bool, 3)
	}
	fixture.groupsSeen[group] = true
}

func (fixture *codexPackageFixture) resetScenarioState(t *testing.T) {
	t.Helper()
	fixture.router.resetCalls()
	for _, runner := range fixture.runners {
		runner.Reset()
	}
	for _, scenario := range fixture.conductorScenarios {
		resetCodexConductorScenario(t, scenario)
	}
	for _, scenario := range fixture.goldenScenarios {
		resetCodexGoldenScenario(t, scenario)
	}
	for _, scenario := range fixture.worktreeScenarios {
		resetCodexWorktreeScenario(t, scenario)
	}
}

func closeCodexPackageFixture() error {
	codexPackageFixtureState.Lock()
	fixture := codexPackageFixtureState.fixture
	codexPackageFixtureState.Unlock()
	if fixture == nil {
		return nil
	}

	var errs []error
	if err := fixture.command.stop(); err != nil {
		errs = append(errs, err)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), codexConductorRunTimeout)
	if err := fixture.process.Close(closeCtx); err != nil {
		errs = append(errs, fmt.Errorf("close shared Codex application process: %w", err))
	}
	cancel()
	select {
	case <-fixture.apiStopped:
	case <-time.After(codexConductorRunTimeout):
		errs = append(errs, fmt.Errorf("shared Codex API server did not close after process cleanup"))
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		errs = append(errs, fmt.Errorf("shared Codex API server starts = %d, want exactly one", got))
	}
	for _, runner := range fixture.runners {
		if got := runner.ActiveCallCount(); got != 0 {
			errs = append(errs, fmt.Errorf("active Codex command calls after package cleanup = %d", got))
		}
	}
	for _, scenario := range fixture.worktreeScenarios {
		if scenario.runner.CallCount() == 0 {
			// Focused conductor or golden runs intentionally do not execute the
			// worktree routes; their package-root cleanup is handled below.
			continue
		}
		if _, err := os.Stat(scenario.checkoutPath); !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("worktree checkout %q remains after scenario cleanup: %v", scenario.checkoutPath, err))
		}
		if _, err := os.Stat(scenario.repoRoot); !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("worktree repository %q remains after scenario cleanup: %v", scenario.repoRoot, err))
		}
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove shared Codex package root %q: %w", fixture.rootDir, err))
	} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("shared Codex package root %q remains after cleanup: %v", fixture.rootDir, err))
	}
	return errors.Join(errs...)
}

func copyCodexFixtureDir(t *testing.T, srcDir, parentDir, label string) string {
	t.Helper()

	dst, err := os.MkdirTemp(parentDir, label+"-")
	if err != nil {
		t.Fatalf("create Codex fixture directory: %v", err)
	}
	err = filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(dst, relative)
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
		t.Fatalf("copy Codex fixture %q: %v", srcDir, err)
	}
	return dst
}

func overwriteCodexFixtureDir(t *testing.T, srcDir, dstDir string) {
	t.Helper()
	if err := os.RemoveAll(dstDir); err != nil {
		t.Fatalf("remove Codex fixture directory %q for reset: %v", dstDir, err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("recreate Codex fixture directory %q: %v", dstDir, err)
	}
	err := filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(dstDir, relative)
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
		t.Fatalf("reset Codex fixture %q: %v", dstDir, err)
	}
}
