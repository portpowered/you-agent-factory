package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/chat_sessions/transports/acp/factorytarget"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// errSessionNewUnavailable marks a "session/new" call this Server was never
// constructed with the collaborators to serve. It never reaches a client
// verbatim -- classifyDependencyFailure maps it to a bounded internal-error
// response the same way it maps every other dependency failure.
var errSessionNewUnavailable = errors.New("acp: session/new collaborators are not configured")

// handleSessionNew executes one "session/new" request: validate before any
// effect, resolve the catalog-backed Factory target picker using the
// validated editor cwd as the catalog client working root, create exactly
// one Chat Session using the catalog's current target, and project the
// catalog result into the response's single target select option. Any
// failure -- validation, catalog resolution, or session creation -- returns
// before the next effect and produces no successful response, so a rejected
// "session/new" never leaves a transport-owned placeholder session behind.
func (s *Server) handleSessionNew(ctx context.Context, env envelope.Envelope) (json.RawMessage, *acpsdk.RequestError) {
	params, err := session.ValidateNewSession(env.Params)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}

	if s.chatSessions == nil || s.catalog == nil || s.resolveHomeDir == nil {
		return nil, classifyDependencyFailure(errSessionNewUnavailable)
	}

	reqIdentity, err := chatRequestIdentity(env.Identity)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}

	homeDir, err := s.resolveInvocationHomeDir(ctx)
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}

	roots, err := factorydefinitions.ResolveNamedFactoryRoots(homeDir, params.Cwd)
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}

	catalogResult, err := s.catalog.ResolveFactoryTargetCatalog(ctx, chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: operatorsettings.DefaultConfigPath(homeDir),
		FactoryDiscovery: chatsessions.FactoryDiscoveryRoots{
			ProjectRoot: roots.Project,
			GlobalRoot:  roots.Global,
		},
		ClientWorkingRoot: params.Cwd,
	})
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}

	option := factorytarget.FromCatalogResult(catalogResult)
	configOption, err := option.ToSessionConfigOption()
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}

	sessionResult, err := s.chatSessions.CreateSession(ctx, chatsessions.CreateSessionRequest{
		RequestID: reqIdentity,
		InitialTarget: chatsessions.ChatTargetRef{
			Kind: chatsessions.ChatTargetKindFactory,
			Ref:  catalogResult.CurrentTarget,
		},
		WorkingRoot: params.Cwd,
	})
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}

	resp := acpsdk.NewSessionResponse{
		SessionId:     acpsdk.SessionId(sessionResult.Session.ID),
		ConfigOptions: []acpsdk.SessionConfigOption{configOption},
	}
	result, err := json.Marshal(resp)
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}
	s.advertiseAvailableCommands(ctx, sessionResult.Session.ID)
	return result, nil
}

// advertiseAvailableCommands tells the client which prompt commands this
// session accepts, so a customer can discover /factory rather than having to
// already know it exists.
//
// It is best effort: a client that cannot receive the notification still has a
// fully usable session, so a delivery failure must not fail session creation.
// The notifier is already carried on every dispatched request's context, so
// this needs no plumbing of its own.
func (s *Server) advertiseAvailableCommands(ctx context.Context, sessionID string) {
	notify := promptNotifierFromContext(ctx)
	if notify == nil {
		return
	}
	if err := notify(acpsdk.SessionNotification{
		SessionId: acpsdk.SessionId(sessionID),
		Update:    mapping.ProjectAvailableCommands(),
	}); err != nil {
		s.logger.Warn("acp available-commands advertisement failed", "sessionId", sessionID)
	}
}

// classifyDependencyFailure converts a downstream Chat Sessions/catalog
// dependency failure into a bounded, protocol-safe *acpsdk.RequestError. A
// context.Canceled or context.DeadlineExceeded cause -- discoverable via
// errors.Is through the typed failures FactoryTargetCatalogService and
// chatsessions.Service already preserve -- classifies as the ACP-defined
// request-cancelled outcome; every other dependency failure classifies as a
// bounded internal error. Neither branch ever serializes the cause's message
// text, so a raw target value, cwd, credential, provider command, or private
// topology detail the cause happens to mention can never reach the client.
func classifyDependencyFailure(cause error) *acpsdk.RequestError {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return acpsdk.NewRequestCancelled(map[string]any{"reason": "cancelled"})
	}
	return acpsdk.NewInternalError(map[string]any{"reason": "dependency_unavailable"})
}

// chatRequestIdentity converts one ACP transport-side, connection-correlated
// RequestIdentity into the canonical chatsessions.RequestIdentity, so a
// bare JSON-RPC id is never used alone as global Chat Sessions identity:
// the connection id travels with it, and an equal wire id received on a
// different connection converts to a distinct chatsessions.RequestIdentity.
// It rejects a minted (non-connection-correlated) identity, since
// "session/new" is always a request carrying a real JSON-RPC id.
func chatRequestIdentity(id identity.RequestIdentity) (chatsessions.RequestIdentity, error) {
	connectionID, ok := id.ConnectionID()
	if !ok {
		return chatsessions.RequestIdentity{}, errors.New("acp: request identity has no connection")
	}
	wireID, ok := id.WireID()
	if !ok {
		return chatsessions.RequestIdentity{}, errors.New("acp: request identity has no json-rpc id")
	}
	raw, err := wireID.MarshalJSON()
	if err != nil {
		return chatsessions.RequestIdentity{}, err
	}

	var converted chatsessions.RequestIdentity
	if len(raw) > 0 && raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return chatsessions.RequestIdentity{}, err
		}
		converted = chatsessions.RequestIdentity{
			Kind:            chatsessions.RequestIdentityKindJSONRPCString,
			ConnectionID:    string(connectionID),
			JSONRPCStringID: value,
		}
	} else {
		converted = chatsessions.RequestIdentity{
			Kind:            chatsessions.RequestIdentityKindJSONRPCNumber,
			ConnectionID:    string(connectionID),
			JSONRPCNumberID: string(bytes.TrimSpace(raw)),
		}
	}

	if err := converted.Validate(); err != nil {
		return chatsessions.RequestIdentity{}, err
	}
	return converted, nil
}
