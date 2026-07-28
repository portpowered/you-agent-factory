package factoryeventkinds

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
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
		"RUN_REQUEST":               "#/components/schemas/RunRequestEventPayload",
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
		[]recordings.FactoryEventType{
			recordings.FactoryEventTypeRunRequest,
			recordings.FactoryEventTypeWorkRequest,
		},
		[]string{"RunRequestEventPayload"},
	)
	if err == nil || !strings.Contains(err.Error(), "WORK_REQUEST") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing enum failure naming WORK_REQUEST, got %v", err)
	}
}

func TestValidateFactoryEventTypePayloadMappingNamesOrphanMappingKey(t *testing.T) {
	mapping, err := ParseFactoryEventTypePayloadMapping(map[string]string{
		"RUN_REQUEST":    "#/components/schemas/RunRequestEventPayload",
		"WORK_REQUEST":   "#/components/schemas/WorkRequestEventPayload",
		"ORPHANED_EVENT": "#/components/schemas/FactoryChangeEventPayload",
	})
	if err != nil {
		t.Fatalf("parse mapping: %v", err)
	}

	err = ValidateFactoryEventTypePayloadMapping(
		mapping,
		[]recordings.FactoryEventType{
			recordings.FactoryEventTypeRunRequest,
			recordings.FactoryEventTypeWorkRequest,
		},
		[]string{"RunRequestEventPayload", "WorkRequestEventPayload", "FactoryChangeEventPayload"},
	)
	if err == nil || !strings.Contains(err.Error(), "ORPHANED_EVENT") || !strings.Contains(err.Error(), "orphan") {
		t.Fatalf("expected orphan mapping failure naming ORPHANED_EVENT, got %v", err)
	}
}

func loadBundledFactoryEventDiscriminatorContract(t *testing.T) (map[string]string, []string, []recordings.FactoryEventType) {
	t.Helper()

	openAPIPath := bundledOpenAPIPath(t)
	data, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read bundled openapi contract %s: %v", openAPIPath, err)
	}

	schemas, err := parseOpenAPIComponentsSchemas(data)
	if err != nil {
		t.Fatal(err)
	}
	enumValues, err := parseFactoryEventTypeEnumFromSchemas(schemas)
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := parseFactoryEventDiscriminatorMappingFromSchemas(schemas)
	if err != nil {
		t.Fatal(err)
	}
	payloadUnionSchemaNames, err := parseFactoryEventPayloadUnionSchemaNamesFromSchemas(schemas)
	if err != nil {
		t.Fatal(err)
	}

	return mapping, payloadUnionSchemaNames, enumValues
}

func bundledOpenAPIPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../api/openapi.yaml"))
}
