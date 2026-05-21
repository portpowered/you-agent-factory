package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestWorkInQueueScheduler_PrioritizesInitializedTraceAge(t *testing.T) {
	sched := NewWorkInQueueScheduler(2)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-new", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-new", PlaceID: "p-init", Color: interfaces.TokenColor{WorkID: "work-new", TraceID: "trace-new", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-old", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-old", PlaceID: "p-init", Color: interfaces.TokenColor{WorkID: "work-old", TraceID: "trace-old", WorkTypeID: "task"}}}}},
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-old": {ID: "tok-old", PlaceID: "p-init", CreatedAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-old", TraceID: "trace-old", WorkTypeID: "task"}},
			"tok-new": {ID: "tok-new", PlaceID: "p-init", CreatedAt: baseTokenTime.Add(5 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-new", TraceID: "trace-new", WorkTypeID: "task"}},
		}},
		DispatchHistory: []interfaces.CompletedDispatch{
			{TransitionID: "tr-old-init", DispatchID: "disp-old", ConsumedTokens: []interfaces.Token{{ID: "legacy-old", PlaceID: "p-init", EnteredAt: baseTokenTime.Add(-2 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-old", TraceID: "trace-old", WorkTypeID: "task"}}}, StartTime: baseTokenTime.Add(-2 * time.Minute), EndTime: baseTokenTime.Add(-1*time.Minute - 30*time.Second)},
			{TransitionID: "tr-new-init", DispatchID: "disp-new", ConsumedTokens: []interfaces.Token{{ID: "legacy-new", PlaceID: "p-init", EnteredAt: baseTokenTime.Add(1 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-new", TraceID: "trace-new", WorkTypeID: "task"}}}, StartTime: baseTokenTime.Add(1 * time.Minute), EndTime: baseTokenTime.Add(1*time.Minute + 30*time.Second)},
		},
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 2 || decisions[0].TransitionID != "tr-old" {
		t.Fatalf("expected initialized older trace first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesStalledInitializedTraceAheadOfOlderUninitializedWork(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-uninitialized-old", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-uninitialized-old", PlaceID: "p-work", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-old", TraceID: "trace-uninitialized", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-initialized-stalled", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-initialized-stalled", PlaceID: "p-work", EnteredAt: baseTokenTime.Add(-5 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-stalled", TraceID: "trace-stalled", WorkTypeID: "task"}}}}},
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-uninitialized-old":   {ID: "tok-uninitialized-old", PlaceID: "p-work", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-old", TraceID: "trace-uninitialized", WorkTypeID: "task"}},
			"tok-initialized-stalled": {ID: "tok-initialized-stalled", PlaceID: "p-work", EnteredAt: baseTokenTime.Add(-5 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-stalled", TraceID: "trace-stalled", WorkTypeID: "task"}},
		}},
		DispatchHistory: []interfaces.CompletedDispatch{{TransitionID: "tr-stalled-init", DispatchID: "disp-stalled", ConsumedTokens: []interfaces.Token{{ID: "legacy-stalled", PlaceID: "p-init", Color: interfaces.TokenColor{WorkID: "work-stalled", TraceID: "trace-stalled", WorkTypeID: "task"}}}, StartTime: baseTokenTime.Add(-45 * time.Minute), EndTime: baseTokenTime.Add(-44 * time.Minute)}},
	}

	if decisions := sched.Select(enabled, &snapshot); len(decisions) != 1 || decisions[0].TransitionID != "tr-initialized-stalled" {
		t.Fatalf("expected stalled initialized trace to win, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesProcessingStateAheadOfInitialState(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-initial", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-initial", PlaceID: "task:init", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-processing", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-processing", PlaceID: "task:review", EnteredAt: baseTokenTime.Add(-5 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-processing", TraceID: "trace-processing", WorkTypeID: "task"}}}}},
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-initial":    {ID: "tok-initial", PlaceID: "task:init", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"}},
			"tok-processing": {ID: "tok-processing", PlaceID: "task:review", EnteredAt: baseTokenTime.Add(-5 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-processing", TraceID: "trace-processing", WorkTypeID: "task"}},
		}},
		Topology: schedulerStatePriorityNet(),
	}

	if decisions := sched.Select(enabled, &snapshot); len(decisions) != 1 || decisions[0].TransitionID != "tr-processing" {
		t.Fatalf("expected processing work first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesMultiInputCandidateWithMoreProcessingWork(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-one-processing", WorkerType: "agent", Bindings: map[string][]interfaces.Token{
			"first":  {{ID: "tok-one-processing", PlaceID: "task:review", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-one-processing", TraceID: "trace-one-processing", WorkTypeID: "task"}}},
			"second": {{ID: "tok-initial", PlaceID: "task:init", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"}}},
		}},
		{TransitionID: "tr-z-two-processing", WorkerType: "agent", Bindings: map[string][]interfaces.Token{
			"first":  {{ID: "tok-processing-left", PlaceID: "task:review", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-left", TraceID: "trace-left", WorkTypeID: "task"}}},
			"second": {{ID: "tok-processing-right", PlaceID: "task:review", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-right", TraceID: "trace-right", WorkTypeID: "task"}}},
		}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerStatePriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-two-processing" {
		t.Fatalf("expected candidate with more processing work first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_AppliesWorkstationKindBeforeFallbackWhenProcessingCountsTie(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-cron", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-cron-processing", PlaceID: "task:review", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-cron", TraceID: "trace-cron", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-z-standard", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-standard-processing", PlaceID: "task:review", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-standard", TraceID: "trace-standard", WorkTypeID: "task"}}}}},
	}

	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-standard" {
		t.Fatalf("expected workstation kind priority first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesWorkerlessGuardedRouteAheadOfStandardWorkstation(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)
	sharedInit := interfaces.Token{ID: "tok-shared-init", PlaceID: "task:init", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-shared", TraceID: "trace-shared", WorkTypeID: "task"}}
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-z-standard", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {sharedInit}}},
		{TransitionID: "tr-a-logical", Bindings: map[string][]interfaces.Token{"input": {sharedInit}}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-a-logical" {
		t.Fatalf("expected logical route first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_DoesNotInflateProcessingCountFromResourceObserveOrDuplicateBindings(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	sharedProcessing := interfaces.Token{ID: "tok-shared-processing", PlaceID: "task:review", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-shared", TraceID: "trace-shared", WorkTypeID: "task"}}
	observedProcessing := interfaces.Token{ID: "tok-observed-processing", PlaceID: "task:review", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-observed", TraceID: "trace-observed", WorkTypeID: "task"}}
	resource := interfaces.Token{ID: "tok-resource", PlaceID: "resource:slot", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}}

	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-duplicate-and-observe", WorkerType: "agent", ArcModes: map[string]interfaces.ArcMode{"context": interfaces.ArcModeObserve}, Bindings: map[string][]interfaces.Token{"context": {observedProcessing}, "resource": {resource}, "first": {sharedProcessing}, "second": {sharedProcessing}}},
		{TransitionID: "tr-z-two-distinct-processing", WorkerType: "agent", Bindings: map[string][]interfaces.Token{
			"first":  {{ID: "tok-processing-left", PlaceID: "task:review", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-left", TraceID: "trace-left", WorkTypeID: "task"}}},
			"second": {{ID: "tok-processing-right", PlaceID: "task:review", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-right", TraceID: "trace-right", WorkTypeID: "task"}}},
		}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerStatePriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-two-distinct-processing" {
		t.Fatalf("expected unique consumed processing work to count, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_DoesNotInflateProcessingCountFromSystemTimeToken(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	initialWork := interfaces.Token{ID: "tok-initial", PlaceID: "task:init", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"}}
	timeWork := interfaces.Token{ID: "tok-time", PlaceID: interfaces.SystemTimePendingPlaceID, EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: interfaces.TokenColor{WorkID: "time-work", TraceID: "time-trace", WorkTypeID: interfaces.SystemTimeWorkTypeID, DataType: interfaces.DataTypeWork}}
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-cron", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {initialWork}, "time": {timeWork}}},
		{TransitionID: "tr-z-standard", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {initialWork}}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-standard" {
		t.Fatalf("expected system time token not to inflate cron priority, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesInitialCustomerWorkAheadOfResourceOnlyCandidate(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-resource", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"resource": {{ID: "tok-resource", PlaceID: "resource:slot", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}}}}},
		{TransitionID: "tr-z-initial", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-initial", PlaceID: "task:init", Color: interfaces.TokenColor{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"}}}}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerStatePriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-initial" {
		t.Fatalf("expected initial customer work to outrank resource-only candidate, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesStandardAndRepeaterAheadOfCronAtEqualStatePriority(t *testing.T) {
	sched := newPriorityAwareScheduler(2)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-cron", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-cron", PlaceID: "task:init", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-cron", TraceID: "trace-cron", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-b-standard", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-standard", PlaceID: "task:init", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-standard", TraceID: "trace-standard", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-c-repeater", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-repeater", PlaceID: "task:init", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-repeater", TraceID: "trace-repeater", WorkTypeID: "task"}}}}},
	}
	decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()})
	if got, want := strings.Join(firingDecisionIDs(decisions), ","), "tr-b-standard,tr-c-repeater"; len(decisions) != 2 || got != want {
		t.Fatalf("expected standard and repeater before cron, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_TreatsStandardAndRepeaterAsEqualKindPriority(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-z-standard", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-standard", PlaceID: "task:init", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-standard", TraceID: "trace-standard", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-b-repeater", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-repeater", PlaceID: "task:init", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-repeater", TraceID: "trace-repeater", WorkTypeID: "task"}}}}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-b-repeater" {
		t.Fatalf("expected transition ID fallback to separate standard/repeater, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_SelectsSystemTimeExpiryWhenOnlyEligibleCleanup(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)
	enabled := []interfaces.EnabledTransition{{TransitionID: interfaces.SystemTimeExpiryTransitionID, Bindings: map[string][]interfaces.Token{"time": {{ID: "tok-expired-time", PlaceID: interfaces.SystemTimePendingPlaceID, EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "time-work", TraceID: "time-trace", WorkTypeID: interfaces.SystemTimeWorkTypeID, DataType: interfaces.DataTypeWork}}}}}}
	decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()})
	if len(decisions) != 1 || decisions[0].TransitionID != interfaces.SystemTimeExpiryTransitionID || strings.Join(decisions[0].ConsumeTokens, ",") != "tok-expired-time" {
		t.Fatalf("expected expiry cleanup transition, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PreservesQueueAgeWithinSameStatePriority(t *testing.T) {
	sched := NewWorkInQueueScheduler(1)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-newer", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-newer", PlaceID: "task:init", EnteredAt: baseTokenTime.Add(10 * time.Minute), Color: interfaces.TokenColor{WorkID: "work-newer", TraceID: "trace-newer", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-z-older", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-older", PlaceID: "task:init", EnteredAt: baseTokenTime, Color: interfaces.TokenColor{WorkID: "work-older", TraceID: "trace-older", WorkTypeID: "task"}}}}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerStatePriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-older" {
		t.Fatalf("expected older same-state work first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_FiltersCompletedAndInvalidCandidates(t *testing.T) {
	sched := NewWorkInQueueScheduler(4)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-complete", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-complete", PlaceID: "p-complete", Color: interfaces.TokenColor{WorkID: "work-complete", TraceID: "trace-complete", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-live", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "tok-live", PlaceID: "p-live", Color: interfaces.TokenColor{WorkID: "work-live", TraceID: "trace-live", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-empty-token", WorkerType: "agent", Bindings: map[string][]interfaces.Token{"input": {{ID: "", PlaceID: "p-live", Color: interfaces.TokenColor{WorkID: "work-empty", TraceID: "trace-empty", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-observe-only", WorkerType: "agent", ArcModes: map[string]interfaces.ArcMode{"context": interfaces.ArcModeObserve}, Bindings: map[string][]interfaces.Token{"context": {{ID: "tok-observed", PlaceID: "p-context", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}}}}},
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		DispatchHistory: []interfaces.CompletedDispatch{{TransitionID: "tr-complete", DispatchID: "disp-complete", ConsumedTokens: []interfaces.Token{{ID: "live-complete", PlaceID: "p-complete", Color: interfaces.TokenColor{WorkID: "work-complete", TraceID: "trace-complete", WorkTypeID: "task"}}}, StartTime: baseTokenTime.Add(-5 * time.Minute), EndTime: baseTokenTime.Add(-4 * time.Minute)}},
	}
	if decisions := sched.Select(enabled, &snapshot); len(decisions) != 1 || decisions[0].TransitionID != "tr-live" {
		t.Fatalf("expected completed and invalid candidates filtered out, got %v", firingDecisionIDs(decisions))
	}
}
