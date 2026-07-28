// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package provider

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestScriptWrapProvider_Infer_CodexExitFailuresNormalizeIntoSharedContract(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	testCases := []struct {
		name  string
		entry ProviderErrorCorpusEntry
	}{
		{
			name:  "Throttled_429",
			entry: providerErrorCorpusEntryForTest(t, "codex_status_429_too_many_requests"),
		},
		{
			name:  "TransientServerError_500",
			entry: providerErrorCorpusEntryForTest(t, "codex_internal_server_status_500"),
		},
		{
			name:  "TransientServerError_HighDemand",
			entry: providerErrorCorpusEntryForTest(t, "codex_high_demand_temporary_errors"),
		},
		{
			name:  "TransientServerError_WindowsExitCode4294967295",
			entry: providerErrorCorpusEntryForTest(t, "codex_windows_exit_code_4294967295"),
		},
		{
			name:  "CursorFamily_TransientServerError_HighDemand",
			entry: providerErrorCorpusEntryForTest(t, "cursor_high_demand_temporary_errors"),
		},
		{
			name:  "BadRequest_InvalidRequest",
			entry: providerErrorCorpusEntryForTest(t, "codex_invalid_request_error"),
		},
		{
			name:  "Timeout_MessageMatch",
			entry: providerErrorCorpusEntryForTest(t, "codex_timeout_waiting_for_provider"),
		},
		{
			name:  "AuthFailure_Unauthorized",
			entry: providerErrorCorpusEntryForTest(t, "codex_authentication_unauthorized"),
		},
		{
			name: "Unknown_Unclassified",
			entry: ProviderErrorCorpusEntry{
				Name:           "codex_unknown_unclassified",
				ExitCode:       1,
				Stderr:         `some brand new failure`,
				ExpectedType:   workerexecution.WorkFailureTypeUnknown,
				ExpectedFamily: workerexecution.WorkFailureFamilyTerminal,
			},
		},
	}

	for _, tc := range testCases {
		entryLabel := providerErrorCorpusEntryLabel(tc.entry)
		t.Run(entryLabel, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.entry.CommandResult()}
			provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("%s expected ProviderError, got %T", entryLabel, err)
			}
			if providerErr.Type != tc.entry.ExpectedType {
				t.Fatalf("%s Type = %q, want %q", entryLabel, providerErr.Type, tc.entry.ExpectedType)
			}
			if providerErr.Family != tc.entry.ExpectedFamily {
				t.Fatalf("%s Family = %q, want %q", entryLabel, providerErr.Family, tc.entry.ExpectedFamily)
			}
			if providerErr.ProviderSession != nil {
				t.Fatalf("%s expected provider session to be nil, got %#v", entryLabel, providerErr.ProviderSession)
			}
			decision := ClassifyProviderFailure(providerErr)
			if decision.Retryable != tc.entry.Retryable {
				t.Fatalf("%s Retryable = %t, want %t", entryLabel, decision.Retryable, tc.entry.Retryable)
			}
			if decision.TriggersThrottlePause != tc.entry.TriggersThrottlePause {
				t.Fatalf("%s TriggersThrottlePause = %t, want %t", entryLabel, decision.TriggersThrottlePause, tc.entry.TriggersThrottlePause)
			}
			wantTerminal := tc.entry.ExpectedFamily == workerexecution.WorkFailureFamilyTerminal
			if decision.Terminal != wantTerminal {
				t.Fatalf("%s Terminal = %t, want %t", entryLabel, decision.Terminal, wantTerminal)
			}
		})
	}
}
func TestScriptWrapProvider_Infer_CodexNormalizedRetryDecisionRegressions(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	testCases := []struct {
		name              string
		stderr            string
		wantType          workerexecution.WorkFailureType
		wantFamily        workerexecution.WorkFailureFamily
		wantRetryable     bool
		wantTerminal      bool
		wantThrottlePause bool
	}{
		{
			name:              "InternalServerError_HighDemandTemporaryErrors_RetriesWithoutThrottlePause",
			stderr:            `ERROR: We're currently experiencing high demand, which may cause temporary errors.`,
			wantType:          workerexecution.WorkFailureTypeInternalServerError,
			wantFamily:        workerexecution.WorkFailureFamilyRetryable,
			wantRetryable:     true,
			wantTerminal:      false,
			wantThrottlePause: false,
		},
		{
			name:              "InternalServerError_UnexpectedStatus500_RetriesWithoutThrottlePause",
			stderr:            `ERROR: unexpected status 500 Internal Server Error`,
			wantType:          workerexecution.WorkFailureTypeInternalServerError,
			wantFamily:        workerexecution.WorkFailureFamilyRetryable,
			wantRetryable:     true,
			wantTerminal:      false,
			wantThrottlePause: false,
		},
		{
			name:              "AuthFailure_UnexpectedStatus401_IsTerminal",
			stderr:            `ERROR: unexpected status 401 Unauthorized {"type":"authentication_error","message":"invalid api key"}`,
			wantType:          workerexecution.WorkFailureTypeAuthFailure,
			wantFamily:        workerexecution.WorkFailureFamilyTerminal,
			wantRetryable:     false,
			wantTerminal:      true,
			wantThrottlePause: false,
		},
		{
			name:              "PermanentBadRequest_UnexpectedStatus400_IsTerminal",
			stderr:            `ERROR: unexpected status 400 Bad Request {"type":"invalid_request_error","message":"bad request"}`,
			wantType:          workerexecution.WorkFailureTypePermanentBadRequest,
			wantFamily:        workerexecution.WorkFailureFamilyTerminal,
			wantRetryable:     false,
			wantTerminal:      true,
			wantThrottlePause: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{
				result: CommandResult{ExitCode: 1, Stderr: []byte(tc.stderr)},
			}, nil, nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantType:          tc.wantType,
				wantFamily:        tc.wantFamily,
				wantRetryable:     tc.wantRetryable,
				wantTerminal:      tc.wantTerminal,
				wantThrottlePause: tc.wantThrottlePause,
			})
		})
	}
}

