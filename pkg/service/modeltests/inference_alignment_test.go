package modeltests

import (
	"context"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/service"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerinference "github.com/portpowered/infinite-you/pkg/workers/inference"
)

func TestInvokeModel_UsesSharedInferenceBindingAndOutputShaping(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	providerRaw := mustMarshalAudioContentResponse(t, audioPath)
	provider := &providerCallRecorder{
		responses: []workerexecution.InferenceResponse{{Content: providerRaw}},
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

	operation := workerconfig.ModelOperation{
		Name: "TTS",
		Outputs: []workerconfig.ModelOperationSlot{{
			Name:         "audio",
			ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio},
		}},
	}
	shaped, err := workerinference.WorkContentFromInferenceOutput(providerRaw, operation)
	if err != nil {
		t.Fatalf("WorkContentFromInferenceOutput: %v", err)
	}
	if len(result.Content) != len(shaped) || result.Content[0].File != shaped[0].File {
		t.Fatalf("InvokeModel content = %#v, want shared shaping %#v", result.Content, shaped)
	}
	if len(provider.Calls()) != 1 || len(provider.Calls()[0].ModelBindings) != 1 {
		t.Fatalf("provider call = %#v, want one resolved binding", provider.Calls())
	}
}

func TestInvokeModel_InferenceAndLegacyBindingFixturesStayAligned(t *testing.T) {
	worker := inferenceBindingWorkerFixture()
	inputTokens := []factorytoken.Token{{
		ID: "token-tts",
		Color: factorytoken.Color{
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Slot: "text",
				Text: "hello world",
			}},
		},
	}}

	inferenceBindings, err := workerinference.ResolveInferenceOperationBindings(
		workerinference.DirectInferenceWorkstationConfig("TTS", nil),
		worker,
		inputTokens,
	)
	if err != nil {
		t.Fatalf("ResolveInferenceOperationBindings inference run: %v", err)
	}
	legacyBindings, err := workerinference.ResolveInferenceOperationBindings(
		&interfaces.FactoryWorkstationConfig{
			Type:      interfaces.WorkstationTypeInvoke,
			Operation: "TTS",
		},
		worker,
		inputTokens,
	)
	if err != nil {
		t.Fatalf("ResolveInferenceOperationBindings legacy model invoke: %v", err)
	}
	if len(inferenceBindings) != len(legacyBindings) || inferenceBindings[0].Content[0].Text != legacyBindings[0].Content[0].Text {
		t.Fatalf("inference = %#v legacy = %#v, want aligned binding resolution", inferenceBindings, legacyBindings)
	}
}

func inferenceBindingWorkerFixture() *workerconfig.Config {
	return &workerconfig.Config{
		Name: "tts-worker",
		Type: interfaces.WorkerTypeInference,
		Operations: []workerconfig.ModelOperation{{
			Name: "TTS",
			Inputs: []workerconfig.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{workerconfig.ModelOperationContentTypeText},
				Required:     true,
			}},
			Outputs: []workerconfig.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio},
			}},
		}},
	}
}
