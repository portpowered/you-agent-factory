package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	canonicaldurable "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/canonical/durable"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestService_StreamMethods_RequireGateway(t *testing.T) {
	t.Parallel()

	var gateway *Service

	if _, err := gateway.SubscribeSessionResponseStream("sess", "dispatch-1", 0); err == nil {
		t.Fatal("SubscribeSessionResponseStream = nil, want gateway required")
	}
	if _, err := gateway.SessionResponseStreamDispatchIDs("sess"); err == nil {
		t.Fatal("SessionResponseStreamDispatchIDs = nil, want gateway required")
	}
	if gateway.InferenceProgressPublisherFactory(nil) != nil {
		t.Fatal("InferenceProgressPublisherFactory = non-nil, want nil without gateway")
	}
	if gateway.DispatchCompletionObserverFactory() != nil {
		t.Fatal("DispatchCompletionObserverFactory = non-nil, want nil without gateway")
	}

	gateway.CloseSessionResponseStreams(&livesession.LiveSession{ID: "sess"})
	if gateway.JavaScriptCheckpointStore(&livesession.LiveSession{ID: "sess"}) != nil {
		t.Fatal("JavaScriptCheckpointStore = non-nil, want nil without gateway")
	}
}

func TestServiceConstructionRequiresResponseStreamRegistry(t *testing.T) {
	t.Parallel()

	if gateway := New(nil, nil); gateway != nil {
		t.Fatalf("New without response-stream registry = %#v, want nil", gateway)
	}
}
func canonicalInspectionReadFixture(t *testing.T) (*Service, *canonicalInspectionLiveRuntimeFake, *canonicalInspectionDurableFake) {
	t.Helper()
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
	return &Service{liveRuntime: live, durable: durable}, live, durable
}

func TestService_CanonicalReadsUseModeOwnersAndRuntimeFreeViews(t *testing.T) {
	t.Parallel()

	service, live, _ := canonicalInspectionReadFixture(t)
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
	gotLiveList, err := service.List(context.Background(), factorysessions.SessionListRequest{Mode: factorysessions.SessionOperationModeLive})
	if err != nil {
		t.Fatalf("canonical live List: %v", err)
	}
	if len(gotLiveList.Sessions) != 1 || gotLiveList.Sessions[0].SessionID != "live-2" || gotLiveList.Sessions[0].Status != "PAUSED" {
		t.Fatalf("live list = %#v, want projected live row", gotLiveList.Sessions)
	}
	live.mu.Lock()
	getCalls, listCalls := live.getCalls, live.listCalls
	live.mu.Unlock()
	if getCalls != 1 || listCalls != 1 {
		t.Fatalf("live owner calls = get:%d list:%d, want direct live reads", getCalls, listCalls)
	}
}

func TestService_CanonicalDurableReadsUseModeOwnersAndRuntimeFreeViews(t *testing.T) {
	t.Parallel()

	service, live, durable := canonicalInspectionReadFixture(t)
	assertCanonicalDurableSessionRead(t, service)
	durableListRequest := assertCanonicalDurableSessionList(t, service, durable)
	assertCanonicalAllSessionList(t, service, live)
	assertCanonicalDurableReadCalls(t, live, durable, durableListRequest)
}

func assertCanonicalDurableSessionRead(t *testing.T, service *Service) {
	t.Helper()
	got, err := service.Get(context.Background(), factorysessions.SessionGetRequest{
		SessionID: " durable-1 ", Mode: factorysessions.SessionOperationModeDurable,
	})
	if err != nil {
		t.Fatalf("canonical durable Get: %v", err)
	}
	if got.Session.SessionID != "durable-1" {
		t.Fatalf("durable session ID = %q, want durable-1", got.Session.SessionID)
	}
	if got.Session.Status != "SUCCEEDED" {
		t.Fatalf("durable status = %q, want SUCCEEDED", got.Session.Status)
	}
	if got.Session.ResultStatus != "FINAL" {
		t.Fatalf("durable result status = %q, want FINAL", got.Session.ResultStatus)
	}
	if got.Session.SourceRef != "factory.js" {
		t.Fatalf("durable source ref = %q, want factory.js", got.Session.SourceRef)
	}
}

