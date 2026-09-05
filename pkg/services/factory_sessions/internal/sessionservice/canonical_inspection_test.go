package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
)

type canonicalInspectionLiveRuntimeFake struct {
	liveruntime.Service
	mu            sync.Mutex
	listResult    []factorysessions.ReadProjection
	getResult     factorysessions.SessionProjection
	resolved      map[string]*livesession.LiveSession
	controlResult factorysessions.LifecycleControlResult
	listCalls     int
	getCalls      int
	controlCalls  int
	closeCalls    int
	lastSessionID string
	lastOperation factorysessions.LifecycleControlKind
	lastControl   factorysessions.ControlRequest
	controlError  error
	closeError    error
}

func (fake *canonicalInspectionLiveRuntimeFake) List(context.Context) ([]factorysessions.ReadProjection, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.listCalls++
	return append([]factorysessions.ReadProjection(nil), fake.listResult...), nil
}

func (fake *canonicalInspectionLiveRuntimeFake) Get(context.Context, string) (factorysessions.SessionProjection, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.getCalls++
	return fake.getResult, nil
}

func (fake *canonicalInspectionLiveRuntimeFake) Resolve(sessionID string) *livesession.LiveSession {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.resolved[sessionID]
}

func (fake *canonicalInspectionLiveRuntimeFake) ApplyControl(
	_ context.Context,
	sessionID string,
	operation factorysessions.LifecycleControlKind,
	control factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.controlCalls++
	fake.lastSessionID = sessionID
	fake.lastOperation = operation
	fake.lastControl = control
	if fake.controlError != nil {
		return factorysessions.LifecycleControlResult{}, fake.controlError
	}
	return fake.controlResult, nil
}

func (fake *canonicalInspectionLiveRuntimeFake) Close(_ context.Context, sessionID string) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.closeCalls++
	fake.lastSessionID = sessionID
	return fake.closeError
}

type canonicalInspectionDurableFake struct {
	durableexecution.Service
	mu            sync.Mutex
	getResult     factorysessions.SessionReadResult
	listResult    factorysessions.ListSessionsResult
	result        factorysessions.ResultReadResult
	dispatches    factorysessions.ListDispatchesResult
	cursor        *factorysessions.ResponseEventCursor
	controlResult durableexecution.CanonicalControlResult
	controlError  error
	getCalls      int
	listCalls     int
	resultCalls   int
	dispatchCalls int
	responseCalls int
	controlCalls  int
	lastList      factorysessions.ListSessionsRequest
	lastResultReq factorysessions.ResultRequest
	lastDispatch  factorysessions.DispatchQueryRequest
	lastResponse  factorysessions.ResponseEventSubscriptionRequest
	lastControl   factorysessions.SessionControlRequest
	legacyCalls   int
}

func (fake *canonicalInspectionDurableFake) GetCanonical(context.Context, string) (factorysessions.SessionReadResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.getCalls++
	return fake.getResult, nil
}

func (fake *canonicalInspectionDurableFake) ListCanonical(
	_ context.Context,
	request factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.listCalls++
	fake.lastList = request
	return fake.listResult, nil
}

func (fake *canonicalInspectionDurableFake) ControlCanonical(
	_ context.Context,
	request factorysessions.SessionControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.controlCalls++
	fake.lastControl = request
	if fake.controlError != nil {
		return durableexecution.CanonicalControlResult{}, fake.controlError
	}
	return fake.controlResult, nil
}

func (fake *canonicalInspectionDurableFake) ReadResultCanonical(
	_ context.Context,
	_ string,
	request factorysessions.ResultRequest,
) (factorysessions.ResultReadResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.resultCalls++
	fake.lastResultReq = request
	return fake.result, nil
}

func (fake *canonicalInspectionDurableFake) QueryDispatchesCanonical(
	_ context.Context,
	request factorysessions.DispatchQueryRequest,
) (factorysessions.ListDispatchesResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.dispatchCalls++
	fake.lastDispatch = request
	return fake.dispatches, nil
}

func (fake *canonicalInspectionDurableFake) SubscribeResponsesCanonical(
	_ context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.responseCalls++
	fake.lastResponse = request
	return fake.cursor, nil
}

func (fake *canonicalInspectionDurableFake) GetSession(context.Context, string) (factorysessions.SessionReadResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.legacyCalls++
	return factorysessions.SessionReadResult{}, errors.New("legacy GetSession selected")
}

func (fake *canonicalInspectionDurableFake) ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.legacyCalls++
	return factorysessions.ListSessionsResult{}, errors.New("legacy ListSessions selected")
}

