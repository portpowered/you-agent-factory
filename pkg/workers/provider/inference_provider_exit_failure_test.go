package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestScriptWrapProvider_Infer_GenericNonCodexExitFailuresPreserveMessageAndClassification(t *testing.T) {
	for _, tc := range genericNonCodexExitFailureTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertInferenceExitFailure(t, tc)
		})
	}
}

func TestScriptWrapProvider_Infer_CodexExitFailuresKeepCodexBehavior(t *testing.T) {
	for _, tc := range codexDerivedExitFailureTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertInferenceExitFailure(t, tc)
		})
	}
}

func TestScriptWrapProvider_Infer_CursorTerminalFailureUsesCanonicalResultAndDecision(t *testing.T) {
	stdout := []byte(strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"cursor-initial-session"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"private transcript"}]}}`,
		`{malformed}`,
		`{"type":"result","subtype":"rate_limit_error","is_error":true,"result":"Cursor model capacity is busy","session_id":"cursor-terminal-session"}`,
	}, "\n"))
	var published []InferenceProgressFragment
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(&recordingProviderExec{result: CommandResult{
			ExitCode: 1,
			Stdout:   stdout,
			Stderr:   []byte("unrelated invalid API key"),
		}}),
		WithInferenceProgressPublisher(func(fragment InferenceProgressFragment) {
			published = append(published, fragment)
		}),
	)

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-cursor-terminal-failure"},
		ModelProvider: string(interfaces.ModelProviderCursor),
		Model:         "gpt-5",
		UserMessage:   "private prompt",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	assertNormalizedProviderFailure(t, providerErr, normalizedProviderFailureExpectation{
		wantType:          interfaces.WorkFailureTypeThrottled,
		wantFamily:        interfaces.WorkFailureFamilyThrottle,
		wantMessage:       "Cursor model capacity is busy",
		wantRetryable:     true,
		wantThrottlePause: true,
		rejectTexts:       []string{"private prompt", "private transcript", "invalid API key"},
	})
	if providerErr.ProviderSession == nil || providerErr.ProviderSession.ID != "cursor-terminal-session" {
		t.Fatalf("provider session = %#v, want cursor-terminal-session", providerErr.ProviderSession)
	}
	if len(published) != 1 || published[0].Kind != FailedFragmentKind || published[0].Payload != providerErr.Message {
		t.Fatalf("published fragments = %#v, want canonical failure message", published)
	}
	if published[0].ProviderSessionRef == nil || published[0].ProviderSessionRef.ID != "cursor-terminal-session" {
		t.Fatalf("published provider session = %#v, want cursor-terminal-session", published[0].ProviderSessionRef)
	}
}

func TestScriptWrapProvider_Infer_CursorStderrFailureUsesCanonicalResultAndDecision(t *testing.T) {
	var published []InferenceProgressFragment
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(&recordingProviderExec{result: CommandResult{
			ExitCode: 1,
			Stdout:   []byte("unrelated partial output"),
			Stderr:   []byte("rate limit reached because Cursor capacity is busy"),
		}}),
		WithInferenceProgressPublisher(func(fragment InferenceProgressFragment) {
			published = append(published, fragment)
		}),
	)

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-cursor-stderr-failure"},
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "private prompt",
	})
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	assertNormalizedProviderFailure(t, providerErr, normalizedProviderFailureExpectation{
		wantType:          interfaces.WorkFailureTypeThrottled,
		wantFamily:        interfaces.WorkFailureFamilyThrottle,
		wantMessage:       "rate limit reached because Cursor capacity is busy",
		wantRetryable:     true,
		wantThrottlePause: true,
		rejectText:        "private prompt",
	})
	if len(published) != 1 || published[0].Payload != providerErr.Message {
		t.Fatalf("published fragments = %#v, want canonical stderr failure", published)
	}
}

func TestScriptWrapProvider_Infer_CursorExecutionFailureUsesParserAndPreservesCause(t *testing.T) {
	runErr := errors.New("cursor pipe broke")
	provider := NewScriptWrapProvider(WithProviderCommandRunner(&recordingProviderExec{
		result: CommandResult{ExitCode: 1, Stderr: []byte("Cursor authentication failed; sign in again")},
		err:    runErr,
	}))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "private prompt",
	})
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	if !errors.Is(providerErr.Cause, runErr) {
		t.Fatalf("cause = %v, want %v", providerErr.Cause, runErr)
	}
	assertNormalizedProviderFailure(t, providerErr, normalizedProviderFailureExpectation{
		wantType:    interfaces.WorkFailureTypeAuthFailure,
		wantFamily:  interfaces.WorkFailureFamilyTerminal,
		wantMessage: "Cursor authentication failed; sign in again",
		rejectText:  "private prompt",
	})
}

func TestScriptWrapProvider_Infer_CodexGPT56SolFailureUsesCanonicalResultAndDecision(t *testing.T) {
	entry := providerErrorCorpusEntryForTest(t, "codex_gpt_5_6_sol_requires_newer_cli")
	result := entry.CommandResult()
	result.Stderr = append([]byte("session_id: sess-codex-gpt-5-6-sol\n"), result.Stderr...)
	fakeExec := &recordingProviderExec{result: result}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCodex),
		Model:         "gpt-5.6-sol",
		UserMessage:   "private prompt that must not appear in the failure",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	assertGPT56SolCanonicalFailure(t, providerErr)
	assertGPT56SolFailureMetadata(t, providerErr, entry, result)
}

func TestScriptWrapProvider_Infer_LogsCorrelatedNormalizedCodexFailureAfterParsing(t *testing.T) {
	const prompt = "synthetic prompt must not appear"
	const credential = "credential-value-must-not-appear"
	sequence := []string{}
	logger := &preparedInvocationTestLogger{sequence: &sequence}
	runner := &preparedInvocationTestRunner{
		sequence: &sequence,
		result: CommandResult{
			ExitCode: 17,
			Stderr:   []byte(`ERROR: {"type":"invalid_request_error","status":400,"message":"The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."}` + "\n" + credential),
		},
	}
	provider := NewScriptWrapProvider(WithProviderLogger(logger), WithProviderCommandRunner(runner))
	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCodex),
		Model:         "gpt-5.6-sol",
		UserMessage:   prompt,
		EnvVars:       map[string]string{"API_TOKEN": credential},
		Dispatch: interfaces.WorkDispatch{
			DispatchID: "dispatch-failure-1",
			Execution: interfaces.ExecutionMetadata{
				RequestID: "request-failure-1", TraceID: "trace-failure-1", WorkIDs: []string{"work-failure-1", "work-failure-2"},
			},
		},
	})
	if err == nil {
		t.Fatal("Infer returned nil error")
	}
	if logger.failureCount != 1 {
		t.Fatalf("normalized failure records = %d, want 1", logger.failureCount)
	}
	if len(sequence) < 3 || sequence[0] != ProviderInvocationPrepared || sequence[1] != "runner" || sequence[2] != ProviderFailureNormalized {
		t.Fatalf("record sequence = %#v, want prepared, runner, normalized failure", sequence)
	}
	assertNormalizedFailureFields(t, logger.failureFields)
	if strings.Contains(logger.allValues, prompt) || strings.Contains(logger.allValues, credential) {
		t.Fatalf("provider logs contain prompt or credential: %s", logger.allValues)
	}
}

func assertNormalizedFailureFields(t *testing.T, fields map[string]any) {
	t.Helper()
	if fields["provider"] != "codex" || fields["model"] != "gpt-5.6-sol" {
		t.Fatalf("provider/model = %#v/%#v", fields["provider"], fields["model"])
	}
	if fields["failure_reason"] != interfaces.WorkFailureTypePermanentBadRequest || fields["failure_message"] != codexGPT56SolUpgradeMessage {
		t.Fatalf("canonical failure = %#v", fields)
	}
	if fields["retryable"] != false || fields["exit_code"] != 17 {
		t.Fatalf("retry/exit fields = %#v", fields)
	}
	if fields["request_id"] != "request-failure-1" || fields["trace_id"] != "trace-failure-1" || fields["work_id"] != "work-failure-1" || fields["dispatch_id"] != "dispatch-failure-1" {
		t.Fatalf("correlation fields = %#v", fields)
	}
	if _, ok := fields["duration_ms"]; !ok {
		t.Fatalf("duration_ms absent: %#v", fields)
	}
}

func TestScriptWrapProvider_Infer_LogsNormalizedFailuresWithoutSyntheticExitCodes(t *testing.T) {
	for _, tc := range []struct {
		name          string
		err           error
		wantReason    interfaces.WorkFailureType
		wantRetryable bool
	}{
		{name: "timeout", err: context.DeadlineExceeded, wantReason: interfaces.WorkFailureTypeTimeout, wantRetryable: true},
		{name: "command start", err: exec.ErrNotFound, wantReason: interfaces.WorkFailureTypeMisconfigured},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sequence := []string{}
			logger := &preparedInvocationTestLogger{sequence: &sequence}
			runner := &preparedInvocationTestRunner{sequence: &sequence, err: tc.err}
			provider := NewScriptWrapProvider(WithProviderLogger(logger), WithProviderCommandRunner(runner))
			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderClaude),
				UserMessage:   "private prompt",
				Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-no-exit"},
			})
			if err == nil {
				t.Fatal("Infer returned nil error")
			}
			if logger.failureCount != 1 || logger.failureFields["failure_reason"] != tc.wantReason || logger.failureFields["retryable"] != tc.wantRetryable {
				t.Fatalf("normalized failure fields = %#v", logger.failureFields)
			}
			if _, ok := logger.failureFields["exit_code"]; ok {
				t.Fatalf("failure record invented exit code: %#v", logger.failureFields)
			}
			if logger.failureFields["model"] != ProviderDefaultModel {
				t.Fatalf("model = %#v, want provider default", logger.failureFields["model"])
			}
		})
	}
}

func TestScriptWrapProvider_Infer_CodexExecutionFailureJSONLogsExcludeCommandOutput(t *testing.T) {
	const prompt = "codex execution prompt must not appear"
	const stdoutSecret = "stdout-secret-must-not-appear"
	const stderrSecret = "stderr-secret-must-not-appear"
	for _, tc := range []struct {
		name        string
		err         error
		wantReason  interfaces.WorkFailureType
		wantMessage string
	}{
		{name: "timeout", err: context.DeadlineExceeded, wantReason: interfaces.WorkFailureTypeTimeout, wantMessage: "Provider request timed out."},
		{name: "command start", err: exec.ErrNotFound, wantReason: interfaces.WorkFailureTypeMisconfigured, wantMessage: "Provider command could not be started."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			encoderConfig := zap.NewProductionEncoderConfig()
			core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(&output), zapcore.DebugLevel)
			provider := NewScriptWrapProvider(
				WithProviderLogger(logging.NewZapLogger(zap.New(core), false)),
				WithProviderCommandRunner(&recordingProviderExec{
					result: CommandResult{Stdout: []byte(stdoutSecret), Stderr: []byte(stderrSecret)},
					err:    tc.err,
				}),
			)
			_, _ = provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderCodex),
				UserMessage:   prompt,
			})

			record := normalizedFailureJSONRecord(t, output.String())
			if record["failure_reason"] != string(tc.wantReason) || record["failure_message"] != tc.wantMessage {
				t.Fatalf("normalized JSON failure = %#v", record)
			}
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatalf("encode normalized failure: %v", err)
			}
			for _, secret := range []string{prompt, stdoutSecret, stderrSecret} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("normalized JSON failure leaked %q: %s", secret, encoded)
				}
			}
		})
	}
}

func normalizedFailureJSONRecord(t *testing.T, logs string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode JSON log: %v", err)
		}
		if record["event_name"] == ProviderFailureNormalized {
			return record
		}
	}
	t.Fatalf("normalized failure absent from JSON logs: %s", logs)
	return nil
}

func TestScriptWrapProvider_Infer_CodexUpgradeFailureIsSearchableInJSONLogs(t *testing.T) {
	var output bytes.Buffer
	encoderConfig := zap.NewProductionEncoderConfig()
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(&output), zapcore.DebugLevel)
	provider := NewScriptWrapProvider(
		WithProviderLogger(logging.NewZapLogger(zap.New(core), false)),
		WithProviderCommandRunner(&recordingProviderExec{result: CommandResult{
			ExitCode: 2,
			Stderr:   []byte(`ERROR: {"type":"invalid_request_error","status":400,"message":"The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."}`),
		}}),
	)
	_, _ = provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCodex),
		UserMessage:   "json log prompt fixture",
	})

	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode JSON log: %v", err)
		}
		if record["event_name"] != ProviderFailureNormalized {
			continue
		}
		if record["failure_reason"] != string(interfaces.WorkFailureTypePermanentBadRequest) || record["failure_message"] != codexGPT56SolUpgradeMessage {
			t.Fatalf("normalized JSON failure = %#v", record)
		}
		if strings.Contains(line, "json log prompt fixture") || strings.Contains(line, `\"status\":400`) {
			t.Fatalf("normalized JSON failure leaked prompt or envelope: %s", line)
		}
		return
	}
	t.Fatalf("normalized failure absent from JSON logs: %s", output.String())
}

