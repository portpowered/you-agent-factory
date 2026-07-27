package cursor

import (
	"encoding/json"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestParseProviderFailure_ClassifiesTerminalResultRecords(t *testing.T) {
	testCases := []struct {
		name    string
		subtype string
		result  string
		want    workerexecution.WorkFailureType
	}{
		{name: "Authentication", subtype: "authentication_error", result: "Please sign in to Cursor", want: workerexecution.WorkFailureTypeAuthFailure},
		{name: "InvalidRequest", subtype: "invalid_request_error", result: "The selected model is unsupported", want: workerexecution.WorkFailureTypePermanentBadRequest},
		{name: "Capacity", subtype: "error", result: "Model capacity is temporarily exhausted", want: workerexecution.WorkFailureTypeThrottled},
		{name: "Timeout", subtype: "error", result: "Request timed out while waiting for Cursor", want: workerexecution.WorkFailureTypeTimeout},
		{name: "Server", subtype: "api_error", result: "Provider unavailable", want: workerexecution.WorkFailureTypeInternalServerError},
		{name: "Unknown", subtype: "canceled", result: "Cursor stopped the request", want: workerexecution.WorkFailureTypeUnknown},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout := []byte(`{"type":"result","subtype":"` + tc.subtype + `","is_error":true,"result":"` + tc.result + `","session_id":"cursor-failure-session"}`)
			got := ParseProviderFailure(FailureInput{Stdout: stdout, ExitCode: 1})
			if got.Reason != tc.want {
				t.Fatalf("reason = %q, want %q", got.Reason, tc.want)
			}
			if got.Message != cursorFailureGuidance(tc.want) {
				t.Fatalf("message = %q, want canonical guidance %q", got.Message, cursorFailureGuidance(tc.want))
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
	if got.Reason != workerexecution.WorkFailureTypeTimeout || got.Message != cursorTimeoutFailureMessage {
		t.Fatalf("failure = %#v, want timeout from is_error terminal record", got)
	}
}

func TestParseProviderFailure_ExitCode124MapsToCanonicalTimeout(t *testing.T) {
	got := ParseProviderFailure(FailureInput{
		Stderr:   []byte("arbitrary local process detail"),
		ExitCode: 124,
	})
	if got.Reason != workerexecution.WorkFailureTypeTimeout ||
		got.Message != cursorTimeoutFailureMessage {
		t.Fatalf("failure = %#v, want canonical timeout", got)
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
	if got.Reason != workerexecution.WorkFailureTypeThrottled || got.Message != cursorThrottleFailureMessage {
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
	if got.Reason != workerexecution.WorkFailureTypeUnknown || got.Message != cursorUnknownFailureMessage {
		t.Fatalf("failure = %#v, want deterministic exit fallback", got)
	}
}

func TestParseProviderFailure_NormalizesBoundsAndRejectsUnsafeResultText(t *testing.T) {
	t.Run("NeverPublishesNativeUnicodeOrControls", func(t *testing.T) {
		result := "server\x00 failure\n" + strings.Repeat("界", FailureMessageLimit+20)
		stdout := terminalFailureJSONForTest(t, "api_error", result)
		got := ParseProviderFailure(FailureInput{Stdout: stdout, ExitCode: 1})
		if got.Message != cursorServerFailureMessage || strings.Contains(got.Message, "界") {
			t.Fatalf("message = %q, want canonical server guidance", got.Message)
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

func TestParseProviderFailure_ClassifiesCursorStderrWithDeterministicPrecedence(t *testing.T) {
	testCases := []struct {
		name        string
		stderr      string
		wantReason  workerexecution.WorkFailureType
		wantMessage string
	}{
		{name: "Authentication", stderr: "Cursor authentication failed; sign in again", wantReason: workerexecution.WorkFailureTypeAuthFailure, wantMessage: cursorAuthFailureMessage},
		{name: "InvalidConfiguration", stderr: "invalid configuration: unsupported model", wantReason: workerexecution.WorkFailureTypePermanentBadRequest, wantMessage: cursorBadRequestFailureMessage},
		{name: "Capacity", stderr: "Cursor model capacity is exhausted", wantReason: workerexecution.WorkFailureTypeThrottled, wantMessage: cursorThrottleFailureMessage},
		{name: "Timeout", stderr: "Cursor request timed out", wantReason: workerexecution.WorkFailureTypeTimeout, wantMessage: cursorTimeoutFailureMessage},
		{name: "Server", stderr: "Cursor provider unavailable with status 503", wantReason: workerexecution.WorkFailureTypeInternalServerError, wantMessage: cursorServerFailureMessage},
		{name: "WindowsCommandLineTooLong", stderr: "The command line is too long.", wantReason: workerexecution.WorkFailureTypeCommandLineTooLong, wantMessage: cursorCommandLineTooLongMessage},
		{
			name:        "CategoryPrecedenceAndDuplicateNoise",
			stderr:      "cleanup noise\nrate limit reached\nRATE LIMIT REACHED\nauthentication failed\nprocess already exited",
			wantReason:  workerexecution.WorkFailureTypeAuthFailure,
			wantMessage: cursorAuthFailureMessage,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseProviderFailure(FailureInput{Stderr: []byte(tc.stderr), ExitCode: 1})
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("failure = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
		})
	}
}

func TestParseProviderFailure_UsesStderrBeforeStdoutAndSafeUnknownFallback(t *testing.T) {
	t.Run("UnrecognizedStderrDoesNotHideRecognizedStdout", func(t *testing.T) {
		got := ParseProviderFailure(FailureInput{
			Stderr:   []byte("Cursor rejected this request"),
			Stdout:   []byte("rate limit reached"),
			ExitCode: 1,
		})
		if got.Reason != workerexecution.WorkFailureTypeThrottled || got.Message != cursorThrottleFailureMessage {
			t.Fatalf("failure = %#v, want recognized stdout throttling result", got)
		}
	})

	t.Run("UnknownNeverUsesNativeText", func(t *testing.T) {
		unknown := strings.Repeat("x", FailureMessageLimit+20)
		got := ParseProviderFailure(FailureInput{
			Stderr:   []byte("cleanup noise\n" + unknown + "\n" + unknown),
			ExitCode: 7,
		})
		if got.Reason != workerexecution.WorkFailureTypeUnknown || got.Message != cursorUnknownFailureMessage {
			t.Fatalf("failure = %#v, want deterministic unknown guidance", got)
		}
	})

	t.Run("NoSafeExcerptUsesExitCode", func(t *testing.T) {
		got := ParseProviderFailure(FailureInput{Stderr: []byte("Authorization: Bearer secret-token"), ExitCode: 7})
		if got.Reason != workerexecution.WorkFailureTypeAuthFailure || got.Message != cursorAuthFailureMessage {
			t.Fatalf("failure = %#v, want safe auth guidance", got)
		}
	})
}

func TestParseProviderFailure_DoesNotSurfaceMalformedStructuredRecords(t *testing.T) {
	privatePrompt := "deploy production using the customer launch phrase"
	got := ParseProviderFailure(FailureInput{
		Stdout:   []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"` + privatePrompt + `"}]}`),
		ExitCode: 7,
	})
	if got.Reason != workerexecution.WorkFailureTypeUnknown || got.Message != cursorUnknownFailureMessage {
		t.Fatalf("failure = %#v, want exit fallback for malformed structured output", got)
	}
	if strings.Contains(got.Message, privatePrompt) {
		t.Fatalf("message = %q, must not surface malformed assistant content", got.Message)
	}
}

func TestParseProviderFailure_DoesNotBorrowCodexOnlyClassification(t *testing.T) {
	got := ParseProviderFailure(FailureInput{
		Stderr:   []byte("The gpt-5.6-sol model requires a newer version of Codex"),
		ExitCode: 1,
	})
	if got.Reason != workerexecution.WorkFailureTypeUnknown || got.Message != cursorUnknownFailureMessage {
		t.Fatalf("failure = %#v, want Cursor-owned unknown classification", got)
	}
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
