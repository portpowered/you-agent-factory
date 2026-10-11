package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type workerGate struct {
	started            chan map[string]string
	release            chan struct{}
	requests           chan platformprocess.CommandRequest
	result             *platformprocess.CommandResult
	progress           <-chan struct{}
	startupDiagnostics []byte
}

type commandRunner struct {
	workers map[string]*workerGate
	revival *workerGate
	factory *workerGate
}

func (r *commandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return r.run(ctx, request, nil)
}

func (r *commandRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	return r.run(ctx, request, observe)
}

func (r *commandRunner) run(ctx context.Context, request platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	env := make(map[string]string)
	for _, item := range request.Env {
		if key, value, ok := strings.Cut(item, "="); ok {
			env[key] = value
		}
	}
	gate := r.workers[env["YOU_WORKER_SESSION_ID"]]
	if gate == nil && strings.Contains(string(request.Stdin), "Factory-origin lead") {
		gate = r.factory
	}
	if gate == nil {
		gate = r.revival
	}
	if gate == nil {
		return platformprocess.CommandResult{}, context.Canceled
	}
	select {
	case gate.started <- env:
	default:
	}
	if gate.requests != nil {
		gate.requests <- request
	}
	if err := gate.observeStart(ctx, observe); err != nil {
		return platformprocess.CommandResult{}, err
	}
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	case <-gate.release:
		return gate.completedOutput(observe), nil
	}
}

