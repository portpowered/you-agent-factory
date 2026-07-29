package workers_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const delWrkDeleteReadyInventoryManifestRel = "docs/internal/processes/del-wrk-delete-ready-inventory.json"

type delWrkDeleteReadyInventoryManifest struct {
	DeleteReadyRelativeDirs []string              `json:"delete_ready_relative_dirs"`
	HeldBack                []delWrkHeldBackEntry `json:"held_back"`
	ExcludedFromDelete      []delWrkExcludedEntry `json:"excluded_from_delete"`
	Gates                   map[string]delWrkGate `json:"gates"`
}

type delWrkHeldBackEntry struct {
	RelativeDir string `json:"relative_dir"`
	HoldReason  string `json:"hold_reason"`
}

type delWrkExcludedEntry struct {
	RelativeDir     string `json:"relative_dir"`
	ExclusionReason string `json:"exclusion_reason"`
}

type delWrkGate struct {
	FactoryComplete bool     `json:"factory_complete"`
	ObservableProof []string `json:"observable_proof"`
}

// DEL-WRK story 001 confirms the delete-ready inventory for emptied Workers
// transitional packages after CLN-WRK-* consumption.

func TestDelWrkDeleteReadyInventoryGate_CLNWrkPrerequisitesFactoryComplete(t *testing.T) {
	t.Parallel()

	manifest := loadDelWrkDeleteReadyInventoryManifest(t)
	root := delWrkRepoRoot(t)

	for gateID, gate := range manifest.Gates {
		gateID := gateID
		gate := gate
		t.Run(gateID, func(t *testing.T) {
			t.Parallel()
			if !gate.FactoryComplete {
				t.Fatalf("%s gate must be Factory-complete before DEL-WRK deletion begins", gateID)
			}
			for _, proofPath := range gate.ObservableProof {
				proofPath := proofPath
				t.Run(proofPath, func(t *testing.T) {
					t.Parallel()
					if _, err := os.Stat(filepath.Join(root, proofPath)); err != nil {
						t.Fatalf("%s observable proof missing at %s: %v", gateID, proofPath, err)
					}
				})
			}
		})
	}
}

func TestDelWrkDeleteReadyInventoryGate_ConfirmedDeleteReadyPathsAreRemoved(t *testing.T) {
	t.Parallel()

	manifest := loadDelWrkDeleteReadyInventoryManifest(t)
	workersDir := workersRootDir(t)

	for _, relative := range manifest.DeleteReadyRelativeDirs {
		relative := relative
		t.Run(relative, func(t *testing.T) {
			t.Parallel()

			packageDir := filepath.Join(workersDir, filepath.FromSlash(relative))
			if relative == "executor" {
				goFiles, err := listGoSourceFiles(packageDir)
				if err != nil {
					if os.IsNotExist(err) {
						return
					}
					t.Fatalf("list Go sources in %s: %v", relative, err)
				}
				if len(goFiles) != 0 {
					t.Fatalf("%s Go sources = %v, want no package files after shim deletion (held-back child packages may remain)", relative, goFiles)
				}
				return
			}

			_, err := os.Stat(packageDir)
			if err == nil {
				t.Fatalf("%s must be deleted after DEL-WRK story 002", relative)
			}
			if !os.IsNotExist(err) {
				t.Fatalf("stat %s: %v", relative, err)
			}
		})
	}
}

func TestDelWrkDeleteReadyInventoryGate_ConfirmedDeleteReadyPathsHaveNoImporters(t *testing.T) {
	t.Parallel()

	manifest := loadDelWrkDeleteReadyInventoryManifest(t)
	importRoots := delWrkImportRoots(manifest.DeleteReadyRelativeDirs)

	for _, packagePath := range listModulePackages(t) {
		for _, importPath := range listDirectImports(t, packagePath) {
			if matched := matchDelWrkImportRoot(importPath, importRoots); matched != "" {
				t.Fatalf(
					"%s imports confirmed delete-ready Workers package %s; remove the import before DEL-WRK story 002 deletes %s",
					packagePath,
					importPath,
					matched,
				)
			}
		}
	}
}

func TestDelWrkDeleteReadyInventoryGate_HeldBackPathsStillHaveCallersOrAreNotShimOnly(t *testing.T) {
	t.Parallel()

	manifest := loadDelWrkDeleteReadyInventoryManifest(t)
	workersDir := workersRootDir(t)
	heldBackRoots := delWrkImportRoots(delWrkHeldBackRelativeDirs(manifest.HeldBack))
	importerIndex := delWrkBuildImporterIndex(t, heldBackRoots)

	for _, entry := range manifest.HeldBack {
		entry := entry
		t.Run(entry.RelativeDir, func(t *testing.T) {
			t.Parallel()

			packageDir := filepath.Join(workersDir, filepath.FromSlash(entry.RelativeDir))
			goFiles, err := listGoSourceFiles(packageDir)
			if err != nil {
				if os.IsNotExist(err) && entry.RelativeDir == "service" {
					return
				}
				t.Fatalf("list Go sources in %s: %v", entry.RelativeDir, err)
			}

			hasImporter := len(importerIndex[entry.RelativeDir]) > 0
			isShimOnly := len(goFiles) == 1 && goFiles[0] == "shim.go"
			if isShimOnly && !hasImporter {
				t.Fatalf(
					"%s is shim-only with zero module importers; move it into delete_ready_relative_dirs instead of held_back",
					entry.RelativeDir,
				)
			}
			if !isShimOnly && !hasImporter && entry.RelativeDir != "service" {
				t.Fatalf(
					"%s is held back but is neither shim-only nor imported; reconcile held_back inventory",
					entry.RelativeDir,
				)
			}
		})
	}
}

