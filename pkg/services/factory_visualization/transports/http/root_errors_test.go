package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestVisualizationRootErrorResponseReturnsFalseForNilError(t *testing.T) {
	t.Parallel()

	if status, response, ok := factoryvisualizationhttp.VisualizationRootErrorResponseForTest(nil); ok {
		t.Fatalf("VisualizationRootErrorResponse(nil) = (%d, %#v, true), want false", status, response)
	}
}

func TestVisualizationRootErrorResponseMapsWrappedLifecycleMissingParameters(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("activate: %w", &factoryvisualization.LifecycleError{
		Kind:    factoryvisualization.LifecycleErrorMissingParameters,
		Message: "activate Factory visualization: required request parameters are missing",
	})
	status, response, ok := factoryvisualizationhttp.VisualizationRootErrorResponseForTest(wrapped)
	if !ok {
		t.Fatal("VisualizationRootErrorResponse = false, want true")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok {
		t.Fatalf("response = %#v, want ErrorResponse", response)
	}
	if errResp.Code != factoryapi.ErrorResponseCode(factoryvisualization.LifecycleErrorMissingParameters) {
		t.Fatalf("code = %q, want MISSING_PARAMETERS", errResp.Code)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("family = %q, want BAD_REQUEST", errResp.Family)
	}
}

func TestVisualizationRootErrorResponseMapsLifecycleStateConflicts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		kind   factoryvisualization.LifecycleErrorKind
		status int
		family factoryapi.ErrorFamily
	}{
		{
			name:   "already activated",
			kind:   factoryvisualization.LifecycleErrorAlreadyActivated,
			status: http.StatusConflict,
			family: factoryapi.ErrorFamilyConflict,
		},
		{
			name:   "not activated",
			kind:   factoryvisualization.LifecycleErrorNotActivated,
			status: http.StatusConflict,
			family: factoryapi.ErrorFamilyConflict,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			status, response, ok := factoryvisualizationhttp.VisualizationRootErrorResponseForTest(
				&factoryvisualization.LifecycleError{
					Kind:    tc.kind,
					Message: "lifecycle conflict",
				},
			)
			if !ok {
				t.Fatal("VisualizationRootErrorResponse = false, want true")
			}
			if status != tc.status {
				t.Fatalf("status = %d, want %d", status, tc.status)
			}
			errResp, ok := response.(factoryapi.ErrorResponse)
			if !ok {
				t.Fatalf("response = %#v, want ErrorResponse", response)
			}
			if errResp.Code != factoryapi.ErrorResponseCode(tc.kind) {
				t.Fatalf("code = %q, want %q", errResp.Code, tc.kind)
			}
			if errResp.Family != tc.family {
				t.Fatalf("family = %q, want %q", errResp.Family, tc.family)
			}
		})
	}
}

func TestVisualizationRootErrorResponseMapsProjectionErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		kind   factoryvisualization.ProjectionErrorKind
		status int
		family factoryapi.ErrorFamily
	}{
		{
			name:   "invalid input",
			kind:   factoryvisualization.ProjectionErrorInvalidInput,
			status: http.StatusBadRequest,
			family: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name:   "snapshot unavailable",
			kind:   factoryvisualization.ProjectionErrorSnapshotUnavailable,
			status: http.StatusServiceUnavailable,
			family: factoryapi.ErrorFamilyInternalServerError,
		},
		{
			name:   "reconstruction failed",
			kind:   factoryvisualization.ProjectionErrorReconstructionFailed,
			status: http.StatusInternalServerError,
			family: factoryapi.ErrorFamilyInternalServerError,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			status, response, ok := factoryvisualizationhttp.VisualizationRootErrorResponseForTest(
				&factoryvisualization.ProjectionError{
					Kind:    tc.kind,
					Message: "projection failure",
				},
			)
			if !ok {
				t.Fatal("VisualizationRootErrorResponse = false, want true")
			}
			if status != tc.status {
				t.Fatalf("status = %d, want %d", status, tc.status)
			}
			errResp, ok := response.(factoryapi.ErrorResponse)
			if !ok {
				t.Fatalf("response = %#v, want ErrorResponse", response)
			}
			if errResp.Code != factoryapi.ErrorResponseCode(tc.kind) {
				t.Fatalf("code = %q, want %q", errResp.Code, tc.kind)
			}
			if errResp.Family != tc.family {
				t.Fatalf("family = %q, want %q", errResp.Family, tc.family)
			}
		})
	}
}

