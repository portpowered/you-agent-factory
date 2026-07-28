package http_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingshttp "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestRootErrorResponse_MapsLoadValidationFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(
		operatorsettingshttp.ErrInvalidLoadPath,
	)
	if !ok {
		t.Fatal("RootErrorResponse = not handled, want typed bad request")
	}
	if status != http.StatusBadRequest ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
		response.Message != "invalid operator settings load request" {
		t.Fatalf("RootErrorResponse = %d %#v, want invalid load request", status, response)
	}
}

func TestRootErrorResponse_MapsUpdateValidationFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(
		operatorsettingshttp.ErrInvalidUpdatePath,
	)
	if !ok {
		t.Fatal("RootErrorResponse = not handled, want typed bad request")
	}
	if status != http.StatusBadRequest ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
		response.Message != "invalid operator settings update request" {
		t.Fatalf("RootErrorResponse = %d %#v, want invalid update request", status, response)
	}
}

func TestRootErrorResponse_MapsDocumentMalformedFailures(t *testing.T) {
	t.Parallel()

	cases := []error{
		operatorsettings.ErrDocumentMalformed,
		typedDocumentFailure(operatorsettings.DocumentFailureKindMalformed),
		fmt.Errorf("load: %w", operatorsettings.ErrDocumentMalformed),
	}
	for _, err := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			status, response, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(err)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed malformed", err)
			}
			if status != http.StatusBadRequest ||
				response.Family != factoryapi.ErrorFamilyBadRequest ||
				response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
				response.Message != "operator document is malformed" {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want malformed", err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsDocumentNotFoundFailures(t *testing.T) {
	t.Parallel()

	cases := []error{
		operatorsettings.ErrDocumentNotFound,
		typedDocumentFailure(operatorsettings.DocumentFailureKindNotFound),
	}
	for _, err := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			status, response, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(err)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed not found", err)
			}
			if status != http.StatusNotFound ||
				response.Family != factoryapi.ErrorFamilyNotFound ||
				response.Code != factoryapi.ErrorResponseCodeNOTFOUND ||
				response.Message != "operator document not found" {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want not found", err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsDocumentUnsupportedFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(
		typedDocumentFailure(operatorsettings.DocumentFailureKindUnsupported),
	)
	if !ok {
		t.Fatal("RootErrorResponse = not handled, want typed unsupported")
	}
	if status != http.StatusBadRequest ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Code != factoryapi.ErrorResponseCode("OPERATOR_DOCUMENT_UNSUPPORTED") ||
		response.Message != "operator document update is unsupported" {
		t.Fatalf("RootErrorResponse = %d %#v, want unsupported document", status, response)
	}
}

func TestRootErrorResponse_MapsDocumentConflictFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(
		typedDocumentFailure(operatorsettings.DocumentFailureKindConflict),
	)
	if !ok {
		t.Fatal("RootErrorResponse = not handled, want typed conflict")
	}
	if status != http.StatusConflict ||
		response.Family != factoryapi.ErrorFamilyConflict ||
		response.Code != factoryapi.ErrorResponseCode("OPERATOR_DOCUMENT_CONFLICT") ||
		response.Message != "operator document persist conflict" {
		t.Fatalf("RootErrorResponse = %d %#v, want document conflict", status, response)
	}
}

func TestRootErrorResponse_DistinguishesMalformedAndNotFoundOutcomes(t *testing.T) {
	t.Parallel()

	malformedStatus, malformedResponse, malformedOK := operatorsettingshttp.SettingsRootErrorResponseForTest(
		operatorsettings.ErrDocumentMalformed,
	)
	notFoundStatus, notFoundResponse, notFoundOK := operatorsettingshttp.SettingsRootErrorResponseForTest(
		operatorsettings.ErrDocumentNotFound,
	)
	if !malformedOK || !notFoundOK {
		t.Fatal("RootErrorResponse must map malformed and not-found failures")
	}
	if malformedStatus == notFoundStatus ||
		malformedResponse.Message == notFoundResponse.Message ||
		malformedResponse.Code == notFoundResponse.Code {
		t.Fatalf(
			"malformed (%d %#v) and not-found (%d %#v) must be distinguishable",
			malformedStatus,
			malformedResponse,
			notFoundStatus,
			notFoundResponse,
		)
	}
}

