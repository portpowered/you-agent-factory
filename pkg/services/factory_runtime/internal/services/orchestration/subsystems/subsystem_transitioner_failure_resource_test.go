package subsystems

import (
	"context"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token_transformer"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestTransitionerPreservesNoArcDiagnosticWhileContextIsActive(t *testing.T) {
	net := workerBatchTestNet()
	net.Transitions["t1"].FailureArcs = nil
	snapshot := workerBatchSnapshot("")
	snapshot.Results[0].Outcome = workerexecution.OutcomeFailed
	snapshot.Results[0].Error = "ordinary failure"
	transitioner := NewTransitioner(
		net, nil, testSubsystemNow, testTokenTransformer(net), nil, nil, nil,
		testWorkPropagationPolicy(),
	)

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err == nil || !strings.Contains(err.Error(), "transition t1 has no arcs for outcome FAILED") {
		t.Fatalf("Execute() = (%#v, %v), want the existing no-arc diagnostic", result, err)
	}
	if result != nil {
		t.Fatalf("Execute() result = %#v, want nil on an unroutable live failure", result)
	}
}

func TestTransitioner_ExpectedArtifactFailureUsesFailureDestination(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
	snapshot := workerBatchSnapshot("worker output")
	snapshot.Dispatches["dispatch-1"].ExpectedArtifactContext = &work.ExpectedArtifactTemplateContext{Project: "project-7", SessionID: "session-9"}
	snapshot.Results[0] = workerexecution.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeFailed,
		Output:       "worker output",
		Error:        "EXPECTED_ARTIFACTS_UNSATISFIED: report=report.json (MISSING)",
		ArtifactVerification: &workerexecution.ExpectedArtifactVerification{
			Code: workerexecution.WorkFailureTypeExpectedArtifactsUnsatisfied,
			Entries: []workerexecution.ExpectedArtifactVerificationEntry{{
				Name: "report", Pattern: "report.json", Reason: workerexecution.ExpectedArtifactVerificationReasonMissing,
			}},
		},
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyTerminal,
			Type:   workerexecution.WorkFailureTypeExpectedArtifactsUnsatisfied,
		},
	}

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 || result.Mutations[0].NewToken == nil {
		t.Fatalf("transitioner result = %#v, want one failure mutation", result)
	}
	if result.Mutations[0].ToPlace != "task:failed" {
		t.Fatalf("failure mutation destination = %q, want task:failed", result.Mutations[0].ToPlace)
	}
	if len(result.CompletedDispatches) != 1 || result.CompletedDispatches[0].Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("completed dispatches = %#v, want failed terminal completion", result.CompletedDispatches)
	}
	if result.CompletedDispatches[0].ArtifactVerification == nil ||
		len(result.CompletedDispatches[0].ArtifactVerification.Entries) != 1 {
		t.Fatalf("completed artifact verification = %#v, want durable failure entries", result.CompletedDispatches[0].ArtifactVerification)
	}
	if result.CompletedDispatches[0].ExpectedArtifactContext == nil ||
		result.CompletedDispatches[0].ExpectedArtifactContext.Project != "project-7" ||
		result.CompletedDispatches[0].ExpectedArtifactContext.SessionID != "session-9" {
		t.Fatalf("completed artifact context = %#v, want recorded context", result.CompletedDispatches[0].ExpectedArtifactContext)
	}
}

func TestTransitioner_TerminalFailureBypassesAuthoredRetryRoute(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	net.Transitions["t1"].FailureArcs = []petri.Arc{{ID: "retry", PlaceID: "task:init"}}
	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
	snapshot := workerBatchSnapshot("")
	snapshot.Results[0] = workerexecution.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "Codex requires a trusted working directory",
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyTerminal,
			Type:   workerexecution.WorkFailureTypePermanentBadRequest,
		},
	}

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("transitioner result = %#v, want one terminal failure mutation", result)
	}
	if got := result.Mutations[0].ToPlace; got != "task:failed" {
		t.Fatalf("failure mutation destination = %q, want task:failed", got)
	}
}

