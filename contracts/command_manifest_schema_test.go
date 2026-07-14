package contracts_test

import (
	"path/filepath"
	"testing"
)

const commandManifestSchemaID = "https://schemas.portpowered.com/you/contracts/cli/command-manifest.schema.json"

func TestCommandManifestSchemaValidIdentityFixture(t *testing.T) {
	schema := compileSchema(
		t,
		filepath.Join("cli", "command-manifest.schema.json"),
		commandManifestSchemaID,
		schemaResource{
			path: filepath.Join("common", "documentation.schema.json"),
			id:   documentationSchemaID,
		},
		schemaResource{
			path: filepath.Join("common", "deprecations.schema.json"),
			id:   deprecationsSchemaID,
		},
	)

	instance := readJSON(t, filepath.Join("testdata", "cli", "valid-identity.json"))
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("validate valid identity fixture: %v", err)
	}
}
