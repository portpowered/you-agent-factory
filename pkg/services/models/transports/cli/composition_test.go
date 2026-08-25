package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
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
	listModels  func(context.Context) (modelinference.List, error)
	getModel    func(context.Context, string) (modelinference.Detail, error)
	pullModel   func(context.Context, string) (modelinference.PullResult, error)
	invokeModel func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error)
}

func (stub compositionModelsRoot) OpenRuntimeScope(context.Context, modelinference.OpenRuntimeScopeRequest) (modelinference.OpenRuntimeScopeResult, error) {
	return modelinference.OpenRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) CloseRuntimeScope(context.Context, modelinference.CloseRuntimeScopeRequest) (modelinference.CloseRuntimeScopeResult, error) {
	return modelinference.CloseRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) ListCatalog(ctx context.Context, request modelinference.ListModelsRequest) (modelinference.ListModelsResult, error) {
	if stub.listModels != nil {
		list, err := stub.listModels(ctx)
		if err != nil {
			return modelinference.ListModelsResult{}, err
		}
		return modelinference.ListModelsResult{Models: list.Results}, nil
	}
	return modelinference.ListModelsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) GetCatalogModel(ctx context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
	if stub.getModel != nil {
		detail, err := stub.getModel(ctx, request.Name)
		if err != nil {
			return modelinference.GetModelResult{}, err
		}
		return modelinference.GetModelResult{Model: detail}, nil
	}
	return modelinference.GetModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) GetModelReadiness(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
	return modelinference.GetModelReadinessResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) ResolveModelReference(context.Context, modelinference.ResolveModelReferenceRequest) (modelinference.ResolveModelReferenceResult, error) {
	return modelinference.ResolveModelReferenceResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) PullModelForScope(ctx context.Context, request modelinference.PullModelRequest) (modelinference.PullResult, error) {
	if stub.pullModel != nil {
		return stub.pullModel(ctx, request.Name)
	}
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

func (stub compositionModelsRoot) InvokeModel(ctx context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
	if stub.invokeModel != nil {
		return stub.invokeModel(ctx, request)
	}
	return modelinference.InvokeModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub compositionModelsRoot) CancelInvocation(context.Context, modelinference.CancelInvocationRequest) (modelinference.CancelInvocationResult, error) {
	return modelinference.CancelInvocationResult{}, modelinference.ErrUnsupportedOperation
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
	modelscli.InvocationTarget,
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

func compositionCatalogScope() (modelscli.InvokeRuntimeScope, error) {
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("composition-test:catalog-scope")
	if err != nil {
		return modelscli.InvokeRuntimeScope{}, err
	}
	return modelscli.InvokeRuntimeScope{Scope: scope}, nil
}

func (inv compositionInvocation) CompositionOpenCatalogScope(context.Context) (modelscli.InvokeRuntimeScope, error) {
	return compositionCatalogScope()
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
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			return compositionCatalogScope()
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
	openCatalog := func(context.Context) (modelscli.InvokeRuntimeScope, error) {
		return compositionCatalogScope()
	}
	bound := modelscli.BindService(modelscli.Config{Models: root, OpenCatalogScope: openCatalog})
	constructed := modelscli.NewService(modelscli.Config{Models: root, OpenCatalogScope: openCatalog})
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

type factorySessionPresentationInvocation struct {
	root      modelinference.Service
	openScope func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error)
}

func (inv factorySessionPresentationInvocation) InvokeModel(
	context.Context,
	modelscli.InvocationTarget,
	string,
	modelinference.Request,
) (modelinference.Result, error) {
	return modelinference.Result{}, errors.New("composition test does not invoke through bootstrap")
}

func (inv factorySessionPresentationInvocation) ResolveModelInvocationFactoryDir(string) (string, error) {
	return "/tmp/factory", nil
}

func (inv factorySessionPresentationInvocation) ExportModelInvocationArtifact(string, string) error {
	return nil
}

func (inv factorySessionPresentationInvocation) CompositionModelsRoot() modelinference.Service {
	return inv.root
}

func (inv factorySessionPresentationInvocation) CompositionOpenCatalogScope(
	ctx context.Context,
) (modelscli.InvokeRuntimeScope, error) {
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("composition-test:catalog-scope")
	if err != nil {
		return modelscli.InvokeRuntimeScope{}, err
	}
	return modelscli.InvokeRuntimeScope{Scope: scope}, nil
}

func (inv factorySessionPresentationInvocation) CompositionOpenInvokeScope(
	ctx context.Context,
	cfg modelscli.InvokeConfig,
) (modelscli.InvokeRuntimeScope, error) {
	if inv.openScope != nil {
		return inv.openScope(ctx, cfg)
	}
	return modelscli.InvokeRuntimeScope{}, nil
}

func TestNewActivatesOwnedPathThroughCompositionProvider(t *testing.T) {
	t.Parallel()

	var listed bool
	root := compositionModelsRoot{
		listModels: func(context.Context) (modelinference.List, error) {
			listed = true
			return modelinference.List{
				Results: []modelinference.Summary{{Name: "OMNIVOICE_Q4_K_M"}},
			}, nil
		},
	}
	invocation := factorySessionPresentationInvocation{root: root}
	service := modelscli.New(compositionHTTPProtocol(t), invocation)
	if service == nil {
		t.Fatal("New() = nil, want composition facade")
	}
	var out bytes.Buffer
	if err := service.List(modelscli.ListConfig{
		Context: context.Background(),
		Output:  &out,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !listed {
		t.Fatal("List() did not route through Models-owned adapter path")
	}
	if !strings.Contains(out.String(), "OMNIVOICE_Q4_K_M") {
		t.Fatalf("List() output = %q, want model name", out.String())
	}
}

func TestConfigFromCompositionBuildsOwnedServiceFromExplicitProvider(t *testing.T) {
	t.Parallel()

	invocation := factorySessionPresentationInvocation{
		root: compositionModelsRoot{
			pullModel: func(_ context.Context, name string) (modelinference.PullResult, error) {
				return modelinference.PullResult{ModelName: name}, nil
			},
		},
	}
	service := modelscli.NewService(modelscli.ConfigFromComposition(
		compositionHTTPProtocol(t),
		invocation,
		invocation,
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

func TestConfigFromCompositionWiresInvokeScopeFromExplicitProvider(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("composition:test:scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	var opened bool
	invocation := factorySessionPresentationInvocation{
		root: compositionModelsRoot{},
		openScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			opened = true
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	}
	cfg := modelscli.ConfigFromComposition(
		compositionHTTPProtocol(t),
		invocation,
	)
	if cfg.Models == nil {
		t.Fatal("ConfigFromComposition() Models = nil, want presentation root")
	}
	if cfg.OpenInvokeScope == nil {
		t.Fatal("ConfigFromComposition() OpenInvokeScope = nil, want presentation scope opener")
	}
	openedScope, err := cfg.OpenInvokeScope(context.Background(), modelscli.InvokeConfig{
		Context: context.Background(),
	})
	if err != nil {
		t.Fatalf("OpenInvokeScope() error = %v", err)
	}
	if !opened {
		t.Fatal("OpenInvokeScope() did not delegate through the composition provider")
	}
	if openedScope.Scope != scope {
		t.Fatalf("opened scope = %q, want %q", openedScope.Scope, scope)
	}
}

func TestNewInvokesThroughCompositionProviderOwnedPath(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	lease := testModelLease(t)
	var openedScope bool
	invocation := factorySessionPresentationInvocation{
		root: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				if request.Scope != scope {
					t.Fatalf("GetCatalogModel scope = %q, want %q", request.Scope, scope)
				}
				return modelinference.GetModelResult{
					Model: modelinference.Detail{
						Summary: modelinference.Summary{Name: request.Name},
						Capabilities: []modelinference.Capability{{
							Worker: "voice-local",
							Operations: []modelinference.Operation{{
								Name: request.Operation,
							}},
						}},
					},
				}, nil
			},
			acquireModelLease: func(_ context.Context, _ modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
				return modelinference.AcquireModelLeaseResult{
					Lease: modelinference.ModelLease{Lease: lease},
				}, nil
			},
			invokeModelWithLease: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				return modelinference.InvokeModelResult{
					ModelName: request.ModelName,
					Operation: request.Operation,
					Content:   []modelinference.InferenceContent{{ContentType: "text/plain", Content: "owned"}},
				}, nil
			},
		},
		openScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			openedScope = true
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	}
	service := modelscli.New(compositionHTTPProtocol(t), invocation)
	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context:   context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Operation: "TTS",
		Text:      "hello",
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !openedScope {
		t.Fatal("Invoke() did not open presentation scope through owned adapter path")
	}
	for _, want := range []string{
		`"mode":"VALIDATION_ONLY"`,
		`"validationOnly":true`,
		`"inferenceExecuted":false`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("Invoke() output = %q, want %q", out.String(), want)
		}
	}
}

func TestRootAdapter_InvokeGenericSingleOutputWritesOnlyPayload(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	var out bytes.Buffer
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModel(request.Name, "EMBED", modelinference.OperationSlot{
					Name: "embedding", Modality: modelinference.ModalityText,
				}), nil
			},
			invokeModel: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				if request.Operation != "EMBED" || len(request.Inputs) != 1 {
					return modelinference.InvokeModelResult{}, fmt.Errorf("unexpected generic request: %#v", request)
				}
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{
					Name: "embedding", Modality: modelinference.ModalityText, Content: "[1,2]",
				}}}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "embed", Operation: "EMBED", Text: "hello", Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if out.String() != "[1,2]" {
		t.Fatalf("generic stdout = %q, want canonical payload only", out.String())
	}
}

func TestRootAdapter_InvokeGenericMultipleOutputsRejectsBeforeRoot(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	called := false
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModel("omni", modelinference.OperationOMNI,
					modelinference.OperationSlot{Name: "text", Modality: modelinference.ModalityText},
					modelinference.OperationSlot{Name: "usage", Modality: modelinference.ModalityJSON}), nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				called = true
				return modelinference.InvokeModelResult{}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "omni", Operation: modelinference.OperationOMNI,
		Text: "hello", Output: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "text, usage") {
		t.Fatalf("Invoke() error = %v, want named multi-output preflight failure", err)
	}
	if called {
		t.Fatal("generic root was called after multi-output preflight rejection")
	}
}