func TestScriptWrapProvider_Infer_CodexWindowsCorpusEntryRemainsDistinctFromAuthFailure(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	testCases := []struct {
		entryName          string
		wantType           workerexecution.WorkFailureType
		wantRetryable      bool
		wantThrottlePause  bool
		wantRejectAuthType bool
	}{
		{
			entryName:          "codex_windows_exit_code_4294967295",
			wantType:           workerexecution.WorkFailureTypeInternalServerError,
			wantRetryable:      true,
			wantThrottlePause:  false,
			wantRejectAuthType: true,
		},
		{
			entryName:         "codex_authentication_unauthorized",
			wantType:          workerexecution.WorkFailureTypeAuthFailure,
			wantRetryable:     false,
			wantThrottlePause: false,
		},
	}

	for _, tc := range testCases {
		entry := providerErrorCorpusEntryForTest(t, tc.entryName)
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: entry.CommandResult()}
			provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("%s expected ProviderError, got %T", providerErrorCorpusEntryLabel(entry), err)
			}
			if providerErr.Type != tc.wantType {
				t.Fatalf("%s Type = %q, want %q", providerErrorCorpusEntryLabel(entry), providerErr.Type, tc.wantType)
			}
			if tc.wantRejectAuthType && providerErr.Type == workerexecution.WorkFailureTypeAuthFailure {
				t.Fatalf("%s Type = %q, want non-auth retryable failure", providerErrorCorpusEntryLabel(entry), providerErr.Type)
			}

			decision := ClassifyProviderFailure(providerErr)
			if decision.Retryable != tc.wantRetryable {
				t.Fatalf("%s Retryable = %t, want %t", providerErrorCorpusEntryLabel(entry), decision.Retryable, tc.wantRetryable)
			}
			if decision.TriggersThrottlePause != tc.wantThrottlePause {
				t.Fatalf("%s TriggersThrottlePause = %t, want %t", providerErrorCorpusEntryLabel(entry), decision.TriggersThrottlePause, tc.wantThrottlePause)
			}
		})
	}
}

