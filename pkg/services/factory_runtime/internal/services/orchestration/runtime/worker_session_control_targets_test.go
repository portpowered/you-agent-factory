package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

type promptProvenanceInvocationInterpolationTestService struct {
	invocationInterpolationTestService
}

func (promptProvenanceInvocationInterpolationTestService) InterpolatePromptWithProvenance(
	authored string,
	invocation *work.InvocationArguments,
	_ interfaces.FileReader,
) (string, []interfaces.InvocationSensitiveTextSpan, error) {
	var spans []interfaces.InvocationSensitiveTextSpan
	if invocation == nil {
		return authored, nil, nil
	}
	var builder strings.Builder
	cursor := 0
	for cursor < len(authored) {
		startOffset := strings.Index(authored[cursor:], "${")
		if startOffset < 0 {
			break
		}
		start := cursor + startOffset
		endOffset := strings.IndexByte(authored[start+2:], '}')
		if endOffset < 0 {
			break
		}
		end := start + 2 + endOffset + 1
		name := authored[start+2 : end-1]
		argument, ok := invocation.Arguments[name]
		if !ok || len(argument.Values) != 1 {
			builder.WriteString(authored[cursor:end])
			cursor = end
			continue
		}
		builder.WriteString(authored[cursor:start])
		replacement := argument.Values[0]
		replacementStart := builder.Len()
		builder.WriteString(replacement)
		if argument.Sensitive && replacement != "" {
			spans = append(spans, interfaces.InvocationSensitiveTextSpan{
				Start: replacementStart,
				End:   builder.Len(),
			})
		}
		cursor = end
	}
	builder.WriteString(authored[cursor:])
	return builder.String(), spans, nil
}

func TestRenderRuntimePromptCarriesDispatchSpecificRedactionProjection(t *testing.T) {
	cfg := &runtimeConfig{
		invocationInterpolation: promptProvenanceInvocationInterpolationTestService{},
		promptRenderer: runtimePromptRendererFunc(func(
			prompt string,
			_ []workers.Token,
			_ *workers.Context,
		) (string, error) {
			return prompt, nil
		}),
	}
	selection := &runtimeExecutionSelection{
		promptTemplate: "prefix=${visible};token=${secret};suffix",
	}
	invocation := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"visible": {Values: []string{"shown"}},
		"secret":  {Values: []string{"hidden"}, Sensitive: true},
	}}

	if err := renderRuntimePrompt(cfg, selection, nil, &workers.Context{}, nil, invocation); err != nil {
		t.Fatalf("renderRuntimePrompt() error = %v", err)
	}
	if selection.userMessage != "prefix=shown;token=hidden;suffix" {
		t.Fatalf("userMessage = %q, want real prompt preserved for execution", selection.userMessage)
	}
	if selection.promptRedaction == nil || selection.promptRedaction.FailClosed {
		t.Fatalf("promptRedaction = %#v, want valid dispatch provenance", selection.promptRedaction)
	}
	if selection.promptRedaction.UserMessage != "prefix=shown;token=<redacted>;suffix" {
		t.Fatalf("recording prompt = %q, want adjacent visible text preserved", selection.promptRedaction.UserMessage)
	}
	if !selection.promptRedaction.RedactUserMessage {
		t.Fatal("RedactUserMessage = false, want explicit sensitive binding provenance")
	}
}

