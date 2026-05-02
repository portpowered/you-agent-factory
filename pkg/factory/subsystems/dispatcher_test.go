package subsystems

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil/runtimefixtures"

	factory_context "github.com/portpowered/agent-factory/pkg/factory/context"
	"github.com/portpowered/agent-factory/pkg/factory/scheduler"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/workers"
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

type dispatcherRuntimeConfig = runtimefixtures.RuntimeDefinitionLookupFixture

func TestDispatcher_ThrottleFailureHistoryFromCompletedDispatches_UsesCompletionTimesAndStableLaneOrdering(t *testing.T) {
	n := &state.Net{
		Transitions: map[string]*petri.Transition{
			"t-a": {ID: "t-a", WorkerType: "worker-a"},
			"t-b": {ID: "t-b", WorkerType: "worker-b"},
		},
	}
	dispatcher := NewDispatcher(
		n,
		&mockScheduler{},
		nil,
		nil,
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {ModelProvider: "claude", Model: "claude-sonnet"},
				"worker-b": {ModelProvider: "openai", Model: "gpt-5.4"},
			},
		}),
	)
	earlier := time.Date(2026, time.May, 1, 10, 0, 0, 0, time.UTC)
	later := earlier.Add(3 * time.Minute)

	history := dispatcher.throttleFailureHistoryFromCompletedDispatches([]interfaces.CompletedDispatch{
		{
			DispatchID:   "dispatch-b",
			TransitionID: "t-b",
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
			EndTime: later,
		},
		{
			DispatchID:   "dispatch-a",
			TransitionID: "t-a",
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
			EndTime: earlier,
		},
	})

	if len(history) != 2 {
		t.Fatalf("history count = %d, want 2", len(history))
	}
	if history[0].Provider != "claude" || history[0].Model != "claude-sonnet" {
		t.Fatalf("history[0] lane = %s/%s, want claude/claude-sonnet", history[0].Provider, history[0].Model)
	}
	if !history[0].OccurredAt.Equal(earlier) {
		t.Fatalf("history[0].OccurredAt = %s, want %s", history[0].OccurredAt, earlier)
	}
	if history[0].ProviderFailure == nil || history[0].ProviderFailure.Family != interfaces.ProviderErrorFamilyThrottle || history[0].ProviderFailure.Type != interfaces.ProviderErrorTypeThrottled {
		t.Fatalf("history[0].ProviderFailure = %#v, want preserved throttle metadata", history[0].ProviderFailure)
	}
	if history[1].Provider != "openai" || history[1].Model != "gpt-5.4" {
		t.Fatalf("history[1] lane = %s/%s, want openai/gpt-5.4", history[1].Provider, history[1].Model)
	}
	if !history[1].OccurredAt.Equal(later) {
		t.Fatalf("history[1].OccurredAt = %s, want %s", history[1].OccurredAt, later)
	}
}

