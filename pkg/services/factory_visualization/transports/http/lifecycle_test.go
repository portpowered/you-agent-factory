package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestActivateLifecycleHTTP_DecodesModeAndMapsSuccess(t *testing.T) {
	t.Parallel()

	root := &lifecycleVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.ActivateLifecycleHTTP(
		context.Background(),
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`),
	)
	if err != nil {
		t.Fatalf("ActivateLifecycleHTTP: %v", err)
	}
	if !root.activateInvoked {
		t.Fatal("Activate was not invoked through the injected Visualization root")
	}
	if root.lastActivateMode != factoryvisualization.ActivateModeRetainedThenLive {
		t.Fatalf("lastActivateMode = %q, want RETAINED_THEN_LIVE", root.lastActivateMode)
	}
	if response.State != string(factoryvisualization.LifecycleStateStarted) {
		t.Fatalf("response.State = %q, want STARTED", response.State)
	}
}

func TestJoinLifecycleHTTP_InvokesRootAndMapsSuccess(t *testing.T) {
	t.Parallel()

	root := &lifecycleVisualizationRootFake{joinState: factoryvisualization.LifecycleStateStarted}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.JoinLifecycleHTTP(context.Background(), strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("JoinLifecycleHTTP: %v", err)
	}
	if !root.joinInvoked {
		t.Fatal("Join was not invoked through the injected Visualization root")
	}
	if response.State != string(factoryvisualization.LifecycleStateStarted) {
		t.Fatalf("response.State = %q, want STARTED", response.State)
	}
}

func TestStopDrainLifecycleHTTP_InvokesRootAndMapsSuccess(t *testing.T) {
	t.Parallel()

	root := &lifecycleVisualizationRootFake{stopDrainState: factoryvisualization.LifecycleStateStopped}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.StopDrainLifecycleHTTP(context.Background(), strings.NewReader(""))
	if err != nil {
		t.Fatalf("StopDrainLifecycleHTTP: %v", err)
	}
	if !root.stopDrainInvoked {
		t.Fatal("StopDrain was not invoked through the injected Visualization root")
	}
	if response.State != string(factoryvisualization.LifecycleStateStopped) {
		t.Fatalf("response.State = %q, want STOPPED", response.State)
	}
}

func TestActivateLifecycleHTTP_RejectsMalformedJSONBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &lifecycleVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.ActivateLifecycleHTTP(context.Background(), strings.NewReader(`{"mode":`))
	if err == nil {
		t.Fatal("ActivateLifecycleHTTP malformed JSON = nil, want error")
	}
	if root.activateInvoked {
		t.Fatal("malformed Activate HTTP request must not invoke root")
	}
}

func TestHandleActivateLifecycle_AcceptsUnknownFieldsWithWarning(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.WarnLevel)
	root := &lifecycleVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.New(core),
	)
	recorder := httptest.NewRecorder()
	handler.HandleActivateLifecycle(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-visualization/lifecycle/activate",
			strings.NewReader(`{"mode":"RETAINED_THEN_LIVE","future":{"value":"secret"}}`),
		),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if !root.activateInvoked || root.lastActivateMode != factoryvisualization.ActivateModeRetainedThenLive {
		t.Fatalf("root invocation = %v/%q, want known mode", root.activateInvoked, root.lastActivateMode)
	}
	warning := recorder.Header().Get("Warning")
	if !strings.Contains(warning, "299") || !strings.Contains(warning, "$.future") {
		t.Fatalf("Warning = %q, want code 299 and $.future", warning)
	}
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("warning log count = %d, want one", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["warning_code"] != int64(httpcompat.WarningCode) ||
		fields["boundary"] != "factory_visualization.http" ||
		fields["operation"] != "activate_lifecycle" {
		t.Fatalf("warning fields = %#v, want HTTP compatibility metadata", fields)
	}
	if got, ok := fields["json_paths"].([]interface{}); !ok || !reflect.DeepEqual(got, []interface{}{"$.future"}) {
		t.Fatalf("json_paths = %#v, want [$.future]", fields["json_paths"])
	}
	if strings.Contains(entries[0].Message, "secret") || strings.Contains(recorder.Body.String(), "secret") {
		t.Fatal("compatibility diagnostics exposed an ignored field value")
	}
}

func TestActivateLifecycleHTTP_PassesMissingModeToRoot(t *testing.T) {
	t.Parallel()

	root := &lifecycleVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.ActivateLifecycleHTTP(context.Background(), strings.NewReader(`{}`))
	if err == nil {
		t.Fatal("ActivateLifecycleHTTP missing mode = nil, want root missing-parameters error")
	}
	if !root.activateInvoked {
		t.Fatal("missing-mode Activate HTTP request must invoke root for typed missing-parameters failure")
	}
	var lifeErr *factoryvisualization.LifecycleError
	if !errors.As(err, &lifeErr) || lifeErr.Kind != factoryvisualization.LifecycleErrorMissingParameters {
		t.Fatalf("err = %v, want LifecycleErrorMissingParameters", err)
	}
}

func TestHandleActivateLifecycle_HTTPRoundTripSuccess(t *testing.T) {
	t.Parallel()

	root := &lifecycleVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-visualization/lifecycle/activate", strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`))
	rec := httptest.NewRecorder()
	handler.HandleActivateLifecycle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response factoryvisualizationhttp.LifecycleHTTPResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.State != string(factoryvisualization.LifecycleStateStarted) {
		t.Fatalf("response.State = %q, want STARTED", response.State)
	}
}

