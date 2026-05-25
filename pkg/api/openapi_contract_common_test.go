package api

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

var canonicalFactoryEventTypeValues = []string{
	"RUN_REQUEST",
	"INITIAL_STRUCTURE_REQUEST",
	"FACTORY_CHANGE",
	"WORK_REQUEST",
	"RELATIONSHIP_CHANGE_REQUEST",
	"DISPATCH_REQUEST",
	"MODEL_REQUEST",
	"MODEL_RESPONSE",
	"INFERENCE_REQUEST",
	"INFERENCE_RESPONSE",
	"SCRIPT_REQUEST",
	"SCRIPT_RESPONSE",
	"DISPATCH_RESPONSE",
	"FACTORY_STATE_RESPONSE",
	"RUN_RESPONSE",
}

var retiredFactoryEventTypeValues = []string{
	"RUN_STARTED",
	"INITIAL_STRUCTURE",
	"RELATIONSHIP_CHANGE",
	"DISPATCH_CREATED",
	"DISPATCH_COMPLETED",
	"FACTORY_STATE_CHANGE",
	"RUN_FINISHED",
}

var bundledFactoryEventContractSchemaNames = []string{
	"FactoryEvent",
	"FactoryEventContext",
	"FactoryEventType",
	"DispatchConsumedWorkRef",
	"DispatchRequestEventMetadata",
	"FactoryState",
	"InferenceOutcome",
	"Diagnostics",
	"ProviderDiagnostic",
	"WorkFailureFamily",
	"WorkFailureType",
	"ProviderFailureMetadata",
	"ProviderSessionMetadata",
	"RenderedPromptDiagnostic",
	"SafeWorkDiagnostics",
	"WallClock",
	"WorkDiagnostics",
	"WorkMetrics",
	"WorkOutcome",
	"RunRequestEventPayload",
	"InitialStructureRequestEventPayload",
	"FactoryChangeEventPayload",
	"WorkRequestEventPayload",
	"RelationshipChangeRequestEventPayload",
	"DispatchRequestEventPayload",
	"ModelRequestEventPayload",
	"ModelResponseEventPayload",
	"InferenceRequestEventPayload",
	"InferenceResponseEventPayload",
	"ScriptRequestEventPayload",
	"ScriptResponseEventPayload",
	"ScriptExecutionOutcome",
	"ScriptFailureType",
	"DispatchResponseEventPayload",
	"FactoryStateResponseEventPayload",
	"RunResponseEventPayload",
}

var bundledFactoryEventPayloadRefs = []string{
	"#/components/schemas/RunRequestEventPayload",
	"#/components/schemas/InitialStructureRequestEventPayload",
	"#/components/schemas/FactoryChangeEventPayload",
	"#/components/schemas/WorkRequestEventPayload",
	"#/components/schemas/RelationshipChangeRequestEventPayload",
	"#/components/schemas/DispatchRequestEventPayload",
	"#/components/schemas/ModelRequestEventPayload",
	"#/components/schemas/ModelResponseEventPayload",
	"#/components/schemas/InferenceRequestEventPayload",
	"#/components/schemas/InferenceResponseEventPayload",
	"#/components/schemas/ScriptRequestEventPayload",
	"#/components/schemas/ScriptResponseEventPayload",
	"#/components/schemas/DispatchResponseEventPayload",
	"#/components/schemas/FactoryStateResponseEventPayload",
	"#/components/schemas/RunResponseEventPayload",
}

var canonicalFactoryEventPayloadSchemaNamesByType = map[string]string{
	"RUN_REQUEST":                 "RunRequestEventPayload",
	"INITIAL_STRUCTURE_REQUEST":   "InitialStructureRequestEventPayload",
	"FACTORY_CHANGE":              "FactoryChangeEventPayload",
	"WORK_REQUEST":                "WorkRequestEventPayload",
	"RELATIONSHIP_CHANGE_REQUEST": "RelationshipChangeRequestEventPayload",
	"DISPATCH_REQUEST":            "DispatchRequestEventPayload",
	"MODEL_REQUEST":               "ModelRequestEventPayload",
	"MODEL_RESPONSE":              "ModelResponseEventPayload",
	"INFERENCE_REQUEST":           "InferenceRequestEventPayload",
	"INFERENCE_RESPONSE":          "InferenceResponseEventPayload",
	"SCRIPT_REQUEST":              "ScriptRequestEventPayload",
	"SCRIPT_RESPONSE":             "ScriptResponseEventPayload",
	"DISPATCH_RESPONSE":           "DispatchResponseEventPayload",
	"FACTORY_STATE_RESPONSE":      "FactoryStateResponseEventPayload",
	"RUN_RESPONSE":                "RunResponseEventPayload",
}

