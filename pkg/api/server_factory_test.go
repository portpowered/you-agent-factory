package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestCreateFactoryRoute_RemovedFromRouter(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /factories status = %d, want route removed", rec.Code)
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

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta","metadata":{"owner":"graph-editor"},"workTypes":[{"name":"beta-task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}]}`)))
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
	srv := newTestServer(&testutil.MockFactory{SaveFactoryForSessionErr: apisurface.ErrInvalidNamedFactory})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
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
	srv := newTestServer(&testutil.MockFactory{SaveFactoryForSessionErr: apisurface.NewTopologyValidationError("dangling output", []factoryapi.FactoryValidationTarget{target})})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
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
	srv := newTestServer(&testutil.MockFactory{SaveFactoryForSessionErr: apisurface.ErrFactoryVersionStale})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
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
		SaveFactoryForSessionErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			[]factoryapi.FactoryValidationTarget{target},
		),
	})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest || response.Code != factoryapi.INVALIDFACTORY {
		t.Fatalf("response = %#v status=%d", response, rec.Code)
	}
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical target", response.Targets)
	}
	got := (*response.Targets)[0]
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

func TestUpsertWorkRequest_NormalizesLegacyStringPayloadIntoCanonicalContent(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-1", `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd","payload":"legacy text"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 || len(mf.Submitted[0].Content) != 1 || mf.Submitted[0].Content[0].Text != "legacy text" {
		t.Fatalf("submitted content = %#v, want canonical text content", mf.Submitted)
	}
}

func TestUpsertWorkRequest_RejectsInvalidContentPartShape(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)
	rec := upsertWorkRequest(t, srv, "/work-requests/request-1", `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd","content":[{"type":"text","file":"wrong"}]}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].content[0].file is not supported")
	if len(mf.Submitted) != 0 || len(mf.WorkRequests) != 0 {
		t.Fatalf("submissions = workRequests:%d submitted:%d, want 0/0", len(mf.WorkRequests), len(mf.Submitted))
	}
}

func TestUpsertWorkRequest_AcceptsCanonicalContent(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-canonical", `{
		"requestId":"request-canonical",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"draft","workTypeName":"prd","content":[{"type":"text","text":"Review this UI."},{"type":"image","file":"fixtures/ui.png"}]}]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("submitted count = %d, want 1", len(mf.Submitted))
	}
	if len(mf.Submitted[0].Content) != 2 {
		t.Fatalf("content count = %d, want 2", len(mf.Submitted[0].Content))
	}
	if mf.Submitted[0].Content[0].Type != interfaces.WorkContentPartTypeText || mf.Submitted[0].Content[0].Text != "Review this UI." {
		t.Fatalf("submitted content[0] = %#v, want canonical text content", mf.Submitted[0].Content[0])
	}
	if mf.Submitted[0].Content[1].Type != interfaces.WorkContentPartTypeImage || mf.Submitted[0].Content[1].File != "fixtures/ui.png" {
		t.Fatalf("submitted content[1] = %#v, want canonical image content", mf.Submitted[0].Content[1])
	}
}

func TestUpsertWorkRequest_AcceptsUppercaseAndExtendedContent(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-model-content", `{
		"requestId":"request-model-content",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{
			"name":"draft",
			"workTypeName":"prd",
			"content":[
				{"type":"IMAGE","file":"fixtures/ui.png","label":"reference"},
				{"type":"BINARY","file":"artifacts/raw.bin","contentType":"application/octet-stream"},
				{"type":"JSON","json":{"mode":"preview"}}
			]
		}]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 || len(mf.Submitted[0].Content) != 3 {
		t.Fatalf("submitted content = %#v, want 3 canonical parts", mf.Submitted)
	}
	if mf.Submitted[0].Content[0].Type != interfaces.WorkContentPartTypeImage || mf.Submitted[0].Content[0].Label != "reference" {
		t.Fatalf("submitted content[0] = %#v, want normalized image part", mf.Submitted[0].Content[0])
	}
	if mf.Submitted[0].Content[1].Type != interfaces.WorkContentPartTypeBinary || mf.Submitted[0].Content[1].ContentType != "application/octet-stream" {
		t.Fatalf("submitted content[1] = %#v, want canonical binary part", mf.Submitted[0].Content[1])
	}
	if mf.Submitted[0].Content[2].Type != interfaces.WorkContentPartTypeJSON {
		t.Fatalf("submitted content[2] = %#v, want canonical json part", mf.Submitted[0].Content[2])
	}
	jsonValue := map[string]any{}
	if err := json.Unmarshal(mf.Submitted[0].Content[2].JSON, &jsonValue); err != nil {
		t.Fatalf("decode json content: %v", err)
	}
	if jsonValue["mode"] != "preview" {
		t.Fatalf("submitted content[2].json = %s, want preview json", mf.Submitted[0].Content[2].JSON)
	}
}

