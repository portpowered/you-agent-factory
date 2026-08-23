package factorysessionexecution

import (
	"context"
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDirectChildExecutor_StructuredMismatchRetriesWithinPolicy(t *testing.T) {
	const diagnostic = "structured output schema violation: instance /answer; expected string"
	attempts := 0
	var requests []workers.ExecuteRequest
	involver := &recordingWorkerExecution{
		result: workers.ExecuteResult{
			Outcome: workers.ExecutionOutcomeFailed,
			Failure: &workers.ExecutionFailure{
				Type:    workers.WorkFailureTypeStructuredOutputSchemaViolation,
				Family:  workers.WorkFailureFamilyTerminal,
				Message: diagnostic,
				Detail: &workers.FailureDetail{
					Reason:  workers.WorkFailureTypeStructuredOutputSchemaViolation,
					Message: diagnostic,
				},
			},
		},
	}
	involver.onExecute = func(request workers.ExecuteRequest) {
		attempts++
		requests = append(requests, request)
		if attempts == 2 {
			involver.result = workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeAccepted,
				StructuredResult: map[string]any{
					"answer": "validated answer",
				},
				StructuredResultPresent: true,
			}
		}
	}
	sink := newChildRecordSink()
	service := &JavaScriptRuntimeService{
		projectRoot: "/project",
		childValues: childTestValues{},
	}
	service.SetDirectWorkerExecution(involver)
	policy := factory.DefaultJavaScriptPolicy()
	policy.MaxRetries = 1
	hooks := service.childExecutorHooks(ChildExecutorModeLive, "direct-structured-retry")
	executor := hooks.NewChildExecutor("direct-structured-retry", sink, policy)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:        "return a structured answer",
		ModelProvider: "codex",
		OutputSchema:  map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if attempts != 2 || len(requests) != 2 {
		t.Fatalf("attempts = %d, requests = %d, want two", attempts, len(requests))
	}
	if requests[0].Attempt.Number != 1 || requests[1].Attempt.Number != 2 ||
		requests[0].Correlation.AttemptID != "dispatch-1/attempt/1" ||
		requests[1].Correlation.AttemptID != "dispatch-1/attempt/2" {
		t.Fatalf("attempt requests = %#v, want numbered detached attempts", requests)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusCompleted || !result.SchemaValidated {
		t.Fatalf("child result = %#v, want completed schema-validated result", result)
	}
	if result.Output["answer"] != "validated answer" {
		t.Fatalf("child answer = %#v, want validated native output", result.Output["answer"])
	}
	terminal := sink.terminalChildDispatch(t)
	if terminal.Attempt != 2 || !terminal.SchemaValidated {
		t.Fatalf("terminal record = %#v, want final attempt 2 and validated metadata", terminal)
	}
	if len(sink.statuses) != 2 {
		t.Fatalf("dispatch statuses = %v, want one queued and one running record", sink.statuses)
	}
}

func TestDirectChildExecutor_ExhaustedStructuredMismatchFailsWithoutOutput(t *testing.T) {
	const diagnostic = "structured output schema violation: instance /answer; expected string"
	attempts := 0
	involver := &recordingWorkerExecution{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeStructuredOutputSchemaViolation,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: diagnostic,
			Detail: &workers.FailureDetail{
				Reason:  workers.WorkFailureTypeStructuredOutputSchemaViolation,
				Message: diagnostic,
			},
		},
	}}
	involver.onExecute = func(_ workers.ExecuteRequest) { attempts++ }
	sink := newChildRecordSink()
	executor := newDirectChildExecutor(
		"direct-structured-exhausted",
		involver,
		sink,
		childTestValues{},
		"/project",
		2,
	)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:        "do not expose invalid output",
		ModelProvider: "codex",
		OutputSchema:  map[string]any{"type": "object"},
	})
	if err == nil || !strings.Contains(err.Error(), "/answer") {
		t.Fatalf("Execute error = %v, want the safe schema path diagnostic", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want initial attempt plus two retries", attempts)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusFailed || len(result.Output) != 0 || result.SchemaValidated {
		t.Fatalf("child result = %#v, want failed with no output or validation metadata", result)
	}
	if strings.Contains(err.Error(), "do not expose invalid output") {
		t.Fatalf("Execute error = %q, must not expose the prompt", err)
	}
	terminal := sink.terminalChildDispatch(t)
	if terminal.Attempt != 3 || terminal.Output != nil || terminal.SchemaValidated {
		t.Fatalf("terminal record = %#v, want final failed attempt without output", terminal)
	}
}