func TestReleaseResourceTokensOnFailure_PreservesConsumedTokenIdentityRegardlessOfInputOrder(t *testing.T) {
	now := time.Date(2026, time.July, 3, 10, 30, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	resourceConsumed := factorytoken.Token{
		ID:        "executor:resource:0",
		PlaceID:   "executor:available",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: factorytoken.Color{
			WorkID:     "executor:0",
			WorkTypeID: "executor",
			DataType:   factorytoken.DataTypeResource,
			Tags:       map[string]string{"pool": "shared"},
		},
		History: factorytoken.History{
			PlaceVisits: map[string]int{"executor:available": 4},
		},
	}
	workConsumed := factorytoken.Token{
		ID:        "tok-1",
		PlaceID:   "wt-code:init",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		Color: factorytoken.Color{
			WorkID:     "w-resource-failure",
			WorkTypeID: "wt-code",
			DataType:   factorytoken.DataTypeWork,
		},
	}
	failureArcs := []petri.Arc{{ID: "a3", Name: "fail", PlaceID: "wt-code:failed", Direction: petri.ArcOutput}}
	transitioner := NewTransitioner(&state.Net{
		Places: map[string]*petri.Place{
			"executor:available": {ID: "executor:available", TypeID: "executor", State: "available"},
		},
	}, nil, func() time.Time { return now }, testTokenTransformer(&state.Net{
		Places: map[string]*petri.Place{
			"executor:available": {ID: "executor:available", TypeID: "executor", State: "available"},
		},
	}),

		nil, nil, nil, testWorkPropagationPolicy())

	orderings := []struct {
		name     string
		consumed []factorytoken.Token
	}{
		{name: "resource-first", consumed: []factorytoken.Token{resourceConsumed, workConsumed}},
		{name: "work-first", consumed: []factorytoken.Token{workConsumed, resourceConsumed}},
	}

	for _, ordering := range orderings {
		t.Run(ordering.name, func(t *testing.T) {
			mutations := transitioner.releaseResourceTokensOnFailureMutations(
				workerexecution.OutcomeFailed,
				"t1",
				ordering.consumed,
				failureArcs,
				now,
			)
			if len(mutations) != 1 {
				t.Fatalf("mutation count = %d, want 1 resource release", len(mutations))
			}
			released := mutations[0]
			if released.ToPlace != "executor:available" {
				t.Fatalf("ToPlace = %q, want executor:available", released.ToPlace)
			}
			if released.NewToken.ID != resourceConsumed.ID {
				t.Fatalf("released ID = %q, want %q", released.NewToken.ID, resourceConsumed.ID)
			}
			if released.NewToken.Color.WorkID != resourceConsumed.Color.WorkID {
				t.Fatalf("released WorkID = %q, want %q", released.NewToken.Color.WorkID, resourceConsumed.Color.WorkID)
			}
			if released.NewToken.Color.Tags["pool"] != "shared" {
				t.Fatalf("released tag pool = %q, want shared", released.NewToken.Color.Tags["pool"])
			}
			if !released.NewToken.CreatedAt.Equal(createdAt) {
				t.Fatalf("CreatedAt = %v, want %v", released.NewToken.CreatedAt, createdAt)
			}
		})
	}
}

type acceptedMixedWorkResourceMutationFixture struct {
	transformer *token_transformer.Transformer
	resource    factorytoken.Token
	work        factorytoken.Token
	arcs        []petri.Arc
	result      resolvedWorkResult
	now         time.Time
}

func newAcceptedMixedWorkResourceMutationFixture() acceptedMixedWorkResourceMutationFixture {
	now := time.Date(2026, time.April, 7, 14, 0, 0, 0, time.UTC)
	createdAt := now.Add(-time.Hour)
	places := map[string]*petri.Place{
		"story:complete":       {ID: "story:complete", TypeID: "story", State: "complete"},
		"agent-slot:available": {ID: "agent-slot:available", TypeID: "agent-slot", State: "available"},
		"story:in-review":      {ID: "story:in-review", TypeID: "story", State: "in-review"},
	}
	workTypes := map[string]*state.WorkType{
		"story": {ID: "story"},
	}
	return acceptedMixedWorkResourceMutationFixture{
		transformer: token_transformer.New(places, workTypes, petri.NewWorkIDGenerator()),
		resource: factorytoken.Token{
			ID:        "agent-slot:resource:0",
			PlaceID:   "agent-slot:available",
			CreatedAt: createdAt,
			EnteredAt: createdAt,
			Color: factorytoken.Color{
				WorkID:     "agent-slot:0",
				WorkTypeID: "agent-slot",
				DataType:   factorytoken.DataTypeResource,
			},
		},
		work: factorytoken.Token{
			ID:        "work-story-1",
			PlaceID:   "story:in-review",
			CreatedAt: createdAt,
			EnteredAt: createdAt,
			Color: factorytoken.Color{
				WorkID:     "work-story-1",
				WorkTypeID: "story",
				DataType:   factorytoken.DataTypeWork,
				TraceID:    "trace-batch-idea-001",
			},
			History: factorytoken.History{
				TotalVisits:         map[string]int{},
				ConsecutiveFailures: map[string]int{},
				PlaceVisits:         map[string]int{},
			},
		},
		arcs: []petri.Arc{
			{ID: "work-out", PlaceID: "story:complete"},
			{ID: "slot-out", PlaceID: "agent-slot:available"},
		},
		result: resolvedWorkResult{
			transitionID: "review-story",
			outcome:      workerexecution.OutcomeAccepted,
			output:       "Done. COMPLETE ACCEPTED",
		},
		now: now,
	}
}

func findWorkAndResourceMutations(mutations []interfaces.MarkingMutation) (*interfaces.MarkingMutation, *interfaces.MarkingMutation) {
	var workMutation *interfaces.MarkingMutation
	var resourceMutation *interfaces.MarkingMutation
	for i := range mutations {
		if mutations[i].NewToken == nil {
			continue
		}
		switch mutations[i].NewToken.Color.DataType {
		case factorytoken.DataTypeWork:
			workMutation = &mutations[i]
		case factorytoken.DataTypeResource:
			resourceMutation = &mutations[i]
		}
	}
	return workMutation, resourceMutation
}

func assertAcceptedMixedWorkOutputMutation(t *testing.T, workMutation *interfaces.MarkingMutation) {
	t.Helper()
	if workMutation == nil {
		t.Fatal("expected work output mutation")
	}
	if workMutation.ToPlace != "story:complete" {
		t.Fatalf("work ToPlace = %q, want story:complete", workMutation.ToPlace)
	}
	if workMutation.NewToken.Color.TraceID != "trace-batch-idea-001" {
		t.Fatalf("work TraceID = %q, want trace-batch-idea-001", workMutation.NewToken.Color.TraceID)
	}
}

func assertReleasedResourceMutationBasics(t *testing.T, resourceMutation *interfaces.MarkingMutation, resource factorytoken.Token) {
	t.Helper()
	if resourceMutation == nil {
		t.Fatal("expected resource release mutation")
	}
	if resourceMutation.ToPlace != "agent-slot:available" {
		t.Fatalf("resource ToPlace = %q, want agent-slot:available", resourceMutation.ToPlace)
	}
	if resourceMutation.NewToken.ID != resource.ID {
		t.Fatalf("released resource ID = %q, want consumed identity %q", resourceMutation.NewToken.ID, resource.ID)
	}
	if resourceMutation.NewToken.Color.WorkID != "agent-slot:0" {
		t.Fatalf("released resource WorkID = %q, want agent-slot:0", resourceMutation.NewToken.Color.WorkID)
	}
	if resourceMutation.NewToken.Color.TraceID != "" {
		t.Fatalf("released resource TraceID = %q, want empty", resourceMutation.NewToken.Color.TraceID)
	}
}

func assertAcceptedMixedWorkResourceMutations(t *testing.T, mutations []interfaces.MarkingMutation, resource factorytoken.Token) {
	t.Helper()
	if len(mutations) != 2 {
		t.Fatalf("mutation count = %d, want 2 (work output + resource release)", len(mutations))
	}
	workMutation, resourceMutation := findWorkAndResourceMutations(mutations)
	assertAcceptedMixedWorkOutputMutation(t, workMutation)
	assertReleasedResourceMutationBasics(t, resourceMutation, resource)
}

func TestCalculateMutations_AcceptedMixedWorkResource_ReleasesConsumedResourceRegardlessOfInputOrder(t *testing.T) {
	fixture := newAcceptedMixedWorkResourceMutationFixture()
	orderings := []struct {
		name     string
		consumed []factorytoken.Token
	}{
		{name: "resource-first", consumed: []factorytoken.Token{fixture.resource, fixture.work}},
		{name: "work-first", consumed: []factorytoken.Token{fixture.work, fixture.resource}},
	}

	for _, ordering := range orderings {
		t.Run(ordering.name, func(t *testing.T) {
			mutations, err := calculateMutations(mutationCalculationInput{
				workPropagation: testWorkPropagationPolicy(),
				transition:      &petri.Transition{ID: "review-story"},
				arcs:            fixture.arcs,
				consumed:        ordering.consumed,
				result:          fixture.result,
				now:             fixture.now,
				history:         fixture.work.History,
				inputColors:     tokenColorsFromTokens(ordering.consumed),
				transformer:     fixture.transformer,
			})
			if err != nil {
				t.Fatalf("calculateMutations() error = %v", err)
			}
			assertAcceptedMixedWorkResourceMutations(t, mutations, fixture.resource)
		})
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-resource-release-fixture review=2026-07-18 removal=split-resource-setup-and-failure-release-assertions-before-next-history-transitioner-change
func TestHistoryTransitionerPipeline_FailureReleasesConsumedResourceTokenIdentity(t *testing.T) {
	tp, snapshot, resourceConsumed := newFailureReleasesConsumedResourceFixture(false)
	result, err := tp.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFailedMixedWorkResourceRelease(t, result, resourceConsumed)
}

func TestHistoryTransitionerPipeline_FailureReleasesConsumedResourceRegardlessOfInputOrder(t *testing.T) {
	orderings := []struct {
		name      string
		workFirst bool
	}{
		{name: "resource-first", workFirst: false},
		{name: "work-first", workFirst: true},
	}

	for _, ordering := range orderings {
		t.Run(ordering.name, func(t *testing.T) {
			tp, snapshot, resourceConsumed := newFailureReleasesConsumedResourceFixture(ordering.workFirst)
			result, err := tp.Execute(context.Background(), snapshot)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertFailedMixedWorkResourceRelease(t, result, resourceConsumed)
		})
	}
}

func newFailureReleasesConsumedResourceFixture(workFirst bool) (*testPipeline, *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], factorytoken.Token) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"wt-code:init":       {ID: "wt-code:init", TypeID: "wt-code", State: "init"},
			"wt-code:failed":     {ID: "wt-code:failed", TypeID: "wt-code", State: "failed"},
			"executor:available": {ID: "executor:available", TypeID: "executor", State: "available"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:         "t1",
				Name:       "code",
				WorkerType: "agent",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "wt-code:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "a2", Name: "resource", PlaceID: "executor:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				FailureArcs: []petri.Arc{
					{ID: "a3", Name: "fail", PlaceID: "wt-code:failed", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"wt-code": {
				ID: "wt-code",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
	}
	tp := newTestPipeline(n, nil)
	tp.WriteResult(workerexecution.WorkResult{DispatchID: "d-1", TransitionID: "t1", Outcome: workerexecution.OutcomeFailed, Error: "agent crashed"})

	now := time.Date(2026, time.April, 6, 11, 0, 0, 0, time.UTC)
	resourceConsumed := factorytoken.Token{
		ID:        "executor:resource:0",
		PlaceID:   "executor:available",
		CreatedAt: now.Add(-3 * time.Hour),
		EnteredAt: now.Add(-3 * time.Hour),
		Color: factorytoken.Color{
			WorkID:     "executor:0",
			WorkTypeID: "executor",
			DataType:   factorytoken.DataTypeResource,
			Tags:       map[string]string{"pool": "shared"},
		},
		History: factorytoken.History{
			PlaceVisits: map[string]int{"executor:available": 4},
		},
	}
	workConsumed := factorytoken.Token{
		ID:        "tok-1",
		PlaceID:   "wt-code:init",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		Color: factorytoken.Color{
			WorkID:     "w-resource-failure",
			WorkTypeID: "wt-code",
		},
		History: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}

	consumedTokens := []factorytoken.Token{resourceConsumed, workConsumed}
	if workFirst {
		consumedTokens = []factorytoken.Token{workConsumed, resourceConsumed}
	}

	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*factorytoken.Token{
				workConsumed.ID:     &workConsumed,
				resourceConsumed.ID: &resourceConsumed,
			},
			PlaceTokens: map[string][]string{
				"wt-code:init":       {workConsumed.ID},
				"executor:available": {resourceConsumed.ID},
			},
		},
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-1": {
				DispatchID:     "d-1",
				TransitionID:   "t1",
				ConsumedTokens: factorytoken.ToWorkerSlice(consumedTokens),
			},
		},
	}
	return tp, snapshot, resourceConsumed
}