func TestHandleActivateLifecycle_HTTPRoundTripBadRequestBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &lifecycleVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-visualization/lifecycle/activate", strings.NewReader(`not-json`))
	rec := httptest.NewRecorder()
	handler.HandleActivateLifecycle(rec, req)

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
	if root.activateInvoked {
		t.Fatal("malformed Activate HTTP request must not invoke root")
	}
}

func TestHandleJoinLifecycle_HTTPRoundTripSuccess(t *testing.T) {
	t.Parallel()

	root := &lifecycleVisualizationRootFake{joinState: factoryvisualization.LifecycleStateStarted}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-visualization/lifecycle/join", bytes.NewReader(nil))
	rec := httptest.NewRecorder()
	handler.HandleJoinLifecycle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleStopDrainLifecycle_HTTPRoundTripSuccess(t *testing.T) {
	t.Parallel()

	root := &lifecycleVisualizationRootFake{stopDrainState: factoryvisualization.LifecycleStateStopped}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-visualization/lifecycle/stop-drain", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	handler.HandleStopDrainLifecycle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

type lifecycleVisualizationRootFake struct {
	activateInvoked  bool
	joinInvoked      bool
	stopDrainInvoked bool

	lastActivateMode factoryvisualization.ActivateMode
	joinState        factoryvisualization.LifecycleState
	stopDrainState   factoryvisualization.LifecycleState
}

var _ factoryvisualization.Root = (*lifecycleVisualizationRootFake)(nil)

func (fake *lifecycleVisualizationRootFake) Activate(
	_ context.Context,
	req factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	fake.activateInvoked = true
	fake.lastActivateMode = req.Mode
	if req.Mode == "" {
		return factoryvisualization.ActivateResult{}, &factoryvisualization.LifecycleError{
			Kind:    factoryvisualization.LifecycleErrorMissingParameters,
			Message: "activate Factory visualization: required request parameters are missing",
		}
	}
	return factoryvisualization.ActivateResult{
		State: factoryvisualization.LifecycleStateStarted,
	}, nil
}

func (fake *lifecycleVisualizationRootFake) Join(
	_ context.Context,
	_ factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	fake.joinInvoked = true
	state := fake.joinState
	if state == "" {
		state = factoryvisualization.LifecycleStateStarted
	}
	return factoryvisualization.JoinResult{State: state}, nil
}

func (fake *lifecycleVisualizationRootFake) StopDrain(
	_ context.Context,
	_ factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	fake.stopDrainInvoked = true
	state := fake.stopDrainState
	if state == "" {
		state = factoryvisualization.LifecycleStateStopped
	}
	return factoryvisualization.StopDrainResult{State: state}, nil
}

func (fake *lifecycleVisualizationRootFake) Observe(
	context.Context,
	factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	panic("unexpected Observe call in lifecycle HTTP adapter test")
}

func (fake *lifecycleVisualizationRootFake) OpenPresentation(
	context.Context,
	factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	panic("unexpected OpenPresentation call in lifecycle HTTP adapter test")
}

func (fake *lifecycleVisualizationRootFake) PresentProgress(
	context.Context,
	factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	panic("unexpected PresentProgress call in lifecycle HTTP adapter test")
}

func (fake *lifecycleVisualizationRootFake) FinalizePresentation(
	context.Context,
	factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	panic("unexpected FinalizePresentation call in lifecycle HTTP adapter test")
}

func (fake *lifecycleVisualizationRootFake) ClosePresentation(
	context.Context,
	factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	panic("unexpected ClosePresentation call in lifecycle HTTP adapter test")
}
