package classify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const classifyFixtureShutdownTimeout = 15 * time.Second

// classifySharedFixture owns one immutable root process and one continuous
// API host for the package's compatible classifier scenarios. Each scenario
// copies the packaged definition to its own folder and opens an explicit
// Factory Session, while the provider edge is swapped only between the
// package's sequential invocations.
type classifySharedFixture struct {
	rootDir        string
	factoryDir     string
	baseURL        string
	process        support.ApplicationProcess
	providerRunner *classifySwitchingProviderRunner
	cancel         context.CancelFunc
	done           chan error
}

type classifySwitchingProviderRunner struct {
	mu       sync.Mutex
	delegate platformprocess.CommandRunner
}

func (r *classifySwitchingProviderRunner) setDelegate(delegate platformprocess.CommandRunner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delegate = delegate
}

func (r *classifySwitchingProviderRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	delegate := r.delegate
	r.mu.Unlock()
	if delegate == nil {
		return platformprocess.CommandResult{}, errors.New("no classify provider command runner installed")
	}
	return delegate.Run(ctx, request)
}

var (
	classifyFixtureOnce sync.Once
	classifyFixture     *classifySharedFixture
	classifyFixtureErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	var closeErr error
	if classifyFixture != nil {
		closeErr = classifyFixture.close()
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "close shared classify fixture: %v\n", closeErr)
			if code == 0 {
				code = 1
			}
		}
	}
	if err := writeClassifyForcedUnwindReport(classifyFixture, closeErr); err != nil {
		fmt.Fprintf(os.Stderr, "write classify forced-unwind report: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func sharedClassifyFixture(t *testing.T) *classifySharedFixture {
	t.Helper()
	classifyFixtureOnce.Do(func() {
		classifyFixture, classifyFixtureErr = startClassifyFixture()
	})
	if classifyFixtureErr != nil {
		t.Fatalf("start shared classify fixture: %v", classifyFixtureErr)
	}
	if classifyFixture == nil {
		t.Fatal("shared classify fixture is unavailable")
	}
	support.WaitForStatus(t, classifyFixture.baseURL, classifyFixtureShutdownTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return classifyFixture
}

func startClassifyFixture() (*classifySharedFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-packaged-classify-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	cleanupRoot := func() { _ = os.RemoveAll(rootDir) }
	homeDir := filepath.Join(rootDir, "home")
	workingDir := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create fixture home: %w", err)
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create fixture working directory: %w", err)
	}

	api := support.NewProcessAPIServer()
	providerRunner := &classifySwitchingProviderRunner{}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: providerRunner,
	})
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}

	env := classifyFixtureEnvironment(homeDir)
	if err := initializeClassifyHome(process, env, workingDir); err != nil {
		closeClassifyProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("initialize packaged Factory home: %w", err)
	}
	factoryDir := filepath.Join(
		homeDir,
		".you-agent-factory",
		"factories",
		filepath.FromSlash(factorydefinitions.PackagedClassifyFactoryName),
	)
	if _, err := os.Stat(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)); err != nil {
		closeClassifyProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("find packaged classify Factory at %s: %w", factoryDir, err)
	}

	commandContext, cancel := context.WithCancel(context.Background())
	inputs := support.FakeInputs(commandContext, []string{
		"you", "run",
		"--dir", factoryDir,
		"--continuously",
		"--with-server",
		"--quiet",
		"--no-record",
		"--provider", "CODEX",
		"--model", "operator-model",
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = factoryDir
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()

	baseURL, err := api.WaitForBaseURL(classifyFixtureShutdownTimeout)
	if err != nil {
		cancel()
		waitForClassifyCommand(done)
		closeClassifyProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("wait for API server: %w", err)
	}
	return &classifySharedFixture{
		rootDir:        rootDir,
		factoryDir:     factoryDir,
		baseURL:        baseURL,
		process:        process,
		providerRunner: providerRunner,
		cancel:         cancel,
		done:           done,
	}, nil
}

func initializeClassifyHome(
	process support.Process,
	env []string,
	workingDir string,
) error {
	missingFactory := filepath.Join(workingDir, "missing-initialization-factory.json")
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--factory", missingFactory,
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDir
	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		return fmt.Errorf(
			"missing Factory probe error = %v, stdout=%q, stderr=%q",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return nil
}

func classifyFixtureEnvironment(homeDir string) []string {
	return []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
}

func waitForClassifyCommand(done <-chan error) {
	select {
	case <-done:
	case <-time.After(classifyFixtureShutdownTimeout):
	}
}

func closeClassifyProcess(process support.ApplicationProcess) {
	if process == nil {
		return
	}
	closeContext, cancel := context.WithTimeout(context.Background(), classifyFixtureShutdownTimeout)
	defer cancel()
	_ = process.Close(closeContext)
}

func (fixture *classifySharedFixture) close() error {
	if fixture == nil {
		return nil
	}
	fixture.cancel()
	var errs []error
	select {
	case err := <-fixture.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			errs = append(errs, fmt.Errorf("continuous command: %w", err))
		}
	case <-time.After(classifyFixtureShutdownTimeout):
		errs = append(errs, errors.New("timed out waiting for continuous command shutdown"))
	}
	closeContext, cancel := context.WithTimeout(context.Background(), classifyFixtureShutdownTimeout)
	if err := fixture.process.Close(closeContext); err != nil {
		errs = append(errs, fmt.Errorf("close root process: %w", err))
	}
	cancel()
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove fixture root: %w", err))
	}
	return errors.Join(errs...)
}

type classifyScenario struct {
	fixture   *classifySharedFixture
	sessionID string
}

func openClassifyScenario(
	t *testing.T,
	runner *support.ShapedProviderCommandRunner,
) *classifyScenario {
	t.Helper()
	fixture := sharedClassifyFixture(t)
	fixture.providerRunner.setDelegate(runner)
	homeDir := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.factoryDir,
		homeDir,
		factorydefinitions.PackagedClassifyFactoryName,
	)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if sessionID == "" {
		t.Fatal("opened classify session has no id")
	}
	t.Cleanup(func() {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	})
	return &classifyScenario{fixture: fixture, sessionID: sessionID}
}
