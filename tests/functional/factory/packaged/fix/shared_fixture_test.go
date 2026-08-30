package fix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
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
	census         *packagedFixResourceCensus
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

func (runner *packagedFixSelectorRunner) registeredCount() int {
	runner.mu.RLock()
	defer runner.mu.RUnlock()
	return len(runner.delegates)
}

func (runner *packagedFixSelectorRunner) registered(selector string) bool {
	runner.mu.RLock()
	defer runner.mu.RUnlock()
	_, ok := runner.delegates[packagedFixSelectorPath(selector)]
	return ok
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

// packagedFixCLIProcessFixture owns the reusable non-hosted customer CLI
// process. Calls are serialized because support.Process documents reusable
// process execution for sequential public invocations; each call still gets a
// fresh home, config scope, Factory copy, workspace, and provider selector.
type packagedFixCLIProcessFixture struct {
	rootDir        string
	factorySeedDir string
	configSeed     []byte
	process        support.ApplicationProcess
	providerRunner *packagedFixSelectorRunner

	mu              sync.Mutex
	processBuilds   int
	factoryInstalls int
	factoryCopies   int
	configCopies    int
	executions      int
}

var (
	packagedFixCLIProcessFixtureOnce     sync.Once
	packagedFixCLIProcessFixtureInstance *packagedFixCLIProcessFixture
	packagedFixCLIProcessFixtureErr      error
)

func sharedPackagedFixCLIProcessFixture(t *testing.T) *packagedFixCLIProcessFixture {
	t.Helper()
	packagedFixCLIProcessFixtureOnce.Do(func() {
		packagedFixCLIProcessFixtureInstance, packagedFixCLIProcessFixtureErr =
			startPackagedFixCLIProcessFixture()
	})
	if packagedFixCLIProcessFixtureErr != nil {
		t.Fatalf("start shared packaged Fix CLI process: %v", packagedFixCLIProcessFixtureErr)
	}
	if packagedFixCLIProcessFixtureInstance == nil {
		t.Fatal("shared packaged Fix CLI process is unavailable")
	}
	return packagedFixCLIProcessFixtureInstance
}

func startPackagedFixCLIProcessFixture() (*packagedFixCLIProcessFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-packaged-fix-cli-")
	if err != nil {
		return nil, fmt.Errorf("create CLI fixture root: %w", err)
	}
	cleanupRoot := func() { _ = os.RemoveAll(rootDir) }
	homeDir := filepath.Join(rootDir, "home")
	workingDir := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create CLI fixture home: %w", err)
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create CLI fixture working directory: %w", err)
	}

	providerRunner := &packagedFixSelectorRunner{}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
	})
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("build CLI root process: %w", err)
	}
	env := packagedFixFixtureEnvironment(homeDir)
	if err := initializePackagedFixHome(process, env, workingDir); err != nil {
		closePackagedFixProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("initialize packaged CLI Factory home: %w", err)
	}
	factorySeedDir := filepath.Join(
		homeDir,
		".you-agent-factory",
		"factories",
		filepath.FromSlash(packagedFixFactoryName),
	)
	if _, err := os.Stat(filepath.Join(factorySeedDir, factorydefinitions.FactoryConfigFile)); err != nil {
		closePackagedFixProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("find packaged Fix CLI seed at %s: %w", factorySeedDir, err)
	}
	configSeed, err := os.ReadFile(filepath.Join(homeDir, ".you-agent-factory", "config.json"))
	if err != nil {
		closePackagedFixProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("read packaged Fix CLI config seed: %w", err)
	}
	return &packagedFixCLIProcessFixture{
		rootDir:         rootDir,
		factorySeedDir:  factorySeedDir,
		configSeed:      configSeed,
		process:         process,
		providerRunner:  providerRunner,
		processBuilds:   1,
		factoryInstalls: 1,
	}, nil
}

func (fixture *packagedFixCLIProcessFixture) copyFactory(
	t *testing.T,
	homeDir string,
) string {
	t.Helper()
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.factorySeedDir,
		homeDir,
		packagedFixFactoryName,
	)
	fixture.copySystemConfig(t, homeDir)
	fixture.mu.Lock()
	fixture.factoryCopies++
	fixture.mu.Unlock()
	return factoryDir
}

