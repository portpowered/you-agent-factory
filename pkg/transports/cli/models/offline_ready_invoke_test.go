package models

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"go.uber.org/zap"
)

type readyLocalModelAssetPuller struct {
	cache localmodels.CacheLayout
}

func (p readyLocalModelAssetPuller) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (apisurface.ModelPullResult, error) {
	return apisurface.ModelPullResult{
		ModelName:      p.cache.ModelName,
		ReadinessState: string(factoryapi.ManagedRuntimeReadinessStateREADY),
	}, nil
}

func (p readyLocalModelAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) error {
	return nil
}

func (p readyLocalModelAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) (localmodels.CacheLayout, error) {
	return p.cache, nil
}

func (p readyLocalModelAssetPuller) InspectRuntimeCache(context.Context, *factoryconfig.LoadedFactoryConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{
		Supported:          true,
		Installed:          true,
		CachePath:          p.cache.CachePath,
		Revision:           p.cache.Revision,
		InstalledFileCount: len(p.cache.Files),
	}, nil
}

type readyLocalModelRuntime struct {
	audioPath string
}

func (r *readyLocalModelRuntime) Supports(resource factoryresource.Config, worker *workerconfig.Config) bool {
	return localmodels.CanonicalBackendName(resource.Backend) == "LLAMACPP" &&
		localmodels.CanonicalModelName(worker.Model) == localmodels.CanonicalModelName("OMNIVOICE_Q4_K_M")
}

func (r *readyLocalModelRuntime) Load(context.Context, localmodels.LoadRequest) (localmodels.Handle, error) {
	return readyLocalModelHandle{audioPath: r.audioPath}, nil
}

type readyLocalModelHandle struct {
	audioPath string
}

func (h readyLocalModelHandle) Invoke(context.Context, localmodels.InvocationRequest) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{
		Content: mustOfflineReadyAudioContentResponse(h.audioPath),
	}, nil
}

func mustOfflineReadyAudioContentResponse(audioPath string) string {
	content := []work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
	}}
	body, err := json.Marshal(content)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func preserveModelsBootstrapGlobals(t *testing.T) {
	t.Helper()
	originalAugment := augmentModelsInvokeBootstrapServiceConfig
	originalBuilder := buildModelInvocationBootstrap
	t.Cleanup(func() {
		augmentModelsInvokeBootstrapServiceConfig = originalAugment
		buildModelInvocationBootstrap = originalBuilder
	})
}

func installOfflineReadyLocalModelBootstrapFixture(t *testing.T) (factoryDir string, audioPath string) {
	t.Helper()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	audioPath = filepath.Join(t.TempDir(), "speech.wav")
	if err := os.WriteFile(audioPath, []byte("RIFF....WAVE"), 0o644); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}

	cache := localmodels.CacheLayout{
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: filepath.Join(t.TempDir(), "cache", "rev-test"),
		Revision:  "rev-test",
		Files: []string{
			"/models/omnivoice-base-Q4_K_M.gguf",
			"/models/omnivoice-tokenizer-Q4_K_M.gguf",
		},
	}
	puller := readyLocalModelAssetPuller{cache: cache}
	runtime := &readyLocalModelRuntime{audioPath: audioPath}

	factoryDir = t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, offlineReadyLocalModelFactoryConfig(healthServer.URL))

	augmentModelsInvokeBootstrapServiceConfig = func(cfg *service.FactoryServiceConfig) {
		if err := service.ApplyInvocationBootstrapLocalModelTestFixture(cfg, healthServer.URL, runtime, puller); err != nil {
			t.Fatalf("ApplyInvocationBootstrapLocalModelTestFixture: %v", err)
		}
		cfg.MockWorkersConfig = factoryconfig.NewEmptyMockWorkersConfig()
	}

	return factoryDir, audioPath
}

func offlineReadyLocalModelFactoryConfig(healthEndpoint string) map[string]any {
	worker := map[string]any{
		"name":          "voice-local",
		"type":          interfaces.WorkerTypeModel,
		"modelProvider": "CODEX",
		"model":         "OMNIVOICE_Q4_K_M",
		"modelLocality": workerconfig.ModelLocalityLocal,
		"resources":     []map[string]any{{"name": "omnivoice-cache", "capacity": 1}},
		"operations": []map[string]any{{
			"name": "TTS",
			"inputs": []map[string]any{{
				"name":         "text",
				"contentTypes": []string{workerconfig.ModelOperationContentTypeText},
				"required":     true,
			}},
			"outputs": []map[string]any{{
				"name":         "audio",
				"contentTypes": []string{workerconfig.ModelOperationContentTypeAudio},
			}},
		}},
	}
	if endpoint := strings.TrimSpace(healthEndpoint); endpoint != "" {
		worker["command"] = localmodels.DefaultOmniVoiceCommand
		worker["args"] = []string{"--health-endpoint", endpoint}
	}
	return map[string]any{
		"name": "factory",
		"resources": []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       factoryresource.TypeModel,
			"capacity":   1,
			"model":      "OMNIVOICE_Q4_K_M",
			"backend":    "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{worker},
	}
}

func TestInvoke_OfflineReadyLocalFixture_JSONMetadataSucceedsWithoutHTTPServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for offline ready local model bootstrap invoke")
	}

	preserveModelsBootstrapGlobals(t)
	factoryDir, _ := installOfflineReadyLocalModelBootstrapFixture(t)

	var out bytes.Buffer
	if err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "hello offline",
		FactoryDir:      factoryDir,
		Server:          failureBaselineUnreachableServer,
		JSON:            true,
		Output:          &out,
		Logger:          zap.NewNop(),
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var response factoryapi.ModelInvocationResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON response: %v\n%s", err, out.String())
	}
	if response.ModelName != "OMNIVOICE_Q4_K_M" || response.Operation != "TTS" {
		t.Fatalf("response identity = %#v, want OMNIVOICE_Q4_K_M TTS", response)
	}
	if response.Worker != "voice-local" {
		t.Fatalf("worker = %q, want voice-local", response.Worker)
	}
	if len(response.Content) == 0 {
		t.Fatal("expected invocation content in JSON metadata response")
	}
	if strings.Contains(out.String(), "models endpoint not reachable") {
		t.Fatalf("output = %q, want bootstrap success instead of HTTP transport failure", out.String())
	}
}

func TestInvoke_OfflineReadyLocalFixture_AudioOutputSucceedsWithoutHTTPServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for offline ready local model bootstrap invoke")
	}

	preserveModelsBootstrapGlobals(t)
	factoryDir, audioPath := installOfflineReadyLocalModelBootstrapFixture(t)

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	var out bytes.Buffer
	if err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "hello offline",
		OutputPath:      outputPath,
		FactoryDir:      factoryDir,
		Server:          failureBaselineUnreachableServer,
		Output:          &out,
		Logger:          zap.NewNop(),
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output audio: %v", err)
	}
	want, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("read fixture audio: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("output audio = %q, want fixture audio %q", string(got), string(want))
	}
	if !strings.Contains(out.String(), "Wrote audio:") {
		t.Fatalf("stdout = %q, want audio write confirmation", out.String())
	}
}