func assertFailedMixedWorkResourceRelease(t *testing.T, result *interfaces.TickResult, resourceConsumed factorytoken.Token) {
	t.Helper()
	if result == nil || len(result.Mutations) != 2 {
		t.Fatalf("expected 2 mutations, got %+v", result)
	}

	var failedWork *interfaces.MarkingMutation
	var releasedResource *interfaces.MarkingMutation
	for i := range result.Mutations {
		if result.Mutations[i].NewToken == nil {
			continue
		}
		switch result.Mutations[i].NewToken.Color.DataType {
		case factorytoken.DataTypeWork:
			failedWork = &result.Mutations[i]
		case factorytoken.DataTypeResource:
			releasedResource = &result.Mutations[i]
		}
	}
	if failedWork == nil {
		t.Fatal("expected failed work mutation")
	}
	if failedWork.ToPlace != "wt-code:failed" {
		t.Fatalf("failed work ToPlace = %q, want wt-code:failed", failedWork.ToPlace)
	}
	if failedWork.NewToken.Color.WorkID != "w-resource-failure" {
		t.Fatalf("failed work WorkID = %q, want w-resource-failure", failedWork.NewToken.Color.WorkID)
	}
	if failedWork.NewToken.History.LastError != "agent crashed" {
		t.Fatalf("failed work LastError = %q, want agent crashed", failedWork.NewToken.History.LastError)
	}
	assertReleasedResourceMutation(t, releasedResource, resourceConsumed)
}

