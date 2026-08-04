// Package stdio implements the ACP agent-side JSON-RPC stdio server:
// caller-owned stream serving, one-connection lifecycle, connection-scoped
// identity assignment, newline-delimited JSON-RPC framing and dispatch for
// the "initialize", "session/new", and "session/set_config_option" methods,
// plus "session/prompt" -- both the "/factory <value>" fallback command
// recognized within it (final-proposal.md §3) and, for every other
// (genuine, non-command) prompt, admission of exactly one version-guarded
// Chat turn against the canonical Chat Sessions authority, followed by
// starting (first turn in an episode) or invoking (later turns) the bound
// Factory Session, mapping its published outcome, deterministically and
// without fabrication, into the one final "session/prompt" response, and
// terminalizing the admitted turn on every outcome (COMPLETED, CANCELED, or
// FAILED) so no admitted turn is ever left stranded non-terminal --
// protocol-safe rejection of malformed input, unsupported methods, and
// unsupported protocol versions, and deterministic termination on clean
// EOF, context cancellation, a partial trailing frame, or a writer failure.
// Every other deferred ACP session and prompt behavior continues to receive
// method-not-found. It is internal to pkg/transports/acp; callers use the
// package root's exported operations instead of this package directly.
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
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/negotiation"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
)

// ErrStreamsRequired marks a Serve call that is missing a required input or
// output stream.
var ErrStreamsRequired = errors.New("acp: stdio serve requires input and output streams")

// errNullInitializeParams marks an initialize request whose "params" member
// is JSON null. json.Unmarshal treats a JSON null target as a no-op success
// even for a non-pointer struct, so an unmarshal into acpsdk.InitializeRequest
// would otherwise silently leave a zero-valued request -- a requested
// protocol version of 0 -- and be misreported as an unsupported protocol
// version by negotiation rather than as the missing params it actually is.
var errNullInitializeParams = errors.New("acp: initialize params must not be null")

// Server serves one ACP JSON-RPC agent-side connection at a time over
// caller-owned stdio streams. Construction performs no I/O and starts no
// goroutine, process, listener, session, or persistence; each Serve call
// begins one connection-scoped invocation over the streams the caller
// supplies for that call.
//
// chatSessions and catalog are the canonical Chat Sessions collaborators
// "session/new", "session/set_config_option", the "/factory" fallback
// command, and ordinary prompt turn admission dispatch to; factoryTarget is
// the consumer-owned Factory Sessions shim an admitted ordinary prompt turn
// starts or invokes against; events is the canonical Events collaborator an
// admitted prompt turn drains chat-session/<id>/events through (see
// streamTurnUpdates in prompt_stream.go) before falling back to V1
// synchronous final text; resolveHomeDir supplies the operator home
// directory used to derive the Operator Settings document path and Factory
// discovery roots for a catalog resolution. Any of the five may be nil, in
// which case a dispatched method reports a bounded internal error instead of
// proceeding (or, for events specifically, streamTurnUpdates degrades to a
// streaming no-op rather than failing the turn -- see its own doc comment)
// -- so a Server constructed for a slice of this transport that never
// exercises them (for example the "initialize"-only smoke tests in this
// package) never has to supply them.
//
// A Server instance holds no reconciliation state of its own for a started-
// but-not-yet-bound Factory Session: that record lives on the episode itself
// (TargetEpisode.PendingFactorySessionID, via
// chatsessions.Service.RecordPendingFactorySession) under the singular Chat/
// Factory Sessions authority, so the "retry after a post-start failure
// cannot create a second Factory Session" guarantee survives this Server
// being reconstructed, not just this one instance staying alive -- see
// startFactorySessionForEpisode.
type Server struct {
	logger         logging.Logger
	chatSessions   chatsessions.Service
	catalog        chatsessions.FactoryTargetCatalogService
	factoryTarget  acp.FactoryTargetService
	events         events.Service
	resolveHomeDir func() (string, error)
}

