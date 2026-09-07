package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
			return genericCacheResult{}, assetContextError(ctx)
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
) (result genericCacheResult, err error) {
	if source.kind == genericSourceHF && genericArtifactsNeedManifest(artifacts) {
		if offline {
			return genericCacheResult{}, &models.AssetOfflineError{
				Missing: []string{source.repository},
			}
		}
		manifest, err := s.fetchGenericManifest(ctx, source)
		if err != nil {
			return genericCacheResult{}, err
		}
		artifacts, err = mergeGenericManifest(artifacts, manifest)
		if err != nil {
			return genericCacheResult{}, err
		}
	}
	s.addGenericURLs(source, artifacts)
	if err := assetContextError(ctx); err != nil {
		return genericCacheResult{}, err
	}
	lock, lockErr := s.lockGenericCache(ctx, kind, source, artifacts, roots)
	if lockErr != nil {
		return genericCacheResult{}, lockErr
	}
	defer func() {
		err = closeAssetStagingLock(lock, err)
	}()

	cached, missing, inspectErr := s.inspectGenericCache(ctx, kind, source, artifacts, roots)
	if inspectErr != nil {
		return cacheResultFromPaths(artifactKind, artifacts, cached), inspectErr
	}
	if len(missing) == 0 {
		cached, err = s.repairLegacyGenericCache(ctx, kind, source, artifacts, cached, roots)
		if err != nil {
			return cacheResultFromPaths(artifactKind, artifacts, cached), err
		}
		return cacheResultFromPaths(artifactKind, artifacts, cached), nil
	}
	if offline {
		return cacheResultFromPaths(artifactKind, artifacts, cached), &models.AssetOfflineError{
			Missing: missingArtifactNames(missing),
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
	if len(missing) > 0 {
		if legacy, found, err := s.inspectLegacyGenericCache(ctx, kind, source, artifacts, roots); err != nil {
			return cached, artifacts, err
		} else if found {
			// A complete, verified legacy snapshot is safer than combining a
			// partial higher-priority cache with a missing artifact. The repair
			// still runs only against the scope-owned root under the lock.
			return legacy, nil, nil
		}
	}
	return cached, missing, nil
}

func (s *service) inspectLegacyGenericCache(
	ctx context.Context,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
	roots []string,
) (map[string]genericCachePath, bool, error) {
	if len(roots) == 0 || !genericArtifactsHaveTrustedFacts(artifacts) {
		return nil, false, nil
	}
	ownedRoot := roots[len(roots)-1]
	records := s.discoverContentAddressedRecords(ownedRoot, kind, source)
	var candidate *genericCacheRecord
	for index := range records {
		record := records[index]
		if !isRepairableGenericCacheRecord(record, kind, source, artifacts) {
			continue
		}
		if candidate != nil {
			// Multiple stale records for the same source and artifact set do
			// not identify a unique owner. Leave them untouched.
			return nil, false, nil
		}
		candidate = &records[index]
	}
	if candidate == nil {
		return nil, false, nil
	}

	cached := make(map[string]genericCachePath, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.Join(candidate.snapshotPath, filepath.FromSlash(artifact.requirement.Name))
		found, ok, err := s.verifyGenericCachedArtifact(ctx, path, artifact.requirement)
		if err != nil {
			return cached, false, err
		}
		if !ok {
			return nil, false, nil
		}
		found.legacySnapshotPath = candidate.snapshotPath
		found.legacyMetadataPath = candidate.metadataPath
		found.legacyRoot = candidate.root
		found.legacyMetadata = candidate.metadata
		cached[artifact.requirement.Name] = found
	}
	return cached, true, nil
}

func (s *service) verifyGenericCachedArtifact(
	ctx context.Context,
	path string,
	requirement models.AssetRequirement,
) (genericCachePath, bool, error) {
	info, err := s.inspectPath(path)
	if err != nil || info == nil || !info.Mode().IsRegular() {
		return genericCachePath{}, false, nil
	}
	actual, err := s.fileSHA256(ctx, path)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return genericCachePath{}, false, contextErr
		}
		return genericCachePath{}, false, nil
	}
	if requirement.Bytes <= 0 || info.Size() != requirement.Bytes {
		return genericCachePath{}, false, nil
	}
	expected := strings.ToLower(strings.TrimSpace(requirement.SHA256))
	if expected == "" || actual != expected {
		return genericCachePath{}, false, nil
	}
	return genericCachePath{
		artifact: models.AssetArtifact{
			Name: requirement.Name, Bytes: info.Size(), SHA256: actual,
		},
		path: path,
	}, true, nil
}

