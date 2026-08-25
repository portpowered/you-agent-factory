package cli_test

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
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const compositionCoverageUnreachableServer = "http://127.0.0.1:1"

func TestNew_ServerValidationUsesHTTPFallbackWhenCompositionRootExists(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/models/known" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"name":"known","operations":[{"name":"TTS"}]}`)
	}))
	t.Cleanup(server.Close)

	localCalled := false
	invocation := factorySessionPresentationInvocation{
		root: compositionModelsRoot{
			getModel: func(context.Context, string) (modelinference.Detail, error) {
				localCalled = true
				return modelinference.Detail{}, nil
			},
		},
	}
	service := modelscli.New(compositionHTTPProtocol(t), invocation)
	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "known", Operation: "TTS", Text: "hello",
		Server: server.URL, JSON: true, Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if localCalled {
		t.Fatal("server-bound validation opened the locally composed Models catalog")
	}
	if !strings.Contains(out.String(), `"mode":"VALIDATION_ONLY"`) {
		t.Fatalf("Invoke() output = %q, want validation-only metadata", out.String())
	}
}

func TestRootAdapter_InvokeGenericJSONIsValidationOnly(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
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
		Text: "hello", JSON: true, Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	var response struct {
		ModelName         string `json:"modelName"`
		Operation         string `json:"operation"`
		Mode              string `json:"mode"`
		ValidationOnly    bool   `json:"validationOnly"`
		InferenceExecuted bool   `json:"inferenceExecuted"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode validation JSON: %v\n%s", err, out.String())
	}
	if response.ModelName != "omni" || response.Operation != modelinference.OperationOMNI ||
		response.Mode != "VALIDATION_ONLY" || !response.ValidationOnly || response.InferenceExecuted {
		t.Fatalf("validation response = %#v, want validation-only metadata", response)
	}
}

func TestRootAdapter_BuiltInCatalogModelSurfacesThroughCLI(t *testing.T) {
	t.Parallel()

	service := parityRootService(t, stubModelsRoot{
		listModels: func(context.Context) (modelinference.List, error) {
			return modelinference.List{Results: []modelinference.Summary{{
				Name:       modelinference.BuiltInModelNameASR,
				Operations: []modelinference.Operation{{Name: modelinference.OperationASR}},
			}}}, nil
		},
		getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
			return modelinference.GetModelResult{Model: modelinference.Detail{Summary: modelinference.Summary{
				Name: request.Name, Operations: []modelinference.Operation{{Name: modelinference.OperationASR}},
			}}}, nil
		},
	})

	var listOutput bytes.Buffer
	if err := service.List(modelscli.ListConfig{Context: context.Background(), Output: &listOutput}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !strings.Contains(listOutput.String(), modelinference.BuiltInModelNameASR) {
		t.Fatalf("List() output = %q, want built-in asr", listOutput.String())
	}

	var inspectOutput bytes.Buffer
	if err := service.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: modelinference.BuiltInModelNameASR, Output: &inspectOutput,
	}); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if !strings.Contains(inspectOutput.String(), "Name:\tasr") || !strings.Contains(inspectOutput.String(), "ASR") {
		t.Fatalf("Inspect() output = %q, want built-in asr detail", inspectOutput.String())
	}
}

func TestRootAdapter_RemoteCommandsPreserveHTTPResponses(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(serveRemoteModelCommand))
	t.Cleanup(server.Close)

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{}, HTTP: compositionHTTPProtocol(t), PullHTTP: compositionHTTPProtocol(t),
	})
	serverURL := server.URL
	var listed bytes.Buffer
	if err := service.List(modelscli.ListConfig{Context: context.Background(), Server: serverURL, JSON: true, Output: &listed}); err != nil {
		t.Fatalf("remote List() = %v", err)
	}
	if !strings.Contains(listed.String(), `"name":"voice"`) {
		t.Fatalf("remote list = %q, want voice", listed.String())
	}
	var inspected bytes.Buffer
	if err := service.Inspect(modelscli.InspectConfig{Context: context.Background(), Server: serverURL, ModelName: "voice", Output: &inspected}); err != nil {
		t.Fatalf("remote Inspect() = %v", err)
	}
	if !strings.Contains(inspected.String(), "Name:\tvoice") {
		t.Fatalf("remote inspect = %q, want human model output", inspected.String())
	}
	var pulled bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{Context: context.Background(), Server: serverURL, ModelName: "voice", JSON: true, Output: &pulled}); err != nil {
		t.Fatalf("remote Pull() = %v", err)
	}
	if !strings.Contains(pulled.String(), `"outcome":"PULLED"`) {
		t.Fatalf("remote pull = %q, want PULLED", pulled.String())
	}
	var removed bytes.Buffer
	if err := service.Remove(modelscli.RemoveConfig{Context: context.Background(), Server: serverURL, ModelName: "voice", Output: &removed}); err != nil {
		t.Fatalf("remote Remove() = %v", err)
	}
	if !strings.Contains(removed.String(), "REMOVE OUTCOME") || !strings.Contains(removed.String(), "42 bytes") {
		t.Fatalf("remote remove = %q, want human removal output", removed.String())
	}
}

func serveRemoteModelCommand(writer http.ResponseWriter, request *http.Request) {
	responses := map[string]string{
		http.MethodGet + " /models":             `{"results":[{"name":"voice","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[]}]}`,
		http.MethodGet + " /models/voice":       `{"name":"voice","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"capabilities":[],"diagnostics":{},"managedRuntime":{"identity":"voice","readinessState":"READY","lifecycleState":"INSTALLED","locality":"LOCAL","supportedOperations":[{"name":"TTS"}],"diagnostics":{}}}`,
		http.MethodPost + " /models/voice/pull": `{"modelName":"voice","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"/models/voice/rev1","revision":"rev1","downloadedFiles":[],"managedRuntimePull":{"identity":"voice","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY"}}`,
		http.MethodDelete + " /models/voice":    `{"modelName":"voice","outcome":"REMOVED","revision":"rev1","cachePath":"/models/voice/rev1","bytesRemoved":42}`,
	}
	response, ok := responses[request.Method+" "+request.URL.Path]
	if !ok {
		http.NotFound(writer, request)
		return
	}
	_, _ = io.WriteString(writer, response)
}

func TestRootAdapter_InvokeOutputPropagatesRuntimeFailure(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	lease := testModelLease(t)
	runtimeErr := errors.New("inference runtime failed")
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{Model: modelinference.Detail{Summary: modelinference.Summary{Name: "voice"}}}, nil
			},
			acquireModelLease: func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
				return modelinference.AcquireModelLeaseResult{Lease: modelinference.ModelLease{Lease: lease}}, nil
			},
			invokeModelWithLease: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				if request.ResponseMode != modelinference.ResponseModeAudioStream {
					t.Fatalf("response mode = %q, want AUDIO_STREAM", request.ResponseMode)
				}
				return modelinference.InvokeModelResult{}, runtimeErr
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	var out bytes.Buffer
	err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "voice", Operation: "TTS", Text: "hello",
		JSON: true, OutputPath: filepath.Join(t.TempDir(), "speech.wav"), Output: &out,
	})
	if !errors.Is(err, runtimeErr) {
		t.Fatalf("Invoke() error = %v, want runtime failure", err)
	}
	if out.Len() != 0 {
		t.Fatalf("Invoke() output = %q, want no validation-only success envelope", out.String())
	}
}

