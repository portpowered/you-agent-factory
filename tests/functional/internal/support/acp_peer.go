package support

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// ACPWorkerUpdateScript is the canonical set of session/update notifications a
// scripted ACP worker emits, in source order: one of every update variant the
// production ACP client models, ending on text carrying the caller's stop
// token so the Factory Runtime reaches its declared terminal state.
//
// It is exported so the scripted peers in different functional test packages
// assert against one script rather than each maintaining a copy that drifts.
// completionText must contain the fixture worker's stopToken, or the Work
// never advances past PROCESSING and every assertion downstream is vacuous.
func ACPWorkerUpdateScript(toolCallID, completionText string) []string {
	return []string{
		fmt.Sprintf(`{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":%q}}`,
			"considering "+toolCallID),
		fmt.Sprintf(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":%q}}`,
			"working on "+toolCallID+" "),
		fmt.Sprintf(`{"sessionUpdate":"tool_call","toolCallId":%q,"title":"Inspect Factory","status":"in_progress","rawInput":{"scope":"factory"}}`,
			toolCallID),
		`{"sessionUpdate":"plan","entries":[{"content":"Complete the ACP turn","priority":"high","status":"in_progress"}]}`,
		`{"sessionUpdate":"usage_update","used":17,"size":4096}`,
		fmt.Sprintf(`{"sessionUpdate":"tool_call_update","toolCallId":%q,"title":"Inspect Factory","status":"completed","rawOutput":{"ok":true},"content":[{"type":"diff","path":"factory/result.txt","newText":"complete\n"}]}`,
			toolCallID),
		fmt.Sprintf(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":%q}}`, completionText),
	}
}

// ACPWorkerPeerConfig parameterizes one scripted ACP worker peer.
//
// SessionIDPrefix and the per-prompt ordinal together give each concurrent
// dispatch a distinct provider session and tool-call id, which is what lets a
// test prove two Workers' child streams stay separately attributed rather than
// merging into one.
type ACPWorkerPeerConfig struct {
	SessionIDPrefix string
	CompletionText  string
}

// RunACPWorkerPeer serves a minimal, hand-rolled JSON-RPC 2.0 ACP agent over
// the supplied pipes until its client disconnects.
//
// It is deliberately not an acp-go-sdk Agent: the production ACP client must
// encode, transport, and decode every message over real pipes, exactly as it
// would against a genuine subprocess agent, so shared SDK types cannot hide a
// wire-compatibility failure. It serves an unbounded number of session/prompt
// turns rather than exiting after the first, because a Factory that dispatches
// more than one Worker reuses one daemon process.
func RunACPWorkerPeer(config ACPWorkerPeerConfig, stdin io.Reader, stdout io.Writer) error {
	peer := &acpWorkerPeer{
		config:  config,
		scanner: bufio.NewScanner(stdin),
		writer:  bufio.NewWriter(stdout),
	}
	// A single ACP frame can carry a whole tool result, so the default 64 KiB
	// scanner limit is not enough for a realistic agent.
	peer.scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return peer.serve()
}

type acpWorkerPeer struct {
	config    ACPWorkerPeerConfig
	scanner   *bufio.Scanner
	writer    *bufio.Writer
	sessions  int
	sessionID string
}

type acpWorkerEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acpWorkerError `json:"error,omitempty"`
}

type acpWorkerError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (p *acpWorkerPeer) serve() error {
	for p.scanner.Scan() {
		var request acpWorkerEnvelope
		if err := json.Unmarshal(p.scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode client RPC: %w", err)
		}
		switch request.Method {
		case "initialize":
			if err := p.respond(request.ID, json.RawMessage(
				`{"protocolVersion":1,"agentCapabilities":{},"authMethods":[]}`)); err != nil {
				return err
			}
		case "session/new":
			p.sessions++
			p.sessionID = fmt.Sprintf("%s-%d", p.config.SessionIDPrefix, p.sessions)
			if err := p.respond(request.ID, json.RawMessage(
				fmt.Sprintf(`{"sessionId":%q,"configOptions":[]}`, p.sessionID))); err != nil {
				return err
			}
		case "session/prompt":
			if err := p.prompt(request); err != nil {
				return err
			}
		case "$/cancel_request", "session/cancel":
			return nil
		default:
			return fmt.Errorf("unexpected client RPC method %q", request.Method)
		}
	}
	return p.scanner.Err()
}

// prompt streams the canonical update script for the session this prompt
// arrived on, so each dispatch's tool-call id is distinguishable in the
// assertions.
func (p *acpWorkerPeer) prompt(request acpWorkerEnvelope) error {
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return fmt.Errorf("decode session/prompt: %w", err)
	}
	sessionID := params.SessionID
	if sessionID == "" {
		sessionID = p.sessionID
	}
	for _, update := range ACPWorkerUpdateScript("tool-"+sessionID, p.config.CompletionText) {
		params := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"update":%s}`, sessionID, update))
		if err := p.write(acpWorkerEnvelope{JSONRPC: "2.0", Method: "session/update", Params: params}); err != nil {
			return err
		}
	}
	return p.respond(request.ID, json.RawMessage(`{"stopReason":"end_turn"}`))
}

func (p *acpWorkerPeer) respond(id, result json.RawMessage) error {
	return p.write(acpWorkerEnvelope{JSONRPC: "2.0", ID: id, Result: result})
}

func (p *acpWorkerPeer) write(message acpWorkerEnvelope) error {
	if err := json.NewEncoder(p.writer).Encode(message); err != nil {
		return err
	}
	return p.writer.Flush()
}