func TestRenderRuntimePromptFailsClosedWithoutPromptProvenance(t *testing.T) {
	cfg := &runtimeConfig{
		invocationInterpolation: invocationInterpolationTestService{},
		promptRenderer: runtimePromptRendererFunc(func(
			prompt string,
			_ []workers.Token,
			_ *workers.Context,
		) (string, error) {
			return prompt, nil
		}),
	}
	selection := &runtimeExecutionSelection{promptTemplate: "visible=${secret}"}
	invocation := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"secret": {Values: []string{"hidden"}, Sensitive: true},
	}}

	if err := renderRuntimePrompt(cfg, selection, nil, &workers.Context{}, nil, invocation); err != nil {
		t.Fatalf("renderRuntimePrompt() error = %v", err)
	}
	if selection.userMessage != "visible=hidden" {
		t.Fatalf("userMessage = %q, want resolved execution prompt", selection.userMessage)
	}
	redaction := selection.promptRedaction
	if redaction == nil || !redaction.FailClosed || !redaction.RedactUserMessage {
		t.Fatalf("promptRedaction = %#v, want complete-field fail-closed projection", redaction)
	}
}

func TestRenderRuntimePromptLeavesUnrelatedDispatchComplete(t *testing.T) {
	cfg := &runtimeConfig{
		invocationInterpolation: promptProvenanceInvocationInterpolationTestService{},
		promptRenderer: runtimePromptRendererFunc(func(
			prompt string,
			_ []workers.Token,
			_ *workers.Context,
		) (string, error) {
			return prompt, nil
		}),
	}
	selection := &runtimeExecutionSelection{promptTemplate: "unrelated dispatch"}
	invocation := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"secret": {Values: []string{"hidden"}, Sensitive: true},
	}}

	if err := renderRuntimePrompt(cfg, selection, nil, &workers.Context{}, nil, invocation); err != nil {
		t.Fatalf("renderRuntimePrompt() error = %v", err)
	}
	if selection.userMessage != "unrelated dispatch" || selection.promptRedaction != nil {
		t.Fatalf("selection = %#v, want complete prompt with no redaction projection", selection)
	}
}

func TestBuildRuntimePromptRedactionFailsClosedOnInvalidSpan(t *testing.T) {
	selection := &runtimeExecutionSelection{
		userMessage:    "visible secret",
		promptTemplate: "visible secret",
		userPromptProvenance: runtimePromptFieldProvenance{
			available: true,
			spans:     []interfaces.InvocationSensitiveTextSpan{{Start: 20, End: 21}},
		},
	}
	redaction := buildRuntimePromptRedaction(nil, selection, nil, nil)
	if redaction == nil || !redaction.FailClosed || !redaction.RedactUserMessage {
		t.Fatalf("redaction = %#v, want fail-closed user prompt projection", redaction)
	}
}

func TestRuntimeWorkstationPromptProvenanceUsesAuthoredBodySource(t *testing.T) {
	lookup := runtimePromptSourceLookupFixture{
		RuntimeDefinitionLookupFixture: runtimefixtures.RuntimeDefinitionLookupFixture{},
		workstation:                    interfaces.PromptSource{Path: "workstation.md"},
	}
	cfg := &runtimeConfig{
		runtimeConfig:           lookup,
		invocationInterpolation: promptProvenanceInvocationInterpolationTestService{},
		promptSourceReader: func(path string) ([]byte, error) {
			if path != "workstation.md" {
				return nil, errors.New("missing source")
			}
			return []byte("---\ntype: MODEL_WORKSTATION\n---\ncontrol=visible secret=${secret}\n"), nil
		},
	}
	invocation := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"secret": {Values: []string{"resolved-secret"}, Sensitive: true},
	}}
	provenance := runtimeWorkstationPromptProvenance(
		cfg,
		&interfaces.FactoryWorkstationConfig{
			Name:           "workstation",
			Body:           "control=visible secret=resolved-secret",
			PromptTemplate: "control=visible secret=resolved-secret",
		},
		invocation,
	)
	if !provenance.body.available || !provenance.body.sensitive() {
		t.Fatalf("body provenance = %#v, want sensitive authored-body spans", provenance.body)
	}
	if provenance.body.resolved != "control=visible secret=resolved-secret" {
		t.Fatalf("body provenance resolved = %q, want interpolated authored body", provenance.body.resolved)
	}

	selection := &runtimeExecutionSelection{systemPrompt: provenance.body.resolved}
	applyRuntimeWorkstationSelection(
		cfg,
		selection,
		invocation,
		&interfaces.FactoryWorkstationConfig{
			Name: "workstation",
			Body: provenance.body.resolved,
		},
		provenance,
	)
	if !selection.systemPromptProvenance.sensitive() {
		t.Fatalf("selection system prompt provenance = %#v, want sensitive spans", selection.systemPromptProvenance)
	}
	safe, ok := redactRuntimePromptText(selection.systemPrompt, selection.systemPromptProvenance.spans)
	if !ok || safe != "control=visible secret=<redacted>" {
		t.Fatalf("safe system prompt = %q, %t, want adjacent control preserved", safe, ok)
	}
}

