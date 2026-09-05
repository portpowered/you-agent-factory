package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
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

func TestHandlerFromRoot_HumanApprovalReadUsesSessionProjection(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		sessions: map[string]factorysessions.SessionProjection{
			"session-approval": {
				Runtime: factorysessions.RuntimeProjection{
					PendingHumanApprovals: []factorydefinitions.FactoryWorldHumanApproval{{
						ApprovalID: "approval-1", SessionID: "session-approval", DispatchID: "dispatch-1",
						WorkstationID: "approval-workstation", WorkstationName: "Release Approval",
						Decisions: []factorydefinitions.HumanApprovalDecision{
							factorydefinitions.HumanApprovalDecisionApprove,
							factorydefinitions.HumanApprovalDecisionReject,
						},
						Status: factorydefinitions.HumanApprovalStatusPending, WorkItemIDs: []string{"work-1"},
						EventID: "event-approval-1",
					}},
				},
			},
			"session-empty": {},
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())

	listRecorder := httptest.NewRecorder()
	handler.ListHumanApprovalsBySessionId(listRecorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-approval/approvals", nil),
		factoryapi.SessionID("session-approval"), factoryapi.ListHumanApprovalsBySessionIdParams{})
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), `"approvalId":"approval-1"`) ||
		!strings.Contains(listRecorder.Body.String(), `"workIds":["work-1"]`) {
		t.Fatalf("list response = %d %s, want session-scoped pending approval", listRecorder.Code, listRecorder.Body.String())
	}
	emptyRecorder := httptest.NewRecorder()
	handler.ListHumanApprovalsBySessionId(emptyRecorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-empty/approvals", nil),
		factoryapi.SessionID("session-empty"), factoryapi.ListHumanApprovalsBySessionIdParams{})
	if emptyRecorder.Code != http.StatusOK || !strings.Contains(emptyRecorder.Body.String(), `"approvals":[]`) {
		t.Fatalf("empty list response = %d %s, want deterministic empty array", emptyRecorder.Code, emptyRecorder.Body.String())
	}

	getRecorder := httptest.NewRecorder()
	handler.GetHumanApprovalBySessionId(getRecorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-approval/approvals/approval-1", nil),
		factoryapi.SessionID("session-approval"), factoryapi.HumanApprovalID("approval-1"))
	if getRecorder.Code != http.StatusOK || !strings.Contains(getRecorder.Body.String(), `"sessionId":"session-approval"`) {
		t.Fatalf("get response = %d %s, want session-scoped approval resource", getRecorder.Code, getRecorder.Body.String())
	}

	statusRecorder := httptest.NewRecorder()
	handler.ListHumanApprovalsBySessionId(statusRecorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-approval/approvals?status=APPROVED", nil),
		factoryapi.SessionID("session-approval"), factoryapi.ListHumanApprovalsBySessionIdParams{
			Status: func() *factoryapi.ListHumanApprovalsBySessionIdParamsStatus {
				status := factoryapi.ListHumanApprovalsBySessionIdParamsStatus("APPROVED")
				return &status
			}(),
		})
	if statusRecorder.Code != http.StatusBadRequest || !strings.Contains(statusRecorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("malformed status response = %d %s, want typed bad request", statusRecorder.Code, statusRecorder.Body.String())
	}

	unknownRecorder := httptest.NewRecorder()
	handler.GetHumanApprovalBySessionId(unknownRecorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-approval/approvals/missing", nil),
		factoryapi.SessionID("session-approval"), factoryapi.HumanApprovalID("missing"))
	if unknownRecorder.Code != http.StatusNotFound || !strings.Contains(unknownRecorder.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("unknown approval response = %d %s, want typed not-found", unknownRecorder.Code, unknownRecorder.Body.String())
	}
}

