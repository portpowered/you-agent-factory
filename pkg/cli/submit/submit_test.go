package submit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestSubmit_MissingWorkTypeName(t *testing.T) {
	err := Submit(SubmitConfig{Name: "submit-task", Payload: "some-file.json", Server: "http://127.0.0.1:8080"})
	if err == nil {
		t.Fatal("expected error for missing work type name")
	}
	if err.Error() != "--work-type-name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSubmit_VerboseLogsRequestAndResponseMetadataWithoutPayloadContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{TraceId: "trace-verbose"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte("# Secret prompt\n\nDo not log this body."), 0o644); err != nil {
		t.Fatal(err)
	}

	var diagnostics bytes.Buffer
	err := Submit(SubmitConfig{
		Name:         "verbose-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server: mustServerBase(t, srv.URL),
		SessionID:    "session-alpha",
		Verbose:      true,
		Diagnostics:  &diagnostics,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	got := diagnostics.String()
	for _, want := range []string{
		"submit request",
		"endpointPath=/factory-sessions/session-alpha/work",
		"server=",
		"session=session-alpha",
		"payloadPath=" + payloadPath,
		"payloadType=markdown",
		"payloadBytes=38",
		`requestName="verbose-submit"`,
		`workTypeName="task"`,
		"submit response",
		"status=201",
		"traceId=trace-verbose",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"Secret prompt", "Do not log this body"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("diagnostics leaked payload content %q:\n%s", forbidden, got)
		}
	}
}