func TestDelWrkDeleteReadyInventoryGate_ServiceCompileShimHeldBack(t *testing.T) {
	t.Parallel()

	workersDir := workersRootDir(t)
	serviceDir := filepath.Join(workersDir, "service")
	goFiles, err := listGoSourceFiles(serviceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("list Go sources in service/: %v", err)
	}
	if len(goFiles) <= 1 {
		t.Fatalf("service/ Go sources = %v, want multi-file owner-local compile shim held until retarget completes", goFiles)
	}
	if !slices.Contains(goFiles, "service.go") {
		t.Fatalf("service/ Go sources = %v, want service.go implementation retained", goFiles)
	}
}

func TestDelWrkDeleteReadyInventoryGate_ProvidersExtractionSourcesRemoved(t *testing.T) {
	t.Parallel()

	manifest := loadDelWrkDeleteReadyInventoryManifest(t)
	workersDir := workersRootDir(t)

	for _, relative := range providersExtractionTopLevelDirs {
		if delWrkManifestExcludesRelativeDir(manifest, relative) {
			t.Fatalf("manifest must not exclude migrated Providers source %q", relative)
		}
		if _, err := os.Stat(filepath.Join(workersDir, relative)); !os.IsNotExist(err) {
			t.Fatalf("Providers extraction source %q must be removed: %v", relative, err)
		}
	}
}

func TestDelWrkDeleteReadyInventoryGate_InternalServicesExcluded(t *testing.T) {
	t.Parallel()

	manifest := loadDelWrkDeleteReadyInventoryManifest(t)
	if !delWrkManifestExcludesRelativeDir(manifest, "internal") {
		t.Fatal("manifest must exclude workers/internal from delete set")
	}

	root := delWrkRepoRoot(t)
	wantSubservices := []string{"runners", "runtime_assembly", "workstations"}
	subservicesRoot := filepath.Join(root, "pkg", "services", "workers", "internal", "services")
	entries, err := os.ReadDir(subservicesRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", subservicesRoot, err)
	}
	var gotSubservices []string
	for _, entry := range entries {
		if entry.IsDir() {
			gotSubservices = append(gotSubservices, entry.Name())
		}
	}
	slices.Sort(gotSubservices)
	slices.Sort(wantSubservices)
	if !slices.Equal(gotSubservices, wantSubservices) {
		t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
	}
}

func TestDelWrkDeleteReadyInventoryGate_InventoryMatchesFoldedLegacyShimDirs(t *testing.T) {
	t.Parallel()

	manifest := loadDelWrkDeleteReadyInventoryManifest(t)
	inventoried := delWrkHeldBackRelativeDirs(manifest.HeldBack)
	slices.Sort(inventoried)

	want := slices.Clone(foldedLegacyShimPackageDirs)
	slices.Sort(want)

	if !slices.Equal(inventoried, want) {
		t.Fatalf("manifest inventoried shim dirs = %v, want %v", inventoried, want)
	}
}

func delWrkImportRoots(relativeDirs []string) map[string]string {
	roots := make(map[string]string, len(relativeDirs))
	for _, relative := range relativeDirs {
		roots[relative] = workersOwnerPrefix + "/" + relative
	}
	return roots
}

func delWrkHeldBackRelativeDirs(entries []delWrkHeldBackEntry) []string {
	relativeDirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		relativeDirs = append(relativeDirs, entry.RelativeDir)
	}
	return relativeDirs
}

func delWrkManifestExcludesRelativeDir(manifest delWrkDeleteReadyInventoryManifest, relative string) bool {
	for _, entry := range manifest.ExcludedFromDelete {
		if entry.RelativeDir == relative {
			return true
		}
	}
	return false
}

func delWrkBuildImporterIndex(t *testing.T, importRoots map[string]string) map[string][]string {
	t.Helper()

	index := make(map[string][]string, len(importRoots))
	for relative := range importRoots {
		index[relative] = nil
	}

	for _, packagePath := range listModulePackages(t) {
		for _, importPath := range listDirectImports(t, packagePath) {
			if matched := matchDelWrkImportRoot(importPath, importRoots); matched != "" {
				index[matched] = append(index[matched], packagePath)
			}
		}
	}
	return index
}

func matchDelWrkImportRoot(importPath string, importRoots map[string]string) string {
	for relative, root := range importRoots {
		if importPath == root {
			return relative
		}
	}
	return ""
}

func loadDelWrkDeleteReadyInventoryManifest(t *testing.T) delWrkDeleteReadyInventoryManifest {
	t.Helper()

	root := delWrkRepoRoot(t)
	path := filepath.Join(root, delWrkDeleteReadyInventoryManifestRel)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read delete-ready inventory manifest %s: %v", path, err)
	}
	var manifest delWrkDeleteReadyInventoryManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode delete-ready inventory manifest: %v", err)
	}
	return manifest
}

func delWrkRepoRoot(t *testing.T) string {
	t.Helper()

	root, err := ownershipinventory.FindRepositoryRoot()
	if err != nil {
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatalf("FindRepositoryRoot() error = %v", err)
		}
		return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	}
	return root
}