const openAPISchemaRefPrefix = "#/components/schemas/"

func loadBundledOpenAPIDocument(t *testing.T) map[string]any {
	t.Helper()

	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}
	return doc
}

func loadBundledOpenAPIComponentSchemas(t *testing.T) map[string]any {
	t.Helper()

	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)
	assertProjectionSchemasPresent(t, schemas)
	assertWorkstationRequestProjectionSliceSchema(t, schemas)
	assertWorkstationRequestViewSchema(t, schemas)
	assertWorkstationRequestRequestSchema(t, schemas)
	assertWorkstationRequestWorkRefSchemas(t, schemas)
	assertWorkstationRequestResponseSchema(t, schemas)
	return schemas
}

func loadValidatedOpenAPIContract(t *testing.T) *openapi3.T {
	t.Helper()

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load openapi contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}
	return doc
}

func componentSchemas(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()

	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("components object is missing")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas object is missing")
	}
	return schemas
}

func openAPISchemaNameFromRef(ref any) (string, bool) {
	refString, ok := ref.(string)
	if !ok || !strings.HasPrefix(refString, openAPISchemaRefPrefix) {
		return "", false
	}
	return strings.TrimPrefix(refString, openAPISchemaRefPrefix), true
}

func schemaObject(t *testing.T, schemas map[string]any, schemaName string) map[string]any {
	t.Helper()

	schema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.%s must be an object schema", schemaName)
	}
	return schema
}

func schemaProperties(t *testing.T, schema map[string]any, schemaName string) map[string]any {
	t.Helper()

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s.properties is missing", schemaName)
	}
	return properties
}

func collectSnakeCaseComponentFields(t *testing.T, schemas map[string]any, rootSchemas []string) []string {
	t.Helper()

	visited := make(map[string]bool)
	offenses := make(map[string]struct{})
	for _, schemaName := range rootSchemas {
		collectSnakeCaseFieldsFromComponent(t, schemas, schemaName, visited, offenses)
	}

	out := make([]string, 0, len(offenses))
	for offense := range offenses {
		out = append(out, offense)
	}
	sort.Strings(out)
	return out
}

func collectSnakeCaseFieldsFromComponent(
	t *testing.T,
	schemas map[string]any,
	schemaName string,
	visited map[string]bool,
	offenses map[string]struct{},
) {
	t.Helper()

	if visited[schemaName] {
		return
	}
	visited[schemaName] = true
	collectSnakeCaseFieldsFromSchema(t, schemas, schemaName, schemaObject(t, schemas, schemaName), visited, offenses)
}

func collectSnakeCaseFieldsFromSchema(
	t *testing.T,
	schemas map[string]any,
	path string,
	schema map[string]any,
	visited map[string]bool,
	offenses map[string]struct{},
) {
	t.Helper()

	if properties, ok := schema["properties"].(map[string]any); ok {
		for propertyName, propertyAny := range properties {
			if strings.Contains(propertyName, "_") {
				offenses[path+"."+propertyName] = struct{}{}
			}
			propertySchema, ok := propertyAny.(map[string]any)
			if !ok {
				continue
			}
			collectSnakeCaseFieldsFromSubSchema(t, schemas, propertySchema, visited, offenses)
		}
	}
	if additionalProperties, ok := schema["additionalProperties"].(map[string]any); ok {
		collectSnakeCaseFieldsFromSubSchema(t, schemas, additionalProperties, visited, offenses)
	}
}

