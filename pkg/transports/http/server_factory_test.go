// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	testCodeDuplicateIdentifier              = "factory.duplicateIdentifier"
	testCodeDanglingWorkerReference          = "factory.worker.danglingReference"
	testCodeDanglingPlaceReference           = "factory.route.danglingPlaceReference"
	testCodeWorkstationMissingRejectionRoute = "factory.workstation.missingRejectionRoute"
	testCodeWorkstationMissingFailureRoute   = "factory.workstation.missingFailureRoute"
	testCodeWorkTypeMissingCompletionState   = "factory.workType.missingCompletionState"
)

type strictFactorySaveAPIFake struct {
	getCurrent  func(context.Context, string) (factoryapi.Factory, error)
	save        func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error)
	saveCurrent func(context.Context, string, factoryapi.Factory) (factoryapi.Factory, error)
}

type strictWorkAPIFake struct {
	submit    func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	move      func(context.Context, string, string, string, string) (work.OperatorMoveResult, error)
	subscribe func(context.Context, string, *workerconfig.FactoryEventReconnectCursor) (*workerconfig.FactoryEventStream, error)
	probe     func(context.Context, string, *workerconfig.FactoryEventReconnectCursor) error
	snapshot  func(context.Context, string) (*workerconfig.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net], error)
	list      func(context.Context, string, work.ListOptions) (work.ListResult, error)
	getWork   func(context.Context, string, string) (work.ReadModel, error)
	moveRead  func(context.Context, string, string, string, string) (work.ReadModel, error)
}

func (fake strictWorkAPIFake) ListWork(ctx context.Context, sessionID string, options work.ListOptions) (work.ListResult, error) {
	if fake.list == nil {
		panic("unexpected WorkReadAPI.ListWork call")
	}
	return fake.list(ctx, sessionID, options)
}

func (fake strictWorkAPIFake) GetWork(ctx context.Context, sessionID, id string) (work.ReadModel, error) {
	if fake.getWork == nil {
		panic("unexpected WorkReadAPI.GetWork call")
	}
	return fake.getWork(ctx, sessionID, id)
}

func (fake strictWorkAPIFake) MoveWorkAndRead(ctx context.Context, sessionID, id, stateName, requestID string) (work.ReadModel, error) {
	if fake.moveRead == nil {
		panic("unexpected WorkReadAPI.MoveWorkAndRead call")
	}
	return fake.moveRead(ctx, sessionID, id, stateName, requestID)
}

type strictLiveSessionAPIFake struct {
	list      func(context.Context) (factoryapi.ListFactorySessionsResponse, error)
	get       func(context.Context, string) (factoryapi.FactorySession, error)
	preflight func(context.Context, string, workerconfig.FactorySessionSyncPreflightOptions) (factoryapi.FactorySessionSyncPreflightResponse, error)
	result    func(context.Context, string) (factoryapi.FactorySessionLiveResult, error)
	partial   func(context.Context, string) (factoryapi.FactorySessionPartialResult, error)
	open      func(context.Context, factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	close     func(context.Context, string) error
	pause     func(context.Context, string, factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	resume    func(context.Context, string, factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	subscribe func(context.Context, factorysessions.ResponseEventSubscriptionRequest) (apisurface.FactoryResponseEventSubscription, error)
}

func (fake strictLiveSessionAPIFake) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	if fake.get == nil {
		panic("unexpected LiveSessionAPI.GetFactorySession call")
	}
	return fake.get(ctx, sessionID)
}

func (fake strictLiveSessionAPIFake) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	if fake.list == nil {
		panic("unexpected LiveSessionAPI.ListFactorySessions call")
	}
	return fake.list(ctx)
}

func (fake strictLiveSessionAPIFake) GetFactorySessionSyncPreflight(ctx context.Context, sessionID string, options workerconfig.FactorySessionSyncPreflightOptions) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	if fake.preflight == nil {
		panic("unexpected LiveSessionAPI.GetFactorySessionSyncPreflight call")
	}
	return fake.preflight(ctx, sessionID, options)
}

func (fake strictLiveSessionAPIFake) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	if fake.result == nil {
		panic("unexpected LiveSessionAPI.GetFactorySessionResult call")
	}
	return fake.result(ctx, sessionID)
}

func (fake strictLiveSessionAPIFake) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error) {
	if fake.partial == nil {
		panic("unexpected LiveSessionAPI.GetFactorySessionPartialResult call")
	}
	return fake.partial(ctx, sessionID)
}

func (fake strictLiveSessionAPIFake) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	if fake.open == nil {
		panic("unexpected LiveSessionAPI.OpenFactorySession call")
	}
	return fake.open(ctx, request)
}

func (fake strictLiveSessionAPIFake) CloseFactorySession(ctx context.Context, sessionID string) error {
	if fake.close == nil {
		panic("unexpected LiveSessionAPI.CloseFactorySession call")
	}
	return fake.close(ctx, sessionID)
}

func (fake strictLiveSessionAPIFake) PauseLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if fake.pause == nil {
		panic("unexpected LiveSessionAPI.PauseLiveFactorySession call")
	}
	return fake.pause(ctx, sessionID, request)
}

func (fake strictLiveSessionAPIFake) ResumeLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if fake.resume == nil {
		panic("unexpected LiveSessionAPI.ResumeLiveFactorySession call")
	}
	return fake.resume(ctx, sessionID, request)
}

func (fake strictLiveSessionAPIFake) SubscribeFactoryResponseEventsForSession(ctx context.Context, request factorysessions.ResponseEventSubscriptionRequest) (apisurface.FactoryResponseEventSubscription, error) {
	if fake.subscribe == nil {
		panic("unexpected LiveSessionAPI.SubscribeFactoryResponseEventsForSession call")
	}
	return fake.subscribe(ctx, request)
}

