package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type listedObservationOwner struct {
	processLocalWorkerSessionService
	gets int
}

func (s *listedObservationOwner) GetObservation(context.Context, workersessions.GetObservationRequest) (workersessions.Observation, error) {
	s.gets++
	return workersessions.Observation{}, errors.New("optional capture unavailable")
}

func TestRecordedListReusesListedFactsAndRefreshesNextRequest(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "shared-provider"}
	owner := &listedObservationOwner{}
	tokens := 12
	owner.observationListResult.Observations = []workersessions.Observation{{
		WorkerSessionID: "worker-early", ProviderSession: ref, ProviderSessionAvailable: true,
		State: workersessions.StateCompleted, WorkIDs: []string{"work"}, AttemptID: "dispatch-early",
		TokenUsage: &workersessions.TokenUsage{TotalTokens: &tokens},
	}}
	service := newRecordedWorkerSessionObservation(owner,
		&recordingfixtures.ScriptedRuntimeLedger{Events: recordedObservationTestEvents(t, base, "work")},
		func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{
				WorkItemsByID: map[string]work.FactoryWorkItem{"work": {ID: "work"}},
				ProviderSessions: []interfaces.FactoryWorldProviderSessionRecord{
					{DispatchID: "dispatch-early", ProviderSession: providers.SessionMetadata{Provider: string(ref.Provider), Kind: ref.Kind, ID: ref.ID}},
					{DispatchID: "dispatch-late", ProviderSession: providers.SessionMetadata{Provider: string(ref.Provider), Kind: ref.Kind, ID: ref.ID}},
				},
			}, nil
		}, platformclock.NewDeterministic(base, time.Second), nil)
	read := func(want int) {
		t.Helper()
		result, err := service.ListObservations(t.Context(), workersessions.ListObservationsRequest{WorkID: "work"})
		if err != nil || len(result.Observations) != 2 {
			t.Fatalf("authoritative rows lost on optional failure: %+v, %v", result, err)
		}
		for _, row := range result.Observations {
			if row.WorkerSessionID == "worker-early" {
				if row.TokenUsage == nil || *row.TokenUsage.TotalTokens != want {
					t.Fatalf("listed facts lost or stale: %+v", row)
				}
				*row.TokenUsage.TotalTokens = -1
			}
		}
	}
	read(12)
	if owner.gets != 1 || tokens != 12 {
		t.Fatalf("gets=%d tokens=%d; want historical fallback only and detached results", owner.gets, tokens)
	}
	tokens = 99
	read(99)
	if owner.gets != 2 {
		t.Fatalf("gets=%d, want one historical fallback per request", owner.gets)
	}
}

func TestListedObservationRequiresExactIdentityAndAssociation(t *testing.T) {
	ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider"}
	row := workersessions.Observation{WorkerSessionID: "worker", ProviderSessionAvailable: true, ProviderSession: ref}
	index := listedObservationIndex([]workersessions.Observation{row})
	if !listedObservationMatches(row, ref, index) {
		t.Fatal("matching listed row not reused")
	}
	for _, change := range []string{"identity", "association", "unavailable"} {
		other := row
		switch change {
		case "identity":
			other.WorkerSessionID = "sibling"
		case "association":
			other.ProviderSession.ID = "other"
		case "unavailable":
			other.ProviderSessionAvailable = false
		}
		if listedObservationMatches(row, ref, listedObservationIndex([]workersessions.Observation{other})) {
			t.Fatalf("reused %s mismatch", change)
		}
	}
}
