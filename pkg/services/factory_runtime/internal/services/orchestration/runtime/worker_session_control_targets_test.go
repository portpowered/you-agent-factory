package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCaptureAssociatedWorkerSessionTargets_SelectsOneDeterministicSnapshot(t *testing.T) {
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: []interfaces.FactoryEvent{
		workerSessionAssociationEvent(t, 30, "association-b", "turn-captured", "worker-b"),
		workerSessionAssociationEvent(t, 10, "association-a", "turn-captured", "worker-a"),
		workerSessionAssociationEvent(t, 20, "association-duplicate", "turn-captured", "worker-b"),
		workerSessionAssociationEvent(t, 40, "association-next-turn", "turn-next", "worker-next"),
		directWorkerEvent(t, 50, "turn-captured", "worker-direct"),
		malformedWorkerSessionAssociationEvent("turn-captured"),
	}}

	captured := captureAssociatedWorkerSessionTargets(ledger, "turn-captured")
	if captured.turnID != "turn-captured" {
		t.Fatalf("captured turn ID = %q, want turn-captured", captured.turnID)
	}
	if got, want := captured.workerSessionIDsSnapshot(), []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("captured Worker Session IDs = %v, want %v", got, want)
	}
	detached := captured.workerSessionIDsSnapshot()
	detached[0] = "mutated"
	if got, want := captured.workerSessionIDsSnapshot(), []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("captured Worker Session IDs after mutating snapshot = %v, want %v", got, want)
	}

	// This deterministic post-capture commit models an association that races
	// after the control's ledger-snapshot linearization point. A retry must
	// retain captured rather than silently reselect this later child.
	ledger.Events = append(ledger.Events, workerSessionAssociationEvent(t, 60, "association-late", "turn-captured", "worker-late"))
	if got, want := captured.workerSessionIDsSnapshot(), []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("captured target set after later association = %v, want immutable %v", got, want)
	}
	if got, want := captureAssociatedWorkerSessionTargets(ledger, "turn-captured").workerSessionIDsSnapshot(), []string{"worker-a", "worker-b", "worker-late"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("new capture after later association = %v, want %v", got, want)
	}
}

func TestSelectAssociatedWorkerSessionTargetsRejectsIncompleteAssociations(t *testing.T) {
	matching := workerSessionAssociationEvent(t, 1, "matching", "turn-1", "worker-1")
	cases := []struct {
		name   string
		events []interfaces.FactoryEvent
		turnID string
		want   []string
	}{
		{name: "matching association", events: []interfaces.FactoryEvent{matching}, turnID: "turn-1", want: []string{"worker-1"}},
		{name: "request ID mismatch", events: []interfaces.FactoryEvent{withRequestID(matching, "other-turn")}, turnID: "turn-1"},
		{name: "nil request ID", events: []interfaces.FactoryEvent{withRequestID(matching, "")}, turnID: "turn-1", want: nil},
		{name: "whitespace request ID", events: []interfaces.FactoryEvent{withRequestID(matching, "   ")}, turnID: "turn-1"},
		{name: "nil dispatch ID", events: []interfaces.FactoryEvent{withDispatchID(matching, "")}, turnID: "turn-1"},
		{name: "whitespace dispatch ID", events: []interfaces.FactoryEvent{withDispatchID(matching, "   ")}, turnID: "turn-1"},
		{name: "wrong event type", events: []interfaces.FactoryEvent{directWorkerEvent(t, 1, "turn-1", "worker-1")}, turnID: "turn-1"},
		{name: "undecodable payload", events: []interfaces.FactoryEvent{malformedWorkerSessionAssociationEvent("turn-1")}, turnID: "turn-1"},
		{name: "empty turn ID", events: []interfaces.FactoryEvent{matching}, turnID: "   "},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := selectAssociatedWorkerSessionTargets(test.events, test.turnID).workerSessionIDsSnapshot(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("selected Worker Session IDs = %v, want %v", got, test.want)
			}
		})
	}
}

