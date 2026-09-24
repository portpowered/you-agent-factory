// Package reclamation owns the coordinated ownership census for managed cache
// snapshots. Asset service policy supplies the selected source identity and
// the filesystem effects; this package decides whether each snapshot is safe
// to reclaim and holds source locks through the caller's revision removal.
package reclamation

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
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

const (
	contentDirectory = ".you-content-addressed"
	metadataName     = ".you-assets.json"
	modelKind        = "model"
	backendKind      = "backend"
	referenceLock    = "references.lock"
)

// FileSystem exposes the exact filesystem effects needed to inspect and
// reclaim cache trees. Removal methods retain the Assets service's symlink
// safe deletion rules.
type FileSystem interface {
	ReadDirectory(string) ([]os.DirEntry, error)
	ReadFile(string) ([]byte, error)
	OpenFile(string) (io.ReadCloser, error)
	InspectPath(string) (os.FileInfo, error)
	RemoveManagedTree(context.Context, string) error
	VerifyManagedPathRemoved(context.Context, string, string) error
}

// SourceIdentity resolves a managed model name to its stable cache identity.
type SourceIdentity interface {
	ManagedSourceIdentity(string) (string, bool)
}

type candidate struct {
	path         string
	relativePath string
	kind         string
	identity     string
	sourceKey    string
	artifacts    []models.AssetRequirement
	bytes        int64
	shared       bool
}

// Plan is held until the selected managed revision has been removed.
type Plan struct {
	candidates []candidate
	Retained   int64
}

// Service coordinates reference publication with a census and deletion.
type Service struct {
	files        FileSystem
	coordination modelseffects.AssetStagingCoordination
	sources      SourceIdentity
	contextError func(context.Context) error
}

// New constructs the cache reclamation component for the Assets service.
func New(
	files FileSystem,
	coordination modelseffects.AssetStagingCoordination,
	sources SourceIdentity,
	contextError func(context.Context) error,
) *Service {
	if contextError == nil {
		contextError = defaultContextError
	}
	return &Service{
		files: files, coordination: coordination, sources: sources,
		contextError: contextError,
	}
}

func (s *Service) checkContext(ctx context.Context) error {
	if s == nil || s.contextError == nil {
		return defaultContextError(ctx)
	}
	return s.contextError(ctx)
}

// LockReferences serializes reference publication with removal planning.
func (s *Service) LockReferences(ctx context.Context, cacheRoot string) (io.Closer, error) {
	if s == nil || s.coordination == nil {
		return nil, errors.New("cache reference coordination is unavailable")
	}
	return s.coordination.Lock(ctx, filepath.Join(cacheRoot, ".you-asset-locks", referenceLock))
}

// UncertainReference classifies coordination and census failures for the
// Models transport mapper while preserving cancellation causes.
func (s *Service) UncertainReference(modelName string, cause error) error {
	return uncertain(modelName, cause)
}

// ReadManagedMetadata loads the selected revision reference after the caller
// has acquired the reference publication lock.
func (s *Service) ReadManagedMetadata(
	ctx context.Context,
	modelRoot, modelName, revision string,
) (ManagedMetadata, error) {
	body, found, err := s.managedRegularFile(ctx, modelRoot, ".managed-cache.json")
	if err != nil {
		return ManagedMetadata{}, uncertain(modelName, err)
	}
	if !found {
		return ManagedMetadata{}, uncertain(modelName, nil)
	}
	var metadata ManagedMetadata
	if err := json.Unmarshal(body, &metadata); err != nil ||
		!strings.EqualFold(strings.TrimSpace(metadata.ModelName), strings.TrimSpace(modelName)) ||
		!strings.EqualFold(strings.TrimSpace(metadata.Revision), strings.TrimSpace(revision)) {
		return ManagedMetadata{}, uncertain(modelName, err)
	}
	return metadata, nil
}

