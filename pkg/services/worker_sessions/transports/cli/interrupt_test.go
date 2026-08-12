package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestInterruptLocalAsyncReturnsAdmissionSnapshotsWithoutStreaming(t *testing.T) {
	boundary := &invokeLocalFake{interruptResult: workersessions.InterruptResult{
		RequestID: "interrupt-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session",
		Phase: workersessions.InterruptPhaseSuccessorAdmission, Accepted: true,
		Source:    workersessions.Session{ID: "source-session", State: workersessions.StateCanceled},
		Successor: workersessions.Session{ID: "successor-session", State: workersessions.StateRunning},
	}}
	var output bytes.Buffer
	err := NewInterrupt(nil, boundary)(InterruptConfig{
		Context: context.Background(), Output: &output, OutputFormat: "json", Async: true,
		RequestID: "interrupt-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session",
		ReplacementMessage: "take a different path",
	})
	if err != nil {
		t.Fatalf("local async interrupt error = %v", err)
	}
	if boundary.streamCalls != 0 {
		t.Fatalf("async observation stream calls = %d, want 0", boundary.streamCalls)
	}
	if len(boundary.interruptRequests) != 1 {
		t.Fatalf("interrupt request count = %d, want 1", len(boundary.interruptRequests))
	}
	request := boundary.interruptRequests[0]
	if request.RequestID != "interrupt-request" || request.SourceWorkerSessionID != "source-session" ||
		request.SuccessorWorkerSessionID != "successor-session" || request.ReplacementMessage != "take a different path" {
		t.Fatalf("interrupt request = %#v, want exact source/successor/input", request)
	}
	var result interruptResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode local async result: %v; output=%q", err, output.String())
	}
	if !result.Accepted || result.Phase != string(workersessions.InterruptPhaseSuccessorAdmission) ||
		result.Source.State != "CANCELED" || result.Successor.State != "RUNNING" {
		t.Fatalf("local async result = %#v, want phase-aware admission snapshots", result)
	}
}

func TestInterruptLocalWaitsForSuccessorTerminalOutput(t *testing.T) {
	boundary := &invokeLocalFake{
		interruptResult: workersessions.InterruptResult{
			RequestID: "interrupt-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session",
			Phase: workersessions.InterruptPhaseSuccessorAdmission, Accepted: true,
			Source:    workersessions.Session{ID: "source-session", State: workersessions.StateCanceled},
			Successor: workersessions.Session{ID: "successor-session", State: workersessions.StateRunning},
		},
		deliveries: []workersessions.ObservationDelivery{
			invokeObservationDelivery(workersessions.ObservationDeliveryRecord, "worker.session.running", `{"state":"RUNNING"}`),
			invokeObservationDelivery(workersessions.ObservationDeliveryTerminal, "worker.session.completed", `{"state":"COMPLETED","output":"replacement output"}`),
		},
	}
	var output bytes.Buffer
	err := NewInterrupt(nil, boundary)(InterruptConfig{
		Context: context.Background(), Output: &output, OutputFormat: "json",
		RequestID: "interrupt-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session",
		ReplacementMessage: "replace the work",
	})
	if err != nil {
		t.Fatalf("local synchronous interrupt error = %v", err)
	}
	var result interruptResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode local result: %v; output=%q", err, output.String())
	}
	if result.Successor.State != "COMPLETED" || result.Output != "replacement output" {
		t.Fatalf("local result = %#v, want terminal successor output", result)
	}
	if boundary.streamCalls != 1 || !boundary.closed {
		t.Fatalf("local observation calls = %d, closed=%t, want one closed stream", boundary.streamCalls, boundary.closed)
	}
}

