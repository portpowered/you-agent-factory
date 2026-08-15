package runtime_api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestModelTransportSmoke_PullUsesConfiguredLegacyCacheWithoutNetwork confirms model pull returns a configured legacy cache hit without upstream network requests.
func TestModelTransportSmoke_PullUsesConfiguredLegacyCacheWithoutNetwork(t *testing.T) {
	dir := support.ScaffoldFactory(t, localCachedModelTransportSmokeConfig())
	cacheDirectory := t.TempDir()
	revision := "cached-revision"
	modelDirectory := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", revision)
	if err := os.MkdirAll(modelDirectory, 0o755); err != nil {
		t.Fatalf("create cached model directory: %v", err)
	}
	assetBody := []byte("cached-model-asset")
	checksum := fmt.Sprintf("%x", sha256.Sum256(assetBody))
	files := []string{"omnivoice-base-Q4_K_M.gguf", "omnivoice-tokenizer-Q4_K_M.gguf"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(modelDirectory, name), assetBody, 0o644); err != nil {
			t.Fatalf("write cached model asset %s: %v", name, err)
		}
	}
	metadata, err := json.Marshal(map[string]any{
		"modelName": "OMNIVOICE_Q4_K_M",
		"revision":  revision,
		"files": []map[string]any{
			{"path": files[0], "bytes": len(assetBody), "sha256": checksum},
			{"path": files[1], "bytes": len(assetBody), "sha256": checksum},
		},
	})
	if err != nil {
		t.Fatalf("marshal cached model metadata: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", ".managed-cache.json"),
		metadata,
		0o644,
	); err != nil {
		t.Fatalf("write cached model metadata: %v", err)
	}

	network := &rejectingModelAssetHTTP{}
	environment := append(os.Environ(), "INFINITE_YOU_OMNIVOICE_CACHE_DIR="+cacheDirectory)
	server := startFunctionalServer(t, dir, false, withEnvironment(environment), func(config *support.FunctionalAPIServerConfig) {
		config.Edges.ModelAssetHTTPClient = network
	})

	pull := postJSON[factoryapi.ModelPullResponse](
		t,
		server.URL()+"/models/OMNIVOICE_Q4_K_M/pull",
		nil,
		"cached asset pull",
	)
	if pull.Outcome != factoryapi.ModelPullOutcomeALREADYPRESENT ||
		pull.CachePath != modelDirectory ||
		pull.Revision != revision ||
		len(pull.DownloadedFiles) != len(files) {
		t.Fatalf("cached asset pull = %#v, want legacy cache hit", pull)
	}
	if network.Calls() != 0 {
		t.Fatalf("cached asset pull made %d upstream requests, want none", network.Calls())
	}
}

func TestModelTransportSmoke_ServiceModeStartupAndDirectModelRoutesStayAligned(t *testing.T) {
	dir := support.ScaffoldFactory(t, providerBackedModelTransportSmokeConfig())
	support.WriteAgentConfig(t, dir, "tts-worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "OMNIVOICE_Q4_K_M"))

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	if err := os.WriteFile(audioPath, []byte("RIFF....WAVEpayload"), 0o644); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}

	providerStub := &modelTransportSmokeProvider{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalFunctionalAudioContentResponse(t, audioPath),
		},
	}
	server := startFunctionalServer(t, dir, false, withProvider(providerStub))

	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("GET /status factory_state = %q, want RUNNING", status.FactoryState)
	}
	if status.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("GET /status runtime_status = %q, want IDLE", status.RuntimeStatus)
	}

	models := getGeneratedJSON[factoryapi.ListModelsResponse](t, server.URL()+"/models")
	declaredModel, ok := findModelSummary(models.Results, "OMNIVOICE_Q4_K_M")
	if !ok {
		t.Fatalf("GET /models did not include declared OMNIVOICE_Q4_K_M; results=%#v", models.Results)
	}
	if declaredModel.ProviderLocality != factoryapi.WorkerModelLocalityCloud {
		t.Fatalf("GET /models declared result = %#v, want OMNIVOICE cloud model", declaredModel)
	}
	if declaredModel.ManagedRuntime.Identity != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("GET /models managed runtime identity = %q, want OMNIVOICE_Q4_K_M", declaredModel.ManagedRuntime.Identity)
	}
	if declaredModel.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("GET /models managed readiness = %s, want READY", declaredModel.ManagedRuntime.ReadinessState)
	}
	if declaredModel.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE {
		t.Fatalf("GET /models managed lifecycle = %s, want NOT_APPLICABLE", declaredModel.ManagedRuntime.LifecycleState)
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

	assertUnsupportedModelInvocationRejected(t, server.URL())
}

