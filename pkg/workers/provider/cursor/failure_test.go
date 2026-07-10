package cursor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestParseProviderFailure_ClassifiesTerminalResultRecords(t *testing.T) {
	testCases := []struct {
		name    string
		subtype string
		result  string
		want    interfaces.WorkFailureType
	}{
		{name: "Authentication", subtype: "authentication_error", result: "Please sign in to Cursor", want: interfaces.WorkFailureTypeAuthFailure},
		{name: "InvalidRequest", subtype: "invalid_request_error", result: "The selected model is unsupported", want: interfaces.WorkFailureTypePermanentBadRequest},
		{name: "Capacity", subtype: "error", result: "Model capacity is temporarily exhausted", want: interfaces.WorkFailureTypeThrottled},
		{name: "Timeout", subtype: "error", result: "Request timed out while waiting for Cursor", want: interfaces.WorkFailureTypeTimeout},
		{name: "Server", subtype: "api_error", result: "Provider unavailable", want: interfaces.WorkFailureTypeInternalServerError},
		{name: "Unknown", subtype: "canceled", result: "Cursor stopped the request", want: interfaces.WorkFailureTypeUnknown},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout := []byte(`{"type":"result","subtype":"` + tc.subtype + `","is_error":true,"result":"` + tc.result + `","session_id":"cursor-failure-session"}`)
			got := ParseProviderFailure(FailureInput{Stdout: stdout, ExitCode: 1})
			if got.Reason != tc.want {
				t.Fatalf("reason = %q, want %q", got.Reason, tc.want)
			}
			if got.Message != tc.result {
				t.Fatalf("message = %q, want safe result text %q", got.Message, tc.result)
			}
			if got.ProviderSession == nil || got.ProviderSession.ID != "cursor-failure-session" {
				t.Fatalf("provider session = %#v, want cursor-failure-session", got.ProviderSession)
			}
		})
	}
}

func TestParseProviderFailure_IsErrorTrueMakesSuccessSubtypeTerminalFailure(t *testing.T) {
	got := ParseProviderFailure(FailureInput{
		Stdout:   []byte(`{"type":"result","subtype":"success","is_error":true,"result":"Request timed out","session_id":"cursor-session"}`),
		ExitCode: 1,
	})
	if got.Reason != interfaces.WorkFailureTypeTimeout || got.Message != "Request timed out" {
		t.Fatalf("failure = %#v, want timeout from is_error terminal record", got)
	}
}

func TestParseProviderFailure_SelectsLastTerminalResultAmidStreamNoise(t *testing.T) {
	stdout := []byte(strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"private prompt"}]}}`,
		`{malformed}`,
		`{"type":"result","subtype":"error","is_error":true,"result":"Old server failure","session_id":"old-session"}`,
		`{"type":"tool_call","subtype":"completed","result":"tool transcript"}`,
		`{"type":"result","subtype":"rate_limit_error","is_error":true,"result":"Cursor capacity is busy","session_id":"final-session"}`,
		`{"type":"progress","message":"cleanup noise"}`,
	}, "\n"))

	got := ParseProviderFailure(FailureInput{
		Stdout:   stdout,
		Stderr:   []byte("unrelated authentication failure"),
		ExitCode: 1,
	})
	if got.Reason != interfaces.WorkFailureTypeThrottled || got.Message != "Cursor capacity is busy" {
		t.Fatalf("failure = %#v, want final structured throttling result", got)
	}
	if got.ProviderSession == nil || got.ProviderSession.ID != "final-session" {
		t.Fatalf("provider session = %#v, want final-session", got.ProviderSession)
	}
}

func TestParseProviderFailure_SuccessTerminalRecordDoesNotReuseEarlierFailure(t *testing.T) {
	stdout := []byte(
		`{"type":"result","subtype":"error","is_error":true,"result":"old error"}` + "\n" +
			`{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"cursor-session"}`,
	)
	got := ParseProviderFailure(FailureInput{Stdout: stdout, ExitCode: 9})
	if got.Reason != interfaces.WorkFailureTypeUnknown || got.Message != "cursor exited with code 9" {
		t.Fatalf("failure = %#v, want deterministic exit fallback", got)
	}
}

func TestParseProviderFailure_NormalizesBoundsAndRejectsUnsafeResultText(t *testing.T) {
	t.Run("NormalizesControlsAndBoundsUnicode", func(t *testing.T) {
		result := "server\x00 failure\n" + strings.Repeat("界", FailureMessageLimit+20)
		stdout := terminalFailureJSONForTest(t, "api_error", result)
		got := ParseProviderFailure(FailureInput{Stdout: stdout, ExitCode: 1})
		if strings.ContainsAny(got.Message, "\x00\n") {
			t.Fatalf("message contains control characters: %q", got.Message)
		}
		if len([]rune(strings.TrimSuffix(got.Message, "..."))) != FailureMessageLimit {
			t.Fatalf("bounded message rune length = %d, want %d", len([]rune(strings.TrimSuffix(got.Message, "..."))), FailureMessageLimit)
		}
	})

	t.Run("UnsafeCredentialTextUsesGuidance", func(t *testing.T) {
		stdout := terminalFailureJSONForTest(t, "authentication_error", "Authorization: Bearer secret-token")
		got := ParseProviderFailure(FailureInput{Stdout: stdout, ExitCode: 1})
		if got.Message != cursorAuthFailureMessage || strings.Contains(got.Message, "secret-token") {
			t.Fatalf("message = %q, want safe authentication guidance", got.Message)
		}
	})

	t.Run("EmptyTextUsesReasonSpecificGuidance", func(t *testing.T) {
		stdout := terminalFailureJSONForTest(t, "rate_limit_error", "")
		got := ParseProviderFailure(FailureInput{Stdout: stdout, ExitCode: 1})
		if got.Message != cursorThrottleFailureMessage {
			t.Fatalf("message = %q, want %q", got.Message, cursorThrottleFailureMessage)
		}
	})
}

func terminalFailureJSONForTest(t *testing.T, subtype, result string) []byte {
	t.Helper()
	payload := resultPayload{
		Type: ResultTypeResult, Subtype: subtype, IsError: true, Result: result, SessionID: "cursor-session",
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal terminal failure: %v", err)
	}
	return encoded
}
