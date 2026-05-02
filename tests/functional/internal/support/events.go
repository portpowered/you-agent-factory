package support

import (
	"testing"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
)

func StringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func FactoryWorksValue(value *[]factoryapi.Work) []factoryapi.Work {
	if value == nil {
		return nil
	}
	return *value
}

func AssertSingleWorkRequestEvent(t *testing.T, events []factoryapi.FactoryEvent, requestID, workID, workTypeName string) {
	t.Helper()

	var matches []factoryapi.WorkRequestEventPayload
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest || StringPointerValue(event.Context.RequestId) != requestID {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
		}
		matches = append(matches, payload)
	}
	if len(matches) != 1 {
		t.Fatalf("WORK_REQUEST events for %q = %d, want 1", requestID, len(matches))
	}
	assertWorkRequestPayloadContainsWork(t, matches[0], workID, workTypeName)
}

func AssertSingleWorkRequestEventByWorkName(t *testing.T, events []factoryapi.FactoryEvent, workName, workTypeName string) {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
		}
		for _, work := range FactoryWorksValue(payload.Works) {
			if work.Name == workName && StringPointerValue(work.WorkTypeName) == workTypeName {
				return
			}
		}
	}
	t.Fatalf("missing WORK_REQUEST work item %q with work_type_name %q", workName, workTypeName)
}

func assertWorkRequestPayloadContainsWork(t *testing.T, payload factoryapi.WorkRequestEventPayload, workID, workTypeName string) {
	t.Helper()

	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("WORK_REQUEST type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
	}
	for _, work := range FactoryWorksValue(payload.Works) {
		if StringPointerValue(work.WorkId) == workID {
			if StringPointerValue(work.WorkTypeName) != workTypeName {
				t.Fatalf("work %q work_type_name = %q, want %q", workID, StringPointerValue(work.WorkTypeName), workTypeName)
			}
			return
		}
	}
	t.Fatalf("WORK_REQUEST missing work_id %q: %#v", workID, FactoryWorksValue(payload.Works))
}

func DispatchInputsIncludeWorkNameFromHistory(
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
	for _, workID := range dispatchInputWorkIDs(payload, event.Context) {
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
		for _, work := range FactoryWorksValue(payload.Works) {
			if workID := StringPointerValue(work.WorkId); workID != "" {
				workByID[workID] = work
			}
		}
	}
	return workByID
}

func dispatchInputWorkIDs(
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

func LastFactoryEventTick(events []factoryapi.FactoryEvent) int {
	tick := 0
	for _, event := range events {
		if event.Context.Tick > tick {
			tick = event.Context.Tick
		}
	}
	return tick
}

func eventStringSlice(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}
