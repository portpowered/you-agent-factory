package subsystems

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type testPipeline struct {
	transitioner *TransitionerSubsystem
	results      *buffers.TypedBuffer[workerexecution.WorkResult]
}

func newTestPipeline(n *state.Net, now func() time.Time) *testPipeline {
	if now == nil {
		now = testSubsystemNow
	}
	return &testPipeline{
		transitioner: NewTransitioner(n, nil, now, testTokenTransformer(n), nil, nil, nil, testWorkPropagationPolicy()),
		results:      buffers.NewTypedBuffer[workerexecution.WorkResult](16),
	}
}

func (tp *testPipeline) WriteResult(r workerexecution.WorkResult) {
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
	tp := newTestPipeline(n, nil)
	tp.WriteResult(workerexecution.WorkResult{DispatchID: "d-1", TransitionID: "t1", Outcome: workerexecution.OutcomeAccepted})

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		factorytoken.Color{WorkID: "w1", WorkTypeID: "wt-code"},
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
	tp := newTestPipeline(n, nil)
	tp.WriteResult(workerexecution.WorkResult{DispatchID: "d-1", TransitionID: "t1", Outcome: workerexecution.OutcomeFailed, Error: "agent crashed"})

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		factorytoken.Color{WorkID: "w1", WorkTypeID: "wt-code"},
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

func TestHistoryTransitionerPipeline_TerminalResultOutcomeRoutes(t *testing.T) {
	now := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name             string
		outcome          workerexecution.WorkOutcome
		feedback         string
		err              string
		cancellation     workerexecution.DispatchCancellationReason
		wantPlace        string
		wantCompletion   workerexecution.WorkOutcome
		wantReason       string
		wantCancellation workerexecution.DispatchCancellationReason
	}{
		{
			name: "accepted", outcome: workerexecution.OutcomeAccepted,
			wantPlace: "wt-code:done", wantCompletion: workerexecution.OutcomeAccepted,
		},
		{
			name: "continue", outcome: workerexecution.OutcomeContinue, feedback: "needs revision",
			wantPlace: "wt-code:init", wantCompletion: workerexecution.OutcomeContinue,
			wantReason: "needs revision",
		},
		{
			name: "rejected", outcome: workerexecution.OutcomeRejected, feedback: "not ready",
			wantPlace: "wt-code:init", wantCompletion: workerexecution.OutcomeRejected,
			wantReason: "not ready",
		},
		{
			name: "failed", outcome: workerexecution.OutcomeFailed, err: "agent crashed",
			wantPlace: "wt-code:failed", wantCompletion: workerexecution.OutcomeFailed,
			wantReason: "agent crashed",
		},
		{
			name: "canceled", outcome: workerexecution.OutcomeCanceled,
			cancellation: workerexecution.DispatchCancellationReasonSuperseded,
			wantPlace:    "wt-code:init", wantCompletion: workerexecution.OutcomeCanceled,
			wantReason:       string(workerexecution.DispatchCancellationReasonSuperseded),
			wantCancellation: workerexecution.DispatchCancellationReasonSuperseded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tp := newTestPipeline(buildPipelineNet(), func() time.Time { return now })
			workerResult := workerexecution.WorkResult{
				DispatchID: "d-1", TransitionID: "t1", Outcome: test.outcome,
				Feedback: test.feedback, Error: test.err,
			}
			if test.cancellation != "" {
				workerResult.Cancellation = &workerexecution.DispatchCancellation{Reason: test.cancellation}
			}
			tp.WriteResult(workerResult)

			snapshot := pipelineSnapshot(
				"wt-code:init", "t1", "d-1",
				factorytoken.Color{WorkID: "w1", WorkTypeID: "wt-code"}, now,
			)
			if test.outcome == workerexecution.OutcomeCanceled {
				snapshot.Marking = petri.MarkingSnapshot{
					Tokens:      map[string]*factorytoken.Token{},
					PlaceTokens: map[string][]string{},
				}
				snapshot.Dispatches["d-1"].HeldMutations = []interfaces.MarkingMutation{{
					Type:      interfaces.MutationConsume,
					TokenID:   "tok-1",
					FromPlace: "wt-code:init",
				}}
			}
			result, err := tp.Execute(context.Background(), &snapshot)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result == nil || len(result.Mutations) != 1 {
				t.Fatalf("Execute() result = %#v, want one route mutation", result)
			}
			mutation := result.Mutations[0]
			if mutation.ToPlace != test.wantPlace || mutation.NewToken == nil ||
				mutation.NewToken.Color.WorkID != "w1" {
				t.Fatalf("route mutation = %#v, want Work w1 at %s", mutation, test.wantPlace)
			}
			if len(result.CompletedDispatches) != 1 {
				t.Fatalf("completed dispatches = %#v, want one", result.CompletedDispatches)
			}
			completed := result.CompletedDispatches[0]
			if completed.Outcome != test.wantCompletion || completed.Reason != test.wantReason {
				t.Fatalf("completion = %#v, want outcome %s reason %q", completed, test.wantCompletion, test.wantReason)
			}
			if test.wantCancellation == "" {
				if completed.Cancellation != nil {
					t.Fatalf("completion cancellation = %#v, want nil", completed.Cancellation)
				}
			} else if completed.Cancellation == nil || completed.Cancellation.Reason != test.wantCancellation {
				t.Fatalf("completion cancellation = %#v, want %s", completed.Cancellation, test.wantCancellation)
			}
		})
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
			transitioner := NewTransitioner(buildPipelineNet(), nil, func() time.Time { return now }, testTokenTransformer(buildPipelineNet()), nil, nil, nil, testWorkPropagationPolicy())
			snapshot := pipelineSnapshot(
				test.placeID,
				"t1",
				"d-late-"+test.name,
				factorytoken.Color{WorkID: "w-terminal", WorkTypeID: "wt-code"},
				now,
			)
			snapshot.Results = []workerexecution.WorkResult{{
				DispatchID: "d-late-" + test.name, TransitionID: "t1", Outcome: workerexecution.OutcomeAccepted,
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
				ignored.ObservedState.Name != test.stateName || ignored.ObservedState.Type != test.stateType {
				t.Fatalf("ignored payload = %#v, want %s/%s accepted", ignored, test.stateName, test.stateType)
			}
		})
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

func TestHistoryTransitionerPipeline_FailedWithoutFailureArcs_UsesConsumedDispatchTokensForFallback(t *testing.T) {
	n := buildPipelineNet()
	n.Transitions["t1"].FailureArcs = nil
	state.NormalizeTransitionTopology(n, nil)
	tp := newTestPipeline(n, nil)
	tp.WriteResult(workerexecution.WorkResult{DispatchID: "d-1", TransitionID: "t1", Outcome: workerexecution.OutcomeFailed, Error: "agent crashed"})
	createdAt := time.Date(2026, time.April, 6, 9, 0, 0, 0, time.UTC)

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		factorytoken.Color{WorkID: "w-fallback", WorkTypeID: "wt-code"},
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
	tp := newTestPipeline(n, nil)
	tp.WriteResult(workerexecution.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeRejected,
		Feedback:     "try again",
	})
	createdAt := time.Date(2026, time.April, 6, 10, 0, 0, 0, time.UTC)

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		factorytoken.Color{WorkID: "w1", WorkTypeID: "wt-code"},
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
	tp := newTestPipeline(n, nil)
	tp.WriteResult(workerexecution.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "provider error: claude rate limit exceeded",
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyThrottle,
			Type:   workerexecution.WorkFailureTypeThrottled,
		},
	})
	createdAt := time.Date(2026, time.April, 6, 10, 0, 0, 0, time.UTC)

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		factorytoken.Color{WorkID: "w-throttle", WorkTypeID: "wt-code"},
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
	tp := newTestPipeline(n, nil)
	tp.WriteResult(workerexecution.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "execution timeout",
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeTimeout,
		},
	})
	createdAt := time.Date(2026, time.April, 6, 10, 30, 0, 0, time.UTC)

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		factorytoken.Color{WorkID: "w-timeout", WorkTypeID: "wt-code"},
		createdAt,
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	assertTimeoutFailureRequeueResult(t, result, createdAt, "w-timeout", "execution timeout")
}

