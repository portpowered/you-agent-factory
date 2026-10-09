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
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type preparedScopedTestLedger struct {
	recordings.RuntimeLedger
	byWork map[string]recordings.WorkerSessionWorkFacts
	err    error
}

func (ledger *preparedScopedTestLedger) CurrentWorkerSessionFacts(ctx context.Context, workerID string) (recordings.WorkerSessionWorkFacts, error) {
	if err := ctx.Err(); err != nil {
		return recordings.WorkerSessionWorkFacts{}, err
	}
	for _, facts := range ledger.byWork {
		for _, association := range facts.Associations {
			if association.WorkerSessionID == workerID {
				return facts, ledger.err
			}
		}
	}
	return recordings.WorkerSessionWorkFacts{}, ledger.err
}

func (ledger *preparedScopedTestLedger) CurrentWorkerSessionWorkFacts(ctx context.Context, id string) (recordings.WorkerSessionWorkFacts, error) {
	if err := ctx.Err(); err != nil {
		return recordings.WorkerSessionWorkFacts{}, err
	}
	return ledger.byWork[id], ledger.err
}

// Legacy exact-read fixtures script a world independently of their event
// payloads. Prepare their selected peer responses during fixture setup, before
// any scoped request; the request must never call their scripted projector.
func prepareScopedTestFacts(service workersessions.Service) {
	s := service.(*recordedWorkerSessionObservation)
	if s.ledger == nil {
		return
	}
	if _, ok := s.ledger.(recordings.WorkerSessionWorkProjectionReader); ok {
		return
	}
	ordered := s.canonicalEvents()
	world, err := s.projectRecordedWorldState(context.Background(), ordered, ordered, latestFactoryEventTick(ordered))
	ledger := &preparedScopedTestLedger{RuntimeLedger: s.ledger, byWork: make(map[string]recordings.WorkerSessionWorkFacts), err: err}
	workIDs := map[string]struct{}{"": {}}
	for id := range world.WorkItemsByID {
		workIDs[id] = struct{}{}
	}
	for _, event := range ordered {
		for _, id := range pointerStringSlice(event.Context.WorkIDs) {
			workIDs[id] = struct{}{}
		}
	}
	for workID := range workIDs {
		ledger.byWork[workID] = preparedScopedTestWorkFacts(workID, s.ledger.StreamGenerationID(), world, ordered)
	}
	s.ledger = ledger
}

func preparedScopedTestWorkFacts(workID, generation string, world interfaces.FactoryWorldState, ordered []interfaces.FactoryEvent) recordings.WorkerSessionWorkFacts {
	associations, requests := recordedDispatchFacts(ordered)
	index := newRecordedDispatchEventIndex(ordered)
	completed := recordedDispatchStateMaps(world)
	facts := recordings.WorkerSessionWorkFacts{
		KnownWork: true, StreamGenerationID: generation,
		Associations:    make(map[string]recordings.WorkerSessionAssociationFacts),
		Requests:        make(map[string]interfaces.FactoryWorldDispatch),
		StateCursors:    make(map[string]recordings.CanonicalEventCursor),
		ResponseCursors: make(map[string]recordings.CanonicalEventCursor),
		ResponseTimes:   make(map[string]time.Time),
		Interruptions:   make(map[string]interfaces.DispatchInterruptedEventPayload),
		World:           interfaces.FactoryWorldState{ActiveDispatches: make(map[string]interfaces.FactoryWorldDispatch)},
	}
	for id, association := range associations {
		fact := recordedDispatchFact(id, association, requests, completed, world.ProviderSessions, world.ActiveDispatches, index)
		if workID != "" && !containsRecordedWorkID(fact.workIDs, workID) {
			continue
		}
		facts.Associations[id] = recordings.WorkerSessionAssociationFacts{WorkerSessionID: association.workerSessionID, TurnID: association.turnID, Model: association.model, ReasoningEffort: association.reasoningEffort, AssociatedAt: association.eventTime}
		facts.Requests[id] = interfaces.FactoryWorldDispatch{DispatchID: id, WorkItemIDs: fact.workIDs, StartedAt: requests[id].startedAt}
		facts.StateCursors[id] = recordings.CanonicalEventCursor{StreamGenerationID: facts.StreamGenerationID, Sequence: recordings.CanonicalEventSequence(index.cursors[id])}
		facts.ResponseTimes[id] = index.responseTimes[id]
		if value, ok := index.interruptions[id]; ok {
			facts.Interruptions[id] = interfaces.DispatchInterruptedEventPayload{InterruptedAt: firstRecordedInterruptionTime(value), Reason: value.reason}
		}
		for _, event := range ordered {
			if event.Type == interfaces.FactoryEventTypeDispatchResponse && stringPointerValue(event.Context.DispatchID) == id {
				facts.ResponseCursors[id] = recordings.CanonicalEventCursor{StreamGenerationID: facts.StreamGenerationID, Sequence: recordings.CanonicalEventSequence(event.Context.Sequence)}
			}
		}
		if value, ok := completed[id]; ok {
			facts.World.CompletedDispatches = append(facts.World.CompletedDispatches, value)
		}
		if value, ok := world.ActiveDispatches[id]; ok {
			facts.World.ActiveDispatches[id] = value
		}
		for _, value := range world.ProviderSessions {
			if value.DispatchID == id {
				facts.World.ProviderSessions = append(facts.World.ProviderSessions, value)
				break
			}
		}
	}
	return facts
}

func firstRecordedInterruptionTime(value recordedDispatchInterruptionFact) time.Time {
	if !value.interruptedAt.IsZero() {
		return value.interruptedAt
	}
	return value.eventTime
}

type unsupportedScopedTestLedger struct{ recordings.RuntimeLedger }

