package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
