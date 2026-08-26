package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestFakeService_LifecycleControl_IdempotentReplayAndConflict(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")

	first, err := service.Pause(context.Background(), "dur-sess-js-run-n-001", ControlRequest{
		RequestID: "ctrl-pause-replay-001",
	})
	if err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	second, err := service.Pause(context.Background(), "dur-sess-js-run-n-001", ControlRequest{
		RequestID: "ctrl-pause-replay-001",
	})
	if err != nil {
		t.Fatalf("replay Pause: %v", err)
	}
	if second.Outcome != first.Outcome || second.Status != first.Status {
		t.Fatalf("replay result = %#v, want %#v", second, first)
	}

	_, err = service.Resume(context.Background(), "dur-sess-js-run-n-001", ControlRequest{
		RequestID: "ctrl-pause-replay-001",
	})
	var controlErr *ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != LifecycleControlOutcomeConflict {
		t.Fatalf("conflict error = %v, want CONFLICT ControlError", err)
	}
	if controlErr.Status != LifecycleStatusPaused {
		t.Fatalf("conflict status = %q, want PAUSED", controlErr.Status)
	}
}

func TestFakeService_LifecycleControls_UpdateStateAndErrors(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")

	paused, err := service.Pause(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Outcome != LifecycleControlOutcomeAccepted || paused.Status != LifecycleStatusPaused {
		t.Fatalf("pause result = %#v", paused)
	}

	resumed, err := service.Resume(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.Status != LifecycleStatusRunning {
		t.Fatalf("resume status = %q, want RUNNING", resumed.Status)
	}

	startAsyncByRequestID(t, service, "req-petri-success-001")
	_, err = service.RetryDispatch(context.Background(), "dur-sess-petri-success-001", RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "disp-petri-success-001",
	})
	var controlErr *ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on terminal error = %v, want TERMINAL_SESSION", err)
	}

	startAsyncByRequestID(t, service, "req-js-failed-partial-001")
	retry, err := service.RetryDispatch(context.Background(), "dur-sess-js-failed-partial-001", RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "disp-js-fail-002",
	})
	if err != nil {
		t.Fatalf("RetryDispatch on failed session: %v", err)
	}
	if retry.Outcome != LifecycleControlOutcomeAccepted || retry.Status != LifecycleStatusRunning {
		t.Fatalf("retry result = %#v", retry)
	}

	_, err = service.RetryDispatch(context.Background(), "dur-sess-js-failed-partial-001", RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "missing-dispatch",
	})
	if !errors.Is(err, ErrDispatchNotFound) {
		t.Fatalf("missing dispatch error = %v", err)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper regression keeps lifecycle mutation assertions together on one scenario.
func TestFakeService_InternalLifecycleHelpers(t *testing.T) {
	t.Parallel()
	state := &fakeSessionState{
		session: SessionReadResult{
			SessionID: "dur-sess-1",
			Status:    LifecycleStatusAwaitingApproval,
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-1",
			SessionStatus: LifecycleStatusAwaitingApproval,
			ResultStatus:  ResultStatusNotReady,
		},
		dispatches: []DispatchSummary{{ID: "disp-1", Status: DispatchStatusFailed, Attempt: 1}},
		dispatchDetails: map[string]DispatchDetail{
			"disp-1": {DispatchSummary: DispatchSummary{ID: "disp-1", Status: DispatchStatusFailed, Attempt: 1}},
		},
	}

	if err := validateLifecycleControlRequest(LifecycleControlApprove, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{}); err != nil {
		t.Fatalf("approve validation: %v", err)
	}
	if err := validateLifecycleControlRequest(LifecycleControlRetryDispatch, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{}); err == nil {
		t.Fatal("retry without dispatch id should fail validation")
	}
	if err := validateLifecycleControlRequest(LifecycleControlInterruptDispatch, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{}); err == nil {
		t.Fatal("interrupt without dispatch id should fail validation")
	}
	if err := validateLifecycleControlRequest(LifecycleControlPause, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{}); err != nil {
		t.Fatalf("pause validation: %v", err)
	}

	accepted := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlApprove, LifecycleControlOutcomeAccepted, RetryDispatchRequest{}, InterruptDispatchRequest{})
	if accepted.Session == nil || accepted.Session.Status != LifecycleStatusAwaitingApproval {
		t.Fatalf("accepted lifecycle control result = %#v", accepted)
	}
	noop := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlPause, LifecycleControlOutcomeNoOp, RetryDispatchRequest{}, InterruptDispatchRequest{})
	if noop.Session == nil {
		t.Fatalf("noop lifecycle control result = %#v", noop)
	}
	retry := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlRetryDispatch, LifecycleControlOutcomeAccepted, RetryDispatchRequest{DispatchID: "disp-1"}, InterruptDispatchRequest{})
	if retry.DispatchID != "disp-1" || retry.RetryDispatchID != "disp-1" {
		t.Fatalf("retry lifecycle control result = %#v", retry)
	}
	interrupt := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlInterruptDispatch, LifecycleControlOutcomeAccepted, RetryDispatchRequest{}, InterruptDispatchRequest{DispatchID: "disp-1"})
	if interrupt.DispatchID != "disp-1" {
		t.Fatalf("interrupt lifecycle control result = %#v", interrupt)
	}

	service := &FakeService{}
	service.mutateSessionForControl(state, LifecycleControlApprove, RetryDispatchRequest{}, InterruptDispatchRequest{})
	if state.session.Status != LifecycleStatusRunning {
		t.Fatalf("approve mutate status = %q, want RUNNING", state.session.Status)
	}
	state.session.Status = LifecycleStatusFailed
	state.result.SessionStatus = LifecycleStatusFailed
	service.mutateSessionForControl(state, LifecycleControlRetryDispatch, RetryDispatchRequest{DispatchID: "disp-1"}, InterruptDispatchRequest{})
	if state.session.Status != LifecycleStatusRunning || state.dispatches[0].Status != DispatchStatusQueued || state.dispatches[0].Attempt != 2 {
		t.Fatalf("retry mutate state = %#v / %#v", state.session, state.dispatches[0])
	}
	state.dispatches[0].Status = DispatchStatusRunning
	state.dispatches[0].Attempt = 1
	service.mutateSessionForControl(state, LifecycleControlInterruptDispatch, RetryDispatchRequest{}, InterruptDispatchRequest{DispatchID: "disp-1"})
	if state.dispatches[0].Status != DispatchStatusInterrupted {
		t.Fatalf("interrupt mutate status = %q, want INTERRUPTED", state.dispatches[0].Status)
	}
}

