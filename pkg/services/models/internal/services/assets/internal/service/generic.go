package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

const (
	assetContentDirectory = ".you-content-addressed"
	assetMetadataName     = ".you-assets.json"
	assetKindModel        = "model"
	assetKindBackend      = "backend"
)

type genericSourceKind string

const (
	genericSourceLocal   genericSourceKind = "LOCAL"
	genericSourceFile    genericSourceKind = "FILE"
	genericSourceHF      genericSourceKind = "HUGGING_FACE"
	genericSourceRelease genericSourceKind = "RELEASE"
)

type genericSource struct {
	kind        genericSourceKind
	modelName   string
	safe        string
	localPath   string
	owner       string
	repository  string
	file        string
	revision    string
	artifactURL string
}

type genericArtifact struct {
	requirement      models.AssetRequirement
	url              string
	localPath        string
	metadataResolved bool
}

type genericCacheResult struct {
	artifacts    []models.AssetArtifact
	paths        []string
	snapshotPath string
	prepared     bool
}

type assetCacheCall struct {
	done   chan struct{}
	result genericCacheResult
	err    error
}

type genericCacheMetadata struct {
	Kind      string                    `json:"kind"`
	Identity  string                    `json:"identity"`
	Source    string                    `json:"source"`
	SourceKey string                    `json:"sourceKey"`
	Artifacts []models.AssetRequirement `json:"artifacts"`
}

type genericCachePath struct {
	artifact models.AssetArtifact
	path     string
}

func shouldPrepareGenericAssets(request models.PrepareModelAssetsRequest) bool {
	if !request.Reference.IsZero() || request.Offline || request.Artifacts != nil ||
		request.BackendArtifacts != nil || !request.BackendReference.IsZero() ||
		strings.TrimSpace(request.Backend) != "" {
		return true
	}
	raw := strings.TrimSpace(request.Name)
	if _, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(raw); ok {
		return true
	}
	return isGenericSourceReference(raw)
}

func isGenericSourceReference(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "hf://") || strings.HasPrefix(lower, "file://") ||
		strings.HasPrefix(lower, "https://") || looksLikeLocalPath(value)
}

func (s *service) prepareGenericAssets(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	if err := request.Validate(); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	if err := assetContextError(ctx); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	preflight, err := s.preflightGenericPreparation(ctx, request)
	if err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	plan := preflight.plan
	expected := make([]models.AssetRequirement, 0, len(plan.modelRequirements)+len(plan.backendRequirements))
	for _, artifact := range plan.modelRequirements {
		expected = append(expected, artifact.requirement)
	}
	for _, artifact := range plan.backendRequirements {
		expected = append(expected, artifact.requirement)
	}
	s.updateActivePull(request.Scope, request.Name, expected, plan.source.revision)

	// Re-inspection under the root's mutation boundary is part of the prepare
	// operation. The backend remains the first content-bearing transaction even
	// when a preflight observed both sources as reachable.
	backendResult, backendErr := s.acquireGenericBackend(ctx, plan, request.Offline)
	if backendErr != nil {
		return genericAssetFailureResult(plan.source, genericCacheResult{}, backendResult), backendErr
	}
	modelResult, err := s.acquireGenericModel(ctx, plan, request.Offline)
	if err != nil {
		return genericAssetFailureResult(plan.source, modelResult, backendResult), err
	}
	if err := s.rememberGenericPreparedRuntime(
		ctx, request, plan, expected, modelResult, backendResult,
	); err != nil {
		return genericAssetFailureResult(plan.source, modelResult, backendResult), err
	}
	return genericAssetResult(plan.source, modelResult, backendResult), nil
}

func (s *service) acquireGenericModel(
	ctx context.Context,
	plan genericPreparationPlan,
	offline bool,
) (genericCacheResult, error) {
	if plan.modelRuntimeCache != nil {
		// Managed-runtime reuse still crosses the asset staging ownership
		// boundary. This preserves access-denied/cancellation behavior for
		// callers that prepare an already-installed model while avoiding any
		// content download or weak content-cache lookup.
		lock, err := s.lockGenericCache(
			ctx, assetKindModel, plan.source, plan.modelRequirements, plan.modelRoots,
		)
		if err != nil {
			return genericCacheResult{}, err
		}
		if err := closeAssetStagingLock(lock, nil); err != nil {
			return genericCacheResult{}, err
		}
		return genericCacheResultFromRuntimeCache(*plan.modelRuntimeCache), nil
	}
	return s.acquireGenericCache(
		ctx, assetKindModel, models.AssetArtifactKindModel, plan.source,
		plan.modelRequirements, plan.modelRoots, offline,
	)
}

