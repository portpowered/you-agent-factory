package subsystems

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// mockScheduler returns pre-configured firing decisions.
type mockScheduler struct {
	decisions []interfaces.FiringDecision
}

func (m *mockScheduler) Select(_ []interfaces.EnabledTransition, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision {
	return m.decisions
}

type recordingScheduler struct {
	callCount int
	received  []interfaces.EnabledTransition
}

func (s *recordingScheduler) Select(enabled []interfaces.EnabledTransition, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision {
	s.callCount++
	s.received = append([]interfaces.EnabledTransition(nil), enabled...)
	decisions := make([]interfaces.FiringDecision, 0, len(enabled))
	claimed := make(map[string]bool)

	for _, et := range enabled {
		arcNames := make([]string, 0, len(et.Bindings))
		for arcName := range et.Bindings {
			arcNames = append(arcNames, arcName)
		}
		sort.Strings(arcNames)

		tokenIDs := make([]string, 0)
		conflict := false
		for _, arcName := range arcNames {
			tokens := et.Bindings[arcName]
			for i := range tokens {
				tokenID := tokens[i].ID
				if claimed[tokenID] {
					conflict = true
					break
				}
				if et.ArcModes[arcName] != interfaces.ArcModeObserve {
					tokenIDs = append(tokenIDs, tokenID)
				}
			}
			if conflict {
				break
			}
		}
		if conflict {
			continue
		}
		for _, tokenID := range tokenIDs {
			claimed[tokenID] = true
		}
		decisions = append(decisions, interfaces.FiringDecision{
			TransitionID:  et.TransitionID,
			ConsumeTokens: tokenIDs,
			WorkerType:    et.WorkerType,
		})
	}

	return decisions
}
func TestDispatcher_ExecuteExposesActiveThrottlePausesFromLoweredInferenceThrottleGuards(t *testing.T) {
	now := time.Date(2026, time.May, 1, 10, 0, 0, 0, time.UTC)
	n := &state.Net{
		Transitions: map[string]*petri.Transition{
			"t-a": {
				ID:         "t-a",
				WorkerType: "worker-a",
				InputArcs: []petri.Arc{{
					ID:          "a-in",
					Name:        "work",
					PlaceID:     "p-init-a",
					Direction:   petri.ArcInput,
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					Guard:       inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 30*time.Minute),
				}},
			},
			"t-b": {
				ID:         "t-b",
				WorkerType: "worker-b",
				InputArcs: []petri.Arc{{
					ID:          "b-in",
					Name:        "work",
					PlaceID:     "p-init-b",
					Direction:   petri.ArcInput,
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					Guard:       inferenceThrottleGuard("openai", "gpt-5.4", "worker-b", 30*time.Minute),
				}},
			},
		},
	}
	dispatcher := NewDispatcher(
		n,
		&mockScheduler{},
		nil,
		nil,
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig(
			interfaces.WorkerConfig{Name: "worker-a", ModelProvider: "claude", Model: "claude-sonnet"},
			interfaces.WorkerConfig{Name: "worker-b", ModelProvider: "openai", Model: "gpt-5.4"},
		)),
	)

	result, err := dispatcher.Execute(context.Background(), &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("dispatch-b", "t-b", now.Add(3*time.Minute)),
			throttledCompletedDispatch("dispatch-a", "t-a", now),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected throttle pause snapshot result")
	}

	if len(result.ActiveThrottlePauses) != 2 {
		t.Fatalf("active pause count = %d, want 2", len(result.ActiveThrottlePauses))
	}
	if !result.ThrottlePausesObserved {
		t.Fatal("expected dispatcher to report authored throttle pause observability")
	}
	if result.ActiveThrottlePauses[0].LaneID != "claude/claude-sonnet" || result.ActiveThrottlePauses[1].LaneID != "openai/gpt-5.4" {
		t.Fatalf("active pauses = %#v, want stable provider/model ordering", result.ActiveThrottlePauses)
	}
}

