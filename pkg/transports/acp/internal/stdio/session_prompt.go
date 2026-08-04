package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// factoryCommandName is the exact "/factory" slash-command token this
// transport recognizes at the "session/prompt" input boundary
// (final-proposal.md §3 "Fallback"), delegating to the same changeTarget
// sequence "session/set_config_option" uses instead of duplicating catalog
// filtering, reference parsing, expected-version policy, state mutation, or
// error translation.
const factoryCommandName = "/factory"

// errMalformedFactoryCommand marks a recognized "/factory" command attempt
// -- content whose first whitespace-delimited token is exactly
// factoryCommandName -- whose value is missing or whose form carries
// anything other than exactly one value token. It never reaches a client
// verbatim: protocol.SafeReject maps it to the existing bounded
// malformed_request classification, and the raw command text is never
// included in that classification.
var errMalformedFactoryCommand = errors.New("acp: malformed /factory command")

// errSessionPromptUnavailable marks a "session/prompt" call this Server was
// never constructed with the Chat Sessions collaborator to serve. It never
// reaches a client verbatim -- classifyDependencyFailure maps it to a
// bounded internal-error response the same way it maps every other
// dependency failure.
var errSessionPromptUnavailable = errors.New("acp: session/prompt collaborators are not configured")

// errFactoryTargetUnavailable marks a "session/prompt" call this Server was
// never constructed with the Factory Sessions delegation collaborator to
// serve. It never reaches a client verbatim -- classifyDependencyFailure
// maps it to a bounded internal-error response the same way it maps every
// other dependency failure.
var errFactoryTargetUnavailable = errors.New("acp: session/prompt factory target collaborator is not configured")

// errEmptyFactorySessionIdentity marks a Factory Sessions start whose
// returned SessionID was blank. An empty identity can never be committed as
// an episode's Factory Session binding, so this is treated as a failure
// rather than silently proceeding with no bound Factory Session.
var errEmptyFactorySessionIdentity = errors.New("acp: factory session start returned an empty session id")

// promptNotifier sends one outbound "session/update" JSON-RPC notification
// on the connection a "session/prompt" call arrived on. It is carried
// request-scoped through context.Context (see contextWithPromptNotifier),
// not as an explicit parameter on handleSessionPrompt/dispatchRequest: the
// only dispatched method that ever needs it is "session/prompt", so an
// explicit parameter would force every other handler -- and every existing
// test calling handleSessionPrompt directly -- to accept a capability they
// never use.
type promptNotifier func(acpsdk.SessionNotification) error

// promptNotifierContextKey is the unexported context key promptNotifier is
// carried under, so no other package can inject or observe it.
type promptNotifierContextKey struct{}

// contextWithPromptNotifier attaches notify to ctx for the duration of one
// dispatched request.
func contextWithPromptNotifier(ctx context.Context, notify promptNotifier) context.Context {
	return context.WithValue(ctx, promptNotifierContextKey{}, notify)
}

// promptNotifierFromContext retrieves the promptNotifier attached by
// contextWithPromptNotifier, or nil when ctx carries none -- for example
// every existing unit test in this package that calls handleSessionPrompt
// directly with context.Background().
func promptNotifierFromContext(ctx context.Context) promptNotifier {
	notify, _ := ctx.Value(promptNotifierContextKey{}).(promptNotifier)
	return notify
}

// deliverPromptText sends text as exactly one "session/update"
// agent_message_chunk notification -- the only mechanism this protocol
// offers to deliver assistant text, since acpsdk.PromptResponse itself
// carries none -- before the final "session/prompt" response is built. A nil
// notify (no outbound notification capability attached to ctx) or empty text
// is a no-op success. Supported text parts are newline-joined into one chunk
// rather than emitted progressively: this transport slice delivers the
// outcome's already-final, already-mapped text exactly once, not a live
// token stream, so sending it as a single chunk is not the
// fabricated/progressive streaming this L1 V1 slice excludes.
func deliverPromptText(notify promptNotifier, sessionID string, text []string) error {
	if notify == nil || len(text) == 0 {
		return nil
	}
	return notify(acpsdk.SessionNotification{
		SessionId: acpsdk.SessionId(sessionID),
		Update: acpsdk.SessionUpdate{
			AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{
				Content: acpsdk.TextBlock(strings.Join(text, "\n")),
			},
		},
	})
}

