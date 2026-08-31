package stdio_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const acpProcessCloseTimeout = 5 * time.Second

type acpCaseContextKey struct{}

type acpCaseState struct {
	id       string
	runner   platformprocess.CommandRunner
	executes atomic.Int32
}

type acpCase struct {
	process    support.Process
	home       string
	cwd        string
	factoryDir string
}

type acpCaseProcess struct {
	fixture *acpRootFixture
	state   *acpCaseState
}

func (p *acpCaseProcess) Execute(input root.Input) error {
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	input.Context = context.WithValue(ctx, acpCaseContextKey{}, p.state)
	return p.fixture.execute(p.state, input)
}

type acpRootFixture struct {
	buildMu    sync.Mutex
	executeMu  sync.Mutex
	process    support.ApplicationProcess
	router     *acpProviderRouter
	buildErr   error
	closeOnce  sync.Once
	closeErr   error
	pathsMu    sync.Mutex
	paths      []string
	pathsOnce  sync.Once
	pathsErr   error
	rootBuilds atomic.Int32
	rootCloses atomic.Int32
}

var acpControlFixture acpRootFixture
var acpControlCaseSequence atomic.Uint64

type acpRootCounter struct {
	label      string
	rootBuilds atomic.Int32
	rootCloses atomic.Int32
}

var (
	acpControlIsolatedRootCounter = acpRootCounter{label: "control-isolated"}
	acpTranscriptRootCounter      = acpRootCounter{label: "transcript"}
	acpOutboundFailureCounter     = acpRootCounter{label: "outbound-failure"}
	acpPermissionRootCounter      = acpRootCounter{label: "permission"}
)

