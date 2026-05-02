package validation

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// buildValidNet creates a minimal valid net: init → [transition] → complete.
func buildValidNet() *state.Net {
	return &state.Net{
		ID: "test-net",
		Places: map[string]*petri.Place{
			"page:init":     {ID: "page:init", TypeID: "page", State: "init"},
			"page:complete": {ID: "page:complete", TypeID: "page", State: "complete"},
		},
		Transitions: map[string]*petri.Transition{
			"process": {
				ID:   "process",
				Name: "process",
				Type: petri.TransitionNormal,
				InputArcs: []petri.Arc{
					{ID: "in-work", Name: "work", PlaceID: "page:init", Direction: petri.ArcInput},
				},
				OutputArcs: []petri.Arc{
					{ID: "out-work", Name: "work", PlaceID: "page:complete", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"page": {
				ID:   "page",
				Name: "Page",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
	}
}

// --- Reachability tests ---

func TestReachability_ValidNet_NoViolations(t *testing.T) {
	n := buildValidNet()
	rv := &ReachabilityValidator{}
	violations := rv.Validate(n)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

func TestReachability_UnreachableTerminal_Error(t *testing.T) {
	n := buildValidNet()

	// Add a second initial place with no transition leading to a terminal.
	n.Places["page:orphan"] = &petri.Place{ID: "page:orphan", TypeID: "page", State: "orphan"}
	n.WorkTypes["page"].States = append(n.WorkTypes["page"].States,
		state.StateDefinition{Value: "orphan", Category: state.StateCategoryInitial},
	)

	rv := &ReachabilityValidator{}
	violations := rv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Level != ViolationError {
		t.Errorf("expected ERROR, got %s", v.Level)
	}
	if v.Code != "UNREACHABLE_TERMINAL" {
		t.Errorf("expected code UNREACHABLE_TERMINAL, got %s", v.Code)
	}
}

func TestReachability_MultiHop(t *testing.T) {
	// init → processing → complete (two hops)
	n := &state.Net{
		ID: "multi-hop",
		Places: map[string]*petri.Place{
			"page:init":       {ID: "page:init", TypeID: "page", State: "init"},
			"page:processing": {ID: "page:processing", TypeID: "page", State: "processing"},
			"page:complete":   {ID: "page:complete", TypeID: "page", State: "complete"},
		},
		Transitions: map[string]*petri.Transition{
			"step1": {
				ID: "step1",
				InputArcs: []petri.Arc{
					{ID: "s1-in", PlaceID: "page:init"},
				},
				OutputArcs: []petri.Arc{
					{ID: "s1-out", PlaceID: "page:processing"},
				},
			},
			"step2": {
				ID: "step2",
				InputArcs: []petri.Arc{
					{ID: "s2-in", PlaceID: "page:processing"},
				},
				OutputArcs: []petri.Arc{
					{ID: "s2-out", PlaceID: "page:complete"},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"page": {
				ID: "page",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "processing", Category: state.StateCategoryProcessing},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
	}

	rv := &ReachabilityValidator{}
	violations := rv.Validate(n)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for multi-hop net, got %d: %+v", len(violations), violations)
	}
}

func TestReachability_FailedStateCountsAsTerminal(t *testing.T) {
	// init → [process] → failed (FAILED category should count as reachable terminal)
	n := &state.Net{
		ID: "failed-terminal",
		Places: map[string]*petri.Place{
			"page:init":   {ID: "page:init", TypeID: "page", State: "init"},
			"page:failed": {ID: "page:failed", TypeID: "page", State: "failed"},
		},
		Transitions: map[string]*petri.Transition{
			"process": {
				ID: "process",
				InputArcs: []petri.Arc{
					{ID: "in", PlaceID: "page:init"},
				},
				FailureArcs: []petri.Arc{
					{ID: "fail", PlaceID: "page:failed"},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"page": {
				ID: "page",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
	}

	rv := &ReachabilityValidator{}
	violations := rv.Validate(n)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations (FAILED counts as terminal), got %d: %+v", len(violations), violations)
	}
}

// --- Completeness tests ---

func TestCompleteness_ValidNet_NoViolations(t *testing.T) {
	n := buildValidNet()
	cv := &CompletenessValidator{}
	violations := cv.Validate(n)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

func TestCompleteness_MissingInputPlace_Error(t *testing.T) {
	n := buildValidNet()
	// Point input arc at a non-existent place.
	t1 := n.Transitions["process"]
	t1.InputArcs[0].PlaceID = "page:nonexistent"
	n.Transitions["process"] = t1

	cv := &CompletenessValidator{}
	violations := cv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Level != ViolationError {
		t.Errorf("expected ERROR, got %s", v.Level)
	}
	if v.Code != "ARC_MISSING_PLACE" {
		t.Errorf("expected code ARC_MISSING_PLACE, got %s", v.Code)
	}
}

func TestCompleteness_MissingOutputPlace_Error(t *testing.T) {
	n := buildValidNet()
	t1 := n.Transitions["process"]
	t1.OutputArcs[0].PlaceID = "page:ghost"
	n.Transitions["process"] = t1

	cv := &CompletenessValidator{}
	violations := cv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if violations[0].Code != "ARC_MISSING_PLACE" {
		t.Errorf("expected ARC_MISSING_PLACE, got %s", violations[0].Code)
	}
}

func TestCompleteness_MissingRejectionPlace_Error(t *testing.T) {
	n := buildValidNet()
	t1 := n.Transitions["process"]
	t1.RejectionArcs = []petri.Arc{
		{ID: "rej", PlaceID: "page:nowhere"},
	}
	n.Transitions["process"] = t1

	cv := &CompletenessValidator{}
	violations := cv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if violations[0].Code != "ARC_MISSING_PLACE" {
		t.Errorf("expected ARC_MISSING_PLACE, got %s", violations[0].Code)
	}
}

func TestCompleteness_MissingFailurePlace_Error(t *testing.T) {
	n := buildValidNet()
	t1 := n.Transitions["process"]
	t1.FailureArcs = []petri.Arc{
		{ID: "fail", PlaceID: "page:void"},
	}
	n.Transitions["process"] = t1

	cv := &CompletenessValidator{}
	violations := cv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if violations[0].Code != "ARC_MISSING_PLACE" {
		t.Errorf("expected ARC_MISSING_PLACE, got %s", violations[0].Code)
	}
}

// --- CompositeValidator tests ---

// --- Boundedness tests ---

func TestBoundedness_BalancedResource_NoViolations(t *testing.T) {
	n := buildNetWithResource(1)
	bv := &BoundednessValidator{}
	violations := bv.Validate(n)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

func TestBoundedness_UnbalancedResource_Warning(t *testing.T) {
	n := buildNetWithResource(1)

	// Remove the resource return from output arcs (unbalanced).
	t1 := n.Transitions["process"]
	t1.OutputArcs = []petri.Arc{
		{ID: "out-work", Name: "work", PlaceID: "page:complete", Direction: petri.ArcOutput},
	}
	n.Transitions["process"] = t1

	bv := &BoundednessValidator{}
	violations := bv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Level != ViolationWarning {
		t.Errorf("expected WARNING, got %s", v.Level)
	}
	if v.Code != "RESOURCE_ARC_UNBALANCED" {
		t.Errorf("expected RESOURCE_ARC_UNBALANCED, got %s", v.Code)
	}
}

func TestBoundedness_ZeroCapacity_Warning(t *testing.T) {
	n := buildNetWithResource(0)
	bv := &BoundednessValidator{}
	violations := bv.Validate(n)

	hasZeroCap := false
	for _, v := range violations {
		if v.Code == "RESOURCE_ZERO_CAPACITY" {
			hasZeroCap = true
		}
	}
	if !hasZeroCap {
		t.Error("expected RESOURCE_ZERO_CAPACITY violation")
	}
}

func TestBoundedness_ObserveArc_NotConsumed(t *testing.T) {
	// An OBSERVE arc should not count as consuming a resource.
	n := buildNetWithResource(1)
	t1 := n.Transitions["process"]
	// Change resource input arc to OBSERVE mode — no consume, so no return needed.
	t1.InputArcs = []petri.Arc{
		{ID: "in-work", Name: "work", PlaceID: "page:init", Direction: petri.ArcInput},
		{ID: "in-gpu", Name: "gpu", PlaceID: "gpu:available", Direction: petri.ArcInput, Mode: interfaces.ArcModeObserve},
	}
	// Remove resource from output arcs.
	t1.OutputArcs = []petri.Arc{
		{ID: "out-work", Name: "work", PlaceID: "page:complete", Direction: petri.ArcOutput},
	}
	n.Transitions["process"] = t1

	bv := &BoundednessValidator{}
	violations := bv.Validate(n)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for OBSERVE arc, got %d: %+v", len(violations), violations)
	}
}

// --- Type safety tests ---

func TestTypeSafety_ValidGuard_NoViolations(t *testing.T) {
	n := buildNetWithGuard("parent_id", "work_id")
	tv := &TypeSafetyValidator{}
	violations := tv.Validate(n)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

func TestTypeSafety_InvalidGuardField_Error(t *testing.T) {
	n := buildNetWithGuard("nonexistent_field", "work_id")
	tv := &TypeSafetyValidator{}
	violations := tv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Level != ViolationError {
		t.Errorf("expected ERROR, got %s", v.Level)
	}
	if v.Code != "GUARD_INVALID_FIELD" {
		t.Errorf("expected GUARD_INVALID_FIELD, got %s", v.Code)
	}
}

func TestTypeSafety_InvalidGuardMatchField_Error(t *testing.T) {
	n := buildNetWithGuard("parent_id", "bad_field")
	tv := &TypeSafetyValidator{}
	violations := tv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if violations[0].Code != "GUARD_INVALID_FIELD" {
		t.Errorf("expected GUARD_INVALID_FIELD, got %s", violations[0].Code)
	}
}

func TestTypeSafety_DuplicateBindingName_Error(t *testing.T) {
	n := buildValidNet()
	t1 := n.Transitions["process"]
	// Add a second input arc with the same binding name "work".
	t1.InputArcs = append(t1.InputArcs, petri.Arc{
		ID: "in-dup", Name: "work", PlaceID: "page:complete", Direction: petri.ArcInput,
	})
	n.Transitions["process"] = t1

	tv := &TypeSafetyValidator{}
	violations := tv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Level != ViolationError {
		t.Errorf("expected ERROR, got %s", v.Level)
	}
	if v.Code != "DUPLICATE_BINDING_NAME" {
		t.Errorf("expected DUPLICATE_BINDING_NAME, got %s", v.Code)
	}
}

func TestTypeSafety_EmptyBindingNames_NoViolation(t *testing.T) {
	n := buildValidNet()
	// Clear binding names — empty names should not trigger duplicate check.
	t1 := n.Transitions["process"]
	t1.InputArcs = []petri.Arc{
		{ID: "in1", Name: "", PlaceID: "page:init", Direction: petri.ArcInput},
		{ID: "in2", Name: "", PlaceID: "page:complete", Direction: petri.ArcInput},
	}
	n.Transitions["process"] = t1

	tv := &TypeSafetyValidator{}
	violations := tv.Validate(n)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for empty names, got %d: %+v", len(violations), violations)
	}
}

func TestTypeAlignment_SingleInputWithTwoSameTypeOutputs_Error(t *testing.T) {
	n := buildValidNet()
	transition := n.Transitions["process"]
	transition.OutputArcs = []petri.Arc{
		{ID: "out-work-1", PlaceID: "page:complete", Direction: petri.ArcOutput},
		{ID: "out-work-2", PlaceID: "page:review", Direction: petri.ArcOutput},
	}
	n.Places["page:review"] = &petri.Place{ID: "page:review", TypeID: "page", State: "review"}
	n.WorkTypes["page"].States = append(n.WorkTypes["page"].States,
		state.StateDefinition{Value: "review", Category: state.StateCategoryProcessing},
	)

	tv := &TypeAlignmentValidator{}
	violations := tv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if violations[0].Code != "TYPE_COUNT_COLLISION" {
		t.Fatalf("expected TYPE_COUNT_COLLISION, got %s", violations[0].Code)
	}
}

func TestTypeAlignment_MultiInputSameTypeMismatchedFailureRouting_Error(t *testing.T) {
	n := buildValidNet()
	transition := n.Transitions["process"]
	transition.InputArcs = []petri.Arc{
		{ID: "in-work-1", PlaceID: "page:init", Direction: petri.ArcInput},
		{ID: "in-work-2", PlaceID: "page:review", Direction: petri.ArcInput},
	}
	transition.OutputArcs = []petri.Arc{
		{ID: "out-work-1", PlaceID: "page:review", Direction: petri.ArcOutput},
		{ID: "out-work-2", PlaceID: "page:complete", Direction: petri.ArcOutput},
	}
	transition.FailureArcs = []petri.Arc{
		{ID: "fail-work", PlaceID: "page:failed", Direction: petri.ArcOutput},
	}
	n.Places["page:review"] = &petri.Place{ID: "page:review", TypeID: "page", State: "review"}
	n.Places["page:failed"] = &petri.Place{ID: "page:failed", TypeID: "page", State: "failed"}
	n.WorkTypes["page"].States = append(n.WorkTypes["page"].States,
		state.StateDefinition{Value: "review", Category: state.StateCategoryProcessing},
		state.StateDefinition{Value: "failed", Category: state.StateCategoryFailed},
	)

	tv := &TypeAlignmentValidator{}
	violations := tv.Validate(n)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if violations[0].Code != "TYPE_COUNT_COLLISION" {
		t.Fatalf("expected TYPE_COUNT_COLLISION, got %s", violations[0].Code)
	}
}

func TestTypeAlignment_CrossTypeFanout_NoViolation(t *testing.T) {
	n := &state.Net{
		ID: "cross-type-fanout",
		Places: map[string]*petri.Place{
			"task:init":      {ID: "task:init", TypeID: "task", State: "init"},
			"page:complete":  {ID: "page:complete", TypeID: "page", State: "complete"},
			"asset:complete": {ID: "asset:complete", TypeID: "asset", State: "complete"},
		},
		Transitions: map[string]*petri.Transition{
			"fanout": {
				ID: "fanout",
				InputArcs: []petri.Arc{
					{ID: "in-task", PlaceID: "task:init", Direction: petri.ArcInput},
				},
				OutputArcs: []petri.Arc{
					{ID: "out-page", PlaceID: "page:complete", Direction: petri.ArcOutput},
					{ID: "out-asset", PlaceID: "asset:complete", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
				},
			},
			"page": {
				ID: "page",
				States: []state.StateDefinition{
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
			"asset": {
				ID: "asset",
				States: []state.StateDefinition{
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
	}

	tv := &TypeAlignmentValidator{}
	violations := tv.Validate(n)

	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

// --- CompositeValidator tests ---

func TestCompositeValidator_CombinesViolations(t *testing.T) {
	n := buildValidNet()

	// Break both reachability and completeness.
	n.Places["page:orphan"] = &petri.Place{ID: "page:orphan", TypeID: "page", State: "orphan"}
	n.WorkTypes["page"].States = append(n.WorkTypes["page"].States,
		state.StateDefinition{Value: "orphan", Category: state.StateCategoryInitial},
	)
	t1 := n.Transitions["process"]
	t1.OutputArcs[0].PlaceID = "page:ghost"
	n.Transitions["process"] = t1

	cv := NewCompositeValidator(&ReachabilityValidator{}, &CompletenessValidator{})
	violations := cv.Validate(n)

	// At least one from each validator.
	hasReachability := false
	hasCompleteness := false
	for _, v := range violations {
		switch v.Code {
		case "UNREACHABLE_TERMINAL":
			hasReachability = true
		case "ARC_MISSING_PLACE":
			hasCompleteness = true
		}
	}
	if !hasReachability {
		t.Error("expected UNREACHABLE_TERMINAL violation from composite")
	}
	if !hasCompleteness {
		t.Error("expected ARC_MISSING_PLACE violation from composite")
	}
}

// --- Test helpers ---

// buildNetWithResource creates a net with a resource "gpu" that is consumed
// by the process transition and returned on the output arc.
func buildNetWithResource(capacity int) *state.Net {
	n := buildValidNet()
	n.Resources = map[string]*state.ResourceDef{
		"gpu": {ID: "gpu", Name: "GPU", Capacity: capacity},
	}
	n.Places["gpu:available"] = &petri.Place{ID: "gpu:available", TypeID: "gpu", State: "available"}

	t1 := n.Transitions["process"]
	t1.InputArcs = append(t1.InputArcs, petri.Arc{
		ID: "in-gpu", Name: "gpu", PlaceID: "gpu:available", Direction: petri.ArcInput,
	})
	t1.OutputArcs = append(t1.OutputArcs, petri.Arc{
		ID: "out-gpu", Name: "gpu-return", PlaceID: "gpu:available", Direction: petri.ArcOutput,
	})
	n.Transitions["process"] = t1

	return n
}

// buildNetWithGuard creates a net with a MatchColorGuard on the second input arc.
func buildNetWithGuard(field, matchField string) *state.Net {
	return &state.Net{
		ID: "guarded-net",
		Places: map[string]*petri.Place{
			"page:init":     {ID: "page:init", TypeID: "page", State: "init"},
			"review:ready":  {ID: "review:ready", TypeID: "review", State: "ready"},
			"page:complete": {ID: "page:complete", TypeID: "page", State: "complete"},
		},
		Transitions: map[string]*petri.Transition{
			"review": {
				ID:   "review",
				Name: "review",
				Type: petri.TransitionNormal,
				InputArcs: []petri.Arc{
					{ID: "in-work", Name: "work", PlaceID: "page:init", Direction: petri.ArcInput},
					{
						ID: "in-review", Name: "review", PlaceID: "review:ready", Direction: petri.ArcInput,
						Guard: &petri.MatchColorGuard{
							Field:        field,
							MatchBinding: "work",
							MatchField:   matchField,
						},
					},
				},
				OutputArcs: []petri.Arc{
					{ID: "out-work", Name: "work", PlaceID: "page:complete", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"page": {
				ID: "page", Name: "Page",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
			"review": {
				ID: "review", Name: "Review",
				States: []state.StateDefinition{
					{Value: "ready", Category: state.StateCategoryInitial},
				},
			},
		},
	}
}
