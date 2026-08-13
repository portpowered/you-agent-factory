package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDetachedResultMaterializationPreservesFailureAndContinuationFacts(t *testing.T) {
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{
			Dispatch: work.WorkDispatch{
				DispatchID:   "logical-dispatch/attempt/2",
				TransitionID: "review",
			},
		},
	}
	result, err := workstationDispatchResultFromExecute(request, workers.ExecuteResult{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "logical-dispatch",
			AttemptID:  "logical-dispatch/attempt/2",
		},
		Outcome: workers.ExecutionOutcomeFailed,
		Output: workers.ProposedOutput{
			Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial output"}},
		},
		Failure: &workers.ExecutionFailure{
			Family:  workers.WorkFailureFamilyRetryable,
			Type:    workers.WorkFailureTypeTimeout,
			Detail:  &workers.FailureDetail{Reason: workers.WorkFailureTypeTimeout, Message: "provider timed out"},
			Message: "provider timed out",
		},
		Continuation: &workers.ProviderContinuationRef{
			Provider:          "codex",
			ProviderSessionID: "provider-session-retry",
		},
		Diagnostics: &workers.SafeDiagnostics{
			Provider: &workers.SafeProviderDiagnostic{
				Provider: "codex",
				Model:    "fixture-model",
				ResponseMetadata: map[string]string{
					"duration_ms": "12",
				},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("workstationDispatchResultFromExecute() error = %v", err)
	}
	if result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed ||
		result.Result.Outcome != workers.OutcomeFailed {
		t.Fatalf("terminal result = %#v, want failed", result)
	}
	if result.Result.DispatchID != request.Execution.Dispatch.DispatchID {
		t.Fatalf("physical result dispatch ID = %q, want %q", result.Result.DispatchID, request.Execution.Dispatch.DispatchID)
	}
	if result.Result.Error != "provider timed out" || result.Result.FailureMetadata == nil ||
		result.Result.FailureMetadata.Type != workers.WorkFailureTypeTimeout {
		t.Fatalf("failure facts = %#v, want timeout metadata", result.Result)
	}
	if result.Result.ProviderSession == nil || result.Result.ProviderSession.ID != "provider-session-retry" {
		t.Fatalf("provider session = %#v, want continuation identity", result.Result.ProviderSession)
	}
	if result.Result.Diagnostics == nil || result.Result.Diagnostics.Provider == nil ||
		result.Result.Diagnostics.Provider.ResponseMetadata["duration_ms"] != "12" {
		t.Fatalf("diagnostics = %#v, want safe provider metadata", result.Result.Diagnostics)
	}
}

func TestAttemptLifecycleRejectsConflictingWorkerCorrelation(t *testing.T) {
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		return workers.ExecuteResult{
			Correlation: workers.ExecutionCorrelation{DispatchID: "different-dispatch", AttemptID: request.Correlation.AttemptID},
			Outcome:     workers.ExecutionOutcomeAccepted,
		}, nil
	})
	lifecycle := newAttemptLifecycle(service, func() string { return "attempt-conflict" }, 1)
	var result workers.ExecuteResult
	if err := lifecycle.start(
		context.Background(), attemptTestRequest("dispatch-conflict", ""), false,
		func(_ context.Context, _ workers.ExecuteRequest, got workers.ExecuteResult, _ error) { result = got },
	); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil ||
		result.Failure.Message != "worker execution returned conflicting correlation" {
		t.Fatalf("conflicting result = %#v, want terminal correlation failure", result)
	}
}

func TestRuntimeExecutionSelectionMergesAuthoredWorkerWorkstationAndFactoryDefaults(t *testing.T) {
	lookup := runtimefixtures.RuntimeConfigLookupFixture{
		FactoryPath: "factory-dir",
		Factory:     &interfaces.FactoryConfig{Runner: "factory-runner"},
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"mock": {
				Name:             "mock",
				Type:             "agent",
				ExecutorProvider: "worker-provider",
				Model:            "worker-model",
				ModelProvider:    "worker-model-provider",
				Command:          "worker-command",
				Args:             []string{"--worker"},
				StopToken:        "worker-stop",
				SkipPermissions:  true,
				Timeout:          "3s",
				AgentTools:       &interfaces.AgentToolsConfig{Policy: "READ_ONLY"},
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {
				Name:             "review",
				WorkerTypeName:   "mock",
				Runner:           "workstation-runner",
				Body:             "authored system prompt",
				OutputSchema:     "authored-schema",
				OutputContract:   "decision",
				OutcomeFormat:    "json",
				Timeout:          "2s",
				WorkingDirectory: "workspace",
				Worktree:         "worktree",
				Env:              map[string]string{"STATION": "yes"},
			},
		},
	}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{
			WorkerType: "mock",
			EnvVars:    map[string]string{"OVERRIDE": "yes"},
		},
	}
	selection := resolveRuntimeExecutionSelection(
		&runtimeConfig{runtimeConfig: lookup},
		request,
		[]workers.WorkInput{{Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "input message",
		}}}},
	)

	if selection.workerName != "mock" || selection.workerType != "agent" ||
		selection.runnerID != "workstation-runner" || selection.providerID != "worker-provider" ||
		selection.model != "worker-model" || selection.modelProvider != "worker-model-provider" {
		t.Fatalf("authored worker selection = %#v", selection)
	}
	if selection.command != "worker-command" || len(selection.args) != 1 || selection.args[0] != "--worker" ||
		selection.stopToken != "worker-stop" || !selection.skipPermissions {
		t.Fatalf("worker execution policy = %#v", selection)
	}
	if selection.systemPrompt != "authored system prompt" || selection.outputSchema != "authored-schema" ||
		selection.outputContract != "decision" || selection.outputFormat != "json" ||
		selection.workingDirectory != "workspace" || selection.worktree != "worktree" {
		t.Fatalf("authored workstation selection = %#v", selection)
	}
	if selection.timeout != 3*time.Second || !selection.decisionEnvelope ||
		selection.toolExecutionMode != workers.RunnerToolExecutionModeRequired {
		t.Fatalf("execution policy defaults = %#v, want worker timeout/tools and decision envelope", selection)
	}
	if selection.environment["STATION"] != "yes" || selection.environment["OVERRIDE"] != "yes" {
		t.Fatalf("merged environment = %#v", selection.environment)
	}
	if selection.userMessage != "input message" {
		t.Fatalf("input-derived user message = %q, want input message", selection.userMessage)
	}
}

