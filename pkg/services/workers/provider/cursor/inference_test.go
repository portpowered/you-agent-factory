package cursor

import (
	"encoding/json"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestParseInferenceResult_Success(t *testing.T) {
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

	parsed, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err != nil {
		t.Fatalf("ParseInferenceResult returned error: %v", err)
	}
	if parsed.Content != "Done reviewing the repo." {
		t.Fatalf("content = %q, want parsed result text", parsed.Content)
	}
	if parsed.ProviderSession == nil {
		t.Fatal("expected provider session metadata")
	}
	if parsed.ProviderSession.Provider != "cursor" {
		t.Fatalf("provider = %q, want cursor", parsed.ProviderSession.Provider)
	}
	if parsed.ProviderSession.Kind != ProviderSessionKindSessionID {
		t.Fatalf("kind = %q, want session_id", parsed.ProviderSession.Kind)
	}
	if parsed.ProviderSession.ID != "c6b62c6f-7ead-4fd6-9922-e952131177ff" {
		t.Fatalf("session id = %q", parsed.ProviderSession.ID)
	}

	assertResponseMetadata(t, parsed.ResponseMetadata, map[string]string{
		ResponseMetadataRequestID:        "10e11780-df2f-45dc-a1ff-4540af32e9c0",
		ResponseMetadataDurationMS:       "1234",
		ResponseMetadataDurationAPIMS:    "1100",
		ResponseMetadataInputTokens:      "1200",
		ResponseMetadataOutputTokens:     "340",
		ResponseMetadataCacheReadTokens:  "50",
		ResponseMetadataCacheWriteTokens: "10",
	})
}

func TestParseInferenceResult_StreamJSONSuccess(t *testing.T) {
	stdout := []byte(
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor-stream-session\"}\n" +
			"{\"type\":\"assistant\",\"timestamp_ms\":1,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Plan \"}]},\"session_id\":\"cursor-stream-session\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"duration_ms\":1234,\"duration_api_ms\":1100,\"result\":\"Plan done\",\"session_id\":\"cursor-stream-session\",\"request_id\":\"req-stream-123\",\"usage\":{\"inputTokens\":12,\"outputTokens\":34}}\n",
	)

	parsed, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err != nil {
		t.Fatalf("ParseInferenceResult returned error: %v", err)
	}
	if parsed.Content != "Plan done" {
		t.Fatalf("content = %q, want stream result text", parsed.Content)
	}
	if parsed.ProviderSession == nil || parsed.ProviderSession.Provider != "cursor" || parsed.ProviderSession.ID != "cursor-stream-session" {
		t.Fatalf("provider session = %#v, want canonical cursor stream session", parsed.ProviderSession)
	}
	assertResponseMetadata(t, parsed.ResponseMetadata, map[string]string{
		ResponseMetadataRequestID:     "req-stream-123",
		ResponseMetadataDurationMS:    "1234",
		ResponseMetadataDurationAPIMS: "1100",
		ResponseMetadataInputTokens:   "12",
		ResponseMetadataOutputTokens:  "34",
	})
}

func TestParseInferenceResult_StreamJSONIgnoresMalformedAndUnknownLinesBeforeResult(t *testing.T) {
	stdout := []byte(
		"{not json}\n" +
			"{\"type\":\"mystery\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"Plan done\",\"session_id\":\"cursor-stream-session\"}\n",
	)

	parsed, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err != nil {
		t.Fatalf("ParseInferenceResult returned error: %v", err)
	}
	if parsed.Content != "Plan done" {
		t.Fatalf("content = %q, want stream result text", parsed.Content)
	}
	if parsed.ProviderSession == nil || parsed.ProviderSession.ID != "cursor-stream-session" {
		t.Fatalf("provider session = %#v, want canonical cursor stream session", parsed.ProviderSession)
	}
}

func TestParseInferenceResult_StreamJSONUsesLastTerminalResult(t *testing.T) {
	stdout := []byte(
		"{\"type\":\"result\",\"subtype\":\"api_error\",\"is_error\":true,\"result\":\"old server failure\",\"session_id\":\"old-session\"}\n" +
			"{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"ignored\"}]}}\n" +
			"{\"type\":\"result\",\"subtype\":\"rate_limit_error\",\"is_error\":true,\"result\":\"Cursor capacity is busy\",\"session_id\":\"final-session\"}\n",
	)

	parsed, failure := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if parsed != nil {
		t.Fatalf("parsed result = %#v, want no successful response", parsed)
	}
	if failure == nil || failure.Type != workerexecution.WorkFailureTypeThrottled || failure.Message != "Cursor capacity is busy" {
		t.Fatalf("failure = %#v, want final throttling result", failure)
	}
	if failure.ProviderSession == nil || failure.ProviderSession.ID != "final-session" {
		t.Fatalf("provider session = %#v, want final-session", failure.ProviderSession)
	}
}

func TestParseInferenceResult_MissingSessionID(t *testing.T) {
	stdout := []byte(`{
		"type": "result",
		"subtype": "success",
		"is_error": false,
		"result": "done",
		"session_id": ""
	}`)

	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for missing session_id")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
}

func TestParseInferenceResult_InvalidSessionID(t *testing.T) {
	stdout := []byte(`{
		"type": "result",
		"subtype": "success",
		"is_error": false,
		"result": "done",
		"session_id": "../cursor-session"
	}`)

	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for invalid session_id")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
}

func TestParseInferenceResult_StreamJSONInvalidSessionID(t *testing.T) {
	stdout := []byte(
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor-stream-session\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"Plan done\",\"session_id\":\"../cursor-stream-session\"}\n",
	)

	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for invalid stream session_id")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
}

func TestParseInferenceResult_MalformedJSON(t *testing.T) {
	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), []byte(`{not json`))
	if err == nil {
		t.Fatal("expected parse error for malformed JSON")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
}

