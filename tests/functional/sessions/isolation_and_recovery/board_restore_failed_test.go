package isolation_and_recovery_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The three shutdown/join/reopen steps serialize local ~default ownership;
// independent test journeys use separate process fixtures and run in parallel.
func TestBoardRestoreReproducesFailedAndEscalatedStates(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	dir := support.ScaffoldFactory(t, boardRestoreFailedConfig())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
	for _, name := range []string{"review", "plan"} {
		support.WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	}
	apis := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	var starts atomic.Int32
	runner := &boardRestoreStartFailedRunner{}
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return apis[starts.Add(1)-1].Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	var before map[string]boardRestoreWorkState
	var beforeEvents []factoryapi.FactoryEvent
	for generation, api := range apis {
		inputs := restartProbeInputs(t, dir)
		command := support.StartProcessCommand(t, process, inputs.Input)
		url := restartProbeReadyURL(t, api, command)
		if generation == 0 {
			putPlainBoardBatch(t, url, "failed-board", []byte(`{"requestId":"failed-board","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"task-1","name":"task","workTypeName":"task","payload":"synthetic review"},{"workId":"review-1","name":"review","workTypeName":"review","payload":"synthetic review"},{"workId":"idea-1","name":"idea","workTypeName":"idea","payload":"synthetic plan"},{"workId":"thoughts-1","name":"thoughts","workTypeName":"thoughts","payload":"synthetic dependent"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"thoughts","targetWorkName":"idea","requiredState":"complete"}]}`))
		}
		support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
			return status.TotalTokens == 4 && status.Categories.Failed == 4
		})
		board := readBoardRestoreStates(t, url)
		want := map[string]boardRestoreWorkState{
			"task-1": {"task", "escalated", "FAILED"}, "review-1": {"review", "fin", "FAILED"},
			"idea-1": {"idea", "failed", "FAILED"}, "thoughts-1": {"thoughts", "failed", "FAILED"},
		}
		if !reflect.DeepEqual(board, want) {
			t.Fatalf("generation %d complete board = %#v, want %#v", generation, board, want)
		}
		events := support.GetFactoryEventsForSessionAt(t, url, "~default")
		assertBoardRestoreFailureFacts(t, events)
		if generation == 0 {
			before, beforeEvents = board, events
		} else {
			if !reflect.DeepEqual(before, board) {
				t.Fatalf("restore changed complete board: before=%v after=%v", before, board)
			}
			assertRestartProbeEventFacts(t, beforeEvents, events)
		}
		restartProbeShutdown(t, url, command)
		if runner.calls.Load() != 2 {
			t.Fatalf("generation %d provider attempts=%d, want two terminal start failures and no redispatch", generation, runner.calls.Load())
		}
	}
}

