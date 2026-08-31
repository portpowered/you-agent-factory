package root_composition_test

import "testing"

// TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction proves
// the one package process composes Factory Sessions without invoking
// lifecycle, runtime-opening, work-admission, response-stream, or request
// identity effects before any routed invocation starts.
func TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	fixture := ensureRootCompositionFixture(t)
	snapshot := fixture.constructionSnapshot()

	if got := snapshot.totalLifecycle(); got != 0 {
		t.Fatalf("lifecycle effect calls = %d during BuildProcess, want 0", got)
	}
	if got := snapshot.totalRuntimeOpening(); got != 0 {
		t.Fatalf("runtime-opening effect calls = %d during BuildProcess, want 0", got)
	}
	if got := snapshot.totalWorkAdmission(); got != 0 {
		t.Fatalf("work-admission effect calls = %d during BuildProcess, want 0", got)
	}
	if got := snapshot.totalResponseStream(); got != 0 {
		t.Fatalf("response-stream effect calls = %d during BuildProcess, want 0", got)
	}
	if got := snapshot.total(); got != 0 {
		t.Fatalf("all injected effect calls = %d during BuildProcess, want 0", got)
	}
}
