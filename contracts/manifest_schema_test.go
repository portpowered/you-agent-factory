package contracts_test

import (
	"path/filepath"
	"slices"
	"testing"
)

const manifestSchemaID = "https://schemas.portpowered.com/you/contracts/manifest.schema.json"

func TestManifestSchemaFixtures(t *testing.T) {
	schema := compileSchema(
		t,
		"manifest.schema.json",
		manifestSchemaID,
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
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "valid active exports", fixture: "valid-active.json", valid: true},
		{name: "valid deprecated export", fixture: "valid-deprecated.json", valid: true},
		{name: "unknown manifest format version", fixture: "invalid-format-version.json", wantPath: "/formatVersion"},
		{name: "malformed package version", fixture: "invalid-package-version.json", wantPath: "/packageVersion"},
		{name: "malformed source commit", fixture: "invalid-source-commit.json", wantPath: "/sourceCommit"},
		{name: "malformed family format version", fixture: "invalid-family-format-version.json", wantPath: "/familyFormatVersions/example.widgets"},
		{name: "malformed export ID", fixture: "invalid-export-id.json", wantPath: ""},
		{name: "non-relative export path", fixture: "invalid-export-path.json", wantPath: "/exports/example.widget.create/path"},
		{name: "invalid artifact hash", fixture: "invalid-artifact-hash.json", wantPath: "/exports/example.widget.create/artifactHash"},
		{name: "missing canonical English", fixture: "invalid-missing-english.json", wantPath: "/exports/example.widget.create/documentation/documentation/description"},
		{name: "malformed lifecycle transition", fixture: "invalid-lifecycle-transition.json", wantPath: "/exports/example.widget.create/lifecycle/removed"},
		{name: "deprecated export without successor", fixture: "invalid-missing-successor.json", wantPath: "/exports/example.widget.create/lifecycle"},
		{name: "unknown export property", fixture: "invalid-unknown-property.json", wantPath: "/exports/example.widget.create"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "manifest", test.fixture))
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

	t.Run("valid development manifest without source provenance", func(t *testing.T) {
		instance := readJSON(t, filepath.Join("testdata", "manifest", "valid-active.json"))
		manifest, ok := instance.(map[string]any)
		if !ok {
			t.Fatalf("fixture = %T, want object", instance)
		}
		delete(manifest, "sourceCommit")
		if err := schema.Validate(manifest); err != nil {
			t.Fatalf("validate development manifest: %v", err)
		}
	})
}
