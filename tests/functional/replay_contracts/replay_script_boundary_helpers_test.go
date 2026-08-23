package replay_contracts

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