func assertTimeoutFailureRequeueResult(t *testing.T, result *interfaces.TickResult, createdAt time.Time, workID string, errorText string) {
	t.Helper()
	if result.Mutations[0].ToPlace != "wt-code:init" {
		t.Fatalf("ToPlace = %s, want wt-code:init", result.Mutations[0].ToPlace)
	}
	if result.Mutations[0].NewToken.Color.WorkID != workID {
		t.Fatalf("WorkID = %s, want %s", result.Mutations[0].NewToken.Color.WorkID, workID)
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
	if result.Mutations[0].NewToken.History.LastError != errorText {
		t.Fatalf("LastError = %q", result.Mutations[0].NewToken.History.LastError)
	}
	if len(result.Mutations[0].NewToken.History.FailureLog) != 1 {
		t.Fatalf("FailureLog length = %d, want 1", len(result.Mutations[0].NewToken.History.FailureLog))
	}
	if result.Mutations[0].NewToken.History.FailureLog[0].Attempt != 1 {
		t.Fatalf("FailureLog attempt = %d, want 1", result.Mutations[0].NewToken.History.FailureLog[0].Attempt)
	}
	if len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatch count = %d, want 1", len(result.CompletedDispatches))
	}
	completed := result.CompletedDispatches[0]
	if completed.FailureMetadata == nil {
		t.Fatal("completed dispatch FailureMetadata = nil, want timeout metadata")
	}
	if completed.FailureMetadata.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("completed dispatch FailureMetadata.Type = %q, want %q", completed.FailureMetadata.Type, workerexecution.WorkFailureTypeTimeout)
	}
	if completed.FailureMetadata.Family != workerexecution.WorkFailureFamilyRetryable {
		t.Fatalf("completed dispatch FailureMetadata.Family = %q, want %q", completed.FailureMetadata.Family, workerexecution.WorkFailureFamilyRetryable)
	}
}