func assertReleasedResourceMutation(t *testing.T, released *interfaces.MarkingMutation, resourceConsumed factorytoken.Token) {
	t.Helper()
	if released == nil {
		t.Fatal("expected released resource mutation")
	}
	if released.ToPlace != "executor:available" {
		t.Fatalf("ToPlace = %q, want %q", released.ToPlace, "executor:available")
	}
	if released.NewToken.ID != resourceConsumed.ID || released.NewToken.Color.WorkID != resourceConsumed.Color.WorkID {
		t.Fatalf("released resource token = %#v, want preserved identity from %#v", released.NewToken, resourceConsumed)
	}
	if !released.NewToken.CreatedAt.Equal(resourceConsumed.CreatedAt) {
		t.Fatalf("CreatedAt = %v, want %v", released.NewToken.CreatedAt, resourceConsumed.CreatedAt)
	}
	if released.NewToken.Color.Tags["pool"] != "shared" {
		t.Fatalf("tag pool = %q, want %q", released.NewToken.Color.Tags["pool"], "shared")
	}
	if released.NewToken.History.PlaceVisits["executor:available"] != 4 {
		t.Fatalf("PlaceVisits = %#v, want preserved history", released.NewToken.History.PlaceVisits)
	}
}

func TestTransitioner_LateDispatchResultForTerminalOrFailedWorkIsIgnored(t *testing.T) {
	now := time.Date(2026, time.August, 29, 12, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		name      string
		placeID   string
		stateName string
		stateType interfaces.StateType
	}{
		{name: "terminal", placeID: "wt-code:done", stateName: "done", stateType: interfaces.StateTypeTerminal},
		{name: "failed", placeID: "wt-code:failed", stateName: "failed", stateType: interfaces.StateTypeFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertIgnoredTerminalResult(t, now, test.placeID, test.name, test.stateName, test.stateType)
		})
	}
}

