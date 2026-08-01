package http

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ListModels decodes the owned list-models HTTP inputs, invokes the accepted
// Models root, and encodes the public list response shape.
func (a *Adapter) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	if a == nil || a.models == nil {
		return factoryapi.ListModelsResponse{}, errModelsServiceRequired
	}
	if a.scope.IsZero() {
		return factoryapi.ListModelsResponse{}, models.ErrRuntimeScopeInvalid
	}
	request := listModelsRequestFromHTTP(a.scope)
	scoped, err := a.models.ListCatalog(ctx, request)
	if err != nil {
		return factoryapi.ListModelsResponse{}, err
	}
	return listToGenerated(models.List{Results: scoped.Models}), nil
}

// GetModel decodes the owned get-model HTTP path input, invokes the accepted
// Models root, and encodes the public detail response shape.
func (a *Adapter) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	if a == nil || a.models == nil {
		return factoryapi.ModelDetail{}, errModelsServiceRequired
	}
	if a.scope.IsZero() {
		return factoryapi.ModelDetail{}, models.ErrRuntimeScopeInvalid
	}
	request, err := getModelRequestFromHTTP(modelName, a.scope)
	if err != nil {
		return factoryapi.ModelDetail{}, err
	}
	scoped, err := a.models.GetCatalogModel(ctx, request)
	if err != nil {
		return factoryapi.ModelDetail{}, err
	}
	return detailToGenerated(scoped.Model), nil
}

func listModelsRequestFromHTTP(scope models.RuntimeScopeRef) models.ListModelsRequest {
	return models.ListModelsRequest{Scope: scope}
}

func getModelRequestFromHTTP(modelName string, scope models.RuntimeScopeRef) (models.GetModelRequest, error) {
	request := models.GetModelRequest{Scope: scope, Name: modelName}
	if err := request.Validate(); err != nil {
		return models.GetModelRequest{}, err
	}
	return request, nil
}