// ROOT1 candidate: an idea reaches to-complete before its dependency fails.
// This protects that recovery shape; it does not reproduce the incident error.
// The same board's stop/reopen steps serialize ~default ownership.
func TestBoardRestoreReproducesFailedAndEscalatedStatesAfterIdeaToComplete(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	repo, home := t.TempDir(), t.TempDir()
	dir := filepath.Join(repo, "factory")
	if err := os.Rename(support.ScaffoldFactory(t, boardRestorePlanThenFailConfig()), dir); err != nil {
		t.Fatal(err)
	}
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
	for _, name := range []string{"review", "plan"} {
		support.WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	}
	apis := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	var starts atomic.Int32
	runner := &boardRestorePlanThenFailRunner{}
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return apis[starts.Add(1)-1].Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	var beforeEvents []factoryapi.FactoryEvent
	want := map[string]boardRestoreWorkState{
		"task-1": {"task", "escalated", "FAILED"}, "review-1": {"review", "fin", "FAILED"}, "idea-1": {"idea", "failed", "FAILED"},
	}
	var recordingPath string
	for generation, api := range apis {
		inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--continuously", "--with-server"})
		inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
		inputs.Input.WorkingDirectory = repo
		if generation == 2 {
			inputs.Input.Args = append(inputs.Input.Args, "--record", recordingPath)
		}
		command := support.StartProcessCommand(t, process, inputs.Input)
		url := restartProbeReadyURL(t, api, command)
		if generation == 0 {
			putPlainBoardBatch(t, url, "failed-join-board", []byte(`{"requestId":"failed-join-board","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"idea-1","name":"idea","workTypeName":"idea","state":"waiting","payload":"synthetic plan"},{"workId":"task-1","name":"task","workTypeName":"task","state":"complete","payload":"synthetic task"},{"workId":"review-1","name":"review","workTypeName":"review","payload":"synthetic review"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"idea","targetWorkName":"task","requiredState":"complete"}]}`))
			restartProbePost(t, support.SessionWorkURL(url, "~default", "/work/idea-1/move"), []byte(`{"stateName":"init"}`))
			support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
				return readBoardRestoreStates(t, url)["idea-1"].State == "to-complete"
			})
			restartProbeShutdown(t, url, command)
			var reference struct{ ArtifactReference string }
			if err := json.Unmarshal(mustReadSeededReplayArtifact(t, filepath.Join(repo, ".you-agent-factory", "current-board.json")), &reference); err != nil || reference.ArtifactReference == "" {
				t.Fatalf("current board reference=%+v, err=%v", reference, err)
			}
			recordingPath = reference.ArtifactReference
			continue
		}
		if generation == 1 {
			restartProbePost(t, support.SessionWorkURL(url, "~default", "/work/task-1/move"), []byte(`{"stateName":"init"}`))
		}
		support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
			return status.TotalTokens == 3 && status.Categories.Failed == 3
		})
		if board := readBoardRestoreStates(t, url); !reflect.DeepEqual(board, want) {
			t.Fatalf("generation %d board=%v, want %v", generation, board, want)
		}
		events := support.GetFactoryEventsForSessionAt(t, url, "~default")
		assertBoardRestoreIdeaCascade(t, events, "idea-1", "task-1")
		if generation == 1 {
			beforeEvents = events
		} else {
			assertRestartProbeEventFacts(t, beforeEvents, events)
		}
		restartProbeShutdown(t, url, command)
		if runner.calls.Load() != 2 {
			t.Fatalf("generation %d dispatched %d times, want successful plan then failed task and no redispatch", generation, runner.calls.Load())
		}
	}
}

func boardRestorePlanThenFailConfig() map[string]any {
	config := boardRestoreFailedConfig()
	for _, definition := range config["workTypes"].([]map[string]any) {
		if definition["name"] == "idea" {
			definition["states"] = append(definition["states"].([]map[string]string), map[string]string{"name": "to-complete", "type": "PROCESSING"}, map[string]string{"name": "waiting", "type": "PROCESSING"})
		}
	}
	for _, station := range config["workstations"].([]map[string]any) {
		if station["name"] == "plan" {
			station["outputs"] = []map[string]string{{"workType": "idea", "state": "to-complete"}}
		}
	}
	return config
}

func assertBoardRestoreIdeaCascade(t *testing.T, events []factoryapi.FactoryEvent, workID, triggerWorkID string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkStateChange {
			continue
		}
		payload, err := event.Payload.AsWorkStateChangeEventPayload()
		if err == nil && payload.WorkId == workID && payload.FromState == "to-complete" && payload.ToState == "failed" && payload.TriggerWorkId != nil && *payload.TriggerWorkId == triggerWorkID && string(payload.Source) == "cascading-failure" {
			return
		}
	}
	t.Fatalf("missing public idea to-complete -> failed cascade caused by %s", triggerWorkID)
}

type boardRestorePlanThenFailRunner struct{ calls atomic.Int32 }

func (runner *boardRestorePlanThenFailRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if runner.calls.Add(1) == 1 {
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(`{"output":"synthetic completed plan"}`)}, nil
	}
	return platformprocess.CommandResult{}, &platformprocess.CommandStartError{
		Command: "codex", CommandLineLength: 32932, CommandLineLimit: platformprocess.WindowsCommandLineLimit,
		Cause: errors.New("controlled command line too long"),
	}
}

type boardRestoreStartFailedRunner struct{ calls atomic.Int32 }

func (runner *boardRestoreStartFailedRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return platformprocess.CommandResult{}, &platformprocess.CommandStartError{
		Command: "codex", CommandLineLength: 32932, CommandLineLimit: platformprocess.WindowsCommandLineLimit,
		Cause: errors.New("controlled command line too long"),
	}
}

type boardRestoreWorkState struct{ WorkType, State, Category string }

func readBoardRestoreStates(t *testing.T, url string) map[string]boardRestoreWorkState {
	t.Helper()
	board := make(map[string]boardRestoreWorkState)
	for _, item := range support.ListDefaultSessionWork(t, url).Results {
		if item.WorkId == nil || item.State == nil || item.WorkTypeName == nil {
			t.Fatalf("Work missing identity or state: %#v", item)
		}
		if _, exists := board[*item.WorkId]; exists {
			t.Fatalf("duplicate Work %s", *item.WorkId)
		}
		board[*item.WorkId] = boardRestoreWorkState{*item.WorkTypeName, item.State.Name, string(item.State.Type)}
	}
	return board
}

func assertBoardRestoreFailureFacts(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	responses, cascades := 0, 0
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			responses++
			assertBoardRestoreStartFailedFact(t, event)
		case factoryapi.FactoryEventTypeDispatchInterrupted:
			t.Fatal("terminal start failure was left open for restart reconciliation")
		case factoryapi.FactoryEventTypeWorkStateChange:
			assertBoardRestoreCascadeFact(t, event)
			cascades++
		}
	}
	if responses != 2 || cascades != 1 {
		t.Fatalf("responses/cascades=%d/%d, want 2/1", responses, cascades)
	}
}

func assertBoardRestoreStartFailedFact(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil || string(payload.Outcome) != "FAILED" || payload.FailureDetail == nil || string(payload.FailureDetail.Reason) != "command_line_too_long" || event.Context.WorkIds == nil {
		t.Fatalf("terminal start failure fact = %#v, err=%v", payload, err)
	}
	assertBoardRestoreFailureOutput(t, payload, *event.Context.WorkIds)
}

func assertBoardRestoreCascadeFact(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	payload, err := event.Payload.AsWorkStateChangeEventPayload()
	if err != nil || payload.WorkId != "thoughts-1" || payload.TriggerWorkId == nil || *payload.TriggerWorkId != "idea-1" || string(payload.Source) != "cascading-failure" || payload.ToState != "failed" {
		t.Fatalf("cascade fact = %#v, err=%v", payload, err)
	}
}

func assertBoardRestoreFailureOutput(t *testing.T, payload factoryapi.DispatchResponseEventPayload, inputIDs []string) {
	t.Helper()
	want := map[string]string{"idea-1": "failed"}
	if payload.TransitionId == "review" {
		want = map[string]string{"task-1": "escalated", "review-1": "fin"}
	}
	if payload.OutputWork == nil || len(*payload.OutputWork) != len(want) || len(inputIDs) != len(want) {
		t.Fatalf("failure lost input/output identities: inputs=%v output=%#v", inputIDs, payload.OutputWork)
	}
	for _, id := range inputIDs {
		if want[id] == "" {
			t.Fatalf("unexpected failed dispatch input %s", id)
		}
	}
	for _, item := range *payload.OutputWork {
		if item.WorkId == nil || item.State == nil || want[*item.WorkId] != item.State.Name || string(item.State.Type) != "FAILED" {
			t.Fatalf("failure output lost authored routing: id=%v state=%+v", item.WorkId, item.State)
		}
	}
}