func TestRuntimeWorkstationPromptProvenanceUsesInlineAuthoredBody(t *testing.T) {
	lookup := runtimePromptSourceLookupFixture{
		RuntimeDefinitionLookupFixture: runtimefixtures.RuntimeDefinitionLookupFixture{},
		workstationProvenance: interfaces.RuntimePromptProvenance{
			Name:           "workstation",
			Body:           "control=visible secret=${secret}",
			PromptTemplate: "control=visible secret=${secret}",
		},
	}
	cfg := &runtimeConfig{
		runtimeConfig:           lookup,
		invocationInterpolation: promptProvenanceInvocationInterpolationTestService{},
	}
	invocation := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"secret": {Values: []string{"resolved-secret"}, Sensitive: true},
	}}
	provenance := runtimeWorkstationPromptProvenance(
		cfg,
		&interfaces.FactoryWorkstationConfig{
			Name:           "workstation",
			Body:           "control=visible secret=resolved-secret",
			PromptTemplate: "control=visible secret=resolved-secret",
		},
		invocation,
	)
	if !provenance.body.available || !provenance.body.sensitive() {
		t.Fatalf("inline body provenance = %#v, want sensitive authored-body spans", provenance.body)
	}
	if !provenance.promptTemplate.available || !provenance.promptTemplate.sensitive() {
		t.Fatalf("inline prompt-template provenance = %#v, want sensitive authored-template spans", provenance.promptTemplate)
	}
	safe, ok := redactRuntimePromptText(
		"control=visible secret=resolved-secret",
		provenance.body.spans,
	)
	if !ok || safe != "control=visible secret=<redacted>" {
		t.Fatalf("safe inline system prompt = %q, %t, want adjacent control preserved", safe, ok)
	}
}

func TestRecordDetachedAgentRunResponseUsesDispatchPromptProjection(t *testing.T) {
	t.Parallel()

	ledger := &agentRunRecordingLedger{
		ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{},
	}
	cfg := &runtimeConfig{
		eventHistory: ledger,
		clock:        testRuntimeClock{},
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"agent": {Name: "agent", Type: interfaces.WorkstationTypeAgent},
			},
		},
	}
	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{DispatchID: "dispatch-secret"},
		Input:       workers.ExecutionInput{Dispatch: work.WorkDispatch{}},
		Target: workers.ExecutionTarget{
			WorkstationName: "agent",
			Prompt: workers.PromptPolicy{
				SystemPrompt: "system token-secret visible",
				UserMessage:  "user token-secret visible",
				Redaction: &workers.PromptRedaction{
					SystemPrompt:       "system <redacted> visible",
					UserMessage:        "user <redacted> visible",
					RedactSystemPrompt: true,
					RedactUserMessage:  true,
				},
			},
		},
	}

	recordDetachedAgentRunResponse(cfg, request, workers.ExecuteResult{}, nil)

	diagnostics, err := workers.SafeWorkDiagnosticsFromEventPayload(ledger.event.Payload.Diagnostics)
	if err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	if diagnostics.AgentRun == nil || len(diagnostics.AgentRun.Transcript) != 2 {
		t.Fatalf("transcript = %#v, want projected system and user entries", diagnostics.AgentRun)
	}
	if diagnostics.AgentRun.Transcript[0].Summary != "system <redacted> visible" ||
		diagnostics.AgentRun.Transcript[1].Summary != "user <redacted> visible" {
		t.Fatalf("transcript = %#v, want adjacent visible text preserved", diagnostics.AgentRun.Transcript)
	}
	if len(ledger.event.DeclaredSecretJSONPointers) != 0 {
		t.Fatalf("transcript provenance = %#v, want no whole-entry fallback", ledger.event.DeclaredSecretJSONPointers)
	}
}

type agentRunRecordingLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
	event workers.AgentRunResponseEvent
}

func (ledger *agentRunRecordingLedger) RecordAgentRunEvent(event workers.AgentRunResponseEvent) {
	ledger.event = event
}

type workerSessionWatermarkedLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
	watermark        recordings.CanonicalEventCursor
	available        bool
	calls            int
	generations      []string
	watermarkForCall func(int, string) (recordings.CanonicalEventCursor, bool)
}

func (ledger *workerSessionWatermarkedLedger) CompletedFlushWatermark(
	streamGenerationID string,
) (recordings.CanonicalEventCursor, bool) {
	ledger.calls++
	ledger.generations = append(ledger.generations, streamGenerationID)
	if ledger.watermarkForCall != nil {
		return ledger.watermarkForCall(ledger.calls, streamGenerationID)
	}
	return ledger.watermark, ledger.available
}

var _ recordings.CompletedFlushWatermarkReader = (*workerSessionWatermarkedLedger)(nil)

func TestRecordedWorkerSessionObservationConfirmationFollowsCompletedFlushWatermark(t *testing.T) {
	base := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	const (
		generationID    = "generation-worker-confirmation"
		workID          = "work-worker-confirmation"
		dispatchID      = "dispatch-worker-confirmation"
		workerSessionID = "worker-worker-confirmation"
	)
	events := []interfaces.FactoryEvent{
		{
			Context: interfaces.FactoryEventContext{
				Tick: 1, Sequence: 1, EventTime: base,
				DispatchID: stringPointerForRecordedTest(dispatchID),
				WorkIDs:    stringSliceForRecordedTest([]string{workID}),
			},
			Id:   "worker-confirmation-request",
			Type: interfaces.FactoryEventTypeDispatchRequest,
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 1, Sequence: 2, EventTime: base.Add(time.Second),
				DispatchID: stringPointerForRecordedTest(dispatchID),
				RequestID:  stringPointerForRecordedTest("turn-worker-confirmation"),
			},
			Id:      "worker-confirmation-association",
			Type:    interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: workerSessionID}),
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 2, Sequence: 3, EventTime: base.Add(2 * time.Second),
				DispatchID: stringPointerForRecordedTest(dispatchID),
			},
			Id:   "worker-confirmation-response",
			Type: interfaces.FactoryEventTypeDispatchResponse,
		},
	}
	ledger := &workerSessionWatermarkedLedger{
		ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{
			Events:       events,
			GenerationID: generationID,
		},
		watermark: recordings.CanonicalEventCursor{StreamGenerationID: generationID, Sequence: 3},
	}
	projector := func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
		return interfaces.FactoryWorldState{
			CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
				DispatchID: dispatchID, StartedAt: base, CompletedAt: base.Add(2 * time.Second),
				WorkItemIDs: []string{workID},
				Result:      interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
			}},
		}, nil
	}
	service := newRecordedWorkerSessionObservation(nil, ledger, factory.WorldStateProjector(projector), platformclock.Real{}, nil)

	before := mustListWorkerSessionObservation(t, service, workID, "before flush")
	if before.ConfirmationState != workersessions.ConfirmationStateUnconfirmed || before.StateSequence != 3 || !before.StateSequenceKnown || before.StreamGenerationID != generationID {
		t.Fatalf("list before flush = %#v, want cursor at sequence 3 and UNCONFIRMED", before)
	}

	showBeforeFlush := mustShowWorkerSessionObservation(t, service, workerSessionID, "before flush")
	if showBeforeFlush.ConfirmationState != workersessions.ConfirmationStateUnconfirmed || showBeforeFlush.State != before.State {
		t.Fatalf("show before flush = %#v, want UNCONFIRMED and same state as list", showBeforeFlush)
	}

	ledger.available = true
	after := mustListWorkerSessionObservation(t, service, workID, "after flush")
	showAfterFlush := mustShowWorkerSessionObservation(t, service, workerSessionID, "after flush")
	if after.ConfirmationState != workersessions.ConfirmationStateConfirmed || showAfterFlush.ConfirmationState != workersessions.ConfirmationStateConfirmed {
		t.Fatalf("after flush confirmation = list:%q show:%q, want CONFIRMED for both surfaces", after.ConfirmationState, showAfterFlush.ConfirmationState)
	}
	if ledger.calls != 4 || len(ledger.generations) != ledger.calls {
		t.Fatalf("watermark calls = %d/%v, want one generation-scoped sample per list/show", ledger.calls, ledger.generations)
	}
	for _, gotGeneration := range ledger.generations {
		if gotGeneration != generationID {
			t.Fatalf("watermark generation = %q, want %q", gotGeneration, generationID)
		}
	}
}

