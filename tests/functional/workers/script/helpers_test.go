package script_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func updateScriptFixtureFactory(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	mutate(cfg)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func assertCommandArgs(t *testing.T, req platformprocess.CommandRequest, want []string) {
	t.Helper()

	if !reflect.DeepEqual(req.Args, want) {
		t.Fatalf("command args = %#v, want %#v", req.Args, want)
	}
}

func assertSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func assertProviderWorkCompleted(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	assertSessionPlaces(t, listed, map[string]int{
		"task:complete": 1, "task:init": 0, "task:failed": 0,
	})
}

func assertDispatchOutput(t *testing.T, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output == nil || *payload.Output != want {
			t.Fatalf("dispatch output = %#v, want %q", payload.Output, want)
		}
		return
	}
	t.Fatalf("Factory Event history has no dispatch response: %#v", events)
}

func assertDispatchErrorContains(t *testing.T, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, payload := range dispatchResponses(t, events) {
		if payload.Error != nil && strings.Contains(*payload.Error, want) {
			return
		}
	}
	t.Fatalf("dispatch responses do not contain error %q", want)
}

func hasFactoryEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func factoryEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func dispatchResponses(t *testing.T, events []factoryapi.FactoryEvent) []factoryapi.DispatchResponseEventPayload {
	t.Helper()
	var responses []factoryapi.DispatchResponseEventPayload
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		responses = append(responses, payload)
	}
	return responses
}
