package bootstrap_portability

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/petri"
)

func assertTokenPayload(t *testing.T, snap *petri.MarkingSnapshot, placeID, want string) {
	t.Helper()

	for _, tok := range snap.Tokens {
		if tok.PlaceID == placeID {
			if got := string(tok.Color.Payload); got != want {
				t.Fatalf("expected payload %q, got %q", want, got)
			}
			return
		}
	}

	t.Fatalf("no token found in %s", placeID)
}
