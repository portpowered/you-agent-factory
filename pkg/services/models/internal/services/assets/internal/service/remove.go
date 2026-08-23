package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// RemoveModelAssets removes exactly the selected managed revision. All path
// traversal is based on directory entries so symlinks are unlinked rather
// than followed.
func (s *service) RemoveModelAssets(
	ctx context.Context,
	request models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	result := models.RemoveModelAssetsResult{
		ModelName: canonicalModelName(request.Name),
		Readiness: models.AssetReadinessMissing,
	}
	if err := request.Validate(); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	scope, err := s.resolveScope(ctx, request.Scope)
	if err != nil {
		return result, err
	}
	spec, source, err := s.resolveSource(scope.Runtime, request.Name)
	if err != nil {
		if errors.Is(err, models.ErrAssetSourceMissing) ||
			errors.Is(err, models.ErrAssetSourceUnsupported) {
			return result, modelCacheNotFound(request.Name)
		}
		return result, err
	}
	result.ModelName = spec.modelName
	if err := assetContextError(ctx); err != nil {
		return result, err
	}

	modelRoot, err := s.modelCacheRoot(scope.CacheDirectory, spec.modelName)
	if err != nil {
		return result, fmt.Errorf("resolve managed model cache: %w", err)
	}
	if err := s.requireManagedDirectoryChild(
		ctx,
		filepath.Dir(modelRoot),
		filepath.Base(modelRoot),
		"model",
	); err != nil {
		return result, err
	}

	snapshot, _, err := s.inspectCache(ctx, scope.CacheDirectory, spec, source)
	if err != nil {
		return result, err
	}
	revision := strings.TrimSpace(snapshot.Revision)
	if revision == "" {
		return result, modelCacheNotFound(spec.modelName)
	}
	revisionPath, err := managedCacheChildPath(modelRoot, revision, "revision")
	if err != nil {
		return result, fmt.Errorf("%w: %v", models.ErrModelCacheUnsafe, err)
	}
	if err := s.requireManagedDirectoryChild(ctx, modelRoot, revision, "revision"); err != nil {
		return result, err
	}

	bytesRemoved, err := s.measureRevisionBytes(ctx, revisionPath)
	if errors.Is(err, os.ErrNotExist) {
		return result, modelCacheNotFound(spec.modelName)
	}
	if err != nil {
		return result, err
	}
	if err := s.removeManagedTree(ctx, revisionPath); err != nil {
		return result, err
	}
	if err := s.verifyManagedPathRemoved(ctx, modelRoot, revision); err != nil {
		return result, err
	}

	s.preparedRuntimeMu.Lock()
	delete(s.preparedRuntime, preparedRuntimeKey(request.Scope, request.Name))
	s.preparedRuntimeMu.Unlock()
	return models.RemoveModelAssetsResult{
		ModelName:    spec.modelName,
		Revision:     revision,
		CachePath:    revisionPath,
		BytesRemoved: bytesRemoved,
		Readiness:    models.AssetReadinessMissing,
		Outcome:      models.AssetRemovalRemoved,
	}, nil
}

func modelCacheNotFound(name string) error {
	return fmt.Errorf("%w: %s", models.ErrModelCacheNotFound, canonicalModelName(name))
}

// requireManagedDirectoryChild verifies a direct directory child without
// resolving links. The configured cache root itself is an accepted location;
// model and revision children must be real directories owned by that root.
func (s *service) requireManagedDirectoryChild(
	ctx context.Context,
	parent string,
	child string,
	kind string,
) error {
	if err := assetContextError(ctx); err != nil {
		return err
	}
	child = strings.TrimSpace(child)
	if child == "" || filepath.Base(child) != child || child == "." || child == ".." {
		return fmt.Errorf("%w: managed cache %s path is invalid", models.ErrModelCacheUnsafe, kind)
	}
	entries, err := s.readDirectory(parent)
	if errors.Is(err, os.ErrNotExist) {
		return modelCacheNotFound(child)
	}
	if err != nil {
		return fmt.Errorf("inspect managed cache %s: %w", kind, err)
	}
	for _, entry := range entries {
		if entry == nil || entry.Name() != child {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: managed cache %s is a symlink", models.ErrModelCacheUnsafe, kind)
		}
		info, infoErr := entry.Info()
		if errors.Is(infoErr, os.ErrNotExist) {
			return modelCacheNotFound(child)
		}
		if infoErr != nil {
			return fmt.Errorf("inspect managed cache %s: %w", kind, infoErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: managed cache %s is not a directory", models.ErrModelCacheUnsafe, kind)
		}
		return nil
	}
	return modelCacheNotFound(child)
}

func (s *service) removeManagedTree(ctx context.Context, directory string) error {
	if err := assetContextError(ctx); err != nil {
		return err
	}
	entries, err := s.readDirectory(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: read managed cache revision: %v", models.ErrModelCacheRemovalFailed, err)
	}
	for _, entry := range entries {
		if err := assetContextError(ctx); err != nil {
			return err
		}
		if entry == nil {
			continue
		}
		name := entry.Name()
		if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
			return fmt.Errorf("%w: invalid managed cache entry %q", models.ErrModelCacheUnsafe, name)
		}
		child := filepath.Join(directory, name)
		if entry.Type()&os.ModeSymlink != 0 {
			if err := s.removeManagedPath(child); err != nil {
				return err
			}
			continue
		}
		info, infoErr := entry.Info()
		if errors.Is(infoErr, os.ErrNotExist) {
			continue
		}
		if infoErr != nil {
			return fmt.Errorf("%w: inspect managed cache entry: %v", models.ErrModelCacheRemovalFailed, infoErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := s.removeManagedPath(child); err != nil {
				return err
			}
			continue
		}
		if info.IsDir() {
			if err := s.removeManagedTree(ctx, child); err != nil {
				return err
			}
			continue
		}
		if err := s.removeManagedPath(child); err != nil {
			return err
		}
	}
	if err := assetContextError(ctx); err != nil {
		return err
	}
	return s.removeManagedPath(directory)
}

func (s *service) removeManagedPath(path string) error {
	if err := s.removePath(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: remove managed cache path: %v", models.ErrModelCacheRemovalFailed, err)
	}
	return nil
}

func (s *service) verifyManagedPathRemoved(ctx context.Context, parent, child string) error {
	if err := assetContextError(ctx); err != nil {
		return err
	}
	entries, err := s.readDirectory(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: verify managed cache removal: %v", models.ErrModelCacheRemovalFailed, err)
	}
	for _, entry := range entries {
		if entry != nil && entry.Name() == child {
			return fmt.Errorf("%w: managed cache revision still exists", models.ErrModelCacheRemovalFailed)
		}
	}
	return nil
}