func TestInterruptRemoteUsesExactSourceRouteAndDoesNotFallback(t *testing.T) {
	var received factoryapi.WorkerSessionInterruptRequest
	var postCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/worker-sessions/source-session/interrupt" {
			t.Fatalf("unexpected remote request %s %s", r.Method, r.URL.Path)
		}
		postCount++
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode interrupt request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionInterruptResponse{
			RequestId: "interrupt-request", SourceWorkerSessionId: "source-session", SuccessorWorkerSessionId: "successor-session",
			Phase: factoryapi.WorkerSessionInterruptResponsePhaseSuccessorAdmission, Accepted: true,
			Source: factoryapi.WorkerSessionInterruptSnapshot{
				WorkerSessionId: "source-session", State: factoryapi.WorkerSessionInterruptSnapshotStateCanceled,
				EventTopic: "worker-session/source-session/events",
			},
			Successor: factoryapi.WorkerSessionInterruptSnapshot{
				WorkerSessionId: "successor-session", State: factoryapi.WorkerSessionInterruptSnapshotStateRunning,
				EventTopic: "worker-session/successor-session/events",
			},
		})
	}))
	defer server.Close()

	boundary := &invokeLocalFake{}
	var output bytes.Buffer
	err := NewInterrupt(testHTTPProtocol(t), boundary)(InterruptConfig{
		Context: context.Background(), Server: server.URL, Remote: true, Output: &output, OutputFormat: "json", Async: true,
		RequestID: "interrupt-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session",
		ReplacementMessage: "remote replacement",
	})
	if err != nil {
		t.Fatalf("remote async interrupt error = %v", err)
	}
	if postCount != 1 || len(boundary.interruptRequests) != 0 {
		t.Fatalf("remote/local calls = POST:%d local:%d, want POST:1 local:0", postCount, len(boundary.interruptRequests))
	}
	if received.RequestId != "interrupt-request" || received.SuccessorWorkerSessionId != "successor-session" || received.ReplacementMessage != "remote replacement" {
		t.Fatalf("remote request = %#v, want supplied interrupt tuple", received)
	}
	var result interruptResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode remote result: %v; output=%q", err, output.String())
	}
	if result.SourceWorkerSessionID != "source-session" || result.SuccessorWorkerSessionID != "successor-session" || result.Source.State != "CANCELED" {
		t.Fatalf("remote result = %#v, want admitted source/successor snapshots", result)
	}
}

func TestInterruptRemoteFailureIncludesPhaseAndDoesNotFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"message":"source cancellation failed","family":"UNAVAILABLE","code":"WORKER_SESSION_INTERRUPT_SOURCE_CANCELLATION_FAILED","phase":"SOURCE_CANCELLATION"}`)
	}))
	defer server.Close()

	boundary := &invokeLocalFake{}
	var output bytes.Buffer
	err := NewInterrupt(testHTTPProtocol(t), boundary)(InterruptConfig{
		Context: context.Background(), Server: server.URL, Remote: true, Output: &output, OutputFormat: "json", Async: true,
		RequestID: "interrupt-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session",
		ReplacementMessage: "remote failure",
	})
	if err == nil {
		t.Fatal("remote interrupt failure = nil, want source cancellation error")
	}
	if len(boundary.interruptRequests) != 0 {
		t.Fatal("remote failure fell back to local interrupt")
	}
	if !strings.Contains(err.Error(), "source cancellation failed") {
		t.Fatalf("remote failure = %v, want server error", err)
	}
	var payload struct {
		Code  string `json:"code"`
		Phase string `json:"phase"`
	}
	if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode remote error payload: %v; output=%q", decodeErr, output.String())
	}
	if payload.Code != "WORKER_SESSION_INTERRUPT_SOURCE_CANCELLATION_FAILED" || payload.Phase != "SOURCE_CANCELLATION" {
		t.Fatalf("remote error payload = %#v, want stable code and phase", payload)
	}
}

func TestInterruptRemoteRejectsMismatchedAdmissionWithoutFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionInterruptResponse{
			RequestId: "interrupt-request", SourceWorkerSessionId: "source-session", SuccessorWorkerSessionId: "other-successor",
			Phase: factoryapi.WorkerSessionInterruptResponsePhaseSuccessorAdmission, Accepted: true,
			Source: factoryapi.WorkerSessionInterruptSnapshot{
				WorkerSessionId: "source-session", State: factoryapi.WorkerSessionInterruptSnapshotStateCanceled, EventTopic: "worker-session/source-session/events",
			},
			Successor: factoryapi.WorkerSessionInterruptSnapshot{
				WorkerSessionId: "other-successor", State: factoryapi.WorkerSessionInterruptSnapshotStateRunning, EventTopic: "worker-session/other-successor/events",
			},
		})
	}))
	defer server.Close()

	boundary := &invokeLocalFake{}
	var output bytes.Buffer
	err := NewInterrupt(testHTTPProtocol(t), boundary)(InterruptConfig{
		Context: context.Background(), Server: server.URL, Remote: true, Output: &output, OutputFormat: "json", Async: true,
		RequestID: "interrupt-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session",
		ReplacementMessage: "remote replacement",
	})
	if err == nil {
		t.Fatal("mismatched remote interrupt response = nil, want response error")
	}
	if len(boundary.interruptRequests) != 0 {
		t.Fatal("mismatched remote response fell back to local interrupt")
	}
	var payload struct {
		Code  string `json:"code"`
		Phase string `json:"phase"`
	}
	if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode mismatched response payload: %v; output=%q", decodeErr, output.String())
	}
	if payload.Code != "WORKER_SESSION_INTERRUPT_RESPONSE_INVALID" || payload.Phase != interruptPhaseResponse {
		t.Fatalf("mismatched response payload = %#v, want response classification", payload)
	}
}
