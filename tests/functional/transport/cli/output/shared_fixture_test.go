package output_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const machineOutputProcessCloseTimeout = 5 * time.Second

var machineOutputShared struct {
	once      sync.Once
	fixture   *machineOutputFixture
	err       error
	rootBuild atomic.Int32
}

// TestMain owns the machine-readable output process for the package. The
// process is built lazily so isolated lifecycle tests retain their own roots.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if fixture := machineOutputShared.fixture; fixture != nil {
		closeContext, cancel := context.WithTimeout(context.Background(), machineOutputProcessCloseTimeout)
		closeErr := fixture.close(closeContext)
		cancel()
		if closeErr != nil && exitCode == 0 {
			fmt.Fprintf(os.Stderr, "close shared CLI output process: %v\n", closeErr)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

type machineOutputFixture struct {
	process        support.ApplicationProcess
	router         *machineOutputCommandRouter
	effects        *machineOutputEffects
	factorySources map[string]string
	sourceRoot     string
	mu             sync.Mutex
	active         atomic.Int32
}

type machineOutputPreActivationInvocation struct {
	inputs         *support.CapturedInputs
	providerRunner platformprocess.CommandRunner
}

type machineOutputPreActivationBatchResult struct {
	errors        []error
	maxConcurrent int32
}

type machineOutputPreActivationResult struct {
	index int
	err   error
}

type machineOutputEffects struct {
	mu      sync.Mutex
	counter *atomic.Int32
	session atomic.Uint64
}

func (effects *machineOutputEffects) observe() {
	effects.mu.Lock()
	defer effects.mu.Unlock()
	if effects.counter != nil {
		effects.counter.Add(1)
	}
}

func (effects *machineOutputEffects) setCounter(counter *atomic.Int32) {
	effects.mu.Lock()
	defer effects.mu.Unlock()
	effects.counter = counter
}

func (effects *machineOutputEffects) nextSessionID() string {
	return fmt.Sprintf("machine-output-session-%d", effects.session.Add(1))
}

type machineOutputCommandRouter struct {
	mu     sync.Mutex
	nextID uint64
	routes map[string]machineOutputRoute
}

type machineOutputRoute struct {
	workingDirectory string
	runner           platformprocess.CommandRunner
}

func newMachineOutputCommandRouter() *machineOutputCommandRouter {
	return &machineOutputCommandRouter{routes: make(map[string]machineOutputRoute)}
}

func (router *machineOutputCommandRouter) bind(
	workingDirectory string,
	runner platformprocess.CommandRunner,
) string {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.nextID++
	routeID := fmt.Sprintf("machine-output-route-%d", router.nextID)
	router.routes[routeID] = machineOutputRoute{
		workingDirectory: workingDirectory,
		runner:           runner,
	}
	return routeID
}

func (router *machineOutputCommandRouter) unbind(routeID string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	delete(router.routes, routeID)
}

func (router *machineOutputCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	router.mu.Lock()
	for _, route := range router.routes {
		if filepath.Clean(route.workingDirectory) != filepath.Clean(request.WorkDir) {
			continue
		}
		runner := route.runner
		router.mu.Unlock()
		return runner.Run(ctx, request)
	}
	router.mu.Unlock()
	return platformprocess.CommandResult{}, fmt.Errorf(
		"CLI output provider route not found for working directory %q",
		request.WorkDir,
	)
}

func (router *machineOutputCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func sharedMachineOutputFixture(t testing.TB) *machineOutputFixture {
	t.Helper()
	machineOutputShared.once.Do(func() {
		router := newMachineOutputCommandRouter()
		effects := &machineOutputEffects{}
		machineOutputShared.rootBuild.Add(1)
		process, err := support.BuildProcessWithContext(
			context.Background(),
			serviceedges.Edges{
				ProviderCommandRunner: router,
				APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
					effects.observe()
					return nil
				},
				BrowserOpener: func(context.Context, string) error {
					effects.observe()
					return nil
				},
				RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
					effects.observe()
				},
				FactorySessionIDGenerator: effects.nextSessionID,
			},
		)
		if err != nil {
			machineOutputShared.err = err
			return
		}
		sourceRoot, sourceHome, sourceErr := materializeMachineOutputSources(t, process)
		if sourceErr != nil {
			_ = process.Close(context.Background())
			machineOutputShared.err = sourceErr
			return
		}
		machineOutputShared.fixture = &machineOutputFixture{
			process:        process,
			router:         router,
			effects:        effects,
			factorySources: machineOutputFactorySources(sourceHome),
			sourceRoot:     sourceRoot,
		}
	})
	if machineOutputShared.err != nil {
		t.Fatalf("BuildProcess() for shared CLI output fixture: %v", machineOutputShared.err)
	}
	if machineOutputShared.fixture == nil {
		t.Fatal("shared CLI output fixture was not initialized")
	}
	return machineOutputShared.fixture
}