func TestFactoryWorkerSessionControlEventsPreferReplayHistory(t *testing.T) {
	live := &recordingfixtures.ScriptedRuntimeLedger{Events: []interfaces.FactoryEvent{
		workerSessionAssociationEvent(t, 2, "live-association", "turn-replay", "worker-live"),
	}}
	f := &factoryImpl{cfg: &runtimeConfig{}, eventHistory: live}
	f.SetReplayEvents([]interfaces.FactoryEvent{
		workerSessionAssociationEvent(t, 4, "replay-association", "turn-replay", "worker-replayed"),
	})

	if got := selectAssociatedWorkerSessionTargets(f.canonicalWorkerSessionControlEvents(), "turn-replay").workerSessionIDsSnapshot(); !reflect.DeepEqual(got, []string{"worker-replayed"}) {
		t.Fatalf("replay Worker Session IDs = %v, want worker-replayed", got)
	}
	f.cfg.replayEvents = nil
	if got := selectAssociatedWorkerSessionTargets(f.canonicalWorkerSessionControlEvents(), "turn-replay").workerSessionIDsSnapshot(); !reflect.DeepEqual(got, []string{"worker-live"}) {
		t.Fatalf("live Worker Session IDs after clearing replay = %v, want worker-live", got)
	}
}

func TestBeginWorkerAttemptRecordsAssociationAndCompletesTerminal(t *testing.T) {
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	sessions := &beginRuntimeAttemptService{Service: &fakeWorkerSessionsService{}}
	f := &factoryImpl{
		cfg:          &runtimeConfig{workerSessions: sessions, clock: platformclock.Real{}},
		eventHistory: ledger,
	}
	request := detachedTargetRequest()

	terminal, err := f.BeginWorkerAttempt(nil, request)
	if err != nil {
		t.Fatalf("BeginWorkerAttempt() error = %v", err)
	}
	if terminal == nil {
		t.Fatal("BeginWorkerAttempt() returned a nil terminal callback")
	}
	associations := ledger.DispatchWorkerSessionAssociationsSnapshot()
	if len(associations) != 1 || associations[0].DispatchID != "dispatch-begin" || associations[0].WorkerSessionID != "dispatch-begin" {
		t.Fatalf("recorded associations = %#v, want dispatch-begin association", associations)
	}
	if got := sessions.request.Execution.Execution.Dispatch.DispatchID; got != "dispatch-begin" {
		t.Fatalf("Worker Session dispatch ID = %q, want dispatch-begin", got)
	}

	result := workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}
	if err := terminal(context.Background(), result, nil); err != nil {
		t.Fatalf("terminal callback error = %v", err)
	}
	if sessions.completed == nil || sessions.completed.DispatchID != "dispatch-begin" || sessions.completeErr != nil {
		t.Fatalf("completed dispatch = %#v, error = %v; want successful dispatch-begin terminal", sessions.completed, sessions.completeErr)
	}
}

func TestBeginWorkerAttemptPreparationFailureDoesNotPublishOrphanAssociation(t *testing.T) {
	beginErr := errors.New("worker attempt preparation failed")
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	sessions := &beginRuntimeAttemptService{
		Service:  &fakeWorkerSessionsService{},
		beginErr: beginErr,
	}
	f := &factoryImpl{
		cfg:          &runtimeConfig{workerSessions: sessions, clock: platformclock.Real{}},
		eventHistory: ledger,
	}
	request := detachedTargetRequest()

	terminal, err := f.BeginWorkerAttempt(nil, request)
	if !errors.Is(err, beginErr) {
		t.Fatalf("BeginWorkerAttempt() error = %v, want %v", err, beginErr)
	}
	if terminal != nil {
		t.Fatal("BeginWorkerAttempt() returned a terminal callback after preparation failed")
	}
	associations := ledger.DispatchWorkerSessionAssociationsSnapshot()
	if len(associations) != 0 {
		t.Fatalf("recorded associations = %#v, want no association before Worker preparation succeeds", associations)
	}
	if sessions.completed != nil {
		t.Fatalf("Worker Session terminal result = %#v, want no callback after BeginRuntimeAttempt failed", sessions.completed)
	}
}

