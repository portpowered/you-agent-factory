package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type fakeLocalModelRuntime struct {
	mu          sync.Mutex
	loads       []localModelLoadRequest
	invocations []localModelInvocationRequest
	response    interfaces.InferenceResponse
	loadErr     error
	invokeErr   error
}

func (r *fakeLocalModelRuntime) Supports(resource interfaces.ResourceConfig, worker *interfaces.WorkerConfig) bool {
	return canonicalBackendName(resource.Backend) == "LLAMACPP" && canonicalModelName(worker.Model) == canonicalModelName("OMNIVOICE_Q4_K_M")
}

func (r *fakeLocalModelRuntime) Load(_ context.Context, request localModelLoadRequest) (localModelHandle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.loads = append(r.loads, request)
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	return fakeLocalModelHandle{runtime: r}, nil
}

func (r *fakeLocalModelRuntime) loadCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.loads)
}

func (r *fakeLocalModelRuntime) invocationCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.invocations)
}

type fakeLocalModelHandle struct {
	runtime *fakeLocalModelRuntime
}

func (h fakeLocalModelHandle) Invoke(_ context.Context, request localModelInvocationRequest) (interfaces.InferenceResponse, error) {
	h.runtime.mu.Lock()
	defer h.runtime.mu.Unlock()

	h.runtime.invocations = append(h.runtime.invocations, request)
	if h.runtime.invokeErr != nil {
		return interfaces.InferenceResponse{}, h.runtime.invokeErr
	}
	return h.runtime.response, nil
}

func TestInvokeModel_UsesManagedLocalModelRuntimeAndReusesLoadedHandle(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", localModelFactoryConfig(), localModelRuntimeWorkers(), nil)
	cache := localModelTestCacheLayout(t)
	svc := &FactoryService{
		runtimeCfg: runtimeCfg,
		cfg:        &FactoryServiceConfig{},
		modelAssets: staticModelAssetPuller{
			cache: cache,
		},
		localModels: newManagedLocalModelManager(staticModelAssetPuller{
			cache: cache,
		}, runtime),
	}

	mode := factoryapi.AUDIOSTREAM
	request := factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello world"),
		},
		Options: &factoryapi.ModelInvocationOptions{ResponseMode: &mode},
	}

	first, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", request)
	if err != nil {
		t.Fatalf("first InvokeModel: %v", err)
	}
	second, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", request)
	if err != nil {
		t.Fatalf("second InvokeModel: %v", err)
	}
	if runtime.loadCount() != 1 {
		t.Fatalf("load count = %d, want 1", runtime.loadCount())
	}
	if runtime.invocationCount() != 2 {
		t.Fatalf("invocation count = %d, want 2", runtime.invocationCount())
	}
	if len(first.Content) != 1 || first.Content[0].Type != interfaces.WorkContentPartTypeAudio || first.StreamFile != audioPath || first.StreamContentType != "audio/wav" {
		t.Fatalf("first result = %#v, want audio content and stream metadata", first)
	}
	if len(second.Content) != 1 || second.Content[0].Type != interfaces.WorkContentPartTypeAudio {
		t.Fatalf("second result = %#v, want audio content", second)
	}
}

