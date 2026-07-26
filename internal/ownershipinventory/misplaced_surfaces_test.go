package ownershipinventory_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestValidateRequiresMisplacedGuards(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.MisplacedGuards = nil

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed without misplaced guards")
	}
	if len(report.MissingMisplacedGuards) == 0 {
		t.Fatalf("missing misplaced guards empty; report=%#v", report)
	}
	for _, id := range ownershipinventory.RequiredMisplacedGuardIDs() {
		if !slices.Contains(report.MissingMisplacedGuards, id) {
			t.Fatalf("expected missing misplaced guard %q; got %#v", id, report.MissingMisplacedGuards)
		}
	}
}

func TestValidateFailsWhenMisplacedGuardLacksReplacementOwner(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	if len(inventory.MisplacedGuards) == 0 {
		t.Fatal("expected misplaced guards in frozen inventory")
	}
	inventory.MisplacedGuards[0].ReplacementOwner = ""

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with blank replacement owner")
	}
	if len(report.InvalidMisplacedGuards) == 0 {
		t.Fatalf("invalid misplaced guards empty; report=%#v", report)
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

func TestBuildInventoryIncludesMisplacedGuardsAndPublicSurfaces(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.BuildInventory(root, []string{"pkg/services/workers"})
	if err != nil {
		t.Fatalf("BuildInventory() error = %v", err)
	}

	if len(inventory.MisplacedGuards) != len(ownershipinventory.RequiredMisplacedGuardIDs()) {
		t.Fatalf("misplaced guards = %d, want %d", len(inventory.MisplacedGuards), len(ownershipinventory.RequiredMisplacedGuardIDs()))
	}
	guardByID := map[string]ownershipinventory.MisplacedGuardEntry{}
	for _, entry := range inventory.MisplacedGuards {
		guardByID[entry.ID] = entry
		if strings.TrimSpace(entry.ReplacementOwner) == "" {
			t.Fatalf("misplaced guard %q missing replacementOwner", entry.ID)
		}
		if entry.CurrentOwnerClaim != "workers" {
			t.Fatalf("misplaced guard %q currentOwnerClaim = %q, want workers", entry.ID, entry.CurrentOwnerClaim)
		}
		switch entry.MisplacedConcern {
		case ownershipinventory.MisplacedConcernProviderInference,
			ownershipinventory.MisplacedConcernHostedPolling:
		default:
			t.Fatalf("misplaced guard %q has unknown concern %q", entry.ID, entry.MisplacedConcern)
		}
		switch entry.MisplacedConcern {
		case ownershipinventory.MisplacedConcernProviderInference:
			if entry.ReplacementOwner != "providers" {
				t.Fatalf("provider_inference guard %q replacement = %q, want providers", entry.ID, entry.ReplacementOwner)
			}
		case ownershipinventory.MisplacedConcernHostedPolling:
			if entry.ReplacementOwner != "automations" {
				t.Fatalf("hosted_polling guard %q replacement = %q, want automations", entry.ID, entry.ReplacementOwner)
			}
		}
	}
	for _, id := range ownershipinventory.RequiredMisplacedGuardIDs() {
		if _, ok := guardByID[id]; !ok {
			t.Fatalf("missing misplaced guard %q", id)
		}
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
