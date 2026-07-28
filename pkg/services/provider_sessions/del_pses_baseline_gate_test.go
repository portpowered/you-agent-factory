package providersessions_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const deletedProviderSessionsServicePackagePath = "pkg/services/provider_sessions/service"

// DEL-PSES story 003 lowers structure, ownership, and package-target baselines
// for the deleted transitional service/ package. Each subtest proves one ledger
// no longer lists the deleted path as retain or move debt.

func TestDelPsesBaselineGate_DeletedTransitionalServiceBaselinesRemoved(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)

	t.Run("ownership_inventory_omits_deleted_service_package", func(t *testing.T) {
		t.Parallel()
		inventory, err := ownershipinventory.Load(root)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		for _, row := range inventory.Packages {
			if row.PackagePath == deletedProviderSessionsServicePackagePath {
				t.Fatalf("ownership inventory still lists deleted transitional package %q", deletedProviderSessionsServicePackagePath)
			}
		}
	})

	t.Run("package_target_manifest_inventory_omits_deleted_service_package", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(root, "docs", "internal", "packaged-service-structure", "package-target-manifest.json")
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		var manifest struct {
			Inventory []string `json:"inventory"`
			Packages  []struct {
				PackagePath string `json:"packagePath"`
			} `json:"packages"`
		}
		if err := json.Unmarshal(payload, &manifest); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		for _, packagePath := range manifest.Inventory {
			if packagePath == deletedProviderSessionsServicePackagePath {
				t.Fatalf("package-target manifest inventory still lists deleted transitional package %q", deletedProviderSessionsServicePackagePath)
			}
		}
		for _, row := range manifest.Packages {
			if row.PackagePath == deletedProviderSessionsServicePackagePath {
				t.Fatalf("package-target manifest packages still list deleted transitional package %q", deletedProviderSessionsServicePackagePath)
			}
		}
	})

	t.Run("package_structure_baseline_omits_deleted_service_unexpected_directory", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(root, "docs", "internal", "baselines", "package-structure-baseline.json")
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		var baseline struct {
			Entries []struct {
				Rule     string `json:"rule"`
				FilePath string `json:"filePath"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(payload, &baseline); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		for _, entry := range baseline.Entries {
			if entry.FilePath == deletedProviderSessionsServicePackagePath {
				t.Fatalf("package-structure baseline still lists deleted transitional path %q under rule %q", entry.FilePath, entry.Rule)
			}
		}
	})
}
