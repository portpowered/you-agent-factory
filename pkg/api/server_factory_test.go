// backendsizecheck:ignore-file merged factory, session-scoping, and provider-session HTTP regressions until story 007 splits server_*_test.go by concern.
// pkgmaintcheck:ignore-file-lines merged factory, session-scoping, and provider-session HTTP regressions until story 007 splits server_*_test.go by concern.
package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
func TestSessionScopedAPI_ReadsAndMutationsTargetOnlyRequestedSession(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	defaultFactoryID := "root-runtime"
	betaFactoryID := "beta-runtime"
	defaultSession := newSessionScopedMockFactory(now, &defaultFactoryID, apisurface.DefaultCurrentFactoryName, "tok-default-1", "default-work-1", "factory-event/work-request/default-history")
	betaSession := newSessionScopedMockFactory(now, &betaFactoryID, "beta", "tok-beta-1", "beta-work-1", "factory-event/work-request/beta-history")
	srv := newTestServer(&testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: apisurface.DefaultCurrentFactoryName, Id: &defaultFactoryID},
		SessionFactories: map[string]*testutil.MockFactory{
			"~default":     defaultSession,
			"session-beta": betaSession,
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	assertScopedSessionSubmit(t, server.URL, betaSession, defaultSession)
	assertScopedSessionList(t, server.URL, betaSession, defaultSession)
	assertScopedSessionWorkRead(t, server.URL)
	assertScopedSessionStatus(t, server.URL)
	assertScopedCurrentFactory(t, server.URL, "beta")
	assertScopedSessionEvents(t, server.URL, "factory-event/work-request/beta-history")
}

func newSessionScopedMockFactory(
	now time.Time,
	factoryID *string,
	factoryName string,
	tokenID string,
	workID string,
	historyEventID string,
) *testutil.MockFactory {
	return &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				tokenID: listWorkToken(tokenID, workID, "task:init", "task", now),
			},
		},
		Net: sessionScopedStateNet(),
		FactoryEventStream: &interfaces.FactoryEventStream{
			History: []factoryapi.FactoryEvent{{Id: historyEventID, Type: factoryapi.FactoryEventTypeWorkRequest}},
			Events:  make(chan factoryapi.FactoryEvent),
		},
		CurrentFactory: &factoryapi.Factory{Name: factoryName, Id: factoryID},
	}
}

func sessionScopedStateNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init": {ID: "task:init", TypeID: "task", State: "init"},
			"task:done": {ID: "task:done", TypeID: "task", State: "done"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "done", Category: state.StateCategoryTerminal},
				},
			},
		},
	}
}

func assertScopedSessionSubmit(t *testing.T, serverURL string, betaSession *testutil.MockFactory, defaultSession *testutil.MockFactory) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodPost, serverURL+"/factory-sessions/session-beta/work", bytes.NewBufferString(`{"name":"scoped-submit","workTypeName":"task","traceId":"trace-scoped-submit","payload":{"title":"scoped"}}`), "application/json", http.StatusCreated)
	defer response.Body.Close()

	var submitBody factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&submitBody); err != nil {
		t.Fatalf("decode scoped submit response: %v", err)
	}
	assertSubmitWorkResponseIdentifiers(t, submitBody, submitWorkResponseExpectation{
		traceID:      "trace-scoped-submit",
		name:         "scoped-submit",
		workTypeName: "task",
		sessionID:    "session-beta",
		workIDSuffix: "-scoped-submit",
	})

	if len(betaSession.WorkRequests) != 1 {
		t.Fatalf("beta submitted work requests = %d, want 1", len(betaSession.WorkRequests))
	}
	if len(defaultSession.WorkRequests) != 0 {
		t.Fatalf("default submitted work requests = %d, want 0", len(defaultSession.WorkRequests))
	}
}

func assertScopedSessionList(t *testing.T, serverURL string, betaSession *testutil.MockFactory, defaultSession *testutil.MockFactory) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/work", nil, "", http.StatusOK)
	defer response.Body.Close()

	var listBody factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode scoped list response: %v", err)
	}
	if len(listBody.Results) != 1 || stringValue(listBody.Results[0].WorkId) != "beta-work-1" {
		t.Fatalf("scoped list results = %#v, want beta-work-1", listBody.Results)
	}
	if betaSession.EngineStateSnapshotCalls == 0 {
		t.Fatal("expected scoped GET /work to read the targeted session snapshot")
	}
	if defaultSession.EngineStateSnapshotCalls != 0 {
		t.Fatalf("default session snapshot calls = %d, want 0 after scoped list", defaultSession.EngineStateSnapshotCalls)
	}
}

func assertScopedSessionWorkRead(t *testing.T, serverURL string) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/work/tok-beta-1", nil, "", http.StatusOK)
	defer response.Body.Close()
}

func assertScopedSessionStatus(t *testing.T, serverURL string) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/status", nil, "", http.StatusOK)
	defer response.Body.Close()
}

func assertScopedCurrentFactory(t *testing.T, serverURL string, wantName string) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/factory", nil, "", http.StatusOK)
	defer response.Body.Close()

	var currentBody factoryapi.Factory
	if err := json.NewDecoder(response.Body).Decode(&currentBody); err != nil {
		t.Fatalf("decode scoped current factory response: %v", err)
	}
	if currentBody.Name != wantName {
		t.Fatalf("scoped current factory name = %q, want %s", currentBody.Name, wantName)
	}
}

func assertScopedSessionEvents(t *testing.T, serverURL string, wantEventID string) {
	t.Helper()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+"/factory-sessions/session-beta/events", nil)
	if err != nil {
		t.Fatalf("new scoped /events request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET /factory-sessions/session-beta/events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET /factory-sessions/session-beta/events status = %d, want 200: %s", response.StatusCode, string(body))
	}

	streamed := readSSEFactoryEvent(t, bufio.NewReader(response.Body))
	if streamed.Id != wantEventID {
		t.Fatalf("scoped streamed event id = %q, want %s", streamed.Id, wantEventID)
	}
}

