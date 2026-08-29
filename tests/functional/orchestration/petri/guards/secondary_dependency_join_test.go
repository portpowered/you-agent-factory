package guards

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	secondaryJoinProducerWorkID = "producer-prerequisite"
	secondaryJoinPlanWorkID     = "plan-joined"
	secondaryJoinTaskWorkID     = "task-joined"
	secondaryJoinName           = "joined-item"
	secondaryJoinProduce        = "produce-prerequisite"
	secondaryJoinTransition     = "join-items"
)

// TestDependsOnSecondaryJoinedInput proves through the injected dispatch edge
// that a SAME_NAME binding remains undispatched while a DEPENDS_ON relation on
// its secondary input is blocked, then dispatches exactly once after that
// prerequisite reaches its required terminal state. The application is built
// and executed through the same root process used by the customer CLI.
// CASE-G-003 and CASE-G-014 cover exact joined binding and public event order.
func TestDependsOnSecondaryJoinedInput(t *testing.T) {
	dir := newSharedGuardScenario(t, secondaryDependencyJoinFactoryConfig())

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "producer",
		WorkID:     secondaryJoinProducerWorkID,
		WorkTypeID: "producer",
		Payload:    []byte(`{"role":"controlled prerequisite"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       secondaryJoinName,
		WorkID:     secondaryJoinPlanWorkID,
		WorkTypeID: "plan",
		Payload:    []byte(`{"role":"primary join input"}`),
	})
	fixture := sharedGuardProcess(t)
	dispatchCountBefore := len(fixture.dispatches.Snapshot())
	gate := newSharedGuardCommandGate()
	joinedStarted := make(chan struct{})
	var joinedOnce sync.Once
	var callsMu sync.Mutex
	calls := 0
	provider := func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		callsMu.Lock()
		calls++
		call := calls
		callsMu.Unlock()
		if call == 1 {
			return gate.responder("COMPLETE")(ctx, request)
		}
		joinedOnce.Do(func() { close(joinedStarted) })
		return sharedGuardFixedProviderOutput("joined: COMPLETE")(ctx, request)
	}
	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: provider})
	waitForSharedGuardSignal(t, gate.started, "producer dispatch")
	waitForGuardFactoryEvent(t, session, factoryapi.FactoryEventTypeModelRequest, "producer model request")
	taskWorkType := "task"
	taskWorkID := secondaryJoinTaskWorkID
	targetWorkID := secondaryJoinProducerWorkID
	requiredState := "complete"
	upsertSharedGuardWorkRequest(t, session, factoryapi.WorkRequest{
		RequestId: "secondary-join-task-request",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         secondaryJoinName,
			WorkId:       &taskWorkID,
			WorkTypeName: &taskWorkType,
			Payload:      map[string]string{"role": "secondary join input"},
		}},
		Relations: &[]factoryapi.WorkRequestRelation{{
			SourceWorkName: secondaryJoinName,
			TargetWorkId:   &targetWorkID,
			RequiredState:  &requiredState,
			Type:           factoryapi.RelationTypeDependsOn,
		}},
	})
	waitForGuardFactoryEvent(t, session, factoryapi.FactoryEventTypeRelationshipChangeRequest, "joined dependency relationship")
	waitForGuardWorkState(t, session, secondaryJoinTaskWorkID, "ready")
	blocked := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, session.sessionID)
	if got := len(dispatchRequestsForTransition(t, blocked, secondaryJoinTransition)); got != 0 {
		t.Fatalf("joined dispatches before producer release = %d, want zero; dispatches=%#v", got, blocked)
	}
	callsMu.Lock()
	gotCalls := calls
	callsMu.Unlock()
	if gotCalls != 1 {
		t.Fatalf("provider command calls before producer release = %d, want one controlled producer call", gotCalls)
	}

	gate.releaseResponse()
	waitForSharedGuardSignal(t, joinedStarted, "joined dispatch")
	waitForGuardDispatchResponse(t, session, secondaryJoinTransition)
	waitForGuardWorkState(t, session, secondaryJoinTaskWorkID, "matched")

	allEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, session.sessionID)
	allDispatches := fixture.dispatches.Snapshot()[dispatchCountBefore:]
	if got := countDispatches(allDispatches, secondaryJoinTransition); got != 1 {
		t.Fatalf("joined dispatches after producer completion = %d, want exactly one; dispatches=%#v", got, allDispatches)
	}
	producer, ok := dispatchForTransition(allDispatches, secondaryJoinProduce)
	if !ok {
		t.Fatalf("missing producer dispatch in %#v", allDispatches)
	}
	joined, ok := dispatchForTransition(allDispatches, secondaryJoinTransition)
	if !ok {
		t.Fatalf("missing joined dispatch in %#v", allDispatches)
	}
	if joined.CreatedTick <= producer.CreatedTick {
		t.Fatalf("joined dispatch tick = %d, want after producer dispatch tick %d", joined.CreatedTick, producer.CreatedTick)
	}
	assertJoinedInputBinding(t, joined, secondaryJoinPlanWorkID, secondaryJoinTaskWorkID)
	joinedRequests := dispatchRequestsForTransition(t, allEvents, secondaryJoinTransition)
	if len(joinedRequests) != 1 {
		t.Fatalf("public joined dispatch requests = %d, want exactly one", len(joinedRequests))
	}
	assertJoinedInputBindingEvent(t, joinedRequests[0], secondaryJoinPlanWorkID, secondaryJoinTaskWorkID)
	assertGuardSessionEventOrdering(t, session.sessionID, allEvents, secondaryJoinProduce, secondaryJoinTransition, secondaryJoinTaskWorkID)
}

func secondaryDependencyJoinFactoryConfig() map[string]any {
	return map[string]any{
		"name": "secondary-dependency-join",
		"workTypes": []map[string]any{
			{
				"name": "producer",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
			{
				"name": "plan",
				"states": []map[string]string{
					{"name": "ready", "type": "INITIAL"},
				},
			},
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "ready", "type": "INITIAL"},
					{"name": "matched", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "producer"},
			{"name": "matcher"},
		},
		"workstations": []map[string]any{
			{
				"name":   secondaryJoinProduce,
				"worker": "producer",
				"inputs": []map[string]string{{"workType": "producer", "state": "init"}},
				"outputs": []map[string]string{{
					"workType": "producer",
					"state":    "complete",
				}},
			},
			{
				"name":   secondaryJoinTransition,
				"worker": "matcher",
				"inputs": []map[string]any{
					{"workType": "plan", "state": "ready"},
					{
						"workType": "task",
						"state":    "ready",
						"guards": []map[string]string{{
							"type":       "SAME_NAME",
							"matchInput": "plan",
						}},
					},
				},
				"outputs": []map[string]string{{
					"workType": "task",
					"state":    "matched",
				}},
			},
		},
	}
}

func countDispatches(records []recordings.FactoryDispatchRecord, transitionID string) int {
	count := 0
	for _, record := range records {
		if record.Dispatch.TransitionID == transitionID {
			count++
		}
	}
	return count
}

func dispatchForTransition(
	records []recordings.FactoryDispatchRecord,
	transitionID string,
) (recordings.FactoryDispatchRecord, bool) {
	for _, record := range records {
		if record.Dispatch.TransitionID == transitionID {
			return record, true
		}
	}
	return recordings.FactoryDispatchRecord{}, false
}

func assertJoinedInputBinding(
	t *testing.T,
	record recordings.FactoryDispatchRecord,
	planWorkID string,
	taskWorkID string,
) {
	t.Helper()
	seen := make(map[string]bool, len(record.Dispatch.Execution.WorkIDs))
	for _, workID := range record.Dispatch.Execution.WorkIDs {
		seen[workID] = true
	}
	if !seen[planWorkID] || !seen[taskWorkID] || len(seen) != 2 {
		t.Fatalf(
			"joined dispatch Work IDs = %#v, want exactly %q and %q",
			record.Dispatch.Execution.WorkIDs,
			planWorkID,
			taskWorkID,
		)
	}
}

func assertJoinedInputBindingEvent(
	t *testing.T,
	event factoryapi.FactoryEvent,
	planWorkID string,
	taskWorkID string,
) {
	t.Helper()
	payload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("decode public joined dispatch: %v", err)
	}
	seen := make(map[string]bool, len(payload.Inputs))
	for _, input := range payload.Inputs {
		seen[input.WorkId] = true
	}
	if !seen[planWorkID] || !seen[taskWorkID] || len(seen) != 2 {
		t.Fatalf("public joined dispatch inputs = %#v, want exactly %q and %q", payload.Inputs, planWorkID, taskWorkID)
	}
}

func dispatchRequestsForTransition(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	transitionID string,
) []factoryapi.FactoryEvent {
	t.Helper()
	var matches []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch request %q: %v", event.Id, err)
		}
		if payload.TransitionId == transitionID {
			matches = append(matches, event)
		}
	}
	return matches
}

func waitForSharedGuardSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	// This channel is an edge-owned completion signal from the controlled
	// provider; the deadline only bounds a missing edge or cancellation and is
	// not a delay inserted into the Factory workflow.
	select {
	case <-signal:
	case <-time.After(sharedGuardFixtureShutdownTimeout):
		t.Fatalf("timed out waiting for %s", label)
	}
}
