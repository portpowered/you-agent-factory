package service

import (
	"context"
	"strings"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelassets "github.com/portpowered/infinite-you/pkg/services/models/internal/assets"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	"go.uber.org/zap"
)

// Root retains the process-wide external effect ports of the injected Models
// service. It is inert until it is bound to a Factory Session runtime.
type Root struct {
	assetPlatform   localmodels.HostPlatform
	assetHTTP       modelassets.HTTPDoer
	assetEndpoints  modelassets.Endpoints
	assetMkdirAll   modelassets.MakeDirectories
	assetStat       modelassets.InspectPath
	assetHome       modelassets.ResolveHomeDirectory
	assetWriteFile  modelassets.WriteFile
	assetRename     modelassets.RenamePath
	assetRemove     modelassets.RemovePath
	assetReadFile   modelassets.ReadFile
	assetReadDir    modelassets.ReadDirectory
	assetCreate     modelassets.CreateFile
	assetOpen       modelassets.OpenFile
	processLauncher modelhost.ProcessLauncher
	hostHTTP        modelhost.HTTPDoer
	hostClock       modelhost.Clock
	runtimeRunner   platformprocess.CommandRunner
	runtimeHTTP     localmodels.HTTPDoer
	runtimeInspect  localmodels.InspectFile
	runtimeTempDir  localmodels.TempDirectory
	runtimeTempFile localmodels.CreateTempFile
	process         models.ProcessDependencies
}

var _ models.Service = (*Root)(nil)

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func NewRoot(
	assetPlatform localmodels.HostPlatform,
	assetHTTP modelassets.HTTPDoer,
	assetEndpoints modelassets.Endpoints,
	assetMkdirAll modelassets.MakeDirectories,
	assetStat modelassets.InspectPath,
	assetHome modelassets.ResolveHomeDirectory,
	assetWriteFile modelassets.WriteFile,
	assetRename modelassets.RenamePath,
	assetRemove modelassets.RemovePath,
	assetReadFile modelassets.ReadFile,
	assetReadDir modelassets.ReadDirectory,
	assetCreate modelassets.CreateFile,
	assetOpen modelassets.OpenFile,
	processLauncher modelhost.ProcessLauncher,
	hostHTTP modelhost.HTTPDoer,
	hostClock modelhost.Clock,
	runtimeRunner platformprocess.CommandRunner,
	runtimeHTTP localmodels.HTTPDoer,
	runtimeInspect localmodels.InspectFile,
	runtimeTempDir localmodels.TempDirectory,
	runtimeTempFile localmodels.CreateTempFile,
	processDependencies ...models.ProcessDependencies,
) (*Root, error) {
	if strings.TrimSpace(assetPlatform.OperatingSystem) == "" || strings.TrimSpace(assetPlatform.Architecture) == "" {
		return nil, missingDependencyError("model asset host platform")
	}
	if assetHTTP == nil {
		return nil, missingDependencyError("model asset HTTP client")
	}
	if assetEndpoints.BaseURL == "" || assetEndpoints.APIBaseURL == "" {
		return nil, missingDependencyError("model asset endpoints")
	}
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
	if assetMkdirAll == nil || assetStat == nil || assetHome == nil || assetWriteFile == nil ||
		assetRename == nil || assetRemove == nil || assetReadFile == nil || assetReadDir == nil ||
		assetCreate == nil || assetOpen == nil {
		return nil, missingDependencyError("model asset cache operations")
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
		assetPlatform: assetPlatform, assetHTTP: assetHTTP, assetEndpoints: assetEndpoints,
		assetMkdirAll: assetMkdirAll, assetStat: assetStat, assetHome: assetHome,
		assetWriteFile: assetWriteFile, assetRename: assetRename, assetRemove: assetRemove,
		assetReadFile: assetReadFile, assetReadDir: assetReadDir, assetCreate: assetCreate, assetOpen: assetOpen,
		processLauncher: processLauncher, hostHTTP: hostHTTP, hostClock: hostClock,
		runtimeRunner: runtimeRunner, runtimeHTTP: runtimeHTTP,
		runtimeInspect: runtimeInspect, runtimeTempDir: runtimeTempDir, runtimeTempFile: runtimeTempFile,
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
	assets, err := localmodels.NewAssetPuller(
		binding.CacheDirectory, o.assetPlatform, o.assetHTTP, o.assetEndpoints,
		o.assetMkdirAll, o.assetStat, o.assetHome, o.assetWriteFile, o.assetRename,
		o.assetRemove, o.assetReadFile, o.assetReadDir, o.assetCreate, o.assetOpen,
	)
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
	context.Context,
	models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	return models.OpenRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (o *Root) CloseRuntimeScope(
	context.Context,
	models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	return models.CloseRuntimeScopeResult{}, models.ErrUnsupportedOperation
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

func (s *runtimeService) InvokeLocal(ctx context.Context, request models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	if err := models.ValidateLocalInvocationRequest(request); err != nil {
		return models.LocalInvocationResult{}, err
	}
	return s.local.InvokeLocal(ctx, request)
}
