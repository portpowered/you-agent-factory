package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type agentMockProvider struct {
	testutil.ProviderServiceAdapter
	response  workerexecution.InferenceResponse
	err       error
	responses []workerexecution.InferenceResponse
	errors    []error
	callCount int
	lastReq   workerexecution.ProviderInferenceRequest
}

type staticRuntimeConfig = runtimefixtures.RuntimeConfigLookupFixture

type decisionEnvelopeExecutorFixture struct {
	result workerexecution.WorkResult
}

func (decisionEnvelopeExecutorFixture) UsesDecisionEnvelopeOutcome(*workerconfig.FactoryWorkstationConfig) bool {
	return true
}

func (decisionEnvelopeExecutorFixture) UsesGoalRoutingDecisionEnvelope(*workerconfig.FactoryWorkstationConfig) bool {
	return false
}

func (fixture decisionEnvelopeExecutorFixture) WorkResultFromDecisionEnvelopeJSONOrFailed(string, string, string) workerexecution.WorkResult {
	return fixture.result
}

func (decisionEnvelopeExecutorFixture) WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(string, string, string) workerexecution.WorkResult {
	return workerexecution.WorkResult{}
}

func TestDecisionEnvelopeWorkResultMergesCompletionValidationDiagnostics(t *testing.T) {
	base := &workerexecution.WorkDiagnostics{Provider: &workerexecution.ProviderDiagnostic{
		ResponseMetadata: map[string]string{
			workerexecution.ProviderResponseMetadataFailureOperation: "provider_inference",
		},
	}}
	parsed := workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeFailed,
		Diagnostics: &workerexecution.WorkDiagnostics{Provider: &workerexecution.ProviderDiagnostic{
			ResponseMetadata: map[string]string{
				workerexecution.ProviderResponseMetadataFailureOperation:      "completion_validation",
				workerexecution.ProviderResponseMetadataFailureClassification: "missing_required_output",
			},
		}},
	}

	result := decisionEnvelopeWorkResult(
		decisionEnvelopeExecutorFixture{result: parsed},
		workerexecution.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{
			DispatchID:   "dispatch-incomplete",
			TransitionID: "transition-review",
		}},
		workerexecution.InferenceResponse{Content: "<COMPLETE>"},
		base,
		0,
		time.Now(),
		time.Now,
	)

	if result.Diagnostics == nil || result.Diagnostics.Provider == nil {
		t.Fatalf("Diagnostics = %#v, want merged diagnostics", result.Diagnostics)
	}
	metadata := result.Diagnostics.Provider.ResponseMetadata
	if metadata[workerexecution.ProviderResponseMetadataFailureOperation] != "completion_validation" ||
		metadata[workerexecution.ProviderResponseMetadataFailureClassification] != "missing_required_output" {
		t.Fatalf("completion diagnostics = %#v, want parser facts preserved over base diagnostics", metadata)
	}
}

func TestAgentExecutor_UsesInjectedClockForWorkMetrics(t *testing.T) {
	times := []time.Time{
		time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 20, 12, 0, 7, 0, time.UTC),
	}
	clock := func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}
	executor := NewAgentExecutor(
		staticRuntimeConfig{},
		&agentMockProvider{},
		nil,
		clock,
	)

	result, err := executor.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		WorkerType: "missing-worker",
		Dispatch: work.WorkDispatch{
			DispatchID:   "dispatch-clock",
			TransitionID: "transition-clock",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.Metrics.Duration, 7*time.Second; got != want {
		t.Fatalf("Metrics.Duration = %s, want %s", got, want)
	}
}

func TestEffectiveWorkerDefinitionClearsEmptyResolvedPlaceholders(t *testing.T) {
	worker := &workerconfig.FactoryWorkerConfig{
		Model:            "${model}",
		ModelProvider:    "${provider}",
		ReasoningEffort:  "${effort}",
		ExecutorProvider: "${runner}",
	}
	effective := effectiveWorkerDefinition(workerexecution.WorkstationExecutionRequest{}, worker)
	if effective.Model != "" ||
		effective.ModelProvider != "" ||
		effective.ReasoningEffort != "" ||
		effective.ExecutorProvider != "" {
		t.Fatalf(
			"effective model/provider/effort/runner = %q/%q/%q/%q, want empty resolved placeholders",
			effective.Model,
			effective.ModelProvider,
			effective.ReasoningEffort,
			effective.ExecutorProvider,
		)
	}
	if worker.ReasoningEffort != "${effort}" {
		t.Fatalf("authored worker mutated: %#v", worker)
	}
}

