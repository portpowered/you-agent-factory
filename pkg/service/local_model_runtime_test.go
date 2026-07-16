package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

type staticModelAssetPuller struct {
	pullResult apisurface.ModelPullResult
	pullErr    error
	ensureErr  error
	cache      localModelCacheLayout
	cacheErr   error
}

func (s staticModelAssetPuller) PullModel(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ string) (apisurface.ModelPullResult, error) {
	return s.pullResult, s.pullErr
}

func (s staticModelAssetPuller) EnsureModelAvailable(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ *workerconfig.Config) error {
	return s.ensureErr
}

func (s staticModelAssetPuller) ResolveModelCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ *workerconfig.Config) (localModelCacheLayout, error) {
	return s.cache, s.cacheErr
}

func (s staticModelAssetPuller) InspectRuntimeCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ string) (localmodels.RuntimeCacheInspection, error) {
	if strings.TrimSpace(s.cache.ModelName) == "" {
		return localmodels.RuntimeCacheInspection{}, nil
	}
	return localmodels.RuntimeCacheInspection{
		Supported:          true,
		Installed:          true,
		CachePath:          s.cache.CachePath,
		Revision:           s.cache.Revision,
		InstalledFileCount: len(s.cache.Files),
	}, nil
}

type fakeLocalModelRuntime struct {
	mu          sync.Mutex
	loads       []localModelLoadRequest
	invocations []localModelInvocationRequest
	response    workerexecution.InferenceResponse
	loadErr     error
	invokeErr   error
}

func (r *fakeLocalModelRuntime) Supports(resource factoryresource.Config, worker *workerconfig.Config) bool {
	return localmodels.CanonicalBackendName(resource.Backend) == "LLAMACPP" && localmodels.CanonicalModelName(worker.Model) == localmodels.CanonicalModelName("OMNIVOICE_Q4_K_M")
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

func (h fakeLocalModelHandle) Invoke(_ context.Context, request localModelInvocationRequest) (workerexecution.InferenceResponse, error) {
	h.runtime.mu.Lock()
	defer h.runtime.mu.Unlock()

	h.runtime.invocations = append(h.runtime.invocations, request)
	if h.runtime.invokeErr != nil {
		return workerexecution.InferenceResponse{}, h.runtime.invokeErr
	}
	return h.runtime.response, nil
}

func TestInvokeModel_UsesModelHostLeasesAndReusesLoadedRuntime(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		"",
		localModelFactoryConfig(),
		localModelRuntimeWorkersWithHealthEndpoint(healthServer.URL),
		nil,
	)
	cache := localModelTestCacheLayout(t)
	puller := staticModelAssetPuller{cache: cache}
	launcher := &serviceTestFakeProcessLauncher{healthEndpoint: healthServer.URL}
	host := newServiceTestSupervisedModelHost(t, puller, launcher)
	leaseExec := modelhost.NewLeaseExecution(host, puller, runtime, localModelHooks())
	svc := &FactoryService{
		policy:      serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{}),
		modelAssets: puller,
		cfg:         serviceTestConfigWithWorkerApplication(t, &FactoryServiceConfig{}),
	}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
		RuntimeCfg:        runtimeCfg,
		ModelAssets:       puller,
		LocalModelRuntime: runtime,
		ModelHost:         host,
		LeaseExecution:    leaseExec,
		ModelResources:    newLocalModelResourceLimiter(),
		LocalModels:       newManagedLocalModelManager(puller, runtime),
	})
	attachModelServiceForTest(t, svc)

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
	if launcher.startCount() != 1 {
		t.Fatalf("supervised process start count = %d, want 1 shared host runtime", launcher.startCount())
	}
	if runtime.loadCount() != 1 {
		t.Fatalf("runtime load count = %d, want 1 reused handle across host leases", runtime.loadCount())
	}
	runtime.mu.Lock()
	servingEndpoint := ""
	if len(runtime.loads) > 0 {
		servingEndpoint = strings.TrimSpace(runtime.loads[0].ServingEndpoint)
	}
	runtime.mu.Unlock()
	if servingEndpoint != healthServer.URL {
		t.Fatalf("serving endpoint = %q, want supervised lease endpoint %q", servingEndpoint, healthServer.URL)
	}
	if runtime.invocationCount() != 2 {
		t.Fatalf("invocation count = %d, want 2", runtime.invocationCount())
	}
	if len(first.Content) != 1 || first.Content[0].Type != work.WorkContentPartTypeAudio || first.StreamFile != audioPath || first.StreamContentType != "audio/wav" {
		t.Fatalf("first result = %#v, want audio content and stream metadata", first)
	}
	if len(second.Content) != 1 || second.Content[0].Type != work.WorkContentPartTypeAudio {
		t.Fatalf("second result = %#v, want audio content", second)
	}
}