// PlanAndLockCandidates returns a stable ownership plan and holds each
// candidate's source lock until CloseLocks is called.
func (s *Service) PlanAndLockCandidates(
	ctx context.Context,
	cacheRoot string,
	modelRoot string,
	modelName string,
	metadata ManagedMetadata,
	sourceKey string,
) (Plan, []io.Closer, error) {
	locks := make([]io.Closer, 0, 2)
	lockedPaths := make(map[string]struct{}, 2)
	for {
		plan, err := s.plan(ctx, cacheRoot, modelRoot, modelName, metadata, sourceKey)
		if err != nil {
			return Plan{}, locks, err
		}
		paths := candidateLockPaths(cacheRoot, plan.candidates, lockedPaths)
		if len(paths) == 0 {
			return plan, locks, nil
		}
		sort.Strings(paths)
		for _, path := range paths {
			lock, err := s.lockCandidate(ctx, path, modelName)
			if err != nil {
				return Plan{}, locks, err
			}
			locks = append(locks, lock)
			lockedPaths[path] = struct{}{}
		}
	}
}

// CloseLocks releases candidate ownership in reverse acquisition order.
func (s *Service) CloseLocks(locks []io.Closer, primary error) error {
	for index := len(locks) - 1; index >= 0; index-- {
		if locks[index] == nil {
			continue
		}
		if err := locks[index].Close(); err != nil {
			primary = errors.Join(primary, fmt.Errorf(
				"%w: coordinated cache removal could not release ownership",
				models.ErrModelCacheRemovalFailed,
			))
		}
	}
	return primary
}

// Apply removes every unshared snapshot in a previously locked plan and
// reports only bytes whose deletion was verified.
func (s *Service) Apply(ctx context.Context, plan Plan) (int64, error) {
	var reclaimed int64
	for _, item := range plan.candidates {
		if item.shared {
			continue
		}
		if err := s.files.RemoveManagedTree(ctx, item.path); err != nil {
			return 0, err
		}
		if err := s.files.VerifyManagedPathRemoved(ctx, filepath.Dir(item.path), filepath.Base(item.path)); err != nil {
			return 0, err
		}
		var err error
		reclaimed, err = addBytes(reclaimed, item.bytes)
		if err != nil {
			return 0, err
		}
	}
	return reclaimed, nil
}

func (s *Service) lockCandidate(ctx context.Context, path, modelName string) (io.Closer, error) {
	if s == nil || s.coordination == nil {
		return nil, uncertain(modelName, errors.New("cache candidate coordination is unavailable"))
	}
	lock, err := s.coordination.Lock(ctx, path)
	if err != nil {
		return nil, uncertain(modelName, err)
	}
	return lock, nil
}

func (s *Service) plan(
	ctx context.Context,
	cacheRoot string,
	modelRoot string,
	modelName string,
	metadata ManagedMetadata,
	sourceKey string,
) (Plan, error) {
	plan := Plan{}
	modelArtifacts, err := requirements(metadata.Files)
	if err != nil || len(modelArtifacts) == 0 {
		return plan, uncertain(modelName, err)
	}
	modelCandidate, present, err := s.inspectSnapshot(
		ctx, cacheRoot, modelKind, sourceKey, modelArtifacts,
	)
	if err != nil {
		return plan, uncertain(modelName, err)
	}
	if present {
		plan.candidates = append(plan.candidates, modelCandidate)
	}
	if metadata.Backend != nil {
		backendCandidate, found, inspectErr := s.inspectBackendSnapshot(ctx, cacheRoot, metadata.Backend)
		if inspectErr != nil {
			return plan, uncertain(modelName, inspectErr)
		}
		if found {
			plan.candidates = append(plan.candidates, backendCandidate)
		}
	}
	if len(plan.candidates) == 0 {
		return plan, nil
	}
	if err := s.markShared(ctx, cacheRoot, modelRoot, plan.candidates); err != nil {
		return Plan{}, uncertain(modelName, err)
	}
	for _, item := range plan.candidates {
		if !item.shared {
			continue
		}
		plan.Retained, err = addBytes(plan.Retained, item.bytes)
		if err != nil {
			return Plan{}, err
		}
	}
	return plan, nil
}

func candidateLockPaths(
	cacheRoot string,
	candidates []candidate,
	locked map[string]struct{},
) []string {
	paths := make([]string, 0, len(candidates))
	for _, item := range candidates {
		path := candidateLockPath(cacheRoot, item)
		if _, ok := locked[path]; !ok {
			paths = append(paths, path)
		}
	}
	return paths
}