func TestAgentExecutor_PreservesProviderContinuationClassification(t *testing.T) {
	providerErr := workerexecution.NewProviderError(
		workerexecution.WorkFailureTypePermanentBadRequest,
		"provider session continuation was rejected",
		providers.ContinuationFailure{Kind: providers.ContinuationFailureKindStale},
	)
	providerErr.ProviderContinuationFailureKind = providers.ContinuationFailureKindStale
	provider := &agentMockProvider{err: providerErr}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model", ModelProvider: string(modelprovider.ProviderCodex)},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "dispatch-continuation", WorkerType: "worker-a", WorkstationName: "review"},
		withAgentPrompts("system", "user"),
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed ||
		result.ProviderContinuationFailureKind != providers.ContinuationFailureKindStale ||
		result.ProviderFailureKind != "" || result.ProviderContinuationOutcome != "" {
		t.Fatalf("Execute() result = %#v, want a stale continuation failure", result)
	}
	if provider.callCount != 1 {
		t.Fatalf("provider calls = %d, want one terminal continuation attempt", provider.callCount)
	}
}

func (m *agentMockProvider) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	m.lastReq = req
	m.callCount++
	if idx := m.callCount - 1; idx < len(m.responses) || idx < len(m.errors) {
		var response workerexecution.InferenceResponse
		if idx < len(m.responses) {
			response = m.responses[idx]
		}
		var err error
		if idx < len(m.errors) {
			err = m.errors[idx]
		}
		return authoritativeTestResponse(response), err
	}
	return authoritativeTestResponse(m.response), m.err
}

func (m *agentMockProvider) ResolveIdentity(
	ctx context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	if request.Identity == "" {
		request.Identity = "codex"
	}
	return m.ProviderServiceAdapter.ResolveIdentity(ctx, request)
}

func (m *agentMockProvider) ValidatePrerequisites(
	ctx context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	if request.ID == "" {
		request.ID = providers.IDCodex
	}
	return m.ProviderServiceAdapter.ValidatePrerequisites(ctx, request)
}

func (m *agentMockProvider) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	adapter := m.ProviderServiceAdapter
	adapter.InferFunc = m.Infer
	return adapter.Execute(ctx, request)
}

func (m *agentMockProvider) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	adapter := m.ProviderServiceAdapter
	adapter.InferFunc = m.Infer
	return adapter.Continue(ctx, request)
}

func (m *agentMockProvider) ContinueReference(
	ctx context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return providers.ContinueReferenceResult{}, providers.ContinuationFailure{
			Kind:      providers.ContinuationFailureKindInvalid,
			Message:   err.Error(),
			Reference: providers.SessionRef{},
		}
	}
	continued, err := m.Continue(ctx, providers.ContinueRequest{
		Reference: reference,
		Attempt:   request.Attempt,
	})
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	continuedReference := continued.Reference
	if continuedReference.Provider == "" {
		continuedReference = reference
	}
	resultReference := continuedReference.ContinuationRef()
	resultReference.ExternalRef = request.Reference.Normalize().ExternalRef
	return providers.ContinueReferenceResult{
		Reference: resultReference,
		Outcome:   continued.Outcome,
		Result:    continued.Result,
	}, nil
}

func authoritativeTestResponse(response workerexecution.InferenceResponse) workerexecution.InferenceResponse {
	if response.Content == "" || response.Diagnostics != nil &&
		(response.Diagnostics.Metadata[workerexecution.ProviderResponseMetadataCompletionEvidence] != "" ||
			response.Diagnostics.Provider != nil && response.Diagnostics.Provider.ResponseMetadata[workerexecution.ProviderResponseMetadataCompletionEvidence] != "") {
		return response
	}
	response.Diagnostics = workerexecution.CloneWorkDiagnostics(response.Diagnostics)
	if response.Diagnostics == nil {
		response.Diagnostics = &workerexecution.WorkDiagnostics{}
	}
	if response.Diagnostics.Metadata == nil {
		response.Diagnostics.Metadata = make(map[string]string, 1)
	}
	response.Diagnostics.Metadata[workerexecution.ProviderResponseMetadataCompletionEvidence] = "provider_response"
	return response
}

