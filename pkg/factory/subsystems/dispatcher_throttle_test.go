package subsystems

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/petri"
)

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
					{ID: "a-in-a", Name: "work", PlaceID: "p-init-a", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 30*time.Minute)},
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
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig(
			interfaces.WorkerConfig{Name: "worker-a", ModelProvider: "claude", Model: "claude-sonnet"},
			interfaces.WorkerConfig{Name: "worker-b", ModelProvider: "openai", Model: "gpt-5.4"},
		)),
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

func TestDispatcher_ThrottleHistoryWithoutAuthoredGuardDoesNotFilterEnabledTransitions(t *testing.T) {
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
	dispatcher := NewDispatcher(n, sched, nil, nil, WithDispatcherClock(func() time.Time { return now }))

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
	received := make([]string, 0, len(sched.received))
	for _, transition := range sched.received {
		received = append(received, transition.TransitionID)
	}
	sort.Strings(received)
	if strings.Join(received, ",") != "t-a,t-b" {
		t.Fatalf("scheduler received transitions %v, want both unguarded lanes", received)
	}
	dispatched := make([]string, 0, len(result.Dispatches))
	for _, record := range result.Dispatches {
		dispatched = append(dispatched, record.Dispatch.TransitionID)
	}
	sort.Strings(dispatched)
	if strings.Join(dispatched, ",") != "t-a,t-b" {
		t.Fatalf("dispatch transitions = %v, want both unguarded lanes", dispatched)
	}
	if result.ThrottlePausesObserved {
		t.Fatal("expected no authored throttle pause observability without authored guards")
	}
	if len(result.ActiveThrottlePauses) != 0 {
		t.Fatalf("active pauses = %#v, want none without authored guards", result.ActiveThrottlePauses)
	}
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
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 10*time.Minute)},
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
		WithDispatcherClock(func() time.Time { return currentTime }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig(
			interfaces.WorkerConfig{Name: "worker-a", ModelProvider: "claude", Model: "claude-sonnet"},
		)),
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

func TestDispatcher_ThrottlePauseRemainsObservedWhileWindowStaysActive(t *testing.T) {
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
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 10*time.Minute)},
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
		WithDispatcherClock(func() time.Time { return currentTime }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig(
			interfaces.WorkerConfig{Name: "worker-a", ModelProvider: "claude", Model: "claude-sonnet"},
		)),
	)

	firstSnapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle", "t-a", currentTime),
		},
	}
	firstResult, err := dispatcher.Execute(context.Background(), &firstSnapshot)
	if err != nil {
		t.Fatalf("unexpected error while creating pause: %v", err)
	}
	firstPause := assertSingleActiveThrottlePause(t, firstResult, "claude", "claude-sonnet", "claude/claude-sonnet")

	currentTime = currentTime.Add(4 * time.Minute)
	stillPausedSnapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"tok-a": {ID: "tok-a", PlaceID: "p-init"},
		}),
		DispatchHistory: []interfaces.CompletedDispatch{
			throttledCompletedDispatch("d-throttle", "t-a", firstPause.PausedAt),
		},
		ActiveThrottlePauses: append([]interfaces.ActiveThrottlePause(nil), firstResult.ActiveThrottlePauses...),
	}

	result, err := dispatcher.Execute(context.Background(), &stillPausedSnapshot)
	if err != nil {
		t.Fatalf("unexpected error while pause remains active: %v", err)
	}
	if result == nil {
		t.Fatal("expected throttle pause snapshot while window remains active")
	}
	if !result.ThrottlePausesObserved {
		t.Fatal("expected dispatcher to keep reporting active throttle pause state")
	}
	pause := assertSingleActiveThrottlePause(t, result, "claude", "claude-sonnet", "claude/claude-sonnet")
	assertThrottlePauseWindow(t, pause, firstPause.PausedAt, firstPause.PausedUntil)
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
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 10*time.Minute)},
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
		WithDispatcherClock(func() time.Time { return currentTime }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig(
			interfaces.WorkerConfig{Name: "worker-a", ModelProvider: "claude", Model: "claude-sonnet"},
		)),
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
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 10*time.Minute)},
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
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig(
			interfaces.WorkerConfig{Name: "worker-a", ModelProvider: "claude", Model: "claude-sonnet"},
		)),
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
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 10*time.Minute)},
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
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig(
			interfaces.WorkerConfig{Name: "worker-a", ModelProvider: "claude", Model: "claude-sonnet"},
		)),
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
					{ID: "a-in", Name: "work", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 10*time.Minute)},
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
		WithDispatcherClock(func() time.Time { return currentTime }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig(
			interfaces.WorkerConfig{Name: "worker-a", ModelProvider: "claude", Model: "claude-sonnet"},
		)),
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
					{ID: "a-work", Name: "work", PlaceID: "p-init-a", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 30*time.Minute)},
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
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig(
			interfaces.WorkerConfig{Name: "worker-a", ModelProvider: "claude", Model: "claude-sonnet"},
			interfaces.WorkerConfig{Name: "worker-b", ModelProvider: "openai", Model: "gpt-5.4"},
		)),
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

func TestDispatcher_AuthoredThrottleGuard_BlocksSiblingTransitionFromRuntimeSnapshot(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init-a": {ID: "p-init-a"},
			"p-init-b": {ID: "p-init-b"},
		},
		Transitions: map[string]*petri.Transition{
			"t-a": {
				ID:         "t-a",
				Name:       "step-a",
				WorkerType: "worker-a",
				InputArcs: []petri.Arc{
					{ID: "a-in", Name: "work", PlaceID: "p-init-a", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-a", 30*time.Minute)},
				},
			},
			"t-b": {
				ID:         "t-b",
				Name:       "step-b",
				WorkerType: "worker-b",
				InputArcs: []petri.Arc{
					{ID: "b-in", Name: "work", PlaceID: "p-init-b", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: inferenceThrottleGuard("claude", "claude-sonnet", "worker-b", 30*time.Minute)},
				},
			},
		},
	}
	sched := &recordingScheduler{}
	now := time.Date(2026, time.May, 2, 5, 0, 0, 0, time.UTC)
	dispatcher := NewDispatcher(
		n,
		sched,
		nil,
		nil,
		WithDispatcherClock(func() time.Time { return now }),
		WithDispatcherRuntimeConfig(dispatcherRuntimeConfig(
			interfaces.WorkerConfig{Name: "worker-a", ModelProvider: "claude", Model: "claude-sonnet"},
			interfaces.WorkerConfig{Name: "worker-b", ModelProvider: "claude", Model: "claude-sonnet"},
		)),
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
	if len(sched.received) != 0 {
		t.Fatalf("scheduler received enabled transitions from a paused sibling lane: %#v", sched.received)
	}
	if result == nil {
		t.Fatal("expected throttle pause result when sibling transition is blocked")
	}
	if len(result.Dispatches) != 0 {
		t.Fatalf("dispatches = %#v, want none while sibling lane is throttled", result.Dispatches)
	}
	assertSingleActiveThrottlePause(t, result, "claude", "claude-sonnet", "claude/claude-sonnet")
}
