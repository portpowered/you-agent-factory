package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// TestModelsInferenceInvokeActivatesThroughRootBuildProcess proves a process
// constructed only through root.BuildProcess executes the current built-in TTS
// model and returns observable inference output while host, runtime, and asset
// external effects are replaced exclusively through published edges.Edges
// fields.
func runModelsInferenceInvokeActivatesThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	audio := []byte("RIFF....WAVE")
	modelServer := characterizationNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)

	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	hostHTTP := &recordingModelHTTPClient{delegate: modelServer.Client()}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	assetFiles := functionalModelAssetFileSystem{home: home}

	dir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	environment := functionalHomeEnvironment(home)
	process := characterizationBuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient:           rejectingNetwork,
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostHTTPClient:            hostHTTP,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(
			context.Context,
			serviceedges.ModelBackendArtifactSelectionRequest,
		) (serviceedges.ModelBackendArtifactSelection, error) {
			return pinnedTTSBackendSelection(), nil
		},
		ModelInvocationBackend: func(
			context.Context,
			models.InvokeModelRequest,
		) ([]models.InferenceContent, []models.InferenceArtifact, error) {
			return []models.InferenceContent{{
				Name: "audio", Modality: models.ModalityAudio,
				ContentType: "audio/wav", MediaType: "audio/wav", Content: string(audio),
			}}, nil, nil
		},
		ModelRuntimeHTTPClient: modelServer.Client(),
	})
	support.CleanupProcess(t, process)

	var output bytes.Buffer
	jsonInvoke := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "tts",
		"--operation", "TTS", "--text", "hello from root composition invoke",
	})
	jsonInvoke.Input.Env = environment
	jsonInvoke.Input.WorkingDirectory = dir
	jsonInvoke.Input.Stdout = &output
	jsonInvoke.Input.Stderr = io.Discard
	if err := process.Execute(jsonInvoke.Input); err != nil {
		t.Fatalf("Process.Execute(models invoke --json) error = %v", err)
	}

	var response struct {
		ModelName         string `json:"modelName"`
		Operation         string `json:"operation"`
		Mode              string `json:"mode"`
		ValidationOnly    bool   `json:"validationOnly"`
		InferenceExecuted bool   `json:"inferenceExecuted"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode models invoke output: %v\n%s", err, output.String())
	}
	if response.ModelName != "tts" || response.Operation != "TTS" ||
		response.Mode != "VALIDATION_ONLY" || !response.ValidationOnly || response.InferenceExecuted {
		t.Fatalf("models invoke response = %#v, want validation-only tts/TTS metadata", response)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("asset network calls = %d during invoke, want 0 via edges", rejectingNetwork.Calls())
	}
	assertRootTTSOutputFile(t, process, dir, environment, audio)
}

func assertRootTTSOutputFile(
	t *testing.T,
	process support.Process,
	directory string,
	environment []string,
	wantAudio []byte,
) {
	t.Helper()
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	var output bytes.Buffer
	audioInvoke := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts",
		"--operation", "TTS", "--text", "write audio from root composition invoke",
		"--output", audioPath,
	})
	audioInvoke.Input.Env = environment
	audioInvoke.Input.WorkingDirectory = directory
	audioInvoke.Input.Stdout = &output
	audioInvoke.Input.Stderr = io.Discard
	if err := process.Execute(audioInvoke.Input); err != nil {
		t.Fatalf("Process.Execute(models invoke --output) error = %v", err)
	}
	written, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("read models invoke audio: %v", err)
	}
	if !bytes.Equal(written, wantAudio) {
		t.Fatalf("models invoke audio = %q, want %q", written, wantAudio)
	}
}

// TestModelsJoinedBuiltinInvokeWithoutFactoryDeclaration proves the built-in
// tts definition reaches the joined kernel through root.BuildProcess and
// Process.Execute without a redundant Factory resource or worker declaration.
func TestModelsJoinedBuiltinInvokeWithoutFactoryDeclaration(t *testing.T) {
	t.Parallel()

	modelServer := characterizationNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/health":
			writer.WriteHeader(http.StatusOK)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	backendResolverCalls := 0
	assetFiles := functionalModelAssetFileSystem{home: home}
	dir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := characterizationBuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient:           rejectingNetwork,
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelResolveBackendArtifact: func(
			context.Context,
			serviceedges.ModelBackendArtifactSelectionRequest,
		) (serviceedges.ModelBackendArtifactSelection, error) {
			backendResolverCalls++
			return pinnedTTSBackendSelection(), nil
		},
		ModelAssetHostPlatform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelHostHTTPClient:    modelServer.Client(),
		ModelRuntimeHTTPClient: modelServer.Client(),
	})
	support.CleanupProcess(t, process)

	var output bytes.Buffer
	jsonInvoke := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "tts", "--operation", "TTS", "--text", "joined cache input",
	})
	jsonInvoke.Input.Env = functionalHomeEnvironment(home)
	jsonInvoke.Input.WorkingDirectory = dir
	jsonInvoke.Input.Stdout = &output
	jsonInvoke.Input.Stderr = io.Discard
	if err := process.Execute(jsonInvoke.Input); err != nil {
		t.Fatalf("Process.Execute(joined built-in invoke) error = %v", err)
	}

	var response struct {
		ModelName         string `json:"modelName"`
		Operation         string `json:"operation"`
		Mode              string `json:"mode"`
		ValidationOnly    bool   `json:"validationOnly"`
		InferenceExecuted bool   `json:"inferenceExecuted"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode joined models invoke output: %v\n%s", err, output.String())
	}
	if response.ModelName != "tts" || response.Operation != "TTS" ||
		response.Mode != "VALIDATION_ONLY" || !response.ValidationOnly || response.InferenceExecuted {
		t.Fatalf("joined models invoke response = %#v, want validation-only tts/TTS metadata", response)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("joined asset network calls = %d, want 0 from content-addressed cache", rejectingNetwork.Calls())
	}
	if backendResolverCalls != 0 {
		t.Fatalf("joined backend resolver calls = %d, want no inference attempt in validation-only mode", backendResolverCalls)
	}

}

