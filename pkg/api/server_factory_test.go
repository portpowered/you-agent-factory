// backendsizecheck:ignore-file merged factory and provider-session HTTP regressions until story 007 splits server_*_test.go by concern.
// pkgmaintcheck:ignore-file-lines merged factory and provider-session HTTP regressions until story 007 splits server_*_test.go by concern.
package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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

func TestGetProviderSessionDetails_LoadsCodexSessionFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", strings.Join([]string{
		`{"type":"session_meta","id":"sess_123"}`,
		`{"type":"response_item","item":{"type":"reasoning"}}`,
		`{"unexpected":true}`,
		`not-json`,
		``,
	}, "\n"))

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	assertProviderSessionResponseIdentity(t, resp)
	assertProviderSessionParseCounts(t, resp.Parse)
	assertProviderSessionTranscriptSummary(t, resp)
	assertProviderSessionParseDiagnostics(t, resp.Parse)
}

func assertProviderSessionResponseIdentity(t *testing.T, resp factoryapi.ProviderSessionDetailResponse) {
	t.Helper()

	if string(resp.ProviderSession.Provider) != "codex" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != "sess_123" {
		t.Fatalf("provider session = %#v, want codex session_id sess_123", resp.ProviderSession)
	}
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted rollout path with metadata", resp.Source)
	}
}

func assertProviderSessionParseCounts(t *testing.T, parse factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if parse.LineCount != 4 || parse.EventCount != 3 || parse.MalformedLineCount != 1 || parse.UnknownEventCount != 1 {
		t.Fatalf("parse summary = %#v, want line/event/malformed/unknown counts", parse)
	}
}

func assertProviderSessionTranscriptSummary(t *testing.T, resp factoryapi.ProviderSessionDetailResponse) {
	t.Helper()

	if len(resp.Transcript) != 1 || resp.Transcript[0].Type != factoryapi.Reasoning || resp.Transcript[0].Order != 1 {
		t.Fatalf("transcript = %#v, want one reasoning transcript entry", resp.Transcript)
	}
	if len(resp.Parse.Turns) != 1 || resp.Parse.Turns[0].ReasoningCount != 1 || len(resp.Parse.Reasoning) != 1 || resp.Parse.Reasoning[0].SourceType != "reasoning" {
		t.Fatalf("parse detail = %#v, want reasoning turn summary", resp.Parse)
	}
}

func assertProviderSessionParseDiagnostics(t *testing.T, parse factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if len(parse.ParseErrors) != 1 || parse.ParseErrors[0].LineNumber != 4 || len(parse.UnknownEvents) != 1 || parse.UnknownEvents[0].LineNumber != 3 {
		t.Fatalf("parse diagnostics = %#v, want malformed line 4 and unknown line 3", parse)
	}
}

func TestGetProviderSessionDetails_LoadsTimestampPrefixedCodexSessionFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl", strings.Join([]string{
		`{"type":"session_meta","id":"019e44f4-580e-7f32-981e-1e54ec6907d6"}`,
		`{"type":"response_item","payload":{"type":"reasoning"}}`,
	}, "\n"))

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=019e44f4-580e-7f32-981e-1e54ec6907d6", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if resp.Source.RelativePath != "2026/05/18/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl" || resp.ProviderSession.Id != "019e44f4-580e-7f32-981e-1e54ec6907d6" || resp.Parse.EventCount != 2 {
		t.Fatalf("provider session detail = %#v, want timestamp-prefixed session path", resp)
	}
}

func TestGetProviderSessionDetails_PrefersExactCodexSessionFileWhenSupportedLayoutsBothExist(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", `{"type":"session_meta","id":"sess_123"}`)
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" {
		t.Fatalf("relative path = %q, want exact rollout basename", resp.Source.RelativePath)
	}
}

