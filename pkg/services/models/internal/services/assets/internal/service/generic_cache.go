package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
)

func (s *service) acquireGenericCache(
	ctx context.Context,
	kind string,
	artifactKind models.AssetArtifactKind,
	source genericSource,
	artifacts []genericArtifact,
	roots []string,
	offline bool,
) (genericCacheResult, error) {
	if source.kind == genericSourceHF {
		if discovered := s.discoverContentAddressedRequirementsAcrossRoots(kind, source, roots); len(discovered) > 0 {
			cachedArtifacts := s.genericArtifactsFromRequirements(source, discovered)
			if len(artifacts) == 0 {
				artifacts = cachedArtifacts
			} else {
				var err error
				artifacts, err = mergeGenericManifest(artifacts, cachedArtifacts)
				if err != nil {
					return genericCacheResult{}, err
				}
			}
		}
	}
	key := genericCacheKey(kind, source, artifacts)
	s.cacheMu.Lock()
	if call, ok := s.inflight[key]; ok {
		done := call.done
		s.cacheMu.Unlock()
		if s.cacheJoinObserver != nil {
			s.cacheJoinObserver()
		}
		select {
		case <-ctx.Done():
			return genericCacheResult{}, ctx.Err()
		case <-done:
			return call.result, call.err
		}
	}
	call := &assetCacheCall{done: make(chan struct{})}
	s.inflight[key] = call
	s.cacheMu.Unlock()

	result, err := s.acquireGenericCacheOnce(
		ctx, kind, artifactKind, source, artifacts, roots, offline,
	)
	s.cacheMu.Lock()
	call.result, call.err = result, err
	delete(s.inflight, key)
	close(call.done)
	s.cacheMu.Unlock()
	return result, err
}

func (s *service) acquireGenericCacheOnce(
	ctx context.Context,
	kind string,
	artifactKind models.AssetArtifactKind,
	source genericSource,
	artifacts []genericArtifact,
	roots []string,
	offline bool,
) (genericCacheResult, error) {
	if len(artifacts) == 0 && source.kind == genericSourceHF {
		if offline {
			return genericCacheResult{}, &models.AssetOfflineError{
				Missing: []string{source.repository},
			}
		}
		manifest, err := s.fetchGenericManifest(ctx, source)
		if err != nil {
			return genericCacheResult{}, err
		}
		artifacts = manifest
	}
	s.addGenericURLs(source, artifacts)
	if err := assetContextError(ctx); err != nil {
		return genericCacheResult{}, err
	}
	cached, missing, inspectErr := s.inspectGenericCache(ctx, kind, source, artifacts, roots)
	if inspectErr != nil {
		return cacheResultFromPaths(artifactKind, artifacts, cached), inspectErr
	}
	if len(missing) == 0 {
		return cacheResultFromPaths(artifactKind, artifacts, cached), nil
	}
	if offline {
		return cacheResultFromPaths(artifactKind, artifacts, cached), &models.AssetOfflineError{
			Missing: missingArtifactNames(missing),
		}
	}
	if source.kind == genericSourceHF && genericArtifactsNeedManifest(artifacts) {
		manifest, err := s.fetchGenericManifest(ctx, source)
		if err != nil {
			return cacheResultFromPaths(artifactKind, artifacts, cached), err
		}
		artifacts, err = mergeGenericManifest(artifacts, manifest)
		if err != nil {
			return cacheResultFromPaths(artifactKind, artifacts, cached), err
		}
		s.addGenericURLs(source, artifacts)
		cached, missing, inspectErr = s.inspectGenericCache(ctx, kind, source, artifacts, roots)
		if inspectErr != nil {
			return cacheResultFromPaths(artifactKind, artifacts, cached), inspectErr
		}
		if len(missing) == 0 {
			return cacheResultFromPaths(artifactKind, artifacts, cached), nil
		}
	}
	return s.publishGenericCache(ctx, kind, artifactKind, source, artifacts, cached, missing, roots)
}

func (s *service) inspectGenericCache(
	ctx context.Context,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
	roots []string,
) (map[string]genericCachePath, []genericArtifact, error) {
	cached := make(map[string]genericCachePath, len(artifacts))
	missing := make([]genericArtifact, 0)
	identity := genericArtifactIdentityHash(kind, source, artifacts)
	for _, artifact := range artifacts {
		if err := assetContextError(ctx); err != nil {
			return cached, append(missing, artifact), err
		}
		found, ok, err := s.findGenericArtifact(ctx, kind, source, identity, artifact, roots)
		if err != nil {
			return cached, append(missing, artifact), err
		}
		if ok {
			cached[artifact.requirement.Name] = found
			continue
		}
		missing = append(missing, artifact)
	}
	return cached, missing, nil
}

