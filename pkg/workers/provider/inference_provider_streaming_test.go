package provider

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	cursorpkg "github.com/portpowered/infinite-you/pkg/workers/provider/cursor"
)

func TestScriptWrapProvider_Infer_CursorErrorFlaggedSuccessPublishesOnlyCanonicalFailure(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, string(interfaces.ModelProviderCursor))
	writeExecutableTestScript(t, scriptPath, "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":true,\"result\":\"Request timed out\",\"session_id\":\"cursor-session-error\"}'\n")
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var publishedMu sync.Mutex
	var published []InferenceProgressFragment
	publish := func(fragment InferenceProgressFragment) {
		publishedMu.Lock()
		published = append(published, fragment)
		publishedMu.Unlock()
	}
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(NewInferenceProgressPublishingCommandRunner(publish, nil)),
		WithInferenceProgressPublisher(publish),
	)

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-cursor-error-flagged-success"},
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "private prompt",
	})
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	if providerErr.Type != interfaces.WorkFailureTypeTimeout || providerErr.Message != "Request timed out" {
		t.Fatalf("provider error = %#v, want canonical timeout", providerErr)
	}

	publishedMu.Lock()
	defer publishedMu.Unlock()
	var failed *InferenceProgressFragment
	for i := range published {
		if published[i].Kind == ResponseFragmentKind {
			t.Fatalf("published fragments = %#v, error result must not emit a response", published)
		}
		if published[i].Kind == FailedFragmentKind {
			failed = &published[i]
		}
	}
	if failed == nil || failed.Payload != providerErr.Message {
		t.Fatalf("published fragments = %#v, want canonical failed marker", published)
	}
	if failed.ProviderSessionRef == nil || failed.ProviderSessionRef.ID != "cursor-session-error" {
		t.Fatalf("failed provider session = %#v, want cursor-session-error", failed.ProviderSessionRef)
	}
}

func TestScriptWrapProvider_Infer_CursorZeroExitTerminalFailureCarriesCanonicalResultOnce(t *testing.T) {
	stdout := []byte(
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor-initial-session\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"timeout\",\"is_error\":true,\"result\":\"Cursor terminal request timed out\",\"session_id\":\"cursor-final-session\"}\n",
	)
	var published []InferenceProgressFragment
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(&recordingProviderExec{result: CommandResult{
			Stdout: stdout,
			Stderr: []byte("unrelated authentication failed"),
		}}),
		WithInferenceProgressPublisher(func(fragment InferenceProgressFragment) {
			published = append(published, fragment)
		}),
	)

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-cursor-zero-exit-failure"},
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "private prompt",
	})
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	if providerErr.Type != interfaces.WorkFailureTypeTimeout || providerErr.Message != "Cursor terminal request timed out" {
		t.Fatalf("provider error = %#v, want canonical terminal timeout", providerErr)
	}
	if providerErr.ProviderSession == nil || providerErr.ProviderSession.ID != "cursor-final-session" {
		t.Fatalf("provider session = %#v, want cursor-final-session", providerErr.ProviderSession)
	}
	if len(published) != 1 || published[0].Kind != FailedFragmentKind || published[0].Payload != providerErr.Message {
		t.Fatalf("published fragments = %#v, want one canonical failed marker", published)
	}
	if published[0].ProviderSessionRef == nil || published[0].ProviderSessionRef.ID != providerErr.ProviderSession.ID {
		t.Fatalf("published provider session = %#v, want final provider error session %#v", published[0].ProviderSessionRef, providerErr.ProviderSession)
	}
}

