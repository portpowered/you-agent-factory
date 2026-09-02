package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// TestModelsASRDirectCLIEndToEndThroughRootBuildProcess proves the complete
// public path for named ASR outputs: file bytes enter the generic request,
// the injected LocalAI fixture returns transcript and timestamped segments,
// and the CLI publishes both outputs atomically before exposing JSON metadata.
func TestModelsASRDirectCLIEndToEndThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	story := setupASRStory(t)
	runASRMappedInvocation(t, story)
	runASRJSONInvocation(t, story)
}

type asrStory struct {
	process           support.Process
	modelDefinition   models.ModelDefinition
	fixture           *localai.Fixture
	home              string
	dir               string
	inputPath         string
	inputBytes        []byte
	transcriptPath    string
	segmentsPath      string
	wantSegments      string
	received          *models.ASRBackendRequest
	rejectingNetwork  *rejectingModelAssetHTTP
	hostLauncher      *recordingModelHostLauncher
	protocol          *joinedProtocolNegotiator
	compatibility     *joinedCompatibilityChecker
	backendSelections *[]serviceedges.ModelBackendArtifactSelectionRequest
}

func setupASRStory(t *testing.T) asrStory {
	t.Helper()
	fixture := functionalStartLocalAI(t)
	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	modelDefinition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameASR)
	if !ok {
		t.Fatal("built-in catalog did not publish the ASR model definition")
	}
	home := functionalTempDir(t)
	writeGenericBuiltinModelCache(t, home, modelDefinition.Source)
	selection, backendBody := fixtureBackendSelection(modelDefinition.Backend)
	writeGenericBackendCache(t, home, modelDefinition.Backend, selection, backendBody)

	inputBytes := []byte{0x00, 0xff, 0x10, 0x80, 0x7f, 0x01}
	inputPath := filepath.Join(functionalTempDir(t), "meeting.wav")
	if err := os.WriteFile(inputPath, inputBytes, 0o644); err != nil {
		t.Fatalf("write ASR input fixture: %v", err)
	}
	transcriptPath := filepath.Join(functionalTempDir(t), "transcript.txt")
	segmentsPath := filepath.Join(functionalTempDir(t), "segments.json")
	const wantSegments = `[{"id":0,"start":0,"end":1500,"text":"LOCALAI_FIXTURE_SEGMENT"}]`

	received := &models.ASRBackendRequest{}
	asrBackend := func(ctx context.Context, request models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		*received = request
		response, err := fixture.ASRBackend(ctx, request)
		if err != nil {
			return models.ASRBackendResponse{}, err
		}
		artifact, err := (models.InferenceArtifactRef{}).Parse("artifact:segments")
		if err != nil {
			return models.ASRBackendResponse{}, err
		}
		response.Artifacts = []models.InferenceArtifact{{
			Name: "segments", Artifact: artifact, MediaType: "application/json",
			SizeBytes:  int64(len(wantSegments)),
			Properties: map[string]string{"digest": "sha256:fixture-segments"},
		}}
		return response, nil
	}

	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	var backendSelections []serviceedges.ModelBackendArtifactSelectionRequest
	dir := functionalScaffoldFactory(t, asrModelFactoryConfig(modelServer.URL, modelDefinition.Name, modelDefinition.Backend))
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
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(ctx context.Context, request serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			if err := ctx.Err(); err != nil {
				return serviceedges.ModelBackendArtifactSelection{}, err
			}
			backendSelections = append(backendSelections, request)
			return selection, nil
		},
		ModelASRBackend:        asrBackend,
		ModelHostHTTPClient:    modelServer.Client(),
		ModelRuntimeHTTPClient: modelServer.Client(),
	})
	t.Cleanup(func() { closeRootProcess(t, process, "close ASR root process") })
	return asrStory{
		process: process, modelDefinition: modelDefinition, fixture: fixture, home: home, dir: dir, inputPath: inputPath, inputBytes: inputBytes,
		transcriptPath: transcriptPath, segmentsPath: segmentsPath, wantSegments: wantSegments,
		received: received, rejectingNetwork: rejectingNetwork, hostLauncher: hostLauncher,
		protocol: protocol, compatibility: compatibility, backendSelections: &backendSelections,
	}
}

