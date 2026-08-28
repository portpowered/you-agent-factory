package root_composition_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

const deliveredASRSegments = `[{"id":0,"start":0,"end":1500,"text":"LOCALAI_FIXTURE_SEGMENT"}]`

// TestDeliveredCLIArtifactReachesProtocolFixture proves that the actual
// cmd/factory artifact can cross the external backend seam. The opt-in
// endpoint is disabled for ordinary production invocations; this test uses a
// JSON bridge that forwards to the pinned LocalAI gRPC fixture.
func TestDeliveredCLIArtifactReachesProtocolFixture(t *testing.T) {
	fixture := characterizationStartLocalAI(t)
	bridge := newDeliveredModelBridge(t, fixture)
	home := characterizationTempDir(t)
	prepareDeliveredModelCaches(t, home)
	binary := buildDeliveredYouBinary(t)
	workDir := characterizationScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	assertDeliveredModelsReady(t, binary, workDir, home, bridge.server.URL)
	inputPath := writeDeliveredASRInput(t)
	transcriptPath := filepath.Join(characterizationTempDir(t), "transcript.txt")
	segmentsPath := filepath.Join(characterizationTempDir(t), "segments.json")
	asr := runDeliveredCLI(t, binary, workDir, home, bridge.server.URL,
		"models", "invoke", "asr", "--operation", "ASR", "--input", "audio=@"+inputPath,
		"--output", "transcript="+transcriptPath, "--output", "segments="+segmentsPath,
	)
	if asr.exitCode != 0 {
		t.Fatalf("delivered ASR exit=%d stdout=%q stderr=%q", asr.exitCode, asr.stdout, asr.stderr)
	}
	t.Logf("delivered ASR command exit=%d stdout=%q stderr=%q", asr.exitCode, asr.stdout, asr.stderr)
	assertDeliveredASROutputs(t, transcriptPath, segmentsPath)
	transcript, _ := os.ReadFile(transcriptPath)
	segments, _ := os.ReadFile(segmentsPath)
	t.Logf("delivered ASR mapped transcript mediaType=text/plain bytes=%q size=%d sha256=%x", transcript, len(transcript), sha256.Sum256(transcript))
	t.Logf("delivered ASR mapped segments mediaType=application/json bytes=%q size=%d sha256=%x", segments, len(segments), sha256.Sum256(segments))

	wantAudio := localai.AudioBytes()
	generic := runDeliveredCLI(t, binary, workDir, home, bridge.server.URL,
		"models", "invoke", "tts", "--operation", "TTS", "--input", "text=hello",
	)
	if generic.exitCode != 0 || !bytes.Equal(generic.stdout, wantAudio) || len(generic.stderr) != 0 {
		t.Fatalf("delivered TTS pipe = exit %d stdout %d bytes stderr=%q; want exact raw audio and empty stderr", generic.exitCode, len(generic.stdout), generic.stderr)
	}
	t.Logf("delivered TTS pipe command exit=%d raw-audio-bytes=%d sha256=%x stderr=%q", generic.exitCode, len(generic.stdout), sha256.Sum256(generic.stdout), generic.stderr)

	aliasPath := filepath.Join(characterizationTempDir(t), "alias.wav")
	alias := runDeliveredCLI(t, binary, workDir, home, bridge.server.URL,
		"models", "invoke", "tts", "--operation", "TTS", "--text", "hello", "--output", aliasPath,
	)
	if alias.exitCode != 0 {
		t.Fatalf("delivered TTS alias exit=%d stdout=%q stderr=%q", alias.exitCode, alias.stdout, alias.stderr)
	}
	aliasAudio, err := os.ReadFile(aliasPath)
	if err != nil || !bytes.Equal(aliasAudio, wantAudio) {
		t.Fatalf("delivered TTS alias audio error=%v bytes=%d; want exact fixture audio %d bytes", err, len(aliasAudio), len(wantAudio))
	}
	if string(alias.stdout) != "Wrote audio: "+aliasPath+"\n" || len(alias.stderr) != 0 {
		t.Fatalf("delivered TTS alias streams stdout=%q stderr=%q", alias.stdout, alias.stderr)
	}
	t.Logf("delivered TTS alias command exit=%d output=%q audio-bytes=%d sha256=%x stderr=%q", alias.exitCode, alias.stdout, len(aliasAudio), sha256.Sum256(aliasAudio), alias.stderr)
	bridge.assertObservedRequests(t, inputPath)
}

