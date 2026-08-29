package local

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

// HostPlatform identifies the operating-system and architecture pair used to
// select compatible managed-model assets. Composition observes this process
// fact and injects it; the Models service never reads process globals.
type HostPlatform struct {
	OperatingSystem string
	Architecture    string
}

// AssetPuller resolves managed local model assets and pull outcomes.
type AssetPuller interface {
	PullModel(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (models.PullResult, error)
	EnsureModelAvailable(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) error
	ResolveModelCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) (CacheLayout, error)
	InspectRuntimeCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (RuntimeCacheInspection, error)
}

type assetPuller struct {
	inner        assets.Service
	resolveScope func() (models.RuntimeScopeRef, error)
	prepare      func(context.Context, string, models.RuntimeScopeRef) (models.PrepareModelAssetsRequest, error)
	scopeMu      sync.Mutex
	scope        models.RuntimeScopeRef
}

// NewScopedAssetPuller adapts the already-constructed Models Assets service to
// the existing local runtime without constructing another puller or effect
// bundle for a selected Factory.
func NewScopedAssetPuller(inner assets.Service, scope models.RuntimeScopeRef) (AssetPuller, error) {
	if inner == nil {
		return nil, fmt.Errorf("construct local model asset adapter: Models Assets service is required")
	}
	if scope.IsZero() {
		return nil, fmt.Errorf("construct local model asset adapter: runtime scope is required")
	}
	return &assetPuller{
		inner: inner,
		resolveScope: func() (models.RuntimeScopeRef, error) {
			return scope, nil
		},
	}, nil
}

// NewScopedAssetPullerWithPreparation adapts the scoped asset service while
// allowing the Models root to supply the already-resolved backend/model
// request. The default constructor remains available for legacy callers that
// intentionally use the plain name-only request.
func NewScopedAssetPullerWithPreparation(
	inner assets.Service,
	scope models.RuntimeScopeRef,
	prepare func(context.Context, string, models.RuntimeScopeRef) (models.PrepareModelAssetsRequest, error),
) (AssetPuller, error) {
	puller, err := NewScopedAssetPuller(inner, scope)
	if err != nil {
		return nil, err
	}
	adapter := puller.(*assetPuller)
	adapter.prepare = prepare
	return adapter, nil
}

func (p *assetPuller) PullModel(ctx context.Context, _ *models.RuntimeConfig, modelName string) (models.PullResult, error) {
	scope, err := p.currentScope()
	if err != nil {
		return models.PullResult{}, err
	}
	request := models.PrepareModelAssetsRequest{Scope: scope, Name: modelName}
	if p.prepare != nil {
		request, err = p.prepare(ctx, modelName, scope)
		if err != nil {
			return models.PullResult{}, err
		}
	}
	result, err := p.inner.PrepareModelAssets(ctx, request)
	projected := pullResultFromAssets(result)
	if err != nil {
		projected.Outcome = legacyPullOutcomeFailed
		projected.ManagedPullOutcome = ""
		projected.ReadinessState = "FAILED"
		projected.LifecycleState = "NOT_INSTALLED"
		projected.PullDiagnostics = pullsupport.PullDiagnosticsFromError(err).WithDefaults(
			modelName, projected.SourceID, projected.Revision, "", "prepare model assets",
		)
		return projected, err
	}
	layout, layoutErr := p.inner.ResolveRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  modelName,
	})
	if layoutErr != nil {
		classified := pullsupport.WrapPullStage(
			models.PullStageCacheInstallation, modelName,
			"resolve managed runtime cache", "", layoutErr,
		)
		projected.PullDiagnostics = pullsupport.PullDiagnosticsFromError(classified).WithDefaults(
			modelName, projected.SourceID, projected.Revision, "", "resolve managed runtime cache",
		)
		return projected, classified
	}
	projected.CachePath = layout.CachePath
	return projected, nil
}

func (p *assetPuller) EnsureModelAvailable(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) error {
	_, err := p.ResolveModelCache(ctx, runtimeCfg, worker)
	return err
}

