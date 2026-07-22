package maptests

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func anyChildFailedFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "request",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "waiting", Type: interfaces.StateTypeProcessing},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "check-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "failure-checker",
				WorkerTypeName: "check-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "waiting", WorkTypeName: "request"},
					{
						StateName:    "failed",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAnyChildFailed,
							ParentInput: "request",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "failed", WorkTypeName: "request"},
				},
			},
		},
	}
}

func assertAnyChildFailedFailureChecker(t *testing.T, outputNet *factoryruntime.Net) {
	t.Helper()

	failureChecker := outputNet.Transitions["failure-checker"]
	if failureChecker == nil {
		t.Fatal("expected transition 'failure-checker' to exist")
	}
	if len(failureChecker.InputArcs) != 2 {
		t.Fatalf("expected 2 input arcs on failure-checker, got %d", len(failureChecker.InputArcs))
	}

	assertAnyChildFailedParentArc(t, failureChecker.InputArcs[0])
	assertAnyChildFailedChildArc(t, failureChecker.InputArcs[1])
}

func assertAnyChildFailedParentArc(t *testing.T, parentArc factoryruntime.PetriArc) {
	t.Helper()

	if parentArc.Name != "parent" {
		t.Errorf("first input arc name: expected 'parent', got %q", parentArc.Name)
	}
	if parentArc.PlaceID != "request:waiting" {
		t.Errorf("parent arc place: expected 'request:waiting', got %q", parentArc.PlaceID)
	}
}

func assertAnyChildFailedChildArc(t *testing.T, childArc factoryruntime.PetriArc) {
	t.Helper()

	if childArc.PlaceID != "page:failed" {
		t.Errorf("child arc place: expected 'page:failed', got %q", childArc.PlaceID)
	}
	if childArc.Mode != interfaces.ArcModeObserve {
		t.Errorf("child arc mode: expected OBSERVE, got %d", childArc.Mode)
	}
	if childArc.Cardinality.Mode != factoryruntime.PetriCardinalityOne {
		t.Errorf("child arc cardinality: expected ONE, got %d", childArc.Cardinality.Mode)
	}

	guard, ok := childArc.Guard.(*factoryruntime.PetriAnyWithParentGuard)
	if !ok {
		t.Fatalf("expected AnyWithParentGuard on child arc, got %T", childArc.Guard)
	}
	if guard.MatchBinding != "parent" {
		t.Errorf("guard match binding: expected 'parent', got %q", guard.MatchBinding)
	}
}