func TestDispatcher_ThrottleFailureHistoryFromCompletedDispatches_IgnoresNonThrottleAndUnresolvedDispatches(t *testing.T) {
	n := &state.Net{
		Transitions: map[string]*petri.Transition{
			"t-a": {ID: "t-a", WorkerType: "worker-a"},
		},
	}
	dispatcher := NewDispatcher(
		n,
		&mockScheduler{},
		nil,
		nil,
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {ModelProvider: "claude", Model: "claude-sonnet"},
			},
		}),
	)
	endTime := time.Date(2026, time.May, 1, 10, 0, 0, 0, time.UTC)

	history := dispatcher.throttleFailureHistoryFromCompletedDispatches([]interfaces.CompletedDispatch{
		{
			DispatchID:   "retryable",
			TransitionID: "t-a",
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyRetryable,
				Type:   interfaces.ProviderErrorTypeInternalServerError,
			},
			EndTime: endTime,
		},
		{
			DispatchID:   "unknown-transition",
			TransitionID: "t-missing",
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
			EndTime: endTime.Add(time.Minute),
		},
	})

	if len(history) != 0 {
		t.Fatalf("history = %#v, want empty after filtering non-throttle and unresolved dispatches", history)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-dispatcher-fixture review=2026-07-18 removal=split-single-transition-fixture-before-next-dispatcher-change
func TestDispatcher_SingleTransitionFires(t *testing.T) {
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

	wfCtx := &factory_context.FactoryContext{FactoryDirectory: "wf-1", WorkDirectory: "/tmp/work", ProjectID: "analytics-platform"}
	dispatcher := NewDispatcher(n, sched, wfCtx, nil)

	markingSnap := makeDispatcherSnapshot(map[string]*interfaces.Token{
		"tok1": {ID: "tok1", PlaceID: "p-init", Color: interfaces.TokenColor{RequestID: "request-1", WorkID: "w1", TraceID: "trace-1"}},
	})
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap, TickCount: 3}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should have 1 CONSUME mutation.
	if len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(result.Mutations))
	}
	m := result.Mutations[0]
	if m.Type != interfaces.MutationConsume {
		t.Errorf("expected CONSUME mutation, got %s", m.Type)
	}
	if m.TokenID != "tok1" {
		t.Errorf("expected token tok1, got %s", m.TokenID)
	}
	if m.FromPlace != "p-init" {
		t.Errorf("expected from place p-init, got %s", m.FromPlace)
	}

	// Should have 1 dispatch record.
	if len(result.Dispatches) != 1 {
		t.Fatalf("expected 1 dispatch record, got %d", len(result.Dispatches))
	}
	rec := result.Dispatches[0]
	d := rec.Dispatch
	if d.DispatchID == "" {
		t.Fatal("expected dispatcher to assign a dispatch ID")
	}
	if d.TransitionID != "t1" {
		t.Errorf("expected transition t1, got %s", d.TransitionID)
	}
	if len(d.InputTokens) != 1 || firstInputToken(d.InputTokens).ID != "tok1" {
		t.Errorf("expected 1 input token (tok1), got %v", d.InputTokens)
	}
	if firstInputToken(d.InputTokens).CreatedAt.IsZero() {
		t.Error("expected consumed token snapshot to preserve CreatedAt")
	}
	if firstInputToken(d.InputTokens).Color.WorkID != "w1" {
		t.Errorf("expected consumed token snapshot to preserve WorkID w1, got %s", firstInputToken(d.InputTokens).Color.WorkID)
	}
	if d.WorkstationName != "do-work" {
		t.Errorf("expected workstation name do-work, got %s", d.WorkstationName)
	}
	if d.ProjectID != "analytics-platform" {
		t.Errorf("expected project ID analytics-platform, got %q", d.ProjectID)
	}
	if d.Execution.CurrentTick != 3 {
		t.Errorf("expected execution current tick 3, got %d", d.Execution.CurrentTick)
	}
	if d.Execution.RequestID != "request-1" {
		t.Errorf("expected execution request ID request-1, got %q", d.Execution.RequestID)
	}
	if d.Execution.TraceID != "trace-1" {
		t.Errorf("expected execution trace ID trace-1, got %q", d.Execution.TraceID)
	}
	if strings.Join(d.Execution.WorkIDs, ",") != "w1" {
		t.Errorf("expected execution work IDs [w1], got %#v", d.Execution.WorkIDs)
	}
	if d.Execution.ReplayKey != "t1/trace-1/w1" {
		t.Errorf("expected replay key t1/trace-1/w1, got %q", d.Execution.ReplayKey)
	}
	// Verify mutations are paired with the dispatch.
	if len(rec.Mutations) != 1 {
		t.Fatalf("expected 1 consume mutation in dispatch record, got %d", len(rec.Mutations))
	}
	if rec.Mutations[0].Type != interfaces.MutationConsume || rec.Mutations[0].TokenID != "tok1" {
		t.Errorf("expected CONSUME mutation for tok1, got %v", rec.Mutations[0])
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
func TestDispatcher_ThrottledResultPausesMatchingProviderModelLane(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init-a": {ID: "p-init-a"},
			"p-init-b": {ID: "p-init-b"},
			"p-done":   {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"t-a": {
				ID:         "t-a",
				Name:       "step-a",
				WorkerType: "worker-a",
				InputArcs: []petri.Arc{
					{ID: "a-in-a", Name: "work", PlaceID: "p-init-a", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a-out-a", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
			"t-b": {
				ID:         "t-b",
				Name:       "step-b",
				WorkerType: "worker-b",
				InputArcs: []petri.Arc{
					{ID: "a-in-b", Name: "work", PlaceID: "p-init-b", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a-out-b", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}
	sched := &recordingScheduler{}
	now := time.Date(2026, time.April, 8, 11, 0, 0, 0, time.UTC)
	dispatcher := NewDispatcher(
		n,
		sched,
		nil,
		nil,
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {ModelProvider: "claude", Model: "claude-sonnet"},
				"worker-b": {ModelProvider: "codex", Model: "gpt-5-codex"},
			},
		}),
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherThrottlePauseDuration(30*time.Minute),
	)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init-a"},
			"tok-b": {ID: "tok-b", PlaceID: "p-init-b"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle", "t-a", now),
		},
	}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected dispatch result")
	}
	if len(sched.received) != 1 {
		t.Fatalf("expected scheduler to receive only the healthy lane, got %d enabled transitions", len(sched.received))
	}
	if sched.received[0].TransitionID != "t-b" {
		t.Fatalf("expected scheduler to receive only healthy transition t-b, got %s", sched.received[0].TransitionID)
	}
	if len(result.Dispatches) != 1 {
		t.Fatalf("expected 1 dispatch after pause filtering, got %d", len(result.Dispatches))
	}
	if result.Dispatches[0].Dispatch.TransitionID != "t-b" {
		t.Fatalf("expected unrelated lane t-b to dispatch, got %s", result.Dispatches[0].Dispatch.TransitionID)
	}
	if !result.ThrottlePausesObserved {
		t.Fatal("expected dispatcher to report observed throttle pauses")
	}
	pause := assertSingleActiveThrottlePause(t, result, "claude", "claude-sonnet", "claude/claude-sonnet")
	assertThrottlePauseWindow(t, pause, now, now.Add(30*time.Minute))
}

// portos:func-length-exception owner=agent-factory reason=legacy-throttle-fixture review=2026-07-18 removal=split-pause-expiry-fixture-before-next-dispatcher-throttle-change
func TestDispatcher_ThrottlePauseExpiresAndAllowsDispatchAgain(t *testing.T) {
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
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a-out", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}
	sched := &mockScheduler{
		decisions: []interfaces.FiringDecision{
			{TransitionID: "t-a", ConsumeTokens: []string{"tok-a"}, WorkerType: "worker-a"},
		},
	}
	currentTime := time.Date(2026, time.April, 8, 11, 0, 0, 0, time.UTC)
	dispatcher := NewDispatcher(
		n,
		sched,
		nil,
		nil,
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {ModelProvider: "claude", Model: "claude-sonnet"},
			},
		}),
		WithDispatcherClock(func() time.Time { return currentTime }),
		WithDispatcherThrottlePauseDuration(10*time.Minute),
	)

	pausedSnapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle", "t-a", currentTime),
		},
	}

	result, err := dispatcher.Execute(context.Background(), &pausedSnapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected throttle pause snapshot while lane is paused")
	}
	if len(result.Dispatches) != 0 {
		t.Fatalf("expected no dispatch while lane is paused, got %+v", result.Dispatches)
	}
	firstPause := assertSingleActiveThrottlePause(t, result, "claude", "claude-sonnet", "claude/claude-sonnet")

	currentTime = currentTime.Add(11 * time.Minute)
	resumedSnapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle", "t-a", firstPause.PausedAt),
		},
		ActiveThrottlePauses: append([]interfaces.ActiveThrottlePause(nil), result.ActiveThrottlePauses...),
	}

	result, err = dispatcher.Execute(context.Background(), &resumedSnapshot)
	if err != nil {
		t.Fatalf("unexpected error after expiry: %v", err)
	}
	if result == nil || len(result.Dispatches) != 1 {
		t.Fatalf("expected paused lane to dispatch after expiry, got %+v", result)
	}
	if !result.ThrottlePausesObserved {
		t.Fatal("expected dispatcher to report expired throttle pause reconciliation")
	}
	if len(result.ActiveThrottlePauses) != 0 {
		t.Fatalf("active throttle pauses after expiry = %d, want 0", len(result.ActiveThrottlePauses))
	}
}