func collectSnakeCaseFieldsFromSubSchema(
	t *testing.T,
	schemas map[string]any,
	schema map[string]any,
	visited map[string]bool,
	offenses map[string]struct{},
) {
	t.Helper()

	if refName, ok := openAPISchemaNameFromRef(schema["$ref"]); ok {
		collectSnakeCaseFieldsFromComponent(t, schemas, refName, visited, offenses)
	}
	if items, ok := schema["items"].(map[string]any); ok {
		collectSnakeCaseFieldsFromSubSchema(t, schemas, items, visited, offenses)
	}
	for _, compositionKey := range []string{"allOf", "anyOf", "oneOf"} {
		items, ok := schema[compositionKey].([]any)
		if !ok {
			continue
		}
		for _, item := range items {
			itemSchema, ok := item.(map[string]any)
			if !ok {
				continue
			}
			collectSnakeCaseFieldsFromSubSchema(t, schemas, itemSchema, visited, offenses)
		}
	}
	if _, ok := schema["properties"].(map[string]any); ok {
		collectSnakeCaseFieldsFromSchema(t, schemas, "<inline>", schema, visited, offenses)
	}
	if additionalProperties, ok := schema["additionalProperties"].(map[string]any); ok {
		collectSnakeCaseFieldsFromSubSchema(t, schemas, additionalProperties, visited, offenses)
	}
}

func requireOpenAPI3ComponentSchema(t *testing.T, doc *openapi3.T, schemaName string) *openapi3.Schema {
	t.Helper()

	schemaRef, ok := doc.Components.Schemas[schemaName]
	if !ok || schemaRef == nil || schemaRef.Value == nil {
		t.Fatalf("components.schemas.%s is missing", schemaName)
	}
	return schemaRef.Value
}

func assertOpenAPI3Description(t *testing.T, path string, description string) {
	t.Helper()
	if strings.TrimSpace(description) == "" {
		t.Fatalf("%s description is empty", path)
	}
}

func assertOpenAPI3PropertyDescription(t *testing.T, schema *openapi3.Schema, schemaName string, propertyName string) *openapi3.Schema {
	t.Helper()
	property, ok := schema.Properties[propertyName]
	if !ok || property == nil || property.Value == nil {
		t.Fatalf("%s.properties.%s is missing", schemaName, propertyName)
	}
	assertOpenAPI3Description(t, schemaName+".properties."+propertyName, property.Value.Description)
	return property.Value
}

func assertOpenAPI3ArrayPropertyDescription(t *testing.T, schema *openapi3.Schema, schemaName string, propertyName string) *openapi3.Schema {
	t.Helper()
	property := assertOpenAPI3PropertyDescription(t, schema, schemaName, propertyName)
	if property.Items == nil || property.Items.Value == nil {
		t.Fatalf("%s.properties.%s.items is missing", schemaName, propertyName)
	}
	return property.Items.Value
}

func assertOpenAPI3RefPropertyDescription(t *testing.T, schema *openapi3.Schema, schemaName string, propertyName string) *openapi3.Schema {
	t.Helper()
	return assertOpenAPI3PropertyDescription(t, schema, schemaName, propertyName)
}

func assertOpenAPI3PropertyRef(t *testing.T, schema *openapi3.Schema, schemaName string, propertyName string, wantRef string) {
	t.Helper()
	property, ok := schema.Properties[propertyName]
	if !ok || property == nil {
		t.Fatalf("%s.properties.%s is missing", schemaName, propertyName)
	}
	if property.Ref != wantRef {
		t.Fatalf("%s.properties.%s.$ref = %q, want %q", schemaName, propertyName, property.Ref, wantRef)
	}
}

func assertRequiredStringValues(t *testing.T, got []string, want ...string) {
	t.Helper()
	for _, wantValue := range want {
		found := false
		for _, gotValue := range got {
			if gotValue == wantValue {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("required fields are missing %q", wantValue)
		}
	}
}

func assertSchemaNamesPresent(t *testing.T, schemas map[string]any, names []string) {
	t.Helper()
	for _, name := range names {
		if _, ok := schemas[name]; !ok {
			t.Fatalf("components.schemas.%s is missing", name)
		}
	}
}

func containsString(values []any, want string) bool {
	for _, value := range values {
		if got, ok := value.(string); ok && got == want {
			return true
		}
	}
	return false
}

func assertSchemaPropertiesPresent(t *testing.T, properties map[string]any, schemaName string, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := properties[field].(map[string]any); !ok {
			t.Fatalf("%s.properties.%s is missing", schemaName, field)
		}
	}
}

func assertRequiredFields(t *testing.T, schema map[string]any, fields ...string) {
	t.Helper()
	requiredFields, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema.required is missing")
	}
	for _, field := range fields {
		if !containsString(requiredFields, field) {
			t.Fatalf("schema.required is missing %q", field)
		}
	}
}

