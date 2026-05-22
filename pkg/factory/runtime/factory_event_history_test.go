package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/replay"
)

func TestNew_SafeDiagnosticsBoundarySurvivesReplayAndSelectedTickProjection(t *testing.T) {
	f := newSafeBoundaryRuntime(t)
	submitSafeBoundaryRequests(t, f)
	tickUntilDispatchResponses(t, tickableFactory(t, f), f, 3)

	events := runtimeGeneratedEvents(t, f)
	assertDispatchResponseCount(t, events, 3)

	loaded := roundTripSafeBoundaryArtifact(t, events)
	assertDispatchResponseCount(t, loaded.Events, 3)
	assertThinDispatchResponsesOmitRetiredProviderAttemptFields(t, loaded.Events)

	worldState := reconstructWorldStateAtFinalTick(t, loaded.Events)
	assertSafeBoundaryWorldState(t, worldState)
	assertSafeBoundaryRequestViews(t, worldState)
	assertSafeBoundaryDoesNotLeakJSON(t, projections.BuildFactoryWorldView(worldState))
}

func TestFactoryEventHistory_RecordsOrderedEventsWithStableIDs(t *testing.T) {
	f := newPassingInlineRuntime(t)
	submitOrderedEventHistoryRequest(t, f)
	tickAndPauseRuntime(t, f)

	events := runtimeGeneratedEvents(t, f)
	assertOrderedEventSequence(t, events)
	assertOrderedEventPayloads(t, events)
	assertOrderedEventProjection(t, events)
	assertRuntimeEventIDsStable(t, f, events)
}

func TestNew_SubmitWorkRequestRecordsCanonicalWorkRequestEvent(t *testing.T) {
	f := newPassingInlineRuntime(t)
	tickable := tickableFactory(t, f)

	request := interfaces.WorkRequest{
		RequestID: "request-canonical-work-event",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
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

	events := runtimeGeneratedEvents(t, f)
	if len(events) < 3 {
		t.Fatalf("events = %#v, want run-started, initial structure, and canonical work request", events)
	}
	event := events[2]
	if event.Type != factoryapi.FactoryEventTypeWorkRequest {
		t.Fatalf("event[2] type = %q, want %q", event.Type, factoryapi.FactoryEventTypeWorkRequest)
	}
	if stringValueForRuntimeTest(event.Context.RequestId) != "request-canonical-work-event" ||
		firstRuntimeTestString(event.Context.TraceIds) != "trace-canonical" {
		t.Fatalf("event context = %#v, want submitted request and trace", event.Context)
	}
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("work request payload: %v", err)
	}
	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch ||
		payload.Works == nil ||
		len(*payload.Works) != 1 ||
		stringValueForRuntimeTest((*payload.Works)[0].WorkId) != "work-canonical" {
		t.Fatalf("work request payload = %#v, want canonical generated batch", payload)
	}
}

func TestFactoryEventHistory_BatchRequestAndRelationshipReplay(t *testing.T) {
	f := newPassingInlineRuntime(t)
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

	events := runtimeGeneratedEvents(t, f)
	assertFactoryEventTypesPrefix(t, factoryEventTypes(events),
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
	)
	assertBatchRequestReplayEvents(t, events)
	assertBatchRequestReplayProjection(t, events)
}

