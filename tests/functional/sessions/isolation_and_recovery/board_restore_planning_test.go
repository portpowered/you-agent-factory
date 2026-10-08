package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Generations serialize the customer-owned local ~default board. The distinct
// provider edge proves live planning before failure; one graph serves every
// plain and resume generation, with isolated home, files and server handles.
func TestDaemonRestartProbeRestoresFailureAfterPlanning(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	repo, home := t.TempDir(), t.TempDir()
	dir := filepath.Join(repo, "factory")
	if err := os.Rename(support.ScaffoldFactory(t, boardRestorePlanningConfig()), dir); err != nil {
		t.Fatal(err)
	}
	support.WriteAgentConfig(t, dir, "planner", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
	support.WriteAgentConfig(t, dir, "script", "---\ntype: SCRIPT_WORKER\ncommand: synthetic-script\n---\n")
	for _, name := range []string{"plan", "script"} {
		support.WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	}
	support.WriteWorkstationConfig(t, dir, "finish", "---\ntype: LOGICAL_MOVE\n---\n")
	apis := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	var starts atomic.Int32
	var protectedPath atomic.Value
	protectedPath.Store("")
	var protectedWrites atomic.Int32
	planner := &boardRestorePlannerRunner{entered: make(chan struct{}), release: make(chan struct{})}
	close(planner.release)
	script := &boardRestoreScriptRunner{}
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: planner, ScriptCommandRunner: script,
		RecordingWriteFile: func(path string, data []byte) error {
			if path == protectedPath.Load().(string) {
				protectedWrites.Add(1)
			}
			return os.WriteFile(path, data, 0600)
		},
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return apis[starts.Add(1)-1].Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	var before map[string]boardRestoreWorkState
	var beforeEvents []factoryapi.FactoryEvent
	sourceCopy := filepath.Join(repo, "same-source-resume.json")
	for generation, api := range apis {
		t.Logf("starting board generation %d", generation)
		inputs := plainBoardFailureInputs(t, repo, home, 24300+generation, "--dir", dir)
		if generation == 3 {
			inputs.Input.Args = append(inputs.Input.Args, "--resume", sourceCopy)
		}
		var sourceBytes []byte
		if generation > 0 {
			source := plainBoardSelectedRecording(t, repo)
			if generation == 3 {
				source = sourceCopy
			}
			protectedPath.Store(source)
			sourceBytes = mustReadSeededReplayArtifact(t, source)
		}
		command := support.StartProcessCommand(t, process, inputs.Input)
		url := restartProbeReadyURL(t, api, command)
		if generation == 0 {
			putPlainBoardBatch(t, url, "plan-then-fail", []byte(`{"requestId":"plan-then-fail","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"idea-1","name":"idea","workTypeName":"idea","payload":"private-synthetic-sentinel"}]}`))
			support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
				return status.TotalTokens == 3 && status.Categories.Terminal == 2
			})
			assertBoardRestorePlanningStates(t, readBoardRestoreStates(t, url))
			restartProbeShutdown(t, url, command)
			assertBoardRestorePlanningCalls(t, 3, planner, script)
			continue
		}
		if generation == 1 {
			// Fail in a successor generation whose logical tick restarted. The
			// latest sequence must win over the predecessor's to-complete tick.
			assertBoardRestorePlanningStates(t, readBoardRestoreStates(t, url))
			restartProbePost(t, support.SessionWorkURL(url, "~default", "/work/idea-1/move"), []byte(`{"stateName":"failed"}`))
		}
		support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
			return status.TotalTokens == 3 && status.Categories.Terminal == 2 && status.Categories.Failed == 1
		})
		board, events := assertPlainPlanningFailureGeneration(t, url, generation, before, beforeEvents)
		if generation == 1 {
			before, beforeEvents = board, events
		}
		restartProbeShutdown(t, url, command)
		if generation == 1 {
			writeRestartProbeFile(t, sourceCopy, mustReadSeededReplayArtifact(t, plainBoardSelectedRecording(t, repo)))
		}
		assertPlanningSourceProtected(t, sourceBytes, &protectedPath, &protectedWrites)
		assertBoardRestorePlanningCalls(t, 3, planner, script)
	}
	protectedPath.Store(plainBoardSelectedRecording(t, repo))
	protectedBytes := mustReadSeededReplayArtifact(t, protectedPath.Load().(string))
	assertPlainPlanningRestoreRejection(t, process, repo, home, dir)
	assertPlanningSourceProtected(t, protectedBytes, &protectedPath, &protectedWrites)
}

func assertPlanningSourceProtected(t *testing.T, sourceBytes []byte, source *atomic.Value, writes *atomic.Int32) {
	t.Helper()
	if writes.Load() != 0 || !bytes.Equal(sourceBytes, mustReadSeededReplayArtifact(t, source.Load().(string))) {
		t.Fatal("restore wrote its protected source")
	}
}

