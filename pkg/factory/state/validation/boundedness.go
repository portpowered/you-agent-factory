package validation

import (
	"fmt"

	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
)

// BoundednessValidator checks that resource places have finite capacity and
// that resource arcs form balanced consume/return pairs across transitions.
type BoundednessValidator struct{}

// Validate checks resource boundedness constraints.
func (bv *BoundednessValidator) Validate(n *state.Net) []Violation {
	var violations []Violation

	// Build set of resource place IDs for quick lookup.
	resourcePlaces := map[string]*state.ResourceDef{}
	for _, r := range n.Resources {
		placeID := fmt.Sprintf("%s:%s", r.ID, interfaces.ResourceStateAvailable)
		resourcePlaces[placeID] = r
	}

	// Check that every resource has finite capacity (> 0).
	for _, r := range n.Resources {
		if r.Capacity <= 0 {
			violations = append(violations, Violation{
				Level:    ViolationWarning,
				Code:     "RESOURCE_ZERO_CAPACITY",
				Message:  fmt.Sprintf("resource %q has capacity %d — no tokens will be available", r.ID, r.Capacity),
				Location: fmt.Sprintf("resource:%s", r.ID),
			})
		}
	}

	// For each transition, check that resource arcs form consume/return pairs.
	// A resource place consumed by an input arc should be returned by an output arc
	// (on at least one outcome path).
	for _, t := range n.Transitions {
		consumed := resourceInputPlaces(t.InputArcs, resourcePlaces)
		returned := resourceOutputPlaces(t, resourcePlaces)

		for placeID := range consumed {
			if !returned[placeID] {
				violations = append(violations, Violation{
					Level:    ViolationWarning,
					Code:     "RESOURCE_ARC_UNBALANCED",
					Message:  fmt.Sprintf("transition %q consumes resource place %q but never returns it", t.ID, placeID),
					Location: fmt.Sprintf("transition:%s.resource:%s", t.ID, placeID),
				})
			}
		}
	}

	return violations
}

// resourceInputPlaces returns resource place IDs consumed by input arcs.
func resourceInputPlaces(arcs []petri.Arc, resourcePlaces map[string]*state.ResourceDef) map[string]bool {
	consumed := map[string]bool{}
	for _, a := range arcs {
		if _, isResource := resourcePlaces[a.PlaceID]; isResource {
			if a.Mode != interfaces.ArcModeObserve {
				consumed[a.PlaceID] = true
			}
		}
	}
	return consumed
}

// resourceOutputPlaces returns resource place IDs produced by any output arc set
// (success, continue, rejection, or failure).
func resourceOutputPlaces(t *petri.Transition, resourcePlaces map[string]*state.ResourceDef) map[string]bool {
	returned := map[string]bool{}
	for _, arcSet := range [][]petri.Arc{t.OutputArcs, t.ContinueArcs, t.RejectionArcs, t.FailureArcs} {
		for _, a := range arcSet {
			if _, isResource := resourcePlaces[a.PlaceID]; isResource {
				returned[a.PlaceID] = true
			}
		}
	}
	return returned
}
