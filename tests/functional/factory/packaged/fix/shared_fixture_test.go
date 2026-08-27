package fix

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
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedFixFixtureShutdownTimeout = 15 * time.Second

// packagedFixSharedFixture owns one root-built process and one continuous API
// host for compatible public-outcome scenarios. Each scenario owns a copied
// packaged Factory, a real Git worktree root, and an explicit Factory Session;
// the command edge routes by that scenario's unique worktree selector.
type packagedFixSharedFixture struct {
	rootDir        string
	factoryDir     string
	baseURL        string
	process        support.ApplicationProcess
	providerRunner *packagedFixSelectorRunner
	cancel         context.CancelFunc
	done           chan error
}

type packagedFixSelectorRunner struct {
	mu        sync.RWMutex
	delegates map[string]platformprocess.CommandRunner
}

func (runner *packagedFixSelectorRunner) register(selector string, delegate platformprocess.CommandRunner) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.delegates == nil {
		runner.delegates = make(map[string]platformprocess.CommandRunner)
	}
	runner.delegates[packagedFixSelectorPath(selector)] = delegate
}

func (runner *packagedFixSelectorRunner) unregister(selector string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	delete(runner.delegates, packagedFixSelectorPath(selector))
}

func (runner *packagedFixSelectorRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := packagedFixSelectorPath(request.WorkDir)
	runner.mu.RLock()
	delegate := runner.delegates[selector]
	runner.mu.RUnlock()
	if delegate == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"no packaged Fix provider command runner registered for selector %q",
			selector,
		)
	}
	return delegate.Run(ctx, request)
}

func packagedFixSelectorPath(path string) string {
	return filepath.Clean(path)
}

var (
	packagedFixFixtureOnce sync.Once
	packagedFixFixture     *packagedFixSharedFixture
	packagedFixFixtureErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if packagedFixFixture != nil {
		if err := packagedFixFixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared packaged Fix fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

func sharedPackagedFixFixture(t *testing.T) *packagedFixSharedFixture {
	t.Helper()
	packagedFixFixtureOnce.Do(func() {
		packagedFixFixture, packagedFixFixtureErr = startPackagedFixFixture()
	})
	if packagedFixFixtureErr != nil {
		t.Fatalf("start shared packaged Fix fixture: %v", packagedFixFixtureErr)
	}
	if packagedFixFixture == nil {
		t.Fatal("shared packaged Fix fixture is unavailable")
	}
	support.WaitForStatus(t, packagedFixFixture.baseURL, packagedFixFixtureShutdownTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return packagedFixFixture
}

func startPackagedFixFixture() (*packagedFixSharedFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-packaged-fix-")
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
	providerRunner := &packagedFixSelectorRunner{}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: providerRunner,
	})
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}

	env := packagedFixFixtureEnvironment(homeDir)
	if err := initializePackagedFixHome(process, env, workingDir); err != nil {
		closePackagedFixProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("initialize packaged Factory home: %w", err)
	}
	factoryDir := filepath.Join(
		homeDir,
		".you-agent-factory",
		"factories",
		filepath.FromSlash(packagedFixFactoryName),
	)
	if _, err := os.Stat(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)); err != nil {
		closePackagedFixProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("find packaged Fix Factory at %s: %w", factoryDir, err)
	}

	commandContext, cancel := context.WithCancel(context.Background())
	inputs := support.FakeInputs(commandContext, []string{
		"you", "run",
		"--dir", factoryDir,
		"--continuously",
		"--with-server",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = factoryDir
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()

	baseURL, err := api.WaitForBaseURL(packagedFixFixtureShutdownTimeout)
	if err != nil {
		cancel()
		waitForPackagedFixCommand(done)
		closePackagedFixProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("wait for API server: %w", err)
	}
	return &packagedFixSharedFixture{
		rootDir:        rootDir,
		factoryDir:     factoryDir,
		baseURL:        baseURL,
		process:        process,
		providerRunner: providerRunner,
		cancel:         cancel,
		done:           done,
	}, nil
}

func initializePackagedFixHome(
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

func packagedFixFixtureEnvironment(homeDir string) []string {
	return []string{
		"HOME=" + homeDir,
		"USERPROFILE=" + homeDir,
		operatorsettings.EnvDefaultWorkerModelProvider + "=CODEX",
		operatorsettings.EnvDefaultWorkerModel + "=operator-configured-model",
	}
}

func waitForPackagedFixCommand(done <-chan error) {
	select {
	case <-done:
	case <-time.After(packagedFixFixtureShutdownTimeout):
	}
}

func closePackagedFixProcess(process support.ApplicationProcess) {
	if process == nil {
		return
	}
	closeContext, cancel := context.WithTimeout(context.Background(), packagedFixFixtureShutdownTimeout)
	defer cancel()
	_ = process.Close(closeContext)
}

func (fixture *packagedFixSharedFixture) close() error {
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
	case <-time.After(packagedFixFixtureShutdownTimeout):
		errs = append(errs, errors.New("timed out waiting for continuous command shutdown"))
	}
	closeContext, cancel := context.WithTimeout(context.Background(), packagedFixFixtureShutdownTimeout)
	if err := fixture.process.Close(closeContext); err != nil {
		errs = append(errs, fmt.Errorf("close root process: %w", err))
	}
	cancel()
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove fixture root: %w", err))
	}
	return errors.Join(errs...)
}

type packagedFixScenario struct {
	fixture      *packagedFixSharedFixture
	homeDir      string
	factoryDir   string
	worktreeName string
	selector     string
	sessionID    string
}

func openPackagedFixScenario(
	t *testing.T,
	runner *packagedFixCommandRunner,
	worktreeName string,
	configure func(*testing.T, string),
) *packagedFixScenario {
	t.Helper()
	fixture := sharedPackagedFixFixture(t)
	homeDir := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(t, fixture.factoryDir, homeDir, packagedFixFactoryName)
	initPackagedFixGitRepositoryAt(t, factoryDir)
	if configure != nil {
		configure(t, factoryDir)
	}
	selector := filepath.Join(factoryDir, ".worktrees", worktreeName)
	fixture.providerRunner.register(selector, runner)
	t.Cleanup(func() { fixture.providerRunner.unregister(selector) })
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatal("opened packaged Fix session has no id")
	}
	if opened.Session.Id == factorysessions.DefaultSessionID {
		t.Fatalf("opened packaged Fix session = %q, want explicit non-default session", opened.Session.Id)
	}
	sessionID := opened.Session.Id
	t.Cleanup(func() {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	})
	return &packagedFixScenario{
		fixture:      fixture,
		homeDir:      homeDir,
		factoryDir:   factoryDir,
		worktreeName: worktreeName,
		selector:     selector,
		sessionID:    sessionID,
	}
}

func initPackagedFixGitRepositoryAt(t *testing.T, workspace string) {
	t.Helper()
	runPackagedFixGit(t, workspace, "init")
	runPackagedFixGit(t, workspace, "config", "user.email", "fix-functional@example.com")
	runPackagedFixGit(t, workspace, "config", "user.name", "fix functional")
	runPackagedFixGit(t, workspace, "commit", "--allow-empty", "-m", "initial Fix functional repository")
}
