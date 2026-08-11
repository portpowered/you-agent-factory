package livechange

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type eventLog struct {
	events       []interfaces.FactoryEvent
	failTypeOnce interfaces.FactoryEventType
}

func (log *eventLog) AppendLiveChangeEvent(event interfaces.FactoryEvent) (interfaces.FactoryEvent, error) {
	if log.failTypeOnce == event.Type {
		log.failTypeOnce = ""
		return interfaces.FactoryEvent{}, errors.New("injected append failure")
	}
	event.SchemaVersion = interfaces.FactoryEventSchemaVersionV1
	event.Context.Sequence = len(log.events)
	if event.Type == interfaces.FactoryEventTypeFactoryChange {
		var payload interfaces.FactoryChangeEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil && payload.ChangeID != "" {
			sequence := event.Context.Sequence
			payload.EffectiveSequence = &sequence
			event.Payload, _ = json.Marshal(payload)
		}
	}
	log.events = append(log.events, event.Clone())
	return event.Clone(), nil
}

func (log *eventLog) LiveChangeEvents() []interfaces.FactoryEvent {
	cloned := make([]interfaces.FactoryEvent, len(log.events))
	for index, event := range log.events {
		cloned[index] = event.Clone()
	}
	return cloned
}

type application struct {
	applyCalls   int
	preflight    factorysessions.LiveChangePreflightResult
	preflightErr error
	applyResult  factorysessions.LiveChangeApplicationResult
	applyErr     error
}

func (app *application) PreflightLiveChange(context.Context, factorysessions.LiveChangeApplicationRequest) (factorysessions.LiveChangePreflightResult, error) {
	return app.preflight, app.preflightErr
}

func (app *application) ApplyLiveChange(context.Context, factorysessions.LiveChangeApplicationRequest) (factorysessions.LiveChangeApplicationResult, error) {
	app.applyCalls++
	return app.applyResult, app.applyErr
}

func testRequest(value string) factorysessions.LiveChangeRequest {
	return factorysessions.LiveChangeRequest{
		RequestID:        "request-1",
		ExpectedRevision: 2,
		Operation:        " resource.capacity.set ",
		TargetID:         "reviewers",
		RequestedValue:   json.RawMessage(value),
		Actor:            " operator ",
		Source:           " api ",
		Reason:           "raise\n throughput",
	}
}

func testState(t *testing.T, lifecycle factorysessions.LiveChangeLifecycle) factorysessions.LiveChangeSessionState {
	t.Helper()
	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{"name": "factory", "resources": []any{}})
	if err != nil {
		t.Fatalf("create test snapshot: %v", err)
	}
	return factorysessions.LiveChangeSessionState{
		SessionID:         "session-1",
		Lifecycle:         lifecycle,
		EffectiveRevision: 2,
		Factory:           snapshot,
	}
}

func TestApplyLiveChange_AppendsCorrelatedRequestAndSuccessOnce(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	service := New(func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC) }, zap.New(core))
	log := &eventLog{}
	state := testState(t, factorysessions.LiveChangeLifecycleRunning)
	app := successfulApplication(t)

	result, err := service.Apply(context.Background(), "session-1", testRequest(" 8 "), func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
		return state, nil
	}, log, app)
	if err != nil {
		t.Fatalf("apply live change: %v", err)
	}
	assertSuccessfulLiveChange(t, result, log, app)

	replayed, err := service.Apply(context.Background(), "session-1", testRequest("8"), func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
		return testState(t, factorysessions.LiveChangeLifecycleCompleted), nil
	}, log, app)
	if err != nil {
		t.Fatalf("replay live change: %v", err)
	}
	if replayed.Outcome != factorysessions.LiveChangeOutcomeReplayed || len(log.events) != 2 || app.applyCalls != 1 ||
		replayed.ResourceCapacity == nil || replayed.ResourceCapacity.ResourceID != "reviewers" ||
		replayed.ResourceCapacity.AvailableCount != 7 || replayed.ResourceCapacity.EffectiveCapacity != 8 {
		t.Fatalf("replay = %#v, events=%d applicationCalls=%d", replayed, len(log.events), app.applyCalls)
	}
	assertSafeLiveChangeLogs(t, observed)
}

