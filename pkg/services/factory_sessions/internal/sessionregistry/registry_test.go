package sessionregistry

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

func TestDefaultSessionUsesDeterministicIdentityOrdering(t *testing.T) {
	t.Parallel()

	registry := New()
	registry.Upsert(&livesession.LiveSession{ID: "session-z", IsDefault: true}, false)
	registry.Upsert(&livesession.LiveSession{ID: "session-a", IsDefault: true}, false)

	for range 20 {
		if got := registry.DefaultSession(); got == nil || got.ID != "session-a" {
			t.Fatalf("DefaultSession() = %#v, want lexically first default session", got)
		}
	}
}

func TestRemoveGenerationPreservesReplacementAndPeerSelection(t *testing.T) {
	t.Parallel()
	registry := New()
	old := &livesession.LiveSession{ID: "a"}
	replacement := &livesession.LiveSession{ID: "a"}
	peer := &livesession.LiveSession{ID: "b"}
	registry.Upsert(old, true)
	registry.Upsert(peer, false)
	registry.Upsert(replacement, true)
	if registry.RemoveGeneration(old) || registry.Get("a") != replacement || registry.Current() != replacement {
		t.Fatal("stale retirement removed or deselected the replacement")
	}
	if !registry.RemoveGeneration(replacement) || registry.Get("a") != nil || registry.Current() != peer {
		t.Fatal("current retirement did not preserve peer selection")
	}
	if registry.RemoveGeneration(replacement) || registry.Get("b") != peer {
		t.Fatal("repeated retirement removed the peer")
	}
}
