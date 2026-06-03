package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestParseCursorInferenceResult_Success(t *testing.T) {
	stdout := []byte(`{
		"type": "result",
		"subtype": "success",
		"is_error": false,
		"duration_ms": 1234,
		"duration_api_ms": 1100,
		"result": "Done reviewing the repo.",
		"session_id": "c6b62c6f-7ead-4fd6-9922-e952131177ff",
		"request_id": "10e11780-df2f-45dc-a1ff-4540af32e9c0",
		"usage": {
			"inputTokens": 1200,
			"outputTokens": 340,
			"cacheReadTokens": 50,
			"cacheWriteTokens": 10
		}
	}`)

	parsed, err := parseCursorInferenceResult(string(interfaces.ModelProviderCursor), stdout)
	if err != nil {
		t.Fatalf("parseCursorInferenceResult returned error: %v", err)
	}
	if parsed.Content != "Done reviewing the repo." {
		t.Fatalf("content = %q, want parsed result text", parsed.Content)
	}
	if parsed.ProviderSession == nil {
		t.Fatal("expected provider session metadata")
	}
	if parsed.ProviderSession.Provider != string(interfaces.ModelProviderCursor) {
		t.Fatalf("provider = %q, want cursor", parsed.ProviderSession.Provider)
	}
	if parsed.ProviderSession.Kind != providerSessionKindSessionID {
		t.Fatalf("kind = %q, want session_id", parsed.ProviderSession.Kind)
	}
	if parsed.ProviderSession.ID != "c6b62c6f-7ead-4fd6-9922-e952131177ff" {
		t.Fatalf("session id = %q", parsed.ProviderSession.ID)
	}

	assertCursorResponseMetadata(t, parsed.ResponseMetadata, map[string]string{
		cursorResponseMetadataRequestID:        "10e11780-df2f-45dc-a1ff-4540af32e9c0",
		cursorResponseMetadataDurationMS:       "1234",
		cursorResponseMetadataDurationAPIMS:    "1100",
		cursorResponseMetadataInputTokens:      "1200",
		cursorResponseMetadataOutputTokens:     "340",
		cursorResponseMetadataCacheReadTokens:  "50",
		cursorResponseMetadataCacheWriteTokens: "10",
	})
}

func TestParseCursorInferenceResult_MissingSessionID(t *testing.T) {
	stdout := []byte(`{
		"type": "result",
		"subtype": "success",
		"is_error": false,
		"result": "done",
		"session_id": ""
	}`)

	_, err := parseCursorInferenceResult(string(interfaces.ModelProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for missing session_id")
	}
	if err.Type != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("error type = %q, want permanent_bad_request", err.Type)
	}
}

func TestParseCursorInferenceResult_MalformedJSON(t *testing.T) {
	_, err := parseCursorInferenceResult(string(interfaces.ModelProviderCursor), []byte(`{not json`))
	if err == nil {
		t.Fatal("expected parse error for malformed JSON")
	}
	if err.Type != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("error type = %q, want permanent_bad_request", err.Type)
	}
}

func TestParseCursorInferenceResult_UnexpectedType(t *testing.T) {
	stdout := []byte(`{"type":"assistant","subtype":"success","result":"hi","session_id":"sess-1"}`)

	_, err := parseCursorInferenceResult(string(interfaces.ModelProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for unexpected type")
	}
	if err.Type != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("error type = %q, want permanent_bad_request", err.Type)
	}
}

func TestParseCursorInferenceResult_ErrorSubtype(t *testing.T) {
	stdout := []byte(`{
		"type": "result",
		"subtype": "error",
		"is_error": true,
		"result": "rate limited",
		"session_id": "sess-1"
	}`)

	_, err := parseCursorInferenceResult(string(interfaces.ModelProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for error subtype")
	}
	if err.Type != interfaces.WorkFailureTypeInternalServerError {
		t.Fatalf("error type = %q, want internal_server_error", err.Type)
	}
}

func cursorSuccessStdoutJSON(result, sessionID string) []byte {
	if result == "" {
		result = "Done. COMPLETE"
	}
	if sessionID == "" {
		sessionID = "cursor-test-session"
	}
	payload := cursorResultPayload{
		Type:      cursorResultTypeResult,
		Subtype:   cursorResultSubtypeSuccess,
		SessionID: sessionID,
		Result:    result,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return encoded
}

func TestBoundedCommandOutputExcerpt_TruncatesWhenOverLimit(t *testing.T) {
	const limit = 8
	long := []byte("0123456789abcdef")
	got := boundedCommandOutputExcerpt(long, limit)
	want := "01234567..."
	if got != want {
		t.Fatalf("excerpt = %q, want %q", got, want)
	}
}

func TestWithCursorCommandOutputExcerpts_AttachesBoundedStdoutAndStderr(t *testing.T) {
	stdout := []byte(strings.Repeat("a", cursorCommandOutputExcerptLimit+10))
	stderr := []byte("rate limited\n")
	diagnostics := withCursorCommandOutputExcerpts(nil, stdout, stderr)
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatal("expected provider diagnostics with excerpts")
	}
	if got := diagnostics.Provider.ResponseMetadata[cursorResponseMetadataStdoutExcerpt]; len(got) != cursorCommandOutputExcerptLimit+3 {
		t.Fatalf("stdout excerpt len = %d, want %d with ellipsis", len(got), cursorCommandOutputExcerptLimit+3)
	}
	if got := diagnostics.Provider.ResponseMetadata[cursorResponseMetadataStderrExcerpt]; got != "rate limited" {
		t.Fatalf("stderr excerpt = %q, want rate limited", got)
	}
}

func assertCursorResponseMetadata(t *testing.T, metadata map[string]string, want map[string]string) {
	t.Helper()
	for key, wantValue := range want {
		if got := metadata[key]; got != wantValue {
			t.Fatalf("response metadata[%q] = %q, want %q", key, got, wantValue)
		}
	}
}
