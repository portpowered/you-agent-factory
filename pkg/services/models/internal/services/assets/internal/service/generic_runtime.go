package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

// publishGenericRuntimeCache promotes a verified content-addressed model
// snapshot into the managed runtime layout. The content snapshots remain the
// reusable source caches; the managed layout is the durable contract consumed
// by runtime resolution and inspection, including the backend snapshot needed
// by a later process for offline reuse.
func (s *service) publishGenericRuntimeCache(
	ctx context.Context,
	cacheDirectory string,
	modelName string,
	source genericSource,
	result genericCacheResult,
	backendResult genericCacheResult,
	backendRevision string,
) (inspection assets.RuntimeCacheInspection, err error) {
	if strings.TrimSpace(modelName) == "" || result.snapshotPath == "" {
		return assets.RuntimeCacheInspection{}, nil
	}
	metadataFiles, err := genericRuntimeMetadataFiles(result.artifacts)
	if err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	if len(metadataFiles) != len(result.paths) {
		return assets.RuntimeCacheInspection{}, fmt.Errorf(
			"%w: generic runtime snapshot is incomplete",
			models.ErrAssetPreparationInterrupted,
		)
	}
	backendMetadata, err := s.genericRuntimeBackendMetadata(
		cacheDirectory, backendResult, backendRevision,
	)
	if err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	if err := assetContextError(ctx); err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	lock, err := s.lockGenericRuntime(ctx, cacheDirectory, modelName)
	if err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	defer func() {
		err = closeAssetStagingLock(lock, err)
	}()

	revision := genericRuntimeRevision(source, result.artifacts)
	existing, _, inspectErr := s.inspectGenericRuntimeCache(
		ctx, cacheDirectory, canonicalModelName(modelName), source,
	)
	if inspectErr != nil {
		return assets.RuntimeCacheInspection{}, inspectErr
	}
	if genericRuntimeInspectionMatches(existing, revision, metadataFiles) &&
		s.genericRuntimeBackendInspectionMatches(existing, cacheDirectory, backendMetadata) {
		return existing, nil
	}

	publication, err := s.newGenericRuntimePublication(
		cacheDirectory, canonicalModelName(modelName), revision,
	)
	if err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	defer func() {
		publication.rollback(s)
	}()
	if err := s.stageGenericRuntimePublication(ctx, publication, result, metadataFiles, backendMetadata); err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	if err := s.commitGenericRuntimePublication(ctx, &publication); err != nil {
		return assets.RuntimeCacheInspection{}, err
	}
	publication.committed = true
	publication.removeBackups(s)

	inspection = genericRuntimeInspectionFromArtifacts(
		publication.finalPath, publication.revision, metadataFiles, result.artifacts,
	)
	return inspection, nil
}

func genericRuntimeInspectionMatches(
	inspection assets.RuntimeCacheInspection,
	revision string,
	metadataFiles []metadataFile,
) bool {
	if !inspection.Installed || inspection.Revision != revision ||
		len(inspection.ObservedArtifacts) != len(metadataFiles) {
		return false
	}
	observed := make(map[string]models.AssetArtifact, len(inspection.ObservedArtifacts))
	for _, artifact := range inspection.ObservedArtifacts {
		observed[filepath.ToSlash(strings.TrimSpace(artifact.Name))] = artifact
	}
	for _, file := range metadataFiles {
		artifact, ok := observed[file.Path]
		if !ok || artifact.Bytes != file.Bytes ||
			!strings.EqualFold(strings.TrimSpace(artifact.SHA256), strings.TrimSpace(file.SHA256)) {
			return false
		}
	}
	return true
}

func (s *service) genericRuntimeBackendInspectionMatches(
	inspection assets.RuntimeCacheInspection,
	cacheDirectory string,
	expected *runtimeBackendMetadata,
) bool {
	if expected == nil {
		return true
	}
	if !inspection.BackendRequired || inspection.BackendRevision != expected.Revision ||
		len(inspection.BackendFiles) != len(expected.Files) {
		return false
	}
	expectedPath, err := s.resolveGenericRuntimeRelativePath(
		cacheDirectory, expected.CachePath, "backend",
	)
	if err != nil || filepath.Clean(inspection.BackendCachePath) != filepath.Clean(expectedPath) {
		return false
	}
	observed := make(map[string]struct{}, len(inspection.BackendFiles))
	for _, path := range inspection.BackendFiles {
		relative, relErr := filepath.Rel(inspection.BackendCachePath, path)
		if relErr != nil || !validGenericRuntimeRelativePath(filepath.ToSlash(relative)) {
			return false
		}
		observed[filepath.ToSlash(relative)] = struct{}{}
	}
	for _, file := range expected.Files {
		if _, ok := observed[file.Path]; !ok {
			return false
		}
	}
	return true
}

