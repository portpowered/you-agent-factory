package support

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

// InstallPackagedFactory executes the customer-facing system initialization
// command through the root-built process and returns the installed named
// Factory directory. Functional tests use this instead of importing packaged
// definition implementations or persisting their JSON directly.
func InstallPackagedFactory(t testing.TB, homeDir, name string) string {
	t.Helper()

	inputs := FakeInputs(t.Context(), []string{"you", "--json", "config", "init"})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(config init) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	var result configInitResult
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatalf("decode config init result: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	for _, factory := range result.PackagedFactories {
		if factory.Name == name {
			return factory.FactoryDir
		}
	}
	t.Fatalf("config init result omitted packaged Factory %q: %#v", name, result.PackagedFactories)
	return ""
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
	initResult := initializeCustomerHome(t, env, workingDirectory)
	return createNamedFactoryAtRoot(t, env, workingDirectory, initResult.NamedFactoriesRoot, name, factoryConfigPath)
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
	return createNamedFactoryAtRoot(
		t,
		os.Environ(),
		workingDirectory,
		namedFactoriesRoot,
		name,
		factoryConfigPath,
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
	return createNamedFactoryAtRootWithActivation(
		t,
		os.Environ(),
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
	return createNamedFactoryAtRootWithActivation(
		t,
		env,
		workingDirectory,
		namedFactoriesRoot,
		name,
		factoryConfigPath,
		false,
	)
}

func createNamedFactoryAtRootWithActivation(
	t testing.TB,
	env []string,
	workingDirectory string,
	namedFactoriesRoot string,
	name string,
	factoryConfigPath string,
	setCurrent bool,
) string {
	t.Helper()

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
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDirectory
	if err := BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
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

type configInitResult struct {
	NamedFactoriesRoot string                  `json:"namedFactoriesRoot"`
	PackagedFactories  []packagedFactoryResult `json:"packagedFactories"`
}

type packagedFactoryResult struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDirectory"`
}

type createNamedFactoryResult struct {
	FactoryDir string `json:"factoryDir"`
}

func initializeCustomerHome(t testing.TB, env []string, workingDirectory string) configInitResult {
	t.Helper()
	inputs := FakeInputs(t.Context(), []string{"you", "--json", "config", "init"})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDirectory
	if err := BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(config init) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var result configInitResult
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatalf("decode config init result: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	if result.NamedFactoriesRoot == "" {
		t.Fatalf("config init result omitted named Factories root: %s", inputs.Stdout())
	}
	return result
}

// RunInitCommand executes the customer-facing init command through the same
// root-built process used by the production binary.
func RunInitCommand(t testing.TB, dir string, extraArgs ...string) {
	t.Helper()

	args := []string{"you", "init", "--dir", dir}
	args = append(args, extraArgs...)
	inputs := FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = dir
	if err := BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
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
