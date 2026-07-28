package factorydefinitions_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	ownerPrefix                           = "pkg/services/factory_definitions/"
	factoryDefinitionsModuleImportPrefix  = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/"
)

var deletedResidualTransitionalPackagePaths = []string{
	ownerPrefix + "namedpaths",
	ownerPrefix + "namedfactories",
	ownerPrefix + "persistence",
	ownerPrefix + "resource",
	ownerPrefix + "loading",
	ownerPrefix + "loadedsource",
	ownerPrefix + "runtimeconfig",
	ownerPrefix + "validation",
	ownerPrefix + "workers",
	ownerPrefix + "namevalue",
	ownerPrefix + "portableconfig",
	ownerPrefix + "snapshotcapture",
	ownerPrefix + "editable",
	ownerPrefix + "replayconfig",
	ownerPrefix + "packages",
	ownerPrefix + "packagedinstallation",
	ownerPrefix + "decisionenvelope",
	ownerPrefix + "invocationinterpolation",
	ownerPrefix + "invocationoutput",
	ownerPrefix + "invocationworktype",
	ownerPrefix + "quorumpolicy",
	ownerPrefix + "workpropagation",
	ownerPrefix + "workstationexecution",
	ownerPrefix + "ttsobservability",
}

// DEL-DEF-RESIDUAL story 003 lowers structure, ownership, and package-target
// baselines for deleted residual transitional public packages. Each subtest
// proves one ledger no longer lists a deleted path as retain or move debt.
func TestDelDefResidualBaselineGate_DeletedTransitionalPackagesBaselinesRemoved(t *testing.T) {
	t.Parallel()

	root := delDefResidualRepoRoot(t)

	t.Run("ownership_inventory_omits_deleted_transitional_packages", func(t *testing.T) {
		t.Parallel()

		inventory, err := ownershipinventory.Load(root)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		for _, row := range inventory.Packages {
			for _, deleted := range deletedResidualTransitionalPackagePaths {
				if row.PackagePath != deleted {
					continue
				}
				t.Fatalf("ownership inventory still lists deleted residual transitional package %q", deleted)
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
			for _, deleted := range deletedResidualTransitionalPackagePaths {
				if packagePath != deleted {
					continue
				}
				t.Fatalf("package-target manifest inventory still lists deleted residual transitional package %q", deleted)
			}
		}
		for _, row := range manifest.Packages {
			for _, deleted := range deletedResidualTransitionalPackagePaths {
				if row.PackagePath != deleted {
					continue
				}
				t.Fatalf("package-target manifest packages still list deleted residual transitional package %q", deleted)
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
			for _, deleted := range deletedResidualTransitionalPackagePaths {
				if entry.FilePath != deleted {
					continue
				}
				t.Fatalf(
					"package-structure baseline still lists deleted residual transitional path %q under rule %q",
					entry.FilePath,
					entry.Rule,
				)
			}
		}
	})

	t.Run("unit_coverage_minimums_omit_deleted_transitional_packages", func(t *testing.T) {
		t.Parallel()
		assertCoverageMinimumsOmitDeletedResidualTransitionalPackages(t, root, "go-unit-coverage-package-minimums.json")
	})

	t.Run("functional_coverage_minimums_omit_deleted_transitional_packages", func(t *testing.T) {
		t.Parallel()
		assertCoverageMinimumsOmitDeletedResidualTransitionalPackages(t, root, "go-functional-coverage-package-minimums.json")
	})
}

func deletedResidualTransitionalImportPaths() []string {
	paths := make([]string, len(deletedResidualTransitionalPackagePaths))
	for i, packagePath := range deletedResidualTransitionalPackagePaths {
		paths[i] = strings.Replace(packagePath, ownerPrefix, factoryDefinitionsModuleImportPrefix, 1)
	}
	paths = append(paths,
		factoryDefinitionsModuleImportPrefix+"packages/goal",
		factoryDefinitionsModuleImportPrefix+"packages/packageassets",
		factoryDefinitionsModuleImportPrefix+"packages/promptassets",
		factoryDefinitionsModuleImportPrefix+"packages/review",
		factoryDefinitionsModuleImportPrefix+"packages/subagent",
		factoryDefinitionsModuleImportPrefix+"packages/tts",
		factoryDefinitionsModuleImportPrefix+"workers/taxonomy",
	)
	return paths
}

func assertCoverageMinimumsOmitDeletedResidualTransitionalPackages(t *testing.T, root, fileName string) {
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

	deletedImportPaths := deletedResidualTransitionalImportPaths()
	for _, row := range baseline.Packages {
		for _, deleted := range deletedImportPaths {
			if row.Package != deleted && !strings.HasPrefix(row.Package, deleted+"/") {
				continue
			}
			t.Fatalf("%s still lists deleted residual transitional package %q", fileName, row.Package)
		}
	}
}