type genericRuntimePublication struct {
	modelName         string
	revision          string
	finalPath         string
	stagePath         string
	metadataPath      string
	metadataStagePath string
	finalBackup       string
	metadataBackup    string
	hadFinal          bool
	hadMetadata       bool
	finalPromoted     bool
	committed         bool
}

func (s *service) newGenericRuntimePublication(
	cacheDirectory, modelName, revision string,
) (genericRuntimePublication, error) {
	root, err := s.modelCacheRoot(cacheDirectory, modelName)
	if err != nil {
		return genericRuntimePublication{}, err
	}
	publication := genericRuntimePublication{
		modelName:         modelName,
		revision:          revision,
		finalPath:         filepath.Join(root, revision),
		stagePath:         filepath.Join(root, revision+".partial"),
		metadataPath:      filepath.Join(root, metadataFileName),
		metadataStagePath: filepath.Join(root, metadataFileName+".partial"),
	}
	if err := s.ensureStageAbsent(publication.stagePath); err != nil {
		return genericRuntimePublication{}, err
	}
	if err := s.ensureStageAbsent(publication.metadataStagePath); err != nil {
		return genericRuntimePublication{}, err
	}
	if err := s.makeDirectory(publication.stagePath, 0o755); err != nil {
		return genericRuntimePublication{}, interruptedAssetError(
			"prepare managed runtime staging directory", err,
		)
	}
	return publication, nil
}

func (s *service) stageGenericRuntimePublication(
	ctx context.Context,
	publication genericRuntimePublication,
	result genericCacheResult,
	metadataFiles []metadataFile,
	backend *runtimeBackendMetadata,
) error {
	for index := range result.artifacts {
		if err := assetContextError(ctx); err != nil {
			return err
		}
		target := filepath.Join(publication.stagePath, filepath.FromSlash(metadataFiles[index].Path))
		if err := s.makeDirectory(filepath.Dir(target), 0o755); err != nil {
			return interruptedAssetError("prepare managed runtime asset directory", err)
		}
		sourcePath := filepath.Join(result.snapshotPath, filepath.FromSlash(metadataFiles[index].Path))
		if err := s.copyCachedFile(ctx, sourcePath, target); err != nil {
			return interruptedAssetError("copy verified generic asset into managed runtime", err)
		}
		if _, err := s.verifyCachedFile(ctx, target, metadataFiles[index]); err != nil {
			return interruptedAssetError("verify managed runtime asset", err)
		}
	}
	metadata := cacheMetadata{
		ModelName: publication.modelName,
		Revision:  publication.revision,
		Files:     metadataFiles,
		Backend:   backend,
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return interruptedAssetError("encode managed runtime metadata", err)
	}
	if err := s.writeFile(publication.metadataStagePath, body, 0o644); err != nil {
		return interruptedAssetError("stage managed runtime metadata", err)
	}
	return assetContextError(ctx)
}

func (s *service) commitGenericRuntimePublication(
	ctx context.Context,
	publication *genericRuntimePublication,
) error {
	var err error
	publication.finalBackup, publication.hadFinal, err = s.moveExistingGenericSnapshot(publication.finalPath)
	if err != nil {
		return fmt.Errorf("%w: replace managed runtime snapshot: %v", models.ErrAssetPreparationInterrupted, err)
	}
	publication.metadataBackup, publication.hadMetadata, err = s.moveExistingGenericMetadata(publication.metadataPath)
	if err != nil {
		return fmt.Errorf("%w: replace managed runtime metadata: %v", models.ErrAssetPreparationInterrupted, err)
	}
	if err := s.renamePath(publication.stagePath, publication.finalPath); err != nil {
		return fmt.Errorf("%w: publish managed runtime snapshot: %v", models.ErrAssetPreparationInterrupted, err)
	}
	publication.finalPromoted = true
	if err := assetContextError(ctx); err != nil {
		return err
	}
	if err := s.renamePath(publication.metadataStagePath, publication.metadataPath); err != nil {
		return fmt.Errorf("%w: publish managed runtime metadata: %v", models.ErrAssetPreparationInterrupted, err)
	}
	return nil
}

