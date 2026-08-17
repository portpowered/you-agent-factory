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
	moves, err := ownershipinventory.LoadUnfinishedMoves(root)
	if err != nil {
		t.Fatalf("LoadUnfinishedMoves() error = %v", err)
	}

	const ownerPrefix = "pkg/services/workers/"
	checked := 0
	for _, row := range moves.Moves {
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(row.PackagePath, ownerPrefix)
		if isWorkersProvidersExtractionRest(rest) {
			continue
		}
		checked++
		wantSuccessor := "pkg/services/" + row.Destination
		if row.Successor != wantSuccessor {
			t.Fatalf("move ledger drift for %q: destination %q => successor %q, ledger has %q",
				row.PackagePath, row.Destination, wantSuccessor, row.Successor)
		}
	}
	if checked == 0 {
		t.Fatal("no Workers rows found in the move ledger; the alignment check proved nothing")
	}
}
