// Package inference implements the Workers-parent-private Inference Runner.
package inference

import (
	"context"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const Identity = "inference"

// Config is the immutable inference definition captured by one Runner.
type Config struct {
	Worker    models.LocalWorker
	Resources []models.LocalResource
	Scope     models.RuntimeScopeRef
}

// ModelInvoker is the Models-root joined invocation edge required by one
// Inference Runner. The runner deliberately consumes the generic operation
// contract so model lifecycle, capacity, codecs, and output normalization stay
// owned by Models.
type ModelInvoker interface {
	InvokeModel(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error)
}

// Dependencies are the exact effects used by one Inference Runner.
type Dependencies struct {
	Models   ModelInvoker
	Delegate workers.Runner
}

type runner struct {
	worker    models.LocalWorker
	resources []models.LocalResource
	scope     models.RuntimeScopeRef
	models    ModelInvoker
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
		scope:     config.Scope,
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
	composition := r.delegate != nil &&
		request.RunnerID != "" &&
		request.RunnerID != Identity
	if composition {
		return r.delegate.Execute(ctx, delegateRequest(request))
	}
	if err := validateRequest(request); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	if err := validateModelBindings(request.ModelBindings); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	scope, worker, _ := modelRuntimeProjection(
		request,
		r.scope,
		r.workerForRequest(request),
		r.resources,
	)
	if !worker.UsesManagedRuntime() {
		if r.delegate != nil {
			return r.delegate.Execute(ctx, delegateRequest(request))
		}
		return workers.RunnerExecutionResult{}, badRequest(
			"inference worker requires a managed local runtime",
			nil,
		)
	}
	invocation, err := genericInvocationRequest(request, scope, worker)
	if err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	if err := invocation.ValidateGeneric(); err != nil {
		return workers.RunnerExecutionResult{}, badRequest("inference request is invalid", err)
	}
	result, err := r.models.InvokeModel(ctx, invocation)
	if err != nil {
		return workers.RunnerExecutionResult{}, r.normalizeInvocationError(err, request)
	}
	if result.Status == models.ModelInvocationStatusFailed ||
		result.Status == models.ModelInvocationStatusCancelled {
		return workers.RunnerExecutionResult{}, r.normalizeInvocationError(
			models.ErrInferenceFailed,
			request,
		)
	}
	output, err := proposedOutputFromModelResult(result)
	if err != nil {
		return workers.RunnerExecutionResult{}, r.normalizeInvocationError(err, request)
	}
	return workers.RunnerExecutionResult{
		Content:        textContentFromProposedOutput(output),
		Outcome:        workers.OutcomeAccepted,
		ProposedOutput: &output,
		Diagnostics: &workers.WorkDiagnostics{Metadata: map[string]string{
			workers.ProviderResponseMetadataCompletionEvidence: "provider_response",
		}},
	}, nil
}

func modelRuntimeProjection(
	request workers.RunnerExecutionRequest,
	fallbackScope models.RuntimeScopeRef,
	fallbackWorker models.LocalWorker,
	fallbackResources []models.LocalResource,
) (models.RuntimeScopeRef, models.LocalWorker, []models.LocalResource) {
	projection := request.ModelRuntime
	if projection == nil || projection.Scope.IsZero() {
		return fallbackScope, fallbackWorker, snapshotResources(fallbackResources)
	}
	worker := snapshotWorker(projection.Worker)
	if worker.Name == "" && worker.Model == "" {
		worker = fallbackWorker
	}
	resources := snapshotResources(projection.Resources)
	if len(resources) == 0 {
		resources = snapshotResources(fallbackResources)
	}
	return projection.Scope, worker, resources
}

func delegateRequest(request workers.RunnerExecutionRequest) workers.RunnerExecutionRequest {
	// A request selected only the private inference strategy when it carried
	// no provider runner. Delegate fallback still needs the provider identity
	// that was resolved on the model target.
	if workers.NormalizeRunnerID(request.RunnerID) == Identity {
		if provider := workers.NormalizeRunnerID(request.ModelProvider); provider != "" {
			request.RunnerID = provider
		}
	}
	return request
}

func validateRequest(request workers.RunnerExecutionRequest) error {
	if request.RunnerID != Identity {
		return badRequest(fmt.Sprintf("inference runner identity must be %q", Identity), nil)
	}
	for _, capability := range request.RequiredOptionalCapabilities {
		switch capability {
		case workers.RunnerOptionalCapabilityWorkingDirectory,
			workers.RunnerOptionalCapabilityWorktree:
		default:
			return &workers.UnsupportedRunnerCapabilityError{
				RunnerID:   Identity,
				Capability: capability,
			}
		}
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
	var providerErr *workers.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		return err
	}
	failure, ok := workers.ClassifyInferenceFailure(err, workers.InferenceFailureContext{
		ModelName:  firstNonEmpty(request.Model, r.worker.Model),
		WorkerName: firstNonEmpty(request.WorkerName, request.WorkerType, r.worker.Name),
		Operation:  request.ModelOperation,
	})
	if ok {
		return failure
	}
	return err
}

func (r *runner) workerForRequest(request workers.RunnerExecutionRequest) models.LocalWorker {
	worker := snapshotWorker(r.worker)
	if name := strings.TrimSpace(request.WorkerName); name != "" {
		worker.Name = name
	}
	if model := strings.TrimSpace(request.Model); model != "" {
		worker.Model = model
	}
	if locality := strings.TrimSpace(request.ModelLocality); locality != "" {
		worker.ModelLocality = locality
	}
	return worker
}

func validateWorker(worker models.LocalWorker) error {
	if strings.TrimSpace(worker.Name) == "" {
		return misconfigured("inference worker name is required", nil)
	}
	if worker.UsesManagedRuntime() && strings.TrimSpace(worker.Model) == "" {
		return misconfigured("inference worker configuration is invalid", models.ErrNotFound)
	}
	return nil
}

func invocationHolder(request workers.RunnerExecutionRequest) string {
	if dispatchID := strings.TrimSpace(request.Dispatch.DispatchID); dispatchID != "" {
		return dispatchID
	}
	return strings.TrimSpace(request.Dispatch.WorkstationName)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
