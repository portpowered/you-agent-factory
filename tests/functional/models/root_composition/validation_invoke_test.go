package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestModelsJSONInvokeValidatesWithoutStartingJoinedRuntime proves metadata
// mode validates a configured generic model without starting its backend or
// consuming a content-addressed cache.
func TestModelsJSONInvokeValidatesWithoutStartingJoinedRuntime(t *testing.T) {
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
		t.Fatalf("joined models invoke response = %#v, want validation-only metadata", response)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("joined asset network calls = %d, want 0 from content-addressed cache", rejectingNetwork.Calls())
	}
	if hostLauncher.Calls() != 0 {
		t.Fatalf("joined host starts = %d, want 0 for validation-only metadata", hostLauncher.Calls())
	}

	closer, ok := process.(interface{ Close(context.Context) error })
	if !ok {
		t.Fatal("root process does not expose lifecycle close")
	}
	if err := closer.Close(context.Background()); err != nil {
		t.Fatalf("close joined root process: %v", err)
	}
	if protocol.Calls() != 0 || compatibility.Calls() != 0 {
		t.Fatalf("validation-only joined effects = protocol %d compatibility %d, want 0/0", protocol.Calls(), compatibility.Calls())
	}
}
