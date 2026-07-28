package acp_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

const goldenACPModeEnvironment = "YOU_TEST_ACP_GOLDEN_MODE"

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func goldenACPCommandFactory(starts *atomic.Int32) platformprocess.CommandFactory {
	return func(name string, args ...string) *exec.Cmd {
		if name == "cursor-agent" && len(args) == 1 && args[0] == "acp" {
			starts.Add(1)
			return exec.Command(os.Args[0], "-test.run=^TestACPGoldenRPCPeerProcess$")
		}
		return exec.Command(name, args...)
	}
}

func TestACPGoldenRPCPeerProcess(t *testing.T) {
	mode := os.Getenv(goldenACPModeEnvironment)
	if mode == "" {
		return
	}
	peer := goldenRPCPeer{
		mode:    mode,
		scanner: bufio.NewScanner(os.Stdin),
		writer:  bufio.NewWriter(os.Stdout),
	}
	if err := peer.serve(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

type goldenRPCPeer struct {
	mode    string
	scanner *bufio.Scanner
	writer  *bufio.Writer
}

func (p *goldenRPCPeer) serve() error {
	for p.scanner.Scan() {
		var request rpcEnvelope
		if err := json.Unmarshal(p.scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode client RPC: %w", err)
		}
		switch request.Method {
		case "initialize":
			if err := validateInitializeParams(request.Params); err != nil {
				return err
			}
			if err := p.respondGolden(request.ID, "initialize_response.json"); err != nil {
				return err
			}
		case "session/new":
			if err := validateNewSessionParams(request.Params); err != nil {
				return err
			}
			if p.mode == "new-fail" {
				return p.respondError(request.ID, -32603, "golden session/new failure")
			}
			result := json.RawMessage(`{"sessionId":"sess_abc123def456","configOptions":[{"type":"select","id":"model","name":"Model","category":"model","currentValue":"default","options":[{"name":"Test model","value":"test-model"}]}]}`)
			if err := p.respond(request.ID, result); err != nil {
				return err
			}
		case "session/set_config_option":
			if p.mode == "config-fail" {
				return p.respondError(request.ID, -32602, "golden model config failure")
			}
			if err := validateModelConfigParams(request.Params); err != nil {
				return err
			}
			if err := p.respond(request.ID, json.RawMessage(`{"configOptions":[]}`)); err != nil {
				return err
			}
		case "session/prompt":
			if err := validatePromptParams(request.Params); err != nil {
				return err
			}
			if strings.HasPrefix(p.mode, "permission-") {
				if err := p.permissionRoundTrip(); err != nil {
					return err
				}
			}
			if err := p.publishGoldenUpdates(); err != nil {
				return err
			}
			if err := p.respond(request.ID, json.RawMessage(`{"stopReason":"end_turn"}`)); err != nil {
				return err
			}
			return nil
		case "$/cancel_request", "session/cancel":
			return nil
		default:
			return fmt.Errorf("unexpected client RPC method %q", request.Method)
		}
	}
	if err := p.scanner.Err(); err != nil {
		return fmt.Errorf("read client RPC: %w", err)
	}
	return nil
}

func validateInitializeParams(raw json.RawMessage) error {
	var params struct {
		ProtocolVersion int             `json:"protocolVersion"`
		Capabilities    json.RawMessage `json:"clientCapabilities"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("decode initialize params: %w", err)
	}
	if params.ProtocolVersion != 1 || len(params.Capabilities) == 0 {
		return fmt.Errorf("initialize params = %s, want protocolVersion 1 and clientCapabilities", raw)
	}
	return nil
}

func validateNewSessionParams(raw json.RawMessage) error {
	var params struct {
		Cwd        string            `json:"cwd"`
		MCPServers []json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("decode session/new params: %w", err)
	}
	want, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	if params.Cwd != want || params.MCPServers == nil || len(params.MCPServers) != 0 {
		return fmt.Errorf("session/new params = %s, want cwd %q and an explicit empty mcpServers list", raw, want)
	}
	if os.Getenv("YOU_ACP_GOLDEN_SENTINEL") != "preserved" {
		return fmt.Errorf("ACP subprocess environment omitted invocation sentinel")
	}
	return nil
}

func validateModelConfigParams(raw json.RawMessage) error {
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("decode model config params: %w", err)
	}
	if params["sessionId"] != "sess_abc123def456" || params["configId"] != "model" || params["value"] != "test-model" {
		return fmt.Errorf("model config params = %s", raw)
	}
	return nil
}

func validatePromptParams(raw json.RawMessage) error {
	var params struct {
		SessionID string `json:"sessionId"`
		Prompt    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"prompt"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("decode prompt params: %w", err)
	}
	if params.SessionID != "sess_abc123def456" || len(params.Prompt) == 0 {
		return fmt.Errorf("prompt params = %s", raw)
	}
	var text string
	for _, block := range params.Prompt {
		if block.Type == "text" {
			text += block.Text
		}
	}
	if !strings.Contains(text, "ACP worker") || !strings.Contains(text, "Test workstation") {
		return fmt.Errorf("prompt did not carry canonical work and worker instructions: %q", text)
	}
	return nil
}

func (p *goldenRPCPeer) permissionRoundTrip() error {
	params, err := acpGoldenFiles.ReadFile("testdata/json_golden/upstream/request_permission_request.json")
	if err != nil {
		return err
	}
	if err := p.write(rpcEnvelope{JSONRPC: "2.0", ID: json.RawMessage(`"permission-1"`), Method: "session/request_permission", Params: params}); err != nil {
		return err
	}
	if !p.scanner.Scan() {
		return fmt.Errorf("permission request received no response")
	}
	var response rpcEnvelope
	if err := json.Unmarshal(p.scanner.Bytes(), &response); err != nil {
		return fmt.Errorf("decode permission response: %w", err)
	}
	var result struct {
		Outcome struct {
			Outcome  string `json:"outcome"`
			OptionID string `json:"optionId"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return fmt.Errorf("decode permission result: %w", err)
	}
	want := "reject-once"
	if p.mode == "permission-allow" {
		want = "allow-once"
	}
	if result.Outcome.Outcome != "selected" || result.Outcome.OptionID != want {
		return fmt.Errorf("permission response = %s, want selected %q", response.Result, want)
	}
	return nil
}

func (p *goldenRPCPeer) publishGoldenUpdates() error {
	for _, name := range []string{
		"session_update_agent_message_chunk.json",
		"session_update_agent_thought_chunk.json",
		"session_update_plan.json",
		"session_update_tool_call_edit.json",
		"session_update_tool_call_update_more_fields.json",
	} {
		update, err := acpGoldenFiles.ReadFile("testdata/json_golden/upstream/" + name)
		if err != nil {
			return err
		}
		params, err := json.Marshal(struct {
			SessionID string          `json:"sessionId"`
			Update    json.RawMessage `json:"update"`
		}{SessionID: "sess_abc123def456", Update: update})
		if err != nil {
			return err
		}
		if err := p.write(rpcEnvelope{JSONRPC: "2.0", Method: "session/update", Params: params}); err != nil {
			return err
		}
	}
	for _, params := range []json.RawMessage{
		json.RawMessage(`{"sessionId":"sess_abc123def456","update":{"sessionUpdate":"usage_update","used":17,"size":4096}}`),
		json.RawMessage(`{"sessionId":"sess_abc123def456","update":{"sessionUpdate":"session_info_update","title":"Golden ACP session","updatedAt":"2026-07-28T00:00:00Z"}}`),
	} {
		if err := p.write(rpcEnvelope{JSONRPC: "2.0", Method: "session/update", Params: params}); err != nil {
			return err
		}
	}
	finalParams := json.RawMessage(`{"sessionId":"sess_abc123def456","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":" golden ACP COMPLETE"}}}`)
	return p.write(rpcEnvelope{JSONRPC: "2.0", Method: "session/update", Params: finalParams})
}

func (p *goldenRPCPeer) respondGolden(id json.RawMessage, name string) error {
	result, err := acpGoldenFiles.ReadFile("testdata/json_golden/upstream/" + name)
	if err != nil {
		return err
	}
	return p.respond(id, result)
}

func (p *goldenRPCPeer) respond(id, result json.RawMessage) error {
	return p.write(rpcEnvelope{JSONRPC: "2.0", ID: id, Result: result})
}

func (p *goldenRPCPeer) respondError(id json.RawMessage, code int, message string) error {
	return p.write(rpcEnvelope{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func (p *goldenRPCPeer) write(message rpcEnvelope) error {
	if err := json.NewEncoder(p.writer).Encode(message); err != nil {
		return err
	}
	return p.writer.Flush()
}
