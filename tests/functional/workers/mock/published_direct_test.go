package mock

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type directExampleDeniedRunner struct{ calls atomic.Int32 }

func (r *directExampleDeniedRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	return platformprocess.CommandResult{}, errors.New("native execution denied for published mock example")
}

// These private hosts have immutable provider policies and deny every native
// effect. Restart joins the original host before reopening its capture profile.
func TestRejectedMockCapturedOutput(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			dir := publishedDirectMockFactory(t)
			mockPath := filepath.Join(dir, "mock-workers.json")
			if err := os.WriteFile(mockPath, []byte(`{"mockWorkers":[{"runType":"reject","rejectConfig":{"stdout":"PRIVATE ordinary stdout 世界 declared-credential","stderr":"PRIVATE ordinary stderr 世界 declared-credential","exitCode":42}}]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			environment := builtcliacceptance.ProcessEnvForIsolatedHome(publishedDirectMockHome(t))
			denied := &directExampleDeniedRunner{}
			cfg := support.FunctionalAPIServerConfig{
				FactoryDir: dir, WaitForServiceModeRuntime: true, Env: environment,
				Args:  []string{"--with-mock-workers", mockPath},
				Edges: serviceedges.Edges{ProviderCommandRunner: denied, ScriptCommandRunner: denied},
			}
			host := support.StartFunctionalAPIServer(t, cfg)
			document := publishedDirectExecutionDocument(t)
			execution := document["execution"].(map[string]any)
			execution["workingDirectory"] = dir
			execution["envVars"] = map[string]string{"API_KEY": "declared-credential"}
			execution["runnerId"], execution["executorProvider"], execution["modelProvider"] = provider, provider, provider
			if provider == "claude" {
				execution["model"] = "claude-test"
			}
			data, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "execution.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			inputs := support.FakeInputs(t.Context(), []string{"you", "--server", host.URL(), "--remote", "--json", "worker-sessions", "invoke", "--execution", path, "--async"})
			inputs.Input.WorkingDirectory, inputs.Input.Env = dir, environment
			if err := host.Execute(t, inputs.Input); err != nil {
				t.Fatalf("invoke: %v %s", err, inputs.Stderr())
			}
			read := func() string {
				inputs := support.FakeInputs(t.Context(), []string{"you", "--server", host.URL(), "--json", "worker-sessions", "read", "--worker-session-id", "direct-example-session", "--view", "logs"})
				inputs.Input.WorkingDirectory, inputs.Input.Env = dir, environment
				if err := host.Execute(t, inputs.Input); err != nil {
					t.Fatalf("read: %v %s", err, inputs.Stderr())
				}
				return inputs.Stdout()
			}
			before, err := support.WaitForObservation(30*time.Second, func() (string, error) { return read(), nil }, func(s string) bool { return strings.Contains(s, `"health":"COMPLETE"`) })
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"PRIVATE ordinary stdout 世界", "PRIVATE ordinary stderr 世界"} {
				if strings.Count(before, marker) != 1 {
					t.Fatalf("output count for %q: %s", marker, before)
				}
			}
			if strings.Index(before, "ordinary stdout") > strings.Index(before, "ordinary stderr") {
				t.Fatalf("stream order: %s", before)
			}
			if strings.Contains(before, "declared-credential") || strings.Count(before, "redacted") != 2 {
				t.Fatalf("declared secret redaction: %s", before)
			}
			show := support.FakeInputs(t.Context(), []string{"you", "--server", host.URL(), "--json", "worker-sessions", "show", "--worker-session-id", "direct-example-session"})
			show.Input.WorkingDirectory, show.Input.Env = dir, environment
			if err := host.Execute(t, show.Input); err != nil || !strings.Contains(show.Stdout(), `"state":"FAILED"`) {
				t.Fatalf("failed show: %v %s %s", err, show.Stdout(), show.Stderr())
			}
			host.Close(t)
			host = support.StartFunctionalAPIServer(t, cfg)
			var original, restored any
			if err := json.Unmarshal([]byte(before), &original); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(read()), &restored); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original, restored) {
				t.Fatalf("restart changed captured output: before=%v after=%v", original, restored)
			}
			if denied.calls.Load() != 0 {
				t.Fatalf("native calls = %d", denied.calls.Load())
			}
		})
	}
}

// Direct origin needs no submitted Work. This private host has its own idle
// Factory Session and profile, and its immutable accept policy differs from
// the named-mock fixture. All commands reuse its one root-built process.
func TestPublishedDirectWorkerSessionMockJourney(t *testing.T) {
	t.Parallel()
	dir := publishedDirectMockFactory(t)
	mockPath := filepath.Join(dir, "mock-workers.json")
	if err := os.WriteFile(mockPath, []byte(`{"unmatchedDispatchPolicy":"accept","mockWorkers":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	denied := &directExampleDeniedRunner{}
	var nativeCalls atomic.Int32
	defer func() {
		t.Logf("effect-denying boundaries: provider/script calls=%d, subprocess/ACP calls=%d", denied.calls.Load(), nativeCalls.Load())
	}()
	environment := builtcliacceptance.ProcessEnvForIsolatedHome(publishedDirectMockHome(t))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:                   environment,
		Args:                  []string{"--with-mock-workers", mockPath},
		DeniedProcessAttempts: &nativeCalls,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: denied, ScriptCommandRunner: denied,
			ProvidersStdioPipeFactory: func() (platformprocess.StdioChannel, error) {
				nativeCalls.Add(1)
				return nil, errors.New("ACP pipe creation denied for published mock example")
			},
		},
	})
	support.WaitForRuntimeIdle(t, server.URL(), 10*time.Second)
	document := publishedDirectExecutionDocument(t)
	document["execution"].(map[string]any)["workingDirectory"] = dir
	path := filepath.Join(dir, "execution.json")
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	publishedDirectMockCommands(t, server, dir, path, environment)
	if denied.calls.Load() != 0 || nativeCalls.Load() != 0 {
		t.Fatalf("mock example attempted native effects: commands=%d subprocess/ACP=%d", denied.calls.Load(), nativeCalls.Load())
	}
}

