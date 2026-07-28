package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestMoveWork_AcceptsValidRelocation(t *testing.T) {
	net := buildTestNet()
	marking := petri.NewMarking("test-wf")
	token := newMoveTestToken("tok-1", "work-1", "task:init")
	marking.AddToken(token)

	eng := newTestFactoryEngine(net, marking, nil)
	result, err := eng.MoveWork(context.Background(), "work-1", "complete")
	if err != nil {
		t.Fatalf("MoveWork: %v", err)
	}
	if result.FromState != "init" || result.ToState != "complete" {
		t.Fatalf("result states = %q -> %q, want init -> complete", result.FromState, result.ToState)
	}
	if result.FromPlaceID != "task:init" || result.ToPlaceID != "task:complete" {
		t.Fatalf("result places = %q -> %q, want task:init -> task:complete", result.FromPlaceID, result.ToPlaceID)
	}
	if marking.Tokens["tok-1"].PlaceID != "task:complete" {
		t.Fatalf("token place = %q, want task:complete", marking.Tokens["tok-1"].PlaceID)
	}
}

func TestMoveWork_RejectsMissingWork(t *testing.T) {
	eng := newTestFactoryEngine(buildTestNet(), petri.NewMarking("test-wf"), nil)
	_, err := eng.MoveWork(context.Background(), "missing-work", "complete")
	if !errors.Is(err, ErrMoveWorkNotFound) {
		t.Fatalf("MoveWork error = %v, want %v", err, ErrMoveWorkNotFound)
	}
}

func TestMoveWork_RejectsInvalidTargetState(t *testing.T) {
	net := buildTestNet()
	marking := petri.NewMarking("test-wf")
	marking.AddToken(newMoveTestToken("tok-1", "work-1", "task:init"))

	eng := newTestFactoryEngine(net, marking, nil)
	_, err := eng.MoveWork(context.Background(), "work-1", "nowhere")
	if !errors.Is(err, ErrMoveWorkInvalidState) {
		t.Fatalf("MoveWork error = %v, want %v", err, ErrMoveWorkInvalidState)
	}
}

func TestMoveWork_RejectsInFlightDispatch(t *testing.T) {
	net := buildTestNet()
	marking := petri.NewMarking("test-wf")
	marking.AddToken(newMoveTestToken("tok-1", "work-1", "task:init"))

	eng := newTestFactoryEngine(net, marking, nil)
	eng.runtimeState.Dispatches["dispatch-1"] = &interfaces.DispatchEntry{
		DispatchID: "dispatch-1",
		ConsumedTokens: []factorytoken.Token{{
			Color: factorytoken.Color{WorkID: "work-1", WorkTypeID: "task"},
		}},
	}

	_, err := eng.MoveWork(context.Background(), "work-1", "complete")
	if !errors.Is(err, ErrMoveWorkInFlightDispatch) {
		t.Fatalf("MoveWork error = %v, want %v", err, ErrMoveWorkInFlightDispatch)
	}
}

func TestMoveWork_RejectsTerminatedEngine(t *testing.T) {
	net := buildTestNet()
	marking := petri.NewMarking("test-wf")
	marking.AddToken(newMoveTestToken("tok-1", "work-1", "task:init"))

	eng := newTestFactoryEngine(net, marking, nil)
	eng.acceptingSubmits = false

	_, err := eng.MoveWork(context.Background(), "work-1", "complete")
	if !errors.Is(err, ErrMoveWorkEngineTerminated) {
		t.Fatalf("MoveWork error = %v, want %v", err, ErrMoveWorkEngineTerminated)
	}
}

func TestMoveWork_LeavingFailedRetainsFailureHistoryAndClearsGuardFields(t *testing.T) {
	net := buildTestNet()
	marking := petri.NewMarking("test-wf")
	token := newMoveTestToken("tok-1", "work-1", "task:failed")
	token.History = factorytoken.History{
		TotalVisits:         map[string]int{"transition-build": 3},
		ConsecutiveFailures: map[string]int{"transition-build": 2},
		PlaceVisits:         map[string]int{"task:failed": 1},
		LastError:           "provider timeout",
		FailureLog: []factorytoken.Failure{{
			TransitionID: "transition-build",
			Timestamp:    time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
			Error:        "provider timeout",
			Attempt:      2,
		}},
	}
	marking.AddToken(token)

	eng := newTestFactoryEngine(net, marking, nil)
	if _, err := eng.MoveWork(context.Background(), "work-1", "init"); err != nil {
		t.Fatalf("MoveWork: %v", err)
	}

	moved := marking.Tokens["tok-1"]
	if moved.PlaceID != "task:init" {
		t.Fatalf("token place = %q, want task:init", moved.PlaceID)
	}
	if len(moved.History.FailureLog) != 1 || moved.History.LastError != "provider timeout" {
		t.Fatalf("failure history = %#v, want retained failure log and last error", moved.History)
	}
	if len(moved.History.TotalVisits) != 0 || len(moved.History.ConsecutiveFailures) != 0 || len(moved.History.PlaceVisits) != 0 {
		t.Fatalf("guard fields = total=%#v consecutive=%#v place=%#v, want cleared maps",
			moved.History.TotalVisits, moved.History.ConsecutiveFailures, moved.History.PlaceVisits)
	}
}

