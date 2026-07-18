package runtime_api

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestAPIUnifiedEventLogSmoke_PublicMutationReadAndSessionEventsShareTimeline(t *testing.T) {
	if testing.Short() {
		t.Skip("slow unified event-log smoke")
	}
	fixture := newUnifiedEventLogSmokeFixture(t)
	host := fixture.host
	stream := openRootRunFactoryEventHTTPStream(t, host)
	runStarted, first := requireFunctionalEventStreamPrelude(t, stream)
	assertUnifiedEventLogUpsert(t, host, fixture)

	liveEvents := collectUnifiedSmokeEvents(t, stream, []factoryapi.FactoryEvent{runStarted, first}, 4, 10*time.Second)
	assertUnifiedEventLogCompletedWork(t, host, fixture)

	stream.close()
	assertUnifiedSmokeCanonicalEventCoverage(t, liveEvents, fixture.traceID, fixture.requestID)
}

type unifiedEventLogSmokeFixture struct {
	host         *support.RootRunFunctionalHost
	traceID      string
	requestID    string
	draftWorkID  string
	reviewWorkID string
}

func newUnifiedEventLogSmokeFixture(t *testing.T) unifiedEventLogSmokeFixture {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	fixture := unifiedEventLogSmokeFixture{
		traceID:      "trace-unified-event-log-smoke",
		requestID:    "request-unified-event-log-smoke",
		draftWorkID:  "work-unified-event-log-draft",
		reviewWorkID: "work-unified-event-log-review",
	}
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: dir,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		if _, finished := host.Result(); finished {
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() cleanup error = %v", shutdownErr)
		}
	})
	fixture.host = host
	return fixture
}

func assertUnifiedEventLogUpsert(t *testing.T, host *support.RootRunFunctionalHost, fixture unifiedEventLogSmokeFixture) {
	t.Helper()

	requiredState := "complete"
	workTypeName := "task"
	upserted := putGeneratedWorkRequest(t, host.Endpoint(), fixture.requestID, factoryapi.WorkRequest{
		RequestId: fixture.requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:         "draft",
				WorkId:       stringPointer(fixture.draftWorkID),
				WorkTypeName: &workTypeName,
				TraceId:      stringPointer(fixture.traceID),
				Payload:      map[string]string{"title": "draft unified event log smoke"},
			},
			{
				Name:         "review",
				WorkId:       stringPointer(fixture.reviewWorkID),
				WorkTypeName: &workTypeName,
				TraceId:      stringPointer(fixture.traceID),
				Payload:      map[string]string{"title": "review unified event log smoke"},
			},
		},
		Relations: &[]factoryapi.Relation{{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "review",
			TargetWorkName: "draft",
			RequiredState:  &requiredState,
		}},
	})
	if upserted.RequestId != fixture.requestID {
		t.Fatalf("PUT /work-requests request_id = %q, want %q", upserted.RequestId, fixture.requestID)
	}
	if upserted.TraceId != fixture.traceID {
		t.Fatalf("PUT /work-requests trace_id = %q, want %q", upserted.TraceId, fixture.traceID)
	}
}

func assertUnifiedEventLogCompletedWork(t *testing.T, host *support.RootRunFunctionalHost, fixture unifiedEventLogSmokeFixture) {
	t.Helper()

	completedWork := waitForGeneratedWorkIDsComplete(t, host.Endpoint(), []string{fixture.draftWorkID, fixture.reviewWorkID}, 10*time.Second)
	if len(completedWork) != 2 {
		t.Fatalf("completed work count = %d, want 2", len(completedWork))
	}
	for _, item := range completedWork {
		if stringPointerValue(item.TraceId) != fixture.traceID || generatedWorkStateName(item.State) != "complete" {
			t.Fatalf("completed work item = %#v, want completed task work for trace %q", item, fixture.traceID)
		}
	}
}

