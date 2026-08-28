package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// TestModelsDirectTTSAliasEndToEndThroughRootBuildProcess proves that the
// generic text input and legacy --text/--output spellings share one request,
// one readiness projection, and one lease-backed fixture invocation.
func TestModelsDirectTTSAliasEndToEndThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	story := setupTTSStory(t)
	assertTTSReady(t, story)
	wantAudio := localai.AudioBytes()
	runGenericTTS(t, story, wantAudio)
	runAliasTTS(t, story, wantAudio)
	assertEquivalentTTSRequests(t, *story.requests)
	runFailedAndRecoveredTTS(t, story, wantAudio)
}

type ttsStory struct {
	process     support.Process
	dir         string
	environment []string
	requests    *[]models.InvokeModelRequest
}

func setupTTSStory(t *testing.T) ttsStory {
	t.Helper()
	fixture := characterizationStartLocalAI(t)
	modelServer := characterizationNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSManagedRuntimeCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	selection := pinnedTTSBackendSelection()
	assetFiles := functionalModelAssetFileSystem{home: home}
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	dir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	var requests []models.InvokeModelRequest
	backend := func(ctx context.Context, request models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		requests = append(requests, request)
		inputs := request.Inputs
		if len(inputs) == 0 {
			inputs = []models.InferenceInput{request.Input}
		}
		if len(inputs) == 1 && inputs[0].Content == "backend failure" {
			return nil, nil, errors.New("fixture TTS backend failure")
		}
		return fixture.InvocationBackend(ctx, request)
	}
	process := characterizationBuildProcess(t, serviceedges.Edges{
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
		ModelResolveBackendArtifact: func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			return selection, nil
		},
		ModelInvocationBackend: backend,
		ModelHostHTTPClient:    modelServer.Client(),
		ModelRuntimeHTTPClient: modelServer.Client(),
	})
	t.Cleanup(func() { closeRootProcess(t, process, "close TTS root process") })
	return ttsStory{process: process, dir: dir, environment: functionalHomeEnvironment(home), requests: &requests}
}

func assertTTSReady(t *testing.T, story ttsStory) {
	t.Helper()
	listInputs := support.FakeInputs(t.Context(), []string{"you", "--json", "models", "list"})
	listInputs.Input.Env = story.environment
	listInputs.Input.WorkingDirectory = story.dir
	if err := story.process.Execute(listInputs.Input); err != nil {
		t.Fatalf("Process.Execute(models list) error = %v", err)
	}
	var listed factoryapi.ListModelsResponse
	if err := jsonUnmarshalFunctional(listInputs.Stdout(), &listed); err != nil {
		t.Fatalf("decode models list: %v\n%s", err, listInputs.Stdout())
	}
	tts, ok := findModelSummary(listed.Results, models.BuiltInModelNameTTS)
	if !ok {
		t.Fatalf("models list did not include %q: %#v", models.BuiltInModelNameTTS, listed.Results)
	}
	if tts.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		tts.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("models list TTS runtime = %#v, want READY/INSTALLED", tts.ManagedRuntime)
	}
	t.Logf("runtime proof command: you --json models list exitCode=0 model=tts readiness=%s lifecycle=%s", tts.ManagedRuntime.ReadinessState, tts.ManagedRuntime.LifecycleState)
}

func runGenericTTS(t *testing.T, story ttsStory, wantAudio []byte) {
	t.Helper()
	var genericStdout, genericStderr bytes.Buffer
	genericInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--input", "text=hello",
	})
	genericInputs.Input.Env = story.environment
	genericInputs.Input.WorkingDirectory = story.dir
	genericInputs.Input.Stdout = &genericStdout
	genericInputs.Input.Stderr = &genericStderr
	if err := story.process.Execute(genericInputs.Input); err != nil {
		t.Fatalf("Process.Execute(generic TTS stdout) error = %v", err)
	}
	if !bytes.Equal(genericStdout.Bytes(), wantAudio) {
		t.Fatalf("generic TTS stdout = %d bytes, want exact fixture audio %d bytes", genericStdout.Len(), len(wantAudio))
	}
	if genericStderr.Len() != 0 {
		t.Fatalf("generic TTS stderr = %q, want empty", genericStderr.String())
	}
	t.Logf("runtime proof command: you models invoke tts --operation TTS --input text=hello")
	t.Logf("runtime proof exitCode=0 stdout=<raw audio bytes> mediaType=audio/wav size=%d sha256=%s stderr=%q", len(genericStdout.Bytes()), ttsDigest(genericStdout.Bytes()), genericStderr.String())
}

func runAliasTTS(t *testing.T, story ttsStory, wantAudio []byte) {
	t.Helper()
	aliasPath := filepath.Join(t.TempDir(), "alias.wav")
	var aliasStdout, aliasStderr bytes.Buffer
	aliasInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "hello", "--output", aliasPath,
	})
	aliasInputs.Input.Env = story.environment
	aliasInputs.Input.WorkingDirectory = story.dir
	aliasInputs.Input.Stdout = &aliasStdout
	aliasInputs.Input.Stderr = &aliasStderr
	if err := story.process.Execute(aliasInputs.Input); err != nil {
		t.Fatalf("Process.Execute(direct TTS alias) error = %v", err)
	}
	aliasAudio, err := os.ReadFile(aliasPath)
	if err != nil {
		t.Fatalf("read direct TTS alias output: %v", err)
	}
	if !bytes.Equal(aliasAudio, wantAudio) {
		t.Fatalf("direct TTS alias audio = %d bytes, want exact fixture audio %d bytes", len(aliasAudio), len(wantAudio))
	}
	if aliasStdout.String() != "Wrote audio: "+aliasPath+"\n" || aliasStderr.Len() != 0 {
		t.Fatalf("direct TTS alias streams = stdout %q stderr %q, want status-only stdout and empty stderr", aliasStdout.String(), aliasStderr.String())
	}
	t.Logf("runtime proof command: you models invoke tts --operation TTS --text hello --output %s", aliasPath)
	t.Logf("runtime proof exitCode=0 stdout=%q stderr=%q output mediaType=audio/wav size=%d sha256=%s", aliasStdout.String(), aliasStderr.String(), len(aliasAudio), ttsDigest(aliasAudio))
}

