package models_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func (unsupportedRuntimeScopePeer) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

// assetScopePeerService proves a peer can implement the scoped asset lifecycle
// with only detached root-package values.
type assetScopePeerService struct {
	*runtimeScopePeerService
	assets   map[string]models.AssetSnapshot
	failures map[string]error
}

func newAssetScopePeerService() *assetScopePeerService {
	return &assetScopePeerService{
		runtimeScopePeerService: newRuntimeScopePeerService("asset-peer"),
		assets:                  make(map[string]models.AssetSnapshot),
		failures:                make(map[string]error),
	}
}

func (s *assetScopePeerService) PrepareModelAssets(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	if err := request.Validate(); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	if err := assetContextError(ctx); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	if err := s.failures["prepare:"+request.Name]; err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	if asset, ok := s.assets[request.Name]; ok && asset.Readiness == models.AssetReadinessAvailable {
		return models.PrepareModelAssetsResult{
			Asset:   asset.Clone(),
			Outcome: models.AssetPreparationAlreadyAvailable,
		}, nil
	}
	asset := models.AssetSnapshot{
		ModelName: request.Name,
		Readiness: models.AssetReadinessAvailable,
		Integrity: models.AssetIntegrityVerified,
		Source: models.SourceMetadata{
			Provider:  "managed-mirror",
			Reference: "models/" + request.Name,
			Revision:  "sha256:prepared",
		},
		Revision:   "sha256:prepared",
		Artifacts:  []models.AssetArtifact{{Name: "weights.bin", Bytes: 42, SHA256: "abc"}},
		TotalBytes: 42,
	}
	s.assets[request.Name] = asset.Clone()
	return models.PrepareModelAssetsResult{
		Asset:   asset.Clone(),
		Outcome: models.AssetPreparationPrepared,
	}, nil
}

func (s *assetScopePeerService) InspectModelAssets(
	ctx context.Context,
	request models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	if err := request.Validate(); err != nil {
		return models.InspectModelAssetsResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.InspectModelAssetsResult{}, err
	}
	if err := assetContextError(ctx); err != nil {
		return models.InspectModelAssetsResult{}, err
	}
	if err := s.failures["inspect:"+request.Name]; err != nil {
		return models.InspectModelAssetsResult{}, err
	}
	asset, ok := s.assets[request.Name]
	if !ok {
		return models.InspectModelAssetsResult{}, fmt.Errorf(
			"%w: %s", models.ErrAssetUnavailable, request.Name,
		)
	}
	if request.VerifyIntegrity && asset.Integrity != models.AssetIntegrityVerified {
		return models.InspectModelAssetsResult{}, models.ErrAssetIntegrityFailed
	}
	return models.InspectModelAssetsResult{Asset: asset.Clone()}, nil
}

func (s *assetScopePeerService) RemoveModelAssets(
	ctx context.Context,
	request models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	if err := request.Validate(); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	if err := assetContextError(ctx); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	if err := s.failures["remove:"+request.Name]; err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	outcome := models.AssetRemovalAlreadyAbsent
	if _, ok := s.assets[request.Name]; ok {
		delete(s.assets, request.Name)
		outcome = models.AssetRemovalRemoved
	}
	return models.RemoveModelAssetsResult{
		ModelName: request.Name,
		Readiness: models.AssetReadinessMissing,
		Outcome:   outcome,
	}, nil
}

func assetContextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", models.ErrAssetCancelled, ctx.Err())
}

func TestAssetLifecycle_PrepareInspectAndRemoveReturnDetachedFacts(t *testing.T) {
	t.Parallel()

	fake := newAssetScopePeerService()
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	prepared, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: opened.Scope,
		Name:  "local-model",
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	assertPreparedAssetFacts(t, prepared)

	prepared.Asset.Artifacts[0].Name = "peer-mutated"
	inspected, err := service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
		Scope:           opened.Scope,
		Name:            "local-model",
		VerifyIntegrity: true,
	})
	if err != nil {
		t.Fatalf("InspectModelAssets: %v", err)
	}
	if inspected.Asset.Artifacts[0].Name != "weights.bin" {
		t.Fatalf("InspectModelAssets retained peer mutation: %#v", inspected.Asset.Artifacts)
	}

	cacheHit, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: opened.Scope,
		Name:  "local-model",
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets cache hit: %v", err)
	}
	if cacheHit.Outcome != models.AssetPreparationAlreadyAvailable {
		t.Fatalf("cache-hit outcome = %q, want %q", cacheHit.Outcome, models.AssetPreparationAlreadyAvailable)
	}

	removed, err := service.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: opened.Scope,
		Name:  "local-model",
	})
	if err != nil {
		t.Fatalf("RemoveModelAssets: %v", err)
	}
	assertRemovedAssetFacts(t, removed)
	removedAgain, err := service.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: opened.Scope,
		Name:  "local-model",
	})
	if err != nil {
		t.Fatalf("RemoveModelAssets repeated: %v", err)
	}
	if removedAgain.Outcome != models.AssetRemovalAlreadyAbsent {
		t.Fatalf("repeated removal outcome = %q, want %q", removedAgain.Outcome, models.AssetRemovalAlreadyAbsent)
	}
}

