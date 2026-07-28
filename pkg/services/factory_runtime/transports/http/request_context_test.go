package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestGetStatus_EndsWithoutBodyWhenContextCanceledBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&runtimeRootFake{
		observe: func(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			invoked = true
			return factoryruntime.ObserveResult{}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	assertHandlerReturnsWithin(t, time.Second, func() {
		adapter.GetStatus(recorder, httptest.NewRequest(http.MethodGet, "/status", nil).WithContext(ctx))
	})

	if invoked {
		t.Fatal("canceled request context must end before Runtime root Observe")
	}
	if recorder.Body.Len() != 0 || strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response = %d %q, want no encoded body on pre-root cancellation", recorder.Code, recorder.Body.String())
	}
}

func TestGetStatus_EndsWithoutErrorWhenCanceledDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	adapter := NewAdapter(&runtimeRootFake{
		observe: func(blockCtx context.Context, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			<-blockCtx.Done()
			return factoryruntime.ObserveResult{}, context.Canceled
		},
	})
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.GetStatus(recorder, httptest.NewRequest(http.MethodGet, "/status", nil).WithContext(ctx))
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("status handler hung after request context cancellation")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) &&
		!strings.Contains(recorder.Body.String(), "factory runtime request timed out") {
		t.Fatalf("response must not map cancellation to ordinary INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestGetStatus_DeadlineExceededReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	adapter := NewAdapter(&runtimeRootFake{
		observe: func(blockCtx context.Context, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			<-blockCtx.Done()
			return factoryruntime.ObserveResult{}, context.DeadlineExceeded
		},
	})
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.GetStatus(recorder, httptest.NewRequest(http.MethodGet, "/status", nil).WithContext(ctx))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("status handler hung after request context timeout")
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	assertTimeoutErrorResponse(t, recorder.Body.Bytes())
}

func TestControlPause_EndsWithoutBodyWhenContextCanceledBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&runtimeRootFake{
		pause: func(context.Context, factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
			invoked = true
			return factoryruntime.PauseResult{}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	assertHandlerReturnsWithin(t, time.Second, func() {
		adapter.ControlPause(recorder, httptest.NewRequest(http.MethodPost, "/control/pause", nil).WithContext(ctx))
	})

	if invoked {
		t.Fatal("canceled request context must end before Runtime root ControlPause")
	}
	if recorder.Body.Len() != 0 || strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response = %d %q, want no encoded body on pre-root cancellation", recorder.Code, recorder.Body.String())
	}
}

func TestControlPause_EndsWithoutErrorWhenCanceledDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	adapter := NewAdapter(&runtimeRootFake{
		pause: func(blockCtx context.Context, _ factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
			<-blockCtx.Done()
			return factoryruntime.PauseResult{}, context.Canceled
		},
	})
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.ControlPause(recorder, httptest.NewRequest(http.MethodPost, "/control/pause", nil).WithContext(ctx))
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("control pause handler hung after request context cancellation")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) &&
		!strings.Contains(recorder.Body.String(), "factory runtime request timed out") {
		t.Fatalf("response must not map cancellation to ordinary INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestControlPause_DeadlineExceededReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	adapter := NewAdapter(&runtimeRootFake{
		pause: func(blockCtx context.Context, _ factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
			<-blockCtx.Done()
			return factoryruntime.PauseResult{}, context.DeadlineExceeded
		},
	})
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.ControlPause(recorder, httptest.NewRequest(http.MethodPost, "/control/pause", nil).WithContext(ctx))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("control pause handler hung after request context timeout")
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	assertTimeoutErrorResponse(t, recorder.Body.Bytes())
}

func TestRuntimeRequestContextErrorResponseForTest(t *testing.T) {
	t.Parallel()

	if status, response, ok := RuntimeRequestContextErrorResponseForTest(context.Canceled); !ok || status != 0 || response != nil {
		t.Fatalf("canceled = (%d, %#v, %v), want (0, nil, true)", status, response, ok)
	}

	status, response, ok := RuntimeRequestContextErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout {
		t.Fatalf("deadline status = %d, ok = %v, want 504 true", status, ok)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !strings.Contains(string(body), "factory runtime request timed out") {
		t.Fatalf("response = %s, want timeout message", body)
	}
}

func assertTimeoutErrorResponse(t *testing.T, body []byte) {
	t.Helper()
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("code = %q, want INTERNAL_ERROR timeout code", response.Code)
	}
	if response.Message != "factory runtime request timed out" {
		t.Fatalf("message = %q, want factory runtime request timed out", response.Message)
	}
}

func assertHandlerReturnsWithin(t *testing.T, timeout time.Duration, fn func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("handler did not return within %s", timeout)
	}
}