func (publication *genericRuntimePublication) rollback(s *service) {
	if publication == nil || publication.committed {
		return
	}
	_ = s.removeTree(publication.stagePath)
	_ = s.removePath(publication.metadataStagePath)
	if publication.finalPromoted {
		_ = s.removeTree(publication.finalPath)
	}
	if publication.hadFinal {
		_ = s.renamePath(publication.finalBackup, publication.finalPath)
	}
	if publication.hadMetadata {
		_ = s.renamePath(publication.metadataBackup, publication.metadataPath)
	}
}

func (publication *genericRuntimePublication) removeBackups(s *service) {
	if publication == nil {
		return
	}
	if publication.hadFinal {
		_ = s.removeTree(publication.finalBackup)
	}
	if publication.hadMetadata {
		_ = s.removePath(publication.metadataBackup)
	}
}

func genericRuntimeMetadataFiles(artifacts []models.AssetArtifact) ([]metadataFile, error) {
	files := make([]metadataFile, 0, len(artifacts))
	seen := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		name := filepath.ToSlash(strings.TrimSpace(artifact.Name))
		requirement := models.AssetRequirement{
			Name: name, Bytes: artifact.Bytes, SHA256: strings.ToLower(strings.TrimSpace(artifact.SHA256)),
		}
		if err := requirement.Validate(); err != nil {
			return nil, fmt.Errorf("%w: generic runtime artifact %q is invalid", models.ErrAssetPreparationInterrupted, name)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("%w: generic runtime artifact %q is duplicated", models.ErrAssetPreparationInterrupted, name)
		}
		seen[name] = struct{}{}
		files = append(files, metadataFile{
			Path: name, Bytes: artifact.Bytes, SHA256: requirement.SHA256,
		})
	}
	return files, nil
}

func (s *service) genericRuntimeBackendMetadata(
	cacheDirectory string,
	result genericCacheResult,
	revision string,
) (*runtimeBackendMetadata, error) {
	if result.snapshotPath == "" && len(result.artifacts) == 0 && len(result.paths) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(result.snapshotPath) == "" || len(result.artifacts) == 0 ||
		len(result.artifacts) != len(result.paths) {
		return nil, fmt.Errorf(
			"%w: generic runtime backend snapshot is incomplete",
			models.ErrAssetPreparationInterrupted,
		)
	}
	files, err := genericRuntimeMetadataFiles(result.artifacts)
	if err != nil {
		return nil, err
	}
	cacheRoot, err := s.genericRuntimeCacheDirectory(cacheDirectory)
	if err != nil {
		return nil, err
	}
	snapshotPath, err := filepath.Abs(filepath.Clean(result.snapshotPath))
	if err != nil {
		return nil, fmt.Errorf("resolve generic runtime backend snapshot: %w", err)
	}
	relative, err := filepath.Rel(cacheRoot, snapshotPath)
	if err != nil || !validGenericRuntimeRelativePath(filepath.ToSlash(relative)) {
		return nil, fmt.Errorf(
			"%w: generic runtime backend snapshot is outside the model cache",
			models.ErrAssetPreparationInterrupted,
		)
	}
	for index, path := range result.paths {
		observedPath, pathErr := filepath.Abs(filepath.Clean(path))
		if pathErr != nil {
			return nil, fmt.Errorf("resolve generic runtime backend artifact: %w", pathErr)
		}
		expectedPath := filepath.Join(snapshotPath, filepath.FromSlash(files[index].Path))
		if filepath.Clean(observedPath) != filepath.Clean(expectedPath) {
			return nil, fmt.Errorf(
				"%w: generic runtime backend artifact escaped its snapshot",
				models.ErrAssetPreparationInterrupted,
			)
		}
	}
	return &runtimeBackendMetadata{
		CachePath: filepath.ToSlash(relative),
		Revision:  strings.TrimSpace(revision),
		Files:     files,
	}, nil
}

