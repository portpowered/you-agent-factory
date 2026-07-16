package modelcatalog

import (
	"context"

	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
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
	return a.inner.InvokeModel(ctx, modelName, request)
}

var _ apisurface.ModelAPI = (*Adapter)(nil)
