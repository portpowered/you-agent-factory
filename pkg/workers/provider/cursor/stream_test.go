package cursor

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestStreamParser_EmitsKnownFragmentsInOrder(t *testing.T) {
	var fragments []StreamFragment
	parser := NewStreamParser(string(interfaces.ModelProviderCursor), func(fragment StreamFragment) {
		fragments = append(fragments, fragment)
	})

	parser.Consume([]byte(
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor-session-123\"}\n" +
			"{\"type\":\"assistant\",\"timestamp_ms\":1,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Plan \"}]},\"session_id\":\"cursor-session-123\"}\n" +
			"{\"type\":\"assistant\",\"timestamp_ms\":2,\"model_call_id\":\"call-1\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Plan \"}]},\"session_id\":\"cursor-session-123\"}\n" +
			"{\"type\":\"tool_call\",\"subtype\":\"started\",\"call_id\":\"call-1\",\"tool_call\":{\"readToolCall\":{\"args\":{\"path\":\"README.md\"}}},\"session_id\":\"cursor-session-123\"}\n",
	))
	parser.Consume([]byte(
		"{\"type\":\"tool_call\",\"subtype\":\"completed\",\"call_id\":\"call-1\",\"tool_call\":{\"readToolCall\":{\"result\":{\"success\":{}}}},\"session_id\":\"cursor-session-123\"}\n" +
			"{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Plan \"}]},\"session_id\":\"cursor-session-123\"}\n" +
			"{\"type\":\"assistant\",\"timestamp_ms\":3,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"done\"}]},\"session_id\":\"cursor-session-123\"}",
	))
	parser.Flush()

	if len(fragments) != 5 {
		t.Fatalf("fragment count = %d, want 5", len(fragments))
	}
	assertStreamFragment(t, fragments[0], StreamFragmentKindProgress, "Cursor session initialized", "cursor-session-123")
	assertStreamFragment(t, fragments[1], StreamFragmentKindResponse, "Plan ", "cursor-session-123")
	assertStreamFragment(t, fragments[2], StreamFragmentKindProgress, "Cursor readToolCall started", "cursor-session-123")
	assertStreamFragment(t, fragments[3], StreamFragmentKindProgress, "Cursor readToolCall completed", "cursor-session-123")
	assertStreamFragment(t, fragments[4], StreamFragmentKindResponse, "done", "cursor-session-123")
}

func TestStreamParser_EmitsTrailingAssistantDeltaOnFlush(t *testing.T) {
	var fragments []StreamFragment
	parser := NewStreamParser(string(interfaces.ModelProviderCursor), func(fragment StreamFragment) {
		fragments = append(fragments, fragment)
	})

	parser.Consume([]byte(`{"type":"assistant","timestamp_ms":7,"message":{"role":"assistant","content":[{"type":"text","text":"tail"}]},"session_id":"cursor-session-456"}`))
	parser.Flush()

	if len(fragments) != 1 {
		t.Fatalf("fragment count = %d, want 1", len(fragments))
	}
	assertStreamFragment(t, fragments[0], StreamFragmentKindResponse, "tail", "cursor-session-456")
}

func assertStreamFragment(t *testing.T, fragment StreamFragment, wantKind StreamFragmentKind, wantPayload string, wantSessionID string) {
	t.Helper()
	if fragment.Kind != wantKind {
		t.Fatalf("fragment kind = %q, want %q", fragment.Kind, wantKind)
	}
	if fragment.Payload != wantPayload {
		t.Fatalf("fragment payload = %q, want %q", fragment.Payload, wantPayload)
	}
	if fragment.ProviderSession == nil {
		t.Fatal("provider session = nil, want canonical cursor session")
	}
	if fragment.ProviderSession.Provider != "cursor" || fragment.ProviderSession.Kind != ProviderSessionKindSessionID || fragment.ProviderSession.ID != wantSessionID {
		t.Fatalf("provider session = %#v, want cursor/session_id/%s", fragment.ProviderSession, wantSessionID)
	}
}