func (s *service) genericRuntimeCacheDirectory(cacheDirectory string) (string, error) {
	root := strings.TrimSpace(cacheDirectory)
	if root == "" {
		home, err := s.resolveHome()
		if err != nil || strings.TrimSpace(home) == "" {
			return "", fmt.Errorf("resolve generic runtime cache directory: %w", models.ErrAssetSourceMissing)
		}
		root = filepath.Join(home, ".agent-factory", "models")
	}
	resolved, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("resolve generic runtime cache directory: %w", err)
	}
	return resolved, nil
}

func validGenericRuntimeRelativePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value != filepath.ToSlash(value) {
		return false
	}
	local := filepath.FromSlash(value)
	if filepath.IsAbs(local) || filepath.VolumeName(local) != "" {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(local))
	return clean == value && clean != "." && clean != ".." &&
		!strings.HasPrefix(clean, "../")
}

func (s *service) resolveGenericRuntimeRelativePath(
	cacheDirectory, value, kind string,
) (string, error) {
	root, err := s.genericRuntimeCacheDirectory(cacheDirectory)
	if err != nil {
		return "", err
	}
	if !validGenericRuntimeRelativePath(value) {
		return "", fmt.Errorf("managed cache %s path is invalid", kind)
	}
	resolved := filepath.Join(root, filepath.FromSlash(value))
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("managed cache %s path escapes its root", kind)
	}
	return resolved, nil
}

func genericRuntimeRevision(source genericSource, artifacts []models.AssetArtifact) string {
	if revision := strings.TrimSpace(source.revision); revision != "" {
		return revision
	}
	generic := make([]genericArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		generic = append(generic, genericArtifact{requirement: models.AssetRequirement{
			Name: artifact.Name, Bytes: artifact.Bytes, SHA256: artifact.SHA256,
		}})
	}
	return genericArtifactIdentityHash(assetKindModel, source, generic)
}

func genericRuntimeInspectionFromArtifacts(
	cachePath string,
	revision string,
	metadataFiles []metadataFile,
	artifacts []models.AssetArtifact,
) assets.RuntimeCacheInspection {
	expected := make([]models.AssetRequirement, 0, len(metadataFiles))
	for _, file := range metadataFiles {
		expected = append(expected, models.AssetRequirement{
			Name: file.Path, Bytes: file.Bytes, SHA256: file.SHA256,
		})
	}
	var cacheBytes int64
	for _, artifact := range artifacts {
		cacheBytes += artifact.Bytes
	}
	return assets.RuntimeCacheInspection{
		Supported:          true,
		Installed:          true,
		Revision:           revision,
		CachePath:          cachePath,
		CacheBytes:         cacheBytes,
		InstalledFileCount: len(artifacts),
		ManifestPresent:    true,
		ManifestValid:      true,
		ExpectedArtifacts:  expected,
		ObservedArtifacts:  append([]models.AssetArtifact(nil), artifacts...),
		IntegrityVerified:  hasVerifiableMetadata(expected),
	}
}

