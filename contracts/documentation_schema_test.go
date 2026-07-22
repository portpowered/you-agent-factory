package contracts_test

import (
	"fmt"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const documentationSchemaID = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"

type schemaResource struct {
	path string
	id   string
}

var (
	schemaCache sync.Map // map[string]*schemaCompileCacheEntry
	jsonCache   sync.Map // map[string]*jsonCacheEntry
)

type schemaCompileCacheEntry struct {
	once   sync.Once
	schema *jsonschema.Schema
	err    error
}

type jsonCacheEntry struct {
	once     sync.Once
	document any
	err      error
}

func TestDocumentationSchemaFixtures(t *testing.T) {
	t.Parallel()
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
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
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
	key := schemaCacheKey(path, id, resources)
	entry := &schemaCompileCacheEntry{}
	actual, _ := schemaCache.LoadOrStore(key, entry)
	cached := actual.(*schemaCompileCacheEntry)
	cached.once.Do(func() {
		cached.schema, cached.err = compileSchemaNoCache(path, id, resources...)
	})
	if cached.err != nil {
		t.Fatalf("compile schema %s: %v", id, cached.err)
	}
	return cached.schema
}

func compileSchemaNoCache(path, id string, resources ...schemaResource) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	for _, resource := range resources {
		resourceDocument, err := readJSONDocument(resource.path)
		if err != nil {
			return nil, fmt.Errorf("read schema resource %s: %w", resource.id, err)
		}
		if err := compiler.AddResource(resource.id, resourceDocument); err != nil {
			return nil, fmt.Errorf("add schema resource %s: %w", resource.id, err)
		}
	}
	document, err := readJSONDocument(path)
	if err != nil {
		return nil, fmt.Errorf("read schema document: %w", err)
	}
	if err := compiler.AddResource(id, document); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	schema, err := compiler.Compile(id)
	if err != nil {
		return nil, err
	}
	return schema, nil
}

func schemaCacheKey(path, id string, resources []schemaResource) string {
	var b strings.Builder
	b.WriteString(path)
	b.WriteRune('|')
	b.WriteString(id)
	b.WriteRune('|')
	for _, resource := range resources {
		b.WriteString(resource.id)
		b.WriteRune('|')
		b.WriteString(resource.path)
		b.WriteRune(';')
	}
	return b.String()
}

func readJSON(t *testing.T, path string) any {
	t.Helper()
	entry := &jsonCacheEntry{}
	actual, _ := jsonCache.LoadOrStore(path, entry)
	cached := actual.(*jsonCacheEntry)
	cached.once.Do(func() {
		cached.document, cached.err = readJSONDocument(path)
	})
	if cached.err != nil {
		t.Fatalf("read %s: %v", path, cached.err)
	}
	return cached.document
}

func readJSONDocument(path string) (any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	document, err := jsonschema.UnmarshalJSON(file)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return document, nil
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
