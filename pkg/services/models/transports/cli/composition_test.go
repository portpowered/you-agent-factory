package cli_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type compositionHTTPClock struct{}

func (compositionHTTPClock) Now() time.Time { return time.Unix(1, 0) }

func compositionHTTPProtocol(t *testing.T) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(&http.Client{}, compositionHTTPClock{})
	if err != nil {
		t.Fatalf("build composition HTTP protocol: %v", err)
	}
	return protocol
}

type compositionModelsRoot struct {
	listModels func(context.Context) (modelinference.List, error)
	getModel   func(context.Context, string) (modelinference.Detail, error)
	pullModel  func(context.Context, string) (modelinference.PullResult, error)
}

func (stub compositionModelsRoot) OpenRuntimeScope(context.Context, modelinference.OpenRuntimeScopeRequest) (modelinference.OpenRuntimeScopeResult, error) {
	return modelinference.OpenRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) CloseRuntimeScope(context.Context, modelinference.CloseRuntimeScopeRequest) (modelinference.CloseRuntimeScopeResult, error) {
	return modelinference.CloseRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) ListCatalog(context.Context, modelinference.ListModelsRequest) (modelinference.ListModelsResult, error) {
	return modelinference.ListModelsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) GetCatalogModel(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
	return modelinference.GetModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) GetModelReadiness(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
	return modelinference.GetModelReadinessResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) PullModelForScope(context.Context, modelinference.PullModelRequest) (modelinference.PullResult, error) {
	return modelinference.PullResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) PrepareModelAssets(context.Context, modelinference.PrepareModelAssetsRequest) (modelinference.PrepareModelAssetsResult, error) {
	return modelinference.PrepareModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) InspectModelAssets(context.Context, modelinference.InspectModelAssetsRequest) (modelinference.InspectModelAssetsResult, error) {
	return modelinference.InspectModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) RemoveModelAssets(context.Context, modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error) {
	return modelinference.RemoveModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) EnsureModelHost(context.Context, modelinference.EnsureModelHostRequest) (modelinference.EnsureModelHostResult, error) {
	return modelinference.EnsureModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) InspectModelHost(context.Context, modelinference.InspectModelHostRequest) (modelinference.InspectModelHostResult, error) {
	return modelinference.InspectModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) StopModelHost(context.Context, modelinference.StopModelHostRequest) (modelinference.StopModelHostResult, error) {
	return modelinference.StopModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) AcquireModelLease(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
	return modelinference.AcquireModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) GetModelLease(context.Context, modelinference.GetModelLeaseRequest) (modelinference.GetModelLeaseResult, error) {
	return modelinference.GetModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) ReleaseModelLease(context.Context, modelinference.ReleaseModelLeaseRequest) (modelinference.ReleaseModelLeaseResult, error) {
	return modelinference.ReleaseModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) InvokeModelWithLease(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
	return modelinference.InvokeModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) CancelInvocation(context.Context, modelinference.CancelInvocationRequest) (modelinference.CancelInvocationResult, error) {
	return modelinference.CancelInvocationResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) ForRuntime(modelinference.RuntimeBinding) (modelinference.Service, error) {
	return nil, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) ListModels(ctx context.Context) (modelinference.List, error) {
	if stub.listModels != nil {
		return stub.listModels(ctx)
	}
	return modelinference.List{}, nil
}

func (stub compositionModelsRoot) GetModel(ctx context.Context, name string) (modelinference.Detail, error) {
	if stub.getModel != nil {
		return stub.getModel(ctx, name)
	}
	return modelinference.Detail{}, modelinference.ErrNotFound
}

