package acp_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// functionalRPCPeer is deliberately an ACP implementation at the process
// boundary, not an acp-go-sdk Agent. The production client therefore has to
// encode, transport, and decode every message over real OS pipes in these
// tests. Keeping the peer as raw JSON-RPC also prevents shared SDK types from
// hiding wire-compatibility failures.
type functionalRPCPeer struct {
	fixture      acpFixtureConfig
	mode         string
	scanner      *bufio.Scanner
	writer       *bufio.Writer
	stderr       io.Writer
	closeOutput  io.Closer
	modelSet     bool
	sessionID    string
	sessions     int
	nextCallID   int
	retryAttempt int
}

func runFunctionalRPCPeer(fixture acpFixtureConfig, stdin io.Reader, stdout, stderr io.Writer) error {
	mode := fixture.Mode
	if mode == "malformed" {
		_, err := fmt.Fprintln(stdout, "{not-json")
		return err
	}
	if mode == "eof" {
		return nil
	}
	retryAttempt := 0
	if mode == "retry-resume" {
		var err error
		retryAttempt, err = currentRetryAttempt(fixture.RetryAttemptDirectory)
		if err != nil {
			return err
		}
	}
	peer := &functionalRPCPeer{
		fixture: fixture, mode: mode, scanner: bufio.NewScanner(stdin), writer: bufio.NewWriter(stdout), stderr: stderr,
		sessionID: fixture.SessionID, retryAttempt: retryAttempt,
	}
	if closeOutput, ok := stdout.(io.Closer); ok {
		peer.closeOutput = closeOutput
	}
	if peer.sessionID == "" {
		peer.sessionID = "acp-session-functional-1"
	}
	if mode == "stderr" {
		_, _ = fmt.Fprintln(stderr, "agent diagnostic token="+os.Getenv("ACP_TEST_API_TOKEN"))
	}
	return peer.serve()
}

func currentRetryAttempt(directory string) (int, error) {
	if directory == "" {
		return 0, fmt.Errorf("retry-resume mode requires retryAttemptDirectory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, fmt.Errorf("read retry attempt directory: %w", err)
	}
	latest := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		attempt, err := strconv.Atoi(entry.Name())
		if err != nil || attempt <= latest {
			continue
		}
		latest = attempt
	}
	if latest == 0 {
		return 0, fmt.Errorf("retry attempt directory %q has no process phase", directory)
	}
	return latest, nil
}