func (s *service) moveExistingGenericMetadata(path string) (string, bool, error) {
	info, err := s.inspectPath(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info == nil || info.IsDir() {
		return "", false, fmt.Errorf("managed runtime metadata destination is not a file")
	}
	backupPath := path + ".previous"
	if err := s.removePath(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	if err := s.renamePath(path, backupPath); err != nil {
		return "", false, err
	}
	return backupPath, true, nil
}

// inspectGenericRuntimeCache reads the durable managed layout without using
// process-local preparation state or contacting the source.
func (s *service) inspectGenericRuntimeCache(
	ctx context.Context,
	cacheDirectory string,
	modelName string,
	source genericSource,
) (assets.RuntimeCacheInspection, bool, error) {
	inspection := assets.RuntimeCacheInspection{
		Supported:         true,
		ExpectedArtifacts: genericRuntimeExpectedArtifacts(source),
	}
	root, err := s.modelCacheRoot(cacheDirectory, canonicalModelName(modelName))
	if err != nil {
		return assets.RuntimeCacheInspection{}, false, err
	}
	metadata, present, invalid, err := s.readGenericRuntimeMetadataForInspection(
		ctx, filepath.Join(root, metadataFileName),
	)
	if err != nil {
		return assets.RuntimeCacheInspection{}, present, err
	}
	if invalid {
		inspection.ManifestPresent = present
		inspection.FailureReason = "managed cache manifest is invalid"
		return inspection, present, nil
	}
	if !present {
		return inspection, false, nil
	}
	inspection.ManifestPresent = true
	inspection.ManifestValid = true
	inspection.ExpectedArtifacts = genericRuntimeRequirements(metadata)
	revisionPath, err := managedCacheChildPath(root, metadata.Revision, "revision")
	if err != nil {
		inspection.ManifestValid = false
		inspection.FailureReason = "managed cache manifest is invalid"
		return inspection, true, nil
	}
	observed, missing, failureReason, err := s.inspectGenericRuntimeFiles(ctx, revisionPath, metadata.Files)
	if err != nil {
		return assets.RuntimeCacheInspection{}, true, err
	}
	inspection.ObservedArtifacts = observed
	inspection.MissingAssets = missing
	inspection.PartialArtifacts = len(observed) > 0 && len(missing) > 0
	if failureReason != "" {
		inspection.InstalledFileCount = len(observed)
		inspection.PartialArtifacts = len(observed) > 0
		inspection.FailureReason = failureReason
		return inspection, true, nil
	}
	if !genericRuntimeSourceMatchesMetadata(source, metadata) {
		inspection.ExpectedArtifacts = genericRuntimeExpectedArtifacts(source)
		inspection.MissingAssets = missingAssetNames(inspection.ExpectedArtifacts, observed)
		inspection.InstalledFileCount = len(observed)
		inspection.PartialArtifacts = len(observed) > 0
		inspection.FailureReason = "managed cache does not match configured source"
		return inspection, true, nil
	}
	if len(missing) > 0 {
		return inspection, true, nil
	}
	inspection.Installed = true
	inspection.Revision = metadata.Revision
	inspection.CachePath = revisionPath
	inspection.InstalledFileCount = len(observed)
	inspection.IntegrityVerified = hasVerifiableMetadata(inspection.ExpectedArtifacts)
	inspection.CacheBytes, err = s.measureRevisionBytes(ctx, revisionPath)
	if err != nil {
		return assets.RuntimeCacheInspection{}, true, err
	}
	if err := s.inspectGenericRuntimeBackend(ctx, cacheDirectory, &inspection, metadata.Backend); err != nil {
		return assets.RuntimeCacheInspection{}, true, err
	}
	return inspection, true, nil
}

func (s *service) readGenericRuntimeMetadataForInspection(
	ctx context.Context,
	path string,
) (cacheMetadata, bool, bool, error) {
	metadata, present, err := s.readGenericRuntimeMetadata(ctx, path)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return metadata, present, false, err
		}
		if !errors.Is(err, models.ErrAssetUnavailable) {
			return metadata, present, false, err
		}
		return metadata, present, true, nil
	}
	if !present {
		return metadata, false, false, nil
	}
	if !validGenericRuntimeMetadata(metadata) {
		return metadata, present, true, nil
	}
	return metadata, true, false, nil
}

func (s *service) inspectGenericRuntimeBackend(
	ctx context.Context,
	cacheDirectory string,
	inspection *assets.RuntimeCacheInspection,
	metadata *runtimeBackendMetadata,
) error {
	if inspection == nil || metadata == nil {
		return nil
	}
	inspection.BackendRequired = true
	backendPath, err := s.resolveGenericRuntimeRelativePath(
		cacheDirectory, metadata.CachePath, "backend",
	)
	if err != nil {
		inspection.Installed = false
		inspection.FailureReason = "managed cache backend is missing or invalid"
		return nil
	}
	inspection.BackendCachePath = backendPath
	inspection.BackendRevision = metadata.Revision
	observed, missing, failureReason, err := s.inspectGenericRuntimeFiles(
		ctx, backendPath, metadata.Files,
	)
	if err != nil {
		return err
	}
	inspection.BackendInstalledFiles = len(observed)
	inspection.BackendFiles = make([]string, 0, len(observed))
	for _, artifact := range observed {
		inspection.BackendFiles = append(inspection.BackendFiles, filepath.Join(
			backendPath, filepath.FromSlash(artifact.Name),
		))
	}
	if failureReason != "" || len(missing) > 0 {
		inspection.Installed = false
		inspection.PartialArtifacts = inspection.PartialArtifacts || len(observed) > 0
		inspection.FailureReason = "managed cache backend is missing or invalid"
	}
	return nil
}

