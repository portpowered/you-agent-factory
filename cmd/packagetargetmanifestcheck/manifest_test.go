package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClosedDestinationVocabularyMatchesCommittedOwners(t *testing.T) {
	t.Parallel()

	vocab := closedDestinationVocabulary()

	wantOwners := []string{
		"factory_definitions",
		"factory_sessions",
		"factory_runtime",
		"work",
		"workers",
		"providers",
		"provider_sessions",
		"models",
		"automations",
		"recordings",
		"factory_visualization",
		"operator_settings",
		"system_initialization",
		"chat_sessions",
		"events",
		"worker_sessions",
		"webhooks",
	}
	wantFamilies := []string{
		"initializer",
		"root",
		"wire",
		"platform",
		"transports",
	}
	wantExceptions := []string{"edges"}

	assertExactStrings(t, "productOwners", vocab.ProductOwners, wantOwners)
	assertExactStrings(t, "nonServiceFamilies", vocab.NonServiceFamilies, wantFamilies)
	assertExactStrings(t, "architectureExceptions", vocab.ArchitectureExceptions, wantExceptions)
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

	if err := validateUnfinishedMovesSchema(moves); err != nil {
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

	err := validateUnfinishedMovesSchema(moves)
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

	err := validateUnfinishedMovesSchema(moves)
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

	err := validateUnfinishedMovesSchema(moves)
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

	err := validateUnfinishedMovesSchema(moves)
	if err == nil || !strings.Contains(err.Error(), "must sit under pkg/services/work") {
		t.Fatalf("successor owner mismatch error = %v, want owner-root requirement", err)
	}
}

// The ledger's end state is zero rows and then no file at all, so an empty
// ledger has to validate rather than trip the schema.
func TestValidateMoveLedgerAcceptsEmptyLedger(t *testing.T) {
	t.Parallel()

	if err := validateUnfinishedMovesSchema(UnfinishedMoves{}); err != nil {
		t.Fatalf("validateUnfinishedMovesSchema() on an empty ledger error = %v", err)
	}
}

func TestValidateManifestRejectsAlteredVocabulary(t *testing.T) {
	t.Parallel()

	vocab := closedDestinationVocabulary()
	vocab.ProductOwners = append(append([]string{}, vocab.ProductOwners...), "new_owner")
	manifest := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: vocab,
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
		FutureDebt: []FutureDebt{edgesFutureDebtEntry()},
	}

	err := validateManifest(manifest)
	if err == nil {
		t.Fatal("validateManifest() error = nil, want vocabulary rejection")
	}
	if !strings.Contains(err.Error(), "destination vocabulary") {
		t.Fatalf("validateManifest() error = %v, want vocabulary message", err)
	}
}

func TestCommittedManifestDeclaresClosedVocabulary(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	moves, err := loadUnfinishedMoves(filepath.Join(repoRoot, unfinishedMovesRelativePath))
	if err != nil {
		t.Fatalf("loadUnfinishedMoves() error = %v", err)
	}
	if err := validateManifestAt(repoRoot, manifest, moves); err != nil {
		t.Fatalf("committed manifest validateManifestAt() error = %v", err)
	}
	assertExactStrings(t, "productOwners", manifest.DestinationVocabulary.ProductOwners, closedDestinationVocabulary().ProductOwners)
	assertExactStrings(t, "nonServiceFamilies", manifest.DestinationVocabulary.NonServiceFamilies, closedDestinationVocabulary().NonServiceFamilies)
	assertExactStrings(t, "architectureExceptions", manifest.DestinationVocabulary.ArchitectureExceptions, closedDestinationVocabulary().ArchitectureExceptions)
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

// edgesFutureDebtEntry is the FND-06 fixture the schema requires. It lives in
// the test now that the checker no longer rewrites the committed manifest.
func edgesFutureDebtEntry() FutureDebt {
	return FutureDebt{
		ID:          edgesFutureDebtID,
		PackagePath: "pkg/services/edges",
		Description: "Narrow Edges implementation imports and the broad external-effect surface; deferred to FND-06. This packet only records the architecture exception.",
	}
}