func TestSubmit_VerboseLogsJSONPayloadMetadataWithoutPayloadContentOrToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{TraceId: "trace-json"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	payloadPath := filepath.Join(t.TempDir(), "work.json")
	payload := []byte(`{"title":"deploy task","prompt":"full JSON work payload must stay private","accessToken":"ghp_successPathToken1234567890"}`)
	if err := os.WriteFile(payloadPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	var diagnostics bytes.Buffer
	err := Submit(SubmitConfig{
		Name:         "json-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server: mustServerBase(t, srv.URL),
		Verbose:      true,
		Diagnostics:  &diagnostics,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	got := diagnostics.String()
	for _, want := range []string{
		"payloadType=json",
		"payloadBytes=" + strconv.Itoa(len(payload)),
		`requestName="json-submit"`,
		`workTypeName="task"`,
		"traceId=trace-json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"deploy task", "full JSON work payload", "ghp_successPathToken1234567890"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("diagnostics leaked JSON payload or token %q:\n%s", forbidden, got)
		}
	}
}

func TestSubmit_VerboseLogsFailureStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Message: "workTypeName is required", Code: "BAD_REQUEST"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	payloadPath := filepath.Join(t.TempDir(), "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"test task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var diagnostics bytes.Buffer
	err := Submit(SubmitConfig{
		Name:         "task-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server: mustServerBase(t, srv.URL),
		Verbose:      true,
		Diagnostics:  &diagnostics,
	})
	if err == nil {
		t.Fatal("expected submit failure")
	}
	got := diagnostics.String()
	if !strings.Contains(got, "submit response") || !strings.Contains(got, "status=400") {
		t.Fatalf("diagnostics missing failure status:\n%s", got)
	}
	if strings.Contains(got, "test task") {
		t.Fatalf("diagnostics leaked payload content:\n%s", got)
	}
}

func TestSubmit_MissingPayload(t *testing.T) {
	err := Submit(SubmitConfig{Name: "submit-task", WorkTypeName: "task", Server: "http://127.0.0.1:8080"})
	if err == nil {
		t.Fatal("expected error for missing payload")
	}
	if err.Error() != "--payload is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSubmit_PayloadFileNotFound(t *testing.T) {
	err := Submit(SubmitConfig{Name: "submit-task", WorkTypeName: "task", Payload: "/nonexistent/file.json", Server: "http://127.0.0.1:8080"})
	if err == nil {
		t.Fatal("expected error for missing payload file")
	}
}

func TestSubmit_MissingName(t *testing.T) {
	err := Submit(SubmitConfig{WorkTypeName: "task", Payload: "some-file.json", Server: "http://127.0.0.1:8080"})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if err.Error() != "--name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSubmit_BlankName(t *testing.T) {
	err := Submit(SubmitConfig{Name: "   ", WorkTypeName: "task", Payload: "some-file.json", Server: "http://127.0.0.1:8080"})
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

	server := mustServerBase(t, srv.URL)

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
		Server:       server,
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

func TestSubmit_JSONStdoutEmitsStableSuccessEnvelope(t *testing.T) {
	workID := "batch-req-1-json-submit"
	name := "json-submit"
	workType := "task"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{
			TraceId:      "json-trace-1",
			WorkId:       &workID,
			Name:         &name,
			WorkTypeName: &workType,
		}); err != nil {
			t.Errorf("encode submit response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"json stdout task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Submit(SubmitConfig{
		Name:         "json-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       mustServerBase(t, srv.URL),
		JSON:         true,
		Output:       &out,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	var envelope SubmitSuccessResult
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not valid submit success JSON: %v\n%s", err, out.String())
	}
	if envelope.WorkID == nil || *envelope.WorkID != workID {
		t.Fatalf("workId = %v, want %q", envelope.WorkID, workID)
	}
	if envelope.Name != name {
		t.Fatalf("name = %q, want %q", envelope.Name, name)
	}
	if envelope.WorkTypeName != workType {
		t.Fatalf("workTypeName = %q, want %q", envelope.WorkTypeName, workType)
	}
	if envelope.TraceID != "json-trace-1" {
		t.Fatalf("traceId = %q, want json-trace-1", envelope.TraceID)
	}
	if envelope.SessionID != "~default" {
		t.Fatalf("sessionId = %q, want ~default", envelope.SessionID)
	}
	if envelope.EndpointPath != "/factory-sessions/~default/work" {
		t.Fatalf("endpointPath = %q, want /factory-sessions/~default/work", envelope.EndpointPath)
	}
	for _, forbidden := range []string{"requestId", "accepted"} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatalf("stdout leaked API field %q:\n%s", forbidden, out.String())
		}
	}
}

func TestSubmit_JSONStdoutEmitsSessionScopedEndpointPath(t *testing.T) {
	sessionID := "session-beta"
	traceID := "scoped-json-trace"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/session-beta/work" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{
			TraceId:   traceID,
			SessionId: &sessionID,
		}); err != nil {
			t.Errorf("encode submit response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"scoped json task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Submit(SubmitConfig{
		Name:         "scoped-json-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       mustServerBase(t, srv.URL),
		SessionID:    sessionID,
		JSON:         true,
		Output:       &out,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	var envelope SubmitSuccessResult
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not valid submit success JSON: %v\n%s", err, out.String())
	}
	if envelope.TraceID != traceID {
		t.Fatalf("traceId = %q, want %q", envelope.TraceID, traceID)
	}
	if envelope.SessionID != sessionID {
		t.Fatalf("sessionId = %q, want %q", envelope.SessionID, sessionID)
	}
	if envelope.EndpointPath != "/factory-sessions/session-beta/work" {
		t.Fatalf("endpointPath = %q, want /factory-sessions/session-beta/work", envelope.EndpointPath)
	}
	if envelope.Name != "scoped-json-submit" {
		t.Fatalf("name = %q, want scoped-json-submit", envelope.Name)
	}
	if envelope.WorkTypeName != "task" {
		t.Fatalf("workTypeName = %q, want task", envelope.WorkTypeName)
	}
}

func TestSubmit_JSONStdoutEncodesNullWorkIdWhenAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{TraceId: "json-trace-no-work-id"}); err != nil {
			t.Errorf("encode submit response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"json stdout task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Submit(SubmitConfig{
		Name:         "json-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       mustServerBase(t, srv.URL),
		JSON:         true,
		Output:       &out,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := raw["workId"]; !ok {
		t.Fatalf("stdout missing workId key: %s", out.String())
	}
	if string(raw["workId"]) != "null" {
		t.Fatalf("workId = %s, want null", raw["workId"])
	}
}

func TestSubmit_HumanStdoutIncludesWorkMetadataAndShowHint(t *testing.T) {
	workID := "batch-req-1-human-submit"
	name := "human-submit"
	workType := "task"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{
			TraceId:      "human-trace-1",
			WorkId:       &workID,
			Name:         &name,
			WorkTypeName: &workType,
		}); err != nil {
			t.Errorf("encode submit response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"human stdout task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Submit(SubmitConfig{
		Name:         "human-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       mustServerBase(t, srv.URL),
		Output:       &out,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Submitted: human-submit (task)\n",
		"traceId: human-trace-1\n",
		"workId: batch-req-1-human-submit\n",
		"Verify: you work show batch-req-1-human-submit\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"workId was not returned",
		"you work list --name",
		`{"title":"human stdout task"}`,
		"requestId",
		"accepted",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("stdout leaked %q:\n%s", forbidden, got)
		}
	}
}

func TestSubmit_HumanStdoutFallsBackToWorkListWithoutWorkId(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{TraceId: "human-trace-2"}); err != nil {
			t.Errorf("encode submit response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"human stdout task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Submit(SubmitConfig{
		Name:         "human-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       mustServerBase(t, srv.URL),
		Output:       &out,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Submitted: human-submit (task)\n",
		"traceId: human-trace-2\n",
		"workId was not returned; verify with:\n",
		"you work list --name human-submit\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "you work show") {
		t.Fatalf("stdout should not suggest work show without workId:\n%s", got)
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

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"scoped task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Submit(SubmitConfig{
		Name:         "scoped-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       mustServerBase(t, srv.URL),
		SessionID:    "session-beta",
	}); err != nil {
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

	server := mustServerBase(t, srv.URL)

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
		Server:       server,
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

	server := mustServerBase(t, srv.URL)

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Submit(SubmitConfig{
		Name:         "task-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       server,
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

	var out bytes.Buffer
	err := Submit(SubmitConfig{
		Name:         "task-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       "http://127.0.0.1:19999",
		Output:       &out,
	})
	if err == nil {
		t.Fatal("expected error when factory is not running")
	}
	if got := err.Error(); !strings.Contains(got, "factory not reachable at http://127.0.0.1:19999") {
		t.Fatalf("error = %q, want factory not reachable at resolved URL", got)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on transport failure", out.String())
	}
}

func TestSubmit_NonJSONErrorBodyUsesBoundedPreview(t *testing.T) {
	longBody := strings.Repeat("x", submitErrorBodyPreviewLimit+30)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(longBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := Submit(SubmitConfig{
		Name:         "task-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       mustServerBase(t, srv.URL),
		Output:       &out,
	})
	if err == nil {
		t.Fatal("expected error for non-JSON failure body")
	}
	wantPreview := longBody[:submitErrorBodyPreviewLimit] + "..."
	if got := err.Error(); got != "submission failed (500): "+wantPreview {
		t.Fatalf("error = %q, want bounded preview %q", got, wantPreview)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on API failure", out.String())
	}
}

func TestSubmit_ErrorIncludesWorkIdWhenPresent(t *testing.T) {
	workID := "batch-req-duplicate-submit"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"message": "work already accepted",
			"family":  "client",
			"code":    "BAD_REQUEST",
			"workId":  workID,
		}); err != nil {
			t.Errorf("encode error response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := Submit(SubmitConfig{
		Name:         "task-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       mustServerBase(t, srv.URL),
		Output:       &out,
	})
	if err == nil {
		t.Fatal("expected error when API returns workId")
	}
	if got := err.Error(); got != "submission failed (409): work already accepted workId="+workID {
		t.Fatalf("error = %q, want stable workId suffix", got)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on API failure", out.String())
	}
}

func TestSubmit_JSONModeDoesNotEmitSuccessOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Message: "invalid payload",
			Code:    factoryapi.BADREQUEST,
			Family:  factoryapi.ErrorFamilyBadRequest,
		}); err != nil {
			t.Errorf("encode error response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := Submit(SubmitConfig{
		Name:         "task-submit",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       mustServerBase(t, srv.URL),
		JSON:         true,
		Output:       &out,
	})
	if err == nil {
		t.Fatal("expected error for JSON-mode failure")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty JSON success envelope on failure", out.String())
	}
}

func mustServerBase(t *testing.T, rawURL string) string {
	t.Helper()
	return strings.TrimSuffix(rawURL, "/")
}
