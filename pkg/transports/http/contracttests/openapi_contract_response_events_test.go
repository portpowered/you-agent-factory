package apicontract_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"gopkg.in/yaml.v3"
)

const responseEventStreamPath = "/factory-sessions/{session_id}/response-events"

func loadAuthoredComponentFragment(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read authored component %s: %v", path, err)
	}
	var fragment map[string]any
	if err := yaml.Unmarshal(data, &fragment); err != nil {
		t.Fatalf("parse authored component %s: %v", path, err)
	}
	return fragment
}

func assertQueryParameter(t *testing.T, parameter map[string]any, name string) {
	t.Helper()
	if parameter["name"] != name || parameter["in"] != "query" || parameter["required"] != false {
		t.Fatalf("parameter %s identity = %#v", name, parameter)
	}
}

func objectField(t *testing.T, object map[string]any, field string) map[string]any {
	t.Helper()
	value, ok := object[field].(map[string]any)
	if !ok {
		t.Fatalf("field %s is missing", field)
	}
	return value
}

func assertResponseFragmentSchema(t *testing.T, response map[string]any, wantRef string) {
	t.Helper()
	content := objectField(t, response, "content")
	applicationJSON := objectField(t, content, "application/json")
	schema := objectField(t, applicationJSON, "schema")
	if got := schema["$ref"]; got != wantRef {
		t.Fatalf("response schema ref = %v, want %s", got, wantRef)
	}
}

func assertResponseFragmentExampleCodes(t *testing.T, response map[string]any, wantCodes ...string) {
	t.Helper()
	examples := objectField(t, objectField(t, objectField(t, response, "content"), "application/json"), "examples")
	gotCodes := make(map[string]bool, len(examples))
	for _, rawExample := range examples {
		example, ok := rawExample.(map[string]any)
		if !ok {
			t.Fatal("response example must be an object")
		}
		value := objectField(t, example, "value")
		code, ok := value["code"].(string)
		if !ok {
			t.Fatal("response example code must be a string")
		}
		gotCodes[code] = true
	}
	for _, wantCode := range wantCodes {
		if !gotCodes[wantCode] {
			t.Fatalf("response examples missing code %s", wantCode)
		}
	}
}

var canonicalFactoryResponseEventKindValues = []string{
	"SESSION",
	"RUN",
	"TURN",
	"MESSAGE",
	"REASONING",
	"TOOL",
	"FILE_CHANGE",
	"PLAN",
	"PROGRESS",
	"USAGE",
	"ERROR",
	"STREAM_GAP",
}

var canonicalFactoryResponseEventContentBlockKindValues = []string{
	"TEXT",
	"REASONING_SUMMARY",
	"TOOL_REQUEST",
	"IMAGE_REF",
	"RESOURCE_REF",
	"STRUCTURED_OUTPUT",
}

var bundledFactoryResponseEventContractSchemaNames = []string{
	"FactoryResponseEvent",
	"FactoryResponseEventKind",
	"FactoryResponseEventPhase",
	"FactoryResponseEventProvenance",
	"FactoryResponseEventProvenanceDelivery",
	"FactoryResponseEventProvenanceRepresentation",
	"FactoryResponseEventProvenanceFidelity",
	"FactoryResponseEventCapabilities",
	"FactoryResponseEventPayload",
	"FactoryResponseEventContentBlock",
	"FactoryResponseEventContentBlockKind",
	"FactoryResponseEventTextContentBlock",
	"FactoryResponseEventReasoningSummaryContentBlock",
	"FactoryResponseEventToolRequestContentBlock",
	"FactoryResponseEventImageRefContentBlock",
	"FactoryResponseEventResourceRefContentBlock",
	"FactoryResponseEventStructuredOutputContentBlock",
	"FactoryResponseEventSessionPayload",
	"FactoryResponseEventRunPayload",
	"FactoryResponseEventTurnPayload",
	"FactoryResponseEventMessagePayload",
	"FactoryResponseEventMessageDeltaPayload",
	"FactoryResponseEventReasoningPayload",
	"FactoryResponseEventToolPayload",
	"FactoryResponseEventToolDeltaPayload",
	"FactoryResponseEventFileChangePayload",
	"FactoryResponseEventPlanPayload",
	"FactoryResponseEventPlanStep",
	"FactoryResponseEventProgressPayload",
	"FactoryResponseEventUsagePayload",
	"FactoryResponseEventErrorPayload",
	"FactoryResponseEventStreamGapPayload",
}

