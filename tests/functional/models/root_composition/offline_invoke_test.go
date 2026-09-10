package root_composition_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// TestModelsInvokeOfflineAllBuiltinFamiliesThroughRootBuildProcess proves the
// authored CLI switch reaches each built-in invocation family while keeping the
// root-composition effects fully controlled.
func TestModelsInvokeOfflineAllBuiltinFamiliesThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	t.Run("LLM", testModelsInvokeOfflineLLMThroughRootBuildProcess)
	t.Run("ASR", testModelsInvokeOfflineASRThroughRootBuildProcess)
	t.Run("TTS", testModelsInvokeOfflineTTSThroughRootBuildProcess)
	t.Run("EMBED", testModelsInvokeOfflineEmbedThroughRootBuildProcess)
}

func testModelsInvokeOfflineLLMThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	story := newOfflineLLMStory(t, true)

	stdout, stderr, err := runStory004CLI(t, story.process, story.dir, story.environment, []string{
		"you", "models", "invoke", models.BuiltInModelNameLLM, "--offline", "--input", "prompt=offline llm",
	})
	if err != nil || stdout != "offline llm" {
		t.Fatalf("offline LLM invocation = err %v stdout %q stderr %q; want successful cached response", err, stdout, stderr)
	}
	if story.fixture.Calls() != 1 {
		t.Fatalf("offline LLM protocol calls = %d, want one cached invocation", story.fixture.Calls())
	}
	if story.network.Calls() != 0 || story.launcher.Calls() == 0 || story.protocol.Calls() == 0 || story.compatibility.Calls() == 0 {
		t.Fatalf("offline LLM effects = network:%d host:%d protocol:%d compatibility:%d; want network 0 and joined execution", story.network.Calls(), story.launcher.Calls(), story.protocol.Calls(), story.compatibility.Calls())
	}
	closeRootProcess(t, story.process, "close offline LLM root process")
	if story.launcher.Active() {
		t.Fatal("offline LLM host remains active after root close")
	}
}

func testModelsInvokeOfflineASRThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	story := setupASRStory(t)

	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", story.modelDefinition.Name, "--offline", "--operation", "ASR",
		"--input", "audio=@" + story.inputPath,
	})
	inputs.Input.Env = functionalHomeEnvironment(story.home)
	inputs.Input.WorkingDirectory = story.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := story.process.Execute(inputs.Input); err != nil {
		t.Fatalf("offline ASR invocation = %v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode offline ASR response: %v\n%s", err, stdout.String())
	}
	if len(response.Outputs) != 2 || response.Outputs[0].Name != "transcript" || response.Outputs[1].Name != "segments" {
		t.Fatalf("offline ASR outputs = %#v, want transcript and segments", response.Outputs)
	}
	if string(story.received.Audio) != string(story.inputBytes) || story.received.MediaType != "audio/wav" {
		t.Fatalf("offline ASR backend request = %#v, want cached input bytes and audio/wav", *story.received)
	}
	assertASRFixtureCall(t, story.fixture.Calls(), base64.StdEncoding.EncodeToString(story.inputBytes))
	if story.rejectingNetwork.Calls() != 0 || story.hostLauncher.Calls() == 0 || story.protocol.Calls() == 0 || story.compatibility.Calls() == 0 {
		t.Fatalf("offline ASR effects = network:%d host:%d protocol:%d compatibility:%d; want network 0 and joined execution", story.rejectingNetwork.Calls(), story.hostLauncher.Calls(), story.protocol.Calls(), story.compatibility.Calls())
	}
	closeRootProcess(t, story.process, "close offline ASR root process")
	if story.hostLauncher.Active() {
		t.Fatal("offline ASR host remains active after root close")
	}
}

func testModelsInvokeOfflineTTSThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	story := setupTTSStory(t)
	runGenericTTS(t, story, localai.AudioBytes())
	protocolBefore := len(story.protocol.Calls())
	networkBefore := story.network.Calls()

	stdout, stderr, err := runStory004CLI(t, story.process, story.dir, story.environment, []string{
		"you", "models", "invoke", models.BuiltInModelNameTTS, "--offline", "--operation", "TTS", "--input", "text=hello",
	})
	if err != nil || !bytes.Equal([]byte(stdout), localai.AudioBytes()) {
		t.Fatalf("offline TTS invocation = err %v stdoutBytes:%d stderr=%q; want cached fixture audio", err, len(stdout), stderr)
	}
	if story.generic.Calls() != 0 || len(story.protocol.Calls())-protocolBefore != 1 || story.network.Calls() != networkBefore {
		t.Fatalf("offline TTS effects = generic:%d privateProtocolDelta:%d networkDelta:%d; want 0/1/0", story.generic.Calls(), len(story.protocol.Calls())-protocolBefore, story.network.Calls()-networkBefore)
	}
	closeRootProcess(t, story.process, "close offline TTS root process")
	starts, stops, waits, active := story.host.Snapshot()
	if starts == 0 || starts != stops || stops != waits || active != 0 {
		t.Fatalf("offline TTS host lifecycle = starts:%d stops:%d waits:%d active:%d; want a fully closed cached invocation", starts, stops, waits, active)
	}
}

func testModelsInvokeOfflineEmbedThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	hostServer := story004HostServer(t)
	home := functionalTempDir(t)
	backendBody := []byte("story-001-offline-embedding-backend")
	selection := story004EmbedBackendSelection(backendBody)
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, backendBody)
	assetNetwork := &rejectingModelAssetHTTP{}
	launcher := &recordingModelHostLauncher{endpoint: hostServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	fixture := newStory004EmbedFixture()
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := functionalBuildProcess(t, story004EmbedEdges(
		home, assetNetwork, hostServer.Client(), launcher, protocol, compatibility,
		selection, fixture,
	))
	environment := functionalHomeEnvironment(home)

	stdout, stderr, err := runStory004CLI(t, process, factoryDir, environment, []string{
		"you", "models", "invoke", models.BuiltInModelNameEmbed, "--offline", "--input", "text=Find similar work",
	})
	if err != nil || stdout != `[0.1,0.2,0.3,0.4]` || stderr != "" {
		t.Fatalf("offline EMBED invocation = err %v stdout %q stderr %q; want cached vector and no diagnostic stream", err, stdout, stderr)
	}
	if fixture.Calls() != 1 || assetNetwork.Calls() != 0 || launcher.Calls() == 0 || protocol.Calls() == 0 || compatibility.Calls() == 0 {
		t.Fatalf("offline EMBED effects = fixture:%d network:%d host:%d protocol:%d compatibility:%d; want 1/0 and joined execution", fixture.Calls(), assetNetwork.Calls(), launcher.Calls(), protocol.Calls(), compatibility.Calls())
	}
	closeRootProcess(t, process, "close offline EMBED root process")
	if launcher.Active() {
		t.Fatal("offline EMBED host remains active after root close")
	}
}

func TestModelsInvokeOfflineMissingArtifactsReportsCompleteSetThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	story := newOfflineLLMStory(t, false)

	stdout, stderr, err := runStory004CLI(t, story.process, story.dir, story.environment, []string{
		"you", "models", "invoke", models.BuiltInModelNameLLM, "--offline", "--input", "prompt=offline miss",
	})
	if err == nil {
		t.Fatal("offline missing-artifact invocation error = nil, want deterministic cache-unavailable failure")
	}
	if stdout != "" {
		t.Fatalf("offline missing-artifact stdout = %q, want empty", stdout)
	}
	t.Logf("offline missing-artifact returned %T: %v", err, err)
	var diagnostic factoryapi.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &diagnostic); decodeErr != nil {
		t.Fatalf("decode offline missing-artifact diagnostic: %v\nstderr=%q", decodeErr, stderr)
	}
	if diagnostic.Code != factoryapi.ErrorResponseCode("MODEL_OFFLINE_CACHE_UNAVAILABLE") || diagnostic.Family != factoryapi.ErrorFamilyConflict {
		t.Fatalf("offline missing-artifact diagnostic = %#v, want MODEL_OFFLINE_CACHE_UNAVAILABLE/CONFLICT", diagnostic)
	}
	var offline *models.AssetOfflineError
	if !errors.As(err, &offline) || offline == nil {
		t.Fatalf("offline missing-artifact error = %v, want AssetOfflineError with complete missing set", err)
	}
	wantMissing := []string{offlineModelArtifactName(story.modelDefinition.Source), story.selection.Name}
	sort.Strings(wantMissing)
	if strings.Join(offline.Missing, "\x00") != strings.Join(wantMissing, "\x00") {
		t.Fatalf("offline missing artifacts = %#v, want sorted complete set %#v", offline.Missing, wantMissing)
	}
	if story.network.Calls() != 0 || story.launcher.Calls() != 0 || story.protocol.Calls() != 0 || story.compatibility.Calls() != 0 {
		t.Fatalf("offline missing-artifact effects = network:%d host:%d protocol:%d compatibility:%d; want all zero", story.network.Calls(), story.launcher.Calls(), story.protocol.Calls(), story.compatibility.Calls())
	}
	closeRootProcess(t, story.process, "close offline missing-artifact root process")
}

type offlineLLMStory struct {
	process         support.ApplicationProcess
	dir             string
	environment     []string
	modelDefinition models.ModelDefinition
	selection       serviceedges.ModelBackendArtifactSelection
	network         *rejectingModelAssetHTTP
	launcher        *recordingModelHostLauncher
	protocol        *joinedProtocolNegotiator
	compatibility   *joinedCompatibilityChecker
	fixture         *omniTextProtocolFixture
}

func newOfflineLLMStory(t *testing.T, withCache bool) offlineLLMStory {
	t.Helper()
	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameLLM)
	if !ok {
		t.Fatal("built-in catalog did not publish the LLM model definition")
	}
	home := functionalTempDir(t)
	selection := genericLlamaBackendSelection()
	if withCache {
		writeGenericBuiltinModelCache(t, home, definition.Source)
		writeGenericBackendCache(t, home, definition.Backend, selection, []byte("localai-llamacpp/linux-amd64"))
	}
	network := &rejectingModelAssetHTTP{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	launcher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	fixture := &omniTextProtocolFixture{response: "offline llm"}
	dir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := functionalBuildProcess(t, serviceedges.Edges{
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
		ModelHostProcessLauncher:       launcher,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(
			context.Context,
			serviceedges.ModelBackendArtifactSelectionRequest,
		) (serviceedges.ModelBackendArtifactSelection, error) {
			return selection, nil
		},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationProtocolClient: fixture,
	})
	return offlineLLMStory{
		process: process, dir: dir, environment: functionalHomeEnvironment(home),
		modelDefinition: definition, selection: selection, network: network,
		launcher: launcher, protocol: protocol, compatibility: compatibility, fixture: fixture,
	}
}

func offlineModelArtifactName(source string) string {
	withoutRevision := source
	if index := strings.LastIndexByte(withoutRevision, '@'); index >= 0 {
		withoutRevision = withoutRevision[:index]
	}
	if index := strings.LastIndexByte(withoutRevision, '/'); index >= 0 {
		return withoutRevision[index+1:]
	}
	return withoutRevision
}
