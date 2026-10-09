package addressing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type replayOwner struct{ session, work, dir, home string }

type replayFixture struct {
	process     support.ApplicationProcess
	url, worker string
	owners      []replayOwner
	calls       atomic.Int32
}

// One immutable root graph hosts both legacy recordings. Only replay bytes and
// the HTTP listener/provider command effects are replaced, through Edges.
func newReplayFixture(t *testing.T) *replayFixture {
	return newReplayFixtureWithRunner(t, nil, 2)
}

func newReplayFixtureWithRunner(t *testing.T, runner platformprocess.CommandRunner, ownerCount int) *replayFixture {
	t.Helper()
	return newReplayFixtureWithEdges(t, runner, ownerCount, serviceedges.Edges{})
}

func newReplayFixtureWithEdges(t *testing.T, runner platformprocess.CommandRunner, ownerCount int, replacements serviceedges.Edges) *replayFixture {
	t.Helper()
	f := &replayFixture{worker: uuid.NewString()}
	payloads := make(map[string][]byte)
	servers := make(map[int]*support.ProcessAPIServer)
	started := make(map[int]chan struct{})
	var host replayOwner
	if runner != nil {
		host = replayOwner{session: uuid.NewString(), dir: t.TempDir(), home: t.TempDir()}
		config, err := os.ReadFile(filepath.Join(support.LegacyFixtureDir(t, "executor_success"), "factory.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(host.dir, "factory.json"), config, 0o600); err != nil {
			t.Fatal(err)
		}
		servers[23999] = support.NewProcessAPIServer()
		started[23999] = make(chan struct{})
	}
	for i := range ownerCount {
		owner := replayOwner{session: uuid.NewString(), work: "work-" + uuid.NewString(), dir: t.TempDir(), home: t.TempDir()}
		f.owners = append(f.owners, owner)
		payloads[filepath.Join(owner.dir, "legacy.json")] = legacyRecording(t, owner, f.worker)
		if err := os.WriteFile(filepath.Join(owner.dir, "legacy.json"), payloads[filepath.Join(owner.dir, "legacy.json")], 0o600); err != nil {
			t.Fatal(err)
		}
		servers[24000+i] = support.NewProcessAPIServer()
		started[24000+i] = make(chan struct{})
	}
	if runner == nil {
		runner = &refuseProviderCalls{calls: &f.calls}
	}
	f.process = support.BuildProcess(t, serviceedges.Merge(serviceedges.Edges{
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			payload, ok := payloads[path]
			if !ok {
				return nil, fmt.Errorf("unknown replay %q", path)
			}
			return append([]byte(nil), payload...), nil
		},
		APIServerStarter: func(ctx context.Context, req platformhttpserver.StartRequest) error {
			close(started[req.Port])
			return servers[req.Port].Start(ctx, req)
		},
		ProviderCommandRunner: runner,
	}, replacements))
	if host.session != "" {
		inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", host.session,
			"--dir", host.dir, "--no-record", "--continuously", "--with-server", "--quiet", "--listen", "127.0.0.1:23999"})
		inputs.Input.Env = append(os.Environ(), "HOME="+host.home, "USERPROFILE="+host.home)
		inputs.Input.WorkingDirectory = host.dir
		command := support.StartProcessCommand(t, f.process, inputs.Input)
		t.Cleanup(func() { command.Stop(t) })
		select {
		case <-started[23999]:
		case <-command.Done():
			t.Fatalf("host opening failed: %v; stderr=%s", command.Err(), inputs.Stderr())
		case <-time.After(30 * time.Second):
			t.Fatal("host listener did not start")
		}
		f.url = servers[23999].WaitForURL(t)
	}
	commands := make([]*support.ProcessCommand, len(f.owners))
	stderrs := make([]func() string, len(f.owners))
	for i, owner := range f.owners {
		inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", owner.session,
			"--dir", owner.dir, "--replay", filepath.Join(owner.dir, "legacy.json"), "--no-record",
			"--continuously", "--with-server", "--quiet", "--listen", fmt.Sprintf("127.0.0.1:%d", 24000+i)})
		inputs.Input.Env = append(os.Environ(), "HOME="+owner.home, "USERPROFILE="+owner.home)
		inputs.Input.WorkingDirectory = owner.dir
		command := support.StartProcessCommand(t, f.process, inputs.Input)
		t.Cleanup(func() { command.Stop(t) })
		commands[i], stderrs[i] = command, inputs.Stderr
	}
	// All scenario-owned replay commands may reach the external reader together;
	// no invocation-wide gate serializes distinct Factory Session completion.
	for i, owner := range f.owners {
		command := commands[i]
		select {
		case <-started[24000+i]:
		case <-command.Done():
			t.Fatalf("replay opening failed: %v; stderr=%s", command.Err(), stderrs[i]())
		case <-time.After(30 * time.Second):
			t.Fatalf("replay opening did not reach listener; stderr=%s", stderrs[i]())
		}
		endpoint := servers[24000+i].WaitForURL(t)
		support.WaitForSessionTerminalStatus(t, endpoint, owner.session, 15*time.Second)
		if host.session == "" {
			f.url = endpoint
		}
	}
	return f
}