func TestLoadWorkersFromConfig_LocalModelWorkerUsesManagedRuntimePath(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	provider := &providerCallRecorder{}
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	factoryCfg := localModelFactoryConfig()
	cache := localModelTestCacheLayout(t)
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, localModelRuntimeWorkers(), map[string]*interfaces.FactoryWorkstationConfig{
		"speak": {
			Name:           "speak",
			Type:           interfaces.WorkstationTypeInvoke,
			WorkerTypeName: "tts-worker",
			Operation:      "TTS",
			OperationBindings: []interfaces.ModelOperationBinding{{
				Slot: "text",
				Selector: &interfaces.ModelOperationBindingSelector{
					Label: "utterance",
					Type:  interfaces.ModelOperationContentTypeText,
				},
			}},
		},
	})

	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		logging.NoopLogger{},
		true,
		provider,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		newLocalModelResourceLimiter(),
		newManagedLocalModelManager(staticModelAssetPuller{
			cache: cache,
		}, runtime),
	)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}
	exec, ok := fc.WorkerExecutors["tts-worker"]
	if !ok {
		t.Fatal("expected tts-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "dispatch-tts",
		TransitionID:    "transition-tts",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "token-tts",
			Color: interfaces.TokenColor{
				WorkID: "work-tts",
				Content: []interfaces.WorkContentPart{{
					Type:  interfaces.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello world",
				}},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if runtime.loadCount() != 1 || runtime.invocationCount() != 1 {
		t.Fatalf("runtime load/invoke counts = %d/%d, want 1/1", runtime.loadCount(), runtime.invocationCount())
	}
	if calls := provider.Calls(); len(calls) != 0 {
		t.Fatalf("provider calls = %#v, want local runtime to bypass provider path", calls)
	}
}

func TestLoadWorkersFromConfig_LocalModelWorkerRecordsModelExecutionEvents(t *testing.T) {
	eventTime := time.Date(2026, time.May, 22, 10, 0, 0, 0, time.UTC)
	wsExec, history, audioPath := localModelExecutionRecorderFixture(t, eventTime)
	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "dispatch-tts",
		TransitionID:    "transition-tts",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		Execution: interfaces.ExecutionMetadata{
			CurrentTick: 2,
			RequestID:   "request-tts",
			TraceID:     "trace-tts",
			WorkIDs:     []string{"work-tts"},
		},
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "token-tts",
			Color: interfaces.TokenColor{
				WorkID: "work-tts",
				Content: []interfaces.WorkContentPart{{
					Type:  interfaces.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello world",
				}},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	assertRecordedLocalModelExecutionEvents(t, history.Events(), audioPath)
}

func TestLoadWorkersFromConfig_LocalModelWorkerDetachesClonedWorkerRequestsFromLaterSourceMutation(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	factoryCfg := localModelFactoryConfig()
	cache := localModelTestCacheLayout(t)
	runtimeWorkers := localModelRuntimeWorkersWithCloneCoverage()
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, runtimeWorkers, map[string]*interfaces.FactoryWorkstationConfig{
		"speak": {
			Name:           "speak",
			Type:           interfaces.WorkstationTypeInvoke,
			WorkerTypeName: "tts-worker",
			Operation:      "TTS",
			OperationBindings: []interfaces.ModelOperationBinding{{
				Slot: "text",
				Selector: &interfaces.ModelOperationBindingSelector{
					Label: "utterance",
					Type:  interfaces.ModelOperationContentTypeText,
				},
			}},
		},
	})
	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		logging.NoopLogger{},
		true,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		newLocalModelResourceLimiter(),
		newManagedLocalModelManager(staticModelAssetPuller{cache: cache}, runtime),
	)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}
	exec, ok := fc.WorkerExecutors["tts-worker"]
	if !ok {
		t.Fatal("expected tts-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), localModelDispatch())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if runtime.loadCount() != 1 || runtime.invocationCount() != 1 {
		t.Fatalf("runtime load/invoke counts = %d/%d, want 1/1", runtime.loadCount(), runtime.invocationCount())
	}

	mutateLocalModelWorkerCloneSource(runtimeWorkers["tts-worker"])

	runtime.mu.Lock()
	if len(runtime.loads) != 1 {
		runtime.mu.Unlock()
		t.Fatalf("load requests = %d, want 1", len(runtime.loads))
	}
	loadWorker := runtime.loads[0].Worker
	if len(runtime.invocations) != 1 {
		runtime.mu.Unlock()
		t.Fatalf("invoke requests = %d, want 1", len(runtime.invocations))
	}
	invokeWorker := runtime.invocations[0].Worker
	runtime.mu.Unlock()

	assertLocalModelCloneCoverageWorker(t, loadWorker)
	assertLocalModelCloneCoverageWorker(t, invokeWorker)
}

func TestNewRecordingModelRunner_LocalModelWorkerEventsStayDetachedFromLaterSourceMutation(t *testing.T) {
	eventTime := time.Date(2026, time.May, 24, 8, 30, 0, 0, time.UTC)
	workerDef := localModelRuntimeWorkersWithCloneCoverage()["tts-worker"]
	var events []factoryapi.FactoryEvent
	runner := newRecordingModelRunner(
		recordingModelRunnerFunc(func(_ context.Context, _ interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
			return interfaces.RunnerExecutionResult{
				Content: mustMarshalAudioContentResponse(t, filepath.Join(t.TempDir(), "speech.wav")),
			}, nil
		}),
		localModelFactoryConfig(),
		workerDef,
		func(event factoryapi.FactoryEvent) {
			events = append(events, event)
		},
		func() time.Time { return eventTime },
	)

	mutateLocalModelWorkerCloneSource(workerDef)

	_, err := runner.Execute(context.Background(), interfaces.RunnerExecutionRequest{
		Dispatch: interfaces.WorkDispatch{
			DispatchID: "dispatch-tts",
			Execution: interfaces.ExecutionMetadata{
				CurrentTick: 2,
				RequestID:   "request-tts",
				TraceID:     "trace-tts",
				WorkIDs:     []string{"work-tts"},
			},
		},
		ModelOperation: "TTS",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("recorded events = %d, want 2", len(events))
	}
	requestPayload, err := events[0].Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode model request payload: %v", err)
	}
	if requestPayload.Worker != "tts-worker" {
		t.Fatalf("request worker = %q, want tts-worker", requestPayload.Worker)
	}
	if requestPayload.Model != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("request model = %q, want OMNIVOICE_Q4_K_M", requestPayload.Model)
	}
	if requestPayload.ProviderLocality != interfaces.ModelLocalityLocal {
		t.Fatalf("request locality = %q, want %q", requestPayload.ProviderLocality, interfaces.ModelLocalityLocal)
	}
	if requestPayload.Resources == nil || len(*requestPayload.Resources) != 1 || (*requestPayload.Resources)[0].Name != "omnivoice-cache" {
		t.Fatalf("request resources = %#v, want omnivoice-cache", requestPayload.Resources)
	}
	responsePayload, err := events[1].Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode model response payload: %v", err)
	}
	if responsePayload.Resources == nil || len(*responsePayload.Resources) != 1 || (*responsePayload.Resources)[0].Name != "omnivoice-cache" {
		t.Fatalf("response resources = %#v, want omnivoice-cache", responsePayload.Resources)
	}
}

