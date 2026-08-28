package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	"go.uber.org/zap"
)

// Root retains the process-wide external effect ports of the injected Models
// service. It is inert until it is bound to a Factory Session runtime.
type Root struct {
	processLauncher            modelhost.ProcessLauncher
	hostHTTP                   modelhost.HTTPDoer
	hostClock                  modelhost.Clock
	runtimeRunner              platformprocess.CommandRunner
	runtimeHTTP                localmodels.HTTPDoer
	runtimeInspect             localmodels.InspectFile
	runtimeTempDir             localmodels.TempDirectory
	runtimeTempFile            localmodels.CreateTempFile
	runtimeScopes              runtimescopes.Service
	assets                     scopedassets.Service
	runtimeHost                runtimehost.Service
	inference                  modelinference.Service
	resolveHuggingFaceRevision func(context.Context, string) (string, error)
	resolveBackendArtifact     modelseffects.BackendArtifactResolver
	cacheLifecycleMu           sync.Mutex
	runtimeMu                  sync.RWMutex
	runtimeByScope             map[models.RuntimeScopeRef]models.Service
	catalog                    modelcatalog.Service
	process                    modelseffects.ProcessDependencies
}

var _ models.Service = (*Root)(nil)

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func NewRoot(
	processLauncher modelhost.ProcessLauncher,
	hostHTTP modelhost.HTTPDoer,
	hostClock modelhost.Clock,
	runtimeRunner platformprocess.CommandRunner,
	runtimeHTTP localmodels.HTTPDoer,
	runtimeInspect localmodels.InspectFile,
	runtimeTempDir localmodels.TempDirectory,
	runtimeTempFile localmodels.CreateTempFile,
	runtimeScopes runtimescopes.Service,
	catalogService modelcatalog.Service,
	assetService scopedassets.Service,
	runtimeHostService runtimehost.Service,
	inferenceService modelinference.Service,
	processDependencies ...modelseffects.ProcessDependencies,
) (*Root, error) {
	if processLauncher == nil {
		return nil, missingDependencyError("model host process launcher")
	}
	if hostHTTP == nil {
		return nil, missingDependencyError("model host HTTP client")
	}
	if hostClock == nil {
		return nil, missingDependencyError("model host clock")
	}
	if runtimeRunner == nil {
		return nil, missingDependencyError("model runtime command runner")
	}
	if runtimeHTTP == nil {
		return nil, missingDependencyError("model runtime HTTP client")
	}
	if runtimeInspect == nil {
		return nil, missingDependencyError("model runtime file inspector")
	}
	if runtimeTempDir == nil {
		return nil, missingDependencyError("model runtime temporary directory resolver")
	}
	if runtimeTempFile == nil {
		return nil, missingDependencyError("model runtime temporary file creator")
	}
	if runtimeScopes == nil {
		return nil, missingDependencyError("Models Runtime Scopes service")
	}
	if catalogService == nil {
		return nil, missingDependencyError("Models Catalog service")
	}
	if assetService == nil {
		return nil, missingDependencyError("Models Assets service")
	}
	if runtimeHostService == nil {
		return nil, missingDependencyError("Models Runtime Host service")
	}
	if inferenceService == nil {
		return nil, missingDependencyError("Models Inference service")
	}
	process := modelseffects.ProcessDependencies{}
	if len(processDependencies) > 0 {
		process = processDependencies[0]
	}
	if process.Logger == nil {
		process.Logger = zap.NewNop()
	}
	if process.Clock == nil {
		return nil, missingDependencyError("Models process clock")
	}
	resolveRevision := process.ResolveHuggingFaceRevision
	if resolveRevision == nil {
		resolveRevision = defaultHuggingFaceRevision
	}
	return &Root{
		processLauncher: processLauncher, hostHTTP: hostHTTP, hostClock: hostClock,
		runtimeRunner: runtimeRunner, runtimeHTTP: runtimeHTTP,
		runtimeInspect: runtimeInspect, runtimeTempDir: runtimeTempDir, runtimeTempFile: runtimeTempFile,
		runtimeScopes: runtimeScopes, catalog: catalogService, assets: assetService,
		runtimeHost: runtimeHostService, inference: inferenceService,
		resolveHuggingFaceRevision: resolveRevision,
		resolveBackendArtifact:     process.ResolveBackendArtifact,
		runtimeByScope:             make(map[models.RuntimeScopeRef]models.Service),
		process:                    process,
	}, nil
}

