package backendsizecheck

import (
	"path/filepath"
	"testing"
)

func TestInventory_DeclaresBackendOwnedRootsAndExplicitExclusions(t *testing.T) {
	t.Parallel()

	inventory := Inventory()
	if len(inventory) != 5 {
		t.Fatalf("inventory entries = %d, want 5 backend-size scope roots", len(inventory))
	}

	wantRoots := map[string]struct{}{
		"cmd":      {},
		"internal": {},
		"pkg":      {},
		"tests":    {},
		"vendor":   {},
	}

	for _, entry := range inventory {
		if _, ok := wantRoots[entry.Root]; !ok {
			t.Fatalf("unexpected backend-size inventory root %q", entry.Root)
		}
		delete(wantRoots, entry.Root)

		if len(entry.Rules) == 0 {
			t.Fatalf("%s must declare at least one backend-size scope rule", entry.Root)
		}
		for _, rule := range entry.Rules {
			if rule.Path == "" || rule.Why == "" {
				t.Fatalf("%s contains incomplete rule %#v", entry.Root, rule)
			}
		}
	}

	if len(wantRoots) != 0 {
		t.Fatalf("missing backend-size roots: %#v", wantRoots)
	}
}

func TestShouldSkipDir_UsesHiddenGeneratedFixtureAndVendorPolicy(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Join("repo", "root")

	if !ShouldSkipDir(filepath.Join(repoRoot, "cmd"), filepath.Join(repoRoot, "cmd", ".cache")) {
		t.Fatal("cmd scope must skip hidden directories")
	}
	if !ShouldSkipDir(filepath.Join(repoRoot, "pkg"), filepath.Join(repoRoot, "pkg", "api", "generated")) {
		t.Fatal("pkg scope must skip generated API output")
	}
	if !ShouldSkipDir(filepath.Join(repoRoot, "pkg"), filepath.Join(repoRoot, "pkg", "service", "testdata")) {
		t.Fatal("pkg scope must skip testdata fixtures")
	}
	if ShouldSkipDir(filepath.Join(repoRoot, "pkg"), filepath.Join(repoRoot, "pkg", "service")) {
		t.Fatal("pkg scope must keep maintained backend package directories")
	}
	if !ShouldSkipDir(filepath.Join(repoRoot, "tests"), filepath.Join(repoRoot, "tests", "functional", "runtime_api", "testdata")) {
		t.Fatal("tests scope must skip testdata fixtures")
	}
	if !ShouldSkipDir(filepath.Join(repoRoot, "vendor"), filepath.Join(repoRoot, "vendor")) {
		t.Fatal("vendor root must stay excluded")
	}
}