func assertIgnoredTerminalResult(t *testing.T, now time.Time, placeID, resultName, stateName string, stateType interfaces.StateType) {
	t.Helper()
	net := buildPipelineNet()
	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
	snapshot := pipelineSnapshot(
		placeID,
		"t1",
		"d-late-"+resultName,
		factorytoken.Color{WorkID: "w-terminal", WorkTypeID: "wt-code"},
		now,
	)
	snapshot.Results = []workerexecution.WorkResult{{
		DispatchID: "d-late-" + resultName, TransitionID: "t1", Outcome: workerexecution.OutcomeAccepted,
		Output: "worker output must not be retained", Error: "worker error must not be retained",
	}}

	result, err := transitioner.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() result = nil, want retired ignored dispatch")
	}
	if len(result.Mutations) != 0 || len(result.GeneratedBatches) != 0 {
		t.Fatalf("ignored result changed runtime = mutations:%#v generated:%#v", result.Mutations, result.GeneratedBatches)
	}
	if len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want one ignored dispatch", result.CompletedDispatches)
	}
	completed := result.CompletedDispatches[0]
	if completed.IgnoredWorkID != "w-terminal" || completed.IgnoredResult == nil {
		t.Fatalf("ignored completion = %#v, want Work w-terminal marker", completed)
	}
	ignored := completed.IgnoredResult
	if ignored.Reason != interfaces.DispatchResultIgnoredReasonWorkAlreadyTerminal ||
		ignored.ResultOutcome != workerexecution.OutcomeAccepted ||
		ignored.ObservedState.Name != stateName || ignored.ObservedState.Type != stateType {
		t.Fatalf("ignored payload = %#v, want %s/%s accepted", ignored, stateName, stateType)
	}
}

