package script_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/executor"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

func testScriptCommandRunner(t testing.TB) executor.CommandRunner {
	t.Helper()
	effect, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil)
	if err != nil {
		t.Fatalf("NewExecCommandRunner() error = %v", err)
	}
	return workerprocess.CommandRunnerWithLogging(workerprocess.AdaptCommandRunner(effect), nil, platformclock.Real{})
}

type capturingCommandRunner struct {
	request executor.CommandRequest
}

func (r *capturingCommandRunner) Run(_ context.Context, request executor.CommandRequest) (executor.CommandResult, error) {
	r.request = request
	return executor.CommandResult{Stdout: []byte("ok")}, nil
}

func testScriptRequest(dispatch work.WorkDispatch, opts ...func(*workerexecution.WorkstationExecutionRequest)) workerexecution.WorkstationExecutionRequest {
	req := workerexecution.WorkstationExecutionRequest{
		Dispatch:           work.CloneWorkDispatch(dispatch),
		ProcessEnvironment: os.Environ(),
		WorkerType:         dispatch.WorkerType,
		ProjectID:          dispatch.ProjectID,
		InputTokens:        append([]any(nil), dispatch.InputTokens...),
	}
	for _, opt := range opts {
		opt(&req)
	}
	return req
}

func echoCommand(msg string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "echo " + msg}
	}
	return "echo", []string{msg}
}

func failCommand(msg string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "echo " + msg + " 1>&2 && exit 1"}
	}
	return "sh", []string{"-c", "echo '" + msg + "' >&2; exit 1"}
}

func TestScriptExecutor_SuccessfulEcho_PopulatesOutput(t *testing.T) {
	cmd, args := echoCommand("hello world")
	executor := &executor.ScriptExecutor{Command: cmd, Args: args, Now: time.Now, CommandRunner: testScriptCommandRunner(t)}

	result, err := executor.Execute(context.Background(), testScriptRequest(work.WorkDispatch{DispatchID: "d-1", TransitionID: "t-1"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if !strings.Contains(result.Output, "hello world") {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestScriptExecutor_FailingCommand_ReturnsFailedResult(t *testing.T) {
	cmd, args := failCommand("something went wrong")
	executor := &executor.ScriptExecutor{Command: cmd, Args: args, Now: time.Now, CommandRunner: testScriptCommandRunner(t)}

	result, err := executor.Execute(context.Background(), testScriptRequest(work.WorkDispatch{DispatchID: "d-1", TransitionID: "t-2"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if !strings.Contains(result.Error, "something went wrong") {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestScriptExecutor_ResolvesInstalledScriptCommandAgainstFactoryDirectory(t *testing.T) {
	factoryDir := t.TempDir()
	runner := &capturingCommandRunner{}
	scriptExecutor := executor.NewScriptExecutorWithRunner(
		&workerconfig.FactoryWorkerConfig{
			Command: "scripts/setup-workspace.py",
			Args:    []string{"factory/scripts/nested/helper.sh"},
		},
		runner,
		nil,
		factoryDir,
		nil,
		time.Now,
	)

	result, err := scriptExecutor.Execute(context.Background(), testScriptRequest(work.WorkDispatch{
		DispatchID:   "dispatch-installed-script",
		TransitionID: "transition-installed-script",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if want := filepath.Join(factoryDir, "scripts", "setup-workspace.py"); runner.request.Command != want {
		t.Fatalf("command = %q, want %q", runner.request.Command, want)
	}
	if want := filepath.Join(factoryDir, "scripts", "nested", "helper.sh"); len(runner.request.Args) != 1 || runner.request.Args[0] != want {
		t.Fatalf("args = %#v, want [%q]", runner.request.Args, want)
	}
}
