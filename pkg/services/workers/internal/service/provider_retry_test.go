package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func TestExecuteProviderWithRetryCarriesSessionAndBoundsAttempts(t *testing.T) {
	t.Parallel()

	service := &Service{}
	request := workers.RunnerExecutionRequest{}
	var attempts []workers.RunnerExecutionRequest
	providerErr := workers.NewProviderError(
		workers.WorkFailureTypeInternalServerError,
		"provider temporarily unavailable",
		nil,
	)
	providerErr.Continuation = (&providers.SessionMetadata{Provider: "codex", ID: "provider-session-1"}).ContinuationRef()

	result, err := service.executeProviderWithRetry(
		context.Background(),
		request,
		func(attempt workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts = append(attempts, attempt)
			if len(attempts) == 1 {
				return workers.RunnerExecutionResult{}, providerErr
			}
			return workers.RunnerExecutionResult{Content: "accepted"}, nil
		},
	)
	if err != nil {
		t.Fatalf("executeProviderWithRetry() error = %v", err)
	}
	if result.Content != "accepted" || len(attempts) != 2 {
		t.Fatalf("result = %#v, attempts = %d, want accepted after two attempts", result, len(attempts))
	}
	if attempts[1].SessionID != "provider-session-1" {
		t.Fatalf("retry session = %q, want provider-session-1", attempts[1].SessionID)
	}
	if len(attempts[1].RequiredOptionalCapabilities) != 1 ||
		attempts[1].RequiredOptionalCapabilities[0] != workers.RunnerOptionalCapabilitySessionResume {
		t.Fatalf("retry capabilities = %#v, want session resume", attempts[1].RequiredOptionalCapabilities)
	}
}

func TestExecuteProviderWithRetryStopsAtMaximumAndHonorsCancellation(t *testing.T) {
	t.Parallel()

	service := &Service{}
	request := workers.RunnerExecutionRequest{}
	providerErr := workers.NewProviderError(
		workers.WorkFailureTypeInternalServerError,
		"provider temporarily unavailable",
		nil,
	)
	var attempts int
	_, err := service.executeProviderWithRetry(
		context.Background(),
		request,
		func(workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts++
			return workers.RunnerExecutionResult{}, providerErr
		},
	)
	if !errors.Is(err, providerErr) || attempts != detachedProviderMaxRetries+1 {
		t.Fatalf("bounded retry = (%v, %d attempts), want original error and %d attempts", err, attempts, detachedProviderMaxRetries+1)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	attempts = 0
	_, err = service.executeProviderWithRetry(
		canceled,
		request,
		func(workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts++
			return workers.RunnerExecutionResult{}, providerErr
		},
	)
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("canceled retry = (%v, %d attempts), want context.Canceled after one attempt", err, attempts)
	}
}

func TestProviderContinuationForRetryPrefersErrorThenResult(t *testing.T) {
	t.Parallel()

	errorSession := (&providers.SessionMetadata{Provider: "codex", ID: "error-session"}).ContinuationRef()
	resultSession := (&providers.SessionMetadata{Provider: "codex", ID: "result-session"}).ContinuationRef()
	if got := providerContinuationForRetry(
		workers.NewProviderErrorWithSession(workers.WorkFailureTypeThrottled, "busy", nil, errorSession),
		workers.RunnerExecutionResult{Continuation: resultSession},
	); got == nil || got.ProviderSessionID != "error-session" {
		t.Fatalf("providerContinuationForRetry(error, result) = %#v, want error continuation", got)
	}
	if got := providerContinuationForRetry(nil, workers.RunnerExecutionResult{Continuation: resultSession}); got == nil || got.ProviderSessionID != "result-session" {
		t.Fatalf("providerContinuationForRetry(nil, result) = %#v, want result continuation", got)
	}
	if got := providerContinuationForRetry(nil, workers.RunnerExecutionResult{}); got != nil {
		t.Fatalf("providerContinuationForRetry(empty) = %#v, want nil", got)
	}
}

