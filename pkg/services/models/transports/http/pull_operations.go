package http

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// PullModel decodes the owned pull-model HTTP path input, invokes the accepted
// Models root, and returns the pull result for HTTP encoding.
func (a *Adapter) PullModel(ctx context.Context, modelName string) (models.PullResult, error) {
	if a == nil || a.models == nil {
		return models.PullResult{}, errModelsServiceRequired
	}
	request, err := pullModelRequestFromHTTP(modelName, a.scope)
	if err != nil {
		return models.PullResult{}, err
	}
	if a.scope.IsZero() {
		return a.models.PullModel(ctx, request.Name)
	}
	return a.models.PullModelForScope(ctx, request)
}

func pullModelRequestFromHTTP(modelName string, scope models.RuntimeScopeRef) (models.PullModelRequest, error) {
	request := models.PullModelRequest{Scope: scope, Name: modelName}
	if err := models.ValidatePullModelRequest(request); err != nil {
		return models.PullModelRequest{}, err
	}
	return request, nil
}
