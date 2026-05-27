package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service/localmodel"
	"github.com/portpowered/infinite-you/pkg/service/modelassets"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestPullModel_DownloadsManagedCacheAssets(t *testing.T) {
	baseBytes := []byte("base-gguf")
	tokenizerBytes := []byte("tokenizer-gguf")
	baseSHA := sha256HexString(baseBytes)
	tokenizerSHA := sha256HexString(tokenizerBytes)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models/Serveurperso/OmniVoice-GGUF":
			_, _ = io.WriteString(w, fmt.Sprintf(`{"sha":"rev-test","siblings":[{"rfilename":"omnivoice-base-Q4_K_M.gguf","size":%d,"lfs":{"oid":"%s","size":%d}},{"rfilename":"omnivoice-tokenizer-Q4_K_M.gguf","size":%d,"lfs":{"oid":"%s","size":%d}}]}`, len(baseBytes), baseSHA, len(baseBytes), len(tokenizerBytes), tokenizerSHA, len(tokenizerBytes)))
		case "/Serveurperso/OmniVoice-GGUF/resolve/rev-test/omnivoice-base-Q4_K_M.gguf":
			_, _ = w.Write(baseBytes)
		case "/Serveurperso/OmniVoice-GGUF/resolve/rev-test/omnivoice-tokenizer-Q4_K_M.gguf":
			_, _ = w.Write(tokenizerBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	puller := newModelAssetPullerForTest(t.TempDir())
	puller.SetEndpointsForTest(server.URL, server.URL+"/api", server.Client())

	runtimeCfg := mustLoadedFactoryConfigForModelPullTest(t, &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	})

	result, err := puller.PullModel(context.Background(), runtimeCfg, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if result.Outcome != modelassets.PullOutcomePulled || result.Revision != "rev-test" {
		t.Fatalf("result = %#v, want pulled rev-test", result)
	}
	for _, path := range []string{
		filepath.Join(result.CachePath, "omnivoice-base-Q4_K_M.gguf"),
		filepath.Join(result.CachePath, "omnivoice-tokenizer-Q4_K_M.gguf"),
	} {
		if _, err := testFileSHA256(path); err != nil {
			t.Fatalf("expected cached file %q: %v", path, err)
		}
	}
	if err := puller.EnsureModelAvailable(context.Background(), runtimeCfg, &interfaces.WorkerConfig{
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: interfaces.ModelLocalityLocal,
	}); err != nil {
		t.Fatalf("EnsureModelAvailable: %v", err)
	}
}

func mustLoadedFactoryConfigForModelPullTest(t *testing.T, cfg *interfaces.FactoryConfig) *factoryconfig.LoadedFactoryConfig {
	t.Helper()
	loaded, err := factoryconfig.NewLoadedFactoryConfig("factory-dir", cfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

func newModelAssetPullerForTest(cacheDir string) *modelassets.Puller {
	return modelassets.NewPuller(cacheDir, runtime.GOOS, runtime.GOARCH)
}

func TestPullModel_ResolveModelCacheUsesPersistedMetadataOffline(t *testing.T) {
	baseBytes := []byte("base-gguf")
	tokenizerBytes := []byte("tokenizer-gguf")
	baseSHA := sha256HexString(baseBytes)
	tokenizerSHA := sha256HexString(tokenizerBytes)
	manifestRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models/Serveurperso/OmniVoice-GGUF":
			manifestRequests++
			_, _ = io.WriteString(w, fmt.Sprintf(`{"sha":"rev-test","siblings":[{"rfilename":"omnivoice-base-Q4_K_M.gguf","size":%d,"lfs":{"oid":"%s","size":%d}},{"rfilename":"omnivoice-tokenizer-Q4_K_M.gguf","size":%d,"lfs":{"oid":"%s","size":%d}}]}`, len(baseBytes), baseSHA, len(baseBytes), len(tokenizerBytes), tokenizerSHA, len(tokenizerBytes)))
		case "/Serveurperso/OmniVoice-GGUF/resolve/rev-test/omnivoice-base-Q4_K_M.gguf":
			_, _ = w.Write(baseBytes)
		case "/Serveurperso/OmniVoice-GGUF/resolve/rev-test/omnivoice-tokenizer-Q4_K_M.gguf":
			_, _ = w.Write(tokenizerBytes)
		default:
			http.NotFound(w, r)
		}
	}))

	cacheDir := t.TempDir()
	puller := newModelAssetPullerForTest(cacheDir)
	puller.SetEndpointsForTest(server.URL, server.URL+"/api", server.Client())

	runtimeCfg := mustLoadedFactoryConfigForModelPullTest(t, &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	})
	worker := &interfaces.WorkerConfig{
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: interfaces.ModelLocalityLocal,
	}

	result, err := puller.PullModel(context.Background(), runtimeCfg, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	server.Close()

	layout, err := puller.ResolveModelCache(context.Background(), runtimeCfg, worker)
	if err != nil {
		t.Fatalf("ResolveModelCache after pull with offline manifest: %v", err)
	}
	if err := puller.EnsureModelAvailable(context.Background(), runtimeCfg, worker); err != nil {
		t.Fatalf("EnsureModelAvailable after pull with offline manifest: %v", err)
	}
	if manifestRequests != 1 {
		t.Fatalf("manifest requests = %d, want 1 during pull only", manifestRequests)
	}
	if layout.CachePath != result.CachePath || layout.Revision != "rev-test" || len(layout.Files) != 2 {
		t.Fatalf("layout = %#v, want pulled cache path and revision", layout)
	}
}

