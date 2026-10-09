package addressing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type replayOwner struct{ session, work, dir, home string }

type replayFixture struct {
	process     support.ApplicationProcess
	url, worker string
	owners      []replayOwner
	calls       atomic.Int32
	group       *addressingProcessGroup
	runner      platformprocess.CommandRunner
}

// A package-owned immutable root graph hosts scenario-owned legacy recordings.
// Replay, listener, provider and persistence effects route through Edges.
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
	group := &sharedAddressingProcess
	if len(replacements.ProviderCatalogCapabilityOverrides) != 0 {
		group = &unsupportedAddressingProcess
	}
	f.group = group
	f.process = group.get(t, replacements)
	hostPort := group.port()
	replayPort := group.port()
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
		servers[hostPort] = support.NewProcessAPIServer()
		started[hostPort] = make(chan struct{})
	}
	for i := range ownerCount {
		owner := replayOwner{session: uuid.NewString(), work: "work-" + uuid.NewString(), dir: t.TempDir(), home: t.TempDir()}
		f.owners = append(f.owners, owner)
		payloads[filepath.Join(owner.dir, "legacy.json")] = legacyRecording(t, owner, f.worker)
		if err := os.WriteFile(filepath.Join(owner.dir, "legacy.json"), payloads[filepath.Join(owner.dir, "legacy.json")], 0o600); err != nil {
			t.Fatal(err)
		}
		servers[replayPort+i] = support.NewProcessAPIServer()
		started[replayPort+i] = make(chan struct{})
	}
	if runner == nil {
		runner = &refuseProviderCalls{calls: &f.calls}
	}
	f.runner = runner
	f.registerReplayEffects(replacements, payloads, servers, started, host)

	if host.session != "" {
		inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", host.session,
			"--dir", host.dir, "--no-record", "--continuously", "--with-server", "--quiet", "--listen", fmt.Sprintf("127.0.0.1:%d", hostPort)})
		inputs.Input.Env = append(os.Environ(), "HOME="+host.home, "USERPROFILE="+host.home)
		inputs.Input.WorkingDirectory = host.dir
		command := support.StartProcessCommand(t, f.process, inputs.Input)
		t.Cleanup(func() { command.Stop(t) })
		select {
		case <-started[hostPort]:
		case <-command.Done():
			t.Fatalf("host opening failed: %v; stderr=%s", command.Err(), inputs.Stderr())
		case <-time.After(30 * time.Second):
			t.Fatal("host listener did not start")
		}
		f.url = servers[hostPort].WaitForURL(t)
	}
	f.startReplayOwners(t, replayPort, servers, started, host.session != "")
	return f
}

func (f *replayFixture) registerReplayEffects(replacements serviceedges.Edges, payloads map[string][]byte, servers map[int]*support.ProcessAPIServer, started map[int]chan struct{}, host replayOwner) {
	group := f.group
	group.mu.Lock()
	for path, payload := range payloads {
		data := append([]byte(nil), payload...)
		reader := replacements.FactorySessionReplayRecordingReader
		if reader == nil {
			reader = func(string) ([]byte, error) { return append([]byte(nil), data...), nil }
		}
		group.readers[path] = reader
	}
	for port, server := range servers {
		group.servers[port] = server
		group.started[port] = started[port]
	}
	if host.dir != "" {
		group.runners[host.dir] = f.runner
	}
	if store, ok := replacements.WorkerRecordingWriter.(*selectedOpeningFailureStore); ok {
		store.WorkerRecordingStore = group.WorkerRecordingStore
		group.failures[store.successor] = store
	}
	if store, ok := replacements.WorkerRecordingWriter.(*selectedTerminalGateStore); ok {
		store.WorkerRecordingStore = group.WorkerRecordingStore
		store.source = f.worker
		group.failures[f.worker] = store
	}
	group.mu.Unlock()
}

func (f *replayFixture) startReplayOwners(t *testing.T, replayPort int, servers map[int]*support.ProcessAPIServer, started map[int]chan struct{}, hasHost bool) {
	t.Helper()
	commands := make([]*support.ProcessCommand, len(f.owners))
	stderrs := make([]func() string, len(f.owners))
	for i, owner := range f.owners {
		inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", owner.session,
			"--dir", owner.dir, "--replay", filepath.Join(owner.dir, "legacy.json"), "--no-record",
			"--continuously", "--with-server", "--quiet", "--listen", fmt.Sprintf("127.0.0.1:%d", replayPort+i)})
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
		case <-started[replayPort+i]:
		case <-command.Done():
			t.Fatalf("replay opening failed: %v; stderr=%s", command.Err(), stderrs[i]())
		case <-time.After(30 * time.Second):
			t.Fatalf("replay opening did not reach listener; stderr=%s", stderrs[i]())
		}
		endpoint := servers[replayPort+i].WaitForURL(t)
		support.WaitForSessionTerminalStatus(t, endpoint, owner.session, 15*time.Second)
		if !hasHost {
			f.url = endpoint
		}
	}
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

