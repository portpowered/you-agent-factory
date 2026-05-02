package validation

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// CompletenessValidator checks that every arc on every transition references
// an existing place in the net.
type CompletenessValidator struct{}

// Validate checks that all arc place references resolve to places in the net.
func (cv *CompletenessValidator) Validate(n *state.Net) []Violation {
	var violations []Violation

	for _, t := range n.Transitions {
		violations = append(violations, checkArcs(n, t.ID, "input_arcs", t.InputArcs)...)
		violations = append(violations, checkArcs(n, t.ID, "output_arcs", t.OutputArcs)...)
		violations = append(violations, checkArcs(n, t.ID, "continue_arcs", t.ContinueArcs)...)
		violations = append(violations, checkArcs(n, t.ID, "rejection_arcs", t.RejectionArcs)...)
		violations = append(violations, checkArcs(n, t.ID, "failure_arcs", t.FailureArcs)...)
	}

	return violations
}

// checkArcs validates that each arc's PlaceID exists in the net's Places map.
func checkArcs(n *state.Net, transitionID, arcSet string, arcs []petri.Arc) []Violation {
	var violations []Violation
	for _, a := range arcs {
		if _, exists := n.Places[a.PlaceID]; !exists {
			violations = append(violations, Violation{
				Level:    ViolationError,
				Code:     "ARC_MISSING_PLACE",
				Message:  fmt.Sprintf("arc %q references non-existent place %q", a.ID, a.PlaceID),
				Location: fmt.Sprintf("transition:%s.%s:%s", transitionID, arcSet, a.ID),
			})
		}
	}
	return violations
}
