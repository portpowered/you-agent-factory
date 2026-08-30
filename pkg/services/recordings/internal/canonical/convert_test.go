package canonical

import (
	"encoding/json"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestFactoryEventFromCanonicalPreservesCanonicalTerminalIdentity(t *testing.T) {
	t.Parallel()

	canonicalID := "550e8400-e29b-41d4-a716-446655440000"
	publicID := "~default"
	contextPayload, err := json.Marshal(factorydefinitions.FactoryEventContext{
		SessionID: &canonicalID,
		EventTime: time.Unix(1_700_000_000, 0).UTC(),
		Sequence:  4,
	})
	if err != nil {
		t.Fatalf("marshal terminal source context: %v", err)
	}

	got := FactoryEventFromCanonical(recordings.CanonicalEvent{
		ID:            "factory-event/session-completed",
		Kind:          recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeSessionCompleted),
		Scope:         recordings.CanonicalEventScope{FactorySessionID: publicID},
		RecordedAt:    time.Unix(1_700_000_000, 0).UTC(),
		SourceContext: string(contextPayload),
	})
	if got.Context.SessionID == nil || *got.Context.SessionID != canonicalID {
		t.Fatalf("terminal session ID = %#v, want canonical %q", got.Context.SessionID, canonicalID)
	}
}

func TestFactoryEventFromCanonicalKeepsDetachedScopeForNonTerminalEvent(t *testing.T) {
	t.Parallel()

	canonicalID := "550e8400-e29b-41d4-a716-446655440000"
	publicID := "~default"
	contextPayload, err := json.Marshal(factorydefinitions.FactoryEventContext{SessionID: &canonicalID})
	if err != nil {
		t.Fatalf("marshal source context: %v", err)
	}

	got := FactoryEventFromCanonical(recordings.CanonicalEvent{
		ID:            "factory-event/work-request",
		Kind:          recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Scope:         recordings.CanonicalEventScope{FactorySessionID: publicID},
		SourceContext: string(contextPayload),
	})
	if got.Context.SessionID == nil || *got.Context.SessionID != publicID {
		t.Fatalf("non-terminal session ID = %#v, want detached scope %q", got.Context.SessionID, publicID)
	}
}
