package subsystems

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestHistorySubsystem_Execute_MergesHistoryFromDispatchConsumedTokens(t *testing.T) {
	timestamp := time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC)
	subsystem := NewHistory(nil)
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Results: []workerexecution.WorkResult{{
			DispatchID:   "dispatch-1",
			TransitionID: "transition-review",
			Outcome:      workerexecution.OutcomeFailed,
		}},
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch-1": {
				DispatchID: "dispatch-1",
				ConsumedTokens: factorytoken.ToWorkerSlice([]factorytoken.Token{
					{
						ID:      "token-1",
						PlaceID: "story:init",
						Color: factorytoken.Color{
							WorkID:     "story-1",
							WorkTypeID: "story",
						},
						History: factorytoken.History{
							TotalVisits: map[string]int{
								"transition-build": 2,
							},
							ConsecutiveFailures: map[string]int{
								"transition-review": 1,
							},
							PlaceVisits: map[string]int{
								"story:init": 3,
							},
							LastError: "previous failure",
							FailureLog: []factorytoken.Failure{{
								TransitionID: "transition-build",
								Timestamp:    timestamp,
								Error:        "build failed",
								Attempt:      1,
							}},
						},
					},
				}),
			},
		},
	}

	result, err := subsystem.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil TickResult")
	}
	if len(result.Histories) != 1 {
		t.Fatalf("len(Histories) = %d, want 1", len(result.Histories))
	}

	history := result.Histories[0]
	if got := history.TotalVisits["transition-build"]; got != 2 {
		t.Fatalf("TotalVisits[transition-build] = %d, want 2", got)
	}
	if got := history.TotalVisits["transition-review"]; got != 1 {
		t.Fatalf("TotalVisits[transition-review] = %d, want 1", got)
	}
	if got := history.ConsecutiveFailures["transition-review"]; got != 2 {
		t.Fatalf("ConsecutiveFailures[transition-review] = %d, want 2", got)
	}
	if got := history.PlaceVisits["story:init"]; got != 3 {
		t.Fatalf("PlaceVisits[story:init] = %d, want 3", got)
	}
	if history.LastError != "previous failure" {
		t.Fatalf("LastError = %q, want %q", history.LastError, "previous failure")
	}
	if len(history.FailureLog) != 1 {
		t.Fatalf("len(FailureLog) = %d, want 1", len(history.FailureLog))
	}
	if history.FailureLog[0].Timestamp != timestamp {
		t.Fatalf("FailureLog[0].Timestamp = %s, want %s", history.FailureLog[0].Timestamp, timestamp)
	}
}

func TestHistorySubsystem_RepeatedSnapshotExecutionDoesNotDoubleCountVisitHistory(t *testing.T) {
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Results: []workerexecution.WorkResult{{
			DispatchID:   "dispatch-repeated",
			TransitionID: "review",
			Outcome:      workerexecution.OutcomeRejected,
		}},
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch-repeated": {
				ConsumedTokens: factorytoken.ToWorkerSlice([]factorytoken.Token{{
					Color: factorytoken.Color{WorkID: "work-repeated", WorkTypeID: "task"},
					History: factorytoken.History{TotalVisits: map[string]int{
						"process": 12,
						"review":  11,
					}},
				}}),
			},
		},
	}

	subsystem := NewHistory(nil)
	first, err := subsystem.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	second, err := subsystem.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("repeated Execute() error = %v", err)
	}
	if first == nil || second == nil {
		t.Fatalf("Execute() results = (%#v, %#v), want history results", first, second)
	}
	if !reflect.DeepEqual(first.Histories, second.Histories) {
		t.Fatalf("repeated histories = %#v, want %#v", second.Histories, first.Histories)
	}
	history := first.Histories[0]
	if history.TotalVisits["process"] != 12 || history.TotalVisits["review"] != 12 {
		t.Fatalf("computed history = %#v, want process=12 review=12", history.TotalVisits)
	}
	if snapshot.Dispatches["dispatch-repeated"].ConsumedTokens[0].History.TotalVisits["review"] != 11 {
		t.Fatalf("source snapshot review visits = %d, want unchanged 11", snapshot.Dispatches["dispatch-repeated"].ConsumedTokens[0].History.TotalVisits["review"])
	}
}