func successfulApplication(t *testing.T) *application {
	t.Helper()
	updated, err := interfaces.NewFactorySnapshot(map[string]any{"name": "factory-next", "resources": []any{"reviewers"}})
	if err != nil {
		t.Fatalf("create updated snapshot: %v", err)
	}
	return &application{
		preflight: factorysessions.LiveChangePreflightResult{Admissible: true},
		applyResult: factorysessions.LiveChangeApplicationResult{
			Factory: updated,
			ResourceCapacity: &factoryruntime.ResourceCapacityResult{
				ResourceID: "reviewers", ResourceName: "Review Pool", PreviousCapacity: 1,
				RequestedCapacity: 8, EffectiveCapacity: 8, InUseCount: 1,
				AvailableCount: 7, MinimumCapacity: 1,
				Outcome: factoryruntime.ResourceCapacityOutcomeApplied,
			},
		},
	}
}

func assertSuccessfulLiveChange(t *testing.T, result factorysessions.LiveChangeResult, log *eventLog, app *application) {
	t.Helper()
	if result.Outcome != factorysessions.LiveChangeOutcomeApplied || result.PreviousRevision != 2 || result.NewRevision != 3 || result.EffectiveSequence != 1 {
		t.Fatalf("result = %#v, want applied revision 2->3 at sequence 1", result)
	}
	if len(log.events) != 2 || log.events[0].Type != interfaces.FactoryEventTypeFactoryChangeRequest || log.events[1].Type != interfaces.FactoryEventTypeFactoryChange {
		t.Fatalf("events = %#v, want request followed by success", log.events)
	}
	assertLiveChangeCorrelation(t, log.events)
	assertLiveChangeRequestPayload(t, log.events[0])
	assertLiveChangeSuccessPayload(t, log.events[1])
	if app.applyCalls != 1 {
		t.Fatalf("application calls = %d, want 1", app.applyCalls)
	}
}

func assertLiveChangeCorrelation(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	for index, event := range events {
		if event.Context.RequestID == nil || *event.Context.RequestID != "request-1" {
			t.Fatalf("event[%d] request correlation = %#v", index, event.Context.RequestID)
		}
	}
}

func assertLiveChangeRequestPayload(t *testing.T, event interfaces.FactoryEvent) {
	t.Helper()
	var payload interfaces.FactoryChangeRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode request payload: %v", err)
	}
	if payload.ChangeID != "live-change/request-1" || payload.Operation != "resource.capacity.set" || payload.Reason != "raise throughput" {
		t.Fatalf("normalized request payload = %#v", payload)
	}
}

func assertLiveChangeSuccessPayload(t *testing.T, event interfaces.FactoryEvent) {
	t.Helper()
	var payload interfaces.FactoryChangeEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode success payload: %v", err)
	}
	assertLiveChangeRevisionedSnapshot(t, payload)
	assertLiveChangeResourceCapacityAccounting(t, payload.ResourceCapacity)
}

func assertLiveChangeRevisionedSnapshot(t *testing.T, payload interfaces.FactoryChangeEventPayload) {
	t.Helper()
	if payload.PreviousRevision == nil || *payload.PreviousRevision != 2 ||
		payload.NewRevision == nil || *payload.NewRevision != 3 ||
		payload.EffectiveSequence == nil || *payload.EffectiveSequence != 1 || payload.Factory == nil {
		t.Fatalf("success payload = %#v, want complete revisioned snapshot", payload)
	}
}

func assertLiveChangeResourceCapacityAccounting(t *testing.T, accounting *interfaces.FactoryResourceCapacityChange) {
	t.Helper()
	if accounting == nil {
		t.Fatal("resource capacity payload is nil, want detached accounting")
	}
	if accounting.ResourceID != "reviewers" || accounting.PreviousCapacity != 1 || accounting.RequestedCapacity != 8 ||
		accounting.EffectiveCapacity != 8 || accounting.InUseCount != 1 || accounting.AvailableCount != 7 ||
		accounting.MinimumCapacity != 1 || accounting.Outcome != string(factoryruntime.ResourceCapacityOutcomeApplied) {
		t.Fatalf("resource capacity payload = %#v, want detached accounting", accounting)
	}
}

