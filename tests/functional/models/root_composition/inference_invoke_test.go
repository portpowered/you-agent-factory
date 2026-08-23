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
// constructed only through root.BuildProcess executes public Models invoke and
// returns observable inference output while host, runtime, and asset external
// effects are replaced exclusively through published edges.Edges fields.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestModelsInferenceInvokeActivatesThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	audio := []byte("RIFF....WAVE")
	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/health":
			writer.WriteHeader(http.StatusOK)
		case "/invoke":
			var payload struct {
				OutputFile string `json:"outputFile"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			if err := os.WriteFile(payload.OutputFile, audio, 0o644); err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
			writer.WriteHeader(http.StatusOK)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(modelServer.Close)

	cacheDirectory := t.TempDir()
	writeReadyOmniVoiceInvokeCache(t, cacheDirectory)

	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	hostHTTP := &recordingModelHTTPClient{delegate: modelServer.Client()}
	assetFiles := functionalModelAssetFileSystem{home: cacheDirectory}

	dir := support.ScaffoldFactory(t, localModelInferenceInvokeFactoryConfig(modelServer.URL))
	environment := functionalHomeEnvironment(cacheDirectory)
	process := support.BuildProcess(t, serviceedges.Edges{
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
		ModelRuntimeHTTPClient:         modelServer.Client(),
	})

	var output bytes.Buffer
	jsonInvoke := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "hello from root composition invoke",
	})
	jsonInvoke.Input.Env = environment
	jsonInvoke.Input.WorkingDirectory = dir
	jsonInvoke.Input.Stdout = &output
	jsonInvoke.Input.Stderr = io.Discard
	if err := process.Execute(jsonInvoke.Input); err != nil {
		t.Fatalf("Process.Execute(models invoke --json) error = %v", err)
	}

	var response factoryapi.ModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode models invoke output: %v\n%s", err, output.String())
	}
	if response.ModelName != "OMNIVOICE_Q4_K_M" || response.Operation != "TTS" {
		t.Fatalf("models invoke identity = %#v, want OMNIVOICE_Q4_K_M/TTS", response)
	}
	if response.Worker != "tts-worker" || len(response.Content) == 0 {
		t.Fatalf("models invoke response = %#v, want tts-worker content", response)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("asset network calls = %d during invoke, want 0 via edges", rejectingNetwork.Calls())
	}
	if hostLauncher.Calls() == 0 {
		t.Fatal("host process launcher calls = 0 after invoke, want host activation through edges")
	}
	if hostHTTP.Calls() == 0 {
		t.Fatal("host HTTP client calls = 0 after invoke, want inference activation through edges")
	}

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	output.Reset()
	audioInvoke := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "write audio from root composition invoke",
		"--output", audioPath,
	})
	audioInvoke.Input.Env = environment
	audioInvoke.Input.WorkingDirectory = dir
	audioInvoke.Input.Stdout = &output
	audioInvoke.Input.Stderr = io.Discard
	if err := process.Execute(audioInvoke.Input); err != nil {
		t.Fatalf("Process.Execute(models invoke --output) error = %v", err)
	}
	written, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("read models invoke audio: %v", err)
	}
	if !bytes.Equal(written, audio) {
		t.Fatalf("models invoke audio = %q, want %q", written, audio)
	}
}

// TestModelsJoinedBuiltinInvokeWithoutFactoryDeclaration proves the built-in
// tts definition reaches the joined kernel through root.BuildProcess and
// Process.Execute without a redundant Factory resource or worker declaration.
func TestModelsJoinedBuiltinInvokeWithoutFactoryDeclaration(t *testing.T) {
	t.Parallel()

	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	process := support.BuildProcess(t, serviceedges.Edges{
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

	var response factoryapi.ModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode joined models invoke output: %v\n%s", err, output.String())
	}
	if response.ModelName != "tts" || response.Operation != "TTS" || len(response.Content) == 0 {
		t.Fatalf("joined models invoke response = %#v, want tts/TTS content", response)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("joined asset network calls = %d, want 0 from content-addressed cache", rejectingNetwork.Calls())
	}
	if backendResolverCalls != 1 {
		t.Fatalf("joined backend resolver calls = %d, want exactly one built-in managed-backend attempt", backendResolverCalls)
	}

	closer, ok := process.(interface{ Close(context.Context) error })
	if !ok {
		t.Fatal("root process does not expose lifecycle close")
	}
	if err := closer.Close(context.Background()); err != nil {
		t.Fatalf("close joined root process: %v", err)
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

// TestModelsGenericHTTPInvocationReachesJoinedRootThroughProcess proves the
// registered generic HTTP route uses the live Models scope and returns named
// output from the joined root rather than the legacy model-invocation envelope.
func TestModelsGenericHTTPInvocationReachesJoinedRootThroughProcess(t *testing.T) {
	t.Parallel()

	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
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
			Model: factoryapi.ModelReference{NameOrUri: "tts"}, Operation: "TTS", Inputs: &inputs,
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
func TestModelsNamedAndGenericHTTPInvocationShareBuiltinResolution(t *testing.T) {
	t.Parallel()

	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
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
	genericResponse := postFunctionalJSON[factoryapi.GenericModelInvocationResponse](
		t,
		server.URL()+"/models/invocations",
		factoryapi.GenericModelInvocationRequest{
			Scope: "factory-session:parity-generic", Holder: "functional-parity",
			Model: factoryapi.ModelReference{NameOrUri: "tts"}, Operation: "TTS", Inputs: &inputs,
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

func TestModelsGenericCLIOutputModesReachJoinedRootThroughProcess(t *testing.T) {
	t.Parallel()

	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinModelCache(t, home, "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3")
	writeGenericBackendCache(t, home, "localai-llamacpp", serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Bytes:    28,
		SHA256:   "9285e7ffc76aaadf4dfcc6b2de5e23c6b01d4e7068e8f2dd65673626cc5de4ed",
	}, []byte("localai-llamacpp/linux-amd64"))
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	dir := support.ScaffoldFactory(t, multiOutputModelFactoryConfig(modelServer.URL))
	process := support.BuildProcess(t, serviceedges.Edges{
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
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelHostHTTPClient:            modelServer.Client(),
		ModelRuntimeHTTPClient:         modelServer.Client(),
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
	var jsonResponse factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &jsonResponse); err != nil {
		t.Fatalf("decode multi-output JSON = %v\n%s", err, output.String())
	}
	if len(jsonResponse.Outputs) != 2 || jsonResponse.Outputs[0].Name != "text" || jsonResponse.Outputs[1].Name != "usage" {
		t.Fatalf("multi-output JSON = %#v, want text and usage", jsonResponse.Outputs)
	}

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
	if hostLauncher.Calls() != 2 {
		t.Fatalf("mapped output response/effects = %q, starts %d; want metadata and one start per invocation", output.String(), hostLauncher.Calls())
	}
	closeRootProcess(t, process, "close multi-output root process")
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

func TestModelsJoinedInvokeRejectsPinnedBackendBeforeProcessStartThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	process := support.BuildProcess(t, serviceedges.Edges{
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

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "tts", "--operation", "TTS", "--text", "must fail preflight",
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

func localModelInferenceInvokeFactoryConfig(endpoint string) map[string]any {
	return localModelReadinessAssetsHostFactoryConfig(endpoint)
}

func writeReadyOmniVoiceInvokeCache(t *testing.T, home string) {
	t.Helper()
	modelRoot := filepath.Join(home, ".agent-factory", "models", "OMNIVOICE_Q4_K_M")
	revisionDir := filepath.Join(modelRoot, "rev-test")
	if err := os.MkdirAll(revisionDir, 0o755); err != nil {
		t.Fatalf("create model cache fixture: %v", err)
	}
	files := []string{"omnivoice-base-Q4_K_M.gguf", "omnivoice-tokenizer-Q4_K_M.gguf"}
	body := []byte("fixture")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(revisionDir, name), body, 0o644); err != nil {
			t.Fatalf("write model cache fixture %s: %v", name, err)
		}
	}
	metadata := map[string]any{
		"modelName": "OMNIVOICE_Q4_K_M", "revision": "rev-test",
		"files": []map[string]any{
			{"path": files[0], "bytes": len(body), "sha256": digest},
			{"path": files[1], "bytes": len(body), "sha256": digest},
		},
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal model cache metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelRoot, ".managed-cache.json"), data, 0o644); err != nil {
		t.Fatalf("write model cache metadata: %v", err)
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