func testAgentRequest(dispatch work.WorkDispatch, opts ...func(*workerexecution.WorkstationExecutionRequest)) workerexecution.WorkstationExecutionRequest {
	req := workerexecution.WorkstationExecutionRequest{
		Dispatch:        work.CloneWorkDispatch(dispatch),
		WorkerType:      dispatch.WorkerType,
		WorkstationType: dispatch.WorkstationName,
		ProjectID:       dispatch.ProjectID,
		InputTokens:     append([]any(nil), dispatch.InputTokens...),
	}
	for _, opt := range opts {
		opt(&req)
	}
	return req
}

func withAgentPrompts(systemPrompt, userMessage string) func(*workerexecution.WorkstationExecutionRequest) {
	return func(req *workerexecution.WorkstationExecutionRequest) {
		req.SystemPrompt = systemPrompt
		req.UserMessage = userMessage
	}
}

func withAgentOutputSchema(schema string) func(*workerexecution.WorkstationExecutionRequest) {
	return func(req *workerexecution.WorkstationExecutionRequest) {
		req.OutputSchema = schema
	}
}

func withAgentOutputContract(contract string) func(*workerexecution.WorkstationExecutionRequest) {
	return func(req *workerexecution.WorkstationExecutionRequest) {
		req.OutputContract = contract
	}
}

func withAgentEnvVars(envVars map[string]string) func(*workerexecution.WorkstationExecutionRequest) {
	return func(req *workerexecution.WorkstationExecutionRequest) {
		req.EnvVars = envVars
	}
}

func withAgentWorktree(worktree string) func(*workerexecution.WorkstationExecutionRequest) {
	return func(req *workerexecution.WorkstationExecutionRequest) {
		req.Worktree = worktree
	}
}

func withAgentWorkingDirectory(workingDirectory string) func(*workerexecution.WorkstationExecutionRequest) {
	return func(req *workerexecution.WorkstationExecutionRequest) {
		req.WorkingDirectory = workingDirectory
	}
}

func withAgentModelOperation(operation string, bindings []workerexecution.ResolvedModelOperationBinding) func(*workerexecution.WorkstationExecutionRequest) {
	return func(req *workerexecution.WorkstationExecutionRequest) {
		req.ModelOperation = operation
		req.ModelBindings = bindings
	}
}

func assertExecutionMetadataEqual(t *testing.T, want, got work.ExecutionMetadata) {
	t.Helper()
	if want.RequestID != got.RequestID {
		t.Fatalf("RequestID = %q, want %q", got.RequestID, want.RequestID)
	}
	if want.TraceID != got.TraceID {
		t.Fatalf("TraceID = %q, want %q", got.TraceID, want.TraceID)
	}
	if len(want.WorkIDs) != len(got.WorkIDs) {
		t.Fatalf("WorkIDs length = %d, want %d", len(got.WorkIDs), len(want.WorkIDs))
	}
	for i := range want.WorkIDs {
		if want.WorkIDs[i] != got.WorkIDs[i] {
			t.Fatalf("WorkIDs[%d] = %q, want %q", i, got.WorkIDs[i], want.WorkIDs[i])
		}
	}
}

func TestAgentExecutor_SuccessfulResponse_PopulatesOutput(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "The answer is 42."}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "claude-sonnet-4-20250514"},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("You are a helpful assistant.", "What is the meaning of life?"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if result.Output != "The answer is 42." {
		t.Fatalf("Output = %q, want %q", result.Output, "The answer is 42.")
	}
	if provider.lastReq.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("Model = %q, want %q", provider.lastReq.Model, "claude-sonnet-4-20250514")
	}
}

