package agent_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	executorpkg "github.com/portpowered/infinite-you/pkg/workers/executor"
)

type agentMockProvider struct {
	response  interfaces.InferenceResponse
	callCount int
	lastReq   interfaces.ProviderInferenceRequest
}

func (m *agentMockProvider) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	m.lastReq = req
	m.callCount++
	return m.response, nil
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

func TestAgentExecutor_StopTokenControlsOutcome(t *testing.T) {
	runtimeCfg := runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "test-model", StopToken: "COMPLETE"},
		},
	}
	executor := executorpkg.NewAgentExecutor(
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

	executor = executorpkg.NewAgentExecutor(
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

	executor = executorpkg.NewAgentExecutor(
		runtimeCfg,
		&agentMockProvider{response: interfaces.InferenceResponse{Content: "Still iterating\n<CONTINUE>"}},
	)
	result, err = executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-3",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeContinue {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeContinue)
	}
}

func TestAgentExecutor_StopTokenComesFromRuntimeConfigWithoutDispatchState(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "Work done. COMPLETE"}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
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
	runtimeCfg := runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": workerDef,
		},
	}
	executor := executorpkg.NewAgentExecutor(runtimeCfg, provider)

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
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
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

func TestAgentExecutor_OutputSchemaSuccess_KeepsRawOutput(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: `{"work_id":"w-1","tags":{"result":"done"}}`}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
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
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
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