type httpSessionsRootFake struct {
	sessions          map[string]factorysessions.SessionProjection
	getSession        func(context.Context, string) (factorysessions.SessionProjection, error)
	listReads         func(context.Context) ([]factorysessions.ReadProjection, error)
	listSessions      func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
	onOpen            func(context.Context, factorysessions.OpenRequest) (*factorysessions.OpenResult, error)
	onClose           func(context.Context, string) error
	onDelete          func(context.Context, string) error
	onStartAsync      func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	onPauseDurable    func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	onPauseLive       func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	onCancelLive      func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	onTerminateLive   func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	onApplyLiveChange func(context.Context, string, factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error)
}

var _ factorysessions.Service = (*httpSessionsRootFake)(nil)

func (fake *httpSessionsRootFake) Start(_ context.Context, _ factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	return factorysessions.SessionStartResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) Invoke(_ context.Context, _ factorysessions.SessionInvokeRequest) (factorysessions.InvocationResult, error) {
	return factorysessions.InvocationResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) Get(_ context.Context, _ factorysessions.SessionGetRequest) (factorysessions.SessionGetResult, error) {
	return factorysessions.SessionGetResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) List(_ context.Context, _ factorysessions.SessionListRequest) (factorysessions.SessionListResult, error) {
	return factorysessions.SessionListResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) Control(_ context.Context, _ factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error) {
	return factorysessions.SessionControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) ReadResult(_ context.Context, _ factorysessions.SessionResultReadRequest) (factorysessions.SessionResultReadResult, error) {
	return factorysessions.SessionResultReadResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) SubscribeResponses(_ context.Context, _ factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error) {
	return factorysessions.SessionResponseSubscriptionResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) ActivateNamedFactory(context.Context, string) error {
	return factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	return factorysessions.InvocationResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	if fake.listReads == nil {
		return nil, nil
	}
	return fake.listReads(ctx)
}

func (fake *httpSessionsRootFake) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	if fake.getSession != nil {
		return fake.getSession(ctx, sessionID)
	}
	if projection, ok := fake.sessions[sessionID]; ok {
		return projection, nil
	}
	return factorysessions.SessionProjection{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) ApplyLiveChange(ctx context.Context, sessionID string, request factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error) {
	if fake.onApplyLiveChange != nil {
		return fake.onApplyLiveChange(ctx, sessionID, request)
	}
	return factorysessions.LiveChangeResult{}, factorysessions.ErrLiveChangeApplicationUnavailable
}

func (fake *httpSessionsRootFake) RecoverLiveChange(context.Context, string, string) (factorysessions.LiveChangeResult, error) {
	return factorysessions.LiveChangeResult{}, factorysessions.ErrLiveChangeApplicationUnavailable
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

func (fake *httpSessionsRootFake) PauseLiveFactorySession(ctx context.Context, sessionID string, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if fake.onPauseLive != nil {
		return fake.onPauseLive(ctx, sessionID, control)
	}
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) ResumeLiveFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) CancelLiveFactorySession(ctx context.Context, sessionID string, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if fake.onCancelLive != nil {
		return fake.onCancelLive(ctx, sessionID, control)
	}
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) TerminateLiveFactorySession(ctx context.Context, sessionID string, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if fake.onTerminateLive != nil {
		return fake.onTerminateLive(ctx, sessionID, control)
	}
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) CloseFactorySession(ctx context.Context, sessionID string) error {
	if fake.onClose != nil {
		return fake.onClose(ctx, sessionID)
	}
	return factorysessions.ErrSessionNotFound
}

