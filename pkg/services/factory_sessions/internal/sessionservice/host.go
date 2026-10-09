package service

import (
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// Host exposes composition-root seams required by the session gateway.
type Host interface {
	controlplane.LiveReadHost
	controlplane.SyncPreflightHost
	controlplane.ResultReadHost
	SessionFactory(string) (factory.Service, error)
	StopLiveSession(string) error
	ObserveLiveLifecycleControl(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error)
}

// keyedSessionHost combines independent readers over the addressed session facts.
// RuntimeRecord/Run selection stays behind the explicit T15 compatibility bridge.
type keyedSessionHost struct {
	sessionIdentityReader
	sessionProjectionReader
	sessionLifecycleReader
	state *sessionruntime.Service
}

func (h keyedSessionHost) RequireSession(sessionID string) (*livesession.LiveSession, error) {
	if h.state == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return runtimebinding.RequireLiveSession(h.state, sessionID)
}

func (h keyedSessionHost) ListLiveSessionIDs() []string {
	if h.state == nil || h.state.Registry() == nil {
		return nil
	}
	return h.state.Registry().IDs()
}

func (h keyedSessionHost) GetLiveSession(sessionID string) *livesession.LiveSession {
	if h.state == nil {
		return nil
	}
	return h.state.Resolve(sessionID)
}

func (h keyedSessionHost) StreamGenerationID(session *livesession.LiveSession) string {
	return runtimebinding.StreamGenerationID(session)
}

func (h keyedSessionHost) LiveSessionEvents(session *livesession.LiveSession) []interfaces.FactoryEvent {
	if h.state == nil {
		return nil
	}
	return runtimebinding.CanonicalEventsFromSession(session)
}

func (h keyedSessionHost) SessionFactory(sessionID string) (factory.Service, error) {
	if h.state == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return runtimebinding.FactoryForSession(h.state, sessionID)
}

func (h keyedSessionHost) JavaScriptCheckpointStore(session *livesession.LiveSession) factory.JavaScriptCheckpointStore {
	if h.state == nil {
		return nil
	}
	return sessionCheckpointStore(session, h.checkpoints)
}

var _ Host = keyedSessionHost{}
