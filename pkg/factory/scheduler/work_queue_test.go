package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

var baseTokenTime = time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)

func newPriorityAwareScheduler(maxDispatches int) *WorkInQueueScheduler {
	return NewWorkInQueueScheduler(maxDispatches, WithRuntimeConfig(schedulerWorkstationPriorityRuntimeConfig()))
}

func TestWorkInQueueScheduler_BatchesMultipleIndependentTransitions(t *testing.T) {
	sched := NewWorkInQueueScheduler(3)

	tokA := interfaces.Token{ID: "tok-a", PlaceID: "p-work"}
	tokB := interfaces.Token{ID: "tok-b", PlaceID: "p-work"}
	tokC := interfaces.Token{ID: "tok-c", PlaceID: "p-work"}
	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-c",
			WorkerType:   "agent",
			Bindings:     map[string][]interfaces.Token{"input": {tokC}},
		},
		{
			TransitionID: "tr-a",
			WorkerType:   "agent",
			Bindings:     map[string][]interfaces.Token{"input": {tokA}},
		},
		{
			TransitionID: "tr-b",
			WorkerType:   "agent",
			Bindings:     map[string][]interfaces.Token{"input": {tokB}},
		},
	}

	decisions := sched.Select(enabled, nil)

	if len(decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(decisions))
	}

	if decisions[0].TransitionID != "tr-a" {
		t.Fatalf("expected ordered decision 0 to be tr-a, got %q", decisions[0].TransitionID)
	}
	if decisions[1].TransitionID != "tr-b" {
		t.Fatalf("expected ordered decision 1 to be tr-b, got %q", decisions[1].TransitionID)
	}
	if decisions[2].TransitionID != "tr-c" {
		t.Fatalf("expected ordered decision 2 to be tr-c, got %q", decisions[2].TransitionID)
	}
}

func TestWorkInQueueScheduler_DeterministicallyOrdersEqualRankCandidates(t *testing.T) {
	sched := NewWorkInQueueScheduler(3)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-gamma",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{ID: "tok-gamma", PlaceID: "p-work", EnteredAt: baseTokenTime}},
			},
		},
		{
			TransitionID: "tr-alpha",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{ID: "tok-alpha", PlaceID: "p-work", EnteredAt: baseTokenTime}},
			},
		},
		{
			TransitionID: "tr-beta",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{ID: "tok-beta", PlaceID: "p-work", EnteredAt: baseTokenTime}},
			},
		},
	}

	for i := 0; i < 10; i++ {
		decisions := sched.Select(enabled, nil)
		got := firingDecisionIDs(decisions)
		want := []string{"tr-alpha", "tr-beta", "tr-gamma"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("iteration %d decision order = %v, want %v", i, got, want)
		}
	}
}

func TestWorkInQueueScheduler_BoundedOutputRespectsDispatchCap(t *testing.T) {
	sched := NewWorkInQueueScheduler(2)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-1",
			WorkerType:   "agent",
			Bindings:     map[string][]interfaces.Token{"input": {{ID: "tok-1", PlaceID: "p-work"}}},
		},
		{
			TransitionID: "tr-2",
			WorkerType:   "agent",
			Bindings:     map[string][]interfaces.Token{"input": {{ID: "tok-2", PlaceID: "p-work"}}},
		},
		{
			TransitionID: "tr-3",
			WorkerType:   "agent",
			Bindings:     map[string][]interfaces.Token{"input": {{ID: "tok-3", PlaceID: "p-work"}}},
		},
	}

	decisions := sched.Select(enabled, nil)

	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions due to bound, got %d", len(decisions))
	}
}