func (fake *canonicalInspectionDurableFake) GetResult(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.legacyCalls++
	return factorysessions.ResultReadResult{}, errors.New("legacy GetResult selected")
}

func (fake *canonicalInspectionDurableFake) QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.legacyCalls++
	return factorysessions.ListDispatchesResult{}, errors.New("legacy QueryDispatches selected")
}

type canonicalInspectionResponseStreamFake struct {
	responsestreamservice.Service
	mu      sync.Mutex
	calls   int
	request responsestreamservice.SubscriptionRequest
	cursor  *factorysessions.ResponseEventCursor
	err     error
}

type canonicalInspectionCheckpointStore struct {
	records []factorydefinitions.JavaScriptCheckpointRecord
}

func (store *canonicalInspectionCheckpointStore) Put(record factorydefinitions.JavaScriptCheckpointRecord) {
	store.records = append(store.records, record)
}

func (store *canonicalInspectionCheckpointStore) List() []factorydefinitions.JavaScriptCheckpointRecord {
	return append([]factorydefinitions.JavaScriptCheckpointRecord(nil), store.records...)
}

func (store *canonicalInspectionCheckpointStore) Get(id string) (factorydefinitions.JavaScriptCheckpointRecord, bool) {
	for _, record := range store.records {
		if record.ID == id {
			return record, true
		}
	}
	return factorydefinitions.JavaScriptCheckpointRecord{}, false
}

type canonicalInspectionResultProjectionFake struct {
	result factoryruntime.SessionResultProjection
}

func (fake *canonicalInspectionResultProjectionFake) ProjectSessionResults(factoryruntime.SessionResultInput) factoryruntime.SessionResultProjection {
	return fake.result
}

type canonicalInspectionResultHost struct {
	Host
	session *livesession.LiveSession
	context factorysessions.ProjectionContext
	store   factoryruntime.JavaScriptCheckpointStore
}

func (host *canonicalInspectionResultHost) RequireSession(sessionID string) (*livesession.LiveSession, error) {
	if host.session == nil || host.session.ID != sessionID {
		return nil, factorysessions.ErrSessionNotFound
	}
	return host.session, nil
}

func (host *canonicalInspectionResultHost) BuildSessionProjectionContext(context.Context, *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
	return host.context, nil
}

func (host *canonicalInspectionResultHost) JavaScriptCheckpointStore(*livesession.LiveSession) factoryruntime.JavaScriptCheckpointStore {
	return host.store
}

var _ controlplane.ResultReadHost = (*canonicalInspectionResultHost)(nil)

func (fake *canonicalInspectionResponseStreamFake) Subscribe(
	_ context.Context,
	_ *responseeventstore.SessionResponseEventStore,
	request responsestreamservice.SubscriptionRequest,
) (*responsestreamservice.Cursor, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.calls++
	fake.request = request
	if fake.err != nil {
		return nil, fake.err
	}
	return fake.cursor, nil
}

func assertCanonicalFieldError(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want field-scoped validation error for %s", field)
	}
	var detached *factorysessions.DetachedRequestError
	if errors.As(err, &detached) {
		if detached.Field != field {
			t.Fatalf("detached validation field = %q, want %q", detached.Field, field)
		}
		return
	}
	var validation *factorysessions.ValidationError
	if errors.As(err, &validation) {
		if validation.Field != field {
			t.Fatalf("validation field = %q, want %q", validation.Field, field)
		}
		return
	}
	t.Fatalf("error = %T %v, want field-scoped validation error for %s", err, err, field)
}

