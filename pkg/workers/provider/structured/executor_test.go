package structured_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/work"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers/provider/structured"
)

func TestProductionProviderBoundarySelectsStructuredClaudeOnlyForResponseStream(t *testing.T) {
	req := workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-claude-production"},
		ModelProvider: string(modelprovider.Claude), Model: "claude-sonnet",
		SessionID: "claude-session-123", UserMessage: "private prompt",
		EnvVars: map[string]string{"GIT_EDITOR": "vim", "GIT_TERMINAL_PROMPT": "1"},
	}

	finalRunner := &recordingRunner{result: workerprovider.CommandResult{Stdout: []byte("authoritative answer")}}
	finalProvider := workerprovider.NewScriptWrapProvider(workerprovider.WithProviderCommandRunner(finalRunner))
	finalResponse, err := finalProvider.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("final-only Infer: %v", err)
	}
	if finalResponse.Content != "authoritative answer" || slices.Contains(finalRunner.request.Args, "--include-partial-messages") {
		t.Fatalf("final-only response/request = %#v / %#v", finalResponse, finalRunner.request)
	}

	streamRunner := &recordingRunner{result: workerprovider.CommandResult{Stdout: []byte(structuredClaudeOutput())}}
	var published []workerprovider.InferenceProgressFragment
	streamProvider := workerprovider.NewScriptWrapProvider(
		workerprovider.WithProviderCommandRunner(streamRunner),
		workerprovider.WithInferenceProgressPublisher(func(fragment workerprovider.InferenceProgressFragment) {
			published = append(published, fragment)
		}),
		workerprovider.WithResponseStreamExecutor(structured.NewExecutor()),
	)
	streamResponse, err := streamProvider.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("response-stream Infer: %v", err)
	}
	if streamResponse.Content != finalResponse.Content || streamResponse.ProviderSession == nil || streamResponse.ProviderSession.ID != "claude-session-123" {
		t.Fatalf("response-stream final response = %#v", streamResponse)
	}
	if !slices.Contains(streamRunner.request.Args, "stream-json") || !slices.Contains(streamRunner.request.Args, "--include-partial-messages") {
		t.Fatalf("response-stream args = %#v", streamRunner.request.Args)
	}
	assertCommandEnv(t, streamRunner.request.Env, "GIT_EDITOR", "true")
	assertCommandEnv(t, streamRunner.request.Env, "GIT_SEQUENCE_EDITOR", "true")
	assertCommandEnv(t, streamRunner.request.Env, "GIT_TERMINAL_PROMPT", "0")
	assertStablePublishedMessage(t, published)
}