func TestNormalizeProviderOverrideResultInfersStopAndContinuationOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		result     workers.RunnerExecutionResult
		request    workers.RunnerExecutionRequest
		wantResult workers.WorkOutcome
	}{
		{
			name:       "existing outcome is preserved",
			result:     workers.RunnerExecutionResult{Outcome: workers.OutcomeAccepted, Content: "ignored"},
			request:    workers.RunnerExecutionRequest{StopToken: "DONE"},
			wantResult: workers.OutcomeAccepted,
		},
		{
			name:       "stop token accepts",
			result:     workers.RunnerExecutionResult{Content: "answer DONE"},
			request:    workers.RunnerExecutionRequest{StopToken: "DONE"},
			wantResult: workers.OutcomeAccepted,
		},
		{
			name:       "continue marker continues",
			result:     workers.RunnerExecutionResult{Content: "<CONTINUE>"},
			request:    workers.RunnerExecutionRequest{StopToken: "DONE"},
			wantResult: workers.OutcomeContinue,
		},
		{
			name:       "missing marker rejects",
			result:     workers.RunnerExecutionResult{Content: "not complete"},
			request:    workers.RunnerExecutionRequest{StopToken: "DONE"},
			wantResult: workers.OutcomeRejected,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeProviderOverrideResult(test.result, test.request)
			if got.Outcome != test.wantResult {
				t.Fatalf("outcome = %q, want %q", got.Outcome, test.wantResult)
			}
		})
	}
}

func TestHasProviderCompletionEvidenceAcceptsProviderMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		result       workers.RunnerExecutionResult
		wantEvidence bool
	}{
		{name: "empty content", result: workers.RunnerExecutionResult{}, wantEvidence: false},
		{
			name: "worker metadata",
			result: workers.RunnerExecutionResult{
				Content: "answer",
				Diagnostics: &workers.WorkDiagnostics{Metadata: map[string]string{
					workers.ProviderResponseMetadataCompletionEvidence: "agent_message",
				}},
			},
			wantEvidence: true,
		},
		{
			name: "provider metadata",
			result: workers.RunnerExecutionResult{
				Content: "answer",
				Diagnostics: &workers.WorkDiagnostics{Provider: &workers.ProviderDiagnostic{
					ResponseMetadata: map[string]string{
						workers.ProviderResponseMetadataCompletionEvidence: "provider_response",
					},
				}},
			},
			wantEvidence: true,
		},
		{
			name: "unrelated metadata",
			result: workers.RunnerExecutionResult{
				Content:     "answer",
				Diagnostics: &workers.WorkDiagnostics{Metadata: map[string]string{"other": "value"}},
			},
			wantEvidence: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasProviderCompletionEvidence(test.result); got != test.wantEvidence {
				t.Fatalf("hasProviderCompletionEvidence() = %t, want %t", got, test.wantEvidence)
			}
		})
	}
}

func TestAdaptRunnerRequestPreservesDetachedInputIdentityAndDispatchFacts(t *testing.T) {
	t.Parallel()

	got := adaptRunnerRequest(detachedAdaptRequest(), runners.ScriptIdentity, nil)
	token := onlyAdaptedInputToken(t, got)
	assertAdaptedTokenIdentity(t, token)
	assertAdaptedTokenProductFacts(t, token)
	assertAdaptedTokenHistory(t, token)
	assertAdaptedDispatchFacts(t, got)
	assertAdaptedExecutionPolicy(t, got)
}