func assertGPT56SolCanonicalFailure(t *testing.T, providerErr *ProviderError) {
	t.Helper()

	if providerErr.Type != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, interfaces.WorkFailureTypePermanentBadRequest)
	}
	const wantMessage = "The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."
	if providerErr.Message != wantMessage {
		t.Fatalf("provider error message = %q, want %q", providerErr.Message, wantMessage)
	}
	if providerErr.Family != interfaces.WorkFailureFamilyTerminal {
		t.Fatalf("provider error family = %q, want %q", providerErr.Family, interfaces.WorkFailureFamilyTerminal)
	}
	decision := WorkFailureDecisionFromProviderError(providerErr)
	if decision.Retryable || !decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("provider failure decision = %#v, want terminal without retry or throttle pause", decision)
	}
}

func assertGPT56SolFailureMetadata(t *testing.T, providerErr *ProviderError, entry ProviderErrorCorpusEntry, result CommandResult) {
	t.Helper()

	if providerErr.ProviderSession == nil || providerErr.ProviderSession.ID != "sess-codex-gpt-5-6-sol" {
		t.Fatalf("provider session = %#v, want captured Codex session", providerErr.ProviderSession)
	}
	if providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil {
		t.Fatal("expected command diagnostics on provider error")
	}
	if providerErr.Diagnostics.Command.ExitCode != entry.ExitCode || providerErr.Diagnostics.Command.Stderr != string(result.Stderr) {
		t.Fatalf("command diagnostics = %#v, want captured exit code and stderr", providerErr.Diagnostics.Command)
	}
	if providerErr.Diagnostics.Command.TimedOut {
		t.Fatal("expected non-timeout Codex failure diagnostics")
	}
	if providerErr.Cause != nil {
		t.Fatalf("provider error cause = %v, want nil for non-zero exit", providerErr.Cause)
	}
}

