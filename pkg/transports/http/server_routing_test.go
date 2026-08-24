package http

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	"go.uber.org/zap"
)

func TestUnknownRouteReturnsStructuredNotFound(t *testing.T) {
	srv := newFactoryDefinitionTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/functional-routing-unknown-route", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "route not found")
}

func TestWrongMethodReturnsDocumentedMethodError(t *testing.T) {
	srv := newFactoryDefinitionTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func TestServerWithRecordings_RoutesDurableResultToRecordingsAdapter(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-server-recordings-001"
	root := &serverRecordingsRoot{}
	srv := NewServerWithRecordings(
		recordingshttp.NewAdapter(root),
		nil, nil, nil, nil, nil, zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/results", nil),
	)

	if recorder.Code != http.StatusOK || !root.queried ||
		!containsJSONField(recorder.Body.String(), "sessionId", sessionID) {
		t.Fatalf("response = %d %s, queried=%v, want Recordings-owned result", recorder.Code, recorder.Body.String(), root.queried)
	}
}

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

	srv.Handler().ServeHTTP(recorder, loopbackShutdownRequest())

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

func TestShutdownServerInvokesInjectedOperationForEachAcceptedRequest(t *testing.T) {
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
	if called.Load() != 2 {
		t.Fatalf("cancellation calls = %d, want injected operation called twice", called.Load())
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

func containsJSONField(body, field, value string) bool {
	return strings.Contains(body, `"`+field+`":"`+value+`"`)
}

type serverRecordingsRoot struct {
	recordings.Service
	queried bool
}

func (root *serverRecordingsRoot) QueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
		RecordingID: request.RecordingID,
		Artifact:    "artifact-server-recordings-001",
		State:       recordings.RecordingFinalized,
	}}, nil
}

func (root *serverRecordingsRoot) QueryHistoricalRecording(
	request recordings.HistoricalRecordingQueryRequest,
) (recordings.HistoricalRecordingQueryResult, error) {
	root.queried = true
	return recordings.HistoricalRecordingQueryResult{
		Recording: request.Recording,
		Status: recordings.RecordingStatusFacts{
			RecordingID: request.Recording.RecordingID,
			State:       recordings.RecordingFinalized,
		},
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Payload:       `{}`,
		},
	}, nil
}
