package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestCreateFactory_ReturnsCreatedFactoryShape(t *testing.T) {
	mf := &testutil.MockFactory{}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.CreatedFactories) != 1 {
		t.Fatalf("created factories = %d, want 1", len(mf.CreatedFactories))
	}
	created := decodeJSONResponse[factoryapi.Factory](t, rec)
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}
	if created.WorkTypes == nil || len(*created.WorkTypes) != 1 || (*created.WorkTypes)[0].Name != "beta-task" {
		t.Fatalf("created factory work types = %#v, want beta-task", created.WorkTypes)
	}
}

func TestGetCurrentFactory_ReturnsFactoryShape(t *testing.T) {
	mf := &testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{
			Name:      factoryapi.FactoryName("beta"),
			WorkTypes: &[]factoryapi.WorkType{{Name: "beta-task", States: []factoryapi.WorkState{{Name: "init", Type: factoryapi.WorkStateTypeINITIAL}, {Name: "done", Type: factoryapi.WorkStateTypeTERMINAL}}}},
		},
	}
	srv := newTestServer(mf)

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
	mf := &testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: apisurface.DefaultCurrentFactoryName, Id: stringPointerForAPITest("root-runtime")},
	}
	srv := newTestServer(mf)

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
	mf := &testutil.MockFactory{
		Models: factoryapi.ListModelsResponse{
			Results: []factoryapi.ModelSummary{{
				Name:             "OMNIVOICE_Q4_K_M",
				ProviderLocality: factoryapi.WorkerModelLocalityLocal,
				Status:           factoryapi.ModelStatusREADY,
				LoadState:        factoryapi.UNLOADED,
				Operations:       []factoryapi.ModelOperation{{Name: "TTS"}},
				Modalities:       []factoryapi.ModelOperationContentType{factoryapi.ModelOperationContentTypeAudio, factoryapi.ModelOperationContentTypeText},
				Resources:        []factoryapi.ModelResourceSummary{{Name: "omnivoice-cache", Type: factoryapi.ResourceTypeModel, Capacity: 1}},
			}},
		},
	}
	srv := newTestServer(mf)

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
}

func TestGetModel_ReturnsDiscoveredModelDetail(t *testing.T) {
	mf := &testutil.MockFactory{
		ModelDetails: map[string]factoryapi.ModelDetail{
			"OMNIVOICE_Q4_K_M": {
				Name:             "OMNIVOICE_Q4_K_M",
				ProviderLocality: factoryapi.WorkerModelLocalityLocal,
				Status:           factoryapi.ModelStatusREADY,
				LoadState:        factoryapi.UNLOADED,
				Operations:       []factoryapi.ModelOperation{{Name: "TTS"}},
				Modalities:       []factoryapi.ModelOperationContentType{factoryapi.ModelOperationContentTypeAudio, factoryapi.ModelOperationContentTypeText},
				Resources:        []factoryapi.ModelResourceSummary{{Name: "omnivoice-cache", Type: factoryapi.ResourceTypeModel, Capacity: 1}},
				Capabilities: []factoryapi.ModelCapability{{
					Worker:           "voice-local",
					ProviderLocality: factoryapi.WorkerModelLocalityLocal,
					Operations:       []factoryapi.ModelOperation{{Name: "TTS"}},
					ResourceNames:    []string{"omnivoice-cache"},
				}},
				Diagnostics: factoryapi.StringMap{"workerCount": "1"},
			},
		},
	}
	srv := newTestServer(mf)

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
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodGet, "/models/MISSING", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "model not found")
}

