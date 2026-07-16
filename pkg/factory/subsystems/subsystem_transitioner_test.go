package subsystems

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestCalculateArcs(t *testing.T) {
	transition := &petri.Transition{
		ID: "t1",
		OutputArcs: []petri.Arc{
			{ID: "accepted-arc", PlaceID: "wt-code:done"},
		},
		ContinueArcs: []petri.Arc{
			{ID: "continue-arc", PlaceID: "wt-code:init"},
		},
		RejectionArcs: []petri.Arc{
			{ID: "rejected-arc", PlaceID: "wt-code:init"},
		},
		FailureArcs: []petri.Arc{
			{ID: "failed-arc", PlaceID: "wt-code:failed"},
		},
	}

	tests := []struct {
		name      string
		outcome   workerexecution.WorkOutcome
		wantArcID string
		wantErr   bool
	}{
		{name: "Accepted_ReturnsOutputArcs", outcome: workerexecution.OutcomeAccepted, wantArcID: "accepted-arc"},
		{name: "Continue_ReturnsContinueArcs", outcome: workerexecution.OutcomeContinue, wantArcID: "continue-arc"},
		{name: "Rejected_ReturnsRejectionArcs", outcome: workerexecution.OutcomeRejected, wantArcID: "rejected-arc"},
		{name: "Failed_ReturnsFailureArcs", outcome: workerexecution.OutcomeFailed, wantArcID: "failed-arc"},
		{name: "UnknownOutcome_ReturnsError", outcome: workerexecution.WorkOutcome("UNKNOWN"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arcs, err := calculateArcs(transition, tt.outcome)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("calculateArcs returned error: %v", err)
			}
			if len(arcs) != 1 {
				t.Fatalf("expected 1 arc, got %d", len(arcs))
			}
			if arcs[0].ID != tt.wantArcID {
				t.Fatalf("arc ID = %s, want %s", arcs[0].ID, tt.wantArcID)
			}
		})
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-transitioner-table-fixture review=2026-07-18 removal=split-outcome-cases-before-next-transitioner-mutation-change
func TestCalculateMutations(t *testing.T) {
	fixture := newCalculateMutationsFixture()

	tests := []struct {
		name            string
		arcs            []petri.Arc
		result          resolvedWorkResult
		wantPlace       string
		wantWorkTypeID  string
		wantWorkID      string
		wantPayload     []byte
		wantFeedback    string
		wantLastError   string
		wantFailureSize int
		wantCreatedAt   time.Time
	}{
		{
			name: "AcceptedSameType_PreservesCreatedAtAndPayload",
			arcs: []petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      workerexecution.OutcomeAccepted,
				output:       "compiled",
			},
			wantPlace:      "wt-code:done",
			wantWorkTypeID: "wt-code",
			wantWorkID:     "w1",
			wantPayload:    []byte("compiled"),
			wantCreatedAt:  fixture.consumed[0].CreatedAt,
		},
		{
			name: "Rejected_AddsRejectionFeedbackTag",
			arcs: []petri.Arc{{ID: "reject", PlaceID: "wt-code:init"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      workerexecution.OutcomeRejected,
				feedback:     "try again",
			},
			wantPlace:      "wt-code:init",
			wantWorkTypeID: "wt-code",
			wantWorkID:     "w1",
			wantFeedback:   "try again",
			wantCreatedAt:  fixture.consumed[0].CreatedAt,
		},
		{
			name: "Failed_AppendsFailureHistory",
			arcs: []petri.Arc{{ID: "fail", PlaceID: "wt-code:failed"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      workerexecution.OutcomeFailed,
				err:          "agent crashed",
			},
			wantPlace:       "wt-code:failed",
			wantWorkTypeID:  "wt-code",
			wantWorkID:      "w1",
			wantLastError:   "agent crashed",
			wantFailureSize: 1,
			wantCreatedAt:   fixture.consumed[0].CreatedAt,
		},
		{
			name: "AcceptedCrossType_GeneratesNewWorkID",
			arcs: []petri.Arc{{ID: "cross", PlaceID: "wt-review:init"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      workerexecution.OutcomeAccepted,
			},
			wantPlace:      "wt-review:init",
			wantWorkTypeID: "wt-review",
			wantWorkID:     "work-wt-review-1",
			wantCreatedAt:  fixture.now,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutations, err := fixture.calculate(tt.arcs, tt.result)
			if err != nil {
				t.Fatalf("calculateMutations() error = %v", err)
			}
			assertCalculatedMutation(t, mutations, tt, fixture.now)
		})
	}
}

func TestCalculateMutations_RecordedOutputWorkOverridesGeneratedIdentity(t *testing.T) {
	now := time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC)
	places := map[string]*petri.Place{
		"wt-code:init":   {ID: "wt-code:init", TypeID: "wt-code", State: "init"},
		"wt-review:init": {ID: "wt-review:init", TypeID: "wt-review", State: "init"},
	}
	workTypes := map[string]*state.WorkType{
		"wt-code":   {ID: "wt-code"},
		"wt-review": {ID: "wt-review"},
	}
	consumed := []factorytoken.Token{{
		ID:      "tok-1",
		PlaceID: "wt-code:init",
		Color: factorytoken.Color{
			WorkID:     "work-code-1",
			WorkTypeID: "wt-code",
			Name:       "story-1",
			TraceID:    "trace-1",
		},
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		History: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}}

	mutations, err := calculateMutations(mutationCalculationInput{
		transition: &petri.Transition{ID: "t1"},
		arcs:       []petri.Arc{{ID: "cross", PlaceID: "wt-review:init"}},
		consumed:   consumed,
		result: resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeAccepted,
			recordedOutputWork: []work.FactoryWorkItem{{
				ID:                       "work-review-99",
				WorkTypeID:               "wt-review",
				DisplayName:              "review-override",
				CurrentChainingTraceID:   "trace-replay",
				PreviousChainingTraceIDs: []string{"trace-parent"},
				ChainingTraceDepth:       7,
				TraceID:                  "trace-replay",
				Tags:                     map[string]string{"source": "recording"},
			}},
		},
		now:         now,
		history:     factorytoken.History{},
		inputColors: tokenColorsFromTokens(consumed),
		transformer: token_transformer.New(places, workTypes, token_transformer.WithWorkIDGenerator(petri.NewWorkIDGenerator())),
	})
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 1 || mutations[0].NewToken == nil {
		t.Fatalf("mutations = %#v, want one created token", mutations)
	}
	token := mutations[0].NewToken
	if token.Color.WorkID != "work-review-99" || token.ID != "work-review-99" {
		t.Fatalf("recorded output identity = (%q,%q), want work-review-99", token.ID, token.Color.WorkID)
	}
	if token.Color.Name != "review-override" {
		t.Fatalf("recorded output name = %q, want review-override", token.Color.Name)
	}
	if token.Color.TraceID != "trace-replay" || token.Color.CurrentChainingTraceID != "trace-replay" {
		t.Fatalf("recorded output trace fields = %#v, want trace-replay", token.Color)
	}
	if len(token.Color.PreviousChainingTraceIDs) != 1 || token.Color.PreviousChainingTraceIDs[0] != "trace-parent" {
		t.Fatalf("recorded output previous chaining trace IDs = %#v, want [trace-parent]", token.Color.PreviousChainingTraceIDs)
	}
	if token.Color.ChainingTraceDepth != 7 {
		t.Fatalf("recorded output chaining trace depth = %d, want 7", token.Color.ChainingTraceDepth)
	}
	if got := token.Color.Tags["source"]; got != "recording" {
		t.Fatalf("recorded output tags = %#v, want source=recording", token.Color.Tags)
	}
}