func TestProductionResponseStreamClaudeRejectsImageBeforeRunner(t *testing.T) {
	runner := &recordingRunner{result: workerprovider.CommandResult{Stdout: []byte(structuredClaudeOutput())}}
	provider := workerprovider.NewScriptWrapProvider(
		workerprovider.WithProviderCommandRunner(runner),
		workerprovider.WithInferenceProgressPublisher(func(workerprovider.InferenceProgressFragment) {}),
		workerprovider.WithResponseStreamExecutor(structured.NewExecutor()),
	)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-claude-image"},
		ModelProvider: string(modelprovider.Claude),
		UserMessage:   "inspect",
		InputTokens: []any{factorytoken.Token{ID: "token-1", Color: factorytoken.Color{Content: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "caption"},
			{Type: work.WorkContentPartTypeImage, File: "fixtures/mockup.png"},
		}}}},
	})
	if err == nil {
		t.Fatal("expected response-stream Claude image content to fail")
	}
	var providerErr *workerprovider.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
	if providerErr.Type != workerexecution.WorkFailureTypePermanentBadRequest ||
		!strings.Contains(providerErr.Message, "input_tokens[0].color.content[1].file") ||
		!strings.Contains(providerErr.Message, "model provider claude") {
		t.Fatalf("provider error = %#v", providerErr)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestProductionProviderBoundarySelectsStructuredCodexOnlyForCapableRunner(t *testing.T) {
	req := workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-codex-production"},
		ModelProvider: string(modelprovider.Codex), Model: "gpt-test",
		UserMessage: "private prompt", WorkingDirectory: t.TempDir(),
	}
	plainRunner := &recordingRunner{result: workerprovider.CommandResult{Stdout: []byte("plain answer")}}
	plainProvider := workerprovider.NewScriptWrapProvider(
		workerprovider.WithProviderCommandRunner(plainRunner),
		workerprovider.WithInferenceProgressPublisher(func(workerprovider.InferenceProgressFragment) {}),
		workerprovider.WithResponseStreamExecutor(structured.NewExecutor()),
	)
	plainResponse, err := plainProvider.Infer(context.Background(), req)
	if err != nil || plainResponse.Content != "plain answer" || slices.Contains(plainRunner.request.Args, "--json") {
		t.Fatalf("non-streaming response/request = %#v / %#v, %v", plainResponse, plainRunner.request, err)
	}

	streamRunner := &codexCapableRunner{recordingRunner: recordingRunner{result: workerprovider.CommandResult{Stdout: []byte(structuredCodexOutput())}}}
	var published []workerprovider.InferenceProgressFragment
	streamProvider := workerprovider.NewScriptWrapProvider(
		workerprovider.WithProviderCommandRunner(streamRunner),
		workerprovider.WithInferenceProgressPublisher(func(fragment workerprovider.InferenceProgressFragment) { published = append(published, fragment) }),
		workerprovider.WithResponseStreamExecutor(structured.NewExecutor()),
	)
	streamResponse, err := streamProvider.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("response-stream Infer: %v", err)
	}
	if streamResponse.Content != "authoritative answer" || streamResponse.ProviderSession == nil || streamResponse.ProviderSession.ID != "thread-codex-production" {
		t.Fatalf("response-stream response = %#v", streamResponse)
	}
	if !slices.Contains(streamRunner.request.Args, "--json") || string(streamRunner.request.Stdin) != req.UserMessage || streamRunner.request.WorkDir != req.WorkingDirectory {
		t.Fatalf("response-stream request = %#v", streamRunner.request)
	}
	if !publishedDraft(published, responseevents.KindMessage, responseevents.PhaseCompleted, "message-codex-production") {
		t.Fatalf("published fragments = %#v, want authoritative native message", published)
	}
	assertPublishedCodexErrorItem(t, published)
}

func assertPublishedCodexErrorItem(t *testing.T, published []workerprovider.InferenceProgressFragment) {
	t.Helper()
	errorDraft := publishedDraftByIdentity(published, responseevents.KindError, responseevents.PhaseUpdated, "error-codex-production")
	if errorDraft == nil || errorDraft.Provenance.NativeEventType != "item.completed" {
		t.Fatalf("published fragments = %#v, want non-terminal native error item", published)
	}
	var errorPayload responseevents.ErrorPayload
	if err := json.Unmarshal(errorDraft.Payload, &errorPayload); err != nil {
		t.Fatalf("decode error item payload: %v", err)
	}
	if errorPayload.Code != "codex_item_error" || errorPayload.Message != "A recoverable operation was skipped." || errorPayload.Retryable {
		t.Fatalf("error item payload = %#v", errorPayload)
	}
}

