package cursor_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	cursor "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/cursor"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestCursorRootPreservesRequestOrderedStreamFinalAndSession(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:         providers.IDCursor,
		AttemptID:        "attempt-cursor-success",
		SystemPrompt:     "system contract",
		UserMessage:      "perform the accepted work",
		WorkingDirectory: "C:/factory",
		Worktree:         "C:/factory/tree",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       "session-previous",
		},
	}
	var received providers.ExecuteRequest
	stream := cursorSuccessStream()
	effect := cursor.EffectFunc(func(
		_ context.Context,
		got providers.ExecuteRequest,
		observe func([]byte) error,
	) (cursor.EffectResult, error) {
		received = got.Clone()
		for _, chunk := range splitEvery(stream, 17) {
			if err := observe(chunk); err != nil {
				return cursor.EffectResult{}, err
			}
		}
		return cursor.EffectResult{
			DurationMillis: 21,
			Metadata:       map[string]string{"transport": "stream-json"},
		}, nil
	})
	root := newCursorRoot(t, effect)

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertCursorSuccessResult(t, result, received, request)

	result.SessionRef.ID = "caller-mutated"
	result.Diagnostics.Progress[2].Metadata["caller"] = "mutated"
	second, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	assertRepeatedCursorResultDetached(t, second)
}

func TestCursorRootResolvesCursorAliasThroughCatalog(t *testing.T) {
	t.Parallel()

	stream := cursorSuccessStream()
	effect := cursor.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (cursor.EffectResult, error) {
		return cursor.EffectResult{}, observe(stream)
	})
	root := newCursorRoot(t, effect)

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:  providers.ID("cursor"),
		AttemptID: "attempt-cursor-alias",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.SessionRef == nil || result.SessionRef.ID != "cursor-session-42" {
		t.Fatalf("SessionRef = %#v", result.SessionRef)
	}
}

func assertCursorSuccessResult(
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
		result.SessionRef.Provider != providers.IDCursor ||
		result.SessionRef.Kind != providers.SessionIDKind ||
		result.SessionRef.ID != "cursor-session-42" {
		t.Fatalf("SessionRef = %#v", result.SessionRef)
	}
	if result.Diagnostics == nil ||
		result.Diagnostics.DurationMillis != 21 ||
		result.Diagnostics.Metadata["transport"] != "stream-json" {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
	assertProgress(t, result.Diagnostics.Progress)
}

func assertRepeatedCursorResultDetached(
	t *testing.T,
	second providers.ExecuteResult,
) {
	t.Helper()
	if second.SessionRef.ID != "cursor-session-42" {
		t.Fatalf("second SessionRef = %#v", second.SessionRef)
	}
	if _, exists := second.Diagnostics.Progress[2].Metadata["caller"]; exists {
		t.Fatal("second result retained caller progress mutation")
	}
}

func assertProgress(t *testing.T, progress []providers.ExecuteProgress) {
	t.Helper()
	wantPhases := []string{
		"session.started",
		"message.delta",
		"message.delta",
		"tool.started",
		"tool.completed",
		"message.delta",
	}
	gotPhases := make([]string, len(progress))
	for index := range progress {
		gotPhases[index] = progress[index].Phase
	}
	if !reflect.DeepEqual(gotPhases, wantPhases) {
		t.Fatalf("progress phases = %#v, want %#v", gotPhases, wantPhases)
	}
	for _, index := range []int{1, 2, 5} {
		if progress[index].Metadata["message_id"] != "cursor-session-42" {
			t.Fatalf("message correlation[%d] = %#v", index, progress[index])
		}
	}
	for _, index := range []int{3, 4} {
		if progress[index].Metadata["correlation_id"] != "call-read-1" {
			t.Fatalf("tool correlation[%d] = %#v", index, progress[index])
		}
	}
}

func cursorSuccessStream() []byte {
	records := []any{
		map[string]any{"type": "system", "subtype": "init", "session_id": "cursor-session-42"},
		map[string]any{
			"type": "assistant", "timestamp_ms": 1, "session_id": "cursor-session-42",
			"message": map[string]any{
				"role": "assistant",
				"content": []map[string]any{{"type": "text", "text": "draft "}},
			},
		},
		map[string]any{
			"type": "assistant", "timestamp_ms": 2, "session_id": "cursor-session-42",
			"message": map[string]any{
				"role": "assistant",
				"content": []map[string]any{{"type": "text", "text": "updated"}},
			},
		},
		map[string]any{
			"type": "tool_call", "subtype": "started", "call_id": "call-read-1",
			"session_id": "cursor-session-42",
			"tool_call": map[string]any{"readToolCall": map[string]any{"args": map[string]any{"path": "README.md"}}},
		},
		map[string]any{
			"type": "tool_call", "subtype": "completed", "call_id": "call-read-1",
			"session_id": "cursor-session-42",
			"tool_call": map[string]any{"readToolCall": map[string]any{"result": map[string]any{"success": map[string]any{}}}},
		},
		map[string]any{
			"type": "assistant", "timestamp_ms": 3, "session_id": "cursor-session-42",
			"message": map[string]any{
				"role": "assistant",
				"content": []map[string]any{{"type": "text", "text": " final"}},
			},
		},
		map[string]any{
			"type": "result", "subtype": "success", "is_error": false,
			"result": "authoritative final answer", "session_id": "cursor-session-42",
		},
	}
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

func newCursorRoot(t *testing.T, effect cursor.Effect) providers.Service {
	t.Helper()
	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatal(err)
	}
	executionService, err := executionwire.NewService(
		catalog,
		cursor.NewRegistration(effect),
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