func (s *service) findGenericArtifact(
	ctx context.Context,
	kind string,
	source genericSource,
	identity string,
	artifact genericArtifact,
	roots []string,
) (genericCachePath, bool, error) {
	for _, root := range roots {
		for _, candidate := range genericCandidatePaths(root, kind, source, identity, artifact.requirement.Name) {
			if err := assetContextError(ctx); err != nil {
				return genericCachePath{}, false, err
			}
			info, err := s.inspectPath(candidate)
			if err != nil || info.IsDir() {
				continue
			}
			actual, err := s.fileSHA256(ctx, candidate)
			if err != nil {
				if contextErr := assetContextError(ctx); contextErr != nil {
					return genericCachePath{}, false, contextErr
				}
				continue
			}
			expected := strings.ToLower(strings.TrimSpace(artifact.requirement.SHA256))
			// A file-backed HF reference has no trustworthy cache identity until
			// its immutable manifest supplies a digest. Size alone cannot reject
			// same-size corruption in a pre-existing hub cache.
			if source.kind == genericSourceHF && expected == "" {
				continue
			}
			if expected != "" && actual != expected {
				continue
			}
			if artifact.requirement.Bytes > 0 && info.Size() != artifact.requirement.Bytes {
				continue
			}
			return genericCachePath{
				artifact: models.AssetArtifact{
					Name: artifact.requirement.Name, Bytes: info.Size(), SHA256: actual,
				},
				path: candidate,
			}, true, nil
		}
	}
	return genericCachePath{}, false, nil
}

func (s *service) publishGenericCache(
	ctx context.Context,
	kind string,
	artifactKind models.AssetArtifactKind,
	source genericSource,
	artifacts []genericArtifact,
	cached map[string]genericCachePath,
	missing []genericArtifact,
	roots []string,
) (genericCacheResult, error) {
	if len(roots) == 0 {
		return cacheResultFromPaths(artifactKind, artifacts, cached), models.ErrAssetSourceMissing
	}
	destinationRoot := roots[len(roots)-1]
	identity := genericCacheKey(kind, source, artifacts)
	identityName := genericArtifactIdentityHash(kind, source, artifacts)
	base := filepath.Join(destinationRoot, assetContentDirectory, kind)
	finalPath := filepath.Join(base, identityName)
	stagePath := finalPath + ".partial"
	if err := s.prepareGenericStage(base, stagePath); err != nil {
		return cacheResultFromPaths(artifactKind, artifacts, cached), err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.removeTree(stagePath)
		}
	}()

	published, err := s.stageGenericArtifacts(
		ctx, artifactKind, source, artifacts, cached, missing, stagePath,
	)
	if err != nil {
		return cacheResultFromPaths(artifactKind, artifacts, published), err
	}
	if err := s.writeGenericMetadata(
		filepath.Join(stagePath, assetMetadataName), kind, identity, source.safe,
		genericSourceIdentity(source), artifacts,
	); err != nil {
		return cacheResultFromPaths(artifactKind, artifacts, published), interruptedAssetError(
			"stage asset metadata", err,
		)
	}
	if err := assetContextError(ctx); err != nil {
		return cacheResultFromPaths(artifactKind, artifacts, published), err
	}
	backupPath, hadExisting, err := s.moveExistingGenericSnapshot(finalPath)
	if err != nil {
		return cacheResultFromPaths(artifactKind, artifacts, published), pullsupport.WrapPullStage(
			models.PullStageCacheInstallation, "", "replace asset snapshot", "",
			interruptedAssetError("replace asset snapshot", err),
		)
	}
	if err := s.renamePath(stagePath, finalPath); err != nil {
		if hadExisting {
			_ = s.renamePath(backupPath, finalPath)
		}
		return cacheResultFromPaths(artifactKind, artifacts, published), pullsupport.WrapPullStage(
			models.PullStageCacheInstallation, "", "publish asset snapshot", "",
			interruptedAssetError("publish asset snapshot", err),
		)
	}
	committed = true
	if hadExisting {
		if err := s.removeTree(backupPath); err != nil {
			return cacheResultFromPaths(artifactKind, artifacts, published), pullsupport.WrapPullStage(
				models.PullStageCacheInstallation, "", "clean replaced asset snapshot", "",
				interruptedAssetError("clean replaced asset snapshot", err),
			)
		}
	}
	result := cacheResultFromPaths(artifactKind, artifacts, published)
	result.snapshotPath = finalPath
	result.prepared = true
	return result, nil
}

