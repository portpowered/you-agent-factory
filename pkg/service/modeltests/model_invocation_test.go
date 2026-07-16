package modeltests

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"go.uber.org/zap"
)

type providerCallRecorder struct {
	mu        sync.Mutex
	calls     []workerexecution.ProviderInferenceRequest
	responses []workerexecution.InferenceResponse
}

func (p *providerCallRecorder) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, workerexecution.CloneProviderInferenceRequest(req))
	if len(p.responses) == 0 {
		return workerexecution.InferenceResponse{Content: "ok"}, nil
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func (p *providerCallRecorder) Calls() []workerexecution.ProviderInferenceRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	calls := make([]workerexecution.ProviderInferenceRequest, len(p.calls))
	for i, call := range p.calls {
		calls[i] = workerexecution.CloneProviderInferenceRequest(call)
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
	if err == nil || !apisurface.IsManagedRuntimeMissing(err) {
		t.Fatalf("InvokeModel error = %v, want managed runtime missing", err)
	}
	failure, ok := apisurface.AsInferenceFailure(err)
	if !ok || failure.Class != apisurface.InferenceFailureClassMissingModel {
		t.Fatalf("InvokeModel error = %T, want missing_model InferenceFailure", err)
	}
	var readinessErr *apisurface.ManagedRuntimeInvocationError
	if !errors.As(err, &readinessErr) && !errors.Is(err, apisurface.ErrManagedRuntimeMissing) {
		t.Fatalf("InvokeModel error = %v, want managed runtime missing cause", err)
	}
	if readinessErr != nil && readinessErr.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING {
		t.Fatalf("readinessState = %s, want MISSING", readinessErr.ReadinessState)
	}
}

func TestInvokeModel_ReturnsCanonicalContentAndBindings(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	provider := &providerCallRecorder{
		responses: []workerexecution.InferenceResponse{{
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
	if len(result.Bindings) != 1 || result.Bindings[0].Slot != "text" || result.Bindings[0].Source != workerexecution.ModelOperationBindingSourceInput {
		t.Fatalf("bindings = %#v, want one input binding", result.Bindings)
	}
	if len(result.Content) != 1 || result.Content[0].Type != work.WorkContentPartTypeAudio || result.StreamFile != audioPath || result.StreamContentType != "audio/wav" {
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
	return attachModelService(t, svc)
}

func cloudModelInvocationConfig() map[string]any {
	cfg := modelCatalogConfig(false)
	cfg["workers"] = []map[string]any{{
		"name":          "tts-worker",
		"type":          interfaces.WorkerTypeModel,
		"model":         "OMNIVOICE_Q4_K_M",
		"modelProvider": "CODEX",
		"modelLocality": workerconfig.ModelLocalityCloud,
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
	body, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
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
