package main

import (
	"strings"
	"testing"
)

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
	}
	err := validateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "FND-06") {
		t.Fatalf("validateManifest() error = %v, want FND-06 future debt requirement", err)
	}
}