func builtInOnlyModelFactoryConfig() map[string]any {
	return map[string]any{
		"name": "models-built-in-only",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
	}
}

func findModelSummary(results []factoryapi.ModelSummary, name string) (factoryapi.ModelSummary, bool) {
	for _, result := range results {
		if result.Name == name {
			return result, true
		}
	}
	return factoryapi.ModelSummary{}, false
}

// TestModelsGenericHTTPInvocationReachesJoinedRootThroughProcess proves the
// registered generic HTTP route uses the live Models scope and returns named
// output from the joined root rather than the legacy model-invocation envelope.
func TestModelsGenericHTTPInvocationReachesJoinedRootThroughProcess(t *testing.T) {
	t.Parallel()

	modelServer := characterizationNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	config := localModelReadinessAssetsHostFactoryConfig(modelServer.URL)
	resources := config["resources"].([]map[string]any)
	resources[0]["model"] = "tts"
	resources[0]["backend"] = "localai-vibevoice"
	workers := config["workers"].([]map[string]any)
	workers[0]["name"] = "tts-worker"
	workers[0]["model"] = "tts"
	workers[0]["args"] = []string{"--grpc-endpoint", modelServer.URL}
	dir := support.ScaffoldFactory(t, config)
	server := characterizationStartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env:                       functionalHomeEnvironment(home),
		Edges:                     genericHTTPInvocationEdges(rejectingNetwork, assetFiles, hostLauncher, protocol, compatibility, modelServer),
	})

	inputs := []factoryapi.ModelInvocationInput{{
		Name: "text", Modality: factoryapi.ModelInvocationContentTypeText,
		Content: func() *string { value := "generic HTTP input"; return &value }(),
	}}
	response := postFunctionalJSON[factoryapi.GenericModelInvocationResponse](
		t,
		server.URL()+"/models/invocations",
		factoryapi.GenericModelInvocationRequest{
			Scope: "factory-session:caller-supplied", Holder: "functional-generic-http",
			Model: factoryapi.ModelReference{NameOrUri: "tts"}, Inputs: &inputs,
		},
		"POST /models/invocations",
	)
	if len(response.Outputs) != 1 || response.Outputs[0].Name != "audio" || response.Outputs[0].Modality != factoryapi.ModelInvocationContentTypeAudio || response.Outputs[0].Content == nil || *response.Outputs[0].Content != "generic HTTP input" {
		t.Fatalf("generic HTTP response = %#v, want one named audio output", response)
	}
	if rejectingNetwork.Calls() != 0 || hostLauncher.Calls() != 1 || protocol.Calls() == 0 || compatibility.Calls() == 0 {
		t.Fatalf("generic HTTP effects = network %d, starts %d, protocol %d, compatibility %d; want cache hit and joined lifecycle", rejectingNetwork.Calls(), hostLauncher.Calls(), protocol.Calls(), compatibility.Calls())
	}
	functionalevidence.Covers(t, "rest/invokeGenericModel")
}