func genericArtifactsHaveTrustedFacts(artifacts []genericArtifact) bool {
	if len(artifacts) == 0 {
		return false
	}
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.requirement.Name) == "" || artifact.requirement.Bytes <= 0 ||
			strings.TrimSpace(artifact.requirement.SHA256) == "" {
			return false
		}
	}
	return true
}

func isRepairableGenericCacheRecord(
	record genericCacheRecord,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
) bool {
	metadata := record.metadata
	if !genericCacheMetadataNeedsRepair(metadata) || metadata.Kind != kind ||
		metadata.Source != source.safe || metadata.SourceKey != genericSourceIdentity(source) ||
		len(metadata.Artifacts) != len(artifacts) || len(metadata.Artifacts) == 0 {
		return false
	}
	metadataArtifacts := genericArtifactsFromRequirements(metadata.Artifacts)
	requested := genericArtifactRequirements(artifacts)
	if genericCacheKey(kind, source, metadataArtifacts) != metadata.Identity ||
		genericArtifactIdentityHash(kind, source, metadataArtifacts) != filepath.Base(record.snapshotPath) ||
		!sameGenericArtifactNames(metadata.Artifacts, requested) ||
		!legacyMetadataFactsAgree(metadata.Artifacts, requested) {
		return false
	}
	return true
}

func genericCacheMetadataNeedsRepair(metadata genericCacheMetadata) bool {
	if len(metadata.Artifacts) == 0 {
		return false
	}
	for _, artifact := range metadata.Artifacts {
		if artifact.Bytes < 0 {
			return false
		}
		if artifact.Bytes == 0 || strings.TrimSpace(artifact.SHA256) == "" {
			return true
		}
	}
	return false
}

func genericArtifactsFromRequirements(requirements []models.AssetRequirement) []genericArtifact {
	artifacts := make([]genericArtifact, 0, len(requirements))
	for _, requirement := range requirements {
		artifacts = append(artifacts, genericArtifact{requirement: requirement})
	}
	return artifacts
}

func genericArtifactRequirements(artifacts []genericArtifact) []models.AssetRequirement {
	requirements := make([]models.AssetRequirement, 0, len(artifacts))
	for _, artifact := range artifacts {
		requirements = append(requirements, artifact.requirement)
	}
	return requirements
}

func sameGenericArtifactNames(first, second []models.AssetRequirement) bool {
	if len(first) != len(second) {
		return false
	}
	seen := make(map[string]struct{}, len(first))
	for _, artifact := range first {
		name := filepath.ToSlash(strings.TrimSpace(artifact.Name))
		if !validGenericArtifactName(name) {
			return false
		}
		if _, ok := seen[name]; ok {
			return false
		}
		seen[name] = struct{}{}
	}
	for _, artifact := range second {
		name := filepath.ToSlash(strings.TrimSpace(artifact.Name))
		if !validGenericArtifactName(name) {
			return false
		}
		if _, ok := seen[name]; !ok {
			return false
		}
		delete(seen, name)
	}
	return len(seen) == 0
}

