package functional_test

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func dispatchInputsIncludeWorkNameFromHistory(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	event factoryapi.FactoryEvent,
	payload factoryapi.DispatchRequestEventPayload,
	workName string,
) bool {
	t.Helper()

	for _, work := range dispatchInputWorksFromHistory(t, events, event, payload) {
		if work.Name == workName {
			return true
		}
	}
	return false
}

func dispatchInputWorksFromHistory(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	event factoryapi.FactoryEvent,
	payload factoryapi.DispatchRequestEventPayload,
) []factoryapi.Work {
	t.Helper()

	workByID := workRequestWorksByID(t, events)
	ordered := make([]factoryapi.Work, 0, len(payload.Inputs))
	for _, workID := range dispatchInputWorkIDsForTests(payload, event.Context) {
		if work, ok := workByID[workID]; ok {
			ordered = append(ordered, work)
		}
	}
	return ordered
}

func workRequestWorksByID(t *testing.T, events []factoryapi.FactoryEvent) map[string]factoryapi.Work {
	t.Helper()

	workByID := make(map[string]factoryapi.Work)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST payload %q: %v", event.Id, err)
		}
		for _, work := range workSliceForTests(payload.Works) {
			if workID := eventString(work.WorkId); workID != "" {
				workByID[workID] = work
			}
		}
	}
	return workByID
}

func dispatchInputWorkIDsForTests(
	payload factoryapi.DispatchRequestEventPayload,
	context factoryapi.FactoryEventContext,
) []string {
	ordered := make([]string, 0, len(payload.Inputs)+len(eventStringSlice(context.WorkIds)))
	for _, input := range payload.Inputs {
		ordered = appendUniqueDispatchWorkID(ordered, input.WorkId)
	}
	for _, workID := range eventStringSlice(context.WorkIds) {
		ordered = appendUniqueDispatchWorkID(ordered, workID)
	}
	return ordered
}

func appendUniqueDispatchWorkID(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func workSliceForTests(works *[]factoryapi.Work) []factoryapi.Work {
	if works == nil {
		return nil
	}
	return *works
}
