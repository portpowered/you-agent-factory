package executor

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestInferenceRequestForExecutionRequest_SetsOpenCodeAgentForOpenCodeDispatch(t *testing.T) {
	req := testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:      "d-opencode",
			TransitionID:    "t-opencode",
			WorkerType:      "worker-a",
			WorkstationName: "implement",
		},
		withAgentPrompts("system", "user"),
	)
	req.RunnerID = interfaces.RunnerIDOpenCode
	req.RunnerSelectionSource = interfaces.RunnerSelectionSourceWorkstation

	got := inferenceRequestForExecutionRequest(req, &interfaces.WorkerConfig{
		Model:         "opencode-model",
		OpenCodeAgent: "reviewer",
	}, &interfaces.FactoryWorkstationConfig{
		Name:          "implement",
		OpenCodeAgent: "implementer",
	})

	if got.ModelProvider != string(interfaces.ModelProviderOpenCode) {
		t.Fatalf("model provider = %q, want %q", got.ModelProvider, interfaces.ModelProviderOpenCode)
	}
	if got.OpenCodeAgent != "implementer" {
		t.Fatalf("OpenCodeAgent = %q, want workstation override implementer", got.OpenCodeAgent)
	}
}

func TestInferenceRequestForExecutionRequest_LeavesOpenCodeAgentEmptyForNonOpenCodeDispatch(t *testing.T) {
	req := testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:      "d-codex",
			TransitionID:    "t-codex",
			WorkerType:      "worker-a",
			WorkstationName: "review",
		},
		withAgentPrompts("system", "user"),
	)

	got := inferenceRequestForExecutionRequest(req, &interfaces.WorkerConfig{
		Model:         "gpt-5",
		ModelProvider: string(interfaces.ModelProviderCodex),
		OpenCodeAgent: "reviewer",
	}, &interfaces.FactoryWorkstationConfig{
		Name:          "review",
		OpenCodeAgent: "implementer",
	})

	if got.ModelProvider != string(interfaces.ModelProviderCodex) {
		t.Fatalf("model provider = %q, want %q", got.ModelProvider, interfaces.ModelProviderCodex)
	}
	if got.OpenCodeAgent != "" {
		t.Fatalf("OpenCodeAgent = %q, want empty for non-opencode dispatch", got.OpenCodeAgent)
	}
}

func TestAgentExecutor_ForwardsOpenCodeAgentOnOpenCodeDispatch(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "ok"}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"implement": {
				Name:          "implement",
				Runner:        interfaces.RunnerIDOpenCode,
				OpenCodeAgent: "implementer",
			},
		},
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {
				Model:         "opencode-model",
				ModelProvider: string(interfaces.ModelProviderClaude),
				OpenCodeAgent: "reviewer",
			},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:      "d-1",
			TransitionID:    "t-1",
			WorkerType:      "worker-a",
			WorkstationName: "implement",
		},
		withAgentPrompts("system", "user"),
		func(req *interfaces.WorkstationExecutionRequest) {
			req.RunnerID = interfaces.RunnerIDOpenCode
			req.RunnerSelectionSource = interfaces.RunnerSelectionSourceWorkstation
		},
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.lastReq.OpenCodeAgent != "implementer" {
		t.Fatalf("OpenCodeAgent = %q, want workstation override implementer", provider.lastReq.OpenCodeAgent)
	}
	if result.Diagnostics == nil || result.Diagnostics.Provider == nil {
		t.Fatal("expected provider diagnostics on work result")
	}
	if got := result.Diagnostics.Provider.RequestMetadata["opencode_agent"]; got != "implementer" {
		t.Fatalf("diagnostic opencode_agent = %q, want implementer", got)
	}
}
