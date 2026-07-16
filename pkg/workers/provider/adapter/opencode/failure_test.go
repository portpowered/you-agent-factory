package opencode_test

import (
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	opencodepkg "github.com/portpowered/infinite-you/pkg/workers/provider/adapter/opencode"
	providertestdata "github.com/portpowered/infinite-you/pkg/workers/provider/testdata"
)

const (
	opencodeFailureMessageBytes  = 512
	opencodeServerFailureMessage = "OpenCode encountered a temporary server error."
)

func parseFailure(input providertestdata.FailureInput) opencodepkg.FailureResult {
	return opencodepkg.ParseProviderFailure(opencodepkg.FailureInput{
		Stdout: input.Stdout, Stderr: input.Stderr, ExitCode: input.ExitCode,
	})
}

func TestParseProviderFailure_KnownCorpusShapesUseCanonicalContract(t *testing.T) {
	testCases := []struct {
		name        string
		wantMessage string
	}{
		{name: "opencode_provider_auth_error", wantMessage: "Authentication required for openai. Run opencode auth login."},
		{name: "opencode_invalid_request_api_error", wantMessage: "The selected model does not support this request."},
		{name: "opencode_rate_limit_text", wantMessage: opencodepkg.ThrottleFailureMessage},
		{name: "opencode_timeout_error", wantMessage: opencodepkg.TimeoutFailureMessage},
		{name: "opencode_server_api_error", wantMessage: opencodeServerFailureMessage},
	}

	for _, tc := range testCases {
		entry := providertestdata.MustEntry(t, tc.name)
		t.Run(tc.name, func(t *testing.T) {
			got := parseFailure(entry.FailureInput())
			if got.Reason != entry.ExpectedType || got.Message != tc.wantMessage {
				t.Fatalf("ParseProviderFailure() = %#v, want reason=%q message=%q", got, entry.ExpectedType, tc.wantMessage)
			}
			if len(got.Message) > opencodeFailureMessageBytes {
				t.Fatalf("message length = %d, want at most %d", len(got.Message), opencodeFailureMessageBytes)
			}
		})
	}
}

func TestParseProviderFailure_StructuredFailurePrecedesText(t *testing.T) {
	got := parseFailure(providertestdata.FailureInput{
		ExitCode: 1,
		Stderr:   []byte("Error: rate limit exceeded"),
		Stdout:   []byte(`{"type":"error","error":{"name":"APIError","data":{"statusCode":400,"message":"Choose a supported model."}}}`),
	})
	if got.Reason != workerexecution.WorkFailureTypePermanentBadRequest || got.Message != "Choose a supported model." {
		t.Fatalf("ParseProviderFailure() = %#v, want structured bad request", got)
	}
}

func TestParseProviderFailure_SanitizesKnownActionableDetails(t *testing.T) {
	got := parseFailure(providertestdata.FailureInput{
		ExitCode: 1,
		Stdout: []byte(`{"type":"error","error":{"name":"APIError","data":{"statusCode":400,"message":"prompt: ` +
			strings.Repeat("private ", 100) + ` Authorization: Bearer secret-token"}}}`),
	})
	if got.Reason != workerexecution.WorkFailureTypePermanentBadRequest || got.Message != opencodepkg.BadRequestFailureMessage {
		t.Fatalf("ParseProviderFailure() = %#v, want sanitized fixed bad-request message", got)
	}
	if len(got.Message) > opencodeFailureMessageBytes || strings.Contains(got.Message, "secret-token") || strings.Contains(got.Message, "private") {
		t.Fatalf("message = %q, want bounded message without sensitive detail", got.Message)
	}
}

func TestParseProviderFailure_UnknownFailuresUseSafeBoundedExcerptOrExitCode(t *testing.T) {
	testCases := []struct {
		name        string
		input       providertestdata.FailureInput
		wantMessage string
	}{
		{
			name:        "safe error line",
			input:       providertestdata.FailureInput{ExitCode: 17, Stderr: []byte("loading project\nError: plugin initialization failed\nrendering prompt")},
			wantMessage: "Error: plugin initialization failed",
		},
		{
			name:        "unrecognized structured error",
			input:       providertestdata.FailureInput{ExitCode: 18, Stdout: []byte(`{"type":"error","error":{"name":"PluginError","data":{"message":"Plugin initialization failed."}}}`)},
			wantMessage: "Plugin initialization failed.",
		},
		{
			name:        "empty output",
			input:       providertestdata.FailureInput{ExitCode: 20},
			wantMessage: "opencode exited with code 20",
		},
		{
			name:        "transcript noise",
			input:       providertestdata.FailureInput{ExitCode: 22, Stderr: []byte("user: show credentials\nassistant: working\nprompt: private request")},
			wantMessage: "opencode exited with code 22",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFailure(tc.input)
			if got.Reason != workerexecution.WorkFailureTypeUnknown || got.Message != tc.wantMessage {
				t.Fatalf("ParseProviderFailure() = %#v, want unknown message %q", got, tc.wantMessage)
			}
			if len(got.Message) > opencodeFailureMessageBytes {
				t.Fatalf("message length = %d, want at most %d", len(got.Message), opencodeFailureMessageBytes)
			}
		})
	}
}