func (s *service) discoverContentAddressedRequirementsAcrossRoots(
	kind string,
	source genericSource,
	roots []string,
) []models.AssetRequirement {
	for _, root := range roots {
		if requirements := s.discoverContentAddressedRequirements(root, kind, source); len(requirements) > 0 {
			return requirements
		}
	}
	return nil
}

func (s *service) moveExistingGenericSnapshot(finalPath string) (string, bool, error) {
	info, err := s.inspectPath(finalPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info == nil || !info.IsDir() {
		return "", false, fmt.Errorf("asset snapshot destination is not a directory")
	}
	backupPath := finalPath + ".previous"
	if err := s.removeTree(backupPath); err != nil {
		return "", false, err
	}
	if err := s.renamePath(finalPath, backupPath); err != nil {
		return "", false, err
	}
	return backupPath, true, nil
}

func (s *service) prepareGenericStage(base, stagePath string) error {
	if err := s.removeTree(stagePath); err != nil {
		return interruptedAssetError("clear asset staging", err)
	}
	if err := s.makeDirectory(base, 0o755); err != nil {
		return interruptedAssetError("prepare asset cache", err)
	}
	if err := s.makeDirectory(stagePath, 0o755); err != nil {
		return interruptedAssetError("prepare asset staging", err)
	}
	return nil
}

func (s *service) stageGenericArtifacts(
	ctx context.Context,
	artifactKind models.AssetArtifactKind,
	source genericSource,
	artifacts []genericArtifact,
	cached map[string]genericCachePath,
	missing []genericArtifact,
	stagePath string,
) (map[string]genericCachePath, error) {
	published := make(map[string]genericCachePath, len(cached)+len(missing))
	for name, artifact := range cached {
		if err := s.preserveGenericArtifact(ctx, artifact, name, stagePath); err != nil {
			return published, err
		}
		artifact.path = filepath.Join(stagePath, filepath.FromSlash(name))
		published[name] = artifact
	}
	for _, artifact := range missing {
		if err := assetContextError(ctx); err != nil {
			return published, err
		}
		path := filepath.Join(stagePath, filepath.FromSlash(artifact.requirement.Name))
		if err := s.makeDirectory(filepath.Dir(path), 0o755); err != nil {
			return published, interruptedAssetError("prepare asset directory", err)
		}
		result, err := s.stageGenericArtifact(ctx, source, artifact.localPath, path, artifact.requirement)
		if err != nil {
			return published, err
		}
		result.Kind = artifactKind
		published[artifact.requirement.Name] = genericCachePath{artifact: result, path: path}
	}
	return published, nil
}

func (s *service) preserveGenericArtifact(
	ctx context.Context,
	artifact genericCachePath,
	name string,
	stagePath string,
) error {
	target := filepath.Join(stagePath, filepath.FromSlash(name))
	if err := s.makeDirectory(filepath.Dir(target), 0o755); err != nil {
		return interruptedAssetError("preserve cached asset", err)
	}
	if err := s.copyCachedFile(ctx, artifact.path, target); err != nil {
		return interruptedAssetError("preserve cached asset", err)
	}
	return nil
}

func (s *service) stageGenericArtifact(
	ctx context.Context,
	source genericSource,
	localPath string,
	target string,
	requirement models.AssetRequirement,
) (models.AssetArtifact, error) {
	switch source.kind {
	case genericSourceLocal, genericSourceFile:
		return s.copyLocalArtifact(ctx, localPath, target, requirement)
	case genericSourceHF, genericSourceRelease:
		return s.downloadGenericArtifact(ctx, source, target, requirement)
	default:
		return models.AssetArtifact{}, models.ErrAssetSourceUnsupported
	}
}

func (s *service) copyCachedFile(ctx context.Context, sourcePath, targetPath string) error {
	input, err := s.openFile(sourcePath)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := s.createFile(targetPath)
	if err != nil {
		return err
	}
	_, _, copyErr := copyStagedAsset(ctx, output, input, filepath.Base(targetPath))
	return copyErr
}

func cacheResultFromPaths(
	kind models.AssetArtifactKind,
	artifacts []genericArtifact,
	paths map[string]genericCachePath,
) genericCacheResult {
	result := genericCacheResult{
		artifacts: make([]models.AssetArtifact, 0, len(artifacts)),
		paths:     make([]string, 0, len(artifacts)),
	}
	for _, artifact := range artifacts {
		found, ok := paths[artifact.requirement.Name]
		if !ok {
			continue
		}
		found.artifact.Kind = kind
		result.artifacts = append(result.artifacts, found.artifact)
		result.paths = append(result.paths, found.path)
	}
	result.snapshotPath = genericSnapshotPath(result.paths)
	return result
}

func genericSnapshotPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	candidate := genericSnapshotPathFor(paths[0])
	if candidate == "" {
		return ""
	}
	for _, path := range paths[1:] {
		if genericSnapshotPathFor(path) != candidate {
			return ""
		}
	}
	return candidate
}

