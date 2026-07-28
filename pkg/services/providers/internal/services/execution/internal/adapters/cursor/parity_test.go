package cursor_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	cursor "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/cursor"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
)

func TestCursorRootRejectsUnsafeSessionRefAndOmitsPromptFromDiagnostics(t *testing.T) {
	t.Parallel()

	secretPrompt := "super-secret-prompt-material"
	stream := cursorUnsafeSessionStream(secretPrompt)
	effect := cursor.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (cursor.EffectResult, error) {
		return cursor.EffectResult{}, observe(stream)
	})
	root := newCursorRoot(t, effect)

	_, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:     providers.IDCursor,
		AttemptID:    "attempt-unsafe-session",
		SystemPrompt: secretPrompt,
		UserMessage:  "do work",
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want invalid session failure")
	}
	if strings.Contains(err.Error(), secretPrompt) {
		t.Fatalf("error leaked prompt: %q", err)
	}
}

func TestCursorRootPreservesPartialStreamCorrelationFacts(t *testing.T) {
	t.Parallel()

	stream := cursorPartialStream()
	effect := cursor.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (cursor.EffectResult, error) {
		for _, chunk := range splitEvery(stream, 11) {
			if err := observe(chunk); err != nil {
				return cursor.EffectResult{}, err
			}
		}
		return cursor.EffectResult{DurationMillis: 9}, nil
	})
	root := newCursorRoot(t, effect)

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDCursor,
		AttemptID:   "attempt-partial-stream",
		UserMessage: "continue",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "partial final" {
		t.Fatalf("Content = %q", result.Content)
	}
	if result.SessionRef == nil || result.SessionRef.ID != "cursor-session-partial" {
		t.Fatalf("SessionRef = %#v", result.SessionRef)
	}
	wantPhases := []string{
		"session.started",
		"message.delta",
		"tool.started",
		"tool.completed",
		"message.delta",
	}
	gotPhases := make([]string, len(result.Diagnostics.Progress))
	for index := range result.Diagnostics.Progress {
		gotPhases[index] = result.Diagnostics.Progress[index].Phase
	}
	if strings.Join(gotPhases, ",") != strings.Join(wantPhases, ",") {
		t.Fatalf("progress phases = %#v, want %#v", gotPhases, wantPhases)
	}
	for _, index := range []int{1, 4} {
		if result.Diagnostics.Progress[index].Metadata["message_id"] != "cursor-session-partial" {
			t.Fatalf("message correlation[%d] = %#v", index, result.Diagnostics.Progress[index])
		}
	}
	for _, index := range []int{2, 3} {
		if result.Diagnostics.Progress[index].Metadata["correlation_id"] != "call-partial-1" {
			t.Fatalf("tool correlation[%d] = %#v", index, result.Diagnostics.Progress[index])
		}
	}
	for _, fact := range result.Diagnostics.Progress {
		if strings.Contains(fact.Detail, secretPromptMarker) {
			t.Fatalf("progress leaked prompt detail: %#v", fact)
		}
	}
}

func TestCursorRootDetachedSessionRefRemainsAllowlisted(t *testing.T) {
	t.Parallel()

	stream := cursorSuccessStream()
	effect := cursor.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (cursor.EffectResult, error) {
		return cursor.EffectResult{}, observe(stream)
	})
	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatal(err)
	}
	executionService, err := executionwire.NewService(catalog, cursor.NewRegistration(effect))
	if err != nil {
		t.Fatal(err)
	}
	root, err := providerservice.New(catalog, executionService)
	if err != nil {
		t.Fatal(err)
	}

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDCursor,
		AttemptID:   "attempt-session-ref",
		UserMessage: "work",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.SessionRef.Provider != providers.IDCursor ||
		result.SessionRef.Kind != providers.SessionIDKind ||
		result.SessionRef.ID != "cursor-session-42" {
		t.Fatalf("SessionRef = %#v", result.SessionRef)
	}
	result.SessionRef.ID = "mutated"
	second, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDCursor,
		AttemptID:   "attempt-session-ref-2",
		UserMessage: "work",
	})
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if second.SessionRef.ID != "cursor-session-42" {
		t.Fatalf("second SessionRef = %#v", second.SessionRef)
	}
}

const secretPromptMarker = "super-secret-prompt-material"

func cursorUnsafeSessionStream(secretPrompt string) []byte {
	records := []any{
		map[string]any{"type": "system", "subtype": "init", "session_id": "bad session id"},
		map[string]any{
			"type": "assistant", "timestamp_ms": 1, "session_id": "bad session id",
			"message": map[string]any{
				"role": "assistant",
				"content": []map[string]any{{"type": "text", "text": secretPrompt}},
			},
		},
		map[string]any{
			"type": "result", "subtype": "success", "is_error": false,
			"result": "done", "session_id": "bad session id",
		},
	}
	return encodeCursorRecords(records)
}

func cursorPartialStream() []byte {
	records := []any{
		map[string]any{"type": "system", "subtype": "init", "session_id": "cursor-session-partial"},
		map[string]any{
			"type": "assistant", "timestamp_ms": 1, "session_id": "cursor-session-partial",
			"message": map[string]any{
				"role": "assistant",
				"content": []map[string]any{{"type": "text", "text": "draft"}},
			},
		},
		map[string]any{
			"type": "tool_call", "subtype": "started", "call_id": "call-partial-1",
			"session_id": "cursor-session-partial",
			"tool_call": map[string]any{"readToolCall": map[string]any{}},
		},
		map[string]any{
			"type": "tool_call", "subtype": "completed", "call_id": "call-partial-1",
			"session_id": "cursor-session-partial",
			"tool_call": map[string]any{"readToolCall": map[string]any{}},
		},
		map[string]any{
			"type": "assistant", "timestamp_ms": 2, "session_id": "cursor-session-partial",
			"message": map[string]any{
				"role": "assistant",
				"content": []map[string]any{{"type": "text", "text": " tail"}},
			},
		},
		map[string]any{
			"type": "result", "subtype": "success", "is_error": false,
			"result": "partial final", "session_id": "cursor-session-partial",
		},
	}
	return encodeCursorRecords(records)
}

func encodeCursorRecords(records []any) []byte {
	var stream []byte
	for _, record := range records {
		encoded, _ := json.Marshal(record)
		stream = append(stream, encoded...)
		stream = append(stream, '\n')
	}
	return stream
}
