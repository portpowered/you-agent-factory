package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func waitForWorkRootContext(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestWorkRequestContextErrorResponseForTest(t *testing.T) {
	t.Parallel()

	if status, response, ok := WorkRequestContextErrorResponseForTest(context.Canceled); !ok || status != 0 || response != nil {
		t.Fatalf("canceled = (%d, %#v, %v), want (0, nil, true)", status, response, ok)
	}

	status, response, ok := WorkRequestContextErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout {
		t.Fatalf("deadline status = %d, ok = %v, want 504 true", status, ok)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok {
		t.Fatalf("deadline response = %#v, want ErrorResponse", response)
	}
	if errResp.Message != "work request timed out" ||
		errResp.Family != factoryapi.ErrorFamilyInternalServerError ||
		errResp.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("deadline response = %#v, want timeout message", errResp)
	}
}

func TestRootErrorResponse_MapsRequestContextFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := RootErrorResponse(context.Canceled)
	if !ok || status != 0 || response.Message != "" {
		t.Fatalf("canceled = (%d, %#v, %v), want handled cancel outcome", status, response, ok)
	}

	status, response, ok = RootErrorResponse(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout || response.Message != "work request timed out" {
		t.Fatalf("deadline = (%d, %#v, %v), want 504 timeout outcome", status, response, ok)
	}
}

func TestShouldEndOnRequestContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !shouldEndOnRequestContext(ctx, nil) {
		t.Fatal("canceled request context should end the handler")
	}
	if !shouldEndOnRequestContext(context.Background(), context.Canceled) {
		t.Fatal("context.Canceled error should end the handler")
	}
	if !shouldEndOnRequestContext(context.Background(), context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded error should end the handler")
	}
	if shouldEndOnRequestContext(context.Background(), errors.New("boom")) {
		t.Fatal("unrelated errors must not end the handler")
	}
}

func TestWriteRootOrInternalError_DoesNotMapCancelToInternalError(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{})
	recorder := httptest.NewRecorder()

	WriteRootOrInternalErrorForTest(adapter, recorder, context.Canceled, "failed to list Work")

	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want default recorder status without encoded body", recorder.Code)
	}
}

func TestWriteRootOrInternalError_MapsDeadlineExceededToGatewayTimeout(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{})
	recorder := httptest.NewRecorder()

	WriteRootOrInternalErrorForTest(adapter, recorder, context.DeadlineExceeded, "failed to list Work")

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Message != "work request timed out" ||
		response.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("response = %s, want timeout ErrorResponse", recorder.Body.String())
	}
}

func TestListWorkBySessionId_EndsWithoutInvokeWhenContextCanceledBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
			invoked = true
			return work.ListResult{}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	assertHandlerReturnsWithin(t, time.Second, func() {
		adapter.ListWorkBySessionId(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work", nil).WithContext(ctx),
			"session-1",
			factoryapi.ListWorkBySessionIdParams{},
		)
	})

	if invoked {
		t.Fatal("canceled request context must end before Work root list")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map cancellation to INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestListWorkBySessionId_EndsWithoutErrorWhenCanceledDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	adapter := NewAdapter(&rootFake{
		listWork: func(rootCtx context.Context, _ string, _ work.ListOptions) (work.ListResult, error) {
			return work.ListResult{}, waitForWorkRootContext(rootCtx)
		},
	})
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.ListWorkBySessionId(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work", nil).WithContext(ctx),
			"session-1",
			factoryapi.ListWorkBySessionIdParams{},
		)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("list handler hung after request context cancellation")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map cancellation to INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestListWorkBySessionId_EndsWithoutErrorWhenTimedOutDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	adapter := NewAdapter(&rootFake{
		listWork: func(rootCtx context.Context, _ string, _ work.ListOptions) (work.ListResult, error) {
			return work.ListResult{}, waitForWorkRootContext(rootCtx)
		},
	})
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.ListWorkBySessionId(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work", nil).WithContext(ctx),
			"session-1",
			factoryapi.ListWorkBySessionIdParams{},
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("list handler hung after request context timeout")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map timeout to INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestGetWorkBySessionId_EndsWithoutInvokeWhenContextCanceledBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		getWork: func(context.Context, string, string) (work.ReadModel, error) {
			invoked = true
			return work.ReadModel{}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	assertHandlerReturnsWithin(t, time.Second, func() {
		adapter.GetWorkBySessionId(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work/work-1", nil).WithContext(ctx),
			"session-1",
			"work-1",
		)
	})

	if invoked {
		t.Fatal("canceled request context must end before Work root get")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map cancellation to INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestStageSubmitWorkFileBySessionId_EndsWithoutInvokeWhenContextCanceledBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		stageContent: func(context.Context, work.StageContentRequest) (work.StageContentResult, error) {
			invoked = true
			return work.StageContentResult{}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	assertHandlerReturnsWithin(t, time.Second, func() {
		adapter.StageSubmitWorkFileBySessionId(
			recorder,
			httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/stage-submit-work-file", strings.NewReader(`{
				"itemType":"image",
				"fileName":"ui.png",
				"mediaType":"image/png",
				"contentBase64":"aGVsbG8="
			}`)).WithContext(ctx),
			"session-1",
		)
	})

	if invoked {
		t.Fatal("canceled request context must end before Work root stage")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map cancellation to INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestStageSubmitWorkFileBySessionId_EndsWithoutErrorWhenCanceledDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	adapter := NewAdapter(&rootFake{
		stageContent: func(rootCtx context.Context, _ work.StageContentRequest) (work.StageContentResult, error) {
			return work.StageContentResult{}, waitForWorkRootContext(rootCtx)
		},
	})
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.StageSubmitWorkFileBySessionId(
			recorder,
			httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/stage-submit-work-file", strings.NewReader(`{
				"itemType":"image",
				"fileName":"ui.png",
				"mediaType":"image/png",
				"contentBase64":"aGVsbG8="
			}`)).WithContext(ctx),
			"session-1",
		)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stage handler hung after request context cancellation")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map cancellation to INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestStageSubmitWorkFileBySessionId_EndsWithoutErrorWhenTimedOutDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	adapter := NewAdapter(&rootFake{
		stageContent: func(rootCtx context.Context, _ work.StageContentRequest) (work.StageContentResult, error) {
			return work.StageContentResult{}, waitForWorkRootContext(rootCtx)
		},
	})
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.StageSubmitWorkFileBySessionId(
			recorder,
			httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/stage-submit-work-file", strings.NewReader(`{
				"itemType":"image",
				"fileName":"ui.png",
				"mediaType":"image/png",
				"contentBase64":"aGVsbG8="
			}`)).WithContext(ctx),
			"session-1",
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stage handler hung after request context timeout")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map timeout to INTERNAL_ERROR: %s", recorder.Body.String())
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
