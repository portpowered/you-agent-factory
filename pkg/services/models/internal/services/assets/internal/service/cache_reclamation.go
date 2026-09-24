package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

type cacheReclamationCandidate struct {
	path         string
	relativePath string
	kind         string
	identity     string
	sourceKey    string
	artifacts    []models.AssetRequirement
	bytes        int64
	shared       bool
}

type cacheReclamationPlan struct {
	candidates []cacheReclamationCandidate
	retained   int64
}

const cacheReferenceLockName = "references.lock"

// lockCacheReferenceUpdates serializes durable managed-cache reference
// publication with a reclamation census and its subsequent deletions. Cache
// content acquisition uses separate source locks; callers take those locks
// for every candidate before applying a reclamation plan.
func (s *service) lockCacheReferenceUpdates(ctx context.Context, cacheRoot string) (io.Closer, error) {
	if s == nil || s.coordination == nil {
		return nil, errors.New("cache reference coordination is unavailable")
	}
	return s.coordination.Lock(ctx, filepath.Join(cacheRoot, ".you-asset-locks", cacheReferenceLockName))
}

func (s *service) planAndLockCacheReclamationCandidates(
	ctx context.Context,
	cacheRoot string,
	modelRoot string,
	modelName string,
	metadata cacheMetadata,
	source genericSource,
) (cacheReclamationPlan, []io.Closer, error) {
	locks := make([]io.Closer, 0, 2)
	lockedPaths := make(map[string]struct{}, 2)
	for {
		plan, err := s.planCacheReclamation(ctx, cacheRoot, modelRoot, modelName, metadata, source)
		if err != nil {
			return cacheReclamationPlan{}, locks, err
		}
		lockPaths := make([]string, 0, len(plan.candidates))
		for _, candidate := range plan.candidates {
			path := cacheCandidateSourceLockPath(cacheRoot, candidate)
			if _, locked := lockedPaths[path]; !locked {
				lockPaths = append(lockPaths, path)
			}
		}
		if len(lockPaths) == 0 {
			return plan, locks, nil
		}
		sort.Strings(lockPaths)
		for _, path := range lockPaths {
			lock, lockErr := s.lockCacheReferencePath(ctx, path)
			if lockErr != nil {
				return cacheReclamationPlan{}, locks, lockErr
			}
			locks = append(locks, lock)
			lockedPaths[path] = struct{}{}
		}
	}
}

func cacheCandidateSourceLockPath(cacheRoot string, candidate cacheReclamationCandidate) string {
	lockRoot := cacheRoot
	if candidate.kind == assetKindBackend {
		lockRoot = filepath.Join(lockRoot, "backend-artifacts")
	}
	identity := sha256.Sum256([]byte(candidate.kind + "|" + candidate.sourceKey))
	return filepath.Join(
		lockRoot, ".you-asset-locks", candidate.kind,
		hex.EncodeToString(identity[:])+".lock",
	)
}

func (s *service) lockCacheReferencePath(ctx context.Context, path string) (io.Closer, error) {
	if s == nil || s.coordination == nil {
		return nil, errors.New("cache candidate coordination is unavailable")
	}
	return s.coordination.Lock(ctx, path)
}

func closeCacheReclamationLocks(locks []io.Closer, primary error) error {
	for index := len(locks) - 1; index >= 0; index-- {
		if locks[index] == nil {
			continue
		}
		if err := locks[index].Close(); err != nil {
			primary = errors.Join(primary, fmt.Errorf("%w: coordinated cache removal could not release ownership", models.ErrModelCacheRemovalFailed))
		}
	}
	return primary
}

