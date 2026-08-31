package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const processCommandStopTimeout = 5 * time.Second

// Process is the exact customer-process capability used by functional tests.
type Process interface {
	Execute(root.Input) error
}

// ACPServer is the protocol capability exposed by a root-built process.
type ACPServer interface {
	Serve(context.Context, io.Reader, io.Writer) error
}

// ProviderRegistry is the immutable provider identity capability exposed by a
// root-built process.
type ProviderRegistry interface {
	CanonicalIdentity(string) (string, error)
}

// ApplicationProcess is the lifecycle-capable process returned by the
// context-aware functional construction boundary.
type ApplicationProcess interface {
	Process
	Close(context.Context) error
	ACPServer() ACPServer
	ProviderRegistry() ProviderRegistry
	WorkerRecordingReader() recordings.WorkerRecordingReader
	ExecutionRuntimeOpening() factorysessions.ExecutionRuntimeOpeningFunc
}

type applicationProcess struct {
	Process
	close            func(context.Context) error
	acpServer        ACPServer
	providerRegistry ProviderRegistry
	recordingReader  recordings.WorkerRecordingReader
	executionOpening factorysessions.ExecutionRuntimeOpeningFunc
}

func (p applicationProcess) Close(ctx context.Context) error {
	return p.close(ctx)
}

func (p applicationProcess) ACPServer() ACPServer {
	return p.acpServer
}

func (p applicationProcess) ProviderRegistry() ProviderRegistry {
	return p.providerRegistry
}

func (p applicationProcess) WorkerRecordingReader() recordings.WorkerRecordingReader {
	return p.recordingReader
}

// ExecutionRuntimeOpening returns the exact public durable-runtime opening
// capability already exposed by the root-built process. Functional tests use
// this only for scenarios whose original assertion enters the runtime through
// the Process.Execute opening boundary (for example seeded replay); it does
// not construct a second service graph or retain runtime state in the support
// wrapper.
func (p applicationProcess) ExecutionRuntimeOpening() factorysessions.ExecutionRuntimeOpeningFunc {
	return p.executionOpening
}

// BuildProcess constructs the same reusable process used by the production
// command entrypoint.
func BuildProcess(t testing.TB, edges serviceedges.Edges) ApplicationProcess {
	t.Helper()
	process, err := BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	return process
}

// BuildProcessWithContext constructs the production application process with
// functional-test-safe external-effect defaults. Explicit scenario edges win
// over these defaults, including browser recorders used by dashboard tests.
func BuildProcessWithContext(
	ctx context.Context,
	edges serviceedges.Edges,
) (ApplicationProcess, error) {
	process, _, err := buildProcessWithContext(ctx, edges)
	return process, err
}

func functionalDefaultEdges() serviceedges.Edges {
	return serviceedges.Edges{
		BrowserOpener: func(context.Context, string) error { return nil },
	}
}

// BuildProcessWithRecordingReader constructs the same reusable process used by
// the production command entrypoint and returns its selected recording read
// capability alongside the customer process.
func BuildProcessWithRecordingReader(
	t testing.TB, edges serviceedges.Edges,
) (Process, recordings.WorkerRecordingReader) {
	t.Helper()
	process, recordingReader, err := buildProcessWithContext(context.Background(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	return process, recordingReader
}

func buildProcessWithContext(
	ctx context.Context,
	edges serviceedges.Edges,
) (ApplicationProcess, recordings.WorkerRecordingReader, error) {
	process, err := root.BuildProcess(ctx, serviceedges.Merge(functionalDefaultEdges(), edges))
	if err != nil {
		return nil, nil, err
	}
	recordingReader := root.WorkerRecordingReaderFromProcess(process)
	var executionOpening factorysessions.ExecutionRuntimeOpeningFunc
	if capability := process.ExecutionRuntimeOpening(); capability != nil {
		var ok bool
		executionOpening, ok = capability.ExecutionRuntimeOpening().(factorysessions.ExecutionRuntimeOpeningFunc)
		if !ok || executionOpening == nil {
			return nil, nil, fmt.Errorf(
				"root process execution opening = %T, want factorysessions.ExecutionRuntimeOpeningFunc",
				capability.ExecutionRuntimeOpening(),
			)
		}
	}
	functionalProcess := applicationProcess{
		Process:          process,
		close:            process.Close,
		acpServer:        process.ACPServer(),
		providerRegistry: process.ProviderRegistry(),
		recordingReader:  recordingReader,
		executionOpening: executionOpening,
	}
	return functionalProcess, recordingReader, nil
}

// RequireSafeCLIDiagnostic verifies the process-boundary fallback used when a
// command failure has no authored public diagnostic contract.
func RequireSafeCLIDiagnostic(t testing.TB, stderr string) factoryapi.ErrorResponse {
	t.Helper()
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &response); err != nil {
		t.Fatalf("decode safe CLI diagnostic: %v\nstderr=%q", err, stderr)
	}
	if response.Code != factoryapi.ErrorResponseCode("CLI_COMMAND_FAILED") || response.Message != "command failed" {
		t.Fatalf("safe CLI diagnostic = %#v, want CLI_COMMAND_FAILED/command failed", response)
	}
	return response
}

// RequireNotFoundCLIDiagnostic verifies the server-owned not-found response
// preserved by the public CLI boundary.
func RequireNotFoundCLIDiagnostic(t testing.TB, stderr string) factoryapi.ErrorResponse {
	t.Helper()
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &response); err != nil {
		t.Fatalf("decode CLI not-found diagnostic: %v\nstderr=%q", err, stderr)
	}
	if response.Code != factoryapi.ErrorResponseCodeNOTFOUND ||
		response.Family != factoryapi.ErrorFamilyNotFound ||
		strings.TrimSpace(response.Message) == "" {
		t.Fatalf("CLI not-found diagnostic = %#v, want NOT_FOUND code/family and a message", response)
	}
	return response
}