func (s *service) rememberGenericPreparedRuntime(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
	plan genericPreparationPlan,
	expected []models.AssetRequirement,
	modelResult genericCacheResult,
	backendResult genericCacheResult,
) error {
	if plan.modelRuntimeCache == nil && modelResult.snapshotPath == "" && backendResult.snapshotPath == "" {
		return nil
	}
	runtimeInspection := scopedassets.RuntimeCacheInspection{}
	if plan.modelRuntimeCache != nil {
		runtimeInspection = *plan.modelRuntimeCache
	} else if modelResult.snapshotPath != "" {
		var err error
		runtimeInspection, err = s.publishGenericRuntimeCache(
			ctx, plan.cacheDirectory, request.Name, plan.source, modelResult,
		)
		if err != nil {
			return err
		}
	}
	if !runtimeInspection.Supported {
		runtimeInspection = scopedassets.RuntimeCacheInspection{
			Supported:          true,
			Installed:          modelResult.snapshotPath != "",
			ManifestPresent:    modelResult.snapshotPath != "",
			ManifestValid:      modelResult.snapshotPath != "",
			ExpectedArtifacts:  append([]models.AssetRequirement(nil), expected...),
			ObservedArtifacts:  append([]models.AssetArtifact(nil), append(modelResult.artifacts, backendResult.artifacts...)...),
			IntegrityVerified:  modelResult.snapshotPath != "",
			Revision:           plan.source.revision,
			CachePath:          modelResult.snapshotPath,
			InstalledFileCount: len(modelResult.artifacts),
		}
	}
	runtimeInspection.BackendRequired = len(plan.backendRequirements) > 0
	runtimeInspection.BackendCachePath = backendResult.snapshotPath
	runtimeInspection.BackendRevision = plan.backendSource.revision
	runtimeInspection.BackendInstalledFiles = len(backendResult.artifacts)
	runtimeInspection.BackendFiles = append([]string(nil), backendResult.paths...)
	s.rememberPreparedRuntime(request.Scope, request.Name, runtimeInspection)
	return nil
}

type genericPreparationPlan struct {
	cacheDirectory      string
	source              genericSource
	modelRequirements   []genericArtifact
	modelRoots          []string
	modelRuntimeCache   *scopedassets.RuntimeCacheInspection
	backendSource       genericSource
	backendRequirements []genericArtifact
	backendRoots        []string
}

