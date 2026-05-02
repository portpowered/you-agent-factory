package subsystems

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
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
		outcome   interfaces.WorkOutcome
		wantArcID string
		wantErr   bool
	}{
		{name: "Accepted_ReturnsOutputArcs", outcome: interfaces.OutcomeAccepted, wantArcID: "accepted-arc"},
		{name: "Continue_ReturnsContinueArcs", outcome: interfaces.OutcomeContinue, wantArcID: "continue-arc"},
		{name: "Rejected_ReturnsRejectionArcs", outcome: interfaces.OutcomeRejected, wantArcID: "rejected-arc"},
		{name: "Failed_ReturnsFailureArcs", outcome: interfaces.OutcomeFailed, wantArcID: "failed-arc"},
		{name: "UnknownOutcome_ReturnsError", outcome: interfaces.WorkOutcome("UNKNOWN"), wantErr: true},
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
	now := time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	baseHistory := interfaces.TokenHistory{
		TotalVisits:         map[string]int{"t0": 1},
		ConsecutiveFailures: map[string]int{},
		PlaceVisits:         map[string]int{"wt-code:init": 1},
	}
	places := map[string]*petri.Place{
		"wt-code:init":   {ID: "wt-code:init", TypeID: "wt-code", State: "init"},
		"wt-code:done":   {ID: "wt-code:done", TypeID: "wt-code", State: "done"},
		"wt-code:failed": {ID: "wt-code:failed", TypeID: "wt-code", State: "failed"},
		"wt-review:init": {ID: "wt-review:init", TypeID: "wt-review", State: "init"},
	}
	workTypes := map[string]*state.WorkType{
		"wt-code":   {ID: "wt-code"},
		"wt-review": {ID: "wt-review"},
	}
	consumed := []interfaces.Token{{
		ID:        "tok-1",
		PlaceID:   "wt-code:init",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: interfaces.TokenColor{
			WorkID:     "w1",
			WorkTypeID: "wt-code",
			Name:       "story-1",
		},
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}}
	inputColors := tokenColorsFromTokens(consumed)
	transition := &petri.Transition{ID: "t1"}

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
				outcome:      interfaces.OutcomeAccepted,
				output:       "compiled",
			},
			wantPlace:      "wt-code:done",
			wantWorkTypeID: "wt-code",
			wantWorkID:     "w1",
			wantPayload:    []byte("compiled"),
			wantCreatedAt:  createdAt,
		},
		{
			name: "Rejected_AddsRejectionFeedbackTag",
			arcs: []petri.Arc{{ID: "reject", PlaceID: "wt-code:init"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      interfaces.OutcomeRejected,
				feedback:     "try again",
			},
			wantPlace:      "wt-code:init",
			wantWorkTypeID: "wt-code",
			wantWorkID:     "w1",
			wantFeedback:   "try again",
			wantCreatedAt:  createdAt,
		},
		{
			name: "Failed_AppendsFailureHistory",
			arcs: []petri.Arc{{ID: "fail", PlaceID: "wt-code:failed"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      interfaces.OutcomeFailed,
				err:          "agent crashed",
			},
			wantPlace:       "wt-code:failed",
			wantWorkTypeID:  "wt-code",
			wantWorkID:      "w1",
			wantLastError:   "agent crashed",
			wantFailureSize: 1,
			wantCreatedAt:   createdAt,
		},
		{
			name: "AcceptedCrossType_GeneratesNewWorkID",
			arcs: []petri.Arc{{ID: "cross", PlaceID: "wt-review:init"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      interfaces.OutcomeAccepted,
			},
			wantPlace:      "wt-review:init",
			wantWorkTypeID: "wt-review",
			wantWorkID:     "work-wt-review-1",
			wantCreatedAt:  now,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutations, err := calculateMutations(mutationCalculationInput{
				transition:  transition,
				arcs:        tt.arcs,
				consumed:    consumed,
				result:      tt.result,
				now:         now,
				history:     baseHistory,
				inputColors: inputColors,
				transformer: token_transformer.New(places, workTypes, token_transformer.WithWorkIDGenerator(petri.NewWorkIDGenerator())),
			})
			if err != nil {
				t.Fatalf("calculateMutations() error = %v", err)
			}
			if len(mutations) != 1 {
				t.Fatalf("expected 1 mutation, got %d", len(mutations))
			}

			token := mutations[0].NewToken
			if mutations[0].ToPlace != tt.wantPlace {
				t.Fatalf("ToPlace = %s, want %s", mutations[0].ToPlace, tt.wantPlace)
			}
			if token.Color.WorkTypeID != tt.wantWorkTypeID {
				t.Fatalf("WorkTypeID = %s, want %s", token.Color.WorkTypeID, tt.wantWorkTypeID)
			}
			if token.Color.WorkID != tt.wantWorkID {
				t.Fatalf("WorkID = %s, want %s", token.Color.WorkID, tt.wantWorkID)
			}
			if !token.CreatedAt.Equal(tt.wantCreatedAt) {
				t.Fatalf("CreatedAt = %v, want %v", token.CreatedAt, tt.wantCreatedAt)
			}
			if !token.EnteredAt.Equal(now) {
				t.Fatalf("EnteredAt = %v, want %v", token.EnteredAt, now)
			}
			if !bytes.Equal(token.Color.Payload, tt.wantPayload) {
				t.Fatalf("Payload = %q, want %q", token.Color.Payload, tt.wantPayload)
			}
			if got := token.Color.Tags[interfaces.RejectionFeedback]; got != tt.wantFeedback {
				t.Fatalf("rejection feedback = %q, want %q", got, tt.wantFeedback)
			}
			if token.History.LastError != tt.wantLastError {
				t.Fatalf("LastError = %q, want %q", token.History.LastError, tt.wantLastError)
			}
			if len(token.History.FailureLog) != tt.wantFailureSize {
				t.Fatalf("FailureLog length = %d, want %d", len(token.History.FailureLog), tt.wantFailureSize)
			}
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
	consumed := []interfaces.Token{{
		ID:      "tok-1",
		PlaceID: "wt-code:init",
		Color: interfaces.TokenColor{
			WorkID:     "work-code-1",
			WorkTypeID: "wt-code",
			Name:       "story-1",
			TraceID:    "trace-1",
		},
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		History: interfaces.TokenHistory{
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
			outcome:      interfaces.OutcomeAccepted,
			recordedOutputWork: []interfaces.FactoryWorkItem{{
				ID:                       "work-review-99",
				WorkTypeID:               "wt-review",
				DisplayName:              "review-override",
				CurrentChainingTraceID:   "trace-replay",
				PreviousChainingTraceIDs: []string{"trace-parent"},
				TraceID:                  "trace-replay",
				Tags:                     map[string]string{"source": "recording"},
			}},
		},
		now:         now,
		history:     interfaces.TokenHistory{},
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
	if got := token.Color.Tags["source"]; got != "recording" {
		t.Fatalf("recorded output tags = %#v, want source=recording", token.Color.Tags)
	}
}

func TestResolveWorkResult_RuntimeConfigStopWordsAcceptConfiguredMarker(t *testing.T) {
	transition := &petri.Transition{
		ID:   "transition-id",
		Name: "runtime-station",
	}
	result := &interfaces.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-id",
		Outcome:      interfaces.OutcomeRejected,
		Output:       "rendered output DONE",
	}

	resolved := resolveWorkResult(transition, result, runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"runtime-station": {StopWords: []string{"DONE"}},
		},
	})

	if resolved.outcome != interfaces.OutcomeAccepted {
		t.Fatalf("resolved outcome = %s, want ACCEPTED", resolved.outcome)
	}
}

func TestResolveWorkResult_RuntimeConfigStopWordsFailMissingMarker(t *testing.T) {
	transition := &petri.Transition{
		ID:   "transition-id",
		Name: "runtime-station",
	}
	result := &interfaces.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-id",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "rendered output without marker",
	}

	resolved := resolveWorkResult(transition, result, runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"runtime-station": {StopWords: []string{"DONE"}},
		},
	})

	if resolved.outcome != interfaces.OutcomeFailed {
		t.Fatalf("resolved outcome = %s, want FAILED", resolved.outcome)
	}
}

