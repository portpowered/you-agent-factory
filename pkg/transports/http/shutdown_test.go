package http

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
)

func TestShutdownServerAcknowledgesBeforeCancellation(t *testing.T) {
	var called atomic.Int32
	recorder := httptest.NewRecorder()
	var sawAcknowledgment atomic.Bool
	srv := NewServerWithRecordingsAndShutdown(nil, nil, nil, nil, nil, nil, zap.NewNop(), func() {
		if recorder.Code == http.StatusAccepted && recorder.Body.Len() > 0 {
			sawAcknowledgment.Store(true)
		}
		called.Add(1)
	})

	req := loopbackShutdownRequest()
	srv.Handler().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	var response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode acknowledgment: %v", err)
	}
	if response.Status != "accepted" || response.Message == "" {
		t.Fatalf("acknowledgment = %#v, want accepted status and message", response)
	}
	if !sawAcknowledgment.Load() {
		t.Fatal("shutdown cancellation ran before the acknowledgment was written")
	}
	if called.Load() != 1 {
		t.Fatalf("cancellation calls = %d, want 1", called.Load())
	}
}

func TestShutdownServerIsIdempotentAndIgnoresForwardedHeaders(t *testing.T) {
	var called atomic.Int32
	srv := NewServerWithRecordingsAndShutdown(nil, nil, nil, nil, nil, nil, zap.NewNop(), func() {
		called.Add(1)
	})
	for range 2 {
		req := loopbackShutdownRequest()
		req.Header.Set("X-Forwarded-For", "203.0.113.10")
		recorder := httptest.NewRecorder()
		srv.Handler().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
		}
	}
	if called.Load() != 1 {
		t.Fatalf("cancellation calls = %d, want idempotent one", called.Load())
	}
}

func TestShutdownServerRejectsNonLoopbackPeerAndListener(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		localAddr  net.Addr
	}{
		{name: "peer", remoteAddr: "192.0.2.10:4000", localAddr: loopbackAddr()},
		{name: "listener", remoteAddr: "127.0.0.1:4000", localAddr: &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 7437}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var called atomic.Int32
			srv := NewServerWithRecordingsAndShutdown(nil, nil, nil, nil, nil, nil, zap.NewNop(), func() {
				called.Add(1)
			})
			req := loopbackShutdownRequest()
			req.RemoteAddr = test.remoteAddr
			req = withLocalAddress(req, test.localAddr)
			recorder := httptest.NewRecorder()
			srv.Handler().ServeHTTP(recorder, req)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			if called.Load() != 0 {
				t.Fatal("rejected shutdown invoked cancellation")
			}
		})
	}
}

func TestShutdownServerReportsUnavailableControl(t *testing.T) {
	srv := NewServerWithRecordingsAndShutdown(nil, nil, nil, nil, nil, nil, zap.NewNop(), nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, loopbackShutdownRequest())
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
}

func loopbackShutdownRequest() *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	req.RemoteAddr = "127.0.0.1:4000"
	return withLocalAddress(req, loopbackAddr())
}

func loopbackAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7437}
}

func withLocalAddress(req *http.Request, address net.Addr) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, address))
}
