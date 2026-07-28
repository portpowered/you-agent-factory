package acp_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// functionalRPCPeer is deliberately an ACP implementation at the process
// boundary, not an acp-go-sdk Agent. The production client therefore has to
// encode, transport, and decode every message over real OS pipes in these
// tests. Keeping the peer as raw JSON-RPC also prevents shared SDK types from
// hiding wire-compatibility failures.
type functionalRPCPeer struct {
	mode       string
	scanner    *bufio.Scanner
	writer     *bufio.Writer
	stderr     io.Writer
	modelSet   bool
	sessionID  string
	nextCallID int
}

func runFunctionalRPCPeer(mode string, stdin io.Reader, stdout, stderr io.Writer) error {
	if mode == "malformed" {
		_, err := fmt.Fprintln(stdout, "{not-json")
		return err
	}
	if mode == "eof" {
		return nil
	}
	peer := &functionalRPCPeer{
		mode: mode, scanner: bufio.NewScanner(stdin), writer: bufio.NewWriter(stdout), stderr: stderr,
		sessionID: os.Getenv("YOU_TEST_ACP_SESSION_ID"),
	}
	if peer.sessionID == "" {
		peer.sessionID = "acp-session-functional-1"
	}
	if mode == "stderr" {
		_, _ = fmt.Fprintln(stderr, "agent diagnostic token="+os.Getenv("ACP_TEST_API_TOKEN"))
	}
	return peer.serve()
}