func (fake strictWorkAPIFake) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if fake.submit == nil {
		panic("unexpected WorkAPI.SubmitWorkRequestForSession call")
	}
	return fake.submit(ctx, sessionID, request)
}

func (fake strictWorkAPIFake) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	if fake.move == nil {
		panic("unexpected WorkAPI.MoveWorkForSession call")
	}
	return fake.move(ctx, sessionID, workID, stateName, requestID)
}

func (fake strictWorkAPIFake) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *workerconfig.FactoryEventReconnectCursor) (*workerconfig.FactoryEventStream, error) {
	if fake.subscribe == nil {
		panic("unexpected WorkAPI.SubscribeFactoryEventsForSession call")
	}
	return fake.subscribe(ctx, sessionID, reconnect)
}

func (fake strictWorkAPIFake) ProbeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *workerconfig.FactoryEventReconnectCursor) error {
	if fake.probe == nil {
		panic("unexpected WorkAPI.ProbeFactoryEventsForSession call")
	}
	return fake.probe(ctx, sessionID, reconnect)
}

func (fake strictWorkAPIFake) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*workerconfig.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net], error) {
	if fake.snapshot == nil {
		panic("unexpected WorkAPI.GetEngineStateSnapshotForSession call")
	}
	return fake.snapshot(ctx, sessionID)
}

func newWorkTransportTestServer(workAPI apisurface.WorkAPI) *Server {
	return newWorkTransportTestServerWithRoles(
		nil,
		workAPI,
		factoryReadFake(factoryapi.Factory{Name: "test-factory"}, nil),
	)
}

func newWorkTransportTestServerWithRoles(sessions apisurface.LiveSessionAPI, workAPI apisurface.WorkAPI, definitions apisurface.FactorySaveAPI) *Server {
	workRead, _ := workAPI.(apisurface.WorkReadAPI)
	var liveLister factorysessions.LiveSessionListReader
	if sessions != nil {
		liveLister = httpLiveSessionListReader{sessions: sessions}
	}
	server := newServerFromRoles(
		nil, nil, sessions, workAPI, workRead, nil, &modelshttp.Handler{},
		definitions, httpFactoryValidator{}, nil,
		nil, nil, nil, nil, nil, liveLister, nil, nil,
		newContentStagingFake(),
		&workRequestPreparationFake{prepare: func(_ context.Context, input work.WorkRequestPreparation) (work.WorkRequest, error) {
			return input.Request, nil
		}},
		nil, nil,
	)
	return server
}

func acceptProgrammedHTTPWorkRequest(request work.WorkRequest) work.WorkRequestSubmitResult {
	requestID := request.RequestID
	if requestID == "" {
		requestID = "strict-http-role-id"
	}
	result := work.WorkRequestSubmitResult{RequestID: requestID, Accepted: true}
	for index, item := range request.Works {
		traceID := item.TraceID
		if traceID == "" {
			traceID = item.CurrentChainingTraceID
		}
		if traceID == "" {
			traceID = request.CurrentChainingTraceID
		}
		if traceID == "" {
			traceID = "strict-http-role-trace"
		}
		workID := item.WorkID
		if workID == "" {
			workID = "batch-" + requestID + "-" + item.Name
		}
		result.Works = append(result.Works, work.WorkRequestSubmittedWork{
			Name: item.Name, WorkTypeName: item.WorkTypeID, WorkID: workID,
		})
		if index == 0 {
			result.TraceID = traceID
			result.WorkID = workID
			result.Name = item.Name
			result.WorkTypeName = item.WorkTypeID
		}
	}
	return result
}

func (fake strictFactorySaveAPIFake) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	if fake.getCurrent == nil {
		panic("unexpected FactorySaveAPI.GetCurrentFactoryForSession call")
	}
	return fake.getCurrent(ctx, sessionID)
}

func (fake strictFactorySaveAPIFake) SaveFactoryForSession(ctx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
	if fake.save == nil {
		panic("unexpected FactorySaveAPI.SaveFactoryForSession call")
	}
	return fake.save(ctx, sessionID, mode, request)
}

func (fake strictFactorySaveAPIFake) SaveCurrentFactoryForSession(ctx context.Context, sessionID string, request factoryapi.Factory) (factoryapi.Factory, error) {
	if fake.saveCurrent == nil {
		panic("unexpected FactorySaveAPI.SaveCurrentFactoryForSession call")
	}
	return fake.saveCurrent(ctx, sessionID, request)
}

func newFactoryDefinitionTestServer(definitions apisurface.FactorySaveAPI, validator workerconfig.SubmittedDefinitionValidationOperation) *Server {
	if validator == nil {
		validator = httpFactoryValidator{}
	}
	return newServerFromRoles(
		nil, nil, nil, nil, nil, nil, &modelshttp.Handler{},
		definitions, validator, nil,
		nil, nil, nil, nil, nil, nil, nil,
		httpPromptTemplatesFake{}, nil, nil, nil, nil,
	)
}

func factoryReadFake(current factoryapi.Factory, readErr error) strictFactorySaveAPIFake {
	return strictFactorySaveAPIFake{getCurrent: func(context.Context, string) (factoryapi.Factory, error) {
		return current, readErr
	}}
}

func TestCreateFactoryRoute_RemovedFromRouter(t *testing.T) {
	srv := newFactoryDefinitionTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /factories status = %d, want route removed", rec.Code)
	}
}

func TestGetCurrentFactory_ReturnsFactoryShape(t *testing.T) {
	srv := newFactoryDefinitionTestServer(factoryReadFake(
		factoryapi.Factory{
			Name:      factoryapi.FactoryName("beta"),
			WorkTypes: &[]factoryapi.WorkType{{Name: "beta-task", States: []factoryapi.WorkState{{Name: "init", Type: factoryapi.WorkStateTypeINITIAL}, {Name: "done", Type: factoryapi.WorkStateTypeTERMINAL}}}},
		},
		nil,
	), nil)

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/factory", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	current := decodeJSONResponse[factoryapi.Factory](t, rec)
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
	if current.WorkTypes == nil || len(*current.WorkTypes) != 1 || (*current.WorkTypes)[0].Name != "beta-task" {
		t.Fatalf("current factory work types = %#v, want beta-task", current.WorkTypes)
	}
}

