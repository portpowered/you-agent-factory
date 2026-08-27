package cross

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

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const crossSharedFixtureShutdownTimeout = 15 * time.Second

// crossSharedProcessFixture owns the immutable root process used by the
// shareable cross-package scenarios. The parity cases share one routed local
// API server while the inspect and idle lifecycle cases retain isolated
// servers; Factory Session state remains scoped to each explicit session.
type crossSharedProcessFixture struct {
	rootDir    string
	factoryDir string
	process    support.ApplicationProcess
	router     *crossAPIServerRouter
}

type crossAPIServerRouter struct {
	mu     sync.Mutex
	server *support.ProcessAPIServer
}

func (router *crossAPIServerRouter) set(server *support.ProcessAPIServer) {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.server = server
}

func (router *crossAPIServerRouter) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	router.mu.Lock()
	server := router.server
	router.mu.Unlock()
	if server == nil {
		return errors.New("cross shared API server is not selected")
	}
	crossCharacterization.recordServerStart()
	err := server.Start(ctx, request)
	if err == nil {
		crossCharacterization.recordServerClose()
	}
	return err
}

var (
	crossFixtureOnce sync.Once
	crossFixture     *crossSharedProcessFixture
	crossFixtureErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if crossFixture != nil {
		if err := crossFixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared cross process fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	if err := crossCharacterization.validateAfterSuite(); err != nil {
		fmt.Fprintf(os.Stderr, "packaged cross characterization: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	if crossCharacterization.completedScenarioCount() == crossCharacterizationExpectedScenarios {
		fmt.Fprintf(os.Stderr, "packaged cross characterization: %s\n", crossCharacterization.summary())
	}
	os.Exit(code)
}

func sharedCrossProcess(t *testing.T) *crossSharedProcessFixture {
	t.Helper()
	crossFixtureOnce.Do(func() {
		crossFixture, crossFixtureErr = startCrossSharedProcessFixture()
	})
	if crossFixtureErr != nil {
		t.Fatalf("start shared cross process fixture: %v", crossFixtureErr)
	}
	if crossFixture == nil {
		t.Fatal("shared cross process fixture is unavailable")
	}
	return crossFixture
}

func startCrossSharedProcessFixture() (*crossSharedProcessFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-packaged-cross-")
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

	router := &crossAPIServerRouter{}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: router.start,
	})
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}
	env := isolatedHomeEnvironment(homeDir)
	if err := initializeCrossHome(process, env, workingDir); err != nil {
		closeCrossProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("initialize packaged Factory home: %w", err)
	}
	factoryDir := filepath.Join(
		homeDir,
		".you-agent-factory",
		"factories",
		filepath.FromSlash(factorydefinitions.PackagedGoalFactoryName),
	)
	if _, err := os.Stat(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)); err != nil {
		closeCrossProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("find packaged goal Factory at %s: %w", factoryDir, err)
	}

	return &crossSharedProcessFixture{
		rootDir: rootDir, factoryDir: factoryDir, process: process, router: router,
	}, nil
}

func initializeCrossHome(
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

func closeCrossProcess(process support.ApplicationProcess) {
	if process == nil {
		return
	}
	closeContext, cancel := context.WithTimeout(context.Background(), crossSharedFixtureShutdownTimeout)
	defer cancel()
	_ = process.Close(closeContext)
}

func (fixture *crossSharedProcessFixture) close() error {
	if fixture == nil || fixture.process == nil {
		return nil
	}
	closeContext, cancel := context.WithTimeout(context.Background(), crossSharedFixtureShutdownTimeout)
	err := fixture.process.Close(closeContext)
	cancel()
	if removeErr := os.RemoveAll(fixture.rootDir); removeErr != nil {
		if err != nil {
			return errors.Join(err, fmt.Errorf("remove fixture root: %w", removeErr))
		}
		return fmt.Errorf("remove fixture root: %w", removeErr)
	}
	if _, statErr := os.Stat(fixture.rootDir); !os.IsNotExist(statErr) {
		if err != nil {
			return errors.Join(err, fmt.Errorf("fixture root %q remains; stat error: %v", fixture.rootDir, statErr))
		}
		return fmt.Errorf("fixture root %q remains; stat error: %v", fixture.rootDir, statErr)
	}
	crossCharacterization.recordSharedFixtureCleanup()
	return err
}

func installSharedPackagedGoal(t *testing.T) (string, string) {
	t.Helper()
	fixture := sharedCrossProcess(t)
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	env := isolatedHomeEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		fixture.process,
		env,
		workingDir,
		factorydefinitions.PackagedGoalFactoryName,
	)
	return homeDir, factoryDir
}

type crossHostedCommand struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	err     error
	stopped bool
}

func startCrossHostedCommand(
	t testing.TB,
	process support.Process,
	inputs *support.CapturedInputs,
) *crossHostedCommand {
	t.Helper()
	if process == nil {
		t.Fatal("startCrossHostedCommand requires a process")
	}
	if inputs == nil {
		t.Fatal("startCrossHostedCommand requires inputs")
	}
	parent := inputs.Input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	inputs.Input.Context = ctx
	command := &crossHostedCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(inputs.Input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	t.Cleanup(func() { command.stop(t) })
	return command
}

func (command *crossHostedCommand) stop(t testing.TB) {
	t.Helper()
	if command == nil {
		return
	}
	command.mu.Lock()
	if command.stopped {
		command.mu.Unlock()
		return
	}
	command.stopped = true
	cancel := command.cancel
	command.mu.Unlock()
	cancel()
	select {
	case <-command.done:
		command.mu.Lock()
		err := command.err
		command.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("shared cross hosted command shutdown: %v", err)
		}
	case <-time.After(crossSharedFixtureShutdownTimeout):
		t.Errorf("timed out waiting for shared cross hosted command shutdown")
	}
}