func TestParseCodexSessionSummary_ExtractsDiagnosticDetails(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"reasoning","summary":["checked input"],"encrypted_content":"sealed"}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":{"cmd":"go test ./pkg/api"}}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok"}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":130}}}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:06Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"call-2","name":"apply_patch","input":"patch text","status":"in_progress"}}`,
		`{"timestamp":"2026-05-18T10:00:07Z","type":"event_msg","payload":{"type":"new_future_event"}}`,
		`{"timestamp":"2026-05-18T10:00:08Z","type":"unexpected_top_level"}`,
		`{bad json`,
	}, "\n")

	summary, err := parseCodexSessionSummary(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session summary: %v", err)
	}
	assertCodexSessionSummaryCoreCounts(t, summary)
	assertCodexSessionSummaryFunctionCalls(t, summary)
	assertCodexSessionSummaryReasoning(t, summary)
	parsed, err := parseCodexSessionDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session details: %v", err)
	}
	assertParsedCodexSessionSummaryTranscript(t, parsed)
	assertCodexSessionSummaryTokenUsage(t, summary)
	assertCodexSessionSummaryUnknowns(t, summary)
}

func TestParseCodexSessionDetails_EmitsMixedTranscriptChronologically(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Inspect the failing run."}]}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"reasoning","summary":["Checking tool output"],"encrypted_content":"sealed"}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":{"cmd":"go test ./pkg/api"}}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok","status":"completed"}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"The package tests passed."}}`,
		`{"timestamp":"2026-05-18T10:00:06Z","type":"event_msg","payload":{"type":"task_started","message":"Applying follow-up patch"}}`,
		`{"timestamp":"2026-05-18T10:00:07Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"Need one more validation step."}}`,
		`{"timestamp":"2026-05-18T10:00:08Z","type":"event_msg","payload":{"type":"new_future_event"}}`,
		`{"timestamp":"2026-05-18T10:00:09Z","type":"unexpected_top_level"}`,
		`{bad json`,
	}, "\n")

	parsed, err := parseCodexSessionDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session details: %v", err)
	}

	assertMixedCodexSessionSummary(t, parsed.Summary)
	assertMixedCodexSessionTranscript(t, parsed)
}

func assertCodexSessionSummaryCoreCounts(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if summary.LineCount != 10 || summary.EventCount != 9 || summary.MalformedLineCount != 1 || summary.UnknownEventCount != 2 || len(summary.Turns) != 2 || len(summary.FunctionCalls) != 2 {
		t.Fatalf("summary = %#v, want parsed counts and two turns/calls", summary)
	}
}

func assertCodexSessionSummaryFunctionCalls(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	firstCall := summary.FunctionCalls[0]
	if firstCall.Order != 1 || stringValue(firstCall.Name) != "exec_command" || stringValue(firstCall.Arguments) != `{"cmd":"go test ./pkg/api"}` || stringValue(firstCall.Output) != "ok" || stringValue(firstCall.Status) != "completed" {
		t.Fatalf("first function call = %#v, want completed exec_command call", firstCall)
	}
	secondCall := summary.FunctionCalls[1]
	if secondCall.Order != 2 || stringValue(secondCall.Name) != "apply_patch" || stringValue(secondCall.Status) != "in_progress" || stringValue(secondCall.Output) != "" {
		t.Fatalf("second function call = %#v, want in-progress custom tool call", secondCall)
	}
}

func assertCodexSessionSummaryReasoning(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if len(summary.Reasoning) != 1 || stringValue(summary.Reasoning[0].Summary) != `["checked input"]` || summary.Reasoning[0].Encrypted == nil || !*summary.Reasoning[0].Encrypted || stringValue(summary.Reasoning[0].EncryptedContent) != "sealed" {
		t.Fatalf("reasoning = %#v, want summary, encrypted marker, and encrypted content", summary.Reasoning)
	}
}