func candidateLockPath(cacheRoot string, item candidate) string {
	root := cacheRoot
	if item.kind == backendKind {
		root = filepath.Join(root, "backend-artifacts")
	}
	digest := sha256.Sum256([]byte(item.kind + "|" + item.sourceKey))
	return filepath.Join(root, ".you-asset-locks", item.kind, hex.EncodeToString(digest[:])+".lock")
}

func (s *Service) inspectSnapshot(
	ctx context.Context,
	cacheRoot string,
	kind string,
	sourceKey string,
	artifacts []models.AssetRequirement,
) (candidate, bool, error) {
	identity, key := artifactIdentity(kind, sourceKey, artifacts)
	kindRoot := filepath.Join(cacheRoot, contentDirectory, kind)
	present, err := s.managedPathPresent(ctx, cacheRoot, contentDirectory, "content-addressed cache")
	if err != nil || !present {
		return candidate{}, false, err
	}
	present, err = s.managedPathPresent(ctx, filepath.Join(cacheRoot, contentDirectory), kind, "content-addressed cache kind")
	if err != nil || !present {
		return candidate{}, false, err
	}
	present, err = s.managedPathPresent(ctx, kindRoot, identity, "content-addressed snapshot")
	if err != nil || !present {
		return candidate{}, false, err
	}
	path := filepath.Join(kindRoot, identity)
	metadata, found, err := s.snapshotMetadata(ctx, path)
	if err != nil {
		return candidate{}, false, err
	}
	if !found || metadata.Kind != kind || metadata.Identity != key || metadata.SourceKey != sourceKey ||
		!sameRequirements(metadata.Artifacts, artifacts) {
		return candidate{}, false, errors.New("content-addressed snapshot metadata does not match its managed reference")
	}
	if err := s.verifyArtifacts(ctx, path, artifacts); err != nil {
		return candidate{}, false, err
	}
	bytes, err := s.measureDirectory(ctx, path)
	if err != nil {
		return candidate{}, false, err
	}
	return candidate{
		path: path, kind: kind, identity: identity, sourceKey: sourceKey,
		artifacts: cloneRequirements(artifacts), bytes: bytes,
	}, true, nil
}

func (s *Service) inspectBackendSnapshot(
	ctx context.Context,
	cacheRoot string,
	backend *BackendMetadata,
) (candidate, bool, error) {
	relative := strings.TrimSpace(backend.CachePath)
	identity, err := backendIdentity(relative)
	if err != nil {
		return candidate{}, false, err
	}
	path, present, err := s.backendSnapshotPath(ctx, cacheRoot, identity)
	if err != nil || !present {
		return candidate{}, false, err
	}
	metadata, found, err := s.snapshotMetadata(ctx, path)
	if err != nil {
		return candidate{}, false, err
	}
	artifacts, err := requirements(backend.Files)
	if err != nil {
		return candidate{}, false, err
	}
	if !backendMetadataMatches(metadata, found, identity, artifacts) {
		return candidate{}, false, errors.New("backend snapshot metadata does not prove ownership")
	}
	if err := s.verifyArtifacts(ctx, path, artifacts); err != nil {
		return candidate{}, false, err
	}
	bytes, err := s.measureDirectory(ctx, path)
	if err != nil {
		return candidate{}, false, err
	}
	return candidate{
		path: path, relativePath: filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative))),
		kind: backendKind, identity: identity, sourceKey: metadata.SourceKey,
		artifacts: artifacts, bytes: bytes,
	}, true, nil
}

func backendIdentity(relative string) (string, error) {
	if !validBackendPath(relative) {
		return "", errors.New("backend cache path is not a YOU-owned content-addressed path")
	}
	identity := filepath.Base(filepath.FromSlash(relative))
	if len(identity) != 64 {
		return "", errors.New("backend cache identity is invalid")
	}
	if _, err := hex.DecodeString(identity); err != nil {
		return "", errors.New("backend cache identity is invalid")
	}
	return identity, nil
}

