package inference_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
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
	// This byte-identity protocol witness selects an explicit Whisper model.
	modelDefinition.Name, modelDefinition.Source, modelDefinition.Backend = "whisper-asr-fixture", pullToReadySource, "localai-whisper"
	home := functionalTempDir(t)
	writeGenericBuiltinModelCache(t, home, modelDefinition.Source)
	writeGenericModelSourceOverride(t, home, modelDefinition.Name, modelDefinition.Source, modelDefinition.Backend)
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
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(ctx context.Context, request serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			if err := ctx.Err(); err != nil {
				return serviceedges.ModelBackendArtifactSelection{}, err
			}
			backendSelections = append(backendSelections, request)
			return selection, nil
		},
		ModelASRBackend:     asrBackend,
		ModelHostHTTPClient: modelServer.Client(),
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

type asrStoryHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
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