func genericSnapshotPathFor(path string) string {
	clean := filepath.Clean(path)
	for _, marker := range []string{assetContentDirectory, "snapshots"} {
		for current := clean; ; current = filepath.Dir(current) {
			if filepath.Base(current) == marker {
				relative, err := filepath.Rel(current, clean)
				if err != nil {
					break
				}
				parts := strings.Split(relative, string(filepath.Separator))
				if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
					return filepath.Join(current, parts[0], parts[1])
				}
				break
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
		}
	}
	return filepath.Dir(clean)
}

func (s *service) copyLocalArtifact(
	ctx context.Context,
	localPath string,
	target string,
	requirement models.AssetRequirement,
) (models.AssetArtifact, error) {
	input, err := s.openFile(localPath)
	if err != nil {
		return models.AssetArtifact{}, fmt.Errorf("%w: local asset is unavailable", models.ErrAssetSourceMissing)
	}
	defer input.Close()
	output, err := s.createFile(target)
	if err != nil {
		return models.AssetArtifact{}, interruptedAssetError("create staged asset", err)
	}
	written, checksum, err := copyStagedAsset(ctx, output, input, requirement.Name)
	if err != nil {
		return models.AssetArtifact{}, err
	}
	return verifiedGenericArtifact(requirement, written, checksum)
}

func (s *service) downloadGenericArtifact(
	ctx context.Context,
	source genericSource,
	target string,
	requirement models.AssetRequirement,
) (models.AssetArtifact, error) {
	assetURL := s.genericAssetURL(source, requirement.Name)
	diagnostics := models.PullDiagnostics{
		ModelName:          source.modelName,
		ResolvedRepository: source.owner + "/" + source.repository,
		Revision:           source.revision,
		File:               requirement.Name,
		Operation:          "download asset",
		RequestURL:         assetURL,
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: asset request is invalid", models.ErrSourceFetchFailed),
		)
	}
	response, err := s.doWithRetry(request)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return models.AssetArtifact{}, contextErr
		}
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: asset download failed", models.ErrSourceFetchFailed),
		)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		diagnostics.UpstreamStatusCode = response.StatusCode
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: asset download failed", models.ErrSourceFetchFailed),
		)
	}
	output, err := s.createFile(target)
	if err != nil {
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			interruptedAssetError("create staged asset", err),
		)
	}
	written, checksum, err := copyStagedAsset(ctx, output, response.Body, requirement.Name)
	if err != nil {
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(diagnostics, err)
	}
	artifact, err := verifiedGenericArtifact(requirement, written, checksum)
	if err != nil {
		diagnostics.Operation = "verify downloaded asset"
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(diagnostics, err)
	}
	return artifact, nil
}

func verifiedGenericArtifact(
	requirement models.AssetRequirement,
	written int64,
	checksum string,
) (models.AssetArtifact, error) {
	if requirement.Bytes > 0 && written != requirement.Bytes {
		return models.AssetArtifact{}, fmt.Errorf(
			"%w: asset %q size does not match", models.ErrAssetIntegrityFailed, requirement.Name,
		)
	}
	if expected := strings.ToLower(strings.TrimSpace(requirement.SHA256)); expected != "" && checksum != expected {
		return models.AssetArtifact{}, fmt.Errorf(
			"%w: asset %q digest does not match", models.ErrAssetIntegrityFailed, requirement.Name,
		)
	}
	return models.AssetArtifact{Name: requirement.Name, Bytes: written, SHA256: checksum}, nil
}

func (s *service) writeGenericMetadata(
	path string,
	kind string,
	identity string,
	source string,
	sourceKey string,
	artifacts []genericArtifact,
) error {
	metadata := genericCacheMetadata{
		Kind: kind, Identity: identity, Source: source, SourceKey: sourceKey,
	}
	metadata.Artifacts = make([]models.AssetRequirement, 0, len(artifacts))
	for _, artifact := range artifacts {
		metadata.Artifacts = append(metadata.Artifacts, artifact.requirement)
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return s.writeFile(path, body, 0o644)
}

func (s *service) removeTree(path string) error {
	info, err := s.inspectPath(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return s.removePath(path)
	}
	entries, err := s.readDirectory(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := s.removeTree(filepath.Join(path, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return s.removePath(path)
}