func (fixture *machineOutputFixture) close(ctx context.Context) error {
	if fixture == nil || fixture.process == nil {
		return nil
	}
	var closeErrors []error
	if builds := machineOutputShared.rootBuild.Load(); builds != 1 {
		closeErrors = append(closeErrors, fmt.Errorf("shared CLI output root builds: %d, want 1", builds))
	}
	if routes := fixture.router.routeCount(); routes != 0 {
		closeErrors = append(closeErrors, fmt.Errorf("shared CLI output provider routes remaining at close: %d", routes))
	}
	if active := fixture.active.Load(); active != 0 {
		closeErrors = append(closeErrors, fmt.Errorf("shared CLI output invocations active at close: %d", active))
	}
	if err := fixture.process.Close(ctx); err != nil {
		closeErrors = append(closeErrors, err)
	}
	if fixture.sourceRoot != "" {
		if err := os.RemoveAll(fixture.sourceRoot); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("remove shared CLI output source root: %w", err))
		} else if _, err := os.Stat(fixture.sourceRoot); err == nil {
			closeErrors = append(closeErrors, fmt.Errorf("shared CLI output source root remains: %s", fixture.sourceRoot))
		} else if !errors.Is(err, os.ErrNotExist) {
			closeErrors = append(closeErrors, fmt.Errorf("inspect shared CLI output source root: %w", err))
		}
	}
	return errors.Join(closeErrors...)
}

func materializeMachineOutputSources(
	t testing.TB,
	process support.ApplicationProcess,
) (sourceRoot, sourceHome string, err error) {
	t.Helper()
	sourceRoot, err = os.MkdirTemp("", "you-cli-output-fixture-")
	if err != nil {
		return "", "", fmt.Errorf("create shared CLI output source root: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(sourceRoot)
		}
	}()
	sourceHome = filepath.Join(sourceRoot, "home")
	workingDirectory := filepath.Join(sourceRoot, "work")
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		return "", "", fmt.Errorf("create shared CLI output source work directory: %w", err)
	}
	env := append(os.Environ(), "HOME="+sourceHome, "USERPROFILE="+sourceHome)
	support.InstallPackagedFactoryWithProcess(
		t,
		process,
		env,
		workingDirectory,
		"@you/goal",
	)
	return sourceRoot, sourceHome, nil
}

func machineOutputFactorySources(sourceHome string) map[string]string {
	factoriesRoot := filepath.Join(sourceHome, ".you-agent-factory", "factories")
	return map[string]string{
		"@you/goal":          filepath.Join(factoriesRoot, "@you", "goal"),
		"@you/plan-parallel": filepath.Join(factoriesRoot, "@you", "plan-parallel"),
	}
}

func runMachineOutputInvocation(
	t testing.TB,
	args []string,
	packagedFactoryName string,
	providerRunner platformprocess.CommandRunner,
	effectsCounter *atomic.Int32,
) (stdout, stderr string, err error) {
	t.Helper()
	fixture, inputs := newMachineOutputInputs(t, args, packagedFactoryName)
	err = fixture.execute(inputs, providerRunner, effectsCounter)
	return inputs.Stdout(), inputs.Stderr(), err
}

func newMachineOutputInputs(
	t testing.TB,
	args []string,
	packagedFactoryName string,
) (*machineOutputFixture, *support.CapturedInputs) {
	t.Helper()
	fixture := sharedMachineOutputFixture(t)
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs := support.FakeInputs(t.Context(), append([]string(nil), args...))
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	if packagedFactoryName != "" {
		sourceDir := fixture.factorySources[packagedFactoryName]
		if sourceDir == "" {
			t.Fatalf("no shared packaged Factory source for %q", packagedFactoryName)
		}
		support.CopyFactoryAsNamed(t, sourceDir, homeDir, packagedFactoryName)
	}
	return fixture, inputs
}