func runFailedAndRecoveredTTS(t *testing.T, story ttsStory, wantAudio []byte) {
	t.Helper()
	failurePath := filepath.Join(t.TempDir(), "failure.wav")
	var failureStdout, failureStderr bytes.Buffer
	failureInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "backend failure", "--output", failurePath,
	})
	failureInputs.Input.Env = story.environment
	failureInputs.Input.WorkingDirectory = story.dir
	failureInputs.Input.Stdout = &failureStdout
	failureInputs.Input.Stderr = &failureStderr
	failureErr := story.process.Execute(failureInputs.Input)
	if failureErr == nil {
		t.Fatal("Process.Execute(failing direct TTS alias) error = nil, want backend failure")
	}
	if _, err := os.Stat(failurePath); !os.IsNotExist(err) {
		t.Fatalf("failed direct TTS output stat error = %v, want no partial file", err)
	}
	if failureStdout.Len() != 0 {
		t.Fatalf("failed direct TTS stdout = %q, want empty", failureStdout.String())
	}
	t.Logf("runtime proof command: you models invoke tts --operation TTS --text backend-failure --output %s", failurePath)
	t.Logf("runtime proof exitCode=1 stdout=%q stderr=%q error=%q outputExists=false", failureStdout.String(), failureStderr.String(), failureErr.Error())

	recoveryPath := filepath.Join(t.TempDir(), "recovery.wav")
	var recoveryStdout, recoveryStderr bytes.Buffer
	recoveryInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "after failure", "--output", recoveryPath,
	})
	recoveryInputs.Input.Env = story.environment
	recoveryInputs.Input.WorkingDirectory = story.dir
	recoveryInputs.Input.Stdout = &recoveryStdout
	recoveryInputs.Input.Stderr = &recoveryStderr
	if err := story.process.Execute(recoveryInputs.Input); err != nil {
		t.Fatalf("Process.Execute(recovered direct TTS alias) error = %v", err)
	}
	recoveryAudio, err := os.ReadFile(recoveryPath)
	if err != nil {
		t.Fatalf("read recovered direct TTS output: %v", err)
	}
	if !bytes.Equal(recoveryAudio, wantAudio) {
		t.Fatalf("recovered direct TTS audio = %d bytes, want exact fixture audio %d bytes", len(recoveryAudio), len(wantAudio))
	}
	t.Logf("runtime proof command: you models invoke tts --operation TTS --text after-failure --output %s", recoveryPath)
	t.Logf("runtime proof exitCode=0 stdout=%q stderr=%q output mediaType=audio/wav size=%d sha256=%s", recoveryStdout.String(), recoveryStderr.String(), len(recoveryAudio), ttsDigest(recoveryAudio))
	if len(*story.requests) != 4 {
		t.Fatalf("fixture-backed TTS request count = %d, want generic, alias, failure, recovery", len(*story.requests))
	}
}

func ttsDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func assertEquivalentTTSRequests(t *testing.T, requests []models.InvokeModelRequest) {
	t.Helper()
	if len(requests) != 2 {
		t.Fatalf("TTS requests = %d, want generic and alias", len(requests))
	}
	for index, request := range requests {
		if request.Model.NameOrURI != models.BuiltInModelNameTTS || request.Operation != models.OperationTTS || len(request.Inputs) != 1 {
			t.Fatalf("TTS request[%d] = %#v, want tts/TTS with one input", index, request)
		}
		input := request.Inputs[0]
		if input.Name != "text" || input.Modality != models.ModalityText || input.ContentType != "text/plain" || input.MediaType != "text/plain" || input.Content != "hello" {
			t.Fatalf("TTS request[%d] input = %#v, want canonical named text input", index, input)
		}
	}
	if requests[0].Model != requests[1].Model || requests[0].Operation != requests[1].Operation || requests[0].Inputs[0] != requests[1].Inputs[0] {
		t.Fatalf("generic and alias TTS requests differ:\ngeneric=%#v\nalias=%#v", requests[0], requests[1])
	}
}

func jsonUnmarshalFunctional(data string, target any) error {
	return json.Unmarshal([]byte(data), target)
}

func writeGenericBuiltinTTSManagedRuntimeCache(t *testing.T, home string) {
	t.Helper()
	const revision = "505114ae6ad17be74df98e6939707434ec49c187"
	body := []byte("joined built-in tts fixture")
	digest := sha256.Sum256(body)
	canonicalModelName := strings.ToUpper(strings.TrimSpace(models.BuiltInModelNameTTS))
	revisionPath := filepath.Join(home, ".agent-factory", "models", canonicalModelName, revision)
	if err := os.MkdirAll(revisionPath, 0o755); err != nil {
		t.Fatalf("create managed TTS runtime fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(revisionPath, "weights.bin"), body, 0o644); err != nil {
		t.Fatalf("write managed TTS runtime fixture: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"modelName": "tts",
		"revision":  revision,
		"files": []map[string]any{{
			"path": "weights.bin", "bytes": len(body), "sha256": hex.EncodeToString(digest[:]),
		}},
	})
	if err != nil {
		t.Fatalf("marshal managed TTS runtime metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(revisionPath), ".managed-cache.json"), metadata, 0o644); err != nil {
		t.Fatalf("write managed TTS runtime metadata: %v", err)
	}
}