func TestRootErrorResponse_MapsResolutionInvalidInputFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(
		operatorsettings.ErrResolutionInvalidInput,
	)
	if !ok {
		t.Fatal("RootErrorResponse = not handled, want typed invalid input")
	}
	if status != http.StatusBadRequest ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
		response.Message != "operator effective resolution input is invalid" {
		t.Fatalf("RootErrorResponse = %d %#v, want invalid resolution input", status, response)
	}
}

func TestRootErrorResponse_MapsResolutionUnsupportedOverrideFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(
		typedResolutionFailure(operatorsettings.ResolutionFailureKindUnsupportedOverride),
	)
	if !ok {
		t.Fatal("RootErrorResponse = not handled, want typed unsupported override")
	}
	if status != http.StatusBadRequest ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Code != factoryapi.ErrorResponseCode("OPERATOR_RESOLUTION_UNSUPPORTED_OVERRIDE") ||
		response.Message != "operator effective resolution override is unsupported" {
		t.Fatalf("RootErrorResponse = %d %#v, want unsupported override", status, response)
	}
}

func TestRootErrorResponse_MapsResolutionConflictFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(
		typedResolutionFailure(operatorsettings.ResolutionFailureKindConflict),
	)
	if !ok {
		t.Fatal("RootErrorResponse = not handled, want typed resolution conflict")
	}
	if status != http.StatusConflict ||
		response.Family != factoryapi.ErrorFamilyConflict ||
		response.Code != factoryapi.ErrorResponseCode("OPERATOR_RESOLUTION_CONFLICT") ||
		response.Message != "operator effective resolution conflict" {
		t.Fatalf("RootErrorResponse = %d %#v, want resolution conflict", status, response)
	}
}

func TestRootErrorResponse_DistinguishesResolutionFailureOutcomes(t *testing.T) {
	t.Parallel()

	invalidStatus, invalidResponse, invalidOK := operatorsettingshttp.SettingsRootErrorResponseForTest(
		operatorsettings.ErrResolutionInvalidInput,
	)
	unsupportedStatus, unsupportedResponse, unsupportedOK := operatorsettingshttp.SettingsRootErrorResponseForTest(
		operatorsettings.ErrResolutionUnsupportedOverride,
	)
	conflictStatus, conflictResponse, conflictOK := operatorsettingshttp.SettingsRootErrorResponseForTest(
		operatorsettings.ErrResolutionConflict,
	)
	if !invalidOK || !unsupportedOK || !conflictOK {
		t.Fatal("RootErrorResponse must map all resolution failure kinds")
	}
	if invalidResponse.Message == unsupportedResponse.Message ||
		invalidResponse.Code == unsupportedResponse.Code {
		t.Fatalf(
			"invalid input (%d %#v) and unsupported override (%d %#v) must be distinguishable",
			invalidStatus,
			invalidResponse,
			unsupportedStatus,
			unsupportedResponse,
		)
	}
	if unsupportedResponse.Message == conflictResponse.Message ||
		unsupportedResponse.Code == conflictResponse.Code {
		t.Fatalf(
			"unsupported override (%d %#v) and conflict (%d %#v) must be distinguishable",
			unsupportedStatus,
			unsupportedResponse,
			conflictStatus,
			conflictResponse,
		)
	}
}

func TestRootErrorResponse_ReturnsFalseForNilAndUnmappedFailures(t *testing.T) {
	t.Parallel()

	if _, _, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(nil); ok {
		t.Fatal("RootErrorResponse(nil) = handled, want false")
	}

	err := errors.New("pkg/services/operator_settings/internal/service: boom")
	if _, _, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(err); ok {
		t.Fatalf("unmapped failure %#v must not be handled", err)
	}
}

func typedDocumentFailure(kind operatorsettings.DocumentFailureKind) error {
	return operatorsettings.DocumentFailure{
		Kind:    kind,
		Message: "fixture",
		Path:    "/tmp/config.json",
	}
}

func typedResolutionFailure(kind operatorsettings.ResolutionFailureKind) error {
	return operatorsettings.ResolutionFailure{
		Kind:    kind,
		Message: "fixture",
		Field:   "workerModelProvider",
	}
}