func TestRootAdapter_InvokeGenericInputJSONPreservesAllNamedOutputs(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	artifact := testArtifactRef(t, "artifact:usage")
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIOperationModel("omni", modelinference.OperationOMNI,
					[]modelinference.OperationSlot{{Name: "prompt", Modality: modelinference.ModalityText}},
					[]modelinference.OperationSlot{
						{Name: "text", Modality: modelinference.ModalityText},
						{Name: "usage", Modality: modelinference.ModalityJSON},
					}), nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{
					{Name: "text", Modality: modelinference.ModalityText, Content: "answer"},
					{Name: "usage", Modality: modelinference.ModalityJSON, Artifact: &modelinference.InferenceArtifact{
						Artifact: artifact, MediaType: "application/json", SizeBytes: 7,
					}},
				}}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "omni", Operation: modelinference.OperationOMNI,
		InputMappings: []string{"prompt=hello"}, JSON: true, Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode generic JSON: %v\n%s", err, out.String())
	}
	if len(response.Outputs) != 2 || response.Outputs[0].Name != "text" || response.Outputs[1].Name != "usage" {
		t.Fatalf("generic outputs = %#v, want all named outputs", response.Outputs)
	}
	if response.Outputs[1].Artifact == nil || response.Outputs[1].Artifact.ArtifactRef != "artifact:usage" || response.Outputs[1].Artifact.SizeBytes == nil || *response.Outputs[1].Artifact.SizeBytes != 7 {
		t.Fatalf("generic artifact = %#v, want preserved metadata", response.Outputs[1].Artifact)
	}
}