func TestScriptWrapProvider_Infer_CursorMalformedStructuredOutputDoesNotPublishPromptText(t *testing.T) {
	privatePrompt := "deploy production using the customer launch phrase"
	stdout := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"` + privatePrompt + `"}]}`)
	var published []InferenceProgressFragment
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(&recordingProviderExec{result: CommandResult{Stdout: stdout, ExitCode: 1}}),
		WithInferenceProgressPublisher(func(fragment InferenceProgressFragment) {
			published = append(published, fragment)
		}),
	)

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-cursor-malformed-structured"},
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   privatePrompt,
	})
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	if strings.Contains(providerErr.Message, privatePrompt) {
		t.Fatalf("provider message = %q, must not surface malformed assistant content", providerErr.Message)
	}
	if len(published) != 1 || published[0].Kind != FailedFragmentKind || published[0].Payload != providerErr.Message {
		t.Fatalf("published fragments = %#v, want one canonical failure marker", published)
	}
	if strings.Contains(published[0].Payload, privatePrompt) {
		t.Fatalf("failure fragment = %q, must not surface malformed assistant content", published[0].Payload)
	}
}

func TestScriptWrapProvider_Infer_CursorParsesStreamJSONResult(t *testing.T) {
	stdout := []byte(
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor-session-abc\"}\n" +
			"{\"type\":\"assistant\",\"timestamp_ms\":1,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Parsed \"}]},\"session_id\":\"cursor-session-abc\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"Parsed assistant answer.\",\"session_id\":\"cursor-session-abc\"}\n",
	)
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: stdout},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	resp, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCursor),
		Model:         "gpt-5",
		UserMessage:   "run the tests",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.Content != "Parsed assistant answer." {
		t.Fatalf("content = %q, want parsed result text", resp.Content)
	}
	if resp.Content == string(stdout) {
		t.Fatal("content must not be raw JSON stdout")
	}
	if resp.ProviderSession == nil {
		t.Fatal("expected provider session metadata")
	}
	if resp.ProviderSession.Provider != "cursor" {
		t.Fatalf("provider = %q, want cursor", resp.ProviderSession.Provider)
	}
	if resp.ProviderSession.ID != "cursor-session-abc" {
		t.Fatalf("session id = %q, want cursor-session-abc", resp.ProviderSession.ID)
	}
	if resp.Diagnostics == nil || resp.Diagnostics.Command == nil {
		t.Fatal("expected command diagnostics on success")
	}
	if string(resp.Diagnostics.Command.Stdout) != string(stdout) {
		t.Fatal("command diagnostics should retain raw stdout for observability")
	}
}

func TestScriptWrapProvider_Infer_CursorPublishesTerminalCompletionMarker(t *testing.T) {
	stdout := cursorpkg.SuccessStdoutJSON("Parsed assistant answer.", "cursor-session-abc")
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: stdout},
	}
	var published []InferenceProgressFragment
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(fakeExec),
		WithInferenceProgressPublisher(func(fragment InferenceProgressFragment) {
			published = append(published, fragment)
		}),
	)

	resp, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-cursor-success"},
		ModelProvider: string(interfaces.ModelProviderCursor),
		Model:         "gpt-5",
		UserMessage:   "run the tests",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.ProviderSession == nil {
		t.Fatal("expected provider session metadata")
	}
	if len(published) != 1 {
		t.Fatalf("published fragments = %#v, want one completion marker", published)
	}
	if published[0].Kind != CompletedFragmentKind {
		t.Fatalf("published kind = %q, want %q", published[0].Kind, CompletedFragmentKind)
	}
	if published[0].DispatchID != "dispatch-cursor-success" {
		t.Fatalf("dispatch id = %q, want dispatch-cursor-success", published[0].DispatchID)
	}
	if published[0].ProviderSessionRef == nil || published[0].ProviderSessionRef.ID != "cursor-session-abc" {
		t.Fatalf("provider session ref = %#v, want cursor-session-abc", published[0].ProviderSessionRef)
	}
}

func TestScriptWrapProvider_Infer_CursorCompletionPublisherPreservesFinalResponse(t *testing.T) {
	stdout := cursorpkg.SuccessStdoutJSON("Parsed assistant answer.", "cursor-session-abc")
	req := interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-cursor-success"},
		ModelProvider: string(interfaces.ModelProviderCursor),
		Model:         "gpt-5",
		UserMessage:   "run the tests",
	}

	withoutPublisher := NewScriptWrapProvider(WithProviderCommandRunner(&recordingProviderExec{
		result: CommandResult{Stdout: stdout},
	}))
	want, err := withoutPublisher.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("Infer without publisher returned error: %v", err)
	}

	var published []InferenceProgressFragment
	withPublisher := NewScriptWrapProvider(
		WithProviderCommandRunner(&recordingProviderExec{
			result: CommandResult{Stdout: stdout},
		}),
		WithInferenceProgressPublisher(func(fragment InferenceProgressFragment) {
			published = append(published, fragment)
		}),
	)
	got, err := withPublisher.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("Infer with publisher returned error: %v", err)
	}

	assertEquivalentInferenceResponse(t, got, want)
	if len(published) != 1 || published[0].Kind != CompletedFragmentKind {
		t.Fatalf("published fragments = %#v, want one completion marker", published)
	}
}

func TestScriptWrapProvider_Infer_CursorMalformedJSONReturnsProviderError(t *testing.T) {
	stdout := []byte(`{"type":"result"`)
	stderr := []byte("cursor stderr detail")
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: stdout, Stderr: stderr},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "run the tests",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Type != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("error type = %q, want permanent_bad_request", providerErr.Type)
	}
	if providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil {
		t.Fatal("expected command diagnostics on parse failure")
	}
	if got := providerErr.Diagnostics.Command.Stdout; got != string(stdout) {
		t.Fatalf("command stdout = %q, want full stdout for worker-internal diagnostics", got)
	}
	if got := providerErr.Diagnostics.Command.Stderr; got != string(stderr) {
		t.Fatalf("command stderr = %q, want full stderr for worker-internal diagnostics", got)
	}
	assertCursorFailureExcerpts(t, providerErr.Diagnostics, string(stdout), string(stderr))
	assertSafeCursorFailureExcerpts(t, providerErr.Diagnostics)
}

func TestScriptWrapProvider_Infer_CursorParseFailureUsesStderrParserResult(t *testing.T) {
	stdout := []byte(`{"type":"result"`)
	stderr := []byte("Cursor authentication failed; sign in again")
	provider := NewScriptWrapProvider(WithProviderCommandRunner(&recordingProviderExec{
		result: CommandResult{Stdout: stdout, Stderr: stderr},
	}))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "private prompt",
	})
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	if providerErr.Type != interfaces.WorkFailureTypeAuthFailure || providerErr.Message != string(stderr) {
		t.Fatalf("provider error = %#v, want canonical stderr authentication result", providerErr)
	}
	if providerErr.Cause == nil {
		t.Fatal("parse failure cause = nil, want original JSON parse cause")
	}
}

func TestScriptWrapProvider_Infer_CursorExitFailurePreservesBoundedDiagnosticsExcerpts(t *testing.T) {
	stdout := []byte("partial json output")
	stderr := []byte("noise before\nERROR: unexpected status 500 from cursor upstream")
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: 1,
		},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "run the tests",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Type != interfaces.WorkFailureTypeInternalServerError {
		t.Fatalf("error type = %q, want internal_server_error", providerErr.Type)
	}
	if providerErr.Message != "ERROR: unexpected status 500 from cursor upstream" {
		t.Fatalf("error message = %q", providerErr.Message)
	}
	assertCursorFailureExcerpts(t, providerErr.Diagnostics, string(stdout), string(stderr))
	assertSafeCursorFailureExcerpts(t, providerErr.Diagnostics)
}

func TestScriptWrapProvider_Infer_CursorExitFailurePublishesTerminalFailureMarker(t *testing.T) {
	stdout := []byte("partial json output")
	stderr := []byte("noise before\nERROR: unexpected status 500 from cursor upstream")
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: 1,
		},
	}
	var published []InferenceProgressFragment
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(fakeExec),
		WithInferenceProgressPublisher(func(fragment InferenceProgressFragment) {
			published = append(published, fragment)
		}),
	)

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-cursor-failure"},
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "run the tests",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	if len(published) != 1 {
		t.Fatalf("published fragments = %#v, want one failure marker", published)
	}
	if published[0].Kind != FailedFragmentKind {
		t.Fatalf("published kind = %q, want %q", published[0].Kind, FailedFragmentKind)
	}
	if published[0].DispatchID != "dispatch-cursor-failure" {
		t.Fatalf("dispatch id = %q, want dispatch-cursor-failure", published[0].DispatchID)
	}
	if published[0].Payload != "ERROR: unexpected status 500 from cursor upstream" {
		t.Fatalf("failure payload = %q, want normalized provider error message", published[0].Payload)
	}
}

func TestScriptWrapProvider_Infer_CursorFailurePublisherPreservesProviderError(t *testing.T) {
	stdout := []byte("partial json output")
	stderr := []byte("noise before\nERROR: unexpected status 500 from cursor upstream")
	req := interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-cursor-failure"},
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "run the tests",
	}

	withoutPublisher := NewScriptWrapProvider(WithProviderCommandRunner(&recordingProviderExec{
		result: CommandResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: 1,
		},
	}))
	_, err := withoutPublisher.Infer(context.Background(), req)
	if err == nil {
		t.Fatal("expected Infer without publisher to fail")
	}
	want, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError without publisher, got %T", err)
	}

	var published []InferenceProgressFragment
	withPublisher := NewScriptWrapProvider(
		WithProviderCommandRunner(&recordingProviderExec{
			result: CommandResult{
				Stdout:   stdout,
				Stderr:   stderr,
				ExitCode: 1,
			},
		}),
		WithInferenceProgressPublisher(func(fragment InferenceProgressFragment) {
			published = append(published, fragment)
		}),
	)
	_, err = withPublisher.Infer(context.Background(), req)
	if err == nil {
		t.Fatal("expected Infer with publisher to fail")
	}
	got, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError with publisher, got %T", err)
	}

	assertEquivalentProviderError(t, got, want)
	if len(published) != 1 || published[0].Kind != FailedFragmentKind {
		t.Fatalf("published fragments = %#v, want one failure marker", published)
	}
}

func TestScriptWrapProvider_Infer_ClaudeCompletionPublisherPreservesFinalResponse(t *testing.T) {
	req := interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-claude-success"},
		ModelProvider: string(interfaces.ModelProviderClaude),
		Model:         "claude-sonnet-4-5-20250514",
		SessionID:     "claude-session-123",
		UserMessage:   "fix it",
	}

	withoutPublisher := NewScriptWrapProvider(WithProviderCommandRunner(&recordingProviderExec{
		result: CommandResult{Stdout: []byte("claude output")},
	}))
	want, err := withoutPublisher.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("Infer without publisher returned error: %v", err)
	}

	var published []InferenceProgressFragment
	withPublisher := NewScriptWrapProvider(
		WithProviderCommandRunner(&recordingProviderExec{
			result: CommandResult{Stdout: []byte("claude output")},
		}),
		WithInferenceProgressPublisher(func(fragment InferenceProgressFragment) {
			published = append(published, fragment)
		}),
	)
	got, err := withPublisher.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("Infer with publisher returned error: %v", err)
	}

	assertEquivalentInferenceResponse(t, got, want)
	if len(published) != 1 || published[0].Kind != CompletedFragmentKind {
		t.Fatalf("published fragments = %#v, want one completion marker", published)
	}
	if published[0].ProviderSessionRef == nil || published[0].ProviderSessionRef.ID != "claude-session-123" {
		t.Fatalf("provider session ref = %#v, want claude-session-123", published[0].ProviderSessionRef)
	}
}

func assertCursorFailureExcerpts(t *testing.T, diagnostics *interfaces.WorkDiagnostics, wantStdout, wantStderr string) {
	t.Helper()
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatal("expected provider diagnostics with failure excerpts")
	}
	metadata := diagnostics.Provider.ResponseMetadata
	if got := metadata[cursorpkg.ResponseMetadataStdoutExcerpt]; got != wantStdout {
		t.Fatalf("stdout excerpt = %q, want %q", got, wantStdout)
	}
	if got := metadata[cursorpkg.ResponseMetadataStderrExcerpt]; got != wantStderr {
		t.Fatalf("stderr excerpt = %q, want %q", got, wantStderr)
	}
}

func assertSafeCursorFailureExcerpts(t *testing.T, diagnostics *interfaces.WorkDiagnostics) {
	t.Helper()
	safe := interfaces.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics)
	if safe == nil || safe.Provider == nil {
		t.Fatal("expected safe provider diagnostics")
	}
	if safe.Provider.ResponseMetadata[cursorpkg.ResponseMetadataStdoutExcerpt] == "" {
		t.Fatal("expected safe stdout excerpt")
	}
	if safe.Provider.ResponseMetadata[cursorpkg.ResponseMetadataStderrExcerpt] == "" {
		t.Fatal("expected safe stderr excerpt")
	}
	if safe.Provider.ResponseMetadata["raw_body"] != "" {
		t.Fatal("safe diagnostics must not include unsafe metadata keys")
	}
}

func assertEquivalentInferenceResponse(t *testing.T, got, want interfaces.InferenceResponse) {
	t.Helper()
	if got.Content != want.Content {
		t.Fatalf("content = %q, want %q", got.Content, want.Content)
	}
	if !reflect.DeepEqual(got.ProviderSession, want.ProviderSession) {
		t.Fatalf("provider session = %#v, want %#v", got.ProviderSession, want.ProviderSession)
	}
	assertEquivalentWorkDiagnostics(t, got.Diagnostics, want.Diagnostics)
}

func assertEquivalentProviderError(t *testing.T, got, want *ProviderError) {
	t.Helper()
	if got.Type != want.Type {
		t.Fatalf("error type = %q, want %q", got.Type, want.Type)
	}
	if got.Message != want.Message {
		t.Fatalf("error message = %q, want %q", got.Message, want.Message)
	}
	if !reflect.DeepEqual(got.ProviderSession, want.ProviderSession) {
		t.Fatalf("provider session = %#v, want %#v", got.ProviderSession, want.ProviderSession)
	}
	assertEquivalentWorkDiagnostics(t, got.Diagnostics, want.Diagnostics)
}

func assertEquivalentWorkDiagnostics(t *testing.T, got, want *interfaces.WorkDiagnostics) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("diagnostics presence = %#v, want %#v", got, want)
	}
	if got == nil {
		return
	}
	if !reflect.DeepEqual(got.Provider, want.Provider) {
		t.Fatalf("provider diagnostics = %#v, want %#v", got.Provider, want.Provider)
	}
	if !reflect.DeepEqual(got.Metadata, want.Metadata) {
		t.Fatalf("diagnostics metadata = %#v, want %#v", got.Metadata, want.Metadata)
	}
	if !reflect.DeepEqual(got.RenderedPrompt, want.RenderedPrompt) {
		t.Fatalf("rendered prompt diagnostics = %#v, want %#v", got.RenderedPrompt, want.RenderedPrompt)
	}
	if !reflect.DeepEqual(got.Panic, want.Panic) {
		t.Fatalf("panic diagnostics = %#v, want %#v", got.Panic, want.Panic)
	}
	assertEquivalentCommandDiagnostic(t, got.Command, want.Command)
}

func assertEquivalentCommandDiagnostic(t *testing.T, got, want *interfaces.CommandDiagnostic) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("command diagnostics presence = %#v, want %#v", got, want)
	}
	if got == nil {
		return
	}
	if got.Command != want.Command ||
		!reflect.DeepEqual(got.Args, want.Args) ||
		got.Stdin != want.Stdin ||
		!reflect.DeepEqual(got.Env, want.Env) ||
		got.Stdout != want.Stdout ||
		got.Stderr != want.Stderr ||
		got.ExitCode != want.ExitCode ||
		got.TimedOut != want.TimedOut ||
		got.WorkingDir != want.WorkingDir {
		t.Fatalf("command diagnostics = %#v, want %#v", got, want)
	}
}
