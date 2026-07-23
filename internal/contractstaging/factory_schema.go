package contractstaging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/portpowered/infinite-you/internal/contractopenapiconverter"
	"github.com/portpowered/infinite-you/internal/contractvalidator"
	"gopkg.in/yaml.v3"
)

const openAPIComponentPrefix = "#/components/schemas/"

func generateFactorySchema(repositoryRoot string) ([]byte, error) {
	factory, components, err := loadFactorySchemaGraph(repositoryRoot)
	if err != nil {
		return nil, err
	}
	return generateFactorySchemaFromGraph(repositoryRoot, factory, components)
}

func generateFactorySchemaFromGraph(
	repositoryRoot string,
	factory map[string]any,
	components map[string]any,
) ([]byte, error) {
	converted, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(factory, components)
	if len(diagnostics) == 0 {
		return marshalFactorySchemaDocument(converted)
	}
	record, err := loadFactorySchemaB16Gaps(repositoryRoot)
	if err != nil {
		return nil, err
	}
	if factorySchemaGapRecordEndorsesConverter(record) {
		return nil, factorySchemaUndocumentedDiagnosticsError(diagnostics)
	}
	expected, err := factorySchemaConverterFailureExpected(repositoryRoot, diagnostics)
	if err != nil {
		return nil, err
	}
	if !expected {
		return nil, factorySchemaUndocumentedDiagnosticsError(diagnostics)
	}
	return legacyGenerateFactorySchema(factory, components)
}

func factorySchemaUndocumentedDiagnosticsError(diagnostics []contractvalidator.Diagnostic) error {
	payload, err := json.Marshal(diagnostics)
	if err != nil {
		return fmt.Errorf("encode factory schema converter diagnostics: %w", err)
	}
	return fmt.Errorf("factory schema conversion is blocked by undocumented diagnostics: %s", payload)
}

func loadFactorySchemaGraph(repositoryRoot string) (map[string]any, map[string]any, error) {
	path := filepath.Join(repositoryRoot, "api", "openapi.yaml")
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read canonical OpenAPI contract: %w", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(payload, &document); err != nil {
		return nil, nil, fmt.Errorf("decode canonical OpenAPI contract: %w", err)
	}
	components, err := schemaComponents(document)
	if err != nil {
		return nil, nil, err
	}
	factory, ok := components["Factory"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("canonical OpenAPI contract has no Factory schema")
	}
	return factory, components, nil
}

func marshalFactorySchemaDocument(root map[string]any) ([]byte, error) {
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["$id"] = "https://schemas.portpowered.com/you/config/factory.schema.json"
	root["title"] = "You Factory configuration"
	return contractjoiner.MarshalCanonicalJSON(root)
}

func marshalFactorySchemaYAML(jsonDocument []byte) ([]byte, error) {
	var schema any
	if err := json.Unmarshal(jsonDocument, &schema); err != nil {
		return nil, fmt.Errorf("decode generated Factory schema for YAML serialization: %w", err)
	}
	payload, err := yaml.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("encode generated Factory schema as YAML: %w", err)
	}
	return payload, nil
}

func legacyGenerateFactorySchema(factory map[string]any, components map[string]any) ([]byte, error) {
	definitions := make(map[string]any)
	root, err := rewriteComponentRefs(factory, components, definitions)
	if err != nil {
		return nil, err
	}
	rootObject, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canonical OpenAPI Factory schema is not an object")
	}
	rootObject["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	rootObject["$id"] = "https://schemas.portpowered.com/you/config/factory.schema.json"
	rootObject["title"] = "You Factory configuration"
	rootObject["$defs"] = definitions
	return marshalDocument(rootObject)
}

func schemaComponents(document map[string]any) (map[string]any, error) {
	components, ok := document["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canonical OpenAPI contract has no components object")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canonical OpenAPI contract has no components.schemas object")
	}
	return schemas, nil
}

