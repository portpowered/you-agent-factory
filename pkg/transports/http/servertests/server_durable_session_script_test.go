package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

// apiExecutionLifecycleRoute is the exact lifecycle role normally supplied by
// the Factory Sessions gateway. Protocol tests route it to their programmed
// public ExecutionService without constructing a product implementation.
type apiExecutionLifecycleRoute struct {
	execution factorysessions.ExecutionService
}

// apiRequestPreparation is a strict public-role transport fake. Durable API
// protocol scenarios author already-prepared values and do not invoke the
// Factory Sessions implementation from outside its owner.
type apiRequestPreparation struct {
	start             func(factorysessions.StartRequest) (factorysessions.StartRequest, error)
	control           func(factorysessions.ControlRequest) (factorysessions.ControlRequest, error)
	approve           func(factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error)
	retryDispatch     func(factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error)
	interruptDispatch func(factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error)
	listSessions      func(factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error)
	result            func(factorysessions.ResultRequest) (factorysessions.ResultRequest, error)
	eventReconnect    func(factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error)
}

func canonicalAPIRequestPreparation() apiRequestPreparation {
	return apiRequestPreparation{
		start: func(request factorysessions.StartRequest) (factorysessions.StartRequest, error) {
			request.RequestID = strings.TrimSpace(request.RequestID)
			if request.RequestID == "" {
				return factorysessions.StartRequest{}, &factorysessions.ExecutionValidationError{Field: "requestId", Message: "requestId is required"}
			}
			return request, nil
		},
		control: func(request factorysessions.ControlRequest) (factorysessions.ControlRequest, error) {
			return request, nil
		},
		approve: func(request factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error) {
			return request, nil
		},
		retryDispatch: func(request factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error) {
			request.DispatchID = strings.TrimSpace(request.DispatchID)
			if request.DispatchID == "" {
				return factorysessions.RetryDispatchRequest{}, &factorysessions.ExecutionValidationError{Field: "dispatchId", Message: "dispatchId is required"}
			}
			return request, nil
		},
		interruptDispatch: func(request factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error) {
			request.DispatchID = strings.TrimSpace(request.DispatchID)
			if request.DispatchID == "" {
				return factorysessions.InterruptDispatchRequest{}, &factorysessions.ExecutionValidationError{Field: "dispatchId", Message: "dispatchId is required"}
			}
			return request, nil
		},
		listSessions: func(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
			if request.Scope == "" {
				request.Scope = factorysessions.SessionListScopeLive
			}
			return request, nil
		},
		result: func(request factorysessions.ResultRequest) (factorysessions.ResultRequest, error) {
			if request.Mode == "" {
				request.Mode = factorysessions.ResultModeFinal
			}
			return request, nil
		},
		eventReconnect: func(request factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error) {
			request.AfterEventID = strings.TrimSpace(request.AfterEventID)
			return request, nil
		},
	}
}

func (fake apiRequestPreparation) PrepareStart(request factorysessions.StartRequest) (factorysessions.StartRequest, error) {
	if fake.start == nil {
		panic("unexpected PrepareStart")
	}
	return fake.start(request)
}
func (fake apiRequestPreparation) PrepareControl(request factorysessions.ControlRequest) (factorysessions.ControlRequest, error) {
	if fake.control == nil {
		panic("unexpected PrepareControl")
	}
	return fake.control(request)
}
func (fake apiRequestPreparation) PrepareApprove(request factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error) {
	if fake.approve == nil {
		panic("unexpected PrepareApprove")
	}
	return fake.approve(request)
}
func (fake apiRequestPreparation) PrepareRetryDispatch(request factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error) {
	if fake.retryDispatch == nil {
		panic("unexpected PrepareRetryDispatch")
	}
	return fake.retryDispatch(request)
}
func (fake apiRequestPreparation) PrepareInterruptDispatch(request factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error) {
	if fake.interruptDispatch == nil {
		panic("unexpected PrepareInterruptDispatch")
	}
	return fake.interruptDispatch(request)
}
func (fake apiRequestPreparation) PrepareListSessions(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
	if fake.listSessions == nil {
		panic("unexpected PrepareListSessions")
	}
	return fake.listSessions(request)
}
func (fake apiRequestPreparation) PrepareResult(request factorysessions.ResultRequest) (factorysessions.ResultRequest, error) {
	if fake.result == nil {
		panic("unexpected PrepareResult")
	}
	return fake.result(request)
}
func (fake apiRequestPreparation) PrepareEventReconnect(request factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error) {
	if fake.eventReconnect == nil {
		panic("unexpected PrepareEventReconnect")
	}
	return fake.eventReconnect(request)
}

