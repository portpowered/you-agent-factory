package contractstaging

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/portpowered/infinite-you/internal/contractopenapiconverter"
	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

type standaloneComponentSchema struct {
	component string
	id        string
	title     string
	target    string
}

var standaloneFactorySchemas = []standaloneComponentSchema{
	{
		component: "FactoryEvent",
		id:        "https://schemas.portpowered.com/you/factory/factory-event.schema.json",
		title:     "Factory Event",
		target:    factoryEventSchemaTarget,
	},
	{
		component: "FactoryRecording",
		id:        "https://schemas.portpowered.com/you/factory/factory-recording.schema.json",
		title:     "Factory Recording",
		target:    factoryRecordingSchemaTarget,
	},
}

func generateStandaloneFactorySchemas(repositoryRoot string) (map[string][]byte, error) {
	_, components, err := loadFactorySchemaGraph(repositoryRoot)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]byte, len(standaloneFactorySchemas))
	for _, specification := range standaloneFactorySchemas {
		payload, err := generateStandaloneComponentSchema(components, specification)
		if err != nil {
			return nil, err
		}
		result[specification.target] = payload
	}
	return result, nil
}

func generateStandaloneComponentSchema(
	canonicalComponents map[string]any,
	specification standaloneComponentSchema,
) ([]byte, error) {
	components := deepCopyValue(canonicalComponents).(map[string]any)
	discriminator, err := detachFactoryEventAnnotations(components)
	if err != nil {
		return nil, err
	}
	stripOpenAPIExtensions(components)
	stripOpenAPIReferenceSiblings(components)

	root, ok := components[specification.component].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canonical OpenAPI contract has no %s schema", specification.component)
	}
	converted, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(root, components)
	if len(diagnostics) != 0 {
		return nil, standaloneSchemaDiagnosticsError(specification.component, diagnostics)
	}
	if err := restoreFactoryEventDiscriminator(converted, specification.component, discriminator); err != nil {
		return nil, err
	}
	if err := applyFactoryEventDiscriminatedUnion(converted, specification.component); err != nil {
		return nil, err
	}

	converted["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	converted["$id"] = specification.id
	converted["title"] = specification.title
	return contractjoiner.MarshalCanonicalJSON(converted)
}

func applyFactoryEventDiscriminatedUnion(root map[string]any, component string) error {
	event, err := factoryEventSchemaObject(root, component)
	if err != nil {
		return err
	}
	properties, ok := event["properties"].(map[string]any)
	if !ok {
		return fmt.Errorf("standalone %s schema FactoryEvent has no properties", component)
	}
	properties["payload"] = map[string]any{}

	discriminator := event["discriminator"].(map[string]any)
	mapping := discriminator["mapping"].(map[string]any)
	eventTypes, err := factoryEventTypes(root, event, properties, mapping, component)
	if err != nil {
		return err
	}

	variants := make([]any, 0, len(eventTypes))
	for _, value := range eventTypes {
		eventTypeName, ok := value.(string)
		if !ok {
			return fmt.Errorf("standalone %s schema has non-string FactoryEventType", component)
		}
		payloadReference, ok := mapping[eventTypeName].(string)
		if !ok {
			return fmt.Errorf("FactoryEvent discriminator has no mapping for %s", eventTypeName)
		}
		variants = append(variants, map[string]any{
			"type":     "object",
			"required": []any{"type", "payload"},
			"properties": map[string]any{
				"type":    map[string]any{"const": eventTypeName},
				"payload": map[string]any{"$ref": payloadReference},
			},
		})
	}
	event["oneOf"] = variants
	return nil
}

