package process_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
	routes []workerOutcomeRoute
}

type workerOutcomeRoute struct {
	selector         string
	workingDirectory string
	runner           platformprocess.CommandRunner
	calls            int
}

func newWorkerOutcomeCommandRouter() *workerOutcomeCommandRouter {
	return &workerOutcomeCommandRouter{routes: []workerOutcomeRoute{
		{
			selector: workerFailurePrompt,
			runner: support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte("provider process failed with private detail"),
			}),
		},
		{
			selector: workerSuccessPrompt,
			runner: support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
				Stdout: []byte(workerSuccessPrimaryResult),
			}),
		},
	}}
}

func (router *workerOutcomeCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	router.mu.Lock()
	for index := range router.routes {
		route := &router.routes[index]
		if route.workingDirectory == "" && !bytes.Contains(request.Stdin, []byte(route.selector)) {
			continue
		}
		if route.workingDirectory != "" && !sameWorkerOutcomeDirectory(route.workingDirectory, request.WorkDir) {
			continue
		}
		route.calls++
		runner := route.runner
		router.mu.Unlock()
		return runner.Run(ctx, request)
	}
	router.mu.Unlock()
	return platformprocess.CommandResult{}, fmt.Errorf(
		"worker outcome route not found for provider request stdin %q",
		string(request.Stdin),
	)
}

func (router *workerOutcomeCommandRouter) bind(selector, workingDirectory string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for index := range router.routes {
		if router.routes[index].selector == selector {
			router.routes[index].workingDirectory = workingDirectory
			return
		}
	}
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
		mu:      &sharedWorkerOutcome.mu,
	}
}

type sharedWorkerOutcomeProcessFixture struct {
	process support.ApplicationProcess
	router  *workerOutcomeCommandRouter
	mu      *sync.Mutex
}

func (fixture *sharedWorkerOutcomeProcessFixture) execute(input root.Input) error {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.process.Execute(input)
}

func (fixture *sharedWorkerOutcomeProcessFixture) bind(
	t testing.TB,
	selector string,
	workingDirectory string,
) {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()

	// go test -count=N repeats the test list in one test process, so TestMain
	// does not recreate package fixtures between repetitions. A reusable root
	// is intentionally shared by the failure/success pair in one repetition,
	// but the runtime graph must be closed before the next repetition starts so
	// its durable runtime state cannot bleed into a fresh Factory fixture.
	if fixture.router.callCount(selector) > 0 {
		closeContext, cancel := context.WithTimeout(context.Background(), packageResourceCloseTimeout)
		closeErr := fixture.process.Close(closeContext)
		cancel()
		if closeErr != nil {
			t.Fatalf("close shared worker-outcome process before repeat: %v", closeErr)
		}
		fixture.router = newWorkerOutcomeCommandRouter()
		fixture.process, sharedWorkerOutcome.err = support.BuildProcessWithContext(
			context.Background(),
			serviceedges.Edges{ProviderCommandRunner: fixture.router},
		)
		sharedWorkerOutcome.process = fixture.process
		if sharedWorkerOutcome.err != nil {
			t.Fatalf("BuildProcess() for repeated shared worker outcomes: %v", sharedWorkerOutcome.err)
		}
	}
	fixture.router.bind(selector, workingDirectory)
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
		"--provider", "codex", "--no-record", "--quiet",
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
