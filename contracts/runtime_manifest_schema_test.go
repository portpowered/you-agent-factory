package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const runtimeManifestSchemaID = "https://schemas.portpowered.com/you/contracts/javascript/runtime-manifest.schema.json"

func runtimeManifestSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	return compileSchema(
		t,
		filepath.Join("javascript", "runtime-manifest.schema.json"),
		runtimeManifestSchemaID,
		schemaResource{
			path: filepath.Join("common", "documentation.schema.json"),
			id:   documentationSchemaID,
		},
		schemaResource{
			path: filepath.Join("common", "deprecations.schema.json"),
			id:   deprecationsSchemaID,
		},
	)
}

func TestRuntimeManifestSchemaValidFixtures(t *testing.T) {
	schema := runtimeManifestSchema(t)

	tests := []struct {
		name    string
		fixture string
	}{
		{name: "nested namespace", fixture: "valid-nested-namespace.json"},
		{name: "value globals", fixture: "valid-value.json"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "javascript", test.fixture))
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("validate valid fixture %s: %v", test.fixture, err)
			}
		})
	}
}