func localModelExecutionRecorderFixture(t *testing.T, eventTime time.Time) (*workers.WorkstationExecutor, *factory.FactoryEventHistory, string) {
	t.Helper()
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	factoryCfg := localModelFactoryConfig()
	cache := localModelTestCacheLayout(t)
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, localModelRuntimeWorkers(), map[string]*interfaces.FactoryWorkstationConfig{
		"speak": {
			Name:           "speak",
			Type:           interfaces.WorkstationTypeInvoke,
			WorkerTypeName: "tts-worker",
			Operation:      "TTS",
			OperationBindings: []interfaces.ModelOperationBinding{{
				Slot: "text",
				Selector: &interfaces.ModelOperationBindingSelector{
					Label: "utterance",
					Type:  interfaces.ModelOperationContentTypeText,
				},
			}},
		},
	})
	history := factory.NewFactoryEventHistory(nil, func() time.Time { return eventTime }, runtimeCfg)
	opts, err := loadWorkersFromConfig("", factoryCfg, "", runtimeCfg, logging.NoopLogger{}, true, nil, nil, nil, nil, nil, history.RecordModelEvent, func() time.Time { return eventTime }, newLocalModelResourceLimiter(), newManagedLocalModelManager(staticModelAssetPuller{cache: cache}, runtime))
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}
	exec, ok := fc.WorkerExecutors["tts-worker"]
	if !ok {
		t.Fatal("expected tts-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	return wsExec, history, audioPath
}

func assertRecordedLocalModelExecutionEvents(t *testing.T, events []factoryapi.FactoryEvent, audioPath string) {
	t.Helper()
	if len(events) != 2 {
		t.Fatalf("recorded events = %d, want 2 model events", len(events))
	}
	if events[0].Type != factoryapi.FactoryEventTypeModelRequest {
		t.Fatalf("first event type = %s, want %s", events[0].Type, factoryapi.FactoryEventTypeModelRequest)
	}
	if events[1].Type != factoryapi.FactoryEventTypeModelResponse {
		t.Fatalf("second event type = %s, want %s", events[1].Type, factoryapi.FactoryEventTypeModelResponse)
	}

	assertRecordedLocalModelRequestPayload(t, events[0])
	assertRecordedLocalModelResponsePayload(t, events[1], audioPath)
}

func assertRecordedLocalModelRequestPayload(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	requestPayload, err := event.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode model request payload: %v", err)
	}
	if requestPayload.Operation != "TTS" || requestPayload.Worker != "tts-worker" || requestPayload.Model != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("request payload = %#v, want operation/worker/model evidence", requestPayload)
	}
	if requestPayload.Resources == nil || len(*requestPayload.Resources) != 1 || (*requestPayload.Resources)[0].Name != "omnivoice-cache" {
		t.Fatalf("request resources = %#v, want omnivoice-cache", requestPayload.Resources)
	}
	if requestPayload.Bindings == nil || len(*requestPayload.Bindings) != 1 || (*requestPayload.Bindings)[0].Slot != "text" || (*requestPayload.Bindings)[0].Source != factoryapi.INPUT {
		t.Fatalf("request bindings = %#v, want one resolved text input binding", requestPayload.Bindings)
	}
}

