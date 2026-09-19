package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

const (
	asrStoryFixtureDirectory    = "tests/fixtures/localai/asr"
	asrStoryFixtureManifestFile = "manifest.json"
	asrStoryFixtureSchema       = "localai.asr-semantic-fixture.v1"
	asrStoryFixtureFile         = "localai-asr-known.wav"
	asrStoryFixtureBytes        = 10340
	asrStoryFixtureSHA256       = "eea86018ce1730baaf7f5dd6ec88c1f727dd90203521a9115b489310a248ea05"
	asrStoryFixtureTranscript   = "zero"
)

type asrStoryFixtureManifest struct {
	Schema     string                        `json:"schema"`
	File       string                        `json:"file"`
	Bytes      int64                         `json:"bytes"`
	SHA256     string                        `json:"sha256"`
	MediaType  string                        `json:"mediaType"`
	WAV        asrStoryFixtureWAV            `json:"wav"`
	Transcript asrStoryFixtureTranscriptData `json:"transcript"`
	Segments   asrStoryFixtureSegments       `json:"segments"`
	Source     asrStoryFixtureSource         `json:"source"`
}

type asrStoryFixtureWAV struct {
	DurationMillis float64 `json:"durationMillis"`
}

type asrStoryFixtureTranscriptData struct {
	Language      string `json:"language"`
	Raw           string `json:"raw"`
	Normalized    string `json:"normalized"`
	Normalization string `json:"normalization"`
}

type asrStoryFixtureSegments struct {
	TimeUnit    string                       `json:"timeUnit"`
	Items       []asrStoryFixtureSegment     `json:"items"`
	Constraints asrStoryFixtureSegmentLimits `json:"constraints"`
}

type asrStoryFixtureSegment struct {
	ID             int     `json:"id"`
	Start          float64 `json:"start"`
	End            float64 `json:"end"`
	Text           string  `json:"text"`
	NormalizedText string  `json:"normalizedText"`
}

type asrStoryFixtureSegmentLimits struct {
	Finite       bool    `json:"finite"`
	Monotonic    bool    `json:"monotonic"`
	MinimumStart float64 `json:"minimumStart"`
	MaximumEnd   float64 `json:"maximumEnd"`
}

type asrStoryFixtureSource struct {
	Revision string `json:"revision"`
	Path     string `json:"path"`
}

