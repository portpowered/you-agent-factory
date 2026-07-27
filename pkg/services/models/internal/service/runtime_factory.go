package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
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
	catalog         modelcatalog.Service
	process         models.ProcessDependencies
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
	processDependencies ...models.ProcessDependencies,
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
	process := models.ProcessDependencies{}
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
		process: process,
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
	assets, err := localmodels.NewDeferredScopedAssetPuller(o.assets, func() (models.RuntimeScopeRef, error) {
		if binding.RuntimeConfig() == nil {
			return models.RuntimeScopeRef{}, models.ErrUnavailable
		}
		privateScope, openErr := o.runtimeScopes.Open(binding)
		if openErr != nil {
			return models.RuntimeScopeRef{}, openErr
		}
		return (models.RuntimeScopeRef{}).Parse(string(privateScope))
	})
	if err != nil {
		return nil, err
	}
	localRuntime, err := localmodels.NewOmniVoiceRuntime(
		o.runtimeRunner, o.runtimeHTTP, o.runtimeInspect, o.runtimeTempDir, o.runtimeTempFile,
	)
	if err != nil {
		return nil, err
	}
	healthChecker := modelhost.HTTPHealthChecker{Client: o.hostHTTP, Path: modelhost.DefaultHealthCheckPath}
	return newRuntimeWithHostEdges(
		binding.CacheDirectory, binding.RuntimeConfig, o.process.Logger, o.process.Clock,
		o.process.PullMetrics, o.process.HostLogger, o.process.HostMetrics, o.process.LocalHooks,
		assets, localRuntime, o.processLauncher, healthChecker, o.hostClock, nil,
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
	ref, err := o.runtimeScopes.Open(models.RuntimeBinding{
		CacheDirectory: config.CacheDirectory,
		RuntimeConfig: func() *models.RuntimeConfig {
			runtimeConfig := config.Runtime
			return &runtimeConfig
		},
	})
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
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (o *Root) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (o *Root) InspectModelHost(
	context.Context,
	models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (o *Root) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func (o *Root) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (o *Root) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (o *Root) ReleaseModelLease(
	context.Context,
	models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (o *Root) InvokeModelWithLease(
	context.Context,
	models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (o *Root) CancelInvocation(
	context.Context,
	models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
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

func (o *Root) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{}, missingDependencyError("Models runtime binding")
}

func newRuntimeWithHostEdges(
	cacheDir string,
	runtimeConfig models.RuntimeConfigLoader,
	logger *zap.Logger,
	now func() time.Time,
	pullMetrics models.PullMetricsRecorder,
	hostLogger models.HostDiagnosticLogger,
	hostMetrics models.HostMetricsRecorder,
	hooks models.LocalRuntimeHooks,
	assetPuller localmodels.AssetPuller,
	localRuntime localmodels.Runtime,
	processLauncher modelhost.ProcessLauncher,
	healthChecker modelhost.HealthChecker,
	hostClock modelhost.Clock,
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
		modelHost, err = modelhost.NewHost(
			gateway,
			gateway,
			processLauncher,
			modelhost.DefaultManagedRuntimeSourceResolverAdapter(),
			modelhost.DefaultReadinessTimeout,
			modelhost.DefaultHealthCheckInterval,
			modelhost.DefaultHealthCheckPath,
			healthChecker,
			hostClock,
			modelhost.DefaultServerStartBuilder,
			modelhost.Diagnostics{Logger: hostLogger, Metrics: hostMetrics},
			0,
			0,
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
