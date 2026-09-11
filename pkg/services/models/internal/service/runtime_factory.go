package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
	correlationSequence        uint64
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
	process.RuntimeEvidence = modelseffects.NewOrderedRuntimeEvidenceRecorder(
		process.RuntimeEvidence,
	)
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

type resolvedHostConfigurationHandoff interface {
	EnsureModelHostWithConfiguration(
		context.Context,
		modelseffects.ResolvedHostConfiguration,
	) (models.EnsureModelHostResult, error)
}

func (o *Root) ensureModelHostWithConfiguration(
	ctx context.Context,
	configuration modelseffects.ResolvedHostConfiguration,
) (models.EnsureModelHostResult, error) {
	if o == nil || o.runtimeHost == nil {
		return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
	}
	o.cacheLifecycleMu.Lock()
	defer o.cacheLifecycleMu.Unlock()
	if handoff, ok := o.runtimeHost.(resolvedHostConfigurationHandoff); ok {
		return handoff.EnsureModelHostWithConfiguration(ctx, configuration.Clone())
	}
	return o.runtimeHost.EnsureModelHost(ctx, models.EnsureModelHostRequest{
		Scope: configuration.Scope,
		Name:  configuration.ModelName,
	})
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
	modelName     string
	backend       string
	revision      string
	correlation   string
	configuration modelseffects.ResolvedHostConfiguration
	operation     models.Operation
	prepared      models.InvokeModelRequest
	lease         models.ModelLeaseRef
}

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
	correlation := nextJoinedInvocationCorrelation(o)
	ctx = modelseffects.WithRuntimeCorrelation(ctx, correlation)
	ctx = modelseffects.WithRuntimeLeaseReleaseTracker(ctx)
	started := joinedInvocationStart(o)
	var invocationEvidence modelseffects.RuntimeEvidenceRecorder
	if o != nil {
		invocationEvidence = modelseffects.NewRuntimeEvidenceInvocation(
			o.process.RuntimeEvidence,
		)
	}
	stage := modelseffects.RuntimeStageArtifactResolve
	modelName := ""
	operationName := request.Operation
	var invocation models.ModelInvocationRef
	backend := ""
	revision := ""
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
		publicErr := err
		diagnosticErr := err
		if publicErr != nil {
			result = joinedInvocationFailureResult(result)
			diagnosticErr = modelseffects.WrapRuntimeFailure(stage, publicErr)
		}
		elapsed := joinedInvocationElapsed(o, started)
		if o != nil && (diagnosticErr == nil || !runtimeHostEvidenceAlreadyRecorded(diagnosticErr)) {
			modelseffects.RecordRuntimeEvidenceStage(
				invocationEvidence, stage, diagnosticErr, elapsed,
			)
		}
		if o != nil {
			modelseffects.RecordRuntimeEvidenceTerminal(
				invocationEvidence, stage, diagnosticErr, elapsed,
			)
		}
		joinedInvocationRecord(
			o, modelName, operationName, invocation, backend, revision,
			correlation, stage, publicErr, elapsed,
		)
		return result.Clone(), publicErr
	}

	if err := validateJoinedRoot(o); err != nil {
		return finish(models.InvokeModelResult{}, err)
	}
	plan, preparedStage, err := o.prepareJoinedInvocation(
		ctx, request, correlation, started,
	)
	stage = preparedStage
	modelName = plan.modelName
	operationName = plan.operation.Name
	backend = plan.backend
	revision = plan.revision
	if err != nil {
		return finish(models.InvokeModelResult{}, joinedInvocationContextError(ctx, err))
	}

	result, invokeStage, err := o.executeJoinedInvocation(ctx, plan, started)
	stage = invokeStage
	return finish(result, joinedInvocationContextError(ctx, err))
}