func (p *functionalRPCPeer) serve() error {
	for p.scanner.Scan() {
		var request rpcEnvelope
		if err := json.Unmarshal(p.scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode client RPC: %w", err)
		}
		switch request.Method {
		case "initialize":
			if p.mode == "init-fail" || p.mode == "stderr" {
				if err := p.respondError(request.ID, -32603, "Internal error", map[string]any{"error": "functional ACP initialization failure"}); err != nil {
					return err
				}
				continue
			}
			version := 1
			if p.mode == "version" {
				version = 999
			}
			authMethods := `[]`
			if p.mode == "auth" {
				authMethods = `[{"id":"login","name":"Agent login"}]`
			}
			result := json.RawMessage(fmt.Sprintf(`{"protocolVersion":%d,"agentCapabilities":{},"authMethods":%s}`, version, authMethods))
			if err := p.respond(request.ID, result); err != nil {
				return err
			}
		case "session/new":
			if p.mode == "auth" {
				if err := p.respondError(request.ID, -32000, "Authentication required", nil); err != nil {
					return err
				}
				continue
			}
			config := `[]`
			if p.mode == "model" {
				config = `[{"type":"select","id":"model","name":"Model","category":"model","currentValue":"default","options":[{"name":"Test model","value":"test-model"}]}]`
			}
			result := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"configOptions":%s}`, p.sessionID, config))
			if err := p.respond(request.ID, result); err != nil {
				return err
			}
		case "session/set_config_option":
			var params struct {
				Value string `json:"value"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return fmt.Errorf("decode model selection: %w", err)
			}
			p.modelSet = params.Value == "test-model"
			if err := p.respond(request.ID, json.RawMessage(`{"configOptions":[]}`)); err != nil {
				return err
			}
		case "session/prompt":
			if err := p.prompt(request); err != nil {
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

func (p *functionalRPCPeer) prompt(request rpcEnvelope) error {
	if p.mode == "block" {
		if signal := os.Getenv("YOU_TEST_ACP_PROMPT_SIGNAL"); signal != "" {
			_ = os.WriteFile(signal, []byte("prompt-started"), 0o600)
		}
		for p.scanner.Scan() {
			var message rpcEnvelope
			if err := json.Unmarshal(p.scanner.Bytes(), &message); err != nil {
				return err
			}
			if message.Method == "session/cancel" || message.Method == "$/cancel_request" {
				return p.respondError(request.ID, -32800, "Request cancelled", nil)
			}
		}
		return p.scanner.Err()
	}
	if p.mode == "resource" {
		var params struct {
			Prompt []map[string]any `json:"prompt"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return fmt.Errorf("decode resource prompt: %w", err)
		}
		found := false
		for _, block := range params.Prompt {
			found = found || (block["type"] == "resource_link" && block["uri"] == "https://example.test/fixture.png" && block["mimeType"] == "image/png")
		}
		if !found {
			return p.respondError(request.ID, -32603, "Internal error", map[string]any{"error": "ACP prompt omitted canonical resource link"})
		}
	}
	if p.mode == "model" && !p.modelSet {
		return p.respondError(request.ID, -32603, "Internal error", map[string]any{"error": "advertised model was not applied"})
	}
	if p.mode == "unsupported" {
		if err := p.assertUnsupportedClientMethods(); err != nil {
			return p.respondError(request.ID, -32603, "Internal error", map[string]any{"error": err.Error()})
		}
	}
	if p.mode == "fail" {
		if err := p.update(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"partial ACP answer"}}`); err != nil {
			return err
		}
		return p.respondError(request.ID, -32603, "Internal error", map[string]any{"error": "functional ACP prompt failure"})
	}
	for _, update := range []string{
		`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"ACP root "}}`,
		`{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"checking the Factory state"}}`,
		`{"sessionUpdate":"tool_call","toolCallId":"tool-1","title":"Inspect Factory","status":"in_progress","rawInput":{"scope":"factory"}}`,
		`{"sessionUpdate":"plan","entries":[{"content":"Complete the ACP turn","priority":"high","status":"in_progress"}]}`,
		`{"sessionUpdate":"usage_update","used":12,"size":4096}`,
		`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"execution COMPLETE"}}`,
		`{"sessionUpdate":"tool_call_update","toolCallId":"tool-1","title":"Inspect Factory","status":"completed","rawOutput":{"ok":true},"content":[{"type":"diff","path":"factory/result.txt","newText":"complete\n"}]}`,
	} {
		if err := p.update(update); err != nil {
			return err
		}
	}
	return p.respond(request.ID, json.RawMessage(`{"stopReason":"end_turn"}`))
}

func (p *functionalRPCPeer) assertUnsupportedClientMethods() error {
	calls := []struct {
		method string
		params string
	}{
		{"fs/read_text_file", `{"sessionId":"acp-session-functional-1","path":"/fixture/read.txt"}`},
		{"fs/write_text_file", `{"sessionId":"acp-session-functional-1","path":"/fixture/write.txt","content":"fixture"}`},
		{"terminal/create", `{"sessionId":"acp-session-functional-1","command":"echo","args":[]}`},
		{"terminal/kill", `{"sessionId":"acp-session-functional-1","terminalId":"terminal-1"}`},
		{"terminal/output", `{"sessionId":"acp-session-functional-1","terminalId":"terminal-1"}`},
		{"terminal/release", `{"sessionId":"acp-session-functional-1","terminalId":"terminal-1"}`},
		{"terminal/wait_for_exit", `{"sessionId":"acp-session-functional-1","terminalId":"terminal-1"}`},
	}
	for _, call := range calls {
		p.nextCallID++
		id := fmt.Sprintf("peer-%d", p.nextCallID)
		idJSON, _ := json.Marshal(id)
		if err := p.write(rpcEnvelope{JSONRPC: "2.0", ID: idJSON, Method: call.method, Params: json.RawMessage(call.params)}); err != nil {
			return err
		}
		if !p.scanner.Scan() {
			return fmt.Errorf("%s received no response", call.method)
		}
		var response rpcEnvelope
		if err := json.Unmarshal(p.scanner.Bytes(), &response); err != nil {
			return fmt.Errorf("decode %s response: %w", call.method, err)
		}
		if response.Error == nil {
			return fmt.Errorf("%s response = %s, want not supported error", call.method, p.scanner.Bytes())
		}
	}
	return nil
}

func (p *functionalRPCPeer) update(update string) error {
	params := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"update":%s}`, p.sessionID, update))
	return p.write(rpcEnvelope{JSONRPC: "2.0", Method: "session/update", Params: params})
}

func (p *functionalRPCPeer) respond(id, result json.RawMessage) error {
	return p.write(rpcEnvelope{JSONRPC: "2.0", ID: id, Result: result})
}

func (p *functionalRPCPeer) respondError(id json.RawMessage, code int, message string, data any) error {
	return p.write(rpcEnvelope{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message, Data: data}})
}

func (p *functionalRPCPeer) write(message rpcEnvelope) error {
	if err := json.NewEncoder(p.writer).Encode(message); err != nil {
		return err
	}
	return p.writer.Flush()
}