func collectUnifiedSmokeEvents(t *testing.T, stream *factoryEventHTTPStream, initialEvents []factoryapi.FactoryEvent, wantCompletions int, timeout time.Duration) []factoryapi.FactoryEvent {
	t.Helper()

	events := append([]factoryapi.FactoryEvent(nil), initialEvents...)
	deadline := time.Now().Add(timeout)
	completions := 0
	for time.Now().Before(deadline) && completions < wantCompletions {
		event := nextUnifiedSmokeEvent(t, stream, time.Until(deadline), events)
		events = append(events, event)
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			completions++
		}
	}
	if completions != wantCompletions {
		t.Fatalf("collected %d completion events, want %d in live /events timeline: %#v", completions, wantCompletions, unifiedSmokeEventSummaries(events))
	}
	return events
}

func nextUnifiedSmokeEvent(t *testing.T, stream *factoryEventHTTPStream, timeout time.Duration, events []factoryapi.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	select {
	case event := <-stream.events:
		return event
	case err := <-stream.errs:
		t.Fatalf("/events stream error after %#v: %v", unifiedSmokeEventSummaries(events), err)
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for /events payload after %#v", unifiedSmokeEventSummaries(events))
	}
	return factoryapi.FactoryEvent{}
}

func assertUnifiedSmokeCanonicalEventCoverage(t *testing.T, events []factoryapi.FactoryEvent, traceID string, requestID string) {
	t.Helper()

	assertFunctionalEventsUseCanonicalVocabulary(t, events,
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeInferenceRequest,
		factoryapi.FactoryEventTypeInferenceResponse,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeFactoryStateResponse,
	)

	indices := requireUnifiedSmokeCanonicalEventIndices(t, events)
	assertUnifiedSmokeCanonicalEventOrdering(t, events, indices)
	assertUnifiedSmokeWorkRequestPayload(t, events[indices.workRequest], traceID, requestID)
	assertUnifiedSmokeRelationshipPayload(t, events[indices.relationship])
	assertUnifiedSmokeDispatchInferenceCorrelation(t, events, indices)
	assertUnifiedSmokeCanonicalEventCounts(t, events)
}

type unifiedSmokeCanonicalEventIndices struct {
	workRequest       int
	relationship      int
	dispatchRequest   int
	inferenceRequest  int
	inferenceResponse int
	dispatchResponse  int
}

func requireUnifiedSmokeCanonicalEventIndices(t *testing.T, events []factoryapi.FactoryEvent) unifiedSmokeCanonicalEventIndices {
	t.Helper()

	indices := unifiedSmokeCanonicalEventIndices{
		workRequest:       indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeWorkRequest, 0),
		relationship:      indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeRelationshipChangeRequest, 0),
		dispatchRequest:   indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchRequest, 0),
		inferenceRequest:  indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceRequest, 0),
		inferenceResponse: indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceResponse, 0),
		dispatchResponse:  indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchResponse, 0),
	}
	if indices.workRequest < 0 || indices.relationship < 0 || indices.dispatchRequest < 0 ||
		indices.inferenceRequest < 0 || indices.inferenceResponse < 0 ||
		indices.dispatchResponse < 0 {
		t.Fatalf("canonical event coverage missing required sequence in %v", functionalEventTypes(events))
	}
	return indices
}

func assertUnifiedSmokeCanonicalEventOrdering(t *testing.T, events []factoryapi.FactoryEvent, indices unifiedSmokeCanonicalEventIndices) {
	t.Helper()

	if !(indices.workRequest < indices.relationship &&
		indices.relationship < indices.dispatchRequest &&
		indices.dispatchRequest < indices.inferenceRequest &&
		indices.inferenceRequest < indices.inferenceResponse &&
		indices.inferenceResponse < indices.dispatchResponse) {
		t.Fatalf("canonical event ordering mismatch in %v", functionalEventTypes(events))
	}
}