func TestDispatcher_OverlappingThrottleFailuresExtendPauseWithoutResettingPausedAt(t *testing.T) {
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
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a-out", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}
	sched := &mockScheduler{}
	currentTime := time.Date(2026, time.April, 8, 11, 0, 0, 0, time.UTC)
	dispatcher := NewDispatcher(
		n,
		sched,
		nil,
		nil,
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {ModelProvider: "claude", Model: "claude-sonnet"},
			},
		}),
		WithDispatcherClock(func() time.Time { return currentTime }),
		WithDispatcherThrottlePauseDuration(10*time.Minute),
	)

	firstFailure := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle-1", "t-a", currentTime),
		},
	}

	result, err := dispatcher.Execute(context.Background(), &firstFailure)
	if err != nil {
		t.Fatalf("unexpected error after first failure: %v", err)
	}
	firstPause := assertSingleActiveThrottlePause(t, result, "claude", "claude-sonnet", "claude/claude-sonnet")
	assertThrottlePauseWindow(t, firstPause, currentTime, currentTime.Add(10*time.Minute))

	currentTime = currentTime.Add(4 * time.Minute)
	secondFailure := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle-1", "t-a", firstPause.PausedAt),
			throttledCompletedDispatch("d-throttle-2", "t-a", currentTime),
		},
	}

	result, err = dispatcher.Execute(context.Background(), &secondFailure)
	if err != nil {
		t.Fatalf("unexpected error after overlapping failure: %v", err)
	}
	secondPause := assertSingleActiveThrottlePause(t, result, "claude", "claude-sonnet", "claude/claude-sonnet")
	assertThrottlePauseWindow(t, secondPause, firstPause.PausedAt, currentTime.Add(10*time.Minute))
}

