package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/internal/reclamation"
)

// genericPreflightState is retained only for the immediately-following
// preparation transaction. It contains no open handles and is deliberately
// not persisted as cache state.
type genericPreflightState struct {
	plan          genericPreparationPlan
	modelMissing  []genericArtifact
	backendMissed []genericArtifact
}

// PreflightModelAssets resolves cache-aware requirements and validates remote
// reachability without reading artifact response bodies. Generic sources use
// a backend-first metadata order so a failed backend cannot be followed by a
// model transfer.
func (s *service) PreflightModelAssets(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PreflightModelAssetsResult, error) {
	if err := request.Validate(); err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	if err := assetContextError(ctx); err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	if shouldPrepareGenericAssets(request) {
		return s.preflightGenericAssets(ctx, request)
	}
	return s.preflightLegacyAssets(ctx, request)
}

func (s *service) preflightGenericAssets(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PreflightModelAssetsResult, error) {
	state, err := s.preflightGenericPreparation(ctx, request, true)
	if err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	backendBytes, err := sumGenericMissingBytes(state.backendMissed)
	if err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	modelBytes, err := sumGenericMissingBytes(state.modelMissing)
	if err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	totalBytes, err := addAssetBytes(backendBytes, modelBytes)
	if err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	return models.PreflightModelAssetsResult{
		ModelName:               strings.TrimSpace(request.Name),
		BackendBytes:            backendBytes,
		ModelBytes:              modelBytes,
		TotalBytes:              totalBytes,
		BackendDownloadRequired: len(state.backendMissed) > 0,
		ModelDownloadRequired:   len(state.modelMissing) > 0,
	}, nil
}

func (s *service) preflightGenericPreparation(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
	requirePositiveMissingBytes bool,
) (genericPreflightState, error) {
	plan, err := s.genericPreparationPlan(ctx, request)
	if err != nil {
		return genericPreflightState{}, wrapAssetPreflightError(request.Name, err)
	}
	// Reject an impossible declared total before any HEAD or metadata request.
	// This keeps an overflow a validation failure rather than allowing a
	// partially observed preflight to proceed.
	if _, err := sumGenericRequirements(plan.modelRequirements, plan.backendRequirements); err != nil {
		return genericPreflightState{}, err
	}

	// Backend reachability is intentionally checked before model metadata. A
	// backend failure therefore cannot cause a later model content request.
	var backendPreflightErr error
	backendFacts := genericPreflightFacts{}
	if len(plan.backendRequirements) > 0 {
		backendFacts, backendPreflightErr = s.preflightGenericArtifactSet(
			ctx, assetKindBackend, plan.backendSource, plan.backendRequirements,
			plan.backendRoots, request.Offline, requirePositiveMissingBytes,
		)
		if backendPreflightErr != nil {
			// Offline mode is an inspection mode: collect the model-side
			// missing set as well so callers receive one complete report. Any
			// other backend failure must stop before model metadata/content.
			var offlineErr *models.AssetOfflineError
			if !errors.As(backendPreflightErr, &offlineErr) {
				if !errors.Is(backendPreflightErr, context.Canceled) && !errors.Is(backendPreflightErr, context.DeadlineExceeded) {
					backendPreflightErr = fmt.Errorf("%w: %w", models.ErrAssetBackendNotReady, backendPreflightErr)
				}
				return genericPreflightState{plan: plan}, wrapAssetPreflightError(request.Name, backendPreflightErr)
			}
		}
	}
	plan.backendRequirements = backendFacts.artifacts

	modelFacts, err := s.preflightInstalledGenericRuntime(ctx, &plan)
	if err != nil {
		return genericPreflightState{plan: plan}, wrapAssetPreflightError(request.Name, err)
	}
	if modelFacts == nil {
		facts, preflightErr := s.preflightGenericArtifactSet(
			ctx, assetKindModel, plan.source, plan.modelRequirements,
			plan.modelRoots, request.Offline, requirePositiveMissingBytes,
		)
		modelFacts = &facts
		if preflightErr != nil {
			if backendPreflightErr != nil {
				if combined := combinedOfflineError(backendPreflightErr, preflightErr); combined != nil {
					return genericPreflightState{plan: plan, modelMissing: modelFacts.missing, backendMissed: backendFacts.missing}, wrapAssetPreflightError(request.Name, combined)
				}
			}
			return genericPreflightState{plan: plan}, wrapAssetPreflightError(request.Name, preflightErr)
		}
	}
	plan.modelRequirements = modelFacts.artifacts
	state := genericPreflightState{
		plan:          plan,
		modelMissing:  modelFacts.missing,
		backendMissed: backendFacts.missing,
	}
	if backendPreflightErr != nil {
		return state, wrapAssetPreflightError(request.Name, backendPreflightErr)
	}
	if _, err := sumGenericMissingBytes(state.modelMissing, state.backendMissed); err != nil {
		return genericPreflightState{}, err
	}
	return state, nil
}

