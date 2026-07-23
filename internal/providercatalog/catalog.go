// Package providercatalog generates the public, data-only provider catalog.
package providercatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractopenapiconverter"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const (
	FormatVersion        = "1.0.0"
	ManifestSchemaID     = "https://schemas.you.dev/model-providers/provider-manifest/1.0.0.schema.json"
	CatalogSchemaID      = "https://schemas.you.dev/model-providers/provider-catalog/1.0.0.schema.json"
	ManifestSchemaPath   = "packages/model-providers/generated/provider-manifest.schema.json"
	CatalogSchemaPath    = "packages/model-providers/generated/provider-catalog.schema.json"
	CatalogPath          = "packages/model-providers/generated/catalog.json"
	openAPIPath          = "api/openapi.yaml"
	authoredProvidersDir = "packages/model-providers/providers"
)

// Catalog is a complete in-memory output plan keyed by repository-relative path.
type Catalog struct {
	Files map[string][]byte
}

// Build plans and validates all generated outputs without mutating the filesystem.
func Build(source fs.FS) (Catalog, error) {
	components, err := loadOpenAPIComponents(source)
	if err != nil {
		return Catalog{}, err
	}
	manifestSchema, err := projectSchema("ProviderManifest", ManifestSchemaID, components)
	if err != nil {
		return Catalog{}, err
	}
	catalogSchema, err := projectSchema("ProviderCatalog", CatalogSchemaID, components)
	if err != nil {
		return Catalog{}, err
	}
	providers, err := loadProviders(source, manifestSchema)
	if err != nil {
		return Catalog{}, err
	}
	if err := validateCatalogSemantics(providers); err != nil {
		return Catalog{}, err
	}
	catalog, err := marshalJSON(map[string]any{
		"formatVersion":  FormatVersion,
		"providerSchema": ManifestSchemaID,
		"providers":      providers,
	})
	if err != nil {
		return Catalog{}, fmt.Errorf("serialize provider catalog: %w", err)
	}
	if err := validateJSON(catalogSchema, catalog, CatalogPath); err != nil {
		return Catalog{}, err
	}
	return Catalog{Files: map[string][]byte{
		ManifestSchemaPath: manifestSchema,
		CatalogSchemaPath:  catalogSchema,
		CatalogPath:        catalog,
	}}, nil
}

func loadOpenAPIComponents(source fs.FS) (map[string]any, error) {
	payload, err := fs.ReadFile(source, openAPIPath)
	if err != nil {
		return nil, fmt.Errorf("read bundled OpenAPI %s: %w", openAPIPath, err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("parse bundled OpenAPI %s: %w", openAPIPath, err)
	}
	components, ok := nestedMap(document, "components", "schemas")
	if !ok {
		return nil, fmt.Errorf("parse bundled OpenAPI %s: components.schemas is missing", openAPIPath)
	}
	return components, nil
}

func projectSchema(name, identifier string, components map[string]any) ([]byte, error) {
	root, ok := components[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("project %s schema: OpenAPI component is missing", name)
	}
	converted, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(root, components)
	if len(diagnostics) != 0 {
		issue := diagnostics[0]
		return nil, fmt.Errorf("project %s schema: %s at %s: %s", name, issue.Code, issue.Path, issue.Message)
	}
	converted["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	converted["$id"] = identifier
	return marshalJSON(converted)
}

func loadProviders(source fs.FS, schemaPayload []byte) ([]any, error) {
	schema, err := compileSchema(schemaPayload)
	if err != nil {
		return nil, fmt.Errorf("compile generated provider manifest schema: %w", err)
	}
	entries, err := fs.ReadDir(source, authoredProvidersDir)
	if err != nil {
		return nil, fmt.Errorf("read authored providers %s: %w", authoredProvidersDir, err)
	}
	var providers []any
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := path.Join(authoredProvidersDir, entry.Name(), "provider.yaml")
		provider, err := loadProvider(source, schema, manifestPath)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].(map[string]any)["id"].(string) < providers[j].(map[string]any)["id"].(string)
	})
	return providers, nil
}

func loadProvider(source fs.FS, schema *jsonschema.Schema, manifestPath string) (map[string]any, error) {
	payload, err := fs.ReadFile(source, manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read authored provider %s: %w", manifestPath, err)
	}
	var yamlValue map[string]any
	if err := yaml.Unmarshal(payload, &yamlValue); err != nil {
		return nil, fmt.Errorf("parse authored provider %s: %w", manifestPath, err)
	}
	jsonPayload, err := json.Marshal(yamlValue)
	if err != nil {
		return nil, fmt.Errorf("normalize authored provider %s: %w", manifestPath, err)
	}
	var provider map[string]any
	if err := json.Unmarshal(jsonPayload, &provider); err != nil {
		return nil, fmt.Errorf("normalize authored provider %s: %w", manifestPath, err)
	}
	if err := validateValue(schema, provider, manifestPath); err != nil {
		return nil, err
	}
	id, _ := provider["id"].(string)
	if id != path.Base(path.Dir(manifestPath)) {
		return nil, fmt.Errorf("%s: provider id %q must match its directory name", manifestPath, id)
	}
	return provider, nil
}

func compileSchema(payload []byte) (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(document.(map[string]any)["$id"].(string), document); err != nil {
		return nil, err
	}
	return compiler.Compile(document.(map[string]any)["$id"].(string))
}

func validateJSON(schemaPayload, document []byte, documentPath string) error {
	schema, err := compileSchema(schemaPayload)
	if err != nil {
		return fmt.Errorf("compile schema for %s: %w", documentPath, err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return fmt.Errorf("parse generated %s: %w", documentPath, err)
	}
	return validateValue(schema, value, documentPath)
}

func validateValue(schema *jsonschema.Schema, value any, documentPath string) error {
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("%s: schema validation failed: %w", documentPath, err)
	}
	return nil
}

func marshalJSON(value any) ([]byte, error) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func nestedMap(root map[string]any, keys ...string) (map[string]any, bool) {
	current := root
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func stringsFrom(value any) []string {
	values, _ := value.([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func boolField(root map[string]any, object, field string) bool {
	nested, _ := root[object].(map[string]any)
	value, _ := nested[field].(bool)
	return value
}

func canonicalSet(values []string) string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}