// TestModelsNamedAndGenericHTTPInvocationShareBuiltinResolution proves both
// public invocation routes use the same effective built-in definition when no
// Factory worker is declared. The generic route keeps its slot-named output
// contract; the named route keeps its legacy worker/content response shape.
func testModelsNamedAndGenericHTTPInvocationShareBuiltinResolution(t *testing.T) {
	t.Parallel()

	modelServer := characterizationNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	dir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	server := characterizationStartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env:                       functionalHomeEnvironment(home),
		Edges:                     genericHTTPInvocationEdges(rejectingNetwork, assetFiles, hostLauncher, protocol, compatibility, modelServer),
	})

	text := "builtin parity input"
	inputs := []factoryapi.ModelInvocationInput{{
		Name: "text", Modality: factoryapi.ModelInvocationContentTypeText,
		Content: &text,
	}}
	operation := factoryapi.ModelOperationName(models.OperationTTS)
	genericResponse := postFunctionalJSON[factoryapi.GenericModelInvocationResponse](
		t,
		server.URL()+"/models/invocations",
		factoryapi.GenericModelInvocationRequest{
			Scope: "factory-session:parity-generic", Holder: "functional-parity",
			Model: factoryapi.ModelReference{NameOrUri: "tts"}, Operation: &operation, Inputs: &inputs,
		},
		"POST /models/invocations built-in parity",
	)
	if len(genericResponse.Outputs) != 1 || genericResponse.Outputs[0].Name != "audio" ||
		genericResponse.Outputs[0].Modality != factoryapi.ModelInvocationContentTypeAudio ||
		genericResponse.Outputs[0].Content == nil || *genericResponse.Outputs[0].Content != text {
		t.Fatalf("generic built-in parity response = %#v, want one audio output containing input", genericResponse)
	}

	namedContent := factoryapi.WorkContent{mustFunctionalTextPart(t, text)}
	namedRequest := factoryapi.ModelInvocationRequest{
		Operation: "TTS", Bindings: localModelReadinessAssetsHostBindings(), Content: &namedContent,
	}
	namedBody, err := json.Marshal(namedRequest)
	if err != nil {
		t.Fatalf("marshal named built-in parity request: %v", err)
	}
	namedHTTPResponse, err := http.Post(
		server.URL()+"/models/tts/invocations", "application/json", bytes.NewReader(namedBody),
	)
	if err != nil {
		t.Fatalf("POST /models/{model_name}/invocations built-in parity: %v", err)
	}
	var namedFailure factoryapi.ErrorResponse
	if err := json.NewDecoder(namedHTTPResponse.Body).Decode(&namedFailure); err != nil {
		namedHTTPResponse.Body.Close()
		t.Fatalf("decode named built-in parity response: %v", err)
	}
	namedHTTPResponse.Body.Close()
	if namedHTTPResponse.StatusCode != http.StatusNotFound ||
		string(namedFailure.Code) != "MODEL_NOT_AVAILABLE" ||
		namedFailure.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("named built-in parity response = status %d, %#v; want effective-definition readiness 404", namedHTTPResponse.StatusCode, namedFailure)
	}
	if strings.Contains(strings.ToLower(namedFailure.Message), "model not found") ||
		strings.Contains(string(namedFailure.Code), "MODEL_INFERENCE_RUNTIME_FAILURE") ||
		namedFailure.Family == factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("named built-in parity retained a worker-lookup failure: %#v", namedFailure)
	}
	if rejectingNetwork.Calls() != 0 || hostLauncher.Calls() != 0 || protocol.Calls() != 0 || compatibility.Calls() != 0 {
		t.Fatalf("built-in parity effects = network %d, starts %d, protocol %d, compatibility %d; want no external effects for the cache-backed generic attempt or named readiness rejection", rejectingNetwork.Calls(), hostLauncher.Calls(), protocol.Calls(), compatibility.Calls())
	}
}