type genericPreflightFacts struct {
	artifacts []genericArtifact
	missing   []genericArtifact
}

// preflightInstalledGenericRuntime reuses a verified managed runtime before
// consulting an upstream source. The durable runtime metadata is the
// authoritative cache record for a model already installed by this service;
// older content-addressed metadata may intentionally lack integrity facts and
// must not force a fresh download when the managed record is verified.
func (s *service) preflightInstalledGenericRuntime(
	ctx context.Context,
	plan *genericPreparationPlan,
) (*genericPreflightFacts, error) {
	if plan == nil {
		return nil, nil
	}
	inspection, reusable, err := s.reusableGenericRuntimeCache(ctx, *plan)
	if err != nil || !reusable {
		return nil, err
	}
	plan.modelRuntimeCache = inspection
	artifacts := s.genericArtifactsFromRequirements(
		plan.source, inspection.ExpectedArtifacts,
	)
	return &genericPreflightFacts{artifacts: artifacts}, nil
}

func (s *service) preflightGenericArtifactSet(
	ctx context.Context,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
	roots []string,
	offline bool,
	requirePositiveMissingBytes bool,
) (genericPreflightFacts, error) {
	resolved, err := s.resolveGenericPreflightArtifacts(ctx, kind, source, artifacts, roots, offline)
	if err != nil {
		return genericPreflightFacts{}, err
	}
	s.addGenericURLs(source, resolved)
	if err := assetContextError(ctx); err != nil {
		return genericPreflightFacts{}, err
	}
	cached, missing, err := s.inspectGenericCache(ctx, kind, source, resolved, roots)
	if err != nil {
		return genericPreflightFacts{}, err
	}
	resolved, cached, missing, err = s.resolveGenericPreflightManifest(
		ctx, kind, source, resolved, roots, cached, missing, offline,
	)
	if err != nil {
		return genericPreflightFacts{artifacts: resolved, missing: missing}, err
	}
	if len(missing) == 0 {
		return genericPreflightFacts{artifacts: resolved}, nil
	}
	if offline {
		return genericPreflightFacts{artifacts: resolved, missing: missing}, &models.AssetOfflineError{
			Missing: missingArtifactNames(missing),
		}
	}
	if source.kind == genericSourceRelease ||
		(requirePositiveMissingBytes && source.kind == genericSourceHF && genericArtifactsNeedSize(missing)) {
		resolved, missing, err = s.headGenericArtifacts(ctx, source, resolved, cached, missing)
		if err != nil {
			return genericPreflightFacts{artifacts: resolved, missing: missing}, err
		}
	}
	if requirePositiveMissingBytes {
		if err := requireGenericMissingSizes(missing); err != nil {
			return genericPreflightFacts{artifacts: resolved, missing: missing}, err
		}
	}
	return genericPreflightFacts{artifacts: resolved, missing: missing}, nil
}

func genericArtifactsNeedSize(artifacts []genericArtifact) bool {
	for _, artifact := range artifacts {
		if artifact.requirement.Bytes <= 0 {
			return true
		}
	}
	return false
}