func legacyMetadataFactsAgree(
	metadata []models.AssetRequirement,
	requested []models.AssetRequirement,
) bool {
	requestedByName := make(map[string]models.AssetRequirement, len(requested))
	for _, artifact := range requested {
		requestedByName[filepath.ToSlash(strings.TrimSpace(artifact.Name))] = artifact
	}
	for _, artifact := range metadata {
		name := filepath.ToSlash(strings.TrimSpace(artifact.Name))
		requirement, ok := requestedByName[name]
		if !ok {
			return false
		}
		// A non-zero legacy fact is retained only when it agrees with the
		// caller's complete requirement. Zero and blank values are the sole
		// facts this compatibility path is allowed to fill.
		if artifact.Bytes < 0 || (artifact.Bytes > 0 && artifact.Bytes != requirement.Bytes) {
			return false
		}
		if digest := strings.TrimSpace(artifact.SHA256); digest != "" &&
			!strings.EqualFold(digest, strings.TrimSpace(requirement.SHA256)) {
			return false
		}
	}
	return true
}

func validGenericArtifactName(name string) bool {
	raw := strings.TrimSpace(name)
	if raw == "" || strings.ContainsAny(raw, "\\\x00") {
		return false
	}
	pathName := filepath.FromSlash(raw)
	if filepath.IsAbs(pathName) || filepath.VolumeName(pathName) != "" {
		return false
	}
	name = filepath.ToSlash(raw)
	return name != "." && name != ".." && !strings.HasPrefix(name, "/") &&
		filepath.ToSlash(filepath.Clean(filepath.FromSlash(name))) == name
}

func (s *service) repairLegacyGenericCache(
	ctx context.Context,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
	cached map[string]genericCachePath,
	roots []string,
) (map[string]genericCachePath, error) {
	record, ok := legacyGenericCacheRecord(cached, artifacts)
	if !ok || !genericArtifactsHaveTrustedFacts(artifacts) || len(roots) == 0 ||
		filepath.Clean(record.root) != filepath.Clean(roots[len(roots)-1]) {
		return cached, nil
	}
	if !isRepairableGenericCacheRecord(record, kind, source, artifacts) {
		return cached, nil
	}
	observed, err := s.reverifyLegacyGenericArtifacts(ctx, record, artifacts)
	if err != nil {
		return cached, err
	}
	if err := assetContextError(ctx); err != nil {
		return cached, err
	}
	identity := genericArtifactIdentityHash(kind, source, observed)
	finalPath := filepath.Join(filepath.Dir(record.snapshotPath), identity)
	if filepath.Clean(finalPath) == filepath.Clean(record.snapshotPath) {
		return cached, fmt.Errorf(
			"%w: legacy cache identity is not incomplete", models.ErrAssetIntegrityFailed,
		)
	}
	if err := s.ensureLegacyRepairDestinationAbsent(finalPath); err != nil {
		return cached, err
	}
	if err := assetContextError(ctx); err != nil {
		return cached, err
	}
	backupPath, hadMetadata, err := s.replaceLegacyGenericMetadata(
		record, kind, source, observed,
	)
	if err != nil {
		return cached, err
	}
	if err := assetContextError(ctx); err != nil {
		restoreErr := s.restoreLegacyMetadata(record.metadataPath, backupPath, hadMetadata)
		return cached, joinLegacyRepairErrors(err, restoreErr)
	}
	if err := s.renamePath(record.snapshotPath, finalPath); err != nil {
		restoreErr := s.restoreLegacyMetadata(record.metadataPath, backupPath, hadMetadata)
		return cached, joinLegacyRepairErrors(
			legacyRepairInterruption("move legacy cache snapshot", err), restoreErr,
		)
	}
	if err := assetContextError(ctx); err != nil {
		rollbackErr := s.rollbackLegacyRepair(finalPath, record.snapshotPath, backupPath, hadMetadata)
		return cached, joinLegacyRepairErrors(err, rollbackErr)
	}

	repaired, err := s.reverifyRepairedGenericArtifacts(ctx, finalPath, observed)
	if err != nil {
		rollbackErr := s.rollbackLegacyRepair(finalPath, record.snapshotPath, backupPath, hadMetadata)
		return cached, joinLegacyRepairErrors(err, rollbackErr)
	}
	if err := assetContextError(ctx); err != nil {
		rollbackErr := s.rollbackLegacyRepair(finalPath, record.snapshotPath, backupPath, hadMetadata)
		return cached, joinLegacyRepairErrors(err, rollbackErr)
	}
	if err := s.removeLegacyMetadataBackup(finalPath, backupPath, hadMetadata); err != nil {
		return repaired, err
	}
	return repaired, nil
}

