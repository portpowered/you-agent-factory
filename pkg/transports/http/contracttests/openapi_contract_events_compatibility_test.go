package apicontract_test

import "testing"

func TestOpenAPIContract_AcceptsAdditiveCanonicalEventFields(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	payload := validRunRequestPayloadFixture()
	payload["futurePayload"] = map[string]any{"introducedBy": "newer-you"}
	event := map[string]any{
		"schemaVersion": "agent-factory.event.v1",
		"id":            "event-future-fields",
		"type":          "RUN_REQUEST",
		"context": map[string]any{
			"sequence":  1,
			"tick":      1,
			"eventTime": "2026-04-10T12:00:00Z",
			"futureContext": map[string]any{
				"introducedBy": "newer-you",
			},
		},
		"payload":        payload,
		"futureEnvelope": true,
	}
	if err := doc.Components.Schemas["FactoryEvent"].Value.VisitJSON(event); err != nil {
		t.Fatalf("FactoryEvent should accept additive envelope, context, and payload fields: %v", err)
	}

	recording := map[string]any{
		"schemaVersion":   "agent-factory.recording.v1",
		"sessionId":       "session-future-fields",
		"events":          []any{event},
		"futureRecording": map[string]any{"revision": 2},
	}
	if err := doc.Components.Schemas["FactoryRecording"].Value.VisitJSON(recording); err != nil {
		t.Fatalf("FactoryRecording should accept additive envelope fields: %v", err)
	}

	sessionEvent := map[string]any{
		"schemaVersion": "agent-factory.event.v1",
		"id":            "event-session-started-future-fields",
		"type":          "SESSION_STARTED",
		"context": map[string]any{
			"sequence":  2,
			"tick":      2,
			"eventTime": "2026-04-10T12:00:01Z",
		},
		"payload": map[string]any{
			"startedAt":     "2026-04-10T12:00:01Z",
			"futurePayload": map[string]any{"revision": 2},
		},
	}
	if err := doc.Components.Schemas["FactoryEvent"].Value.VisitJSON(sessionEvent); err != nil {
		t.Fatalf("SessionStartedEventPayload should accept additive fields: %v", err)
	}

	event["schemaVersion"] = "agent-factory.event.v2"
	if err := doc.Components.Schemas["FactoryEvent"].Value.VisitJSON(event); err == nil {
		t.Fatal("FactoryEvent accepted an invalid known schemaVersion")
	}
}
