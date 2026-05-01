package subsystems

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/buffers"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type testPipeline struct {
	transitioner *TransitionerSubsystem
	results      *buffers.TypedBuffer[interfaces.WorkResult]
}

func newTestPipeline(n *state.Net) *testPipeline {
	return &testPipeline{
		transitioner: NewTransitioner(n, nil),
		results:      buffers.NewTypedBuffer[interfaces.WorkResult](16),
	}
}

func (tp *testPipeline) WriteResult(r interfaces.WorkResult) {
	tp.results.Write(context.Background(), r)
}

func (tp *testPipeline) Execute(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	snap := *snapshot
	for tp.results.HasData() {
		result, ok := tp.results.Read()
		if !ok {
			break
		}
		snap.Results = append(snap.Results, result)
	}
	return tp.transitioner.Execute(ctx, &snap)
}

func TestHistoryTransitionerPipeline_AcceptedRoutesUsingConsumedDispatchTokens(t *testing.T) {
	n := buildPipelineNet()
	tp := newTestPipeline(n)
	tp.WriteResult(interfaces.WorkResult{DispatchID: "d-1", TransitionID: "t1", Outcome: interfaces.OutcomeAccepted})

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		interfaces.TokenColor{WorkID: "w1", WorkTypeID: "wt-code"},
		time.Time{},
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	if result.Mutations[0].ToPlace != "wt-code:done" {
		t.Fatalf("ToPlace = %s, want wt-code:done", result.Mutations[0].ToPlace)
	}
	if result.Mutations[0].NewToken.Color.WorkID != "w1" {
		t.Fatalf("WorkID = %s, want w1", result.Mutations[0].NewToken.Color.WorkID)
	}
}

func TestHistoryTransitionerPipeline_FailedRoutesUsingConsumedDispatchTokens(t *testing.T) {
	n := buildPipelineNet()
	tp := newTestPipeline(n)
	tp.WriteResult(interfaces.WorkResult{DispatchID: "d-1", TransitionID: "t1", Outcome: interfaces.OutcomeFailed, Error: "agent crashed"})

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		interfaces.TokenColor{WorkID: "w1", WorkTypeID: "wt-code"},
		time.Time{},
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	if result.Mutations[0].ToPlace != "wt-code:failed" {
		t.Fatalf("ToPlace = %s, want wt-code:failed", result.Mutations[0].ToPlace)
	}
}

func TestHistoryTransitionerPipeline_FailedWithoutFailureArcs_UsesConsumedDispatchTokensForFallback(t *testing.T) {
	n := buildPipelineNet()
	n.Transitions["t1"].FailureArcs = nil
	state.NormalizeTransitionTopology(n)
	tp := newTestPipeline(n)
	tp.WriteResult(interfaces.WorkResult{DispatchID: "d-1", TransitionID: "t1", Outcome: interfaces.OutcomeFailed, Error: "agent crashed"})
	createdAt := time.Date(2026, time.April, 6, 9, 0, 0, 0, time.UTC)

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		interfaces.TokenColor{WorkID: "w-fallback", WorkTypeID: "wt-code"},
		createdAt,
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	if result.Mutations[0].ToPlace != "wt-code:failed" {
		t.Fatalf("ToPlace = %s, want wt-code:failed", result.Mutations[0].ToPlace)
	}
	if result.Mutations[0].NewToken.Color.WorkID != "w-fallback" {
		t.Fatalf("WorkID = %s, want w-fallback", result.Mutations[0].NewToken.Color.WorkID)
	}
	if !result.Mutations[0].NewToken.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", result.Mutations[0].NewToken.CreatedAt, createdAt)
	}
}

func TestHistoryTransitionerPipeline_RepeaterRejectedReturnsToInputPlace(t *testing.T) {
	n := buildRepeaterPipelineNet()
	tp := newTestPipeline(n)
	tp.WriteResult(interfaces.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeRejected,
		Feedback:     "try again",
	})
	createdAt := time.Date(2026, time.April, 6, 10, 0, 0, 0, time.UTC)

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		interfaces.TokenColor{WorkID: "w1", WorkTypeID: "wt-code"},
		createdAt,
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	if result.Mutations[0].ToPlace != "wt-code:init" {
		t.Fatalf("ToPlace = %s, want wt-code:init", result.Mutations[0].ToPlace)
	}
	if result.Mutations[0].NewToken.Color.Tags["_rejection_feedback"] != "try again" {
		t.Fatalf("missing rejection feedback tag")
	}
	if !result.Mutations[0].NewToken.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", result.Mutations[0].NewToken.CreatedAt, createdAt)
	}
}