func TestInvokeModel_ReturnsInvocationMetadata(t *testing.T) {
	mf := &testutil.MockFactory{
		InvokeModelResult: apisurface.ModelInvocationResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			Worker:           "tts-worker",
			Operation:        "TTS",
			ProviderLocality: interfaces.ModelLocalityLocal,
			Content: []interfaces.WorkContentPart{{
				Type:        interfaces.WorkContentPartTypeAudio,
				File:        "artifacts/output.wav",
				ContentType: "audio/wav",
			}},
			Bindings: []interfaces.ResolvedModelOperationBinding{{
				Slot:   "text",
				Source: interfaces.ModelOperationBindingSourceInput,
				Content: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeText,
					Text: "hello world",
				}},
			}},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/invocations", bytes.NewBufferString(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello world"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.InvokedModelNames) != 1 || mf.InvokedModelNames[0] != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("invoked model names = %#v, want OMNIVOICE_Q4_K_M", mf.InvokedModelNames)
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
	mf := &testutil.MockFactory{
		InvokeModelResult: apisurface.ModelInvocationResult{
			ModelName:         "OMNIVOICE_Q4_K_M",
			Worker:            "tts-worker",
			Operation:         "TTS",
			ProviderLocality:  interfaces.ModelLocalityLocal,
			StreamFile:        audioPath,
			StreamContentType: "audio/wav",
		},
	}
	srv := newTestServer(mf)

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
	mf := &testutil.MockFactory{
		InvokeModelErr: fmt.Errorf("%w: required assets missing", apisurface.ErrModelNotAvailable),
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/invocations", bytes.NewBufferString(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello world"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "MODEL_NOT_AVAILABLE", "model not available: required assets missing")
}

func TestPullModel_ReturnsManagedCachePullMetadata(t *testing.T) {
	mf := &testutil.MockFactory{
		PullModelResult: apisurface.ModelPullResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: interfaces.ModelLocalityLocal,
			Outcome:          "PULLED",
			CachePath:        "/tmp/models/OMNIVOICE_Q4_K_M/rev1",
			Revision:         "rev1",
			DownloadedFiles: []apisurface.ModelPullDownloadedFile{{
				Path:   "omnivoice-base-Q4_K_M.gguf",
				Bytes:  407,
				SHA256: "abc123",
			}},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/pull", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.PulledModelNames) != 1 || mf.PulledModelNames[0] != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("pulled model names = %#v, want OMNIVOICE_Q4_K_M", mf.PulledModelNames)
	}
	response := decodeJSONResponse[factoryapi.ModelPullResponse](t, rec)
	if response.Outcome != factoryapi.ModelPullOutcome("PULLED") || response.CachePath == "" || len(response.DownloadedFiles) != 1 {
		t.Fatalf("pull response = %#v, want pull metadata", response)
	}
}

func TestGetCurrentFactory_ReturnsDefinitionAndVersion(t *testing.T) {
	versionTime := time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC)
	mf := &testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{
			Name:      factoryapi.FactoryName("beta"),
			WorkTypes: &[]factoryapi.WorkType{{Name: "beta-task", States: []factoryapi.WorkState{{Name: "init", Type: factoryapi.WorkStateTypeINITIAL}, {Name: "done", Type: factoryapi.WorkStateTypeTERMINAL}}}},
		},
		FactoryVersion: factoryapi.HybridLogicalTimestamp{Logical: 42, Physical: versionTime},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/factory", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	current := decodeJSONResponse[factoryapi.Factory](t, rec)
	if current.Name != factoryapi.FactoryName("beta") || current.Version == nil || current.Version.Logical != 42 || !current.Version.Physical.Equal(versionTime) {
		t.Fatalf("current factory response = %#v, want beta @ logical 42", current)
	}
}

