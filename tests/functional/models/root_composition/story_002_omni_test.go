package root_composition_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// The direct Models command keeps the video's visual input and its decoded
// audio observation in one answer. Decoder and model processes are controlled
// external effects; the production media codec and runtime wiring remain live.
func TestModelsVideoEmbeddedAudioAndExplicitAudioReachLLM(t *testing.T) {
	t.Parallel()
	home := functionalTempDir(t)
	backend := functionalStartLocalAI(t)
	writeGenericConformanceCaches(t, home)
	source := filepath.Join(home, "llm-source")
	writeVideoReadinessModelSource(t, source, true)
	writeVideoReadinessOperatorConfig(t, home, source)
	writeVideoReadinessManagedCache(t, home, true)
	edges, _, _, _ := localAIConformanceEdges(home, backend)
	edges.ModelInvocationProtocolClient = nil
	edges.ModelInvocationBackend = nil
	edges.ModelHostCompatibilityChecker = nil
	audio := localai.AudioBytes()
	edges.ModelASRBackend = func(ctx context.Context, request models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		if !bytes.Equal(request.Audio, audio) {
			return models.ASRBackendResponse{}, fmt.Errorf("ASR received different audio")
		}
		return models.ASRBackendResponse{Text: localai.FixtureTranscript,
			Segments: []models.ASRBackendSegment{{ID: 0, Start: 0, End: 19, Text: localai.FixtureTranscriptSegment}}}, ctx.Err()
	}
	runner := &omniVideoDecoder{audio: audio}
	edges.ModelRuntimeCommandRunner = runner
	process := functionalBuildProcess(t, edges)
	dir := functionalTempDir(t)
	video := append([]byte{0, 0, 0, 24}, []byte("ftypmp42controlled-video-with-audio")...)
	videoPath := filepath.Join(dir, "clip.mp4")
	audioPath := filepath.Join(dir, "voice.wav")
	for path, data := range map[string][]byte{videoPath: video, audioPath: audio} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	execute := func(args ...string) string {
		t.Helper()
		input := support.FakeInputs(t.Context(), append([]string{"you", "models", "invoke"}, args...))
		input.Input.Env = videoReadinessEnvironment(home)
		input.Input.WorkingDirectory = dir
		var stdout, stderr bytes.Buffer
		input.Input.Stdout, input.Input.Stderr = &stdout, &stderr
		if err := process.Execute(input.Input); err != nil {
			t.Fatalf("Models invocation %v: %v; stderr=%s", args, err, stderr.String())
		}
		return stdout.String()
	}
	execute("llm", "--input", "prompt=Prepare controlled projector")
	for _, explicitAudio := range []bool{false, true} {
		args := []string{"llm", "--input", "prompt=Describe what is seen and heard", "--input", "video=@" + videoPath}
		if explicitAudio {
			args = append(args, "--input", "audio=@"+audioPath)
		}
		before := len(backend.Calls())
		output := execute(args...)
		assertOmniAudioVideoObservations(t, backend.Calls()[before:], output, explicitAudio, audio, video)
	}
	if runner.probes != 2 || runner.decodes != 2 || !bytes.Equal(runner.video, video) {
		t.Fatalf("decoded clip probes=%d decodes=%d input=%q", runner.probes, runner.decodes, runner.video)
	}
	runner.silent = true
	before := len(backend.Calls())
	output := execute("llm", "--input", "prompt=Describe the silent clip", "--input", "video=@"+videoPath)
	var predictions []localai.Call
	for _, call := range backend.Calls()[before:] {
		if call.Method == "Predict" {
			predictions = append(predictions, call)
		}
	}
	if len(predictions) != 1 || len(predictions[0].Audios) != 0 ||
		output != localai.ExpectedOmniText("Describe the silent clip", nil, nil, predictions[0].Videos) || runner.decodes != 2 {
		t.Fatalf("silent clip answer=%s calls=%#v decodes=%d, want visual observation without invented audio", output, predictions, runner.decodes)
	}
	transcriptPath := filepath.Join(dir, "transcript.txt")
	execute("asr", "--input", "audio=@"+audioPath,
		"--output-map", "transcript="+transcriptPath, "--output-map", "segments="+filepath.Join(dir, "segments.json"))
	transcript, err := os.ReadFile(transcriptPath)
	if err != nil || !strings.Contains(string(transcript), localai.FixtureTranscript) {
		t.Fatalf("ASR transcript=%s, want fixture spoken words", transcript)
	}
	var embeddings []float64
	if err := json.Unmarshal([]byte(execute("embed", "--input", "text=Find similar audio")), &embeddings); err != nil || len(embeddings) == 0 {
		t.Fatalf("embedding output=%v, error=%v", embeddings, err)
	}
	closeRootProcess(t, process, "close media Models process")
}