func TestVisualizationRootErrorResponseMapsPresentationErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		kind   factoryvisualization.PresentationErrorKind
		status int
		family factoryapi.ErrorFamily
	}{
		{
			name:   "invalid input",
			kind:   factoryvisualization.PresentationErrorInvalidInput,
			status: http.StatusBadRequest,
			family: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name:   "enqueue after close",
			kind:   factoryvisualization.PresentationErrorEnqueueAfterClose,
			status: http.StatusConflict,
			family: factoryapi.ErrorFamilyConflict,
		},
		{
			name:   "finalize without writer",
			kind:   factoryvisualization.PresentationErrorFinalizeWithoutWriter,
			status: http.StatusConflict,
			family: factoryapi.ErrorFamilyConflict,
		},
		{
			name:   "backpressure rejected",
			kind:   factoryvisualization.PresentationErrorBackpressureRejected,
			status: http.StatusServiceUnavailable,
			family: factoryapi.ErrorFamilyInternalServerError,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			status, response, ok := factoryvisualizationhttp.VisualizationRootErrorResponseForTest(
				&factoryvisualization.PresentationError{
					Kind:    tc.kind,
					Message: "presentation failure",
				},
			)
			if !ok {
				t.Fatal("VisualizationRootErrorResponse = false, want true")
			}
			if status != tc.status {
				t.Fatalf("status = %d, want %d", status, tc.status)
			}
			errResp, ok := response.(factoryapi.ErrorResponse)
			if !ok {
				t.Fatalf("response = %#v, want ErrorResponse", response)
			}
			if errResp.Code != factoryapi.ErrorResponseCode(tc.kind) {
				t.Fatalf("code = %q, want %q", errResp.Code, tc.kind)
			}
			if errResp.Family != tc.family {
				t.Fatalf("family = %q, want %q", errResp.Family, tc.family)
			}
		})
	}
}

func TestVisualizationRootErrorResponseReturnsFalseForUnknownError(t *testing.T) {
	t.Parallel()

	if _, _, ok := factoryvisualizationhttp.VisualizationRootErrorResponseForTest(errors.New("opaque")); ok {
		t.Fatal("VisualizationRootErrorResponse = true, want false")
	}
}

