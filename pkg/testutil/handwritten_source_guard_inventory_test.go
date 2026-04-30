package testutil

import "testing"

func TestHandwrittenSourceGuardInventory_CoversTargetedGuardsAndClassifications(t *testing.T) {
	t.Parallel()

	inventory := HandwrittenSourceGuardInventory()
	if len(inventory) != 5 {
		t.Fatalf("inventory entries = %d, want 5 targeted handwritten-source guards", len(inventory))
	}

	wantGuards := map[string]struct{}{
		"pkg/api/legacy_model_guard_test.go":                         {},
		"pkg/petri/transition_contract_guard_test.go":                {},
		"pkg/interfaces/world_view_contract_guard_test.go#boundary":  {},
		"pkg/interfaces/world_view_contract_guard_test.go#canonical": {},
		"pkg/interfaces/runtime_lookup_contract_guard_test.go":       {},
	}

	for _, entry := range inventory {
		if _, ok := wantGuards[entry.GuardFile]; !ok {
			t.Fatalf("unexpected guard inventory entry %q", entry.GuardFile)
		}
		delete(wantGuards, entry.GuardFile)

		var hasHandwritten bool
		for _, rule := range entry.Rules {
			if rule.Class == HandwrittenSourcePathClassScanHandwritten {
				hasHandwritten = true
			}
			if rule.Path == "" || rule.Why == "" {
				t.Fatalf("%s contains incomplete rule %#v", entry.GuardFile, rule)
			}
		}
		if !hasHandwritten {
			t.Fatalf("%s must declare at least one handwritten scan rule", entry.GuardFile)
		}
	}

	if len(wantGuards) != 0 {
		t.Fatalf("missing guard inventory entries: %#v", wantGuards)
	}
}

func TestHandwrittenSourceGuardInventory_RecordsHiddenAndGeneratedExclusionsForBroadWalkers(t *testing.T) {
	t.Parallel()

	inventory := HandwrittenSourceGuardInventory()
	for _, entry := range inventory {
		if entry.WalkRoot != "repo-root" && entry.WalkRoot != "pkg" {
			continue
		}

		var hasGenerated bool
		var hasHidden bool
		for _, rule := range entry.Rules {
			switch rule.Class {
			case HandwrittenSourcePathClassExcludeGenerated:
				hasGenerated = true
			case HandwrittenSourcePathClassExcludeHiddenRoot:
				hasHidden = true
			}
		}

		if entry.WalkRoot == "repo-root" && !hasHidden {
			t.Fatalf("%s must record hidden-root exclusions for repo-root walks", entry.GuardFile)
		}
		if entry.GuardFile != "pkg/interfaces/world_view_contract_guard_test.go#boundary" && !hasGenerated && entry.WalkRoot != "pkg/interfaces" {
			t.Fatalf("%s must record generated-output exclusions when scanning broad handwritten-source roots", entry.GuardFile)
		}
	}
}