func (s *service) inspectGenericRuntimeFiles(
	ctx context.Context,
	revisionPath string,
	files []metadataFile,
) ([]models.AssetArtifact, []string, string, error) {
	observed := make([]models.AssetArtifact, 0, len(files))
	missing := make([]string, 0)
	for _, file := range files {
		if err := assetContextError(ctx); err != nil {
			return nil, nil, "", err
		}
		artifact, verifyErr := s.verifyCachedFile(
			ctx, filepath.Join(revisionPath, filepath.FromSlash(file.Path)), file,
		)
		if errors.Is(verifyErr, os.ErrNotExist) {
			missing = append(missing, file.Path)
			continue
		}
		if verifyErr != nil {
			observed = append(observed, artifact)
			return observed, missing, safeAssetFailureReason(verifyErr), nil
		}
		observed = append(observed, artifact)
	}
	return observed, missing, "", nil
}

func genericRuntimeSourceMatchesMetadata(source genericSource, metadata cacheMetadata) bool {
	if source.kind != genericSourceHF {
		return true
	}
	if revision := strings.TrimSpace(source.revision); revision != "" &&
		!strings.EqualFold(revision, strings.TrimSpace(metadata.Revision)) {
		return false
	}
	if expected := filepath.ToSlash(strings.TrimSpace(source.file)); expected != "" {
		for _, file := range metadata.Files {
			if filepath.ToSlash(strings.TrimSpace(file.Path)) == expected {
				return true
			}
		}
		return false
	}
	return true
}

func (s *service) reusableGenericRuntimeCache(
	ctx context.Context,
	plan genericPreparationPlan,
) (*assets.RuntimeCacheInspection, bool, error) {
	if strings.TrimSpace(plan.source.modelName) == "" {
		return nil, false, nil
	}
	inspection, present, err := s.inspectGenericRuntimeCache(
		ctx, plan.cacheDirectory, plan.source.modelName, plan.source,
	)
	if err != nil || !present || !inspection.Installed ||
		!inspection.ManifestValid || !inspection.IntegrityVerified ||
		!genericRuntimeCacheMatchesPlan(plan, inspection) {
		return nil, false, err
	}
	return &inspection, true, nil
}

func genericRuntimeCacheMatchesPlan(
	plan genericPreparationPlan,
	inspection assets.RuntimeCacheInspection,
) bool {
	requestedRevision := strings.TrimSpace(plan.source.revision)
	if requestedRevision != "" {
		if !strings.EqualFold(strings.TrimSpace(inspection.Revision), requestedRevision) {
			return false
		}
	} else if len(plan.modelRequirements) == 0 {
		// Without an immutable source revision or requested artifact facts there
		// is no safe identity with which to associate a managed cache record.
		return false
	}
	requestedArtifacts := make([]models.AssetRequirement, 0, len(plan.modelRequirements))
	for _, artifact := range plan.modelRequirements {
		requestedArtifacts = append(requestedArtifacts, artifact.requirement)
	}
	return genericRuntimeRequirementsSatisfy(
		requestedArtifacts, inspection.ExpectedArtifacts,
	) && genericRuntimeBackendRequirementsSatisfy(plan, inspection)
}

func genericRuntimeBackendRequirementsSatisfy(
	plan genericPreparationPlan,
	inspection assets.RuntimeCacheInspection,
) bool {
	if len(plan.backendRequirements) == 0 {
		return true
	}
	if !inspection.BackendRequired || strings.TrimSpace(inspection.BackendCachePath) == "" ||
		inspection.BackendInstalledFiles != len(inspection.BackendFiles) ||
		len(inspection.BackendFiles) != len(plan.backendRequirements) {
		return false
	}
	requestedRevision := strings.TrimSpace(plan.backendSource.revision)
	if requestedRevision != "" && !strings.EqualFold(
		strings.TrimSpace(inspection.BackendRevision), requestedRevision,
	) {
		return false
	}
	observed := make(map[string]struct{}, len(inspection.BackendFiles))
	for _, path := range inspection.BackendFiles {
		relative, err := filepath.Rel(inspection.BackendCachePath, path)
		if err != nil || !validGenericRuntimeRelativePath(filepath.ToSlash(relative)) {
			return false
		}
		observed[filepath.ToSlash(relative)] = struct{}{}
	}
	for _, artifact := range plan.backendRequirements {
		name := filepath.ToSlash(strings.TrimSpace(artifact.requirement.Name))
		if !validGenericRuntimeRelativePath(name) {
			return false
		}
		if _, ok := observed[name]; !ok {
			return false
		}
	}
	return true
}