type apiLiveSessionScript struct {
	apisurface.LiveSessionAPI
	list   func(context.Context) (factoryapi.ListFactorySessionsResponse, error)
	get    func(context.Context, string) (factoryapi.FactorySession, error)
	pause  func(context.Context, string, factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	resume func(context.Context, string, factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
}

func (script apiLiveSessionScript) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	return script.list(ctx)
}

func (script apiLiveSessionScript) ListScopedLiveSessions(ctx context.Context) ([]factorysessions.ScopedLiveSessionSummary, error) {
	response, err := script.ListFactorySessions(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]factorysessions.ScopedLiveSessionSummary, 0, len(response.Sessions))
	for _, session := range response.Sessions {
		target := factorysessions.TargetRef{Kind: factorysessions.TargetKind(session.Target.Kind)}
		if session.Target.Name != nil {
			target.Name = strings.TrimSpace(*session.Target.Name)
		}
		rows = append(rows, factorysessions.ScopedLiveSessionSummary{
			ID: session.Id, FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
			Project: session.Project, IsDefault: session.IsDefault, Target: target,
		})
	}
	return rows, nil
}

func (script apiLiveSessionScript) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	return script.get(ctx, sessionID)
}

func (script apiLiveSessionScript) PauseLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return script.pause(ctx, sessionID, request)
}

func (script apiLiveSessionScript) ResumeLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return script.resume(ctx, sessionID, request)
}

func (route apiExecutionLifecycleRoute) PauseDurableFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return route.execution.Pause(ctx, sessionID, request)
}

func (route apiExecutionLifecycleRoute) ResumeDurableFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return route.execution.Resume(ctx, sessionID, request)
}

func (route apiExecutionLifecycleRoute) CancelDurableFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return route.execution.Cancel(ctx, sessionID, request)
}

func (route apiExecutionLifecycleRoute) TerminateDurableFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return route.execution.Terminate(ctx, sessionID, request)
}

func (route apiExecutionLifecycleRoute) ApproveDurableFactorySession(ctx context.Context, sessionID string, request factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return route.execution.Approve(ctx, sessionID, request)
}

func (route apiExecutionLifecycleRoute) RetryDurableFactorySessionDispatch(ctx context.Context, sessionID string, request factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return route.execution.RetryDispatch(ctx, sessionID, request)
}

func (route apiExecutionLifecycleRoute) InterruptDurableFactorySessionDispatch(ctx context.Context, sessionID string, request factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return route.execution.InterruptDispatch(ctx, sessionID, request)
}

func (route apiExecutionLifecycleRoute) ReadDurableFactorySessionEventStream(ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest) (*interfaces.FactoryEventStream, error) {
	result, err := route.execution.ReadEvents(ctx, sessionID, request)
	if err != nil {
		return nil, err
	}
	return factorysessions.MaterializeEventReadStream(result), nil
}

func (route apiExecutionLifecycleRoute) ProbeDurableFactorySessionEvents(ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest) error {
	_, err := route.execution.ReadEvents(ctx, sessionID, request)
	return err
}

func newDurableAPITestServer(execution factorysessions.ExecutionService) *api.Server {
	return newDurableAndLiveAPITestServer(execution, nil)
}

func newDurableAndLiveAPITestServer(execution factorysessions.ExecutionService, live apisurface.LiveSessionAPI) *api.Server {
	preparation := canonicalAPIRequestPreparation()
	var durable *factorysessionmapping.DurableAPI
	if execution != nil {
		durable = factorysessionmapping.NewDurableAPI(
			execution,
			apiExecutionLifecycleRoute{execution: execution},
		)
	}
	liveLister, _ := live.(factorysessionshttp.LiveSessionListReader)
	return newAPIServerFromRoles(
		nil, nil, live, nil, nil, nil, nil, nil, nil, nil,
		durable, durable, durable, durable, execution, liveLister, nil, nil,
		nil, nil, preparation, zap.NewNop(),
	)
}

func newWorkAPITestServer(work apisurface.WorkAPI) *api.Server {
	return newAPIServerFromRoles(
		nil, nil, nil, work, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop(),
	)
}

func durableRoleHTTPServer(t *testing.T, execution factorysessions.ExecutionService) string {
	t.Helper()
	server := httptest.NewServer(newDurableAPITestServer(execution).Handler())
	t.Cleanup(server.Close)
	return server.URL
}