func requireGenericMissingSizes(artifacts []genericArtifact) error {
	for _, artifact := range artifacts {
		if artifact.requirement.Bytes <= 0 {
			return fmt.Errorf(
				"%w: asset %q size is unavailable after metadata resolution",
				models.ErrSourceFetchFailed, artifact.requirement.Name,
			)
		}
	}
	return nil
}

func (s *service) resolveGenericPreflightArtifacts(
	ctx context.Context,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
	roots []string,
	offline bool,
) ([]genericArtifact, error) {
	resolved := append([]genericArtifact(nil), artifacts...)
	if source.kind != genericSourceHF {
		return resolved, nil
	}
	discovered := s.discoverContentAddressedRequirementsAcrossRoots(kind, source, roots)
	if len(discovered) > 0 {
		discoveredArtifacts := s.genericArtifactsFromRequirements(source, discovered)
		// Legacy content metadata can describe the artifact names while still
		// carrying zero/blank integrity facts. It is useful discovery input, but
		// it cannot suppress immutable-manifest resolution before repair has
		// reverified the bytes.
		if genericArtifactsHaveTrustedFacts(discoveredArtifacts) {
			markGenericArtifactsResolved(discoveredArtifacts)
		}
		if len(resolved) == 0 {
			resolved = discoveredArtifacts
		} else {
			merged, err := mergeDiscoveredGenericArtifacts(resolved, discoveredArtifacts)
			if err != nil {
				return nil, err
			}
			resolved = merged
		}
	}
	if len(resolved) > 0 {
		return resolved, nil
	}
	if offline {
		return nil, &models.AssetOfflineError{Missing: []string{source.repository}}
	}
	return s.fetchGenericManifest(ctx, source)
}

func (s *service) resolveGenericPreflightManifest(
	ctx context.Context,
	kind string,
	source genericSource,
	resolved []genericArtifact,
	roots []string,
	cached map[string]genericCachePath,
	missing []genericArtifact,
	offline bool,
) ([]genericArtifact, map[string]genericCachePath, []genericArtifact, error) {
	if source.kind != genericSourceHF || len(missing) == 0 || !genericArtifactsNeedManifest(resolved) {
		return resolved, cached, missing, nil
	}
	if offline {
		return resolved, cached, missing, &models.AssetOfflineError{
			Missing: missingArtifactNames(missing),
		}
	}
	manifest, err := s.fetchGenericManifest(ctx, source)
	if err != nil {
		return resolved, cached, missing, err
	}
	resolved, err = mergeGenericManifest(resolved, manifest)
	if err != nil {
		return nil, nil, nil, err
	}
	s.addGenericURLs(source, resolved)
	cached, missing, err = s.inspectGenericCache(ctx, kind, source, resolved, roots)
	return resolved, cached, missing, err
}

func genericArtifactsNeedManifest(artifacts []genericArtifact) bool {
	if len(artifacts) == 0 {
		return true
	}
	for _, artifact := range artifacts {
		if !artifact.metadataResolved {
			return true
		}
	}
	return false
}

func markGenericArtifactsResolved(artifacts []genericArtifact) {
	for index := range artifacts {
		artifacts[index].metadataResolved = true
	}
}

// headGenericArtifacts checks release reachability without consuming the
// response body. Requirement sizes remain authoritative when supplied by the
// selected backend artifact; otherwise Content-Length supplies the estimate.
func (s *service) headGenericArtifacts(
	ctx context.Context,
	source genericSource,
	artifacts []genericArtifact,
	cached map[string]genericCachePath,
	missing []genericArtifact,
) ([]genericArtifact, []genericArtifact, error) {
	missingByName := make(map[string]int, len(missing))
	for index, artifact := range missing {
		missingByName[artifact.requirement.Name] = index
	}
	for index := range artifacts {
		artifact := artifacts[index]
		missingIndex, isMissing := missingByName[artifact.requirement.Name]
		if !isMissing {
			continue
		}
		if _, alreadyCached := cached[artifact.requirement.Name]; alreadyCached {
			continue
		}
		checked, err := s.headGenericArtifact(ctx, source, artifact)
		if err != nil {
			return artifacts, missing, err
		}
		artifacts[index] = checked
		missing[missingIndex] = checked
	}
	return artifacts, missing, nil
}

