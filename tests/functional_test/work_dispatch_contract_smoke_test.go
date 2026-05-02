package functional_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestWorkDispatchContractSmoke_CanonicalWorkRequestPreservesPayload(t *testing.T) {
	req, dir := runWorkDispatchContractSmoke(t, dispatchContractScenario{
		commandOutput: "canonical dispatch output",
		submit: func(harness *testutil.ServiceTestHarness) {
			harness.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
				RequestID: "request-dispatch-smoke-001",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works: []interfaces.Work{{
					Name:       "canonical-dispatch-smoke",
					WorkID:     "work-dispatch-smoke-001",
					WorkTypeID: "task",
					TraceID:    "trace-dispatch-smoke-001",
					Payload:    map[string]any{"title": "canonical dispatch contract"},
					Tags:       map[string]string{"branch": "factory-struct-cleanup", "team": "agent-factory"},
				}},
			})
		},
	})
	assertCommandWorkDispatch(t, req, expectedDispatchPayload{
		requestID:                "request-dispatch-smoke-001",
		workID:                   "work-dispatch-smoke-001",
		workTypeID:               "task",
		traceID:                  "trace-dispatch-smoke-001",
		currentChainingTraceID:   "trace-dispatch-smoke-001",
		previousChainingTraceIDs: []string{"trace-dispatch-smoke-001"},
		workName:                 "canonical-dispatch-smoke",
		branch:                   "factory-struct-cleanup",
		team:                     "agent-factory",
		workingDirectory:         resolvedRuntimePath(dir, "/tmp/factory-struct-cleanup"),
		payloadTitle:             "canonical dispatch contract",
	})
}

func TestWorkDispatchContractSmoke_LegacySubmitRequestAdapterPreservesPayload(t *testing.T) {
	req, dir := runWorkDispatchContractSmoke(t, dispatchContractScenario{
		commandOutput: "legacy dispatch output",
		submit: func(harness *testutil.ServiceTestHarness) {
			harness.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
				RequestID:  "request-legacy-smoke-001",
				Name:       "legacy-dispatch-smoke",
				WorkID:     "work-legacy-smoke-001",
				WorkTypeID: "task",
				TraceID:    "trace-legacy-smoke-001",
				Payload:    []byte(`{"title":"legacy dispatch contract"}`),
				Tags: map[string]string{
					"branch": "legacy-adapter",
					"team":   "agent-factory",
				},
			}})
		},
	})
	assertCommandWorkDispatch(t, req, expectedDispatchPayload{
		requestID:                "request-legacy-smoke-001",
		workID:                   "work-legacy-smoke-001",
		workTypeID:               "task",
		traceID:                  "trace-legacy-smoke-001",
		currentChainingTraceID:   "trace-legacy-smoke-001",
		previousChainingTraceIDs: []string{"trace-legacy-smoke-001"},
		workName:                 "legacy-dispatch-smoke",
		branch:                   "legacy-adapter",
		team:                     "agent-factory",
		workingDirectory:         resolvedRuntimePath(dir, "/tmp/legacy-adapter"),
		payloadTitle:             "legacy dispatch contract",
	})
}

func TestWorkDispatchContractSmoke_RecordReplayKeepsSplitContractCorrelation(t *testing.T) {
	run := runRecordedWorkDispatchContractSmoke(t, dispatchContractScenario{
		commandOutput: "recorded dispatch output",
		submit: func(harness *testutil.ServiceTestHarness) {
			harness.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
				RequestID: "request-recorded-smoke-001",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works: []interfaces.Work{{
					Name:       "recorded-dispatch-smoke",
					WorkID:     "work-recorded-smoke-001",
					WorkTypeID: "task",
					TraceID:    "trace-recorded-smoke-001",
					Payload:    map[string]any{"title": "recorded dispatch contract"},
					Tags:       map[string]string{"branch": "record-replay", "team": "agent-factory"},
				}},
			})
		},
	})
	want := expectedDispatchPayload{
		requestID:                "request-recorded-smoke-001",
		workID:                   "work-recorded-smoke-001",
		workTypeID:               "task",
		traceID:                  "trace-recorded-smoke-001",
		currentChainingTraceID:   "trace-recorded-smoke-001",
		previousChainingTraceIDs: []string{"trace-recorded-smoke-001"},
		workName:                 "recorded-dispatch-smoke",
		branch:                   "record-replay",
		team:                     "agent-factory",
		workingDirectory:         resolvedRuntimePath(run.dir, "/tmp/record-replay"),
		payloadTitle:             "recorded dispatch contract",
	}

	assertCommandWorkDispatch(t, run.request, want)
	indices := requireScriptResponseEventIndices(t, run.events)
	assertDispatchSmokeEventCorrelation(t, run.events, indices, run.request, want, run.commandOutput)
	assertDispatchSmokeEventsRecordedInArtifact(t, run.events, run.artifact.Events)
	assertScriptEventsRecordedInArtifact(t, run.events, run.artifact.Events)

	replayHarness := testutil.AssertReplaySucceeds(t, run.artifactPath, 10*time.Second)
	replayHarness.Service.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