// TestModelsNamedBuiltinRouteUsesEffectiveDefinitionWithoutWorker proves the
// named route uses the discovered built-in definition even when the Factory
// declares no matching inference worker, and preserves actionable readiness
// and unknown-reference failures.
func TestModelsNamedBuiltinRouteUsesEffectiveDefinitionWithoutWorker(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	runner := support.NewRecordingCommandRunner("provider should not run before managed readiness")
	server := characterizationStartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})

	assertEffectiveBuiltinDiscovery(t, server.URL())
	assertEffectiveBuiltinReadinessFailures(t, server.URL())
	assertUnknownBuiltinFailure(t, server.URL())
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want readiness to reject unavailable built-ins before execution", runner.CallCount())
	}
}

func assertEffectiveBuiltinDiscovery(t *testing.T, serverURL string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListModelsResponse](t, serverURL+"/models")
	if _, ok := findModelSummary(listed.Results, models.BuiltInModelNameTTS); !ok {
		t.Fatalf("GET /models did not expose effective built-in %q; results=%#v", models.BuiltInModelNameTTS, listed.Results)
	}
	inspected := support.GetJSON[factoryapi.ModelDetail](t, serverURL+"/models/"+models.BuiltInModelNameTTS)
	if inspected.Name != models.BuiltInModelNameTTS || len(inspected.Operations) != 1 || inspected.Operations[0].Name != models.OperationTTS {
		t.Fatalf("GET /models/%s = %#v, want effective TTS definition", models.BuiltInModelNameTTS, inspected)
	}
}

func assertEffectiveBuiltinReadinessFailures(t *testing.T, serverURL string) {
	t.Helper()
	for _, modelName := range []string{models.BuiltInModelNameTTS, models.BuiltInModelNameASR} {
		operation := models.OperationTTS
		if modelName == models.BuiltInModelNameASR {
			operation = models.OperationASR
		}
		failure := postNamedBuiltinFailure(t, serverURL, modelName, operation)
		if failure.StatusCode != http.StatusNotFound || string(failure.Body.Code) != "MODEL_NOT_AVAILABLE" {
			t.Fatalf("POST /models/%s/invocations = status %d, failure %#v; want effective-definition readiness failure", modelName, failure.StatusCode, failure.Body)
		}
		if strings.Contains(strings.ToLower(failure.Body.Message), "model not found") ||
			strings.Contains(string(failure.Body.Code), "MODEL_INFERENCE_RUNTIME_FAILURE") ||
			failure.Body.Family == factoryapi.ErrorFamilyInternalServerError {
			t.Fatalf("POST /models/%s/invocations retained a worker-lookup failure: %#v", modelName, failure.Body)
		}
	}
}

type namedBuiltinFailure struct {
	StatusCode int
	Body       factoryapi.ErrorResponse
}

func postNamedBuiltinFailure(t *testing.T, serverURL, modelName, operation string) namedBuiltinFailure {
	t.Helper()
	body, err := json.Marshal(factoryapi.ModelInvocationRequest{Operation: operation})
	if err != nil {
		t.Fatalf("marshal %s invocation: %v", modelName, err)
	}
	response, err := http.Post(serverURL+"/models/"+modelName+"/invocations", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /models/%s/invocations: %v", modelName, err)
	}
	defer response.Body.Close()
	var failure factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatalf("decode %s invocation failure: %v", modelName, err)
	}
	return namedBuiltinFailure{StatusCode: response.StatusCode, Body: failure}
}

func assertUnknownBuiltinFailure(t *testing.T, serverURL string) {
	t.Helper()
	failure := postNamedBuiltinFailure(t, serverURL, "unknown-discovered-model", models.OperationTTS)
	if failure.StatusCode != http.StatusNotFound ||
		string(failure.Body.Code) != "MODEL_NOT_AVAILABLE" ||
		failure.Body.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("POST /models/unknown-discovered-model/invocations = status %d, failure %#v; want actionable model-not-available 404", failure.StatusCode, failure.Body)
	}
	if strings.Contains(string(failure.Body.Code), "MODEL_INFERENCE_RUNTIME_FAILURE") ||
		failure.Body.Family == factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("unknown model invocation retained an internal failure classification: %#v", failure.Body)
	}
}

