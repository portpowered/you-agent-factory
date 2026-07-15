package contracts_test

import (
	"path/filepath"
	"slices"
	"testing"
)

const sensitivitySchemaID = "https://schemas.portpowered.com/you/contracts/common/sensitivity.schema.json"

func TestSensitivitySchemaFixtures(t *testing.T) {
	schema := compileSchema(
		t,
		filepath.Join("common", "sensitivity.schema.json"),
		sensitivitySchemaID,
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
		{name: "valid public classification", fixture: "valid-public.json", valid: true},
		{name: "unknown classification", fixture: "invalid-unknown-classification.json", wantPath: "/classification"},
		{name: "unknown property", fixture: "invalid-unknown-property.json", wantPath: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "common", "sensitivity", test.fixture))
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