func TestBuildHistory_PreservesCompactedFailureCount(t *testing.T) {
	history := buildHistory([]factorytoken.Token{{
		Color: factorytoken.Color{WorkID: "task-1", WorkTypeID: "task"},
		History: factorytoken.History{
			FailureLogDroppedCount: 11,
			FailureLog: []factorytoken.Failure{{
				TransitionID: "review", Error: "latest failure",
			}},
		},
	}}, &workerexecution.WorkResult{
		TransitionID: "review",
		Outcome:      workerexecution.OutcomeFailed,
	}, "task-1")

	if history.FailureLogDroppedCount != 11 {
		t.Fatalf("FailureLogDroppedCount = %d, want 11", history.FailureLogDroppedCount)
	}
}

func TestBuildHistory_MergesSharedLineageVisitCountsWithMaxNotSum(t *testing.T) {
	consumed := []factorytoken.Token{
		{
			Color: factorytoken.Color{WorkID: "task-1", WorkTypeID: "task"},
			History: factorytoken.History{
				TotalVisits: map[string]int{"process": 3, "review": 2},
			},
		},
		{
			Color: factorytoken.Color{WorkID: "review-1", WorkTypeID: "review", ParentID: "task-1"},
			History: factorytoken.History{
				TotalVisits: map[string]int{"process": 3, "review": 1},
			},
		},
	}

	history := buildHistory(consumed, &workerexecution.WorkResult{
		TransitionID: "review",
		Outcome:      workerexecution.OutcomeContinue,
	}, "task-1")

	if got := history.TotalVisits["process"]; got != 3 {
		t.Fatalf("TotalVisits[process] = %d, want 3", got)
	}
	if got := history.TotalVisits["review"]; got != 3 {
		t.Fatalf("TotalVisits[review] = %d, want 3", got)
	}
}

func TestBuildHistory_ContinueResetsConsecutiveFailureStrike(t *testing.T) {
	consumed := []factorytoken.Token{{
		Color: factorytoken.Color{WorkID: "task-1", WorkTypeID: "task"},
		History: factorytoken.History{
			TotalVisits:         map[string]int{"review": 4},
			ConsecutiveFailures: map[string]int{"review": 2},
		},
	}}

	history := buildHistory(consumed, &workerexecution.WorkResult{
		TransitionID: "review",
		Outcome:      workerexecution.OutcomeContinue,
	}, "task-1")

	if got := history.TotalVisits["review"]; got != 5 {
		t.Fatalf("TotalVisits[review] = %d, want 5 for the continued visit", got)
	}
	if got := history.ConsecutiveFailures["review"]; got != 0 {
		t.Fatalf("ConsecutiveFailures[review] = %d, want 0 for a non-failing continue", got)
	}
}

func TestBuildHistory_IncompleteOutputRemainsConsecutiveFailure(t *testing.T) {
	consumed := []factorytoken.Token{{
		Color: factorytoken.Color{WorkID: "review-1", WorkTypeID: "review"},
		History: factorytoken.History{
			ConsecutiveFailures: map[string]int{"review": 1},
		},
	}}

	history := buildHistory(consumed, &workerexecution.WorkResult{
		TransitionID: "review",
		Outcome:      workerexecution.OutcomeFailed,
		Diagnostics: &workerexecution.WorkDiagnostics{Provider: &workerexecution.ProviderDiagnostic{
			ResponseMetadata: map[string]string{
				workerexecution.ProviderResponseMetadataFailureOperation:      "completion_validation",
				workerexecution.ProviderResponseMetadataFailureClassification: "missing_required_output",
			},
		}},
	}, "review-1")

	if got := history.ConsecutiveFailures["review"]; got != 2 {
		t.Fatalf("ConsecutiveFailures[review] = %d, want 2 for INCOMPLETE_OUTPUT", got)
	}
}

