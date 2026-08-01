package claude_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	effects "github.com/portpowered/infinite-you/pkg/services/providers/internal/effects"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	claude "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestCommandEffectPropagatesStreamingObserverFailure(t *testing.T) {
	t.Parallel()

	observerErr := errors.New("output observer failed")
	effect := claude.NewCommandEffect(streamingObserverErrorRunner{})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDClaude,
		AttemptID:   "observer-failure",
		UserMessage: "perform work",
	}, func([]byte) error { return observerErr })
	if !errors.Is(err, observerErr) {
		t.Fatalf("Execute() error = %v, want observer failure", err)
	}
}

type streamingObserverErrorRunner struct{}

func (streamingObserverErrorRunner) Run(
	context.Context,
	effects.CommandRequest,
) (effects.CommandResult, error) {
	return effects.CommandResult{}, nil
}

func (streamingObserverErrorRunner) RunStreaming(
	_ context.Context,
	_ effects.CommandRequest,
	observer effects.OutputChunkObserver,
) (effects.CommandResult, error) {
	return effects.CommandResult{}, observer(effects.OutputStreamStdout, []byte("output"))
}

func TestClaudeRootPreservesRequestOrderedStreamFinalAndSession(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:         providers.IDClaude,
		AttemptID:        "attempt-claude-success",
		SystemPrompt:     "system contract",
		UserMessage:      "perform the accepted work",
		OutputSchema:     `{"type":"object"}`,
		WorkingDirectory: "C:/factory",
		Worktree:         "C:/factory/tree",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDClaude,
			Kind:     providers.SessionIDKind,
			ID:       "session-previous",
		},
	}
	var received providers.ExecuteRequest
	stream := claudeSuccessStream()
	effect := claude.EffectFunc(func(
		_ context.Context,
		got providers.ExecuteRequest,
		observe func([]byte) error,
	) (claude.EffectResult, error) {
		received = got.Clone()
		for _, chunk := range splitEvery(stream, 17) {
			if err := observe(chunk); err != nil {
				return claude.EffectResult{}, err
			}
		}
		return claude.EffectResult{
			DurationMillis: 19,
			Metadata:       map[string]string{"transport": "stream-json"},
		}, nil
	})
	root := newClaudeRoot(t, effect)

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertClaudeSuccessResult(t, result, received, request)

	result.SessionRef.ID = "caller-mutated"
	result.Diagnostics.Progress[2].Metadata["caller"] = "mutated"
	second, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	assertRepeatedClaudeResultDetached(t, second)
}

func TestClaudeStreamDeltaPreservesWhitespace(t *testing.T) {
	t.Parallel()

	stream := encodeRecords([]any{
		map[string]any{"type": "system", "subtype": "init", "session_id": "claude-session-delta"},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-delta",
			"event": map[string]any{
				"type":    "message_start",
				"message": map[string]any{"id": "msg_delta", "role": "assistant", "content": []any{}},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-delta",
			"event": map[string]any{
				"type": "content_block_start", "index": 0,
				"content_block": map[string]any{"type": "text", "text": ""},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-delta",
			"event": map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "text_delta", "text": "Parity "},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-delta",
			"event": map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "text_delta", "text": "hello "},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-delta",
			"event": map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "text_delta", "text": "world COMPLETE"},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-delta",
			"event": map[string]any{"type": "message_stop"},
		},
		map[string]any{
			"type": "result", "subtype": "success", "is_error": false,
			"result": "Parity hello world COMPLETE", "session_id": "claude-session-delta",
		},
	})
	effect := claude.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (claude.EffectResult, error) {
		return claude.EffectResult{}, observe(stream)
	})
	root := newClaudeRoot(t, effect)

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDClaude,
		AttemptID:   "attempt-claude-delta-whitespace",
		UserMessage: "perform the accepted work",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var deltaText string
	for _, fact := range result.Diagnostics.Progress {
		if fact.Phase != "message.delta" {
			continue
		}
		deltaText += fact.Detail
	}
	if deltaText != "Parity hello world COMPLETE" {
		t.Fatalf("concatenated delta detail = %q, want %q", deltaText, "Parity hello world COMPLETE")
	}
}