func (s *service) headGenericArtifact(
	ctx context.Context,
	source genericSource,
	artifact genericArtifact,
) (genericArtifact, error) {
	if err := assetContextError(ctx); err != nil {
		return artifact, err
	}
	assetURL := s.genericAssetURL(source, artifact.requirement.Name)
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, assetURL, nil)
	if err != nil {
		return artifact, fmt.Errorf("%w: build asset reachability request", models.ErrSourceFetchFailed)
	}
	response, err := s.doWithRetry(request)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return artifact, contextErr
		}
		return artifact, fmt.Errorf("%w: check backend asset reachability", models.ErrSourceFetchFailed)
	}
	if response == nil {
		return artifact, fmt.Errorf("%w: empty asset reachability response", models.ErrSourceFetchFailed)
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return artifact, fmt.Errorf(
			"%w: backend asset reachability failed (%d)", models.ErrSourceFetchFailed, response.StatusCode,
		)
	}
	contentLength := response.ContentLength
	declared := artifact.requirement.Bytes
	if declared <= 0 {
		if contentLength <= 0 {
			return artifact, fmt.Errorf(
				"%w: asset %q size is unavailable from HEAD", models.ErrSourceFetchFailed, artifact.requirement.Name,
			)
		}
		declared = contentLength
		artifact.requirement.Bytes = declared
	}
	// A hand-built or proxy response may leave ContentLength at zero even
	// though the selected requirement already supplies the authoritative
	// estimate. Only a positive observed length can contradict that
	// declaration; HEAD is a reachability check, never an integrity proof.
	if contentLength > 0 && declared != contentLength {
		return artifact, fmt.Errorf(
			"%w: asset %q HEAD size does not match its declared size", models.ErrAssetIntegrityFailed, artifact.requirement.Name,
		)
	}
	artifact.metadataResolved = true
	return artifact, nil
}

func (s *service) preflightLegacyAssets(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PreflightModelAssetsResult, error) {
	scope, err := s.resolveScope(ctx, request.Scope)
	if err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	spec, source, err := s.resolveSource(scope.Runtime, request.Name)
	if err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	if snapshot, available, inspectErr := s.inspectVerifiedCache(ctx, scope.CacheDirectory, spec, source); inspectErr != nil {
		return models.PreflightModelAssetsResult{}, inspectErr
	} else if available && snapshot.Integrity == models.AssetIntegrityVerified {
		return models.PreflightModelAssetsResult{ModelName: strings.TrimSpace(request.Name)}, nil
	}
	manifest, err := s.fetchManifest(ctx, spec)
	if err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	snapshot, available, err := s.inspectManifestCache(ctx, scope.CacheDirectory, spec, source, manifest)
	if err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	if available {
		return models.PreflightModelAssetsResult{ModelName: strings.TrimSpace(request.Name)}, nil
	}
	observed := make(map[string]struct{}, len(snapshot.Artifacts))
	for _, artifact := range snapshot.Artifacts {
		observed[artifact.Name] = struct{}{}
	}
	missing := make([]genericArtifact, 0, len(manifest.files))
	for _, file := range manifest.files {
		if _, ok := observed[file.path]; ok {
			continue
		}
		missing = append(missing, genericArtifact{
			requirement:      models.AssetRequirement{Name: file.path, Bytes: file.bytes, SHA256: file.sha256},
			metadataResolved: true,
		})
	}
	modelBytes, err := sumGenericMissingBytes(missing)
	if err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	return models.PreflightModelAssetsResult{
		ModelName:             strings.TrimSpace(request.Name),
		ModelBytes:            modelBytes,
		TotalBytes:            modelBytes,
		ModelDownloadRequired: len(missing) > 0,
	}, nil
}