func TestWorkInQueueScheduler_EnforcesTokenExclusivityAcrossBatch(t *testing.T) {
	sched := NewWorkInQueueScheduler(3)

	shared := interfaces.Token{ID: "shared", PlaceID: "p-work"}
	unique := interfaces.Token{ID: "unique", PlaceID: "p-work"}
	other := interfaces.Token{ID: "other", PlaceID: "p-work"}

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-1",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {shared, unique},
			},
		},
		{
			TransitionID: "tr-2",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {shared},
			},
		},
		{
			TransitionID: "tr-3",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {other},
			},
		},
	}

	decisions := sched.Select(enabled, nil)

	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions (tr-1 and tr-3), got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-1" {
		t.Fatalf("expected first decision tr-1, got %q", decisions[0].TransitionID)
	}
	if decisions[1].TransitionID != "tr-3" {
		t.Fatalf("expected second decision tr-3, got %q", decisions[1].TransitionID)
	}
}

func TestWorkInQueueScheduler_DeterministicallyBatchesPriorityTiersBeforeFallbacks(t *testing.T) {
	sched := newPriorityAwareScheduler(3)
	enabled := []interfaces.EnabledTransition{
		priorityEnabledTransition("tr-a-cron-processing", "task:review", "tok-cron-processing", baseTokenTime.Add(-30*time.Minute)),
		priorityEnabledTransition("tr-b-cron-initial", "task:init", "tok-cron-initial", baseTokenTime.Add(-45*time.Minute)),
		priorityEnabledTransition("tr-z-standard-processing", "task:review", "tok-standard-processing", baseTokenTime),
		priorityEnabledTransition("tr-c-repeater-initial", "task:init", "tok-repeater-initial", baseTokenTime.Add(-20*time.Minute)),
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerWorkstationPriorityNet(),
	}
	want := []string{"tr-z-standard-processing", "tr-a-cron-processing", "tr-c-repeater-initial"}

	for i := 0; i < 10; i++ {
		decisions := sched.Select(enabled, &snapshot)
		got := firingDecisionIDs(decisions)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("iteration %d decision order = %v, want %v", i, got, want)
		}
	}
}

