package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type stubModelsRoot struct {
	listModels           func(context.Context) (modelinference.List, error)
	getModel             func(context.Context, string) (modelinference.Detail, error)
	pullModel            func(context.Context, string) (modelinference.PullResult, error)
	removeModel          func(context.Context, modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error)
	getCatalogModel      func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error)
	getReadiness         func(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error)
	acquireModelLease    func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error)
	invokeModelWithLease func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error)
	invokeModel          func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error)
}

func (stub stubModelsRoot) PreflightModelAssets(context.Context, modelinference.PrepareModelAssetsRequest) (modelinference.PreflightModelAssetsResult, error) {
	return modelinference.PreflightModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) OpenRuntimeScope(context.Context, modelinference.OpenRuntimeScopeRequest) (modelinference.OpenRuntimeScopeResult, error) {
	return modelinference.OpenRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) CloseRuntimeScope(context.Context, modelinference.CloseRuntimeScopeRequest) (modelinference.CloseRuntimeScopeResult, error) {
	return modelinference.CloseRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) ListCatalog(ctx context.Context, request modelinference.ListModelsRequest) (modelinference.ListModelsResult, error) {
	if stub.listModels != nil {
		list, err := stub.listModels(ctx)
		if err != nil {
			return modelinference.ListModelsResult{}, err
		}
		return modelinference.ListModelsResult{Models: list.Results}, nil
	}
	return modelinference.ListModelsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) GetCatalogModel(ctx context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
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

func (stub stubModelsRoot) GetModelReadiness(ctx context.Context, request modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
	if stub.getReadiness != nil {
		return stub.getReadiness(ctx, request)
	}
	return modelinference.GetModelReadinessResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) ResolveModelReference(context.Context, modelinference.ResolveModelReferenceRequest) (modelinference.ResolveModelReferenceResult, error) {
	return modelinference.ResolveModelReferenceResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) PullModelForScope(ctx context.Context, request modelinference.PullModelRequest) (modelinference.PullResult, error) {
	if stub.pullModel != nil {
		return stub.pullModel(ctx, request.Name)
	}
	return modelinference.PullResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) PrepareModelAssets(context.Context, modelinference.PrepareModelAssetsRequest) (modelinference.PrepareModelAssetsResult, error) {
	return modelinference.PrepareModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) InspectModelAssets(context.Context, modelinference.InspectModelAssetsRequest) (modelinference.InspectModelAssetsResult, error) {
	return modelinference.InspectModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) RemoveModelAssets(ctx context.Context, request modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error) {
	if stub.removeModel != nil {
		return stub.removeModel(ctx, request)
	}
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

func (stub stubModelsRoot) InvokeModel(ctx context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
	if stub.invokeModel != nil {
		return stub.invokeModel(ctx, request)
	}
	return modelinference.InvokeModelResult{}, modelinference.ErrUnsupportedOperation
}

func (stub stubModelsRoot) CancelInvocation(context.Context, modelinference.CancelInvocationRequest) (modelinference.CancelInvocationResult, error) {
	return modelinference.CancelInvocationResult{}, modelinference.ErrUnsupportedOperation
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

func TestRootAdapter_InvokeGenericUsesRequiredTextInputWhenOptionalSlotsSortFirst(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	optional := false
	required := true
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{Model: modelinference.Detail{
					Summary: modelinference.Summary{
						Name: request.Name,
						Operations: []modelinference.Operation{{
							Name: modelinference.OperationTTS,
							Inputs: []modelinference.OperationSlot{
								{Name: "parameters", Modality: modelinference.ModalityJSON, Required: &optional, MediaTypes: []string{"application/json"}},
								{Name: "text", Modality: modelinference.ModalityText, Required: &required, MediaTypes: []string{"text/plain"}},
							},
							Outputs: []modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio}},
						}},
					},
				},
				}, nil
			},
			invokeModel: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				if len(request.Inputs) != 1 || request.Inputs[0].Name != "text" || request.Inputs[0].Modality != modelinference.ModalityText {
					t.Fatalf("joined generic request inputs = %#v, want required text input", request.Inputs)
				}
				return modelinference.InvokeModelResult{
					ModelName: request.Model.NameOrURI,
					Operation: request.Operation,
					Content:   []modelinference.InferenceContent{{ContentType: "audio/wav", Content: "synthesized"}},
				}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	var output bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: modelinference.BuiltInModelNameTTS,
		Operation: modelinference.OperationTTS, Text: "hello", OutputPath: "speech.wav", JSON: true, Output: &output,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !strings.Contains(output.String(), "synthesized") {
		t.Fatalf("Invoke() output = %q, want synthesized content", output.String())
	}
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
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("service-test:scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
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
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
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
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			scope, parseErr := (modelinference.RuntimeScopeRef{}).Parse("service-test:scope")
			if parseErr != nil {
				return modelscli.InvokeRuntimeScope{}, parseErr
			}
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
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
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			scope, parseErr := (modelinference.RuntimeScopeRef{}).Parse("service-test:scope")
			if parseErr != nil {
				return modelscli.InvokeRuntimeScope{}, parseErr
			}
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
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

// TestModelsCLICharacterizationSuccessProjections pins the current human and
// JSON projections at the public Models CLI service boundary. The Models root
// is a deterministic collaborator because the real local runtime shells out
// to an executable that is intentionally absent from this repository.
func TestModelsCLICharacterizationSuccessProjections(t *testing.T) {
	t.Parallel()
	t.Run("list", testModelsCLICharacterizationList)
	t.Run("inspect", testModelsCLICharacterizationInspect)
	t.Run("pull", testModelsCLICharacterizationPull)
	t.Run("remove", testModelsCLICharacterizationRemove)
}

func testModelsCLICharacterizationList(t *testing.T) {
	service := characterizationModelsCLIService(t, stubModelsRoot{
		listModels: func(context.Context) (modelinference.List, error) {
			return modelinference.List{Results: []modelinference.Summary{
				characterizationModelSummary(),
			}}, nil
		},
	})

	var human bytes.Buffer
	if err := service.List(modelscli.ListConfig{Context: context.Background(), Output: &human}); err != nil {
		t.Fatalf("List() human error = %v", err)
	}
	if got, want := human.String(), "NAME\tREADINESS\tLIFECYCLE\tLOCALITY\tOPERATIONS\tMODALITIES\tRESOURCES\tCACHE SIZE\nOMNIVOICE_Q4_K_M\tREADY\tINSTALLED\tLOCAL\tTTS\tAUDIO,TEXT\t1\tNOT_INSTALLED\n"; got != want {
		t.Fatalf("List() human = %q, want %q", got, want)
	}

	var structured bytes.Buffer
	if err := service.List(modelscli.ListConfig{
		Context: context.Background(), JSON: true, Output: &structured,
	}); err != nil {
		t.Fatalf("List() JSON error = %v", err)
	}
	assertCharacterizationJSON(t, structured.String(), `{"results":[{"loadState":"UNLOADED","managedRuntime":{"diagnostics":{"cache":"omnivoice-cache"},"identity":"OMNIVOICE_Q4_K_M","lifecycleState":"INSTALLED","locality":"LOCAL","readinessState":"READY","supportedOperations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}]},"modalities":["TEXT","AUDIO"],"name":"OMNIVOICE_Q4_K_M","operations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}],"providerLocality":"LOCAL","resources":[{"backend":"LLAMACPP","capacity":1,"loadPolicy":"ON_DEMAND","model":"OMNIVOICE_Q4_K_M","name":"omnivoice-cache","type":"MODEL"}],"status":"READY"}]}`)
}

func testModelsCLICharacterizationInspect(t *testing.T) {
	service := characterizationModelsCLIService(t, stubModelsRoot{
		getModel: func(context.Context, string) (modelinference.Detail, error) {
			return characterizationModelDetail(), nil
		},
	})

	var human bytes.Buffer
	if err := service.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &human,
	}); err != nil {
		t.Fatalf("Inspect() human error = %v", err)
	}
	if got, want := human.String(), "Name:\tOMNIVOICE_Q4_K_M\nReadiness:\tREADY\nLifecycle:\tINSTALLED\nLocality:\tLOCAL\nRevision:\tNOT_INSTALLED\nCache Size:\tNOT_INSTALLED\nCache Path:\tNOT_INSTALLED\nOperations:\tTTS\nModalities:\tAUDIO,TEXT\nResources:\t1\nCapabilities:\n- tts-executor\tLOCAL\tTTS\nDiagnostics:\n- cache=omnivoice-cache\n"; got != want {
		t.Fatalf("Inspect() human = %q, want %q", got, want)
	}

	var structured bytes.Buffer
	if err := service.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", JSON: true, Output: &structured,
	}); err != nil {
		t.Fatalf("Inspect() JSON error = %v", err)
	}
	assertCharacterizationJSON(t, structured.String(), `{"capabilities":[{"modelProvider":"CODEX","operations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}],"providerLocality":"LOCAL","resourceNames":["omnivoice-cache"],"worker":"tts-executor"}],"diagnostics":{"statusReason":"managed runtime is discoverable"},"loadState":"UNLOADED","managedRuntime":{"diagnostics":{"cache":"omnivoice-cache"},"identity":"OMNIVOICE_Q4_K_M","lifecycleState":"INSTALLED","locality":"LOCAL","readinessState":"READY","supportedOperations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}]},"modalities":["TEXT","AUDIO"],"name":"OMNIVOICE_Q4_K_M","operations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}],"providerLocality":"LOCAL","resources":[{"backend":"LLAMACPP","capacity":1,"loadPolicy":"ON_DEMAND","model":"OMNIVOICE_Q4_K_M","name":"omnivoice-cache","type":"MODEL"}],"status":"READY"}`)
}

