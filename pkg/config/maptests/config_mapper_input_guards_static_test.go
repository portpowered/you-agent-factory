package maptests

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func staticAllChildrenCompleteFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "request",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "waiting", Type: interfaces.StateTypeProcessing},
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
			{Name: "collect-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "collector",
				WorkerTypeName: "collect-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "waiting", WorkTypeName: "request"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "request",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "request"},
				},
			},
		},
	}
}

func assertStaticAllChildrenCompleteCollector(t *testing.T, outputNet *state.Net) {
	t.Helper()

	collector := outputNet.Transitions["collector"]
	if collector == nil {
		t.Fatal("expected transition 'collector' to exist")
	}
	if len(collector.InputArcs) != 2 {
		t.Fatalf("expected 2 input arcs on collector, got %d", len(collector.InputArcs))
	}

	assertStaticAllChildrenCompleteParentArc(t, collector.InputArcs[0])
	assertStaticAllChildrenCompleteChildArc(t, collector.InputArcs[1])
}

func assertStaticAllChildrenCompleteParentArc(t *testing.T, parentArc petri.Arc) {
	t.Helper()

	if parentArc.Name != "parent" {
		t.Errorf("first input arc name: expected 'parent', got %q", parentArc.Name)
	}
	if parentArc.PlaceID != "request:waiting" {
		t.Errorf("parent arc place: expected 'request:waiting', got %q", parentArc.PlaceID)
	}
}

func assertStaticAllChildrenCompleteChildArc(t *testing.T, childArc petri.Arc) {
	t.Helper()

	if childArc.PlaceID != "page:complete" {
		t.Errorf("child arc place: expected 'page:complete', got %q", childArc.PlaceID)
	}
	if childArc.Mode != interfaces.ArcModeObserve {
		t.Errorf("child arc mode: expected OBSERVE, got %d", childArc.Mode)
	}
	if childArc.Cardinality.Mode != petri.CardinalityAll {
		t.Errorf("child arc cardinality: expected ALL, got %d", childArc.Cardinality.Mode)
	}

	guard, ok := childArc.Guard.(*petri.AllWithParentGuard)
	if !ok {
		t.Fatalf("expected AllWithParentGuard on child arc, got %T", childArc.Guard)
	}
	if guard.MatchBinding != "parent" {
		t.Errorf("guard match binding: expected 'parent', got %q", guard.MatchBinding)
	}
}
