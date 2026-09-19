package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestModelsOmniFileInputsPreserveDetectedTypesAndImageOrderThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	const generated = "The fixture sees every exact input in order"
	fixture := buildOmniFileInputFixture(t, generated)
	process := fixture.process
	home := fixture.home
	dir := fixture.dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm",
		"--input", "prompt=@" + fixture.media.promptPath,
		"--input", "image=@" + fixture.media.image.ResolvedPath,
		"--input", "image=@" + fixture.media.image.ResolvedPath,
		"--input", "video=@" + fixture.media.video.ResolvedPath,
	})
	inputs.Input.Env = functionalHomeEnvironment(home)
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models invoke llm --input files) error = %v", err)
	}
	t.Logf("command: you models invoke llm --input prompt=@%s --input image=@%s --input image=@%s --input video=@%s", fixture.media.promptPath, fixture.media.image.ResolvedPath, fixture.media.image.ResolvedPath, fixture.media.video.ResolvedPath)
	t.Logf("stdout:\n%s\n--- end stdout", stdout.String())
	t.Logf("stderr:\n%s\n--- end stderr", stderr.String())
	if stdout.String() != generated {
		t.Fatalf("models invoke stdout = %q, want exact fixture response %q", stdout.String(), generated)
	}
	if stderr.Len() != 0 {
		t.Fatalf("models invoke stderr = %q, want diagnostics-free fixture output", stderr.String())
	}
	wantReads := []string{fixture.media.promptPath, fixture.media.image.ResolvedPath, fixture.media.image.ResolvedPath, fixture.media.video.ResolvedPath}
	request := fixture.protocol.Request()
	t.Logf("protocol request: %s", omniRequestSummary(request))
	if request.Operation != models.OperationOMNI || request.Prompt != string(fixture.media.promptBytes) || len(request.Inputs) != 4 {
		t.Fatalf("protocol request identity = operation=%q prompt=%q inputs=%d, want ordered OMNI prompt/image/image/video request", request.Operation, request.Prompt, len(request.Inputs))
	}
	wantInputs := []struct {
		slot, modality, mediaType string
		content                   []byte
	}{
		{slot: "prompt", modality: string(models.ModalityText), mediaType: "text/plain", content: fixture.media.promptBytes},
		{slot: "image", modality: string(models.ModalityImage), mediaType: fixture.media.image.MediaType, content: fixture.media.imageBytes},
		{slot: "image", modality: string(models.ModalityImage), mediaType: fixture.media.image.MediaType, content: fixture.media.imageBytes},
		{slot: "video", modality: string(models.ModalityVideo), mediaType: fixture.media.video.MediaType, content: fixture.media.videoBytes},
	}
	for index, want := range wantInputs {
		assertExactOmniProtocolInput(t, index, request.Inputs[index], want.slot, models.Modality(want.modality), want.mediaType, want.content)
	}
	if fixture.protocol.Calls() != 1 {
		t.Fatalf("protocol fixture calls = %d, want one codec-backed generation", fixture.protocol.Calls())
	}
	inputReads := fixture.inputReader.Paths()
	if len(inputReads) != len(wantReads) {
		t.Fatalf("input read order = %#v, want %#v", inputReads, wantReads)
	}
	for index, want := range wantReads {
		if inputReads[index] != want {
			t.Fatalf("input read[%d] = %q, want %q", index, inputReads[index], want)
		}
	}
	if fixture.network.Calls() != 0 {
		t.Fatalf("asset network calls = %d, want 0 from content-addressed fixtures", fixture.network.Calls())
	}
	if fixture.media.manifest.SchemaVersion != functionalOmniManifestSchemaV1 {
		t.Fatalf("fixture manifest schema = %q, want pinned OMNI schema", fixture.media.manifest.SchemaVersion)
	}
	closeRootProcess(t, process, "close Omni file-input root process")
}

