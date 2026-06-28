package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestLoadWorkersFromConfig_InferenceWorkerUsesModelHostLeases(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	cache := localModelTestCacheLayout(t)
	puller := staticModelAssetPuller{cache: cache}
	launcher := &serviceTestFakeProcessLauncher{healthEndpoint: healthServer.URL}
	domain := modelHostBackedLocalModelDomain(t, puller, launcher, runtime)

	factoryCfg := inferenceModelHostFactoryConfig()
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, inferenceModelHostRuntimeWorkers(healthServer.URL), map[string]*interfaces.FactoryWorkstationConfig{
		"speak": inferenceModelHostWorkstation(),
	})

	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		nil,
		logging.NoopLogger{},
		true,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		domain,
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

	result, err := wsExec.Execute(context.Background(), inferenceModelHostDispatch())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	assertModelInvokeAcceptedAudioOutput(t, result.Output, audioPath)
	if launcher.startCount() != 1 {
		t.Fatalf("supervised process start count = %d, want 1 shared host runtime", launcher.startCount())
	}
	if runtime.loadCount() != 1 || runtime.invocationCount() != 1 {
		t.Fatalf("runtime load/invoke counts = %d/%d, want 1/1", runtime.loadCount(), runtime.invocationCount())
	}
	if got := runtime.lastLoadServingEndpoint(); got != healthServer.URL {
		t.Fatalf("serving endpoint = %q, want supervised lease endpoint %q", got, healthServer.URL)
	}
}

func TestFactorySessionInvocation_LocalLlamaCppInferenceUsesModelHostLeases(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}

	server, launcher, svc, shutdown := startLocalModelInferenceTestServer(t, runtime)
	defer shutdown()
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := svc.InvokeFactorySession(ctx, factorysessions.DefaultSessionID, factoryapi.InvocationRequest{
		SourceKind: factoryapi.InvocationInputSourceKindText,
		Content: factoryapi.WorkContent{
			mustGeneratedLocalModelHTTPTextPart(t, "hello factory session inference"),
		},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	if result.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED (error=%q message=%q work=%q state=%q)", result.Status, result.ErrorCode, result.Message, result.WorkName, result.WorkState)
	}
	if len(result.PrimaryResult) == 0 {
		t.Fatalf("primaryResult = %#v, want invocation to participate in primary-result selection", result.PrimaryResult)
	}
	events, err := svc.GetFactoryEvents(ctx)
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	assertModelHostInferenceEventsInHistory(t, events, audioPath)
	if launcher.startCount() != 1 {
		t.Fatalf("supervised process start count = %d, want 1 shared host runtime", launcher.startCount())
	}
	if runtime.loadCount() != 1 || runtime.invocationCount() != 1 {
		t.Fatalf("runtime load/invoke counts = %d/%d, want 1/1", runtime.loadCount(), runtime.invocationCount())
	}
	if got := runtime.lastLoadServingEndpoint(); got != launcher.healthEndpoint {
		t.Fatalf("serving endpoint = %q, want supervised lease endpoint %q", got, launcher.healthEndpoint)
	}
}

func TestWorkerWorkstationTaxonomyRuntime_InferenceWorkerUsesModelHostLeases(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	cfg := mustTaxonomyRuntimeFactoryConfigFromOpenAPI(
		t,
		interfaces.WorkerTypeInference,
		interfaces.WorkstationTypeInference,
		interfaces.WorkerTypeModel,
		interfaces.WorkstationTypeInvoke,
	)
	wsExec, launcher := taxonomyOmniVoiceInferenceWorkstationExecutorWithModelHost(t, runtime, cfg, healthServer.URL)

	result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	assertModelInvokeAcceptedAudioOutput(t, result.Output, audioPath)
	if launcher.startCount() != 1 {
		t.Fatalf("supervised process start count = %d, want 1 shared host runtime", launcher.startCount())
	}
	if got := runtime.lastLoadServingEndpoint(); got != healthServer.URL {
		t.Fatalf("serving endpoint = %q, want supervised lease endpoint %q", got, healthServer.URL)
	}
}

