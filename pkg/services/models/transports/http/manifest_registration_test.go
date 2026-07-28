package http_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	httpAdapterPackagePath = "pkg/services/models/transports/http"
	httpAdapterImportPath  = "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	modelsOwner            = "models"
)

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

	foundInventory := false
	for _, packagePath := range manifest.Inventory {
		if packagePath == httpAdapterPackagePath {
			foundInventory = true
			break
		}
	}
	if !foundInventory {
		t.Fatalf("package-target manifest inventory missing %q", httpAdapterPackagePath)
	}

	for _, row := range manifest.Packages {
		if row.PackagePath != httpAdapterPackagePath {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("package-target manifest disposition = %q, want %q", row.Disposition, ownershipinventory.DispositionRetain)
		}
		if row.Destination != modelsOwner {
			t.Fatalf("package-target manifest destination = %q, want %q", row.Destination, modelsOwner)
		}
		return
	}
	t.Fatalf("package-target manifest packages missing %q", httpAdapterPackagePath)
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

	for _, row := range inventory.Packages {
		if row.PackagePath != httpAdapterPackagePath {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("ownership inventory disposition = %q, want %q", row.Disposition, ownershipinventory.DispositionRetain)
		}
		if row.Destination != modelsOwner {
			t.Fatalf("ownership inventory destination = %q, want %q", row.Destination, modelsOwner)
		}
		if row.DestinationKind != ownershipinventory.DestinationKindOwner {
			t.Fatalf(
				"ownership inventory destinationKind = %q, want %q",
				row.DestinationKind,
				ownershipinventory.DestinationKindOwner,
			)
		}
		return
	}
	t.Fatalf("ownership inventory packages missing %q", httpAdapterPackagePath)
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

	for _, entry := range manifest.Packages {
		if entry.Package != httpAdapterImportPath {
			continue
		}
		if entry.Minimum < 0 {
			t.Fatalf("%s coverage minimum for %q must be non-negative", lane, httpAdapterImportPath)
		}
		return
	}
	t.Fatalf("%s coverage manifest missing %q", lane, httpAdapterImportPath)
}
