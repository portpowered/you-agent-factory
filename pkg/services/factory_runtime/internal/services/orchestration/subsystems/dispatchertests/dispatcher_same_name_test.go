package subsystems_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDispatcher_SameNameWakeDispatchesEveryEligiblePairExceptActiveAndStaleBindings(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"task:in-review":          {ID: "task:in-review"},
			"task:complete":           {ID: "task:complete"},
			"review:init":             {ID: "review:init"},
			"executor-slot:available": {ID: "executor-slot:available"},
		},
		Transitions: map[string]*petri.Transition{
			"review": {
				ID: "review", Name: "review", WorkerType: "reviewer",
				InputArcs: []petri.Arc{
					{ID: "task-in", Name: "task", PlaceID: "task:in-review", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "review-in", Name: "review", PlaceID: "review:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: &petri.SameNameGuard{MatchBinding: "task"}},
					{ID: "slot-in", Name: "slot", PlaceID: "executor-slot:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}
	tokens := sameNameDispatcherTokens()
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking:  makeDispatcherSnapshot(tokens),
		Topology: n,
		Dispatches: map[string]*interfaces.DispatchEntry{
			"active-review": {
				DispatchID: "active-review",
				ConsumedTokens: factorytoken.ToWorkerSlice([]factorytoken.Token{
					*tokens["00-active-task"], *tokens["00-active-review"], *tokens["00-active-slot"],
				}),
			},
			"completed-predecessor": {
				DispatchID: "completed-predecessor",
				ConsumedTokens: factorytoken.ToWorkerSlice([]factorytoken.Token{
					*sameNameDispatcherWorkToken("completed-task-input", "task:ready", "eligible-1", "work-eligible-1-task"),
					*sameNameDispatcherWorkToken("completed-review-input", "review:classifying", "eligible-1", "work-eligible-1-review"),
				}),
			},
		},
		Results: []workers.WorkResult{{DispatchID: "completed-predecessor"}},
	}
	dispatcher := subsystems.NewDispatcher(
		n,
		scheduler.NewWorkInQueueScheduler(50, nil),
		nil,
		nil,
		nil,
		time.Now,
		testDispatchID,
	)

	result, err := dispatcher.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("dispatcher.Execute: %v", err)
	}
	if result == nil {
		t.Fatal("same-name dispatch result = nil, want five later eligible pairs in one wake")
	}
	if got := len(result.Dispatches); got != 5 {
		t.Fatalf("same-name dispatch count = %d, want five later eligible pairs in one wake", got)
	}

	seen := make(map[string]bool)
	for _, record := range result.Dispatches {
		var taskName, reviewName string
		for _, input := range workers.WorkDispatchInputTokens(record.Dispatch) {
			if input.Color.DataType == factorytoken.DataTypeResource {
				continue
			}
			if seen[input.Color.WorkID] {
				t.Fatalf("Work %q dispatched more than once", input.Color.WorkID)
			}
			seen[input.Color.WorkID] = true
			if input.Color.WorkTypeID == "task" {
				taskName = input.Color.Name
			} else if input.Color.WorkTypeID == "review" {
				reviewName = input.Color.Name
			}
		}
		if taskName == "" || reviewName == "" || taskName != reviewName {
			t.Fatalf("dispatch input names task=%q review=%q, want one matching pair", taskName, reviewName)
		}
	}
	for _, excluded := range []string{
		"work-active-task", "work-active-review", "work-active-review-duplicate",
		"work-stale-review", "work-terminal-task", "work-terminal-review",
	} {
		if seen[excluded] {
			t.Fatalf("active, duplicate, stale, or terminal Work %q was redispatched", excluded)
		}
	}
}

func sameNameDispatcherTokens() map[string]*factorytoken.Token {
	tokens := map[string]*factorytoken.Token{
		"00-active-task":      sameNameDispatcherWorkToken("00-active-task", "task:in-review", "active", "work-active-task"),
		"00-active-review":    sameNameDispatcherWorkToken("00-active-review", "review:init", "active", "work-active-review"),
		"00-active-duplicate": sameNameDispatcherWorkToken("00-active-duplicate", "review:init", "active", "work-active-review-duplicate"),
		"00-active-slot":      sameNameDispatcherResourceToken("00-active-slot"),
		"01-stale-review":     sameNameDispatcherWorkToken("01-stale-review", "review:init", "stale", "work-stale-review"),
		"02-terminal-task":    sameNameDispatcherWorkToken("02-terminal-task", "task:complete", "terminal", "work-terminal-task"),
		"02-terminal-review":  sameNameDispatcherWorkToken("02-terminal-review", "review:init", "terminal", "work-terminal-review"),
	}
	for index := 1; index <= 5; index++ {
		name := fmt.Sprintf("eligible-%d", index)
		tokens[fmt.Sprintf("%02d-task", index+2)] = sameNameDispatcherWorkToken(fmt.Sprintf("%02d-task", index+2), "task:in-review", name, "work-"+name+"-task")
		tokens[fmt.Sprintf("%02d-review", index+2)] = sameNameDispatcherWorkToken(fmt.Sprintf("%02d-review", index+2), "review:init", name, "work-"+name+"-review")
		tokens[fmt.Sprintf("%02d-slot", index+2)] = sameNameDispatcherResourceToken(fmt.Sprintf("%02d-slot", index+2))
	}
	return tokens
}

func sameNameDispatcherWorkToken(id, placeID, name, workID string) *factorytoken.Token {
	return &factorytoken.Token{
		ID: id, PlaceID: placeID,
		Color: factorytoken.Color{
			Name: name, WorkID: workID, WorkTypeID: strings.Split(placeID, ":")[0], DataType: factorytoken.DataTypeWork,
		},
	}
}

func sameNameDispatcherResourceToken(id string) *factorytoken.Token {
	return &factorytoken.Token{
		ID: id, PlaceID: "executor-slot:available", Color: factorytoken.Color{DataType: factorytoken.DataTypeResource},
	}
}
