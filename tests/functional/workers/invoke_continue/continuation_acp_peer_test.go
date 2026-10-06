package acceptance

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Protocol traffic stays in memory. The command edge supplies only a short-lived
// host shell carrier for the real supervisor's Start/Wait contract; it neither
// executes a provider nor reexecutes a test binary. This proves no OS kill edge.
type continuationACPPeer struct {
	loadSupported    bool
	stale            bool
	changedHandshake bool
	ready            chan struct{}
	mu               sync.Mutex
	calls            []continuationACPRPC
}

type continuationACPRPC struct {
	Method string          `json:"method"`
	ID     json.RawMessage `json:"id"`
	Params json.RawMessage `json:"params"`
}

func (peer *continuationACPPeer) record(call continuationACPRPC) {
	peer.mu.Lock()
	defer peer.mu.Unlock()
	peer.calls = append(peer.calls, call)
}

func (peer *continuationACPPeer) requests() []continuationACPRPC {
	peer.mu.Lock()
	defer peer.mu.Unlock()
	return append([]continuationACPRPC(nil), peer.calls...)
}

const continuationACPConfig = `[{"id":"model","name":"Model","category":"model","type":"select","currentValue":"functional-model","options":[{"value":"functional-model","name":"Functional model"}]}]`

func (peer *continuationACPPeer) serve(reader io.Reader, writer io.Writer) {
	scanner := bufio.NewScanner(reader)
	sessionID := "opaque-acp-source"
	loaded := false
	for scanner.Scan() {
		var call continuationACPRPC
		if json.Unmarshal(scanner.Bytes(), &call) != nil {
			return
		}
		peer.record(call)
		result := `{}`
		switch call.Method {
		case "initialize":
			result = peer.initializeResult()
		case "session/new":
			result = fmt.Sprintf(`{"sessionId":%q,"configOptions":%s}`, sessionID, continuationACPConfig)
		case "session/load":
			if peer.stale {
				if sendContinuationACPRPC(writer, map[string]any{"jsonrpc": "2.0", "id": call.ID, "error": map[string]any{"code": -32002, "message": "Resource not found"}}) != nil {
					return
				}
				continue
			}
			loaded = true
			result = fmt.Sprintf(`{"configOptions":%s}`, continuationACPConfig)
		case "session/set_config_option":
			result = fmt.Sprintf(`{"configOptions":%s}`, continuationACPConfig)
		case "session/prompt":
			if !loaded {
				close(peer.ready)
				// Source stays active until the customer's joined terminate closes
				// its request-owned connection. No sleep or package-wide gate.
				continue
			}
			if sendContinuationACPRPC(writer, map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
				"sessionId": sessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "ACP continued COMPLETE"}},
			}}) != nil {
				return
			}
			result = `{"stopReason":"end_turn"}`
		}
		if len(call.ID) != 0 && sendContinuationACPRPC(writer, map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": json.RawMessage(result)}) != nil {
			return
		}
	}
}

func (peer *continuationACPPeer) initializeResult() string {
	loadSupported := peer.loadSupported
	if peer.changedHandshake {
		initializations := 0
		for _, prior := range peer.requests() {
			if prior.Method == "initialize" {
				initializations++
			}
		}
		loadSupported = initializations == 1
	}
	return fmt.Sprintf(`{"protocolVersion":1,"agentCapabilities":{"loadSession":%t}}`, loadSupported)
}

func sendContinuationACPRPC(writer io.Writer, value any) error {
	return json.NewEncoder(writer).Encode(value)
}

type continuationACPLocator struct{}

func (continuationACPLocator) LookPath(file string) (string, error) { return file, nil }
