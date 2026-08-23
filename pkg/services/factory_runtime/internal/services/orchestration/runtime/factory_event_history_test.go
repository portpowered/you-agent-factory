package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNew_ProviderBoundPetriDispatchPreservesPublicContractOnReplay(t *testing.T) {
	net := buildSimpleNet()
	history := &recordingfixtures.ScriptedRuntimeLedger{}
	f, err := newTestFactory(
		withNet(net),
		withInlineDispatch(),
		withFactoryEventHistory(history),
		withWorkerExecutor("mock", providerBoundaryExecutor{record: history.RecordInferenceEvent}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID: "work-provider-contract", WorkTypeID: "task", TraceID: "trace-provider-contract",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if history.CallCount("RecordInferenceEvent") != 1 {
		t.Fatalf("RecordInferenceEvent calls = %d, want 1", history.CallCount("RecordInferenceEvent"))
	}
	if history.CallCount("RecordWorkstationResponse") != 1 {
		t.Fatalf("RecordWorkstationResponse calls = %d, want 1", history.CallCount("RecordWorkstationResponse"))
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snapshot.DispatchHistory) != 1 {
		t.Fatalf("dispatch history = %#v, want one completed dispatch", snapshot.DispatchHistory)
	}
	impl := f.(*factoryImpl)
	intent, ok := impl.dispatchPlan.Intent(snapshot.DispatchHistory[0].DispatchID)
	if !ok || intent.Status != dispatchplanning.OutboxIntentStatusRetired || intent.Attempts != 1 {
		t.Fatalf("dispatch outbox intent = %#v, %t; want one published and retired intent", intent, ok)
	}
	// Recordings owns canonical inference-event construction and replay shape;
	// Workers owns provider emission ordering.
}

type providerBoundaryExecutor struct {
	record workerexecution.InferenceEventRecorder
}

func (e providerBoundaryExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	response := "provider contract output"
	session := &providers.SessionMetadata{
		Provider: "mock", Kind: "session_id", ID: "petri-provider-session-1",
	}
	diagnostics := json.RawMessage(`{"provider":{"provider":"mock","model":"fixture-model","responseMetadata":{"provider_session_id":"petri-provider-session-1"}}}`)
	if e.record != nil {
		// RecordingProvider's request/response emission is covered by the
		// Workers-owned TestRecordingProvider_Infer_SuccessEmitsRequestAndResponseEventsInOrder.
		// Factory Runtime starts at the injected Worker inference-fact edge.
		e.record(workerexecution.InferenceEvent{
			ID:         "factory-event/inference-response/" + dispatch.DispatchID,
			Kind:       workerexecution.InferenceEventKindResponse,
			EventTime:  time.Now().UTC(),
			Tick:       dispatch.Execution.CurrentTick,
			DispatchID: dispatch.DispatchID,
			RequestID:  dispatch.Execution.RequestID,
			TraceIDs:   []string{dispatch.Execution.TraceID},
			WorkIDs:    append([]string(nil), dispatch.Execution.WorkIDs...),
			Response: &workerexecution.InferenceResponseEventPayload{
				Attempt: 1, Diagnostics: diagnostics, InferenceRequestID: dispatch.DispatchID + "/inference-request/1",
				Outcome: workerexecution.InferenceOutcomeSucceeded, Continuation: (session).ContinuationRef(), Response: &response,
			},
		})
	}
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       response,
		Continuation: (session).ContinuationRef(),
		Diagnostics: &workerexecution.WorkDiagnostics{Provider: &workerexecution.ProviderDiagnostic{
			Provider: "mock", Model: "fixture-model",
		}},
	}, nil
}

func TestNew_SafeDiagnosticsBoundarySurvivesReplayAndSelectedTickProjection(t *testing.T) {
	f, history := newSafeBoundaryRuntime(t)
	submitSafeBoundaryRequests(t, f)
	tickUntilDispatchResponses(t, tickableFactory(t, f), history, 3)
	liveSnapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	if len(liveSnapshot.DispatchHistory) != 3 {
		t.Fatalf("live dispatch history = %#v, want three completed dispatches", liveSnapshot.DispatchHistory)
	}
	if history.CallCount("RecordWorkstationResponse") != 3 {
		t.Fatalf("RecordWorkstationResponse calls = %d, want 3", history.CallCount("RecordWorkstationResponse"))
	}
	// Canonical selected-tick reconstruction and presentation are Recordings
	// invariants covered by TestReconstructFactoryWorldState_PreservesSafeResponseDiagnostics,
	// TestReconstructFactoryWorldState_PreservesSafeInferenceAttemptDiagnostics, and
	// TestBuildFactoryWorldView_ProjectsCanonicalDispatchAndProviderSessionInputsFromEvents.
}

func TestFactoryEventHistory_RecordsOrderedEventsWithStableIDs(t *testing.T) {
	f, history := newPassingInlineRuntimeWithLedger(t)
	submitOrderedEventHistoryRequest(t, f)
	tickAndPauseRuntime(t, f)

	calls := history.CallsSnapshot()
	assertRuntimeLedgerCallsInOrder(t, calls,
		"RecordRunRequest",
		"RecordInitialStructure",
		"RecordWorkRequest",
		"RecordWorkstationRequest",
		"RecordWorkstationResponse",
		"RecordSessionPaused",
	)
	// Recordings reduction of the ordered stream is covered owner-locally by
	// TestBuildFactoryWorldView_SelectedTickProjectionComesFromEventHistory.
	// Stable canonical IDs are Recordings-owned and covered by its event-history
	// constructor tests; Runtime owns only the order in which it reports facts.
}

func TestNew_SubmitWorkRequestRecordsCanonicalWorkRequestEvent(t *testing.T) {
	f, history := newPassingInlineRuntimeWithLedger(t)
	tickable := tickableFactory(t, f)

	request := work.WorkRequest{
		RequestID: "request-canonical-work-event",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "canonical",
			WorkID:     "work-canonical",
			WorkTypeID: "task",
			TraceID:    "trace-canonical",
		}},
	}
	if _, err := f.SubmitWorkRequest(context.Background(), request); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(history.WorkRequests) != 1 {
		t.Fatalf("work request records = %#v, want one", history.WorkRequests)
	}
	record := history.WorkRequests[0]
	if record.RequestID != "request-canonical-work-event" ||
		record.Type != work.WorkRequestTypeFactoryRequestBatch ||
		record.TraceID != "trace-canonical" ||
		len(record.WorkItems) != 1 ||
		record.WorkItems[0].ID != "work-canonical" {
		t.Fatalf("work request record = %#v, want canonical batch facts", record)
	}
	// Recordings owns conversion of this root record into the public event.
}