// dispatchOutcome carries what a Factory dispatch branch
// (startFactorySessionForEpisode/invokeFactorySessionForEpisode) observed
// about its own downstream call, decoupled from whatever *acpsdk.RequestError
// classification or json.RawMessage response admitPromptTurn eventually
// builds from it: the mapped ACP prompt outcome and the terminal
// chatsessions.TurnState the admitted turn must be advanced to. terminal is
// meaningful only when the branch returns a nil error -- a non-nil error
// overrides it via terminalStateForFailure, since the branch's own
// terminal choice is only ever a genuine downstream-published outcome
// (completed/canceled/timed-out/failed), never a Go-level call failure.
// sessionVersion is the Chat Session's own current version as of the last
// Chat Sessions mutation observed by the time the branch returns --
// startFactorySessionForEpisode's own BindFactorySession result for its
// branch, or a fresh currentSessionVersion re-read for
// invokeFactorySessionForEpisode. Neither branch may use
// startResult.Session.Version directly: dispatchFactoryInvocation can run a
// Chat Sessions-owned Factory response-event bridge concurrently with its
// wrapped Factory invocation (see acp.ResponseBridge), and that bridge
// independently advances Session.Version via AdvanceStreamHead for every
// event it bridges, so a version captured at turn admission -- before that
// concurrent advancement -- is stale by the time either branch's own
// version-guarded calls (BindFactorySession, and deliverPromptUpdates' later
// AcknowledgeAttachment calls) run. See currentSessionVersion's own doc
// comment for why a single fresh read is safe here without its own retry
// loop. liveDelivered reports whether dispatchFactoryInvocation's own
// liveDrain callback (see its own doc comment) already delivered at least
// one canonical agent_message_chunk concurrently with the invocation itself;
// deliverPromptUpdates combines it with its own post-invocation streamTurnUpdates
// result to decide whether the V1 synchronous final-text fallback must be
// suppressed, since a message delivered live is never re-observed by that
// later sweep (both share one attachment cursor).
type dispatchOutcome struct {
	outcome        protocol.PromptOutcome
	terminal       chatsessions.TurnState
	sessionVersion uint64
	liveDelivered  bool
}

// parseFactoryCommand inspects one validated prompt turn's content and
// reports whether it is a "/factory <value>" command attempt. matched is
// false only when the content is not an attempt at this command at all --
// more than one content block, or a first token other than
// factoryCommandName -- in which case content is a genuine prompt this L1
// V0 transport slice does not dispatch to any effect. When matched is true
// and err is non-nil, the content's leading token was factoryCommandName
// but its value was missing or its form carried more than one token: a
// recognized but malformed command attempt, distinct from an unrelated
// prompt.
func parseFactoryCommand(content []session.TextContent) (value string, matched bool, err error) {
	if len(content) != 1 {
		return "", false, nil
	}
	fields := strings.Fields(content[0].Text)
	if len(fields) == 0 || fields[0] != factoryCommandName {
		return "", false, nil
	}
	if len(fields) != 2 {
		return "", true, errMalformedFactoryCommand
	}
	return fields[1], true, nil
}

// handleSessionPrompt executes one "session/prompt" request: validate the
// request before any effect, then either recognize the exact "/factory
// <value>" fallback command form (final-proposal.md §3) and delegate to the
// same changeTarget sequence "session/set_config_option" uses -- no catalog
// filtering, reference parsing, expected-version policy, state mutation, or
// error translation is duplicated here -- or, for every other (genuine,
// non-command) prompt, admit exactly one version-guarded Chat turn via
// admitPromptTurn. Both paths resolve the connection-scoped request
// identity before any Chat Sessions effect.
func (s *Server) handleSessionPrompt(ctx context.Context, env envelope.Envelope) (json.RawMessage, *acpsdk.RequestError) {
	var req acpsdk.PromptRequest
	if err := json.Unmarshal(env.Params, &req); err != nil {
		return nil, protocol.SafeReject(err)
	}
	turn, err := session.ValidatePrompt(req)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}

	value, matched, cmdErr := parseFactoryCommand(turn.Content)
	if cmdErr != nil {
		return nil, protocol.SafeReject(cmdErr)
	}

	reqIdentity, err := chatRequestIdentity(env.Identity)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}

	if matched {
		if _, rpcErr := s.changeTarget(ctx, string(turn.SessionID), value, reqIdentity); rpcErr != nil {
			return nil, rpcErr
		}

		resp := acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}
		result, err := json.Marshal(resp)
		if err != nil {
			return nil, classifyDependencyFailure(err)
		}
		return result, nil
	}

	return s.admitPromptTurn(ctx, turn, reqIdentity)
}

