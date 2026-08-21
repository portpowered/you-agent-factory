package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
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
	safe        string
	localPath   string
	owner       string
	repository  string
	file        string
	revision    string
	artifactURL string
}

type genericArtifact struct {
	requirement models.AssetRequirement
	url         string
	localPath   string
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
	plan, err := s.genericPreparationPlan(ctx, request)
	if err != nil {
		return models.PrepareModelAssetsResult{}, err
	}

	modelResult, modelErr := s.acquireGenericCache(
		ctx, assetKindModel, models.AssetArtifactKindModel, plan.source,
		plan.modelRequirements, plan.modelRoots, request.Offline,
	)
	backendResult, backendErr := s.acquireGenericBackend(ctx, plan, request.Offline)
	if err := genericPreparationError(modelErr, backendErr); err != nil {
		return genericAssetFailureResult(plan.source, modelResult, backendResult), err
	}
	if modelResult.snapshotPath != "" || backendResult.snapshotPath != "" {
		s.rememberPreparedRuntime(request.Scope, request.Name, scopedassets.RuntimeCacheInspection{
			Supported:             true,
			Installed:             modelResult.snapshotPath != "",
			Revision:              plan.source.revision,
			CachePath:             modelResult.snapshotPath,
			InstalledFileCount:    len(modelResult.artifacts),
			BackendRequired:       len(plan.backendRequirements) > 0,
			BackendCachePath:      backendResult.snapshotPath,
			BackendRevision:       plan.backendSource.revision,
			BackendInstalledFiles: len(backendResult.artifacts),
		})
	}
	return genericAssetResult(plan.source, modelResult, backendResult), nil
}

type genericPreparationPlan struct {
	source              genericSource
	modelRequirements   []genericArtifact
	modelRoots          []string
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
		return genericPreparationPlan{}, err
	}
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
	return genericPreparationPlan{
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
			return genericSource{}, nil, nil, err
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

func genericPreparationError(modelErr, backendErr error) error {
	if modelErr == nil && backendErr == nil {
		return nil
	}
	if offlineErr := combinedOfflineError(modelErr, backendErr); offlineErr != nil {
		return offlineErr
	}
	if modelErr != nil {
		return modelErr
	}
	return backendErr
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
		artifact := genericArtifact{requirement: requirement}
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

func (s *service) discoverCachedRequirements(
	kind string,
	source genericSource,
	roots []string,
) []models.AssetRequirement {
	for _, root := range roots {
		if requirements := s.discoverContentAddressedRequirements(root, kind, source); len(requirements) > 0 {
			return requirements
		}
		if source.kind != genericSourceHF {
			continue
		}
		snapshot := filepath.Join(root, "models--"+source.owner+"--"+source.repository,
			"snapshots", source.revision)
		if requirements := s.discoverSnapshotRequirements(snapshot); len(requirements) > 0 {
			return requirements
		}
	}
	return nil
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: model manifest request is invalid", models.ErrSourceFetchFailed)
	}
	response, err := s.doWithRetry(request)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("%w: model manifest fetch failed", models.ErrSourceFetchFailed)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: model manifest fetch failed", models.ErrSourceFetchFailed)
	}
	var payload upstreamModel
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: model manifest is invalid", models.ErrSourceFetchFailed)
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
			return nil, fmt.Errorf("%w: model manifest is missing an artifact", models.ErrSourceFetchFailed)
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
			requirement: models.AssetRequirement{Name: name, Bytes: size, SHA256: digest},
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
			models.ErrSourceFetchFailed, artifact.requirement.Name,
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

func parseGenericSource(raw string) (genericSource, error) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(lower, "hf://"):
		return parseGenericHFSource(raw)
	case strings.HasPrefix(lower, "file://"):
		return parseGenericFileSource(raw)
	case strings.HasPrefix(lower, "https://"):
		return parseGenericReleaseSource(raw)
	case strings.Contains(raw, "://"):
		return genericSource{}, models.ErrAssetSourceUnsupported
	case looksLikeLocalPath(raw):
		return genericSource{kind: genericSourceLocal, safe: "local://path", localPath: raw}, nil
	default:
		return genericSource{}, models.ErrAssetSourceUnsupported
	}
}

