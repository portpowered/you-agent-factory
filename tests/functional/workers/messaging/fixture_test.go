package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
	return startHostMode(t, home, dir, settings, false, workers...)
}

func startHostMode(t *testing.T, home, dir, settings string, mock bool, workers ...string) *fixture {
	t.Helper()
	return startHostEffects(t, home, dir, settings, mock, serviceedges.Edges{}, workers...)
}

func startHostEffects(t *testing.T, home, dir, settings string, mock bool, effects serviceedges.Edges, workers ...string) *fixture {
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
	f := &fixture{home: home, dir: dir, env: builtcliacceptance.ProcessEnvForIsolatedHome(home), runner: &commandRunner{workers: make(map[string]*workerGate)}}
	for _, id := range workers {
		f.runner.workers[id] = &workerGate{started: make(chan map[string]string, 1), release: make(chan struct{})}
	}
	f.server = support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WorkingDirectory: dir, Env: f.env,
		UseMockWorkers: mock,
		Args:           []string{"--server", "http://127.0.0.1:1"},
		Edges:          serviceedges.Merge(serviceedges.Edges{ProviderCommandRunner: f.runner}, effects),
		BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
			support.InitializeCustomerHomeWithProcess(tb, process, input.Env, dir)
		},
	})
	return f
}

func (f *fixture) invoke(t *testing.T, id string, caller map[string]string) map[string]string {
	t.Helper()
	session := support.OpenFactorySessionAt(t, f.server.URL(), f.dir).Session.Id
	request := map[string]any{"requestId": id + "-invoke", "workerSessionId": id, "execution": map[string]any{
		"workstationName": "direct", "workingDirectory": f.dir, "factorySessionId": session,
		"workerType": "direct-worker", "runnerId": "codex", "executorProvider": "codex", "modelProvider": "codex", "model": "functional-model", "userMessage": "controlled input",
		"dispatch": map[string]any{"dispatchId": id + "-attempt", "workstationName": "direct", "workerType": "direct-worker"},
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