func (o *Root) runtimeForBindingWithAssets(
	scope models.RuntimeScopeRef,
	binding models.RuntimeBinding,
	assets localmodels.AssetPuller,
) (models.Service, error) {
	localRuntime, err := localmodels.NewOmniVoiceRuntime(
		o.runtimeRunner, o.runtimeHTTP, o.runtimeInspect, o.runtimeTempDir, o.runtimeTempFile,
	)
	if err != nil {
		return nil, err
	}
	return newRuntimeWithHostEdges(
		scope,
		binding.RuntimeConfig,
		o.process.Logger,
		o.process.Clock,
		o.process.PullMetrics,
		o.process.HostLogger,
		o.process.HostMetrics,
		o.process.LocalHooks,
		assets,
		localRuntime,
		o.runtimeHost,
		nil,
	)
}

func (o *Root) OpenRuntimeScope(
	ctx context.Context,
	request models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	if o == nil || o.runtimeScopes == nil {
		return models.OpenRuntimeScopeResult{}, models.ErrUnsupportedOperation
	}
	if err := ctx.Err(); err != nil {
		return models.OpenRuntimeScopeResult{}, err
	}
	config := request.Config.Clone()
	binding := models.RuntimeBinding{
		CacheDirectory: config.CacheDirectory,
		OperatorModels: cloneModelOverlays(config.OperatorModels),
		RuntimeConfig: func() *models.RuntimeConfig {
			runtimeConfig := config.Runtime
			return &runtimeConfig
		},
	}
	ref, err := o.runtimeScopes.Open(binding)
	if err != nil {
		return models.OpenRuntimeScopeResult{}, err
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(string(ref))
	if err != nil {
		return models.OpenRuntimeScopeResult{}, err
	}
	return models.OpenRuntimeScopeResult{Scope: scope}, nil
}

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
	return models.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
}

// Close shuts down every supervised model host retained by the process-wide
// Models root. This is an internal process-lifecycle hook; the public Models
// contract remains focused on scoped customer operations.
func (o *Root) Close(ctx context.Context) error {
	if o == nil || o.runtimeHost == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	shutdown, ok := o.runtimeHost.(interface {
		Shutdown(context.Context) error
	})
	if !ok {
		return fmt.Errorf("Models runtime host does not support process shutdown")
	}
	if err := shutdown.Shutdown(ctx); err != nil {
		return err
	}
	o.runtimeMu.Lock()
	o.runtimeByScope = make(map[models.RuntimeScopeRef]models.Service)
	o.runtimeMu.Unlock()
	return nil
}

func (o *Root) ListCatalog(
	ctx context.Context,
	request models.ListModelsRequest,
) (models.ListModelsResult, error) {
	if o == nil || o.catalog == nil {
		return models.ListModelsResult{}, models.ErrUnsupportedOperation
	}
	return o.catalog.ListCatalog(ctx, request)
}

func (o *Root) GetCatalogModel(
	ctx context.Context,
	request models.GetModelRequest,
) (models.GetModelResult, error) {
	if o == nil || o.catalog == nil {
		return models.GetModelResult{}, models.ErrUnsupportedOperation
	}
	return o.catalog.GetCatalogModel(ctx, request)
}

func (o *Root) GetModelReadiness(
	ctx context.Context,
	request models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	if o == nil || o.catalog == nil {
		return models.GetModelReadinessResult{}, models.ErrUnsupportedOperation
	}
	return o.catalog.GetModelReadiness(ctx, request)
}

