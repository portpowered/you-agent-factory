package functional_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

// portos:func-length-exception owner=agent-factory reason=unified-event-log-e2e-smoke review=2026-07-18 removal=split-live-record-replay-projection-and-divergence-assertions-before-next-unified-smoke-change
func TestUnifiedEventLogEndToEndSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow unified event-log smoke")
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "unified-event-log.replay.json")
	const traceID = "trace-unified-event-log-smoke"
	const requestID = "request-unified-event-log-smoke"
	const draftWorkID = "work-unified-event-log-draft"
	const reviewWorkID = "work-unified-event-log-review"
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"worker-a": {{
			Content: "draft stage one complete. COMPLETE",
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-unified-event-log-draft-step-one",
			},
		}, {
			Content: "review stage one complete. COMPLETE",
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-unified-event-log-review-step-one",
			},
		}},
		"worker-b": {{
			Content: "draft stage two complete. COMPLETE",
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-unified-event-log-draft-step-two",
			},
		}, {
			Content: "review stage two complete. COMPLETE",
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-unified-event-log-review-step-two",
			},
		}},
	})

	server := StartFunctionalServerWithConfig(
		t,
		dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			cfg.RecordPath = artifactPath
			cfg.RecordFlushInterval = 10 * time.Millisecond
			cfg.ProviderOverride = provider
			cfg.Logger = zap.NewNop()
		},
		factory.WithServiceMode(),
	)
	stream := openFactoryEventHTTPStream(t, server.URL()+"/events")
	runStarted, first := requireFunctionalEventStreamPrelude(t, stream)

	requiredState := "complete"
	workTypeName := "task"
	upserted := putGeneratedWorkRequest(t, server.URL(), requestID, factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:         "draft",
				WorkId:       stringPointer(draftWorkID),
				WorkTypeName: &workTypeName,
				TraceId:      stringPointer(traceID),
				Payload: map[string]string{
					"title": "draft unified event log smoke",
				},
			},
			{
				Name:         "review",
				WorkId:       stringPointer(reviewWorkID),
				WorkTypeName: &workTypeName,
				TraceId:      stringPointer(traceID),
				Payload: map[string]string{
					"title": "review unified event log smoke",
				},
			},
		},
		Relations: &[]factoryapi.Relation{{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "review",
			TargetWorkName: "draft",
			RequiredState:  &requiredState,
		}},
	})
	if upserted.RequestId != requestID {
		t.Fatalf("PUT /work-requests request_id = %q, want %q", upserted.RequestId, requestID)
	}
	if upserted.TraceId != traceID {
		t.Fatalf("PUT /work-requests trace_id = %q, want %q", upserted.TraceId, traceID)
	}

	liveEvents := collectUnifiedSmokeEvents(t, stream, []factoryapi.FactoryEvent{runStarted, first}, 4, 10*time.Second)
	completedWork := waitForGeneratedWorkIDsComplete(t, server.URL(), []string{draftWorkID, reviewWorkID}, 10*time.Second)
	if len(completedWork) != 2 {
		t.Fatalf("completed work count = %d, want 2", len(completedWork))
	}
	for _, token := range completedWork {
		if token.TraceId != traceID || token.PlaceId != "task:complete" {
			t.Fatalf("completed work token = %#v, want completed task token for trace %q", token, traceID)
		}
	}

	stopFunctionalServerForRecording(t, server)
	liveEvents = collectUnifiedSmokeEventsUntilRunResponse(t, stream, liveEvents, 10*time.Second)
	stream.close()
	assertUnifiedSmokeCanonicalEventCoverage(t, liveEvents, traceID, requestID)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertUnifiedSmokeCanonicalEventCoverage(t, artifact.Events, traceID, requestID)
	assertUnifiedSmokeArtifactHasEventTypes(t, artifact, []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeInferenceRequest,
		factoryapi.FactoryEventTypeInferenceResponse,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeFactoryStateResponse,
		factoryapi.FactoryEventTypeRunResponse,
	})
	assertLiveEventsMatchRecordedArtifact(t, liveEvents, artifact)

	dispatchCreated := requireUnifiedSmokeEvent(t, artifact.Events, factoryapi.FactoryEventTypeDispatchRequest)
	if _, err := dispatchCreated.Payload.AsDispatchRequestEventPayload(); err != nil {
		t.Fatalf("decode recorded dispatch event %q: %v", dispatchCreated.Id, err)
	}
	activeState, err := projections.ReconstructFactoryWorldState(artifact.Events, dispatchCreated.Context.Tick)
	if err != nil {
		t.Fatalf("reconstruct selected tick %d: %v", dispatchCreated.Context.Tick, err)
	}
	activeView := projections.BuildFactoryWorldView(activeState)
	if activeView.Runtime.InFlightDispatchCount == 0 {
		t.Fatalf("selected tick %d has no in-flight dispatches in view: %#v", dispatchCreated.Context.Tick, activeView.Runtime)
	}
	dispatchID := stringPointerValue(dispatchCreated.Context.DispatchId)
	if _, ok := activeState.ActiveDispatches[dispatchID]; !ok {
		t.Fatalf("selected tick %d active dispatches = %#v, want %s from event %s", dispatchCreated.Context.Tick, activeState.ActiveDispatches, dispatchID, dispatchCreated.Id)
	}

	finalTick := maxUnifiedSmokeTick(artifact.Events)
	finalState, err := projections.ReconstructFactoryWorldState(artifact.Events, finalTick)
	if err != nil {
		t.Fatalf("reconstruct final tick %d: %v", finalTick, err)
	}
	finalView := projections.BuildFactoryWorldView(finalState)
	if finalView.Runtime.Session.CompletedCount != 4 {
		t.Fatalf("final completed dispatch count = %d, want 4", finalView.Runtime.Session.CompletedCount)
	}
	assertUnifiedSmokeProjectionRetainsBatchInferenceAndRelations(t, finalState, finalView, traceID, draftWorkID, reviewWorkID)
	assertUnifiedSmokeTraceLinksEventsToView(t, finalState, traceID, dispatchID, dispatchCreated.Id, finalView.Runtime.Session.DispatchHistory)

	testutil.AssertReplaySucceeds(t, artifactPath, 10*time.Second)
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

