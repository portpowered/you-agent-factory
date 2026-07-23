package responsestreamremovalgate_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	removalgate "github.com/portpowered/infinite-you/internal/testutil/responsestreamremovalgate"
)

func TestRepoRoot(t *testing.T) {
	root, err := removalgate.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	goMod := filepath.Join(root, "go.mod")
	if _, err := os.Stat(goMod); err != nil {
		t.Fatalf("repo root %q missing go.mod: %v", root, err)
	}
}

func TestAssertGate_RejectsEmptyRepoRoot(t *testing.T) {
	if err := removalgate.AssertGate(context.Background(), ""); err == nil {
		t.Fatal("expected empty repo root to fail")
	}
}

func TestAssertClosure_RejectsEmptyRepoRoot(t *testing.T) {
	if err := removalgate.AssertClosure(context.Background(), ""); err == nil {
		t.Fatal("expected empty repo root to fail")
	}
}

func TestAssertReleaseNotesMigrationMapping_RejectsEmptyRepoRoot(t *testing.T) {
	if err := removalgate.AssertReleaseNotesMigrationMapping(""); err == nil {
		t.Fatal("expected empty repo root to fail")
	}
}

func TestAssertReleaseNotesMigrationMapping_RejectsIncompleteNotes(t *testing.T) {
	repoRoot := t.TempDir()
	notesDir := filepath.Join(repoRoot, "docs", "release-notes")
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(notesDir, "response-stream-private-ndjson-removal.md")
	if err := os.WriteFile(path, []byte("incomplete migration note"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := removalgate.AssertReleaseNotesMigrationMapping(repoRoot)
	if err == nil || !strings.Contains(err.Error(), "migration marker") {
		t.Fatalf("expected missing migration marker error, got %v", err)
	}
}

func TestAssertLegacyCompatMapperDeleted_RejectsEmptyRepoRoot(t *testing.T) {
	if err := removalgate.AssertLegacyCompatMapperDeleted(""); err == nil {
		t.Fatal("expected empty repo root to fail")
	}
}

func TestAssertLegacyCompatMapperDeleted_RejectsRevivedCompatPackage(t *testing.T) {
	repoRoot := t.TempDir()
	compatDir := filepath.Join(repoRoot, "pkg", "factory", "sessions", "responsestream", "compat")
	if err := os.MkdirAll(compatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := removalgate.AssertLegacyCompatMapperDeleted(repoRoot)
	if err == nil || !strings.Contains(err.Error(), "legacy compat mapper package still exists") {
		t.Fatalf("expected revived compat package error, got %v", err)
	}
}

func TestAssertLegacyCompatMapperDeleted_RejectsRevivedCompatImport(t *testing.T) {
	repoRoot := t.TempDir()
	pkgDir := filepath.Join(repoRoot, "pkg", "example")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	badSource := `package example

import "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream/compat"
`
	if err := os.WriteFile(filepath.Join(pkgDir, "bad_import.go"), []byte(badSource), 0o644); err != nil {
		t.Fatal(err)
	}
	err := removalgate.AssertLegacyCompatMapperDeleted(repoRoot)
	if err == nil || !strings.Contains(err.Error(), "retired legacy compat mapper marker") {
		t.Fatalf("expected revived compat import error, got %v", err)
	}
}

func writeTempProductionSurface(t *testing.T, repoRoot string, relPath string, source string) {
	t.Helper()
	for _, relRoot := range []string{
		"pkg/transports/cli/run",
		"pkg/transports/mapping",
		"pkg/transports/http",
	} {
		dir := filepath.Join(repoRoot, filepath.FromSlash(relRoot))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	if err := os.WriteFile(target, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAssertNoPrivateNDJSONInProductionSurfaces_RejectsPrivateLiteral(t *testing.T) {
	repoRoot := t.TempDir()
	badSource := "package run\n\n// " + `"recordType":"progress"` + "\n"
	writeTempProductionSurface(t, repoRoot, "pkg/transports/cli/run/bad_emitter.go", badSource)
	err := removalgate.AssertNoPrivateNDJSONInProductionSurfaces(repoRoot)
	if err == nil || !strings.Contains(err.Error(), "private NDJSON literal") {
		t.Fatalf("expected private NDJSON literal error, got %v", err)
	}
}

func TestAssertPublicTransportLayersDoNotImportLegacyCompat_RejectsCompatImport(t *testing.T) {
	repoRoot := t.TempDir()
	badSource := `package run

import _ "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream/compat"

`
	writeTempProductionSurface(t, repoRoot, "pkg/transports/cli/run/bad_import.go", badSource)
	err := removalgate.AssertPublicTransportLayersDoNotImportLegacyCompat(repoRoot)
	if err == nil || !strings.Contains(err.Error(), "legacy compat mapper") {
		t.Fatalf("expected legacy compat import error, got %v", err)
	}
}

func TestAssertNoRetiredPrivateContractSymbolsInProductionSurfaces_RejectsSymbol(t *testing.T) {
	repoRoot := t.TempDir()
	badSource := "package run\n\n// " + `"recordType":"primary_result"` + "\n"
	writeTempProductionSurface(t, repoRoot, "pkg/transports/cli/run/bad_symbol.go", badSource)
	err := removalgate.AssertNoRetiredPrivateContractSymbolsInProductionSurfaces(repoRoot)
	if err == nil || !strings.Contains(err.Error(), "retired private-contract symbol") {
		t.Fatalf("expected retired private-contract symbol error, got %v", err)
	}
}

func TestPrivateContractRemovalGate_ConsolidatedEvidence(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertGate(context.Background(), repoRoot); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateContractRemovalGate_Batch09Closure(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertClosure(context.Background(), repoRoot); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateContractRemovalGate_DocsPrerequisite(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertDocsPrerequisite(repoRoot); err != nil {
		t.Fatal(err)
	}
}

func TestAssertDocsPrerequisite_RejectsMissingCanonicalTopic(t *testing.T) {
	err := removalgate.AssertDocsPrerequisite(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), `read canonical docs topic "run"`) {
		t.Fatalf("expected missing canonical docs topic error, got %v", err)
	}
}

func TestPrivateContractRemovalGate_NoPrivateNDJSONInProductionSurfaces(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertNoPrivateNDJSONInProductionSurfaces(repoRoot); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateContractRemovalGate_PublicTransportDoesNotImportLegacyCompat(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertPublicTransportLayersDoNotImportLegacyCompat(repoRoot); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateContractRemovalGate_ReleaseNotesMigrationMapping(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertReleaseNotesMigrationMapping(repoRoot); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateContractRemovalGate_PrivateNDJSONRecordTypesRejected(t *testing.T) {
	if err := removalgate.AssertPrivateNDJSONRecordTypesRejected(); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateContractRemovalGate_LegacyCompatMapperDeleted(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertLegacyCompatMapperDeleted(repoRoot); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateContractRemovalGate_NoRetiredPrivateContractSymbolsInProductionSurfaces(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertNoRetiredPrivateContractSymbolsInProductionSurfaces(repoRoot); err != nil {
		t.Fatal(err)
	}
}