func TestFactoryEventHistory_BatchRequestAndRelationshipReplay(t *testing.T) {
	f, history := newPassingInlineRuntimeWithLedger(t)
	request := mustUnmarshalRuntimeWorkRequest(t, `{
		"requestId": "request-batch-events",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "first", "workId": "work-first", "workTypeName": "task", "traceId": "trace-batch"},
			{"name": "second", "workId": "work-second", "workTypeName": "task"}
		],
		"relations": [
			{"type": "DEPENDS_ON", "sourceWorkName": "second", "targetWorkName": "first", "requiredState": "done"}
		]
	}`)

	assertIdempotentBatchSubmit(t, f, request)
	if err := tickableFactory(t, f).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(history.WorkRequests) != 1 {
		t.Fatalf("work request records = %#v, want one after idempotent retry", history.WorkRequests)
	}
	record := history.WorkRequests[0]
	if record.RequestID != "request-batch-events" || record.TraceID != "trace-batch" ||
		len(record.WorkItems) != 2 || len(record.Relations) != 1 {
		t.Fatalf("batch record = %#v, want two works and one relationship", record)
	}
	relation := record.Relations[0]
	if relation.SourceWorkName != "second" || relation.TargetWorkID != "work-first" ||
		relation.TargetWorkName != "first" || relation.RequiredState != "done" {
		t.Fatalf("relationship record = %#v, want named batch dependency", relation)
	}
	// Recordings owns relationship replay reduction; its named owner invariant
	// is TestReconstructFactoryWorldState_ResolvesBatchRelationSourcesByWorkName.
}

func TestFactoryEventHistory_GeneratedBatchPreservesMetadataAndOrdering(t *testing.T) {
	history := &recordingfixtures.ScriptedRuntimeLedger{}
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withSubmissionHook(&generatedBatchHook{batch: generatedRuntimeBatchFixture()}),
		withFactoryEventHistory(history),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := tickableFactory(t, f).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(history.WorkRequests) != 1 {
		t.Fatalf("generated work request records = %#v, want one", history.WorkRequests)
	}
	record := history.WorkRequests[0]
	if record.RequestID != "generated-request-events" ||
		record.Source != "worker-output:dispatch-parent" ||
		len(record.ParentLineage) != 2 ||
		len(record.WorkItems) != 2 ||
		len(record.Relations) != 1 {
		t.Fatalf("generated batch record = %#v, want metadata and declaration order", record)
	}
	if record.WorkItems[0].ID != "work-draft" || record.WorkItems[1].ID != "work-review" {
		t.Fatalf("generated work item order = %#v, want draft then review", record.WorkItems)
	}
	// Generated request and relationship payloads are asserted above. Their
	// replay reduction is the Recordings-owned
	// TestReconstructFactoryWorldState_ResolvesBatchRelationSourcesByWorkName.
}

