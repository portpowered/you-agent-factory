// Package wire is the Models service composition boundary. Application Wire
// uses these constructors without importing Models implementation packages.
package wire

import (
	"context"
	"fmt"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelassets "github.com/portpowered/infinite-you/pkg/services/models/internal/assets"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models/internal/service"
	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"go.uber.org/zap"
)

// NewService constructs the inert, process-scoped Models service.
func NewService(
	assetPlatform models.AssetHostPlatform,
	assetHTTP models.AssetHTTPDoer,
	assetEndpoints models.RuntimeAssetEndpoints,
	assetMkdirAll models.AssetMakeDirectories,
	assetStat models.AssetInspectPath,
	assetHome models.AssetResolveHomeDirectory,
	assetWriteFile models.AssetWriteFile,
	assetRename models.AssetRenamePath,
	assetRemove models.AssetRemovePath,
	assetReadFile models.AssetReadFile,
	assetReadDir models.AssetReadDirectory,
	assetCreate models.AssetCreateFile,
	assetOpen models.AssetOpenFile,
	processLauncher models.HostProcessLauncher,
	hostHTTP models.HostHTTPDoer,
	hostClock models.HostClock,
	runtimeRunner platformprocess.CommandRunner,
	runtimeHTTP models.RuntimeHTTPDoer,
	runtimeInspect models.RuntimeInspectFile,
	runtimeTempDir models.RuntimeTempDirectory,
	runtimeTempFile models.RuntimeCreateTempFile,
	logger *zap.Logger,
	now func() time.Time,
	pullMetrics models.PullMetricsRecorder,
	hostLogger models.HostDiagnosticLogger,
	hostMetrics models.HostMetricsRecorder,
	localHooks models.LocalRuntimeHooks,
) (models.Service, error) {
	defaultEndpoints := modelassets.DefaultEndpoints()
	if assetEndpoints.BaseURL != "" {
		defaultEndpoints.BaseURL = assetEndpoints.BaseURL
	}
	if assetEndpoints.APIBaseURL != "" {
		defaultEndpoints.APIBaseURL = assetEndpoints.APIBaseURL
	}
	var launcher modelhost.ProcessLauncher
	if processLauncher != nil {
		launcher = hostProcessLauncher{next: processLauncher}
	}
	var clock modelhost.Clock
	if hostClock != nil {
		clock = hostClockAdapter{next: hostClock}
	}
	var createTempFile localmodels.CreateTempFile
	if runtimeTempFile != nil {
		createTempFile = runtimeTempFileAdapter{next: runtimeTempFile}.create
	}
	runtimeScopes, err := runtimescopeswire.NewService(func() string {
		return runtimeScopeIssuerID(now, assetHTTP, hostHTTP)
	})
	if err != nil {
		return nil, err
	}
	catalogService, err := catalogwire.NewService(runtimeScopes, newCatalogReadinessQuery(catalogReadinessEdges{
		platform: localmodels.HostPlatform(assetPlatform), client: assetHTTP, endpoints: defaultEndpoints,
		mkdirAll: modelassets.MakeDirectories(assetMkdirAll), stat: modelassets.InspectPath(assetStat),
		home: modelassets.ResolveHomeDirectory(assetHome), writeFile: modelassets.WriteFile(assetWriteFile),
		rename: modelassets.RenamePath(assetRename), remove: modelassets.RemovePath(assetRemove),
		readFile: modelassets.ReadFile(assetReadFile), readDir: modelassets.ReadDirectory(assetReadDir),
		create: modelassets.CreateFile(assetCreate), open: modelassets.OpenFile(assetOpen),
	}))
	if err != nil {
		return nil, err
	}
	service, err := modelsservice.NewRoot(
		localmodels.HostPlatform(assetPlatform), assetHTTP, defaultEndpoints,
		modelassets.MakeDirectories(assetMkdirAll), modelassets.InspectPath(assetStat),
		modelassets.ResolveHomeDirectory(assetHome), modelassets.WriteFile(assetWriteFile),
		modelassets.RenamePath(assetRename), modelassets.RemovePath(assetRemove),
		modelassets.ReadFile(assetReadFile), modelassets.ReadDirectory(assetReadDir),
		modelassets.CreateFile(assetCreate), modelassets.OpenFile(assetOpen),
		launcher, hostHTTP, clock,
		runtimeRunner, runtimeHTTP, localmodels.InspectFile(runtimeInspect),
		localmodels.TempDirectory(runtimeTempDir), createTempFile,
		runtimeScopes, catalogService,
		models.ProcessDependencies{
			Logger: logger, Clock: now, PullMetrics: pullMetrics,
			HostLogger: hostLogger, HostMetrics: hostMetrics, LocalHooks: localHooks,
		},
	)
	if err != nil {
		return nil, err
	}
	return service, nil
}