type dispatchContractScenario struct {
	commandOutput string
	submit        func(*testutil.ServiceTestHarness)
}

type recordedDispatchContractRun struct {
	request       workers.CommandRequest
	dir           string
	artifactPath  string
	commandOutput string
	events        []factoryapi.FactoryEvent
	artifact      *interfaces.ReplayArtifact
}

func runWorkDispatchContractSmoke(t *testing.T, scenario dispatchContractScenario) (workers.CommandRequest, string) {
	t.Helper()

	run := runRecordedWorkDispatchContractSmoke(t, scenario)
	return run.request, run.dir
}

func runRecordedWorkDispatchContractSmoke(t *testing.T, scenario dispatchContractScenario) recordedDispatchContractRun {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
	artifactPath := filepath.Join(t.TempDir(), "work-dispatch-contract-smoke.replay.json")
	setWorkingDirectory(t, dir)
	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstation["workingDirectory"] = `/tmp/{{ index (index .Inputs 0).Tags "branch" }}`
		workstation["env"] = map[string]any{
			"BRANCH": `{{ index (index .Inputs 0).Tags "branch" }}`,
			"TEAM":   `{{ index (index .Inputs 0).Tags "team" }}`,
		}
	})

	runner := newRecordingCommandRunner(scenario.commandOutput)
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
		testutil.WithRecordPath(artifactPath),
	)
	scenario.submit(harness)
	harness.RunUntilComplete(t, 10*time.Second)
	harness.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("script command runner calls = %d, want 1", got)
	}
	events, err := harness.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}

	return recordedDispatchContractRun{
		request:       runner.LastRequest(),
		dir:           dir,
		artifactPath:  artifactPath,
		commandOutput: scenario.commandOutput,
		events:        events,
		artifact:      testutil.LoadReplayArtifact(t, artifactPath),
	}
}

type expectedDispatchPayload struct {
	requestID                string
	workID                   string
	workTypeID               string
	traceID                  string
	currentChainingTraceID   string
	previousChainingTraceIDs []string
	workName                 string
	branch                   string
	team                     string
	workingDirectory         string
	payloadTitle             string
}

func assertCommandWorkDispatch(t *testing.T, req workers.CommandRequest, want expectedDispatchPayload) {
	t.Helper()

	assertCommandRequestEnvelope(t, req, want)
	assertCommandExecutionMetadata(t, req, want)
	assertCommandEnvironment(t, req, want)
	assertCommandInputToken(t, req, want)
}

