package service

import (
	"context"
	"fmt"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// NewNamedFactoryActivator routes activation to the currently selected record.
// The per-record opening owner remains the T17 compatibility boundary.
func NewNamedFactoryActivator(state *sessionruntime.Service) func(context.Context, string) error {
	return func(ctx context.Context, name string) error {
		if state == nil {
			return factorysessions.ErrRuntimeNotAvailable
		}
		session := state.Current()
		if session == nil {
			return factorysessions.ErrSessionNotFound
		}
		bound := runtimebinding.SessionStateFrom(session)
		if bound == nil {
			return factorysessions.ErrRuntimeNotAvailable
		}
		owner, ok := bound.Owner.(interface {
			ActivateNamedFactory(context.Context, string) error
		})
		if !ok || owner == nil {
			return fmt.Errorf("%w: session activation owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
		}
		return owner.ActivateNamedFactory(ctx, name)
	}
}

// definitionActivationGateway adapts the legacy Definition activation edge to
// the fixed owner using a captured scope. T13's keyed authority remains separate.
type definitionActivationGateway struct{ definitionHost }

func (fs *SessionRuntime) DefinitionActivationGateway() factorysessions.DefinitionActivationGateway {
	return definitionActivationGateway{definitionHost{runtime: fs}}
}
func (g definitionActivationGateway) RunSessionID() string { return g.runtime.runSessionID() }
func (g definitionActivationGateway) SessionForActivation(id string) *factorydefinitions.DefinitionSession {
	return projectDefinitionSession(g.runtime.owner.state.Resolve(id))
}
func (g definitionActivationGateway) NamedFactoryActivationPaths(session *factorydefinitions.DefinitionSession) (string, string) {
	return NamedFactoryActivationPaths(g.runtime.factoryRootDir, g.runtime.dir, g.liveSession(session))
}
func (g definitionActivationGateway) SaveNow() time.Time { return g.runtime.clock.Now().UTC() }
func (g definitionActivationGateway) WithActivationLock(fn func() error) error {
	return g.runtime.owner.state.WithActivationLock(fn)
}
func (g definitionActivationGateway) RequireIdleRuntimeForSession(ctx context.Context, id string) error {
	return g.runtime.requireIdleRuntimeForSession(ctx, id)
}
func (g definitionActivationGateway) RequireIdleBeforeNamedFactoryActivation(ctx context.Context, id string, session *factorydefinitions.DefinitionSession) error {
	live := g.liveSession(session)
	return RequireIdleBeforeNamedActivation(ctx, id, live, runtimebinding.HandleFromSession(live) != nil,
		g.runtime.requireIdleRuntimeForSession, g.runtime.requireIdleRuntime)
}
func (g definitionActivationGateway) ActivateSessionEditableFactory(ctx context.Context, session *factorydefinitions.DefinitionSession, id, root, dir, name, runtimeName string) error {
	return g.runtime.activateSessionEditableFactory(ctx, g.liveSession(session), id, root, dir, name, runtimeName)
}
func (g definitionActivationGateway) SwapPersistedNamedFactoryRuntime(ctx context.Context, id string, session *factorydefinitions.DefinitionSession, root, folder, dir, name string) error {
	return g.runtime.swapPersistedNamedFactoryRuntime(ctx, id, g.liveSession(session), root, folder, dir, name)
}

var _ factorysessions.DefinitionActivationGateway = definitionActivationGateway{}

// keyedDefinitionActivationGateway reads the live authority without retaining a
// final Root or an opening runtime. Runtime ownership remains the T15/T17 bridge.
type keyedDefinitionActivationGateway struct {
	state *sessionruntime.Service
	clock factoryruntime.Clock
}

func NewKeyedDefinitionActivationGateway(state *sessionruntime.Service, clock factoryruntime.Clock) factorydefinitions.DefinitionActivationGateway {
	return keyedDefinitionActivationGateway{state: state, clock: clock}
}

func (g keyedDefinitionActivationGateway) owner(sessionID string) (*SessionRuntime, error) {
	session, err := runtimebinding.RequireLiveSession(g.state, sessionID)
	if err != nil {
		return nil, err
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	owner, ok := bound.Owner.(*SessionRuntime)
	if !ok || owner == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return owner, nil
}

func (g keyedDefinitionActivationGateway) RunSessionID() string {
	if session := g.state.Current(); session != nil {
		if owner, err := g.owner(session.ID); err == nil {
			return owner.runSessionID()
		}
	}
	return factorysessions.DefaultSessionID
}

func (g keyedDefinitionActivationGateway) SessionForActivation(id string) *factorydefinitions.DefinitionSession {
	return projectDefinitionSession(g.state.Resolve(id))
}

func (g keyedDefinitionActivationGateway) RequireSession(id string) (*factorydefinitions.DefinitionSession, error) {
	session, err := runtimebinding.RequireLiveSession(g.state, id)
	return projectDefinitionSession(session), err
}

func (g keyedDefinitionActivationGateway) liveSession(session *factorydefinitions.DefinitionSession) *livesession.LiveSession {
	if session == nil {
		return nil
	}
	if live := g.state.Resolve(session.ID); live != nil {
		return live
	}
	return &livesession.LiveSession{ID: session.ID, IsDefault: session.IsDefault,
		SessionState: livesession.SessionState{FolderPath: session.FolderPath, FactoryDir: session.FactoryDir}}
}

func (g keyedDefinitionActivationGateway) roots(session *factorydefinitions.DefinitionSession) (string, string) {
	id := g.RunSessionID()
	if session != nil {
		id = session.ID
	}
	if owner, err := g.owner(id); err == nil {
		return owner.factoryRootDir, owner.dir
	}
	return "", ""
}

func (g keyedDefinitionActivationGateway) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	root, _ := g.roots(session)
	return logicaltarget.SessionFactoryPersistRoot(root, g.liveSession(session))
}

func (g keyedDefinitionActivationGateway) NamedFactoryActivationPaths(session *factorydefinitions.DefinitionSession) (string, string) {
	root, configured := g.roots(session)
	return NamedFactoryActivationPaths(root, configured, g.liveSession(session))
}

func (g keyedDefinitionActivationGateway) SaveNow() time.Time { return g.clock.Now().UTC() }
func (g keyedDefinitionActivationGateway) WithActivationLock(fn func() error) error {
	return g.state.WithActivationLock(fn)
}

func (g keyedDefinitionActivationGateway) RequireIdleRuntimeForSession(ctx context.Context, id string) error {
	owner, err := g.owner(id)
	if err != nil {
		return err
	}
	return owner.requireIdleRuntimeForSession(ctx, id)
}

func (g keyedDefinitionActivationGateway) RequireIdleBeforeNamedFactoryActivation(ctx context.Context, id string, session *factorydefinitions.DefinitionSession) error {
	owner, err := g.owner(id)
	if err != nil {
		return err
	}
	live := g.liveSession(session)
	return RequireIdleBeforeNamedActivation(ctx, id, live, runtimebinding.HandleFromSession(live) != nil,
		owner.requireIdleRuntimeForSession, owner.requireIdleRuntime)
}

func (g keyedDefinitionActivationGateway) ActivateSessionEditableFactory(ctx context.Context, session *factorydefinitions.DefinitionSession, id, root, dir, name, runtimeName string) error {
	owner, err := g.owner(id)
	if err != nil {
		return err
	}
	return owner.activateSessionEditableFactory(ctx, g.liveSession(session), id, root, dir, name, runtimeName)
}

func (g keyedDefinitionActivationGateway) SwapPersistedNamedFactoryRuntime(ctx context.Context, id string, session *factorydefinitions.DefinitionSession, root, folder, dir, name string) error {
	owner, err := g.owner(id)
	if err != nil {
		return err
	}
	return owner.swapPersistedNamedFactoryRuntime(ctx, id, g.liveSession(session), root, folder, dir, name)
}

var _ factorydefinitions.DefinitionActivationGateway = keyedDefinitionActivationGateway{}
