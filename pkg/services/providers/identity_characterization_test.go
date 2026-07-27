package providers_test

import (
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestIdentityContract_Characterization_RoundTripsDescriptorAndSessionRef(t *testing.T) {
	t.Parallel()

	original := providers.Descriptor{
		ID:          providers.IDCodex,
		Aliases:     []string{"openai-codex", "codex-cli"},
		DisplayName: "Codex",
	}
	if err := original.ID.Validate(); err != nil {
		t.Fatalf("ID.Validate() = %v", err)
	}

	roundTripped := original.Clone()
	if roundTripped.ID != original.ID ||
		roundTripped.DisplayName != original.DisplayName ||
		len(roundTripped.Aliases) != len(original.Aliases) {
		t.Fatalf("descriptor round trip = %#v, want %#v", roundTripped, original)
	}
	for i := range original.Aliases {
		if roundTripped.Aliases[i] != original.Aliases[i] {
			t.Fatalf("alias[%d] = %q, want %q", i, roundTripped.Aliases[i], original.Aliases[i])
		}
	}
	if &roundTripped.Aliases[0] == &original.Aliases[0] {
		t.Fatal("descriptor clone shares alias backing array")
	}

	session := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "session-root-1",
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("SessionRef.Validate() = %v", err)
	}

	clonedSession := session.Clone()
	if clonedSession != session {
		t.Fatalf("session ref round trip = %#v, want %#v", clonedSession, session)
	}
}
