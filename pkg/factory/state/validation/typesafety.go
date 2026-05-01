package validation

import (
	"fmt"

	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/petri"
)

// validColorFields lists the fields available on TokenColor for guard matching.
var validColorFields = map[string]bool{
	"work_id":      true,
	"work_type_id": true,
	"trace_id":     true,
	"parent_id":    true,
}

// TypeSafetyValidator checks that guard expressions reference valid token color
// fields and that arc binding names are unique per transition.
type TypeSafetyValidator struct{}

// Validate checks type safety constraints on guards and arc bindings.
func (tv *TypeSafetyValidator) Validate(n *state.Net) []Violation {
	var violations []Violation

	for _, t := range n.Transitions {
		violations = append(violations, checkBindingUniqueness(t)...)
		violations = append(violations, checkGuardFields(t)...)
	}

	return violations
}

// checkBindingUniqueness ensures arc binding names (Arc.Name) are unique across
// all input arcs of a transition. Duplicate names would cause ambiguous bindings.
func checkBindingUniqueness(t *petri.Transition) []Violation {
	var violations []Violation
	seen := map[string]bool{}

	for _, a := range t.InputArcs {
		if a.Name == "" {
			continue
		}
		if seen[a.Name] {
			violations = append(violations, Violation{
				Level:    ViolationError,
				Code:     "DUPLICATE_BINDING_NAME",
				Message:  fmt.Sprintf("transition %q has duplicate input arc binding name %q", t.ID, a.Name),
				Location: fmt.Sprintf("transition:%s.input_arc:%s", t.ID, a.ID),
			})
		}
		seen[a.Name] = true
	}

	return violations
}

// checkGuardFields validates that MatchColorGuard fields reference valid TokenColor fields.
func checkGuardFields(t *petri.Transition) []Violation {
	var violations []Violation

	allArcs := [][]petri.Arc{t.InputArcs, t.OutputArcs, t.ContinueArcs, t.RejectionArcs, t.FailureArcs}
	for _, arcs := range allArcs {
		for _, a := range arcs {
			if a.Guard == nil {
				continue
			}
			mcg, ok := a.Guard.(*petri.MatchColorGuard)
			if !ok {
				continue
			}
			if !validColorFields[mcg.Field] {
				violations = append(violations, Violation{
					Level:    ViolationError,
					Code:     "GUARD_INVALID_FIELD",
					Message:  fmt.Sprintf("guard on arc %q references invalid token color field %q", a.ID, mcg.Field),
					Location: fmt.Sprintf("transition:%s.arc:%s.guard.field", t.ID, a.ID),
				})
			}
			if !validColorFields[mcg.MatchField] {
				violations = append(violations, Violation{
					Level:    ViolationError,
					Code:     "GUARD_INVALID_FIELD",
					Message:  fmt.Sprintf("guard on arc %q references invalid token color match field %q", a.ID, mcg.MatchField),
					Location: fmt.Sprintf("transition:%s.arc:%s.guard.match_field", t.ID, a.ID),
				})
			}
		}
	}

	return violations
}