func newSafeBoundaryRuntime(t *testing.T) (factoryhost.Engine, *recordingfixtures.ScriptedRuntimeLedger) {
	t.Helper()
	f, history, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNetWithFailureArc()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &safeDiagnosticsBoundaryExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f, history
}

func submitSafeBoundaryRequests(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	_, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{
		{WorkID: "work-safe-success", WorkTypeID: "task", TraceID: "trace-safe-success", Payload: json.RawMessage(`{"story":"safe success"}`)},
		{WorkID: "work-safe-failure", WorkTypeID: "task", TraceID: "trace-safe-failure", Payload: json.RawMessage(`{"story":"safe failure"}`)},
		{WorkID: "work-safe-windows-process-failure", WorkTypeID: "task", TraceID: "trace-safe-windows-process-failure", Payload: json.RawMessage(`{"story":"safe windows process failure"}`)},
	})
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
}

func tickUntilDispatchResponses(
	t *testing.T,
	tickable TickableFactory,
	history *recordingfixtures.ScriptedRuntimeLedger,
	want int,
) {
	t.Helper()
	for attempt := 0; attempt < want; attempt++ {
		if err := tickable.Tick(context.Background()); err != nil {
			t.Fatalf("Tick attempt %d: %v", attempt+1, err)
		}
		if history.CallCount("RecordWorkstationResponse") == want {
			return
		}
	}
}

func assertRuntimeLedgerCallsInOrder(t *testing.T, calls []string, want ...string) {
	t.Helper()
	next := 0
	for _, call := range calls {
		if next < len(want) && call == want[next] {
			next++
		}
	}
	if next != len(want) {
		t.Fatalf("runtime ledger calls = %v, want ordered subsequence %v", calls, want)
	}
}

func submitOrderedEventHistoryRequest(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	_, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     "work-1",
		Name:       "Write PRD",
		WorkTypeID: "task",
		TraceID:    "trace-1",
		Relations: []work.Relation{{
			Type:          work.RelationDependsOn,
			TargetWorkID:  "upstream-1",
			RequiredState: "done",
		}},
	}})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
}

func tickAndPauseRuntime(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	tickable := tickableFactory(t, f)
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
}

func mustUnmarshalRuntimeWorkRequest(t *testing.T, body string) work.WorkRequest {
	t.Helper()
	var request work.WorkRequest
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("Unmarshal WorkRequest: %v", err)
	}
	return request
}

func assertIdempotentBatchSubmit(t *testing.T, f factoryhost.Engine, request work.WorkRequest) {
	t.Helper()
	result, err := f.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if result.RequestID != "request-batch-events" || result.TraceID != "trace-batch" || !result.Accepted {
		t.Fatalf("submit result = %#v, want accepted stable request metadata", result)
	}
	repeated, err := f.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate SubmitWorkRequest: %v", err)
	}
	if repeated.RequestID != result.RequestID || repeated.TraceID != result.TraceID || repeated.Accepted {
		t.Fatalf("duplicate submit result = %#v, want original metadata with Accepted=false", repeated)
	}
}

func generatedRuntimeBatchFixture() work.GeneratedSubmissionBatch {
	return work.GeneratedSubmissionBatch{
		Request: work.WorkRequest{
			RequestID: "generated-request-events",
			Type:      work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.Work{
				{Name: "draft", WorkID: "work-draft", WorkTypeID: "task", TraceID: "trace-generated"},
				{Name: "review", WorkID: "work-review", WorkTypeID: "task"},
			},
			Relations: []work.WorkRelation{{
				Type:           work.WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "draft",
				RequiredState:  "done",
			}},
		},
		Metadata: work.GeneratedSubmissionBatchMetadata{
			Source:        "worker-output:dispatch-parent",
			ParentLineage: []string{"request-parent", "work-parent"},
		},
		Submissions: []work.SubmitRequest{{
			Name:        "review",
			WorkID:      "work-review",
			TargetState: "done",
			Tags:        map[string]string{"runtime": "true"},
		}},
	}
}

