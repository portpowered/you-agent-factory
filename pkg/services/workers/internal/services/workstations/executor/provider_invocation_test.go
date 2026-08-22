package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewProviderInvocationExecutorAbsentInvocationYieldsNoExecutor(t *testing.T) {
	if executor := NewProviderInvocationExecutor(nil); executor != nil {
		t.Fatalf("NewProviderInvocationExecutor(nil) = %#v, want nil", executor)
	}
}

func TestProviderInvocationExecutorResolvesEverySelectionFromRequest(t *testing.T) {
	invocation := &recordingInvocation{result: workerexecution.InvocationResult{
		Response: workerexecution.InferenceResponse{Content: `{"text":"done"}`},
	}}
	executor := NewProviderInvocationExecutor(invocation)

	result, err := executor.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch:         work.WorkDispatch{DispatchID: "dispatch-1", TransitionID: "t-1"},
		WorkerType:       "worker-a",
		ExecutorProvider: "codex",
		ModelProvider:    "codex",
		Model:            "codex-test-model",
		ReasoningEffort:  "high",
		SystemPrompt:     "be brief",
		UserMessage:      "summarize",
		OutputSchema:     `{"type":"object"}`,
		WorkingDirectory: "/project",
		SkipPermissions:  true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted || result.Output != `{"text":"done"}` {
		t.Fatalf("result = %#v, want accepted provider content", result)
	}

	request := invocation.input.Request
	if request.WorkerType != "worker-a" || request.Model != "codex-test-model" || request.ModelProvider != "codex" {
		t.Fatalf("selection = %#v, want caller's worker/model/provider", request)
	}
	if request.ReasoningEffort != "high" || request.SystemPrompt != "be brief" || request.UserMessage != "summarize" {
		t.Fatalf("prompt selections did not survive: %#v", request)
	}
	if !request.SkipPermissions || request.RunnerID != "codex" {
		t.Fatalf("resolved request = %#v, want skip-permissions and codex runner", request)
	}
	if invocation.input.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1; attempt budgeting belongs to Worker Sessions", invocation.input.Attempt)
	}
}

func TestProviderInvocationExecutorPreservesWorkersFailureClassification(t *testing.T) {
	metadata := &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyRetryable,
		Type:   workerexecution.WorkFailureTypeInternalServerError,
	}
	invocation := &recordingInvocation{
		err: errors.New("provider execution failed"),
		result: workerexecution.InvocationResult{
			FailureMetadata: metadata,
			FailureDetail: &workerexecution.FailureDetail{
				Reason:  workerexecution.WorkFailureTypeInternalServerError,
				Message: "provider returned 500",
			},
		},
	}
	executor := NewProviderInvocationExecutor(invocation)

	result, err := executor.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-1", TransitionID: "t-1"},
	})
	if err == nil || result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Execute() = %#v, %v, want failed provider result", result, err)
	}
	if result.FailureMetadata != metadata || result.DispatchID != "dispatch-1" || result.TransitionID != "t-1" {
		t.Fatalf("failure = %#v, want Workers classification and dispatch identity", result)
	}
	if result.Error == "" {
		t.Fatal("failure error = empty, want a non-empty description")
	}
}

func TestProviderInvocationExecutorUnavailableInvocationFailsDispatch(t *testing.T) {
	var executor *ProviderInvocationExecutor
	result, err := executor.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
	})
	if err != nil {
		t.Fatalf("Execute error = %v, want the failure carried on the result", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed || result.DispatchID != "dispatch-1" {
		t.Fatalf("result = %#v, want a failed result for dispatch-1", result)
	}
}

func TestProviderInvocationExecutorExplicitRunnerWins(t *testing.T) {
	invocation := &recordingInvocation{}
	executor := NewProviderInvocationExecutor(invocation)

	if _, err := executor.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch:         work.WorkDispatch{DispatchID: "dispatch-1"},
		RunnerID:         "claude",
		ExecutorProvider: "codex",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := invocation.input.Request.RunnerID; got != "claude" {
		t.Fatalf("runner = %q, want the caller's explicit selection", got)
	}
}

type recordingInvocation struct {
	input  workerexecution.InvocationInput
	result workerexecution.InvocationResult
	err    error
}

var _ workerexecution.InvocationExecutor = (*recordingInvocation)(nil)

func (i *recordingInvocation) Execute(
	_ context.Context,
	input workerexecution.InvocationInput,
) (workerexecution.InvocationResult, error) {
	i.input = input
	return i.result, i.err
}