type catalogReadinessEdges struct {
	platform  localmodels.HostPlatform
	client    modelassets.HTTPDoer
	endpoints modelassets.Endpoints
	mkdirAll  modelassets.MakeDirectories
	stat      modelassets.InspectPath
	home      modelassets.ResolveHomeDirectory
	writeFile modelassets.WriteFile
	rename    modelassets.RenamePath
	remove    modelassets.RemovePath
	readFile  modelassets.ReadFile
	readDir   modelassets.ReadDirectory
	create    modelassets.CreateFile
	open      modelassets.OpenFile
}

func newCatalogReadinessQuery(edges catalogReadinessEdges) catalog.ReadinessQuery {
	return func(
		ctx context.Context,
		scope models.RuntimeScopeConfig,
		detail models.Detail,
	) (models.Runtime, error) {
		puller, err := localmodels.NewAssetPuller(
			scope.CacheDirectory, edges.platform, edges.client, edges.endpoints,
			edges.mkdirAll, edges.stat, edges.home, edges.writeFile, edges.rename,
			edges.remove, edges.readFile, edges.readDir, edges.create, edges.open,
		)
		if err != nil {
			return models.Runtime{}, err
		}
		return localmodels.ManagedRuntimeReadinessForFactoryContext(
			ctx,
			&scope.Runtime,
			detail.Name,
			puller,
			localmodels.DefaultManagedRuntimeSourceResolver(),
		)
	}
}

func runtimeScopeIssuerID(now func() time.Time, assetHTTP models.AssetHTTPDoer, hostHTTP models.HostHTTPDoer) string {
	if now == nil {
		return ""
	}
	return fmt.Sprintf("%d:%p:%p", now().UTC().UnixNano(), assetHTTP, hostHTTP)
}

// NewInvocationArtifactExporter constructs the Models-owned invocation artifact exporter.
func NewInvocationArtifactExporter(fileSystem models.InvocationArtifactFileSystem) (models.InvocationArtifactExporter, error) {
	return artifacts.NewExporter(fileSystem)
}

type hostProcessLauncher struct{ next models.HostProcessLauncher }

func (a hostProcessLauncher) Start(ctx context.Context, spec modelhost.ProcessStartSpec) (modelhost.ManagedProcess, error) {
	process, err := a.next.Start(ctx, models.HostProcessStartSpec{
		Command: spec.Command, Args: spec.Args, Env: spec.Env, WorkDir: spec.WorkDir, HealthEndpoint: spec.HealthEndpoint,
	})
	if err != nil {
		return nil, err
	}
	return process, nil
}

type hostClockAdapter struct{ next models.HostClock }

func (a hostClockAdapter) Now() time.Time { return a.next.Now() }
func (a hostClockAdapter) NewTimer(duration time.Duration) modelhost.Timer {
	return a.next.NewTimer(duration)
}

type runtimeTempFileAdapter struct{ next models.RuntimeCreateTempFile }

func (a runtimeTempFileAdapter) create(dir, pattern string) (localmodels.TempFile, error) {
	return a.next(dir, pattern)
}