func TestExecutorReconcilesCodexTerminalDraftsWithCommandOutcome(t *testing.T) {
	tests := []struct {
		name            string
		result          workerprovider.CommandResult
		runErr          error
		wantFailure     workerexecution.WorkFailureType
		wantNativeError string
	}{
		{
			name: "recognized failure survives later cleanup",
			result: workerprovider.CommandResult{Stdout: []byte(strings.Join([]string{
				`{"type":"thread.started","thread_id":"thread-terminal"}`,
				`{"type":"turn.failed","error":{"message":"unexpected status 429"}}`,
				`{"type":"error","message":"cleanup detail"}`,
			}, "\n") + "\n")},
			wantFailure: workerexecution.WorkFailureTypeThrottled, wantNativeError: "turn.failed",
		},
		{
			name:   "deadline suppresses native failure",
			result: workerprovider.CommandResult{Stdout: []byte(`{"type":"turn.failed","error":{"message":"unexpected status 429"}}` + "\n")},
			runErr: context.DeadlineExceeded, wantFailure: workerexecution.WorkFailureTypeTimeout,
		},
		{
			name:        "exit 124 suppresses native failure",
			result:      workerprovider.CommandResult{ExitCode: 124, Stdout: []byte(`{"type":"turn.failed","error":{"message":"unexpected status 429"}}` + "\n")},
			wantFailure: workerexecution.WorkFailureTypeTimeout,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &recordingRunner{result: tc.result, err: tc.runErr}
			var published []workerprovider.InferenceProgressFragment
			result := structured.NewExecutor().Execute(context.Background(), workerexecution.ProviderInferenceRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-terminal"}, ModelProvider: string(modelprovider.Codex), UserMessage: "private prompt",
			}, false, nil, runner, func(fragment workerprovider.InferenceProgressFragment) { published = append(published, fragment) }, nil)
			if result.FailureType != tc.wantFailure {
				t.Fatalf("failure = %#v, want %q", result, tc.wantFailure)
			}
			var nativeErrors []responseevents.Draft
			for _, fragment := range published {
				if draft, ok := fragment.CanonicalDraft.(responseevents.Draft); ok && draft.Kind == responseevents.KindError {
					nativeErrors = append(nativeErrors, draft)
				}
			}
			if tc.wantNativeError == "" && len(nativeErrors) != 0 {
				t.Fatalf("native terminal drafts = %#v, want none", nativeErrors)
			}
			if tc.wantNativeError != "" && (len(nativeErrors) != 1 || nativeErrors[0].Provenance.NativeEventType != tc.wantNativeError || !result.CanonicalFailurePublished) {
				t.Fatalf("native terminal drafts/result = %#v / %#v", nativeErrors, result)
			}
		})
	}
}

func TestExecutorStreamsRealCommandAndReportsSupport(t *testing.T) {
	executor := structured.NewExecutor()
	if !executor.Supports(" CLAUDE ") || executor.Supports("unknown-provider") {
		t.Fatalf("Supports() did not use normalized registered identities")
	}
	var nilExecutor *structured.Executor
	if nilExecutor.Supports("claude") {
		t.Fatal("nil executor unexpectedly supports Claude")
	}

	fixturePath := filepath.Join(t.TempDir(), "claude-output.jsonl")
	if err := os.WriteFile(fixturePath, []byte(structuredClaudeOutput()), 0o600); err != nil {
		t.Fatalf("write Claude fixture: %v", err)
	}
	commandDir := t.TempDir()
	writeClaudeFixtureCommand(t, commandDir)
	t.Setenv("PATH", commandDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var published []workerprovider.InferenceProgressFragment
	result := executor.Execute(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-real-command"},
		ModelProvider: string(modelprovider.Claude), UserMessage: "private prompt",
		EnvVars: map[string]string{"CLAUDE_FIXTURE": fixturePath},
	}, false, nil, nil, func(fragment workerprovider.InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil)
	if result.Err != nil || result.FailureType != "" || result.Response.Content != "authoritative answer" {
		t.Fatalf("real command result = %#v", result)
	}
	if len(published) == 0 {
		t.Fatal("real streaming command published no canonical drafts")
	}
}

func TestExecutorReturnsStructuredFailureFromBufferedRunner(t *testing.T) {
	executor := structured.NewExecutor()
	result := executor.Execute(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-failure"},
		ModelProvider: string(modelprovider.Claude), UserMessage: "private prompt",
	}, false, nil, &recordingRunner{result: workerprovider.CommandResult{
		Stdout: []byte(`{"type":"system","subtype":"api_retry","attempt":2,"retry_delay_ms":1000}` + "\n"),
		Stderr: []byte("private provider warning"), ExitCode: 1,
	}}, nil, nil)
	if result.FailureType == "" || result.FailureMessage == "" {
		t.Fatalf("failure result = %#v", result)
	}
}

