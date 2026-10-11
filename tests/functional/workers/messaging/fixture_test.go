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
)

type workerGate struct {
	started chan map[string]string
	release chan struct{}
}

type commandRunner struct{ workers map[string]*workerGate }

func (r *commandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	env := make(map[string]string)
	for _, item := range request.Env {
		if key, value, ok := strings.Cut(item, "="); ok {
			env[key] = value
		}
	}
	gate := r.workers[env["YOU_WORKER_SESSION_ID"]]
	if gate == nil {
		return platformprocess.CommandResult{}, context.Canceled
	}
	gate.started <- env
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	case <-gate.release:
		return platformprocess.CommandResult{Stdout: []byte("{\"type\":\"thread.started\",\"thread_id\":\"controlled-thread\"}\n{\"type\":\"turn.completed\"}\n")}, nil
	}
}

type fixture struct {
	server    *support.FunctionalAPIServer
	home, dir string
	env       []string
	runner    *commandRunner
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
	return result.Message
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
	return result.Message
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
	_, err := support.WaitForObservation(15*time.Second, func() (int, error) {
		select {
		case err := <-done:
			done <- err
			return 0, fmt.Errorf("replay exited: %v %s", err, inputs.Stderr())
		default:
		}
		status, _ := f.request(t, http.MethodGet, "/worker-sessions/"+worker, nil, nil)
		return status, nil
	}, func(status int) bool { return status == http.StatusConflict })
	if err != nil {
		t.Fatalf("legacy replay did not expose its equal-ID peer: %v", err)
	}
}