func collectUnifiedSmokeEventsUntilRunResponse(t *testing.T, stream *factoryEventHTTPStream, initialEvents []factoryapi.FactoryEvent, timeout time.Duration) []factoryapi.FactoryEvent {
	t.Helper()

	events := append([]factoryapi.FactoryEvent(nil), initialEvents...)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		event := nextUnifiedSmokeEvent(t, stream, time.Until(deadline), events)
		events = append(events, event)
		if event.Type == factoryapi.FactoryEventTypeRunResponse {
			return events
		}
	}
	t.Fatalf("timed out waiting for RUN_RESPONSE in live /events timeline: %#v", unifiedSmokeEventSummaries(events))
	return nil
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

func stopFunctionalServerForRecording(t *testing.T, server *FunctionalServer) {
	t.Helper()

	server.cancel()
	select {
	case <-server.done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for functional server to stop and flush recording")
	}
}

func assertUnifiedSmokeArtifactHasEventTypes(t *testing.T, artifact *interfaces.ReplayArtifact, wantSubsequence []factoryapi.FactoryEventType) {
	t.Helper()

	next := 0
	for _, event := range artifact.Events {
		if next < len(wantSubsequence) && event.Type == wantSubsequence[next] {
			next++
		}
	}
	if next != len(wantSubsequence) {
		t.Fatalf("recorded event sequence = %#v, want subsequence %#v", unifiedSmokeEventSummaries(artifact.Events), wantSubsequence)
	}
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
		factoryapi.FactoryEventTypeRunResponse,
	)

	indices := requireUnifiedSmokeCanonicalEventIndices(t, events)
	assertUnifiedSmokeCanonicalEventOrdering(t, events, indices)
	assertUnifiedSmokeWorkRequestPayload(t, events[indices.workRequest], traceID, requestID)
	assertUnifiedSmokeRelationshipPayload(t, events[indices.relationship])
	assertUnifiedSmokeDispatchInferenceCorrelation(t, events, indices)
	assertUnifiedSmokeTerminalStatePayloads(t, events, indices)
	assertUnifiedSmokeCanonicalEventCounts(t, events)
}

