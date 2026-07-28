package codex_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestCodexRootPreservesRequestOrderedStreamFinalAndSession(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:         providers.IDCodex,
		AttemptID:        "attempt-codex-success",
		SystemPrompt:     "system contract",
		UserMessage:      "perform the accepted work",
		OutputSchema:     `{"type":"object"}`,
		WorkingDirectory: "C:/factory",
		Worktree:         "C:/factory/tree",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "session-previous",
		},
	}
	var received providers.ExecuteRequest
	stream := codexSuccessStream()
	effect := codex.EffectFunc(func(
		_ context.Context,
		got providers.ExecuteRequest,
		observe func([]byte) error,
	) (codex.EffectResult, error) {
		received = got.Clone()
		for _, chunk := range splitEvery(stream, 17) {
			if err := observe(chunk); err != nil {
				return codex.EffectResult{}, err
			}
		}
		return codex.EffectResult{
			DurationMillis: 23,
			Metadata:       map[string]string{"transport": "jsonl"},
		}, nil
	})
	root := newCodexRoot(t, effect)

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertCodexSuccessResult(t, result, received, request)

	result.SessionRef.ID = "caller-mutated"
	result.Diagnostics.Progress[2].Metadata["caller"] = "mutated"
	second, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	assertRepeatedCodexResultDetached(t, second)
}

func assertCodexSuccessResult(
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
	if result.Content != "authoritative second answer" {
		t.Fatalf("Content = %q", result.Content)
	}
	if result.SessionRef == nil ||
		result.SessionRef.Provider != providers.IDCodex ||
		result.SessionRef.Kind != providers.SessionIDKind ||
		result.SessionRef.ID != "thread-codex-42" {
		t.Fatalf("SessionRef = %#v", result.SessionRef)
	}
	if result.Diagnostics == nil ||
		result.Diagnostics.DurationMillis != 23 ||
		result.Diagnostics.Metadata["transport"] != "jsonl" {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
	assertProgress(t, result.Diagnostics.Progress)
}

func assertRepeatedCodexResultDetached(
	t *testing.T,
	second providers.ExecuteResult,
) {
	t.Helper()
	if second.SessionRef.ID != "thread-codex-42" {
		t.Fatalf("second SessionRef = %#v", second.SessionRef)
	}
	if _, exists := second.Diagnostics.Progress[2].Metadata["caller"]; exists {
		t.Fatal("second result retained caller progress mutation")
	}
}

func TestCodexDecoderFinalizesUnterminatedRecordOnce(t *testing.T) {
	t.Parallel()

	stream := []byte(
		"{\"type\":\"thread.started\",\"thread_id\":\"thread-partial\"}\n" +
			"{\"type\":\"item.completed\",\"item\":{\"id\":\"message-final\"," +
			"\"type\":\"agent_message\",\"text\":\"unterminated final\"}}",
	)
	effect := codex.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (codex.EffectResult, error) {
		for _, chunk := range splitEvery(stream, 3) {
			if err := observe(chunk); err != nil {
				return codex.EffectResult{}, err
			}
		}
		return codex.EffectResult{}, nil
	})
	result, err := newCodexRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-partial",
	})
	if err != nil || result.Content != "unterminated final" {
		t.Fatalf("Execute() = (%#v, %v)", result, err)
	}
	if got := countPhase(result.Diagnostics.Progress, "message.completed"); got != 1 {
		t.Fatalf("completed message facts = %d, want 1", got)
	}
}

func assertProgress(t *testing.T, progress []providers.ExecuteProgress) {
	t.Helper()
	wantPhases := []string{
		"session.started",
		"turn.started",
		"message.started",
		"message.delta",
		"message.completed",
		"reasoning.completed",
		"plan.completed",
		"file_change.completed",
		"tool.started",
		"tool.updated",
		"tool.completed",
		"tool.completed",
		"usage.updated",
		"turn.completed",
		"message.completed",
	}
	gotPhases := make([]string, len(progress))
	for index := range progress {
		gotPhases[index] = progress[index].Phase
	}
	if !reflect.DeepEqual(gotPhases, wantPhases) {
		t.Fatalf("progress phases = %#v, want %#v", gotPhases, wantPhases)
	}
	for _, index := range []int{2, 3, 4} {
		if progress[index].Metadata["item_id"] != "message-1" {
			t.Fatalf("message correlation[%d] = %#v", index, progress[index])
		}
	}
	for _, index := range []int{8, 9, 10} {
		if progress[index].Metadata["correlation_id"] != "command-1" {
			t.Fatalf("tool correlation[%d] = %#v", index, progress[index])
		}
	}
}

func codexSuccessStream() []byte {
	records := []any{
		map[string]any{"type": "thread.started", "thread_id": "thread-codex-42"},
		map[string]any{"type": "turn.started"},
		itemRecord("item.started", "message-1", "agent_message", map[string]any{"text": "draft"}),
		itemRecord("item.updated", "message-1", "agent_message", map[string]any{"text": "updated"}),
		itemRecord("item.completed", "message-1", "agent_message", map[string]any{"text": "first answer"}),
		itemRecord("item.completed", "reason-1", "reasoning", map[string]any{"text": "safe summary"}),
		itemRecord("item.completed", "plan-1", "todo_list", map[string]any{
			"items": []map[string]any{{"text": "implement", "completed": true}},
		}),
		itemRecord("item.completed", "files-1", "file_change", map[string]any{
			"changes": []map[string]any{{"path": "adapter.go", "kind": "update"}},
		}),
		itemRecord("item.started", "command-1", "command_execution", map[string]any{"command": "go test"}),
		itemRecord("item.updated", "command-1", "command_execution", map[string]any{"aggregated_output": "running"}),
		itemRecord("item.completed", "command-1", "command_execution", map[string]any{"aggregated_output": "passed"}),
		itemRecord("item.completed", "mcp-1", "mcp_tool_call", map[string]any{"server": "docs", "tool": "lookup"}),
		map[string]any{"type": "turn.completed", "usage": map[string]any{
			"input_tokens": 10, "cached_input_tokens": 2,
			"output_tokens": 5, "reasoning_output_tokens": 1,
		}},
		itemRecord("item.completed", "message-2", "agent_message", map[string]any{
			"text": "authoritative second answer",
		}),
	}
	var stream []byte
	for _, record := range records {
		encoded, _ := json.Marshal(record)
		stream = append(stream, encoded...)
		stream = append(stream, '\n')
	}
	return stream
}

func itemRecord(nativeType, id, itemType string, fields map[string]any) map[string]any {
	item := map[string]any{"id": id, "type": itemType}
	for key, value := range fields {
		item[key] = value
	}
	return map[string]any{"type": nativeType, "item": item}
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

func newCodexRoot(t *testing.T, effect codex.Effect) providers.Service {
	t.Helper()
	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatal(err)
	}
	executionService, err := executionwire.NewService(
		catalog,
		codex.NewRegistration(effect),
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
