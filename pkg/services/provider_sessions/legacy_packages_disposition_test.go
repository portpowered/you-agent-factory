package providersessions_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// TestProviderSessionsZeroExtraPublicSiblingAbsenceLocked seals
// pss-cln-pses-legacy-packages-002: inventory/generator assertions keep INV's
// zero-extra absence locked so a new unexpected public top-level sibling cannot
// silently retain without an explicit remap or consistent INV flag flip.
func TestProviderSessionsZeroExtraPublicSiblingAbsenceLocked(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsZeroExtraPublicSiblingAbsence(root); err != nil {
		t.Fatalf("VerifyProviderSessionsZeroExtraPublicSiblingAbsence() error = %v", err)
	}

	inventory, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}
	if inventory.HasUnexpectedPublicSiblingsBeyondService {
		t.Fatal("hasUnexpectedPublicSiblingsBeyondService = true, want false for zero-extra lock")
	}
	if len(inventory.UnexpectedPublicSiblingsBeyondService) != 0 {
		t.Fatalf("unexpectedPublicSiblingsBeyondService = %v, want none", inventory.UnexpectedPublicSiblingsBeyondService)
	}

	beyondService := ownershipinventory.ProviderSessionsUnexpectedPublicSiblingBeyondServicePackagePaths(inventory)
	if len(beyondService) != 0 {
		t.Fatalf("beyond-service package paths = %v, want none while zero-extra lock is active", beyondService)
	}

	var unexpected []string
	for _, child := range inventory.Children {
		if child.Directory == "service" {
			continue
		}
		if child.Classification == ownershipinventory.ProviderSessionsTopLevelUnexpectedPublicSibling ||
			child.Classification == ownershipinventory.ProviderSessionsTopLevelINVUnexpectedPublicSibling {
			unexpected = append(unexpected, child.Directory)
		}
	}
	if len(unexpected) != 0 {
		t.Fatalf("unexpected public siblings beyond service/ = %v, want none", unexpected)
	}
}

// TestProviderSessionsINVDispositionBeyondServiceConsumesZeroExtraPath seals
// pss-cln-pses-legacy-packages-001: when INV records zero unexpected public
// siblings beyond service/, this packet confirms the zero-extra path and makes
// no package moves for non-service siblings.
func TestProviderSessionsINVDispositionBeyondServiceConsumesZeroExtraPath(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsINVDispositionBeyondService(root); err != nil {
		t.Fatalf("VerifyProviderSessionsINVDispositionBeyondService() error = %v", err)
	}

	inventory, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}
	if inventory.HasUnexpectedPublicSiblingsBeyondService {
		t.Fatal("hasUnexpectedPublicSiblingsBeyondService = true, want false for zero-extra path")
	}
	if len(inventory.UnexpectedPublicSiblingsBeyondService) != 0 {
		t.Fatalf("unexpectedPublicSiblingsBeyondService = %v, want none", inventory.UnexpectedPublicSiblingsBeyondService)
	}
}

// TestProductionPackagesDoNotImportUnexpectedPublicSiblingsBeyondService seals
// pss-cln-pses-legacy-packages-001: no unexpected public sibling beyond service/
// may remain a live production import target outside INV-recorded private
// destinations.
func TestProductionPackagesDoNotImportUnexpectedPublicSiblingsBeyondService(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}

	var forbiddenImportPrefixes []string
	for _, packagePath := range ownershipinventory.ProviderSessionsUnexpectedPublicSiblingBeyondServicePackagePaths(inventory) {
		forbiddenImportPrefixes = append(forbiddenImportPrefixes, moduleImportPath(packagePath))
	}

	var violations []string
	for _, packagePath := range listPackagesOutsideProviderSessionsOwner(t) {
		for _, dep := range listTransitiveDeps(t, packagePath) {
			for _, forbidden := range forbiddenImportPrefixes {
				if dep != forbidden && !strings.HasPrefix(dep, forbidden+"/") {
					continue
				}
				violations = append(
					violations,
					fmt.Sprintf(
						"%s must not depend on unexpected public sibling %s outside INV private destinations (found %s)",
						packagePath,
						forbidden,
						dep,
					),
				)
			}
		}
	}
	if len(violations) > 0 {
		t.Fatalf("forbidden unexpected public sibling imports beyond service/:\n%s", strings.Join(violations, "\n"))
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return dir
}

func moduleImportPath(packagePath string) string {
	return "github.com/portpowered/infinite-you/" + packagePath
}