func TestHandleActivateLifecycle_HTTPRoundTripMissingParametersTypedError(t *testing.T) {
	t.Parallel()

	root := &lifecycleVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-visualization/lifecycle/activate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.HandleActivateLifecycle(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	assertVisualizationErrorResponse(
		t,
		rec.Body.Bytes(),
		factoryapi.ErrorFamilyBadRequest,
		factoryapi.ErrorResponseCode(factoryvisualization.LifecycleErrorMissingParameters),
		"activate Factory visualization: required request parameters are missing",
	)
}

func TestHandleJoinLifecycle_HTTPRoundTripNotActivatedTypedError(t *testing.T) {
	t.Parallel()

	root := &joinNotActivatedVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-visualization/lifecycle/join", bytes.NewReader(nil))
	rec := httptest.NewRecorder()
	handler.HandleJoinLifecycle(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	assertVisualizationErrorResponse(
		t,
		rec.Body.Bytes(),
		factoryapi.ErrorFamilyConflict,
		factoryapi.ErrorResponseCode(factoryvisualization.LifecycleErrorNotActivated),
		"join Factory visualization: not activated",
	)
}

func TestHandleObserve_HTTPRoundTripSnapshotUnavailableTypedError(t *testing.T) {
	t.Parallel()

	root := &observeSnapshotUnavailableVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-visualization/observe",
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`),
	)
	rec := httptest.NewRecorder()
	handler.HandleObserve(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	assertVisualizationErrorResponse(
		t,
		rec.Body.Bytes(),
		factoryapi.ErrorFamilyInternalServerError,
		factoryapi.ErrorResponseCode(factoryvisualization.ProjectionErrorSnapshotUnavailable),
		"observe Factory visualization: snapshot unavailable",
	)
}

func TestHandlePresentProgress_HTTPRoundTripEnqueueAfterCloseTypedError(t *testing.T) {
	t.Parallel()

	root := &presentProgressEnqueueAfterCloseVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-visualization/presentation/progress",
		strings.NewReader(`{"session_id":"sess-001","records":[]}`),
	)
	rec := httptest.NewRecorder()
	handler.HandlePresentProgress(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	assertVisualizationErrorResponse(
		t,
		rec.Body.Bytes(),
		factoryapi.ErrorFamilyConflict,
		factoryapi.ErrorResponseCode(factoryvisualization.PresentationErrorEnqueueAfterClose),
		"present progress: presentation session is closed",
	)
}

func TestHandleObserve_HTTPRoundTripUnmappedErrorDoesNotLeakInternalPaths(t *testing.T) {
	t.Parallel()

	root := &observeOpaqueFailureVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-visualization/observe",
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`),
	)
	rec := httptest.NewRecorder()
	handler.HandleObserve(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	assertVisualizationErrorResponse(
		t,
		rec.Body.Bytes(),
		factoryapi.ErrorFamilyInternalServerError,
		factoryapi.ErrorResponseCodeINTERNALERROR,
		"factory visualization request failed",
	)
	body := rec.Body.String()
	if strings.Contains(body, "pkg/services/factory_visualization/internal") {
		t.Fatalf("error body leaked internal package path: %s", body)
	}
}

type joinNotActivatedVisualizationRootFake struct {
	lifecycleVisualizationRootFake
}

func (fake *joinNotActivatedVisualizationRootFake) Join(
	context.Context,
	factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	return factoryvisualization.JoinResult{}, &factoryvisualization.LifecycleError{
		Kind:    factoryvisualization.LifecycleErrorNotActivated,
		Message: "join Factory visualization: not activated",
	}
}

type observeSnapshotUnavailableVisualizationRootFake struct {
	lifecycleVisualizationRootFake
}

func (fake *observeSnapshotUnavailableVisualizationRootFake) Observe(
	context.Context,
	factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
		Kind:    factoryvisualization.ProjectionErrorSnapshotUnavailable,
		Message: "observe Factory visualization: snapshot unavailable",
	}
}

type presentProgressEnqueueAfterCloseVisualizationRootFake struct {
	lifecycleVisualizationRootFake
}

func (fake *presentProgressEnqueueAfterCloseVisualizationRootFake) PresentProgress(
	context.Context,
	factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	return factoryvisualization.PresentProgressResult{}, &factoryvisualization.PresentationError{
		Kind:    factoryvisualization.PresentationErrorEnqueueAfterClose,
		Message: "present progress: presentation session is closed",
	}
}

type observeOpaqueFailureVisualizationRootFake struct {
	lifecycleVisualizationRootFake
}

func (fake *observeOpaqueFailureVisualizationRootFake) Observe(
	context.Context,
	factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	return factoryvisualization.ObserveResult{}, errors.New(
		"pkg/services/factory_visualization/internal/services/live_view_projection: reconstruction exploded",
	)
}

func assertVisualizationErrorResponse(
	t *testing.T,
	body []byte,
	wantFamily factoryapi.ErrorFamily,
	wantCode factoryapi.ErrorResponseCode,
	wantMessage string,
) {
	t.Helper()

	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v body=%s", err, string(body))
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
