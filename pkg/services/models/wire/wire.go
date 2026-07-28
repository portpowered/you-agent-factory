// Package wire is the Models service composition boundary.
//
// Wire performs construction only, returns the singular models.Service root
// interface, and starts no lifecycle components. Parent-private runtime_scopes,
// catalog, assets, runtime_host, and inference owner wiring stays inside the
// owner service assembly path; peers depend on models.Service rather than owner
// internals or construction ports.
package wire

import (
	"context"
	"encoding/hex"
	"fmt"
	"reflect"
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
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	inferencewire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/wire"
	runtimehostwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/wire"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"go.uber.org/zap"
)

const (
	defaultAssetBaseURL    = "https://huggingface.co"
	defaultAssetAPIBaseURL = "https://huggingface.co/api"
)

// NewService constructs an inert Models root from construction and process-edge
// ports. It composes the accepted root through parent-private runtime_scopes,
// catalog, assets, runtime_host, and inference owner construction without
// publishing owner types on the returned peer surface. Missing required
// construction ports fail with a deterministic construction error and a nil
// service.
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
	if err := validateConstructionInputs(
		assetPlatform,
		assetHTTP,
		assetMkdirAll,
		assetStat,
		assetHome,
		assetWriteFile,
		assetRename,
		assetRemove,
		assetReadFile,
		assetReadDir,
		assetCreate,
		assetOpen,
		processLauncher,
		hostHTTP,
		hostClock,
		runtimeRunner,
		runtimeHTTP,
		runtimeInspect,
		runtimeTempDir,
		runtimeTempFile,
		now,
		issuerEntropy,
	); err != nil {
		return nil, err
	}
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
		assetService,
		catalogService,
		runtimeHost,
		inference.InputEchoInvocationRuntime{},
		inference.InertArtifactFileSystem{},
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

func validateConstructionInputs(
	assetPlatform models.AssetHostPlatform,
	assetHTTP models.AssetHTTPDoer,
	assetMkdirAll models.AssetMakeDirectories,
	assetStat models.AssetInspectPath,
	assetHome models.AssetResolveHomeDirectory,
	assetWriteFile models.AssetWriteFile,
	assetRename models.AssetRenamePath,
	assetRemove models.AssetRemovePath,
	assetReadFile models.AssetReadFile,
	assetReadDirectory models.AssetReadDirectory,
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
	now func() time.Time,
	issuerEntropy platformrandom.Source,
) error {
	switch {
	case issuerEntropy == nil:
		return fmt.Errorf("construct Models: issuer entropy is required")
	case assetPlatform.OperatingSystem == "" || assetPlatform.Architecture == "":
		return fmt.Errorf("construct Models: asset host platform is required")
	case isNilDependency(assetHTTP):
		return fmt.Errorf("construct Models: asset HTTP client is required")
	case isNilDependency(assetMkdirAll):
		return fmt.Errorf("construct Models: asset make-directories effect is required")
	case isNilDependency(assetStat):
		return fmt.Errorf("construct Models: asset inspect-path effect is required")
	case isNilDependency(assetHome):
		return fmt.Errorf("construct Models: asset resolve-home effect is required")
	case isNilDependency(assetWriteFile):
		return fmt.Errorf("construct Models: asset write-file effect is required")
	case isNilDependency(assetRename):
		return fmt.Errorf("construct Models: asset rename-path effect is required")
	case isNilDependency(assetRemove):
		return fmt.Errorf("construct Models: asset remove-path effect is required")
	case isNilDependency(assetReadFile):
		return fmt.Errorf("construct Models: asset read-file effect is required")
	case isNilDependency(assetReadDirectory):
		return fmt.Errorf("construct Models: asset read-directory effect is required")
	case isNilDependency(assetCreate):
		return fmt.Errorf("construct Models: asset create-file effect is required")
	case isNilDependency(assetOpen):
		return fmt.Errorf("construct Models: asset open-file effect is required")
	case isNilDependency(processLauncher):
		return fmt.Errorf("construct Models: model host process launcher is required")
	case isNilDependency(hostHTTP):
		return fmt.Errorf("construct Models: model host HTTP client is required")
	case isNilDependency(hostClock):
		return fmt.Errorf("construct Models: model host clock is required")
	case isNilDependency(runtimeRunner):
		return fmt.Errorf("construct Models: model runtime command runner is required")
	case isNilDependency(runtimeHTTP):
		return fmt.Errorf("construct Models: model runtime HTTP client is required")
	case isNilDependency(runtimeInspect):
		return fmt.Errorf("construct Models: model runtime file inspector is required")
	case isNilDependency(runtimeTempDir):
		return fmt.Errorf("construct Models: model runtime temporary directory resolver is required")
	case isNilDependency(runtimeTempFile):
		return fmt.Errorf("construct Models: model runtime temporary file creator is required")
	case isNilDependency(now):
		return fmt.Errorf("construct Models: process clock is required")
	default:
		return nil
	}
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func runtimeScopeIssuerID(entropy platformrandom.Source) (string, error) {
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
