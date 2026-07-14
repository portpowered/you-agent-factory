package cli

import (
	"testing"
)

func TestUseGeneratedWorkFamilyCutoverEnabled(t *testing.T) {
	if !useGeneratedWorkFamily {
		t.Fatal("useGeneratedWorkFamily = false, want production cutover enabled")
	}
}

func TestProductionWorkCommandUsesGeneratedFamily(t *testing.T) {
	work := productionWorkCommand(&cliGlobalOptions{}, &cliDiagnosticsOptions{})
	if work == nil {
		t.Fatal("productionWorkCommand() = nil, want work command")
	}
	if work.RunE != nil {
		t.Fatal("generated work parent must remain non-runnable")
	}
	for _, path := range []string{"list", "show", "move", "visualize"} {
		if _, _, err := work.Find([]string{path}); err != nil {
			t.Fatalf("generated work tree missing %q: %v", path, err)
		}
	}
}

func TestProductionWorkCommandAttachesHandwrittenRunE(t *testing.T) {
	work := productionWorkCommand(&cliGlobalOptions{}, &cliDiagnosticsOptions{})
	list, _, err := work.Find([]string{"list"})
	if err != nil {
		t.Fatalf("Find(list) error = %v", err)
	}
	if list.RunE == nil {
		t.Fatal("generated work list must attach handwritten RunE")
	}
	show, _, err := work.Find([]string{"show"})
	if err != nil {
		t.Fatalf("Find(show) error = %v", err)
	}
	if show.RunE == nil {
		t.Fatal("generated work show must attach handwritten RunE")
	}
	move, _, err := work.Find([]string{"move"})
	if err != nil {
		t.Fatalf("Find(move) error = %v", err)
	}
	if move.RunE == nil {
		t.Fatal("generated work move must attach handwritten RunE")
	}
	visualize, _, err := work.Find([]string{"visualize"})
	if err != nil {
		t.Fatalf("Find(visualize) error = %v", err)
	}
	if visualize.RunE == nil {
		t.Fatal("generated work visualize must attach handwritten RunE")
	}
}

func TestLegacyWorkFamilyConstructorsRemainCallable(t *testing.T) {
	legacy := NewLegacyWorkFamilyCommand()
	if legacy == nil {
		t.Fatal("NewLegacyWorkFamilyCommand() = nil")
	}
	if _, _, err := legacy.Find([]string{"list"}); err != nil {
		t.Fatalf("legacy work tree missing list: %v", err)
	}

	legacyRoot := NewLegacyWorkFamilyRootForParity()
	if legacyRoot == nil {
		t.Fatal("NewLegacyWorkFamilyRootForParity() = nil")
	}
	work, _, err := legacyRoot.Find([]string{"work"})
	if err != nil {
		t.Fatalf("Find(work) error = %v", err)
	}
	if _, _, err := work.Find([]string{"visualize"}); err != nil {
		t.Fatalf("legacy parity root missing visualize: %v", err)
	}
}
