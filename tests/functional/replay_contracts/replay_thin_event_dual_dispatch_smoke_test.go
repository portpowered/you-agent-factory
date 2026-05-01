package replay_contracts

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryboundary "github.com/portpowered/agent-factory/pkg/api"
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

const dualDispatchSmokeRequestID = "request-thin-dual-dispatch-smoke"
const dualDispatchSmokeTraceID = "trace-thin-dual-dispatch-smoke"
const dualDispatchSmokeModelWorkID = "work-thin-dual-dispatch-model"
const dualDispatchSmokeScriptWorkID = "work-thin-dual-dispatch-script"
const dualDispatchSmokeScriptWorkType = "script-task"

type dualDispatchSmokeFixture struct {
	liveEvents   []factoryapi.FactoryEvent
	artifact     *interfaces.ReplayArtifact
	artifactPath string
	requestID    string
	traceID      string
	modelWorkID  string
	scriptWorkID string
}

func TestReplayThinEventDualDispatchSmoke_SharedArtifactCapturesModelAndScriptDispatches(t *testing.T) {
	smoke := runThinEventDualDispatchSmoke(t)

	assertThinEventWorkRequestContainsBothPaths(t, smoke.liveEvents, smoke.requestID, smoke.traceID, smoke.modelWorkID, smoke.scriptWorkID)
	assertThinEventDispatchLifecycleForWork(t, smoke.liveEvents, smoke.modelWorkID, factoryapi.FactoryEventTypeInferenceRequest, factoryapi.FactoryEventTypeInferenceResponse)
	assertThinEventDispatchLifecycleForWork(t, smoke.liveEvents, smoke.scriptWorkID, factoryapi.FactoryEventTypeScriptRequest, factoryapi.FactoryEventTypeScriptResponse)
	assertThinEventDispatchLifecycleForWork(t, smoke.artifact.Events, smoke.modelWorkID, factoryapi.FactoryEventTypeInferenceRequest, factoryapi.FactoryEventTypeInferenceResponse)
	assertThinEventDispatchLifecycleForWork(t, smoke.artifact.Events, smoke.scriptWorkID, factoryapi.FactoryEventTypeScriptRequest, factoryapi.FactoryEventTypeScriptResponse)
	assertLiveEventsMatchRecordedArtifact(t, smoke.liveEvents, smoke.artifact)
}

func TestReplayThinEventDualDispatchSmoke_SharedArtifactGuardsThinRawContract(t *testing.T) {
	smoke := runThinEventDualDispatchSmoke(t)

	assertThinEventRawArtifactOmitsRetiredFields(t, smoke.artifact.Events)
	assertThinEventModelAttemptCorrelation(t, smoke.artifact.Events, smoke.modelWorkID)
	assertThinEventScriptBoundaryFacts(t, smoke.artifact.Events, smoke.scriptWorkID)
}