func TestDispatcher_ThrottlePauseObservedWhenCronTransitionPausedBeforeScheduling(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init": {ID: "p-init"},
		},
		Transitions: map[string]*petri.Transition{
			"t-cron": {
				ID:         "t-cron",
				Name:       "scheduled-work",
				WorkerType: "worker-a",
				InputArcs: []petri.Arc{
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}
	now := time.Date(2026, time.April, 8, 11, 0, 0, 0, time.UTC)
	dispatcher := NewDispatcher(
		n,
		&mockScheduler{},
		nil,
		nil,
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {ModelProvider: "claude", Model: "claude-sonnet"},
			},
		}),
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherThrottlePauseDuration(10*time.Minute),
	)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle", "t-cron", now),
		},
	}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected throttle pause snapshot result")
	}
	if !result.ThrottlePausesObserved {
		t.Fatal("expected dispatcher to report observed throttle pause from service-owned transition")
	}
	assertSingleActiveThrottlePause(t, result, "claude", "claude-sonnet", "claude/claude-sonnet")
}

func TestDispatcher_ThrottlePauseSkipsSchedulerWhenAllEnabledLanesPaused(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init": {ID: "p-init"},
		},
		Transitions: map[string]*petri.Transition{
			"t-a": {
				ID:         "t-a",
				Name:       "step-a",
				WorkerType: "worker-a",
				InputArcs: []petri.Arc{
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}
	sched := &recordingScheduler{}
	now := time.Date(2026, time.April, 8, 11, 0, 0, 0, time.UTC)
	dispatcher := NewDispatcher(
		n,
		sched,
		nil,
		nil,
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {ModelProvider: "claude", Model: "claude-sonnet"},
			},
		}),
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherThrottlePauseDuration(10*time.Minute),
	)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle", "t-a", now),
		},
	}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sched.callCount != 0 {
		t.Fatalf("expected scheduler Select to be skipped when every enabled lane is paused, got %d call(s)", sched.callCount)
	}
	if result == nil {
		t.Fatal("expected throttle pause snapshot result")
	}
	if len(result.Dispatches) != 0 {
		t.Fatalf("expected no dispatches while every enabled lane is paused, got %+v", result.Dispatches)
	}
	if !result.ThrottlePausesObserved {
		t.Fatal("expected dispatcher to report observed throttle pauses")
	}
	assertSingleActiveThrottlePause(t, result, "claude", "claude-sonnet", "claude/claude-sonnet")
}

