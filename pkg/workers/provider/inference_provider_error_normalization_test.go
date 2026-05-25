package provider

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestScriptWrapProvider_Infer_CodexExitFailuresNormalizeIntoSharedContract(t *testing.T) {
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
				ExpectedType:   interfaces.ProviderErrorTypeUnknown,
				ExpectedFamily: interfaces.ProviderErrorFamilyTerminal,
			},
		},
	}

	for _, tc := range testCases {
		entryLabel := providerErrorCorpusEntryLabel(tc.entry)
		t.Run(entryLabel, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.entry.CommandResult()}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
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
			wantTerminal := tc.entry.ExpectedFamily == interfaces.ProviderErrorFamilyTerminal
			if decision.Terminal != wantTerminal {
				t.Fatalf("%s Terminal = %t, want %t", entryLabel, decision.Terminal, wantTerminal)
			}
		})
	}
}

func TestScriptWrapProvider_Infer_CodexNormalizedRetryDecisionRegressions(t *testing.T) {
	testCases := []struct {
		name              string
		stderr            string
		wantType          interfaces.ProviderErrorType
		wantFamily        interfaces.ProviderErrorFamily
		wantRetryable     bool
		wantTerminal      bool
		wantThrottlePause bool
	}{
		{
			name:              "InternalServerError_HighDemandTemporaryErrors_RetriesWithoutThrottlePause",
			stderr:            `ERROR: We're currently experiencing high demand, which may cause temporary errors.`,
			wantType:          interfaces.ProviderErrorTypeInternalServerError,
			wantFamily:        interfaces.ProviderErrorFamilyRetryable,
			wantRetryable:     true,
			wantTerminal:      false,
			wantThrottlePause: false,
		},
		{
			name:              "InternalServerError_UnexpectedStatus500_RetriesWithoutThrottlePause",
			stderr:            `ERROR: unexpected status 500 Internal Server Error`,
			wantType:          interfaces.ProviderErrorTypeInternalServerError,
			wantFamily:        interfaces.ProviderErrorFamilyRetryable,
			wantRetryable:     true,
			wantTerminal:      false,
			wantThrottlePause: false,
		},
		{
			name:              "AuthFailure_UnexpectedStatus401_IsTerminal",
			stderr:            `ERROR: unexpected status 401 Unauthorized {"type":"authentication_error","message":"invalid api key"}`,
			wantType:          interfaces.ProviderErrorTypeAuthFailure,
			wantFamily:        interfaces.ProviderErrorFamilyTerminal,
			wantRetryable:     false,
			wantTerminal:      true,
			wantThrottlePause: false,
		},
		{
			name:              "PermanentBadRequest_UnexpectedStatus400_IsTerminal",
			stderr:            `ERROR: unexpected status 400 Bad Request {"type":"invalid_request_error","message":"bad request"}`,
			wantType:          interfaces.ProviderErrorTypePermanentBadRequest,
			wantFamily:        interfaces.ProviderErrorFamilyTerminal,
			wantRetryable:     false,
			wantTerminal:      true,
			wantThrottlePause: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewScriptWrapProvider(WithProviderCommandRunner(&recordingProviderExec{
				result: CommandResult{ExitCode: 1, Stderr: []byte(tc.stderr)},
			}))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
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
	testCases := []struct {
		entryName          string
		wantType           interfaces.ProviderErrorType
		wantRetryable      bool
		wantThrottlePause  bool
		wantRejectAuthType bool
	}{
		{
			entryName:          "codex_windows_exit_code_4294967295",
			wantType:           interfaces.ProviderErrorTypeInternalServerError,
			wantRetryable:      true,
			wantThrottlePause:  false,
			wantRejectAuthType: true,
		},
		{
			entryName:         "codex_authentication_unauthorized",
			wantType:          interfaces.ProviderErrorTypeAuthFailure,
			wantRetryable:     false,
			wantThrottlePause: false,
		},
	}

	for _, tc := range testCases {
		entry := providerErrorCorpusEntryForTest(t, tc.entryName)
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: entry.CommandResult()}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
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
			if tc.wantRejectAuthType && providerErr.Type == interfaces.ProviderErrorTypeAuthFailure {
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
	testCases := []struct {
		name              string
		result            CommandResult
		wantType          interfaces.ProviderErrorType
		wantFamily        interfaces.ProviderErrorFamily
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
			wantType:          interfaces.ProviderErrorTypeInternalServerError,
			wantFamily:        interfaces.ProviderErrorFamilyRetryable,
			wantMessage:       "codex exited with code 4294967295",
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
			wantType:          interfaces.ProviderErrorTypeAuthFailure,
			wantFamily:        interfaces.ProviderErrorFamilyTerminal,
			wantMessage:       `ERROR: unexpected status 401 Unauthorized {"type":"authentication_error","message":"invalid api key"}`,
			wantRetryable:     false,
			wantTerminal:      true,
			wantThrottlePause: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
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

func TestScriptWrapProvider_Infer_CodexExitFailureExtractsBoundedErrorLine(t *testing.T) {
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
			wantLine:   capacityLine,
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
			wantLine:   "ERROR: The process with PID 1234 could not be terminated",
			rejectText: "trailing cleanup note",
		},
		{
			name: "FinalMatchingErrorWinsAcrossStreams",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("ERROR: First provider failure"),
				Stdout:   []byte("  ERROR: Final provider failure  \nnot final"),
			},
			wantLine:   "ERROR: Final provider failure",
			rejectText: "ERROR: First provider failure",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
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

func TestScriptWrapProvider_Infer_KnownCodexErrorLinesMapToProviderFailureCategories(t *testing.T) {
	capacityEntry := providerErrorCorpusEntryForTest(t, "codex_model_capacity_selected_model")
	capacityLine := providerErrorCorpusLastErrorLine(t, capacityEntry)

	testCases := []struct {
		name                 string
		result               CommandResult
		wantType             interfaces.ProviderErrorType
		wantFamily           interfaces.ProviderErrorFamily
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
			wantType:             interfaces.ProviderErrorTypeThrottled,
			wantFamily:           interfaces.ProviderErrorFamilyThrottle,
			wantMessage:          capacityLine,
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
			wantType:      interfaces.ProviderErrorTypeTimeout,
			wantFamily:    interfaces.ProviderErrorFamilyRetryable,
			wantMessage:   "ERROR: command timed out while waiting for codex",
			wantRetryable: true,
		},
		{
			name: "CodexContextDeadline_IsRetryableTimeout",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   []byte("ERROR: context deadline exceeded"),
			},
			wantType:      interfaces.ProviderErrorTypeTimeout,
			wantFamily:    interfaces.ProviderErrorFamilyRetryable,
			wantMessage:   "ERROR: context deadline exceeded",
			wantRetryable: true,
		},
		{
			name: "ProcessTerminationCleanupError_IsTerminalWithoutThrottlePause",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("ERROR: The process with PID 1234 could not be terminated"),
			},
			wantType:          interfaces.ProviderErrorTypeUnknown,
			wantFamily:        interfaces.ProviderErrorFamilyTerminal,
			wantMessage:       "ERROR: The process with PID 1234 could not be terminated",
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
			wantType:             interfaces.ProviderErrorTypeThrottled,
			wantFamily:           interfaces.ProviderErrorFamilyThrottle,
			wantMessage:          capacityLine,
			wantRetryable:        true,
			wantThrottlePause:    true,
			rejectMessageContent: "bad request",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
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
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
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
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			ExitCode: 1,
			Stderr:   []byte(`{"event":"session.created","session_id":"sess_codex_error_123"}`),
		},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(ModelProviderCodex),
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
			name: "Unknown_Unclassified",
			entry: ProviderErrorCorpusEntry{
				Name:           "claude_unknown_unclassified",
				ExitCode:       1,
				Stderr:         `some brand new claude failure`,
				ExpectedType:   interfaces.ProviderErrorTypeUnknown,
				ExpectedFamily: interfaces.ProviderErrorFamilyTerminal,
			},
		},
	}

	for _, tc := range testCases {
		entryLabel := providerErrorCorpusEntryLabel(tc.entry)
		t.Run(entryLabel, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.entry.CommandResult()}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderClaude),
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

func TestScriptWrapProvider_Infer_RunErrorsNormalizeTimeoutAndMisconfigured(t *testing.T) {
	capacityEntry := providerErrorCorpusEntryForTest(t, "codex_model_capacity_selected_model")
	capacityLine := providerErrorCorpusLastErrorLine(t, capacityEntry)

	testCases := []struct {
		name        string
		result      CommandResult
		runErr      error
		wantType    interfaces.ProviderErrorType
		wantFamily  interfaces.ProviderErrorFamily
		wantMessage string
		rejectText  string
	}{
		{
			name:        "DeadlineExceeded_IsTimeout",
			runErr:      context.DeadlineExceeded,
			wantType:    interfaces.ProviderErrorTypeTimeout,
			wantFamily:  interfaces.ProviderErrorFamilyRetryable,
			wantMessage: "execution timeout",
		},
		{
			name: "CanceledCommandWithTimeoutOutput_IsTimeout",
			result: CommandResult{
				Stderr: []byte("context canceled after command timed out"),
			},
			runErr:      context.Canceled,
			wantType:    interfaces.ProviderErrorTypeTimeout,
			wantFamily:  interfaces.ProviderErrorFamilyRetryable,
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
			wantType:    interfaces.ProviderErrorTypeTimeout,
			wantFamily:  interfaces.ProviderErrorFamilyRetryable,
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
			wantType:    interfaces.ProviderErrorTypeTimeout,
			wantFamily:  interfaces.ProviderErrorFamilyRetryable,
			wantMessage: "ERROR: context deadline exceeded while waiting for codex",
			rejectText:  "raw inference transcript",
		},
		{
			name:       "ExecutableMissing_IsMisconfigured",
			runErr:     exec.ErrNotFound,
			wantType:   interfaces.ProviderErrorTypeMisconfigured,
			wantFamily: interfaces.ProviderErrorFamilyTerminal,
		},
		{
			name:       "UnknownRuntimeFailure_IsUnknown",
			runErr:     errors.New("pipe broke"),
			wantType:   interfaces.ProviderErrorTypeUnknown,
			wantFamily: interfaces.ProviderErrorFamilyTerminal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result, err: tc.runErr}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantType:               tc.wantType,
				wantFamily:             tc.wantFamily,
				wantMessage:            tc.wantMessage,
				rejectText:             tc.rejectText,
				wantRetryable:          tc.wantType == interfaces.ProviderErrorTypeTimeout,
				requireTimeoutDecision: tc.wantType == interfaces.ProviderErrorTypeTimeout,
				requireTimeoutDiag:     tc.wantType == interfaces.ProviderErrorTypeTimeout,
			})
		})
	}
}

func TestScriptWrapProvider_Infer_ProviderTimeoutTextNormalizesToRetryableTimeout(t *testing.T) {
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
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantType:               interfaces.ProviderErrorTypeTimeout,
				wantRetryable:          true,
				requireTimeoutDecision: true,
			})
		})
	}
}

type normalizedProviderFailureExpectation struct {
	wantType               interfaces.ProviderErrorType
	wantFamily             interfaces.ProviderErrorFamily
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