var bundledFactoryResponseEventPayloadRefs = []string{
	"#/components/schemas/FactoryResponseEventSessionPayload",
	"#/components/schemas/FactoryResponseEventRunPayload",
	"#/components/schemas/FactoryResponseEventTurnPayload",
	"#/components/schemas/FactoryResponseEventMessagePayload",
	"#/components/schemas/FactoryResponseEventMessageDeltaPayload",
	"#/components/schemas/FactoryResponseEventReasoningPayload",
	"#/components/schemas/FactoryResponseEventToolPayload",
	"#/components/schemas/FactoryResponseEventToolDeltaPayload",
	"#/components/schemas/FactoryResponseEventFileChangePayload",
	"#/components/schemas/FactoryResponseEventPlanPayload",
	"#/components/schemas/FactoryResponseEventProgressPayload",
	"#/components/schemas/FactoryResponseEventUsagePayload",
	"#/components/schemas/FactoryResponseEventErrorPayload",
	"#/components/schemas/FactoryResponseEventStreamGapPayload",
}

var representativeResponseEventFixtureNames = []string{
	"text_delta",
	"message_snapshot",
	"tool_lifecycle",
	"retry",
	"final_only_message",
	"usage",
	"stream_gap",
	"item_stream_gap",
}

var canonicalFactoryResponseEventPayloadSchemaNames = []string{
	"FactoryResponseEventSessionPayload",
	"FactoryResponseEventRunPayload",
	"FactoryResponseEventTurnPayload",
	"FactoryResponseEventMessagePayload",
	"FactoryResponseEventMessageDeltaPayload",
	"FactoryResponseEventReasoningPayload",
	"FactoryResponseEventToolPayload",
	"FactoryResponseEventToolDeltaPayload",
	"FactoryResponseEventFileChangePayload",
	"FactoryResponseEventPlanPayload",
	"FactoryResponseEventProgressPayload",
	"FactoryResponseEventUsagePayload",
	"FactoryResponseEventErrorPayload",
	"FactoryResponseEventStreamGapPayload",
}

var factoryResponseEventKindPayloadSchemaNames = map[string]string{
	"SESSION":     "FactoryResponseEventSessionPayload",
	"RUN":         "FactoryResponseEventRunPayload",
	"TURN":        "FactoryResponseEventTurnPayload",
	"REASONING":   "FactoryResponseEventReasoningPayload",
	"FILE_CHANGE": "FactoryResponseEventFileChangePayload",
	"PLAN":        "FactoryResponseEventPlanPayload",
	"PROGRESS":    "FactoryResponseEventProgressPayload",
	"USAGE":       "FactoryResponseEventUsagePayload",
	"ERROR":       "FactoryResponseEventErrorPayload",
	"STREAM_GAP":  "FactoryResponseEventStreamGapPayload",
}

func TestOpenAPIContract_DefinesFactoryResponseEventSchemas(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)

	assertSchemaNamesPresent(t, schemas, bundledFactoryResponseEventContractSchemaNames)
	assertFactoryResponseEventEnvelope(t, schemas)
	assertFactoryResponseEventPayloadUnion(t, schemas)
	assertFactoryResponseEventContentBlockDiscriminator(t, schemas)
}

func TestOpenAPIContract_FactoryResponseEventPayloadUnionCoversAllVariants(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	payloadSchema := schemaObject(t, componentSchemas(t, doc), "FactoryResponseEventPayload")
	assertSchemaOneOfRefs(t, payloadSchema, "FactoryResponseEventPayload", bundledFactoryResponseEventPayloadRefs)
}

func TestOpenAPIContract_FactoryResponseEventContentBlockDiscriminatorCoversAllKinds(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	contentBlockSchema := schemaObject(t, componentSchemas(t, doc), "FactoryResponseEventContentBlock")

	discriminator, ok := contentBlockSchema["discriminator"].(map[string]any)
	if !ok {
		t.Fatal("FactoryResponseEventContentBlock.discriminator is missing")
	}
	if got, _ := discriminator["propertyName"].(string); got != "kind" {
		t.Fatalf("FactoryResponseEventContentBlock.discriminator.propertyName = %q, want kind", got)
	}
	mapping, ok := discriminator["mapping"].(map[string]any)
	if !ok {
		t.Fatal("FactoryResponseEventContentBlock.discriminator.mapping is missing")
	}
	for _, kind := range canonicalFactoryResponseEventContentBlockKindValues {
		if _, ok := mapping[kind]; !ok {
			t.Fatalf("FactoryResponseEventContentBlock.discriminator.mapping is missing %q", kind)
		}
	}
	if len(mapping) != len(canonicalFactoryResponseEventContentBlockKindValues) {
		t.Fatalf(
			"FactoryResponseEventContentBlock.discriminator.mapping has %d entries, want %d",
			len(mapping),
			len(canonicalFactoryResponseEventContentBlockKindValues),
		)
	}

	kindSchema := schemaObject(t, componentSchemas(t, doc), "FactoryResponseEventContentBlockKind")
	assertEnumValues(t, kindSchema, "FactoryResponseEventContentBlockKind", canonicalFactoryResponseEventContentBlockKindValues)
}

