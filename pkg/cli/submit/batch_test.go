package submit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestSubmitBatch_DryRunPipedStdinWithNoArgs(t *testing.T) {
	json := validBatchJSON("batch-stdin-pipe", "alpha")

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Stdin:      strings.NewReader(json),
		StdinIsTTY: func() bool { return false },
		DryRun:     true,
		Server:     "http://127.0.0.1:1",
		Output:     &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"requestId: batch-stdin-pipe",
		"batchSource: stdin",
		"dry-run: no request sent",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestSubmitBatch_DryRunExplicitStdinDash(t *testing.T) {
	json := validBatchJSON("batch-stdin-dash", "alpha")

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:   []string{"-"},
		Stdin:  strings.NewReader(json),
		DryRun: true,
		Server: "http://127.0.0.1:1",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	if !strings.Contains(out.String(), "batchSource: stdin") {
		t.Fatalf("output missing stdin source:\n%s", out.String())
	}
}

func TestSubmitBatch_DryRunFileFlag(t *testing.T) {
	path := writeBatchFile(t, validBatchJSON("batch-file-flag-dry", "alpha"))

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		FileFlag: path,
		DryRun:   true,
		Server:   "http://127.0.0.1:1",
		Output:   &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"requestId: batch-file-flag-dry",
		"batchSource: file",
		"dry-run: no request sent",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestSubmitBatch_DryRunFileFlagStdinDash(t *testing.T) {
	json := validBatchJSON("batch-file-flag-stdin", "alpha")

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		FileFlag: "-",
		Stdin:    strings.NewReader(json),
		DryRun:   true,
		Server:   "http://127.0.0.1:1",
		Output:   &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	if !strings.Contains(out.String(), "batchSource: stdin") {
		t.Fatalf("output missing stdin source:\n%s", out.String())
	}
}

func TestSubmitBatch_NoArgsInteractiveTTYFailsWithUsageGuidance(t *testing.T) {
	err := SubmitBatch(BatchConfig{
		StdinIsTTY: func() bool { return true },
		Server:     "http://127.0.0.1:1",
		Output:     io.Discard,
	})
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(err.Error(), "batch input required") {
		t.Fatalf("error = %v, want usage guidance", err)
	}
	if !strings.Contains(err.Error(), "you submit batch --help") {
		t.Fatalf("error = %v, want help pointer", err)
	}
}

func TestSubmitBatch_EmptyPipedStdinFailsBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	err := SubmitBatch(BatchConfig{
		Stdin:      strings.NewReader("\n"),
		StdinIsTTY: func() bool { return false },
		Server:     mustServerBase(t, srv.URL),
		Output:     io.Discard,
	})
	if err == nil {
		t.Fatal("expected empty stdin error")
	}
	if !strings.Contains(err.Error(), "stdin input is empty") {
		t.Fatalf("error = %v, want empty stdin message", err)
	}
	if called {
		t.Fatal("expected no HTTP call for empty stdin")
	}
}

func TestSubmitBatch_PUTFromPipedStdinUsesSessionScopedPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
			RequestId: "batch-stdin-put",
			TraceId:   "trace-stdin-put",
			Works:     []factoryapi.UpsertWorkRequestSubmittedWork{{Name: "alpha", WorkTypeName: "task", WorkId: "work-1"}},
		})
	}))
	defer srv.Close()

	err := SubmitBatch(BatchConfig{
		Stdin:      strings.NewReader(validBatchJSON("batch-stdin-put", "alpha")),
		StdinIsTTY: func() bool { return false },
		Server:     mustServerBase(t, srv.URL),
		Output:     io.Discard,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}
	if gotPath != "/factory-sessions/~default/work-requests/batch-stdin-put" {
		t.Fatalf("path = %q, want session-scoped work-requests path", gotPath)
	}
}

func TestSubmitBatch_FilePathIgnoresStdinContent(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
			RequestId: "batch-file-wins",
			TraceId:   "trace-file-wins",
			Works:     []factoryapi.UpsertWorkRequestSubmittedWork{{Name: "alpha", WorkTypeName: "task", WorkId: "work-1"}},
		})
	}))
	defer srv.Close()

	path := writeBatchFile(t, validBatchJSON("batch-file-wins", "alpha"))
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Stdin:  strings.NewReader(validBatchJSON("batch-wrong", "wrong")),
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}
	if !strings.Contains(gotBody, "batch-file-wins") {
		t.Fatalf("request body = %q, want file batch requestId", gotBody)
	}
	if strings.Contains(gotBody, "batch-wrong") {
		t.Fatalf("request body used stdin content:\n%s", gotBody)
	}
}

