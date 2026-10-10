package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// R1-R6 share one process with immutable listener and provider edge routing.
// HTTP reads prove the session's retained Work and reconnect cursor contract;
// execution and failure admission enter through Process.Execute.
func TestAuthoredDefaultRecordReplayJourney(t *testing.T) {
	t.Parallel()
	apis := map[int]*support.ProcessAPIServer{18001: support.NewProcessAPIServer(), 18002: support.NewProcessAPIServer()}
	peerDir := authoredReplayFactory(t)
	gate := make(chan struct{})
	t.Cleanup(func() { closeUnlessReleased(gate) })
	runner := &authoredReplayRunner{peerDir: peerDir, gate: gate, started: make(chan struct{}), delegate: support.NewRecordingCommandRunner("<result>ACCEPTED</result>")}
	effects := newComposedRecordingEffects()
	edges := effects.edges(nil, runner)
	edges.APIServerStarter = func(ctx context.Context, request platformhttpserver.StartRequest) error {
		api := apis[request.Port]
		if api == nil {
			return fmt.Errorf("unexpected test listener selection %d", request.Port)
		}
		return api.Start(ctx, request)
	}
	process := support.BuildProcess(t, edges)
	for _, cell := range []struct {
		name     string
		defaults bool
		port     int
	}{{"authored-defaults", true, 18001}, {"no-input-control", false, 18002}} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			dir := authoredReplayFactory(t)
			if !cell.defaults {
				if err := os.RemoveAll(filepath.Join(dir, "inputs")); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(t.TempDir(), "authored-default.json")
			live := authoredReplayInput(t, dir, "--session", uuid.NewString(), "--work", testutil.MustRepoPath(t, "docs/examples/startup-work.json"), "--record", path)
			if err := process.Execute(live.Input); err != nil {
				t.Fatalf("record: %v; stderr=%s", err, live.Stderr())
			}
			wantWork := 1
			if cell.defaults {
				wantWork = 2
			}
			before := mustReadFile(t, path)
			calls := runner.callsIn(dir)
			id := uuid.NewString()
			replay := authoredReplayInput(t, dir, "--session", id, "--replay", path, "--no-record", "--continuously", "--with-server", "--listen", fmt.Sprintf("127.0.0.1:%d", cell.port))
			command := support.StartProcessCommand(t, process, replay.Input)
			url := apis[cell.port].WaitForURL(t)
			support.WaitForSessionTerminalStatus(t, url, id, 15*time.Second)
			assertAuthoredReplaySession(t, url, id, wantWork)
			if runner.callsIn(dir) != calls {
				t.Fatal("historical replay admitted fresh provider work")
			}
			if cell.defaults {
				assertAuthoredPeerAndFailures(t, process, runner, command, dir, path)
			} else {
				command.Stop(t)
			}
			if !bytes.Equal(before, mustReadFile(t, path)) {
				t.Fatal("historical replay mutated its source")
			}
		})
	}
}

func assertAuthoredPeerAndFailures(t *testing.T, process support.Process, runner *authoredReplayRunner, command *support.ProcessCommand, dir, path string) {
	t.Helper()
	peer := authoredReplayInput(t, runner.peerDir, "--session", uuid.NewString(), "--work", testutil.MustRepoPath(t, "docs/examples/startup-work.json"), "--no-record")
	peerCommand := support.StartProcessCommand(t, process, peer.Input)
	select {
	case <-runner.started:
	case <-time.After(support.ScaledTimeout(15 * time.Second)):
		t.Fatal("peer never reached controlled provider")
	}
	assertAuthoredCorruption(t, process, dir, path)
	assertAuthoredDispatchMismatch(t, process, dir, path)
	command.Stop(t)
	close(runner.gate)
	select {
	case <-peerCommand.Done():
	case <-time.After(support.ScaledTimeout(15 * time.Second)):
		t.Fatal("live peer did not complete after historical close")
	}
	if err := peerCommand.Err(); err != nil {
		t.Fatalf("live peer failed: %v", err)
	}
	if runner.callsIn(runner.peerDir) != 4 {
		t.Fatalf("peer provider calls=%d, want 4", runner.callsIn(runner.peerDir))
	}
}

func assertAuthoredReplaySession(t *testing.T, url, id string, wantWork int) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(url, id, "/work"))
	if len(listed.Results) != wantWork || support.CountWorkAtCustomerState(listed, "story:complete") != wantWork {
		t.Fatalf("replayed Work=%#v, want %d terminal items", listed.Results, wantWork)
	}
	events := support.GetFactoryEventsForSessionAt(t, url, id)
	assertAuthoredReplayHistory(t, events, wantWork*2)
	pivot := events[len(events)/2]
	tail := support.GetFactoryEventsAfterForSessionAt(t, url, id, support.FactoryEventReadCursor{AfterEventID: pivot.Id})
	if len(tail) != len(events)-len(events)/2-1 {
		t.Fatalf("cursor tail=%d, want %d", len(tail), len(events)-len(events)/2-1)
	}
	for index, event := range tail {
		if event.Id != events[len(events)/2+1+index].Id {
			t.Fatal("cursor rewrote retained event order")
		}
	}
}

