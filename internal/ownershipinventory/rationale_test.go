package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestValidateRequiresTopLevelOwnerRationaleCards(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.OwnerRationales = nil

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed without owner rationale cards")
	}
	if len(report.MissingOwnerRationales) == 0 {
		t.Fatalf("missing owner rationales empty; report=%#v", report)
	}
	for _, owner := range ownershipinventory.ProductOwners {
		found := false
		for _, missing := range report.MissingOwnerRationales {
			if missing == owner {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected missing rationale for %q; got %#v", owner, report.MissingOwnerRationales)
		}
	}
}

func TestValidateRequiresNestedServiceRationaleCards(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	filtered := make([]ownershipinventory.OwnerRationaleCard, 0, len(inventory.OwnerRationales))
	for _, card := range inventory.OwnerRationales {
		if card.Kind == ownershipinventory.RationaleKindNested &&
			card.ServiceID == "work/admission" {
			continue
		}
		filtered = append(filtered, card)
	}
	inventory.OwnerRationales = filtered

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed without nested rationale cards")
	}
	found := false
	for _, missing := range report.MissingNestedRationales {
		if missing == "work/admission" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing nested rationale work/admission; got %#v", report.MissingNestedRationales)
	}
}

func TestValidateFailsWhenRationaleFieldBlank(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	if len(inventory.OwnerRationales) == 0 {
		t.Fatal("expected owner rationales in frozen inventory")
	}
	inventory.OwnerRationales[0].Authority = "   "

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with blank rationale field")
	}
	if len(report.InvalidRationaleFields) == 0 {
		t.Fatalf("invalid rationale fields empty; report=%#v", report)
	}
}

func TestValidateRequiresResponsibilityClusters(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.ResponsibilityClusters = nil

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed without responsibility clusters")
	}
	if len(report.MissingResponsibilityClusters) == 0 {
		t.Fatalf("missing responsibility clusters empty; report=%#v", report)
	}
}

func TestBuildInventoryIncludesCommittedRationaleCards(t *testing.T) {
	inventory, err := ownershipinventory.BuildInventory([]string{"pkg/services/work"})
	if err != nil {
		t.Fatalf("BuildInventory() error = %v", err)
	}
	if len(inventory.OwnerRationales) == 0 {
		t.Fatal("BuildInventory() omitted owner rationales")
	}
	topLevel := map[string]bool{}
	nested := map[string]bool{}
	for _, card := range inventory.OwnerRationales {
		switch card.Kind {
		case ownershipinventory.RationaleKindTopLevel:
			topLevel[card.ServiceID] = true
		case ownershipinventory.RationaleKindNested:
			nested[card.ServiceID] = true
		default:
			t.Fatalf("unexpected rationale kind %q on %q", card.Kind, card.ServiceID)
		}
		for _, field := range []string{
			card.Authority,
			card.StateStore,
			card.Lifecycle,
			card.Consumers,
			card.TransactionBoundary,
			card.FailureRecovery,
		} {
			if strings.TrimSpace(field) == "" {
				t.Fatalf("rationale card %q has a blank required field", card.ServiceID)
			}
		}
	}
	for _, owner := range ownershipinventory.ProductOwners {
		if !topLevel[owner] {
			t.Fatalf("missing top-level rationale for %q", owner)
		}
	}
	for _, serviceID := range []string{
		"factory_definitions/catalog",
		"factory_sessions/identity",
		"factory_runtime/orchestration",
		"work/admission",
		"workers/runners/agent",
		"providers/catalog",
		"provider_sessions/codex_reader",
		"models/runtime_host/leases",
		"automations/hosted_sources",
		"recordings/canonical_ledger",
		"factory_visualization/activation_lifecycle",
		"operator_settings/document",
	} {
		if !nested[serviceID] {
			t.Fatalf("missing nested rationale for %q", serviceID)
		}
	}
	if len(inventory.ResponsibilityClusters) == 0 {
		t.Fatal("BuildInventory() omitted responsibility clusters")
	}
}
