package exitfailure_test

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	claudepkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/claude/exitfailure"
)

const (
	failureMessageBytes      = 1024
	throttleFailureMessage   = claudepkg.ThrottleFailureMessage
	timeoutFailureMessage    = claudepkg.TimeoutFailureMessage
	badRequestFailureMessage = "Claude rejected the request as invalid."
	serverFailureMessage     = "Claude encountered a temporary server error."
)

func parseFailure(stdout, stderr []byte, exitCode int) claudepkg.FailureResult {
	return claudepkg.ParseProviderFailure(claudepkg.FailureInput{
		Stdout: stdout, Stderr: stderr, ExitCode: exitCode,
	})
}

func boundUTF8Bytes(message string, limit int) string {
	if len(message) <= limit {
		return message
	}
	for limit > 0 && !utf8.ValidString(message[:limit]) {
		limit--
	}
	return message[:limit]
}

func TestParseProviderFailure_StructuredTypesAndStatusesAreCanonical(t *testing.T) {
	testCases := []struct {
		name        string
		stderr      string
		wantReason  workerexecution.WorkFailureType
		wantMessage string
	}{
		{
			name:        "AuthenticationTypePreservesActionableMessage",
			stderr:      `API Error: 500 {"type":"error","error":{"type":"authentication_error","message":"  Sign in again\nwith Claude Code.  "}}`,
			wantReason:  workerexecution.WorkFailureTypeAuthFailure,
			wantMessage: "Sign in again with Claude Code.",
		},
		{
			name:        "PermissionStatusFallbackPreservesActionableMessage",
			stderr:      `API Error: 403 {"type":"error","error":{"message":"Ask an organization owner to grant access."}}`,
			wantReason:  workerexecution.WorkFailureTypeAuthFailure,
			wantMessage: "Ask an organization owner to grant access.",
		},
		{
			name:        "InvalidRequestPreservesCorrection",
			stderr:      `API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Reduce the request size below 20 MB."}}`,
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: "Reduce the request size below 20 MB.",
		},
		{
			name:        "RequestSizeStatusFallback",
			stderr:      `API Error: 413 {"type":"error","error":{"message":"The request is too large; remove an attachment."}}`,
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: "The request is too large; remove an attachment.",
		},
		{
			name:        "RateLimitUsesStableGuidance",
			stderr:      `API Error: 429 {"type":"error","error":{"type":"rate_limit_error","message":"rate limit exceeded"}}`,
			wantReason:  workerexecution.WorkFailureTypeThrottled,
			wantMessage: throttleFailureMessage,
		},
		{
			name:        "OverloadTypeWinsOverConflictingStatus",
			stderr:      `API Error: 401 {"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`,
			wantReason:  workerexecution.WorkFailureTypeThrottled,
			wantMessage: throttleFailureMessage,
		},
		{
			name:        "ServerStatusFallback",
			stderr:      `API Error: 502 {"type":"error","error":{"message":"gateway included internal diagnostics"}}`,
			wantReason:  workerexecution.WorkFailureTypeInternalServerError,
			wantMessage: serverFailureMessage,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFailure(nil, []byte(tc.stderr), 1)
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("ParseProviderFailure() = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
		})
	}
}

func TestParseProviderFailure_StructuredRecordPrecedesSurroundingText(t *testing.T) {
	stderr := []byte(strings.Join([]string{
		"cleanup mentioned 429 rate limit and forbidden credentials",
		`API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Choose a model available to this project."}}`,
		"post-command timeout while deleting a temporary directory",
	}, "\n"))

	got := parseFailure(nil, stderr, 1)
	if got.Reason != workerexecution.WorkFailureTypePermanentBadRequest || got.Message != "Choose a model available to this project." {
		t.Fatalf("ParseProviderFailure() = %#v, want structured invalid-request result", got)
	}
}

