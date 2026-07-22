package kiro_test

import (
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	kiropkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/kiro"
	providertestdata "github.com/portpowered/infinite-you/pkg/services/workers/provider/testdata"
)

func parseFailure(input providertestdata.FailureInput) kiropkg.FailureResult {
	return kiropkg.ParseProviderFailure(kiropkg.FailureInput{
		Stdout: input.Stdout, Stderr: input.Stderr, ExitCode: input.ExitCode,
	})
}

func knownKiroMessage(reason workerexecution.WorkFailureType) string {
	switch reason {
	case workerexecution.WorkFailureTypeAuthFailure:
		return "Kiro authentication failed. Sign in again and retry."
	case workerexecution.WorkFailureTypePermanentBadRequest:
		return "Kiro rejected the request as invalid."
	case workerexecution.WorkFailureTypeThrottled:
		return "Kiro is temporarily unavailable due to usage or capacity limits."
	case workerexecution.WorkFailureTypeTimeout:
		return kiropkg.TimeoutFailureMessage
	case workerexecution.WorkFailureTypeInternalServerError:
		return "Kiro encountered a temporary service error."
	default:
		return ""
	}
}

func TestParseProviderFailure_KnownCorpusShapesUseCanonicalContract(t *testing.T) {
	for _, name := range []string{
		"kiro_structured_authentication_error",
		"kiro_structured_invalid_request_stdout",
		"kiro_text_authentication_stdout",
		"kiro_structured_throttle_precedes_text",
		"kiro_text_capacity_error",
		"kiro_text_timeout_malformed_structured",
		"kiro_structured_service_unavailable",
	} {
		entry := providertestdata.MustEntry(t, name)
		t.Run(name, func(t *testing.T) {
			got := parseFailure(entry.FailureInput())
			if got.Reason != entry.ExpectedType {
				t.Fatalf("Reason = %q, want %q", got.Reason, entry.ExpectedType)
			}
			if got.Message != knownKiroMessage(entry.ExpectedType) {
				t.Fatalf("Message = %q, want %q", got.Message, knownKiroMessage(entry.ExpectedType))
			}
			for _, rejected := range entry.RejectMessageContains {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestParseProviderFailure_UnknownFailuresUseBoundedParserMessages(t *testing.T) {
	testCases := map[string]string{
		"kiro_unknown_stderr_excerpt_precedes_stdout":     "Kiro error: model registry handshake failed",
		"kiro_unknown_stdout_excerpt_after_unsafe_stderr": "Kiro error: plugin bridge failed",
		"kiro_unknown_noise_only_exit_fallback":           "kiro-cli exited with code 11",
	}

	for name, wantMessage := range testCases {
		entry := providertestdata.MustEntry(t, name)
		t.Run(name, func(t *testing.T) {
			got := parseFailure(entry.FailureInput())
			if got.Reason != workerexecution.WorkFailureTypeUnknown || got.Message != wantMessage {
				t.Fatalf("ParseProviderFailure() = %#v, want unknown message %q", got, wantMessage)
			}
			for _, rejected := range entry.RejectMessageContains {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestParseProviderFailure_StructuredStatusAndPrefixSignals(t *testing.T) {
	testCases := []struct {
		name       string
		stderr     string
		wantReason workerexecution.WorkFailureType
	}{
		{
			name:       "Status422",
			stderr:     `KIRO_ERROR: {"status":422,"message":"field missing"}`,
			wantReason: workerexecution.WorkFailureTypePermanentBadRequest,
		},
		{
			name:       "Status408",
			stderr:     `Error: {"status_code":408}`,
			wantReason: workerexecution.WorkFailureTypeTimeout,
		},
		{
			name:       "Status503",
			stderr:     `ERROR: {"statusCode":"503","name":"service_unavailable"}`,
			wantReason: workerexecution.WorkFailureTypeInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFailure(providertestdata.FailureInput{ExitCode: 1, Stderr: []byte(tc.stderr)})
			if got.Reason != tc.wantReason {
				t.Fatalf("ParseProviderFailure() = %#v, want reason=%q", got, tc.wantReason)
			}
		})
	}
}

func TestParseProviderFailure_ExitCode124MapsToTimeout(t *testing.T) {
	got := parseFailure(providertestdata.FailureInput{ExitCode: 124})
	if got.Reason != workerexecution.WorkFailureTypeTimeout || got.Message != kiropkg.TimeoutFailureMessage {
		t.Fatalf("ParseProviderFailure() = %#v, want timeout", got)
	}
}

func TestParseProviderFailure_MissingOutputFallsBackToExitCode(t *testing.T) {
	got := parseFailure(providertestdata.FailureInput{ExitCode: 9})
	if got.Reason != workerexecution.WorkFailureTypeUnknown || got.Message != "kiro-cli exited with code 9" {
		t.Fatalf("ParseProviderFailure() = %#v, want exit fallback", got)
	}
}