func cloneModelOverlays(overlays map[string]models.ModelOverlay) map[string]models.ModelOverlay {
	if overlays == nil {
		return nil
	}
	cloned := make(map[string]models.ModelOverlay, len(overlays))
	for name, overlay := range overlays {
		cloned[name] = overlay.Clone()
	}
	return cloned
}

func runtimeScopeError(err error) error {
	switch {
	case errors.Is(err, runtimescopes.ErrScopeForeign):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeForeign, err)
	case errors.Is(err, runtimescopes.ErrScopeClosed):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeClosed, err)
	case errors.Is(err, runtimescopes.ErrScopeUnknown):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeStale, err)
	default:
		return models.ErrUnavailable
	}
}

func (o *Root) PrepareModelAssets(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	if o == nil || o.assets == nil {
		return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
	}
	o.cacheLifecycleMu.Lock()
	defer o.cacheLifecycleMu.Unlock()
	return o.assets.PrepareModelAssets(ctx, request)
}

func (o *Root) PullModelForScope(
	ctx context.Context,
	request models.PullModelRequest,
) (models.PullResult, error) {
	if err := models.ValidatePullModelRequest(request); err != nil {
		return models.PullResult{}, err
	}
	o.cacheLifecycleMu.Lock()
	defer o.cacheLifecycleMu.Unlock()
	runtime, err := o.scopedRuntime(request.Scope)
	if err != nil {
		return models.PullResult{}, err
	}
	puller, ok := runtime.(interface {
		PullModel(context.Context, string) (models.PullResult, error)
	})
	if !ok {
		return models.PullResult{}, models.ErrUnsupportedOperation
	}
	result, err := puller.PullModel(ctx, request.Name)
	if err == nil || !errors.Is(err, models.ErrNotFound) {
		return result, err
	}
	return o.pullResolvedModelAfterCatalogMiss(ctx, request, err)
}

func (o *Root) InspectModelAssets(
	ctx context.Context,
	request models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	if o == nil || o.assets == nil {
		return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
	}
	return o.assets.InspectModelAssets(ctx, request)
}

func (o *Root) RemoveModelAssets(
	ctx context.Context,
	request models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	if o == nil || o.assets == nil {
		return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
	}
	if err := request.Validate(); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	started := joinedInvocationStart(o)
	removeModelCacheStartLog(o, request)
	o.cacheLifecycleMu.Lock()
	result, err := o.removeModelAssets(ctx, request)
	o.cacheLifecycleMu.Unlock()
	removeModelCacheTerminalLog(o, request, result, err, joinedInvocationElapsed(o, started))
	return result, err
}

func (o *Root) removeModelAssets(
	ctx context.Context,
	request models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	// Runtime Host owns the live process and lease state. Stop it before the
	// Assets service mutates the selected cache, while preserving the Assets
	// service's no-cache classification for absent or unsupported models.
	if o.runtimeHost != nil {
		inspection, inspectErr := o.assets.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
			Scope: request.Scope,
			Name:  request.Name,
		})
		if inspectErr != nil && !isRemovableCacheAbsence(inspectErr) {
			return models.RemoveModelAssetsResult{}, inspectErr
		}
		if inspectErr == nil && inspection.Installed {
			if _, stopErr := o.runtimeHost.StopModelHost(ctx, models.StopModelHostRequest{
				Scope: request.Scope,
				Name:  request.Name,
			}); stopErr != nil {
				if errors.Is(stopErr, models.ErrHostCapacityExhausted) {
					return models.RemoveModelAssetsResult{}, fmt.Errorf(
						"%w: %s", models.ErrModelCacheInUse, strings.TrimSpace(request.Name),
					)
				}
				return models.RemoveModelAssetsResult{}, stopErr
			}
		}
	}
	return o.assets.RemoveModelAssets(ctx, request)
}

func isRemovableCacheAbsence(err error) bool {
	return errors.Is(err, models.ErrModelCacheNotFound) ||
		errors.Is(err, models.ErrAssetSourceMissing) ||
		errors.Is(err, models.ErrAssetSourceUnsupported) ||
		errors.Is(err, models.ErrAssetUnavailable) ||
		errors.Is(err, models.ErrNotAvailable)
}