func TestRootAdapter_BuiltInPullSurfacesThroughCLI(t *testing.T) {
	t.Parallel()

	root := stubModelsRoot{
		pullModel: func(_ context.Context, name string) (modelinference.PullResult, error) {
			return modelinference.PullResult{
				ModelName:          name,
				ProviderLocality:   string(modelinference.LocalityLocal),
				Outcome:            "PULLED",
				ManagedPullOutcome: "INSTALLED_SUCCESSFULLY",
				ReadinessState:     "READY",
				LifecycleState:     "INSTALLED",
			}, nil
		},
	}
	service := parityRootService(t, root)

	for _, name := range []string{
		modelinference.BuiltInModelNameASR,
		modelinference.BuiltInModelNameTTS,
	} {
		var output bytes.Buffer
		if err := service.Pull(modelscli.PullConfig{
			Context: context.Background(), ModelName: name, JSON: true, Output: &output,
		}); err != nil {
			t.Fatalf("Pull(%q): %v", name, err)
		}
		var response factoryapi.ModelPullResponse
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			t.Fatalf("Pull(%q) JSON: %v\n%s", name, err, output.String())
		}
		if response.ModelName != name || response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY {
			t.Fatalf("Pull(%q) response = %#v, want successful built-in pull", name, response)
		}
	}
}

