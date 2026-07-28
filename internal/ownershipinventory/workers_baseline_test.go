package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestFrozenInventoryWorkersRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	const ownerPrefix = "pkg/services/workers/"
	for _, row := range inventory.Packages {
		if row.PackagePath == "pkg/services/workers" {
			continue
		}
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(row.PackagePath, ownerPrefix)
		if workersCanonicalRetainRest(rest) {
			continue
		}
		if isWorkersProvidersExtractionRest(rest) {
			if row.Disposition != ownershipinventory.DispositionMove || row.Destination != "providers" {
				t.Fatalf("providers extraction %q = %#v, want move→providers", row.PackagePath, row)
			}
			if row.Successor == "" || row.DeletionCondition == "" {
				t.Fatalf("providers extraction %q missing successor/deletionCondition: %#v", row.PackagePath, row)
			}
			continue
		}
		if row.Disposition == ownershipinventory.DispositionRetain && row.Destination == "workers" {
			t.Fatalf("frozen inventory row retain→workers for %q", row.PackagePath)
		}
		if row.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("frozen inventory row %q disposition = %q, want move", row.PackagePath, row.Disposition)
		}
		if row.Successor == "" || row.DeletionCondition == "" {
			t.Fatalf("frozen inventory row %q missing successor/deletionCondition: %#v", row.PackagePath, row)
		}
	}
}

func TestWorkersCommittedBaselinesAlignMoveDestinations(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	manifest, err := loadPackageTargetManifest(root)
	if err != nil {
		t.Fatalf("loadPackageTargetManifest() error = %v", err)
	}

	ownershipByPath := make(map[string]ownershipinventory.PackageRow, len(inventory.Packages))
	for _, row := range inventory.Packages {
		ownershipByPath[row.PackagePath] = row
	}

	const ownerPrefix = "pkg/services/workers/"
	for _, row := range manifest.Packages {
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionMove {
			continue
		}
		rest := strings.TrimPrefix(row.PackagePath, ownerPrefix)
		if isWorkersProvidersExtractionRest(rest) {
			continue
		}

		ownershipRow, ok := ownershipByPath[row.PackagePath]
		if !ok {
			t.Fatalf("ownership inventory missing committed manifest move row %q", row.PackagePath)
		}
		wantSuccessor := "pkg/services/" + row.Destination
		if ownershipRow.Successor != wantSuccessor {
			t.Fatalf("dual-ledger drift for %q: manifest destination %q => successor %q, ownership has %q",
				row.PackagePath, row.Destination, wantSuccessor, ownershipRow.Successor)
		}
	}
}