// The catalog capability refusal is a different immutable production edge and
// is the only separate graph (FSTD-005 section 11). Every ordinary scenario,
// including recording faults and replay barriers, shares the first process.
var sharedAddressingProcess, unsupportedAddressingProcess addressingProcessGroup

type addressingProcessGroup struct {
	once     sync.Once
	process  support.ApplicationProcess
	err      error
	mu       sync.RWMutex
	nextPort int
	readers  map[string]func(string) ([]byte, error)
	servers  map[int]*support.ProcessAPIServer
	started  map[int]chan struct{}
	runners  map[string]platformprocess.CommandRunner
	failures map[string]recordings.WorkerRecordingWriter
	recordings.WorkerRecordingStore
}

func TestMain(m *testing.M) {
	code := m.Run()
	if err := closeAddressingProcesses(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

// FunctionalMonolithCleanup retains the native TestMain lifecycle when the
// functional lane consolidates package execution into its shared test binary.
func FunctionalMonolithCleanup(t *testing.T) {
	t.Helper()
	if err := closeAddressingProcesses(); err != nil {
		t.Errorf("close addressing processes: %v", err)
	}
}

func closeAddressingProcesses() error {
	for _, group := range []*addressingProcessGroup{&sharedAddressingProcess, &unsupportedAddressingProcess} {
		if group.process != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := group.process.Close(ctx)
			cancel()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *addressingProcessGroup) get(t *testing.T, replacements serviceedges.Edges) support.ApplicationProcess {
	t.Helper()
	g.once.Do(func() {
		g.readers = make(map[string]func(string) ([]byte, error))
		g.servers = make(map[int]*support.ProcessAPIServer)
		g.started = make(map[int]chan struct{})
		g.runners = make(map[string]platformprocess.CommandRunner)
		g.failures = make(map[string]recordings.WorkerRecordingWriter)
		g.nextPort = 25000
		g.process, g.err = support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
			ProviderCatalogCapabilityOverrides: replacements.ProviderCatalogCapabilityOverrides,
			FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
				g.mu.RLock()
				reader := g.readers[path]
				g.mu.RUnlock()
				if reader == nil {
					return nil, fmt.Errorf("unknown scenario replay %q", path)
				}
				return reader(path)
			},
			APIServerStarter: func(ctx context.Context, req platformhttpserver.StartRequest) error {
				g.mu.RLock()
				server, started := g.servers[req.Port], g.started[req.Port]
				g.mu.RUnlock()
				if server == nil {
					return fmt.Errorf("unknown scenario port %d", req.Port)
				}
				close(started)
				return server.Start(ctx, req)
			},
			ProviderCommandRunner:        g,
			WorkerRecordingWriter:        g,
			WorkerRecordingStoreObserver: func(store recordings.WorkerRecordingStore) { g.WorkerRecordingStore = store },
		})
	})
	if g.err != nil {
		t.Fatal(g.err)
	}
	return g.process
}

func (g *addressingProcessGroup) port() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	port := g.nextPort
	g.nextPort += 4
	return port
}

func (g *addressingProcessGroup) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return g.RunStreaming(ctx, req, nil)
}

func (g *addressingProcessGroup) RunStreaming(ctx context.Context, req platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	g.mu.RLock()
	runner := g.runners[req.WorkDir]
	g.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("unknown scenario provider directory %q", req.WorkDir)
	}
	if streaming, ok := runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	}); ok {
		return streaming.RunStreaming(ctx, req, observe)
	}
	return runner.Run(ctx, req)
}

func (g *addressingProcessGroup) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	g.mu.RLock()
	failure := g.failures[record.WorkerSessionID]
	g.mu.RUnlock()
	if failure != nil {
		return failure.PersistWorkerRecord(ctx, record)
	}
	return g.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
}

// Preserve the real store's summary capability when replacing only recording
// writes; archived lookup must retain the same unknown-ID behavior.
func (g *addressingProcessGroup) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	reader, ok := g.WorkerRecordingStore.(recordings.WorkerCapturedSummaryReader)
	if !ok {
		return recordings.WorkerCapturedSummary{}, fmt.Errorf("recording store has no summary reader")
	}
	return reader.LookupWorkerSessionSummary(ctx, id)
}
