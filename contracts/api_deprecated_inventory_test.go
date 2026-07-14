package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func TestAPIDeprecatedInventoryValidatesAgainstSchema(t *testing.T) {
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

	instance := readJSON(t, apiDeprecatedInventoryPath)
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("validate API deprecated inventory: %v", err)
	}
	diagnostics := contractvalidator.CompatibilityInventorySemanticsDiagnostics(apiDeprecatedInventoryPath, instance)
	if len(diagnostics) != 0 {
		t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
	}
}