func assertCommandRequestEnvelope(t *testing.T, req workers.CommandRequest, want expectedDispatchPayload) {
	t.Helper()

	if req.Command != "echo" {
		t.Fatalf("command = %q, want echo", req.Command)
	}
	if req.WorkDir != want.workingDirectory {
		t.Fatalf("command work dir = %q, want %q", req.WorkDir, want.workingDirectory)
	}
	if req.DispatchID == "" {
		t.Fatal("dispatch ID is empty")
	}
	if req.TransitionID != "run-script" {
		t.Fatalf("transition ID = %q, want run-script", req.TransitionID)
	}
	if req.WorkerType != "script-worker" {
		t.Fatalf("worker type = %q, want script-worker", req.WorkerType)
	}
	if req.WorkstationName != "run-script" {
		t.Fatalf("workstation name = %q, want run-script", req.WorkstationName)
	}
	if req.CurrentChainingTraceID != want.currentChainingTraceID {
		t.Fatalf("current chaining trace ID = %q, want %q", req.CurrentChainingTraceID, want.currentChainingTraceID)
	}
	if len(req.PreviousChainingTraceIDs) != len(want.previousChainingTraceIDs) {
		t.Fatalf("previous chaining trace IDs = %v, want %v", req.PreviousChainingTraceIDs, want.previousChainingTraceIDs)
	}
	for i := range want.previousChainingTraceIDs {
		if req.PreviousChainingTraceIDs[i] != want.previousChainingTraceIDs[i] {
			t.Fatalf("previous chaining trace IDs = %v, want %v", req.PreviousChainingTraceIDs, want.previousChainingTraceIDs)
		}
	}
}

func assertCommandExecutionMetadata(t *testing.T, req workers.CommandRequest, want expectedDispatchPayload) {
	t.Helper()

	if req.Execution.DispatchCreatedTick == 0 {
		t.Fatal("dispatch created tick is zero")
	}
	if req.Execution.CurrentTick != req.Execution.DispatchCreatedTick {
		t.Fatalf("current tick = %d, want dispatch created tick %d", req.Execution.CurrentTick, req.Execution.DispatchCreatedTick)
	}
	if req.Execution.RequestID != want.requestID {
		t.Fatalf("execution request ID = %q, want %q", req.Execution.RequestID, want.requestID)
	}
	if req.Execution.TraceID != want.traceID {
		t.Fatalf("execution trace ID = %q, want %q", req.Execution.TraceID, want.traceID)
	}
	if len(req.Execution.WorkIDs) != 1 || req.Execution.WorkIDs[0] != want.workID {
		t.Fatalf("execution work IDs = %v, want [%s]", req.Execution.WorkIDs, want.workID)
	}
	if req.Execution.ReplayKey == "" {
		t.Fatal("execution replay key is empty")
	}
}

func assertCommandEnvironment(t *testing.T, req workers.CommandRequest, want expectedDispatchPayload) {
	t.Helper()

	if !containsEnv(req.Env, "BRANCH="+want.branch) {
		t.Fatalf("command env missing BRANCH=%s in %v", want.branch, req.Env)
	}
	if !containsEnv(req.Env, "TEAM="+want.team) {
		t.Fatalf("command env missing TEAM=%s in %v", want.team, req.Env)
	}
}

func assertCommandInputToken(t *testing.T, req workers.CommandRequest, want expectedDispatchPayload) {
	t.Helper()

	token := firstCommandRequestInputToken(t, req)
	if token.PlaceID != "task:init" {
		t.Fatalf("input token place ID = %q, want task:init", token.PlaceID)
	}
	if token.Color.RequestID != want.requestID {
		t.Fatalf("input token request ID = %q, want %q", token.Color.RequestID, want.requestID)
	}
	if token.Color.WorkID != want.workID {
		t.Fatalf("input token work ID = %q, want %q", token.Color.WorkID, want.workID)
	}
	if token.Color.WorkTypeID != want.workTypeID {
		t.Fatalf("input token work type ID = %q, want %q", token.Color.WorkTypeID, want.workTypeID)
	}
	if token.Color.TraceID != want.traceID {
		t.Fatalf("input token trace ID = %q, want %q", token.Color.TraceID, want.traceID)
	}
	if token.Color.Name != want.workName {
		t.Fatalf("input token name = %q, want %q", token.Color.Name, want.workName)
	}
	if token.Color.Tags["branch"] != want.branch {
		t.Fatalf("input token tag branch = %q, want %q", token.Color.Tags["branch"], want.branch)
	}
	if token.Color.Tags["team"] != want.team {
		t.Fatalf("input token tag team = %q, want %q", token.Color.Tags["team"], want.team)
	}

	var payload map[string]string
	if err := json.Unmarshal(token.Color.Payload, &payload); err != nil {
		t.Fatalf("input token payload is not JSON object: %v", err)
	}
	if payload["title"] != want.payloadTitle {
		t.Fatalf("input token payload title = %q, want %q", payload["title"], want.payloadTitle)
	}
}

