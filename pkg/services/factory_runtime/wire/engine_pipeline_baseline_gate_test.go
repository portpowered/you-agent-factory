package wire_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// DEL-RUN-ENGINE-PIPELINE stories 004 and 005 lower structure, ownership,
// package-target, and coverage baselines for deleted public engine/pipeline
// packages. Each subtest proves one ledger no longer lists a deleted path as
// retain or move debt.

const factoryRuntimeModuleImportPrefix = "github.com/portpowered/infinite-you/pkg/services/factory_runtime/"

func deletedEnginePipelinePublicPackagePaths() []string {
	children := deletedEnginePipelinePublicTopLevelChildren()
	paths := make([]string, len(children))
	for i, name := range children {
		paths[i] = "pkg/services/factory_runtime/" + name
	}
	return paths
}

func deletedEnginePipelinePublicImportPaths() []string {
	children := deletedEnginePipelinePublicTopLevelChildren()
	paths := make([]string, len(children))
	for i, name := range children {
		paths[i] = factoryRuntimeModuleImportPrefix + name
	}
	return paths
}

func TestEnginePipelineBaselineGate_DeletedPublicPipelineBaselinesRemoved(t *testing.T) {
	t.Parallel()

	root := serviceDeletionRepoRoot(t)
	deletedPaths := deletedEnginePipelinePublicPackagePaths()

	t.Run("ownership_inventory_omits_deleted_public_pipeline_packages", func(t *testing.T) {
		t.Parallel()

		inventory, err := ownershipinventory.Load(root)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		for _, row := range inventory.Packages {
			for _, deleted := range deletedPaths {
				if row.PackagePath == deleted || strings.HasPrefix(row.PackagePath, deleted+"/") {
					t.Fatalf("ownership inventory still lists deleted pipeline package %q", row.PackagePath)
				}
			}
		}
	})

	t.Run("package_target_manifest_inventory_omits_deleted_public_pipeline_packages", func(t *testing.T) {
		t.Parallel()

		manifest := loadPackageTargetManifestBaseline(t, root)
		for _, packagePath := range manifest.Inventory {
			for _, deleted := range deletedPaths {
				if packagePath == deleted || strings.HasPrefix(packagePath, deleted+"/") {
					t.Fatalf("package-target manifest inventory still lists deleted pipeline package %q", packagePath)
				}
			}
		}
	})

	t.Run("package_target_manifest_packages_omit_deleted_public_pipeline_packages", func(t *testing.T) {
		t.Parallel()

		manifest := loadPackageTargetManifestBaseline(t, root)
		for _, row := range manifest.Packages {
			for _, deleted := range deletedPaths {
				if row.PackagePath == deleted || strings.HasPrefix(row.PackagePath, deleted+"/") {
					t.Fatalf("package-target manifest packages still list deleted pipeline package %q", row.PackagePath)
				}
			}
		}
	})

	t.Run("package_structure_baseline_omits_deleted_public_pipeline_unexpected_directories", func(t *testing.T) {
		t.Parallel()

		entries := loadPackageStructureBaselineEntries(t, root)
		for _, entry := range entries {
			for _, deleted := range deletedPaths {
				if entry.FilePath == deleted || strings.HasPrefix(entry.FilePath, deleted+"/") {
					t.Fatalf(
						"package-structure baseline still lists deleted pipeline path %q under rule %q",
						entry.FilePath,
						entry.Rule,
					)
				}
			}
		}
	})

	t.Run("unit_coverage_minimums_omit_deleted_public_pipeline_packages", func(t *testing.T) {
		t.Parallel()
		assertCoverageMinimumsOmitDeletedPublicPipelinePackages(t, root, "go-unit-coverage-package-minimums.json")
	})

	t.Run("functional_coverage_minimums_omit_deleted_public_pipeline_packages", func(t *testing.T) {
		t.Parallel()
		assertCoverageMinimumsOmitDeletedPublicPipelinePackages(t, root, "go-functional-coverage-package-minimums.json")
	})
}

func assertCoverageMinimumsOmitDeletedPublicPipelinePackages(t *testing.T, root, fileName string) {
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
	deletedImportPaths := deletedEnginePipelinePublicImportPaths()
	for _, row := range baseline.Packages {
		if strings.HasPrefix(row.Package, factoryRuntimeModuleImportPrefix+"internal/") {
			continue
		}
		suffix := strings.TrimPrefix(row.Package, factoryRuntimeModuleImportPrefix)
		for _, deleted := range deletedImportPaths {
			deletedSuffix := strings.TrimPrefix(deleted, factoryRuntimeModuleImportPrefix)
			if suffix == deletedSuffix || strings.HasPrefix(suffix, deletedSuffix+"/") {
				t.Fatalf("%s still lists deleted public pipeline package %q", fileName, row.Package)
			}
		}
	}
}

type packageTargetManifestBaseline struct {
	Inventory []string `json:"inventory"`
	Packages  []struct {
		PackagePath string `json:"packagePath"`
	} `json:"packages"`
}

type packageStructureBaselineEntry struct {
	Rule     string `json:"rule"`
	FilePath string `json:"filePath"`
}

func loadPackageTargetManifestBaseline(t *testing.T, root string) packageTargetManifestBaseline {
	t.Helper()

	path := filepath.Join(root, "docs", "internal", "packaged-service-structure", "package-target-manifest.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var manifest packageTargetManifestBaseline
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return manifest
}

func loadPackageStructureBaselineEntries(t *testing.T, root string) []packageStructureBaselineEntry {
	t.Helper()

	path := filepath.Join(root, "docs", "internal", "baselines", "package-structure-baseline.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var baseline struct {
		Entries []packageStructureBaselineEntry `json:"entries"`
	}
	if err := json.Unmarshal(payload, &baseline); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return baseline.Entries
}
