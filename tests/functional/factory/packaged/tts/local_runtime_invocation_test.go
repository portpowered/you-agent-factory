package tts

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedTTSLocalRuntimePayloadPreservesExactBoundText proves the
// installed local TTS route carries the complete customer value through the
// operation-binding boundary and into Models' joined built-in invocation.
func TestPackagedTTSLocalRuntimePayloadPreservesExactBoundText(t *testing.T) {
	text := "The release is ready, with every submitted word preserved exactly."
	homeDir := t.TempDir()
	support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	cacheDir := t.TempDir()
	writePackagedTTSReadyModelCache(t, cacheDir)
	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)
	backend := newPackagedTTSModelsBackend([]byte(packagedTTSFakeAudioFixture))
	launcher := &packagedTTSModelHostLauncher{endpoint: modelServer.URL}
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--named", factorydefinitions.PackagedTTSFactoryName,
		"--no-record",
		"--output", "primary",
		"--to", text,
	})
	inputs.Input.Env = append(
		os.Environ(),
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		run.ModelCacheDirEnvironment+"="+cacheDir,
	)
	inputs.Input.WorkingDirectory = t.TempDir()

	process := support.BuildProcess(t, serviceedges.Edges{
		ModelAssetHostPlatform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(
			context.Context,
			serviceedges.ModelBackendArtifactSelectionRequest,
		) (serviceedges.ModelBackendArtifactSelection, error) {
			return packagedTTSPinnedBackendSelection(), nil
		},
		ModelHostProcessLauncher:      launcher,
		ModelHostProtocolNegotiator:   packagedTTSHostProtocolNegotiator{},
		ModelHostCompatibilityChecker: packagedTTSHostCompatibilityChecker{},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationBackend:        backend.Invoke,
	})
	t.Cleanup(func() {
		if launcher.StartCount() != 1 || launcher.StopCount() != 1 {
			t.Errorf("local model host lifecycle = starts %d, stops %d; want one start and one cleanup stop", launcher.StartCount(), launcher.StopCount())
		}
	})
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(packaged local TTS) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if response.RequestId == "" || response.TraceId == "" {
		t.Fatalf("local TTS invocation identity = request %q trace %q, want non-empty values", response.RequestId, response.TraceId)
	}
	if backend.CallCount() != 1 {
		t.Fatalf("Models TTS invocation count = %d, want one local-runtime attempt", backend.CallCount())
	}

	request := backend.LastRequest(t)
	if request.Operation != models.OperationTTS || request.Model.NameOrURI != factorydefinitions.DefaultTTSModelName {
		t.Fatalf("joined TTS request identity = %#v, want TTS/%s", request, factorydefinitions.DefaultTTSModelName)
	}
	if len(request.Inputs) != 1 || request.Inputs[0].Name != "text" || request.Inputs[0].Content != text {
		t.Fatalf("joined TTS text input = %#v, want one exact text input %q", request.Inputs, text)
	}
	audio := packagedTTSPrimaryAudio(t, response.PrimaryResult)
	if string(audio) != packagedTTSFakeAudioFixture {
		t.Fatalf("joined audio Work = %q; want fixture", audio)
	}
}

type packagedTTSModelsBackend struct {
	mu        sync.Mutex
	audio     []byte
	request   *models.InvokeModelRequest
	artifacts []models.InferenceArtifact
	calls     int
	failure   error
}

func newPackagedTTSModelsBackend(audio []byte) *packagedTTSModelsBackend {
	return &packagedTTSModelsBackend{audio: append([]byte(nil), audio...)}
}

func (backend *packagedTTSModelsBackend) Invoke(
	_ context.Context,
	request models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	backend.mu.Lock()
	backend.calls++
	failure := backend.failure
	cloned := request
	cloned.Inputs = append([]models.InferenceInput(nil), request.Inputs...)
	backend.request = &cloned
	audio := append([]byte(nil), backend.audio...)
	artifacts := make([]models.InferenceArtifact, len(backend.artifacts))
	for index, artifact := range backend.artifacts {
		artifacts[index] = artifact.Clone()
	}
	backend.mu.Unlock()
	if failure != nil {
		return nil, nil, failure
	}
	return []models.InferenceContent{{
		Name: "audio", Modality: models.ModalityAudio,
		ContentType: "audio/wav", MediaType: "audio/wav", Content: string(audio),
	}}, artifacts, nil
}

func (backend *packagedTTSModelsBackend) CallCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.calls
}

func (backend *packagedTTSModelsBackend) SetFailure(failure error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.failure = failure
}

func (backend *packagedTTSModelsBackend) SetArtifacts(artifacts []models.InferenceArtifact) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.artifacts = make([]models.InferenceArtifact, len(artifacts))
	for index, artifact := range artifacts {
		backend.artifacts[index] = artifact.Clone()
	}
}

func (backend *packagedTTSModelsBackend) LastRequest(t testing.TB) models.InvokeModelRequest {
	t.Helper()
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.request == nil {
		t.Fatal("Models TTS invocation backend was not called")
	}
	request := *backend.request
	request.Inputs = append([]models.InferenceInput(nil), backend.request.Inputs...)
	return request
}

type packagedTTSModelHostLauncher struct {
	mu       sync.Mutex
	endpoint string
	starts   int
	stops    int
}

func (launcher *packagedTTSModelHostLauncher) Start(
	context.Context,
	serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.mu.Lock()
	launcher.starts++
	launcher.mu.Unlock()
	return &packagedTTSModelHostProcess{
		endpoint: launcher.endpoint,
		stopped:  make(chan struct{}),
		onStop:   launcher.recordStop,
	}, nil
}

