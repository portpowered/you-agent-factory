package root_composition_test

import "testing"

// TestOperatorSettingsEffectsRemainInertThroughRootBuildProcessConstruction proves
// canonical process composition retains Operator Settings without invoking operator-config
// filesystem, temporary-file creation, or backend-scope ID generation external
// effects before runtime lifecycle starts.
func TestOperatorSettingsEffectsRemainInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	assertOperatorSettingsRouteFailures(t)
	fixture := ensureSharedOperatorSettingsFixture(t)
	snapshot := fixture.constructionEffectSnapshot()

	if got := snapshot.fileSystemCalls; got != 0 {
		t.Fatalf("operator-config filesystem effect calls = %d during process construction, want 0", got)
	}
	if got := snapshot.createTemporaryCalls; got != 0 {
		t.Fatalf("operator-config CreateTemporaryFile calls = %d during process construction, want 0", got)
	}
	if got := snapshot.operatorIDCalls; got != 0 {
		t.Fatalf("operator-config IDGenerator calls = %d during process construction, want 0", got)
	}
}
