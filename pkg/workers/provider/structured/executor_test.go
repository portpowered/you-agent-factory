package structured_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers/provider/structured"
)

func TestProductionProviderBoundarySelectsStructuredClaudeOnlyForResponseStream(t *testing.T) {
	req := interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-claude-production"},
		ModelProvider: string(interfaces.ModelProviderClaude), Model: "claude-sonnet",
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

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-claude-image"},
		ModelProvider: string(interfaces.ModelProviderClaude),
		UserMessage:   "inspect",
		InputTokens: []any{interfaces.Token{ID: "token-1", Color: interfaces.TokenColor{Content: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "caption"},
			{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/mockup.png"},
		}}}},
	})
	if err == nil {
		t.Fatal("expected response-stream Claude image content to fail")
	}
	var providerErr *workerprovider.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
	if providerErr.Type != interfaces.WorkFailureTypePermanentBadRequest ||
		!strings.Contains(providerErr.Message, "input_tokens[0].color.content[1].file") ||
		!strings.Contains(providerErr.Message, "model provider claude") {
		t.Fatalf("provider error = %#v", providerErr)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
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
	result := executor.Execute(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-real-command"},
		ModelProvider: string(interfaces.ModelProviderClaude), UserMessage: "private prompt",
		EnvVars: map[string]string{"CLAUDE_FIXTURE": fixturePath},
	}, false, nil, func(fragment workerprovider.InferenceProgressFragment) {
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
	result := executor.Execute(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch:      interfaces.WorkDispatch{DispatchID: "dispatch-failure"},
		ModelProvider: string(interfaces.ModelProviderClaude), UserMessage: "private prompt",
	}, false, &recordingRunner{result: workerprovider.CommandResult{
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

type recordingRunner struct {
	request workerprovider.CommandRequest
	result  workerprovider.CommandResult
	calls   int
}

func (r *recordingRunner) Run(_ context.Context, req workerprovider.CommandRequest) (workerprovider.CommandResult, error) {
	r.calls++
	r.request = req
	return r.result, nil
}

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