func TestLoadWorkersFromConfig_LocalModelWorkerUsesManagedRuntimePath(t *testing.T) {
	provider := &providerCallRecorder{}
	wsExec, runtime := localModelManagedRuntimeWorkerExecutor(t, provider)

	result, err := wsExec.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-tts",
		TransitionID:    "transition-tts",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		InputTokens: workers.InputTokens(factorytoken.Token{
			ID: "token-tts",
			Color: factorytoken.Color{
				WorkID: "work-tts",
				Content: []work.WorkContentPart{{
					Type:  work.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello world",
				}},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
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
	result, err := wsExec.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-tts",
		TransitionID:    "transition-tts",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		Execution: work.ExecutionMetadata{
			CurrentTick: 2,
			RequestID:   "request-tts",
			TraceID:     "trace-tts",
			WorkIDs:     []string{"work-tts"},
		},
		InputTokens: workers.InputTokens(factorytoken.Token{
			ID: "token-tts",
			Color: factorytoken.Color{
				WorkID: "work-tts",
				Content: []work.WorkContentPart{{
					Type:  work.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello world",
				}},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	assertRecordedLocalModelExecutionEvents(t, generatedFactoryEventsForTest(t, history.CanonicalEvents()), audioPath)
}

func TestLoadWorkersFromConfig_LocalModelWorkerDetachesClonedWorkerRequestsFromLaterSourceMutation(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
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
					Type:  workerconfig.ModelOperationContentTypeText,
				},
			}},
		},
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
		nil,
		nil,
		LocalModelDomain{
			Resources: newLocalModelResourceLimiter(),
			Assets:    staticModelAssetPuller{cache: cache},
			Runtime:   runtime,
			Manager:   newManagedLocalModelManager(staticModelAssetPuller{cache: cache}, runtime),
		},
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
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
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
	var events []workerexecution.ModelEvent
	runner := newRecordingModelRunner(
		recordingModelRunnerFunc(func(_ context.Context, _ workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
			return workerexecution.RunnerExecutionResult{
				Content: mustMarshalAudioContentResponse(t, filepath.Join(t.TempDir(), "speech.wav")),
			}, nil
		}),
		localModelFactoryConfig(),
		workerDef,
		func(event workerexecution.ModelEvent) {
			events = append(events, event)
		},
		func() time.Time { return eventTime },
	)

	mutateLocalModelWorkerCloneSource(workerDef)

	_, err := runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-tts",
			Execution: work.ExecutionMetadata{
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
	requestPayload := events[0].Request
	if requestPayload == nil {
		t.Fatal("model request payload = nil")
	}
	if requestPayload.Worker != "tts-worker" {
		t.Fatalf("request worker = %q, want tts-worker", requestPayload.Worker)
	}
	if requestPayload.Model != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("request model = %q, want OMNIVOICE_Q4_K_M", requestPayload.Model)
	}
	if requestPayload.ProviderLocality != workerconfig.ModelLocalityLocal {
		t.Fatalf("request locality = %q, want %q", requestPayload.ProviderLocality, workerconfig.ModelLocalityLocal)
	}
	if requestPayload.Resources == nil || len(*requestPayload.Resources) != 1 || (*requestPayload.Resources)[0].Name != "omnivoice-cache" {
		t.Fatalf("request resources = %#v, want omnivoice-cache", requestPayload.Resources)
	}
	responsePayload := events[1].Response
	if responsePayload == nil {
		t.Fatal("model response payload = nil")
	}
	if responsePayload.Resources == nil || len(*responsePayload.Resources) != 1 || (*responsePayload.Resources)[0].Name != "omnivoice-cache" {
		t.Fatalf("response resources = %#v, want omnivoice-cache", responsePayload.Resources)
	}
}

func localModelManagedRuntimeWorkerExecutor(t *testing.T, provider *providerCallRecorder) (*workers.WorkstationExecutor, *fakeLocalModelRuntime) {
	t.Helper()
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
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
					Type:  workerconfig.ModelOperationContentTypeText,
				},
			}},
		},
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
		provider,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		LocalModelDomain{
			Resources: newLocalModelResourceLimiter(),
			Assets: staticModelAssetPuller{
				cache: cache,
			},
			Runtime: runtime,
			Manager: newManagedLocalModelManager(staticModelAssetPuller{
				cache: cache,
			}, runtime),
		},
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
	return wsExec, runtime
}