func (p *assetPuller) ResolveModelCache(ctx context.Context, _ *models.RuntimeConfig, worker *models.RuntimeWorker) (CacheLayout, error) {
	if worker == nil || strings.TrimSpace(worker.ModelLocality) != models.RuntimeModelLocalityLocal {
		return CacheLayout{}, nil
	}
	scope, err := p.currentScope()
	if err != nil {
		return CacheLayout{}, err
	}
	layout, err := p.inner.ResolveRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  worker.Model,
	})
	if err != nil {
		return CacheLayout{}, err
	}
	return CacheLayout{
		ModelName: layout.ModelName,
		CachePath: layout.CachePath,
		Revision:  layout.Revision,
		Files:     layout.Files,
	}, nil
}

func (p *assetPuller) InspectRuntimeCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (RuntimeCacheInspection, error) {
	if runtimeCfg == nil {
		return RuntimeCacheInspection{}, fmt.Errorf("runtime config is not available")
	}
	if modelScopedResource(runtimeCfg, modelName) == nil {
		// Factory workers without an explicit model resource retain the legacy
		// catalog-only behavior. Effective built-in definitions, which are
		// absent from RuntimeConfig, still need the durable generic cache probe.
		if factoryModelWorker(runtimeCfg, modelName) != nil {
			return RuntimeCacheInspection{}, nil
		}
		if _, builtIn := (models.BuiltInCatalog{}).ModelDefinitionFor(strings.ToLower(strings.TrimSpace(modelName))); !builtIn {
			return RuntimeCacheInspection{}, nil
		}
	}
	scope, err := p.currentScope()
	if err != nil {
		return RuntimeCacheInspection{}, err
	}
	inspection, err := p.inner.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  modelName,
	})
	if err != nil {
		return RuntimeCacheInspection{}, err
	}
	return RuntimeCacheInspection{
		Supported:          inspection.Supported,
		Installed:          inspection.Installed,
		Revision:           inspection.Revision,
		CachePath:          inspection.CachePath,
		CacheBytes:         inspection.CacheBytes,
		InstalledFileCount: inspection.InstalledFileCount,
		MissingAssets:      inspection.MissingAssets,
		PartialArtifacts:   inspection.PartialArtifacts,
		ManifestPresent:    inspection.ManifestPresent,
		ManifestValid:      inspection.ManifestValid,
		ExpectedArtifacts:  append([]models.AssetRequirement(nil), inspection.ExpectedArtifacts...),
		ObservedArtifacts:  append([]models.AssetArtifact(nil), inspection.ObservedArtifacts...),
		ActivePull:         inspection.ActivePull,
		IntegrityVerified:  inspection.IntegrityVerified,
		FailureReason:      inspection.FailureReason,
	}, nil
}

func (p *assetPuller) currentScope() (models.RuntimeScopeRef, error) {
	if p == nil || p.resolveScope == nil {
		return models.RuntimeScopeRef{}, fmt.Errorf("Models runtime scope is unavailable")
	}
	p.scopeMu.Lock()
	defer p.scopeMu.Unlock()
	if !p.scope.IsZero() {
		return p.scope, nil
	}
	scope, err := p.resolveScope()
	if err != nil {
		return models.RuntimeScopeRef{}, err
	}
	if scope.IsZero() {
		return models.RuntimeScopeRef{}, fmt.Errorf("Models runtime scope is unavailable")
	}
	p.scope = scope
	return scope, nil
}

func factoryModelWorker(runtimeCfg *models.RuntimeConfig, modelName string) *models.RuntimeWorker {
	if runtimeCfg == nil {
		return nil
	}
	key := strings.ToUpper(strings.TrimSpace(modelName))
	for index := range runtimeCfg.Workers {
		worker := &runtimeCfg.Workers[index]
		if strings.ToUpper(strings.TrimSpace(worker.Model)) == key {
			return worker
		}
	}
	return nil
}

func pullResultFromAssets(result models.PrepareModelAssetsResult) models.PullResult {
	files := make([]models.DownloadedFile, 0, len(result.Asset.Artifacts))
	for _, artifact := range result.Asset.Artifacts {
		files = append(files, models.DownloadedFile{
			Path: artifact.Name, Bytes: artifact.Bytes, SHA256: artifact.SHA256,
		})
	}
	outcome := "PULLED"
	managedOutcome := "INSTALLED_SUCCESSFULLY"
	if result.Outcome == models.AssetPreparationAlreadyAvailable {
		outcome = "ALREADY_PRESENT"
		managedOutcome = "ALREADY_READY"
	}
	return models.PullResult{
		ModelName:          result.Asset.ModelName,
		ProviderLocality:   models.RuntimeModelLocalityLocal,
		Outcome:            outcome,
		Revision:           result.Asset.Revision,
		DownloadedFiles:    files,
		ManagedPullOutcome: managedOutcome,
		ReadinessState:     "READY",
		LifecycleState:     "INSTALLED",
		SourceKind:         result.Asset.Source.Provider,
		SourceID:           result.Asset.Source.Reference,
	}
}