func assertCanonicalDurableSessionList(t *testing.T, service *Service, durable *canonicalInspectionDurableFake) factorysessions.ListSessionsRequest {
	t.Helper()
	got, err := service.List(context.Background(), factorysessions.SessionListRequest{
		Mode: factorysessions.SessionOperationModeDurable,
		Filters: factorysessions.SessionListFilters{
			SourceRef:         " factory ",
			OrchestratorKinds: []string{" JAVASCRIPT "},
		},
	})
	if err != nil {
		t.Fatalf("canonical durable List: %v", err)
	}
	if len(got.Sessions) != 1 {
		t.Fatalf("durable list count = %d, want one", len(got.Sessions))
	}
	row := got.Sessions[0]
	if row.SessionID != "durable-2" {
		t.Fatalf("durable list session ID = %q, want durable-2", row.SessionID)
	}
	if row.Mode != factorysessions.SessionOperationModeDurable {
		t.Fatalf("durable list mode = %q, want durable", row.Mode)
	}
	if row.ResultStatus != "PARTIAL" {
		t.Fatalf("durable list result status = %q, want PARTIAL", row.ResultStatus)
	}
	durable.mu.Lock()
	defer durable.mu.Unlock()
	return durable.lastList
}

func assertCanonicalAllSessionList(t *testing.T, service *Service, live *canonicalInspectionLiveRuntimeFake) {
	t.Helper()
	got, err := service.List(context.Background(), factorysessions.SessionListRequest{Mode: factorysessions.SessionOperationModeAll})
	if err != nil {
		t.Fatalf("canonical all List: %v", err)
	}
	if len(got.Sessions) != 2 {
		t.Fatalf("all list count = %d, want two", len(got.Sessions))
	}
	if got.Sessions[0].Mode != factorysessions.SessionOperationModeLive {
		t.Fatalf("all list first mode = %q, want live", got.Sessions[0].Mode)
	}
	if got.Sessions[1].Mode != factorysessions.SessionOperationModeDurable {
		t.Fatalf("all list second mode = %q, want durable", got.Sessions[1].Mode)
	}
	if got.Sessions[0].SessionID != "live-2" {
		t.Fatalf("all list first session ID = %q, want live-2", got.Sessions[0].SessionID)
	}
	if got.Sessions[1].SessionID != "durable-2" {
		t.Fatalf("all list second session ID = %q, want durable-2", got.Sessions[1].SessionID)
	}
	live.mu.Lock()
	defer live.mu.Unlock()
	if live.getCalls != 0 {
		t.Fatalf("all list live Get calls = %d, want zero", live.getCalls)
	}
	if live.listCalls != 1 {
		t.Fatalf("all list live List calls = %d, want one", live.listCalls)
	}
}