func TestFactoryEventHistory_GeneratedBatchPreservesMetadataAndOrdering(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithSubmissionHook(&generatedBatchHook{batch: generatedRuntimeBatchFixture()}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := tickableFactory(t, f).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	events := runtimeGeneratedEvents(t, f)
	assertFactoryEventTypesPrefix(t, factoryEventTypes(events),
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
	)
	assertGeneratedBatchEvents(t, events)
	assertGeneratedBatchProjection(t, events)
}

func newSafeBoundaryRuntime(t *testing.T) factory.Factory {
	t.Helper()
	f, err := New(
		factory.WithNet(buildSimpleNetWithFailureArc()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &safeDiagnosticsBoundaryExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f
}

func submitSafeBoundaryRequests(t *testing.T, f factory.Factory) {
	t.Helper()
	_, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{
		{WorkID: "work-safe-success", WorkTypeID: "task", TraceID: "trace-safe-success", Payload: json.RawMessage(`{"story":"safe success"}`)},
		{WorkID: "work-safe-failure", WorkTypeID: "task", TraceID: "trace-safe-failure", Payload: json.RawMessage(`{"story":"safe failure"}`)},
		{WorkID: "work-safe-windows-process-failure", WorkTypeID: "task", TraceID: "trace-safe-windows-process-failure", Payload: json.RawMessage(`{"story":"safe windows process failure"}`)},
	})
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
}

func tickUntilDispatchResponses(t *testing.T, tickable TickableFactory, f factory.Factory, want int) {
	t.Helper()
	for attempt := 0; attempt < want; attempt++ {
		if err := tickable.Tick(context.Background()); err != nil {
			t.Fatalf("Tick attempt %d: %v", attempt+1, err)
		}
		if countFactoryEventType(runtimeGeneratedEvents(t, f), factoryapi.FactoryEventTypeDispatchResponse) == want {
			return
		}
	}
}

func assertDispatchResponseCount(t *testing.T, events []factoryapi.FactoryEvent, want int) {
	t.Helper()
	if got := countFactoryEventType(events, factoryapi.FactoryEventTypeDispatchResponse); got != want {
		t.Fatalf("dispatch completed event count = %d, want %d; events = %#v", got, want, events)
	}
}

func roundTripSafeBoundaryArtifact(t *testing.T, events []factoryapi.FactoryEvent) *interfaces.ReplayArtifact {
	t.Helper()
	recordedAt := time.Date(2026, time.April, 21, 20, 0, 0, 0, time.UTC)
	artifact, err := replay.NewEventLogArtifactFromFactory(recordedAt, safeBoundaryGeneratedFactory(), nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifactFromFactory: %v", err)
	}
	artifact.Events = append(artifact.Events, events...)

	artifactPath := filepath.Join(t.TempDir(), "safe-boundary.replay.json")
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save replay artifact: %v", err)
	}

	artifactJSON, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("ReadFile replay artifact: %v", err)
	}
	assertSafeBoundaryDoesNotLeak(t, string(artifactJSON))

	loaded, err := replay.Load(artifactPath)
	if err != nil {
		t.Fatalf("Load replay artifact: %v", err)
	}
	return loaded
}

func reconstructWorldStateAtFinalTick(t *testing.T, events []factoryapi.FactoryEvent) interfaces.FactoryWorldState {
	t.Helper()
	worldState, err := projections.ReconstructFactoryWorldState(events, maxEventTick(events))
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	return worldState
}

func assertSafeBoundaryWorldState(t *testing.T, worldState interfaces.FactoryWorldState) {
	t.Helper()
	if got := len(worldState.CompletedDispatches); got != 3 {
		t.Fatalf("completed dispatch count = %d, want 3", got)
	}
	if got := worldState.FailureDetailsByWorkID["work-safe-failure"].FailureReason; got != string(interfaces.ProviderErrorTypeTimeout) {
		t.Fatalf("failed work detail reason = %q, want timeout", got)
	}
	windowsDetail := worldState.FailureDetailsByWorkID["work-safe-windows-process-failure"]
	if windowsDetail.FailureReason != string(interfaces.ProviderErrorTypeInternalServerError) {
		t.Fatalf("windows failed work detail reason = %q, want %q", windowsDetail.FailureReason, interfaces.ProviderErrorTypeInternalServerError)
	}
	if windowsDetail.FailureMessage != "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)" {
		t.Fatalf("windows failed work detail message = %q", windowsDetail.FailureMessage)
	}
	assertNoAuthRemediationText(t, windowsDetail.FailureMessage)
	assertSafeBoundaryDoesNotLeakJSON(t, worldState)
}

func assertSafeBoundaryRequestViews(t *testing.T, worldState interfaces.FactoryWorldState) {
	t.Helper()
	view := projections.BuildFactoryWorldView(worldState)
	if view.Runtime.Session.DispatchedCount != 3 || view.Runtime.Session.FailedCount != 2 || len(view.Runtime.Session.DispatchHistory) != 3 {
		t.Fatalf("session counts = %#v, want dispatched=3 failed=2 with three request history rows", view.Runtime.Session)
	}

	assertSafeBoundaryRequestView(t, worldState, requestViewForWork(t, worldState, "work-safe-success"),
		"work-safe-success", "resp-safe-success", "", "", "")
	assertSafeBoundaryRequestView(t, worldState, requestViewForWork(t, worldState, "work-safe-failure"),
		"work-safe-failure", "sess-safe-failure", string(interfaces.ProviderErrorFamilyRetryable), string(interfaces.ProviderErrorTypeTimeout), "provider timed out")

	windowsRequest := requestViewForWork(t, worldState, "work-safe-windows-process-failure")
	assertSafeBoundaryRequestView(t, worldState, windowsRequest,
		"work-safe-windows-process-failure",
		"sess-safe-windows-4294967295",
		string(interfaces.ProviderErrorFamilyRetryable),
		string(interfaces.ProviderErrorTypeInternalServerError),
		"provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)",
	)
	assertNoAuthRemediationText(t, stringValueForRuntimeTest(windowsRequest.Response.FailureMessage))
}