type refuseProviderCalls struct{ calls *atomic.Int32 }

func (r *refuseProviderCalls) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	return platformprocess.CommandResult{}, fmt.Errorf("historical addressing must not execute a provider")
}

func legacyRecording(t *testing.T, owner replayOwner, worker string) []byte {
	t.Helper()
	config := map[string]any{"name": "legacy-addressing", "workTypes": []any{map[string]any{
		"name": "task", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "complete", "type": "TERMINAL"}}}},
		"workers": []any{map[string]any{"name": "worker"}}, "workstations": []any{map[string]any{
			"name": "process", "worker": "worker", "inputs": []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs": []any{map[string]any{"workType": "task", "state": "complete"}}}}}
	snapshot, err := definitions.NewFactorySnapshot(config)
	if err != nil {
		t.Fatal(err)
	}
	dispatch := "dispatch-" + owner.session
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	types := []definitions.FactoryEventType{definitions.FactoryEventTypeRunRequest, definitions.FactoryEventTypeWorkRequest,
		definitions.FactoryEventTypeDispatchRequest, definitions.FactoryEventTypeDispatchWorkerSessionAssoc,
		definitions.FactoryEventTypeDispatchResponse, definitions.FactoryEventTypeRunResponse}
	payloads := []any{definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: base},
		map[string]any{"source": "external-submit", "type": "FACTORY_REQUEST_BATCH", "works": []any{map[string]any{
			"name": "legacy", "workId": owner.work, "requestId": "request-" + owner.session, "workTypeId": "task", "state": map[string]string{"name": "init", "type": "INITIAL"}, "traceId": "trace-" + owner.session}}},
		map[string]any{"transitionId": "process", "inputs": []any{map[string]string{"workId": owner.work}}},
		definitions.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: worker},
		map[string]any{"transitionId": "process", "outcome": "ACCEPTED", "output": "COMPLETE"},
		map[string]any{"state": "COMPLETED"}}
	var events []definitions.FactoryEvent
	for i, kind := range types {
		payload, err := json.Marshal(payloads[i])
		if err != nil {
			t.Fatal(err)
		}
		event := definitions.FactoryEvent{Id: fmt.Sprintf("%s-%d", owner.session, i), Type: kind,
			SchemaVersion: definitions.FactoryEventSchemaVersionV1, Payload: payload,
			Context: definitions.FactoryEventContext{SessionID: &owner.session, Sequence: i, Tick: i,
				EventTime: base.Add(time.Duration(i) * time.Second)}}
		if i >= 1 && i <= 4 {
			works := []string{owner.work}
			event.Context.WorkIDs = &works
		}
		if i >= 2 && i <= 4 {
			event.Context.DispatchID = &dispatch
		}
		events = append(events, event)
	}
	bytes, err := json.Marshal(definitions.ReplayArtifact{SchemaVersion: definitions.ReplayV1SourceFormat, RecordedAt: base, Events: events})
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}
