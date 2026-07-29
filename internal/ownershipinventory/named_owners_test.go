package ownershipinventory_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestValidateRequiresNamedOwnerConfirmations(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.NamedOwnerConfirmations = nil

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed without named owner confirmations")
	}
	if len(report.MissingNamedOwners) == 0 {
		t.Fatalf("missing named owners empty; report=%#v", report)
	}
	for _, owner := range ownershipinventory.RequiredNamedOwners {
		if !slices.Contains(report.MissingNamedOwners, owner) {
			t.Fatalf("expected missing named owner %q; got %#v", owner, report.MissingNamedOwners)
		}
	}
}

func TestValidateFailsWhenNamedOwnerNeedsDiscovery(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	if len(inventory.NamedOwnerConfirmations) == 0 {
		t.Fatal("expected named owner confirmations in frozen inventory")
	}
	inventory.NamedOwnerConfirmations[0].Status = "needs_discovery"

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with needs_discovery status")
	}
	if len(report.UnconfirmedNamedOwners) == 0 {
		t.Fatalf("unconfirmed named owners empty; report=%#v", report)
	}
}

func TestValidateFailsWhenNamedOwnerNestedMapMissing(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	found := false
	for i := range inventory.NamedOwnerConfirmations {
		if inventory.NamedOwnerConfirmations[i].Owner != "providers" {
			continue
		}
		inventory.NamedOwnerConfirmations[i].NestedSubservices = nil
		found = true
		break
	}
	if !found {
		t.Fatal("providers named-owner confirmation missing from inventory")
	}

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed without providers nested map")
	}
	if len(report.InvalidNamedOwnerMaps) == 0 {
		t.Fatalf("invalid named owner maps empty; report=%#v", report)
	}
}

func TestValidateFailsWhenAlternateTopLevelOwnerIntroduced(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.NamedOwnerConfirmations = append(inventory.NamedOwnerConfirmations, ownershipinventory.NamedOwnerConfirmation{
		Owner:             "provider_catalog",
		DisplayName:       "Provider Catalog",
		TargetPath:        "pkg/services/provider_catalog",
		Status:            ownershipinventory.NamedOwnerStatusConfirmed,
		NestedSubservices: []string{},
		Note:              "competing catalog abstraction",
	})

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with alternate top-level owner")
	}
	if len(report.InvalidNamedOwnerMaps) == 0 {
		t.Fatalf("invalid named owner maps empty; report=%#v", report)
	}
}

func TestValidateFailsWhenResidualPackageRuleUnmapped(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	for i := range inventory.NamedOwnerConfirmations {
		if inventory.NamedOwnerConfirmations[i].Owner != "providers" {
			continue
		}
		inventory.NamedOwnerConfirmations[i].ResidualPackageRules = append(
			inventory.NamedOwnerConfirmations[i].ResidualPackageRules,
			ownershipinventory.ResidualPackageRule{
				PackagePrefix: "pkg/services/providers/internal/services/execution/internal/provider",
				Destination:   "workers",
				Disposition:   ownershipinventory.DispositionRetain,
				Note:          "intentionally wrong residual mapping for test",
			},
		)
		break
	}

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with residual rule mismatch")
	}
	if len(report.InvalidNamedOwnerMaps) == 0 {
		t.Fatalf("invalid named owner maps empty; report=%#v", report)
	}
}

func TestBuildInventoryIncludesNamedOwnerConfirmations(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.BuildInventory(root, []string{"pkg/services/providers"})
	if err != nil {
		t.Fatalf("BuildInventory() error = %v", err)
	}
	if len(inventory.NamedOwnerConfirmations) != len(ownershipinventory.RequiredNamedOwners) {
		t.Fatalf("named owners = %d, want %d", len(inventory.NamedOwnerConfirmations), len(ownershipinventory.RequiredNamedOwners))
	}
	byOwner := map[string]ownershipinventory.NamedOwnerConfirmation{}
	for _, confirmation := range inventory.NamedOwnerConfirmations {
		byOwner[confirmation.Owner] = confirmation
		if confirmation.Status != ownershipinventory.NamedOwnerStatusConfirmed {
			t.Fatalf("owner %q status = %q, want confirmed", confirmation.Owner, confirmation.Status)
		}
		if strings.TrimSpace(confirmation.TargetPath) == "" {
			t.Fatalf("owner %q missing targetPath", confirmation.Owner)
		}
		if strings.Contains(confirmation.Status, "discovery") || strings.Contains(confirmation.Status, "decomposition") {
			t.Fatalf("owner %q still marked for discovery/decomposition", confirmation.Owner)
		}
	}
	for _, owner := range ownershipinventory.RequiredNamedOwners {
		confirmation, ok := byOwner[owner]
		if !ok {
			t.Fatalf("missing named owner confirmation for %q", owner)
		}
		wantNested := ownershipinventory.NamedOwnerNestedSubservices[owner]
		if !slices.Equal(confirmation.NestedSubservices, wantNested) {
			t.Fatalf("owner %q nested = %#v, want %#v", owner, confirmation.NestedSubservices, wantNested)
		}
	}
	providers := byOwner["providers"]
	if len(providers.ResidualPackageRules) == 0 {
		t.Fatal("providers confirmation missing residual package rules")
	}
}