func assertParsedCodexSessionSummaryTranscript(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if len(parsed.Transcript) != 4 {
		t.Fatalf("transcript = %#v, want four ordered transcript entries", parsed.Transcript)
	}
	if parsed.Transcript[0].Type != factoryapi.Reasoning || stringValue(parsed.Transcript[0].SourceType) != "reasoning" || intValue(parsed.Transcript[0].LineNumber) != 2 {
		t.Fatalf("first transcript entry = %#v, want reasoning line 2", parsed.Transcript[0])
	}
	if parsed.Transcript[1].Type != factoryapi.ToolCall || stringValue(parsed.Transcript[1].Name) != "exec_command" || stringValue(parsed.Transcript[1].Arguments) != `{"cmd":"go test ./pkg/api"}` {
		t.Fatalf("second transcript entry = %#v, want exec_command tool call", parsed.Transcript[1])
	}
	if parsed.Transcript[2].Type != factoryapi.ToolOutput || stringValue(parsed.Transcript[2].Output) != "ok" || stringValue(parsed.Transcript[2].Status) != "completed" {
		t.Fatalf("third transcript entry = %#v, want completed tool output", parsed.Transcript[2])
	}
	if parsed.Transcript[3].Type != factoryapi.ToolCall || stringValue(parsed.Transcript[3].Name) != "apply_patch" || stringValue(parsed.Transcript[3].Status) != "in_progress" {
		t.Fatalf("fourth transcript entry = %#v, want in-progress apply_patch tool call", parsed.Transcript[3])
	}
	if parsed.Transcript[3].Order != 4 {
		t.Fatalf("final transcript entry order = %d, want 4", parsed.Transcript[3].Order)
	}
}

func assertCodexSessionSummaryTokenUsage(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if summary.TokenUsage == nil || intValue(summary.TokenUsage.InputTokens) != 100 || intValue(summary.TokenUsage.CachedInputTokens) != 40 || intValue(summary.TokenUsage.OutputTokens) != 25 || intValue(summary.TokenUsage.ReasoningOutputTokens) != 5 || intValue(summary.TokenUsage.TotalTokens) != 130 {
		t.Fatalf("token usage = %#v, want total consumed token fields", summary.TokenUsage)
	}
}

func assertCodexSessionSummaryUnknowns(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if len(summary.UnknownEvents) != 2 || summary.UnknownEvents[0].LineNumber != 8 || stringValue(summary.UnknownEvents[0].Type) != "event_msg" || stringValue(summary.UnknownEvents[0].PayloadType) != "new_future_event" || summary.UnknownEvents[1].LineNumber != 9 || stringValue(summary.UnknownEvents[1].Type) != "unexpected_top_level" {
		t.Fatalf("unknown events = %#v, want compact line-level unknown records", summary.UnknownEvents)
	}
	if len(summary.ParseErrors) != 1 || summary.ParseErrors[0].LineNumber != 10 {
		t.Fatalf("parse errors = %#v, want malformed line retained", summary.ParseErrors)
	}
}

func assertMixedCodexSessionSummary(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if summary.LineCount != 11 || summary.EventCount != 10 || summary.MalformedLineCount != 1 || summary.UnknownEventCount != 2 {
		t.Fatalf("summary = %#v, want mixed-session diagnostic counts", summary)
	}
	if len(summary.Turns) != 1 || summary.Turns[0].FunctionCallCount != 1 || summary.Turns[0].ReasoningCount != 2 {
		t.Fatalf("turn summary = %#v, want one turn with function and reasoning counts", summary.Turns)
	}
	if len(summary.UnknownEvents) != 2 || summary.UnknownEvents[0].LineNumber != 9 || summary.UnknownEvents[1].LineNumber != 10 {
		t.Fatalf("unknown events = %#v, want unknown event_msg and top-level event retained", summary.UnknownEvents)
	}
	if len(summary.ParseErrors) != 1 || summary.ParseErrors[0].LineNumber != 11 {
		t.Fatalf("parse errors = %#v, want malformed line 11 retained", summary.ParseErrors)
	}
}

func assertMixedCodexSessionTranscriptLength(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if len(parsed.Transcript) != 7 {
		t.Fatalf("transcript = %#v, want seven ordered transcript entries", parsed.Transcript)
	}
}

