package support

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

func FactoryRelationsValue(value *[]factoryapi.Relation) []factoryapi.Relation {
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

func CountFactoryEvents(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}