func TestSubmitBatch_DryRunInlineJSONPositional(t *testing.T) {
	json := validBatchJSON("batch-inline-dry", "alpha")

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:   []string{json},
		DryRun: true,
		Server: "http://127.0.0.1:1",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"requestId: batch-inline-dry",
		"batchSource: inline",
		"dry-run: no request sent",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestSubmitBatch_NonexistentPathWithoutJSONPrefixFailsAsMissingFile(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	err := SubmitBatch(BatchConfig{
		Args:   []string{"/no/such/batch-file.json"},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected missing file error")
	}
	if !strings.Contains(err.Error(), "batch file not found") {
		t.Fatalf("error = %v, want missing file message", err)
	}
	if strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("error treated path as JSON: %v", err)
	}
	if called {
		t.Fatal("expected no HTTP call for missing file")
	}
}

func TestSubmitBatch_PUTFromInlineJSONUsesRequestIdFromBody(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
			RequestId: "batch-inline-put",
			TraceId:   "trace-inline-put",
			Works:     []factoryapi.UpsertWorkRequestSubmittedWork{{Name: "alpha", WorkTypeName: "task", WorkId: "work-1"}},
		})
	}))
	defer srv.Close()

	inline := validBatchJSON("batch-inline-put", "alpha")
	err := SubmitBatch(BatchConfig{
		Args:   []string{inline},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}
	if !strings.Contains(gotBody, "batch-inline-put") {
		t.Fatalf("request body = %q, want inline batch requestId", gotBody)
	}
}

func TestSubmitBatch_DryRunValidFileExitsWithoutHTTP(t *testing.T) {
	path := writeBatchFile(t, `{
		"requestId": "batch-dry-run-1",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{"name": "alpha", "workTypeName": "task", "payload": {"title": "A"}}],
		"relations": [{"type": "DEPENDS_ON", "sourceWorkName": "alpha", "targetWorkName": "beta"}]
	}`)

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:    []string{path},
		DryRun:  true,
		Server:  "http://127.0.0.1:1",
		Output:  &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"requestId: batch-dry-run-1",
		"work count: 1",
		"works: alpha",
		"relationCount: 1",
		"batchSource: file",
		"dry-run: no request sent",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestSubmitBatch_DryRunSucceedsWhenFactoryUnreachable(t *testing.T) {
	path := writeBatchFile(t, validBatchJSON("batch-offline", "task-a"))

	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		DryRun: true,
		Server: "http://127.0.0.1:1",
		Output: io.Discard,
	})
	if err != nil {
		t.Fatalf("SubmitBatch dry-run: %v", err)
	}
}

func TestSubmitBatch_PUTUsesSessionScopedWorkRequestsPath(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
			RequestId: "batch-put-1",
			TraceId:   "trace-put-1",
			Works: []factoryapi.UpsertWorkRequestSubmittedWork{{
				Name: "alpha", WorkTypeName: "task", WorkId: "work-1",
			}},
		})
	}))
	defer srv.Close()

	path := writeBatchFile(t, validBatchJSON("batch-put-1", "alpha"))
	err := SubmitBatch(BatchConfig{
		Args:      []string{path},
		Server:    mustServerBase(t, srv.URL),
		SessionID: "session-beta",
		Output:    io.Discard,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/factory-sessions/session-beta/work-requests/batch-put-1" {
		t.Fatalf("path = %q, want session-scoped work-requests path", gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
}

func TestSubmitBatch_DefaultSessionUsesCompatibilitySession(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
			RequestId: "batch-default",
			TraceId:   "trace-default",
			Works:     []factoryapi.UpsertWorkRequestSubmittedWork{{Name: "alpha", WorkTypeName: "task", WorkId: "work-1"}},
		})
	}))
	defer srv.Close()

	path := writeBatchFile(t, validBatchJSON("batch-default", "alpha"))
	if err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	}); err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}
	if gotPath != "/factory-sessions/~default/work-requests/batch-default" {
		t.Fatalf("path = %q, want default session work-requests path", gotPath)
	}
}

func TestSubmitBatch_ValidationFailsBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	path := writeBatchFile(t, `{"requestId":"batch-empty","type":"FACTORY_REQUEST_BATCH","works":[]}`)
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("error = %v, want empty works validation", err)
	}
	if called {
		t.Fatal("expected no HTTP call for invalid batch")
	}
}

func TestSubmitBatch_HTTPErrorSurfacesAPIMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Message: "request_id path and requestId body must match"})
	}))
	defer srv.Close()

	path := writeBatchFile(t, validBatchJSON("batch-put-1", "alpha"))
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "request_id path and requestId body must match") {
		t.Fatalf("error = %v, want status and API message", err)
	}
}