func assertCanonicalDurableReadCalls(t *testing.T, live *canonicalInspectionLiveRuntimeFake, durable *canonicalInspectionDurableFake, request factorysessions.ListSessionsRequest) {
	t.Helper()
	durable.mu.Lock()
	listCalls, getCalls, legacyCalls := durable.listCalls, durable.getCalls, durable.legacyCalls
	durable.mu.Unlock()
	if listCalls != 2 {
		t.Fatalf("durable list calls = %d, want two", listCalls)
	}
	if getCalls != 1 {
		t.Fatalf("durable get calls = %d, want one", getCalls)
	}
	if legacyCalls != 0 {
		t.Fatalf("durable legacy calls = %d, want zero", legacyCalls)
	}
	if request.Scope != factorysessions.SessionListScopePersisted {
		t.Fatalf("durable list scope = %q, want persisted", request.Scope)
	}
	if request.Filters.SourceRef != "factory" {
		t.Fatalf("durable list source ref = %q, want factory", request.Filters.SourceRef)
	}
	if len(request.Filters.OrchestratorKinds) != 1 || request.Filters.OrchestratorKinds[0] != "JAVASCRIPT" {
		t.Fatalf("durable list orchestrators = %#v, want JAVASCRIPT", request.Filters.OrchestratorKinds)
	}
	live.mu.Lock()
	defer live.mu.Unlock()
	if live.getCalls != 0 || live.listCalls != 1 {
		t.Fatalf("live owner calls = get:%d list:%d, want all-list live read", live.getCalls, live.listCalls)
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
	assertCanonicalControlOperations(t, service, durable, operations)
	assertCanonicalControlPayloadClone(t, service, durable)

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
	if controlCalls != len(operations)+2 || legacyCalls != 0 {
		t.Fatalf("control calls = canonical:%d legacy:%d, want canonical-only owner dispatch", controlCalls, legacyCalls)
	}
}

func assertCanonicalControlOperations(t *testing.T, service *Service, durable *canonicalInspectionDurableFake, operations []factorysessions.SessionControlOperation) {
	t.Helper()
	for _, operation := range operations {
		setCanonicalControlResult(durable, operation)
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
		assertCanonicalControlResult(t, operation, got)
	}
}

func setCanonicalControlResult(durable *canonicalInspectionDurableFake, operation factorysessions.SessionControlOperation) {
	durable.mu.Lock()
	defer durable.mu.Unlock()
	durable.controlResult = durableexecution.CanonicalControlResult{}
	if operation == factorysessions.SessionControlRecover {
		durable.controlResult.Recovery = &factorysessions.AsyncStartResult{SessionID: "recovered", Status: "QUEUED"}
		return
	}
	durable.controlResult.Lifecycle = &factorysessions.LifecycleControlResult{
		Outcome:           factorysessions.LifecycleControlOutcomeAccepted,
		Status:            factorysessions.LifecycleStatusRunning,
		Detail:            "owner detail",
		ApprovalPreviewID: "preview-1",
		DispatchID:        "dispatch-1",
		RetryDispatchID:   "retry-1",
	}
}

func assertCanonicalControlResult(t *testing.T, operation factorysessions.SessionControlOperation, got factorysessions.SessionControlResult) {
	t.Helper()
	if got.SessionID != "durable-1" || got.Mode != factorysessions.SessionOperationModeDurable || got.Operation != operation {
		t.Fatalf("Control(%s) identity = %#v, want normalized owner identity", operation, got)
	}
	if operation == factorysessions.SessionControlRecover {
		if got.Recovery == nil || got.Recovery.SessionID != "recovered" || got.Status != factorysessions.LifecycleStatus("QUEUED") {
			t.Fatalf("Control(%s) recovery = %#v, want typed recovery result", operation, got)
		}
		return
	}
	if got.Outcome != factorysessions.LifecycleControlOutcomeAccepted || got.Status != factorysessions.LifecycleStatusRunning || got.DispatchID != "dispatch-1" {
		t.Fatalf("Control(%s) result = %#v, want characterized lifecycle outcome", operation, got)
	}
}

func assertCanonicalControlPayloadClone(t *testing.T, service *Service, durable *canonicalInspectionDurableFake) {
	t.Helper()
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
}

// TestService_CanonicalResultDispatchAndResponseReadsValidateAndClone proves
// terminal/partial result, filtered dispatch, and durable response reads keep
// typed values, normalized cursors, and caller isolation at the root boundary.
func canonicalInspectionResultFixture() (*Service, *canonicalInspectionDurableFake) {
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
	return &Service{durable: durable}, durable
}

func TestService_CanonicalResultDispatchAndResponseReadsValidateAndClone(t *testing.T) {
	t.Parallel()

	service, durable := canonicalInspectionResultFixture()
	assertCanonicalDurableResultRead(t, service, durable)
	assertCanonicalDurableDispatchAndResponseReads(t, service, durable)
	assertCanonicalDurableInspectionValidation(t)
}

func assertCanonicalDurableResultRead(t *testing.T, service *Service, durable *canonicalInspectionDurableFake) {
	t.Helper()
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
}

func assertCanonicalDurableInspectionValidation(t *testing.T) {
	t.Helper()
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
	dispatchCalls, responseCalls := invalid.dispatchCalls, invalid.responseCalls
	invalid.mu.Unlock()
	if dispatchCalls != 0 || responseCalls != 0 {
		t.Fatalf("invalid input owner calls = dispatch:%d response:%d, want zero", dispatchCalls, responseCalls)
	}
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

type canonicalDurableExecutionFake struct {
	durableexecution.Service
	canonicalDurableOwner
	asyncRequest   factorysessions.StartRequest
	syncRequest    factorysessions.StartRequest
	asyncResult    factorysessions.AsyncStartResult
	syncResult     factorysessions.SyncStartResult
	asyncErr       error
	syncErr        error
	canonicalCalls int
	asyncCalls     int
	syncCalls      int
}

type canonicalDurableOwner interface {
	canonicaldurable.Service
}

func (fake *canonicalDurableExecutionFake) StartCanonical(
	_ context.Context,
	request factorysessions.StartRequest,
	synchronous bool,
) (durableexecution.CanonicalStartResult, error) {
	fake.canonicalCalls++
	if synchronous {
		fake.syncRequest = request
		started := fake.syncResult
		if fake.syncErr != nil {
			return durableexecution.CanonicalStartResult{}, fake.syncErr
		}
		return durableexecution.CanonicalStartResult{Sync: &started}, nil
	}
	fake.asyncRequest = request
	started := fake.asyncResult
	if fake.asyncErr != nil {
		return durableexecution.CanonicalStartResult{}, fake.asyncErr
	}
	return durableexecution.CanonicalStartResult{Async: &started}, nil
}

func (fake *canonicalDurableExecutionFake) StartAsync(
	_ context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	fake.asyncCalls++
	fake.asyncRequest = request
	return fake.asyncResult, fake.asyncErr
}

func (fake *canonicalDurableExecutionFake) StartSync(
	_ context.Context,
	request factorysessions.StartRequest,
) (factorysessions.SyncStartResult, error) {
	fake.syncCalls++
	fake.syncRequest = request
	return fake.syncResult, fake.syncErr
}

type canonicalSessionInvokerFake struct {
	roles.SessionInvoker
	sessionID       string
	requestID       string
	timeout         int64
	cancelOnTimeout bool
	input           *work.PreparedInvocationInput
	result          factorydefinitions.FactoryInvocationResult
	err             error
	canonicalCalls  int
	legacyCalls     int
	calls           int
	mutateInput     bool
}

func (fake *canonicalSessionInvokerFake) Invoke(
	_ context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	fake.canonicalCalls++
	return fake.recordInvocation(sessionID, request)
}

func (fake *canonicalSessionInvokerFake) InvokeFactorySession(
	_ context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	fake.legacyCalls++
	return fake.recordInvocation(sessionID, request)
}

func (fake *canonicalSessionInvokerFake) recordInvocation(
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	fake.calls++
	fake.sessionID = sessionID
	if request.RequestID != nil {
		fake.requestID = *request.RequestID
	}
	if request.TimeoutMillis != nil {
		fake.timeout = *request.TimeoutMillis
	}
	fake.cancelOnTimeout = request.CancelOnTimeout
	fake.input = request.PreparedInvocationInput.Clone()
	if fake.mutateInput && request.PreparedInvocationInput != nil && request.PreparedInvocationInput.ResolvedInput != nil {
		request.PreparedInvocationInput.ResolvedInput.Text = "owner mutation"
	}
	return fake.result, fake.err
}

func TestService_CanonicalStartDurableMapsAndClonesAsyncRequest(t *testing.T) {
	t.Parallel()

	fake := &canonicalDurableExecutionFake{
		asyncResult: factorysessions.AsyncStartResult{
			SessionID: "durable-async-1",
			Status:    "QUEUED",
			Policy: factorysessions.PolicyProjection{
				Requested: map[string]any{"nested": map[string]any{"value": "kept"}},
			},
		},
	}
	service := &Service{durable: fake}
	request := factorysessions.SessionStartRequest{
		Mode: factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{
			RequestID: "  start-async-1  ",
		},
		Source: factorysessions.Source{
			FactoryInline: json.RawMessage(`{"name":"factory"}`),
		},
		Args: map[string]any{
			"nested": map[string]any{"value": "original"},
		},
		Policy: map[string]any{"mode": "safe"},
		Orchestrator: &factorysessions.OrchestratorOverride{
			Kind: "petri",
			Raw:  json.RawMessage(`{"version":1}`),
		},
		RuntimeOptions: &factorysessions.RuntimeOptions{ChildExecutorMode: "direct"},
		Wait: factorysessions.SessionOperationWait{
			TimeoutMillis:   250,
			CancelOnTimeout: true,
		},
	}
	got, err := service.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	assertCanonicalAsyncStartResult(t, got)
	assertCanonicalAsyncStartRequest(t, fake)
	assertCanonicalAsyncStartIsolation(t, fake, request, got)
}

func assertCanonicalAsyncStartResult(t *testing.T, got factorysessions.SessionStartResult) {
	t.Helper()
	if got.SessionID != "durable-async-1" {
		t.Fatalf("Start() session ID = %q, want durable-async-1", got.SessionID)
	}
	if got.Mode != factorysessions.SessionOperationModeDurable {
		t.Fatalf("Start() mode = %q, want durable", got.Mode)
	}
	if got.Status != "QUEUED" {
		t.Fatalf("Start() status = %q, want QUEUED", got.Status)
	}
	if got.Async == nil {
		t.Fatalf("Start() async branch = nil, want async result")
	}
	if got.Sync != nil {
		t.Fatalf("Start() sync branch = non-nil, want async only")
	}
}

func assertCanonicalAsyncStartRequest(t *testing.T, fake *canonicalDurableExecutionFake) {
	t.Helper()
	if fake.canonicalCalls != 1 {
		t.Fatalf("durable canonical calls = %d, want one", fake.canonicalCalls)
	}
	if fake.asyncCalls != 0 || fake.syncCalls != 0 {
		t.Fatalf("durable legacy calls = async:%d sync:%d, want zero", fake.asyncCalls, fake.syncCalls)
	}
	if fake.asyncRequest.RequestID != "start-async-1" {
		t.Fatalf("durable request ID = %q, want start-async-1", fake.asyncRequest.RequestID)
	}
	if fake.asyncRequest.Wait == nil || fake.asyncRequest.Wait.TimeoutMillis == nil {
		t.Fatalf("durable request wait = %#v, want timeout", fake.asyncRequest.Wait)
	}
	if *fake.asyncRequest.Wait.TimeoutMillis != 250 || !fake.asyncRequest.Wait.CancelOnTimeout {
		t.Fatalf("durable request wait = %#v, want timeout/cancel policy", fake.asyncRequest.Wait)
	}
	if fake.asyncRequest.EventConsumer != nil {
		t.Fatal("canonical durable Start installed an event consumer")
	}
}

func assertCanonicalAsyncStartIsolation(t *testing.T, fake *canonicalDurableExecutionFake, request factorysessions.SessionStartRequest, got factorysessions.SessionStartResult) {
	t.Helper()
	if fake.asyncRequest.Args["nested"].(map[string]any)["value"] != "original" {
		t.Fatalf("durable request args = %#v, want cloned original", fake.asyncRequest.Args)
	}
	if string(fake.asyncRequest.Source.FactoryInline) != `{"name":"factory"}` {
		t.Fatalf("durable request source = %#v, want cloned inline source", fake.asyncRequest.Source)
	}
	if string(fake.asyncRequest.Orchestrator.Raw) != `{"version":1}` {
		t.Fatalf("durable request orchestrator = %#v, want cloned raw value", fake.asyncRequest.Orchestrator)
	}

	request.Args["nested"].(map[string]any)["value"] = "caller mutation"
	request.Source.FactoryInline[1] = 'x'
	request.Orchestrator.Raw[1] = 'x'
	if fake.asyncRequest.Args["nested"].(map[string]any)["value"] != "original" {
		t.Fatal("canonical durable Start crossed caller-owned nested args")
	}
	if string(fake.asyncRequest.Source.FactoryInline) != `{"name":"factory"}` || string(fake.asyncRequest.Orchestrator.Raw) != `{"version":1}` {
		t.Fatal("canonical durable Start crossed caller-owned raw source values")
	}

	fake.asyncResult.Policy.Requested["nested"].(map[string]any)["value"] = "owner mutation"
	if got.Async.Policy.Requested["nested"].(map[string]any)["value"] != "kept" {
		t.Fatal("canonical durable Start returned an aliased result map")
	}
}

func TestService_CanonicalStartDurableSelectsSyncFromRequestValue(t *testing.T) {
	t.Parallel()

	fake := &canonicalDurableExecutionFake{
		syncResult: factorysessions.SyncStartResult{
			AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "durable-sync-1"},
			SyncOutcome:      factorysessions.SyncOutcome("COMPLETED"),
		},
	}
	service := &Service{durable: fake}
	got, err := service.Start(context.Background(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "sync-1"},
		Synchronous: true,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if fake.canonicalCalls != 1 || fake.asyncCalls != 0 || fake.syncCalls != 0 {
		t.Fatalf("durable calls = canonical:%d async:%d sync:%d, want 1/0/0", fake.canonicalCalls, fake.asyncCalls, fake.syncCalls)
	}
	if got.SessionID != "durable-sync-1" || got.Status != "COMPLETED" || got.Sync == nil || got.Async != nil {
		t.Fatalf("Start() = %#v, want synchronous completed result", got)
	}
}

func TestService_CanonicalInvokeMapsIdentityAndClonesPreparedWork(t *testing.T) {
	t.Parallel()

	fake := &canonicalSessionInvokerFake{
		mutateInput: true,
		result: factorydefinitions.FactoryInvocationResult{
			RequestID: "invoke-1",
			TraceID:   "trace-1",
			Status:    factorydefinitions.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{
				Type:     work.WorkContentPartTypeText,
				Text:     "done",
				Metadata: map[string]any{"source": "owner"},
			}},
			SessionID: "session-1",
			WorkID:    "work-1",
			WorkName:  "invoke",
			WorkState: "SUCCEEDED",
		},
	}
	service := &Service{invoker: fake}
	input := &work.PreparedInvocationInput{
		Source: work.InputSourcePositionalText,
		ResolvedInput: &work.ResolvedInput{
			Source: work.InputSourcePositionalText,
			Text:   "caller input",
		},
	}
	got, err := service.Invoke(context.Background(), factorysessions.SessionInvokeRequest{
		SessionID:   "  session-1  ",
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "  invoke-1  "},
		Input:       input,
		Wait:        factorysessions.SessionOperationWait{TimeoutMillis: 500, CancelOnTimeout: true},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	assertCanonicalInvokeRequest(t, fake, input)
	assertCanonicalInvokeResult(t, fake, got)
}

func assertCanonicalInvokeRequest(t *testing.T, fake *canonicalSessionInvokerFake, input *work.PreparedInvocationInput) {
	t.Helper()
	if fake.canonicalCalls != 1 || fake.legacyCalls != 0 || fake.calls != 1 || fake.sessionID != "session-1" || fake.requestID != "invoke-1" || fake.timeout != 500 || !fake.cancelOnTimeout {
		t.Fatalf("invoker request = canonical:%d legacy:%d session:%q request:%q timeout:%d cancel:%t calls:%d, want canonical-only normalized values", fake.canonicalCalls, fake.legacyCalls, fake.sessionID, fake.requestID, fake.timeout, fake.cancelOnTimeout, fake.calls)
	}
	if input.ResolvedInput.Text != "caller input" {
		t.Fatal("canonical Invoke crossed caller-owned prepared Work input")
	}
	if fake.input == nil || fake.input.ResolvedInput == nil || fake.input.ResolvedInput.Text != "caller input" {
		t.Fatalf("invoker input = %#v, want cloned prepared input", fake.input)
	}
}

func assertCanonicalInvokeResult(t *testing.T, fake *canonicalSessionInvokerFake, got factorysessions.InvocationResult) {
	t.Helper()
	if got.RequestID != "invoke-1" || got.TraceID != "trace-1" || got.SessionID != "session-1" || got.WorkID != "work-1" || got.WorkName != "invoke" || got.WorkState != "SUCCEEDED" || got.Status != factorysessions.InvocationTerminalStatusCompleted || len(got.PrimaryResult) != 1 || got.PrimaryResult[0].Text != "done" {
		t.Fatalf("Invoke() = %#v, want characterized identity/result", got)
	}
	fake.result.PrimaryResult[0].Metadata["source"] = "owner mutation"
	if got.PrimaryResult[0].Metadata["source"] != "owner" {
		t.Fatal("canonical Invoke returned an aliased primary result")
	}
}