func TestHistoryTransitionerPipeline_ThrottledFailureRequeuesConsumedWorkToOriginalPlace(t *testing.T) {
	n := buildPipelineNet()
	tp := newTestPipeline(n)
	tp.WriteResult(interfaces.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeFailed,
		Error:        "provider error: claude rate limit exceeded",
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyThrottle,
			Type:   interfaces.ProviderErrorTypeThrottled,
		},
	})
	createdAt := time.Date(2026, time.April, 6, 10, 0, 0, 0, time.UTC)

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		interfaces.TokenColor{WorkID: "w-throttle", WorkTypeID: "wt-code"},
		createdAt,
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	if result.Mutations[0].ToPlace != "wt-code:init" {
		t.Fatalf("ToPlace = %s, want wt-code:init", result.Mutations[0].ToPlace)
	}
	if result.Mutations[0].NewToken.Color.WorkID != "w-throttle" {
		t.Fatalf("WorkID = %s, want w-throttle", result.Mutations[0].NewToken.Color.WorkID)
	}
	if !result.Mutations[0].NewToken.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", result.Mutations[0].NewToken.CreatedAt, createdAt)
	}
	if got := result.Mutations[0].NewToken.History.TotalVisits["t1"]; got != 1 {
		t.Fatalf("TotalVisits[t1] = %d, want 1", got)
	}
	if got := result.Mutations[0].NewToken.History.ConsecutiveFailures["t1"]; got != 1 {
		t.Fatalf("ConsecutiveFailures[t1] = %d, want 1", got)
	}
	if result.Mutations[0].NewToken.History.LastError != "provider error: claude rate limit exceeded" {
		t.Fatalf("LastError = %q", result.Mutations[0].NewToken.History.LastError)
	}
	if len(result.Mutations[0].NewToken.History.FailureLog) != 1 {
		t.Fatalf("FailureLog length = %d, want 1", len(result.Mutations[0].NewToken.History.FailureLog))
	}
	if result.Mutations[0].NewToken.History.FailureLog[0].Attempt != 1 {
		t.Fatalf("FailureLog attempt = %d, want 1", result.Mutations[0].NewToken.History.FailureLog[0].Attempt)
	}
}

func TestHistoryTransitionerPipeline_TimeoutFailureRequeuesConsumedWorkToOriginalPlace(t *testing.T) {
	n := buildPipelineNet()
	tp := newTestPipeline(n)
	tp.WriteResult(interfaces.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeFailed,
		Error:        "execution timeout",
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyRetryable,
			Type:   interfaces.ProviderErrorTypeTimeout,
		},
	})
	createdAt := time.Date(2026, time.April, 6, 10, 30, 0, 0, time.UTC)

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		interfaces.TokenColor{WorkID: "w-timeout", WorkTypeID: "wt-code"},
		createdAt,
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	if result.Mutations[0].ToPlace != "wt-code:init" {
		t.Fatalf("ToPlace = %s, want wt-code:init", result.Mutations[0].ToPlace)
	}
	if result.Mutations[0].NewToken.Color.WorkID != "w-timeout" {
		t.Fatalf("WorkID = %s, want w-timeout", result.Mutations[0].NewToken.Color.WorkID)
	}
	if !result.Mutations[0].NewToken.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", result.Mutations[0].NewToken.CreatedAt, createdAt)
	}
	if got := result.Mutations[0].NewToken.History.TotalVisits["t1"]; got != 1 {
		t.Fatalf("TotalVisits[t1] = %d, want 1", got)
	}
	if got := result.Mutations[0].NewToken.History.ConsecutiveFailures["t1"]; got != 1 {
		t.Fatalf("ConsecutiveFailures[t1] = %d, want 1", got)
	}
	if result.Mutations[0].NewToken.History.LastError != "execution timeout" {
		t.Fatalf("LastError = %q", result.Mutations[0].NewToken.History.LastError)
	}
	if len(result.Mutations[0].NewToken.History.FailureLog) != 1 {
		t.Fatalf("FailureLog length = %d, want 1", len(result.Mutations[0].NewToken.History.FailureLog))
	}
	if result.Mutations[0].NewToken.History.FailureLog[0].Attempt != 1 {
		t.Fatalf("FailureLog attempt = %d, want 1", result.Mutations[0].NewToken.History.FailureLog[0].Attempt)
	}
}