func (s *Service) backendSnapshotPath(ctx context.Context, cacheRoot, identity string) (string, bool, error) {
	root := filepath.Join(cacheRoot, "backend-artifacts")
	if present, err := s.managedPathPresent(ctx, cacheRoot, "backend-artifacts", "backend cache"); err != nil || !present {
		return "", false, err
	}
	if present, err := s.managedPathPresent(ctx, root, contentDirectory, "backend content-addressed cache"); err != nil || !present {
		return "", false, err
	}
	kindRoot := filepath.Join(root, contentDirectory, backendKind)
	if present, err := s.managedPathPresent(ctx, filepath.Dir(kindRoot), backendKind, "backend content-addressed cache kind"); err != nil || !present {
		return "", false, err
	}
	if present, err := s.managedPathPresent(ctx, kindRoot, identity, "backend content-addressed snapshot"); err != nil || !present {
		return "", false, err
	}
	return filepath.Join(kindRoot, identity), true, nil
}

func backendMetadataMatches(
	metadata SnapshotMetadata,
	found bool,
	identity string,
	artifacts []models.AssetRequirement,
) bool {
	return found && metadata.Kind == backendKind && len(artifacts) > 0 &&
		sameRequirements(metadata.Artifacts, artifacts) && metadata.Source != "" &&
		metadata.SourceKey != "" && validSnapshotIdentity(metadata, identity)
}

func (s *Service) managedPathPresent(ctx context.Context, parent, child, kind string) (bool, error) {
	if err := s.checkContext(ctx); err != nil {
		return false, err
	}
	entries, err := s.files.ReadDirectory(parent)
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
		if err := validateDirectoryEntry(entry, child, kind); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s *Service) snapshotMetadata(ctx context.Context, root string) (SnapshotMetadata, bool, error) {
	body, found, err := s.managedRegularFile(ctx, root, metadataName)
	if err != nil || !found {
		return SnapshotMetadata{}, found, err
	}
	var metadata SnapshotMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return SnapshotMetadata{}, true, fmt.Errorf("decode content-addressed cache metadata: %w", err)
	}
	return metadata, true, nil
}

func (s *Service) managedRegularFile(ctx context.Context, parent, name string) ([]byte, bool, error) {
	if err := s.checkContext(ctx); err != nil {
		return nil, false, err
	}
	entries, err := s.files.ReadDirectory(parent)
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
			return nil, true, errors.New("managed metadata is a symlink")
		}
		info, err := entry.Info()
		if err != nil {
			return nil, true, err
		}
		if !info.Mode().IsRegular() {
			return nil, true, errors.New("managed metadata is not a regular file")
		}
		body, err := s.files.ReadFile(filepath.Join(parent, name))
		return body, true, err
	}
	return nil, false, nil
}

func (s *Service) verifyArtifacts(ctx context.Context, root string, artifacts []models.AssetRequirement) error {
	for _, artifact := range artifacts {
		if err := s.checkContext(ctx); err != nil {
			return err
		}
		if !validRelativePath(artifact.Name) {
			return errors.New("content-addressed artifact path is invalid")
		}
		path := filepath.Join(root, filepath.FromSlash(artifact.Name))
		if err := s.requireArtifactPath(root, artifact.Name); err != nil {
			return err
		}
		info, err := s.files.InspectPath(path)
		if err != nil || info == nil || !info.Mode().IsRegular() || info.Size() != artifact.Bytes {
			return fmt.Errorf("content-addressed artifact is not verified: %s", artifact.Name)
		}
		file, err := s.files.OpenFile(path)
		if err != nil {
			return fmt.Errorf("content-addressed artifact is not verified: %s", artifact.Name)
		}
		hasher := sha256.New()
		actualBytes, hashErr := io.Copy(hasher, file)
		closeErr := file.Close()
		if hashErr != nil || closeErr != nil || artifact.Bytes <= 0 || actualBytes != artifact.Bytes ||
			hex.EncodeToString(hasher.Sum(nil)) != strings.ToLower(strings.TrimSpace(artifact.SHA256)) {
			return fmt.Errorf("content-addressed artifact is not verified: %s", artifact.Name)
		}
	}
	return nil
}

func (s *Service) requireArtifactPath(root, relative string) error {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	parent := root
	for index, part := range parts {
		if err := s.requireArtifactPathPart(parent, part, index == len(parts)-1); err != nil {
			return err
		}
		parent = filepath.Join(parent, filepath.FromSlash(part))
	}
	return nil
}

func (s *Service) requireArtifactPathPart(parent, part string, leaf bool) error {
	if part == "" || part == "." || part == ".." {
		return errors.New("content-addressed artifact path is invalid")
	}
	entries, err := s.files.ReadDirectory(parent)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry == nil || entry.Name() != part {
			continue
		}
		return validArtifactEntry(entry, leaf)
	}
	return os.ErrNotExist
}

