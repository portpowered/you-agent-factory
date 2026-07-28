package http

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// rootFake is a focused Models root fake for adapter-edge tests. It avoids
// constructing catalog assemblers, asset caches, host supervisors, lease
// managers, inference runtimes, or service-local Wire graphs.
type rootFake struct {
	models.Service

	list         func(context.Context) (models.List, error)
	get          func(context.Context, string) (models.Detail, error)
	pull         func(context.Context, string) (models.PullResult, error)
	listCatalog  func(context.Context, models.ListModelsRequest) (models.ListModelsResult, error)
	getCatalog   func(context.Context, models.GetModelRequest) (models.GetModelResult, error)
	pullForScope func(context.Context, models.PullModelRequest) (models.PullResult, error)
}

var _ models.Service = (*rootFake)(nil)

func (fake *rootFake) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return fake, nil
}

func (fake *rootFake) ListModels(ctx context.Context) (models.List, error) {
	if fake.list != nil {
		return fake.list(ctx)
	}
	return models.List{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) GetModel(ctx context.Context, name string) (models.Detail, error) {
	if fake.get != nil {
		return fake.get(ctx, name)
	}
	return models.Detail{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) PullModel(ctx context.Context, name string) (models.PullResult, error) {
	if fake.pull != nil {
		return fake.pull(ctx, name)
	}
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) ListCatalog(
	ctx context.Context,
	request models.ListModelsRequest,
) (models.ListModelsResult, error) {
	if fake.listCatalog != nil {
		return fake.listCatalog(ctx, request)
	}
	return models.ListModelsResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) GetCatalogModel(
	ctx context.Context,
	request models.GetModelRequest,
) (models.GetModelResult, error) {
	if fake.getCatalog != nil {
		return fake.getCatalog(ctx, request)
	}
	return models.GetModelResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) PullModelForScope(
	ctx context.Context,
	request models.PullModelRequest,
) (models.PullResult, error) {
	if fake.pullForScope != nil {
		return fake.pullForScope(ctx, request)
	}
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) OpenRuntimeScope(
	context.Context,
	models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	return models.OpenRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) CloseRuntimeScope(
	context.Context,
	models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	return models.CloseRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) GetModelReadiness(
	context.Context,
	models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	return models.GetModelReadinessResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) InspectModelHost(
	context.Context,
	models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) ReleaseModelLease(
	context.Context,
	models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) InvokeModelWithLease(
	context.Context,
	models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) CancelInvocation(
	context.Context,
	models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) AcquireLease(
	context.Context,
	models.AcquireLeaseRequest,
) (models.HostLease, error) {
	return models.HostLease{}, models.ErrUnsupportedOperation
}

func (fake *rootFake) ReleaseLease(context.Context, models.ReleaseLeaseRequest) error {
	return models.ErrUnsupportedOperation
}

func (fake *rootFake) InvokeLocal(
	context.Context,
	models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{}, models.ErrUnsupportedOperation
}