func TestScriptWrapProvider_Infer_CodexWindowsExitCode4294967295Normalization(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	testCases := []struct {
		name              string
		result            CommandResult
		wantType          workerexecution.WorkFailureType
		wantFamily        workerexecution.WorkFailureFamily
		wantMessage       string
		wantRetryable     bool
		wantTerminal      bool
		wantThrottlePause bool
	}{
		{
			name: "NoAuditedSignal_UsesRetryableInternalServerError",
			result: CommandResult{
				ExitCode: codexWindowsProcessFailureExitCode,
				Stderr: []byte(strings.Join([]string{
					"OpenAI Codex v0.118.0 (research preview)",
					"--------",
					"provider: openai",
				}, "\n")),
			},
			wantType:          workerexecution.WorkFailureTypeInternalServerError,
			wantFamily:        workerexecution.WorkFailureFamilyRetryable,
			wantMessage:       codexServerFailureMessage,
			wantRetryable:     true,
			wantTerminal:      false,
			wantThrottlePause: false,
		},
		{
			name: "ExplicitAuthSignalStillWins",
			result: CommandResult{
				ExitCode: codexWindowsProcessFailureExitCode,
				Stderr:   []byte(`ERROR: unexpected status 401 Unauthorized {"type":"authentication_error","message":"invalid api key"}`),
			},
			wantType:          workerexecution.WorkFailureTypeAuthFailure,
			wantFamily:        workerexecution.WorkFailureFamilyTerminal,
			wantMessage:       codexAuthFailureMessage,
			wantRetryable:     false,
			wantTerminal:      true,
			wantThrottlePause: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantType:          tc.wantType,
				wantFamily:        tc.wantFamily,
				wantMessage:       tc.wantMessage,
				wantRetryable:     tc.wantRetryable,
				wantTerminal:      tc.wantTerminal,
				wantThrottlePause: tc.wantThrottlePause,
			})
		})
	}
}

func TestScriptWrapProvider_Infer_CodexExitFailureReturnsSafeBoundedMessage(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	capacityEntry := providerErrorCorpusEntryForTest(t, "codex_model_capacity_selected_model")
	capacityLine := providerErrorCorpusLastErrorLine(t, capacityEntry)

	testCases := []struct {
		name       string
		result     CommandResult
		wantLine   string
		rejectText string
	}{
		{
			name: "StdoutCapacityErrorAfterTranscript",
			result: CommandResult{
				ExitCode: 1,
				Stdout: []byte(strings.Join([]string{
					strings.Repeat("inference transcript token ", 4000),
					"agent looked successful",
					capacityLine,
				}, "\n")),
			},
			wantLine:   codexThrottleFailureMessage,
			rejectText: "inference transcript token",
		},
		{
			name: "StderrErrorBeforeTrailingLines",
			result: CommandResult{
				ExitCode: 1,
				Stderr: []byte(strings.Join([]string{
					"OpenAI Codex v0.118.0 (research preview)",
					"ERROR: The process with PID 1234 could not be terminated",
					"trailing cleanup note",
					"retry after cleanup",
				}, "\n")),
			},
			wantLine:   codexUnknownFailureMessage,
			rejectText: "trailing cleanup note",
		},
		{
			name: "FinalMatchingErrorWinsAcrossStreams",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("ERROR: First provider failure"),
				Stdout:   []byte("  ERROR: Final provider failure  \nnot final"),
			},
			wantLine:   codexUnknownFailureMessage,
			rejectText: "ERROR: First provider failure",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantMessage: tc.wantLine,
				rejectText:  tc.rejectText,
			})
		})
	}
}

// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestScriptWrapProvider_Infer_KnownCodexErrorLinesMapToProviderFailureCategories(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	capacityEntry := providerErrorCorpusEntryForTest(t, "codex_model_capacity_selected_model")
	capacityLine := providerErrorCorpusLastErrorLine(t, capacityEntry)

	testCases := []struct {
		name                 string
		result               CommandResult
		wantType             workerexecution.WorkFailureType
		wantFamily           workerexecution.WorkFailureFamily
		wantMessage          string
		wantRetryable        bool
		wantTerminal         bool
		wantThrottlePause    bool
		rejectMessageContent string
	}{
		{
			name: "SelectedModelCapacity_IsThrottled",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   capacityEntry.CommandResult().Stdout,
			},
			wantType:             workerexecution.WorkFailureTypeThrottled,
			wantFamily:           workerexecution.WorkFailureFamilyThrottle,
			wantMessage:          codexThrottleFailureMessage,
			wantRetryable:        true,
			wantThrottlePause:    true,
			rejectMessageContent: "thinking transcript",
		},
		{
			name: "CodexCommandTimeout_IsRetryableTimeout",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("ERROR: command timed out while waiting for codex"),
			},
			wantType:      workerexecution.WorkFailureTypeTimeout,
			wantFamily:    workerexecution.WorkFailureFamilyRetryable,
			wantMessage:   codexTimeoutFailureMessage,
			wantRetryable: true,
		},
		{
			name: "CodexContextDeadline_IsRetryableTimeout",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   []byte("ERROR: context deadline exceeded"),
			},
			wantType:      workerexecution.WorkFailureTypeTimeout,
			wantFamily:    workerexecution.WorkFailureFamilyRetryable,
			wantMessage:   codexTimeoutFailureMessage,
			wantRetryable: true,
		},
		{
			name: "ProcessTerminationCleanupError_IsTerminalWithoutThrottlePause",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("ERROR: The process with PID 1234 could not be terminated"),
			},
			wantType:          workerexecution.WorkFailureTypeUnknown,
			wantFamily:        workerexecution.WorkFailureFamilyTerminal,
			wantMessage:       codexUnknownFailureMessage,
			wantTerminal:      true,
			wantThrottlePause: false,
		},
		{
			name: "SpecificErrorLineWinsOverGenericOutput",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte(capacityLine),
				Stdout:   []byte("request failed with 400 bad request after full transcript"),
			},
			wantType:             workerexecution.WorkFailureTypeThrottled,
			wantFamily:           workerexecution.WorkFailureFamilyThrottle,
			wantMessage:          codexThrottleFailureMessage,
			wantRetryable:        true,
			wantThrottlePause:    true,
			rejectMessageContent: "bad request",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantType:          tc.wantType,
				wantFamily:        tc.wantFamily,
				wantMessage:       tc.wantMessage,
				rejectText:        tc.rejectMessageContent,
				wantRetryable:     tc.wantRetryable,
				wantTerminal:      tc.wantTerminal,
				wantThrottlePause: tc.wantThrottlePause,
			})
		})
	}
}

func TestScriptWrapProvider_Infer_April11RecordingFailureShapesNormalize(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	fixture := loadApril11FailureShapeFixture(t)

	for _, sample := range fixture.Samples {
		t.Run(sample.Name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{
				result: CommandResult{
					ExitCode: sample.ExitCode,
					Stdout:   []byte(sample.Stdout),
					Stderr:   []byte(sample.Stderr),
				},
			}
			provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "replay April 11 failure shape",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantType:          sample.WantType,
				wantMessage:       sample.WantMessage,
				wantRetryable:     sample.WantRetryable,
				wantTerminal:      sample.WantTerminal,
				wantThrottlePause: sample.WantThrottlePause,
				rejectTexts:       sample.RejectMessageContains,
			})
		})
	}
}

func TestScriptWrapProvider_Infer_CodexExitFailurePreservesSessionMetadata(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			ExitCode: 1,
			Stderr:   []byte(`{"event":"session.created","session_id":"sess_codex_error_123"}`),
		},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "fix it",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}

	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.ProviderSession == nil {
		t.Fatal("expected provider session metadata on failure")
	}
	if providerErr.ProviderSession.ID != "sess_codex_error_123" {
		t.Fatalf("provider session id = %q, want %q", providerErr.ProviderSession.ID, "sess_codex_error_123")
	}
}