func TestReplayThinEventDualDispatchSmoke_ReplayAndReadersReuseSharedArtifact(t *testing.T) {
	smoke := runThinEventDualDispatchSmoke(t)

	replayHarness := testutil.AssertReplaySucceeds(t, smoke.artifactPath, 10*time.Second)
	replayHarness.Service.Assert().
		PlaceTokenCount("task:complete", 1).
		PlaceTokenCount(dualDispatchSmokeScriptWorkType+":done", 1).
		HasNoTokenInPlace("task:failed").
		HasNoTokenInPlace(dualDispatchSmokeScriptWorkType + ":failed")

	finalTick := lastFactoryEventTick(smoke.artifact.Events)
	worldState, err := projections.ReconstructFactoryWorldState(smoke.artifact.Events, finalTick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	assertThinEventReconstructedModelReader(t, smoke, worldState)
	assertThinEventReconstructedScriptReader(t, smoke, worldState)
	assertThinEventWorkstationRequestProjection(t, smoke, worldState)
}

func runThinEventDualDispatchSmoke(t *testing.T) dualDispatchSmokeFixture {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	configureThinEventDualDispatchFixture(t, dir)

	artifactPath := filepath.Join(t.TempDir(), "thin-event-dual-dispatch.replay.json")
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"worker-a": {{
			Content: "model draft complete. COMPLETE",
		}},
		"worker-b": {{
			Content: "model review complete. COMPLETE",
		}},
	})
	runner := newRecordingCommandRunner("script dispatch complete")
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithCommandRunner(runner),
		testutil.WithRecordPath(artifactPath),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	smoke := dualDispatchSmokeFixture{
		requestID:    dualDispatchSmokeRequestID,
		traceID:      dualDispatchSmokeTraceID,
		modelWorkID:  dualDispatchSmokeModelWorkID,
		scriptWorkID: dualDispatchSmokeScriptWorkID,
	}
	harness.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: smoke.requestID,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{
				Name:       "model-path",
				WorkID:     smoke.modelWorkID,
				WorkTypeID: "task",
				TraceID:    smoke.traceID,
				Payload: map[string]any{
					"title": "model dual-dispatch smoke",
				},
			},
			{
				Name:       "script-path",
				WorkID:     smoke.scriptWorkID,
				WorkTypeID: dualDispatchSmokeScriptWorkType,
				TraceID:    smoke.traceID,
				Payload: map[string]any{
					"title": "script dual-dispatch smoke",
				},
			},
		},
	})

	harness.RunUntilComplete(t, 10*time.Second)
	harness.Assert().
		PlaceTokenCount("task:complete", 1).
		PlaceTokenCount(dualDispatchSmokeScriptWorkType+":done", 1).
		HasNoTokenInPlace("task:failed").
		HasNoTokenInPlace(dualDispatchSmokeScriptWorkType + ":failed")

	if got := provider.CallCount("worker-a"); got != 1 {
		t.Fatalf("worker-a provider calls = %d, want 1", got)
	}
	if got := provider.CallCount("worker-b"); got != 1 {
		t.Fatalf("worker-b provider calls = %d, want 1", got)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("script command runner calls = %d, want 1", got)
	}

	events, err := harness.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	smoke.liveEvents = events
	smoke.artifactPath = artifactPath
	smoke.artifact = testutil.LoadReplayArtifact(t, artifactPath)
	return smoke
}

func configureThinEventDualDispatchFixture(t *testing.T, dir string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["workTypes"] = append(cfg["workTypes"].([]any), map[string]any{
			"name": dualDispatchSmokeScriptWorkType,
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "done", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		})
		cfg["workers"] = append(cfg["workers"].([]any), map[string]any{"name": "script-worker"})
		cfg["workstations"] = append(cfg["workstations"].([]any), map[string]any{
			"name":   "run-script",
			"worker": "script-worker",
			"inputs": []any{
				map[string]any{"workType": dualDispatchSmokeScriptWorkType, "state": "init"},
			},
			"outputs": []any{
				map[string]any{"workType": dualDispatchSmokeScriptWorkType, "state": "done"},
			},
			"onFailure": map[string]any{"workType": dualDispatchSmokeScriptWorkType, "state": "failed"},
		})
	})

	writeReplayFixtureFile(t, dir, []string{"workstations", "run-script", "AGENTS.md"}, "---\ntype: MODEL_WORKSTATION\n---\nExecute the script.\n")
	writeReplayFixtureFile(t, dir, []string{"workers", "script-worker", "AGENTS.md"}, "---\ntype: SCRIPT_WORKER\ncommand: script-tool\nargs:\n  - --mode\n  - smoke\n---\nExecute the script.\n")
}

