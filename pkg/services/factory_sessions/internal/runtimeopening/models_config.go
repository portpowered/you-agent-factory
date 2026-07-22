package runtimeopening

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

func ProjectModelsRuntimeConfig(source factorydefinitions.RuntimeConfigLookup) *models.RuntimeConfig {
	if source == nil || source.FactoryConfig() == nil {
		return nil
	}
	factory := source.FactoryConfig()
	projected := &models.RuntimeConfig{
		FactoryDirectory: source.FactoryDir(),
		BaseDirectory:    source.RuntimeBaseDir(),
		Workers:          make([]models.RuntimeWorker, len(factory.Workers)),
		Resources:        projectModelsRuntimeResources(factory.Resources),
	}
	for i := range factory.Workers {
		worker := factory.Workers[i]
		projected.Workers[i] = models.RuntimeWorker{
			Name:          worker.Name,
			Type:          worker.Type,
			Model:         worker.Model,
			ModelProvider: worker.ModelProvider,
			ModelLocality: worker.ModelLocality,
			Command:       worker.Command,
			Args:          append([]string(nil), worker.Args...),
			Operations:    projectModelsRuntimeOperations(worker.Operations),
			Resources:     projectModelsRuntimeResources(worker.Resources),
		}
	}
	return projected
}

func projectModelsRuntimeResources(resources []factorydefinitions.ResourceConfig) []models.RuntimeResource {
	projected := make([]models.RuntimeResource, len(resources))
	for i := range resources {
		resource := resources[i]
		projected[i] = models.RuntimeResource{
			ID:         resource.ID,
			Name:       resource.Name,
			Type:       resource.Type,
			Capacity:   resource.Capacity,
			Model:      resource.Model,
			Backend:    resource.Backend,
			LoadPolicy: resource.LoadPolicy,
			Provider:   resource.Provider,
		}
	}
	return projected
}

func projectModelsRuntimeOperations(operations []factorydefinitions.ModelOperation) []models.RuntimeOperation {
	projected := make([]models.RuntimeOperation, len(operations))
	for i := range operations {
		projected[i] = models.RuntimeOperation{
			Name:    operations[i].Name,
			Inputs:  projectModelsRuntimeOperationSlots(operations[i].Inputs),
			Outputs: projectModelsRuntimeOperationSlots(operations[i].Outputs),
		}
	}
	return projected
}

func projectModelsRuntimeOperationSlots(slots []factorydefinitions.ModelOperationSlot) []models.RuntimeOperationSlot {
	projected := make([]models.RuntimeOperationSlot, len(slots))
	for i := range slots {
		projected[i] = models.RuntimeOperationSlot{
			Name:         slots[i].Name,
			ContentTypes: append([]string(nil), slots[i].ContentTypes...),
			Required:     slots[i].Required,
		}
	}
	return projected
}