func testModelsCLICharacterizationPull(t *testing.T) {
	service := characterizationModelsCLIService(t, stubModelsRoot{
		pullModel: func(_ context.Context, name string) (modelinference.PullResult, error) {
			return modelinference.PullResult{
				ModelName: name, ProviderLocality: "LOCAL", Outcome: "PULLED",
				CachePath: "/models/OMNIVOICE_Q4_K_M/rev-2026", Revision: "rev-2026",
				ManagedPullOutcome: "INSTALLED_SUCCESSFULLY", ReadinessState: "READY",
				DownloadedFiles: []modelinference.DownloadedFile{
					{Path: "weights.gguf", Bytes: 42, SHA256: "abc123"},
					{Path: "config.json", Bytes: 7},
				},
			}, nil
		},
	})

	var human bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &human,
	}); err != nil {
		t.Fatalf("Pull() human error = %v", err)
	}
	if got, want := human.String(), "MODEL\tPULL OUTCOME\tREADINESS\tLIFECYCLE\tREVISION\tCACHE PATH\nOMNIVOICE_Q4_K_M\tINSTALLED_SUCCESSFULLY\tREADY\tINSTALLED\trev-2026\t/models/OMNIVOICE_Q4_K_M/rev-2026\nFILES\nconfig.json\t7\nweights.gguf\t42\n"; got != want {
		t.Fatalf("Pull() human = %q, want %q", got, want)
	}

	var structured bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", JSON: true, Output: &structured,
	}); err != nil {
		t.Fatalf("Pull() JSON error = %v", err)
	}
	assertCharacterizationJSON(t, structured.String(), `{"cachePath":"/models/OMNIVOICE_Q4_K_M/rev-2026","downloadedFiles":[{"bytes":42,"path":"weights.gguf","sha256":"abc123"},{"bytes":7,"path":"config.json"}],"managedRuntimePull":{"cachePath":"/models/OMNIVOICE_Q4_K_M/rev-2026","downloadedFiles":[{"bytes":42,"path":"weights.gguf","sha256":"abc123"},{"bytes":7,"path":"config.json"}],"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY","revision":"rev-2026"},"modelName":"OMNIVOICE_Q4_K_M","outcome":"PULLED","providerLocality":"LOCAL","revision":"rev-2026"}`)
}