type exitFailureInferenceTestCase struct {
	name        string
	provider    string
	result      CommandResult
	wantMessage string
	wantType    interfaces.WorkFailureType
}

func genericNonCodexExitFailureTestCases() []exitFailureInferenceTestCase {
	return []exitFailureInferenceTestCase{
		{
			name:        "GeminiPrefersProcessOutputForThrottle",
			provider:    string(interfaces.ModelProviderGemini),
			result:      CommandResult{ExitCode: 1, Stderr: []byte("resource exhausted by 429 quota")},
			wantMessage: "resource exhausted by 429 quota",
			wantType:    interfaces.WorkFailureTypeThrottled,
		},
		{
			name:        "KiroFallsBackToProviderExitCodeWhenOutputMissing",
			provider:    string(interfaces.ModelProviderKiro),
			result:      CommandResult{ExitCode: 9},
			wantMessage: "kiro-cli exited with code 9",
			wantType:    interfaces.WorkFailureTypeUnknown,
		},
		{
			name:     "OpenCodeUsesStructuredAuthenticationFailure",
			provider: string(interfaces.ModelProviderOpenCode),
			result: CommandResult{ExitCode: 1, Stdout: []byte(
				`{"type":"error","error":{"name":"ProviderAuthError","data":{"message":"Authentication required. Run opencode auth login."}}}`,
			)},
			wantMessage: "Authentication required. Run opencode auth login.",
			wantType:    interfaces.WorkFailureTypeAuthFailure,
		},
	}
}

