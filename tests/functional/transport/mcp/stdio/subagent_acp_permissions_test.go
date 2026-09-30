package stdio_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	mcpACPTestCommand = "mcp-custom-acp"
	mcpACPMarkerArg   = "--mcp-acp-permission-marker="
)

// TestMCPSubagentCustomACPHandlesPermissionRequest proves the public
// you.subagent tool routes a custom ACP permission request through the
// noninteractive client policy and returns the provider completion.
func TestMCPSubagentCustomACPHandlesPermissionRequest(t *testing.T) {
	projectRoot := t.TempDir()
	homeDir := t.TempDir()
	markerPath := filepath.Join(t.TempDir(), "permission-outcome.txt")
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create isolated operator config directory: %v", err)
	}
	config := []byte(`{"workers":{"acp":{"integrations":[{"id":"mcp-test-acp","name":"mcp-test-acp","transport":"stdio","command":"mcp-custom-acp acp"}]}}}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write isolated operator config: %v", err)
	}

	launcher := process.CommandFactory(func(name string, args ...string) *exec.Cmd {
		if name == mcpACPTestCommand && len(args) == 1 && args[0] == "acp" {
			childArgs := []string{
				"-test.run=^TestMCPSubagentCustomACPPermissionPeerProcess$",
				"--",
				mcpACPMarkerArg + markerPath,
			}
			return exec.Command(os.Args[0], childArgs...)
		}
		return exec.Command(name, args...)
	})
	processRoot, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		PlatformProcessCommandFactory: launcher,
		ProvidersExecutableLocator:    mcpACPAvailableExecutableLocator{},
	})
	if err != nil {
		t.Fatalf("build root application process: %v", err)
	}
	server := startRuntimeBackedMCPServerWithProcess(t, processRoot, projectRoot, homeDir)
	initializeMCPClient(t, server.client)

	response := server.client.call("tools/call", map[string]any{
		"name": "you.subagent",
		"arguments": map[string]any{
			"prompt":   "Return the permission witness.",
			"provider": "mcp-test-acp",
			"model":    "test-model",
		},
	})
	if response.Error != nil {
		t.Fatalf("you.subagent JSON-RPC error = %#v", response.Error)
	}
	if response.Result == nil {
		t.Fatal("you.subagent returned no tool result")
	}
	toolText := mcpToolResultText(t, response.Result)
	if !strings.Contains(toolText, "permission witness complete") {
		t.Fatalf("you.subagent result text = %q, want the fake ACP completion", toolText)
	}

	outcome, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read custom ACP permission outcome: %v", err)
	}
	if got := strings.TrimSpace(string(outcome)); got != "allow-once" {
		t.Fatalf("custom ACP permission outcome = %q, want allow-once after handling the request", got)
	}
}

func startRuntimeBackedMCPServerWithProcess(
	t *testing.T,
	processRoot support.ApplicationProcess,
	projectRoot string,
	homeDir string,
) *stdioMCPServer {
	t.Helper()
	support.CleanupProcess(t, processRoot)
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("create MCP stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		t.Fatalf("create MCP stdout pipe: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	env := builtcliacceptance.ProcessEnvForIsolatedHome(homeDir)
	go func() {
		serveErr <- processRoot.Execute(root.Input{
			Args:             []string{"you", "server", "mcp", "--project-root", projectRoot},
			Env:              env,
			Stdin:            stdinRead,
			Stdout:           stdoutWrite,
			Stderr:           io.Discard,
			Context:          ctx,
			WorkingDirectory: projectRoot,
		})
	}()
	server := &stdioMCPServer{
		t:            t,
		home:         homeDir,
		client:       newStdioMCPClient(t, stdinWrite, stdoutRead),
		stdin:        stdinWrite,
		stdinRead:    stdinRead,
		stdout:       bufio.NewReader(stdoutRead),
		stdoutRead:   stdoutRead,
		stdoutWrite:  stdoutWrite,
		serveErr:     serveErr,
		cancel:       cancel,
		serveDone:    make(chan struct{}),
		shutdownDone: make(chan struct{}),
	}
	t.Cleanup(server.cleanup)
	return server
}

func mcpToolResultText(t *testing.T, result map[string]any) string {
	t.Helper()
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("tool result content = %#v, want non-empty content", result["content"])
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("tool result content item = %#v, want object", content[0])
	}
	text, _ := item["text"].(string)
	if text == "" {
		t.Fatalf("tool result content item text = %#v, want non-empty text", item["text"])
	}
	return text
}

type mcpACPEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

type mcpACPAvailableExecutableLocator struct{}

func (mcpACPAvailableExecutableLocator) LookPath(file string) (string, error) {
	return file, nil
}

// TestMCPSubagentCustomACPPermissionPeerProcess runs only as the real child
// process launched through the injected command-factory edge.
func TestMCPSubagentCustomACPPermissionPeerProcess(t *testing.T) {
	markerPath := ""
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, mcpACPMarkerArg) {
			markerPath = strings.TrimPrefix(arg, mcpACPMarkerArg)
		}
	}
	if markerPath == "" {
		return
	}
	peer := mcpACPPermissionPeer{scanner: bufio.NewScanner(os.Stdin), writer: bufio.NewWriter(os.Stdout), markerPath: markerPath}
	peer.scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	if err := peer.serve(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

type mcpACPPermissionPeer struct {
	scanner    *bufio.Scanner
	writer     *bufio.Writer
	markerPath string
}

func (peer *mcpACPPermissionPeer) serve() error {
	for peer.scanner.Scan() {
		var request mcpACPEnvelope
		if err := json.Unmarshal(peer.scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode ACP request: %w", err)
		}
		switch request.Method {
		case "initialize":
			if err := peer.respond(request.ID, `{"protocolVersion":1,"agentCapabilities":{},"authMethods":[]}`); err != nil {
				return err
			}
		case "session/new":
			if err := peer.respond(request.ID, `{"sessionId":"mcp-permission-session","configOptions":[{"type":"select","id":"model","name":"Model","category":"model","currentValue":"default","options":[{"name":"Test model","value":"test-model"}]}]}`); err != nil {
				return err
			}
		case "session/set_config_option":
			var params struct {
				Value string `json:"value"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return fmt.Errorf("decode model selection: %w", err)
			}
			if params.Value != "test-model" {
				return fmt.Errorf("selected model = %q, want test-model", params.Value)
			}
			if err := peer.respond(request.ID, `{"configOptions":[]}`); err != nil {
				return err
			}
		case "session/prompt":
			if err := peer.write(mcpACPEnvelope{
				JSONRPC: "2.0", ID: json.RawMessage(`"permission-request"`), Method: "session/request_permission",
				Params: json.RawMessage(`{"sessionId":"mcp-permission-session","toolCall":{"toolCallId":"permission-tool-call"},"options":[{"optionId":"allow-once","name":"Allow once","kind":"allow_once"},{"optionId":"reject-once","name":"Reject","kind":"reject_once"}]}`),
			}); err != nil {
				return err
			}
			if !peer.scanner.Scan() {
				return fmt.Errorf("permission request received no client response: %w", peer.scanner.Err())
			}
			var permissionResponse mcpACPEnvelope
			if err := json.Unmarshal(peer.scanner.Bytes(), &permissionResponse); err != nil {
				return fmt.Errorf("decode permission response: %w", err)
			}
			if string(permissionResponse.ID) != `"permission-request"` {
				return fmt.Errorf("permission response id = %s, want permission-request", permissionResponse.ID)
			}
			var selection struct {
				Outcome struct {
					OptionID string `json:"optionId"`
				} `json:"outcome"`
			}
			if err := json.Unmarshal(permissionResponse.Result, &selection); err != nil {
				return fmt.Errorf("decode permission selection: %w", err)
			}
			if err := os.WriteFile(peer.markerPath, []byte(selection.Outcome.OptionID), 0o600); err != nil {
				return fmt.Errorf("record permission selection: %w", err)
			}
			update := mcpACPEnvelope{
				JSONRPC: "2.0", Method: "session/update",
				Params: json.RawMessage(`{"sessionId":"mcp-permission-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"permission witness complete"}}}`),
			}
			if err := peer.write(update); err != nil {
				return err
			}
			if err := peer.respond(request.ID, `{"stopReason":"end_turn"}`); err != nil {
				return err
			}
			return nil
		case "$/cancel_request", "session/cancel":
			return nil
		default:
			return fmt.Errorf("unexpected ACP method %q", request.Method)
		}
	}
	return peer.scanner.Err()
}

func (peer *mcpACPPermissionPeer) respond(id json.RawMessage, result string) error {
	return peer.write(mcpACPEnvelope{JSONRPC: "2.0", ID: id, Result: json.RawMessage(result)})
}

func (peer *mcpACPPermissionPeer) write(message mcpACPEnvelope) error {
	if err := json.NewEncoder(peer.writer).Encode(message); err != nil {
		return err
	}
	return peer.writer.Flush()
}
