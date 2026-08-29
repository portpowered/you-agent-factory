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
	"sync"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

const metadataFileName = ".managed-cache.json"

const maxManagedCacheBytes = int64(1<<63 - 1)

type service struct {
	scopes             runtimescopes.Service
	platform           models.AssetHostPlatform
	client             modelseffects.AssetHTTPDoer
	endpoints          models.RuntimeAssetEndpoints
	makeDirectory      modelseffects.AssetMakeDirectories
	inspectPath        modelseffects.AssetInspectPath
	resolveHome        modelseffects.AssetResolveHomeDirectory
	writeFile          modelseffects.AssetWriteFile
	renamePath         modelseffects.AssetRenamePath
	removePath         modelseffects.AssetRemovePath
	readFile           modelseffects.AssetReadFile
	readDirectory      modelseffects.AssetReadDirectory
	createFile         modelseffects.AssetCreateFile
	openFile           modelseffects.AssetOpenFile
	resolveEnvironment modelseffects.AssetResolveEnvironment
	resolveRevision    func(context.Context, string) (string, error)
	coordination       modelseffects.AssetStagingCoordination

	cacheMu  sync.Mutex
	inflight map[string]*assetCacheCall
	// cacheJoinObserver is an optional concurrency observer used by tests to
	// coordinate followers at the shared-cache boundary.
	cacheJoinObserver func()

	pullStateMu sync.RWMutex
	activePulls map[string]activePullState
	pullFailure map[string]string

	preparedRuntimeMu sync.RWMutex
	preparedRuntime   map[string]assets.RuntimeCacheInspection
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

type activePullState struct {
	expected []models.AssetRequirement
	revision string
}

var _ assets.Service = (*service)(nil)

// New constructs an inert scoped asset inspector.
func New(
	scopes runtimescopes.Service,
	platform models.AssetHostPlatform,
	client modelseffects.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints,
	makeDirectory modelseffects.AssetMakeDirectories,
	inspectPath modelseffects.AssetInspectPath,
	resolveHome modelseffects.AssetResolveHomeDirectory,
	writeFile modelseffects.AssetWriteFile,
	renamePath modelseffects.AssetRenamePath,
	removePath modelseffects.AssetRemovePath,
	readFile modelseffects.AssetReadFile,
	readDirectory modelseffects.AssetReadDirectory,
	createFile modelseffects.AssetCreateFile,
	openFile modelseffects.AssetOpenFile,
	options ...assets.ConstructionOptions,
) assets.Service {
	resolveEnvironment := modelseffects.AssetResolveEnvironment(func(string) string { return "" })
	var resolveRevision func(context.Context, string) (string, error)
	var coordination modelseffects.AssetStagingCoordination
	if len(options) > 0 {
		if options[0].ResolveEnvironment != nil {
			resolveEnvironment = options[0].ResolveEnvironment
		}
		resolveRevision = options[0].ResolveRevision
		coordination = options[0].Coordination
	}
	if resolveRevision == nil {
		resolveRevision = func(context.Context, string) (string, error) {
			return "", models.ErrModelRevisionUnresolved
		}
	}
	return &service{
		scopes:             scopes,
		platform:           platform,
		client:             client,
		endpoints:          endpoints,
		makeDirectory:      makeDirectory,
		inspectPath:        inspectPath,
		resolveHome:        resolveHome,
		writeFile:          writeFile,
		renamePath:         renamePath,
		removePath:         removePath,
		readFile:           readFile,
		readDirectory:      readDirectory,
		createFile:         createFile,
		openFile:           openFile,
		resolveEnvironment: resolveEnvironment,
		resolveRevision:    resolveRevision,
		coordination:       coordination,
		inflight:           make(map[string]*assetCacheCall),
		activePulls:        make(map[string]activePullState),
		pullFailure:        make(map[string]string),
		preparedRuntime:    make(map[string]assets.RuntimeCacheInspection),
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

func (s *service) ResolveRuntimeCache(
	ctx context.Context,
	request models.InspectModelAssetsRequest,
) (assets.RuntimeCacheLayout, error) {
	if err := request.Validate(); err != nil {
		return assets.RuntimeCacheLayout{}, err
	}
	scope, err := s.resolveScope(ctx, request.Scope)
	if err != nil {
		return assets.RuntimeCacheLayout{}, err
	}
	spec, source, err := s.resolveSource(scope.Runtime, request.Name)
	if errors.Is(err, models.ErrAssetSourceUnsupported) {
		return s.resolveGenericRuntimeCache(ctx, scope, request.Name)
	}
	if err != nil {
		return assets.RuntimeCacheLayout{}, err
	}
	snapshot, available, err := s.inspectCache(ctx, scope.CacheDirectory, spec, source)
	if err != nil {
		return assets.RuntimeCacheLayout{}, err
	}
	if !available {
		return assets.RuntimeCacheLayout{}, fmt.Errorf(
			"%w: required assets missing for %s", models.ErrNotAvailable, spec.modelName,
		)
	}
	root, err := s.modelCacheRoot(scope.CacheDirectory, spec.modelName)
	if err != nil {
		return assets.RuntimeCacheLayout{}, err
	}
	cachePath, err := managedCacheChildPath(root, snapshot.Revision, "revision")
	if err != nil {
		return assets.RuntimeCacheLayout{}, err
	}
	files := make([]string, 0, len(snapshot.Artifacts))
	for _, artifact := range snapshot.Artifacts {
		files = append(files, filepath.Join(cachePath, filepath.FromSlash(artifact.Name)))
	}
	return assets.RuntimeCacheLayout{
		ModelName: snapshot.ModelName,
		CachePath: cachePath,
		Revision:  snapshot.Revision,
		Files:     files,
	}, nil
}

func (s *service) InspectRuntimeCache(
	ctx context.Context,
	request models.InspectModelAssetsRequest,
) (assets.RuntimeCacheInspection, error) {
	if err := request.Validate(); err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	scope, err := s.resolveScope(ctx, request.Scope)
	if err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	active, isActive, pullFailure := s.activePullStateFor(request.Scope, request.Name)
	configured, handled, err := s.inspectConfiguredRuntimeCache(
		ctx, scope, request.Name, active, isActive, pullFailure,
	)
	if err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	if handled {
		return configured, nil
	}

	genericSource, sourceErr := s.resolveGenericSource(ctx, scope, request.Name)
	genericInspection, persisted, inspectErr := s.inspectGenericRuntimeCache(
		ctx, scope.CacheDirectory, request.Name, genericSource,
	)
	if inspectErr != nil {
		return assets.RuntimeCacheInspection{}, inspectErr
	}
	if sourceErr == nil || persisted {
		if prepared, ok := s.preparedRuntimeInspection(request.Scope, request.Name); ok {
			genericInspection = mergeGenericRuntimeBackendFacts(genericInspection, prepared)
		}
		return s.applyActivePullFacts(genericInspection, active, isActive, pullFailure), nil
	}
	if prepared, ok := s.preparedRuntimeInspection(request.Scope, request.Name); ok {
		prepared = s.applyActivePullFacts(prepared, active, isActive, pullFailure)
		if prepared.Installed && strings.TrimSpace(prepared.CachePath) != "" {
			cacheBytes, measureErr := s.measureRevisionBytes(ctx, prepared.CachePath)
			if measureErr != nil {
				return assets.RuntimeCacheInspection{}, measureErr
			}
			prepared.CacheBytes = cacheBytes
		}
		return prepared, nil
	}
	return assets.RuntimeCacheInspection{}, nil
}

func (s *service) resolveRuntimeCacheSource(
	config models.RuntimeConfig,
	modelName string,
) (assetSpec, models.SourceMetadata, bool, error) {
	spec, source, err := s.resolveSource(config, modelName)
	if errors.Is(err, models.ErrAssetSourceUnsupported) {
		return assetSpec{}, models.SourceMetadata{}, false, nil
	}
	if err != nil {
		return assetSpec{}, models.SourceMetadata{}, false, err
	}
	return spec, source, true, nil
}

func (s *service) invalidRuntimeCacheInspection(
	ctx context.Context,
	expected []models.AssetRequirement,
	active activePullState,
	isActive bool,
	pullFailure string,
) (assets.RuntimeCacheInspection, error) {
	if contextErr := assetContextError(ctx); contextErr != nil {
		return assets.RuntimeCacheInspection{}, contextErr
	}
	return s.applyActivePullFacts(assets.RuntimeCacheInspection{
		Supported:         true,
		ManifestPresent:   true,
		ExpectedArtifacts: expected,
		FailureReason:     "managed cache manifest is invalid",
	}, active, isActive, pullFailure), nil
}

func (s *service) inspectRuntimeCacheFiles(
	ctx context.Context,
	cacheDirectory string,
	spec assetSpec,
	source models.SourceMetadata,
	expected []models.AssetRequirement,
	cacheRoot string,
	metadata cacheMetadata,
	manifestPresent bool,
) (assets.RuntimeCacheInspection, error) {
	snapshot, available, err := s.inspectCache(ctx, cacheDirectory, spec, source)
	if err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	manifestValid := manifestPresent && manifestContainsRequiredArtifacts(spec, metadata)
	result := assets.RuntimeCacheInspection{
		Supported:         true,
		Installed:         available,
		Revision:          snapshot.Revision,
		ManifestPresent:   manifestPresent,
		ManifestValid:     manifestValid,
		ExpectedArtifacts: append([]models.AssetRequirement(nil), expected...),
		ObservedArtifacts: append([]models.AssetArtifact(nil), snapshot.Artifacts...),
		MissingAssets:     missingAssetNames(expected, snapshot.Artifacts),
		PartialArtifacts:  len(snapshot.Artifacts) > 0 && !available,
		FailureReason:     cacheInspectionFailureReason(expected, snapshot.Artifacts, manifestValid),
	}
	if snapshot.Revision != "" {
		cachePath, pathErr := managedCacheChildPath(cacheRoot, snapshot.Revision, "revision")
		if pathErr != nil {
			return assets.RuntimeCacheInspection{}, pathErr
		}
		result.CachePath = cachePath
	}
	if available {
		result.InstalledFileCount = len(snapshot.Artifacts)
		result.MissingAssets = nil
		cacheBytes, measureErr := s.measureRevisionBytes(ctx, result.CachePath)
		if measureErr != nil {
			return assets.RuntimeCacheInspection{}, measureErr
		}
		result.CacheBytes = cacheBytes
	}
	return s.verifyRuntimeCacheIntegrity(ctx, cacheDirectory, spec, source, expected, result), nil
}

func (s *service) verifyRuntimeCacheIntegrity(
	ctx context.Context,
	cacheDirectory string,
	spec assetSpec,
	source models.SourceMetadata,
	expected []models.AssetRequirement,
	result assets.RuntimeCacheInspection,
) assets.RuntimeCacheInspection {
	if !result.ManifestValid || !hasVerifiableMetadata(expected) {
		return result
	}
	verified, verifiedAvailable, verifyErr := s.inspectVerifiedCache(ctx, cacheDirectory, spec, source)
	if verifyErr != nil {
		result.FailureReason = safeAssetFailureReason(verifyErr)
		result.Installed = false
		result.ObservedArtifacts = append([]models.AssetArtifact(nil), verified.Artifacts...)
		result.InstalledFileCount = len(verified.Artifacts)
		result.PartialArtifacts = len(verified.Artifacts) > 0
		return result
	}
	if !verifiedAvailable {
		return result
	}
	result.Installed = true
	result.IntegrityVerified = true
	result.ObservedArtifacts = append([]models.AssetArtifact(nil), verified.Artifacts...)
	result.InstalledFileCount = len(verified.Artifacts)
	result.MissingAssets = nil
	return result
}

func (s *service) applyActivePullFacts(
	inspection assets.RuntimeCacheInspection,
	active activePullState,
	isActive bool,
	pullFailure string,
) assets.RuntimeCacheInspection {
	inspection.ExpectedArtifacts = append([]models.AssetRequirement(nil), inspection.ExpectedArtifacts...)
	inspection.ObservedArtifacts = append([]models.AssetArtifact(nil), inspection.ObservedArtifacts...)
	inspection.MissingAssets = append([]string(nil), inspection.MissingAssets...)
	if isActive {
		inspection.ActivePull = true
		if len(active.expected) > 0 {
			inspection.ExpectedArtifacts = append([]models.AssetRequirement(nil), active.expected...)
			inspection.MissingAssets = missingAssetNames(active.expected, inspection.ObservedArtifacts)
		}
		if active.revision != "" {
			inspection.Revision = active.revision
		}
	} else if strings.TrimSpace(inspection.FailureReason) == "" {
		inspection.FailureReason = strings.TrimSpace(pullFailure)
	}
	return inspection
}

func requirementsFromMetadata(spec assetSpec, metadata cacheMetadata) []models.AssetRequirement {
	byName := make(map[string]metadataFile, len(metadata.Files))
	for _, file := range metadata.Files {
		byName[filepath.ToSlash(strings.TrimSpace(file.Path))] = file
	}
	result := make([]models.AssetRequirement, 0, len(spec.requiredArtifacts))
	for _, name := range spec.requiredArtifacts {
		file := byName[name]
		result = append(result, models.AssetRequirement{
			Name: name, Bytes: file.Bytes, SHA256: strings.ToLower(strings.TrimSpace(file.SHA256)),
		})
	}
	return result
}

func manifestContainsRequiredArtifacts(spec assetSpec, metadata cacheMetadata) bool {
	seen := make(map[string]struct{}, len(metadata.Files))
	for _, file := range metadata.Files {
		seen[filepath.ToSlash(strings.TrimSpace(file.Path))] = struct{}{}
	}
	for _, name := range spec.requiredArtifacts {
		if _, ok := seen[name]; !ok {
			return false
		}
	}
	return strings.TrimSpace(metadata.Revision) != ""
}

func hasVerifiableMetadata(expected []models.AssetRequirement) bool {
	if len(expected) == 0 {
		return false
	}
	for _, artifact := range expected {
		if artifact.Bytes <= 0 || len(strings.TrimSpace(artifact.SHA256)) != 64 {
			return false
		}
	}
	return true
}

func missingAssetNames(
	expected []models.AssetRequirement,
	observed []models.AssetArtifact,
) []string {
	seen := make(map[string]struct{}, len(observed))
	for _, artifact := range observed {
		seen[filepath.ToSlash(strings.TrimSpace(artifact.Name))] = struct{}{}
	}
	missing := make([]string, 0, len(expected))
	for _, artifact := range expected {
		if _, ok := seen[filepath.ToSlash(strings.TrimSpace(artifact.Name))]; !ok {
			missing = append(missing, artifact.Name)
		}
	}
	return missing
}

func cacheInspectionFailureReason(
	expected []models.AssetRequirement,
	observed []models.AssetArtifact,
	manifestValid bool,
) string {
	if !manifestValid && len(observed) > 0 {
		return "managed cache manifest is missing or incomplete"
	}
	byName := make(map[string]models.AssetArtifact, len(observed))
	for _, artifact := range observed {
		byName[filepath.ToSlash(strings.TrimSpace(artifact.Name))] = artifact
	}
	for _, requirement := range expected {
		artifact, ok := byName[filepath.ToSlash(strings.TrimSpace(requirement.Name))]
		if ok && requirement.Bytes > 0 && artifact.Bytes != requirement.Bytes {
			return fmt.Sprintf("managed cache artifact %q has unexpected size", requirement.Name)
		}
	}
	return ""
}

func preparedRuntimeKey(scope models.RuntimeScopeRef, name string) string {
	return scope.String() + "|" + strings.ToUpper(strings.TrimSpace(name))
}

func (s *service) rememberPreparedRuntime(
	scope models.RuntimeScopeRef,
	name string,
	inspection assets.RuntimeCacheInspection,
) {
	if s == nil || scope.IsZero() || strings.TrimSpace(name) == "" {
		return
	}
	inspection.MissingAssets = append([]string(nil), inspection.MissingAssets...)
	inspection.ExpectedArtifacts = append([]models.AssetRequirement(nil), inspection.ExpectedArtifacts...)
	inspection.ObservedArtifacts = append([]models.AssetArtifact(nil), inspection.ObservedArtifacts...)
	inspection.BackendFiles = append([]string(nil), inspection.BackendFiles...)
	s.preparedRuntimeMu.Lock()
	s.preparedRuntime[preparedRuntimeKey(scope, name)] = inspection
	s.preparedRuntimeMu.Unlock()
}

func (s *service) preparedRuntimeInspection(
	scope models.RuntimeScopeRef,
	name string,
) (assets.RuntimeCacheInspection, bool) {
	if s == nil {
		return assets.RuntimeCacheInspection{}, false
	}
	s.preparedRuntimeMu.RLock()
	inspection, ok := s.preparedRuntime[preparedRuntimeKey(scope, name)]
	s.preparedRuntimeMu.RUnlock()
	if !ok {
		return assets.RuntimeCacheInspection{}, false
	}
	inspection.MissingAssets = append([]string(nil), inspection.MissingAssets...)
	inspection.ExpectedArtifacts = append([]models.AssetRequirement(nil), inspection.ExpectedArtifacts...)
	inspection.ObservedArtifacts = append([]models.AssetArtifact(nil), inspection.ObservedArtifacts...)
	inspection.BackendFiles = append([]string(nil), inspection.BackendFiles...)
	return inspection, true
}

func (s *service) beginActivePull(scope models.RuntimeScopeRef, name string, expected []models.AssetRequirement) {
	if s == nil || scope.IsZero() || strings.TrimSpace(name) == "" {
		return
	}
	key := preparedRuntimeKey(scope, name)
	s.pullStateMu.Lock()
	s.activePulls[key] = activePullState{expected: cloneAssetRequirements(expected)}
	delete(s.pullFailure, key)
	s.pullStateMu.Unlock()
}

func (s *service) updateActivePull(
	scope models.RuntimeScopeRef,
	name string,
	expected []models.AssetRequirement,
	revision string,
) {
	if s == nil || scope.IsZero() || strings.TrimSpace(name) == "" {
		return
	}
	key := preparedRuntimeKey(scope, name)
	s.pullStateMu.Lock()
	state, ok := s.activePulls[key]
	if ok {
		state.expected = cloneAssetRequirements(expected)
		state.revision = strings.TrimSpace(revision)
		s.activePulls[key] = state
	}
	s.pullStateMu.Unlock()
}

func (s *service) finishActivePull(scope models.RuntimeScopeRef, name string, err error) {
	if s == nil || scope.IsZero() || strings.TrimSpace(name) == "" {
		return
	}
	key := preparedRuntimeKey(scope, name)
	s.pullStateMu.Lock()
	delete(s.activePulls, key)
	if err == nil {
		delete(s.pullFailure, key)
	} else {
		s.pullFailure[key] = safeAssetFailureReason(err)
	}
	s.pullStateMu.Unlock()
}

func (s *service) activePullStateFor(scope models.RuntimeScopeRef, name string) (activePullState, bool, string) {
	if s == nil {
		return activePullState{}, false, ""
	}
	key := preparedRuntimeKey(scope, name)
	s.pullStateMu.RLock()
	state, active := s.activePulls[key]
	failure := s.pullFailure[key]
	s.pullStateMu.RUnlock()
	state.expected = cloneAssetRequirements(state.expected)
	return state, active, failure
}

func cloneAssetRequirements(requirements []models.AssetRequirement) []models.AssetRequirement {
	if len(requirements) == 0 {
		return nil
	}
	return append([]models.AssetRequirement(nil), requirements...)
}

func safeAssetFailureReason(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "asset pull cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "asset pull timed out"
	case errors.Is(err, models.ErrAssetIntegrityFailed):
		return "asset integrity verification failed"
	case errors.Is(err, models.ErrAssetSourceMissing):
		return "asset source is missing"
	case errors.Is(err, models.ErrAssetSourceUnsupported):
		return "asset source is unsupported"
	case errors.Is(err, models.ErrSourceFetchFailed):
		return "asset source fetch failed"
	case errors.Is(err, models.ErrAssetPreparationInterrupted):
		return "asset preparation was interrupted"
	default:
		return "asset pull failed"
	}
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
		OperatorModels: cloneModelOverlays(binding.OperatorModels),
	}.Clone(), nil
}

func cloneModelOverlays(overlays map[string]models.ModelOverlay) map[string]models.ModelOverlay {
	if overlays == nil {
		return nil
	}
	cloned := make(map[string]models.ModelOverlay, len(overlays))
	for name, overlay := range overlays {
		cloned[name] = overlay.Clone()
	}
	return cloned
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
	return managedCacheChildPath(root, modelName, "model")
}

func managedCacheChildPath(root, child, kind string) (string, error) {
	child = strings.TrimSpace(child)
	if child == "" || child == "." || child == ".." ||
		filepath.Base(child) != child || filepath.IsAbs(child) ||
		filepath.VolumeName(child) != "" || strings.ContainsAny(child, `/\\`) {
		return "", fmt.Errorf("managed cache %s path is invalid", kind)
	}
	root, err := filepath.Abs(filepath.Clean(strings.TrimSpace(root)))
	if err != nil {
		return "", fmt.Errorf("resolve managed cache root: %w", err)
	}
	resolved := filepath.Join(root, child)
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("managed cache %s path escapes its root", kind)
	}
	return resolved, nil
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
	revisionPath, err := managedCacheChildPath(root, revision, "revision")
	if err != nil {
		return unavailableSnapshot(spec.modelName, source, revision, nil), false, fmt.Errorf(
			"%w: %v", models.ErrModelCacheUnsafe, err,
		)
	}
	artifacts := make([]models.AssetArtifact, 0, len(spec.requiredArtifacts))
	for _, name := range spec.requiredArtifacts {
		if err := assetContextError(ctx); err != nil {
			return missingSnapshot(spec.modelName, source), false, err
		}
		info, err := s.inspectPath(filepath.Join(revisionPath, filepath.FromSlash(name)))
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
		if entry == nil || entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
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
	return &assetCancellationError{
		cause: errors.Join(ctx.Err(), context.Cause(ctx)),
	}
}

type assetCancellationError struct {
	cause error
}

func (failure *assetCancellationError) Error() string {
	if failure == nil {
		return ""
	}
	return models.ErrAssetCancelled.Error()
}

func (failure *assetCancellationError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return errors.Join(models.ErrAssetCancelled, failure.cause)
}