func factoryEventTypes(
	root map[string]any,
	event map[string]any,
	properties map[string]any,
	mapping map[string]any,
	component string,
) ([]any, error) {
	typeSchema, ok := properties["type"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("standalone %s schema FactoryEvent has no type property", component)
	}
	if values, ok := typeSchema["enum"].([]any); ok {
		return values, nil
	}
	reference, ok := typeSchema["$ref"].(string)
	if !ok {
		names := make([]string, 0, len(mapping))
		for name := range mapping {
			names = append(names, name)
		}
		sort.Strings(names)
		values := make([]any, len(names))
		for index, name := range names {
			values[index] = name
		}
		return values, nil
	}
	if !strings.HasPrefix(reference, "#/$defs/") {
		return nil, fmt.Errorf("standalone %s schema FactoryEvent type has unsupported enum reference", component)
	}
	definitions := event["$defs"]
	if definitions == nil {
		definitions = root["$defs"]
	}
	definitionMap, ok := definitions.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("standalone %s schema FactoryEvent has no definitions", component)
	}
	name := strings.TrimPrefix(reference, "#/$defs/")
	eventType, ok := definitionMap[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("standalone %s schema has no %s definition", component, name)
	}
	values, ok := eventType["enum"].([]any)
	if !ok {
		return nil, fmt.Errorf("standalone %s schema %s has no enum", component, name)
	}
	return values, nil
}

func factoryEventSchemaObject(root map[string]any, component string) (map[string]any, error) {
	if component == "FactoryEvent" {
		return root, nil
	}
	definitions, ok := root["$defs"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("standalone %s schema has no definitions", component)
	}
	event, ok := definitions["FactoryEvent"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("standalone %s schema has no FactoryEvent definition", component)
	}
	return event, nil
}

func detachFactoryEventAnnotations(components map[string]any) (map[string]any, error) {
	event, ok := components["FactoryEvent"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canonical OpenAPI contract has no FactoryEvent schema")
	}
	discriminator, ok := event["discriminator"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canonical FactoryEvent schema has no discriminator")
	}
	delete(event, "discriminator")
	return discriminator, nil
}

func stripOpenAPIExtensions(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.HasPrefix(key, "x-") || isOpenAPIOnlyAnnotation(key) {
				delete(typed, key)
				continue
			}
			stripOpenAPIExtensions(child)
		}
	case []any:
		for _, child := range typed {
			stripOpenAPIExtensions(child)
		}
	}
}

func isOpenAPIOnlyAnnotation(key string) bool {
	switch key {
	case "deprecated", "example", "externalDocs", "readOnly", "writeOnly", "xml":
		return true
	default:
		return false
	}
}

func stripOpenAPIReferenceSiblings(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if reference, ok := typed["$ref"]; ok {
			for key := range typed {
				delete(typed, key)
			}
			typed["$ref"] = reference
			return
		}
		for _, child := range typed {
			stripOpenAPIReferenceSiblings(child)
		}
	case []any:
		for _, child := range typed {
			stripOpenAPIReferenceSiblings(child)
		}
	}
}

func restoreFactoryEventDiscriminator(
	root map[string]any,
	component string,
	discriminator map[string]any,
) error {
	rewritten := deepCopyValue(discriminator).(map[string]any)
	if mapping, ok := rewritten["mapping"].(map[string]any); ok {
		for eventType, value := range mapping {
			reference, ok := value.(string)
			if !ok || !strings.HasPrefix(reference, openAPIComponentPrefix) {
				return fmt.Errorf("FactoryEvent discriminator mapping %s has unsupported reference", eventType)
			}
			mapping[eventType] = "#/$defs/" + strings.TrimPrefix(reference, openAPIComponentPrefix)
		}
	}
	if component == "FactoryEvent" {
		root["discriminator"] = rewritten
		return nil
	}
	definitions, ok := root["$defs"].(map[string]any)
	if !ok {
		return fmt.Errorf("standalone %s schema has no definitions", component)
	}
	event, ok := definitions["FactoryEvent"].(map[string]any)
	if !ok {
		return fmt.Errorf("standalone %s schema has no FactoryEvent definition", component)
	}
	event["discriminator"] = rewritten
	return nil
}

func standaloneSchemaDiagnosticsError(
	component string,
	diagnostics []contractvalidator.Diagnostic,
) error {
	payload, err := json.Marshal(diagnostics)
	if err != nil {
		return fmt.Errorf("encode %s schema converter diagnostics: %w", component, err)
	}
	return fmt.Errorf("convert canonical %s schema: %s", component, payload)
}