func TestUpsertWorkRequest_FirstSubmitAndRepeatedRequestID(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	var firstTraceID string
	for i, body := range []string{
		`{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","traceId":"trace-original","payload":{"title":"Draft"}}]}`,
		`{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"changed-draft","workTypeName":"task","traceId":"trace-retry","payload":{"title":"Changed retry"}}]}`,
	} {
		rec := upsertWorkRequest(t, srv, "/work-requests/request-api-1", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
		}
		resp := decodeJSONResponse[factoryapi.UpsertWorkRequestResponse](t, rec)
		if resp.RequestId != "request-api-1" || resp.TraceId == "" {
			t.Fatalf("upsert response = %#v, want request and trace", resp)
		}
		if i == 0 {
			firstTraceID = resp.TraceId
		} else if resp.TraceId != firstTraceID {
			t.Fatalf("repeated trace_id = %q, want original %q", resp.TraceId, firstTraceID)
		}
	}

	if len(mf.WorkRequests) != 1 || len(mf.Submitted) != 1 {
		t.Fatalf("submissions = workRequests:%d submitted:%d, want 1/1", len(mf.WorkRequests), len(mf.Submitted))
	}
	if mf.Submitted[0].RequestID != "request-api-1" || mf.Submitted[0].TraceID != "trace-original" || mf.Submitted[0].Name != "draft" {
		t.Fatalf("submitted request = %#v, want original request metadata", mf.Submitted[0])
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this upsert boundary test keeps the full relation and runtime mapping contract inline for reviewer-readable coverage.
func TestUpsertWorkRequest_MapsWorkTypeNameAndRelationsToRuntime(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-batch", `{
		"requestId":"request-api-batch",
		"currentChainingTraceId":"chain-request-batch",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[
			{"name":"draft","workTypeName":"task","state":"queued","currentChainingTraceId":"chain-draft","traceId":"chain-draft","payload":{"title":"Draft"}},
			{"name":"review","workTypeName":"review","payload":"review draft"}
		],
		"relations":[{"type":"DEPENDS_ON","sourceWorkName":"review","targetWorkName":"draft","requiredState":"complete"}]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	submittedRequest := mf.WorkRequests[0]
	if len(mf.WorkRequests) != 1 || len(submittedRequest.Works) != 2 {
		t.Fatalf("work request submissions = %#v, want one request with two works", mf.WorkRequests)
	}
	if submittedRequest.CurrentChainingTraceID != "chain-request-batch" || submittedRequest.Works[0].CurrentChainingTraceID != "chain-draft" || submittedRequest.Works[1].CurrentChainingTraceID != "chain-request-batch" {
		t.Fatalf("work request chaining traces = %#v", submittedRequest)
	}
	if submittedRequest.Works[0].WorkTypeID != "task" || submittedRequest.Works[1].WorkTypeID != "review" || submittedRequest.Works[0].State != "queued" {
		t.Fatalf("domain works = %#v, want task/review and queued draft", submittedRequest.Works)
	}
	if len(submittedRequest.Relations) != 1 || submittedRequest.Relations[0].SourceWorkName != "review" || submittedRequest.Relations[0].TargetWorkName != "draft" {
		t.Fatalf("domain relation = %#v, want review depends on draft", submittedRequest.Relations)
	}
	if len(mf.Submitted) != 2 {
		t.Fatalf("normalized submissions = %d, want 2", len(mf.Submitted))
	}
	relation := mf.Submitted[1].Relations[0]
	if relation.TargetWorkID != "batch-request-api-batch-draft" || relation.RequiredState != "complete" {
		t.Fatalf("normalized relation = %#v, want dependency on draft completion", relation)
	}
}

func TestUpsertWorkRequest_ReturnsPerWorkIdentifiers(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-batch", `{
		"requestId":"request-api-batch",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[
			{"name":"draft","workTypeName":"task","payload":{"title":"Draft"}},
			{"name":"review","workTypeName":"review","payload":"review draft"}
		]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.UpsertWorkRequestResponse](t, rec)
	if resp.RequestId != "request-api-batch" || resp.TraceId == "" {
		t.Fatalf("upsert response = %#v, want request and trace", resp)
	}
	if len(resp.Works) != 2 {
		t.Fatalf("upsert works = %#v, want 2 items", resp.Works)
	}
	want := []factoryapi.UpsertWorkRequestSubmittedWork{
		{Name: "draft", WorkTypeName: "task", WorkId: "batch-request-api-batch-draft"},
		{Name: "review", WorkTypeName: "review", WorkId: "batch-request-api-batch-review"},
	}
	for i, work := range resp.Works {
		if work != want[i] {
			t.Fatalf("upsert works[%d] = %#v, want %#v", i, work, want[i])
		}
	}
}

func TestUpsertWorkRequest_AcceptsParentChildRelationsByWorkName(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-parent-child", `{
		"requestId":"request-api-parent-child",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[
			{"name":"parent","workTypeName":"task","traceId":"trace-parent-child","payload":{"title":"Parent"}},
			{"name":"prerequisite","workTypeName":"task","payload":{"title":"Prerequisite"}},
			{"name":"child","workTypeName":"task","payload":{"title":"Child"}}
		],
		"relations":[
			{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"},
			{"type":"DEPENDS_ON","sourceWorkName":"child","targetWorkName":"prerequisite"}
		]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Relations) != 2 || mf.WorkRequests[0].Relations[0].Type != interfaces.WorkRelationParentChild {
		t.Fatalf("work request relations = %#v, want parent-child plus dependency", mf.WorkRequests)
	}
	child := submittedRequestNamed(t, mf.Submitted, "child")
	if child.TraceID != "trace-parent-child" || len(child.Relations) != 2 {
		t.Fatalf("normalized child = %#v, want inherited trace and relations", child)
	}
	assertSubmittedChildRelations(t, child.Relations)
}

func TestUpsertWorkRequest_CopiesWorkTagMapBeforeRuntimeSubmission(t *testing.T) {
	workTags := factoryapi.StringMap{"priority": "high"}
	req := factoryapi.WorkRequest{
		RequestId: "request-tag-copy",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:         "draft",
				WorkTypeName: stringPointerForAPITest("task"),
				Payload:      map[string]any{"title": "Draft"},
				Tags:         &workTags,
			},
		},
	}
	domain, err := generatedWorkRequestToDomain(req)
	if err != nil {
		t.Fatalf("generatedWorkRequestToDomain error = %v", err)
	}
	if len(domain.Works) != 1 {
		t.Fatalf("domain works = %#v, want one work", domain.Works)
	}

	workTags["priority"] = "mutated"
	workTags["post"] = "added"

	if domain.Works[0].Tags["priority"] != "high" {
		t.Fatalf("domain work tags = %#v, want pre-mutation values", domain.Works[0].Tags)
	}
	if _, ok := domain.Works[0].Tags["post"]; ok {
		t.Fatalf("domain work tags = %#v, want copied map to omit post-decode additions", domain.Works[0].Tags)
	}
}

