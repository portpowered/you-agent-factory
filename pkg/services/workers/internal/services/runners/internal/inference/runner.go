// Package inference implements the Workers-parent-private Inference Runner.
package inference

import (
	"context"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const Identity = "inference"

// Config is the immutable inference definition captured by one Runner.
type Config struct {
	Worker    models.LocalWorker
	Resources []models.LocalResource
}

// LocalInvoker is the Models-root local invocation edge required by one Runner.
type LocalInvoker interface {
	InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error)
}

// Dependencies are the exact effects used by one Inference Runner.
type Dependencies struct {
	Models   LocalInvoker
	Delegate workers.Runner
}

type runner struct {
	worker    models.LocalWorker
	resources []models.LocalResource
	models    LocalInvoker
	delegate  workers.Runner
}

var _ workers.Runner = (*runner)(nil)

// New validates and snapshots an Inference Runner and its exact Models edge.
func New(config Config, dependencies Dependencies) (workers.Runner, error) {
	if dependencies.Models == nil {
		return nil, misconfigured("inference Models service is required", nil)
	}
	worker := snapshotWorker(config.Worker)
	if err := validateWorker(worker); err != nil {
		return nil, err
	}
	return &runner{
		worker:    worker,
		resources: snapshotResources(config.Resources),
		models:    dependencies.Models,
		delegate:  dependencies.Delegate,
	}, nil
}

func (r *runner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	request = workers.CloneProviderInferenceRequest(request)
	if err := validateRequest(request); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	if err := validateModelBindings(request.ModelBindings); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	invocation := r.localInvocationRequest(request)
	if err := models.ValidateLocalInvocationRequest(invocation); err != nil {
		return workers.RunnerExecutionResult{}, badRequest("inference request is invalid", err)
	}
	result, err := r.models.InvokeLocal(ctx, invocation)
	if !result.Handled {
		if r.delegate != nil {
			return r.delegate.Execute(ctx, request)
		}
		return workers.RunnerExecutionResult{}, err
	}
	if err != nil {
		return workers.RunnerExecutionResult{}, r.normalizeInvocationError(err, request)
	}
	return workers.RunnerExecutionResult{Content: result.Content}, nil
}

func (r *runner) localInvocationRequest(
	request workers.RunnerExecutionRequest,
) models.LocalInvocationRequest {
	return models.LocalInvocationRequest{
		Holder:           invocationHolder(request),
		Worker:           r.worker,
		Resources:        r.resources,
		Dispatch:         request.Dispatch,
		ModelOperation:   request.ModelOperation,
		ModelBindings:    modelBindingsForLocalRuntime(request.ModelBindings),
		WorkingDirectory: effectiveWorkingDirectory(request),
	}
}

func validateRequest(request workers.RunnerExecutionRequest) error {
	if request.RunnerID != Identity {
		return badRequest(fmt.Sprintf("inference runner identity must be %q", Identity), nil)
	}
	if strings.TrimSpace(request.ModelOperation) == "" {
		return badRequest("inference model operation is required", nil)
	}
	return nil
}

func validateModelBindings(bindings []workers.ResolvedModelOperationBinding) error {
	for index, binding := range bindings {
		if strings.TrimSpace(binding.Slot) == "" {
			return badRequest(fmt.Sprintf("inference model binding[%d] slot is required", index), nil)
		}
		if !isValidModelBindingSource(binding.Source) {
			return badRequest(fmt.Sprintf("inference model binding[%d] source is invalid", index), nil)
		}
	}
	return nil
}

func isValidModelBindingSource(source workers.ModelOperationBindingSource) bool {
	switch source {
	case workers.ModelOperationBindingSourceInput,
		workers.ModelOperationBindingSourceConfig,
		workers.ModelOperationBindingSourceDefault,
		workers.ModelOperationBindingSourceOmitted:
		return true
	default:
		return false
	}
}

func (r *runner) normalizeInvocationError(
	err error,
	request workers.RunnerExecutionRequest,
) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	failure, ok := workers.ClassifyInferenceFailure(err, workers.InferenceFailureContext{
		ModelName:  r.worker.Model,
		WorkerName: r.worker.Name,
		Operation:  request.ModelOperation,
	})
	if ok {
		return failure
	}
	return err
}

func validateWorker(worker models.LocalWorker) error {
	if strings.TrimSpace(worker.Name) == "" {
		return misconfigured("inference worker name is required", nil)
	}
	if err := models.ValidateLocalInvocationRequest(models.LocalInvocationRequest{
		Worker: worker,
	}); err != nil {
		return misconfigured("inference worker configuration is invalid", err)
	}
	return nil
}

func invocationHolder(request workers.RunnerExecutionRequest) string {
	if dispatchID := strings.TrimSpace(request.Dispatch.DispatchID); dispatchID != "" {
		return dispatchID
	}
	return strings.TrimSpace(request.Dispatch.WorkstationName)
}

func effectiveWorkingDirectory(request workers.RunnerExecutionRequest) string {
	if request.WorkingDirectory != "" {
		return request.WorkingDirectory
	}
	return request.Worktree
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
			Slot:    value.Slot,
			Source:  string(value.Source),
			Content: work.CloneWorkContentParts(value.Content),
		}
	}
	return result
}

func snapshotWorker(worker models.LocalWorker) models.LocalWorker {
	return models.LocalWorker{
		Name:          worker.Name,
		Type:          worker.Type,
		Model:         worker.Model,
		ModelLocality: worker.ModelLocality,
		Resources:     snapshotResources(worker.Resources),
	}
}

func snapshotResources(resources []models.LocalResource) []models.LocalResource {
	if len(resources) == 0 {
		return nil
	}
	result := make([]models.LocalResource, len(resources))
	for index, resource := range resources {
		result[index] = models.LocalResource{
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
	return result
}

// WorkerFromFactory snapshots one Factory worker into Models-owned vocabulary.
func WorkerFromFactory(worker *interfaces.FactoryWorkerConfig) models.LocalWorker {
	if worker == nil {
		return models.LocalWorker{}
	}
	return models.LocalWorker{
		Name:          worker.Name,
		Type:          worker.Type,
		Model:         worker.Model,
		ModelLocality: worker.ModelLocality,
		Resources:     ResourcesFromFactory(worker.Resources),
	}
}

// ResourcesFromFactory snapshots Factory resources into Models-owned vocabulary.
func ResourcesFromFactory(resources []interfaces.ResourceConfig) []models.LocalResource {
	if len(resources) == 0 {
		return nil
	}
	result := make([]models.LocalResource, len(resources))
	for index, resource := range resources {
		result[index] = models.LocalResource{
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
	return result
}

func badRequest(message string, cause error) error {
	return workers.NewProviderError(
		workers.WorkFailureTypePermanentBadRequest,
		message,
		cause,
	)
}

func misconfigured(message string, cause error) error {
	return workers.NewProviderError(
		workers.WorkFailureTypeMisconfigured,
		message,
		cause,
	)
}
