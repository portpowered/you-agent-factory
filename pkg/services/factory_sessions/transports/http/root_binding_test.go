package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestHandlerFromRoot_GetFactorySessionInvokesSessionsRoot(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		sessions: map[string]factorysessions.SessionProjection{
			"session-alpha": {
				Context: factorysessions.ProjectionContext{
					FactorySessionID: "session-alpha",
					Session: &factorysessions.ScopedLiveSessionSummary{
						ID: "session-alpha", FactoryDir: "/workspace/alpha", FolderPath: "/workspace",
						Project: "alpha", Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed},
					},
				},
			},
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetFactorySession(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha", nil), "session-alpha")

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"id":"session-alpha"`) {
		t.Fatalf("response = %d %s, want encoded session from Sessions root", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerFromRoot_ListFactorySessionsInvokesSessionsRoot(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		listSessions: func(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			if request.Scope != factorysessions.SessionListScopeAll {
				t.Fatalf("durable inventory scope = %q, want all", request.Scope)
			}
			return factorysessions.ListSessionsResult{Scope: factorysessions.SessionListScopeLive}, nil
		},
		listReads: func(context.Context) ([]factorysessions.ReadProjection, error) {
			return []factorysessions.ReadProjection{{
				Context: factorysessions.ProjectionContext{
					FactorySessionID: "session-alpha",
					Session: &factorysessions.ScopedLiveSessionSummary{
						ID: "session-alpha", FactoryDir: "/workspace/alpha", FolderPath: "/workspace", Project: "alpha",
					},
				},
			}}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.ListFactorySessions(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions", nil), factoryapi.ListFactorySessionsParams{})

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"id":"session-alpha"`) {
		t.Fatalf("response = %d %s, want encoded session list from Sessions root", recorder.Code, recorder.Body.String())
	}
}

type httpSessionsRootFake struct {
	sessions     map[string]factorysessions.SessionProjection
	listReads    func(context.Context) ([]factorysessions.ReadProjection, error)
	listSessions func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
	onOpen       func(context.Context, factorysessions.OpenRequest) (*factorysessions.OpenResult, error)
	onClose      func(context.Context, string) error
	onStartAsync func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	onPauseDurable func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	onPauseLive    func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
}

var _ factorysessions.Service = (*httpSessionsRootFake)(nil)

func (fake *httpSessionsRootFake) ForRuntime(factorysessions.OpeningBindingRequest) (factorysessions.Service, error) {
	return fake, nil
}

func (fake *httpSessionsRootFake) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	if fake.listReads == nil {
		return nil, nil
	}
	return fake.listReads(ctx)
}

func (fake *httpSessionsRootFake) GetFactorySession(_ context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	if projection, ok := fake.sessions[sessionID]; ok {
		return projection, nil
	}
	return factorysessions.SessionProjection{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) ListSessions(ctx context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	if fake.listSessions == nil {
		return factorysessions.ListSessionsResult{}, nil
	}
	return fake.listSessions(ctx, request)
}

func (fake *httpSessionsRootFake) OpenFactorySession(ctx context.Context, request factorysessions.OpenRequest) (*factorysessions.OpenResult, error) {
	if fake.onOpen != nil {
		return fake.onOpen(ctx, request)
	}
	return &factorysessions.OpenResult{SessionID: factorysessions.DefaultSessionID}, nil
}

func (fake *httpSessionsRootFake) OpenFactorySessionFromFolder(context.Context, string, *factorysessions.TargetRef, bool, bool) (*factorysessions.OpenResult, error) {
	return &factorysessions.OpenResult{SessionID: factorysessions.DefaultSessionID}, nil
}

func (fake *httpSessionsRootFake) GetFactorySessionSyncPreflight(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor, *factorydefinitions.FactorySessionLogicalResolveHint) (factorysessions.SyncPreflightResult, error) {
	return factorysessions.SyncPreflightResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) GetFactorySessionResult(context.Context, string) (factoryruntime.LiveSessionResult, error) {
	return factoryruntime.LiveSessionResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) GetFactorySessionPartialResult(context.Context, string) (factoryruntime.PartialSessionResult, error) {
	return factoryruntime.PartialSessionResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) SubscribeFactoryResponseEvents(context.Context, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	return nil, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	return nil, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) ProbeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) error {
	return factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) ReadDurableFactorySessionEventStream(context.Context, string, factorysessions.EventReconnectRequest) (*factorydefinitions.FactoryEventStream, error) {
	return nil, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) ProbeDurableFactorySessionEvents(context.Context, string, factorysessions.EventReconnectRequest) error {
	return factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) GetEngineStateSnapshotForSession(context.Context, string) (*factoryruntime.StateSnapshot, error) {
	return nil, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) ObserveForSession(context.Context, string, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) PauseLiveFactorySession(ctx context.Context, sessionID string, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if fake.onPauseLive != nil {
		return fake.onPauseLive(ctx, sessionID, control)
	}
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) ResumeLiveFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) CloseFactorySession(ctx context.Context, sessionID string) error {
	if fake.onClose != nil {
		return fake.onClose(ctx, sessionID)
	}
	return factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) PauseDurableFactorySession(ctx context.Context, sessionID string, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if fake.onPauseDurable != nil {
		return fake.onPauseDurable(ctx, sessionID, control)
	}
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) ResumeDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) CancelDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) TerminateDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) ApproveDurableFactorySession(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) RetryDurableFactorySessionDispatch(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) InterruptDurableFactorySessionDispatch(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) StartAsync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	if fake.onStartAsync != nil {
		return fake.onStartAsync(ctx, request)
	}
	return factorysessions.AsyncStartResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) StartSync(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	return factorysessions.SyncStartResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) ResumeInterruptedSession(context.Context, string, factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error) {
	return factorysessions.AsyncStartResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) GetSession(context.Context, string) (factorysessions.SessionReadResult, error) {
	return factorysessions.SessionReadResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) Pause(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) Resume(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) Cancel(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) Terminate(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) Approve(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) RetryDispatch(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) InterruptDispatch(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) GetResult(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	return factorysessions.ResultReadResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *httpSessionsRootFake) ListDispatches(context.Context, string) (factorysessions.ListDispatchesResult, error) {
	return factorysessions.ListDispatchesResult{}, nil
}

func (fake *httpSessionsRootFake) QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	return factorysessions.ListDispatchesResult{}, nil
}

func (fake *httpSessionsRootFake) GetDispatch(context.Context, string, string) (factorysessions.DispatchDetail, error) {
	return factorysessions.DispatchDetail{}, factorysessions.ErrDispatchNotFound
}

func (fake *httpSessionsRootFake) ListArtifacts(context.Context, string) (factorysessions.ListArtifactsResult, error) {
	return factorysessions.ListArtifactsResult{}, nil
}

func (fake *httpSessionsRootFake) GetArtifact(context.Context, string, string) (factorysessions.ArtifactDetail, error) {
	return factorysessions.ArtifactDetail{}, factorysessions.ErrArtifactNotFound
}

func (fake *httpSessionsRootFake) ReadEvents(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	return factorysessions.EventReadResult{}, factorysessions.ErrDurableSessionNotFound
}