func TestHistoryTransitionerPipeline_TimeoutFailureRequeuesDespiteRenderedErrorText(t *testing.T) {
	n := buildPipelineNet()
	tp := newTestPipeline(n, nil)
	tp.WriteResult(workerexecution.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "provider error: timeout: context deadline exceeded",
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeTimeout,
		},
	})
	createdAt := time.Date(2026, time.April, 6, 10, 45, 0, 0, time.UTC)

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		factorytoken.Color{WorkID: "w-timeout-rendered", WorkTypeID: "wt-code"},
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
	tp := newTestPipeline(n, nil)
	tp.WriteResult(workerexecution.WorkResult{
		DispatchID:   "d-1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "provider error: internal_server_error",
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeInternalServerError,
		},
	})

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		factorytoken.Color{WorkID: "w-retryable", WorkTypeID: "wt-code"},
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
		metadata *workerexecution.WorkFailureMetadata
		workID   string
	}{
		{
			name: "MissingFamily",
			metadata: &workerexecution.WorkFailureMetadata{
				Type: workerexecution.WorkFailureTypeInternalServerError,
			},
			workID: "w-retryable-missing-family",
		},
		{
			name: "StaleTerminalFamily",
			metadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyTerminal,
				Type:   workerexecution.WorkFailureTypeInternalServerError,
			},
			workID: "w-retryable-stale-family",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			n := buildPipelineNet()
			tp := newTestPipeline(n, nil)
			tp.WriteResult(workerexecution.WorkResult{
				DispatchID:      "d-1",
				TransitionID:    "t1",
				Outcome:         workerexecution.OutcomeFailed,
				Error:           "provider error: internal_server_error",
				FailureMetadata: (*workerexecution.WorkFailureMetadata)(tc.metadata),
			})

			snapshot := pipelineSnapshot(
				"wt-code:init",
				"t1",
				"d-1",
				factorytoken.Color{WorkID: tc.workID, WorkTypeID: "wt-code"},
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
	const errorText = "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)"
	createdAt := time.Date(2026, time.April, 6, 11, 5, 0, 0, time.UTC)
	failedAt := createdAt.Add(5 * time.Minute)
	providerFailure := &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyRetryable,
		Type:   workerexecution.WorkFailureTypeInternalServerError,
	}
	tp := newTestPipeline(buildPipelineNet(), func() time.Time { return failedAt })
	tp.WriteResult(workerexecution.WorkResult{
		DispatchID:      "d-1",
		TransitionID:    "t1",
		Outcome:         workerexecution.OutcomeFailed,
		Error:           errorText,
		FailureMetadata: providerFailure,
	})

	snapshot := pipelineSnapshot(
		"wt-code:init",
		"t1",
		"d-1",
		factorytoken.Color{WorkID: "w-codex-windows-4294967295", WorkTypeID: "wt-code"},
		createdAt,
	)
	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %+v", result)
	}
	assertWindowsProviderFailureRequeue(t, result.Mutations[0], createdAt, failedAt, errorText)
	completedMetadata := assertWindowsProviderFailedDispatch(t, result, errorText)

	providerFailure.Type = workerexecution.WorkFailureTypeAuthFailure
	providerFailure.Family = workerexecution.WorkFailureFamilyTerminal
	assertRetryableInternalServerMetadata(t, completedMetadata)
}