func TestFactoryWork_ModelResponseRecordsStructuredAudio(t *testing.T) {
	dir := support.ScaffoldFactory(t, factoryWorkModelTransportSmokeConfig())
	support.WriteWorkstationConfig(t, dir, "speak", "---\ntype: MODEL_INVOKE\n---\nGenerate speech output.\n")

	audioPath := filepath.Join(t.TempDir(), "factory-work-speech.wav")
	if err := os.WriteFile(audioPath, []byte("RIFF....WAVEfactory-work-audio"), 0o644); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}
	provider := &modelTransportSmokeProvider{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalFunctionalAudioContentResponse(t, audioPath),
		},
	}
	server := startFunctionalServer(t, dir, false, withProvider(provider))

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPtr("factory-work-model-response"),
		WorkTypeName: "task",
		Content: &factoryapi.WorkContent{
			mustGeneratedFunctionalTextPart(t, "hello from Factory Work"),
		},
	})
	completed := waitForGeneratedWorkAtPlace(t, server.URL(), traceID, "task:complete", 10*time.Second)
	var completedWork factoryapi.Work
	for _, item := range completed.Results {
		if item.TraceId != nil && *item.TraceId == traceID && generatedWorkPlaceID(item) == "task:complete" {
			completedWork = item
			break
		}
	}
	if completedWork.Content == nil || len(*completedWork.Content) != 1 {
		t.Fatalf("completed Factory Work content = %#v, want exactly one audio part", completedWork.Content)
	}
	workAudio, err := (*completedWork.Content)[0].AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("decode completed Factory Work audio: %v", err)
	}
	if generatedAudioPath(workAudio) != audioPath || stringPointerValue(workAudio.ContentType) != "audio/wav" {
		t.Fatalf("completed Factory Work audio = %#v, want audio/wav at %s", workAudio, audioPath)
	}

	var requests []factoryapi.ModelRequestEventPayload
	var responses []factoryapi.ModelResponseEventPayload
	events := server.GetFactoryEvents(t)
	requestIndex, responseIndex := -1, -1
	var requestEvent, responseEvent factoryapi.FactoryEvent
	for index, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeModelRequest:
			payload, payloadErr := event.Payload.AsModelRequestEventPayload()
			if payloadErr == nil && payload.Operation == "TTS" {
				requests = append(requests, payload)
				requestIndex, requestEvent = index, event
			}
		case factoryapi.FactoryEventTypeModelResponse:
			payload, payloadErr := event.Payload.AsModelResponseEventPayload()
			if payloadErr == nil && payload.Operation == "TTS" {
				responses = append(responses, payload)
				responseIndex, responseEvent = index, event
			}
		}
	}
	if len(requests) != 1 || len(responses) != 1 {
		t.Fatalf("Factory Work TTS model events = requests:%d responses:%d, want one of each", len(requests), len(responses))
	}
	if responseIndex <= requestIndex {
		t.Fatalf("Factory Work TTS model event order = request:%d response:%d, want request before response", requestIndex, responseIndex)
	}
	requestDispatchID := stringPointerValue(requestEvent.Context.DispatchId)
	responseDispatchID := stringPointerValue(responseEvent.Context.DispatchId)
	if requestDispatchID == "" || requestDispatchID != responseDispatchID {
		t.Fatalf("Factory Work TTS model event dispatch correlation = request:%q response:%q, want one non-empty id", requestDispatchID, responseDispatchID)
	}
	if responses[0].Outcome != factoryapi.InferenceOutcomeSucceeded || responses[0].ModelRequestId != requests[0].ModelRequestId {
		t.Fatalf("Factory Work TTS model response = %#v, want succeeded response correlated to request %q", responses[0], requests[0].ModelRequestId)
	}
	if responses[0].OutputContent == nil || len(*responses[0].OutputContent) != 1 {
		t.Fatalf("Factory Work TTS response output = %#v, want exactly one audio part", responses[0].OutputContent)
	}
	responseAudio, err := (*responses[0].OutputContent)[0].AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("decode Factory Work TTS response audio: %v", err)
	}
	if generatedAudioPath(responseAudio) != audioPath || stringPointerValue(responseAudio.ContentType) != "audio/wav" {
		t.Fatalf("Factory Work TTS response audio = %#v, want audio/wav at %s", responseAudio, audioPath)
	}
}

