package factorydefinitions

import (
	"path/filepath"
	"testing"
)

func TestOwnedDefaultRootsRequireExplicitEdges(t *testing.T) {
	home := filepath.Join("home", "customer")
	if got, err := NamedFactoriesRootForHome(home); err != nil || got != NamedFactoriesRoot(home) {
		t.Fatalf("NamedFactoriesRootForHome() = %q, %v", got, err)
	}
	workingDir := filepath.Join("repo", "app")
	if got, err := ProjectFactoriesRootForWorkingDir(workingDir); err != nil || got != ProjectFactoriesRoot(workingDir) {
		t.Fatalf("ProjectFactoriesRootForWorkingDir() = %q, %v", got, err)
	}
	if _, err := NamedFactoriesRootForHome(" "); err == nil {
		t.Fatal("NamedFactoriesRootForHome() must reject an absent home edge")
	}
	if _, err := ProjectFactoriesRootForWorkingDir(" "); err == nil {
		t.Fatal("ProjectFactoriesRootForWorkingDir() must reject an absent working-directory edge")
	}
}

func TestResolveNamedFactoryRootsPreservesProjectFirstLookupInputs(t *testing.T) {
	home := filepath.Join("home", "customer")
	workingDir := filepath.Join("repo", "app")
	got, err := ResolveNamedFactoryRoots(home, workingDir)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryRoots() error = %v", err)
	}
	if got.Project != ProjectFactoriesRoot(workingDir) || got.Global != NamedFactoriesRoot(home) {
		t.Fatalf("ResolveNamedFactoryRoots() = %#v", got)
	}
}