// handleSessionCancel executes one "session/cancel" notification: forward it
// to the addressed Chat Session's currently captured Factory Session, through
// this same Server's Factory Sessions-owned target-execution capability's
// Cancel operation, with the caller's own context and a ControlRequest
// correlated to the addressed session. The captured identity is the
// episode's committed FactorySessionID once a prior turn's bind has
// succeeded, or -- for the exact window between a first turn's StartAsync
// call and its own still-in-flight InvokeFactorySession/BindFactorySession
// completion -- its PendingFactorySessionID (see
// startFactorySessionForEpisode and chatsessions.TargetEpisode's own doc
// comment): that is the real runtime a first, currently-admitted turn is
// blocked inside, and it is exactly the case this transport must be able to
// cancel, not only an already-bound later turn's. Per JSON-RPC 2.0 and the
// ACP protocol, session/cancel is a notification: there is no response to
// build or write for it, so every failure here -- malformed params, an
// unknown Chat Session, an episode with neither a bound nor a pending
// Factory Session yet, a missing collaborator, or the downstream Cancel call
// itself failing -- is a silent no-op. The caller observes cancellation's
// real effect only in the concurrently in-flight "session/prompt" request's
// own eventual response: factoryInvocationTurnState already maps a genuine
// downstream-canceled outcome to TurnStateCanceled/StopReasonCancelled the
// same way it does for every other Factory invocation outcome, so this
// method's only job is reaching the exact captured runtime a prior turn
// started, not reclassifying anything itself.
func (s *Server) handleSessionCancel(ctx context.Context, env envelope.Envelope) {
	if s.chatSessions == nil || s.factoryTarget == nil {
		return
	}
	var req acpsdk.CancelNotification
	if err := json.Unmarshal(env.Params, &req); err != nil {
		return
	}
	params, err := session.ValidateCancel(req)
	if err != nil {
		return
	}

	getResult, err := s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: string(params.SessionID)})
	if err != nil {
		return
	}
	factorySessionID := getResult.Episode.FactorySessionID
	if factorySessionID == "" {
		factorySessionID = getResult.Episode.PendingFactorySessionID
	}
	if factorySessionID == "" {
		return
	}

	_, _ = s.factoryTarget.Cancel(ctx, factorySessionID, factorysessions.ControlRequest{
		RequestID: string(params.SessionID) + "/cancel",
		Reason:    "acp session/cancel",
	})
}

// admitPromptTurn executes the ordinary (non-"/factory") prompt admission
// sequence: read the addressed Chat Session and call its canonical
// StartTurn exactly once with the full request identity, the real session
// id, and the version observed from that read. An unknown session
// (*chatsessions.NotFoundError), a stale expected version
// (*chatsessions.ConflictError), or a busy active turn
// (*chatsessions.BusyError) all classify as a bounded protocol-safe
// rejection and never reach StartTurn or (for the GetSession failures) leave
// any effect at all.
//
// A successful admission is never left stranded in TurnStateAdmitted: it is
// advanced to TurnStateRunning before any Factory effect, and dispatchFactoryTurn
// guarantees an explicit terminal advancement (COMPLETED, CANCELED, or
// FAILED) regardless of how the Factory dispatch and response mapping below
// it turn out, clearing the session's active-turn/busy state in every case
// so a later prompt can always be admitted.
//
// StartTurn treats a reused RequestID as an idempotent retry and returns the
// existing turn instead of admitting a new one; a returned Turn.State other
// than TurnStateAdmitted therefore means this call redelivered a request this
// Session already handled (or is still handling), not a fresh admission.
// respondToRedeliveredTurn classifies that case without ever reaching a
// Factory effect: a still-busy existing turn (RUNNING) rejects the same way
// a genuinely distinct concurrent duplicate would, and an already-terminal
// existing turn returns a deterministic response derived only from that
// turn's own recorded terminal state.
func (s *Server) admitPromptTurn(ctx context.Context, turn session.PromptTurn, reqIdentity chatsessions.RequestIdentity) (json.RawMessage, *acpsdk.RequestError) {
	if s.chatSessions == nil {
		return nil, classifyDependencyFailure(errSessionPromptUnavailable)
	}

	getResult, err := s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: string(turn.SessionID)})
	if err != nil {
		return nil, classifyTurnAdmissionFailure(err)
	}

	startResult, err := s.chatSessions.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       reqIdentity,
		SessionID:       getResult.Session.ID,
		ExpectedVersion: getResult.Session.Version,
	})
	if err != nil {
		return nil, classifyTurnAdmissionFailure(err)
	}

	if startResult.Turn.State != chatsessions.TurnStateAdmitted {
		return respondToRedeliveredTurn(startResult.Session.ID, startResult.Turn)
	}

	if _, err := s.chatSessions.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: startResult.Session.ID,
		TurnID:    startResult.Turn.ID,
		Next:      chatsessions.TurnStateRunning,
	}); err != nil {
		// The turn is still ADMITTED (this transition never took effect), so
		// the only legal terminal fallback from here is CANCELED -- see
		// TurnState.CanTransitionTo. Without this recovery attempt, a failed
		// ADMITTED->RUNNING transition would leave the session's
		// ActiveTurnID set forever, stranding every later prompt behind a
		// busy turn that never actually started running.
		s.recoverStrandedTurn(ctx, startResult.Session.ID, startResult.Turn.ID, chatsessions.TurnStateCanceled)
		return nil, classifyDependencyFailure(err)
	}

	return s.dispatchFactoryTurn(ctx, startResult, turn, reqIdentity)
}

