package contractopenapiconverter

import (
	"fmt"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

// ConvertCoreSchema converts one OpenAPI 3.0.3 schema object using the
// core-shapes profile documented in
// docs/internal/contract/openapi-to-draft-2020-12-converter-profile.md.
func ConvertCoreSchema(schema map[string]any) (map[string]any, []contractvalidator.Diagnostic) {
	converted, diagnostics := convertSchemaObject(schema, "")
	if len(diagnostics) != 0 {
		contractvalidator.SortDiagnostics(diagnostics)
		return nil, diagnostics
	}
	result, ok := converted.(map[string]any)
	if !ok {
		diagnostic := invalidSchemaValue("")
		contractvalidator.SortDiagnostics([]contractvalidator.Diagnostic{diagnostic})
		return nil, []contractvalidator.Diagnostic{diagnostic}
	}
	return result, nil
}

func convertSchemaObject(schema map[string]any, path string) (any, []contractvalidator.Diagnostic) {
	for key := range schema {
		if !isCoreShapeKeyword(key) {
			return nil, []contractvalidator.Diagnostic{unsupportedKeyword(key, joinPath(path, key), profileStageCoreShapes)}
		}
	}

	result := make(map[string]any, len(schema))
	for key, value := range schema {
		childPath := joinPath(path, key)
		switch key {
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok {
				return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
			}
			converted := make(map[string]any, len(properties))
			for name, property := range properties {
				propertySchema, ok := property.(map[string]any)
				if !ok {
					return nil, []contractvalidator.Diagnostic{invalidSchemaValue(joinPath(childPath, name))}
				}
				propertyValue, diagnostics := convertSchemaObject(propertySchema, joinPath(childPath, name))
				if len(diagnostics) != 0 {
					return nil, diagnostics
				}
				converted[name] = propertyValue
			}
			result[key] = converted
		case "items":
			itemSchema, ok := value.(map[string]any)
			if !ok {
				return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
			}
			itemValue, diagnostics := convertSchemaObject(itemSchema, childPath)
			if len(diagnostics) != 0 {
				return nil, diagnostics
			}
			result[key] = itemValue
		case "additionalProperties":
			switch typed := value.(type) {
			case bool:
				result[key] = typed
			case map[string]any:
				additionalValue, diagnostics := convertSchemaObject(typed, childPath)
				if len(diagnostics) != 0 {
					return nil, diagnostics
				}
				result[key] = additionalValue
			default:
				return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
			}
		case "type":
			typeValue, ok := value.(string)
			if !ok {
				return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
			}
			if _, supported := supportedPrimitiveTypes[typeValue]; !supported {
				return nil, []contractvalidator.Diagnostic{unsupportedKeyword("type:"+typeValue, childPath, profileStageCoreShapes)}
			}
			result[key] = typeValue
		case "required", "enum", "description", "title", "format", "default",
			"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
			"minLength", "maxLength", "pattern", "minItems", "maxItems", "uniqueItems":
			result[key] = value
		default:
			return nil, []contractvalidator.Diagnostic{unsupportedKeyword(key, childPath, profileStageCoreShapes)}
		}
	}
	return result, nil
}

func joinPath(base, segment string) string {
	if base == "" {
		return "/" + segment
	}
	return fmt.Sprintf("%s/%s", base, segment)
}
