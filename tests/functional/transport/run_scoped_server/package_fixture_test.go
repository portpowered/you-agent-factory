package run_scoped_server_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const runScopedProcessCloseTimeout = 5 * time.Second

type runScopedRootProfile string

const (
	runScopedControlledProfile runScopedRootProfile = "controlled"
	runScopedValidationProfile runScopedRootProfile = "validation"
	runScopedProductionProfile runScopedRootProfile = "production"
)

type runScopedCaseContextKey struct{}

type runScopedCaseState struct {
	listenerStarts atomic.Int32
	listenerStops  atomic.Int32
	browserCalls   atomic.Int32
	edgeEffects    atomic.Int32
	start          func(context.Context, platformhttpserver.StartRequest) error
}

type runScopedCase struct {
	process process
	*runScopedCaseState
}

type runScopedCaseProcess struct {
	fixture *runScopedRootFixture
	state   *runScopedCaseState
}

func (p *runScopedCaseProcess) Execute(input root.Input) error {
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	input.Context = context.WithValue(ctx, runScopedCaseContextKey{}, p.state)
	return p.fixture.execute(p.state, input)
}

type runScopedRootFixture struct {
	profile runScopedRootProfile

	buildMu    sync.Mutex
	executeMu  sync.Mutex
	process    support.ApplicationProcess
	router     *runScopedEdgeRouter
	buildErr   error
	closeOnce  sync.Once
	closeErr   error
	rootBuilds atomic.Int32
	rootCloses atomic.Int32
}

var runScopedFixtures = struct {
	controlled runScopedRootFixture
	validation runScopedRootFixture
	production runScopedRootFixture
}{
	controlled: runScopedRootFixture{profile: runScopedControlledProfile},
	validation: runScopedRootFixture{profile: runScopedValidationProfile},
	production: runScopedRootFixture{profile: runScopedProductionProfile},
}

