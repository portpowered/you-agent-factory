package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
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
								Modality: modelinference.ModalityText, MediaTypes: []string{"text/plain"}, Repeatable: true,
							}},
							Outputs: []modelinference.OperationSlot{{
								Name: "audio", ContentTypes: []string{"audio"},
								Modality: modelinference.ModalityAudio, MediaTypes: []string{"audio/wav"},
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
	if !strings.Contains(out.String(), `"text"`) || !strings.Contains(out.String(), `"audio"`) ||
		!strings.Contains(out.String(), `"text/plain"`) || !strings.Contains(out.String(), `"repeatable":true`) {
		t.Fatalf("Inspect() JSON = %q, want mapped operation slot facts", out.String())
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

func TestRootAdapter_InvokeGenericInputPreflightRejectsBeforeBackend(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		mappings    []string
		readErr     error
		readBytes   []byte
		wantClass   modelinference.InvocationFailureClass
		wantReads   int
		wantInvokes int
	}{
		{name: "missing assignment", mappings: []string{"audio"}, wantClass: modelinference.InvocationFailureClassInvalidSlot},
		{name: "empty slot", mappings: []string{"=audio.wav"}, wantClass: modelinference.InvocationFailureClassInvalidSlot},
		{name: "unknown slot", mappings: []string{"unknown=@audio.wav"}, wantClass: modelinference.InvocationFailureClassInvalidSlot},
		{name: "duplicate nonrepeatable", mappings: []string{"audio=@one.wav", "audio=@two.wav"}, wantClass: modelinference.InvocationFailureClassSlotArity},
		{name: "missing required slot", mappings: []string{"prompt=hint"}, wantClass: modelinference.InvocationFailureClassInvalidSlot},
		{name: "unreadable file", mappings: []string{"audio=@audio.wav"}, readErr: errors.New("not readable"), wantReads: 1},
		{name: "unsupported media", mappings: []string{"audio=@notes.txt"}, readBytes: []byte("not audio"), wantClass: modelinference.InvocationFailureClassMediaCapability, wantReads: 1},
	}

	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scope := testRuntimeScope(t)
			reads := 0
			invokes := 0
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
					invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
						invokes++
						return modelinference.InvokeModelResult{}, nil
					},
				},
				InputFileReader: func(context.Context, string, int64) ([]byte, error) {
					reads++
					return test.readBytes, test.readErr
				},
				OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
					return modelscli.InvokeRuntimeScope{Scope: scope}, nil
				},
			})

			err := service.Invoke(modelscli.InvokeConfig{
				Context: context.Background(), ModelName: "asr", Operation: modelinference.OperationASR,
				InputMappings: test.mappings, JSON: true, Output: io.Discard,
			})
			if test.readErr != nil {
				var failure *clidiag.LocalFailure
				if !errors.As(err, &failure) {
					t.Fatalf("Invoke() error = %v, want local input failure", err)
				}
			} else {
				var failure *modelinference.InvocationFailure
				if !errors.As(err, &failure) || failure.Class != test.wantClass {
					t.Fatalf("Invoke() error = %v, want invocation failure class %q", err, test.wantClass)
				}
			}
			if reads != test.wantReads || invokes != test.wantInvokes {
				t.Fatalf("preflight effects = reads:%d invokes:%d, want reads:%d invokes:%d", reads, invokes, test.wantReads, test.wantInvokes)
			}
		})
	}
}

func TestRootAdapter_InvokeASRRequiresEveryNamedOutputBeforeEffects(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	reads := 0
	invokes := 0
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIOperationModel("asr", modelinference.OperationASR,
					[]modelinference.OperationSlot{{
						Name: "audio", Modality: modelinference.ModalityAudio,
						Required: boolPointer(true), MediaTypes: []string{"audio/*"},
					}},
					[]modelinference.OperationSlot{
						{Name: "transcript", Modality: modelinference.ModalityText, Required: boolPointer(true)},
						{Name: "segments", Modality: modelinference.ModalityJSON, Required: boolPointer(true)},
					}), nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				invokes++
				return modelinference.InvokeModelResult{}, nil
			},
		},
		InputFileReader: func(context.Context, string, int64) ([]byte, error) {
			reads++
			return []byte("audio"), nil
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})
	err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "asr", Operation: modelinference.OperationASR,
		InputMappings:  []string{"audio=@meeting.wav"},
		OutputMappings: []string{"transcript=" + filepath.Join(t.TempDir(), "transcript.txt")},
		Output:         io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "transcript, segments") {
		t.Fatalf("ASR incomplete output mapping error = %v, want both output slots", err)
	}
	if reads != 0 || invokes != 0 {
		t.Fatalf("ASR incomplete mapping effects = reads:%d invokes:%d, want 0/0", reads, invokes)
	}
}

type localOutputFileSystem struct{}

