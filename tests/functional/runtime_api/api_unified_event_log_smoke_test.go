package runtime_api

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// portos:func-length-exception owner=agent-factory reason=unified-event-log-e2e-smoke review=2026-07-18 removal=split-live-record-replay-projection-and-divergence-assertions-before-next-unified-smoke-change
func TestAPIUnifiedEventLogSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline(t *testing.T) {
	if testing.Short() {
		t.Skip("slow unified event-log smoke")
	}
	// C06-ISOLATED CASE-22: recording finalization and the second replay
	// server must observe a closed first process and artifact, preserving the
	// live/replay identity witness.
	fixture := newUnifiedEventLogSmokeFixture(t)
	server := fixture.server
	stream := openDefaultSessionFactoryEventHTTPStream(t, server.URL())
	runStarted, first := requireFunctionalEventStreamPrelude(t, stream)
	assertUnifiedEventLogUpsert(t, server, fixture)

	liveEvents := collectUnifiedSmokeEvents(t, stream, []factoryapi.FactoryEvent{runStarted, first}, 4, 10*time.Second)
	assertUnifiedEventLogCompletedWork(t, server, fixture)

	stopFunctionalServerForRecording(t, server)
	liveEvents = collectUnifiedSmokeEventsUntilRunResponse(t, stream, liveEvents, 10*time.Second)
	stream.close()
	assertUnifiedEventLogRecording(t, liveEvents, fixture)
	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", fixture.artifactPath},
	})
	support.WaitForTerminalStatus(t, replayServer.URL(), 10*time.Second)
	replayServer.Stop(t)
}

type unifiedEventLogSmokeFixture struct {
	server       *functionalAPIServer
	artifactPath string
	traceID      string
	requestID    string
	draftWorkID  string
	reviewWorkID string
}

func newUnifiedEventLogSmokeFixture(t *testing.T) unifiedEventLogSmokeFixture {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, testutil.MustRepoPath(t, "tests/functional_test/testdata/service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "unified-event-log.replay.json")
	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"worker-a": {{
			Content: "draft stage one complete. COMPLETE",
			ProviderSession: &providers.SessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-unified-event-log-draft-step-one",
			},
		}, {
			Content: "review stage one complete. COMPLETE",
			ProviderSession: &providers.SessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-unified-event-log-review-step-one",
			},
		}},
		"worker-b": {{
			Content: "draft stage two complete. COMPLETE",
			ProviderSession: &providers.SessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-unified-event-log-draft-step-two",
			},
		}, {
			Content: "review stage two complete. COMPLETE",
			ProviderSession: &providers.SessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-unified-event-log-review-step-two",
			},
		}},
	})
	fixture := unifiedEventLogSmokeFixture{
		artifactPath: artifactPath,
		traceID:      "trace-unified-event-log-smoke",
		requestID:    "request-unified-event-log-smoke",
		draftWorkID:  "work-unified-event-log-draft",
		reviewWorkID: "work-unified-event-log-review",
	}
	fixture.server = startFunctionalServerWithArgs(
		t,
		dir,
		false,
		[]string{"--record", artifactPath},
		withProvider(provider),
	)
	return fixture
}

func assertUnifiedEventLogUpsert(t *testing.T, server *functionalAPIServer, fixture unifiedEventLogSmokeFixture) {
	t.Helper()

	requiredState := "complete"
	workTypeName := "task"
	upserted := putGeneratedWorkRequest(t, server.URL(), fixture.requestID, factoryapi.WorkRequest{
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
		Relations: &[]factoryapi.WorkRequestRelation{{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "review",
			TargetWorkName: stringPointer("draft"),
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

func assertUnifiedEventLogCompletedWork(t *testing.T, server *functionalAPIServer, fixture unifiedEventLogSmokeFixture) {
	t.Helper()

	completedWork := waitForGeneratedWorkIDsComplete(t, server.URL(), []string{fixture.draftWorkID, fixture.reviewWorkID}, 10*time.Second)
	if len(completedWork) != 2 {
		t.Fatalf("completed work count = %d, want 2", len(completedWork))
	}
	for _, item := range completedWork {
		if stringPointerValue(item.TraceId) != fixture.traceID || generatedWorkStateName(item.State) != "complete" {
			t.Fatalf("completed work item = %#v, want completed task work for trace %q", item, fixture.traceID)
		}
	}
}

func assertUnifiedEventLogRecording(t *testing.T, liveEvents []factoryapi.FactoryEvent, fixture unifiedEventLogSmokeFixture) *interfaces.ReplayArtifact {
	t.Helper()

	assertUnifiedSmokeCanonicalEventCoverage(t, liveEvents, fixture.traceID, fixture.requestID)
	artifact := testutil.LoadReplayArtifact(t, fixture.artifactPath)
	generatedEvents := testutil.GeneratedFactoryEvents(t, artifact.Events)
	assertUnifiedSmokeCanonicalEventCoverage(t, generatedEvents, fixture.traceID, fixture.requestID)
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
	return artifact
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

func stopFunctionalServerForRecording(t *testing.T, server *functionalAPIServer) {
	t.Helper()
	server.Stop(t)
}

func assertUnifiedSmokeArtifactHasEventTypes(t *testing.T, artifact *interfaces.ReplayArtifact, wantSubsequence []factoryapi.FactoryEventType) {
	t.Helper()

	next := 0
	for _, event := range artifact.Events {
		if next < len(wantSubsequence) && string(event.Type) == string(wantSubsequence[next]) {
			next++
		}
	}
	if next != len(wantSubsequence) {
		t.Fatalf("recorded event sequence = %#v, want subsequence %#v", unifiedSmokeEventSummaries(testutil.GeneratedFactoryEvents(t, artifact.Events)), wantSubsequence)
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
	// RUN_RESPONSE now precedes session completion, so the recorded timeline may carry
	// SESSION_RESULT_UPDATED and SESSION_COMPLETED afterward; no run-scoped event may
	// follow it (PR #1997).
	for _, event := range events[indices.runResponse+1:] {
		if event.Type != factoryapi.FactoryEventTypeSessionResultUpdated && event.Type != factoryapi.FactoryEventTypeSessionCompleted {
			t.Fatalf("event type = %s follows RUN_RESPONSE, want session-lifecycle trailer in %v", event.Type, functionalEventTypes(events))
		}
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
	for _, event := range testutil.GeneratedFactoryEvents(t, artifact.Events) {
		recordedByID[event.Id] = event
	}
	for _, live := range liveEvents {
		recorded, ok := recordedByID[live.Id]
		if !ok {
			t.Fatalf("live event %s (%s) missing from recorded artifact events: %#v", live.Id, live.Type, unifiedSmokeEventSummaries(testutil.GeneratedFactoryEvents(t, artifact.Events)))
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

func lastIndexOfFunctionalEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == eventType {
			return i
		}
	}
	return -1
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