func submitOrderedEventHistoryRequest(t *testing.T, f factory.Factory) {
	t.Helper()
	_, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		WorkID:     "work-1",
		Name:       "Write PRD",
		WorkTypeID: "task",
		TraceID:    "trace-1",
		Relations: []interfaces.Relation{{
			Type:          interfaces.RelationDependsOn,
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
	wantTypes := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeFactoryStateResponse,
	}
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

func assertOrderedEventPayloads(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	batch, err := events[2].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("work request payload: %v", err)
	}
	if events[2].Context.RequestId == nil || batch.Type != factoryapi.WorkRequestTypeFactoryRequestBatch || firstRuntimeTestString(events[2].Context.TraceIds) != "trace-1" {
		t.Fatalf("work request payload = %#v, want canonical batch identity", batch)
	}
	if batch.Works == nil || len(*batch.Works) != 1 || stringValueForRuntimeTest((*batch.Works)[0].WorkId) != "work-1" {
		t.Fatalf("work request items = %#v, want work-1", batch.Works)
	}

	relation, err := events[3].Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("relationship payload: %v", err)
	}
	if relation.Relation.Type != factoryapi.RelationTypeDependsOn ||
		events[3].Context.WorkIds == nil ||
		stringValueForRuntimeTest(relation.Relation.TargetWorkId) != "upstream-1" {
		t.Fatalf("relationship payload = %#v, want submitted dependency", relation)
	}

	request, err := events[4].Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("dispatch created payload: %v", err)
	}
	if stringValueForRuntimeTest(events[4].Context.DispatchId) == "" || request.TransitionId != "t-process" {
		t.Fatalf("workstation request payload = %#v, want dispatch identity", request)
	}
	if len(request.Inputs) != 1 || request.Inputs[0].WorkId != "work-1" {
		t.Fatalf("workstation request inputs = %#v, want consumed work item", request.Inputs)
	}

	response, err := events[5].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch completed payload: %v", err)
	}
	if stringValueForRuntimeTest(events[5].Context.DispatchId) != stringValueForRuntimeTest(events[4].Context.DispatchId) || response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("workstation response payload = %#v, want accepted dispatch response", response)
	}
	if response.OutputWork == nil || len(*response.OutputWork) == 0 || stringValueForRuntimeTest((*response.OutputWork)[0].WorkId) != "work-1" {
		t.Fatalf("output work = %#v, want completed work item", response.OutputWork)
	}
}