func TestHistoryTransitionerPipeline_TimeoutFailureRequeuesDespiteRenderedErrorText(t *testing.T) {
	n := buildPipelineNet()
	tp := newTestPipeline(n)
	tp.WriteResult(interfaces.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeFailed,
		Error:        "provider error: timeout: context deadline exceeded",
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyRetryable,
			Type:   interfaces.ProviderErrorTypeTimeout,
		},
	})
	createdAt := time.Date(2026, time.April, 6, 10, 45, 0, 0, time.UTC)

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		interfaces.TokenColor{WorkID: "w-timeout-rendered", WorkTypeID: "wt-code"},
		createdAt,
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	if result.Mutations[0].ToPlace != "wt-code:init" {
		t.Fatalf("ToPlace = %s, want wt-code:init", result.Mutations[0].ToPlace)
	}
	if result.Mutations[0].NewToken.Color.WorkID != "w-timeout-rendered" {
		t.Fatalf("WorkID = %s, want w-timeout-rendered", result.Mutations[0].NewToken.Color.WorkID)
	}
	if !result.Mutations[0].NewToken.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", result.Mutations[0].NewToken.CreatedAt, createdAt)
	}
	if result.Mutations[0].NewToken.History.LastError != "provider error: timeout: context deadline exceeded" {
		t.Fatalf("LastError = %q", result.Mutations[0].NewToken.History.LastError)
	}
}

func TestHistoryTransitionerPipeline_InternalServerFailureRequeuesConsumedWorkToOriginalPlace(t *testing.T) {
	n := buildPipelineNet()
	tp := newTestPipeline(n)
	tp.WriteResult(interfaces.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeFailed,
		Error:        "provider error: internal_server_error",
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyRetryable,
			Type:   interfaces.ProviderErrorTypeInternalServerError,
		},
	})

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		interfaces.TokenColor{WorkID: "w-retryable", WorkTypeID: "wt-code"},
		time.Date(2026, time.April, 6, 10, 50, 0, 0, time.UTC),
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	if result.Mutations[0].ToPlace != "wt-code:init" {
		t.Fatalf("ToPlace = %s, want wt-code:init", result.Mutations[0].ToPlace)
	}
	if result.Mutations[0].NewToken.Color.WorkID != "w-retryable" {
		t.Fatalf("WorkID = %s, want w-retryable", result.Mutations[0].NewToken.Color.WorkID)
	}
}

func TestHistoryTransitionerPipeline_InternalServerFailureRequeuesFromNormalizedTypeWhenFamilyIsMissingOrStale(t *testing.T) {
	testCases := []struct {
		name     string
		metadata *interfaces.ProviderFailureMetadata
		workID   string
	}{
		{
			name: "MissingFamily",
			metadata: &interfaces.ProviderFailureMetadata{
				Type: interfaces.ProviderErrorTypeInternalServerError,
			},
			workID: "w-retryable-missing-family",
		},
		{
			name: "StaleTerminalFamily",
			metadata: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyTerminal,
				Type:   interfaces.ProviderErrorTypeInternalServerError,
			},
			workID: "w-retryable-stale-family",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			n := buildPipelineNet()
			tp := newTestPipeline(n)
			tp.WriteResult(interfaces.WorkResult{
				DispatchID:      "d-1",
				TransitionID:    "t1",
				Outcome:         interfaces.OutcomeFailed,
				Error:           "provider error: internal_server_error",
				ProviderFailure: tc.metadata,
			})

			snapshot := pipelineSnapshot(
				"wt-code:init",
				"t1",
				"d-1",
				interfaces.TokenColor{WorkID: tc.workID, WorkTypeID: "wt-code"},
				time.Date(2026, time.April, 6, 10, 55, 0, 0, time.UTC),
			)
			result, err := tp.Execute(context.Background(), &snapshot)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == nil || len(result.Mutations) != 1 {
				t.Fatalf("expected 1 mutation, got %+v", result)
			}
			if result.Mutations[0].ToPlace != "wt-code:init" {
				t.Fatalf("ToPlace = %s, want wt-code:init", result.Mutations[0].ToPlace)
			}
			if result.Mutations[0].NewToken.Color.WorkID != tc.workID {
				t.Fatalf("WorkID = %s, want %s", result.Mutations[0].NewToken.Color.WorkID, tc.workID)
			}
		})
	}
}