func (o *Root) prepareJoinedInvocation(
	ctx context.Context,
	request models.InvokeModelRequest,
	correlation string,
	started time.Time,
) (joinedInvocationPlan, modelseffects.RuntimeStage, error) {
	plan := joinedInvocationPlan{}
	if err := request.ValidateGeneric(); err != nil {
		return plan, modelseffects.RuntimeStageArtifactResolve, err
	}

	resolution, err := o.ResolveModelReference(ctx, models.ResolveModelReferenceRequest{
		Scope: request.Scope, Reference: request.Model,
	})
	if err != nil {
		return plan, modelseffects.RuntimeStageArtifactResolve, err
	}
	resolved := resolution.Resolved
	plan.configuration = joinedHostConfiguration(
		request, resolved, o.process.BackendArtifactPlatform,
	)
	plan.modelName = plan.configuration.ModelName
	plan.backend = plan.configuration.Backend
	plan.revision = plan.configuration.Revision
	plan.correlation = correlation
	plan.prepared, plan.operation, err = models.PrepareGenericInvocation(request, resolved.Definition)
	if err != nil {
		return plan, modelseffects.RuntimeStageArtifactResolve, err
	}
	plan.prepared.ModelName = plan.modelName
	plan.prepared.Operation = plan.operation.Name
	if len(plan.prepared.Inputs) > 0 && inferenceInputIsZero(plan.prepared.Input) {
		plan.prepared.Input = plan.prepared.Inputs[0].Clone()
	}

	backendArtifact, err := o.resolveJoinedBackendArtifact(ctx, plan.configuration)
	if err != nil {
		return plan, modelseffects.RuntimeStageArtifactResolve, err
	}
	plan.configuration.BackendArtifact = backendArtifact
	assetRequest, err := joinedAssetPreparationRequestWithConfiguration(
		request, plan.configuration, resolved,
	)
	if err != nil {
		return plan, modelseffects.RuntimeStageArtifactResolve, err
	}
	if _, err := o.PreflightModelAssets(ctx, assetRequest); err != nil {
		return plan, joinedAssetRuntimeStage(err), joinedInvocationAssetError(request, err)
	}
	if _, err := o.PrepareModelAssets(ctx, assetRequest); err != nil {
		return plan, joinedAssetRuntimeStage(err), joinedInvocationAssetError(request, err)
	}
	if err := o.enrichJoinedHostConfiguration(ctx, &plan.configuration); err != nil {
		return plan, joinedAssetRuntimeStage(err), joinedInvocationAssetError(request, err)
	}
	plan.modelName = plan.configuration.ModelName
	plan.backend = plan.configuration.Backend
	plan.revision = plan.configuration.Revision
	joinedInvocationLifecycleRecord(
		o, plan.modelName, plan.backend, plan.revision, correlation,
		plan.operation.Name, models.ModelInvocationRef{},
		joinedLifecycleStageArtifactProvision, joinedLifecycleOutcomeCompleted,
		joinedInvocationElapsed(o, started), nil,
	)
	ensureResult, err := o.ensureModelHostWithConfiguration(ctx, plan.configuration)
	if err != nil {
		return plan, modelseffects.RuntimeStageBackendStart, err
	}
	if hostRevision := strings.TrimSpace(ensureResult.Host.Diagnostics["revision"]); hostRevision != "" {
		plan.revision = hostRevision
	}
	joinedInvocationLifecycleRecord(
		o, plan.modelName, plan.backend, plan.revision, correlation,
		plan.operation.Name, models.ModelInvocationRef{},
		joinedLifecycleStageBackendStart, joinedLifecycleOutcomeCompleted,
		joinedInvocationElapsed(o, started), nil,
	)
	joinedInvocationLifecycleRecord(
		o, plan.modelName, plan.backend, plan.revision, correlation,
		plan.operation.Name, models.ModelInvocationRef{},
		joinedLifecycleStageHealth, joinedLifecycleOutcomeCompleted,
		joinedInvocationElapsed(o, started), nil,
	)
	leaseResult, err := o.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{
		Scope: request.Scope, Name: plan.modelName, Holder: request.Holder,
	})
	if err != nil {
		return plan, modelseffects.RuntimeStageBackendStart, err
	}
	plan.lease = leaseResult.Lease.Lease
	if plan.lease.IsZero() {
		return plan, modelseffects.RuntimeStageBackendStart, models.ErrHostLeaseNotFound
	}
	plan.prepared.Lease = plan.lease
	return plan, modelseffects.RuntimeStageInvoke, nil
}

func joinedAssetRuntimeStage(err error) modelseffects.RuntimeStage {
	switch {
	case errors.Is(err, models.ErrAssetIntegrityFailed):
		return modelseffects.RuntimeStageArtifactDigest
	case errors.Is(err, models.ErrInferenceArtifactInvalid):
		return modelseffects.RuntimeStageArtifactDigest
	case errors.Is(err, models.ErrAssetSourceMissing),
		errors.Is(err, models.ErrAssetSourceUnsupported),
		errors.Is(err, models.ErrAssetOffline),
		errors.Is(err, models.ErrAssetUnavailable),
		errors.Is(err, models.ErrAssetBackendNotReady):
		return modelseffects.RuntimeStageArtifactDownload
	default:
		return modelseffects.RuntimeStageArtifactResolve
	}
}