func assertRecordedLocalModelResponsePayload(t *testing.T, event factoryapi.FactoryEvent, audioPath string) {
	t.Helper()
	responsePayload, err := event.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode model response payload: %v", err)
	}
	if responsePayload.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("response outcome = %s, want %s", responsePayload.Outcome, factoryapi.InferenceOutcomeSucceeded)
	}
	if responsePayload.ResourceAcquired == nil || !*responsePayload.ResourceAcquired {
		t.Fatalf("response resourceAcquired = %#v, want true", responsePayload.ResourceAcquired)
	}
	if responsePayload.LoadRequested == nil || !*responsePayload.LoadRequested {
		t.Fatalf("response loadRequested = %#v, want true", responsePayload.LoadRequested)
	}
	if responsePayload.OutputContent == nil || len(*responsePayload.OutputContent) != 1 {
		t.Fatalf("response outputContent = %#v, want one audio content part", responsePayload.OutputContent)
	}
	audioPart, audioErr := (*responsePayload.OutputContent)[0].AsWorkAudioContentPart()
	if audioErr != nil || audioPart.File != audioPath {
		t.Fatalf("response output audio = %#v, %v, want file %q", responsePayload.OutputContent, audioErr, audioPath)
	}
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
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "speak",
			WorkerTypeName: "tts-worker",
		}},
		Workers: []interfaces.WorkerConfig{{
			Name: "tts-worker",
		}},
	}
}

func localModelRuntimeWorkers() map[string]*interfaces.WorkerConfig {
	return map[string]*interfaces.WorkerConfig{
		"tts-worker": {
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
		},
	}
}

func localModelRuntimeWorkersWithCloneCoverage() map[string]*interfaces.WorkerConfig {
	return map[string]*interfaces.WorkerConfig{
		"tts-worker": {
			Name:          "tts-worker",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelProvider: interfaces.RunnerIDCodex,
			ModelLocality: interfaces.ModelLocalityLocal,
			Args:          []string{"--voice", "alloy"},
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
			Auth: &interfaces.HostedWorkerAuthConfig{
				SecretRef: "secret/local-model",
			},
			Linear: &interfaces.HostedLinearWorkerConfig{
				PollInterval: "30s",
				TeamIDs:      []string{"team-a"},
				StateIDs:     []string{"state-ready"},
				Claim: &interfaces.HostedLinearWorkerClaimConfig{
					AssigneeField: "owner",
				},
			},
		},
	}
}

func localModelTestCacheLayout(t *testing.T) localModelCacheLayout {
	t.Helper()
	return localModelCacheLayout{
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: filepath.Join(t.TempDir(), "cache", "rev-test"),
		Revision:  "rev-test",
		Files: []string{
			"/models/omnivoice-base-Q4_K_M.gguf",
			"/models/omnivoice-tokenizer-Q4_K_M.gguf",
		},
	}
}