func TestScriptWrapProvider_Infer_ClaudeExitFailuresNormalizeIntoSharedContract(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	testCases := []struct {
		name  string
		entry ProviderErrorCorpusEntry
	}{
		{
			name:  "Throttled_RateLimitError",
			entry: providerErrorCorpusEntryForTest(t, "claude_rate_limit_error"),
		},
		{
			name:  "Throttled_OverloadedError",
			entry: providerErrorCorpusEntryForTest(t, "claude_overloaded_error"),
		},
		{
			name:  "TransientServerError_ApiError500",
			entry: providerErrorCorpusEntryForTest(t, "claude_internal_server_api_error"),
		},
		{
			name:  "BadRequest_InvalidRequest",
			entry: providerErrorCorpusEntryForTest(t, "claude_invalid_request_error"),
		},
		{
			name:  "Timeout_MessageMatch",
			entry: providerErrorCorpusEntryForTest(t, "claude_timeout_waiting_for_provider"),
		},
		{
			name:  "AuthFailure_AuthenticationError",
			entry: providerErrorCorpusEntryForTest(t, "claude_authentication_error"),
		},
		{
			name:  "Unknown_Unclassified",
			entry: providerErrorCorpusEntryForTest(t, "claude_unknown_unclassified"),
		},
	}

	for _, tc := range testCases {
		entryLabel := providerErrorCorpusEntryLabel(tc.entry)
		t.Run(entryLabel, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.entry.CommandResult()}
			provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderClaude),
				Model:         "claude-sonnet-4-5-20250514",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("%s expected ProviderError, got %T", entryLabel, err)
			}
			if providerErr.Type != tc.entry.ExpectedType {
				t.Fatalf("%s Type = %q, want %q", entryLabel, providerErr.Type, tc.entry.ExpectedType)
			}
			if providerErr.Family != tc.entry.ExpectedFamily {
				t.Fatalf("%s Family = %q, want %q", entryLabel, providerErr.Family, tc.entry.ExpectedFamily)
			}
		})
	}
}

func TestParseClaudeProviderFailure_TranscriptSignalsCannotDrivePolicy(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	testCases := []string{
		"User: debug this configuration error",
		"Human: explain this api key failure",
		"Assistant: an invalid request would fail",
		"System: rate limit requests before dispatch",
		"Prompt: the provider is overloaded",
		"User: simulate an internal server error",
		"Assistant: describe what timed out means",
	}

	for _, transcript := range testCases {
		t.Run(strings.Fields(transcript)[0]+strings.Fields(transcript)[2], func(t *testing.T) {
			assertClaudeFailureAndPolicy(t, CommandResult{ExitCode: 7, Stderr: []byte(transcript)}, claudeFailureExpectation{
				reason:    workerexecution.WorkFailureTypeUnknown,
				family:    workerexecution.WorkFailureFamilyTerminal,
				message:   "claude exited with code 7",
				terminal:  true,
				retryable: false,
			})
		})
	}
}

func TestParseClaudeProviderFailure_UnsafeDiagnosticDetailsNeverPassThrough(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	testCases := []struct {
		name        string
		stderr      string
		wantReason  workerexecution.WorkFailureType
		wantMessage string
		rejectText  string
	}{
		{
			name:        "EmbeddedPromptMarkerRejectsTextRecord",
			stderr:      "Invalid request: User: private prompt content",
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: "claude exited with code 4",
		},
		{
			name:        "StructuredPromptMarkerUsesCategoryFallback",
			stderr:      `API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Invalid request: Prompt: private content"}}`,
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: claudeBadRequestFailureMessage,
		},
		{
			name:        "AuthorizationCredentialUsesCategoryFallback",
			stderr:      "Invalid request: replace Authorization: secret-value",
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: claudeBadRequestFailureMessage,
		},
		{
			name:        "BearerCredentialUsesCategoryFallback",
			stderr:      "Invalid request: replace Bearer secret-value",
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: claudeBadRequestFailureMessage,
		},
		{
			name:        "APIKeyCredentialUsesCategoryFallback",
			stderr:      "Invalid request: replace api_key=secret-value",
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: claudeBadRequestFailureMessage,
		},
		{
			name:        "AnthropicCredentialUsesCategoryFallback",
			stderr:      "Invalid request: replace sk-ant-secret-value",
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: claudeBadRequestFailureMessage,
		},
		{
			name:        "SensitiveEnvironmentAssignmentUsesCategoryFallback",
			stderr:      "Configuration error: ANTHROPIC_AUTH_TOKEN=customer-private-value",
			wantReason:  workerexecution.WorkFailureTypeMisconfigured,
			wantMessage: claudeConfigFailureMessage,
			rejectText:  "customer-private-value",
		},
		{
			name:        "StructuredSensitiveEnvironmentAssignmentUsesCategoryFallback",
			stderr:      `API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Set ANTHROPIC_AUTH_TOKEN=customer-private-value"}}`,
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: claudeBadRequestFailureMessage,
			rejectText:  "customer-private-value",
		},
		{
			name:        "SpacedSensitiveEnvironmentAssignmentUsesCategoryFallback",
			stderr:      "Configuration error: ANTHROPIC_AUTH_TOKEN = customer-private-value",
			wantReason:  workerexecution.WorkFailureTypeMisconfigured,
			wantMessage: claudeConfigFailureMessage,
			rejectText:  "customer-private-value",
		},
		{
			name:        "StructuredSpacedSensitiveHeaderUsesCategoryFallback",
			stderr:      `API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Replace X_API_KEY : customer-private-value"}}`,
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: claudeBadRequestFailureMessage,
			rejectText:  "customer-private-value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CommandResult{ExitCode: 4, Stderr: []byte(tc.stderr)}
			assertClaudeFailureAndPolicy(t, result, claudeFailureExpectation{
				reason:    tc.wantReason,
				family:    workerexecution.WorkFailureFamilyTerminal,
				message:   tc.wantMessage,
				terminal:  true,
				retryable: false,
			})
			if parsed := ParseClaudeProviderFailure(result); tc.rejectText != "" && strings.Contains(parsed.Message, tc.rejectText) {
				t.Fatalf("message %q must not contain %q", parsed.Message, tc.rejectText)
			}
		})
	}
}

