package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type ownedCoverageModelsRoot struct {
	listModels           func(context.Context) (modelinference.List, error)
	getModel             func(context.Context, string) (modelinference.Detail, error)
	pullModel            func(context.Context, string) (modelinference.PullResult, error)
	getCatalogModel      func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error)
	acquireModelLease    func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error)
	invokeModelWithLease func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error)
}

func (stub ownedCoverageModelsRoot) OpenRuntimeScope(context.Context, modelinference.OpenRuntimeScopeRequest) (modelinference.OpenRuntimeScopeResult, error) {
	return modelinference.OpenRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) CloseRuntimeScope(context.Context, modelinference.CloseRuntimeScopeRequest) (modelinference.CloseRuntimeScopeResult, error) {
	return modelinference.CloseRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) ListCatalog(ctx context.Context, request modelinference.ListModelsRequest) (modelinference.ListModelsResult, error) {
	if stub.listModels != nil {
		list, err := stub.listModels(ctx)
		if err != nil {
			return modelinference.ListModelsResult{}, err
		}
		return modelinference.ListModelsResult{Models: list.Results}, nil
	}
	return modelinference.ListModelsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) GetCatalogModel(ctx context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
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

func (stub ownedCoverageModelsRoot) GetModelReadiness(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
	return modelinference.GetModelReadinessResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) ResolveModelReference(context.Context, modelinference.ResolveModelReferenceRequest) (modelinference.ResolveModelReferenceResult, error) {
	return modelinference.ResolveModelReferenceResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) PullModelForScope(ctx context.Context, request modelinference.PullModelRequest) (modelinference.PullResult, error) {
	if stub.pullModel != nil {
		return stub.pullModel(ctx, request.Name)
	}
	return modelinference.PullResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) PrepareModelAssets(context.Context, modelinference.PrepareModelAssetsRequest) (modelinference.PrepareModelAssetsResult, error) {
	return modelinference.PrepareModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) InspectModelAssets(context.Context, modelinference.InspectModelAssetsRequest) (modelinference.InspectModelAssetsResult, error) {
	return modelinference.InspectModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) RemoveModelAssets(context.Context, modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error) {
	return modelinference.RemoveModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) EnsureModelHost(context.Context, modelinference.EnsureModelHostRequest) (modelinference.EnsureModelHostResult, error) {
	return modelinference.EnsureModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) InspectModelHost(context.Context, modelinference.InspectModelHostRequest) (modelinference.InspectModelHostResult, error) {
	return modelinference.InspectModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) StopModelHost(context.Context, modelinference.StopModelHostRequest) (modelinference.StopModelHostResult, error) {
	return modelinference.StopModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) AcquireModelLease(ctx context.Context, request modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
	if stub.acquireModelLease != nil {
		return stub.acquireModelLease(ctx, request)
	}
	return modelinference.AcquireModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) GetModelLease(context.Context, modelinference.GetModelLeaseRequest) (modelinference.GetModelLeaseResult, error) {
	return modelinference.GetModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) ReleaseModelLease(context.Context, modelinference.ReleaseModelLeaseRequest) (modelinference.ReleaseModelLeaseResult, error) {
	return modelinference.ReleaseModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) InvokeModelWithLease(ctx context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
	if stub.invokeModelWithLease != nil {
		return stub.invokeModelWithLease(ctx, request)
	}
	return modelinference.InvokeModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) InvokeModel(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
	return modelinference.InvokeModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) CancelInvocation(context.Context, modelinference.CancelInvocationRequest) (modelinference.CancelInvocationResult, error) {
	return modelinference.CancelInvocationResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) ListModels(ctx context.Context) (modelinference.List, error) {
	if stub.listModels != nil {
		return stub.listModels(ctx)
	}
	return modelinference.List{}, nil
}

func (stub ownedCoverageModelsRoot) GetModel(ctx context.Context, name string) (modelinference.Detail, error) {
	if stub.getModel != nil {
		return stub.getModel(ctx, name)
	}
	return modelinference.Detail{}, modelinference.ErrNotFound
}

func (stub ownedCoverageModelsRoot) PullModel(ctx context.Context, name string) (modelinference.PullResult, error) {
	if stub.pullModel != nil {
		return stub.pullModel(ctx, name)
	}
	return modelinference.PullResult{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) InspectRuntime(context.Context, string) (modelinference.Runtime, error) {
	return modelinference.Runtime{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) AcquireLease(context.Context, modelinference.AcquireLeaseRequest) (modelinference.HostLease, error) {
	return modelinference.HostLease{}, modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) ReleaseLease(context.Context, modelinference.ReleaseLeaseRequest) error {
	return modelinference.ErrUnsupportedOperation
}

func (stub ownedCoverageModelsRoot) InvokeLocal(context.Context, modelinference.LocalInvocationRequest) (modelinference.LocalInvocationResult, error) {
	return modelinference.LocalInvocationResult{}, modelinference.ErrUnsupportedOperation
}

type ownedCoverageHTTPClock struct{}

func (ownedCoverageHTTPClock) Now() time.Time { return time.Unix(1, 0) }

func ownedCoverageHTTPProtocol(t *testing.T) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(&http.Client{}, ownedCoverageHTTPClock{})
	if err != nil {
		t.Fatalf("build owned-coverage HTTP protocol: %v", err)
	}
	return protocol
}

type ownedCoverageCompositionInvocation struct {
	root ownedCoverageModelsRoot
}

func (inv ownedCoverageCompositionInvocation) InvokeModel(
	context.Context,
	InvocationTarget,
	string,
	modelinference.Request,
) (modelinference.Result, error) {
	return modelinference.Result{}, errors.New("owned coverage does not invoke through bootstrap")
}

func (inv ownedCoverageCompositionInvocation) ResolveModelInvocationFactoryDir(string) (string, error) {
	return "/tmp/factory", nil
}

func (inv ownedCoverageCompositionInvocation) ExportModelInvocationArtifact(string, string) error {
	return nil
}

func (inv ownedCoverageCompositionInvocation) CompositionModelsRoot() modelinference.Service {
	return inv.root
}

func (inv ownedCoverageCompositionInvocation) CompositionOpenCatalogScope(
	context.Context,
) (InvokeRuntimeScope, error) {
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("owned-coverage:test-scope")
	if err != nil {
		return InvokeRuntimeScope{}, err
	}
	return InvokeRuntimeScope{Scope: scope}, nil
}

func (inv ownedCoverageCompositionInvocation) CompositionOpenInvokeScope(
	ctx context.Context,
	_ InvokeConfig,
) (InvokeRuntimeScope, error) {
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("owned-coverage:test-scope")
	if err != nil {
		return InvokeRuntimeScope{}, err
	}
	return InvokeRuntimeScope{Scope: scope}, nil
}

func ownedCoverageService(t *testing.T, root ownedCoverageModelsRoot) Service {
	t.Helper()
	openScope := func(context.Context) (InvokeRuntimeScope, error) {
		scope, err := (modelinference.RuntimeScopeRef{}).Parse("owned-coverage:test-scope")
		if err != nil {
			return InvokeRuntimeScope{}, err
		}
		return InvokeRuntimeScope{Scope: scope}, nil
	}
	service := BindService(Config{
		Models:           root,
		OpenCatalogScope: openScope,
		OpenInvokeScope: func(context.Context, InvokeConfig) (InvokeRuntimeScope, error) {
			return openScope(context.Background())
		},
	})
	if service == nil {
		t.Fatal("BindService() = nil, want owned Models CLI service")
	}
	return service
}

// TestOwnedAdapter_ListInspectPullPreserveAcceptedOutput proves owned adapter
// list/inspect/pull behavior through the Models root.
func TestOwnedAdapter_ListInspectPullPreserveAcceptedOutput(t *testing.T) {
	t.Parallel()

	root := ownedCoverageModelsRoot{
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
	service := ownedCoverageService(t, root)

	var listHuman bytes.Buffer
	if err := service.List(ListConfig{
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
	if err := service.Inspect(InspectConfig{
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
	if err := service.Pull(PullConfig{
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

// TestOwnedAdapter_CompositionFacadeRoutesOwnedListThroughModelsRoot proves
// composition-stable New() delegates durable behavior through the owned adapter.
func TestOwnedAdapter_CompositionFacadeRoutesOwnedListThroughModelsRoot(t *testing.T) {
	t.Parallel()

	root := ownedCoverageModelsRoot{
		listModels: func(context.Context) (modelinference.List, error) {
			return modelinference.List{
				Results: []modelinference.Summary{{Name: "OMNIVOICE_Q4_K_M"}},
			}, nil
		},
	}
	service := New(
		ownedCoverageHTTPProtocol(t),
		ownedCoverageCompositionInvocation{root: root},
	)
	if service == nil {
		t.Fatal("New() = nil, want composition facade")
	}

	var out bytes.Buffer
	if err := service.List(ListConfig{
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

// TestOwnedAdapter_InvokeValidationResolvesThroughModelsRoot proves validation
// mode reaches the owned adapter's catalog boundary without acquiring a lease.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestOwnedAdapter_InvokeResolvesThroughModelsRoot(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("owned-coverage:test-scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	lease, err := (modelinference.ModelLeaseRef{}).Parse("owned-coverage:test-lease")
	if err != nil {
		t.Fatalf("parse model lease: %v", err)
	}
	var gotCatalog, gotAcquire, gotInvoke bool
	service := NewService(Config{
		Models: ownedCoverageModelsRoot{
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
		OpenInvokeScope: func(context.Context, InvokeConfig) (InvokeRuntimeScope, error) {
			return InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	var out bytes.Buffer
	if err := service.Invoke(InvokeConfig{
		Context:   context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Operation: "TTS",
		Text:      "hello world",
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !gotCatalog || gotAcquire || gotInvoke {
		t.Fatalf("validation path calls: catalog=%v acquire=%v invoke=%v", gotCatalog, gotAcquire, gotInvoke)
	}
	for _, want := range []string{
		"OMNIVOICE_Q4_K_M",
		`"mode":"VALIDATION_ONLY"`,
		`"validationOnly":true`,
		`"inferenceExecuted":false`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("Invoke() JSON missing %q:\n%s", want, out.String())
		}
	}
}

func TestHTTPServiceValidationUsesSelectedCatalogTarget(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("validation:test-scope")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	var gotRequest modelinference.GetModelRequest
	closed := false
	root := ownedCoverageModelsRoot{
		getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
			gotRequest = request
			return modelinference.GetModelResult{}, nil
		},
	}
	service := &httpService{
		models: root,
		openInvokeScope: func(context.Context, InvokeConfig) (InvokeRuntimeScope, error) {
			return InvokeRuntimeScope{
				Scope: scope,
				Close: func(context.Context) error {
					closed = true
					return nil
				},
			}, nil
		},
	}
	if err := service.validateModelInvoke(InvokeConfig{Context: context.Background()}, "voice", "TTS"); err != nil {
		t.Fatalf("local validation error = %v", err)
	}
	if gotRequest.Scope != scope || gotRequest.Name != "voice" || gotRequest.Operation != "TTS" {
		t.Fatalf("local validation request = %#v", gotRequest)
	}
	if !closed {
		t.Fatal("local validation did not close its runtime scope")
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/models/voice" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"name":"voice","operations":[{"name":"TTS"}]}`))
	}))
	t.Cleanup(server.Close)
	remote := &httpService{http: testHTTPProtocol(t), models: root, openInvokeScope: func(context.Context, InvokeConfig) (InvokeRuntimeScope, error) {
		t.Fatal("remote validation opened the local scope")
		return InvokeRuntimeScope{}, nil
	}}
	if err := remote.validateModelInvoke(InvokeConfig{
		Context: context.Background(), Server: server.URL,
	}, "voice", "TTS"); err != nil {
		t.Fatalf("remote validation error = %v", err)
	}
}

func TestHTTPServiceValidationReportsTargetFailures(t *testing.T) {
	t.Parallel()

	if err := (&httpService{}).validateModelInvoke(InvokeConfig{
		Context: context.Background(), Server: "http://factory.test",
	}, "voice", "TTS"); err == nil || !strings.Contains(err.Error(), "HTTP protocol") {
		t.Fatalf("nil HTTP validation error = %v, want protocol failure", err)
	}

	openErr := errors.New("factory layout unavailable")
	service := &httpService{
		models: ownedCoverageModelsRoot{},
		openInvokeScope: func(context.Context, InvokeConfig) (InvokeRuntimeScope, error) {
			return InvokeRuntimeScope{}, openErr
		},
	}
	if err := service.validateModelInvoke(InvokeConfig{Context: context.Background()}, "voice", "TTS"); !errors.Is(err, openErr) {
		t.Fatalf("scope opener error = %v, want %v", err, openErr)
	}

	service = &httpService{
		models: ownedCoverageModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{}, modelinference.ErrNotFound
			},
		},
		openCatalogScope: func(context.Context) (InvokeRuntimeScope, error) {
			return InvokeRuntimeScope{}, nil
		},
	}
	if err := service.validateModelInvoke(InvokeConfig{Context: context.Background()}, "voice", "TTS"); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("catalog validation error = %v, want ErrModelNotFound", err)
	}
	if err := (&httpService{}).validateModelInvoke(InvokeConfig{Context: context.Background()}, "voice", "TTS"); err != nil {
		t.Fatalf("unscoped compatibility validation error = %v, want nil", err)
	}
}

func TestGeneratedModelSupportsOperationChecksAllPublicProjections(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		model factoryapi.ModelDetail
		want  bool
	}{
		{
			name:  "detail operations",
			model: factoryapi.ModelDetail{Operations: []factoryapi.ModelInvocationOperation{{Name: "TTS"}}},
			want:  true,
		},
		{
			name: "capability operations",
			model: factoryapi.ModelDetail{Capabilities: []factoryapi.ModelCapability{{
				Operations: []factoryapi.ModelInvocationOperation{{Name: "TTS"}},
			}}},
			want: true,
		},
		{
			name: "managed runtime operations",
			model: factoryapi.ModelDetail{ManagedRuntime: factoryapi.ManagedRuntime{
				SupportedOperations: []factoryapi.ModelInvocationOperation{{Name: "TTS"}},
			}},
			want: true,
		},
		{name: "missing operation", model: factoryapi.ModelDetail{}, want: false},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if got := generatedModelSupportsOperation(test.model, "TTS"); got != test.want {
				t.Fatalf("generatedModelSupportsOperation() = %v, want %v", got, test.want)
			}
		})
	}
}