func runModelsGenericCLIOutputModesReachJoinedRootThroughProcess(t *testing.T) {
	t.Parallel()

	modelServer := characterizationNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinModelCache(t, home, "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3")
	selection := genericLlamaBackendSelection()
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, []byte("localai-llamacpp/linux-amd64"))
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	dir := support.ScaffoldFactory(t, multiOutputModelFactoryConfig(modelServer.URL))
	process := characterizationBuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient: rejectingNetwork,
		ModelResolveBackendArtifact: func(_ context.Context, request serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			return selection, nil
		},
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelHostHTTPClient:            modelServer.Client(),
		ModelRuntimeHTTPClient:         modelServer.Client(),
		ModelInvocationProtocolClient:  genericCLIProtocolClient{},
	})
	environment := functionalHomeEnvironment(home)

	var output bytes.Buffer
	rejected := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI", "--text", "multi output preflight",
	})
	err := executeRootModelsCLI(t, process, dir, environment, &output, rejected.Input.Args)
	if err == nil || !strings.Contains(err.Error(), "text, usage") {
		t.Fatalf("multi-output CLI error = %v, want named preflight slots", err)
	}
	if hostLauncher.Calls() != 0 || rejectingNetwork.Calls() != 0 {
		t.Fatalf("multi-output preflight effects = starts %d, network %d; want 0/0", hostLauncher.Calls(), rejectingNetwork.Calls())
	}

	jsonInvoke := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "llm", "--operation", "OMNI", "--text", "all outputs",
	})
	if err := executeRootModelsCLI(t, process, dir, environment, &output, jsonInvoke.Input.Args); err != nil {
		t.Fatalf("Process.Execute(multi-output --json) error = %v", err)
	}
	assertGenericCLIValidationOnly(t, &output)

	textPath := filepath.Join(t.TempDir(), "text.out")
	usagePath := filepath.Join(t.TempDir(), "usage.out")
	output.Reset()
	mappedInvoke := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI", "--text", "mapped outputs",
		"--output-map", "text=" + textPath, "--output-map", "usage=" + usagePath,
	})
	if err := executeRootModelsCLI(t, process, dir, environment, &output, mappedInvoke.Input.Args); err != nil {
		t.Fatalf("Process.Execute(multi-output mappings) error = %v", err)
	}
	for _, test := range []struct{ path, want string }{
		{path: textPath, want: "mapped outputs"}, {path: usagePath, want: "mapped outputs"},
	} {
		data, err := os.ReadFile(test.path)
		if err != nil || string(data) != test.want {
			t.Fatalf("mapped output %s = %q, %v; want %q", test.path, data, err, test.want)
		}
	}
	assertMappedGenericCLIResponse(t, &output)
	if hostLauncher.Calls() != 1 {
		t.Fatalf("mapped output response/effects = %q, starts %d; want one start for the mapped invocation", output.String(), hostLauncher.Calls())
	}
	closeRootProcess(t, process, "close multi-output root process")
}

