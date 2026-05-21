package projections

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func workersFromGenerated(workers *[]factoryapi.Worker) []interfaces.FactoryWorker {
	generated := sliceValue(workers)
	converted := make([]interfaces.FactoryWorker, 0, len(generated))
	for _, worker := range generated {
		config := map[string]string{}
		if workerType := enumStringValue(worker.Type); workerType != "" {
			config["type"] = workerType
		}
		converted = append(converted, interfaces.FactoryWorker{
			ID:            worker.Name,
			Name:          worker.Name,
			Provider:      enumStringValue(worker.ExecutorProvider),
			ModelProvider: enumStringValue(worker.ModelProvider),
			Model:         stringValue(worker.Model),
			Config:        nilIfEmptyStringMap(config),
		})
	}
	return converted
}

func workstationsFromGenerated(workstations *[]factoryapi.Workstation) []interfaces.FactoryWorkstation {
	generated := sliceValue(workstations)
	converted := make([]interfaces.FactoryWorkstation, 0, len(generated))
	for _, workstation := range generated {
		id := stringValue(workstation.Id)
		if id == "" {
			id = workstation.Name
		}
		config := map[string]string{}
		if runtimeType := enumStringValue(workstation.Type); runtimeType != "" {
			config["type"] = runtimeType
		}
		if workstation.Worker != "" {
			config["worker"] = workstation.Worker
			config["configured_worker"] = workstation.Worker
		}
		converted = append(converted, interfaces.FactoryWorkstation{
			ID:                id,
			Name:              workstation.Name,
			WorkerID:          workstation.Worker,
			Kind:              workstationKindString(workstation.Behavior),
			Config:            nilIfEmptyStringMap(config),
			InputPlaceIDs:     placeIDsFromGeneratedIOs(workstation.Inputs),
			OutputPlaceIDs:    placeIDsFromGeneratedIOs(workstation.Outputs),
			ContinuePlaceIDs:  placeIDsFromGeneratedIOsPtr(workstation.OnContinue),
			RejectionPlaceIDs: placeIDsFromGeneratedIOsPtr(workstation.OnRejection),
			FailurePlaceIDs:   placeIDsFromGeneratedIOsPtr(workstation.OnFailure),
		})
	}
	return converted
}

func nilIfEmptyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return values
}