// TestService_CanonicalReadsUseModeOwnersAndRuntimeFreeViews proves the root
// maps live and durable owner projections without selecting compatibility
// methods or exposing runtime implementation state.
func TestService_CanonicalReadsUseModeOwnersAndRuntimeFreeViews(t *testing.T) {
	t.Parallel()

	live := &canonicalInspectionLiveRuntimeFake{
		getResult: factorysessions.SessionProjection{
			Context: factorysessions.ProjectionContext{
				FactorySessionID: "live-1",
				Session: &factorysessions.ScopedLiveSessionSummary{
					ID:         "live-1",
					FactoryDir: "/factory/live",
					FolderPath: "/factory",
					Project:    "live-project",
					IsDefault:  true,
					Target:     factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "live"},
					Runtime:    &factorysessions.RuntimeProjection{Status: "RUNNING"},
				},
			},
			Runtime: factorysessions.RuntimeProjection{Status: "RUNNING", OrchestratorKind: "PETRI"},
		},
		listResult: []factorysessions.ReadProjection{{
			Context: factorysessions.ProjectionContext{
				FactorySessionID: "live-2",
				Session:          &factorysessions.ScopedLiveSessionSummary{ID: "live-2", Project: "second"},
			},
			Runtime:          factorysessions.RuntimeProjection{Status: "PAUSED"},
			RuntimeAvailable: true,
		}},
	}
	durable := &canonicalInspectionDurableFake{
		getResult: factorysessions.SessionReadResult{
			SessionID:        "durable-1",
			Status:           factorysessions.LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			ResolvedSource:   factorysessions.ResolvedSource{SourceRef: "factory.js"},
			SourceHash:       "hash-1",
			ResultSummary:    &factorysessions.ResultSummary{ResultStatus: "FINAL"},
		},
		listResult: factorysessions.ListSessionsResult{
			DurableSessions: []factorysessions.DurableSessionListSummary{{
				SessionID:        "durable-2",
				Status:           factorysessions.LifecycleStatusInterrupted,
				OrchestratorKind: "PETRI",
				ResolvedSource:   factorysessions.ResolvedSource{SourceRef: "workflow.json"},
				SourceHash:       "hash-2",
				ResultSummary:    &factorysessions.ResultSummary{ResultStatus: "PARTIAL"},
			}},
		},
	}
	service := &Service{liveRuntime: live, durable: durable}

	gotLive, err := service.Get(context.Background(), factorysessions.SessionGetRequest{
		SessionID: " live-1 ", Mode: factorysessions.SessionOperationModeLive,
	})
	if err != nil {
		t.Fatalf("canonical live Get: %v", err)
	}
	if gotLive.Session.SessionID != "live-1" || gotLive.Session.Status != "RUNNING" || !gotLive.Session.RuntimeAvailable || !gotLive.Session.IsDefault {
		t.Fatalf("live view = %#v, want stable runtime-free projection", gotLive.Session)
	}
	if gotLive.Session.FactoryDir != "/factory/live" || gotLive.Session.Target.Name != "live" {
		t.Fatalf("live identity = %#v, want owner fields", gotLive.Session)
	}

	gotLiveList, err := service.List(context.Background(), factorysessions.SessionListRequest{
		Mode: factorysessions.SessionOperationModeLive,
	})
	if err != nil {
		t.Fatalf("canonical live List: %v", err)
	}
	if len(gotLiveList.Sessions) != 1 || gotLiveList.Sessions[0].SessionID != "live-2" || gotLiveList.Sessions[0].Status != "PAUSED" {
		t.Fatalf("live list = %#v, want projected live row", gotLiveList.Sessions)
	}

	gotDurable, err := service.Get(context.Background(), factorysessions.SessionGetRequest{
		SessionID: " durable-1 ", Mode: factorysessions.SessionOperationModeDurable,
	})
	if err != nil {
		t.Fatalf("canonical durable Get: %v", err)
	}
	if gotDurable.Session.SessionID != "durable-1" || gotDurable.Session.Status != "SUCCEEDED" || gotDurable.Session.ResultStatus != "FINAL" || gotDurable.Session.SourceRef != "factory.js" {
		t.Fatalf("durable view = %#v, want stable result/readiness fields", gotDurable.Session)
	}

	gotDurableList, err := service.List(context.Background(), factorysessions.SessionListRequest{
		Mode: factorysessions.SessionOperationModeDurable,
		Filters: factorysessions.SessionListFilters{
			SourceRef:         " factory ",
			OrchestratorKinds: []string{" JAVASCRIPT "},
		},
	})
	if err != nil {
		t.Fatalf("canonical durable List: %v", err)
	}
	if len(gotDurableList.Sessions) != 1 || gotDurableList.Sessions[0].SessionID != "durable-2" || gotDurableList.Sessions[0].Mode != factorysessions.SessionOperationModeDurable || gotDurableList.Sessions[0].ResultStatus != "PARTIAL" {
		t.Fatalf("durable list = %#v, want projected durable row", gotDurableList.Sessions)
	}
	durable.mu.Lock()
	durableListRequest := durable.lastList
	durable.mu.Unlock()

	gotAll, err := service.List(context.Background(), factorysessions.SessionListRequest{
		Mode: factorysessions.SessionOperationModeAll,
	})
	if err != nil {
		t.Fatalf("canonical all List: %v", err)
	}
	if len(gotAll.Sessions) != 2 || gotAll.Sessions[0].Mode != factorysessions.SessionOperationModeLive || gotAll.Sessions[1].Mode != factorysessions.SessionOperationModeDurable || gotAll.Sessions[0].SessionID != "live-2" || gotAll.Sessions[1].SessionID != "durable-2" {
		t.Fatalf("all list = %#v, want live rows before durable rows", gotAll.Sessions)
	}

	durable.mu.Lock()
	listCalls, getCalls, legacyCalls := durable.listCalls, durable.getCalls, durable.legacyCalls
	durable.mu.Unlock()
	if listCalls != 2 || getCalls != 1 || legacyCalls != 0 {
		t.Fatalf("durable owner calls = list:%d get:%d legacy:%d, want canonical-only reads", listCalls, getCalls, legacyCalls)
	}
	if durableListRequest.Scope != factorysessions.SessionListScopePersisted || durableListRequest.Filters.SourceRef != "factory" || len(durableListRequest.Filters.OrchestratorKinds) != 1 || durableListRequest.Filters.OrchestratorKinds[0] != "JAVASCRIPT" {
		t.Fatalf("durable list request = %#v, want normalized cloned filter values", durableListRequest)
	}

	live.mu.Lock()
	getCalls, listCalls = live.getCalls, live.listCalls
	live.mu.Unlock()
	if getCalls != 1 || listCalls != 2 {
		t.Fatalf("live owner calls = get:%d list:%d, want direct live reads for live/all", getCalls, listCalls)
	}
}

