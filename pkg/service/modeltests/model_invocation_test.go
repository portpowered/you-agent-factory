package modeltests

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

type providerCallRecorder struct {
	mu        sync.Mutex
	calls     []interfaces.ProviderInferenceRequest
	responses []interfaces.InferenceResponse
}

func (p *providerCallRecorder) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, interfaces.CloneProviderInferenceRequest(req))
	if len(p.responses) == 0 {
		return interfaces.InferenceResponse{Content: "ok"}, nil
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func (p *providerCallRecorder) Calls() []interfaces.ProviderInferenceRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	calls := make([]interfaces.ProviderInferenceRequest, len(p.calls))
	for i, call := range p.calls {
		calls[i] = interfaces.CloneProviderInferenceRequest(call)
	}
	return calls
}

func TestInvokeModel_ReturnsModelNotAvailableWhenManagedCacheIsMissing(t *testing.T) {
	cacheDir := t.TempDir()
	writeManagedCacheMetadata(t, cacheDir)
	svc := buildModelCatalogServiceWithOptions(t, modelCatalogConfig(true), service.FactoryServiceConfig{
		ModelCacheDir: cacheDir,
	})

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

func TestInvokeModel_ReturnsCanonicalContentAndBindings(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	provider := &providerCallRecorder{
		responses: []interfaces.InferenceResponse{{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		}},
	}
	svc := buildModelCatalogServiceWithOptions(t, cloudModelInvocationConfig(), service.FactoryServiceConfig{
		ProviderOverride: provider,
	})

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

func TestInvokeModel_ReturnsNotFoundWhenModelDoesNotExist(t *testing.T) {
	svc := buildModelCatalogServiceWithOptions(t, map[string]any{"name": "factory"}, service.FactoryServiceConfig{})
	_, err := svc.InvokeModel(context.Background(), "MISSING", factoryapi.ModelInvocationRequest{Operation: "TTS"})
	if err == nil || !errors.Is(err, apisurface.ErrModelNotFound) {
		t.Fatalf("InvokeModel error = %v, want ErrModelNotFound", err)
	}
}

func buildModelCatalogServiceWithOptions(t *testing.T, cfg map[string]any, options service.FactoryServiceConfig) *service.FactoryService {
	t.Helper()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, cfg)
	options.Dir = dir
	options.MockWorkersConfig = factoryconfig.NewEmptyMockWorkersConfig()
	options.Logger = zap.NewNop()
	svc, err := service.BuildFactoryService(context.Background(), &options)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	return svc
}

func cloudModelInvocationConfig() map[string]any {
	cfg := modelCatalogConfig(false)
	cfg["workers"] = []map[string]any{{
		"name":          "tts-worker",
		"type":          interfaces.WorkerTypeModel,
		"model":         "OMNIVOICE_Q4_K_M",
		"modelProvider": "CODEX",
		"modelLocality": interfaces.ModelLocalityCloud,
		"operations": []map[string]any{{
			"name": "TTS",
			"inputs": []map[string]any{{
				"name":         "text",
				"contentTypes": []string{interfaces.ModelOperationContentTypeText},
				"required":     true,
			}},
			"outputs": []map[string]any{{
				"name":         "audio",
				"contentTypes": []string{interfaces.ModelOperationContentTypeAudio},
			}},
		}},
	}}
	return cfg
}

func writeManagedCacheMetadata(t *testing.T, cacheDir string) {
	t.Helper()
	modelDir := filepath.Join(cacheDir, "OMNIVOICE_Q4_K_M")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("create model cache dir: %v", err)
	}
	body := []byte(`{"modelName":"OMNIVOICE_Q4_K_M","revision":"rev-test","files":[{"path":"omnivoice-base-Q4_K_M.gguf"},{"path":"omnivoice-tokenizer-Q4_K_M.gguf"}]}`)
	if err := os.WriteFile(filepath.Join(modelDir, ".managed-cache.json"), body, 0o644); err != nil {
		t.Fatalf("write managed cache metadata: %v", err)
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

func mustMarshalAudioContentResponse(t *testing.T, audioPath string) string {
	t.Helper()
	body, err := json.Marshal([]interfaces.WorkContentPart{{
		Type:        interfaces.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
	}})
	if err != nil {
		t.Fatalf("marshal audio content: %v", err)
	}
	return string(body)
}

func stringPtr(value string) *string {
	return &value
}
