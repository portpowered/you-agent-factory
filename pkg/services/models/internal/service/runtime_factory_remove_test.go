package service

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

func TestRootRemoveModelAssetsRefusesAnInUseCacheBeforeMutation(t *testing.T) {
	scope, err := (models.RuntimeScopeRef{}).Parse("remove-guard:scope")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}

	assets := &removeGuardAssets{
		inspection: scopedassets.RuntimeCacheInspection{Installed: true},
	}
	root := &Root{
		assets:      assets,
		runtimeHost: &removeGuardHost{stopErr: models.ErrHostCapacityExhausted},
	}

	_, err = root.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: scope,
		Name:  "managed-model",
	})
	if !errors.Is(err, models.ErrModelCacheInUse) {
		t.Fatalf("RemoveModelAssets error = %v, want ErrModelCacheInUse", err)
	}
	if assets.removeCalls != 0 {
		t.Fatalf("asset removal calls = %d, want 0 while cache is in use", assets.removeCalls)
	}
}

type removeGuardAssets struct {
	inspection  scopedassets.RuntimeCacheInspection
	removeCalls int
}

func (assets *removeGuardAssets) PrepareModelAssets(context.Context, models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *removeGuardAssets) InspectModelAssets(context.Context, models.InspectModelAssetsRequest) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *removeGuardAssets) RemoveModelAssets(context.Context, models.RemoveModelAssetsRequest) (models.RemoveModelAssetsResult, error) {
	assets.removeCalls++
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *removeGuardAssets) ResolveRuntimeCache(context.Context, models.InspectModelAssetsRequest) (scopedassets.RuntimeCacheLayout, error) {
	return scopedassets.RuntimeCacheLayout{}, models.ErrUnsupportedOperation
}

func (assets *removeGuardAssets) InspectRuntimeCache(context.Context, models.InspectModelAssetsRequest) (scopedassets.RuntimeCacheInspection, error) {
	return assets.inspection, nil
}

type removeGuardHost struct {
	stopErr error
}

func (host *removeGuardHost) InspectModelHost(context.Context, models.InspectModelHostRequest) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (host *removeGuardHost) EnsureModelHost(context.Context, models.EnsureModelHostRequest) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (host *removeGuardHost) StopModelHost(context.Context, models.StopModelHostRequest) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, host.stopErr
}

func (host *removeGuardHost) AcquireModelLease(context.Context, models.AcquireModelLeaseRequest) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (host *removeGuardHost) GetModelLease(context.Context, models.GetModelLeaseRequest) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (host *removeGuardHost) ReleaseModelLease(context.Context, models.ReleaseModelLeaseRequest) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}
