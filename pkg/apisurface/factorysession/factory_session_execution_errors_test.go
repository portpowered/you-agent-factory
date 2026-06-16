package factorysession_test

import (
	"errors"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

func TestExecutionErrorResponse_MapsValidationAndConflictErrors(t *testing.T) {
	status, response, ok := factorysession.ExecutionErrorResponse(
		factorysessionexecution.NewValidationError("requestId", "requestId is required"),
	)
	if !ok {
		t.Fatal("ExecutionErrorResponse = false, want true")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if response.Code != factoryapi.BADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", response.Code)
	}

	status, response, ok = factorysession.ExecutionErrorResponse(
		factorysessionexecution.ErrExecutionRequestIDConflict,
	)
	if !ok {
		t.Fatal("ExecutionErrorResponse = false, want true")
	}
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Code != factoryapi.EXECUTIONREQUESTIDCONFLICT {
		t.Fatalf("code = %q, want EXECUTION_REQUEST_ID_CONFLICT", response.Code)
	}
}

func TestExecutionErrorResponse_MapsRequestValidationError(t *testing.T) {
	status, response, ok := factorysession.ExecutionErrorResponse(
		&apisurface.RequestValidationError{Message: "source.kind is invalid"},
	)
	if !ok {
		t.Fatal("ExecutionErrorResponse = false, want true")
	}
	if status != http.StatusBadRequest || response.Code != factoryapi.BADREQUEST {
		t.Fatalf("response = %#v, want 400 BAD_REQUEST", response)
	}
}

func TestExecutionErrorResponse_ReturnsFalseForUnknownErrors(t *testing.T) {
	if _, _, ok := factorysession.ExecutionErrorResponse(errors.New("other")); ok {
		t.Fatal("ExecutionErrorResponse = true, want false")
	}
}