type unifiedSmokeCanonicalEventIndices struct {
	workRequest       int
	relationship      int
	dispatchRequest   int
	inferenceRequest  int
	inferenceResponse int
	dispatchResponse  int
	factoryState      int
	runResponse       int
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
		factoryState:      lastIndexOfFunctionalEventType(events, factoryapi.FactoryEventTypeFactoryStateResponse),
		runResponse:       lastIndexOfFunctionalEventType(events, factoryapi.FactoryEventTypeRunResponse),
	}
	if indices.workRequest < 0 || indices.relationship < 0 || indices.dispatchRequest < 0 ||
		indices.inferenceRequest < 0 || indices.inferenceResponse < 0 ||
		indices.dispatchResponse < 0 || indices.factoryState < 0 || indices.runResponse < 0 {
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
		indices.inferenceResponse < indices.dispatchResponse &&
		indices.factoryState < indices.runResponse) {
		t.Fatalf("canonical event ordering mismatch in %v", functionalEventTypes(events))
	}
	if indices.runResponse != len(events)-1 {
		t.Fatalf("final event type = %s, want RUN_RESPONSE in %v", events[len(events)-1].Type, functionalEventTypes(events))
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
	works := factoryWorksValue(workRequest.Works)
	if len(works) != 2 {
		t.Fatalf("WORK_REQUEST works = %#v, want two batch items", works)
	}
	traceIDs := sliceValue(event.Context.TraceIds)
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
	requestDispatchID := stringPointerValue(events[indices.inferenceRequest].Context.DispatchId)
	responseDispatchID := stringPointerValue(events[indices.inferenceResponse].Context.DispatchId)
	dispatchResponseID := stringPointerValue(events[indices.dispatchResponse].Context.DispatchId)
	if requestDispatchID != responseDispatchID || responseDispatchID != dispatchResponseID {
		t.Fatalf("inference/dispatch correlation mismatch: request=%s response=%s dispatch=%s", requestDispatchID, responseDispatchID, dispatchResponseID)
	}
}

func assertUnifiedSmokeTerminalStatePayloads(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	indices unifiedSmokeCanonicalEventIndices,
) {
	t.Helper()

	stateResponse, err := events[indices.factoryState].Payload.AsFactoryStateResponseEventPayload()
	if err != nil {
		t.Fatalf("decode FACTORY_STATE_RESPONSE payload: %v", err)
	}
	if stateResponse.State != factoryapi.FactoryStateCompleted {
		t.Fatalf("FACTORY_STATE_RESPONSE state = %s, want COMPLETED", stateResponse.State)
	}
	runResponse, err := events[indices.runResponse].Payload.AsRunResponseEventPayload()
	if err != nil {
		t.Fatalf("decode RUN_RESPONSE payload: %v", err)
	}
	if runResponse.State == nil || *runResponse.State != factoryapi.FactoryStateCompleted {
		t.Fatalf("RUN_RESPONSE state = %#v, want COMPLETED", runResponse.State)
	}
}

func assertUnifiedSmokeCanonicalEventCounts(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	if countFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchResponse) != 4 {
		t.Fatalf("dispatch response count = %d, want 4", countFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchResponse))
	}
	if countFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceRequest) != 4 ||
		countFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceResponse) != 4 {
		t.Fatalf("inference event counts = request:%d response:%d, want 4 each",
			countFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceRequest),
			countFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceResponse),
		)
	}
}

func assertLiveEventsMatchRecordedArtifact(t *testing.T, liveEvents []factoryapi.FactoryEvent, artifact *interfaces.ReplayArtifact) {
	t.Helper()

	recordedByID := make(map[string]factoryapi.FactoryEvent, len(artifact.Events))
	for _, event := range artifact.Events {
		recordedByID[event.Id] = event
	}
	for _, live := range liveEvents {
		recorded, ok := recordedByID[live.Id]
		if !ok {
			t.Fatalf("live event %s (%s) missing from recorded artifact events: %#v", live.Id, live.Type, unifiedSmokeEventSummaries(artifact.Events))
		}
		if recorded.Type != live.Type || recorded.Context.Tick != live.Context.Tick {
			t.Fatalf("recorded event %s = type %s tick %d, live type %s tick %d", live.Id, recorded.Type, recorded.Context.Tick, live.Type, live.Context.Tick)
		}
		if unifiedSmokeDispatchID(recorded) != unifiedSmokeDispatchID(live) {
			t.Fatalf("recorded event %s dispatch id = %q, live dispatch id = %q", live.Id, unifiedSmokeDispatchID(recorded), unifiedSmokeDispatchID(live))
		}
		if strings.Join(unifiedSmokeWorkIDs(recorded), ",") != strings.Join(unifiedSmokeWorkIDs(live), ",") {
			t.Fatalf("recorded event %s work ids = %#v, live work ids = %#v", live.Id, unifiedSmokeWorkIDs(recorded), unifiedSmokeWorkIDs(live))
		}
	}
}

