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
	"errors"
	"fmt"
	"reflect"
	"time"

	"encoding/hex"
	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models/internal/service"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	assetswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/wire"
	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	inferencewire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/wire"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimehostwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"go.uber.org/zap"
)

const (
	defaultAssetBaseURL    = "https://huggingface.co"
	defaultAssetAPIBaseURL = "https://huggingface.co/api"
)

// InvocationBackend is the narrow external effect used to execute one
// prepared generic request. The Models service owns preparation, lifecycle,
// output normalization, and lease release; an injected backend supplies only
// detached provider-neutral output facts.
type InvocationBackend func(
	context.Context,
	models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error)

// ASRBackend is the typed external effect used by the Models-owned ASR
// codec/runtime. Its request and response remain provider-neutral at this
// construction boundary.
type ASRBackend func(
	context.Context,
	models.ASRBackendRequest,
) (models.ASRBackendResponse, error)

// EmbeddingBackend is the typed external effect used by the Models-owned
// EMBED codec/runtime. Its request and response remain provider-neutral at the
// construction boundary.
type EmbeddingBackend func(
	context.Context,
	models.EmbeddingBackendRequest,
) (models.EmbeddingBackendResponse, error)

// InvocationProtocolClient is the provider-neutral construction port for the
// pinned generic protocol. Protocol-native request types stay inside the
// private LocalAI adapter.
type InvocationProtocolClient interface {
	Predict(context.Context, models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error)
}

// InvocationProtocolDialer is the policy-free transport port used by the
// pinned LocalAI adapter when no provider-neutral fixture client is supplied.
type InvocationProtocolDialer = platformgrpc.Dialer

// RuntimeEvidenceRecord and RuntimeEvidenceRecorder are aliases for the
// parent-private Models evidence seam. They are re-exposed only at this
// construction boundary so the canonical pkg/wire graph can inject the
// integration-only sink without importing an internal package it cannot own.
type RuntimeEvidenceRecord = modelseffects.RuntimeEvidenceRecord
type RuntimeEvidenceRecorder = modelseffects.RuntimeEvidenceRecorder

// NewPinnedGRPCHostProtocolNegotiator exposes the LocalAI-owned readiness
// adapter through the Models construction boundary without exporting any
// backend-native message types.
func NewPinnedGRPCHostProtocolNegotiator(
	dialer InvocationProtocolDialer,
	resolveSymlinks HostResolveSymlinks,
) HostProtocolNegotiator {
	return localai.NewPinnedGRPCHostProtocolNegotiator(dialer, resolveSymlinks)
}

type invocationRuntimeOptions struct {
	Backend          InvocationBackend
	ASR              ASRBackend
	Embedding        EmbeddingBackend
	Client           InvocationProtocolClient
	Dialer           InvocationProtocolDialer
	VideoAudioRunner platformprocess.CommandRunner
	ASRTempDirectory func() string
	ASRCreateTemp    localai.TempFileFactory
	ASRWriteFile     localai.InputFileWriter
	ASRReadFile      func(string) ([]byte, error)
	ASRRemoveFile    localai.InputFileRemover
	TTSTempDirectory func() string
	TTSCreateTemp    localai.TempFileFactory
	TTSWriteFile     localai.InputFileWriter
	TTSInspectFile   localai.TTSOutputInspector
	TTSReadFile      localai.TTSOutputReader
	TTSRemoveFile    localai.InputFileRemover
}

type invocationRuntime interface {
	Invoke(context.Context, inference.InvocationRuntimeRequest) (inference.InvocationRuntimeResult, error)
}

// NewService supplies completed leaves and individually selected effects to Root.
func NewService(
	runtimeScopes RuntimeScopes, assets Assets, catalog Catalog, runtimeHost RuntimeHost, inference Inference,
	resources *ResourceLimiter,
	localExecution ScopedLocalExecution,
	logger *zap.Logger, now func() time.Time, pullMetrics PullMetricsRecorder,
	runtimeEvidence RuntimeEvidenceRecorder, legacyRevisionOverride func(context.Context, string) (string, error),
	backendResolver BackendArtifactResolver, assetPlatform models.AssetHostPlatform,
) (models.Service, error) {
	if isNilDependency(localExecution) {
		return nil, fmt.Errorf("construct Models: scoped local execution is required")
	}
	if legacyRevisionOverride == nil {
		legacyRevisionOverride = NewUnresolvedAssetRevisionResolver()
	}
	return modelsservice.NewRoot(
		resources, localExecution.PullModelForScope, localExecution.InvokeLocal,
		localExecution.CloseScope, localExecution.Close,
		runtimeScopes, catalog, assets, runtimeHost, inference,
		logger, now, pullMetrics, runtimeEvidence,
		legacyRevisionOverride, backendResolver, assetPlatform,
	)
}