func runASRMappedInvocation(t *testing.T, story asrStory) {
	t.Helper()

	var output, invokeStderr bytes.Buffer
	invoke := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", story.modelDefinition.Name, "--operation", "ASR", "--input", "audio=@" + story.inputPath,
		"--output", "transcript=" + story.transcriptPath, "--output", "segments=" + story.segmentsPath,
	})
	invoke.Input.Env = functionalHomeEnvironment(story.home)
	invoke.Input.WorkingDirectory = story.dir
	invoke.Input.Stdout = &output
	invoke.Input.Stderr = &invokeStderr
	if err := story.process.Execute(invoke.Input); err != nil {
		t.Fatalf("Process.Execute(ASR mapped outputs) error = %v", err)
	}
	transcript, err := os.ReadFile(story.transcriptPath)
	if err != nil {
		t.Fatalf("read transcript output: %v", err)
	}
	if string(transcript) != localai.FixtureTranscript {
		t.Fatalf("transcript output = %q, want %q", transcript, localai.FixtureTranscript)
	}
	segments, err := os.ReadFile(story.segmentsPath)
	if err != nil {
		t.Fatalf("read segments output: %v", err)
	}
	if string(segments) != story.wantSegments {
		t.Fatalf("segments output = %s, want canonical timestamped JSON", segments)
	}
	if string(story.received.Audio) != string(story.inputBytes) || story.received.MediaType != "audio/wav" {
		t.Fatalf("ASR backend request = %#v, want exact bytes and audio/wav", *story.received)
	}
	assertASRFixtureCall(t, story.fixture.Calls(), base64.StdEncoding.EncodeToString(story.inputBytes))
	transcriptDigest := sha256.Sum256(transcript)
	segmentsDigest := sha256.Sum256(segments)
	t.Logf("runtime proof command: you models invoke asr --operation ASR --input audio=@%s --output transcript=%s --output segments=%s", story.inputPath, story.transcriptPath, story.segmentsPath)
	t.Logf("runtime proof exitCode=0 stdout=%q stderr=%q", output.String(), invokeStderr.String())
	t.Logf("runtime proof output transcript mediaType=text/plain bytes=%q size=%d sha256=%s", string(transcript), len(transcript), hex.EncodeToString(transcriptDigest[:]))
	t.Logf("runtime proof output segments mediaType=application/json bytes=%s size=%d sha256=%s", string(segments), len(segments), hex.EncodeToString(segmentsDigest[:]))

	assertASRCacheEffects(t, story)
}

func runASRJSONInvocation(t *testing.T, story asrStory) {
	t.Helper()
	var output bytes.Buffer
	var jsonStderr bytes.Buffer
	jsonInvoke := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", story.modelDefinition.Name, "--operation", "ASR", "--input", "audio=@" + story.inputPath,
	})
	jsonInvoke.Input.Env = functionalHomeEnvironment(story.home)
	jsonInvoke.Input.WorkingDirectory = story.dir
	jsonInvoke.Input.Stdout = &output
	jsonInvoke.Input.Stderr = &jsonStderr
	if err := story.process.Execute(jsonInvoke.Input); err != nil {
		t.Fatalf("Process.Execute(ASR --json) error = %v", err)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode ASR JSON response: %v\n%s", err, output.String())
	}
	if len(response.Outputs) != 2 || response.Outputs[0].Name != "transcript" || response.Outputs[1].Name != "segments" {
		t.Fatalf("ASR JSON outputs = %#v, want transcript then segments", response.Outputs)
	}
	if response.Outputs[0].MediaType == nil || *response.Outputs[0].MediaType != "text/plain" || response.Outputs[1].MediaType == nil || *response.Outputs[1].MediaType != "application/json" {
		t.Fatalf("ASR JSON media metadata = %#v", response.Outputs)
	}
	artifact := response.Outputs[1].Artifact
	if artifact == nil || artifact.ArtifactRef != "artifact:segments" || artifact.SizeBytes == nil || *artifact.SizeBytes != int64(len(story.wantSegments)) || artifact.Properties == nil || (*artifact.Properties)["digest"] != "sha256:fixture-segments" {
		t.Fatalf("ASR JSON artifact metadata = %#v, want opaque ref/size/digest", artifact)
	}
	t.Logf("runtime proof command: you --json models invoke asr --operation ASR --input audio=@%s", story.inputPath)
	t.Logf("runtime proof exitCode=0 stdout=%s stderr=%q", output.String(), jsonStderr.String())
	t.Logf("runtime proof JSON outputs transcript mediaType=text/plain segments mediaType=application/json artifactRef=%s size=%d digest=%s", artifact.ArtifactRef, *artifact.SizeBytes, (*artifact.Properties)["digest"])
}