func parseGenericReleaseSource(raw string) (genericSource, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		!strings.Contains(path.Clean(parsed.Path), "/releases/download/") {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\\\x00") {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	checksum := sha256.Sum256([]byte(raw))
	return genericSource{
		kind: genericSourceRelease, safe: "release://" + hex.EncodeToString(checksum[:]),
		artifactURL: raw, revision: hex.EncodeToString(checksum[:]),
	}, nil
}

func parseGenericHFSource(raw string) (genericSource, error) {
	rest := strings.TrimSpace(raw[len("hf://"):])
	if rest == "" || strings.ContainsAny(rest, "\x00?#\\") {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	at := strings.LastIndex(rest, "@")
	base, revision := rest, ""
	if at >= 0 {
		base, revision = rest[:at], rest[at+1:]
	}
	parts := strings.Split(base, "/")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || part == "." || part == ".." || strings.ContainsAny(part, " @\t\r\n") {
			return genericSource{}, models.ErrModelReferenceInvalid
		}
	}
	if strings.ContainsAny(revision, "\x00/@\\?# \t\r\n") {
		return genericSource{}, models.ErrModelRevisionUnresolved
	}
	file := strings.Join(parts[2:], "/")
	safe := "hf://" + parts[0] + "/" + parts[1]
	if file != "" {
		safe += "/" + file
	}
	if revision != "" {
		safe += "@" + revision
	}
	return genericSource{
		kind: genericSourceHF, safe: safe, owner: parts[0], repository: parts[1],
		file: file, revision: revision,
	}, nil
}

func genericHFSafeReference(source genericSource) string {
	safe := "hf://" + source.owner + "/" + source.repository
	if source.file != "" {
		safe += "/" + source.file
	}
	return safe + "@" + source.revision
}

func genericRevisionFailure() error {
	return &models.InvocationFailure{
		Class:   models.InvocationFailureClassRevisionResolution,
		Message: "model source revision could not be resolved to an immutable commit",
		Cause:   models.ErrModelRevisionUnresolved,
	}
}

func parseGenericFileSource(raw string) (genericSource, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !validGenericFileURL(parsed) {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	localPath, err := genericFileURLPath(parsed)
	if err != nil {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	if localPath == "" {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	return genericSource{
		kind: genericSourceFile, safe: "file://local", localPath: filepath.FromSlash(localPath),
	}, nil
}

func validGenericFileURL(parsed *url.URL) bool {
	return parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func genericFileURLPath(parsed *url.URL) (string, error) {
	localPath, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		if len(parsed.Host) != 2 || parsed.Host[1] != ':' || !isASCIIAlphaByte(parsed.Host[0]) {
			return "", models.ErrModelReferenceInvalid
		}
		localPath = parsed.Host + localPath
	}
	if len(localPath) >= 3 && localPath[0] == '/' && localPath[2] == ':' && isASCIIAlphaByte(localPath[1]) {
		localPath = localPath[1:]
	}
	return localPath, nil
}

func isImmutableGenericRevision(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (s *service) genericAssetURL(source genericSource, name string) string {
	if source.kind == genericSourceRelease {
		return source.artifactURL
	}
	return strings.TrimRight(s.endpoints.BaseURL, "/") + "/" + source.owner + "/" +
		source.repository + "/resolve/" + source.revision + "/" + url.PathEscape(name) + "?download=true"
}

func (s *service) addGenericURLs(source genericSource, artifacts []genericArtifact) {
	if source.kind != genericSourceHF {
		return
	}
	for index := range artifacts {
		if artifacts[index].url == "" {
			artifacts[index].url = s.genericAssetURL(source, artifacts[index].requirement.Name)
		}
	}
}

func sourceDisplayName(source genericSource) string {
	if source.kind == genericSourceHF {
		return source.owner + "/" + source.repository
	}
	return "local-model"
}

func sourceMetadata(source genericSource) models.SourceMetadata {
	if source.kind == genericSourceHF {
		return models.SourceMetadata{Provider: "HUGGINGFACE", Reference: source.owner + "/" + source.repository, Revision: source.revision}
	}
	if source.kind == genericSourceRelease {
		return models.SourceMetadata{Provider: "PINNED_BACKEND", Reference: "pinned-backend", Revision: source.revision}
	}
	return models.SourceMetadata{Provider: "LOCAL", Reference: source.safe}
}

func genericCacheKey(kind string, source genericSource, artifacts []genericArtifact) string {
	names := genericArtifactIdentityNames(artifacts)
	sort.Strings(names)
	return kind + "|" + genericSourceIdentity(source) + "|" + strings.Join(names, ",")
}

func genericSourceIdentity(source genericSource) string {
	if source.kind != genericSourceLocal && source.kind != genericSourceFile {
		return source.safe
	}
	checksum := sha256.Sum256([]byte(source.localPath))
	return source.safe + "|" + hex.EncodeToString(checksum[:])
}

func genericArtifactIdentityNames(artifacts []genericArtifact) []string {
	names := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		requirement := artifact.requirement
		names = append(names, fmt.Sprintf(
			"%s:%d:%s",
			requirement.Name,
			requirement.Bytes,
			strings.ToLower(strings.TrimSpace(requirement.SHA256)),
		))
	}
	return names
}

func genericArtifactIdentityHash(kind string, source genericSource, artifacts []genericArtifact) string {
	return genericIdentityHash(kind, source, genericArtifactIdentityNames(artifacts))
}

func genericIdentityHash(kind string, source genericSource, names []string) string {
	identity := kind + "|" + genericSourceIdentity(source)
	if len(names) > 0 {
		cloned := append([]string(nil), names...)
		sort.Strings(cloned)
		identity += "|" + strings.Join(cloned, ",")
	}
	hash := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(hash[:])
}

func missingArtifactNames(artifacts []genericArtifact) []string {
	names := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		names = append(names, artifact.requirement.Name)
	}
	sort.Strings(names)
	return uniqueStrings(names)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func isASCIIAlphaByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func looksLikeLocalPath(value string) bool {
	return filepath.IsAbs(value) || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") ||
		strings.HasPrefix(value, ".\\") || strings.HasPrefix(value, "..\\") ||
		strings.ContainsAny(value, `/\\`) || filepath.Ext(value) != ""
}
