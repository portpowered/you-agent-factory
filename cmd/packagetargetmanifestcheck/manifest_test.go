package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestDerivedVocabularyMatchesTheLiveServicesDirectory proves the product-owner
// half of the vocabulary is read off the tree. The expectation is built by
// listing pkg/services here, so a service added to the repository shows up in the
// checker with no edit to this file or to the checker.
func TestDerivedVocabularyMatchesTheLiveServicesDirectory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	vocab, err := derivedDestinationVocabulary(repoRoot)
	if err != nil {
		t.Fatalf("derivedDestinationVocabulary() error = %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(repoRoot, filepath.FromSlash(servicesRootRelative)))
	if err != nil {
		t.Fatalf("read services root: %v", err)
	}
	var wantOwners []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "edges" {
			continue
		}
		wantOwners = append(wantOwners, entry.Name())
	}
	slices.Sort(wantOwners)
	if len(wantOwners) == 0 {
		t.Fatal("expected the live pkg/services tree to contain services")
	}

	assertExactStrings(t, "productOwners", vocab.ProductOwners, wantOwners)
	assertExactStrings(t, "nonServiceFamilies", vocab.NonServiceFamilies, []string{
		"initializer", "platform", "root", "transports", "wire",
	})
	assertExactStrings(t, "architectureExceptions", vocab.ArchitectureExceptions, []string{"edges"})
}

// testVocabulary is the derived-shaped vocabulary schema-level unit tests run
// against, so they exercise validation rules rather than the live tree.
func testVocabulary() DestinationVocabulary {
	return DestinationVocabulary{
		ProductOwners:          []string{"factory_definitions", "recordings", "work", "workers"},
		NonServiceFamilies:     []string{"initializer", "platform", "root", "transports", "wire"},
		ArchitectureExceptions: []string{"edges"},
	}
}

func TestValidateMoveLedgerAcceptsMoveAndNestedOwnerDestination(t *testing.T) {
	t.Parallel()

	moves := moveLedgerFixture(
		PackageMapping{
			PackagePath: "pkg/services/work/content_staging",
			Destination: "work/internal/services/content_staging",
			Successor:   "pkg/services/work/internal/services/content_staging",
		},
		PackageMapping{
			PackagePath: "pkg/services/work/legacy_lineage",
			Destination: "work/internal",
			Successor:   "pkg/services/work/internal",
		},
	)

	if err := validateUnfinishedMovesSchema(moves, testVocabulary()); err != nil {
		t.Fatalf("validateUnfinishedMovesSchema() error = %v", err)
	}
}

func TestValidateMoveLedgerRejectsDestinationOutsideClosedSet(t *testing.T) {
	t.Parallel()

	moves := moveLedgerFixture(PackageMapping{
		PackagePath: "pkg/services/mystery",
		Destination: "mystery_service",
		Successor:   "pkg/services/mystery_service",
	})

	err := validateUnfinishedMovesSchema(moves, testVocabulary())
	if err == nil {
		t.Fatal("validateUnfinishedMovesSchema() error = nil, want closed-set rejection")
	}
	if !strings.Contains(err.Error(), "mystery_service") {
		t.Fatalf("validateUnfinishedMovesSchema() error = %v, want destination name", err)
	}
	if !strings.Contains(err.Error(), "outside closed destination set") {
		t.Fatalf("validateUnfinishedMovesSchema() error = %v, want closed-set message", err)
	}
}

func TestValidateMoveLedgerRejectsNestedDestinationWithUnknownOwner(t *testing.T) {
	t.Parallel()

	moves := moveLedgerFixture(PackageMapping{
		PackagePath: "pkg/services/work/legacy",
		Destination: "mystery/internal/services/admission",
		Successor:   "pkg/services/mystery/internal/services/admission",
	})

	err := validateUnfinishedMovesSchema(moves, testVocabulary())
	if err == nil {
		t.Fatal("validateUnfinishedMovesSchema() error = nil, want nested owner rejection")
	}
	if !strings.Contains(err.Error(), "mystery") {
		t.Fatalf("validateUnfinishedMovesSchema() error = %v, want unknown owner", err)
	}
}

// An open move has to say where the package goes. Without a successor the row
// records intent that can never be closed out.
func TestValidateMoveLedgerRequiresSuccessor(t *testing.T) {
	t.Parallel()

	moves := moveLedgerFixture(PackageMapping{
		PackagePath: "pkg/services/work/legacy_lineage",
		Destination: "work/internal",
	})

	err := validateUnfinishedMovesSchema(moves, testVocabulary())
	if err == nil || !strings.Contains(err.Error(), "successor is required") {
		t.Fatalf("missing successor error = %v, want successor requirement", err)
	}
}

// The destination bucket and the successor path describe the same target, so a
// successor that lands under a different owner is a contradiction.
func TestValidateMoveLedgerRejectsSuccessorOutsideDestinationOwner(t *testing.T) {
	t.Parallel()

	moves := moveLedgerFixture(PackageMapping{
		PackagePath: "pkg/services/work/legacy_lineage",
		Destination: "work/internal",
		Successor:   "pkg/services/recordings/internal",
	})

	err := validateUnfinishedMovesSchema(moves, testVocabulary())
	if err == nil || !strings.Contains(err.Error(), "must sit under pkg/services/work") {
		t.Fatalf("successor owner mismatch error = %v, want owner-root requirement", err)
	}
}

// The ledger's end state is zero rows and then no file at all, so an empty
// ledger has to validate rather than trip the schema.
func TestValidateMoveLedgerAcceptsEmptyLedger(t *testing.T) {
	t.Parallel()

	if err := validateUnfinishedMovesSchema(UnfinishedMoves{}, testVocabulary()); err != nil {
		t.Fatalf("validateUnfinishedMovesSchema() on an empty ledger error = %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getcwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repository root from test working directory")
		}
		dir = parent
	}
}

func assertExactStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d (%v)", label, len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %q, want %q", label, i, got[i], want[i])
		}
	}
}

// committedMoveLedgerRows loads the committed open-move ledger keyed by package
// path. Every row in it is an open move; a package that is settled where it
// already lives carries no row at all.
func committedMoveLedgerRows(t *testing.T, repoRoot string) map[string]PackageMapping {
	t.Helper()
	moves, err := loadUnfinishedMoves(filepath.Join(repoRoot, unfinishedMovesRelativePath))
	if err != nil {
		t.Fatalf("loadUnfinishedMoves() error = %v", err)
	}
	byPath := make(map[string]PackageMapping, len(moves.Moves))
	for _, row := range moves.Moves {
		byPath[row.PackagePath] = row
	}
	return byPath
}

// moveLedgerFixture builds a schema-valid open-move ledger around the supplied
// rows so each test only has to state the row property under test.
func moveLedgerFixture(rows ...PackageMapping) UnfinishedMoves {
	return UnfinishedMoves{
		Version: 1,
		Stage:   unfinishedMovesStage,
		Moves:   rows,
	}
}
