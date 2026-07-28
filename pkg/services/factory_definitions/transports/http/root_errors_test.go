package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionshttp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestValidateFactory_InvalidFactoryDefinitionPayloadReturnsTypedErrorResponse(t *testing.T) {
	t.Parallel()

	validation := &validationErrorFake{err: factorydefinitions.ErrInvalidFactoryDefinitionPayload}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Validation: validation},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		strings.NewReader(minimalValidationFactoryBody),
	)
	request.Header.Set("Content-Type", "application/json")

	handler.ValidateFactory(recorder, request)

	assertErrorResponse(
		t,
		recorder.Body.Bytes(),
		factoryapi.ErrorFamilyBadRequest,
		factoryapi.ErrorResponseCodeBADREQUEST,
		"invalid request payload",
	)
}

func TestValidateFactory_ValidationFailedReturnsInvalidFactoryWithTargets(t *testing.T) {
	t.Parallel()

	validation := &validationErrorFake{
		err: &factorydefinitions.FactoryDefinitionValidationFailure{
			Validation: factorydefinitions.ValidationResult{
				Targets: []factorydefinitions.ValidationTarget{{
					Code:     "factory.validation.stub",
					Severity: factorydefinitions.ValidationSeverityError,
					Message:  "stub validation finding",
					Path:     "workers[0].model",
					Subject: factorydefinitions.ValidationSubject{
						Type:     factorydefinitions.ValidationSubjectTypeWorker,
						ID:       "planner",
						Location: factorydefinitions.ValidationSubjectLocationDefinition,
					},
				}},
			},
		},
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Validation: validation},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		strings.NewReader(minimalValidationFactoryBody),
	)
	request.Header.Set("Content-Type", "application/json")

	handler.ValidateFactory(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}

	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != factoryapi.ErrorResponseCode("INVALID_FACTORY") {
		t.Fatalf("code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("family = %q, want bad_request", errResp.Family)
	}
	if errResp.Message != "Factory payload is not a valid Agent Factory definition." {
		t.Fatalf("message = %q, want invalid factory message", errResp.Message)
	}
	if errResp.Targets == nil || len(*errResp.Targets) != 1 {
		t.Fatalf("targets = %#v, want one encoded finding", errResp.Targets)
	}
	target := (*errResp.Targets)[0]
	if target.Code != "factory.validation.stub" {
		t.Fatalf("target code = %q, want factory.validation.stub", target.Code)
	}
}

func TestValidateFactory_OpaqueRootFailureDoesNotLeakInternalDetails(t *testing.T) {
	t.Parallel()

	const leakedPath = "/var/lib/factory/pkg/services/factory_definitions/internal/catalog"
	validation := &validationErrorFake{
		err: fmt.Errorf("load catalog: %s", leakedPath),
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Validation: validation},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		strings.NewReader(minimalValidationFactoryBody),
	)
	request.Header.Set("Content-Type", "application/json")

	handler.ValidateFactory(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), leakedPath) {
		t.Fatalf("response leaked internal path: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "factory_definitions/internal") {
		t.Fatalf("response leaked internal package path: %s", recorder.Body.String())
	}
}

func TestGetCurrentFactoryBySessionId_NotFoundReturnsTypedErrorResponse(t *testing.T) {
	t.Parallel()

	root := &capturingCurrentFactoryRootFake{
		getErr: factorydefinitions.ErrCurrentFactoryNotFound,
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Definitions: root},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-2/factory", nil)

	handler.GetCurrentFactoryBySessionId(recorder, request, "session-2")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", recorder.Code, recorder.Body.String())
	}
	assertErrorResponse(
		t,
		recorder.Body.Bytes(),
		factoryapi.ErrorFamilyNotFound,
		factoryapi.ErrorResponseCodeNOTFOUND,
		"Current factory not found.",
	)
}

func TestSaveCurrentFactoryBySessionId_AtomicWriteFailedReturnsInternalErrorWithoutLeakage(t *testing.T) {
	t.Parallel()

	const leakedPath = "/var/lib/factory/pkg/services/factory_definitions/internal/persistence"
	root := &capturingCurrentFactoryRootFake{
		saveErr: &factorydefinitions.AtomicFactoryWriteFailure{
			Name:  "alpha",
			Cause: fmt.Errorf("persist failed at %s", leakedPath),
		},
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Definitions: root},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPut,
		"/factory-sessions/session-2/factory",
		strings.NewReader(saveCurrentFactoryRequestBody(minimalValidationFactoryBody)),
	)
	request.Header.Set("Content-Type", "application/json")

	handler.SaveCurrentFactoryBySessionId(recorder, request, "session-2")

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", recorder.Code, recorder.Body.String())
	}
	assertErrorResponse(
		t,
		recorder.Body.Bytes(),
		factoryapi.ErrorFamilyInternalServerError,
		factoryapi.ErrorResponseCodeINTERNALERROR,
		"failed to save current factory",
	)
	if strings.Contains(recorder.Body.String(), leakedPath) {
		t.Fatalf("response leaked internal path: %s", recorder.Body.String())
	}
}

func TestDefinitionsRootErrorResponseReturnsFalseForNilError(t *testing.T) {
	t.Parallel()

	if status, response, ok := factorydefinitionshttp.DefinitionsRootErrorResponseForTest(nil); ok {
		t.Fatalf("definitionsRootErrorResponse(nil) = (%d, %#v, true), want false", status, response)
	}
}

func TestDefinitionsRootErrorResponseMapsWrappedInvalidPayload(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("decode: %w", factorydefinitions.ErrInvalidFactoryDefinitionPayload)
	status, response, ok := factorydefinitionshttp.DefinitionsRootErrorResponseForTest(wrapped)
	if !ok {
		t.Fatal("definitionsRootErrorResponse = false, want true")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok {
		t.Fatalf("response = %#v, want ErrorResponse", response)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", errResp.Code)
	}
}

func TestDefinitionsRootErrorResponseReturnsFalseForUnknownError(t *testing.T) {
	t.Parallel()

	if _, _, ok := factorydefinitionshttp.DefinitionsRootErrorResponseForTest(errors.New("opaque")); ok {
		t.Fatal("definitionsRootErrorResponse = true, want false")
	}
}

type validationErrorFake struct {
	invoked bool
	err     error
}

func (fake *validationErrorFake) ValidateSubmittedDefinition(
	_ context.Context,
	_ factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	fake.invoked = true
	return factorydefinitions.ValidationResult{}, fake.err
}

var _ factorydefinitions.SubmittedDefinitionValidationOperation = (*validationErrorFake)(nil)

func assertErrorResponse(
	t *testing.T,
	body []byte,
	wantFamily factoryapi.ErrorFamily,
	wantCode factoryapi.ErrorResponseCode,
	wantMessage string,
) {
	t.Helper()

	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Family != wantFamily {
		t.Fatalf("family = %q, want %q", errResp.Family, wantFamily)
	}
	if errResp.Code != wantCode {
		t.Fatalf("code = %q, want %q", errResp.Code, wantCode)
	}
	if errResp.Message != wantMessage {
		t.Fatalf("message = %q, want %q", errResp.Message, wantMessage)
	}
}