func testModelsCLICharacterizationRemove(t *testing.T) {
	service := characterizationModelsCLIService(t, stubModelsRoot{
		removeModel: func(_ context.Context, request modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error) {
			return modelinference.RemoveModelAssetsResult{
				ModelName: request.Name, Revision: "rev-2026",
				CachePath: "/models/OMNIVOICE_Q4_K_M/rev-2026", BytesRemoved: 42,
				Readiness: modelinference.AssetReadinessMissing,
				Outcome:   modelinference.AssetRemovalRemoved,
			}, nil
		},
	})

	var human bytes.Buffer
	if err := service.Remove(modelscli.RemoveConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &human,
	}); err != nil {
		t.Fatalf("Remove() human error = %v", err)
	}
	if got, want := human.String(), "MODEL\tREMOVE OUTCOME\tREVISION\tCACHE PATH\tBYTES REMOVED\nOMNIVOICE_Q4_K_M\tREMOVED\trev-2026\t/models/OMNIVOICE_Q4_K_M/rev-2026\t42 B (42 bytes)\n"; got != want {
		t.Fatalf("Remove() human = %q, want %q", got, want)
	}

	var structured bytes.Buffer
	if err := service.Remove(modelscli.RemoveConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", JSON: true, Output: &structured,
	}); err != nil {
		t.Fatalf("Remove() JSON error = %v", err)
	}
	var response factoryapi.ModelRemoveResponse
	if err := json.Unmarshal(structured.Bytes(), &response); err != nil {
		t.Fatalf("Remove() JSON decode error = %v", err)
	}
	if response.ModelName != "OMNIVOICE_Q4_K_M" || response.Revision != "rev-2026" || response.BytesRemoved != 42 {
		t.Fatalf("Remove() response = %#v", response)
	}
}