func TestDispatcher_ExecuteOmitsThrottlePauseObservabilityWithoutAuthoredInferenceThrottleGuard(t *testing.T) {
	now := time.Date(2026, time.May, 1, 10, 0, 0, 0, time.UTC)
	n := &state.Net{
		Transitions: map[string]*petri.Transition{
			"t-a": {
				ID: "t-a",
				InputArcs: []petri.Arc{{
					ID:          "a-in",
					Name:        "work",
					PlaceID:     "p-init-a",
					Direction:   petri.ArcInput,
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
				}},
			},
		},
	}
	dispatcher := NewDispatcher(n, &mockScheduler{}, nil, nil, WithDispatcherClock(func() time.Time { return now }))

	result, err := dispatcher.Execute(context.Background(), &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("dispatch-a", "t-a", now),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want no dispatcher output without authored pause observability", result)
	}
}

func TestDispatcher_ExecuteLeavesLaneRunnableWhenAuthoredThrottleRuntimeLookupIsUnresolved(t *testing.T) {
	now := time.Date(2026, time.May, 1, 10, 0, 0, 0, time.UTC)
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init": {ID: "p-init"},
			"p-done": {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"t-a": {
				ID:         "t-a",
				Name:       "step-a",
				WorkerType: "worker-a",
				InputArcs: []petri.Arc{
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 30*time.Minute)},
				},
				OutputArcs: []petri.Arc{
					{ID: "a-out", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}
	sched := &recordingScheduler{}
	dispatcher := NewDispatcher(
		n,
		sched,
		nil,
		nil,
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig()),
	)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("dispatch-a", "t-a", now),
		},
	}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected dispatch result")
	}
	if len(sched.received) != 1 || sched.received[0].TransitionID != "t-a" {
		t.Fatalf("scheduler received transitions = %#v, want enabled transition t-a", sched.received)
	}
	if len(result.Dispatches) != 1 || result.Dispatches[0].Dispatch.TransitionID != "t-a" {
		t.Fatalf("dispatches = %#v, want runnable transition t-a", result.Dispatches)
	}
	if result.ThrottlePausesObserved {
		t.Fatal("expected unresolved runtime lookup to avoid authored throttle pause observability")
	}
	if len(result.ActiveThrottlePauses) != 0 {
		t.Fatalf("active pauses = %#v, want none when runtime lookup is unresolved", result.ActiveThrottlePauses)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-dispatcher-fixture review=2026-07-18 removal=split-single-transition-fixture-before-next-dispatcher-change
func TestDispatcher_SingleTransitionFires(t *testing.T) {
	dispatcher, snapshot := newSingleTransitionDispatchFixture()
	result, err := dispatcher.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSingleTransitionDispatchResult(t, result)
}

func TestDispatcher_PreservesCanonicalChainingLineageWhenLegacyTraceDiffers(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init": {ID: "p-init"},
			"p-done": {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:         "t1",
				Name:       "do-work",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a2", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}

	sched := &mockScheduler{
		decisions: []interfaces.FiringDecision{
			{TransitionID: "t1", ConsumeTokens: []string{"tok1"}, WorkerType: "script"},
		},
	}

	dispatcher := NewDispatcher(n, sched, nil, nil)
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok1": {
				ID:      "tok1",
				PlaceID: "p-init",
				Color: interfaces.TokenColor{
					RequestID:              "request-1",
					WorkID:                 "w1",
					WorkTypeID:             "task",
					DataType:               interfaces.DataTypeWork,
					CurrentChainingTraceID: "chain-1",
					TraceID:                "trace-1",
				},
			},
		}),
		TickCount: 3,
	}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Dispatches) != 1 {
		t.Fatalf("dispatch result = %#v, want one dispatch", result)
	}
	dispatch := result.Dispatches[0].Dispatch
	if dispatch.CurrentChainingTraceID != "chain-1" {
		t.Fatalf("dispatch current chaining trace ID = %q, want chain-1", dispatch.CurrentChainingTraceID)
	}
	if strings.Join(dispatch.PreviousChainingTraceIDs, ",") != "chain-1" {
		t.Fatalf("dispatch previous chaining trace IDs = %#v, want [chain-1]", dispatch.PreviousChainingTraceIDs)
	}
	if dispatch.Execution.TraceID != "trace-1" {
		t.Fatalf("execution trace ID = %q, want legacy trace-1 compatibility", dispatch.Execution.TraceID)
	}
}