func (p *functionalRPCPeer) serve() error {
	for p.scanner.Scan() {
		var request rpcEnvelope
		if err := json.Unmarshal(p.scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode client RPC: %w", err)
		}
		switch request.Method {
		case "initialize":
			if err := p.initialize(request); err != nil {
				return err
			}
		case "session/new":
			if err := p.createSession(request); err != nil {
				return err
			}
		case "session/load":
			if err := p.loadSession(request); err != nil {
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
			if err := holdACPHelperUntilReleased(p.fixture); err != nil {
				return err
			}
			if p.mode == "package-conformance" {
				return p.waitForPackageConformanceRelease()
			}
			if p.mode == "disconnect-once" && p.sessions == 1 {
				marker := p.fixture.DisconnectMarkerPath
				ready := p.fixture.DisconnectReadyPath
				release := p.fixture.DisconnectReleasePath
				if marker == "" {
					return fmt.Errorf("disconnect-once mode requires disconnectMarkerPath")
				}
				if ready == "" || release == "" {
					return fmt.Errorf("disconnect-once mode requires disconnectReadyPath and disconnectReleasePath")
				}
				if _, err := os.Stat(marker); os.IsNotExist(err) {
					if err := os.WriteFile(ready, []byte("response-ready"), 0o600); err != nil {
						return fmt.Errorf("write ACP response-ready marker: %w", err)
					}
					for {
						if _, err := os.Stat(release); err == nil {
							break
						} else if !os.IsNotExist(err) {
							return fmt.Errorf("inspect ACP disconnect release: %w", err)
						}
						time.Sleep(10 * time.Millisecond)
					}
					if p.closeOutput == nil {
						return fmt.Errorf("disconnect-once mode cannot close its output")
					}
					if err := p.closeOutput.Close(); err != nil {
						return fmt.Errorf("close disconnected ACP output: %w", err)
					}
					if err := os.WriteFile(marker, []byte("disconnected"), 0o600); err != nil {
						return fmt.Errorf("write ACP disconnect marker: %w", err)
					}
					for p.scanner.Scan() {
					}
					return p.scanner.Err()
				} else if err != nil {
					return fmt.Errorf("inspect ACP disconnect marker: %w", err)
				}
			}
			if (p.mode == "spawn" && p.sessions >= 4) ||
				(p.mode == "tournament" && p.sessions >= 3) ||
				((p.mode == "persistent" || p.mode == "serialize") && p.sessions >= 2) {
				return nil
			}
			if p.mode != "persistent" && p.mode != "serialize" && p.mode != "spawn" && p.mode != "tournament" {
				return nil
			}
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

func (p *functionalRPCPeer) waitForPackageConformanceRelease() error {
	// Keep the stdio peer alive until the test has observed the completed Work.
	// The ACP client must drain the prompt's preceding session/update
	// notifications after receiving the response; exiting here races that drain
	// and turns a successful prompt into peer-disconnected.
	release := p.fixture.PackageConformanceReleasePath
	if release == "" {
		return fmt.Errorf("package-conformance mode requires packageConformanceReleasePath")
	}
	for {
		if _, err := os.Stat(release); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect package-conformance release: %w", err)
		}
		runtime.Gosched()
	}
}

func (p *functionalRPCPeer) initialize(request rpcEnvelope) error {
	if p.mode == "init-fail" || p.mode == "stderr" {
		return p.respondError(request.ID, -32603, "Internal error", map[string]any{"error": "functional ACP initialization failure"})
	}
	version := 1
	if p.mode == "version" {
		version = 999
	}
	authMethods := `[]`
	if p.mode == "auth" {
		// Advertises all three real ACP auth method shapes (default agent-handled,
		// env_var, terminal) so every authentication-hint shape is exercised.
		authMethods = `[{"id":"login","name":"Agent login"},{"type":"env_var","id":"env-login","name":"Env var login","vars":[]},{"type":"terminal","id":"terminal-login","name":"Terminal login"}]`
	}
	capabilities := `{}`
	if p.supportsSessionLoad() {
		// A real ACP agent that truthfully supports resume advertises loadSession,
		// matched by resolveContinuationProvider's negotiated-capability check.
		capabilities = `{"loadSession":true}`
	}
	result := json.RawMessage(fmt.Sprintf(`{"protocolVersion":%d,"agentCapabilities":%s,"authMethods":%s}`, version, capabilities, authMethods))
	return p.respond(request.ID, result)
}

func (p *functionalRPCPeer) createSession(request rpcEnvelope) error {
	if p.mode == "auth" {
		return p.respondError(request.ID, -32000, "Authentication required", nil)
	}
	if err := p.rejectUnexpectedNewSession(); err != nil {
		return err
	}
	config := `[]`
	if p.mode == "model" || p.mode == "package-conformance" {
		config = `[{"type":"select","id":"model","name":"Model","category":"model","currentValue":"default","options":[{"name":"Test model","value":"test-model"}]}]`
	}
	p.sessions++
	sessionID := p.sessionID
	if p.mode == "persistent" || p.mode == "serialize" {
		sessionID = fmt.Sprintf("acp-session-functional-1-%d", p.sessions)
	}
	p.sessionID = sessionID
	result := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"configOptions":%s}`, sessionID, config))
	return p.respond(request.ID, result)
}

func (p *functionalRPCPeer) loadSession(request rpcEnvelope) error {
	if p.mode == "resume-not-found" {
		// -32002 is the ACP schema's ErrorCodeResourceNotFound returned for an
		// unrecognized session/load id by a conformant agent.
		return p.respondError(request.ID, -32002, "no rollout found for that session", nil)
	}
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return fmt.Errorf("decode session/load: %w", err)
	}
	if err := p.assertExactRetryResume(params.SessionID); err != nil {
		return err
	}
	p.sessionID = params.SessionID
	p.sessions++
	return p.respond(request.ID, json.RawMessage(`{"configOptions":[]}`))
}

func (p *functionalRPCPeer) supportsSessionLoad() bool {
	return p.mode == "resume" || p.mode == "resume-not-found" || p.mode == "retry-resume"
}

func (p *functionalRPCPeer) rejectUnexpectedNewSession() error {
	if p.mode == "resume" || p.mode == "resume-not-found" {
		return fmt.Errorf("unexpected session/new during a continuation - the continued attempt must resume through session/load instead of starting a fresh session")
	}
	if p.mode != "retry-resume" {
		return nil
	}
	if p.retryAttempt != 1 {
		return fmt.Errorf("unexpected session/new on retry attempt %d - the retry must resume through session/load", p.retryAttempt)
	}
	return nil
}

func (p *functionalRPCPeer) assertExactRetryResume(sessionID string) error {
	if p.mode != "retry-resume" {
		return nil
	}
	if p.retryAttempt != 2 {
		return fmt.Errorf("session/load on retry attempt %d, want the second ACP process", p.retryAttempt)
	}
	if sessionID != p.sessionID {
		return fmt.Errorf("session/load id = %q, want original %q", sessionID, p.sessionID)
	}
	return nil
}

func (p *functionalRPCPeer) prompt(request rpcEnvelope) error {
	if handled, err := p.respondToPackagedPrompt(request); handled {
		return err
	}
	if p.mode == "shared-spine" && sharedSpineFailurePrompt(request) {
		if err := p.update(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"partial shared-spine answer"}}`); err != nil {
			return err
		}
		return p.respondError(request.ID, -32603, "Internal error", map[string]any{"error": "shared ACP protocol failure"})
	}
	if p.mode == "crash-once" {
		marker := p.fixture.CrashMarkerPath
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			if err := os.WriteFile(marker, []byte("crashed"), 0o600); err != nil {
				return err
			}
			return fmt.Errorf("intentional ACP peer crash")
		}
	}
	if p.mode == "retry-resume" {
		switch p.retryAttempt {
		case 1:
			if err := p.respondError(request.ID, -32001, "temporarily unavailable", nil); err != nil {
				return err
			}
			return holdFailedRetryPeer(p.fixture.RetryHoldPath)
		case 2:
			break
		default:
			return fmt.Errorf("unexpected retry-resume prompt on ACP process attempt %d", p.retryAttempt)
		}
	}
	if p.mode == "serialize" && p.sessions == 1 {
		if signal := p.fixture.PromptSignalPath; signal != "" {
			_ = os.WriteFile(signal, []byte("first-prompt-started"), 0o600)
		}
		release := p.fixture.PromptReleasePath
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(release); err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("timed out waiting for first prompt release")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if p.mode == "block" {
		if signal := p.fixture.PromptSignalPath; signal != "" {
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
	if err := p.validatePromptPayload(request); err != nil {
		return err
	}
	if (p.mode == "model" || p.mode == "package-conformance") && !p.modelSet {
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

func sharedSpineFailurePrompt(request rpcEnvelope) bool {
	var params struct {
		Prompt []struct {
			Text string `json:"text"`
		} `json:"prompt"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return false
	}
	for _, block := range params.Prompt {
		if strings.Contains(block.Text, "shared ACP failure") {
			return true
		}
	}
	return false
}

func holdFailedRetryPeer(holdMarker string) error {
	if holdMarker == "" {
		return nil
	}
	if err := os.WriteFile(holdMarker, []byte("first prompt failed and peer remains live"), 0o600); err != nil {
		return err
	}
	// Keep the failed peer alive and unresponsive so the public retry can only
	// succeed after the provider retires this process. The production stop
	// deadline is the failure guard for this fixture; no test-side delay is
	// needed.
	for {
		runtime.Gosched()
	}
}

// respondToPackagedPrompt handles the prompt() modes whose reply depends on
// which packaged prompt or self-reported stop reason arrived, before
// prompt()'s default multi-update turn runs. cancelled-response answers with
// StopReasonCancelled after one partial update, without any session/cancel
// notification ever having been sent -- a real ACP agent may self-report
// cancellation this way.
func (p *functionalRPCPeer) respondToPackagedPrompt(request rpcEnvelope) (bool, error) {
	if p.mode == "cancelled-response" {
		if err := p.update(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"partial ACP answer before self-cancellation"}}`); err != nil {
			return true, err
		}
		return true, p.respond(request.ID, json.RawMessage(`{"stopReason":"cancelled"}`))
	}
	responses := map[string][]string{
		"tournament": {"candidate one", "candidate two", `{"winner":"B","rationale":"candidate two is stronger"}`},
		"spawn":      {`{"tasks":["research climate","research cost"]}`, `{"result":"climate findings"}`, `{"result":"cost findings"}`, `{"answer":"merged travel answer"}`},
	}
	modeResponses, ok := responses[p.mode]
	if !ok {
		return false, nil
	}
	index := p.sessions - 1
	if index < 0 || index >= len(modeResponses) {
		message := fmt.Sprintf("unexpected packaged %s prompt", p.mode)
		return true, p.respondError(request.ID, -32603, "Internal error", map[string]any{"error": message})
	}
	if err := p.update(fmt.Sprintf(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":%q}}`, modeResponses[index])); err != nil {
		return true, err
	}
	return true, p.respond(request.ID, json.RawMessage(`{"stopReason":"end_turn"}`))
}

// validatePromptPayload checks that the inbound session/prompt request
// carried the exact content the "resource" and "content" modes each expect
// the production ACP execution path to have populated, replying with an RPC
// error (rather than returning err directly) so a mismatch surfaces through
// the same functional Execute failure path as every other assertion in this
// peer.
func (p *functionalRPCPeer) validatePromptPayload(request rpcEnvelope) error {
	if p.mode != "resource" && p.mode != "content" {
		return nil
	}
	var params struct {
		Prompt []map[string]any `json:"prompt"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return fmt.Errorf("decode %s prompt: %w", p.mode, err)
	}
	if p.mode == "resource" {
		found := false
		for _, block := range params.Prompt {
			found = found || (block["type"] == "resource_link" && block["uri"] == "https://example.test/fixture.png" && block["mimeType"] == "image/png")
		}
		if !found {
			return p.respondError(request.ID, -32603, "Internal error", map[string]any{"error": "ACP prompt omitted canonical resource link"})
		}
		return nil
	}
	want := p.fixture.ContentSentinel
	found := false
	for _, block := range params.Prompt {
		text, _ := block["text"].(string)
		found = found || strings.Contains(text, want)
	}
	if want == "" || !found {
		return p.respondError(request.ID, -32603, "Internal error", map[string]any{"error": "ACP prompt omitted input Work content"})
	}
	return nil
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