func (gate *workerGate) observeStart(ctx context.Context, observe platformprocess.OutputChunkObserver) error {
	if observe != nil && gate.result == nil {
		if gate.progress != nil {
			select {
			case <-gate.progress:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		observe(platformprocess.OutputStreamStdout, []byte("{\"type\":\"thread.started\",\"thread_id\":\"controlled-thread\"}\n{\"type\":\"turn.started\"}\n"))
		if len(gate.startupDiagnostics) != 0 {
			for _, line := range bytes.SplitAfter(gate.startupDiagnostics, []byte("\n")) {
				if err := ctx.Err(); err != nil {
					return err
				}
				if len(line) != 0 {
					observe(platformprocess.OutputStreamStderr, line)
				}
			}
		}
	}
	return nil
}

func (gate *workerGate) completedOutput(observe platformprocess.OutputChunkObserver) platformprocess.CommandResult {

	if gate.result != nil {
		if observe != nil {
			observe(platformprocess.OutputStreamStdout, gate.result.Stdout)
			observe(platformprocess.OutputStreamStderr, gate.result.Stderr)
		}
		return *gate.result
	}
	output := []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"controlled-answer\",\"type\":\"agent_message\",\"text\":\"COMPLETE\"}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n")
	if observe != nil {
		observe(platformprocess.OutputStreamStdout, output)
	} else {
		output = append([]byte("{\"type\":\"thread.started\",\"thread_id\":\"controlled-thread\"}\n"), output...)
	}
	return platformprocess.CommandResult{Stdout: output}
}

type fixture struct {
	server    *support.FunctionalAPIServer
	home, dir string
	env       []string
	runner    *commandRunner
	outcomes  *deliveryOutcomeSignal
}

// One immutable profile requires one graph; leaves share its host and own
// their worker IDs, inbox threads and gates. Restart explicitly replaces it.
func startHost(t *testing.T, home, dir string, settings string, workers ...string) *fixture {
	t.Helper()
	return startHostEffects(t, home, dir, settings, serviceedges.Edges{}, workers...)
}

func startHostEffects(t *testing.T, home, dir, settings string, effects serviceedges.Edges, workers ...string) *fixture {
	t.Helper()
	return startHostTransport(t, home, dir, settings, effects, nil, workers...)
}

func startHostTransport(t *testing.T, home, dir, settings string, effects serviceedges.Edges, wrap func(platformhttpserver.Starter) platformhttpserver.Starter, workers ...string) *fixture {
	return startHostRunner(t, home, dir, settings, effects, wrap, nil, workers...)
}

func startHostRunner(t *testing.T, home, dir, settings string, effects serviceedges.Edges, wrap func(platformhttpserver.Starter) platformhttpserver.Starter, revival *workerGate, workers ...string) *fixture {
	return startHostDelivery(t, home, dir, settings, effects, wrap, revival, nil, workers...)
}

func startHostDelivery(t *testing.T, home, dir, settings string, effects serviceedges.Edges, wrap func(platformhttpserver.Starter) platformhttpserver.Starter, revival, factory *workerGate, workers ...string) *fixture {
	t.Helper()
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if settings != "" {
		configDir := filepath.Join(home, ".you-agent-factory")
		if err := os.MkdirAll(configDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(settings), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	definition, err := os.ReadFile(filepath.Join(support.LegacyFixtureDir(t, "executor_success"), "factory.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), definition, 0o600); err != nil {
		t.Fatal(err)
	}
	support.WriteAgentConfig(t, dir, "worker", "---\ntype: MODEL_WORKER\nmodel: functional-model\nmodelProvider: CODEX\nstopToken: COMPLETE\n---\nProcess the input task.\n")
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\nControlled Factory input: {{ (index .Inputs 0).Payload }}\n")
	f := &fixture{home: home, dir: dir, env: builtcliacceptance.ProcessEnvForIsolatedHome(home), runner: &commandRunner{workers: make(map[string]*workerGate)}}
	f.runner.revival = revival
	f.runner.factory = factory
	f.outcomes = &deliveryOutcomeSignal{Core: zapcore.NewNopCore(), done: map[string]chan struct{}{}}
	core := zapcore.Core(f.outcomes)
	if effects.ProcessLogger != nil {
		core = zapcore.NewTee(effects.ProcessLogger.Core(), core)
	}
	effects.ProcessLogger = zap.New(core)
	for _, id := range workers {
		f.runner.workers[id] = &workerGate{started: make(chan map[string]string, 1), release: make(chan struct{})}
	}
	f.server = support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WorkingDirectory: dir, Env: f.env,
		Args:                 []string{"--server", "http://127.0.0.1:1"},
		Edges:                serviceedges.Merge(serviceedges.Edges{ProviderCommandRunner: f.runner}, effects),
		WrapAPIServerStarter: wrap,
		BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
			support.InitializeCustomerHomeWithProcess(tb, process, input.Env, dir)
		},
	})
	return f
}

func (f *fixture) invoke(t *testing.T, id string, caller map[string]string) map[string]string {
	t.Helper()
	return f.invokeProject(t, id, "", caller)
}

func (f *fixture) continueCaller(t *testing.T, source, successor string) map[string]string {
	t.Helper()
	status, raw := f.request(t, http.MethodPost, "/worker-sessions/"+source+"/continue", map[string]any{
		"resolveHead": true, "requestId": successor + "-visit", "successorWorkerSessionId": successor, "followUpInput": "authorized later visit",
	}, nil)
	if status != http.StatusAccepted {
		t.Fatalf("sender continuation after restart: %d %s", status, raw)
	}
	select {
	case caller := <-f.runner.workers[successor].started:
		if caller["YOU_WORKER_SESSION_ID"] != successor || caller["YOU_WORKER_SESSION_TOKEN"] == "" {
			t.Fatal("new sender lacks genuine execution credentials")
		}
		return caller
	case <-time.After(15 * time.Second):
		t.Fatal("new sender did not start")
	}
	return nil
}

func (f *fixture) invokeProject(t *testing.T, id, project string, caller map[string]string) map[string]string {
	t.Helper()
	session := support.OpenFactorySessionAt(t, f.server.URL(), f.dir).Session.Id
	request := map[string]any{"requestId": id + "-invoke", "workerSessionId": id, "execution": map[string]any{
		"workstationName": "direct", "workingDirectory": f.dir, "factorySessionId": session,
		"workerType": "direct-worker", "runnerId": "codex", "executorProvider": "codex", "modelProvider": "codex", "model": "functional-model", "userMessage": "controlled input", "projectId": project,
		"dispatch": map[string]any{"dispatchId": id + "-attempt", "workstationName": "direct", "workerType": "direct-worker", "projectId": project},
	}}
	status, body := f.request(t, http.MethodPost, "/worker-sessions", request, caller)
	if status != http.StatusAccepted {
		t.Fatalf("invoke: %d %s", status, body)
	}
	select {
	case env := <-f.runner.workers[id].started:
		if env["YOU_WORKER_SESSION_ID"] != id || env["YOU_WORKER_SESSION_TOKEN"] == "" {
			t.Fatal("missing genuine execution credentials")
		}
		return env
	case <-time.After(15 * time.Second):
		t.Fatal("worker did not reach controlled command edge")
	}
	return nil
}

func (f *fixture) request(t *testing.T, method, path string, body any, caller map[string]string) (int, []byte) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, f.server.URL()+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if caller != nil {
		req.Header.Set("X-You-Worker-Session-Id", caller["YOU_WORKER_SESSION_ID"])
		req.Header.Set("Authorization", "Bearer "+caller["YOU_WORKER_SESSION_TOKEN"])
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}

func (f *fixture) send(t *testing.T, request map[string]any, caller map[string]string) api.AgentMessage {
	t.Helper()
	status, data := f.request(t, http.MethodPost, "/messages", request, caller)
	if status != http.StatusAccepted {
		t.Fatalf("send: %d %s", status, data)
	}
	var result api.AgentMessageSendResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return f.awaitDeliveryOutcome(t, caller, result.Message)
}

func (f *fixture) cli(t *testing.T, caller map[string]string, args ...string) []byte {
	t.Helper()
	argv := append([]string{"you", "--json", "--server", f.server.URL(), "message"}, args...)
	argv = append(argv, "--output", "json")
	inputs := support.FakeInputs(t.Context(), argv)
	inputs.Input.Env, inputs.Input.WorkingDirectory = append([]string{}, f.env...), f.dir
	if caller != nil {
		inputs.Input.Env = append(inputs.Input.Env, "YOU_WORKER_SESSION_ID="+caller["YOU_WORKER_SESSION_ID"], "YOU_WORKER_SESSION_TOKEN="+caller["YOU_WORKER_SESSION_TOKEN"])
	}
	if err := f.server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI message: %v %s", err, inputs.Stderr())
	}
	return []byte(inputs.Stdout())
}