func TestDispatcher_MultipleDecisionsProcessInOneTick(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init-a": {ID: "p-init-a"},
			"p-init-b": {ID: "p-init-b"},
			"p-done":   {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"t-b": {
				ID:         "t-b",
				Name:       "work-b",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "b-in", Name: "work", PlaceID: "p-init-b", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "b-out", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
			"t-a": {
				ID:         "t-a",
				Name:       "work-a",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "a-in", Name: "work", PlaceID: "p-init-a", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a-out", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}

	sched := &mockScheduler{
		decisions: []interfaces.FiringDecision{
			{TransitionID: "t-b", ConsumeTokens: []string{"tok-b"}, WorkerType: "script"},
			{TransitionID: "t-a", ConsumeTokens: []string{"tok-a"}, WorkerType: "script"},
		},
	}

	dispatcher := NewDispatcher(n, sched, nil, nil)

	markingSnap := makeDispatcherSnapshot(map[string]*interfaces.Token{
		"tok-a": {ID: "tok-a", PlaceID: "p-init-a", Color: interfaces.TokenColor{WorkID: "w-a", WorkTypeID: "wt"}},
		"tok-b": {ID: "tok-b", PlaceID: "p-init-b", Color: interfaces.TokenColor{WorkID: "w-b", WorkTypeID: "wt"}},
	})
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Dispatches) != 2 {
		t.Fatalf("expected 2 dispatch records, got %d", len(result.Dispatches))
	}
	if len(result.Mutations) != 2 {
		t.Fatalf("expected 2 consume mutations, got %d", len(result.Mutations))
	}

	if result.Dispatches[0].Dispatch.TransitionID != "t-b" {
		t.Fatalf("expected first dispatch t-b, got %s", result.Dispatches[0].Dispatch.TransitionID)
	}
	if result.Dispatches[1].Dispatch.TransitionID != "t-a" {
		t.Fatalf("expected second dispatch t-a, got %s", result.Dispatches[1].Dispatch.TransitionID)
	}
}

func TestDispatcher_AllowsRepeatedTransitionWithDistinctTokensInOneTick(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init": {ID: "p-init"},
			"p-done": {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"process": {
				ID:         "process",
				Name:       "process-work",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "work-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "work-out", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}
	sched := &mockScheduler{
		decisions: []interfaces.FiringDecision{
			{TransitionID: "process", ConsumeTokens: []string{"tok-a"}, WorkerType: "script"},
			{TransitionID: "process", ConsumeTokens: []string{"tok-b"}, WorkerType: "script"},
		},
	}
	dispatcher := NewDispatcher(n, sched, nil, nil)
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
		"tok-a": {ID: "tok-a", PlaceID: "p-init", Color: interfaces.TokenColor{WorkID: "w-a", WorkTypeID: "task"}},
		"tok-b": {ID: "tok-b", PlaceID: "p-init", Color: interfaces.TokenColor{WorkID: "w-b", WorkTypeID: "task"}},
	})}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected dispatch result")
	}
	if len(result.Dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2", len(result.Dispatches))
	}
	if len(result.Mutations) != 2 {
		t.Fatalf("consume mutation count = %d, want 2", len(result.Mutations))
	}
	if result.Dispatches[0].Dispatch.TransitionID != "process" || result.Dispatches[1].Dispatch.TransitionID != "process" {
		t.Fatalf("dispatch transition IDs = %s,%s; want process,process",
			result.Dispatches[0].Dispatch.TransitionID,
			result.Dispatches[1].Dispatch.TransitionID)
	}
}

