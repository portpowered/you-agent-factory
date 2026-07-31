package model_list_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type functionalModelsRoot struct {
	listModels           func(context.Context) (modelinference.List, error)
	getModel             func(context.Context, string) (modelinference.Detail, error)
	pullModel            func(context.Context, string) (modelinference.PullResult, error)
	getCatalogModel      func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error)
	acquireModelLease    func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error)
	invokeModelWithLease func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error)
}

func (stub functionalModelsRoot) OpenRuntimeScope(context.Context, modelinference.OpenRuntimeScopeRequest) (modelinference.OpenRuntimeScopeResult, error) {
	return modelinference.OpenRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) CloseRuntimeScope(context.Context, modelinference.CloseRuntimeScopeRequest) (modelinference.CloseRuntimeScopeResult, error) {
	return modelinference.CloseRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) ListCatalog(ctx context.Context, request modelinference.ListModelsRequest) (modelinference.ListModelsResult, error) {
	if stub.listModels != nil {
		list, err := stub.listModels(ctx)
		if err != nil {
			return modelinference.ListModelsResult{}, err
		}
		return modelinference.ListModelsResult{Models: list.Results}, nil
	}
	return modelinference.ListModelsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) GetCatalogModel(ctx context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
	if stub.getCatalogModel != nil {
		return stub.getCatalogModel(ctx, request)
	}
	if stub.getModel != nil {
		detail, err := stub.getModel(ctx, request.Name)
		if err != nil {
			return modelinference.GetModelResult{}, err
		}
		return modelinference.GetModelResult{Model: detail}, nil
	}
	return modelinference.GetModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) GetModelReadiness(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
	return modelinference.GetModelReadinessResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) PullModelForScope(ctx context.Context, request modelinference.PullModelRequest) (modelinference.PullResult, error) {
	if stub.pullModel != nil {
		return stub.pullModel(ctx, request.Name)
	}
	return modelinference.PullResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) PrepareModelAssets(context.Context, modelinference.PrepareModelAssetsRequest) (modelinference.PrepareModelAssetsResult, error) {
	return modelinference.PrepareModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) InspectModelAssets(context.Context, modelinference.InspectModelAssetsRequest) (modelinference.InspectModelAssetsResult, error) {
	return modelinference.InspectModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) RemoveModelAssets(context.Context, modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error) {
	return modelinference.RemoveModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) EnsureModelHost(context.Context, modelinference.EnsureModelHostRequest) (modelinference.EnsureModelHostResult, error) {
	return modelinference.EnsureModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) InspectModelHost(context.Context, modelinference.InspectModelHostRequest) (modelinference.InspectModelHostResult, error) {
	return modelinference.InspectModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) StopModelHost(context.Context, modelinference.StopModelHostRequest) (modelinference.StopModelHostResult, error) {
	return modelinference.StopModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) AcquireModelLease(ctx context.Context, request modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
	if stub.acquireModelLease != nil {
		return stub.acquireModelLease(ctx, request)
	}
	return modelinference.AcquireModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) GetModelLease(context.Context, modelinference.GetModelLeaseRequest) (modelinference.GetModelLeaseResult, error) {
	return modelinference.GetModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) ReleaseModelLease(context.Context, modelinference.ReleaseModelLeaseRequest) (modelinference.ReleaseModelLeaseResult, error) {
	return modelinference.ReleaseModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) InvokeModelWithLease(ctx context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
	if stub.invokeModelWithLease != nil {
		return stub.invokeModelWithLease(ctx, request)
	}
	return modelinference.InvokeModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) CancelInvocation(context.Context, modelinference.CancelInvocationRequest) (modelinference.CancelInvocationResult, error) {
	return modelinference.CancelInvocationResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) ForRuntime(modelinference.RuntimeBinding) (modelinference.Service, error) {
	return nil, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) ListModels(ctx context.Context) (modelinference.List, error) {
	if stub.listModels != nil {
		return stub.listModels(ctx)
	}
	return modelinference.List{}, nil
}

func (stub functionalModelsRoot) GetModel(ctx context.Context, name string) (modelinference.Detail, error) {
	if stub.getModel != nil {
		return stub.getModel(ctx, name)
	}
	return modelinference.Detail{}, modelinference.ErrNotFound
}

