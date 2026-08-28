package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestRunAtRootReportsSixOrderedWrites(t *testing.T) {
	root := commandRepositoryRoot(t)
	var stdout bytes.Buffer
	if err := runAtRoot(root, &stdout); err != nil {
		t.Fatalf("runAtRoot() error = %v", err)
	}

	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}
	inventory, err := ownershipinventory.BuildInventory(root, packages)
	if err != nil {
		t.Fatalf("BuildInventory() error = %v", err)
	}
	candidates, err := ownershipinventory.BuildSnapshotCandidates(root)
	if err != nil {
		t.Fatalf("BuildSnapshotCandidates() error = %v", err)
	}
	want := []string{
		fmt.Sprintf("wrote %s (%d packages)", ownershipinventory.InventoryRelativePath, len(inventory.Packages)),
		fmt.Sprintf("wrote %s (2 packets)", ownershipinventory.PathLeaseFreezeRelativePath),
		fmt.Sprintf("wrote %s (%d files)", ownershipinventory.OperatorSettingsRootGoInventoryRelativePath, len(candidates.OperatorSettingsRootGo.Files)),
		fmt.Sprintf("wrote %s (%d directories)", ownershipinventory.OperatorSettingsTopLevelInventoryRelativePath, len(candidates.OperatorSettingsTopLevel.Children)),
		fmt.Sprintf("wrote %s (%d files)", ownershipinventory.ProviderSessionsRootGoInventoryRelativePath, len(candidates.ProviderSessionsRootGo.Files)),
		fmt.Sprintf("wrote %s (%d directories)", ownershipinventory.ProviderSessionsTopLevelInventoryRelativePath, len(candidates.ProviderSessionsTopLevel.Children)),
	}
	got := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if !slices.Equal(got, want) {
		t.Fatalf("runAtRoot() output = %q, want ordered lines %q", got, want)
	}
}

func TestRunFailsOutsideRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout bytes.Buffer
	err := run(&stdout)
	if err == nil {
		t.Fatal("run() error = nil, want repository-root failure")
	}
	if !strings.Contains(err.Error(), "find repository root") {
		t.Fatalf("run() error = %v, want root-discovery stage", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote stdout on root-discovery failure: %q", stdout.String())
	}
}

func commandRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