func assertEnumValues(t *testing.T, schema map[string]any, schemaName string, values []string) {
	t.Helper()
	enumValues, ok := schema["enum"].([]any)
	if !ok {
		t.Fatalf("%s.enum is missing", schemaName)
	}
	if len(enumValues) != len(values) {
		t.Fatalf("%s.enum has %d values, want %d", schemaName, len(enumValues), len(values))
	}
	for _, value := range values {
		if !containsString(enumValues, value) {
			t.Fatalf("%s.enum is missing %q", schemaName, value)
		}
	}
}

func assertEnumOmitValues(t *testing.T, schema map[string]any, schemaName string, values []string) {
	t.Helper()
	enumValues, ok := schema["enum"].([]any)
	if !ok {
		t.Fatalf("%s.enum is missing", schemaName)
	}
	for _, value := range values {
		if containsString(enumValues, value) {
			t.Fatalf("%s.enum must not contain %q", schemaName, value)
		}
	}
}

func assertPayloadUnionRefs(t *testing.T, properties map[string]any, wantRefs []string) {
	t.Helper()
	payload, ok := properties["payload"].(map[string]any)
	if !ok {
		t.Fatalf("FactoryEvent.properties.payload is missing")
	}
	oneOf, ok := payload["oneOf"].([]any)
	if !ok {
		t.Fatalf("FactoryEvent.properties.payload.oneOf is missing")
	}
	if len(oneOf) != len(wantRefs) {
		t.Fatalf("FactoryEvent payload union has %d refs, want %d", len(oneOf), len(wantRefs))
	}
	for _, wantRef := range wantRefs {
		found := false
		for _, item := range oneOf {
			refObject, ok := item.(map[string]any)
			if ok && refObject["$ref"] == wantRef {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("FactoryEvent payload union is missing %s", wantRef)
		}
	}
}

func assertPropertyRef(t *testing.T, properties map[string]any, propertyName string, wantRef string) {
	t.Helper()
	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s is missing", propertyName)
	}
	if got, ok := property["$ref"].(string); ok {
		if got != wantRef {
			t.Fatalf("properties.%s.$ref = %v, want %s", propertyName, got, wantRef)
		}
		return
	}
	allOf, ok := property["allOf"].([]any)
	if !ok || len(allOf) == 0 {
		t.Fatalf("properties.%s has neither $ref nor allOf", propertyName)
	}
	first, ok := allOf[0].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s.allOf[0] must be an object", propertyName)
	}
	if got, ok := first["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("properties.%s ref = %v, want %s", propertyName, first["$ref"], wantRef)
	}
}

func assertSchemaRef(t *testing.T, schemas map[string]any, schemaName string, wantRef string) {
	t.Helper()
	schema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.%s must be an object schema", schemaName)
	}
	if got, ok := schema["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("components.schemas.%s.$ref = %v, want %s", schemaName, schema["$ref"], wantRef)
	}
}

func assertArrayItemRef(t *testing.T, properties map[string]any, propertyName string, wantRef string) {
	t.Helper()
	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s is missing", propertyName)
	}
	items, ok := property["items"].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s.items is missing", propertyName)
	}
	if got, ok := items["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("properties.%s.items.$ref = %v, want %s", propertyName, items["$ref"], wantRef)
	}
}

func assertSchemaOneOfRefs(t *testing.T, schema map[string]any, schemaName string, wantRefs []string) {
	t.Helper()
	oneOf, ok := schema["oneOf"].([]any)
	if !ok {
		t.Fatalf("%s.oneOf is missing", schemaName)
	}
	if len(oneOf) != len(wantRefs) {
		t.Fatalf("%s.oneOf has %d refs, want %d", schemaName, len(oneOf), len(wantRefs))
	}
	for _, wantRef := range wantRefs {
		found := false
		for _, item := range oneOf {
			refObject, ok := item.(map[string]any)
			if ok && refObject["$ref"] == wantRef {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s.oneOf is missing %s", schemaName, wantRef)
		}
	}
}

func assertParameterRef(t *testing.T, parameters []any, wantRef string) {
	t.Helper()
	for _, parameter := range parameters {
		ref, ok := parameter.(map[string]any)["$ref"].(string)
		if ok && ref == wantRef {
			return
		}
	}
	t.Fatalf("operation parameters missing %s: %#v", wantRef, parameters)
}

func assertStringArrayProperty(t *testing.T, properties map[string]any, propertyName string) {
	t.Helper()
	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s is missing", propertyName)
	}
	items, ok := property["items"].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s.items is missing", propertyName)
	}
	if got, ok := items["type"].(string); !ok || got != "string" {
		t.Fatalf("properties.%s.items.type = %v, want string", propertyName, items["type"])
	}
}