func TestCalculateMutations_MultiOutputFanoutPreservesAuthoredNameAcrossAllLanes(t *testing.T) {
	now := time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC)
	places := map[string]*petri.Place{
		"wt-code:init":     {ID: "wt-code:init", TypeID: "wt-code", State: "init"},
		"wt-review-a:init": {ID: "wt-review-a:init", TypeID: "wt-review-a", State: "init"},
		"wt-review-b:init": {ID: "wt-review-b:init", TypeID: "wt-review-b", State: "init"},
		"wt-review-c:init": {ID: "wt-review-c:init", TypeID: "wt-review-c", State: "init"},
	}
	workTypes := map[string]*state.WorkType{
		"wt-code":     {ID: "wt-code"},
		"wt-review-a": {ID: "wt-review-a"},
		"wt-review-b": {ID: "wt-review-b"},
		"wt-review-c": {ID: "wt-review-c"},
	}
	consumed := []factorytoken.Token{{
		ID:      "tok-1",
		PlaceID: "wt-code:init",
		Color: factorytoken.Color{
			WorkID:     "work-code-1",
			WorkTypeID: "wt-code",
			Name:       "prd-shared-name",
			RequestID:  "request-1",
			TraceID:    "trace-1",
		},
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		History: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}}

	mutations, err := calculateMutations(mutationCalculationInput{
		transition: &petri.Transition{ID: "t1"},
		arcs: []petri.Arc{
			{ID: "review-a", PlaceID: "wt-review-a:init"},
			{ID: "review-b", PlaceID: "wt-review-b:init"},
			{ID: "review-c", PlaceID: "wt-review-c:init"},
		},
		consumed: consumed,
		result: resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeAccepted,
		},
		now:         now,
		history:     factorytoken.History{},
		inputColors: tokenColorsFromTokens(consumed),
		transformer: token_transformer.New(places, workTypes, token_transformer.WithWorkIDGenerator(petri.NewWorkIDGenerator())),
	})
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 3 {
		t.Fatalf("mutation count = %d, want 3", len(mutations))
	}

	wantPlaces := []string{"wt-review-a:init", "wt-review-b:init", "wt-review-c:init"}
	wantTypes := []string{"wt-review-a", "wt-review-b", "wt-review-c"}
	for i := range mutations {
		token := mutations[i].NewToken
		if mutations[i].ToPlace != wantPlaces[i] {
			t.Fatalf("mutation %d ToPlace = %q, want %q", i, mutations[i].ToPlace, wantPlaces[i])
		}
		if token == nil {
			t.Fatalf("mutation %d token is nil", i)
		}
		if token.Color.Name != "prd-shared-name" {
			t.Fatalf("mutation %d name = %q, want prd-shared-name", i, token.Color.Name)
		}
		if token.Color.WorkTypeID != wantTypes[i] {
			t.Fatalf("mutation %d work type = %q, want %q", i, token.Color.WorkTypeID, wantTypes[i])
		}
		if token.Color.WorkID == "" || token.Color.WorkID == "work-code-1" {
			t.Fatalf("mutation %d work ID = %q, want fresh generated work ID", i, token.Color.WorkID)
		}
		if token.Color.ParentID != "work-code-1" {
			t.Fatalf("mutation %d parent ID = %q, want work-code-1", i, token.Color.ParentID)
		}
		if token.Color.TraceID != "trace-1" || token.Color.CurrentChainingTraceID != "trace-1" {
			t.Fatalf("mutation %d trace fields = %#v, want trace-1", i, token.Color)
		}
	}
}

