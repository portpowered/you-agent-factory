package script_test

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers/executor"
)

func testScriptRequest(dispatch interfaces.WorkDispatch, opts ...func(*interfaces.WorkstationExecutionRequest)) interfaces.WorkstationExecutionRequest {
	req := interfaces.WorkstationExecutionRequest{
		Dispatch:    interfaces.CloneWorkDispatch(dispatch),
		WorkerType:  dispatch.WorkerType,
		ProjectID:   dispatch.ProjectID,
		InputTokens: append([]any(nil), dispatch.InputTokens...),
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
	executor := &executor.ScriptExecutor{Command: cmd, Args: args}

	result, err := executor.Execute(context.Background(), testScriptRequest(interfaces.WorkDispatch{DispatchID: "d-1", TransitionID: "t-1"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if !strings.Contains(result.Output, "hello world") {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestScriptExecutor_FailingCommand_ReturnsFailedResult(t *testing.T) {
	cmd, args := failCommand("something went wrong")
	executor := &executor.ScriptExecutor{Command: cmd, Args: args}

	result, err := executor.Execute(context.Background(), testScriptRequest(interfaces.WorkDispatch{DispatchID: "d-1", TransitionID: "t-2"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if !strings.Contains(result.Error, "something went wrong") {
		t.Fatalf("Error = %q", result.Error)
	}
}
