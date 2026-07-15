package contracts_test

import (
	"path/filepath"
	"slices"
	"testing"
)

const precedenceSchemaID = "https://schemas.portpowered.com/you/contracts/common/precedence.schema.json"

func TestPrecedenceSchemaFixtures(t *testing.T) {
	schema := compileSchema(
		t,
		filepath.Join("common", "precedence.schema.json"),
		precedenceSchemaID,
		schemaResource{
			path: filepath.Join("common", "documentation.schema.json"),
			id:   documentationSchemaID,
		},
	)

	tests := []struct {
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "valid file env flag", fixture: "valid-file-env-flag.json", valid: true},
		{name: "valid file only", fixture: "valid-file-only.json", valid: true},
		{name: "valid no layers", fixture: "valid-no-layers.json", valid: true},
		{name: "unknown property", fixture: "invalid-unknown-property.json", wantPath: ""},
		{name: "unknown layer", fixture: "invalid-unknown-layer.json", wantPath: "/layers/3"},
		{name: "unknown format version", fixture: "invalid-format-version.json", wantPath: "/formatVersion"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "common", "precedence", test.fixture))
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
