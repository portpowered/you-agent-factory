package projections

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ProjectActiveThrottlePauses converts dispatcher-owned runtime pause windows
// into the world-view/dashboard pause shape using authored topology metadata.
func ProjectActiveThrottlePauses(
	topology interfaces.InitialStructurePayload,
	pauses []interfaces.ActiveThrottlePause,
) []interfaces.FactoryWorldThrottlePause {
	if len(pauses) == 0 {
		return nil
	}

	workersByID := make(map[string]interfaces.FactoryWorker, len(topology.Workers))
	for _, worker := range topology.Workers {
		if worker.ID == "" {
			continue
		}
		workersByID[worker.ID] = worker
	}

	placesByID := make(map[string]interfaces.FactoryPlace, len(topology.Places))
	for _, place := range topology.Places {
		if place.ID == "" {
			continue
		}
		placesByID[place.ID] = place
	}

	projected := make([]interfaces.FactoryWorldThrottlePause, 0, len(pauses))
	for _, pause := range pauses {
		projected = append(projected, interfaces.FactoryWorldThrottlePause{
			LaneID:                   pause.LaneID,
			Provider:                 pause.Provider,
			Model:                    pause.Model,
			PausedAt:                 pause.PausedAt,
			PausedUntil:              pause.PausedUntil,
			RecoverAt:                pause.PausedUntil,
			AffectedTransitionIDs:    affectedTransitionIDsForPause(topology.Workstations, workersByID, pause),
			AffectedWorkstationNames: affectedWorkstationNamesForPause(topology.Workstations, workersByID, pause),
			AffectedWorkerTypes:      affectedWorkerTypesForPause(topology.Workstations, workersByID, pause),
			AffectedWorkTypeIDs:      affectedWorkTypeIDsForPause(topology.Workstations, workersByID, placesByID, pause),
		})
	}

	return projected
}

func BuildFactoryWorldViewWithActiveThrottlePauses(
	state interfaces.FactoryWorldState,
	pauses []interfaces.ActiveThrottlePause,
) interfaces.FactoryWorldView {
	view := BuildFactoryWorldView(state)
	view.Runtime.ActiveThrottlePauses = ProjectActiveThrottlePauses(state.Topology, pauses)
	return view
}

func affectedTransitionIDsForPause(
	workstations []interfaces.FactoryWorkstation,
	workersByID map[string]interfaces.FactoryWorker,
	pause interfaces.ActiveThrottlePause,
) []string {
	var ids []string
	for _, workstation := range workstations {
		if !workstationMatchesPause(workstation, workersByID, pause) {
			continue
		}
		ids = appendUnique(ids, workstation.ID)
	}
	return sortedStrings(ids)
}

func affectedWorkstationNamesForPause(
	workstations []interfaces.FactoryWorkstation,
	workersByID map[string]interfaces.FactoryWorker,
	pause interfaces.ActiveThrottlePause,
) []string {
	var names []string
	for _, workstation := range workstations {
		if !workstationMatchesPause(workstation, workersByID, pause) {
			continue
		}
		names = appendUnique(names, workstation.Name)
	}
	return sortedStrings(names)
}

func affectedWorkerTypesForPause(
	workstations []interfaces.FactoryWorkstation,
	workersByID map[string]interfaces.FactoryWorker,
	pause interfaces.ActiveThrottlePause,
) []string {
	var workerTypes []string
	for _, workstation := range workstations {
		if !workstationMatchesPause(workstation, workersByID, pause) {
			continue
		}
		workerTypes = appendUnique(workerTypes, workstation.WorkerID)
	}
	return sortedStrings(workerTypes)
}

func affectedWorkTypeIDsForPause(
	workstations []interfaces.FactoryWorkstation,
	workersByID map[string]interfaces.FactoryWorker,
	placesByID map[string]interfaces.FactoryPlace,
	pause interfaces.ActiveThrottlePause,
) []string {
	var workTypeIDs []string
	for _, workstation := range workstations {
		if !workstationMatchesPause(workstation, workersByID, pause) {
			continue
		}
		for _, placeID := range workstation.InputPlaceIDs {
			place, ok := placesByID[placeID]
			if !ok || place.TypeID == "" || interfaces.IsSystemTimeWorkType(place.TypeID) {
				continue
			}
			workTypeIDs = appendUnique(workTypeIDs, place.TypeID)
		}
	}
	return sortedStrings(workTypeIDs)
}

func workstationMatchesPause(
	workstation interfaces.FactoryWorkstation,
	workersByID map[string]interfaces.FactoryWorker,
	pause interfaces.ActiveThrottlePause,
) bool {
	if workstation.WorkerID == "" {
		return false
	}
	worker, ok := workersByID[workstation.WorkerID]
	if !ok {
		return false
	}
	provider := firstNonEmpty(worker.ModelProvider, worker.Provider)
	return strings.EqualFold(provider, pause.Provider) && worker.Model == pause.Model
}
