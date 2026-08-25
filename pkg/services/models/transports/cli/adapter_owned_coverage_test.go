package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	removeModel          func(context.Context, modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error)
	getCatalogModel      func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error)
	getModelReadiness    func(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error)
	acquireModelLease    func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error)
	invokeModel          func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error)
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

func (stub ownedCoverageModelsRoot) GetModelReadiness(ctx context.Context, request modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
	if stub.getModelReadiness != nil {
		return stub.getModelReadiness(ctx, request)
	}
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

func (stub ownedCoverageModelsRoot) RemoveModelAssets(ctx context.Context, request modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error) {
	if stub.removeModel != nil {
		return stub.removeModel(ctx, request)
	}
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

func (stub ownedCoverageModelsRoot) InvokeModel(ctx context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
	if stub.invokeModel != nil {
		return stub.invokeModel(ctx, request)
	}
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

func TestGenericCLIInputHelpersCoverNamedTextJSONAndFileForms(t *testing.T) {
	t.Parallel()

	textSlot, jsonSlot, audioSlot := genericCLIInputCoverageSlots()
	readCalls := 0
	var readLimit int64
	service := newOwnedCoverageInputReader(&readCalls, &readLimit)
	cfg := InvokeConfig{Context: context.Background()}

	assertGenericCLIInputSuccessForms(t, service, cfg, textSlot, jsonSlot, &readCalls, &readLimit)
	assertGenericCLIInputFailureForms(t, service, cfg, textSlot, audioSlot)
	assertGenericCLIInputMediaTypeForms(t)
}

func genericCLIInputCoverageSlots() (modelinference.OperationSlot, modelinference.OperationSlot, modelinference.OperationSlot) {
	textRequired := true
	textSlot := modelinference.OperationSlot{
		Name: "text", Modality: modelinference.ModalityText, Required: &textRequired,
		MediaTypes: []string{"text/plain"},
	}
	jsonSlot := modelinference.OperationSlot{
		Name: "parameters", Modality: modelinference.ModalityJSON,
		MediaTypes: []string{"application/json"},
	}
	audioSlot := modelinference.OperationSlot{
		Name: "audio", Modality: modelinference.ModalityAudio,
		MediaTypes: []string{"audio/*"},
	}
	return textSlot, jsonSlot, audioSlot
}

func newOwnedCoverageInputReader(readCalls *int, readLimit *int64) *rootService {
	return &rootService{
		inputFileReader: func(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
			*readCalls = *readCalls + 1
			*readLimit = maxBytes
			if path != "note.txt" {
				return nil, errors.New("unexpected input path")
			}
			return []byte("file text"), ctx.Err()
		},
	}
}

func assertGenericCLIInputSuccessForms(
	t *testing.T,
	service *rootService,
	cfg InvokeConfig,
	textSlot, jsonSlot modelinference.OperationSlot,
	readCalls *int,
	readLimit *int64,
) {
	t.Helper()
	text, err := service.genericCLIInput(cfg, genericCLIInputMapping{slot: "text", value: "inline text"}, textSlot)
	if err != nil || text.Content != "inline text" || text.MediaType != "text/plain" {
		t.Fatalf("inline text input = %#v, error = %v", text, err)
	}
	parameters, err := service.genericCLIInput(cfg, genericCLIInputMapping{
		slot: "parameters", value: `json:{"normalize":true}`,
	}, jsonSlot)
	if err != nil || parameters.Content != `{"normalize":true}` || parameters.MediaType != "application/json" {
		t.Fatalf("inline JSON input = %#v, error = %v", parameters, err)
	}
	if _, err := service.genericCLIInput(cfg, genericCLIInputMapping{slot: "parameters", value: "json:not-json"}, jsonSlot); err == nil {
		t.Fatal("invalid JSON input error = nil")
	}

	fileInput, err := service.genericCLIInput(cfg, genericCLIInputMapping{slot: "text", value: "@note.txt"}, textSlot)
	if err != nil || fileInput.Content != "file text" || fileInput.MediaType != "text/plain" {
		t.Fatalf("file input = %#v, error = %v", fileInput, err)
	}
	if *readCalls != 1 || *readLimit != genericCLIInputMaxFileBytes {
		t.Fatalf("file reader calls/limit = %d/%d, want 1/%d", *readCalls, *readLimit, genericCLIInputMaxFileBytes)
	}
}

func assertGenericCLIInputFailureForms(
	t *testing.T,
	service *rootService,
	cfg InvokeConfig,
	textSlot, audioSlot modelinference.OperationSlot,
) {
	t.Helper()
	if _, err := service.genericCLIInput(cfg, genericCLIInputMapping{slot: "audio", value: "inline audio"}, audioSlot); err == nil {
		t.Fatal("inline binary-like input error = nil")
	}
	if _, err := service.genericCLIInput(cfg, genericCLIInputMapping{slot: "text", value: "@"}, textSlot); err == nil {
		t.Fatal("empty file path error = nil")
	}

	tooLarge := &rootService{inputFileReader: func(context.Context, string, int64) ([]byte, error) {
		return []byte(strings.Repeat("x", int(genericCLIInputMaxFileBytes+1))), nil
	}}
	if _, err := tooLarge.genericCLIInput(cfg, genericCLIInputMapping{slot: "text", value: "@note.txt"}, textSlot); err == nil {
		t.Fatal("oversized file input error = nil")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancelReader := &rootService{inputFileReader: func(context.Context, string, int64) ([]byte, error) {
		cancel()
		return []byte("late"), nil
	}}
	if _, err := cancelReader.genericCLIInput(InvokeConfig{Context: cancelled}, genericCLIInputMapping{slot: "text", value: "@note.txt"}, textSlot); err == nil {
		t.Fatal("canceled file input error = nil")
	}

	if _, err := (&rootService{}).genericCLIInput(cfg, genericCLIInputMapping{slot: "text", value: "@note.txt"}, textSlot); err == nil {
		t.Fatal("unconfigured file reader error = nil")
	}
	if _, err := service.genericCLIInput(cfg, genericCLIInputMapping{slot: "text", value: "@note.txt"}, modelinference.OperationSlot{
		Name: "text", Modality: modelinference.ModalityText, MediaTypes: []string{"application/json"},
	}); err == nil {
		t.Fatal("file media capability error = nil")
	}
}

func assertGenericCLIInputMediaTypeForms(t *testing.T) {
	t.Helper()
	for _, test := range []struct {
		name, input, want string
	}{
		{name: "wav alias", input: " audio/x-wav; charset=binary ", want: "audio/wav"},
		{name: "ogg alias", input: "application/ogg; codecs=opus", want: "audio/ogg"},
		{name: "lowercase", input: "Application/JSON; charset=utf-8", want: "application/json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := genericCLIInputNormalizeMediaType(test.input); got != test.want {
				t.Fatalf("genericCLIInputNormalizeMediaType(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
	if got := genericCLIJSONInputValue("  {\"normalize\":true} "); got != "  {\"normalize\":true} " {
		t.Fatalf("unprefixed JSON value = %q, want original value", got)
	}
}

func TestGenericCLIModelOperationInferenceCoversBuiltInAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, model, want string
	}{
		{name: "llm", model: " LLM ", want: modelinference.OperationOMNI},
		{name: "asr", model: "AsR", want: modelinference.OperationASR},
		{name: "tts", model: "tts", want: modelinference.OperationTTS},
		{name: "embed", model: " EMBED ", want: modelinference.OperationEMBED},
		{name: "unknown", model: "custom", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := inferGenericCLIModelOperation(test.model); got != test.want {
				t.Fatalf("inferGenericCLIModelOperation(%q) = %q, want %q", test.model, got, test.want)
			}
		})
	}
}

func TestGenericCLIEmbedInvocationBindsNamedInputsAndPublishesOutputPath(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("owned-coverage:embed-scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	textRequired := true
	parametersRequired := false
	var gotRequest modelinference.InvokeModelRequest
	service := NewService(Config{
		Models: ownedCoverageModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				if request.Name != modelinference.BuiltInModelNameEmbed || request.Operation != modelinference.OperationEMBED {
					t.Fatalf("catalog request = %#v, want EMBED alias", request)
				}
				return modelinference.GetModelResult{Model: modelinference.Detail{
					Summary: modelinference.Summary{
						Name: modelinference.BuiltInModelNameEmbed,
						Operations: []modelinference.Operation{{
							Name: modelinference.OperationEMBED,
							Inputs: []modelinference.OperationSlot{
								{Name: "text", Modality: modelinference.ModalityText, Required: &textRequired, MediaTypes: []string{"text/plain"}},
								{Name: "parameters", Modality: modelinference.ModalityJSON, Required: &parametersRequired, MediaTypes: []string{"application/json"}},
							},
							Outputs: []modelinference.OperationSlot{{Name: "embedding", Modality: modelinference.ModalityJSON, MediaTypes: []string{"application/json"}}},
						}},
					},
				},
				}, nil
			},
			getModelReadiness: func(_ context.Context, request modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
				if request.Name != modelinference.BuiltInModelNameEmbed || request.Operation != modelinference.OperationEMBED {
					t.Fatalf("readiness request = %#v, want EMBED alias", request)
				}
				return modelinference.GetModelReadinessResult{ModelName: modelinference.BuiltInModelNameEmbed, Readiness: modelinference.Runtime{
					Identity: modelinference.BuiltInModelNameEmbed, ReadinessState: modelinference.ReadinessStateReady,
				}}, nil
			},
			invokeModel: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				gotRequest = request
				return modelinference.InvokeModelResult{
					ModelName: modelinference.BuiltInModelNameEmbed,
					Operation: modelinference.OperationEMBED,
					Outputs:   []modelinference.InferenceOutput{{Name: "embedding", Modality: modelinference.ModalityJSON, ContentType: "application/json", Content: `[0.1,0.2]`}},
				}, nil
			},
		},
		OpenInvokeScope: func(context.Context, InvokeConfig) (InvokeRuntimeScope, error) {
			return InvokeRuntimeScope{Scope: scope}, nil
		},
		OutputFileSystem: ownedCoverageOutputFileSystem{},
	})

	outputPath := filepath.Join(t.TempDir(), "embedding.json")
	var out bytes.Buffer
	if err := service.Invoke(InvokeConfig{
		Context: context.Background(), ModelName: " embed ", InputMappings: []string{
			"text=hello", `parameters=json:{"normalize":true}`,
		}, OutputPath: outputPath, Output: &out,
	}); err != nil {
		t.Fatalf("EMBED Invoke() error = %v", err)
	}
	assertGenericCLIEmbedRequest(t, gotRequest)
	assertGenericCLIEmbedOutput(t, outputPath, out.String())
}

func assertGenericCLIEmbedRequest(t *testing.T, request modelinference.InvokeModelRequest) {
	t.Helper()
	if request.Operation != modelinference.OperationEMBED || request.Model.NameOrURI != modelinference.BuiltInModelNameEmbed {
		t.Fatalf("invoke request = %#v, want inferred EMBED operation", request)
	}
	if len(request.Inputs) != 2 || request.Inputs[0].Name != "text" || request.Inputs[0].Content != "hello" || request.Inputs[1].Content != `{"normalize":true}` {
		t.Fatalf("invoke inputs = %#v, want ordered text and JSON inputs", request.Inputs)
	}
}

func assertGenericCLIEmbedOutput(t *testing.T, outputPath, report string) {
	t.Helper()
	data, err := os.ReadFile(outputPath)
	if err != nil || string(data) != `[0.1,0.2]` {
		t.Fatalf("EMBED output = %q, error = %v, want embedding JSON", data, err)
	}
	if !strings.Contains(report, "Wrote audio:") {
		t.Fatalf("output report = %q, want publication report", report)
	}
}

type ownedCoverageOutputFileSystem struct{}

func (ownedCoverageOutputFileSystem) CreateTemp(dir, pattern string) (OutputTemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

func (ownedCoverageOutputFileSystem) Inspect(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (ownedCoverageOutputFileSystem) Remove(path string) error {
	return os.Remove(path)
}

func (ownedCoverageOutputFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func TestGenericCLIOutputPublicationRestoresAnExistingTargetAfterRollback(t *testing.T) {
	t.Parallel()

	fileSystem := ownedCoverageOutputFileSystem{}
	targetPath := filepath.Join(t.TempDir(), "embedding.json")
	if err := os.WriteFile(targetPath, []byte("old-vector"), 0o600); err != nil {
		t.Fatalf("write existing target: %v", err)
	}
	backups, err := backupGenericCLIOutputTargets(context.Background(), fileSystem, []genericCLIOutputMapping{{slot: "embedding", path: targetPath}})
	if err != nil {
		t.Fatalf("backupGenericCLIOutputTargets() error = %v", err)
	}
	if len(backups) != 1 || backups[0].targetPath != targetPath {
		t.Fatalf("backups = %#v, want one backup for target", backups)
	}
	if err := os.WriteFile(targetPath, []byte("new-vector"), 0o600); err != nil {
		t.Fatalf("write replacement target: %v", err)
	}
	rollbackGenericCLIOutputPublication(fileSystem, []genericCLIOutputStage{{targetPath: targetPath}}, backups, 1)
	data, err := os.ReadFile(targetPath)
	if err != nil || string(data) != "old-vector" {
		t.Fatalf("restored target = %q, error = %v, want old-vector", data, err)
	}
}

func TestGenericCLIInputMediaAndContentTypeFallbacks(t *testing.T) {
	t.Parallel()

	if got := genericCLIInputContentType(modelinference.OperationSlot{
		Modality: modelinference.ModalityText, MediaTypes: []string{" text/* ", "application/custom"},
	}); got != "application/custom" {
		t.Fatalf("content type = %q, want first concrete declaration", got)
	}
	if got := genericCLIInputContentType(modelinference.OperationSlot{Modality: modelinference.ModalityJSON}); got != "application/json" {
		t.Fatalf("JSON fallback content type = %q, want application/json", got)
	}
	if got := genericCLIInputContentType(modelinference.OperationSlot{}); got != "text/plain" {
		t.Fatalf("text fallback content type = %q, want text/plain", got)
	}
	for _, test := range []struct {
		path, want string
		data       []byte
	}{
		{path: "note.txt", want: "text/plain"},
		{path: "note.xml", want: "text/xml"},
		{path: "note.unknown", want: "application/octet-stream", data: []byte{0x00, 0x01}},
	} {
		if got := genericCLIInputMediaType(test.path, test.data); got != test.want {
			t.Fatalf("genericCLIInputMediaType(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

func TestCompositionServiceRecognizesDirectTTSAliasForOwnedOutputPath(t *testing.T) {
	t.Parallel()

	if !isDirectTTSAlias(InvokeConfig{ModelName: " tTs ", Operation: " tTs "}) {
		t.Fatal("isDirectTTSAlias() = false, want true for the built-in alias")
	}
	if isDirectTTSAlias(InvokeConfig{ModelName: "embed", Operation: modelinference.OperationEMBED}) {
		t.Fatal("isDirectTTSAlias() = true, want false for EMBED")
	}
}

func TestModelsCLIRemoteRemovePublishesTheDeleteResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/models/embed" {
			t.Fatalf("remove request = %s %s, want DELETE /models/embed", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"modelName":"embed","outcome":"REMOVED","revision":"rev-1","cachePath":"/cache/embed","bytesRemoved":3}`))
	}))
	defer server.Close()

	service := NewService(Config{Models: ownedCoverageModelsRoot{}, HTTP: ownedCoverageHTTPProtocol(t)})
	var output bytes.Buffer
	if err := service.Remove(RemoveConfig{Context: context.Background(), Server: server.URL, ModelName: "embed", JSON: true, Output: &output}); err != nil {
		t.Fatalf("remote Remove() error = %v", err)
	}
	var response factoryapi.ModelRemoveResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode remote remove response: %v\n%s", err, output.String())
	}
	if response.ModelName != "embed" || response.Revision != "rev-1" || response.BytesRemoved != 3 {
		t.Fatalf("remote remove response = %#v, want detached delete response", response)
	}
}

func TestModelsCLIInvocationDiagnosticsCoverEveryPublicFailureFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		class modelinference.InvocationFailureClass
		code  string
	}{
		{class: modelinference.InvocationFailureClassInvalidModelReference, code: modelsRootModelUnavailableCode},
		{class: modelinference.InvocationFailureClassRevisionResolution, code: modelsRootModelUnavailableCode},
		{class: modelinference.InvocationFailureClassInvalidOperation, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassInvalidSlot, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassSlotArity, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassInvalidParameter, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassMediaCapability, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassArtifact, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassOfflineCache, code: "MODEL_OFFLINE_CACHE_UNAVAILABLE"},
		{class: modelinference.InvocationFailureClassBackendReadiness, code: "MODEL_BACKEND_NOT_READY"},
		{class: modelinference.InvocationFailureClassBackendProtocol, code: "MODEL_BACKEND_FAILURE"},
		{class: modelinference.InvocationFailureClassMalformedResponse, code: "MODEL_BACKEND_FAILURE"},
		{class: modelinference.InvocationFailureClassCancellation, code: "MODEL_INFERENCE_TIMEOUT"},
		{class: modelinference.InvocationFailureClassTimeout, code: "MODEL_INFERENCE_TIMEOUT"},
		{class: modelinference.InvocationFailureClassConfiguration, code: "MODEL_CONFIGURATION_FAILURE"},
	}
	for _, test := range tests {
		t.Run(string(test.class), func(t *testing.T) {
			mapped, ok := mapModelsInvocationError(&modelinference.InvocationFailure{Class: test.class, Message: "failure"})
			if !ok || mapped == nil {
				t.Fatalf("mapModelsInvocationError(%q) = (%v, %v), want mapped error", test.class, mapped, ok)
			}
			rootError, ok := mapped.(*modelsRootError)
			if !ok || rootError.CLIErrorCode() != test.code {
				t.Fatalf("mapped %q = %#v, want code %q", test.class, mapped, test.code)
			}
		})
	}
}
func TestRootInvokeValidatesGenericCLIInputForms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  InvokeConfig
		want string
	}{
		{name: "context", cfg: InvokeConfig{Output: io.Discard}, want: "context is required"},
		{name: "output", cfg: InvokeConfig{Context: context.Background()}, want: "output writer is required"},
		{name: "model", cfg: InvokeConfig{Context: context.Background(), Output: io.Discard}, want: "model name is required"},
		{name: "custom named input needs operation", cfg: InvokeConfig{Context: context.Background(), ModelName: "custom", InputMappings: []string{"prompt=hello"}, Output: io.Discard}, want: "--operation is required"},
		{name: "text needs operation", cfg: InvokeConfig{Context: context.Background(), ModelName: "custom", Text: "hello", Output: io.Discard}, want: "--operation is required"},
		{name: "text is required", cfg: InvokeConfig{Context: context.Background(), ModelName: "custom", Operation: modelinference.OperationOMNI, Output: io.Discard}, want: "--text is required"},
		{name: "text and named input conflict", cfg: InvokeConfig{Context: context.Background(), ModelName: "llm", InputMappings: []string{"prompt=hello"}, Text: "hello", Output: io.Discard}, want: "--text cannot be used with --input"},
		{name: "remote server", cfg: InvokeConfig{Context: context.Background(), ModelName: "custom", Operation: modelinference.OperationOMNI, Text: "hello", Server: "http://remote", Output: io.Discard}, want: "composition-stable HTTP service"},
		{name: "scope opener", cfg: InvokeConfig{Context: context.Background(), ModelName: "custom", Operation: modelinference.OperationOMNI, Text: "hello", Output: io.Discard}, want: "runtime scope opener is required"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			service := &rootService{}
			err := service.Invoke(test.cfg)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Invoke() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestWriteGenericCLIOutputSelectsDeclaredInlineOutput(t *testing.T) {
	t.Parallel()

	required := true
	optional := false
	catalog := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: modelinference.OperationOMNI,
		Outputs: []modelinference.OperationSlot{
			{Name: "usage", Modality: modelinference.ModalityJSON, Required: &optional},
			{Name: "text", Modality: modelinference.ModalityText, Required: &required},
		},
	}}}}
	var out bytes.Buffer
	err := writeGenericCLIOutput(&out, modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{
		{Name: "usage", Modality: modelinference.ModalityJSON, Content: `{"tokens":3}`},
		{Name: "text", Modality: modelinference.ModalityText, Content: "answer"},
	}}, catalog, modelinference.OperationOMNI)
	if err != nil {
		t.Fatalf("writeGenericCLIOutput: %v", err)
	}
	if out.String() != "answer" {
		t.Fatalf("inline output = %q, want required text", out.String())
	}
}

func TestWriteGenericCLIOutputRejectsAmbiguousAndInvalidResults(t *testing.T) {
	t.Parallel()

	required := true
	catalog := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: modelinference.OperationOMNI,
		Outputs: []modelinference.OperationSlot{
			{Name: "text", Modality: modelinference.ModalityText, Required: &required},
			{Name: "summary", Modality: modelinference.ModalityText, Required: &required},
		},
	}}}}
	cases := []struct {
		name   string
		result modelinference.InvokeModelResult
		cat    modelinference.Detail
		want   string
	}{
		{name: "ambiguous", cat: catalog, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "one"}, {Name: "summary", Content: "two"}}}, want: "multiple model outputs"},
		{name: "unknown operation", result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "one"}, {Name: "summary", Content: "two"}}}, want: "multiple model outputs"},
		{name: "empty inline value", result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, want: "no inline output"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			err := writeGenericCLIOutput(&out, test.result, test.cat, modelinference.OperationOMNI)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("writeGenericCLIOutput error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestWriteGenericCLIOutputPathRejectsInvalidResults(t *testing.T) {
	t.Parallel()

	validConfig := InvokeConfig{Context: context.Background(), OutputPath: "answer.txt", Output: io.Discard}
	validOutputSystem := &outputPathTestFileSystem{}
	cases := []struct {
		name    string
		service *rootService
		result  modelinference.InvokeModelResult
		want    string
	}{
		{name: "missing filesystem", service: &rootService{}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "answer"}}}, want: "filesystem is required"},
		{name: "multiple outputs", service: &rootService{outputFileSystem: validOutputSystem}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "one"}, {Name: "usage", Content: "two"}}}, want: "multiple model outputs"},
		{name: "unnamed output", service: &rootService{outputFileSystem: validOutputSystem}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Content: "answer"}}}, want: "unnamed output"},
		{name: "empty output", service: &rootService{outputFileSystem: validOutputSystem}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, want: "has no inline bytes"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := test.service.writeGenericCLIOutputPath(validConfig, test.result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("writeGenericCLIOutputPath error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestWriteGenericCLIOutputMappingsRejectsInvalidResults(t *testing.T) {
	t.Parallel()

	service := &rootService{outputFileSystem: &outputPathTestFileSystem{}}
	cfg := InvokeConfig{Context: context.Background(), OutputMappings: []string{"text=answer.txt"}, Output: io.Discard}
	cases := []struct {
		name    string
		service *rootService
		cfg     InvokeConfig
		result  modelinference.InvokeModelResult
		want    string
	}{
		{name: "missing filesystem", service: &rootService{}, cfg: cfg, result: modelinference.InvokeModelResult{}, want: "filesystem is required"},
		{name: "invalid mapping", service: service, cfg: InvokeConfig{Context: context.Background(), OutputMappings: []string{"text"}, Output: io.Discard}, result: modelinference.InvokeModelResult{}, want: "expected slot=path"},
		{name: "output count", service: service, cfg: cfg, result: modelinference.InvokeModelResult{}, want: "returned 0 outputs"},
		{name: "unmapped slot", service: service, cfg: cfg, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "usage", Content: "3"}}}, want: "unmapped output slot"},
		{name: "empty content", service: service, cfg: cfg, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, want: "has no inline bytes"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := test.service.writeGenericCLIOutputMappings(test.cfg, test.result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("writeGenericCLIOutputMappings error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestGenericCLIJSONResultHonorsJSONAndDeclaredShape(t *testing.T) {
	t.Parallel()

	catalog := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name:    modelinference.OperationOMNI,
		Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
	}}}}
	result := modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "answer"}}}
	cases := []struct {
		name string
		cfg  InvokeConfig
		res  modelinference.InvokeModelResult
		cat  modelinference.Detail
		want bool
	}{
		{name: "not JSON", cfg: InvokeConfig{}, res: result, cat: catalog},
		{name: "empty outputs", cfg: InvokeConfig{JSON: true}, res: modelinference.InvokeModelResult{}, cat: catalog},
		{name: "multiple outputs", cfg: InvokeConfig{JSON: true}, res: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}, {Name: "usage"}}}, cat: catalog, want: true},
		{name: "declared inline output", cfg: InvokeConfig{JSON: true}, res: result, cat: catalog, want: true},
		{name: "unknown operation", cfg: InvokeConfig{JSON: true}, res: result, cat: catalog, want: false},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			operation := modelinference.OperationOMNI
			if test.name == "unknown operation" {
				operation = "missing"
			}
			if got := genericCLIJSONResult(test.cfg, test.cat, operation, test.res); got != test.want {
				t.Fatalf("genericCLIJSONResult() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseGenericCLIOutputMappingsRejectsAmbiguousMappings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "missing equals", values: []string{"text"}, want: "expected slot=path"},
		{name: "empty slot", values: []string{"=answer.txt"}, want: "slot and path are required"},
		{name: "empty path", values: []string{"text="}, want: "slot and path are required"},
		{name: "stdout path", values: []string{"text=-"}, want: "path '-' is not supported"},
		{name: "duplicate slot", values: []string{"text=one.txt", "text=two.txt"}, want: "duplicate output mapping"},
		{name: "duplicate path", values: []string{"text=answer.txt", "usage=answer.txt"}, want: "same path"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseGenericCLIOutputMappings(test.values); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseGenericCLIOutputMappings error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateGenericCLIOutputMappingsRequiresExactDeclaredSlots(t *testing.T) {
	t.Parallel()

	operation := modelinference.Operation{
		Name:    modelinference.OperationOMNI,
		Outputs: []modelinference.OperationSlot{{Name: "text"}, {Name: "usage"}},
	}
	cases := []struct {
		name   string
		values []string
		found  bool
		want   string
	}{
		{name: "unknown operation", values: []string{"text=text.txt", "usage=usage.json"}, want: "unknown operation"},
		{name: "missing slot", values: []string{"text=text.txt"}, found: true, want: "cover every output slot"},
		{name: "unknown slot", values: []string{"text=text.txt", "other=other.txt"}, found: true, want: "unknown slot"},
		{name: "valid", values: []string{"text=text.txt", "usage=usage.json"}, found: true},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := validateGenericCLIOutputMappings(test.values, operation, test.found)
			if test.want == "" {
				if err != nil {
					t.Fatalf("validateGenericCLIOutputMappings() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateGenericCLIOutputMappings() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRefreshInvokeReadinessUsesObservedRuntimeAndMapsFailures(t *testing.T) {
	t.Parallel()

	catalog := modelinference.Detail{Summary: modelinference.Summary{ManagedRuntime: modelinference.Runtime{Identity: "catalog"}}}
	readiness := modelinference.Runtime{Identity: "observed", ReadinessState: modelinference.ReadinessStateReady}
	cases := []struct {
		name      string
		result    modelinference.GetModelReadinessResult
		err       error
		wantID    string
		wantError string
	}{
		{name: "observed readiness", result: modelinference.GetModelReadinessResult{Readiness: readiness}, wantID: "observed"},
		{name: "unsupported fallback", err: modelinference.ErrUnsupportedOperation, wantID: "catalog"},
		{name: "mapped failure", err: errors.New("readiness fixture failed"), wantError: "readiness fixture failed"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			root := &rootService{models: readinessModelRoot{result: test.result, err: test.err}}
			got, err := root.refreshInvokeReadiness(InvokeConfig{Context: context.Background()}, modelinference.RuntimeScopeRef{}, "model", modelinference.OperationOMNI, catalog)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("refreshInvokeReadiness error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil || got.ManagedRuntime.Identity != test.wantID {
				t.Fatalf("refreshInvokeReadiness = %#v, error %v, want identity %q", got.ManagedRuntime, err, test.wantID)
			}
		})
	}
}

type readinessModelRoot struct {
	modelinference.Service
	result modelinference.GetModelReadinessResult
	err    error
}

func (root readinessModelRoot) GetModelReadiness(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
	return root.result, root.err
}

func TestStageGenericCLIOutputFileHandlesFilesystemFailures(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	closeCancelled, cancelAfterClose := context.WithCancel(context.Background())
	cases := []struct {
		name string
		ctx  context.Context
		fs   *scriptedOutputFileSystem
		want string
	}{
		{name: "cancelled before create", ctx: cancelled, fs: &scriptedOutputFileSystem{}, want: context.Canceled.Error()},
		{name: "create failure", ctx: context.Background(), fs: &scriptedOutputFileSystem{createErr: errors.New("create failed")}, want: "create failed"},
		{name: "nil temporary", ctx: context.Background(), fs: &scriptedOutputFileSystem{createNil: true}, want: "no handle"},
		{name: "write failure", ctx: context.Background(), fs: &scriptedOutputFileSystem{temp: &scriptedOutputTemporaryFile{name: "tmp", writeErr: errors.New("write failed")}}, want: "write failed"},
		{name: "short write", ctx: context.Background(), fs: &scriptedOutputFileSystem{temp: &scriptedOutputTemporaryFile{name: "tmp", shortWrite: true}}, want: io.ErrShortWrite.Error()},
		{name: "close failure", ctx: context.Background(), fs: &scriptedOutputFileSystem{temp: &scriptedOutputTemporaryFile{name: "tmp", closeErr: errors.New("close failed")}}, want: "close failed"},
		{name: "cancelled after close", ctx: closeCancelled, fs: &scriptedOutputFileSystem{temp: &scriptedOutputTemporaryFile{name: "tmp", onClose: cancelAfterClose}}, want: context.Canceled.Error()},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			_, err := stageGenericCLIOutputFile(test.ctx, test.fs, "answer.txt", []byte("answer"))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("stageGenericCLIOutputFile error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPublishAndBackupGenericCLIOutputsHandleHostFailures(t *testing.T) {
	t.Parallel()

	t.Run("publish cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		published, err := publishGenericCLIOutputs(ctx, &scriptedOutputFileSystem{}, modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, []genericCLIOutputStage{{temporary: "tmp", targetPath: "answer.txt"}})
		if published != 0 || !errors.Is(err, context.Canceled) {
			t.Fatalf("publish cancellation = count:%d error:%v", published, err)
		}
	})
	t.Run("publish rename failure", func(t *testing.T) {
		published, err := publishGenericCLIOutputs(context.Background(), &scriptedOutputFileSystem{renameErr: errors.New("rename failed")}, modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, []genericCLIOutputStage{{temporary: "tmp", targetPath: "answer.txt"}})
		if published != 0 || err == nil || !strings.Contains(err.Error(), "rename failed") {
			t.Fatalf("publish rename = count:%d error:%v", published, err)
		}
	})
	t.Run("backup inspection failure", func(t *testing.T) {
		_, err := backupGenericCLIOutputTargets(context.Background(), &scriptedOutputFileSystem{inspectErr: errors.New("inspect failed")}, []genericCLIOutputMapping{{slot: "text", path: "answer.txt"}})
		if err == nil || !strings.Contains(err.Error(), "inspect failed") {
			t.Fatalf("backup inspect = %v, want inspect failure", err)
		}
	})
	t.Run("backup reservation failure", func(t *testing.T) {
		_, err := backupGenericCLIOutputTargets(context.Background(), &scriptedOutputFileSystem{inspectExists: true, createErr: errors.New("reserve failed")}, []genericCLIOutputMapping{{slot: "text", path: "answer.txt"}})
		if err == nil || !strings.Contains(err.Error(), "reserve failed") {
			t.Fatalf("backup reserve = %v, want reserve failure", err)
		}
	})
	t.Run("backup rename failure", func(t *testing.T) {
		_, err := backupGenericCLIOutputTargets(context.Background(), &scriptedOutputFileSystem{
			inspectExists: true,
			temp:          &scriptedOutputTemporaryFile{name: "backup.tmp"},
			renameErr:     errors.New("backup rename failed"),
		}, []genericCLIOutputMapping{{slot: "text", path: "answer.txt"}})
		if err == nil || !strings.Contains(err.Error(), "backup rename failed") {
			t.Fatalf("backup rename = %v, want rename failure", err)
		}
	})
}

type scriptedOutputFileSystem struct {
	temp          *scriptedOutputTemporaryFile
	createErr     error
	createNil     bool
	inspectErr    error
	inspectExists bool
	removeErr     error
	renameErr     error
}

func (fs *scriptedOutputFileSystem) CreateTemp(string, string) (OutputTemporaryFile, error) {
	if fs.createErr != nil {
		return nil, fs.createErr
	}
	if fs.createNil {
		return nil, nil
	}
	if fs.temp == nil {
		fs.temp = &scriptedOutputTemporaryFile{name: "temporary.tmp"}
	}
	return fs.temp, nil
}

func (fs *scriptedOutputFileSystem) Inspect(string) (os.FileInfo, error) {
	if fs.inspectErr != nil {
		return nil, fs.inspectErr
	}
	if fs.inspectExists {
		return scriptedFileInfo{}, nil
	}
	return nil, os.ErrNotExist
}

func (fs *scriptedOutputFileSystem) Remove(string) error { return fs.removeErr }

func (fs *scriptedOutputFileSystem) Rename(string, string) error { return fs.renameErr }

type scriptedOutputTemporaryFile struct {
	name       string
	writeErr   error
	shortWrite bool
	closeErr   error
	onClose    func()
}

func (file *scriptedOutputTemporaryFile) Write(data []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	if file.shortWrite {
		return len(data) - 1, nil
	}
	return len(data), nil
}

func (file *scriptedOutputTemporaryFile) Close() error {
	if file.onClose != nil {
		file.onClose()
	}
	return file.closeErr
}

func (file *scriptedOutputTemporaryFile) Name() string { return file.name }

type scriptedFileInfo struct{}

func (scriptedFileInfo) Name() string       { return "answer.txt" }
func (scriptedFileInfo) Size() int64        { return 1 }
func (scriptedFileInfo) Mode() os.FileMode  { return 0o600 }
func (scriptedFileInfo) ModTime() time.Time { return time.Time{} }
func (scriptedFileInfo) IsDir() bool        { return false }
func (scriptedFileInfo) Sys() any           { return nil }

type outputPathTestFileSystem struct{}

func (*outputPathTestFileSystem) CreateTemp(string, string) (OutputTemporaryFile, error) {
	return nil, errors.New("unexpected CreateTemp")
}

func (*outputPathTestFileSystem) Inspect(string) (os.FileInfo, error) {
	return nil, errors.New("unexpected Inspect")
}

func (*outputPathTestFileSystem) Remove(string) error { return errors.New("unexpected Remove") }

func (*outputPathTestFileSystem) Rename(string, string) error { return errors.New("unexpected Rename") }
