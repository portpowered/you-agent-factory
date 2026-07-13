package apicontract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

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
	if roundTripped.Kind != event.Kind || roundTripped.Phase != event.Phase {
		t.Fatalf(
			"round-tripped response-event %s kind/phase = %s/%s, want %s/%s",
			event.EventId,
			roundTripped.Kind,
			roundTripped.Phase,
			event.Kind,
			event.Phase,
		)
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
	path := filepath.FromSlash("../../factorysessions/responseevents/testdata/fixtures/" + name + ".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read representative response-event fixture %s: %v", name, err)
	}
	return data
}
