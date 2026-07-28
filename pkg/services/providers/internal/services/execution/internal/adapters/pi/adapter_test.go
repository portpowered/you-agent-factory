package pi_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	pi "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/pi"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestPiRootPreservesRequestOrderedStreamFinalAndSession(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:         providers.IDPi,
		AttemptID:        "attempt-pi-success",
		SystemPrompt:     "system contract",
		UserMessage:      "perform the accepted work",
		WorkingDirectory: "C:/factory",
		Model:            "anthropic/claude-sonnet-4",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDPi,
			Kind:     providers.SessionIDKind,
			ID:       "pi-session-production",
		},
	}
	var received providers.ExecuteRequest
	stream := piSuccessStream()
	effect := pi.EffectFunc(func(
		_ context.Context,
		got providers.ExecuteRequest,
		observe func([]byte) error,
	) (pi.EffectResult, error) {
		received = got.Clone()
		for _, chunk := range splitEvery(stream, 17) {
			if err := observe(chunk); err != nil {
				return pi.EffectResult{}, err
			}
		}
		return pi.EffectResult{
			DurationMillis: 21,
			Metadata:       map[string]string{"transport": "json"},
			Command:        pi.CommandFacts{Stdout: append([]byte(nil), stream...)},
		}, nil
	})
	root := newPiRoot(t, effect)

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertPiSuccessResult(t, result, received, request)

	result.SessionRef.ID = "caller-mutated"
	if len(result.Diagnostics.Progress) > 2 {
		result.Diagnostics.Progress[2].Metadata["caller"] = "mutated"
	}
	second, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	assertRepeatedPiResultDetached(t, second)
}

func assertPiSuccessResult(
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
	if result.Content != "authoritative answer" {
		t.Fatalf("Content = %q", result.Content)
	}
	if result.SessionRef == nil ||
		result.SessionRef.Provider != providers.IDPi ||
		result.SessionRef.Kind != providers.SessionIDKind ||
		result.SessionRef.ID != "pi-session-production" {
		t.Fatalf("SessionRef = %#v", result.SessionRef)
	}
	if result.Diagnostics == nil ||
		result.Diagnostics.DurationMillis != 21 ||
		result.Diagnostics.Metadata["transport"] != "json" {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
	assertPiProgress(t, result.Diagnostics.Progress)
}

func assertRepeatedPiResultDetached(t *testing.T, second providers.ExecuteResult) {
	t.Helper()
	if second.SessionRef.ID != "pi-session-production" {
		t.Fatalf("second SessionRef = %#v", second.SessionRef)
	}
	if len(second.Diagnostics.Progress) > 2 {
		if _, exists := second.Diagnostics.Progress[2].Metadata["caller"]; exists {
			t.Fatal("second result retained caller progress mutation")
		}
	}
}

func assertPiProgress(t *testing.T, progress []providers.ExecuteProgress) {
	t.Helper()
	wantPhases := []string{
		"session.started",
		"run.started",
		"turn.started",
		"message.started",
		"message.delta",
		"message.completed",
		"turn.completed",
		"run.completed",
	}
	gotPhases := make([]string, len(progress))
	for index := range progress {
		gotPhases[index] = progress[index].Phase
	}
	if len(gotPhases) < len(wantPhases) {
		t.Fatalf("progress phases = %#v, want at least %#v", gotPhases, wantPhases)
	}
	for index, want := range wantPhases {
		if gotPhases[index] != want {
			t.Fatalf("progress[%d] phase = %q, want %q (all=%#v)", index, gotPhases[index], want, gotPhases)
		}
	}
	for _, fact := range progress {
		if fact.Phase == "message.delta" && fact.Metadata["message_id"] != "msg-production" {
			t.Fatalf("message correlation = %#v", fact)
		}
	}
}

func piSuccessStream() []byte {
	records := []any{
		map[string]any{"type": "session", "id": "pi-session-production"},
		map[string]any{"type": "agent_start"},
		map[string]any{"type": "turn_start"},
		map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg-production", "role": "assistant", "content": []any{},
			},
		},
		map[string]any{
			"type": "message_update",
			"message": map[string]any{
				"id": "msg-production", "role": "assistant", "content": []any{},
			},
			"assistantMessageEvent": map[string]any{
				"type": "text_delta", "delta": "authoritative answer", "contentIndex": 0,
			},
		},
		map[string]any{
			"type": "message_end",
			"message": map[string]any{
				"id": "msg-production", "role": "assistant",
				"content": []map[string]any{{"type": "text", "text": "authoritative answer"}},
				"stopReason": "stop",
			},
		},
		map[string]any{"type": "turn_end"},
		map[string]any{"type": "agent_end"},
	}
	var builder strings.Builder
	for _, record := range records {
		encoded, _ := json.Marshal(record)
		builder.Write(encoded)
		builder.WriteByte('\n')
	}
	return []byte(builder.String())
}

func splitEvery(payload []byte, size int) [][]byte {
	if size <= 0 || len(payload) == 0 {
		return [][]byte{payload}
	}
	chunks := make([][]byte, 0, (len(payload)+size-1)/size)
	for start := 0; start < len(payload); start += size {
		end := start + size
		if end > len(payload) {
			end = len(payload)
		}
		chunks = append(chunks, payload[start:end])
	}
	return chunks
}

func newPiRoot(t *testing.T, effect pi.Effect) providers.Service {
	t.Helper()
	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() error = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalog,
		pi.NewRegistration(effect),
	)
	if err != nil {
		t.Fatalf("executionwire.NewService() error = %v", err)
	}
	root, err := providerservice.New(catalog, executionService)
	if err != nil {
		t.Fatalf("providerservice.New() error = %v", err)
	}
	return root
}