func (stub functionalModelsRoot) PullModel(ctx context.Context, name string) (modelinference.PullResult, error) {
	if stub.pullModel != nil {
		return stub.pullModel(ctx, name)
	}
	return modelinference.PullResult{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) InspectRuntime(context.Context, string) (modelinference.Runtime, error) {
	return modelinference.Runtime{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) AcquireLease(context.Context, modelinference.AcquireLeaseRequest) (modelinference.HostLease, error) {
	return modelinference.HostLease{}, modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) ReleaseLease(context.Context, modelinference.ReleaseLeaseRequest) error {
	return modelinference.ErrUnsupportedOperation
}

func (stub functionalModelsRoot) InvokeLocal(context.Context, modelinference.LocalInvocationRequest) (modelinference.LocalInvocationResult, error) {
	return modelinference.LocalInvocationResult{}, modelinference.ErrUnsupportedOperation
}

type functionalHTTPClock struct{}

func (functionalHTTPClock) Now() time.Time { return time.Unix(1, 0) }

func functionalHTTPProtocol(t *testing.T) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(&http.Client{}, functionalHTTPClock{})
	if err != nil {
		t.Fatalf("build functional HTTP protocol: %v", err)
	}
	return protocol
}

type functionalCompositionInvocation struct {
	root functionalModelsRoot
}

func (inv functionalCompositionInvocation) InvokeModel(
	context.Context,
	factorysessions.InvocationTarget,
	string,
	modelinference.Request,
) (modelinference.Result, error) {
	return modelinference.Result{}, errors.New("functional coverage does not invoke through bootstrap")
}

func (inv functionalCompositionInvocation) ResolveModelInvocationFactoryDir(string) (string, error) {
	return "/tmp/factory", nil
}

func (inv functionalCompositionInvocation) ExportModelInvocationArtifact(string, string) error {
	return nil
}

func (inv functionalCompositionInvocation) CompositionModelsRoot() modelinference.Service {
	return inv.root
}

func (inv functionalCompositionInvocation) CompositionOpenCatalogScope(
	context.Context,
) (modelscli.InvokeRuntimeScope, error) {
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("functional-coverage:test-scope")
	if err != nil {
		return modelscli.InvokeRuntimeScope{}, err
	}
	return modelscli.InvokeRuntimeScope{Scope: scope}, nil
}

func (inv functionalCompositionInvocation) CompositionOpenInvokeScope(
	ctx context.Context,
	_ modelscli.InvokeConfig,
) (modelscli.InvokeRuntimeScope, error) {
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("functional-coverage:test-scope")
	if err != nil {
		return modelscli.InvokeRuntimeScope{}, err
	}
	return modelscli.InvokeRuntimeScope{Scope: scope}, nil
}

func functionalOwnedService(t *testing.T, root functionalModelsRoot) modelscli.Service {
	t.Helper()
	openScope := func(context.Context) (modelscli.InvokeRuntimeScope, error) {
		scope, err := (modelinference.RuntimeScopeRef{}).Parse("functional-coverage:test-scope")
		if err != nil {
			return modelscli.InvokeRuntimeScope{}, err
		}
		return modelscli.InvokeRuntimeScope{Scope: scope}, nil
	}
	service := modelscli.BindService(modelscli.Config{
		Models:           root,
		OpenCatalogScope: openScope,
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return openScope(context.Background())
		},
	})
	if service == nil {
		t.Fatal("BindService() = nil, want owned Models CLI service")
	}
	return service
}

// TestFunctionalModelsOwnedAdapter_ListInspectPullPreserveAcceptedOutput proves
// owned adapter list/inspect/pull behavior through the Models root.
func TestFunctionalModelsOwnedAdapter_ListInspectPullPreserveAcceptedOutput(t *testing.T) {
	t.Parallel()

	root := functionalModelsRoot{
		listModels: func(context.Context) (modelinference.List, error) {
			return modelinference.List{
				Results: []modelinference.Summary{{
					Name: "OMNIVOICE_Q4_K_M",
					ManagedRuntime: modelinference.Runtime{
						ReadinessState: modelinference.ReadinessStateReady,
						LifecycleState: modelinference.LifecycleStateInstalled,
					},
					Operations: []modelinference.Operation{{Name: "TTS"}},
					Modalities: []string{"TEXT"},
				}},
			}, nil
		},
		getModel: func(_ context.Context, name string) (modelinference.Detail, error) {
			return modelinference.Detail{
				Summary: modelinference.Summary{
					Name: name,
					ManagedRuntime: modelinference.Runtime{
						ReadinessState: modelinference.ReadinessStateReady,
						LifecycleState: modelinference.LifecycleStateInstalled,
					},
					Operations: []modelinference.Operation{{Name: "TTS"}},
				},
				Capabilities: []modelinference.Capability{{
					Worker: "voice-local",
					Operations: []modelinference.Operation{{
						Name: "TTS",
					}},
				}},
			}, nil
		},
		pullModel: func(_ context.Context, name string) (modelinference.PullResult, error) {
			return modelinference.PullResult{
				ModelName:          name,
				ProviderLocality:   "LOCAL",
				Outcome:            "PULLED",
				CachePath:          "/tmp/models/" + name,
				Revision:           "rev1",
				ManagedPullOutcome: "INSTALLED_SUCCESSFULLY",
				ReadinessState:     "READY",
				DownloadedFiles:    []modelinference.DownloadedFile{{Path: "weights.gguf", Bytes: 42}},
			}, nil
		},
	}
	service := functionalOwnedService(t, root)

	var listHuman bytes.Buffer
	if err := service.List(modelscli.ListConfig{
		Context: context.Background(),
		Output:  &listHuman,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", "READY", "TTS"} {
		if !strings.Contains(listHuman.String(), want) {
			t.Fatalf("List() human output missing %q:\n%s", want, listHuman.String())
		}
	}

	var inspectHuman bytes.Buffer
	if err := service.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &inspectHuman,
	}); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	for _, want := range []string{"Name:\tOMNIVOICE_Q4_K_M", "voice-local"} {
		if !strings.Contains(inspectHuman.String(), want) {
			t.Fatalf("Inspect() human output missing %q:\n%s", want, inspectHuman.String())
		}
	}

	var pullHuman bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &pullHuman,
	}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", "INSTALLED_SUCCESSFULLY", "weights.gguf"} {
		if !strings.Contains(pullHuman.String(), want) {
			t.Fatalf("Pull() human output missing %q:\n%s", want, pullHuman.String())
		}
	}
}

