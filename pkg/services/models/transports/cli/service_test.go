package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
)

type stubModelsRoot struct {
	listModels         func(context.Context) (modelinference.List, error)
	getModel           func(context.Context, string) (modelinference.Detail, error)
	pullModel          func(context.Context, string) (modelinference.PullResult, error)
	getCatalogModel    func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error)
	acquireModelLease  func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error)
	invokeModelWithLease func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error)
}

func (stub stubModelsRoot) OpenRuntimeScope(context.Context, modelinference.OpenRuntimeScopeRequest) (modelinference.OpenRuntimeScopeResult, error) {
	return modelinference.OpenRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) CloseRuntimeScope(context.Context, modelinference.CloseRuntimeScopeRequest) (modelinference.CloseRuntimeScopeResult, error) {
	return modelinference.CloseRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) ListCatalog(context.Context, modelinference.ListModelsRequest) (modelinference.ListModelsResult, error) {
	return modelinference.ListModelsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) GetCatalogModel(ctx context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
	if stub.getCatalogModel != nil {
		return stub.getCatalogModel(ctx, request)
	}
	return modelinference.GetModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) GetModelReadiness(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
	return modelinference.GetModelReadinessResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) PullModelForScope(context.Context, modelinference.PullModelRequest) (modelinference.PullResult, error) {
	return modelinference.PullResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) PrepareModelAssets(context.Context, modelinference.PrepareModelAssetsRequest) (modelinference.PrepareModelAssetsResult, error) {
	return modelinference.PrepareModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) InspectModelAssets(context.Context, modelinference.InspectModelAssetsRequest) (modelinference.InspectModelAssetsResult, error) {
	return modelinference.InspectModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) RemoveModelAssets(context.Context, modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error) {
	return modelinference.RemoveModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) EnsureModelHost(context.Context, modelinference.EnsureModelHostRequest) (modelinference.EnsureModelHostResult, error) {
	return modelinference.EnsureModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) InspectModelHost(context.Context, modelinference.InspectModelHostRequest) (modelinference.InspectModelHostResult, error) {
	return modelinference.InspectModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) StopModelHost(context.Context, modelinference.StopModelHostRequest) (modelinference.StopModelHostResult, error) {
	return modelinference.StopModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) AcquireModelLease(ctx context.Context, request modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
	if stub.acquireModelLease != nil {
		return stub.acquireModelLease(ctx, request)
	}
	return modelinference.AcquireModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) GetModelLease(context.Context, modelinference.GetModelLeaseRequest) (modelinference.GetModelLeaseResult, error) {
	return modelinference.GetModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) ReleaseModelLease(context.Context, modelinference.ReleaseModelLeaseRequest) (modelinference.ReleaseModelLeaseResult, error) {
	return modelinference.ReleaseModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) InvokeModelWithLease(ctx context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
	if stub.invokeModelWithLease != nil {
		return stub.invokeModelWithLease(ctx, request)
	}
	return modelinference.InvokeModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) CancelInvocation(context.Context, modelinference.CancelInvocationRequest) (modelinference.CancelInvocationResult, error) {
	return modelinference.CancelInvocationResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) ForRuntime(modelinference.RuntimeBinding) (modelinference.Service, error) {
	return nil, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) ListModels(ctx context.Context) (modelinference.List, error) {
	if stub.listModels != nil {
		return stub.listModels(ctx)
	}
	return modelinference.List{}, nil
}

func (stub stubModelsRoot) GetModel(ctx context.Context, name string) (modelinference.Detail, error) {
	if stub.getModel != nil {
		return stub.getModel(ctx, name)
	}
	return modelinference.Detail{}, modelinference.ErrNotFound
}

func (stub stubModelsRoot) PullModel(ctx context.Context, name string) (modelinference.PullResult, error) {
	if stub.pullModel != nil {
		return stub.pullModel(ctx, name)
	}
	return modelinference.PullResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) InspectRuntime(context.Context, string) (modelinference.Runtime, error) {
	return modelinference.Runtime{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) AcquireLease(context.Context, modelinference.AcquireLeaseRequest) (modelinference.HostLease, error) {
	return modelinference.HostLease{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) ReleaseLease(context.Context, modelinference.ReleaseLeaseRequest) error {
	return modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) InvokeLocal(context.Context, modelinference.LocalInvocationRequest) (modelinference.LocalInvocationResult, error) {
	return modelinference.LocalInvocationResult{}, modelinference.ErrUnsupportedOperation
}

func TestNewServiceRequiresModelsRoot(t *testing.T) {
	t.Parallel()
	if service := modelscli.NewService(modelscli.Config{}); service != nil {
		t.Fatalf("NewService() = %T, want nil without Models root", service)
	}
}

func TestConstructedService_ListSuccessThroughModelsRoot(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			listModels: func(context.Context) (modelinference.List, error) {
				return modelinference.List{
					Results: []modelinference.Summary{{
						Name: "OMNIVOICE_Q4_K_M",
						ManagedRuntime: modelinference.Runtime{
							ReadinessState: modelinference.ReadinessStateReady,
							LifecycleState: modelinference.LifecycleStateInstalled,
						},
					}},
				}, nil
			},
		},
	})
	if service == nil {
		t.Fatal("NewService() = nil, want Models CLI service")
	}
	if err := service.List(modelscli.ListConfig{
		Context: context.Background(),
		Output:  &out,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !strings.Contains(out.String(), "OMNIVOICE_Q4_K_M") {
		t.Fatalf("List() output = %q, want model name", out.String())
	}
}

func TestConstructedService_InspectMapsModelsRootNotFound(t *testing.T) {
	t.Parallel()

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getModel: func(context.Context, string) (modelinference.Detail, error) {
				return modelinference.Detail{}, modelinference.ErrNotFound
			},
		},
	})
	if service == nil {
		t.Fatal("NewService() = nil, want Models CLI service")
	}
	err := service.Inspect(modelscli.InspectConfig{
		Context:   context.Background(),
		ModelName: "missing-model",
		Output:    new(bytes.Buffer),
	})
	if err == nil {
		t.Fatal("Inspect() error = nil, want not-found failure")
	}
	if !errors.Is(err, modelscli.ErrModelNotFound) {
		t.Fatalf("Inspect() error = %v, want ErrModelNotFound", err)
	}
}

func TestBindServiceDelegatesToNewService(t *testing.T) {
	t.Parallel()

	service := modelscli.BindService(modelscli.Config{
		Models: stubModelsRoot{
			listModels: func(context.Context) (modelinference.List, error) {
				return modelinference.List{}, nil
			},
		},
	})
	if service == nil {
		t.Fatal("BindService() = nil, want Models CLI service")
	}
	if err := service.List(modelscli.ListConfig{
		Context: context.Background(),
		Output:  new(bytes.Buffer),
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
}