func mustMarshalAudioContentResponse(t *testing.T, audioPath string) string {
	t.Helper()
	content := []interfaces.WorkContentPart{{
		Type:        interfaces.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
	}}
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal audio content response: %v", err)
	}
	return string(data)
}

func localModelDispatch() interfaces.WorkDispatch {
	return interfaces.WorkDispatch{
		DispatchID:      "dispatch-tts",
		TransitionID:    "transition-tts",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "token-tts",
			Color: interfaces.TokenColor{
				WorkID: "work-tts",
				Content: []interfaces.WorkContentPart{{
					Type:  interfaces.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello world",
				}},
			},
		}),
	}
}

func mutateLocalModelWorkerCloneSource(worker *interfaces.WorkerConfig) {
	worker.Args[0] = "--mutated"
	worker.Resources[0].Name = "mutated-cache"
	worker.Resources[0].Capacity = 9
	worker.Operations[0].Inputs[0].ContentTypes[0] = interfaces.ModelOperationContentTypeJSON
	worker.Operations[0].Outputs[0].ContentTypes[0] = interfaces.ModelOperationContentTypeBinary
	worker.Auth.SecretRef = "secret/mutated"
	worker.Linear.TeamIDs[0] = "team-b"
	worker.Linear.StateIDs[0] = "state-done"
	worker.Linear.Claim.AssigneeField = "mutated-owner"
	worker.Model = "mutated-model"
	worker.ModelLocality = interfaces.ModelLocalityCloud
}

func assertLocalModelCloneCoverageWorker(t *testing.T, worker *interfaces.WorkerConfig) {
	t.Helper()
	if worker == nil {
		t.Fatal("expected cloned worker, got nil")
	}
	if len(worker.Args) != 2 || worker.Args[0] != "--voice" || worker.Args[1] != "alloy" {
		t.Fatalf("worker args = %#v, want [--voice alloy]", worker.Args)
	}
	if len(worker.Resources) != 1 || worker.Resources[0].Name != "omnivoice-cache" || worker.Resources[0].Capacity != 1 {
		t.Fatalf("worker resources = %#v, want omnivoice-cache capacity 1", worker.Resources)
	}
	if len(worker.Operations) != 1 || len(worker.Operations[0].Inputs) != 1 || len(worker.Operations[0].Inputs[0].ContentTypes) != 1 || worker.Operations[0].Inputs[0].ContentTypes[0] != interfaces.ModelOperationContentTypeText {
		t.Fatalf("worker input operations = %#v, want text input content type", worker.Operations)
	}
	if len(worker.Operations[0].Outputs) != 1 || len(worker.Operations[0].Outputs[0].ContentTypes) != 1 || worker.Operations[0].Outputs[0].ContentTypes[0] != interfaces.ModelOperationContentTypeAudio {
		t.Fatalf("worker output operations = %#v, want audio output content type", worker.Operations)
	}
	if worker.Auth == nil || worker.Auth.SecretRef != "secret/local-model" {
		t.Fatalf("worker auth = %#v, want secret/local-model", worker.Auth)
	}
	if worker.Linear == nil {
		t.Fatal("worker linear config = nil, want values")
	}
	if len(worker.Linear.TeamIDs) != 1 || worker.Linear.TeamIDs[0] != "team-a" {
		t.Fatalf("worker linear team IDs = %#v, want team-a", worker.Linear.TeamIDs)
	}
	if len(worker.Linear.StateIDs) != 1 || worker.Linear.StateIDs[0] != "state-ready" {
		t.Fatalf("worker linear state IDs = %#v, want state-ready", worker.Linear.StateIDs)
	}
	if worker.Linear.Claim == nil || worker.Linear.Claim.AssigneeField != "owner" {
		t.Fatalf("worker linear claim = %#v, want owner", worker.Linear.Claim)
	}
}

type recordingModelRunnerFunc func(context.Context, interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error)

func (fn recordingModelRunnerFunc) Execute(ctx context.Context, request interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	return fn(ctx, request)
}