func assertSafeLiveChangeLogs(t *testing.T, observed *observer.ObservedLogs) {
	t.Helper()
	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("log entries = %d, want admission and terminal outcome", len(entries))
	}
	for _, entry := range entries {
		fields := entry.ContextMap()
		if _, ok := fields["requested_value"]; ok {
			t.Fatalf("safe log contains requested value: %#v", fields)
		}
		if _, ok := fields["reason"]; ok {
			t.Fatalf("safe log contains reason: %#v", fields)
		}
		assertLiveChangeLogFields(t, fields)
	}
}

func assertLiveChangeLogFields(t *testing.T, fields map[string]interface{}) {
	t.Helper()
	for _, key := range []string{"session_id", "request_id", "change_id", "revision", "operation", "target_id"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("safe log fields = %#v, missing %q", fields, key)
		}
	}
}

func TestApplyLiveChange_PreAdmissionRejectionsDoNotAppend(t *testing.T) {
	tests := []struct {
		name      string
		request   factorysessions.LiveChangeRequest
		lifecycle factorysessions.LiveChangeLifecycle
		preflight factorysessions.LiveChangePreflightResult
		want      error
	}{
		{name: "stale revision", request: testRequest("1"), lifecycle: factorysessions.LiveChangeLifecycleRunning, want: factorysessions.ErrLiveChangeRevisionConflict},
		{name: "terminal lifecycle", request: testRequest("1"), lifecycle: factorysessions.LiveChangeLifecycleCompleted, want: factorysessions.ErrLiveChangeLifecycleConflict},
		{name: "exact no-op", request: testRequest("1"), lifecycle: factorysessions.LiveChangeLifecycleRunning, preflight: factorysessions.LiveChangePreflightResult{NoOp: true}, want: factorysessions.ErrLiveChangeNoOp},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			log := &eventLog{}
			state := testState(t, test.lifecycle)
			if test.name == "stale revision" {
				state.EffectiveRevision = 3
			}
			app := &application{preflight: test.preflight, applyResult: factorysessions.LiveChangeApplicationResult{Factory: state.Factory}}
			_, err := New(nil, nil).Apply(context.Background(), "session-1", test.request, func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
				return state, nil
			}, log, app)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want errors.Is(..., %v)", err, test.want)
			}
			if len(log.events) != 0 || app.applyCalls != 0 {
				t.Fatalf("pre-admission rejection mutated events=%d applicationCalls=%d", len(log.events), app.applyCalls)
			}
		})
	}
}

func TestNormalizeLiveChangeRequest_CanonicalizesBodyAndRejectsMalformedInput(t *testing.T) {
	normalized, err := factorysessions.NormalizeLiveChangeRequest(factorysessions.LiveChangeRequest{
		RequestID:        " request-1 ",
		ExpectedRevision: 0,
		Operation:        " RESOURCE.CAPACITY.SET ",
		TargetID:         " reviewers ",
		RequestedValue:   json.RawMessage(" {\"capacity\": 8, \"available\": [3, 2]} "),
		Reason:           " raise\n throughput ",
	})
	if err != nil {
		t.Fatalf("NormalizeLiveChangeRequest: %v", err)
	}
	if normalized.RequestID != "request-1" || normalized.ChangeID != "live-change/request-1" ||
		normalized.Operation != "resource.capacity.set" || normalized.TargetID != "reviewers" ||
		string(normalized.RequestedValue) != `{"available":[3,2],"capacity":8}` || normalized.Reason != "raise throughput" {
		t.Fatalf("normalized request = %#v, want canonical identity/body", normalized)
	}

	_, err = factorysessions.NormalizeLiveChangeRequest(factorysessions.LiveChangeRequest{
		RequestID: "request-invalid", ExpectedRevision: 0, Operation: "change", TargetID: "target",
		RequestedValue: json.RawMessage("not-json"),
	})
	var typed *factorysessions.LiveChangeError
	if !errors.As(err, &typed) || typed.Code != factorysessions.LiveChangeErrorInvalidRequest || typed.Field != "requestedValue" {
		t.Fatalf("malformed requested value error = %v, want typed requestedValue validation", err)
	}
}