func (fake *httpSessionsRootFake) DeleteFactorySession(ctx context.Context, sessionID string) error {
	if fake.onDelete != nil {
		return fake.onDelete(ctx, sessionID)
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

func (fake *httpSessionsRootFake) Pause(ctx context.Context, sessionID string, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if fake.onPauseDurable != nil {
		return fake.onPauseDurable(ctx, sessionID, control)
	}
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

type recordingRequestPreparation struct {
	factorysessionshttp.RequestPreparation
	resultMode    string
	afterEventID  string
	afterSequence *int
	resultErr     error
	reconnectErr  error
}

func (p *recordingRequestPreparation) PrepareResult(
	request factorysessions.ResultRequest,
) (factorysessions.ResultRequest, error) {
	p.resultMode = string(request.Mode)
	if p.resultErr != nil {
		return factorysessions.ResultRequest{}, p.resultErr
	}
	return factorysessions.ResultRequest{Mode: "final", IncludeArtifacts: !request.IncludeArtifacts}, nil
}

func (p *recordingRequestPreparation) PrepareEventReconnect(
	request factorysessions.EventReconnectRequest,
) (factorysessions.EventReconnectRequest, error) {
	p.afterEventID = request.AfterEventID
	p.afterSequence = request.AfterSequence
	if p.reconnectErr != nil {
		return factorysessions.EventReconnectRequest{}, p.reconnectErr
	}
	normalized := 9
	return factorysessions.EventReconnectRequest{AfterEventID: "normalized", AfterSequence: &normalized}, nil
}

func TestDurableRequestPreparation_CarriesResultFieldsThroughTheServiceRole(t *testing.T) {
	t.Parallel()

	role := &recordingRequestPreparation{}
	adapter := factorysessionshttp.NewDurableRequestPreparation(role)
	prepared, err := adapter.PrepareResult(factorysessionmapping.DurableResultInput{
		Mode: "partial", IncludeArtifacts: true,
	})
	if err != nil {
		t.Fatalf("PrepareResult: %v", err)
	}
	if role.resultMode != "partial" {
		t.Fatalf("service role received mode = %q, want partial", role.resultMode)
	}
	if prepared.Mode != "final" || prepared.IncludeArtifacts {
		t.Fatalf("prepared = %#v, want normalized mode final and includeArtifacts false", prepared)
	}
}

func TestDurableRequestPreparation_CarriesReconnectFieldsThroughTheServiceRole(t *testing.T) {
	t.Parallel()

	role := &recordingRequestPreparation{}
	adapter := factorysessionshttp.NewDurableRequestPreparation(role)
	requested := 4
	prepared, err := adapter.PrepareEventReconnect(factorysessionmapping.DurableEventReconnectInput{
		AfterEventID: "event-1", AfterSequence: &requested,
	})
	if err != nil {
		t.Fatalf("PrepareEventReconnect: %v", err)
	}
	if role.afterEventID != "event-1" || role.afterSequence == nil || *role.afterSequence != 4 {
		t.Fatalf("service role received %q/%v, want event-1/4", role.afterEventID, role.afterSequence)
	}
	if prepared.AfterEventID != "normalized" || prepared.AfterSequence == nil || *prepared.AfterSequence != 9 {
		t.Fatalf("prepared = %#v, want normalized/9", prepared)
	}
}

func TestDurableRequestPreparation_ReportsServiceFailuresAndAbsentRoles(t *testing.T) {
	t.Parallel()

	failure := errors.New("normalization rejected")
	role := &recordingRequestPreparation{resultErr: failure, reconnectErr: failure}
	adapter := factorysessionshttp.NewDurableRequestPreparation(role)
	if _, err := adapter.PrepareResult(factorysessionmapping.DurableResultInput{}); !errors.Is(err, failure) {
		t.Fatalf("PrepareResult error = %v, want %v", err, failure)
	}
	if _, err := adapter.PrepareEventReconnect(factorysessionmapping.DurableEventReconnectInput{}); !errors.Is(err, failure) {
		t.Fatalf("PrepareEventReconnect error = %v, want %v", err, failure)
	}
	if factorysessionshttp.NewDurableRequestPreparation(nil) != nil {
		t.Fatal("absent preparation role should not produce a bound adapter")
	}
	var typedNil *recordingRequestPreparation
	if factorysessionshttp.NewDurableRequestPreparation(typedNil) != nil {
		t.Fatal("typed-nil preparation role should not produce a bound adapter")
	}
}
