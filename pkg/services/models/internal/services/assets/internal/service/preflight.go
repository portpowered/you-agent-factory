package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
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
	state, err := s.preflightGenericPreparation(ctx, request)
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
			plan.backendRoots, request.Offline,
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
			plan.modelRoots, request.Offline,
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
	if source.kind == genericSourceRelease {
		resolved, missing, err = s.headGenericArtifacts(ctx, source, resolved, cached, missing)
		if err != nil {
			return genericPreflightFacts{artifacts: resolved, missing: missing}, err
		}
	}
	return genericPreflightFacts{artifacts: resolved, missing: missing}, nil
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
		markGenericArtifactsResolved(discoveredArtifacts)
		if len(resolved) == 0 {
			resolved = discoveredArtifacts
		} else {
			merged, err := mergeGenericManifest(resolved, discoveredArtifacts)
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
		if contentLength < 0 {
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
