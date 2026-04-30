package workers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil/runtimefixtures"
)

type agentMockProvider struct {
	response  interfaces.InferenceResponse
	err       error
	responses []interfaces.InferenceResponse
	errors    []error
	callCount int
	lastReq   interfaces.ProviderInferenceRequest
}

type staticRuntimeConfig = runtimefixtures.RuntimeConfigLookupFixture

func (m *agentMockProvider) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	m.lastReq = req
	m.callCount++
	if idx := m.callCount - 1; idx < len(m.responses) || idx < len(m.errors) {
		var response interfaces.InferenceResponse
		if idx < len(m.responses) {
			response = m.responses[idx]
		}
		var err error
		if idx < len(m.errors) {
			err = m.errors[idx]
		}
		return response, err
	}
	return m.response, m.err
}

func testAgentRequest(dispatch interfaces.WorkDispatch, opts ...func(*interfaces.WorkstationExecutionRequest)) interfaces.WorkstationExecutionRequest {
	req := interfaces.WorkstationExecutionRequest{
		Dispatch:        interfaces.CloneWorkDispatch(dispatch),
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

func withAgentPrompts(systemPrompt, userMessage string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.SystemPrompt = systemPrompt
		req.UserMessage = userMessage
	}
}

func withAgentOutputSchema(schema string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.OutputSchema = schema
	}
}

func withAgentEnvVars(envVars map[string]string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.EnvVars = envVars
	}
}

func withAgentWorktree(worktree string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.Worktree = worktree
	}
}

func withAgentWorkingDirectory(workingDirectory string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.WorkingDirectory = workingDirectory
	}
}

func TestAgentExecutor_SuccessfulResponse_PopulatesOutput(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "The answer is 42."}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "claude-sonnet-4-20250514"},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("You are a helpful assistant.", "What is the meaning of life?"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
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
		response: interfaces.InferenceResponse{
			Content: "diagnosed response",
			Diagnostics: &interfaces.WorkDiagnostics{
				Provider: &interfaces.ProviderDiagnostic{
					ResponseMetadata: map[string]string{"request_id": "provider-request-1"},
				},
			},
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "claude-sonnet-4-20250514", ModelProvider: "claude"},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
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

func TestAgentExecutor_PropagatesExecutionMetadataToProviderRequest(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "done"}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "claude-sonnet-4-20250514", ModelProvider: "claude"},
		},
	}, provider)

	want := interfaces.ExecutionMetadata{
		DispatchCreatedTick: 7,
		CurrentTick:         8,
		TraceID:             "trace-1",
		WorkIDs:             []string{"work-1", "work-2"},
		ReplayKey:           "transition-1/trace-1/work-1/work-2",
	}
	_, err := executor.Execute(context.Background(), testAgentRequest(interfaces.WorkDispatch{
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

func TestAgentExecutor_InferenceRequestUsesCanonicalWorkDispatchPayload(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "done"}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {
				Model:         "claude-sonnet-4-20250514",
				ModelProvider: string(ModelProviderClaude),
				SessionID:     "session-1",
			},
		},
	}, provider)

	inputToken := interfaces.Token{
		ID: "token-1",
		Color: interfaces.TokenColor{
			WorkID:     "work-1",
			WorkTypeID: "task",
			TraceID:    "trace-1",
		},
	}
	dispatch := interfaces.WorkDispatch{
		DispatchID:      "dispatch-1",
		TransitionID:    "transition-1",
		WorkerType:      "worker-a",
		WorkstationName: "review",
		Execution:       interfaces.ExecutionMetadata{ReplayKey: "transition-1/trace-1/work-1", TraceID: "trace-1", WorkIDs: []string{"work-1"}},
		InputTokens:     InputTokens(inputToken),
		InputBindings:   map[string][]string{"task": {"token-1"}},
	}
	request := testAgentRequest(
		dispatch,
		withAgentWorktree("feature-worktree"),
		withAgentWorkingDirectory("C:\\repo"),
		withAgentEnvVars(map[string]string{"PORTOS_TEST_ENV": "enabled"}),
		withAgentPrompts("system prompt", "user prompt"),
		withAgentOutputSchema(`{"type":"object"}`),
	)

	_, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := provider.lastReq
	if req.Dispatch.DispatchID != dispatch.DispatchID || req.Dispatch.TransitionID != dispatch.TransitionID || req.Dispatch.WorkerType != dispatch.WorkerType {
		t.Fatalf("request identity = %#v, want dispatch identity %#v", req, dispatch)
	}
	if req.Dispatch.WorkstationName != dispatch.WorkstationName || req.WorkstationType != dispatch.WorkstationName {
		t.Fatalf("request workstation fields = name %q type %q, want %q", req.Dispatch.WorkstationName, req.WorkstationType, dispatch.WorkstationName)
	}
	if req.SystemPrompt != request.SystemPrompt || req.UserMessage != request.UserMessage || req.OutputSchema != request.OutputSchema {
		t.Fatalf("request prompt fields differ from execution request: %#v", req)
	}
	if req.Worktree != request.Worktree || req.WorkingDirectory != request.WorkingDirectory {
		t.Fatalf("request paths = worktree %q working_directory %q", req.Worktree, req.WorkingDirectory)
	}
	if req.Model != "claude-sonnet-4-20250514" || req.ModelProvider != string(ModelProviderClaude) || req.SessionID != "session-1" {
		t.Fatalf("request provider fields = model %q provider %q session %q", req.Model, req.ModelProvider, req.SessionID)
	}
	if req.EnvVars["PORTOS_TEST_ENV"] != "enabled" {
		t.Fatalf("request env vars = %#v", req.EnvVars)
	}
	if got := req.Dispatch.InputBindings["task"]; len(got) != 1 || got[0] != "token-1" {
		t.Fatalf("request input bindings = %#v", req.Dispatch.InputBindings)
	}
	tokens := cloneInputTokens(req.InputTokens)
	if len(tokens) != 1 || tokens[0].ID != inputToken.ID || tokens[0].Color.WorkID != inputToken.Color.WorkID {
		t.Fatalf("request input tokens = %#v, want %#v", tokens, inputToken)
	}
	assertExecutionMetadataEqual(t, dispatch.Execution, req.Dispatch.Execution)
}

