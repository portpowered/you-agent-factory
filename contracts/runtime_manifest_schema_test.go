package contracts_test

import (
	"path/filepath"
	"slices"
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

func TestRuntimeManifestSchemaCallableFixtures(t *testing.T) {
	schema := runtimeManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "valid synchronous function", fixture: "valid-synchronous-function.json", valid: true},
		{name: "valid asynchronous function", fixture: "valid-asynchronous-function.json", valid: true},
		{
			name:     "async callable with synchronous return",
			fixture:  "invalid-impossible-async-return.json",
			wantPath: "/symbols/example.bad.async-sync-return/return/kind",
		},
		{
			name:     "sync callable with promise return",
			fixture:  "invalid-impossible-async-return.json",
			wantPath: "/symbols/example.bad.sync-promise-return/return/kind",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "javascript", test.fixture))
			err := schema.Validate(instance)
			if test.valid {
				if err != nil {
					t.Fatalf("validate valid fixture: %v", err)
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
