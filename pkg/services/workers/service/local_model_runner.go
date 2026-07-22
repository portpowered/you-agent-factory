package service

import (
	"context"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type managedLocalModelRunner struct {
	inner       workers.Runner
	localModels models.Service
	factory     *interfaces.FactoryConfig
	worker      *interfaces.FactoryWorkerConfig
}

func wrapLocalModelRunner(
	inner workers.Runner,
	localModels models.Service,
	factory *interfaces.FactoryConfig,
	worker *interfaces.FactoryWorkerConfig,
) workers.Runner {
	if inner == nil || localModels == nil || factory == nil || worker == nil {
		return inner
	}
	return &managedLocalModelRunner{
		inner: inner, localModels: localModels, factory: factory, worker: worker,
	}
}

func (r *managedLocalModelRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	result, err := r.localModels.InvokeLocal(ctx, models.LocalInvocationRequest{
		Holder:           localInvocationHolder(request),
		Worker:           localWorkerProjection(r.worker),
		Resources:        localResourceProjections(r.factory.Resources),
		Dispatch:         request.Dispatch,
		ModelOperation:   request.ModelOperation,
		ModelBindings:    modelBindingsForLocalRuntime(request.ModelBindings),
		WorkingDirectory: request.WorkingDirectory,
	})
	if !result.Handled {
		return r.inner.Execute(ctx, request)
	}
	return workers.InferenceResponse{Content: result.Content}, err
}

func modelBindingsForLocalRuntime(
	values []workers.ResolvedModelOperationBinding,
) []models.ResolvedModelOperationBinding {
	if len(values) == 0 {
		return nil
	}
	result := make([]models.ResolvedModelOperationBinding, len(values))
	for index, value := range values {
		result[index] = models.ResolvedModelOperationBinding{
			Slot: value.Slot, Source: string(value.Source), Content: value.Content,
		}
	}
	return result
}

func localWorkerProjection(worker *interfaces.FactoryWorkerConfig) models.LocalWorker {
	if worker == nil {
		return models.LocalWorker{}
	}
	return models.LocalWorker{
		Name: worker.Name, Type: worker.Type, Model: worker.Model,
		ModelLocality: worker.ModelLocality,
		Resources:     localResourceProjections(worker.Resources),
	}
}

func localResourceProjections(resources []interfaces.ResourceConfig) []models.LocalResource {
	if len(resources) == 0 {
		return nil
	}
	result := make([]models.LocalResource, len(resources))
	for index, resource := range resources {
		result[index] = models.LocalResource{
			ID: resource.ID, Name: resource.Name, Type: resource.Type,
			Capacity: resource.Capacity, Model: resource.Model,
			Backend: resource.Backend, LoadPolicy: resource.LoadPolicy,
			Provider: resource.Provider,
		}
	}
	return result
}

func localInvocationHolder(request workers.RunnerExecutionRequest) string {
	if dispatchID := strings.TrimSpace(request.Dispatch.DispatchID); dispatchID != "" {
		return dispatchID
	}
	return strings.TrimSpace(request.Dispatch.WorkstationName)
}
