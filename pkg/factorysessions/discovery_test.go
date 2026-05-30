package factorysessions

import (
	"os"
	"path/filepath"
	"testing"
)

func alwaysRunnableProbe(folderPath, factoryDir string, ref TargetRef) (Target, bool) {
	return BuildTargetFromConfig(folderPath, factoryDir, ref, filepath.Base(factoryDir)), true
}

func TestDiscoverTargets_ReturnsDefaultAndNamedTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "beta"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	targets, err := DiscoverTargets(root, alwaysRunnableProbe)
	if err != nil {
		t.Fatalf("DiscoverTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("len(targets) = %d, want 2", len(targets))
	}
	if targets[0].Ref.Kind != TargetKindDefault || targets[1].Ref != (TargetRef{Kind: TargetKindNamed, Name: "beta"}) {
		t.Fatalf("targets = %#v, want default then named beta", targets)
	}
}

func TestDiscoverTargets_RejectsFolderWithoutRunnableTargets(t *testing.T) {
	root := t.TempDir()
	_, err := DiscoverTargets(root, func(string, string, TargetRef) (Target, bool) {
		return Target{}, false
	})
	if err == nil {
		t.Fatal("DiscoverTargets(empty) error = nil, want not runnable")
	}
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != ValidationReasonNotRunnable || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want not_runnable folderPath", reason, field, ok)
	}
}

func TestSelectTarget_AutoSelectsSingleTarget(t *testing.T) {
	targets := []Target{{
		Ref:   TargetRef{Kind: TargetKindDefault},
		Label: "default",
	}}
	selected, err := SelectTarget(targets, nil)
	if err != nil {
		t.Fatalf("SelectTarget: %v", err)
	}
	if selected == nil || selected.Ref.Kind != TargetKindDefault {
		t.Fatalf("selected = %#v, want default target", selected)
	}
}

func TestSelectTarget_ReturnsNilForAmbiguousFolder(t *testing.T) {
	targets := []Target{
		{Ref: TargetRef{Kind: TargetKindDefault}},
		{Ref: TargetRef{Kind: TargetKindNamed, Name: "beta"}},
	}
	selected, err := SelectTarget(targets, nil)
	if err != nil {
		t.Fatalf("SelectTarget: %v", err)
	}
	if selected != nil {
		t.Fatalf("selected = %#v, want nil for multi-target picker", selected)
	}
}

func TestSelectTarget_RejectsMissingNamedTarget(t *testing.T) {
	targets := []Target{{Ref: TargetRef{Kind: TargetKindDefault}}}
	_, err := SelectTarget(targets, &TargetRef{Kind: TargetKindNamed, Name: "missing"})
	if err == nil {
		t.Fatal("SelectTarget(missing) error = nil, want target_not_found")
	}
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != ValidationReasonTargetNotFound || field != "target.name" {
		t.Fatalf("validation = (%q, %q, %v), want target_not_found target.name", reason, field, ok)
	}
}

func TestCloneTargets_ReturnsDefensiveCopy(t *testing.T) {
	original := []Target{{Ref: TargetRef{Kind: TargetKindDefault}, Label: "default"}}
	cloned := CloneTargets(original)
	original[0].Label = "mutated"
	if cloned[0].Label != "default" {
		t.Fatalf("cloned label = %q, want unchanged copy", cloned[0].Label)
	}
}
