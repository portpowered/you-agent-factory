package work_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	deletedTransitionalWorkServicePackagePath            = "pkg/services/work/service"
	deletedTransitionalStateAccessRecordingsPackagePath = "pkg/services/work/stateaccessrecordings"
)

// DEL-WORK stories 003 and 004 lower structure, ownership, package-target, and
// coverage baselines for the deleted transitional service/ and
// stateaccessrecordings/ packages. Each subtest proves one ledger no longer
// lists a deleted path as retain or move debt.

func TestDelWorkBaselineGate_DeletedTransitionalPackagesBaselinesRemoved(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)

	t.Run("ownership_inventory_omits_deleted_transitional_packages", func(t *testing.T) {
		t.Parallel()
		inventory, err := ownershipinventory.Load(root)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		for _, row := range inventory.Packages {
			switch row.PackagePath {
			case deletedTransitionalWorkServicePackagePath:
				t.Fatalf("ownership inventory still lists deleted transitional package %q", deletedTransitionalWorkServicePackagePath)
			case deletedTransitionalStateAccessRecordingsPackagePath:
				t.Fatalf("ownership inventory still lists deleted transitional package %q", deletedTransitionalStateAccessRecordingsPackagePath)
			}
		}
	})

	t.Run("package_target_manifest_inventory_omits_deleted_transitional_packages", func(t *testing.T) {
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
			switch packagePath {
			case deletedTransitionalWorkServicePackagePath:
				t.Fatalf("package-target manifest inventory still lists deleted transitional package %q", deletedTransitionalWorkServicePackagePath)
			case deletedTransitionalStateAccessRecordingsPackagePath:
				t.Fatalf("package-target manifest inventory still lists deleted transitional package %q", deletedTransitionalStateAccessRecordingsPackagePath)
			}
		}
		for _, row := range manifest.Packages {
			switch row.PackagePath {
			case deletedTransitionalWorkServicePackagePath:
				t.Fatalf("package-target manifest packages still list deleted transitional package %q", deletedTransitionalWorkServicePackagePath)
			case deletedTransitionalStateAccessRecordingsPackagePath:
				t.Fatalf("package-target manifest packages still list deleted transitional package %q", deletedTransitionalStateAccessRecordingsPackagePath)
			}
		}
	})

	t.Run("package_structure_baseline_omits_deleted_transitional_unexpected_directories", func(t *testing.T) {
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
			switch entry.FilePath {
			case deletedTransitionalWorkServicePackagePath:
				t.Fatalf("package-structure baseline still lists deleted transitional path %q under rule %q", entry.FilePath, entry.Rule)
			case deletedTransitionalStateAccessRecordingsPackagePath:
				t.Fatalf("package-structure baseline still lists deleted transitional path %q under rule %q", entry.FilePath, entry.Rule)
			}
		}
	})

	t.Run("unit_coverage_minimums_omit_deleted_transitional_packages", func(t *testing.T) {
		t.Parallel()
		assertCoverageMinimumsOmitDeletedTransitionalPackages(t, root, "go-unit-coverage-package-minimums.json")
	})

	t.Run("functional_coverage_minimums_omit_deleted_transitional_packages", func(t *testing.T) {
		t.Parallel()
		assertCoverageMinimumsOmitDeletedTransitionalPackages(t, root, "go-functional-coverage-package-minimums.json")
	})
}

func assertCoverageMinimumsOmitDeletedTransitionalPackages(t *testing.T, root, fileName string) {
	t.Helper()

	path := filepath.Join(root, "docs", "internal", "baselines", fileName)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var baseline struct {
		Packages []struct {
			Package string `json:"package"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(payload, &baseline); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, row := range baseline.Packages {
		switch row.Package {
		case deletedTransitionalWorkServiceImportPath:
			t.Fatalf("%s still lists deleted transitional package %q", fileName, deletedTransitionalWorkServiceImportPath)
		case deletedTransitionalStateAccessRecordingsImportPath:
			t.Fatalf("%s still lists deleted transitional package %q", fileName, deletedTransitionalStateAccessRecordingsImportPath)
		}
	}
}
