package errors_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	transporterrors "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/errors"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestRootErrorResponse_MapsSharedRuntimeSentinels(t *testing.T) {
	t.Parallel()

	operations := []transporterrors.Operation{
		transporterrors.OperationObserve,
		transporterrors.OperationControl,
		transporterrors.OperationMoveWork,
		transporterrors.OperationDispatchPlan,
	}
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   factoryapi.ErrorResponseCode
		wantMsg    string
	}{
		{
			name:       "not running",
			err:        factoryruntime.ErrNotRunning,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   factoryapi.ErrorResponseCode("SERVICE_UNAVAILABLE"),
			wantMsg:    "factory runtime is not running",
		},
		{
			name:       "not found",
			err:        factoryruntime.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   factoryapi.ErrorResponseCodeNOTFOUND,
			wantMsg:    "factory runtime target not found",
		},
	}

	for _, operation := range operations {
		for _, tc := range cases {
			tc := tc
			operation := operation
			t.Run(fmt.Sprintf("%s/%s", operationName(operation), tc.name), func(t *testing.T) {
				t.Parallel()
				status, response, ok := transporterrors.RootErrorResponse(tc.err, operation)
				if !ok {
					t.Fatalf("transporterrors.RootErrorResponse(%v, %s) = not handled", tc.err, operationName(operation))
				}
				if status != tc.wantStatus || response.Code != tc.wantCode || response.Message != tc.wantMsg {
					t.Fatalf("transporterrors.RootErrorResponse(%v, %s) = %d %#v, want %d code=%s msg=%q",
						tc.err, operationName(operation), status, response, tc.wantStatus, tc.wantCode, tc.wantMsg)
				}
			})
		}
	}
}

func TestRootErrorResponse_MapsObservationFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   factoryapi.ErrorResponseCode
		wantMsg    string
	}{
		{
			name:       "session not found",
			err:        factorysessions.ErrSessionNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   factoryapi.ErrorResponseCodeNOTFOUND,
			wantMsg:    "factory session not found",
		},
		{
			name:       "invalid observation scope",
			err:        factoryruntime.ErrInvalidObservationScope,
			wantStatus: http.StatusBadRequest,
			wantCode:   factoryapi.ErrorResponseCodeBADREQUEST,
			wantMsg:    "invalid observation scope",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, response, ok := transporterrors.RootErrorResponse(tc.err, transporterrors.OperationObserve)
			if !ok {
				t.Fatalf("transporterrors.RootErrorResponse(%v) = not handled", tc.err)
			}
			if status != tc.wantStatus || response.Code != tc.wantCode || response.Message != tc.wantMsg {
				t.Fatalf("transporterrors.RootErrorResponse(%v) = %d %#v, want %d code=%s msg=%q",
					tc.err, status, response, tc.wantStatus, tc.wantCode, tc.wantMsg)
			}
		})
	}
}