// recoverStrandedTurn makes one best-effort attempt to move turnID to
// fallback after its primary terminalizing AdvanceTurn call already failed,
// so that failure alone can never strand the session's busy/active-turn
// state forever. Its own outcome is intentionally not surfaced to the
// caller: the caller already has the primary failure to report, and no
// further fallback is attempted beyond this single recovery call.
func (s *Server) recoverStrandedTurn(ctx context.Context, sessionID, turnID string, fallback chatsessions.TurnState) {
	_, _ = s.chatSessions.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: sessionID,
		TurnID:    turnID,
		Next:      fallback,
	})
}

// respondToRedeliveredTurn classifies a StartTurn result that identified an
// already-admitted turn (via RequestID reuse) rather than a fresh admission.
// A busy existing turn rejects exactly the way classifyTurnAdmissionFailure
// classifies any other *chatsessions.BusyError; an already-terminal existing
// turn returns the deterministic response its own recorded TurnState implies,
// via turnStateStopReason, with no Factory effect and no further Chat
// Sessions mutation in either case.
func respondToRedeliveredTurn(sessionID string, turn chatsessions.Turn) (json.RawMessage, *acpsdk.RequestError) {
	if turn.State.IsBusy() {
		return nil, classifyTurnAdmissionFailure(&chatsessions.BusyError{
			Value: "Session", ID: sessionID,
			ActiveTurnID: turn.ID, ActiveTurnState: turn.State,
		})
	}

	resp := acpsdk.PromptResponse{StopReason: turnStateStopReason(turn.State)}
	result, err := json.Marshal(resp)
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}
	return result, nil
}

// turnStateStopReason maps an already-terminal Turn's recorded TurnState to
// the ACP stop reason a redelivered request for it deterministically
// reports: TurnStateCompleted to end_turn, TurnStateCanceled to cancelled,
// and TurnStateFailed -- or any other value, which Turn.Validate never
// actually allows here since respondToRedeliveredTurn only reaches this for
// an already-terminal turn -- to the same end_turn safe fallback this
// transport uses elsewhere for a Factory failure.
func turnStateStopReason(state chatsessions.TurnState) acpsdk.StopReason {
	if state == chatsessions.TurnStateCanceled {
		return acpsdk.StopReasonCancelled
	}
	return acpsdk.StopReasonEndTurn
}

// dispatchFactoryTurn runs the one Factory effect a running turn owns --
// starting a fresh Factory Session for an unbound episode or invoking an
// already-bound one -- maps its outcome into the final ACP prompt response,
// delivers canonical chat-session updates and/or V1 fallback text via
// deliverPromptUpdates (see its own doc comment for the suppression rule),
// and terminalizes the running turn on every path: COMPLETED when dispatch
// and response mapping both succeed and the downstream outcome itself
// reports a genuine completed status, CANCELED when the failure cause is
// context.Canceled/context.DeadlineExceeded or the downstream outcome itself
// reports canceled/timed-out, and FAILED for every other dispatch, update-
// delivery, terminal-result, or response-mapping failure. The terminalizing
// AdvanceTurn call always runs -- using the exact session/turn identity
// StartTurn admitted, not a value derived from whatever failed -- so no
// admitted turn is ever left stranded in a non-terminal state regardless of
// which step failed.
func (s *Server) dispatchFactoryTurn(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	turn session.PromptTurn,
	reqIdentity chatsessions.RequestIdentity,
) (json.RawMessage, *acpsdk.RequestError) {
	var dispatched dispatchOutcome
	var dispatchErr error
	if startResult.Episode.FactorySessionID == "" {
		dispatched, dispatchErr = s.startFactorySessionForEpisode(ctx, startResult, turn, reqIdentity.ConnectionID)
	} else {
		dispatched, dispatchErr = s.invokeFactorySessionForEpisode(ctx, startResult, turn, reqIdentity.ConnectionID)
	}

	terminal := dispatched.terminal
	var result json.RawMessage
	var rpcErr *acpsdk.RequestError
	if dispatchErr != nil {
		terminal = terminalStateForFailure(dispatchErr)
		rpcErr = classifyDependencyFailure(dispatchErr)
	} else if notifyErr := s.deliverPromptUpdates(ctx, startResult, dispatched.sessionVersion, reqIdentity, dispatched.liveDelivered, dispatched.outcome.Text); notifyErr != nil {
		terminal = chatsessions.TurnStateFailed
		rpcErr = classifyDependencyFailure(notifyErr)
	} else {
		resp := acpsdk.PromptResponse{StopReason: dispatched.outcome.StopReason}
		marshaled, marshalErr := json.Marshal(resp)
		if marshalErr != nil {
			terminal = chatsessions.TurnStateFailed
			rpcErr = classifyDependencyFailure(marshalErr)
		} else {
			result = marshaled
		}
	}

	if err := s.advanceTurnWithRecovery(ctx, startResult.Session.ID, startResult.Turn.ID, terminal); err != nil {
		return nil, classifyDependencyFailure(err)
	}

	return result, rpcErr
}

