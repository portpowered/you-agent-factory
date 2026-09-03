// Package stdio implements the ACP agent-side JSON-RPC stdio server:
// caller-owned stream serving, one-connection lifecycle, connection-scoped
// identity assignment, newline-delimited JSON-RPC framing and dispatch for
// the "initialize", "session/new", "session/load", "session/resume", and
// "session/set_config_option" methods, plus "session/prompt" -- both the
// "/factory <value>" fallback command
// recognized within it (final-proposal.md §3) and, for every other
// (genuine, non-command) prompt, admission of exactly one version-guarded
// Chat turn against the canonical Chat Sessions authority, followed by
// starting (first turn in an episode) or invoking (later turns) the bound
// Factory Session, mapping its published outcome, deterministically and
// without fabrication, into the one final "session/prompt" response, and
// terminalizing the admitted turn on every outcome (COMPLETED, CANCELED, or
// FAILED) so no admitted turn is ever left stranded non-terminal, plus the
// "session/cancel" notification and "session/close" request, each routed
// through a captured Chat control intent to the exact bound or pending
// Factory Session using the same Factory Sessions-owned target-execution
// capability -- protocol-safe rejection of malformed input, unsupported
// methods, and unsupported protocol versions, and deterministic termination
// on clean EOF, context cancellation, a partial trailing frame, or a writer
// failure. Every other deferred ACP session and prompt behavior continues to
// receive method-not-found. It is internal to pkg/transports/acp; callers use
// the package root's exported operations instead of this package directly.
package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformstdio "github.com/portpowered/infinite-you/pkg/platform/stdio"
	"github.com/portpowered/infinite-you/pkg/platform/taskgroup"
	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
// the Factory Sessions-owned target-execution capability an admitted prompt
// starts or invokes against; events is the canonical aggregate stream an
// admitted turn drains before falling back to V1 final text. A narrowly
// constructed Server may omit events, in which case streaming is a no-op.
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
	factoryTarget  factorysessions.TargetExecutionService
	events         events.Service
	resolveHomeDir func() (string, error)
	responseBridge acp.ResponseBridge
	wireRecorder   acp.WireRecorder
}

func (s *Server) resolveInvocationHomeDir(ctx context.Context) (string, error) {
	if profile, ok := acp.InvocationProfileFromContext(ctx); ok {
		if homeDir := strings.TrimSpace(profile.HomeDir); homeDir != "" {
			return homeDir, nil
		}
	}
	return s.resolveHomeDir()
}

// promptFlightRegistry coalesces duplicate session/prompt requests for the
// lifetime of one connection. Prompt processing is asynchronous so a later
// line can carry a session/cancel notification while the Factory call is in
// flight. That also means an immediate redelivery of the same JSON-RPC
// request can otherwise observe the turn before its first handler has moved
// it out of ADMITTED. A flight lets that redelivery await the original
// response instead of racing another admission or Factory dispatch.
//
// The key is RequestIdentity's JSON representation, which includes both the
// connection identifier and JSON-RPC id. This keeps identical ids on
// independent connections distinct, as RequestIdentity itself requires.
type promptFlightRegistry struct {
	mu        sync.Mutex
	byRequest map[string]*promptFlight
}

type promptFlight struct {
	done   chan struct{}
	result json.RawMessage
	rpcErr *acpsdk.RequestError
}

func (r *promptFlightRegistry) start(req identity.RequestIdentity) (*promptFlight, bool) {
	keyBytes, err := json.Marshal(req)
	if err != nil {
		// envelope.Decode already constructs a valid correlated identity for
		// every request. Keeping a defensive, per-flight fallback prevents an
		// unforeseen marshal problem from accidentally coalescing unrelated
		// requests.
		return &promptFlight{done: make(chan struct{})}, true
	}
	key := string(keyBytes)
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing := r.byRequest[key]; existing != nil {
		return existing, false
	}
	flight := &promptFlight{done: make(chan struct{})}
	if r.byRequest == nil {
		r.byRequest = make(map[string]*promptFlight)
	}
	r.byRequest[key] = flight
	return flight, true
}

func (f *promptFlight) finish(result json.RawMessage, rpcErr *acpsdk.RequestError) {
	f.result = result
	f.rpcErr = rpcErr
	close(f.done)
}