func (o *Root) EnsureModelHost(
	ctx context.Context,
	request models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	if o == nil || o.runtimeHost == nil {
		return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
	}
	o.cacheLifecycleMu.Lock()
	defer o.cacheLifecycleMu.Unlock()
	return o.runtimeHost.EnsureModelHost(ctx, request)
}

func (o *Root) InspectModelHost(
	ctx context.Context,
	request models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	if o == nil || o.runtimeHost == nil {
		return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
	}
	return o.runtimeHost.InspectModelHost(ctx, request)
}

func (o *Root) StopModelHost(
	ctx context.Context,
	request models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	if o == nil || o.runtimeHost == nil {
		return models.StopModelHostResult{}, models.ErrUnsupportedOperation
	}
	o.cacheLifecycleMu.Lock()
	defer o.cacheLifecycleMu.Unlock()
	return o.runtimeHost.StopModelHost(ctx, request)
}

func (o *Root) AcquireModelLease(
	ctx context.Context,
	request models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	if o == nil || o.runtimeHost == nil {
		return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
	}
	o.cacheLifecycleMu.Lock()
	defer o.cacheLifecycleMu.Unlock()
	return o.runtimeHost.AcquireModelLease(ctx, request)
}

func (o *Root) GetModelLease(
	ctx context.Context,
	request models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	if o == nil || o.runtimeHost == nil {
		return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
	}
	return o.runtimeHost.GetModelLease(ctx, request)
}

func (o *Root) ReleaseModelLease(
	ctx context.Context,
	request models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	if o == nil || o.runtimeHost == nil {
		return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
	}
	o.cacheLifecycleMu.Lock()
	defer o.cacheLifecycleMu.Unlock()
	return o.runtimeHost.ReleaseModelLease(ctx, request)
}

func (o *Root) InvokeModelWithLease(
	ctx context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	if o == nil || o.inference == nil {
		return models.InvokeModelResult{}, models.ErrUnsupportedOperation
	}
	// The lease-backed inference owner keeps the live lease registered for the
	// duration of the call. Removal can therefore inspect that state and return
	// ErrModelCacheInUse instead of waiting for a long-running inference.
	return o.inference.InvokeModelWithLease(ctx, request)
}

type joinedInvocationPlan struct {
	modelName string
	operation models.Operation
	prepared  models.InvokeModelRequest
	lease     models.ModelLeaseRef
}

const joinedInvocationFailureRuntime = "runtime_failure"

// InvokeModel owns the complete prepared-model transaction. The injected
// Inference owner remains responsible for the lease-backed primitive; this
// root method supplies the preparation and compensating lifecycle around it.
func (o *Root) InvokeModel(
	ctx context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := joinedInvocationStart(o)
	stage := "validate"
	modelName := ""
	operationName := request.Operation
	var invocation models.ModelInvocationRef
	finish := func(result models.InvokeModelResult, err error) (models.InvokeModelResult, error) {
		if result.ModelName != "" {
			modelName = result.ModelName
		}
		if result.Operation != "" {
			operationName = result.Operation
		}
		if !result.Invocation.IsZero() {
			invocation = result.Invocation
		}
		if err != nil {
			result = joinedInvocationFailureResult(result)
		}
		joinedInvocationRecord(
			o, modelName, operationName, invocation, stage, err,
			joinedInvocationElapsed(o, started),
		)
		return result.Clone(), err
	}

	if err := validateJoinedRoot(o); err != nil {
		return finish(models.InvokeModelResult{}, err)
	}
	plan, preparedStage, err := o.prepareJoinedInvocation(ctx, request)
	stage = preparedStage
	modelName = plan.modelName
	operationName = plan.operation.Name
	if err != nil {
		return finish(models.InvokeModelResult{}, joinedInvocationContextError(ctx, err))
	}

	result, invokeStage, err := o.executeJoinedInvocation(ctx, plan)
	stage = invokeStage
	return finish(result, joinedInvocationContextError(ctx, err))
}