func localModelExecutionRecorderFixture(t *testing.T, eventTime time.Time) (*workers.WorkstationExecutor, *factoryevents.FactoryEventHistory, string) {
	t.Helper()
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
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
					Type:  workerconfig.ModelOperationContentTypeText,
				},
			}},
		},
	})
	history := factoryevents.NewFactoryEventHistory(nil, func() time.Time { return eventTime }, runtimeCfg)
	opts, err := loadWorkersFromConfig("", factoryCfg, "", runtimeCfg, nil, logging.NoopLogger{}, true, nil, nil, nil, nil, nil, nil, nil, history.RecordModelEvent, nil, func() time.Time { return eventTime }, LocalModelDomain{
		Resources: newLocalModelResourceLimiter(),
		Assets:    staticModelAssetPuller{cache: cache},
		Runtime:   runtime,
		Manager:   newManagedLocalModelManager(staticModelAssetPuller{cache: cache}, runtime),
	})
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

func TestModelEventContext_NormalizesEventTimeToUTC(t *testing.T) {
	localZone := time.FixedZone("Model/Local", 9*60*60)
	event := modelEvent(workerexecution.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-model",
			Execution:  work.ExecutionMetadata{CurrentTick: 6},
		},
	}, workerexecution.ModelEventKindRequest, "model-request", time.Date(2026, 4, 20, 21, 15, 0, 0, localZone), &workerexecution.ModelRequestEventPayload{}, nil)

	if event.EventTime.Location() != time.UTC {
		t.Fatalf("event time location = %v, want UTC", event.EventTime.Location())
	}
	if got, want := event.EventTime.Format(time.RFC3339), "2026-04-20T12:15:00Z"; got != want {
		t.Fatalf("event time = %q, want %q", got, want)
	}
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
	if audioErr != nil || generatedAudioFileValue(audioPart.File) != audioPath {
		t.Fatalf("response output audio = %#v, %v, want file %q", responsePayload.OutputContent, audioErr, audioPath)
	}
}

func localModelFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Resources: []factoryresource.Config{{
			Name:       "omnivoice-cache",
			Type:       factoryresource.TypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "speak",
			WorkerTypeName: "tts-worker",
		}},
		Workers: []workerconfig.Config{{
			Name: "tts-worker",
		}},
	}
}

func localModelRuntimeWorkers() map[string]*workerconfig.Config {
	return localModelRuntimeWorkersWithHealthEndpoint("")
}