func TestParseInferenceResult_UnexpectedType(t *testing.T) {
	stdout := []byte(`{"type":"assistant","subtype":"success","result":"hi","session_id":"sess-1"}`)

	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for unexpected type")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
}

func TestParseInferenceResult_ErrorSubtype(t *testing.T) {
	oversizedResult := strings.Repeat("x", FailureMessageLimit+20)
	stdout := []byte(`{
		"type": "result",
		"subtype": "error",
		"is_error": true,
		"result": "` + oversizedResult + `",
		"session_id": "sess-1"
	}`)

	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for error subtype")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
	if !strings.Contains(err.Message, "...") {
		t.Fatalf("error message = %q, want bounded result preview", err.Message)
	}
	if strings.Contains(err.Message, oversizedResult) {
		t.Fatalf("error message = %q, should not include full oversized result", err.Message)
	}
}

func TestBoundedCommandOutputExcerpt_TruncatesWhenOverLimit(t *testing.T) {
	const limit = 8
	long := []byte("0123456789abcdef")
	got := BoundedCommandOutputExcerpt(long, limit)
	want := "01234567..."
	if got != want {
		t.Fatalf("excerpt = %q, want %q", got, want)
	}
}

func TestBoundedText_PreservesSpacingForPublishedAssistantText(t *testing.T) {
	got := boundedText(" hi", 2)
	if got != " h..." {
		t.Fatalf("boundedText() = %q, want preserved leading spacing with truncation", got)
	}
}

func TestWithCommandOutputExcerpts_AttachesBoundedStdoutAndStderr(t *testing.T) {
	stdout := []byte(strings.Repeat("a", CommandOutputExcerptLimit+10))
	stderr := []byte("rate limited\n")
	diagnostics := WithCommandOutputExcerpts(nil, stdout, stderr)
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatal("expected provider diagnostics with excerpts")
	}
	if got := diagnostics.Provider.ResponseMetadata[ResponseMetadataStdoutExcerpt]; len(got) != CommandOutputExcerptLimit+3 {
		t.Fatalf("stdout excerpt len = %d, want %d with ellipsis", len(got), CommandOutputExcerptLimit+3)
	}
	if got := diagnostics.Provider.ResponseMetadata[ResponseMetadataStderrExcerpt]; got != "rate limited" {
		t.Fatalf("stderr excerpt = %q, want rate limited", got)
	}
}

func assertResponseMetadata(t *testing.T, metadata map[string]string, want map[string]string) {
	t.Helper()
	for key, wantValue := range want {
		if got := metadata[key]; got != wantValue {
			t.Fatalf("response metadata[%q] = %q, want %q", key, got, wantValue)
		}
	}
}

func TestSuccessStdoutJSON(t *testing.T) {
	encoded := SuccessStdoutJSON("hello", "sess-1")
	var payload resultPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Result != "hello" || payload.SessionID != "sess-1" {
		t.Fatalf("payload = %#v", payload)
	}
}