func genericRuntimeRequirementsSatisfy(
	requested []models.AssetRequirement,
	cached []models.AssetRequirement,
) bool {
	if len(requested) == 0 {
		return true
	}
	byName := make(map[string]models.AssetRequirement, len(cached))
	for _, requirement := range cached {
		byName[filepath.ToSlash(strings.TrimSpace(requirement.Name))] = requirement
	}
	for _, requirement := range requested {
		cachedRequirement, ok := byName[filepath.ToSlash(strings.TrimSpace(requirement.Name))]
		if !ok {
			return false
		}
		if requirement.Bytes > 0 && cachedRequirement.Bytes != requirement.Bytes {
			return false
		}
		if digest := strings.TrimSpace(requirement.SHA256); digest != "" &&
			!strings.EqualFold(digest, strings.TrimSpace(cachedRequirement.SHA256)) {
			return false
		}
	}
	return true
}

func genericCacheResultFromRuntimeCache(
	inspection assets.RuntimeCacheInspection,
) genericCacheResult {
	result := genericCacheResult{
		artifacts:    make([]models.AssetArtifact, 0, len(inspection.ObservedArtifacts)),
		paths:        make([]string, 0, len(inspection.ObservedArtifacts)),
		snapshotPath: strings.TrimSpace(inspection.CachePath),
	}
	for _, artifact := range inspection.ObservedArtifacts {
		name := filepath.ToSlash(strings.TrimSpace(artifact.Name))
		if name == "" || strings.TrimSpace(inspection.CachePath) == "" {
			continue
		}
		artifact.Name = name
		artifact.Kind = models.AssetArtifactKindModel
		result.artifacts = append(result.artifacts, artifact)
		result.paths = append(result.paths, filepath.Join(
			inspection.CachePath, filepath.FromSlash(name),
		))
	}
	return result
}

func (s *service) resolveGenericRuntimeCache(
	ctx context.Context,
	scope models.RuntimeScopeConfig,
	scopeRef models.RuntimeScopeRef,
	modelName string,
) (assets.RuntimeCacheLayout, error) {
	// Source resolution is used only to classify a generic runtime as a
	// supported model and to provide a useful expected artifact when the
	// durable record is absent. The cache record itself remains authoritative.
	source, sourceErr := s.resolveGenericSource(ctx, scope, modelName)
	inspection, persisted, err := s.inspectGenericRuntimeCache(
		ctx, scope.CacheDirectory, modelName, source,
	)
	if err != nil {
		return assets.RuntimeCacheLayout{}, err
	}
	if sourceErr != nil && !persisted {
		return assets.RuntimeCacheLayout{}, fmt.Errorf(
			"%w: required assets missing for %s", models.ErrNotAvailable, canonicalModelName(modelName),
		)
	}
	if prepared, ok := s.preparedRuntimeInspection(scopeRef, modelName); ok {
		inspection = mergeGenericRuntimeBackendFacts(inspection, prepared)
	}
	if !inspection.Installed {
		return assets.RuntimeCacheLayout{}, fmt.Errorf(
			"%w: required assets missing for %s", models.ErrNotAvailable, canonicalModelName(modelName),
		)
	}
	files := make([]string, 0, len(inspection.ObservedArtifacts))
	for _, artifact := range inspection.ObservedArtifacts {
		files = append(files, filepath.Join(inspection.CachePath, filepath.FromSlash(artifact.Name)))
	}
	return assets.RuntimeCacheLayout{
		ModelName:        canonicalModelName(modelName),
		CachePath:        inspection.CachePath,
		Revision:         inspection.Revision,
		Files:            files,
		BackendCachePath: inspection.BackendCachePath,
		BackendRevision:  inspection.BackendRevision,
		BackendFiles:     append([]string(nil), inspection.BackendFiles...),
	}, nil
}