func (s *service) replaceLegacyGenericMetadata(
	record genericCacheRecord,
	kind string,
	source genericSource,
	observed []genericArtifact,
) (string, bool, error) {
	metadataStagePath := record.metadataPath + ".partial"
	if err := s.removeTree(metadataStagePath); err != nil {
		return "", false, legacyRepairInterruption("clear legacy metadata staging", err)
	}
	if err := s.writeGenericMetadata(
		metadataStagePath, kind, genericCacheKey(kind, source, observed), source.safe,
		genericSourceIdentity(source), observed,
	); err != nil {
		cleanupErr := s.removeTree(metadataStagePath)
		return "", false, joinLegacyRepairErrors(
			legacyRepairInterruption("stage legacy cache metadata", err), cleanupErr,
		)
	}
	backupPath, hadMetadata, err := s.moveExistingGenericMetadata(record.metadataPath)
	if err != nil {
		cleanupErr := s.removeTree(metadataStagePath)
		return "", false, joinLegacyRepairErrors(
			legacyRepairInterruption("backup legacy cache metadata", err), cleanupErr,
		)
	}
	if err := s.renamePath(metadataStagePath, record.metadataPath); err != nil {
		restoreErr := s.restoreLegacyMetadata(record.metadataPath, backupPath, hadMetadata)
		cleanupErr := s.removeTree(metadataStagePath)
		return "", false, joinLegacyRepairErrors(
			legacyRepairInterruption("replace legacy cache metadata", err),
			joinLegacyRepairErrors(restoreErr, cleanupErr),
		)
	}
	return backupPath, hadMetadata, nil
}

func (s *service) removeLegacyMetadataBackup(path, backupPath string, hadMetadata bool) error {
	if !hadMetadata {
		return nil
	}
	backup := filepath.Join(path, filepath.Base(backupPath))
	if err := s.removePath(backup); err != nil {
		return legacyRepairInterruption("clean legacy cache metadata backup", err)
	}
	return nil
}

func legacyGenericCacheRecord(
	cached map[string]genericCachePath,
	artifacts []genericArtifact,
) (genericCacheRecord, bool) {
	var record genericCacheRecord
	for _, artifact := range artifacts {
		found, ok := cached[artifact.requirement.Name]
		if !ok || found.legacySnapshotPath == "" || found.legacyMetadataPath == "" {
			return genericCacheRecord{}, false
		}
		if record.snapshotPath == "" {
			record = genericCacheRecord{
				root:         found.legacyRoot,
				snapshotPath: found.legacySnapshotPath,
				metadataPath: found.legacyMetadataPath,
				metadata:     found.legacyMetadata,
			}
			continue
		}
		if record.snapshotPath != found.legacySnapshotPath || record.metadataPath != found.legacyMetadataPath {
			return genericCacheRecord{}, false
		}
		if record.root != found.legacyRoot {
			return genericCacheRecord{}, false
		}
	}
	return record, record.snapshotPath != ""
}

func (s *service) reverifyLegacyGenericArtifacts(
	ctx context.Context,
	record genericCacheRecord,
	artifacts []genericArtifact,
) ([]genericArtifact, error) {
	observed := make([]genericArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.Join(record.snapshotPath, filepath.FromSlash(artifact.requirement.Name))
		found, ok, err := s.verifyGenericCachedArtifact(ctx, path, artifact.requirement)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf(
				"%w: legacy asset %q changed before repair", models.ErrAssetIntegrityFailed,
				artifact.requirement.Name,
			)
		}
		observed = append(observed, genericArtifact{
			requirement: models.AssetRequirement{
				Name:   artifact.requirement.Name,
				Bytes:  found.artifact.Bytes,
				SHA256: found.artifact.SHA256,
			},
			metadataResolved: true,
		})
	}
	return observed, nil
}

