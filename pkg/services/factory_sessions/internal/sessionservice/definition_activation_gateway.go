package service

import (
	"context"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

type definitionActivationGateway struct {
	runtime *SessionRuntime
}

// NewDefinitionActivationGateway is retained only for the serialized
// construction bridge. Current-Factory operations use the private concrete
// coordinator returned by activationCoordinator instead of publishing this
// capability to Definitions.
func NewDefinitionActivationGateway(runtime *SessionRuntime) factorysessions.DefinitionActivationGateway {
	if runtime == nil {
		return nil
	}
	return definitionActivationGateway{runtime: runtime}
}

func (fs *SessionRuntime) activationCoordinator() *definitionActivationGateway {
	if fs == nil {
		return nil
	}
	return &definitionActivationGateway{runtime: fs}
}

// DefinitionActivationGateway returns the activation gateway owned by this
// Factory Session runtime.
var _ factorysessions.DefinitionActivationGatewayProvider = (*SessionRuntime)(nil)

func (fs *SessionRuntime) DefinitionActivationGateway() factorysessions.DefinitionActivationGateway {
	return NewDefinitionActivationGateway(fs)
}

func (g definitionActivationGateway) callbacks() DefinitionHostCallbacks {
	return DefinitionCallbacks(g.runtime)
}

func (g definitionActivationGateway) RunSessionID() string {
	return g.callbacks().RunSessionID()
}

func (g definitionActivationGateway) SessionForActivation(sessionID string) *factorydefinitions.DefinitionSession {
	return projectDefinitionSession(g.callbacks().SessionForActivation(sessionID))
}

func (g definitionActivationGateway) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	session, err := g.callbacks().RequireSession(sessionID)
	return projectDefinitionSession(session), err
}

func (g definitionActivationGateway) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	return g.callbacks().SessionFactoryPersistRoot(g.liveSession(session))
}

func (g definitionActivationGateway) NamedFactoryActivationPaths(session *factorydefinitions.DefinitionSession) (string, string) {
	return g.callbacks().NamedFactoryActivationPaths(g.liveSession(session))
}

func (g definitionActivationGateway) SaveNow() time.Time {
	return g.callbacks().SaveNow()
}

func (g definitionActivationGateway) WithActivationLock(fn func() error) error {
	return g.callbacks().WithActivationLock(fn)
}

func (g definitionActivationGateway) RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error {
	return g.callbacks().RequireIdleRuntimeForSession(ctx, sessionID)
}

func (g definitionActivationGateway) RequireIdleBeforeNamedFactoryActivation(
	ctx context.Context,
	sessionID string,
	session *factorydefinitions.DefinitionSession,
) error {
	return g.callbacks().RequireIdleBeforeNamedFactoryActivation(ctx, sessionID, g.liveSession(session))
}

func (g definitionActivationGateway) ActivateSessionEditableFactory(
	ctx context.Context,
	session *factorydefinitions.DefinitionSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
) error {
	return g.callbacks().ActivateSessionEditableFactory(
		ctx,
		g.liveSession(session),
		sessionID,
		sessionRootDir,
		factoryDir,
		name,
		runtimeName,
	)
}

func (g definitionActivationGateway) SwapPersistedNamedFactoryRuntime(
	ctx context.Context,
	sessionID string,
	session *factorydefinitions.DefinitionSession,
	persistRoot string,
	folderPath string,
	factoryDir string,
	name string,
) error {
	return g.callbacks().SwapPersistedNamedFactoryRuntime(
		ctx,
		sessionID,
		g.liveSession(session),
		persistRoot,
		folderPath,
		factoryDir,
		name,
	)
}

func (g definitionActivationGateway) liveSession(
	session *factorydefinitions.DefinitionSession,
) *livesession.LiveSession {
	if session == nil {
		return nil
	}
	if live, err := g.callbacks().RequireSession(session.ID); err == nil && live != nil {
		return live
	}
	return &livesession.LiveSession{
		ID: session.ID,
		SessionState: livesession.SessionState{
			FolderPath: session.FolderPath,
			FactoryDir: session.FactoryDir,
		},
		IsDefault: session.IsDefault,
	}
}

var _ factorysessions.DefinitionActivationGateway = definitionActivationGateway{}
