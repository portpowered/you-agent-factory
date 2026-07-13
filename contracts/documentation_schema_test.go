package contracts_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const documentationSchemaID = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"

type schemaResource struct {
	path string
	id   string
}

func TestDocumentationSchemaFixtures(t *testing.T) {
	schema := compileSchema(t, filepath.Join("common", "documentation.schema.json"), documentationSchemaID)

	tests := []struct {
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "valid public documentation", fixture: "valid-public.json", valid: true},
		{name: "valid internal documentation", fixture: "valid-internal.json", valid: true},
		{name: "duplicate documentation ID", fixture: "invalid-duplicate-documentation-id.json", wantPath: "/documentation/description/id"},
		{name: "missing canonical English title", fixture: "invalid-missing-english-title.json", wantPath: "/documentation/title"},
		{name: "missing canonical English description", fixture: "invalid-missing-english-description.json", wantPath: "/documentation/description"},
		{name: "unknown format version", fixture: "invalid-format-version.json", wantPath: "/formatVersion"},
		{name: "malformed item ID", fixture: "invalid-item-id.json", wantPath: "/itemId"},
		{name: "malformed documentation ID", fixture: "invalid-documentation-id.json", wantPath: "/documentation/title/id"},
		{name: "invalid source hash", fixture: "invalid-source-hash.json", wantPath: "/sourceHash"},
		{name: "unknown property", fixture: "invalid-unknown-property.json", wantPath: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "common", "documentation", test.fixture))
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

func compileSchema(t *testing.T, path, id string, resources ...schemaResource) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	for _, resource := range resources {
		if err := compiler.AddResource(resource.id, readJSON(t, resource.path)); err != nil {
			t.Fatalf("add schema resource %s: %v", resource.id, err)
		}
	}
	document := readJSON(t, path)
	if err := compiler.AddResource(id, document); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	schema, err := compiler.Compile(id)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return schema
}

func readJSON(t *testing.T, path string) any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	document, err := jsonschema.UnmarshalJSON(file)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return document
}

func validationPaths(t *testing.T, err error) []string {
	t.Helper()
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("validation returned %T, want *jsonschema.ValidationError", err)
	}
	paths := make([]string, 0)
	collectValidationPaths(validationErr, &paths)
	return paths
}

func collectValidationPaths(err *jsonschema.ValidationError, paths *[]string) {
	if len(err.Causes) == 0 {
		*paths = append(*paths, jsonPointer(err.InstanceLocation))
		return
	}
	for _, cause := range err.Causes {
		collectValidationPaths(cause, paths)
	}
}

func jsonPointer(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = strings.NewReplacer("~", "~0", "/", "~1").Replace(segment)
	}
	return "/" + strings.Join(escaped, "/")
}