// LocalRuntime is the completed, inert local execution adapter shared by scopes.
type LocalRuntime = localmodels.Runtime

// ResourceLimiter retains only scoped reservation state, independently of execution behavior.
type ResourceLimiter = localmodels.ResourceLimiter

type ScopedLocalExecution = modelsservice.ScopedLocalExecution

func NewScopedLocalExecution(scopes RuntimeScopes, assets Assets, host RuntimeHost,
	runtime LocalRuntime, resources *ResourceLimiter, hooks LocalRuntimeHooks,
	now func() time.Time) (ScopedLocalExecution, error) {
	return modelsservice.NewScopedLocalExecution(scopes, assets, host, runtime, resources, hooks, now)
}

func NewResourceLimiter(hooks LocalRuntimeHooks, now func() time.Time) (*ResourceLimiter, error) {
	return localmodels.NewResourceLimiter(hooks, now)
}

func NewLocalRuntime(runner platformprocess.CommandRunner, client RuntimeHTTPDoer,
	inspect RuntimeInspectFile, temp RuntimeTempDirectory, create RuntimeCreateTempFile) (LocalRuntime, error) {
	for _, required := range []struct {
		value any
		name  string
	}{
		{runner, "model runtime command runner"}, {client, "model runtime HTTP client"},
		{inspect, "model runtime file inspector"}, {temp, "model runtime temporary directory resolver"},
		{create, "model runtime temporary file creator"},
	} {
		if isNilDependency(required.value) {
			return nil, fmt.Errorf("construct Models: %s is required", required.name)
		}
	}
	return localmodels.NewOmniVoiceRuntime(runner, client, localmodels.InspectFile(inspect),
		localmodels.TempDirectory(temp), runtimeTempFileAdapter{next: create}.create)
}

func resolveAssetEndpoints(overrides models.RuntimeAssetEndpoints) models.RuntimeAssetEndpoints {
	resolved := models.RuntimeAssetEndpoints{
		BaseURL: defaultAssetBaseURL, APIBaseURL: defaultAssetAPIBaseURL,
	}
	if overrides.BaseURL != "" {
		resolved.BaseURL = overrides.BaseURL
	}
	if overrides.APIBaseURL != "" {
		resolved.APIBaseURL = overrides.APIBaseURL
	}
	return resolved
}