func mergeGenericRuntimeBackendFacts(
	inspection assets.RuntimeCacheInspection,
	prepared assets.RuntimeCacheInspection,
) assets.RuntimeCacheInspection {
	if !prepared.BackendRequired {
		return inspection
	}
	inspection.BackendRequired = true
	inspection.BackendCachePath = prepared.BackendCachePath
	inspection.BackendRevision = prepared.BackendRevision
	inspection.BackendInstalledFiles = prepared.BackendInstalledFiles
	inspection.BackendFiles = append([]string(nil), prepared.BackendFiles...)
	return inspection
}

func (s *service) readGenericRuntimeMetadata(
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
		return cacheMetadata{}, true, fmt.Errorf("read managed cache metadata: %w", err)
	}
	var metadata cacheMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return cacheMetadata{}, true, fmt.Errorf(
			"%w: decode managed cache metadata: %v", models.ErrAssetUnavailable, err,
		)
	}
	return metadata, true, nil
}

func validGenericRuntimeMetadata(metadata cacheMetadata) bool {
	if strings.TrimSpace(metadata.Revision) == "" || len(metadata.Files) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(metadata.Files))
	for _, file := range metadata.Files {
		if !validGenericRuntimeMetadataFile(file) {
			return false
		}
		name := file.Path
		if _, exists := seen[name]; exists {
			return false
		}
		seen[name] = struct{}{}
	}
	return validGenericRuntimeBackendMetadata(metadata.Backend)
}

func validGenericRuntimeMetadataFile(file metadataFile) bool {
	name := filepath.ToSlash(strings.TrimSpace(file.Path))
	if name != file.Path || !validGenericRuntimeRelativePath(name) {
		return false
	}
	return (models.AssetRequirement{
		Name: name, Bytes: file.Bytes, SHA256: strings.ToLower(strings.TrimSpace(file.SHA256)),
	}).Validate() == nil
}

func validGenericRuntimeBackendMetadata(metadata *runtimeBackendMetadata) bool {
	if metadata == nil {
		return true
	}
	if !validGenericRuntimeRelativePath(metadata.CachePath) || len(metadata.Files) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(metadata.Files))
	for _, file := range metadata.Files {
		if !validGenericRuntimeMetadataFile(file) {
			return false
		}
		if _, exists := seen[file.Path]; exists {
			return false
		}
		seen[file.Path] = struct{}{}
	}
	return true
}

func genericRuntimeRequirements(metadata cacheMetadata) []models.AssetRequirement {
	result := make([]models.AssetRequirement, 0, len(metadata.Files))
	for _, file := range metadata.Files {
		result = append(result, models.AssetRequirement{
			Name: file.Path, Bytes: file.Bytes, SHA256: strings.ToLower(strings.TrimSpace(file.SHA256)),
		})
	}
	return result
}

func genericRuntimeExpectedArtifacts(source genericSource) []models.AssetRequirement {
	if name := filepath.ToSlash(strings.TrimSpace(source.file)); name != "" {
		return []models.AssetRequirement{{Name: name}}
	}
	return nil
}

func (s *service) inspectConfiguredRuntimeCache(
	ctx context.Context,
	scope models.RuntimeScopeConfig,
	modelName string,
	active activePullState,
	isActive bool,
	pullFailure string,
) (assets.RuntimeCacheInspection, bool, error) {
	spec, source, supported, err := s.resolveRuntimeCacheSource(scope.Runtime, modelName)
	if err != nil {
		return assets.RuntimeCacheInspection{}, true, err
	}
	if !supported {
		return assets.RuntimeCacheInspection{}, false, nil
	}
	expected := assetRequirementsForSpec(spec)
	cacheRoot, err := s.modelCacheRoot(scope.CacheDirectory, spec.modelName)
	if err != nil {
		return assets.RuntimeCacheInspection{}, true, err
	}
	metadata, manifestPresent, err := s.readMetadata(
		ctx, filepath.Join(cacheRoot, metadataFileName),
	)
	if err != nil {
		inspection, inspectionErr := s.invalidRuntimeCacheInspection(
			ctx, expected, active, isActive, pullFailure,
		)
		return inspection, true, inspectionErr
	}
	if manifestPresent {
		expected = requirementsFromMetadata(spec, metadata)
	}
	result, err := s.inspectRuntimeCacheFiles(
		ctx, scope.CacheDirectory, spec, source, expected, cacheRoot, metadata, manifestPresent,
	)
	if err != nil {
		return assets.RuntimeCacheInspection{}, true, err
	}
	return s.applyActivePullFacts(result, active, isActive, pullFailure), true, nil
}
