package functional_test

import (
	"testing"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

type recordedFactoryWorkRequestEvent struct {
	RequestID string
	Source    string
	Payload   factoryapi.WorkRequestEventPayload
}

func replayEventCount(artifact *interfaces.ReplayArtifact, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range artifact.Events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func assertReplayWorkRequestRecorded(t *testing.T, artifact *interfaces.ReplayArtifact, requestID, source string, workItems int, relations int) {
	t.Helper()

	for _, record := range replayWorkRequestEvents(t, artifact) {
		if record.RequestID != requestID {
			continue
		}
		if record.Source != source {
			t.Fatalf("work request %s source = %q, want %q", requestID, record.Source, source)
		}
		if got := len(factoryWorksValue(record.Payload.Works)); got != workItems {
			t.Fatalf("work request %s work items = %d, want %d", requestID, got, workItems)
		}
		if got := len(factoryRelationsValue(record.Payload.Relations)); got != relations {
			t.Fatalf("work request %s relations = %d, want %d", requestID, got, relations)
		}
		return
	}
	t.Fatalf("replay artifact missing work request %s: %#v", requestID, replayWorkRequestEvents(t, artifact))
}

func replayWorkRequestEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []recordedFactoryWorkRequestEvent {
	if t != nil {
		t.Helper()
	}
	return replayWorkRequestEventsFromEvents(t, artifact.Events)
}

func replayWorkRequestEventsFromEvents(t *testing.T, events []factoryapi.FactoryEvent) []recordedFactoryWorkRequestEvent {
	if t != nil {
		t.Helper()
	}

	var out []recordedFactoryWorkRequestEvent
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			if t == nil {
				panic(err)
			}
			t.Fatalf("decode work request event %q: %v", event.Id, err)
		}
		source := stringPointerValue(payload.Source)
		if source == "" {
			source = stringPointerValue(event.Context.Source)
		}
		out = append(out, recordedFactoryWorkRequestEvent{
			RequestID: stringPointerValue(event.Context.RequestId),
			Source:    source,
			Payload:   payload,
		})
	}
	return out
}

func factoryWorksValue(value *[]factoryapi.Work) []factoryapi.Work {
	if value == nil {
		return nil
	}
	return *value
}

func factoryRelationsValue(value *[]factoryapi.Relation) []factoryapi.Relation {
	if value == nil {
		return nil
	}
	return *value
}

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func eventString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func eventStringSlice(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}

func stringSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}