func TestAgentExecutor_AttachesProviderDiagnosticsToWorkResult(t *testing.T) {
	provider := &agentMockProvider{
		response: workerexecution.InferenceResponse{
			Content: "diagnosed response",
			Diagnostics: &workerexecution.WorkDiagnostics{
				Provider: &workerexecution.ProviderDiagnostic{
					ResponseMetadata: map[string]string{"request_id": "provider-request-1"},
				},
			},
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "claude-sonnet-4-20250514", ModelProvider: "claude"},
		},
	}, provider, nil, time.Now)
	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:      "d-1",
			TransitionID:    "t-1",
			WorkerType:      "worker-a",
			WorkstationName: "review",
		},
		withAgentPrompts("System prompt", "User prompt"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Diagnostics == nil || result.Diagnostics.Provider == nil {
		t.Fatal("expected provider diagnostics on work result")
	}
	if result.Diagnostics.RenderedPrompt == nil || result.Diagnostics.RenderedPrompt.UserMessageHash == "" {
		t.Fatal("expected rendered prompt hashes on work result")
	}
	if result.Diagnostics.Provider.Provider != "claude" {
		t.Fatalf("diagnostic provider = %q, want claude", result.Diagnostics.Provider.Provider)
	}
	if result.Diagnostics.Provider.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("diagnostic model = %q", result.Diagnostics.Provider.Model)
	}
	if result.Diagnostics.Provider.RequestMetadata["workstation_type"] != "review" {
		t.Fatalf("diagnostic workstation = %q, want review", result.Diagnostics.Provider.RequestMetadata["workstation_type"])
	}
	if result.Diagnostics.Provider.ResponseMetadata["request_id"] != "provider-request-1" {
		t.Fatalf("diagnostic response metadata = %#v", result.Diagnostics.Provider.ResponseMetadata)
	}
	if result.Diagnostics.Provider.ResponseMetadata["content_bytes"] == "" {
		t.Fatal("expected diagnostic response content size")
	}
}

func TestAgentExecutor_SuccessResponseDiagnosticsStayDetachedFromProviderMutation(t *testing.T) {
	responseDiagnostics := &workerexecution.WorkDiagnostics{
		Provider: &workerexecution.ProviderDiagnostic{
			ResponseMetadata: map[string]string{"request_id": "provider-request-1"},
		},
		Command: &workerexecution.CommandDiagnostic{
			Command: "provider-cli",
			Args:    []string{"--prompt", "story"},
			Env:     map[string]string{"API_KEY": "redacted"},
		},
		Panic: &workerexecution.PanicDiagnostic{
			Message: "panic message",
			Stack:   "panic stack",
		},
		Metadata: map[string]string{"phase": "initial"},
	}
	provider := &agentMockProvider{
		response: workerexecution.InferenceResponse{
			Content:     "diagnosed response",
			Diagnostics: responseDiagnostics,
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "claude-sonnet-4-20250514", ModelProvider: "claude"},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:      "d-1",
			TransitionID:    "t-1",
			WorkerType:      "worker-a",
			WorkstationName: "review",
		},
		withAgentPrompts("System prompt", "User prompt"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	responseDiagnostics.Provider.ResponseMetadata["request_id"] = "provider-request-mutated"
	responseDiagnostics.Command.Args[0] = "--mutated"
	responseDiagnostics.Command.Env["API_KEY"] = "mutated"
	responseDiagnostics.Panic.Message = "mutated panic"
	responseDiagnostics.Metadata["phase"] = "mutated"

	if got := result.Diagnostics.Provider.ResponseMetadata["request_id"]; got != "provider-request-1" {
		t.Fatalf("provider response metadata = %q, want provider-request-1", got)
	}
	if got := result.Diagnostics.Command.Args[0]; got != "--prompt" {
		t.Fatalf("command args[0] = %q, want --prompt", got)
	}
	if got := result.Diagnostics.Command.Env["API_KEY"]; got != "redacted" {
		t.Fatalf("command env API_KEY = %q, want redacted", got)
	}
	if got := result.Diagnostics.Panic.Message; got != "panic message" {
		t.Fatalf("panic message = %q, want panic message", got)
	}
	if got := result.Diagnostics.Metadata["phase"]; got != "initial" {
		t.Fatalf("metadata phase = %q, want initial", got)
	}
}

func TestAgentExecutor_ErrorDiagnosticsStayDetachedFromProviderMutation(t *testing.T) {
	errorDiagnostics := &workerexecution.WorkDiagnostics{
		Provider: &workerexecution.ProviderDiagnostic{
			ResponseMetadata: map[string]string{"request_id": "provider-request-1"},
		},
		Command: &workerexecution.CommandDiagnostic{
			Command: "provider-cli",
			Args:    []string{"--prompt", "story"},
			Env:     map[string]string{"API_KEY": "redacted"},
		},
		Panic: &workerexecution.PanicDiagnostic{
			Message: "panic message",
			Stack:   "panic stack",
		},
		Metadata: map[string]string{"phase": "initial"},
	}
	provider := &agentMockProvider{
		err: workerexecution.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "provider 500", nil),
	}
	var providerErr *ProviderError
	if !errors.As(provider.err, &providerErr) {
		t.Fatal("expected ProviderError test fixture")
	}
	providerErr.Diagnostics = errorDiagnostics

	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "claude-sonnet-4-20250514", ModelProvider: "claude"},
		},
	}, provider, nil, time.Now)
	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:      "d-1",
			TransitionID:    "t-1",
			WorkerType:      "worker-a",
			WorkstationName: "review",
		},
		withAgentPrompts("System prompt", "User prompt"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	errorDiagnostics.Provider.ResponseMetadata["request_id"] = "provider-request-mutated"
	errorDiagnostics.Command.Args[0] = "--mutated"
	errorDiagnostics.Command.Env["API_KEY"] = "mutated"
	errorDiagnostics.Panic.Message = "mutated panic"
	errorDiagnostics.Metadata["phase"] = "mutated"

	if got := result.Diagnostics.Provider.ResponseMetadata["request_id"]; got != "provider-request-1" {
		t.Fatalf("provider response metadata = %q, want provider-request-1", got)
	}
	if got := result.Diagnostics.Command.Args[0]; got != "--prompt" {
		t.Fatalf("command args[0] = %q, want --prompt", got)
	}
	if got := result.Diagnostics.Command.Env["API_KEY"]; got != "redacted" {
		t.Fatalf("command env API_KEY = %q, want redacted", got)
	}
	if got := result.Diagnostics.Panic.Message; got != "panic message" {
		t.Fatalf("panic message = %q, want panic message", got)
	}
	if got := result.Diagnostics.Metadata["phase"]; got != "initial" {
		t.Fatalf("metadata phase = %q, want initial", got)
	}
}

