package support

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

const processCommandStopTimeout = 5 * time.Second

// Process is the exact customer-process capability used by functional tests.
type Process interface {
	Execute(root.Input) error
}

// BuildProcess constructs the same reusable process used by the production
// command entrypoint.
func BuildProcess(t testing.TB, edges serviceedges.Edges) Process {
	t.Helper()
	process, err := root.BuildProcess(context.Background(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	return process
}

// ExecuteProcess runs one customer command through the shared root-built process.
func ExecuteProcess(t testing.TB, inputs *CapturedInputs) error {
	t.Helper()
	return BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
}

// InstallPackagedFactory enters through a normal runtime command so the
// Initializer materializes packaged Factories, then returns the selected path.
func InstallPackagedFactory(t testing.TB, homeDir, name string) string {
	t.Helper()

	workingDirectory := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	namedFactoriesRoot := initializeCustomerHome(t, env, workingDirectory)
	factoryDir := filepath.Join(namedFactoriesRoot, filepath.FromSlash(name))
	if _, err := os.Stat(filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatalf("initializer omitted packaged Factory %q at %s: %v", name, factoryDir, err)
	}
	return factoryDir
}

// CreateNamedFactory executes the customer-facing named Factory create command
// through the root-built process and returns the persisted Factory directory.
func CreateNamedFactory(
	t testing.TB,
	homeDir string,
	workingDirectory string,
	name string,
	factoryConfigPath string,
) string {
	t.Helper()

	env := append(
		os.Environ(),
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
	)
	namedFactoriesRoot := initializeCustomerHome(t, env, workingDirectory)
	return createNamedFactoryAtRoot(t, env, workingDirectory, namedFactoriesRoot, name, factoryConfigPath)
}

// CreateNamedFactoryAtRoot executes the public create command against an
// explicitly supplied named-Factories root. Functional server fixtures use
// this customer boundary before starting a server over that root.
func CreateNamedFactoryAtRoot(
	t testing.TB,
	workingDirectory string,
	namedFactoriesRoot string,
	name string,
	factoryConfigPath string,
) string {
	t.Helper()
	return createNamedFactoryAtRootWithProcess(
		t,
		BuildProcess(t, serviceedges.Edges{}),
		os.Environ(),
		workingDirectory,
		namedFactoriesRoot,
		name,
		factoryConfigPath,
		false,
	)
}

// CreateAndActivateNamedFactoryAtRoot executes the public create command with
// --set-current and returns the directory reported by its JSON result.
func CreateAndActivateNamedFactoryAtRoot(
	t testing.TB,
	workingDirectory string,
	namedFactoriesRoot string,
	name string,
	factoryConfigPath string,
) string {
	t.Helper()
	return createNamedFactoryAtRootWithProcess(
		t,
		BuildProcess(t, serviceedges.Edges{}),
		os.Environ(),
		workingDirectory,
		namedFactoriesRoot,
		name,
		factoryConfigPath,
		true,
	)
}

// CreateNamedFactoryAtRootWithProcess executes the public Factory create
// command on a caller-owned reusable process. The process is immutable wiring;
// the environment, working directory, streams, and Factory root remain local
// to this invocation.
func CreateNamedFactoryAtRootWithProcess(
	t testing.TB,
	process Process,
	env []string,
	workingDirectory string,
	namedFactoriesRoot string,
	name string,
	factoryConfigPath string,
) string {
	t.Helper()
	return createNamedFactoryAtRootWithProcess(
		t,
		process,
		env,
		workingDirectory,
		namedFactoriesRoot,
		name,
		factoryConfigPath,
		false,
	)
}

// CreateAndActivateNamedFactoryAtRootWithProcess executes the public Factory
// create-and-activate command on a caller-owned reusable process.
func CreateAndActivateNamedFactoryAtRootWithProcess(
	t testing.TB,
	process Process,
	env []string,
	workingDirectory string,
	namedFactoriesRoot string,
	name string,
	factoryConfigPath string,
) string {
	t.Helper()
	return createNamedFactoryAtRootWithProcess(
		t,
		process,
		env,
		workingDirectory,
		namedFactoriesRoot,
		name,
		factoryConfigPath,
		true,
	)
}

func createNamedFactoryAtRoot(
	t testing.TB,
	env []string,
	workingDirectory string,
	namedFactoriesRoot string,
	name string,
	factoryConfigPath string,
) string {
	return createNamedFactoryAtRootWithProcess(
		t,
		BuildProcess(t, serviceedges.Edges{}),
		env,
		workingDirectory,
		namedFactoriesRoot,
		name,
		factoryConfigPath,
		false,
	)
}

func createNamedFactoryAtRootWithProcess(
	t testing.TB,
	process Process,
	env []string,
	workingDirectory string,
	namedFactoriesRoot string,
	name string,
	factoryConfigPath string,
	setCurrent bool,
) string {
	t.Helper()
	if process == nil {
		t.Fatal("CreateNamedFactoryAtRootWithProcess requires a process")
	}

	args := []string{
		"you",
		"--json",
		"factory",
		"create",
		name,
		"--from",
		factoryConfigPath,
		"--dir",
		namedFactoriesRoot,
	}
	if setCurrent {
		args = append(args, "--set-current")
	}
	inputs := FakeInputs(
		t.Context(),
		args,
	)
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory create %q) error = %v\nstdout:\n%s\nstderr:\n%s",
			name,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	var result createNamedFactoryResult
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatalf("decode factory create result: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	if _, err := os.Stat(
		filepath.Join(result.FactoryDir, "factory.json"),
	); err != nil {
		t.Fatalf(
			"customer-created named Factory %q missing factory.json at %s: %v",
			name,
			result.FactoryDir,
			err,
		)
	}
	return result.FactoryDir
}

type createNamedFactoryResult struct {
	FactoryDir string `json:"factoryDir"`
}

func initializeCustomerHome(t testing.TB, env []string, workingDirectory string) string {
	t.Helper()
	homeDir := environmentHome(env)
	namedFactoriesRoot := filepath.Join(homeDir, ".you-agent-factory", "factories")
	missingFactory := filepath.Join(workingDirectory, "missing-initialization-factory.json")
	inputs := FakeInputs(t.Context(), []string{
		"you", "run", "--factory", missingFactory,
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDirectory
	err := BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		t.Fatalf(
			"Process.Execute(run missing Factory) error = %v, want missing Factory diagnostic\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return namedFactoriesRoot
}

func environmentHome(env []string) string {
	for index := len(env) - 1; index >= 0; index-- {
		item := env[index]
		if strings.HasPrefix(item, "HOME=") {
			return strings.TrimPrefix(item, "HOME=")
		}
		if strings.HasPrefix(item, "USERPROFILE=") {
			return strings.TrimPrefix(item, "USERPROFILE=")
		}
	}
	return ""
}

// ProcessCommand owns one asynchronous Process.Execute invocation.
type ProcessCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu            sync.Mutex
	err           error
	errorAccepted bool
}

// StartProcessCommand starts one command on an existing reusable process.
func StartProcessCommand(
	t testing.TB,
	process Process,
	input root.Input,
) *ProcessCommand {
	t.Helper()
	if process == nil {
		t.Fatal("StartProcessCommand requires a process")
	}
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &ProcessCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	t.Cleanup(func() {
		command.Stop(t)
	})
	return command
}

// Stop cancels the invocation and waits for all initializer-owned lifecycle
// work to finish. Cancellation is a successful daemon shutdown.
func (command *ProcessCommand) Stop(t testing.TB) {
	t.Helper()
	if command == nil {
		return
	}
	command.cancel()
	select {
	case <-command.done:
		err := command.Err()
		if err != nil && !errors.Is(err, context.Canceled) && !command.errorWasAccepted() {
			t.Errorf("Process.Execute() after shutdown error = %v", err)
		}
	case <-time.After(processCommandStopTimeout):
		t.Errorf("timed out waiting for Process.Execute() shutdown")
	}
}

// AcceptError marks a terminal command error as asserted by the caller.
func (command *ProcessCommand) AcceptError() {
	if command == nil {
		return
	}
	command.mu.Lock()
	command.errorAccepted = true
	command.mu.Unlock()
}

func (command *ProcessCommand) errorWasAccepted() bool {
	command.mu.Lock()
	defer command.mu.Unlock()
	return command.errorAccepted
}

// Done closes after Process.Execute returns.
func (command *ProcessCommand) Done() <-chan struct{} {
	if command == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return command.done
}

// Err returns the terminal Execute error after Done closes.
func (command *ProcessCommand) Err() error {
	if command == nil {
		return nil
	}
	command.mu.Lock()
	defer command.mu.Unlock()
	return command.err
}
