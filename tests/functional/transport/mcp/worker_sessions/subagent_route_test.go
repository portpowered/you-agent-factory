package workersessions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// One root-built host owns the dedicated profile. Document edits run before
// parallel clients because those edits are intentionally process-wide. Each
// START leaf owns its directory, identity and held-open command observation.
func TestSubagentRouteDefaults(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "mcp/mcp.tool.you.subagent", "rest/startWorkerSession")
	client := newSelectedHostClientProcess(t)
	home, workDir := t.TempDir(), t.TempDir()
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	released := make(chan struct{})
	commands := make(map[string]chan platformprocess.CommandRequest)
	for _, name := range []string{"default", "model-override", "codex", "claude", "agy"} {
		commands[name] = make(chan platformprocess.CommandRequest, 2)
	}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: support.ScaffoldSingleStepFactory(t, "subagent-route"), WorkingDirectory: workDir,
		Edges: serviceedges.Edges{
			FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
			ProviderCommandRunner:              subagentRouteRunner{commands: commands, released: released},
		},
	})
	t.Cleanup(func() { close(released) })
	session, ctx := startMCP(t, client, host.URL())
	assertSubagentInvalidDefaults(t, ctx, session, configPath)
	writeSubagentProfile(t, configPath, map[string]any{"provider": "codex", "model": "dedicated-model"})
	assertSubagentExplicitRouteFailures(t, ctx, session)
	for _, cell := range []struct{ name, provider, model, effort string }{
		{"default", "", "", ""}, {"model-override", "", "override-model", "high"},
		{"codex", "codex", "explicit-codex", "high"}, {"claude", "claude", "explicit-claude", "high"},
		{"agy", "antigravity", "gemini-3.1-pro-high", ""},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			fresh, leafCtx := startMCP(t, client, host.URL())
			directory := filepath.Join(workDir, cell.name)
			if err := os.MkdirAll(directory, 0o755); err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"requestId": cell.name + "-request", "workerSessionId": cell.name,
				"prompt": "controlled route prompt", "workingRoot": directory}
			for key, value := range map[string]string{"provider": cell.provider, "model": cell.model, "reasoningEffort": cell.effort} {
				if value != "" {
					args[key] = value
				}
			}
			result := callTool(t, leafCtx, fresh, "you.subagent", args)
			assertSubagentAdmission(t, result, cell.name)
			select {
			case command := <-commands[cell.name]:
				assertSubagentRouteCommand(t, command, cell.provider, cell.model, cell.effort)
			case <-leafCtx.Done():
				t.Fatal(leafCtx.Err())
			}
			read := callTool(t, leafCtx, fresh, "you.subagent", map[string]any{"action": "READ", "workerSessionId": cell.name})
			if read.IsError || !strings.Contains(read.Content[0].(*mcp.TextContent).Text, `"state":"RUNNING"`) {
				t.Fatalf("active selected route: %s", read.Content)
			}
			assertSubagentProfileSiblings(t, configPath)
		})
	}
}

