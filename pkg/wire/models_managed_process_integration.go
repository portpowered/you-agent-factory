//go:build managed_process_integration

package wire

import (
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
)

// NewModelsServiceForManagedProcessIntegration exposes the canonical Models
// construction path only to the dedicated compiled-artifact integration lane.
// The build tag keeps this test seam out of the ordinary package and product
// API while preserving the production pkg/wire launcher and defaults.
func NewModelsServiceForManagedProcessIntegration(edges serviceedges.Edges) (models.Service, error) {
	processLogger, err := provideProcessLogger(edges)
	if err != nil {
		return nil, err
	}
	scopes, err := provideModelRuntimeScopes()
	if err != nil {
		return nil, err
	}
	platform, client := provideModelAssetHostPlatform(edges), provideModelAssetHTTP(edges)
	assets, err := managedProcessIntegrationAssets(edges, scopes, platform, client)
	if err != nil {
		return nil, err
	}
	catalog, err := modelswire.NewCatalog(scopes, modelswire.NewCatalogReadinessQuery(assets))
	if err != nil {
		return nil, err
	}
	source, err := provideModelRuntimeEvidenceSource()
	if err != nil {
		return nil, err
	}
	evidence := provideModelOrderedRuntimeEvidence(source)
	launcher, hostHTTP := provideModelHostLauncher(edges, source), provideModelHostHTTP(edges)
	clock, logger, metrics := provideModelHostClock(edges), provideModelHostLogger(processLogger), provideModelHostMetrics(edges)
	host, err := managedProcessIntegrationHost(edges, scopes, assets, platform, launcher, hostHTTP, clock, logger, metrics, evidence)
	if err != nil {
		return nil, err
	}
	runner, err := provideModelRuntimeRunner(edges)
	if err != nil {
		return nil, err
	}
	temp, create, inspect := provideModelRuntimeTempDirectory(edges), provideModelRuntimeTempFile(edges), provideModelRuntimeInspectFile(edges)
	runtime, err := provideModelInvocationRuntime(edges, runner, temp, create, provideModelAssetWriteFile(edges), inspect,
		provideModelAssetReadFile(edges), provideModelAssetRemovePath(edges))
	if err != nil {
		return nil, err
	}
	now := provideModelNow(edges)
	registrar, err := modelswire.NewInvocationArtifactRegistrar(modelswire.NewInertInvocationArtifactFileSystem())
	if err != nil {
		return nil, err
	}
	inference, err := provideModelInference(scopes, assets, catalog, host, runtime, registrar, now, modelswire.NewExecutionDeadline())
	if err != nil {
		return nil, err
	}
	resolver, err := provideModelBackendArtifactResolver(edges, runner, client)
	if err != nil {
		return nil, err
	}
	resources, err := provideModelResourceLimiter(now)
	if err != nil {
		return nil, err
	}
	execution, err := provideModelScopedLocalExecution(scopes, assets)
	if err != nil {
		return nil, err
	}
	return provideModelsService(edges, scopes, assets, catalog, host, inference, resources,
		now, execution, evidence, resolver, provideModelAssetRevision(edges), platform, processLogger)
}

// The tagged seam consumes the canonical providers without selecting defaults
// or assembling a separate compatibility graph.
func managedProcessIntegrationAssets(edges serviceedges.Edges, scopes modelswire.RuntimeScopes,
	platform models.AssetHostPlatform, client modelswire.AssetHTTPDoer) (modelswire.Assets, error) {
	coordination, err := provideModelAssetCoordination(edges)
	if err != nil {
		return nil, err
	}
	return provideModelAssets(scopes, platform, client, provideModelAssetEndpoints(edges),
		provideModelAssetMakeDirectories(edges), provideModelAssetInspectPath(edges), provideModelAssetResolveHomeDirectory(edges),
		provideModelAssetWriteFile(edges), provideModelAssetRenamePath(edges), provideModelAssetRemovePath(edges),
		provideModelAssetReadFile(edges), provideModelAssetReadDirectory(edges), provideModelAssetCreateFile(edges),
		provideModelAssetOpenFile(edges), provideModelAssetResolveEnvironment(edges), provideModelAssetRevision(edges), coordination)
}

func managedProcessIntegrationHost(edges serviceedges.Edges, scopes modelswire.RuntimeScopes, assets modelswire.Assets,
	platform models.AssetHostPlatform, launcher modelswire.HostProcessLauncher, http modelswire.HostHTTPDoer,
	clock modelswire.HostClock, logger modelswire.HostDiagnosticLogger, metrics modelswire.HostMetricsRecorder,
	evidence modelswire.RuntimeEvidenceRecorder) (modelswire.RuntimeHost, error) {
	state := modelswire.NewSlotState()
	coordinator := provideModelSlotCoordinator(state, scopes, clock, logger, metrics)
	leases, err := modelswire.NewHostLeases(clock, provideModelSlotFacts(state, scopes, assets), coordinator)
	if err != nil {
		return nil, err
	}
	compatibility, err := provideModelHostCompatibility(edges)
	if err != nil {
		return nil, err
	}
	symlinks := provideModelHostSymlinks()
	return provideModelRuntimeHost(scopes, assets, leases, state, launcher, http, clock, logger, metrics, platform,
		provideModelHostProtocol(edges, symlinks), compatibility, symlinks, evidence)
}