func TestTransitioner_LateDispatchResultGuardsMultiInputResultAtomically(t *testing.T) {
	now := time.Date(2026, time.August, 29, 12, 31, 0, 0, time.UTC)
	net := buildPipelineNet()
	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
	snapshot := pipelineSnapshot(
		"wt-code:done", "t1", "d-multi-late",
		factorytoken.Color{WorkID: "w-terminal", WorkTypeID: "wt-code"}, now,
	)
	second := factorytoken.Token{
		ID: "tok-2", PlaceID: "wt-code:init", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{WorkID: "w-processing", WorkTypeID: "wt-code"},
	}
	snapshot.Marking.Tokens[second.ID] = &second
	snapshot.Marking.PlaceTokens[second.PlaceID] = []string{second.ID}
	first := *snapshot.Marking.Tokens["tok-1"]
	snapshot.Dispatches["d-multi-late"].ConsumedTokens = factorytoken.ToWorkerSlice([]factorytoken.Token{first, second})
	snapshot.Results = []workerexecution.WorkResult{{
		DispatchID: "d-multi-late", TransitionID: "t1", Outcome: workerexecution.OutcomeFailed,
		Output: "must not generate or route", Error: "late failure",
	}}

	result, err := transitioner.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || len(result.Mutations) != 0 || len(result.GeneratedBatches) != 0 || len(result.CompletedDispatches) != 1 {
		t.Fatalf("atomic ignored result = %#v, want no mutations/generated and one completion", result)
	}
	completed := result.CompletedDispatches[0]
	if completed.IgnoredWorkID != "w-terminal" || completed.IgnoredResult == nil ||
		completed.IgnoredResult.ObservedState.Type != interfaces.StateTypeTerminal {
		t.Fatalf("multi-input ignored completion = %#v, want first terminal Work marker", completed)
	}
}