func TestBuildHistory_ExcludesDifferentWorkOnSharedTrace(t *testing.T) {
	const sharedTrace = "batch-trace"
	consumed := []factorytoken.Token{
		{
			Color:   factorytoken.Color{WorkID: "task-a", WorkTypeID: "task", CurrentChainingTraceID: sharedTrace},
			History: factorytoken.History{TotalVisits: map[string]int{"process": 1, "review": 0}},
		},
		{
			Color:   factorytoken.Color{WorkID: "review-b", WorkTypeID: "review", ParentID: "task-b", CurrentChainingTraceID: sharedTrace},
			History: factorytoken.History{TotalVisits: map[string]int{"process": 7, "review": 6}},
		},
	}

	history := buildHistory(consumed, &workerexecution.WorkResult{
		TransitionID: "review",
		Outcome:      workerexecution.OutcomeRejected,
	}, "task-a")

	if got := history.TotalVisits["process"]; got != 1 {
		t.Fatalf("TotalVisits[process] = %d, want 1 from task-a only", got)
	}
	if got := history.TotalVisits["review"]; got != 1 {
		t.Fatalf("TotalVisits[review] = %d, want first review visit for task-a", got)
	}
}

func TestBuildHistory_AccumulatesRepeatedCandidateCycles(t *testing.T) {
	consumed := []factorytoken.Token{{
		Color:   factorytoken.Color{WorkID: "task-a", WorkTypeID: "task"},
		History: factorytoken.History{TotalVisits: map[string]int{"process": 2, "review": 3}},
	}}

	history := buildHistory(consumed, &workerexecution.WorkResult{
		TransitionID: "review",
		Outcome:      workerexecution.OutcomeRejected,
	}, "task-a")

	if got := history.TotalVisits["process"]; got != 2 {
		t.Fatalf("TotalVisits[process] = %d, want 2", got)
	}
	if got := history.TotalVisits["review"]; got != 4 {
		t.Fatalf("TotalVisits[review] = %d, want 4", got)
	}
}

func TestCandidateWorkID_UsesAuthoredInputOrderInsteadOfDispatchTokenOrder(t *testing.T) {
	net := &state.Net{Transitions: map[string]*petri.Transition{
		"review": {
			ID: "review",
			InputArcs: []petri.Arc{
				{PlaceID: "task:in-review"},
				{PlaceID: "review:init"},
			},
		},
	}}
	consumed := []factorytoken.Token{
		{PlaceID: "review:init", Color: factorytoken.Color{WorkID: "review-b", ParentID: "task-b"}},
		{PlaceID: "task:in-review", Color: factorytoken.Color{WorkID: "task-a"}},
	}

	if got := candidateWorkID(net, "review", consumed); got != "task-a" {
		t.Fatalf("candidateWorkID() = %q, want task-a", got)
	}
}

func TestBuildHistory_WhenDispatchLookupMissing_UsesOnlyCurrentResult(t *testing.T) {
	history := buildHistory(nil, &workerexecution.WorkResult{
		DispatchID:   "dispatch-missing",
		TransitionID: "transition-review",
		Outcome:      workerexecution.OutcomeAccepted,
	}, "")

	if got := history.TotalVisits["transition-review"]; got != 1 {
		t.Fatalf("TotalVisits[transition-review] = %d, want 1", got)
	}
	if got := history.ConsecutiveFailures["transition-review"]; got != 0 {
		t.Fatalf("ConsecutiveFailures[transition-review] = %d, want 0", got)
	}
	if len(history.PlaceVisits) != 0 {
		t.Fatalf("PlaceVisits should be empty, got %+v", history.PlaceVisits)
	}
}

type transitionerLogEntry struct {
	level   string
	message string
	fields  map[string]any
}

type transitionerLogCapture struct {
	entries []transitionerLogEntry
}

func (l *transitionerLogCapture) Debug(message string, fields ...any) {
	l.record("debug", message, fields...)
}
func (l *transitionerLogCapture) Info(message string, fields ...any) {
	l.record("info", message, fields...)
}
func (l *transitionerLogCapture) Warn(message string, fields ...any) {
	l.record("warn", message, fields...)
}
func (l *transitionerLogCapture) Error(message string, fields ...any) {
	l.record("error", message, fields...)
}
func (l *transitionerLogCapture) Verbose(message string, fields ...any) {
	l.record("verbose", message, fields...)
}
func (l *transitionerLogCapture) record(level, message string, fields ...any) {
	values := make(map[string]any)
	for index := 0; index+1 < len(fields); index += 2 {
		key, ok := fields[index].(string)
		if ok {
			values[key] = fields[index+1]
		}
	}
	l.entries = append(l.entries, transitionerLogEntry{level: level, message: message, fields: values})
}