func TestResolveWorkResult_MissingRuntimeConfigPreservesOriginalOutcome(t *testing.T) {
	transition := &petri.Transition{
		ID: "runtime-station-id",
	}
	result := &interfaces.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "runtime-station-id",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "rendered output without marker",
	}

	resolved := resolveWorkResult(transition, result, nil)

	if resolved.outcome != interfaces.OutcomeAccepted {
		t.Fatalf("resolved outcome = %s, want original ACCEPTED when runtime config is missing", resolved.outcome)
	}
}

func TestResolveWorkResult_RuntimeConfigFallsBackToTransitionID(t *testing.T) {
	transition := &petri.Transition{
		ID: "runtime-station-id",
	}
	result := &interfaces.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "runtime-station-id",
		Outcome:      interfaces.OutcomeRejected,
		Output:       "rendered output DONE",
	}

	resolved := resolveWorkResult(transition, result, runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"runtime-station-id": {StopWords: []string{"DONE"}},
		},
	})

	if resolved.outcome != interfaces.OutcomeAccepted {
		t.Fatalf("resolved outcome = %s, want ACCEPTED from transition ID fallback", resolved.outcome)
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
				ConsumedTokens: []interfaces.Token{{
					ID:      "tok-1",
					PlaceID: "task:init",
					Color: interfaces.TokenColor{
						WorkID:     "work-1",
						WorkTypeID: "task",
					},
				}},
			},
		},
		Results: []interfaces.WorkResult{{
			DispatchID:   "d-1",
			TransitionID: "t1",
			Outcome:      interfaces.OutcomeAccepted,
			Output:       "done",
			ProviderSession: &interfaces.ProviderSessionMetadata{
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
}

func TestTransitioner_WorkerEmittedGeneratedSubmissionBatchCreatesGeneratedWork(t *testing.T) {
	now := time.Date(2026, time.April, 16, 22, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	output := `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"child","tags":{"priority":"high"}},{"name":"review","workTypeName":"child"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"review","targetWorkName":"draft"}]}}`
	result := executeWorkerBatchTransition(t, transitioner, workerBatchSnapshot(output))
	batch, requestID := assertGeneratedWorkerBatchMetadata(t, result)
	normalized := normalizeGeneratedWorkerBatch(t, batch)
	first := assertGeneratedWorkerBatchSubmissions(t, requestID, batch.Metadata.Source, normalized)
	assertRepeatedGeneratedWorkerBatchRequestID(t, transitioner, output, requestID)
	assertGeneratedWorkerBatchOutcome(t, result, first, normalized[1])
}

func executeWorkerBatchTransition(
	t *testing.T,
	transitioner *TransitionerSubsystem,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) *interfaces.TickResult {
	t.Helper()

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}
	return result
}

func assertGeneratedWorkerBatchMetadata(t *testing.T, result *interfaces.TickResult) (interfaces.GeneratedSubmissionBatch, string) {
	t.Helper()

	if len(result.Mutations) != 0 {
		t.Fatalf("mutation count = %d, want engine-owned generated work only", len(result.Mutations))
	}
	if len(result.GeneratedBatches) != 1 {
		t.Fatalf("generated batches = %d, want 1", len(result.GeneratedBatches))
	}
	batch := result.GeneratedBatches[0]
	requestID := batch.Request.RequestID
	if requestID == "" {
		t.Fatal("expected deterministic generated request ID")
	}
	if batch.Metadata.Source != "worker-output:dispatch-1" {
		t.Fatalf("batch source = %q, want worker-output:dispatch-1", batch.Metadata.Source)
	}
	return batch, requestID
}

func normalizeGeneratedWorkerBatch(t *testing.T, batch interfaces.GeneratedSubmissionBatch) []interfaces.SubmitRequest {
	t.Helper()

	normalized, err := factory.NormalizeGeneratedSubmissionBatch(batch, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true, "child": true},
	})
	if err != nil {
		t.Fatalf("NormalizeGeneratedSubmissionBatch: %v", err)
	}
	return normalized
}