func (unsupportedScopedTestLedger) CanonicalEvents() []interfaces.FactoryEvent {
	panic("scoped list must not fall back to canonical history")
}

func TestScopedWorkListRequiresPreparedFactsEvenWithLiveRows(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name                    string
		withLive, withProjector bool
	}{
		{name: "recorded only"},
		{name: "matching live attempt", withLive: true},
		{name: "recorded with legacy projector", withProjector: true},
		{name: "live with legacy projector", withLive: true, withProjector: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			live := &scopedWorkLiveOwner{}
			if scenario.withLive {
				live.observationListResult.Observations = []workersessions.Observation{{WorkerSessionID: "worker", WorkIDs: []string{"work"}, State: workersessions.StateRunning}}
			}
			s := &recordedWorkerSessionObservation{Service: live, ledger: unsupportedScopedTestLedger{}}
			if scenario.withProjector {
				s.projector = func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
					t.Fatal("unsupported selected reader consulted legacy projector")
					return interfaces.FactoryWorldState{}, nil
				}
			}
			result, err := s.ListObservations(t.Context(), workersessions.ListObservationsRequest{WorkID: "work"})
			if !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) || len(result.Observations) != 0 {
				t.Fatalf("unsupported selected reader = %+v, %v; want typed error without partial rows", result, err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := s.ListObservations(ctx, workersessions.ListObservationsRequest{WorkID: "work"}); !errors.Is(err, workersessions.ErrObservationCanceled) {
				t.Fatalf("canceled selected read = %v", err)
			}
		})
	}
}

func TestScopedWorkListUsesPreparedFactsWithoutWorldProjector(t *testing.T) {
	t.Parallel()
	ledger := &selectedWorkFactsLedger{facts: recordings.WorkerSessionWorkFacts{KnownWork: true}}
	s := &recordedWorkerSessionObservation{ledger: ledger}
	result, err := s.ListObservations(t.Context(), workersessions.ListObservationsRequest{WorkID: "empty"})
	if err != nil || result.Observations == nil || len(result.Observations) != 0 || ledger.reads != 1 || ledger.workID != "empty" {
		t.Fatalf("prepared known empty Work = %+v, %v; reads=%d selected=%q", result, err, ledger.reads, ledger.workID)
	}
}

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
	prepareScopedTestFacts(service)
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
	if owner.gets != 0 || tokens != 12 {
		t.Fatalf("gets=%d tokens=%d; want no provider lookup and detached results", owner.gets, tokens)
	}
	tokens = 99
	read(99)
	if owner.gets != 0 {
		t.Fatalf("gets=%d, want no provider lookup", owner.gets)
	}
}

// Legacy scripted fixtures prepare their selected facts outside the request.
func (s *recordedWorkerSessionObservation) projectRecordedWorldState(
	ctx context.Context,
	events []interfaces.FactoryEvent,
	ordered []interfaces.FactoryEvent,
	selectedTick int,
) (interfaces.FactoryWorldState, error) {
	if restored, ok := restoredWorldStateForEvents(s.restoredWorldState, s.restoredEventPrefix, events); ok {
		return *restored, nil
	}
	if s == nil || s.projector == nil {
		return interfaces.FactoryWorldState{}, workersessions.ErrObservationProjectionUnavailable
	}
	world, err := s.projector(ordered, selectedTick)
	if err != nil {
		return interfaces.FactoryWorldState{}, workersessions.ErrObservationProjectionUnavailable
	}
	return world, nil
}

func sameFactoryEventIdentity(left, right interfaces.FactoryEvent) bool {
	if left.Id != "" || right.Id != "" {
		return left.Id == right.Id
	}
	return left.Type == right.Type &&
		left.Context.Tick == right.Context.Tick &&
		left.Context.Sequence == right.Context.Sequence &&
		left.Context.EventTime.Equal(right.Context.EventTime)
}

func factoryEventRequiresWorkerSessionProjection(
	state interfaces.FactoryWorldState,
	event interfaces.FactoryEvent,
) bool {
	switch event.Type {
	case interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventTypeInitialStructureRequest,
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeSessionLifecycleControl,
		interfaces.FactoryEventTypeSessionPaused,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionResumed,
		interfaces.FactoryEventTypeSessionCompleted,
		interfaces.FactoryEventTypeRunResponse:
		return false
	case interfaces.FactoryEventTypeWorkRequest:
		return !restoredWorkRequestEventIsKnown(state, event)
	default:
		return true
	}
}

func restoredWorkRequestEventIsKnown(
	state interfaces.FactoryWorldState,
	event interfaces.FactoryEvent,
) bool {
	workIDs := pointerStringSlice(event.Context.WorkIDs)
	for _, workID := range workIDs {
		if _, ok := state.WorkItemsByID[workID]; !ok {
			return false
		}
	}
	requestID := stringPointerValue(event.Context.RequestID)
	if requestID != "" {
		for key, request := range state.WorkRequestsByID {
			if key == requestID || request.RequestID == requestID {
				return len(workIDs) > 0 || len(request.WorkItems) > 0
			}
		}
		return false
	}
	return len(workIDs) > 0
}

func restoredWorldStateForEvents(
	state *interfaces.FactoryWorldState,
	prefix []interfaces.FactoryEvent,
	events []interfaces.FactoryEvent,
) (*interfaces.FactoryWorldState, bool) {
	if state == nil || len(prefix) == 0 || len(events) < len(prefix) {
		return nil, false
	}
	for index := range prefix {
		if !sameFactoryEventIdentity(prefix[index], events[index]) {
			return nil, false
		}
	}
	for _, event := range events[len(prefix):] {
		if factoryEventRequiresWorkerSessionProjection(*state, event) {
			return nil, false
		}
	}
	return state, true
}