// terminalRecoveryOrder lists every terminal TurnState in a fixed priority
// order: FAILED, CANCELED, COMPLETED. RUNNING->{COMPLETED,CANCELED,FAILED}
// are all legal transitions (see TurnState.CanTransitionTo), so this list is
// exactly the set of fallback terminal states advanceTurnWithRecovery can
// still try after its primary attempt fails, regardless of which of the
// three the primary attempt targeted. FAILED and CANCELED are tried before
// COMPLETED so a recovered turn is never fabricated as a genuine success --
// COMPLETED is only ever used as a last-resort fallback (when both FAILED
// and CANCELED are themselves the failed primary or also fail) purely to
// avoid stranding the session's busy state entirely.
var terminalRecoveryOrder = []chatsessions.TurnState{
	chatsessions.TurnStateFailed,
	chatsessions.TurnStateCanceled,
	chatsessions.TurnStateCompleted,
}

// advanceTurnWithRecovery advances turnID to primary and, if that call
// itself fails, retries every other state in terminalRecoveryOrder in turn
// until one succeeds or all have been exhausted. This guarantees the turn
// reaches some terminal state -- releasing the session's busy/active-turn
// state -- as long as any single legal RUNNING->terminal transition still
// succeeds, not only the specific one primary happened to target: a prior
// version of this recovery only ever attempted FAILED as a fallback, which
// left a turn stranded whenever the primary attempt's target was already
// FAILED and that exact call failed. The original failure is what gets
// returned to the caller regardless of whether a fallback attempt
// succeeded, since that failure is what actually happened to the operation
// the caller cares about (dispatch, notification delivery, or response
// marshaling); only the turn's own busy/terminal bookkeeping is repaired
// here.
func (s *Server) advanceTurnWithRecovery(ctx context.Context, sessionID, turnID string, primary chatsessions.TurnState) error {
	_, err := s.chatSessions.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: sessionID, TurnID: turnID, Next: primary,
	})
	if err == nil {
		return nil
	}
	for _, fallback := range terminalRecoveryOrder {
		if fallback == primary {
			continue
		}
		if _, fallbackErr := s.chatSessions.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
			SessionID: sessionID, TurnID: turnID, Next: fallback,
		}); fallbackErr == nil {
			return err
		}
	}
	return err
}

// terminalStateForFailure classifies a Factory dispatch or response-mapping
// failure cause into the TurnState the admitted turn must terminalize to:
// context.Canceled/context.DeadlineExceeded (a caller-cancelled or
// deadline-exceeded request) advances to TurnStateCanceled; every other
// cause advances to TurnStateFailed.
func terminalStateForFailure(cause error) chatsessions.TurnState {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return chatsessions.TurnStateCanceled
	}
	return chatsessions.TurnStateFailed
}

// factoryInvocationTurnState maps one published Factory Session invocation's
// terminal status to the TurnState an admitted turn terminalizes to when
// InvokeFactorySession itself returns no Go error: a genuine published
// completed outcome advances to TurnStateCompleted, a published
// caller-canceled or timed-out outcome advances to TurnStateCanceled, and a
// published failure -- or any other unmapped status -- safely advances to
// TurnStateFailed rather than being reported as a false completion.
func factoryInvocationTurnState(status factorysessions.InvocationTerminalStatus) chatsessions.TurnState {
	switch status {
	case factorysessions.InvocationTerminalStatusCompleted:
		return chatsessions.TurnStateCompleted
	case factorysessions.InvocationTerminalStatusCanceled, factorysessions.InvocationTerminalStatusTimedOut:
		return chatsessions.TurnStateCanceled
	default:
		return chatsessions.TurnStateFailed
	}
}

