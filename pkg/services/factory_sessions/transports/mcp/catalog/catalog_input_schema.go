package catalog

import (
	"encoding/json"
	"fmt"
	"strings"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

// CatalogInputSchema records one authored catalog tool input schema projection.
type CatalogInputSchema struct {
	Name   string
	Schema map[string]any
}

// CatalogInputSchemasFromCatalogDocument extracts resolved input schemas from one
// MCP tool-catalog document value keyed by canonical public tool name.
func CatalogInputSchemasFromCatalogDocument(value any) ([]CatalogInputSchema, error) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("catalog document is not an object")
	}
	toolsValue, ok := root["tools"]
	if !ok {
		return nil, fmt.Errorf("catalog document missing tools")
	}
	tools, ok := toolsValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("catalog tools is not an object")
	}

	schemas := make([]CatalogInputSchema, 0, len(tools))
	for key, raw := range tools {
		record, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("catalog tool %q is not an object", key)
		}
		name, _ := record["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("catalog tool %q has empty name", key)
		}
		inputValue, ok := record["input"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("catalog tool %q input is not an object", key)
		}
		schemaValue, ok := inputValue["schema"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("catalog tool %q input.schema is not an object", key)
		}
		schemas = append(schemas, CatalogInputSchema{Name: name, Schema: schemaValue})
	}
	return schemas, nil
}

// PrepareCatalogInputSchemaForParity strips authored-catalog draft metadata and
// canonicalizes one input schema for semantic comparison with DiscoverTools.
func PrepareCatalogInputSchemaForParity(schema map[string]any) (map[string]any, error) {
	copied, err := deepCopySchemaValue(schema)
	if err != nil {
		return nil, err
	}
	root, ok := copied.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("input schema is not an object")
	}
	delete(root, "$schema")
	normalizeInputSchemaForParity(root)
	prepared, err := mcpfactorysession.CanonicalizeInputSchema(root)
	if err != nil {
		return nil, err
	}
	return prepared, nil
}

// VerifyCatalogInputSchemaParity ensures every discovered canonical tool input
// schema matches the authored catalog after reference resolution and parity
// normalization without mutating discovery maps.
func VerifyCatalogInputSchemaParity(catalog []CatalogInputSchema, discovered []mcpfactorysession.ToolDefinition) error {
	byName := make(map[string]map[string]any, len(catalog))
	for _, entry := range catalog {
		if _, ok := byName[entry.Name]; ok {
			return fmt.Errorf("duplicate catalog input schema for tool %q", entry.Name)
		}
		prepared, err := PrepareCatalogInputSchemaForParity(entry.Schema)
		if err != nil {
			return fmt.Errorf("catalog tool %q input schema: %w", entry.Name, err)
		}
		byName[entry.Name] = prepared
	}

	for _, tool := range discovered {
		catalogSchema, ok := byName[tool.Name]
		if !ok {
			return fmt.Errorf("catalog missing input schema for discovered tool %q", tool.Name)
		}
		discoveredSchema, err := PrepareCatalogInputSchemaForParity(tool.InputSchema)
		if err != nil {
			return fmt.Errorf("discovered tool %q input schema: %w", tool.Name, err)
		}
		catalogJSON, err := json.Marshal(catalogSchema)
		if err != nil {
			return fmt.Errorf("marshal catalog input schema for %q: %w", tool.Name, err)
		}
		discoveredJSON, err := json.Marshal(discoveredSchema)
		if err != nil {
			return fmt.Errorf("marshal discovered input schema for %q: %w", tool.Name, err)
		}
		if string(catalogJSON) != string(discoveredJSON) {
			return fmt.Errorf(
				"catalog input schema for %q differs from DiscoverTools semantics:\ncatalog=%s\ndiscovered=%s",
				tool.Name,
				catalogJSON,
				discoveredJSON,
			)
		}
	}

	if len(catalog) != len(discovered) {
		discoveredNames := make(map[string]struct{}, len(discovered))
		for _, tool := range discovered {
			discoveredNames[tool.Name] = struct{}{}
		}
		for _, entry := range catalog {
			if _, ok := discoveredNames[entry.Name]; !ok {
				return fmt.Errorf("catalog contains extra input schema for tool %q", entry.Name)
			}
		}
		return fmt.Errorf("catalog input schema count = %d, want %d discovered canonical tools", len(catalog), len(discovered))
	}

	return nil
}

func normalizeInputSchemaForParity(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "$schema")
		delete(typed, "description")
		if typeName, _ := typed["type"].(string); typeName == "object" {
			if additional, ok := typed["additionalProperties"]; !ok || additional == nil {
				typed["additionalProperties"] = true
			} else if _, isBool := additional.(bool); !isBool {
				typed["additionalProperties"] = true
			}
		}
		for key, child := range typed {
			if key == "$schema" || key == "description" {
				continue
			}
			normalizeInputSchemaForParity(child)
		}
	case []any:
		for _, item := range typed {
			normalizeInputSchemaForParity(item)
		}
	}
}

func deepCopySchemaValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var copied any
	if err := json.Unmarshal(encoded, &copied); err != nil {
		return nil, err
	}
	return copied, nil
}

// CatalogInputSchemaToolPath returns the JSON pointer path to one tool input schema.
func CatalogInputSchemaToolPath(toolName string) string {
	return "/tools/" + escapeCatalogJSONPointerToken(CatalogToolIDForName(toolName)) + "/input/schema"
}

func escapeCatalogJSONPointerToken(token string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(token)
}
