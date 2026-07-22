package runtimetests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Hermetic S02 failure-baseline fixtures for named-factory path resolution when
// one-shot @you/goal-style named runs cannot resolve a runnable factory directory.

func TestFailureBaseline_NamedPath_MissingLocalFactoryReportsCrossRootNotFound(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	_, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "missing-alpha")
	if err == nil {
		t.Fatal("expected missing local named factory to fail")
	}
	if !errors.Is(err, ErrNamedFactoryNotFound) {
		t.Fatalf("error = %v, want ErrNamedFactoryNotFound", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
	if got := err.Error(); !containsAll(got, `resolve named factory "missing-alpha"`, "project root", "global root", `named factory "missing-alpha" not found`) {
		t.Fatalf("error = %q, want cross-root not-found guidance", got)
	}
}

func TestFailureBaseline_NamedPath_InvalidCanonicalNameRejectsScopedPathContract(t *testing.T) {
	_, err := ResolveNamedFactoryAcrossRoots(t.TempDir(), t.TempDir(), "@you")
	if err == nil {
		t.Fatal("expected invalid scoped named factory name to fail")
	}
	if !errors.Is(err, ErrInvalidNamedFactoryName) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactoryName", err)
	}
	if errors.Is(err, ErrNamedFactoryNotFound) {
		t.Fatalf("error = %v, did not want ErrNamedFactoryNotFound", err)
	}
	if got := err.Error(); !containsAll(got, `invalid named factory name "@you"`, `must be scoped as @scope/name`) {
		t.Fatalf("error = %q, want invalid scoped named-path guidance", got)
	}
}

func TestFailureBaseline_NamedPath_UnknownBuiltInGoalStyleNameReportsNotFound(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	_, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/missing")
	if err == nil {
		t.Fatal("expected unknown built-in named factory to fail")
	}
	if !errors.Is(err, ErrNamedFactoryNotFound) {
		t.Fatalf("error = %v, want ErrNamedFactoryNotFound", err)
	}
	if got := err.Error(); stringsContainsMaterializeBuiltIn(err.Error()) || !containsAll(got, `resolve named factory "@you/missing"`, "project root", "global root") {
		t.Fatalf("error = %q, want deterministic built-in named-path not-found guidance", got)
	}
}

func stringsContainsMaterializeBuiltIn(value string) bool {
	return containsAll(value, "materialize built-in named factory")
}

func TestFailureBaseline_NamedPath_InstalledGoalUsesHierarchicalLayout(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	if _, err := factorydefinitioncomposition.PersistNamedFactory(globalRoot, "@you/goal", namedFactoryPayload(t, "installed-goal"), ownerFactoryDefinitionValidator()); err != nil {
		t.Fatalf("PersistNamedFactory(@you/goal): %v", err)
	}

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(@you/goal): %v", err)
	}
	if resolution.Name != "@you/goal" {
		t.Fatalf("resolution name = %q, want @you/goal", resolution.Name)
	}

	wantDir := filepath.Join(globalRoot, "@you", "goal")
	if resolution.FactoryDir != wantDir {
		t.Fatalf("factory dir = %q, want hierarchical layout %q", resolution.FactoryDir, wantDir)
	}
	if strings.Contains(resolution.FactoryDir, "%2F") {
		t.Fatalf("factory dir = %q, must not use percent-encoded scoped leaf names", resolution.FactoryDir)
	}
	if _, statErr := os.Stat(wantDir); statErr != nil {
		t.Fatalf("materialized factory dir %q: %v", wantDir, statErr)
	}
}
