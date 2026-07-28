package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func waitForProvidersRootContext(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestExecuteRequestContextErrorResponseForTest(t *testing.T) {
	t.Parallel()

	status, response, ok := ExecuteRequestContextErrorResponseForTest(context.Canceled)
	if !ok || status != http.StatusInternalServerError {
		t.Fatalf("canceled = (%d, %#v, %v), want 500 handled outcome", status, response, ok)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok {
		t.Fatalf("canceled response = %#v, want ErrorResponse", response)
	}
	if errResp.Message != "provider execution canceled" ||
		errResp.Code != factoryapi.ErrorResponseCode("PROVIDER_EXECUTION_CANCELED") {
		t.Fatalf("canceled response = %#v, want execute cancel outcome", errResp)
	}

	status, response, ok = ExecuteRequestContextErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout {
		t.Fatalf("deadline status = %d, ok = %v, want 504 true", status, ok)
	}
	errResp, ok = response.(factoryapi.ErrorResponse)
	if !ok {
		t.Fatalf("deadline response = %#v, want ErrorResponse", response)
	}
	if errResp.Message != "provider execution timed out" ||
		errResp.Code != factoryapi.ErrorResponseCode("PROVIDER_EXECUTION_TIMEOUT") {
		t.Fatalf("deadline response = %#v, want execute timeout outcome", errResp)
	}
}

func TestExecuteRootErrorResponse_MapsRequestContextFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := ExecuteRootErrorResponse(context.Canceled)
	if !ok || status != http.StatusInternalServerError ||
		string(response.Code) != "PROVIDER_EXECUTION_CANCELED" {
		t.Fatalf("canceled = (%d, %#v, %v), want handled cancel outcome", status, response, ok)
	}

	status, response, ok = ExecuteRootErrorResponse(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout ||
		string(response.Code) != "PROVIDER_EXECUTION_TIMEOUT" {
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

func TestWriteExecuteOrInternalError_MapsCanceledRequestContextToExecuteCancelOutcome(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{})
	recorder := httptest.NewRecorder()

	WriteExecuteOrInternalErrorForTest(adapter, recorder, context.Canceled)

	assertExecuteHTTPError(
		t,
		recorder,
		http.StatusInternalServerError,
		"PROVIDER_EXECUTION_CANCELED",
		"provider execution canceled",
	)
}

func TestWriteExecuteOrInternalError_MapsDeadlineExceededToExecuteTimeoutOutcome(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{})
	recorder := httptest.NewRecorder()

	WriteExecuteOrInternalErrorForTest(adapter, recorder, context.DeadlineExceeded)

	assertExecuteHTTPError(
		t,
		recorder,
		http.StatusGatewayTimeout,
		"PROVIDER_EXECUTION_TIMEOUT",
		"provider execution timed out",
	)
}

func TestAdapter_ExecuteHTTPMapsCanceledRequestContextBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			invoked = true
			return providers.ExecuteResult{}, nil
		},
	}
	adapter := NewAdapter(fake)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recorder := httptest.NewRecorder()

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(`{"attemptId":"attempt-1"}`)).WithContext(ctx),
		"codex",
	)

	if invoked {
		t.Fatal("canceled request context must end before Providers root execute")
	}
	assertExecuteHTTPError(
		t,
		recorder,
		http.StatusInternalServerError,
		"PROVIDER_EXECUTION_CANCELED",
		"provider execution canceled",
	)
}

func TestAdapter_ExecuteHTTPMapsCanceledRequestContextDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	fake := &rootFake{
		execute: func(rootCtx context.Context, _ providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, waitForProvidersRootContext(rootCtx)
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.ExecuteHTTP(
			recorder,
			httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(`{"attemptId":"attempt-1"}`)).WithContext(ctx),
			"codex",
		)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("execute handler hung after request context cancellation")
	}
	assertExecuteHTTPError(
		t,
		recorder,
		http.StatusInternalServerError,
		"PROVIDER_EXECUTION_CANCELED",
		"provider execution canceled",
	)
}

func TestAdapter_ExecuteHTTPMapsDeadlineExceededRequestContextDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	fake := &rootFake{
		execute: func(rootCtx context.Context, _ providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, waitForProvidersRootContext(rootCtx)
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.ExecuteHTTP(
			recorder,
			httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(`{"attemptId":"attempt-1"}`)).WithContext(ctx),
			"codex",
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("execute handler hung after request context timeout")
	}
	assertExecuteHTTPError(
		t,
		recorder,
		http.StatusGatewayTimeout,
		"PROVIDER_EXECUTION_TIMEOUT",
		"provider execution timed out",
	)
}

