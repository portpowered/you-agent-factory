package service_test

import (
	"context"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	"go.uber.org/zap"
)

var serviceTestClock = platformclock.Real{}

func newServiceTestResponseStream() *responsestream.SessionResponseStream {
	return responsestream.NewSessionResponseStream(serviceTestClock)
}

func newServiceTestGateway(host interface {
	factorysessionservice.Host
	controlplane.DurableLifecycleHost
	stream.Host
}) *factorysessionservice.Service {
	registry := responsestream.NewRegistry(newServiceTestResponseStream, serviceTestClock)
	return factorysessionservice.NewWithLiveChangeCoordinator(host,
		stream.NewManagerWithResponseService(host, host, registry, nil),
		nil, nil, nil, nil, nil, host.DurableExecution(), nil, nil, nil)
}

type openTestHost struct {
	requireSession  *livesession.LiveSession
	requireSessionE error
	sessionIDs      []string
	sessions        map[string]*livesession.LiveSession
	projectionErr   error
}

func (h *openTestHost) RequireSession(_ string) (*livesession.LiveSession, error) {
	if h.requireSessionE != nil {
		return nil, h.requireSessionE
	}
	return h.requireSession, nil
}

func (h *openTestHost) ListLiveSessionIDs() []string {
	return h.sessionIDs
}

func (h *openTestHost) GetLiveSession(sessionID string) *livesession.LiveSession {
	if h.sessions == nil {
		return h.requireSession
	}
	return h.sessions[sessionID]
}

func (h *openTestHost) BuildSessionProjectionContext(
	_ context.Context,
	session *livesession.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if h.projectionErr != nil {
		return factorysessions.ProjectionContext{}, h.projectionErr
	}
	return factorysessions.ProjectionContext{
		Session: &factorysessions.ScopedLiveSessionSummary{
			ID: livesession.CanonicalID(session), FactoryDir: session.FactoryDir,
			FolderPath: session.FolderPath, Project: session.Project,
			IsDefault: session.IsDefault, Target: session.Target,
		},
		FactorySessionID: livesession.CanonicalID(session),
	}, nil
}

func (h *openTestHost) ResolveSyncPreflightTarget(_ string, _ *interfaces.FactorySessionLogicalResolveHint) (controlplane.SyncPreflightTarget, error) {
	return controlplane.SyncPreflightTarget{Session: h.requireSession}, nil
}

func (h *openTestHost) BackendScopeID() string {
	return "runtime-test"
}

func (h *openTestHost) LogicalSessionKeyID(session *livesession.LiveSession) string {
	return controlplane.LogicalSessionKeyID(session)
}

func (h *openTestHost) StreamGenerationID(_ *livesession.LiveSession) string {
	return "runtime-test::sess-1"
}

func (h *openTestHost) LiveSessionEvents(_ *livesession.LiveSession) []interfaces.FactoryEvent {
	return nil
}

func (h *openTestHost) SessionFactory(_ string) (factoryruntime.Service, error) {
	return nil, h.requireSessionE
}

func (h *openTestHost) StopLiveSession(_ string) error {
	return h.requireSessionE
}

func (h *openTestHost) ObserveLiveLifecycleControl(
	_ string,
	_ factorysessionexecution.LifecycleControlKind,
	_ factorysessionexecution.ControlRequest,
	_ factorysessionexecution.LifecycleControlOutcome,
	_ factorysessionexecution.LifecycleStatus,
	_ error,
) {
}

func (h *openTestHost) DurableExecution() factorysessionexecution.Service {
	return nil
}

func (h *openTestHost) ResponseStreams(*livesession.LiveSession) *responsestream.StreamSet {
	return nil
}

func (h *openTestHost) NewResponseStream() *responsestream.SessionResponseStream {
	return newServiceTestResponseStream()
}

func (h *openTestHost) CloseResponseStreams(*livesession.LiveSession) {}

func (h *openTestHost) CloseResponseStreamDispatch(*livesession.LiveSession, string) bool {
	return false
}

func (h *openTestHost) JavaScriptCheckpointStore(*livesession.LiveSession) factoryruntime.JavaScriptCheckpointStore {
	return nil
}

func (h *openTestHost) ObserveResponseStreamPublished(*livesession.LiveSession, string, responsestream.Event) {
}

func (h *openTestHost) ObserveResponseStreamCompaction(
	*livesession.LiveSession,
	string,
	string,
	responsestream.CompactionSummary,
) {
}

func (h *openTestHost) ObserveResponseStreamDegraded(
	*livesession.LiveSession,
	string,
	string,
	string,
	*zap.Logger,
	error,
) {
}