// TestFunctionalModelsCompositionFacade_RoutesOwnedListThroughModelsRoot proves
// composition-stable New() delegates durable behavior through the owned adapter.
func TestFunctionalModelsCompositionFacade_RoutesOwnedListThroughModelsRoot(t *testing.T) {
	t.Parallel()

	root := functionalModelsRoot{
		listModels: func(context.Context) (modelinference.List, error) {
			return modelinference.List{
				Results: []modelinference.Summary{{Name: "OMNIVOICE_Q4_K_M"}},
			}, nil
		},
	}
	service := modelscli.New(
		functionalHTTPProtocol(t),
		functionalCompositionInvocation{root: root},
	)
	if service == nil {
		t.Fatal("New() = nil, want composition facade")
	}

	var out bytes.Buffer
	if err := service.List(modelscli.ListConfig{
		Context: context.Background(),
		JSON:    true,
		Output:  &out,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var response factoryapi.ListModelsResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode list output: %v\n%s", err, out.String())
	}
	if len(response.Results) != 1 || response.Results[0].Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("list response = %#v, want OMNIVOICE_Q4_K_M", response)
	}
}

// TestFunctionalModelsOwnedAdapter_InvokeResolvesThroughModelsRoot proves invoke
// catalog→lease→inference ordering through the owned adapter.
func TestFunctionalModelsOwnedAdapter_InvokeResolvesThroughModelsRoot(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("functional-coverage:test-scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	lease, err := (modelinference.ModelLeaseRef{}).Parse("functional-coverage:test-lease")
	if err != nil {
		t.Fatalf("parse model lease: %v", err)
	}
	var gotCatalog, gotAcquire, gotInvoke bool
	service := modelscli.NewService(modelscli.Config{
		Models: functionalModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				gotCatalog = true
				if request.Scope != scope || request.Name != "OMNIVOICE_Q4_K_M" || request.Operation != "TTS" {
					t.Fatalf("GetCatalogModel request = %#v", request)
				}
				return modelinference.GetModelResult{}, nil
			},
			acquireModelLease: func(_ context.Context, request modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
				gotAcquire = true
				if request.Scope != scope || request.Name != "OMNIVOICE_Q4_K_M" || request.Holder != "you-models-cli-invoke" {
					t.Fatalf("AcquireModelLease request = %#v", request)
				}
				return modelinference.AcquireModelLeaseResult{
					Lease: modelinference.ModelLease{Lease: lease},
				}, nil
			},
			invokeModelWithLease: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				gotInvoke = true
				if request.Scope != scope || request.ModelName != "OMNIVOICE_Q4_K_M" || request.Operation != "TTS" {
					t.Fatalf("InvokeModelWithLease request = %#v", request)
				}
				return modelinference.InvokeModelResult{
					ModelName: "OMNIVOICE_Q4_K_M",
					Operation: "TTS",
					Content:   []modelinference.InferenceContent{{ContentType: "text/plain", Content: "synthesized"}},
				}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context:   context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Operation: "TTS",
		Text:      "hello world",
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !gotCatalog || !gotAcquire || !gotInvoke {
		t.Fatalf("invoke path calls: catalog=%v acquire=%v invoke=%v", gotCatalog, gotAcquire, gotInvoke)
	}
	if !strings.Contains(out.String(), "OMNIVOICE_Q4_K_M") {
		t.Fatalf("Invoke() JSON missing model name:\n%s", out.String())
	}
}
