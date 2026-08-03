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

// errFactoryDelegationNotImplemented marks a validated, admitted ordinary
// prompt turn whose downstream dispatch this transport slice does not yet
// finish: mapping a Factory Session's outcome into a final ACP response, and
// invoking an already-bound Factory Session for a later turn, are later
// stories' scope (003/004). It never reaches a client verbatim --
// classifyDependencyFailure maps it to the same bounded internal-error
// response every other dependency failure receives.
var errFactoryDelegationNotImplemented = errors.New("acp: factory session delegation is not yet implemented")

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
// Once admitted, the turn's episode snapshot decides the one Factory effect
// this story owns: when the episode has no Factory Session ID yet, start one
// and bind the returned identity onto that exact episode/turn/version. An
// already-bound episode (a later turn in the same episode) makes zero start
// calls -- invoking that bound Factory Session is story 003's scope, not
// this one's -- so this method still reports the bounded not-yet-implemented
// failure after a successful start+bind or an already-bound episode, since
// neither downstream dispatch nor final response mapping exist yet.
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

	if startResult.Episode.FactorySessionID == "" {
		if err := s.startFactorySessionForEpisode(ctx, startResult, turn); err != nil {
			return nil, classifyDependencyFailure(err)
		}
	}

	return nil, classifyDependencyFailure(errFactoryDelegationNotImplemented)
}

// startFactorySessionForEpisode starts exactly one Factory Session through
// the consumer-owned Factory Sessions shim for the given turn's newly
// admitted (unbound) episode, and binds the returned identity onto that
// exact episode/turn/version. The start request carries the episode's
// canonical Factory target (Source.FactoryID), the Chat Session's exact
// editor working root, and the validated prompt content -- never a process
// cwd or transport fallback. A blank returned Factory Session ID
// (errEmptyFactorySessionIdentity) or a BindFactorySession failure is
// returned unclassified for the caller to map; no second start is ever
// attempted here.
func (s *Server) startFactorySessionForEpisode(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	turn session.PromptTurn,
) error {
	if s.factoryTarget == nil {
		return errFactoryTargetUnavailable
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
		return err
	}
	if startOutcome.SessionID == "" {
		return errEmptyFactorySessionIdentity
	}

	_, err = s.chatSessions.BindFactorySession(ctx, chatsessions.BindFactorySessionRequest{
		SessionID:        startResult.Session.ID,
		ExpectedVersion:  startResult.Session.Version,
		Episode:          startResult.Episode.Number,
		TurnID:           startResult.Turn.ID,
		FactorySessionID: startOutcome.SessionID,
	})
	return err
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
