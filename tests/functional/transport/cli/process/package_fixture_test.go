package process_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var processCLIBinary struct {
	once     sync.Once
	tempDir  string
	path     string
	err      error
	buildLog []byte
}

var sharedWorkerOutcome struct {
	once    sync.Once
	process support.ApplicationProcess
	router  *workerOutcomeCommandRouter
	err     error
	mu      sync.Mutex
}

// TestMain owns package-scoped resources that are intentionally shared across
// the two eligible worker outcomes and all retained built-CLI witnesses. The
// resources are released only after every test has finished, so a test cleanup
// cannot invalidate a sibling test that is still using the shared root or
// executable.
func TestMain(m *testing.M) {
	exitCode := m.Run()

	if sharedWorkerOutcome.process != nil {
		closeContext, cancel := context.WithTimeout(context.Background(), packageResourceCloseTimeout)
		if err := sharedWorkerOutcome.process.Close(closeContext); err != nil && exitCode == 0 {
			fmt.Fprintf(os.Stderr, "close shared worker-outcome process: %v\n", err)
			exitCode = 1
		}
		cancel()
	}
	if processCLIBinary.tempDir != "" {
		if err := os.RemoveAll(processCLIBinary.tempDir); err != nil && exitCode == 0 {
			fmt.Fprintf(os.Stderr, "remove reusable CLI binary directory %s: %v\n", processCLIBinary.tempDir, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

const packageResourceCloseTimeout = 5 * time.Second

func buildYouBinary(t testing.TB, ctx context.Context, repoRoot string) string {
	t.Helper()
	processCLIBinary.once.Do(func() {
		processCLIBinary.tempDir, processCLIBinary.err = os.MkdirTemp("", "you-cli-process-package-")
		if processCLIBinary.err != nil {
			return
		}
		binaryName := "you"
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		processCLIBinary.path = filepath.Join(processCLIBinary.tempDir, binaryName)
		command := exec.CommandContext(ctx, "go", "build", "-o", processCLIBinary.path, "./cmd/factory")
		command.Dir = repoRoot
		processCLIBinary.buildLog, processCLIBinary.err = command.CombinedOutput()
	})
	if processCLIBinary.err != nil {
		t.Fatalf("build you CLI: %v\n%s", processCLIBinary.err, processCLIBinary.buildLog)
	}
	return processCLIBinary.path
}

func runBuiltYouBinary(
	ctx context.Context,
	binaryPath string,
	session *builtcliacceptance.Session,
	args ...string,
) (builtcliacceptance.RunResult, error) {
	command := exec.CommandContext(ctx, binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := builtcliacceptance.RunResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return result, err
	}
	result.ExitCode = exitErr.ExitCode()
	return result, err
}

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
	sharedWorkerOutcome.once.Do(func() {
		sharedWorkerOutcome.router = newWorkerOutcomeCommandRouter()
		sharedWorkerOutcome.process, sharedWorkerOutcome.err = support.BuildProcessWithContext(
			context.Background(),
			serviceedges.Edges{ProviderCommandRunner: sharedWorkerOutcome.router},
		)
	})
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

func (fixture *sharedWorkerOutcomeProcessFixture) bind(selector, workingDirectory string) {
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