func requireHTTPSuccess(
	t *testing.T,
	method string,
	url string,
	body io.Reader,
	contentType string,
	wantStatus int,
) *http.Response {
	t.Helper()

	var (
		response *http.Response
		err      error
	)
	switch method {
	case http.MethodGet:
		response, err = http.Get(url)
	case http.MethodPost:
		response, err = http.Post(url, contentType, body)
	default:
		request, requestErr := http.NewRequestWithContext(context.Background(), method, url, body)
		if requestErr != nil {
			t.Fatalf("%s %s request: %v", method, url, requestErr)
		}
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		response, err = http.DefaultClient.Do(request)
	}
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	if response.StatusCode != wantStatus {
		bodyBytes, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("%s %s status = %d, want %d: %s", method, url, response.StatusCode, wantStatus, string(bodyBytes))
	}
	return response
}

func TestSessionScopedAPI_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/missing-session/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestFactorySessionsAPI_ListFactorySessions(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		FactorySessions: factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{
				{
					FactoryDir: "/workspace/root",
					FolderPath: "/workspace/root",
					Id:         "~default",
					IsDefault:  true,
					Project:    "root",
					Target: factoryapi.FactorySessionTargetRef{
						Kind: factoryapi.FactorySessionTargetRefKindDefault,
					},
				},
				{
					FactoryDir: "/workspace/root/beta",
					FolderPath: "/workspace/root",
					Id:         "session-beta",
					IsDefault:  false,
					Project:    "beta",
					Target: factoryapi.FactorySessionTargetRef{
						Kind: factoryapi.FactorySessionTargetRefKindNamed,
						Name: stringPointerForAPITest("beta"),
					},
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.ListFactorySessionsResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode list factory sessions response: %v", err)
	}
	if len(response.Sessions) != 2 || response.Sessions[1].Id != "session-beta" {
		t.Fatalf("factory sessions = %#v, want default and beta sessions", response.Sessions)
	}
}

func TestFactorySessionsAPI_OpenFactorySession(t *testing.T) {
	mf := &testutil.MockFactory{
		OpenFactorySessionResult: factoryapi.OpenFactorySessionResponse{
			Session: &factoryapi.FactorySessionSummary{
				FactoryDir: "/workspace/fleet/beta",
				FolderPath: "/workspace/fleet",
				Id:         "session-beta",
				IsDefault:  false,
				Project:    "beta",
				Target: factoryapi.FactorySessionTargetRef{
					Kind: factoryapi.FactorySessionTargetRefKindNamed,
					Name: stringPointerForAPITest("beta"),
				},
			},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions", bytes.NewBufferString(`{"folderPath":"/workspace/fleet","target":{"kind":"named","name":"beta"}}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-sessions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(mf.OpenedFactorySessions) != 1 {
		t.Fatalf("opened factory sessions = %d, want 1", len(mf.OpenedFactorySessions))
	}
	if mf.OpenedFactorySessions[0].FolderPath != "/workspace/fleet" {
		t.Fatalf("opened session folder = %q, want /workspace/fleet", mf.OpenedFactorySessions[0].FolderPath)
	}
	if mf.OpenedFactorySessions[0].Target == nil ||
		mf.OpenedFactorySessions[0].Target.Kind != factoryapi.FactorySessionTargetRefKindNamed ||
		mf.OpenedFactorySessions[0].Target.Name == nil ||
		*mf.OpenedFactorySessions[0].Target.Name != "beta" {
		t.Fatalf("opened session target = %#v, want named beta", mf.OpenedFactorySessions[0].Target)
	}
	var response factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode open factory session response: %v", err)
	}
	if response.Session == nil || response.Session.Id != "session-beta" {
		t.Fatalf("open factory session response = %#v, want session-beta", response)
	}
}

func TestFactorySessionsAPI_OpenFactorySession_ValidationTargets(t *testing.T) {
	mf := &testutil.MockFactory{
		OpenFactorySessionErr: apiTestSessionValidationError{
			message: "folder validation failed",
			targets: []factoryapi.FactoryValidationTarget{
				factoryvalidation.FactorySessionFieldTarget("missing", "folderPath", "folder validation failed"),
			},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions", bytes.NewBufferString(`{"folderPath":"/workspace/missing","validateOnly":true}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /factory-sessions validation status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	var response factoryapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode open factory session error response: %v", err)
	}
	if response.Code != factoryapi.BADREQUEST {
		t.Fatalf("open factory session error code = %q, want BAD_REQUEST", response.Code)
	}
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("open factory session error targets = %#v, want one target", response.Targets)
	}
	target := (*response.Targets)[0]
	if target.Code != "factory.session.field.missing" ||
		target.Subject.Type != factoryapi.FactoryValidationSubjectTypeFactory ||
		target.Subject.Id != "folderPath" ||
		target.Subject.Location != factoryapi.FactoryValidationSubjectLocationReference {
		t.Fatalf("open factory session error target = %#v, want structured folder validation target", target)
	}
}

func TestFactorySessionsAPI_CloseFactorySession(t *testing.T) {
	mf := &testutil.MockFactory{}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodDelete, "/factory-sessions/session-beta", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /factory-sessions/session-beta status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(mf.ClosedFactorySessions) != 1 || mf.ClosedFactorySessions[0] != "session-beta" {
		t.Fatalf("closed factory sessions = %#v, want [session-beta]", mf.ClosedFactorySessions)
	}
}

