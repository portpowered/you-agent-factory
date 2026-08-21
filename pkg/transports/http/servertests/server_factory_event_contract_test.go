package apiserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestGetFactorySessionEvents_AllPublicEventTypesMatchBundledSchema(t *testing.T) {
	canonical := loadRepresentativePublicFactoryEvents(t)
	doc := loadBundledFactoryEventContract(t)
	assertRepresentativePublicFactoryEventCoverage(t, doc, canonical)

	closed := make(chan interfaces.FactoryEvent)
	close(closed)
	srv := newWorkAPITestServer(liveFactoryEventWorkAPI{stream: &interfaces.FactoryEventStream{
		History: canonical,
		Events:  closed,
	}})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/factory-sessions/session-contract/events")
	if err != nil {
		t.Fatalf("GET Factory Events route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET Factory Events route status = %d, want 200: %s", response.StatusCode, readBody(t, response))
	}

	served := readAllSSEFactoryEvents(t, bufio.NewReader(response.Body))
	if len(served) != len(canonical) {
		t.Fatalf("served Factory Event count = %d, want %d", len(served), len(canonical))
	}

	for index, event := range served {
		want := canonical[index]
		t.Run(string(event.Type), func(t *testing.T) {
			if event.Id != want.Id {
				t.Fatalf("served event id = %q, want %q", event.Id, want.Id)
			}
			if event.Type != factoryapi.FactoryEventType(want.Type) {
				t.Fatalf("served event type = %q, want %q", event.Type, want.Type)
			}
			assertServedFactoryEventUsesBundledSchema(t, doc, event)
		})
	}
}

func loadBundledFactoryEventContract(t *testing.T) *openapi3.T {
	t.Helper()

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load bundled OpenAPI contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate bundled OpenAPI contract: %v", err)
	}
	return doc
}

func assertRepresentativePublicFactoryEventCoverage(t *testing.T, doc *openapi3.T, events []interfaces.FactoryEvent) {
	t.Helper()

	factoryEventSchema := doc.Components.Schemas["FactoryEvent"].Value
	supported := factoryEventSchema.Discriminator.Mapping
	if len(supported) == 0 {
		t.Fatal("bundled FactoryEvent schema has no discriminator payload mappings")
	}

	got := make(map[string]bool, len(events))
	for _, event := range events {
		publicType := string(event.Type)
		if got[publicType] {
			t.Fatalf("duplicate representative Factory Event fixture for type %q", publicType)
		}
		got[publicType] = true
	}

	for publicType := range supported {
		if !got[publicType] {
			t.Fatalf("missing representative Factory Event fixture for public type %q", publicType)
		}
	}
	for publicType := range got {
		if _, ok := supported[publicType]; !ok {
			t.Fatalf("representative Factory Event fixture has unsupported public type %q", publicType)
		}
	}
}

func assertServedFactoryEventUsesBundledSchema(t *testing.T, doc *openapi3.T, event factoryapi.FactoryEvent) {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal served Factory Event: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode served Factory Event: %v", err)
	}

	factoryEventSchema := doc.Components.Schemas["FactoryEvent"].Value
	payloadRef, ok := factoryEventSchema.Discriminator.Mapping[string(event.Type)]
	if !ok {
		t.Fatalf("FactoryEvent schema has no payload mapping for type %q", event.Type)
	}
	payloadSchemaName := strings.TrimSuffix(path.Base(payloadRef), ".yaml")
	payloadSchemaRef, ok := doc.Components.Schemas[payloadSchemaName]
	if !ok || payloadSchemaRef == nil || payloadSchemaRef.Value == nil {
		t.Fatalf("FactoryEvent schema payload mapping for type %q resolves to missing component %q", event.Type, payloadSchemaName)
	}

	// The published FactoryEvent union is discriminated by the envelope's type,
	// while kin-openapi validates the payload oneOf structurally. Validate the
	// complete envelope with the shipped discriminator-selected payload schema so
	// overlapping historical payload shapes do not mask a strict field leak.
	envelopeSchema := *factoryEventSchema
	envelopeSchema.Properties = make(map[string]*openapi3.SchemaRef, len(factoryEventSchema.Properties))
	for name, property := range factoryEventSchema.Properties {
		envelopeSchema.Properties[name] = property
	}
	envelopeSchema.Properties["payload"] = &openapi3.SchemaRef{Value: payloadSchemaRef.Value}
	if err := envelopeSchema.VisitJSON(document); err != nil {
		t.Fatalf("served Factory Event type %q does not validate against bundled schema: %v", event.Type, err)
	}
}

func loadRepresentativePublicFactoryEvents(t *testing.T) []interfaces.FactoryEvent {
	t.Helper()

	data, err := os.ReadFile("../testdata/canonical-event-vocabulary-stream.json")
	if err != nil {
		t.Fatalf("read canonical Factory Event vocabulary fixture: %v", err)
	}
	var events []interfaces.FactoryEvent
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("decode canonical Factory Event vocabulary fixture: %v", err)
	}

	for index := range events {
		if events[index].Type == interfaces.FactoryEventTypeDispatchWorkerSessionAssoc {
			events[index].Payload = json.RawMessage(`{"workerSessionId":"worker-session-1","model":"gpt-5.6-luna","reasoningEffort":"high"}`)
		}
	}
	return events
}