func validateJoinedRoot(o *Root) error {
	if o == nil || o.runtimeScopes == nil || o.assets == nil || o.runtimeHost == nil || o.inference == nil {
		return models.ErrUnsupportedOperation
	}
	return nil
}

func (o *Root) prepareJoinedInvocation(
	ctx context.Context,
	request models.InvokeModelRequest,
) (joinedInvocationPlan, string, error) {
	plan := joinedInvocationPlan{}
	if err := request.ValidateGeneric(); err != nil {
		return plan, "validate", err
	}

	resolution, err := o.ResolveModelReference(ctx, models.ResolveModelReferenceRequest{
		Scope: request.Scope, Reference: request.Model,
	})
	if err != nil {
		return plan, "resolve", err
	}
	resolved := resolution.Resolved
	plan.modelName = resolved.Definition.Name
	plan.prepared, plan.operation, err = models.PrepareGenericInvocation(request, resolved.Definition)
	if err != nil {
		return plan, "resolve", err
	}
	plan.prepared.ModelName = plan.modelName
	plan.prepared.Operation = plan.operation.Name
	if len(plan.prepared.Inputs) > 0 && inferenceInputIsZero(plan.prepared.Input) {
		plan.prepared.Input = plan.prepared.Inputs[0].Clone()
	}

	backendArtifact, err := o.resolveJoinedBackendArtifact(ctx, resolved.Definition)
	if err != nil {
		return plan, "resolve_backend", err
	}
	assetRequest := joinedAssetPreparationRequestWithBackend(
		request, plan.modelName, resolved, backendArtifact,
	)
	if _, err := o.PreflightModelAssets(ctx, assetRequest); err != nil {
		return plan, "preflight_assets", joinedInvocationAssetError(request, err)
	}
	if _, err := o.PrepareModelAssets(ctx, assetRequest); err != nil {
		return plan, "acquire_assets", joinedInvocationAssetError(request, err)
	}
	if _, err := o.EnsureModelHost(ctx, models.EnsureModelHostRequest{
		Scope: request.Scope,
		Name:  plan.modelName,
	}); err != nil {
		return plan, "ensure_host", err
	}
	leaseResult, err := o.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{
		Scope: request.Scope, Name: plan.modelName, Holder: request.Holder,
	})
	if err != nil {
		return plan, "acquire_lease", err
	}
	plan.lease = leaseResult.Lease.Lease
	if plan.lease.IsZero() {
		return plan, "acquire_lease", models.ErrHostLeaseNotFound
	}
	plan.prepared.Lease = plan.lease
	return plan, "invoke", nil
}

func (o *Root) resolveJoinedBackendArtifact(
	ctx context.Context,
	definition models.ModelDefinition,
) (modelseffects.BackendArtifactSelection, error) {
	if !isJoinedPinnedBackend(definition.Backend) {
		return modelseffects.BackendArtifactSelection{}, nil
	}
	if o == nil || o.resolveBackendArtifact == nil {
		return modelseffects.BackendArtifactSelection{}, fmt.Errorf(
			"%w: pinned backend artifact selector is unavailable",
			models.ErrHostMissingAssets,
		)
	}
	selection, err := o.resolveBackendArtifact(ctx, modelseffects.BackendArtifactSelectionRequest{
		Backend:         strings.TrimSpace(definition.Backend),
		Platform:        o.process.BackendArtifactPlatform,
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	})
	if err != nil {
		return modelseffects.BackendArtifactSelection{}, fmt.Errorf(
			"%w: pinned backend artifact selection failed",
			models.ErrHostMissingAssets,
		)
	}
	requirement := models.AssetRequirement{
		Name: selection.Name, Bytes: selection.Bytes, SHA256: selection.SHA256,
	}
	if strings.TrimSpace(selection.Location) == "" || selection.Bytes <= 0 || requirement.Validate() != nil {
		return modelseffects.BackendArtifactSelection{}, fmt.Errorf(
			"%w: pinned backend artifact facts are invalid",
			models.ErrHostMissingAssets,
		)
	}
	return selection, nil
}

