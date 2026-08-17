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

// TestCheckPassesWhenPackageIsAddedInsideExistingServiceWithNoRegistryEdit is the
// core of the registration-tax retirement: package churn inside a service must
// not require a registry edit. The check runs before and after a new package
// appears, against byte-identical registry files.
func TestCheckPassesWhenPackageIsAddedInsideExistingServiceWithNoRegistryEdit(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoPackage(t, repoRoot, "pkg/services/work", "package work\n")
	writeGoPackage(t, repoRoot, "pkg/services/work/internal/lineagegraph", "package lineagegraph\n")
	manifestPath, movesPath := writeFixtureRegistries(t, repoRoot, []PackageMapping{{
		PackagePath: "pkg/services/work/internal/lineagegraph",
		Destination: "work/internal",
		Successor:   "pkg/services/work/internal",
	}})
	manifestBefore := readFileBytes(t, manifestPath)
	movesBefore := readFileBytes(t, movesPath)

	if err := runCheck(t, repoRoot); err != nil {
		t.Fatalf("check on the starting tree error = %v", err)
	}

	// A brand-new package inside an existing service, with no registry edit.
	writeGoPackage(t, repoRoot, "pkg/services/work/internal/newsubsystem", "package newsubsystem\n")
	if err := runCheck(t, repoRoot); err != nil {
		t.Fatalf("check after adding a package with no registry edit error = %v", err)
	}

	// Deleting a package that never had a row is likewise a no-edit change.
	if err := os.RemoveAll(filepath.Join(repoRoot, filepath.FromSlash("pkg/services/work/internal/newsubsystem"))); err != nil {
		t.Fatalf("remove added package: %v", err)
	}
	if err := runCheck(t, repoRoot); err != nil {
		t.Fatalf("check after deleting a package with no registry edit error = %v", err)
	}

	if after := readFileBytes(t, manifestPath); !bytes.Equal(manifestBefore, after) {
		t.Fatal("the check rewrote the manifest; it must be read-only")
	}
	if after := readFileBytes(t, movesPath); !bytes.Equal(movesBefore, after) {
		t.Fatal("the check rewrote the move ledger; it must be read-only")
	}
}

// TestCheckFailsWhenMoveRowNamesAbsentPackage proves the surviving rows cannot
// rot: completeness is retired in one direction only.
func TestCheckFailsWhenMoveRowNamesAbsentPackage(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoPackage(t, repoRoot, "pkg/services/work", "package work\n")
	writeFixtureRegistries(t, repoRoot, []PackageMapping{{
		PackagePath: "pkg/services/work/internal/alreadymoved",
		Destination: "work/internal",
		Successor:   "pkg/services/work/internal",
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

// TestCheckRejectsResurrectedRetainRow locks the retirement in place. An
// old-style row that only restates where a package already lives carries no
// successor, so pasting the retired enumeration back in fails the check rather
// than passing as a no-op.
func TestCheckRejectsResurrectedRetainRow(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoPackage(t, repoRoot, "pkg/services/work", "package work\n")
	writeFixtureRegistries(t, repoRoot, nil)
	writeRawMoveLedger(t, repoRoot, `{
  "version": 1,
  "stage": "`+unfinishedMovesStage+`",
  "moves": [
    {
      "packagePath": "pkg/services/work",
      "disposition": "retain",
      "destination": "work"
    }
  ]
}`)

	err := runCheck(t, repoRoot)
	if err == nil {
		t.Fatal("check error = nil, want rejection of a rowless retain restatement")
	}
	if !strings.Contains(err.Error(), "successor is required") {
		t.Fatalf("check error = %v, want a missing-successor message", err)
	}
}

// TestCommittedManifestPassesTheCheck runs the real committed registries against
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
	return run(config{
		root:         repoRoot,
		manifestPath: manifestRelativePath,
		movesPath:    unfinishedMovesRelativePath,
	}, &bytes.Buffer{}, &bytes.Buffer{})
}

// writeFixtureRegistries writes a schema-valid manifest plus an open-move ledger
// carrying only the given rows, and returns both paths.
func writeFixtureRegistries(t *testing.T, repoRoot string, rows []PackageMapping) (manifestPath, movesPath string) {
	t.Helper()
	manifest := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
		FutureDebt: []FutureDebt{edgesFutureDebtEntry()},
	}
	moves := UnfinishedMoves{
		Version: 1,
		Stage:   unfinishedMovesStage,
		Moves:   rows,
	}
	return writeFixtureJSON(t, repoRoot, manifestRelativePath, manifest),
		writeFixtureJSON(t, repoRoot, unfinishedMovesRelativePath, moves)
}

func writeFixtureJSON(t *testing.T, repoRoot, relativePath string, value any) string {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode fixture %s: %v", relativePath, err)
	}
	path := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir for %s: %v", relativePath, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", relativePath, err)
	}
	return path
}

// writeRawMoveLedger writes ledger JSON verbatim so a test can express a shape
// the Go structs no longer model, such as a resurrected retain row.
func writeRawMoveLedger(t *testing.T, repoRoot, payload string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(unfinishedMovesRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir move ledger dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(payload+"\n"), 0o644); err != nil {
		t.Fatalf("write raw move ledger: %v", err)
	}
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