func TestRecordedWorkerSessionObservationHistoricalProjectionOutcomes(t *testing.T) {
	fixture := newRecordedExactObservationFixture(t)
	adapter := fixture.service.(*recordedWorkerSessionObservation)

	adapter.providerSessions = nil
	if _, err := adapter.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: fixture.ref}); err != nil {
		t.Fatalf("GetObservation(without optional Provider Sessions detail) error = %v", err)
	}
	if _, err := adapter.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: fixture.ref}); !errors.Is(err, workersessions.ErrObservationTranscriptProjectionUnavailable) {
		t.Fatalf("ReadTranscript(without Provider Sessions detail) error = %v", err)
	}

	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "canceled", err: context.Canceled, want: workersessions.ErrObservationCanceled},
		{name: "provider canceled", err: providersessions.ErrOperationCanceled, want: workersessions.ErrObservationCanceled},
		{name: "source unavailable", err: providersessions.ErrSessionNotFound, want: workersessions.ErrObservationTranscriptUnavailable},
		{name: "projection failure", err: errors.New("projection failed"), want: workersessions.ErrObservationTranscriptProjectionUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := newRecordedExactObservationFixture(t)
			currentAdapter := current.service.(*recordedWorkerSessionObservation)
			currentAdapter.providerSessions = &historicalProviderSessions{err: test.err}
			_, err := currentAdapter.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: current.ref})
			if !errors.Is(err, test.want) {
				t.Fatalf("ReadTranscript() error = %v, want %v", err, test.want)
			}
		})
	}

	current := newRecordedExactObservationFixture(t)
	currentAdapter := current.service.(*recordedWorkerSessionObservation)
	currentAdapter.providerSessions = &historicalProviderSessions{err: context.Canceled}
	if _, err := currentAdapter.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: current.ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("GetObservation(provider canceled) error = %v", err)
	}
	currentAdapter.providerSessions = &historicalProviderSessions{err: errors.New("optional detail unavailable")}
	if observation, err := currentAdapter.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: current.ref}); err != nil || observation.Transcript != workersessions.TranscriptAvailabilityUnavailable {
		t.Fatalf("GetObservation(optional detail failure) = %#v, %v", observation, err)
	}

	if _, err := currentAdapter.readRecordedTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: current.ref}, recordedDispatchObservation{}); !errors.Is(err, workersessions.ErrObservationTranscriptUnavailable) {
		t.Fatalf("readRecordedTranscript(without provider metadata) error = %v", err)
	}
	if _, err := historicalTranscriptResult(recordedDispatchObservation{}, nil, current.ref); err == nil {
		t.Fatal("historicalTranscriptResult(invalid identity) error = nil")
	}
}

func TestRecordedWorkerSessionObservationStreamOutcomes(t *testing.T) {
	fixture := newRecordedExactObservationFixture(t)
	adapter := fixture.service.(*recordedWorkerSessionObservation)
	ledger := adapter.ledger.(*recordingfixtures.ScriptedRuntimeLedger)

	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "canceled subscribe", err: context.Canceled, want: workersessions.ErrObservationCanceled},
		{name: "source subscribe failure", err: errors.New("subscribe failed"), want: workersessions.ErrObservationSourceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			ledger.SubscribeError = test.err
			_, err := adapter.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: fixture.ref})
			if !errors.Is(err, test.want) {
				t.Fatalf("StreamObservations() error = %v, want %v", err, test.want)
			}
			ledger.SubscribeError = nil
		})
	}

	missing := fixture.ref.Clone()
	missing.ID = "missing-provider-session"
	if _, err := adapter.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: missing}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("StreamObservations(missing) error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.StreamObservations(canceled, workersessions.StreamObservationsRequest{ProviderSession: fixture.ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("StreamObservations(canceled) error = %v", err)
	}
	if _, handled, err := (&recordedWorkerSessionObservation{}).streamRecorded(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: fixture.ref}); handled || err != nil {
		t.Fatalf("streamRecorded(without recording projection) = handled=%v err=%v", handled, err)
	}

	liveEvents := make(chan interfaces.FactoryEvent, 1)
	terminalEvent := interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{Sequence: 4, DispatchID: stringPointerForRecordedTest("dispatch-recorded-exact")},
		Id:      "live-terminal",
		Type:    interfaces.FactoryEventTypeDispatchInterrupted,
	}
	liveEvents <- terminalEvent
	ledger.SubscribeResult = interfaces.FactoryEventStream{History: []interfaces.FactoryEvent{terminalEvent}, Events: liveEvents}
	subscription, err := adapter.StreamObservations(nil, workersessions.StreamObservationsRequest{ProviderSession: fixture.ref})
	if err != nil {
		t.Fatalf("StreamObservations(nil context) error = %v", err)
	}
	if delivery := subscription.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryTerminalReplay {
		t.Fatalf("live terminal replay delivery = %#v", delivery)
	}
	subscription.Close()
}