func assertProjectionSchemasPresent(t *testing.T, schemas map[string]any) {
	t.Helper()
	for _, schema := range []string{
		"FactoryWorldWorkstationRequestProjectionSlice",
		"FactoryWorldWorkstationRequestView",
		"FactoryWorldWorkstationRequestCountView",
		"FactoryWorldWorkstationRequestRequestView",
		"FactoryWorldWorkstationRequestResponseView",
		"FactoryWorldWorkItemRef",
		"FactoryWorldTokenView",
		"FactoryWorldMutationView",
		"FactoryWorldWorkDiagnostics",
		"FactoryWorldProviderDiagnostic",
		"FactoryWorldRenderedPromptDiagnostic",
	} {
		if _, ok := schemas[schema]; !ok {
			t.Fatalf("components.schemas.%s is missing", schema)
		}
	}
}

func assertWorkstationRequestProjectionSchemasPresent(t *testing.T, schemas map[string]any) {
	t.Helper()
	for _, schema := range []string{
		"FactoryWorldWorkstationRequestProjectionSlice",
		"FactoryWorldScriptRequestView",
		"FactoryWorldScriptResponseView",
		"FactoryWorldWorkstationRequestView",
		"FactoryWorldWorkstationRequestCountView",
		"FactoryWorldWorkstationRequestRequestView",
		"FactoryWorldWorkstationRequestResponseView",
		"FactoryWorldWorkItemRef",
		"FactoryWorldTokenView",
		"FactoryWorldMutationView",
		"FactoryWorldWorkDiagnostics",
		"FactoryWorldProviderDiagnostic",
		"FactoryWorldRenderedPromptDiagnostic",
	} {
		if _, ok := schemas[schema]; !ok {
			t.Fatalf("components.schemas.%s is missing", schema)
		}
	}
}

func assertWorkstationRequestProjectionSliceSchema(t *testing.T, schemas map[string]any) {
	t.Helper()
	projectionSlice := schemaObject(t, schemas, "FactoryWorldWorkstationRequestProjectionSlice")
	sliceProperties := schemaProperties(t, projectionSlice, "FactoryWorldWorkstationRequestProjectionSlice")
	workstationRequestsByDispatchID, ok := sliceProperties["workstationRequestsByDispatchId"].(map[string]any)
	if !ok {
		t.Fatalf("FactoryWorldWorkstationRequestProjectionSlice.properties.workstationRequestsByDispatchId is missing")
	}
	additionalProperties, ok := workstationRequestsByDispatchID["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatalf("FactoryWorldWorkstationRequestProjectionSlice.properties.workstationRequestsByDispatchId.additionalProperties is missing")
	}
	if got, ok := additionalProperties["$ref"].(string); !ok || got != "#/components/schemas/FactoryWorldWorkstationRequestView" {
		t.Fatalf("FactoryWorldWorkstationRequestProjectionSlice workstation request map ref = %v, want %s", additionalProperties["$ref"], "#/components/schemas/FactoryWorldWorkstationRequestView")
	}
}

func assertWorkstationRequestViewSchema(t *testing.T, schemas map[string]any) {
	t.Helper()
	requestView := schemaObject(t, schemas, "FactoryWorldWorkstationRequestView")
	assertRequiredFields(t, requestView, "dispatchId", "transitionId", "counts", "request")
	requestViewProperties := schemaProperties(t, requestView, "FactoryWorldWorkstationRequestView")
	assertPropertyRef(t, requestViewProperties, "counts", "#/components/schemas/FactoryWorldWorkstationRequestCountView")
	assertPropertyRef(t, requestViewProperties, "request", "#/components/schemas/FactoryWorldWorkstationRequestRequestView")
	assertPropertyRef(t, requestViewProperties, "response", "#/components/schemas/FactoryWorldWorkstationRequestResponseView")

	countView := schemaObject(t, schemas, "FactoryWorldWorkstationRequestCountView")
	assertRequiredFields(t, countView, "dispatchedCount", "respondedCount", "erroredCount")
}