// TestService_CanonicalInspectionValidationPrecedesOwnerCalls proves invalid
// IDs, modes, operations, filters, and cursors fail before either owner runs.
func TestService_CanonicalInspectionValidationPrecedesOwnerCalls(t *testing.T) {
	t.Parallel()

	live := &canonicalInspectionLiveRuntimeFake{}
	durable := &canonicalInspectionDurableFake{}
	service := &Service{liveRuntime: live, durable: durable}
	cases := []struct {
		name  string
		field string
		call  func() error
	}{
		{
			name:  "get mode",
			field: "mode",
			call: func() error {
				_, err := service.Get(context.Background(), factorysessions.SessionGetRequest{
					SessionID: "session", Mode: factorysessions.SessionOperationMode("unknown"),
				})
				return err
			},
		},
		{
			name:  "list mode",
			field: "mode",
			call: func() error {
				_, err := service.List(context.Background(), factorysessions.SessionListRequest{
					Mode: factorysessions.SessionOperationMode("unknown"),
				})
				return err
			},
		},
		{
			name:  "control operation",
			field: "operation",
			call: func() error {
				_, err := service.Control(context.Background(), factorysessions.SessionControlRequest{
					SessionID: "session", Mode: factorysessions.SessionOperationModeLive,
					Operation: factorysessions.SessionControlOperation("unknown"),
				})
				return err
			},
		},
		{
			name:  "result mode",
			field: "mode",
			call: func() error {
				_, err := service.ReadResult(context.Background(), factorysessions.SessionResultReadRequest{
					SessionID: "session", Mode: factorysessions.SessionOperationModeDurable,
					Request: factorysessions.ResultRequest{Mode: factorysessions.ResultMode("unknown")},
				})
				return err
			},
		},
		{
			name:  "dispatch filter",
			field: "status",
			call: func() error {
				_, err := service.QueryDispatches(context.Background(), factorysessions.DispatchQueryRequest{
					SessionID: "session", Filters: factorysessions.DispatchFilters{Status: "unknown"},
				})
				return err
			},
		},
		{
			name:  "response cursor",
			field: "afterSequence",
			call: func() error {
				_, err := service.SubscribeResponses(context.Background(), factorysessions.SessionResponseSubscriptionRequest{
					SessionID: "session", AfterSequence: -1,
				})
				return err
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assertCanonicalFieldError(t, testCase.call(), testCase.field)
		})
	}

	live.mu.Lock()
	liveGetCalls, liveListCalls, liveControlCalls, liveCloseCalls := live.getCalls, live.listCalls, live.controlCalls, live.closeCalls
	live.mu.Unlock()
	durable.mu.Lock()
	durableGetCalls, durableListCalls := durable.getCalls, durable.listCalls
	durableControlCalls, durableResultCalls := durable.controlCalls, durable.resultCalls
	durableDispatchCalls, durableResponseCalls := durable.dispatchCalls, durable.responseCalls
	durable.mu.Unlock()
	if liveGetCalls != 0 || liveListCalls != 0 || liveControlCalls != 0 || liveCloseCalls != 0 || durableGetCalls != 0 || durableListCalls != 0 || durableControlCalls != 0 || durableResultCalls != 0 || durableDispatchCalls != 0 || durableResponseCalls != 0 {
		t.Fatalf("invalid input owner calls = live(get:%d list:%d control:%d close:%d) durable(get:%d list:%d control:%d result:%d dispatch:%d response:%d), want all zero", liveGetCalls, liveListCalls, liveControlCalls, liveCloseCalls, durableGetCalls, durableListCalls, durableControlCalls, durableResultCalls, durableDispatchCalls, durableResponseCalls)
	}
}

