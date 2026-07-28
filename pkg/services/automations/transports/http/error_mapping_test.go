package http_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationshttp "github.com/portpowered/infinite-you/pkg/services/automations/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestRootErrorResponse_MapsInvalidRequestFailures(t *testing.T) {
	t.Parallel()

	cases := []error{
		automations.ErrInvalidRequest,
		typedAutomationsError("Reconcile", automations.ErrorCodeInvalid, automations.ErrInvalidRequest),
		automationshttp.ErrInvalidSourceIdentity,
		automationshttp.ErrInvalidReconcileDesired,
	}
	for _, err := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			status, response, ok := automationshttp.AutomationsRootErrorResponseForTest(err)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed bad request", err)
			}
			if status != http.StatusBadRequest ||
				response.Family != factoryapi.ErrorFamilyBadRequest ||
				response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
				response.Message != "invalid automations request" {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want bad request", err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsNotFoundFailures(t *testing.T) {
	t.Parallel()

	cases := []error{
		automations.ErrNotFound,
		typedAutomationsError("SourceStatus", automations.ErrorCodeNotFound, automations.ErrNotFound),
	}
	for _, err := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			status, response, ok := automationshttp.AutomationsRootErrorResponseForTest(err)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed not found", err)
			}
			if status != http.StatusNotFound ||
				response.Family != factoryapi.ErrorFamilyNotFound ||
				response.Code != factoryapi.ErrorResponseCodeNOTFOUND ||
				response.Message != "automations resource not found" {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want not found", err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsConflictFailures(t *testing.T) {
	t.Parallel()

	cases := []error{
		automations.ErrConflict,
		typedAutomationsError("GetCursor", automations.ErrorCodeConflict, automations.ErrConflict),
	}
	for _, err := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			status, response, ok := automationshttp.AutomationsRootErrorResponseForTest(err)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed conflict", err)
			}
			if status != http.StatusConflict ||
				response.Family != factoryapi.ErrorFamilyConflict ||
				response.Code != factoryapi.ErrorResponseCode("CONFLICT") ||
				response.Message != "automations operation conflicted with observed state" {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want conflict", err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsNotReadyFailures(t *testing.T) {
	t.Parallel()

	cases := []error{
		automations.ErrNotReady,
		typedAutomationsError("StartSource", automations.ErrorCodeNotReady, automations.ErrNotReady),
	}
	for _, err := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			status, response, ok := automationshttp.AutomationsRootErrorResponseForTest(err)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed not ready", err)
			}
			if status != http.StatusServiceUnavailable ||
				response.Family != factoryapi.ErrorFamilyInternalServerError ||
				response.Code != factoryapi.ErrorResponseCode("SERVICE_UNAVAILABLE") ||
				response.Message != "automations service is not ready" {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want service unavailable", err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsSupervisionFailedFailures(t *testing.T) {
	t.Parallel()

	err := typedAutomationsError(
		"WaitSource",
		automations.ErrorCodeFailed,
		fmt.Errorf("%w: observed failed state", automations.ErrSupervisionFailed),
	)
	status, response, ok := automationshttp.AutomationsRootErrorResponseForTest(err)
	if !ok {
		t.Fatalf("RootErrorResponse(%v) = not handled, want typed supervision failure", err)
	}
	if status != http.StatusInternalServerError ||
		response.Family != factoryapi.ErrorFamilyInternalServerError ||
		response.Code != factoryapi.ErrorResponseCodeINTERNALERROR ||
		response.Message != "automation source supervision failed" {
		t.Fatalf("RootErrorResponse(%v) = %d %#v, want supervision failure", err, status, response)
	}
}

func TestRootErrorResponse_ReturnsFalseForNilAndUnmappedFailures(t *testing.T) {
	t.Parallel()

	if _, _, ok := automationshttp.AutomationsRootErrorResponseForTest(nil); ok {
		t.Fatal("RootErrorResponse(nil) = handled, want false")
	}

	err := errors.New("pkg/services/automations/internal/service: boom")
	if _, _, ok := automationshttp.AutomationsRootErrorResponseForTest(err); ok {
		t.Fatalf("unmapped failure %#v must not be handled", err)
	}
}

func TestWriteRootOrInternalError_SanitizesUnmappedFailures(t *testing.T) {
	t.Parallel()

	adapter := automationshttp.NewAdapterFromRoot(automationshttp.RootBinding{
		Automations: automations.Root{Operations: &rootFakeProbe{}},
	})
	recorder := httptest.NewRecorder()
	err := errors.New("pkg/services/automations/internal/service: boom")

	automationshttp.WriteRootOrInternalErrorForTest(adapter, recorder, err)

	body := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError ||
		!strings.Contains(body, `"code":"INTERNAL_ERROR"`) ||
		!strings.Contains(body, `"family":"INTERNAL_SERVER_ERROR"`) ||
		!strings.Contains(body, `"message":"automations request failed"`) ||
		strings.Contains(body, "pkg/services/automations") ||
		strings.Contains(body, "boom") {
		t.Fatalf("response = %d %s, want sanitized internal error", recorder.Code, body)
	}
}

type rootFakeProbe struct{}

func (rootFakeProbe) Reconcile(context.Context, automations.ReconcileRequest) (automations.ReconcileResult, error) {
	return automations.ReconcileResult{}, nil
}
func (rootFakeProbe) StartSource(context.Context, automations.StartSourceRequest) (automations.StartSourceResult, error) {
	return automations.StartSourceResult{}, nil
}
func (rootFakeProbe) StopSource(context.Context, automations.StopSourceRequest) (automations.StopSourceResult, error) {
	return automations.StopSourceResult{}, nil
}
func (rootFakeProbe) WaitSource(context.Context, automations.WaitSourceRequest) (automations.WaitSourceResult, error) {
	return automations.WaitSourceResult{}, nil
}
func (rootFakeProbe) SourceStatus(context.Context, automations.SourceStatusRequest) (automations.SourceStatusResult, error) {
	return automations.SourceStatusResult{}, nil
}
func (rootFakeProbe) GetStatus(context.Context, automations.GetStatusRequest) (automations.GetStatusResult, error) {
	return automations.GetStatusResult{}, nil
}
func (rootFakeProbe) GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error) {
	return automations.GetCursorResult{}, nil
}

func typedAutomationsError(op string, code automations.ErrorCode, err error) error {
	return &automations.Error{
		Op:   op,
		Code: code,
		Err:  err,
	}
}