func mustListWorkerSessionObservation(
	t *testing.T,
	service workersessions.Service,
	workID string,
	phase string,
) workersessions.Observation {
	t.Helper()
	result, err := service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil {
		t.Fatalf("ListObservations(%s) error = %v", phase, err)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("ListObservations(%s) count = %d, want 1", phase, len(result.Observations))
	}
	return result.Observations[0]
}

func mustShowWorkerSessionObservation(
	t *testing.T,
	service workersessions.Service,
	workerSessionID string,
	phase string,
) workersessions.Observation {
	t.Helper()
	result, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerSessionID})
	if err != nil {
		t.Fatalf("GetObservationByWorkerSessionID(%s) error = %v", phase, err)
	}
	return result
}

func TestRecordedWorkerSessionObservationListSamplesWatermarkOnceForMergedSources(t *testing.T) {
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	const (
		generationID = "generation-worker-list-sample"
		workID       = "work-worker-list-sample"
	)
	events := recordedObservationTestEvents(t, base, workID)
	ledger := &workerSessionWatermarkedLedger{
		ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{Events: events, GenerationID: generationID},
		watermarkForCall: func(call int, generation string) (recordings.CanonicalEventCursor, bool) {
			if call == 1 {
				return recordings.CanonicalEventCursor{StreamGenerationID: generation, Sequence: 2}, true
			}
			return recordings.CanonicalEventCursor{StreamGenerationID: generation, Sequence: 0}, true
		},
	}
	live := &processLocalWorkerSessionService{
		observationListResult: workersessions.ListObservationsResult{Observations: []workersessions.Observation{{
			WorkerSessionID: "worker-live-only", AttemptID: "dispatch-live-only",
			ConfirmationState:  workersessions.ConfirmationStateConfirmed,
			StreamGenerationID: generationID, StateSequence: 2, StateSequenceKnown: true,
		}}},
	}
	service := newRecordedWorkerSessionObservation(
		live, ledger,
		func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{WorkItemsByID: map[string]work.FactoryWorkItem{workID: {ID: workID}}}, nil
		},
		platformclock.Real{}, nil,
	)

	result, err := service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil {
		t.Fatalf("ListObservations() error = %v", err)
	}
	if ledger.calls != 1 || len(ledger.generations) != 1 || ledger.generations[0] != generationID {
		t.Fatalf("watermark samples = %d/%v, want one sample for %q", ledger.calls, ledger.generations, generationID)
	}
	if len(result.Observations) != 3 {
		t.Fatalf("ListObservations() returned %d observations, want recorded and live observations", len(result.Observations))
	}
	for _, observation := range result.Observations {
		if observation.ConfirmationState != workersessions.ConfirmationStateConfirmed {
			t.Fatalf("observation %q confirmation = %q, want CONFIRMED from the single sample", observation.WorkerSessionID, observation.ConfirmationState)
		}
	}
}

