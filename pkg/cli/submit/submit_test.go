package submit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestSubmit_MissingWorkTypeName(t *testing.T) {
	err := Submit(SubmitConfig{Name: "submit-task", Payload: "some-file.json", Port: 8080})
	if err == nil {
		t.Fatal("expected error for missing work type name")
	}
	if err.Error() != "--work-type-name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSubmit_MissingPayload(t *testing.T) {
	err := Submit(SubmitConfig{Name: "submit-task", WorkTypeName: "task", Port: 8080})
	if err == nil {
		t.Fatal("expected error for missing payload")
	}
	if err.Error() != "--payload is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSubmit_PayloadFileNotFound(t *testing.T) {
	err := Submit(SubmitConfig{Name: "submit-task", WorkTypeName: "task", Payload: "/nonexistent/file.json", Port: 8080})
	if err == nil {
		t.Fatal("expected error for missing payload file")
	}
}

func TestSubmit_MissingName(t *testing.T) {
	err := Submit(SubmitConfig{WorkTypeName: "task", Payload: "some-file.json", Port: 8080})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if err.Error() != "--name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSubmit_BlankName(t *testing.T) {
	err := Submit(SubmitConfig{Name: "   ", WorkTypeName: "task", Payload: "some-file.json", Port: 8080})
	if err == nil {
		t.Fatal("expected error for blank name")
	}
	if err.Error() != "--name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this CLI boundary test keeps the JSON request contract, headers, and response assertions in one flow.
func TestSubmit_JSONPayloadPostsWorkTypeName(t *testing.T) {
	// Start a mock server that validates the request and returns success.
	var receivedReq factoryapi.SubmitWorkRequest
	var rawReq map[string]json.RawMessage
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost || r.URL.Path != "/factory-sessions/~default/work" {
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
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{TraceId: "test-trace-1"}); err != nil {
			t.Errorf("encode submit response: %v", err)
		}
	}))
	defer srv.Close()

	port := mustServerPort(t, srv.URL)

	// Create a JSON payload file.
	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"test task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Submit(SubmitConfig{
		Name:         "  CLI JSON submit  ",
		WorkTypeName: "code-change",
		Payload:      payloadPath,
		Port:         port,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if receivedReq.Name != "CLI JSON submit" {
		t.Errorf("Name = %q, want %q", receivedReq.Name, "CLI JSON submit")
	}
	if receivedReq.WorkTypeName != "code-change" {
		t.Errorf("WorkTypeName = %q, want %q", receivedReq.WorkTypeName, "code-change")
	}
	if gotPath != "/factory-sessions/~default/work" {
		t.Fatalf("path = %q, want /factory-sessions/~default/work", gotPath)
	}
	if _, ok := rawReq["name"]; !ok {
		t.Fatalf("request should include name, got keys %#v", rawReq)
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

func TestSubmit_SessionScopedRouteUsesFactorySessionPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{TraceId: "scoped-trace-1"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	parsedURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	port, err := strconv.Atoi(parsedURL.Port())
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"scoped task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err = Submit(SubmitConfig{
		Name:         "scoped-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Port:         port,
		SessionID:    "session-beta",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if gotPath != "/factory-sessions/session-beta/work" {
		t.Fatalf("path = %q, want /factory-sessions/session-beta/work", gotPath)
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
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{TraceId: "md-trace-1"}); err != nil {
			t.Errorf("encode markdown submit response: %v", err)
		}
	}))
	defer srv.Close()

	port := mustServerPort(t, srv.URL)

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "request.md")
	mdContent := "# Fix Bug\n\nPlease fix the login page."
	if err := os.WriteFile(payloadPath, []byte(mdContent), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Submit(SubmitConfig{
		Name:         "markdown-submit",
		WorkTypeName: "prd",
		Payload:      payloadPath,
		Port:         port,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if receivedReq.Name != "markdown-submit" {
		t.Errorf("Name = %q, want %q", receivedReq.Name, "markdown-submit")
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
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Message: "workTypeName is required", Code: "BAD_REQUEST"}); err != nil {
			t.Errorf("encode error response: %v", err)
		}
	}))
	defer srv.Close()

	port := mustServerPort(t, srv.URL)

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Submit(SubmitConfig{
		Name:         "task-submit",
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
		Name:         "task-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Port:         19999,
	})
	if err == nil {
		t.Fatal("expected error when factory is not running")
	}
}

func mustServerPort(t *testing.T, rawURL string) int {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse server url %q: %v", rawURL, err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse server port from %q: %v", rawURL, err)
	}
	return port
}
