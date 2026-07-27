// Package wire is the Models service composition boundary. Application Wire
// uses these constructors without importing Models implementation packages.
package wire

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models/internal/service"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	assetswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/wire"
	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	inferencewire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/wire"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	runtimehostwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/wire"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"go.uber.org/zap"
)

const (
	defaultAssetBaseURL    = "https://huggingface.co"
	defaultAssetAPIBaseURL = "https://huggingface.co/api"
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
	issuerEntropy platformrandom.Source,
	pullMetrics models.PullMetricsRecorder,
	hostLogger models.HostDiagnosticLogger,
	hostMetrics models.HostMetricsRecorder,
	localHooks models.LocalRuntimeHooks,
) (models.Service, error) {
	defaultEndpoints := models.RuntimeAssetEndpoints{
		BaseURL: defaultAssetBaseURL, APIBaseURL: defaultAssetAPIBaseURL,
	}
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
	issuerID, err := runtimeScopeIssuerID(issuerEntropy)
	if err != nil {
		return nil, fmt.Errorf("construct Models Runtime Scopes issuer identity: %w", err)
	}
	runtimeScopes, err := runtimescopeswire.NewService(func() string { return issuerID })
	if err != nil {
		return nil, err
	}
	assetService, err := assetswire.NewService(
		runtimeScopes, assetPlatform, assetHTTP,
		models.RuntimeAssetEndpoints{
			BaseURL: defaultEndpoints.BaseURL, APIBaseURL: defaultEndpoints.APIBaseURL,
		},
		assetMkdirAll, assetStat, assetHome, assetWriteFile, assetRename,
		assetRemove, assetReadFile, assetReadDir, assetCreate, assetOpen,
	)
	if err != nil {
		return nil, err
	}
	catalogService, err := catalogwire.NewService(
		runtimeScopes,
		newCatalogReadinessQuery(assetService),
	)
	if err != nil {
		return nil, err
	}
	runtimeHost, err := runtimehostwire.NewService(
		runtimeScopes,
		assetService,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
	)
	if err != nil {
		return nil, err
	}
	inferenceService, err := inferencewire.NewService(
		runtimeScopes,
		catalogService,
		runtimeHost,
		inference.InputEchoInvocationRuntime{},
		now,
	)
	if err != nil {
		return nil, err
	}
	service, err := modelsservice.NewRoot(
		launcher, hostHTTP, clock,
		runtimeRunner, runtimeHTTP, localmodels.InspectFile(runtimeInspect),
		localmodels.TempDirectory(runtimeTempDir), createTempFile,
		runtimeScopes, catalogService, assetService, runtimeHost, inferenceService,
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

func newCatalogReadinessQuery(assetService scopedassets.Service) catalog.ReadinessQuery {
	return func(
		ctx context.Context,
		scopeRef models.RuntimeScopeRef,
		scope models.RuntimeScopeConfig,
		detail models.Detail,
	) (models.Runtime, error) {
		puller, err := localmodels.NewScopedAssetPuller(assetService, scopeRef)
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

func runtimeScopeIssuerID(entropy platformrandom.Source) (string, error) {
	if entropy == nil {
		return "", fmt.Errorf("issuer entropy is required")
	}
	var identity [16]byte
	for index := range identity {
		value, err := entropy.Int63n(256)
		if err != nil {
			return "", err
		}
		identity[index] = byte(value)
	}
	return hex.EncodeToString(identity[:]), nil
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