func detachedAdaptRequest() workers.ExecuteRequest {
	original := workers.Token{
		ID:    "token-1",
		State: "draft",
		Color: workers.Color{
			Name:                     "source-name",
			RequestID:                "request-1",
			WorkID:                   "work-1",
			WorkTypeID:               "task",
			DataType:                 workers.DataTypeWork,
			ChainingTraceDepth:       2,
			CurrentChainingTraceID:   "input-chain",
			PreviousChainingTraceIDs: []string{"input-previous"},
			TraceID:                  "old-trace",
			ParentID:                 "old-parent",
			Tags:                     map[string]string{"old": "tag"},
			Relations:                []work.Relation{{Type: work.RelationDependsOn, TargetWorkID: "work-0"}},
			Content:                  []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "old-content"}},
			Payload:                  []byte("old-payload"),
			StructuredResult:         map[string]any{"answer": "old"},
			StructuredResultPresent:  true,
			InvocationArguments:      &work.InvocationArguments{},
		},
		CreatedAt: time.Unix(10, 0), EnteredAt: time.Unix(11, 0),
		History: workers.History{
			TotalVisits: map[string]int{"transition-1": 2},
			LastError:   "old-error",
			FailureLog: []workers.Failure{{
				TransitionID: "transition-1",
				Error:        "old-error",
				Attempt:      2,
			}},
		},
	}
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-1", RuntimeID: "runtime-1",
			GenerationID: "generation-1",
			DispatchID:   "dispatch-1",
			RequestID:    "request-2",
			TraceID:      "trace-2",
		},
		Target: workers.ExecutionTarget{
			WorkerName:      "worker",
			WorkerType:      "worker-type",
			WorkstationName: "station",
			RunnerID:        runners.ScriptIdentity,
			Command:         "run-worker",
			Args:            []string{"--detached"},
			Environment: workers.EnvironmentPolicy{
				Vars:               map[string]string{"WORKER_ENV": "set"},
				ProcessEnvironment: []string{"PROCESS_ENV=set"},
				WorkingDirectory:   "working-directory",
			},
			Workspace: workers.WorkspacePolicy{Worktree: "worktree-1"},
		},
		Input: workers.ExecutionInput{
			Dispatch: work.WorkDispatch{
				DispatchID:               "dispatch-1",
				TransitionID:             "transition-1",
				WorkerType:               "dispatch-worker",
				WorkstationName:          "dispatch-station",
				ProjectID:                "project-1",
				CurrentChainingTraceID:   "dispatch-chain",
				PreviousChainingTraceIDs: []string{"dispatch-previous"},
				Execution: work.ExecutionMetadata{
					RequestID: "request-1",
					TraceID:   "trace-1",
					WorkIDs:   []string{"work-1"},
				},
				InputTokens: workers.InputTokens(original),
				InputBindings: map[string][]string{
					"input": {"token-1"},
				},
			},
			Work: []workers.WorkInput{{
				Kind:       string(workers.DataTypeWork),
				State:      "review",
				InputNames: []string{"input"},
				WorkID:     "work-1",
				Name:       "detached-name",
				WorkTypeID: "task",
				RequestID:  "request-2",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "new-content",
				}},
				Tags:      map[string]string{"new": "tag"},
				Relations: []work.Relation{{Type: work.RelationParentChild, TargetWorkID: "work-2"}},
				Lineage: workers.WorkLineage{
					ParentWorkID: "new-parent",
					TraceID:      "new-trace",
					OriginRef:    "detached-name",
				},
				AttemptFacts: workers.AttemptFacts{AttemptNumber: 3, LastFailure: "new-failure"},
			}},
		},
	}
}

func onlyAdaptedInputToken(t *testing.T, request workers.RunnerExecutionRequest) workers.Token {
	t.Helper()
	tokens := workers.WorkDispatchInputTokens(request.Dispatch)
	if len(tokens) != 1 {
		t.Fatalf("adapted input tokens = %#v, want one token", tokens)
	}
	return tokens[0]
}

func assertAdaptedTokenIdentity(t *testing.T, token workers.Token) {
	t.Helper()
	if token.ID != "token-1" || token.State != "review" {
		t.Fatalf("adapted token identity/state = (%q, %q), want (token-1, review)", token.ID, token.State)
	}
	if token.Color.Name != "detached-name" || token.Color.RequestID != "request-2" ||
		token.Color.TraceID != "new-trace" || token.Color.ParentID != "new-parent" {
		t.Fatalf("adapted token identity facts = %#v", token.Color)
	}
}