func assertWorkstationRequestRequestSchema(t *testing.T, schemas map[string]any) {
	t.Helper()
	requestPayload := schemaObject(t, schemas, "FactoryWorldWorkstationRequestRequestView")
	requestPayloadProperties := schemaProperties(t, requestPayload, "FactoryWorldWorkstationRequestRequestView")
	assertSchemaPropertiesPresent(t, requestPayloadProperties, "FactoryWorldWorkstationRequestRequestView", "startedAt", "currentChainingTraceId")
	assertStringArrayProperty(t, requestPayloadProperties, "previousChainingTraceIds")
	assertArrayItemRef(t, requestPayloadProperties, "inputWorkItems", "#/components/schemas/FactoryWorldWorkItemRef")
	assertArrayItemRef(t, requestPayloadProperties, "consumedTokens", "#/components/schemas/FactoryWorldTokenView")
}

func assertWorkstationRequestWorkRefSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	workItemRef := schemaObject(t, schemas, "FactoryWorldWorkItemRef")
	workItemRefProperties := schemaProperties(t, workItemRef, "FactoryWorldWorkItemRef")
	assertSchemaPropertiesPresent(t, workItemRefProperties, "FactoryWorldWorkItemRef", "workId", "workTypeId", "displayName", "traceId", "currentChainingTraceId")
	assertStringArrayProperty(t, workItemRefProperties, "previousChainingTraceIds")

	tokenView := schemaObject(t, schemas, "FactoryWorldTokenView")
	tokenViewProperties := schemaProperties(t, tokenView, "FactoryWorldTokenView")
	assertSchemaPropertiesPresent(t, tokenViewProperties, "FactoryWorldTokenView", "tokenId", "placeId", "workId", "workTypeId", "traceId", "currentChainingTraceId")
	assertStringArrayProperty(t, tokenViewProperties, "previousChainingTraceIds")
}

func assertWorkstationRequestPayloadSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	requestPayload := schemaObject(t, schemas, "FactoryWorldWorkstationRequestRequestView")
	requestPayloadProperties := schemaProperties(t, requestPayload, "FactoryWorldWorkstationRequestRequestView")
	assertSchemaPropertiesPresent(t, requestPayloadProperties, "FactoryWorldWorkstationRequestRequestView", "startedAt")
	assertArrayItemRef(t, requestPayloadProperties, "inputWorkItems", "#/components/schemas/FactoryWorldWorkItemRef")
	assertArrayItemRef(t, requestPayloadProperties, "consumedTokens", "#/components/schemas/FactoryWorldTokenView")
	assertPropertyRef(t, requestPayloadProperties, "scriptRequest", "#/components/schemas/FactoryWorldScriptRequestView")

	responsePayload := schemaObject(t, schemas, "FactoryWorldWorkstationRequestResponseView")
	responsePayloadProperties := schemaProperties(t, responsePayload, "FactoryWorldWorkstationRequestResponseView")
	assertPropertyRef(t, responsePayloadProperties, "scriptResponse", "#/components/schemas/FactoryWorldScriptResponseView")
	assertArrayItemRef(t, responsePayloadProperties, "outputWorkItems", "#/components/schemas/FactoryWorldWorkItemRef")
	assertArrayItemRef(t, responsePayloadProperties, "outputMutations", "#/components/schemas/FactoryWorldMutationView")
	assertSchemaPropertiesPresent(t, responsePayloadProperties, "FactoryWorldWorkstationRequestResponseView", "outcome", "feedback", "failureReason", "failureMessage", "endTime", "durationMillis")
}

func assertWorkstationRequestScriptBoundarySchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	scriptRequestPayload := schemaObject(t, schemas, "FactoryWorldScriptRequestView")
	scriptRequestPayloadProperties := schemaProperties(t, scriptRequestPayload, "FactoryWorldScriptRequestView")
	assertSchemaPropertiesPresent(t, scriptRequestPayloadProperties, "FactoryWorldScriptRequestView", "scriptRequestId", "attempt", "command", "args")

	scriptResponsePayload := schemaObject(t, schemas, "FactoryWorldScriptResponseView")
	scriptResponsePayloadProperties := schemaProperties(t, scriptResponsePayload, "FactoryWorldScriptResponseView")
	assertSchemaPropertiesPresent(t, scriptResponsePayloadProperties, "FactoryWorldScriptResponseView", "scriptRequestId", "attempt", "outcome", "stdout", "stderr", "durationMillis", "exitCode", "failureType")
}

