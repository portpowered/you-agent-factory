package analyzers

import (
	"fmt"
	"slices"
	"strings"
)

// backEdge is one arc of the cut set: an import direction that has to be
// reversed or removed for the service graph to become acyclic.
type backEdge struct {
	from     string
	to       string
	weight   int
	carriers []string
}

// cutSet returns the arcs that point backwards in the supplied ordering,
// which is exactly the minimum feedback arc set the solver selected. The
// result is ordered heaviest-first so the most valuable cut leads the report.
func (graph *serviceGraph) cutSet(ordering []int) []backEdge {
	positions := orderedPositions(ordering)
	index := make(map[string]int, len(graph.services))
	for position, service := range graph.services {
		index[service] = position
	}

	var edges []backEdge
	for edge, weight := range graph.weights {
		from, fromKnown := index[edge.from]
		to, toKnown := index[edge.to]
		if !fromKnown || !toKnown || weight <= 0 {
			continue
		}
		if positions[from] <= positions[to] {
			continue
		}
		edges = append(edges, backEdge{
			from:     edge.from,
			to:       edge.to,
			weight:   weight,
			carriers: graph.carriers[edge],
		})
	}
	slices.SortFunc(edges, func(left, right backEdge) int {
		if left.weight != right.weight {
			return right.weight - left.weight
		}
		if byFrom := strings.Compare(left.from, right.from); byFrom != 0 {
			return byFrom
		}
		return strings.Compare(left.to, right.to)
	})
	return edges
}

func serviceCycleDiagnostic(graph *serviceGraph, head string) error {
	ceiling, present, err := serviceCycleCeiling(head)
	if err != nil {
		return err
	}
	if len(graph.services) == 0 {
		if present {
			return fmt.Errorf("stale service-cycle-weight: no service source owners remain")
		}
		return nil
	}
	if !present {
		return fmt.Errorf("missing service-cycle-weight singleton in %s", baselinePath)
	}
	solution, err := minimumFeedbackArcSet(graph.matrix())
	if err != nil {
		return err
	}
	if solution.weight == ceiling {
		return nil
	}
	direction := "regression"
	if solution.weight < ceiling {
		direction = "uncaptured improvement; lower the ceiling"
	}
	message := fmt.Sprintf("%s: measured %d, ceiling %d, drift %+d; never raise the ceiling", direction, solution.weight, ceiling, solution.weight-ceiling)
	for _, edge := range graph.cutSet(solution.ordering) {
		message += fmt.Sprintf("\n%s -> %s (weight %d); carrier packages: %s", edge.from, edge.to, edge.weight, strings.Join(edge.carriers, ", "))
	}
	return fmt.Errorf("%s", message)
}