func TestRecordedObservationSubscriptionMapsLiveAndClosedOutcomes(t *testing.T) {
	dispatchID := "dispatch-subscription"
	event := func(eventType interfaces.FactoryEventType, sequence int, id string) interfaces.FactoryEvent {
		return interfaces.FactoryEvent{
			Context: interfaces.FactoryEventContext{DispatchID: stringPointerForRecordedTest(dispatchID), Sequence: sequence},
			Id:      id,
			Type:    eventType,
		}
	}

	closedEvents := make(chan interfaces.FactoryEvent)
	close(closedEvents)
	closed := newRecordedObservationSubscription(interfaces.FactoryEventStream{Events: closedEvents}, dispatchID, false, nil, nil)
	if delivery := closed.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliverySourceFailure || !errors.Is(delivery.Err, workersessions.ErrObservationSourceClosed) {
		t.Fatalf("closed source delivery = %#v", delivery)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	canceledSubscription := newRecordedObservationSubscription(interfaces.FactoryEventStream{Events: make(chan interfaces.FactoryEvent)}, dispatchID, false, nil, nil)
	if delivery := canceledSubscription.Next(canceled); delivery.Kind != workersessions.ObservationDeliveryCanceled || !errors.Is(delivery.Err, workersessions.ErrObservationCanceled) {
		t.Fatalf("canceled subscription delivery = %#v", delivery)
	}

	sourceContext, sourceCancel := context.WithCancel(context.Background())
	sourceCancel()
	sourceClosed := newRecordedObservationSubscription(interfaces.FactoryEventStream{Events: make(chan interfaces.FactoryEvent)}, dispatchID, false, nil, sourceContext)
	if delivery := sourceClosed.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("source-context-closed delivery = %#v", delivery)
	}

	liveEvents := make(chan interfaces.FactoryEvent, 1)
	liveEvents <- event(interfaces.FactoryEventTypeDispatchReconciled, 2, "live-reconciled")
	live := newRecordedObservationSubscription(interfaces.FactoryEventStream{Events: liveEvents}, dispatchID, false, nil, nil)
	if delivery := live.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryTerminal || delivery.Event.SourceSequence != 2 {
		t.Fatalf("live terminal delivery = %#v", delivery)
	}

	if got := (&recordedObservationSubscription{}).streamContext(); got == nil {
		t.Fatal("streamContext(nil source context) returned nil")
	}
	nilEvents := newRecordedObservationSubscription(interfaces.FactoryEventStream{}, dispatchID, false, nil, nil)
	if delivery := nilEvents.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliverySourceFailure || !errors.Is(delivery.Err, workersessions.ErrObservationSourceClosed) {
		t.Fatalf("nil source delivery = %#v", delivery)
	}
}

func TestRecordedTranscriptDiagnostics(t *testing.T) {
	if got := recordedDiagnosticMessage(""); got == "" {
		t.Fatal("empty diagnostic message returned empty fallback")
	}
	for _, message := range []string{"path C:\\secret", "authorization bearer token", "secret prompt", strings.Repeat("x", 300)} {
		if got := recordedDiagnosticMessage(message); got == "" || len(got) > 256 {
			t.Fatalf("recordedDiagnosticMessage(%q) = %q", message, got)
		}
	}
}

func TestRecordedTranscriptTokenAndParseValues(t *testing.T) {
	values := []int{1, 2, 3, 4, 5, 6}
	usage := recordedTokenUsage(&providersessions.TokenUsage{
		CacheWriteTokens: &values[0], CachedInputTokens: &values[1], InputTokens: &values[2],
		OutputTokens: &values[3], ReasoningOutputTokens: &values[4], TotalTokens: &values[5],
	})
	if usage == nil || usage.TotalTokens == nil || *usage.TotalTokens != 6 || recordedTokenUsage(nil) != nil {
		t.Fatalf("recordedTokenUsage() = %#v", usage)
	}
	parse := recordedParseDiagnostics(providersessions.ParseSummary{ParseErrors: []providersessions.LineError{{LineNumber: 7, Message: "parse failed"}}})
	if len(parse.Errors) != 1 || parse.Errors[0].LineNumber != 7 {
		t.Fatalf("recordedParseDiagnostics() = %#v", parse)
	}
	negative := recordedObservationEvent(interfaces.FactoryEvent{Context: interfaces.FactoryEventContext{Sequence: -1}, Id: "negative"})
	if negative.Position != 0 || negative.SourceSequence != 0 {
		t.Fatalf("recordedObservationEvent(negative sequence) = %#v", negative)
	}
}