func (o *Root) executeJoinedInvocation(
	ctx context.Context,
	plan joinedInvocationPlan,
) (models.InvokeModelResult, string, error) {
	result, err := o.InvokeModelWithLease(ctx, plan.prepared)
	result = joinedInvocationResultIdentity(result, plan)
	if err != nil {
		return o.finishJoinedFailure(ctx, plan, result, "invoke", err)
	}
	if result.Status == models.ModelInvocationStatusCompleted {
		result.Outputs, err = models.NormalizeGenericInvocationOutputs(
			plan.operation, result.Content, result.Artifacts,
		)
		if err != nil {
			result.Status = models.ModelInvocationStatusFailed
			return o.finishJoinedFailure(ctx, plan, result, "invoke", err)
		}
	}
	if result.Status == models.ModelInvocationStatusAccepted {
		result.Status = models.ModelInvocationStatusFailed
		return o.finishJoinedFailure(
			ctx, plan, result, "invoke",
			fmt.Errorf("%w: joined invocation did not complete", models.ErrInferenceFailed),
		)
	}
	if result.Status == models.ModelInvocationStatusFailed {
		return o.finishJoinedFailure(ctx, plan, result, "invoke", models.ErrInferenceFailed)
	}
	if result.Status == models.ModelInvocationStatusCancelled {
		return o.finishJoinedFailure(ctx, plan, result, "invoke", models.ErrInferenceCancelled)
	}
	if result.Status != models.ModelInvocationStatusCompleted {
		result.Status = models.ModelInvocationStatusFailed
		return o.finishJoinedFailure(ctx, plan, result, "invoke", models.ErrInferenceFailed)
	}
	return o.releaseJoinedInvocation(ctx, plan, result)
}

func (o *Root) finishJoinedFailure(
	ctx context.Context,
	plan joinedInvocationPlan,
	result models.InvokeModelResult,
	stage string,
	invokeErr error,
) (models.InvokeModelResult, string, error) {
	if !joinedInvocationLeaseReleased(result) {
		releaseErr := o.releaseJoinedLease(ctx, plan.prepared.Scope, plan.lease)
		if releaseErr == nil {
			result.LeaseDisposition = models.InvocationLeaseReleased
		}
		invokeErr = errors.Join(invokeErr, releaseErr)
	}
	return result, stage, invokeErr
}

func (o *Root) releaseJoinedInvocation(
	ctx context.Context,
	plan joinedInvocationPlan,
	result models.InvokeModelResult,
) (models.InvokeModelResult, string, error) {
	if joinedInvocationLeaseReleased(result) {
		return result, "release", nil
	}
	releaseErr := o.releaseJoinedLease(ctx, plan.prepared.Scope, plan.lease)
	if releaseErr == nil {
		result.LeaseDisposition = models.InvocationLeaseReleased
	} else {
		result.Status = models.ModelInvocationStatusFailed
	}
	return result, "release", releaseErr
}

func joinedInvocationResultIdentity(
	result models.InvokeModelResult,
	plan joinedInvocationPlan,
) models.InvokeModelResult {
	result.Scope = plan.prepared.Scope
	result.Lease = plan.lease
	result.ModelName = plan.modelName
	result.Operation = plan.operation.Name
	return result
}

func joinedInvocationFailureResult(result models.InvokeModelResult) models.InvokeModelResult {
	result.Content = nil
	result.Artifacts = nil
	result.Outputs = nil
	return result
}

func joinedAssetReference(
	reference models.ModelReference,
	resolved models.ResolvedModelReference,
) models.ModelReference {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(resolved.Definition.Source)), "hf://") {
		return models.ModelReference{NameOrURI: resolved.Definition.Source}
	}
	return reference
}

func joinedAssetPreparationRequest(
	request models.InvokeModelRequest,
	modelName string,
	resolved models.ResolvedModelReference,
) models.PrepareModelAssetsRequest {
	return joinedAssetPreparationRequestWithBackend(
		request, modelName, resolved, modelseffects.BackendArtifactSelection{},
	)
}