func assertOrderedEventProjection(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	world, err := projections.ReconstructFactoryWorldState(events, events[5].Context.Tick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(world.CompletedDispatches) != 1 || world.CompletedDispatches[0].DispatchID != stringValueForRuntimeTest(events[4].Context.DispatchId) {
		t.Fatalf("CompletedDispatches = %#v, want completed dispatch reconstructed from canonical events", world.CompletedDispatches)
	}
	if got := world.PlaceOccupancyByID["task:done"].WorkItemIDs; len(got) != 1 || got[0] != "work-1" {
		t.Fatalf("task:done occupancy = %#v, want work-1", world.PlaceOccupancyByID["task:done"])
	}
	view := projections.BuildFactoryWorldView(world)
	if view.Runtime.Session.CompletedCount != 1 {
		t.Fatalf("CompletedCount = %d, want 1", view.Runtime.Session.CompletedCount)
	}
	if got := view.Runtime.PlaceTokenCounts["task:done"]; got != 1 {
		t.Fatalf("task:done count = %d, want 1", got)
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

func mustUnmarshalRuntimeWorkRequest(t *testing.T, body string) interfaces.WorkRequest {
	t.Helper()
	var request interfaces.WorkRequest
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("Unmarshal WorkRequest: %v", err)
	}
	return request
}

func assertIdempotentBatchSubmit(t *testing.T, f factory.Factory, request interfaces.WorkRequest) {
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

func assertBatchRequestReplayEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	batch, err := events[2].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("batch payload: %v", err)
	}
	if stringValueForRuntimeTest(events[2].Context.RequestId) != "request-batch-events" ||
		stringValueForRuntimeTest(batch.Source) != "external-submit" ||
		firstRuntimeTestString(events[2].Context.TraceIds) != "trace-batch" {
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

	relation, err := events[3].Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("relationship payload: %v", err)
	}
	if relation.Relation.SourceWorkName != "second" ||
		stringValueForRuntimeTest(relation.Relation.TargetWorkId) != "work-first" ||
		relation.Relation.TargetWorkName != "first" ||
		stringValueForRuntimeTest(relation.Relation.RequiredState) != "done" ||
		stringValueForRuntimeTest(events[3].Context.RequestId) != "request-batch-events" ||
		firstRuntimeTestString(events[3].Context.TraceIds) != "trace-batch" {
		t.Fatalf("relationship payload = %#v, want named batch dependency", relation)
	}
}

func assertBatchRequestReplayProjection(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	world, err := projections.ReconstructFactoryWorldState(events, events[3].Context.Tick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if got := world.WorkRequestsByID["request-batch-events"].WorkItems; len(got) != 2 {
		t.Fatalf("replayed batch work items = %#v, want 2 items", got)
	}
	relations := world.RelationsByWorkID["work-second"]
	if len(relations) != 1 || relations[0].TargetWorkID != "work-first" || relations[0].RequiredState != "done" {
		t.Fatalf("replayed relations = %#v, want second depends on first", relations)
	}
}

func generatedRuntimeBatchFixture() interfaces.GeneratedSubmissionBatch {
	return interfaces.GeneratedSubmissionBatch{
		Request: interfaces.WorkRequest{
			RequestID: "generated-request-events",
			Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
			Works: []interfaces.Work{
				{Name: "draft", WorkID: "work-draft", WorkTypeID: "task", TraceID: "trace-generated"},
				{Name: "review", WorkID: "work-review", WorkTypeID: "task"},
			},
			Relations: []interfaces.WorkRelation{{
				Type:           interfaces.WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "draft",
				RequiredState:  "done",
			}},
		},
		Metadata: interfaces.GeneratedSubmissionBatchMetadata{
			Source:        "worker-output:dispatch-parent",
			ParentLineage: []string{"request-parent", "work-parent"},
		},
		Submissions: []interfaces.SubmitRequest{{
			Name:        "review",
			WorkID:      "work-review",
			TargetState: "done",
			Tags:        map[string]string{"runtime": "true"},
		}},
	}
}

func assertGeneratedBatchEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	requestPayload, err := events[2].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("request payload: %v", err)
	}
	if stringValueForRuntimeTest(events[2].Context.RequestId) != "generated-request-events" ||
		stringValueForRuntimeTest(requestPayload.Source) != "worker-output:dispatch-parent" ||
		firstRuntimeTestString(events[2].Context.TraceIds) != "trace-generated" {
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

	relationPayload, err := events[3].Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("relationship payload: %v", err)
	}
	if relationPayload.Relation.SourceWorkName != "review" ||
		stringValueForRuntimeTest(relationPayload.Relation.TargetWorkId) != "work-draft" ||
		stringValueForRuntimeTest(events[3].Context.RequestId) != "generated-request-events" ||
		firstRuntimeTestString(events[3].Context.TraceIds) != "trace-generated" {
		t.Fatalf("relationship payload = %#v, want generated request dependency", relationPayload)
	}
}

func assertGeneratedBatchProjection(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	world, err := projections.ReconstructFactoryWorldState(events, events[3].Context.Tick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	replayed := world.WorkRequestsByID["generated-request-events"]
	if got := strings.Join(replayed.ParentLineage, ","); got != "request-parent,work-parent" {
		t.Fatalf("replayed parent lineage = %#v, want generated lineage metadata", replayed.ParentLineage)
	}
	if len(replayed.WorkItems) != 2 {
		t.Fatalf("replayed work items = %#v, want generated request work", replayed.WorkItems)
	}
	relations := world.RelationsByWorkID["work-review"]
	if len(relations) != 1 ||
		relations[0].TargetWorkID != "work-draft" ||
		relations[0].TargetWorkName != "draft" ||
		relations[0].RequiredState != "done" {
		t.Fatalf("replayed generated relations = %#v, want review depends on draft", relations)
	}
}