func sumGenericRequirements(groups ...[]genericArtifact) (int64, error) {
	var total int64
	for _, group := range groups {
		for _, artifact := range group {
			var err error
			total, err = addAssetBytes(total, artifact.requirement.Bytes)
			if err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}

func sumGenericMissingBytes(groups ...[]genericArtifact) (int64, error) {
	return sumGenericRequirements(groups...)
}

func addAssetBytes(left, right int64) (int64, error) {
	if left < 0 || right < 0 || left > math.MaxInt64-right {
		return 0, fmt.Errorf("%w: asset byte total cannot be represented", models.ErrAssetEstimateOverflow)
	}
	return left + right, nil
}

func wrapAssetPreflightError(modelName string, err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	stage := pullsupport.PullStageForError(err)
	if stage == "" {
		stage = models.PullStageAssembly
	}
	return pullsupport.WrapPullStage(stage, modelName, "preflight model assets", "", err)
}

// RemoveModelAssets removes exactly the selected managed revision. All path
// traversal is based on directory entries so symlinks are unlinked rather
// than followed.
func (s *service) RemoveModelAssets(
	ctx context.Context,
	request models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	result := models.RemoveModelAssetsResult{
		ModelName: canonicalModelName(request.Name),
		Readiness: models.AssetReadinessMissing,
	}
	if err := request.Validate(); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	scope, err := s.resolveScope(ctx, request.Scope)
	if err != nil {
		return result, err
	}
	spec, source, err := s.resolveRemovalSource(scope.Runtime, request.Name)
	if err != nil {
		if !errors.Is(err, models.ErrModelCacheNotFound) {
			return result, err
		}
		return s.removeGenericModelAssets(ctx, request, result, scope)
	}
	result.ModelName = spec.modelName
	if err := assetContextError(ctx); err != nil {
		return result, err
	}
	return s.removeResolvedModelAssets(ctx, request, result, scope, spec, source)
}

func (s *service) removeGenericModelAssets(
	ctx context.Context,
	request models.RemoveModelAssetsRequest,
	result models.RemoveModelAssetsResult,
	scope models.RuntimeScopeConfig,
) (models.RemoveModelAssetsResult, error) {
	target, err := s.resolveGenericRemovalTarget(ctx, scope, request.Name)
	if err != nil {
		return result, err
	}
	resolve := func() (genericRemovalTarget, error) {
		return s.resolveGenericRemovalTarget(ctx, scope, request.Name)
	}
	return s.removeManagedModelTarget(ctx, request, result, target, resolve)
}

type genericRemovalTarget struct {
	modelName    string
	modelRoot    string
	revision     string
	revisionPath string
	sourceKey    string
}

func (s *service) resolveGenericRemovalTarget(
	ctx context.Context,
	scope models.RuntimeScopeConfig,
	modelName string,
) (genericRemovalTarget, error) {
	source, err := s.resolveGenericSource(ctx, scope, modelName)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return genericRemovalTarget{}, contextErr
		}
		return genericRemovalTarget{}, modelCacheNotFound(modelName)
	}
	inspection, present, err := s.inspectGenericRuntimeCache(
		ctx, scope.CacheDirectory, modelName, source,
	)
	if err != nil {
		return genericRemovalTarget{}, err
	}
	if !genericRemovalInspectionReady(present, inspection) {
		return genericRemovalTarget{}, modelCacheNotFound(modelName)
	}
	return s.validateGenericRemovalTarget(
		ctx, scope.CacheDirectory, canonicalModelName(modelName), inspection, source,
	)
}

func genericRemovalInspectionReady(
	present bool,
	inspection assets.RuntimeCacheInspection,
) bool {
	return present && inspection.Installed &&
		strings.TrimSpace(inspection.Revision) != "" &&
		strings.TrimSpace(inspection.CachePath) != ""
}