func prepareDeliveredModelCaches(t *testing.T, home string) {
	t.Helper()
	writeGenericBuiltinModelCache(t, home, "hf://ggerganov/whisper.cpp/ggml-base.en.bin@5359861c739e955e79d9a303bcbc70fb988958b1")
	writeGenericBackendCache(t, home, "localai-whisper", pinnedASRBackendSelection(), []byte("pinned-asr-backend-fixture"))
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSManagedRuntimeCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
}

func assertDeliveredModelsReady(t *testing.T, binary, workDir, home, endpoint string) {
	t.Helper()
	result := runDeliveredCLI(t, binary, workDir, home, endpoint, "--json", "models", "list")
	if result.exitCode != 0 {
		t.Fatalf("delivered models list exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	t.Logf("delivered models list command exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	var listed factoryapi.ListModelsResponse
	if err := json.Unmarshal(result.stdout, &listed); err != nil {
		t.Fatalf("decode delivered models list: %v\n%s", err, result.stdout)
	}
	tts, ok := findModelSummary(listed.Results, models.BuiltInModelNameTTS)
	if !ok || tts.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY || tts.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("delivered TTS readiness = %#v, want READY/INSTALLED", tts.ManagedRuntime)
	}
}

func writeDeliveredASRInput(t *testing.T) string {
	t.Helper()
	path := filepath.Join(characterizationTempDir(t), "meeting.wav")
	if err := os.WriteFile(path, []byte{0x00, 0xff, 0x10, 0x80, 0x7f, 0x01}, 0o644); err != nil {
		t.Fatalf("write delivered ASR input: %v", err)
	}
	return path
}

func assertDeliveredASROutputs(t *testing.T, transcriptPath, segmentsPath string) {
	t.Helper()
	transcript, transcriptErr := os.ReadFile(transcriptPath)
	segments, segmentsErr := os.ReadFile(segmentsPath)
	if transcriptErr != nil || string(transcript) != localai.FixtureTranscript {
		t.Fatalf("delivered transcript = %q, read error=%v", transcript, transcriptErr)
	}
	if segmentsErr != nil || string(segments) != deliveredASRSegments {
		t.Fatalf("delivered segments = %q, read error=%v", segments, segmentsErr)
	}
}

type deliveredCLIResult struct {
	exitCode int
	stdout   []byte
	stderr   []byte
}

func buildDeliveredYouBinary(t *testing.T) string {
	t.Helper()
	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(characterizationTempDir(t), binaryName)
	command := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", binaryPath, "./cmd/factory")
	command.Dir = testutil.MustRepoRoot(t)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build delivered you binary: %v\n%s", err, output)
	}
	return binaryPath
}

func runDeliveredCLI(t *testing.T, binary, workDir, home, endpoint string, args ...string) deliveredCLIResult {
	t.Helper()
	command := exec.CommandContext(t.Context(), binary, args...)
	command.Dir = workDir
	command.Env = deliveredCLIEnvironment(home, endpoint)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := deliveredCLIResult{stdout: stdout.Bytes(), stderr: stderr.Bytes()}
	if err == nil {
		return result
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("run delivered CLI: %v", err)
	}
	result.exitCode = exitError.ExitCode()
	return result
}

func deliveredCLIEnvironment(home, endpoint string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "HOME=") || strings.HasPrefix(value, "USERPROFILE=") || strings.HasPrefix(value, "YOU_MODELS_BACKEND_ENDPOINT=") {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment, "HOME="+home, "USERPROFILE="+home, "YOU_MODELS_BACKEND_ENDPOINT="+endpoint)
}

type deliveredModelBridge struct {
	fixture  *localai.Fixture
	server   *httptest.Server
	mu       sync.Mutex
	asrAudio []byte
	asrMedia string
	ttsCalls int
}

func newDeliveredModelBridge(t *testing.T, fixture *localai.Fixture) *deliveredModelBridge {
	t.Helper()
	bridge := &deliveredModelBridge{fixture: fixture}
	bridge.server = characterizationNewHTTPServer(t, http.HandlerFunc(bridge.serveHTTP))
	t.Cleanup(bridge.server.Close)
	return bridge
}