func TestSaveCurrentFactory_SubmitsCompleteDefinitionAndReturnsVersion(t *testing.T) {
	versionTime := time.Date(2026, 5, 18, 10, 45, 0, 0, time.UTC)
	mf := &testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: factoryapi.FactoryName("beta")},
		FactoryVersion: factoryapi.HybridLogicalTimestamp{Logical: 44, Physical: versionTime},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(`{"name":"beta","metadata":{"owner":"graph-editor"},"workTypes":[{"name":"beta-task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.SavedCurrentFactories) != 1 {
		t.Fatalf("saved current factories = %d, want 1", len(mf.SavedCurrentFactories))
	}
	if saved := mf.SavedCurrentFactories[0]; saved.Metadata == nil || (*saved.Metadata)["owner"] != "graph-editor" {
		t.Fatalf("saved metadata = %#v, want owner graph-editor", saved.Metadata)
	}

	saved := decodeJSONResponse[factoryapi.Factory](t, rec)
	if saved.Version == nil || saved.Version.Logical != 44 || !saved.Version.Physical.Equal(versionTime) {
		t.Fatalf("save current factory version = %#v, want logical 44 physical %s", saved.Version, versionTime)
	}
}

func TestSaveCurrentFactory_MapsValidationErrorsToTargets(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{SaveCurrentFactoryErr: apisurface.ErrInvalidNamedFactory})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(`{"name":"beta"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest || response.Code != factoryapi.ErrorResponseCode("INVALID_FACTORY") || response.Message != "Factory payload is not a valid Agent Factory definition." {
		t.Fatalf("validation response = %#v status=%d", response, rec.Code)
	}
	if response.Targets == nil || len(*response.Targets) != 1 || (*response.Targets)[0].Code != factoryvalidation.CodeFactoryPayloadInvalid {
		t.Fatalf("error targets = %#v, want canonical invalid factory payload target", response.Targets)
	}
}

func TestSaveCurrentFactory_MapsTopologyValidationTargets(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     factoryvalidation.CodeDanglingPlaceReference,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  "workstation process routes to unknown place.",
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "process",
			Location: factoryapi.FactoryValidationSubjectLocationOutputs,
		},
	}
	srv := newTestServer(&testutil.MockFactory{SaveCurrentFactoryErr: apisurface.NewTopologyValidationError("dangling output", []factoryapi.FactoryValidationTarget{target})})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(`{"name":"beta"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest || response.Code != factoryapi.ErrorResponseCode("INVALID_FACTORY") || response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("topology response = %#v status=%d", response, rec.Code)
	}
	gotTarget := (*response.Targets)[0]
	if gotTarget.Code != factoryvalidation.CodeDanglingPlaceReference ||
		gotTarget.Subject.Type != factoryapi.FactoryValidationSubjectTypeWorkstation ||
		gotTarget.Subject.Id != "process" ||
		gotTarget.Subject.Location != factoryapi.FactoryValidationSubjectLocationOutputs {
		t.Fatalf("error target = %#v, want dangling output workstation target", gotTarget)
	}
}

func TestSaveCurrentFactory_MapsStaleVersion(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{SaveCurrentFactoryErr: apisurface.ErrFactoryVersionStale})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(`{"name":"beta"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusConflict || response.Code != factoryapi.ErrorResponseCode("STALE_FACTORY_VERSION") || response.Targets == nil || len(*response.Targets) != 1 || (*response.Targets)[0].Code != factoryvalidation.CodeFactoryVersionStale {
		t.Fatalf("stale-version response = %#v status=%d", response, rec.Code)
	}
}

func TestGetCurrentFactoryWorkstationPromptTemplateContract(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{
			Name:         "beta",
			Workstations: &[]factoryapi.Workstation{{Name: "Review", Worker: "reviewer", Inputs: []factoryapi.WorkstationIO{{State: "queued", WorkType: "task"}}, Outputs: &[]factoryapi.WorkstationIO{{State: "reviewed", WorkType: "task"}}}},
		},
	})

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
}

func TestValidateCurrentFactoryWorkstationPromptTemplate(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{
			Name:         "beta",
			Workstations: &[]factoryapi.Workstation{{Name: "Review", Worker: "reviewer", Inputs: []factoryapi.WorkstationIO{{State: "queued", WorkType: "task"}}, Outputs: &[]factoryapi.WorkstationIO{{State: "reviewed", WorkType: "task"}}}},
		},
	})

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
	srv := newTestServer(&testutil.MockFactory{CurrentFactory: &factoryapi.Factory{Name: "beta"}})

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/factory/workstations/Missing/prompt-template-validation", bytes.NewBufferString(`{"prompt":"{{ .Context.Project }}"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "Current factory workstation not found.")
}

func TestCreateFactory_RejectsDuplicateFactoryName(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{CreateNamedFactoryErr: factoryconfig.ErrNamedFactoryAlreadyExists})
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusConflict, "FACTORY_ALREADY_EXISTS", "Named factory already exists.")
}

