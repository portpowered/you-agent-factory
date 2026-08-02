package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

// RemoveModelAssets validates the scoped identity and delegates the entire
// filesystem mutation to the injected handle-relative platform effect. It
// intentionally does not resolve a source, inspect readiness, or consult the
// host platform: an old cache remains removable after those settings change.
func (s *service) RemoveModelAssets(
	ctx context.Context,
	request models.RemoveModelAssetsRequest,
) (result models.RemoveModelAssetsResult, resultErr error) {
	modelIdentity := safeRemovalModelIdentity(request.Name)
	start := s.operationNow()
	removal := modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeNotAttempted}
	s.logAssetRemovalStart(modelIdentity)
	defer func() {
		s.logAssetRemovalTerminal(modelIdentity, start, result, resultErr, removal)
	}()

	if err := request.Validate(); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	scope, err := s.resolveScope(ctx, request.Scope)
	if err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	spec, err := removalAssetSpec(request.Name)
	if err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	if err := assetContextError(ctx); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	parent, err := s.modelCacheParent(scope.CacheDirectory)
	if err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	removal, err = s.removeTree(ctx, parent, spec.modelName)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return models.RemoveModelAssetsResult{}, contextErr
		}
		return models.RemoveModelAssetsResult{}, fmt.Errorf(
			"%w: remove managed model assets: %w", models.ErrAssetUnavailable, err,
		)
	}
	if err := assetContextError(ctx); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	switch removal.State {
	case modelseffects.AssetRemoveTreeRemoved:
		return removedAssetResult(spec.modelName, models.AssetRemovalRemoved), nil
	case modelseffects.AssetRemoveTreeAbsent:
		return removedAssetResult(spec.modelName, models.AssetRemovalAlreadyAbsent), nil
	default:
		return models.RemoveModelAssetsResult{}, fmt.Errorf(
			"%w: removal returned non-terminal state %q",
			models.ErrAssetUnavailable,
			removal.State,
		)
	}
}

func (s *service) modelCacheParent(cacheDirectory string) (string, error) {
	parent := strings.TrimSpace(cacheDirectory)
	if parent != "" {
		// Normalize every valid relative cache directory before it reaches the
		// secure platform boundary. Absolute, drive-relative, UNC, and device
		// paths remain the boundary's responsibility to accept or reject.
		if filepath.IsAbs(parent) || filepath.VolumeName(parent) != "" {
			return parent, nil
		}
		absolute, err := filepath.Abs(filepath.Clean(parent))
		if err != nil {
			return "", fmt.Errorf("normalize managed model cache directory: %w", err)
		}
		return absolute, nil
	}
	home, err := s.resolveHome()
	if err != nil {
		return "", fmt.Errorf("resolve managed model cache directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolve managed model cache directory: empty home directory")
	}
	return filepath.Abs(filepath.Join(home, ".agent-factory", "models"))
}
func removalAssetSpec(modelName string) (assetSpec, error) {
	spec, ok := supportedAssetSpecs()[canonicalModelName(modelName)]
	if !ok {
		return assetSpec{}, fmt.Errorf(
			"%w: %s", models.ErrAssetSourceUnsupported, modelName,
		)
	}
	return spec, nil
}

func removedAssetResult(
	modelName string,
	outcome models.AssetRemovalOutcome,
) models.RemoveModelAssetsResult {
	return models.RemoveModelAssetsResult{
		ModelName: modelName,
		Readiness: models.AssetReadinessMissing,
		Outcome:   outcome,
	}
}

func (s *service) operationNow() time.Time {
	if s == nil || s.now == nil {
		return time.Time{}
	}
	return s.now()
}

func safeRemovalModelIdentity(name string) string {
	if spec, err := removalAssetSpec(name); err == nil {
		return spec.modelName
	}
	if strings.TrimSpace(name) == "" {
		return "<empty>"
	}
	return "<invalid>"
}