func assertPreparedAssetFacts(t *testing.T, prepared models.PrepareModelAssetsResult) {
	t.Helper()
	if prepared.Outcome != models.AssetPreparationPrepared ||
		prepared.Asset.Readiness != models.AssetReadinessAvailable ||
		prepared.Asset.Integrity != models.AssetIntegrityVerified {
		t.Fatalf("PrepareModelAssets = %#v, want newly prepared available/verified assets", prepared)
	}
	if len(prepared.Asset.Artifacts) != 1 || prepared.Asset.Artifacts[0].Name != "weights.bin" {
		t.Fatalf("PrepareModelAssets artifacts = %#v, want detached artifact facts", prepared.Asset.Artifacts)
	}
}

func assertRemovedAssetFacts(t *testing.T, removed models.RemoveModelAssetsResult) {
	t.Helper()
	if removed.Outcome != models.AssetRemovalRemoved || removed.Readiness != models.AssetReadinessMissing {
		t.Fatalf("RemoveModelAssets = %#v, want removed/missing", removed)
	}
}

func TestAssetLifecycle_NormalizedFailuresRemainDistinct(t *testing.T) {
	t.Parallel()

	fake := newAssetScopePeerService()
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	foreign, err := (models.RuntimeScopeRef{}).Parse("other:1")
	if err != nil {
		t.Fatalf("Parse foreign scope: %v", err)
	}
	stale, err := (models.RuntimeScopeRef{}).Parse("asset-peer:999")
	if err != nil {
		t.Fatalf("Parse stale scope: %v", err)
	}

	assertPrepareAssetErrorIs(t, service, models.PrepareModelAssetsRequest{Name: "model"}, models.ErrRuntimeScopeInvalid)
	assertPrepareAssetErrorIs(t, service, models.PrepareModelAssetsRequest{
		Scope: foreign, Name: "model",
	}, models.ErrRuntimeScopeForeign)
	assertPrepareAssetErrorIs(t, service, models.PrepareModelAssetsRequest{
		Scope: stale, Name: "model",
	}, models.ErrRuntimeScopeStale)

	failures := []struct {
		name string
		err  error
	}{
		{name: "missing-source", err: models.ErrAssetSourceMissing},
		{name: "unsupported-source", err: models.ErrAssetSourceUnsupported},
		{name: "interrupted", err: models.ErrAssetPreparationInterrupted},
	}
	for _, failure := range failures {
		fake.failures["prepare:"+failure.name] = failure.err
		assertPrepareAssetErrorIs(t, service, models.PrepareModelAssetsRequest{
			Scope: opened.Scope, Name: failure.name,
		}, failure.err)
	}

	_, err = service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
		Scope: opened.Scope, Name: "missing-assets",
	})
	if !errors.Is(err, models.ErrAssetUnavailable) {
		t.Fatalf("InspectModelAssets unavailable = %v, want ErrAssetUnavailable", err)
	}
	fake.failures["inspect:corrupt"] = models.ErrAssetIntegrityFailed
	_, err = service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
		Scope: opened.Scope, Name: "corrupt", VerifyIntegrity: true,
	})
	if !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("InspectModelAssets integrity = %v, want ErrAssetIntegrityFailed", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.PrepareModelAssets(cancelled, models.PrepareModelAssetsRequest{
		Scope: opened.Scope, Name: "cancelled",
	})
	if !errors.Is(err, models.ErrAssetCancelled) {
		t.Fatalf("PrepareModelAssets cancelled = %v, want ErrAssetCancelled", err)
	}

	if _, err := service.CloseRuntimeScope(context.Background(), models.CloseRuntimeScopeRequest{
		Scope: opened.Scope,
	}); err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}
	assertPrepareAssetErrorIs(t, service, models.PrepareModelAssetsRequest{
		Scope: opened.Scope, Name: "model",
	}, models.ErrRuntimeScopeClosed)
}

func assertPrepareAssetErrorIs(
	t *testing.T,
	service models.Service,
	request models.PrepareModelAssetsRequest,
	want error,
) {
	t.Helper()
	_, err := service.PrepareModelAssets(context.Background(), request)
	if !errors.Is(err, want) {
		t.Fatalf("PrepareModelAssets(%q) = %v, want %v", request.Name, err, want)
	}
}