func TestFakeService_LifecycleControl_ErrorBranches(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Pause(canceledCtx, "dur-sess-js-run-n-001", ControlRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Pause error = %v", err)
	}
	if _, err := service.Pause(context.Background(), " ", ControlRequest{}); err == nil {
		t.Fatal("blank session id should fail")
	}
	if _, err := service.Pause(context.Background(), "missing-session", ControlRequest{}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing session Pause error = %v", err)
	}

	startAsyncByRequestID(t, service, "req-petri-success-001")
	if _, err := service.Terminate(context.Background(), "dur-sess-petri-success-001", ControlRequest{RequestID: "term-1"}); err == nil {
		t.Fatal("terminate on terminal session should fail")
	}
	if _, err := service.Terminate(context.Background(), "dur-sess-petri-success-001", ControlRequest{RequestID: "term-1"}); err == nil {
		t.Fatal("terminate replay should repeat stored error")
	}

	startAsyncByRequestID(t, service, "req-js-awaiting-001")
	if _, err := service.Approve(context.Background(), "dur-sess-js-awaiting-001", ApproveRequest{}); err != nil {
		t.Fatalf("Approve awaiting session: %v", err)
	}
}

func TestFakeService_ReadProjections_MatchFixtureDispatchesArtifactsEvents(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	dispatches, err := service.ListDispatches(context.Background(), "dur-sess-petri-success-001")
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].ID != "disp-petri-success-001" {
		t.Fatalf("dispatches = %#v", dispatches.Dispatches)
	}

	artifacts, err := service.ListArtifacts(context.Background(), "dur-sess-petri-success-001")
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 1 || artifacts.Artifacts[0].ID != "art-petri-final-001" {
		t.Fatalf("artifacts = %#v", artifacts.Artifacts)
	}

	events, err := service.ReadEvents(context.Background(), "dur-sess-petri-success-001", EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) == 0 {
		t.Fatal("events missing")
	}
	result, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if err := ValidateResultMatchesEventProjection(result, events.Events); err != nil {
		t.Fatalf("ValidateResultMatchesEventProjection: %v", err)
	}
}

func TestFakeService_ReadMethods_ErrorAndFallbackBranches(t *testing.T) {
	t.Parallel()
	t.Run("session and result branches", testFakeServiceReadSessionAndResultBranches)
	t.Run("dispatch and artifact branches", testFakeServiceReadDispatchAndArtifactBranches)
	t.Run("event and listing branches", testFakeServiceReadEventAndListingBranches)
}

func testFakeServiceReadSessionAndResultBranches(t *testing.T) {
	t.Helper()

	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.GetSession(canceledCtx, "dur-sess-petri-success-001"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled GetSession error = %v", err)
	}
	if _, err := service.GetSession(context.Background(), " "); err == nil {
		t.Fatal("blank GetSession id should fail")
	}
	if _, err := service.GetSession(context.Background(), "missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing GetSession error = %v", err)
	}

	if _, err := service.GetResult(canceledCtx, "dur-sess-petri-success-001", ResultRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled GetResult error = %v", err)
	}
	if _, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{Mode: ResultMode("bad")}); err == nil {
		t.Fatal("invalid GetResult mode should fail")
	}
}

