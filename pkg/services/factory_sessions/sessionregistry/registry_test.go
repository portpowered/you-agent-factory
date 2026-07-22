package sessionregistry

import (
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestDefaultSessionUsesDeterministicIdentityOrdering(t *testing.T) {
	t.Parallel()

	registry := New()
	registry.Upsert(&factorysessions.LiveSession{ID: "session-z", IsDefault: true}, false)
	registry.Upsert(&factorysessions.LiveSession{ID: "session-a", IsDefault: true}, false)

	for range 20 {
		if got := registry.DefaultSession(); got == nil || got.ID != "session-a" {
			t.Fatalf("DefaultSession() = %#v, want lexically first default session", got)
		}
	}
}
