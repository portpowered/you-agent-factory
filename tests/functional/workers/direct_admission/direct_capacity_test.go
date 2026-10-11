package directadmission_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The bound is host-wide, so this isolated cohort shares one root host and one
// MCP connection while its two admitted providers coexist. Transport parity is
// the customer contract; all unsafe effects enter through command-runner Edges.
func TestDirectCapacityAdmission(t *testing.T) {
	t.Parallel()
	client := newSelectedHostClientProcess(t)
	home, workDir := t.TempDir(), t.TempDir()
	path := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"defaults":{"directSessionCapacity":2}}`), 0600); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	launched := make(chan platformprocess.CommandRequest, 2)
	var calls atomic.Int32
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: support.ScaffoldSingleStepFactory(t, "direct-capacity"), WorkingDirectory: workDir,
		Edges: serviceedges.Edges{FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
			ProviderCommandRunner: capacityProvider{launched: launched, released: release, calls: &calls},
			ProviderCatalogProbe: func(_ context.Context, d providers.Descriptor) (providers.Descriptor, error) {
				d.Readiness = providers.ReadinessReady
				return d, nil
			}},
	})
	t.Cleanup(func() { close(release) })
	baseline := capacityFactoryIDs(t, host.URL())
	session, ctx, cancel := startCancellableMCP(t, client, host.URL(), workDir)
	args := func(id string) map[string]any {
		return map[string]any{"requestId": id, "workerSessionId": id, "prompt": "controlled", "provider": "codex", "model": "controlled-model", "reasoningEffort": "high", "workingRoot": workDir}
	}
	type response struct {
		result *mcp.CallToolResult
		err    error
	}
	admitted := make(chan response, 2)
	for _, id := range []string{"first", "second"} {
		go func() {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: args(id)})
			admitted <- response{result, err}
		}()
	}
	for range 2 {
		r := <-admitted
		if r.err != nil || r.result.IsError {
			t.Fatalf("concurrent START=%+v", r)
		}
	}
	for range 2 {
		command := <-launched
		if command.Command != "codex" || command.WorkDir != workDir {
			t.Fatalf("route changed: command=%s dir=%s", command.Command, command.WorkDir)
		}
	}
	assertCapacityParity(t, client, ctx, session, host.URL(), workDir, args("excess"))
	assertSubagentAdmission(t, callTool(t, ctx, session, "you.subagent", args("first")), "first")
	conflict := args("first")
	conflict["prompt"] = "conflict"
	assertToolError(t, callTool(t, ctx, session, "you.subagent", conflict), "worker_session.conflict", false)
	cancel()
	fresh, freshCtx := startMCP(t, client, host.URL())
	read := callTool(t, freshCtx, fresh, "you.subagent", map[string]any{"action": "READ", "workerSessionId": "first"})
	if read.IsError || !strings.Contains(read.Content[0].(*mcp.TextContent).Text, `"state":"RUNNING"`) {
		t.Fatal("detached direct session lost running state")
	}
	assertToolError(t, callTool(t, freshCtx, fresh, "you.subagent", args("detached-excess")), "worker_session.capacity_exceeded", true)
	if calls.Load() != 2 {
		t.Fatalf("refusal/replay executed a provider: %d", calls.Load())
	}
	if after := capacityFactoryIDs(t, host.URL()); !reflect.DeepEqual(after, baseline) {
		t.Fatal("direct START created a Factory Session")
	}
}

func assertCapacityParity(t *testing.T, client selectedHostClientProcess, ctx context.Context, session *mcp.ClientSession, host, workDir string, args map[string]any) {
	t.Helper()
	refusal := callTool(t, ctx, session, "you.subagent", args)
	assertToolError(t, refusal, "worker_session.capacity_exceeded", true)
	var envelope struct {
		Error struct {
			Details struct {
				Host     string `json:"host"`
				Capacity int64  `json:"capacity"`
			} `json:"details"`
		} `json:"error"`
	}
	encoded, err := json.Marshal(refusal.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Details.Host != host || envelope.Error.Details.Capacity != 2 {
		t.Fatalf("capacity details=%+v", envelope.Error.Details)
	}
	postHostJSON(t, ctx, host+"/worker-sessions", args, 429)
	assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": "excess"}), "worker_session.not_found", false)
	inputs := support.FakeInputs(ctx, []string{"you", "--server", host, "--json", "worker-sessions", "start", "--request-id", "cli-excess", "--worker-session-id", "cli-excess", "--provider", "codex", "--model", "controlled-model", "--working-root", workDir, "controlled"})
	inputs.Input.WorkingDirectory = workDir
	err = client.Execute(inputs.Input)
	if err == nil || !strings.Contains(inputs.Stderr()+inputs.Stdout(), "WORKER_SESSION_DIRECT_CAPACITY_EXCEEDED") || !strings.Contains(err.Error(), "direct capacity 2") {
		t.Fatalf("CLI capacity=%v", err)
	}
}

type capacityProvider struct {
	launched chan<- platformprocess.CommandRequest
	released <-chan struct{}
	calls    *atomic.Int32
}

func (r capacityProvider) Run(ctx context.Context, command platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	r.launched <- command
	select {
	case <-r.released:
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("controlled completion")}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func capacityFactoryIDs(t *testing.T, host string) map[string]bool {
	t.Helper()
	listing := getHost(t, host+"/factory-sessions").(map[string]any)
	ids := make(map[string]bool)
	for _, value := range listing["sessions"].([]any) {
		session := value.(map[string]any)
		id, ok := session["id"].(string)
		if !ok || id == "" {
			t.Fatal("Factory Session identity missing")
		}
		ids[id] = true
	}
	return ids
}