// startFactorySessionForEpisode starts exactly one Factory Session through
// the Factory Sessions-owned target-execution capability for the given
// turn's newly admitted (unbound) episode -- unless a prior attempt for this
// same episode already started one and durably recorded it as pending (see
// below), in which case it reuses that identity instead of starting a
// second one -- then dispatches this turn's validated prompt content into
// it via InvokeFactorySession (the exact same operation
// invokeFactorySessionForEpisode uses for every later turn) and binds the
// identity onto the episode.
//
// StartAsync itself only opens the runtime: the shared
// factorysessions.Service.StartAsync it forwards to has no dedicated
// content field and, more fundamentally, its Source vocabulary only resolves
// named JavaScript workflow factories, not an ordinary packaged Factory --
// see ondemandtarget.Service.StartAsync's own doc comment. So this turn's
// content, source kind, and correlated request ID travel through the
// immediate follow-up InvokeFactorySession call instead, whose real, terminal
// InvocationResult (ordered primary-result text included) is what
// protocol.MapFactoryInvocationOutcome projects -- exactly the same
// truthful-outcome guarantee invokeFactorySessionForEpisode already provides
// for later turns, not a placeholder that always reports success. A blank
// SessionID returned by StartAsync (errEmptyFactorySessionIdentity)
// is returned unclassified for the caller to map.
//
// A start and its bind are not atomic against this transport's own process
// -- BindFactorySession can fail after StartAsync already succeeded
// -- so this method reconciles across separate calls through the singular
// Chat/Factory Sessions authority itself, not this Server instance: the
// admitted episode snapshot's own Episode.PendingFactorySessionID (durably
// recorded via RecordPendingFactorySession right after a successful start,
// before the bind attempt) is the reconciliation record, so a retry survives
// this Server being reconstructed, not just staying alive. A later admitted
// turn for that same still-unbound episode (whether the original bind is
// still failing, or a genuinely distinct retry request) observes the pending
// identity in its own fresh episode snapshot and dispatches into that exact
// already-live runtime instead of starting a second one, then retries the
// bind. A successful bind itself clears the pending record (see
// chatsessions.Service.BindFactorySession). Only a
// *chatsessions.FactorySessionConflictError -- a different identity already
// won the episode -- abandons the pending runtime: it is closed (best-effort;
// a close failure is joined into the returned error) and the pending record
// is explicitly cleared, since the next call will observe the already-bound
// episode and correctly invoke the winner instead. Every other bind failure
// (for example a transient version conflict) keeps the pending record and
// the runtime open, so no later retry can ever lose track of it and start a
// second Factory Session for the same episode.
// factoryStartRequestID derives the stable idempotency key
// startFactorySessionForEpisode passes as StartRequest.RequestID: the
// session ID and episode number, both fixed for the life of one target
// episode, never the admitted Turn's own ID (which is different on every
// retry). This is what lets a retried start -- for example after a
// successful StartAsync whose immediately-following
// RecordPendingFactorySession call failed -- converge on the exact same
// on-demand Factory Sessions activation instead of starting a second one.
func factoryStartRequestID(sessionID string, episode uint64) string {
	return fmt.Sprintf("%s/episode/%d", sessionID, episode)
}

func (s *Server) startFactorySessionForEpisode(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	turn session.PromptTurn,
	connectionID string,
) (dispatchOutcome, error) {
	if s.factoryTarget == nil {
		return dispatchOutcome{}, errFactoryTargetUnavailable
	}

	factorySessionID := startResult.Episode.PendingFactorySessionID
	if factorySessionID == "" {
		startReq := factorysessions.StartRequest{
			// A stable per-episode key, not the admitted Turn's own ID: a
			// retry of this same still-unbound episode (whether the original
			// RecordPendingFactorySession call below failed, or a genuinely
			// distinct later turn observes the episode still unbound) reuses
			// this exact RequestID, so ondemandtarget.Service.StartAsync's
			// own request-scoped deduplication converges on the identical
			// runtime instead of opening a second one -- see that method's
			// doc comment. The Turn's own ID changes on every retry and
			// would defeat this.
			RequestID: factoryStartRequestID(startResult.Session.ID, startResult.Episode.Number),
			Source: factorysessions.Source{
				Kind:      factoryruntime.WorkflowSourceKindFactoryID,
				FactoryID: startResult.Episode.Target.Ref,
			},
			// StartRequest has no dedicated root field; Args is the one
			// JSON-compatible channel this published contract offers, so the
			// session's exact editor working root travels through it rather
			// than being silently dropped or replaced by a process cwd.
			Args: map[string]any{
				"workingRoot": startResult.Session.WorkingRoot,
			},
		}
		startOutcome, err := s.factoryTarget.StartAsync(ctx, startReq)
		if err != nil {
			return dispatchOutcome{}, err
		}
		if startOutcome.SessionID == "" {
			return dispatchOutcome{}, errEmptyFactorySessionIdentity
		}
		factorySessionID = startOutcome.SessionID

		if _, err := s.chatSessions.RecordPendingFactorySession(ctx, chatsessions.RecordPendingFactorySessionRequest{
			SessionID:        startResult.Session.ID,
			ExpectedVersion:  startResult.Session.Version,
			Episode:          startResult.Episode.Number,
			TurnID:           startResult.Turn.ID,
			FactorySessionID: factorySessionID,
		}); err != nil {
			return dispatchOutcome{}, err
		}
	}

	requestID := startResult.Turn.ID
	sourceKind := factorysessions.InvocationInputSourceKindText
	outcome, liveDelivered, err := s.dispatchFactoryInvocation(ctx, connectionID, startResult.Session.ID, startResult.Session.Version, factorySessionID,
		func(invokeCtx context.Context) (factorysessions.InvocationResult, error) {
			return s.factoryTarget.InvokeFactorySession(invokeCtx, factorySessionID, factorysessions.InvocationRequest{
				Content:         promptContentToWorkParts(turn.Content),
				ContentProvided: true,
				RequestID:       &requestID,
				SourceKind:      &sourceKind,
			})
		},
	)
	if err != nil {
		return dispatchOutcome{}, err
	}
	outcome.SessionID = factorySessionID

	bindResult, err := s.bindStartedFactorySession(ctx, startResult, factorySessionID)
	if err != nil {
		return dispatchOutcome{}, err
	}

	return dispatchOutcome{
		outcome:        protocol.MapFactoryInvocationOutcome(outcome),
		terminal:       factoryInvocationTurnState(outcome.Status),
		sessionVersion: bindResult.Session.Version,
		liveDelivered:  liveDelivered,
	}, nil
}

