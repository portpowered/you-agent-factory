package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

type recordedLogEntry struct {
	message string
	fields  map[string]any
}

// recordingLogger implements logging.Logger for asserting the exact safe
// fields the workstation pool emits without depending on a concrete logging
// backend.
type recordingLogger struct {
	mu      sync.Mutex
	entries []recordedLogEntry
}

func (l *recordingLogger) Debug(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Info(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Warn(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Error(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Verbose(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) record(message string, keysAndValues []any) {
	fields := make(map[string]any, len(keysAndValues)/2)
	for index := 0; index+1 < len(keysAndValues); index += 2 {
		key, ok := keysAndValues[index].(string)
		if !ok {
			continue
		}
		fields[key] = keysAndValues[index+1]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, recordedLogEntry{message: message, fields: fields})
}

func (l *recordingLogger) entriesFor(message string) []recordedLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var matches []recordedLogEntry
	for _, entry := range l.entries {
		if entry.message == message {
			matches = append(matches, entry)
		}
	}
	return matches
}

func TestPoolLogsStartAcceptedAndTerminalDispatchOutcomes(t *testing.T) {
	t.Parallel()

	logger := &recordingLogger{}
	pool := New(logger)
	executor := &recordingExecutor{
		result: workers.WorkResult{Outcome: workers.OutcomeAccepted, Output: "done"},
	}
	if _, err := pool.start(context.Background(), []workstations.Route{
		{WorkstationName: "review", Executor: executor},
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	starts := logger.entriesFor("workers workstation pool start")
	if len(starts) != 1 ||
		starts[0].fields["outcome"] != string(workers.WorkstationPoolLifecycleOutcomeStarted) {
		t.Fatalf("start log = %#v", starts)
	}

	if _, err := pool.start(
		context.Background(),
		[]workstations.Route{{WorkstationName: "other"}},
	); err != nil {
		t.Fatalf("repeated Start() error = %v", err)
	}
	starts = logger.entriesFor("workers workstation pool start")
	if len(starts) != 2 ||
		starts[1].fields["outcome"] != string(workers.WorkstationPoolLifecycleOutcomeAlreadyRunning) {
		t.Fatalf("repeated start log = %#v", starts)
	}

	result, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-review", "transition-review", "review"),
	)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
		t.Fatalf("Dispatch() terminal outcome = %q", result.TerminalOutcome)
	}

	accepted := logger.entriesFor("workers workstation dispatch accepted")
	if len(accepted) != 1 || len(accepted[0].fields) != 2 ||
		accepted[0].fields["workstation_name"] != "review" ||
		accepted[0].fields["dispatch_id"] != "dispatch-review" {
		t.Fatalf("accepted log = %#v", accepted)
	}

	terminal := logger.entriesFor("workers workstation dispatch terminal")
	if len(terminal) != 1 || len(terminal[0].fields) != 3 ||
		terminal[0].fields["workstation_name"] != "review" ||
		terminal[0].fields["dispatch_id"] != "dispatch-review" ||
		terminal[0].fields["terminal_outcome"] != string(workers.WorkstationDispatchTerminalOutcomeCompleted) {
		t.Fatalf("terminal log = %#v", terminal)
	}
}

func TestPoolLogsCancellationOutcomesIncludingNoOps(t *testing.T) {
	t.Parallel()

	logger := &recordingLogger{}
	pool := New(logger)
	cancelExecutor := newControlledExecutor("dispatch-cancel")
	completeExecutor := newControlledExecutor("dispatch-complete")
	if _, err := pool.start(context.Background(), []workstations.Route{
		{
			WorkstationName: "review",
			Executor:        cancelExecutor,
			Capacity:        1,
			QueueCapacity:   1,
		},
		{
			WorkstationName: "implement",
			Executor:        completeExecutor,
			Capacity:        1,
			QueueCapacity:   1,
		},
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	canceledCompletion := dispatchResultAsync(pool, context.Background(), "dispatch-cancel", "review")
	assertStarted(t, cancelExecutor, "dispatch-cancel")

	canceled, err := pool.Cancel(
		context.Background(),
		workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-cancel"},
	)
	if err != nil || canceled.Outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
		t.Fatalf("Cancel() = %#v, %v", canceled, err)
	}
	<-canceledCompletion

	cancelEntries := logger.entriesFor("workers workstation dispatch cancellation")
	if len(cancelEntries) != 1 ||
		cancelEntries[0].fields["dispatch_id"] != "dispatch-cancel" ||
		cancelEntries[0].fields["outcome"] != string(workers.WorkstationDispatchCancelOutcomeCanceled) {
		t.Fatalf("cancellation log = %#v", cancelEntries)
	}
	terminalEntries := logger.entriesFor("workers workstation dispatch terminal")
	if len(terminalEntries) != 1 ||
		terminalEntries[0].fields["terminal_outcome"] != string(workers.WorkstationDispatchTerminalOutcomeCanceled) {
		t.Fatalf("terminal log after cancel = %#v", terminalEntries)
	}

	repeatCanceled, err := pool.Cancel(
		context.Background(),
		workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-cancel"},
	)
	if err != nil || repeatCanceled.Outcome != workers.WorkstationDispatchCancelOutcomeAlreadyCanceled {
		t.Fatalf("repeated Cancel() = %#v, %v", repeatCanceled, err)
	}
	cancelEntries = logger.entriesFor("workers workstation dispatch cancellation")
	if len(cancelEntries) != 2 ||
		cancelEntries[1].fields["outcome"] != string(workers.WorkstationDispatchCancelOutcomeAlreadyCanceled) {
		t.Fatalf("repeated cancellation log = %#v", cancelEntries)
	}
	// A repeated no-op cancellation must not replace or duplicate the
	// canonical terminal-completion record.
	if terminalEntries := logger.entriesFor("workers workstation dispatch terminal"); len(terminalEntries) != 1 {
		t.Fatalf("terminal log after repeat cancel = %#v", terminalEntries)
	}

	completeSignal := dispatchResultAsync(pool, context.Background(), "dispatch-complete", "implement")
	assertStarted(t, completeExecutor, "dispatch-complete")
	completeExecutor.release("dispatch-complete")
	<-completeSignal

	lateCancel, err := pool.Cancel(
		context.Background(),
		workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-complete"},
	)
	if lateCancel.Outcome != workers.WorkstationDispatchCancelOutcomeAlreadyTerminal ||
		!errors.Is(err, workers.ErrWorkstationDispatchAlreadyTerminal) {
		t.Fatalf("late Cancel() = %#v, %v", lateCancel, err)
	}
	cancelEntries = logger.entriesFor("workers workstation dispatch cancellation")
	if len(cancelEntries) != 3 ||
		cancelEntries[2].fields["dispatch_id"] != "dispatch-complete" ||
		cancelEntries[2].fields["outcome"] != string(workers.WorkstationDispatchCancelOutcomeAlreadyTerminal) {
		t.Fatalf("late cancellation log = %#v", cancelEntries)
	}
}