func TestSubmitBatch_HTTP404SurfacesAPIMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Message: "factory session not found"})
	}))
	defer srv.Close()

	path := writeBatchFile(t, validBatchJSON("batch-404", "alpha"))
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "factory session not found") {
		t.Fatalf("error = %v, want status and API message", err)
	}
}

func TestSubmitBatch_HTTPErrorUsesBoundedNonJSONBodyPreview(t *testing.T) {
	longBody := strings.Repeat("x", batchErrorBodyPreviewLimit+30)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, longBody)
	}))
	defer srv.Close()

	path := writeBatchFile(t, validBatchJSON("batch-bounded", "alpha"))
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	wantPreview := longBody[:batchErrorBodyPreviewLimit] + "..."
	if !strings.Contains(err.Error(), wantPreview) {
		t.Fatalf("error = %v, want bounded preview %q", err, wantPreview)
	}
	if strings.Contains(err.Error(), longBody) {
		t.Fatalf("error included full response body")
	}
}

func TestSubmitBatch_InvalidJSONFailsBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	err := SubmitBatch(BatchConfig{
		Args:   []string{`{not-json`},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
	if !strings.Contains(err.Error(), "parse inline JSON") {
		t.Fatalf("error = %v, want inline JSON parse context", err)
	}
	if !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("error = %v, want JSON parse detail", err)
	}
	if called {
		t.Fatal("expected no HTTP call for invalid JSON")
	}
}

func TestSubmitBatch_EmptyRequestIDFailsBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	path := writeBatchFile(t, `{"requestId":"","type":"FACTORY_REQUEST_BATCH","works":[{"name":"alpha","workTypeName":"task","payload":{"title":"A"}}]}`)
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "requestId is required") {
		t.Fatalf("error = %v, want empty requestId validation", err)
	}
	if called {
		t.Fatal("expected no HTTP call for empty requestId")
	}
}

func TestSubmitBatch_RetiredFieldFailsWithGuidanceBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	path := writeBatchFile(t, `{
		"requestId": "batch-retired",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{"name": "alpha", "work_type_id": "task", "payload": {"title": "A"}}]
	}`)
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected retired-field validation error")
	}
	if !strings.Contains(err.Error(), "retired work_type_id") || !strings.Contains(err.Error(), "workTypeName") {
		t.Fatalf("error = %v, want retired-field guidance", err)
	}
	if called {
		t.Fatal("expected no HTTP call for retired-field batch")
	}
}

func TestSubmitBatch_HTTPErrorDoesNotEmitSuccessJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Message: "invalid batch"})
	}))
	defer srv.Close()

	path := writeBatchFile(t, validBatchJSON("batch-json-err", "alpha"))
	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		JSON:   true,
		Output: &out,
	})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on HTTP failure with --json", out.String())
	}
}

func TestSubmitBatch_UnreachableFactoryMatchesUnaryStyle(t *testing.T) {
	path := writeBatchFile(t, validBatchJSON("batch-offline", "alpha"))
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: "http://127.0.0.1:1",
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected unreachable factory error")
	}
	if !strings.Contains(err.Error(), "factory not reachable at") {
		t.Fatalf("error = %v, want unary-style unreachable message", err)
	}
}

func TestSubmitBatch_VerboseLogsMetadataWithoutPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
			RequestId: "batch-verbose",
			TraceId:   "trace-verbose",
			Works:     []factoryapi.UpsertWorkRequestSubmittedWork{{Name: "alpha", WorkTypeName: "task", WorkId: "work-1"}},
		})
	}))
	defer srv.Close()

	path := writeBatchFile(t, `{
		"requestId": "batch-verbose",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{"name": "alpha", "workTypeName": "task", "payload": {"secret": "do-not-log"}}]
	}`)

	var diagnostics bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:        []string{path},
		Server:      mustServerBase(t, srv.URL),
		Verbose:     true,
		Diagnostics: &diagnostics,
		Output:      io.Discard,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	got := diagnostics.String()
	for _, want := range []string{
		"submit batch request",
		"endpointPath=/factory-sessions/~default/work-requests/batch-verbose",
		"batchSource=file",
		"requestId=\"batch-verbose\"",
		"workCount=1",
		"submit batch response",
		"status=201",
		"traceId=trace-verbose",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "do-not-log") {
		t.Fatalf("diagnostics leaked payload content:\n%s", got)
	}
}

