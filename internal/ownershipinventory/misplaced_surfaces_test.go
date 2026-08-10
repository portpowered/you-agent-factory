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

func TestValidateRequiresPublicSurfaces(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.PublicSurfaces = nil

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed without public surfaces")
	}
	if len(report.MissingPublicSurfaces) == 0 {
		t.Fatalf("missing public surfaces empty; report=%#v", report)
	}
}

func TestValidateFailsWhenPublicSurfaceLacksReplacementOwner(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	if len(inventory.PublicSurfaces) == 0 {
		t.Fatal("expected public surfaces in frozen inventory")
	}
	inventory.PublicSurfaces[0].ReplacementOwner = ""

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with blank public-surface replacement owner")
	}
	if len(report.InvalidPublicSurfaces) == 0 {
		t.Fatalf("invalid public surfaces empty; report=%#v", report)
	}
}

func TestValidateRequiresOwnedRoles(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.OwnedRoles = nil

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed without owned roles")
	}
	if len(report.MissingOwnedRoles) == 0 {
		t.Fatalf("missing owned roles empty; report=%#v", report)
	}
}

func TestValidateFailsWhenOwnedRoleLacksDestination(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	if len(inventory.OwnedRoles) == 0 {
		t.Fatal("expected owned roles in frozen inventory")
	}
	inventory.OwnedRoles[0].Destination = ""

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with blank owned-role destination")
	}
	if len(report.InvalidOwnedRoles) == 0 {
		t.Fatalf("invalid owned roles empty; report=%#v", report)
	}
}

func TestBuildInventoryIncludesCorrectedGuardLedgerAndPublicSurfaces(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.BuildInventory(root, []string{"pkg/services/workers"})
	if err != nil {
		t.Fatalf("BuildInventory() error = %v", err)
	}

	if len(inventory.MisplacedGuards) != 0 {
		t.Fatalf("misplaced guards = %#v, want none after ownership correction", inventory.MisplacedGuards)
	}

	if len(inventory.PublicSurfaces) != len(ownershipinventory.RequiredPublicSurfaceIDs()) {
		t.Fatalf("public surfaces = %d, want %d", len(inventory.PublicSurfaces), len(ownershipinventory.RequiredPublicSurfaceIDs()))
	}
	for _, entry := range inventory.PublicSurfaces {
		if strings.TrimSpace(entry.ReplacementOwner) == "" {
			t.Fatalf("public surface %q missing replacementOwner", entry.ID)
		}
		if !ownershipinventory.IsKnownDestination(entry.ReplacementOwner) {
			t.Fatalf("public surface %q replacementOwner %q outside closed vocabulary", entry.ID, entry.ReplacementOwner)
		}
	}

	kinds := map[string]int{}
	for _, entry := range inventory.OwnedRoles {
		kinds[entry.Kind]++
		if strings.TrimSpace(entry.Destination) == "" {
			t.Fatalf("owned role %q missing destination", entry.ID)
		}
		if !ownershipinventory.IsKnownDestination(entry.Destination) && entry.Destination != ownershipinventory.DestinationDeletionQueue {
			t.Fatalf("owned role %q destination %q outside closed vocabulary", entry.ID, entry.Destination)
		}
	}
	for _, kind := range []string{
		ownershipinventory.OwnedRoleKindConstructor,
		ownershipinventory.OwnedRoleKindDatastore,
		ownershipinventory.OwnedRoleKindLifecycleRole,
		ownershipinventory.OwnedRoleKindProtocolAdapter,
	} {
		if kinds[kind] == 0 {
			t.Fatalf("owned roles missing kind %q", kind)
		}
	}
}
