package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func assertRetainedTerminalFrames(t *testing.T, recorder *httptest.ResponseRecorder, service *fakeObservationService) {
	t.Helper()
	assertSSESuccess(t, recorder)
	frames := decodeSSEFrames(t, recorder.Body.String())
	assertSSEFrameKinds(t, frames)
	assertSSEFrameIdentity(t, frames[0])
	assertSSEFramePayload(t, frames[0])
	assertSSESubscriptionClosed(t, service)
}

func assertSSESuccess(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}
}

func assertSSEFrameKinds(t *testing.T, frames []sseTestFrame) {
	t.Helper()
	if len(frames) != 2 || frames[0].Delivery != "RECORD" || frames[1].Delivery != "TERMINAL" {
		t.Fatalf("frames = %#v, want RECORD then TERMINAL", frames)
	}
}

func assertSSEFrameIdentity(t *testing.T, frame sseTestFrame) {
	t.Helper()
	if frame.WorkerSessionID != "worker-session-1" || frame.ProviderSession == nil || frame.ProviderSession.Id != "provider-session-1" {
		t.Fatalf("frame identity = %#v, want exact worker/provider identity", frame)
	}
}

func assertSSEFramePayload(t *testing.T, frame sseTestFrame) {
	t.Helper()
	if frame.Event == nil || frame.Event.Position != 1 || string(frame.Event.Payload) != `{"state":"RUNNING"}` {
		t.Fatalf("first event = %#v, want canonical event payload", frame.Event)
	}
	if frame.Event.Cursor == nil || frame.Event.Cursor.Position != 1 {
		t.Fatalf("first event cursor = %#v, want position 1", frame.Event.Cursor)
	}
}

func assertSSESubscriptionClosed(t *testing.T, service *fakeObservationService) {
	t.Helper()
	if service.streamSubscription == nil || !service.streamSubscription.closed {
		t.Fatal("stream subscription was not closed after terminal delivery")
	}
	if service.streamRequest.Limit != workersessions.DefaultObservationStreamLimit {
		t.Fatalf("stream limit = %d, want stable default %d", service.streamRequest.Limit, workersessions.DefaultObservationStreamLimit)
	}
}