func (s *service) genericPreparationPlan(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (genericPreparationPlan, error) {
	scope, err := s.resolveScope(ctx, request.Scope)
	if err != nil {
		return genericPreparationPlan{}, err
	}
	rawReference := strings.TrimSpace(request.Name)
	if !request.Reference.IsZero() {
		rawReference = strings.TrimSpace(request.Reference.NameOrURI)
	}
	source, err := s.resolveGenericSource(ctx, scope, rawReference)
	if err != nil {
		return genericPreparationPlan{}, pullsupport.WrapPullStage(
			models.PullStageSourceResolution, request.Name,
			"resolve model source", rawReference, err,
		)
	}
	source.modelName = request.Name
	modelRequirements, err := s.genericModelRequirements(ctx, source, request.Artifacts)
	if err != nil {
		return genericPreparationPlan{}, err
	}
	modelRoots, err := s.genericCacheRoots(scope, models.AssetArtifactKindModel)
	if err != nil {
		return genericPreparationPlan{}, err
	}
	backendSource, backendRequirements, backendRoots, err := s.genericBackendPlan(
		ctx, scope, source, request,
	)
	if err != nil {
		return genericPreparationPlan{}, err
	}
	backendSource.modelName = request.Name
	return genericPreparationPlan{
		cacheDirectory:      scope.CacheDirectory,
		source:              source,
		modelRequirements:   modelRequirements,
		modelRoots:          modelRoots,
		backendSource:       backendSource,
		backendRequirements: backendRequirements,
		backendRoots:        backendRoots,
	}, nil
}

func (s *service) genericBackendPlan(
	ctx context.Context,
	scope models.RuntimeScopeConfig,
	source genericSource,
	request models.PrepareModelAssetsRequest,
) (genericSource, []genericArtifact, []string, error) {
	requirements := append([]models.AssetRequirement(nil), request.BackendArtifacts...)
	backendSource := source
	if !request.BackendReference.IsZero() {
		var err error
		backendSource, err = s.resolveGenericSource(ctx, scope, request.BackendReference.NameOrURI)
		if err != nil {
			return genericSource{}, nil, nil, pullsupport.WrapPullStage(
				models.PullStageSourceResolution, request.Name,
				"resolve backend source", request.BackendReference.NameOrURI, err,
			)
		}
	}
	if backend := strings.TrimSpace(request.Backend); backend != "" {
		backendSource.safe = "backend://" + backend + "/" + backendSource.safe
	}
	backendRoots, err := s.genericCacheRoots(scope, models.AssetArtifactKindBackend)
	if err != nil {
		return genericSource{}, nil, nil, err
	}
	return backendSource, s.genericArtifactsFromRequirements(backendSource, requirements), backendRoots, nil
}

func (s *service) acquireGenericBackend(
	ctx context.Context,
	plan genericPreparationPlan,
	offline bool,
) (genericCacheResult, error) {
	if len(plan.backendRequirements) == 0 {
		return genericCacheResult{}, nil
	}
	return s.acquireGenericCache(
		ctx, assetKindBackend, models.AssetArtifactKindBackend, plan.backendSource,
		plan.backendRequirements, plan.backendRoots, offline,
	)
}

func genericAssetResult(
	source genericSource,
	modelResult genericCacheResult,
	backendResult genericCacheResult,
) models.PrepareModelAssetsResult {
	artifacts := append([]models.AssetArtifact(nil), modelResult.artifacts...)
	backendArtifacts := append([]models.AssetArtifact(nil), backendResult.artifacts...)
	var totalBytes int64
	for _, artifact := range artifacts {
		totalBytes += artifact.Bytes
	}
	for _, artifact := range backendArtifacts {
		totalBytes += artifact.Bytes
	}
	snapshot := models.AssetSnapshot{
		ModelName:        sourceDisplayName(source),
		Readiness:        models.AssetReadinessAvailable,
		Integrity:        models.AssetIntegrityVerified,
		Source:           sourceMetadata(source),
		Revision:         source.revision,
		Artifacts:        artifacts,
		BackendArtifacts: backendArtifacts,
		TotalBytes:       totalBytes,
	}
	outcome := models.AssetPreparationAlreadyAvailable
	if modelResult.prepared || backendResult.prepared {
		outcome = models.AssetPreparationPrepared
	}
	return models.PrepareModelAssetsResult{Asset: snapshot.Clone(), Outcome: outcome}
}

func genericAssetFailureResult(
	source genericSource,
	modelResult genericCacheResult,
	backendResult genericCacheResult,
) models.PrepareModelAssetsResult {
	result := genericAssetResult(source, modelResult, backendResult)
	result.Asset.Readiness = models.AssetReadinessFailed
	result.Asset.Integrity = models.AssetIntegrityFailed
	return result
}

func combinedOfflineError(first, second error) *models.AssetOfflineError {
	missing := make([]string, 0)
	for _, err := range []error{first, second} {
		var offline *models.AssetOfflineError
		if !errors.As(err, &offline) || offline == nil {
			continue
		}
		missing = append(missing, offline.Missing...)
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return &models.AssetOfflineError{Missing: uniqueStrings(missing)}
}

func (s *service) genericModelRequirements(
	ctx context.Context,
	source genericSource,
	explicit []models.AssetRequirement,
) ([]genericArtifact, error) {
	if len(explicit) > 0 {
		return s.genericArtifactsFromRequirements(source, explicit), nil
	}
	if source.kind == genericSourceLocal || source.kind == genericSourceFile {
		requirements, err := s.localRequirements(source.localPath)
		if err != nil {
			return nil, err
		}
		return s.genericArtifactsFromRequirements(source, requirements), nil
	}
	if source.file != "" {
		return s.genericArtifactsFromRequirements(source, []models.AssetRequirement{{Name: source.file}}), nil
	}
	if err := assetContextError(ctx); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *service) genericArtifactsFromRequirements(
	source genericSource,
	requirements []models.AssetRequirement,
) []genericArtifact {
	localDirectory := false
	if source.kind == genericSourceLocal || source.kind == genericSourceFile {
		if info, err := s.inspectPath(source.localPath); err == nil {
			localDirectory = info.IsDir()
		}
	}
	artifacts := make([]genericArtifact, 0, len(requirements))
	for _, requirement := range requirements {
		artifact := genericArtifact{
			requirement: requirement,
			metadataResolved: source.kind != genericSourceHF ||
				(requirement.Bytes > 0 && strings.TrimSpace(requirement.SHA256) != ""),
		}
		if source.kind == genericSourceLocal || source.kind == genericSourceFile {
			artifact.localPath = source.localPath
			if localDirectory {
				artifact.localPath = filepath.Join(source.localPath, filepath.FromSlash(requirement.Name))
			}
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}
func (s *service) genericCacheRoots(
	scope models.RuntimeScopeConfig,
	kind models.AssetArtifactKind,
) ([]string, error) {
	home, err := s.resolveHome()
	home = strings.TrimSpace(home)
	hasHome := err == nil && home != ""
	youRoot := strings.TrimSpace(scope.CacheDirectory)
	if youRoot == "" && hasHome {
		youRoot = filepath.Join(home, ".agent-factory", "models")
	}
	if kind == models.AssetArtifactKindBackend {
		if youRoot == "" {
			return nil, fmt.Errorf("%w: model cache home is unavailable", models.ErrAssetSourceMissing)
		}
		return []string{filepath.Join(youRoot, "backend-artifacts")}, nil
	}
	roots := make([]string, 0, 4)
	appendRoot := func(root string) {
		root = strings.TrimSpace(root)
		if root == "" {
			return
		}
		for _, existing := range roots {
			if filepath.Clean(existing) == filepath.Clean(root) {
				return
			}
		}
		roots = append(roots, root)
	}
	appendRoot(s.resolveEnvironment("HUGGINGFACE_HUB_CACHE"))
	if hfHome := strings.TrimSpace(s.resolveEnvironment("HF_HOME")); hfHome != "" {
		appendRoot(filepath.Join(hfHome, "hub"))
	}
	if hasHome {
		appendRoot(filepath.Join(home, ".cache", "huggingface", "hub"))
	}
	if youRoot != "" {
		appendRoot(youRoot)
	}
	return roots, nil
}

func genericCandidatePaths(root, kind string, source genericSource, identity, name string) []string {
	paths := []string{filepath.Join(
		root, assetContentDirectory, kind, identity, filepath.FromSlash(name),
	)}
	legacyIdentity := genericIdentityHash(kind, source, nil)
	if identity != legacyIdentity {
		paths = append(paths, filepath.Join(
			root, assetContentDirectory, kind, legacyIdentity, filepath.FromSlash(name),
		))
	}
	if source.kind == genericSourceHF {
		paths = append(paths,
			filepath.Join(root, "models--"+source.owner+"--"+source.repository,
				"snapshots", source.revision, filepath.FromSlash(name)),
			filepath.Join(root, source.owner, source.repository, source.revision,
				filepath.FromSlash(name)),
		)
	}
	return paths
}

func (s *service) discoverContentAddressedRequirements(
	root, kind string,
	source genericSource,
) []models.AssetRequirement {
	base := filepath.Join(root, assetContentDirectory, kind)
	entries, err := s.readDirectory(base)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasSuffix(entry.Name(), ".partial") {
			continue
		}
		if requirements := s.matchContentAddressedRequirements(base, kind, source, entry.Name()); len(requirements) > 0 {
			return requirements
		}
	}
	return nil
}

func (s *service) matchContentAddressedRequirements(
	base, kind string,
	source genericSource,
	entryName string,
) []models.AssetRequirement {
	body, err := s.readFile(filepath.Join(base, entryName, assetMetadataName))
	if err != nil {
		return nil
	}
	var metadata genericCacheMetadata
	if json.Unmarshal(body, &metadata) != nil || !matchesContentAddressedMetadata(metadata, kind, source) {
		return nil
	}
	return append([]models.AssetRequirement(nil), metadata.Artifacts...)
}

func matchesContentAddressedMetadata(metadata genericCacheMetadata, kind string, source genericSource) bool {
	if metadata.Kind != kind || len(metadata.Artifacts) == 0 {
		return false
	}
	if metadata.SourceKey != "" {
		return metadata.SourceKey == genericSourceIdentity(source)
	}
	if metadata.Source != "" {
		return metadata.Source == source.safe
	}
	return strings.HasPrefix(metadata.Identity, kind+"|"+source.safe+"|")
}

func (s *service) discoverSnapshotRequirements(root string) []models.AssetRequirement {
	result := s.walkSnapshotRequirements(root, "")
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (s *service) walkSnapshotRequirements(root, relative string) []models.AssetRequirement {
	entries, err := s.readDirectory(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return nil
	}
	result := make([]models.AssetRequirement, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == assetMetadataName || strings.HasPrefix(name, ".") {
			continue
		}
		next := name
		if relative != "" {
			next = filepath.ToSlash(filepath.Join(filepath.FromSlash(relative), name))
		}
		path := filepath.Join(root, filepath.FromSlash(next))
		info, statErr := s.inspectPath(path)
		if statErr != nil {
			continue
		}
		if info.IsDir() {
			result = append(result, s.walkSnapshotRequirements(root, next)...)
			continue
		}
		result = append(result, models.AssetRequirement{Name: next, Bytes: info.Size()})
	}
	return result
}

func (s *service) localRequirements(path string) ([]models.AssetRequirement, error) {
	info, err := s.inspectPath(path)
	if err != nil {
		return nil, fmt.Errorf("%w: local model source is unavailable", models.ErrAssetSourceMissing)
	}
	if !info.IsDir() {
		return []models.AssetRequirement{{Name: filepath.Base(path), Bytes: info.Size()}}, nil
	}
	return s.walkLocalRequirements(path, "")
}

func (s *service) walkLocalRequirements(root, relative string) ([]models.AssetRequirement, error) {
	entries, err := s.readDirectory(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return nil, fmt.Errorf("%w: local model source is unavailable", models.ErrAssetSourceMissing)
	}
	result := make([]models.AssetRequirement, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." || strings.Contains(name, "\\") {
			continue
		}
		next := name
		if relative != "" {
			next = filepath.ToSlash(filepath.Join(filepath.FromSlash(relative), name))
		}
		path := filepath.Join(root, filepath.FromSlash(next))
		info, statErr := s.inspectPath(path)
		if statErr != nil {
			return nil, fmt.Errorf("%w: local model source is unavailable", models.ErrAssetSourceMissing)
		}
		if info.IsDir() {
			nested, nestedErr := s.walkLocalRequirements(root, next)
			if nestedErr != nil {
				return nil, nestedErr
			}
			result = append(result, nested...)
			continue
		}
		result = append(result, models.AssetRequirement{Name: next, Bytes: info.Size()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s *service) fetchGenericManifest(
	ctx context.Context,
	source genericSource,
) ([]genericArtifact, error) {
	requestURL := strings.TrimRight(s.endpoints.APIBaseURL, "/") + "/models/" +
		source.owner + "/" + source.repository + "?revision=" + url.QueryEscape(source.revision)
	diagnostics := models.PullDiagnostics{
		ModelName:          source.modelName,
		ResolvedRepository: source.owner + "/" + source.repository,
		Revision:           source.revision,
		Operation:          "fetch model manifest",
		RequestURL:         requestURL,
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: model manifest request is invalid", models.ErrSourceFetchFailed),
		)
	}
	response, err := s.doWithRetry(request)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return nil, contextErr
		}
		return nil, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: model manifest fetch failed", models.ErrSourceFetchFailed),
		)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		diagnostics.UpstreamStatusCode = response.StatusCode
		return nil, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: model manifest fetch failed", models.ErrSourceFetchFailed),
		)
	}
	var payload upstreamModel
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		diagnostics.Operation = "decode model manifest"
		return nil, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: model manifest is invalid", models.ErrSourceFetchFailed),
		)
	}
	byName := make(map[string]upstreamSibling, len(payload.Siblings))
	for _, sibling := range payload.Siblings {
		name := filepath.ToSlash(strings.TrimSpace(sibling.Path))
		if name != "" {
			byName[name] = sibling
		}
	}
	names := make([]string, 0, len(byName))
	if source.file != "" {
		names = append(names, source.file)
	} else {
		for name := range byName {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	result := make([]genericArtifact, 0, len(names))
	for _, name := range names {
		sibling, ok := byName[name]
		if !ok {
			missing := diagnostics
			missing.File = name
			missing.Operation = "resolve manifest artifact"
			return nil, pullsupport.WrapPullDiagnostics(
				missing,
				pullsupport.WrapPullStage(
					models.PullStageSourceResolution, source.modelName,
					missing.Operation, name,
					fmt.Errorf("%w: model manifest is missing an artifact", models.ErrModelReferenceUnknown),
				),
			)
		}
		size := sibling.Size
		digest := ""
		if sibling.LFS != nil {
			if sibling.LFS.Size > 0 {
				size = sibling.LFS.Size
			}
			digest = strings.ToLower(strings.TrimSpace(sibling.LFS.OID))
		}
		result = append(result, genericArtifact{
			requirement:      models.AssetRequirement{Name: name, Bytes: size, SHA256: digest},
			metadataResolved: true,
		})
	}
	return result, nil
}

func mergeGenericManifest(
	requested []genericArtifact,
	manifest []genericArtifact,
) ([]genericArtifact, error) {
	byName := make(map[string]genericArtifact, len(manifest))
	for _, artifact := range manifest {
		byName[artifact.requirement.Name] = artifact
	}
	if len(requested) == 0 {
		return manifest, nil
	}
	result := make([]genericArtifact, 0, len(requested))
	for _, artifact := range requested {
		if found, ok := byName[artifact.requirement.Name]; ok {
			if artifact.requirement.Bytes > 0 && found.requirement.Bytes > 0 &&
				artifact.requirement.Bytes != found.requirement.Bytes {
				return nil, fmt.Errorf(
					"%w: asset %q size does not match its immutable manifest",
					models.ErrAssetIntegrityFailed, artifact.requirement.Name,
				)
			}
			if expected := strings.TrimSpace(artifact.requirement.SHA256); expected != "" &&
				strings.TrimSpace(found.requirement.SHA256) != "" &&
				!strings.EqualFold(expected, found.requirement.SHA256) {
				return nil, fmt.Errorf(
					"%w: asset %q digest does not match its immutable manifest",
					models.ErrAssetIntegrityFailed, artifact.requirement.Name,
				)
			}
			if artifact.requirement.Bytes > 0 {
				found.requirement.Bytes = artifact.requirement.Bytes
			}
			if strings.TrimSpace(artifact.requirement.SHA256) != "" {
				found.requirement.SHA256 = strings.ToLower(strings.TrimSpace(artifact.requirement.SHA256))
			}
			result = append(result, found)
			continue
		}
		return nil, fmt.Errorf(
			"%w: model manifest is missing asset %q",
			models.ErrModelReferenceUnknown, artifact.requirement.Name,
		)
	}
	return result, nil
}

func (s *service) resolveGenericSource(
	ctx context.Context,
	scope models.RuntimeScopeConfig,
	raw string,
) (genericSource, error) {
	raw = strings.TrimSpace(raw)
	if !isGenericSourceReference(raw) {
		canonical := strings.ToLower(raw)
		definition, builtIn := (models.BuiltInCatalog{}).ModelDefinitionFor(canonical)
		overlayName, overlay, hasOverlay := genericOverlay(scope.OperatorModels, canonical)
		if hasOverlay {
			if !builtIn {
				definition = models.ModelDefinition{Name: canonical}
			}
			if overlay.Source != nil {
				definition.Source = strings.TrimSpace(*overlay.Source)
			}
			if strings.TrimSpace(definition.Source) == "" {
				return genericSource{}, models.ModelConfigurationFailure{
					ModelName: overlayName, Field: "source", Message: "is required",
				}
			}
			raw = definition.Source
		} else if builtIn {
			raw = definition.Source
		} else {
			return genericSource{}, fmt.Errorf("%w: model name is unknown", models.ErrAssetSourceMissing)
		}
	}
	source, err := parseGenericSource(raw)
	if err != nil {
		return genericSource{}, err
	}
	if source.kind != genericSourceHF || isImmutableGenericRevision(source.revision) {
		return source, nil
	}
	if err := assetContextError(ctx); err != nil {
		return genericSource{}, err
	}
	resolved, err := s.resolveRevision(ctx, source.safe)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return genericSource{}, contextErr
		}
		return genericSource{}, genericRevisionFailure()
	}
	if !isImmutableGenericRevision(resolved) {
		return genericSource{}, genericRevisionFailure()
	}
	source.revision = strings.TrimSpace(resolved)
	source.safe = genericHFSafeReference(source)
	return source, nil
}

func genericOverlay(
	overlays map[string]models.ModelOverlay,
	canonical string,
) (string, models.ModelOverlay, bool) {
	names := make([]string, 0, 1)
	for name := range overlays {
		if strings.ToLower(strings.TrimSpace(name)) == canonical {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", models.ModelOverlay{}, false
	}
	sort.Strings(names)
	return names[0], overlays[names[0]].Clone(), true
}
