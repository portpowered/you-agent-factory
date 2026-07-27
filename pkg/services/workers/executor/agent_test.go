package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
)

type agentMockProvider struct {
	response  workerexecution.InferenceResponse
	err       error
	responses []workerexecution.InferenceResponse
	errors    []error
	callCount int
	lastReq   workerexecution.ProviderInferenceRequest
}

type staticRuntimeConfig = runtimefixtures.RuntimeConfigLookupFixture

var deterministicRetryRandom = platformrandom.SourceFunc(func(int64) (int64, error) {
	return 0, nil
})

type sequenceRetryRandom struct {
	values []int64
	bounds []int64
}

func (source *sequenceRetryRandom) Int63n(upperBound int64) (int64, error) {
	source.bounds = append(source.bounds, upperBound)
	value := source.values[0]
	source.values = source.values[1:]
	return value, nil
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
		deterministicRetryRandom,
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
		return response, err
	}
	return m.response, m.err
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
	}, provider, nil, time.Now, deterministicRetryRandom)

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
	}, provider, nil, time.Now, deterministicRetryRandom)
	executor.retryConfig.maxRetries = 0

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
	}, provider, nil, time.Now, deterministicRetryRandom)

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
		err: workerprovider.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "provider 500", nil),
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
	}, provider, nil, time.Now, deterministicRetryRandom)
	executor.retryConfig.maxRetries = 0

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
	}, provider, nil, time.Now, deterministicRetryRandom)

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
	}, provider, nil, time.Now, deterministicRetryRandom)

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
			ProviderSession: &workerexecution.ProviderSessionMetadata{
				Provider: string(modelprovider.ProviderClaude),
				Kind:     providerSessionKindSessionID,
				ID:       "claude-session-123",
			},
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
	}, provider, nil, time.Now, deterministicRetryRandom)

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
	if result.ProviderSession == nil {
		t.Fatal("expected provider session metadata on successful result")
	}
	if result.ProviderSession.Provider != string(modelprovider.ProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", result.ProviderSession.Provider, modelprovider.ProviderClaude)
	}
	if result.ProviderSession.ID != "claude-session-123" {
		t.Fatalf("provider session id = %q, want %q", result.ProviderSession.ID, "claude-session-123")
	}
}

func TestAgentExecutor_ProviderError_ReturnsFailedResult(t *testing.T) {
	provider := &agentMockProvider{err: errors.New("connection refused")}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider, nil, time.Now, deterministicRetryRandom)

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
}

func TestAgentExecutor_SuccessfulResponse_PreservesProviderSession(t *testing.T) {
	provider := &agentMockProvider{
		response: workerexecution.InferenceResponse{
			Content: "The answer is 42.",
			ProviderSession: &workerexecution.ProviderSessionMetadata{
				Provider: string(modelprovider.ProviderCodex),
				Kind:     providerSessionKindSessionID,
				ID:       "sess_codex_123",
			},
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "gpt-5-codex", ModelProvider: "codex"},
		},
	}, provider, nil, time.Now, deterministicRetryRandom)

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
			workerprovider.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "provider 500", nil),
			workerprovider.NewProviderError(workerexecution.WorkFailureTypeTimeout, "provider timeout", nil),
			nil,
		},
		responses: []workerexecution.InferenceResponse{
			{},
			{},
			{Content: "Recovered. COMPLETE"},
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider, nil, time.Now, deterministicRetryRandom)

	var sleeps []time.Duration
	executor.retryConfig.sleep = func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}
	executor.retryConfig.jitter = func(baseDelay time.Duration) (time.Duration, error) {
		return baseDelay / 2, nil
	}

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

	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
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

func TestAgentExecutor_RetryJitterUsesInjectedRandomSource(t *testing.T) {
	provider := &agentMockProvider{
		errors: []error{
			workerprovider.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "first failure", nil),
			workerprovider.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "second failure", nil),
			nil,
		},
		responses: []workerexecution.InferenceResponse{{}, {}, {Content: "recovered"}},
	}
	random := &sequenceRetryRandom{values: []int64{int64(25 * time.Millisecond), int64(50 * time.Millisecond)}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider, nil, time.Now, random)
	var sleeps []time.Duration
	executor.retryConfig.sleep = func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "d-jitter", TransitionID: "t-jitter", WorkerType: "worker-a"},
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	wantSleeps := []time.Duration{125 * time.Millisecond, 250 * time.Millisecond}
	if len(sleeps) != len(wantSleeps) || sleeps[0] != wantSleeps[0] || sleeps[1] != wantSleeps[1] {
		t.Fatalf("retry sleeps = %v, want %v", sleeps, wantSleeps)
	}
	wantBounds := []int64{int64(50*time.Millisecond) + 1, int64(100*time.Millisecond) + 1}
	if len(random.bounds) != len(wantBounds) || random.bounds[0] != wantBounds[0] || random.bounds[1] != wantBounds[1] {
		t.Fatalf("random bounds = %v, want %v", random.bounds, wantBounds)
	}
}

