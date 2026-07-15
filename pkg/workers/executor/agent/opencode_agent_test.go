package agent_test

import (
	"context"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	executorpkg "github.com/portpowered/infinite-you/pkg/workers/executor"
)

func TestAgentExecutor_ForwardsOpenCodeAgentOnOpenCodeDispatch(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "ok"}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"implement": {
				Name:          "implement",
				Runner:        workerexecution.RunnerIDOpenCode,
				OpenCodeAgent: "implementer",
			},
		},
		Workers: map[string]*workerconfig.Config{
			"worker-a": {
				Model:         "opencode-model",
				ModelProvider: string(modelprovider.Claude),
				OpenCodeAgent: "reviewer",
			},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:      "d-1",
			TransitionID:    "t-1",
			WorkerType:      "worker-a",
			WorkstationName: "implement",
		},
		withAgentPrompts("system", "user"),
		func(req *workerexecution.WorkstationExecutionRequest) {
			req.RunnerID = workerexecution.RunnerIDOpenCode
			req.RunnerSelectionSource = workerexecution.RunnerSelectionSourceWorkstation
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
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "ok"}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {
				Name:          "review",
				OpenCodeAgent: "implementer",
			},
		},
		Workers: map[string]*workerconfig.Config{
			"worker-a": {
				Model:         "gpt-5",
				ModelProvider: string(modelprovider.Codex),
				OpenCodeAgent: "reviewer",
			},
		},
	}, provider)

	_, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
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
