package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
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
	s.logAssetRemovalStart(modelIdentity)
	defer func() {
		s.logAssetRemovalTerminal(modelIdentity, start, result, resultErr)
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
	changed, err := s.removeTree(ctx, parent, spec.modelName)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return models.RemoveModelAssetsResult{}, contextErr
		}
		return models.RemoveModelAssetsResult{}, fmt.Errorf(
			"%w: remove managed model assets: %w", models.ErrAssetUnavailable, err,
		)
	}
	if changed {
		return removedAssetResult(spec.modelName, models.AssetRemovalRemoved), nil
	}
	return removedAssetResult(spec.modelName, models.AssetRemovalAlreadyAbsent), nil
}

func (s *service) modelCacheParent(cacheDirectory string) (string, error) {
	parent := strings.TrimSpace(cacheDirectory)
	if parent != "" {
		return parent, nil
	}
	home, err := s.resolveHome()
	if err != nil {
		return "", fmt.Errorf("resolve managed model cache directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolve managed model cache directory: empty home directory")
	}
	return filepath.Join(home, ".agent-factory", "models"), nil
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
