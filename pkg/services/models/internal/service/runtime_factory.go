package service

import (
	"context"
	"errors"
	"fmt"
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
	processLauncher modelhost.ProcessLauncher
	hostHTTP        modelhost.HTTPDoer
	hostClock       modelhost.Clock
	runtimeRunner   platformprocess.CommandRunner
	runtimeHTTP     localmodels.HTTPDoer
	runtimeInspect  localmodels.InspectFile
	runtimeTempDir  localmodels.TempDirectory
	runtimeTempFile localmodels.CreateTempFile
	runtimeScopes   runtimescopes.Service
	assets          scopedassets.Service
	runtimeHost     runtimehost.Service
	inference       modelinference.Service
	runtimeMu       sync.RWMutex
	runtimeByScope  map[models.RuntimeScopeRef]models.Service
	catalog         modelcatalog.Service
	process         modelseffects.ProcessDependencies
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
	return &Root{
		processLauncher: processLauncher, hostHTTP: hostHTTP, hostClock: hostClock,
		runtimeRunner: runtimeRunner, runtimeHTTP: runtimeHTTP,
		runtimeInspect: runtimeInspect, runtimeTempDir: runtimeTempDir, runtimeTempFile: runtimeTempFile,
		runtimeScopes: runtimeScopes, catalog: catalogService, assets: assetService,
		runtimeHost: runtimeHostService, inference: inferenceService,
		runtimeByScope: make(map[models.RuntimeScopeRef]models.Service),
		process:        process,
	}, nil
}

