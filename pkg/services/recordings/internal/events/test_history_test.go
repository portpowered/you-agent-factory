package events

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewFactoryEventHistoryRejectsMissingClockAndStreamGenerationID(t *testing.T) {
	t.Parallel()
	if history := NewFactoryEventHistory(nil, nil, "stream"); history != nil {
		t.Fatal("history constructed without a clock")
	}
	if history := NewFactoryEventHistory(nil, time.Now, ""); history != nil {
		t.Fatal("history constructed without a stream generation ID")
	}
}

func TestNewRuntimeLedgerRejectsMissingClockAndStreamGenerationID(t *testing.T) {
	t.Parallel()
	if ledger := NewRuntimeLedger(nil, nil, "stream", nil); ledger != nil {
		t.Fatal("ledger constructed without a clock")
	}
	if ledger := NewRuntimeLedger(nil, time.Now, "", nil); ledger != nil {
		t.Fatal("ledger constructed without a stream generation ID")
	}
	if ledger := NewRuntimeLedger(nil, time.Now, "stream", nil); ledger == nil {
		t.Fatal("ledger not constructed with clock and stream generation ID")
	}
}

func newTestFactoryEventHistory(
	topology recordings.InitialStructureSource,
	now func() time.Time,
	runtimeConfigs ...interfaces.RuntimeDefinitionLookup,
) *FactoryEventHistory {
	return NewFactoryEventHistory(topology, now, "test-stream-generation", runtimeConfigs...)
}

// Every exported FactoryEventHistory method opens with an `if h == nil` guard so
// callers holding an unconstructed history -- a service wired before its
// recordings root is activated -- observe documented zero values instead of a
// panic. Nothing exercised those guards directly, so the contract was only
// incidentally covered by tests that happened to hold a real history. These two
// tests assert the guards themselves: delete one and the corresponding call
// panics here rather than in a caller far from the cause.

// TestNilHistoryRecordingCallsAreNoOps covers the recording and mutation methods,
// which return nothing and must simply not panic on a nil history.
func TestNilHistoryRecordingCallsAreNoOps(t *testing.T) {
	var history *FactoryEventHistory
	var token workerexecution.Token
	var state interfaces.FactoryState
	var relation work.FactoryRelation
	when := time.Unix(0, 0).UTC()

	// A panic in any call below fails the test by unwinding it.
	history.AddEventRecorder(func(interfaces.FactoryEvent) {})
	history.AddEventTypeRecorder(func(interfaces.FactoryEventType) {})
	history.CloseLiveSubscriptions()
	history.RecordInitialStructure()
	history.RecordRunRequest()
	history.SetInitialStructureFactory(nil)
	history.SetFactoryRunnerOverride("runner-override")
	history.RecordRunResponse(1, state, "reason", when)
	history.RecordFactoryChange(1, interfaces.FactoryChangeEventPayload{}, when)
	history.RecordRelationshipChange(1, "request-id", "trace-id", 0, relation, when)
	history.RecordWorkInput(1, work.SubmitRequest{}, token, when)
	history.RecordWorkRequest(1, work.WorkRequestRecord{}, when)
	history.RecordWorkstationRequest(1, interfaces.FactoryDispatchRecord{}, when)
	history.RecordWorkstationResponse(1, workerexecution.WorkResult{}, interfaces.CompletedDispatch{})
	history.RecordHumanApprovalRequested(1, interfaces.FactoryDispatchRecord{}, when)
	history.RecordDispatchWorkerSessionAssociation(1, "dispatch-id", "worker-session-id", "request-id", when)
}

// TestNilHistoryQueriesReturnDocumentedZeroValues covers the methods that return
// a value, where the guard's contract is the specific value it yields.
func TestNilHistoryQueriesReturnDocumentedZeroValues(t *testing.T) {
	var history *FactoryEventHistory

	if events := history.CanonicalEvents(); events != nil {
		t.Fatalf("CanonicalEvents() on a nil history = %#v, want nil", events)
	}
	if generation := history.StreamGenerationID(); generation != "" {
		t.Fatalf("StreamGenerationID() on a nil history = %q, want an empty string", generation)
	}

	if _, err := history.AppendRecordedEventWithResult(interfaces.FactoryEvent{}); err == nil {
		t.Fatal("AppendRecordedEventWithResult() on a nil history returned no error, want an unavailable-history error")
	}
	validated := func(interfaces.FactoryEvent) error { return nil }
	if _, err := history.AppendRecordedEventWithValidation(interfaces.FactoryEvent{}, validated); err == nil {
		t.Fatal("AppendRecordedEventWithValidation() on a nil history returned no error, want an unavailable-history error")
	}

	// A nil history still hands back a usable, already-closed stream so a
	// subscriber's receive loop terminates instead of blocking forever.
	stream, err := history.Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("Subscribe() on a nil history returned error %v, want a closed stream and no error", err)
	}
	if _, open := <-stream.Events; open {
		t.Fatal("Subscribe() on a nil history yielded an open event channel, want it already closed")
	}
}