func TestSubmitBatch_HumanSuccessOutputIncludesWorkDetailsAndHints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
			RequestId: "batch-human-1",
			TraceId:   "trace-human-1",
			Works: []factoryapi.UpsertWorkRequestSubmittedWork{
				{Name: "alpha", WorkTypeName: "task", WorkId: "work-alpha"},
				{Name: "beta", WorkTypeName: "review", WorkId: ""},
			},
		})
	}))
	defer srv.Close()

	path := writeBatchFile(t, `{
		"requestId": "batch-human-1",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "alpha", "workTypeName": "task", "payload": {"secret": "hidden"}},
			{"name": "beta", "workTypeName": "review", "payload": {"secret": "hidden"}}
		],
		"relations": [{"type": "DEPENDS_ON", "sourceWorkName": "alpha", "targetWorkName": "beta"}]
	}`)

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"requestId: batch-human-1",
		"traceId: trace-human-1",
		"work count: 2",
		"relationCount: 1",
		"  alpha (task) workId=work-alpha",
		"you work show work-alpha",
		"  beta (review)",
		"you work list --name beta",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hidden") || strings.Contains(got, `"payload"`) {
		t.Fatalf("output leaked batch payload:\n%s", got)
	}
}

func TestSubmitBatch_HumanSuccessOutputTruncatesLongWorkLists(t *testing.T) {
	works := make([]factoryapi.UpsertWorkRequestSubmittedWork, 0, 12)
	batchWorks := make([]string, 0, 12)
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("work-%02d", i)
		works = append(works, factoryapi.UpsertWorkRequestSubmittedWork{
			Name: name, WorkTypeName: "task", WorkId: "id-" + name,
		})
		batchWorks = append(batchWorks, fmt.Sprintf(`{"name": %q, "workTypeName": "task", "payload": {"n": %d}}`, name, i))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
			RequestId: "batch-truncate",
			TraceId:   "trace-truncate",
			Works:     works,
		})
	}))
	defer srv.Close()

	path := writeBatchFile(t, `{
		"requestId": "batch-truncate",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [`+strings.Join(batchWorks, ",")+`]
	}`)

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "work count: 12") {
		t.Fatalf("output missing work count:\n%s", got)
	}
	if !strings.Contains(got, "  work-01 (task) workId=id-work-01") {
		t.Fatalf("output missing first work line:\n%s", got)
	}
	if !strings.Contains(got, "  work-10 (task) workId=id-work-10") {
		t.Fatalf("output missing tenth work line:\n%s", got)
	}
	if strings.Contains(got, "  work-11 (task)") {
		t.Fatalf("output should truncate after ten work lines:\n%s", got)
	}
	if !strings.Contains(got, "... and 2 more work(s)") {
		t.Fatalf("output missing truncation summary:\n%s", got)
	}
}

func TestSubmitBatch_JSONSuccessOutputIncludesRequiredFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
			RequestId: "batch-json-1",
			TraceId:   "trace-json-1",
			Works: []factoryapi.UpsertWorkRequestSubmittedWork{
				{Name: "alpha", WorkTypeName: "task", WorkId: "work-alpha"},
				{Name: "beta", WorkTypeName: "review", WorkId: ""},
			},
		})
	}))
	defer srv.Close()

	path := writeBatchFile(t, `{
		"requestId": "batch-json-1",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "alpha", "workTypeName": "task", "payload": {"secret": "hidden"}},
			{"name": "beta", "workTypeName": "review", "payload": {"secret": "hidden"}}
		],
		"relations": [{"type": "DEPENDS_ON", "sourceWorkName": "alpha", "targetWorkName": "beta"}]
	}`)

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:      []string{path},
		Server:    mustServerBase(t, srv.URL),
		SessionID: "session-json",
		JSON:      true,
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	var got BatchSubmitJSONResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, out.String())
	}
	if got.RequestID != "batch-json-1" {
		t.Fatalf("requestId = %q, want batch-json-1", got.RequestID)
	}
	if got.TraceID != "trace-json-1" {
		t.Fatalf("traceId = %q, want trace-json-1", got.TraceID)
	}
	if got.WorkCount != 2 {
		t.Fatalf("workCount = %d, want 2", got.WorkCount)
	}
	if got.RelationCount != 1 {
		t.Fatalf("relationCount = %d, want 1", got.RelationCount)
	}
	if got.SessionID != "session-json" {
		t.Fatalf("sessionId = %q, want session-json", got.SessionID)
	}
	if got.EndpointPath != "/factory-sessions/session-json/work-requests/batch-json-1" {
		t.Fatalf("endpointPath = %q, want session-scoped work-requests path", got.EndpointPath)
	}
	if got.BatchSource != batchSourceFile {
		t.Fatalf("batchSource = %q, want %q", got.BatchSource, batchSourceFile)
	}
	if len(got.Works) != 2 {
		t.Fatalf("works len = %d, want 2", len(got.Works))
	}
	if got.Works[0].Name != "alpha" || got.Works[0].WorkTypeName != "task" || got.Works[0].WorkID != "work-alpha" {
		t.Fatalf("works[0] = %#v, want alpha/task/work-alpha", got.Works[0])
	}
	if got.Works[1].Name != "beta" || got.Works[1].WorkTypeName != "review" || got.Works[1].WorkID != "" {
		t.Fatalf("works[1] = %#v, want beta/review without workId", got.Works[1])
	}
	if got.DryRun {
		t.Fatal("dryRun should be omitted or false on HTTP success")
	}
	if strings.Contains(out.String(), "hidden") {
		t.Fatalf("JSON output leaked batch payload:\n%s", out.String())
	}
}