func TestParseProviderFailure_BoundsAndRejectsCredentialBearingMessages(t *testing.T) {
	longCorrection := strings.Repeat("remove one attachment ", 100)
	testCases := []struct {
		name        string
		message     string
		wantMessage string
	}{
		{
			name:        "BoundsNormalizedActionableDetail",
			message:     longCorrection,
			wantMessage: boundUTF8Bytes(strings.TrimSpace(longCorrection), failureMessageBytes),
		},
		{
			name:        "CredentialSignalUsesProductFallback",
			message:     "Replace Authorization: Bearer sk-ant-private",
			wantMessage: badRequestFailureMessage,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{
				"type": "error",
				"error": map[string]string{
					"type":    "invalid_request_error",
					"message": tc.message,
				},
			})
			if err != nil {
				t.Fatalf("marshal fixture: %v", err)
			}
			got := parseFailure(nil, []byte("API Error: 400 "+string(payload)), 1)
			if got.Message != tc.wantMessage || len(got.Message) > failureMessageBytes {
				t.Fatalf("Message = %q (%d bytes), want %q bounded to %d bytes", got.Message, len(got.Message), tc.wantMessage, failureMessageBytes)
			}
		})
	}
}

type fallbackTestCase struct {
	name        string
	stderr      []byte
	exitCode    int
	wantReason  workerexecution.WorkFailureType
	wantMessage string
}

func fallbackTestCases() []fallbackTestCase {
	return []fallbackTestCase{
		{
			name:        "TextConfigurationPreservesCorrection",
			stderr:      []byte("Configuration error: set ANTHROPIC_BASE_URL to a valid URL."),
			exitCode:    1,
			wantReason:  workerexecution.WorkFailureTypeMisconfigured,
			wantMessage: "Configuration error: set ANTHROPIC_BASE_URL to a valid URL.",
		},
		{
			name:        "TextAuthenticationPreservesCorrection",
			stderr:      []byte("Not logged in. Run /login to continue."),
			exitCode:    1,
			wantReason:  workerexecution.WorkFailureTypeAuthFailure,
			wantMessage: "Not logged in. Run /login to continue.",
		},
		{
			name:        "MalformedEnvelopeFallsThroughToTextSignal",
			stderr:      []byte(`API Error: 429 {"type":"error","error":{"type":"rate_limit_error"`),
			exitCode:    1,
			wantReason:  workerexecution.WorkFailureTypeThrottled,
			wantMessage: throttleFailureMessage,
		},
		{
			name: "LastRecognizedTextRecordExcludesCleanup",
			stderr: []byte(strings.Join([]string{
				"rate limit exceeded",
				"Invalid request: select a supported Claude model.",
				"cleanup finished after timeout directory removal",
			}, "\n")),
			exitCode:    1,
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: "Invalid request: select a supported Claude model.",
		},
		{
			name:        "TimeoutExitUsesStableGuidance",
			exitCode:    124,
			wantReason:  workerexecution.WorkFailureTypeTimeout,
			wantMessage: timeoutFailureMessage,
		},
		{
			name:        "SingleSafeUnknownUsesBoundedExcerpt",
			stderr:      []byte("some brand new claude failure"),
			exitCode:    9,
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: "Claude failed: some brand new claude failure",
		},
		{
			name:        "NoisyUnknownUsesExitCodeFallback",
			stderr:      []byte("User: private prompt\ncleanup complete"),
			exitCode:    7,
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: "claude exited with code 7",
		},
		{
			name:        "CredentialUnknownUsesExitCodeFallback",
			stderr:      []byte("Authorization: Bearer sk-ant-private"),
			exitCode:    2,
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: "claude exited with code 2",
		},
	}
}

func TestParseProviderFailure_TextMalformedNoisyAndUnknownOutcomes(t *testing.T) {
	for _, tc := range fallbackTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFailure(nil, tc.stderr, tc.exitCode)
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("ParseProviderFailure() = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
			if len(got.Message) > failureMessageBytes {
				t.Fatalf("Message length = %d, want at most %d bytes", len(got.Message), failureMessageBytes)
			}
		})
	}
}
