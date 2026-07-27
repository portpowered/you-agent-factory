package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

const metadataFileName = ".managed-cache.json"

type service struct {
	scopes        runtimescopes.Service
	platform      models.AssetHostPlatform
	client        models.AssetHTTPDoer
	endpoints     models.RuntimeAssetEndpoints
	makeDirectory models.AssetMakeDirectories
	inspectPath   models.AssetInspectPath
	resolveHome   models.AssetResolveHomeDirectory
	writeFile     models.AssetWriteFile
	renamePath    models.AssetRenamePath
	removePath    models.AssetRemovePath
	readFile      models.AssetReadFile
	readDirectory models.AssetReadDirectory
	createFile    models.AssetCreateFile
	openFile      models.AssetOpenFile
}

type assetSpec struct {
	modelName         string
	repository        string
	requiredArtifacts []string
	allowedPlatforms  map[string]struct{}
}

type cacheMetadata struct {
	ModelName string         `json:"modelName"`
	Revision  string         `json:"revision"`
	Files     []metadataFile `json:"files"`
}

type metadataFile struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

var _ assets.Service = (*service)(nil)

// New constructs an inert scoped asset inspector.
func New(
	scopes runtimescopes.Service,
	platform models.AssetHostPlatform,
	client models.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints,
	makeDirectory models.AssetMakeDirectories,
	inspectPath models.AssetInspectPath,
	resolveHome models.AssetResolveHomeDirectory,
	writeFile models.AssetWriteFile,
	renamePath models.AssetRenamePath,
	removePath models.AssetRemovePath,
	readFile models.AssetReadFile,
	readDirectory models.AssetReadDirectory,
	createFile models.AssetCreateFile,
	openFile models.AssetOpenFile,
) assets.Service {
	return &service{
		scopes:        scopes,
		platform:      platform,
		client:        client,
		endpoints:     endpoints,
		makeDirectory: makeDirectory,
		inspectPath:   inspectPath,
		resolveHome:   resolveHome,
		writeFile:     writeFile,
		renamePath:    renamePath,
		removePath:    removePath,
		readFile:      readFile,
		readDirectory: readDirectory,
		createFile:    createFile,
		openFile:      openFile,
	}
}

func (s *service) InspectModelAssets(
	ctx context.Context,
	request models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	if err := request.Validate(); err != nil {
		return models.InspectModelAssetsResult{}, err
	}
	scope, err := s.resolveScope(ctx, request.Scope)
	if err != nil {
		return models.InspectModelAssetsResult{}, err
	}
	spec, source, err := s.resolveSource(scope.Runtime, request.Name)
	if err != nil {
		return models.InspectModelAssetsResult{}, err
	}
	if err := assetContextError(ctx); err != nil {
		return models.InspectModelAssetsResult{}, err
	}

	var snapshot models.AssetSnapshot
	var available bool
	if request.VerifyIntegrity {
		snapshot, available, err = s.inspectVerifiedCache(
			ctx, scope.CacheDirectory, spec, source,
		)
	} else {
		snapshot, available, err = s.inspectCache(
			ctx, scope.CacheDirectory, spec, source,
		)
	}
	result := models.InspectModelAssetsResult{Asset: snapshot.Clone()}
	if err != nil {
		return result, err
	}
	if !available {
		return result, fmt.Errorf("%w: %s", models.ErrAssetUnavailable, spec.modelName)
	}
	return result, nil
}

func (s *service) resolveScope(
	ctx context.Context,
	ref models.RuntimeScopeRef,
) (models.RuntimeScopeConfig, error) {
	if err := assetContextError(ctx); err != nil {
		return models.RuntimeScopeConfig{}, err
	}
	if ref.IsZero() {
		return models.RuntimeScopeConfig{}, models.ErrRuntimeScopeInvalid
	}
	if s == nil || s.scopes == nil {
		return models.RuntimeScopeConfig{}, models.ErrUnavailable
	}
	binding, err := s.scopes.Resolve(runtimescopes.Reference(ref.String()))
	if err != nil {
		return models.RuntimeScopeConfig{}, scopeError(err)
	}
	if binding.RuntimeConfig == nil {
		return models.RuntimeScopeConfig{}, models.ErrUnavailable
	}
	runtimeConfig := binding.RuntimeConfig()
	if runtimeConfig == nil {
		return models.RuntimeScopeConfig{}, models.ErrUnavailable
	}
	return models.RuntimeScopeConfig{
		CacheDirectory: binding.CacheDirectory,
		Runtime:        *runtimeConfig,
	}.Clone(), nil
}