func assertMixedCodexSessionTranscriptEntry(t *testing.T, parsed parsedCodexSessionDetails, index int, wantType factoryapi.ProviderSessionTranscriptEntryType, wantLine int, wantText string) {
	t.Helper()

	entry := parsed.Transcript[index]
	if entry.Order != index+1 || entry.Type != wantType || intValue(entry.LineNumber) != wantLine || stringValue(entry.Text) != wantText {
		t.Fatalf("transcript[%d] = %#v, want order=%d type=%q line=%d text=%q", index, entry, index+1, wantType, wantLine, wantText)
	}
	if intValue(entry.TurnIndex) != 1 {
		t.Fatalf("transcript[%d] turn index = %#v, want 1", index, entry.TurnIndex)
	}
}

func assertMixedCodexSessionTranscript(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	assertMixedCodexSessionTranscriptLength(t, parsed)
	assertMixedCodexSessionTranscriptUserMessage(t, parsed)
	assertMixedCodexSessionTranscriptReasoning(t, parsed)
	assertMixedCodexSessionTranscriptToolEvents(t, parsed)
	assertMixedCodexSessionTranscriptAssistantAndSystemEvents(t, parsed)
}

func assertMixedCodexSessionTranscriptUserMessage(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	assertMixedCodexSessionTranscriptEntry(t, parsed, 0, factoryapi.UserMessage, 2, "Inspect the failing run.")
	if parsed.Transcript[0].SourceType == nil || *parsed.Transcript[0].SourceType != "message" {
		t.Fatalf("first transcript source type = %#v, want message", parsed.Transcript[0].SourceType)
	}
}

func assertMixedCodexSessionTranscriptReasoning(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if parsed.Transcript[1].Order != 2 || parsed.Transcript[1].Type != factoryapi.Reasoning || intValue(parsed.Transcript[1].LineNumber) != 3 || stringValue(parsed.Transcript[1].Summary) != `["Checking tool output"]` || parsed.Transcript[1].Encrypted == nil || !*parsed.Transcript[1].Encrypted || stringValue(parsed.Transcript[1].EncryptedContent) != "sealed" {
		t.Fatalf("transcript[1] = %#v, want encrypted reasoning summary and content on line 3", parsed.Transcript[1])
	}

	assertMixedCodexSessionTranscriptEntry(t, parsed, 6, factoryapi.Reasoning, 8, "Need one more validation step.")
	if parsed.Transcript[6].SourceType == nil || *parsed.Transcript[6].SourceType != "agent_reasoning" {
		t.Fatalf("final reasoning transcript source type = %#v, want agent_reasoning", parsed.Transcript[6].SourceType)
	}
}

func assertMixedCodexSessionTranscriptToolEvents(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if parsed.Transcript[2].Order != 3 || parsed.Transcript[2].Type != factoryapi.ToolCall || intValue(parsed.Transcript[2].LineNumber) != 4 || stringValue(parsed.Transcript[2].Name) != "exec_command" {
		t.Fatalf("transcript[2] = %#v, want tool call on line 4", parsed.Transcript[2])
	}
	if parsed.Transcript[3].Order != 4 || parsed.Transcript[3].Type != factoryapi.ToolOutput || intValue(parsed.Transcript[3].LineNumber) != 5 || stringValue(parsed.Transcript[3].Output) != "ok" || stringValue(parsed.Transcript[3].Status) != "completed" {
		t.Fatalf("transcript[3] = %#v, want tool output on line 5", parsed.Transcript[3])
	}
}

func assertMixedCodexSessionTranscriptAssistantAndSystemEvents(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	assertMixedCodexSessionTranscriptEntry(t, parsed, 4, factoryapi.AssistantMessage, 6, "The package tests passed.")
	if parsed.Transcript[4].SourceType == nil || *parsed.Transcript[4].SourceType != "agent_message" {
		t.Fatalf("assistant transcript source type = %#v, want agent_message", parsed.Transcript[4].SourceType)
	}

	assertMixedCodexSessionTranscriptEntry(t, parsed, 5, factoryapi.SystemEvent, 7, "Applying follow-up patch")
	if parsed.Transcript[5].SourceType == nil || *parsed.Transcript[5].SourceType != "task_started" {
		t.Fatalf("system-event transcript source type = %#v, want task_started", parsed.Transcript[5].SourceType)
	}
}