type packagedTTSModelHostProcess struct {
	endpoint string
	stopped  chan struct{}
	once     sync.Once
	onStop   func()
}

func (process *packagedTTSModelHostProcess) HealthEndpoint() string { return process.endpoint }
func (process *packagedTTSModelHostProcess) Wait() error {
	<-process.stopped
	return nil
}
func (process *packagedTTSModelHostProcess) Stop(context.Context) error {
	process.once.Do(func() {
		close(process.stopped)
		if process.onStop != nil {
			process.onStop()
		}
	})
	return nil
}

func (launcher *packagedTTSModelHostLauncher) recordStop() {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	launcher.stops++
}

func (launcher *packagedTTSModelHostLauncher) StartCount() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.starts
}

func (launcher *packagedTTSModelHostLauncher) StopCount() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.stops
}

type packagedTTSHostProtocolNegotiator struct{}

func (packagedTTSHostProtocolNegotiator) Negotiate(
	context.Context,
	string,
	serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: "localai-backend-v1",
		Backend:         "localai-vibevoice",
		Ready:           true,
	}, nil
}

type packagedTTSHostCompatibilityChecker struct{}

func (packagedTTSHostCompatibilityChecker) Check(
	context.Context,
	serviceedges.ModelHostCompatibilityRequest,
) error {
	return nil
}

func writePackagedTTSReadyModelCache(t testing.TB, cacheDir string) {
	t.Helper()
	const source = "hf://vibevoice/VibeVoice-7B@505114ae6ad17be74df98e6939707434ec49c187"
	const modelAssetName = "weights.bin"
	modelBody := []byte("joined built-in tts fixture")
	modelDigest := fmt.Sprintf("%x", sha256.Sum256(modelBody))
	modelIdentity := fmt.Sprintf("model|%s|%s:%d:%s", source, modelAssetName, len(modelBody), modelDigest)
	modelIdentityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(modelIdentity)))
	modelSnapshot := filepath.Join(cacheDir, ".you-content-addressed", "model", modelIdentityHash)
	if err := os.MkdirAll(modelSnapshot, 0o755); err != nil {
		t.Fatalf("create packaged TTS generic model snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelSnapshot, modelAssetName), modelBody, 0o644); err != nil {
		t.Fatalf("write packaged TTS generic model asset: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "model", "identity": modelIdentity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": modelAssetName, "Bytes": len(modelBody), "SHA256": modelDigest}},
	})
	if err != nil {
		t.Fatalf("marshal packaged TTS generic model metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelSnapshot, ".you-assets.json"), metadata, 0o644); err != nil {
		t.Fatalf("write packaged TTS generic model metadata: %v", err)
	}

	const backend = "localai-vibevoice"
	selection := packagedTTSPinnedBackendSelection()
	backendURLHash := fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Location)))
	backendSource := "backend://" + backend + "/release://" + backendURLHash
	backendIdentity := fmt.Sprintf("backend|%s|%s:%d:%s", backendSource, selection.Name, selection.Bytes, selection.SHA256)
	backendIdentityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(backendIdentity)))
	backendSnapshot := filepath.Join(cacheDir, "backend-artifacts", ".you-content-addressed", "backend", backendIdentityHash)
	if err := os.MkdirAll(backendSnapshot, 0o755); err != nil {
		t.Fatalf("create packaged TTS generic backend snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backendSnapshot, selection.Name), []byte("pinned-backend-fixture"), 0o644); err != nil {
		t.Fatalf("write packaged TTS generic backend asset: %v", err)
	}
	backendMetadata, err := json.Marshal(map[string]any{
		"kind": "backend", "identity": backendIdentity, "source": backendSource, "sourceKey": backendSource,
		"artifacts": []map[string]any{{"Name": selection.Name, "Bytes": selection.Bytes, "SHA256": selection.SHA256}},
	})
	if err != nil {
		t.Fatalf("marshal packaged TTS generic backend metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backendSnapshot, ".you-assets.json"), backendMetadata, 0o644); err != nil {
		t.Fatalf("write packaged TTS generic backend metadata: %v", err)
	}
}

func packagedTTSPinnedBackendSelection() serviceedges.ModelBackendArtifactSelection {
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		Bytes:    22,
		SHA256:   "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
	}
}

func packagedTTSPrimaryAudio(
	t testing.TB,
	primaryResult *factoryapi.WorkContent,
) []byte {
	t.Helper()
	if primaryResult == nil || len(*primaryResult) == 0 {
		t.Fatal("primary result is empty, want one audio Work part")
	}
	for _, part := range *primaryResult {
		if audioPart, err := part.AsWorkAudioContentPart(); err == nil {
			const prefix = "data:audio/wav;base64,"
			if !strings.HasPrefix(audioPart.Url, prefix) {
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(audioPart.Url, prefix))
			if err != nil {
				t.Fatalf("decode joined audio Work URL: %v", err)
			}
			return decoded
		}
		textPart, err := part.AsWorkTextContentPart()
		if err != nil {
			continue
		}
		const prefix = "data:audio/wav;base64,"
		if !strings.HasPrefix(textPart.Text, prefix) {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(textPart.Text, prefix))
		if err != nil {
			t.Fatalf("decode joined audio data URL: %v", err)
		}
		return decoded
	}
	t.Fatalf("primary result = %#v, want one audio/wav data URL Work part", primaryResult)
	return nil
}