// TestModelsCLICharacterizationInvokeAudioProjectionAndLeaseRelease pins the
// non-JSON audio export and the current release behavior. The release is
// characterized, not endorsed: a stateful Models boundary reports the lease
// as RELEASED after the invocation completes.
func TestModelsCLICharacterizationInvokeAudioProjectionAndLeaseRelease(t *testing.T) {
	t.Parallel()

	root := newCharacterizationLeaseModelsRoot(t, modelinference.InvokeModelResult{
		ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS",
		Artifacts: []modelinference.InferenceArtifact{{Artifact: testArtifactRef(t, "characterized-source.wav")}},
	})
	var scopeClosed bool
	var exportedSource, exportedDestination string
	service := characterizationModelsCLIServiceWithLifecycle(t, root, &scopeClosed, func(sourcePath, destinationPath string) error {
		exportedSource, exportedDestination = sourcePath, destinationPath
		return nil
	})

	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS",
		Text: "hello world", OutputPath: "speech.wav", Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() audio error = %v", err)
	}
	if got, want := out.String(), "Wrote audio: speech.wav\n"; got != want {
		t.Fatalf("Invoke() audio stdout = %q, want %q", got, want)
	}
	if strings.Contains(out.String(), "{") {
		t.Fatalf("Invoke() audio stdout unexpectedly contains JSON: %q", out.String())
	}
	if exportedSource != "characterized-source.wav" || exportedDestination != "speech.wav" {
		t.Fatalf("exported artifact = (%q, %q), want characterized-source.wav/speech.wav", exportedSource, exportedDestination)
	}
	if root.responseMode != modelinference.ResponseModeAudioStream {
		t.Fatalf("Invoke() response mode = %q, want AUDIO_STREAM", root.responseMode)
	}
	if !scopeClosed {
		t.Fatal("Invoke() did not close its presentation scope")
	}
	assertCharacterizationLeaseReleased(t, root)
}

func TestModelsCLICharacterizationInvokeJSONIsValidationOnly(t *testing.T) {
	t.Parallel()

	root := newCharacterizationLeaseModelsRoot(t, modelinference.InvokeModelResult{
		ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS",
		Content: []modelinference.InferenceContent{{ContentType: "audio/wav", Content: "characterized-speech.wav"}},
	})
	var out bytes.Buffer
	service := characterizationModelsCLIService(t, root)

	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS",
		Text: "hello world", JSON: true, Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() JSON error = %v", err)
	}
	assertCharacterizationJSON(t, out.String(), `{"modelName":"OMNIVOICE_Q4_K_M","operation":"TTS","mode":"VALIDATION_ONLY","validationOnly":true,"inferenceExecuted":false}`)
	if root.releaseCalls != 0 {
		t.Fatalf("validation-only release calls = %d, want 0", root.releaseCalls)
	}
}