func TestWorkInQueueScheduler_HigherPriorityCandidateClaimsSharedTokenBeforeFallbackCandidate(t *testing.T) {
	sched := NewWorkInQueueScheduler(2)
	sharedProcessing := interfaces.Token{
		ID:        "tok-shared-processing",
		PlaceID:   "task:review",
		EnteredAt: baseTokenTime.Add(-30 * time.Minute),
		Color:     interfaces.TokenColor{WorkID: "work-shared", TraceID: "trace-shared", WorkTypeID: "task"},
	}

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-a-lower-priority-shared",
			WorkerType:   "agent",
			Bindings:     map[string][]interfaces.Token{"input": {sharedProcessing}},
		},
		{
			TransitionID: "tr-z-higher-priority-shared",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"left":  {sharedProcessing},
				"right": {{ID: "tok-processing-right", PlaceID: "task:review", Color: interfaces.TokenColor{WorkID: "work-right", TraceID: "trace-right", WorkTypeID: "task"}}},
			},
		},
		priorityEnabledTransition("tr-independent", "task:init", "tok-independent", baseTokenTime),
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerStatePriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	got := strings.Join(firingDecisionIDs(decisions), ",")
	want := "tr-z-higher-priority-shared,tr-independent"
	if got != want {
		t.Fatalf("expected higher-priority shared-token candidate to claim token before fallback candidate, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_CompileTimeInterface(t *testing.T) {
	var _ Scheduler = (*WorkInQueueScheduler)(nil)
}

func TestWorkInQueueScheduler_PrioritizesInitializedTraceAge(t *testing.T) {
	sched := NewWorkInQueueScheduler(2)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-new",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{ID: "tok-new", PlaceID: "p-init", Color: interfaces.TokenColor{WorkID: "work-new", TraceID: "trace-new", WorkTypeID: "task"}}},
			},
		},
		{
			TransitionID: "tr-old",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{ID: "tok-old", PlaceID: "p-init", Color: interfaces.TokenColor{WorkID: "work-old", TraceID: "trace-old", WorkTypeID: "task"}}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-old": {ID: "tok-old", PlaceID: "p-init", CreatedAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-old", TraceID: "trace-old", WorkTypeID: "task"}},
				"tok-new": {ID: "tok-new", PlaceID: "p-init", CreatedAt: baseTokenTime.Add(5 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-new", TraceID: "trace-new", WorkTypeID: "task"}},
			},
		},
		DispatchHistory: []interfaces.CompletedDispatch{
			{
				TransitionID: "tr-old-init",
				DispatchID:   "disp-old",
				ConsumedTokens: []interfaces.Token{
					{
						ID:        "legacy-old",
						PlaceID:   "p-init",
						EnteredAt: baseTokenTime.Add(-2 * time.Minute),
						Color:     interfaces.TokenColor{WorkID: "work-old", TraceID: "trace-old", WorkTypeID: "task"},
					},
				},
				StartTime: baseTokenTime.Add(-2 * time.Minute),
				EndTime:   baseTokenTime.Add(-1*time.Minute - 30*time.Second),
			},
			{
				TransitionID: "tr-new-init",
				DispatchID:   "disp-new",
				ConsumedTokens: []interfaces.Token{
					{
						ID:        "legacy-new",
						PlaceID:   "p-init",
						EnteredAt: baseTokenTime.Add(1 * time.Minute),
						Color:     interfaces.TokenColor{WorkID: "work-new", TraceID: "trace-new", WorkTypeID: "task"},
					},
				},
				StartTime: baseTokenTime.Add(1 * time.Minute),
				EndTime:   baseTokenTime.Add(1*time.Minute + 30*time.Second),
			},
		},
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-old" {
		t.Fatalf("expected initialized older trace to be prioritized first, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_PrioritizesStalledInitializedTraceAheadOfOlderUninitializedWork(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-uninitialized-old",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-uninitialized-old",
					PlaceID:   "p-work",
					EnteredAt: baseTokenTime.Add(-30 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-old", TraceID: "trace-uninitialized", WorkTypeID: "task"},
				}},
			},
		},
		{
			TransitionID: "tr-initialized-stalled",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-initialized-stalled",
					PlaceID:   "p-work",
					EnteredAt: baseTokenTime.Add(-5 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-stalled", TraceID: "trace-stalled", WorkTypeID: "task"},
				}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-uninitialized-old": {
					ID:        "tok-uninitialized-old",
					PlaceID:   "p-work",
					EnteredAt: baseTokenTime.Add(-30 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-old", TraceID: "trace-uninitialized", WorkTypeID: "task"},
				},
				"tok-initialized-stalled": {
					ID:        "tok-initialized-stalled",
					PlaceID:   "p-work",
					EnteredAt: baseTokenTime.Add(-5 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-stalled", TraceID: "trace-stalled", WorkTypeID: "task"},
				},
			},
		},
		DispatchHistory: []interfaces.CompletedDispatch{
			{
				TransitionID: "tr-stalled-init",
				DispatchID:   "disp-stalled",
				ConsumedTokens: []interfaces.Token{
					{
						ID:      "legacy-stalled",
						PlaceID: "p-init",
						Color:   interfaces.TokenColor{WorkID: "work-stalled", TraceID: "trace-stalled", WorkTypeID: "task"},
					},
				},
				StartTime: baseTokenTime.Add(-45 * time.Minute),
				EndTime:   baseTokenTime.Add(-44 * time.Minute),
			},
		},
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 bounded decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-initialized-stalled" {
		t.Fatalf("expected stalled initialized trace to be prioritized, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_PrioritizesProcessingStateAheadOfInitialState(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-initial",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-initial",
					PlaceID:   "task:init",
					EnteredAt: baseTokenTime.Add(-30 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"},
				}},
			},
		},
		{
			TransitionID: "tr-processing",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-processing",
					PlaceID:   "task:review",
					EnteredAt: baseTokenTime.Add(-5 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-processing", TraceID: "trace-processing", WorkTypeID: "task"},
				}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-initial": {
					ID:        "tok-initial",
					PlaceID:   "task:init",
					EnteredAt: baseTokenTime.Add(-30 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"},
				},
				"tok-processing": {
					ID:        "tok-processing",
					PlaceID:   "task:review",
					EnteredAt: baseTokenTime.Add(-5 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-processing", TraceID: "trace-processing", WorkTypeID: "task"},
				},
			},
		},
		Topology: schedulerStatePriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 bounded decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-processing" {
		t.Fatalf("expected processing-state work to be prioritized, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_PrioritizesMultiInputCandidateWithMoreProcessingWork(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-a-one-processing",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"first": {{
					ID:        "tok-one-processing",
					PlaceID:   "task:review",
					EnteredAt: baseTokenTime.Add(-30 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-one-processing", TraceID: "trace-one-processing", WorkTypeID: "task"},
				}},
				"second": {{
					ID:        "tok-initial",
					PlaceID:   "task:init",
					EnteredAt: baseTokenTime.Add(-30 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"},
				}},
			},
		},
		{
			TransitionID: "tr-z-two-processing",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"first": {{
					ID:        "tok-processing-left",
					PlaceID:   "task:review",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-left", TraceID: "trace-left", WorkTypeID: "task"},
				}},
				"second": {{
					ID:        "tok-processing-right",
					PlaceID:   "task:review",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-right", TraceID: "trace-right", WorkTypeID: "task"},
				}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerStatePriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 bounded decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-z-two-processing" {
		t.Fatalf("expected multi-input candidate with more processing work, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_AppliesWorkstationKindBeforeFallbackWhenProcessingCountsTie(t *testing.T) {
	sched := newPriorityAwareScheduler(1)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-a-cron",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-cron-processing",
					PlaceID:   "task:review",
					EnteredAt: baseTokenTime.Add(-30 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-cron", TraceID: "trace-cron", WorkTypeID: "task"},
				}},
			},
		},
		{
			TransitionID: "tr-z-standard",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-standard-processing",
					PlaceID:   "task:review",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-standard", TraceID: "trace-standard", WorkTypeID: "task"},
				}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerWorkstationPriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 bounded decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-z-standard" {
		t.Fatalf("expected workstation kind to run before fallback ordering, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_PrioritizesWorkerlessGuardedRouteAheadOfStandardWorkstation(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)

	sharedInit := interfaces.Token{
		ID:        "tok-shared-init",
		PlaceID:   "task:init",
		EnteredAt: baseTokenTime,
		Color:     interfaces.TokenColor{WorkID: "work-shared", TraceID: "trace-shared", WorkTypeID: "task"},
	}

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-z-standard",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {sharedInit},
			},
		},
		{
			TransitionID: "tr-a-logical",
			Bindings: map[string][]interfaces.Token{
				"input": {sharedInit},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerWorkstationPriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 bounded decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-a-logical" {
		t.Fatalf("expected workerless guarded route to outrank the standard workstation, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_DoesNotInflateProcessingCountFromResourceObserveOrDuplicateBindings(t *testing.T) {
	sched := newPriorityAwareScheduler(1)

	sharedProcessing := interfaces.Token{
		ID:        "tok-shared-processing",
		PlaceID:   "task:review",
		EnteredAt: baseTokenTime.Add(-30 * time.Minute),
		Color:     interfaces.TokenColor{WorkID: "work-shared", TraceID: "trace-shared", WorkTypeID: "task"},
	}
	observedProcessing := interfaces.Token{
		ID:        "tok-observed-processing",
		PlaceID:   "task:review",
		EnteredAt: baseTokenTime.Add(-30 * time.Minute),
		Color:     interfaces.TokenColor{WorkID: "work-observed", TraceID: "trace-observed", WorkTypeID: "task"},
	}
	resource := interfaces.Token{
		ID:      "tok-resource",
		PlaceID: "resource:slot",
		Color:   interfaces.TokenColor{DataType: interfaces.DataTypeResource},
	}

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-a-duplicate-and-observe",
			WorkerType:   "agent",
			ArcModes:     map[string]interfaces.ArcMode{"context": interfaces.ArcModeObserve},
			Bindings: map[string][]interfaces.Token{
				"context":  {observedProcessing},
				"resource": {resource},
				"first":    {sharedProcessing},
				"second":   {sharedProcessing},
			},
		},
		{
			TransitionID: "tr-z-two-distinct-processing",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"first": {{
					ID:        "tok-processing-left",
					PlaceID:   "task:review",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-left", TraceID: "trace-left", WorkTypeID: "task"},
				}},
				"second": {{
					ID:        "tok-processing-right",
					PlaceID:   "task:review",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-right", TraceID: "trace-right", WorkTypeID: "task"},
				}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerStatePriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 bounded decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-z-two-distinct-processing" {
		t.Fatalf("expected only unique consumed processing work to count, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_DoesNotInflateProcessingCountFromSystemTimeToken(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	initialWork := interfaces.Token{
		ID:        "tok-initial",
		PlaceID:   "task:init",
		EnteredAt: baseTokenTime,
		Color:     interfaces.TokenColor{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"},
	}
	timeWork := interfaces.Token{
		ID:        "tok-time",
		PlaceID:   interfaces.SystemTimePendingPlaceID,
		EnteredAt: baseTokenTime.Add(-30 * time.Minute),
		Color: interfaces.TokenColor{
			WorkID:     "time-work",
			TraceID:    "time-trace",
			WorkTypeID: interfaces.SystemTimeWorkTypeID,
			DataType:   interfaces.DataTypeWork,
		},
	}

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-a-cron",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {initialWork},
				"time":  {timeWork},
			},
		},
		{
			TransitionID: "tr-z-standard",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {initialWork},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerWorkstationPriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 bounded decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-z-standard" {
		t.Fatalf("expected internal system time token not to inflate cron priority, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_PrioritizesInitialCustomerWorkAheadOfResourceOnlyCandidate(t *testing.T) {
	sched := newPriorityAwareScheduler(1)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-a-resource",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"resource": {{
					ID:      "tok-resource",
					PlaceID: "resource:slot",
					Color:   interfaces.TokenColor{DataType: interfaces.DataTypeResource},
				}},
			},
		},
		{
			TransitionID: "tr-z-initial",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:      "tok-initial",
					PlaceID: "task:init",
					Color:   interfaces.TokenColor{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"},
				}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerStatePriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 bounded decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-z-initial" {
		t.Fatalf("expected initial customer work to outrank resource-only candidate, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_PrioritizesStandardAndRepeaterAheadOfCronAtEqualStatePriority(t *testing.T) {
	sched := newPriorityAwareScheduler(2)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-a-cron",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-cron",
					PlaceID:   "task:init",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-cron", TraceID: "trace-cron", WorkTypeID: "task"},
				}},
			},
		},
		{
			TransitionID: "tr-b-standard",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-standard",
					PlaceID:   "task:init",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-standard", TraceID: "trace-standard", WorkTypeID: "task"},
				}},
			},
		},
		{
			TransitionID: "tr-c-repeater",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-repeater",
					PlaceID:   "task:init",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-repeater", TraceID: "trace-repeater", WorkTypeID: "task"},
				}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerWorkstationPriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 2 {
		t.Fatalf("expected 2 bounded decisions, got %d", len(decisions))
	}
	got := strings.Join(firingDecisionIDs(decisions), ",")
	want := "tr-b-standard,tr-c-repeater"
	if got != want {
		t.Fatalf("expected standard and repeater to outrank cron, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_TreatsStandardAndRepeaterAsEqualKindPriority(t *testing.T) {
	sched := newPriorityAwareScheduler(1)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-z-standard",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-standard",
					PlaceID:   "task:init",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-standard", TraceID: "trace-standard", WorkTypeID: "task"},
				}},
			},
		},
		{
			TransitionID: "tr-b-repeater",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-repeater",
					PlaceID:   "task:init",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-repeater", TraceID: "trace-repeater", WorkTypeID: "task"},
				}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerWorkstationPriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 bounded decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-b-repeater" {
		t.Fatalf("expected transition ID fallback to separate standard/repeater, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_SelectsSystemTimeExpiryWhenOnlyEligibleCleanup(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: interfaces.SystemTimeExpiryTransitionID,
			Bindings: map[string][]interfaces.Token{
				"time": {{
					ID:        "tok-expired-time",
					PlaceID:   interfaces.SystemTimePendingPlaceID,
					EnteredAt: baseTokenTime,
					Color: interfaces.TokenColor{
						WorkID:     "time-work",
						TraceID:    "time-trace",
						WorkTypeID: interfaces.SystemTimeWorkTypeID,
						DataType:   interfaces.DataTypeWork,
					},
				}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerWorkstationPriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected expiry cleanup decision, got %d decisions", len(decisions))
	}
	if decisions[0].TransitionID != interfaces.SystemTimeExpiryTransitionID {
		t.Fatalf("expected expiry cleanup transition, got %q", decisions[0].TransitionID)
	}
	if got := strings.Join(decisions[0].ConsumeTokens, ","); got != "tok-expired-time" {
		t.Fatalf("expected expired time token to be consumed, got %v", decisions[0].ConsumeTokens)
	}
}

func TestWorkInQueueScheduler_PreservesQueueAgeWithinSameStatePriority(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-a-newer",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-newer",
					PlaceID:   "task:init",
					EnteredAt: baseTokenTime.Add(10 * time.Minute),
					Color:     interfaces.TokenColor{WorkID: "work-newer", TraceID: "trace-newer", WorkTypeID: "task"},
				}},
			},
		},
		{
			TransitionID: "tr-z-older",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{
					ID:        "tok-older",
					PlaceID:   "task:init",
					EnteredAt: baseTokenTime,
					Color:     interfaces.TokenColor{WorkID: "work-older", TraceID: "trace-older", WorkTypeID: "task"},
				}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Topology: schedulerStatePriorityNet(),
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 bounded decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-z-older" {
		t.Fatalf("expected queue-age fallback to prioritize older same-state work, got %q", decisions[0].TransitionID)
	}
}