// bindStartedFactorySession binds factorySessionID onto startResult's
// episode using a freshly re-read session version (see
// currentSessionVersion's own doc comment for why startResult.Session.Version
// itself may already be stale by this point). On a genuine
// *chatsessions.FactorySessionConflictError -- a different identity already
// won the episode -- it abandons this call's own started runtime: the
// pending record is cleared and the runtime is closed (best-effort; a close
// failure is joined into the returned error) so a later retry observes the
// already-bound episode and invokes the winner instead, matching
// startFactorySessionForEpisode's own doc comment.
func (s *Server) bindStartedFactorySession(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	factorySessionID string,
) (chatsessions.BindFactorySessionResult, error) {
	sessionVersion, err := s.currentSessionVersion(ctx, startResult.Session.ID, startResult.Session.Version)
	if err != nil {
		return chatsessions.BindFactorySessionResult{}, err
	}

	bindResult, err := s.chatSessions.BindFactorySession(ctx, chatsessions.BindFactorySessionRequest{
		SessionID:        startResult.Session.ID,
		ExpectedVersion:  sessionVersion,
		Episode:          startResult.Episode.Number,
		TurnID:           startResult.Turn.ID,
		FactorySessionID: factorySessionID,
	})
	if err != nil {
		var conflictErr *chatsessions.FactorySessionConflictError
		if errors.As(err, &conflictErr) {
			_, clearErr := s.chatSessions.RecordPendingFactorySession(ctx, chatsessions.RecordPendingFactorySessionRequest{
				SessionID:       startResult.Session.ID,
				ExpectedVersion: sessionVersion,
				Episode:         startResult.Episode.Number,
				TurnID:          startResult.Turn.ID,
			})
			return chatsessions.BindFactorySessionResult{}, errors.Join(err, clearErr, s.factoryTarget.CloseFactorySession(ctx, factorySessionID))
		}
		return chatsessions.BindFactorySessionResult{}, err
	}
	return bindResult, nil
}