func rewriteComponentRefs(value any, components, definitions map[string]any) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if key == "$ref" {
				reference, ok := child.(string)
				if ok && strings.HasPrefix(reference, openAPIComponentPrefix) {
					name := strings.TrimPrefix(reference, openAPIComponentPrefix)
					if err := addDefinition(name, components, definitions); err != nil {
						return nil, err
					}
					result[key] = "#/$defs/" + name
					continue
				}
			}
			rewritten, err := rewriteComponentRefs(child, components, definitions)
			if err != nil {
				return nil, err
			}
			result[key] = rewritten
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			rewritten, err := rewriteComponentRefs(child, components, definitions)
			if err != nil {
				return nil, err
			}
			result[index] = rewritten
		}
		return result, nil
	default:
		return value, nil
	}
}

func addDefinition(name string, components, definitions map[string]any) error {
	if _, exists := definitions[name]; exists {
		return nil
	}
	component, exists := components[name]
	if !exists {
		return fmt.Errorf("canonical OpenAPI Factory schema references missing component %s", name)
	}
	definitions[name] = map[string]any{}
	rewritten, err := rewriteComponentRefs(component, components, definitions)
	if err != nil {
		return err
	}
	definitions[name] = rewritten
	return nil
}

func marshalDocument(value any) ([]byte, error) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode generated contract: %w", err)
	}
	return append(payload, '\n'), nil
}

func collectFactorySchemaConverterDiagnostics(
	factory map[string]any,
	components map[string]any,
) []contractvalidator.Diagnostic {
	seen := make(map[string]struct{})
	var collected []contractvalidator.Diagnostic
	for {
		_, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(factory, components)
		if len(diagnostics) == 0 {
			return collected
		}
		diagnostic := diagnostics[0]
		key := diagnostic.Code + "|" + diagnostic.Path + "|" + diagnostic.Message
		if _, exists := seen[key]; exists {
			return append(collected, diagnostic)
		}
		seen[key] = struct{}{}
		collected = append(collected, diagnostic)
		if !stripFactorySchemaConverterGap(factory, components, diagnostic) {
			return collected
		}
	}
}

func stripFactorySchemaConverterGap(
	factory map[string]any,
	components map[string]any,
	diagnostic contractvalidator.Diagnostic,
) bool {
	switch diagnostic.Code {
	case "openapi.convert.unsupported_keyword":
		return deleteFactorySchemaAtPath(factory, components, diagnostic.Path)
	case "openapi.convert.unsupported_reference":
		if strings.Contains(diagnostic.Message, "$ref must be the only keyword") {
			return stripFactorySchemaRefSiblings(factory, components, diagnostic.Path)
		}
	}
	return deleteFactorySchemaAtPath(factory, components, diagnostic.Path)
}

func stripFactorySchemaRefSiblings(factory, components map[string]any, path string) bool {
	node, ok := resolveFactorySchemaNode(factory, components, path).(map[string]any)
	if !ok {
		return false
	}
	ref, ok := node["$ref"]
	if !ok {
		return false
	}
	for key := range node {
		delete(node, key)
	}
	node["$ref"] = ref
	return true
}

func deleteFactorySchemaAtPath(factory, components map[string]any, path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return false
	}
	if parts[0] == "$defs" {
		if len(parts) < 2 {
			return false
		}
		component, ok := components[parts[1]].(map[string]any)
		if !ok {
			return false
		}
		if len(parts) == 2 {
			delete(components, parts[1])
			return true
		}
		return deleteFactorySchemaPath(component, parts[2:])
	}
	return deleteFactorySchemaPath(factory, parts)
}

func deleteFactorySchemaPath(root map[string]any, parts []string) bool {
	var current any = root
	for index, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return false
		}
		if index == len(parts)-1 {
			delete(object, part)
			return true
		}
		current = object[part]
	}
	return false
}

func resolveFactorySchemaNode(factory, components map[string]any, path string) any {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 && parts[0] == "$defs" {
		if len(parts) < 2 {
			return nil
		}
		current, ok := components[parts[1]]
		if !ok {
			return nil
		}
		for _, part := range parts[2:] {
			object, ok := current.(map[string]any)
			if !ok {
				return nil
			}
			current = object[part]
		}
		return current
	}
	current := any(factory)
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[part]
	}
	return current
}