// CleanupProcess closes a caller-owned reusable process after its sequential
// public invocations finish. Process wiring is immutable, but lifecycle-owned
// resources may be retained by commands until the process is closed.
func CleanupProcess(t testing.TB, process Process) {
	t.Helper()
	closer, ok := process.(interface{ Close(context.Context) error })
	if !ok {
		return
	}
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), processCommandStopTimeout)
		defer cancel()
		if err := closer.Close(closeCtx); err != nil {
			t.Errorf("close reusable application process: %v", err)
		}
	})
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

// CopyFactoryAsNamed copies a customized fixture to a customer-owned named
// Factory. Bundled @you directories are refreshed during every initialization,
// so tests that intentionally edit a packaged payload must invoke the copy.
func CopyFactoryAsNamed(t testing.TB, sourceDir, homeDir, name string) string {
	t.Helper()
	targetDir := filepath.Join(
		append([]string{homeDir, ".you-agent-factory", "factories"}, strings.Split(name, "/")...)...,
	)
	if err := copyFactoryDirectory(sourceDir, targetDir); err != nil {
		t.Fatalf("copy Factory fixture %q to %q: %v", sourceDir, targetDir, err)
	}
	return targetDir
}

func copyFactoryDirectory(sourceDir, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(targetDir, 0o755)
		}
		targetPath := filepath.Join(targetDir, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode().Perm())
	})
}

// InstallPackagedFactoryWithProcess materializes a packaged Factory through a
// caller-owned root process and invocation-local environment.
func InstallPackagedFactoryWithProcess(
	t testing.TB,
	process Process,
	env []string,
	workingDirectory string,
	name string,
) string {
	t.Helper()
	namedFactoriesRoot := InitializeCustomerHomeWithProcess(t, process, env, workingDirectory)
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

	process := BuildProcess(t, serviceedges.Edges{})
	CleanupProcess(t, process)
	return CreateNamedFactoryWithProcess(
		t,
		process,
		homeDir,
		workingDirectory,
		name,
		factoryConfigPath,
	)
}

// CreateNamedFactoryWithProcess executes the public named Factory create flow
// on a caller-owned reusable process. Only immutable process wiring is shared;
// the home, environment, working directory, and Factory root remain local to
// this invocation.
func CreateNamedFactoryWithProcess(
	t testing.TB,
	process Process,
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
	namedFactoriesRoot := InitializeCustomerHomeWithProcess(t, process, env, workingDirectory)
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
	process := BuildProcess(t, serviceedges.Edges{})
	CleanupProcess(t, process)
	return InitializeCustomerHomeWithProcess(t, process, env, workingDirectory)
}

// InitializeCustomerHomeWithProcess performs the same public missing-Factory
// bootstrap probe on a caller-owned process and returns the invocation's
// isolated named-Factories root.
func InitializeCustomerHomeWithProcess(
	t testing.TB,
	process Process,
	env []string,
	workingDirectory string,
) string {
	t.Helper()
	if process == nil {
		t.Fatal("InitializeCustomerHomeWithProcess requires a process")
	}
	homeDir := environmentHome(env)
	namedFactoriesRoot := filepath.Join(homeDir, ".you-agent-factory", "factories")
	missingFactory := filepath.Join(workingDirectory, "missing-initialization-factory.json")
	inputs := FakeInputs(t.Context(), []string{
		"you", "run", "--factory", missingFactory,
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDirectory
	err := process.Execute(inputs.Input)
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
