package help_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	helpPackageCloseTimeout      = 5 * time.Second
	helpUnexpectedProviderExit   = 97
	invocationHelpEmptyFactory   = "invocation-help-empty"
	helpPackageWorkstationPrompt = "---\ntype: MODEL_WORKSTATION\n---\nDo the help-only work.\n"
)

// helpPackageFixture owns the one production-composed process used by every
// help scenario. Each command still receives a unique invocation root and
// fresh streams. Cleanup assertions are limited to resources observable at
// this functional edge: Process.Close, invocation completion, provider calls,
// and temporary roots. Factory/Worker Session, response-stream, subprocess,
// listener, and port closure belong to their public/integration lifecycle
// gates; this fixture must not manufacture identities or counters for them.
type helpPackageFixture struct {
	process        support.ApplicationProcess
	providerRunner *testutil.ProviderCommandRunner
	processBuilds  atomic.Int32

	rootDir              string
	homeDir              string
	workingRoot          string
	fullFactoryPath      string
	emptyFactoryPath     string
	malformedFactoryPath string

	nextInvocation    atomic.Uint64
	mu                sync.Mutex
	activeInvocations map[string]struct{}
	closeOnce         sync.Once
	closeErr          error
}

type helpInvocationResources struct {
	id          string
	workingRoot string
}

type helpInvocationResult struct {
	inputs *support.CapturedInputs
	err    error
}

var helpPackageFixtureState struct {
	sync.Once
	fixture *helpPackageFixture
	err     error
}

// TestMain closes the package-owned process after every parallel test has
// released its invocation resources. The close timeout is a cleanup ceiling,
// not synchronization for any help behavior; help never starts external work.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeHelpPackageFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "help package fixture cleanup failed: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func helpPackageFixtureForTest(t *testing.T) *helpPackageFixture {
	t.Helper()
	helpPackageFixtureState.Do(func() {
		helpPackageFixtureState.fixture, helpPackageFixtureState.err = newHelpPackageFixture(t)
	})
	if helpPackageFixtureState.err != nil {
		t.Fatalf("set up shared help package fixture: %v", helpPackageFixtureState.err)
	}
	return helpPackageFixtureState.fixture
}

func newHelpPackageFixture(t *testing.T) (*helpPackageFixture, error) {
	t.Helper()

	rootDir, err := os.MkdirTemp("", "c11-help-package-")
	if err != nil {
		return nil, fmt.Errorf("create help package root: %w", err)
	}
	fixture := &helpPackageFixture{
		rootDir:           rootDir,
		homeDir:           filepath.Join(rootDir, "home"),
		workingRoot:       filepath.Join(rootDir, "working"),
		activeInvocations: make(map[string]struct{}),
		providerRunner: testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			ExitCode: helpUnexpectedProviderExit,
			Stderr:   []byte("help must not dispatch external work"),
		}),
	}
	if err := fixture.prepareFiles(); err != nil {
		_ = os.RemoveAll(rootDir)
		return nil, err
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: fixture.providerRunner,
	})
	if err != nil {
		_ = os.RemoveAll(rootDir)
		return nil, fmt.Errorf("build help package process: %w", err)
	}
	fixture.process = process
	fixture.processBuilds.Add(1)

	if err := fixture.createNamedFactory(t); err != nil {
		_ = fixture.process.Close(context.Background())
		_ = os.RemoveAll(rootDir)
		return nil, err
	}
	return fixture, nil
}

func (fixture *helpPackageFixture) prepareFiles() error {
	for _, dir := range []string{fixture.homeDir, fixture.workingRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create help fixture directory %q: %w", dir, err)
		}
	}

	fullDir := filepath.Join(fixture.rootDir, "full-factory")
	fullConfig := invocationHelpFactoryConfig()
	if err := writeHelpFactoryFiles(fullDir, fullConfig); err != nil {
		return fmt.Errorf("write full help Factory: %w", err)
	}
	fixture.fullFactoryPath = filepath.Join(fullDir, interfaces.FactoryConfigFile)

	emptyDir := filepath.Join(fixture.rootDir, "empty-factory")
	emptyConfig := invocationHelpFactoryConfig()
	emptyConfig["name"] = invocationHelpEmptyFactory
	emptyConfig["invocationSignature"] = map[string]any{
		"parameters": []any{},
		"examples":   []any{},
	}
	if err := writeHelpFactoryFiles(emptyDir, emptyConfig); err != nil {
		return fmt.Errorf("write empty help Factory: %w", err)
	}
	fixture.emptyFactoryPath = filepath.Join(emptyDir, interfaces.FactoryConfigFile)

	malformedDir := filepath.Join(fixture.rootDir, "malformed-factory")
	if err := os.MkdirAll(malformedDir, 0o755); err != nil {
		return fmt.Errorf("create malformed Factory directory: %w", err)
	}
	fixture.malformedFactoryPath = filepath.Join(malformedDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(fixture.malformedFactoryPath, []byte(`{"name":"malformed",`), 0o644); err != nil {
		return fmt.Errorf("write malformed Factory: %w", err)
	}
	return nil
}

