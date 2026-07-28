package recordingsqueries

import (
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ProjectActiveThrottlePauses converts dispatcher-owned runtime pause windows into
// dashboard presentation facts using authored topology metadata.
func ProjectActiveThrottlePauses(
	topology factorydefinitions.InitialStructurePayload,
	pauses []factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	if len(pauses) == 0 {
		return nil
	}

	workersByID := make(map[string]factorydefinitions.FactoryWorker, len(topology.Workers))
	for _, worker := range topology.Workers {
		if worker.ID == "" {
			continue
		}
		workersByID[worker.ID] = worker
	}

	placesByID := make(map[string]factorydefinitions.FactoryPlace, len(topology.Places))
	for _, place := range topology.Places {
		if place.ID == "" {
			continue
		}
		placesByID[place.ID] = place
	}

	projected := make([]factorydefinitions.FactoryWorldThrottlePause, 0, len(pauses))
	for _, pause := range pauses {
		projected = append(projected, factorydefinitions.FactoryWorldThrottlePause{
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

func affectedTransitionIDsForPause(
	workstations []factorydefinitions.FactoryWorkstation,
	workersByID map[string]factorydefinitions.FactoryWorker,
	pause factorydefinitions.ActiveThrottlePause,
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
	workstations []factorydefinitions.FactoryWorkstation,
	workersByID map[string]factorydefinitions.FactoryWorker,
	pause factorydefinitions.ActiveThrottlePause,
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
	workstations []factorydefinitions.FactoryWorkstation,
	workersByID map[string]factorydefinitions.FactoryWorker,
	pause factorydefinitions.ActiveThrottlePause,
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
	workstations []factorydefinitions.FactoryWorkstation,
	workersByID map[string]factorydefinitions.FactoryWorker,
	placesByID map[string]factorydefinitions.FactoryPlace,
	pause factorydefinitions.ActiveThrottlePause,
) []string {
	var workTypeIDs []string
	for _, workstation := range workstations {
		if !workstationMatchesPause(workstation, workersByID, pause) {
			continue
		}
		for _, placeID := range workstation.InputPlaceIDs {
			place, ok := placesByID[placeID]
			if !ok || place.TypeID == "" || factorydefinitions.IsSystemTimeWorkType(place.TypeID) {
				continue
			}
			workTypeIDs = appendUnique(workTypeIDs, place.TypeID)
		}
	}
	return sortedStrings(workTypeIDs)
}

func workstationMatchesPause(
	workstation factorydefinitions.FactoryWorkstation,
	workersByID map[string]factorydefinitions.FactoryWorker,
	pause factorydefinitions.ActiveThrottlePause,
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	return values
}