func publishedDirectMockCommands(t *testing.T, server *support.FunctionalAPIServer, dir, path string, environment []string) {
	t.Helper()
	execute := func(args ...string) string {
		inputs := support.FakeInputs(t.Context(), append([]string{"you", "--server", server.URL(), "--json"}, args...))
		inputs.Input.WorkingDirectory, inputs.Input.Env = dir, environment
		if err := server.Execute(t, inputs.Input); err != nil || inputs.Stderr() != "" {
			t.Fatalf("%q: %v stderr=%s", args, err, inputs.Stderr())
		}
		return inputs.Stdout()
	}
	execute("factory", "config", "validate", filepath.Join(dir, "factory.json"))
	execute("worker-sessions", "--help")
	body := execute("--remote", "worker-sessions", "invoke", "--execution", path, "--user-message", "Reply with DIRECT_EXAMPLE_OK.", "--async")
	var admitted struct {
		Accepted bool   `json:"accepted"`
		ID       string `json:"workerSessionId"`
	}
	if err := json.Unmarshal([]byte(body), &admitted); err != nil || !admitted.Accepted || admitted.ID != "direct-example-session" {
		t.Fatalf("mock canonical admission: %v %s", err, body)
	}
	execute("worker-sessions", "show", "--worker-session-id", admitted.ID)
	// Capture is asynchronous. Only this private session's public committed
	// logs can establish synthetic terminal output; provider gates cannot.
	logs, err := support.WaitForObservation(30*time.Second, func() (string, error) {
		return execute("worker-sessions", "read", "--worker-session-id", admitted.ID, "--view", "logs"), nil
	}, func(body string) bool { return strings.Contains(body, `"health":"COMPLETE"`) })
	if err != nil || !strings.Contains(logs, "mock worker accepted") || !strings.Contains(logs, admitted.ID) {
		t.Fatalf("captured synthetic result: %v %s", err, logs)
	}
	if body := execute("worker-sessions", "show", "--worker-session-id", admitted.ID); !strings.Contains(body, `"state":"COMPLETED"`) || !strings.Contains(body, admitted.ID) {
		t.Fatalf("mock terminal show: %s", body)
	}
	execute("server", "stop")
	server.Close(t) // joins the private host invocation and all owned resources
}

func publishedDirectMockFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name":         "direct-demo",
		"workTypes":    []map[string]any{{"name": "task", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}}}},
		"workers":      []map[string]string{{"name": "processor"}},
		"workstations": []map[string]any{{"name": "process-task", "worker": "processor", "inputs": []map[string]string{{"workType": "task", "state": "init"}}, "outputs": []map[string]string{{"workType": "task", "state": "complete"}}, "onFailure": []map[string]string{{"workType": "task", "state": "failed"}}}},
	})
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: AGENT_WORKER\nmodelProvider: CODEX\nmodel: gpt-5\nexecutorProvider: SCRIPT_WRAP\n---\nReturn the requested short reply.\n")
	path := filepath.Join(dir, "workstations", "process-task", "AGENTS.md")
	if err := os.WriteFile(path, []byte("---\ntype: AGENT_RUN\n---\nReturn the requested short reply.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Keep this docs fixture in the test compilation rather than the production graph.
func publishedDirectExecutionDocument(t testing.TB) map[string]any {
	t.Helper()
	data, err := os.ReadFile(testutil.MustRepoPath(t, "docs/reference/operations.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(strings.ReplaceAll(string(data), "\r\n", "\n"), "Save this complete request as `execution.json`:")
	if !ok {
		t.Fatal("published execution request is missing")
	}
	_, block, ok := strings.Cut(section, "```json\n")
	if !ok {
		t.Fatal("published execution JSON is missing")
	}
	block, _, ok = strings.Cut(block, "\n```")
	if !ok {
		t.Fatal("published execution JSON is incomplete")
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(block), &document); err != nil {
		t.Fatalf("published execution JSON: %v", err)
	}
	return document
}

func publishedDirectMockHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	configDir := filepath.Join(home, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"gpt-5"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}