func (s *service) planCacheReclamation(
	ctx context.Context,
	cacheRoot string,
	modelRoot string,
	modelName string,
	metadata cacheMetadata,
	source genericSource,
) (cacheReclamationPlan, error) {
	plan := cacheReclamationPlan{}
	modelRequirements, err := cacheRequirements(metadata.Files)
	if err != nil {
		return plan, uncertainCacheReferences(modelName, err)
	}
	if len(modelRequirements) == 0 {
		return plan, uncertainCacheReferences(modelName, nil)
	}
	if len(modelRequirements) > 0 {
		modelArtifacts := s.genericArtifactsFromRequirements(source, modelRequirements)
		identity := genericArtifactIdentityHash(assetKindModel, source, modelArtifacts)
		candidate, present, candidateErr := s.inspectReclaimableSnapshot(
			ctx, cacheRoot, assetKindModel, identity, genericCacheKey(assetKindModel, source, modelArtifacts),
			genericSourceIdentity(source), modelRequirements,
		)
		if candidateErr != nil {
			return plan, uncertainCacheReferences(modelName, candidateErr)
		}
		if present {
			candidate.relativePath = filepath.ToSlash(filepath.Join(assetContentDirectory, assetKindModel, identity))
			plan.candidates = append(plan.candidates, candidate)
		}
	}
	if metadata.Backend != nil {
		candidate, present, candidateErr := s.inspectReclaimableBackendSnapshot(
			ctx, cacheRoot, metadata.Backend,
		)
		if candidateErr != nil {
			return plan, uncertainCacheReferences(modelName, candidateErr)
		}
		if present {
			plan.candidates = append(plan.candidates, candidate)
		}
	}
	if len(plan.candidates) == 0 {
		return plan, nil
	}
	if err := s.markSharedReclamationCandidates(ctx, cacheRoot, modelRoot, plan.candidates); err != nil {
		return cacheReclamationPlan{}, uncertainCacheReferences(modelName, err)
	}
	for _, candidate := range plan.candidates {
		if candidate.shared {
			plan.retained, err = addManagedCacheBytes(plan.retained, candidate.bytes)
			if err != nil {
				return cacheReclamationPlan{}, err
			}
		}
	}
	return plan, nil
}

func (s *service) inspectReclaimableSnapshot(
	ctx context.Context,
	cacheRoot string,
	kind string,
	identity string,
	wantIdentity string,
	wantSourceKey string,
	artifacts []models.AssetRequirement,
) (cacheReclamationCandidate, bool, error) {
	kindRoot := filepath.Join(cacheRoot, assetContentDirectory, kind)
	present, err := s.managedDirectoryPresent(ctx, cacheRoot, assetContentDirectory, "content-addressed cache")
	if err != nil || !present {
		return cacheReclamationCandidate{}, false, err
	}
	present, err = s.managedDirectoryPresent(ctx, filepath.Join(cacheRoot, assetContentDirectory), kind, "content-addressed cache kind")
	if err != nil || !present {
		return cacheReclamationCandidate{}, false, err
	}
	present, err = s.managedDirectoryPresent(ctx, kindRoot, identity, "content-addressed snapshot")
	if err != nil || !present {
		return cacheReclamationCandidate{}, false, err
	}
	snapshotPath := filepath.Join(kindRoot, identity)
	metadata, found, err := s.readGenericCacheMetadata(ctx, snapshotPath)
	if err != nil {
		return cacheReclamationCandidate{}, false, err
	}
	if !found || metadata.Kind != kind || metadata.Identity != wantIdentity ||
		metadata.SourceKey != wantSourceKey || !sameAssetRequirements(metadata.Artifacts, artifacts) {
		return cacheReclamationCandidate{}, false, fmt.Errorf("content-addressed snapshot metadata does not match its managed reference")
	}
	if err := s.verifyReclamationArtifacts(ctx, snapshotPath, artifacts); err != nil {
		return cacheReclamationCandidate{}, false, err
	}
	bytes, err := s.measureDirectoryBytes(ctx, snapshotPath)
	if err != nil {
		return cacheReclamationCandidate{}, false, err
	}
	return cacheReclamationCandidate{
		path: snapshotPath, kind: kind, identity: identity,
		sourceKey: wantSourceKey, artifacts: cloneAssetRequirements(artifacts), bytes: bytes,
	}, true, nil
}

