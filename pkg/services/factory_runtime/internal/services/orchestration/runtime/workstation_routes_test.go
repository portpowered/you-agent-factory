package runtime

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type routeNamesTestExecutor struct{}

func (routeNamesTestExecutor) Execute(context.Context, work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{}, nil
}

func TestRuntimeWorkstationRouteNamesIncludeWorkerAndWorkstationKeys(t *testing.T) {
	net := &state.Net{
		Transitions: map[string]*petri.Transition{
			"tr-1": {ID: "tr-1", Name: "review", WorkerType: "swe"},
		},
	}
	names := runtimeWorkstationRouteNames(net, map[string]workers.WorkerExecutor{
		"swe": routeNamesTestExecutor{},
	})
	want := map[string]struct{}{"tr-1": {}, "review": {}, "swe": {}}
	if len(names) != len(want) {
		t.Fatalf("route names = %v, want %v", names, want)
	}
	for _, name := range names {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected route name %q in %v", name, names)
		}
	}
}

func TestApplyRuntimeWorkstationSelectionMarksGoalRoutingEnvelope(t *testing.T) {
	selection := runtimeExecutionSelection{}
	applyRuntimeWorkstationSelection(nil, &selection, nil, &interfaces.FactoryWorkstationConfig{
		OutcomeFormat: interfaces.DecisionEnvelopeOutcomeFormat,
		ClassificationRoutes: []interfaces.ClassificationRouteConfig{{
			Label: "accepted",
		}},
	})

	if !selection.decisionEnvelope || !selection.goalRoutingDecisionEnvelope {
		t.Fatalf("selection output policy = %#v, want decision and goal-routing envelopes", selection)
	}
}

func TestFinalizeRuntimeExecutionSelectionUsesProviderRunner(t *testing.T) {
	tests := []struct {
		name          string
		providerID    string
		modelProvider string
		wantRunner    string
	}{
		{name: "authored claude model provider", modelProvider: "claude", wantRunner: workers.RunnerIDClaude},
		{name: "authored agy executor provider", providerID: "agy", modelProvider: "codex", wantRunner: workers.RunnerIDAntigravity},
		{name: "unknown provider uses codex default", modelProvider: "operator-provider", wantRunner: workers.RunnerIDCodex},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection := runtimeExecutionSelection{
				providerID:    test.providerID,
				modelProvider: test.modelProvider,
				model:         "model",
				workerType:    interfaces.WorkerTypeModel,
			}
			finalizeRuntimeExecutionSelection(&selection, nil)
			if selection.runnerID != test.wantRunner {
				t.Fatalf("runnerID = %q, want %q; selection = %#v", selection.runnerID, test.wantRunner, selection)
			}
		})
	}
}

func TestApplyRuntimeWorkerSelectionUsesWorkerBodyAsSystemPrompt(t *testing.T) {
	selection := runtimeExecutionSelection{}
	applyRuntimeWorkerSelection(nil, &selection, workers.WorkstationExecutionRequest{}, nil, &interfaces.FactoryWorkerConfig{
		Name: "worker",
		Type: interfaces.WorkerTypeModel,
		Body: "worker system prompt",
	})

	if selection.systemPrompt != "worker system prompt" {
		t.Fatalf("systemPrompt = %q, want worker body", selection.systemPrompt)
	}
}