func writeClaudeFixtureCommand(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "claude.cmd")
		if err := os.WriteFile(path, []byte("@type \"%CLAUDE_FIXTURE%\"\r\n"), 0o600); err != nil {
			t.Fatalf("write Claude command: %v", err)
		}
		return
	}
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat \"$CLAUDE_FIXTURE\"\n"), 0o700); err != nil {
		t.Fatalf("write Claude command: %v", err)
	}
}

func assertStablePublishedMessage(t *testing.T, fragments []workerprovider.InferenceProgressFragment) {
	t.Helper()
	var delta, completed *responseevents.Draft
	for _, fragment := range fragments {
		draft, ok := fragment.CanonicalDraft.(responseevents.Draft)
		if !ok || draft.ItemID != "msg-production" {
			continue
		}
		if draft.Phase == responseevents.PhaseDelta {
			delta = &draft
		}
		if draft.Phase == responseevents.PhaseCompleted {
			completed = &draft
		}
	}
	if delta == nil || completed == nil || delta.ItemID != completed.ItemID || delta.DispatchID != "dispatch-claude-production" {
		t.Fatalf("published fragments = %#v, want correlated stable message lifecycle", fragments)
	}
}

func structuredClaudeOutput() string {
	return strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"claude-session-123"}`,
		`{"type":"stream_event","session_id":"claude-session-123","event":{"type":"message_start","message":{"id":"msg-production","role":"assistant","content":[]}}}`,
		`{"type":"stream_event","session_id":"claude-session-123","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}`,
		`{"type":"stream_event","session_id":"claude-session-123","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"authoritative answer"}}}`,
		`{"type":"stream_event","session_id":"claude-session-123","event":{"type":"content_block_stop","index":0}}`,
		`{"type":"stream_event","session_id":"claude-session-123","event":{"type":"message_stop"}}`,
		`{"type":"assistant","session_id":"claude-session-123","message":{"id":"msg-production","role":"assistant","content":[{"type":"text","text":"authoritative answer"}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"authoritative answer","session_id":"claude-session-123"}`,
	}, "\n") + "\n"
}

func structuredCodexOutput() string {
	return strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-codex-production"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"error-codex-production","type":"error","message":"A recoverable operation was skipped."}}`,
		`{"type":"item.completed","item":{"id":"message-codex-production","type":"agent_message","text":"authoritative answer"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":4,"output_tokens":2}}`,
	}, "\n") + "\n"
}

func publishedDraft(fragments []workerprovider.InferenceProgressFragment, kind responseevents.Kind, phase responseevents.Phase, itemID string) bool {
	return publishedDraftByIdentity(fragments, kind, phase, itemID) != nil
}

func publishedDraftByIdentity(fragments []workerprovider.InferenceProgressFragment, kind responseevents.Kind, phase responseevents.Phase, itemID string) *responseevents.Draft {
	for _, fragment := range fragments {
		if draft, ok := fragment.CanonicalDraft.(responseevents.Draft); ok && draft.Kind == kind && draft.Phase == phase && draft.ItemID == itemID {
			return &draft
		}
	}
	return nil
}

type recordingRunner struct {
	request workerprovider.CommandRequest
	result  workerprovider.CommandResult
	calls   int
	err     error
}

func (r *recordingRunner) Run(_ context.Context, req workerprovider.CommandRequest) (workerprovider.CommandResult, error) {
	r.calls++
	r.request = req
	return r.result, r.err
}

type codexCapableRunner struct{ recordingRunner }

func (*codexCapableRunner) SupportsResponseStreaming() bool { return true }

func assertCommandEnv(t *testing.T, env []string, name, want string) {
	t.Helper()
	prefix := name + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if got := strings.TrimPrefix(entry, prefix); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
			return
		}
	}
	t.Fatalf("environment does not contain %s", name)
}