func TestGetCurrentFactory_AllowsDefaultRuntimeIdentifier(t *testing.T) {
	srv := newFactoryDefinitionTestServer(factoryReadFake(
		factoryapi.Factory{Name: apisurface.DefaultCurrentFactoryName, Id: stringPointerForAPITest("root-runtime")},
		nil,
	), nil)

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/factory", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	current := decodeJSONResponse[factoryapi.Factory](t, rec)
	if current.Name != apisurface.DefaultCurrentFactoryName || current.Id == nil || *current.Id != "root-runtime" {
		t.Fatalf("current factory = %#v, want default runtime identifier", current)
	}
}

func TestListModels_ReturnsDiscoveredModelSummaries(t *testing.T) {
	managedRuntime := modelinference.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: modelinference.ReadinessStateReady,
		LifecycleState: modelinference.LifecycleStateNotInstalled,
		Locality:       modelinference.LocalityLocal,
		SupportedOperations: []modelinference.Operation{{
			Name: "TTS",
		}},
	}
	srv := newStrictModelTestServer(strictModelsServiceFake{list: func(context.Context) (modelinference.List, error) {
		return modelinference.List{
			Results: []modelinference.Summary{{
				Name:             "OMNIVOICE_Q4_K_M",
				ManagedRuntime:   managedRuntime,
				ProviderLocality: modelinference.LocalityLocal,
				Status:           modelinference.StatusReady,
				LoadState:        modelinference.LoadStateUnloaded,
				Operations:       []modelinference.Operation{{Name: "TTS"}},
				Modalities:       []string{"AUDIO", "TEXT"},
				Resources:        []modelinference.ResourceSummary{{Name: "omnivoice-cache", Type: "MODEL", Capacity: 1}},
			}},
		}, nil
	}})

	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	response := decodeJSONResponse[factoryapi.ListModelsResponse](t, rec)
	if len(response.Results) != 1 || response.Results[0].Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("list models response = %#v, want OMNIVOICE model", response)
	}
	if response.Results[0].ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("managed readiness = %s, want READY", response.Results[0].ManagedRuntime.ReadinessState)
	}
}

func TestGetModel_ReturnsDiscoveredModelDetail(t *testing.T) {
	srv := newStrictModelTestServer(strictModelsServiceFake{get: func(_ context.Context, name string) (modelinference.Detail, error) {
		if name != "OMNIVOICE_Q4_K_M" {
			return modelinference.Detail{}, modelinference.ErrNotFound
		}
		return modelinference.Detail{
			Summary: modelinference.Summary{
				Name: "OMNIVOICE_Q4_K_M",
				ManagedRuntime: modelinference.Runtime{
					Identity:       "OMNIVOICE_Q4_K_M",
					ReadinessState: modelinference.ReadinessStateReady,
					LifecycleState: modelinference.LifecycleStateNotInstalled,
					Locality:       modelinference.LocalityLocal,
					SupportedOperations: []modelinference.Operation{{
						Name: "TTS",
					}},
				},
				ProviderLocality: modelinference.LocalityLocal,
				Status:           modelinference.StatusReady,
				LoadState:        modelinference.LoadStateUnloaded,
				Operations:       []modelinference.Operation{{Name: "TTS"}},
				Modalities:       []string{"AUDIO", "TEXT"},
				Resources:        []modelinference.ResourceSummary{{Name: "omnivoice-cache", Type: "MODEL", Capacity: 1}},
			},
			Capabilities: []modelinference.Capability{{
				Worker:           "voice-local",
				ProviderLocality: modelinference.LocalityLocal,
				Operations:       []modelinference.Operation{{Name: "TTS"}},
				ResourceNames:    []string{"omnivoice-cache"},
			}},
			Diagnostics: map[string]string{"workerCount": "1"},
		}, nil
	}})

	req := httptest.NewRequest(http.MethodGet, "/models/OMNIVOICE_Q4_K_M", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	model := decodeJSONResponse[factoryapi.ModelDetail](t, rec)
	if model.Name != "OMNIVOICE_Q4_K_M" || len(model.Capabilities) != 1 {
		t.Fatalf("model detail = %#v, want OMNIVOICE model capability detail", model)
	}
}

func TestGetModel_ReturnsNotFoundForUnknownDiscoveredModel(t *testing.T) {
	srv := newStrictModelTestServer(strictModelsServiceFake{get: func(context.Context, string) (modelinference.Detail, error) {
		return modelinference.Detail{}, modelinference.ErrNotFound
	}})
	req := httptest.NewRequest(http.MethodGet, "/models/MISSING", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "model not found")
}

func TestInvokeModel_ReturnsInvocationMetadata(t *testing.T) {
	var invokedModelNames []string
	srv := newStrictModelTestServer(strictModelsServiceFake{invoke: func(_ context.Context, name string, _ modelinference.Request) (modelinference.Result, error) {
		invokedModelNames = append(invokedModelNames, name)
		return modelinference.Result{
			ModelName:        "OMNIVOICE_Q4_K_M",
			Worker:           "tts-worker",
			Operation:        "TTS",
			ProviderLocality: workerconfig.ModelLocalityLocal,
			Content: []work.WorkContentPart{{
				Type:        work.WorkContentPartTypeAudio,
				File:        "artifacts/output.wav",
				ContentType: "audio/wav",
			}},
			Bindings: []modelinference.ResolvedModelOperationBinding{{
				Slot:   "text",
				Source: "INPUT",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "hello world",
				}},
			}},
		}, nil
	}})

	req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/invocations", bytes.NewBufferString(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello world"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(invokedModelNames) != 1 || invokedModelNames[0] != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("invoked model names = %#v, want OMNIVOICE_Q4_K_M", invokedModelNames)
	}

	response := decodeJSONResponse[factoryapi.ModelInvocationResponse](t, rec)
	if response.ModelName != "OMNIVOICE_Q4_K_M" || response.Worker != "tts-worker" || len(response.Bindings) != 1 || len(response.Content) != 1 {
		t.Fatalf("invoke response = %#v, want invocation metadata", response)
	}
}

