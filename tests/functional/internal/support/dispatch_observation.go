package support

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// DispatchEventObservation joins the customer-visible request and response
// records for one dispatch from the public Factory Event stream.
type DispatchEventObservation struct {
	DispatchID  string
	WorkIDs     []string
	StartedAt   time.Time
	CompletedAt time.Time
	Request     factoryapi.DispatchRequestEventPayload
	Response    *factoryapi.DispatchResponseEventPayload
}

// ObserveDispatchEvents projects public dispatch events without consulting
// runtime marking, scheduler, or service state.
func ObserveDispatchEvents(
	t testing.TB,
	events []factoryapi.FactoryEvent,
) []DispatchEventObservation {
	t.Helper()

	observations := make([]DispatchEventObservation, 0)
	byID := make(map[string]int)
	for _, event := range events {
		if event.Context.DispatchId == nil || *event.Context.DispatchId == "" {
			continue
		}
		dispatchID := *event.Context.DispatchId
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			request, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_REQUEST %q: %v", dispatchID, err)
			}
			observation := DispatchEventObservation{
				DispatchID: dispatchID,
				StartedAt:  event.Context.EventTime,
				Request:    request,
			}
			if event.Context.WorkIds != nil {
				observation.WorkIDs = append([]string(nil), (*event.Context.WorkIds)...)
			}
			byID[dispatchID] = len(observations)
			observations = append(observations, observation)
		case factoryapi.FactoryEventTypeDispatchResponse:
			response, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_RESPONSE %q: %v", dispatchID, err)
			}
			index, ok := byID[dispatchID]
			if !ok {
				observation := DispatchEventObservation{
					DispatchID:  dispatchID,
					StartedAt:   event.Context.EventTime,
					CompletedAt: event.Context.EventTime,
					Request: factoryapi.DispatchRequestEventPayload{
						TransitionId: response.TransitionId,
					},
					Response: &response,
				}
				if event.Context.WorkIds != nil {
					observation.WorkIDs = append([]string(nil), (*event.Context.WorkIds)...)
				}
				byID[dispatchID] = len(observations)
				observations = append(observations, observation)
				continue
			}
			observations[index].CompletedAt = event.Context.EventTime
			observations[index].Response = &response
		}
	}
	return observations
}

func DispatchObservationIncludesWork(
	observation DispatchEventObservation,
	workID string,
) bool {
	for _, candidate := range observation.WorkIDs {
		if candidate == workID {
			return true
		}
	}
	for _, input := range observation.Request.Inputs {
		if input.WorkId == workID {
			return true
		}
	}
	return false
}