func writeReplayFixtureFile(t *testing.T, dir string, pathParts []string, content string) {
	t.Helper()

	path := filepath.Join(append([]string{dir}, pathParts...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertThinEventWorkRequestContainsBothPaths(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	requestID string,
	traceID string,
	modelWorkID string,
	scriptWorkID string,
) {
	t.Helper()

	index := indexOfReplayContractEventType(events, factoryapi.FactoryEventTypeWorkRequest, 0)
	if index < 0 {
		t.Fatalf("event order = %v, want WORK_REQUEST for the shared smoke batch", replayContractEventTypes(events))
	}
	event := events[index]
	workRequest, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode WORK_REQUEST payload: %v", err)
	}
	if stringPointerValue(event.Context.RequestId) != requestID {
		t.Fatalf("WORK_REQUEST request_id = %q, want %q", stringPointerValue(event.Context.RequestId), requestID)
	}
	if got := stringSlicePointerValue(event.Context.TraceIds); len(got) != 1 || got[0] != traceID {
		t.Fatalf("WORK_REQUEST trace_ids = %#v, want [%q]", got, traceID)
	}

	works := factoryWorksValue(workRequest.Works)
	if len(works) != 2 {
		t.Fatalf("WORK_REQUEST works = %#v, want 2", works)
	}
	if !factoryWorksIncludeID(works, modelWorkID) || !factoryWorksIncludeID(works, scriptWorkID) {
		t.Fatalf("WORK_REQUEST works = %#v, want model work %q and script work %q", works, modelWorkID, scriptWorkID)
	}
}

func assertThinEventDispatchLifecycleForWork(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	workID string,
	requestType factoryapi.FactoryEventType,
	responseType factoryapi.FactoryEventType,
) {
	t.Helper()

	dispatchIndex := requireThinEventDispatchRequestIndexForWork(t, events, workID)
	dispatchID := thinEventDispatchIDFromEvent(t, events[dispatchIndex], workID)

	requestIndex := requireThinEventDispatchEventIndexAfter(t, events, requestType, dispatchID, dispatchIndex+1)
	responseIndex := requireThinEventDispatchEventIndexAfter(t, events, responseType, dispatchID, requestIndex+1)
	completedIndex := requireThinEventDispatchEventIndexAfter(t, events, factoryapi.FactoryEventTypeDispatchResponse, dispatchID, responseIndex+1)

	if requestIndex > responseIndex || responseIndex > completedIndex {
		t.Fatalf("dispatch %s event order = %v, want %s then %s then DISPATCH_RESPONSE", dispatchID, replayContractEventTypes(events), requestType, responseType)
	}

	for _, index := range []int{dispatchIndex, requestIndex, responseIndex, completedIndex} {
		if !functionalEventContextHasWorkID(events[index], workID) {
			t.Fatalf("event %s (%s) work_ids = %#v, want %q", events[index].Id, events[index].Type, events[index].Context.WorkIds, workID)
		}
	}
}

func requireThinEventDispatchRequestIndexForWork(t *testing.T, events []factoryapi.FactoryEvent, workID string) int {
	t.Helper()

	for i, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchRequest && functionalEventContextHasWorkID(event, workID) {
			return i
		}
	}
	t.Fatalf("missing DISPATCH_REQUEST for work %q in %v", workID, replayContractEventTypes(events))
	return -1
}

func requireThinEventDispatchEventIndexAfter(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	eventType factoryapi.FactoryEventType,
	dispatchID string,
	start int,
) int {
	t.Helper()

	for i := start; i < len(events); i++ {
		if events[i].Type == eventType && stringPointerValue(events[i].Context.DispatchId) == dispatchID {
			return i
		}
	}
	t.Fatalf("missing %s for dispatch %q in %v", eventType, dispatchID, replayContractEventTypes(events))
	return -1
}

func functionalEventContextHasWorkID(event factoryapi.FactoryEvent, workID string) bool {
	for _, candidate := range stringSlicePointerValue(event.Context.WorkIds) {
		if candidate == workID {
			return true
		}
	}
	return false
}

func factoryWorksIncludeID(works []factoryapi.Work, workID string) bool {
	for _, work := range works {
		if stringPointerValue(work.WorkId) == workID {
			return true
		}
	}
	return false
}

func assertThinEventRawArtifactOmitsRetiredFields(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	rawEvents := rawThinEventArtifactEvents(t, events)
	for _, event := range rawEvents {
		switch rawThinEventType(t, event) {
		case string(factoryapi.FactoryEventTypeWorkRequest):
			for _, path := range []string{
				"payload.requestId",
				"payload.traceIds",
				"payload.workIds",
				"payload.dispatchId",
			} {
				assertThinEventRawPathAbsent(t, event, path)
			}
		case string(factoryapi.FactoryEventTypeDispatchRequest):
			assertThinEventRawDispatchPayloadIsThin(t, event)
		case string(factoryapi.FactoryEventTypeDispatchResponse):
			assertThinEventRawDispatchPayloadIsThin(t, event)
			for _, path := range []string{
				"payload.inferenceRequestId",
				"payload.attempt",
				"payload.prompt",
				"payload.response",
				"payload.errorClass",
				"payload.scriptRequestId",
				"payload.command",
				"payload.args",
				"payload.stdout",
				"payload.stderr",
				"payload.exitCode",
				"payload.failureType",
			} {
				assertThinEventRawPathAbsent(t, event, path)
			}
		case string(factoryapi.FactoryEventTypeScriptRequest), string(factoryapi.FactoryEventTypeScriptResponse):
			for _, path := range []string{"payload.stdin", "payload.env"} {
				assertThinEventRawPathAbsent(t, event, path)
			}
		}
	}
}

func assertThinEventModelAttemptCorrelation(t *testing.T, events []factoryapi.FactoryEvent, workID string) {
	t.Helper()

	rawEvents := rawThinEventArtifactEvents(t, events)
	dispatchIndex := requireThinEventDispatchRequestIndexForWork(t, events, workID)
	dispatchID := thinEventDispatchIDFromEvent(t, events[dispatchIndex], workID)

	requestIndex := requireThinEventDispatchEventIndexAfter(t, events, factoryapi.FactoryEventTypeInferenceRequest, dispatchID, dispatchIndex+1)
	request := assertReplayInferenceRequest(t, events[requestIndex], dispatchID, 1)
	responseIndex := requireThinEventDispatchEventIndexAfter(t, events, factoryapi.FactoryEventTypeInferenceResponse, dispatchID, requestIndex+1)
	response := assertReplayInferenceResponse(t, events[responseIndex], dispatchID, request.InferenceRequestId, request.Attempt)

	requireThinEventRawPath(t, rawEvents[requestIndex], "payload.inferenceRequestId")
	requireThinEventRawPath(t, rawEvents[responseIndex], "payload.inferenceRequestId")

	if stringPointerValue(events[requestIndex].Context.DispatchId) != dispatchID {
		t.Fatalf("INFERENCE_REQUEST context.dispatchId = %q, want %q", stringPointerValue(events[requestIndex].Context.DispatchId), dispatchID)
	}
	if stringPointerValue(events[responseIndex].Context.DispatchId) != dispatchID {
		t.Fatalf("INFERENCE_RESPONSE context.dispatchId = %q, want %q", stringPointerValue(events[responseIndex].Context.DispatchId), dispatchID)
	}
	if !functionalEventContextHasWorkID(events[requestIndex], workID) || !functionalEventContextHasWorkID(events[responseIndex], workID) {
		t.Fatalf("inference event work IDs = request:%#v response:%#v, want %q", events[requestIndex].Context.WorkIds, events[responseIndex].Context.WorkIds, workID)
	}
	if request.InferenceRequestId == "" {
		t.Fatalf("INFERENCE_REQUEST inferenceRequestId is empty")
	}
	if stringPointerValue(response.Response) == "" {
		t.Fatalf("INFERENCE_RESPONSE response is empty for dispatch %q", dispatchID)
	}
}

func assertThinEventScriptBoundaryFacts(t *testing.T, events []factoryapi.FactoryEvent, workID string) {
	t.Helper()

	rawEvents := rawThinEventArtifactEvents(t, events)
	dispatchIndex := requireThinEventDispatchRequestIndexForWork(t, events, workID)
	dispatchID := thinEventDispatchIDFromEvent(t, events[dispatchIndex], workID)

	requestIndex := requireThinEventDispatchEventIndexAfter(t, events, factoryapi.FactoryEventTypeScriptRequest, dispatchID, dispatchIndex+1)
	request, err := events[requestIndex].Payload.AsScriptRequestEventPayload()
	if err != nil {
		t.Fatalf("decode SCRIPT_REQUEST payload for %s: %v", workID, err)
	}
	responseIndex := requireThinEventDispatchEventIndexAfter(t, events, factoryapi.FactoryEventTypeScriptResponse, dispatchID, requestIndex+1)
	response, err := events[responseIndex].Payload.AsScriptResponseEventPayload()
	if err != nil {
		t.Fatalf("decode SCRIPT_RESPONSE payload for %s: %v", workID, err)
	}

	for _, path := range []string{"payload.scriptRequestId", "payload.command", "payload.args"} {
		requireThinEventRawPath(t, rawEvents[requestIndex], path)
	}
	for _, path := range []string{"payload.scriptRequestId", "payload.outcome", "payload.stdout", "payload.stderr", "payload.durationMillis", "payload.exitCode"} {
		requireThinEventRawPath(t, rawEvents[responseIndex], path)
	}

	if request.ScriptRequestId == "" {
		t.Fatalf("SCRIPT_REQUEST scriptRequestId is empty")
	}
	if request.DispatchId != dispatchID || response.DispatchId != dispatchID {
		t.Fatalf("script dispatch correlation mismatch: dispatch=%s request=%s response=%s", dispatchID, request.DispatchId, response.DispatchId)
	}
	if response.ScriptRequestId != request.ScriptRequestId {
		t.Fatalf("script request correlation mismatch: request=%s response=%s", request.ScriptRequestId, response.ScriptRequestId)
	}
	if request.Command != "script-tool" {
		t.Fatalf("SCRIPT_REQUEST command = %q, want script-tool", request.Command)
	}
	if len(request.Args) != 2 || request.Args[0] != "--mode" || request.Args[1] != "smoke" {
		t.Fatalf("SCRIPT_REQUEST args = %#v, want [\"--mode\", \"smoke\"]", request.Args)
	}
	if response.Outcome != factoryapi.ScriptExecutionOutcomeSucceeded {
		t.Fatalf("SCRIPT_RESPONSE outcome = %s, want SUCCEEDED", response.Outcome)
	}
	if response.ExitCode == nil || *response.ExitCode != 0 {
		t.Fatalf("SCRIPT_RESPONSE exitCode = %#v, want 0", response.ExitCode)
	}
	if response.Stdout != "script dispatch complete" {
		t.Fatalf("SCRIPT_RESPONSE stdout = %q, want %q", response.Stdout, "script dispatch complete")
	}
	if response.Stderr != "" {
		t.Fatalf("SCRIPT_RESPONSE stderr = %q, want empty", response.Stderr)
	}
	if response.DurationMillis < 0 {
		t.Fatalf("SCRIPT_RESPONSE durationMillis = %d, want non-negative", response.DurationMillis)
	}
	if stringPointerValue(events[requestIndex].Context.DispatchId) != dispatchID {
		t.Fatalf("SCRIPT_REQUEST context.dispatchId = %q, want %q", stringPointerValue(events[requestIndex].Context.DispatchId), dispatchID)
	}
	if stringPointerValue(events[responseIndex].Context.DispatchId) != dispatchID {
		t.Fatalf("SCRIPT_RESPONSE context.dispatchId = %q, want %q", stringPointerValue(events[responseIndex].Context.DispatchId), dispatchID)
	}
	if !functionalEventContextHasWorkID(events[requestIndex], workID) || !functionalEventContextHasWorkID(events[responseIndex], workID) {
		t.Fatalf("script event work IDs = request:%#v response:%#v, want %q", events[requestIndex].Context.WorkIds, events[responseIndex].Context.WorkIds, workID)
	}

	assertFunctionalScriptEventDoesNotLeak(t, events[requestIndex], []string{`"stdin"`, `"env"`})
	assertFunctionalScriptEventDoesNotLeak(t, events[responseIndex], []string{`"stdin"`, `"env"`})
}

func assertThinEventRawDispatchPayloadIsThin(t *testing.T, event map[string]any) {
	t.Helper()

	for _, path := range []string{
		"payload.metadata.requestId",
		"payload.model",
		"payload.provider",
		"payload.promptFile",
		"payload.promptTemplate",
		"payload.outputSchema",
		"payload.worktree",
		"payload.workingDirectory",
		"payload.workerType",
		"payload.workstationName",
		"payload.workstationType",
		"payload.stdin",
		"payload.env",
	} {
		assertThinEventRawPathAbsent(t, event, path)
	}
}

func rawThinEventArtifactEvents(t *testing.T, events []factoryapi.FactoryEvent) []map[string]any {
	t.Helper()

	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal raw thin-event artifact: %v", err)
	}

	var raw []map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("unmarshal raw thin-event artifact: %v", err)
	}
	return raw
}