func firstCommandRequestInputToken(t *testing.T, req workers.CommandRequest) interfaces.Token {
	t.Helper()

	tokens := workers.CommandRequestInputTokens(req)
	if len(tokens) == 0 {
		t.Fatal("command request has no input tokens")
	}
	for _, token := range tokens {
		if token.Color.DataType != interfaces.DataTypeResource {
			return token
		}
	}
	t.Fatal("command request has no work input token")
	return interfaces.Token{}
}

func assertDispatchSmokeEventCorrelation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	indices scriptBoundaryEventIndices,
	req workers.CommandRequest,
	want expectedDispatchPayload,
	wantOutput string,
) {
	t.Helper()

	dispatchRequest, err := events[indices.dispatch].Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("decode dispatch request payload: %v", err)
	}
	scriptRequest, err := events[indices.request].Payload.AsScriptRequestEventPayload()
	if err != nil {
		t.Fatalf("decode script request payload: %v", err)
	}
	scriptResponse, err := events[indices.response].Payload.AsScriptResponseEventPayload()
	if err != nil {
		t.Fatalf("decode script response payload: %v", err)
	}
	dispatchResponse, err := events[indices.completed].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode dispatch response payload: %v", err)
	}

	dispatchID := stringValueForFunctionalTest(events[indices.dispatch].Context.DispatchId)
	if dispatchID != req.DispatchID {
		t.Fatalf("dispatch event context dispatchId = %q, want command request dispatchId %q", dispatchID, req.DispatchID)
	}
	if got := stringValueForFunctionalTest(events[indices.dispatch].Context.RequestId); got != want.requestID {
		t.Fatalf("dispatch request context requestId = %q, want %q", got, want.requestID)
	}
	for _, idx := range []int{indices.dispatch, indices.request, indices.response, indices.completed} {
		assertDispatchSmokeEventContext(t, events[idx], want, dispatchID)
	}

	if dispatchRequest.TransitionId != req.TransitionID {
		t.Fatalf("dispatch request transitionId = %q, want %q", dispatchRequest.TransitionId, req.TransitionID)
	}
	if dispatchResponse.TransitionId != req.TransitionID {
		t.Fatalf("dispatch response transitionId = %q, want %q", dispatchResponse.TransitionId, req.TransitionID)
	}
	if scriptRequest.DispatchId != dispatchID || scriptResponse.DispatchId != dispatchID {
		t.Fatalf("script event dispatch correlation mismatch: request=%q response=%q want=%q", scriptRequest.DispatchId, scriptResponse.DispatchId, dispatchID)
	}
	if scriptRequest.TransitionId != req.TransitionID || scriptResponse.TransitionId != req.TransitionID {
		t.Fatalf("script event transition correlation mismatch: request=%q response=%q want=%q", scriptRequest.TransitionId, scriptResponse.TransitionId, req.TransitionID)
	}
	if scriptRequest.Command != req.Command {
		t.Fatalf("script request command = %q, want %q", scriptRequest.Command, req.Command)
	}
	if !equalStringSlicesFunctionalTest(scriptRequest.Args, req.Args) {
		t.Fatalf("script request args = %#v, want %#v", scriptRequest.Args, req.Args)
	}
	if scriptResponse.ScriptRequestId != scriptRequest.ScriptRequestId {
		t.Fatalf("script response request ID = %q, want %q", scriptResponse.ScriptRequestId, scriptRequest.ScriptRequestId)
	}
	if normalizeFunctionalStdout(scriptResponse.Stdout, true) != wantOutput {
		t.Fatalf("script response stdout = %q, want %q", normalizeFunctionalStdout(scriptResponse.Stdout, true), wantOutput)
	}
	if dispatchResponse.Output == nil || normalizeFunctionalStdout(*dispatchResponse.Output, true) != wantOutput {
		t.Fatalf("dispatch response output = %#v, want %q", dispatchResponse.Output, wantOutput)
	}

	assertDispatchSmokeChaining(t, dispatchRequest.CurrentChainingTraceId, dispatchRequest.PreviousChainingTraceIds, want, "dispatch request")
	assertDispatchSmokeChaining(t, dispatchResponse.CurrentChainingTraceId, dispatchResponse.PreviousChainingTraceIds, want, "dispatch response")
	if len(dispatchRequest.Inputs) != 1 || dispatchRequest.Inputs[0].WorkId != want.workID {
		t.Fatalf("dispatch request inputs = %#v, want work %q", dispatchRequest.Inputs, want.workID)
	}
}

