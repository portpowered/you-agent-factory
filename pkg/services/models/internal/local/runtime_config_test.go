package local

import models "github.com/portpowered/infinite-you/pkg/services/models"

type modelRuntimeConfig = models.RuntimeConfig
type modelRuntimeWorker = models.RuntimeWorker
type modelRuntimeResource = models.RuntimeResource

type testFactoryConfig struct {
	Name             string
	Workers          []modelRuntimeWorker
	Resources        []modelRuntimeResource
	ResourceManifest *testResourceManifest
}
type testResourceManifest struct{ RequiredTools []testRequiredTool }
type testRequiredTool struct{ Name, Command string }

func projectTestModelsRuntimeConfig(factoryDir string, cfg *testFactoryConfig) *modelRuntimeConfig {
	if cfg == nil {
		return nil
	}
	result := &modelRuntimeConfig{FactoryDirectory: factoryDir}
	result.Resources = projectTestModelsResources(cfg.Resources)
	result.Workers = make([]modelRuntimeWorker, len(cfg.Workers))
	for i, worker := range cfg.Workers {
		result.Workers[i] = modelRuntimeWorker{
			Name: worker.Name, Type: worker.Type, Model: worker.Model, ModelProvider: worker.ModelProvider,
			ModelLocality: worker.ModelLocality, Command: worker.Command, Args: append([]string(nil), worker.Args...),
			Resources:  projectTestModelsResources(worker.Resources),
			Operations: projectTestModelsOperations(worker.Operations),
		}
	}
	return result
}

func projectTestModelsResources(resources []modelRuntimeResource) []modelRuntimeResource {
	result := make([]modelRuntimeResource, len(resources))
	for i, resource := range resources {
		result[i] = modelRuntimeResource{
			ID: resource.ID, Name: resource.Name, Type: resource.Type, Capacity: resource.Capacity,
			Model: resource.Model, Backend: resource.Backend, LoadPolicy: resource.LoadPolicy, Provider: resource.Provider,
		}
	}
	return result
}

func projectTestModelsOperations(operations []models.RuntimeOperation) []models.RuntimeOperation {
	result := make([]models.RuntimeOperation, len(operations))
	for i, operation := range operations {
		result[i].Name = operation.Name
		result[i].Inputs = projectTestModelsOperationSlots(operation.Inputs)
		result[i].Outputs = projectTestModelsOperationSlots(operation.Outputs)
	}
	return result
}

func projectTestModelsOperationSlots(slots []models.RuntimeOperationSlot) []models.RuntimeOperationSlot {
	result := make([]models.RuntimeOperationSlot, len(slots))
	for i, slot := range slots {
		result[i] = models.RuntimeOperationSlot{
			Name: slot.Name, ContentTypes: append([]string(nil), slot.ContentTypes...), Required: slot.Required,
		}
	}
	return result
}