func TestBeginWorkerAttemptCompletesEveryTerminalExitExactlyOnce(t *testing.T) {
	tests := []struct {
		name        string
		result      workers.ExecuteResult
		executeErr  error
		wantOutcome workers.WorkstationDispatchTerminalOutcome
	}{
		{
			name:        "ordinary completion",
			result:      workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted},
			wantOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		},
		{
			name:        "caller cancellation",
			result:      workers.ExecuteResult{},
			executeErr:  context.Canceled,
			wantOutcome: workers.WorkstationDispatchTerminalOutcomeCanceled,
		},
		{
			name:        "provider failure",
			result:      workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted},
			executeErr:  errors.New("provider failed"),
			wantOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		},
		{
			name:        "empty result",
			result:      workers.ExecuteResult{},
			wantOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ledger := &recordingfixtures.ScriptedRuntimeLedger{}
			sessions := &beginRuntimeAttemptService{Service: &fakeWorkerSessionsService{}}
			f := &factoryImpl{
				cfg:          &runtimeConfig{workerSessions: sessions, clock: platformclock.Real{}},
				eventHistory: ledger,
			}

			terminal, err := f.BeginWorkerAttempt(nil, detachedTargetRequest())
			if err != nil {
				t.Fatalf("BeginWorkerAttempt() error = %v", err)
			}
			if err := terminal(context.Background(), test.result, test.executeErr); err != nil {
				t.Fatalf("first terminal callback error = %v", err)
			}
			if err := terminal(context.Background(), workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted}, nil); err != nil {
				t.Fatalf("duplicate terminal callback error = %v", err)
			}

			if sessions.completeCalls != 1 {
				t.Fatalf("terminal callback calls = %d, want exactly one", sessions.completeCalls)
			}
			if sessions.completed == nil || sessions.completed.TerminalOutcome != test.wantOutcome {
				t.Fatalf("completed dispatch = %#v, want terminal outcome %q", sessions.completed, test.wantOutcome)
			}
		})
	}
}

func TestBeginWorkerAttemptReopensTerminalSessionWithPhysicalAttemptIdentity(t *testing.T) {
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	sessions := &beginRuntimeAttemptService{
		Service: &fakeWorkerSessionsService{},
		existing: workersessions.Session{
			ID:    "dispatch-begin",
			State: workersessions.StateCanceled,
		},
	}
	f := &factoryImpl{
		cfg:          &runtimeConfig{workerSessions: sessions, clock: platformclock.Real{}},
		eventHistory: ledger,
	}
	request := detachedTargetRequest()

	if _, err := f.BeginWorkerAttempt(nil, request); err != nil {
		t.Fatalf("BeginWorkerAttempt() error = %v", err)
	}
	associations := ledger.DispatchWorkerSessionAssociationsSnapshot()
	if len(associations) != 1 || associations[0].DispatchID != "dispatch-begin" || associations[0].WorkerSessionID != "attempt-begin" {
		t.Fatalf("recorded associations = %#v, want physical attempt identity", associations)
	}
	if sessions.request.ID != "attempt-begin" {
		t.Fatalf("Worker Session ID = %q, want attempt-begin", sessions.request.ID)
	}
}

func TestWorkstationDispatchRequestFromExecutePreservesDetachedSelection(t *testing.T) {
	request := detachedTargetRequest()
	request.Target.WorkstationName = " "
	request.Input.Dispatch.WorkstationName = "authored-workstation"
	request.Target.Provider.ID = ""
	request.Target.Provider.Alias = "provider-alias"
	request.Target.Workspace.WorkingDirectory = "/workspace"
	request.Input.WorkflowContext = &workers.Context{ProjectID: "project-1"}

	converted := workstationDispatchRequestFromExecute(request)
	if converted.WorkstationName != "authored-workstation" {
		t.Fatalf("workstation name = %q, want authored-workstation", converted.WorkstationName)
	}
	if converted.Execution.Dispatch.DispatchID != "dispatch-begin" || converted.Execution.Dispatch.Execution.RequestID != "request-begin" {
		t.Fatalf("converted dispatch = %#v, want detached correlation", converted.Execution.Dispatch)
	}
	if converted.Execution.ModelProvider != "provider-alias" || converted.Execution.WorkingDirectory != "/workspace" || converted.Execution.ProjectID != "project-1" {
		t.Fatalf("converted selection = %#v, want provider/workspace/project facts", converted.Execution)
	}
}

func detachedTargetRequest() workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-begin",
			RuntimeID:        "runtime-begin",
			GenerationID:     "generation-begin",
			DispatchID:       "dispatch-begin",
			AttemptID:        "attempt-begin",
			RequestID:        "request-begin",
		},
		Target: workers.ExecutionTarget{
			WorkerName:      "worker-begin",
			RunnerID:        "script",
			WorkstationName: "review",
			Command:         "echo done",
			Provider:        workers.ProviderReference{ID: "provider-id"},
			Workspace:       workers.WorkspacePolicy{WorkingDirectory: "/default"},
		},
	}
}