func TestCreateFactory_RejectsInvalidFactoryName(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("nested/name", "task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "INVALID_FACTORY_NAME", "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.")
}

func TestCreateFactory_RejectsReservedCurrentFactoryName(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody(string(apisurface.DefaultCurrentFactoryName), "task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "INVALID_FACTORY_NAME", "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.")
}

func TestCreateFactory_RejectsInvalidFactoryPayload(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{CreateNamedFactoryErr: apisurface.ErrInvalidNamedFactory})
	body := `{"name":"beta","workTypes":[{"name":"beta-task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"missing-worker","inputs":[{"workType":"beta-task","state":"init"}],"outputs":[{"workType":"beta-task","state":"done"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "INVALID_FACTORY", "Factory payload is not a valid Agent Factory definition.")
}

func TestCreateFactory_RejectsNonIdleRuntime(t *testing.T) {
	mf := &testutil.MockFactory{
		EngineState:           engineStateWithRuntimeStatus(interfaces.RuntimeStatusActive),
		CreateNamedFactoryErr: apisurface.ErrFactoryActivationRequiresIdle,
		CurrentFactoryErr:     apisurface.ErrCurrentFactoryNotFound,
	}
	srv := newTestServer(mf)
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusConflict, "FACTORY_NOT_IDLE", "Current factory runtime must be idle before activation.")
}

func TestGetCurrentFactory_ReturnsNotFoundWithoutStoredNamedFactory(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{CurrentFactoryErr: apisurface.ErrCurrentFactoryNotFound})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/factory", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "Current factory not found.")
}

func TestLegacyCreateFactoryRoute_RemovedFromRouter(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodPost, "/factory", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /factory status = %d, want route removed", rec.Code)
	}
}

func TestValidateFactory_ReturnsEmptyTargetsForValidFactory(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

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
	srv := newTestServer(&testutil.MockFactory{})

	req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(factoryvalidation.CrossPathInvalidFactoryJSON))
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
	assertHasValidationTargetCode(t, result.Targets, factoryvalidation.CodeDuplicateIdentifier)
	assertHasValidationTargetCode(t, result.Targets, factoryvalidation.CodeDanglingWorkerReference)
	assertHasValidationTargetCode(t, result.Targets, factoryvalidation.CodeDanglingPlaceReference)
}

func TestValidateFactory_ReturnsCanonicalWorkstationSubjects(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

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
		factoryvalidation.CodeWorkstationMissingRejectionRoute,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"process",
		factoryapi.FactoryValidationSubjectLocationOnRejection,
		"process ON_REJECTION target",
	)
	assertHasValidationTarget(
		t,
		result.Targets,
		factoryvalidation.CodeWorkTypeMissingCompletionState,
		factoryapi.FactoryValidationSubjectTypeWorkType,
		"task",
		factoryapi.FactoryValidationSubjectLocationStates,
		"task STATES target",
	)
}

func TestValidateFactory_RejectsMalformedPayload(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(`{"name":"alpha"`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFactoryValidation_EquivalentCanonicalTargetsAcrossPackageConfigAndAPIPaths(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}
	cfg, err := factoryconfig.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}

	explicit := factoryvalidation.Validate(&cfg)
	packageSignatures := factoryvalidation.CanonicalTargetSignatures(explicit.Targets)

	configFindings := factoryconfig.CanonicalStructuralFindings(&cfg)
	if len(configFindings) != len(explicit.Targets) {
		t.Fatalf("config findings = %d, package targets = %d, want equivalent coverage",
			len(configFindings), len(explicit.Targets))
	}
	for index, target := range explicit.Targets {
		if configFindings[index].Rule != target.Code {
			t.Fatalf("config finding[%d].Rule = %q, want package target code %q",
				index, configFindings[index].Rule, target.Code)
		}
	}

	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		bytes.NewBufferString(factoryvalidation.CrossPathInvalidFactoryJSON),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-validations status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	apiSignatures := factoryvalidation.CanonicalAPITargetSignatures(result.Targets)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(packageSignatures, apiSignatures) {
		t.Fatalf("package signatures = %#v, api signatures = %#v, want equivalent canonical targets",
			packageSignatures, apiSignatures)
	}
}

func TestValidateFactory_OperationalCronRulesRemainConfigScoped(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	body := `{
		"name":"alpha",
		"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"workers":[{"name":"w1"}],
		"workstations":[{
			"name":"daily-refresh",
			"behavior":"CRON",
			"worker":"w1",
			"outputs":[{"workType":"task","state":"init"}]
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
	for _, target := range result.Targets {
		if target.Code == "cron-config" || target.Code == "cron-schedule" {
			t.Fatalf("validation-only API targets = %#v, want operational cron rules to stay config-scoped", result.Targets)
		}
	}

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "w1"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "daily-refresh",
			Kind:           interfaces.WorkstationKindCron,
			WorkerTypeName: "w1",
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		}},
	}
	configFindings := factoryconfig.NewConfigValidator().Validate(cfg).Findings
	assertConfigFindingRule(t, configFindings, "cron-config")
}

func TestValidateFactory_PreservesCanonicalStructuralCodesFromCrossPathFixture(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		bytes.NewBufferString(factoryvalidation.CrossPathInvalidFactoryJSON),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	wantCodes := []string{
		factoryvalidation.CodeDuplicateIdentifier,
		factoryvalidation.CodeDanglingWorkerReference,
		factoryvalidation.CodeDanglingPlaceReference,
		factoryvalidation.CodeWorkstationMissingFailureRoute,
		factoryvalidation.CodeWorkTypeMissingCompletionState,
		factoryvalidation.CodeWorkTypeMissingFailureState,
		factoryvalidation.CodeWorkStateMissingTerminalPath,
	}
	for _, code := range wantCodes {
		assertHasValidationTargetCode(t, result.Targets, code)
	}
}

func TestSaveCurrentFactory_ReturnsMultipleTopologyValidationTargets(t *testing.T) {
	targets := []factoryapi.FactoryValidationTarget{
		{
			Code:     factoryvalidation.CodeDuplicateIdentifier,
			Severity: factoryapi.FactoryValidationSeverityError,
			Message:  `duplicate worker identifier "worker-a".`,
			Subject: factoryapi.FactoryValidationSubject{
				Type:     factoryapi.FactoryValidationSubjectTypeWorker,
				Id:       "worker-a",
				Location: factoryapi.FactoryValidationSubjectLocationDefinition,
			},
		},
		{
			Code:     factoryvalidation.CodeDanglingWorkerReference,
			Severity: factoryapi.FactoryValidationSeverityError,
			Message:  `workstation "process" references unknown worker "missing-worker".`,
			Subject: factoryapi.FactoryValidationSubject{
				Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
				Id:       "process",
				Location: factoryapi.FactoryValidationSubjectLocationDefinition,
			},
		},
		{
			Code:     factoryvalidation.CodeDanglingPlaceReference,
			Severity: factoryapi.FactoryValidationSeverityError,
			Message:  `workstation "process" routes to unknown place.`,
			Subject: factoryapi.FactoryValidationSubject{
				Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
				Id:       "process",
				Location: factoryapi.FactoryValidationSubjectLocationOutputs,
			},
		},
	}
	srv := newTestServer(&testutil.MockFactory{
		SaveCurrentFactoryErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			targets,
		),
	})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(`{"name":"beta"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.INVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) < 3 {
		t.Fatalf("targets = %#v, want multiple blocking validation targets", response.Targets)
	}
	assertHasValidationTargetCode(t, *response.Targets, factoryvalidation.CodeDuplicateIdentifier)
	assertHasValidationTargetCode(t, *response.Targets, factoryvalidation.CodeDanglingWorkerReference)
	assertHasValidationTargetCode(t, *response.Targets, factoryvalidation.CodeDanglingPlaceReference)
	assertBlockingValidationTarget(t, (*response.Targets)[0])
}

func TestCreateFactory_ReturnsTopologyValidationTargets(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     factoryvalidation.CodeDanglingPlaceReference,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  "workstation process routes to unknown place.",
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "process",
			Location: factoryapi.FactoryValidationSubjectLocationOutputs,
		},
	}
	srv := newTestServer(&testutil.MockFactory{
		CreateNamedFactoryErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			[]factoryapi.FactoryValidationTarget{target},
		),
	})

	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.INVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical target", response.Targets)
	}
	gotTarget := (*response.Targets)[0]
	assertBlockingValidationTarget(t, gotTarget)
	if gotTarget.Code != factoryvalidation.CodeDanglingPlaceReference ||
		gotTarget.Subject.Type != factoryapi.FactoryValidationSubjectTypeWorkstation ||
		gotTarget.Subject.Id != "process" ||
		gotTarget.Subject.Location != factoryapi.FactoryValidationSubjectLocationOutputs {
		t.Fatalf("error target = %#v, want dangling output workstation target", gotTarget)
	}
}

func TestCreateFactory_RejectsInvalidFactoryPayloadWithTargets(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{CreateNamedFactoryErr: apisurface.ErrInvalidNamedFactory})
	body := `{"name":"beta","workTypes":[{"name":"beta-task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"missing-worker","inputs":[{"workType":"beta-task","state":"init"}],"outputs":[{"workType":"beta-task","state":"done"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.INVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) != 1 || (*response.Targets)[0].Code != factoryvalidation.CodeFactoryPayloadInvalid {
		t.Fatalf("error targets = %#v, want canonical invalid factory payload target", response.Targets)
	}
	assertBlockingValidationTarget(t, (*response.Targets)[0])
}

func TestSaveCurrentFactory_ReturnsBobWorkstationOnFailureTarget(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     factoryvalidation.CodeWorkstationMissingFailureRoute,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  `workstation "bob" must define a failure route.`,
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "bob",
			Location: factoryapi.FactoryValidationSubjectLocationOnFailure,
		},
	}
	srv := newTestServer(&testutil.MockFactory{
		SaveCurrentFactoryErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			[]factoryapi.FactoryValidationTarget{target},
		),
	})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(`{"name":"beta"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("response = %#v status=%d", response, rec.Code)
	}
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.INVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical target", response.Targets)
	}
	got := (*response.Targets)[0]
	assertBlockingValidationTarget(t, got)
	assertHasValidationTarget(
		t,
		[]factoryapi.FactoryValidationTarget{got},
		factoryvalidation.CodeWorkstationMissingFailureRoute,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"bob",
		factoryapi.FactoryValidationSubjectLocationOnFailure,
		"bob ON_FAILURE target",
	)
}

func TestCreateFactory_ReturnsBobWorkstationOnFailureTarget(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     factoryvalidation.CodeWorkstationMissingFailureRoute,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  `workstation "bob" must define a failure route.`,
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "bob",
			Location: factoryapi.FactoryValidationSubjectLocationOnFailure,
		},
	}
	srv := newTestServer(&testutil.MockFactory{
		CreateNamedFactoryErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			[]factoryapi.FactoryValidationTarget{target},
		),
	})

	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("response = %#v status=%d", response, rec.Code)
	}
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.INVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical target", response.Targets)
	}
	got := (*response.Targets)[0]
	assertBlockingValidationTarget(t, got)
	assertHasValidationTarget(
		t,
		[]factoryapi.FactoryValidationTarget{got},
		factoryvalidation.CodeWorkstationMissingFailureRoute,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"bob",
		factoryapi.FactoryValidationSubjectLocationOnFailure,
		"bob ON_FAILURE target",
	)
}

func assertConfigFindingRule(t *testing.T, findings []factoryconfig.Finding, rule string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule == rule {
			return
		}
	}
	t.Fatalf("expected config finding rule %q, got %#v", rule, findings)
}

func assertErrorResponsePreservesLegacyFields(
	t *testing.T,
	response factoryapi.ErrorResponse,
	wantCode factoryapi.ErrorResponseCode,
	wantFamily factoryapi.ErrorFamily,
	wantMessage string,
) {
	t.Helper()
	if response.Code != wantCode {
		t.Fatalf("error code = %q, want %q", response.Code, wantCode)
	}
	if response.Family != wantFamily {
		t.Fatalf("error family = %q, want %q", response.Family, wantFamily)
	}
	if response.Message != wantMessage {
		t.Fatalf("error message = %q, want %q", response.Message, wantMessage)
	}
}

func assertBlockingValidationTarget(t *testing.T, target factoryapi.FactoryValidationTarget) {
	t.Helper()
	if target.Severity != factoryapi.FactoryValidationSeverityError {
		t.Fatalf("target severity = %q, want error", target.Severity)
	}
	if target.Code == "" {
		t.Fatal("target code is empty, want stable validation code")
	}
	if target.Message == "" {
		t.Fatal("target message is empty, want human-readable validation message")
	}
	if target.Subject.Type == "" || target.Subject.Location == "" {
		t.Fatalf("target subject = %#v, want structured type and location", target.Subject)
	}
	if target.Subject.Type != factoryapi.FactoryValidationSubjectTypeFactory && target.Subject.Id == "" {
		t.Fatalf("target subject = %#v, want non-empty id for component-scoped targets", target.Subject)
	}
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
