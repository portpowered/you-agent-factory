package dispatch

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// assertPublicDispatchEvents protects the customer-visible dispatch lifecycle
// used by the characterization scenarios. It deliberately validates only the
// Factory Event projection; scheduler and Petri implementation details remain
// outside this functional package's contract.
func assertPublicDispatchEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantAtLeast int,
) []support.DispatchEventObservation {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) < wantAtLeast {
		t.Fatalf("public dispatch observations = %d, want at least %d", len(dispatches), wantAtLeast)
	}

	seen := make(map[string]struct{}, len(dispatches))
	for index, dispatch := range dispatches {
		if dispatch.DispatchID == "" {
			t.Errorf("dispatch observation %d has empty dispatch ID", index)
		}
		if _, ok := seen[dispatch.DispatchID]; ok {
			t.Errorf("dispatch ID %q appears more than once", dispatch.DispatchID)
		}
		seen[dispatch.DispatchID] = struct{}{}
		if dispatch.Request.TransitionId == "" {
			t.Errorf("dispatch %q has empty request transition ID", dispatch.DispatchID)
		}
		if dispatch.Response == nil {
			t.Errorf("dispatch %q has no public response event", dispatch.DispatchID)
			continue
		}
		if dispatch.Response.TransitionId != dispatch.Request.TransitionId {
			t.Errorf(
				"dispatch %q response transition = %q, want request transition %q",
				dispatch.DispatchID,
				dispatch.Response.TransitionId,
				dispatch.Request.TransitionId,
			)
		}
		if dispatch.CompletedAt.Before(dispatch.StartedAt) {
			t.Errorf(
				"dispatch %q completed at %s before it started at %s",
				dispatch.DispatchID,
				dispatch.CompletedAt,
				dispatch.StartedAt,
			)
		}
	}
	return dispatches
}

func assertDispatchTransitionSequence(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	want []string,
) {
	t.Helper()

	if len(dispatches) != len(want) {
		t.Fatalf("dispatch transition sequence length = %d, want %d", len(dispatches), len(want))
	}
	for index, transitionID := range want {
		if got := dispatches[index].Request.TransitionId; got != transitionID {
			t.Errorf("dispatch transition %d = %q, want %q", index, got, transitionID)
		}
		if dispatches[index].Response == nil || dispatches[index].Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Errorf(
				"dispatch transition %d (%q) response = %#v, want ACCEPTED",
				index,
				transitionID,
				dispatches[index].Response,
			)
		}
	}
}

func assertDispatchTransitionSubsequence(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	want []string,
) {
	t.Helper()

	position := 0
	for _, dispatch := range dispatches {
		if position < len(want) && dispatch.Request.TransitionId == want[position] {
			position++
		}
	}
	if position != len(want) {
		t.Fatalf("dispatch transitions = %v, want ordered subsequence %v", dispatchTransitionIDs(dispatches), want)
	}
}

func assertDispatchTransitionOutcomes(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	transitionID string,
	want []factoryapi.WorkOutcome,
) {
	t.Helper()

	var got []factoryapi.WorkOutcome
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId == transitionID && dispatch.Response != nil {
			got = append(got, dispatch.Response.Outcome)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("%s dispatch outcomes = %v, want %v", transitionID, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("%s dispatch outcome %d = %q, want %q", transitionID, index, got[index], want[index])
		}
	}
}

func assertDispatchTransitionOutcomeCount(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	transitionID string,
	outcome factoryapi.WorkOutcome,
	want int,
) {
	t.Helper()

	got := 0
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId == transitionID && dispatch.Response != nil && dispatch.Response.Outcome == outcome {
			got++
		}
	}
	if got != want {
		t.Errorf("%s %s dispatches = %d, want %d", transitionID, outcome, got, want)
	}
}

func assertDispatchesDoNotOverlap(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	transitions map[string]struct{},
) {
	t.Helper()

	var previous *support.DispatchEventObservation
	for index := range dispatches {
		dispatch := &dispatches[index]
		if _, ok := transitions[dispatch.Request.TransitionId]; !ok {
			continue
		}
		if previous != nil && dispatch.StartedAt.Before(previous.CompletedAt) {
			t.Errorf(
				"resource-limited dispatch %q started at %s before prior dispatch %q completed at %s",
				dispatch.DispatchID,
				dispatch.StartedAt,
				previous.DispatchID,
				previous.CompletedAt,
			)
		}
		previous = dispatch
	}
}

func dispatchTransitionIDs(dispatches []support.DispatchEventObservation) []string {
	ids := make([]string, 0, len(dispatches))
	for _, dispatch := range dispatches {
		ids = append(ids, dispatch.Request.TransitionId)
	}
	return ids
}