func (o *Root) resolveJoinedBackendArtifact(
	ctx context.Context,
	configuration modelseffects.ResolvedHostConfiguration,
) (modelseffects.BackendArtifactSelection, error) {
	if !isJoinedPinnedBackend(configuration.Backend) {
		return modelseffects.BackendArtifactSelection{}, nil
	}
	if o == nil || o.resolveBackendArtifact == nil {
		return modelseffects.BackendArtifactSelection{}, fmt.Errorf(
			"%w: pinned backend artifact selector is unavailable",
			models.ErrHostMissingAssets,
		)
	}
	selection, err := o.resolveBackendArtifact(ctx, configuration.Clone())
	if err != nil {
		return modelseffects.BackendArtifactSelection{}, fmt.Errorf(
			"%w: pinned backend artifact selection failed",
			models.ErrHostMissingAssets,
		)
	}
	requirement := models.AssetRequirement{
		Name: selection.Name, Bytes: selection.Bytes, SHA256: selection.SHA256,
	}
	if strings.TrimSpace(selection.Location) == "" || strings.TrimSpace(selection.SHA256) == "" ||
		selection.Bytes <= 0 || requirement.Validate() != nil {
		return modelseffects.BackendArtifactSelection{}, fmt.Errorf(
			"%w: pinned backend artifact facts are invalid",
			models.ErrHostMissingAssets,
		)
	}
	return selection, nil
}

func joinedHostConfiguration(
	request models.InvokeModelRequest,
	resolved models.ResolvedModelReference,
	platform models.AssetHostPlatform,
) modelseffects.ResolvedHostConfiguration {
	return modelseffects.ResolvedHostConfiguration{
		Scope:           request.Scope,
		ModelName:       strings.TrimSpace(resolved.Definition.Name),
		Source:          joinedAssetReference(request.Model, resolved),
		Revision:        joinedInvocationRevision(resolved.Definition.Source),
		Backend:         strings.TrimSpace(resolved.Definition.Backend),
		Platform:        platform,
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	}
}

func (o *Root) enrichJoinedHostConfiguration(
	ctx context.Context,
	configuration *modelseffects.ResolvedHostConfiguration,
) error {
	if configuration == nil || o == nil || o.assets == nil {
		return models.ErrUnsupportedOperation
	}
	layout, err := o.assets.ResolveRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: configuration.Scope,
		Name:  configuration.ModelName,
	})
	if errors.Is(err, models.ErrUnsupportedOperation) {
		// Some parent-private component fakes intentionally stop at asset
		// preparation. Production Assets supplies the cache bridge; the fake
		// capability is not a host/cache result and is safe to leave empty.
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(layout.CachePath) == "" || len(layout.Files) == 0 {
		return fmt.Errorf(
			"%w: resolved model cache facts are incomplete",
			models.ErrHostMissingAssets,
		)
	}

	configuration.ModelCachePath = strings.TrimSpace(layout.CachePath)
	configuration.BackendCachePath = strings.TrimSpace(layout.BackendCachePath)
	configuration.ModelFiles = append([]string(nil), layout.Files...)
	configuration.BackendFiles = append([]string(nil), layout.BackendFiles...)
	configuration.ModelPath, configuration.MMProjPath = joinedHostModelPaths(layout.Files)
	if configuration.ModelPath == "" {
		return fmt.Errorf(
			"%w: resolved model file facts are incomplete",
			models.ErrHostMissingAssets,
		)
	}
	if isJoinedPinnedBackend(configuration.Backend) && configuration.BackendArtifact.Name != "" &&
		(strings.TrimSpace(configuration.BackendCachePath) == "" || len(configuration.BackendFiles) == 0) {
		return fmt.Errorf(
			"%w: resolved backend cache facts are incomplete",
			models.ErrHostMissingAssets,
		)
	}
	if revision := strings.TrimSpace(layout.Revision); revision != "" {
		configuration.Revision = revision
	}
	return nil
}

func joinedHostModelPaths(files []string) (string, string) {
	var modelPath, mmProjPath string
	for _, raw := range files {
		file := strings.TrimSpace(raw)
		if file == "" {
			continue
		}
		base := strings.ToLower(filepath.Base(filepath.Clean(file)))
		if strings.Contains(base, "mmproj") {
			if mmProjPath == "" {
				mmProjPath = file
			}
			continue
		}
		if modelPath == "" && !strings.Contains(base, "tokenizer") && !strings.HasPrefix(base, "voice") {
			modelPath = file
		}
	}
	if modelPath == "" {
		for _, raw := range files {
			file := strings.TrimSpace(raw)
			base := strings.ToLower(filepath.Base(filepath.Clean(file)))
			if file == "" || strings.Contains(base, "mmproj") {
				continue
			}
			modelPath = file
			break
		}
	}
	return modelPath, mmProjPath
}