func assertClaudeSuccessResult(
	t *testing.T,
	result providers.ExecuteResult,
	received providers.ExecuteRequest,
	request providers.ExecuteRequest,
) {
	t.Helper()
	if !reflect.DeepEqual(received, request) {
		t.Fatalf("native request = %#v, want %#v", received, request)
	}
	if received.ResumeSession == request.ResumeSession {
		t.Fatal("native request retained caller SessionRef pointer")
	}
	if result.Content != "authoritative final answer" {
		t.Fatalf("Content = %q", result.Content)
	}
	if result.SessionRef == nil ||
		result.SessionRef.Provider != providers.IDClaude ||
		result.SessionRef.Kind != providers.SessionIDKind ||
		result.SessionRef.ID != "claude-session-42" {
		t.Fatalf("SessionRef = %#v", result.SessionRef)
	}
	if result.Diagnostics == nil ||
		result.Diagnostics.DurationMillis != 19 ||
		result.Diagnostics.Metadata["transport"] != "stream-json" {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
	assertProgress(t, result.Diagnostics.Progress)
}

func assertRepeatedClaudeResultDetached(
	t *testing.T,
	second providers.ExecuteResult,
) {
	t.Helper()
	if second.SessionRef.ID != "claude-session-42" {
		t.Fatalf("second SessionRef = %#v", second.SessionRef)
	}
	if _, exists := second.Diagnostics.Progress[2].Metadata["caller"]; exists {
		t.Fatal("second result retained caller progress mutation")
	}
}

func TestClaudeDecoderFinalizesUnterminatedRecordOnce(t *testing.T) {
	t.Parallel()

	stream := []byte(
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"claude-session-partial\"}\n" +
			"{\"type\":\"assistant\",\"session_id\":\"claude-session-partial\"," +
			"\"message\":{\"id\":\"msg_partial\",\"role\":\"assistant\"," +
			"\"content\":[{\"type\":\"text\",\"text\":\"ignored for final\"}]}}\n" +
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false," +
			"\"result\":\"unterminated final\",\"session_id\":\"claude-session-partial\"}",
	)
	effect := claude.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (claude.EffectResult, error) {
		for _, chunk := range splitEvery(stream, 3) {
			if err := observe(chunk); err != nil {
				return claude.EffectResult{}, err
			}
		}
		return claude.EffectResult{}, nil
	})
	result, err := newClaudeRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
		Provider:  providers.IDClaude,
		AttemptID: "attempt-partial",
	})
	if err != nil || result.Content != "unterminated final" {
		t.Fatalf("Execute() = (%#v, %v)", result, err)
	}
	if got := countPhase(result.Diagnostics.Progress, "message.completed"); got < 1 {
		t.Fatalf("completed message facts = %d, want at least 1", got)
	}
}

func TestClaudeDecoderMapsMixedTextAndToolProgress(t *testing.T) {
	t.Parallel()

	stream := claudeToolStream()
	effect := claude.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (claude.EffectResult, error) {
		return claude.EffectResult{}, observe(stream)
	})
	result, err := newClaudeRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
		Provider:  providers.IDClaude,
		AttemptID: "attempt-tool",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "Hello world" {
		t.Fatalf("Content = %q", result.Content)
	}
	if got := countPhase(result.Diagnostics.Progress, "tool.started"); got != 1 {
		t.Fatalf("tool.started facts = %d, want 1", got)
	}
	if got := countPhase(result.Diagnostics.Progress, "tool.updated"); got != 1 {
		t.Fatalf("tool.updated facts = %d, want 1", got)
	}
	if got := countPhase(result.Diagnostics.Progress, "tool.completed"); got < 1 {
		t.Fatalf("tool.completed facts = %d, want at least 1", got)
	}
}

func assertProgress(t *testing.T, progress []providers.ExecuteProgress) {
	t.Helper()
	wantContains := []string{
		"session.started",
		"run.started",
		"message.started",
		"message.delta",
		"message.completed",
		"tool.started",
		"tool.updated",
		"tool.completed",
	}
	for _, phase := range wantContains {
		if countPhase(progress, phase) == 0 {
			t.Fatalf("progress missing phase %q in %#v", phase, progress)
		}
	}
	for _, fact := range progress {
		if fact.Phase == "message.delta" && fact.Metadata["message_id"] != "msg_claude_1" {
			t.Fatalf("message correlation = %#v", fact)
		}
		if fact.Phase == "tool.started" && fact.Metadata["correlation_id"] != "toolu_claude_1" {
			t.Fatalf("tool correlation = %#v", fact)
		}
	}
}