// TestService_CanonicalControlsPreserveTypedOutcomesAndPayloads proves every
// admitted durable control is sent to the canonical owner and mapped back to
// the shared typed result vocabulary.
func TestService_CanonicalControlsPreserveTypedOutcomesAndPayloads(t *testing.T) {
	t.Parallel()

	stale := errors.New("stale terminal association")
	durable := &canonicalInspectionDurableFake{}
	service := &Service{durable: durable}
	operations := []factorysessions.SessionControlOperation{
		factorysessions.SessionControlPause,
		factorysessions.SessionControlResume,
		factorysessions.SessionControlCancel,
		factorysessions.SessionControlTerminate,
		factorysessions.SessionControlRecover,
		factorysessions.SessionControlApprove,
		factorysessions.SessionControlRetryDispatch,
		factorysessions.SessionControlInterruptDispatch,
	}
	for _, operation := range operations {
		durable.mu.Lock()
		durable.controlResult = durableexecution.CanonicalControlResult{}
		if operation == factorysessions.SessionControlRecover {
			durable.controlResult.Recovery = &factorysessions.AsyncStartResult{SessionID: "recovered", Status: "QUEUED"}
		} else {
			durable.controlResult.Lifecycle = &factorysessions.LifecycleControlResult{
				Outcome:           factorysessions.LifecycleControlOutcomeAccepted,
				Status:            factorysessions.LifecycleStatusRunning,
				Detail:            "owner detail",
				ApprovalPreviewID: "preview-1",
				DispatchID:        "dispatch-1",
				RetryDispatchID:   "retry-1",
			}
		}
		durable.mu.Unlock()
		request := factorysessions.SessionControlRequest{
			SessionID:   " durable-1 ",
			Mode:        factorysessions.SessionOperationModeDurable,
			Operation:   operation,
			Correlation: factorysessions.SessionOperationCorrelation{RequestID: " correlation-1 ", TurnID: " turn-1 "},
		}
		if operation == factorysessions.SessionControlApprove {
			request.Approve = &factorysessions.ApproveRequest{ApprovedPolicy: map[string]any{
				"nested": map[string]any{"value": "original"},
			}}
		}
		got, err := service.Control(context.Background(), request)
		if err != nil {
			t.Fatalf("canonical Control(%s): %v", operation, err)
		}
		if got.SessionID != "durable-1" || got.Mode != factorysessions.SessionOperationModeDurable || got.Operation != operation {
			t.Fatalf("Control(%s) identity = %#v, want normalized owner identity", operation, got)
		}
		if operation == factorysessions.SessionControlRecover {
			if got.Recovery == nil || got.Recovery.SessionID != "recovered" || got.Status != factorysessions.LifecycleStatus("QUEUED") {
				t.Fatalf("Control(%s) recovery = %#v, want typed recovery result", operation, got)
			}
		} else if got.Outcome != factorysessions.LifecycleControlOutcomeAccepted || got.Status != factorysessions.LifecycleStatusRunning || got.DispatchID != "dispatch-1" {
			t.Fatalf("Control(%s) result = %#v, want characterized lifecycle outcome", operation, got)
		}
	}

	request := factorysessions.SessionControlRequest{
		SessionID: "durable-approve",
		Mode:      factorysessions.SessionOperationModeDurable,
		Operation: factorysessions.SessionControlApprove,
		Approve: &factorysessions.ApproveRequest{ApprovedPolicy: map[string]any{
			"nested": map[string]any{"value": "caller"},
		}},
	}
	durable.mu.Lock()
	durable.controlResult = durableexecution.CanonicalControlResult{Lifecycle: &factorysessions.LifecycleControlResult{Outcome: factorysessions.LifecycleControlOutcomeAccepted}}
	durable.mu.Unlock()
	if _, err := service.Control(context.Background(), request); err != nil {
		t.Fatalf("canonical Control(approve payload): %v", err)
	}
	request.Approve.ApprovedPolicy["nested"].(map[string]any)["value"] = "caller mutation"
	durable.mu.Lock()
	ownerRequest := durable.lastControl
	durable.mu.Unlock()
	if ownerRequest.Approve == nil || ownerRequest.Approve.ApprovedPolicy["nested"].(map[string]any)["value"] != "caller" {
		t.Fatalf("control payload = %#v, want caller-owned policy cloned", ownerRequest.Approve)
	}

	durable.mu.Lock()
	durable.controlError = stale
	durable.mu.Unlock()
	if _, err := service.Control(context.Background(), factorysessions.SessionControlRequest{
		SessionID: "durable-stale", Mode: factorysessions.SessionOperationModeDurable,
		Operation: factorysessions.SessionControlRetryDispatch,
	}); !errors.Is(err, stale) {
		t.Fatalf("stale control error = %v, want owner error %v", err, stale)
	}

	durable.mu.Lock()
	controlCalls, legacyCalls := durable.controlCalls, durable.legacyCalls
	durable.mu.Unlock()
	if controlCalls != len(operations)+1+1 || legacyCalls != 0 {
		t.Fatalf("control calls = canonical:%d legacy:%d, want canonical-only owner dispatch", controlCalls, legacyCalls)
	}
}

