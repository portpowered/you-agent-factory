package contracts_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const (
	compatibilityInventorySchemaID    = "https://schemas.portpowered.com/you/contracts/compatibility-inventory.schema.json"
	compatibilityVocabularySchemaID   = "https://schemas.portpowered.com/you/contracts/common/compatibility-inventory.schema.json"
	compatibilityInventoryFixtureRoot = "compatibility-inventory"
)

func TestCompatibilityInventorySchemaFixtures(t *testing.T) {
	schema := compileSchema(
		t,
		"compatibility-inventory.schema.json",
		compatibilityInventorySchemaID,
		schemaResource{
			path: filepath.Join("common", "compatibility-inventory.schema.json"),
			id:   compatibilityVocabularySchemaID,
		},
		schemaResource{
			path: filepath.Join("common", "documentation.schema.json"),
			id:   documentationSchemaID,
		},
		schemaResource{
			path: filepath.Join("common", "deprecations.schema.json"),
			id:   deprecationsSchemaID,
		},
	)

	tests := []struct {
		name         string
		fixture      string
		valid        bool
		wantPath     string
		semanticOnly bool
	}{
		{name: "valid mcp retain-temporarily", fixture: "valid-mcp-retain.json", valid: true},
		{name: "valid api remove-now", fixture: "valid-api-remove-now.json", valid: true},
		{name: "valid cli separately-approved", fixture: "valid-cli-separately-approved.json", valid: true},
		{name: "missing classification", fixture: "invalid-missing-classification.json", wantPath: "/records/mcp.alias.you.workflow.validate"},
		{name: "missing successor", fixture: "invalid-missing-successor.json", wantPath: "/records/mcp.alias.you.workflow.validate/lifecycle"},
		{name: "unknown family", fixture: "invalid-unknown-family.json", wantPath: "/family"},
		{name: "unknown classification", fixture: "invalid-unknown-classification.json", wantPath: "/records/mcp.alias.you.workflow.validate/classification"},
		{name: "unknown property", fixture: "invalid-unknown-property.json", wantPath: "/records/mcp.alias.you.workflow.validate"},
		{name: "unknown format version", fixture: "invalid-format-version.json", wantPath: "/formatVersion"},
		{name: "record family mismatch", fixture: "invalid-record-family-mismatch.json", semanticOnly: true, wantPath: "/records/mcp.alias.you.workflow.validate/family"},
		{name: "duplicate stable IDs", fixture: "invalid-duplicate-stable-ids.json", semanticOnly: true, wantPath: "/records/mcp.alias.you.workflow.run/itemId"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", compatibilityInventoryFixtureRoot, test.fixture))
			err := schema.Validate(instance)
			if test.semanticOnly {
				if err != nil {
					t.Fatalf("schema validation should pass before semantics: %v", err)
				}
				diagnostics := contractvalidator.CompatibilityInventorySemanticsDiagnostics(test.fixture, instance)
				if test.valid {
					if len(diagnostics) != 0 {
						t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
					}
					return
				}
				if len(diagnostics) == 0 {
					t.Fatal("expected semantic validation to fail")
				}
				paths := semanticDiagnosticPaths(diagnostics)
				if !slices.Contains(paths, test.wantPath) {
					t.Fatalf("semantic paths = %v, want %q", paths, test.wantPath)
				}
				return
			}

			if test.valid {
				if err != nil {
					t.Fatalf("validate valid fixture: %v", err)
				}
				diagnostics := contractvalidator.CompatibilityInventorySemanticsDiagnostics(test.fixture, instance)
				if len(diagnostics) != 0 {
					t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
				}
				return
			}
			if err == nil {
				t.Fatal("expected fixture validation to fail")
			}
			if paths := validationPaths(t, err); !slices.Contains(paths, test.wantPath) {
				t.Fatalf("validation paths = %v, want %q", paths, test.wantPath)
			}
		})
	}
}

func semanticDiagnosticPaths(diagnostics []contractvalidator.Diagnostic) []string {
	paths := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		paths = append(paths, diagnostic.Path)
	}
	return paths
}
