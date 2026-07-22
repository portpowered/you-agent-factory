package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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
	// Recordings owns canonical inference-event construction and replay shape;
	// Workers owns provider emission ordering.
}

type providerBoundaryExecutor struct {
	record workerexecution.InferenceEventRecorder
}

func (e providerBoundaryExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	response := "provider contract output"
	session := &workerexecution.ProviderSessionMetadata{
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
				Outcome: workerexecution.InferenceOutcomeSucceeded, ProviderSession: session, Response: &response,
			},
		})
	}
	return workerexecution.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         workerexecution.OutcomeAccepted,
		Output:          response,
		ProviderSession: session,
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

func newSafeBoundaryRuntime(t *testing.T) (factory.Factory, *recordingfixtures.ScriptedRuntimeLedger) {
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

func submitSafeBoundaryRequests(t *testing.T, f factory.Factory) {
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

func assertDispatchResponseCount(t *testing.T, events []factoryapi.FactoryEvent, want int) {
	t.Helper()
	if got := countFactoryEventType(events, factoryapi.FactoryEventTypeDispatchResponse); got != want {
		t.Fatalf("dispatch completed event count = %d, want %d; events = %#v", got, want, events)
	}
}

func submitOrderedEventHistoryRequest(t *testing.T, f factory.Factory) {
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

func tickAndPauseRuntime(t *testing.T, f factory.Factory) {
	t.Helper()
	tickable := tickableFactory(t, f)
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
}

func assertOrderedEventSequence(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	wantTypes := append(append([]factoryapi.FactoryEventType(nil), runtimeStartupEventTypes()...), []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeFactoryStateResponse,
		factoryapi.FactoryEventTypeSessionLifecycleControl,
		factoryapi.FactoryEventTypeSessionPaused,
	}...)
	if len(events) != len(wantTypes) {
		t.Fatalf("event count = %d, want %d: %#v", len(events), len(wantTypes), events)
	}
	for i, wantType := range wantTypes {
		if events[i].Type != wantType {
			t.Fatalf("event[%d] type = %q, want %q", i, events[i].Type, wantType)
		}
		if events[i].Id == "" {
			t.Fatalf("event[%d] has empty id", i)
		}
		if i > 0 && events[i].Context.Tick < events[i-1].Context.Tick {
			t.Fatalf("event[%d] tick = %d before event[%d] tick = %d", i, events[i].Context.Tick, i-1, events[i-1].Context.Tick)
		}
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper intentionally checks the ordered event payload contract in one reviewer-readable pass.
func assertOrderedEventPayloads(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	batch, err := events[runtimeEventIndex(0)].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("work request payload: %v", err)
	}
	workRequestEvent := events[runtimeEventIndex(0)]
	if workRequestEvent.Context.RequestId == nil || batch.Type != factoryapi.WorkRequestTypeFactoryRequestBatch || firstRuntimeTestString(workRequestEvent.Context.TraceIds) != "trace-1" {
		t.Fatalf("work request payload = %#v, want canonical batch identity", batch)
	}
	if batch.Works == nil || len(*batch.Works) != 1 || stringValueForRuntimeTest((*batch.Works)[0].WorkId) != "work-1" {
		t.Fatalf("work request items = %#v, want work-1", batch.Works)
	}

	relation, err := events[runtimeEventIndex(1)].Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("relationship payload: %v", err)
	}
	relationEvent := events[runtimeEventIndex(1)]
	if relation.Relation.Type != factoryapi.RelationTypeDependsOn ||
		relationEvent.Context.WorkIds == nil ||
		stringValueForRuntimeTest(relation.Relation.TargetWorkId) != "upstream-1" {
		t.Fatalf("relationship payload = %#v, want submitted dependency", relation)
	}

	request, err := events[runtimeEventIndex(2)].Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("dispatch created payload: %v", err)
	}
	dispatchRequestEvent := events[runtimeEventIndex(2)]
	if stringValueForRuntimeTest(dispatchRequestEvent.Context.DispatchId) == "" || request.TransitionId != "t-process" {
		t.Fatalf("workstation request payload = %#v, want dispatch identity", request)
	}
	if len(request.Inputs) != 1 || request.Inputs[0].WorkId != "work-1" {
		t.Fatalf("workstation request inputs = %#v, want consumed work item", request.Inputs)
	}

	response, err := events[runtimeEventIndex(3)].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch completed payload: %v", err)
	}
	if stringValueForRuntimeTest(events[runtimeEventIndex(3)].Context.DispatchId) != stringValueForRuntimeTest(dispatchRequestEvent.Context.DispatchId) || response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("workstation response payload = %#v, want accepted dispatch response", response)
	}
	if response.OutputWork == nil || len(*response.OutputWork) == 0 || stringValueForRuntimeTest((*response.OutputWork)[0].WorkId) != "work-1" {
		t.Fatalf("output work = %#v, want completed work item", response.OutputWork)
	}
}