func TestRecordedTranscriptOptionalValuesAndSourceClassification(t *testing.T) {
	applyRecordedProviderDetail(nil, providersessions.Detail{})
	if cloneRecordedBool(nil) != nil || cloneRecordedInt(nil) != nil || cloneRecordedString(nil) != nil || cloneRecordedTime(nil) != nil {
		t.Fatal("nil recorded pointer clones returned values")
	}
	for _, sourceErr := range []error{
		providersessions.ErrSessionNotFound,
		providersessions.ErrAmbiguousSessionFile,
		providersessions.ErrSessionSourceNotRegularFile,
		providersessions.ErrSessionStorageUnavailable,
		providersessions.ErrSessionOutsideRoot,
	} {
		if !recordedTranscriptSourceUnavailable(sourceErr) {
			t.Fatalf("recordedTranscriptSourceUnavailable(%v) = false", sourceErr)
		}
	}
	if recordedTranscriptSourceUnavailable(errors.New("other")) {
		t.Fatal("recordedTranscriptSourceUnavailable(other) = true")
	}
}

// TestRuntimeSupersededCommandHelperProcess is the child process used by the
// Runtime/Workers/process integration test below. The winner exits normally;
// the loser stays alive until Runtime propagates SUPERSEDED to the command
// context and the process cleanup boundary force-kills it.
func TestRuntimeSupersededCommandHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_RUNTIME_COMMAND_HELPER") != "1" {
		return
	}
	if len(os.Args) == 0 {
		os.Exit(2)
	}
	switch os.Args[len(os.Args)-1] {
	case "winner":
		os.Exit(0)
	case "loser":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func TestEngine_SameTickSupersededLoserRestoresResourcesWhileWinnerCompletes(t *testing.T) {
	if testing.Short() {
		t.Skip("real child-process integration")
	}
	workersService, workerSessions := newRuntimeSupersededServices(t)
	logger := &recordingLogger{}
	harness := startServiceModeRunHarness(t,
		withNet(runtimeSameWorkObserveConsumeNet()),
		withServiceMode(),
		withScheduler(scheduler.NewWorkInQueueScheduler(2, nil)),
		withWorkerService(workersService),
		withWorkerSessions(workerSessions),
		withLogger(logger),
	)
	t.Cleanup(harness.stop)

	workID := "runtime-superseded-work"
	if _, err := submitWorkRequests(t.Context(), harness.Factory, []work.SubmitRequest{{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    "trace-" + workID,
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	started := waitForRuntimeSupersededProcesses(t, workersService)
	winner, winnerOK := started["consume-work"]
	loser, loserOK := started["observe-work"]
	if !winnerOK || !loserOK {
		t.Fatalf("started transitions = %#v, want consume-work and observe-work", started)
	}
	cancelOutcome, err := harness.Factory.(*factoryImpl).cfg.attempts.cancel(
		context.Background(), loser.DispatchID, workerexecution.WorkstationDispatchCancelReasonSuperseded,
	)
	if err != nil {
		t.Fatalf("supersede losing attempt: %v", err)
	}
	if cancelOutcome != workerexecution.WorkstationDispatchCancelOutcomeCanceled {
		t.Fatalf("supersede outcome = %q, want CANCELED", cancelOutcome)
	}
	results := waitForRuntimeSupersededResults(t, workersService)
	assertRuntimeSupersededCommandResults(t, results)
	snapshot := waitForAggregateSnapshotWithTimeout(t, harness.Factory, 5*time.Second, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount == 0 && markingContainsWorkAtPlace(&snapshot.Marking, workID, "task:done")
	})
	assertRuntimeSupersededOutcome(t, snapshot, workID, winner, loser, workerSessions, logger)
}

func newRuntimeSupersededServices(t *testing.T) (*runtimeSupersededProcessWorkers, *runtimeWorkerSessionsService) {
	t.Helper()
	runner, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, logging.NoopLogger{}, nil)
	if err != nil {
		t.Fatalf("NewExecCommandRunner: %v", err)
	}
	workersService := &runtimeSupersededProcessWorkers{
		runner:  runner,
		started: make(chan runtimeProcessStarted, 2),
		results: make(chan runtimeProcessResult, 2),
	}
	workerSessions := newRuntimeWorkerSessionsService(workersService)
	return workersService, workerSessions
}

func waitForRuntimeSupersededProcesses(t *testing.T, service *runtimeSupersededProcessWorkers) map[string]runtimeProcessStarted {
	t.Helper()
	started := make(map[string]runtimeProcessStarted, 2)
	for range 2 {
		select {
		case process := <-service.started:
			started[process.TransitionID] = process
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for same-tick processes; started=%#v", started)
		}
	}
	return started
}

func waitForRuntimeSupersededResults(t *testing.T, service *runtimeSupersededProcessWorkers) map[string]runtimeProcessResult {
	t.Helper()
	results := make(map[string]runtimeProcessResult, 2)
	for range 2 {
		select {
		case result := <-service.results:
			results[result.TransitionID] = result
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for command results; results=%#v", results)
		}
	}
	return results
}

func assertRuntimeSupersededCommandResults(t *testing.T, results map[string]runtimeProcessResult) {
	t.Helper()
	winner, ok := results["consume-work"]
	if !ok || winner.err != nil || winner.command.ExitCode != 0 || winner.command.CancellationReason != "" {
		t.Fatalf("winner command result = %#v, want normal zero-exit result", winner)
	}
	loser, ok := results["observe-work"]
	if !ok || !errors.Is(loser.err, context.Canceled) || loser.command.ExitCode != 0 ||
		loser.command.CancellationReason != platformprocess.CancellationReasonSuperseded {
		t.Fatalf("loser command result = %#v, want zero-exit SUPERSEDED cancellation", loser)
	}
}

func assertRuntimeSupersededOutcome(
	t *testing.T,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	workID string,
	winner runtimeProcessStarted,
	loser runtimeProcessStarted,
	workerSessions workersessions.Service,
	logger *recordingLogger,
) {
	t.Helper()
	if len(snapshot.Marking.PlaceTokens["task:failed"]) != 0 ||
		len(snapshot.Marking.PlaceTokens["slot:available"]) != 1 {
		t.Fatalf("final marking = %#v, want no failed Work and one restored slot", snapshot.Marking.PlaceTokens)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, workID, "task:done") {
		t.Fatalf("final marking = %#v, want winning Work at task:done", snapshot.Marking.PlaceTokens)
	}
	history := make(map[string]interfaces.CompletedDispatch, len(snapshot.DispatchHistory))
	for _, completed := range snapshot.DispatchHistory {
		history[completed.TransitionID] = completed
	}
	if completed := history["consume-work"]; completed.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("winner dispatch history = %#v, want accepted completion", history)
	}
	completed, ok := history["observe-work"]
	if !ok || completed.Outcome != workerexecution.OutcomeCanceled || completed.Cancellation == nil ||
		completed.Cancellation.Reason != workerexecution.DispatchCancellationReasonSuperseded {
		t.Fatalf("loser dispatch history = %#v, want SUPERSEDED cancellation", history)
	}
	assertRuntimeSupersededSessions(t, workerSessions, winner.DispatchID, loser.DispatchID)
	for _, entry := range logger.entries {
		if entry.message == "transitioner: result failed" {
			t.Fatalf("superseded cancellation logged as transition failure: %#v", entry)
		}
	}
}

func assertRuntimeSupersededSessions(t *testing.T, service workersessions.Service, winnerID, loserID string) {
	t.Helper()
	loser, err := service.Get(context.Background(), workersessions.GetRequest{ID: loserID})
	if err != nil {
		t.Fatalf("Get losing Worker Session: %v", err)
	}
	if loser.State != workersessions.StateCanceled {
		t.Fatalf("losing Worker Session = %#v, want CANCELED", loser)
	}
	if loser.Result != nil && loser.Result.Cause != nil && loser.Result.Cause.Kind == workersessions.FailureCauseWorkersExecutionFailure {
		t.Fatalf("losing Worker Session classified SUPERSEDED as execution failure: %#v", loser)
	}
	winner, err := service.Get(context.Background(), workersessions.GetRequest{ID: winnerID})
	if err != nil {
		t.Fatalf("Get winning Worker Session: %v", err)
	}
	if winner.State != workersessions.StateCompleted || winner.Result == nil || winner.Result.Outcome != workersessions.TerminalOutcomeCompleted {
		t.Fatalf("winning Worker Session = %#v, want COMPLETED", winner)
	}
}

type runtimeProcessStarted struct {
	DispatchID   string
	TransitionID string
}

type runtimeProcessResult struct {
	DispatchID   string
	TransitionID string
	command      platformprocess.CommandResult
	err          error
}

type runtimeSupersededProcessWorkers struct {
	runner  platformprocess.CommandRunner
	started chan runtimeProcessStarted
	results chan runtimeProcessResult
}

func (service *runtimeSupersededProcessWorkers) Execute(ctx context.Context, request workerexecution.ExecuteRequest) (workerexecution.ExecuteResult, error) {
	mode := "winner"
	if request.Input.Dispatch.TransitionID == "observe-work" {
		mode = "loser"
	}
	observer := &runtimeProcessObserver{
		delegate:     request.Input.ProcessLifecycleObserver,
		dispatchID:   request.Correlation.DispatchID,
		transitionID: request.Input.Dispatch.TransitionID,
		started:      service.started,
	}
	command, err := service.runner.Run(ctx, platformprocess.CommandRequest{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestRuntimeSupersededCommandHelperProcess", "--", mode},
		Env:     append(os.Environ(), "GO_WANT_RUNTIME_COMMAND_HELPER=1"), ProcessLifecycleObserver: observer,
	})
	service.results <- runtimeProcessResult{
		DispatchID: request.Correlation.DispatchID, TransitionID: request.Input.Dispatch.TransitionID,
		command: command, err: err,
	}
	if command.CancellationReason == platformprocess.CancellationReasonSuperseded {
		return workerexecution.ExecuteResult{
			Correlation: request.Correlation, Outcome: workerexecution.ExecutionOutcomeCanceled,
			Cancellation: &workerexecution.DispatchCancellation{Reason: workerexecution.DispatchCancellationReasonSuperseded},
		}, err
	}
	if err != nil {
		return workerexecution.ExecuteResult{Correlation: request.Correlation, Outcome: workerexecution.ExecutionOutcomeFailed}, err
	}
	return workerexecution.ExecuteResult{Correlation: request.Correlation, Outcome: workerexecution.ExecutionOutcomeAccepted}, nil
}

func (*runtimeSupersededProcessWorkers) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, errors.New("runtime process test Workers service does not support model invocation")
}

