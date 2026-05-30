package submit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

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

	got := decodeBatchSubmitJSONResult(t, out.Bytes())
	assertBatchSubmitJSONSuccessFields(t, got, out.String())
}

func assertBatchSubmitJSONSuccessFields(t *testing.T, got BatchSubmitJSONResult, stdout string) {
	t.Helper()

	want := BatchSubmitJSONResult{
		RequestID:     "batch-json-1",
		TraceID:       "trace-json-1",
		WorkCount:     2,
		RelationCount: 1,
		SessionID:     "session-json",
		EndpointPath:  "/factory-sessions/session-json/work-requests/batch-json-1",
		BatchSource:   batchSourceFile,
		Works: []BatchSubmitJSONWork{
			{Name: "alpha", WorkTypeName: "task", WorkID: "work-alpha"},
			{Name: "beta", WorkTypeName: "review"},
		},
	}
	if got.RequestID != want.RequestID {
		t.Fatalf("requestId = %q, want %q", got.RequestID, want.RequestID)
	}
	if got.TraceID != want.TraceID {
		t.Fatalf("traceId = %q, want %q", got.TraceID, want.TraceID)
	}
	if got.WorkCount != want.WorkCount {
		t.Fatalf("workCount = %d, want %d", got.WorkCount, want.WorkCount)
	}
	if got.RelationCount != want.RelationCount {
		t.Fatalf("relationCount = %d, want %d", got.RelationCount, want.RelationCount)
	}
	if got.SessionID != want.SessionID {
		t.Fatalf("sessionId = %q, want %q", got.SessionID, want.SessionID)
	}
	if got.EndpointPath != want.EndpointPath {
		t.Fatalf("endpointPath = %q, want %q", got.EndpointPath, want.EndpointPath)
	}
	if got.BatchSource != want.BatchSource {
		t.Fatalf("batchSource = %q, want %q", got.BatchSource, want.BatchSource)
	}
	if len(got.Works) != len(want.Works) {
		t.Fatalf("works len = %d, want %d", len(got.Works), len(want.Works))
	}
	for i := range want.Works {
		if got.Works[i] != want.Works[i] {
			t.Fatalf("works[%d] = %#v, want %#v", i, got.Works[i], want.Works[i])
		}
	}
	if got.DryRun {
		t.Fatal("dryRun should be omitted or false on HTTP success")
	}
	if strings.Contains(stdout, "hidden") {
		t.Fatalf("JSON output leaked batch payload:\n%s", stdout)
	}
}

func decodeBatchSubmitJSONResult(t *testing.T, raw []byte) BatchSubmitJSONResult {
	t.Helper()

	var got BatchSubmitJSONResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, string(raw))
	}
	return got
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

	got := decodeBatchSubmitJSONResult(t, out.Bytes())
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

	got := decodeBatchSubmitJSONResult(t, out.Bytes())
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