func TestResolveWorkResult_RuntimeConfigStopWordsAcceptConfiguredMarker(t *testing.T) {
	transition := &petri.Transition{
		ID:   "transition-id",
		Name: "runtime-station",
	}
	result := &workerexecution.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-id",
		Outcome:      workerexecution.OutcomeRejected,
		Output:       "rendered output DONE",
	}

	resolved := resolveWorkResult(transition, result, runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"runtime-station": {StopWords: []string{"DONE"}},
		},
	})

	if resolved.outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("resolved outcome = %s, want ACCEPTED", resolved.outcome)
	}
}

func TestResolveWorkResult_RuntimeConfigStopWordsFailMissingMarker(t *testing.T) {
	transition := &petri.Transition{
		ID:   "transition-id",
		Name: "runtime-station",
	}
	result := &workerexecution.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-id",
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "rendered output without marker",
	}

	resolved := resolveWorkResult(transition, result, runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"runtime-station": {StopWords: []string{"DONE"}},
		},
	})

	if resolved.outcome != workerexecution.OutcomeFailed {
		t.Fatalf("resolved outcome = %s, want FAILED", resolved.outcome)
	}
}

func TestResolveWorkResult_MissingRuntimeConfigPreservesOriginalOutcome(t *testing.T) {
	transition := &petri.Transition{
		ID: "runtime-station-id",
	}
	result := &workerexecution.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "runtime-station-id",
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "rendered output without marker",
	}

	resolved := resolveWorkResult(transition, result, nil)

	if resolved.outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("resolved outcome = %s, want original ACCEPTED when runtime config is missing", resolved.outcome)
	}
}