func assertThinEventRawPathAbsent(t *testing.T, event map[string]any, path string) {
	t.Helper()

	if value, ok := thinEventRawPath(event, path); ok {
		t.Fatalf("event %s (%s) unexpectedly included %s=%#v", rawThinEventID(t, event), rawThinEventType(t, event), path, value)
	}
}

func requireThinEventRawPath(t *testing.T, event map[string]any, path string) any {
	t.Helper()

	value, ok := thinEventRawPath(event, path)
	if !ok {
		t.Fatalf("event %s (%s) is missing required %s", rawThinEventID(t, event), rawThinEventType(t, event), path)
	}
	return value
}

func thinEventRawPath(event map[string]any, path string) (any, bool) {
	var current any = event
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func rawThinEventType(t *testing.T, event map[string]any) string {
	t.Helper()

	value, ok := event["type"].(string)
	if !ok || value == "" {
		t.Fatalf("raw event type = %#v, want non-empty string", event["type"])
	}
	return value
}

func rawThinEventID(t *testing.T, event map[string]any) string {
	t.Helper()

	value, ok := event["id"].(string)
	if !ok || value == "" {
		t.Fatalf("raw event id = %#v, want non-empty string", event["id"])
	}
	return value
}

func assertThinEventReconstructedModelReader(t *testing.T, smoke dualDispatchSmokeFixture, worldState interfaces.FactoryWorldState) {
	t.Helper()

	dispatchID := thinEventDispatchIDForWork(t, smoke.artifact.Events, smoke.modelWorkID)
	completion := thinEventCompletedDispatchForID(t, worldState.CompletedDispatches, dispatchID)
	if completion.Result.Outcome != string(interfaces.OutcomeAccepted) {
		t.Fatalf("model dispatch outcome = %q, want ACCEPTED", completion.Result.Outcome)
	}
	if len(completion.InputWorkItems) != 1 || completion.InputWorkItems[0].ID != smoke.modelWorkID {
		t.Fatalf("model input work items = %#v, want %q", completion.InputWorkItems, smoke.modelWorkID)
	}
	if len(completion.OutputWorkItems) != 1 || completion.OutputWorkItems[0].WorkTypeID != "task" {
		t.Fatalf("model output work items = %#v, want one terminal task output", completion.OutputWorkItems)
	}
	if len(completion.TraceIDs) != 1 || completion.TraceIDs[0] != smoke.traceID {
		t.Fatalf("model completion trace IDs = %#v, want [%q]", completion.TraceIDs, smoke.traceID)
	}

	attempts := worldState.InferenceAttemptsByDispatchID[dispatchID]
	if len(attempts) != 1 {
		t.Fatalf("model inference attempts = %#v, want one attempt", attempts)
	}
	for _, attempt := range attempts {
		if attempt.InferenceRequestID == "" || attempt.Response == "" || attempt.ResponseTime.IsZero() {
			t.Fatalf("model attempt = %#v, want request ID, response text, and response time", attempt)
		}
		if attempt.DispatchID != dispatchID {
			t.Fatalf("model attempt dispatch = %q, want %q", attempt.DispatchID, dispatchID)
		}
	}
}

func assertThinEventReconstructedScriptReader(t *testing.T, smoke dualDispatchSmokeFixture, worldState interfaces.FactoryWorldState) {
	t.Helper()

	dispatchID := thinEventDispatchIDForWork(t, smoke.artifact.Events, smoke.scriptWorkID)
	completion := thinEventCompletedDispatchForID(t, worldState.CompletedDispatches, dispatchID)
	if completion.Result.Outcome != string(interfaces.OutcomeAccepted) {
		t.Fatalf("script dispatch outcome = %q, want ACCEPTED", completion.Result.Outcome)
	}
	if len(completion.InputWorkItems) != 1 || completion.InputWorkItems[0].ID != smoke.scriptWorkID {
		t.Fatalf("script input work items = %#v, want %q", completion.InputWorkItems, smoke.scriptWorkID)
	}
	if len(completion.TraceIDs) != 1 || completion.TraceIDs[0] != smoke.traceID {
		t.Fatalf("script completion trace IDs = %#v, want [%q]", completion.TraceIDs, smoke.traceID)
	}

	requests := worldState.ScriptRequestsByDispatchID[dispatchID]
	if len(requests) != 1 {
		t.Fatalf("script requests = %#v, want one request", requests)
	}
	var request interfaces.FactoryWorldScriptRequest
	for _, candidate := range requests {
		request = candidate
	}
	if request.Command != "script-tool" || len(request.Args) != 2 || request.Args[0] != "--mode" || request.Args[1] != "smoke" {
		t.Fatalf("script request = %#v, want script-tool [--mode smoke]", request)
	}
	if request.RequestTime.IsZero() {
		t.Fatalf("script request time is zero: %#v", request)
	}

	responses := worldState.ScriptResponsesByDispatchID[dispatchID]
	if len(responses) != 1 {
		t.Fatalf("script responses = %#v, want one response", responses)
	}
	var response interfaces.FactoryWorldScriptResponse
	for _, candidate := range responses {
		response = candidate
	}
	if response.ScriptRequestID != request.ScriptRequestID {
		t.Fatalf("script response request ID = %q, want %q", response.ScriptRequestID, request.ScriptRequestID)
	}
	if response.Outcome != string(factoryapi.ScriptExecutionOutcomeSucceeded) ||
		response.Stdout != "script dispatch complete" ||
		response.Stderr != "" ||
		response.ExitCode == nil || *response.ExitCode != 0 ||
		response.ResponseTime.IsZero() {
		t.Fatalf("script response = %#v, want succeeded stdout/stderr/exit_code details", response)
	}
}

func assertThinEventWorkstationRequestProjection(t *testing.T, smoke dualDispatchSmokeFixture, worldState interfaces.FactoryWorldState) {
	t.Helper()

	projection := factoryboundary.BuildFactoryWorldWorkstationRequestProjectionSlice(worldState)
	if projection.WorkstationRequestsByDispatchId == nil {
		t.Fatal("workstation request projection missing request map")
	}
	requests := *projection.WorkstationRequestsByDispatchId

	modelDispatchID := thinEventDispatchIDForWork(t, smoke.artifact.Events, smoke.modelWorkID)
	model := requests[modelDispatchID]
	if model.Request.RequestTime == nil || model.Response == nil || model.Response.ResponseText == nil {
		t.Fatalf("model workstation request projection = %#v, want request time and response text", model)
	}
	if *model.Response.ResponseText == "" {
		t.Fatalf("model response text = %q, want non-empty", *model.Response.ResponseText)
	}

	scriptDispatchID := thinEventDispatchIDForWork(t, smoke.artifact.Events, smoke.scriptWorkID)
	script := requests[scriptDispatchID]
	if script.Request.ScriptRequest == nil || script.Request.ScriptRequest.Command == nil || *script.Request.ScriptRequest.Command != "script-tool" {
		t.Fatalf("script workstation request = %#v, want script request command", script.Request.ScriptRequest)
	}
	if script.Response == nil || script.Response.ScriptResponse == nil {
		t.Fatalf("script workstation response = %#v, want script response", script.Response)
	}
	if script.Response.ScriptResponse.Outcome == nil || *script.Response.ScriptResponse.Outcome != string(factoryapi.ScriptExecutionOutcomeSucceeded) {
		t.Fatalf("script workstation outcome = %#v, want SUCCEEDED", script.Response.ScriptResponse)
	}
	if script.Response.ScriptResponse.ExitCode == nil || *script.Response.ScriptResponse.ExitCode != 0 {
		t.Fatalf("script workstation exit code = %#v, want 0", script.Response.ScriptResponse.ExitCode)
	}
	if script.Request.TraceIds == nil || len(*script.Request.TraceIds) != 1 || (*script.Request.TraceIds)[0] != smoke.traceID {
		t.Fatalf("script workstation trace IDs = %#v, want [%q]", script.Request.TraceIds, smoke.traceID)
	}
}

func thinEventDispatchIDForWork(t *testing.T, events []factoryapi.FactoryEvent, workID string) string {
	t.Helper()

	index := requireThinEventDispatchRequestIndexForWork(t, events, workID)
	return thinEventDispatchIDFromEvent(t, events[index], workID)
}

func thinEventDispatchIDFromEvent(t *testing.T, event factoryapi.FactoryEvent, workID string) string {
	t.Helper()

	dispatchID := stringPointerValue(event.Context.DispatchId)
	if dispatchID == "" {
		t.Fatalf("DISPATCH_REQUEST context.dispatchId is empty for work %q", workID)
	}
	return dispatchID
}

func thinEventCompletedDispatchForID(
	t *testing.T,
	completions []interfaces.FactoryWorldDispatchCompletion,
	dispatchID string,
) interfaces.FactoryWorldDispatchCompletion {
	t.Helper()

	for _, completion := range completions {
		if completion.DispatchID == dispatchID {
			return completion
		}
	}
	t.Fatalf("completed dispatches = %#v, want dispatch %q", completions, dispatchID)
	return interfaces.FactoryWorldDispatchCompletion{}
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
			t.Fatalf("live event %s (%s) missing from recorded artifact events: %#v", live.Id, live.Type, replayContractEventTypes(artifact.Events))
		}
		if recorded.Type != live.Type || recorded.Context.Tick != live.Context.Tick {
			t.Fatalf("recorded event %s = type %s tick %d, live type %s tick %d", live.Id, recorded.Type, recorded.Context.Tick, live.Type, live.Context.Tick)
		}
		if stringPointerValue(recorded.Context.DispatchId) != stringPointerValue(live.Context.DispatchId) {
			t.Fatalf("recorded event %s dispatch id = %q, live dispatch id = %q", live.Id, stringPointerValue(recorded.Context.DispatchId), stringPointerValue(live.Context.DispatchId))
		}
		if strings.Join(stringSlicePointerValue(recorded.Context.WorkIds), ",") != strings.Join(stringSlicePointerValue(live.Context.WorkIds), ",") {
			t.Fatalf("recorded event %s work ids = %#v, live work ids = %#v", live.Id, stringSlicePointerValue(recorded.Context.WorkIds), stringSlicePointerValue(live.Context.WorkIds))
		}
	}
}

