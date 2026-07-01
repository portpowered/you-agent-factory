package factorysessions

import "testing"

func TestIsDefaultSessionSelector(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		want      bool
	}{
		{name: "alias", sessionID: DefaultSessionID, want: true},
		{name: "blank", sessionID: "   ", want: true},
		{name: "uuid", sessionID: "f47ac10b-58cc-4372-a567-0e02b2c3d479", want: false},
		{name: "named", sessionID: "session-beta", want: false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDefaultSessionSelector(tc.sessionID); got != tc.want {
				t.Fatalf("IsDefaultSessionSelector(%q) = %t, want %t", tc.sessionID, got, tc.want)
			}
		})
	}
}

func TestRegistry_DefaultSessionAndAliasLookup(t *testing.T) {
	registry := NewRegistry()
	defaultID := NewSessionID()
	betaID := NewSessionID()
	registry.Upsert(NewLiveSession(
		defaultID,
		"/factories/alpha",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		"handle-default",
		true,
		"alpha",
	), true)
	registry.Upsert(NewLiveSession(
		betaID,
		"/factories/beta",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindNamed, Name: "beta"},
		"handle-beta",
		false,
		"beta",
	), false)

	if got := registry.DefaultSession(); got == nil || got.ID != defaultID {
		t.Fatalf("DefaultSession() = %#v, want id %q", got, defaultID)
	}
	if got := registry.Get(DefaultSessionID); got == nil || got.ID != defaultID {
		t.Fatalf("Get(~default) = %#v, want id %q", got, defaultID)
	}
	if got := registry.Get(""); got == nil || got.ID != defaultID {
		t.Fatalf("Get(blank) = %#v, want id %q", got, defaultID)
	}
	if got := registry.Get(betaID); got == nil || got.ID != betaID {
		t.Fatalf("Get(beta) = %#v, want id %q", got, betaID)
	}

	registry.Remove(DefaultSessionID)
	if registry.Get(defaultID) != nil {
		t.Fatal("removed default session is still registered by uuid")
	}
	if registry.DefaultSession() != nil {
		t.Fatal("DefaultSession() after remove = non-nil, want nil")
	}
}