// ForRuntime binds the injected Models service to one Factory Session.
func (o *Root) ForRuntime(binding models.RuntimeBinding) (models.Service, error) {
	if o == nil {
		return nil, missingDependencyError("Models service")
	}
	if err := models.ValidateRuntimeBinding(binding); err != nil {
		return nil, err
	}
	privateScope, err := o.runtimeScopes.Open(binding)
	if err != nil {
		return nil, err
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(string(privateScope))
	if err != nil {
		return nil, err
	}
	assets, err := localmodels.NewScopedAssetPuller(o.assets, scope)
	if err != nil {
		return nil, err
	}
	return o.runtimeForBindingWithAssets(scope, binding, assets)
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
	return o.assets.PrepareModelAssets(ctx, request)
}

func (o *Root) PullModelForScope(
	ctx context.Context,
	request models.PullModelRequest,
) (models.PullResult, error) {
	if err := models.ValidatePullModelRequest(request); err != nil {
		return models.PullResult{}, err
	}
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
	return puller.PullModel(ctx, request.Name)
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
	return o.assets.RemoveModelAssets(ctx, request)
}

func (o *Root) EnsureModelHost(
	ctx context.Context,
	request models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	if o == nil || o.runtimeHost == nil {
		return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
	}
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
	return o.runtimeHost.StopModelHost(ctx, request)
}

func (o *Root) AcquireModelLease(
	ctx context.Context,
	request models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	if o == nil || o.runtimeHost == nil {
		return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
	}
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
	return o.runtimeHost.ReleaseModelLease(ctx, request)
}

func (o *Root) InvokeModelWithLease(
	ctx context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	if o == nil || o.inference == nil {
		return models.InvokeModelResult{}, models.ErrUnsupportedOperation
	}
	return o.inference.InvokeModelWithLease(ctx, request)
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

func (o *Root) scopedRuntime(scope models.RuntimeScopeRef) (models.Service, error) {
	return o.scopedRuntimeWithBuilder(scope, func(binding models.RuntimeBinding) (models.Service, error) {
		assets, err := localmodels.NewScopedAssetPuller(o.assets, scope)
		if err != nil {
			return nil, err
		}
		return o.runtimeForBindingWithAssets(scope, binding, assets)
	})
}

func (o *Root) scopedRuntimeWithBuilder(
	scope models.RuntimeScopeRef,
	builder func(models.RuntimeBinding) (models.Service, error),
) (models.Service, error) {
	if o == nil || o.runtimeScopes == nil {
		return nil, models.ErrUnsupportedOperation
	}
	if scope.IsZero() {
		return nil, models.ErrRuntimeScopeInvalid
	}
	binding, err := o.runtimeScopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		return nil, runtimeScopeError(err)
	}
	o.runtimeMu.RLock()
	runtime := o.runtimeByScope[scope]
	o.runtimeMu.RUnlock()
	if runtime != nil {
		return runtime, nil
	}
	runtime, err = builder(binding)
	if err != nil {
		return nil, err
	}
	o.runtimeMu.Lock()
	if _, err := o.runtimeScopes.Resolve(runtimescopes.Reference(scope.String())); err != nil {
		o.runtimeMu.Unlock()
		return nil, runtimeScopeError(err)
	}
	if existing := o.runtimeByScope[scope]; existing != nil {
		runtime = existing
	} else {
		o.runtimeByScope[scope] = runtime
	}
	o.runtimeMu.Unlock()
	return runtime, nil
}

func newRuntimeWithHostEdges(
	scope models.RuntimeScopeRef,
	runtimeConfig models.RuntimeConfigLoader,
	logger *zap.Logger,
	now func() time.Time,
	pullMetrics modelseffects.PullMetricsRecorder,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
	hooks modelseffects.LocalRuntimeHooks,
	assetPuller localmodels.AssetPuller,
	localRuntime localmodels.Runtime,
	runtimeHost runtimehost.Service,
	host modelhost.Host,
) (models.Service, error) {
	if assetPuller == nil {
		return nil, missingDependencyError("model asset puller")
	}
	if localRuntime == nil {
		return nil, missingDependencyError("local model runtime")
	}
	manager, err := localmodels.NewManagedRuntime(assetPuller, localRuntime, hooks, now)
	if err != nil {
		return nil, err
	}
	resources, err := localmodels.NewResourceLimiter(hooks, now)
	if err != nil {
		return nil, err
	}
	modelHost := host
	if modelHost == nil {
		gateway := modelhost.NewLocalAssetGateway(assetPuller)
		modelHost, err = modelhost.NewScopedCompatHost(
			scope,
			runtimeHost,
			gateway,
			modelhost.DefaultManagedRuntimeSourceResolverAdapter(),
			modelhost.Diagnostics{Logger: hostLogger, Metrics: hostMetrics},
		)
		if err != nil {
			return nil, err
		}
	}
	modelService, err := NewService(
		runtimeConfig,
		modelHost,
		assetPuller,
		logger,
		now,
		pullMetrics,
	)
	if err != nil {
		return nil, err
	}
	localExecutor, err := newLocalExecutor(
		runtimeConfig,
		modelHost,
		assetPuller,
		localRuntime,
		manager,
		resources,
		hooks,
		now,
	)
	if err != nil {
		return nil, err
	}
	return &runtimeService{Service: modelService, local: localExecutor}, nil
}

type runtimeService struct {
	*Service
	local *localExecutor
}

var _ models.Service = (*runtimeService)(nil)

func (s *runtimeService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return s, nil
}

func (s *runtimeService) OpenRuntimeScope(
	context.Context,
	models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	return models.OpenRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) CloseRuntimeScope(
	context.Context,
	models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	return models.CloseRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) PullModelForScope(
	ctx context.Context,
	request models.PullModelRequest,
) (models.PullResult, error) {
	if err := models.ValidatePullModelRequest(request); err != nil {
		return models.PullResult{}, err
	}
	return s.PullModel(ctx, request.Name)
}

func (s *runtimeService) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) InvokeLocal(ctx context.Context, request models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	if err := models.ValidateLocalInvocationRequest(request); err != nil {
		return models.LocalInvocationResult{}, err
	}
	return s.local.InvokeLocal(ctx, request)
}