func assertDispatchSmokeEventContext(
	t *testing.T,
	event factoryapi.FactoryEvent,
	want expectedDispatchPayload,
	wantDispatchID string,
) {
	t.Helper()

	if got := stringValueForFunctionalTest(event.Context.DispatchId); got != wantDispatchID {
		t.Fatalf("%s context dispatchId = %q, want %q", event.Type, got, wantDispatchID)
	}
	if event.Context.RequestId != nil {
		if got := stringValueForFunctionalTest(event.Context.RequestId); got != want.requestID {
			t.Fatalf("%s context requestId = %q, want %q", event.Type, got, want.requestID)
		}
	}
	if got := event.Context.TraceIds; got == nil || len(*got) != 1 || (*got)[0] != want.traceID {
		t.Fatalf("%s context traceIds = %#v, want [%s]", event.Type, got, want.traceID)
	}
	if got := event.Context.WorkIds; got == nil || len(*got) != 1 || (*got)[0] != want.workID {
		t.Fatalf("%s context workIds = %#v, want [%s]", event.Type, got, want.workID)
	}
	if event.Type == factoryapi.FactoryEventTypeDispatchRequest || event.Type == factoryapi.FactoryEventTypeDispatchResponse {
		assertDispatchSmokeChaining(t, event.Context.CurrentChainingTraceId, event.Context.PreviousChainingTraceIds, want, string(event.Type)+" context")
	}
}

func assertDispatchSmokeChaining(
	t *testing.T,
	current *string,
	previous *[]string,
	want expectedDispatchPayload,
	label string,
) {
	t.Helper()

	if got := stringValueForFunctionalTest(current); got != want.currentChainingTraceID {
		t.Fatalf("%s current chaining trace ID = %q, want %q", label, got, want.currentChainingTraceID)
	}
	if previous == nil || !equalStringSlicesFunctionalTest(*previous, want.previousChainingTraceIDs) {
		t.Fatalf("%s previous chaining trace IDs = %#v, want %v", label, previous, want.previousChainingTraceIDs)
	}
}

func assertDispatchSmokeEventsRecordedInArtifact(
	t *testing.T,
	liveEvents []factoryapi.FactoryEvent,
	recordedEvents []factoryapi.FactoryEvent,
) {
	t.Helper()

	recordedByID := make(map[string]factoryapi.FactoryEvent, len(recordedEvents))
	for _, event := range recordedEvents {
		recordedByID[event.Id] = event
	}

	for _, live := range liveEvents {
		if live.Type != factoryapi.FactoryEventTypeDispatchRequest && live.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		recorded, ok := recordedByID[live.Id]
		if !ok {
			t.Fatalf("recorded artifact missing dispatch event %s from live history; artifact events=%v", live.Id, functionalEventTypes(recordedEvents))
		}
		if recorded.Type != live.Type {
			t.Fatalf("recorded dispatch event %s = type %s, live type %s", live.Id, recorded.Type, live.Type)
		}

		liveJSON, err := json.Marshal(live)
		if err != nil {
			t.Fatalf("marshal live dispatch event %s: %v", live.Id, err)
		}
		recordedJSON, err := json.Marshal(recorded)
		if err != nil {
			t.Fatalf("marshal recorded dispatch event %s: %v", live.Id, err)
		}
		if string(recordedJSON) != string(liveJSON) {
			t.Fatalf("recorded dispatch event %s does not match live history\nrecorded=%s\nlive=%s", live.Id, recordedJSON, liveJSON)
		}
	}
}

func equalStringSlicesFunctionalTest(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