var _ logging.Logger = (*transitionerLogCapture)(nil)

type transitionerLogCase struct {
	name          string
	result        workerexecution.WorkResult
	wantLevel     string
	wantMessage   string
	wantReason    string
	wantCancel    string
	wantSafeField string
}

func TestTransitioner_CanceledDispatchRestoresConsumedWorkWithoutFailureRoute(t *testing.T) {
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(
		net, nil, func() time.Time { return now }, testTokenTransformer(net),
		nil, nil, nil, testWorkPropagationPolicy(),
	)
	snapshot := workerBatchSnapshot("")
	snapshot.Dispatches["dispatch-1"].HeldMutations = []interfaces.MarkingMutation{{
		Type: interfaces.MutationConsume, TokenID: "tok-source", FromPlace: "task:init",
	}}
	snapshot.Results[0] = workerexecution.WorkResult{
		DispatchID: "dispatch-1", TransitionID: "t1", Outcome: workerexecution.OutcomeCanceled,
		Cancellation: &workerexecution.DispatchCancellation{Reason: workerexecution.DispatchCancellationReasonSuperseded},
		Error:        "losing dispatch was superseded",
	}

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	assertCanceledDispatchRestoration(t, result, now)
}

func assertCanceledDispatchRestoration(
	t *testing.T,
	result *interfaces.TickResult,
	now time.Time,
) {
	t.Helper()
	if result == nil {
		t.Fatal("Execute() result = nil, want canceled dispatch retirement")
	}
	if len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want exactly one", result.CompletedDispatches)
	}
	assertCanceledDispatchCompletion(t, result.CompletedDispatches[0])
	assertCanceledDispatchMutation(t, result.Mutations, now)
}

func assertCanceledDispatchCompletion(t *testing.T, completed interfaces.CompletedDispatch) {
	t.Helper()
	if completed.Outcome != workerexecution.OutcomeCanceled || completed.Cancellation == nil ||
		completed.Cancellation.Reason != workerexecution.DispatchCancellationReasonSuperseded {
		t.Fatalf("completed dispatch = %#v, want SUPERSEDED cancellation", completed)
	}
	if completed.FailureDetail != nil || completed.FailureMetadata != nil {
		t.Fatalf("completed cancellation retained failure facts: %#v", completed)
	}
	if len(completed.OutputMutations) != 1 || completed.OutputMutations[0].Type != interfaces.MutationCreate {
		t.Fatalf("completed output mutations = %#v, want the non-failure restoration", completed.OutputMutations)
	}
}

func assertCanceledDispatchMutation(t *testing.T, mutations []interfaces.MarkingMutation, now time.Time) {
	t.Helper()
	if len(mutations) != 1 {
		t.Fatalf("mutations = %#v, want exactly one restored Work token", mutations)
	}
	restored := mutations[0]
	if restored.Type != interfaces.MutationCreate || restored.TokenID != "tok-source" ||
		restored.ToPlace != "task:init" || restored.NewToken == nil {
		t.Fatalf("restored mutation = %#v, want CREATE tok-source at task:init", restored)
	}
	if restored.NewToken.ID != "tok-source" || restored.NewToken.State != "init" ||
		restored.NewToken.EnteredAt != now {
		t.Fatalf("restored token = %#v, want original identity at cancellation time", restored.NewToken)
	}
	if restored.ToPlace == "task:failed" {
		t.Fatal("canceled dispatch emitted a terminal failure mutation")
	}
}

func TestCalculateArcs_CanceledDispatchHasNoBusinessRoute(t *testing.T) {
	transition := &petri.Transition{
		ID:          "t1",
		OutputArcs:  []petri.Arc{{ID: "accepted", PlaceID: "task:complete"}},
		FailureArcs: []petri.Arc{{ID: "failed", PlaceID: "task:failed"}},
	}
	arcs, err := calculateArcs(transition, workerexecution.OutcomeCanceled)
	if err != nil {
		t.Fatalf("calculateArcs(CANCELED) error = %v, want nil", err)
	}
	if len(arcs) != 0 {
		t.Fatalf("calculateArcs(CANCELED) = %#v, want no business route", arcs)
	}
}