func TestMain(m *testing.M) {
	exitCode := m.Run()

	closeCtx, cancel := context.WithTimeout(context.Background(), acpProcessCloseTimeout)
	if err := acpControlFixture.close(closeCtx); err != nil {
		fmt.Fprintf(os.Stderr, "close ACP shared package root: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	cancel()

	if err := acpControlFixture.cleanupPaths(); err != nil {
		fmt.Fprintf(os.Stderr, "remove ACP shared case paths: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	if err := acpFixtureCleanupError(); err != nil {
		fmt.Fprintf(os.Stderr, "ACP package fixture cleanup: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	fmt.Fprintf(os.Stderr, "GATE-ACP-TOPOLOGY: %s\n", acpFixtureSummary())
	os.Exit(exitCode)
}

func newACPControlCase(
	t *testing.T,
	id string,
	runner platformprocess.CommandRunner,
) *acpCase {
	t.Helper()

	home := acpControlFixture.newTempDir(t, "you-acp-shared-home-")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cwd := acpControlFixture.newTempDir(t, "you-acp-shared-cwd-")
	targetID := fixtureFactoryTargetID
	if id != "ACP-05" {
		sequence := acpControlCaseSequence.Add(1)
		targetID = fmt.Sprintf(
			"%s@%s-%d/%s",
			operatorsettings.ACPFactoryTargetNamespace,
			fixtureFactoryScope,
			sequence,
			fixtureFactoryName,
		)
	}
	factoryDir := seedFixtureFactoryForTarget(t, cwd, targetID)
	support.SeedACPAgentProfile(t, home, targetID, []string{targetID})

	return acpControlFixture.newCase(t, id, runner, home, cwd, factoryDir)
}

func newACPIsolatedControlCase(
	t *testing.T,
	id string,
	runner platformprocess.CommandRunner,
) *acpCase {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cwd := t.TempDir()
	factoryDir := seedFixtureFactory(t, cwd)
	support.SeedACPAgentProfile(t, home, fixtureFactoryTargetID, []string{fixtureFactoryTargetID})
	process := buildACPIsolatedProcess(t, &acpControlIsolatedRootCounter, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	return &acpCase{
		process:    process,
		home:       home,
		cwd:        cwd,
		factoryDir: factoryDir,
	}
}

func (fixture *acpRootFixture) newTempDir(t testing.TB, pattern string) string {
	t.Helper()
	directory, err := os.MkdirTemp("", pattern)
	if err != nil {
		t.Fatalf("MkdirTemp(%q): %v", pattern, err)
	}
	fixture.pathsMu.Lock()
	fixture.paths = append(fixture.paths, directory)
	fixture.pathsMu.Unlock()
	return directory
}

func (fixture *acpRootFixture) newCase(
	t testing.TB,
	id string,
	runner platformprocess.CommandRunner,
	home string,
	cwd string,
	factoryDir string,
) *acpCase {
	t.Helper()
	fixture.ensure(t)
	state := &acpCaseState{id: id, runner: runner}
	return &acpCase{
		process:    &acpCaseProcess{fixture: fixture, state: state},
		home:       home,
		cwd:        cwd,
		factoryDir: factoryDir,
	}
}

func (fixture *acpRootFixture) ensure(t testing.TB) {
	t.Helper()
	fixture.buildMu.Lock()
	defer fixture.buildMu.Unlock()
	if fixture.process == nil && fixture.buildErr == nil {
		fixture.router = &acpProviderRouter{}
		fixture.process, fixture.buildErr = support.BuildProcessWithContext(
			context.Background(), serviceedges.Edges{
				ProviderCommandRunner: fixture.router,
			},
		)
		if fixture.buildErr == nil {
			fixture.rootBuilds.Add(1)
		}
	}
	if fixture.buildErr != nil {
		t.Fatalf("BuildProcess() for shared ACP cases: %v", fixture.buildErr)
	}
}

func (fixture *acpRootFixture) execute(state *acpCaseState, input root.Input) error {
	fixture.executeMu.Lock()
	defer fixture.executeMu.Unlock()
	restore := fixture.router.setActive(state)
	defer restore()
	state.executes.Add(1)
	return fixture.process.Execute(input)
}

func (fixture *acpRootFixture) close(ctx context.Context) error {
	fixture.buildMu.Lock()
	process := fixture.process
	fixture.buildMu.Unlock()
	if process == nil {
		return nil
	}
	fixture.closeOnce.Do(func() {
		fixture.closeErr = process.Close(ctx)
		fixture.rootCloses.Add(1)
	})
	return fixture.closeErr
}

func (fixture *acpRootFixture) cleanupPaths() error {
	fixture.pathsOnce.Do(func() {
		fixture.pathsMu.Lock()
		paths := append([]string(nil), fixture.paths...)
		fixture.pathsMu.Unlock()
		for index := len(paths) - 1; index >= 0; index-- {
			if err := os.RemoveAll(paths[index]); err != nil {
				fixture.pathsErr = errors.Join(fixture.pathsErr, fmt.Errorf(
					"remove %q: %w", paths[index], err,
				))
			}
		}
	})
	return fixture.pathsErr
}

func buildACPIsolatedProcess(
	t *testing.T,
	counter *acpRootCounter,
	edges serviceedges.Edges,
) support.ApplicationProcess {
	t.Helper()
	process, err := support.BuildProcessWithContext(t.Context(), edges)
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	counter.rootBuilds.Add(1)
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), acpProcessCloseTimeout)
		defer cancel()
		if err := process.Close(closeCtx); err != nil {
			t.Errorf("close isolated ACP %s root: %v", counter.label, err)
		}
		counter.rootCloses.Add(1)
	})
	return process
}

type acpProviderRouter struct {
	activeMu   sync.RWMutex
	active     *acpCaseState
	routeCalls atomic.Int32
}

func (router *acpProviderRouter) setActive(state *acpCaseState) func() {
	router.activeMu.Lock()
	previous := router.active
	router.active = state
	router.activeMu.Unlock()
	return func() {
		router.activeMu.Lock()
		router.active = previous
		router.activeMu.Unlock()
	}
}

func (router *acpProviderRouter) state(ctx context.Context) *acpCaseState {
	if ctx != nil {
		if state, ok := ctx.Value(acpCaseContextKey{}).(*acpCaseState); ok {
			return state
		}
	}
	router.activeMu.RLock()
	defer router.activeMu.RUnlock()
	return router.active
}

func (router *acpProviderRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	state := router.state(ctx)
	if state == nil || state.runner == nil {
		return platformprocess.CommandResult{}, errors.New("ACP provider route has no active case")
	}
	router.routeCalls.Add(1)
	return state.runner.Run(ctx, request)
}

func acpFixtureCleanupError() error {
	if acpControlFixture.rootBuilds.Load() != acpControlFixture.rootCloses.Load() {
		return fmt.Errorf(
			"shared control roots built/closed = %d/%d",
			acpControlFixture.rootBuilds.Load(), acpControlFixture.rootCloses.Load(),
		)
	}
	for _, counter := range []*acpRootCounter{
		&acpControlIsolatedRootCounter,
		&acpTranscriptRootCounter,
		&acpOutboundFailureCounter,
		&acpPermissionRootCounter,
	} {
		if counter.rootBuilds.Load() != counter.rootCloses.Load() {
			return fmt.Errorf(
				"%s roots built/closed = %d/%d",
				counter.label, counter.rootBuilds.Load(), counter.rootCloses.Load(),
			)
		}
	}
	return nil
}

func acpFixtureSummary() string {
	routes := int32(0)
	if acpControlFixture.router != nil {
		routes = acpControlFixture.router.routeCalls.Load()
	}
	return fmt.Sprintf(
		"shared-control roots=%d/%d routes=%d; control-isolated roots=%d/%d; transcript roots=%d/%d; outbound-failure roots=%d/%d; permission roots=%d/%d",
		acpControlFixture.rootBuilds.Load(), acpControlFixture.rootCloses.Load(), routes,
		acpControlIsolatedRootCounter.rootBuilds.Load(), acpControlIsolatedRootCounter.rootCloses.Load(),
		acpTranscriptRootCounter.rootBuilds.Load(), acpTranscriptRootCounter.rootCloses.Load(),
		acpOutboundFailureCounter.rootBuilds.Load(), acpOutboundFailureCounter.rootCloses.Load(),
		acpPermissionRootCounter.rootBuilds.Load(), acpPermissionRootCounter.rootCloses.Load(),
	)
}

var _ platformprocess.CommandRunner = (*acpProviderRouter)(nil)
