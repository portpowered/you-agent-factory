// Package stdio implements the ACP agent-side JSON-RPC stdio server:
// caller-owned stream serving, one-connection lifecycle, connection-scoped
// identity assignment, and newline-delimited JSON-RPC framing and dispatch
// for the "initialize" method. Every other method -- including deferred ACP
// session and prompt methods -- is not yet implemented by this transport and
// receives method-not-found rather than being dispatched. Finer-grained
// protocol-error classification (parse error vs. invalid request vs.
// invalid params) and shutdown semantics beyond clean EOF are added by
// later stories in this slice. It is internal to pkg/transports/acp;
// callers use the package root's exported operations instead of this
// package directly.
package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/negotiation"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
)

// ErrStreamsRequired marks a Serve call that is missing a required input or
// output stream.
var ErrStreamsRequired = errors.New("acp: stdio serve requires input and output streams")

// Server serves one ACP JSON-RPC agent-side connection at a time over
// caller-owned stdio streams. Construction performs no I/O and starts no
// goroutine, process, listener, session, or persistence; each Serve call
// begins one connection-scoped invocation over the streams the caller
// supplies for that call.
type Server struct {
	logger logging.Logger
}

// New constructs an inert stdio Server. Construction alone performs no
// reads, writes, goroutine starts, process starts, endpoint binding,
// session creation, or persistence.
func New(logger logging.Logger) *Server {
	return &Server{logger: logging.EnsureLogger(logger)}
}

// Serve begins one connection-scoped serving invocation over caller-owned
// input and output streams and returns once the connection terminates.
// Starting the invocation creates one connection-scoped identity, minted
// via identity.NewConnectionID, that is stable for the duration of this
// call and distinct from every other invocation. Start and terminal
// outcomes are logged as structured, payload-free diagnostics carrying only
// the connection id and a bounded outcome label.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	if s == nil {
		return errors.New("acp: stdio server is required")
	}
	if in == nil || out == nil {
		return ErrStreamsRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	connectionID := identity.NewConnectionID()
	logger := logging.EnsureLogger(s.logger)
	logger.Info("acp stdio connection started", "connectionId", string(connectionID))

	err := serveConnection(connectionID, in, out)

	logger.Info("acp stdio connection terminated",
		"connectionId", string(connectionID),
		"outcome", terminalOutcomeLabel(err),
	)
	return err
}

// terminalOutcomeLabel classifies a Serve result into a bounded, safe label
// for diagnostics. It never includes the error's message text, so a cause
// that happens to mention sensitive request content can never reach a log
// record through this label.
func terminalOutcomeLabel(err error) string {
	if err == nil {
		return "eof"
	}
	return "error"
}

// serveConnection reads line-delimited input to a clean EOF, treating every
// complete non-empty line as one JSON-RPC 2.0 message. Each notification is
// dispatched with no response ever written for it, and every other message
// receives exactly one complete newline-terminated JSON-RPC response before
// the next line is read, so no two responses can interleave.
func serveConnection(connectionID identity.ConnectionID, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var notificationSeq uint64
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		raw := append(json.RawMessage(nil), line...)

		env, result, rpcErr := dispatchRequest(connectionID, notificationSeq, raw)
		if env.IsNotification {
			notificationSeq++
			continue
		}
		if err := writeResponse(out, env.Identity, result, rpcErr); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// dispatchRequest decodes and executes one JSON-RPC message received on
// connectionID. It returns the decoded envelope -- the zero Envelope for a
// message that never successfully decoded -- alongside the outcome: a
// non-nil result for a successful initialize exchange, or a bounded,
// protocol-safe *acpsdk.RequestError for every rejection. The only method
// this transport dispatches to an effect in this slice is "initialize";
// protocol-version policy is delegated entirely to the existing V0
// negotiation behavior rather than re-implemented here.
func dispatchRequest(connectionID identity.ConnectionID, notificationSeq uint64, raw json.RawMessage) (envelope.Envelope, json.RawMessage, *acpsdk.RequestError) {
	env, err := envelope.Decode(connectionID, notificationSeq, raw)
	if err != nil {
		return envelope.Envelope{}, nil, protocol.SafeReject(err)
	}
	if env.IsNotification {
		return env, nil, nil
	}

	if env.Method != acpsdk.AgentMethodInitialize {
		return env, nil, protocol.MethodNotFound(env.Method)
	}

	var req acpsdk.InitializeRequest
	if err := json.Unmarshal(env.Params, &req); err != nil {
		return env, nil, protocol.SafeReject(err)
	}

	resp, negotiateErr := negotiation.Negotiate(req)
	if negotiateErr != nil {
		reqErr, ok := negotiateErr.(*acpsdk.RequestError)
		if !ok {
			reqErr = protocol.SafeReject(negotiateErr)
		}
		return env, nil, reqErr
	}

	result, err := json.Marshal(resp)
	if err != nil {
		return env, nil, protocol.SafeReject(err)
	}
	return env, result, nil
}

// rpcMessage is the wire shape of one JSON-RPC 2.0 response: exactly one of
// Result or Error is ever populated.
type rpcMessage struct {
	JSONRPC string               `json:"jsonrpc"`
	ID      json.RawMessage      `json:"id"`
	Result  json.RawMessage      `json:"result,omitempty"`
	Error   *acpsdk.RequestError `json:"error,omitempty"`
}

// nullID is the JSON-RPC id for a response that cannot be correlated to a
// request id -- for example a message that never decoded far enough to
// recover one.
var nullID = json.RawMessage("null")

// writeResponse serializes and writes exactly one complete
// newline-terminated JSON-RPC response for a request identity, correlating
// it to the identity's original wire id when one is available. A short
// write is reported as io.ErrShortWrite rather than treated as success, so
// a truncated response can never be mistaken for a complete one.
func writeResponse(out io.Writer, reqIdentity identity.RequestIdentity, result json.RawMessage, rpcErr *acpsdk.RequestError) error {
	msg := rpcMessage{JSONRPC: "2.0", ID: nullID, Result: result, Error: rpcErr}
	if wireID, ok := reqIdentity.WireID(); ok {
		idBytes, err := wireID.MarshalJSON()
		if err != nil {
			return err
		}
		msg.ID = idBytes
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	body = append(body, '\n')

	n, err := out.Write(body)
	if err != nil {
		return err
	}
	if n != len(body) {
		return io.ErrShortWrite
	}
	return nil
}
