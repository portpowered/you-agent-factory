package factoryeventkinds

import (
	"fmt"
	"sort"
	"strings"

	factorycontracts "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"gopkg.in/yaml.v3"
)

// FactoryEventTypePayloadMappingEntry names one FactoryEventType discriminator
// key and the OpenAPI payload schema component selected for that type.
type FactoryEventTypePayloadMappingEntry struct {
	EventType     factorycontracts.FactoryEventType
	PayloadSchema string
}

// OpenAPISchemaNameFromRef extracts the components.schemas name from a bundled
// OpenAPI schema ref such as "#/components/schemas/RunRequestEventPayload".
func OpenAPISchemaNameFromRef(ref string) (string, error) {
	const prefix = "#/components/schemas/"
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, prefix) {
		return "", fmt.Errorf("openapi schema ref %q is not under %s", ref, prefix)
	}
	name := strings.TrimPrefix(ref, prefix)
	if name == "" {
		return "", fmt.Errorf("openapi schema ref %q has an empty schema name", ref)
	}
	return name, nil
}

// ParseFactoryEventTypePayloadMapping converts authored discriminator.mapping
// entries into typed FactoryEventType to payload-schema mappings.
func ParseFactoryEventTypePayloadMapping(rawMapping map[string]string) ([]FactoryEventTypePayloadMappingEntry, error) {
	if len(rawMapping) == 0 {
		return nil, fmt.Errorf("factory event type payload mapping is empty")
	}

	entries := make([]FactoryEventTypePayloadMappingEntry, 0, len(rawMapping))
	seenTypes := make(map[factorycontracts.FactoryEventType]struct{}, len(rawMapping))
	seenPayloadSchemas := make(map[string]factorycontracts.FactoryEventType, len(rawMapping))

	for eventTypeValue, payloadRef := range rawMapping {
		eventType := factorycontracts.FactoryEventType(eventTypeValue)
		if eventType == "" {
			return nil, fmt.Errorf("factory event type payload mapping contains an empty event type key")
		}
		if _, ok := seenTypes[eventType]; ok {
			return nil, fmt.Errorf("factory event type payload mapping contains duplicate event type %q", eventType)
		}
		seenTypes[eventType] = struct{}{}

		payloadSchema, err := OpenAPISchemaNameFromRef(payloadRef)
		if err != nil {
			return nil, fmt.Errorf("factory event type %q payload mapping: %w", eventType, err)
		}
		if priorType, ok := seenPayloadSchemas[payloadSchema]; ok {
			return nil, fmt.Errorf(
				"factory event type payload mapping maps duplicate payload schema %q for types %q and %q",
				payloadSchema,
				priorType,
				eventType,
			)
		}
		seenPayloadSchemas[payloadSchema] = eventType

		entries = append(entries, FactoryEventTypePayloadMappingEntry{
			EventType:     eventType,
			PayloadSchema: payloadSchema,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].EventType < entries[j].EventType
	})
	return entries, nil
}

// ValidateFactoryEventTypePayloadMapping fails closed when a FactoryEventType
// enum value lacks a mapping entry, a mapping key is orphaned from the enum,
// or a mapped payload schema is absent from the authored payload oneOf union.
func ValidateFactoryEventTypePayloadMapping(
	mapping []FactoryEventTypePayloadMappingEntry,
	enumValues []factorycontracts.FactoryEventType,
	payloadUnionSchemaNames []string,
) error {
	mappingByType := make(map[factorycontracts.FactoryEventType]FactoryEventTypePayloadMappingEntry, len(mapping))
	for _, entry := range mapping {
		mappingByType[entry.EventType] = entry
	}

	unionSet := make(map[string]struct{}, len(payloadUnionSchemaNames))
	for _, schemaName := range payloadUnionSchemaNames {
		unionSet[schemaName] = struct{}{}
	}

	enumSet := make(map[factorycontracts.FactoryEventType]struct{}, len(enumValues))
	for _, eventType := range enumValues {
		enumSet[eventType] = struct{}{}
		entry, ok := mappingByType[eventType]
		if !ok {
			return fmt.Errorf("factory event type %q is missing from OpenAPI type payload mapping", eventType)
		}
		if _, ok := unionSet[entry.PayloadSchema]; !ok {
			return fmt.Errorf(
				"factory event type %q maps to payload schema %q, which is missing from FactoryEvent.payload oneOf",
				eventType,
				entry.PayloadSchema,
			)
		}
	}

	for _, entry := range mapping {
		if _, ok := enumSet[entry.EventType]; !ok {
			return fmt.Errorf("factory event type payload mapping contains orphan event type %q", entry.EventType)
		}
	}

	return nil
}

func parseOpenAPIComponentsSchemas(openAPIYAML []byte) (map[string]any, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(openAPIYAML, &doc); err != nil {
		return nil, fmt.Errorf("parse bundled openapi contract: %w", err)
	}
	components, ok := doc["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("components object is missing")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("components.schemas object is missing")
	}
	return schemas, nil
}