func (s *service) ensureLegacyRepairDestinationAbsent(path string) error {
	info, err := s.inspectPath(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return legacyRepairInterruption("inspect legacy cache destination", err)
	}
	if info == nil {
		return legacyRepairInterruption("inspect legacy cache destination", errors.New("destination info unavailable"))
	}
	return fmt.Errorf("%w: legacy cache identity collision", models.ErrAssetIntegrityFailed)
}

func (s *service) reverifyRepairedGenericArtifacts(
	ctx context.Context,
	root string,
	artifacts []genericArtifact,
) (map[string]genericCachePath, error) {
	repaired := make(map[string]genericCachePath, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(artifact.requirement.Name))
		found, ok, err := s.verifyGenericCachedArtifact(ctx, path, artifact.requirement)
		if err != nil {
			return repaired, err
		}
		if !ok {
			return repaired, legacyRepairInterruption(
				"validate repaired legacy cache artifact", errors.New("verified file changed"),
			)
		}
		repaired[artifact.requirement.Name] = found
	}
	return repaired, nil
}

func (s *service) rollbackLegacyRepair(
	currentPath string,
	legacyPath string,
	backupPath string,
	hadMetadata bool,
) error {
	if err := s.renamePath(currentPath, legacyPath); err != nil {
		return legacyRepairInterruption("restore legacy cache snapshot", err)
	}
	return s.restoreLegacyMetadata(filepath.Join(legacyPath, assetMetadataName), backupPath, hadMetadata)
}

func (s *service) restoreLegacyMetadata(path, backupPath string, hadMetadata bool) error {
	if !hadMetadata {
		return nil
	}
	if err := s.removePath(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return legacyRepairInterruption("remove replacement legacy metadata", err)
	}
	if err := s.renamePath(backupPath, path); err != nil {
		return legacyRepairInterruption("restore legacy cache metadata", err)
	}
	return nil
}

func legacyRepairInterruption(operation string, cause error) error {
	return pullsupport.WrapPullStage(
		models.PullStageCacheInstallation, "", operation, "",
		interruptedAssetError(operation, cause),
	)
}

