package factoryeventkinds

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"gopkg.in/yaml.v3"
)

func TestOpenAPIFactoryEventTypePayloadMappingCoversEveryFactoryEventType(t *testing.T) {
	rawMapping, payloadUnionSchemaNames, enumValues := loadBundledFactoryEventDiscriminatorContract(t)
	mapping, err := ParseFactoryEventTypePayloadMapping(rawMapping)
	if err != nil {
		t.Fatalf("parse factory event type payload mapping: %v", err)
	}

	if err := ValidateFactoryEventTypePayloadMapping(mapping, enumValues, payloadUnionSchemaNames); err != nil {
		t.Fatal(err)
	}
}

func TestParseFactoryEventTypePayloadMappingRejectsDuplicatePayloadSchemas(t *testing.T) {
	_, err := ParseFactoryEventTypePayloadMapping(map[string]string{
		"RUN_REQUEST":              "#/components/schemas/RunRequestEventPayload",
		"INITIAL_STRUCTURE_REQUEST": "#/components/schemas/RunRequestEventPayload",
	})
	if err == nil {
		t.Fatal("expected duplicate payload schema mapping to fail")
	}
}

func TestValidateFactoryEventTypePayloadMappingNamesMissingEnumValue(t *testing.T) {
	mapping, err := ParseFactoryEventTypePayloadMapping(map[string]string{
		"RUN_REQUEST": "#/components/schemas/RunRequestEventPayload",
	})
	if err != nil {
		t.Fatalf("parse mapping: %v", err)
	}

	err = ValidateFactoryEventTypePayloadMapping(
		mapping,
		[]factoryapi.FactoryEventType{
			factoryapi.FactoryEventTypeRunRequest,
			factoryapi.FactoryEventTypeWorkRequest,
		},
		[]string{"RunRequestEventPayload"},
	)
	if err == nil || !strings.Contains(err.Error(), "WORK_REQUEST") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing enum failure naming WORK_REQUEST, got %v", err)
	}
}

func TestValidateFactoryEventTypePayloadMappingNamesOrphanMappingKey(t *testing.T) {
	mapping, err := ParseFactoryEventTypePayloadMapping(map[string]string{
		"RUN_REQUEST":     "#/components/schemas/RunRequestEventPayload",
		"WORK_REQUEST":    "#/components/schemas/WorkRequestEventPayload",
		"ORPHANED_EVENT":  "#/components/schemas/FactoryChangeEventPayload",
	})
	if err != nil {
		t.Fatalf("parse mapping: %v", err)
	}

	err = ValidateFactoryEventTypePayloadMapping(
		mapping,
		[]factoryapi.FactoryEventType{
			factoryapi.FactoryEventTypeRunRequest,
			factoryapi.FactoryEventTypeWorkRequest,
		},
		[]string{"RunRequestEventPayload", "WorkRequestEventPayload", "FactoryChangeEventPayload"},
	)
	if err == nil || !strings.Contains(err.Error(), "ORPHANED_EVENT") || !strings.Contains(err.Error(), "orphan") {
		t.Fatalf("expected orphan mapping failure naming ORPHANED_EVENT, got %v", err)
	}
}

func loadBundledFactoryEventDiscriminatorContract(t *testing.T) (map[string]string, []string, []factoryapi.FactoryEventType) {
	t.Helper()

	openAPIPath := bundledOpenAPIPath(t)
	data, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read bundled openapi contract %s: %v", openAPIPath, err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse bundled openapi contract: %v", err)
	}

	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatal("components object is missing")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("components.schemas object is missing")
	}

	eventTypeSchema, ok := schemas["FactoryEventType"].(map[string]any)
	if !ok {
		t.Fatal("components.schemas.FactoryEventType is missing")
	}
	rawEnum, ok := eventTypeSchema["enum"].([]any)
	if !ok {
		t.Fatal("FactoryEventType.enum is missing")
	}
	enumValues := make([]factoryapi.FactoryEventType, 0, len(rawEnum))
	for index, value := range rawEnum {
		eventType, ok := value.(string)
		if !ok {
			t.Fatalf("FactoryEventType.enum[%d] = %T, want string", index, value)
		}
		enumValues = append(enumValues, factoryapi.FactoryEventType(eventType))
	}

	factoryEvent, ok := schemas["FactoryEvent"].(map[string]any)
	if !ok {
		t.Fatal("components.schemas.FactoryEvent is missing")
	}
	discriminator, ok := factoryEvent["discriminator"].(map[string]any)
	if !ok {
		t.Fatal("FactoryEvent.discriminator is missing")
	}
	if got, _ := discriminator["propertyName"].(string); got != "type" {
		t.Fatalf("FactoryEvent.discriminator.propertyName = %q, want type", got)
	}
	rawMapping, ok := discriminator["mapping"].(map[string]any)
	if !ok {
		t.Fatal("FactoryEvent.discriminator.mapping is missing")
	}

	mapping := make(map[string]string, len(rawMapping))
	for eventType, payloadRefValue := range rawMapping {
		payloadRef, ok := payloadRefValue.(string)
		if !ok {
			t.Fatalf("FactoryEvent.discriminator.mapping[%q] = %T, want string", eventType, payloadRefValue)
		}
		mapping[eventType] = payloadRef
	}

	properties, ok := factoryEvent["properties"].(map[string]any)
	if !ok {
		t.Fatal("FactoryEvent.properties is missing")
	}
	payloadProperty, ok := properties["payload"].(map[string]any)
	if !ok {
		t.Fatal("FactoryEvent.properties.payload is missing")
	}
	oneOf, ok := payloadProperty["oneOf"].([]any)
	if !ok {
		t.Fatal("FactoryEvent.properties.payload.oneOf is missing")
	}

	payloadUnionSchemaNames := make([]string, 0, len(oneOf))
	for index, item := range oneOf {
		refObject, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("FactoryEvent.properties.payload.oneOf[%d] = %T, want object", index, item)
		}
		ref, ok := refObject["$ref"].(string)
		if !ok {
			t.Fatalf("FactoryEvent.properties.payload.oneOf[%d].$ref is missing", index)
		}
		schemaName, err := OpenAPISchemaNameFromRef(ref)
		if err != nil {
			t.Fatalf("FactoryEvent.properties.payload.oneOf[%d]: %v", index, err)
		}
		payloadUnionSchemaNames = append(payloadUnionSchemaNames, schemaName)
	}

	return mapping, payloadUnionSchemaNames, enumValues
}

func bundledOpenAPIPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../api/openapi.yaml"))
}