func assertGeneratedWorkerBatchSubmissions(
	t *testing.T,
	requestID string,
	source string,
	normalized []interfaces.SubmitRequest,
) interfaces.SubmitRequest {
	t.Helper()

	record := factory.WorkRequestRecordFromSubmitRequests(requestID, source, normalized)
	if len(record.Relations) != 1 {
		t.Fatalf("request relation count = %d, want 1", len(record.Relations))
	}
	if len(normalized) != 2 {
		t.Fatalf("normalized submissions = %d, want 2", len(normalized))
	}

	first := normalized[0]
	if first.RequestID != requestID {
		t.Fatalf("generated request ID = %q, want %q", first.RequestID, requestID)
	}
	if first.TraceID != "trace-source" || first.CurrentChainingTraceID != "trace-source" {
		t.Fatalf("first generated trace fields = %#v, want trace-source", first)
	}
	if len(first.PreviousChainingTraceIDs) != 1 || first.PreviousChainingTraceIDs[0] != "trace-source" {
		t.Fatalf("generated previous chaining trace IDs = %#v, want [trace-source]", first.PreviousChainingTraceIDs)
	}
	if first.Tags["tenant"] != "port" || first.Tags["priority"] != "high" {
		t.Fatalf("generated tags = %#v, want source and item tags", first.Tags)
	}
	if first.Tags["_parent_work_id"] != "work-source" || first.Tags["_parent_request_id"] != "request-source" {
		t.Fatalf("generated lineage tags = %#v", first.Tags)
	}
	if first.Tags["_source_dispatch_id"] != "dispatch-1" || first.Tags["_source_transition_id"] != "t1" {
		t.Fatalf("generated execution tags = %#v", first.Tags)
	}
	return first
}

