package local

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type localModelCommandRunnerFunc func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error)

func (fn localModelCommandRunnerFunc) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return fn(ctx, req)
}

func mustNewOmniVoiceRuntime(t *testing.T, runner platformprocess.CommandRunner) Runtime {
	t.Helper()
	if runner == nil {
		runner = localModelCommandRunnerFunc(func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
			return platformprocess.CommandResult{}, nil
		})
	}
	runtime, err := NewOmniVoiceRuntime(runner, http.DefaultClient, os.Stat, os.TempDir, func(dir, pattern string) (TempFile, error) {
		return os.CreateTemp(dir, pattern)
	})
	if err != nil {
		t.Fatalf("NewOmniVoiceRuntime: %v", err)
	}
	return runtime
}

func TestOmniVoiceLocalRuntime_LoadAndInvoke_ReturnsAudioContentFromOutputFile(t *testing.T) {
	t.Parallel()
	cachePath, files := writeOmniVoiceCacheFiles(t)
	worker := cloneLocalModelRuntimeWorker()
	worker.Command = "omnivoice-test"
	worker.Args = []string{"--backend", "llamacpp"}

	var got platformprocess.CommandRequest
	runtime := mustNewOmniVoiceRuntime(t, localModelCommandRunnerFunc(func(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		got = req
		outputPath := commandArgValue(t, req.Args, "--output")
		if err := os.WriteFile(outputPath, []byte("RIFF"), 0o644); err != nil {
			t.Fatalf("write output audio: %v", err)
		}
		return platformprocess.CommandResult{}, nil
	}))

	handle, err := runtime.Load(context.Background(), LoadRequest{
		Resource:  localModelFactoryConfig().Resources[0],
		Worker:    worker,
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: cachePath,
		Revision:  "rev-test",
		Files:     files,
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	workdir := t.TempDir()
	response, err := handle.Invoke(context.Background(), InvocationRequest{
		Request: ModelInvocation{
			Dispatch: work.WorkDispatch{
				DispatchID:      "dispatch-1",
				TransitionID:    "transition-1",
				WorkerType:      "tts-worker",
				WorkstationName: "speak",
			},
			ModelOperation:   "TTS",
			WorkingDirectory: workdir,
			ModelBindings: []modelinference.ResolvedModelOperationBinding{{
				Slot:   "text",
				Source: "INPUT",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "hello local runtime",
				}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	assertOmniVoiceCommandRequest(t, got, workdir, files)
	assertOmniVoiceInvocationPayload(t, got.Stdin)
	assertOmniVoiceResponseContent(t, response.Content)
}

func TestOmniVoiceLocalRuntime_Invoke_RequiresResolvedTextSlot(t *testing.T) {
	t.Parallel()
	cachePath, files := writeOmniVoiceCacheFiles(t)
	runtime := mustNewOmniVoiceRuntime(t, localModelCommandRunnerFunc(func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		t.Fatal("expected command runner not to be called")
		return platformprocess.CommandResult{}, nil
	}))

	handle, err := runtime.Load(context.Background(), LoadRequest{
		Resource:  localModelFactoryConfig().Resources[0],
		Worker:    cloneLocalModelRuntimeWorker(),
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: cachePath,
		Revision:  "rev-test",
		Files:     files,
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, err = handle.Invoke(context.Background(), InvocationRequest{
		Request: ModelInvocation{
			ModelOperation: "TTS",
		},
	})
	if err == nil || !strings.Contains(err.Error(), `requires resolved slot "text"`) {
		t.Fatalf("Invoke error = %v, want missing text slot error", err)
	}
}

func TestOmniVoiceLocalRuntime_Invoke_PropagatesRuntimeExitFailure(t *testing.T) {
	t.Parallel()
	cachePath, files := writeOmniVoiceCacheFiles(t)
	runtime := mustNewOmniVoiceRuntime(t, localModelCommandRunnerFunc(func(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		outputPath := commandArgValue(t, req.Args, "--output")
		if err := os.WriteFile(outputPath, []byte(""), 0o644); err != nil {
			t.Fatalf("seed output file: %v", err)
		}
		return platformprocess.CommandResult{
			ExitCode: 17,
			Stdout:   []byte("partial output"),
			Stderr:   []byte("backend failed"),
		}, nil
	}))

	handle, err := runtime.Load(context.Background(), LoadRequest{
		Resource:  localModelFactoryConfig().Resources[0],
		Worker:    cloneLocalModelRuntimeWorker(),
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: cachePath,
		Revision:  "rev-test",
		Files:     files,
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, err = handle.Invoke(context.Background(), InvocationRequest{
		Request: ModelInvocation{
			ModelOperation: "TTS",
			ModelBindings: []modelinference.ResolvedModelOperationBinding{{
				Slot:   "text",
				Source: "INPUT",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "fail me",
				}},
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "exited with code 17") || !strings.Contains(err.Error(), "backend failed") {
		t.Fatalf("Invoke error = %v, want surfaced runtime exit failure", err)
	}
}

func TestOmniVoiceLocalRuntime_LoadRejectsMissingTokenizerAsset(t *testing.T) {
	t.Parallel()
	cachePath := t.TempDir()
	modelPath := filepath.Join(cachePath, "omnivoice-base-Q4_K_M.gguf")
	if err := os.WriteFile(modelPath, []byte("gguf"), 0o644); err != nil {
		t.Fatalf("write model file: %v", err)
	}
	runtime := mustNewOmniVoiceRuntime(t, nil)
	_, err := runtime.Load(context.Background(), LoadRequest{
		Resource:  localModelFactoryConfig().Resources[0],
		Worker:    cloneLocalModelRuntimeWorker(),
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: cachePath,
		Revision:  "rev-test",
		Files:     []string{modelPath},
	})
	if err == nil || !strings.Contains(err.Error(), "tokenizer asset is required") {
		t.Fatalf("Load error = %v, want missing tokenizer asset error", err)
	}
}

func TestOmniVoiceLocalRuntime_Invoke_PropagatesRunnerError(t *testing.T) {
	t.Parallel()
	cachePath, files := writeOmniVoiceCacheFiles(t)
	runtime := mustNewOmniVoiceRuntime(t, localModelCommandRunnerFunc(func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		return platformprocess.CommandResult{}, errors.New("runner crashed")
	}))
	handle, err := runtime.Load(context.Background(), LoadRequest{
		Resource:  localModelFactoryConfig().Resources[0],
		Worker:    cloneLocalModelRuntimeWorker(),
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: cachePath,
		Revision:  "rev-test",
		Files:     files,
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, err = handle.Invoke(context.Background(), InvocationRequest{
		Request: ModelInvocation{
			ModelOperation: "TTS",
			ModelBindings: []modelinference.ResolvedModelOperationBinding{{
				Slot:   "text",
				Source: "INPUT",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "hello",
				}},
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "runner crashed") {
		t.Fatalf("Invoke error = %v, want runner error", err)
	}
}

func assertOmniVoiceCommandRequest(t *testing.T, got platformprocess.CommandRequest, workdir string, files []string) {
	t.Helper()
	if got.Command != "omnivoice-test" {
		t.Fatalf("command = %q, want omnivoice-test", got.Command)
	}
	if got.WorkDir != workdir {
		t.Fatalf("workdir = %q, want %q", got.WorkDir, workdir)
	}
	wantArgs := []string{
		"--backend", "llamacpp",
		"invoke",
		"--model", files[0],
		"--tokenizer", files[1],
		"--output", commandArgValue(t, got.Args, "--output"),
	}
	assertLocalModelStringSlicesEqual(t, wantArgs, got.Args)
}

func assertOmniVoiceInvocationPayload(t *testing.T, stdin []byte) {
	t.Helper()
	var payload OmniVoiceInvocationPayload
	if err := json.Unmarshal(stdin, &payload); err != nil {
		t.Fatalf("decode stdin payload: %v", err)
	}
	if payload.Operation != "TTS" || payload.Text != "hello local runtime" || payload.ModelName != "OMNIVOICE_Q4_K_M" || payload.Revision != "rev-test" {
		t.Fatalf("stdin payload = %#v, want TTS text payload with model identity", payload)
	}
}

func assertOmniVoiceResponseContent(t *testing.T, responseContent string) {
	t.Helper()
	var content []work.WorkContentPart
	if err := json.Unmarshal([]byte(responseContent), &content); err != nil {
		t.Fatalf("decode response content: %v", err)
	}
	if len(content) != 1 {
		t.Fatalf("content count = %d, want 1", len(content))
	}
	if content[0].Type != work.WorkContentPartTypeAudio || content[0].ContentType != OmniVoiceAudioContentType || strings.TrimSpace(content[0].File) == "" {
		t.Fatalf("content = %#v, want AUDIO content with output file", content)
	}
	if _, err := os.Stat(content[0].File); err != nil {
		t.Fatalf("stat audio output %q: %v", content[0].File, err)
	}
}

func writeOmniVoiceCacheFiles(t *testing.T) (string, []string) {
	t.Helper()
	cachePath := filepath.Join(t.TempDir(), "cache", "rev-test")
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatalf("mkdir cache path: %v", err)
	}
	files := []string{
		filepath.Join(cachePath, "omnivoice-base-Q4_K_M.gguf"),
		filepath.Join(cachePath, "omnivoice-tokenizer-Q4_K_M.gguf"),
	}
	for _, file := range files {
		if err := os.WriteFile(file, []byte("gguf"), 0o644); err != nil {
			t.Fatalf("write cache asset %q: %v", file, err)
		}
	}
	return cachePath, files
}

func cloneLocalModelRuntimeWorker() *modelRuntimeWorker {
	worker := localModelRuntimeWorker()
	worker.Resources = append([]modelRuntimeResource(nil), worker.Resources...)
	worker.Operations = append([]models.RuntimeOperation(nil), worker.Operations...)
	return &worker
}

func localModelFactoryConfig() *testFactoryConfig {
	return &testFactoryConfig{
		Resources: []modelRuntimeResource{{
			Name:       "omnivoice-cache",
			Type:       models.RuntimeResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	}
}

func localModelRuntimeWorker() modelRuntimeWorker {
	return modelRuntimeWorker{
		Name:          "tts-worker",
		Type:          models.RuntimeWorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelProvider: "codex",
		ModelLocality: models.RuntimeModelLocalityLocal,
		Resources: []modelRuntimeResource{{
			Name:     "omnivoice-cache",
			Capacity: 1,
		}},
		Operations: []models.RuntimeOperation{{
			Name: "TTS",
			Inputs: []models.RuntimeOperationSlot{{
				Name:         "text",
				ContentTypes: []string{models.RuntimeContentTypeText},
				Required:     true,
			}},
			Outputs: []models.RuntimeOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{models.RuntimeContentTypeAudio},
			}},
		}},
	}
}

func commandArgValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	t.Fatalf("flag %q not found in args %v", flag, args)
	return ""
}

func assertLocalModelStringSlicesEqual(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("expected %d args, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("expected arg %d to be %q, got %q; full args: %v", i, want[i], got[i], got)
		}
	}
}

func TestOmniVoiceLocalRuntime_LoadAndInvoke_UsesSupervisedServingEndpoint(t *testing.T) {
	cachePath, files := writeOmniVoiceCacheFiles(t)
	worker := cloneLocalModelRuntimeWorker()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"type":"audio","slot":"audio","file":"/tmp/speech.wav","contentType":"audio/wav"}]`))
	}))
	t.Cleanup(server.Close)

	runtime := mustNewOmniVoiceRuntime(t, localModelCommandRunnerFunc(func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		t.Fatal("expected CLI runner not to be called for supervised serving endpoint")
		return platformprocess.CommandResult{}, nil
	}))

	handle, err := runtime.Load(context.Background(), LoadRequest{
		Resource:        localModelFactoryConfig().Resources[0],
		Worker:          worker,
		ModelName:       "OMNIVOICE_Q4_K_M",
		CachePath:       cachePath,
		Revision:        "rev-test",
		Files:           files,
		ServingEndpoint: server.URL + "/health",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	response, err := handle.Invoke(context.Background(), InvocationRequest{
		Request: ModelInvocation{
			ModelOperation: "TTS",
			ModelBindings: []modelinference.ResolvedModelOperationBinding{{
				Slot:   "text",
				Source: "INPUT",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "hello supervised runtime",
				}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if gotPath != "/invoke" {
		t.Fatalf("invoke path = %q, want /invoke", gotPath)
	}
	if strings.TrimSpace(response.Content) == "" {
		t.Fatalf("response content is empty, want supervised invoke payload")
	}
}

func TestSupervisedInvokeURL_DerivesInvokePathFromHealthEndpoint(t *testing.T) {
	t.Parallel()
	if got := SupervisedInvokeURL("http://127.0.0.1:8080/health"); got != "http://127.0.0.1:8080/invoke" {
		t.Fatalf("invoke URL = %q, want http://127.0.0.1:8080/invoke", got)
	}
}