func boardRestoreFailedConfig() map[string]any {
	types := []map[string]any{}
	for name, failed := range map[string]string{"task": "escalated", "review": "fin", "idea": "failed", "thoughts": "failed"} {
		types = append(types, map[string]any{"name": name, "states": []map[string]string{
			{"name": "init", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"}, {"name": failed, "type": "FAILED"},
		}})
	}
	return map[string]any{
		"name": "failed-board", "workTypes": types, "workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{
			{"name": "review", "worker": "worker-a", "inputs": []map[string]string{{"workType": "task", "state": "init"}, {"workType": "review", "state": "init"}}, "outputs": []map[string]string{{"workType": "task", "state": "complete"}, {"workType": "review", "state": "complete"}}, "onFailure": []map[string]string{{"workType": "task", "state": "escalated"}, {"workType": "review", "state": "fin"}}},
			{"name": "plan", "worker": "worker-a", "inputs": []map[string]string{{"workType": "idea", "state": "init"}}, "outputs": []map[string]string{{"workType": "idea", "state": "complete"}}, "onFailure": []map[string]string{{"workType": "idea", "state": "failed"}}},
		},
	}
}

// The generated child and its parent's unconsumed join belong to the same
// board, so shutdown/reopen is ordered; independent boards remain parallel.
func TestBoardRestoreReproducesFailedAndEscalatedStatesGeneratedChild(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	repo, home := t.TempDir(), t.TempDir()
	dir := filepath.Join(repo, "factory")
	prepareGeneratedFailureFactory(t, dir)
	apis := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	var starts atomic.Int32
	runner := &boardRestoreGeneratedChildRunner{}
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return apis[starts.Add(1)-1].Start(ctx, request)
		}})
	support.CleanupProcess(t, process)
	var before map[string]boardRestoreWorkState
	var events []factoryapi.FactoryEvent
	var recordingPath string
	var taskID, ideaID string
	for generation, api := range apis {
		inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--continuously", "--with-server"})
		inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
		inputs.Input.WorkingDirectory = repo
		if generation == 2 {
			inputs.Input.Args = append(inputs.Input.Args, "--record", recordingPath)
		}
		command := support.StartProcessCommand(t, process, inputs.Input)
		url := restartProbeReadyURL(t, api, command)
		if generation == 2 {
			if board := readBoardRestoreStates(t, url); !reflect.DeepEqual(board, before) {
				t.Fatalf("explicit record restart lost latest failures: board=%v, want %v", board, before)
			}
		}
		if generation == 0 {
			ideaID, taskID = seedGeneratedFailureBoard(t, url)
		} else {
			if generation == 1 {
				restartProbePost(t, support.SessionWorkURL(url, "~default", "/work/"+taskID+"/move"), []byte(`{"stateName":"init"}`))
			}
			support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
				return status.TotalTokens == 4 && status.Categories.Failed == 3
			})
			assertBoardRestoreIdeaCascade(t, support.GetFactoryEventsForSessionAt(t, url, "~default"), ideaID, taskID)
			if generation == 1 {
				before, events = readBoardRestoreStates(t, url), support.GetFactoryEventsForSessionAt(t, url, "~default")
				assertGracefulFailureTickReset(t, events, ideaID)
			} else {
				if board := readBoardRestoreStates(t, url); !reflect.DeepEqual(board, before) {
					t.Fatalf("restored board=%v, want %v", board, before)
				}
				assertRestartProbeEventFacts(t, events, support.GetFactoryEventsForSessionAt(t, url, "~default"))
			}
		}
		restartProbeShutdown(t, url, command)
		var reference struct{ ArtifactReference string }
		if err := json.Unmarshal(mustReadSeededReplayArtifact(t, filepath.Join(repo, ".you-agent-factory", "current-board.json")), &reference); err != nil {
			t.Fatal(err)
		}
		recordingPath = reference.ArtifactReference
	}
	if runner.calls.Load() != 2 {
		t.Fatalf("provider calls=%d, want planning plus failure without redispatch", runner.calls.Load())
	}
}

