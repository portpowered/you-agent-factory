package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestOpenPresentationHTTP_DecodesModeAndMapsSuccess(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{
		openResult: factoryvisualization.OpenPresentationResult{
			SessionID: "presentation-1",
			Mode:      factoryvisualization.PresentationDeliveryLossless,
		},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.OpenPresentationHTTP(
		context.Background(),
		strings.NewReader(`{"mode":"LOSSLESS"}`),
	)
	if err != nil {
		t.Fatalf("OpenPresentationHTTP: %v", err)
	}
	if !root.openInvoked {
		t.Fatal("OpenPresentation was not invoked through the injected Visualization root")
	}
	if root.lastOpenMode != factoryvisualization.PresentationDeliveryLossless {
		t.Fatalf("lastOpenMode = %q, want LOSSLESS", root.lastOpenMode)
	}
	if response.SessionID != "presentation-1" {
		t.Fatalf("response.SessionID = %q, want presentation-1", response.SessionID)
	}
	if response.Mode != string(factoryvisualization.PresentationDeliveryLossless) {
		t.Fatalf("response.Mode = %q, want LOSSLESS", response.Mode)
	}
}

func TestPresentProgressHTTP_DecodesSessionAndRecordsAndMapsSuccess(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{
		presentResult: factoryvisualization.PresentProgressResult{AcceptedCount: 2},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.PresentProgressHTTP(
		context.Background(),
		strings.NewReader(`{"session_id":"presentation-1","records":[{"payload":"aGVsbG8="},{"payload":"d29ybGQ="}]}`),
	)
	if err != nil {
		t.Fatalf("PresentProgressHTTP: %v", err)
	}
	if !root.presentInvoked {
		t.Fatal("PresentProgress was not invoked through the injected Visualization root")
	}
	if root.lastPresentSessionID != "presentation-1" {
		t.Fatalf("lastPresentSessionID = %q, want presentation-1", root.lastPresentSessionID)
	}
	if len(root.lastPresentRecords) != 2 {
		t.Fatalf("lastPresentRecords len = %d, want 2", len(root.lastPresentRecords))
	}
	if string(root.lastPresentRecords[0].Payload) != "hello" {
		t.Fatalf("first record payload = %q, want hello", root.lastPresentRecords[0].Payload)
	}
	if string(root.lastPresentRecords[1].Payload) != "world" {
		t.Fatalf("second record payload = %q, want world", root.lastPresentRecords[1].Payload)
	}
	if response.AcceptedCount != 2 {
		t.Fatalf("response.AcceptedCount = %d, want 2", response.AcceptedCount)
	}
}

func TestFinalizePresentationHTTP_DecodesSessionAndTerminalAndMapsSuccess(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{
		finalizeResult: factoryvisualization.FinalizePresentationResult{
			Finalized:    true,
			ProgressSeen: true,
		},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.FinalizePresentationHTTP(
		context.Background(),
		strings.NewReader(`{"session_id":"presentation-1","terminal":{"payload":"ZG9uZQ=="}}`),
	)
	if err != nil {
		t.Fatalf("FinalizePresentationHTTP: %v", err)
	}
	if !root.finalizeInvoked {
		t.Fatal("FinalizePresentation was not invoked through the injected Visualization root")
	}
	if root.lastFinalizeSessionID != "presentation-1" {
		t.Fatalf("lastFinalizeSessionID = %q, want presentation-1", root.lastFinalizeSessionID)
	}
	if root.lastFinalizeTerminal == nil {
		t.Fatal("lastFinalizeTerminal = nil, want terminal payload")
	}
	if string(root.lastFinalizeTerminal.Payload) != "done" {
		t.Fatalf("terminal payload = %q, want done", root.lastFinalizeTerminal.Payload)
	}
	if !response.Finalized || !response.ProgressSeen {
		t.Fatalf("response = %#v, want finalized with progress seen", response)
	}
}

func TestClosePresentationHTTP_DecodesSessionAndMapsSuccess(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{
		closeResult: factoryvisualization.ClosePresentationResult{DroppedCount: 3},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.ClosePresentationHTTP(
		context.Background(),
		strings.NewReader(`{"session_id":"presentation-1"}`),
	)
	if err != nil {
		t.Fatalf("ClosePresentationHTTP: %v", err)
	}
	if !root.closeInvoked {
		t.Fatal("ClosePresentation was not invoked through the injected Visualization root")
	}
	if root.lastCloseSessionID != "presentation-1" {
		t.Fatalf("lastCloseSessionID = %q, want presentation-1", root.lastCloseSessionID)
	}
	if response.DroppedCount != 3 {
		t.Fatalf("response.DroppedCount = %d, want 3", response.DroppedCount)
	}
}

func TestOpenPresentationHTTP_RejectsMalformedJSONBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.OpenPresentationHTTP(context.Background(), strings.NewReader(`{"mode":`))
	if err == nil {
		t.Fatal("OpenPresentationHTTP malformed JSON = nil, want error")
	}
	if root.openInvoked {
		t.Fatal("malformed OpenPresentation HTTP request must not invoke root")
	}
}

func TestPresentProgressHTTP_RejectsUnknownFieldsBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.PresentProgressHTTP(
		context.Background(),
		strings.NewReader(`{"session_id":"presentation-1","records":[],"extra":true}`),
	)
	if err == nil {
		t.Fatal("PresentProgressHTTP unknown field = nil, want error")
	}
	if root.presentInvoked {
		t.Fatal("unknown-field PresentProgress HTTP request must not invoke root")
	}
}

func TestOpenPresentationHTTP_PassesMissingModeToRoot(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.OpenPresentationHTTP(context.Background(), strings.NewReader(`{}`))
	if err == nil {
		t.Fatal("OpenPresentationHTTP missing mode = nil, want root invalid-input error")
	}
	if !root.openInvoked {
		t.Fatal("missing-mode OpenPresentation HTTP request must invoke root for typed invalid-input failure")
	}
	var presErr *factoryvisualization.PresentationError
	if !errors.As(err, &presErr) || presErr.Kind != factoryvisualization.PresentationErrorInvalidInput {
		t.Fatalf("err = %v, want PresentationErrorInvalidInput", err)
	}
}

func TestPresentProgressHTTP_PassesMissingSessionIDToRoot(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.PresentProgressHTTP(
		context.Background(),
		strings.NewReader(`{"records":[{"payload":"YQ=="}]}`),
	)
	if err == nil {
		t.Fatal("PresentProgressHTTP missing session id = nil, want root invalid-input error")
	}
	if !root.presentInvoked {
		t.Fatal("missing-session PresentProgress HTTP request must invoke root for typed invalid-input failure")
	}
	var presErr *factoryvisualization.PresentationError
	if !errors.As(err, &presErr) || presErr.Kind != factoryvisualization.PresentationErrorInvalidInput {
		t.Fatalf("err = %v, want PresentationErrorInvalidInput", err)
	}
}

func TestHandleOpenPresentation_HTTPRoundTripSuccess(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{
		openResult: factoryvisualization.OpenPresentationResult{
			SessionID: "presentation-9",
			Mode:      factoryvisualization.PresentationDeliveryBestEffort,
		},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-visualization/presentation/open",
		strings.NewReader(`{"mode":"BEST_EFFORT"}`),
	)
	rec := httptest.NewRecorder()
	handler.HandleOpenPresentation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response factoryvisualizationhttp.OpenPresentationHTTPResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.SessionID != "presentation-9" || response.Mode != "BEST_EFFORT" {
		t.Fatalf("response = %#v, want session presentation-9 mode BEST_EFFORT", response)
	}
}

func TestHandlePresentProgress_HTTPRoundTripBadRequestBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-visualization/presentation/progress",
		strings.NewReader(`not-json`),
	)
	rec := httptest.NewRecorder()
	handler.HandlePresentProgress(rec, req)

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
	if root.presentInvoked {
		t.Fatal("malformed PresentProgress HTTP request must not invoke root")
	}
}