func (s *service) validateGenericRemovalTarget(
	ctx context.Context,
	cacheDirectory string,
	modelName string,
	inspection assets.RuntimeCacheInspection,
	source genericSource,
) (genericRemovalTarget, error) {
	modelRoot, err := s.modelCacheRoot(cacheDirectory, modelName)
	if err != nil {
		return genericRemovalTarget{}, fmt.Errorf("resolve managed model cache: %w", err)
	}
	if err := s.requireManagedDirectoryChild(
		ctx,
		filepath.Dir(modelRoot),
		filepath.Base(modelRoot),
		"model",
	); err != nil {
		return genericRemovalTarget{}, err
	}
	revisionPath, err := managedCacheChildPath(modelRoot, inspection.Revision, "revision")
	if err != nil {
		return genericRemovalTarget{}, fmt.Errorf("%w: %v", models.ErrModelCacheUnsafe, err)
	}
	if filepath.Clean(inspection.CachePath) != filepath.Clean(revisionPath) {
		return genericRemovalTarget{}, fmt.Errorf(
			"%w: managed cache revision path does not match its manifest",
			models.ErrModelCacheUnsafe,
		)
	}
	if err := s.requireManagedDirectoryChild(ctx, modelRoot, inspection.Revision, "revision"); err != nil {
		return genericRemovalTarget{}, err
	}
	return genericRemovalTarget{
		modelName:    modelName,
		modelRoot:    modelRoot,
		revision:     inspection.Revision,
		revisionPath: revisionPath,
		sourceKey:    genericSourceIdentity(source),
	}, nil
}

func (s *service) resolveRemovalSource(
	runtime models.RuntimeConfig,
	modelName string,
) (assetSpec, models.SourceMetadata, error) {
	spec, source, err := s.resolveSource(runtime, modelName)
	if errors.Is(err, models.ErrAssetSourceMissing) ||
		errors.Is(err, models.ErrAssetSourceUnsupported) {
		return assetSpec{}, models.SourceMetadata{}, modelCacheNotFound(modelName)
	}
	return spec, source, err
}

func (s *service) removeResolvedModelAssets(
	ctx context.Context,
	request models.RemoveModelAssetsRequest,
	result models.RemoveModelAssetsResult,
	scope models.RuntimeScopeConfig,
	spec assetSpec,
	source models.SourceMetadata,
) (resultOut models.RemoveModelAssetsResult, resultErr error) {
	resultOut = result
	target, err := s.resolveManagedRemovalTarget(ctx, scope, spec, source)
	if err != nil {
		return result, err
	}
	resolve := func() (genericRemovalTarget, error) {
		return s.resolveManagedRemovalTarget(ctx, scope, spec, source)
	}
	return s.removeManagedModelTarget(ctx, request, result, target, resolve)
}

func (s *service) resolveManagedRemovalTarget(
	ctx context.Context,
	scope models.RuntimeScopeConfig,
	spec assetSpec,
	source models.SourceMetadata,
) (genericRemovalTarget, error) {
	modelRoot, err := s.modelCacheRoot(scope.CacheDirectory, spec.modelName)
	if err != nil {
		return genericRemovalTarget{}, fmt.Errorf("resolve managed model cache: %w", err)
	}
	if err := s.requireManagedDirectoryChild(ctx, filepath.Dir(modelRoot), filepath.Base(modelRoot), "model"); err != nil {
		return genericRemovalTarget{}, err
	}
	snapshot, _, err := s.inspectCache(ctx, scope.CacheDirectory, spec, source)
	if err != nil {
		return genericRemovalTarget{}, err
	}
	revision := strings.TrimSpace(snapshot.Revision)
	if revision == "" {
		return genericRemovalTarget{}, modelCacheNotFound(spec.modelName)
	}
	revisionPath, err := managedCacheChildPath(modelRoot, revision, "revision")
	if err != nil {
		return genericRemovalTarget{}, fmt.Errorf("%w: %v", models.ErrModelCacheUnsafe, err)
	}
	if err := s.requireManagedDirectoryChild(ctx, modelRoot, revision, "revision"); err != nil {
		return genericRemovalTarget{}, err
	}
	managedSource, sourceFound := genericSourceForManagedModel(spec.modelName)
	var sourceKey string
	if sourceFound {
		sourceKey = genericSourceIdentity(managedSource)
	}
	return genericRemovalTarget{
		modelName: spec.modelName, modelRoot: modelRoot, revision: revision,
		revisionPath: revisionPath, sourceKey: sourceKey,
	}, nil
}