func TestAgentExecutor_ClaudeSessionIDFromRuntimeConfigFlowsIntoProviderRequest(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "The answer is 42."}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {
				Model:         "claude-sonnet-4-20250514",
				ModelProvider: string(ModelProviderClaude),
				SessionID:     "claude-session-123",
			},
		},
	}, provider)

	_, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
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
		response: interfaces.InferenceResponse{
			Content: "The answer is 42.",
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: string(ModelProviderClaude),
				Kind:     providerSessionKindSessionID,
				ID:       "claude-session-123",
			},
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {
				Model:         "claude-sonnet-4-20250514",
				ModelProvider: string(ModelProviderClaude),
				SessionID:     "claude-session-123",
			},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("", "What is the meaning of life?"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderSession == nil {
		t.Fatal("expected provider session metadata on successful result")
	}
	if result.ProviderSession.Provider != string(ModelProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", result.ProviderSession.Provider, ModelProviderClaude)
	}
	if result.ProviderSession.ID != "claude-session-123" {
		t.Fatalf("provider session id = %q, want %q", result.ProviderSession.ID, "claude-session-123")
	}
}

func TestAgentExecutor_ProviderError_ReturnsFailedResult(t *testing.T) {
	provider := &agentMockProvider{err: errors.New("connection refused")}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != "provider error: connection refused" {
		t.Fatalf("Error = %q, want %q", result.Error, "provider error: connection refused")
	}
	if result.Metrics.RetryCount != 0 {
		t.Fatalf("RetryCount = %d, want 0", result.Metrics.RetryCount)
	}
}