func assertRepeatedGeneratedWorkerBatchRequestID(
	t *testing.T,
	transitioner *TransitionerSubsystem,
	output string,
	requestID string,
) {
	t.Helper()

	repeated := executeWorkerBatchTransition(t, transitioner, workerBatchSnapshot(output))
	if repeated.GeneratedBatches[0].Request.RequestID != requestID {
		t.Fatalf("generated request ID = %q, want deterministic %q", repeated.GeneratedBatches[0].Request.RequestID, requestID)
	}
}

func assertGeneratedWorkerBatchOutcome(
	t *testing.T,
	result *interfaces.TickResult,
	first interfaces.SubmitRequest,
	second interfaces.SubmitRequest,
) {
	t.Helper()

	if second.CurrentChainingTraceID != "trace-source" {
		t.Fatalf("second generated current chaining trace ID = %q, want trace-source", second.CurrentChainingTraceID)
	}
	if len(second.PreviousChainingTraceIDs) != 1 || second.PreviousChainingTraceIDs[0] != "trace-source" {
		t.Fatalf("second generated previous chaining trace IDs = %#v, want [trace-source]", second.PreviousChainingTraceIDs)
	}
	if len(second.Relations) != 1 || second.Relations[0].TargetWorkID != first.WorkID {
		t.Fatalf("generated dependency relation = %#v, want target %q", second.Relations, first.WorkID)
	}
	if result.CompletedDispatches[0].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("completed outcome = %s, want ACCEPTED", result.CompletedDispatches[0].Outcome)
	}
}

