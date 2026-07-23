package factorysessions_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	. "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
)

var registryResponseEventIdentity atomic.Uint64

func registryResponseEventID() string {
	return fmt.Sprintf("registry-response-event-%d", registryResponseEventIdentity.Add(1))
}

func registrySessionID() string { return "550e8400-e29b-41d4-a716-446655440000" }

func TestRegistry_UpsertSelectAndRemove(t *testing.T) {
	registry := sessionregistry.New()

	defaultSession := livesession.New(DefaultSessionID, "/factories/alpha", "/workspace", "/workspace", TargetRef{Kind: TargetKindDefault}, "handle-default", true, "alpha", platformclock.Real{}, registrySessionID, registryResponseEventID)
	betaSession := livesession.New("session-beta", "/factories/beta", "/workspace", "/workspace", TargetRef{Kind: TargetKindNamed, Name: "beta"}, "handle-beta", false, "beta", platformclock.Real{}, registrySessionID, registryResponseEventID)

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
	registry := sessionregistry.New()
	if registry.Select("missing") {
		t.Fatal("Select(missing) = true, want false")
	}
}

// The following compatibility-only cases prove retained selector acceptance.
// Canonical live identity is covered by the owner-private live-session tests.
func TestCompatibilityOnlyIsDefaultSessionSelector(t *testing.T) {
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
			if got := logicaltarget.IsLiveSessionDefaultSelector(tc.sessionID); got != tc.want {
				t.Fatalf("IsDefaultSessionSelector(%q) = %t, want %t", tc.sessionID, got, tc.want)
			}
		})
	}
}

func TestRegistry_CompatibilityOnlyDefaultSessionAliasLookupAndRemoval(t *testing.T) {
	registry := sessionregistry.New()
	defaultID := "550e8400-e29b-41d4-a716-446655440001"
	betaID := "550e8400-e29b-41d4-a716-446655440002"
	registry.Upsert(livesession.New(
		defaultID,
		"/factories/alpha",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		"handle-default",
		true,
		"alpha",
		platformclock.Real{},
		registrySessionID,
		registryResponseEventID,
	), true)
	registry.Upsert(livesession.New(
		betaID,
		"/factories/beta",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindNamed, Name: "beta"},
		"handle-beta",
		false,
		"beta",
		platformclock.Real{},
		registrySessionID,
		registryResponseEventID,
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

func TestLogicalSessionKeyID_DefaultTargetUsesStableKey(t *testing.T) {
	session := &LiveSession{
		SessionState: SessionState{
			FolderPath: "/workspace/root",
		},
		Target: TargetRef{
			Kind: TargetKindDefault,
		},
	}
	if got := logicaltarget.LegacyLiveSessionKeyID(session); got != "/workspace/root::default::" {
		t.Fatalf("LogicalSessionKeyID(default) = %q, want /workspace/root::default::", got)
	}
}

func TestLogicalSessionKeyID_NamedTargetIncludesFactoryName(t *testing.T) {
	session := &LiveSession{
		SessionState: SessionState{
			FolderPath: "/workspace/root",
		},
		Target: TargetRef{
			Kind: TargetKindNamed,
			Name: "beta",
		},
	}
	if got := logicaltarget.LegacyLiveSessionKeyID(session); got != "/workspace/root::named::beta" {
		t.Fatalf("LogicalSessionKeyID(named) = %q, want /workspace/root::named::beta", got)
	}
}

func TestRegistry_FindByLogicalSessionKeyID_ReturnsMatchingSession(t *testing.T) {
	registry := sessionregistry.New()
	defaultSession := &LiveSession{
		ID: "session-default",
		SessionState: SessionState{
			FolderPath: "/workspace/root",
		},
		Target: TargetRef{Kind: TargetKindDefault},
	}
	namedSession := &LiveSession{
		ID: "session-beta",
		SessionState: SessionState{
			FolderPath: "/workspace/root",
		},
		Target: TargetRef{Kind: TargetKindNamed, Name: "beta"},
	}
	registry.Upsert(defaultSession, true)
	registry.Upsert(namedSession, false)

	if got := registry.FindByLogicalSessionKeyID("/workspace/root::default::"); got != defaultSession {
		t.Fatalf("FindByLogicalSessionKeyID(default) = %#v, want default session", got)
	}
	if got := registry.FindByLogicalSessionKeyID("/workspace/root::named::beta"); got != namedSession {
		t.Fatalf("FindByLogicalSessionKeyID(named) = %#v, want named session", got)
	}
	if got := registry.FindByLogicalSessionKeyID("/workspace/other::default::"); got != nil {
		t.Fatalf("FindByLogicalSessionKeyID(missing) = %#v, want nil", got)
	}
}