func TestPullModel_RetriesManifestLookupAfterDNSError(t *testing.T) {
	baseBytes := []byte("base-gguf")
	tokenizerBytes := []byte("tokenizer-gguf")
	baseSHA := sha256HexString(baseBytes)
	tokenizerSHA := sha256HexString(tokenizerBytes)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models/Serveurperso/OmniVoice-GGUF":
			_, _ = io.WriteString(w, fmt.Sprintf(`{"sha":"rev-test","siblings":[{"rfilename":"omnivoice-base-Q4_K_M.gguf","size":%d,"lfs":{"oid":"%s","size":%d}},{"rfilename":"omnivoice-tokenizer-Q4_K_M.gguf","size":%d,"lfs":{"oid":"%s","size":%d}}]}`, len(baseBytes), baseSHA, len(baseBytes), len(tokenizerBytes), tokenizerSHA, len(tokenizerBytes)))
		case "/Serveurperso/OmniVoice-GGUF/resolve/rev-test/omnivoice-base-Q4_K_M.gguf":
			_, _ = w.Write(baseBytes)
		case "/Serveurperso/OmniVoice-GGUF/resolve/rev-test/omnivoice-tokenizer-Q4_K_M.gguf":
			_, _ = w.Write(tokenizerBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	puller := newModelAssetPullerForTest(t.TempDir())
	puller.SetEndpointsForTest(server.URL, server.URL+"/api", &http.Client{
		Transport: &manifestRetryRoundTripper{
			base: server.Client().Transport,
		},
	})

	runtimeCfg := mustLoadedFactoryConfigForModelPullTest(t, &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	})

	result, err := puller.PullModel(context.Background(), runtimeCfg, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel with manifest retry: %v", err)
	}
	if result.Revision != "rev-test" || len(result.DownloadedFiles) != 2 {
		t.Fatalf("result = %#v, want rev-test with both managed files", result)
	}
}

func TestPullModel_ReturnsUnsupportedWhenRuntimeHasNoMatchingModelResource(t *testing.T) {
	puller := newModelAssetPullerForTest(t.TempDir())
	runtimeCfg := mustLoadedFactoryConfigForModelPullTest(t, &interfaces.FactoryConfig{})
	_, err := puller.PullModel(context.Background(), runtimeCfg, "OMNIVOICE_Q4_K_M")
	if err == nil || !strings.Contains(err.Error(), apisurface.ErrModelPullUnsupported.Error()) {
		t.Fatalf("PullModel error = %v, want unsupported", err)
	}
}