func assertWorkstationRequestResponseSchema(t *testing.T, schemas map[string]any) {
	t.Helper()
	responsePayload := schemaObject(t, schemas, "FactoryWorldWorkstationRequestResponseView")
	responsePayloadProperties := schemaProperties(t, responsePayload, "FactoryWorldWorkstationRequestResponseView")
	assertArrayItemRef(t, responsePayloadProperties, "outputWorkItems", "#/components/schemas/FactoryWorldWorkItemRef")
	assertArrayItemRef(t, responsePayloadProperties, "outputMutations", "#/components/schemas/FactoryWorldMutationView")
	assertSchemaPropertiesPresent(t, responsePayloadProperties, "FactoryWorldWorkstationRequestResponseView", "outcome", "feedback", "failureReason", "failureMessage", "endTime", "durationMillis")
}

func pathOperation(t *testing.T, paths map[string]any, path string, method string) map[string]any {
	t.Helper()
	pathItem, ok := paths[path].(map[string]any)
	if !ok {
		t.Fatalf("paths.%s is missing", path)
	}
	operation, ok := pathItem[method].(map[string]any)
	if !ok {
		t.Fatalf("paths.%s.%s is missing", path, method)
	}
	return operation
}

func assertEventStreamSchemaRef(t *testing.T, operation map[string]any, wantRef string) {
	t.Helper()
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses is missing")
	}
	response, ok := responses["200"].(map[string]any)
	if !ok {
		t.Fatal("operation.responses.200 is missing")
	}
	content, ok := response["content"].(map[string]any)
	if !ok {
		t.Fatal("operation.responses.200.content is missing")
	}
	eventStream, ok := content["text/event-stream"].(map[string]any)
	if !ok {
		t.Fatal("operation.responses.200.content.text/event-stream is missing")
	}
	xEventSchema, ok := eventStream["x-event-schema"].(string)
	if !ok {
		t.Fatal("operation.responses.200.content.text/event-stream.x-event-schema is missing")
	}
	if xEventSchema != wantRef {
		t.Fatalf("operation.responses.200.content.text/event-stream.x-event-schema = %q, want %s", xEventSchema, wantRef)
	}
}

func assertResponseSchemaRef(t *testing.T, operation map[string]any, status string, wantRef string) {
	t.Helper()
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses is missing")
	}
	response, ok := responses[status].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses.%s is missing", status)
	}
	content, ok := response["content"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses.%s.content is missing", status)
	}
	applicationJSON, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses.%s.content.application/json is missing", status)
	}
	schema, ok := applicationJSON["schema"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses.%s.content.application/json.schema is missing", status)
	}
	if got, ok := schema["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("operation.responses.%s.content.application/json.schema.$ref = %v, want %s", status, schema["$ref"], wantRef)
	}
}

func assertRequestSchemaRef(t *testing.T, operation map[string]any, wantRef string) {
	t.Helper()
	requestBody, ok := operation["requestBody"].(map[string]any)
	if !ok {
		t.Fatal("operation.requestBody is missing")
	}
	content, ok := requestBody["content"].(map[string]any)
	if !ok {
		t.Fatal("operation.requestBody.content is missing")
	}
	applicationJSON, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatal("operation.requestBody.content.application/json is missing")
	}
	schema, ok := applicationJSON["schema"].(map[string]any)
	if !ok {
		t.Fatal("operation.requestBody.content.application/json.schema is missing")
	}
	if got, ok := schema["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("operation.requestBody.content.application/json.schema.$ref = %v, want %s", schema["$ref"], wantRef)
	}
}

func assertResponseRef(t *testing.T, operation map[string]any, status string, wantRef string) {
	t.Helper()
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses is missing")
	}
	response, ok := responses[status].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses.%s is missing", status)
	}
	if got, ok := response["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("operation.responses.%s.$ref = %v, want %s", status, response["$ref"], wantRef)
	}
}