// TestService_CanonicalResultDispatchAndResponseReadsValidateAndClone proves
// terminal/partial result, filtered dispatch, and durable response reads keep
// typed values, normalized cursors, and caller isolation at the root boundary.
func TestService_CanonicalResultDispatchAndResponseReadsValidateAndClone(t *testing.T) {
	t.Parallel()

	durable := &canonicalInspectionDurableFake{
		result: factorysessions.ResultReadResult{
			SessionID:        "durable-result",
			ResultStatus:     factorysessions.ResultStatusFailedWithPartial,
			SessionStatus:    factorysessions.LifecycleStatusFailed,
			Mode:             factorysessions.ResultModePartial,
			IncludeArtifacts: true,
			PrimaryResult:    []byte("partial-result"),
			ArtifactIDs:      []string{"artifact-1"},
			Failure:          &factorysessions.FailureSummary{Reason: "worker", PartialResultAvailable: true},
			Availability:     &factorysessions.ResultAvailabilityDetail{Reason: "partial", Retryable: true},
		},
		dispatches: factorysessions.ListDispatchesResult{
			SessionID: "durable-dispatch",
			Dispatches: []factorysessions.DispatchSummary{{
				ID: "dispatch-1", Status: factorysessions.DispatchStatus("COMPLETED"),
				ProviderSessionRefs: []factorysessions.ProviderSessionRef{{Provider: "provider", ID: "provider-session"}},
				OutputArtifactIDs:   []string{"artifact-1"},
				Usage:               &factorysessions.DispatchUsage{TotalTokens: 5},
				Warnings:            []factorysessions.DispatchWarning{{Code: "warning"}},
			}},
		},
		cursor: &factorysessions.ResponseEventCursor{},
	}
	service := &Service{durable: durable}

	result, err := service.ReadResult(context.Background(), factorysessions.SessionResultReadRequest{
		SessionID: " durable-result ", Mode: factorysessions.SessionOperationModeDurable,
		Request: factorysessions.ResultRequest{IncludeArtifacts: true},
	})
	if err != nil {
		t.Fatalf("canonical ReadResult: %v", err)
	}
	if result.SessionID != "durable-result" || result.Status != string(factorysessions.ResultStatusFailedWithPartial) || result.Durable == nil || result.Durable.Mode != factorysessions.ResultModePartial || result.Durable.Failure == nil || !result.Durable.Failure.PartialResultAvailable {
		t.Fatalf("result = %#v, want typed failed-with-partial projection", result)
	}
	durable.mu.Lock()
	durable.result.PrimaryResult[0] = 'X'
	durable.mu.Unlock()
	if string(result.Durable.PrimaryResult) != "partial-result" {
		t.Fatal("canonical ReadResult returned owner-owned primary result bytes")
	}

	dispatches, err := service.QueryDispatches(context.Background(), factorysessions.DispatchQueryRequest{
		SessionID: " durable-dispatch ", Filters: factorysessions.DispatchFilters{Status: " completed "},
	})
	if err != nil {
		t.Fatalf("canonical QueryDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].ID != "dispatch-1" || dispatches.Dispatches[0].Usage == nil || dispatches.Dispatches[0].Usage.TotalTokens != 5 {
		t.Fatalf("dispatches = %#v, want cloned dispatch summary", dispatches)
	}
	durable.mu.Lock()
	durable.dispatches.Dispatches[0].Warnings[0].Code = "owner mutation"
	lastDispatch := durable.lastDispatch
	durable.mu.Unlock()
	if dispatches.Dispatches[0].Warnings[0].Code != "warning" || lastDispatch.Filters.Status != factorysessions.DispatchStatus("COMPLETED") {
		t.Fatalf("dispatch projection/request = %#v / %#v, want cloned result and normalized filter", dispatches.Dispatches, lastDispatch)
	}

	subscription, err := service.SubscribeResponses(context.Background(), factorysessions.SessionResponseSubscriptionRequest{
		SessionID: " durable-dispatch ", AfterSequence: 4, DispatchID: " dispatch-1 ", Kinds: []factorysessions.ResponseEventKind{factorysessions.ResponseEventKindMessage},
	})
	if err != nil {
		t.Fatalf("canonical durable SubscribeResponses: %v", err)
	}
	if subscription.Cursor != durable.cursor {
		t.Fatal("canonical durable SubscribeResponses did not return owner cursor")
	}
	durable.mu.Lock()
	responseCalls, responseRequest := durable.responseCalls, durable.lastResponse
	durable.mu.Unlock()
	if responseCalls != 1 || responseRequest.SessionID != "durable-dispatch" || responseRequest.AfterSequence != 4 || responseRequest.DispatchID != "dispatch-1" {
		t.Fatalf("response request = calls:%d request:%#v, want normalized cursor/filter", responseCalls, responseRequest)
	}

	durable.mu.Lock()
	resultCalls, dispatchCalls, legacyCalls := durable.resultCalls, durable.dispatchCalls, durable.legacyCalls
	durable.mu.Unlock()
	if resultCalls != 1 || dispatchCalls != 1 || legacyCalls != 0 {
		t.Fatalf("inspection calls = result:%d dispatch:%d legacy:%d, want canonical-only reads", resultCalls, dispatchCalls, legacyCalls)
	}

	invalid := &canonicalInspectionDurableFake{}
	invalidService := &Service{durable: invalid}
	if _, err := invalidService.QueryDispatches(context.Background(), factorysessions.DispatchQueryRequest{
		SessionID: "session", Filters: factorysessions.DispatchFilters{Status: "invalid"},
	}); err == nil {
		t.Fatal("invalid dispatch status returned nil error")
	}
	if _, err := invalidService.SubscribeResponses(context.Background(), factorysessions.SessionResponseSubscriptionRequest{
		SessionID: "session", Kinds: []factorysessions.ResponseEventKind{"invalid"},
	}); !errors.Is(err, factorysessions.ErrInvalidResponseEventFilter) {
		t.Fatalf("invalid response kind error = %v, want ErrInvalidResponseEventFilter", err)
	}
	if _, err := invalidService.SubscribeResponses(context.Background(), factorysessions.SessionResponseSubscriptionRequest{
		SessionID: "session", AfterSequence: -1,
	}); err == nil {
		t.Fatal("negative response cursor returned nil error")
	}
	invalid.mu.Lock()
	if invalid.dispatchCalls != 0 || invalid.responseCalls != 0 {
		t.Fatalf("invalid input owner calls = dispatch:%d response:%d, want zero", invalid.dispatchCalls, invalid.responseCalls)
	}
	invalid.mu.Unlock()
}

// TestService_CanonicalLiveResultReadsMapCompleteAndPartialProjections proves
// live result reads remain on the control-plane result owner and return cloned
// checkpoint/artifact references without leaking runtime state.
func TestService_CanonicalLiveResultReadsMapCompleteAndPartialProjections(t *testing.T) {
	t.Parallel()

	const sessionID = "live-result"
	checkpoint := &canonicalInspectionCheckpointStore{records: []factorydefinitions.JavaScriptCheckpointRecord{{
		ID: "checkpoint-1", Label: "checkpoint", Summary: "partial", ArtifactID: "artifact-1",
		ContentHash: "hash-1", SizeBytes: 12,
	}}}
	resultArtifactID := "result-artifact"
	projection := &canonicalInspectionResultProjectionFake{result: factoryruntime.SessionResultProjection{
		Live: factoryruntime.LiveSessionResult{
			SessionID:      sessionID,
			Status:         "SUCCEEDED",
			CheckpointRefs: []factorydefinitions.FactorySessionJavaScriptCheckpointEventRef{{ID: "result-checkpoint"}},
			ResultArtifactRef: &factorydefinitions.FactoryArtifactRef{
				ID: resultArtifactID,
			},
		},
	}}
	host := &canonicalInspectionResultHost{
		session: &livesession.LiveSession{ID: sessionID},
		context: factorysessions.ProjectionContext{
			FactorySessionID: sessionID,
			FactoryCfg: &factorydefinitions.FactoryConfig{
				Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{Kind: factorydefinitions.OrchestratorKindJavaScript},
			},
		},
		store: checkpoint,
	}
	service := &Service{host: host, results: projection}

	complete, err := service.ReadResult(context.Background(), factorysessions.SessionResultReadRequest{
		SessionID: sessionID, Mode: factorysessions.SessionOperationModeLive,
	})
	if err != nil {
		t.Fatalf("canonical live complete result: %v", err)
	}
	if complete.Status != "SUCCEEDED" || complete.Live == nil || complete.Live.Status != "SUCCEEDED" || complete.Live.ResultArtifactRef == nil || complete.Live.ResultArtifactRef.ID != resultArtifactID {
		t.Fatalf("complete result = %#v, want projected terminal result", complete)
	}

	partial, err := service.ReadResult(context.Background(), factorysessions.SessionResultReadRequest{
		SessionID: sessionID, Mode: factorysessions.SessionOperationModeLive,
		Request: factorysessions.ResultRequest{Mode: factorysessions.ResultModePartial},
	})
	if err != nil {
		t.Fatalf("canonical live partial result: %v", err)
	}
	if partial.Status != "PARTIAL" || partial.Live == nil || len(partial.Live.CheckpointRefs) != 1 || partial.Live.CheckpointRefs[0].ID != "checkpoint-1" || partial.Live.ResultArtifactRef == nil || partial.Live.ResultArtifactRef.ID != "artifact-1" {
		t.Fatalf("partial result = %#v, want checkpoint-backed partial projection", partial)
	}
	checkpoint.records[0].ID = "owner mutation"
	if partial.Live.CheckpointRefs[0].ID != "checkpoint-1" {
		t.Fatal("canonical live partial result returned owner-owned checkpoint data")
	}
}

// TestService_CanonicalLiveControlAndResponseRouting keeps live controls and
// live response cursors on the live owner even when a durable owner is bound.
func TestService_CanonicalLiveControlAndResponseRouting(t *testing.T) {
	t.Parallel()

	const sessionID = "live-control"
	liveSession := &livesession.LiveSession{
		ID:             sessionID,
		ResponseEvents: responseeventstore.NewSessionResponseEventStore(sessionID, platformclock.Real{}, func() string { return "event-1" }),
	}
	live := &canonicalInspectionLiveRuntimeFake{
		resolved: map[string]*livesession.LiveSession{sessionID: liveSession},
		controlResult: factorysessions.LifecycleControlResult{
			Outcome: factorysessions.LifecycleControlOutcomeAccepted,
			Status:  factorysessions.LifecycleStatusPaused,
		},
	}
	response := &canonicalInspectionResponseStreamFake{cursor: &factorysessions.ResponseEventCursor{}}
	service := &Service{liveRuntime: live, responseEvents: response}

	paused, err := service.Control(context.Background(), factorysessions.SessionControlRequest{
		SessionID: " live-control ", Mode: factorysessions.SessionOperationModeLive,
		Operation:   factorysessions.SessionControlPause,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "control-1", TurnID: "turn-1"},
	})
	if err != nil {
		t.Fatalf("canonical live pause: %v", err)
	}
	if paused.Outcome != factorysessions.LifecycleControlOutcomeAccepted || paused.Status != factorysessions.LifecycleStatusPaused || paused.Closed {
		t.Fatalf("live pause = %#v, want typed lifecycle result", paused)
	}
	live.mu.Lock()
	controlCalls, closeCalls, operation, control := live.controlCalls, live.closeCalls, live.lastOperation, live.lastControl
	live.mu.Unlock()
	if controlCalls != 1 || closeCalls != 0 || operation != factorysessions.LifecycleControlPause || control.RequestID != "control-1" || control.TurnID != "turn-1" {
		t.Fatalf("live pause owner call = control:%d close:%d operation:%q request:%#v, want direct pause owner", controlCalls, closeCalls, operation, control)
	}

	closed, err := service.Control(context.Background(), factorysessions.SessionControlRequest{
		SessionID: sessionID, Mode: factorysessions.SessionOperationModeLive,
		Operation: factorysessions.SessionControlCancel,
	})
	if err != nil {
		t.Fatalf("canonical live cancel: %v", err)
	}
	if !closed.Closed || closed.Operation != factorysessions.SessionControlCancel {
		t.Fatalf("live cancel = %#v, want closed compatibility characterization", closed)
	}
	live.mu.Lock()
	controlCalls, closeCalls = live.controlCalls, live.closeCalls
	live.mu.Unlock()
	if controlCalls != 1 || closeCalls != 1 {
		t.Fatalf("live cancel owner calls = control:%d close:%d, want close-only", controlCalls, closeCalls)
	}

	subscription, err := service.SubscribeResponses(context.Background(), factorysessions.SessionResponseSubscriptionRequest{
		SessionID: sessionID, AfterSequence: 0, Kinds: []factorysessions.ResponseEventKind{factorysessions.ResponseEventKindMessage},
	})
	if err != nil {
		t.Fatalf("canonical live SubscribeResponses: %v", err)
	}
	if subscription.Cursor != response.cursor {
		t.Fatal("canonical live SubscribeResponses did not return live owner cursor")
	}
	response.mu.Lock()
	responseCalls := response.calls
	response.mu.Unlock()
	if responseCalls != 1 {
		t.Fatalf("live response owner calls = %d, want one", responseCalls)
	}
}