func validArtifactEntry(entry os.DirEntry, leaf bool) error {
	if entry.Type()&os.ModeSymlink != 0 {
		return errors.New("content-addressed artifact path contains a symlink")
	}
	info, err := entry.Info()
	if err != nil {
		return err
	}
	if leaf && !info.Mode().IsRegular() {
		return errors.New("content-addressed artifact is not a regular file")
	}
	if !leaf && !info.IsDir() {
		return errors.New("content-addressed artifact parent is not a directory")
	}
	return nil
}

func (s *Service) markShared(ctx context.Context, cacheRoot, selectedRoot string, candidates []candidate) error {
	entries, err := s.files.ReadDirectory(cacheRoot)
	if err != nil {
		return err
	}
	selectedRoot = filepath.Clean(selectedRoot)
	for _, entry := range entries {
		if err := s.markSharedEntry(ctx, cacheRoot, selectedRoot, entry, candidates); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) markSharedEntry(
	ctx context.Context,
	cacheRoot, selectedRoot string,
	entry os.DirEntry,
	candidates []candidate,
) error {
	if err := s.checkContext(ctx); err != nil {
		return err
	}
	if skipManagedRoot(entry) {
		return nil
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return errors.New("managed model root is a symlink")
	}
	info, err := entry.Info()
	if err != nil || !info.IsDir() {
		return err
	}
	modelRoot := filepath.Join(cacheRoot, entry.Name())
	if filepath.Clean(modelRoot) == selectedRoot {
		return nil
	}
	body, found, err := s.managedRegularFile(ctx, modelRoot, ".managed-cache.json")
	if err != nil || !found {
		return err
	}
	var metadata ManagedMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return fmt.Errorf("decode managed model reference: %w", err)
	}
	otherName := strings.TrimSpace(metadata.ModelName)
	if otherName == "" {
		otherName = entry.Name()
	}
	return markCandidatesShared(candidates, metadata, otherName, s.sources)
}

func skipManagedRoot(entry os.DirEntry) bool {
	return entry == nil || entry.Name() == contentDirectory || entry.Name() == "backend-artifacts" ||
		entry.Name() == ".you-asset-locks"
}

func markCandidatesShared(
	candidates []candidate,
	metadata ManagedMetadata,
	modelName string,
	sources SourceIdentity,
) error {
	for index := range candidates {
		shared, uncertain, err := sharedByReference(candidates[index], metadata, modelName, sources)
		if err != nil {
			return err
		}
		if uncertain {
			return errors.New("another managed model has an unresolved reference to the model snapshot")
		}
		candidates[index].shared = candidates[index].shared || shared
	}
	return nil
}

func sharedByReference(
	item candidate,
	metadata ManagedMetadata,
	modelName string,
	sources SourceIdentity,
) (bool, bool, error) {
	sharedBackend, err := sharedBackendReference(item, metadata.Backend)
	if err != nil || sharedBackend || item.kind != modelKind {
		return sharedBackend, false, err
	}
	return sharedModelReference(item, metadata, modelName, sources)
}

func sharedBackendReference(item candidate, backend *BackendMetadata) (bool, error) {
	if item.kind != backendKind || backend == nil {
		return false, nil
	}
	relative := strings.TrimSpace(backend.CachePath)
	if !validBackendPath(relative) {
		return false, errors.New("managed backend cache reference is unsafe")
	}
	artifacts, err := requirements(backend.Files)
	if err != nil || len(artifacts) == 0 || strings.TrimSpace(backend.Revision) == "" {
		return false, errors.New("managed backend cache reference is incomplete")
	}
	relative = filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	if relative != item.relativePath {
		return false, nil
	}
	if !sameRequirements(artifacts, item.artifacts) {
		return false, errors.New("managed backend cache reference does not match its snapshot")
	}
	return true, nil
}