const (
	legacyPullOutcomePulled         = "PULLED"
	legacyPullOutcomeAlreadyPresent = "ALREADY_PRESENT"
	legacyPullOutcomeFailed         = "FAILED"

	managedPullOutcomeAlreadyReady                = "ALREADY_READY"
	managedPullOutcomeInstalledSuccessfully       = "INSTALLED_SUCCESSFULLY"
	managedPullOutcomeAlreadyPresent              = "ALREADY_PRESENT"
	managedPullOutcomeStillLoading                = "STILL_LOADING"
	managedPullOutcomeTimedOut                    = "TIMED_OUT"
	managedPullOutcomeCancelled                   = "CANCELLED"
	managedPullOutcomeSourceFetchFailed           = "SOURCE_FETCH_FAILED"
	managedPullOutcomeUnsupportedRuntime          = "UNSUPPORTED_RUNTIME"
	managedPullOutcomeSourceResolutionFailed      = "SOURCE_RESOLUTION_FAILED"
	managedPullOutcomeIntegrityVerificationFailed = "INTEGRITY_VERIFICATION_FAILED"
	managedPullOutcomeAssemblyFailed              = "ASSEMBLY_FAILED"
	managedPullOutcomeCacheInstallationFailed     = "CACHE_INSTALLATION_FAILED"
	managedPullOutcomeReadinessEvaluationFailed   = "READINESS_EVALUATION_FAILED"
	managedPullOutcomeAssetPreparationFailed      = "ASSET_PREPARATION_FAILED"

	managedReadinessReady       = "READY"
	managedReadinessMissing     = "MISSING"
	managedReadinessLoading     = "LOADING"
	managedReadinessFailed      = "FAILED"
	managedReadinessUnsupported = "UNSUPPORTED"

	managedLifecycleInstalling   = "INSTALLING"
	managedLifecycleInstalled    = "INSTALLED"
	managedLifecycleNotInstalled = "NOT_INSTALLED"
)

// EnrichPullResult projects a service-owned pull result into managed-runtime
// readiness, lifecycle, and source diagnostics using post-pull cache inspection.
func EnrichPullResult(
	result models.PullResult,
	inspection RuntimeCacheInspection,
	resolution ManagedRuntimeSourceResolution,
) models.PullResult {
	outcome, readiness, lifecycle := classifySuccessfulPull(result, inspection)
	result.ManagedPullOutcome = outcome
	result.ReadinessState = readiness
	result.LifecycleState = lifecycle
	if readiness == managedReadinessFailed && result.FailureStage == "" {
		result.FailureStage = pullStageForManagedOutcome(outcome)
	}
	result.SourceKind = strings.TrimSpace(resolution.SourceKind)
	result.SourceID = strings.TrimSpace(resolution.SourceID)
	result.ResolverNotes = strings.TrimSpace(resolution.ResolverNotes)
	return result
}

// ClassifyPullFailure maps pull errors to managed-runtime pull outcomes and
// readiness states for logging, metrics, and stable customer-facing vocabulary.
func ClassifyPullFailure(err error) (pullOutcome string, readiness string) {
	if err == nil {
		return "", ""
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return managedPullOutcomeTimedOut, managedReadinessFailed
	case errors.Is(err, context.Canceled), errors.Is(err, models.ErrAssetCancelled):
		return managedPullOutcomeCancelled, managedReadinessFailed
	case errors.Is(err, models.ErrPullUnsupported):
		return managedPullOutcomeUnsupportedRuntime, managedReadinessUnsupported
	}
	switch pullsupport.PullStageForError(err) {
	case models.PullStageSourceResolution:
		return managedPullOutcomeSourceResolutionFailed, managedReadinessFailed
	case models.PullStageSourceFetch:
		return managedPullOutcomeSourceFetchFailed, managedReadinessFailed
	case models.PullStageIntegrityVerification:
		return managedPullOutcomeIntegrityVerificationFailed, managedReadinessFailed
	case models.PullStageAssembly:
		return managedPullOutcomeAssemblyFailed, managedReadinessFailed
	case models.PullStageCacheInstallation:
		return managedPullOutcomeCacheInstallationFailed, managedReadinessFailed
	case models.PullStageReadinessEvaluation:
		return managedPullOutcomeReadinessEvaluationFailed, managedReadinessFailed
	}
	if isIntegrityFailureMessage(err.Error()) {
		return managedPullOutcomeIntegrityVerificationFailed, managedReadinessFailed
	}
	if isSourceFetchFailureMessage(err.Error()) {
		return managedPullOutcomeSourceFetchFailed, managedReadinessFailed
	}
	return managedPullOutcomeAssetPreparationFailed, managedReadinessFailed
}