func TestOpenAPIContract_RepresentativeResponseEventFixturesValidate(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	for _, fixtureName := range representativeResponseEventFixtureNames {
		fixtureName := fixtureName
		t.Run(fixtureName, func(t *testing.T) {
			event := loadRepresentativeResponseEventFixture(t, fixtureName)
			assertOpenAPIFixtureValidates(t, doc, "FactoryResponseEvent", event)
			assertTextOmitsInternalResponseStreamTerms(t, mustMarshalJSON(t, event))
		})
	}
}

func TestOpenAPIContract_StreamGapPayloadAcceptsOnlyCompleteExclusiveShapes(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	schema := doc.Components.Schemas["FactoryResponseEventStreamGapPayload"].Value
	for name, payload := range map[string]map[string]any{
		"retention": {"fromSequence": float64(1), "toSequence": float64(4), "firstAvailableSequence": float64(5), "reason": "retention_window"},
		"item":      {"affectedItemId": "cursor-tool/call-1", "toolCallId": "call-1", "reason": "provider_reconnect"},
	} {
		if err := schema.VisitJSON(payload); err != nil {
			t.Fatalf("%s stream gap should validate: %v", name, err)
		}
	}
	for name, payload := range map[string]map[string]any{
		"empty":                      {},
		"partial retention":          {"firstAvailableSequence": float64(5)},
		"item without reason":        {"affectedItemId": "cursor-tool/call-1"},
		"tool without affected item": {"toolCallId": "call-1", "reason": "provider_reconnect"},
		"mixed":                      {"fromSequence": float64(1), "toSequence": float64(4), "firstAvailableSequence": float64(5), "affectedItemId": "cursor-tool/call-1", "reason": "provider_reconnect"},
	} {
		if err := schema.VisitJSON(payload); err == nil {
			t.Fatalf("%s stream gap should be rejected: %#v", name, payload)
		}
	}
}

func TestOpenAPIContract_ResponseEventPayloadCoverageFixturesValidate(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	fixtures := loadResponseEventPayloadCoverageFixture(t)
	if len(fixtures) != len(canonicalFactoryResponseEventPayloadSchemaNames) {
		t.Fatalf(
			"payload coverage fixture length = %d, want %d",
			len(fixtures),
			len(canonicalFactoryResponseEventPayloadSchemaNames),
		)
	}

	seenPayloadSchemas := make(map[string]int, len(fixtures))
	for index, event := range fixtures {
		assertFactoryResponseEventFixtureComponentSchemasValidate(t, doc, index, event)
		assertTextOmitsInternalResponseStreamTerms(t, mustMarshalJSON(t, event))

		payloadSchemaName, ok := inferFactoryResponseEventPayloadSchemaName(event)
		if !ok {
			t.Fatalf("payload coverage fixture event %d has no payload schema mapping", index)
		}
		seenPayloadSchemas[payloadSchemaName]++
	}
	for _, payloadSchemaName := range canonicalFactoryResponseEventPayloadSchemaNames {
		if seenPayloadSchemas[payloadSchemaName] != 1 {
			t.Fatalf("payload coverage for %s = %d, want 1", payloadSchemaName, seenPayloadSchemas[payloadSchemaName])
		}
	}
}

func TestOpenAPIContract_ResponseEventContentBlockCoverageFixtureValidates(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	event := loadResponseEventContentBlockCoverageFixture(t)
	assertOpenAPIFixtureValidates(t, doc, "FactoryResponseEvent", event)

	payload, ok := event["payload"].(map[string]any)
	if !ok {
		t.Fatalf("content block coverage payload = %T, want object", event["payload"])
	}
	contentBlocks, ok := payload["contentBlocks"].([]any)
	if !ok {
		t.Fatalf("content block coverage payload.contentBlocks = %T, want array", payload["contentBlocks"])
	}
	seenKinds := make(map[string]int, len(contentBlocks))
	for index, blockValue := range contentBlocks {
		block, ok := blockValue.(map[string]any)
		if !ok {
			t.Fatalf("content block coverage block %d = %T, want object", index, blockValue)
		}
		kind, ok := block["kind"].(string)
		if !ok {
			t.Fatalf("content block coverage block %d kind = %T, want string", index, block["kind"])
		}
		seenKinds[kind]++
		if err := doc.Components.Schemas["FactoryResponseEventContentBlock"].Value.VisitJSON(block); err != nil {
			t.Fatalf("content block coverage block %d should validate: %v", index, err)
		}
	}
	for _, kind := range canonicalFactoryResponseEventContentBlockKindValues {
		if seenKinds[kind] != 1 {
			t.Fatalf("content block coverage for %s = %d, want 1", kind, seenKinds[kind])
		}
	}
}

func TestOpenAPIContract_PublicArtifactsExposeFactoryResponseEventContract(t *testing.T) {
	requiredMarkers := []string{
		"FactoryResponseEvent",
		"FactoryResponseEventPayload",
		"agent-factory.response-event.v1",
	}
	paths := []string{
		"../../../../api/openapi.yaml",
		"../generated/server.gen.go",
		"../../../../ui/src/api/generated/openapi.ts",
	}

	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read public artifact %s: %v", path, err)
			}
			text := string(data)
			assertTextOmitsInternalResponseStreamTerms(t, text)
			for _, marker := range requiredMarkers {
				if !strings.Contains(text, marker) {
					t.Fatalf("public artifact %s is missing required response-event marker %q", path, marker)
				}
			}
		})
	}
}

