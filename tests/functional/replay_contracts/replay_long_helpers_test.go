//go:build functionallong

package replay_contracts

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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

func normalizeReplayContractStdout(stdout string, trim bool) string {
	if trim {
		return strings.TrimSpace(stdout)
	}
	return stdout
}

func assertReplayArtifactDoesNotContainRawValue(t *testing.T, artifactPath, rawValue string) {
	t.Helper()

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read replay artifact %s: %v", artifactPath, err)
	}
	if strings.Contains(string(data), rawValue) {
		t.Fatalf("replay artifact %s leaked raw environment value %q", artifactPath, rawValue)
	}
}

func replayEventCount(artifact *interfaces.ReplayArtifact, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range artifact.Events {
		if string(event.Type) == string(eventType) {
			count++
		}
	}
	return count
}

func factoryRelationsValue(value *[]factoryapi.Relation) []factoryapi.Relation {
	if value == nil {
		return nil
	}
	return *value
}

func strPtr(value string) *string {
	return &value
}
