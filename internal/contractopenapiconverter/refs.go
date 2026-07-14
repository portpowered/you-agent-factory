package contractopenapiconverter

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const openAPIComponentSchemaPrefix = "#/components/schemas/"

// ConvertRefsSchema converts one OpenAPI 3.0.3 schema graph using the refs
// profile documented in
// docs/internal/contract/openapi-to-draft-2020-12-converter-profile.md.
// root is the schema object to convert; components is the OpenAPI
// components.schemas map that supplies referenced component bodies.
func ConvertRefsSchema(root map[string]any, components map[string]any) (map[string]any, []contractvalidator.Diagnostic) {
	if components == nil {
		components = map[string]any{}
	}
	ctx := &refsContext{
		components: components,
		defs:       make(map[string]any),
		visiting:   make(map[string]bool),
	}
	converted, diagnostics := ctx.convertNode(root, "")
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
	if len(ctx.defs) != 0 {
		result["$defs"] = ctx.defs
	}
	return result, nil
}

type refsContext struct {
	components map[string]any
	defs       map[string]any
	visiting   map[string]bool
}

func (ctx *refsContext) convertNode(value any, path string) (any, []contractvalidator.Diagnostic) {
	switch typed := value.(type) {
	case map[string]any:
		return ctx.convertSchemaObject(typed, path)
	case []any:
		converted := make([]any, len(typed))
		for index, child := range typed {
			childPath := joinPath(path, fmt.Sprintf("%d", index))
			childValue, diagnostics := ctx.convertNode(child, childPath)
			if len(diagnostics) != 0 {
				return nil, diagnostics
			}
			converted[index] = childValue
		}
		return converted, nil
	default:
		return value, nil
	}
}

func (ctx *refsContext) convertSchemaObject(schema map[string]any, path string) (any, []contractvalidator.Diagnostic) {
	if ref, hasRef := schema["$ref"]; hasRef {
		if len(schema) != 1 {
			return nil, []contractvalidator.Diagnostic{refWithSiblingKeywords(path)}
		}
		return ctx.convertComponentRef(ref, path)
	}

	for key := range schema {
		if !isCoreShapeKeyword(key) {
			return nil, []contractvalidator.Diagnostic{unsupportedKeyword(key, joinPath(path, key), profileStageRefs)}
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
				propertyValue, diagnostics := ctx.convertNode(property, joinPath(childPath, name))
				if len(diagnostics) != 0 {
					return nil, diagnostics
				}
				propertySchema, ok := propertyValue.(map[string]any)
				if !ok {
					return nil, []contractvalidator.Diagnostic{invalidSchemaValue(joinPath(childPath, name))}
				}
				converted[name] = propertySchema
			}
			result[key] = converted
		case "items":
			itemValue, diagnostics := ctx.convertNode(value, childPath)
			if len(diagnostics) != 0 {
				return nil, diagnostics
			}
			itemSchema, ok := itemValue.(map[string]any)
			if !ok {
				return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
			}
			result[key] = itemSchema
		case "additionalProperties":
			switch typed := value.(type) {
			case bool:
				result[key] = typed
			case map[string]any:
				additionalValue, diagnostics := ctx.convertNode(typed, childPath)
				if len(diagnostics) != 0 {
					return nil, diagnostics
				}
				additionalSchema, ok := additionalValue.(map[string]any)
				if !ok {
					return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
				}
				result[key] = additionalSchema
			default:
				return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
			}
		case "type":
			typeValue, ok := value.(string)
			if !ok {
				return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
			}
			if _, supported := supportedPrimitiveTypes[typeValue]; !supported {
				return nil, []contractvalidator.Diagnostic{unsupportedKeyword("type:"+typeValue, childPath, profileStageRefs)}
			}
			result[key] = typeValue
		case "required", "enum", "description", "title", "format", "default",
			"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
			"minLength", "maxLength", "pattern", "minItems", "maxItems", "uniqueItems":
			result[key] = value
		default:
			return nil, []contractvalidator.Diagnostic{unsupportedKeyword(key, childPath, profileStageRefs)}
		}
	}
	return result, nil
}

func (ctx *refsContext) convertComponentRef(value any, path string) (map[string]any, []contractvalidator.Diagnostic) {
	reference, ok := value.(string)
	if !ok || reference == "" {
		return nil, []contractvalidator.Diagnostic{invalidReference(path)}
	}
	if !strings.HasPrefix(reference, openAPIComponentSchemaPrefix) {
		return nil, []contractvalidator.Diagnostic{unsupportedReference(reference, path)}
	}
	name := strings.TrimPrefix(reference, openAPIComponentSchemaPrefix)
	if name == "" || strings.Contains(name, "/") {
		return nil, []contractvalidator.Diagnostic{unsupportedReference(reference, path)}
	}
	if err := ctx.materializeDefinition(name); err != nil {
		return nil, err
	}
	return map[string]any{"$ref": "#/$defs/" + name}, nil
}

func (ctx *refsContext) materializeDefinition(name string) []contractvalidator.Diagnostic {
	if _, exists := ctx.defs[name]; exists {
		return nil
	}
	if ctx.visiting[name] {
		return []contractvalidator.Diagnostic{referenceCycle(name)}
	}
	component, exists := ctx.components[name]
	if !exists {
		return []contractvalidator.Diagnostic{missingComponent(name)}
	}
	componentSchema, ok := component.(map[string]any)
	if !ok {
		return []contractvalidator.Diagnostic{invalidSchemaValue("/$defs/" + name)}
	}

	ctx.visiting[name] = true
	converted, diagnostics := ctx.convertNode(componentSchema, "/$defs/"+name)
	delete(ctx.visiting, name)
	if len(diagnostics) != 0 {
		return diagnostics
	}
	convertedSchema, ok := converted.(map[string]any)
	if !ok {
		return []contractvalidator.Diagnostic{invalidSchemaValue("/$defs/" + name)}
	}
	ctx.defs[name] = convertedSchema
	return nil
}