func (localOutputFileSystem) CreateTemp(dir, pattern string) (modelscli.OutputTemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

func (localOutputFileSystem) Inspect(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (localOutputFileSystem) Remove(path string) error {
	return os.Remove(path)
}

func (localOutputFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func assertMappedCLIFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("mapped output %s = %q, %v; want %q", path, got, err, want)
	}
}

func assertMappedCLIResponse(t *testing.T, data []byte) {
	t.Helper()
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode mapped response: %v", err)
	}
	if len(response.Outputs) != 2 || response.Outputs[1].Name != "usage" || response.Outputs[1].MediaType == nil || *response.Outputs[1].MediaType != "application/json" {
		t.Fatalf("mapped response outputs = %#v, want named media metadata", response.Outputs)
	}
	if response.Outputs[1].Artifact == nil || response.Outputs[1].Artifact.SizeBytes == nil || *response.Outputs[1].Artifact.SizeBytes != 14 || response.Outputs[1].Artifact.Properties == nil || (*response.Outputs[1].Artifact.Properties)["digest"] != "sha256:usage" {
		t.Fatalf("mapped response artifact = %#v, want digest and bytes metadata", response.Outputs[1].Artifact)
	}
}

func genericCLIModel(name, operation string, outputs ...modelinference.OperationSlot) modelinference.GetModelResult {
	return modelinference.GetModelResult{Model: modelinference.Detail{
		Summary: modelinference.Summary{Name: name, Operations: []modelinference.Operation{{Name: operation, Outputs: outputs}}},
	}}
}

func genericCLIModelWithInputs(name, operation string, inputsAndOutputs ...modelinference.OperationSlot) modelinference.GetModelResult {
	return genericCLIOperationModel(name, operation,
		append([]modelinference.OperationSlot(nil), inputsAndOutputs[:2]...),
		append([]modelinference.OperationSlot(nil), inputsAndOutputs[2:]...),
	)
}

func genericCLIOperationModel(name, operation string, inputs, outputs []modelinference.OperationSlot) modelinference.GetModelResult {
	return modelinference.GetModelResult{Model: modelinference.Detail{
		Summary: modelinference.Summary{Name: name, Operations: []modelinference.Operation{{Name: operation, Inputs: inputs, Outputs: outputs}}},
	}}
}

func boolPointer(value bool) *bool {
	return &value
}

func TestRootAdapter_InvokeNamedLLMInputInfersOmniAndWritesRequiredText(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	required := boolPointer(true)
	optional := boolPointer(false)
	var gotRequest modelinference.InvokeModelRequest
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				if request.Operation != modelinference.OperationOMNI {
					return modelinference.GetModelResult{}, fmt.Errorf("operation = %q, want OMNI", request.Operation)
				}
				return genericCLIOperationModel(
					request.Name,
					modelinference.OperationOMNI,
					[]modelinference.OperationSlot{{Name: "prompt", Modality: modelinference.ModalityText, Required: required, MediaTypes: []string{"text/plain"}}},
					[]modelinference.OperationSlot{
						{Name: "text", Modality: modelinference.ModalityText, Required: required},
						{Name: "usage", Modality: modelinference.ModalityJSON, Required: optional},
					},
				), nil
			},
			invokeModel: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				gotRequest = request
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{
					{Name: "text", Modality: modelinference.ModalityText, Content: "fixture answer"},
					{Name: "usage", Modality: modelinference.ModalityJSON, Content: `{"tokens":3}`},
				}}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: modelinference.BuiltInModelNameLLM,
		InputMappings: []string{"prompt=Write a haiku"}, Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if out.String() != "fixture answer" {
		t.Fatalf("stdout = %q, want required text only", out.String())
	}
	if gotRequest.Operation != modelinference.OperationOMNI || len(gotRequest.Inputs) != 1 || gotRequest.Inputs[0].Name != "prompt" || gotRequest.Inputs[0].Content != "Write a haiku" {
		t.Fatalf("joined request = %#v, want inferred OMNI prompt input", gotRequest)
	}
}

func TestRootAdapter_InvokeGenericOutputPathPublishesSingleOutput(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	outputPath := filepath.Join(t.TempDir(), "answer.txt")
	if err := os.WriteFile(outputPath, []byte("old answer"), 0o600); err != nil {
		t.Fatalf("seed output path: %v", err)
	}
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModel("omni", modelinference.OperationOMNI,
					modelinference.OperationSlot{Name: "text", Modality: modelinference.ModalityText, Required: boolPointer(true)}), nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{
					Name: "text", Modality: modelinference.ModalityText, Content: "path answer",
				}}}, nil
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
		Text: "hello", OutputPath: outputPath, Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	assertMappedCLIFile(t, outputPath, "path answer")
	if out.String() != "Wrote audio: "+outputPath+"\n" {
		t.Fatalf("output notice = %q, want publication notice", out.String())
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