func TestAgentExecutor_CodexWindowsExitCode4294967295_RetriesAndReturnsRetryableProviderMetadata(t *testing.T) {
	provider := &agentMockProvider{
		err: workerprovider.NewProviderErrorWithSession(
			workerexecution.WorkFailureTypeInternalServerError,
			"Codex encountered a temporary server error.",
			nil,
			&workerexecution.ProviderSessionMetadata{
				Provider: string(modelprovider.ProviderCodex),
				Kind:     providerSessionKindSessionID,
				ID:       "sess-codex-windows-4294967295",
			},
		),
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "gpt-5.3-codex-spark", ModelProvider: string(modelprovider.ProviderCodex)},
		},
	}, provider, nil, time.Now, deterministicRetryRandom)

	var sleeps []time.Duration
	executor.retryConfig.sleep = func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}
	executor.retryConfig.jitter = func(time.Duration) (time.Duration, error) { return 0, nil }

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
		t.Fatalf("provider call count = %d, want 3", provider.callCount)
	}
	if result.Metrics.RetryCount != 2 {
		t.Fatalf("RetryCount = %d, want 2", result.Metrics.RetryCount)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleep count = %d, want 2", len(sleeps))
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
	decision := workerprovider.WorkFailureDecisionFromMetadata(result.FailureMetadata)
	if !decision.Retryable || decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("WorkFailureDecisionFromMetadata(%#v) = %#v, want retryable non-terminal non-throttle", result.FailureMetadata, decision)
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
			workerprovider.NewProviderErrorWithSession(
				workerexecution.WorkFailureTypeAuthFailure,
				"auth failed",
				nil,
				&workerexecution.ProviderSessionMetadata{
					Provider: string(modelprovider.ProviderCodex),
					Kind:     providerSessionKindSessionID,
					ID:       "sess_codex_error_123",
				},
			),
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider, nil, time.Now, deterministicRetryRandom)

	sleepCalled := false
	executor.retryConfig.sleep = func(_ context.Context, _ time.Duration) error {
		sleepCalled = true
		return nil
	}
	executor.retryConfig.jitter = func(time.Duration) (time.Duration, error) { return 0, nil }

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
			workerprovider.NewProviderErrorWithSession(
				workerexecution.WorkFailureTypeAuthFailure,
				"auth failed",
				nil,
				&workerexecution.ProviderSessionMetadata{
					Provider: string(modelprovider.ProviderClaude),
					Kind:     providerSessionKindSessionID,
					ID:       "claude-session-123",
				},
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
	}, provider, nil, time.Now, deterministicRetryRandom)

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
	if result.ProviderSession == nil {
		t.Fatal("expected provider session metadata on failed result")
	}
	if result.ProviderSession.Provider != string(modelprovider.ProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", result.ProviderSession.Provider, modelprovider.ProviderClaude)
	}
	if result.ProviderSession.ID != "claude-session-123" {
		t.Fatalf("provider session id = %q, want %q", result.ProviderSession.ID, "claude-session-123")
	}
}

func TestAgentExecutor_RawDeadlineExceeded_RetriesBeforeSuccess(t *testing.T) {
	provider := &agentMockProvider{
		errors: []error{
			context.DeadlineExceeded,
			context.DeadlineExceeded,
			nil,
		},
		responses: []workerexecution.InferenceResponse{
			{},
			{},
			{Content: "Recovered. COMPLETE"},
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider, nil, time.Now, deterministicRetryRandom)

	var sleeps []time.Duration
	executor.retryConfig.sleep = func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}
	executor.retryConfig.jitter = func(baseDelay time.Duration) (time.Duration, error) {
		return baseDelay / 2, nil
	}

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-raw-timeout-success",
			TransitionID: "t-raw-timeout-success",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
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
}

func TestAgentExecutor_RawDeadlineExceeded_ExhaustsRetriesIntoStructuredTimeoutFailure(t *testing.T) {
	provider := &agentMockProvider{err: context.DeadlineExceeded}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider, nil, time.Now, deterministicRetryRandom)

	var sleeps []time.Duration
	executor.retryConfig.sleep = func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}
	executor.retryConfig.jitter = func(time.Duration) (time.Duration, error) { return 0, nil }

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-raw-timeout-fail",
			TransitionID: "t-raw-timeout-fail",
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
	if result.Error != "execution timeout" {
		t.Fatalf("Error = %q, want %q", result.Error, "execution timeout")
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
	if result.FailureMetadata == nil {
		t.Fatal("FailureMetadata = nil, want timeout metadata")
	}
	if result.FailureMetadata.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("FailureMetadata.Type = %q, want %q", result.FailureMetadata.Type, workerexecution.WorkFailureTypeTimeout)
	}
	if result.FailureMetadata.Family != workerexecution.WorkFailureFamilyRetryable {
		t.Fatalf("FailureMetadata.Family = %q, want %q", result.FailureMetadata.Family, workerexecution.WorkFailureFamilyRetryable)
	}
}