func prepareGeneratedFailureFactory(t *testing.T, dir string) {
	t.Helper()
	config := boardRestorePlanThenFailConfig()
	for _, definition := range config["workTypes"].([]map[string]any) {
		if definition["name"] == "task" {
			definition["states"] = append(definition["states"].([]map[string]string), map[string]string{"name": "waiting", "type": "PROCESSING"})
		}
	}
	if err := os.Rename(support.ScaffoldFactory(t, config), dir); err != nil {
		t.Fatal(err)
	}
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
	for _, name := range []string{"review", "plan"} {
		support.WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	}
}

func seedGeneratedFailureBoard(t *testing.T, url string) (string, string) {
	t.Helper()
	putPlainBoardBatch(t, url, "generated-child", []byte(`{"requestId":"generated-child","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"idea-1","name":"idea","workTypeName":"idea","state":"waiting","payload":"synthetic plan"},{"workId":"review-1","name":"review","workTypeName":"review","payload":"synthetic review"}]}`))
	// Advance predecessor history before planning so its tick exceeds the
	// successor's failure tick. Each move is acknowledged through public Work.
	for cycle := 0; cycle < 12; cycle++ {
		for _, state := range []string{"complete", "init"} {
			restartProbePost(t, support.SessionWorkURL(url, "~default", "/work/review-1/move"), []byte(`{"stateName":"`+state+`"}`))
			support.WaitForStatus(t, url, 15*time.Second, func(factoryapi.StatusResponse) bool { return readBoardRestoreStates(t, url)["review-1"].State == state })
		}
	}
	restartProbePost(t, support.SessionWorkURL(url, "~default", "/work/idea-1/move"), []byte(`{"stateName":"init"}`))
	var ideaID, taskID string
	support.WaitForStatus(t, url, 15*time.Second, func(factoryapi.StatusResponse) bool {
		board := readBoardRestoreStates(t, url)
		for id, item := range board {
			if item.WorkType == "idea" && id != "idea-1" && item.State == "to-complete" {
				ideaID = id
			}
			if item.WorkType == "task" && item.State == "waiting" {
				taskID = id
			}
		}
		return len(board) == 4 && board["idea-1"].State == "to-complete" && taskID != "" && ideaID != ""
	})
	return ideaID, taskID
}

func assertGracefulFailureTickReset(t *testing.T, events []factoryapi.FactoryEvent, ideaID string) {
	t.Helper()
	starts, predecessorTick := 0, 0
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeSessionResumed {
			t.Fatal("graceful reopen unexpectedly emitted a resume marker")
		}
		if event.Type == factoryapi.FactoryEventTypeSessionStarted {
			starts++
		}
		if starts < 2 && event.Context.Tick > predecessorTick {
			predecessorTick = event.Context.Tick
		}
		if starts == 2 && event.Type == factoryapi.FactoryEventTypeWorkStateChange {
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err == nil && payload.WorkId == ideaID && payload.ToState == "failed" && event.Context.Tick < predecessorTick {
				return
			}
		}
	}
	t.Fatal("missing public failure after graceful restart with a lower logical tick")
}

type boardRestoreGeneratedChildRunner struct{ calls atomic.Int32 }

func (runner *boardRestoreGeneratedChildRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if runner.calls.Add(1) == 1 {
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(`{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"workId":"task-1","name":"task","workTypeName":"task","state":"waiting","payload":"synthetic task"},{"name":"generated-idea","workTypeName":"idea","state":"to-complete","payload":"synthetic generated plan"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"generated-idea","targetWorkName":"task","requiredState":"complete"}]}}`)}, nil
	}
	return platformprocess.CommandResult{}, &platformprocess.CommandStartError{
		Command: "codex", CommandLineLength: 32932, CommandLineLimit: platformprocess.WindowsCommandLineLimit,
		Cause: errors.New("controlled command line too long"),
	}
}
