package process_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var sharedWorkerOutcome struct {
	process support.ApplicationProcess
	router  *workerOutcomeCommandRouter
	err     error
	mu      sync.Mutex
}

// TestMain owns the package-scoped root shared by the two eligible worker
// outcomes. It is released only after every test has finished.
func TestMain(m *testing.M) {
	exitCode := m.Run()

	sharedWorkerOutcome.mu.Lock()
	sharedProcess := sharedWorkerOutcome.process
	sharedWorkerOutcome.mu.Unlock()
	if sharedProcess != nil {
		closeContext, cancel := context.WithTimeout(context.Background(), packageResourceCloseTimeout)
		if err := sharedProcess.Close(closeContext); err != nil && exitCode == 0 {
			fmt.Fprintf(os.Stderr, "close shared worker-outcome process: %v\n", err)
			exitCode = 1
		}
		cancel()
	}
	os.Exit(exitCode)
}

const packageResourceCloseTimeout = 5 * time.Second

type workerOutcomeCommandRouter struct {
	mu     sync.Mutex
	routes map[string]*workerOutcomeRoute
}

type workerOutcomeRoute struct {
	selector         string
	workingDirectory string
	runner           platformprocess.CommandRunner
	calls            int
}

func newWorkerOutcomeCommandRouter() *workerOutcomeCommandRouter {
	return &workerOutcomeCommandRouter{routes: make(map[string]*workerOutcomeRoute)}
}

func (router *workerOutcomeCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	router.mu.Lock()
	for _, route := range router.routes {
		if sameWorkerOutcomeDirectory(route.workingDirectory, request.WorkDir) {
			route.calls++
			runner := route.runner
			router.mu.Unlock()
			return runner.Run(ctx, request)
		}
	}
	router.mu.Unlock()
	return platformprocess.CommandResult{}, fmt.Errorf(
		"worker outcome route not found for provider request stdin %q",
		string(request.Stdin),
	)
}

func (router *workerOutcomeCommandRouter) bind(selector, workingDirectory string) error {
	router.mu.Lock()
	defer router.mu.Unlock()
	key := filepath.Clean(workingDirectory)
	if _, exists := router.routes[key]; exists {
		return fmt.Errorf("worker outcome route already registered for %q", workingDirectory)
	}
	var result platformprocess.CommandResult
	switch selector {
	case workerFailurePrompt:
		result = platformprocess.CommandResult{ExitCode: 1, Stderr: []byte("provider process failed with private detail")}
	case workerSuccessPrompt:
		result = platformprocess.CommandResult{Stdout: []byte(workerSuccessPrimaryResult)}
	default:
		return fmt.Errorf("unknown worker outcome selector %q", selector)
	}
	router.routes[key] = &workerOutcomeRoute{
		selector: selector, workingDirectory: workingDirectory,
		runner: support.NewShapedProviderCommandRunner(result),
	}
	return nil
}

func (router *workerOutcomeCommandRouter) unbind(workingDirectory string) {
	router.mu.Lock()
	delete(router.routes, filepath.Clean(workingDirectory))
	router.mu.Unlock()
}

func sameWorkerOutcomeDirectory(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func (router *workerOutcomeCommandRouter) callCount(selector string) int {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, route := range router.routes {
		if route.selector == selector {
			return route.calls
		}
	}
	return 0
}

func sharedWorkerOutcomeProcess(t testing.TB) *sharedWorkerOutcomeProcessFixture {
	t.Helper()
	sharedWorkerOutcome.mu.Lock()
	defer sharedWorkerOutcome.mu.Unlock()
	if sharedWorkerOutcome.process == nil && sharedWorkerOutcome.err == nil {
		sharedWorkerOutcome.router = newWorkerOutcomeCommandRouter()
		sharedWorkerOutcome.process, sharedWorkerOutcome.err = support.BuildProcessWithContext(
			context.Background(),
			serviceedges.Edges{ProviderCommandRunner: sharedWorkerOutcome.router},
		)
	}
	if sharedWorkerOutcome.err != nil {
		t.Fatalf("BuildProcess() for shared worker outcomes: %v", sharedWorkerOutcome.err)
	}
	return &sharedWorkerOutcomeProcessFixture{
		process: sharedWorkerOutcome.process,
		router:  sharedWorkerOutcome.router,
	}
}

type sharedWorkerOutcomeProcessFixture struct {
	process support.ApplicationProcess
	router  *workerOutcomeCommandRouter
}

func (fixture *sharedWorkerOutcomeProcessFixture) execute(input root.Input) error {
	return fixture.process.Execute(input)
}

func (fixture *sharedWorkerOutcomeProcessFixture) bind(
	t testing.TB,
	selector string,
	workingDirectory string,
) {
	t.Helper()
	if err := fixture.router.bind(selector, workingDirectory); err != nil {
		t.Fatalf("bind worker outcome route: %v", err)
	}
	t.Cleanup(func() { fixture.router.unbind(workingDirectory) })
}

func newCLIExitCodeInputs(
	t testing.TB,
	factoryDir string,
	factoryPath string,
	prompt string,
) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", factoryPath,
		"--provider", "codex", "--session", uuid.NewString(), "--no-record", "--quiet",
		prompt,
	})
	inputs.Input.Env = builtcliacceptance.ProcessEnvForIsolatedHome(t.TempDir())
	inputs.Input.WorkingDirectory = factoryDir
	return inputs
}

const (
	workerFailurePrompt        = "worker-failure-exit"
	workerSuccessPrompt        = "worker-success-exit"
	workerSuccessPrimaryResult = "worker success exit COMPLETE"
)

var _ platformprocess.CommandRunner = (*workerOutcomeCommandRouter)(nil)