func claudeSuccessStream() []byte {
	records := []any{
		map[string]any{"type": "system", "subtype": "init", "session_id": "claude-session-42"},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-42",
			"event": map[string]any{
				"type":    "message_start",
				"message": map[string]any{"id": "msg_claude_1", "role": "assistant", "content": []any{}},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-42",
			"event": map[string]any{
				"type": "content_block_start", "index": 0,
				"content_block": map[string]any{"type": "text", "text": ""},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-42",
			"event": map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "text_delta", "text": "draft "},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-42",
			"event": map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "text_delta", "text": "answer"},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-42",
			"event": map[string]any{"type": "content_block_stop", "index": 0},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-42",
			"event": map[string]any{
				"type": "content_block_start", "index": 1,
				"content_block": map[string]any{
					"type": "tool_use", "id": "toolu_claude_1", "name": "lookup", "input": map[string]any{},
				},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-42",
			"event": map[string]any{
				"type": "content_block_delta", "index": 1,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": `{"query":"safe"}`},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-42",
			"event": map[string]any{"type": "content_block_stop", "index": 1},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-42",
			"event": map[string]any{"type": "message_stop"},
		},
		map[string]any{
			"type": "assistant", "session_id": "claude-session-42",
			"message": map[string]any{
				"id": "msg_claude_1", "role": "assistant",
				"content": []map[string]any{
					{"type": "text", "text": "draft answer"},
					{"type": "tool_use", "id": "toolu_claude_1", "name": "lookup", "input": map[string]any{"query": "safe"}},
				},
			},
		},
		map[string]any{
			"type": "result", "subtype": "success", "is_error": false,
			"result": "authoritative final answer", "session_id": "claude-session-42",
		},
	}
	return encodeRecords(records)
}

func claudeToolStream() []byte {
	records := []any{
		map[string]any{"type": "system", "subtype": "init", "session_id": "claude-session-tool"},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-tool",
			"event": map[string]any{
				"type":    "message_start",
				"message": map[string]any{"id": "msg_tool", "role": "assistant", "content": []any{}},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-tool",
			"event": map[string]any{
				"type": "content_block_start", "index": 0,
				"content_block": map[string]any{"type": "text", "text": ""},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-tool",
			"event": map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "text_delta", "text": "Hello "},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-tool",
			"event": map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "text_delta", "text": "world"},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-tool",
			"event": map[string]any{"type": "message_stop"},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-tool",
			"event": map[string]any{
				"type": "content_block_start", "index": 0,
				"content_block": map[string]any{
					"type": "tool_use", "id": "toolu_conformance", "name": "weather", "input": map[string]any{},
				},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-tool",
			"event": map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": `{"city":"Oslo"}`},
			},
		},
		map[string]any{
			"type": "stream_event", "session_id": "claude-session-tool",
			"event": map[string]any{"type": "content_block_stop", "index": 0},
		},
		map[string]any{
			"type": "result", "subtype": "success", "is_error": false,
			"result": "Hello world", "session_id": "claude-session-tool",
		},
	}
	return encodeRecords(records)
}

func encodeRecords(records []any) []byte {
	var stream []byte
	for _, record := range records {
		encoded, _ := json.Marshal(record)
		stream = append(stream, encoded...)
		stream = append(stream, '\n')
	}
	return stream
}

func splitEvery(value []byte, size int) [][]byte {
	var chunks [][]byte
	for len(value) > 0 {
		count := min(size, len(value))
		chunks = append(chunks, value[:count])
		value = value[count:]
	}
	return chunks
}

func countPhase(progress []providers.ExecuteProgress, phase string) int {
	count := 0
	for _, fact := range progress {
		if fact.Phase == phase {
			count++
		}
	}
	return count
}

func newClaudeRoot(t *testing.T, effect claude.Effect) providers.Service {
	t.Helper()
	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatal(err)
	}
	executionService, err := executionwire.NewService(
		catalog,
		claude.NewRegistration(effect),
	)
	if err != nil {
		t.Fatal(err)
	}
	root, err := providerservice.New(catalog, executionService)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