// execute serializes shared-process calls while keeping each invocation's
// route, streams, environment, home, and session state independent.
func (fixture *machineOutputFixture) execute(
	inputs *support.CapturedInputs,
	providerRunner platformprocess.CommandRunner,
	effectsCounter *atomic.Int32,
) error {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.effects.setCounter(effectsCounter)
	defer fixture.effects.setCounter(nil)
	var routeID string
	if providerRunner != nil {
		routeID = fixture.router.bind(inputs.Input.WorkingDirectory, providerRunner)
		defer fixture.router.unbind(routeID)
	}
	fixture.active.Add(1)
	defer fixture.active.Add(-1)
	return fixture.process.Execute(inputs.Input)
}

func assertMachineOutputFixtureIdle(t testing.TB, fixture *machineOutputFixture) {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if routes := fixture.router.routeCount(); routes != 0 {
		t.Fatalf("shared CLI output provider routes = %d, want 0", routes)
	}
	if active := fixture.active.Load(); active != 0 {
		t.Fatalf("shared CLI output invocations active = %d, want 0", active)
	}
}

// executePreActivationBatch admits the fixed bad-input/output-selection batch
// concurrently while keeping all other shared-root executions serialized. The
// cases in this batch return during CLI parsing or output validation, before
// Factory/runtime activation; unique inputs and working-directory routes keep
// their streams and controlled provider edges independent if that invariant
// regresses. The shared effect counter proves that no activation boundary was
// crossed by any member of the batch.
func (fixture *machineOutputFixture) executePreActivationBatch(
	invocations []machineOutputPreActivationInvocation,
	effectsCounter *atomic.Int32,
) machineOutputPreActivationBatchResult {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.effects.setCounter(effectsCounter)
	defer fixture.effects.setCounter(nil)

	routeIDs := make([]string, len(invocations))
	for index, invocation := range invocations {
		if invocation.providerRunner == nil {
			continue
		}
		routeIDs[index] = fixture.router.bind(
			invocation.inputs.Input.WorkingDirectory,
			invocation.providerRunner,
		)
	}
	defer func() {
		for _, routeID := range routeIDs {
			if routeID != "" {
				fixture.router.unbind(routeID)
			}
		}
	}()

	if len(invocations) == 0 {
		return machineOutputPreActivationBatchResult{}
	}

	start := make(chan struct{})
	ready := make(chan struct{}, len(invocations))
	results := make(chan machineOutputPreActivationResult, len(invocations))
	var current atomic.Int32
	var maxConcurrent atomic.Int32
	var waitGroup sync.WaitGroup
	for index, invocation := range invocations {
		fixture.active.Add(1)
		waitGroup.Add(1)
		go func(index int, invocation machineOutputPreActivationInvocation) {
			defer waitGroup.Done()
			defer fixture.active.Add(-1)
			ready <- struct{}{}
			<-start

			active := current.Add(1)
			for {
				observed := maxConcurrent.Load()
				if active <= observed || maxConcurrent.CompareAndSwap(observed, active) {
					break
				}
			}
			err := fixture.process.Execute(invocation.inputs.Input)
			current.Add(-1)
			results <- machineOutputPreActivationResult{index: index, err: err}
		}(index, invocation)
	}
	for range invocations {
		<-ready
	}
	close(start)
	batchErrors := make([]error, len(invocations))
	for range invocations {
		result := <-results
		batchErrors[result.index] = result.err
	}
	waitGroup.Wait()

	// The buffered result channel lets each invocation publish independently;
	// this collection restores the caller's original case order.
	return machineOutputPreActivationBatchResult{
		errors:        batchErrors,
		maxConcurrent: maxConcurrent.Load(),
	}
}

func machineOutputAcceptedProviderRunner() platformprocess.CommandRunner {
	return support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("{\"decision\":\"accepted\",\"feedback\":\"\",\"output\":\"mock worker accepted\"}"),
	})
}

func machineOutputRejectedProviderRunner() platformprocess.CommandRunner {
	return support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 7,
		Stderr:   []byte("deterministic worker rejection"),
	})
}

var _ platformprocess.CommandRunner = (*machineOutputCommandRouter)(nil)
