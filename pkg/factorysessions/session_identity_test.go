package factorysessions

import "testing"

func TestCanonicalFactorySessionID_PrefersRuntimeIdentityForDefaultAlias(t *testing.T) {
	session := &LiveSession{
		ID:                      DefaultSessionID,
		IsDefault:               true,
		RuntimeFactorySessionID: "550e8400-e29b-41d4-a716-446655440000",
	}

	if got := CanonicalFactorySessionID(session); got != session.RuntimeFactorySessionID {
		t.Fatalf("CanonicalFactorySessionID() = %q, want runtime id %q", got, session.RuntimeFactorySessionID)
	}
}

func TestCanonicalFactorySessionID_FallsBackToRegistryID(t *testing.T) {
	session := &LiveSession{ID: "session-beta"}

	if got := CanonicalFactorySessionID(session); got != "session-beta" {
		t.Fatalf("CanonicalFactorySessionID() = %q, want session-beta", got)
	}
}

func TestEnsureRuntimeFactorySessionID_AssignsUUIDForDefaultAlias(t *testing.T) {
	session := &LiveSession{ID: DefaultSessionID, IsDefault: true}

	EnsureRuntimeFactorySessionID(session)
	if session.RuntimeFactorySessionID == "" {
		t.Fatal("RuntimeFactorySessionID = empty, want UUID")
	}
	if !IsUUIDFactorySessionID(session.RuntimeFactorySessionID) {
		t.Fatalf("RuntimeFactorySessionID = %q, want UUID", session.RuntimeFactorySessionID)
	}
}

func TestEnsureRuntimeFactorySessionID_IsIdempotent(t *testing.T) {
	session := &LiveSession{
		ID:                      DefaultSessionID,
		IsDefault:               true,
		RuntimeFactorySessionID: "550e8400-e29b-41d4-a716-446655440000",
	}

	EnsureRuntimeFactorySessionID(session)
	if session.RuntimeFactorySessionID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("RuntimeFactorySessionID = %q, want preserved UUID", session.RuntimeFactorySessionID)
	}
}
