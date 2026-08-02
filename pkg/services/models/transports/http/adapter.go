package http

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

var (
	errModelsServiceRequired = errors.New("Models service is required")
)

const modelsHTTPInvokeHolder = "you-models-http-invoke"

// Adapter maps Models service values at the outward HTTP boundary.
type Adapter struct {
	models models.Service
	scope  models.RuntimeScopeRef
}

type modelInvocationHTTPResult struct {
	result            models.InvokeModelResult
	catalog           models.Detail
	input             models.InferenceInput
	inputContent      []work.WorkContentPart
	streamFile        string
	streamContentType string
}

// NewAdapter constructs the Models HTTP representation adapter.
func NewAdapter(
	service models.Service,
	scopes ...models.RuntimeScopeRef,
) *Adapter {
	if service == nil {
		return nil
	}
	var scope models.RuntimeScopeRef
	if len(scopes) > 0 {
		scope = scopes[0]
	}
	return &Adapter{models: service, scope: scope}
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
) (modelInvocationHTTPResult, error) {
	if a == nil || a.models == nil {
		return modelInvocationHTTPResult{}, errModelsServiceRequired
	}
	if a.scope.IsZero() {
		return modelInvocationHTTPResult{}, models.ErrRuntimeScopeInvalid
	}
	if err := validateModelInvocationOperation(request); err != nil {
		return modelInvocationHTTPResult{}, err
	}

	input, inputContent, err := inferenceInputFromHTTP(request)
	if err != nil {
		return modelInvocationHTTPResult{}, err
	}
	catalogResult, err := a.models.GetCatalogModel(ctx, models.GetModelRequest{
		Scope: a.scope, Name: strings.TrimSpace(modelName), Operation: strings.TrimSpace(request.Operation),
	})
	if err != nil {
		return modelInvocationHTTPResult{}, err
	}
	if runtime := catalogResult.Model.ManagedRuntime; strings.TrimSpace(runtime.Identity) != "" {
		if err := runtime.InvocationError(); err != nil {
			return modelInvocationHTTPResult{}, err
		}
	}

	leaseResult, err := a.models.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{
		Scope: a.scope, Name: strings.TrimSpace(modelName), Holder: modelsHTTPInvokeHolder,
	})
	if err != nil {
		return modelInvocationHTTPResult{}, err
	}
	leaseOwned := true
	defer func() {
		if !leaseOwned {
			return
		}
		_, _ = a.models.ReleaseModelLease(context.WithoutCancel(ctx), models.ReleaseModelLeaseRequest{
			Scope: a.scope, Lease: leaseResult.Lease.Lease,
		})
	}()

	rootRequest := models.InvokeModelRequest{
		Scope:     a.scope,
		Lease:     leaseResult.Lease.Lease,
		Holder:    modelsHTTPInvokeHolder,
		ModelName: strings.TrimSpace(modelName),
		Operation: strings.TrimSpace(request.Operation),
		Input:     input,
	}
	if request.Options != nil && request.Options.ResponseMode != nil &&
		string(*request.Options.ResponseMode) == string(factoryapi.AUDIOSTREAM) {
		rootRequest.ResponseMode = models.ResponseModeAudioStream
	}
	result, invokeErr := a.models.InvokeModelWithLease(ctx, rootRequest)
	if invokeErr != nil {
		if result.LeaseDisposition != "" {
			leaseOwned = false
		}
		return modelInvocationHTTPResult{}, invokeErr
	}
	leaseOwned = false
	if result.ModelName == "" {
		result.ModelName = strings.TrimSpace(modelName)
	}
	if result.Operation == "" {
		result.Operation = strings.TrimSpace(request.Operation)
	}

	streamFile, streamContentType, err := invocationStreamFromResult(result, request)
	if err != nil {
		return modelInvocationHTTPResult{}, err
	}
	return modelInvocationHTTPResult{
		result: result, catalog: catalogResult.Model, input: input,
		inputContent: inputContent, streamFile: streamFile, streamContentType: streamContentType,
	}, nil
}