func TestAgentExecutor_PropagatesExecutionMetadataToProviderRequest(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "done"}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "claude-sonnet-4-20250514", ModelProvider: "claude"},
		},
	}, provider, nil, time.Now)

	want := work.ExecutionMetadata{
		DispatchCreatedTick: 7,
		CurrentTick:         8,
		TraceID:             "trace-1",
		WorkIDs:             []string{"work-1", "work-2"},
		ReplayKey:           "transition-1/trace-1/work-1/work-2",
	}
	_, err := executor.Execute(context.Background(), testAgentRequest(work.WorkDispatch{
		DispatchID:      "d-1",
		TransitionID:    "transition-1",
		WorkerType:      "worker-a",
		WorkstationName: "workstation-a",
		Execution:       want,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertExecutionMetadataEqual(t, want, provider.lastReq.Dispatch.Execution)
}

func TestAgentExecutor_ClaudeSessionIDFromRuntimeConfigFlowsIntoProviderRequest(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "The answer is 42."}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {
				Model:         "claude-sonnet-4-20250514",
				ModelProvider: string(modelprovider.ProviderClaude),
				SessionID:     "claude-session-123",
			},
		},
	}, provider, nil, time.Now)

	_, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("", "What is the meaning of life?"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.lastReq.SessionID != "claude-session-123" {
		t.Fatalf("provider request session id = %q, want %q", provider.lastReq.SessionID, "claude-session-123")
	}
}