func assertResponseExampleCodeFamilies(t *testing.T, responses map[string]any, responseName string, wantCodeFamilies map[string]string) {
	t.Helper()
	response, ok := responses[responseName].(map[string]any)
	if !ok {
		t.Fatalf("components.responses.%s is missing", responseName)
	}
	content, ok := response["content"].(map[string]any)
	if !ok {
		t.Fatalf("components.responses.%s.content is missing", responseName)
	}
	applicationJSON, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("components.responses.%s.content.application/json is missing", responseName)
	}
	examples, ok := applicationJSON["examples"].(map[string]any)
	if !ok {
		t.Fatalf("components.responses.%s.content.application/json.examples is missing", responseName)
	}

	seenCodeFamilies := make(map[string]string, len(examples))
	for exampleName, value := range examples {
		example, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("components.responses.%s example %s must be an object", responseName, exampleName)
		}
		payload, ok := example["value"].(map[string]any)
		if !ok {
			t.Fatalf("components.responses.%s example %s value must be an object", responseName, exampleName)
		}
		code, ok := payload["code"].(string)
		if !ok {
			t.Fatalf("components.responses.%s example %s code must be a string", responseName, exampleName)
		}
		family, ok := payload["family"].(string)
		if !ok {
			t.Fatalf("components.responses.%s example %s family must be a string", responseName, exampleName)
		}
		seenCodeFamilies[code] = family
	}

	if len(seenCodeFamilies) != len(wantCodeFamilies) {
		t.Fatalf("components.responses.%s example count = %d, want %d", responseName, len(seenCodeFamilies), len(wantCodeFamilies))
	}
	for code, wantFamily := range wantCodeFamilies {
		if gotFamily, ok := seenCodeFamilies[code]; !ok {
			t.Fatalf("components.responses.%s is missing example for code %q", responseName, code)
		} else if gotFamily != wantFamily {
			t.Fatalf("components.responses.%s example for code %q family = %q, want %q", responseName, code, gotFamily, wantFamily)
		}
	}
}

func assertPropertiesAbsent(t *testing.T, properties map[string]any, schemaName string, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := properties[field]; ok {
			t.Fatalf("%s.properties.%s must not be advertised", schemaName, field)
		}
	}
}

func assertJSONStringLiteralMissing(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if strings.Contains(haystack, `"`+needle+`"`) {
			t.Fatalf("unexpected retired string %q found in fixture", needle)
		}
	}
}

func assertStringSetsMatch(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("string set length = %d, want %d", len(got), len(want))
	}
	gotCounts := make(map[string]int, len(got))
	for _, value := range got {
		gotCounts[value]++
	}
	for _, value := range want {
		if gotCounts[value] == 0 {
			t.Fatalf("string set is missing %q", value)
		}
		gotCounts[value]--
	}
	for value, remaining := range gotCounts {
		if remaining != 0 {
			t.Fatalf("string set contains unexpected count for %q: %d", value, remaining)
		}
	}
}

func assertNoDispatchConfigCopies(t *testing.T, properties map[string]any, schemaName string) {
	t.Helper()
	for _, field := range []string{
		"model",
		"provider",
		"promptFile",
		"promptTemplate",
		"outputSchema",
		"worktree",
		"workingDirectory",
		"workerType",
		"workstationName",
		"workstationType",
	} {
		if _, ok := properties[field]; ok {
			t.Fatalf("%s.properties.%s duplicates Worker or Workstation configuration", schemaName, field)
		}
	}
}

func assertJSONKeysAbsent(t *testing.T, object map[string]any, name string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := object[key]; ok {
			t.Fatalf("%s.%s must not be present", name, key)
		}
	}
}

func assertJSONKeysPresent(t *testing.T, object map[string]any, name string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			t.Fatalf("%s.%s is missing", name, key)
		}
	}
}

func loadCanonicalFactoryEventVocabularyFixture(t *testing.T) []map[string]any {
	t.Helper()
	fixtureBytes, err := os.ReadFile("testdata/canonical-event-vocabulary-stream.json")
	if err != nil {
		t.Fatalf("read canonical event vocabulary fixture: %v", err)
	}
	assertJSONStringLiteralMissing(t, string(fixtureBytes), retiredFactoryEventTypeValues...)

	var fixture []map[string]any
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("parse canonical event vocabulary fixture: %v", err)
	}
	if len(fixture) != len(canonicalFactoryEventTypeValues) {
		t.Fatalf("canonical event vocabulary fixture length = %d, want %d", len(fixture), len(canonicalFactoryEventTypeValues))
	}
	return fixture
}
