package submit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSubmitBatch_DryRunPipedStdinWithNoArgs(t *testing.T) {
	json := validBatchJSON("batch-stdin-pipe", "alpha")

	var out bytes.Buffer
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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

	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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

	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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

	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
		Args:   []string{path},
		DryRun: true,
		Server: "http://127.0.0.1:1",
		Output: &out,
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

	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	if err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTestWithPreparation(t, BatchConfig{Context: context.Background(),
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	}, batchPreparationFailure("batch works must contain at least one item"))
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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

	err := submitBatchForTestWithPreparation(t, BatchConfig{Context: context.Background(),
		Args:   []string{`{not-json`},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	}, batchPreparationFailure("invalid character 'n' looking for beginning of object key string"))
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
	err := submitBatchForTestWithPreparation(t, BatchConfig{Context: context.Background(),
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	}, batchPreparationFailure("batch requestId is required"))
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
	err := submitBatchForTestWithPreparation(t, BatchConfig{Context: context.Background(),
		Args:   []string{path},
		Server: mustServerBase(t, srv.URL),
		Output: io.Discard,
	}, batchPreparationFailure("works[0].work_type_id is not supported; use workTypeName"))
	if err == nil {
		t.Fatal("expected retired-field validation error")
	}
	want := "works[0].work_type_id is not supported; use workTypeName"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
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

func TestSubmitBatch_UsesDocsExampleStartupWorkFile(t *testing.T) {
	path := testutil.MustRepoPath(t, "docs/examples/startup-work.json")
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
		Args:   []string{path},
		DryRun: true,
		Server: "http://127.0.0.1:1",
		Output: io.Discard,
	})
	if err != nil {
		t.Fatalf("SubmitBatch dry-run on docs example: %v", err)
	}
}

func TestSubmitBatch_DryRunLocalAgentCliRuntimeBatchExample(t *testing.T) {
	path := testutil.MustRepoPath(
		t,
		"tests/functional/smoke/testdata/factory-batch-local-agent-cli-runtime.json",
	)

	var out bytes.Buffer
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
		Args:   []string{path},
		DryRun: true,
		Server: "http://127.0.0.1:1",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch dry-run on local agent CLI runtime batch example: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"requestId: local-agent-cli-runtime-20260615",
		"work count: 6",
		"relationCount: 10",
		"local-agent-cli-runtime-loopback",
		"dry-run: no request sent",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestSubmitBatch_DryRunFactoryDocsBatchInputExample(t *testing.T) {
	path := testutil.MustRepoPath(t, "factory/docs/batch-input-example.json")

	var out bytes.Buffer
	err := submitBatchForTest(t, BatchConfig{Context: context.Background(),
		Args:      []string{path},
		DryRun:    true,
		SessionID: "c803e7f7-1361-4ba6-bb2b-b5c9cfeb2754",
		Server:    "http://127.0.0.1:1",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("SubmitBatch dry-run on factory docs batch input example: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"requestId: planner-docs-live-session-example-20260620",
		"work count: 4",
		"relationCount: 3",
		"planner-docs-live-session-loopback",
		"dry-run: no request sent",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func submitBatchForTest(t *testing.T, cfg BatchConfig) error {
	return submitBatchForTestWithPreparation(t, cfg, testFactoryRequestBatchPreparation{})
}

func submitBatchForTestWithPreparation(
	t *testing.T,
	cfg BatchConfig,
	prepare work.FactoryRequestBatchPreparation,
) error {
	t.Helper()
	path := strings.TrimSpace(cfg.FileFlag)
	if path == "" && len(cfg.Args) == 1 {
		candidate := strings.TrimSpace(cfg.Args[0])
		if candidate != "-" && !looksLikeInlineJSON(candidate) {
			path = candidate
		}
	}
	if path != "" && path != "-" {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read explicit batch test fixture %q: %v", path, err)
		}
		files := map[string][]byte{}
		if err == nil {
			files[path] = data
		}
		cfg.FileSystem = batchInputFileSystemFake{files: files}
	}
	cfg.HTTP = testHTTPProtocol(t)
	return SubmitBatch(prepare, cfg)
}

func batchPreparationFailure(message string) work.FactoryRequestBatchPreparation {
	return factoryRequestBatchPreparationFunc(func(context.Context, []byte) (work.PreparedFactoryRequestBatch, error) {
		return work.PreparedFactoryRequestBatch{}, fmt.Errorf("%s", message)
	})
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
