package support

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFactoryResponseEventStreamWaitResultReportsEOF(t *testing.T) {
	var releaseOnce sync.Once
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher := writeResponseEventStreamHeaders(w)
		_, _ = fmt.Fprint(w, "id: 1\ndata: {\"eventId\":\"event-1\",\"sequence\":1}\n\n")
		flusher.Flush()
		<-release
	}))
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	t.Cleanup(server.Close)

	stream := OpenFactoryResponseEventStreamAt(t, server.URL)
	frame := stream.TryNextFrameResult(time.Second)
	if frame.Outcome != FactoryResponseEventStreamOutcomeFrame {
		t.Fatalf("first wait outcome = %q, want frame: %s", frame.Outcome, frame.Diagnostic())
	}
	if frame.StatusCode != http.StatusOK {
		t.Fatalf("first wait status = %d, want %d", frame.StatusCode, http.StatusOK)
	}

	releaseOnce.Do(func() { close(release) })
	terminal := stream.TryNextFrameResult(time.Second)
	if terminal.Outcome != FactoryResponseEventStreamOutcomeEOF {
		t.Fatalf("terminal wait outcome = %q, want EOF: %s", terminal.Outcome, terminal.Diagnostic())
	}
	if terminal.StatusCode != http.StatusOK {
		t.Fatalf("terminal status = %d, want %d", terminal.StatusCode, http.StatusOK)
	}
	if terminal.FrameCount != 1 {
		t.Fatalf("terminal frame count = %d, want 1", terminal.FrameCount)
	}
	if terminal.Waited >= time.Second {
		t.Fatalf("terminal wait = %s, want less than configured timeout", terminal.Waited)
	}
	for _, want := range []string{
		"factory response event stream closed",
		"HTTP status=200",
		"reason=EOF",
		"waited=",
		"frames=1",
	} {
		if !strings.Contains(terminal.Diagnostic(), want) {
			t.Fatalf("EOF diagnostic = %q, want substring %q", terminal.Diagnostic(), want)
		}
	}
	if strings.Contains(terminal.Diagnostic(), "timed out") {
		t.Fatalf("EOF diagnostic = %q, want no timeout wording", terminal.Diagnostic())
	}

	if _, ok := stream.TryNextFrame(time.Millisecond); ok {
		t.Fatal("legacy TryNextFrame() returned a frame after EOF")
	}
}

func TestFactoryResponseEventStreamWaitResultReportsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeResponseEventStreamHeaders(w)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	stream := OpenFactoryResponseEventStreamAt(t, server.URL)
	result := stream.TryNextFrameResult(30 * time.Millisecond)
	if result.Outcome != FactoryResponseEventStreamOutcomeTimeout {
		t.Fatalf("wait outcome = %q, want timeout: %s", result.Outcome, result.Diagnostic())
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("timeout status = %d, want %d", result.StatusCode, http.StatusOK)
	}
	if result.Waited < 20*time.Millisecond {
		t.Fatalf("timeout wait = %s, want at least 20ms", result.Waited)
	}
	for _, want := range []string{
		"timed out waiting",
		"HTTP status=200",
		"waited=",
		"frames=0",
	} {
		if !strings.Contains(result.Diagnostic(), want) {
			t.Fatalf("timeout diagnostic = %q, want substring %q", result.Diagnostic(), want)
		}
	}
	if strings.Contains(result.Diagnostic(), "stream closed") {
		t.Fatalf("timeout diagnostic = %q, want no close wording", result.Diagnostic())
	}
}

func TestFactoryResponseEventStreamWaitResultReportsDeliberateCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeResponseEventStreamHeaders(w)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	stream := OpenFactoryResponseEventStreamAt(t, server.URL)
	stream.Close()
	result := stream.TryNextFrameResult(time.Millisecond)
	if result.Outcome != FactoryResponseEventStreamOutcomeCanceled {
		t.Fatalf("wait outcome = %q, want canceled: %s", result.Outcome, result.Diagnostic())
	}
	if !strings.Contains(result.Diagnostic(), "reason=deliberate cancellation") {
		t.Fatalf("cancellation diagnostic = %q, want deliberate cancellation", result.Diagnostic())
	}
}

func TestFactoryResponseEventStreamWaitResultPreservesReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher := writeResponseEventStreamHeaders(w)
		_, _ = fmt.Fprint(w, "id: 1\ndata: not-json\n\n")
		flusher.Flush()
	}))
	t.Cleanup(server.Close)

	stream := OpenFactoryResponseEventStreamAt(t, server.URL)
	result := stream.TryNextFrameResult(time.Second)
	if result.Outcome != FactoryResponseEventStreamOutcomeReadError {
		t.Fatalf("wait outcome = %q, want read error: %s", result.Outcome, result.Diagnostic())
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "decode response-event SSE data") {
		t.Fatalf("read error = %v, want underlying decode context", result.Err)
	}
	if !strings.Contains(result.Diagnostic(), result.Err.Error()) {
		t.Fatalf("read-error diagnostic = %q, want underlying error %q", result.Diagnostic(), result.Err)
	}
}

func writeResponseEventStreamHeaders(w http.ResponseWriter) http.Flusher {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher := w.(http.Flusher)
	flusher.Flush()
	return flusher
}
