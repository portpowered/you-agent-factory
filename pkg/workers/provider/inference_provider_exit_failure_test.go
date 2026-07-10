package provider

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
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
		`{"type":"system","subtype":"init","session_id":"cursor-terminal-session"}`,
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
			name:        "OpenCodeClassifiesAuthenticationOutput",
			provider:    string(interfaces.ModelProviderOpenCode),
			result:      CommandResult{ExitCode: 1, Stdout: []byte("login required before continuing")},
			wantMessage: "login required before continuing",
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
