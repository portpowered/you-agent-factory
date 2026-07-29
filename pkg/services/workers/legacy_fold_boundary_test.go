package workers_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// foldedLegacyWorkersImportRoots are transitional public Workers packages folded
// into private destinations by CLN-WRK-LEGACY-PACKAGES.
var foldedLegacyWorkersImportRoots = []string{
	workersOwnerPrefix + "/process",
	workersOwnerPrefix + "/runner",
	workersOwnerPrefix + "/executor/agentrun",
	workersOwnerPrefix + "/invocation",
	workersOwnerPrefix + "/prompting",
	workersOwnerPrefix + "/worktree",
	workersOwnerPrefix + "/services/testing",
}

// allowedFoldedLegacyImporterPrefixes lists package trees that may still import
// folded legacy public Workers packages through documented delete-ready shims.
var allowedFoldedLegacyImporterPrefixes = []string{
	workersOwnerPrefix + "/process",
	workersOwnerPrefix + "/runner",
	workersOwnerPrefix + "/executor/agentrun",
	workersOwnerPrefix + "/invocation",
	workersOwnerPrefix + "/prompting",
	workersOwnerPrefix + "/worktree",
	workersOwnerPrefix + "/services/testing",
	workersOwnerPrefix + "/internal",
	workersOwnerPrefix + "/wire",
	workersOwnerPrefix + "/provider",
	modulePrefix + "pkg/wire",
	modulePrefix + "pkg/services/providers",
}

var foldedLegacyShimPackageDirs = []string{}

var providersExtractionTopLevelDirs = []string{"provider", "agypty", "provider_test"}

// TestProductionPackagesDoNotImportFoldedLegacyWorkersPackages seals the folded
// legacy import boundary: only documented delete-ready shims, owner-private
// composition, Providers extraction sources, root wire, and the narrow
// Providers service execution edge may depend on transitional public paths.
func TestProductionPackagesDoNotImportFoldedLegacyWorkersPackages(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listModulePackages(t) {
		if isAllowedFoldedLegacyImporter(packagePath) {
			continue
		}
		for _, importPath := range listDirectImports(t, packagePath) {
			if isFoldedLegacyWorkersImport(importPath) {
				t.Fatalf(
					"%s must not import folded legacy Workers package %s; reach Workers through %spkg/services/workers/wire or private owner composition",
					packagePath,
					importPath,
					modulePrefix,
				)
			}
		}
	}
}

// TestFoldedLegacyShimPackagesAreDeleteReady proves no transitional public
// shim packages remain after the final migration.
func TestFoldedLegacyShimPackagesAreDeleteReady(t *testing.T) {
	t.Parallel()

	workersDir := workersRootDir(t)
	for _, relative := range foldedLegacyShimPackageDirs {
		t.Run(relative, func(t *testing.T) {
			t.Parallel()

			packageDir := filepath.Join(workersDir, filepath.FromSlash(relative))
			goFiles, err := listGoSourceFiles(packageDir)
			if err != nil {
				t.Fatalf("list Go sources in %s: %v", relative, err)
			}
			if len(goFiles) != 1 || goFiles[0] != "shim.go" {
				t.Fatalf("%s Go sources = %v, want only shim.go", relative, goFiles)
			}
		})
	}
}

// TestProvidersExtractionSourcesMovedOutOfWorkers proves the final Providers
// extraction left no peer-owned implementation below Workers.
func TestProvidersExtractionSourcesMovedOutOfWorkers(t *testing.T) {
	t.Parallel()

	workersDir := workersRootDir(t)
	for _, relative := range providersExtractionTopLevelDirs {
		if _, err := os.Stat(filepath.Join(workersDir, relative)); !os.IsNotExist(err) {
			t.Fatalf("providers extraction source %q must be absent after final migration: %v", relative, err)
		}
	}

	internalDir := filepath.Join(workersDir, "internal")
	for _, forbidden := range []string{"provider", "agypty", "cliprovider"} {
		if _, err := os.Stat(filepath.Join(internalDir, forbidden)); err == nil {
			t.Fatalf("workers/internal/%s must not exist; Providers extraction sources stay top-level", forbidden)
		}
	}
}

func isAllowedFoldedLegacyImporter(packagePath string) bool {
	for _, prefix := range allowedFoldedLegacyImporterPrefixes {
		if packagePath == prefix || strings.HasPrefix(packagePath, prefix+"/") {
			return true
		}
	}
	return false
}

func isFoldedLegacyWorkersImport(importPath string) bool {
	for _, root := range foldedLegacyWorkersImportRoots {
		if importPath == root || strings.HasPrefix(importPath, root+"/") {
			return true
		}
	}
	return false
}

func workersRootDir(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename)))
}

func listGoSourceFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}
