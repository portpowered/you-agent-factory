package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	fixture := localai.Start(t)
	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	process := support.BuildProcess(t, serviceedges.Edges{
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
	environment := functionalHomeEnvironment(home)

	listInputs := support.FakeInputs(t.Context(), []string{"you", "--json", "models", "list"})
	listInputs.Input.Env = environment
	listInputs.Input.WorkingDirectory = dir
	if err := process.Execute(listInputs.Input); err != nil {
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

	wantAudio := localai.AudioBytes()
	var genericStdout, genericStderr bytes.Buffer
	genericInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--input", "text=hello",
	})
	genericInputs.Input.Env = environment
	genericInputs.Input.WorkingDirectory = dir
	genericInputs.Input.Stdout = &genericStdout
	genericInputs.Input.Stderr = &genericStderr
	if err := process.Execute(genericInputs.Input); err != nil {
		t.Fatalf("Process.Execute(generic TTS stdout) error = %v", err)
	}
	if !bytes.Equal(genericStdout.Bytes(), wantAudio) {
		t.Fatalf("generic TTS stdout = %d bytes, want exact fixture audio %d bytes", genericStdout.Len(), len(wantAudio))
	}
	if genericStderr.Len() != 0 {
		t.Fatalf("generic TTS stderr = %q, want empty", genericStderr.String())
	}

	aliasPath := filepath.Join(t.TempDir(), "alias.wav")
	var aliasStdout, aliasStderr bytes.Buffer
	aliasInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "hello", "--output", aliasPath,
	})
	aliasInputs.Input.Env = environment
	aliasInputs.Input.WorkingDirectory = dir
	aliasInputs.Input.Stdout = &aliasStdout
	aliasInputs.Input.Stderr = &aliasStderr
	if err := process.Execute(aliasInputs.Input); err != nil {
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
	assertEquivalentTTSRequests(t, requests[:2])

	failurePath := filepath.Join(t.TempDir(), "failure.wav")
	var failureStdout bytes.Buffer
	failureInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "backend failure", "--output", failurePath,
	})
	failureInputs.Input.Env = environment
	failureInputs.Input.WorkingDirectory = dir
	failureInputs.Input.Stdout = &failureStdout
	failureInputs.Input.Stderr = io.Discard
	if err := process.Execute(failureInputs.Input); err == nil {
		t.Fatal("Process.Execute(failing direct TTS alias) error = nil, want backend failure")
	}
	if _, err := os.Stat(failurePath); !os.IsNotExist(err) {
		t.Fatalf("failed direct TTS output stat error = %v, want no partial file", err)
	}
	if failureStdout.Len() != 0 {
		t.Fatalf("failed direct TTS stdout = %q, want empty", failureStdout.String())
	}

	recoveryPath := filepath.Join(t.TempDir(), "recovery.wav")
	var recoveryStdout bytes.Buffer
	recoveryInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "after failure", "--output", recoveryPath,
	})
	recoveryInputs.Input.Env = environment
	recoveryInputs.Input.WorkingDirectory = dir
	recoveryInputs.Input.Stdout = &recoveryStdout
	recoveryInputs.Input.Stderr = io.Discard
	if err := process.Execute(recoveryInputs.Input); err != nil {
		t.Fatalf("Process.Execute(recovered direct TTS alias) error = %v", err)
	}
	recoveryAudio, err := os.ReadFile(recoveryPath)
	if err != nil {
		t.Fatalf("read recovered direct TTS output: %v", err)
	}
	if !bytes.Equal(recoveryAudio, wantAudio) {
		t.Fatalf("recovered direct TTS audio = %d bytes, want exact fixture audio %d bytes", len(recoveryAudio), len(wantAudio))
	}
	if len(requests) != 4 {
		t.Fatalf("fixture-backed TTS request count = %d, want generic, alias, failure, recovery", len(requests))
	}
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
	revisionPath := filepath.Join(home, ".agent-factory", "models", "tts", revision)
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
