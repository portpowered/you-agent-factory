package apicontract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

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
		"../../../api/openapi.yaml",
		"../generated/server.gen.go",
		"../../../ui/src/api/generated/openapi.ts",
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

func TestOpenAPIContract_NoResponseEventSSERoute(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("paths object is missing")
	}
	for path := range paths {
		if strings.Contains(path, "response-events") {
			t.Fatalf("paths must not expose response-event SSE route %q in this lane", path)
		}
	}
}

func TestOpenAPIAuthoring_ResponseEventSchemasUseDedicatedFragments(t *testing.T) {
	doc := loadAuthoredOpenAPIDoc(t)
	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"FactoryResponseEvent":                              "./components/schemas/response-events/FactoryResponseEvent.yaml",
		"FactoryResponseEventKind":                        "./components/schemas/response-events/FactoryResponseEventKind.yaml",
		"FactoryResponseEventPhase":                       "./components/schemas/response-events/FactoryResponseEventPhase.yaml",
		"FactoryResponseEventProvenance":                  "./components/schemas/response-events/FactoryResponseEventProvenance.yaml",
		"FactoryResponseEventProvenanceDelivery":          "./components/schemas/response-events/FactoryResponseEventProvenanceDelivery.yaml",
		"FactoryResponseEventProvenanceRepresentation":    "./components/schemas/response-events/FactoryResponseEventProvenanceRepresentation.yaml",
		"FactoryResponseEventProvenanceFidelity":          "./components/schemas/response-events/FactoryResponseEventProvenanceFidelity.yaml",
		"FactoryResponseEventCapabilities":                "./components/schemas/response-events/FactoryResponseEventCapabilities.yaml",
		"FactoryResponseEventPayload":                     "./components/schemas/response-events/FactoryResponseEventPayload.yaml",
		"FactoryResponseEventContentBlock":                "./components/schemas/response-events/FactoryResponseEventContentBlock.yaml",
		"FactoryResponseEventContentBlockKind":            "./components/schemas/response-events/FactoryResponseEventContentBlockKind.yaml",
		"FactoryResponseEventTextContentBlock":            "./components/schemas/response-events/content-blocks/FactoryResponseEventTextContentBlock.yaml",
		"FactoryResponseEventReasoningSummaryContentBlock": "./components/schemas/response-events/content-blocks/FactoryResponseEventReasoningSummaryContentBlock.yaml",
		"FactoryResponseEventToolRequestContentBlock":     "./components/schemas/response-events/content-blocks/FactoryResponseEventToolRequestContentBlock.yaml",
		"FactoryResponseEventImageRefContentBlock":        "./components/schemas/response-events/content-blocks/FactoryResponseEventImageRefContentBlock.yaml",
		"FactoryResponseEventResourceRefContentBlock":     "./components/schemas/response-events/content-blocks/FactoryResponseEventResourceRefContentBlock.yaml",
		"FactoryResponseEventStructuredOutputContentBlock": "./components/schemas/response-events/content-blocks/FactoryResponseEventStructuredOutputContentBlock.yaml",
		"FactoryResponseEventSessionPayload":              "./components/schemas/response-events/payloads/FactoryResponseEventSessionPayload.yaml",
		"FactoryResponseEventRunPayload":                  "./components/schemas/response-events/payloads/FactoryResponseEventRunPayload.yaml",
		"FactoryResponseEventTurnPayload":                 "./components/schemas/response-events/payloads/FactoryResponseEventTurnPayload.yaml",
		"FactoryResponseEventMessagePayload":              "./components/schemas/response-events/payloads/FactoryResponseEventMessagePayload.yaml",
		"FactoryResponseEventMessageDeltaPayload":         "./components/schemas/response-events/payloads/FactoryResponseEventMessageDeltaPayload.yaml",
		"FactoryResponseEventReasoningPayload":            "./components/schemas/response-events/payloads/FactoryResponseEventReasoningPayload.yaml",
		"FactoryResponseEventToolPayload":                 "./components/schemas/response-events/payloads/FactoryResponseEventToolPayload.yaml",
		"FactoryResponseEventToolDeltaPayload":            "./components/schemas/response-events/payloads/FactoryResponseEventToolDeltaPayload.yaml",
		"FactoryResponseEventFileChangePayload":           "./components/schemas/response-events/payloads/FactoryResponseEventFileChangePayload.yaml",
		"FactoryResponseEventPlanPayload":                 "./components/schemas/response-events/payloads/FactoryResponseEventPlanPayload.yaml",
		"FactoryResponseEventPlanStep":                    "./components/schemas/response-events/payloads/FactoryResponseEventPlanStep.yaml",
		"FactoryResponseEventProgressPayload":             "./components/schemas/response-events/payloads/FactoryResponseEventProgressPayload.yaml",
		"FactoryResponseEventUsagePayload":                "./components/schemas/response-events/payloads/FactoryResponseEventUsagePayload.yaml",
		"FactoryResponseEventErrorPayload":                "./components/schemas/response-events/payloads/FactoryResponseEventErrorPayload.yaml",
		"FactoryResponseEventStreamGapPayload":            "./components/schemas/response-events/payloads/FactoryResponseEventStreamGapPayload.yaml",
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
	assertEventStreamSchemaRef(t, pathOperation(t, paths, "/events", "get"), "#/components/schemas/FactoryEvent")
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
	path := filepath.FromSlash("../../factorysessions/responseevents/testdata/fixtures/" + name + ".json")
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
	payload, ok := event["payload"].(map[string]any)
	if !ok {
		return "", false
	}

	switch kind {
	case "SESSION":
		return "FactoryResponseEventSessionPayload", true
	case "RUN":
		return "FactoryResponseEventRunPayload", true
	case "TURN":
		return "FactoryResponseEventTurnPayload", true
	case "MESSAGE":
		if phase == "DELTA" {
			return "FactoryResponseEventMessageDeltaPayload", true
		}
		return "FactoryResponseEventMessagePayload", true
	case "REASONING":
		return "FactoryResponseEventReasoningPayload", true
	case "TOOL":
		if phase == "DELTA" {
			return "FactoryResponseEventToolDeltaPayload", true
		}
		return "FactoryResponseEventToolPayload", true
	case "FILE_CHANGE":
		return "FactoryResponseEventFileChangePayload", true
	case "PLAN":
		return "FactoryResponseEventPlanPayload", true
	case "PROGRESS":
		return "FactoryResponseEventProgressPayload", true
	case "USAGE":
		return "FactoryResponseEventUsagePayload", true
	case "ERROR":
		return "FactoryResponseEventErrorPayload", true
	case "STREAM_GAP":
		return "FactoryResponseEventStreamGapPayload", true
	default:
		_ = payload
		return "", false
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