func TestTransitioner_PreserveInput_SpawnedWorkKeepsConsumedWorkData(t *testing.T) {
	now := time.Date(2026, time.June, 20, 12, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	net.Transitions["t1"].Name = "spawner"

	transitioner := NewTransitioner(net, nil,
		WithTransitionerClock(func() time.Time { return now }),
		WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"spawner": {
					WorkPropagation: &interfaces.WorkPropagationConfig{
						Mode: interfaces.WorkPropagationModePreserveInput,
					},
				},
			},
		}),
	)

	snapshot := workerBatchSnapshot("worker-output")
	snapshot.Dispatches["dispatch-1"].ConsumedTokens[0].Color.Payload = []byte("input-payload")
	snapshot.Dispatches["dispatch-1"].ConsumedTokens[0].Color.Content = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "input-content",
	}}
	snapshot.Dispatches["dispatch-1"].ConsumedTokens[0].Color.Tags = map[string]string{"objective": "goal-1"}
	snapshot.Results[0].SpawnedWork = []factorytoken.Color{
		{WorkID: "child-1", WorkTypeID: "child", DataType: factorytoken.DataTypeWork, ParentID: "work-source"},
		{WorkID: "child-2", WorkTypeID: "child", DataType: factorytoken.DataTypeWork, ParentID: "work-source"},
	}

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}

	childTokens := make([]*factorytoken.Token, 0, 2)
	for i := range result.Mutations {
		if result.Mutations[i].ToPlace == "child:init" {
			childTokens = append(childTokens, result.Mutations[i].NewToken)
		}
	}
	if len(childTokens) != 2 {
		t.Fatalf("spawned child count = %d, want 2", len(childTokens))
	}
	for i, child := range childTokens {
		if string(child.Color.Payload) != "input-payload" {
			t.Fatalf("child %d payload = %q, want input-payload", i, child.Color.Payload)
		}
		if len(child.Color.Content) != 1 || child.Color.Content[0].Text != "input-content" {
			t.Fatalf("child %d content = %#v, want preserved input content", i, child.Color.Content)
		}
		if child.Color.Tags["objective"] != "goal-1" {
			t.Fatalf("child %d tags = %#v, want preserved input tags", i, child.Color.Tags)
		}
	}
}

func TestTransitioner_SpawnedWorkRelationsRemainDetachedFromResultMutation(t *testing.T) {
	now := time.Date(2026, time.May, 24, 2, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	snapshot := workerBatchSnapshot("")
	snapshot.Results[0].SpawnedWork = []factorytoken.Color{{
		WorkID:     "child-work-1",
		WorkTypeID: "child",
		DataType:   factorytoken.DataTypeWork,
		Relations: []work.Relation{{
			Type:          work.RelationDependsOn,
			TargetWorkID:  "work-source",
			RequiredState: "complete",
		}},
	}}

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}

	snapshot.Results[0].SpawnedWork[0].Relations[0].TargetWorkID = "mutated-after-execute"

	var child *factorytoken.Token
	for i := range result.Mutations {
		if result.Mutations[i].ToPlace == "child:init" {
			child = result.Mutations[i].NewToken
			break
		}
	}
	if child == nil {
		t.Fatalf("mutations = %#v, want spawned child token", result.Mutations)
	}
	if len(child.Color.Relations) != 1 {
		t.Fatalf("spawned token relations = %#v, want one dependency relation", child.Color.Relations)
	}
	if child.Color.Relations[0].TargetWorkID != "work-source" {
		t.Fatalf("spawned token relation target = %q, want detached work-source", child.Color.Relations[0].TargetWorkID)
	}
}

