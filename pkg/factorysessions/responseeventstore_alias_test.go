package factorysessions_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

func TestNewSessionResponseEventStoreAlias(t *testing.T) {
	t.Parallel()

	store := factorysessions.NewSessionResponseEventStore("session-alias")
	if store == nil {
		t.Fatal("NewSessionResponseEventStore returned nil")
	}
	if got := store.FactorySessionID(); got != "session-alias" {
		t.Fatalf("FactorySessionID() = %q, want session-alias", got)
	}
}