func classifySuccessfulPull(result models.PullResult, inspection RuntimeCacheInspection) (pullOutcome, readiness, lifecycle string) {
	legacyOutcome := strings.ToUpper(strings.TrimSpace(result.Outcome))
	switch legacyOutcome {
	case legacyPullOutcomePulled:
		pullOutcome = managedPullOutcomeInstalledSuccessfully
	case legacyPullOutcomeAlreadyPresent:
		pullOutcome = managedPullOutcomeAlreadyPresent
	default:
		pullOutcome = managedPullOutcomeUnsupportedRuntime
	}

	if inspection.Supported {
		projection := managedruntime.ProjectManagedRuntimeState(
			managedRuntimeCacheFacts(managedruntime.LocalityLocal, inspection),
			models.ManagedRuntimeHostFacts{},
		)
		readiness = string(projection.ReadinessState)
		lifecycle = string(projection.LifecycleState)
		switch projection.ReadinessState {
		case managedruntime.ReadinessStateReady:
			if pullOutcome == managedPullOutcomeAlreadyPresent {
				pullOutcome = managedPullOutcomeAlreadyReady
			}
		case managedruntime.ReadinessStateLoading:
			pullOutcome = managedPullOutcomeStillLoading
		case managedruntime.ReadinessStateFailed:
			pullOutcome = pullOutcomeForFailedInspection(inspection)
		case managedruntime.ReadinessStateMissing:
			if pullOutcome == managedPullOutcomeInstalledSuccessfully {
				readiness = managedReadinessLoading
				lifecycle = managedLifecycleInstalling
				pullOutcome = managedPullOutcomeStillLoading
			}
		}
		return pullOutcome, readiness, lifecycle
	}

	readiness = managedReadinessReady
	lifecycle = managedLifecycleInstalled
	if pullOutcome == managedPullOutcomeAlreadyPresent {
		pullOutcome = managedPullOutcomeAlreadyReady
	}
	return pullOutcome, readiness, lifecycle
}

func pullOutcomeForFailedInspection(inspection RuntimeCacheInspection) string {
	reason := strings.ToLower(strings.TrimSpace(inspection.FailureReason))
	if strings.Contains(reason, "integrity") || strings.Contains(reason, "checksum") ||
		strings.Contains(reason, "unexpected size") {
		return managedPullOutcomeIntegrityVerificationFailed
	}
	return managedPullOutcomeReadinessEvaluationFailed
}

func pullStageForManagedOutcome(outcome string) models.PullStage {
	switch outcome {
	case managedPullOutcomeSourceResolutionFailed:
		return models.PullStageSourceResolution
	case managedPullOutcomeSourceFetchFailed:
		return models.PullStageSourceFetch
	case managedPullOutcomeIntegrityVerificationFailed:
		return models.PullStageIntegrityVerification
	case managedPullOutcomeAssemblyFailed:
		return models.PullStageAssembly
	case managedPullOutcomeCacheInstallationFailed:
		return models.PullStageCacheInstallation
	case managedPullOutcomeReadinessEvaluationFailed:
		return models.PullStageReadinessEvaluation
	default:
		return ""
	}
}

func isIntegrityFailureMessage(message string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(message))
	if trimmed == "" {
		return false
	}
	for _, fragment := range []string{"integrity", "checksum", "unexpected size"} {
		if strings.Contains(trimmed, fragment) {
			return true
		}
	}
	return false
}

func isSourceFetchFailureMessage(message string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(message))
	if trimmed == "" {
		return false
	}
	for _, fragment := range []string{
		"pull model manifest",
		"download model asset",
		"model asset request failed",
	} {
		if strings.Contains(trimmed, fragment) {
			return true
		}
	}
	return false
}