func (s *service) inspectReclaimableBackendSnapshot(
	ctx context.Context,
	cacheRoot string,
	backend *runtimeBackendMetadata,
) (cacheReclamationCandidate, bool, error) {
	relative := strings.TrimSpace(backend.CachePath)
	if !validBackendCacheReferencePath(relative) {
		return cacheReclamationCandidate{}, false, fmt.Errorf("backend cache path is not a YOU-owned content-addressed path")
	}
	identity := filepath.Base(filepath.FromSlash(relative))
	if len(identity) != 64 {
		return cacheReclamationCandidate{}, false, fmt.Errorf("backend cache identity is invalid")
	}
	if _, err := hex.DecodeString(identity); err != nil {
		return cacheReclamationCandidate{}, false, fmt.Errorf("backend cache identity is invalid")
	}
	backendRoot := filepath.Join(cacheRoot, "backend-artifacts")
	present, err := s.managedDirectoryPresent(ctx, cacheRoot, "backend-artifacts", "backend cache")
	if err != nil || !present {
		return cacheReclamationCandidate{}, false, err
	}
	present, err = s.managedDirectoryPresent(ctx, backendRoot, assetContentDirectory, "backend content-addressed cache")
	if err != nil || !present {
		return cacheReclamationCandidate{}, false, err
	}
	present, err = s.managedDirectoryPresent(ctx, filepath.Join(backendRoot, assetContentDirectory), assetKindBackend, "backend content-addressed cache kind")
	if err != nil || !present {
		return cacheReclamationCandidate{}, false, err
	}
	backendKindRoot := filepath.Join(backendRoot, assetContentDirectory, assetKindBackend)
	present, err = s.managedDirectoryPresent(ctx, backendKindRoot, identity, "backend content-addressed snapshot")
	if err != nil || !present {
		return cacheReclamationCandidate{}, false, err
	}
	snapshotPath := filepath.Join(backendKindRoot, identity)
	metadata, found, err := s.readGenericCacheMetadata(ctx, snapshotPath)
	if err != nil {
		return cacheReclamationCandidate{}, false, err
	}
	artifacts, err := cacheRequirements(backend.Files)
	if err != nil {
		return cacheReclamationCandidate{}, false, err
	}
	if !found || metadata.Kind != assetKindBackend || len(artifacts) == 0 ||
		!sameAssetRequirements(metadata.Artifacts, artifacts) || metadata.Source == "" || metadata.SourceKey == "" ||
		!validContentAddressedIdentity(metadata, identity) {
		return cacheReclamationCandidate{}, false, fmt.Errorf("backend snapshot metadata does not prove ownership")
	}
	if err := s.verifyReclamationArtifacts(ctx, snapshotPath, artifacts); err != nil {
		return cacheReclamationCandidate{}, false, err
	}
	bytes, err := s.measureDirectoryBytes(ctx, snapshotPath)
	if err != nil {
		return cacheReclamationCandidate{}, false, err
	}
	return cacheReclamationCandidate{
		path: snapshotPath, relativePath: filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative))),
		kind: assetKindBackend, identity: identity, sourceKey: metadata.SourceKey,
		artifacts: artifacts, bytes: bytes,
	}, true, nil
}

func (s *service) managedDirectoryPresent(
	ctx context.Context,
	parent string,
	child string,
	kind string,
) (bool, error) {
	if err := assetContextError(ctx); err != nil {
		return false, err
	}
	entries, err := s.readDirectory(parent)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry == nil || entry.Name() != child {
			continue
		}
		if err := s.validateManagedDirectoryChild(entry, child, kind); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s *service) readGenericCacheMetadata(
	ctx context.Context,
	snapshotPath string,
) (genericCacheMetadata, bool, error) {
	body, found, err := s.readManagedRegularFile(ctx, snapshotPath, assetMetadataName)
	if err != nil || !found {
		return genericCacheMetadata{}, found, err
	}
	var metadata genericCacheMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return genericCacheMetadata{}, true, fmt.Errorf("decode content-addressed cache metadata: %w", err)
	}
	return metadata, true, nil
}