func TestParseCodexSessionSummary_AcceptsLargeJSONLRecords(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"reasoning","content":"` + strings.Repeat("x", 128*1024) + `"}}`,
	}, "\n")

	summary, err := parseCodexSessionSummary(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session summary: %v", err)
	}
	if summary.LineCount != 2 || summary.EventCount != 2 || len(summary.Reasoning) != 1 {
		t.Fatalf("summary = %#v, want large response item parsed successfully", summary)
	}
}

func TestGetProviderSessionDetails_NotFoundIsDistinguishable(t *testing.T) {
	srv := newTestServerWithCodexRoot(t.TempDir())
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_IgnoresUnsupportedRolloutFileNames(t *testing.T) {
	root := t.TempDir()
	writeNamedProviderSessionFixture(t, root, "rollout-backup-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-backup-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_RejectsPathLikeAndMalformedIdentifiers(t *testing.T) {
	for _, target := range []string{
		"/provider-sessions/detail?provider=codex&kind=session_id&id=../secret",
		"/provider-sessions/detail?provider=codex&kind=session_id&id=/tmp/rollout-session.jsonl",
		"/provider-sessions/detail?provider=codex&kind=session_id&id=session.with.dot",
	} {
		t.Run(target, func(t *testing.T) {
			srv := newTestServerWithCodexRoot(t.TempDir())
			req := httptest.NewRequest("GET", target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a codex session_id identifier without path separators")
		})
	}
}

func TestGetProviderSessionDetails_RejectsUnsupportedProviderOrKindByContract(t *testing.T) {
	for _, target := range []string{
		"/provider-sessions/detail?provider=openai&kind=session_id&id=sess-123",
		"/provider-sessions/detail?provider=codex&kind=path&id=sess-123",
	} {
		t.Run(target, func(t *testing.T) {
			srv := newTestServerWithCodexRoot(t.TempDir())
			req := httptest.NewRequest("GET", target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request parameter")
		})
	}
}

func TestGetProviderSessionDetails_RejectsSessionSymlinkOutsideConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideSessionPath := filepath.Join(outside, "rollout-sess-outside.jsonl")
	if err := os.WriteFile(outsideSessionPath, []byte(`{"type":"session_meta"}`), 0o600); err != nil {
		t.Fatalf("write outside session fixture: %v", err)
	}
	sessionDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := os.Symlink(outsideSessionPath, filepath.Join(sessionDir, "rollout-sess-outside.jsonl")); err != nil {
		t.Fatalf("create provider session symlink: %v", err)
	}

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess-outside", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a codex session_id identifier without path separators")
}

func TestGetProviderSessionDetails_RejectsSessionSymlinkOutsideConfiguredRootEvenWhenValidMatchExists(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess-shared", `{"type":"session_meta","id":"sess-shared"}`)
	outside := t.TempDir()
	outsideSessionPath := filepath.Join(outside, "rollout-2026-05-20T17-35-24-sess-shared.jsonl")
	if err := os.WriteFile(outsideSessionPath, []byte(`{"type":"session_meta"}`), 0o600); err != nil {
		t.Fatalf("write outside session fixture: %v", err)
	}
	sessionDir := filepath.Join(root, "2026", "05", "18")
	if err := os.Symlink(outsideSessionPath, filepath.Join(sessionDir, "rollout-2026-05-20T17-35-24-sess-shared.jsonl")); err != nil {
		t.Fatalf("create provider session symlink: %v", err)
	}

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess-shared", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a codex session_id identifier without path separators")
}

func TestGetProviderSessionDetails_FailsForAmbiguousTimestampPrefixedMatches(t *testing.T) {
	root := t.TempDir()
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)
	sessionDir := filepath.Join(root, "2026", "05", "19")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("create provider session fixture directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "rollout-2026-05-20T17-45-24-sess_123.jsonl"), []byte(`{"type":"session_meta","id":"sess_123"}`), 0o600); err != nil {
		t.Fatalf("write second timestamp-prefixed provider session fixture: %v", err)
	}

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusInternalServerError, "INTERNAL_ERROR", "multiple provider session files match session identifier")
}
