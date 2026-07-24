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

func TestValidateManifestAcceptsRetainMoveAndNestedOwnerDestination(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
		Packages: []PackageMapping{
			{
				PackagePath: "pkg/services/work",
				Disposition: DispositionRetain,
				Destination: "work",
			},
			{
				PackagePath: "pkg/services/work/content_staging",
				Disposition: DispositionMove,
				Destination: "work/internal/services/content_staging",
			},
			{
				PackagePath: "pkg/services/edges",
				Disposition: DispositionRetain,
				Destination: "edges",
			},
		},
	}

	if err := validateManifest(manifest); err != nil {
		t.Fatalf("validateManifest() error = %v", err)
	}
}

func TestValidateManifestRejectsDestinationOutsideClosedSet(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
		Packages: []PackageMapping{
			{
				PackagePath: "pkg/services/mystery",
				Disposition: DispositionRetain,
				Destination: "mystery_service",
			},
		},
	}

	err := validateManifest(manifest)
	if err == nil {
		t.Fatal("validateManifest() error = nil, want closed-set rejection")
	}
	if !strings.Contains(err.Error(), "mystery_service") {
		t.Fatalf("validateManifest() error = %v, want destination name", err)
	}
	if !strings.Contains(err.Error(), "outside closed destination set") {
		t.Fatalf("validateManifest() error = %v, want closed-set message", err)
	}
}

func TestValidateManifestRejectsNestedDestinationWithUnknownOwner(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
		Packages: []PackageMapping{
			{
				PackagePath: "pkg/services/work/legacy",
				Disposition: DispositionMove,
				Destination: "mystery/internal/services/admission",
			},
		},
	}

	err := validateManifest(manifest)
	if err == nil {
		t.Fatal("validateManifest() error = nil, want nested owner rejection")
	}
	if !strings.Contains(err.Error(), "mystery") {
		t.Fatalf("validateManifest() error = %v, want unknown owner", err)
	}
}

func TestValidateManifestRequiresDeletionSuccessorAndCondition(t *testing.T) {
	t.Parallel()

	base := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
	}

	missingBoth := base
	missingBoth.Packages = []PackageMapping{{
		PackagePath: "pkg/legacy",
		Disposition: DispositionDelete,
		Destination: "work",
	}}
	if err := validateManifest(missingBoth); err == nil || !strings.Contains(err.Error(), "deletionSuccessor") {
		t.Fatalf("missing deletion fields error = %v", err)
	}

	missingCondition := base
	missingCondition.Packages = []PackageMapping{{
		PackagePath:       "pkg/legacy",
		Disposition:       DispositionDelete,
		Destination:       "work",
		DeletionSuccessor: "pkg/services/work",
	}}
	if err := validateManifest(missingCondition); err == nil || !strings.Contains(err.Error(), "deletionCondition") {
		t.Fatalf("missing deletionCondition error = %v", err)
	}

	validDelete := base
	validDelete.Packages = []PackageMapping{{
		PackagePath:       "pkg/legacy",
		Disposition:       DispositionDelete,
		Destination:       "work",
		DeletionSuccessor: "pkg/services/work",
		DeletionCondition: "delete when no importers remain",
	}}
	if err := validateManifest(validDelete); err != nil {
		t.Fatalf("valid delete mapping error = %v", err)
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
	if err := validateManifest(manifest); err != nil {
		t.Fatalf("committed manifest validateManifest() error = %v", err)
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
