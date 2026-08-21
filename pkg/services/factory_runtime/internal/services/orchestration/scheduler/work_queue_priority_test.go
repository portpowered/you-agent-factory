package scheduler

import (
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestWorkInQueueScheduler_PrioritizesInitializedTraceAge(t *testing.T) {
	sched := NewWorkInQueueScheduler(2, nil)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-new", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-new", State: "p-init", Color: workerexecution.Color{WorkID: "work-new", TraceID: "trace-new", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-old", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-old", State: "p-init", Color: workerexecution.Color{WorkID: "work-old", TraceID: "trace-old", WorkTypeID: "task"}}}}},
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			"tok-old": {ID: "tok-old", PlaceID: "p-init", CreatedAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-old", TraceID: "trace-old", WorkTypeID: "task"}},
			"tok-new": {ID: "tok-new", PlaceID: "p-init", CreatedAt: baseTokenTime.Add(5 * time.Minute), Color: workerexecution.Color{WorkID: "work-new", TraceID: "trace-new", WorkTypeID: "task"}},
		}},
		DispatchHistory: []interfaces.CompletedDispatch{
			{TransitionID: "tr-old-init", DispatchID: "disp-old", ConsumedTokens: []workerexecution.Token{{ID: "legacy-old", State: "p-init", EnteredAt: baseTokenTime.Add(-2 * time.Minute), Color: workerexecution.Color{WorkID: "work-old", TraceID: "trace-old", WorkTypeID: "task"}}}, StartTime: baseTokenTime.Add(-2 * time.Minute), EndTime: baseTokenTime.Add(-1*time.Minute - 30*time.Second)},
			{TransitionID: "tr-new-init", DispatchID: "disp-new", ConsumedTokens: []workerexecution.Token{{ID: "legacy-new", State: "p-init", EnteredAt: baseTokenTime.Add(1 * time.Minute), Color: workerexecution.Color{WorkID: "work-new", TraceID: "trace-new", WorkTypeID: "task"}}}, StartTime: baseTokenTime.Add(1 * time.Minute), EndTime: baseTokenTime.Add(1*time.Minute + 30*time.Second)},
		},
	}

	decisions := sched.Select(enabled, &snapshot)
	if len(decisions) != 2 || decisions[0].TransitionID != "tr-old" {
		t.Fatalf("expected initialized older trace first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesStalledInitializedTraceAheadOfOlderUninitializedWork(t *testing.T) {
	sched := NewWorkInQueueScheduler(1, nil)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-uninitialized-old", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-uninitialized-old", State: "p-work", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: workerexecution.Color{WorkID: "work-old", TraceID: "trace-uninitialized", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-initialized-stalled", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-initialized-stalled", State: "p-work", EnteredAt: baseTokenTime.Add(-5 * time.Minute), Color: workerexecution.Color{WorkID: "work-stalled", TraceID: "trace-stalled", WorkTypeID: "task"}}}}},
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			"tok-uninitialized-old":   {ID: "tok-uninitialized-old", PlaceID: "p-work", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: workerexecution.Color{WorkID: "work-old", TraceID: "trace-uninitialized", WorkTypeID: "task"}},
			"tok-initialized-stalled": {ID: "tok-initialized-stalled", PlaceID: "p-work", EnteredAt: baseTokenTime.Add(-5 * time.Minute), Color: workerexecution.Color{WorkID: "work-stalled", TraceID: "trace-stalled", WorkTypeID: "task"}},
		}},
		DispatchHistory: []interfaces.CompletedDispatch{{TransitionID: "tr-stalled-init", DispatchID: "disp-stalled", ConsumedTokens: []workerexecution.Token{{ID: "legacy-stalled", State: "p-init", Color: workerexecution.Color{WorkID: "work-stalled", TraceID: "trace-stalled", WorkTypeID: "task"}}}, StartTime: baseTokenTime.Add(-45 * time.Minute), EndTime: baseTokenTime.Add(-44 * time.Minute)}},
	}

	if decisions := sched.Select(enabled, &snapshot); len(decisions) != 1 || decisions[0].TransitionID != "tr-initialized-stalled" {
		t.Fatalf("expected stalled initialized trace to win, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesProcessingStateAheadOfInitialState(t *testing.T) {
	sched := NewWorkInQueueScheduler(1, nil)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-initial", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-initial", State: "init", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: workerexecution.Color{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-processing", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-processing", State: "review", EnteredAt: baseTokenTime.Add(-5 * time.Minute), Color: workerexecution.Color{WorkID: "work-processing", TraceID: "trace-processing", WorkTypeID: "task"}}}}},
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			"tok-initial":    {ID: "tok-initial", PlaceID: "task:init", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: workerexecution.Color{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"}},
			"tok-processing": {ID: "tok-processing", PlaceID: "task:review", EnteredAt: baseTokenTime.Add(-5 * time.Minute), Color: workerexecution.Color{WorkID: "work-processing", TraceID: "trace-processing", WorkTypeID: "task"}},
		}},
		Topology: schedulerStatePriorityNet(),
	}

	if decisions := sched.Select(enabled, &snapshot); len(decisions) != 1 || decisions[0].TransitionID != "tr-processing" {
		t.Fatalf("expected processing work first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesMultiInputCandidateWithMoreProcessingWork(t *testing.T) {
	sched := NewWorkInQueueScheduler(1, nil)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-one-processing", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{
			"first":  {{ID: "tok-one-processing", State: "review", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: workerexecution.Color{WorkID: "work-one-processing", TraceID: "trace-one-processing", WorkTypeID: "task"}}},
			"second": {{ID: "tok-initial", State: "init", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: workerexecution.Color{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"}}},
		}},
		{TransitionID: "tr-z-two-processing", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{
			"first":  {{ID: "tok-processing-left", State: "review", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-left", TraceID: "trace-left", WorkTypeID: "task"}}},
			"second": {{ID: "tok-processing-right", State: "review", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-right", TraceID: "trace-right", WorkTypeID: "task"}}},
		}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerStatePriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-two-processing" {
		t.Fatalf("expected candidate with more processing work first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_AppliesWorkstationKindBeforeFallbackWhenProcessingCountsTie(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-cron", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-cron-processing", State: "review", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: workerexecution.Color{WorkID: "work-cron", TraceID: "trace-cron", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-z-standard", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-standard-processing", State: "review", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-standard", TraceID: "trace-standard", WorkTypeID: "task"}}}}},
	}

	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-standard" {
		t.Fatalf("expected workstation kind priority first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesWorkerlessGuardedRouteAheadOfStandardWorkstation(t *testing.T) {
	sched := NewWorkInQueueScheduler(1, nil)
	sharedInit := workerexecution.Token{ID: "tok-shared-init", State: "init", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-shared", TraceID: "trace-shared", WorkTypeID: "task"}}
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-z-standard", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {sharedInit}}},
		{TransitionID: "tr-a-logical", Bindings: map[string][]workerexecution.Token{"input": {sharedInit}}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-a-logical" {
		t.Fatalf("expected logical route first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_DoesNotInflateProcessingCountFromResourceObserveOrDuplicateBindings(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	sharedProcessing := workerexecution.Token{ID: "tok-shared-processing", State: "review", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: workerexecution.Color{WorkID: "work-shared", TraceID: "trace-shared", WorkTypeID: "task"}}
	observedProcessing := workerexecution.Token{ID: "tok-observed-processing", State: "review", EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: workerexecution.Color{WorkID: "work-observed", TraceID: "trace-observed", WorkTypeID: "task"}}
	resource := workerexecution.Token{ID: "tok-resource", State: "slot", Color: workerexecution.Color{DataType: workerexecution.DataTypeResource}}

	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-duplicate-and-observe", WorkerType: "agent", ArcModes: map[string]interfaces.ArcMode{"context": interfaces.ArcModeObserve}, Bindings: map[string][]workerexecution.Token{"context": {observedProcessing}, "resource": {resource}, "first": {sharedProcessing}, "second": {sharedProcessing}}},
		{TransitionID: "tr-z-two-distinct-processing", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{
			"first":  {{ID: "tok-processing-left", State: "review", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-left", TraceID: "trace-left", WorkTypeID: "task"}}},
			"second": {{ID: "tok-processing-right", State: "review", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-right", TraceID: "trace-right", WorkTypeID: "task"}}},
		}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerStatePriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-two-distinct-processing" {
		t.Fatalf("expected unique consumed processing work to count, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_DoesNotInflateProcessingCountFromSystemTimeToken(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	initialWork := workerexecution.Token{ID: "tok-initial", State: "init", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"}}
	timeWork := workerexecution.Token{ID: "tok-time", State: interfaces.SystemTimePendingState, EnteredAt: baseTokenTime.Add(-30 * time.Minute), Color: workerexecution.Color{WorkID: "time-work", TraceID: "time-trace", WorkTypeID: interfaces.SystemTimeWorkTypeID, DataType: workerexecution.DataTypeWork}}
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-cron", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {initialWork}, "time": {timeWork}}},
		{TransitionID: "tr-z-standard", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {initialWork}}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-standard" {
		t.Fatalf("expected system time token not to inflate cron priority, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesInitialCustomerWorkAheadOfResourceOnlyCandidate(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-resource", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"resource": {{ID: "tok-resource", State: "slot", Color: workerexecution.Color{DataType: workerexecution.DataTypeResource}}}}},
		{TransitionID: "tr-z-initial", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-initial", State: "init", Color: workerexecution.Color{WorkID: "work-initial", TraceID: "trace-initial", WorkTypeID: "task"}}}}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerStatePriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-initial" {
		t.Fatalf("expected initial customer work to outrank resource-only candidate, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PrioritizesStandardAndRepeaterAheadOfCronAtEqualStatePriority(t *testing.T) {
	sched := newPriorityAwareScheduler(2)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-cron", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-cron", State: "init", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-cron", TraceID: "trace-cron", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-b-standard", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-standard", State: "init", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-standard", TraceID: "trace-standard", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-c-repeater", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-repeater", State: "init", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-repeater", TraceID: "trace-repeater", WorkTypeID: "task"}}}}},
	}
	decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()})
	if got, want := strings.Join(firingDecisionIDs(decisions), ","), "tr-b-standard,tr-c-repeater"; len(decisions) != 2 || got != want {
		t.Fatalf("expected standard and repeater before cron, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_TreatsStandardAndRepeaterAsEqualKindPriority(t *testing.T) {
	sched := newPriorityAwareScheduler(1)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-z-standard", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-standard", State: "init", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-standard", TraceID: "trace-standard", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-b-repeater", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-repeater", State: "init", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-repeater", TraceID: "trace-repeater", WorkTypeID: "task"}}}}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-b-repeater" {
		t.Fatalf("expected authored transition names to preserve standard/repeater ordering, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_SelectsSystemTimeExpiryWhenOnlyEligibleCleanup(t *testing.T) {
	sched := NewWorkInQueueScheduler(1, nil)
	enabled := []interfaces.EnabledTransition{{TransitionID: interfaces.SystemTimeExpiryTransitionID, Bindings: map[string][]workerexecution.Token{"time": {{ID: "tok-expired-time", State: interfaces.SystemTimePendingState, EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "time-work", TraceID: "time-trace", WorkTypeID: interfaces.SystemTimeWorkTypeID, DataType: workerexecution.DataTypeWork}}}}}}
	decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerWorkstationPriorityNet()})
	if len(decisions) != 1 || decisions[0].TransitionID != interfaces.SystemTimeExpiryTransitionID || strings.Join(decisions[0].ConsumeTokens, ",") != "tok-expired-time" {
		t.Fatalf("expected expiry cleanup transition, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_PreservesQueueAgeWithinSameStatePriority(t *testing.T) {
	sched := NewWorkInQueueScheduler(1, nil)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-a-newer", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-newer", State: "init", EnteredAt: baseTokenTime.Add(10 * time.Minute), Color: workerexecution.Color{WorkID: "work-newer", TraceID: "trace-newer", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-z-older", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-older", State: "init", EnteredAt: baseTokenTime, Color: workerexecution.Color{WorkID: "work-older", TraceID: "trace-older", WorkTypeID: "task"}}}}},
	}
	if decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: schedulerStatePriorityNet()}); len(decisions) != 1 || decisions[0].TransitionID != "tr-z-older" {
		t.Fatalf("expected older same-state work first, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_FiltersCompletedAndInvalidCandidates(t *testing.T) {
	sched := NewWorkInQueueScheduler(4, nil)
	enabled := []interfaces.EnabledTransition{
		{TransitionID: "tr-complete", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-complete", State: "p-complete", Color: workerexecution.Color{WorkID: "work-complete", TraceID: "trace-complete", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-live", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "tok-live", State: "p-live", Color: workerexecution.Color{WorkID: "work-live", TraceID: "trace-live", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-empty-token", WorkerType: "agent", Bindings: map[string][]workerexecution.Token{"input": {{ID: "", State: "p-live", Color: workerexecution.Color{WorkID: "work-empty", TraceID: "trace-empty", WorkTypeID: "task"}}}}},
		{TransitionID: "tr-observe-only", WorkerType: "agent", ArcModes: map[string]interfaces.ArcMode{"context": interfaces.ArcModeObserve}, Bindings: map[string][]workerexecution.Token{"context": {{ID: "tok-observed", State: "p-context", Color: workerexecution.Color{DataType: workerexecution.DataTypeResource}}}}},
	}
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		DispatchHistory: []interfaces.CompletedDispatch{{TransitionID: "tr-complete", DispatchID: "disp-complete", ConsumedTokens: []workerexecution.Token{{ID: "live-complete", State: "p-complete", Color: workerexecution.Color{WorkID: "work-complete", TraceID: "trace-complete", WorkTypeID: "task"}}}, StartTime: baseTokenTime.Add(-5 * time.Minute), EndTime: baseTokenTime.Add(-4 * time.Minute)}},
	}
	if decisions := sched.Select(enabled, &snapshot); len(decisions) != 1 || decisions[0].TransitionID != "tr-live" {
		t.Fatalf("expected completed and invalid candidates filtered out, got %v", firingDecisionIDs(decisions))
	}
}

func TestWorkInQueueScheduler_IncludesObservedBindingsWithoutConsumingThem(t *testing.T) {
	sched := NewWorkInQueueScheduler(1, nil)
	parent := workerexecution.Token{ID: "parent", State: "waiting", Color: workerexecution.Color{WorkID: "work-parent", WorkTypeID: "parent"}}
	childA := workerexecution.Token{ID: "child-a", State: "complete", Color: workerexecution.Color{WorkID: "work-a", WorkTypeID: "child"}}
	childB := workerexecution.Token{ID: "child-b", State: "complete", Color: workerexecution.Color{WorkID: "work-b", WorkTypeID: "child"}}
	enabled := []interfaces.EnabledTransition{{
		TransitionID: "merge",
		WorkerType:   "merger",
		Bindings: map[string][]workerexecution.Token{
			"parent":   {parent},
			"children": {childA, childB},
		},
		ArcModes: map[string]interfaces.ArcMode{"children": interfaces.ArcModeObserve},
	}}

	decisions := sched.Select(enabled, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Topology: &state.Net{}})
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	decision := decisions[0]
	if strings.Join(decision.InputTokens, ",") != "child-a,child-b,parent" {
		t.Fatalf("input tokens = %v, want observed children and consumed parent", decision.InputTokens)
	}
	if strings.Join(decision.ConsumeTokens, ",") != "parent" {
		t.Fatalf("consume tokens = %v, want only parent", decision.ConsumeTokens)
	}
	if strings.Join(decision.InputBindings["children"], ",") != "child-a,child-b" {
		t.Fatalf("child input bindings = %v, want both observed children", decision.InputBindings["children"])
	}
}