func TestInvokeModel_ReturnsModelNotAvailableWhenManagedCacheIsMissing(t *testing.T) {
	puller := newModelAssetPullerForTest(t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/models/Serveurperso/OmniVoice-GGUF" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"sha":"rev-test","siblings":[{"rfilename":"omnivoice-base-Q4_K_M.gguf","size":10,"lfs":{"oid":"abc","size":10}},{"rfilename":"omnivoice-tokenizer-Q4_K_M.gguf","size":10,"lfs":{"oid":"def","size":10}}]}`)
	}))
	defer server.Close()
	puller.SetEndpointsForTest(server.URL, server.URL+"/api", server.Client())

	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workers: []interfaces.WorkerConfig{{Name: "tts-worker"}},
	}, map[string]*interfaces.WorkerConfig{
		"tts-worker": {
			Name:          "tts-worker",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelProvider: interfaces.RunnerIDCodex,
			ModelLocality: interfaces.ModelLocalityLocal,
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
		},
	}, nil)
	svc := &FactoryService{
		runtimeCfg:  runtimeCfg,
		cfg:         &FactoryServiceConfig{},
		modelAssets: modelAssetPullerAdapter{inner: puller},
	}
	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello world"),
		},
	})
	if err == nil || !strings.Contains(err.Error(), apisurface.ErrModelNotAvailable.Error()) {
		t.Fatalf("InvokeModel error = %v, want model not available", err)
	}
}

func sha256HexString(input []byte) string {
	sum := sha256.Sum256(input)
	return hex.EncodeToString(sum[:])
}

func testFileSHA256(path string) (string, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, input); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

type manifestRetryRoundTripper struct {
	base http.RoundTripper
	mu   sync.Mutex
	hit  bool
}

func (rt *manifestRetryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	shouldFail := !rt.hit && req != nil && req.URL != nil && req.URL.Path == "/api/models/Serveurperso/OmniVoice-GGUF"
	if shouldFail {
		rt.hit = true
	}
	rt.mu.Unlock()
	if shouldFail {
		return nil, &net.DNSError{Err: "lookup huggingface.co: no such host", Name: "huggingface.co", IsNotFound: true}
	}
	return rt.base.RoundTrip(req)
}

type staticModelAssetPuller struct {
	pullResult apisurface.ModelPullResult
	pullErr    error
	ensureErr  error
	cache      localModelCacheLayout
	cacheErr   error
}

func (s staticModelAssetPuller) PullModel(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ string) (apisurface.ModelPullResult, error) {
	return s.pullResult, s.pullErr
}

func (s staticModelAssetPuller) EnsureModelAvailable(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ *interfaces.WorkerConfig) error {
	return s.ensureErr
}

func (s staticModelAssetPuller) ResolveModelCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ *interfaces.WorkerConfig) (localModelCacheLayout, error) {
	return s.cache, s.cacheErr
}

