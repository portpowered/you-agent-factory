package factorydefinitions

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamedFactoryPathRootWrappersPreserveCanonicalLayouts(t *testing.T) {
	if err := ValidateName("@you/goal"); err != nil {
		t.Fatalf("ValidateName(@you/goal): %v", err)
	}
	segments, err := PathSegments("@you/goal")
	if err != nil || len(segments) != 2 || segments[0] != "@you" || segments[1] != "goal" {
		t.Fatalf("PathSegments(@you/goal) = %#v, %v", segments, err)
	}
	if got, err := NameFromPathSegments(segments); err != nil || got != "@you/goal" {
		t.Fatalf("NameFromPathSegments(%#v) = %q, %v", segments, got, err)
	}
	root := filepath.Join("home", "factories")
	if got, err := MapDir(root, "@you/goal"); err != nil || got != filepath.Join(root, "@you", "goal") {
		t.Fatalf("MapDir() = %q, %v", got, err)
	}
	if got := LegacyNamedFactoriesRoot("home"); !strings.HasSuffix(got, filepath.Join(".you-agent-factory", "you-agent-factories")) {
		t.Fatalf("LegacyNamedFactoriesRoot() = %q", got)
	}

	if err := ValidateName("../escape"); err == nil || !errors.Is(err, ErrInvalidName) {
		t.Fatalf("ValidateName(../escape) = %v, want ErrInvalidName", err)
	}
	if _, err := NameFromPathSegments([]string{"@you"}); err == nil {
		t.Fatal("NameFromPathSegments(scope-only) unexpectedly succeeded")
	}
}

func TestResolveNamedFactoryRootsReportsMissingExplicitEdges(t *testing.T) {
	if _, err := ResolveNamedFactoryRoots("", "repo"); err == nil {
		t.Fatal("ResolveNamedFactoryRoots() accepted an empty home directory")
	}
	if _, err := ResolveNamedFactoryRoots("home", ""); err == nil {
		t.Fatal("ResolveNamedFactoryRoots() accepted an empty working directory")
	}
}

func TestPolicyAndResolutionContractHelpersRemainDetached(t *testing.T) {
	workstation := &FactoryWorkstationConfig{Name: "workstation"}
	if got := WorkPropagationPolicyFunc(func(got *FactoryWorkstationConfig) WorkPropagationMode {
		if got != workstation {
			t.Fatalf("policy received %p, want %p", got, workstation)
		}
		return WorkPropagationMode("clone")
	}).Mode(workstation); got != WorkPropagationMode("clone") {
		t.Fatalf("WorkPropagationPolicyFunc.Mode() = %q", got)
	}

	if got := (&ExecutionCatalogError{}).Error(); got != "execution catalog resolution failed" {
		t.Fatalf("empty ExecutionCatalogError = %q", got)
	}
	if got := (&ExecutionCatalogError{Diagnostics: []ExecutionCatalogDiagnostic{{Message: "unknown worker"}}}).Error(); !strings.Contains(got, "unknown worker") {
		t.Fatalf("diagnostic ExecutionCatalogError = %q", got)
	}
	original := ResolveExecutionCatalogResult{
		ResolvedExecutionCatalog: ResolvedExecutionCatalog{
			DefinitionVersion: "v1",
			Workers:           map[string]ResolvedWorkerDefinition{"worker": {Name: "worker"}},
		},
		Diagnostics: []ExecutionCatalogDiagnostic{{Message: "diagnostic"}},
	}
	cloned := original.Clone()
	cloned.Workers["worker"] = ResolvedWorkerDefinition{Name: "changed"}
	if original.Workers["worker"].Name != "worker" || cloned.Diagnostics[0].Message != "diagnostic" {
		t.Fatalf("ResolveExecutionCatalogResult.Clone() did not detach values: original=%#v clone=%#v", original, cloned)
	}
}
