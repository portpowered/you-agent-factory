package subsystems

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

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
		net,
		nil,
		func() time.Time { return now },
		testTokenTransformer(net),
		nil,
		nil,
		nil,
		testWorkPropagationPolicy(),
	)
	snapshot := workerBatchSnapshot("")
	snapshot.Dispatches["dispatch-1"].HeldMutations = []interfaces.MarkingMutation{
		{
			Type:      interfaces.MutationConsume,
			TokenID:   "tok-source",
			FromPlace: "task:init",
		},
	}
	snapshot.Results[0] = workerexecution.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeCanceled,
		Cancellation: workerexecution.NewDispatchCancellation(workerexecution.DispatchCancellationReasonSuperseded),
		Error:        "losing dispatch was superseded",
	}

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("Execute() result = nil, want canceled dispatch retirement")
	}
	if len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want exactly one", result.CompletedDispatches)
	}
	completed := result.CompletedDispatches[0]
	if completed.Outcome != workerexecution.OutcomeCanceled || completed.Cancellation == nil ||
		completed.Cancellation.Reason != workerexecution.DispatchCancellationReasonSuperseded {
		t.Fatalf("completed dispatch = %#v, want SUPERSEDED cancellation", completed)
	}
	if completed.FailureDetail != nil || completed.FailureMetadata != nil {
		t.Fatalf("completed cancellation retained failure facts: %#v", completed)
	}
	if len(result.Mutations) != 1 {
		t.Fatalf("mutations = %#v, want exactly one restored Work token", result.Mutations)
	}
	restored := result.Mutations[0]
	if restored.Type != interfaces.MutationCreate || restored.TokenID != "tok-source" || restored.ToPlace != "task:init" || restored.NewToken == nil {
		t.Fatalf("restored mutation = %#v, want CREATE tok-source at task:init", restored)
	}
	if restored.NewToken.ID != "tok-source" || restored.NewToken.State != "init" || restored.NewToken.EnteredAt != now {
		t.Fatalf("restored token = %#v, want original identity at cancellation time", restored.NewToken)
	}
	if restored.ToPlace == "task:failed" {
		t.Fatal("canceled dispatch emitted a terminal failure mutation")
	}
	if len(completed.OutputMutations) != 1 || completed.OutputMutations[0].Type != interfaces.MutationCreate {
		t.Fatalf("completed output mutations = %#v, want the non-failure restoration", completed.OutputMutations)
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
				Cancellation: workerexecution.NewDispatchCancellation(workerexecution.DispatchCancellationReasonSuperseded),
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