func assertUnifiedSmokeWorkRequestPayload(t *testing.T, event factoryapi.FactoryEvent, traceID string, requestID string) {
	t.Helper()

	workRequest, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode WORK_REQUEST payload: %v", err)
	}
	if stringValueFromFunctionalPtr(event.Context.RequestId) != requestID {
		t.Fatalf("WORK_REQUEST request_id = %q, want %q", stringValueFromFunctionalPtr(event.Context.RequestId), requestID)
	}
	works := support.FactoryWorksValue(workRequest.Works)
	if len(works) != 2 {
		t.Fatalf("WORK_REQUEST works = %#v, want two batch items", works)
	}
	var traceIDs []string
	if event.Context.TraceIds != nil {
		traceIDs = *event.Context.TraceIds
	}
	if len(traceIDs) != 1 || traceIDs[0] != traceID {
		t.Fatalf("WORK_REQUEST trace IDs = %#v, want [%q]", traceIDs, traceID)
	}
}

func assertUnifiedSmokeRelationshipPayload(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	relation, err := event.Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("decode RELATIONSHIP_CHANGE_REQUEST payload: %v", err)
	}
	if relation.Relation.Type != factoryapi.RelationTypeDependsOn ||
		relation.Relation.SourceWorkName != "review" ||
		relation.Relation.TargetWorkName != "draft" ||
		stringValueFromFunctionalPtr(relation.Relation.RequiredState) != "complete" {
		t.Fatalf("relationship payload = %#v, want review depends on draft completion", relation)
	}
}

func assertUnifiedSmokeDispatchInferenceCorrelation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	indices unifiedSmokeCanonicalEventIndices,
) {
	t.Helper()

	if _, err := events[indices.inferenceRequest].Payload.AsInferenceRequestEventPayload(); err != nil {
		t.Fatalf("decode INFERENCE_REQUEST payload: %v", err)
	}
	if _, err := events[indices.inferenceResponse].Payload.AsInferenceResponseEventPayload(); err != nil {
		t.Fatalf("decode INFERENCE_RESPONSE payload: %v", err)
	}
	if _, err := events[indices.dispatchResponse].Payload.AsDispatchResponseEventPayload(); err != nil {
		t.Fatalf("decode DISPATCH_RESPONSE payload: %v", err)
	}
	requestDispatchID := stringValueFromFunctionalPtr(events[indices.inferenceRequest].Context.DispatchId)
	responseDispatchID := stringValueFromFunctionalPtr(events[indices.inferenceResponse].Context.DispatchId)
	dispatchResponseID := stringValueFromFunctionalPtr(events[indices.dispatchResponse].Context.DispatchId)
	if requestDispatchID != responseDispatchID || responseDispatchID != dispatchResponseID {
		t.Fatalf("inference/dispatch correlation mismatch: request=%s response=%s dispatch=%s", requestDispatchID, responseDispatchID, dispatchResponseID)
	}
}

func assertUnifiedSmokeCanonicalEventCounts(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	dispatchResponses := 0
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_RESPONSE payload: %v", err)
		}
		if payload.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("DISPATCH_RESPONSE outcome = %s, want ACCEPTED", payload.Outcome)
		}
		dispatchResponses++
	}
	if dispatchResponses != 4 {
		t.Fatalf("accepted dispatch response count = %d, want 4", dispatchResponses)
	}
	if countFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceRequest) != 4 ||
		countFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceResponse) != 4 {
		t.Fatalf("inference event counts = request:%d response:%d, want 4 each",
			countFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceRequest),
			countFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceResponse),
		)
	}
}

func unifiedSmokeEventSummaries(events []factoryapi.FactoryEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, string(event.Type)+"@"+event.Id)
	}
	return out
}

func stringPointer(value string) *string {
	return &value
}

func countFunctionalEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func indexOfFunctionalEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType, start int) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(events); i++ {
		if events[i].Type == eventType {
			return i
		}
	}
	return -1
}