func TestResolveWorkResult_RuntimeConfigUsesTransitionName(t *testing.T) {
	transition := &petri.Transition{
		ID:   "runtime-station-id",
		Name: "runtime-station",
	}
	result := &workerexecution.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "runtime-station-id",
		Outcome:      workerexecution.OutcomeRejected,
		Output:       "rendered output DONE",
	}

	resolved := resolveWorkResult(transition, result, runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"runtime-station": {StopWords: []string{"DONE"}},
		},
	})

	if resolved.outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("resolved outcome = %s, want ACCEPTED from transition name lookup", resolved.outcome)
	}
}

func TestTransitioner_CompletedDispatchPreservesProviderSession(t *testing.T) {
	now := time.Date(2026, time.April, 8, 20, 0, 0, 0, time.UTC)
	net := &state.Net{
		Places: map[string]*petri.Place{
			"task:init":     {ID: "task:init", TypeID: "task", State: "init"},
			"task:complete": {ID: "task:complete", TypeID: "task", State: "complete"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {ID: "task"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID: "t1",
				OutputArcs: []petri.Arc{
					{ID: "out", PlaceID: "task:complete"},
				},
			},
		},
	}
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-1": {
				DispatchID:      "d-1",
				TransitionID:    "t1",
				WorkstationName: "codex-worker",
				StartTime:       now.Add(-2 * time.Second),
				ConsumedTokens: []factorytoken.Token{{
					ID:      "tok-1",
					PlaceID: "task:init",
					Color: factorytoken.Color{
						WorkID:     "work-1",
						WorkTypeID: "task",
						Tags:       map[string]string{"owner": "dispatcher"},
					},
				}},
			},
		},
		Results: []workerexecution.WorkResult{{
			DispatchID:   "d-1",
			TransitionID: "t1",
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       "done",
			ProviderSession: &workerexecution.ProviderSessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess_codex_123",
			},
		}},
	}

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %d, want 1", len(result.CompletedDispatches))
	}
	completed := result.CompletedDispatches[0]
	if completed.ProviderSession == nil {
		t.Fatal("expected provider session metadata on completed dispatch")
	}
	if completed.ProviderSession.ID != "sess_codex_123" {
		t.Fatalf("provider session id = %q, want %q", completed.ProviderSession.ID, "sess_codex_123")
	}
	if len(completed.OutputMutations) != 1 || completed.OutputMutations[0].Token == nil {
		t.Fatalf("completed output mutations = %#v, want one cloned output token", completed.OutputMutations)
	}

	snapshot.Results[0].ProviderSession.ID = "mutated-session"
	snapshot.Dispatches["d-1"].ConsumedTokens[0].Color.Tags["owner"] = "mutated"
	result.Mutations[0].NewToken.Color.Payload[0] = 'X'

	if completed.ProviderSession.ID != "sess_codex_123" {
		t.Fatalf("completed provider session id = %q, want detached original", completed.ProviderSession.ID)
	}
	if completed.ConsumedTokens[0].Color.Tags["owner"] != "dispatcher" {
		t.Fatalf("completed consumed token tags = %#v, want detached original", completed.ConsumedTokens[0].Color.Tags)
	}
	if string(completed.OutputMutations[0].Token.Color.Payload) != "done" {
		t.Fatalf("completed output mutation payload = %q, want detached original", completed.OutputMutations[0].Token.Color.Payload)
	}
}

func workerBatchTestNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init":      {ID: "task:init", TypeID: "task", State: "init"},
			"task:complete":  {ID: "task:complete", TypeID: "task", State: "complete"},
			"task:failed":    {ID: "task:failed", TypeID: "task", State: "failed"},
			"child:init":     {ID: "child:init", TypeID: "child", State: "init"},
			"child:complete": {ID: "child:complete", TypeID: "child", State: "complete"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
			"child": {
				ID: "child",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID: "t1",
				OutputArcs: []petri.Arc{
					{ID: "accepted", PlaceID: "task:complete"},
				},
				FailureArcs: []petri.Arc{
					{ID: "failed", PlaceID: "task:failed"},
				},
			},
		},
	}
}

