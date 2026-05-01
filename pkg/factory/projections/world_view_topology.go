package projections

import (
	"sort"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func buildFactoryWorldTopologyView(topology interfaces.InitialStructurePayload) interfaces.FactoryWorldTopologyView {
	placesByID := make(map[string]interfaces.FactoryPlace, len(topology.Places))
	for _, place := range topology.Places {
		placesByID[place.ID] = place
	}
	workTypeIDs := make(map[string]struct{}, len(topology.WorkTypes))
	for _, workType := range topology.WorkTypes {
		if interfaces.IsSystemTimeWorkType(workType.ID) {
			continue
		}
		workTypeIDs[workType.ID] = struct{}{}
	}
	resourceIDs := make(map[string]struct{}, len(topology.Resources))
	for _, resource := range topology.Resources {
		resourceIDs[resource.ID] = struct{}{}
	}
	workstationsByID := make(map[string]interfaces.FactoryWorkstation, len(topology.Workstations))
	nodeIDs := make([]string, 0, len(topology.Workstations))
	for _, workstation := range topology.Workstations {
		if isSystemTimeWorkstation(workstation.ID) {
			continue
		}
		workstationsByID[workstation.ID] = workstation
		nodeIDs = append(nodeIDs, workstation.ID)
	}
	sort.Strings(nodeIDs)

	nodesByID := make(map[string]interfaces.FactoryWorldWorkstationNode, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		workstation := workstationsByID[nodeID]
		inputPlaceIDs := filterCustomerPlaceIDs(workstation.InputPlaceIDs)
		outputPlaceIDs := append([]string(nil), workstation.OutputPlaceIDs...)
		outputPlaceIDs = append(outputPlaceIDs, workstation.ContinuePlaceIDs...)
		outputPlaceIDs = append(outputPlaceIDs, workstation.RejectionPlaceIDs...)
		outputPlaceIDs = append(outputPlaceIDs, workstation.FailurePlaceIDs...)
		outputPlaceIDs = filterCustomerPlaceIDs(outputPlaceIDs)
		nodesByID[nodeID] = interfaces.FactoryWorldWorkstationNode{
			NodeID:            nodeID,
			TransitionID:      nodeID,
			WorkstationName:   workstation.Name,
			WorkerType:        workstation.WorkerID,
			WorkstationKind:   workstation.Kind,
			InputPlaces:       buildFactoryWorldPlaceRefs(inputPlaceIDs, placesByID, workTypeIDs, resourceIDs),
			OutputPlaces:      buildFactoryWorldPlaceRefs(outputPlaceIDs, placesByID, workTypeIDs, resourceIDs),
			InputPlaceIDs:     sortedStrings(inputPlaceIDs),
			OutputPlaceIDs:    sortedStrings(outputPlaceIDs),
			InputWorkTypeIDs:  workTypeIDsForPlaces(inputPlaceIDs, placesByID, workTypeIDs),
			OutputWorkTypeIDs: workTypeIDsForPlaces(outputPlaceIDs, placesByID, workTypeIDs),
		}
	}

	return interfaces.FactoryWorldTopologyView{
		SubmitWorkTypes:      buildFactoryWorldSubmitWorkTypes(topology.WorkTypes),
		WorkstationNodeIDs:   nodeIDs,
		WorkstationNodesByID: nodesByID,
		Edges:                buildFactoryWorldEdges(nodeIDs, workstationsByID, placesByID, workTypeIDs),
	}
}

func buildFactoryWorldSubmitWorkTypes(workTypes []interfaces.FactoryWorkType) []interfaces.FactoryWorldSubmitWorkType {
	if len(workTypes) == 0 {
		return nil
	}

	submitWorkTypes := make([]interfaces.FactoryWorldSubmitWorkType, 0, len(workTypes))
	for _, workType := range workTypes {
		if interfaces.IsSystemTimeWorkType(workType.ID) || !factoryWorkTypeHasInitialState(workType) {
			continue
		}
		submitWorkTypes = append(submitWorkTypes, interfaces.FactoryWorldSubmitWorkType{
			WorkTypeName: factoryWorkTypeContractName(workType),
		})
	}
	if len(submitWorkTypes) == 0 {
		return nil
	}
	sort.Slice(submitWorkTypes, func(i, j int) bool {
		return submitWorkTypes[i].WorkTypeName < submitWorkTypes[j].WorkTypeName
	})
	return submitWorkTypes
}

func factoryWorkTypeContractName(workType interfaces.FactoryWorkType) string {
	if workType.Name != "" {
		return workType.Name
	}
	return workType.ID
}

func factoryWorkTypeHasInitialState(workType interfaces.FactoryWorkType) bool {
	for _, state := range workType.States {
		if state.Category == string(interfaces.StateTypeInitial) {
			return true
		}
	}
	return false
}

func buildFactoryWorldEdges(nodeIDs []string, workstations map[string]interfaces.FactoryWorkstation, places map[string]interfaces.FactoryPlace, workTypes map[string]struct{}) []interfaces.FactoryWorldWorkstationEdge {
	inputsByPlace := make(map[string][]string)
	for _, nodeID := range nodeIDs {
		for _, placeID := range workstations[nodeID].InputPlaceIDs {
			if _, ok := workTypes[places[placeID].TypeID]; !ok {
				continue
			}
			inputsByPlace[placeID] = append(inputsByPlace[placeID], nodeID)
		}
	}
	for placeID := range inputsByPlace {
		sort.Strings(inputsByPlace[placeID])
	}

	var edges []interfaces.FactoryWorldWorkstationEdge
	seen := make(map[string]struct{})
	for _, sourceID := range nodeIDs {
		workstation := workstations[sourceID]
		edges = append(edges, buildFactoryWorldEdgesForPlaces(sourceID, workstation.OutputPlaceIDs, "accepted", inputsByPlace, places, seen)...)
		edges = append(edges, buildFactoryWorldEdgesForPlaces(sourceID, workstation.ContinuePlaceIDs, "continue", inputsByPlace, places, seen)...)
		edges = append(edges, buildFactoryWorldEdgesForPlaces(sourceID, workstation.RejectionPlaceIDs, "rejected", inputsByPlace, places, seen)...)
		edges = append(edges, buildFactoryWorldEdgesForPlaces(sourceID, workstation.FailurePlaceIDs, "failed", inputsByPlace, places, seen)...)
	}
	return edges
}

func buildFactoryWorldEdgesForPlaces(sourceID string, placeIDs []string, outcome string, inputsByPlace map[string][]string, places map[string]interfaces.FactoryPlace, seen map[string]struct{}) []interfaces.FactoryWorldWorkstationEdge {
	var edges []interfaces.FactoryWorldWorkstationEdge
	for _, placeID := range sortedStrings(placeIDs) {
		place := places[placeID]
		for _, destID := range inputsByPlace[placeID] {
			edgeID := sourceID + ":" + destID + ":" + placeID + ":" + outcome
			if _, ok := seen[edgeID]; ok {
				continue
			}
			seen[edgeID] = struct{}{}
			edges = append(edges, interfaces.FactoryWorldWorkstationEdge{
				EdgeID:        edgeID,
				FromNodeID:    sourceID,
				ToNodeID:      destID,
				ViaPlaceID:    placeID,
				WorkTypeID:    place.TypeID,
				StateValue:    place.State,
				StateCategory: place.Category,
				OutcomeKind:   outcome,
			})
		}
	}
	return edges
}

func buildFactoryWorldPlaceRefs(placeIDs []string, places map[string]interfaces.FactoryPlace, workTypes map[string]struct{}, resources map[string]struct{}) []interfaces.FactoryWorldPlaceRef {
	refs := make([]interfaces.FactoryWorldPlaceRef, 0, len(placeIDs))
	for _, placeID := range sortedStrings(placeIDs) {
		if interfaces.IsSystemTimePlace(placeID) {
			continue
		}
		place, ok := places[placeID]
		if !ok {
			continue
		}
		kind := "constraint"
		if _, ok := workTypes[place.TypeID]; ok {
			kind = "work_state"
		} else if _, ok := resources[place.TypeID]; ok {
			kind = "resource"
		}
		refs = append(refs, interfaces.FactoryWorldPlaceRef{
			PlaceID:       place.ID,
			TypeID:        place.TypeID,
			StateValue:    place.State,
			Kind:          kind,
			StateCategory: place.Category,
		})
	}
	return refs
}

func workTypeIDsForPlaces(placeIDs []string, places map[string]interfaces.FactoryPlace, workTypes map[string]struct{}) []string {
	var ids []string
	for _, placeID := range placeIDs {
		typeID := places[placeID].TypeID
		if _, ok := workTypes[typeID]; ok {
			ids = appendUnique(ids, typeID)
		}
	}
	return sortedStrings(ids)
}
