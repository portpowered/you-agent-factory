package providers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider"
)

const (
	alignmentPrivatePrompt   = "alignment-private-submitted-prompt-must-not-leak"
	alignmentPrivateResponse = "alignment-private-provider-response-body-must-not-leak"
)

func TestCodexFailureResolution_PreservesBoundedInternalCauseWithoutPublicLeakage(t *testing.T) {
	testCases := []struct {
		name            string
		result          provider.CommandResult
		input           provider.CodexFailureResolutionInput
		wantMessage     string
		wantType        workerexecution.WorkFailureType
		wantCausePrefix string
		forbidden       []string
	}{
		{
			name: "structured_stream_recognized",
			result: provider.CommandResult{
				ExitCode: 1,
				Stdout: codexStructuredStreamStdoutWithPrompt(
					"unexpected status 401",
					alignmentPrivatePrompt,
					alignmentPrivateResponse,
				),
				Stderr: []byte("ERROR: unexpected status 429\n"),
			},
			wantType:        workerexecution.WorkFailureTypeAuthFailure,
			wantMessage:     codexAuthFailureMessage,
			wantCausePrefix: "unexpected status 401",
			forbidden:       []string{alignmentPrivatePrompt, alignmentPrivateResponse},
		},
		{
			name: "process_exit_stderr_recognized",
			result: provider.CommandResult{
				ExitCode: 1,
				Stdout:   []byte(alignmentPrivateResponse + "\n"),
				Stderr:   []byte("ERROR: unexpected status 429\nprompt echo: " + alignmentPrivatePrompt + "\n"),
			},
			wantType:        workerexecution.WorkFailureTypeThrottled,
			wantMessage:     codexThrottleFailureMessage,
			wantCausePrefix: "unexpected status 429",
			forbidden:       []string{alignmentPrivatePrompt, alignmentPrivateResponse},
		},
		{
			name: "exit_fallback_without_echoing_prompt_or_response",
			result: provider.CommandResult{
				ExitCode: 17,
				Stdout:   []byte(alignmentPrivateResponse + "\n"),
				Stderr:   []byte("prompt echo: " + alignmentPrivatePrompt + "\n"),
			},
			wantType:        workerexecution.WorkFailureTypeUnknown,
			wantMessage:     codexUnknownFailureMessage,
			wantCausePrefix: "exit code 17",
			forbidden:       []string{alignmentPrivatePrompt, alignmentPrivateResponse},
		},
		{
			name: "timeout_wins_with_safe_cause",
			result: provider.CommandResult{
				ExitCode: 124,
				Stdout:   codexStructuredStreamStdout(alignmentPrivateResponse),
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
			input:           provider.CodexFailureResolutionInput{CommandError: context.DeadlineExceeded},
			wantType:        workerexecution.WorkFailureTypeTimeout,
			wantMessage:     "Codex execution timed out.",
			wantCausePrefix: "context deadline exceeded",
			forbidden:       []string{alignmentPrivatePrompt, alignmentPrivateResponse},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok := provider.ResolveCodexProviderFailure(tc.result, tc.input)
			if !ok {
				t.Fatal("ResolveCodexProviderFailure() ok = false, want true")
			}
			if resolved.Result.Reason != tc.wantType || resolved.Result.Message != tc.wantMessage {
				t.Fatalf("resolution = %#v, want type=%q message=%q", resolved, tc.wantType, tc.wantMessage)
			}
			if resolved.InternalCause == "" {
				t.Fatal("expected bounded internal cause on winning resolution")
			}
			if tc.wantCausePrefix != "" && !strings.HasPrefix(resolved.InternalCause, tc.wantCausePrefix) {
				t.Fatalf("internal cause = %q, want prefix %q", resolved.InternalCause, tc.wantCausePrefix)
			}

			providerErr := provider.NewProviderErrorFromResult(resolved.Result, provider.ProviderFailureInternalCauseError(resolved.InternalCause))
			if providerErr.Cause == nil {
				t.Fatal("expected provider error cause for maintainer diagnostics")
			}
			if providerErr.Message == providerErr.Cause.Error() && tc.wantMessage != tc.wantCausePrefix {
				t.Fatalf("public message must not echo internal cause verbatim: %q", providerErr.Message)
			}

			fixture := provider.CodexSanitizedFailureFixtureFromResolution(resolved)
			assertCodexSanitizedFailureFixtureSafe(t, fixture, tc.forbidden)
		})
	}
}