// apiExecutionScript is a protocol-test double for the public Factory Sessions
// execution contract. Tests must install every callback they exercise; it has
// no storage, reduction, lifecycle, replay, or execution behavior of its own.
type apiExecutionScript struct {
	factorysessions.ExecutionService
	startAsync               func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	startSync                func(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error)
	resumeInterruptedSession func(context.Context, string, factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error)
	getSession               func(context.Context, string) (factorysessions.SessionReadResult, error)
	pause                    func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	resume                   func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	cancel                   func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	terminate                func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	approve                  func(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error)
	retryDispatch            func(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error)
	interruptDispatch        func(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error)
	getResult                func(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error)
	listDispatches           func(context.Context, string) (factorysessions.ListDispatchesResult, error)
	queryDispatches          func(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	getDispatch              func(context.Context, string, string) (factorysessions.DispatchDetail, error)
	listArtifacts            func(context.Context, string) (factorysessions.ListArtifactsResult, error)
	getArtifact              func(context.Context, string, string) (factorysessions.ArtifactDetail, error)
	readEvents               func(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error)
	listSessions             func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
}

func (script apiExecutionScript) StartAsync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	return script.startAsync(ctx, request)
}

func (script apiExecutionScript) StartSync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	return script.startSync(ctx, request)
}

func (script apiExecutionScript) ResumeInterruptedSession(ctx context.Context, sessionID string, request factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error) {
	return script.resumeInterruptedSession(ctx, sessionID, request)
}

func (script apiExecutionScript) GetSession(ctx context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	return script.getSession(ctx, sessionID)
}

func (script apiExecutionScript) Pause(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return script.pause(ctx, sessionID, request)
}

func (script apiExecutionScript) Resume(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return script.resume(ctx, sessionID, request)
}

func (script apiExecutionScript) Cancel(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return script.cancel(ctx, sessionID, request)
}

func (script apiExecutionScript) Terminate(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return script.terminate(ctx, sessionID, request)
}

func (script apiExecutionScript) Approve(ctx context.Context, sessionID string, request factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return script.approve(ctx, sessionID, request)
}

func (script apiExecutionScript) RetryDispatch(ctx context.Context, sessionID string, request factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return script.retryDispatch(ctx, sessionID, request)
}

func (script apiExecutionScript) InterruptDispatch(ctx context.Context, sessionID string, request factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return script.interruptDispatch(ctx, sessionID, request)
}

func (script apiExecutionScript) GetResult(ctx context.Context, sessionID string, request factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	return script.getResult(ctx, sessionID, request)
}

func (script apiExecutionScript) ListDispatches(ctx context.Context, sessionID string) (factorysessions.ListDispatchesResult, error) {
	return script.listDispatches(ctx, sessionID)
}

func (script apiExecutionScript) QueryDispatches(ctx context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	if script.queryDispatches != nil {
		return script.queryDispatches(ctx, request)
	}
	return script.listDispatches(ctx, request.SessionID)
}

func (script apiExecutionScript) GetDispatch(ctx context.Context, sessionID, dispatchID string) (factorysessions.DispatchDetail, error) {
	return script.getDispatch(ctx, sessionID, dispatchID)
}

func (script apiExecutionScript) ListArtifacts(ctx context.Context, sessionID string) (factorysessions.ListArtifactsResult, error) {
	return script.listArtifacts(ctx, sessionID)
}

func (script apiExecutionScript) GetArtifact(ctx context.Context, sessionID, artifactID string) (factorysessions.ArtifactDetail, error) {
	return script.getArtifact(ctx, sessionID, artifactID)
}

func (script apiExecutionScript) ReadEvents(ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	return script.readEvents(ctx, sessionID, request)
}

func (script apiExecutionScript) ListSessions(ctx context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	return script.listSessions(ctx, request)
}

func terminalAPIReadResult(sessionID string) factorysessions.SessionReadResult {
	return factorysessions.SessionReadResult{
		SessionID: sessionID,
		Status:    factorysessions.LifecycleStatusSucceeded,
		ResolvedSource: factorysessions.ResolvedSource{
			SourceRef:  ".claude/workflows/simple-final.js",
			SourceHash: "sha256:source",
		},
		SourceHash: "sha256:source",
		Policy: factorysessions.PolicyProjection{
			EffectiveHash: "sha256:policy",
		},
		ResultSummary: &factorysessions.ResultSummary{ResultStatus: "FINAL"},
		Usage:         factorysessions.EmptySessionUsage(),
		Links:         apiInspectionLinks(sessionID),
	}
}

func runningAPIReadResult(sessionID string) factorysessions.SessionReadResult {
	return factorysessions.SessionReadResult{
		SessionID: sessionID,
		Status:    factorysessions.LifecycleStatusRunning,
		Progress:  &factorysessions.ProgressCounts{},
		ResultSummary: &factorysessions.ResultSummary{
			ResultStatus: "PARTIAL",
		},
		Usage: factorysessions.EmptySessionUsage(),
		Links: apiInspectionLinks(sessionID),
	}
}

