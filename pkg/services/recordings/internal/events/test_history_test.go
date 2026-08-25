package events

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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

func TestRunFinishedFactoryEventPreservesTerminalClockInUTC(t *testing.T) {
	startedAt := time.Date(2026, 8, 24, 9, 10, 11, 0, time.FixedZone("factory-local", -7*60*60))
	finishedAt := startedAt.Add(17 * time.Second)

	event := RunFinishedFactoryEvent(startedAt, finishedAt)
	if event.Id != RunFinishedFactoryEventID || event.SchemaVersion != interfaces.FactoryEventSchemaVersionV1 {
		t.Fatalf("terminal event identity = (%q, %q), want stable ID and schema", event.Id, event.SchemaVersion)
	}
	if event.Type != interfaces.FactoryEventTypeRunResponse || !event.Context.EventTime.Equal(finishedAt.UTC()) {
		t.Fatalf("terminal event envelope = %#v, want RUN_RESPONSE at the UTC finish time", event)
	}

	var payload recordings.RunResponseEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode terminal event payload: %v", err)
	}
	if payload.State == nil || *payload.State != recordings.FactoryStateCompleted || payload.WallClock == nil {
		t.Fatalf("terminal payload = %#v, want completed state and wall clock", payload)
	}
	if payload.WallClock.StartedAt == nil || !payload.WallClock.StartedAt.Equal(startedAt.UTC()) ||
		payload.WallClock.FinishedAt == nil || !payload.WallClock.FinishedAt.Equal(finishedAt.UTC()) {
		t.Fatalf("terminal wall clock = %#v, want both instants normalized to UTC", payload.WallClock)
	}
}

func TestFactoryEventHistory_RecordModelEventsPreservesTypedAttemptBoundaries(t *testing.T) {
	history := newTestFactoryEventHistory(nil, time.Now)
	requestTime := time.Date(2026, 8, 24, 10, 0, 0, 0, time.FixedZone("model-local", -7*60*60))
	responseTime := requestTime.Add(250 * time.Millisecond)
	request := workerexecution.ModelRequestEventPayload{
		Attempt:        2,
		Model:          "local-model",
		ModelRequestID: "model-request-1",
		Operation:      "chat",
		Worker:         "worker-1",
	}
	response := workerexecution.ModelResponseEventPayload{
		Attempt:        2,
		DurationMillis: 250,
		Model:          request.Model,
		ModelRequestID: request.ModelRequestID,
		Operation:      request.Operation,
		Outcome:        workerexecution.InferenceOutcomeSucceeded,
		Worker:         request.Worker,
	}
	traceIDs := []string{"trace-1"}
	workIDs := []string{"work-1"}
	history.RecordModelEvent(workerexecution.ModelEvent{
		ID: "model-request-1", Kind: workerexecution.ModelEventKindRequest, EventTime: requestTime,
		Tick: 4, DispatchID: "dispatch-model-1", RequestID: "request-1", TraceIDs: traceIDs, WorkIDs: workIDs, Request: &request,
	})
	history.RecordModelEvent(workerexecution.ModelEvent{
		ID: "model-response-1", Kind: workerexecution.ModelEventKindResponse, EventTime: responseTime,
		Tick: 5, DispatchID: "dispatch-model-1", RequestID: "request-1", TraceIDs: traceIDs, WorkIDs: workIDs, Response: &response,
	})
	traceIDs[0] = "mutated"
	workIDs[0] = "mutated"
	assertModelEventHistoryFacts(t, history, request, response)
	history.RecordModelEvent(workerexecution.ModelEvent{
		ID: "invalid-model-event", Kind: workerexecution.ModelEventKindRequest,
		DispatchID: "dispatch-model-1", Response: &response,
	})
	if got := len(history.CanonicalEvents()); got != 2 {
		t.Fatalf("canonical events after malformed model envelope = %d, want 2", got)
	}
}

func assertModelEventHistoryFacts(
	t *testing.T,
	history *FactoryEventHistory,
	request workerexecution.ModelRequestEventPayload,
	response workerexecution.ModelResponseEventPayload,
) {
	t.Helper()
	events := history.CanonicalEvents()
	if len(events) != 2 {
		t.Fatalf("canonical model events = %d, want request and response", len(events))
	}
	if events[0].Type != interfaces.FactoryEventTypeModelRequest || events[1].Type != interfaces.FactoryEventTypeModelResponse {
		t.Fatalf("model event types = [%s, %s], want MODEL_REQUEST then MODEL_RESPONSE", events[0].Type, events[1].Type)
	}
	assertModelEventContexts(t, events)
	assertModelRequestPayload(t, events[0], request)
	assertModelResponsePayload(t, events[1], response)
}

func assertModelEventContexts(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	for index, event := range events {
		if event.Context.DispatchID == nil || *event.Context.DispatchID != "dispatch-model-1" ||
			event.Context.TraceIDs == nil || !reflect.DeepEqual(*event.Context.TraceIDs, []string{"trace-1"}) ||
			event.Context.WorkIDs == nil || !reflect.DeepEqual(*event.Context.WorkIDs, []string{"work-1"}) {
			t.Fatalf("model event %d context = %#v, want detached dispatch and lineage facts", index, event.Context)
		}
		if event.Context.EventTime.Location() != time.UTC {
			t.Fatalf("model event %d time location = %s, want UTC", index, event.Context.EventTime.Location())
		}
	}
}

func assertModelRequestPayload(t *testing.T, event interfaces.FactoryEvent, want workerexecution.ModelRequestEventPayload) {
	t.Helper()
	var got workerexecution.ModelRequestEventPayload
	if err := event.DecodePayload(&got); err != nil {
		t.Fatalf("decode model request payload: %v", err)
	}
	if got.ModelRequestID != want.ModelRequestID || got.Attempt != want.Attempt {
		t.Fatalf("model request payload = %#v, want worker request facts", got)
	}
}