func TestApplyLiveChange_RequestIDConflictAndChangeIDCollisionDoNotMutate(t *testing.T) {
	state := testState(t, factorysessions.LiveChangeLifecycleRunning)
	updated, err := interfaces.NewFactorySnapshot(map[string]any{"name": "updated"})
	if err != nil {
		t.Fatalf("create updated snapshot: %v", err)
	}
	app := &application{
		preflight:   factorysessions.LiveChangePreflightResult{Admissible: true},
		applyResult: factorysessions.LiveChangeApplicationResult{Factory: updated},
	}
	log := &eventLog{}
	service := New(nil, nil)
	if _, err := service.Apply(context.Background(), "session-1", testRequest("1"), func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
		return state, nil
	}, log, app); err != nil {
		t.Fatalf("initial live change: %v", err)
	}

	conflicting := testRequest("2")
	_, err = service.Apply(context.Background(), "session-1", conflicting, func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
		return state, nil
	}, log, app)
	if !errors.Is(err, factorysessions.ErrLiveChangeRequestConflict) || len(log.events) != 2 || app.applyCalls != 1 {
		t.Fatalf("same request ID conflict = %v events=%d applyCalls=%d, want typed conflict without mutation", err, len(log.events), app.applyCalls)
	}

	colliding := testRequest("1")
	colliding.RequestID = "request-2"
	colliding.ChangeID = "live-change/request-1"
	_, err = service.Apply(context.Background(), "session-1", colliding, func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
		return state, nil
	}, log, app)
	if !errors.Is(err, factorysessions.ErrLiveChangeRequestConflict) || len(log.events) != 2 || app.applyCalls != 1 {
		t.Fatalf("change ID collision = %v events=%d applyCalls=%d, want typed conflict without mutation", err, len(log.events), app.applyCalls)
	}
}

func TestApplyLiveChange_MissingSessionIsRejectedBeforeDependenciesOrEvents(t *testing.T) {
	log := &eventLog{}
	_, err := New(nil, nil).Apply(context.Background(), " ", testRequest("1"), nil, log, nil)
	if !errors.Is(err, factorysessions.ErrLiveChangeSessionNotFound) || len(log.events) != 0 {
		t.Fatalf("missing session error = %v events=%d, want typed rejection without append", err, len(log.events))
	}
}

func TestApplyLiveChange_AdmittedApplicationFailureClosesAndReplays(t *testing.T) {
	log := &eventLog{}
	state := testState(t, factorysessions.LiveChangeLifecycleRunning)
	app := &application{
		preflight: factorysessions.LiveChangePreflightResult{Admissible: true},
		applyErr:  errors.New("provider secret and stack should not escape"),
	}
	service := New(nil, nil)
	result, err := service.Apply(context.Background(), "session-1", testRequest("1"), func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
		return state, nil
	}, log, app)
	if !errors.Is(err, factorysessions.ErrLiveChangeApplicationFailed) || result.Outcome != factorysessions.LiveChangeOutcomeFailed {
		t.Fatalf("failure result=%#v error=%v", result, err)
	}
	if len(log.events) != 2 || log.events[1].Type != interfaces.FactoryEventTypeFactoryChangeFailed {
		t.Fatalf("events = %#v, want request and failure", log.events)
	}
	var failure interfaces.FactoryChangeFailedEventPayload
	if err := log.events[1].DecodePayload(&failure); err != nil {
		t.Fatalf("decode failure: %v", err)
	}
	if failure.FailureMessage != "live change application failed" || failure.PreviousRevision != 2 {
		t.Fatalf("failure payload = %#v, want safe unchanged revision", failure)
	}
	before := len(log.events)
	replayed, replayErr := service.Apply(context.Background(), "session-1", testRequest("1"), func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
		return testState(t, factorysessions.LiveChangeLifecycleCompleted), nil
	}, log, app)
	if !errors.Is(replayErr, factorysessions.ErrLiveChangeApplicationFailed) || replayed.Outcome != factorysessions.LiveChangeOutcomeReplayed || len(log.events) != before || app.applyCalls != 1 {
		t.Fatalf("replayed failure=%#v error=%v events=%d applicationCalls=%d", replayed, replayErr, len(log.events), app.applyCalls)
	}
}