func joinLegacyRepairErrors(primary, cleanup error) error {
	if primary == nil {
		return cleanup
	}
	if cleanup == nil {
		return primary
	}
	return errors.Join(primary, legacyRepairInterruption("clean legacy cache repair", cleanup))
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
	stagingIdentityName := genericArtifactIdentityHash(kind, source, artifacts)
	base := filepath.Join(destinationRoot, assetContentDirectory, kind)
	stagePath := filepath.Join(base, stagingIdentityName+".partial")
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
	// Requested metadata may be incomplete; only verified staged results are
	// allowed to define the durable content-addressed identity and record.
	observed, err := observedGenericArtifacts(artifacts, published)
	if err != nil {
		return cacheResultFromPaths(artifactKind, artifacts, published), err
	}
	identity := genericCacheKey(kind, source, observed)
	identityName := genericArtifactIdentityHash(kind, source, observed)
	finalPath := filepath.Join(base, identityName)
	if err := s.writeGenericMetadata(
		filepath.Join(stagePath, assetMetadataName), kind, identity, source.safe,
		genericSourceIdentity(source), observed,
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

	result, err := s.committedGenericCacheResult(artifactKind, observed, published, finalPath)
	if err != nil {
		return genericCacheResult{}, pullsupport.WrapPullStage(
			models.PullStageCacheInstallation, "", "validate committed asset snapshot", "", err,
		)
	}
	if hadExisting {
		if err := s.removeTree(backupPath); err != nil {
			return result, pullsupport.WrapPullStage(
				models.PullStageCacheInstallation, "", "clean replaced asset snapshot", "",
				interruptedAssetError("clean replaced asset snapshot", err),
			)
		}
	}
	result.prepared = true
	return result, nil
}

func observedGenericArtifacts(
	requested []genericArtifact,
	published map[string]genericCachePath,
) ([]genericArtifact, error) {
	observed := make([]genericArtifact, 0, len(requested))
	for _, artifact := range requested {
		found, ok := published[artifact.requirement.Name]
		if !ok {
			return nil, fmt.Errorf(
				"%w: verified asset %q has no observed result",
				models.ErrAssetIntegrityFailed, artifact.requirement.Name,
			)
		}
		if found.artifact.Bytes < 0 || strings.TrimSpace(found.artifact.SHA256) == "" {
			return nil, fmt.Errorf(
				"%w: verified asset %q has incomplete observed identity",
				models.ErrAssetIntegrityFailed, artifact.requirement.Name,
			)
		}
		observed = append(observed, genericArtifact{
			requirement: models.AssetRequirement{
				Name:   artifact.requirement.Name,
				Bytes:  found.artifact.Bytes,
				SHA256: strings.ToLower(strings.TrimSpace(found.artifact.SHA256)),
			},
			url:              artifact.url,
			localPath:        artifact.localPath,
			metadataResolved: true,
		})
	}
	return observed, nil
}

func (s *service) committedGenericCacheResult(
	kind models.AssetArtifactKind,
	artifacts []genericArtifact,
	published map[string]genericCachePath,
	finalPath string,
) (genericCacheResult, error) {
	absoluteRoot, err := filepath.Abs(finalPath)
	if err != nil {
		return genericCacheResult{}, interruptedAssetError("resolve committed asset snapshot", err)
	}
	rebased := make(map[string]genericCachePath, len(published))
	for name, artifact := range published {
		artifact.path = filepath.Join(absoluteRoot, filepath.FromSlash(name))
		info, statErr := s.inspectPath(artifact.path)
		if statErr != nil {
			return genericCacheResult{}, interruptedAssetError("validate committed asset file", statErr)
		}
		if info == nil || !info.Mode().IsRegular() {
			return genericCacheResult{}, fmt.Errorf("committed asset is not a regular file")
		}
		rebased[name] = artifact
	}
	result := cacheResultFromPaths(kind, artifacts, rebased)
	result.snapshotPath = absoluteRoot
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

func (s *service) lockGenericCache(
	ctx context.Context,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
	roots []string,
) (io.Closer, error) {
	if len(roots) == 0 {
		return nil, nil
	}
	identity := genericArtifactIdentityHash(kind, source, artifacts)
	lockPath := filepath.Join(
		roots[len(roots)-1], ".you-asset-locks", kind, identity+".lock",
	)
	return s.lockAssetStaging(ctx, lockPath)
}

func (s *service) lockGenericRuntime(
	ctx context.Context,
	cacheDirectory string,
	modelName string,
) (io.Closer, error) {
	root, err := s.modelCacheRoot(cacheDirectory, canonicalModelName(modelName))
	if err != nil {
		return nil, err
	}
	identity := sha256.Sum256([]byte(canonicalModelName(modelName)))
	return s.lockAssetStaging(
		ctx,
		filepath.Join(filepath.Dir(root), ".you-asset-locks", "runtime", hex.EncodeToString(identity[:])+".lock"),
	)
}

func (s *service) lockAssetStaging(ctx context.Context, path string) (io.Closer, error) {
	if s == nil || s.coordination == nil {
		return nil, pullsupport.WrapPullStage(
			models.PullStageCacheInstallation, "", "acquire asset staging ownership", "",
			interruptedAssetError("acquire asset staging ownership", errors.New("coordination is unavailable")),
		)
	}
	lock, err := s.coordination.Lock(ctx, path)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return nil, contextErr
		}
		return nil, pullsupport.WrapPullStage(
			models.PullStageCacheInstallation, "", "acquire asset staging ownership", "",
			interruptedAssetError("acquire asset staging ownership", err),
		)
	}
	if err := assetContextError(ctx); err != nil {
		_ = lock.Close()
		return nil, err
	}
	return lock, nil
}

func closeAssetStagingLock(lock io.Closer, primary error) error {
	if lock == nil {
		return primary
	}
	if err := lock.Close(); err != nil {
		closeErr := pullsupport.WrapPullStage(
			models.PullStageCacheInstallation, "", "release asset staging ownership", "",
			interruptedAssetError("release asset staging ownership", err),
		)
		return errors.Join(primary, closeErr)
	}
	return primary
}