type apiTestSessionValidationError struct {
	message string
	targets []factoryapi.FactoryValidationTarget
}

func (e apiTestSessionValidationError) Error() string {
	return e.message
}

func (e apiTestSessionValidationError) ErrorTargets() []factoryapi.FactoryValidationTarget {
	return e.targets
}

func TestFactorySessionsAPI_CloseFactorySession_NotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		CloseFactorySessionErr: apisurface.ErrFactorySessionNotFound,
	})

	req := httptest.NewRequest(http.MethodDelete, "/factory-sessions/missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestGetCurrentFactoryBySessionId_ReturnsSessionDefinitionAndVersion(t *testing.T) {
	defaultVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 1).UTC(), Logical: 1}
	sessionVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 2).UTC(), Logical: 2}
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {
				CurrentFactory: &factoryapi.Factory{Name: "alpha"},
				FactoryVersion: defaultVersion,
			},
			"session-2": {
				CurrentFactory: &factoryapi.Factory{Name: "beta"},
				FactoryVersion: sessionVersion,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-2/factory", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET factory status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.Factory
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode factory response: %v", err)
	}
	if response.Name != "beta" || response.Version == nil || *response.Version != sessionVersion {
		t.Fatalf("factory response = %#v, want beta/%#v", response, sessionVersion)
	}
}

func TestSaveCurrentFactoryBySessionId_SubmitsToTargetedSessionOnly(t *testing.T) {
	defaultVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 1).UTC(), Logical: 1}
	sessionVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 2).UTC(), Logical: 2}
	defaultFactory := &testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: "alpha"},
		FactoryVersion: defaultVersion,
	}
	sessionFactory := &testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: "beta"},
		FactoryVersion: sessionVersion,
	}
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default":  defaultFactory,
			"session-2": sessionFactory,
		},
	})

	body := saveFactoryForSessionRequestBody(`{"name":"beta","version":{"physical":"1970-01-01T00:00:00.000000002Z","logical":2},"workTypes":[],"workstations":[],"workers":[]}`)
	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/session-2/factory", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT factory status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(defaultFactory.SavedCurrentFactories) != 0 {
		t.Fatalf("default session save count = %d, want 0", len(defaultFactory.SavedCurrentFactories))
	}
	if len(sessionFactory.SavedCurrentFactories) != 1 {
		t.Fatalf("session save count = %d, want 1", len(sessionFactory.SavedCurrentFactories))
	}
	saved := sessionFactory.SavedCurrentFactories[0]
	if saved.Name != "beta" {
		t.Fatalf("saved factory = %#v, want beta definition", saved)
	}
}