func TestInvokeModel_ReturnsCanonicalContentAndBindings(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	provider := &providerCallRecorder{
		responses: []interfaces.InferenceResponse{{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		}},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{
		Workers: []interfaces.WorkerConfig{{Name: "tts-worker"}},
	}, map[string]*interfaces.WorkerConfig{
		"tts-worker": {
			Name:          "tts-worker",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelProvider: interfaces.RunnerIDCodex,
			ModelLocality: interfaces.ModelLocalityLocal,
			Operations: []interfaces.ModelOperation{{
				Name: "TTS",
				Inputs: []interfaces.ModelOperationSlot{
					{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}, Required: true},
				},
				Outputs: []interfaces.ModelOperationSlot{{
					Name:         "audio",
					ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		},
	}, nil)
	svc := &FactoryService{
		runtimeCfg:  runtimeCfg,
		cfg:         &FactoryServiceConfig{ProviderOverride: provider},
		modelAssets: staticModelAssetPuller{},
	}

	mode := factoryapi.AUDIOSTREAM
	result, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello world"),
		},
		Options: &factoryapi.ModelInvocationOptions{ResponseMode: &mode},
	})
	if err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	if result.ModelName != "OMNIVOICE_Q4_K_M" || result.Worker != "tts-worker" {
		t.Fatalf("result identity = %#v, want OMNIVOICE tts-worker", result)
	}
	if len(result.Bindings) != 1 || result.Bindings[0].Slot != "text" || result.Bindings[0].Source != interfaces.ModelOperationBindingSourceInput {
		t.Fatalf("bindings = %#v, want one input binding", result.Bindings)
	}
	if len(result.Content) != 1 || result.Content[0].Type != interfaces.WorkContentPartTypeAudio || result.StreamFile != audioPath || result.StreamContentType != "audio/wav" {
		t.Fatalf("result content = %#v stream=%q type=%q, want audio output", result.Content, result.StreamFile, result.StreamContentType)
	}

	calls := provider.Calls()
	if len(calls) != 1 || calls[0].ModelOperation != "TTS" || len(calls[0].ModelBindings) != 1 || calls[0].ModelBindings[0].Content[0].Text != "hello world" {
		t.Fatalf("provider calls = %#v, want one TTS call with resolved text binding", calls)
	}
}

func TestModelInvocationExecutor_UsesCanonicalModelExecutorPath(t *testing.T) {
	provider := &providerCallRecorder{
		responses: []interfaces.InferenceResponse{{
			Content: "executor-output",
		}},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{
		Workers: []interfaces.WorkerConfig{{Name: "tts-worker"}},
	}, map[string]*interfaces.WorkerConfig{
		"tts-worker": {
			Name:          "tts-worker",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelProvider: interfaces.RunnerIDCodex,
			ModelLocality: interfaces.ModelLocalityLocal,
			Body:          "You are a TTS worker.",
			Operations: []interfaces.ModelOperation{{
				Name: "TTS",
				Inputs: []interfaces.ModelOperationSlot{
					{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}, Required: true},
				},
			}},
		},
	}, nil)
	svc := &FactoryService{
		cfg: &FactoryServiceConfig{ProviderOverride: provider},
	}

	executor, err := svc.modelInvocationExecutor(runtimeCfg, runtimeCfg.FactoryConfig(), "tts-worker")
	if err != nil {
		t.Fatalf("modelInvocationExecutor: %v", err)
	}

	result, err := executor.Execute(context.Background(), interfaces.WorkstationExecutionRequest{
		Dispatch: interfaces.WorkDispatch{
			DispatchID:      "direct-model-invocation",
			TransitionID:    "direct-model-invocation",
			WorkerType:      "tts-worker",
			WorkstationName: "direct-model-invocation",
		},
		WorkerType:            "tts-worker",
		WorkstationType:       "direct-model-invocation",
		RunnerID:              interfaces.RunnerIDCodex,
		RunnerSelectionSource: interfaces.RunnerSelectionSourceDefault,
		ModelOperation:        "TTS",
		ModelBindings: []interfaces.ResolvedModelOperationBinding{{
			Slot:   "text",
			Source: interfaces.ModelOperationBindingSourceInput,
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "hello world",
				Slot: "text",
			}},
		}},
		SystemPrompt: "You are a TTS worker.",
		UserMessage:  `{"operation":"TTS"}`,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted || result.Output != "executor-output" {
		t.Fatalf("execution result = %#v, want accepted executor-output", result)
	}

	calls := provider.Calls()
	if len(calls) != 1 {
		t.Fatalf("provider call count = %d, want 1", len(calls))
	}
	if calls[0].ModelOperation != "TTS" || len(calls[0].ModelBindings) != 1 || calls[0].ModelBindings[0].Slot != "text" {
		t.Fatalf("provider call = %#v, want one resolved TTS binding", calls[0])
	}
	if got := calls[0].ModelBindings[0].Content[0].Text; got != "hello world" {
		t.Fatalf("provider binding text = %q, want hello world", got)
	}
}