// portos:func-length-exception owner=agent-factory reason=cron-dispatch-fixture review=2026-07-18 removal=split-cron-dispatch-fixture-before-next-cron-dispatch-change
func TestDispatcher_CronTransitionDispatchesThroughWorkerPathWithTimeToken(t *testing.T) {
	currentTime := time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC)
	n := &state.Net{
		Places: map[string]*petri.Place{
			"signal:init":                       {ID: "signal:init"},
			interfaces.SystemTimePendingPlaceID: {ID: interfaces.SystemTimePendingPlaceID},
		},
		Transitions: map[string]*petri.Transition{
			"poll-with-input": {
				ID:         "poll-with-input",
				Name:       "poll-with-input",
				WorkerType: "cron-worker",
				InputArcs: []petri.Arc{
					{
						ID:          "signal-in",
						Name:        "signal",
						PlaceID:     "signal:init",
						Direction:   petri.ArcInput,
						Mode:        interfaces.ArcModeConsume,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					},
					{
						ID:          "time-in",
						Name:        "time",
						PlaceID:     interfaces.SystemTimePendingPlaceID,
						Direction:   petri.ArcInput,
						Mode:        interfaces.ArcModeConsume,
						Guard:       &petri.CronTimeWindowGuard{Workstation: "poll-with-input"},
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					},
				},
			},
		},
	}

	dispatcher := NewDispatcher(
		n,
		scheduler.NewFIFOScheduler(),
		nil,
		nil,
		WithDispatcherClock(func() time.Time { return currentTime }),
	)
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"signal-token": {
				ID:      "signal-token",
				PlaceID: "signal:init",
				Color: interfaces.TokenColor{
					RequestID:  "request-signal",
					WorkID:     "signal-work",
					WorkTypeID: "signal",
					DataType:   interfaces.DataTypeWork,
					TraceID:    "trace-signal",
				},
			},
			"time-work": dispatcherCronTimeToken("time-work", "poll-with-input", currentTime.Add(-time.Second), currentTime.Add(time.Minute)),
		}),
		TickCount: 42,
	}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one cron dispatch", result)
	}

	dispatch := result.Dispatches[0].Dispatch
	if dispatch.WorkerType != "cron-worker" {
		t.Fatalf("worker type = %q, want cron-worker", dispatch.WorkerType)
	}
	inputTokens := workers.WorkDispatchInputTokens(dispatch)
	if len(inputTokens) != 2 {
		t.Fatalf("input token count = %d, want signal and time tokens: %#v", len(inputTokens), inputTokens)
	}
	if !dispatchHasInputWorkID(inputTokens, "signal-work") || !dispatchHasInputWorkID(inputTokens, "time-work") {
		t.Fatalf("cron dispatch inputs = %#v, want signal and time work tokens", inputTokens)
	}
	if dispatch.Execution.RequestID != "request-signal" {
		t.Fatalf("execution request ID = %q, want request-signal", dispatch.Execution.RequestID)
	}
	if dispatch.Execution.TraceID != "trace-signal" {
		t.Fatalf("execution trace ID = %q, want trace-signal", dispatch.Execution.TraceID)
	}
	if strings.Join(dispatch.Execution.WorkIDs, ",") != "signal-work" {
		t.Fatalf("execution work IDs = %#v, want only customer work signal-work", dispatch.Execution.WorkIDs)
	}
	if dispatch.Execution.ReplayKey != "poll-with-input/trace-signal/signal-work" {
		t.Fatalf("replay key = %q, want customer-work replay key", dispatch.Execution.ReplayKey)
	}
}