func finalAPIResult(sessionID string) factorysessions.ResultReadResult {
	return factorysessions.ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  factorysessions.ResultStatus("FINAL"),
		SessionStatus: factorysessions.LifecycleStatusSucceeded,
		Mode:          factorysessions.ResultModeFinal,
		PrimaryResult: json.RawMessage(`[{"type":"text","text":"complete"}]`),
	}
}

func notReadyAPIResult(sessionID string, status factorysessions.LifecycleStatus) factorysessions.ResultReadResult {
	return factorysessions.ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  factorysessions.ResultStatusNotReady,
		SessionStatus: status,
		Mode:          factorysessions.ResultModeFinal,
		Availability: &factorysessions.ResultAvailabilityDetail{
			Reason:    "RESULT_NOT_READY",
			Message:   "Session is still running.",
			Retryable: true,
		},
	}
}

func apiInspectionLinks(sessionID string) factorysessions.InspectionLinks {
	base := "/factory-sessions/" + sessionID
	return factorysessions.InspectionLinks{
		Session:    base,
		Status:     base,
		Events:     base + "/events",
		Results:    base + "/results",
		Dispatches: base + "/dispatches",
		Artifacts:  base + "/artifacts",
	}
}

func apiSyncStartCallback(
	sessionID string,
) func(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	return func(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
		return factorysessions.SyncStartResult{
			AsyncStartResult: factorysessions.AsyncStartResult{
				SessionID: sessionID,
				Status:    string(factorysessions.LifecycleStatusSucceeded),
			},
			SyncOutcome: factorysessions.SyncOutcome("COMPLETED"),
		}, nil
	}
}

func apiTerminalEvents(sessionID string) factorysessions.EventReadResult {
	return factorysessions.EventReadResult{
		SessionID: sessionID,
		Events: []json.RawMessage{
			apiCanonicalEvent("session-started/"+sessionID, "SESSION_STARTED", sessionID, 1),
			apiCanonicalEvent("session-result-updated/"+sessionID, "SESSION_RESULT_UPDATED", sessionID, 2),
			apiCanonicalEvent("session-completed/"+sessionID, "SESSION_COMPLETED", sessionID, 3),
		},
	}
}

func apiTerminalEventsAfter(
	sessionID string,
	request factorysessions.EventReconnectRequest,
) factorysessions.EventReadResult {
	result := apiTerminalEvents(sessionID)
	if request.AfterEventID != "" {
		for index, event := range result.Events {
			var envelope struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(event, &envelope) == nil && envelope.ID == request.AfterEventID {
				result.Events = result.Events[index+1:]
				return result
			}
		}
	}
	if request.AfterSequence != nil {
		sequence := *request.AfterSequence
		if sequence >= len(result.Events) {
			result.Events = nil
		} else {
			result.Events = result.Events[sequence:]
		}
	}
	return result
}

func apiCanonicalEvent(
	id string,
	eventType string,
	sessionID string,
	sequence int,
) json.RawMessage {
	encoded, _ := json.Marshal(map[string]any{
		"schemaVersion": "agent-factory.event.v1",
		"id":            id,
		"type":          eventType,
		"context": map[string]any{
			"sequence":  sequence,
			"tick":      sequence,
			"eventTime": "2026-01-01T00:00:00Z",
			"sessionId": sessionID,
		},
		"payload": map[string]any{},
	})
	return encoded
}

func apiControlResult(
	sessionID string,
	operation factorysessions.LifecycleControlKind,
	outcome factorysessions.LifecycleControlOutcome,
	status factorysessions.LifecycleStatus,
) factorysessions.LifecycleControlResult {
	return factorysessions.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   outcome,
		Status:    status,
		Links: factorysessions.LifecycleControlLinks{
			Session:    apiInspectionLinks(sessionID).Session,
			Status:     apiInspectionLinks(sessionID).Status,
			Results:    apiInspectionLinks(sessionID).Results,
			Dispatches: apiInspectionLinks(sessionID).Dispatches,
			Artifacts:  apiInspectionLinks(sessionID).Artifacts,
			Events:     apiInspectionLinks(sessionID).Events,
		},
	}
}

func apiControlError(
	operation factorysessions.LifecycleControlKind,
	outcome factorysessions.LifecycleControlOutcome,
	status factorysessions.LifecycleStatus,
) error {
	return &factorysessions.ControlError{
		Operation: operation,
		Outcome:   outcome,
		Status:    status,
		Message:   string(outcome),
	}
}