func TestInvokeModel_StreamsAudioOutput(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	audioBytes := []byte("RIFF....WAVE")
	if err := os.WriteFile(audioPath, audioBytes, 0o644); err != nil {
		t.Fatalf("write audio file: %v", err)
	}
	srv := newStrictModelTestServer(strictModelsServiceFake{invoke: func(context.Context, string, modelinference.Request) (modelinference.Result, error) {
		return modelinference.Result{
			ModelName:         "OMNIVOICE_Q4_K_M",
			Worker:            "tts-worker",
			Operation:         "TTS",
			ProviderLocality:  workerconfig.ModelLocalityLocal,
			StreamFile:        audioPath,
			StreamContentType: "audio/wav",
		}, nil
	}})

	req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/invocations", bytes.NewBufferString(`{"operation":"TTS","options":{"responseMode":"AUDIO_STREAM"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "audio/wav" {
		t.Fatalf("content-type = %q, want audio/wav", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), audioBytes) {
		t.Fatalf("streamed body = %q, want %q", rec.Body.Bytes(), audioBytes)
	}
}

func TestInvokeModel_ReturnsModelNotAvailableWhenLocalAssetsAreMissing(t *testing.T) {
	srv := newStrictModelTestServer(strictModelsServiceFake{invoke: func(context.Context, string, modelinference.Request) (modelinference.Result, error) {
		return modelinference.Result{}, fmt.Errorf("%w: required assets missing", modelinference.ErrNotAvailable)
	}})

	req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/invocations", bytes.NewBufferString(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello world"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "MODEL_NOT_AVAILABLE", "model not available: required assets missing")
}

func TestPullModel_ReturnsManagedCachePullMetadata(t *testing.T) {
	var pulledModelNames []string
	srv := newStrictModelTestServer(strictModelsServiceFake{pull: func(_ context.Context, name string) (modelinference.PullResult, error) {
		pulledModelNames = append(pulledModelNames, name)
		return modelinference.PullResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: workerconfig.ModelLocalityLocal,
			Outcome:          "PULLED",
			CachePath:        "/tmp/models/OMNIVOICE_Q4_K_M/rev1",
			Revision:         "rev1",
			DownloadedFiles: []modelinference.DownloadedFile{{
				Path:   "omnivoice-base-Q4_K_M.gguf",
				Bytes:  407,
				SHA256: "abc123",
			}},
		}, nil
	}})

	req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/pull", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(pulledModelNames) != 1 || pulledModelNames[0] != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("pulled model names = %#v, want OMNIVOICE_Q4_K_M", pulledModelNames)
	}
	response := decodeJSONResponse[factoryapi.ModelPullResponse](t, rec)
	if response.Outcome != factoryapi.ModelPullOutcome("PULLED") || response.CachePath == "" || len(response.DownloadedFiles) != 1 {
		t.Fatalf("pull response = %#v, want pull metadata", response)
	}
	if response.ManagedRuntimePull.Identity != "OMNIVOICE_Q4_K_M" ||
		response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY ||
		response.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("managed runtime pull = %#v, want installed successfully", response.ManagedRuntimePull)
	}
}

func TestGetCurrentFactory_ReturnsDefinitionAndVersion(t *testing.T) {
	versionTime := time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC)
	current := factoryapi.Factory{
		Name:      factoryapi.FactoryName("beta"),
		WorkTypes: &[]factoryapi.WorkType{{Name: "beta-task", States: []factoryapi.WorkState{{Name: "init", Type: factoryapi.WorkStateTypeINITIAL}, {Name: "done", Type: factoryapi.WorkStateTypeTERMINAL}}}},
	}
	current.Version = &factoryapi.HybridLogicalTimestamp{Logical: 42, Physical: versionTime}
	srv := newFactoryDefinitionTestServer(factoryReadFake(current, nil), nil)

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/factory", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	response := decodeJSONResponse[factoryapi.Factory](t, rec)
	if response.Name != factoryapi.FactoryName("beta") || response.Version == nil || response.Version.Logical != 42 || !response.Version.Physical.Equal(versionTime) {
		t.Fatalf("current factory response = %#v, want beta @ logical 42", response)
	}
}

func TestSaveCurrentFactory_SubmitsCompleteDefinitionAndReturnsVersion(t *testing.T) {
	versionTime := time.Date(2026, 5, 18, 10, 45, 0, 0, time.UTC)
	var savedCurrentFactories []factoryapi.Factory
	srv := newFactoryDefinitionTestServer(strictFactorySaveAPIFake{save: func(_ context.Context, _ string, _ factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
		savedCurrentFactories = append(savedCurrentFactories, request)
		request.Version = &factoryapi.HybridLogicalTimestamp{Logical: 44, Physical: versionTime}
		return request, nil
	}}, nil)

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta","metadata":{"owner":"graph-editor"},"workTypes":[{"name":"beta-task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}]}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(savedCurrentFactories) != 1 {
		t.Fatalf("saved current factories = %d, want 1", len(savedCurrentFactories))
	}
	if saved := savedCurrentFactories[0]; saved.Metadata == nil || (*saved.Metadata)["owner"] != "graph-editor" {
		t.Fatalf("saved metadata = %#v, want owner graph-editor", saved.Metadata)
	}

	saved := decodeJSONResponse[factoryapi.Factory](t, rec)
	if saved.Version == nil || saved.Version.Logical != 44 || !saved.Version.Physical.Equal(versionTime) {
		t.Fatalf("save current factory version = %#v, want logical 44 physical %s", saved.Version, versionTime)
	}
}