func TestDispatcher_ExpiredThrottlePauseObservedWhenSchedulerReturnsNoDecisions(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init": {ID: "p-init"},
		},
		Transitions: map[string]*petri.Transition{
			"t-a": {
				ID:         "t-a",
				Name:       "step-a",
				WorkerType: "worker-a",
				InputArcs: []petri.Arc{
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}
	currentTime := time.Date(2026, time.April, 8, 11, 0, 0, 0, time.UTC)
	dispatcher := NewDispatcher(
		n,
		&mockScheduler{},
		nil,
		nil,
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {ModelProvider: "claude", Model: "claude-sonnet"},
			},
		}),
		WithDispatcherClock(func() time.Time { return currentTime }),
		WithDispatcherThrottlePauseDuration(10*time.Minute),
	)

	pausedSnapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle", "t-a", currentTime),
		},
	}
	result, err := dispatcher.Execute(context.Background(), &pausedSnapshot)
	if err != nil {
		t.Fatalf("unexpected error while creating pause: %v", err)
	}
	assertSingleActiveThrottlePause(t, result, "claude", "claude-sonnet", "claude/claude-sonnet")

	currentTime = currentTime.Add(11 * time.Minute)
	expiredSnapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle", "t-a", result.ActiveThrottlePauses[0].PausedAt),
		},
		ActiveThrottlePauses: append([]interfaces.ActiveThrottlePause(nil), result.ActiveThrottlePauses...),
	}
	result, err = dispatcher.Execute(context.Background(), &expiredSnapshot)
	if err != nil {
		t.Fatalf("unexpected error after expiry: %v", err)
	}
	if result == nil {
		t.Fatal("expected throttle pause snapshot result after no-decision reconciliation")
	}
	if !result.ThrottlePausesObserved {
		t.Fatal("expected dispatcher to report expired throttle pause reconciliation")
	}
	if len(result.ActiveThrottlePauses) != 0 {
		t.Fatalf("active throttle pauses after expiry = %d, want 0", len(result.ActiveThrottlePauses))
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-throttle-resource-fixture review=2026-07-18 removal=split-throttle-resource-fixture-before-next-dispatcher-throttle-change
func TestDispatcher_ThrottlePauseExcludesPausedLaneBeforeSchedulingSharedResource(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init-a":       {ID: "p-init-a"},
			"p-init-b":       {ID: "p-init-b"},
			"slot:available": {ID: "slot:available"},
			"p-done":         {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"t-a": {
				ID:         "t-a",
				Name:       "step-a",
				WorkerType: "worker-a",
				InputArcs: []petri.Arc{
					{ID: "a-work", Name: "work", PlaceID: "p-init-a", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "a-slot", Name: "slot", PlaceID: "slot:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a-out", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
			"t-b": {
				ID:         "t-b",
				Name:       "step-b",
				WorkerType: "worker-b",
				InputArcs: []petri.Arc{
					{ID: "b-work", Name: "work", PlaceID: "p-init-b", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "b-slot", Name: "slot", PlaceID: "slot:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "b-out", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}
	sched := &recordingScheduler{}
	now := time.Date(2026, time.April, 8, 11, 0, 0, 0, time.UTC)
	dispatcher := NewDispatcher(
		n,
		sched,
		nil,
		nil,
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {ModelProvider: "claude", Model: "claude-sonnet"},
				"worker-b": {ModelProvider: "codex", Model: "gpt-5-codex"},
			},
		}),
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherThrottlePauseDuration(30*time.Minute),
	)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a":  {ID: "tok-a", PlaceID: "p-init-a"},
			"tok-b":  {ID: "tok-b", PlaceID: "p-init-b"},
			"slot-1": {ID: "slot-1", PlaceID: "slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle", "t-a", now),
		},
	}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sched.received) != 1 {
		t.Fatalf("expected scheduler to receive only the unpaused transition, got %d enabled transitions", len(sched.received))
	}
	if sched.received[0].TransitionID != "t-b" {
		t.Fatalf("expected scheduler to receive only healthy transition t-b, got %s", sched.received[0].TransitionID)
	}
	if result == nil || len(result.Dispatches) != 1 {
		t.Fatalf("expected 1 healthy dispatch, got %+v", result)
	}
	if result.Dispatches[0].Dispatch.TransitionID != "t-b" {
		t.Fatalf("expected healthy transition t-b to dispatch, got %s", result.Dispatches[0].Dispatch.TransitionID)
	}
	if !result.ThrottlePausesObserved {
		t.Fatal("expected dispatcher to keep reporting the paused lane in throttle pause observability")
	}
	assertSingleActiveThrottlePause(t, result, "claude", "claude-sonnet", "claude/claude-sonnet")
}

// portos:func-length-exception owner=agent-factory reason=legacy-dispatcher-determinism-fixture review=2026-07-18 removal=split-determinism-fixture-before-next-dispatcher-determinism-change
func TestDispatcher_RepeatedRunsProduceStableDispatchAndTokenSequences(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-work-a":         {ID: "p-work-a"},
			"p-work-b":         {ID: "p-work-b"},
			"slot-a:available": {ID: "slot-a:available"},
			"slot-b:available": {ID: "slot-b:available"},
			"p-done":           {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"transition-b": {
				ID:         "transition-b",
				Name:       "step-b",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "arc-work-b", Name: "work", PlaceID: "p-work-b", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "arc-slot-b", Name: "slot", PlaceID: "slot-b:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "arc-out-b", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
			"transition-a": {
				ID:         "transition-a",
				Name:       "step-a",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "arc-work-a", Name: "work", PlaceID: "p-work-a", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "arc-slot-a", Name: "slot", PlaceID: "slot-a:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "arc-out-a", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}

	wantDispatches := []string{"transition-a", "transition-b"}
	wantWorkTokens := []string{"tok-a", "tok-b"}
	wantResourceTokens := []string{"slot-a-1", "slot-b-1"}

	for i := 0; i < 10; i++ {
		dispatcher := NewDispatcher(n, scheduler.NewFIFOScheduler(), nil, nil)
		snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			Marking: petri.MarkingSnapshot{
				Tokens: map[string]*interfaces.Token{
					"tok-b":    {ID: "tok-b", PlaceID: "p-work-b", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
					"tok-a":    {ID: "tok-a", PlaceID: "p-work-a", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
					"slot-a-2": {ID: "slot-a-2", PlaceID: "slot-a:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
					"slot-a-1": {ID: "slot-a-1", PlaceID: "slot-a:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
					"slot-b-2": {ID: "slot-b-2", PlaceID: "slot-b:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
					"slot-b-1": {ID: "slot-b-1", PlaceID: "slot-b:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
				},
				PlaceTokens: map[string][]string{
					"p-work-b":         {"tok-b"},
					"p-work-a":         {"tok-a"},
					"slot-a:available": {"slot-a-2", "slot-a-1"},
					"slot-b:available": {"slot-b-2", "slot-b-1"},
				},
			},
		}

		result, err := dispatcher.Execute(context.Background(), &snapshot)
		if err != nil {
			t.Fatalf("iteration %d unexpected error: %v", i, err)
		}
		if result == nil {
			t.Fatalf("iteration %d expected dispatch result", i)
		}

		gotDispatches, gotWorkTokens, gotResourceTokens := dispatchSequences(result.Dispatches)
		if strings.Join(gotDispatches, ",") != strings.Join(wantDispatches, ",") {
			t.Fatalf("iteration %d dispatch sequence = %v, want %v", i, gotDispatches, wantDispatches)
		}
		if strings.Join(gotWorkTokens, ",") != strings.Join(wantWorkTokens, ",") {
			t.Fatalf("iteration %d work token sequence = %v, want %v", i, gotWorkTokens, wantWorkTokens)
		}
		if strings.Join(gotResourceTokens, ",") != strings.Join(wantResourceTokens, ",") {
			t.Fatalf("iteration %d resource token sequence = %v, want %v", i, gotResourceTokens, wantResourceTokens)
		}
	}
}

func TestDispatcher_UsesDispatcherClockForCronTimeWindowGuard(t *testing.T) {
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	dueAt := base.Add(2 * time.Minute)
	expiresAt := base.Add(7 * time.Minute)
	currentTime := dueAt.Add(-time.Nanosecond)

	n := &state.Net{
		Places: map[string]*petri.Place{
			interfaces.SystemTimePendingPlaceID: {ID: interfaces.SystemTimePendingPlaceID},
		},
		Transitions: map[string]*petri.Transition{
			"cron-refresh": {
				ID:         "cron-refresh",
				Name:       "refresh",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{
						ID:          "cron-time",
						Name:        "time",
						PlaceID:     interfaces.SystemTimePendingPlaceID,
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.CronTimeWindowGuard{Workstation: "refresh"},
					},
				},
			},
		},
	}

	dispatcher := NewDispatcher(
		n,
		scheduler.NewFIFOScheduler(),
		nil,
		nil,
		WithDispatcherClock(func() time.Time { return currentTime }),
	)
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"time-refresh": dispatcherCronTimeToken("time-refresh", "refresh", dueAt, expiresAt),
		}),
	}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error before due: %v", err)
	}
	if result != nil {
		t.Fatalf("result before due = %#v, want nil", result)
	}

	currentTime = dueAt
	result, err = dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error at due: %v", err)
	}
	if result == nil || len(result.Dispatches) != 1 {
		t.Fatalf("dispatches at due = %#v, want one dispatch", result)
	}
	if result.Dispatches[0].Dispatch.TransitionID != "cron-refresh" {
		t.Fatalf("transition id = %q, want cron-refresh", result.Dispatches[0].Dispatch.TransitionID)
	}
}

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