func (s *service) readManagedRegularFile(
	ctx context.Context,
	parent string,
	name string,
) ([]byte, bool, error) {
	if err := assetContextError(ctx); err != nil {
		return nil, false, err
	}
	entries, err := s.readDirectory(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	for _, entry := range entries {
		if entry == nil || entry.Name() != name {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, true, fmt.Errorf("managed metadata is a symlink")
		}
		info, err := entry.Info()
		if err != nil {
			return nil, true, err
		}
		if info == nil || !info.Mode().IsRegular() {
			return nil, true, fmt.Errorf("managed metadata is not a regular file")
		}
		body, err := s.readFile(filepath.Join(parent, name))
		if err != nil {
			return nil, true, err
		}
		return body, true, nil
	}
	return nil, false, nil
}

func (s *service) verifyReclamationArtifacts(
	ctx context.Context,
	root string,
	artifacts []models.AssetRequirement,
) error {
	for _, artifact := range artifacts {
		if err := assetContextError(ctx); err != nil {
			return err
		}
		if !validGenericRuntimeRelativePath(artifact.Name) {
			return fmt.Errorf("content-addressed artifact path is invalid")
		}
		if err := s.requireManagedArtifactPath(ctx, root, artifact.Name); err != nil {
			return err
		}
		path := filepath.Join(root, filepath.FromSlash(artifact.Name))
		verified, ok, err := s.verifyGenericCachedArtifact(ctx, path, artifact)
		if err != nil || !ok {
			return fmt.Errorf("content-addressed artifact is not verified: %s", artifact.Name)
		}
		_ = verified
	}
	return nil
}

func (s *service) requireManagedArtifactPath(ctx context.Context, root, relative string) error {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	parent := root
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("content-addressed artifact path is invalid")
		}
		entries, err := s.readDirectory(parent)
		if err != nil {
			return err
		}
		found := false
		for _, entry := range entries {
			if entry == nil || entry.Name() != part {
				continue
			}
			found = true
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("content-addressed artifact path contains a symlink")
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if index < len(parts)-1 && !info.IsDir() {
				return fmt.Errorf("content-addressed artifact parent is not a directory")
			}
			if index == len(parts)-1 && !info.Mode().IsRegular() {
				return fmt.Errorf("content-addressed artifact is not a regular file")
			}
			break
		}
		if !found {
			return os.ErrNotExist
		}
		parent = filepath.Join(parent, part)
	}
	return nil
}

func (s *service) markSharedReclamationCandidates(
	ctx context.Context,
	cacheRoot string,
	selectedModelRoot string,
	candidates []cacheReclamationCandidate,
) error {
	entries, err := s.readDirectory(cacheRoot)
	if err != nil {
		return err
	}
	selectedModelRoot = filepath.Clean(selectedModelRoot)
	for _, entry := range entries {
		if err := assetContextError(ctx); err != nil {
			return err
		}
		if entry == nil || entry.Name() == assetContentDirectory || entry.Name() == "backend-artifacts" ||
			entry.Name() == ".you-asset-locks" {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed model root is a symlink")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() {
			continue
		}
		modelRoot := filepath.Join(cacheRoot, entry.Name())
		if filepath.Clean(modelRoot) == selectedModelRoot {
			continue
		}
		body, found, err := s.readManagedRegularFile(ctx, modelRoot, metadataFileName)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		var metadata cacheMetadata
		if err := json.Unmarshal(body, &metadata); err != nil {
			return fmt.Errorf("decode managed model reference: %w", err)
		}
		otherName := strings.TrimSpace(metadata.ModelName)
		if otherName == "" {
			otherName = entry.Name()
		}
		for index := range candidates {
			sharedBackend, refErr := candidateReferencesBackend(candidates[index], metadata.Backend)
			if refErr != nil {
				return refErr
			}
			if sharedBackend {
				candidates[index].shared = true
			}
			if candidates[index].kind == assetKindModel {
				shared, uncertain, refErr := modelCandidateReferencedByMetadata(
					candidates[index], otherName, metadata,
				)
				if refErr != nil {
					return refErr
				}
				if uncertain {
					return fmt.Errorf("another managed model has an unresolved reference to the model snapshot")
				}
				if shared {
					candidates[index].shared = true
				}
			}
		}
	}
	return nil
}

func candidateReferencesBackend(candidate cacheReclamationCandidate, backend *runtimeBackendMetadata) (bool, error) {
	if candidate.kind != assetKindBackend || backend == nil {
		return false, nil
	}
	relative := strings.TrimSpace(backend.CachePath)
	if !validBackendCacheReferencePath(relative) {
		return false, fmt.Errorf("managed backend cache reference is unsafe")
	}
	requirements, err := cacheRequirements(backend.Files)
	if err != nil || len(requirements) == 0 || strings.TrimSpace(backend.Revision) == "" {
		return false, fmt.Errorf("managed backend cache reference is incomplete")
	}
	relative = filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	if relative != candidate.relativePath {
		return false, nil
	}
	if !sameAssetRequirements(requirements, candidate.artifacts) {
		return false, fmt.Errorf("managed backend cache reference does not match its snapshot")
	}
	return true, nil
}

func validBackendCacheReferencePath(relative string) bool {
	if !validGenericRuntimeRelativePath(relative) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	parts := strings.Split(clean, "/")
	if len(parts) != 4 || parts[0] != "backend-artifacts" ||
		parts[1] != assetContentDirectory || parts[2] != assetKindBackend || len(parts[3]) != 64 {
		return false
	}
	_, err := hex.DecodeString(parts[3])
	return err == nil
}