func parseFactoryEventTypeEnumFromSchemas(schemas map[string]any) ([]factorycontracts.FactoryEventType, error) {
	eventTypeSchema, ok := schemas["FactoryEventType"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("components.schemas.FactoryEventType is missing")
	}
	rawEnum, ok := eventTypeSchema["enum"].([]any)
	if !ok {
		return nil, fmt.Errorf("FactoryEventType.enum is missing")
	}
	enumValues := make([]factorycontracts.FactoryEventType, 0, len(rawEnum))
	for index, value := range rawEnum {
		eventType, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("FactoryEventType.enum[%d] = %T, want string", index, value)
		}
		enumValues = append(enumValues, factorycontracts.FactoryEventType(eventType))
	}
	return enumValues, nil
}

func parseFactoryEventDiscriminatorMappingFromSchemas(schemas map[string]any) (map[string]string, error) {
	factoryEvent, ok := schemas["FactoryEvent"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("components.schemas.FactoryEvent is missing")
	}
	discriminator, ok := factoryEvent["discriminator"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("FactoryEvent.discriminator is missing")
	}
	if got, _ := discriminator["propertyName"].(string); got != "type" {
		return nil, fmt.Errorf("FactoryEvent.discriminator.propertyName = %q, want type", got)
	}
	rawMapping, ok := discriminator["mapping"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("FactoryEvent.discriminator.mapping is missing")
	}
	mapping := make(map[string]string, len(rawMapping))
	for eventType, payloadRefValue := range rawMapping {
		payloadRef, ok := payloadRefValue.(string)
		if !ok {
			return nil, fmt.Errorf(
				"FactoryEvent.discriminator.mapping[%q] = %T, want string",
				eventType,
				payloadRefValue,
			)
		}
		mapping[eventType] = payloadRef
	}
	return mapping, nil
}

func parseFactoryEventPayloadUnionSchemaNamesFromSchemas(schemas map[string]any) ([]string, error) {
	factoryEvent, ok := schemas["FactoryEvent"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("components.schemas.FactoryEvent is missing")
	}
	properties, ok := factoryEvent["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("FactoryEvent.properties is missing")
	}
	payloadProperty, ok := properties["payload"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("FactoryEvent.properties.payload is missing")
	}
	oneOf, ok := payloadProperty["oneOf"].([]any)
	if !ok {
		return nil, fmt.Errorf("FactoryEvent.properties.payload.oneOf is missing")
	}
	payloadUnionSchemaNames := make([]string, 0, len(oneOf))
	for index, item := range oneOf {
		refObject, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("FactoryEvent.properties.payload.oneOf[%d] = %T, want object", index, item)
		}
		ref, ok := refObject["$ref"].(string)
		if !ok {
			return nil, fmt.Errorf("FactoryEvent.properties.payload.oneOf[%d].$ref is missing", index)
		}
		schemaName, err := OpenAPISchemaNameFromRef(ref)
		if err != nil {
			return nil, fmt.Errorf("FactoryEvent.properties.payload.oneOf[%d]: %w", index, err)
		}
		payloadUnionSchemaNames = append(payloadUnionSchemaNames, schemaName)
	}
	return payloadUnionSchemaNames, nil
}

func factoryEventKindParityInputFromSchemas(schemas map[string]any) (FactoryEventKindParityInput, error) {
	mapping, err := parseFactoryEventDiscriminatorMappingFromSchemas(schemas)
	if err != nil {
		return FactoryEventKindParityInput{}, err
	}
	parsedMapping, err := ParseFactoryEventTypePayloadMapping(mapping)
	if err != nil {
		return FactoryEventKindParityInput{}, err
	}
	openAPIMappingKinds := make([]factorycontracts.FactoryEventType, 0, len(parsedMapping))
	for _, entry := range parsedMapping {
		openAPIMappingKinds = append(openAPIMappingKinds, entry.EventType)
	}
	return FactoryEventKindParityInput{
		RuntimeKinds:        PublicEmittableFactoryEventKinds(),
		ContractOnlyKinds:   ContractOnlyFactoryEventKinds(),
		OpenAPIMappingKinds: openAPIMappingKinds,
	}, nil
}

// LoadFactoryEventKindParityInputFromOpenAPIYAML reads the bundled OpenAPI
// FactoryEvent discriminator mapping and enum values needed for runtime↔contract
// parity checks.
func LoadFactoryEventKindParityInputFromOpenAPIYAML(openAPIYAML []byte) (FactoryEventKindParityInput, error) {
	schemas, err := parseOpenAPIComponentsSchemas(openAPIYAML)
	if err != nil {
		return FactoryEventKindParityInput{}, err
	}
	return factoryEventKindParityInputFromSchemas(schemas)
}