type beginRuntimeAttemptService struct {
	workersessions.Service
	request       workersessions.RuntimeAttemptRequest
	completed     *workers.WorkstationDispatchResult
	completeErr   error
	completeCalls int
	beginErr      error
	existing      workersessions.Session
	getErr        error
}

func (service *beginRuntimeAttemptService) Get(context.Context, workersessions.GetRequest) (workersessions.Session, error) {
	if service.getErr != nil {
		return workersessions.Session{}, service.getErr
	}
	return service.existing, nil
}

func (service *beginRuntimeAttemptService) BeginRuntimeAttempt(
	_ context.Context,
	request workersessions.RuntimeAttemptRequest,
) (workersessions.RuntimeAttempt, error) {
	service.request = request
	if service.beginErr != nil {
		return nil, service.beginErr
	}
	return workersessions.RuntimeAttempt(func(
		_ context.Context,
		result workers.WorkstationDispatchResult,
		err error,
	) error {
		service.completeCalls++
		if service.completed == nil {
			service.completed = &result
			service.completeErr = err
		}
		return nil
	}), nil
}

func TestCaptureAssociatedWorkerSessionTargets_IsolatesFactorySessionLedgersAndEmptyTurns(t *testing.T) {
	currentFactorySession := &recordingfixtures.ScriptedRuntimeLedger{Events: []interfaces.FactoryEvent{
		workerSessionAssociationEvent(t, 1, "current-association", "turn-1", "worker-current"),
	}}
	otherFactorySession := &recordingfixtures.ScriptedRuntimeLedger{Events: []interfaces.FactoryEvent{
		workerSessionAssociationEvent(t, 1, "other-association", "turn-1", "worker-other"),
	}}

	if got, want := captureAssociatedWorkerSessionTargets(currentFactorySession, "turn-1").workerSessionIDsSnapshot(), []string{"worker-current"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("current Factory Session targets = %v, want %v", got, want)
	}
	if got, want := captureAssociatedWorkerSessionTargets(otherFactorySession, "turn-1").workerSessionIDsSnapshot(), []string{"worker-other"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("other Factory Session targets = %v, want %v", got, want)
	}
	if got := captureAssociatedWorkerSessionTargets(currentFactorySession, " ").workerSessionIDsSnapshot(); len(got) != 0 {
		t.Fatalf("blank turn target set = %v, want no-op empty selection", got)
	}
}

func workerSessionAssociationEvent(t *testing.T, sequence int, id, turnID, workerSessionID string) interfaces.FactoryEvent {
	t.Helper()
	payload, err := json.Marshal(interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: workerSessionID})
	if err != nil {
		t.Fatalf("marshal association payload: %v", err)
	}
	dispatchID := "dispatch-" + workerSessionID
	return interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC),
			RequestID:  &turnID,
			Sequence:   sequence,
		},
		Id:            id,
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	}
}

func directWorkerEvent(t *testing.T, sequence int, turnID, workerSessionID string) interfaces.FactoryEvent {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"workerSessionId": workerSessionID})
	if err != nil {
		t.Fatalf("marshal direct Worker event payload: %v", err)
	}
	return interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			EventTime: time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC),
			RequestID: &turnID,
			Sequence:  sequence,
		},
		Id:            "direct-worker-event",
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchResponse,
	}
}

func malformedWorkerSessionAssociationEvent(turnID string) interfaces.FactoryEvent {
	dispatchID := "dispatch-malformed"
	return interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC),
			RequestID:  &turnID,
			Sequence:   55,
		},
		Id:            "association-malformed",
		Payload:       []byte(`not-json`),
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	}
}

func withRequestID(event interfaces.FactoryEvent, requestID string) interfaces.FactoryEvent {
	event.Context.RequestID = nil
	if requestID != "" {
		event.Context.RequestID = &requestID
	}
	return event
}

func withDispatchID(event interfaces.FactoryEvent, dispatchID string) interfaces.FactoryEvent {
	event.Context.DispatchID = nil
	if dispatchID != "" {
		event.Context.DispatchID = &dispatchID
	}
	return event
}
