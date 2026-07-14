package contractstaging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const openAPIComponentPrefix = "#/components/schemas/"

func generateFactorySchema(repositoryRoot string) ([]byte, error) {
	path := filepath.Join(repositoryRoot, "api", "openapi.yaml")
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read canonical OpenAPI contract: %w", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("decode canonical OpenAPI contract: %w", err)
	}
	components, err := schemaComponents(document)
	if err != nil {
		return nil, err
	}
	factory, ok := components["Factory"]
	if !ok {
		return nil, fmt.Errorf("canonical OpenAPI contract has no Factory schema")
	}

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
