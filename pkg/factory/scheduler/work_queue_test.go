package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestWorkInQueueScheduler_BatchesMultipleIndependentTransitions(t *testing.T) {
	sched := NewWorkInQueueScheduler(3)

	tokA := interfaces.Token{ID: "tok-a", PlaceID: "p-work"}
	tokB := interfaces.Token{ID: "tok-b", PlaceID: "p-work"}
	tokC := interfaces.Token{ID: "tok-c", PlaceID: "p-work"}
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-c", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {tokC}}},
		{TransitionID: "tr-a", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {tokA}}},
		{TransitionID: "tr-b", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {tokB}}},
	}

	decisions := sched.Select(enabled, nil)
	if len(decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-a" || decisions[1].TransitionID != "tr-b" || decisions[2].TransitionID != "tr-c" {
		t.Fatalf("unexpected decision order: %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_DeterministicallyOrdersEqualRankCandidates(t *testing.T) {
	sched := NewWorkInQueueScheduler(3)

	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-gamma", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-gamma", PlaceID: "p-work", EnteredAt: baseTokenTime}}}},
		{TransitionID: "tr-alpha", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-alpha", PlaceID: "p-work", EnteredAt: baseTokenTime}}}},
		{TransitionID: "tr-beta", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-beta", PlaceID: "p-work", EnteredAt: baseTokenTime}}}},
	}

	for i := 0; i < 10; i++ {
		if got, want := strings.Join(firingDecisionIDs(sched.Select(enabled, nil)), ","), "tr-alpha,tr-beta,tr-gamma"; got != want {
			t.Fatalf("iteration %d decision order = %s, want %s", i, got, want)
		}
	}
}

func TestWorkInQueueScheduler_BoundedOutputRespectsDispatchCap(t *testing.T) {
	sched := NewWorkInQueueScheduler(2)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-1", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-1", PlaceID: "p-work"}}}},
		{TransitionID: "tr-2", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-2", PlaceID: "p-work"}}}},
		{TransitionID: "tr-3", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-3", PlaceID: "p-work"}}}},
	}

	if decisions := sched.Select(enabled, nil); len(decisions) != 2 {
		t.Fatalf("expected 2 decisions due to bound, got %d", len(decisions))
	}
}

func TestWorkInQueueScheduler_EnforcesTokenExclusivityAcrossBatch(t *testing.T) {
	sched := NewWorkInQueueScheduler(3)
	shared := interfaces.Token{ID: "shared", PlaceID: "p-work"}
	unique := interfaces.Token{ID: "unique", PlaceID: "p-work"}
	other := interfaces.Token{ID: "other", PlaceID: "p-work"}

	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-1", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {shared, unique}}},
		{TransitionID: "tr-2", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {shared}}},
		{TransitionID: "tr-3", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {other}}},
	}

	decisions := sched.Select(enabled, nil)
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions (tr-1 and tr-3), got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-1" || decisions[1].TransitionID != "tr-3" {
		t.Fatalf("unexpected exclusive scheduling order: %v", firingDecisionIDs(decisions))
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
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()}

	for i := 0; i < 10; i++ {
		if got, want := strings.Join(firingDecisionIDs(sched.Select(enabled, &snapshot)), ","), "tr-z-standard-processing,tr-a-cron-processing,tr-c-repeater-initial"; got != want {
			t.Fatalf("iteration %d decision order = %s, want %s", i, got, want)
		}
	}
}

func TestWorkInQueueScheduler_HigherPriorityCandidateClaimsSharedTokenBeforeFallbackCandidate(t *testing.T) {
	sched := NewWorkInQueueScheduler(2)
	sharedProcessing := interfaces.Token{
		ID: "tok-shared-processing", PlaceID: "task:review", EnteredAt: baseTokenTime.Add(-30 * time.Minute),
		Color: interfaces.TokenColor{WorkID: "work-shared", TraceID: "trace-shared", WorkTypeID: "task"},
	}

	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-lower-priority-shared", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {sharedProcessing}}},
		{TransitionID: "tr-z-higher-priority-shared", WorkerType: "agent", Bindings: map[string][]interfaces.Token{
			"left":  {sharedProcessing},
			"right": {{ID: "tok-processing-right", PlaceID: "task:review", Color: interfaces.TokenColor{WorkID: "work-right", TraceID: "trace-right", WorkTypeID: "task"}}},
		}},
		priorityEnabledTransition("tr-independent", "task:init", "tok-independent", baseTokenTime),
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerStatePriorityNet()}

	if got, want := strings.Join(firingDecisionIDs(sched.Select(enabled, &snapshot)), ","), "tr-z-higher-priority-shared,tr-independent"; got != want {
		t.Fatalf("expected higher-priority shared-token candidate to claim token before fallback candidate, got %s", got)
	}
}

func TestWorkInQueueScheduler_CompileTimeInterface(t *testing.T) {
	var _ Scheduler = (*WorkInQueueScheduler)(nil)
}
