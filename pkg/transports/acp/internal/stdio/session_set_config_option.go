package stdio

import (
	"context"
	"encoding/json"
	"errors"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/chat_sessions/transports/acp/factorytarget"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// errSessionSetConfigOptionUnavailable marks a "session/set_config_option"
// call this Server was never constructed with the collaborators to serve. It
// never reaches a client verbatim -- classifyDependencyFailure maps it to a
// bounded internal-error response the same way it maps every other
// dependency failure.
var errSessionSetConfigOptionUnavailable = errors.New("acp: session/set_config_option collaborators are not configured")

// errUnsupportedConfigOption marks a "session/set_config_option" request
// addressing a configId this transport does not implement: the Factory
// target select option is the only configuration option this L1 V0
// boundary exposes.
var errUnsupportedConfigOption = errors.New("acp: unsupported session configuration option")

// errUnsupportedConfigOptionShape marks a "session/set_config_option"
// request carrying the boolean payload variant against the Factory target
// option, which is always the select-option variant.
var errUnsupportedConfigOptionShape = errors.New("acp: factory target configuration option requires a value-id payload")

// errSessionWorkingRootUnknown marks an addressed Chat Session whose
// WorkingRoot is blank: every session "session/new" creates carries the
// validated editor cwd it was created with, so a blank WorkingRoot signals a
// broken invariant in the session record itself, not a caller mistake.
var errSessionWorkingRootUnknown = errors.New("acp: session working root is unknown")

// handleSessionSetConfigOption executes one "session/set_config_option"
// request: validate the option shape before any effect, reject any option
// other than the Factory target select option, then delegate to
// changeTarget for the shared read-revalidate-mutate sequence.
func (s *Server) handleSessionSetConfigOption(ctx context.Context, env envelope.Envelope) (json.RawMessage, *acpsdk.RequestError) {
	parsed, err := session.ValidateSetConfigOption(env.Params)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}
	if parsed.ConfigID != string(acp.FactoryTargetOptionID) {
		return nil, protocol.SafeReject(errUnsupportedConfigOption)
	}
	if parsed.Boolean != nil {
		return nil, protocol.SafeReject(errUnsupportedConfigOptionShape)
	}
	if parsed.ValueID == nil || *parsed.ValueID == "" {
		return nil, protocol.SafeReject(errors.New("acp: value is required"))
	}

	reqIdentity, err := chatRequestIdentity(env.Identity)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}

	configOption, rpcErr := s.changeTarget(ctx, string(parsed.SessionID), *parsed.ValueID, reqIdentity)
	if rpcErr != nil {
		return nil, rpcErr
	}

	resp := acpsdk.SetSessionConfigOptionResponse{ConfigOptions: []acpsdk.SessionConfigOption{configOption}}
	result, err := json.Marshal(resp)
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}
	return result, nil
}

