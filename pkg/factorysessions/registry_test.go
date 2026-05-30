package factorysessions

import "testing"

func TestRegistry_UpsertSelectAndRemove(t *testing.T) {
	registry := NewRegistry()

	defaultSession := NewLiveSession(DefaultSessionID, "/factories/alpha", "/workspace", TargetRef{Kind: TargetKindDefault}, "handle-default", true, "alpha")
	betaSession := NewLiveSession("session-beta", "/factories/beta", "/workspace", TargetRef{Kind: TargetKindNamed, Name: "beta"}, "handle-beta", false, "beta")

	registry.Upsert(defaultSession, true)
	if got := registry.Current(); got != defaultSession {
		t.Fatalf("Current() = %#v, want default session", got)
	}
	if registry.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", registry.Count())
	}

	registry.Upsert(betaSession, false)
	if got := registry.Current(); got != defaultSession {
		t.Fatalf("Current() after non-select upsert = %#v, want default session", got)
	}
	if registry.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", registry.Count())
	}

	if !registry.Select("session-beta") {
		t.Fatal("Select(session-beta) = false, want true")
	}
	if got := registry.Current(); got != betaSession {
		t.Fatalf("Current() after select = %#v, want beta session", got)
	}

	registry.Remove(DefaultSessionID)
	if got := registry.Current(); got != betaSession {
		t.Fatalf("Current() after removing default = %#v, want beta session", got)
	}
	if registry.Get(DefaultSessionID) != nil {
		t.Fatal("removed default session is still registered")
	}

	registry.Remove("session-beta")
	if registry.Current() != nil {
		t.Fatalf("Current() after removing all = %#v, want nil", registry.Current())
	}
	if got := registry.IDs(); len(got) != 0 {
		t.Fatalf("IDs() = %#v, want empty", got)
	}
}

func TestRegistry_SelectUnknownReturnsFalse(t *testing.T) {
	registry := NewRegistry()
	if registry.Select("missing") {
		t.Fatal("Select(missing) = true, want false")
	}
}