func codexDerivedExitFailureTestCases() []exitFailureInferenceTestCase {
	return []exitFailureInferenceTestCase{
		{
			name:        "CodexUsesCodexErrorExtraction",
			provider:    string(interfaces.ModelProviderCodex),
			result:      CommandResult{ExitCode: 1, Stderr: []byte("noise before\nERROR: unexpected status 500 from codex upstream")},
			wantMessage: codexServerFailureMessage,
			wantType:    interfaces.WorkFailureTypeInternalServerError,
		},
	}
}

func assertInferenceExitFailure(t *testing.T, tc exitFailureInferenceTestCase) {
	t.Helper()

	fakeExec := &recordingProviderExec{result: tc.result}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: tc.provider,
		UserMessage:   "run the task",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Type != tc.wantType {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, tc.wantType)
	}
	if providerErr.Message != tc.wantMessage {
		t.Fatalf("provider error message = %q, want %q", providerErr.Message, tc.wantMessage)
	}
}

func TestInferenceProgressPublishingCommandRunner_NormalizesCodexStructuredEvents(t *testing.T) {
	scriptPath := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' '{\"event\":\"session.created\",\"session_id\":\"sess-codex-1\"}'\n" +
		"printf '%s\\n' '{\"type\":\"response.output_text.delta\",\"delta\":\"hello from delta\"}'\n" +
		"printf '%s\\n' '{\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello final\"}]}]}}'\n" +
		"printf '%s\\n' 'planning update' 1>&2\n"
	writeExecutableTestScript(t, scriptPath, script)

	var published []InferenceProgressFragment
	var publishedMu sync.Mutex
	runner := NewInferenceProgressPublishingCommandRunner(func(fragment InferenceProgressFragment) {
		publishedMu.Lock()
		published = append(published, fragment)
		publishedMu.Unlock()
	}, nil)

	_, err := runner.Run(context.Background(), CommandRequest{
		Command:         scriptPath,
		DispatchID:      "dispatch-codex-json-1",
		WorkstationName: "review",
		Execution: interfaces.ExecutionMetadata{
			WorkIDs: []string{"work-codex-json-1"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	publishedMu.Lock()
	defer publishedMu.Unlock()
	if len(published) != 4 {
		t.Fatalf("published fragments = %#v, want 4 normalized fragments", published)
	}

	startedFragment := fragmentByType(published, NormalizedEventTypeStarted)
	deltaFragment := fragmentByType(published, NormalizedEventTypeTextDelta)
	finalFragment := fragmentByType(published, NormalizedEventTypeFinalText)
	progressFragment := fragmentByType(published, NormalizedEventTypeProgress)

	assertCodexStartedFragment(t, startedFragment, "sess-codex-1", "work-codex-json-1")
	assertCodexResponseFragment(t, deltaFragment, NormalizedEventTypeTextDelta, "hello from delta")
	assertCodexResponseFragment(t, finalFragment, NormalizedEventTypeFinalText, "hello final")
	assertCodexProgressFragment(t, progressFragment, "planning update")
	if finalFragment.ProviderSessionRef == nil || finalFragment.ProviderSessionRef.ID != "sess-codex-1" {
		t.Fatalf("final provider session = %#v, want session propagated", finalFragment.ProviderSessionRef)
	}
}

func TestInferenceProgressPublishingCommandRunner_MapsUnknownAndMalformedCodexEventsToBoundedDiagnostics(t *testing.T) {
	scriptPath := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' '{\"event\":\"session.created\",\"session_id\":\"sess-codex-2\"}'\n" +
		"printf '%s\\n' '{\"type\":\"response.mystery\",\"message\":\"secret-token-123 should never be retained\"}'\n" +
		"printf '%s\\n' '{\"type\":\"response.progress\"' \n" +
		"printf 'event: response.output_text.delta\\n'\n" +
		"printf '\\n'\n" +
		"printf 'event: response.output_text.delta\\n'\n" +
		"printf 'data: {\"delta\":\"hello after malformed frames\"}\\n'\n" +
		"printf '\\n'\n"
	writeExecutableTestScript(t, scriptPath, script)

	var published []InferenceProgressFragment
	var publishedMu sync.Mutex
	runner := NewInferenceProgressPublishingCommandRunner(func(fragment InferenceProgressFragment) {
		publishedMu.Lock()
		published = append(published, fragment)
		publishedMu.Unlock()
	}, nil)

	_, err := runner.Run(context.Background(), CommandRequest{
		Command:         scriptPath,
		DispatchID:      "dispatch-codex-json-2",
		WorkstationName: "review",
		Execution: interfaces.ExecutionMetadata{
			WorkIDs: []string{"work-codex-json-2"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	publishedMu.Lock()
	defer publishedMu.Unlock()
	if len(published) != 5 {
		t.Fatalf("published fragments = %#v, want 5 fragments with bounded unknown diagnostics", published)
	}

	assertCodexStartedFragment(t, &published[0], "sess-codex-2", "work-codex-json-2")
	assertUnknownCodexDiagnostic(t, &published[1], "response.mystery", codexDiagnosticUnknownEvent)
	assertUnknownCodexDiagnostic(t, &published[2], "", codexDiagnosticMalformedJSON)
	assertUnknownCodexDiagnostic(t, &published[3], "response.output_text.delta", codexDiagnosticIncompleteSSE)
	assertCodexResponseFragment(t, &published[4], NormalizedEventTypeTextDelta, "hello after malformed frames")
	if published[4].ProviderSessionRef == nil || published[4].ProviderSessionRef.ID != "sess-codex-2" {
		t.Fatalf("final provider session = %#v, want session carried across malformed frames", published[4].ProviderSessionRef)
	}
}

func TestInferenceProgressPublishingCommandRunner_MapsFailureCancelAndTruncation(t *testing.T) {
	// Do not run in parallel: Linux CI can return "text file busy" when executing
	// the freshly written shell script under heavy parallel package load.

	progressPayload := strings.Repeat("p", codexRetainedProgressBytes+73)
	deltaPayload := strings.Repeat("d", codexRetainedTextBytes+29)
	finalPayload := strings.Repeat("f", codexRetainedTextBytes+41)
	failurePayload := strings.Repeat("e", codexRetainedProgressBytes+17)
	cancelPayload := strings.Repeat("c", codexRetainedProgressBytes+9)

	scriptPath := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' '{\"event\":\"session.created\",\"session_id\":\"sess-codex-3\"}'\n" +
		"printf '%s\\n' '{\"type\":\"response.progress\",\"message\":\"" + progressPayload + "\"}'\n" +
		"printf '%s\\n' '{\"type\":\"response.output_text.delta\",\"delta\":\"" + deltaPayload + "\"}'\n" +
		"printf '%s\\n' '{\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"" + finalPayload + "\"}]}]}}'\n" +
		"printf '%s\\n' '{\"type\":\"response.failed\",\"error\":\"" + failurePayload + "\"}'\n" +
		"printf '%s\\n' '{\"type\":\"response.canceled\",\"status\":\"" + cancelPayload + "\"}'\n"
	writeExecutableTestScript(t, scriptPath, script)

	var published []InferenceProgressFragment
	var publishedMu sync.Mutex
	runner := NewInferenceProgressPublishingCommandRunner(func(fragment InferenceProgressFragment) {
		publishedMu.Lock()
		published = append(published, fragment)
		publishedMu.Unlock()
	}, nil)

	_, err := runner.Run(context.Background(), CommandRequest{
		Command:         scriptPath,
		DispatchID:      "dispatch-codex-json-3",
		WorkstationName: "review",
		Execution: interfaces.ExecutionMetadata{
			WorkIDs: []string{"work-codex-json-3"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	publishedMu.Lock()
	defer publishedMu.Unlock()
	if len(published) != 6 {
		t.Fatalf("published fragments = %#v, want 6 normalized fragments", published)
	}

	assertCodexStartedFragment(t, &published[0], "sess-codex-3", "work-codex-json-3")
	assertCodexBoundedFragment(t, &published[1], ProgressFragmentKind, NormalizedEventTypeProgress, "response.progress", progressPayload, codexRetainedProgressBytes)
	assertCodexBoundedFragment(t, &published[2], ResponseFragmentKind, NormalizedEventTypeTextDelta, "response.output_text.delta", deltaPayload, codexRetainedTextBytes)
	assertCodexBoundedFragment(t, &published[3], ResponseFragmentKind, NormalizedEventTypeFinalText, "response.completed", finalPayload, codexRetainedTextBytes)
	assertCodexBoundedFragment(t, &published[4], ProgressFragmentKind, NormalizedEventTypeFailed, "response.failed", failurePayload, codexRetainedProgressBytes)
	assertCodexBoundedFragment(t, &published[5], ProgressFragmentKind, NormalizedEventTypeCanceled, "response.canceled", cancelPayload, codexRetainedProgressBytes)

	if published[5].ProviderSessionRef == nil || published[5].ProviderSessionRef.ID != "sess-codex-3" {
		t.Fatalf("cancel provider session = %#v, want session propagated", published[5].ProviderSessionRef)
	}
}

func fragmentByType(published []InferenceProgressFragment, fragmentType string) *InferenceProgressFragment {
	for i := range published {
		if published[i].Type == fragmentType {
			return &published[i]
		}
	}
	return nil
}

func assertCodexStartedFragment(t *testing.T, fragment *InferenceProgressFragment, sessionID string, workID string) {
	t.Helper()
	if fragment == nil || fragment.ExternalEventType != "session.created" {
		t.Fatalf("started fragment = %#v, want session.created", fragment)
	}
	if fragment.ProviderSessionRef == nil || fragment.ProviderSessionRef.ID != sessionID {
		t.Fatalf("start provider session = %#v, want %s", fragment.ProviderSessionRef, sessionID)
	}
	if got := fragment.Metadata[codexMetadataRunnerIDKey]; got != "codex" {
		t.Fatalf("start metadata runner_id = %q, want codex", got)
	}
	if got := fragment.Metadata[codexMetadataWorkstationKey]; got != "review" {
		t.Fatalf("start metadata workstation_name = %q, want review", got)
	}
	if got := fragment.Metadata[codexMetadataWorkIDKey]; got != workID {
		t.Fatalf("start metadata work_id = %q, want %q", got, workID)
	}
}

func assertCodexResponseFragment(t *testing.T, fragment *InferenceProgressFragment, fragmentType string, payload string) {
	t.Helper()
	if fragment == nil || fragment.Kind != ResponseFragmentKind || fragment.Type != fragmentType || fragment.Payload != payload {
		t.Fatalf("response fragment = %#v, want %s payload %q", fragment, fragmentType, payload)
	}
}

func assertCodexProgressFragment(t *testing.T, fragment *InferenceProgressFragment, payload string) {
	t.Helper()
	if fragment == nil || fragment.Kind != ProgressFragmentKind || fragment.Payload != payload {
		t.Fatalf("progress fragment = %#v, want progress payload %q", fragment, payload)
	}
}

func assertUnknownCodexDiagnostic(t *testing.T, fragment *InferenceProgressFragment, externalEventType string, diagnosticClass string) {
	t.Helper()
	if fragment == nil || fragment.Type != NormalizedEventTypeUnknown || fragment.Kind != ProgressFragmentKind {
		t.Fatalf("unknown fragment = %#v, want UNKNOWN progress fragment", fragment)
	}
	if fragment.ExternalEventType != externalEventType {
		t.Fatalf("unknown external event = %q, want %q", fragment.ExternalEventType, externalEventType)
	}
	if fragment.Payload != "codex event omitted" || strings.Contains(fragment.Payload, "secret-token-123") {
		t.Fatalf("unknown payload = %q, want bounded omitted diagnostic", fragment.Payload)
	}
	if got := fragment.Metadata[codexMetadataDiagnosticKey]; got != diagnosticClass {
		t.Fatalf("diagnostic_class = %q, want %q", got, diagnosticClass)
	}
	if diagnosticClass == codexDiagnosticUnknownEvent && (fragment.Metadata[codexMetadataRawSHA256Key] == "" || fragment.Metadata[codexMetadataRawBytesKey] == "") {
		t.Fatalf("unknown metadata = %#v, want raw digest metadata", fragment.Metadata)
	}
}

func assertCodexBoundedFragment(
	t *testing.T,
	fragment *InferenceProgressFragment,
	kind string,
	fragmentType string,
	externalEventType string,
	originalPayload string,
	retainedBytes int,
) {
	t.Helper()
	if fragment == nil {
		t.Fatalf("fragment = nil, want %s %s", kind, fragmentType)
	}
	if fragment.Kind != kind || fragment.Type != fragmentType {
		t.Fatalf("fragment kind/type = (%q, %q), want (%q, %q)", fragment.Kind, fragment.Type, kind, fragmentType)
	}
	if fragment.ExternalEventType != externalEventType {
		t.Fatalf("external event type = %q, want %q", fragment.ExternalEventType, externalEventType)
	}
	if len([]byte(fragment.Payload)) != retainedBytes {
		t.Fatalf("payload bytes = %d, want %d", len([]byte(fragment.Payload)), retainedBytes)
	}
	if fragment.Payload != originalPayload[:retainedBytes] {
		t.Fatalf("payload retained wrong prefix length: got %d bytes", len([]byte(fragment.Payload)))
	}
	if got := fragment.Metadata[codexMetadataTextBytesKey]; got != strconv.Itoa(len([]byte(originalPayload))) {
		t.Fatalf("text_bytes = %q, want %d", got, len([]byte(originalPayload)))
	}
	if got := fragment.Metadata[codexMetadataTruncatedKey]; got != "true" {
		t.Fatalf("payload_truncated = %q, want true", got)
	}
}

func TestParseOpenCodeProviderFailure_KnownCorpusShapesUseCanonicalContract(t *testing.T) {
	testCases := []struct {
		name        string
		wantMessage string
	}{
		{name: "opencode_provider_auth_error", wantMessage: "Authentication required for openai. Run opencode auth login."},
		{name: "opencode_invalid_request_api_error", wantMessage: "The selected model does not support this request."},
		{name: "opencode_rate_limit_text", wantMessage: opencodeThrottleFailureMessage},
		{name: "opencode_timeout_error", wantMessage: opencodeTimeoutFailureMessage},
		{name: "opencode_server_api_error", wantMessage: opencodeServerFailureMessage},
	}

	for _, tc := range testCases {
		entry := providerErrorCorpusEntryForTest(t, tc.name)
		t.Run(tc.name, func(t *testing.T) {
			got := ParseOpenCodeProviderFailure(entry.CommandResult())
			if got.Reason != entry.ExpectedType || got.Message != tc.wantMessage {
				t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want reason=%q message=%q", got, entry.ExpectedType, tc.wantMessage)
			}
			if len(got.Message) > opencodeFailureMessageBytes {
				t.Fatalf("message length = %d, want at most %d", len(got.Message), opencodeFailureMessageBytes)
			}
		})
	}
}

func TestParseOpenCodeProviderFailure_StructuredFailurePrecedesText(t *testing.T) {
	result := CommandResult{
		ExitCode: 1,
		Stderr:   []byte("Error: rate limit exceeded"),
		Stdout:   []byte(`{"type":"error","error":{"name":"APIError","data":{"statusCode":400,"message":"Choose a supported model."}}}`),
	}

	got := ParseOpenCodeProviderFailure(result)
	if got.Reason != interfaces.WorkFailureTypePermanentBadRequest || got.Message != "Choose a supported model." {
		t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want structured bad request", got)
	}
}

func TestParseOpenCodeProviderFailure_SanitizesKnownActionableDetails(t *testing.T) {
	result := CommandResult{
		ExitCode: 1,
		Stdout: []byte(`{"type":"error","error":{"name":"APIError","data":{"statusCode":400,"message":"prompt: ` +
			strings.Repeat("private ", 100) + ` Authorization: Bearer secret-token"}}}`),
	}

	got := ParseOpenCodeProviderFailure(result)
	if got.Reason != interfaces.WorkFailureTypePermanentBadRequest || got.Message != opencodeBadRequestFailureMessage {
		t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want sanitized fixed bad-request message", got)
	}
	if len(got.Message) > opencodeFailureMessageBytes || strings.Contains(got.Message, "secret-token") || strings.Contains(got.Message, "private") {
		t.Fatalf("message = %q, want bounded message without sensitive detail", got.Message)
	}
}

func TestParseOpenCodeProviderFailure_UnknownFailuresUseSafeBoundedExcerptOrExitCode(t *testing.T) {
	testCases := []struct {
		name        string
		result      CommandResult
		wantMessage string
	}{
		{
			name:        "safe error line",
			result:      CommandResult{ExitCode: 17, Stderr: []byte("loading project\nError: plugin initialization failed\nrendering prompt")},
			wantMessage: "Error: plugin initialization failed",
		},
		{
			name:        "unrecognized structured error",
			result:      CommandResult{ExitCode: 18, Stdout: []byte(`{"type":"error","error":{"name":"PluginError","data":{"message":"Plugin initialization failed."}}}`)},
			wantMessage: "Plugin initialization failed.",
		},
		{
			name:        "oversized safe error",
			result:      CommandResult{ExitCode: 19, Stderr: []byte("Error: " + strings.Repeat("x", opencodeFailureMessageBytes))},
			wantMessage: ("Error: " + strings.Repeat("x", opencodeFailureMessageBytes))[:opencodeFailureMessageBytes],
		},
		{
			name:        "empty output",
			result:      CommandResult{ExitCode: 20},
			wantMessage: "opencode exited with code 20",
		},
		{
			name:        "malformed structured output",
			result:      CommandResult{ExitCode: 21, Stdout: []byte(`{"type":"error","message":"unfinished"`)},
			wantMessage: "opencode exited with code 21",
		},
		{
			name:        "transcript noise",
			result:      CommandResult{ExitCode: 22, Stderr: []byte("user: show credentials\nassistant: working\nprompt: private request")},
			wantMessage: "opencode exited with code 22",
		},
		{
			name:        "secret bearing error",
			result:      CommandResult{ExitCode: 23, Stderr: []byte("Error: Authorization: Bearer secret-token")},
			wantMessage: "opencode exited with code 23",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseOpenCodeProviderFailure(tc.result)
			if got.Reason != interfaces.WorkFailureTypeUnknown || got.Message != tc.wantMessage {
				t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want unknown message %q", got, tc.wantMessage)
			}
			if len(got.Message) > opencodeFailureMessageBytes {
				t.Fatalf("message length = %d, want at most %d", len(got.Message), opencodeFailureMessageBytes)
			}
		})
	}
}

func TestNormalizeProviderExitFailure_SelectsOpenCodeParserFromNormalizedIdentity(t *testing.T) {
	entry := providerErrorCorpusEntryForTest(t, "opencode_rate_limit_text")
	providerErr := normalizeProviderExitFailure("  OPENCODE  ", entry.CommandResult(), nil, nil)

	if providerErr.Type != interfaces.WorkFailureTypeThrottled || providerErr.Message != opencodeThrottleFailureMessage {
		t.Fatalf("normalizeProviderExitFailure() = %#v, want OpenCode throttle failure", providerErr)
	}
	decision := WorkFailureDecisionFromProviderError(providerErr)
	if !decision.Retryable || decision.Terminal || !decision.TriggersThrottlePause {
		t.Fatalf("decision = %#v, want central throttle policy", decision)
	}
}

func TestNormalizeProviderExitFailure_OpenCodeCorpusUsesCentralPolicy(t *testing.T) {
	for _, name := range []string{
		"opencode_provider_auth_error",
		"opencode_invalid_request_api_error",
		"opencode_rate_limit_text",
		"opencode_timeout_error",
		"opencode_server_api_error",
	} {
		entry := providerErrorCorpusEntryForTest(t, name)
		t.Run(name, func(t *testing.T) {
			providerErr := normalizeProviderExitFailure(string(entry.Provider), entry.CommandResult(), nil, nil)
			if providerErr.Type != entry.ExpectedType || providerErr.Family != entry.ExpectedFamily {
				t.Fatalf("normalized failure = %#v, want type=%q family=%q", providerErr, entry.ExpectedType, entry.ExpectedFamily)
			}
			decision := WorkFailureDecisionFromProviderError(providerErr)
			if decision.Retryable != entry.Retryable || decision.Terminal == entry.Retryable || decision.TriggersThrottlePause != entry.TriggersThrottlePause {
				t.Fatalf("decision = %#v, want retryable=%t terminal=%t throttlePause=%t", decision, entry.Retryable, !entry.Retryable, entry.TriggersThrottlePause)
			}
		})
	}
}