func TestNewCompositionFacadeDelegatesListAndPullThroughOwnedRoot(t *testing.T) {
	t.Parallel()

	var pulled string
	root := compositionModelsRoot{
		listModels: func(context.Context) (modelinference.List, error) {
			return modelinference.List{
				Results: []modelinference.Summary{{Name: "OMNIVOICE_Q4_K_M"}},
			}, nil
		},
		pullModel: func(_ context.Context, name string) (modelinference.PullResult, error) {
			pulled = name
			return modelinference.PullResult{ModelName: name}, nil
		},
	}
	service := modelscli.New(
		compositionHTTPProtocol(t),
		compositionInvocation{root: root},
	)
	if service == nil {
		t.Fatal("New() = nil, want composition facade")
	}

	var listOut bytes.Buffer
	if err := service.List(modelscli.ListConfig{
		Context: context.Background(),
		Output:  &listOut,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !strings.Contains(listOut.String(), "OMNIVOICE_Q4_K_M") {
		t.Fatalf("List() output = %q, want model name", listOut.String())
	}

	var pullOut bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &pullOut,
	}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if pulled != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("pulled model = %q, want OMNIVOICE_Q4_K_M", pulled)
	}
}

func TestNewCompositionFacadeInvokeFallsBackToLegacyWhenServerSet(t *testing.T) {
	t.Parallel()

	err := modelscli.New(
		compositionHTTPProtocol(t),
		compositionInvocation{root: compositionModelsRoot{}},
	).Invoke(modelscli.InvokeConfig{
		Context:    context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello",
		Server:     compositionCoverageUnreachableServer,
		FactoryDir: t.TempDir(),
		Output:     io.Discard,
	})
	if err == nil {
		t.Fatal("Invoke() error = nil, want legacy remote/bootstrap failure")
	}
}

type exportingCompositionInvocation struct {
	compositionInvocation
}

func (exportingCompositionInvocation) ExportModelInvocationArtifact(sourcePath, destinationPath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(destinationPath, data, 0o600)
}

func TestConfigFromCompositionExportsInvocationArtifacts(t *testing.T) {
	t.Parallel()

	source := filepath.Join(t.TempDir(), "artifact.txt")
	if err := os.WriteFile(source, []byte("artifact"), 0o600); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "copied.txt")

	cfg := modelscli.ConfigFromComposition(
		compositionHTTPProtocol(t),
		exportingCompositionInvocation{compositionInvocation{root: compositionModelsRoot{}}},
	)
	if cfg.Artifacts == nil {
		t.Fatal("ConfigFromComposition().Artifacts = nil, want artifact exporter")
	}
	if err := cfg.Artifacts.ExportInvocationArtifact(source, destination); err != nil {
		t.Fatalf("ExportInvocationArtifact() error = %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read copied artifact: %v", err)
	}
	if string(got) != "artifact" {
		t.Fatalf("copied artifact = %q, want %q", string(got), "artifact")
	}
}