type asrStoryOutputSegment struct {
	ID    int32  `json:"id"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Text  string `json:"text"`
}

// TestModelsASRDirectCLIEndToEndThroughRootBuildProcess proves the complete
// public path for named ASR outputs: the immutable repository fixture enters
// the generic request, the controlled LocalAI boundary returns the manifest's
// transcript and timestamped segment semantics, and direct CLI, HTTP, and
// explicit-server routes publish equivalent results and metadata.
func TestModelsASRDirectCLIEndToEndThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	story := setupASRStory(t)
	runASRMappedInvocation(t, story)
	directJSON := runASRJSONInvocation(t, story)
	runASRRouteParity(t, story, directJSON)
}

type asrStory struct {
	process            support.Process
	modelDefinition    models.ModelDefinition
	manifest           asrStoryFixtureManifest
	fixture            *localai.Fixture
	home               string
	dir                string
	inputPath          string
	inputBytes         []byte
	transcriptPath     string
	segmentsPath       string
	wantSegments       string
	wantSegmentsDigest string
	received           *models.ASRBackendRequest
	rejectingNetwork   *rejectingModelAssetHTTP
	hostLauncher       *recordingModelHostLauncher
	protocol           *joinedProtocolNegotiator
	compatibility      *joinedCompatibilityChecker
	backendSelections  *[]serviceedges.ModelBackendArtifactSelectionRequest
	modelServerURL     string
	modelServerClient  asrStoryHTTPClient
	asrBackend         serviceedges.ModelASRBackend
}

func setupASRStory(t *testing.T) asrStory {
	t.Helper()
	manifest, inputPath, inputBytes, wantSegments, wantSegmentsDigest := loadASRStoryFixture(t)
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

	transcriptPath := filepath.Join(functionalTempDir(t), "transcript.txt")
	segmentsPath := filepath.Join(functionalTempDir(t), "segments.json")

	received := &models.ASRBackendRequest{}
	manifestSegments := asrStoryBackendSegments(t, manifest)
	asrBackend := func(ctx context.Context, request models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		*received = cloneASRStoryBackendRequest(request)
		response, err := fixture.ASRBackend(ctx, request)
		if err != nil {
			return models.ASRBackendResponse{}, err
		}
		response.Text = manifest.Transcript.Normalized
		response.Segments = append([]models.ASRBackendSegment(nil), manifestSegments...)
		artifact, err := (models.InferenceArtifactRef{}).Parse("artifact:segments")
		if err != nil {
			return models.ASRBackendResponse{}, err
		}
		response.Artifacts = []models.InferenceArtifact{{
			Name: "segments", Artifact: artifact, MediaType: "application/json",
			SizeBytes:  int64(len(wantSegments)),
			Properties: map[string]string{"digest": wantSegmentsDigest},
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
		process: process, modelDefinition: modelDefinition, manifest: manifest, fixture: fixture, home: home, dir: dir, inputPath: inputPath, inputBytes: inputBytes,
		transcriptPath: transcriptPath, segmentsPath: segmentsPath, wantSegments: wantSegments, wantSegmentsDigest: wantSegmentsDigest,
		received: received, rejectingNetwork: rejectingNetwork, hostLauncher: hostLauncher,
		protocol: protocol, compatibility: compatibility, backendSelections: &backendSelections,
		modelServerURL: modelServer.URL, modelServerClient: modelServer.Client(), asrBackend: asrBackend,
	}
}

func loadASRStoryFixture(t *testing.T) (asrStoryFixtureManifest, string, []byte, string, string) {
	t.Helper()
	manifestPath := testutil.MustRepoPath(t, filepath.Join(asrStoryFixtureDirectory, asrStoryFixtureManifestFile))
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read ASR semantic fixture manifest: %v", err)
	}
	var manifest asrStoryFixtureManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("decode ASR semantic fixture manifest: %v", err)
	}
	if manifest.Schema != asrStoryFixtureSchema || manifest.File != asrStoryFixtureFile ||
		manifest.Bytes != asrStoryFixtureBytes || manifest.SHA256 != asrStoryFixtureSHA256 ||
		manifest.MediaType != "audio/wav" || manifest.Transcript.Normalized != asrStoryFixtureTranscript {
		t.Fatalf("ASR semantic fixture identity = %#v, want schema/file/size/digest/media/transcript contract", manifest)
	}
	if manifest.WAV.DurationMillis != 643.5 || manifest.Transcript.Raw != "Zero." ||
		manifest.Segments.TimeUnit != "milliseconds" || !manifest.Segments.Constraints.Finite ||
		!manifest.Segments.Constraints.Monotonic || manifest.Segments.Constraints.MaximumEnd != 643.5 ||
		manifest.Source.Revision == "" || manifest.Source.Path == "" {
		t.Fatalf("ASR semantic fixture manifest omitted required duration, transcript, segment, or provenance facts: %#v", manifest)
	}
	if len(manifest.Segments.Items) != 1 {
		t.Fatalf("ASR semantic fixture segments = %d, want one canonical segment", len(manifest.Segments.Items))
	}
	segment := manifest.Segments.Items[0]
	if segment.ID != 0 || segment.Start != 0 || segment.End != 500 || segment.Text != " Zero." || segment.NormalizedText != asrStoryFixtureTranscript {
		t.Fatalf("ASR semantic fixture segment = %#v, want canonical zero segment", segment)
	}

	inputPath := filepath.Join(filepath.Dir(manifestPath), manifest.File)
	inputBytes, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read canonical ASR fixture: %v", err)
	}
	digest := sha256.Sum256(inputBytes)
	if len(inputBytes) != int(manifest.Bytes) || hex.EncodeToString(digest[:]) != manifest.SHA256 {
		t.Fatalf("canonical ASR fixture identity = bytes:%d sha256:%s, want bytes:%d sha256:%s", len(inputBytes), hex.EncodeToString(digest[:]), manifest.Bytes, manifest.SHA256)
	}

	outputSegment := asrStoryOutputSegment{ID: int32(segment.ID), Start: int64(segment.Start), End: int64(segment.End), Text: segment.Text}
	if float64(outputSegment.Start) != segment.Start || float64(outputSegment.End) != segment.End {
		t.Fatalf("ASR semantic fixture segment bounds are not representable by the public integer timestamp contract: %#v", segment)
	}
	segmentBytes, err := json.Marshal([]asrStoryOutputSegment{outputSegment})
	if err != nil {
		t.Fatalf("marshal canonical ASR segment: %v", err)
	}
	segmentDigest := sha256.Sum256(segmentBytes)
	return manifest, inputPath, inputBytes, string(segmentBytes), "sha256:" + hex.EncodeToString(segmentDigest[:])
}

func asrStoryBackendSegments(t *testing.T, manifest asrStoryFixtureManifest) []models.ASRBackendSegment {
	t.Helper()
	segments := make([]models.ASRBackendSegment, len(manifest.Segments.Items))
	for index, item := range manifest.Segments.Items {
		start, end := int64(item.Start), int64(item.End)
		if float64(start) != item.Start || float64(end) != item.End {
			t.Fatalf("ASR manifest segment %d has non-integral timestamps: %#v", index, item)
		}
		segments[index] = models.ASRBackendSegment{ID: int32(item.ID), Start: start, End: end, Text: item.Text}
	}
	return segments
}

func cloneASRStoryBackendRequest(request models.ASRBackendRequest) models.ASRBackendRequest {
	request.Audio = append([]byte(nil), request.Audio...)
	return request
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
	if string(transcript) != story.manifest.Transcript.Normalized {
		t.Fatalf("transcript output = %q, want manifest normalized transcript %q", transcript, story.manifest.Transcript.Normalized)
	}
	segments, err := os.ReadFile(story.segmentsPath)
	if err != nil {
		t.Fatalf("read segments output: %v", err)
	}
	if string(segments) != story.wantSegments {
		t.Fatalf("segments output = %s, want canonical timestamped JSON", segments)
	}
	assertASRStoryBackendRequest(t, story, *story.received)
	assertASRFixtureCall(t, story.fixture.Calls(), base64.StdEncoding.EncodeToString(story.inputBytes))
	transcriptDigest := sha256.Sum256(transcript)
	segmentsDigest := sha256.Sum256(segments)
	t.Logf("runtime proof command: you models invoke asr --operation ASR --input audio=@%s --output transcript=%s --output segments=%s", story.inputPath, story.transcriptPath, story.segmentsPath)
	t.Logf("runtime proof exitCode=0 stdout=%q stderr=%q", output.String(), invokeStderr.String())
	t.Logf("runtime proof output transcript mediaType=text/plain bytes=%q size=%d sha256=%s", string(transcript), len(transcript), hex.EncodeToString(transcriptDigest[:]))
	t.Logf("runtime proof output segments mediaType=application/json bytes=%s size=%d sha256=%s", string(segments), len(segments), hex.EncodeToString(segmentsDigest[:]))

	assertASRCacheEffects(t, story)
}

func runASRJSONInvocation(t *testing.T, story asrStory) factoryapi.GenericModelInvocationResponse {
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
	assertASRStoryResponse(t, story, response)
	if len(response.Outputs) != 2 || response.Outputs[0].Name != "transcript" || response.Outputs[1].Name != "segments" {
		t.Fatalf("ASR JSON outputs = %#v, want transcript then segments", response.Outputs)
	}
	if response.Outputs[0].Content == nil || *response.Outputs[0].Content != story.manifest.Transcript.Normalized ||
		response.Outputs[1].Content == nil || *response.Outputs[1].Content != story.wantSegments ||
		response.Outputs[0].MediaType == nil || *response.Outputs[0].MediaType != "text/plain" || response.Outputs[1].MediaType == nil || *response.Outputs[1].MediaType != "application/json" {
		t.Fatalf("ASR JSON media metadata = %#v", response.Outputs)
	}
	artifact := response.Outputs[1].Artifact
	if artifact == nil || artifact.ArtifactRef != "artifact:segments" || artifact.MediaType == nil || *artifact.MediaType != "application/json" || artifact.SizeBytes == nil || *artifact.SizeBytes != int64(len(story.wantSegments)) || artifact.Properties == nil || (*artifact.Properties)["digest"] != story.wantSegmentsDigest {
		t.Fatalf("ASR JSON artifact metadata = %#v, want opaque ref/size/digest", artifact)
	}
	t.Logf("runtime proof command: you --json models invoke asr --operation ASR --input audio=@%s", story.inputPath)
	t.Logf("runtime proof exitCode=0 stdout=%s stderr=%q", output.String(), jsonStderr.String())
	t.Logf("runtime proof JSON outputs transcript mediaType=text/plain segments mediaType=application/json artifactRef=%s size=%d digest=%s", artifact.ArtifactRef, *artifact.SizeBytes, (*artifact.Properties)["digest"])
	return response
}

func assertASRStoryBackendRequest(t *testing.T, story asrStory, request models.ASRBackendRequest) {
	t.Helper()
	if !bytes.Equal(request.Audio, story.inputBytes) || request.MediaType != story.manifest.MediaType {
		t.Fatalf("ASR backend request = mediaType:%q bytes:%d sha256:%s, want mediaType:%q bytes:%d sha256:%s", request.MediaType, len(request.Audio), asrStorySHA256(request.Audio), story.manifest.MediaType, len(story.inputBytes), story.manifest.SHA256)
	}
}

func asrStorySHA256(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func assertASRStoryResponse(t *testing.T, story asrStory, response factoryapi.GenericModelInvocationResponse) {
	t.Helper()
	if response.Failure != nil || len(response.Outputs) != 2 {
		t.Fatalf("ASR public response = %#v, want two successful outputs", response)
	}
	if response.Outputs[0].Content == nil || *response.Outputs[0].Content != story.manifest.Transcript.Normalized ||
		response.Outputs[1].Content == nil || *response.Outputs[1].Content != story.wantSegments {
		t.Fatalf("ASR public semantic outputs = %#v, want transcript %q and segments %s", response.Outputs, story.manifest.Transcript.Normalized, story.wantSegments)
	}
}

func runASRRouteParity(t *testing.T, story asrStory, directResponse factoryapi.GenericModelInvocationResponse) {
	t.Helper()
	closeRootProcess(t, story.process, "close direct ASR process before configured-server parity")
	assertLocalAIOMNIHostReleased(t, story.hostLauncher, "direct ASR")

	serverEdges := newASRStoryEdgeSet(t, story.home, story.modelDefinition, story.modelServerClient, story.modelServerURL, story.asrBackend)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                story.dir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       functionalHomeEnvironment(story.home),
		Edges:                     serverEdges.edges,
	})

	httpResponse := invokeASRStoryHTTP(t, server.URL()+"/models/invocations", story)
	assertASRStoryBackendRequest(t, story, *story.received)
	serverResponse := invokeASRStoryExplicitServer(t, server, story)
	assertASRStoryBackendRequest(t, story, *story.received)
	assertASRStoryResponse(t, story, httpResponse)
	assertASRStoryResponse(t, story, serverResponse)

	directObservation, err := localAIASRObservationFromResponse(directResponse)
	if err != nil {
		t.Fatalf("normalize direct ASR response: %v", err)
	}
	httpObservation, err := localAIASRObservationFromResponse(httpResponse)
	if err != nil {
		t.Fatalf("normalize HTTP ASR response: %v", err)
	}
	serverObservation, err := localAIASRObservationFromResponse(serverResponse)
	if err != nil {
		t.Fatalf("normalize explicit-server ASR response: %v", err)
	}
	if !reflect.DeepEqual(directObservation, httpObservation) || !reflect.DeepEqual(directObservation, serverObservation) {
		t.Fatalf("ASR normalized success observations differ across direct CLI, HTTP, and --server CLI: direct=%#v http=%#v server=%#v", directObservation, httpObservation, serverObservation)
	}
	t.Logf("ASR parity proof: direct CLI, HTTP %s, and explicit --server %s returned equivalent transcript/segments, media metadata, artifact size, and digest", server.URL()+"/models/invocations", server.URL())

	server.Close(t)
	assertLocalAIOMNIServerReleased(t, server, serverEdges.hostLauncher)
	assertASRStoryEdgeEffects(t, serverEdges, story.modelDefinition, "configured ASR server")
	if err := story.fixture.Close(); err != nil {
		t.Fatalf("close ASR protocol fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, story.fixture.Endpoint())
}

type asrStoryHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type asrStoryEdgeSet struct {
	edges             serviceedges.Edges
	network           *rejectingModelAssetHTTP
	hostLauncher      *recordingModelHostLauncher
	protocol          *joinedProtocolNegotiator
	compatibility     *joinedCompatibilityChecker
	backendSelections *[]serviceedges.ModelBackendArtifactSelectionRequest
}

func newASRStoryEdgeSet(
	t *testing.T,
	home string,
	modelDefinition models.ModelDefinition,
	hostHTTP asrStoryHTTPClient,
	modelServerURL string,
	asrBackend serviceedges.ModelASRBackend,
) asrStoryEdgeSet {
	t.Helper()
	writeGenericBuiltinModelCache(t, home, modelDefinition.Source)
	selection, backendBody := fixtureBackendSelection(modelDefinition.Backend)
	writeGenericBackendCache(t, home, modelDefinition.Backend, selection, backendBody)
	network := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServerURL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	backendSelections := make([]serviceedges.ModelBackendArtifactSelectionRequest, 0)
	return asrStoryEdgeSet{
		edges: serviceedges.Edges{
			ModelAssetHTTPClient:           network,
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
			ModelHostHTTPClient:    hostHTTP,
			ModelRuntimeHTTPClient: hostHTTP,
		},
		network: network, hostLauncher: hostLauncher, protocol: protocol,
		compatibility: compatibility, backendSelections: &backendSelections,
	}
}

func invokeASRStoryHTTP(t *testing.T, endpoint string, story asrStory) factoryapi.GenericModelInvocationResponse {
	t.Helper()
	contentType := "AUDIO"
	mediaType := story.manifest.MediaType
	operation := factoryapi.ModelOperationName(models.OperationASR)
	inputs := []factoryapi.ModelInvocationInput{{
		Name: "audio", Modality: factoryapi.ModelInvocationContentTypeAudio,
		ContentType: &contentType, MediaType: &mediaType, ContentBase64: &story.inputBytes,
	}}
	offline := true
	payload := factoryapi.GenericModelInvocationRequest{
		Holder: "localai-asr-story", Model: factoryapi.ModelReference{NameOrUri: story.modelDefinition.Name},
		Operation: &operation, Inputs: &inputs, Scope: "factory-session:localai-asr-story", Offline: &offline,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal ASR HTTP request: %v", err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build ASR HTTP request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("POST ASR HTTP request: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read ASR HTTP response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ASR HTTP status = %d body = %s", response.StatusCode, responseBody)
	}
	var result factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		t.Fatalf("decode ASR HTTP response: %v; body=%s", err, responseBody)
	}
	t.Logf("ASR HTTP proof: request mediaType=%s contentBase64Bytes=%d status=%d", mediaType, len(story.inputBytes), response.StatusCode)
	return result
}

func invokeASRStoryExplicitServer(t *testing.T, server *support.FunctionalAPIServer, story asrStory) factoryapi.GenericModelInvocationResponse {
	t.Helper()
	var output, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", strings.TrimSuffix(server.URL(), "/"), "models", "invoke", story.modelDefinition.Name,
		"--operation", models.OperationASR, "--input", "audio=@" + story.inputPath,
	})
	inputs.Input.Env = functionalHomeEnvironment(story.home)
	inputs.Input.WorkingDirectory = story.dir
	inputs.Input.Stdout = &output
	inputs.Input.Stderr = &stderr
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("Process.Execute(ASR explicit --server) error = %v stderr=%q", err, stderr.String())
	}
	var result factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode explicit-server ASR response: %v\n%s", err, output.String())
	}
	t.Logf("ASR explicit-server proof: server=%s exitCode=0 stdout=%s stderr=%q", server.URL(), output.String(), stderr.String())
	return result
}

func assertASRCacheEffects(t *testing.T, story asrStory) {
	t.Helper()
	assertASRStoryEdgeEffects(t, asrStoryEdgeSet{
		network: story.rejectingNetwork, hostLauncher: story.hostLauncher,
		protocol: story.protocol, compatibility: story.compatibility,
		backendSelections: story.backendSelections,
	}, story.modelDefinition, "direct ASR")
}

func assertASRStoryEdgeEffects(
	t *testing.T,
	edges asrStoryEdgeSet,
	modelDefinition models.ModelDefinition,
	label string,
) {
	t.Helper()
	if edges.network.Calls() != 0 || edges.hostLauncher.Calls() == 0 || edges.protocol.Calls() == 0 || edges.compatibility.Calls() == 0 {
		t.Fatalf("%s effects = asset network %d, host starts %d, protocol %d, compatibility %d; want cache-backed joined execution", label, edges.network.Calls(), edges.hostLauncher.Calls(), edges.protocol.Calls(), edges.compatibility.Calls())
	}
	if edges.backendSelections == nil || len(*edges.backendSelections) == 0 {
		t.Fatalf("%s did not resolve a backend artifact", label)
	}
	for _, request := range *edges.backendSelections {
		if request.Backend != modelDefinition.Backend {
			t.Fatalf("%s resolved backend request = %#v, want production catalog backend %q", label, request, modelDefinition.Backend)
		}
	}
	t.Logf("%s production catalog backend resolution: model=%s backend=%s requests=%d", label, modelDefinition.Name, modelDefinition.Backend, len(*edges.backendSelections))
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