func (s *service) removeManagedModelTarget(
	ctx context.Context,
	request models.RemoveModelAssetsRequest,
	result models.RemoveModelAssetsResult,
	target genericRemovalTarget,
	resolve func() (genericRemovalTarget, error),
) (resultOut models.RemoveModelAssetsResult, resultErr error) {
	resultOut = result
	var plan reclamation.Plan
	var locks []io.Closer
	if request.ReclaimUnusedCache {
		var err error
		reservation, err := s.lockModelCacheForRemoval(ctx, target, resolve)
		if err != nil {
			return models.RemoveModelAssetsResult{}, err
		}
		target, plan, locks = reservation.target, reservation.plan, reservation.locks
		defer func() {
			resultErr = s.reclamation.CloseLocks(locks, resultErr)
			if resultErr != nil {
				resultOut = models.RemoveModelAssetsResult{}
			}
		}()
	}
	bytesRemoved, err := s.reclamation.MeasureRevisionBytes(ctx, target.revisionPath)
	if errors.Is(err, os.ErrNotExist) {
		return result, modelCacheNotFound(target.modelName)
	}
	if err != nil {
		return result, err
	}
	if err := s.removeManagedTree(ctx, target.revisionPath); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	if err := s.verifyManagedPathRemoved(ctx, target.modelRoot, target.revision); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	s.preparedRuntimeMu.Lock()
	delete(s.preparedRuntime, preparedRuntimeKey(request.Scope, request.Name))
	s.preparedRuntimeMu.Unlock()
	var reclaimed int64
	if request.ReclaimUnusedCache {
		reclaimed, err = s.reclamation.Apply(ctx, plan)
		if err != nil {
			return models.RemoveModelAssetsResult{}, err
		}
	}
	return models.RemoveModelAssetsResult{
		ModelName: target.modelName, Revision: target.revision,
		CachePath: target.revisionPath, BytesRemoved: bytesRemoved,
		ReclaimedCacheBytes: reclaimed, RetainedSharedCacheBytes: plan.Retained,
		Readiness: models.AssetReadinessMissing, Outcome: models.AssetRemovalRemoved,
	}, nil
}

type cacheReclamationReservation struct {
	target genericRemovalTarget
	plan   reclamation.Plan
	locks  []io.Closer
}

func (s *service) readReclamationMetadata(
	ctx context.Context,
	modelRoot, modelName, revision string,
) (cacheMetadata, error) {
	return s.reclamation.ReadManagedMetadata(ctx, modelRoot, modelName, revision)
}

func (s *service) lockModelCacheForRemoval(
	ctx context.Context,
	target genericRemovalTarget,
	resolve func() (genericRemovalTarget, error),
) (cacheReclamationReservation, error) {
	cacheRoot := filepath.Dir(target.modelRoot)
	lock, err := s.reclamation.LockReferences(ctx, cacheRoot)
	if err != nil {
		return cacheReclamationReservation{}, s.reclamation.UncertainReference(target.modelName, err)
	}
	locks := []io.Closer{lock}
	if resolve != nil {
		target, err = resolve()
		if err != nil {
			return cacheReclamationReservation{}, s.reclamation.CloseLocks(locks, err)
		}
	}
	if strings.TrimSpace(target.sourceKey) == "" {
		err := s.reclamation.UncertainReference(target.modelName, nil)
		return cacheReclamationReservation{}, s.reclamation.CloseLocks(locks, err)
	}
	metadata, err := s.readReclamationMetadata(ctx, target.modelRoot, target.modelName, target.revision)
	if err != nil {
		return cacheReclamationReservation{}, s.reclamation.CloseLocks(locks, err)
	}
	plan, candidateLocks, err := s.reclamation.PlanAndLockCandidates(
		ctx, cacheRoot, target.modelRoot, target.modelName, metadata, target.sourceKey,
	)
	locks = append(locks, candidateLocks...)
	if err != nil {
		return cacheReclamationReservation{}, s.reclamation.CloseLocks(locks, err)
	}
	return cacheReclamationReservation{target: target, plan: plan, locks: locks}, nil
}

