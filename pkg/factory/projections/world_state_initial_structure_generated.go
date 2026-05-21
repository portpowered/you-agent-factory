package projections

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func initialStructureFromGenerated(payload factoryapi.InitialStructureRequestEventPayload) interfaces.InitialStructurePayload {
	factoryPayload := payload.Factory
	resources, resourcePlaces := resourcesAndPlacesFromGenerated(factoryPayload.Resources)
	workTypes, workTypePlaces := workTypesAndPlacesFromGenerated(factoryPayload.WorkTypes)

	places := make([]interfaces.FactoryPlace, 0, len(resourcePlaces)+len(workTypePlaces))
	places = append(places, resourcePlaces...)
	places = append(places, workTypePlaces...)

	return interfaces.InitialStructurePayload{
		Name:         string(factoryPayload.Name),
		Resources:    resources,
		Workers:      workersFromGenerated(factoryPayload.Workers),
		WorkTypes:    workTypes,
		Workstations: workstationsFromGenerated(factoryPayload.Workstations),
		Places:       places,
	}
}

func resourcesAndPlacesFromGenerated(resources *[]factoryapi.Resource) ([]interfaces.FactoryResource, []interfaces.FactoryPlace) {
	generated := sliceValue(resources)
	convertedResources := make([]interfaces.FactoryResource, 0, len(generated))
	convertedPlaces := make([]interfaces.FactoryPlace, 0, len(generated))
	for _, resource := range generated {
		convertedResources = append(convertedResources, interfaces.FactoryResource{
			ID:       resource.Name,
			Name:     resource.Name,
			Capacity: resource.Capacity,
		})
		convertedPlaces = append(convertedPlaces, interfaces.FactoryPlace{
			ID:       generatedPlaceID(resource.Name, "available"),
			TypeID:   resource.Name,
			State:    "available",
			Category: "PROCESSING",
		})
	}
	return convertedResources, convertedPlaces
}

func workTypesAndPlacesFromGenerated(workTypes *[]factoryapi.WorkType) ([]interfaces.FactoryWorkType, []interfaces.FactoryPlace) {
	generated := sliceValue(workTypes)
	convertedWorkTypes := make([]interfaces.FactoryWorkType, 0, len(generated))
	convertedPlaces := make([]interfaces.FactoryPlace, 0)
	for _, workType := range generated {
		converted := interfaces.FactoryWorkType{
			ID:   workType.Name,
			Name: workType.Name,
		}
		for _, stateDef := range workType.States {
			category := string(stateDef.Type)
			converted.States = append(converted.States, interfaces.FactoryStateDefinition{
				Value:    stateDef.Name,
				Category: category,
			})
			convertedPlaces = append(convertedPlaces, interfaces.FactoryPlace{
				ID:       generatedPlaceID(workType.Name, stateDef.Name),
				TypeID:   workType.Name,
				State:    stateDef.Name,
				Category: category,
			})
		}
		convertedWorkTypes = append(convertedWorkTypes, converted)
	}
	return convertedWorkTypes, convertedPlaces
}
