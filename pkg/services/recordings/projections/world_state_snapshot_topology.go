package projections

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// snapshotFactoryTopology is the Factory-owned subset of a serialized Factory
// needed to rebuild world-state topology. Keeping this decoder beside the
// reducer avoids making canonical projection depend on generated transports.
type snapshotFactoryTopology struct {
	Name         string                          `json:"name"`
	Resources    []snapshotFactoryResource       `json:"resources"`
	Layout       *interfaces.FactoryLayoutConfig `json:"layout"`
	Workers      []snapshotFactoryWorker         `json:"workers"`
	WorkTypes    []snapshotFactoryWorkType       `json:"workTypes"`
	Workstations []snapshotFactoryWorkstation    `json:"workstations"`
}

type snapshotFactoryResource struct {
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
}

type snapshotFactoryWorker struct {
	Name             string  `json:"name"`
	Type             *string `json:"type"`
	ExecutorProvider *string `json:"executorProvider"`
	ModelProvider    *string `json:"modelProvider"`
	Model            *string `json:"model"`
}

type snapshotFactoryWorkType struct {
	Name   string                     `json:"name"`
	States []snapshotFactoryWorkState `json:"states"`
}

type snapshotFactoryWorkState struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type snapshotFactoryWorkstation struct {
	ID          *string              `json:"id"`
	Name        string               `json:"name"`
	Worker      string               `json:"worker"`
	Behavior    *string              `json:"behavior"`
	Type        *string              `json:"type"`
	Inputs      []snapshotFactoryIO  `json:"inputs"`
	Outputs     *[]snapshotFactoryIO `json:"outputs"`
	OnContinue  *[]snapshotFactoryIO `json:"onContinue"`
	OnRejection *[]snapshotFactoryIO `json:"onRejection"`
	OnFailure   *[]snapshotFactoryIO `json:"onFailure"`
}

type snapshotFactoryIO struct {
	WorkType string `json:"workType"`
	State    string `json:"state"`
}

func initialStructureFromSnapshot(snapshot *interfaces.FactorySnapshot) (interfaces.InitialStructurePayload, error) {
	var factory snapshotFactoryTopology
	if err := snapshot.Decode(&factory); err != nil {
		return interfaces.InitialStructurePayload{}, err
	}

	resources, resourcePlaces := resourcesAndPlacesFromSnapshot(factory.Resources)
	workTypes, workTypePlaces := workTypesAndPlacesFromSnapshot(factory.WorkTypes)
	places := make([]interfaces.FactoryPlace, 0, len(resourcePlaces)+len(workTypePlaces))
	places = append(places, resourcePlaces...)
	places = append(places, workTypePlaces...)

	return interfaces.InitialStructurePayload{
		Name:         factory.Name,
		Resources:    resources,
		Layout:       factory.Layout,
		Workers:      workersFromSnapshot(factory.Workers),
		WorkTypes:    workTypes,
		Workstations: workstationsFromSnapshot(factory.Workstations),
		Places:       places,
	}, nil
}

func resourcesAndPlacesFromSnapshot(resources []snapshotFactoryResource) ([]interfaces.FactoryResource, []interfaces.FactoryPlace) {
	convertedResources := make([]interfaces.FactoryResource, 0, len(resources))
	convertedPlaces := make([]interfaces.FactoryPlace, 0, len(resources))
	for _, resource := range resources {
		convertedResources = append(convertedResources, interfaces.FactoryResource{
			ID: resource.Name, Name: resource.Name, Capacity: resource.Capacity,
		})
		convertedPlaces = append(convertedPlaces, interfaces.FactoryPlace{
			ID:     topologyPlaceID(resource.Name, interfaces.ResourceStateAvailable),
			TypeID: resource.Name, State: interfaces.ResourceStateAvailable, Category: "PROCESSING",
		})
	}
	return convertedResources, convertedPlaces
}

func workTypesAndPlacesFromSnapshot(workTypes []snapshotFactoryWorkType) ([]interfaces.FactoryWorkType, []interfaces.FactoryPlace) {
	convertedWorkTypes := make([]interfaces.FactoryWorkType, 0, len(workTypes))
	convertedPlaces := make([]interfaces.FactoryPlace, 0)
	for _, workType := range workTypes {
		converted := interfaces.FactoryWorkType{ID: workType.Name, Name: workType.Name}
		for _, state := range workType.States {
			converted.States = append(converted.States, interfaces.FactoryStateDefinition{
				Value: state.Name, Category: state.Type,
			})
			convertedPlaces = append(convertedPlaces, interfaces.FactoryPlace{
				ID:     topologyPlaceID(workType.Name, state.Name),
				TypeID: workType.Name, State: state.Name, Category: state.Type,
			})
		}
		convertedWorkTypes = append(convertedWorkTypes, converted)
	}
	return convertedWorkTypes, convertedPlaces
}

func workersFromSnapshot(workers []snapshotFactoryWorker) []interfaces.FactoryWorker {
	converted := make([]interfaces.FactoryWorker, 0, len(workers))
	for _, worker := range workers {
		config := map[string]string{}
		if workerType := stringValue(worker.Type); workerType != "" {
			config["type"] = workerType
		}
		converted = append(converted, interfaces.FactoryWorker{
			ID: worker.Name, Name: worker.Name,
			Provider: stringValue(worker.ExecutorProvider), ModelProvider: stringValue(worker.ModelProvider),
			Model: stringValue(worker.Model), Config: nilIfEmptyStringMap(config),
		})
	}
	return converted
}

func workstationsFromSnapshot(workstations []snapshotFactoryWorkstation) []interfaces.FactoryWorkstation {
	converted := make([]interfaces.FactoryWorkstation, 0, len(workstations))
	for _, workstation := range workstations {
		id := stringValue(workstation.ID)
		if id == "" {
			id = workstation.Name
		}
		config := map[string]string{}
		if runtimeType := stringValue(workstation.Type); runtimeType != "" {
			config["type"] = runtimeType
		}
		if workstation.Worker != "" {
			config["worker"] = workstation.Worker
			config["configured_worker"] = workstation.Worker
		}
		converted = append(converted, interfaces.FactoryWorkstation{
			ID: id, Name: workstation.Name, WorkerID: workstation.Worker,
			Kind: stringValue(workstation.Behavior), Config: nilIfEmptyStringMap(config),
			InputPlaceIDs:     placeIDsFromSnapshotIOs(workstation.Inputs),
			OutputPlaceIDs:    placeIDsFromSnapshotIOsPtr(workstation.Outputs),
			ContinuePlaceIDs:  placeIDsFromSnapshotIOsPtr(workstation.OnContinue),
			RejectionPlaceIDs: placeIDsFromSnapshotIOsPtr(workstation.OnRejection),
			FailurePlaceIDs:   placeIDsFromSnapshotIOsPtr(workstation.OnFailure),
		})
	}
	return converted
}

func placeIDsFromSnapshotIOs(values []snapshotFactoryIO) []string {
	if len(values) == 0 {
		return nil
	}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		ids = append(ids, topologyPlaceID(value.WorkType, value.State))
	}
	return ids
}

func placeIDsFromSnapshotIOsPtr(values *[]snapshotFactoryIO) []string {
	if values == nil {
		return nil
	}
	return placeIDsFromSnapshotIOs(*values)
}

func nilIfEmptyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return values
}