func TestRootAdapter_InvokeGenericExplicitMappingsPublishBytesAndMetadata(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	textPath := filepath.Join(t.TempDir(), "text.out")
	usagePath := filepath.Join(t.TempDir(), "usage.out")
	artifact := testArtifactRef(t, "artifact:usage")
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModel("omni", modelinference.OperationOMNI,
					modelinference.OperationSlot{Name: "text", Modality: modelinference.ModalityText},
					modelinference.OperationSlot{Name: "usage", Modality: modelinference.ModalityJSON}), nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{
					{Name: "text", Modality: modelinference.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: "answer"},
					{Name: "usage", Modality: modelinference.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: "{\"tokens\":2}", Artifact: &modelinference.InferenceArtifact{
						Artifact: artifact, MediaType: "application/json", SizeBytes: 14,
						Properties: map[string]string{"digest": "sha256:usage"},
					}},
				}}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		OutputFileSystem: localOutputFileSystem{},
	})

	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "omni", Operation: modelinference.OperationOMNI,
		Text: "hello", OutputMappings: []string{"text=" + textPath, "usage=" + usagePath}, Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	assertMappedCLIFile(t, textPath, "answer")
	assertMappedCLIFile(t, usagePath, "{\"tokens\":2}")
	assertMappedCLIResponse(t, out.Bytes())
}

