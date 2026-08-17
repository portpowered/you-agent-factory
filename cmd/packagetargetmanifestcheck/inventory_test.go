package main

import (
	"bytes"
	"encoding/json"
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
		t.Fatalf("re-running package discovery changed the ordered package list")
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

// TestCheckPassesWhenPackageIsAddedInsideExistingServiceWithNoManifestEdit is the
// core of the registration-tax retirement: package churn inside a service must
// not require a manifest edit. The check runs before and after a new package
// appears, against a byte-identical manifest.
func TestCheckPassesWhenPackageIsAddedInsideExistingServiceWithNoManifestEdit(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoPackage(t, repoRoot, "pkg/services/work", "package work\n")
	writeGoPackage(t, repoRoot, "pkg/services/work/internal/lineagegraph", "package lineagegraph\n")
	manifestPath := writeFixtureManifest(t, repoRoot, []PackageMapping{{
		PackagePath: "pkg/services/work/internal/lineagegraph",
		Disposition: DispositionMove,
		Destination: "work/internal",
	}})
	before := readFileBytes(t, manifestPath)

	if err := runCheck(t, repoRoot); err != nil {
		t.Fatalf("check on the starting tree error = %v", err)
	}

	// A brand-new package inside an existing service, with no registry edit.
	writeGoPackage(t, repoRoot, "pkg/services/work/internal/newsubsystem", "package newsubsystem\n")
	if err := runCheck(t, repoRoot); err != nil {
		t.Fatalf("check after adding a package with no manifest edit error = %v", err)
	}

	// Deleting a package that never had a row is likewise a no-edit change.
	if err := os.RemoveAll(filepath.Join(repoRoot, filepath.FromSlash("pkg/services/work/internal/newsubsystem"))); err != nil {
		t.Fatalf("remove added package: %v", err)
	}
	if err := runCheck(t, repoRoot); err != nil {
		t.Fatalf("check after deleting a package with no manifest edit error = %v", err)
	}

	if after := readFileBytes(t, manifestPath); !bytes.Equal(before, after) {
		t.Fatal("the check rewrote the manifest; it must be read-only")
	}
}

// TestCheckFailsWhenMoveRowNamesAbsentPackage proves the surviving rows cannot
// rot: completeness is retired in one direction only.
func TestCheckFailsWhenMoveRowNamesAbsentPackage(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoPackage(t, repoRoot, "pkg/services/work", "package work\n")
	writeFixtureManifest(t, repoRoot, []PackageMapping{{
		PackagePath: "pkg/services/work/internal/alreadymoved",
		Disposition: DispositionMove,
		Destination: "work/internal",
	}})

	err := runCheck(t, repoRoot)
	if err == nil {
		t.Fatal("check error = nil, want failure for a row naming an absent package")
	}
	if !strings.Contains(err.Error(), "pkg/services/work/internal/alreadymoved") {
		t.Fatalf("check error = %v, want the stale package path named", err)
	}
	if !strings.Contains(err.Error(), "LINT_VIOLATION_COUNT: 1") {
		t.Fatalf("check error = %v, want a machine-readable violation count", err)
	}
}

// TestCheckRejectsRetainDisposition locks the retirement in place: a row that
// only restates where a package already lives is no longer accepted, so the
// enumeration cannot creep back one row at a time.
func TestCheckRejectsRetainDisposition(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoPackage(t, repoRoot, "pkg/services/work", "package work\n")
	writeFixtureManifest(t, repoRoot, []PackageMapping{{
		PackagePath: "pkg/services/work",
		Disposition: DispositionRetain,
		Destination: "work",
	}})

	err := runCheck(t, repoRoot)
	if err == nil {
		t.Fatal("check error = nil, want retain disposition rejection")
	}
	if !strings.Contains(err.Error(), "retired") {
		t.Fatalf("check error = %v, want a retired-disposition message", err)
	}
}

// TestCommittedManifestPassesTheCheck runs the real committed manifest against
// the real tree, which is what the packaged-service-structure lint target does.
func TestCommittedManifestPassesTheCheck(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := runCheck(t, repoRoot); err != nil {
		t.Fatalf("committed manifest check error = %v", err)
	}
}

func runCheck(t *testing.T, repoRoot string) error {
	t.Helper()
	return run(config{root: repoRoot, manifestPath: manifestRelativePath}, &bytes.Buffer{}, &bytes.Buffer{})
}

// writeFixtureManifest writes a schema-valid manifest carrying only the given
// rows and returns its path.
func writeFixtureManifest(t *testing.T, repoRoot string, rows []PackageMapping) string {
	t.Helper()
	manifest := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
		FutureDebt: []FutureDebt{edgesFutureDebtEntry()},
		Packages:   rows,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode fixture manifest: %v", err)
	}
	path := filepath.Join(repoRoot, filepath.FromSlash(manifestRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture manifest dir: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write fixture manifest: %v", err)
	}
	return path
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
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
