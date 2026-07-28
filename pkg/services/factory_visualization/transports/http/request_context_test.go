package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

type blockingVisualizationRootFake struct {
	mu sync.Mutex

	observeInvoked bool
	activateInvoked bool

	observe func(context.Context, factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error)
	activate func(context.Context, factoryvisualization.ActivateRequest) (factoryvisualization.ActivateResult, error)
}

var _ factoryvisualization.Root = (*blockingVisualizationRootFake)(nil)

func (fake *blockingVisualizationRootFake) Activate(
	ctx context.Context,
	req factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	fake.mu.Lock()
	fake.activateInvoked = true
	activate := fake.activate
	fake.mu.Unlock()
	if activate != nil {
		return activate(ctx, req)
	}
	<-ctx.Done()
	return factoryvisualization.ActivateResult{}, ctx.Err()
}

func (fake *blockingVisualizationRootFake) Join(
	context.Context,
	factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	panic("unexpected Join call in request-context HTTP adapter test")
}

func (fake *blockingVisualizationRootFake) StopDrain(
	context.Context,
	factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	panic("unexpected StopDrain call in request-context HTTP adapter test")
}

func (fake *blockingVisualizationRootFake) Observe(
	ctx context.Context,
	req factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	fake.mu.Lock()
	fake.observeInvoked = true
	observe := fake.observe
	fake.mu.Unlock()
	if observe != nil {
		return observe(ctx, req)
	}
	<-ctx.Done()
	return factoryvisualization.ObserveResult{}, ctx.Err()
}

func (fake *blockingVisualizationRootFake) OpenPresentation(
	context.Context,
	factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	panic("unexpected OpenPresentation call in request-context HTTP adapter test")
}

func (fake *blockingVisualizationRootFake) PresentProgress(
	context.Context,
	factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	panic("unexpected PresentProgress call in request-context HTTP adapter test")
}

func (fake *blockingVisualizationRootFake) FinalizePresentation(
	context.Context,
	factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	panic("unexpected FinalizePresentation call in request-context HTTP adapter test")
}

func (fake *blockingVisualizationRootFake) ClosePresentation(
	context.Context,
	factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	panic("unexpected ClosePresentation call in request-context HTTP adapter test")
}

func TestHandleObserve_CanceledDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	root := &blockingVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-visualization/observe",
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`),
	).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.HandleObserve(recorder, req)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleObserve hung after request cancellation")
	}
	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestHandleObserve_CanceledBeforeRootCallCompletesWithoutBody(t *testing.T) {
	t.Parallel()

	root := &blockingVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-visualization/observe",
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`),
	).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	handler.HandleObserve(recorder, req)

	root.mu.Lock()
	invoked := root.observeInvoked
	root.mu.Unlock()
	if invoked {
		t.Fatal("Observe was invoked after request context was canceled")
	}
	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestHandleObserve_DeadlineExceededReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	root := &blockingVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		req := httptest.NewRequest(
			http.MethodPost,
			"/factory-visualization/observe",
			strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`),
		).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		handler.HandleObserve(recorder, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleObserve hung after request deadline")
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	assertVisualizationErrorResponse(
		t,
		recorder.Body.Bytes(),
		factoryapi.ErrorFamilyInternalServerError,
		factoryapi.ErrorResponseCodeINTERNALERROR,
		"factory visualization request timed out",
	)
}

func TestHandleActivateLifecycle_CanceledDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	root := &blockingVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-visualization/lifecycle/activate",
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`),
	).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.HandleActivateLifecycle(recorder, req)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleActivateLifecycle hung after request cancellation")
	}
	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestHandleActivateLifecycle_DeadlineExceededReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	root := &blockingVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		req := httptest.NewRequest(
			http.MethodPost,
			"/factory-visualization/lifecycle/activate",
			strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`),
		).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		handler.HandleActivateLifecycle(recorder, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleActivateLifecycle hung after request deadline")
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	assertVisualizationErrorResponse(
		t,
		recorder.Body.Bytes(),
		factoryapi.ErrorFamilyInternalServerError,
		factoryapi.ErrorResponseCodeINTERNALERROR,
		"factory visualization request timed out",
	)
}

func TestVisualizationRequestContextErrorResponseForTest(t *testing.T) {
	t.Parallel()

	if status, response, ok := factoryvisualizationhttp.VisualizationRequestContextErrorResponseForTest(context.Canceled); !ok || status != 0 || response != nil {
		t.Fatalf("canceled = (%d, %#v, %v), want (0, nil, true)", status, response, ok)
	}

	status, response, ok := factoryvisualizationhttp.VisualizationRequestContextErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout {
		t.Fatalf("deadline status = %d, ok = %v, want 504 true", status, ok)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !strings.Contains(string(body), "factory visualization request timed out") {
		t.Fatalf("response = %s, want timeout message", body)
	}
}