func TestSubmitBatch_JSONDryRunIncludesSummaryWithoutTraceIDUnlessPresent(t *testing.T) {
	path := writeBatchFile(t, validBatchJSON("batch-json-dry", "alpha"))

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		DryRun: true,
		JSON:   true,
		Server: "http://127.0.0.1:1",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	var got BatchSubmitJSONResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, out.String())
	}
	if !got.DryRun {
		t.Fatal("dryRun = false, want true")
	}
	if got.RequestID != "batch-json-dry" {
		t.Fatalf("requestId = %q, want batch-json-dry", got.RequestID)
	}
	if got.TraceID != "" {
		t.Fatalf("traceId = %q, want omitted when absent from input", got.TraceID)
	}
	if got.WorkCount != 1 {
		t.Fatalf("workCount = %d, want 1", got.WorkCount)
	}
	if got.RelationCount != 0 {
		t.Fatalf("relationCount = %d, want 0", got.RelationCount)
	}
	if got.SessionID != "~default" {
		t.Fatalf("sessionId = %q, want ~default", got.SessionID)
	}
	if got.EndpointPath != "/factory-sessions/~default/work-requests/batch-json-dry" {
		t.Fatalf("endpointPath = %q, want default session dry-run path", got.EndpointPath)
	}
	if got.BatchSource != batchSourceFile {
		t.Fatalf("batchSource = %q, want %q", got.BatchSource, batchSourceFile)
	}
	if len(got.WorkNames) != 1 || got.WorkNames[0] != "alpha" {
		t.Fatalf("workNames = %#v, want [alpha]", got.WorkNames)
	}
	if len(got.Works) != 0 {
		t.Fatalf("works = %#v, want omitted on dry-run", got.Works)
	}
}

func TestSubmitBatch_JSONDryRunIncludesTraceIDWhenPresentInInput(t *testing.T) {
	path := writeBatchFile(t, `{
		"requestId": "batch-json-trace",
		"type": "FACTORY_REQUEST_BATCH",
		"currentChainingTraceId": "trace-from-input",
		"works": [{"name": "alpha", "workTypeName": "task", "payload": {"title": "A"}}]
	}`)

	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		DryRun: true,
		JSON:   true,
		Server: "http://127.0.0.1:1",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	var got BatchSubmitJSONResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, out.String())
	}
	if got.TraceID != "trace-from-input" {
		t.Fatalf("traceId = %q, want trace-from-input", got.TraceID)
	}
}

func TestSubmitBatch_ValidationErrorDoesNotEmitSuccessJSON(t *testing.T) {
	var out bytes.Buffer
	err := SubmitBatch(BatchConfig{
		Args:   []string{`{"requestId":"bad","type":"FACTORY_REQUEST_BATCH","works":[]}`},
		JSON:   true,
		Server: "http://127.0.0.1:1",
		Output: &out,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on validation failure", out.String())
	}
}

func TestSubmitBatch_UsesDocsExampleStartupWorkFile(t *testing.T) {
	path := testutil.MustRepoPath(t, "docs/examples/startup-work.json")
	err := SubmitBatch(BatchConfig{
		Args:   []string{path},
		DryRun: true,
		Server: "http://127.0.0.1:1",
		Output: io.Discard,
	})
	if err != nil {
		t.Fatalf("SubmitBatch dry-run on docs example: %v", err)
	}
}

func writeBatchFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func validBatchJSON(requestID, workName string) string {
	return `{
		"requestId": "` + requestID + `",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{"name": "` + workName + `", "workTypeName": "task", "payload": {"title": "Task"}}]
	}`
}