func (fixture *packagedFixCLIProcessFixture) copySystemConfig(
	t *testing.T,
	homeDir string,
) {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(fixture.configSeed, &document); err != nil {
		t.Fatalf("decode packaged Fix CLI config seed: %v", err)
	}
	backendScopeID, err := json.Marshal("local-" + uuid.NewString())
	if err != nil {
		t.Fatalf("encode packaged Fix CLI backend scope: %v", err)
	}
	document["backendScopeID"] = backendScopeID
	config, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode packaged Fix CLI config: %v", err)
	}
	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create packaged Fix CLI config directory: %v", err)
	}
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		t.Fatalf("copy packaged Fix CLI config: %v", err)
	}
	fixture.mu.Lock()
	fixture.configCopies++
	fixture.mu.Unlock()
}

func (fixture *packagedFixCLIProcessFixture) execute(
	t *testing.T,
	inputs *support.CapturedInputs,
) error {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.executions++
	return fixture.process.Execute(inputs.Input)
}

func (fixture *packagedFixCLIProcessFixture) close() error {
	if fixture == nil {
		return nil
	}
	closeContext, cancel := context.WithTimeout(context.Background(), packagedFixFixtureShutdownTimeout)
	closeErr := fixture.process.Close(closeContext)
	cancel()
	if closeErr != nil {
		closeErr = fmt.Errorf("close reusable CLI root process: %w", closeErr)
	}
	if fixture.providerRunner.registeredCount() != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"%d provider selectors remain after CLI fixture cleanup",
			fixture.providerRunner.registeredCount(),
		))
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("remove CLI fixture root: %w", err))
	}
	if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		closeErr = errors.Join(closeErr, fmt.Errorf("CLI fixture root remains: %v", err))
	}
	fixture.mu.Lock()
	fmt.Fprintf(os.Stderr,
		"GATE-FIX-OPT Fix CLI reuse counts: roots=%d installs=%d factoryCopies=%d configCopies=%d executions=%d selectors=%d\n",
		fixture.processBuilds, fixture.factoryInstalls, fixture.factoryCopies,
		fixture.configCopies, fixture.executions, fixture.providerRunner.registeredCount(),
	)
	fixture.mu.Unlock()
	return closeErr
}

// packagedFixGitSeed is an immutable real Git repository used only as a
// metadata seed. Each scenario receives a distinct real Git clone and retains
// real Git worktree creation and failure behavior without copying transient
// administrative lock files.
type packagedFixGitSeed struct {
	rootDir string

	mu              sync.Mutex
	metadataCopies  int
	worktreeCreates int
}

var (
	packagedFixGitSeedOnce     sync.Once
	packagedFixGitSeedInstance *packagedFixGitSeed
	packagedFixGitSeedErr      error
)

func sharedPackagedFixGitSeed(t *testing.T) *packagedFixGitSeed {
	t.Helper()
	packagedFixGitSeedOnce.Do(func() {
		packagedFixGitSeedInstance, packagedFixGitSeedErr = startPackagedFixGitSeed()
	})
	if packagedFixGitSeedErr != nil {
		t.Fatalf("start packaged Fix Git seed: %v", packagedFixGitSeedErr)
	}
	if packagedFixGitSeedInstance == nil {
		t.Fatal("packaged Fix Git seed is unavailable")
	}
	return packagedFixGitSeedInstance
}

func startPackagedFixGitSeed() (*packagedFixGitSeed, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-packaged-fix-git-seed-")
	if err != nil {
		return nil, fmt.Errorf("create Git seed root: %w", err)
	}
	cleanupRoot := func() { _ = os.RemoveAll(rootDir) }
	commands := [][]string{
		{"init"},
		{"config", "user.email", "fix-functional@example.com"},
		{"config", "user.name", "fix functional"},
		{"config", "gc.auto", "0"},
		{"config", "maintenance.auto", "false"},
		{"commit", "--allow-empty", "-m", "initial Fix functional repository"},
	}
	for _, args := range commands {
		if output, err := runPackagedFixGitCommand(rootDir, args...); err != nil {
			cleanupRoot()
			return nil, fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, output)
		}
	}
	return &packagedFixGitSeed{rootDir: rootDir}, nil
}

func runPackagedFixGitCommand(workspace string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = workspace
	return command.CombinedOutput()
}