func joinedAssetPreparationRequestWithBackend(
	request models.InvokeModelRequest,
	modelName string,
	resolved models.ResolvedModelReference,
	backendArtifact modelseffects.BackendArtifactSelection,
) models.PrepareModelAssetsRequest {
	assetReference := joinedAssetReference(request.Model, resolved)
	prepared := models.PrepareModelAssetsRequest{
		Scope:     request.Scope,
		Name:      modelName,
		Reference: assetReference,
		Offline:   request.Offline,
		Backend:   strings.TrimSpace(resolved.Definition.Backend),
		Artifacts: joinedSourceAssetRequirements(assetReference.NameOrURI),
	}
	if backendArtifact.Name != "" {
		prepared.BackendReference = models.ModelReference{NameOrURI: backendArtifact.Location}
		prepared.BackendArtifacts = []models.AssetRequirement{{
			Name: backendArtifact.Name, Bytes: backendArtifact.Bytes, SHA256: backendArtifact.SHA256,
		}}
		return prepared
	}
	if backend := strings.TrimSpace(resolved.Definition.Backend); isJoinedSourceReference(backend) {
		prepared.Backend = ""
		prepared.BackendReference = models.ModelReference{NameOrURI: backend}
		prepared.BackendArtifacts = joinedSourceAssetRequirements(backend)
	}
	return prepared
}

func isJoinedPinnedBackend(value string) bool {
	canonical := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(canonical, "localai-") || canonical == "localai" ||
		canonical == "localai_grpc" || canonical == "localai-grpc"
}

func isJoinedSourceReference(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "hf://") || strings.HasPrefix(lower, "file://") ||
		strings.HasPrefix(lower, "./") || strings.HasPrefix(lower, "../") ||
		strings.HasPrefix(lower, "/") || strings.HasPrefix(lower, "\\") ||
		(len(lower) > 2 && lower[1] == ':')
}

func joinedSourceAssetRequirements(source string) []models.AssetRequirement {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(source), "hf://") {
		rest := strings.TrimPrefix(source, "hf://")
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			rest = rest[:at]
		}
		parts := strings.Split(rest, "/")
		if len(parts) > 2 {
			name := path.Clean(strings.Join(parts[2:], "/"))
			if name != "." && name != "" {
				return []models.AssetRequirement{{Name: name}}
			}
		}
		return nil
	}
	if strings.HasPrefix(strings.ToLower(source), "file://") {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Path == "" {
			return nil
		}
		return []models.AssetRequirement{{Name: path.Base(parsed.Path)}}
	}
	if strings.Contains(source, "://") {
		return nil
	}
	return nil
}

func inferenceInputIsZero(input models.InferenceInput) bool {
	return input.Name == "" && input.Modality == "" && input.ContentType == "" &&
		input.MediaType == "" && input.Content == "" && input.Artifact == nil
}

func joinedInvocationLeaseReleased(result models.InvokeModelResult) bool {
	return result.LeaseDisposition == models.InvocationLeaseReleased ||
		result.LeaseDisposition == models.InvocationLeaseExpired
}

func (o *Root) releaseJoinedLease(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	lease models.ModelLeaseRef,
) error {
	if o == nil || o.runtimeHost == nil || lease.IsZero() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	releaseContext := context.WithoutCancel(ctx)
	_, err := o.ReleaseModelLease(releaseContext, models.ReleaseModelLeaseRequest{
		Scope: scope, Lease: lease,
	})
	return err
}

func joinedInvocationContextError(ctx context.Context, err error) error {
	if err == nil || ctx == nil || ctx.Err() == nil || errors.Is(err, models.ErrInferenceCancelled) {
		return err
	}
	return errors.Join(models.ErrInferenceCancelled, err)
}

func joinedInvocationStart(o *Root) time.Time {
	if o != nil && o.process.Clock != nil {
		return o.process.Clock()
	}
	return time.Time{}
}