func authoredReplayFactory(t *testing.T) string {
	t.Helper()
	dir := copyDirectory(t, testutil.MustRepoPath(t, "examples/simple-tasks"))
	// Keep authored inputs, workstation behavior and stop tokens; route the
	// copied workers through the controlled Codex command edge.
	for _, name := range []string{"executor", "reviewer"} {
		path := filepath.Join(dir, "workers", name, "AGENTS.md")
		data := strings.ReplaceAll(string(mustReadFile(t, path)), "modelProvider: CLAUDE", "modelProvider: CODEX")
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Return the existing fake-input shape without introducing a second command path.
func authoredReplayInput(t *testing.T, dir string, args ...string) *support.CapturedInputs {
	t.Helper()
	input := support.FakeInputs(t.Context(), append([]string{"you", "run", "--dir", dir}, args...))
	input.Input.Env = isolatedReplayEnvironmentFor(t)
	input.Input.WorkingDirectory = dir
	return input
}

func assertAuthoredReplayHistory(t *testing.T, events []factoryapi.FactoryEvent, dispatches int) {
	t.Helper()
	if composedLiveEventCount(events, factoryapi.FactoryEventTypeDispatchRequest) != dispatches || composedLiveEventCount(events, factoryapi.FactoryEventTypeDispatchResponse) != dispatches {
		t.Fatal("replay duplicated or lost dispatch history")
	}
	seen := make(map[string]bool)
	for index, event := range events {
		if seen[event.Id] {
			t.Fatalf("duplicate event %s", event.Id)
		}
		seen[event.Id] = true
		if index > 0 && event.Context.Sequence <= events[index-1].Context.Sequence {
			t.Fatalf("retained event order regressed: previous=%s/%d current=%s/%d", events[index-1].Id, events[index-1].Context.Sequence, event.Id, event.Context.Sequence)
		}
	}
}

func assertAuthoredCorruption(t *testing.T, process support.Process, dir, source string) {
	t.Helper()
	artifact := testutil.LoadReplayArtifact(t, source)
	artifact.Events[3].Id = artifact.Events[2].Id
	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := authoredReplayInput(t, dir, "--session", uuid.NewString(), "--replay", path, "--no-record")
	run := replayRun{err: process.Execute(inputs.Input), stderr: inputs.Stderr(), stdout: inputs.Stdout()}
	if run.err == nil || !strings.Contains(run.err.Error()+run.stderr, "INVALID_RECORDING_IDENTITY") {
		t.Fatalf("corrupt recording=%v; stderr=%s", run.err, run.stderr)
	}
	if !bytes.Equal(data, mustReadFile(t, path)) {
		t.Fatal("corrupt source was rewritten")
	}
}

func assertAuthoredDispatchMismatch(t *testing.T, process support.Process, dir, source string) {
	t.Helper()
	artifact := testutil.LoadReplayArtifact(t, source)
	for index := range artifact.Events {
		event := &artifact.Events[index]
		if string(event.Type) != string(factoryapi.FactoryEventTypeDispatchRequest) {
			continue
		}
		var payload factoryapi.DispatchRequestEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		payload.TransitionId = "unexpected-transition"
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		event.Payload = data
		break
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "divergent.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := authoredReplayInput(t, dir, "--session", uuid.NewString(), "--replay", path, "--no-record")
	run := replayRun{err: process.Execute(inputs.Input), stderr: inputs.Stderr(), stdout: inputs.Stdout()}
	if run.err == nil || !strings.Contains(run.err.Error(), "category=dispatch_mismatch") {
		t.Fatalf("genuine divergence=%v; stderr=%s", run.err, run.stderr)
	}
}

type authoredReplayRunner struct {
	peerDir  string
	gate     chan struct{}
	started  chan struct{}
	once     sync.Once
	delegate *support.RecordingCommandRunner
}

func (r *authoredReplayRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if strings.EqualFold(filepath.Clean(request.WorkDir), filepath.Clean(r.peerDir)) {
		r.once.Do(func() { close(r.started) })
		select {
		case <-r.gate:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	return r.delegate.Run(ctx, request)
}

func (r *authoredReplayRunner) callsIn(dir string) int {
	count := 0
	for _, request := range r.delegate.Requests() {
		if strings.EqualFold(filepath.Clean(request.WorkDir), filepath.Clean(dir)) {
			count++
		}
	}
	return count
}

func closeUnlessReleased(gate chan struct{}) {
	select {
	case <-gate:
	default:
		close(gate)
	}
}
