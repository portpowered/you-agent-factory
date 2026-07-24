package ownershipinventory_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestValidateAcceptsFrozenRepositoryInventory(t *testing.T) {
	root := repositoryRoot(t)
	report, err := ownershipinventory.Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !report.OK() {
		t.Fatalf("Validate() report = %#v", report)
	}
}

func TestValidateFailsWhenProductionPackageMissing(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.Packages = inventory.Packages[1:]

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with a missing package mapping")
	}
	if len(report.MissingPackages) == 0 {
		t.Fatalf("missing packages empty; report=%#v", report)
	}
}

func TestValidateFailsWhenPackageDuplicated(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	dup := inventory.Packages[0]
	dup.Destination = "work"
	inventory.Packages = append(inventory.Packages, dup)

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with a duplicated package mapping")
	}
	if len(report.DuplicatePackages) == 0 {
		t.Fatalf("duplicate packages empty; report=%#v", report)
	}
}

func TestValidateFailsWhenDestinationMissing(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.Packages[0].Destination = ""
	inventory.Packages[0].Disposition = ownershipinventory.DispositionRetain
	inventory.Packages[0].Successor = ""
	inventory.Packages[0].DeletionCondition = ""

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with an empty destination")
	}
	if len(report.InvalidMappings) == 0 {
		t.Fatalf("invalid mappings empty; report=%#v", report)
	}
}

func TestValidateFailsWhenDeletionQueueLacksSuccessorOrCondition(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.Packages[0].Disposition = ownershipinventory.DispositionDelete
	inventory.Packages[0].Destination = ownershipinventory.DestinationDeletionQueue
	inventory.Packages[0].Successor = ""
	inventory.Packages[0].DeletionCondition = ""

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with incomplete deletion-queue mapping")
	}
	if len(report.InvalidMappings) == 0 {
		t.Fatalf("invalid mappings empty; report=%#v", report)
	}
}

func TestValidateRequiresProcessEdgesArchitectureException(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.ProcessEdgesException = ownershipinventory.ProcessEdgesException{}
	for i := range inventory.Packages {
		if inventory.Packages[i].PackagePath == "pkg/services/edges" {
			inventory.Packages[i].Destination = "workers"
			inventory.Packages[i].DestinationKind = ownershipinventory.DestinationKindOwner
		}
	}

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed without Process Edges exception")
	}
	if !report.MissingProcessEdgesException {
		t.Fatalf("did not flag Process Edges exception; report=%#v", report)
	}
}

func TestValidateFailsWhenInventoryNotStableSorted(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	if len(inventory.Packages) < 2 {
		t.Fatal("need at least two packages")
	}
	inventory.Packages[0], inventory.Packages[1] = inventory.Packages[1], inventory.Packages[0]

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with unstable sort order")
	}
	if !report.UnstableSort {
		t.Fatalf("did not flag unstable sort; report=%#v", report)
	}
}

func TestFrozenInventoryReusesFND01SeedWhenPresent(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}

	tmp := t.TempDir()
	writeJSON(t, filepath.Join(tmp, ownershipinventory.InventoryRelativePath), inventory)
	writeJSON(t, filepath.Join(tmp, ownershipinventory.FND01SeedRelativePath), ownershipinventory.PackageTargetManifest{
		Version:  1,
		Stage:    "pss-fnd-01-package-target-manifest",
		SortKey:  ownershipinventory.SortKeyDescription,
		Packages: inventory.Packages,
	})

	// Corrupt the ownership-inventory package rows so validation can only pass
	// by reusing the FND-01 seed destinations.
	corrupted := inventory
	corrupted.Packages = append([]ownershipinventory.PackageRow(nil), inventory.Packages...)
	corrupted.Packages[0].Destination = ""
	writeJSON(t, filepath.Join(tmp, ownershipinventory.InventoryRelativePath), corrupted)

	// Point package discovery at a tiny pkg tree that mirrors the frozen paths
	// without copying the full repository.
	for _, pkgPath := range packages {
		dir := filepath.Join(tmp, filepath.FromSlash(pkgPath))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "doc.go"), []byte("package "+filepath.Base(dir)+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", dir, err)
		}
	}

	report, err := ownershipinventory.Validate(tmp)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !report.OK() {
		t.Fatalf("Validate() should reuse FND-01 seed; report=%#v", report)
	}
	if !report.ReusedFND01Seed {
		t.Fatal("Validate() did not report FND-01 seed reuse")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := ownershipinventory.FindRepositoryRoot()
	if err != nil {
		t.Fatalf("FindRepositoryRoot() error = %v", err)
	}
	return root
}

func loadedInventoryAndPackages(t *testing.T) (ownershipinventory.Inventory, []string) {
	t.Helper()
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}
	if len(inventory.Packages) == 0 || len(packages) == 0 {
		t.Fatal("expected frozen inventory and production packages")
	}
	return inventory, packages
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