func (o *Root) executeJoinedInvocation(
	ctx context.Context,
	plan joinedInvocationPlan,
	started time.Time,
) (models.InvokeModelResult, modelseffects.RuntimeStage, error) {
	result, err := o.InvokeModelWithLease(ctx, plan.prepared)
	result = joinedInvocationResultIdentity(result, plan)
	if err != nil {
		return o.finishJoinedFailure(ctx, plan, result, modelseffects.RuntimeStageInvoke, err)
	}
	if result.Status == models.ModelInvocationStatusCompleted {
		joinedInvocationLifecycleRecord(
			o, plan.modelName, plan.backend, plan.revision, plan.correlation,
			plan.operation.Name, result.Invocation, joinedLifecycleStageInvoke,
			joinedLifecycleOutcomeCompleted, joinedInvocationElapsed(o, started), nil,
		)
		result.Outputs, err = models.NormalizeGenericInvocationOutputs(
			plan.operation, result.Content, result.Artifacts,
		)
		if err != nil {
			result.Status = models.ModelInvocationStatusFailed
			return o.finishJoinedFailure(ctx, plan, result, modelseffects.RuntimeStageInvoke, err)
		}
		joinedInvocationLifecycleRecord(
			o, plan.modelName, plan.backend, plan.revision, plan.correlation,
			plan.operation.Name, result.Invocation, joinedLifecycleStageOutput,
			joinedLifecycleOutcomeCompleted, joinedInvocationElapsed(o, started), nil,
		)
	}
	if result.Status == models.ModelInvocationStatusAccepted {
		result.Status = models.ModelInvocationStatusFailed
		return o.finishJoinedFailure(
			ctx, plan, result, modelseffects.RuntimeStageInvoke,
			fmt.Errorf("%w: joined invocation did not complete", models.ErrInferenceFailed),
		)
	}
	if result.Status == models.ModelInvocationStatusFailed {
		return o.finishJoinedFailure(ctx, plan, result, modelseffects.RuntimeStageInvoke, models.ErrInferenceFailed)
	}
	if result.Status == models.ModelInvocationStatusCancelled {
		return o.finishJoinedFailure(ctx, plan, result, modelseffects.RuntimeStageInvoke, models.ErrInferenceCancelled)
	}
	if result.Status != models.ModelInvocationStatusCompleted {
		result.Status = models.ModelInvocationStatusFailed
		return o.finishJoinedFailure(ctx, plan, result, modelseffects.RuntimeStageInvoke, models.ErrInferenceFailed)
	}
	released, stage, releaseErr := o.releaseJoinedInvocation(ctx, plan, result)
	if releaseErr != nil {
		joinedInvocationLifecycleRecord(
			o, plan.modelName, plan.backend, plan.revision, plan.correlation,
			plan.operation.Name, released.Invocation, joinedLifecycleStageRelease,
			joinedLifecycleOutcomeFailed, joinedInvocationElapsed(o, started), releaseErr,
		)
		return released, stage, releaseErr
	}
	joinedInvocationLifecycleRecord(
		o, plan.modelName, plan.backend, plan.revision, plan.correlation,
		plan.operation.Name, released.Invocation, joinedLifecycleStageRelease,
		joinedLifecycleOutcomeCompleted, joinedInvocationElapsed(o, started), nil,
	)
	return released, stage, nil
}

func (o *Root) finishJoinedFailure(
	ctx context.Context,
	plan joinedInvocationPlan,
	result models.InvokeModelResult,
	stage modelseffects.RuntimeStage,
	invokeErr error,
) (models.InvokeModelResult, modelseffects.RuntimeStage, error) {
	if !joinedInvocationLeaseReleased(result) && !modelseffects.RuntimeLeaseReleaseAttempted(ctx) {
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
) (models.InvokeModelResult, modelseffects.RuntimeStage, error) {
	if joinedInvocationLeaseReleased(result) {
		return result, modelseffects.RuntimeStageInvoke, nil
	}
	releaseErr := o.releaseJoinedLease(ctx, plan.prepared.Scope, plan.lease)
	if releaseErr == nil {
		result.LeaseDisposition = models.InvocationLeaseReleased
	} else {
		result.Status = models.ModelInvocationStatusFailed
	}
	return result, modelseffects.RuntimeStageInvoke, releaseErr
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

func joinedAssetPreparationRequest(
	request models.InvokeModelRequest,
	modelName string,
	resolved models.ResolvedModelReference,
) (models.PrepareModelAssetsRequest, error) {
	configuration := modelseffects.ResolvedHostConfiguration{
		Scope: request.Scope, ModelName: modelName,
		Source:  joinedAssetReference(request.Model, resolved),
		Backend: strings.TrimSpace(resolved.Definition.Backend),
	}
	return joinedAssetPreparationRequestWithConfiguration(request, configuration, resolved)
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
