package projections_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWorkstationResultFromGenerated_MapsProviderFailureOnlyWireToFailureMetadata(t *testing.T) {
	t0 := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	family := factoryapi.WorkFailureFamily(interfaces.WorkFailureFamilyRetryable)
	failureType := factoryapi.WorkFailureType(interfaces.WorkFailureTypeTimeout)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-wire-timeout",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeDispatchResponse,
			"response/dispatch-wire-timeout",
			3,
			t0.Add(3*time.Second),
			factoryapi.FactoryEventContext{
				DispatchId: stringPtrForProjectionTest("dispatch-wire-timeout"),
				TraceIds:   stringSlicePtrForProjectionTest([]string{"trace-1"}),
				WorkIds:    stringSlicePtrForProjectionTest([]string{"work-1"}),
			},
			factoryapi.DispatchResponseEventPayload{
				TransitionId: "t-review",
				Outcome:      factoryapi.WorkOutcomeFailed,
				ProviderFailure: &factoryapi.ProviderFailureMetadata{
					Family: &family,
					Type:   &failureType,
				},
			},
		),
	}

	state, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(state.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want 1 completion", state.CompletedDispatches)
	}
	got := state.CompletedDispatches[0].Result
	if got.FailureMetadata == nil {
		t.Fatal("projected failure metadata = nil, want retryable/timeout from wire provider_failure")
	}
	if got.FailureMetadata.Family != interfaces.WorkFailureFamilyRetryable {
		t.Fatalf("projected family = %q, want retryable", got.FailureMetadata.Family)
	}
	if got.FailureMetadata.Type != interfaces.WorkFailureTypeTimeout {
		t.Fatalf("projected type = %q, want timeout", got.FailureMetadata.Type)
	}
}

func TestWorkstationResultFromGenerated_OmitsFailureMetadataWhenWireUnset(t *testing.T) {
	t0 := time.Date(2026, 4, 19, 12, 5, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-accepted",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeDispatchResponse,
			"response/dispatch-accepted",
			3,
			t0.Add(3*time.Second),
			factoryapi.FactoryEventContext{
				DispatchId: stringPtrForProjectionTest("dispatch-accepted"),
				TraceIds:   stringSlicePtrForProjectionTest([]string{"trace-1"}),
				WorkIds:    stringSlicePtrForProjectionTest([]string{"work-1"}),
			},
			factoryapi.DispatchResponseEventPayload{
				TransitionId: "t-review",
				Outcome:      factoryapi.WorkOutcomeAccepted,
			},
		),
	}

	state, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(state.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want 1 completion", state.CompletedDispatches)
	}
	got := state.CompletedDispatches[0].Result
	if got.FailureMetadata != nil {
		t.Fatalf("projected failure metadata = %#v, want nil", got.FailureMetadata)
	}
}