func assertSubagentAdmission(t *testing.T, result *mcp.CallToolResult, id string) {
	t.Helper()
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("START result: %+v", result)
	}
	encoded := result.Content[0].(*mcp.TextContent).Text
	var envelope struct {
		Result struct {
			ID       string `json:"workerSessionId"`
			Accepted bool   `json:"accepted"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(encoded), &envelope); err != nil {
		t.Fatal(err)
	}
	if result.IsError || !envelope.Result.Accepted || envelope.Result.ID != id {
		t.Fatalf("START admission: %s", encoded)
	}
}

func assertSubagentInvalidDefaults(t *testing.T, ctx context.Context, session *mcp.ClientSession, path string) {
	t.Helper()
	// Successive fixture edits model an operator correcting one document; no
	// provider is launched in this phase, and parallel leaves start afterward.
	for _, cell := range []struct {
		name  string
		route any
	}{
		{"missing", nil},
		{"null", json.RawMessage("null")},
		{"array", []any{}},
		{"missing-provider", map[string]any{"model": "model"}},
		{"incomplete", map[string]any{"provider": "codex"}},
		{"empty-model", map[string]any{"provider": "codex", "model": " "}},
		{"null-model", map[string]any{"provider": "codex", "model": nil}},
		{"numeric-model", map[string]any{"provider": "codex", "model": 1}},
		{"unknown-property", map[string]any{"provider": "codex", "model": "model", "paidFallback": true}},
		{"invalid-effort", map[string]any{"provider": "codex", "model": "model", "reasoningEffort": "unsupported"}},
	} {
		writeSubagentProfile(t, path, cell.route)
		result := callTool(t, ctx, session, "you.subagent", map[string]any{
			"requestId": cell.name + "-request", "workerSessionId": cell.name, "prompt": "must not run",
		})
		assertToolError(t, result, "worker_session.invalid_request", false)
		assertToolError(t, callTool(t, ctx, session, "you.subagent", map[string]any{
			"action": "READ", "workerSessionId": cell.name,
		}), "worker_session.not_found", false)
		assertSubagentProfileSiblings(t, path)
	}
}

func assertSubagentExplicitRouteFailures(t *testing.T, ctx context.Context, session *mcp.ClientSession) {
	t.Helper()
	for _, cell := range []struct{ name, provider, model, effort string }{
		{"unknown-provider", "unknown-provider", "model", ""},
		{"different-provider-without-model", "claude", "", ""},
		{"claude-minimal", "claude", "model", "minimal"},
		{"agy-effort", "antigravity", "gemini-3.1-pro-high", "high"},
	} {
		args := map[string]any{"requestId": cell.name + "-request", "workerSessionId": cell.name,
			"prompt": "must not run", "provider": cell.provider}
		if cell.model != "" {
			args["model"] = cell.model
		}
		if cell.effort != "" {
			args["reasoningEffort"] = cell.effort
		}
		result := callTool(t, ctx, session, "you.subagent", args)
		assertToolError(t, result, "worker_session.invalid_request", false)
		assertToolError(t, callTool(t, ctx, session, "you.subagent", map[string]any{
			"action": "READ", "workerSessionId": cell.name,
		}), "worker_session.not_found", false)
	}
}

func writeSubagentProfile(t *testing.T, path string, route any) {
	t.Helper()
	defaults := map[string]any{"workerModelProvider": "codex", "workerModel": "generic-model", "futureDefault": "retained"}
	if route != nil {
		defaults["subagent"] = route
	}
	data, err := json.Marshal(map[string]any{"defaults": defaults, "futureRoot": map[string]any{"owner": "retained"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertSubagentProfileSiblings(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var profile struct {
		Defaults struct {
			Provider string `json:"workerModelProvider"`
			Model    string `json:"workerModel"`
			Future   string `json:"futureDefault"`
		} `json:"defaults"`
		Future map[string]string `json:"futureRoot"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Defaults.Provider != "codex" || profile.Defaults.Model != "generic-model" ||
		profile.Defaults.Future != "retained" || profile.Future["owner"] != "retained" {
		t.Fatalf("START changed generic defaults or siblings: %s", data)
	}
}

func assertSubagentRouteCommand(t *testing.T, command platformprocess.CommandRequest, provider, model, effort string) {
	t.Helper()
	if provider == "" {
		provider = "codex"
	}
	if model == "" {
		model = "dedicated-model"
	}
	args := strings.Join(command.Args, " ")
	executable := provider
	if provider == "antigravity" {
		executable = "agy"
	}
	if command.Command != executable || !strings.Contains(args, "--model "+model) ||
		!strings.Contains(args+string(command.Stdin), "controlled route prompt") {
		t.Fatalf("provider route = %q %q", command.Command, command.Args)
	}
	if provider == "codex" && effort != "" && !strings.Contains(args, `model_reasoning_effort="`+effort+`"`) {
		t.Fatalf("codex effort = %q", command.Args)
	}
	if provider == "claude" && !strings.Contains(args, "--effort "+effort) {
		t.Fatalf("claude effort = %q", command.Args)
	}
}

type subagentRouteRunner struct {
	commands map[string]chan platformprocess.CommandRequest
	released <-chan struct{}
}

func (r subagentRouteRunner) Run(ctx context.Context, command platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	observed := r.commands[filepath.Base(command.WorkDir)]
	if observed == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider execution in %q", command.WorkDir)
	}
	observed <- command
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	case <-r.released:
		return platformprocess.CommandResult{Stdout: subagentRouteOutput(command.Command)}, nil
	}
}

func subagentRouteOutput(command string) []byte {
	switch command {
	case "claude":
		return []byte(`{"type":"result","subtype":"success","is_error":false,"result":"controlled route completed","session_id":"controlled-claude"}` + "\n")
	case "agy":
		return []byte(`{"event":"result","result":{"status":"SUCCESS","response":"controlled route completed","duration_seconds":0,"num_turns":1,"usage":{"input_tokens":0,"output_tokens":0,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":0}}}` + "\n")
	default:
		return support.CodexSuccessStdout("controlled route completed")
	}
}