func (bridge *deliveredModelBridge) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/invoke":
		bridge.serveGeneric(writer, request)
	case "/asr":
		bridge.serveASR(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

type deliveredGenericRequest struct {
	ModelName string                  `json:"modelName"`
	Operation string                  `json:"operation"`
	Inputs    []deliveredGenericInput `json:"inputs"`
}

type deliveredGenericInput struct {
	Name          string `json:"name"`
	Modality      string `json:"modality"`
	ContentType   string `json:"contentType"`
	MediaType     string `json:"mediaType"`
	ContentBase64 string `json:"contentBase64"`
}

type deliveredGenericResponse struct {
	Outputs []deliveredGenericOutput `json:"outputs"`
}

type deliveredGenericOutput struct {
	Name          string `json:"name"`
	Modality      string `json:"modality"`
	ContentType   string `json:"contentType"`
	MediaType     string `json:"mediaType"`
	ContentBase64 string `json:"contentBase64"`
}

func (bridge *deliveredModelBridge) serveGeneric(writer http.ResponseWriter, request *http.Request) {
	var input deliveredGenericRequest
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	inputs := make([]models.InferenceInput, len(input.Inputs))
	for index, value := range input.Inputs {
		content, err := base64.StdEncoding.DecodeString(value.ContentBase64)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		inputs[index] = models.InferenceInput{Name: value.Name, Modality: models.Modality(value.Modality), ContentType: value.ContentType, MediaType: value.MediaType, Content: string(content)}
	}
	bridge.mu.Lock()
	bridge.ttsCalls++
	bridge.mu.Unlock()
	contents, _, err := bridge.fixture.InvocationBackend(request.Context(), models.InvokeModelRequest{
		Model: models.ModelReference{NameOrURI: input.ModelName}, Operation: input.Operation, Inputs: inputs,
	})
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadGateway)
		return
	}
	outputs := make([]deliveredGenericOutput, len(contents))
	for index, content := range contents {
		outputs[index] = deliveredGenericOutput{Name: content.Name, Modality: string(content.Modality), ContentType: content.ContentType, MediaType: content.MediaType, ContentBase64: base64.StdEncoding.EncodeToString([]byte(content.Content))}
	}
	writeDeliveredJSON(writer, deliveredGenericResponse{Outputs: outputs})
}

type deliveredASRRequest struct {
	AudioBase64 string         `json:"audioBase64"`
	MediaType   string         `json:"mediaType"`
	Prompt      string         `json:"prompt,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type deliveredASRResponse struct {
	Text      string                     `json:"text"`
	Segments  []models.ASRBackendSegment `json:"segments"`
	Artifacts []deliveredArtifact        `json:"artifacts"`
}

type deliveredArtifact struct {
	Ref        string            `json:"ref"`
	Name       string            `json:"name"`
	MediaType  string            `json:"mediaType"`
	SizeBytes  int64             `json:"sizeBytes"`
	Properties map[string]string `json:"properties,omitempty"`
}

func (bridge *deliveredModelBridge) serveASR(writer http.ResponseWriter, request *http.Request) {
	var input deliveredASRRequest
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	audio, err := base64.StdEncoding.DecodeString(input.AudioBase64)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	bridge.mu.Lock()
	bridge.asrAudio = append([]byte(nil), audio...)
	bridge.asrMedia = input.MediaType
	bridge.mu.Unlock()
	response, err := bridge.fixture.ASRBackend(request.Context(), models.ASRBackendRequest{Audio: audio, MediaType: input.MediaType, Prompt: input.Prompt, Parameters: input.Parameters})
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadGateway)
		return
	}
	writeDeliveredJSON(writer, deliveredASRResponse{
		Text: response.Text, Segments: response.Segments,
		Artifacts: []deliveredArtifact{{Ref: "artifact:segments", Name: "segments", MediaType: "application/json", SizeBytes: int64(len(deliveredASRSegments)), Properties: map[string]string{"digest": "sha256:fixture-segments"}}},
	})
}

func writeDeliveredJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func (bridge *deliveredModelBridge) assertObservedRequests(t *testing.T, inputPath string) {
	t.Helper()
	input, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	bridge.mu.Lock()
	audio := append([]byte(nil), bridge.asrAudio...)
	media := bridge.asrMedia
	ttsCalls := bridge.ttsCalls
	bridge.mu.Unlock()
	if !bytes.Equal(audio, input) || media != "audio/wav" || ttsCalls < 2 {
		t.Fatalf("fixture bridge observations audio=%v media=%q ttsCalls=%d; want exact ASR bytes/audio/wav and both TTS journeys", audio, media, ttsCalls)
	}
	t.Logf("fixture bridge observed exact ASR audio=%x mediaType=%q ttsCalls=%d", audio, media, ttsCalls)
}
