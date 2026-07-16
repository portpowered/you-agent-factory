package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestModelTransportSmoke_ServiceModeStartupAndDirectModelRoutesStayAligned(t *testing.T) {
	dir := support.ScaffoldFactory(t, providerBackedModelTransportSmokeConfig())
	support.WriteAgentConfig(t, dir, "tts-worker", support.BuildModelWorkerConfig(modelprovider.Codex, "OMNIVOICE_Q4_K_M"))

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	if err := os.WriteFile(audioPath, []byte("RIFF....WAVEpayload"), 0o644); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}

	providerStub := &modelTransportSmokeProvider{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalFunctionalAudioContentResponse(t, audioPath),
		},
	}
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.ProviderOverride = providerStub
		cfg.SkipBuiltInRunnerPrerequisiteValidation = true
	}, factory.WithServiceMode())

	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("GET /status factory_state = %q, want RUNNING", status.FactoryState)
	}
	if status.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("GET /status runtime_status = %q, want IDLE", status.RuntimeStatus)
	}

	models := getGeneratedJSON[factoryapi.ListModelsResponse](t, server.URL()+"/models")
	if len(models.Results) != 1 {
		t.Fatalf("GET /models result count = %d, want 1", len(models.Results))
	}
	if models.Results[0].Name != "OMNIVOICE_Q4_K_M" || models.Results[0].ProviderLocality != factoryapi.WorkerModelLocalityCloud {
		t.Fatalf("GET /models first result = %#v, want OMNIVOICE cloud model", models.Results[0])
	}
	if models.Results[0].ManagedRuntime.Identity != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("GET /models managed runtime identity = %q, want OMNIVOICE_Q4_K_M", models.Results[0].ManagedRuntime.Identity)
	}
	if models.Results[0].ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("GET /models managed readiness = %s, want READY", models.Results[0].ManagedRuntime.ReadinessState)
	}
	if models.Results[0].ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE {
		t.Fatalf("GET /models managed lifecycle = %s, want NOT_APPLICABLE", models.Results[0].ManagedRuntime.LifecycleState)
	}

	model := getGeneratedJSON[factoryapi.ModelDetail](t, server.URL()+"/models/OMNIVOICE_Q4_K_M")
	if model.Name != "OMNIVOICE_Q4_K_M" || len(model.Capabilities) != 1 || model.Capabilities[0].Worker != "tts-worker" {
		t.Fatalf("GET /models/OMNIVOICE_Q4_K_M = %#v, want one tts-worker capability", model)
	}
	if model.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("GET /models/{name} managed readiness = %s, want READY", model.ManagedRuntime.ReadinessState)
	}

	responseMode := factoryapi.METADATA
	response := postJSON[factoryapi.ModelInvocationResponse](t, server.URL()+"/models/OMNIVOICE_Q4_K_M/invocations", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Bindings:  providerBackedModelTransportBindings(),
		Content: &factoryapi.WorkContent{
			mustGeneratedFunctionalTextPart(t, "hello world"),
		},
		Options: &factoryapi.ModelInvocationOptions{ResponseMode: &responseMode},
	}, "model transport smoke invocation failure")
	if response.ModelName != "OMNIVOICE_Q4_K_M" || response.Worker != "tts-worker" || response.Operation != "TTS" {
		t.Fatalf("POST /models/.../invocations identity = %#v, want OMNIVOICE_Q4_K_M/tts-worker/TTS", response)
	}
	if response.ProviderLocality != factoryapi.WorkerModelLocalityCloud {
		t.Fatalf("POST /models/.../invocations provider locality = %q, want CLOUD", response.ProviderLocality)
	}
	if len(response.Bindings) != 1 || response.Bindings[0].Slot != "text" || response.Bindings[0].Source != factoryapi.INPUT {
		t.Fatalf("POST /models/.../invocations bindings = %#v, want one input text binding", response.Bindings)
	}
	if len(response.Content) != 1 {
		t.Fatalf("POST /models/.../invocations content count = %d, want 1", len(response.Content))
	}
	audioPart, err := response.Content[0].AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("decode invocation audio content: %v", err)
	}
	if stringPointerValue(audioPart.ContentType) != "audio/wav" || generatedAudioPath(audioPart) != audioPath {
		t.Fatalf("POST /models/.../invocations audio part = %#v, want audio/wav at %s", audioPart, audioPath)
	}

	calls := providerStub.Calls()
	if len(calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(calls))
	}
	if calls[0].Model != "OMNIVOICE_Q4_K_M" || calls[0].ModelOperation != "TTS" {
		t.Fatalf("provider call = %#v, want OMNIVOICE_Q4_K_M TTS", calls[0])
	}
	if len(calls[0].ModelBindings) != 1 || len(calls[0].ModelBindings[0].Content) != 1 || calls[0].ModelBindings[0].Content[0].Text != "hello world" {
		t.Fatalf("provider bindings = %#v, want one text binding for hello world", calls[0].ModelBindings)
	}

	unsupportedBody, err := json.Marshal(factoryapi.ModelInvocationRequest{Operation: "EMBED"})
	if err != nil {
		t.Fatalf("marshal unsupported invocation: %v", err)
	}
	unsupportedResponse, err := http.Post(
		server.URL()+"/models/OMNIVOICE_Q4_K_M/invocations",
		"application/json",
		bytes.NewReader(unsupportedBody),
	)
	if err != nil {
		t.Fatalf("POST unsupported model invocation: %v", err)
	}
	defer unsupportedResponse.Body.Close()
	if unsupportedResponse.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(unsupportedResponse.Body)
		t.Fatalf("unsupported invocation status = %d, want 400: %s", unsupportedResponse.StatusCode, body)
	}
}

func providerBackedModelTransportSmokeConfig() map[string]any {
	return map[string]any{
		"name": "model-transport-smoke",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
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
		}},
	}
}

func mustMarshalFunctionalAudioContentResponse(t *testing.T, audioPath string) string {
	t.Helper()
	body, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
	}})
	if err != nil {
		t.Fatalf("marshal functional audio content: %v", err)
	}
	return string(body)
}

func providerBackedModelTransportBindings() *[]factoryapi.WorkstationOperationBinding {
	return &[]factoryapi.WorkstationOperationBinding{{
		Slot: "text",
		Selector: &factoryapi.WorkstationOperationBindingSelector{
			Type: func() *factoryapi.ModelOperationContentType {
				value := factoryapi.ModelOperationContentTypeText
				return &value
			}(),
		},
	}}
}

type modelTransportSmokeProvider struct {
	mu       sync.Mutex
	calls    []workerexecution.ProviderInferenceRequest
	response workerexecution.InferenceResponse
}

func (p *modelTransportSmokeProvider) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, workerexecution.CloneProviderInferenceRequest(req))
	return p.response, nil
}

func (p *modelTransportSmokeProvider) Calls() []workerexecution.ProviderInferenceRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	calls := make([]workerexecution.ProviderInferenceRequest, len(p.calls))
	for i, call := range p.calls {
		calls[i] = workerexecution.CloneProviderInferenceRequest(call)
	}
	return calls
}

var _ provider.Provider = (*modelTransportSmokeProvider)(nil)
