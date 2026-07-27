package runtime_api

import (
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func normalizeSubmitRequestsForFunctionalTest(requests []work.SubmitRequest) []work.SubmitRequest {
	if len(requests) == 0 {
		return nil
	}
	normalized := make([]work.SubmitRequest, len(requests))
	copy(normalized, requests)
	traceID := ""
	for _, request := range normalized {
		if request.TraceID != "" {
			traceID = request.TraceID
			break
		}
	}
	if traceID == "" {
		traceID = fmt.Sprintf("trace-functional-%d", time.Now().UnixNano())
	}
	for i := range normalized {
		if normalized[i].TraceID == "" {
			normalized[i].TraceID = traceID
		}
	}
	return normalized
}

func functionalEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	out := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		out = append(out, event.Type)
	}
	return out
}

var retiredFunctionalFactoryEventTypes = []string{
	"RUN_STARTED",
	"INITIAL_STRUCTURE",
	"RELATIONSHIP_CHANGE",
	"DISPATCH_CREATED",
	"DISPATCH_COMPLETED",
	"FACTORY_STATE_CHANGE",
	"RUN_FINISHED",
}

func stringValueFromFunctionalPtr[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func requireGeneratedSchemaRunStartedPayload(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.RunRequestEventPayload {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeRunRequest {
			continue
		}
		payload, err := event.Payload.AsRunRequestEventPayload()
		if err != nil {
			t.Fatalf("decode run-request payload %q: %v", event.Id, err)
		}
		if payload.Factory.WorkTypes == nil || len(*payload.Factory.WorkTypes) == 0 {
			t.Fatalf("run-request payload factory missing work types: %#v", payload.Factory)
		}
		return payload
	}
	t.Fatalf("recorded events missing RUN_REQUEST: %#v", functionalEventTypes(events))
	return factoryapi.RunRequestEventPayload{}
}