// New constructs an inert stdio Server. Construction alone performs no
// reads, writes, goroutine starts, process starts, endpoint binding,
// session creation, or persistence. responseBridge may be nil, in which case
// an admitted prompt turn's Factory dispatch never starts the response
// bridge (see runResponseBridge in response_bridge.go) and behaves exactly
// as it did before that collaborator existed.
func New(
	logger logging.Logger,
	chatSessions chatsessions.Service,
	catalog chatsessions.FactoryTargetCatalogService,
	factoryTarget factorysessions.TargetExecutionService,
	eventsService events.Service,
	resolveHomeDir func() (string, error),
	responseBridge acp.ResponseBridge,
	wireRecorder acp.WireRecorder,
) *Server {
	return &Server{
		logger:         logging.EnsureLogger(logger),
		chatSessions:   chatSessions,
		catalog:        catalog,
		factoryTarget:  factoryTarget,
		events:         eventsService,
		resolveHomeDir: resolveHomeDir,
		responseBridge: responseBridge,
		wireRecorder:   wireRecorder,
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
// complete non-empty line as one JSON-RPC 2.0 message. Every message except
// "session/prompt" is dispatched and, for a request, answered before the
// next line is read, exactly as if this were a single-threaded loop.
// "session/prompt" is instead dispatched through promptGroup (see
// dispatchAsyncSessionPrompt) so the read loop can keep reading and
// dispatching the lines that follow it on the same connection -- most
// importantly a "session/cancel" notification for the turn that same
// "session/prompt" is still admitting, which must reach handleSessionCancel
// while the prompt's own downstream Factory call is still blocked, not
// after it already returned. safeOut (platformstdio.SerializeWrites)
// serializes every response and notification write so a slow
// "session/prompt" response can never tear or interleave with another
// line's write, even though the two may now be produced concurrently;
// responses across different lines may therefore complete, and be written,
// out of the order their requests were read -- each is still correlated to
// its own request id, which is all JSON-RPC 2.0 promises.
//
// The read loop itself (readConnectionLines) runs on its own goroutine,
// tracked by readGroup, precisely because its underlying scanner.Scan call
// is an ordinary blocking read this method cannot otherwise interrupt.
// Running it concurrently is what lets this method itself react as soon as
// promptGroup's own Failed() channel reports a dispatched "session/prompt"
// response write failure -- the exact "output already broken, but input is
// still open with nothing more coming" case a synchronous read loop would
// otherwise block on forever. When that happens, this method calls
// closeInputForShutdown(in) to unblock readConnectionLines' own blocked Scan
// call -- exactly the same "close the stream to interrupt a pending read"
// technique runServeACP (pkg/transports/cli/root_serve.go) already applies
// from the caller side on context cancellation, applied here from inside the
// connection itself, since a dispatched write failure is a decision this
// method makes on its own, not one the caller's context reflects -- and then
// waits for readGroup to actually finish before returning. So every
// connection goroutine this method starts, including the read loop, has
// always fully returned by the time Serve itself returns; no goroutine is
// ever abandoned mid-read. If in does not support Close (readInputCloser
// returns ok=false), this method still waits for readGroup, matching the
// existing contract that a caller-owned stream without a way to interrupt a
// pending read may leave a blocking call parked until that stream itself
// unblocks it (see runServeACP's own doc comment for the same limitation on
// the context-cancellation path).
//
// A partial trailing frame at EOF, and a response write failure on the read
// loop itself or on a dispatched "session/prompt" (surfaced through
// promptGroup.Failed/Wait), both end the connection with their own error
// instead of being treated as success. Every outstanding "session/prompt"
// tracked by promptGroup is always awaited (promptGroup.Wait, deferred)
// before this method returns, so Serve never returns while a response it may
// still write is in flight.
//
// Because readGroup is always joined before this method returns, no request,
// notification, response write, or new "session/prompt" registration can
// ever happen after Serve returns: the goroutine that could have issued one
// has, by construction, already finished.
func (s *Server) serveConnection(ctx context.Context, connectionID identity.ConnectionID, in io.Reader, out io.Writer) error {
	// Wrapping the connection's own pipes records every byte in both
	// directions, including frames the decoder later rejects and any future
	// write path that does not go through today's two response funnels.
	// Hooking those funnels individually would capture neither.
	if transcript := s.openWireTranscript(connectionID); transcript != nil {
		defer func() { _ = transcript.Close() }()
		conn := string(connectionID)
		in = wiretranscript.TeeReader(in, transcript, conn, wiretranscript.PeerClient, wiretranscript.StreamStdin)
		out = wiretranscript.TeeWriter(out, transcript, conn, wiretranscript.PeerAgent, wiretranscript.StreamStdout)
	}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	scanner.Split(scanCompleteLines)

	safeOut := platformstdio.SerializeWrites(out)
	writeResponseLocked := func(reqIdentity identity.RequestIdentity, result json.RawMessage, rpcErr *acpsdk.RequestError) error {
		return writeResponse(safeOut, reqIdentity, result, rpcErr)
	}

	// notify is this connection's one outbound "session/update" delivery
	// capability, carried through each dispatched request's context (see
	// contextWithPromptNotifier in session_prompt.go) rather than threaded as
	// an explicit parameter -- the only dispatched method that ever uses it
	// is "session/prompt", and adding a parameter every other handler and
	// every existing unit test would have to accept for a capability they
	// never use is exactly the kind of unnecessary plumbing a request-scoped
	// context value avoids. safeOut keeps a notification write from tearing
	// or interleaving with a concurrently-produced response or another
	// notification.
	notify := func(n acpsdk.SessionNotification) error {
		return writeNotification(safeOut, n)
	}

	// Attachments are connection-scoped even though prompts execute
	// concurrently, so every dispatched prompt receives this same safe cache.
	attachments := &attachmentCache{}
	promptFlights := &promptFlightRegistry{}
	var promptGroup taskgroup.Group
	defer func() {
		_ = promptGroup.Wait()
		s.detachAttachments(ctx, attachments)
	}()

	var readGroup taskgroup.Group
	readGroup.Go(func() error {
		return s.readConnectionLines(ctx, scanner, connectionID, writeResponseLocked, notify, attachments, promptFlights, &promptGroup)
	})

	select {
	case <-promptGroup.Failed():
		closeInputForShutdown(in)
		<-readGroup.Done()
		return promptGroup.Err()
	case <-readGroup.Done():
		return readGroup.Wait()
	}
}

// closeInputForShutdown closes in if it supports Close, ignoring the result.
// It is the connection-internal counterpart to runServeACP's own
// context.AfterFunc-triggered close (pkg/transports/cli/root_serve.go): both
// exist for the same reason -- a blocking Scan call on a caller-owned stream
// has no way to be interrupted except by closing that stream -- applied here
// when the connection itself decides to end because a dispatched
// "session/prompt" response write already failed, rather than because the
// caller's context was cancelled. A stream that does not support Close (its
// dynamic type has no Close method) is left exactly as it was; the read loop
// blocked on it can only unblock the same way it always could, when the
// caller-owned stream itself is closed by other means.
func closeInputForShutdown(in io.Reader) {
	if closer, ok := in.(io.Closer); ok {
		_ = closer.Close()
	}
}

// readConnectionLines is serveConnection's read loop, run on its own
// goroutine (see serveConnection's own doc comment for why). It checks both
// ctx.Err() and promptGroup.Failed() before every blocking Scan call, so a
// deliberate shutdown or an already-reported dispatched-prompt write failure
// stops this loop from reading further input as soon as it is between two
// reads. A Scan call already in flight when that happens can only return
// once closeInputForShutdown (called by serveConnection's own concurrent
// select) has closed the underlying stream, at which point Scan reports that
// closure as its own error and this loop returns without acting on the
// closure as if it were a real line.
func (s *Server) readConnectionLines(
	ctx context.Context,
	scanner *bufio.Scanner,
	connectionID identity.ConnectionID,
	writeResponseLocked func(identity.RequestIdentity, json.RawMessage, *acpsdk.RequestError) error,
	notify promptNotifier,
	attachments *attachmentCache,
	promptFlights *promptFlightRegistry,
	promptGroup *taskgroup.Group,
) error {
	var notificationSeq uint64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-promptGroup.Failed():
			return promptGroup.Err()
		default:
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				return err
			}
			return promptGroup.Wait()
		}

		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		raw := append(json.RawMessage(nil), line...)

		stop, err := s.dispatchConnectionLine(ctx, connectionID, raw, &notificationSeq, notify, attachments, writeResponseLocked, promptFlights, promptGroup)
		if stop {
			return err
		}
	}
}