func TestMoveWork_DoesNotRecordDispatchEvents(t *testing.T) {
	net := buildTestNet()
	marking := petri.NewMarking("test-wf")
	marking.AddToken(newMoveTestToken("tok-1", "work-1", "task:init"))

	dispatchCount := 0
	eng := newTestFactoryEngine(net, marking, nil, WithDispatchHandler(func(work.WorkDispatch) {
		dispatchCount++
	}))

	if _, err := eng.MoveWork(context.Background(), "work-1", "complete"); err != nil {
		t.Fatalf("MoveWork: %v", err)
	}
	if dispatchCount != 0 {
		t.Fatalf("dispatch handler calls = %d, want 0", dispatchCount)
	}
	if len(eng.runtimeState.Dispatches) != 0 {
		t.Fatalf("active dispatches = %#v, want none", eng.runtimeState.Dispatches)
	}
}

func TestMoveWork_WakesRunLoopWithoutBufferedInputs(t *testing.T) {
	net := buildTestNet()
	marking := petri.NewMarking("test-wf")
	marking.AddToken(newMoveTestToken("tok-1", "work-1", "task:failed"))

	dispatched := make(chan struct{}, 1)
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			for _, token := range snap.Marking.TokensInPlace("task:init") {
				if token.Color.WorkID != "work-1" {
					continue
				}
				return &interfaces.TickResult{
					Dispatches: []interfaces.DispatchRecord{{
						Dispatch: work.WorkDispatch{
							DispatchID:   "dispatch-after-move",
							TransitionID: "transition-after-move",
							WorkerType:   "worker-a",
						},
						Mutations: []interfaces.MarkingMutation{{
							Type:      interfaces.MutationConsume,
							TokenID:   token.ID,
							FromPlace: "task:init",
						}},
					}},
				}, nil
			}
			return nil, nil
		},
	}
	eng := newTestFactoryEngine(net, marking, []subsystems.Subsystem{dispatchSub}, WithDispatchHandler(func(dispatch work.WorkDispatch) {
		if dispatch.DispatchID == "dispatch-after-move" {
			select {
			case dispatched <- struct{}{}:
			default:
			}
		}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() {
		runErr <- eng.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	if _, err := eng.MoveWork(context.Background(), "work-1", "init"); err != nil {
		t.Fatalf("MoveWork: %v", err)
	}

	select {
	case <-dispatched:
	case err := <-runErr:
		t.Fatalf("Run returned before moved work redispatched: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for moved work to redispatch")
	}
}

func newMoveTestToken(tokenID, workID, placeID string) *factorytoken.Token {
	return &factorytoken.Token{
		ID:        tokenID,
		PlaceID:   placeID,
		CreatedAt: time.Now(),
		EnteredAt: time.Now(),
		Color: factorytoken.Color{
			WorkID:     workID,
			WorkTypeID: "task",
		},
		History: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}
}

func TestClearGuardBlockingFields_PreservesFailureHistory(t *testing.T) {
	history := factorytoken.History{
		TotalVisits:         map[string]int{"t1": 1},
		ConsecutiveFailures: map[string]int{"t1": 2},
		PlaceVisits:         map[string]int{"task:failed": 1},
		LastError:           "boom",
		FailureLog: []factorytoken.Failure{{
			Error: "boom",
		}},
	}
	factorytoken.ClearGuardBlockingFields(&history)
	if history.LastError != "boom" || len(history.FailureLog) != 1 {
		t.Fatalf("failure history = %#v, want preserved", history)
	}
	if len(history.TotalVisits) != 0 || len(history.ConsecutiveFailures) != 0 || len(history.PlaceVisits) != 0 {
		t.Fatalf("guard fields not cleared: %#v", history)
	}
}

func TestCategoryForState_FailedPlace(t *testing.T) {
	net := buildTestNet()
	if got := state.CategoryForState(net.WorkTypes, "task", "failed"); got != state.StateCategoryFailed {
		t.Fatalf("category = %q, want FAILED", got)
	}
}