func assertPlainPlanningFailureGeneration(t *testing.T, url string, generation int, before map[string]boardRestoreWorkState, beforeEvents []factoryapi.FactoryEvent) (map[string]boardRestoreWorkState, []factoryapi.FactoryEvent) {
	t.Helper()
	board := readBoardRestoreStates(t, url)
	if board["idea-1"] != (boardRestoreWorkState{"idea", "failed", "FAILED"}) {
		t.Fatalf("generation %d lost latest failure: %v", generation, board)
	}
	events := support.GetFactoryEventsForSessionAt(t, url, "~default")
	if generation > 1 {
		if !reflect.DeepEqual(board, before) {
			t.Fatalf("generation %d changed Work IDs or states: before=%v after=%v", generation, before, board)
		}
		assertRestartProbeEventFacts(t, beforeEvents, events)
	}
	return board, events
}

func assertPlainPlanningRestoreRejection(t *testing.T, process support.Process, repo, home, dir string) {
	t.Helper()
	source := plainBoardSelectedRecording(t, repo)
	before := mustReadSeededReplayArtifact(t, source)
	refPath := filepath.Join(repo, ".you-agent-factory", "current-board.json")
	reference := mustReadSeededReplayArtifact(t, refPath)
	path := filepath.Join(dir, "factory.json")
	definition := bytes.ReplaceAll(mustReadSeededReplayArtifact(t, path), []byte("failed"), []byte("replacement"))
	if err := os.WriteFile(path, definition, 0600); err != nil {
		t.Fatal(err)
	}
	for _, debug := range []bool{false, true} {
		inputs := plainBoardFailureInputs(t, repo, home, 24310, "--dir", dir)
		if debug {
			inputs.Input.Args = append(inputs.Input.Args, "--debug")
		}
		err := process.Execute(inputs.Input)
		var diagnostic interface{ CLIErrorCode() string }
		if err == nil || !errors.As(err, &diagnostic) {
			t.Fatalf("restore conflict did not return coded error: %v", err)
		}
		output := inputs.Stderr() + inputs.Stdout()
		for _, want := range []string{"idea-1", "idea:failed", "event sequence:", "WORK_STATE_CHANGE"} {
			if !strings.Contains(output, want) {
				t.Fatalf("restore diagnostic lacks %q: %s", want, output)
			}
		}
		if strings.Contains(output, "private-synthetic-sentinel") {
			t.Fatal("restore diagnostic leaked payload")
		}
		if !bytes.Equal(before, mustReadSeededReplayArtifact(t, source)) || !bytes.Equal(reference, mustReadSeededReplayArtifact(t, refPath)) {
			t.Fatal("rejected restore mutated recording or reference")
		}
	}
}