func assertOmniAudioVideoObservations(t *testing.T, observations []localai.Call, output string, explicitAudio bool, audio, video []byte) {
	t.Helper()
	var calls []localai.Call
	for _, call := range observations {
		if call.Method == "Predict" {
			calls = append(calls, call)
		}
	}
	wantPasses := 2
	if explicitAudio {
		wantPasses = 3
	}
	if len(calls) != wantPasses {
		t.Fatalf("explicit audio=%t: backend calls=%#v, want %d observations", explicitAudio, calls, wantPasses)
	}
	for _, call := range calls[:len(calls)-1] {
		if len(call.Audios) != 1 || len(call.Videos) != 0 || call.Audios[0] != base64.StdEncoding.EncodeToString(audio) {
			t.Fatalf("audio observation = %#v, want exact decoded WAV without visual inputs", call)
		}
		if !strings.Contains(output, localai.ExpectedOmniText(call.Prompt, nil, call.Audios, nil)) {
			t.Fatalf("combined answer lost audible observation: %s", output)
		}
	}
	visual := calls[len(calls)-1]
	if len(visual.Videos) != 1 || len(visual.Audios) != 0 || visual.Videos[0] != base64.StdEncoding.EncodeToString(video) {
		t.Fatalf("visual observation = %#v, want original clip without audio inputs", visual)
	}
	if !strings.Contains(output, "Embedded audio from video input 1:") ||
		!strings.Contains(output, "Video: "+localai.ExpectedOmniText(visual.Prompt, nil, nil, visual.Videos)) ||
		(explicitAudio && !strings.Contains(output, "Audio input 2:")) {
		t.Fatalf("combined answer lost labeled sound or visual observation: %s", output)
	}
}

type omniVideoDecoder struct {
	audio, video    []byte
	probes, decodes int
	silent          bool
}

func (runner *omniVideoDecoder) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	runner.video = append([]byte(nil), request.Stdin...)
	switch request.Command {
	case "ffprobe":
		runner.probes++
		if runner.silent {
			return platformprocess.CommandResult{Stdout: []byte(`{"streams":[{"codec_type":"video"}],"format":{"duration":"1"}}`)}, nil
		}
		return platformprocess.CommandResult{Stdout: []byte(`{"streams":[{"codec_type":"video"},{"codec_type":"audio"}],"format":{"duration":"1"}}`)}, nil
	case "ffmpeg":
		runner.decodes++
		return platformprocess.CommandResult{Stdout: runner.audio}, nil
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected media process %q", request.Command)
	}
}

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
	mu                     sync.Mutex
	request                models.InvocationProtocolRequest
	response               string
	calls                  int
	preparationCallPending bool
}

func (fixture *omniTextProtocolFixture) Predict(
	ctx context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.InvocationProtocolResponse{}, err
	}
	fixture.mu.Lock()
	if fixture.preparationCallPending {
		fixture.preparationCallPending = false
		fixture.mu.Unlock()
		return models.InvocationProtocolResponse{Text: "controlled fixture cache prepared"}, nil
	}
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

// A bad backend publication must fail truthfully and be retried on the next pull.
func TestModelsPullRetriesInvalidBackendPublication(t *testing.T) {
	t.Parallel()
	home := functionalTempDir(t)
	network := &invalidBackendPublication{}
	process := functionalBuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient:           network,
		ModelAssetResolveHomeDirectory: func() (string, error) { return home, nil },
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", Accelerator: "cuda", CUDAAvailable: true},
	})
	defer closeRootProcess(t, process, "close publication Models process")
	for attempt := 0; attempt < 2; attempt++ {
		inputs := support.FakeInputs(t.Context(), []string{"you", "models", "pull", models.BuiltInModelNameASR})
		inputs.Input.Env = functionalHomeEnvironment(home)
		inputs.Input.WorkingDirectory = home
		var output, diagnostic bytes.Buffer
		inputs.Input.Stdout, inputs.Input.Stderr = &output, &diagnostic
		err := process.Execute(inputs.Input)
		if err == nil || strings.Contains(output.String(), `"readinessState":"READY"`) {
			t.Fatalf("invalid publication pull = %v, stdout=%s stderr=%s", err, output.String(), diagnostic.String())
		}
	}
	network.mu.Lock()
	defer network.mu.Unlock()
	if network.indexReads != 2 || network.manifestReads != 2 {
		t.Fatalf("publication reads index=%d manifest=%d, want retry for each pull", network.indexReads, network.manifestReads)
	}
}

type invalidBackendPublication struct {
	mu                        sync.Mutex
	indexReads, manifestReads int
}

func (client *invalidBackendPublication) Do(request *http.Request) (*http.Response, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	tag := "localai-backends-v1-" + strings.Repeat("a", 64)
	body := ""
	switch {
	case request.URL.Host == "api.github.com" && request.URL.Path == "/repos/portpowered/you-agent-factory/releases":
		client.indexReads++
		body = `[{"tag_name":"` + tag + `","assets":[{"name":"manifest.json"}]}]`
	case request.URL.Host == "github.com" && strings.HasSuffix(request.URL.Path, "/"+tag+"/manifest.json"):
		client.manifestReads++
		body = `{"publication":{"releaseTag":"untrusted-release"}}`
	default:
		return nil, fmt.Errorf("controlled unavailable backend: %s", request.URL)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
}