func TestContinuationFromLegacySessionKeepsProviderIdentity(t *testing.T) {
	if got := continuationFromLegacySession(nil); got != nil {
		t.Fatalf("nil legacy session = %#v, want nil", got)
	}
	ref := &providers.SessionRef{Provider: providers.IDCodex, ID: "session-1"}
	got := continuationFromLegacySession(ref)
	if got == nil || got.Provider != string(providers.IDCodex) ||
		got.ProviderSessionID != "session-1" || got.ExternalRef != "session-1" {
		t.Fatalf("legacy continuation = %#v, want provider session identity", got)
	}
}

func TestInvokeWorkerResultFromPreservesSessionOutcomeAndSafeDiagnostics(t *testing.T) {
	completed := invokeWorkerResultFrom(
		"dispatch-completed",
		"session-completed",
		workersessions.InvokeSessionResult{
			Session: workersessions.Session{ID: "session-completed", State: workersessions.StateCompleted},
			Dispatch: workers.WorkstationDispatchResult{Result: workers.WorkResult{
				Output: "completed output",
				ProviderSession: &workers.ProviderSessionMetadata{
					Provider: "codex",
					ID:       "provider-session-completed",
				},
			}},
			Attempts: 2,
		},
	)
	if completed.Outcome != factory.InvokeWorkerOutcomeCompleted || completed.Output != "completed output" ||
		completed.Attempts != 2 || completed.Provider != "codex" ||
		completed.ProviderSessionRef != "provider-session-completed" {
		t.Fatalf("completed InvokeWorker result = %#v", completed)
	}

	failed := invokeWorkerResultFrom(
		"dispatch-failed",
		"session-failed",
		workersessions.InvokeSessionResult{
			Session: workersessions.Session{
				ID:    "session-failed",
				State: workersessions.StateFailed,
				Result: &workersessions.TerminalResult{Cause: &workersessions.FailureCause{
					Kind:   workersessions.FailureCauseWorkersExecutionFailure,
					Detail: "provider timed out",
				}},
			},
			Dispatch: workers.WorkstationDispatchResult{Result: workers.WorkResult{
				FailureMetadata: &workers.WorkFailureMetadata{
					Family: workers.WorkFailureFamilyRetryable,
					Type:   workers.WorkFailureTypeTimeout,
				},
			}},
			Attempts: 1,
		},
	)
	if failed.Outcome != factory.InvokeWorkerOutcomeFailed || failed.Diagnostic != "provider timed out" ||
		failed.FailureReason != string(workers.WorkFailureTypeTimeout) || failed.Retryable == nil || !*failed.Retryable {
		t.Fatalf("failed InvokeWorker result = %#v, want retryable timeout", failed)
	}

	withoutCause := invokeWorkerResultFrom(
		"dispatch-no-cause",
		"session-no-cause",
		workersessions.InvokeSessionResult{
			Session:  workersessions.Session{ID: "session-no-cause", State: workersessions.StateFailed},
			Dispatch: workers.WorkstationDispatchResult{Result: workers.WorkResult{Error: "adapter failed"}},
		},
	)
	if withoutCause.Diagnostic != "adapter failed" {
		t.Fatalf("failure without cause diagnostic = %q, want adapter error", withoutCause.Diagnostic)
	}

	withoutDetails := invokeWorkerResultFrom(
		"dispatch-no-details",
		"session-no-details",
		workersessions.InvokeSessionResult{
			Session: workersessions.Session{ID: "session-no-details", State: workersessions.StateFailed},
		},
	)
	if withoutDetails.Diagnostic != "Provider execution failed." {
		t.Fatalf("failure without details diagnostic = %q, want bounded fallback", withoutDetails.Diagnostic)
	}
}
