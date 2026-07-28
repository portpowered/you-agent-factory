package workers_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	delWrkWorkersPackagePrefix      = "pkg/services/workers/"
	workersModuleImportPrefix       = "github.com/portpowered/infinite-you/pkg/services/workers/"
	heldBackExecutorAgentRunPackage = delWrkWorkersPackagePrefix + "executor/agentrun"
)

var deletedTransitionalWorkersPackagePaths = []string{
	delWrkWorkersPackagePrefix + "construction",
	delWrkWorkersPackagePrefix + "diagnostics",
	delWrkWorkersPackagePrefix + "draftvalidation",
	delWrkWorkersPackagePrefix + "execution",
	delWrkWorkersPackagePrefix + "execution/recording",
	delWrkWorkersPackagePrefix + "executor",
	delWrkWorkersPackagePrefix + "inferencefailure",
	delWrkWorkersPackagePrefix + "interface",
	delWrkWorkersPackagePrefix + "services/agents",
	delWrkWorkersPackagePrefix + "services/inference",
	delWrkWorkersPackagePrefix + "skippermissions",
	delWrkWorkersPackagePrefix + "workstationpool",
}

// DEL-WRK story 003 lowers structure, ownership, package-target, and coverage
// baselines for deleted Workers transitional packages. Each subtest proves one
// ledger no longer lists a deleted path as retain or move debt.

func TestDelWrkBaselineGate_DeletedTransitionalPackagesBaselinesRemoved(t *testing.T) {
	t.Parallel()

	root := delWrkRepoRoot(t)

	t.Run("ownership_inventory_omits_deleted_transitional_packages", func(t *testing.T) {
		t.Parallel()

		inventory, err := ownershipinventory.Load(root)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		for _, row := range inventory.Packages {
			for _, deleted := range deletedTransitionalWorkersPackagePaths {
				if row.PackagePath != deleted {
					continue
				}
				t.Fatalf("ownership inventory still lists deleted transitional package %q", deleted)
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
			for _, deleted := range deletedTransitionalWorkersPackagePaths {
				if packagePath != deleted {
					continue
				}
				t.Fatalf("package-target manifest inventory still lists deleted transitional package %q", deleted)
			}
		}
		for _, row := range manifest.Packages {
			for _, deleted := range deletedTransitionalWorkersPackagePaths {
				if row.PackagePath != deleted {
					continue
				}
				t.Fatalf("package-target manifest packages still list deleted transitional package %q", deleted)
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
			for _, deleted := range deletedTransitionalWorkersPackagePaths {
				if entry.FilePath != deleted {
					continue
				}
				if deleted == delWrkWorkersPackagePrefix+"executor" {
					continue
				}
				t.Fatalf(
					"package-structure baseline still lists deleted transitional path %q under rule %q",
					entry.FilePath,
					entry.Rule,
				)
			}
		}
	})

	t.Run("unit_coverage_minimums_omit_deleted_transitional_packages", func(t *testing.T) {
		t.Parallel()
		assertCoverageMinimumsOmitDeletedTransitionalWorkersPackages(t, root, "go-unit-coverage-package-minimums.json")
	})

	t.Run("functional_coverage_minimums_omit_deleted_transitional_packages", func(t *testing.T) {
		t.Parallel()
		assertCoverageMinimumsOmitDeletedTransitionalWorkersPackages(t, root, "go-functional-coverage-package-minimums.json")
	})
}

func deletedTransitionalWorkersImportPaths() []string {
	paths := make([]string, len(deletedTransitionalWorkersPackagePaths))
	for i, packagePath := range deletedTransitionalWorkersPackagePaths {
		paths[i] = strings.Replace(packagePath, delWrkWorkersPackagePrefix, workersModuleImportPrefix, 1)
	}
	return paths
}

func assertCoverageMinimumsOmitDeletedTransitionalWorkersPackages(t *testing.T, root, fileName string) {
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

	deletedImportPaths := deletedTransitionalWorkersImportPaths()
	heldBackAgentRunImport := strings.Replace(
		heldBackExecutorAgentRunPackage,
		delWrkWorkersPackagePrefix,
		workersModuleImportPrefix,
		1,
	)
	for _, row := range baseline.Packages {
		if row.Package == heldBackAgentRunImport {
			continue
		}
		for _, deleted := range deletedImportPaths {
			if row.Package != deleted {
				continue
			}
			t.Fatalf("%s still lists deleted transitional package %q", fileName, row.Package)
		}
	}
}