func joinedInvocationElapsed(o *Root, started time.Time) time.Duration {
	if o == nil || o.process.Clock == nil || started.IsZero() {
		return 0
	}
	ended := o.process.Clock()
	if ended.Before(started) {
		return 0
	}
	return ended.Sub(started)
}

func joinedInvocationRecord(
	o *Root,
	modelName string,
	operation string,
	invocation models.ModelInvocationRef,
	stage string,
	err error,
	elapsed time.Duration,
) {
	if o == nil || o.process.Logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("model_name", modelName),
		zap.String("operation", operation),
		zap.String("invocation", invocation.String()),
		zap.String("stage", stage),
		zap.Duration("duration", elapsed),
	}
	if err != nil {
		fields = append(fields,
			zap.String("outcome", "FAILED"),
			zap.String("failure_class", joinedInvocationFailureClass(err)),
		)
		o.process.Logger.Warn("models invocation completed", fields...)
		return
	}
	fields = append(fields, zap.String("outcome", "COMPLETED"))
	o.process.Logger.Info("models invocation completed", fields...)
}

func joinedInvocationFailureClass(err error) string {
	var invocationFailure *models.InvocationFailure
	if errors.As(err, &invocationFailure) && invocationFailure != nil && invocationFailure.Class != "" {
		return string(invocationFailure.Class)
	}
	var configurationFailure models.ModelConfigurationFailure
	if errors.As(err, &configurationFailure) {
		return string(models.InvocationFailureClassConfiguration)
	}
	switch {
	case errors.Is(err, models.ErrInferenceCancelled), errors.Is(err, models.ErrAssetCancelled), errors.Is(err, models.ErrHostCancelled):
		return string(models.InvocationFailureClassCancellation)
	case errors.Is(err, models.ErrInferenceTimeout), errors.Is(err, models.ErrHostLoadingTimeout):
		return string(models.InvocationFailureClassTimeout)
	case errors.Is(err, models.ErrHostProtocolIncompatible):
		return string(models.InvocationFailureClassBackendProtocol)
	case errors.Is(err, models.ErrHostRuntimeNotReady), errors.Is(err, models.ErrHostProcessCrash):
		return string(models.InvocationFailureClassBackendReadiness)
	case errors.Is(err, models.ErrAssetOffline):
		return string(models.InvocationFailureClassOfflineCache)
	case errors.Is(err, models.ErrInferenceArtifactInvalid):
		return string(models.InvocationFailureClassArtifact)
	case errors.Is(err, models.ErrModelRevisionUnresolved):
		return string(models.InvocationFailureClassRevisionResolution)
	case errors.Is(err, models.ErrModelReferenceInvalid):
		return string(models.InvocationFailureClassInvalidModelReference)
	case errors.Is(err, models.ErrInferenceFailed):
		return joinedInvocationFailureRuntime
	default:
		return joinedInvocationFailureRuntime
	}
}

func (o *Root) CancelInvocation(
	ctx context.Context,
	request models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	if o == nil || o.inference == nil {
		return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
	}
	return o.inference.CancelInvocation(ctx, request)
}

func (o *Root) ListModels(context.Context) (models.List, error) {
	return models.List{}, missingDependencyError("Models runtime binding")
}

func (o *Root) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, missingDependencyError("Models runtime binding")
}

func (o *Root) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, missingDependencyError("Models runtime binding")
}

func (o *Root) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, missingDependencyError("Models runtime binding")
}

func (o *Root) AcquireLease(context.Context, models.AcquireLeaseRequest) (models.HostLease, error) {
	return models.HostLease{}, missingDependencyError("Models runtime binding")
}

func (o *Root) ReleaseLease(context.Context, models.ReleaseLeaseRequest) error {
	return missingDependencyError("Models runtime binding")
}

func (o *Root) InvokeLocal(
	ctx context.Context,
	request models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	runtime, err := o.scopedRuntime(request.Scope)
	if err != nil {
		return models.LocalInvocationResult{}, err
	}
	return runtime.InvokeLocal(ctx, request)
}
