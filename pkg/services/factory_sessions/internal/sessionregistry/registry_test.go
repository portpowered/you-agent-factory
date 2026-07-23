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
