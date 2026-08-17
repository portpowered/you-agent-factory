package main

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestCommittedLedgerOperatorSettingsMovesUnexpectedSiblingsOffTheOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	topLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}
	moveByPath := committedMoveLedgerRows(t, repoRoot)

	for _, packagePath := range ownershipinventory.OperatorSettingsUnexpectedPublicSiblingPackagePaths(topLevel) {
		row, ok := moveByPath[packagePath]
		if !ok {
			t.Fatalf("move ledger missing unexpected public sibling %q; it must still carry an open move", packagePath)
		}
		if row.Destination == "operator_settings" {
			t.Fatalf("move ledger row %q destination = owner root, want nested plan path", packagePath)
		}
	}
}

func TestVerifyOperatorSettingsUnexpectedPublicSiblingRemapsPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsUnexpectedPublicSiblingRemaps(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsUnexpectedPublicSiblingRemaps() error = %v", err)
	}
}

func TestVerifyOperatorSettingsTopLevelInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsTopLevelInventory(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsTopLevelInventory() error = %v", err)
	}
}

func TestVerifyOperatorSettingsRootGoInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsRootGoInventory(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsRootGoInventory() error = %v", err)
	}
}

func TestVerifyOperatorSettingsCommittedRootContractInventoryAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsCommittedRootContractInventoryAlignment(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsCommittedRootContractInventoryAlignment() error = %v", err)
	}
}

func TestVerifyOperatorSettingsRootReconciliationPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsRootReconciliation(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsRootReconciliation() error = %v", err)
	}
}

func TestOperatorSettingsCommittedBaselinesAlignMoveDestinations(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	moveByPath := committedMoveLedgerRows(t, repoRoot)

	const ownerPrefix = "pkg/services/operator_settings/"
	checked := 0
	for packagePath, row := range moveByPath {
		if packagePath != "pkg/services/operator_settings" && !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		checked++
		wantSuccessor := "pkg/services/" + row.Destination
		if row.Successor != wantSuccessor {
			t.Fatalf("move ledger drift for %q: destination %q => successor %q, ledger has %q",
				packagePath, row.Destination, wantSuccessor, row.Successor)
		}
	}
	if checked == 0 {
		t.Fatal("no Operator Settings rows found in the move ledger; the alignment check proved nothing")
	}
}

func operatorSettingsCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/document"):
		return true
	case strings.HasPrefix(rest, "internal/services/resolution"):
		return true
	default:
		return false
	}
}