func TestProcessLocalWorkerSessionObservationsRemainUnconfirmed(t *testing.T) {
	live := &processLocalWorkerSessionService{
		topLevelResult: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{{
			WorkerSessionID: "worker-live-running", AttemptID: "attempt-live-running",
			State: workersessions.StateRunning, ConfirmationState: workersessions.ConfirmationStateConfirmed,
		}}},
		getByWorkerResult: workersessions.Observation{
			WorkerSessionID: "worker-live-terminal", AttemptID: "attempt-live-terminal",
			State: workersessions.StateTerminated, ConfirmationState: workersessions.ConfirmationStateConfirmed,
		},
	}
	service := newRecordedWorkerSessionObservation(live, nil, nil, platformclock.Real{}, nil)

	listed, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{})
	if err != nil {
		t.Fatalf("ListWorkerSessionObservations() error = %v", err)
	}
	if len(listed.Observations) != 1 || listed.Observations[0].ConfirmationState != workersessions.ConfirmationStateUnconfirmed {
		t.Fatalf("live list = %#v, want one UNCONFIRMED running observation", listed.Observations)
	}
	shown, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-live-terminal"})
	if err != nil {
		t.Fatalf("GetObservationByWorkerSessionID() error = %v", err)
	}
	if shown.State != workersessions.StateTerminated || shown.ConfirmationState != workersessions.ConfirmationStateUnconfirmed {
		t.Fatalf("live show = %#v, want terminated UNCONFIRMED observation", shown)
	}
}

func TestRecordedWorkerSessionObservationReplayUsesWatermarkForConfirmation(t *testing.T) {
	base := time.Date(2026, 8, 23, 13, 0, 0, 0, time.UTC)
	const (
		generationID    = "generation-worker-replay-confirmation"
		dispatchID      = "dispatch-worker-replay-confirmation"
		workerSessionID = "worker-worker-replay-confirmation"
	)
	replayEvents := []interfaces.FactoryEvent{
		{Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: base, DispatchID: stringPointerForRecordedTest(dispatchID)}, Id: "worker-replay-request", Type: interfaces.FactoryEventTypeDispatchRequest},
		{Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 2, EventTime: base.Add(time.Second), DispatchID: stringPointerForRecordedTest(dispatchID)}, Id: "worker-replay-association", Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: workerSessionID})},
		{Context: interfaces.FactoryEventContext{Tick: 2, Sequence: 3, EventTime: base.Add(2 * time.Second), DispatchID: stringPointerForRecordedTest(dispatchID)}, Id: "worker-replay-response", Type: interfaces.FactoryEventTypeDispatchResponse},
	}
	ledger := &workerSessionWatermarkedLedger{
		ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: generationID},
		watermark:             recordings.CanonicalEventCursor{StreamGenerationID: generationID, Sequence: 3}, available: true,
	}
	service := newRecordedWorkerSessionObservationWithRestoredState(
		nil, ledger,
		func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
				DispatchID: dispatchID, StartedAt: base, CompletedAt: base.Add(2 * time.Second),
				Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
			}}}, nil
		},
		platformclock.Real{}, nil, replayEvents, "", nil, nil, nil,
	)

	show, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerSessionID})
	if err != nil {
		t.Fatalf("GetObservationByWorkerSessionID(replay) error = %v", err)
	}
	if show.ConfirmationState != workersessions.ConfirmationStateConfirmed || show.StateSequence != 3 || show.StreamGenerationID != generationID {
		t.Fatalf("replay observation = %#v, want CONFIRMED at generation sequence 3", show)
	}
}