func testFakeServiceReadDispatchAndArtifactBranches(t *testing.T) {
	t.Helper()

	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := service.ListDispatches(canceledCtx, "dur-sess-petri-success-001"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ListDispatches error = %v", err)
	}
	detail, err := service.GetDispatch(context.Background(), "dur-sess-petri-success-001", "disp-petri-success-001")
	if err != nil {
		t.Fatalf("GetDispatch detail: %v", err)
	}
	if detail.OrchestratorKind == "" {
		t.Fatalf("dispatch detail = %#v", detail)
	}
	fallbackService := mustNewFakeService(t, FakeScenario{
		RequestID: "fallback-dispatch",
		Session: SessionReadResult{
			SessionID:        "dur-sess-fallback",
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "PETRI",
		},
		Result: ResultReadResult{
			SessionID:     "dur-sess-fallback",
			SessionStatus: LifecycleStatusRunning,
			ResultStatus:  ResultStatusNotReady,
		},
		Dispatches: []DispatchSummary{{ID: "disp-fallback", DispatchKind: "PETRI", Status: DispatchStatusQueued}},
	})

	startAsyncByRequestID(t, fallbackService, "fallback-dispatch")
	fallback, err := fallbackService.GetDispatch(context.Background(), "dur-sess-fallback", "disp-fallback")
	if err != nil {
		t.Fatalf("GetDispatch fallback: %v", err)
	}
	if fallback.ID != "disp-fallback" || fallback.OrchestratorKind != "PETRI" {
		t.Fatalf("fallback dispatch detail = %#v", fallback)
	}
	if _, err := service.GetDispatch(context.Background(), "dur-sess-petri-success-001", "missing"); !errors.Is(err, ErrDispatchNotFound) {
		t.Fatalf("missing GetDispatch error = %v", err)
	}

	if _, err := service.ListArtifacts(canceledCtx, "dur-sess-petri-success-001"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ListArtifacts error = %v", err)
	}
	artifact, err := service.GetArtifact(context.Background(), "dur-sess-petri-success-001", "art-petri-final-001")
	if err != nil {
		t.Fatalf("GetArtifact detail: %v", err)
	}
	if artifact.ID != "art-petri-final-001" {
		t.Fatalf("artifact detail = %#v", artifact)
	}
	fallbackArtifactService := mustNewFakeService(t, FakeScenario{
		RequestID: "fallback-artifact",
		Session: SessionReadResult{
			SessionID: "dur-sess-art-fallback",
			Status:    LifecycleStatusSucceeded,
		},
		Result: ResultReadResult{
			SessionID:     "dur-sess-art-fallback",
			SessionStatus: LifecycleStatusSucceeded,
			ResultStatus:  ResultStatusFinal,
		},
		Artifacts: []ArtifactSummary{{ID: "art-fallback", Kind: "LOG", Visibility: "PUBLIC"}},
	})

	startAsyncByRequestID(t, fallbackArtifactService, "fallback-artifact")
	fallbackArtifact, err := fallbackArtifactService.GetArtifact(context.Background(), "dur-sess-art-fallback", "art-fallback")
	if err != nil {
		t.Fatalf("GetArtifact fallback: %v", err)
	}
	if fallbackArtifact.ID != "art-fallback" {
		t.Fatalf("fallback artifact detail = %#v", fallbackArtifact)
	}
	if _, err := service.GetArtifact(context.Background(), "dur-sess-petri-success-001", "missing"); !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("missing GetArtifact error = %v", err)
	}
}

func testFakeServiceReadEventAndListingBranches(t *testing.T) {
	t.Helper()

	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := service.ReadEvents(canceledCtx, "dur-sess-petri-success-001", EventReconnectRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ReadEvents error = %v", err)
	}
	sequence := -1
	if _, err := service.ReadEvents(context.Background(), "dur-sess-petri-success-001", EventReconnectRequest{AfterSequence: &sequence}); err == nil {
		t.Fatal("negative reconnect sequence should fail")
	}

	if _, err := service.ListSessions(canceledCtx, ListSessionsRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ListSessions error = %v", err)
	}
	if _, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScope("bad")}); err == nil {
		t.Fatal("invalid list sessions scope should fail")
	}
}