func TestAgentExecutor_SuccessfulResponse_PreservesProviderSession(t *testing.T) {
	provider := &agentMockProvider{
		response: interfaces.InferenceResponse{
			Content: "The answer is 42.",
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: string(ModelProviderCodex),
				Kind:     providerSessionKindSessionID,
				ID:       "sess_codex_123",
			},
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "gpt-5-codex", ModelProvider: "codex"},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("", "What is the meaning of life?"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderSession == nil {
		t.Fatal("expected provider session metadata on successful result")
	}
	if result.ProviderSession.ID != "sess_codex_123" {
		t.Fatalf("provider session id = %q, want %q", result.ProviderSession.ID, "sess_codex_123")
	}
}

func TestAgentExecutor_RetryableProviderError_RetriesTwiceBeforeSuccess(t *testing.T) {
	provider := &agentMockProvider{
		errors: []error{
			NewProviderError(interfaces.ProviderErrorTypeInternalServerError, "provider 500", nil),
			NewProviderError(interfaces.ProviderErrorTypeTimeout, "provider timeout", nil),
			nil,
		},
		responses: []interfaces.InferenceResponse{
			{},
			{},
			{Content: "Recovered. COMPLETE"},
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider)
	var sleeps []time.Duration
	executor.retryConfig.sleep = func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}
	executor.retryConfig.jitter = func(baseDelay time.Duration) time.Duration {
		return baseDelay / 2
	}

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if result.Output != "Recovered. COMPLETE" {
		t.Fatalf("Output = %q, want %q", result.Output, "Recovered. COMPLETE")
	}
	if provider.callCount != 3 {
		t.Fatalf("provider call count = %d, want 3", provider.callCount)
	}
	if result.Metrics.RetryCount != 2 {
		t.Fatalf("RetryCount = %d, want 2", result.Metrics.RetryCount)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleep count = %d, want 2", len(sleeps))
	}
	if sleeps[0] != 150*time.Millisecond {
		t.Fatalf("first backoff = %v, want %v", sleeps[0], 150*time.Millisecond)
	}
	if sleeps[1] != 300*time.Millisecond {
		t.Fatalf("second backoff = %v, want %v", sleeps[1], 300*time.Millisecond)
	}
}

func TestAgentExecutor_CodexWindowsExitCode4294967295_RetriesAndReturnsRetryableProviderMetadata(t *testing.T) {
	provider := &agentMockProvider{
		err: normalizeProviderExitFailure(
			string(ModelProviderCodex),
			CommandResult{
				ExitCode: codexWindowsProcessFailureExitCode,
				Stderr:   []byte("OpenAI Codex v0.118.0 (research preview)\n--------\nERROR: Windows provider subprocess exited unexpectedly"),
			},
			&interfaces.ProviderSessionMetadata{
				Provider: string(ModelProviderCodex),
				Kind:     providerSessionKindSessionID,
				ID:       "sess-codex-windows-4294967295",
			},
			nil,
		),
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "gpt-5.3-codex-spark", ModelProvider: string(ModelProviderCodex)},
		},
	}, provider)
	var sleeps []time.Duration
	executor.retryConfig.sleep = func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}
	executor.retryConfig.jitter = func(time.Duration) time.Duration { return 0 }

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("", "trigger Codex Windows process failure"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if provider.callCount != 3 {
		t.Fatalf("provider call count = %d, want 3", provider.callCount)
	}
	if result.Metrics.RetryCount != 2 {
		t.Fatalf("RetryCount = %d, want 2", result.Metrics.RetryCount)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleep count = %d, want 2", len(sleeps))
	}
	if result.ProviderFailure == nil {
		t.Fatal("expected provider failure metadata on failed result")
	}
	if result.ProviderFailure.Type != interfaces.ProviderErrorTypeInternalServerError {
		t.Fatalf("provider failure type = %q, want %q", result.ProviderFailure.Type, interfaces.ProviderErrorTypeInternalServerError)
	}
	if result.ProviderFailure.Family != interfaces.ProviderErrorFamilyRetryable {
		t.Fatalf("provider failure family = %q, want %q", result.ProviderFailure.Family, interfaces.ProviderErrorFamilyRetryable)
	}
	decision := ProviderFailureDecisionFromMetadata(result.ProviderFailure)
	if !decision.Retryable || decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("ProviderFailureDecisionFromMetadata(%#v) = %#v, want retryable non-terminal non-throttle", result.ProviderFailure, decision)
	}
	if result.ProviderSession == nil {
		t.Fatal("expected provider session metadata on failed result")
	}
	if result.ProviderSession.ID != "sess-codex-windows-4294967295" {
		t.Fatalf("provider session id = %q, want %q", result.ProviderSession.ID, "sess-codex-windows-4294967295")
	}
}

