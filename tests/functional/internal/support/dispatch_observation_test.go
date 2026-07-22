package support_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestObserveDispatchEvents_ResponseOnlyRetainsPublicTransitionAndWorkIdentity(t *testing.T) {
	payload := factoryapi.FactoryEvent_Payload{}
	if err := payload.FromDispatchResponseEventPayload(factoryapi.DispatchResponseEventPayload{
		TransitionId: "review",
		Outcome:      factoryapi.WorkOutcomeAccepted,
	}); err != nil {
		t.Fatalf("encode response payload: %v", err)
	}
	dispatchID := "dispatch-1"
	workIDs := []string{"work-1"}
	eventTime := time.Now().UTC()

	observed := support.ObserveDispatchEvents(t, []factoryapi.FactoryEvent{{
		Type:    factoryapi.FactoryEventTypeDispatchResponse,
		Payload: payload,
		Context: factoryapi.FactoryEventContext{
			DispatchId: &dispatchID,
			EventTime:  eventTime,
			WorkIds:    &workIDs,
		},
	}})

	if len(observed) != 1 {
		t.Fatalf("observations = %#v, want one", observed)
	}
	if observed[0].Request.TransitionId != "review" || observed[0].Response == nil {
		t.Fatalf("observation = %#v, want response-only public dispatch", observed[0])
	}
	if !support.DispatchObservationIncludesWork(observed[0], "work-1") {
		t.Fatalf("observation = %#v, want work-1 correlation", observed[0])
	}
	if !observed[0].StartedAt.Equal(eventTime) || !observed[0].CompletedAt.Equal(eventTime) {
		t.Fatalf("observation times = %s/%s, want %s", observed[0].StartedAt, observed[0].CompletedAt, eventTime)
	}
}
