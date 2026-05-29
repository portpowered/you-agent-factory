package agent_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	executorpkg "github.com/portpowered/infinite-you/pkg/workers/executor"
)

func TestAgentExecutor_ForwardsOpenCodeAgentOnOpenCodeDispatch(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "ok"}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
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

func TestAgentExecutor_LeavesOpenCodeAgentEmptyForNonOpenCodeDispatch(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "ok"}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {
				Name:          "review",
				OpenCodeAgent: "implementer",
			},
		},
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {
				Model:         "gpt-5",
				ModelProvider: string(interfaces.ModelProviderCodex),
				OpenCodeAgent: "reviewer",
			},
		},
	}, provider)

	_, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:      "d-codex",
			TransitionID:    "t-codex",
			WorkerType:      "worker-a",
			WorkstationName: "review",
		},
		withAgentPrompts("system", "user"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.lastReq.OpenCodeAgent != "" {
		t.Fatalf("OpenCodeAgent = %q, want empty for non-opencode dispatch", provider.lastReq.OpenCodeAgent)
	}
}
