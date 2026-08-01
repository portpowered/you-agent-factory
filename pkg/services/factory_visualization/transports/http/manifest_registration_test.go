package http_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	httpAdapterPackagePath    = "pkg/services/factory_visualization/transports/http"
	httpAdapterImportPath     = "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	factoryVisualizationOwner = "factory_visualization"
)

var httpAdapterPackagePaths = []string{
	httpAdapterPackagePath,
	httpAdapterPackagePath + "/binding",
	httpAdapterPackagePath + "/common",
	httpAdapterPackagePath + "/errors",
	httpAdapterPackagePath + "/lifecycle",
	httpAdapterPackagePath + "/observe",
	httpAdapterPackagePath + "/presentation",
}

var httpAdapterImportPaths = []string{
	httpAdapterImportPath,
	httpAdapterImportPath + "/binding",
	httpAdapterImportPath + "/common",
	httpAdapterImportPath + "/errors",
	httpAdapterImportPath + "/lifecycle",
	httpAdapterImportPath + "/observe",
	httpAdapterImportPath + "/presentation",
}

type coverageMinimumManifest struct {
	Lane     string `json:"lane"`
	Packages []struct {
		Package string  `json:"package"`
		Minimum float64 `json:"minimum"`
	} `json:"packages"`
}

func TestManifestRegistration_HTTPAdapterPackageIsRegistered(t *testing.T) {
	t.Helper()

	assertPackageTargetManifestRegistration(t)
	assertOwnershipInventoryRegistration(t)
	assertCoverageMinimumRegistration(t, "unit", "docs/internal/baselines/go-unit-coverage-package-minimums.json")
	assertCoverageMinimumRegistration(t, "functional", "docs/internal/baselines/go-functional-coverage-package-minimums.json")
}

func assertPackageTargetManifestRegistration(t *testing.T) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, "docs/internal/packaged-service-structure/package-target-manifest.json"))
	if err != nil {
		t.Fatalf("read package-target manifest: %v", err)
	}

	var manifest struct {
		Inventory []string `json:"inventory"`
		Packages  []struct {
			PackagePath string `json:"packagePath"`
			Disposition string `json:"disposition"`
			Destination string `json:"destination"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode package-target manifest: %v", err)
	}

	foundInventory := make(map[string]bool, len(httpAdapterPackagePaths))
	for _, packagePath := range manifest.Inventory {
		for _, expectedPath := range httpAdapterPackagePaths {
			if packagePath == expectedPath {
				foundInventory[expectedPath] = true
			}
		}
	}
	for _, expectedPath := range httpAdapterPackagePaths {
		if !foundInventory[expectedPath] {
			t.Fatalf("package-target manifest inventory missing %q", expectedPath)
		}
	}

	expectedPackages := make(map[string]bool, len(httpAdapterPackagePaths))
	for _, expectedPath := range httpAdapterPackagePaths {
		expectedPackages[expectedPath] = true
	}
	foundPackages := make(map[string]bool, len(httpAdapterPackagePaths))
	for _, row := range manifest.Packages {
		if !expectedPackages[row.PackagePath] {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("package-target manifest disposition for %q = %q, want %q", row.PackagePath, row.Disposition, ownershipinventory.DispositionRetain)
		}
		if row.Destination != factoryVisualizationOwner {
			t.Fatalf("package-target manifest destination for %q = %q, want %q", row.PackagePath, row.Destination, factoryVisualizationOwner)
		}
		foundPackages[row.PackagePath] = true
	}
	for _, expectedPath := range httpAdapterPackagePaths {
		if !foundPackages[expectedPath] {
			t.Fatalf("package-target manifest packages missing %q", expectedPath)
		}
	}
}

func assertOwnershipInventoryRegistration(t *testing.T) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, ownershipinventory.InventoryRelativePath))
	if err != nil {
		t.Fatalf("read ownership inventory: %v", err)
	}

	var inventory struct {
		Packages []ownershipinventory.PackageRow `json:"packages"`
	}
	if err := json.Unmarshal(data, &inventory); err != nil {
		t.Fatalf("decode ownership inventory: %v", err)
	}

	expectedPackages := make(map[string]bool, len(httpAdapterPackagePaths))
	for _, expectedPath := range httpAdapterPackagePaths {
		expectedPackages[expectedPath] = true
	}
	foundPackages := make(map[string]bool, len(httpAdapterPackagePaths))
	for _, row := range inventory.Packages {
		if !expectedPackages[row.PackagePath] {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("ownership inventory disposition for %q = %q, want %q", row.PackagePath, row.Disposition, ownershipinventory.DispositionRetain)
		}
		if row.Destination != factoryVisualizationOwner {
			t.Fatalf("ownership inventory destination for %q = %q, want %q", row.PackagePath, row.Destination, factoryVisualizationOwner)
		}
		if row.DestinationKind != ownershipinventory.DestinationKindOwner {
			t.Fatalf(
				"ownership inventory destinationKind for %q = %q, want %q",
				row.PackagePath,
				row.DestinationKind,
				ownershipinventory.DestinationKindOwner,
			)
		}
		foundPackages[row.PackagePath] = true
	}
	for _, expectedPath := range httpAdapterPackagePaths {
		if !foundPackages[expectedPath] {
			t.Fatalf("ownership inventory packages missing %q", expectedPath)
		}
	}
}

func assertCoverageMinimumRegistration(t *testing.T, lane string, relativePath string) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, relativePath))
	if err != nil {
		t.Fatalf("read %s coverage manifest: %v", lane, err)
	}

	var manifest coverageMinimumManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode %s coverage manifest: %v", lane, err)
	}
	if manifest.Lane != lane {
		t.Fatalf("%s coverage manifest lane = %q, want %q", relativePath, manifest.Lane, lane)
	}

	expectedPackages := make(map[string]bool, len(httpAdapterImportPaths))
	for _, expectedPath := range httpAdapterImportPaths {
		expectedPackages[expectedPath] = true
	}
	foundPackages := make(map[string]bool, len(httpAdapterImportPaths))
	for _, entry := range manifest.Packages {
		if !expectedPackages[entry.Package] {
			continue
		}
		if entry.Minimum < 0 {
			t.Fatalf("%s coverage minimum for %q must be non-negative", lane, entry.Package)
		}
		foundPackages[entry.Package] = true
	}
	for _, expectedPath := range httpAdapterImportPaths {
		if !foundPackages[expectedPath] {
			t.Fatalf("%s coverage manifest missing %q", lane, expectedPath)
		}
	}
}