func workerBatchSnapshot(output string) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch-1": {
				DispatchID:   "dispatch-1",
				TransitionID: "t1",
				ConsumedTokens: []factorytoken.Token{{
					ID:        "tok-source",
					PlaceID:   "task:init",
					CreatedAt: time.Date(2026, time.April, 16, 21, 0, 0, 0, time.UTC),
					Color: factorytoken.Color{
						Name:       "source",
						RequestID:  "request-source",
						WorkID:     "work-source",
						WorkTypeID: "task",
						DataType:   factorytoken.DataTypeWork,
						TraceID:    "trace-source",
						Tags:       map[string]string{"tenant": "port"},
					},
				}},
			},
		},
		Results: []workerexecution.WorkResult{{
			DispatchID:   "dispatch-1",
			TransitionID: "t1",
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       output,
		}},
	}
}
func TestHistoryTransitionerPipeline_ProcessAcceptPreservesSiblingLaneReviewInit(t *testing.T) {
	const traceID = "trace-process-pipeline-sibling"
	now := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)

	marking := petri.NewMarking("process-reconcile-pipeline-sibling")
	marking.AddToken(&factorytoken.Token{
		ID: "review-lane-a-old", PlaceID: "review:init", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-review-a-old", WorkTypeID: "review", Name: "lane-a",
			CurrentChainingTraceID: traceID,
		},
	})
	marking.AddToken(&factorytoken.Token{
		ID: "review-lane-b-sibling", PlaceID: "review:init", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-review-b", WorkTypeID: "review", Name: "lane-b",
			CurrentChainingTraceID: traceID,
		},
	})

	taskToken := factorytoken.Token{
		ID: "task-init", PlaceID: "task:init", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-task-a", WorkTypeID: "task", Name: "lane-a",
			CurrentChainingTraceID: traceID,
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: marking.Snapshot(),
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-process": {
				DispatchID:     "d-process",
				TransitionID:   "process",
				ConsumedTokens: []factorytoken.Token{taskToken},
			},
		},
		Results: []workerexecution.WorkResult{{
			DispatchID:   "d-process",
			TransitionID: "process",
			Outcome:      workerexecution.OutcomeAccepted,
		}},
	}

	tp := newTestPipeline(buildProcessReconcilePipelineNet())
	tp.transitioner.now = func() time.Time { return now }

	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil {
		t.Fatal("expected tick result")
	}

	consumed := mutationTokenIDs(result.Mutations)
	if !consumed["review-lane-a-old"] {
		t.Fatalf("missing reconcile consume for same-lane review in %#v", result.Mutations)
	}
	if consumed["review-lane-b-sibling"] {
		t.Fatalf("consumed sibling lane review:init on same trace: %#v", consumed)
	}
}

