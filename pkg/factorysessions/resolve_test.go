package factorysessions

import "testing"

func TestRegistry_FindByLogicalSessionKeyID_ReturnsMatchingSession(t *testing.T) {
	registry := NewRegistry()
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