type omniFileInputFixture struct {
	process     support.Process
	home        string
	dir         string
	media       omniExactMediaFixture
	inputReader *omniExactInputReader
	protocol    *omniTextProtocolFixture
	network     *rejectingModelAssetHTTP
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

	dir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	media := newExactOmniMediaFixture(t, dir)
	fixture := &omniFileInputFixture{
		home: home, dir: dir, media: media, inputReader: media.inputReader,
		protocol: &omniTextProtocolFixture{response: response}, network: &rejectingModelAssetHTTP{},
	}
	assetFiles := functionalModelAssetFileSystem{home: home}
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
		ModelCLIInputReadFile:    fixture.inputReader.Read,
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

// omniExactMediaFixture is the shared local-real input used by the controlled
// root-composition probes. This local test reader validates the same checked-in
// manifest identity and bytes without importing an integration-only package.
const functionalOmniManifestSchemaV1 = "you.localai.omni-media-fixture.v1"

type functionalOmniManifest struct {
	SchemaVersion string                   `json:"schemaVersion"`
	Artifacts     []functionalOmniArtifact `json:"artifacts"`
}

type functionalOmniArtifact struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	MediaType    string `json:"mediaType"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
	ResolvedPath string `json:"-"`
}

type omniExactMediaFixture struct {
	manifest    functionalOmniManifest
	image       functionalOmniArtifact
	video       functionalOmniArtifact
	promptPath  string
	promptBytes []byte
	imageBytes  []byte
	videoBytes  []byte
	inputReader *omniExactInputReader
}

func newExactOmniMediaFixture(t testing.TB, directory string) omniExactMediaFixture {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate OMNI functional test")
	}
	manifestPath := filepath.Join(
		filepath.Dir(sourceFile), "..", "..", "..", "..",
		"tests", "integration", "models", "testdata", "omni_media", "manifest.json",
	)
	manifest, err := loadFunctionalOmniManifest(manifestPath)
	if err != nil {
		t.Fatalf("load exact OMNI media manifest: %v", err)
	}
	if len(manifest.Artifacts) != 2 {
		t.Fatalf("exact OMNI media artifacts = %d, want image and video", len(manifest.Artifacts))
	}
	imageArtifact := manifest.Artifacts[0]
	videoArtifact := manifest.Artifacts[1]
	imageBytes, err := os.ReadFile(imageArtifact.ResolvedPath)
	if err != nil {
		t.Fatalf("read exact OMNI image fixture: %v", err)
	}
	videoBytes, err := os.ReadFile(videoArtifact.ResolvedPath)
	if err != nil {
		t.Fatalf("read exact OMNI video fixture: %v", err)
	}
	promptBytes := []byte("Compare these exact inputs")
	promptPath := filepath.Join(directory, "prompt.txt")
	if err := os.WriteFile(promptPath, promptBytes, 0o644); err != nil {
		t.Fatalf("write exact OMNI prompt fixture: %v", err)
	}
	reader := newOmniExactInputReader(promptPath, imageArtifact.ResolvedPath, videoArtifact.ResolvedPath)
	return omniExactMediaFixture{
		manifest: manifest, image: imageArtifact, video: videoArtifact,
		promptPath: promptPath, promptBytes: promptBytes,
		imageBytes: imageBytes, videoBytes: videoBytes, inputReader: reader,
	}
}

func loadFunctionalOmniManifest(path string) (functionalOmniManifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return functionalOmniManifest{}, err
	}
	var manifest functionalOmniManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return functionalOmniManifest{}, fmt.Errorf("decode OMNI manifest: %w", err)
	}
	if manifest.SchemaVersion != functionalOmniManifestSchemaV1 || len(manifest.Artifacts) != 2 {
		return functionalOmniManifest{}, fmt.Errorf("OMNI manifest schema=%q artifacts=%d", manifest.SchemaVersion, len(manifest.Artifacts))
	}
	root := filepath.Dir(path)
	for index := range manifest.Artifacts {
		artifact := &manifest.Artifacts[index]
		resolved := filepath.Join(root, filepath.Clean(artifact.Path))
		relative, err := filepath.Rel(root, resolved)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return functionalOmniManifest{}, fmt.Errorf("OMNI artifact[%d] escapes fixture root", index)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return functionalOmniManifest{}, fmt.Errorf("read OMNI artifact[%d]: %w", index, err)
		}
		digest := sha256.Sum256(data)
		if int64(len(data)) != artifact.Bytes || hex.EncodeToString(digest[:]) != artifact.SHA256 {
			return functionalOmniManifest{}, fmt.Errorf("OMNI artifact[%d] identity mismatch", index)
		}
		artifact.ResolvedPath = resolved
	}
	return manifest, nil
}

type omniExactInputReader struct {
	mu      sync.Mutex
	allowed map[string]struct{}
	reads   []string
}

func newOmniExactInputReader(paths ...string) *omniExactInputReader {
	allowed := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		allowed[path] = struct{}{}
	}
	return &omniExactInputReader{allowed: allowed}
}

func (reader *omniExactInputReader) Read(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, ok := reader.allowed[path]; !ok {
		return nil, fmt.Errorf("unexpected exact OMNI input path %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("exact OMNI input %q exceeds %d bytes", path, maxBytes)
	}
	reader.mu.Lock()
	reader.reads = append(reader.reads, path)
	reader.mu.Unlock()
	return append([]byte(nil), data...), nil
}

func (reader *omniExactInputReader) Paths() []string {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return append([]string(nil), reader.reads...)
}

func assertExactOmniProtocolInput(
	t testing.TB,
	index int,
	got models.InvocationProtocolInput,
	wantSlot string,
	wantModality models.Modality,
	wantMediaType string,
	wantContent []byte,
) {
	t.Helper()
	gotContent := []byte(got.Content)
	gotHash := omniExactDigest(gotContent)
	wantHash := omniExactDigest(wantContent)
	if got.Slot != wantSlot || got.Modality != wantModality || got.MediaType != wantMediaType || !bytes.Equal(gotContent, wantContent) {
		t.Fatalf("protocol input[%d] identity = slot=%q modality=%q mediaType=%q bytes=%d sha256=%s, want slot=%q modality=%q mediaType=%q bytes=%d sha256=%s", index, got.Slot, got.Modality, got.MediaType, len(gotContent), gotHash, wantSlot, wantModality, wantMediaType, len(wantContent), wantHash)
	}
}

func omniExactDigest(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

func omniRequestSummary(request models.InvocationProtocolRequest) string {
	inputs := make([]string, len(request.Inputs))
	for index, input := range request.Inputs {
		content := []byte(input.Content)
		inputs[index] = fmt.Sprintf("%d:%s/%s/%s/%d/%s", index, input.Slot, input.Modality, input.MediaType, len(content), omniExactDigest(content))
	}
	return fmt.Sprintf("operation=%q prompt=%q inputs=[%s]", request.Operation, request.Prompt, strings.Join(inputs, ", "))
}