func modelCandidateReferencedByMetadata(
	candidate cacheReclamationCandidate,
	modelName string,
	metadata cacheMetadata,
) (bool, bool, error) {
	requirements, err := cacheRequirements(metadata.Files)
	if err != nil {
		return false, false, err
	}
	if !sameAssetRequirements(requirements, candidate.artifacts) {
		return false, false, nil
	}
	source, ok := genericSourceForManagedModel(modelName)
	if !ok {
		return false, true, nil
	}
	artifacts := make([]genericArtifact, 0, len(requirements))
	for _, requirement := range requirements {
		artifacts = append(artifacts, genericArtifact{requirement: requirement})
	}
	identity := genericArtifactIdentityHash(assetKindModel, source, artifacts)
	return identity == candidate.identity, false, nil
}

func genericSourceForManagedModel(name string) (genericSource, bool) {
	if definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(name); ok {
		source, err := parseGenericSource(definition.Source)
		return source, err == nil
	}
	source, err := parseGenericSource(name)
	return source, err == nil
}

func cacheRequirements(files []metadataFile) ([]models.AssetRequirement, error) {
	if len(files) == 0 {
		return nil, nil
	}
	result := make([]models.AssetRequirement, 0, len(files))
	for _, file := range files {
		name := filepath.ToSlash(strings.TrimSpace(file.Path))
		if !validGenericRuntimeRelativePath(name) || file.Bytes <= 0 || len(file.SHA256) != 64 {
			return nil, fmt.Errorf("managed model asset facts are incomplete")
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return nil, fmt.Errorf("managed model asset digest is invalid")
		}
		result = append(result, models.AssetRequirement{
			Name: name, Bytes: file.Bytes, SHA256: strings.ToLower(file.SHA256),
		})
	}
	return result, nil
}

func sameAssetRequirements(left, right []models.AssetRequirement) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]models.AssetRequirement(nil), left...)
	right = append([]models.AssetRequirement(nil), right...)
	sort.Slice(left, func(i, j int) bool { return left[i].Name < left[j].Name })
	sort.Slice(right, func(i, j int) bool { return right[i].Name < right[j].Name })
	for index := range left {
		if left[index].Name != right[index].Name || left[index].Bytes != right[index].Bytes ||
			!strings.EqualFold(left[index].SHA256, right[index].SHA256) {
			return false
		}
	}
	return true
}

func validContentAddressedIdentity(metadata genericCacheMetadata, pathIdentity string) bool {
	if metadata.SourceKey == "" || len(metadata.Artifacts) == 0 {
		return false
	}
	artifacts := make([]genericArtifact, 0, len(metadata.Artifacts))
	for _, requirement := range metadata.Artifacts {
		artifacts = append(artifacts, genericArtifact{requirement: requirement})
	}
	names := genericArtifactIdentityNames(artifacts)
	sort.Strings(names)
	identity := metadata.Kind + "|" + metadata.SourceKey + "|" + strings.Join(names, ",")
	if metadata.Identity != identity {
		return false
	}
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:]) == pathIdentity
}

func (s *service) applyCacheReclamationPlan(
	ctx context.Context,
	plan cacheReclamationPlan,
) (int64, error) {
	var reclaimed int64
	for _, candidate := range plan.candidates {
		if candidate.shared {
			continue
		}
		if err := s.removeManagedTree(ctx, candidate.path); err != nil {
			return 0, err
		}
		if err := s.verifyManagedPathRemoved(ctx, filepath.Dir(candidate.path), filepath.Base(candidate.path)); err != nil {
			return 0, err
		}
		var err error
		reclaimed, err = addManagedCacheBytes(reclaimed, candidate.bytes)
		if err != nil {
			return 0, err
		}
	}
	return reclaimed, nil
}

func uncertainCacheReferences(modelName string, cause error) error {
	if errors.Is(cause, models.ErrModelCacheReferenceUncertain) {
		return cause
	}
	if cause == nil {
		return fmt.Errorf("%w: %s", models.ErrModelCacheReferenceUncertain, canonicalModelName(modelName))
	}
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	return fmt.Errorf("%w: %s", models.ErrModelCacheReferenceUncertain, canonicalModelName(modelName))
}