type runtimeProcessObserver struct {
	delegate     platformprocess.ProcessLifecycleObserver
	dispatchID   string
	transitionID string
	started      chan<- runtimeProcessStarted
}

func (observer *runtimeProcessObserver) ProcessStarted(info platformprocess.ProcessInfo) {
	observer.started <- runtimeProcessStarted{DispatchID: observer.dispatchID, TransitionID: observer.transitionID}
	if observer.delegate != nil {
		observer.delegate.ProcessStarted(info)
	}
}

func (observer *runtimeProcessObserver) ProcessExited(info platformprocess.ProcessInfo) {
	if observer.delegate != nil {
		observer.delegate.ProcessExited(info)
	}
}

func runtimeSameWorkObserveConsumeNet() *state.Net {
	net := buildSimpleNet()
	delete(net.Transitions, "t-process")
	resource := &state.ResourceDef{ID: "slot", Name: "Slot", Capacity: 1}
	resourcePlace, _ := state.GenerateResourcePlaces(resource, time.Time{})
	net.Resources[resource.ID] = resource
	net.Places[resourcePlace.ID] = resourcePlace
	net.Transitions["consume-work"] = &petri.Transition{
		ID: "consume-work", Name: "Consume Work", WorkerType: "worker-a",
		InputArcs: []petri.Arc{{
			ID: "consume-work-input", Name: "work", PlaceID: "task:init", Direction: petri.ArcInput,
			Mode: interfaces.ArcModeConsume, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
		OutputArcs: []petri.Arc{{
			ID: "consume-work-output", Name: "done", PlaceID: "task:done", Direction: petri.ArcOutput,
			Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
	}
	net.Transitions["observe-work"] = &petri.Transition{
		ID: "observe-work", Name: "Observe Work", WorkerType: "worker-b",
		InputArcs: []petri.Arc{
			{
				ID: "observe-work-input", Name: "work", PlaceID: "task:init", Direction: petri.ArcInput,
				Mode: interfaces.ArcModeObserve, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
			},
			{
				ID: "consume-slot-input", Name: "slot", PlaceID: resourcePlace.ID, Direction: petri.ArcInput,
				Mode: interfaces.ArcModeConsume, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
			},
		},
	}
	return net
}

var _ workerexecution.Service = (*runtimeSupersededProcessWorkers)(nil)
var _ platformprocess.ProcessLifecycleObserver = (*runtimeProcessObserver)(nil)