// dispatchConnectionLine decodes and dispatches exactly one already-scanned,
// non-empty JSON-RPC line: a malformed-envelope rejection, a
// "session/cancel" forward, an asynchronous "session/prompt" registration,
// or a synchronous request dispatch (including "session/close") and response
// write. stop reports whether the read loop must end, carrying err as its
// result when it does.
func (s *Server) dispatchConnectionLine(
	ctx context.Context,
	connectionID identity.ConnectionID,
	raw json.RawMessage,
	notificationSeq *uint64,
	notify promptNotifier,
	attachments *attachmentCache,
	writeResponseLocked func(identity.RequestIdentity, json.RawMessage, *acpsdk.RequestError) error,
	promptFlights *promptFlightRegistry,
	promptGroup *taskgroup.Group,
) (stop bool, err error) {
	reqCtx := contextWithAttachmentCache(contextWithPromptNotifier(ctx, notify), attachments)
	env, decodeErr := envelope.Decode(connectionID, *notificationSeq, raw)
	if decodeErr != nil {
		rpcErr, wireID, hasID := protocol.RejectEnvelope(decodeErr)
		var rejectedIdentity identity.RequestIdentity
		if hasID {
			// connectionID is always non-blank for a real connection, and
			// wireID is already validated by envelope.Decode, so
			// NewCorrelated can never fail here.
			rejectedIdentity, _ = identity.NewCorrelated(connectionID, wireID)
		}
		if err := writeResponseLocked(rejectedIdentity, nil, rpcErr); err != nil {
			return true, err
		}
		return false, nil
	}

	if env.IsNotification {
		*notificationSeq++
		if env.Method == acpsdk.AgentMethodSessionCancel {
			s.handleSessionCancel(reqCtx, env)
		}
		return false, nil
	}

	if env.Method == acpsdk.AgentMethodSessionPrompt {
		s.dispatchAsyncSessionPrompt(reqCtx, env, promptFlights, promptGroup, writeResponseLocked)
		return false, nil
	}

	result, rpcErr := s.dispatchRequest(reqCtx, env)
	if err := writeResponseLocked(env.Identity, result, rpcErr); err != nil {
		return true, err
	}
	return false, nil
}