// TestModelsCLICharacterizationInvokeFailureReleasesLease pins the current
// failure cleanup. This release outcome is characterized, not endorsed: the
// Models inference boundary releases capacity before returning its failure.
func TestModelsCLICharacterizationInvokeFailureReleasesLease(t *testing.T) {
	t.Parallel()

	root := newCharacterizationLeaseModelsRoot(t, modelinference.InvokeModelResult{})
	root.invokeErr = errors.New("characterized inference failure")
	var scopeClosed bool
	service := characterizationModelsCLIServiceWithLifecycle(t, root, &scopeClosed, nil)

	var out bytes.Buffer
	err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS",
		Text: "hello world", OutputPath: "speech.wav", Output: &out,
	})
	if err == nil || err.Error() != "characterized inference failure" {
		t.Fatalf("Invoke() failure = %v, want exact characterized inference failure", err)
	}
	if out.Len() != 0 {
		t.Fatalf("Invoke() failure stdout = %q, want empty", out.String())
	}
	if !scopeClosed {
		t.Fatal("Invoke() did not close its presentation scope after failure")
	}
	assertCharacterizationLeaseReleased(t, root)
}

type characterizationLeaseModelsRoot struct {
	stubModelsRoot
	lease        modelinference.ModelLease
	invokeResult modelinference.InvokeModelResult
	invokeErr    error
	responseMode modelinference.ResponseMode
	releaseCalls int
}

func newCharacterizationLeaseModelsRoot(t *testing.T, result modelinference.InvokeModelResult) *characterizationLeaseModelsRoot {
	t.Helper()
	return &characterizationLeaseModelsRoot{
		stubModelsRoot: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{Model: characterizationModelDetail()}, nil
			},
		},
		lease: modelinference.ModelLease{Lease: testModelLease(t)}, invokeResult: result,
	}
}

func (root *characterizationLeaseModelsRoot) AcquireModelLease(
	_ context.Context, request modelinference.AcquireModelLeaseRequest,
) (modelinference.AcquireModelLeaseResult, error) {
	root.lease.Scope = request.Scope
	root.lease.ModelName = request.Name
	root.lease.Holder = request.Holder
	root.lease.Status = modelinference.ModelLeaseStatusActive
	root.lease.HostReadiness = modelinference.ReadinessStateReady
	return modelinference.AcquireModelLeaseResult{Lease: root.lease}, nil
}

func (root *characterizationLeaseModelsRoot) GetModelLease(
	_ context.Context, request modelinference.GetModelLeaseRequest,
) (modelinference.GetModelLeaseResult, error) {
	return modelinference.GetModelLeaseResult{Lease: root.lease}, nil
}

func (root *characterizationLeaseModelsRoot) ReleaseModelLease(
	_ context.Context, request modelinference.ReleaseModelLeaseRequest,
) (modelinference.ReleaseModelLeaseResult, error) {
	if request.Lease.String() != root.lease.Lease.String() {
		return modelinference.ReleaseModelLeaseResult{}, modelinference.ErrHostLeaseNotFound
	}
	root.lease.Status = modelinference.ModelLeaseStatusReleased
	root.releaseCalls++
	return modelinference.ReleaseModelLeaseResult{
		Lease: root.lease, Outcome: modelinference.ModelLeaseReleased,
	}, nil
}

func (root *characterizationLeaseModelsRoot) InvokeModelWithLease(
	ctx context.Context, request modelinference.InvokeModelRequest,
) (modelinference.InvokeModelResult, error) {
	root.responseMode = request.ResponseMode
	if _, err := root.ReleaseModelLease(ctx, modelinference.ReleaseModelLeaseRequest{
		Scope: request.Scope, Lease: request.Lease,
	}); err != nil {
		return modelinference.InvokeModelResult{}, err
	}
	result := root.invokeResult
	result.Scope = request.Scope
	result.Lease = request.Lease
	result.ModelName = request.ModelName
	result.Operation = request.Operation
	result.LeaseDisposition = modelinference.InvocationLeaseReleased
	return result, root.invokeErr
}