func TestSaveCurrentFactory_MapsValidationErrorsToTargets(t *testing.T) {
	srv := newFactoryDefinitionTestServer(strictFactorySaveAPIFake{save: func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error) {
		return factoryapi.Factory{}, apisurface.ErrInvalidNamedFactory
	}}, nil)

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest || response.Code != factoryapi.ErrorResponseCode("INVALID_FACTORY") || response.Message != "Factory payload is not a valid Agent Factory definition." {
		t.Fatalf("validation response = %#v status=%d", response, rec.Code)
	}
	if response.Targets == nil || len(*response.Targets) != 1 || (*response.Targets)[0].Code != workerconfig.ValidationCodeFactoryPayloadInvalid {
		t.Fatalf("error targets = %#v, want canonical invalid factory payload target", response.Targets)
	}
}

func TestSaveCurrentFactory_MapsTopologyValidationTargets(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     testCodeDanglingPlaceReference,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  "workstation process routes to unknown place.",
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "process",
			Location: factoryapi.FactoryValidationSubjectLocationOutputs,
		},
	}
	srv := newFactoryDefinitionTestServer(strictFactorySaveAPIFake{save: func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error) {
		return factoryapi.Factory{}, apisurface.NewTopologyValidationError("dangling output", []factoryapi.FactoryValidationTarget{target})
	}}, nil)

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest || response.Code != factoryapi.ErrorResponseCode("INVALID_FACTORY") || response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("topology response = %#v status=%d", response, rec.Code)
	}
	gotTarget := (*response.Targets)[0]
	if gotTarget.Code != testCodeDanglingPlaceReference ||
		gotTarget.Subject.Type != factoryapi.FactoryValidationSubjectTypeWorkstation ||
		gotTarget.Subject.Id != "process" ||
		gotTarget.Subject.Location != factoryapi.FactoryValidationSubjectLocationOutputs {
		t.Fatalf("error target = %#v, want dangling output workstation target", gotTarget)
	}
}

func TestSaveCurrentFactory_MapsStaleVersion(t *testing.T) {
	srv := newFactoryDefinitionTestServer(strictFactorySaveAPIFake{save: func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error) {
		return factoryapi.Factory{}, apisurface.ErrFactoryVersionStale
	}}, nil)

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusConflict || response.Code != factoryapi.ErrorResponseCode("STALE_FACTORY_VERSION") || response.Targets == nil || len(*response.Targets) != 1 || (*response.Targets)[0].Code != workerconfig.ValidationCodeFactoryVersionStale {
		t.Fatalf("stale-version response = %#v status=%d", response, rec.Code)
	}
}

func TestGetCurrentFactoryWorkstationPromptTemplateContract(t *testing.T) {
	srv := newFactoryDefinitionTestServer(factoryReadFake(factoryapi.Factory{
		Name: "beta",
		SupportingFiles: &factoryapi.ResourceManifest{
			BundledFiles: &[]factoryapi.BundledFile{{
				TargetPath: "factory/docs/overview.md",
				Type:       factoryapi.BundledFileTypeDOC,
			}},
		},
		Workstations: &[]factoryapi.Workstation{{Name: "Review", Worker: "reviewer", Inputs: []factoryapi.WorkstationIO{{State: "queued", WorkType: "task"}}, Outputs: &[]factoryapi.WorkstationIO{{State: "reviewed", WorkType: "task"}}}},
	}, nil), nil)

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/factory/workstations/Review/prompt-template-contract", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	contract := decodeJSONResponse[factoryapi.PromptTemplateContract](t, rec)
	if contract.InputCount != 1 || len(contract.AvailableVariables) == 0 || contract.AvailableVariables[0].Path == "" {
		t.Fatalf("prompt contract = %#v, want populated variable list", contract)
	}
	if !promptTemplateContractHasPath(contract.AvailableVariables, ".Context.SessionID") {
		t.Fatalf("prompt contract = %#v, want .Context.SessionID", contract.AvailableVariables)
	}
	if !promptTemplateContractHasPath(contract.AvailableVariables, `.Docs["factory/docs/overview.md"]`) {
		t.Fatalf("prompt contract = %#v, want bundled doc reference", contract.AvailableVariables)
	}
}