func modelCacheNotFound(name string) error {
	return fmt.Errorf("%w: %s", models.ErrModelCacheNotFound, canonicalModelName(name))
}

// requireManagedDirectoryChild verifies a direct directory child without
// resolving links. The configured cache root itself is an accepted location;
// model and revision children must be real directories owned by that root.
func (s *service) requireManagedDirectoryChild(
	ctx context.Context,
	parent string,
	child string,
	kind string,
) error {
	if err := assetContextError(ctx); err != nil {
		return err
	}
	child = strings.TrimSpace(child)
	if child == "" || filepath.Base(child) != child || child == "." || child == ".." {
		return fmt.Errorf("%w: managed cache %s path is invalid", models.ErrModelCacheUnsafe, kind)
	}
	entries, err := s.readDirectory(parent)
	if errors.Is(err, os.ErrNotExist) {
		return modelCacheNotFound(child)
	}
	if err != nil {
		return fmt.Errorf("inspect managed cache %s: %w", kind, err)
	}
	for _, entry := range entries {
		if entry != nil && entry.Name() == child {
			return s.validateManagedDirectoryChild(entry, child, kind)
		}
	}
	return modelCacheNotFound(child)
}

func (s *service) validateManagedDirectoryChild(
	entry os.DirEntry,
	child string,
	kind string,
) error {
	if entry.Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: managed cache %s is a symlink", models.ErrModelCacheUnsafe, kind)
	}
	info, err := entry.Info()
	if errors.Is(err, os.ErrNotExist) {
		return modelCacheNotFound(child)
	}
	if err != nil {
		return fmt.Errorf("inspect managed cache %s: %w", kind, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: managed cache %s is not a directory", models.ErrModelCacheUnsafe, kind)
	}
	return nil
}

func (s *service) removeManagedTree(ctx context.Context, directory string) error {
	if err := assetContextError(ctx); err != nil {
		return err
	}
	entries, err := s.readDirectory(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: read managed cache revision: %v", models.ErrModelCacheRemovalFailed, err)
	}
	for _, entry := range entries {
		if err := assetContextError(ctx); err != nil {
			return err
		}
		if entry != nil {
			if err := s.removeManagedEntry(ctx, directory, entry); err != nil {
				return err
			}
		}
	}
	if err := assetContextError(ctx); err != nil {
		return err
	}
	return s.removeManagedPath(directory)
}

func (s *service) removeManagedEntry(
	ctx context.Context,
	directory string,
	entry os.DirEntry,
) error {
	name := entry.Name()
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return fmt.Errorf("%w: invalid managed cache entry %q", models.ErrModelCacheUnsafe, name)
	}
	child := filepath.Join(directory, name)
	if entry.Type()&os.ModeSymlink != 0 {
		return s.removeManagedPath(child)
	}
	info, err := entry.Info()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: inspect managed cache entry: %v", models.ErrModelCacheRemovalFailed, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return s.removeManagedPath(child)
	}
	if info.IsDir() {
		return s.removeManagedTree(ctx, child)
	}
	return s.removeManagedPath(child)
}

func (s *service) removeManagedPath(path string) error {
	if err := s.removePath(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: remove managed cache path: %v", models.ErrModelCacheRemovalFailed, err)
	}
	return nil
}

func (s *service) verifyManagedPathRemoved(ctx context.Context, parent, child string) error {
	if err := assetContextError(ctx); err != nil {
		return err
	}
	entries, err := s.readDirectory(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: verify managed cache removal: %v", models.ErrModelCacheRemovalFailed, err)
	}
	for _, entry := range entries {
		if entry != nil && entry.Name() == child {
			return fmt.Errorf("%w: managed cache revision still exists", models.ErrModelCacheRemovalFailed)
		}
	}
	return nil
}