func TestInvokeModel_ReturnsNotFoundWhenModelDoesNotExist(t *testing.T) {
	svc := &FactoryService{
		runtimeCfg: newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{}, nil, nil),
		cfg:        &FactoryServiceConfig{},
	}
	_, err := svc.InvokeModel(context.Background(), "MISSING", factoryapi.ModelInvocationRequest{Operation: "TTS"})
	if err == nil || !errors.Is(err, apisurface.ErrModelNotFound) {
		t.Fatalf("InvokeModel error = %v, want ErrModelNotFound", err)
	}
}

func mustGeneratedServiceTextPart(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
		Slot: stringPtr("text"),
	}); err != nil {
		t.Fatalf("build text content part: %v", err)
	}
	return part
}

func stringPtr(value string) *string {
	return &value
}

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
	runtime := newOmniVoiceLocalRuntime(localModelCommandRunnerFunc(func(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
		got = workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(req))
		outputPath := commandArgValue(t, req.Args, "--output")
		if err := os.WriteFile(outputPath, []byte("RIFF"), 0o644); err != nil {
			t.Fatalf("write output audio: %v", err)
		}
		return workers.CommandResult{}, nil
	}))

	handle, err := runtime.Load(context.Background(), localModelLoadRequest{
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
	response, err := handle.Invoke(context.Background(), localModelInvocationRequest{
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
	var payload localmodel.OmniVoiceInvocationPayload
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
	if content[0].Type != interfaces.WorkContentPartTypeAudio || content[0].ContentType != localmodel.OmniVoiceAudioContentType || strings.TrimSpace(content[0].File) == "" {
		t.Fatalf("content = %#v, want AUDIO content with output file", content)
	}
	if _, err := os.Stat(content[0].File); err != nil {
		t.Fatalf("stat audio output %q: %v", content[0].File, err)
	}
}

func TestOmniVoiceLocalRuntime_Invoke_RequiresResolvedTextSlot(t *testing.T) {
	cachePath, files := writeOmniVoiceCacheFiles(t)
	runtime := newOmniVoiceLocalRuntime(localModelCommandRunnerFunc(func(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
		t.Fatal("expected command runner not to be called")
		return workers.CommandResult{}, nil
	}))

	handle, err := runtime.Load(context.Background(), localModelLoadRequest{
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

	_, err = handle.Invoke(context.Background(), localModelInvocationRequest{
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
	runtime := newOmniVoiceLocalRuntime(localModelCommandRunnerFunc(func(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
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

	handle, err := runtime.Load(context.Background(), localModelLoadRequest{
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

	_, err = handle.Invoke(context.Background(), localModelInvocationRequest{
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
	worker := *localModelRuntimeWorkers()["tts-worker"]
	worker.Resources = append([]interfaces.ResourceConfig(nil), worker.Resources...)
	worker.Operations = append([]interfaces.ModelOperation(nil), worker.Operations...)
	return &worker
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

func TestOmniVoiceLocalRuntime_LoadRejectsMissingTokenizerAsset(t *testing.T) {
	cachePath := t.TempDir()
	modelPath := filepath.Join(cachePath, "omnivoice-base-Q4_K_M.gguf")
	if err := os.WriteFile(modelPath, []byte("gguf"), 0o644); err != nil {
		t.Fatalf("write model file: %v", err)
	}
	runtime := newOmniVoiceLocalRuntime(nil)
	_, err := runtime.Load(context.Background(), localModelLoadRequest{
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
	runtime := newOmniVoiceLocalRuntime(localModelCommandRunnerFunc(func(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
		return workers.CommandResult{}, errors.New("runner crashed")
	}))
	handle, err := runtime.Load(context.Background(), localModelLoadRequest{
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
	_, err = handle.Invoke(context.Background(), localModelInvocationRequest{
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
