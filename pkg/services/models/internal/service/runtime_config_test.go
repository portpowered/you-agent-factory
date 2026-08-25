package service

import (
	"context"
	"errors"
	"testing"
)

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

func TestRootLegacyOperationsClassifyMissingRuntimeBinding(t *testing.T) {
	t.Parallel()

	root := &Root{}
	if _, err := root.ListModels(context.Background()); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("ListModels error = %v, want missing runtime binding", err)
	}
	if _, err := root.GetModel(context.Background(), "voice"); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("GetModel error = %v, want missing runtime binding", err)
	}
	if _, err := root.PullModel(context.Background(), "voice"); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("PullModel error = %v, want missing runtime binding", err)
	}
	if _, err := root.InspectRuntime(context.Background(), "voice"); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("InspectRuntime error = %v, want missing runtime binding", err)
	}
	if _, err := root.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "voice"}); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("AcquireLease error = %v, want missing runtime binding", err)
	}
	if err := root.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "lease"}); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("ReleaseLease error = %v, want missing runtime binding", err)
	}
}

func TestRuntimeServiceContractOnlyOperationsFailExplicitly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := &runtimeService{}
	_, err := svc.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "OpenRuntimeScope", err)
	_, err = svc.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "CloseRuntimeScope", err)
	_, err = svc.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "PrepareModelAssets", err)
	_, err = svc.InspectModelAssets(ctx, models.InspectModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "InspectModelAssets", err)
	_, err = svc.RemoveModelAssets(ctx, models.RemoveModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "RemoveModelAssets", err)
	_, err = svc.ResolveModelReference(ctx, models.ResolveModelReferenceRequest{})
	assertContractOnlyUnsupported(t, "ResolveModelReference", err)
	_, err = svc.InvokeModel(ctx, models.InvokeModelRequest{})
	assertContractOnlyUnsupported(t, "InvokeModel", err)
	result, err := svc.InvokeLocal(ctx, models.LocalInvocationRequest{})
	if err != nil || result.Handled {
		t.Fatalf("InvokeLocal result = %#v, error = %v, want declined no-op", result, err)
	}
	_, err = svc.InvokeLocal(ctx, models.LocalInvocationRequest{
		Worker: models.LocalWorker{
			Type:          models.RuntimeWorkerTypeInference,
			ModelLocality: models.RuntimeModelLocalityLocal,
		},
	})
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("managed InvokeLocal error = %v, want ErrNotFound", err)
	}
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