func TestAgentExecutor_SuccessfulClaudeResponse_PreservesConfiguredSessionID(t *testing.T) {
	provider := &agentMockProvider{
		response: workerexecution.InferenceResponse{
			Content: "The answer is 42.",
			Continuation: providers.ContinuationFromSessionMetadata(&providers.SessionMetadata{
				Provider: string(modelprovider.ProviderClaude),
				Kind:     providerSessionKindSessionID,
				ID:       "claude-session-123",
			}),
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {
				Model:         "claude-sonnet-4-20250514",
				ModelProvider: string(modelprovider.ProviderClaude),
				SessionID:     "claude-session-123",
			},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("", "What is the meaning of life?"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if providers.SessionMetadataFromContinuation(result.Continuation) == nil {
		t.Fatal("expected provider session metadata on successful result")
	}
	if providers.SessionMetadataFromContinuation(result.Continuation).Provider != string(modelprovider.ProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", providers.SessionMetadataFromContinuation(result.Continuation).Provider, modelprovider.ProviderClaude)
	}
	if providers.SessionMetadataFromContinuation(result.Continuation).ID != "claude-session-123" {
		t.Fatalf("provider session id = %q, want %q", providers.SessionMetadataFromContinuation(result.Continuation).ID, "claude-session-123")
	}
}

func TestAgentExecutor_ProviderError_ReturnsFailedResult(t *testing.T) {
	provider := &agentMockProvider{err: errors.New("connection refused")}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Error != "provider error: connection refused" {
		t.Fatalf("Error = %q, want %q", result.Error, "provider error: connection refused")
	}
	if result.Metrics.RetryCount != 0 {
		t.Fatalf("RetryCount = %d, want 0", result.Metrics.RetryCount)
	}
	if result.Diagnostics == nil || result.Diagnostics.Provider == nil {
		t.Fatalf("Diagnostics = %#v, want structured provider failure diagnostics", result.Diagnostics)
	}
	metadata := result.Diagnostics.Provider.ResponseMetadata
	if _, exists := metadata["error"]; exists || metadata[workerexecution.ProviderResponseMetadataFailureOperation] != "provider_inference" {
		t.Fatalf("failure diagnostics = %#v, want no raw error and a stable operation", metadata)
	}
}

func TestAgentExecutor_EmptySuccessfulResponse_ReturnsMissingCompletionFailure(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "d-empty-completion", TransitionID: "t-empty-completion", WorkerType: "worker-a"},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.FailureMetadata == nil || result.FailureMetadata.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("FailureMetadata = %#v, want terminal unknown classification", result.FailureMetadata)
	}
	if result.Diagnostics == nil || result.Diagnostics.Provider == nil {
		t.Fatalf("Diagnostics = %#v, want completion-validation diagnostics", result.Diagnostics)
	}
	metadata := result.Diagnostics.Provider.ResponseMetadata
	if metadata[workerexecution.ProviderResponseMetadataFailureOperation] != "completion_validation" ||
		metadata[workerexecution.ProviderResponseMetadataFailureClassification] != "missing_completion_evidence" {
		t.Fatalf("completion diagnostics = %#v, want stable operation/classification", metadata)
	}
	if result.Error == "" || strings.Contains(result.Error, "sys") || strings.Contains(result.Error, "msg") {
		t.Fatalf("Error = %q, want a safe non-empty completion cause", result.Error)
	}
}

func TestAgentExecutor_TaskCompleteWithoutAuthoritativeFinalFails(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{
		Content: "partial output before the provider task-complete record",
		Diagnostics: &workerexecution.WorkDiagnostics{
			Command: &workerexecution.CommandDiagnostic{ExitCode: 0},
			Metadata: map[string]string{
				"artifact_present": "true",
			},
			Provider: &workerexecution.ProviderDiagnostic{
				ResponseMetadata: map[string]string{
					workerexecution.ProviderResponseMetadataCompletionEvidence: "task_complete",
				},
			},
		},
	}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "d-contradictory-completion", TransitionID: "t-contradictory-completion", WorkerType: "worker-a"},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Output != "" {
		t.Fatalf("Output = %q, want partial output withheld on failed completion validation", result.Output)
	}
	if result.Error != "provider completion evidence was contradictory" {
		t.Fatalf("Error = %q, want safe contradictory-completion cause", result.Error)
	}
	if result.Diagnostics == nil || result.Diagnostics.Provider == nil {
		t.Fatalf("Diagnostics = %#v, want completion-validation diagnostics", result.Diagnostics)
	}
	metadata := result.Diagnostics.Provider.ResponseMetadata
	if metadata[workerexecution.ProviderResponseMetadataFailureClassification] != "contradictory_completion" {
		t.Fatalf("completion diagnostics = %#v, want contradictory_completion classification", metadata)
	}
}