func assertWindowsProviderFailureRequeue(
	t *testing.T,
	mutation interfaces.MarkingMutation,
	createdAt time.Time,
	failedAt time.Time,
	errorText string,
) {
	t.Helper()
	if mutation.ToPlace != "wt-code:init" {
		t.Fatalf("ToPlace = %s, want wt-code:init", mutation.ToPlace)
	}
	token := mutation.NewToken
	if token.Color.WorkID != "w-codex-windows-4294967295" {
		t.Fatalf("WorkID = %s, want w-codex-windows-4294967295", token.Color.WorkID)
	}
	if !token.CreatedAt.Equal(createdAt) || !token.EnteredAt.Equal(failedAt) {
		t.Fatalf("token timestamps = (%v, %v), want (%v, %v)", token.CreatedAt, token.EnteredAt, createdAt, failedAt)
	}
	assertWindowsProviderFailureHistory(t, token.History, failedAt, errorText)
}

func assertWindowsProviderFailureHistory(t *testing.T, history factorytoken.History, failedAt time.Time, errorText string) {
	t.Helper()
	if got := history.TotalVisits["t1"]; got != 1 {
		t.Fatalf("TotalVisits[t1] = %d, want 1", got)
	}
	if got := history.ConsecutiveFailures["t1"]; got != 1 {
		t.Fatalf("ConsecutiveFailures[t1] = %d, want 1", got)
	}
	if history.LastError != errorText {
		t.Fatalf("LastError = %q, want %q", history.LastError, errorText)
	}
	if len(history.FailureLog) != 1 {
		t.Fatalf("FailureLog length = %d, want 1", len(history.FailureLog))
	}
	record := history.FailureLog[0]
	if record.TransitionID != "t1" || record.Attempt != 1 {
		t.Fatalf("FailureLog identity = (%q, attempt %d), want (t1, attempt 1)", record.TransitionID, record.Attempt)
	}
	if record.Error != errorText || !record.Timestamp.Equal(failedAt) {
		t.Fatalf("FailureLog evidence = (%q, %v), want (%q, %v)", record.Error, record.Timestamp, errorText, failedAt)
	}
}