func TestOpenAPIAuthoring_ResponseEventSchemasUseDedicatedFragments(t *testing.T) {
	doc := loadAuthoredOpenAPIDoc(t)
	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"FactoryResponseEvent":                             "./components/schemas/response-events/FactoryResponseEvent.yaml",
		"FactoryResponseEventKind":                         "./components/schemas/response-events/FactoryResponseEventKind.yaml",
		"FactoryResponseEventPhase":                        "./components/schemas/response-events/FactoryResponseEventPhase.yaml",
		"FactoryResponseEventProvenance":                   "./components/schemas/response-events/FactoryResponseEventProvenance.yaml",
		"FactoryResponseEventProvenanceDelivery":           "./components/schemas/response-events/FactoryResponseEventProvenanceDelivery.yaml",
		"FactoryResponseEventProvenanceRepresentation":     "./components/schemas/response-events/FactoryResponseEventProvenanceRepresentation.yaml",
		"FactoryResponseEventProvenanceFidelity":           "./components/schemas/response-events/FactoryResponseEventProvenanceFidelity.yaml",
		"FactoryResponseEventCapabilities":                 "./components/schemas/response-events/FactoryResponseEventCapabilities.yaml",
		"FactoryResponseEventPayload":                      "./components/schemas/response-events/FactoryResponseEventPayload.yaml",
		"FactoryResponseEventContentBlock":                 "./components/schemas/response-events/FactoryResponseEventContentBlock.yaml",
		"FactoryResponseEventContentBlockKind":             "./components/schemas/response-events/FactoryResponseEventContentBlockKind.yaml",
		"FactoryResponseEventTextContentBlock":             "./components/schemas/response-events/content-blocks/FactoryResponseEventTextContentBlock.yaml",
		"FactoryResponseEventReasoningSummaryContentBlock": "./components/schemas/response-events/content-blocks/FactoryResponseEventReasoningSummaryContentBlock.yaml",
		"FactoryResponseEventToolRequestContentBlock":      "./components/schemas/response-events/content-blocks/FactoryResponseEventToolRequestContentBlock.yaml",
		"FactoryResponseEventImageRefContentBlock":         "./components/schemas/response-events/content-blocks/FactoryResponseEventImageRefContentBlock.yaml",
		"FactoryResponseEventResourceRefContentBlock":      "./components/schemas/response-events/content-blocks/FactoryResponseEventResourceRefContentBlock.yaml",
		"FactoryResponseEventStructuredOutputContentBlock": "./components/schemas/response-events/content-blocks/FactoryResponseEventStructuredOutputContentBlock.yaml",
		"FactoryResponseEventSessionPayload":               "./components/schemas/response-events/payloads/FactoryResponseEventSessionPayload.yaml",
		"FactoryResponseEventRunPayload":                   "./components/schemas/response-events/payloads/FactoryResponseEventRunPayload.yaml",
		"FactoryResponseEventTurnPayload":                  "./components/schemas/response-events/payloads/FactoryResponseEventTurnPayload.yaml",
		"FactoryResponseEventMessagePayload":               "./components/schemas/response-events/payloads/FactoryResponseEventMessagePayload.yaml",
		"FactoryResponseEventMessageDeltaPayload":          "./components/schemas/response-events/payloads/FactoryResponseEventMessageDeltaPayload.yaml",
		"FactoryResponseEventReasoningPayload":             "./components/schemas/response-events/payloads/FactoryResponseEventReasoningPayload.yaml",
		"FactoryResponseEventToolPayload":                  "./components/schemas/response-events/payloads/FactoryResponseEventToolPayload.yaml",
		"FactoryResponseEventToolDeltaPayload":             "./components/schemas/response-events/payloads/FactoryResponseEventToolDeltaPayload.yaml",
		"FactoryResponseEventFileChangePayload":            "./components/schemas/response-events/payloads/FactoryResponseEventFileChangePayload.yaml",
		"FactoryResponseEventPlanPayload":                  "./components/schemas/response-events/payloads/FactoryResponseEventPlanPayload.yaml",
		"FactoryResponseEventPlanStep":                     "./components/schemas/response-events/payloads/FactoryResponseEventPlanStep.yaml",
		"FactoryResponseEventProgressPayload":              "./components/schemas/response-events/payloads/FactoryResponseEventProgressPayload.yaml",
		"FactoryResponseEventUsagePayload":                 "./components/schemas/response-events/payloads/FactoryResponseEventUsagePayload.yaml",
		"FactoryResponseEventErrorPayload":                 "./components/schemas/response-events/payloads/FactoryResponseEventErrorPayload.yaml",
		"FactoryResponseEventStreamGapPayload":             "./components/schemas/response-events/payloads/FactoryResponseEventStreamGapPayload.yaml",
	}
	for schemaName, wantRef := range expectedRefs {
		assertSchemaRef(t, schemas, schemaName, wantRef)
	}
}

