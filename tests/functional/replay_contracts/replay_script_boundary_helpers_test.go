package replay_contracts

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type scriptBoundaryEventIndices struct {
	dispatch  int
	request   int
	response  int
	completed int
}

func requireScriptResponseEventIndices(t *testing.T, events []factoryapi.FactoryEvent) scriptBoundaryEventIndices {
	t.Helper()

	indices := scriptBoundaryEventIndices{
		dispatch:  indexOfReplayContractEventType(events, factoryapi.FactoryEventTypeDispatchRequest, 0),
		request:   indexOfReplayContractEventType(events, factoryapi.FactoryEventTypeScriptRequest, 0),
		response:  indexOfReplayContractEventType(events, factoryapi.FactoryEventTypeScriptResponse, 0),
		completed: indexOfReplayContractEventType(events, factoryapi.FactoryEventTypeDispatchResponse, 0),
	}
	if indices.dispatch < 0 || indices.request < 0 || indices.response < 0 || indices.completed < 0 {
		t.Fatalf("event order = %v, want dispatch-request, script-request, script-response, dispatch-response", replayContractEventTypes(events))
	}
	return indices
}

func assertScriptEventsRecordedInArtifact(t *testing.T, liveEvents []factoryapi.FactoryEvent, recordedEvents []factoryapi.FactoryEvent) {
	t.Helper()

	recordedByID := make(map[string]factoryapi.FactoryEvent, len(recordedEvents))
	for _, event := range recordedEvents {
		recordedByID[event.Id] = event
	}

	for _, live := range liveEvents {
		if live.Type != factoryapi.FactoryEventTypeScriptRequest && live.Type != factoryapi.FactoryEventTypeScriptResponse {
			continue
		}

		recorded, ok := recordedByID[live.Id]
		if !ok {
			t.Fatalf("recorded artifact missing script event %s from live history; artifact events=%v", live.Id, replayContractEventTypes(recordedEvents))
		}
		if recorded.Type != live.Type {
			t.Fatalf("recorded script event %s = type %s, live type %s", live.Id, recorded.Type, live.Type)
		}

		liveJSON, err := json.Marshal(live)
		if err != nil {
			t.Fatalf("marshal live script event %s: %v", live.Id, err)
		}
		recordedJSON, err := json.Marshal(recorded)
		if err != nil {
			t.Fatalf("marshal recorded script event %s: %v", recorded.Id, err)
		}
		if string(recordedJSON) != string(liveJSON) {
			t.Fatalf("recorded script event %s does not match live history\nrecorded=%s\nlive=%s", live.Id, recordedJSON, liveJSON)
		}
	}
}

func assertFunctionalScriptEventDoesNotLeak(t *testing.T, event factoryapi.FactoryEvent, forbidden []string) {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal script event: %v", err)
	}
	body := string(encoded)
	for _, value := range forbidden {
		if strings.Contains(body, value) {
			t.Fatalf("script event leaked %s: %s", value, body)
		}
	}
}

func indexOfReplayContractEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType, start int) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(events); i++ {
		if events[i].Type == eventType {
			return i
		}
	}
	return -1
}

func replayContractEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	return types
}

func normalizeReplayContractStdout(stdout string, trim bool) string {
	if trim {
		return strings.TrimSpace(stdout)
	}
	return stdout
}
