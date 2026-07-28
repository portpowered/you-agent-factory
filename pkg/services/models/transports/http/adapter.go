package http

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	workerinferencemapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerinference"
)

var (
	errModelsServiceRequired           = errors.New("Models service is required")
	errModelInvocationServicesRequired = errors.New("model invocation and Work content preparation services are required")
)

// Adapter maps Models service values at the outward HTTP boundary.
type Adapter struct {
	models  models.Service
	scope   models.RuntimeScopeRef
	invoker workers.ModelInvoker
	content work.ContentPreparation
}

// NewAdapter constructs the Models HTTP representation adapter.
func NewAdapter(
	service models.Service,
	invoker workers.ModelInvoker,
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

func (a *Adapter) PullModel(ctx context.Context, modelName string) (models.PullResult, error) {
	if a == nil || a.models == nil {
		return models.PullResult{}, errModelsServiceRequired
	}
	if !a.scope.IsZero() {
		return a.models.PullModelForScope(ctx, models.PullModelRequest{
			Scope: a.scope,
			Name:  modelName,
		})
	}
	return a.models.PullModel(ctx, modelName)
}

func (a *Adapter) InvokeModel(
	ctx context.Context,
	modelName string,
	request factoryapi.ModelInvocationRequest,
) (models.Result, error) {
	if a == nil || a.invoker == nil || a.content == nil {
		return models.Result{}, errModelInvocationServicesRequired
	}
	mapped := invocationRequestFromGenerated(request)
	prepared, err := a.content.PrepareWorkContent(ctx, mapped.Content)
	if err != nil {
		return models.Result{}, err
	}
	mapped.Content = prepared
	return a.invoker.InvokeModel(ctx, modelName, mapped)
}

func invocationRequestFromGenerated(request factoryapi.ModelInvocationRequest) models.Request {
	domain := models.Request{
		Operation: request.Operation,
		Content:   contentmapping.PartsFromGenerated(request.Content),
		Bindings:  modelBindingsFromGenerated(request.Bindings),
	}
	if request.Options != nil {
		domain.Options = &models.Options{}
		if request.Options.ResponseMode != nil {
			domain.Options.ResponseMode = models.ResponseMode(*request.Options.ResponseMode)
		}
	}
	return domain
}

func modelBindingsFromGenerated(bindings *[]factoryapi.WorkstationOperationBinding) []models.ModelOperationBinding {
	authored := workerinferencemapping.OperationBindingsFromGenerated(bindings)
	result := make([]models.ModelOperationBinding, len(authored))
	for i := range authored {
		result[i] = models.ModelOperationBinding{
			Slot: authored[i].Slot, Config: work.CloneWorkContentParts(authored[i].Config),
			DefaultContent: work.CloneWorkContentParts(authored[i].DefaultContent),
		}
		if authored[i].Selector != nil {
			result[i].Selector = &models.ModelOperationBindingSelector{
				Slot: authored[i].Selector.Slot, Label: authored[i].Selector.Label,
				Type: authored[i].Selector.Type, Role: authored[i].Selector.Role,
			}
		}
	}
	return result
}