func TestBoardRestoreReproducesFailedAndEscalatedStatesSecondRestorePlanning(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	dir := support.ScaffoldFactory(t, boardRestorePlanningConfig())
	support.WriteAgentConfig(t, dir, "planner", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
	support.WriteAgentConfig(t, dir, "script", "---\ntype: SCRIPT_WORKER\ncommand: synthetic-script\n---\n")
	for _, name := range []string{"plan", "script"} {
		support.WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	}
	support.WriteWorkstationConfig(t, dir, "finish", "---\ntype: LOGICAL_MOVE\n---\n")
	apis := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	var starts atomic.Int32
	planner := &boardRestorePlannerRunner{entered: make(chan struct{}), release: make(chan struct{})}
	script := &boardRestoreScriptRunner{}
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: planner, ScriptCommandRunner: script,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return apis[starts.Add(1)-1].Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	var completed map[string]boardRestoreWorkState
	var completedEvents []factoryapi.FactoryEvent
	for generation, api := range apis {
		inputs := restartProbeInputs(t, dir)
		command := support.StartProcessCommand(t, process, inputs.Input)
		url := restartProbeReadyURL(t, api, command)
		switch generation {
		case 0:
			if board := readBoardRestoreStates(t, url); len(board) != 0 {
				t.Fatalf("fresh empty board=%v", board)
			}
		case 1:
			if board := readBoardRestoreStates(t, url); len(board) != 0 {
				t.Fatalf("restored empty board=%v", board)
			}
			putPlainBoardBatch(t, url, "pending-plan", []byte(`{"requestId":"pending-plan","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"idea-1","name":"idea","workTypeName":"idea","state":"waiting","payload":"synthetic plan"}]}`))
			if board := readBoardRestoreStates(t, url); !reflect.DeepEqual(board, map[string]boardRestoreWorkState{"idea-1": {"idea", "waiting", "PROCESSING"}}) {
				t.Fatalf("initial quiescent board=%v", board)
			}
		case 2:
			completeBoardRestorePlan(t, url, planner)
			completed = readBoardRestoreStates(t, url)
			assertBoardRestorePlanningStates(t, completed)
			completedEvents = support.GetFactoryEventsForSessionAt(t, url, "~default")
		case 3:
			board := readBoardRestoreStates(t, url)
			if !reflect.DeepEqual(board, completed) {
				t.Fatalf("second restore changed complete board/IDs: before=%v after=%v", completed, board)
			}
			assertBoardRestorePlanningStates(t, board)
			assertRestartProbeEventFacts(t, completedEvents, support.GetFactoryEventsForSessionAt(t, url, "~default"))
		}
		restartProbeShutdown(t, url, command)
		assertBoardRestorePlanningCalls(t, generation, planner, script)
	}
}

func assertBoardRestorePlanningCalls(t *testing.T, generation int, planner *boardRestorePlannerRunner, script *boardRestoreScriptRunner) {
	t.Helper()
	if generation < 2 && (planner.calls.Load() != 0 || script.calls.Load() != 0) {
		t.Fatal("quiescent board dispatched work")
	}
	if generation >= 2 && (planner.calls.Load() != 1 || script.calls.Load() != 2) {
		t.Fatalf("plan/script calls=%d/%d, want 1/2 with no redispatch", planner.calls.Load(), script.calls.Load())
	}
}

func completeBoardRestorePlan(t *testing.T, url string, planner *boardRestorePlannerRunner) {
	t.Helper()
	// Release planning only after the first restored board is publicly ready.
	if board := readBoardRestoreStates(t, url); !reflect.DeepEqual(board, map[string]boardRestoreWorkState{"idea-1": {"idea", "waiting", "PROCESSING"}}) {
		t.Fatalf("first restored board=%v", board)
	}
	restartProbePost(t, support.SessionWorkURL(url, "~default", "/work/idea-1/move"), []byte(`{"stateName":"init"}`))
	select {
	case <-planner.entered:
	case <-time.After(15 * time.Second):
		t.Fatal("post-restore plan did not reach controlled provider")
	}
	close(planner.release)
	support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.TotalTokens == 3 && status.Categories.Terminal == 2
	})
}

type boardRestorePlannerRunner struct {
	calls            atomic.Int32
	entered, release chan struct{}
}

func (runner *boardRestorePlannerRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if runner.calls.Add(1) == 1 {
		close(runner.entered)
	}
	select {
	case <-runner.release:
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(`{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"generated-a","workTypeName":"task","payload":"script a"},{"name":"generated-b","workTypeName":"task","payload":"script b"}]}}`)}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

type boardRestoreScriptRunner struct{ calls atomic.Int32 }

func (runner *boardRestoreScriptRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return platformprocess.CommandResult{Stdout: []byte("script complete")}, nil
}

func assertBoardRestorePlanningStates(t *testing.T, board map[string]boardRestoreWorkState) {
	t.Helper()
	if len(board) != 3 || board["idea-1"] != (boardRestoreWorkState{"idea", "to-complete", "PROCESSING"}) {
		t.Fatalf("plan board=%v, want retained idea and two generated tasks", board)
	}
	for id, state := range board {
		if id != "idea-1" && state != (boardRestoreWorkState{"task", "complete", "TERMINAL"}) {
			t.Fatalf("generated task %s=%v", id, state)
		}
	}
}

func boardRestorePlanningConfig() map[string]any {
	return map[string]any{
		"name": "planning-board",
		"workTypes": []map[string]any{
			{"name": "idea", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "waiting", "type": "PROCESSING"}, {"name": "to-complete", "type": "PROCESSING"}, {"name": "failed", "type": "FAILED"}}},
			{"name": "task", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "scripted", "type": "PROCESSING"}, {"name": "complete", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}}},
		},
		"workers": []map[string]string{{"name": "planner"}, {"name": "script"}},
		"workstations": []map[string]any{
			{"name": "plan", "worker": "planner", "inputs": []map[string]string{{"workType": "idea", "state": "init"}}, "outputs": []map[string]string{{"workType": "idea", "state": "to-complete"}}, "onFailure": []map[string]string{{"workType": "idea", "state": "failed"}}},
			{"name": "script", "worker": "script", "inputs": []map[string]string{{"workType": "task", "state": "init"}}, "outputs": []map[string]string{{"workType": "task", "state": "scripted"}}, "onFailure": []map[string]string{{"workType": "task", "state": "failed"}}},
			{"name": "finish", "type": "LOGICAL_MOVE", "inputs": []map[string]string{{"workType": "task", "state": "scripted"}}, "outputs": []map[string]string{{"workType": "task", "state": "complete"}}},
		},
	}
}
