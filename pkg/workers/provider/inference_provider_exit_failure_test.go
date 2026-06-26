package provider

import (
	"context"
	"os"
	"path/filepath"
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

func TestScriptWrapProvider_Infer_CursorAndCodexExitFailuresKeepCodexDerivedBehavior(t *testing.T) {
	for _, tc := range codexDerivedExitFailureTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertInferenceExitFailure(t, tc)
		})
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
			name:        "CursorUsesCodexErrorExtraction",
			provider:    string(interfaces.ModelProviderCursor),
			result:      CommandResult{ExitCode: 1, Stderr: []byte("noise before\nERROR: unexpected status 500 from cursor upstream")},
			wantMessage: "ERROR: unexpected status 500 from cursor upstream",
			wantType:    interfaces.WorkFailureTypeInternalServerError,
		},
		{
			name:        "CodexUsesCodexErrorExtraction",
			provider:    string(interfaces.ModelProviderCodex),
			result:      CommandResult{ExitCode: 1, Stderr: []byte("noise before\nERROR: unexpected status 500 from codex upstream")},
			wantMessage: "ERROR: unexpected status 500 from codex upstream",
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
	t.Parallel()

	scriptPath := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' '{\"event\":\"session.created\",\"session_id\":\"sess-codex-1\"}'\n" +
		"printf '%s\\n' '{\"type\":\"response.output_text.delta\",\"delta\":\"hello from delta\"}'\n" +
		"printf '%s\\n' '{\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello final\"}]}]}}'\n" +
		"printf '%s\\n' 'planning update' 1>&2\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

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
	t.Parallel()

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
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

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
