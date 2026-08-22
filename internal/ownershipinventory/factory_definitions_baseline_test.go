package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// committedMoveLedger loads the consolidated open-move ledger keyed by package
// path. Every row in it is an open move; a package that is settled where it
// already lives carries no row at all.
func committedMoveLedger(t *testing.T, root string) map[string]ownershipinventory.UnfinishedMoveRow {
	t.Helper()
	moves, err := ownershipinventory.LoadUnfinishedMoves(root)
	if err != nil {
		t.Fatalf("LoadUnfinishedMoves() error = %v", err)
	}
	byPath := make(map[string]ownershipinventory.UnfinishedMoveRow, len(moves.Moves))
	for _, row := range moves.Moves {
		byPath[row.PackagePath] = row
	}
	return byPath
}

func TestFrozenInventoryFactoryDefinitionsRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	const ownerPrefix = "pkg/services/factory_definitions/"
	for _, row := range inventory.Packages {
		if row.PackagePath == "pkg/services/factory_definitions" {
			continue
		}
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(row.PackagePath, ownerPrefix)
		if factoryDefinitionsCanonicalRetainRest(rest) {
			continue
		}
		if row.Disposition == ownershipinventory.DispositionRetain && row.Destination == "factory_definitions" {
			t.Fatalf("frozen inventory row retain→factory_definitions for %q", row.PackagePath)
		}
		if row.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("frozen inventory row %q disposition = %q, want move", row.PackagePath, row.Disposition)
		}
		if row.Successor == "" || row.DeletionCondition == "" {
			t.Fatalf("frozen inventory row %q missing successor/deletionCondition: %#v", row.PackagePath, row)
		}
	}
}

// Each consolidated row carries both the owner-relative destination bucket and
// the repository-relative successor path. For Factory Definitions the two must
// describe the same target, so a row cannot claim one destination while sending
// the package somewhere else.
func TestFactoryDefinitionsCommittedBaselinesAlignMoveDestinations(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	moves, err := ownershipinventory.LoadUnfinishedMoves(root)
	if err != nil {
		t.Fatalf("LoadUnfinishedMoves() error = %v", err)
	}

	const ownerPrefix = "pkg/services/factory_definitions/"
	checked := 0
	for _, row := range moves.Moves {
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
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
		t.Fatal("no Factory Definitions rows found in the move ledger; the alignment check proved nothing")
	}
}

func factoryDefinitionsCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "internal":
		return true
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/catalog"):
		return true
	case strings.HasPrefix(rest, "internal/services/authoring_layout"):
		return true
	case strings.HasPrefix(rest, "internal/services/validation"):
		return true
	case strings.HasPrefix(rest, "internal/services/compilation"):
		return true
	case strings.HasPrefix(rest, "internal/services/distribution"):
		return true
	case strings.HasPrefix(rest, "internal/services/invocation_policy"):
		return true
	case strings.HasPrefix(rest, "internal/services/snapshots_portability"):
		return true
	case strings.HasPrefix(rest, "internal/lifecycle"):
		return true
	case strings.HasPrefix(rest, "internal/contracts"):
		return true
	default:
		return false
	}
}
