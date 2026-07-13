package contracts_test

import (
	"path/filepath"
	"slices"
	"testing"
)

const deprecationsSchemaID = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"

func TestDeprecationsSchemaFixtures(t *testing.T) {
	schema := compileSchema(
		t,
		filepath.Join("common", "deprecations.schema.json"),
		deprecationsSchemaID,
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
		{name: "valid active lifecycle", fixture: "valid-active.json", valid: true},
		{name: "valid deprecated lifecycle", fixture: "valid-deprecated.json", valid: true},
		{name: "valid removed lifecycle", fixture: "valid-removed.json", valid: true},
		{name: "active with deprecated version", fixture: "invalid-active-deprecated.json", wantPath: "/deprecated"},
		{name: "active with successor", fixture: "invalid-active-successor.json", wantPath: "/successor"},
		{name: "deprecated without deprecated version", fixture: "invalid-deprecated-missing-version.json", wantPath: ""},
		{name: "deprecated with removed version", fixture: "invalid-deprecated-removed.json", wantPath: "/removed"},
		{name: "removed without deprecated version", fixture: "invalid-removed-missing-deprecated.json", wantPath: ""},
		{name: "deprecated without successor", fixture: "invalid-missing-successor.json", wantPath: ""},
		{name: "removed without successor", fixture: "invalid-removed-missing-successor.json", wantPath: ""},
		{name: "successor without target", fixture: "invalid-successor-missing-target.json", wantPath: "/successor"},
		{name: "successor without guidance", fixture: "invalid-successor-missing-guidance.json", wantPath: "/successor"},
		{name: "successor with empty guidance", fixture: "invalid-successor-empty-guidance.json", wantPath: "/successor/canonicalEnglish"},
		{name: "malformed since version", fixture: "invalid-since-version.json", wantPath: "/since"},
		{name: "malformed deprecated version", fixture: "invalid-deprecated-version.json", wantPath: "/deprecated"},
		{name: "malformed removed version", fixture: "invalid-removed-version.json", wantPath: "/removed"},
		{name: "unknown format version", fixture: "invalid-format-version.json", wantPath: "/formatVersion"},
		{name: "malformed item ID", fixture: "invalid-item-id.json", wantPath: "/itemId"},
		{name: "malformed successor item ID", fixture: "invalid-successor-item-id.json", wantPath: "/successor/targetItemId"},
		{name: "unknown property", fixture: "invalid-unknown-property.json", wantPath: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "common", "deprecations", test.fixture))
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
