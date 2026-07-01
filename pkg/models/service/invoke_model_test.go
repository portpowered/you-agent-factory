package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestService_InvokeModel_ReturnsCanonicalContentAndBindings(t *testing.T) {
	t.Parallel()

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     func() modelhost.Host { return readyInvokeHost{} },
		ModelInvocationExecutor: func(_ *factoryconfig.LoadedFactoryConfig, _ *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
			return stubInvocationExecutor{
				workerName: workerName,
				output:     mustMarshalAudioContentResponse(t, audioPath),
			}, nil
		},
	})

	mode := factoryapi.AUDIOSTREAM
	result, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedInvokeTextPart(t, "hello world"),
		},
		Options: &factoryapi.ModelInvocationOptions{ResponseMode: &mode},
	})
	if err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	if result.ModelName != "OMNIVOICE_Q4_K_M" || result.Worker != "voice-local" {
		t.Fatalf("result identity = %#v, want OMNIVOICE voice-local", result)
	}
	if len(result.Bindings) != 1 || result.Bindings[0].Slot != "text" {
		t.Fatalf("bindings = %#v, want one text binding", result.Bindings)
	}
	if len(result.Content) != 1 || result.Content[0].Type != interfaces.WorkContentPartTypeAudio ||
		result.StreamFile != audioPath || result.StreamContentType != "audio/wav" {
		t.Fatalf("result content = %#v stream=%q type=%q, want audio output", result.Content, result.StreamFile, result.StreamContentType)
	}
}

func TestService_InvokeModel_ReturnsNotFoundWhenModelDoesNotExist(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, &interfaces.FactoryConfig{Name: "factory"})
	svc := modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
	})

	_, err := svc.InvokeModel(context.Background(), "MISSING", factoryapi.ModelInvocationRequest{Operation: "TTS"})
	if err == nil || !errors.Is(err, apisurface.ErrModelNotFound) {
		t.Fatalf("InvokeModel error = %v, want ErrModelNotFound", err)
	}
}

func TestService_InvokeModel_ReturnsManagedRuntimeMissingWhenCacheNotReady(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     func() modelhost.Host { return missingCacheInspectHost{} },
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedInvokeTextPart(t, "hello world"),
		},
	})
	if err == nil || !apisurface.IsManagedRuntimeMissing(err) {
		t.Fatalf("InvokeModel error = %v, want managed runtime missing", err)
	}
}

type readyInvokeHost struct{}

func (readyInvokeHost) ResolveIdentity(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.Identity, error) {
	return modelhost.Identity{Name: modelName, Locality: factoryapi.WorkerModelLocalityLocal}, nil
}

func (readyInvokeHost) InspectReadiness(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: modelName, Locality: factoryapi.WorkerModelLocalityLocal},
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
	}, nil
}

func (readyInvokeHost) Pull(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, errors.New("pull unavailable in test host")
}

func (readyInvokeHost) AcquireLease(context.Context, *factoryconfig.LoadedFactoryConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, errors.New("lease unavailable in test host")
}

func (readyInvokeHost) ReleaseLease(context.Context, string) error {
	return errors.New("lease unavailable in test host")
}

func (readyInvokeHost) Unload(context.Context, *factoryconfig.LoadedFactoryConfig, string) error {
	return errors.New("unload unavailable in test host")
}

type stubInvocationExecutor struct {
	workerName string
	output     string
}

func (s stubInvocationExecutor) Execute(_ context.Context, request interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	if request.WorkerType != s.workerName {
		return interfaces.WorkResult{}, errors.New("unexpected worker")
	}
	return interfaces.WorkResult{
		Outcome: interfaces.OutcomeAccepted,
		Output:  s.output,
	}, nil
}

func mustGeneratedInvokeTextPart(t *testing.T, text string) factoryapi.WorkContentPart {
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
