package stdio

import (
	"context"
	"encoding/json"
	"errors"
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
type dispatchOutcome struct {
	outcome  protocol.PromptOutcome
	terminal chatsessions.TurnState
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

	return s.dispatchFactoryTurn(ctx, startResult, turn)
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
// delivers any mapped text via deliverPromptText (the only ACP mechanism
// that carries assistant text, since the response itself never does), and
// terminalizes the running turn on every path: COMPLETED when dispatch and
// response mapping both succeed and the downstream outcome itself reports a
// genuine completed status, CANCELED when the failure cause is
// context.Canceled/context.DeadlineExceeded or the downstream outcome itself
// reports canceled/timed-out, and FAILED for every other dispatch, text-
// delivery, terminal-result, or response-mapping failure. The terminalizing
// AdvanceTurn call always runs -- using the exact session/turn identity
// StartTurn admitted, not a value derived from whatever failed -- so no
// admitted turn is ever left stranded in a non-terminal state regardless of
// which step failed.
func (s *Server) dispatchFactoryTurn(ctx context.Context, startResult chatsessions.StartTurnResult, turn session.PromptTurn) (json.RawMessage, *acpsdk.RequestError) {
	var dispatched dispatchOutcome
	var dispatchErr error
	if startResult.Episode.FactorySessionID == "" {
		dispatched, dispatchErr = s.startFactorySessionForEpisode(ctx, startResult, turn)
	} else {
		dispatched, dispatchErr = s.invokeFactorySessionForEpisode(ctx, startResult, turn)
	}

	terminal := dispatched.terminal
	var result json.RawMessage
	var rpcErr *acpsdk.RequestError
	if dispatchErr != nil {
		terminal = terminalStateForFailure(dispatchErr)
		rpcErr = classifyDependencyFailure(dispatchErr)
	} else if notifyErr := deliverPromptText(promptNotifierFromContext(ctx), startResult.Session.ID, dispatched.outcome.Text); notifyErr != nil {
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

	if _, err := s.chatSessions.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: startResult.Session.ID,
		TurnID:    startResult.Turn.ID,
		Next:      terminal,
	}); err != nil {
		// The turn is still RUNNING (this transition never took effect). If
		// the attempted terminal state was not already the safe FAILED
		// fallback, make one recovery attempt at it -- RUNNING->FAILED is
		// always a legal transition, so this is the one fallback guaranteed
		// available regardless of which terminal state the primary attempt
		// targeted. Without this, a failed terminal AdvanceTurn call would
		// leave the session's ActiveTurnID set forever, stranding every
		// later prompt behind a busy turn that already finished dispatching.
		if terminal != chatsessions.TurnStateFailed {
			s.recoverStrandedTurn(ctx, startResult.Session.ID, startResult.Turn.ID, chatsessions.TurnStateFailed)
		}
		return nil, classifyDependencyFailure(err)
	}

	return result, rpcErr
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
// InvokeFactoryTarget itself returns no Go error: a genuine published
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
// the consumer-owned Factory Sessions shim for the given turn's newly
// admitted (unbound) episode, and binds the returned identity onto that
// exact episode/turn/version. The start request carries the episode's
// canonical Factory target (Source.FactoryID), the Chat Session's exact
// editor working root, and the validated prompt content -- never a process
// cwd or transport fallback. A blank returned Factory Session ID
// (errEmptyFactorySessionIdentity) is returned unclassified for the caller
// to map; no second start is ever attempted here.
//
// StartFactoryTarget's underlying activation (OnDemandFactoryTargetService)
// is synchronous under the hood: it already observes a genuine terminal
// factorysessions.InvocationResult before returning, so this branch's
// returned outcome is protocol.MapFactoryInvocationOutcome's deterministic
// projection of that same real result -- both its ordered primary-result
// text and its terminal status -- exactly like invokeFactorySessionForEpisode
// projects a later turn's invocation, rather than a placeholder that always
// reports success. The returned dispatchOutcome terminalizes to whatever
// factoryInvocationTurnState derives from that published status, so a
// first-turn start that itself completed the dispatch call but published a
// FAILED or TIMED_OUT status still terminalizes the Chat turn accordingly
// instead of always reporting COMPLETED.
//
// A BindFactorySession failure after a successful start is compensated by
// closing the just-opened runtime (best-effort; a close failure is joined
// into the returned error rather than discarded) so a live Factory Session
// is never left both unbound and unreachable -- without this, a later retry
// would leave the original runtime orphaned while opening a second one for
// the still-unbound episode.
func (s *Server) startFactorySessionForEpisode(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	turn session.PromptTurn,
) (dispatchOutcome, error) {
	if s.factoryTarget == nil {
		return dispatchOutcome{}, errFactoryTargetUnavailable
	}

	startReq := factorysessions.StartRequest{
		RequestID: startResult.Turn.ID,
		Source: factorysessions.Source{
			Kind:      factoryruntime.WorkflowSourceKindFactoryID,
			FactoryID: startResult.Episode.Target.Ref,
		},
		// StartRequest has no dedicated content/root fields; Args is the
		// one JSON-compatible channel this published contract offers, so
		// the validated prompt content (in the same work.WorkContentPart
		// shape InvocationRequest.Content already uses) and the session's
		// exact editor working root travel through it rather than being
		// silently dropped or replaced by a process cwd.
		Args: map[string]any{
			"content":     promptContentToWorkParts(turn.Content),
			"workingRoot": startResult.Session.WorkingRoot,
		},
	}

	startOutcome, err := s.factoryTarget.StartFactoryTarget(ctx, startReq)
	if err != nil {
		return dispatchOutcome{}, err
	}
	if startOutcome.SessionID == "" {
		return dispatchOutcome{}, errEmptyFactorySessionIdentity
	}

	if _, err := s.chatSessions.BindFactorySession(ctx, chatsessions.BindFactorySessionRequest{
		SessionID:        startResult.Session.ID,
		ExpectedVersion:  startResult.Session.Version,
		Episode:          startResult.Episode.Number,
		TurnID:           startResult.Turn.ID,
		FactorySessionID: startOutcome.SessionID,
	}); err != nil {
		return dispatchOutcome{}, errors.Join(err, s.factoryTarget.CloseFactoryTarget(ctx, startOutcome.SessionID))
	}

	return dispatchOutcome{
		outcome:  protocol.MapFactoryInvocationOutcome(startOutcome),
		terminal: factoryInvocationTurnState(startOutcome.Status),
	}, nil
}

// invokeFactorySessionForEpisode invokes the given turn's already-bound
// Factory Session exactly once, with that exact bound identity, the
// validated prompt content in the shared work.WorkContentPart shape, the text
// source kind, and the admitted turn's ID as the correlated request ID. It
// never starts a second Factory Session for an already-bound episode -- an
// unbound episode is startFactorySessionForEpisode's job, not this one's.
// The returned outcome is protocol.MapFactoryInvocationOutcome's
// deterministic, safe projection of the shim's own published
// InvocationResult -- its terminal status and only the "text" parts of its
// primary result, never the raw result itself. Unlike the start branch, this
// call is synchronous: on success the returned dispatchOutcome terminalizes
// to whatever factoryInvocationTurnState derives from the invocation's own
// published terminal status, so a genuine Factory failure (InvocationResult
// carrying InvocationTerminalStatusFailed) still terminalizes the Chat turn
// to TurnStateFailed even though InvokeFactoryTarget itself returned no Go
// error.
func (s *Server) invokeFactorySessionForEpisode(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	turn session.PromptTurn,
) (dispatchOutcome, error) {
	if s.factoryTarget == nil {
		return dispatchOutcome{}, errFactoryTargetUnavailable
	}

	requestID := startResult.Turn.ID
	sourceKind := factorysessions.InvocationInputSourceKindText
	invokeResult, err := s.factoryTarget.InvokeFactoryTarget(ctx, startResult.Episode.FactorySessionID, factorysessions.InvocationRequest{
		Content:         promptContentToWorkParts(turn.Content),
		ContentProvided: true,
		RequestID:       &requestID,
		SourceKind:      &sourceKind,
	})
	if err != nil {
		return dispatchOutcome{}, err
	}

	return dispatchOutcome{
		outcome:  protocol.MapFactoryInvocationOutcome(invokeResult),
		terminal: factoryInvocationTurnState(invokeResult.Status),
	}, nil
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