func assertModelResponsePayload(t *testing.T, event interfaces.FactoryEvent, want workerexecution.ModelResponseEventPayload) {
	t.Helper()
	var got workerexecution.ModelResponseEventPayload
	if err := event.DecodePayload(&got); err != nil {
		t.Fatalf("decode model response payload: %v", err)
	}
	if got.ModelRequestID != want.ModelRequestID || got.Outcome != want.Outcome {
		t.Fatalf("model response payload = %#v, want worker response facts", got)
	}
}

func TestFactoryEventHistory_ValidatedAppendPublishesOnlyAcceptedEvents(t *testing.T) {
	history := newTestFactoryEventHistory(nil, time.Now)
	validationErr := errors.New("owner rejected event")
	event := interfaces.FactoryEvent{
		Id: "validated-event", Type: interfaces.FactoryEventTypeRunResponse,
		Context: interfaces.FactoryEventContext{EventTime: time.Date(2026, 8, 24, 11, 0, 0, 0, time.FixedZone("local", -7*60*60))},
		Payload: json.RawMessage(`{"state":"COMPLETED"}`),
	}
	if _, err := history.AppendRecordedEventWithValidation(event, func(candidate interfaces.FactoryEvent) error {
		if candidate.Context.Sequence != 0 || candidate.Context.EventTime.Location() != time.UTC {
			t.Fatalf("validation candidate = %#v, want assigned sequence and UTC time", candidate.Context)
		}
		return validationErr
	}); !errors.Is(err, validationErr) {
		t.Fatalf("rejected append error = %v, want %v", err, validationErr)
	}
	if got := history.CanonicalEvents(); len(got) != 0 {
		t.Fatalf("canonical events after rejected append = %#v, want none", got)
	}
	accepted, err := history.AppendRecordedEventWithValidation(event, func(candidate interfaces.FactoryEvent) error {
		if candidate.Context.Sequence != 0 {
			t.Fatalf("accepted candidate sequence = %d, want 0 after rejected append", candidate.Context.Sequence)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("accepted append error = %v", err)
	}
	if accepted.Context.Sequence != 0 || len(history.CanonicalEvents()) != 1 {
		t.Fatalf("accepted append = %#v, history = %#v, want one sequence-zero event", accepted, history.CanonicalEvents())
	}
}

func TestFactoryEventHistory_InvocationSecretPointersKeepOnlyExistingFactoryPaths(t *testing.T) {
	history := newTestFactoryEventHistory(nil, time.Now)
	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{
		"credentials": map[string]any{"token": "secret"},
		"items":       []any{map[string]any{"value": "first"}},
		"nested/key":  map[string]any{"value": "slash"},
		"tilde~key":   "tilde",
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	history.SetInitialStructureFactory(snapshot)
	history.SetInvocationSensitiveJSONPointers([]string{
		"", "/credentials/token", "/items/0/value", "/nested~1key/value", "/tilde~0key",
		"/items/1/value", "/credentials/missing", "/credentials/token~2bad", "credentials/token",
	})
	history.RecordRunRequest()
	events := history.CanonicalEvents()
	if len(events) != 1 {
		t.Fatalf("canonical run request events = %d, want one", len(events))
	}
	secrets := history.SecretProvenanceForEvent(events[0])
	wantPointers := []string{"/factory", "/factory/credentials/token", "/factory/items/0/value", "/factory/nested~1key/value", "/factory/tilde~0key"}
	if len(secrets) != len(wantPointers) {
		t.Fatalf("secret provenance = %#v, want only existing paths %#v", secrets, wantPointers)
	}
	for index, secret := range secrets {
		if secret.JSONPointer != wantPointers[index] || secret.Provenance != recordings.RecordingSecretProvenanceDeclared {
			t.Fatalf("secret provenance[%d] = %#v, want declared %q", index, secret, wantPointers[index])
		}
	}
	secrets[0].JSONPointer = "/mutated"
	if got := history.SecretProvenanceForEvent(events[0])[0].JSONPointer; got != "/factory" {
		t.Fatalf("stored secret provenance was aliased through returned slice: %q", got)
	}
}

func TestFactoryEventHistory_RuntimeReadObservationsAndCountersAreDetached(t *testing.T) {
	history := newTestFactoryEventHistory(nil, time.Now)
	var observed []recordings.RuntimeReadMetric
	history.SetRuntimeReadMetricsRecorder(func(metric recordings.RuntimeReadMetric) {
		metric.Labels["callback"] = "copy"
		observed = append(observed, metric)
	})
	labels := map[string]string{"scope": "live"}
	history.RecordRuntimeReadMetric(recordings.RuntimeReadMetric{Name: "history-read", Labels: labels})
	if labels["callback"] != "" {
		t.Fatal("runtime metric recorder received the caller's labels map by alias")
	}
	if len(observed) != 1 || observed[0].Name != "history-read" || observed[0].Labels["scope"] != "live" || observed[0].Labels["callback"] != "copy" {
		t.Fatalf("observed metrics = %#v, want one detached labeled observation", observed)
	}
	history.CanonicalEvents()
	history.CanonicalEvents()
	history.RecordCanonicalHistoryReduction()
	stats := history.CanonicalHistoryReadStats()
	if stats.CanonicalEventsCalls != 2 || stats.CanonicalEventsCopied != 0 || stats.FullHistoryReductions != 1 {
		t.Fatalf("history read stats = %#v, want two reads, zero copied events, one reduction", stats)
	}
}