func TestValidateCurrentFactoryWorkstationPromptTemplate_AcceptsBundledDocReference(t *testing.T) {
	srv := newFactoryDefinitionTestServer(factoryReadFake(factoryapi.Factory{
		Name: "beta",
		SupportingFiles: &factoryapi.ResourceManifest{
			BundledFiles: &[]factoryapi.BundledFile{{
				TargetPath: "factory/docs/overview.md",
				Type:       factoryapi.BundledFileTypeDOC,
			}},
		},
		Workstations: &[]factoryapi.Workstation{{Name: "Review", Worker: "reviewer", Inputs: []factoryapi.WorkstationIO{{State: "queued", WorkType: "task"}}, Outputs: &[]factoryapi.WorkstationIO{{State: "reviewed", WorkType: "task"}}}},
	}, nil), nil)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/factory/workstations/Review/prompt-template-validation", bytes.NewBufferString(`{"prompt":"{{ index .Docs \"factory/docs/overview.md\" }}"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.PromptTemplateValidationResult](t, rec)
	if !result.Valid || len(result.Diagnostics) != 0 {
		t.Fatalf("validation result = %#v, want valid with no diagnostics", result)
	}
}

func TestValidateCurrentFactoryWorkstationPromptTemplate_AcceptsContextSessionID(t *testing.T) {
	srv := newFactoryDefinitionTestServer(factoryReadFake(factoryapi.Factory{
		Name:         "beta",
		Workstations: &[]factoryapi.Workstation{{Name: "Review", Worker: "reviewer", Inputs: []factoryapi.WorkstationIO{{State: "queued", WorkType: "task"}}, Outputs: &[]factoryapi.WorkstationIO{{State: "reviewed", WorkType: "task"}}}},
	}, nil), nil)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/factory/workstations/Review/prompt-template-validation", bytes.NewBufferString(`{"prompt":"you submit --session {{ .Context.SessionID }} --work follow-up"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.PromptTemplateValidationResult](t, rec)
	if !result.Valid || len(result.Diagnostics) != 0 {
		t.Fatalf("validation result = %#v, want valid with no diagnostics", result)
	}
}

func promptTemplateContractHasPath(references []factoryapi.PromptTemplateVariableReference, want string) bool {
	for _, reference := range references {
		if reference.Path == want {
			return true
		}
	}
	return false
}

func TestValidateCurrentFactoryWorkstationPromptTemplate(t *testing.T) {
	srv := newFactoryDefinitionTestServer(factoryReadFake(factoryapi.Factory{
		Name:         "beta",
		Workstations: &[]factoryapi.Workstation{{Name: "Review", Worker: "reviewer", Inputs: []factoryapi.WorkstationIO{{State: "queued", WorkType: "task"}}, Outputs: &[]factoryapi.WorkstationIO{{State: "reviewed", WorkType: "task"}}}},
	}, nil), nil)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/factory/workstations/Review/prompt-template-validation", bytes.NewBufferString(`{"prompt":"{{ (index .Inputs 1).Payload }}"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.PromptTemplateValidationResult](t, rec)
	if result.Valid || len(result.Diagnostics) != 1 || result.Diagnostics[0].Kind != factoryapi.UNAVAILABLEVARIABLE {
		t.Fatalf("validation result = %#v, want unavailable variable diagnostic", result)
	}
}

func TestValidateCurrentFactoryWorkstationPromptTemplate_UnknownWorkstation(t *testing.T) {
	srv := newFactoryDefinitionTestServer(factoryReadFake(factoryapi.Factory{Name: "beta"}, nil), nil)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/factory/workstations/Missing/prompt-template-validation", bytes.NewBufferString(`{"prompt":"{{ .Context.Project }}"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "Current factory workstation not found.")
}

func TestGetCurrentFactory_ReturnsNotFoundWithoutStoredNamedFactory(t *testing.T) {
	srv := newFactoryDefinitionTestServer(factoryReadFake(factoryapi.Factory{}, apisurface.ErrCurrentFactoryNotFound), nil)
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/factory", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "Current factory not found.")
}

func TestLegacyCreateFactoryRoute_RemovedFromRouter(t *testing.T) {
	srv := newFactoryDefinitionTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/factory", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /factory status = %d, want route removed", rec.Code)
	}
}

func TestValidateFactory_ReturnsEmptyTargetsForValidFactory(t *testing.T) {
	srv := newTestServerWithValidationTargets()

	req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	if len(result.Targets) != 0 {
		t.Fatalf("targets = %#v, want empty slice", result.Targets)
	}
}