func findModelSummary(results []factoryapi.ModelSummary, name string) (factoryapi.ModelSummary, bool) {
	for _, result := range results {
		if result.Name == name {
			return result, true
		}
	}
	return factoryapi.ModelSummary{}, false
}

func localCachedModelTransportSmokeConfig() map[string]any {
	return map[string]any{
		"name": "cached-local-model-transport",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       "MODEL",
			"capacity":   1,
			"model":      "OMNIVOICE_Q4_K_M",
			"backend":    "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name":          "tts-worker",
			"type":          "MODEL_WORKER",
			"model":         "OMNIVOICE_Q4_K_M",
			"modelProvider": "CODEX",
			"modelLocality": "LOCAL",
			"command":       "unused-local-runtime",
			"resources": []map[string]any{{
				"name": "omnivoice-cache", "capacity": 1,
			}},
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name": "text", "contentTypes": []string{"TEXT"}, "required": true,
				}},
				"outputs": []map[string]any{{
					"name": "audio", "contentTypes": []string{"AUDIO"},
				}},
			}},
		}},
	}
}

func assertUnsupportedModelInvocationRejected(t *testing.T, serverURL string) {
	t.Helper()
	unsupportedBody, err := json.Marshal(factoryapi.ModelInvocationRequest{Operation: "EMBED"})
	if err != nil {
		t.Fatalf("marshal unsupported invocation: %v", err)
	}
	unsupportedResponse, err := http.Post(
		serverURL+"/models/OMNIVOICE_Q4_K_M/invocations",
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
		}},
	}
}

func factoryWorkModelTransportSmokeConfig() map[string]any {
	cfg := providerBackedModelTransportSmokeConfig()
	cfg["workstations"] = []map[string]any{{
		"name":      "speak",
		"type":      interfaces.WorkstationTypeInvoke,
		"operation": "TTS",
		"worker":    "tts-worker",
		"operationBindings": []map[string]any{{
			"slot": "text",
			"selector": map[string]any{
				"type": interfaces.ModelOperationContentTypeText,
			},
		}},
		"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
		"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
		"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
	}}
	return cfg
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

type rejectingModelAssetHTTP struct {
	mu    sync.Mutex
	calls int
}

func (client *rejectingModelAssetHTTP) Do(*http.Request) (*http.Response, error) {
	client.mu.Lock()
	client.calls++
	client.mu.Unlock()
	return nil, fmt.Errorf("unexpected model asset network request")
}

func (client *rejectingModelAssetHTTP) Calls() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.calls
}

func (p *modelTransportSmokeProvider) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, workerexecution.CloneProviderInferenceRequest(req))
	response := p.response
	if response.Content != "" && response.Diagnostics == nil {
		response.Diagnostics = &workerexecution.WorkDiagnostics{Metadata: map[string]string{
			workerexecution.ProviderResponseMetadataCompletionEvidence: "provider_response",
		}}
	}
	return response, nil
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