func TestDispatcher_InvalidAndDuplicateDecisionTargetsAreSkipped(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init-a": {ID: "p-init-a"},
			"p-init-b": {ID: "p-init-b"},
			"p-done":   {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"t-a": {
				ID:         "t-a",
				Name:       "work-a",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "a-in", Name: "work", PlaceID: "p-init-a", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a-out", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
			"t-b": {
				ID:         "t-b",
				Name:       "work-b",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "b-in", Name: "work", PlaceID: "p-init-b", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "b-out", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}

	sched := &mockScheduler{
		decisions: []interfaces.FiringDecision{
			{TransitionID: "t-missing", ConsumeTokens: []string{"tok-missing"}, WorkerType: "script"},
			{TransitionID: "t-a", ConsumeTokens: []string{"tok-a"}, WorkerType: "script"},
			{TransitionID: "t-b", ConsumeTokens: []string{"tok-a"}, WorkerType: "script"},
		},
	}

	dispatcher := NewDispatcher(n, sched, nil, nil)

	markingSnap := makeDispatcherSnapshot(map[string]*interfaces.Token{
		"tok-a": {ID: "tok-a", PlaceID: "p-init-a", Color: interfaces.TokenColor{WorkID: "w-a", WorkTypeID: "wt"}},
		"tok-b": {ID: "tok-b", PlaceID: "p-init-b", Color: interfaces.TokenColor{WorkID: "w-b", WorkTypeID: "wt"}},
	})
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Invalid transition should be dropped and conflicting/duplicate token claims should be skipped.
	if len(result.Dispatches) != 1 {
		t.Fatalf("expected 1 valid dispatch after filtering, got %d", len(result.Dispatches))
	}
	if result.Dispatches[0].Dispatch.TransitionID != "t-a" {
		t.Fatalf("expected t-a to dispatch, got %s", result.Dispatches[0].Dispatch.TransitionID)
	}
	if len(result.Mutations) != 1 {
		t.Fatalf("expected 1 consume mutation, got %d", len(result.Mutations))
	}
	if result.Mutations[0].TokenID != "tok-a" {
		t.Fatalf("expected consume of tok-a, got %s", result.Mutations[0].TokenID)
	}
}

func TestDispatcher_AlwaysProducesDispatches(t *testing.T) {
	// The dispatcher always produces WorkDispatches and never synthesizes
	// completion tokens directly.
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init": {ID: "p-init"},
			"p-done": {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:         "t1",
				Name:       "do-work",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a2", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}

	sched := &mockScheduler{
		decisions: []interfaces.FiringDecision{
			{TransitionID: "t1", ConsumeTokens: []string{"tok1"}, WorkerType: "script"},
		},
	}

	dispatcher := NewDispatcher(n, sched, nil, nil)

	markingSnap := makeDispatcherSnapshot(map[string]*interfaces.Token{
		"tok1": {ID: "tok1", PlaceID: "p-init", Color: interfaces.TokenColor{WorkID: "w1", WorkTypeID: "wt-code"}},
	})
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should always produce dispatch records.
	if len(result.Dispatches) != 1 {
		t.Errorf("expected 1 dispatch record, got %d", len(result.Dispatches))
	}

	// Should have exactly 1 CONSUME mutation.
	consumeCount := 0
	for _, m := range result.Mutations {
		if m.Type == interfaces.MutationConsume {
			consumeCount++
		}
	}
	if consumeCount != 1 {
		t.Errorf("expected 1 CONSUME mutation, got %d", consumeCount)
	}
}

func TestDispatcher_NoEnabledTransitions(t *testing.T) {
	n := &state.Net{
		Places:      map[string]*petri.Place{"p1": {ID: "p1"}},
		Transitions: map[string]*petri.Transition{},
	}

	sched := &mockScheduler{}
	dispatcher := NewDispatcher(n, sched, nil, nil)

	markingSnap := makeDispatcherSnapshot(map[string]*interfaces.Token{})
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result when no transitions enabled, got %+v", result)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-throttle-fixture review=2026-07-18 removal=split-throttle-fixture-before-next-dispatcher-throttle-change
func throttledCompletedDispatch(dispatchID string, transitionID string, endTime time.Time) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		Outcome:      interfaces.OutcomeFailed,
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyThrottle,
			Type:   interfaces.ProviderErrorTypeThrottled,
		},
		EndTime: endTime,
	}
}

func newSingleTransitionDispatchFixture() (*DispatcherSubsystem, *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init": {ID: "p-init"},
			"p-done": {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:         "t1",
				Name:       "do-work",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a2", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}

	dispatcher := NewDispatcher(n, &mockScheduler{
		decisions: []interfaces.FiringDecision{
			{TransitionID: "t1", ConsumeTokens: []string{"tok1"}, WorkerType: "script"},
		},
	}, &factory_context.FactoryContext{
		FactoryDirectory: "wf-1",
		WorkDirectory:    "/tmp/work",
		ProjectID:        "analytics-platform",
	}, nil)

	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok1": {ID: "tok1", PlaceID: "p-init", Color: interfaces.TokenColor{RequestID: "request-1", WorkID: "w1", TraceID: "trace-1"}},
		}),
		TickCount: 3,
	}
	return dispatcher, snapshot
}