func (s *service) resolveSource(
	config models.RuntimeConfig,
	modelName string,
) (assetSpec, models.SourceMetadata, error) {
	spec, ok := supportedAssetSpecs()[canonicalModelName(modelName)]
	if !ok || !s.supportsPlatform(spec) {
		return assetSpec{}, models.SourceMetadata{}, fmt.Errorf(
			"%w: %s", models.ErrAssetSourceUnsupported, modelName,
		)
	}
	resource := modelResource(config.Resources, modelName)
	if resource == nil {
		return assetSpec{}, models.SourceMetadata{}, fmt.Errorf(
			"%w: %s", models.ErrAssetSourceMissing, modelName,
		)
	}

	provider := strings.ToUpper(strings.TrimSpace(resource.Provider))
	switch provider {
	case "", "HUGGINGFACE", "UPSTREAM_REPOSITORY":
		provider = "UPSTREAM_REPOSITORY"
	case "MODELSCOPE", "MANAGED_MIRROR":
		provider = "MANAGED_MIRROR"
	default:
		return assetSpec{}, models.SourceMetadata{}, fmt.Errorf(
			"%w: provider %q", models.ErrAssetSourceUnsupported, resource.Provider,
		)
	}
	return spec, models.SourceMetadata{
		Provider:  provider,
		Reference: spec.repository,
	}, nil
}

func (s *service) supportsPlatform(spec assetSpec) bool {
	if s == nil {
		return false
	}
	key := strings.TrimSpace(s.platform.OperatingSystem) + "/" +
		strings.TrimSpace(s.platform.Architecture)
	_, ok := spec.allowedPlatforms[key]
	return ok
}

func (s *service) inspectCache(
	ctx context.Context,
	cacheDirectory string,
	spec assetSpec,
	source models.SourceMetadata,
) (models.AssetSnapshot, bool, error) {
	root, err := s.modelCacheRoot(cacheDirectory, spec.modelName)
	if err != nil {
		return missingSnapshot(spec.modelName, source), false, err
	}
	metadata, found, err := s.readMetadata(ctx, filepath.Join(root, metadataFileName))
	if err != nil {
		return missingSnapshot(spec.modelName, source), false, err
	}
	if found {
		return s.inspectRevision(ctx, root, spec, source, metadata)
	}
	return s.discoverRevision(ctx, root, spec, source)
}

func (s *service) modelCacheRoot(cacheDirectory string, modelName string) (string, error) {
	root := strings.TrimSpace(cacheDirectory)
	if root == "" {
		home, err := s.resolveHome()
		if err != nil {
			return "", fmt.Errorf("resolve managed model cache directory: %w", err)
		}
		root = filepath.Join(home, ".agent-factory", "models")
	}
	return filepath.Join(root, modelName), nil
}

func (s *service) readMetadata(
	ctx context.Context,
	path string,
) (cacheMetadata, bool, error) {
	if err := assetContextError(ctx); err != nil {
		return cacheMetadata{}, false, err
	}
	body, err := s.readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cacheMetadata{}, false, nil
	}
	if err != nil {
		return cacheMetadata{}, false, fmt.Errorf("read managed cache metadata: %w", err)
	}
	var metadata cacheMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return cacheMetadata{}, false, fmt.Errorf(
			"%w: decode managed cache metadata: %v",
			models.ErrAssetUnavailable,
			err,
		)
	}
	if strings.TrimSpace(metadata.Revision) == "" {
		return cacheMetadata{}, false, nil
	}
	return metadata, true, nil
}

func (s *service) inspectRevision(
	ctx context.Context,
	root string,
	spec assetSpec,
	source models.SourceMetadata,
	metadata cacheMetadata,
) (models.AssetSnapshot, bool, error) {
	revision := strings.TrimSpace(metadata.Revision)
	metadataByName := make(map[string]metadataFile, len(metadata.Files))
	for _, file := range metadata.Files {
		metadataByName[filepath.ToSlash(strings.TrimSpace(file.Path))] = file
	}
	artifacts := make([]models.AssetArtifact, 0, len(spec.requiredArtifacts))
	for _, name := range spec.requiredArtifacts {
		if err := assetContextError(ctx); err != nil {
			return missingSnapshot(spec.modelName, source), false, err
		}
		info, err := s.inspectPath(filepath.Join(root, revision, filepath.FromSlash(name)))
		if errors.Is(err, os.ErrNotExist) || (err == nil && info.IsDir()) {
			return unavailableSnapshot(spec.modelName, source, revision, artifacts), false, nil
		}
		if err != nil {
			return unavailableSnapshot(spec.modelName, source, revision, artifacts), false, fmt.Errorf(
				"inspect managed cache artifact %q: %w", name, err,
			)
		}
		metadataFile := metadataByName[name]
		artifacts = append(artifacts, models.AssetArtifact{
			Name:   name,
			Bytes:  info.Size(),
			SHA256: strings.ToLower(strings.TrimSpace(metadataFile.SHA256)),
		})
	}
	return availableSnapshot(spec.modelName, source, revision, artifacts), true, nil
}

