package main

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const deletedProviderSessionsServicePackagePath = "pkg/services/provider_sessions/service"

func TestVerifyProviderSessionsUnexpectedPublicSiblingRemapsPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsUnexpectedPublicSiblingRemaps(repoRoot); err != nil {
		t.Fatalf("VerifyProviderSessionsUnexpectedPublicSiblingRemaps() error = %v", err)
	}
}

func TestVerifyProviderSessionsTopLevelInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsTopLevelInventory(repoRoot); err != nil {
		t.Fatalf("VerifyProviderSessionsTopLevelInventory() error = %v", err)
	}
}

func TestVerifyProviderSessionsZeroExtraPublicSiblingAbsencePassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsZeroExtraPublicSiblingAbsence(repoRoot); err != nil {
		t.Fatalf("VerifyProviderSessionsZeroExtraPublicSiblingAbsence() error = %v", err)
	}
}

func TestVerifyProviderSessionsINVDispositionBeyondServicePassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsINVDispositionBeyondService(repoRoot); err != nil {
		t.Fatalf("VerifyProviderSessionsINVDispositionBeyondService() error = %v", err)
	}
}

// Provider Sessions finished its migration, so it holds no open moves at all.
// The lock is the absence of a row: reopening a Provider Sessions move fails
// here and has to be a deliberate change rather than a quiet one.
func TestProviderSessionsHasNoOpenMovesInTheLedger(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	moveByPath := committedMoveLedgerRows(t, repoRoot)

	const ownerPrefix = "pkg/services/provider_sessions/"
	for packagePath, row := range moveByPath {
		if packagePath != "pkg/services/provider_sessions" && !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		t.Fatalf("move ledger reopened a Provider Sessions move for %q (successor %q); the owner is settled",
			packagePath, row.Successor)
	}
}