func TestHandleClosePresentation_HTTPRoundTripSuccess(t *testing.T) {
	t.Parallel()

	root := &presentationVisualizationRootFake{
		closeResult: factoryvisualization.ClosePresentationResult{DroppedCount: 1},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-visualization/presentation/close",
		strings.NewReader(`{"session_id":"presentation-1"}`),
	)
	rec := httptest.NewRecorder()
	handler.HandleClosePresentation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response factoryvisualizationhttp.ClosePresentationHTTPResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.DroppedCount != 1 {
		t.Fatalf("response.DroppedCount = %d, want 1", response.DroppedCount)
	}
}

type presentationVisualizationRootFake struct {
	openInvoked     bool
	presentInvoked  bool
	finalizeInvoked bool
	closeInvoked    bool

	lastOpenMode          factoryvisualization.PresentationDeliveryMode
	lastPresentSessionID  factoryvisualization.PresentationSessionID
	lastPresentRecords    []factoryvisualization.ProgressRecord
	lastFinalizeSessionID factoryvisualization.PresentationSessionID
	lastFinalizeTerminal  *factoryvisualization.TerminalWrite
	lastCloseSessionID    factoryvisualization.PresentationSessionID

	openResult     factoryvisualization.OpenPresentationResult
	presentResult  factoryvisualization.PresentProgressResult
	finalizeResult factoryvisualization.FinalizePresentationResult
	closeResult    factoryvisualization.ClosePresentationResult
}