func (f *fixture) cliSend(t *testing.T, caller map[string]string, args ...string) api.AgentMessage {
	t.Helper()
	var result api.AgentMessageSendResponse
	if err := json.Unmarshal(f.cli(t, caller, args...), &result); err != nil {
		t.Fatal(err)
	}
	return f.awaitDeliveryOutcome(t, caller, result.Message)
}

// The logger edge supplies only a completion signal after commit. The public
// read proves the final outcome. Raw-send journeys separately protect pending
// responses and register live-only follow before sending.
func (f *fixture) awaitDeliveryOutcome(t *testing.T, caller map[string]string, message api.AgentMessage) api.AgentMessage {
	t.Helper()
	if message.Reason == nil || *message.Reason != "REVIVE_PENDING" {
		return message
	}
	select {
	case <-f.outcomes.signal(message.MessageId):
	case <-time.After(15 * time.Second):
		t.Fatal("provider outcome did not commit")
	}
	page := readPage(t, f, "/messages?threadId="+message.ThreadId, caller)
	for _, m := range page.Messages {
		if m.MessageId == message.MessageId {
			return m
		}
	}
	t.Fatal("settled message disappeared from public read")
	return api.AgentMessage{}
}

type deliveryOutcomeSignal struct {
	zapcore.Core
	mu   sync.Mutex
	done map[string]chan struct{}
}

func (s *deliveryOutcomeSignal) Enabled(zapcore.Level) bool        { return true }
func (s *deliveryOutcomeSignal) With([]zapcore.Field) zapcore.Core { return s }
func (s *deliveryOutcomeSignal) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return checked.AddCore(entry, s)
}
func (s *deliveryOutcomeSignal) signal(id string) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done[id] == nil {
		s.done[id] = make(chan struct{})
	}
	return s.done[id]
}
func (s *deliveryOutcomeSignal) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if entry.Message != "Agent Message revival outcome committed" {
		return nil
	}
	var id, state string
	for _, field := range fields {
		if field.Key == "message_id" {
			id = field.String
		}
		if field.Key == "state" {
			state = field.String
		}
	}
	if id == "" || state == "ADMITTED" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done[id] == nil {
		s.done[id] = make(chan struct{})
	}
	select {
	case <-s.done[id]:
	default:
		close(s.done[id])
	}
	return nil
}

