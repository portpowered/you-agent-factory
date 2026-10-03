package stdio_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessionmcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// M01/M08: each connection selects its own working root and owns its streams
// and profile. The immutable command edge attributes results by working root,
// so simultaneous invocations need no mutable provider selector or long lock.
func TestMCPStartSyncRunsFactorySessionThroughComposedProcess(t *testing.T) {
	t.Parallel()
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: mcpRootResultRunner{}})
	t.Run("completion and ordered events", func(t *testing.T) {
		t.Parallel()
		server := startComposedMemoryMCP(t, process)
		initializeMCPClient(t, server.client)
		sessionID := assertComposedMCPSync(t, server, "first")
		assertComposedMCPEvents(t, server.client, sessionID)
		server.closeInput(t)
	})
	t.Run("closing one selection leaves its peer usable", func(t *testing.T) {
		t.Parallel()
		first := startComposedMemoryMCP(t, process)
		second := startComposedMemoryMCP(t, process)
		initializeMCPClient(t, first.client)
		initializeMCPClient(t, second.client)
		firstID := assertComposedMCPSync(t, first, "first")
		secondID := assertComposedMCPSync(t, second, "second")
		if firstID == secondID {
			t.Fatal("isolated requests shared a Factory Session identity")
		}
		first.closeInput(t)
		assertComposedMCPEvents(t, second.client, secondID)
		assertComposedMCPSync(t, second, "after-peer-close")
		second.closeInput(t)
	})
}

type mcpRootResultRunner struct{}

func (mcpRootResultRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("completed at " + filepath.Clean(req.WorkDir))}, nil
}

type composedMemoryMCP struct {
	client *stdioMCPClient
	root   string
	input  *io.PipeWriter
	done   <-chan error
}

func startComposedMemoryMCP(t *testing.T, process support.ApplicationProcess) *composedMemoryMCP {
	t.Helper()
	projectRoot := support.ScaffoldSingleStepFactory(t, "composed-mcp")
	stdin, input := io.Pipe()
	output, stdout := io.Pipe()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	done := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		_ = input.Close()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = output.Close()
	})
	home := t.TempDir()
	go func() {
		err := process.Execute(root.Input{
			Args:             []string{"you", "server", "mcp", "--project-root", projectRoot},
			Env:              builtcliacceptance.ProcessEnvForIsolatedHome(home),
			WorkingDirectory: projectRoot, Context: ctx,
			Stdin: stdin, Stdout: stdout, Stderr: io.Discard,
		})
		_ = stdout.Close()
		done <- err
	}()
	return &composedMemoryMCP{client: newStdioMCPClient(t, input, output), root: projectRoot, input: input, done: done}
}

func (server *composedMemoryMCP) closeInput(t *testing.T) {
	t.Helper()
	if err := server.input.Close(); err != nil {
		t.Fatalf("close caller input: %v", err)
	}
	// Completion is observed directly; this deadline only bounds a stuck test.
	select {
	case err := <-server.done:
		if err != nil {
			t.Fatalf("Process.Execute after input close: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Process.Execute did not finish after caller input closed")
	}
}

func assertComposedMCPSync(t *testing.T, server *composedMemoryMCP, requestID string) string {
	t.Helper()
	workflow := `return (async function () { return await agent.run({prompt: "complete", modelProvider: "codex", model: "gpt-5-codex"}); })();`
	response := server.client.call("tools/call", map[string]any{
		"name": factorysessionmcp.ToolStartSync,
		"arguments": map[string]any{
			"requestId": requestID + "-" + uuid.NewString(),
			"source":    map[string]any{"kind": "INLINE_WORKFLOW", "inlineWorkflow": map[string]any{"inlineSource": map[string]any{"encoding": "utf-8", "inline": workflow}}},
		},
	})
	result := decodeComposedTool[factoryapi.FactorySessionSyncExecutionResponse](t, response)
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted || result.SessionId == "" {
		t.Fatalf("start_sync = %#v, want COMPLETED and session identity", result)
	}
	encoded, err := json.Marshal(result.Result)
	if err != nil || !strings.Contains(string(encoded), "completed at ") || !strings.Contains(string(encoded), strings.ReplaceAll(server.root, `\`, `\\`)) {
		t.Fatalf("start_sync result = %s (%v), want provider output from working root %q", encoded, err, server.root)
	}
	return result.SessionId
}

func assertComposedMCPEvents(t *testing.T, client *stdioMCPClient, sessionID string) {
	t.Helper()
	result := decodeComposedTool[factorysessionmcp.ReadEventsResult](t, client.call("tools/call", map[string]any{
		"name": factorysessionmcp.ToolReadEvents, "arguments": map[string]any{"sessionId": sessionID},
	}))
	if result.SessionID != sessionID || len(result.Events) == 0 {
		t.Fatalf("read_events = %#v, want nonempty events for %q", result, sessionID)
	}
	for index, event := range result.Events {
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			t.Fatalf("event %d belongs to %#v, want %q", index, event.Context.SessionId, sessionID)
		}
		if index > 0 && event.Context.Sequence <= result.Events[index-1].Context.Sequence {
			t.Fatalf("events are not ordered at %d: %#v", index, result.Events)
		}
	}
}

func decodeComposedTool[T any](t *testing.T, response mcpJSONRPCResponse) T {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("MCP protocol error: %#v", response.Error)
	}
	var envelope factorysessionmcp.ToolResponse[T]
	encoded, err := json.Marshal(response.Result["structuredContent"])
	if err != nil {
		t.Fatalf("encode structured tool result: %v", err)
	}
	if string(encoded) == "null" {
		encoded = []byte(mcpToolResultText(t, response.Result))
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("decode tool result: %v; response=%#v", err, response.Result)
	}
	if envelope.Error != nil || envelope.Result == nil {
		t.Fatal(fmt.Sprintf("MCP tool outcome = %s, want success", encoded))
	}
	return *envelope.Result
}