func assertSingleTransitionDispatchResult(t *testing.T, result *interfaces.TickResult) {
	t.Helper()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(result.Mutations))
	}
	mutation := result.Mutations[0]
	if mutation.Type != interfaces.MutationConsume || mutation.TokenID != "tok1" || mutation.FromPlace != "p-init" {
		t.Fatalf("consume mutation = %#v, want tok1 from p-init", mutation)
	}
	if len(result.Dispatches) != 1 {
		t.Fatalf("expected 1 dispatch record, got %d", len(result.Dispatches))
	}

	record := result.Dispatches[0]
	dispatch := record.Dispatch
	input := firstInputToken(dispatch.InputTokens)
	if dispatch.DispatchID == "" || dispatch.TransitionID != "t1" || len(dispatch.InputTokens) != 1 || input.ID != "tok1" {
		t.Fatalf("dispatch = %#v, want single t1 dispatch for tok1", dispatch)
	}
	if input.CreatedAt.IsZero() || input.Color.WorkID != "w1" {
		t.Fatalf("dispatch input token = %#v, want preserved work token metadata", input)
	}
	if dispatch.WorkstationName != "do-work" || dispatch.ProjectID != "analytics-platform" {
		t.Fatalf("dispatch routing = %#v, want do-work in analytics-platform", dispatch)
	}
	if dispatch.Execution.CurrentTick != 3 || dispatch.Execution.RequestID != "request-1" || dispatch.Execution.TraceID != "trace-1" {
		t.Fatalf("dispatch execution = %#v, want request/trace/tick preserved", dispatch.Execution)
	}
	if strings.Join(dispatch.Execution.WorkIDs, ",") != "w1" || dispatch.Execution.ReplayKey != "t1/trace-1/w1" {
		t.Fatalf("dispatch execution IDs = %#v, want work ID w1 and stable replay key", dispatch.Execution)
	}
	if len(record.Mutations) != 1 || record.Mutations[0].Type != interfaces.MutationConsume || record.Mutations[0].TokenID != "tok1" {
		t.Fatalf("dispatch record mutations = %#v, want paired consume for tok1", record.Mutations)
	}
}

