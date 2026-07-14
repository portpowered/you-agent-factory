package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func TestMCPDeprecatedInventoryValidatesAgainstSchema(t *testing.T) {
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

	instance := readJSON(t, mcpDeprecatedInventoryPath)
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("validate MCP deprecated inventory: %v", err)
	}
	diagnostics := contractvalidator.CompatibilityInventorySemanticsDiagnostics(mcpDeprecatedInventoryPath, instance)
	if len(diagnostics) != 0 {
		t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
	}
}