func TestParseClaudeProviderFailure_CredentialProseNeverPassesThrough(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	testCases := []struct {
		name        string
		stderr      string
		wantReason  workerexecution.WorkFailureType
		wantMessage string
	}{
		{
			name:        "StructuredSensitiveEnvironmentProse",
			stderr:      `API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Set ANTHROPIC_AUTH_TOKEN to customer-private-value"}}`,
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: claudeBadRequestFailureMessage,
		},
		{
			name:        "CredentialProse",
			stderr:      "Authentication error: Credential customer-private-value is invalid",
			wantReason:  workerexecution.WorkFailureTypeAuthFailure,
			wantMessage: claudeAuthFailureMessage,
		},
		{
			name:        "TokenProse",
			stderr:      "Authentication error: Token customer-private-value is invalid",
			wantReason:  workerexecution.WorkFailureTypeAuthFailure,
			wantMessage: claudeAuthFailureMessage,
		},
		{
			name:        "SecretProse",
			stderr:      "Authentication error: Secret customer-private-value was rejected",
			wantReason:  workerexecution.WorkFailureTypeAuthFailure,
			wantMessage: claudeAuthFailureMessage,
		},
		{name: "APIKeyWhitespaceProse", stderr: "Invalid request: api key customer-private-value is invalid", wantReason: workerexecution.WorkFailureTypeAuthFailure, wantMessage: claudeAuthFailureMessage},
		{name: "StructuredHyphenatedAPIKeyWhitespaceProse", stderr: `API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Replace api-key customer-private-value"}}`, wantReason: workerexecution.WorkFailureTypePermanentBadRequest, wantMessage: claudeBadRequestFailureMessage},
		{name: "PrefixedAPIKeyWhitespaceProse", stderr: "Invalid request: x-api-key customer-private-value is invalid", wantReason: workerexecution.WorkFailureTypePermanentBadRequest, wantMessage: claudeBadRequestFailureMessage},
		{name: "StructuredPrefixedAPIKeyWhitespaceProse", stderr: `API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Replace x-api-key customer-private-value"}}`, wantReason: workerexecution.WorkFailureTypePermanentBadRequest, wantMessage: claudeBadRequestFailureMessage},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CommandResult{ExitCode: 4, Stderr: []byte(tc.stderr)}
			assertClaudeFailureAndPolicy(t, result, claudeFailureExpectation{
				reason:    tc.wantReason,
				family:    workerexecution.WorkFailureFamilyTerminal,
				message:   tc.wantMessage,
				terminal:  true,
				retryable: false,
			})
			if parsed := ParseClaudeProviderFailure(result); strings.Contains(parsed.Message, "customer-private-value") {
				t.Fatalf("message %q must not contain the credential value", parsed.Message)
			}
		})
	}
}

