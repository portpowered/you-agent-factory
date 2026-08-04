package contractopenapiconverter

import (
	"fmt"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func (ctx *convertContext) convertPropertiesField(value any, childPath string) (map[string]any, []contractvalidator.Diagnostic) {
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
	return converted, nil
}

func (ctx *convertContext) convertItemsField(value any, childPath string) (map[string]any, []contractvalidator.Diagnostic) {
	itemValue, diagnostics := ctx.convertNode(value, childPath)
	if len(diagnostics) != 0 {
		return nil, diagnostics
	}
	itemSchema, ok := itemValue.(map[string]any)
	if !ok {
		return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
	}
	return itemSchema, nil
}

func (ctx *convertContext) convertNotField(value any, childPath string) (map[string]any, []contractvalidator.Diagnostic) {
	childValue, diagnostics := ctx.convertNode(value, childPath)
	if len(diagnostics) != 0 {
		return nil, diagnostics
	}
	childSchema, ok := childValue.(map[string]any)
	if !ok {
		return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
	}
	return childSchema, nil
}

func (ctx *convertContext) convertAdditionalPropertiesField(value any, childPath string) (any, []contractvalidator.Diagnostic) {
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case map[string]any:
		additionalValue, diagnostics := ctx.convertNode(typed, childPath)
		if len(diagnostics) != 0 {
			return nil, diagnostics
		}
		additionalSchema, ok := additionalValue.(map[string]any)
		if !ok {
			return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
		}
		return additionalSchema, nil
	default:
		return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
	}
}

func (ctx *convertContext) convertCompositionField(value any, childPath string) ([]any, []contractvalidator.Diagnostic) {
	schemas, ok := value.([]any)
	if !ok {
		return nil, []contractvalidator.Diagnostic{invalidSchemaValue(childPath)}
	}
	converted := make([]any, len(schemas))
	for index, child := range schemas {
		indexPath := joinPath(childPath, fmt.Sprintf("%d", index))
		childValue, diagnostics := ctx.convertNode(child, indexPath)
		if len(diagnostics) != 0 {
			return nil, diagnostics
		}
		childSchema, ok := childValue.(map[string]any)
		if !ok {
			return nil, []contractvalidator.Diagnostic{invalidSchemaValue(indexPath)}
		}
		converted[index] = childSchema
	}
	return converted, nil
}