func TestOpenAPIContract_FactoryEventLaneRemainsIsolatedFromResponseEvents(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)

	assertSchemaNamesPresent(t, schemas, bundledFactoryEventContractSchemaNames)
	factoryEventProperties := schemaProperties(t, schemaObject(t, schemas, "FactoryEvent"), "FactoryEvent")
	assertPayloadUnionRefs(t, factoryEventProperties, bundledFactoryEventPayloadRefs)

	for _, ref := range bundledFactoryEventPayloadRefs {
		if strings.Contains(ref, "FactoryResponseEvent") {
			t.Fatalf("FactoryEvent payload union must not reference response-event schema %s", ref)
		}
	}
	for _, schemaName := range bundledFactoryEventContractSchemaNames {
		if strings.HasPrefix(schemaName, "FactoryResponseEvent") {
			t.Fatalf("bundledFactoryEventContractSchemaNames must not include response-event schema %s", schemaName)
		}
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("paths object is missing")
	}
	assertEventStreamSchemaRef(t, pathOperation(t, paths, "/factory-sessions/{session_id}/events", "get"), "#/components/schemas/FactoryEvent")
	assertEventStreamSchemaRef(t, pathOperation(t, paths, responseEventStreamPath, "get"), "#/components/schemas/FactoryResponseEvent")
}

func assertFactoryResponseEventEnvelope(t *testing.T, schemas map[string]any) {
	t.Helper()

	envelope := schemaObject(t, schemas, "FactoryResponseEvent")
	assertRequiredFields(
		t,
		envelope,
		"schemaVersion",
		"eventId",
		"sequence",
		"recordedAt",
		"factorySessionId",
		"runId",
		"kind",
		"phase",
		"provenance",
		"payload",
	)
	properties := schemaProperties(t, envelope, "FactoryResponseEvent")
	assertPropertyRef(t, properties, "kind", "#/components/schemas/FactoryResponseEventKind")
	assertPropertyRef(t, properties, "phase", "#/components/schemas/FactoryResponseEventPhase")
	assertPropertyRef(t, properties, "provenance", "#/components/schemas/FactoryResponseEventProvenance")
	assertPropertyRef(t, properties, "payload", "#/components/schemas/FactoryResponseEventPayload")

	kindSchema := schemaObject(t, schemas, "FactoryResponseEventKind")
	assertEnumValues(t, kindSchema, "FactoryResponseEventKind", canonicalFactoryResponseEventKindValues)
}

func assertFactoryResponseEventPayloadUnion(t *testing.T, schemas map[string]any) {
	t.Helper()
	payloadSchema := schemaObject(t, schemas, "FactoryResponseEventPayload")
	assertSchemaOneOfRefs(t, payloadSchema, "FactoryResponseEventPayload", bundledFactoryResponseEventPayloadRefs)
}

func assertFactoryResponseEventContentBlockDiscriminator(t *testing.T, schemas map[string]any) {
	t.Helper()
	contentBlockSchema := schemaObject(t, schemas, "FactoryResponseEventContentBlock")
	discriminator, ok := contentBlockSchema["discriminator"].(map[string]any)
	if !ok {
		t.Fatal("FactoryResponseEventContentBlock.discriminator is missing")
	}
	mapping, ok := discriminator["mapping"].(map[string]any)
	if !ok {
		t.Fatal("FactoryResponseEventContentBlock.discriminator.mapping is missing")
	}
	if len(mapping) != len(canonicalFactoryResponseEventContentBlockKindValues) {
		t.Fatalf(
			"FactoryResponseEventContentBlock discriminator mapping has %d entries, want %d",
			len(mapping),
			len(canonicalFactoryResponseEventContentBlockKindValues),
		)
	}
}

func loadRepresentativeResponseEventFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	path := filepath.FromSlash("../../../services/factory_sessions/responseevents/testdata/fixtures/" + name + ".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read representative response-event fixture %s: %v", name, err)
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("parse representative response-event fixture %s: %v", name, err)
	}
	return event
}

func loadResponseEventPayloadCoverageFixture(t *testing.T) []map[string]any {
	t.Helper()
	data, err := os.ReadFile("../testdata/canonical-response-event-payload-coverage.json")
	if err != nil {
		t.Fatalf("read response-event payload coverage fixture: %v", err)
	}
	assertTextOmitsInternalResponseStreamTerms(t, string(data))

	var fixtures []map[string]any
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("parse response-event payload coverage fixture: %v", err)
	}
	return fixtures
}