func localModelRuntimeWorkersWithHealthEndpoint(healthEndpoint string) map[string]*workerconfig.Config {
	worker := &workerconfig.Config{
		Name:          "tts-worker",
		Type:          interfaces.WorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelProvider: workerexecution.RunnerIDCodex,
		ModelLocality: workerconfig.ModelLocalityLocal,
		Resources: []factoryresource.Config{{
			Name:     "omnivoice-cache",
			Capacity: 1,
		}},
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
	if strings.TrimSpace(healthEndpoint) != "" {
		worker.Args = []string{"--health-endpoint", healthEndpoint}
	}
	return map[string]*workerconfig.Config{
		"tts-worker": worker,
	}
}

func localModelRuntimeWorkersWithCloneCoverage() map[string]*workerconfig.Config {
	return map[string]*workerconfig.Config{
		"tts-worker": {
			Name:          "tts-worker",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelProvider: workerexecution.RunnerIDCodex,
			ModelLocality: workerconfig.ModelLocalityLocal,
			Args:          []string{"--voice", "alloy"},
			Resources: []factoryresource.Config{{
				Name:     "omnivoice-cache",
				Capacity: 1,
			}},
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
			Auth: &workerconfig.HostedWorkerAuthConfig{
				SecretRef: "secret/local-model",
			},
			Linear: &workerconfig.HostedLinearWorkerConfig{
				PollInterval: "30s",
				TeamIDs:      []string{"team-a"},
				StateIDs:     []string{"state-ready"},
				Claim: &workerconfig.HostedLinearWorkerClaimConfig{
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
	content := []work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
	}}
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal audio content response: %v", err)
	}
	return string(data)
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

func localModelDispatch() work.WorkDispatch {
	return work.WorkDispatch{
		DispatchID:      "dispatch-tts",
		TransitionID:    "transition-tts",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		InputTokens: workers.InputTokens(factorytoken.Token{
			ID: "token-tts",
			Color: factorytoken.Color{
				WorkID: "work-tts",
				Content: []work.WorkContentPart{{
					Type:  work.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello world",
				}},
			},
		}),
	}
}

func mutateLocalModelWorkerCloneSource(worker *workerconfig.Config) {
	worker.Args[0] = "--mutated"
	worker.Resources[0].Name = "mutated-cache"
	worker.Resources[0].Capacity = 9
	worker.Operations[0].Inputs[0].ContentTypes[0] = workerconfig.ModelOperationContentTypeJSON
	worker.Operations[0].Outputs[0].ContentTypes[0] = workerconfig.ModelOperationContentTypeBinary
	worker.Auth.SecretRef = "secret/mutated"
	worker.Linear.TeamIDs[0] = "team-b"
	worker.Linear.StateIDs[0] = "state-done"
	worker.Linear.Claim.AssigneeField = "mutated-owner"
	worker.Model = "mutated-model"
	worker.ModelLocality = workerconfig.ModelLocalityCloud
}

func assertLocalModelCloneCoverageWorker(t *testing.T, worker *workerconfig.Config) {
	t.Helper()
	if worker == nil {
		t.Fatal("expected cloned worker, got nil")
	}
	assertLocalModelCloneCoverageArgs(t, worker)
	assertLocalModelCloneCoverageResources(t, worker)
	assertLocalModelCloneCoverageOperations(t, worker)
	assertLocalModelCloneCoverageAuth(t, worker)
	assertLocalModelCloneCoverageLinear(t, worker)
}

func assertLocalModelCloneCoverageArgs(t *testing.T, worker *workerconfig.Config) {
	t.Helper()
	if len(worker.Args) != 2 || worker.Args[0] != "--voice" || worker.Args[1] != "alloy" {
		t.Fatalf("worker args = %#v, want [--voice alloy]", worker.Args)
	}
}

func assertLocalModelCloneCoverageResources(t *testing.T, worker *workerconfig.Config) {
	t.Helper()
	if len(worker.Resources) != 1 || worker.Resources[0].Name != "omnivoice-cache" || worker.Resources[0].Capacity != 1 {
		t.Fatalf("worker resources = %#v, want omnivoice-cache capacity 1", worker.Resources)
	}
}

func assertLocalModelCloneCoverageOperations(t *testing.T, worker *workerconfig.Config) {
	t.Helper()
	if len(worker.Operations) != 1 || len(worker.Operations[0].Inputs) != 1 || len(worker.Operations[0].Inputs[0].ContentTypes) != 1 || worker.Operations[0].Inputs[0].ContentTypes[0] != workerconfig.ModelOperationContentTypeText {
		t.Fatalf("worker input operations = %#v, want text input content type", worker.Operations)
	}
	if len(worker.Operations[0].Outputs) != 1 || len(worker.Operations[0].Outputs[0].ContentTypes) != 1 || worker.Operations[0].Outputs[0].ContentTypes[0] != workerconfig.ModelOperationContentTypeAudio {
		t.Fatalf("worker output operations = %#v, want audio output content type", worker.Operations)
	}
}

func assertLocalModelCloneCoverageAuth(t *testing.T, worker *workerconfig.Config) {
	t.Helper()
	if worker.Auth == nil || worker.Auth.SecretRef != "secret/local-model" {
		t.Fatalf("worker auth = %#v, want secret/local-model", worker.Auth)
	}
}

func assertLocalModelCloneCoverageLinear(t *testing.T, worker *workerconfig.Config) {
	t.Helper()
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

type recordingModelRunnerFunc func(context.Context, workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error)

func (fn recordingModelRunnerFunc) Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	return fn(ctx, request)
}

type blockingRunner struct {
	mu          sync.Mutex
	current     int
	maxObserved int
	enterCh     chan struct{}
	releaseCh   chan struct{}
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{
		enterCh:   make(chan struct{}, 8),
		releaseCh: make(chan struct{}),
	}
}

func (r *blockingRunner) Execute(_ context.Context, _ workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	r.mu.Lock()
	r.current++
	if r.current > r.maxObserved {
		r.maxObserved = r.current
	}
	r.mu.Unlock()

	r.enterCh <- struct{}{}
	<-r.releaseCh

	r.mu.Lock()
	r.current--
	r.mu.Unlock()
	return workerexecution.RunnerExecutionResult{Content: "ok"}, nil
}

func (r *blockingRunner) MaxObserved() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxObserved
}

func TestLocalModelResourceLimiter_BoundsSharedLocalModelConcurrencyAcrossSessions(t *testing.T) {
	limiter := newLocalModelResourceLimiter()
	factoryCfg := &interfaces.FactoryConfig{
		Resources: []factoryresource.Config{{
			Name:       "omnivoice-cache",
			Type:       factoryresource.TypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	}
	workerDef := &workerconfig.Config{
		Name:          "tts-worker",
		ModelLocality: workerconfig.ModelLocalityLocal,
		Resources:     []factoryresource.Config{{Name: "omnivoice-cache", Capacity: 1}},
	}

	inner := newBlockingRunner()
	first := limiter.WrapRunner(inner, factoryCfg, workerDef)
	second := limiter.WrapRunner(inner, factoryCfg, workerDef)

	ctx := context.Background()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = first.Execute(ctx, workerexecution.RunnerExecutionRequest{})
	}()

	select {
	case <-inner.enterCh:
	case <-time.After(2 * time.Second):
		t.Fatal("first execution did not enter runner")
	}

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		_, _ = second.Execute(ctx, workerexecution.RunnerExecutionRequest{})
	}()

	select {
	case <-inner.enterCh:
		t.Fatal("second execution entered before shared local model resource was released")
	case <-time.After(150 * time.Millisecond):
	}

	close(inner.releaseCh)

	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("second execution did not complete after release")
	}
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first execution did not complete after release")
	}

	if got := inner.MaxObserved(); got != 1 {
		t.Fatalf("max observed local-model concurrency = %d, want 1", got)
	}
}
