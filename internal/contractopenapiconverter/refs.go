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
	return convertSchemaGraph(root, components, profileStageRefs)
}

// ConvertCompositionNullableSchema converts one OpenAPI 3.0.3 schema graph using
// the composition-nullable profile documented in
// docs/internal/contract/openapi-to-draft-2020-12-converter-profile.md.
func ConvertCompositionNullableSchema(root map[string]any, components map[string]any) (map[string]any, []contractvalidator.Diagnostic) {
	return convertSchemaGraph(root, components, profileStageCompositionNullable)
}

// ConvertFailClosedSchema converts one OpenAPI 3.0.3 schema graph using the
// fail-closed profile documented in
// docs/internal/contract/openapi-to-draft-2020-12-converter-profile.md.
func ConvertFailClosedSchema(root map[string]any, components map[string]any) (map[string]any, []contractvalidator.Diagnostic) {
	return convertSchemaGraph(root, components, profileStageFailClosed)
}

func convertSchemaGraph(root map[string]any, components map[string]any, stage string) (map[string]any, []contractvalidator.Diagnostic) {
	if components == nil {
		components = map[string]any{}
	}
	ctx := &convertContext{
		stage:      stage,
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

type convertContext struct {
	stage      string
	components map[string]any
	defs       map[string]any
	visiting   map[string]bool
}

func (ctx *convertContext) convertNode(value any, path string) (any, []contractvalidator.Diagnostic) {
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

func (ctx *convertContext) convertSchemaObject(schema map[string]any, path string) (any, []contractvalidator.Diagnostic) {
	if ref, hasRef := schema["$ref"]; hasRef {
		if diagnostics := ctx.validateRefSiblings(schema, path); len(diagnostics) != 0 {
			return nil, diagnostics
		}
		return ctx.convertComponentRef(ref, path)
	}

	if diagnostics := ctx.validateFailClosedSemantics(schema, path); len(diagnostics) != 0 {
		return nil, diagnostics
	}

	if diagnostics := ctx.validateNullableComposition(schema, path); len(diagnostics) != 0 {
		return nil, diagnostics
	}

	for key := range schema {
		if !isKeywordAllowed(ctx.stage, key) {
			return nil, []contractvalidator.Diagnostic{unsupportedKeyword(key, joinPath(path, key), ctx.stage)}
		}
	}

	result := make(map[string]any, len(schema))
	exclusiveMinimum, hasExclusiveMinimum := openAPIExclusiveBound(schema, "minimum", "exclusiveMinimum")
	exclusiveMaximum, hasExclusiveMaximum := openAPIExclusiveBound(schema, "maximum", "exclusiveMaximum")
	for key, value := range schema {
		if key == "nullable" {
			continue
		}
		childPath := joinPath(path, key)
		switch key {
		case "properties":
			converted, diagnostics := ctx.convertPropertiesField(value, childPath)
			if len(diagnostics) != 0 {
				return nil, diagnostics
			}
			result[key] = converted
		case "items":
			itemSchema, diagnostics := ctx.convertItemsField(value, childPath)
			if len(diagnostics) != 0 {
				return nil, diagnostics
			}
			result[key] = itemSchema
		case "additionalProperties":
			additionalSchema, diagnostics := ctx.convertAdditionalPropertiesField(value, childPath)
			if len(diagnostics) != 0 {
				return nil, diagnostics
			}
			result[key] = additionalSchema
		case "allOf", "oneOf", "anyOf":
			converted, diagnostics := ctx.convertCompositionField(value, childPath)
			if len(diagnostics) != 0 {
				return nil, diagnostics
			}
			result[key] = converted
		case "type":
			typeValue, ok := value.(string)
			if !ok {
				return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
			}
			if _, supported := supportedPrimitiveTypes[typeValue]; !supported {
				return nil, []contractvalidator.Diagnostic{unsupportedKeyword("type:"+typeValue, childPath, ctx.stage)}
			}
			result[key] = typeValue
		case "minimum":
			if !hasExclusiveMinimum {
				result[key] = value
			}
		case "maximum":
			if !hasExclusiveMaximum {
				result[key] = value
			}
		case "exclusiveMinimum":
			if hasExclusiveMinimum {
				result[key] = exclusiveMinimum
			} else if enabled, ok := value.(bool); !ok || enabled {
				return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
			}
		case "exclusiveMaximum":
			if hasExclusiveMaximum {
				result[key] = exclusiveMaximum
			} else if enabled, ok := value.(bool); !ok || enabled {
				return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
			}
		case "required", "enum", "description", "title", "format", "default",
			"minLength", "maxLength", "pattern", "minItems", "maxItems", "uniqueItems":
			result[key] = value
		default:
			return nil, []contractvalidator.Diagnostic{unsupportedKeyword(key, childPath, ctx.stage)}
		}
	}
	if diagnostics := ctx.applyNullable(schema, result, path); len(diagnostics) != 0 {
		return nil, diagnostics
	}
	return result, nil
}

func (ctx *convertContext) applyNullable(schema, result map[string]any, path string) []contractvalidator.Diagnostic {
	if ctx.stage != profileStageCompositionNullable && ctx.stage != profileStageFailClosed {
		return nil
	}
	nullableValue, hasNullable := schema["nullable"]
	if !hasNullable {
		return nil
	}
	nullable, ok := nullableValue.(bool)
	if !ok {
		return []contractvalidator.Diagnostic{invalidSchemaValue(joinPath(path, "nullable"))}
	}
	if !nullable {
		return nil
	}
	typeValue, ok := result["type"].(string)
	if !ok {
		return []contractvalidator.Diagnostic{ambiguousNullable(
			joinPath(path, "nullable"),
			"nullable: true requires a supported primitive type keyword",
		)}
	}
	result["type"] = []any{typeValue, "null"}
	return nil
}

func (ctx *convertContext) validateRefSiblings(schema map[string]any, path string) []contractvalidator.Diagnostic {
	if len(schema) == 1 {
		return nil
	}
	if ctx.stage == profileStageFailClosed {
		if nullable, ok := schema["nullable"].(bool); ok && nullable {
			return []contractvalidator.Diagnostic{ambiguousNullable(
				joinPath(path, "nullable"),
				fmt.Sprintf("nullable: true cannot appear with $ref in the %s converter profile", ctx.stage),
			)}
		}
	}
	return []contractvalidator.Diagnostic{refWithSiblingKeywords(path)}
}

func (ctx *convertContext) validateFailClosedSemantics(schema map[string]any, path string) []contractvalidator.Diagnostic {
	if ctx.stage != profileStageFailClosed {
		return nil
	}
	if _, ok := schema["discriminator"]; ok {
		return []contractvalidator.Diagnostic{ambiguousDiscriminator(joinPath(path, "discriminator"))}
	}
	compositionCount := 0
	for key := range compositionKeywords {
		if _, ok := schema[key]; ok {
			compositionCount++
		}
	}
	if compositionCount > 1 {
		compositionPath := path
		if compositionPath == "" {
			compositionPath = "/"
		}
		return []contractvalidator.Diagnostic{ambiguousComposition(
			compositionPath,
			fmt.Sprintf("schema cannot combine multiple composition keywords in the %s converter profile", ctx.stage),
		)}
	}
	if _, hasDefault := schema["default"]; hasDefault {
		_, hasType := schema["type"]
		_, hasEnum := schema["enum"]
		if !hasType && !hasEnum {
			return []contractvalidator.Diagnostic{ambiguousDefault(joinPath(path, "default"))}
		}
	}
	return nil
}

func (ctx *convertContext) validateNullableComposition(schema map[string]any, path string) []contractvalidator.Diagnostic {
	if ctx.stage != profileStageCompositionNullable && ctx.stage != profileStageFailClosed {
		return nil
	}
	nullable, hasNullable := schema["nullable"].(bool)
	if !hasNullable || !nullable {
		return nil
	}
	for key := range compositionKeywords {
		if _, ok := schema[key]; ok {
			return []contractvalidator.Diagnostic{ambiguousNullable(
				joinPath(path, "nullable"),
				fmt.Sprintf("nullable: true cannot appear with %q in the %s converter profile", key, ctx.stage),
			)}
		}
	}
	return nil
}

func (ctx *convertContext) convertComponentRef(value any, path string) (map[string]any, []contractvalidator.Diagnostic) {
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

func (ctx *convertContext) materializeDefinition(name string) []contractvalidator.Diagnostic {
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