func TestHistoryTransitionerPipeline_ProcessAcceptReconcilesDuplicateReviewInit(t *testing.T) {
	const (
		traceID  = "trace-process-pipeline"
		laneName = "lane-process-reconcile"
	)
	now := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)

	marking := petri.NewMarking("process-reconcile-pipeline")
	marking.AddToken(&factorytoken.Token{
		ID: "review-old-1", PlaceID: "review:init", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-review-old-1", WorkTypeID: "review", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	})
	marking.AddToken(&factorytoken.Token{
		ID: "review-old-2", PlaceID: "review:init", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-review-old-2", WorkTypeID: "review", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	})

	taskToken := factorytoken.Token{
		ID: "task-init", PlaceID: "task:init", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-task-1", WorkTypeID: "task", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: marking.Snapshot(),
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-process": {
				DispatchID:     "d-process",
				TransitionID:   "process",
				ConsumedTokens: []factorytoken.Token{taskToken},
			},
		},
		Results: []workerexecution.WorkResult{{
			DispatchID:   "d-process",
			TransitionID: "process",
			Outcome:      workerexecution.OutcomeAccepted,
		}},
	}

	tp := newTestPipeline(buildProcessReconcilePipelineNet())
	tp.transitioner.now = func() time.Time { return now }

	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil {
		t.Fatal("expected tick result")
	}

	consumed := mutationTokenIDs(result.Mutations)
	for _, id := range []string{"review-old-1", "review-old-2"} {
		if !consumed[id] {
			t.Fatalf("missing reconcile consume for %q in %#v", id, result.Mutations)
		}
	}
}

func TestHistoryTransitionerPipeline_ReviewAcceptReconcilesDuplicateReviewAndStaleTask(t *testing.T) {
	const (
		traceID  = "trace-review-pipeline"
		laneName = "lane-reconcile"
	)
	now := time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC)

	marking := petri.NewMarking("review-reconcile-pipeline")
	marking.AddToken(&factorytoken.Token{
		ID: "review-extra", PlaceID: "review:init", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-review-extra", WorkTypeID: "review", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	})
	marking.AddToken(&factorytoken.Token{
		ID: "task-stale-init", PlaceID: "task:init", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-task-stale-init", WorkTypeID: "task", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	})
	marking.AddToken(&factorytoken.Token{
		ID: "task-stale-failed", PlaceID: "task:failed", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-task-stale-failed", WorkTypeID: "task", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	})

	taskToken := factorytoken.Token{
		ID: "task-in-review", PlaceID: "task:in-review", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-task-1", WorkTypeID: "task", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	}
	reviewToken := factorytoken.Token{
		ID: "review-active", PlaceID: "review:init", CreatedAt: now, EnteredAt: now,
		Color: factorytoken.Color{
			WorkID: "work-review-active", WorkTypeID: "review", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: marking.Snapshot(),
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-review": {
				DispatchID:     "d-review",
				TransitionID:   "review",
				ConsumedTokens: []factorytoken.Token{taskToken, reviewToken},
			},
		},
		Results: []workerexecution.WorkResult{{
			DispatchID:   "d-review",
			TransitionID: "review",
			Outcome:      workerexecution.OutcomeAccepted,
		}},
	}

	tp := newTestPipeline(buildReviewReconcilePipelineNet())
	tp.transitioner.now = func() time.Time { return now }

	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil {
		t.Fatal("expected tick result")
	}

	consumed := mutationTokenIDs(result.Mutations)
	for _, id := range []string{"review-extra", "task-stale-init", "task-stale-failed"} {
		if !consumed[id] {
			t.Fatalf("missing reconcile consume for %q in %#v", id, result.Mutations)
		}
	}
	if consumed["review-active"] || consumed["task-in-review"] {
		t.Fatalf("consumed active dispatch tokens: %#v", consumed)
	}
}

func buildProcessReconcilePipelineNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init":       {ID: "task:init", TypeID: "task", State: "init"},
			"task:in-review":  {ID: "task:in-review", TypeID: "task", State: "in-review"},
			"task:failed":     {ID: "task:failed", TypeID: "task", State: "failed"},
			"review:init":     {ID: "review:init", TypeID: "review", State: "init"},
			"review:complete": {ID: "review:complete", TypeID: "review", State: "complete"},
		},
		Transitions: map[string]*petri.Transition{
			"process": {
				ID:         "process",
				Name:       "process",
				WorkerType: "processor",
				InputArcs: []petri.Arc{
					{ID: "task-in", Name: "task", PlaceID: "task:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "task-out", Name: "task", PlaceID: "task:in-review", Direction: petri.ArcOutput},
					{ID: "review-out", Name: "review", PlaceID: "review:init", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "in-review", Category: state.StateCategoryProcessing},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
			"review": {
				ID: "review",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
	}
}

func buildReviewReconcilePipelineNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init":        {ID: "task:init", TypeID: "task", State: "init"},
			"task:in-review":   {ID: "task:in-review", TypeID: "task", State: "in-review"},
			"task:to-complete": {ID: "task:to-complete", TypeID: "task", State: "to-complete"},
			"task:failed":      {ID: "task:failed", TypeID: "task", State: "failed"},
			"review:init":      {ID: "review:init", TypeID: "review", State: "init"},
			"review:complete":  {ID: "review:complete", TypeID: "review", State: "complete"},
		},
		Transitions: map[string]*petri.Transition{
			"review": {
				ID:         "review",
				Name:       "review",
				WorkerType: "processor",
				InputArcs: []petri.Arc{
					{ID: "task-in", Name: "task", PlaceID: "task:in-review", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "review-in", Name: "review", PlaceID: "review:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "task-out", Name: "task", PlaceID: "task:to-complete", Direction: petri.ArcOutput},
					{ID: "review-out", Name: "review", PlaceID: "review:complete", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "in-review", Category: state.StateCategoryProcessing},
					{Value: "to-complete", Category: state.StateCategoryProcessing},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
			"review": {
				ID: "review",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
	}
}