func loadResponseEventContentBlockCoverageFixture(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("../testdata/canonical-response-event-content-block-coverage.json")
	if err != nil {
		t.Fatalf("read response-event content block coverage fixture: %v", err)
	}
	assertTextOmitsInternalResponseStreamTerms(t, string(data))

	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("parse response-event content block coverage fixture: %v", err)
	}
	return event
}

func inferFactoryResponseEventPayloadSchemaName(event map[string]any) (string, bool) {
	kind, _ := event["kind"].(string)
	phase, _ := event["phase"].(string)
	if _, ok := event["payload"].(map[string]any); !ok {
		return "", false
	}

	switch kind {
	case "MESSAGE":
		if phase == "DELTA" {
			return "FactoryResponseEventMessageDeltaPayload", true
		}
		return "FactoryResponseEventMessagePayload", true
	case "TOOL":
		if phase == "DELTA" {
			return "FactoryResponseEventToolDeltaPayload", true
		}
		return "FactoryResponseEventToolPayload", true
	default:
		schemaName, ok := factoryResponseEventKindPayloadSchemaNames[kind]
		return schemaName, ok
	}
}

func assertFactoryResponseEventFixtureComponentSchemasValidate(
	t *testing.T,
	doc *openapi3.T,
	index int,
	event map[string]any,
) {
	t.Helper()

	if got, ok := event["schemaVersion"].(string); !ok || got != "agent-factory.response-event.v1" {
		t.Fatalf("response-event fixture %d schemaVersion = %#v, want agent-factory.response-event.v1", index, event["schemaVersion"])
	}
	for _, field := range []string{"eventId", "recordedAt", "factorySessionId", "runId"} {
		if _, ok := event[field].(string); !ok {
			t.Fatalf("response-event fixture %d %s = %#v, want string", index, field, event[field])
		}
	}
	if _, ok := event["sequence"].(float64); !ok {
		t.Fatalf("response-event fixture %d sequence = %#v, want number", index, event["sequence"])
	}
	if err := doc.Components.Schemas["FactoryResponseEventKind"].Value.VisitJSON(event["kind"]); err != nil {
		t.Fatalf("response-event fixture %d kind should validate: %v", index, err)
	}
	if err := doc.Components.Schemas["FactoryResponseEventPhase"].Value.VisitJSON(event["phase"]); err != nil {
		t.Fatalf("response-event fixture %d phase should validate: %v", index, err)
	}
	if err := doc.Components.Schemas["FactoryResponseEventProvenance"].Value.VisitJSON(event["provenance"]); err != nil {
		t.Fatalf("response-event fixture %d provenance should validate: %v", index, err)
	}

	payloadSchemaName, ok := inferFactoryResponseEventPayloadSchemaName(event)
	if !ok {
		t.Fatalf("response-event fixture %d has no payload schema mapping", index)
	}
	if err := doc.Components.Schemas[payloadSchemaName].Value.VisitJSON(event["payload"]); err != nil {
		t.Fatalf("response-event fixture %d payload should validate against %s: %v", index, payloadSchemaName, err)
	}
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON fixture: %v", err)
	}
	return string(encoded)
}

func TestGeneratedFactoryResponseEventRepresentativeFixturesRoundTrip(t *testing.T) {
	for _, fixtureName := range representativeResponseEventFixtureNames {
		fixtureName := fixtureName
		t.Run(fixtureName, func(t *testing.T) {
			raw := readRepresentativeResponseEventFixtureBytes(t, fixtureName)
			assertTextOmitsInternalResponseStreamTerms(t, string(raw))

			var event factoryapi.FactoryResponseEvent
			decodeRoundTripJSON(t, raw, &event, "representative response-event fixture "+fixtureName)
			assertGeneratedFactoryResponseEventRoundTrip(t, event)
		})
	}
}

func TestFactoryResponseEventRepresentativeFixturesHaveCanonicalWireParity(t *testing.T) {
	for _, fixtureName := range representativeResponseEventFixtureNames {
		fixtureName := fixtureName
		t.Run(fixtureName, func(t *testing.T) {
			raw := readRepresentativeResponseEventFixtureBytes(t, fixtureName)

			var domainEvent factorysessions.FactoryResponseEvent
			if err := json.Unmarshal(raw, &domainEvent); err != nil {
				t.Fatalf("unmarshal domain FactoryResponseEvent: %v", err)
			}
			var generatedEvent factoryapi.FactoryResponseEvent
			if err := json.Unmarshal(raw, &generatedEvent); err != nil {
				t.Fatalf("unmarshal generated FactoryResponseEvent: %v", err)
			}

			domainJSON, err := json.Marshal(domainEvent)
			if err != nil {
				t.Fatalf("marshal domain FactoryResponseEvent: %v", err)
			}
			generatedJSON, err := json.Marshal(generatedEvent)
			if err != nil {
				t.Fatalf("marshal generated FactoryResponseEvent: %v", err)
			}
			if !bytes.Equal(domainJSON, generatedJSON) {
				t.Fatalf(
					"canonical FactoryResponseEvent bytes differ:\ndomain=%s\ngenerated=%s",
					domainJSON,
					generatedJSON,
				)
			}

			if fixtureName == "stream_gap" {
				assertStreamGapBounds(t, domainEvent.Payload, 100, 150, 151)
				assertStreamGapBounds(t, generatedJSONPayload(t, generatedEvent), 100, 150, 151)
			}
		})
	}
}