func (s *service) discoverRevision(
	ctx context.Context,
	root string,
	spec assetSpec,
	source models.SourceMetadata,
) (models.AssetSnapshot, bool, error) {
	if err := assetContextError(ctx); err != nil {
		return missingSnapshot(spec.modelName, source), false, err
	}
	entries, err := s.readDirectory(root)
	if errors.Is(err, os.ErrNotExist) {
		return missingSnapshot(spec.modelName, source), false, nil
	}
	if err != nil {
		return missingSnapshot(spec.modelName, source), false, fmt.Errorf(
			"read managed model cache: %w", err,
		)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metadata := cacheMetadata{Revision: entry.Name()}
		snapshot, available, inspectErr := s.inspectRevision(ctx, root, spec, source, metadata)
		if inspectErr != nil {
			return snapshot, false, inspectErr
		}
		if available {
			return snapshot, true, nil
		}
	}
	return missingSnapshot(spec.modelName, source), false, nil
}

func availableSnapshot(
	modelName string,
	source models.SourceMetadata,
	revision string,
	artifacts []models.AssetArtifact,
) models.AssetSnapshot {
	source.Revision = revision
	var totalBytes int64
	for _, artifact := range artifacts {
		totalBytes += artifact.Bytes
	}
	return models.AssetSnapshot{
		ModelName:  modelName,
		Readiness:  models.AssetReadinessAvailable,
		Integrity:  models.AssetIntegrityUnknown,
		Source:     source,
		Revision:   revision,
		Artifacts:  append([]models.AssetArtifact(nil), artifacts...),
		TotalBytes: totalBytes,
	}
}

func unavailableSnapshot(
	modelName string,
	source models.SourceMetadata,
	revision string,
	artifacts []models.AssetArtifact,
) models.AssetSnapshot {
	source.Revision = revision
	var totalBytes int64
	for _, artifact := range artifacts {
		totalBytes += artifact.Bytes
	}
	return models.AssetSnapshot{
		ModelName:  modelName,
		Readiness:  models.AssetReadinessMissing,
		Integrity:  models.AssetIntegrityUnknown,
		Source:     source,
		Revision:   revision,
		Artifacts:  append([]models.AssetArtifact(nil), artifacts...),
		TotalBytes: totalBytes,
	}
}

func missingSnapshot(modelName string, source models.SourceMetadata) models.AssetSnapshot {
	return unavailableSnapshot(modelName, source, "", nil)
}

func modelResource(resources []models.RuntimeResource, modelName string) *models.RuntimeResource {
	key := canonicalModelName(modelName)
	for _, resource := range resources {
		if strings.TrimSpace(resource.Type) != models.RuntimeResourceTypeModel ||
			canonicalModelName(resource.Model) != key {
			continue
		}
		copied := resource
		return &copied
	}
	return nil
}

func canonicalModelName(name string) string {
	return strings.ToUpper(strings.TrimSpace(name))
}

func supportedAssetSpecs() map[string]assetSpec {
	return map[string]assetSpec{
		"OMNIVOICE_Q4_K_M": {
			modelName:  "OMNIVOICE_Q4_K_M",
			repository: "Serveurperso/OmniVoice-GGUF",
			requiredArtifacts: []string{
				"omnivoice-base-Q4_K_M.gguf",
				"omnivoice-tokenizer-Q4_K_M.gguf",
			},
			allowedPlatforms: map[string]struct{}{
				"darwin/arm64":  {},
				"darwin/amd64":  {},
				"linux/amd64":   {},
				"linux/arm64":   {},
				"windows/amd64": {},
				"windows/arm64": {},
			},
		},
	}
}

func scopeError(err error) error {
	switch {
	case errors.Is(err, runtimescopes.ErrScopeForeign):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeForeign, err)
	case errors.Is(err, runtimescopes.ErrScopeClosed):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeClosed, err)
	case errors.Is(err, runtimescopes.ErrScopeUnknown):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeStale, err)
	default:
		return models.ErrUnavailable
	}
}

func assetContextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", models.ErrAssetCancelled, ctx.Err())
}
