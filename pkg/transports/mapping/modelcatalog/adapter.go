package modelcatalog

import (
	"context"
	"errors"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/models/inference"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	contentmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	workerinferencemapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerinference"
)

// Adapter maps model-owned service values at the outward transport boundary.
type Adapter struct {
	inner *modelsservice.Service
}

// NewAdapter constructs a transport-facing adapter for one model service.
func NewAdapter(inner *modelsservice.Service) *Adapter {
	if inner == nil {
		return nil
	}
	return &Adapter{inner: inner}
}

func (a *Adapter) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	models, err := a.inner.ListModels(ctx)
	if err != nil {
		return factoryapi.ListModelsResponse{}, err
	}
	return ListToGenerated(models), nil
}

func (a *Adapter) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	model, err := a.inner.GetModel(ctx, modelName)
	if err != nil {
		return factoryapi.ModelDetail{}, err
	}
	return DetailToGenerated(model), nil
}

func (a *Adapter) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	return a.inner.PullModel(ctx, modelName)
}

func (a *Adapter) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	result, err := a.inner.InvokeModel(ctx, modelName, invocationRequestFromGenerated(request))
	if err == nil {
		return result, nil
	}
	return apisurface.ModelInvocationResult{}, invocationErrorFromDomain(err, modelName, request.Operation)
}

func invocationErrorFromDomain(err error, modelName, operation string) error {
	failureContext := apisurface.InferenceFailureContext{
		ModelName: strings.TrimSpace(modelName),
		Operation: strings.TrimSpace(operation),
	}
	var targeted *modelinference.TargetError
	if errors.As(err, &targeted) && targeted != nil {
		failureContext.ModelName = targeted.ModelName
		failureContext.WorkerName = targeted.WorkerName
		failureContext.Operation = targeted.Operation
	}
	if failure, ok := apisurface.ClassifyInferenceFailure(err, failureContext); ok {
		return failure
	}
	return err
}

func invocationRequestFromGenerated(request factoryapi.ModelInvocationRequest) modelinference.Request {
	domain := modelinference.Request{
		Operation: request.Operation,
		Content:   contentmapping.PartsFromGenerated(request.Content),
		Bindings:  workerinferencemapping.OperationBindingsFromGenerated(request.Bindings),
	}
	if request.Options != nil {
		domain.Options = &modelinference.Options{}
		if request.Options.ResponseMode != nil {
			domain.Options.ResponseMode = modelinference.ResponseMode(*request.Options.ResponseMode)
		}
	}
	return domain
}

var _ apisurface.ModelAPI = (*Adapter)(nil)