func (stub compositionModelsRoot) PullModel(ctx context.Context, name string) (modelinference.PullResult, error) {
	if stub.pullModel != nil {
		return stub.pullModel(ctx, name)
	}
	return modelinference.PullResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) InspectRuntime(context.Context, string) (modelinference.Runtime, error) {
	return modelinference.Runtime{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) AcquireLease(context.Context, modelinference.AcquireLeaseRequest) (modelinference.HostLease, error) {
	return modelinference.HostLease{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) ReleaseLease(context.Context, modelinference.ReleaseLeaseRequest) error {
	return modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) InvokeLocal(context.Context, modelinference.LocalInvocationRequest) (modelinference.LocalInvocationResult, error) {
	return modelinference.LocalInvocationResult{}, modelinference.ErrUnsupportedOperation
}

type compositionInvocation struct {
	root      modelinference.Service
	openScope func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error)
}

func (inv compositionInvocation) InvokeModel(
	context.Context,
	factorysessions.InvocationTarget,
	string,
	modelinference.Request,
) (modelinference.Result, error) {
	return modelinference.Result{}, errors.New("composition test does not invoke through bootstrap")
}

func (inv compositionInvocation) ResolveModelInvocationFactoryDir(string) (string, error) {
	return "/tmp/factory", nil
}

func (inv compositionInvocation) ExportModelInvocationArtifact(string, string) error {
	return nil
}

func (inv compositionInvocation) CompositionModelsRoot() modelinference.Service {
	return inv.root
}

func (inv compositionInvocation) CompositionOpenInvokeScope(
	ctx context.Context,
	cfg modelscli.InvokeConfig,
) (modelscli.InvokeRuntimeScope, error) {
	if inv.openScope != nil {
		return inv.openScope(ctx, cfg)
	}
	return modelscli.InvokeRuntimeScope{}, nil
}

func TestBindServiceDelegatesThroughAdapterService(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	service := modelscli.BindService(modelscli.Config{
		Models: compositionModelsRoot{
			listModels: func(context.Context) (modelinference.List, error) {
				return modelinference.List{
					Results: []modelinference.Summary{{Name: "OMNIVOICE_Q4_K_M"}},
				}, nil
			},
		},
	})
	if service == nil {
		t.Fatal("BindService() = nil, want Models CLI service")
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

func TestBindServiceMatchesNewServiceFacade(t *testing.T) {
	t.Parallel()

	root := compositionModelsRoot{
		getModel: func(context.Context, string) (modelinference.Detail, error) {
			return modelinference.Detail{}, modelinference.ErrNotFound
		},
	}
	bound := modelscli.BindService(modelscli.Config{Models: root})
	constructed := modelscli.NewService(modelscli.Config{Models: root})
	if bound == nil || constructed == nil {
		t.Fatal("BindService() or NewService() = nil, want Models CLI service")
	}
	boundErr := bound.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: "missing", Output: new(bytes.Buffer),
	})
	constructedErr := constructed.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: "missing", Output: new(bytes.Buffer),
	})
	if (boundErr == nil) != (constructedErr == nil) {
		t.Fatalf("bound error = %v, constructed error = %v", boundErr, constructedErr)
	}
	if boundErr != nil && !errors.Is(boundErr, modelscli.ErrModelNotFound) {
		t.Fatalf("bound error = %v, want ErrModelNotFound", boundErr)
	}
}

func TestNewDelegatesThroughOwnedServiceWhenModelsRootAvailable(t *testing.T) {
	t.Parallel()

	var inspected string
	root := compositionModelsRoot{
		getModel: func(_ context.Context, name string) (modelinference.Detail, error) {
			inspected = name
			return modelinference.Detail{Summary: modelinference.Summary{Name: name}}, nil
		},
	}
	service := modelscli.New(
		compositionHTTPProtocol(t),
		compositionInvocation{root: root},
	)
	if service == nil {
		t.Fatal("New() = nil, want composition facade")
	}
	var out bytes.Buffer
	if err := service.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &out,
	}); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if inspected != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("inspected model = %q, want OMNIVOICE_Q4_K_M", inspected)
	}
	if !strings.Contains(out.String(), "OMNIVOICE_Q4_K_M") {
		t.Fatalf("Inspect() output = %q, want model name", out.String())
	}
}

func TestNewPreservesCompositionStableCollaboratorShapes(t *testing.T) {
	t.Parallel()

	if service := modelscli.New(nil, compositionInvocation{root: compositionModelsRoot{}}); service != nil {
		t.Fatalf("New(nil, invocation) = %T, want nil without HTTP protocol", service)
	}
	if service := modelscli.New(compositionHTTPProtocol(t), nil); service != nil {
		t.Fatalf("New(http, nil) = %T, want nil without invocation operation", service)
	}
}

func TestConfigFromCompositionMapsRepresentativeCommandInputs(t *testing.T) {
	t.Parallel()

	root := compositionModelsRoot{
		pullModel: func(_ context.Context, name string) (modelinference.PullResult, error) {
			if name != "OMNIVOICE_Q4_K_M" {
				t.Fatalf("pull model = %q, want OMNIVOICE_Q4_K_M", name)
			}
			return modelinference.PullResult{ModelName: name}, nil
		},
	}
	service := modelscli.NewService(modelscli.ConfigFromComposition(
		compositionHTTPProtocol(t),
		compositionInvocation{root: root},
	))
	if service == nil {
		t.Fatal("NewService(ConfigFromComposition()) = nil, want owned service")
	}
	var out bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &out,
	}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !strings.Contains(out.String(), "OMNIVOICE_Q4_K_M") {
		t.Fatalf("Pull() output = %q, want model name", out.String())
	}
}
