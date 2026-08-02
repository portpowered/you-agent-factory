package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func waitForModelsRoot(ctx context.Context, _ string) (models.Detail, error) {
	<-ctx.Done()
	return models.Detail{}, ctx.Err()
}

func waitForModelsPull(ctx context.Context, _ string) (models.PullResult, error) {
	<-ctx.Done()
	return models.PullResult{}, ctx.Err()
}

func TestHandler_ListModelsCanceledBeforeRootCallCompletesWithoutBody(t *testing.T) {
	t.Parallel()

	root := &rootFake{}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	handler.ListModels(recorder, httptest.NewRequest(http.MethodGet, "/models", nil).WithContext(ctx))

	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestHandler_GetModelCanceledDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		get: waitForModelsRoot,
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/models/voice", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.GetModel(recorder, req, "voice")
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GetModel hung after request cancellation")
	}
	body := recorder.Body.String()
	if body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestHandler_PullModelCanceledDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		pull: waitForModelsPull,
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.PullModel(recorder, req, "voice")
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PullModel hung after request cancellation")
	}
	body := recorder.Body.String()
	if body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestHandler_ListModelsDeadlineExceededReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		list: func(ctx context.Context) (models.List, error) {
			<-ctx.Done()
			return models.List{}, ctx.Err()
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ListModels(recorder, httptest.NewRequest(http.MethodGet, "/models", nil).WithContext(ctx))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ListModels hung after request deadline")
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	assertCatalogHTTPError(
		t,
		recorder,
		http.StatusGatewayTimeout,
		"INTERNAL_ERROR",
		"models request timed out",
	)
}

func TestHandler_PullModelDeadlineExceededReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		pull: func(ctx context.Context, _ string) (models.PullResult, error) {
			<-ctx.Done()
			return models.PullResult{}, ctx.Err()
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil).WithContext(ctx), "voice")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PullModel hung after request deadline")
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	assertCatalogHTTPError(
		t,
		recorder,
		http.StatusGatewayTimeout,
		"INTERNAL_ERROR",
		"models request timed out",
	)
}

func TestHandler_InvokeModelCanceledDuringModelsRootCallCompletesWithoutBody(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	root := &rootFake{
		getCatalog: func(ctx context.Context, _ models.GetModelRequest) (models.GetModelResult, error) {
			close(started)
			<-ctx.Done()
			return models.GetModelResult{}, ctx.Err()
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}]}`)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.InvokeModel(recorder, req, "voice")
		close(done)
	}()
	<-started
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("InvokeModel hung after request cancellation")
	}
	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestHandler_InvokeModelDeadlineDuringModelsRootCallReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	root := &rootFake{
		getCatalog: func(ctx context.Context, _ models.GetModelRequest) (models.GetModelResult, error) {
			close(started)
			<-ctx.Done()
			return models.GetModelResult{}, ctx.Err()
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}]}`)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.InvokeModel(recorder, req, "voice")
		close(done)
	}()
	<-started

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("InvokeModel hung after request deadline")
	}
	assertCatalogHTTPError(t, recorder, http.StatusGatewayTimeout, "INTERNAL_ERROR", "models request timed out")
}

func TestModelsRequestContextErrorResponseForTest(t *testing.T) {
	t.Parallel()

	if status, response, ok := ModelsRequestContextErrorResponseForTest(context.Canceled); !ok || status != 0 || response != nil {
		t.Fatalf("canceled = (%d, %#v, %v), want (0, nil, true)", status, response, ok)
	}

	status, response, ok := ModelsRequestContextErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout {
		t.Fatalf("deadline status = %d, ok = %v, want 504 true", status, ok)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !strings.Contains(string(body), "models request timed out") {
		t.Fatalf("response = %s, want timeout message", body)
	}
	if !strings.Contains(string(body), string(factoryapi.ErrorResponseCodeINTERNALERROR)) {
		t.Fatalf("response = %s, want INTERNAL_ERROR code", body)
	}
}
