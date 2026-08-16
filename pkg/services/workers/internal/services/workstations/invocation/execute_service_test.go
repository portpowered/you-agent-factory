package invocation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerinvocation "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/invocation"
)

func TestExecuteServiceAdapterMapsInvocationToCanonicalRequest(t *testing.T) {
	continuation := (&providers.SessionMetadata{
		Provider: "codex",
		Kind:     "thread",
		ID:       "session-1",
	}).ContinuationRef()
	service := &executeServiceTestDouble{
		result: workerexecution.ExecuteResult{
			Outcome: workerexecution.ExecutionOutcomeAccepted,
			Output: workerexecution.ProposedOutput{
				Primary:        []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "output"}},
				Feedback:       "ready",
				Classification: "accepted",
			},
			Continuation: continuation,
			Diagnostics: &workerexecution.SafeDiagnostics{
				Metadata: map[string]string{"source": "execute"},
			},
		},
	}
	publisher := func(workerexecution.ProgressFragment) {}
	executor := workerinvocation.NewExecuteServiceWithProgress(service, publisher)

	request := workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:  "dispatch-1",
			WorkerType:  "agent",
			ProjectID:   "project-1",
			InputTokens: []any{"token"},
			Execution: work.ExecutionMetadata{
				RequestID: "request-1",
				TraceID:   "trace-1",
			},
		},
		Correlation: workerexecution.ExecutionCorrelation{
			FactorySessionID: "session-1",
			RuntimeID:        "runtime-1",
			GenerationID:     "generation-1",
			DispatchID:       "dispatch-1",
			AttemptID:        "attempt-1",
			RequestID:        "request-1",
			TraceID:          "trace-1",
		},
		WorkerName:       "agent-worker",
		WorkerType:       "agent",
		WorkstationType:  "agent-run",
		RunnerID:         "codex",
		ExecutorProvider: "ACP",
		Model:            "gpt",
		ModelProvider:    "codex",
		ReasoningEffort:  "high",
		UserMessage:      "finish the task",
		OutputSchema:     `{"type":"object"}`,
		OutputContract:   "decision",
		OutputFormat:     "json",
		StopToken:        "<COMPLETE>",
		DecisionEnvelope: true,
		EnvVars:          map[string]string{"MODE": "test"},
		WorkingDirectory: "C:/workspace",
		Worktree:         "C:/workspace/.worktree",
		PrintTimeout:     3,
		SkipPermissions:  true,
		Continuation:     continuation,
		WorkflowContext:  &workerexecution.Context{SessionID: "session-1"},
	}
	result, err := executor.Execute(context.Background(), workerexecution.InvocationInput{
		Request: request,
		Attempt: 2,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if service.calls != 1 {
		t.Fatalf("Execute() calls = %d, want one", service.calls)
	}
	if got := service.request.Correlation; got != request.Correlation {
		t.Fatalf("canonical correlation = %#v, want %#v", got, request.Correlation)
	}
	if got := service.request.Target; got.RunnerID != "codex" ||
		got.Provider.ID != "codex" || got.ExecutorProvider != "ACP" ||
		got.Prompt.UserMessage != "finish the task" || got.Output.Contract != "decision" ||
		got.Environment.Vars["MODE"] != "test" || got.Workspace.Worktree != request.Worktree ||
		!got.Permissions.SkipPermissions {
		t.Fatalf("canonical target = %#v, want mapped invocation target", got)
	}
	if service.request.Input.ProgressPublisher == nil ||
		service.request.Input.Resume == continuation ||
		service.request.Input.Resume.ProviderSessionID != continuation.ProviderSessionID {
		t.Fatalf("canonical input = %#v, want detached continuation and progress", service.request.Input)
	}
	if result.Attempt != 2 || result.Response.Content != "output" ||
		result.Response.Feedback != "ready" || result.Response.Classification != "accepted" ||
		result.Response.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("invocation result = %#v, want mapped accepted result", result)
	}
	if result.Continuation == continuation || result.Response.Continuation == continuation {
		t.Fatal("invocation result shares canonical continuation state")
	}
	if result.Response.Diagnostics == nil || result.Response.Diagnostics.Metadata["source"] != "execute" {
		t.Fatalf("invocation diagnostics = %#v, want Execute metadata", result.Response.Diagnostics)
	}
}

func TestExecuteServiceAdapterPreservesCanonicalFailureClassification(t *testing.T) {
	service := &executeServiceTestDouble{
		result: workerexecution.ExecuteResult{
			Outcome: workerexecution.ExecutionOutcomeFailed,
			Failure: &workerexecution.ExecutionFailure{
				Family: workerexecution.WorkFailureFamilyRetryable,
				Type:   workerexecution.WorkFailureTypeThrottled,
				Detail: &workerexecution.FailureDetail{
					Reason:  workerexecution.WorkFailureTypeThrottled,
					Message: "retry later",
				},
			},
		},
	}
	executor := workerinvocation.NewExecuteService(service)
	result, err := executor.Execute(context.Background(), workerexecution.InvocationInput{
		Request: workerexecution.ProviderInferenceRequest{RunnerID: "codex"},
		Attempt: 4,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Attempt != 4 || result.Response.Outcome != workerexecution.OutcomeFailed ||
		result.FailureMetadata == nil || result.FailureMetadata.Type != workerexecution.WorkFailureTypeThrottled ||
		result.FailureDecision == nil || !result.FailureDecision.Retryable ||
		result.FailureDetail == nil || result.FailureDetail.Message != "retry later" {
		t.Fatalf("failure result = %#v, want preserved retryable classification", result)
	}
}

func TestExecuteServiceAdapterReturnsPreStartErrorAsInvocationFailure(t *testing.T) {
	preStart := errors.New("invalid detached request")
	executor := workerinvocation.NewExecuteService(&executeServiceTestDouble{err: preStart})
	result, err := executor.Execute(context.Background(), workerexecution.InvocationInput{Attempt: 3})
	if !errors.Is(err, preStart) || result.Attempt != 3 || result.FailureDetail == nil {
		t.Fatalf("pre-start result = %#v, error %v, want original error and failure detail", result, err)
	}
}

type executeServiceTestDouble struct {
	request workerexecution.ExecuteRequest
	result  workerexecution.ExecuteResult
	err     error
	calls   int
}

func (service *executeServiceTestDouble) Execute(
	_ context.Context,
	request workerexecution.ExecuteRequest,
) (workerexecution.ExecuteResult, error) {
	service.calls++
	service.request = request
	return service.result, service.err
}
