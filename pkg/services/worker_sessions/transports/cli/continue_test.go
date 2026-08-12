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

func TestContinueLocalAsyncReturnsLineageAfterAdmissionWithoutStreaming(t *testing.T) {
	boundary := &invokeLocalFake{continueResult: workersessions.ContinueResult{
		RequestID: "continue-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session",
		Session: workersessions.Session{ID: "successor-session", State: workersessions.StateRunning},
	}}
	var output bytes.Buffer
	err := NewContinue(nil, boundary)(ContinueConfig{
		Context: context.Background(), Output: &output, OutputFormat: "json", Async: true,
		RequestID: "continue-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session",
		FollowUpInput: "continue the work",
	})
	if err != nil {
		t.Fatalf("local async continue error = %v", err)
	}
	if boundary.streamCalls != 0 {
		t.Fatalf("async observation stream calls = %d, want 0", boundary.streamCalls)
	}
	if len(boundary.continueRequests) != 1 {
		t.Fatalf("continuation request count = %d, want 1", len(boundary.continueRequests))
	}
	request := boundary.continueRequests[0]
	if request.RequestID != "continue-request" || request.SourceWorkerSessionID != "source-session" ||
		request.SuccessorWorkerSessionID != "successor-session" || request.FollowUpInput != "continue the work" {
		t.Fatalf("continuation request = %#v, want exact lineage and input", request)
	}
	var result continueResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode local async result: %v; output=%q", err, output.String())
	}
	if !result.Accepted || result.SourceWorkerSessionID != "source-session" || result.SuccessorWorkerSessionID != "successor-session" || result.State != "RUNNING" {
		t.Fatalf("local async result = %#v, want admitted lineage/state", result)
	}
}

func TestContinueLocalWaitsForSuccessorTerminalOutput(t *testing.T) {
	boundary := &invokeLocalFake{
		continueResult: workersessions.ContinueResult{SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session", Session: workersessions.Session{ID: "successor-session", State: workersessions.StateRunning}},
		deliveries: []workersessions.ObservationDelivery{
			invokeObservationDelivery(workersessions.ObservationDeliveryRecord, "worker.session.running", `{"state":"RUNNING"}`),
			invokeObservationDelivery(workersessions.ObservationDeliveryTerminal, "worker.session.completed", `{"state":"COMPLETED","output":"continued output"}`),
		},
	}
	var output bytes.Buffer
	err := NewContinue(nil, boundary)(ContinueConfig{
		Context: context.Background(), Output: &output, OutputFormat: "json",
		SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session", FollowUpInput: "finish the continuation",
	})
	if err != nil {
		t.Fatalf("local synchronous continue error = %v", err)
	}
	var result continueResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode local result: %v; output=%q", err, output.String())
	}
	if result.State != "COMPLETED" || result.Output != "continued output" || result.SuccessorWorkerSessionID != "successor-session" {
		t.Fatalf("local result = %#v, want terminal successor output", result)
	}
	if boundary.streamCalls != 1 || !boundary.closed {
		t.Fatalf("local observation calls = %d, closed=%t, want one closed stream", boundary.streamCalls, boundary.closed)
	}
}

func TestContinueRemoteUsesExactSourceRouteAndDoesNotFallback(t *testing.T) {
	var received factoryapi.WorkerSessionContinueRequest
	var postCount, getCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/worker-sessions/source-session/continue" {
			t.Fatalf("unexpected remote request %s %s", r.Method, r.URL.Path)
		}
		postCount++
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode continuation request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionContinueResponse{
			RequestId: "continue-request", SourceWorkerSessionId: "source-session", SuccessorWorkerSessionId: "successor-session",
			PredecessorWorkerSessionId: "source-session", Accepted: true,
			State: factoryapi.WorkerSessionContinueResponseStateRunning, EventTopic: "worker-session/successor-session/events",
		})
		if getCount != 0 {
			t.Errorf("async continuation opened an observation stream")
		}
	}))
	defer server.Close()

	boundary := &invokeLocalFake{}
	var output bytes.Buffer
	err := NewContinue(testHTTPProtocol(t), boundary)(ContinueConfig{
		Context: context.Background(), Server: server.URL, Remote: true, Output: &output, OutputFormat: "json", Async: true,
		RequestID: "continue-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session", FollowUpInput: "remote follow up",
	})
	if err != nil {
		t.Fatalf("remote async continue error = %v", err)
	}
	if postCount != 1 || getCount != 0 || len(boundary.continueRequests) != 0 {
		t.Fatalf("remote/local calls = POST:%d GET:%d local:%d, want POST:1 GET:0 local:0", postCount, getCount, len(boundary.continueRequests))
	}
	if received.RequestId != "continue-request" || received.SuccessorWorkerSessionId != "successor-session" || received.FollowUpInput != "remote follow up" {
		t.Fatalf("remote request = %#v, want supplied continuation tuple", received)
	}
	var result continueResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode remote result: %v; output=%q", err, output.String())
	}
	if result.SourceWorkerSessionID != "source-session" || result.SuccessorWorkerSessionID != "successor-session" || result.State != "RUNNING" {
		t.Fatalf("remote result = %#v, want admitted lineage", result)
	}
}

func TestContinueRemoteWaitsOnSuccessorEventRoute(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionContinueResponse{
				RequestId: "continue-request", SourceWorkerSessionId: "source-session", SuccessorWorkerSessionId: "successor-session",
				PredecessorWorkerSessionId: "source-session", Accepted: true, State: factoryapi.WorkerSessionContinueResponseStateRunning,
			})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"delivery\":\"TERMINAL\",\"event\":{\"schemaId\":\"worker.session.completed\",\"payload\":{\"state\":\"COMPLETED\",\"output\":\"remote continued output\"}}}\n\n")
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewContinue(testHTTPProtocol(t), nil)(ContinueConfig{
		Context: context.Background(), Server: server.URL, Remote: true, Output: &output, OutputFormat: "json",
		RequestID: "continue-request", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session", FollowUpInput: "remote wait",
	})
	if err != nil {
		t.Fatalf("remote synchronous continue error = %v", err)
	}
	if len(paths) != 2 || paths[0] != "/worker-sessions/source-session/continue" || paths[1] != "/worker-sessions/successor-session/events" {
		t.Fatalf("remote paths = %#v, want source continuation then successor events", paths)
	}
	var result continueResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode remote terminal result: %v; output=%q", err, output.String())
	}
	if result.State != "COMPLETED" || result.Output != "remote continued output" || result.SuccessorWorkerSessionID != "successor-session" {
		t.Fatalf("remote terminal result = %#v, want successor output and lineage", result)
	}
}

func TestContinueRemoteFailureDoesNotFallbackToLocal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"message":"admission unavailable","family":"UNAVAILABLE","code":"WORKER_SESSION_CONTINUATION_ADMISSION_FAILED"}`)
	}))
	defer server.Close()

	boundary := &invokeLocalFake{}
	var output bytes.Buffer
	err := NewContinue(testHTTPProtocol(t), boundary)(ContinueConfig{
		Context: context.Background(), Server: server.URL, Remote: true, Output: &output, OutputFormat: "json", Async: true,
		SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session", FollowUpInput: "remote failure",
	})
	if err == nil {
		t.Fatal("remote continuation failure = nil, want admission error")
	}
	if len(boundary.continueRequests) != 0 {
		t.Fatal("remote failure fell back to local continuation")
	}
	if !strings.Contains(err.Error(), "admission unavailable") {
		t.Fatalf("remote failure = %v, want server error", err)
	}
}