// dispatchAsyncSessionPrompt runs one "session/prompt" exchange on its own
// goroutine, tracked by group, and writes its eventual response through
// writeResponseLocked exactly the way the synchronous dispatch path in
// serveConnection does. A write failure is reported through group's own
// first-error tracking instead of returned, since this goroutine has no
// caller left to return it to by the time it completes; serveConnection's
// read loop observes it through its own deferred group.Wait() call and ends
// the connection with it, the same as a synchronous write failure would.
// Running "session/prompt" this way, instead of inline like every other
// method, is what lets serveConnection's read loop keep consuming the lines
// that follow it on the same connection -- in particular a "session/cancel"
// notification for the same turn -- while its own downstream Factory call
// is still in flight; see serveConnection's own doc comment.
func (s *Server) dispatchAsyncSessionPrompt(
	ctx context.Context,
	env envelope.Envelope,
	flights *promptFlightRegistry,
	group *taskgroup.Group,
	writeResponseLocked func(identity.RequestIdentity, json.RawMessage, *acpsdk.RequestError) error,
) {
	flight, leader := flights.start(env.Identity)
	group.Go(func() error {
		if !leader {
			<-flight.done
			return writeResponseLocked(env.Identity, flight.result, flight.rpcErr)
		}
		result, rpcErr := s.handleSessionPrompt(ctx, env)
		flight.finish(result, rpcErr)
		return writeResponseLocked(env.Identity, result, rpcErr)
	})
}

// dispatchRequest executes one already-decoded, non-notification JSON-RPC
// request envelope: a non-nil result for a successful "initialize",
// "session/new", "session/set_config_option", "session/close", or
// "/factory <value>"-recognized "session/prompt" exchange, or a bounded,
// protocol-safe *acpsdk.RequestError for every rejection. protocol-version policy is
// delegated entirely to the existing V0 negotiation behavior rather than
// re-implemented here. serveConnection dispatches "session/prompt" itself
// (see dispatchAsyncSessionPrompt) instead of routing it through this
// method, so this method's own "session/prompt" case only ever runs when a
// caller (for example an existing unit test) invokes dispatchRequest
// directly.
func (s *Server) dispatchRequest(ctx context.Context, env envelope.Envelope) (json.RawMessage, *acpsdk.RequestError) {
	switch env.Method {
	case acpsdk.AgentMethodInitialize:
		return dispatchInitialize(env.Params)
	case acpsdk.AgentMethodSessionNew:
		return s.handleSessionNew(ctx, env)
	case acpsdk.AgentMethodSessionLoad:
		return s.handleSessionLoad(ctx, env)
	case acpsdk.AgentMethodSessionResume:
		return s.handleSessionResume(ctx, env)
	case acpsdk.AgentMethodSessionSetConfigOption:
		return s.handleSessionSetConfigOption(ctx, env)
	case acpsdk.AgentMethodSessionClose:
		return s.handleSessionClose(ctx, env)
	case acpsdk.AgentMethodSessionPrompt:
		return s.handleSessionPrompt(ctx, env)
	default:
		return nil, protocol.MethodNotFound(env.Method)
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

// openWireTranscript opens this connection's recording, or returns nil when
// recording is disabled or cannot be opened. A recording failure is logged and
// then ignored: a diagnostic artifact must never cost a customer their
// session.
func (s *Server) openWireTranscript(connectionID identity.ConnectionID) acp.WireTranscript {
	if s.wireRecorder == nil {
		return nil
	}
	transcript, err := s.wireRecorder(string(connectionID))
	if err != nil || transcript == nil {
		s.logger.Warn("acp wire transcript unavailable", "connectionId", string(connectionID))
		return nil
	}
	// The path is the whole point of the feature: a customer has to be able to
	// find the file. It is a location they own, not payload, so naming it is
	// consistent with the payload-free logging invariant.
	s.logger.Info("acp wire transcript opened",
		"connectionId", string(connectionID), "path", transcript.Path())
	return transcript
}
