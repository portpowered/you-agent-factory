//go:build functionallong

package replay_contracts

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReplayWorkDispatchContractSmoke_CanonicalWorkRequestPreservesPayload(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay work-dispatch canonical request sweep")

	req, dir := runWorkDispatchContractSmoke(t, dispatchContractScenario{
		commandOutput: "canonical dispatch output",
		submit: func(server *support.FunctionalAPIServer) {
			support.UpsertDefaultSessionWorkRequest(t, server.URL(), work.WorkRequest{
				RequestID: "request-dispatch-smoke-001",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works: []work.Work{{
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
		workingDirectory:         support.ResolvedRuntimePath(dir, "/tmp/factory-struct-cleanup"),
		payloadTitle:             "canonical dispatch contract",
	})
}

func TestReplayWorkDispatchContractSmoke_LegacySubmitRequestAdapterPreservesPayload(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay work-dispatch legacy adapter sweep")

	var submitted factoryapi.SubmitWorkResponse
	req, dir := runWorkDispatchContractSmoke(t, dispatchContractScenario{
		commandOutput: "legacy dispatch output",
		submit: func(server *support.FunctionalAPIServer) {
			traceID := "trace-legacy-smoke-001"
			tags := factoryapi.StringMap{
				"branch": "legacy-adapter",
				"team":   "agent-factory",
			}
			submitted = support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
				Name:         "legacy-dispatch-smoke",
				WorkTypeName: "task",
				TraceId:      &traceID,
				Payload:      map[string]any{"title": "legacy dispatch contract"},
				Tags:         &tags,
			})
		},
	})
	assertCommandWorkDispatch(t, req, expectedDispatchPayload{
		requestID:                submitted.RequestId,
		workID:                   stringPointerValue(submitted.WorkId),
		workTypeID:               "task",
		traceID:                  "trace-legacy-smoke-001",
		currentChainingTraceID:   "trace-legacy-smoke-001",
		previousChainingTraceIDs: []string{"trace-legacy-smoke-001"},
		workName:                 "legacy-dispatch-smoke",
		branch:                   "legacy-adapter",
		team:                     "agent-factory",
		workingDirectory:         support.ResolvedRuntimePath(dir, "/tmp/legacy-adapter"),
		payloadTitle:             "legacy dispatch contract",
	})
}

func TestReplayWorkDispatchContractSmoke_RecordReplayKeepsSplitContractCorrelation(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay work-dispatch record/replay correlation sweep")

	run := runRecordedWorkDispatchContractSmoke(t, dispatchContractScenario{
		commandOutput: "recorded dispatch output",
		submit: func(server *support.FunctionalAPIServer) {
			support.UpsertDefaultSessionWorkRequest(t, server.URL(), work.WorkRequest{
				RequestID: "request-recorded-smoke-001",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works: []work.Work{{
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
		workingDirectory:         support.ResolvedRuntimePath(run.dir, "/tmp/record-replay"),
		payloadTitle:             "recorded dispatch contract",
	}

	assertCommandWorkDispatch(t, run.request, want)
	indices := requireScriptResponseEventIndices(t, run.events)
	assertDispatchSmokeEventCorrelation(t, run.events, indices, run.request, want, run.commandOutput)
	recordedEvents := testutil.GeneratedFactoryEvents(t, run.artifact.Events)
	assertDispatchSmokeEventsRecordedInArtifact(t, run.events, recordedEvents)
	assertScriptEventsRecordedInArtifact(t, run.events, recordedEvents)

	replay := observeReplayThroughRoot(t, run.artifactPath, 10*time.Second)
	assertReplayPlaceCounts(t, replay.Work, map[string]int{
		"task:done": 1, "task:init": 0, "task:failed": 0,
	})
}

type dispatchContractScenario struct {
	commandOutput string
	submit        func(*support.FunctionalAPIServer)
}

type recordedDispatchContractRun struct {
	request       platformprocess.CommandRequest
	dir           string
	artifactPath  string
	commandOutput string
	events        []factoryapi.FactoryEvent
	artifact      *interfaces.ReplayArtifact
}

func runWorkDispatchContractSmoke(t *testing.T, scenario dispatchContractScenario) (platformprocess.CommandRequest, string) {
	t.Helper()

	run := runRecordedWorkDispatchContractSmoke(t, scenario)
	return run.request, run.dir
}

func runRecordedWorkDispatchContractSmoke(t *testing.T, scenario dispatchContractScenario) recordedDispatchContractRun {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	artifactPath := filepath.Join(t.TempDir(), "work-dispatch-contract-smoke.replay.json")
	support.SetWorkingDirectory(t, dir)
	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstation["workingDirectory"] = `/tmp/{{ index (index .Inputs 0).Tags "branch" }}`
		workstation["env"] = map[string]any{
			"BRANCH": `{{ index (index .Inputs 0).Tags "branch" }}`,
			"TEAM":   `{{ index (index .Inputs 0).Tags "team" }}`,
		}
	})

	runner := support.NewRecordingCommandRunner(scenario.commandOutput)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ScriptCommandRunner: runner,
		},
	})
	scenario.submit(server)
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	assertReplayPlaceCounts(t, support.GetDefaultSession(t, server.URL()), map[string]int{
		"task:done": 1, "task:init": 0, "task:failed": 0,
	})
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("script command runner calls = %d, want 1", got)
	}
	events := server.GetFactoryEvents(t)
	server.Stop(t)

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

func assertCommandWorkDispatch(t *testing.T, req platformprocess.CommandRequest, want expectedDispatchPayload) {
	t.Helper()

	assertCommandRequestEnvelope(t, req, want)
	assertCommandEnvironment(t, req, want)
}

func assertCommandRequestEnvelope(t *testing.T, req platformprocess.CommandRequest, want expectedDispatchPayload) {
	t.Helper()

	if req.Command != "echo" {
		t.Fatalf("command = %q, want echo", req.Command)
	}
	if req.WorkDir != want.workingDirectory {
		t.Fatalf("command work dir = %q, want %q", req.WorkDir, want.workingDirectory)
	}
}

func assertCommandEnvironment(t *testing.T, req platformprocess.CommandRequest, want expectedDispatchPayload) {
	t.Helper()

	if !commandEnvContains(req.Env, "BRANCH="+want.branch) {
		t.Fatalf("command env missing BRANCH=%s in %v", want.branch, req.Env)
	}
	if !commandEnvContains(req.Env, "TEAM="+want.team) {
		t.Fatalf("command env missing TEAM=%s in %v", want.team, req.Env)
	}
}

func assertDispatchSmokeEventCorrelation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	indices scriptBoundaryEventIndices,
	req platformprocess.CommandRequest,
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

	dispatchID := stringPointerValue(events[indices.dispatch].Context.DispatchId)
	if dispatchID == "" {
		t.Fatal("dispatch event context dispatchId is empty")
	}
	if got := stringPointerValue(events[indices.dispatch].Context.RequestId); got != want.requestID {
		t.Fatalf("dispatch request context requestId = %q, want %q", got, want.requestID)
	}
	for _, idx := range []int{indices.dispatch, indices.request, indices.response, indices.completed} {
		assertDispatchSmokeEventContext(t, events[idx], want, dispatchID)
	}

	if dispatchRequest.TransitionId != "run-script" {
		t.Fatalf("dispatch request transitionId = %q, want run-script", dispatchRequest.TransitionId)
	}
	if dispatchResponse.TransitionId != "run-script" {
		t.Fatalf("dispatch response transitionId = %q, want run-script", dispatchResponse.TransitionId)
	}
	if scriptRequest.DispatchId != dispatchID || scriptResponse.DispatchId != dispatchID {
		t.Fatalf("script event dispatch correlation mismatch: request=%q response=%q want=%q", scriptRequest.DispatchId, scriptResponse.DispatchId, dispatchID)
	}
	if scriptRequest.TransitionId != "run-script" || scriptResponse.TransitionId != "run-script" {
		t.Fatalf("script event transition correlation mismatch: request=%q response=%q want=run-script", scriptRequest.TransitionId, scriptResponse.TransitionId)
	}
	if scriptRequest.Command != req.Command {
		t.Fatalf("script request command = %q, want %q", scriptRequest.Command, req.Command)
	}
	if !equalStringSlices(scriptRequest.Args, req.Args) {
		t.Fatalf("script request args = %#v, want %#v", scriptRequest.Args, req.Args)
	}
	if scriptResponse.ScriptRequestId != scriptRequest.ScriptRequestId {
		t.Fatalf("script response request ID = %q, want %q", scriptResponse.ScriptRequestId, scriptRequest.ScriptRequestId)
	}
	if normalizeReplayContractStdout(scriptResponse.Stdout, true) != wantOutput {
		t.Fatalf("script response stdout = %q, want %q", normalizeReplayContractStdout(scriptResponse.Stdout, true), wantOutput)
	}
	if dispatchResponse.Output == nil || normalizeReplayContractStdout(*dispatchResponse.Output, true) != wantOutput {
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

	if got := stringPointerValue(event.Context.DispatchId); got != wantDispatchID {
		t.Fatalf("%s context dispatchId = %q, want %q", event.Type, got, wantDispatchID)
	}
	if event.Context.RequestId != nil {
		if got := stringPointerValue(event.Context.RequestId); got != want.requestID {
			t.Fatalf("%s context requestId = %q, want %q", event.Type, got, want.requestID)
		}
	}
	if got := event.Context.TraceIds; got == nil || len(*got) != 1 || (*got)[0] != want.traceID {
		t.Fatalf("%s context traceIds = %#v, want [%s]", event.Type, got, want.traceID)
	}
	if got := event.Context.WorkIds; got == nil || len(*got) != 1 || (*got)[0] != want.workID {
		t.Fatalf("%s context workIds = %#v, want [%s]", event.Type, got, want.workID)
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

	if got := stringPointerValue(current); got != want.currentChainingTraceID {
		t.Fatalf("%s current chaining trace ID = %q, want %q", label, got, want.currentChainingTraceID)
	}
	if previous == nil || !equalStringSlices(*previous, want.previousChainingTraceIDs) {
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
			t.Fatalf("recorded artifact missing dispatch event %s from live history; artifact events=%v", live.Id, replayContractEventTypes(recordedEvents))
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
		var recordedValue, liveValue any
		if err := json.Unmarshal(recordedJSON, &recordedValue); err != nil {
			t.Fatalf("decode recorded dispatch event %s: %v", live.Id, err)
		}
		if err := json.Unmarshal(liveJSON, &liveValue); err != nil {
			t.Fatalf("decode live dispatch event %s: %v", live.Id, err)
		}
		if !reflect.DeepEqual(recordedValue, liveValue) {
			t.Fatalf("recorded dispatch event %s does not match live history\nrecorded=%s\nlive=%s", live.Id, recordedJSON, liveJSON)
		}
	}
}

func equalStringSlices(left, right []string) bool {
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
