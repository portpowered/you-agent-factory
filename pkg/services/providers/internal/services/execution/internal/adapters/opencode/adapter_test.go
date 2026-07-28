package opencode_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	opencode "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/opencode"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestOpenCodeRootPreservesRequestOrderedStreamFinalAndSession(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:         providers.IDOpenCode,
		AttemptID:        "attempt-opencode-success",
		SystemPrompt:     "system contract",
		UserMessage:      "perform the accepted work",
		Model:            "openai/gpt-5",
		WorkingDirectory: "/workspace",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDOpenCode,
			Kind:     providers.SessionIDKind,
			ID:       "ses_open_42",
		},
	}
	var received providers.ExecuteRequest
	stream := openCodeStructuredSuccessStream()
	effect := opencode.EffectFunc(func(
		_ context.Context,
		got providers.ExecuteRequest,
		observe func([]byte) error,
	) (opencode.EffectResult, error) {
		received = got.Clone()
		for _, chunk := range splitEvery(stream, 19) {
			if err := observe(chunk); err != nil {
				return opencode.EffectResult{}, err
			}
		}
		return opencode.EffectResult{
			DurationMillis: 18,
			Metadata:       map[string]string{"transport": "structured-jsonl"},
		}, nil
	})
	root := newOpenCodeRoot(t, effect, opencode.ModeStructured)

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertOpenCodeStructuredSuccessResult(t, result, received, request)

	result.SessionRef.ID = "caller-mutated"
	result.Diagnostics.Progress[2].Metadata["caller"] = "mutated"
	second, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	assertRepeatedOpenCodeResultDetached(t, second)
}

func TestOpenCodeRootFinalOnlyPreservesDetachedSessionAndContent(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:    providers.IDOpenCode,
		AttemptID:   "attempt-opencode-final-only",
		UserMessage: "summarize the work",
	}
	effect := opencode.EffectFunc(func(
		_ context.Context,
		got providers.ExecuteRequest,
		observe func([]byte) error,
	) (opencode.EffectResult, error) {
		if got.Provider != providers.IDOpenCode {
			t.Fatalf("native provider = %q", got.Provider)
		}
		return opencode.EffectResult{
			DurationMillis: 9,
		}, observe([]byte("authoritative final-only answer\n"))
	})
	root := newOpenCodeRoot(t, effect, opencode.ModeFinalOnly)

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "authoritative final-only answer" {
		t.Fatalf("Content = %q", result.Content)
	}
	if result.SessionRef != nil {
		t.Fatalf("SessionRef = %#v, want sessionless final-only success", result.SessionRef)
	}
	wantPhases := []string{"run.started", "message.completed", "run.completed"}
	gotPhases := make([]string, len(result.Diagnostics.Progress))
	for index := range result.Diagnostics.Progress {
		gotPhases[index] = result.Diagnostics.Progress[index].Phase
	}
	if !reflect.DeepEqual(gotPhases, wantPhases) {
		t.Fatalf("progress phases = %#v, want %#v", gotPhases, wantPhases)
	}
}

func assertOpenCodeStructuredSuccessResult(
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
	if result.Content != "Hello world" {
		t.Fatalf("Content = %q", result.Content)
	}
	if result.SessionRef == nil ||
		result.SessionRef.Provider != providers.IDOpenCode ||
		result.SessionRef.Kind != providers.SessionIDKind ||
		result.SessionRef.ID != "session-42" {
		t.Fatalf("SessionRef = %#v", result.SessionRef)
	}
	if result.Diagnostics == nil ||
		result.Diagnostics.DurationMillis != 18 ||
		result.Diagnostics.Metadata["transport"] != "structured-jsonl" {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
	assertStructuredProgress(t, result.Diagnostics.Progress)
}

func assertRepeatedOpenCodeResultDetached(
	t *testing.T,
	second providers.ExecuteResult,
) {
	t.Helper()
	if second.SessionRef.ID != "session-42" {
		t.Fatalf("second SessionRef = %#v", second.SessionRef)
	}
	if _, exists := second.Diagnostics.Progress[2].Metadata["caller"]; exists {
		t.Fatal("second result retained caller progress mutation")
	}
}

func assertStructuredProgress(t *testing.T, progress []providers.ExecuteProgress) {
	t.Helper()
	wantPhases := []string{
		"run.started",
		"message.completed",
		"tool.started",
		"tool.completed",
		"run.completed",
	}
	gotPhases := make([]string, len(progress))
	for index := range progress {
		gotPhases[index] = progress[index].Phase
	}
	if !reflect.DeepEqual(gotPhases, wantPhases) {
		t.Fatalf("progress phases = %#v, want %#v", gotPhases, wantPhases)
	}
	if progress[1].Metadata["message_id"] != "message-7" {
		t.Fatalf("message correlation = %#v", progress[1])
	}
	if progress[2].Metadata["correlation_id"] != "call-9" {
		t.Fatalf("tool correlation = %#v", progress[2])
	}
	if progress[3].Metadata["correlation_id"] != "call-9" {
		t.Fatalf("tool completed correlation = %#v", progress[3])
	}
}

func openCodeStructuredSuccessStream() []byte {
	records := []any{
		map[string]any{"type": "step_start", "sessionID": "session-42"},
		map[string]any{
			"type": "text", "sessionID": "session-42",
			"part": map[string]any{
				"id": "message-7", "text": "Hello world", "time": map[string]any{"end": 1},
			},
		},
		map[string]any{
			"type": "tool_use", "sessionID": "session-42",
			"part": map[string]any{
				"id": "tool-item-9", "callID": "call-9", "tool": "weather",
				"state": map[string]any{"status": "completed"},
			},
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

func newOpenCodeRoot(t *testing.T, effect opencode.Effect, mode opencode.Mode) providers.Service {
	t.Helper()
	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatal(err)
	}
	executionService, err := executionwire.NewService(
		catalog,
		opencode.NewRegistrationWithMode(effect, mode),
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
