package sessionregistry

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

func TestRegistry_ConcurrentUpsertSelectAndRemove(t *testing.T) {
	t.Parallel()

	registry := New()
	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers * 4)

	for index := range workers {
		sessionID := fmt.Sprintf("session-%02d", index)
		session := &livesession.LiveSession{ID: sessionID}
		go func() {
			defer wg.Done()
			for range 50 {
				registry.Upsert(session, index%2 == 0)
			}
		}()
		go func() {
			defer wg.Done()
			for range 50 {
				registry.Select(sessionID)
			}
		}()
		go func() {
			defer wg.Done()
			for range 20 {
				registry.Remove(sessionID)
				registry.Upsert(session, false)
			}
		}()
	}

	for range workers {
		go func() {
			defer wg.Done()
			for range 100 {
				_ = registry.Get("session-01")
				_ = registry.IDs()
				_ = registry.Count()
				_ = registry.Current()
			}
		}()
	}

	wg.Wait()
	if registry.Count() == 0 {
		t.Fatal("registry lost every session after concurrent mutation")
	}
	current := registry.Current()
	if current == nil {
		t.Fatal("registry current session is nil after concurrent select")
	}
	if registry.Get(current.ID) == nil {
		t.Fatalf("current session %q is not registered", current.ID)
	}
}

func TestRegistry_ConcurrentDefaultSessionLookupRemainsDeterministic(t *testing.T) {
	t.Parallel()

	registry := New()
	registry.Upsert(&livesession.LiveSession{ID: "session-z", IsDefault: true}, true)
	registry.Upsert(&livesession.LiveSession{ID: "session-a", IsDefault: true}, false)

	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				if got := registry.DefaultSession(); got == nil || got.ID != "session-a" {
					t.Errorf("DefaultSession() = %#v, want lexically first default", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestRegistry_ConcurrentRemoveDoesNotDoubleCount(t *testing.T) {
	t.Parallel()

	registry := New()
	session := &livesession.LiveSession{ID: "session-close"}
	registry.Upsert(session, true)

	var removed atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.Remove(session.ID)
			removed.Add(1)
		}()
	}
	wg.Wait()

	if registry.Count() != 0 {
		t.Fatalf("Count() = %d after concurrent remove, want 0", registry.Count())
	}
	if registry.Current() != nil {
		t.Fatal("current session remains after concurrent remove")
	}
	if removed.Load() != 16 {
		t.Fatalf("remove attempts = %d, want 16", removed.Load())
	}
}
