package providersessions_test

import (
	"fmt"
	"os"
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

// providerSessionsProductionPublicSurfacePackages are owner production packages
// outside INV-recorded private destinations. They must depend only on the
// committed public surface (thin root, wire/, and transports/), not unexpected
// public siblings.
var providerSessionsProductionPublicSurfacePackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/wire",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/transports/http",
}

// providerSessionsInternalBehaviorProofFiles are folded implementation tests
// that exercise wire-constructed Details, Inspect, and Project on Codex- and
// Cursor-backed reader fixtures for the published Service peer surface.
var providerSessionsInternalBehaviorProofFiles = []string{
	"wire_behavioral_proof_test.go",
	"details_providers_boundary_test.go",
	"inspect_providers_boundary_test.go",
	"project_providers_boundary_test.go",
	"service_test.go",
	"readers_providers_boundary_test.go",
}

// TestProviderSessionsRootBehaviorPreserved seals pss-cln-pses-legacy-packages-004:
// focused wire-constructed behavioral proofs and Providers-root boundary tests
// remain under provider_sessions/internal and the root-go inventory gate stays
// green so legacy-sibling cleanup cannot silently drop Details/Inspect/Project
// observability coverage.
func TestProviderSessionsRootBehaviorPreserved(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsRootGoInventory(root); err != nil {
		t.Fatalf("VerifyProviderSessionsRootGoInventory() error = %v", err)
	}

	internalRoot := filepath.Join(root, "pkg", "services", "provider_sessions", "internal")
	for _, name := range providerSessionsInternalBehaviorProofFiles {
		path := filepath.Join(internalRoot, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("internal behavior proof file %q missing: %v", name, err)
		}
	}
}

// TestProductionPackagesDoNotImportUnexpectedPublicSiblingsBeyondService seals
// pss-cln-pses-legacy-packages-003: no production importer outside INV-recorded
// private destinations may depend on an unexpected Provider Sessions public
// sibling path beyond service/.
func TestProductionPackagesDoNotImportUnexpectedPublicSiblingsBeyondService(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	forbiddenImportPrefixes := forbiddenUnexpectedPublicSiblingBeyondServiceImportPrefixes(t, root)

	var violations []string
	for _, packagePath := range listProductionPackagesSubjectToUnexpectedPublicSiblingImportGuard(t) {
		violations = append(violations, unexpectedPublicSiblingImportViolations(t, packagePath, forbiddenImportPrefixes)...)
	}
	if len(violations) > 0 {
		t.Fatalf("forbidden unexpected public sibling imports beyond service/:\n%s", strings.Join(violations, "\n"))
	}
}

// TestOwnerProductionPublicSurfaceDoesNotImportUnexpectedPublicSiblingsBeyondService
// seals pss-cln-pses-legacy-packages-003: provider_sessions/wire and other
// owner public-surface production packages must not retain imports of
// unexpected public sibling packages beyond service/.
func TestOwnerProductionPublicSurfaceDoesNotImportUnexpectedPublicSiblingsBeyondService(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	forbiddenImportPrefixes := forbiddenUnexpectedPublicSiblingBeyondServiceImportPrefixes(t, root)

	var violations []string
	for _, packagePath := range providerSessionsProductionPublicSurfacePackages {
		violations = append(violations, unexpectedPublicSiblingImportViolations(t, packagePath, forbiddenImportPrefixes)...)
	}
	if len(violations) > 0 {
		t.Fatalf("owner public-surface unexpected public sibling imports beyond service/:\n%s", strings.Join(violations, "\n"))
	}
}

func listProductionPackagesSubjectToUnexpectedPublicSiblingImportGuard(t *testing.T) []string {
	t.Helper()

	packages := append([]string(nil), listPackagesOutsideProviderSessionsOwner(t)...)
	packages = append(packages, providerSessionsProductionPublicSurfacePackages...)
	return packages
}

func forbiddenUnexpectedPublicSiblingBeyondServiceImportPrefixes(t *testing.T, root string) []string {
	t.Helper()

	inventory, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}

	var forbiddenImportPrefixes []string
	for _, packagePath := range ownershipinventory.ProviderSessionsUnexpectedPublicSiblingBeyondServicePackagePaths(inventory) {
		forbiddenImportPrefixes = append(forbiddenImportPrefixes, moduleImportPath(packagePath))
	}
	return forbiddenImportPrefixes
}

func unexpectedPublicSiblingImportViolations(t *testing.T, packagePath string, forbiddenImportPrefixes []string) []string {
	t.Helper()

	var violations []string
	for _, dep := range listTransitiveDeps(t, packagePath) {
		for _, forbidden := range forbiddenImportPrefixes {
			if !matchesForbiddenUnexpectedPublicSiblingImport(dep, forbidden) {
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
	return violations
}

func matchesForbiddenUnexpectedPublicSiblingImport(dep, forbidden string) bool {
	return dep == forbidden || strings.HasPrefix(dep, forbidden+"/")
}

func TestMatchesForbiddenUnexpectedPublicSiblingImport(t *testing.T) {
	t.Parallel()

	forbidden := moduleImportPath("pkg/services/provider_sessions/surprise")
	tests := []struct {
		dep  string
		want bool
	}{
		{forbidden, true},
		{forbidden + "/nested", true},
		{moduleImportPath("pkg/services/provider_sessions/wire"), false},
		{moduleImportPath("pkg/services/provider_sessions"), false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.dep, func(t *testing.T) {
			t.Parallel()
			if got := matchesForbiddenUnexpectedPublicSiblingImport(test.dep, forbidden); got != test.want {
				t.Fatalf("matchesForbiddenUnexpectedPublicSiblingImport(%q, %q) = %t, want %t", test.dep, forbidden, got, test.want)
			}
		})
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
