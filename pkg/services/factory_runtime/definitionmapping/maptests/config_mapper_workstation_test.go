package maptests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestConfigMapping_WorkstationTypeDefaultsToStandard(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				// Type not set — should default to "standard"
			},
		},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["processor"]
	if tr == nil {
		t.Fatal("expected mapped transition for processor")
	}
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:failed" {
		t.Fatalf("default standard workstation should reject through failure routing, got %+v", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("default standard workstation should fail through failed-state routing, got %+v", tr.FailureArcs)
	}
}

func TestConfigMapping_WorkstationTypeExplicitStandard(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Kind: interfaces.WorkstationKindStandard,
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["processor"]
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:failed" {
		t.Fatalf("explicit standard workstation should reject through failure routing, got %+v", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("explicit standard workstation should fail to task:failed, got %+v", tr.FailureArcs)
	}
}

func TestConfigMapping_WorkstationTypeRepeater(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Kind: interfaces.WorkstationKindRepeater,
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["processor"]
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:init" {
		t.Fatalf("expected auto rejection arc to task:init, got %+v", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("expected auto failure arc to task:failed, got %+v", tr.FailureArcs)
	}
}

func TestConfigMapping_WorkstationKindPollerUsesImplicitFailureRouting(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{
			Name: "poller-worker",
			Type: interfaces.WorkerTypeScript,
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "poll-task",
			Kind:           interfaces.WorkstationKindPoller,
			Type:           interfaces.WorkstationTypeModel,
			WorkerTypeName: "poller-worker",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
		}},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["poll-task"]
	if tr == nil {
		t.Fatal("expected mapped transition for poll-task")
	}
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:failed" {
		t.Fatalf("poller rejection arcs = %+v, want default failed-state routing", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("poller failure arcs = %+v, want default failed-state routing", tr.FailureArcs)
	}
}

func TestConfigMapping_ModelInvokeWorkstationUsesImplicitFailureRouting(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{
			Name: "tts-worker",
			Type: interfaces.WorkerTypeModel,
			Operations: []interfaces.ModelOperation{{
				Name: "TTS",
				Inputs: []interfaces.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{interfaces.ModelOperationContentTypeText},
					Required:     true,
				}},
				Outputs: []interfaces.ModelOperationSlot{{
					Name:         "audio",
					ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "speak-task",
			Type:           interfaces.WorkstationTypeInvoke,
			WorkerTypeName: "tts-worker",
			Operation:      "TTS",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
		}},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["speak-task"]
	if tr == nil {
		t.Fatal("expected mapped transition for speak-task")
	}
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:failed" {
		t.Fatalf("model invoke rejection arcs = %+v, want default failed-state routing", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("model invoke failure arcs = %+v, want default failed-state routing", tr.FailureArcs)
	}
}

func TestConfigMapping_ClassifierRoutesBecomeLabeledAcceptedArcs(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "approved", Type: interfaces.StateTypeTerminal},
				{Name: "review", Type: interfaces.StateTypeProcessing},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "classifier"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "classify-task",
			Type:           interfaces.WorkstationTypeClassify,
			WorkerTypeName: "classifier",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			ClassificationRoutes: []interfaces.ClassificationRouteConfig{
				{Label: "approved", Outputs: []interfaces.IOConfig{{StateName: "approved", WorkTypeName: "task"}}},
				{Label: "needs_review", Outputs: []interfaces.IOConfig{{StateName: "review", WorkTypeName: "task"}}},
			},
			OnFailure: []interfaces.IOConfig{{StateName: "failed", WorkTypeName: "task"}},
		}},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["classify-task"]
	if tr == nil {
		t.Fatal("expected mapped transition for classify-task")
	}
	if len(tr.OutputArcs) != 2 {
		t.Fatalf("classifier output arcs = %d, want 2", len(tr.OutputArcs))
	}
	if tr.OutputArcs[0].PlaceID != "task:approved" || tr.OutputArcs[0].ClassificationLabel != "approved" {
		t.Fatalf("first classifier output arc = %#v, want approved route metadata", tr.OutputArcs[0])
	}
	if tr.OutputArcs[1].PlaceID != "task:review" || tr.OutputArcs[1].ClassificationLabel != "needs_review" {
		t.Fatalf("second classifier output arc = %#v, want needs_review route metadata", tr.OutputArcs[1])
	}
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:failed" {
		t.Fatalf("classifier rejection arcs = %#v, want default failure routing only", tr.RejectionArcs)
	}
}

