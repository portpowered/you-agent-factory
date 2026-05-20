package config

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// --- per-input guard tests ---

// portos:func-length-exception owner=agent-factory reason=legacy-input-guard-fixture review=2026-07-18 removal=split-static-guard-fixture-before-next-input-guard-change
func TestConfigMapping_PerInputGuard_StaticAllChildrenComplete(t *testing.T) {
	input := &interfaces.FactoryConfig{
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
		Workers: []interfaces.WorkerConfig{
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

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	collector := outputNet.Transitions["collector"]
	if collector == nil {
		t.Fatal("expected transition 'collector' to exist")
	}

	// Should have 2 input arcs: parent consume + child observe.
	if len(collector.InputArcs) != 2 {
		t.Fatalf("expected 2 input arcs on collector, got %d", len(collector.InputArcs))
	}

	// First arc: parent consume with named binding "parent".
	parentArc := collector.InputArcs[0]
	if parentArc.Name != "parent" {
		t.Errorf("first input arc name: expected 'parent', got %q", parentArc.Name)
	}
	if parentArc.PlaceID != "request:waiting" {
		t.Errorf("parent arc place: expected 'request:waiting', got %q", parentArc.PlaceID)
	}

	// Second arc: child observation with AllWithParentGuard.
	childArc := collector.InputArcs[1]
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

// portos:func-length-exception owner=agent-factory reason=legacy-input-guard-fixture review=2026-07-18 removal=split-dynamic-guard-fixture-before-next-input-guard-change
func TestConfigMapping_PerInputGuard_DynamicFanout(t *testing.T) {
	input := &interfaces.FactoryConfig{
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
		Workers: []interfaces.WorkerConfig{
			{Name: "parse-worker"},
			{Name: "complete-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "parser",
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
							SpawnedBy:   "parser",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "chapter"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	assertDynamicFanoutTransition(t, outputNet)
}

func TestConfigMapping_PerInputGuard_AnyChildFailed(t *testing.T) {
	input := &interfaces.FactoryConfig{
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
		Workers: []interfaces.WorkerConfig{
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

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	tr := outputNet.Transitions["failure-checker"]
	if tr == nil {
		t.Fatal("expected transition 'failure-checker' to exist")
	}

	if len(tr.InputArcs) != 2 {
		t.Fatalf("expected 2 input arcs, got %d", len(tr.InputArcs))
	}

	childArc := tr.InputArcs[1]
	if childArc.Mode != interfaces.ArcModeObserve {
		t.Errorf("child arc mode: expected OBSERVE, got %d", childArc.Mode)
	}
	if childArc.Cardinality.Mode != petri.CardinalityOne {
		t.Errorf("child arc cardinality: expected ONE, got %d", childArc.Cardinality.Mode)
	}

	guard, ok := childArc.Guard.(*petri.AnyWithParentGuard)
	if !ok {
		t.Fatalf("expected AnyWithParentGuard, got %T", childArc.Guard)
	}
	if guard.MatchBinding != "parent" {
		t.Errorf("guard match binding: expected 'parent', got %q", guard.MatchBinding)
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsMissingParentInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type: interfaces.GuardTypeAllChildrenComplete,
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard missing parent_input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSelfReference(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "bad-guard",
				Inputs: []interfaces.IOConfig{
					{
						StateName:    "init",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "page", // Self-reference.
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "page"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard referencing its own input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsInvalidParentInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "nonexistent",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard referencing non-existent parent input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsInvalidSpawnedBy(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "task",
							SpawnedBy:   "nonexistent-workstation",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard referencing non-existent spawned_by workstation")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsUnsupportedType(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeVisitCount,
							ParentInput: "task",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for unsupported per-input guard type")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSameNameMissingMatchInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type: interfaces.GuardTypeSameName,
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-name guard missing match_input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSameNameSelfReference(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{
						StateName:    "ready",
						WorkTypeName: "plan",
					},
					{
						StateName:    "ready",
						WorkTypeName: "task",
					},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameName,
							MatchInput: "task",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-name guard referencing its own input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSameNameUnknownMatchInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameName,
							MatchInput: "other",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-name guard referencing non-existent input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSameTraceIDMissingMatchInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type: interfaces.GuardTypeSameTraceID,
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-trace guard missing match_input")
	}
	if !strings.Contains(err.Error(), "[per-input-guard-same-trace-match-input]") {
		t.Fatalf("expected same-trace-specific validation rule in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `workstations[0](match-items).inputs[1].guard`) {
		t.Fatalf("expected input guard path in error, got %v", err)
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSameTraceIDSelfReference(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{
						StateName:    "ready",
						WorkTypeName: "plan",
					},
					{
						StateName:    "ready",
						WorkTypeName: "task",
					},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameTraceID,
							MatchInput: "task",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-trace guard referencing its own input")
	}
	if !strings.Contains(err.Error(), "[per-input-guard-same-trace-self-ref]") {
		t.Fatalf("expected same-trace self-reference validation rule in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `workstations[0](match-items).inputs[2].guard`) {
		t.Fatalf("expected input guard path in error, got %v", err)
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSameTraceIDUnknownMatchInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameTraceID,
							MatchInput: "other",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-trace guard referencing non-existent input")
	}
	if !strings.Contains(err.Error(), "[per-input-guard-same-trace-match-input]") {
		t.Fatalf("expected same-trace-specific validation rule in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `workstations[0](match-items).inputs[1].guard`) {
		t.Fatalf("expected input guard path in error, got %v", err)
	}
}

func TestConfigMapping_PerInputGuard_SameNameBuildsConsumeGuardAgainstPeerInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameName,
							MatchInput: "plan",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	transition := net.Transitions["match-items"]
	if transition == nil {
		t.Fatal("expected match-items transition")
	}

	var planArc *petri.Arc
	var taskArc *petri.Arc
	for i := range transition.InputArcs {
		arc := &transition.InputArcs[i]
		switch arc.PlaceID {
		case "plan:ready":
			planArc = arc
		case "task:ready":
			taskArc = arc
		}
	}

	if planArc == nil || taskArc == nil {
		t.Fatalf("expected plan/task input arcs, got %#v", transition.InputArcs)
	}
	if taskArc.Mode != interfaces.ArcModeConsume {
		t.Fatalf("same-name guarded arc mode = %v, want consume", taskArc.Mode)
	}
	if taskArc.Cardinality.Mode != petri.CardinalityOne {
		t.Fatalf("same-name guarded arc cardinality = %v, want one", taskArc.Cardinality.Mode)
	}
	guard, ok := taskArc.Guard.(*petri.SameNameGuard)
	if !ok {
		t.Fatalf("same-name guarded arc guard = %T, want *petri.SameNameGuard", taskArc.Guard)
	}
	if guard.MatchBinding != planArc.Name {
		t.Fatalf("same-name guard binding = %q, want %q", guard.MatchBinding, planArc.Name)
	}
}

func TestConfigMapping_PerInputGuard_SameTraceIDBuildsConsumeGuardAgainstPeerInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameTraceID,
							MatchInput: "plan",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	transition := net.Transitions["match-items"]
	if transition == nil {
		t.Fatal("expected match-items transition")
	}

	var planArc *petri.Arc
	var taskArc *petri.Arc
	for i := range transition.InputArcs {
		arc := &transition.InputArcs[i]
		switch arc.PlaceID {
		case "plan:ready":
			planArc = arc
		case "task:ready":
			taskArc = arc
		}
	}

	if planArc == nil || taskArc == nil {
		t.Fatalf("expected plan/task input arcs, got %#v", transition.InputArcs)
	}
	if taskArc.Mode != interfaces.ArcModeConsume {
		t.Fatalf("same-trace guarded arc mode = %v, want consume", taskArc.Mode)
	}
	if taskArc.Cardinality.Mode != petri.CardinalityOne {
		t.Fatalf("same-trace guarded arc cardinality = %v, want one", taskArc.Cardinality.Mode)
	}
	guard, ok := taskArc.Guard.(*petri.SameTraceIDGuard)
	if !ok {
		t.Fatalf("same-trace guarded arc guard = %T, want *petri.SameTraceIDGuard", taskArc.Guard)
	}
	if guard.MatchBinding != planArc.Name {
		t.Fatalf("same-trace guard binding = %q, want %q", guard.MatchBinding, planArc.Name)
	}
}

func assertDynamicFanoutTransition(t *testing.T, outputNet *state.Net) {
	t.Helper()

	tr := outputNet.Transitions["chapter-complete"]
	if tr == nil {
		t.Fatal("expected transition 'chapter-complete' to exist")
	}
	if len(tr.InputArcs) != 3 {
		t.Fatalf("expected 3 input arcs, got %d", len(tr.InputArcs))
	}

	parentArc := tr.InputArcs[0]
	if parentArc.Name != "parent" {
		t.Errorf("arc[0] name: expected 'parent', got %q", parentArc.Name)
	}
	if parentArc.PlaceID != "chapter:processing" {
		t.Errorf("arc[0] place: expected 'chapter:processing', got %q", parentArc.PlaceID)
	}

	countArc := tr.InputArcs[1]
	if countArc.Name != "fanout-count" {
		t.Errorf("arc[1] name: expected 'fanout-count', got %q", countArc.Name)
	}
	if countArc.PlaceID != "parser:fanout-count" {
		t.Errorf("arc[1] place: expected 'parser:fanout-count', got %q", countArc.PlaceID)
	}
	if countArc.Mode != interfaces.ArcModeConsume {
		t.Errorf("arc[1] mode: expected CONSUME, got %d", countArc.Mode)
	}
	if _, ok := countArc.Guard.(*petri.MatchColorGuard); !ok {
		t.Fatalf("arc[1] guard: expected MatchColorGuard, got %T", countArc.Guard)
	}

	childArc := tr.InputArcs[2]
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

	if outputNet.FanoutGroups == nil {
		t.Fatal("expected FanoutGroups to be set")
	}
	if outputNet.FanoutGroups["parser"] != "parser:fanout-count" {
		t.Errorf("FanoutGroups[parser]: expected 'parser:fanout-count', got %q", outputNet.FanoutGroups["parser"])
	}
}