func TestHistoryTransitionerPipeline_CodexWindowsExitCode4294967295RequeuesAndPreservesRetryableProviderMetadata(t *testing.T) {
	n := buildPipelineNet()
	tp := newTestPipeline(n)
	errorText := "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)"
	tp.WriteResult(interfaces.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeFailed,
		Error:        errorText,
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyRetryable,
			Type:   interfaces.ProviderErrorTypeInternalServerError,
		},
	})

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		interfaces.TokenColor{WorkID: "w-codex-windows-4294967295", WorkTypeID: "wt-code"},
		time.Date(2026, time.April, 6, 11, 5, 0, 0, time.UTC),
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	if result.Mutations[0].ToPlace != "wt-code:init" {
		t.Fatalf("ToPlace = %s, want wt-code:init", result.Mutations[0].ToPlace)
	}
	if len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatch count = %d, want 1", len(result.CompletedDispatches))
	}
	completed := result.CompletedDispatches[0]
	if completed.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("completed dispatch outcome = %q, want %q", completed.Outcome, interfaces.OutcomeFailed)
	}
	if completed.Reason != errorText {
		t.Fatalf("completed dispatch reason = %q, want %q", completed.Reason, errorText)
	}
	if completed.ProviderFailure == nil {
		t.Fatal("expected completed dispatch provider failure metadata")
	}
	if completed.ProviderFailure.Type != interfaces.ProviderErrorTypeInternalServerError {
		t.Fatalf("completed dispatch provider failure type = %q, want %q", completed.ProviderFailure.Type, interfaces.ProviderErrorTypeInternalServerError)
	}
	if completed.ProviderFailure.Family != interfaces.ProviderErrorFamilyRetryable {
		t.Fatalf("completed dispatch provider failure family = %q, want %q", completed.ProviderFailure.Family, interfaces.ProviderErrorFamilyRetryable)
	}
	decision := workers.ProviderFailureDecisionFromMetadata(completed.ProviderFailure)
	if !decision.Retryable || decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("ProviderFailureDecisionFromMetadata(%#v) = %#v, want retryable non-terminal non-throttle", completed.ProviderFailure, decision)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-resource-release-fixture review=2026-07-18 removal=split-resource-setup-and-failure-release-assertions-before-next-history-transitioner-change
func TestHistoryTransitionerPipeline_FailureReleasesConsumedResourceTokenIdentity(t *testing.T) {
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
	tp := newTestPipeline(n)
	tp.WriteResult(interfaces.WorkResult{DispatchID: "d-1", TransitionID: "t1", Outcome: interfaces.OutcomeFailed, Error: "agent crashed"})
	now := time.Date(2026, time.April, 6, 11, 0, 0, 0, time.UTC)
	resourceCreatedAt := now.Add(-3 * time.Hour)
	resourceConsumed := interfaces.Token{
		ID:        "executor:resource:0",
		PlaceID:   "executor:available",
		CreatedAt: resourceCreatedAt,
		EnteredAt: resourceCreatedAt,
		Color: interfaces.TokenColor{
			WorkID:     "executor:0",
			WorkTypeID: "executor",
			DataType:   interfaces.DataTypeResource,
			Tags:       map[string]string{"pool": "shared"},
		},
		History: interfaces.TokenHistory{
			PlaceVisits: map[string]int{"executor:available": 4},
		},
	}
	workConsumed := interfaces.Token{
		ID:        "tok-1",
		PlaceID:   "wt-code:init",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		Color: interfaces.TokenColor{
			WorkID:     "w-resource-failure",
			WorkTypeID: "wt-code",
		},
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
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
				DispatchID:   "d-1",
				TransitionID: "t1",
				ConsumedTokens: []interfaces.Token{
					workConsumed,
					resourceConsumed,
				},
			},
		},
	}

	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 2 {
		t.Fatalf("expected 2 mutations, got %+v", result)
	}

	var released *interfaces.MarkingMutation
	for i := range result.Mutations {
		if result.Mutations[i].NewToken != nil && result.Mutations[i].NewToken.Color.DataType == interfaces.DataTypeResource {
			released = &result.Mutations[i]
			break
		}
	}
	if released == nil {
		t.Fatal("expected released resource mutation")
	}
	if released.ToPlace != "executor:available" {
		t.Fatalf("ToPlace = %q, want %q", released.ToPlace, "executor:available")
	}
	if released.NewToken.ID != resourceConsumed.ID {
		t.Fatalf("resource ID = %q, want %q", released.NewToken.ID, resourceConsumed.ID)
	}
	if released.NewToken.Color.WorkID != resourceConsumed.Color.WorkID {
		t.Fatalf("resource WorkID = %q, want %q", released.NewToken.Color.WorkID, resourceConsumed.Color.WorkID)
	}
	if !released.NewToken.CreatedAt.Equal(resourceCreatedAt) {
		t.Fatalf("CreatedAt = %v, want %v", released.NewToken.CreatedAt, resourceCreatedAt)
	}
	if released.NewToken.Color.Tags["pool"] != "shared" {
		t.Fatalf("tag pool = %q, want %q", released.NewToken.Color.Tags["pool"], "shared")
	}
	if released.NewToken.History.PlaceVisits["executor:available"] != 4 {
		t.Fatalf("PlaceVisits = %#v, want preserved history", released.NewToken.History.PlaceVisits)
	}
}

