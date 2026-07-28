package relationships

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	dependencyStartWorkstation  = "start"
	dependencyFinishWorkstation = "finish"
	dependencyRequiredState     = "complete"
)

// TestDependentWorkWaitsForPrerequisiteTargetState proves through public Work
// listings and Factory Event dispatch observations that a DEPENDS_ON dependent
// stays undispatched at its initial state until the prerequisite reaches the
// declared requiredState, then proceeds through the public work session once
// that prerequisite target state is satisfied.
func TestDependentWorkWaitsForPrerequisiteTargetState(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	prerequisiteWorkID := "task-prerequisite-a"
	dependentWorkID := "task-dependent-b"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     prerequisiteWorkID,
		Payload:    []byte("prerequisite task"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     dependentWorkID,
		Payload:    []byte("dependent task"),
		Relations: []work.Relation{
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteWorkID,
				RequiredState: dependencyRequiredState,
			},
		},
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
	)

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		15*time.Second,
	)

	assertDependencyWorkLocations(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
		support.WorkCustomerLocation("task", "complete"):   2,
	})
	if !support.HasWorkAtCustomerState(listed, prerequisiteWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("prerequisite work %q not at %q in public listing: %#v", prerequisiteWorkID, dependencyRequiredState, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("dependent work %q not at %q in public listing: %#v", dependentWorkID, dependencyRequiredState, listed)
	}

	if got := len(support.ProviderCallsForWorker(provider, "starter")); got != 2 {
		t.Fatalf("starter provider calls = %d, want 2 (prerequisite then dependent)", got)
	}
	if got := len(support.ProviderCallsForWorker(provider, "finisher")); got != 2 {
		t.Fatalf("finisher provider calls = %d, want 2 (prerequisite then dependent)", got)
	}

	prerequisiteCompleteSequence, dependentStartSequence := dependencyDispatchOrdering(
		t,
		events,
		prerequisiteWorkID,
		dependentWorkID,
	)
	if dependentStartSequence <= prerequisiteCompleteSequence {
		t.Fatalf(
			"dependent %q dispatch at %q sequence = %d, want after prerequisite %q complete sequence %d",
			dependentWorkID,
			dependencyStartWorkstation,
			dependentStartSequence,
			prerequisiteWorkID,
			prerequisiteCompleteSequence,
		)
	}

	if session.Runtime.Progress.Categories.Terminal != 2 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want two terminal and zero failed", session.Runtime.Progress.Categories)
	}
}

// TestDependentWorkDoesNotDispatchAfterPrerequisiteFailure proves through public
// Work listings and Factory Event dispatch observations that a DEPENDS_ON
// dependent never receives a worker dispatch when its prerequisite reaches a
// failed terminal outcome instead of the declared requiredState.
func TestDependentWorkDoesNotDispatchAfterPrerequisiteFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	prerequisiteWorkID := "task-prerequisite-a"
	dependentWorkID := "task-dependent-b"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     prerequisiteWorkID,
		Payload:    []byte("prerequisite task"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     dependentWorkID,
		Payload:    []byte("dependent task"),
		Relations: []work.Relation{
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteWorkID,
				RequiredState: dependencyRequiredState,
			},
		},
	})

	provider := testutil.NewMockProviderWithErrors(
		[]workerexecution.InferenceResponse{
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
		},
		[]error{
			nil,
			errors.New("prerequisite finish failed"),
		},
	)

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		15*time.Second,
	)

	if !support.HasWorkAtCustomerState(listed, prerequisiteWorkID, support.WorkCustomerLocation("task", "failed")) {
		t.Fatalf("prerequisite work %q not at failed in public listing: %#v", prerequisiteWorkID, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", "failed")) {
		t.Fatalf("dependent work %q not at blocked failed state in public listing: %#v", dependentWorkID, listed)
	}
	if support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("dependent work %q reached %q after prerequisite failure: %#v", dependentWorkID, dependencyRequiredState, listed)
	}
	if support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", "processing")) {
		t.Fatalf("dependent work %q reached processing after prerequisite failure: %#v", dependentWorkID, listed)
	}

	if got := len(support.ProviderCallsForWorker(provider, "starter")); got != 1 {
		t.Fatalf("starter provider calls = %d, want 1 (prerequisite only)", got)
	}
	if got := len(support.ProviderCallsForWorker(provider, "finisher")); got != 1 {
		t.Fatalf("finisher provider calls = %d, want 1 (prerequisite only)", got)
	}

	assertNoDependentStartDispatch(t, events, dependentWorkID)

	if session.Runtime.Progress.Categories.Terminal != 0 || session.Runtime.Progress.Categories.Failed != 2 {
		t.Fatalf("session progress categories = %+v, want zero terminal and two failed", session.Runtime.Progress.Categories)
	}
}

func assertDependencyWorkLocations(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Fatalf("CountWorkAtCustomerState(%q) = %d, want %d; listed=%#v", location, got, want, listed)
		}
	}
}

func dependencyDispatchOrdering(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	prerequisiteWorkID, dependentWorkID string,
) (prerequisiteCompleteSequence, dependentStartSequence int) {
	t.Helper()

	prerequisiteCompleteSequence = -1
	dependentStartSequence = -1

	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				continue
			}
			if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.TransitionId != dependencyFinishWorkstation {
				continue
			}
			if !dispatchEventIncludesWork(event.Context.WorkIds, prerequisiteWorkID) {
				continue
			}
			prerequisiteCompleteSequence = event.Context.Sequence
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				continue
			}
			if payload.TransitionId != dependencyStartWorkstation {
				continue
			}
			if !dispatchRequestIncludesWork(payload, dependentWorkID) {
				continue
			}
			if prerequisiteCompleteSequence < 0 {
				t.Fatalf(
					"dependent work %q received %q dispatch before prerequisite %q reached %q",
					dependentWorkID,
					dependencyStartWorkstation,
					prerequisiteWorkID,
					dependencyRequiredState,
				)
			}
			if dependentStartSequence < 0 {
				dependentStartSequence = event.Context.Sequence
			}
		}
	}

	if prerequisiteCompleteSequence < 0 {
		t.Fatalf("prerequisite work %q never reached %q through public dispatch", prerequisiteWorkID, dependencyRequiredState)
	}
	if dependentStartSequence < 0 {
		t.Fatalf("dependent work %q never received a public %q dispatch", dependentWorkID, dependencyStartWorkstation)
	}
	return prerequisiteCompleteSequence, dependentStartSequence
}

func dispatchRequestIncludesWork(payload factoryapi.DispatchRequestEventPayload, workID string) bool {
	for _, input := range payload.Inputs {
		if input.WorkId == workID {
			return true
		}
	}
	return false
}

func dispatchEventIncludesWork(workIDs *[]string, workID string) bool {
	if workIDs == nil {
		return false
	}
	for _, candidate := range *workIDs {
		if candidate == workID {
			return true
		}
	}
	return false
}

func assertNoDependentStartDispatch(t *testing.T, events []factoryapi.FactoryEvent, dependentWorkID string) {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			continue
		}
		if payload.TransitionId != dependencyStartWorkstation {
			continue
		}
		if dispatchRequestIncludesWork(payload, dependentWorkID) {
			t.Fatalf(
				"dependent work %q received public %q dispatch after prerequisite failure at sequence %d",
				dependentWorkID,
				dependencyStartWorkstation,
				event.Context.Sequence,
			)
		}
	}
}