func assertCharacterizationLeaseReleased(t *testing.T, root *characterizationLeaseModelsRoot) {
	t.Helper()
	current, err := root.GetModelLease(context.Background(), modelinference.GetModelLeaseRequest{
		Scope: root.lease.Scope, Lease: root.lease.Lease,
	})
	if err != nil {
		t.Fatalf("GetModelLease() after invocation = %v", err)
	}
	if current.Lease.Status != modelinference.ModelLeaseStatusReleased || root.releaseCalls != 1 {
		t.Fatalf("lease after invocation = status:%q releaseCalls:%d, want RELEASED/1", current.Lease.Status, root.releaseCalls)
	}
}

func characterizationModelsCLIService(t *testing.T, root modelinference.Service) modelscli.Service {
	t.Helper()
	return characterizationModelsCLIServiceWithLifecycle(t, root, nil, nil)
}

func characterizationModelsCLIServiceWithLifecycle(
	t *testing.T,
	root modelinference.Service,
	scopeClosed *bool,
	export func(string, string) error,
) modelscli.Service {
	t.Helper()
	scope := testRuntimeScope(t)
	closeScope := func(context.Context) error {
		if scopeClosed != nil {
			*scopeClosed = true
		}
		return nil
	}
	if export == nil {
		export = func(string, string) error { return nil }
	}
	service := modelscli.NewService(modelscli.Config{
		Models:    root,
		Artifacts: characterizationArtifactExporter{export: export},
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope, Close: closeScope}, nil
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope, Close: closeScope}, nil
		},
	})
	if service == nil {
		t.Fatal("NewService() = nil, want Models CLI service")
	}
	return service
}

type characterizationArtifactExporter struct {
	export func(string, string) error
}

func (exporter characterizationArtifactExporter) ExportInvocationArtifact(sourcePath, destinationPath string) error {
	return exporter.export(sourcePath, destinationPath)
}

func characterizationModelSummary() modelinference.Summary {
	return modelinference.Summary{
		Name: "OMNIVOICE_Q4_K_M", ProviderLocality: modelinference.LocalityLocal,
		Status: modelinference.StatusReady, LoadState: modelinference.LoadStateUnloaded,
		Operations: []modelinference.Operation{characterizationTTSOperation()},
		Modalities: []string{"TEXT", "AUDIO"},
		Resources: []modelinference.ResourceSummary{{
			Name: "omnivoice-cache", Type: "MODEL", Capacity: 1,
			Model: characterizationString("OMNIVOICE_Q4_K_M"), Backend: characterizationString("LLAMACPP"),
			LoadPolicy: characterizationString("ON_DEMAND"),
		}},
		ManagedRuntime: modelinference.Runtime{
			Identity: "OMNIVOICE_Q4_K_M", ReadinessState: modelinference.ReadinessStateReady,
			LifecycleState: modelinference.LifecycleStateInstalled, Locality: modelinference.LocalityLocal,
			SupportedOperations: []modelinference.Operation{characterizationTTSOperation()},
			Diagnostics:         map[string]string{"cache": "omnivoice-cache"},
		},
	}
}

func characterizationModelDetail() modelinference.Detail {
	summary := characterizationModelSummary()
	return modelinference.Detail{
		Summary: summary,
		Capabilities: []modelinference.Capability{{
			Worker: "tts-executor", ProviderLocality: modelinference.LocalityLocal,
			ModelProvider: characterizationString("CODEX"), Operations: []modelinference.Operation{characterizationTTSOperation()},
			ResourceNames: []string{"omnivoice-cache"},
		}},
		Diagnostics: map[string]string{"statusReason": "managed runtime is discoverable"},
	}
}

func characterizationTTSOperation() modelinference.Operation {
	required := true
	return modelinference.Operation{
		Name: "TTS",
		Inputs: []modelinference.OperationSlot{{
			Name: "text", ContentTypes: []string{"TEXT"}, Required: &required,
		}},
		Outputs: []modelinference.OperationSlot{{
			Name: "audio", ContentTypes: []string{"AUDIO"},
		}},
	}
}

func characterizationString(value string) *string {
	return &value
}

func assertCharacterizationJSON(t *testing.T, got, want string) {
	t.Helper()
	if strings.TrimSpace(got) != want {
		t.Fatalf("JSON = %q, want exact %q", strings.TrimSpace(got), want)
	}
}