func assertGenericCLIValidationOnly(t testing.TB, output *bytes.Buffer) {
	t.Helper()
	var response struct {
		ModelName         string `json:"modelName"`
		Operation         string `json:"operation"`
		Mode              string `json:"mode"`
		ValidationOnly    bool   `json:"validationOnly"`
		InferenceExecuted bool   `json:"inferenceExecuted"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode multi-output validation JSON = %v\n%s", err, output.String())
	}
	if response.ModelName != "llm" || response.Operation != "OMNI" ||
		response.Mode != "VALIDATION_ONLY" || !response.ValidationOnly || response.InferenceExecuted {
		t.Fatalf("multi-output validation JSON = %#v, want validation-only metadata", response)
	}
}

func assertMappedGenericCLIResponse(t testing.TB, output *bytes.Buffer) {
	t.Helper()
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode mapped output response: %v\n%s", err, output.String())
	}
	if len(response.Outputs) != 2 || response.Outputs[0].Name != "text" || response.Outputs[0].Content == nil || *response.Outputs[0].Content != "mapped outputs" {
		t.Fatalf("mapped output inline metadata = %#v, want text bytes", response.Outputs)
	}
	if response.Outputs[0].ContentType == nil || *response.Outputs[0].ContentType != "text/plain" || response.Outputs[1].MediaType == nil || *response.Outputs[1].MediaType != "application/json" {
		t.Fatalf("mapped output media metadata = %#v, want text/plain and application/json", response.Outputs)
	}
}

func executeRootModelsCLI(
	t testing.TB,
	process support.Process,
	directory string,
	environment []string,
	output io.Writer,
	args []string,
) error {
	t.Helper()
	inputs := support.FakeInputs(context.Background(), args)
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = directory
	inputs.Input.Stdout = output
	inputs.Input.Stderr = io.Discard
	return process.Execute(inputs.Input)
}

func closeRootProcess(t testing.TB, process support.Process, message string) {
	t.Helper()
	closer, ok := process.(interface{ Close(context.Context) error })
	if !ok {
		t.Fatal("root process does not expose lifecycle close")
	}
	if err := closer.Close(context.Background()); err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func genericHTTPInvocationEdges(
	rejectingNetwork *rejectingModelAssetHTTP,
	assetFiles functionalModelAssetFileSystem,
	hostLauncher *recordingModelHostLauncher,
	protocol *joinedProtocolNegotiator,
	compatibility *joinedCompatibilityChecker,
	modelServer *httptest.Server,
) serviceedges.Edges {
	return serviceedges.Edges{
		ModelAssetHTTPClient: rejectingNetwork,
		ModelResolveBackendArtifact: func(_ context.Context, request serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			switch request.Backend {
			case "localai-llamacpp":
				return genericLlamaBackendSelection(), nil
			case "localai-vibevoice":
				return pinnedTTSBackendSelection(), nil
			default:
				return serviceedges.ModelBackendArtifactSelection{}, fmt.Errorf("unexpected generic fixture backend %q", request.Backend)
			}
		},
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelHostHTTPClient:            modelServer.Client(),
		ModelRuntimeHTTPClient:         modelServer.Client(),
	}
}

func multiOutputModelFactoryConfig(endpoint string) map[string]any {
	config := localModelReadinessAssetsHostFactoryConfig(endpoint)
	resources := config["resources"].([]map[string]any)
	resources[0]["name"] = "llm-cache"
	resources[0]["model"] = "llm"
	resources[0]["backend"] = "localai-llamacpp"
	workers := config["workers"].([]map[string]any)
	workers[0]["name"] = "llm-worker"
	workers[0]["model"] = "llm"
	workers[0]["command"] = "llama-cpp"
	workers[0]["args"] = []string{"--grpc-endpoint", endpoint}
	workerResources := workers[0]["resources"].([]map[string]any)
	workerResources[0]["name"] = "llm-cache"
	workers[0]["operations"] = []map[string]any{{
		"name": "OMNI",
		"inputs": []map[string]any{{
			"name": "prompt", "contentTypes": []string{interfaces.ModelOperationContentTypeText}, "required": true,
		}},
		"outputs": []map[string]any{
			{"name": "text", "contentTypes": []string{interfaces.ModelOperationContentTypeText}},
			{"name": "usage", "contentTypes": []string{interfaces.ModelOperationContentTypeJSON}},
		},
	}}
	return config
}

func runModelsJoinedInvokeRejectsPinnedBackendBeforeProcessStartThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	modelServer := characterizationNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{failure: errors.New("fixture backend is incompatible")}
	assetFiles := functionalModelAssetFileSystem{home: home}
	config := localModelReadinessAssetsHostFactoryConfig(modelServer.URL)
	resources := config["resources"].([]map[string]any)
	resources[0]["model"] = "tts"
	resources[0]["backend"] = "localai-vibevoice"
	workers := config["workers"].([]map[string]any)
	workers[0]["model"] = "tts"
	workers[0]["args"] = []string{"--grpc-endpoint", modelServer.URL}
	dir := support.ScaffoldFactory(t, config)
	process := characterizationBuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient:           rejectingNetwork,
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "freebsd", Architecture: "amd64"},
		ModelHostHTTPClient:            modelServer.Client(),
		ModelRuntimeHTTPClient:         modelServer.Client(),
	})

	outputPath := filepath.Join(t.TempDir(), "must-fail.wav")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "must fail preflight", "--output", outputPath,
	})
	inputs.Input.Env = functionalHomeEnvironment(home)
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Stdout = io.Discard
	inputs.Input.Stderr = io.Discard
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(incompatible joined invoke) error = nil, want preflight failure")
	}
	if compatibility.Calls() != 0 || protocol.Calls() != 0 || hostLauncher.Calls() != 0 {
		t.Fatalf(
			"pinned preflight effects = compatibility %d, protocol %d, starts %d; want 0/0/0",
			compatibility.Calls(), protocol.Calls(), hostLauncher.Calls(),
		)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("pinned preflight asset network calls = %d, want 0", rejectingNetwork.Calls())
	}
}

func writeGenericBuiltinTTSCache(t *testing.T, home string) {
	t.Helper()
	writeGenericBuiltinModelCache(t, home, "hf://vibevoice/VibeVoice-7B@505114ae6ad17be74df98e6939707434ec49c187")
}

func writeGenericBuiltinModelCache(t *testing.T, home, source string) {
	t.Helper()
	name := "weights.bin"
	if sourcePath := strings.Split(strings.TrimSuffix(strings.TrimSpace(source), "@"), "@")[0]; strings.Contains(sourcePath, "/") {
		parts := strings.Split(strings.Trim(sourcePath, "/"), "/")
		if candidate := strings.TrimSpace(parts[len(parts)-1]); candidate != "" && candidate != "VibeVoice-7B" {
			name = candidate
		}
	}
	body := []byte("joined built-in tts fixture")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	identity := fmt.Sprintf("model|%s|%s:%d:%s", source, name, len(body), digest)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", ".you-content-addressed", "model", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create generic model snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, name), body, 0o644); err != nil {
		t.Fatalf("write generic model snapshot: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "model", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": name, "Bytes": len(body), "SHA256": digest}},
	})
	if err != nil {
		t.Fatalf("marshal generic model metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o644); err != nil {
		t.Fatalf("write generic model metadata: %v", err)
	}
}

func pinnedTTSBackendSelection() serviceedges.ModelBackendArtifactSelection {
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		Bytes:    22,
		SHA256:   "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
	}
}

func writeGenericBuiltinTTSBackendCache(t *testing.T, home string) {
	t.Helper()
	selection := pinnedTTSBackendSelection()
	writeGenericBackendCache(t, home, "localai-vibevoice", selection, []byte("pinned-backend-fixture"))
}

func writeGenericBackendCache(t *testing.T, home, backend string, selection serviceedges.ModelBackendArtifactSelection, body []byte) {
	t.Helper()
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Location)))
	source := "backend://" + backend + "/release://" + urlHash
	digest := selection.SHA256
	identity := fmt.Sprintf("backend|%s|%s:%d:%s", source, selection.Name, selection.Bytes, digest)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", "backend-artifacts", ".you-content-addressed", "backend", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create generic backend snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, selection.Name), body, 0o644); err != nil {
		t.Fatalf("write generic backend snapshot: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "backend", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{
			"Name": selection.Name, "Bytes": selection.Bytes, "SHA256": selection.SHA256,
		}},
	})
	if err != nil {
		t.Fatalf("marshal generic backend metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o644); err != nil {
		t.Fatalf("write generic backend metadata: %v", err)
	}
}

type joinedProtocolNegotiator struct {
	mu    sync.Mutex
	calls int
}

func (negotiator *joinedProtocolNegotiator) Negotiate(
	_ context.Context,
	_ string,
	request serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	negotiator.mu.Lock()
	negotiator.calls++
	negotiator.mu.Unlock()
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: "localai-backend-v1",
		Backend:         request.Backend,
		Ready:           true,
	}, nil
}

func (negotiator *joinedProtocolNegotiator) Calls() int {
	negotiator.mu.Lock()
	defer negotiator.mu.Unlock()
	return negotiator.calls
}

type joinedCompatibilityChecker struct {
	mu      sync.Mutex
	calls   int
	failure error
}

func (checker *joinedCompatibilityChecker) Check(
	context.Context,
	serviceedges.ModelHostCompatibilityRequest,
) error {
	checker.mu.Lock()
	checker.calls++
	failure := checker.failure
	checker.mu.Unlock()
	return failure
}

func (checker *joinedCompatibilityChecker) Calls() int {
	checker.mu.Lock()
	defer checker.mu.Unlock()
	return checker.calls
}

func functionalHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return append(os.Environ(), "USERPROFILE="+home)
	}
	if runtime.GOOS == "plan9" {
		return append(os.Environ(), "home="+home)
	}
	return append(os.Environ(), "HOME="+home)
}

type recordingModelHTTPClient struct {
	mu       sync.Mutex
	calls    int
	delegate *http.Client
}

func (client *recordingModelHTTPClient) Do(request *http.Request) (*http.Response, error) {
	c06Ledger.hostHTTPCalls.Add(1)
	client.mu.Lock()
	client.calls++
	delegate := client.delegate
	client.mu.Unlock()
	if delegate == nil {
		return nil, http.ErrUseLastResponse
	}
	return delegate.Do(request)
}

func (client *recordingModelHTTPClient) Calls() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.calls
}
