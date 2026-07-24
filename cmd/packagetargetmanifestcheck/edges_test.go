package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMapEdgesExceptionPackageRetainsCanonicalRoot(t *testing.T) {
	t.Parallel()

	got, ok := mapEdgesExceptionPackage("pkg/services/edges")
	if !ok {
		t.Fatal("mapEdgesExceptionPackage(pkg/services/edges) ok = false")
	}
	want := PackageMapping{
		PackagePath: "pkg/services/edges",
		Disposition: DispositionRetain,
		Destination: "edges",
	}
	if got != want {
		t.Fatalf("mapEdgesExceptionPackage() = %#v, want %#v", got, want)
	}
}

func TestMapEdgesExceptionPackageRetainsChildPackages(t *testing.T) {
	t.Parallel()

	got, ok := mapEdgesExceptionPackage("pkg/services/edges/internal/fake")
	if !ok {
		t.Fatal("mapEdgesExceptionPackage(child) ok = false")
	}
	if got.Disposition != DispositionRetain || got.Destination != "edges" {
		t.Fatalf("mapEdgesExceptionPackage(child) = %#v, want retain/edges", got)
	}
}

func TestMapEdgesExceptionPackageSkipsNonEdges(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"pkg/services/work",
		"pkg/platform/clock",
		"pkg/services/edgesx",
	} {
		if _, ok := mapEdgesExceptionPackage(path); ok {
			t.Fatalf("mapEdgesExceptionPackage(%q) ok = true, want false", path)
		}
	}
}

func TestBuildEdgesExceptionPackagesCoversInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	rows, err := buildEdgesExceptionPackages(manifest.Inventory)
	if err != nil {
		t.Fatalf("buildEdgesExceptionPackages() error = %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("buildEdgesExceptionPackages() returned no rows")
	}
	for _, row := range rows {
		if row.Disposition != DispositionRetain || row.Destination != "edges" {
			t.Fatalf("edges row %#v must retain destination edges", row)
		}
		if !strings.HasPrefix(row.PackagePath, "pkg/services/edges") {
			t.Fatalf("unexpected edges row path %q", row.PackagePath)
		}
	}
}

func TestCommittedManifestRecordsEdgesAsSoleArchitectureException(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	if err := validateManifestAt(repoRoot, manifest); err != nil {
		t.Fatalf("validateManifestAt() error = %v", err)
	}
	if err := validateEdgesExceptionCoverage(manifest); err != nil {
		t.Fatalf("validateEdgesExceptionCoverage() error = %v", err)
	}

	wantNote := edgesArchitectureExceptionNote
	if got := manifest.ArchitectureExceptionNotes["edges"]; got != wantNote {
		t.Fatalf("architectureExceptionNotes.edges = %q, want %q", got, wantNote)
	}
	if len(manifest.DestinationVocabulary.ArchitectureExceptions) != 1 ||
		manifest.DestinationVocabulary.ArchitectureExceptions[0] != "edges" {
		t.Fatalf("architectureExceptions = %v, want sole [edges]", manifest.DestinationVocabulary.ArchitectureExceptions)
	}

	foundFND06 := false
	for _, debt := range manifest.FutureDebt {
		if debt.ID == "FND-06" && strings.Contains(strings.ToLower(debt.Description), "edges") {
			foundFND06 = true
			break
		}
	}
	if !foundFND06 {
		t.Fatal("futureDebt must record FND-06 Edges narrowing as deferred debt")
	}

	exceptionDestinations := map[string]struct{}{}
	for _, row := range manifest.Packages {
		root, _, ok := splitDestination(row.Destination)
		if !ok {
			continue
		}
		if _, isException := exceptionSet()[root]; isException {
			exceptionDestinations[root] = struct{}{}
			if root != "edges" {
				t.Fatalf("peer-wide external-effect exception %q is not allowed; only edges", root)
			}
			if row.Disposition != DispositionRetain {
				t.Fatalf("edges exception row %#v must retain", row)
			}
		}
	}
	if _, ok := exceptionDestinations["edges"]; !ok {
		t.Fatal("committed packages missing edges exception destination rows")
	}
}

func TestValidateManifestRequiresExactEdgesExceptionNote(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": "not the committed sole-exception statement",
		},
		FutureDebt: []FutureDebt{{
			ID:          "FND-06",
			PackagePath: "pkg/services/edges",
			Description: "Narrow Edges implementation imports; deferred to FND-06.",
		}},
		Packages: []PackageMapping{{
			PackagePath: "pkg/services/edges",
			Disposition: DispositionRetain,
			Destination: "edges",
		}},
	}
	err := validateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "architectureExceptionNotes.edges") {
		t.Fatalf("validateManifest() error = %v, want exact edges note rejection", err)
	}
}

func TestValidateManifestRequiresFND06FutureDebt(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
		Packages: []PackageMapping{{
			PackagePath: "pkg/services/edges",
			Disposition: DispositionRetain,
			Destination: "edges",
		}},
	}
	err := validateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "FND-06") {
		t.Fatalf("validateManifest() error = %v, want FND-06 future debt requirement", err)
	}
}