func assertAdaptedTokenProductFacts(t *testing.T, token workers.Token) {
	t.Helper()
	if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "new-content" ||
		string(token.Color.Payload) != "new-content" {
		t.Fatalf("adapted token content/payload = %#v/%q", token.Color.Content, token.Color.Payload)
	}
	if token.Color.Tags["new"] != "tag" || len(token.Color.Relations) != 1 ||
		token.Color.Relations[0].TargetWorkID != "work-2" {
		t.Fatalf("adapted token metadata = %#v", token.Color)
	}
	if token.Color.ChainingTraceDepth != 2 || token.Color.CurrentChainingTraceID != "input-chain" ||
		len(token.Color.PreviousChainingTraceIDs) != 1 || token.Color.PreviousChainingTraceIDs[0] != "input-previous" {
		t.Fatalf("adapted token chaining facts = %#v", token.Color)
	}
}

func assertAdaptedTokenHistory(t *testing.T, token workers.Token) {
	t.Helper()
	if token.History.TotalVisits["transition-1"] != 2 || token.History.LastError != "old-error" ||
		len(token.History.FailureLog) != 1 {
		t.Fatalf("adapted token history = %#v", token.History)
	}
	result, ok := token.Color.StructuredResult.(map[string]any)
	if !ok || result["answer"] != "old" || !token.Color.StructuredResultPresent {
		t.Fatalf("adapted structured result = %#v (present=%t)", token.Color.StructuredResult, token.Color.StructuredResultPresent)
	}
}

func assertAdaptedDispatchFacts(t *testing.T, request workers.RunnerExecutionRequest) {
	t.Helper()
	if request.Dispatch.CurrentChainingTraceID != "dispatch-chain" ||
		len(request.Dispatch.PreviousChainingTraceIDs) != 1 ||
		request.Dispatch.PreviousChainingTraceIDs[0] != "dispatch-previous" ||
		request.Dispatch.InputBindings["input"][0] != "token-1" {
		t.Fatalf("adapted dispatch facts = %#v", request.Dispatch)
	}
}

func assertAdaptedExecutionPolicy(t *testing.T, request workers.RunnerExecutionRequest) {
	t.Helper()
	if request.Command != "run-worker" || request.Args[0] != "--detached" ||
		request.EnvVars["WORKER_ENV"] != "set" || request.ProcessEnvironment[0] != "PROCESS_ENV=set" ||
		request.WorkingDirectory != "working-directory" || request.Worktree != "worktree-1" {
		t.Fatalf("adapted execution policy = %#v", request)
	}
}

func TestAdaptRunnerRequestProjectsInputNamesAndKindsWithoutOriginalTokens(t *testing.T) {
	t.Parallel()

	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-1",
			RuntimeID:        "runtime-1",
			GenerationID:     "generation-1",
			DispatchID:       "dispatch-1",
			RequestID:        "request-1",
			TraceID:          "trace-1",
		},
		Target: workers.ExecutionTarget{RunnerID: runners.ScriptIdentity},
		Input: workers.ExecutionInput{
			Work: []workers.WorkInput{
				{
					Kind:       string(workers.DataTypeWork),
					State:      "review",
					InputNames: []string{"primary"},
					WorkID:     "work-1",
					Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "work"}},
				},
				{
					Kind:       string(workers.DataTypeResource),
					State:      "ready",
					InputNames: []string{"capacity"},
					WorkID:     "resource-1",
					Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "resource"}},
				},
			},
		},
	}

	got := adaptRunnerRequest(request, runners.ScriptIdentity, nil)
	tokens := workers.WorkDispatchInputTokens(got.Dispatch)
	if len(tokens) != 2 || tokens[0].ID != "work-input-0" || tokens[1].ID != "work-input-1" {
		t.Fatalf("generated input identities = %#v, want stable detached identities", tokens)
	}
	if tokens[0].Color.DataType != workers.DataTypeWork || tokens[1].Color.DataType != workers.DataTypeResource ||
		tokens[0].State != "review" || tokens[1].State != "ready" {
		t.Fatalf("generated input facts = %#v, want ordered work/resource states", tokens)
	}
	if got.Dispatch.InputBindings["primary"][0] != "work-input-0" ||
		got.Dispatch.InputBindings["capacity"][0] != "work-input-1" {
		t.Fatalf("generated input bindings = %#v", got.Dispatch.InputBindings)
	}
}