func TestConstructedService_RemoteListReportsUnreachableEndpoint(t *testing.T) {
	t.Parallel()

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{},
		HTTP:   compositionHTTPProtocol(t),
	})
	if service == nil {
		t.Fatal("NewService() = nil, want Models CLI service")
	}
	err := service.List(modelscli.ListConfig{
		Context: context.Background(),
		Server:  compositionCoverageUnreachableServer,
		Output:  io.Discard,
	})
	if err == nil {
		t.Fatal("List() error = nil, want unreachable endpoint failure")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("List() error = %q, want %q", err.Error(), want)
	}
}

func TestConstructedService_RemoteInspectReportsUnreachableEndpoint(t *testing.T) {
	t.Parallel()

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{},
		HTTP:   compositionHTTPProtocol(t),
	})
	err := service.Inspect(modelscli.InspectConfig{
		Context:   context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    compositionCoverageUnreachableServer,
		Output:    io.Discard,
	})
	if err == nil {
		t.Fatal("Inspect() error = nil, want unreachable endpoint failure")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Inspect() error = %q, want %q", err.Error(), want)
	}
}

func TestConstructedService_RemotePullReportsUnreachableEndpoint(t *testing.T) {
	t.Parallel()

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{},
		HTTP:   compositionHTTPProtocol(t),
	})
	err := service.Pull(modelscli.PullConfig{
		Context:   context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    compositionCoverageUnreachableServer,
		Output:    io.Discard,
	})
	if err == nil {
		t.Fatal("Pull() error = nil, want unreachable endpoint failure")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M/pull"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Pull() error = %q, want %q", err.Error(), want)
	}
}

func TestConstructedService_InspectMapsOperationSlots(t *testing.T) {
	t.Parallel()

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getModel: func(context.Context, string) (modelinference.Detail, error) {
				return modelinference.Detail{
					Summary: modelinference.Summary{
						Name: "OMNIVOICE_Q4_K_M",
						Operations: []modelinference.Operation{{
							Name: "TTS",
							Inputs: []modelinference.OperationSlot{{
								Name: "text", ContentTypes: []string{"text"}, Required: boolPtr(true),
							}},
							Outputs: []modelinference.OperationSlot{{
								Name: "audio", ContentTypes: []string{"audio"},
							}},
						}},
					},
					Capabilities: []modelinference.Capability{{
						Worker: "tts-worker",
						Operations: []modelinference.Operation{{
							Name: "TTS",
							Inputs: []modelinference.OperationSlot{{
								Name: "text", ContentTypes: []string{"text"},
							}},
						}},
						ResourceNames: []string{"gpu"},
					}},
				}, nil
			},
		},
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			scope, err := (modelinference.RuntimeScopeRef{}).Parse("composition-coverage:catalog-scope")
			if err != nil {
				return modelscli.InvokeRuntimeScope{}, err
			}
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})
	var out bytes.Buffer
	if err := service.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", JSON: true, Output: &out,
	}); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if !strings.Contains(out.String(), `"text"`) || !strings.Contains(out.String(), `"audio"`) {
		t.Fatalf("Inspect() JSON = %q, want mapped operation slots", out.String())
	}
	if !strings.Contains(out.String(), `"tts-worker"`) || !strings.Contains(out.String(), `"gpu"`) {
		t.Fatalf("Inspect() JSON = %q, want mapped capabilities", out.String())
	}
}