// Hydrate a legacy equal-ID observation through the same public replay path
// used by AM-T2, without registry mutation or another root graph.
func (f *fixture) replayPeer(t *testing.T, worker, session string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, "factory.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	snapshot, err := definitions.NewFactorySnapshot(config)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	types := []definitions.FactoryEventType{definitions.FactoryEventTypeRunRequest, definitions.FactoryEventTypeWorkRequest,
		definitions.FactoryEventTypeDispatchRequest, definitions.FactoryEventTypeDispatchWorkerSessionAssoc,
		definitions.FactoryEventTypeDispatchResponse, definitions.FactoryEventTypeRunResponse}
	payloads := []any{definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: base},
		map[string]any{"source": "external-submit", "type": "FACTORY_REQUEST_BATCH", "works": []any{map[string]any{
			"name": "legacy", "workId": "legacy-work", "requestId": "legacy-request", "workTypeId": "task", "state": map[string]string{"name": "init", "type": "INITIAL"}, "traceId": "legacy-trace"}}},
		map[string]any{"transitionId": "process", "inputs": []any{map[string]string{"workId": "legacy-work"}}},
		definitions.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: worker},
		map[string]any{"transitionId": "process", "outcome": "ACCEPTED", "output": "COMPLETE"},
		map[string]any{"state": "COMPLETED"}}
	var events []definitions.FactoryEvent
	for i, kind := range types {
		payload, err := json.Marshal(payloads[i])
		if err != nil {
			t.Fatal(err)
		}
		event := definitions.FactoryEvent{Id: fmt.Sprintf("legacy-event-%d", i), Type: kind,
			SchemaVersion: definitions.FactoryEventSchemaVersionV1, Payload: payload,
			Context: definitions.FactoryEventContext{SessionID: &session, Sequence: i, Tick: i, EventTime: base.Add(time.Duration(i) * time.Second)}}
		if i >= 1 && i <= 4 {
			works := []string{"legacy-work"}
			event.Context.WorkIDs = &works
		}
		if i >= 2 && i <= 4 {
			dispatch := "legacy-dispatch"
			event.Context.DispatchID = &dispatch
		}
		events = append(events, event)
	}
	data, err = json.Marshal(definitions.ReplayArtifact{SchemaVersion: definitions.ReplayV1SourceFormat, RecordedAt: base, Events: events})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	f.startReplayPeer(t, worker, session, dir, path)
}

func (f *fixture) startReplayPeer(t *testing.T, worker, session, dir, path string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--session", session, "--dir", dir, "--replay", path, "--no-record", "--quiet", "--continuously"})
	inputs.Input.Env, inputs.Input.WorkingDirectory = f.env, dir
	done := make(chan error, 1)
	go func() { done <- f.server.Execute(t, inputs.Input) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("legacy replay did not stop")
		}
	})
	// An ambiguous unscoped read only proves that replay has published the
	// association. Wait for its terminal observation before testing candidates.
	_, err := support.WaitForObservation(15*time.Second, func() (bool, error) {
		select {
		case err := <-done:
			done <- err
			return false, fmt.Errorf("replay exited: %v %s", err, inputs.Stderr())
		default:
		}
		status, data := f.request(t, http.MethodGet, "/worker-sessions/"+worker+"?factorySessionId="+session, nil, nil)
		var observation api.WorkerSessionObservation
		if status != http.StatusOK {
			return false, nil
		}
		if err := json.Unmarshal(data, &observation); err != nil {
			return false, err
		}
		return observation.WorkerSessionId == worker && observation.FactorySessionId != nil &&
			*observation.FactorySessionId == session && observation.State == "COMPLETED", nil
	}, func(ready bool) bool { return ready })
	if err != nil {
		t.Fatalf("legacy replay did not expose its completed equal-ID peer: %v", err)
	}
}