func TestTransitioner_CalculateMutations_PreservesCreatedAtForSameTypeTransitions(t *testing.T) {
	n := buildPipelineNet()
	transitioner := NewTransitioner(n, nil)
	now := time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	consumed := []interfaces.Token{{
		ID:        "tok-1",
		PlaceID:   "wt-code:init",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: interfaces.TokenColor{
			WorkID:     "w1",
			WorkTypeID: "wt-code",
		},
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}}

	mutations, err := calculateMutations(
		mutationCalculationInput{
			transition:  n.Transitions["t1"],
			arcs:        n.Transitions["t1"].OutputArcs,
			consumed:    consumed,
			result:      resolvedWorkResult{dispatchID: "d-1", transitionID: "t1", outcome: interfaces.OutcomeAccepted},
			now:         now,
			history:     interfaces.TokenHistory{TotalVisits: map[string]int{}, ConsecutiveFailures: map[string]int{}, PlaceVisits: map[string]int{}},
			inputColors: tokenColorsFromTokens(consumed),
			transformer: transitioner.transformer,
		},
	)
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(mutations))
	}
	if !mutations[0].NewToken.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", mutations[0].NewToken.CreatedAt, createdAt)
	}
	if !mutations[0].NewToken.EnteredAt.Equal(now) {
		t.Fatalf("EnteredAt = %v, want %v", mutations[0].NewToken.EnteredAt, now)
	}
}

func buildPipelineNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"wt-code:init":   {ID: "wt-code:init", TypeID: "wt-code", State: "init"},
			"wt-code:done":   {ID: "wt-code:done", TypeID: "wt-code", State: "done"},
			"wt-code:failed": {ID: "wt-code:failed", TypeID: "wt-code", State: "failed"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:         "t1",
				Name:       "code",
				WorkerType: "agent",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "wt-code:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a2", Name: "out", PlaceID: "wt-code:done", Direction: petri.ArcOutput},
				},
				RejectionArcs: []petri.Arc{
					{ID: "a3", Name: "reject", PlaceID: "wt-code:init", Direction: petri.ArcOutput},
				},
				FailureArcs: []petri.Arc{
					{ID: "a4", Name: "fail", PlaceID: "wt-code:failed", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"wt-code": {
				ID: "wt-code",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "done", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
	}
}

func buildRepeaterPipelineNet() *state.Net {
	n := buildPipelineNet()
	n.Transitions["t1"].RejectionArcs = nil
	state.NormalizeTransitionTopology(n, runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"t1": {Name: "t1", Kind: interfaces.WorkstationKindRepeater},
		},
	})
	return n
}

func pipelineSnapshot(placeID, transitionID, dispatchID string, color interfaces.TokenColor, createdAt time.Time) interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	if createdAt.IsZero() {
		createdAt = time.Date(2026, time.April, 6, 8, 0, 0, 0, time.UTC)
	}
	token := interfaces.Token{
		ID:        "tok-1",
		PlaceID:   placeID,
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color:     color,
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}

	return interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{
			Tokens:      map[string]*interfaces.Token{"tok-1": &token},
			PlaceTokens: map[string][]string{placeID: {"tok-1"}},
		},
		Dispatches: map[string]*interfaces.DispatchEntry{
			dispatchID: {
				DispatchID:     dispatchID,
				TransitionID:   transitionID,
				ConsumedTokens: []interfaces.Token{token},
			},
		},
	}
}
