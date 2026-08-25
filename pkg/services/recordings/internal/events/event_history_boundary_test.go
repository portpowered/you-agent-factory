package events

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

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
		ID:         "model-request-1",
		Kind:       workerexecution.ModelEventKindRequest,
		EventTime:  requestTime,
		Tick:       4,
		DispatchID: "dispatch-model-1",
		RequestID:  "request-1",
		TraceIDs:   traceIDs,
		WorkIDs:    workIDs,
		Request:    &request,
	})
	history.RecordModelEvent(workerexecution.ModelEvent{
		ID:         "model-response-1",
		Kind:       workerexecution.ModelEventKindResponse,
		EventTime:  responseTime,
		Tick:       5,
		DispatchID: "dispatch-model-1",
		RequestID:  "request-1",
		TraceIDs:   traceIDs,
		WorkIDs:    workIDs,
		Response:   &response,
	})

	traceIDs[0] = "mutated"
	workIDs[0] = "mutated"
	events := history.CanonicalEvents()
	if len(events) != 2 {
		t.Fatalf("canonical model events = %d, want request and response", len(events))
	}
	if events[0].Type != interfaces.FactoryEventTypeModelRequest || events[1].Type != interfaces.FactoryEventTypeModelResponse {
		t.Fatalf("model event types = [%s, %s], want MODEL_REQUEST then MODEL_RESPONSE", events[0].Type, events[1].Type)
	}
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

	var gotRequest workerexecution.ModelRequestEventPayload
	if err := events[0].DecodePayload(&gotRequest); err != nil {
		t.Fatalf("decode model request payload: %v", err)
	}
	if gotRequest.ModelRequestID != request.ModelRequestID || gotRequest.Attempt != request.Attempt {
		t.Fatalf("model request payload = %#v, want worker request facts", gotRequest)
	}
	var gotResponse workerexecution.ModelResponseEventPayload
	if err := events[1].DecodePayload(&gotResponse); err != nil {
		t.Fatalf("decode model response payload: %v", err)
	}
	if gotResponse.ModelRequestID != response.ModelRequestID || gotResponse.Outcome != response.Outcome {
		t.Fatalf("model response payload = %#v, want worker response facts", gotResponse)
	}

	// A malformed worker envelope is ignored rather than becoming an ambiguous
	// canonical event that a replay could not classify.
	history.RecordModelEvent(workerexecution.ModelEvent{
		ID:         "invalid-model-event",
		Kind:       workerexecution.ModelEventKindRequest,
		DispatchID: "dispatch-model-1",
		Response:   &response,
	})
	if got := len(history.CanonicalEvents()); got != 2 {
		t.Fatalf("canonical events after malformed model envelope = %d, want 2", got)
	}
}

func TestFactoryEventHistory_ValidatedAppendPublishesOnlyAcceptedEvents(t *testing.T) {
	history := newTestFactoryEventHistory(nil, time.Now)
	validationErr := errors.New("owner rejected event")
	event := interfaces.FactoryEvent{
		Id:      "validated-event",
		Type:    interfaces.FactoryEventTypeRunResponse,
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
		"",
		"/credentials/token",
		"/items/0/value",
		"/nested~1key/value",
		"/tilde~0key",
		"/items/1/value",
		"/credentials/missing",
		"/credentials/token~2bad",
		"credentials/token",
	})
	history.RecordRunRequest()
	events := history.CanonicalEvents()
	if len(events) != 1 {
		t.Fatalf("canonical run request events = %d, want one", len(events))
	}

	secrets := history.SecretProvenanceForEvent(events[0])
	wantPointers := []string{
		"/factory",
		"/factory/credentials/token",
		"/factory/items/0/value",
		"/factory/nested~1key/value",
		"/factory/tilde~0key",
	}
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
