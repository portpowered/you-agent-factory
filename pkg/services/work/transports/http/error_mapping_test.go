package http

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestRootErrorResponse_MapsNotFoundFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{name: "factory session", err: apisurface.ErrFactorySessionNotFound},
		{name: "work", err: work.ErrWorkNotFound},
		{name: "runtime move work", err: state.ErrMoveWorkNotFound},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			status, response, ok := RootErrorResponse(testCase.err)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed not found", testCase.err)
			}
			if status != http.StatusNotFound ||
				response.Family != factoryapi.ErrorFamilyNotFound ||
				response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want not found", testCase.err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsAlreadyAppliedMoveFailure(t *testing.T) {
	t.Parallel()

	status, response, ok := RootErrorResponse(work.ErrMoveWorkRequestAlreadyApplied)
	if !ok {
		t.Fatal("RootErrorResponse(ErrMoveWorkRequestAlreadyApplied) = not handled")
	}
	if status != http.StatusConflict ||
		response.Family != factoryapi.ErrorFamilyConflict ||
		response.Code != factoryapi.ErrorResponseCodeMOVEWORKREQUESTALREADYAPPLIED ||
		response.Message != "Operator move request was already applied." {
		t.Fatalf("RootErrorResponse = %d %#v, want already-applied move conflict", status, response)
	}
}

func TestRootErrorResponse_MapsMoveValidationFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err     error
		message string
	}{
		{err: state.ErrMoveWorkInvalidState, message: "invalid target state for work type"},
		{err: state.ErrMoveWorkInFlightDispatch, message: "work is in an active dispatch"},
		{err: state.ErrMoveWorkEngineTerminated, message: "engine has terminated"},
	}
	for _, testCase := range cases {
		t.Run(testCase.err.Error(), func(t *testing.T) {
			t.Parallel()
			status, response, ok := RootErrorResponse(testCase.err)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed bad request", testCase.err)
			}
			if status != http.StatusBadRequest ||
				response.Family != factoryapi.ErrorFamilyBadRequest ||
				response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
				response.Message != testCase.message {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want move validation bad request", testCase.err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsAdmissionFailures(t *testing.T) {
	t.Parallel()

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		status, response, ok := RootErrorResponse(work.ErrInvalidWorkRequest)
		if !ok || status != http.StatusBadRequest || response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
			t.Fatalf("RootErrorResponse(ErrInvalidWorkRequest) = %d %#v %v, want bad request", status, response, ok)
		}
	})

	t.Run("conflict", func(t *testing.T) {
		t.Parallel()
		status, response, ok := RootErrorResponse(work.ErrWorkRequestConflict)
		if !ok ||
			status != http.StatusConflict ||
			response.Family != factoryapi.ErrorFamilyConflict ||
			response.Code != factoryapi.ErrorResponseCode(workErrorCodeConflict) {
			t.Fatalf("RootErrorResponse(ErrWorkRequestConflict) = %d %#v %v, want conflict", status, response, ok)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		t.Parallel()
		status, response, ok := RootErrorResponse(work.ErrWorkRequestRejected)
		if !ok || status != http.StatusBadRequest || response.Message != "Work Request rejected" {
			t.Fatalf("RootErrorResponse(ErrWorkRequestRejected) = %d %#v %v, want rejected bad request", status, response, ok)
		}
	})
}

func TestRootErrorResponse_MapsValidationAndStagingFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := RootErrorResponse(&work.ValidationError{
		Field:   "sortBy",
		Message: "sortBy must be state.type",
	})
	if !ok || status != http.StatusBadRequest || response.Message != "sortBy must be state.type" {
		t.Fatalf("validation error = %d %#v %v, want bad request", status, response, ok)
	}

	status, response, ok = RootErrorResponse(&work.ContentStagingError{Message: "staging unavailable"})
	if !ok || status != http.StatusBadRequest || response.Message != "staging unavailable" {
		t.Fatalf("staging error = %d %#v %v, want bad request", status, response, ok)
	}
}

func TestRootErrorResponse_MapsWorkRequestValidationFailures(t *testing.T) {
	t.Parallel()

	err := errors.New(`work_request: works[0] ("draft") references unknown work type "missing"`)
	status, response, ok := RootErrorResponse(err)
	if !ok || status != http.StatusBadRequest || !strings.Contains(response.Message, "work type name") {
		t.Fatalf("RootErrorResponse(%v) = %d %#v %v, want work request bad request", err, status, response, ok)
	}
}

func TestRootErrorResponse_ReturnsFalseForNilAndUnmappedFailures(t *testing.T) {
	t.Parallel()

	if _, _, ok := RootErrorResponse(nil); ok {
		t.Fatal("RootErrorResponse(nil) = handled, want false")
	}

	err := fmt.Errorf("pkg/services/work/internal/service: boom")
	if _, _, ok := RootErrorResponse(err); ok {
		t.Fatalf("unmapped failure %#v must not be handled", err)
	}
}

func TestWriteRootOrInternalError_SanitizesUnmappedFailures(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{})
	recorder := httptest.NewRecorder()
	err := errors.New("pkg/services/work/internal/service: boom")

	adapter.writeRootOrInternalError(recorder, err, "failed to list Work")

	body := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError ||
		!strings.Contains(body, `"code":"INTERNAL_ERROR"`) ||
		!strings.Contains(body, `"family":"INTERNAL_SERVER_ERROR"`) ||
		!strings.Contains(body, `"message":"failed to list Work"`) ||
		strings.Contains(body, "pkg/services/work") ||
		strings.Contains(body, "boom") {
		t.Fatalf("response = %d %s, want sanitized internal error", recorder.Code, body)
	}
}

func TestTypedRootFailuresDoNotCollapseToInternalError(t *testing.T) {
	t.Parallel()

	cases := []error{
		work.ErrWorkNotFound,
		work.ErrMoveWorkRequestAlreadyApplied,
		work.ErrInvalidWorkRequest,
		work.ErrWorkRequestConflict,
		work.ErrWorkRequestRejected,
		state.ErrMoveWorkInvalidState,
		&work.ValidationError{Message: "invalid query"},
		&work.ContentStagingError{Message: "staging failed"},
	}
	for _, err := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			_, response, ok := RootErrorResponse(err)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled", err)
			}
			if response.Code == factoryapi.ErrorResponseCodeINTERNALERROR {
				t.Fatalf("RootErrorResponse(%v) = INTERNAL_ERROR, want typed mapping", err)
			}
		})
	}
}