func TestCurrentFactoryBySessionId_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {},
		},
	})

	getReq := httptest.NewRequest(http.MethodGet, "/factory-sessions/missing-session/factory", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	assertJSONError(t, getRec, http.StatusNotFound, "NOT_FOUND", "factory session not found")

	putReq := httptest.NewRequest(http.MethodPut, "/factory-sessions/missing-session/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRec, putReq)
	assertJSONError(t, putRec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestSubmitWorkThenListWork_ConfirmsObservedJSONFields(t *testing.T) {
	now := time.Date(2026, 4, 12, 16, 30, 0, 0, time.UTC)
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	submitted := assertSubmitThenListWorkRequest(t, srv, mf)
	assertSubmitThenListWorkListing(t, srv, mf, submitted, now)
}

func assertSubmitThenListWorkRequest(t *testing.T, srv *Server, mf *testutil.MockFactory) interfaces.SubmitRequest {
	t.Helper()

	rec := submitWorkRequest(t, srv, `{"name":"Inventory story","workTypeName":"task","traceId":"trace-inventory-1","payload":{"title":"Document current API"},"tags":{"branch":"api-standardization"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /work status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.SubmitWorkResponse](t, rec)
	if resp.TraceId != "trace-inventory-1" || len(mf.Submitted) != 1 {
		t.Fatalf("submit response = %#v submitted=%#v", resp, mf.Submitted)
	}
	submitted := mf.Submitted[0]
	if submitted.Name != "Inventory story" || submitted.WorkTypeID != "task" || submitted.TraceID != "trace-inventory-1" {
		t.Fatalf("submitted request = %#v, want name/work type/trace from JSON body", submitted)
	}
	return submitted
}

func TestSubmitWork_OmitsUnsetOptionalBoundaryFields(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"Inventory story","workTypeName":"task","payload":{"title":"Document current API"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /work status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("submitted requests = %d, want 1", len(mf.Submitted))
	}

	submitted := mf.Submitted[0]
	if submitted.Name != "Inventory story" || submitted.WorkTypeID != "task" {
		t.Fatalf("submitted request = %#v, want canonical required fields", submitted)
	}
	if submitted.TraceID == "" || submitted.CurrentChainingTraceID == "" {
		t.Fatalf("submitted chaining identifiers = %#v, want server-owned defaults when optionals are omitted", submitted)
	}
	if submitted.Relations != nil {
		t.Fatalf("submitted relations = %#v, want nil when omitted", submitted.Relations)
	}
}

func assertSubmitThenListWorkListing(t *testing.T, srv *Server, mf *testutil.MockFactory, submitted interfaces.SubmitRequest, now time.Time) {
	t.Helper()

	mf.Marking.Tokens["tok-inventory-1"] = &interfaces.Token{
		ID:      "tok-inventory-1",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			Name:       submitted.Name,
			WorkID:     "work-inventory-1",
			WorkTypeID: submitted.WorkTypeID,
			TraceID:    submitted.TraceID,
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: "Inspect this"},
				{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/inventory.png"},
			},
			Tags: submitted.Tags,
		},
		CreatedAt: now,
		EnteredAt: now,
	}
	mf.Net = &state.Net{
		Places:    map[string]*petri.Place{"task:init": {ID: "task:init", TypeID: "task", State: "init"}},
		WorkTypes: map[string]*state.WorkType{"task": {ID: "task", States: []state.StateDefinition{{Value: "init", Category: state.StateCategoryInitial}}}},
	}

	req := httptest.NewRequest(http.MethodGet, "/work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /work status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	listResp := decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
	if len(listResp.Results) != 1 {
		t.Fatalf("work result count = %d, want 1", len(listResp.Results))
	}
	work := listResp.Results[0]
	if work.Name != "Inventory story" || stringValue(work.WorkId) != "work-inventory-1" || stringValue(work.WorkTypeName) != "task" || stringValue(work.TraceId) != "trace-inventory-1" {
		t.Fatalf("work = %#v, want canonical identity fields", work)
	}
	if work.State == nil || work.State.Name != "init" || work.State.Type != factoryapi.WorkStateTypeINITIAL {
		t.Fatalf("state = %#v, want init/INITIAL", work.State)
	}
	assertGeneratedWorkContentParts(t, work.Content, []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "Inspect this"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/inventory.png"},
	})
	if work.Tags == nil || (*work.Tags)["branch"] != "api-standardization" {
		t.Fatalf("tags = %#v, want branch api-standardization", work.Tags)
	}
}

func TestGetWork(t *testing.T) {
	now := time.Now()
	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-prd-1": {
				ID:      "tok-prd-1",
				PlaceID: "prd:init",
				Color: interfaces.TokenColor{
					WorkID:                   "work-prd-1",
					WorkTypeID:               "prd",
					ChainingTraceDepth:       4,
					CurrentChainingTraceID:   "chain-1",
					PreviousChainingTraceIDs: []string{"chain-a", "chain-b"},
					TraceID:                  "trace-1",
					Content:                  []interfaces.WorkContentPart{{Type: interfaces.WorkContentPartTypeText, Text: "Review screenshot"}, {Type: interfaces.WorkContentPartTypeImage, File: "fixtures/review.png"}},
				},
				CreatedAt: now,
				EnteredAt: now,
				History: interfaces.TokenHistory{
					TotalVisits:         map[string]int{"execute": 1},
					ConsecutiveFailures: make(map[string]int),
					PlaceVisits:         map[string]int{"prd:init": 1},
				},
			},
		}},
	})

	req := httptest.NewRequest("GET", "/work/tok-prd-1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.Work](t, rec)
	if stringValue(resp.WorkId) != "work-prd-1" || stringValue(resp.WorkTypeName) != "prd" {
		t.Fatalf("work response = %#v, want work-prd-1 prd type", resp)
	}
	if resp.ChainingTraceDepth == nil || *resp.ChainingTraceDepth != 4 || resp.CurrentChainingTraceId == nil || *resp.CurrentChainingTraceId != "chain-1" || resp.PreviousChainingTraceIds == nil || len(*resp.PreviousChainingTraceIds) != 2 {
		t.Fatalf("chaining trace fields = %#v, want preserved trace lineage", resp)
	}
	assertGeneratedWorkContentParts(t, resp.Content, []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "Review screenshot"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/review.png"},
	})
}

func TestGetWork_ByWorkID(t *testing.T) {
	now := time.Now()
	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-prd-1": {
				ID:      "tok-prd-1",
				PlaceID: "prd:init",
				Color: interfaces.TokenColor{
					WorkID:     "work-prd-1",
					WorkTypeID: "prd",
					TraceID:    "trace-1",
				},
				CreatedAt: now,
				EnteredAt: now,
			},
		}},
	})

	req := httptest.NewRequest("GET", "/work/work-prd-1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.Work](t, rec)
	if stringValue(resp.WorkId) != "work-prd-1" {
		t.Fatalf("work response = %#v, want work-prd-1", resp)
	}
}

func TestGetWork_OmitsEmptyOptionalCollections(t *testing.T) {
	now := time.Now()
	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-prd-2": {
				ID:      "tok-prd-2",
				PlaceID: "prd:init",
				Color: interfaces.TokenColor{
					WorkID:     "work-prd-2",
					WorkTypeID: "prd",
					TraceID:    "trace-2",
				},
				CreatedAt: now,
				EnteredAt: now,
			},
		}},
	})

	req := httptest.NewRequest("GET", "/work/tok-prd-2", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.Work](t, rec)
	if resp.CurrentChainingTraceId == nil || *resp.CurrentChainingTraceId != "trace-2" {
		t.Fatalf("current chaining trace ID = %#v, want trace fallback", resp.CurrentChainingTraceId)
	}
	if resp.PreviousChainingTraceIds != nil || resp.Tags != nil {
		t.Fatalf("optional collections = %#v, want omitted empty fields", resp)
	}
}

func TestTokenToResponse_CopiesOptionalTagMap(t *testing.T) {
	token := &interfaces.Token{
		ID:      "tok-prd-copy",
		PlaceID: "prd:init",
		Color: interfaces.TokenColor{
			WorkID:     "work-prd-copy",
			WorkTypeID: "prd",
			TraceID:    "trace-copy",
			Tags: map[string]string{
				"branch": "stable",
			},
		},
	}

	resp := tokenToResponse(token, false)
	token.Color.Tags["branch"] = "mutated"
	token.Color.Tags["new"] = "late-tag"

	if resp.Tags == nil || (*resp.Tags)["branch"] != "stable" {
		t.Fatalf("response tags = %#v, want copied pre-mutation values", resp.Tags)
	}
	if _, ok := (*resp.Tags)["new"]; ok {
		t.Fatalf("response tags = %#v, want copied map to omit post-shaping additions", resp.Tags)
	}
}

func TestTokenToResponse_CopiesOptionalPreviousChainingTraceIDs(t *testing.T) {
	token := &interfaces.Token{
		ID:      "tok-prd-copy-slice",
		PlaceID: "prd:init",
		Color: interfaces.TokenColor{
			WorkID:                   "work-prd-copy-slice",
			WorkTypeID:               "prd",
			TraceID:                  "trace-copy-slice",
			CurrentChainingTraceID:   "chain-current",
			PreviousChainingTraceIDs: []string{"chain-parent"},
		},
	}

	resp := tokenToResponse(token, false)
	token.Color.PreviousChainingTraceIDs[0] = "chain-mutated"
	token.Color.PreviousChainingTraceIDs = append(token.Color.PreviousChainingTraceIDs, "chain-late")

	if resp.PreviousChainingTraceIds == nil || len(*resp.PreviousChainingTraceIds) != 1 || (*resp.PreviousChainingTraceIds)[0] != "chain-parent" {
		t.Fatalf("response previous chaining trace IDs = %#v, want copied pre-mutation values", resp.PreviousChainingTraceIds)
	}
}

func TestGetWorkNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}})
	req := httptest.NewRequest("GET", "/work/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "work not found")
}

func TestGetStatus_ReturnsAggregateSnapshotStatus(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
		Topology: &state.Net{
			Places: map[string]*petri.Place{
				"task:init":            {ID: "task:init", TypeID: "task", State: "init"},
				"task:review":          {ID: "task:review", TypeID: "task", State: "review"},
				"task:complete":        {ID: "task:complete", TypeID: "task", State: "complete"},
				"task:failed":          {ID: "task:failed", TypeID: "task", State: "failed"},
				"agent-slot:available": {ID: "agent-slot:available", TypeID: "agent-slot", State: "available"},
			},
			WorkTypes: map[string]*state.WorkType{"task": {ID: "task", States: []state.StateDefinition{{Value: "init", Category: state.StateCategoryInitial}, {Value: "review", Category: state.StateCategoryProcessing}, {Value: "complete", Category: state.StateCategoryTerminal}, {Value: "failed", Category: state.StateCategoryFailed}}}},
			Resources: map[string]*state.ResourceDef{"agent-slot": {ID: "agent-slot", Name: "agent-slot", Capacity: 2}},
		},
		Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-init":              {ID: "tok-init", PlaceID: "task:init", Color: interfaces.TokenColor{WorkID: "work-init", WorkTypeID: "task"}, CreatedAt: now, EnteredAt: now},
			"tok-review":            {ID: "tok-review", PlaceID: "task:review", Color: interfaces.TokenColor{WorkID: "work-review", WorkTypeID: "task"}, CreatedAt: now, EnteredAt: now},
			"tok-complete":          {ID: "tok-complete", PlaceID: "task:complete", Color: interfaces.TokenColor{WorkID: "work-complete", WorkTypeID: "task"}, CreatedAt: now, EnteredAt: now},
			"tok-failed":            {ID: "tok-failed", PlaceID: "task:failed", Color: interfaces.TokenColor{WorkID: "work-failed", WorkTypeID: "task"}, CreatedAt: now, EnteredAt: now},
			"agent-slot:resource:0": {ID: "agent-slot:resource:0", PlaceID: "agent-slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}, CreatedAt: now, EnteredAt: now},
			"tok-time":              {ID: "tok-time", PlaceID: interfaces.SystemTimePendingPlaceID, Color: interfaces.TokenColor{WorkID: "time-daily-refresh", WorkTypeID: interfaces.SystemTimeWorkTypeID, TraceID: "trace-time"}, CreatedAt: now, EnteredAt: now},
		}},
	}
	srv := newTestServer(&testutil.MockFactory{EngineState: snapshot})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /status status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.StatusResponse](t, rec)
	if resp.FactoryState != "RUNNING" || resp.RuntimeStatus != "ACTIVE" || resp.TotalTokens != 5 {
		t.Fatalf("status response = %#v, want RUNNING/ACTIVE with 5 tokens", resp)
	}
	if resp.Categories.Initial != 1 || resp.Categories.Processing != 1 || resp.Categories.Terminal != 1 || resp.Categories.Failed != 1 {
		t.Fatalf("categories = %#v, want one token in each category", resp.Categories)
	}
	if resp.Resources == nil || len(*resp.Resources) != 1 {
		t.Fatalf("resources = %#v, want one resource summary", resp.Resources)
	}
	resource := (*resp.Resources)[0]
	if resource.Name != "agent-slot" || resource.Available != 1 || resource.Total != 2 {
		t.Fatalf("resource = %#v, want agent-slot 1/2", resource)
	}
}

func TestListWork_HidesInternalTimeWorkTokens(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-story": {ID: "tok-story", PlaceID: "story:init", Color: interfaces.TokenColor{WorkID: "work-story", WorkTypeID: "story", TraceID: "trace-story"}, CreatedAt: now, EnteredAt: now},
		"tok-time":  {ID: "tok-time", PlaceID: interfaces.SystemTimePendingPlaceID, Color: interfaces.TokenColor{WorkID: "time-daily-refresh", WorkTypeID: interfaces.SystemTimeWorkTypeID, TraceID: "trace-time", Tags: map[string]string{interfaces.TimeWorkTagKeyCronWorkstation: "daily-refresh"}}, CreatedAt: now, EnteredAt: now},
	}}})

	req := httptest.NewRequest(http.MethodGet, "/work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /work status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
	if len(resp.Results) != 1 || stringValue(resp.Results[0].WorkId) != "work-story" {
		t.Fatalf("listed work = %#v, want only customer work", resp.Results)
	}
	if resp.PaginationContext == nil || stringValue(resp.PaginationContext.NextToken) != "" {
		t.Fatalf("pagination context = %#v, want metadata without next token after internal token filtering", resp.PaginationContext)
	}
}

func TestListWork_FiltersInternalTokensBeforePagination(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-filter-1": {ID: "tok-filter-1", PlaceID: "story:init", Color: interfaces.TokenColor{WorkID: "work-filter-1", WorkTypeID: "story", TraceID: "trace-filter-1"}, CreatedAt: now, EnteredAt: now},
		"tok-filter-2": {ID: "tok-filter-2", PlaceID: interfaces.SystemTimePendingPlaceID, Color: interfaces.TokenColor{WorkID: "time-daily-refresh", WorkTypeID: interfaces.SystemTimeWorkTypeID, TraceID: "trace-filter-2"}, CreatedAt: now, EnteredAt: now},
		"tok-filter-3": {ID: "tok-filter-3", PlaceID: "story:init", Color: interfaces.TokenColor{WorkID: "work-filter-3", WorkTypeID: "story", TraceID: "trace-filter-3"}, CreatedAt: now, EnteredAt: now},
		"tok-filter-4": {ID: "tok-filter-4", PlaceID: "story:init", Color: interfaces.TokenColor{WorkID: "work-filter-4", WorkTypeID: "story", TraceID: "trace-filter-4"}, CreatedAt: now, EnteredAt: now},
	}}})

	firstResp := decodeListWorkPage(t, srv, "/work?maxResults=2")
	if len(firstResp.Results) != 2 || stringValue(firstResp.Results[0].WorkId) != "work-filter-1" || stringValue(firstResp.Results[1].WorkId) != "work-filter-3" || firstResp.PaginationContext == nil {
		t.Fatalf("first page = %#v, want public work before pagination", firstResp)
	}
	nextToken := stringValue(firstResp.PaginationContext.NextToken)
	if nextToken == "" {
		t.Fatal("expected first page nextToken")
	}

	secondResp := decodeListWorkPage(t, srv, "/work?maxResults=2&nextToken="+nextToken)
	if len(secondResp.Results) != 1 || stringValue(secondResp.Results[0].WorkId) != "work-filter-4" {
		t.Fatalf("second page listed work = %#v, want remaining public work", secondResp.Results)
	}
	if secondResp.PaginationContext == nil || stringValue(secondResp.PaginationContext.NextToken) != "" {
		t.Fatalf("second page pagination context = %#v, want metadata without next token on final page", secondResp.PaginationContext)
	}
}

func TestGetWork_HidesInternalTimeWorkToken(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-time": {ID: "tok-time", PlaceID: interfaces.SystemTimePendingPlaceID, Color: interfaces.TokenColor{WorkID: "time-daily-refresh", WorkTypeID: interfaces.SystemTimeWorkTypeID, TraceID: "trace-time"}, CreatedAt: now, EnteredAt: now},
	}}})
	req := httptest.NewRequest(http.MethodGet, "/work/tok-time", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "work not found")
}

func TestListWork_HidesResourceTokens(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-story":           {ID: "tok-story", PlaceID: "story:init", Color: interfaces.TokenColor{WorkID: "work-story", WorkTypeID: "story", TraceID: "trace-story"}, CreatedAt: now, EnteredAt: now},
		"agent-slot:resource": {ID: "agent-slot:resource", PlaceID: "agent-slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource, WorkID: "resource-work", WorkTypeID: "agent-slot"}, CreatedAt: now, EnteredAt: now},
	}}})

	req := httptest.NewRequest(http.MethodGet, "/work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /work status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
	if len(resp.Results) != 1 || stringValue(resp.Results[0].WorkId) != "work-story" {
		t.Fatalf("listed work = %#v, want only non-resource work", resp.Results)
	}
}

func TestGetWork_HidesResourceToken(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"agent-slot:resource": {ID: "agent-slot:resource", PlaceID: "agent-slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource, WorkID: "resource-work", WorkTypeID: "agent-slot"}, CreatedAt: now, EnteredAt: now},
	}}})
	req := httptest.NewRequest(http.MethodGet, "/work/agent-slot:resource", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "work not found")
}

func TestListWork(t *testing.T) {
	tokens := makeListWorkTokens("prd", 3, time.Now())
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}})
	resp := decodeListWorkPage(t, srv, "/work?maxResults=2")
	if len(resp.Results) != 2 || resp.PaginationContext == nil || stringValue(resp.PaginationContext.NextToken) == "" {
		t.Fatalf("list work response = %#v, want paginated first page", resp)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this list-work contract test keeps the relation, pagination, and status assertions together to preserve route-level intent.
func TestListWork_ReturnsRuntimeRelationsWithSourceToTargetDirection(t *testing.T) {
	now := time.Now()
	tokens := map[string]*interfaces.Token{
		"tok-1": listWorkToken("tok-1", "work-review", "task:init", "task", now),
		"tok-2": listWorkToken("tok-2", "work-draft", "task:init", "task", now),
		"tok-3": listWorkToken("tok-3", "work-parent", "task:init", "task", now),
		"tok-4": listWorkToken("tok-4", "work-standalone", "task:init", "task", now),
		"tok-5": listWorkToken("tok-5", "work-origin", "task:init", "task", now),
	}
	tokens["tok-1"].Color.Name = "review"
	tokens["tok-1"].Color.Relations = []interfaces.Relation{{Type: interfaces.RelationDependsOn, TargetWorkID: "work-draft", RequiredState: "complete"}, {Type: interfaces.RelationParentChild, TargetWorkID: "work-parent"}, {Type: interfaces.RelationSpawnedBy, TargetWorkID: "work-origin"}}
	tokens["tok-2"].Color.Name = "draft"
	tokens["tok-3"].Color.Name = "parent"
	tokens["tok-4"].Color.Name = "standalone"
	tokens["tok-5"].Color.Name = "origin"

	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}, Net: listWorkFilterTopology()})
	resp := decodeListWorkPage(t, srv, "/work?state.name=init")
	review := listedWorkByID(t, resp.Results, "work-review")
	if review.Relations == nil || len(*review.Relations) != 3 {
		t.Fatalf("review relations = %#v, want runtime relations", review.Relations)
	}
	relations := *review.Relations
	if got := relations[0]; got.Type != factoryapi.RelationTypeDependsOn || got.SourceWorkName != "review" || got.TargetWorkName != "draft" || stringValue(got.TargetWorkId) != "work-draft" || stringValue(got.RequiredState) != "complete" {
		t.Fatalf("depends_on relation = %#v, want review -> draft complete", got)
	}
	if got := relations[1]; got.Type != factoryapi.RelationTypeParentChild || got.SourceWorkName != "review" || got.TargetWorkName != "parent" || stringValue(got.TargetWorkId) != "work-parent" || got.RequiredState != nil {
		t.Fatalf("parent_child relation = %#v, want review -> parent without required state", got)
	}
	if got := relations[2]; got.Type != factoryapi.RelationTypeSpawnedBy || got.SourceWorkName != "review" || got.TargetWorkName != "origin" || stringValue(got.TargetWorkId) != "work-origin" || got.RequiredState != nil {
		t.Fatalf("spawned_by relation = %#v, want review -> origin without required state", got)
	}
	if standalone := listedWorkByID(t, resp.Results, "work-standalone"); standalone.Relations != nil {
		t.Fatalf("standalone relations = %#v, want omitted relations", *standalone.Relations)
	}
}

func TestListWork_FiltersByWorkTypeNameNameSubstringAndTraceId(t *testing.T) {
	now := time.Now()
	tokens := map[string]*interfaces.Token{
		"tok-1": listWorkTokenWithTraces("tok-1", "work-story", "Review PRD", "task:review", "story", "trace-root", "", now),
		"tok-2": listWorkTokenWithTraces("tok-2", "work-bug", "Fix bug", "task:init", "bug", "", "trace-chain-1", now),
		"tok-3": listWorkTokenWithTraces("tok-3", "work-plan", "Plan feature", "task:init", "story", "trace-plan", "", now),
	}
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}, Net: listWorkFilterTopology()})
	for _, tc := range []struct {
		name        string
		query       string
		wantWorkIDs []string
	}{
		{name: "work type name", query: "workTypeName=story", wantWorkIDs: []string{"work-plan", "work-story"}},
		{name: "name substring", query: "name=prd", wantWorkIDs: []string{"work-story"}},
		{name: "trace id on current chaining trace", query: "traceId=trace-chain-1", wantWorkIDs: []string{"work-bug"}},
		{name: "trace id on trace id", query: "traceId=trace-plan", wantWorkIDs: []string{"work-plan"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := decodeListWorkPage(t, srv, "/work?"+tc.query)
			if len(resp.Results) != len(tc.wantWorkIDs) {
				t.Fatalf("results = %d, want %d: %#v", len(resp.Results), len(tc.wantWorkIDs), resp.Results)
			}
			gotIDs := make([]string, len(resp.Results))
			for i, work := range resp.Results {
				gotIDs[i] = stringValue(work.WorkId)
			}
			for i, wantWorkID := range tc.wantWorkIDs {
				if gotIDs[i] != wantWorkID {
					t.Fatalf("result[%d] workId = %q, want %q (all=%v)", i, gotIDs[i], wantWorkID, gotIDs)
				}
			}
		})
	}
}

func TestListWork_FiltersByNameBeforePagination(t *testing.T) {
	now := time.Now()
	tokens := map[string]*interfaces.Token{
		"tok-1": listWorkTokenWithTraces("tok-1", "work-alpha", "Alpha one", "task:init", "task", "", "", now),
		"tok-2": listWorkTokenWithTraces("tok-2", "work-beta", "Other item", "task:init", "task", "", "", now),
		"tok-3": listWorkTokenWithTraces("tok-3", "work-gamma", "Alpha two", "task:init", "task", "", "", now),
	}
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}, Net: listWorkFilterTopology()})
	resp := decodeListWorkPage(t, srv, "/work?name=alpha&maxResults=2")
	assertListedWorkIDs(t, resp.Results, []string{"work-alpha", "work-gamma"})
	if resp.PaginationContext == nil || stringValue(resp.PaginationContext.NextToken) != "" {
		t.Fatalf("pagination = %#v, want terminal page after name filter", resp.PaginationContext)
	}
}

func TestListWork_FiltersByStateNameAndType(t *testing.T) {
	now := time.Now()
	tokens := map[string]*interfaces.Token{
		"tok-1": listWorkToken("tok-1", "work-init", "task:init", "task", now),
		"tok-2": listWorkToken("tok-2", "work-review", "task:review", "task", now),
		"tok-3": listWorkToken("tok-3", "work-failed", "task:failed", "task", now),
	}
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}, Net: listWorkFilterTopology()})
	for _, tc := range []struct {
		name        string
		query       string
		wantWorkIDs []string
	}{
		{name: "state name", query: "state.name=review", wantWorkIDs: []string{"work-review"}},
		{name: "state type", query: "state.type=PROCESSING", wantWorkIDs: []string{"work-review"}},
		{name: "combined", query: "state.name=review&state.type=PROCESSING", wantWorkIDs: []string{"work-review"}},
		{name: "combined mismatch", query: "state.name=review&state.type=FAILED", wantWorkIDs: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := decodeListWorkPage(t, srv, "/work?"+tc.query)
			if len(resp.Results) != len(tc.wantWorkIDs) {
				t.Fatalf("results = %d, want %d: %#v", len(resp.Results), len(tc.wantWorkIDs), resp.Results)
			}
			for i, wantWorkID := range tc.wantWorkIDs {
				if got := stringValue(resp.Results[i].WorkId); got != wantWorkID || resp.Results[i].State == nil {
					t.Fatalf("result[%d] = %#v, want work %q with state", i, resp.Results[i], wantWorkID)
				}
			}
		})
	}
}

func TestListWork_DefaultOrderingSurfacesActiveWorkBeforeTerminalWork(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-1": listWorkToken("tok-1", "work-complete", "task:complete", "task", time.Now()),
		"tok-2": listWorkToken("tok-2", "work-failed", "task:failed", "task", time.Now()),
		"tok-3": listWorkToken("tok-3", "work-review", "task:review", "task", time.Now()),
		"tok-4": listWorkToken("tok-4", "work-init", "task:init", "task", time.Now()),
	}}, Net: listWorkFilterTopology()})
	resp := decodeListWorkPage(t, srv, "/work")
	assertListedWorkIDs(t, resp.Results, []string{"work-init", "work-review", "work-failed", "work-complete"})
}

func TestListWork_SortsByStateType(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-1": listWorkToken("tok-1", "work-complete", "task:complete", "task", time.Now()),
		"tok-2": listWorkToken("tok-2", "work-failed", "task:failed", "task", time.Now()),
		"tok-3": listWorkToken("tok-3", "work-review", "task:review", "task", time.Now()),
		"tok-4": listWorkToken("tok-4", "work-init", "task:init", "task", time.Now()),
	}}, Net: listWorkFilterTopology()})
	resp := decodeListWorkPage(t, srv, "/work?sortBy=state.type")
	assertListedWorkIDs(t, resp.Results, []string{"work-failed", "work-init", "work-review", "work-complete"})
}

func TestListWork_InvalidStateTypeReturnsBadRequest(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodGet, "/work?state.type=UNKNOWN", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED")
}

func TestListWork_InvalidSortByReturnsBadRequest(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodGet, "/work?sortBy=name", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "sortBy must be state.type")
}

func TestListWork_InvalidMaxResultsUsesGeneratedBadRequest(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	for _, tc := range []struct{ name, path string }{{"empty", "/work?maxResults="}, {"invalid", "/work?maxResults=abc"}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request parameter")
		})
	}
}

func TestListWork_NonPositiveMaxResultsDefaultsToCurrentBehavior(t *testing.T) {
	tokens := makeListWorkTokens("legacy", 3, time.Now())
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}})
	for _, tc := range []struct{ name, path string }{{"absent", "/work"}, {"non_positive", "/work?maxResults=0"}} {
		t.Run(tc.name, func(t *testing.T) {
			resp := decodeListWorkPage(t, srv, tc.path)
			if len(resp.Results) != len(tokens) {
				t.Fatalf("expected defaulted response with %d results, got %d", len(tokens), len(resp.Results))
			}
			if resp.PaginationContext == nil || resp.PaginationContext.MaxResults != defaultMaxResults || stringValue(resp.PaginationContext.NextToken) != "" {
				t.Fatalf("expected pagination context with maxResults %d and no next token, got %#v", defaultMaxResults, resp.PaginationContext)
			}
		})
	}
}

func TestListWork_NextTokenContinuesPublicRoutePagination(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: makeListWorkTokens("cursor", 3, time.Now())}})
	firstResp := decodeListWorkPage(t, srv, "/work?maxResults=2")
	if len(firstResp.Results) != 2 || firstResp.PaginationContext == nil {
		t.Fatalf("first page = %#v, want paginated response", firstResp)
	}
	nextToken := stringValue(firstResp.PaginationContext.NextToken)
	if nextToken == "" {
		t.Fatal("expected first page nextToken")
	}
	secondResp := decodeListWorkPage(t, srv, "/work?maxResults=2&nextToken="+nextToken)
	if len(secondResp.Results) != 1 || secondResp.PaginationContext == nil || secondResp.PaginationContext.MaxResults != 2 || stringValue(secondResp.PaginationContext.NextToken) != "" {
		t.Fatalf("second page = %#v, want one result and terminal pagination context", secondResp)
	}
	if stringValue(firstResp.Results[0].WorkId) != "work-cursor-1" || stringValue(firstResp.Results[1].WorkId) != "work-cursor-2" || stringValue(secondResp.Results[0].WorkId) != "work-cursor-3" {
		t.Fatalf("unexpected continued page results: first=%#v second=%#v", firstResp.Results, secondResp.Results)
	}
	trailingResp := decodeListWorkPage(t, srv, "/work?maxResults=2&nextToken="+encodeNextToken("tok-cursor-3"))
	if len(trailingResp.Results) != 0 || trailingResp.PaginationContext == nil || trailingResp.PaginationContext.MaxResults != 2 || stringValue(trailingResp.PaginationContext.NextToken) != "" {
		t.Fatalf("trailing page = %#v, want empty final page", trailingResp)
	}
}

func decodeListWorkPage(t *testing.T, srv *Server, path string) factoryapi.ListWorkResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200: %s", path, rec.Code, rec.Body.String())
	}
	return decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
}

func assertListedWorkIDs(t *testing.T, works []factoryapi.Work, want []string) {
	t.Helper()
	if len(works) != len(want) {
		t.Fatalf("results = %d, want %d: %#v", len(works), len(want), works)
	}
	for i, wantWorkID := range want {
		if got := stringValue(works[i].WorkId); got != wantWorkID {
			t.Fatalf("result[%d].workId = %q, want %q: %#v", i, got, wantWorkID, works)
		}
	}
}