func assertASRCacheEffects(t *testing.T, story asrStory) {
	t.Helper()
	if story.rejectingNetwork.Calls() != 0 || story.hostLauncher.Calls() == 0 || story.protocol.Calls() == 0 || story.compatibility.Calls() == 0 {
		t.Fatalf("ASR effects = asset network %d, host starts %d, protocol %d, compatibility %d; want cache-backed joined execution", story.rejectingNetwork.Calls(), story.hostLauncher.Calls(), story.protocol.Calls(), story.compatibility.Calls())
	}
	if len(*story.backendSelections) == 0 {
		t.Fatal("built-in ASR invocation did not resolve a backend artifact")
	}
	for _, request := range *story.backendSelections {
		if request.Backend != story.modelDefinition.Backend {
			t.Fatalf("resolved backend request = %#v, want production catalog backend %q", request, story.modelDefinition.Backend)
		}
	}
	t.Logf("production catalog backend resolution: model=%s backend=%s requests=%d", story.modelDefinition.Name, story.modelDefinition.Backend, len(*story.backendSelections))
}

func asrModelFactoryConfig(endpoint, modelName, backend string) map[string]any {
	config := localModelReadinessAssetsHostFactoryConfig(endpoint)
	resources := config["resources"].([]map[string]any)
	resources[0]["name"] = "asr-cache"
	resources[0]["model"] = modelName
	resources[0]["backend"] = backend
	workers := config["workers"].([]map[string]any)
	workers[0]["name"] = "asr-worker"
	workers[0]["model"] = modelName
	workers[0]["command"] = "whisper"
	workers[0]["args"] = []string{"--grpc-endpoint", endpoint}
	workerResources := workers[0]["resources"].([]map[string]any)
	workerResources[0]["name"] = "asr-cache"
	workers[0]["operations"] = []map[string]any{{
		"name": "ASR",
		"inputs": []map[string]any{
			{"name": "audio", "contentTypes": []string{interfaces.ModelOperationContentTypeAudio}, "required": true},
			{"name": "prompt", "contentTypes": []string{interfaces.ModelOperationContentTypeText}},
			{"name": "parameters", "contentTypes": []string{interfaces.ModelOperationContentTypeJSON}},
		},
		"outputs": []map[string]any{
			{"name": "transcript", "contentTypes": []string{interfaces.ModelOperationContentTypeText}},
			{"name": "segments", "contentTypes": []string{interfaces.ModelOperationContentTypeJSON}},
		},
	}}
	return config
}

func pinnedASRBackendSelection() serviceedges.ModelBackendArtifactSelection {
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-whisper-linux-amd64-fixture.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-fixture/localai-backend-localai-whisper-linux-amd64-fixture.tar.gz",
		Bytes:    26,
		SHA256:   "d1481b62fccf94404c3ca599efa30c432d87bdad4bc7493c7e8f82ff84e0e61b",
	}
}

func assertASRFixtureCall(t *testing.T, calls []localai.Call, wantPrompt string) {
	t.Helper()
	for _, call := range calls {
		if call.Method == "AudioTranscription" {
			if call.Prompt != wantPrompt {
				t.Fatalf("ASR fixture prompt = %q, want base64 audio bytes %q", call.Prompt, wantPrompt)
			}
			return
		}
	}
	t.Fatalf("LocalAI fixture calls = %#v, want AudioTranscription", calls)
}
