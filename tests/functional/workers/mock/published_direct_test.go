package mock

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type directExampleDeniedRunner struct{ calls atomic.Int32 }

func (r *directExampleDeniedRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	return platformprocess.CommandResult{}, errors.New("native execution denied for published mock example")
}

// Direct origin needs no submitted Work. This private host has its own idle
// Factory Session and profile, and its immutable accept policy differs from
// the named-mock fixture. All commands reuse its one root-built process.
func TestPublishedDirectWorkerSessionMockJourney(t *testing.T) {
	t.Parallel()
	dir := publishedDirectMockFactory(t)
	mockPath := filepath.Join(dir, "mock-workers.json")
	mockConfig := map[string]any{"unmatchedDispatchPolicy": "accept", "mockWorkers": []map[string]any{
		{"workstationName": "usage-cross", "runType": "accept", "resultBody": map[string]string{"output": "MOCK_USAGE_DIRECT_OK"}, "usage": map[string]any{"provider": "codex", "model": "gpt-5-codex", "inputTokens": 17, "outputTokens": 5, "cachedInputTokens": 0, "reasoningOutputTokens": 0}},
		{"workstationName": "usage-match", "runType": "accept", "resultBody": map[string]string{"output": "MOCK_USAGE_DIRECT_OK"}, "usage": map[string]any{"provider": "codex", "model": "gpt-5-codex", "inputTokens": 17, "outputTokens": 5, "cachedInputTokens": 0, "reasoningOutputTokens": 0}},
		{"workstationName": "usage-zero", "runType": "accept", "resultBody": map[string]string{"output": "MOCK_USAGE_DIRECT_OK"}, "usage": map[string]any{"provider": "codex", "model": "gpt-5-codex", "inputTokens": 0, "outputTokens": 5}},
		{"workstationName": "usage-omitted", "runType": "accept", "resultBody": map[string]string{"output": "MOCK_USAGE_DIRECT_OK"}, "usage": map[string]any{"provider": "codex", "model": "gpt-5-codex", "outputTokens": 5}},
		{"workstationName": "usage-none", "runType": "accept", "resultBody": map[string]string{"output": "MOCK_USAGE_DIRECT_OK"}},
	}}
	mockBytes, err := json.Marshal(mockConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mockPath, mockBytes, 0o600); err != nil {
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
	t.Run("MockUsage", func(t *testing.T) {
		for _, name := range []string{"cross", "match", "zero", "omitted", "none"} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				publishedDirectUsage(t, server, dir, environment, name)
			})
		}
	})
	t.Run("MockUsagePeerCursor", func(t *testing.T) {
		assertMockUsagePeerCursor(t, server, dir, environment, "usage-cross", "usage-match")
	})
	publishedDirectMockCommands(t, server, dir, path, environment)
	if denied.calls.Load() != 0 || nativeCalls.Load() != 0 {
		t.Fatalf("mock example attempted native effects: commands=%d subprocess/ACP=%d", denied.calls.Load(), nativeCalls.Load())
	}
}

func publishedDirectUsage(t *testing.T, server *support.FunctionalAPIServer, dir string, environment []string, name string) {
	t.Helper()
	execute := func(args ...string) string {
		inputs := support.FakeInputs(t.Context(), append([]string{"you", "--server", server.URL(), "--json"}, args...))
		inputs.Input.WorkingDirectory, inputs.Input.Env = dir, environment
		if err := server.Execute(t, inputs.Input); err != nil {
			t.Fatalf("%v: %v stderr=%s", args, err, inputs.Stderr())
		}
		return inputs.Stdout()
	}
	id := "usage-" + name
	provider, model := "claude", "claude-sonnet-4-6"
	if name == "match" {
		provider, model = "codex", "gpt-5-codex"
	}
	document := publishedDirectExecutionDocument(t)
	document["workerSessionId"], document["requestId"] = id, id+"-request"
	execution := document["execution"].(map[string]any)
	execution["workingDirectory"], execution["workstationName"] = dir, id
	execution["runnerId"], execution["executorProvider"], execution["modelProvider"], execution["model"] = provider, provider, provider, model
	execution["dispatch"] = map[string]any{"dispatchId": id + "-attempt", "workstationName": id}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	execute("--remote", "worker-sessions", "invoke", "--execution", path, "--async")
	logs, err := support.WaitForObservation(30*time.Second, func() (string, error) {
		return execute("worker-sessions", "read", "--worker-session-id", id, "--view", "logs"), nil
	}, func(body string) bool { return strings.Contains(body, `"health":"COMPLETE"`) })
	if err != nil {
		t.Fatal(err)
	}
	var summary factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(execute("worker-sessions", "show", "--worker-session-id", id)), &summary); err != nil || summary.Provider == nil || *summary.Provider != provider || summary.Model == nil || *summary.Model != model || summary.FactorySessionId != nil {
		t.Fatalf("direct execution identity: %+v %v", summary, err)
	}
	assertMockUsageReadParity(t, server, dir, environment, summary, logs)
	if name == "none" {
		if summary.TokenUsage != nil || strings.Contains(logs, `"kind":"USAGE"`) || !strings.Contains(logs, "MOCK_USAGE_DIRECT_OK") {
			t.Fatalf("no declaration fabricated usage: %s", logs)
		}
		return
	}
	if summary.TokenUsage == nil {
		t.Fatalf("declared usage missing: %s", logs)
	}
	total := 22
	if name == "zero" || name == "omitted" {
		total = 5
		if summary.TokenUsage.CachedInputTokens != nil || summary.TokenUsage.ReasoningOutputTokens != nil || (summary.TokenUsage.InputTokens == nil) != (name == "omitted") {
			t.Fatalf("omitted/zero classes changed: %+v", summary.TokenUsage)
		}
		if name == "zero" {
			assertMockUsageObservationToken(t, summary.TokenUsage.InputTokens, 0, "input")
		}
	} else {
		assertMockUsageObservationToken(t, summary.TokenUsage.InputTokens, 17, "input")
		assertMockUsageObservationToken(t, summary.TokenUsage.CachedInputTokens, 0, "cached")
		assertMockUsageObservationToken(t, summary.TokenUsage.ReasoningOutputTokens, 0, "reasoning")
	}
	assertMockUsageObservationToken(t, summary.TokenUsage.OutputTokens, 5, "output")
	assertMockUsageObservationToken(t, summary.TokenUsage.TotalTokens, total, "total")
	assertCapturedMockUsage(t, logs, provider, "gpt-5-codex", "MOCK_USAGE_DIRECT_OK", int64(total))
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
