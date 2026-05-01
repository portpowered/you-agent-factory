package submit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
)

func TestSubmit_MissingWorkTypeName(t *testing.T) {
	err := Submit(SubmitConfig{Payload: "some-file.json", Port: 8080})
	if err == nil {
		t.Fatal("expected error for missing work type name")
	}
	if err.Error() != "--work-type-name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSubmit_MissingPayload(t *testing.T) {
	err := Submit(SubmitConfig{WorkTypeName: "task", Port: 8080})
	if err == nil {
		t.Fatal("expected error for missing payload")
	}
	if err.Error() != "--payload is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSubmit_PayloadFileNotFound(t *testing.T) {
	err := Submit(SubmitConfig{WorkTypeName: "task", Payload: "/nonexistent/file.json", Port: 8080})
	if err == nil {
		t.Fatal("expected error for missing payload file")
	}
}

func TestSubmit_JSONPayloadPostsWorkTypeName(t *testing.T) {
	// Start a mock server that validates the request and returns success.
	var receivedReq factoryapi.SubmitWorkRequest
	var rawReq map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/work" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&rawReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body, err := json.Marshal(rawReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &receivedReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{TraceId: "test-trace-1"})
	}))
	defer srv.Close()

	// Extract port from test server URL.
	var port int
	fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port)

	// Create a JSON payload file.
	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"test task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Submit(SubmitConfig{
		WorkTypeName: "code-change",
		Payload:      payloadPath,
		Port:         port,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if receivedReq.WorkTypeName != "code-change" {
		t.Errorf("WorkTypeName = %q, want %q", receivedReq.WorkTypeName, "code-change")
	}
	if _, ok := rawReq["workTypeName"]; !ok {
		t.Fatalf("request should include workTypeName, got keys %#v", rawReq)
	}
	if _, ok := rawReq["work_type_id"]; ok {
		t.Fatalf("request should not include work_type_id, got %#v", rawReq)
	}
	// Payload should be the raw JSON from the file.
	payload, err := json.Marshal(receivedReq.Payload)
	if err != nil {
		t.Fatalf("marshal received payload: %v", err)
	}
	if string(payload) != `{"title":"test task"}` {
		t.Errorf("Payload = %s, want %s", string(payload), `{"title":"test task"}`)
	}
}

func TestSubmit_MarkdownPayload(t *testing.T) {
	var receivedReq factoryapi.SubmitWorkRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{TraceId: "md-trace-1"})
	}))
	defer srv.Close()

	var port int
	fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port)

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "request.md")
	mdContent := "# Fix Bug\n\nPlease fix the login page."
	if err := os.WriteFile(payloadPath, []byte(mdContent), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Submit(SubmitConfig{
		WorkTypeName: "prd",
		Payload:      payloadPath,
		Port:         port,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if receivedReq.WorkTypeName != "prd" {
		t.Errorf("WorkTypeName = %q, want %q", receivedReq.WorkTypeName, "prd")
	}
	// Markdown payload should be JSON-encoded as a string.
	decoded, ok := receivedReq.Payload.(string)
	if !ok {
		t.Fatalf("payload should be a JSON string, got %T", receivedReq.Payload)
	}
	if decoded != mdContent {
		t.Errorf("decoded payload = %q, want %q", decoded, mdContent)
	}
}

func TestSubmit_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Message: "workTypeName is required", Code: "BAD_REQUEST"})
	}))
	defer srv.Close()

	var port int
	fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port)

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Submit(SubmitConfig{
		WorkTypeName: "task",
		Payload:      payloadPath,
		Port:         port,
	})
	if err == nil {
		t.Fatal("expected error for server error response")
	}
	if got := err.Error(); got != "submission failed (400): workTypeName is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestSubmit_FactoryNotRunning(t *testing.T) {
	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Use a port that nothing is listening on.
	err := Submit(SubmitConfig{
		WorkTypeName: "task",
		Payload:      payloadPath,
		Port:         19999,
	})
	if err == nil {
		t.Fatal("expected error when factory is not running")
	}
}
