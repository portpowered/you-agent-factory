package executor

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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

	executor = NewAgentExecutor(
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