func TestWorkInQueueScheduler_FiltersCompletedAndInvalidCandidates(t *testing.T) {
	sched := NewWorkInQueueScheduler(4)

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-complete",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{ID: "tok-complete", PlaceID: "p-complete", Color: interfaces.TokenColor{WorkID: "work-complete", TraceID: "trace-complete", WorkTypeID: "task"}}},
			},
		},
		{
			TransitionID: "tr-live",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{ID: "tok-live", PlaceID: "p-live", Color: interfaces.TokenColor{WorkID: "work-live", TraceID: "trace-live", WorkTypeID: "task"}}},
			},
		},
		{
			TransitionID: "tr-empty-token",
			WorkerType:   "agent",
			Bindings: map[string][]interfaces.Token{
				"input": {{ID: "", PlaceID: "p-live", Color: interfaces.TokenColor{WorkID: "work-empty", TraceID: "trace-empty", WorkTypeID: "task"}}},
			},
		},
		{
			TransitionID: "tr-observe-only",
			WorkerType:   "agent",
			ArcModes:     map[string]interfaces.ArcMode{"context": interfaces.ArcModeObserve},
			Bindings: map[string][]interfaces.Token{
				"context": {{ID: "tok-observed", PlaceID: "p-context", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}}},
			},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		DispatchHistory: []interfaces.CompletedDispatch{
			{
				TransitionID: "tr-complete",
				DispatchID:   "disp-complete",
				ConsumedTokens: []interfaces.Token{
					{ID: "live-complete", PlaceID: "p-complete", Color: interfaces.TokenColor{WorkID: "work-complete", TraceID: "trace-complete", WorkTypeID: "task"}},
				},
				StartTime: baseTokenTime.Add(-5 * time.Minute),
				EndTime:   baseTokenTime.Add(-4 * time.Minute),
			},
		},
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 1 {
		t.Fatalf("expected completed and invalid candidates to be filtered out, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-live" {
		t.Fatalf("expected tr-live to be scheduled, got %q", decisions[0].TransitionID)
	}
}

func firingDecisionIDs(decisions []interfaces.FiringDecision) []string {
	ids := make([]string, len(decisions))
	for i := range decisions {
		ids[i] = decisions[i].TransitionID
	}
	return ids
}

func schedulerStatePriorityNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init":   {ID: "task:init", TypeID: "task", State: "init"},
			"task:review": {ID: "task:review", TypeID: "task", State: "review"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "review", Category: state.StateCategoryProcessing},
				},
			},
		},
	}
}