func TestTransitioner_WorkerEmittedFactoryRequestBatchReleasesConsumedResources(t *testing.T) {
	now := time.Date(2026, time.April, 17, 0, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	net.Places["agent-slot:available"] = &petri.Place{ID: "agent-slot:available", TypeID: "agent-slot", State: "available"}
	net.Resources = map[string]*state.ResourceDef{
		"agent-slot": {ID: "agent-slot", Capacity: 1},
	}
	net.Transitions["t1"].InputArcs = []petri.Arc{
		{ID: "task-in", PlaceID: "task:init"},
		{ID: "slot-in", PlaceID: "agent-slot:available"},
	}
	net.Transitions["t1"].OutputArcs = []petri.Arc{
		{ID: "accepted", PlaceID: "task:complete"},
		{ID: "slot-out", PlaceID: "agent-slot:available"},
	}
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	output := `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"follow-up","workTypeName":"child"}]}}`
	snapshot := workerBatchSnapshot(output)
	snapshot.Dispatches["dispatch-1"].ConsumedTokens = append(snapshot.Dispatches["dispatch-1"].ConsumedTokens, interfaces.Token{
		ID:        "agent-slot:resource:0",
		PlaceID:   "agent-slot:available",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		Color: interfaces.TokenColor{
			WorkID:     "agent-slot:0",
			WorkTypeID: "agent-slot",
			DataType:   interfaces.DataTypeResource,
		},
	})

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}

	var generatedWork int
	var releasedResource *interfaces.Token
	for i := range result.Mutations {
		mutation := result.Mutations[i]
		if mutation.NewToken == nil {
			continue
		}
		switch mutation.NewToken.Color.DataType {
		case interfaces.DataTypeWork:
			if mutation.ToPlace == "child:init" {
				generatedWork++
			}
			if mutation.ToPlace == "task:complete" {
				t.Fatalf("worker-emitted batch should replace normal accepted work output, got mutation %#v", mutation)
			}
		case interfaces.DataTypeResource:
			if mutation.ToPlace == "agent-slot:available" {
				releasedResource = mutation.NewToken
			}
		}
	}
	if generatedWork != 0 {
		t.Fatalf("generated work mutations = %d, want engine-owned generated work", generatedWork)
	}
	if releasedResource == nil {
		t.Fatalf("resource release mutation missing from %#v", result.Mutations)
	}
	if releasedResource.ID != "agent-slot:resource:0" {
		t.Fatalf("released resource token ID = %q, want consumed token identity", releasedResource.ID)
	}
	if !releasedResource.EnteredAt.Equal(now) {
		t.Fatalf("released resource EnteredAt = %v, want %v", releasedResource.EnteredAt, now)
	}
	if len(result.GeneratedBatches) != 1 {
		t.Fatalf("generated batches = %d, want 1", len(result.GeneratedBatches))
	}
}

func TestTransitioner_RawWorkerEmittedFactoryRequestBatchRoutesAsAcceptedOutput(t *testing.T) {
	now := time.Date(2026, time.April, 18, 1, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	output := `{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"raw","work_type_name":"child"}]}`
	snapshot := workerBatchSnapshot(output)

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}
	if len(result.GeneratedBatches) != 0 {
		t.Fatalf("generated batches = %d, want raw worker output treated as ordinary output", len(result.GeneratedBatches))
	}
	if len(result.Mutations) != 1 || result.Mutations[0].ToPlace != "task:complete" {
		t.Fatalf("mutations = %#v, want ordinary accepted-output mutation", result.Mutations)
	}
	token := result.Mutations[0].NewToken
	if token == nil {
		t.Fatal("accepted output mutation missing token")
	}
	if string(token.Color.Payload) != output {
		t.Fatalf("accepted output payload = %q, want raw JSON", token.Color.Payload)
	}
	if result.CompletedDispatches[0].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("completed outcome = %s, want ACCEPTED", result.CompletedDispatches[0].Outcome)
	}
}