func TestConstructedService_RemotePullJSONEncodesSuccessResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M/pull" {
			t.Fatalf("path = %q, want pull path", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(factoryapi.ModelPullResponse{ModelName: "OMNIVOICE_Q4_K_M"})
	}))
	defer server.Close()

	var out bytes.Buffer
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{},
		HTTP:   compositionHTTPProtocol(t),
	})
	if err := service.Pull(modelscli.PullConfig{
		Context:   context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    strings.TrimSuffix(server.URL, "/"),
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !strings.Contains(out.String(), "OMNIVOICE_Q4_K_M") {
		t.Fatalf("Pull() JSON = %q, want model name", out.String())
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestRootAdapterPullProgressStaysOnStderr(t *testing.T) {
	scope := testRuntimeScope(t)
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			pullModel: func(ctx context.Context, name string) (modelinference.PullResult, error) {
				pullsupport.ReportProgress(ctx, pullsupport.ProgressObservation{
					ModelName: name, Artifact: "model.bin", TransferredBytes: 512, TotalBytes: 1024,
				})
				return modelinference.PullResult{ModelName: name}, nil
			},
		},
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		PullProgressInterval: time.Hour,
	})

	var stdout, stderr bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "voice", Output: &stdout, Progress: &stderr,
	}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !strings.Contains(stderr.String(), `phase=pull`) ||
		!strings.Contains(stderr.String(), `transferredBytes=512 totalBytes=1024 percent=50.0%`) {
		t.Fatalf("pull stderr = %q, want pull progress with byte totals", stderr.String())
	}
	if strings.Contains(stdout.String(), "models pull progress") {
		t.Fatalf("pull stdout = %q, must contain final result only", stdout.String())
	}
}

func TestRootAdapterInvokeProgressCoversImplicitPreparation(t *testing.T) {
	scope := testRuntimeScope(t)
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIOperationModel(
					request.Name,
					modelinference.OperationTTS,
					[]modelinference.OperationSlot{{
						Name: "text", Modality: modelinference.ModalityText, Required: boolPointer(true),
					}},
					[]modelinference.OperationSlot{{
						Name: "result", Modality: modelinference.ModalityText, Required: boolPointer(true),
					}},
				), nil
			},
			invokeModel: func(ctx context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				pullsupport.ReportProgress(ctx, pullsupport.ProgressObservation{
					ModelName: request.Model.NameOrURI, Artifact: "voice.bin", TransferredBytes: 3, TotalBytes: 4,
				})
				return modelinference.InvokeModelResult{
					ModelName: request.Model.NameOrURI, Operation: request.Operation,
					Outputs: []modelinference.InferenceOutput{{
						Name: "result", Modality: modelinference.ModalityText, Content: "ready",
					}},
				}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		PullProgressInterval: time.Hour,
	})

	var stdout, stderr bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "voice", Operation: modelinference.OperationTTS,
		Text: "hello", JSON: true, Output: &stdout, Progress: &stderr,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !strings.Contains(stderr.String(), `phase=preparation`) ||
		!strings.Contains(stderr.String(), `transferredBytes=3 totalBytes=4 percent=75.0%`) {
		t.Fatalf("invoke stderr = %q, want preparation progress with byte totals", stderr.String())
	}
	if strings.Contains(stdout.String(), "models pull progress") || !strings.Contains(stdout.String(), "ready") {
		t.Fatalf("invoke stdout = %q, want final JSON result only", stdout.String())
	}
}

func TestRootAdapterPullProgressPreservesFailureAndStops(t *testing.T) {
	scope := testRuntimeScope(t)
	wantErr := errors.New("transfer failed")
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			pullModel: func(ctx context.Context, name string) (modelinference.PullResult, error) {
				pullsupport.ReportProgress(ctx, pullsupport.ProgressObservation{
					ModelName: name, Artifact: "model.bin", TransferredBytes: 1, TotalBytes: 2,
				})
				return modelinference.PullResult{}, wantErr
			},
		},
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		PullProgressInterval: time.Hour,
	})

	var stdout, stderr bytes.Buffer
	err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "voice", Output: &stdout, Progress: &stderr,
	})
	if err == nil || (!errors.Is(err, wantErr) && !strings.Contains(err.Error(), wantErr.Error())) {
		t.Fatalf("Pull() error = %v, want transfer failure", err)
	}
	if !strings.Contains(stderr.String(), `phase=pull`) {
		t.Fatalf("failure stderr = %q, want progress before failure", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure stdout = %q, want no success result", stdout.String())
	}
}