func TestAdapter_ExecuteHTTPMapsRootExecuteCancelledFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ErrExecuteCancelled
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(`{"attemptId":"attempt-1"}`)),
		"codex",
	)

	assertExecuteHTTPError(
		t,
		recorder,
		http.StatusInternalServerError,
		"PROVIDER_EXECUTION_CANCELED",
		"provider execution canceled",
	)
}

func TestAdapter_ExecuteHTTPMapsRootExecuteTimeoutFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ErrExecuteTimeout
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(`{"attemptId":"attempt-1"}`)),
		"codex",
	)

	assertExecuteHTTPError(
		t,
		recorder,
		http.StatusGatewayTimeout,
		"PROVIDER_EXECUTION_TIMEOUT",
		"provider execution timed out",
	)
}

func TestAdapter_ExecuteHTTPMapsExecuteFailureCanceledFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindCanceled,
				Message: "attempt cancelled by peer policy",
			}
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(`{"attemptId":"attempt-1"}`)),
		"codex",
	)

	assertExecuteHTTPError(
		t,
		recorder,
		http.StatusInternalServerError,
		"PROVIDER_EXECUTION_CANCELED",
		"attempt cancelled by peer policy",
	)
}

func TestAdapter_ExecuteHTTPMapsExecuteFailureTimeoutFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindTimeout,
			}
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(`{"attemptId":"attempt-1"}`)),
		"codex",
	)

	assertExecuteHTTPError(
		t,
		recorder,
		http.StatusGatewayTimeout,
		"PROVIDER_EXECUTION_TIMEOUT",
		"provider execution timed out",
	)
}

func TestAdapter_ExecuteMapsCanceledRequestContextDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	fake := &rootFake{
		execute: func(rootCtx context.Context, _ providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, waitForProvidersRootContext(rootCtx)
		},
	}
	adapter := NewAdapter(fake)

	done := make(chan error, 1)
	go func() {
		_, err := adapter.Execute(ctx, ExecuteInput{
			ProviderID: "codex",
			Body:       strings.NewReader(`{"attemptId":"attempt-1"}`),
		})
		done <- err
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Execute hung after request context cancellation")
	}
}

func TestAdapter_ExecuteMapsDeadlineExceededRequestContextDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	fake := &rootFake{
		execute: func(rootCtx context.Context, _ providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, waitForProvidersRootContext(rootCtx)
		},
	}
	adapter := NewAdapter(fake)

	done := make(chan error, 1)
	go func() {
		_, err := adapter.Execute(ctx, ExecuteInput{
			ProviderID: "codex",
			Body:       strings.NewReader(`{"attemptId":"attempt-1"}`),
		})
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Execute error = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Execute hung after request context deadline")
	}
}

func TestAdapter_ExecuteMapsRootExecuteCancelledFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ErrExecuteCancelled
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.Execute(context.Background(), ExecuteInput{
		ProviderID: "codex",
		Body:       strings.NewReader(`{"attemptId":"attempt-1"}`),
	})
	if !errors.Is(err, providers.ErrExecuteCancelled) {
		t.Fatalf("Execute error = %v, want ErrExecuteCancelled", err)
	}
}

func TestAdapter_ExecuteMapsRootExecuteTimeoutFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ErrExecuteTimeout
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.Execute(context.Background(), ExecuteInput{
		ProviderID: "codex",
		Body:       strings.NewReader(`{"attemptId":"attempt-1"}`),
	})
	if !errors.Is(err, providers.ErrExecuteTimeout) {
		t.Fatalf("Execute error = %v, want ErrExecuteTimeout", err)
	}
}

func TestExecuteRootErrorResponse_DoesNotMapCancelToInternalError(t *testing.T) {
	t.Parallel()

	status, response, ok := ExecuteRootErrorResponse(context.Canceled)
	if !ok || status != http.StatusInternalServerError {
		t.Fatalf("canceled = (%d, %#v, %v), want handled cancel outcome", status, response, ok)
	}
	if string(response.Code) == string(factoryapi.ErrorResponseCodeINTERNALERROR) {
		t.Fatalf("response = %#v, must not map cancellation to INTERNAL_ERROR", response)
	}
}
