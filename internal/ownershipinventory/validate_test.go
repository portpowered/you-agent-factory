package ownershipinventory_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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

func TestValidateAcceptsLivePackageWithNoInventoryRow(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	// pkg/initializer used to carry a "retain" row that only restated the
	// directory it already lives in. It must now be absent from the registry and
	// still validate.
	const retired = "pkg/initializer"
	if !slices.Contains(packages, retired) {
		t.Fatalf("%q is not a live production package; pick another rowless package", retired)
	}
	for _, row := range inventory.Packages {
		if row.PackagePath == retired {
			t.Fatalf("%q still carries a registry row %#v; retain rows were meant to be retired", retired, row)
		}
	}

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if !report.OK() {
		t.Fatalf("ValidateInventory() rejected rowless live package %q; report=%#v", retired, report)
	}
}

func TestValidateAcceptsPackageAddedInsideExistingServiceWithNoRegistryEdit(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	added := "pkg/services/workers/dcp2scratchpackage"
	packages = append(append([]string(nil), packages...), added)

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if !report.OK() {
		t.Fatalf("ValidateInventory() rejected new package %q that has no registry row; report=%#v", added, report)
	}
	owner, ok := ownershipinventory.OwnerForPackage(added)
	if !ok || owner != "workers" {
		t.Fatalf("OwnerForPackage(%q) = %q, %v; want \"workers\", true", added, owner, ok)
	}
}

func TestValidateFailsWhenMoveRowNamesAbsentPackage(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.Packages = append(append([]ownershipinventory.PackageRow(nil), inventory.Packages...), ownershipinventory.PackageRow{
		PackagePath:       "pkg/services/workers/dcp2deletedpackage",
		Disposition:       ownershipinventory.DispositionMove,
		Destination:       "workers",
		DestinationKind:   ownershipinventory.DestinationKindOwner,
		Successor:         "pkg/services/workers/internal",
		DeletionCondition: "delete after cutover proof",
	})

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with a move row naming a package that is absent from disk")
	}
	if len(report.UnexpectedPackages) == 0 {
		t.Fatalf("unexpected packages empty; report=%#v", report)
	}
}

func TestViolationCountCountsStructuredFindingsIndividually(t *testing.T) {
	report := ownershipinventory.Report{
		UnexpectedPackages:          []string{"pkg/a", "pkg/b"},
		MissingCrossServiceEdges:    []string{"runtime->models", "recordings->events"},
		UnexpectedCrossServiceEdges: []string{"workers->sessions"},
	}
	if got := report.ViolationCount(); got != 5 {
		t.Fatalf("ViolationCount() = %d, want 5 structured findings", got)
	}
	report.UnexpectedPackages = append(report.UnexpectedPackages, "pkg/c")
	if got := report.ViolationCount(); got != 6 {
		t.Fatalf("ViolationCount() after one added package = %d, want 6", got)
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

// The ownership-inventory artifact no longer carries package rows at all: open
// moves live in one consolidated ledger. Loading a root whose inventory file has
// no rows must still produce the ledger's rows.
func TestLoadReadsPackageRowsFromConsolidatedMoveLedger(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmp := t.TempDir()
	writeJSON(t, filepath.Join(tmp, ownershipinventory.InventoryRelativePath), inventory)
	writeJSON(t, filepath.Join(tmp, ownershipinventory.UnfinishedMovesRelativePath), ownershipinventory.UnfinishedMoves{
		Version:  1,
		Stage:    ownershipinventory.UnfinishedMovesStage,
		EndState: "shrinks to zero, then the file is deleted",
		SortKey:  ownershipinventory.SortKeyDescription,
		Moves: []ownershipinventory.UnfinishedMoveRow{{
			PackagePath:       "pkg/services/workers/service",
			Destination:       "workers/internal",
			Successor:         "pkg/services/workers/internal",
			DeletionCondition: "delete after cutover proof",
		}},
	})

	reloaded, err := ownershipinventory.Load(tmp)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(reloaded.Packages) != 1 {
		t.Fatalf("Load() attached %d package row(s) from the move ledger, want 1: %#v", len(reloaded.Packages), reloaded.Packages)
	}
	row := reloaded.Packages[0]
	if row.PackagePath != "pkg/services/workers/service" {
		t.Fatalf("row packagePath = %q, want pkg/services/workers/service", row.PackagePath)
	}
	if row.Disposition != ownershipinventory.DispositionMove {
		t.Fatalf("row disposition = %q, want move", row.Disposition)
	}
	// Owner and destination kind are derived from the destination bucket rather
	// than restated on every ledger row.
	if row.Destination != "workers" || row.DestinationKind != ownershipinventory.DestinationKindOwner {
		t.Fatalf("row destination = %q/%q, want workers/owner", row.Destination, row.DestinationKind)
	}
	if row.Successor != "pkg/services/workers/internal" {
		t.Fatalf("row successor = %q, want pkg/services/workers/internal", row.Successor)
	}
}

// A landed migration deletes its row, so the ledger's own end state — no file at
// all — has to load as an empty row set rather than an error.
func TestLoadTreatsAbsentMoveLedgerAsNoOpenMoves(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmp := t.TempDir()
	writeJSON(t, filepath.Join(tmp, ownershipinventory.InventoryRelativePath), inventory)

	reloaded, err := ownershipinventory.Load(tmp)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(reloaded.Packages) != 0 {
		t.Fatalf("Load() attached %d package row(s) with no ledger present, want 0", len(reloaded.Packages))
	}
}

func TestValidateFailsWhenMoveLedgerSuccessorLeavesItsDestinationOwner(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.UnfinishedMoves.Moves = append(
		append([]ownershipinventory.UnfinishedMoveRow(nil), inventory.UnfinishedMoves.Moves...),
		ownershipinventory.UnfinishedMoveRow{
			PackagePath: "pkg/services/workers/service",
			Destination: "workers/internal",
			Successor:   "pkg/services/recordings/internal",
		},
	)

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with a successor outside its destination owner")
	}
	if len(report.InvalidMappings) == 0 {
		t.Fatalf("invalid mappings empty; report=%#v", report)
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