func assertRuntimeEventIDsStable(t *testing.T, f factory.Factory, events []factoryapi.FactoryEvent) {
	t.Helper()
	again := runtimeGeneratedEvents(t, f)
	for i := range events {
		if again[i].Id != events[i].Id {
			t.Fatalf("event[%d] id changed from %q to %q", i, events[i].Id, again[i].Id)
		}
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

func assertIdempotentBatchSubmit(t *testing.T, f factory.Factory, request work.WorkRequest) {
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

func assertFactoryEventTypesPrefix(t *testing.T, got []factoryapi.FactoryEventType, want ...factoryapi.FactoryEventType) {
	t.Helper()
	if len(got) < len(want) {
		t.Fatalf("event types = %v, want at least %v", got, want)
	}
	for i, expected := range want {
		if got[i] != expected {
			t.Fatalf("event[%d] type = %q, want %q (all types %v)", i, got[i], expected, got)
		}
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper keeps the batch replay event contract visible in one assertion owner.
func assertBatchRequestReplayEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	batch, err := events[runtimeEventIndex(0)].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("batch payload: %v", err)
	}
	workRequestEvent := events[runtimeEventIndex(0)]
	if stringValueForRuntimeTest(workRequestEvent.Context.RequestId) != "request-batch-events" ||
		stringValueForRuntimeTest(batch.Source) != "external-submit" ||
		firstRuntimeTestString(workRequestEvent.Context.TraceIds) != "trace-batch" {
		t.Fatalf("batch payload = %#v, want request/source/trace metadata", batch)
	}
	if batch.Works == nil || len(*batch.Works) != 2 ||
		stringValueForRuntimeTest((*batch.Works)[0].WorkId) != "work-first" ||
		stringValueForRuntimeTest((*batch.Works)[1].WorkId) != "work-second" ||
		stringValueForRuntimeTest((*batch.Works)[0].WorkTypeName) != "task" ||
		stringValueForRuntimeTest((*batch.Works)[1].WorkTypeName) != "task" {
		t.Fatalf("batch work items = %#v, want first and second", batch.Works)
	}
	if workRequestEvents := countFactoryEventsByType(events, factoryapi.FactoryEventTypeWorkRequest); workRequestEvents != 1 {
		t.Fatalf("work request events = %d, want 1 after idempotent retry", workRequestEvents)
	}

	relation, err := events[runtimeEventIndex(1)].Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("relationship payload: %v", err)
	}
	relationEvent := events[runtimeEventIndex(1)]
	if relation.Relation.SourceWorkName != "second" ||
		stringValueForRuntimeTest(relation.Relation.TargetWorkId) != "work-first" ||
		relation.Relation.TargetWorkName != "first" ||
		stringValueForRuntimeTest(relation.Relation.RequiredState) != "done" ||
		stringValueForRuntimeTest(relationEvent.Context.RequestId) != "request-batch-events" ||
		firstRuntimeTestString(relationEvent.Context.TraceIds) != "trace-batch" {
		t.Fatalf("relationship payload = %#v, want named batch dependency", relation)
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

// pkgmaintcheck:ignore-cyclomatic-complexity this helper keeps the generated batch event contract together across request, relation, and response assertions.
func assertGeneratedBatchEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	requestPayload, err := events[runtimeEventIndex(0)].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("request payload: %v", err)
	}
	workRequestEvent := events[runtimeEventIndex(0)]
	if stringValueForRuntimeTest(workRequestEvent.Context.RequestId) != "generated-request-events" ||
		stringValueForRuntimeTest(requestPayload.Source) != "worker-output:dispatch-parent" ||
		firstRuntimeTestString(workRequestEvent.Context.TraceIds) != "trace-generated" {
		t.Fatalf("request payload = %#v, want generated request metadata", requestPayload)
	}
	if got := strings.Join(sliceValueForRuntimeTest(requestPayload.ParentLineage), ","); got != "request-parent,work-parent" {
		t.Fatalf("parent lineage = %#v, want generated lineage metadata", requestPayload.ParentLineage)
	}
	if requestPayload.Works == nil || len(*requestPayload.Works) != 2 {
		t.Fatalf("request works = %#v, want generated work metadata", requestPayload.Works)
	}
	if requestPayload.Relations == nil || len(*requestPayload.Relations) != 1 {
		t.Fatalf("request relations = %#v, want canonical generated dependency", requestPayload.Relations)
	}
	if got := (*requestPayload.Relations)[0]; got.SourceWorkName != "review" ||
		got.TargetWorkName != "draft" ||
		stringValueForRuntimeTest(got.TargetWorkId) != "work-draft" ||
		stringValueForRuntimeTest(got.RequiredState) != "done" {
		t.Fatalf("request relation = %#v, want review depends on draft", got)
	}
	for _, work := range *requestPayload.Works {
		if stringValueForRuntimeTest(work.CurrentChainingTraceId) != "trace-generated" {
			t.Fatalf("generated work current chaining trace ID = %q, want trace-generated", stringValueForRuntimeTest(work.CurrentChainingTraceId))
		}
		if got := sliceValueForRuntimeTest(work.PreviousChainingTraceIds); len(got) != 0 {
			t.Fatalf("generated hook work previous chaining trace IDs = %#v, want none without consumed input lineage", got)
		}
	}

	relationPayload, err := events[runtimeEventIndex(1)].Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("relationship payload: %v", err)
	}
	relationEvent := events[runtimeEventIndex(1)]
	if relationPayload.Relation.SourceWorkName != "review" ||
		stringValueForRuntimeTest(relationPayload.Relation.TargetWorkId) != "work-draft" ||
		stringValueForRuntimeTest(relationEvent.Context.RequestId) != "generated-request-events" ||
		firstRuntimeTestString(relationEvent.Context.TraceIds) != "trace-generated" {
		t.Fatalf("relationship payload = %#v, want generated request dependency", relationPayload)
	}
}