// NewCatalogReadinessQuery connects readiness to the selected Assets role.
func NewCatalogReadinessQuery(assetService Assets) CatalogReadinessQuery {
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
		readiness, readinessErr := localmodels.ManagedRuntimeReadinessForFactoryContext(
			ctx,
			&scope.Runtime,
			detail.Name,
			puller,
			localmodels.DefaultManagedRuntimeSourceResolver(),
		)
		if readinessErr != nil && errors.Is(readinessErr, models.ErrNotFound) &&
			detail.Diagnostics["catalogSource"] == "EFFECTIVE_DEFINITION" {
			return localmodels.ManagedRuntimeReadinessForEffectiveDefinitionContext(
				ctx,
				detail.ManagedRuntime,
				&scope.Runtime,
				detail.Name,
				puller,
			)
		}
		return readiness, readinessErr
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
func NewInvocationArtifactExporter(fileSystem InvocationArtifactFileSystem) (InvocationArtifactExporter, error) {
	return inferencewire.NewInvocationArtifactExporter(fileSystem)
}

type runtimeTempFileAdapter struct {
	next modelseffects.RuntimeCreateTempFile
}

func (a runtimeTempFileAdapter) create(dir, pattern string) (localmodels.TempFile, error) {
	return a.next(dir, pattern)
}

type SlotState = runtimehostwire.SlotState
type HostLeases = runtimehostwire.HostLeases
type SlotFactsProvider = modelseffects.SlotFactsProvider
type SlotCapacityCoordinator = modelseffects.SlotCapacityCoordinator

func NewSlotState() *SlotState { return runtimehostwire.NewSlotState() }
func NewSlotFacts(scopes runtimescopes.Service, assets scopedassets.Service, state *SlotState) SlotFactsProvider {
	return runtimehostwire.NewSlotFacts(scopes, assets, state)
}
func NewSlotCoordinator(state *SlotState, scopes runtimescopes.Service, clock HostClock, logger HostDiagnosticLogger, metrics HostMetricsRecorder, idleUnloadAfter time.Duration) SlotCapacityCoordinator {
	return runtimehostwire.NewSlotCoordinator(state, scopes, clock, logger, metrics, idleUnloadAfter)
}
func NewHostLeases(clock HostClock, facts SlotFactsProvider, coordinator SlotCapacityCoordinator) (HostLeases, error) {
	return runtimehostwire.NewLeases(clock, facts, coordinator)
}

// NewUnresolvedAssetRevisionResolver preserves the disabled asset resolution
// outcome independently of the legacy Root's immutable revision fallback.
func NewUnresolvedAssetRevisionResolver() func(context.Context, string) (string, error) {
	return func(context.Context, string) (string, error) { return "", models.ErrModelRevisionUnresolved }
}

// NormalizeAssetEndpoints retains the caller-facing asset endpoint defaults.
func NormalizeAssetEndpoints(endpoints models.RuntimeAssetEndpoints) models.RuntimeAssetEndpoints {
	return resolveAssetEndpoints(endpoints)
}

// NewOrderedRuntimeEvidenceRecorder retains ordering across the selected leaves.
func NewOrderedRuntimeEvidenceRecorder(next RuntimeEvidenceRecorder) RuntimeEvidenceRecorder {
	return modelseffects.NewOrderedRuntimeEvidenceRecorder(next)
}

// NewInertInvocationArtifactFileSystem retains the fixed leaf artifact policy.
func NewInertInvocationArtifactFileSystem() InvocationArtifactFileSystem {
	return inference.InertArtifactFileSystem{}
}

// Fixed leaf aliases expose construction roles without widening models.Service.
type RuntimeScopes = runtimescopes.Service
type Assets = scopedassets.Service
type Catalog = catalog.Service
type RuntimeHost = runtimehost.Service
type Inference = inference.Service
type CatalogReadinessQuery = catalog.ReadinessQuery
type InvocationRuntime = invocationRuntime
type HostResolveSymlinks = modelseffects.HostResolveSymlinks

// NewRuntimeScopes constructs the process issuer without opening a scope.
func NewRuntimeScopes(issuerEntropy platformrandom.Source) (RuntimeScopes, error) {
	if isNilDependency(issuerEntropy) {
		return nil, fmt.Errorf("construct Models: issuer entropy is required")
	}
	issuerID, err := runtimeScopeIssuerID(issuerEntropy)
	if err != nil {
		return nil, fmt.Errorf("construct Models Runtime Scopes issuer identity: %w", err)
	}
	return runtimescopeswire.NewService(func() string { return issuerID })
}

// NewAssets forwards individually selected asset effects.
func NewAssets(
	scopes RuntimeScopes, platform models.AssetHostPlatform, client AssetHTTPDoer, endpoints models.RuntimeAssetEndpoints,
	makeDirectories AssetMakeDirectories, inspectPath AssetInspectPath, resolveHome AssetResolveHomeDirectory,
	writeFile AssetWriteFile, renamePath AssetRenamePath, removePath AssetRemovePath, readFile AssetReadFile,
	readDirectory AssetReadDirectory, createFile AssetCreateFile, openFile AssetOpenFile,
	resolveEnvironment AssetResolveEnvironment, resolveRevision func(context.Context, string) (string, error),
	coordination AssetStagingCoordination,
) (Assets, error) {
	if err := validateAssetConstructionEffects(client, makeDirectories, inspectPath, resolveHome, writeFile,
		renamePath, removePath, readFile, readDirectory, createFile, openFile); err != nil {
		return nil, err
	}
	if isNilDependency(scopes) {
		return nil, fmt.Errorf("Models Assets runtime scopes service is required")
	}
	if coordination == nil {
		return nil, fmt.Errorf("Models Assets staging coordination is required")
	}
	return assetswire.NewService(scopes, platform, client, endpoints, makeDirectories, inspectPath, resolveHome,
		writeFile, renamePath, removePath, readFile, readDirectory, createFile, openFile, resolveEnvironment, resolveRevision, coordination)
}

func validateAssetConstructionEffects(client AssetHTTPDoer, mkdir AssetMakeDirectories, inspect AssetInspectPath,
	home AssetResolveHomeDirectory, write AssetWriteFile, rename AssetRenamePath, remove AssetRemovePath,
	read AssetReadFile, readDir AssetReadDirectory, create AssetCreateFile, open AssetOpenFile) error {
	for _, required := range []struct {
		value any
		name  string
	}{
		{client, "asset HTTP client"}, {mkdir, "asset make-directories effect"}, {inspect, "asset inspect-path effect"},
		{home, "asset resolve-home effect"}, {write, "asset write-file effect"}, {rename, "asset rename-path effect"},
		{remove, "asset remove-path effect"}, {read, "asset read-file effect"}, {readDir, "asset read-directory effect"},
		{create, "asset create-file effect"}, {open, "asset open-file effect"},
	} {
		if isNilDependency(required.value) {
			return fmt.Errorf("construct Models: %s is required", required.name)
		}
	}
	return nil
}

// NewCatalog consumes the selected readiness effect.
func NewCatalog(scopes RuntimeScopes, readiness CatalogReadinessQuery) (Catalog, error) {
	if isNilDependency(scopes) {
		return nil, fmt.Errorf("Models Catalog runtime scopes service is required")
	}
	if readiness == nil {
		return nil, fmt.Errorf("Models Catalog readiness query is required")
	}
	return catalogwire.NewService(scopes, readiness)
}

// NewRuntimeHost forwards the existing T30 state, lease and host effects.
func NewRuntimeHost(
	scopes RuntimeScopes, assets Assets, leases HostLeases, state *SlotState,
	processLauncher HostProcessLauncher, hostHTTP HostHTTPDoer, hostClock HostClock,
	hostLogger HostDiagnosticLogger, hostMetrics HostMetricsRecorder, platform models.AssetHostPlatform,
	protocol HostProtocolNegotiator, compatibility HostCompatibilityChecker, resolveSymlinks HostResolveSymlinks,
	evidence RuntimeEvidenceRecorder, idleUnloadAfter time.Duration, maxLoadedRuntimes int,
) (RuntimeHost, error) {
	if isNilDependency(processLauncher) {
		return nil, fmt.Errorf("construct Models: model host process launcher is required")
	}
	if isNilDependency(hostHTTP) {
		return nil, fmt.Errorf("construct Models: model host HTTP client is required")
	}
	if isNilDependency(hostClock) {
		return nil, fmt.Errorf("construct Models: model host clock is required")
	}
	return runtimehostwire.NewService(scopes, assets, leases, state, processLauncher, hostHTTP, hostClock,
		hostLogger, hostMetrics, platform, protocol, compatibility, resolveSymlinks, evidence, idleUnloadAfter, maxLoadedRuntimes)
}

// NewInvocationRuntime binds protocol and media effects without selecting a runner.
func NewInvocationRuntime(
	backend InvocationBackend, asr ASRBackend, embedding EmbeddingBackend,
	client InvocationProtocolClient, dialer InvocationProtocolDialer, videoAudioRunner platformprocess.CommandRunner,
	tempDirectory RuntimeTempDirectory, createTemp RuntimeCreateTempFile, writeFile AssetWriteFile,
	inspectFile RuntimeInspectFile, readFile AssetReadFile, removeFile AssetRemovePath,
) (InvocationRuntime, error) {
	options := invocationRuntimeOptions{Backend: backend, ASR: asr, Embedding: embedding,
		Client: client, Dialer: dialer, VideoAudioRunner: videoAudioRunner}
	options = bindASRStaging(options, tempDirectory, createTemp, writeFile, readFile, removeFile)
	options = bindTTSStaging(options, tempDirectory, createTemp, writeFile, inspectFile, readFile, removeFile)
	return inferenceRuntime(options)
}

type InvocationArtifactRegistrar = inferencewire.InvocationArtifactRegistrar

// NewInvocationArtifactRegistrar supplies the completed Inference resource.
func NewInvocationArtifactRegistrar(fileSystem InvocationArtifactFileSystem) (*InvocationArtifactRegistrar, error) {
	return inferencewire.NewInvocationArtifactRegistrar(fileSystem)
}

// NewExecutionDeadline selects the existing Models execution policy.
func NewExecutionDeadline() func() time.Duration {
	return inferencewire.NewExecutionDeadline()
}

// NewInference consumes completed roles and selected time policy.
func NewInference(scopes RuntimeScopes, assets Assets, catalog Catalog, host RuntimeHost,
	runtime InvocationRuntime, registrar *InvocationArtifactRegistrar, now func() time.Time,
	executionDeadline func() time.Duration,
) (Inference, error) {
	if now == nil {
		return nil, fmt.Errorf("construct Models: process clock is required")
	}
	return inferencewire.NewService(scopes, assets, catalog, host, runtime, registrar, now, executionDeadline)
}
