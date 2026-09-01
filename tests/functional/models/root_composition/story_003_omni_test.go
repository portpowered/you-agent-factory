package root_composition_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestModelsOmniFileInputsPreserveDetectedTypesAndImageOrderThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	const generated = "The fixture sees every input in order"
	fixture := buildOmniFileInputFixture(t, generated)
	process := fixture.process
	home := fixture.home
	dir := fixture.dir

	promptPath := filepath.Join(dir, "prompt.txt")
	imageAPath := filepath.Join(dir, "a.png")
	imageBPath := filepath.Join(dir, "b.png")
	audioPath := filepath.Join(dir, "voice.wav")
	videoPath := filepath.Join(dir, "clip.mp4")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm",
		"--input", "prompt=@" + promptPath,
		"--input", "image=@" + imageAPath,
		"--input", "image=@" + imageBPath,
		"--input", "audio=@" + audioPath,
		"--input", "video=@" + videoPath,
	})
	inputs.Input.Env = functionalHomeEnvironment(home)
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models invoke llm --input files) error = %v", err)
	}
	t.Logf("command: you models invoke llm --input prompt=@%s --input image=@%s --input image=@%s --input audio=@%s --input video=@%s", promptPath, imageAPath, imageBPath, audioPath, videoPath)
	t.Logf("stdout:\n%s\n--- end stdout", stdout.String())
	t.Logf("stderr:\n%s\n--- end stderr", stderr.String())
	if stdout.String() != generated {
		t.Fatalf("models invoke stdout = %q, want exact fixture response %q", stdout.String(), generated)
	}
	if stderr.Len() != 0 {
		t.Fatalf("models invoke stderr = %q, want diagnostics-free fixture output", stderr.String())
	}
	wantReads := []string{promptPath, imageAPath, imageBPath, audioPath, videoPath}
	request := fixture.protocol.Request()
	t.Logf("protocol request: operation=%q prompt=%q inputs=%#v", request.Operation, request.Prompt, request.Inputs)
	if request.Operation != models.OperationOMNI || request.Prompt != "Compare these inputs" || len(request.Inputs) != 5 {
		t.Fatalf("protocol request = %#v, want ordered OMNI file request", request)
	}
	wantInputs := []struct {
		slot, modality, mediaType, content string
	}{
		{slot: "prompt", modality: string(models.ModalityText), mediaType: "text/plain", content: "Compare these inputs"},
		{slot: "image", modality: string(models.ModalityImage), mediaType: "image/png", content: "PNG-A"},
		{slot: "image", modality: string(models.ModalityImage), mediaType: "image/png", content: "PNG-B"},
		{slot: "audio", modality: string(models.ModalityAudio), mediaType: "audio/wav", content: "RIFF-VOICE"},
		{slot: "video", modality: string(models.ModalityVideo), mediaType: "video/mp4", content: "MP4-CLIP"},
	}
	for index, want := range wantInputs {
		got := request.Inputs[index]
		if got.Slot != want.slot || string(got.Modality) != want.modality || got.MediaType != want.mediaType || got.Content != want.content {
			t.Fatalf("protocol input[%d] = %#v, want slot=%q modality=%q media=%q content=%q", index, got, want.slot, want.modality, want.mediaType, want.content)
		}
	}
	if fixture.protocol.Calls() != 1 {
		t.Fatalf("protocol fixture calls = %d, want one codec-backed generation", fixture.protocol.Calls())
	}
	if len(fixture.inputReads) != len(wantReads) {
		t.Fatalf("input read order = %#v, want %#v", fixture.inputReads, wantReads)
	}
	for index, want := range wantReads {
		if fixture.inputReads[index] != want {
			t.Fatalf("input read[%d] = %q, want %q", index, fixture.inputReads[index], want)
		}
	}
	if fixture.network.Calls() != 0 {
		t.Fatalf("asset network calls = %d, want 0 from content-addressed fixtures", fixture.network.Calls())
	}
	closeRootProcess(t, process, "close Omni file-input root process")
}

type omniFileInputFixture struct {
	process    support.Process
	home       string
	dir        string
	inputReads []string
	protocol   *omniTextProtocolFixture
	network    *rejectingModelAssetHTTP
}

func buildOmniFileInputFixture(t *testing.T, response string) *omniFileInputFixture {
	t.Helper()
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
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, []byte("localai-llamacpp/linux-amd64"))

	fixture := &omniFileInputFixture{
		home:     home,
		dir:      functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig()),
		protocol: &omniTextProtocolFixture{response: response},
		network:  &rejectingModelAssetHTTP{},
	}
	assetFiles := functionalModelAssetFileSystem{home: home}
	fileInputs := map[string][]byte{
		filepath.Join(fixture.dir, "prompt.txt"): []byte("Compare these inputs"),
		filepath.Join(fixture.dir, "a.png"):      []byte("PNG-A"),
		filepath.Join(fixture.dir, "b.png"):      []byte("PNG-B"),
		filepath.Join(fixture.dir, "voice.wav"):  []byte("RIFF-VOICE"),
		filepath.Join(fixture.dir, "clip.mp4"):   []byte("MP4-CLIP"),
	}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	fixture.process = functionalBuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient: fixture.network, ModelAssetMakeDirectories: assetFiles.MkdirAll,
		ModelAssetInspectPath: assetFiles.Stat, ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment: func(string) string { return "" }, ModelAssetWriteFile: assetFiles.WriteFile,
		ModelAssetRenamePath: assetFiles.Rename, ModelAssetRemovePath: assetFiles.Remove,
		ModelAssetReadFile: assetFiles.ReadFile, ModelAssetReadDirectory: assetFiles.ReadDir,
		ModelAssetCreateFile: assetFiles.Create, ModelAssetOpenFile: assetFiles.Open,
		ModelCLIInputReadFile: func(_ context.Context, path string, _ int64) ([]byte, error) {
			fixture.inputReads = append(fixture.inputReads, path)
			data, ok := fileInputs[path]
			if !ok {
				return nil, fmt.Errorf("unexpected fixture input path %q", path)
			}
			return append([]byte(nil), data...), nil
		},
		ModelHostProcessLauncher: hostLauncher, ModelHostProtocolNegotiator: protocol,
		ModelHostCompatibilityChecker: compatibility,
		ModelResolveBackendArtifact: func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			return selection, nil
		},
		ModelAssetHostPlatform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelHostHTTPClient:    modelServer.Client(), ModelRuntimeHTTPClient: modelServer.Client(),
		ModelInvocationProtocolClient: fixture.protocol,
	})
	return fixture
}