// invokeFactorySessionForEpisode invokes the given turn's already-bound
// Factory Session exactly once, with that exact bound identity, the
// validated prompt content in the shared work.WorkContentPart shape, the text
// source kind, and the admitted turn's ID as the correlated request ID. It
// never starts a second Factory Session for an already-bound episode -- an
// unbound episode is startFactorySessionForEpisode's job, not this one's.
// The returned outcome is protocol.MapFactoryInvocationOutcome's
// deterministic, safe projection of the capability's own published
// InvocationResult -- its terminal status and only the "text" parts of its
// primary result, never the raw result itself. Unlike the start branch, this
// call is synchronous: on success the returned dispatchOutcome terminalizes
// to whatever factoryInvocationTurnState derives from the invocation's own
// published terminal status, so a genuine Factory failure (InvocationResult
// carrying InvocationTerminalStatusFailed) still terminalizes the Chat turn
// to TurnStateFailed even though InvokeFactorySession itself returned no Go
// error.
func (s *Server) invokeFactorySessionForEpisode(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	turn session.PromptTurn,
	connectionID string,
) (dispatchOutcome, error) {
	if s.factoryTarget == nil {
		return dispatchOutcome{}, errFactoryTargetUnavailable
	}

	requestID := startResult.Turn.ID
	sourceKind := factorysessions.InvocationInputSourceKindText
	invokeResult, liveDelivered, err := s.dispatchFactoryInvocation(ctx, connectionID, startResult.Session.ID, startResult.Session.Version, startResult.Episode.FactorySessionID,
		func(invokeCtx context.Context) (factorysessions.InvocationResult, error) {
			return s.factoryTarget.InvokeFactorySession(invokeCtx, startResult.Episode.FactorySessionID, factorysessions.InvocationRequest{
				Content:         promptContentToWorkParts(turn.Content),
				ContentProvided: true,
				RequestID:       &requestID,
				SourceKind:      &sourceKind,
			})
		},
	)
	if err != nil {
		return dispatchOutcome{}, err
	}

	sessionVersion, err := s.currentSessionVersion(ctx, startResult.Session.ID, startResult.Session.Version)
	if err != nil {
		return dispatchOutcome{}, err
	}

	return dispatchOutcome{
		outcome:        protocol.MapFactoryInvocationOutcome(invokeResult),
		terminal:       factoryInvocationTurnState(invokeResult.Status),
		sessionVersion: sessionVersion,
		liveDelivered:  liveDelivered,
	}, nil
}

// currentSessionVersion re-reads sessionID's current Session.Version through
// GetSession rather than trusting a value captured before or during
// dispatchFactoryInvocation. dispatchFactoryInvocation may run a Chat
// Sessions-owned Factory response-event bridge concurrently with the
// synchronous Factory invocation it wraps (see acp.ResponseBridge); that
// bridge independently advances the session's Version via AdvanceStreamHead
// for every event it bridges, so a version read at turn admission --
// startResult.Session.Version, this method's fallback -- can already be
// stale by the time dispatchFactoryInvocation returns. It is safe to read
// fresh here without its own retry loop because dispatchFactoryInvocation
// always fully joins its bridge before returning (see
// responsebridge.Service.Run's own doc comment): no further
// concurrent mutation from that bridge can race this read. A nil
// s.chatSessions (a narrower slice construction that never reaches this
// method in production, since admitPromptTurn already rejects a nil
// s.chatSessions before ever calling dispatchFactoryTurn) falls back to the
// caller-supplied value rather than panicking.
func (s *Server) currentSessionVersion(ctx context.Context, sessionID string, fallback uint64) (uint64, error) {
	if s.chatSessions == nil {
		return fallback, nil
	}
	result, err := s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: sessionID})
	if err != nil {
		return 0, err
	}
	return result.Session.Version, nil
}

// promptContentToWorkParts converts validated ACP text prompt content into
// the shared work.WorkContentPart shape InvocationRequest.Content already
// uses, so a Factory Session's input carries the same content vocabulary
// regardless of whether it reached that Factory Session through a start or
// an invoke.
func promptContentToWorkParts(content []session.TextContent) []work.WorkContentPart {
	parts := make([]work.WorkContentPart, len(content))
	for i, c := range content {
		parts[i] = work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: c.Text}
	}
	return parts
}

// classifyTurnAdmissionFailure converts a failure from reading a Chat
// Session or calling StartTurn into a bounded, protocol-safe
// *acpsdk.RequestError. A context.Canceled or context.DeadlineExceeded
// cause classifies as the ACP-defined request-cancelled outcome via
// classifyDependencyFailure. A failure this package attributes to the
// caller's own request -- an unknown session (*chatsessions.NotFoundError),
// a stale expected version (*chatsessions.ConflictError), a busy active turn
// (*chatsessions.BusyError), or a malformed request value
// (*chatsessions.ValidationError) -- classifies as a bounded invalid-params
// rejection via protocol.SafeReject. Every other cause classifies as a
// bounded internal error via classifyDependencyFailure. No branch ever
// serializes the cause's message text, so a raw prompt, session id, or
// private topology detail can never reach the client through this
// classification.
func classifyTurnAdmissionFailure(cause error) *acpsdk.RequestError {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return classifyDependencyFailure(cause)
	}

	var notFoundErr *chatsessions.NotFoundError
	if errors.As(cause, &notFoundErr) {
		return protocol.SafeReject(cause)
	}
	var conflictErr *chatsessions.ConflictError
	if errors.As(cause, &conflictErr) {
		return protocol.SafeReject(cause)
	}
	var busyErr *chatsessions.BusyError
	if errors.As(cause, &busyErr) {
		return protocol.SafeReject(cause)
	}
	var validationErr *chatsessions.ValidationError
	if errors.As(cause, &validationErr) {
		return protocol.SafeReject(cause)
	}

	return classifyDependencyFailure(cause)
}