func schedulerWorkstationPriorityNet() *state.Net {
	net := schedulerStatePriorityNet()
	net.Places[interfaces.SystemTimePendingPlaceID] = &petri.Place{
		ID:     interfaces.SystemTimePendingPlaceID,
		TypeID: interfaces.SystemTimeWorkTypeID,
		State:  interfaces.SystemTimePendingState,
	}
	net.WorkTypes[interfaces.SystemTimeWorkTypeID] = &state.WorkType{
		ID: interfaces.SystemTimeWorkTypeID,
		States: []state.StateDefinition{
			{Value: interfaces.SystemTimePendingState, Category: state.StateCategoryProcessing},
		},
	}
	net.Transitions = map[string]*petri.Transition{
		"tr-a-cron": {
			ID:         "tr-a-cron",
			WorkerType: "agent",
		},
		"tr-a-cron-processing": {
			ID:         "tr-a-cron-processing",
			WorkerType: "agent",
		},
		"tr-b-repeater": {
			ID:         "tr-b-repeater",
			WorkerType: "agent",
		},
		"tr-b-cron-initial": {
			ID:         "tr-b-cron-initial",
			WorkerType: "agent",
		},
		"tr-b-standard": {
			ID:         "tr-b-standard",
			WorkerType: "agent",
		},
		"tr-c-repeater": {
			ID:         "tr-c-repeater",
			WorkerType: "agent",
		},
		"tr-c-repeater-initial": {
			ID:         "tr-c-repeater-initial",
			WorkerType: "agent",
		},
		"tr-z-standard": {
			ID:         "tr-z-standard",
			WorkerType: "agent",
		},
		"tr-a-logical": {
			ID: "tr-a-logical",
		},
		"tr-z-standard-processing": {
			ID:         "tr-z-standard-processing",
			WorkerType: "agent",
		},
		interfaces.SystemTimeExpiryTransitionID: {
			ID:   interfaces.SystemTimeExpiryTransitionID,
			Type: petri.TransitionExhaustion,
		},
	}
	return net
}

