package maptests

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

const dynamicFanoutSpawnedByWorkstation = "parser"

func dynamicFanoutFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "chapter",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "processing", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []workerconfig.Config{
			{Name: "parse-worker"},
			{Name: "complete-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           dynamicFanoutSpawnedByWorkstation,
				WorkerTypeName: "parse-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "chapter"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "processing", WorkTypeName: "chapter"},
				},
			},
			{
				Name:           "chapter-complete",
				WorkerTypeName: "complete-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "processing", WorkTypeName: "chapter"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "chapter",
							SpawnedBy:   dynamicFanoutSpawnedByWorkstation,
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "chapter"},
				},
			},
		},
	}
}

func assertDynamicFanoutTransition(t *testing.T, outputNet *state.Net) {
	t.Helper()

	assertDynamicFanoutTopology(t, outputNet)

	tr := outputNet.Transitions["chapter-complete"]
	if tr == nil {
		t.Fatal("expected transition 'chapter-complete' to exist")
	}
	if len(tr.InputArcs) != 3 {
		t.Fatalf("expected 3 input arcs, got %d", len(tr.InputArcs))
	}
	assertDynamicFanoutParentArc(t, tr.InputArcs[0])
	assertDynamicFanoutCountArc(t, tr.InputArcs[1])
	assertDynamicFanoutChildArc(t, tr.InputArcs[2])
	assertDynamicFanoutSpawnedBy(t, outputNet)
}

func assertDynamicFanoutTopology(t *testing.T, outputNet *state.Net) {
	t.Helper()

	if outputNet.Transitions[dynamicFanoutSpawnedByWorkstation] == nil {
		t.Fatalf("expected transition %q to exist for spawned-by fanout source", dynamicFanoutSpawnedByWorkstation)
	}
}

func assertDynamicFanoutSpawnedBy(t *testing.T, outputNet *state.Net) {
	t.Helper()

	if outputNet.FanoutGroups == nil {
		t.Fatal("expected FanoutGroups to be set for spawned-by fanout tracking")
	}
	countPlaceID, ok := outputNet.FanoutGroups[dynamicFanoutSpawnedByWorkstation]
	if !ok {
		t.Fatalf("expected FanoutGroups to include spawned-by workstation %q", dynamicFanoutSpawnedByWorkstation)
	}
	expectedCountPlaceID := dynamicFanoutSpawnedByWorkstation + ":fanout-count"
	if countPlaceID != expectedCountPlaceID {
		t.Errorf("FanoutGroups[%q]: expected %q, got %q", dynamicFanoutSpawnedByWorkstation, expectedCountPlaceID, countPlaceID)
	}
	if outputNet.Places[expectedCountPlaceID] == nil {
		t.Fatalf("expected fanout count place %q to exist for spawned-by workstation %q", expectedCountPlaceID, dynamicFanoutSpawnedByWorkstation)
	}
}

func assertDynamicFanoutParentArc(t *testing.T, parentArc petri.Arc) {
	t.Helper()

	if parentArc.Name != "parent" {
		t.Errorf("arc[0] name: expected 'parent', got %q", parentArc.Name)
	}
	if parentArc.PlaceID != "chapter:processing" {
		t.Errorf("arc[0] place: expected 'chapter:processing', got %q", parentArc.PlaceID)
	}
}

func assertDynamicFanoutCountArc(t *testing.T, countArc petri.Arc) {
	t.Helper()

	if countArc.Name != "fanout-count" {
		t.Errorf("arc[1] name: expected 'fanout-count', got %q", countArc.Name)
	}
	expectedCountPlaceID := dynamicFanoutSpawnedByWorkstation + ":fanout-count"
	if countArc.PlaceID != expectedCountPlaceID {
		t.Errorf("arc[1] place: expected %q, got %q", expectedCountPlaceID, countArc.PlaceID)
	}
	if countArc.Mode != interfaces.ArcModeConsume {
		t.Errorf("arc[1] mode: expected CONSUME, got %d", countArc.Mode)
	}

	matchGuard, ok := countArc.Guard.(*petri.MatchColorGuard)
	if !ok {
		t.Fatalf("arc[1] guard: expected MatchColorGuard, got %T", countArc.Guard)
	}
	if matchGuard.Field != "parent_id" {
		t.Errorf("spawned-by count guard field: expected 'parent_id', got %q", matchGuard.Field)
	}
	if matchGuard.MatchBinding != "parent" {
		t.Errorf("spawned-by count guard match binding: expected 'parent', got %q", matchGuard.MatchBinding)
	}
	if matchGuard.MatchField != "work_id" {
		t.Errorf("spawned-by count guard match field: expected 'work_id', got %q", matchGuard.MatchField)
	}
}

func assertDynamicFanoutChildArc(t *testing.T, childArc petri.Arc) {
	t.Helper()

	if childArc.PlaceID != "page:complete" {
		t.Errorf("arc[2] place: expected 'page:complete', got %q", childArc.PlaceID)
	}
	if childArc.Mode != interfaces.ArcModeObserve {
		t.Errorf("arc[2] mode: expected OBSERVE, got %d", childArc.Mode)
	}
	if childArc.Cardinality.Mode != petri.CardinalityZeroOrMore {
		t.Errorf("arc[2] cardinality: expected ZERO_OR_MORE, got %d", childArc.Cardinality.Mode)
	}
	fcGuard, ok := childArc.Guard.(*petri.FanoutCountGuard)
	if !ok {
		t.Fatalf("arc[2] guard: expected FanoutCountGuard, got %T", childArc.Guard)
	}
	if fcGuard.MatchBinding != "parent" {
		t.Errorf("fanout guard match binding: expected 'parent', got %q", fcGuard.MatchBinding)
	}
	if fcGuard.CountBinding != "fanout-count" {
		t.Errorf("fanout guard count binding: expected 'fanout-count', got %q", fcGuard.CountBinding)
	}
}
