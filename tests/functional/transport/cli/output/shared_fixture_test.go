package output_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const machineOutputProcessCloseTimeout = 5 * time.Second

var machineOutputShared struct {
	once    sync.Once
	fixture *machineOutputFixture
	err     error
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
	factorySources map[string]string
	sourceRoot     string
	baseURL        string
	daemonCancel   context.CancelFunc
	daemonDone     chan error
}

type machineOutputCommandRouter struct {
	mu     sync.Mutex
	nextID uint64
	routes map[string]machineOutputRoute
}

type machineOutputRoute struct {
	executionScopeID string
	runner           platformprocess.CommandRunner
}

func newMachineOutputCommandRouter() *machineOutputCommandRouter {
	return &machineOutputCommandRouter{routes: make(map[string]machineOutputRoute)}
}

func (router *machineOutputCommandRouter) bind(
	executionScopeID string,
	runner platformprocess.CommandRunner,
) string {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.nextID++
	routeID := fmt.Sprintf("machine-output-route-%d", router.nextID)
	router.routes[routeID] = machineOutputRoute{
		executionScopeID: executionScopeID,
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
		if route.executionScopeID != request.ExecutionScopeID {
			continue
		}
		runner := route.runner
		router.mu.Unlock()
		return runner.Run(ctx, request)
	}
	router.mu.Unlock()
	return platformprocess.CommandResult{}, fmt.Errorf(
		"CLI output provider route not found for Factory Session %q",
		request.ExecutionScopeID,
	)
}

func sharedMachineOutputFixture(t testing.TB) *machineOutputFixture {
	t.Helper()
	machineOutputShared.once.Do(func() {
		router := newMachineOutputCommandRouter()
		api := support.NewProcessAPIServer()
		process, err := support.BuildProcessWithContext(
			context.Background(),
			serviceedges.Edges{
				ProviderCommandRunner: router,
				APIServerStarter:      api.Start,
				BrowserOpener:         func(context.Context, string) error { return nil },
				RuntimeHostObserver:   func(factorysessions.RuntimeHostBinding) {},
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
		daemonContext, daemonCancel := context.WithCancel(context.Background())
		daemonInputs := support.FakeInputs(daemonContext, []string{
			"you", "run", "--continuously", "--with-server", "--quiet",
			"--factory", machineOutputFactorySources(sourceHome)["@you/goal"], "--no-record",
		})
		daemonInputs.Input.Env = builtcliacceptance.ProcessEnvForIsolatedHome(filepath.Join(sourceRoot, "daemon-home"))
		daemonInputs.Input.WorkingDirectory = sourceRoot
		daemonDone := make(chan error, 1)
		go func() { daemonDone <- process.Execute(daemonInputs.Input) }()
		baseURL, startErr := api.WaitForBaseURL(15 * time.Second)
		if startErr != nil {
			daemonCancel()
			_ = process.Close(context.Background())
			_ = os.RemoveAll(sourceRoot)
			machineOutputShared.err = startErr
			return
		}
		machineOutputShared.fixture = &machineOutputFixture{
			process:        process,
			router:         router,
			factorySources: machineOutputFactorySources(sourceHome),
			sourceRoot:     sourceRoot,
			baseURL:        baseURL,
			daemonCancel:   daemonCancel,
			daemonDone:     daemonDone,
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
	if fixture.daemonCancel != nil {
		fixture.daemonCancel()
	}
	if fixture.daemonDone != nil {
		select {
		case err := <-fixture.daemonDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				closeErrors = append(closeErrors, fmt.Errorf("stop shared CLI output host: %w", err))
			}
		case <-ctx.Done():
			closeErrors = append(closeErrors, fmt.Errorf("stop shared CLI output host: %w", ctx.Err()))
		}
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
	env := builtcliacceptance.ProcessEnvForIsolatedHome(sourceHome)
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
		"@you/goal": filepath.Join(factoriesRoot, "@you", "goal"),
	}
}

func runMachineOutputInvocation(
	t testing.TB,
	args []string,
	packagedFactoryName string,
	providerRunner platformprocess.CommandRunner,
) (stdout, stderr string, err error) {
	t.Helper()
	fixture, inputs, factoryDir := newMachineOutputInputs(t, args, packagedFactoryName)
	err = fixture.execute(t, inputs, factoryDir, providerRunner)
	return inputs.Stdout(), inputs.Stderr(), err
}

func newMachineOutputInputs(
	t testing.TB,
	args []string,
	packagedFactoryName string,
) (*machineOutputFixture, *support.CapturedInputs, string) {
	t.Helper()
	fixture := sharedMachineOutputFixture(t)
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	env := builtcliacceptance.ProcessEnvForIsolatedHome(homeDir)
	inputs := support.FakeInputs(t.Context(), append([]string(nil), args...))
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	factoryDir := machineOutputExplicitFactoryDir(args)
	if packagedFactoryName != "" {
		sourceDir := fixture.factorySources[packagedFactoryName]
		if sourceDir == "" {
			t.Fatalf("no shared packaged Factory source for %q", packagedFactoryName)
		}
		factoryDir = support.CopyFactoryAsNamed(t, sourceDir, homeDir, packagedFactoryName)
	}
	return fixture, inputs, factoryDir
}

func (fixture *machineOutputFixture) execute(
	t testing.TB,
	inputs *support.CapturedInputs,
	factoryDir string,
	providerRunner platformprocess.CommandRunner,
) error {
	t.Helper()
	if factoryDir == "" {
		// Missing targets and command-shape failures return before runtime
		// ownership and therefore need no hosted Factory Session.
		return fixture.process.Execute(inputs.Input)
	}
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	defer support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	var routeID string
	if providerRunner != nil {
		routeID = fixture.router.bind(sessionID, providerRunner)
		defer fixture.router.unbind(routeID)
	}
	args := []string{"you", "--remote", "--server", fixture.baseURL}
	for _, arg := range inputs.Input.Args[1:] {
		args = append(args, arg)
		if arg == "run" {
			args = append(args, "--session", sessionID)
		}
	}
	inputs.Input.Args = args
	return fixture.process.Execute(inputs.Input)
}

func machineOutputExplicitFactoryDir(args []string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] != "--factory" {
			continue
		}
		path := args[index+1]
		if filepath.Ext(path) != "" {
			return filepath.Dir(path)
		}
		return path
	}
	return ""
}

// executeValidation is limited to command-shape failures that return before a
// Factory Session is opened. Those calls do not own ~default and can overlap
// customer invocations that do.
func (fixture *machineOutputFixture) executeValidation(inputs *support.CapturedInputs) error {
	return fixture.process.Execute(inputs.Input)
}

func runMachineOutputValidationInvocation(
	t testing.TB,
	args []string,
) (stdout, stderr string, err error) {
	t.Helper()
	fixture, inputs, _ := newMachineOutputInputs(t, args, "")
	err = fixture.executeValidation(inputs)
	return inputs.Stdout(), inputs.Stderr(), err
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