func TestCodexFailureReportingPaths_SanitizedFixturesRetainInternalCauseWithoutLeakage(t *testing.T) {
	testCases := []struct {
		name          string
		streamMessage string
		exitStderr    string
		exitCode      int
	}{
		{
			name:          "auth",
			streamMessage: "unexpected status 401",
			exitStderr:    "ERROR: unexpected status 401\n",
			exitCode:      1,
		},
		{
			name:          "malformed_with_private_echo",
			streamMessage: "operation failed with " + alignmentPrivatePrompt,
			exitStderr:    `ERROR: {"type":"error","error":{"message":"` + alignmentPrivateResponse + `"}}` + "\n",
			exitCode:      1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout := codexStructuredStreamStdoutWithPrompt(tc.streamMessage, alignmentPrivatePrompt, alignmentPrivateResponse)
			exitResult := codexProcessExitResult(tc.exitStderr, tc.exitCode)

			streamResolution, ok := provider.ResolveCodexProviderFailure(provider.CommandResult{
				ExitCode: 1,
				Stdout:   stdout,
			}, provider.CodexFailureResolutionInput{})
			if !ok {
				t.Fatal("ResolveCodexProviderFailure() ok = false, want true stream resolution")
			}
			streamFixture := provider.CodexSanitizedFailureFixtureFromResolution(streamResolution)

			exitResolution, ok := provider.ResolveCodexProviderFailure(exitResult, provider.CodexFailureResolutionInput{})
			if !ok {
				t.Fatal("ResolveCodexProviderFailure() ok = false, want true exit resolution")
			}
			exitFixture := provider.CodexSanitizedFailureFixtureFromResolution(exitResolution)

			for label, fixture := range map[string]provider.CodexSanitizedFailureFixture{
				"structured-stream": streamFixture,
				"process-exit":      exitFixture,
			} {
				t.Run(label, func(t *testing.T) {
					if fixture.InternalCause == "" {
						t.Fatal("expected sanitized fixture to retain internal cause metadata")
					}
					assertCodexSanitizedFailureFixtureSafe(t, fixture, []string{alignmentPrivatePrompt, alignmentPrivateResponse})
				})
			}
		})
	}
}

func TestNormalizeProviderExitFailure_CodexPreservesInternalCauseWithoutPublicLeakage(t *testing.T) {
	result := provider.CommandResult{
		ExitCode: 1,
		Stdout:   []byte(alignmentPrivateResponse + "\n"),
		Stderr:   []byte("ERROR: unexpected status 503\nprompt echo: " + alignmentPrivatePrompt + "\n"),
	}
	providerErr := provider.NormalizeProviderExitFailure(string(modelprovider.ProviderCodex), result, nil, nil)
	if providerErr == nil {
		t.Fatal("expected provider error")
	}
	if providerErr.Type != workerexecution.WorkFailureTypeInternalServerError || providerErr.Message != codexServerFailureMessage {
		t.Fatalf("provider error = %#v, want safe server failure", providerErr)
	}
	if providerErr.Cause == nil || !strings.Contains(providerErr.Cause.Error(), "unexpected status 503") {
		t.Fatalf("provider cause = %v, want recognized stderr diagnostic", providerErr.Cause)
	}
	fixture := provider.CodexSanitizedFailureFixtureFromProviderError(providerErr)
	assertCodexSanitizedFailureFixtureSafe(t, fixture, []string{alignmentPrivatePrompt, alignmentPrivateResponse})
}

func codexStructuredStreamStdoutWithPrompt(message, prompt, response string) []byte {
	record, err := json.Marshal(map[string]any{
		"type":  "turn.failed",
		"error": map[string]string{"message": message},
		"echo": map[string]string{
			"prompt":   prompt,
			"response": response,
		},
	})
	if err != nil {
		panic(err)
	}
	return append(record, '\n')
}

func assertCodexSanitizedFailureFixtureSafe(t *testing.T, fixture provider.CodexSanitizedFailureFixture, forbidden []string) {
	t.Helper()
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode sanitized fixture: %v", err)
	}
	payload := string(encoded)
	for _, secret := range forbidden {
		if secret == "" {
			continue
		}
		if strings.Contains(fixture.Message, secret) || strings.Contains(fixture.InternalCause, secret) || strings.Contains(payload, secret) {
			t.Fatalf("sanitized fixture leaked %q: %s", secret, payload)
		}
	}
	publicDetail := provider.SafeProviderFailureDetail(provider.NewProviderError(fixture.Type, fixture.Message, provider.ProviderFailureInternalCauseError(fixture.InternalCause)))
	if publicDetail == nil {
		t.Fatal("expected safe public failure detail")
	}
	for _, secret := range forbidden {
		if strings.Contains(publicDetail.Message, secret) {
			t.Fatalf("public failure detail leaked %q: %#v", secret, publicDetail)
		}
	}
}