func generatedJSONPayload(t *testing.T, event factoryapi.FactoryResponseEvent) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(event.Payload)
	if err != nil {
		t.Fatalf("marshal generated FactoryResponseEvent payload: %v", err)
	}
	return encoded
}

func assertStreamGapBounds(t *testing.T, payload json.RawMessage, wantFrom, wantTo, wantFirstAvailable int64) {
	t.Helper()
	var gap struct {
		FromSequence           int64 `json:"fromSequence"`
		ToSequence             int64 `json:"toSequence"`
		FirstAvailableSequence int64 `json:"firstAvailableSequence"`
	}
	if err := json.Unmarshal(payload, &gap); err != nil {
		t.Fatalf("unmarshal STREAM_GAP payload: %v", err)
	}
	if gap.FromSequence != wantFrom || gap.ToSequence != wantTo {
		t.Fatalf(
			"STREAM_GAP bounds = %d..%d, want %d..%d",
			gap.FromSequence,
			gap.ToSequence,
			wantFrom,
			wantTo,
		)
	}
	if gap.FirstAvailableSequence != wantFirstAvailable {
		t.Fatalf("STREAM_GAP first available sequence = %d, want %d", gap.FirstAvailableSequence, wantFirstAvailable)
	}
}

func TestGeneratedFactoryResponseEventPayloadCoverageFixturesRoundTrip(t *testing.T) {
	raw, err := os.ReadFile(filepath.FromSlash("../testdata/canonical-response-event-payload-coverage.json"))
	if err != nil {
		t.Fatalf("read payload coverage fixture: %v", err)
	}
	assertTextOmitsInternalResponseStreamTerms(t, string(raw))

	var events []factoryapi.FactoryResponseEvent
	decodeRoundTripJSON(t, raw, &events, "response-event payload coverage fixture")
	if len(events) != len(canonicalFactoryResponseEventPayloadSchemaNames) {
		t.Fatalf(
			"payload coverage fixture count = %d, want %d",
			len(events),
			len(canonicalFactoryResponseEventPayloadSchemaNames),
		)
	}

	seen := make(map[string]int, len(events))
	for index, event := range events {
		payloadSchemaName := assertGeneratedFactoryResponseEventPayloadDecodes(t, event)
		seen[payloadSchemaName]++
		assertGeneratedFactoryResponseEventRoundTrip(t, event)
		if event.SchemaVersion != factoryapi.AgentFactoryResponseEventV1 {
			t.Fatalf("payload coverage fixture event %d schemaVersion = %q, want %q", index, event.SchemaVersion, factoryapi.AgentFactoryResponseEventV1)
		}
	}
	for _, payloadSchemaName := range canonicalFactoryResponseEventPayloadSchemaNames {
		if seen[payloadSchemaName] != 1 {
			t.Fatalf("generated payload coverage for %s = %d, want 1", payloadSchemaName, seen[payloadSchemaName])
		}
	}
}

func TestGeneratedFactoryResponseEventContentBlockCoverageFixtureRoundTrip(t *testing.T) {
	raw, err := os.ReadFile(filepath.FromSlash("../testdata/canonical-response-event-content-block-coverage.json"))
	if err != nil {
		t.Fatalf("read content block coverage fixture: %v", err)
	}

	var event factoryapi.FactoryResponseEvent
	decodeRoundTripJSON(t, raw, &event, "response-event content block coverage fixture")
	payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
	if err != nil {
		t.Fatalf("decode content block coverage message payload: %v", err)
	}
	if len(payload.ContentBlocks) != len(canonicalFactoryResponseEventContentBlockKindValues) {
		t.Fatalf(
			"content block coverage count = %d, want %d",
			len(payload.ContentBlocks),
			len(canonicalFactoryResponseEventContentBlockKindValues),
		)
	}

	seen := make(map[string]int, len(payload.ContentBlocks))
	for index, block := range payload.ContentBlocks {
		kind, err := block.Discriminator()
		if err != nil {
			t.Fatalf("content block %d discriminator: %v", index, err)
		}
		seen[kind]++
		if _, err := block.ValueByDiscriminator(); err != nil {
			t.Fatalf("content block %d decode: %v", index, err)
		}
	}
	for _, kindValue := range canonicalFactoryResponseEventContentBlockKindValues {
		if seen[kindValue] != 1 {
			t.Fatalf("generated content block coverage for %s = %d, want 1", kindValue, seen[kindValue])
		}
	}

	assertGeneratedFactoryResponseEventRoundTrip(t, event)
}

