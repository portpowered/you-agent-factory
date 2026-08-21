package http

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

var (
	errModelsServiceRequired           = errors.New("Models service is required")
	errModelInvocationServicesRequired = errors.New("model invocation and Work content preparation services are required")
)

// Adapter maps Models service values at the outward HTTP boundary.
type Adapter struct {
	models  models.Service
	scope   models.RuntimeScopeRef
	invoker ModelInvoker
	content work.ContentPreparation
}

// NewAdapter constructs the Models HTTP representation adapter.
func NewAdapter(
	service models.Service,
	invoker ModelInvoker,
	content work.ContentPreparation,
	scopes ...models.RuntimeScopeRef,
) *Adapter {
	if service == nil || invoker == nil || content == nil {
		return nil
	}
	var scope models.RuntimeScopeRef
	if len(scopes) > 0 {
		scope = scopes[0]
	}
	return &Adapter{models: service, scope: scope, invoker: invoker, content: content}
}

// Root returns the accepted Models root consumed by adapter-owned operations.
func (a *Adapter) Root() models.Service {
	if a == nil {
		return nil
	}
	return a.models
}

func (a *Adapter) InvokeModel(
	ctx context.Context,
	modelName string,
	request factoryapi.ModelInvocationRequest,
) (models.Result, error) {
	if a == nil || a.invoker == nil || a.content == nil {
		return models.Result{}, errModelInvocationServicesRequired
	}
	mapped := modelInvocationRequestFromHTTP(request)
	prepared, err := a.content.PrepareWorkContent(ctx, mapped.Content)
	if err != nil {
		return models.Result{}, err
	}
	mapped.Content = prepared
	return a.invoker.InvokeModel(ctx, modelName, mapped)
}

// InvokeGenericModel maps and validates the provider-neutral request before it
// crosses the Models root boundary. The root owns resolution, preparation,
// host readiness, capacity, execution, and compensating release.
func (a *Adapter) InvokeGenericModel(
	ctx context.Context,
	request factoryapi.GenericModelInvocationRequest,
) (models.GenericInvocationResult, error) {
	if a == nil || a.models == nil {
		return models.GenericInvocationResult{}, errModelsServiceRequired
	}
	if a.scope.IsZero() {
		return models.GenericInvocationResult{}, models.ErrRuntimeScopeInvalid
	}
	mapped, err := GenericInvocationRequestFromGenerated(request)
	if err != nil {
		return models.GenericInvocationResult{}, err
	}
	// The HTTP handler is already bound to the live Models runtime scope. Keep
	// that process-owned scope authoritative instead of trusting a caller to
	// select a different runtime through the generic request body.
	mapped.Scope = a.scope
	if err := mapped.ValidateGeneric(); err != nil {
		return models.GenericInvocationResult{}, err
	}
	return a.models.InvokeModel(ctx, mapped)
}