func assertReplayInferenceRequest(
	t *testing.T,
	event factoryapi.FactoryEvent,
	dispatchID string,
	attempt int,
) factoryapi.InferenceRequestEventPayload {
	t.Helper()

	request, err := event.Payload.AsInferenceRequestEventPayload()
	if err != nil {
		t.Fatalf("decode inference-request payload: %v", err)
	}
	if stringPointerValue(event.Context.DispatchId) != dispatchID || request.Attempt != attempt {
		t.Fatalf("inference request correlation = %#v, want dispatch=%s attempt=%d", request, dispatchID, attempt)
	}
	if request.InferenceRequestId == "" || request.Prompt == "" {
		t.Fatalf("inference request missing request ID or prompt: %#v", request)
	}
	return request
}

func assertReplayInferenceResponse(
	t *testing.T,
	event factoryapi.FactoryEvent,
	dispatchID string,
	requestID string,
	attempt int,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	response, err := event.Payload.AsInferenceResponseEventPayload()
	if err != nil {
		t.Fatalf("decode inference-response payload: %v", err)
	}
	if stringPointerValue(event.Context.DispatchId) != dispatchID ||
		response.InferenceRequestId != requestID || response.Attempt != attempt {
		t.Fatalf("inference response correlation = %#v, want dispatch=%s request=%s attempt=%d", response, dispatchID, requestID, attempt)
	}
	if response.DurationMillis < 0 {
		t.Fatalf("durationMillis = %d, want non-negative", response.DurationMillis)
	}
	return response
}