func TestAgentExecutor_TerminalProviderError_DoesNotRetry(t *testing.T) {
	provider := &agentMockProvider{
		errors: []error{
			NewProviderErrorWithSession(
				interfaces.ProviderErrorTypeAuthFailure,
				"auth failed",
				nil,
				&interfaces.ProviderSessionMetadata{
					Provider: string(ModelProviderCodex),
					Kind:     providerSessionKindSessionID,
					ID:       "sess_codex_error_123",
				},
			),
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider)
	sleepCalled := false
	executor.retryConfig.sleep = func(_ context.Context, _ time.Duration) error {
		sleepCalled = true
		return nil
	}
	executor.retryConfig.jitter = func(time.Duration) time.Duration { return 0 }

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
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
	if result.ProviderSession == nil {
		t.Fatal("expected provider session metadata on failed result")
	}
	if result.ProviderSession.ID != "sess_codex_error_123" {
		t.Fatalf("provider session id = %q, want %q", result.ProviderSession.ID, "sess_codex_error_123")
	}
	if sleepCalled {
		t.Fatal("expected terminal provider error to skip retry sleep")
	}
}

func TestAgentExecutor_ClaudeProviderError_PreservesConfiguredSessionID(t *testing.T) {
	provider := &agentMockProvider{
		errors: []error{
			NewProviderErrorWithSession(
				interfaces.ProviderErrorTypeAuthFailure,
				"auth failed",
				nil,
				&interfaces.ProviderSessionMetadata{
					Provider: string(ModelProviderClaude),
					Kind:     providerSessionKindSessionID,
					ID:       "claude-session-123",
				},
			),
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {
				Model:         "claude-sonnet-4-20250514",
				ModelProvider: string(ModelProviderClaude),
				SessionID:     "claude-session-123",
			},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderSession == nil {
		t.Fatal("expected provider session metadata on failed result")
	}
	if result.ProviderSession.Provider != string(ModelProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", result.ProviderSession.Provider, ModelProviderClaude)
	}
	if result.ProviderSession.ID != "claude-session-123" {
		t.Fatalf("provider session id = %q, want %q", result.ProviderSession.ID, "claude-session-123")
	}
}

func TestAgentExecutor_OutputSchemaSuccess_KeepsRawOutput(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: `{"work_id":"w-1","tags":{"result":"done"}}`}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
		withAgentOutputSchema(`{"type":"object"}`),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if result.Output != `{"work_id":"w-1","tags":{"result":"done"}}` {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestAgentExecutor_OutputSchemaParseFailure_ReturnsFailedResult(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "not valid json at all"}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
		withAgentOutputSchema(`{"type":"object"}`),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error == "" {
		t.Fatal("expected parse error")
	}
	if result.Output != "not valid json at all" {
		t.Fatalf("Output = %q, want raw response", result.Output)
	}
}

func TestAgentExecutor_StopTokenControlsOutcome(t *testing.T) {
	runtimeCfg := staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "test-model", StopToken: "COMPLETE"},
		},
	}
	executor := NewAgentExecutor(
		runtimeCfg,
		&agentMockProvider{response: interfaces.InferenceResponse{Content: "Work done. COMPLETE"}},
	)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}

	executor = NewAgentExecutor(
		runtimeCfg,
		&agentMockProvider{response: interfaces.InferenceResponse{Content: "Still working"}},
	)
	result, err = executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-2",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeRejected {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeRejected)
	}
}

func TestAgentExecutor_StopTokenComesFromRuntimeConfigWithoutDispatchState(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "Work done. COMPLETE"}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "test-model", StopToken: "COMPLETE"},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
}

func TestAgentExecutor_RuntimeStopTokenChangesAffectSubsequentDispatches(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "Work done. COMPLETE"}}
	workerDef := &interfaces.WorkerConfig{Model: "test-model", StopToken: "COMPLETE"}
	runtimeCfg := staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": workerDef,
		},
	}
	executor := NewAgentExecutor(runtimeCfg, provider)

	dispatch := interfaces.WorkDispatch{
		DispatchID:   "d-1",
		TransitionID: "t-1",
		WorkerType:   "worker-a",
	}

	firstRequest := testAgentRequest(dispatch, withAgentPrompts("sys", "msg"))
	first, err := executor.Execute(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("first execute error: %v", err)
	}
	if first.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("first outcome = %s, want %s", first.Outcome, interfaces.OutcomeAccepted)
	}

	workerDef.StopToken = "DONE"

	second, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-2",
			TransitionID: dispatch.TransitionID,
			WorkerType:   dispatch.WorkerType,
		},
		withAgentPrompts(firstRequest.SystemPrompt, firstRequest.UserMessage),
	))
	if err != nil {
		t.Fatalf("second execute error: %v", err)
	}
	if second.Outcome != interfaces.OutcomeRejected {
		t.Fatalf("second outcome = %s, want %s", second.Outcome, interfaces.OutcomeRejected)
	}
}

func TestAgentExecutor_ResolvesWorkerConfigPerDispatch(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "done"}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "model-a", ModelProvider: "claude"},
			"worker-b": {Model: "model-b", ModelProvider: "codex"},
		},
	}, provider)

	first, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg-a"),
	))
	if err != nil {
		t.Fatalf("first execute error: %v", err)
	}
	if first.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("first outcome = %s, want %s", first.Outcome, interfaces.OutcomeAccepted)
	}
	if provider.lastReq.Model != "model-a" || provider.lastReq.ModelProvider != "claude" {
		t.Fatalf("first request = %#v", provider.lastReq)
	}

	second, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-2",
			TransitionID: "t-2",
			WorkerType:   "worker-b",
		},
		withAgentPrompts("sys", "msg-b"),
	))
	if err != nil {
		t.Fatalf("second execute error: %v", err)
	}
	if second.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("second outcome = %s, want %s", second.Outcome, interfaces.OutcomeAccepted)
	}
	if provider.lastReq.Model != "model-b" || provider.lastReq.ModelProvider != "codex" {
		t.Fatalf("second request = %#v", provider.lastReq)
	}
}