// changeTarget executes the shared target-change sequence used by both
// "session/set_config_option" and the "/factory" command fallback
// (handleSessionPrompt in session_prompt.go): read the addressed Chat
// Session, resolve the requested value through the existing Factory target
// catalog using that session's own WorkingRoot, project the refreshed picker
// from that catalog result, and only once projection succeeds call Chat
// Sessions' SetTarget with the canonical revalidated target, the caller's
// identity, the session id, and the version observed from the read.
// Projection runs before SetTarget precisely so a picker-projection failure
// (e.g. an empty catalog) never mutates the session: any failure -- an
// unknown session, a stale version, a disallowed/uninstalled/malformed/
// root-incompatible target, a picker-projection failure, or a dependency
// fault -- returns before the mutating effect, so a rejected change never
// mutates target, episode history, or session version.
func (s *Server) changeTarget(ctx context.Context, sessionID string, requestedValue string, reqIdentity chatsessions.RequestIdentity) (acpsdk.SessionConfigOption, *acpsdk.RequestError) {
	if s.chatSessions == nil || s.catalog == nil || s.resolveHomeDir == nil {
		return acpsdk.SessionConfigOption{}, classifyDependencyFailure(errSessionSetConfigOptionUnavailable)
	}

	getResult, err := s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: sessionID})
	if err != nil {
		return acpsdk.SessionConfigOption{}, classifyTargetSelectionFailure(err)
	}

	cwd := getResult.Session.WorkingRoot
	if cwd == "" {
		return acpsdk.SessionConfigOption{}, classifyDependencyFailure(errSessionWorkingRootUnknown)
	}

	homeDir, err := s.resolveInvocationHomeDir(ctx)
	if err != nil {
		return acpsdk.SessionConfigOption{}, classifyDependencyFailure(err)
	}

	roots, err := factorydefinitions.ResolveNamedFactoryRoots(homeDir, cwd)
	if err != nil {
		return acpsdk.SessionConfigOption{}, classifyDependencyFailure(err)
	}

	catalogResult, err := s.catalog.ResolveFactoryTargetCatalog(ctx, chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: operatorsettings.DefaultConfigPath(homeDir),
		FactoryDiscovery: chatsessions.FactoryDiscoveryRoots{
			ProjectRoot: roots.Project,
			GlobalRoot:  roots.Global,
		},
		ClientWorkingRoot: cwd,
		CurrentTarget:     requestedValue,
	})
	if err != nil {
		return acpsdk.SessionConfigOption{}, classifyTargetSelectionFailure(err)
	}

	option := factorytarget.FromCatalogResult(catalogResult)
	configOption, err := option.ToSessionConfigOption()
	if err != nil {
		return acpsdk.SessionConfigOption{}, classifyDependencyFailure(err)
	}

	if _, err := s.chatSessions.SetTarget(ctx, chatsessions.SetTargetRequest{
		RequestID:       reqIdentity,
		SessionID:       getResult.Session.ID,
		ExpectedVersion: getResult.Session.Version,
		Target: chatsessions.ChatTargetRef{
			Kind: chatsessions.ChatTargetKindFactory,
			Ref:  catalogResult.CurrentTarget,
		},
	}); err != nil {
		return acpsdk.SessionConfigOption{}, classifyTargetSelectionFailure(err)
	}

	return configOption, nil
}

// classifyTargetSelectionFailure converts a failure from reading a Chat
// Session, resolving the Factory target catalog, or calling SetTarget into
// a bounded, protocol-safe *acpsdk.RequestError. A context.Canceled or
// context.DeadlineExceeded cause classifies as the ACP-defined
// request-cancelled outcome via classifyDependencyFailure. A failure this
// package attributes to the caller's own request -- an unknown session
// (*chatsessions.NotFoundError), a stale expected version
// (*chatsessions.ConflictError), a malformed request value
// (*chatsessions.ValidationError), or a target the catalog rejects as
// malformed, uninstalled, disallowed, working-root-incompatible, or leaving
// no target at all (the corresponding *chatsessions.FactoryTargetCatalogError
// sentinels) -- classifies as a bounded invalid-params rejection via
// protocol.SafeReject. Every other cause -- an unavailable profile or
// catalog dependency -- classifies as a bounded internal error via
// classifyDependencyFailure. No branch ever serializes the cause's message
// text, so a raw target value, cwd, credential, provider command, or
// private topology detail can never reach the client through this
// classification.
func classifyTargetSelectionFailure(cause error) *acpsdk.RequestError {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return classifyDependencyFailure(cause)
	}

	var catalogErr *chatsessions.FactoryTargetCatalogError
	if errors.As(cause, &catalogErr) {
		switch {
		case errors.Is(cause, chatsessions.ErrFactoryTargetReferenceMalformed),
			errors.Is(cause, chatsessions.ErrFactoryTargetNotInstalled),
			errors.Is(cause, chatsessions.ErrFactoryTargetNotAllowed),
			errors.Is(cause, chatsessions.ErrFactoryTargetWorkingRootIncompatible),
			errors.Is(cause, chatsessions.ErrFactoryTargetCatalogEmpty):
			return protocol.SafeReject(cause)
		default:
			return classifyDependencyFailure(cause)
		}
	}

	var notFoundErr *chatsessions.NotFoundError
	if errors.As(cause, &notFoundErr) {
		return protocol.SafeReject(cause)
	}
	var conflictErr *chatsessions.ConflictError
	if errors.As(cause, &conflictErr) {
		return protocol.SafeReject(cause)
	}
	var validationErr *chatsessions.ValidationError
	if errors.As(cause, &validationErr) {
		return protocol.SafeReject(cause)
	}

	return classifyDependencyFailure(cause)
}