func (seed *packagedFixGitSeed) copyMetadata(t *testing.T, workspace string) {
	t.Helper()
	cloneDir, err := os.MkdirTemp("", "you-functional-packaged-fix-git-clone-")
	if err != nil {
		t.Fatalf("create temporary packaged Fix Git clone: %v", err)
	}
	defer os.RemoveAll(cloneDir)
	if output, err := runPackagedFixGitCommand(
		"", "clone", "--quiet", "--no-local", "--no-checkout",
		"--config", "gc.auto=0",
		"--config", "maintenance.auto=false",
		"--config", "user.email=fix-functional@example.com",
		"--config", "user.name=fix functional",
		seed.rootDir,
		cloneDir,
	); err != nil {
		t.Fatalf("clone packaged Fix Git metadata: %v\n%s", err, output)
	}
	if err := os.Rename(filepath.Join(cloneDir, ".git"), filepath.Join(workspace, ".git")); err != nil {
		t.Fatalf("install cloned packaged Fix Git metadata: %v", err)
	}
	seed.mu.Lock()
	seed.metadataCopies++
	seed.mu.Unlock()
}

func (seed *packagedFixGitSeed) recordWorktreeCreate() {
	seed.mu.Lock()
	defer seed.mu.Unlock()
	seed.worktreeCreates++
}

func (seed *packagedFixGitSeed) close() error {
	if seed == nil {
		return nil
	}
	seed.mu.Lock()
	metadataCopies := seed.metadataCopies
	worktreeCreates := seed.worktreeCreates
	seed.mu.Unlock()
	fmt.Fprintf(os.Stderr,
		"GATE-FIX-OPT Fix Git seed counts: gitInit=1 metadataCopies=%d worktreeCreates=%d\n",
		metadataCopies, worktreeCreates,
	)
	if err := os.RemoveAll(seed.rootDir); err != nil {
		return fmt.Errorf("remove Git seed root: %w", err)
	}
	if _, err := os.Stat(seed.rootDir); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("Git seed root remains: %v", err)
	}
	return nil
}

