package state

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// NormalizeTransitionTopology materializes implicit routing onto transition arc sets.
// Repeater workstations get rejection arcs back to their work-token inputs, and
// transitions without explicit failure arcs get routed to each input work type's
// failed state. Standard workstations without explicit rejection arcs also reject
// into those failed-state arcs. Runtime routing can then use a single outcome ->
// arc-set path.
func NormalizeTransitionTopology(net *Net, workstationKinds map[string]interfaces.WorkstationKind) {
	if net == nil {
		return
	}
	for _, transition := range net.Transitions {
		ensureDefaultFailureArcs(net, transition)
		ensureDefaultRejectionArcs(net, transition, workstationKinds)
	}
}

func ensureDefaultRejectionArcs(net *Net, transition *petri.Transition, workstationKinds map[string]interfaces.WorkstationKind) {
	if transition == nil || len(transition.RejectionArcs) > 0 {
		return
	}

	if transitionWorkstationKind(transition, workstationKinds) == interfaces.WorkstationKindRepeater {
		appendWorkInputArcs(net, transition, &transition.RejectionArcs, "auto-rejection")
		return
	}

	transition.RejectionArcs = cloneArcs(transition.FailureArcs, transition.ID, "auto-rejection")
}

func transitionWorkstationKind(transition *petri.Transition, workstationKinds map[string]interfaces.WorkstationKind) interfaces.WorkstationKind {
	if transition == nil || len(workstationKinds) == 0 {
		return ""
	}
	if transition.Name != "" {
		if kind, ok := workstationKinds[transition.Name]; ok {
			return kind
		}
	}
	if transition.ID == "" || transition.ID == transition.Name {
		return ""
	}
	return workstationKinds[transition.ID]
}

func appendWorkInputArcs(net *Net, transition *petri.Transition, target *[]petri.Arc, suffix string) {
	for _, inputArc := range transition.InputArcs {
		place, ok := net.Places[inputArc.PlaceID]
		if !ok {
			continue
		}
		if _, isWorkType := net.WorkTypes[place.TypeID]; !isWorkType {
			continue
		}
		*target = append(*target, petri.Arc{
			ID:           fmt.Sprintf("%s:%s:%s", transition.ID, suffix, inputArc.PlaceID),
			Name:         fmt.Sprintf("%s:%s:%s", transition.ID, suffix, inputArc.PlaceID),
			PlaceID:      inputArc.PlaceID,
			TransitionID: transition.ID,
			Direction:    petri.ArcOutput,
			Cardinality: petri.ArcCardinality{
				Mode: petri.CardinalityOne,
			},
		})
	}
}

func cloneArcs(arcs []petri.Arc, transitionID string, suffix string) []petri.Arc {
	if len(arcs) == 0 {
		return nil
	}
	cloned := make([]petri.Arc, 0, len(arcs))
	for _, arc := range arcs {
		cloned = append(cloned, petri.Arc{
			ID:           fmt.Sprintf("%s:%s:%s", transitionID, suffix, arc.PlaceID),
			Name:         fmt.Sprintf("%s:%s:%s", transitionID, suffix, arc.PlaceID),
			PlaceID:      arc.PlaceID,
			TransitionID: transitionID,
			Direction:    petri.ArcOutput,
			Cardinality:  arc.Cardinality,
		})
	}
	return cloned
}

func ensureDefaultFailureArcs(net *Net, transition *petri.Transition) {
	if transition == nil || len(transition.FailureArcs) > 0 {
		return
	}

	seen := make(map[string]struct{})
	for _, inputArc := range transition.InputArcs {
		place, ok := net.Places[inputArc.PlaceID]
		if !ok {
			continue
		}
		workType, ok := net.WorkTypes[place.TypeID]
		if !ok {
			continue
		}

		failedPlaceID := failedPlaceIDForWorkType(workType)
		if failedPlaceID == "" {
			continue
		}
		if _, exists := seen[failedPlaceID]; exists {
			continue
		}
		seen[failedPlaceID] = struct{}{}

		transition.FailureArcs = append(transition.FailureArcs, petri.Arc{
			ID:           fmt.Sprintf("%s:auto-failure:%s", transition.ID, failedPlaceID),
			Name:         fmt.Sprintf("%s:auto-failure:%s", transition.ID, failedPlaceID),
			PlaceID:      failedPlaceID,
			TransitionID: transition.ID,
			Direction:    petri.ArcOutput,
			Cardinality: petri.ArcCardinality{
				Mode: petri.CardinalityOne,
			},
		})
	}
}

func failedPlaceIDForWorkType(workType *WorkType) string {
	if workType == nil {
		return ""
	}
	for _, stateDef := range workType.States {
		if stateDef.Category == StateCategoryFailed {
			return PlaceID(workType.ID, stateDef.Value)
		}
	}
	return ""
}