func sharedModelReference(
	item candidate,
	metadata ManagedMetadata,
	modelName string,
	sources SourceIdentity,
) (bool, bool, error) {
	artifacts, err := requirements(metadata.Files)
	if err != nil {
		return false, false, err
	}
	if !sameRequirements(artifacts, item.artifacts) {
		return false, false, nil
	}
	sourceKey, ok := sources.ManagedSourceIdentity(modelName)
	if !ok {
		return false, true, nil
	}
	identity, _ := artifactIdentity(modelKind, sourceKey, artifacts)
	return identity == item.identity, false, nil
}

func (s *Service) measureDirectory(ctx context.Context, root string) (int64, error) {
	if err := s.checkContext(ctx); err != nil {
		return 0, err
	}
	entries, err := s.files.ReadDirectory(root)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		if entry == nil || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		if info.IsDir() {
			bytes, err := s.measureDirectory(ctx, filepath.Join(root, entry.Name()))
			if err != nil {
				return 0, err
			}
			total, err = addBytes(total, bytes)
			if err != nil {
				return 0, err
			}
			continue
		}
		if info.Mode().IsRegular() {
			total, err = addBytes(total, info.Size())
			if err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}

func validateDirectoryEntry(entry os.DirEntry, child, kind string) error {
	if entry.Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: managed cache %s is a symlink", models.ErrModelCacheUnsafe, kind)
	}
	info, err := entry.Info()
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", models.ErrModelCacheNotFound, child)
	}
	if err != nil {
		return fmt.Errorf("inspect managed cache %s: %w", kind, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: managed cache %s is not a directory", models.ErrModelCacheUnsafe, kind)
	}
	return nil
}

func requirements(files []ManagedFile) ([]models.AssetRequirement, error) {
	result := make([]models.AssetRequirement, 0, len(files))
	for _, file := range files {
		name := filepath.ToSlash(strings.TrimSpace(file.Path))
		if !validRelativePath(name) || file.Bytes <= 0 || len(file.SHA256) != 64 {
			return nil, errors.New("managed model asset facts are incomplete")
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return nil, errors.New("managed model asset digest is invalid")
		}
		result = append(result, models.AssetRequirement{Name: name, Bytes: file.Bytes, SHA256: strings.ToLower(file.SHA256)})
	}
	return result, nil
}

func artifactIdentity(kind, sourceKey string, artifacts []models.AssetRequirement) (string, string) {
	names := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		names = append(names, fmt.Sprintf("%s:%d:%s", artifact.Name, artifact.Bytes,
			strings.ToLower(strings.TrimSpace(artifact.SHA256))))
	}
	sort.Strings(names)
	key := kind + "|" + sourceKey + "|" + strings.Join(names, ",")
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:]), key
}

func validSnapshotIdentity(metadata SnapshotMetadata, pathIdentity string) bool {
	if metadata.SourceKey == "" || len(metadata.Artifacts) == 0 {
		return false
	}
	_, key := artifactIdentity(metadata.Kind, metadata.SourceKey, metadata.Artifacts)
	if metadata.Identity != key {
		return false
	}
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:]) == pathIdentity
}

func validBackendPath(relative string) bool {
	if !validRelativePath(relative) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	parts := strings.Split(clean, "/")
	if len(parts) != 4 || parts[0] != "backend-artifacts" || parts[1] != contentDirectory ||
		parts[2] != backendKind || len(parts[3]) != 64 {
		return false
	}
	_, err := hex.DecodeString(parts[3])
	return err == nil
}

func validRelativePath(relative string) bool {
	if strings.TrimSpace(relative) == "" || strings.Contains(relative, "\\") || strings.HasPrefix(relative, "/") {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func sameRequirements(left, right []models.AssetRequirement) bool {
	if len(left) != len(right) {
		return false
	}
	left = cloneRequirements(left)
	right = cloneRequirements(right)
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

func cloneRequirements(source []models.AssetRequirement) []models.AssetRequirement {
	return append([]models.AssetRequirement(nil), source...)
}

func addBytes(total, next int64) (int64, error) {
	if next < 0 || total > int64(^uint64(0)>>1)-next {
		return 0, errors.New("managed cache byte count exceeds int64 range")
	}
	return total + next, nil
}

func defaultContextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	return ctx.Err()
}

func uncertain(modelName string, cause error) error {
	if errors.Is(cause, models.ErrModelCacheReferenceUncertain) ||
		errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	return fmt.Errorf("%w: %s", models.ErrModelCacheReferenceUncertain, strings.TrimSpace(modelName))
}