func writeHelpFactoryFiles(dir string, cfg map[string]any) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal factory config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), raw, 0o644); err != nil {
		return err
	}
	workstations, ok := cfg["workstations"].([]map[string]any)
	if !ok {
		return nil
	}
	for _, workstation := range workstations {
		name, _ := workstation["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		workstationDir := filepath.Join(dir, "workstations", name)
		if err := os.MkdirAll(workstationDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(helpPackageWorkstationPrompt), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (fixture *helpPackageFixture) createNamedFactory(t *testing.T) error {
	t.Helper()

	missingPath := filepath.Join(fixture.workingRoot, "missing-initialization-factory.json")
	missing := fixture.execute(t, "you", "run", "--factory", missingPath)
	if missing.err == nil || !strings.Contains(missing.err.Error(), filepath.Base(missingPath)) {
		return fmt.Errorf("initialize named Factory home: error = %v", missing.err)
	}

	namedFactoriesRoot := filepath.Join(fixture.homeDir, ".you-agent-factory", "factories")
	created := fixture.execute(t,
		"you", "--json", "factory", "create", invocationHelpNamedFactoryName,
		"--from", fixture.fullFactoryPath, "--dir", namedFactoriesRoot,
	)
	if created.err != nil {
		return fmt.Errorf("create named Factory: %w", created.err)
	}
	var result struct {
		FactoryDir string `json:"factoryDir"`
	}
	if err := json.Unmarshal([]byte(created.inputs.Stdout()), &result); err != nil {
		return fmt.Errorf("decode named Factory result: %w", err)
	}
	if _, err := os.Stat(filepath.Join(result.FactoryDir, interfaces.FactoryConfigFile)); err != nil {
		return fmt.Errorf("named Factory config missing at %s: %w", result.FactoryDir, err)
	}
	return nil
}

func (fixture *helpPackageFixture) execute(t *testing.T, args ...string) helpInvocationResult {
	t.Helper()

	invocationNumber := fixture.nextInvocation.Add(1)
	id := fmt.Sprintf("help-invocation-%d", invocationNumber)
	workingRoot := filepath.Join(fixture.workingRoot, id)
	if err := os.MkdirAll(workingRoot, 0o755); err != nil {
		t.Fatalf("create invocation working directory: %v", err)
	}
	homeDir := filepath.Join(workingRoot, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create invocation home directory: %v", err)
	}
	if err := copyHelpDirectory(fixture.homeDir, homeDir); err != nil {
		t.Fatalf("copy prepared Factory home into invocation root: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	inputs := support.FakeInputs(ctx, append([]string(nil), args...))
	inputs.Input.Context = ctx
	inputs.Input.Args = append([]string(nil), args...)
	inputs.Input.Env = helpProcessEnvironment(homeDir)
	inputs.Input.Stdin = strings.NewReader("")
	inputs.Input.WorkingDirectory = workingRoot
	stdinTTY, stdoutTTY, stderrTTY := false, false, false
	inputs.Input.StdinIsTTY = &stdinTTY
	inputs.Input.StdoutIsTTY = &stdoutTTY
	inputs.Input.StderrIsTTY = &stderrTTY

	resources := helpInvocationResources{
		id:          id,
		workingRoot: workingRoot,
	}
	fixture.openInvocation(resources)
	defer func() {
		cancel()
		fixture.closeInvocation(t, resources)
	}()

	err := fixture.process.Execute(inputs.Input)
	return helpInvocationResult{inputs: inputs, err: err}
}

func copyHelpDirectory(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(target, 0o755)
		}
		targetPath := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode().Perm())
	})
}

func helpProcessEnvironment(homeDir string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok && (strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE")) {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return env
}

func (fixture *helpPackageFixture) openInvocation(resources helpInvocationResources) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.activeInvocations[resources.id] = struct{}{}
}

func (fixture *helpPackageFixture) closeInvocation(t testing.TB, resources helpInvocationResources) {
	t.Helper()
	fixture.mu.Lock()
	_, wasActive := fixture.activeInvocations[resources.id]
	delete(fixture.activeInvocations, resources.id)
	fixture.mu.Unlock()
	if !wasActive {
		t.Errorf("help invocation %q was closed more than once", resources.id)
	}
	if err := os.RemoveAll(resources.workingRoot); err != nil {
		t.Errorf("remove help invocation root %q: %v", resources.workingRoot, err)
	}
}

func closeHelpPackageFixture() error {
	fixture := helpPackageFixtureState.fixture
	if fixture == nil {
		return nil
	}
	fixture.closeOnce.Do(func() {
		var errs []error
		closeCtx, cancel := context.WithTimeout(context.Background(), helpPackageCloseTimeout)
		processCloseErr := fixture.process.Close(closeCtx)
		cancel()
		if processCloseErr != nil {
			errs = append(errs, fmt.Errorf("close help application process: %w", processCloseErr))
		}

		fixture.mu.Lock()
		active := len(fixture.activeInvocations)
		fixture.mu.Unlock()
		if active != 0 {
			errs = append(errs, fmt.Errorf("active help invocation handles after cleanup = %d", active))
		}
		if got := fixture.processBuilds.Load(); got != 1 {
			errs = append(errs, fmt.Errorf("root application builds = %d, want exactly one", got))
		}
		if got := fixture.providerRunner.CallCount(); got != 0 {
			errs = append(errs, fmt.Errorf("provider command calls after help cleanup = %d, want zero", got))
		}

		rootRemoved := false
		if err := os.RemoveAll(fixture.rootDir); err != nil {
			errs = append(errs, fmt.Errorf("remove help package root: %w", err))
		} else {
			_, statErr := os.Stat(fixture.rootDir)
			rootRemoved = errors.Is(statErr, os.ErrNotExist)
			if !rootRemoved {
				errs = append(errs, fmt.Errorf("help package root remains after cleanup: %v", statErr))
			}
		}
		fmt.Fprintf(
			os.Stderr,
			"help package topology: builds=%d active_invocations=%d provider_calls=%d process_closed=%t root_removed=%t\n",
			fixture.processBuilds.Load(), active, fixture.providerRunner.CallCount(),
			processCloseErr == nil, rootRemoved,
		)
		fixture.closeErr = errors.Join(errs...)
	})
	return fixture.closeErr
}