func modelHostBackedLocalModelDomain(
	t *testing.T,
	puller modelAssetPuller,
	launcher *serviceTestFakeProcessLauncher,
	runtime *fakeLocalModelRuntime,
) localModelDomain {
	t.Helper()
	host := newServiceTestSupervisedModelHost(t, puller, launcher)
	leaseExec := modelhost.NewLeaseExecution(host, puller, runtime, localModelHooks())
	return localModelDomain{
		resources:      newLocalModelResourceLimiter(),
		assets:         puller,
		runtime:        runtime,
		manager:        newManagedLocalModelManager(puller, runtime),
		host:           host,
		leaseExecution: leaseExec,
	}
}

func inferenceModelHostFactoryConfig() *interfaces.FactoryConfig {
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

func inferenceModelHostRuntimeWorkers(healthEndpoint string) map[string]*interfaces.WorkerConfig {
	worker := &interfaces.WorkerConfig{
		Name:          "tts-worker",
		Type:          interfaces.WorkerTypeInference,
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
	}
	if strings.TrimSpace(healthEndpoint) != "" {
		worker.Args = []string{"--health-endpoint", healthEndpoint}
	}
	return map[string]*interfaces.WorkerConfig{"tts-worker": worker}
}

func inferenceModelHostWorkstation() *interfaces.FactoryWorkstationConfig {
	return &interfaces.FactoryWorkstationConfig{
		Name:           "speak",
		Type:           interfaces.WorkstationTypeInference,
		WorkerTypeName: "tts-worker",
		Operation:      "TTS",
		OperationBindings: []interfaces.ModelOperationBinding{{
			Slot: "text",
			Selector: &interfaces.ModelOperationBindingSelector{
				Label: "utterance",
				Type:  interfaces.ModelOperationContentTypeText,
			},
		}},
	}
}

func inferenceModelHostDispatch() interfaces.WorkDispatch {
	return interfaces.WorkDispatch{
		DispatchID:      "dispatch-inference-lease",
		TransitionID:    "transition-inference-lease",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "token-inference-lease",
			Color: interfaces.TokenColor{
				WorkID: "work-inference-lease",
				Content: []interfaces.WorkContentPart{{
					Type:  interfaces.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello inference lease",
				}},
			},
		}),
	}
}

func taxonomyOmniVoiceInferenceWorkstationExecutorWithModelHost(
	t *testing.T,
	runtime *fakeLocalModelRuntime,
	cfg *interfaces.FactoryConfig,
	healthEndpoint string,
) (*workers.WorkstationExecutor, *serviceTestFakeProcessLauncher) {
	t.Helper()

	cache := localModelTestCacheLayout(t)
	puller := staticModelAssetPuller{cache: cache}
	launcher := &serviceTestFakeProcessLauncher{healthEndpoint: healthEndpoint}
	domain := modelHostBackedLocalModelDomain(t, puller, launcher, runtime)
	factoryCfg := localModelFactoryConfig()
	runtimeWorkers := inferenceModelHostRuntimeWorkers(healthEndpoint)
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, runtimeWorkers, map[string]*interfaces.FactoryWorkstationConfig{
		"speak": taxonomyRuntimeModelInvokeWorkstation(cfg.Workstations[0].Type),
	})
	eventTime := taxonomyRuntimeEventTime()
	history := factoryevents.NewFactoryEventHistory(nil, func() time.Time { return eventTime }, runtimeCfg)
	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		nil,
		logging.NoopLogger{},
		true,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		history.RecordModelEvent,
		func() time.Time { return eventTime },
		domain,
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
	return wsExec, launcher
}

func (r *fakeLocalModelRuntime) lastLoadServingEndpoint() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.loads) == 0 {
		return ""
	}
	return strings.TrimSpace(r.loads[0].ServingEndpoint)
}

func assertModelHostInferenceEventsInHistory(t *testing.T, events []factoryapi.FactoryEvent, audioPath string) {
	t.Helper()
	modelEvents := make([]factoryapi.FactoryEvent, 0, 2)
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeModelRequest || event.Type == factoryapi.FactoryEventTypeModelResponse {
			modelEvents = append(modelEvents, event)
		}
	}
	assertRecordedLocalModelExecutionEvents(t, modelEvents, audioPath)
}