func TestValidateFactory_ReturnsMultipleTargetsForInvalidFactory(t *testing.T) {
	srv := newTestServerWithValidationTargets(
		httpValidationTarget(testCodeDuplicateIdentifier, "factory", workerconfig.ValidationSubjectTypeFactory, "factory", workerconfig.ValidationSubjectLocationDefinition),
		httpValidationTarget(testCodeDanglingWorkerReference, "worker", workerconfig.ValidationSubjectTypeWorkstation, "process", workerconfig.ValidationSubjectLocationDefinition),
		httpValidationTarget(testCodeDanglingPlaceReference, "route", workerconfig.ValidationSubjectTypeRoute, "process->missing", workerconfig.ValidationSubjectLocationOutputs),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(factoryfixtures.CrossPathInvalidFactoryJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	if len(result.Targets) < 2 {
		t.Fatalf("targets = %d, want multiple validation targets", len(result.Targets))
	}
	assertHasValidationTargetCode(t, result.Targets, testCodeDuplicateIdentifier)
	assertHasValidationTargetCode(t, result.Targets, testCodeDanglingWorkerReference)
	assertHasValidationTargetCode(t, result.Targets, testCodeDanglingPlaceReference)
}

func TestValidateFactory_ReturnsCanonicalWorkstationSubjects(t *testing.T) {
	srv := newTestServerWithValidationTargets(
		httpValidationTarget(testCodeWorkstationMissingRejectionRoute, "rejection", workerconfig.ValidationSubjectTypeWorkstation, "process", workerconfig.ValidationSubjectLocationOnRejection),
		httpValidationTarget(testCodeWorkTypeMissingCompletionState, "completion", workerconfig.ValidationSubjectTypeWorkType, "task", workerconfig.ValidationSubjectLocationStates),
	)

	body := `{
		"name":"alpha",
		"workTypes":[{"name":"task","states":[{"name":"queued","type":"INITIAL"}]}],
		"workers":[{"name":"worker-a"}],
		"workstations":[{
			"name":"process",
			"behavior":"REPEATER",
			"worker":"worker-a",
			"outputs":[{"workType":"task","state":"queued"}]
		}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	assertHasValidationTarget(
		t,
		result.Targets,
		testCodeWorkstationMissingRejectionRoute,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"process",
		factoryapi.FactoryValidationSubjectLocationOnRejection,
		"process ON_REJECTION target",
	)
	assertHasValidationTarget(
		t,
		result.Targets,
		testCodeWorkTypeMissingCompletionState,
		factoryapi.FactoryValidationSubjectTypeWorkType,
		"task",
		factoryapi.FactoryValidationSubjectLocationStates,
		"task STATES target",
	)
}

func TestValidateFactory_RejectsMalformedPayload(t *testing.T) {
	srv := newFactoryDefinitionTestServer(nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(`{"name":"alpha"`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSaveCurrentFactory_ReturnsBobWorkstationOnFailureTarget(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     testCodeWorkstationMissingFailureRoute,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  `workstation "bob" must define a failure route.`,
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "bob",
			Location: factoryapi.FactoryValidationSubjectLocationOnFailure,
		},
	}
	srv := newFactoryDefinitionTestServer(strictFactorySaveAPIFake{save: func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error) {
		return factoryapi.Factory{}, apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			[]factoryapi.FactoryValidationTarget{target},
		)
	}}, nil)

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest || response.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("response = %#v status=%d", response, rec.Code)
	}
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical target", response.Targets)
	}
	got := (*response.Targets)[0]
	assertHasValidationTarget(
		t,
		[]factoryapi.FactoryValidationTarget{got},
		testCodeWorkstationMissingFailureRoute,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"bob",
		factoryapi.FactoryValidationSubjectLocationOnFailure,
		"bob ON_FAILURE target",
	)
}

func assertHasValidationTarget(
	t *testing.T,
	targets []factoryapi.FactoryValidationTarget,
	code string,
	subjectType factoryapi.FactoryValidationSubjectType,
	subjectID string,
	location factoryapi.FactoryValidationSubjectLocation,
	want string,
) {
	t.Helper()
	for _, target := range targets {
		if target.Code != code {
			continue
		}
		if target.Subject.Type != subjectType || target.Subject.Id != subjectID || target.Subject.Location != location {
			continue
		}
		return
	}
	t.Fatalf("validation targets = %#v, want %s", targets, want)
}

func assertHasValidationTargetCode(t *testing.T, targets []factoryapi.FactoryValidationTarget, code string) {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return
		}
	}
	t.Fatalf("targets = %#v, want code %q", targets, code)
}

func TestUpsertWorkRequestBySessionId_Returns201AndSubmitsToSessionFactory(t *testing.T) {
	var defaultRequests, sessionRequests []work.WorkRequest
	srv := newWorkTransportTestServer(strictWorkAPIFake{submit: func(_ context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
		switch sessionID {
		case factorysessions.DefaultSessionID:
			defaultRequests = append(defaultRequests, request)
		case "session-alpha":
			sessionRequests = append(sessionRequests, request)
		default:
			return work.WorkRequestSubmitResult{}, apisurface.ErrFactorySessionNotFound
		}
		return acceptProgrammedHTTPWorkRequest(request), nil
	}})

	rec := upsertWorkRequest(t, srv, "/factory-sessions/session-alpha/work-requests/request-scoped-upsert", `{
		"requestId":"request-scoped-upsert",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"scoped-batch","workTypeName":"task","traceId":"trace-scoped-upsert","payload":{"title":"Scoped upsert"}}]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /factory-sessions/session-alpha/work-requests/request-scoped-upsert status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.UpsertWorkRequestResponse](t, rec)
	if resp.RequestId != "request-scoped-upsert" || resp.TraceId == "" {
		t.Fatalf("upsert response = %#v, want request and trace", resp)
	}
	if len(resp.Works) != 1 || resp.Works[0].Name != "scoped-batch" || resp.Works[0].WorkTypeName != "task" {
		t.Fatalf("upsert works = %#v, want scoped-batch task", resp.Works)
	}
	if len(sessionRequests) != 1 || len(sessionRequests[0].Works) != 1 {
		t.Fatalf("session Work requests = %#v, want one request with one Work", sessionRequests)
	}
	if sessionRequests[0].RequestID != "request-scoped-upsert" || sessionRequests[0].Works[0].Name != "scoped-batch" {
		t.Fatalf("session Work request = %#v, want scoped upsert metadata", sessionRequests[0])
	}
	if len(defaultRequests) != 0 {
		t.Fatalf("default Work requests = %d, want 0", len(defaultRequests))
	}
}

func TestUpsertWorkRequestBySessionId_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newWorkTransportTestServer(strictWorkAPIFake{submit: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
		return work.WorkRequestSubmitResult{}, apisurface.ErrFactorySessionNotFound
	}})

	rec := upsertWorkRequest(t, srv, "/factory-sessions/missing-session/work-requests/request-missing-session", `{
		"requestId":"request-missing-session",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"draft","workTypeName":"task","payload":{"title":"Draft"}}]
	}`)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestUnscopedWorkRoutes_RemovedFromRouter(t *testing.T) {
	srv := newWorkTransportTestServer(nil)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "GET /work", method: http.MethodGet, path: "/work"},
		{name: "POST /work", method: http.MethodPost, path: "/work", body: `{"name":"retired","workTypeName":"task","payload":{"title":"x"}}`},
		{name: "POST /work/staged-files", method: http.MethodPost, path: "/work/staged-files", body: `{"itemType":"document","fileName":"x.txt","mediaType":"text/plain","contentBase64":"eA=="}`},
		{name: "PUT /work-requests/{request_id}", method: http.MethodPut, path: "/work-requests/req-retired", body: `{"works":[]}`},
		{name: "GET /work/{id}", method: http.MethodGet, path: "/work/work-retired"},
		{name: "POST /work/{id}/move", method: http.MethodPost, path: "/work/work-retired/move", body: `{"stateName":"complete"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = bytes.NewBufferString(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s status = %d, want route removed: %s", tc.method, tc.path, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSessionScopedWorkRoutes_AcceptValidRequests(t *testing.T) {
	srv := newWorkTransportTestServer(strictWorkAPIFake{
		submit: func(_ context.Context, _ string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return acceptProgrammedHTTPWorkRequest(request), nil
		},
		list: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
			return work.ListResult{MaxResults: work.DefaultListMaxResults}, nil
		},
	})

	submitRec := submitWorkRequest(t, srv, `{"name":"scoped-submit","workTypeName":"task","traceId":"trace-scoped","payload":{"title":"scoped"}}`)
	if submitRec.Code != http.StatusCreated {
		t.Fatalf("POST /factory-sessions/~default/work status = %d, want 201: %s", submitRec.Code, submitRec.Body.String())
	}

	listRec := httptest.NewRequest(http.MethodGet, defaultSessionWorkAPIPrefix+"/work", nil)
	listResp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listResp, listRec)
	if listResp.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/~default/work status = %d, want 200: %s", listResp.Code, listResp.Body.String())
	}

	upsertRec := upsertWorkRequest(t, srv, "/work-requests/batch-req-1", `{"requestId":"batch-req-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"batch-work","workTypeName":"task","payload":{"title":"batch"}}]}`)
	if upsertRec.Code != http.StatusOK && upsertRec.Code != http.StatusCreated {
		t.Fatalf("PUT /factory-sessions/~default/work-requests/{id} status = %d, want success: %s", upsertRec.Code, upsertRec.Body.String())
	}
}

func TestSessionScopedWorkRoutes_UnknownSessionReturnsNotFound(t *testing.T) {
	missing := func() error { return apisurface.ErrFactorySessionNotFound }
	srv := newWorkTransportTestServerWithRoles(
		strictLiveSessionAPIFake{get: func(context.Context, string) (factoryapi.FactorySession, error) {
			return factoryapi.FactorySession{}, missing()
		}},
		strictWorkAPIFake{
			submit: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
				return work.WorkRequestSubmitResult{}, missing()
			},
			move: func(context.Context, string, string, string, string) (work.OperatorMoveResult, error) {
				return work.OperatorMoveResult{}, missing()
			},
			snapshot: func(context.Context, string) (*workerconfig.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net], error) {
				return nil, missing()
			},
			list: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
				return work.ListResult{}, missing()
			},
			getWork: func(context.Context, string, string) (work.ReadModel, error) {
				return work.ReadModel{}, missing()
			},
			moveRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
				return work.ReadModel{}, missing()
			},
		},
		factoryReadFake(factoryapi.Factory{}, missing()),
	)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "GET work list", method: http.MethodGet, path: "/factory-sessions/missing-session/work"},
		{name: "POST submit", method: http.MethodPost, path: "/factory-sessions/missing-session/work", body: `{"name":"x","workTypeName":"task","payload":{"title":"x"}}`},
		{name: "POST staged-files", method: http.MethodPost, path: "/factory-sessions/missing-session/work/staged-files", body: `{"itemType":"document","fileName":"x.txt","mediaType":"text/plain","contentBase64":"eA=="}`},
		{name: "PUT work request", method: http.MethodPut, path: "/factory-sessions/missing-session/work-requests/req-1", body: `{"requestId":"req-1","type":"FACTORY_REQUEST_BATCH","works":[]}`},
		{name: "GET work by id", method: http.MethodGet, path: "/factory-sessions/missing-session/work/work-1"},
		{name: "POST move", method: http.MethodPost, path: "/factory-sessions/missing-session/work/work-1/move", body: `{"stateName":"complete"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = bytes.NewBufferString(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
		})
	}
}

type httpFactoryValidator struct {
	result workerconfig.ValidationResult
}

func (validator httpFactoryValidator) ValidateSubmittedDefinition(
	ctx context.Context,
	request workerconfig.SubmittedDefinitionValidationRequest,
) (workerconfig.ValidationResult, error) {
	result := validator.Validate(ctx, request.Config, request.WorkflowSourceReader)
	return result, nil
}

func (validator httpFactoryValidator) Validate(
	context.Context,
	*workerconfig.FactoryConfig,
	workerconfig.WorkflowSourceReader,
) workerconfig.ValidationResult {
	return validator.result
}

func (validator httpFactoryValidator) ValidateBlockingLoad(
	context.Context,
	*workerconfig.FactoryConfig,
) workerconfig.ValidationResult {
	return validator.result
}

func (httpFactoryValidator) ValidateTopology(
	context.Context,
	*workerconfig.FactoryConfig,
	workerconfig.RequiredToolChecker,
) workerconfig.TopologyValidationResult {
	return workerconfig.TopologyValidationResult{}
}

func (httpFactoryValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*workerconfig.FactoryConfig,
) []workerconfig.ValidationTarget {
	return nil
}

func (httpFactoryValidator) WorkTypeHandlingBehavior(
	context.Context,
	*workerconfig.FactoryConfig,
	bool,
) []workerconfig.ValidationTarget {
	return nil
}

func (validator httpFactoryValidator) PruneLayout(
	context.Context,
	*workerconfig.FactoryConfig,
	workerconfig.PendingFactoryGraphTopology,
) workerconfig.ValidationResult {
	return validator.result
}

func newTestServerWithValidationTargets(
	targets ...workerconfig.ValidationTarget,
) *Server {
	return newFactoryDefinitionTestServer(nil, httpFactoryValidator{
		result: workerconfig.ValidationResult{Targets: append([]workerconfig.ValidationTarget(nil), targets...)},
	})
}

func httpValidationTarget(
	code string,
	message string,
	subjectType workerconfig.ValidationSubjectType,
	subjectID string,
	location workerconfig.ValidationSubjectLocation,
) workerconfig.ValidationTarget {
	return workerconfig.ValidationTarget{
		Code:     code,
		Severity: workerconfig.ValidationSeverityError,
		Message:  message,
		Subject: workerconfig.ValidationSubject{
			Type: subjectType, ID: subjectID, Location: location,
		},
	}
}