func TestRootAdapter_InvokeDirectTTSAliasOutputPathPublishesBytes(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	outputPath := filepath.Join(t.TempDir(), "answer.txt")
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModel(modelinference.BuiltInModelNameTTS, modelinference.OperationTTS,
					modelinference.OperationSlot{Name: "answer", Modality: modelinference.ModalityText}), nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				return modelinference.InvokeModelResult{
					Outputs: []modelinference.InferenceOutput{{
						Name: "answer", Modality: modelinference.ModalityText, Content: "published answer",
					}},
				}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		OutputFileSystem: localOutputFileSystem{},
	})

	var output bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: modelinference.BuiltInModelNameTTS, Operation: modelinference.OperationTTS,
		Text: "hello", OutputPath: outputPath, Output: &output,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	assertMappedCLIFile(t, outputPath, "published answer")
	if !strings.Contains(output.String(), "Wrote audio: "+outputPath) {
		t.Fatalf("Invoke() stdout = %q, want publication confirmation", output.String())
	}

	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: modelinference.BuiltInModelNameTTS, Operation: modelinference.OperationTTS,
		Text: "hello again", OutputPath: outputPath, Output: &output,
	}); err != nil {
		t.Fatalf("Invoke() replacement error = %v", err)
	}
	assertMappedCLIFile(t, outputPath, "published answer")
}

func TestRootAdapter_InvokeUsesCurrentReadinessProjection(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	var readinessRequest modelinference.GetModelReadinessRequest
	var invoked bool
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModel(request.Name, request.Operation,
					modelinference.OperationSlot{Name: "answer", Modality: modelinference.ModalityText}), nil
			},
			getReadiness: func(_ context.Context, request modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
				readinessRequest = request
				return modelinference.GetModelReadinessResult{
					ModelName: request.Name,
					Readiness: modelinference.Runtime{
						Identity: request.Name, ReadinessState: modelinference.ReadinessStateReady,
						LifecycleState: modelinference.LifecycleStateInstalled,
					},
				}, nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				invoked = true
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{
					Name: "answer", Modality: modelinference.ModalityText, Content: "ready answer",
				}}}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})
	var output bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "omni", Operation: modelinference.OperationOMNI,
		Text: "hello", Output: &output,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if readinessRequest.Scope != scope || readinessRequest.Name != "omni" || readinessRequest.Operation != modelinference.OperationOMNI {
		t.Fatalf("readiness request = %#v, want scoped omni/OMNI request", readinessRequest)
	}
	if !invoked || !strings.Contains(output.String(), "ready answer") {
		t.Fatalf("Invoke() invoked=%v output=%q, want ready answer", invoked, output.String())
	}
}

func TestRootAdapter_InvokeStopsOnReadinessFailure(t *testing.T) {
	t.Parallel()

	invoked := false
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModel(request.Name, request.Operation,
					modelinference.OperationSlot{Name: "answer", Modality: modelinference.ModalityText}), nil
			},
			getReadiness: func(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
				return modelinference.GetModelReadinessResult{}, modelinference.ErrUnavailable
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				invoked = true
				return modelinference.InvokeModelResult{}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: testRuntimeScope(t)}, nil
		},
	})
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "omni", Operation: modelinference.OperationOMNI,
		Text: "hello", JSON: true, Output: io.Discard,
	}); err == nil || !errors.Is(err, modelinference.ErrUnavailable) {
		t.Fatalf("Invoke() error = %v, want ErrUnavailable", err)
	}
	if invoked {
		t.Fatal("InvokeModel called after readiness failure")
	}
}