func TestMain(m *testing.M) {
	exitCode := m.Run()

	if err := closeRunScopedFixtures(); err != nil {
		fmt.Fprintf(os.Stderr, "close run-scoped package roots: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	if err := runScopedFixtureCleanupError(); err != nil {
		fmt.Fprintf(os.Stderr, "run-scoped package fixture cleanup: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	fmt.Fprintf(os.Stderr, "GATE-RS-TOPOLOGY: %s\n", runScopedFixtureSummary())
	os.Exit(exitCode)
}

func newRunScopedControlledCase(t testing.TB, id string) *runScopedCase {
	t.Helper()
	return runScopedFixtures.controlled.newCase(t, id)
}

func newRunScopedValidationCase(t testing.TB, id string) *runScopedCase {
	t.Helper()
	return runScopedFixtures.validation.newCase(t, id)
}

func newRunScopedProductionCase(t testing.TB, id string) *runScopedCase {
	t.Helper()
	return runScopedFixtures.production.newCase(t, id)
}

func (fixture *runScopedRootFixture) newCase(t testing.TB, id string) *runScopedCase {
	t.Helper()
	fixture.ensure(t)
	state := &runScopedCaseState{}
	return &runScopedCase{
		process:            &runScopedCaseProcess{fixture: fixture, state: state},
		runScopedCaseState: state,
	}
}

func (fixture *runScopedRootFixture) ensure(t testing.TB) {
	t.Helper()
	fixture.buildMu.Lock()
	defer fixture.buildMu.Unlock()
	if fixture.process == nil && fixture.buildErr == nil {
		fixture.router = newRunScopedEdgeRouter(fixture.profile)
		fixture.process, fixture.buildErr = support.BuildProcessWithContext(
			context.Background(), fixture.router.edges(),
		)
		if fixture.buildErr == nil {
			fixture.rootBuilds.Add(1)
		}
	}
	if fixture.buildErr != nil {
		t.Fatalf("BuildProcess() for %s run-scoped cases: %v", fixture.profile, fixture.buildErr)
	}
}

func (fixture *runScopedRootFixture) execute(state *runScopedCaseState, input root.Input) error {
	fixture.executeMu.Lock()
	defer fixture.executeMu.Unlock()
	restore := fixture.router.setActive(state)
	defer restore()
	return fixture.process.Execute(input)
}

func (fixture *runScopedRootFixture) close(ctx context.Context) error {
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

func closeRunScopedFixtures() error {
	fixtures := []*runScopedRootFixture{
		&runScopedFixtures.controlled,
		&runScopedFixtures.validation,
		&runScopedFixtures.production,
	}
	var errs []error
	for _, fixture := range fixtures {
		ctx, cancel := context.WithTimeout(context.Background(), runScopedProcessCloseTimeout)
		if err := fixture.close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s root: %w", fixture.profile, err))
		}
		cancel()
	}
	return errors.Join(errs...)
}

func runScopedFixtureCleanupError() error {
	fixtures := []*runScopedRootFixture{
		&runScopedFixtures.controlled,
		&runScopedFixtures.validation,
		&runScopedFixtures.production,
	}
	var errs []error
	for _, fixture := range fixtures {
		if fixture.rootBuilds.Load() != fixture.rootCloses.Load() {
			errs = append(errs, fmt.Errorf(
				"%s roots built/closed = %d/%d",
				fixture.profile, fixture.rootBuilds.Load(), fixture.rootCloses.Load(),
			))
		}
		if fixture.router != nil {
			if starts, stops := fixture.router.listenerStarts.Load(), fixture.router.listenerStops.Load(); starts != stops {
				errs = append(errs, fmt.Errorf(
					"%s listener starts/stops = %d/%d", fixture.profile, starts, stops,
				))
			}
			if active := fixture.router.activeListeners.Load(); active != 0 {
				errs = append(errs, fmt.Errorf(
					"%s active listeners after package cleanup = %d", fixture.profile, active,
				))
			}
		}
	}
	return errors.Join(errs...)
}

func runScopedFixtureSummary() string {
	fixtures := []*runScopedRootFixture{
		&runScopedFixtures.controlled,
		&runScopedFixtures.validation,
		&runScopedFixtures.production,
	}
	var summary string
	for index, fixture := range fixtures {
		if index > 0 {
			summary += "; "
		}
		summary += fmt.Sprintf(
			"%s roots=%d/%d", fixture.profile,
			fixture.rootBuilds.Load(), fixture.rootCloses.Load(),
		)
		if fixture.router != nil {
			summary += fmt.Sprintf(
				" listeners=%d/%d active=%d browsers=%d generators=%d",
				fixture.router.listenerStarts.Load(),
				fixture.router.listenerStops.Load(),
				fixture.router.activeListeners.Load(),
				fixture.router.browserCalls.Load(),
				fixture.router.generatorCalls.Load(),
			)
		}
	}
	return summary
}

type runScopedEdgeRouter struct {
	profile         runScopedRootProfile
	provider        platformprocess.CommandRunner
	withGenerator   bool
	activeMu        sync.RWMutex
	active          *runScopedCaseState
	nextSessionID   atomic.Uint64
	listenerStarts  atomic.Int32
	listenerStops   atomic.Int32
	activeListeners atomic.Int32
	browserCalls    atomic.Int32
	generatorCalls  atomic.Int32
}

func newRunScopedEdgeRouter(profile runScopedRootProfile) *runScopedEdgeRouter {
	router := &runScopedEdgeRouter{profile: profile}
	if profile == runScopedControlledProfile {
		providerResult := platformprocess.CommandResult{
			Stdout: []byte("{\"decision\":\"accepted\",\"feedback\":\"\",\"output\":\"mock worker accepted\"}"),
		}
		// The queue covers repeated focused runs in one test process while
		// keeping every provider response identical and JSON-valid.
		providerResults := make([]platformprocess.CommandResult, 32)
		for index := range providerResults {
			providerResults[index] = providerResult
		}
		router.provider = support.NewShapedProviderCommandRunner(providerResults...)
	}
	router.withGenerator = profile == runScopedValidationProfile
	return router
}

func (router *runScopedEdgeRouter) edges() serviceedges.Edges {
	if router.profile == runScopedProductionProfile {
		return serviceedges.Edges{}
	}
	edges := serviceedges.Edges{
		APIServerStarter: router.start,
		BrowserOpener:    router.openBrowser,
	}
	if router.provider != nil {
		edges.ProviderCommandRunner = router.provider
	}
	if router.withGenerator {
		edges.FactorySessionIDGenerator = router.nextFactorySessionID
	}
	return edges
}

func (router *runScopedEdgeRouter) setActive(state *runScopedCaseState) func() {
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

func (router *runScopedEdgeRouter) state(ctx context.Context) *runScopedCaseState {
	if ctx != nil {
		if state, ok := ctx.Value(runScopedCaseContextKey{}).(*runScopedCaseState); ok {
			return state
		}
	}
	router.activeMu.RLock()
	defer router.activeMu.RUnlock()
	return router.active
}

func (router *runScopedEdgeRouter) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	state := router.state(ctx)
	if state == nil {
		return fmt.Errorf("run-scoped API starter has no active case")
	}
	state.listenerStarts.Add(1)
	state.edgeEffects.Add(1)
	router.listenerStarts.Add(1)
	router.activeListeners.Add(1)
	defer func() {
		state.listenerStops.Add(1)
		router.listenerStops.Add(1)
		router.activeListeners.Add(-1)
	}()
	if state.start != nil {
		return state.start(ctx, request)
	}
	if request.OnBound != nil {
		request.OnBound(platformhttpserver.Binding{Port: request.Port})
	}
	<-ctx.Done()
	return ctx.Err()
}

func (router *runScopedEdgeRouter) openBrowser(ctx context.Context, target string) error {
	state := router.state(ctx)
	if state == nil {
		return fmt.Errorf("run-scoped browser opener has no active case for %q", target)
	}
	state.browserCalls.Add(1)
	state.edgeEffects.Add(1)
	router.browserCalls.Add(1)
	return nil
}

func (router *runScopedEdgeRouter) nextFactorySessionID() string {
	state := router.state(nil)
	if state != nil {
		state.edgeEffects.Add(1)
	}
	router.generatorCalls.Add(1)
	sequence := router.nextSessionID.Add(1)
	return fmt.Sprintf("run-scoped-session-%d", sequence)
}