func TestTransitionerLogsCancellationAndFailuresWithSafeDispatchContext(t *testing.T) {
	cases := []transitionerLogCase{
		{
			name: "superseded cancellation",
			result: workerexecution.WorkResult{
				DispatchID: "dispatch-1", TransitionID: "t1", Outcome: workerexecution.OutcomeCanceled,
				Cancellation: &workerexecution.DispatchCancellation{Reason: workerexecution.DispatchCancellationReasonSuperseded},
				Error:        "raw command payload must not be logged",
			},
			wantLevel:   "info",
			wantMessage: "transitioner: result canceled",
			wantCancel:  string(workerexecution.DispatchCancellationReasonSuperseded),
		},
		{
			name: "timeout failure",
			result: workerexecution.WorkResult{
				DispatchID: "dispatch-1", TransitionID: "t1", Outcome: workerexecution.OutcomeFailed,
				FailureMetadata: &workerexecution.WorkFailureMetadata{Type: workerexecution.WorkFailureTypeTimeout},
				Error:           "raw provider prompt and secret should not be logged",
			},
			wantLevel:     "error",
			wantMessage:   "transitioner: result failed",
			wantReason:    string(workerexecution.WorkFailureTypeTimeout),
			wantSafeField: "failure_reason",
		},
		{
			name: "execution failure",
			result: workerexecution.WorkResult{
				DispatchID: "dispatch-1", TransitionID: "t1", Outcome: workerexecution.OutcomeFailed,
				FailureDetail: &workerexecution.FailureDetail{
					Reason:  workerexecution.WorkFailureTypeUnknown,
					Message: "worker process failed safely",
				},
				Error: "raw provider prompt and secret should not be logged",
			},
			wantLevel:     "error",
			wantMessage:   "transitioner: result failed",
			wantReason:    string(workerexecution.WorkFailureTypeUnknown),
			wantSafeField: "failure_message",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertTransitionerLogCase(t, tc)
		})
	}
}

func assertTransitionerLogCase(t *testing.T, tc transitionerLogCase) {
	t.Helper()
	net := workerBatchTestNet()
	logger := &transitionerLogCapture{}
	transitioner := NewTransitioner(
		net, logger, testSubsystemNow, testTokenTransformer(net),
		nil, nil, nil, testWorkPropagationPolicy(),
	)
	snapshot := workerBatchSnapshot("")
	snapshot.Results[0] = tc.result

	if _, err := transitioner.Execute(context.Background(), snapshot); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var outcome *transitionerLogEntry
	for index := range logger.entries {
		entry := &logger.entries[index]
		if entry.message == tc.wantMessage {
			outcome = entry
		}
	}
	if outcome == nil {
		t.Fatalf("logs = %#v, want %q", logger.entries, tc.wantMessage)
	}
	if outcome.level != tc.wantLevel {
		t.Fatalf("outcome log level = %q, want %q: %#v", outcome.level, tc.wantLevel, outcome)
	}
	for key, want := range map[string]any{
		"dispatch_id":   "dispatch-1",
		"transition_id": "t1",
		"work_id":       "work-source",
		"work_name":     "source",
	} {
		if outcome.fields[key] != want {
			t.Fatalf("log field %s = %#v, want %#v: %#v", key, outcome.fields[key], want, outcome.fields)
		}
	}
	if tc.wantCancel != "" && outcome.fields["cancellation_reason"] != tc.wantCancel {
		t.Fatalf("cancellation_reason = %#v, want %q", outcome.fields["cancellation_reason"], tc.wantCancel)
	}
	if tc.wantReason != "" && outcome.fields["failure_reason"] != tc.wantReason {
		t.Fatalf("failure_reason = %#v, want %q", outcome.fields["failure_reason"], tc.wantReason)
	}
	if tc.wantSafeField != "" && outcome.fields[tc.wantSafeField] == nil {
		t.Fatalf("safe field %q missing: %#v", tc.wantSafeField, outcome.fields)
	}
	if _, ok := outcome.fields["error"]; ok {
		t.Fatalf("outcome log exposed raw error: %#v", outcome.fields["error"])
	}
}
