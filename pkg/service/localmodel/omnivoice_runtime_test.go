package localmodel

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type localModelCommandRunnerFunc func(context.Context, workers.CommandRequest) (workers.CommandResult, error)

func (fn localModelCommandRunnerFunc) Run(ctx context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	return fn(ctx, req)
}

func TestOmniVoiceLocalRuntime_LoadAndInvoke_ReturnsAudioContentFromOutputFile(t *testing.T) {
	cachePath, files := writeOmniVoiceCacheFiles(t)
	worker := cloneLocalModelRuntimeWorker()
	worker.Command = "omnivoice-test"
	worker.Args = []string{"--backend", "llamacpp"}

	var got workers.CommandRequest
	runtime := NewOmniVoiceRuntime(localModelCommandRunnerFunc(func(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
		got = workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(req))
		outputPath := commandArgValue(t, req.Args, "--output")
		if err := os.WriteFile(outputPath, []byte("RIFF"), 0o644); err != nil {
			t.Fatalf("write output audio: %v", err)
		}
		return workers.CommandResult{}, nil
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
		Request: interfaces.RunnerExecutionRequest{
			Dispatch: interfaces.WorkDispatch{
				DispatchID:      "dispatch-1",
				TransitionID:    "transition-1",
				WorkerType:      "tts-worker",
				WorkstationName: "speak",
			},
			ModelOperation:   "TTS",
			WorkingDirectory: workdir,
			ModelBindings: []interfaces.ResolvedModelOperationBinding{{
				Slot:   "text",
				Source: interfaces.ModelOperationBindingSourceInput,
				Content: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeText,
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
	cachePath, files := writeOmniVoiceCacheFiles(t)
	runtime := NewOmniVoiceRuntime(localModelCommandRunnerFunc(func(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
		t.Fatal("expected command runner not to be called")
		return workers.CommandResult{}, nil
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
		Request: interfaces.RunnerExecutionRequest{
			ModelOperation: "TTS",
		},
	})
	if err == nil || !strings.Contains(err.Error(), `requires resolved slot "text"`) {
		t.Fatalf("Invoke error = %v, want missing text slot error", err)
	}
}

func TestOmniVoiceLocalRuntime_Invoke_PropagatesRuntimeExitFailure(t *testing.T) {
	cachePath, files := writeOmniVoiceCacheFiles(t)
	runtime := NewOmniVoiceRuntime(localModelCommandRunnerFunc(func(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
		outputPath := commandArgValue(t, req.Args, "--output")
		if err := os.WriteFile(outputPath, []byte(""), 0o644); err != nil {
			t.Fatalf("seed output file: %v", err)
		}
		return workers.CommandResult{
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
		Request: interfaces.RunnerExecutionRequest{
			ModelOperation: "TTS",
			ModelBindings: []interfaces.ResolvedModelOperationBinding{{
				Slot:   "text",
				Source: interfaces.ModelOperationBindingSourceInput,
				Content: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeText,
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
	cachePath := t.TempDir()
	modelPath := filepath.Join(cachePath, "omnivoice-base-Q4_K_M.gguf")
	if err := os.WriteFile(modelPath, []byte("gguf"), 0o644); err != nil {
		t.Fatalf("write model file: %v", err)
	}
	runtime := NewOmniVoiceRuntime(nil)
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
	cachePath, files := writeOmniVoiceCacheFiles(t)
	runtime := NewOmniVoiceRuntime(localModelCommandRunnerFunc(func(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
		return workers.CommandResult{}, errors.New("runner crashed")
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
		Request: interfaces.RunnerExecutionRequest{
			ModelOperation: "TTS",
			ModelBindings: []interfaces.ResolvedModelOperationBinding{{
				Slot:   "text",
				Source: interfaces.ModelOperationBindingSourceInput,
				Content: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeText,
					Text: "hello",
				}},
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "runner crashed") {
		t.Fatalf("Invoke error = %v, want runner error", err)
	}
}

func assertOmniVoiceCommandRequest(t *testing.T, got workers.CommandRequest, workdir string, files []string) {
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
	var content []interfaces.WorkContentPart
	if err := json.Unmarshal([]byte(responseContent), &content); err != nil {
		t.Fatalf("decode response content: %v", err)
	}
	if len(content) != 1 {
		t.Fatalf("content count = %d, want 1", len(content))
	}
	if content[0].Type != interfaces.WorkContentPartTypeAudio || content[0].ContentType != OmniVoiceAudioContentType || strings.TrimSpace(content[0].File) == "" {
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

func cloneLocalModelRuntimeWorker() *interfaces.WorkerConfig {
	worker := localModelRuntimeWorker()
	worker.Resources = append([]interfaces.ResourceConfig(nil), worker.Resources...)
	worker.Operations = append([]interfaces.ModelOperation(nil), worker.Operations...)
	return &worker
}

func localModelFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	}
}

func localModelRuntimeWorker() interfaces.WorkerConfig {
	return interfaces.WorkerConfig{
		Name:          "tts-worker",
		Type:          interfaces.WorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelProvider: interfaces.RunnerIDCodex,
		ModelLocality: interfaces.ModelLocalityLocal,
		Resources: []interfaces.ResourceConfig{{
			Name:     "omnivoice-cache",
			Capacity: 1,
		}},
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{interfaces.ModelOperationContentTypeText},
				Required:     true,
			}},
			Outputs: []interfaces.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
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