func assertWindowsProviderFailedDispatch(t *testing.T, result *interfaces.TickResult, errorText string) *workerexecution.WorkFailureMetadata {
	t.Helper()
	if len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatch count = %d, want 1", len(result.CompletedDispatches))
	}
	completed := result.CompletedDispatches[0]
	if completed.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("completed dispatch outcome = %q, want %q", completed.Outcome, workerexecution.OutcomeFailed)
	}
	if completed.Reason != errorText {
		t.Fatalf("completed dispatch reason = %q, want %q", completed.Reason, errorText)
	}
	if completed.FailureMetadata == nil {
		t.Fatal("expected completed dispatch failure metadata")
	}
	return completed.FailureMetadata
}

func assertRetryableInternalServerMetadata(t *testing.T, metadata *workerexecution.WorkFailureMetadata) {
	t.Helper()
	if metadata.Type != workerexecution.WorkFailureTypeInternalServerError {
		t.Fatalf("completed dispatch failure type after source mutation = %q, want %q", metadata.Type, workerexecution.WorkFailureTypeInternalServerError)
	}
	if metadata.Family != workerexecution.WorkFailureFamilyRetryable {
		t.Fatalf("completed dispatch failure family after source mutation = %q, want %q", metadata.Family, workerexecution.WorkFailureFamilyRetryable)
	}
	decision := workerexecution.FailureDecisionFromMetadata(metadata)
	if !decision.Retryable || decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("WorkFailureDecisionFromMetadata(%#v) = %#v, want retryable non-terminal non-throttle", metadata, decision)
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

func TestHistoryTransitionerPipeline_AcceptedReleasesConsumedResourceTokenIdentityRegardlessOfInputOrder(t *testing.T) {
	orderings := []struct {
		name           string
		consumedTokens []factorytoken.Token
	}{
		{name: "resource-first"},
		{name: "work-first"},
	}

	for _, ordering := range orderings {
		t.Run(ordering.name, func(t *testing.T) {
			tp, snapshot, resourceConsumed := newAcceptedReleasesConsumedResourceFixture(ordering.name == "work-first")
			result, err := tp.Execute(context.Background(), snapshot)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertAcceptedMixedWorkResourceRelease(t, result, resourceConsumed)
		})
	}
}

func newAcceptedReleasesConsumedResourceFixture(workFirst bool) (*testPipeline, *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], factorytoken.Token) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"story:in-review":      {ID: "story:in-review", TypeID: "story", State: "in-review"},
			"story:complete":       {ID: "story:complete", TypeID: "story", State: "complete"},
			"agent-slot:available": {ID: "agent-slot:available", TypeID: "agent-slot", State: "available"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:         "t1",
				Name:       "review-story",
				WorkerType: "reviewer",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "story:in-review", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "a2", Name: "resource", PlaceID: "agent-slot:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a3", Name: "complete", PlaceID: "story:complete", Direction: petri.ArcOutput},
					{ID: "a4", Name: "resource", PlaceID: "agent-slot:available", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"story": {
				ID: "story",
				States: []state.StateDefinition{
					{Value: "in-review", Category: state.StateCategoryProcessing},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
	}
	tp := newTestPipeline(n, nil)
	tp.WriteResult(workerexecution.WorkResult{DispatchID: "d-1", TransitionID: "t1", Outcome: workerexecution.OutcomeAccepted, Output: "Done. COMPLETE ACCEPTED"})

	now := time.Date(2026, time.April, 7, 14, 0, 0, 0, time.UTC)
	resourceConsumed := factorytoken.Token{
		ID:        "agent-slot:resource:0",
		PlaceID:   "agent-slot:available",
		CreatedAt: now.Add(-3 * time.Hour),
		EnteredAt: now.Add(-3 * time.Hour),
		Color: factorytoken.Color{
			WorkID:     "agent-slot:0",
			WorkTypeID: "agent-slot",
			DataType:   factorytoken.DataTypeResource,
			Tags:       map[string]string{"pool": "shared"},
		},
		History: factorytoken.History{
			PlaceVisits: map[string]int{"agent-slot:available": 4},
		},
	}
	workConsumed := factorytoken.Token{
		ID:        "tok-1",
		PlaceID:   "story:in-review",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
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
				"story:in-review":      {workConsumed.ID},
				"agent-slot:available": {resourceConsumed.ID},
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

func assertReleasedResourcePipelineHistory(t *testing.T, released *factorytoken.Token, resourceConsumed factorytoken.Token) {
	t.Helper()
	if released.ID != resourceConsumed.ID || released.Color.WorkID != resourceConsumed.Color.WorkID {
		t.Fatalf("released resource token = %#v, want preserved identity from %#v", released, resourceConsumed)
	}
	if !released.CreatedAt.Equal(resourceConsumed.CreatedAt) {
		t.Fatalf("CreatedAt = %v, want %v", released.CreatedAt, resourceConsumed.CreatedAt)
	}
	if released.Color.Tags["pool"] != "shared" {
		t.Fatalf("tag pool = %q, want %q", released.Color.Tags["pool"], "shared")
	}
	if released.History.PlaceVisits["agent-slot:available"] != 4 {
		t.Fatalf("PlaceVisits = %#v, want preserved history", released.History.PlaceVisits)
	}
}

func assertAcceptedMixedWorkResourceRelease(t *testing.T, result *interfaces.TickResult, resourceConsumed factorytoken.Token) {
	t.Helper()
	if result == nil || len(result.Mutations) != 2 {
		t.Fatalf("expected 2 mutations, got %+v", result)
	}
	workMutation, released := findWorkAndResourceMutations(result.Mutations)
	assertAcceptedMixedWorkOutputMutation(t, workMutation)
	assertReleasedResourceMutationBasics(t, released, resourceConsumed)
	releasedToken := factorytoken.FromWorker(*released.NewToken)
	assertReleasedResourcePipelineHistory(t, &releasedToken, resourceConsumed)
}

func TestTransitioner_CalculateMutations_PreservesCreatedAtForSameTypeTransitions(t *testing.T) {
	n := buildPipelineNet()
	transitioner := NewTransitioner(n, nil, testSubsystemNow, testTokenTransformer(n), nil, nil, nil, testWorkPropagationPolicy())
	now := time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	consumed := []factorytoken.Token{{
		ID:        "tok-1",
		PlaceID:   "wt-code:init",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: factorytoken.Color{
			WorkID:     "w1",
			WorkTypeID: "wt-code",
		},
		History: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}}

	mutations, err := calculateMutations(
		mutationCalculationInput{
			workPropagation: testWorkPropagationPolicy(),
			transition:      n.Transitions["t1"],
			arcs:            n.Transitions["t1"].OutputArcs,
			consumed:        consumed,
			result:          resolvedWorkResult{dispatchID: "d-1", transitionID: "t1", outcome: workerexecution.OutcomeAccepted},
			now:             now,
			history:         factorytoken.History{TotalVisits: map[string]int{}, ConsecutiveFailures: map[string]int{}, PlaceVisits: map[string]int{}},
			inputColors:     tokenColorsFromTokens(consumed),
			transformer:     transitioner.transformer,
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
	state.NormalizeTransitionTopology(n, map[string]interfaces.WorkstationKind{
		"t1": interfaces.WorkstationKindRepeater,
	})
	return n
}

func pipelineSnapshot(placeID, transitionID, dispatchID string, color factorytoken.Color, createdAt time.Time) interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	if createdAt.IsZero() {
		createdAt = time.Date(2026, time.April, 6, 8, 0, 0, 0, time.UTC)
	}
	token := factorytoken.Token{
		ID:        "tok-1",
		PlaceID:   placeID,
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color:     color,
		History: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}

	return interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{
			Tokens:      map[string]*factorytoken.Token{"tok-1": &token},
			PlaceTokens: map[string][]string{placeID: {"tok-1"}},
		},
		Dispatches: map[string]*interfaces.DispatchEntry{
			dispatchID: {
				DispatchID:     dispatchID,
				TransitionID:   transitionID,
				ConsumedTokens: factorytoken.ToWorkerSlice([]factorytoken.Token{token}),
			},
		},
	}
}
