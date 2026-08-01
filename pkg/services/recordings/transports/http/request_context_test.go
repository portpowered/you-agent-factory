package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestGetEventsBySessionId_EndsWithoutErrorWhenContextCanceledBeforeSubscribe(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		subscribeFrom: func(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			invoked = true
			return recordings.SubscribeResult{}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	assertHandlerReturnsWithin(t, time.Second, func() {
		adapter.GetEventsBySessionId(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/events", nil).WithContext(ctx),
			"session-1",
			factoryapi.GetEventsBySessionIdParams{},
		)
	})

	if invoked {
		t.Fatal("canceled request context must end before Recordings root subscribe")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map cancellation to INTERNAL_ERROR: %s", recorder.Body.String())
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

func TestGetEventsBySessionId_EndsWithoutErrorWhenSubscribeCanceledDuringFakeRoot(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		subscribeFrom: func(ctx context.Context, _ recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			<-ctx.Done()
			return recordings.SubscribeResult{}, context.Canceled
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.GetEventsBySessionId(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/events", nil).WithContext(ctx),
			"session-1",
			factoryapi.GetEventsBySessionIdParams{},
		)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscribe handler hung after request context cancellation")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map cancellation to INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestGetEventsBySessionId_EndsWithoutErrorWhenSubscribeTimesOutDuringFakeRoot(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		subscribeFrom: func(ctx context.Context, _ recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			<-ctx.Done()
			return recordings.SubscribeResult{}, context.DeadlineExceeded
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		adapter.GetEventsBySessionId(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/events", nil).WithContext(ctx),
			"session-1",
			factoryapi.GetEventsBySessionIdParams{},
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscribe handler hung after request context timeout")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map timeout to INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestGetEventsBySessionId_EndsStreamWhenContextCanceledDuringNext(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		subscribeFrom: func(_ context.Context, _ recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			return recordings.SubscribeResult{
				Subscription: recordings.EventSubscription(func(ctx context.Context) recordings.SubscriptionOutcome {
					<-ctx.Done()
					return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
				}),
			}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.GetEventsBySessionId(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/events", nil).WithContext(ctx),
			"session-1",
			factoryapi.GetEventsBySessionIdParams{},
		)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event stream hung after request context cancellation")
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("response = %d headers=%#v, want committed SSE stream", recorder.Code, recorder.Header())
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("stream must not map cancellation to INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestListFactorySessionArtifacts_EndsWithoutBodyWhenContextCanceledBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			invoked = true
			return recordings.RecordingStatusResult{}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	assertHandlerReturnsWithin(t, time.Second, func() {
		adapter.ListFactorySessionArtifacts(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/artifacts", nil).WithContext(ctx),
			"session-1",
		)
	})

	if invoked {
		t.Fatal("canceled request context must end before Recordings root artifact read")
	}
	if recorder.Body.Len() != 0 || strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response = %d %q, want no encoded body on pre-root cancellation", recorder.Code, recorder.Body.String())
	}
}

func TestListFactorySessionArtifacts_EndsWithoutErrorWhenCanceledDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(_ recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			<-ctx.Done()
			return recordings.RecordingStatusResult{}, context.Canceled
		},
	})
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.ListFactorySessionArtifacts(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/artifacts", nil).WithContext(ctx),
			"session-1",
		)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("artifact list handler hung after request context cancellation")
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response must not map cancellation to INTERNAL_ERROR: %s", recorder.Body.String())
	}
}

func TestListFactorySessionArtifacts_EndsWithoutErrorWhenTimedOutDuringFakeRoot(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(_ recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			<-ctx.Done()
			return recordings.RecordingStatusResult{}, context.DeadlineExceeded
		},
	})
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		adapter.ListFactorySessionArtifacts(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/artifacts", nil).WithContext(ctx),
			"session-1",
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("artifact list handler hung after request context timeout")
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