func TestAgentExecutor_ProviderSessionInspectionFailureIsTerminalAndSafe(t *testing.T) {
	inspectionErr := &providersessions.LookupError{
		Provider:  providersessions.ProviderCodex,
		SessionID: "rollout-inspection-limit",
		Err:       errors.Join(providersessions.ErrResourceLimitExceeded, errors.New("raw rollout and prompt must not escape")),
	}
	provider := &agentMockProvider{err: inspectionErr}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model", ModelProvider: string(modelprovider.ProviderCodex)},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "d-inspection-limit", TransitionID: "t-inspection-limit", WorkerType: "worker-a"},
		withAgentPrompts("secret system prompt", "secret user prompt"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed || provider.callCount != 1 {
		t.Fatalf("result = %#v, provider calls = %d, want one terminal failed attempt", result, provider.callCount)
	}
	if result.Error != "provider error: unknown: provider session inspection reached its configured limit" {
		t.Fatalf("result.Error = %q, want bounded inspection cause", result.Error)
	}
	if providers.SessionMetadataFromContinuation(result.Continuation) == nil || providers.SessionMetadataFromContinuation(result.Continuation).ID != "rollout-inspection-limit" {
		t.Fatalf("ProviderSession = %#v, want stable inspection session identity", providers.SessionMetadataFromContinuation(result.Continuation))
	}
	if strings.Contains(result.Error, "raw rollout") || strings.Contains(result.Error, "prompt") {
		t.Fatalf("result.Error leaked untrusted inspection context: %q", result.Error)
	}
	if result.Diagnostics == nil || result.Diagnostics.Provider == nil {
		t.Fatalf("Diagnostics = %#v, want provider diagnostics", result.Diagnostics)
	}
	if result.Diagnostics.Provider.RequestMetadata["dispatch_id"] != "d-inspection-limit" {
		t.Fatalf("request metadata = %#v, want stable dispatch id", result.Diagnostics.Provider.RequestMetadata)
	}
	metadata := result.Diagnostics.Provider.ResponseMetadata
	if metadata[workerexecution.ProviderResponseMetadataFailureOperation] != "provider_session_ingestion" ||
		metadata[workerexecution.ProviderResponseMetadataFailureClassification] != "resource_limit" {
		t.Fatalf("response metadata = %#v, want stable inspection classification", metadata)
	}
}

func TestAgentExecutor_SuccessfulResponse_PreservesProviderSession(t *testing.T) {
	provider := &agentMockProvider{
		response: workerexecution.InferenceResponse{
			Content: "The answer is 42.",
			Continuation: providers.ContinuationFromSessionMetadata(&providers.SessionMetadata{
				Provider: string(modelprovider.ProviderCodex),
				Kind:     providerSessionKindSessionID,
				ID:       "sess_codex_123",
			}),
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "gpt-5-codex", ModelProvider: "codex"},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("", "What is the meaning of life?"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if providers.SessionMetadataFromContinuation(result.Continuation) == nil {
		t.Fatal("expected provider session metadata on successful result")
	}
	if providers.SessionMetadataFromContinuation(result.Continuation).ID != "sess_codex_123" {
		t.Fatalf("provider session id = %q, want %q", providers.SessionMetadataFromContinuation(result.Continuation).ID, "sess_codex_123")
	}
}

func TestAgentExecutor_CodexWindowsExitCode4294967295_ReturnsRetryableProviderMetadata(t *testing.T) {
	provider := &agentMockProvider{
		err: workerexecution.NewProviderErrorWithSession(
			workerexecution.WorkFailureTypeInternalServerError,
			"Codex encountered a temporary server error.",
			nil,
			providers.ContinuationFromSessionMetadata(&providers.SessionMetadata{
				Provider: string(modelprovider.ProviderCodex),
				Kind:     providerSessionKindSessionID,
				ID:       "sess-codex-windows-4294967295",
			}),
		),
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "gpt-5.3-codex-spark", ModelProvider: string(modelprovider.ProviderCodex)},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("", "trigger Codex Windows process failure"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if provider.callCount != 3 {
		t.Fatalf("provider call count = %d, want 3 after retryable exhaustion", provider.callCount)
	}
	if result.FailureMetadata == nil {
		t.Fatal("expected failure metadata on failed result")
	}
	if result.FailureMetadata.Type != workerexecution.WorkFailureTypeInternalServerError {
		t.Fatalf("failure metadata type = %q, want %q", result.FailureMetadata.Type, workerexecution.WorkFailureTypeInternalServerError)
	}
	if result.FailureMetadata.Family != workerexecution.WorkFailureFamilyRetryable {
		t.Fatalf("failure metadata family = %q, want %q", result.FailureMetadata.Family, workerexecution.WorkFailureFamilyRetryable)
	}
	decision := workerexecution.WorkFailureDecisionFromMetadata(result.FailureMetadata)
	if !decision.Retryable || decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("WorkFailureDecisionFromMetadata(%#v) = %#v, want retryable non-terminal non-throttle", result.FailureMetadata, decision)
	}
	if providers.SessionMetadataFromContinuation(result.Continuation) == nil {
		t.Fatal("expected provider session metadata on failed result")
	}
	if providers.SessionMetadataFromContinuation(result.Continuation).ID != "sess-codex-windows-4294967295" {
		t.Fatalf("provider session id = %q, want %q", providers.SessionMetadataFromContinuation(result.Continuation).ID, "sess-codex-windows-4294967295")
	}
}

func TestAgentExecutor_TerminalProviderError_DoesNotRetry(t *testing.T) {
	provider := &agentMockProvider{
		errors: []error{
			workerexecution.NewProviderErrorWithSession(
				workerexecution.WorkFailureTypeAuthFailure,
				"auth failed",
				nil,
				providers.ContinuationFromSessionMetadata(&providers.SessionMetadata{
					Provider: string(modelprovider.ProviderCodex),
					Kind:     providerSessionKindSessionID,
					ID:       "sess_codex_error_123",
				}),
			),
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Error != "provider error: auth_failure: auth failed" {
		t.Fatalf("Error = %q, want %q", result.Error, "provider error: auth_failure: auth failed")
	}
	if provider.callCount != 1 {
		t.Fatalf("provider call count = %d, want 1", provider.callCount)
	}
	if result.Metrics.RetryCount != 0 {
		t.Fatalf("RetryCount = %d, want 0", result.Metrics.RetryCount)
	}
	if providers.SessionMetadataFromContinuation(result.Continuation) == nil {
		t.Fatal("expected provider session metadata on failed result")
	}
	if providers.SessionMetadataFromContinuation(result.Continuation).ID != "sess_codex_error_123" {
		t.Fatalf("provider session id = %q, want %q", providers.SessionMetadataFromContinuation(result.Continuation).ID, "sess_codex_error_123")
	}

}

func TestAgentExecutor_ClaudeProviderError_PreservesConfiguredSessionID(t *testing.T) {
	provider := &agentMockProvider{
		errors: []error{
			workerexecution.NewProviderErrorWithSession(
				workerexecution.WorkFailureTypeAuthFailure,
				"auth failed",
				nil,
				providers.ContinuationFromSessionMetadata(&providers.SessionMetadata{
					Provider: string(modelprovider.ProviderClaude),
					Kind:     providerSessionKindSessionID,
					ID:       "claude-session-123",
				}),
			),
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {
				Model:         "claude-sonnet-4-20250514",
				ModelProvider: string(modelprovider.ProviderClaude),
				SessionID:     "claude-session-123",
			},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if providers.SessionMetadataFromContinuation(result.Continuation) == nil {
		t.Fatal("expected provider session metadata on failed result")
	}
	if providers.SessionMetadataFromContinuation(result.Continuation).Provider != string(modelprovider.ProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", providers.SessionMetadataFromContinuation(result.Continuation).Provider, modelprovider.ProviderClaude)
	}
	if providers.SessionMetadataFromContinuation(result.Continuation).ID != "claude-session-123" {
		t.Fatalf("provider session id = %q, want %q", providers.SessionMetadataFromContinuation(result.Continuation).ID, "claude-session-123")
	}
}