func TestRootErrorResponse_MapsControlFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   factoryapi.ErrorResponseCode
		wantMsg    string
	}{
		{
			name:       "already stopped",
			err:        factoryruntime.ErrAlreadyStopped,
			wantStatus: http.StatusConflict,
			wantCode:   factoryapi.ErrorResponseCode("CONFLICT"),
			wantMsg:    "factory runtime is already stopped",
		},
		{
			name:       "invalid lifecycle transition",
			err:        factoryruntime.ErrInvalidLifecycleTransition,
			wantStatus: http.StatusBadRequest,
			wantCode:   factoryapi.ErrorResponseCodeBADREQUEST,
			wantMsg:    "factory runtime invalid lifecycle transition",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, response, ok := transporterrors.RootErrorResponse(tc.err, transporterrors.OperationControl)
			if !ok {
				t.Fatalf("transporterrors.RootErrorResponse(%v) = not handled", tc.err)
			}
			if status != tc.wantStatus || response.Code != tc.wantCode || response.Message != tc.wantMsg {
				t.Fatalf("transporterrors.RootErrorResponse(%v) = %d %#v", tc.err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsMoveWorkFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   factoryapi.ErrorResponseCode
		wantMsg    string
	}{
		{
			name:       "work not found",
			err:        factoryruntime.ErrMoveWorkNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   factoryapi.ErrorResponseCodeNOTFOUND,
			wantMsg:    "work not found",
		},
		{
			name:       "invalid state",
			err:        factoryruntime.ErrMoveWorkInvalidState,
			wantStatus: http.StatusBadRequest,
			wantCode:   factoryapi.ErrorResponseCodeBADREQUEST,
			wantMsg:    "invalid target state for work type",
		},
		{
			name:       "in flight dispatch",
			err:        factoryruntime.ErrMoveWorkInFlightDispatch,
			wantStatus: http.StatusBadRequest,
			wantCode:   factoryapi.ErrorResponseCodeBADREQUEST,
			wantMsg:    "work is in an active dispatch",
		},
		{
			name:       "engine terminated",
			err:        factoryruntime.ErrMoveWorkEngineTerminated,
			wantStatus: http.StatusBadRequest,
			wantCode:   factoryapi.ErrorResponseCodeBADREQUEST,
			wantMsg:    "engine has terminated",
		},
		{
			name:       "request conflict",
			err:        factoryruntime.ErrMoveWorkRequestConflict,
			wantStatus: http.StatusConflict,
			wantCode:   factoryapi.ErrorResponseCodeMOVEWORKREQUESTALREADYAPPLIED,
			wantMsg:    "Operator move request was already applied.",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, response, ok := transporterrors.RootErrorResponse(tc.err, transporterrors.OperationMoveWork)
			if !ok {
				t.Fatalf("transporterrors.RootErrorResponse(%v) = not handled", tc.err)
			}
			if status != tc.wantStatus || response.Code != tc.wantCode || response.Message != tc.wantMsg {
				t.Fatalf("transporterrors.RootErrorResponse(%v) = %d %#v", tc.err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsDispatchPlanFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   factoryapi.ErrorResponseCode
		wantMsg    string
	}{
		{
			name:       "duplicate intent",
			err:        factoryruntime.ErrDuplicateDispatchIntent,
			wantStatus: http.StatusConflict,
			wantCode:   factoryapi.ErrorResponseCode("CONFLICT"),
			wantMsg:    "dispatch intent conflicts with an existing plan",
		},
		{
			name:       "unknown correlation",
			err:        factoryruntime.ErrUnknownDispatchCorrelation,
			wantStatus: http.StatusNotFound,
			wantCode:   factoryapi.ErrorResponseCodeNOTFOUND,
			wantMsg:    "dispatch correlation not found",
		},
		{
			name:       "invalid result boundary",
			err:        factoryruntime.ErrInvalidDispatchResultBoundary,
			wantStatus: http.StatusBadRequest,
			wantCode:   factoryapi.ErrorResponseCodeBADREQUEST,
			wantMsg:    "invalid dispatch result boundary",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, response, ok := transporterrors.RootErrorResponse(tc.err, transporterrors.OperationDispatchPlan)
			if !ok {
				t.Fatalf("transporterrors.RootErrorResponse(%v) = not handled", tc.err)
			}
			if status != tc.wantStatus || response.Code != tc.wantCode || response.Message != tc.wantMsg {
				t.Fatalf("transporterrors.RootErrorResponse(%v) = %d %#v", tc.err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_IgnoresCrossOperationTypedFailures(t *testing.T) {
	t.Parallel()

	if _, _, ok := transporterrors.RootErrorResponse(factoryruntime.ErrMoveWorkNotFound, transporterrors.OperationControl); ok {
		t.Fatal("move-work not found must not map through control operation")
	}
	if _, _, ok := transporterrors.RootErrorResponse(factoryruntime.ErrAlreadyStopped, transporterrors.OperationMoveWork); ok {
		t.Fatal("already stopped must not map through move-work operation")
	}
	if _, _, ok := transporterrors.RootErrorResponse(factoryruntime.ErrDuplicateDispatchIntent, transporterrors.OperationControl); ok {
		t.Fatal("duplicate dispatch intent must not map through control operation")
	}
	if _, _, ok := transporterrors.RootErrorResponse(apisurface.ErrFactorySessionNotFound, transporterrors.OperationControl); ok {
		t.Fatal("session not found must not map through control operation")
	}
}

func TestRootErrorResponse_ReturnsFalseForUnmappedFailures(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("pkg/services/factory_runtime/internal/service: boom")
	operations := []transporterrors.Operation{
		transporterrors.OperationObserve,
		transporterrors.OperationControl,
		transporterrors.OperationMoveWork,
		transporterrors.OperationDispatchPlan,
	}
	for _, operation := range operations {
		if _, _, ok := transporterrors.RootErrorResponse(err, operation); ok {
			t.Fatalf("unmapped failure %#v must not be handled for %s", err, operationName(operation))
		}
	}
}

func TestWriteRootOrInternalError_SanitizesUnmappedFailures(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	err := errors.New("pkg/services/factory_runtime/internal/service: boom")

	transporterrors.WriteRootOrInternalError(recorder, context.Background(), transporterrors.OperationObserve, "failed to observe factory runtime status", err)

	body := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError ||
		!strings.Contains(body, `"code":"INTERNAL_ERROR"`) ||
		!strings.Contains(body, `"family":"INTERNAL_SERVER_ERROR"`) ||
		strings.Contains(body, "pkg/services/factory_runtime") ||
		strings.Contains(body, "boom") {
		t.Fatalf("response = %d %s, want sanitized internal error", recorder.Code, body)
	}
}

func TestWriteRootOrInternalError_HandlesContextOutcomeCarriedByErr(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	transporterrors.WriteRootOrInternalError(recorder, context.Background(), transporterrors.OperationObserve, "failed to observe factory runtime status", context.DeadlineExceeded)

	body := recorder.Body.String()
	if recorder.Code != http.StatusGatewayTimeout ||
		!strings.Contains(body, `"message":"factory runtime request timed out"`) {
		t.Fatalf("response = %d %s, want gateway-timeout outcome for a context deadline carried by err", recorder.Code, body)
	}
}

func operationName(operation transporterrors.Operation) string {
	switch operation {
	case transporterrors.OperationObserve:
		return "observe"
	case transporterrors.OperationControl:
		return "control"
	case transporterrors.OperationMoveWork:
		return "move-work"
	case transporterrors.OperationDispatchPlan:
		return "dispatch-plan"
	default:
		return fmt.Sprintf("operation(%d)", operation)
	}
}
