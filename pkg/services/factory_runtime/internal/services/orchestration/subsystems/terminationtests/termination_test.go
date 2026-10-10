package subsystems_test

import (
	"context"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestTerminationCheck_TerminatesWhenNoWorkIsInTheSystem(t *testing.T) {
	n := buildTerminationNet()
	tc := subsystems.NewTerminationCheckWithRuntime(n, logging.NoopLogger{}, interfaces.RuntimeModeBatch, nil, func() time.Time { return time.Unix(100, 0) }, terminationEnablement{})

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: factorytoken.Color{WorkID: "gpu:0", WorkTypeID: "gpu"}},
			"time-tok": {
				ID:      "time-tok",
				PlaceID: interfaces.SystemTimePendingPlaceID,
				Color: factorytoken.Color{
					DataType:   factorytoken.DataTypeWork,
					WorkID:     "system-time",
					WorkTypeID: interfaces.SystemTimeWorkTypeID,
				},
			},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.ShouldTerminate {
		t.Fatal("should terminate when no work is in the system")
	}
	if result.Termination == nil || result.Termination.Classification != interfaces.TerminationClassificationComplete {
		t.Fatalf("termination = %+v, want complete classification", result.Termination)
	}
}

func TestTerminationCheck_DoesNotTerminateWithImmediatelyRunnableWork(t *testing.T) {
	n := buildTerminationNet()
	tc := subsystems.NewTerminationCheckWithRuntime(
		n,
		logging.NoopLogger{},
		interfaces.RuntimeModeBatch,
		nil,
		func() time.Time { return time.Unix(100, 0) },
		terminationEnablement{enabled: []interfaces.EnabledTransition{{TransitionID: "tr1"}}},
	)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"tok1":     {ID: "tok1", PlaceID: "wt:init", Color: factorytoken.Color{WorkID: "w1"}},
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: factorytoken.Color{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && result.ShouldTerminate {
		t.Fatal("should not terminate while non-terminal work remains immediately runnable")
	}
}

func TestTerminationCheck_TerminatesWhenAllWorkIsTerminal(t *testing.T) {
	n := buildTerminationNet()
	tc := subsystems.NewTerminationCheckWithRuntime(n, logging.NoopLogger{}, interfaces.RuntimeModeBatch, nil, func() time.Time { return time.Unix(100, 0) }, terminationEnablement{})

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"tok1":     {ID: "tok1", PlaceID: "wt:done", Color: factorytoken.Color{WorkID: "w1"}},
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: factorytoken.Color{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.ShouldTerminate {
		t.Fatal("should terminate when all work is terminal")
	}
	if result.Termination == nil || result.Termination.Classification != interfaces.TerminationClassificationComplete {
		t.Fatalf("termination = %+v, want complete classification", result.Termination)
	}
}

func TestTerminationCheck_TerminatesWhenAllWorkHasFailed(t *testing.T) {
	n := buildTerminationNet()
	tc := subsystems.NewTerminationCheckWithRuntime(n, logging.NoopLogger{}, interfaces.RuntimeModeBatch, nil, func() time.Time { return time.Unix(100, 0) }, terminationEnablement{})

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"tok1":     {ID: "tok1", PlaceID: "wt:failed", Color: factorytoken.Color{WorkID: "w1"}},
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: factorytoken.Color{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.ShouldTerminate {
		t.Fatal("should terminate when all work has failed")
	}
	if result.Termination == nil || result.Termination.Classification != interfaces.TerminationClassificationComplete {
		t.Fatalf("termination = %+v, want complete classification", result.Termination)
	}
}

func TestTerminationCheck_ClassifiesDrainedNonTerminalWorkByDistinctCustomerWorkID(t *testing.T) {
	n := buildTerminationNetNoTransitions()
	tc := subsystems.NewTerminationCheckWithRuntime(n, logging.NoopLogger{}, interfaces.RuntimeModeBatch, nil, func() time.Time { return time.Unix(100, 0) }, terminationEnablement{})

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"tok1":      {ID: "tok1", PlaceID: "wt:init", Color: factorytoken.Color{WorkID: "w1"}},
			"tok1-copy": {ID: "tok1-copy", PlaceID: "wt:init", Color: factorytoken.Color{WorkID: "w1"}},
			"tok2":      {ID: "tok2", PlaceID: "wt:init", Color: factorytoken.Color{WorkID: "w2"}},
			"res-tok0":  {ID: "res-tok0", PlaceID: "gpu:available", Color: factorytoken.Color{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.ShouldTerminate {
		t.Fatal("drained non-terminal work should terminate the finite runtime")
	}
	if result.Termination == nil {
		t.Fatal("expected explicit termination classification")
	}
	if result.Termination.Classification != interfaces.TerminationClassificationIncomplete {
		t.Fatalf("classification = %q, want incomplete", result.Termination.Classification)
	}
	if result.Termination.NonTerminalWorkCount != 2 {
		t.Fatalf("non-terminal work count = %d, want 2 distinct customer Work items", result.Termination.NonTerminalWorkCount)
	}
}

func TestTerminationCheck_DoesNotTerminateWhileDispatchesAreInFlight(t *testing.T) {
	n := buildTerminationNet()
	tc := subsystems.NewTerminationCheckWithRuntime(n, logging.NoopLogger{}, interfaces.RuntimeModeBatch, nil, func() time.Time { return time.Unix(100, 0) }, terminationEnablement{})

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		InFlightCount: 1,
		Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"tok1":     {ID: "tok1", PlaceID: "wt:init", Color: factorytoken.Color{WorkID: "w1"}},
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: factorytoken.Color{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && result.ShouldTerminate {
		t.Fatal("should not terminate while work is still in flight")
	}
}

func TestTerminationCheck_DoesNotTerminateWhileObservedResponseAwaitsRetirement(t *testing.T) {
	n := buildTerminationNet()
	tc := subsystems.NewTerminationCheckWithRuntime(n, logging.NoopLogger{}, interfaces.RuntimeModeBatch, nil, func() time.Time { return time.Unix(100, 0) }, terminationEnablement{})

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		InFlightCount: 1,
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch-throttled": {DispatchID: "dispatch-throttled"},
		},
		Results: []workerexecution.WorkResult{{
			DispatchID: "dispatch-throttled",
			Outcome:    workerexecution.OutcomeFailed,
		}},
		Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"tok1":     {ID: "tok1", PlaceID: "wt:done", Color: factorytoken.Color{WorkID: "w1"}},
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: factorytoken.Color{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && result.ShouldTerminate {
		t.Fatal("should not terminate before the observed dispatch is retired")
	}
}

func TestTerminationCheck_DoesNotTerminateUntilResourcesReturn(t *testing.T) {
	n := buildTerminationNet()
	tc := subsystems.NewTerminationCheckWithRuntime(n, logging.NoopLogger{}, interfaces.RuntimeModeBatch, nil, func() time.Time { return time.Unix(100, 0) }, terminationEnablement{})

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"tok1": {ID: "tok1", PlaceID: "wt:done", Color: factorytoken.Color{WorkID: "w1"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && result.ShouldTerminate {
		t.Fatal("should not terminate until resource tokens are returned")
	}
}

func TestTerminationCheck_ResourcesOnlyTerminates(t *testing.T) {
	n := buildTerminationNetNoTransitions()
	tc := subsystems.NewTerminationCheckWithRuntime(n, logging.NoopLogger{}, interfaces.RuntimeModeBatch, nil, func() time.Time { return time.Unix(100, 0) }, terminationEnablement{})

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: factorytoken.Color{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.ShouldTerminate {
		t.Fatal("should terminate when only returned resources remain")
	}
	if result.Termination == nil || result.Termination.Classification != interfaces.TerminationClassificationComplete {
		t.Fatalf("termination = %+v, want complete classification", result.Termination)
	}
}

func TestTerminationCheck_ServiceModeDoesNotTerminateIdleRuntime(t *testing.T) {
	n := buildTerminationNetNoTransitions()
	tc := subsystems.NewTerminationCheckWithRuntime(n, logging.NoopLogger{}, interfaces.RuntimeModeService, nil, func() time.Time { return time.Unix(100, 0) }, terminationEnablement{})

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: factorytoken.Color{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && result.ShouldTerminate {
		t.Fatal("service mode should stay alive while idle")
	}
}

// buildTerminationNet creates a test net with one work type and one resource.
func buildTerminationNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"wt:init":                           {ID: "wt:init", TypeID: "wt", State: "init"},
			"wt:done":                           {ID: "wt:done", TypeID: "wt", State: "done"},
			"wt:failed":                         {ID: "wt:failed", TypeID: "wt", State: "failed"},
			"gpu:available":                     {ID: "gpu:available", TypeID: "gpu", State: "available"},
			interfaces.SystemTimePendingPlaceID: {ID: interfaces.SystemTimePendingPlaceID, TypeID: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID: "t1",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "wt:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a2", Name: "out", PlaceID: "wt:done", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"wt": {
				ID: "wt",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "done", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
		Resources: map[string]*state.ResourceDef{
			"gpu": {ID: "gpu", Name: "GPU", Capacity: 1},
		},
	}
}

// buildTerminationNetNoTransitions creates a test net with no transitions.
func buildTerminationNetNoTransitions() *state.Net {
	net := buildTerminationNet()
	net.Transitions = map[string]*petri.Transition{}
	return net
}

func makeTerminationSnapshot(tokens map[string]*factorytoken.Token) petri.MarkingSnapshot {
	placeTokens := make(map[string][]string)
	for id, tok := range tokens {
		if tok.CreatedAt.IsZero() {
			tok.CreatedAt = time.Unix(100, 0)
		}
		if tok.EnteredAt.IsZero() {
			tok.EnteredAt = time.Unix(100, 0)
		}
		placeTokens[tok.PlaceID] = append(placeTokens[tok.PlaceID], id)
	}
	return petri.MarkingSnapshot{
		Tokens:      tokens,
		PlaceTokens: placeTokens,
	}
}

// terminationEnablement controls runnable decisions independently of guard evaluation.
type terminationEnablement struct {
	enabled []interfaces.EnabledTransition
}

func (e terminationEnablement) FindEnabledTransitionsWithSnapshot(
	context.Context,
	*state.Net,
	*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	logging.Logger,
	func() time.Time,
	interfaces.RuntimeDefinitionLookup,
) []interfaces.EnabledTransition {
	return e.enabled
}

func (e terminationEnablement) ExpandRepeatedBindings(
	_ *state.Net,
	_ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	enabled []interfaces.EnabledTransition,
	_ logging.Logger,
	_ func() time.Time,
	_ interfaces.RuntimeDefinitionLookup,
) []interfaces.EnabledTransition {
	return enabled
}

type scopedTerminationEnablement struct {
	terminationEnablement
	evaluate func(context.Context, *state.Net, *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], logging.Logger, func() time.Time, interfaces.RuntimeDefinitionLookup) []interfaces.EnabledTransition
}

func (e scopedTerminationEnablement) FindEnabledTransitionsWithSnapshot(
	ctx context.Context,
	net *state.Net,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	logger logging.Logger,
	now func() time.Time,
	lookup interfaces.RuntimeDefinitionLookup,
) []interfaces.EnabledTransition {
	return e.evaluate(ctx, net, snapshot, logger, now, lookup)
}

func TestTerminationCheck_ForwardsIndependentScopesToCompletedEnablement(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	evaluator := scopedTerminationEnablement{}
	snapshots := make([]*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], 2)
	nets := []*state.Net{buildTerminationNet(), buildTerminationNet()}
	clocks := []time.Time{time.Unix(100, 0), time.Unix(200, 0)}
	lookups := []*runtimefixtures.RuntimeDefinitionLookupFixture{{}, {}}
	checks := make([]*subsystems.TerminationCheckSubsystem, 2)
	calls := [2]int{}
	evaluator.evaluate = func(gotCtx context.Context, net *state.Net, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], logger logging.Logger, now func() time.Time, lookup interfaces.RuntimeDefinitionLookup) []interfaces.EnabledTransition {
		for i := range nets {
			if net != nets[i] {
				continue
			}
			calls[i]++
			if gotCtx != ctx || snap != snapshots[i] || logger != (logging.NoopLogger{}) || lookup != lookups[i] || !now().Equal(clocks[i]) {
				t.Fatal("enablement received another scope")
			}
			if now().Unix() >= 200 {
				return []interfaces.EnabledTransition{{TransitionID: "t1"}}
			}
			return nil
		}
		t.Fatal("enablement received unknown topology")
		return nil
	}
	for i := range checks {
		snapshots[i] = &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{
			"work":     {ID: "work", PlaceID: "wt:init", Color: factorytoken.Color{WorkID: "w1"}},
			"resource": {ID: "resource", PlaceID: "gpu:available", Color: factorytoken.Color{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		})}
		checks[i] = subsystems.NewTerminationCheckWithRuntime(nets[i], logging.NoopLogger{}, interfaces.RuntimeModeBatch, lookups[i], func() time.Time { return clocks[i] }, evaluator)
	}
	for _, i := range []int{0, 1, 0} {
		result, err := checks[i].Execute(ctx, snapshots[i])
		assertTerminationScopeResult(t, i, result, err)
	}
	clocks[0] = time.Unix(200, 0)
	if result, err := checks[0].Execute(ctx, snapshots[0]); err != nil || result != nil {
		t.Fatalf("advanced scope result = %+v, %v", result, err)
	}
	if calls != [2]int{3, 1} {
		t.Fatalf("evaluation calls = %v", calls)
	}
}

func assertTerminationScopeResult(t *testing.T, scope int, result *interfaces.TickResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if scope == 0 && (result == nil || result.Termination.Classification != interfaces.TerminationClassificationIncomplete) {
		t.Fatalf("scope 0 result = %+v", result)
	}
	if scope == 1 && result != nil {
		t.Fatalf("scope 1 runnable result = %+v", result)
	}
}

func TestTerminationCheck_DefaultServiceAndDispatchOnlyWaiting(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		mode     interfaces.RuntimeMode
		dispatch bool
		complete bool
	}{
		{"empty mode defaults to batch", "", false, true},
		{"service remains live", interfaces.RuntimeModeService, false, false},
		{"dispatch alone prevents completion", interfaces.RuntimeModeBatch, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: makeTerminationSnapshot(map[string]*factorytoken.Token{})}
			net := buildTerminationNet()
			net.Resources = nil
			if test.dispatch {
				snapshot.Dispatches = map[string]*interfaces.DispatchEntry{"active": {}}
			}
			checker := subsystems.NewTerminationCheckWithRuntime(net, logging.NoopLogger{}, test.mode, nil, func() time.Time { return time.Unix(100, 0) }, terminationEnablement{})
			result, err := checker.Execute(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if (result != nil && result.ShouldTerminate) != test.complete {
				t.Fatalf("result = %+v, want complete=%t", result, test.complete)
			}
		})
	}
}