var _ factoryvisualization.Root = (*presentationVisualizationRootFake)(nil)

func (fake *presentationVisualizationRootFake) Activate(
	context.Context,
	factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	panic("unexpected Activate call in presentation HTTP adapter test")
}

func (fake *presentationVisualizationRootFake) Join(
	context.Context,
	factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	panic("unexpected Join call in presentation HTTP adapter test")
}

func (fake *presentationVisualizationRootFake) StopDrain(
	context.Context,
	factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	panic("unexpected StopDrain call in presentation HTTP adapter test")
}

func (fake *presentationVisualizationRootFake) Observe(
	context.Context,
	factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	panic("unexpected Observe call in presentation HTTP adapter test")
}

func (fake *presentationVisualizationRootFake) OpenPresentation(
	_ context.Context,
	req factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	fake.openInvoked = true
	fake.lastOpenMode = req.Mode
	if req.Mode == "" {
		return factoryvisualization.OpenPresentationResult{}, &factoryvisualization.PresentationError{
			Kind:    factoryvisualization.PresentationErrorInvalidInput,
			Message: "open Factory visualization presentation: required request parameters are missing",
		}
	}
	return fake.openResult, nil
}

func (fake *presentationVisualizationRootFake) PresentProgress(
	_ context.Context,
	req factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	fake.presentInvoked = true
	fake.lastPresentSessionID = req.SessionID
	fake.lastPresentRecords = append([]factoryvisualization.ProgressRecord(nil), req.Records...)
	if req.SessionID == "" {
		return factoryvisualization.PresentProgressResult{}, &factoryvisualization.PresentationError{
			Kind:    factoryvisualization.PresentationErrorInvalidInput,
			Message: "Factory visualization presentation: session id is required",
		}
	}
	return fake.presentResult, nil
}

func (fake *presentationVisualizationRootFake) FinalizePresentation(
	_ context.Context,
	req factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	fake.finalizeInvoked = true
	fake.lastFinalizeSessionID = req.SessionID
	if req.Terminal != nil {
		terminal := *req.Terminal
		fake.lastFinalizeTerminal = &terminal
	} else {
		fake.lastFinalizeTerminal = nil
	}
	return fake.finalizeResult, nil
}

func (fake *presentationVisualizationRootFake) ClosePresentation(
	_ context.Context,
	req factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	fake.closeInvoked = true
	fake.lastCloseSessionID = req.SessionID
	return fake.closeResult, nil
}