func assertUnifiedSmokeProjectionRetainsBatchInferenceAndRelations(
	t *testing.T,
	finalState interfaces.FactoryWorldState,
	finalView interfaces.FactoryWorldView,
	traceID string,
	draftWorkID string,
	reviewWorkID string,
) {
	t.Helper()

	if finalState.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("final reconstructed factory_state = %q, want %q", finalState.FactoryState, interfaces.FactoryStateCompleted)
	}
	request := finalState.WorkRequestsByID["request-unified-event-log-smoke"]
	if len(request.WorkItems) != 2 {
		t.Fatalf("reconstructed work request items = %#v, want 2", request.WorkItems)
	}
	relation := finalState.RelationsByWorkID[reviewWorkID]
	if len(relation) != 1 || relation[0].TargetWorkID != draftWorkID || relation[0].RequiredState != "complete" {
		t.Fatalf("reconstructed relations = %#v, want %s depends on %s complete", relation, reviewWorkID, draftWorkID)
	}
	trace, ok := finalState.TracesByID[traceID]
	if !ok {
		t.Fatalf("final reconstructed traces = %#v, want %q", finalState.TracesByID, traceID)
	}
	if !stringSliceContains(trace.WorkItemIDs, draftWorkID) || !stringSliceContains(trace.WorkItemIDs, reviewWorkID) {
		t.Fatalf("trace work items = %#v, want %s and %s", trace.WorkItemIDs, draftWorkID, reviewWorkID)
	}
	if len(trace.DispatchIDs) != 4 {
		t.Fatalf("trace dispatch IDs = %#v, want 4 dispatches", trace.DispatchIDs)
	}

	totalAttempts := 0
	for _, attemptsByID := range finalState.InferenceAttemptsByDispatchID {
		totalAttempts += len(attemptsByID)
	}
	if totalAttempts != 4 {
		t.Fatalf("reconstructed inference attempts = %d, want 4", totalAttempts)
	}
	if finalView.Runtime.Session.CompletedCount != 4 {
		t.Fatalf("projected completed dispatch count = %d, want 4", finalView.Runtime.Session.CompletedCount)
	}
}

func assertUnifiedSmokeTraceLinksEventsToView(
	t *testing.T,
	state interfaces.FactoryWorldState,
	traceID string,
	dispatchID string,
	dispatchEventID string,
	history []interfaces.FactoryWorldDispatchCompletion,
) {
	t.Helper()

	trace, ok := state.TracesByID[traceID]
	if !ok {
		t.Fatalf("final reconstructed state missing trace %q: %#v", traceID, state.TracesByID)
	}
	if !stringSliceContains(trace.DispatchIDs, dispatchID) {
		t.Fatalf("trace %q dispatch ids = %#v, want %s from event %s", traceID, trace.DispatchIDs, dispatchID, dispatchEventID)
	}
	for _, entry := range history {
		if entry.DispatchID == dispatchID {
			if !strings.HasSuffix(dispatchEventID, dispatchID) {
				t.Fatalf("dispatch event id %q does not include dashboard dispatch id %q", dispatchEventID, dispatchID)
			}
			return
		}
	}
	t.Fatalf("dashboard dispatch history = %#v, want dispatch %s from event %s", history, dispatchID, dispatchEventID)
}

func requireUnifiedSmokeEvent(t *testing.T, events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) factoryapi.FactoryEvent {
	t.Helper()

	for _, event := range events {
		if event.Type == eventType {
			return event
		}
	}
	t.Fatalf("missing event type %s in timeline %#v", eventType, unifiedSmokeEventSummaries(events))
	return factoryapi.FactoryEvent{}
}

func maxUnifiedSmokeTick(events []factoryapi.FactoryEvent) int {
	maxTick := 0
	for _, event := range events {
		if event.Context.Tick > maxTick {
			maxTick = event.Context.Tick
		}
	}
	return maxTick
}

func unifiedSmokeDispatchID(event factoryapi.FactoryEvent) string {
	if event.Context.DispatchId != nil {
		return *event.Context.DispatchId
	}
	return ""
}

func unifiedSmokeWorkIDs(event factoryapi.FactoryEvent) []string {
	if event.Context.WorkIds == nil {
		return nil
	}
	out := make([]string, len(*event.Context.WorkIds))
	copy(out, *event.Context.WorkIds)
	return out
}

func unifiedSmokeEventSummaries(events []factoryapi.FactoryEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, string(event.Type)+"@"+event.Id)
	}
	return out
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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

func lastIndexOfFunctionalEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == eventType {
			return i
		}
	}
	return -1
}