func TestParseClaudeProviderFailure_LongCleanupTailCannotEvictStructuredRecord(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	structured := `API Error: 429 {"type":"error","error":{"type":"rate_limit_error","message":"rate limit exceeded"}}`
	cleanup := strings.Repeat("cleanup completed successfully\n", claudeFailureScanBytes/16)
	result := CommandResult{ExitCode: 1, Stderr: []byte(structured + "\n" + cleanup)}

	assertClaudeFailureAndPolicy(t, result, claudeFailureExpectation{
		reason:        workerexecution.WorkFailureTypeThrottled,
		family:        workerexecution.WorkFailureFamilyThrottle,
		message:       claudeThrottleFailureMessage,
		retryable:     true,
		throttlePause: true,
	})
}

type claudeFailureExpectation struct {
	reason        workerexecution.WorkFailureType
	family        workerexecution.WorkFailureFamily
	message       string
	retryable     bool
	terminal      bool
	throttlePause bool
}

func assertClaudeFailureAndPolicy(t *testing.T, result CommandResult, want claudeFailureExpectation) {
	t.Helper()
	parsed := ParseClaudeProviderFailure(result)
	if parsed.Reason != want.reason || parsed.Message != want.message {
		t.Fatalf("parsed = %#v, want reason=%q message=%q", parsed, want.reason, want.message)
	}
	providerErr := NewProviderErrorFromResult(parsed, nil)
	decision := WorkFailureDecisionFromProviderError(providerErr)
	if providerErr.Family != want.family ||
		decision.Retryable != want.retryable ||
		decision.Terminal != want.terminal ||
		decision.TriggersThrottlePause != want.throttlePause {
		t.Fatalf("policy = family %q decision %#v, want family=%q retryable=%t terminal=%t throttle=%t", providerErr.Family, decision, want.family, want.retryable, want.terminal, want.throttlePause)
	}
}

func TestScriptWrapProvider_Infer_RunErrorsNormalizeTimeoutAndMisconfigured(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	capacityEntry := providerErrorCorpusEntryForTest(t, "codex_model_capacity_selected_model")
	capacityLine := providerErrorCorpusLastErrorLine(t, capacityEntry)
	testCases := []struct {
		name        string
		result      CommandResult
		runErr      error
		wantType    workerexecution.WorkFailureType
		wantFamily  workerexecution.WorkFailureFamily
		wantMessage string
		rejectText  string
	}{
		{
			name:        "DeadlineExceeded_IsTimeout",
			runErr:      context.DeadlineExceeded,
			wantType:    workerexecution.WorkFailureTypeTimeout,
			wantFamily:  workerexecution.WorkFailureFamilyRetryable,
			wantMessage: "execution timeout",
		},
		{
			name: "CanceledCommandWithTimeoutOutput_IsTimeout",
			result: CommandResult{
				Stderr: []byte("context canceled after command timed out"),
			},
			runErr:      context.Canceled,
			wantType:    workerexecution.WorkFailureTypeTimeout,
			wantFamily:  workerexecution.WorkFailureFamilyRetryable,
			wantMessage: "context canceled after command timed out",
		},
		{
			name: "DeadlineExceededWithCodexErrorLine_PreservesConciseError",
			result: CommandResult{
				Stdout: []byte(strings.Join([]string{
					strings.Repeat("raw inference transcript ", 4000),
					"agent looked successful",
					capacityLine,
					"cleanup finished after provider error",
				}, "\n")),
			},
			runErr:      context.DeadlineExceeded,
			wantType:    workerexecution.WorkFailureTypeTimeout,
			wantFamily:  workerexecution.WorkFailureFamilyRetryable,
			wantMessage: capacityLine,
			rejectText:  "raw inference transcript",
		},
		{
			name: "CanceledTimeoutOutputWithCodexErrorLine_PreservesConciseError",
			result: CommandResult{
				Stderr: []byte("context canceled after command timed out"),
				Stdout: []byte(strings.Join([]string{
					strings.Repeat("raw inference transcript ", 4000),
					"ERROR: context deadline exceeded while waiting for codex",
					"cleanup finished after provider error",
				}, "\n")),
			},
			runErr:      context.Canceled,
			wantType:    workerexecution.WorkFailureTypeTimeout,
			wantFamily:  workerexecution.WorkFailureFamilyRetryable,
			wantMessage: "ERROR: context deadline exceeded while waiting for codex",
			rejectText:  "raw inference transcript",
		},
		{
			name:       "ExecutableMissing_HasExplicitType",
			runErr:     exec.ErrNotFound,
			wantType:   workerexecution.WorkFailureTypeMissingExecutable,
			wantFamily: workerexecution.WorkFailureFamilyTerminal,
		},
		{
			name:       "UnknownRuntimeFailure_IsUnknown",
			runErr:     errors.New("pipe broke"),
			wantType:   workerexecution.WorkFailureTypeUnknown,
			wantFamily: workerexecution.WorkFailureFamilyTerminal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result, err: tc.runErr}
			provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantType:               tc.wantType,
				wantFamily:             tc.wantFamily,
				wantMessage:            tc.wantMessage,
				rejectText:             tc.rejectText,
				wantRetryable:          tc.wantType == workerexecution.WorkFailureTypeTimeout,
				requireTimeoutDecision: tc.wantType == workerexecution.WorkFailureTypeTimeout,
				requireTimeoutDiag:     tc.wantType == workerexecution.WorkFailureTypeTimeout,
			})
		})
	}
}

