package validation

import (
	"fmt"

	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
)

const (
	arcSetOutput    = "output"
	arcSetRejection = "rejection"
	arcSetFailure   = "failure"
)

// TypeAlignmentValidator checks that normalized transition arc sets preserve
// same-type work-token counts from consumed inputs to routed outputs.
type TypeAlignmentValidator struct{}

// Validate rejects transitions whose output/rejection/failure arc sets route a
// work type to a different number of same-type arcs than the number of consumed
// inputs for that type. Cross-type fanout remains allowed.
func (tv *TypeAlignmentValidator) Validate(n *state.Net) []Violation {
	if n == nil {
		return nil
	}

	var violations []Violation
	for _, transition := range n.Transitions {
		violations = append(violations, checkTypeAlignment(transition, transition.OutputArcs, n, arcSetOutput)...)
		violations = append(violations, checkTypeAlignment(transition, transition.ContinueArcs, n, arcSetOutput)...)
		violations = append(violations, checkTypeAlignment(transition, transition.RejectionArcs, n, arcSetRejection)...)
		violations = append(violations, checkTypeAlignment(transition, transition.FailureArcs, n, arcSetFailure)...)
	}

	return violations
}

func checkTypeAlignment(transition *petri.Transition, arcs []petri.Arc, net *state.Net, arcSet string) []Violation {
	if transition == nil || len(arcs) == 0 || net == nil {
		return nil
	}

	inputCounts := countConsumedWorkInputsByType(transition.InputArcs, net)
	if len(inputCounts) == 0 {
		return nil
	}

	outputCounts := countWorkOutputsByType(arcs, net)
	if len(outputCounts) == 0 {
		return nil
	}

	var violations []Violation
	for typeID, inputCount := range inputCounts {
		outputCount, exists := outputCounts[typeID]
		if !exists || inputCount == outputCount {
			continue
		}

		violations = append(violations, Violation{
			Level:    ViolationError,
			Code:     "TYPE_COUNT_COLLISION",
			Message:  fmt.Sprintf("transition %q has a type-count collision on %s arcs for work type %q: %d consumed input(s) but %d routed arc(s)", transition.ID, arcSet, typeID, inputCount, outputCount),
			Location: fmt.Sprintf("transition:%s.%s_arcs:%s", transition.ID, arcSet, typeID),
		})
	}

	return violations
}

func countConsumedWorkInputsByType(inputArcs []petri.Arc, net *state.Net) map[string]int {
	counts := make(map[string]int)
	for _, arc := range inputArcs {
		if arc.Mode == interfaces.ArcModeObserve {
			continue
		}

		place, ok := net.Places[arc.PlaceID]
		if !ok {
			continue
		}
		if _, isWorkType := net.WorkTypes[place.TypeID]; !isWorkType {
			continue
		}

		counts[place.TypeID] += consumedInputMultiplicity(arc.Cardinality)
	}
	return counts
}

func consumedInputMultiplicity(cardinality petri.ArcCardinality) int {
	if cardinality.Mode == petri.CardinalityN && cardinality.Count > 0 {
		return cardinality.Count
	}
	return 1
}

func countWorkOutputsByType(arcs []petri.Arc, net *state.Net) map[string]int {
	counts := make(map[string]int)
	for _, arc := range arcs {
		place, ok := net.Places[arc.PlaceID]
		if !ok {
			continue
		}
		if _, isWorkType := net.WorkTypes[place.TypeID]; !isWorkType {
			continue
		}

		counts[place.TypeID]++
	}
	return counts
}
