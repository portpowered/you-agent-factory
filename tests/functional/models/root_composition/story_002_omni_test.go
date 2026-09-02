package root_composition_test

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestModelsOmniTextInputReachesPinnedCodecThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	const generated = "Moonlit moss beneath the rain\nSoft steps vanish into dawn\nThe quiet wakes"
	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := functionalTempDir(t)
	writeGenericBuiltinModelCache(t, home, "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3")
	selection := serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51feae2f1.tar.gz",
		Bytes:    28,
		SHA256:   "9285e7ffc76aaadf4dfcc6b2de5e23c6b01d4e7068e8f2dd65673626cc5de4ed",
	}
	// Keep the fixture's content-addressed backend identity aligned with the
	// injected selection while avoiding any network or artifact publishing.
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, []byte("localai-llamacpp/linux-amd64"))

	rejectingNetwork := &rejectingModelAssetHTTP{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	fixture := &omniTextProtocolFixture{response: generated}
	dir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := functionalBuildProcess(t, serviceedges.Edges{
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
			return selection, nil
		},
		ModelAssetHostPlatform:        models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationProtocolClient: fixture,
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm", "--input", "prompt=Write a haiku",
	})
	inputs.Input.Env = functionalHomeEnvironment(home)
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models invoke llm --input) error = %v", err)
	}
	t.Log("command: you models invoke llm --input prompt=\"Write a haiku\"")
	t.Logf("stdout:\n%s\n--- end stdout", stdout.String())
	t.Logf("stderr:\n%s\n--- end stderr", stderr.String())
	if stdout.String() != generated {
		t.Fatalf("models invoke stdout = %q, want exact fixture response %q", stdout.String(), generated)
	}
	request := fixture.Request()
	t.Logf("protocol request: operation=%q prompt=%q inputs=%#v", request.Operation, request.Prompt, request.Inputs)
	if request.Operation != models.OperationOMNI || request.Prompt != "Write a haiku" || len(request.Inputs) != 1 {
		t.Fatalf("protocol request = %#v, want OMNI prompt request", request)
	}
	input := request.Inputs[0]
	if input.Slot != "prompt" || input.Modality != models.ModalityText ||
		input.MediaType != "text/plain" || input.Content != "Write a haiku" {
		t.Fatalf("protocol input = %#v, want normalized prompt", input)
	}
	if fixture.Calls() != 1 {
		t.Fatalf("protocol fixture calls = %d, want one codec-backed generation", fixture.Calls())
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("asset network calls = %d, want 0 from content-addressed fixtures", rejectingNetwork.Calls())
	}
	closeRootProcess(t, process, "close Omni root process")
}

type omniTextProtocolFixture struct {
	mu       sync.Mutex
	request  models.InvocationProtocolRequest
	response string
	calls    int
}

func (fixture *omniTextProtocolFixture) Predict(
	ctx context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.InvocationProtocolResponse{}, err
	}
	fixture.mu.Lock()
	fixture.calls++
	fixture.request = request
	response := fixture.response
	fixture.mu.Unlock()
	return models.InvocationProtocolResponse{Text: response}, nil
}

func (fixture *omniTextProtocolFixture) Request() models.InvocationProtocolRequest {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	request := fixture.request
	request.Inputs = append([]models.InvocationProtocolInput(nil), request.Inputs...)
	return request
}

func (fixture *omniTextProtocolFixture) Calls() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.calls
}