func TestScriptWrapProvider_Infer_ProviderTimeoutTextNormalizesToRetryableTimeout(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	testCases := []struct {
		name   string
		result CommandResult
	}{
		{
			name: "CodexTimeoutText",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   []byte("provider timeout while waiting for response"),
			},
		},
		{
			name: "CommandTimeoutExitCode",
			result: CommandResult{
				ExitCode: 124,
				Stderr:   []byte("command timed out"),
			},
		},
		{
			name: "ContextDeadlineText",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("context deadline exceeded"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantType:               workerexecution.WorkFailureTypeTimeout,
				wantRetryable:          true,
				requireTimeoutDecision: true,
			})
		})
	}
}

type normalizedProviderFailureExpectation struct {
	wantType               workerexecution.WorkFailureType
	wantFamily             workerexecution.WorkFailureFamily
	wantMessage            string
	rejectText             string
	rejectTexts            []string
	wantRetryable          bool
	wantTerminal           bool
	wantThrottlePause      bool
	requireTimeoutDecision bool
	requireTimeoutDiag     bool
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper intentionally validates the full normalized provider failure contract in one place.
func assertNormalizedProviderFailure(t *testing.T, err error, want normalizedProviderFailureExpectation) {
	t.Helper()

	if err == nil {
		t.Fatal("expected ProviderError, got nil")
	}

	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if want.wantType != "" && providerErr.Type != want.wantType {
		t.Fatalf("Type = %q, want %q", providerErr.Type, want.wantType)
	}
	if want.wantFamily != "" && providerErr.Family != want.wantFamily {
		t.Fatalf("Family = %q, want %q", providerErr.Family, want.wantFamily)
	}
	if want.wantMessage != "" && providerErr.Message != want.wantMessage {
		t.Fatalf("Message = %q, want %q", providerErr.Message, want.wantMessage)
	}
	if want.rejectText != "" && strings.Contains(providerErr.Message, want.rejectText) {
		t.Fatalf("Message = %q, should not contain %q", providerErr.Message, want.rejectText)
	}
	for _, rejectText := range want.rejectTexts {
		if strings.Contains(providerErr.Message, rejectText) {
			t.Fatalf("Message = %q, should not contain %q", providerErr.Message, rejectText)
		}
	}
	if want.wantRetryable || want.wantTerminal || want.wantThrottlePause || want.requireTimeoutDecision {
		decision := ClassifyProviderFailure(providerErr)
		if decision.Retryable != want.wantRetryable || decision.Terminal != want.wantTerminal || decision.TriggersThrottlePause != want.wantThrottlePause {
			t.Fatalf("ClassifyProviderFailure(%#v) = %#v, want retryable=%t terminal=%t throttlePause=%t", providerErr, decision, want.wantRetryable, want.wantTerminal, want.wantThrottlePause)
		}
	}
	if want.requireTimeoutDiag {
		if providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil || !providerErr.Diagnostics.Command.TimedOut {
			t.Fatalf("timeout diagnostics = %#v, want command timed_out", providerErr.Diagnostics)
		}
	}
}
