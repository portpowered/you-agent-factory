package executor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

func TestPortableCopiedScriptExecution_PreservesCommandArgsWorkingDirectoryAndEnvironment(t *testing.T) {
	factoryDir := t.TempDir()
	worker := &interfaces.FactoryWorkerConfig{
		Name:    "executor",
		Type:    interfaces.WorkerTypeScript,
		Command: "powershell",
		Args:    []string{"-File", "scripts/execute-story.ps1"},
		Timeout: "45m",
	}
	runner := &portableRecordingScriptRunner{stdout: "portable copied script accepted"}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			FactoryPath: factoryDir,
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"executor": worker,
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"execute-story": {
					Type:             interfaces.WorkstationTypeModel,
					WorkingDirectory: "repo/{{ (index .Inputs 0).WorkID }}",
					Env:              map[string]string{"SCRIPT_MODE": "portable"},
				},
			},
		},
		NewScriptExecutorWithRunner(worker, runner, nil, "", nil, time.Now),
	)
	we.Renderer = portableStaticPromptRenderer("")

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-1",
		TransitionID:    "transition-1",
		WorkerType:      "executor",
		WorkstationName: "execute-story",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID:      "token-1",
			PlaceID: "task:init",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:     "work-001",
				WorkTypeID: "task",
				DataType:   factoryruntime.RuntimeTokenDataTypeWork,
				TraceID:    "trace-001",
				Payload:    []byte("portable task"),
			},
		}),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted ||
		result.Output != "portable copied script accepted" {
		t.Fatalf("result = %#v, want accepted portable output", result)
	}

	request := runner.request
	if request.Command != "powershell" {
		t.Fatalf("command = %q, want powershell", request.Command)
	}
	if len(request.Args) != 2 ||
		request.Args[0] != "-File" ||
		request.Args[1] != "scripts/execute-story.ps1" {
		t.Fatalf("args = %#v, want portable script path", request.Args)
	}
	if request.WorkDir != filepath.Join(factoryDir, "repo", "work-001") {
		t.Fatalf("work dir = %q, want Factory-relative work directory", request.WorkDir)
	}
	if !portableEnvironmentContains(request.Env, "SCRIPT_MODE=portable") {
		t.Fatalf("environment = %v, want SCRIPT_MODE=portable", request.Env)
	}
}

type portableStaticPromptRenderer string

func (renderer portableStaticPromptRenderer) Render(
	string,
	[]workerexecution.Token,
	*workerexecution.Context,
) (string, error) {
	return string(renderer), nil
}

type portableRecordingScriptRunner struct {
	request CommandRequest
	stdout  string
}

func (runner *portableRecordingScriptRunner) Run(
	_ context.Context,
	request CommandRequest,
) (CommandResult, error) {
	request.Args = append([]string(nil), request.Args...)
	request.Env = append([]string(nil), request.Env...)
	runner.request = request
	return CommandResult{Stdout: []byte(runner.stdout)}, nil
}

func portableEnvironmentContains(environment []string, want string) bool {
	for _, value := range environment {
		if value == want {
			return true
		}
	}
	return false
}

func assertScriptFailureMetadata(
	t *testing.T,
	got *workerexecution.WorkFailureMetadata,
	want *workerexecution.WorkFailureType,
) {
	t.Helper()
	if want == nil && got != nil {
		t.Fatalf("failure metadata = %#v, want nil", got)
	}
	if want != nil && (got == nil || got.Type != *want) {
		t.Fatalf("failure metadata = %#v, want type %q", got, *want)
	}
}

type scriptTestTB interface {
	Helper()
	Fatalf(string, ...any)
}

func bindLegacyScriptRunner(t scriptTestTB, executor *ScriptExecutor) {
	t.Helper()
	docs := executor.FactoryDocs
	if docs == nil {
		docs = func(string) (map[string]string, error) { return nil, nil }
	}
	binding, err := resolveScriptRunner(
		&interfaces.FactoryWorkerConfig{
			Command: executor.Command,
			Args:    append([]string(nil), executor.Args...),
		},
		streamingTestCommandRunner(executor.CommandRunner),
		executor.FactoryDir,
		executor.Publish,
		executor.recorder,
		executor.Now,
		docs,
	)
	if err != nil {
		t.Fatalf("resolve legacy test Script Runner: %v", err)
	}
	executor.runner = binding.Runner
}

type completeOutputTestCommandRunner struct {
	CommandRunner
}

func streamingTestCommandRunner(runner CommandRunner) CommandRunner {
	if _, ok := runner.(interface {
		RunStreaming(
			context.Context,
			CommandRequest,
			workerprocess.OutputChunkObserver,
		) (CommandResult, error)
	}); ok {
		return runner
	}
	return completeOutputTestCommandRunner{CommandRunner: runner}
}

