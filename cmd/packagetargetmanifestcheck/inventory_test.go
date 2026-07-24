package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestListProductionPkgPackagesIsStableSortedAndIdempotent(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	first, err := listProductionPkgPackages(repoRoot)
	if err != nil {
		t.Fatalf("listProductionPkgPackages() error = %v", err)
	}
	if len(first) == 0 {
		t.Fatal("listProductionPkgPackages() returned no packages")
	}
	for _, pkgPath := range first {
		if !strings.HasPrefix(pkgPath, "pkg/") {
			t.Fatalf("package path %q must be repository-relative under pkg/", pkgPath)
		}
		if strings.Contains(pkgPath, "\\") {
			t.Fatalf("package path %q must use slash separators", pkgPath)
		}
	}
	if !slices.IsSorted(first) {
		t.Fatalf("listProductionPkgPackages() is not byte-order sorted: first unsorted pair near %q", first[unsortedIndex(first)])
	}

	second, err := listProductionPkgPackages(repoRoot)
	if err != nil {
		t.Fatalf("second listProductionPkgPackages() error = %v", err)
	}
	if !slices.Equal(first, second) {
		t.Fatalf("re-running inventory generation changed the ordered package list")
	}
}

func TestListProductionPkgPackagesIncludesProductionAndOmitsTestOnly(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoPackage(t, repoRoot, "pkg/alpha", "package alpha\n")
	writeGoPackage(t, repoRoot, "pkg/beta", "package beta_test\n", "beta_test.go")
	writeGoPackage(t, repoRoot, "pkg/gamma/nested", "package nested\n")
	writeGoPackage(t, repoRoot, "pkg/testdata/ignored", "package ignored\n")

	got, err := listProductionPkgPackages(repoRoot)
	if err != nil {
		t.Fatalf("listProductionPkgPackages() error = %v", err)
	}
	want := []string{"pkg/alpha", "pkg/gamma/nested"}
	if !slices.Equal(got, want) {
		t.Fatalf("listProductionPkgPackages() = %v, want %v", got, want)
	}
}

func TestValidateManifestRequiresCompleteStableInventory(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoPackage(t, repoRoot, "pkg/alpha", "package alpha\n")
	writeGoPackage(t, repoRoot, "pkg/beta", "package beta\n")

	base := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
	}

	missing := base
	missing.Inventory = []string{"pkg/alpha"}
	if err := validateManifestAt(repoRoot, missing); err == nil || !strings.Contains(err.Error(), "inventory") {
		t.Fatalf("incomplete inventory error = %v", err)
	}

	unsorted := base
	unsorted.Inventory = []string{"pkg/beta", "pkg/alpha"}
	if err := validateManifestAt(repoRoot, unsorted); err == nil || !strings.Contains(err.Error(), "stable-sorted") {
		t.Fatalf("unsorted inventory error = %v", err)
	}

	complete := base
	complete.Inventory = []string{"pkg/alpha", "pkg/beta"}
	if err := validateManifestAt(repoRoot, complete); err != nil {
		t.Fatalf("complete inventory error = %v", err)
	}
}

func TestCommittedManifestInventoryMatchesLiveProductionPackages(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	if err := validateManifestAt(repoRoot, manifest); err != nil {
		t.Fatalf("committed manifest inventory validation error = %v", err)
	}
	live, err := listProductionPkgPackages(repoRoot)
	if err != nil {
		t.Fatalf("listProductionPkgPackages() error = %v", err)
	}
	if !slices.Equal(manifest.Inventory, live) {
		t.Fatalf("committed inventory diverges from live production packages (manifest=%d live=%d)", len(manifest.Inventory), len(live))
	}
	if len(manifest.Inventory) == 0 {
		t.Fatal("committed inventory is empty")
	}
}

func writeGoPackage(t *testing.T, repoRoot, packagePath, source string, fileName ...string) {
	t.Helper()
	name := "doc.go"
	if len(fileName) > 0 {
		name = fileName[0]
	}
	dir := filepath.Join(repoRoot, filepath.FromSlash(packagePath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func unsortedIndex(values []string) int {
	for i := 1; i < len(values); i++ {
		if values[i-1] > values[i] {
			return i
		}
	}
	return 0
}