func schedulerWorkstationPriorityRuntimeConfig() runtimefixtures.RuntimeWorkstationLookupFixture {
	return runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"tr-a-cron":                {Name: "tr-a-cron", Kind: interfaces.WorkstationKindCron},
			"tr-a-cron-processing":     {Name: "tr-a-cron-processing", Kind: interfaces.WorkstationKindCron},
			"tr-b-cron-initial":        {Name: "tr-b-cron-initial", Kind: interfaces.WorkstationKindCron},
			"tr-b-repeater":            {Name: "tr-b-repeater", Kind: interfaces.WorkstationKindRepeater},
			"tr-b-standard":            {Name: "tr-b-standard", Kind: interfaces.WorkstationKindStandard},
			"tr-c-repeater":            {Name: "tr-c-repeater", Kind: interfaces.WorkstationKindRepeater},
			"tr-c-repeater-initial":    {Name: "tr-c-repeater-initial", Kind: interfaces.WorkstationKindRepeater},
			"tr-z-standard":            {Name: "tr-z-standard", Kind: interfaces.WorkstationKindStandard},
			"tr-z-standard-processing": {Name: "tr-z-standard-processing", Kind: interfaces.WorkstationKindStandard},
		},
	}
}

func priorityEnabledTransition(transitionID, placeID, tokenID string, enteredAt time.Time) interfaces.EnabledTransition {
	return interfaces.EnabledTransition{
		TransitionID: transitionID,
		WorkerType:   "agent",
		Bindings: map[string][]interfaces.Token{
			"input": {{
				ID:        tokenID,
				PlaceID:   placeID,
				EnteredAt: enteredAt,
				Color:     interfaces.TokenColor{WorkID: "work-" + tokenID, TraceID: "trace-" + tokenID, WorkTypeID: "task"},
			}},
		},
	}
}