func TestTransitioner_WorkerEmittedGeneratedSubmissionBatchUsesBatchMetadataSource(t *testing.T) {
	now := time.Date(2026, time.April, 18, 0, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	output := `{"request":{"requestId":"metadata-request","type":"FACTORY_REQUEST_BATCH","works":[{"name":"generated","workId":"work-generated","workTypeName":"child","payload":"generated"}]},"metadata":{"source":"generator:unit-test","parentLineage":["request-parent","work-parent"]},"submissions":[{"name":"generated","workId":"work-generated","targetState":"complete","executionId":"exec-child","tags":{"runtime":"true"}}]}`
	snapshot := workerBatchSnapshot(output)

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}
	if len(result.GeneratedBatches) != 1 {
		t.Fatalf("generated batches = %d, want 1", len(result.GeneratedBatches))
	}
	batch := result.GeneratedBatches[0]
	if batch.Metadata.Source != "generator:unit-test" {
		t.Fatalf("batch source = %q, want generator:unit-test", batch.Metadata.Source)
	}
	if batch.Request.RequestID != "metadata-request" {
		t.Fatalf("work request id = %q, want metadata-request", batch.Request.RequestID)
	}
	if len(batch.Metadata.ParentLineage) != 2 {
		t.Fatalf("parent lineage = %#v, want metadata preserved", batch.Metadata.ParentLineage)
	}
	normalized, err := factory.NormalizeGeneratedSubmissionBatch(batch, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true, "child": true},
	})
	if err != nil {
		t.Fatalf("NormalizeGeneratedSubmissionBatch: %v", err)
	}
	if len(normalized) != 1 {
		t.Fatalf("normalized submissions = %d, want 1", len(normalized))
	}
	if normalized[0].TargetState != "complete" {
		t.Fatalf("work input target state = %q, want complete", normalized[0].TargetState)
	}
	if normalized[0].ExecutionID != "exec-child" {
		t.Fatalf("work input execution id = %q, want exec-child", normalized[0].ExecutionID)
	}
	if got := normalized[0].Tags["runtime"]; got != "true" {
		t.Fatalf("runtime tag = %q, want true", got)
	}
}

func TestTransitioner_MalformedWorkerEmittedFactoryRequestBatchFailsDispatch(t *testing.T) {
	now := time.Date(2026, time.April, 16, 22, 5, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	snapshot := workerBatchSnapshot(`{"request":{"requestId":"bad-request","type":"FACTORY_REQUEST_BATCH","works":[]}}`)

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want 1", result)
	}
	completed := result.CompletedDispatches[0]
	if completed.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("completed outcome = %s, want FAILED", completed.Outcome)
	}
	if !strings.Contains(completed.Reason, "worker-emitted work request batch") {
		t.Fatalf("completed reason = %q, want worker-emitted validation prefix", completed.Reason)
	}
	if !strings.Contains(completed.Reason, "works array must contain at least one item") {
		t.Fatalf("completed reason = %q, want validation failure", completed.Reason)
	}
	if len(result.Mutations) != 1 || result.Mutations[0].ToPlace != "task:failed" {
		t.Fatalf("failure mutations = %#v, want failed arc", result.Mutations)
	}
	if len(result.GeneratedBatches) != 0 {
		t.Fatalf("generated batches = %#v, want none", result.GeneratedBatches)
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
				ConsumedTokens: []interfaces.Token{{
					ID:        "tok-source",
					PlaceID:   "task:init",
					CreatedAt: time.Date(2026, time.April, 16, 21, 0, 0, 0, time.UTC),
					Color: interfaces.TokenColor{
						Name:       "source",
						RequestID:  "request-source",
						WorkID:     "work-source",
						WorkTypeID: "task",
						DataType:   interfaces.DataTypeWork,
						TraceID:    "trace-source",
						Tags:       map[string]string{"tenant": "port"},
					},
				}},
			},
		},
		Results: []interfaces.WorkResult{{
			DispatchID:   "dispatch-1",
			TransitionID: "t1",
			Outcome:      interfaces.OutcomeAccepted,
			Output:       output,
		}},
	}
}
