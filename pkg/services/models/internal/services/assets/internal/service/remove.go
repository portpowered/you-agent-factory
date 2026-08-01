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

var errAssetCachePathOutsideRoot = errors.New("model asset cache path is outside its model root")

func (s *service) RemoveModelAssets(
	ctx context.Context,
	request models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	if err := request.Validate(); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	scope, err := s.resolveScope(ctx, request.Scope)
	if err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	spec, _, err := s.resolveSource(scope.Runtime, request.Name)
	if err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	if err := assetContextError(ctx); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	root, err := s.modelCacheRoot(scope.CacheDirectory, spec.modelName)
	if err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	info, err := s.inspectPath(root)
	if errors.Is(err, os.ErrNotExist) {
		return removedAssetResult(spec.modelName, models.AssetRemovalAlreadyAbsent), nil
	}
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return models.RemoveModelAssetsResult{}, contextErr
		}
		return models.RemoveModelAssetsResult{}, fmt.Errorf(
			"inspect managed model cache for removal: %w", err,
		)
	}
	if err := assetContextError(ctx); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	if info == nil || !info.IsDir() {
		return models.RemoveModelAssetsResult{}, fmt.Errorf(
			"%w: managed model cache root is not a directory",
			models.ErrAssetUnavailable,
		)
	}

	removed, err := s.removeAssetDirectory(ctx, root)
	if err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	outcome := models.AssetRemovalAlreadyAbsent
	if removed {
		outcome = models.AssetRemovalRemoved
	}
	return removedAssetResult(spec.modelName, outcome), nil
}

func (s *service) removeAssetDirectory(ctx context.Context, root string) (bool, error) {
	if err := assetContextError(ctx); err != nil {
		return false, err
	}
	entries, err := s.readDirectory(root)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return false, contextErr
		}
		return false, fmt.Errorf(
			"read managed model cache for removal: %w",
			err,
		)
	}

	removed := false
	for _, entry := range entries {
		if err := assetContextError(ctx); err != nil {
			return removed, err
		}
		if entry == nil {
			return removed, fmt.Errorf(
				"%w: %w: cache directory returned an empty entry",
				models.ErrAssetUnavailable, errAssetCachePathOutsideRoot,
			)
		}
		path, err := assetPathWithinRoot(root, entry.Name())
		if err != nil {
			return removed, err
		}
		var childRemoved bool
		if entry.IsDir() {
			childRemoved, err = s.removeAssetDirectory(ctx, path)
		} else {
			childRemoved, err = s.removeAssetPath(ctx, path)
		}
		if err != nil {
			return removed || childRemoved, err
		}
		removed = removed || childRemoved
	}

	childRemoved, err := s.removeAssetPath(ctx, root)
	if err != nil {
		return removed || childRemoved, err
	}
	return removed || childRemoved, nil
}

func (s *service) removeAssetPath(ctx context.Context, path string) (bool, error) {
	if err := assetContextError(ctx); err != nil {
		return false, err
	}
	if err := s.removePath(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if contextErr := assetContextError(ctx); contextErr != nil {
			return false, contextErr
		}
		return false, fmt.Errorf("remove managed model cache path %q: %w", filepath.Base(path), err)
	}
	if err := assetContextError(ctx); err != nil {
		return true, err
	}
	return true, nil
}

func assetPathWithinRoot(root, name string) (string, error) {
	if strings.TrimSpace(name) == "" || name == "." || name == ".." ||
		filepath.IsAbs(name) || filepath.VolumeName(name) != "" || filepath.Base(name) != name {
		return "", assetCachePathError(name)
	}
	cleanRoot := filepath.Clean(root)
	path := filepath.Clean(filepath.Join(cleanRoot, name))
	relative, err := filepath.Rel(cleanRoot, path)
	if err != nil {
		return "", assetCachePathError(name)
	}
	if relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", assetCachePathError(name)
	}
	return path, nil
}

func assetCachePathError(name string) error {
	return fmt.Errorf(
		"%w: %w: %q",
		models.ErrAssetUnavailable, errAssetCachePathOutsideRoot, name,
	)
}

func removedAssetResult(modelName string, outcome models.AssetRemovalOutcome) models.RemoveModelAssetsResult {
	return models.RemoveModelAssetsResult{
		ModelName: modelName,
		Readiness: models.AssetReadinessMissing,
		Outcome:   outcome,
	}
}
