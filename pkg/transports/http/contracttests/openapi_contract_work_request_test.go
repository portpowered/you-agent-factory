package apicontract_test

import "testing"

func assertWorkRequestSurfaceSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	workRequestSchema := schemaObject(t, schemas, "WorkRequest")
	assertRequiredFields(t, workRequestSchema, "requestId", "type")
	workRequestProperties := schemaProperties(t, workRequestSchema, "WorkRequest")
	assertSchemaPropertiesPresent(t, workRequestProperties, "WorkRequest", "requestId", "currentChainingTraceId", "type", "works", "relations")
	assertArrayItemRef(t, workRequestProperties, "relations", "#/components/schemas/WorkRequestRelation")
	workRequestRelation := schemaObject(t, schemas, "WorkRequestRelation")
	assertRequiredFields(t, workRequestRelation, "type", "sourceWorkName")
	workRequestRelationProperties := schemaProperties(t, workRequestRelation, "WorkRequestRelation")
	assertSchemaPropertiesPresent(t, workRequestRelationProperties, "WorkRequestRelation", "type", "sourceWorkName", "targetWorkId", "targetWorkName", "requiredState")
	if anyOf, ok := workRequestRelation["anyOf"].([]any); !ok || len(anyOf) != 2 {
		t.Fatalf("WorkRequestRelation.anyOf = %#v, want name-or-ID alternatives", workRequestRelation["anyOf"])
	}
	workRequestType := schemaObject(t, schemas, "WorkRequestType")
	assertEnumValues(t, workRequestType, "WorkRequestType", []string{"FACTORY_REQUEST_BATCH"})
	workRequestTypeVarNames, ok := workRequestType["x-enum-varnames"].([]any)
	if !ok {
		t.Fatalf("components.schemas.WorkRequestType.x-enum-varnames is missing")
	}
	if containsString(workRequestTypeVarNames, "WorkRequestTypeDefault") {
		t.Fatalf("components.schemas.WorkRequestType must not advertise legacy DEFAULT request type")
	}

	workSchema := schemaObject(t, schemas, "Work")
	workProperties := schemaProperties(t, workSchema, "Work")
	assertSchemaPropertiesPresent(t, workProperties, "Work", "name", "workId", "requestId", "workTypeName", "state", "currentChainingTraceId", "previousChainingTraceIds", "traceId", "content", "payload", "tags", "relations")
	assertPropertyRef(t, workProperties, "content", "#/components/schemas/WorkContent")
	assertArrayItemRef(t, workProperties, "relations", "#/components/schemas/Relation")
	assertPropertiesAbsent(t, workProperties, "Work", "work_type_id", "target_state")
}
