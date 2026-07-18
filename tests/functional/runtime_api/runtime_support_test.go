package runtime_api

import (
	"fmt"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
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

func tokenPlaces(snap petri.MarkingSnapshot) map[string]int {
	places := make(map[string]int)
	for _, tok := range snap.Tokens {
		places[tok.PlaceID]++
	}
	return places
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