func TestMain(m *testing.M) {
	code := m.Run()
	if packagedFixCLIProcessFixtureInstance != nil {
		if err := packagedFixCLIProcessFixtureInstance.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared packaged Fix CLI process: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	var closeErr error
	if packagedFixFixture != nil {
		closeErr = packagedFixFixture.close()
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "close shared packaged Fix fixture: %v\n", closeErr)
			if code == 0 {
				code = 1
			}
		}
	}
	if err := writePackagedFixForcedUnwindReport(packagedFixFixture, closeErr); err != nil {
		fmt.Fprintf(os.Stderr, "write packaged Fix forced-unwind report: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	if packagedFixGitSeedInstance != nil {
		if err := packagedFixGitSeedInstance.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close packaged Fix Git seed: %v\n", err)
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
	census := newPackagedFixResourceCensus()
	census.recordProcessStart()

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
	// The continuous runtime writes lifecycle files below its Factory directory.
	// Copy a static template before starting it so per-scenario copies never
	// race with runtime artifact creation or observe a half-written file.
	templateDir := filepath.Join(rootDir, "factory-template")
	if err := os.CopyFS(templateDir, os.DirFS(factoryDir)); err != nil {
		closePackagedFixProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("snapshot packaged Fix Factory: %w", err)
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
		factoryDir:     templateDir,
		baseURL:        baseURL,
		process:        process,
		providerRunner: providerRunner,
		census:         census,
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
		// The done channel is the deterministic lifecycle observation. This
		// timer is only a bounded startup-failure/teardown safety ceiling: a
		// failed command must not leave fixture setup hanging forever while its
		// context and injected API server unwind. It is never scenario
		// synchronization.
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
	if fixture.census != nil {
		fixture.census.recordPath(packagedFixCleanupCancellation)
	}
	fixture.cancel()
	var errs []error
	select {
	case err := <-fixture.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			errs = append(errs, fmt.Errorf("continuous command: %w", err))
		}
	case <-time.After(packagedFixFixtureShutdownTimeout):
		// The done channel is the deterministic lifecycle observation. This
		// timer is only a bounded teardown safety ceiling for a command that
		// failed to honor cancellation; without it TestMain could hang while
		// closing the injected API server. It is not normal scenario waiting.
		errs = append(errs, errors.New("timed out waiting for continuous command shutdown"))
		if fixture.census != nil {
			fixture.census.recordPath(packagedFixCleanupTimeout)
		}
	}
	closeContext, cancel := context.WithTimeout(context.Background(), packagedFixFixtureShutdownTimeout)
	if err := fixture.process.Close(closeContext); err != nil {
		errs = append(errs, fmt.Errorf("close root process: %w", err))
	}
	cancel()
	if err := assertPackagedFixPortClosed(fixture.baseURL); err != nil {
		errs = append(errs, err)
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove fixture root: %w", err))
	}
	if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("fixture root remains after cleanup: %v", err))
	}
	if fixture.providerRunner.registeredCount() != 0 {
		errs = append(errs, fmt.Errorf("%d provider selectors remain after cleanup", fixture.providerRunner.registeredCount()))
	}
	if fixture.census != nil {
		fixture.census.recordPath(packagedFixCleanupPackageTeardown)
		fmt.Fprintf(os.Stderr, "GATE-CLEAN-004 Fix cleanup paths: %s\n", fixture.census.cleanupPathSummary())
		if err := fixture.census.closedError(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type packagedFixScenario struct {
	rootDir      string
	fixture      *packagedFixSharedFixture
	factoryDir   string
	worktreeName string
	selector     string
	sessionID    string
	requestID    string
}

func openPackagedFixScenario(
	t *testing.T,
	runner *packagedFixCommandRunner,
	worktreeName string,
	configure func(*testing.T, string),
) *packagedFixScenario {
	t.Helper()
	fixture := sharedPackagedFixFixture(t)
	rootDir, err := os.MkdirTemp("", "you-functional-packaged-fix-scenario-")
	if err != nil {
		t.Fatalf("create packaged Fix scenario root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(rootDir) })
	factoryDir := filepath.Join(rootDir, "factory")
	if err := os.CopyFS(factoryDir, os.DirFS(fixture.factoryDir)); err != nil {
		t.Fatalf("copy static packaged Fix Factory: %v", err)
	}
	initPackagedFixGitRepositoryAt(t, factoryDir)
	if configure != nil {
		configure(t, factoryDir)
	}
	selector := filepath.Join(factoryDir, ".worktrees", worktreeName)
	fixture.providerRunner.register(selector, runner)
	t.Cleanup(func() { fixture.providerRunner.unregister(selector) })
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("opened packaged Fix session = %#v, want non-empty session identity", opened)
	}
	if opened.Session.Id == factorysessions.DefaultSessionID {
		t.Fatalf("opened packaged Fix session = %q, want explicit non-default session", opened.Session.Id)
	}
	requestID := "packaged-fix-" + worktreeName
	scenario := &packagedFixScenario{
		rootDir:      rootDir,
		fixture:      fixture,
		factoryDir:   factoryDir,
		worktreeName: worktreeName,
		selector:     selector,
		sessionID:    opened.Session.Id,
		requestID:    requestID,
	}
	fixture.census.register(packagedFixCensusRecord{
		name:         worktreeName,
		rootDir:      rootDir,
		factoryDir:   factoryDir,
		workspace:    factoryDir,
		selector:     selector,
		worktreeName: worktreeName,
		requestID:    requestID,
		sessionID:    scenario.sessionID,
	})
	t.Cleanup(func() {
		sessionDeleted := false
		rootAbsent := false
		if t.Failed() {
			fixture.census.recordPath(packagedFixCleanupAssertionFailure)
		}
		defer func() {
			if err := os.RemoveAll(rootDir); err != nil {
				t.Errorf("remove packaged Fix scenario root: %v", err)
			}
			if _, err := os.Stat(rootDir); errors.Is(err, os.ErrNotExist) {
				rootAbsent = true
			} else {
				t.Errorf("packaged Fix scenario root remains after cleanup: %v", err)
			}
			selectorGone := !fixture.providerRunner.registered(scenario.selector)
			fixture.census.recordCleanup(scenario.requestID, sessionDeleted, rootAbsent, selectorGone)
		}()
		support.CloseFactorySessionAt(t, fixture.baseURL, scenario.sessionID)
		sessionDeleted = assertPackagedFixSessionDeleted(t, fixture.baseURL, scenario.sessionID)
		fixture.providerRunner.unregister(scenario.selector)
	})
	return scenario
}

func initPackagedFixGitRepositoryAt(t *testing.T, workspace string) {
	t.Helper()
	sharedPackagedFixGitSeed(t).copyMetadata(t, workspace)
}
