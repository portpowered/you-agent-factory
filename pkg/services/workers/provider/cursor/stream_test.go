package cursor

import (
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestStreamParser_EmitsKnownFragmentsInOrder(t *testing.T) {
	var fragments []StreamFragment
	parser := NewStreamParser(string(modelprovider.ProviderCursor), func(fragment StreamFragment) {
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
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"Plan done\",\"session_id\":\"cursor-session-123\"}",
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
	parser := NewStreamParser(string(modelprovider.ProviderCursor), func(fragment StreamFragment) {
		fragments = append(fragments, fragment)
	})

	parser.Consume([]byte(`{"type":"assistant","timestamp_ms":7,"message":{"role":"assistant","content":[{"type":"text","text":"tail"}]},"session_id":"cursor-session-456"}`))
	parser.Flush()

	if len(fragments) != 1 {
		t.Fatalf("fragment count = %d, want 1", len(fragments))
	}
	assertStreamFragment(t, fragments[0], StreamFragmentKindResponse, "tail", "cursor-session-456")
}

func TestStreamParser_BoundsLargeAssistantDeltaWithoutTrimmingSpacing(t *testing.T) {
	var fragments []StreamFragment
	parser := NewStreamParser(string(modelprovider.ProviderCursor), func(fragment StreamFragment) {
		fragments = append(fragments, fragment)
	})

	text := " " + strings.Repeat("a", PublishedTextLimit+5)
	parser.Consume([]byte(`{"type":"assistant","timestamp_ms":7,"message":{"role":"assistant","content":[{"type":"text","text":"` + text + `"}]},"session_id":"cursor-session-456"}`))
	parser.Flush()

	if len(fragments) != 1 {
		t.Fatalf("fragment count = %d, want 1", len(fragments))
	}
	if got := fragments[0].Payload; len(got) != PublishedTextLimit+3 {
		t.Fatalf("payload len = %d, want %d with ellipsis", len(got), PublishedTextLimit+3)
	}
	if !strings.HasPrefix(fragments[0].Payload, " ") {
		t.Fatalf("payload = %q, want preserved leading spacing", fragments[0].Payload)
	}
	if !strings.HasSuffix(fragments[0].Payload, "...") {
		t.Fatalf("payload = %q, want truncated suffix", fragments[0].Payload)
	}
}

func TestStreamParser_OmitsProviderSessionForInvalidSessionID(t *testing.T) {
	var fragments []StreamFragment
	parser := NewStreamParser(string(modelprovider.ProviderCursor), func(fragment StreamFragment) {
		fragments = append(fragments, fragment)
	})

	parser.Consume([]byte(`{"type":"assistant","timestamp_ms":7,"message":{"role":"assistant","content":[{"type":"text","text":"tail"}]},"session_id":"../cursor-session-456"}`))
	parser.Flush()

	if len(fragments) != 1 {
		t.Fatalf("fragment count = %d, want 1", len(fragments))
	}
	if fragments[0].ProviderSession != nil {
		t.Fatalf("provider session = %#v, want nil for invalid session_id", fragments[0].ProviderSession)
	}
}

func TestStreamParser_EmitsBoundedDiagnosticsForMalformedAndUnknownRecordsWithoutBlockingLaterEvents(t *testing.T) {
	var fragments []StreamFragment
	parser := NewStreamParser(string(modelprovider.ProviderCursor), func(fragment StreamFragment) {
		fragments = append(fragments, fragment)
	})

	parser.Consume([]byte(
		"{not json}\n" +
			"{\"type\":\"mystery_event_name_that_is_longer_than_the_preview_limit_and_should_be_truncated\",\"session_id\":\"cursor-session-789\"}\n" +
			"{\"type\":\"assistant\",\"timestamp_ms\":9,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"tail\"}]},\"session_id\":\"cursor-session-789\"}\n",
	))
	parser.Flush()

	if len(fragments) != 3 {
		t.Fatalf("fragment count = %d, want 3", len(fragments))
	}
	assertStreamFragmentPayloadOnly(t, fragments[0], StreamFragmentKindProgress, "Cursor stream ignored malformed JSON record")
	if fragments[1].Kind != StreamFragmentKindProgress {
		t.Fatalf("fragment kind = %q, want %q", fragments[1].Kind, StreamFragmentKindProgress)
	}
	if !strings.HasPrefix(fragments[1].Payload, "Cursor stream ignored unknown event type \"mystery_event_") {
		t.Fatalf("fragment payload = %q, want bounded unknown event diagnostic", fragments[1].Payload)
	}
	if !strings.HasSuffix(fragments[1].Payload, "...\"") {
		t.Fatalf("fragment payload = %q, want truncated suffix", fragments[1].Payload)
	}
	if fragments[1].ProviderSession != nil {
		t.Fatalf("provider session = %#v, want nil", fragments[1].ProviderSession)
	}
	assertStreamFragment(t, fragments[2], StreamFragmentKindResponse, "tail", "cursor-session-789")
}

func TestStreamParser_EmitsResultSubtypeDiagnosticsForFailureAndCancel(t *testing.T) {
	var fragments []StreamFragment
	parser := NewStreamParser(string(modelprovider.ProviderCursor), func(fragment StreamFragment) {
		fragments = append(fragments, fragment)
	})

	parser.Consume([]byte(
		"{\"type\":\"result\",\"subtype\":\"error\",\"is_error\":true,\"result\":\"provider unavailable\",\"session_id\":\"cursor-session-321\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"canceled\",\"is_error\":true,\"result\":\"user canceled request\",\"session_id\":\"cursor-session-654\"}\n",
	))
	parser.Flush()

	if len(fragments) != 2 {
		t.Fatalf("fragment count = %d, want 2", len(fragments))
	}
	assertStreamFragment(t, fragments[0], StreamFragmentKindProgress, "Cursor result error: provider unavailable", "cursor-session-321")
	assertStreamFragment(t, fragments[1], StreamFragmentKindProgress, "Cursor result canceled: user canceled request", "cursor-session-654")
}

func TestStreamParser_DoesNotEmitErrorFlaggedSuccessResultAsResponse(t *testing.T) {
	var fragments []StreamFragment
	parser := NewStreamParser(string(modelprovider.ProviderCursor), func(fragment StreamFragment) {
		fragments = append(fragments, fragment)
	})

	parser.Consume([]byte(
		"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":true,\"result\":\"Request timed out\",\"session_id\":\"cursor-session-error\"}\n",
	))
	parser.Flush()

	if len(fragments) != 1 {
		t.Fatalf("fragments = %#v, want one failure diagnostic", fragments)
	}
	assertStreamFragment(t, fragments[0], StreamFragmentKindProgress, "Cursor result success: Request timed out", "cursor-session-error")
}

func TestStreamParser_EmitsCompletionDiagnosticWhenResultDoesNotExtendEarlierDelta(t *testing.T) {
	var fragments []StreamFragment
	parser := NewStreamParser(string(modelprovider.ProviderCursor), func(fragment StreamFragment) {
		fragments = append(fragments, fragment)
	})

	parser.Consume([]byte(
		"{\"type\":\"assistant\",\"timestamp_ms\":1,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"tail\"}]},\"session_id\":\"cursor-session-999\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"done\",\"session_id\":\"cursor-session-999\"}\n",
	))
	parser.Flush()

	if len(fragments) != 2 {
		t.Fatalf("fragment count = %d, want 2", len(fragments))
	}
	assertStreamFragment(t, fragments[0], StreamFragmentKindResponse, "tail", "cursor-session-999")
	assertStreamFragment(t, fragments[1], StreamFragmentKindProgress, "Cursor result completed", "cursor-session-999")
}

func TestUnknownStreamEventMessage_BoundsEventTypePreview(t *testing.T) {
	got := unknownStreamEventMessage("mystery_event_name_that_is_longer_than_the_preview_limit_and_should_be_truncated")
	if !strings.HasPrefix(got, "Cursor stream ignored unknown event type \"mystery_event_") {
		t.Fatalf("unknownStreamEventMessage() = %q, want bounded unknown event diagnostic", got)
	}
	if !strings.HasSuffix(got, "...\"") {
		t.Fatalf("unknownStreamEventMessage() = %q, want truncated suffix", got)
	}
}

func TestStreamParser_BoundsToolCallNameInProgressDiagnostics(t *testing.T) {
	var fragments []StreamFragment
	parser := NewStreamParser(string(modelprovider.ProviderCursor), func(fragment StreamFragment) {
		fragments = append(fragments, fragment)
	})

	toolName := strings.Repeat("x", PublishedDiagnosticLimit+10)
	parser.Consume([]byte(`{"type":"tool_call","subtype":"started","tool_call":{"function":{"name":"` + toolName + `"}},"session_id":"cursor-session-123"}`))
	parser.Flush()

	if len(fragments) != 1 {
		t.Fatalf("fragment count = %d, want 1", len(fragments))
	}
	if !strings.Contains(fragments[0].Payload, "... started") {
		t.Fatalf("payload = %q, want truncated tool name diagnostic", fragments[0].Payload)
	}
	if len(fragments[0].Payload) > len("Cursor ")+PublishedDiagnosticLimit+len(" started")+3 {
		t.Fatalf("payload len = %d, want bounded diagnostic", len(fragments[0].Payload))
	}
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

func assertStreamFragmentPayloadOnly(t *testing.T, fragment StreamFragment, wantKind StreamFragmentKind, wantPayload string) {
	t.Helper()
	if fragment.Kind != wantKind {
		t.Fatalf("fragment kind = %q, want %q", fragment.Kind, wantKind)
	}
	if fragment.Payload != wantPayload {
		t.Fatalf("fragment payload = %q, want %q", fragment.Payload, wantPayload)
	}
	if fragment.ProviderSession != nil {
		t.Fatalf("provider session = %#v, want nil", fragment.ProviderSession)
	}
}