func TestFakeService_ReadEvents_ReturnsCanonicalFixtureEventsAndHonorsCursor(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	cases := []struct {
		requestID string
		sessionID string
		wantCount int
	}{
		{"req-js-run-n-001", "dur-sess-js-run-n-001", 2},
		{"req-js-success-002", "dur-sess-js-success-002", 3},
		{"req-js-awaiting-001", "dur-sess-js-awaiting-001", 2},
	}
	for _, tc := range cases {
		t.Run(tc.sessionID, func(t *testing.T) {
			startAsyncByRequestID(t, service, tc.requestID)
			all, err := service.ReadEvents(context.Background(), tc.sessionID, EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			if len(all.Events) != tc.wantCount {
				t.Fatalf("events = %d, want %d", len(all.Events), tc.wantCount)
			}
			for index, raw := range all.Events {
				assertCanonicalEventEnvelope(t, raw, "", "")
				_ = index
			}

			afterStart, err := service.ReadEvents(context.Background(), tc.sessionID, EventReconnectRequest{
				AfterEventID: "session-started/" + tc.sessionID,
			})
			if err != nil {
				t.Fatalf("ReadEvents after start: %v", err)
			}
			if len(afterStart.Events) != tc.wantCount-1 {
				t.Fatalf("after start events = %d, want %d", len(afterStart.Events), tc.wantCount-1)
			}
		})
	}
}

func TestFakeService_ReadEvents_InvalidCursorReturnsError(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")
	_, err := service.ReadEvents(context.Background(), "dur-sess-js-run-n-001", EventReconnectRequest{
		AfterEventID: "missing-event-id",
	})
	if !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func TestFakeService_DerivedProjectionEvents_AreCanonicalWhenFixtureEventsMissing(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-run-001")
	events, err := service.ReadEvents(context.Background(), "dur-sess-petri-run-001", EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) == 0 {
		t.Fatal("derived events missing")
	}
	for _, raw := range events.Events {
		assertCanonicalEventEnvelope(t, raw, "", "")
	}
}

func TestFakeService_ListSessions_ScopedPersistedAndLive(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-run-001")

	live, err := service.ListSessions(context.Background(), ListSessionsRequest{
		Scope: SessionListScopeLive,
	})
	if err != nil {
		t.Fatalf("ListSessions live: %v", err)
	}
	foundLiveRunning := false
	for _, session := range live.LiveSessions {
		if session.ID == "dur-sess-petri-run-001" {
			foundLiveRunning = true
			break
		}
	}
	if !foundLiveRunning {
		t.Fatalf("live sessions = %#v, want current-process running row", live.LiveSessions)
	}
	if len(live.DurableSessions) != 0 {
		t.Fatalf("live durable sessions = %#v, want none", live.DurableSessions)
	}

	persisted, err := service.ListSessions(context.Background(), ListSessionsRequest{
		Scope: SessionListScopePersisted,
	})
	if err != nil {
		t.Fatalf("ListSessions persisted: %v", err)
	}
	if len(persisted.DurableSessions) == 0 {
		t.Fatal("persisted sessions missing seeded terminal rows")
	}
	for _, summary := range persisted.DurableSessions {
		if summary.SessionID == "dur-sess-petri-run-001" {
			t.Fatalf("persisted scope unexpectedly contains running session %#v", summary)
		}
	}

	all, err := service.ListSessions(context.Background(), ListSessionsRequest{
		Scope: SessionListScopeAll,
	})
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}
	foundAllLiveRunning := false
	for _, session := range all.LiveSessions {
		if session.ID == "dur-sess-petri-run-001" {
			foundAllLiveRunning = true
			break
		}
	}
	if !foundAllLiveRunning {
		t.Fatalf("all live sessions = %#v, want current-process running row", all.LiveSessions)
	}
}

func TestFilterDispatches_PhaseStatusAndValidation(t *testing.T) {
	t.Parallel()
	input := ListDispatchesResult{SessionID: "dur-sess-filter-001", Dispatches: []DispatchSummary{
		{ID: "one", Phase: "plan", Status: DispatchStatusCompleted},
		{ID: "two", Phase: "build", Status: DispatchStatusFailed},
		{ID: "three", Phase: "build", Status: DispatchStatusCompleted},
	}}

	filtered, err := FilterDispatches(input, DispatchFilters{Phase: "build", Status: " completed "})
	if err != nil {
		t.Fatalf("FilterDispatches: %v", err)
	}
	if len(filtered.Dispatches) != 1 || filtered.Dispatches[0].ID != "three" {
		t.Fatalf("filtered dispatches = %#v, want dispatch three", filtered.Dispatches)
	}
	empty, err := FilterDispatches(input, DispatchFilters{Phase: "unknown"})
	if err != nil || len(empty.Dispatches) != 0 || empty.Dispatches == nil {
		t.Fatalf("unknown phase = %#v, %v; want non-nil empty list", empty.Dispatches, err)
	}
	if _, err := FilterDispatches(input, DispatchFilters{Status: "BROKEN"}); err == nil {
		t.Fatal("invalid status error = nil, want ValidationError")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "status" {
		t.Fatalf("invalid status error = %#v, want status ValidationError", err)
	}
}

type recordingDispatchListReader struct {
	sessionID string
	result    ListDispatchesResult
}

func (reader *recordingDispatchListReader) ListDispatches(_ context.Context, sessionID string) (ListDispatchesResult, error) {
	reader.sessionID = sessionID
	return reader.result, nil
}

func TestQueryDispatches_ReadsAndFiltersInsideExecutionOwner(t *testing.T) {
	t.Parallel()
	reader := &recordingDispatchListReader{result: ListDispatchesResult{
		SessionID: "dur-sess-query-001",
		Dispatches: []DispatchSummary{
			{ID: "plan", Phase: "plan", Status: DispatchStatusCompleted},
			{ID: "build", Phase: "build", Status: DispatchStatusCompleted},
		},
	}}

	result, err := queryDispatches(context.Background(), reader, DispatchQueryRequest{
		SessionID: "dur-sess-query-001",
		Filters:   DispatchFilters{Phase: "build", Status: " completed "},
	})
	if err != nil {
		t.Fatalf("queryDispatches: %v", err)
	}
	if reader.sessionID != "dur-sess-query-001" {
		t.Fatalf("ListDispatches sessionID = %q", reader.sessionID)
	}
	if len(result.Dispatches) != 1 || result.Dispatches[0].ID != "build" {
		t.Fatalf("query result = %#v, want build dispatch", result.Dispatches)
	}
}

func TestProgressCountsFromDispatches_GroupsEveryCanonicalStatus(t *testing.T) {
	t.Parallel()
	dispatches := []DispatchSummary{
		{Status: DispatchStatusQueued}, {Status: DispatchStatusRunning},
		{Status: DispatchStatusCompleted}, {Status: DispatchStatusFailed},
		{Status: DispatchStatusCanceled}, {Status: DispatchStatusTimedOut},
		{Status: DispatchStatusSkipped}, {Status: DispatchStatusInterrupted},
	}

	got := progressCountsFromDispatches(dispatches, 3)
	if got.TotalDispatches != 8 || got.PhaseCount != 3 || got.InFlightDispatches != 2 ||
		got.CompletedDispatches != 1 || got.FailedDispatches != 1 ||
		got.QueuedDispatches != 1 || got.RunningDispatches != 1 ||
		got.CanceledDispatches != 1 || got.TimedOutDispatches != 1 ||
		got.SkippedDispatches != 1 || got.InterruptedDispatches != 1 {
		t.Fatalf("progress counts = %#v, want one per canonical status", got)
	}
}

func TestNormalizeResultRequest_DefaultsAndValidation(t *testing.T) {
	t.Parallel()
	normalized, err := NormalizeResultRequest(ResultRequest{})
	if err != nil {
		t.Fatalf("NormalizeResultRequest: %v", err)
	}
	if normalized.Mode != ResultModeFinal {
		t.Fatalf("mode = %q, want final", normalized.Mode)
	}

	partial, err := NormalizeResultRequest(ResultRequest{Mode: ResultModePartial, IncludeArtifacts: true})
	if err != nil {
		t.Fatalf("NormalizeResultRequest partial: %v", err)
	}
	if !partial.IncludeArtifacts {
		t.Fatal("includeArtifacts = false, want true")
	}

	_, err = NormalizeResultRequest(ResultRequest{Mode: ResultMode("invalid")})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
}

func TestNormalizeEventReconnectRequest_RejectsNegativeSequence(t *testing.T) {
	t.Parallel()
	sequence := -1
	_, err := NormalizeEventReconnectRequest(EventReconnectRequest{AfterSequence: &sequence})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
}

func TestValidateResultMatchesSessionRead(t *testing.T) {
	t.Parallel()
	session := SessionReadResult{
		SessionID: "dur-sess-001",
		Status:    LifecycleStatusRunning,
		ResultSummary: &ResultSummary{
			ResultStatus: string(ResultStatusPartial),
		},
	}
	result := ResultReadResult{
		SessionID:     "dur-sess-001",
		ResultStatus:  ResultStatusPartial,
		SessionStatus: LifecycleStatusRunning,
	}
	if err := ValidateResultMatchesSessionRead(session, result); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}

	mismatch := result
	mismatch.ResultStatus = ResultStatusFinal
	if err := ValidateResultMatchesSessionRead(session, mismatch); err == nil {
		t.Fatal("error = nil, want mismatch")
	}
}

func TestValidateDispatchListMatchesSessionProgress(t *testing.T) {
	t.Parallel()
	session := SessionReadResult{
		SessionID: "dur-sess-001",
		Progress: &ProgressCounts{
			TotalDispatches: 3,
		},
	}
	dispatches := []DispatchSummary{
		{ID: "disp-1"},
		{ID: "disp-2"},
		{ID: "disp-3"},
	}
	if err := ValidateDispatchListMatchesSessionProgress(session, dispatches); err != nil {
		t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
	}

	tooMany := append(dispatches, DispatchSummary{ID: "disp-4"})
	if err := ValidateDispatchListMatchesSessionProgress(session, tooMany); err == nil {
		t.Fatal("error = nil, want dispatch count mismatch")
	}
}

func TestValidateResultMatchesEventProjection(t *testing.T) {
	t.Parallel()
	events := []json.RawMessage{
		json.RawMessage(`{"type":"SESSION_RESULT_UPDATED","payload":{"resultStatus":"PARTIAL"}}`),
		json.RawMessage(`{"type":"SESSION_RESULT_UPDATED","payload":{"resultStatus":"FINAL"}}`),
	}
	result := ResultReadResult{
		SessionID:    "dur-sess-001",
		ResultStatus: ResultStatusFinal,
	}
	if err := ValidateResultMatchesEventProjection(result, events); err != nil {
		t.Fatalf("ValidateResultMatchesEventProjection: %v", err)
	}

	mismatch := result
	mismatch.ResultStatus = ResultStatusPartial
	if err := ValidateResultMatchesEventProjection(mismatch, events); err == nil {
		t.Fatal("error = nil, want event mismatch")
	}
}

func TestProjectionServiceMethods_PropagateContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var service interface {
		GetResult(context.Context, string, ResultRequest) (ResultReadResult, error)
	}
	service = stubProjectionCancelAwareService{}
	if _, err := service.GetResult(ctx, "dur-sess-001", ResultRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetResult error = %v, want context.Canceled", err)
	}
}

