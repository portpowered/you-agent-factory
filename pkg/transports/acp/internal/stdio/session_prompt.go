package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
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

// errFactoryDelegationNotImplemented marks a validated, admitted ordinary
// prompt turn this transport slice does not yet dispatch to a Factory
// Session: starting or invoking a Factory Session and mapping its outcome
// into a final ACP response are later stories' scope. It never reaches a
// client verbatim -- classifyDependencyFailure maps it to the same bounded
// internal-error response every other dependency failure receives.
var errFactoryDelegationNotImplemented = errors.New("acp: factory session delegation is not yet implemented")

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
// id, and the version observed from that read -- no Factory Session is
// started or invoked by this story, so an admitted turn's downstream
// dispatch and terminal response mapping are always reported as the bounded
// not-yet-implemented internal error rather than fabricated success. An
// unknown session (*chatsessions.NotFoundError), a stale expected version
// (*chatsessions.ConflictError), or a busy active turn
// (*chatsessions.BusyError) all classify as a bounded protocol-safe
// rejection and never reach StartTurn or (for the GetSession failures) leave
// any effect at all.
func (s *Server) admitPromptTurn(ctx context.Context, turn session.PromptTurn, reqIdentity chatsessions.RequestIdentity) (json.RawMessage, *acpsdk.RequestError) {
	if s.chatSessions == nil {
		return nil, classifyDependencyFailure(errSessionPromptUnavailable)
	}

	getResult, err := s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: string(turn.SessionID)})
	if err != nil {
		return nil, classifyTurnAdmissionFailure(err)
	}

	if _, err := s.chatSessions.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       reqIdentity,
		SessionID:       getResult.Session.ID,
		ExpectedVersion: getResult.Session.Version,
	}); err != nil {
		return nil, classifyTurnAdmissionFailure(err)
	}

	return nil, classifyDependencyFailure(errFactoryDelegationNotImplemented)
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