func makeDispatcherSnapshot(tokens map[string]*interfaces.Token) petri.MarkingSnapshot {
	placeTokens := make(map[string][]string)
	for id, tok := range tokens {
		if tok.CreatedAt.IsZero() {
			tok.CreatedAt = time.Now()
		}
		if tok.EnteredAt.IsZero() {
			tok.EnteredAt = time.Now()
		}
		placeTokens[tok.PlaceID] = append(placeTokens[tok.PlaceID], id)
	}
	return petri.MarkingSnapshot{
		Tokens:      tokens,
		PlaceTokens: placeTokens,
	}
}

func dispatcherCronTimeToken(id string, workstation string, dueAt time.Time, expiresAt time.Time) *interfaces.Token {
	return &interfaces.Token{
		ID:      id,
		PlaceID: interfaces.SystemTimePendingPlaceID,
		Color: interfaces.TokenColor{
			WorkID:     id,
			WorkTypeID: interfaces.SystemTimeWorkTypeID,
			DataType:   interfaces.DataTypeWork,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
				interfaces.TimeWorkTagKeyCronWorkstation: workstation,
				interfaces.TimeWorkTagKeyDueAt:           dueAt.Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.Format(time.RFC3339Nano),
			},
		},
	}
}

func dispatchHasInputWorkID(tokens []interfaces.Token, workID string) bool {
	for _, token := range tokens {
		if token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func dispatchSequences(dispatches []interfaces.DispatchRecord) ([]string, []string, []string) {
	transitionIDs := make([]string, 0, len(dispatches))
	workTokenIDs := make([]string, 0, len(dispatches))
	resourceTokenIDs := make([]string, 0, len(dispatches))

	for _, dispatch := range dispatches {
		transitionIDs = append(transitionIDs, dispatch.Dispatch.TransitionID)
		for _, token := range workers.WorkDispatchInputTokens(dispatch.Dispatch) {
			switch token.Color.DataType {
			case interfaces.DataTypeResource:
				resourceTokenIDs = append(resourceTokenIDs, token.ID)
			default:
				workTokenIDs = append(workTokenIDs, token.ID)
			}
		}
	}

	return transitionIDs, workTokenIDs, resourceTokenIDs
}

func dispatcherRuntimeConfig(workers ...interfaces.WorkerConfig) runtimefixtures.RuntimeDefinitionLookupFixture {
	lookup := runtimefixtures.RuntimeDefinitionLookupFixture{
		Workers: make(map[string]*interfaces.WorkerConfig, len(workers)),
	}
	for i := range workers {
		worker := workers[i]
		lookup.Workers[worker.Name] = &worker
	}
	return lookup
}

func inferenceThrottleGuard(provider string, model string, workerName string, refreshWindow time.Duration) *petri.InferenceThrottleGuard {
	return &petri.InferenceThrottleGuard{
		Provider:      provider,
		Model:         model,
		WorkerName:    workerName,
		RefreshWindow: refreshWindow,
	}
}

func assertSingleActiveThrottlePause(t *testing.T, result *interfaces.TickResult, provider string, model string, laneID string) interfaces.ActiveThrottlePause {
	t.Helper()
	if result == nil {
		t.Fatal("expected non-nil tick result")
	}
	if len(result.ActiveThrottlePauses) != 1 {
		t.Fatalf("active throttle pauses = %d, want 1", len(result.ActiveThrottlePauses))
	}
	pause := result.ActiveThrottlePauses[0]
	if pause.Provider != provider || pause.Model != model || pause.LaneID != laneID {
		t.Fatalf("unexpected active throttle pause lane: %#v", pause)
	}
	return pause
}

func assertThrottlePauseWindow(t *testing.T, pause interfaces.ActiveThrottlePause, pausedAt time.Time, pausedUntil time.Time) {
	t.Helper()
	if !pause.PausedAt.Equal(pausedAt) {
		t.Fatalf("PausedAt = %s, want %s", pause.PausedAt, pausedAt)
	}
	if !pause.PausedUntil.Equal(pausedUntil) {
		t.Fatalf("PausedUntil = %s, want %s", pause.PausedUntil, pausedUntil)
	}
}
