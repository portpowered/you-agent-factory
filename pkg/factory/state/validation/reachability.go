package validation

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// ReachabilityValidator checks that every INITIAL place can reach at least one
// TERMINAL place via graph traversal through transitions.
type ReachabilityValidator struct{}

// Validate checks reachability from all INITIAL places to at least one TERMINAL place.
func (rv *ReachabilityValidator) Validate(n *state.Net) []Violation {
	// Collect initial and terminal place IDs from work type definitions.
	initialPlaces := map[string]bool{}
	terminalPlaces := map[string]bool{}
	for _, wt := range n.WorkTypes {
		for _, s := range wt.States {
			pid := state.PlaceID(wt.ID, s.Value)
			switch s.Category {
			case state.StateCategoryInitial:
				initialPlaces[pid] = true
			case state.StateCategoryTerminal, state.StateCategoryFailed:
				terminalPlaces[pid] = true
			}
		}
	}

	// Build adjacency: place → set of places reachable in one transition hop.
	// A place P1 can reach place P2 if there exists a transition T where:
	//   - P1 is referenced by an input arc of T
	//   - P2 is referenced by an output/rejection/failure arc of T
	placeAdj := buildPlaceAdjacency(n)

	var violations []Violation
	for pid := range initialPlaces {
		if !canReachTerminal(pid, terminalPlaces, placeAdj) {
			violations = append(violations, Violation{
				Level:    ViolationError,
				Code:     "UNREACHABLE_TERMINAL",
				Message:  fmt.Sprintf("initial place %q cannot reach any terminal place", pid),
				Location: fmt.Sprintf("place:%s", pid),
			})
		}
	}

	return violations
}

// buildPlaceAdjacency builds a forward adjacency map: place ID → set of place IDs
// reachable through one transition hop.
func buildPlaceAdjacency(n *state.Net) map[string]map[string]bool {
	adj := map[string]map[string]bool{}

	for _, t := range n.Transitions {
		// Collect all input place IDs for this transition.
		inputPlaces := collectPlaceIDs(t.InputArcs)

		// Collect all output place IDs (success + continue + rejection + failure).
		outputPlaces := collectPlaceIDs(t.OutputArcs)
		for pid := range collectPlaceIDs(t.ContinueArcs) {
			outputPlaces[pid] = true
		}
		for pid := range collectPlaceIDs(t.RejectionArcs) {
			outputPlaces[pid] = true
		}
		for pid := range collectPlaceIDs(t.FailureArcs) {
			outputPlaces[pid] = true
		}

		// Each input place can reach each output place via this transition.
		for ip := range inputPlaces {
			if adj[ip] == nil {
				adj[ip] = map[string]bool{}
			}
			for op := range outputPlaces {
				adj[ip][op] = true
			}
		}
	}

	return adj
}

// collectPlaceIDs extracts unique place IDs from a slice of arcs.
func collectPlaceIDs(arcs []petri.Arc) map[string]bool {
	ids := map[string]bool{}
	for _, a := range arcs {
		ids[a.PlaceID] = true
	}
	return ids
}

// canReachTerminal does a BFS from startPlace to see if any terminal place is reachable.
func canReachTerminal(startPlace string, terminalPlaces map[string]bool, adj map[string]map[string]bool) bool {
	if terminalPlaces[startPlace] {
		return true
	}

	visited := map[string]bool{startPlace: true}
	queue := []string{startPlace}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for next := range adj[current] {
			if terminalPlaces[next] {
				return true
			}
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}

	return false
}