func TestConfigMapping_ClassifierWithoutOnFailureGetsImplicitFailureArc(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "approved", Type: interfaces.StateTypeTerminal},
				{Name: "review", Type: interfaces.StateTypeProcessing},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "classifier"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "classify-task",
			Type:           interfaces.WorkstationTypeClassify,
			WorkerTypeName: "classifier",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			ClassificationRoutes: []interfaces.ClassificationRouteConfig{
				{Label: "approved", Outputs: []interfaces.IOConfig{{StateName: "approved", WorkTypeName: "task"}}},
				{Label: "needs_review", Outputs: []interfaces.IOConfig{{StateName: "review", WorkTypeName: "task"}}},
			},
		}},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["classify-task"]
	if tr == nil {
		t.Fatal("expected mapped transition for classify-task")
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("classifier failure arcs = %#v, want implicit failed-state routing", tr.FailureArcs)
	}
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:failed" {
		t.Fatalf("classifier rejection arcs = %#v, want implicit failed-state routing", tr.RejectionArcs)
	}
}

func TestConfigMapping_UsesEffectiveRuntimeConfigWorkstationKindsForNormalization(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"name":     "retry-task",
			"behavior": "REPEATER",
			"worker":   "executor",
			"inputs":   []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "executor", `---
type: MODEL_WORKER
modelProvider: openai
model: gpt-5.4
---
Execute work.
`)
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "retry-task", `---
type: MODEL_WORKSTATION
worker: executor
---
Retry work.
`)

	payload, err := os.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read Factory config: %v", err)
	}
	config, err := factorymapping.NewFactoryConfigMapper().Expand(payload)
	if err != nil {
		t.Fatalf("expand Factory config: %v", err)
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["retry-task"]
	if tr == nil {
		t.Fatal("expected mapped transition for retry-task")
	}
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:init" {
		t.Fatalf("effective runtime repeater should reject back to task:init, got %+v", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("effective runtime repeater should still fail through task:failed, got %+v", tr.FailureArcs)
	}
}

func TestConfigMapping_AuthoredWorkstationTransitionsUseMatchingIDAndName(t *testing.T) {
	t.Parallel()

	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "review", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "reviewer"},
			{Name: "publisher"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "review-task",
				WorkerTypeName: "reviewer",
				Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
				Outputs:        []interfaces.IOConfig{{StateName: "review", WorkTypeName: "task"}},
			},
			{
				Name:           "publish-task",
				WorkerTypeName: "publisher",
				Inputs:         []interfaces.IOConfig{{StateName: "review", WorkTypeName: "task"}},
				Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
				OnFailure:      []interfaces.IOConfig{{StateName: "failed", WorkTypeName: "task"}},
			},
		},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, workstationName := range []string{"review-task", "publish-task"} {
		transition := net.Transitions[workstationName]
		if transition == nil {
			t.Fatalf("expected transition %q to exist", workstationName)
		}
		if transition.ID != workstationName {
			t.Fatalf("transition %q ID = %q, want %q", workstationName, transition.ID, workstationName)
		}
		if transition.Name != workstationName {
			t.Fatalf("transition %q Name = %q, want %q", workstationName, transition.Name, workstationName)
		}
		if transition.ID != transition.Name {
			t.Fatalf("transition %q invariant failed: ID %q != Name %q", workstationName, transition.ID, transition.Name)
		}
	}
}

func TestConfigMapping_DefaultNonRepeaterFanInRejectionUsesFailureDestinations(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "fan-in",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{StateName: "ready", WorkTypeName: "page"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
					{StateName: "complete", WorkTypeName: "page"},
				},
			},
		},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["fan-in"]
	if tr == nil {
		t.Fatal("expected mapped transition for fan-in")
	}
	if len(tr.FailureArcs) != 2 {
		t.Fatalf("expected default failure routing to both failed states, got %+v", tr.FailureArcs)
	}
	assertTransitionArcPlaces(t, tr.FailureArcs, "task:failed", "page:failed")
	if len(tr.RejectionArcs) != 2 {
		t.Fatalf("expected default rejection routing to clone failure destinations, got %+v", tr.RejectionArcs)
	}
	assertTransitionArcPlaces(t, tr.RejectionArcs, "task:failed", "page:failed")
}

func assertTransitionArcPlaces(t *testing.T, arcs []factoryruntime.PetriArc, wantPlaces ...string) {
	t.Helper()

	places := make(map[string]struct{}, len(arcs))
	for _, arc := range arcs {
		places[arc.PlaceID] = struct{}{}
	}
	for _, want := range wantPlaces {
		if _, ok := places[want]; !ok {
			t.Fatalf("arc places = %+v, want %s destination", arcs, want)
		}
	}
}