// New constructs an inert stdio Server. Construction alone performs no
// reads, writes, goroutine starts, process starts, endpoint binding,
// session creation, or persistence.
func New(
	logger logging.Logger,
	chatSessions chatsessions.Service,
	catalog chatsessions.FactoryTargetCatalogService,
	factoryTarget acp.FactoryTargetService,
	eventsService events.Service,
	resolveHomeDir func() (string, error),
) *Server {
	return &Server{
		logger:         logging.EnsureLogger(logger),
		chatSessions:   chatSessions,
		catalog:        catalog,
		factoryTarget:  factoryTarget,
		events:         eventsService,
		resolveHomeDir: resolveHomeDir,
	}
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

	err := s.serveConnection(ctx, connectionID, in, out)

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
	switch {
	case err == nil:
		return "eof"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "cancelled"
	default:
		return "error"
	}
}

// errPartialTrailingFrame marks input that reaches EOF with a non-empty
// remainder that was never terminated by a newline. Treating that remainder
// as a final request -- the way bufio.ScanLines does by default -- would
// execute less than the full message the client meant to send, so it is
// instead a deterministic protocol failure that ends the connection.
var errPartialTrailingFrame = errors.New("acp: stdio input ended with a partial trailing frame")

// scanCompleteLines is a bufio.SplitFunc that only ever yields complete,
// newline-terminated lines, unlike bufio.ScanLines' default behavior of
// also yielding a final non-newline-terminated remainder at EOF.
func scanCompleteLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		if len(bytes.TrimSpace(data)) > 0 {
			return 0, nil, errPartialTrailingFrame
		}
		return len(data), nil, nil
	}
	return 0, nil, nil
}

// serveConnection reads line-delimited input to a clean EOF, treating every
// complete non-empty line as one JSON-RPC 2.0 message. Each notification is
// dispatched with no response ever written for it, and every other message
// receives exactly one complete newline-terminated JSON-RPC response before
// the next line is read, so no two responses can interleave. Context
// cancellation is checked before every read: it stops accepting new work
// immediately, and once a read fails or ends because the caller-owned
// stream itself was closed or cancelled on the context's behalf, the
// context's error is reported instead of the stream's raw error, so a
// deliberate shutdown is never mistaken for a fault. A partial trailing
// frame at EOF and a response write failure both end the connection with
// their own error instead of being treated as success.
func (s *Server) serveConnection(ctx context.Context, connectionID identity.ConnectionID, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	scanner.Split(scanCompleteLines)

	// notify is this connection's one outbound "session/update" delivery
	// capability, carried through each dispatched request's context (see
	// contextWithPromptNotifier in session_prompt.go) rather than threaded as
	// an explicit parameter -- the only dispatched method that ever uses it
	// is "session/prompt", and adding a parameter every other handler and
	// every existing unit test would have to accept for a capability they
	// never use is exactly the kind of unnecessary plumbing a request-scoped
	// context value avoids. Writing a notification line and writing this
	// same request's eventual response line can never interleave with each
	// other or with another request's lines: dispatchRequest runs to
	// completion (and any notify call within it, synchronously) before this
	// loop's single writeResponse call for the same line, and the next line
	// is never read until that happens.
	notify := func(n acpsdk.SessionNotification) error {
		return writeNotification(out, n)
	}

	// attachments is this connection's one chatsessions.Attachment cache
	// (see attachmentCache's own doc comment): reused across every
	// "session/prompt" call on this connection so a later turn resumes the
	// same delivery cursor instead of a fresh, later-arriving attachment
	// silently observing the same records again. detachAttachments releases
	// every attachment this connection ever registered once this call
	// returns, on every exit path (clean EOF, context cancellation, a
	// partial trailing frame, or a writer failure alike) -- disconnect must
	// always free the delivery consumer, never leaving it registered against
	// a session this connection will never resume from again.
	attachments := &attachmentCache{}
	defer s.detachAttachments(ctx, attachments)

	var notificationSeq uint64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				return err
			}
			return nil
		}

		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		raw := append(json.RawMessage(nil), line...)

		reqCtx := contextWithAttachmentCache(contextWithPromptNotifier(ctx, notify), attachments)
		env, result, rpcErr := s.dispatchRequest(reqCtx, connectionID, notificationSeq, raw)
		if env.IsNotification {
			notificationSeq++
			continue
		}
		if err := writeResponse(out, env.Identity, result, rpcErr); err != nil {
			return err
		}
	}
}