func TestUpsertWorkRequest_WorkTypeIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-legacy", `{"requestId":"request-api-legacy","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","work_type_id":"legacy-task","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].work_type_id is not supported; use workTypeName")
}

func TestUpsertWorkRequest_TargetStateReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-state-alias", `{"requestId":"request-api-state-alias","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","target_state":"queued","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].target_state is not supported; use state")
}

func TestUpsertWorkRequest_ConflictingCurrentChainingTraceIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-chaining-conflict", `{"requestId":"request-api-chaining-conflict","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","currentChainingTraceId":"chain-a","traceId":"trace-b","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].currentChainingTraceId and traceId must match when both are provided")
}

func TestUpsertWorkRequest_InvalidExplicitStateReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		Net: &state.Net{WorkTypes: map[string]*state.WorkType{
			"task": {ID: "task", States: []state.StateDefinition{{Value: "init", Category: state.StateCategoryInitial}, {Value: "complete", Category: state.StateCategoryTerminal}}},
		}},
	}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-invalid-state", `{"requestId":"request-api-invalid-state","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","state":"queued","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", `work_request: works[0] ("draft") references unknown state "queued" for work type name "task"`)
}

func TestUpsertWorkRequestValidationFailures(t *testing.T) {
	runUpsertValidationFailureCases(t, []upsertValidationFailureCase{
		{name: "invalid_json", path: "/work-requests/request-api-1", body: `{"requestId":`, wantMsg: "invalid request payload"},
		{name: "missing_required_request_id", path: "/work-requests/request-api-1", body: `{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task"}]}`, wantMsg: "requestId is required"},
		{name: "path_body_mismatch", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-2","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task"}]}`, wantMsg: "request_id path and requestId body must match"},
		{name: "cycle_error", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"a","workTypeName":"task"},{"name":"b","workTypeName":"task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"a","targetWorkName":"b"},{"type":"DEPENDS_ON","sourceWorkName":"b","targetWorkName":"a"}]}`, wantMsg: `work_request: dependency cycle detected involving "a"`},
		{name: "malformed_relation", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"a","workTypeName":"task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"a","targetWorkName":"missing"}]}`, wantMsg: `work_request: relations[0] references unknown targetWorkName "missing"`},
		{name: "self_parenting_relation", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"a","workTypeName":"task"}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":"a","targetWorkName":"a"}]}`, wantMsg: `work_request: relations[0] has self-parenting on "a"`},
	})

	runUpsertValidationFailureCases(t, []upsertValidationFailureCase{
		{name: "duplicate_parent_child_relation", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"parent","workTypeName":"task"},{"name":"child","workTypeName":"task"}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"},{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"}]}`, wantMsg: `work_request: relations[1] duplicates relations[0] ("PARENT_CHILD" "child" -> "parent")`},
		{name: "missing_work_type_name", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft"}]}`, wantMsg: `work_request: works[0] ("draft") is missing workTypeName`},
		{name: "work_type_id_not_supported", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","work_type_id":"legacy-task"}]}`, wantMsg: `works[0].work_type_id is not supported; use workTypeName`},
		{name: "unknown_work_type", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"unknown"}]}`, factory: &testutil.MockFactory{SubmitWorkRequestErr: errors.New(`work_request: works[0] ("draft") references unknown work type "unknown"`)}, wantMsg: `work_request: works[0] ("draft") references unknown work type name "unknown"`},
		{
			name: "invalid_dependency_required_state",
			path: "/work-requests/request-api-1",
			body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task"},{"name":"review","workTypeName":"task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"review","targetWorkName":"draft","requiredState":"queued"}]}`,
			factory: &testutil.MockFactory{
				Net: &state.Net{
					WorkTypes: map[string]*state.WorkType{
						"task": {
							ID: "task",
							States: []state.StateDefinition{
								{Value: "init", Category: state.StateCategoryInitial},
								{Value: "complete", Category: state.StateCategoryTerminal},
							},
						},
					},
				},
			},
			wantMsg: `work_request: relations[0] references unknown requiredState "queued" for target work type name "task"`,
		},
	})
}

type upsertValidationFailureCase struct {
	name    string
	path    string
	body    string
	factory *testutil.MockFactory
	wantMsg string
}

func runUpsertValidationFailureCases(t *testing.T, cases []upsertValidationFailureCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mf := tc.factory
			if mf == nil {
				mf = &testutil.MockFactory{}
			}
			mf.Marking = &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}
			srv := newTestServer(mf)

			rec := upsertWorkRequest(t, srv, tc.path, tc.body)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", tc.wantMsg)
			if len(mf.Submitted) != 0 {
				t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
			}
		})
	}
}
