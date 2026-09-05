package service

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

func (o *Root) CloseRuntimeScope(
	ctx context.Context,
	request models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	if o == nil || o.runtimeScopes == nil {
		return models.CloseRuntimeScopeResult{}, models.ErrUnsupportedOperation
	}
	if err := ctx.Err(); err != nil {
		return models.CloseRuntimeScopeResult{}, err
	}
	if request.Scope.IsZero() {
		return models.CloseRuntimeScopeResult{}, models.ErrRuntimeScopeInvalid
	}
	err := o.runtimeScopes.Close(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.CloseRuntimeScopeResult{}, runtimeScopeError(err)
	}
	o.runtimeMu.Lock()
	delete(o.runtimeByScope, request.Scope)
	o.runtimeMu.Unlock()
	if closer, ok := o.runtimeHost.(interface {
		CloseRuntimeScope(context.Context, models.RuntimeScopeRef) error
	}); ok {
		if err := closer.CloseRuntimeScope(context.WithoutCancel(ctx), request.Scope); err != nil {
			return models.CloseRuntimeScopeResult{}, err
		}
	}
	return models.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
}