func TestRootAdapter_InvokeGenericFileInputsPreservesOrderBytesAndMedia(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	wantBytes := []byte{0x00, 0xff, 0x10, 0x80, 0x7f}
	var gotRequest modelinference.InvokeModelRequest
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModelWithInputs("asr", modelinference.OperationASR,
					modelinference.OperationSlot{Name: "audio", Modality: modelinference.ModalityAudio, Required: boolPointer(true), MediaTypes: []string{"audio/*"}},
					modelinference.OperationSlot{Name: "prompt", Modality: modelinference.ModalityText, MediaTypes: []string{"text/plain"}},
					modelinference.OperationSlot{Name: "transcript", Modality: modelinference.ModalityText},
					modelinference.OperationSlot{Name: "segments", Modality: modelinference.ModalityJSON},
				), nil
			},
			invokeModel: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				gotRequest = request
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{
					{Name: "transcript", Modality: modelinference.ModalityText, Content: "heard"},
					{Name: "segments", Modality: modelinference.ModalityJSON, Content: "[]"},
				}}, nil
			},
		},
		InputFileReader: func(_ context.Context, path string, _ int64) ([]byte, error) {
			if path != "meeting.wav" {
				return nil, fmt.Errorf("unexpected input path %q", path)
			}
			return wantBytes, nil
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "asr", Operation: modelinference.OperationASR,
		InputMappings: []string{"audio=@meeting.wav", "prompt=transcribe exactly"}, JSON: true, Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(gotRequest.Inputs) != 2 {
		t.Fatalf("generic inputs = %#v, want two ordered inputs", gotRequest.Inputs)
	}
	audio, prompt := gotRequest.Inputs[0], gotRequest.Inputs[1]
	if audio.Name != "audio" || audio.Modality != modelinference.ModalityAudio || audio.ContentType != "audio/wav" || audio.MediaType != "audio/wav" || audio.Content != string(wantBytes) {
		t.Fatalf("audio input = %#v, want exact bytes and audio/wav metadata", audio)
	}
	if prompt.Name != "prompt" || prompt.Modality != modelinference.ModalityText || prompt.ContentType != "text/plain" || prompt.MediaType != "text/plain" || prompt.Content != "transcribe exactly" {
		t.Fatalf("prompt input = %#v, want ordered text input", prompt)
	}
}

func TestRootAdapter_InvokeGenericInlineJSONInputPreservesContent(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	var gotInputs []modelinference.InferenceInput
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIOperationModel("asr", modelinference.OperationASR,
					[]modelinference.OperationSlot{
						{Name: "audio", Modality: modelinference.ModalityAudio, Required: boolPointer(true), MediaTypes: []string{"audio/*"}},
						{Name: "parameters", Modality: modelinference.ModalityJSON, MediaTypes: []string{"application/json"}},
					},
					[]modelinference.OperationSlot{
						{Name: "transcript", Modality: modelinference.ModalityText},
						{Name: "segments", Modality: modelinference.ModalityJSON},
					}), nil
			},
			invokeModel: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				gotInputs = append([]modelinference.InferenceInput(nil), request.Inputs...)
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{
					{Name: "transcript", Modality: modelinference.ModalityText, Content: "heard"},
					{Name: "segments", Modality: modelinference.ModalityJSON, Content: "[]"},
				}}, nil
			},
		},
		InputFileReader: func(context.Context, string, int64) ([]byte, error) { return []byte("RIFF"), nil },
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "asr", Operation: modelinference.OperationASR,
		InputMappings: []string{"audio=@meeting.wav", `parameters={"language":"en"}`}, JSON: true, Output: io.Discard,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(gotInputs) != 2 || gotInputs[1].Name != "parameters" || gotInputs[1].ContentType != "application/json" || gotInputs[1].MediaType != "application/json" || gotInputs[1].Content != `{"language":"en"}` {
		t.Fatalf("inline JSON input = %#v, want exact ordered JSON input", gotInputs)
	}
}