func TestGeneratedPublicArtifactsExposeFactoryResponseEventUnion(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash("../generated/server.gen.go"))
	if err != nil {
		t.Fatalf("read generated server models: %v", err)
	}
	text := string(data)
	assertTextOmitsInternalResponseStreamTerms(t, text)

	for _, marker := range []string{
		"type FactoryResponseEvent struct",
		"type FactoryResponseEventPayload struct",
		"AgentFactoryResponseEventV1",
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("generated server models missing %q", marker)
		}
	}
	for _, decoder := range generatedFactoryResponseEventPayloadDecoders {
		if !strings.Contains(text, decoder.methodName) {
			t.Fatalf("generated server models missing payload decoder %q", decoder.methodName)
		}
	}
}

type generatedFactoryResponseEventPayloadDecoder struct {
	schemaName string
	methodName string
	decode     func(factoryapi.FactoryResponseEventPayload) error
}

var generatedFactoryResponseEventPayloadDecoders = []generatedFactoryResponseEventPayloadDecoder{
	{
		schemaName: "FactoryResponseEventSessionPayload",
		methodName: "AsFactoryResponseEventSessionPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventSessionPayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventRunPayload",
		methodName: "AsFactoryResponseEventRunPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventRunPayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventTurnPayload",
		methodName: "AsFactoryResponseEventTurnPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventTurnPayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventMessagePayload",
		methodName: "AsFactoryResponseEventMessagePayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventMessagePayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventMessageDeltaPayload",
		methodName: "AsFactoryResponseEventMessageDeltaPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventMessageDeltaPayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventReasoningPayload",
		methodName: "AsFactoryResponseEventReasoningPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventReasoningPayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventToolPayload",
		methodName: "AsFactoryResponseEventToolPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventToolPayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventToolDeltaPayload",
		methodName: "AsFactoryResponseEventToolDeltaPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventToolDeltaPayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventFileChangePayload",
		methodName: "AsFactoryResponseEventFileChangePayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventFileChangePayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventPlanPayload",
		methodName: "AsFactoryResponseEventPlanPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventPlanPayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventProgressPayload",
		methodName: "AsFactoryResponseEventProgressPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventProgressPayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventUsagePayload",
		methodName: "AsFactoryResponseEventUsagePayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventUsagePayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventErrorPayload",
		methodName: "AsFactoryResponseEventErrorPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventErrorPayload()
			return err
		},
	},
	{
		schemaName: "FactoryResponseEventStreamGapPayload",
		methodName: "AsFactoryResponseEventStreamGapPayload",
		decode: func(payload factoryapi.FactoryResponseEventPayload) error {
			_, err := payload.AsFactoryResponseEventStreamGapPayload()
			return err
		},
	},
}

func assertGeneratedFactoryResponseEventRoundTrip(t *testing.T, event factoryapi.FactoryResponseEvent) {
	t.Helper()

	assertGeneratedFactoryResponseEventPayloadDecodes(t, event)

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal generated FactoryResponseEvent %s: %v", event.EventId, err)
	}
	assertTextOmitsInternalResponseStreamTerms(t, string(encoded))

	var roundTripped factoryapi.FactoryResponseEvent
	decodeRoundTripJSON(t, encoded, &roundTripped, "round-tripped response-event "+event.EventId)
	roundTrippedJSON, err := json.Marshal(roundTripped)
	if err != nil {
		t.Fatalf("marshal round-tripped FactoryResponseEvent %s: %v", event.EventId, err)
	}
	if !bytes.Equal(encoded, roundTrippedJSON) {
		t.Fatalf("generated FactoryResponseEvent round trip changed wire bytes:\nbefore=%s\nafter=%s", encoded, roundTrippedJSON)
	}
	assertGeneratedFactoryResponseEventPayloadDecodes(t, roundTripped)
}

func assertGeneratedFactoryResponseEventPayloadDecodes(t *testing.T, event factoryapi.FactoryResponseEvent) string {
	t.Helper()

	wantSchema, ok := inferFactoryResponseEventPayloadSchemaName(map[string]any{
		"kind":    string(event.Kind),
		"phase":   string(event.Phase),
		"payload": map[string]any{},
	})
	if !ok {
		t.Fatalf("response-event %s has no payload schema mapping for kind=%s phase=%s", event.EventId, event.Kind, event.Phase)
	}

	for _, decoder := range generatedFactoryResponseEventPayloadDecoders {
		if decoder.schemaName != wantSchema {
			continue
		}
		if err := decoder.decode(event.Payload); err != nil {
			t.Fatalf("decode %s payload for %s: %v", wantSchema, event.EventId, err)
		}
		return wantSchema
	}
	t.Fatalf("missing generated payload decoder for %s", wantSchema)
	return ""
}

func readRepresentativeResponseEventFixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.FromSlash("../../../services/factory_sessions/responseevents/testdata/fixtures/" + name + ".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read representative response-event fixture %s: %v", name, err)
	}
	return data
}
