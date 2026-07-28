package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestObserveHTTP_DecodesModeAndMapsSuccess(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, 7, 28, 2, 30, 0, 0, time.UTC)
	root := &observeVisualizationRootFake{
		observeView: factoryvisualization.ProjectedView{
			TickCount:          9,
			RetainedEventCount: 3,
			ObservedAt:         observedAt,
		},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.ObserveHTTP(
		context.Background(),
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`),
	)
	if err != nil {
		t.Fatalf("ObserveHTTP: %v", err)
	}
	if !root.observeInvoked {
		t.Fatal("Observe was not invoked through the injected Visualization root")
	}
	if root.lastObserveMode != factoryvisualization.ObserveModeRetainedThenLive {
		t.Fatalf("lastObserveMode = %q, want RETAINED_THEN_LIVE", root.lastObserveMode)
	}
	if root.lastObserveReconnect != nil {
		t.Fatalf("lastObserveReconnect = %#v, want nil", root.lastObserveReconnect)
	}
	if response.View.TickCount != 9 {
		t.Fatalf("response.View.TickCount = %d, want 9", response.View.TickCount)
	}
	if response.View.RetainedEventCount != 3 {
		t.Fatalf("response.View.RetainedEventCount = %d, want 3", response.View.RetainedEventCount)
	}
	if !response.View.ObservedAt.Equal(observedAt) {
		t.Fatalf("response.View.ObservedAt = %v, want %v", response.View.ObservedAt, observedAt)
	}
}

func TestObserveHTTP_DecodesReconnectCursorAndMapsSuccess(t *testing.T) {
	t.Parallel()

	afterSequence := 7
	root := &observeVisualizationRootFake{
		observeView: factoryvisualization.ProjectedView{
			TickCount:          4,
			RetainedEventCount: 2,
			ObservedAt:         time.Date(2026, 7, 28, 2, 31, 0, 0, time.UTC),
		},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.ObserveHTTP(
		context.Background(),
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE","reconnect":{"after_event_id":"evt-1","after_sequence":7}}`),
	)
	if err != nil {
		t.Fatalf("ObserveHTTP: %v", err)
	}
	if !root.observeInvoked {
		t.Fatal("Observe was not invoked through the injected Visualization root")
	}
	if root.lastObserveReconnect == nil {
		t.Fatal("lastObserveReconnect = nil, want reconnect cursor")
	}
	if root.lastObserveReconnect.AfterEventID != "evt-1" {
		t.Fatalf("AfterEventID = %q, want evt-1", root.lastObserveReconnect.AfterEventID)
	}
	if root.lastObserveReconnect.AfterSequence == nil || *root.lastObserveReconnect.AfterSequence != afterSequence {
		t.Fatalf("AfterSequence = %#v, want %d", root.lastObserveReconnect.AfterSequence, afterSequence)
	}
	if response.View.TickCount != 4 {
		t.Fatalf("response.View.TickCount = %d, want 4", response.View.TickCount)
	}
}

func TestObserveHTTP_RejectsMalformedJSONBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &observeVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.ObserveHTTP(context.Background(), strings.NewReader(`{"mode":`))
	if err == nil {
		t.Fatal("ObserveHTTP malformed JSON = nil, want error")
	}
	if root.observeInvoked {
		t.Fatal("malformed Observe HTTP request must not invoke root")
	}
}

func TestObserveHTTP_RejectsUnknownFieldsBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &observeVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.ObserveHTTP(
		context.Background(),
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE","extra":true}`),
	)
	if err == nil {
		t.Fatal("ObserveHTTP unknown field = nil, want error")
	}
	if root.observeInvoked {
		t.Fatal("unknown-field Observe HTTP request must not invoke root")
	}
}

func TestObserveHTTP_PassesMissingModeToRoot(t *testing.T) {
	t.Parallel()

	root := &observeVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.ObserveHTTP(context.Background(), strings.NewReader(`{}`))
	if err == nil {
		t.Fatal("ObserveHTTP missing mode = nil, want root invalid-input error")
	}
	if !root.observeInvoked {
		t.Fatal("missing-mode Observe HTTP request must invoke root for typed invalid-input failure")
	}
	var projErr *factoryvisualization.ProjectionError
	if !errors.As(err, &projErr) || projErr.Kind != factoryvisualization.ProjectionErrorInvalidInput {
		t.Fatalf("err = %v, want ProjectionErrorInvalidInput", err)
	}
}

func TestHandleObserve_HTTPRoundTripSuccess(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, 7, 28, 2, 32, 0, 0, time.UTC)
	root := &observeVisualizationRootFake{
		observeView: factoryvisualization.ProjectedView{
			TickCount:          11,
			RetainedEventCount: 5,
			ObservedAt:         observedAt,
		},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-visualization/observe", strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`))
	rec := httptest.NewRecorder()
	handler.HandleObserve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response factoryvisualizationhttp.ObserveHTTPResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.View.TickCount != 11 || response.View.RetainedEventCount != 5 {
		t.Fatalf("response.View = %#v, want tick 11 retained 5", response.View)
	}
	if !response.View.ObservedAt.Equal(observedAt) {
		t.Fatalf("response.View.ObservedAt = %v, want %v", response.View.ObservedAt, observedAt)
	}
}

func TestHandleObserve_HTTPRoundTripBadRequestBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &observeVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-visualization/observe", strings.NewReader(`not-json`))
	rec := httptest.NewRecorder()
	handler.HandleObserve(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Family != factoryapi.ErrorFamilyBadRequest || response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("error response = %#v, want bad-request ErrorResponse", response)
	}
	if root.observeInvoked {
		t.Fatal("malformed Observe HTTP request must not invoke root")
	}
}

type observeVisualizationRootFake struct {
	observeInvoked bool

	lastObserveMode      factoryvisualization.ObserveMode
	lastObserveReconnect *factoryvisualization.ObserveReconnectCursor
	observeView          factoryvisualization.ProjectedView
}

var _ factoryvisualization.Root = (*observeVisualizationRootFake)(nil)

func (fake *observeVisualizationRootFake) Activate(
	context.Context,
	factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	panic("unexpected Activate call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) Join(
	context.Context,
	factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	panic("unexpected Join call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) StopDrain(
	context.Context,
	factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	panic("unexpected StopDrain call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) Observe(
	_ context.Context,
	req factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	fake.observeInvoked = true
	fake.lastObserveMode = req.Mode
	fake.lastObserveReconnect = req.Reconnect
	if req.Mode == "" {
		return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
			Kind:    factoryvisualization.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: required request parameters are missing",
		}
	}
	if req.Reconnect != nil && req.Reconnect.AfterEventID == "" && req.Reconnect.AfterSequence == nil {
		return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
			Kind:    factoryvisualization.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: reconnect cursor is empty",
		}
	}
	view := fake.observeView
	if view.ObservedAt.IsZero() {
		view.ObservedAt = time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	}
	return factoryvisualization.ObserveResult{View: view}, nil
}

func (fake *observeVisualizationRootFake) OpenPresentation(
	context.Context,
	factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	panic("unexpected OpenPresentation call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) PresentProgress(
	context.Context,
	factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	panic("unexpected PresentProgress call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) FinalizePresentation(
	context.Context,
	factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	panic("unexpected FinalizePresentation call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) ClosePresentation(
	context.Context,
	factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	panic("unexpected ClosePresentation call in observe HTTP adapter test")
}
