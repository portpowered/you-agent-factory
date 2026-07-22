package agent_test

import (
	"context"
	"testing"
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/services/work"
	executorpkg "github.com/portpowered/infinite-you/pkg/services/workers/executor"
)

type agentMockProvider struct {
	response  workerexecution.InferenceResponse
	callCount int
	lastReq   workerexecution.ProviderInferenceRequest
}

func (m *agentMockProvider) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	m.lastReq = req
	m.callCount++
	return m.response, nil
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

func TestAgentExecutor_StopTokenControlsOutcome(t *testing.T) {
	runtimeCfg := runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model", StopToken: "COMPLETE"},
		},
	}
	executor := executorpkg.NewAgentExecutor(
		runtimeCfg,
		&agentMockProvider{response: workerexecution.InferenceResponse{Content: "Work done. COMPLETE"}}, nil, time.Now, deterministicRetryRandom)

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

	executor = executorpkg.NewAgentExecutor(
		runtimeCfg,
		&agentMockProvider{response: workerexecution.InferenceResponse{Content: "Still working"}}, nil, time.Now, deterministicRetryRandom)

	result, err = executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-2",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeRejected {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeRejected)
	}

	executor = executorpkg.NewAgentExecutor(
		runtimeCfg,
		&agentMockProvider{response: workerexecution.InferenceResponse{Content: "Still iterating\n<CONTINUE>"}}, nil, time.Now, deterministicRetryRandom)

	result, err = executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-3",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeContinue {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeContinue)
	}
}

func TestAgentExecutor_StopTokenComesFromRuntimeConfigWithoutDispatchState(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "Work done. COMPLETE"}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model", StopToken: "COMPLETE"},
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
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
}

func TestAgentExecutor_RuntimeStopTokenChangesAffectSubsequentDispatches(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "Work done. COMPLETE"}}
	workerDef := &workerconfig.FactoryWorkerConfig{Model: "test-model", StopToken: "COMPLETE"}
	runtimeCfg := runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": workerDef,
		},
	}
	executor := executorpkg.NewAgentExecutor(runtimeCfg, provider, nil, time.Now, deterministicRetryRandom)

	dispatch := work.WorkDispatch{
		DispatchID:   "d-1",
		TransitionID: "t-1",
		WorkerType:   "worker-a",
	}

	firstRequest := testAgentRequest(dispatch, withAgentPrompts("sys", "msg"))
	first, err := executor.Execute(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("first execute error: %v", err)
	}
	if first.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("first outcome = %s, want %s", first.Outcome, workerexecution.OutcomeAccepted)
	}

	workerDef.StopToken = "DONE"

	second, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-2",
			TransitionID: dispatch.TransitionID,
			WorkerType:   dispatch.WorkerType,
		},
		withAgentPrompts(firstRequest.SystemPrompt, firstRequest.UserMessage),
	))
	if err != nil {
		t.Fatalf("second execute error: %v", err)
	}
	if second.Outcome != workerexecution.OutcomeRejected {
		t.Fatalf("second outcome = %s, want %s", second.Outcome, workerexecution.OutcomeRejected)
	}
}

func TestAgentExecutor_ResolvesWorkerConfigPerDispatch(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "done"}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "model-a", ModelProvider: "claude"},
			"worker-b": {Model: "model-b", ModelProvider: "codex"},
		},
	}, provider, nil, time.Now, deterministicRetryRandom)

	first, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg-a"),
	))
	if err != nil {
		t.Fatalf("first execute error: %v", err)
	}
	if first.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("first outcome = %s, want %s", first.Outcome, workerexecution.OutcomeAccepted)
	}
	if provider.lastReq.Model != "model-a" || provider.lastReq.ModelProvider != "claude" {
		t.Fatalf("first request = %#v", provider.lastReq)
	}

	second, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-2",
			TransitionID: "t-2",
			WorkerType:   "worker-b",
		},
		withAgentPrompts("sys", "msg-b"),
	))
	if err != nil {
		t.Fatalf("second execute error: %v", err)
	}
	if second.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("second outcome = %s, want %s", second.Outcome, workerexecution.OutcomeAccepted)
	}
	if provider.lastReq.Model != "model-b" || provider.lastReq.ModelProvider != "codex" {
		t.Fatalf("second request = %#v", provider.lastReq)
	}
}

func TestAgentExecutor_OutputSchemaSuccess_KeepsRawOutput(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: `{"work_id":"w-1","tags":{"result":"done"}}`}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
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
		withAgentOutputSchema(`{"type":"object"}`),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if result.Output != `{"work_id":"w-1","tags":{"result":"done"}}` {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestAgentExecutor_OutputSchemaParseFailure_ReturnsFailedResult(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "not valid json at all"}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
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
		withAgentOutputSchema(`{"type":"object"}`),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Error == "" {
		t.Fatal("expected parse error")
	}
	if result.Output != "not valid json at all" {
		t.Fatalf("Output = %q, want raw response", result.Output)
	}
}