func TestRecoverLiveChange_ClosesPendingRequestAfterAppendFailure(t *testing.T) {
	log := &eventLog{failTypeOnce: interfaces.FactoryEventTypeFactoryChange}
	state := testState(t, factorysessions.LiveChangeLifecycleRunning)
	updated, err := interfaces.NewFactorySnapshot(map[string]any{"name": "factory-after-recovery"})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	app := &application{
		preflight:   factorysessions.LiveChangePreflightResult{Admissible: true},
		applyResult: factorysessions.LiveChangeApplicationResult{Factory: updated},
	}
	service := New(nil, nil)
	_, firstErr := service.Apply(context.Background(), "session-1", testRequest("1"), func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
		return state, nil
	}, log, app)
	if !errors.Is(firstErr, factorysessions.ErrLiveChangeEventAppendFailed) || len(log.events) != 1 {
		t.Fatalf("first attempt error=%v events=%d, want pending request after terminal append failure", firstErr, len(log.events))
	}
	result, recoveryErr := service.Recover(context.Background(), "session-1", "request-1", func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
		return state, nil
	}, log, app)
	if recoveryErr != nil || result.Outcome != factorysessions.LiveChangeOutcomeApplied || len(log.events) != 2 {
		t.Fatalf("recovery result=%#v error=%v events=%d", result, recoveryErr, len(log.events))
	}
	if log.events[0].Type != interfaces.FactoryEventTypeFactoryChangeRequest || log.events[1].Type != interfaces.FactoryEventTypeFactoryChange {
		t.Fatalf("recovery events = %#v, want one request and one terminal success", log.events)
	}
}

func TestProjectState_UsesOnlySuccessfulLiveChangeForRevision(t *testing.T) {
	initial, err := interfaces.NewFactorySnapshot(map[string]any{"name": "initial"})
	if err != nil {
		t.Fatalf("create initial snapshot: %v", err)
	}
	updated, err := interfaces.NewFactorySnapshot(map[string]any{"name": "updated"})
	if err != nil {
		t.Fatalf("create updated snapshot: %v", err)
	}
	previous, next, sequence := 0, 1, 4
	requestPayload, _ := json.Marshal(interfaces.FactoryChangeRequestEventPayload{
		ChangeID: "change-1", ExpectedRevision: 0, Operation: "resource.capacity.set", TargetID: "reviewers", RequestedValue: json.RawMessage("8"),
	})
	successPayload, _ := json.Marshal(interfaces.FactoryChangeEventPayload{
		Factory: updated, ChangeID: "change-1", PreviousRevision: &previous, NewRevision: &next, EffectiveSequence: &sequence,
	})
	events := []interfaces.FactoryEvent{
		{Type: interfaces.FactoryEventTypeInitialStructureRequest, Payload: mustJSON(t, interfaces.InitialStructureRequestEventPayload{Factory: initial})},
		{Type: interfaces.FactoryEventTypeFactoryChangeRequest, Context: interfaces.FactoryEventContext{RequestID: stringPtr("request-1")}, Payload: requestPayload},
		{Type: interfaces.FactoryEventTypeFactoryChangeFailed, Payload: mustJSON(t, interfaces.FactoryChangeFailedEventPayload{ChangeID: "failed", PreviousRevision: 0, FailureCode: "APPLICATION_FAILED", FailureMessage: "safe"})},
		{Type: interfaces.FactoryEventTypeFactoryChange, Context: interfaces.FactoryEventContext{Sequence: sequence, SessionID: stringPtr("session-1")}, Payload: successPayload},
	}
	state := ProjectState("session-1", events)
	if state.EffectiveRevision != 1 || state.EffectiveSequence != 4 || state.Factory == nil || !reflect.DeepEqual(*state.Factory, *updated) {
		t.Fatalf("projected state = %#v, want revision 1 and updated snapshot", state)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal test payload: %v", err)
	}
	return encoded
}

func stringPtr(value string) *string { return &value }
