package factorysessionsse

import (
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestFactorySessionSSEResolvedIdentity_ExactAndDefaultSelectorsPublishCanonicalID(t *testing.T) {
	t.Parallel()

	const canonicalID = "3c1d4c6b-0d6a-4e8f-b0c0-9e5a2bb1d8aa"
	fixture := NewFactorySessionSSEFixture(t)
	service := newProgrammedFactorySessionEvents()
	history := factorySessionSSEEventsFromAPI(t, fixture.Retained)
	events := make(chan interfaces.FactoryEvent)
	close(events)
	for _, selector := range []string{canonicalID, factorysessions.DefaultSessionID} {
		service.SetSession(selector, factorySessionEventProgram{
			stream: interfaces.FactoryEventStream{
				StreamGenerationID:  factorySessionSSEFixtureStreamGenerationID,
				BackendScopeID:      factorySessionSSEFixtureBackendScopeID,
				LogicalSessionKeyID: factorySessionSSEFixtureLogicalSessionKey,
				FactorySessionID:    canonicalID,
				History:             history,
				Events:              events,
			},
		})
	}

	server := httptest.NewServer(newAPITestServer(service).Handler())
	defer server.Close()
	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	for _, selector := range []string{canonicalID, factorysessions.DefaultSessionID} {
		stream := harness.Open(server.URL, selector, "")

		if got := stream.Identity.FactorySessionID; got != canonicalID {
			t.Fatalf("%q stream identity = %q, want %q", selector, got, canonicalID)
		}
		if got := stream.Response.Header.Get(factorySessionSSEFactorySessionHeader); got != canonicalID {
			t.Fatalf("%q Factory Session header = %q, want %q", selector, got, canonicalID)
		}
		count, err := strconv.Atoi(stream.Response.Header.Get(factorySessionSSERetainedEventCountHeader))
		if err != nil || count != len(fixture.Retained) {
			t.Fatalf("%q retained count = %q (%v), want %d", selector, stream.Response.Header.Get(factorySessionSSERetainedEventCountHeader), err, len(fixture.Retained))
		}
		got := stream.ReadEvents(len(fixture.Retained))
		if !reflect.DeepEqual(got, fixture.Retained) {
			t.Fatalf("%q retained events changed across the legacy SSE boundary: got %#v, want %#v", selector, got, fixture.Retained)
		}
		stream.Close()
	}
}
