package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestStartSession_UsesDocumentedDurableSessionEndpoint(t *testing.T) {
	t.Helper()
	var gotPath string
	var gotRequest factoryapi.FactorySessionExecutionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionExecutionResponse{SessionId: "session-browser-001"})
	}))
	defer server.Close()

	workflowName := "browser-test"
	sessionID, err := startSession(context.Background(), server.URL, "async", factoryapi.FactorySessionExecutionRequest{
		RequestId: "request-browser-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &workflowName,
		},
	})
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	if gotPath != "/factory-sessions/async" {
		t.Fatalf("session request path = %q, want /factory-sessions/async", gotPath)
	}
	if gotRequest.RequestId != "request-browser-001" {
		t.Fatalf("request ID = %q", gotRequest.RequestId)
	}
	if sessionID != "session-browser-001" {
		t.Fatalf("session ID = %q", sessionID)
	}
}

func TestStartSession_ReportsPublicFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid workflow", http.StatusBadRequest)
	}))
	defer server.Close()

	_, err := startSession(context.Background(), server.URL, "sync", factoryapi.FactorySessionExecutionRequest{})
	if err == nil {
		t.Fatal("startSession succeeded for API failure")
	}
	if !strings.Contains(err.Error(), "400 Bad Request") {
		t.Fatalf("startSession error = %q, want public HTTP status", err)
	}
}
