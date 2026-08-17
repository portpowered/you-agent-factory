package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestNoMisplacedGuardsRemainAfterOwnershipCorrection(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	if got := ownershipinventory.RequiredMisplacedGuardIDs(); len(got) != 0 {
		t.Fatalf("required misplaced guards = %v, want none after ownership correction", got)
	}
	if len(inventory.MisplacedGuards) != 0 {
		t.Fatalf("frozen misplaced guards = %#v, want none after ownership correction", inventory.MisplacedGuards)
	}

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if len(report.MissingMisplacedGuards) != 0 || len(report.InvalidMisplacedGuards) != 0 {
		t.Fatalf("corrected misplaced-guard ledger produced defects: %#v", report)
	}
}

func TestValidateRejectsRetiredMisplacedGuard(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.MisplacedGuards = []ownershipinventory.MisplacedGuardEntry{{
		ID:                "retired:workers-provider-effect",
		Kind:              ownershipinventory.MisplacedGuardKindStandard,
		SurfacePath:       "docs/internal/standards/code/general-backend-standards.md",
		CurrentOwnerClaim: "workers",
		MisplacedConcern:  ownershipinventory.MisplacedConcernProviderInference,
		ReplacementOwner:  "providers",
		Note:              "retired stale assignment",
	}}

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly accepted a retired misplaced guard")
	}
	if !strings.Contains(strings.Join(report.InvalidMisplacedGuards, "\n"), "retired:workers-provider") {
		t.Fatalf("retired misplaced guard was not reported: %#v", report)
	}
}

func TestBuildInventoryEmitsCorrectedGuardLedger(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.BuildInventory(root, []string{"pkg/services/workers"})
	if err != nil {
		t.Fatalf("BuildInventory() error = %v", err)
	}

	if len(inventory.MisplacedGuards) != 0 {
		t.Fatalf("misplaced guards = %#v, want none after ownership correction", inventory.MisplacedGuards)
	}
}