type stubProjectionCancelAwareService struct{}

func (stubProjectionCancelAwareService) GetResult(ctx context.Context, _ string, _ ResultRequest) (ResultReadResult, error) {
	if err := ctx.Err(); err != nil {
		return ResultReadResult{}, err
	}
	return ResultReadResult{}, nil
}

func TestFakeService_DetailReadersAndRemainingControlWrappers(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)

	startAsyncByRequestID(t, service, "req-petri-success-001")
	dispatch, err := service.GetDispatch(context.Background(), "dur-sess-petri-success-001", "disp-petri-success-001")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.DispatchSummary.ID != "disp-petri-success-001" {
		t.Fatalf("dispatch detail = %#v", dispatch)
	}

	artifact, err := service.GetArtifact(context.Background(), "dur-sess-petri-success-001", "art-petri-final-001")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if artifact.ArtifactSummary.ID != "art-petri-final-001" {
		t.Fatalf("artifact detail = %#v", artifact)
	}

	startAsyncByRequestID(t, service, "req-js-run-n-001")
	cancelled, err := service.Cancel(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != LifecycleStatusCanceling {
		t.Fatalf("cancel status = %q, want CANCELING", cancelled.Status)
	}

	terminated, err := service.Terminate(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if terminated.Status != LifecycleStatusTerminated {
		t.Fatalf("terminate status = %q, want TERMINATED", terminated.Status)
	}

	startAsyncByRequestID(t, service, "req-js-awaiting-001")
	approved, err := service.Approve(context.Background(), "dur-sess-js-awaiting-001", ApproveRequest{})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Status != LifecycleStatusRunning {
		t.Fatalf("approve status = %q, want RUNNING", approved.Status)
	}
}

func TestMaterializeEventReadStream_OwnsFiniteClosedLifecycleAndDetachedHistory(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"id":"event-1","type":"factory.session.started","sequence":1,"payload":{"value":"original"}}`)
	stream := MaterializeEventReadStream(EventReadResult{Events: []json.RawMessage{
		raw,
		json.RawMessage(`{not-json}`),
	}})
	if stream == nil || len(stream.History) != 1 || stream.History[0].Id != "event-1" {
		t.Fatalf("stream = %#v, want one canonical history event", stream)
	}
	if _, open := <-stream.Events; open {
		t.Fatal("finite durable event stream live channel is open")
	}
	raw[len(raw)-3] = 'X'
	if got := string(stream.History[0].Payload); got != `{"value":"original"}` {
		t.Fatalf("detached payload = %q, want original payload", got)
	}
}

func TestMaterializeEventReadStream_EmptyResultStillReturnsClosedStream(t *testing.T) {
	t.Parallel()
	stream := MaterializeEventReadStream(EventReadResult{})
	if stream == nil || len(stream.History) != 0 {
		t.Fatalf("stream = %#v, want empty materialized stream", stream)
	}
	if _, open := <-stream.Events; open {
		t.Fatal("empty durable event stream live channel is open")
	}
}

func TestDispatchReadPreservesLatestLifecycleCursorAndDefaultsUnconfirmed(t *testing.T) {
	t.Parallel()

	input := []DispatchSummary{{
		ID: "dispatch-1", Status: DispatchStatusCompleted, DispatchKind: "JAVASCRIPT_AGENT",
		ConfirmationState: ConfirmationStateConfirmed,
	}}
	events := []json.RawMessage{
		json.RawMessage(`{"type":"DISPATCH_QUEUED","context":{"dispatchId":"dispatch-1","sequence":2}}`),
		json.RawMessage(`{"type":"DISPATCH_RECONCILED","context":{"dispatchId":"dispatch-1","sequence":7}}`),
		json.RawMessage(`{"type":"DISPATCH_INTERRUPTED","context":{"dispatchId":"other","sequence":9}}`),
	}

	result := dispatchesForRead(input, events)
	if len(result) != 1 {
		t.Fatalf("dispatchesForRead() = %#v, want one dispatch", result)
	}
	if result[0].ConfirmationState != ConfirmationStateUnconfirmed {
		t.Fatalf("confirmation state = %q, want UNCONFIRMED", result[0].ConfirmationState)
	}
	if !result[0].StateSequenceKnown || result[0].StateSequence != 7 {
		t.Fatalf("state cursor = %#v, want known sequence 7", result[0])
	}
	if input[0].ConfirmationState != ConfirmationStateConfirmed || input[0].StateSequenceKnown {
		t.Fatalf("input dispatch mutated = %#v", input[0])
	}
}

func TestLiveDispatchListAndDetailConfirmAfterCompletedFlush(t *testing.T) {
	const (
		sessionID    = "dur-sess-dispatch-watermark"
		generationID = "generation-dispatch-watermark"
		dispatchID   = "dispatch-watermark"
	)
	reader := &dispatchDurabilityReader{}
	service := &JavaScriptRuntimeService{
		sessions: map[string]*runtimeSessionState{
			sessionID: {
				session:    SessionReadResult{SessionID: sessionID, OrchestratorKind: "JAVASCRIPT"},
				dispatches: []DispatchSummary{{ID: dispatchID, Status: DispatchStatusCompleted, DispatchKind: "AGENT"}},
				events: []json.RawMessage{
					json.RawMessage(`{"type":"DISPATCH_RECONCILED","context":{"dispatchId":"dispatch-watermark","sequence":7}}`),
				},
			},
		},
	}
	service.SetDispatchDurability(reader, generationID)

	list, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches before flush: %v", err)
	}
	if len(list.Dispatches) != 1 || list.Dispatches[0].ConfirmationState != ConfirmationStateUnconfirmed {
		t.Fatalf("ListDispatches before flush = %#v, want UNCONFIRMED", list.Dispatches)
	}
	if reader.calls != 1 {
		t.Fatalf("ListDispatches watermark calls = %d, want one sample", reader.calls)
	}

	detail, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch before flush: %v", err)
	}
	if detail.ConfirmationState != ConfirmationStateUnconfirmed {
		t.Fatalf("GetDispatch before flush = %#v, want UNCONFIRMED", detail)
	}

	reader.available = true
	reader.cursor = recordings.CanonicalEventCursor{StreamGenerationID: generationID, Sequence: 7}
	list, err = service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches after flush: %v", err)
	}
	if list.Dispatches[0].ConfirmationState != ConfirmationStateConfirmed {
		t.Fatalf("ListDispatches after flush = %#v, want CONFIRMED", list.Dispatches)
	}

	detail, err = service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch after flush: %v", err)
	}
	if detail.ConfirmationState != ConfirmationStateConfirmed {
		t.Fatalf("GetDispatch after flush = %#v, want CONFIRMED", detail)
	}
	if reader.calls != 4 {
		t.Fatalf("dispatch read watermark calls = %d, want one sample per response", reader.calls)
	}
}

type dispatchDurabilityReader struct {
	cursor    recordings.CanonicalEventCursor
	available bool
	calls     int
}

func TestLegacyTerminalSnapshot_MigratesOnNextSuccessfulSaveAndReload(t *testing.T) {
	const sessionID = "dur-sess-dddddddddddddddddddddddddddddddd"
	store, service := seedLegacyTerminalSnapshot(t, sessionID)
	read, err := service.GetSession(context.Background(), sessionID)
	if err != nil || read.Status != LifecycleStatusSucceeded {
		t.Fatalf("GetSession legacy snapshot = %#v, %v, want SUCCEEDED", read, err)
	}
	if err := service.RecordPetriSessionCompletion(sessionID, PetriSessionCompletion{Status: LifecycleStatusSucceeded}); err != nil {
		t.Fatalf("RecordPetriSessionCompletion migration: %v", err)
	}
	assertLegacyTerminalSnapshotCompacted(t, store, service, sessionID)
}

func seedLegacyTerminalSnapshot(t *testing.T, sessionID string) (*petriCompactionStore, *JavaScriptRuntimeService) {
	t.Helper()
	store := &petriCompactionStore{}
	legacy := runtimeSessionState{session: SessionReadResult{SessionID: sessionID, Status: LifecycleStatusSucceeded, OrchestratorKind: interfaces.OrchestratorKindPetri}, result: ResultReadResult{SessionID: sessionID, SessionStatus: LifecycleStatusSucceeded}, petriMutations: legacyTerminalTokenMutations(7)}
	encoded, err := json.Marshal(persistedSnapshotFromRuntimeStateWithFailureLogCapacity(legacy, 0))
	if err != nil {
		t.Fatalf("marshal legacy snapshot: %v", err)
	}
	if err := store.Save(sessionID, encoded); err != nil {
		t.Fatalf("seed legacy snapshot: %v", err)
	}
	return store, &JavaScriptRuntimeService{persistence: store, sessions: make(map[string]*runtimeSessionState)}
}

func assertLegacyTerminalSnapshotCompacted(t *testing.T, store *petriCompactionStore, service *JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	saved, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("load migrated snapshot: %v", err)
	}
	var migrated PersistedRuntimeSessionState
	if err := json.Unmarshal(saved, &migrated); err != nil {
		t.Fatalf("decode migrated snapshot: %v", err)
	}
	if len(migrated.Records) != 1 || migrated.Records[0].PetriSummary == nil || migrated.Records[0].PetriSummary.WorkID != "work-7" || migrated.Records[0].PetriSummary.State != "done" {
		t.Fatalf("migrated records = %#v, want one work-7 summary", migrated.Records)
	}
	if strings.Contains(string(saved), "large-worker-output") || strings.Contains(string(saved), "structured_result") {
		t.Fatal("migrated snapshot retained legacy worker output")
	}
	loaded, err := service.snapshotSessionState(sessionID)
	if err != nil || len(loaded.petriMutations) != 0 || len(loaded.petriSummaries) != 1 {
		t.Fatalf("hot migrated history = %d mutations, %d summaries, %v", len(loaded.petriMutations), len(loaded.petriSummaries), err)
	}
}

func TestJavaScriptRuntimeService_SnapshotSizeLimitHonorsExactBoundAndRejectsBeforeSave(t *testing.T) {
	root := t.TempDir()
	store := &petriCompactionStore{}
	service := &JavaScriptRuntimeService{persistence: store, projectRoot: root}
	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	state := runtimeSessionState{session: SessionReadResult{SessionID: sessionID, Status: LifecycleStatusSucceeded}}
	exact, err := json.MarshalIndent(persistedSnapshotFromRuntimeStateWithFailureLogCapacity(state, defaultPersistedTokenFailureLogCapacity), "", "  ")
	if err != nil {
		t.Fatalf("marshal exact snapshot: %v", err)
	}
	service.persistedSnapshotMaxBytes = len(exact)
	if err := service.persistSessionSnapshot(state); err != nil || store.saveCalls != 1 || string(store.payload) != string(exact) {
		t.Fatalf("exact-bound persistence = %v, calls=%d bytes=%d, want one exact save", err, store.saveCalls, len(store.payload))
	}
	previous := []byte(`{"status":"RUNNING","sequence":7}`)
	store.saveCalls, store.payload = 0, append([]byte(nil), previous...)
	service.persistedSnapshotMaxBytes = len(exact) - 1
	err = service.persistSessionSnapshot(state)
	var sizeErr *SnapshotSizeLimitError
	if !errors.As(err, &sizeErr) || sizeErr.Path != runtimepersist.SnapshotPathForProjectRoot(root, sessionID) || sizeErr.ActualBytes != len(exact) || sizeErr.MaxBytes != len(exact)-1 {
		t.Fatalf("one-byte-over diagnostic = %v / %#v, want exact size rejection", err, sizeErr)
	}
	if strings.Contains(err.Error(), "RUNNING") || strings.Contains(err.Error(), "sequence") || store.saveCalls != 0 || string(store.payload) != string(previous) {
		t.Fatalf("one-byte-over persistence changed prior snapshot or leaked content")
	}
}

func (reader *dispatchDurabilityReader) CompletedFlushWatermark(
	string,
) (recordings.CanonicalEventCursor, bool) {
	reader.calls++
	return reader.cursor, reader.available
}

func TestRecordPetriTokenMutations_CompactsCandidateAndPublishesOnlyAfterSave(t *testing.T) {
	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	store := &petriCompactionStore{}
	service := &JavaScriptRuntimeService{persistence: store, sessions: map[string]*runtimeSessionState{
		sessionID: {session: SessionReadResult{SessionID: sessionID, Status: LifecycleStatusRunning}},
	}}
	if err := service.RecordPetriTokenMutations(sessionID, largeTerminalTokenMutations(3, "large-output")); err != nil {
		t.Fatalf("RecordPetriTokenMutations: %v", err)
	}
	if store.saveCalls != 1 || len(service.sessions[sessionID].petriMutations) != 0 || len(service.sessions[sessionID].petriSummaries) != 1 {
		t.Fatalf("saved candidate = calls %d, %d mutations, %d summaries, want 1, 0, 1", store.saveCalls, len(service.sessions[sessionID].petriMutations), len(service.sessions[sessionID].petriSummaries))
	}
	var persisted PersistedRuntimeSessionState
	if err := json.Unmarshal(store.payload, &persisted); err != nil {
		t.Fatalf("decode persisted compacted state: %v", err)
	}
	if len(persisted.Records) != 1 || persisted.Records[0].PetriSummary == nil || persisted.Records[0].PetriSummary.WorkID != "work-3" {
		t.Fatalf("persisted records = %#v, want work-3 summary", persisted.Records)
	}

	const failedID = "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	wantErr := errors.New("checkpoint unavailable")
	initial := largeTerminalTokenMutations(4, "large-output")
	failedStore := &petriCompactionStore{saveErr: wantErr}
	failedService := &JavaScriptRuntimeService{persistence: failedStore, sessions: map[string]*runtimeSessionState{
		failedID: {session: SessionReadResult{SessionID: failedID, Status: LifecycleStatusRunning}, petriMutations: clonePetriMutations(initial)},
	}}
	if err := failedService.RecordPetriTokenMutations(failedID, []interfaces.TokenMutationRecord{initial[2]}); !errors.Is(err, wantErr) {
		t.Fatalf("failed RecordPetriTokenMutations error = %v, want %v", err, wantErr)
	}
	if len(failedService.sessions[failedID].petriMutations) != len(initial) || len(failedService.sessions[failedID].petriSummaries) != 0 {
		t.Fatalf("state after failed save = %d mutations, %d summaries, want original %d and 0", len(failedService.sessions[failedID].petriMutations), len(failedService.sessions[failedID].petriSummaries), len(initial))
	}
}