func TestRootAdapter_InspectMapsTypedInvocationValidation(t *testing.T) {
	t.Parallel()

	cause := errors.New("validation cause")
	failure := &modelinference.InvocationFailure{
		Class:   modelinference.InvocationFailureClassInvalidSlot,
		Message: "required input slot is missing: audio",
		Cause:   cause,
	}
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{}, failure
			},
		},
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			return parityInvokeScope(t), nil
		},
	})

	err := service.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: "asr", JSON: true, Output: io.Discard,
	})
	if err == nil {
		t.Fatal("Inspect() error = nil, want typed validation failure")
	}
	var classified *modelinference.InvocationFailure
	if !errors.As(err, &classified) || classified != failure || !errors.Is(err, cause) {
		t.Fatalf("Inspect() error = %v, failure = %#v, want original typed failure and cause", err, classified)
	}
	var coded interface {
		CLIErrorCode() string
		CLIErrorFamily() factoryapi.ErrorFamily
		CLIErrorMessage() string
	}
	if !errors.As(err, &coded) {
		t.Fatalf("Inspect() error = %T, want CLI diagnostic contract", err)
	}
	if coded.CLIErrorCode() != "BAD_REQUEST" || coded.CLIErrorFamily() != factoryapi.ErrorFamilyBadRequest || coded.CLIErrorMessage() != failure.Message {
		t.Fatalf("Inspect() CLI fields = (%q, %q, %q), want BAD_REQUEST/BAD_REQUEST/%q", coded.CLIErrorCode(), coded.CLIErrorFamily(), coded.CLIErrorMessage(), failure.Message)
	}
}

func TestRootAdapter_InvokeGenericFileInputRejectsOversizedBeforeInvocation(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	invokes := 0
	var receivedLimit int64
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModelWithInputs("asr", modelinference.OperationASR,
					modelinference.OperationSlot{
						Name: "audio", Modality: modelinference.ModalityAudio,
						Required: boolPointer(true), MediaTypes: []string{"audio/*"},
					},
					modelinference.OperationSlot{Name: "transcript", Modality: modelinference.ModalityText},
				), nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				invokes++
				return modelinference.InvokeModelResult{}, nil
			},
		},
		InputFileReader: func(_ context.Context, _ string, maxBytes int64) ([]byte, error) {
			receivedLimit = maxBytes
			return bytes.Repeat([]byte{'x'}, int(maxBytes+1)), nil
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "asr", Operation: modelinference.OperationASR,
		InputMappings: []string{"audio=@oversized.wav"}, JSON: true, Output: io.Discard,
	})
	var localFailure *clidiag.LocalFailure
	if err == nil || !errors.As(err, &localFailure) {
		t.Fatalf("oversized generic input error = %v, want safe local failure", err)
	}
	if receivedLimit <= 0 || invokes != 0 {
		t.Fatalf("oversized generic input effects = limit:%d invokes:%d, want positive limit and zero invokes", receivedLimit, invokes)
	}
}

func TestRootAdapter_InvokeGenericFileInputCancellationStopsPreparation(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	started := make(chan struct{})
	invokes := 0
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModelWithInputs("asr", modelinference.OperationASR,
					modelinference.OperationSlot{
						Name: "audio", Modality: modelinference.ModalityAudio,
						Required: boolPointer(true), MediaTypes: []string{"audio/*"},
					},
					modelinference.OperationSlot{Name: "transcript", Modality: modelinference.ModalityText},
				), nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				invokes++
				return modelinference.InvokeModelResult{}, nil
			},
		},
		InputFileReader: func(ctx context.Context, _ string, _ int64) ([]byte, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- service.Invoke(modelscli.InvokeConfig{
			Context: ctx, ModelName: "asr", Operation: modelinference.OperationASR,
			InputMappings: []string{"audio=@meeting.wav"}, JSON: true, Output: io.Discard,
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("generic input reader did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled generic input error = %v, want context.Canceled", err)
		}
		if invokes != 0 {
			t.Fatalf("canceled generic input invokes = %d, want zero", invokes)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled generic input did not stop preparation")
	}
}