// dispatchRequest decodes and executes one JSON-RPC message received on
// connectionID. It returns the decoded envelope -- the zero Envelope, or an
// Envelope carrying only a correlated Identity, for a message that never
// successfully decoded -- alongside the outcome: a non-nil result for a
// successful "initialize", "session/new", "session/set_config_option", or
// "/factory <value>"-recognized "session/prompt" exchange, or a bounded,
// protocol-safe *acpsdk.RequestError for every rejection. An
// envelope.Decode failure is classified by protocol.RejectEnvelope into a
// parse error (uncorrelated, for unparseable JSON) or an invalid-request
// error (correlated to the message's id when that id was itself
// syntactically valid); a decoded "initialize" envelope with valid params
// that Negotiate rejects becomes the richer unsupported-protocol-version
// error unwrapped, and every other rejection -- an unimplemented method,
// params that fail to decode into the pinned request shape, or
// "session/prompt" content that is not the "/factory <value>" command --
// becomes method-not-found or invalid-params. The only methods this
// transport dispatches to an effect in this slice are "initialize",
// "session/new", "session/set_config_option", and "session/prompt" (only
// for its "/factory <value>" fallback command form); protocol-version
// policy is delegated entirely to the existing V0 negotiation behavior
// rather than re-implemented here.
func (s *Server) dispatchRequest(ctx context.Context, connectionID identity.ConnectionID, notificationSeq uint64, raw json.RawMessage) (envelope.Envelope, json.RawMessage, *acpsdk.RequestError) {
	env, err := envelope.Decode(connectionID, notificationSeq, raw)
	if err != nil {
		rpcErr, wireID, hasID := protocol.RejectEnvelope(err)
		rejected := envelope.Envelope{}
		if hasID {
			// connectionID is always non-blank for a real connection, and
			// wireID is already validated by envelope.Decode, so
			// NewCorrelated can never fail here.
			rejected.Identity, _ = identity.NewCorrelated(connectionID, wireID)
		}
		return rejected, nil, rpcErr
	}
	if env.IsNotification {
		return env, nil, nil
	}

	switch env.Method {
	case acpsdk.AgentMethodInitialize:
		result, rpcErr := dispatchInitialize(env.Params)
		return env, result, rpcErr
	case acpsdk.AgentMethodSessionNew:
		result, rpcErr := s.handleSessionNew(ctx, env)
		return env, result, rpcErr
	case acpsdk.AgentMethodSessionSetConfigOption:
		result, rpcErr := s.handleSessionSetConfigOption(ctx, env)
		return env, result, rpcErr
	case acpsdk.AgentMethodSessionPrompt:
		result, rpcErr := s.handleSessionPrompt(ctx, env)
		return env, result, rpcErr
	default:
		return env, nil, protocol.MethodNotFound(env.Method)
	}
}

// dispatchInitialize executes the "initialize" method against raw params,
// negotiating the P0 text-first capability profile.
func dispatchInitialize(params json.RawMessage) (json.RawMessage, *acpsdk.RequestError) {
	if bytes.Equal(bytes.TrimSpace(params), []byte("null")) {
		return nil, protocol.SafeReject(errNullInitializeParams)
	}
	var req acpsdk.InitializeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, protocol.SafeReject(err)
	}

	resp, negotiateErr := negotiation.Negotiate(req)
	if negotiateErr != nil {
		reqErr, ok := negotiateErr.(*acpsdk.RequestError)
		if !ok {
			reqErr = protocol.SafeReject(negotiateErr)
		}
		return nil, reqErr
	}

	result, err := json.Marshal(resp)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}
	return result, nil
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

// notificationMessage is the wire shape of one JSON-RPC 2.0 notification: no
// "id" member at all (not even null), matching NotificationMethods' contract
// that a notification never has a response counterpart.
type notificationMessage struct {
	JSONRPC string                     `json:"jsonrpc"`
	Method  string                     `json:"method"`
	Params  acpsdk.SessionNotification `json:"params"`
}

// writeNotification serializes and writes exactly one complete
// newline-terminated "session/update" JSON-RPC notification. A short write
// is reported as io.ErrShortWrite rather than treated as success, matching
// writeResponse's own short-write handling.
func writeNotification(out io.Writer, params acpsdk.SessionNotification) error {
	body, err := json.Marshal(notificationMessage{
		JSONRPC: "2.0",
		Method:  acpsdk.ClientMethodSessionUpdate,
		Params:  params,
	})
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