func (runner completeOutputTestCommandRunner) RunStreaming(
	ctx context.Context,
	request CommandRequest,
	observer workerprocess.OutputChunkObserver,
) (CommandResult, error) {
	result, err := runner.Run(ctx, request)
	if observer != nil && len(result.Stdout) > 0 {
		observer(workerprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
	}
	if observer != nil && len(result.Stderr) > 0 {
		observer(workerprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
	}
	return result, err
}

func NewScriptExecutor(
	definition *interfaces.FactoryWorkerConfig,
	logger logging.Logger,
	factoryDirectory string,
	record workerexecution.ScriptEventRecorder,
	now func() time.Time,
) *ScriptExecutor {
	executor := &ScriptExecutor{Logger: logger, recorder: record, Now: now}
	if definition != nil {
		executor.Command = definition.Command
		executor.Args = append([]string(nil), definition.Args...)
		executor.FactoryDir = strings.TrimSpace(factoryDirectory)
	}
	return executor
}

func NewScriptExecutorWithRunner(
	definition *interfaces.FactoryWorkerConfig,
	runner CommandRunner,
	logger logging.Logger,
	factoryDirectory string,
	record workerexecution.ScriptEventRecorder,
	now func() time.Time,
) *ScriptExecutor {
	executor := NewScriptExecutor(definition, logger, factoryDirectory, record, now)
	executor.CommandRunner = runner
	if definition != nil && runner != nil && now != nil {
		bindLegacyScriptRunner(testPanicTB{}, executor)
	}
	return executor
}

func NewScriptExecutorWithDependencies(
	definition *interfaces.FactoryWorkerConfig,
	commandRunner CommandRunner,
	logger logging.Logger,
	factoryDirectory string,
	record workerexecution.ScriptEventRecorder,
	now func() time.Time,
	factoryDocs workerexecution.FactoryDocsLoader,
) (*ScriptExecutor, error) {
	if definition == nil {
		return nil, errors.New("construct script worker: definition is required")
	}
	if commandRunner == nil {
		return nil, errors.New("construct script worker: command runner is required")
	}
	if factoryDocs == nil {
		return nil, errors.New("construct script worker: Factory docs loader is required")
	}
	executor := NewScriptExecutor(definition, logger, factoryDirectory, record, now)
	executor.CommandRunner = commandRunner
	executor.FactoryDocs = factoryDocs
	return executor, nil
}

type testPanicTB struct{}

func (testPanicTB) Helper() {}
func (testPanicTB) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

func TestScriptFactoryConstructsRegistryBackedExecutor(t *testing.T) {
	command := streamingTestCommandRunner(fixedCommandRunner{stdout: []byte("factory output")})
	clock := workerprocess.ClockFunc(func() time.Time {
		return time.Date(2026, time.July, 26, 23, 0, 0, 0, time.UTC)
	})
	docs := func(string) (map[string]string, error) {
		return map[string]string{}, nil
	}
	factory, err := NewScriptFactory(command, clock, docs)
	if err != nil {
		t.Fatalf("NewScriptFactory() error = %v", err)
	}
	executor, err := factory.New(
		&interfaces.FactoryWorkerConfig{Command: "fixture"},
		nil,
		"factory-root",
		nil,
		nil,
		clock.Now,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := executor.Execute(t.Context(), testScriptRequest(work.WorkDispatch{
		DispatchID:   "dispatch-factory",
		TransitionID: "transition-factory",
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted || result.Output != "factory output" {
		t.Fatalf("Execute() result = %#v, want accepted factory output", result)
	}

	replaced, err := factory.WithCommandRunner(streamingTestCommandRunner(fixedCommandRunner{stdout: []byte("replacement")}))
	if err != nil || replaced == factory {
		t.Fatalf("WithCommandRunner(replacement) = (%p, %v), want detached factory", replaced, err)
	}
}

func TestScriptFactoryRejectsIncompleteDependencies(t *testing.T) {
	clock := workerprocess.ClockFunc(time.Now)
	docs := func(string) (map[string]string, error) { return nil, nil }
	for _, tc := range []struct {
		name   string
		runner CommandRunner
		clock  workerprocess.Clock
		docs   workerexecution.FactoryDocsLoader
	}{
		{name: "command runner", clock: clock, docs: docs},
		{name: "clock", runner: fixedCommandRunner{}, docs: docs},
		{name: "Factory docs", runner: fixedCommandRunner{}, clock: clock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewScriptFactory(tc.runner, tc.clock, tc.docs); err == nil {
				t.Fatalf("NewScriptFactory() error = nil, want missing %s", tc.name)
			}
		})
	}
}

func TestNewScriptExecutorRejectsIncompleteRegistryDependencies(t *testing.T) {
	command := fixedCommandRunner{}
	docs := func(string) (map[string]string, error) { return nil, nil }
	if _, err := newScriptExecutor(
		nil, command, nil, "", nil, nil, time.Now, docs,
	); err == nil {
		t.Fatal("newScriptExecutor() error = nil, want missing definition")
	}
	if executor := NewScriptExecutor(nil, nil, "", nil, time.Now); executor == nil {
		t.Fatal("NewScriptExecutor(nil) = nil, want compatibility executor")
	}
}

func TestScriptExecutionErrorMessageFallsBackForUnclassifiedError(t *testing.T) {
	err := errors.New("unclassified")
	if got := scriptExecutionErrorMessage(err); got != "execution cancelled: unclassified" {
		t.Fatalf("scriptExecutionErrorMessage() = %q", got)
	}
}
