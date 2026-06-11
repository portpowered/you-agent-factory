package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func contractFixturesPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "api", "testdata", "durable-session-contract-fixtures.json")
}

func newContractFakeService(t *testing.T) *FakeService {
	t.Helper()
	service, err := NewFakeServiceFromContractFixtures(contractFixturesPath(t))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func startAsyncByRequestID(t *testing.T, service *FakeService, requestID string) AsyncStartResult {
	t.Helper()
	result, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: requestID,
		Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartAsync(%q): %v", requestID, err)
	}
	return result
}

func TestFakeService_StartAsync_ProjectsFixtureScenarios(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		requestID     string
		sessionID     string
		status        LifecycleStatus
		result        ResultStatus
		resultRequest ResultRequest
	}{
		{"req-petri-run-001", "dur-sess-petri-run-001", LifecycleStatusRunning, ResultStatusNotReady, ResultRequest{Mode: ResultModeFinal}},
		{"req-js-run-n-001", "dur-sess-js-run-n-001", LifecycleStatusRunning, ResultStatusPartial, ResultRequest{Mode: ResultModePartial}},
		{"req-js-awaiting-001", "dur-sess-js-awaiting-001", LifecycleStatusAwaitingApproval, ResultStatusNotReady, ResultRequest{Mode: ResultModeFinal}},
		{"req-petri-success-001", "dur-sess-petri-success-001", LifecycleStatusSucceeded, ResultStatusFinal, ResultRequest{Mode: ResultModeFinal}},
		{"req-js-failed-partial-001", "dur-sess-js-failed-partial-001", LifecycleStatusFailed, ResultStatusFailedWithPartial, ResultRequest{Mode: ResultModePartial}},
		{"req-petri-cancel-001", "dur-sess-petri-cancel-001", LifecycleStatusCanceled, ResultStatusUnavailable, ResultRequest{Mode: ResultModeFinal}},
		{"req-js-timeout-001", "dur-sess-js-timeout-001", LifecycleStatusRunning, ResultStatusNotReady, ResultRequest{Mode: ResultModeFinal}},
		{"req-js-interrupted-001", "dur-sess-js-interrupted-001", LifecycleStatusInterrupted, ResultStatusPartial, ResultRequest{Mode: ResultModePartial}},
	}
	for _, tc := range cases {
		t.Run(tc.requestID, func(t *testing.T) {
			started, err := service.StartAsync(context.Background(), StartRequest{
				RequestID: tc.requestID,
				Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
			})
			if err != nil {
				t.Fatalf("StartAsync: %v", err)
			}
			if started.SessionID != tc.sessionID {
				t.Fatalf("sessionId = %q, want %q", started.SessionID, tc.sessionID)
			}
			read, err := service.GetSession(context.Background(), tc.sessionID)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if read.Status != tc.status {
				t.Fatalf("status = %q, want %q", read.Status, tc.status)
			}
			result, err := service.GetResult(context.Background(), tc.sessionID, tc.resultRequest)
			if err != nil {
				t.Fatalf("GetResult: %v", err)
			}
			if result.ResultStatus != tc.result {
				t.Fatalf("resultStatus = %q, want %q", result.ResultStatus, tc.result)
			}
		})
	}
}

func TestFakeService_StartAsync_UnknownRequestIDReturnsValidationError(t *testing.T) {
	service := newContractFakeService(t)
	_, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: "req-unknown-scenario",
		Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err == nil {
		t.Fatal("error = nil, want validation error for unknown scenario")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "requestId" {
		t.Fatalf("error = %v, want requestId validation error", err)
	}
	if !strings.Contains(validationErr.Message, "req-unknown-scenario") {
		t.Fatalf("message = %q, want actionable unknown scenario detail", validationErr.Message)
	}
}

func TestFakeService_StartAsync_RepeatedStartReturnsStableOutcome(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		requestID string
		sessionID string
	}{
		{"req-petri-success-001", "dur-sess-petri-success-001"},
		{"req-petri-run-001", "dur-sess-petri-run-001"},
		{"req-js-failed-partial-001", "dur-sess-js-failed-partial-001"},
		{"req-js-interrupted-001", "dur-sess-js-interrupted-001"},
	}
	for _, tc := range cases {
		t.Run(tc.requestID, func(t *testing.T) {
			req := StartRequest{
				RequestID: tc.requestID,
				Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
			}
			first, err := service.StartAsync(context.Background(), req)
			if err != nil {
				t.Fatalf("first StartAsync: %v", err)
			}
			second, err := service.StartAsync(context.Background(), req)
			if err != nil {
				t.Fatalf("second StartAsync: %v", err)
			}
			if second.SessionID != first.SessionID || second.SessionID != tc.sessionID {
				t.Fatalf("sessionId = %q, want stable %q", second.SessionID, tc.sessionID)
			}
			if second.Status != first.Status {
				t.Fatalf("status = %q, want stable %q", second.Status, first.Status)
			}
			if second.Links != first.Links {
				t.Fatalf("links changed on replay: first=%#v second=%#v", first.Links, second.Links)
			}
			if second.Links.Session == "" || second.Links.Results == "" {
				t.Fatalf("inspection links missing session/results: %#v", second.Links)
			}

			firstRead, err := service.GetSession(context.Background(), tc.sessionID)
			if err != nil {
				t.Fatalf("first GetSession: %v", err)
			}
			secondRead, err := service.GetSession(context.Background(), tc.sessionID)
			if err != nil {
				t.Fatalf("second GetSession: %v", err)
			}
			if secondRead.Status != firstRead.Status || secondRead.Phase != firstRead.Phase {
				t.Fatalf("session read changed on replay: first=%#v second=%#v", firstRead, secondRead)
			}
			if secondRead.Links != firstRead.Links {
				t.Fatalf("session read links changed on replay: first=%#v second=%#v", firstRead.Links, secondRead.Links)
			}
		})
	}
}

func TestFakeService_StartAsync_IdempotentReplay(t *testing.T) {
	service := newContractFakeService(t)
	req := StartRequest{
		RequestID: "req-idempotent-replay-001",
		Source: Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/idempotent.yaml",
		},
		Args: map[string]any{"task": "replay"},
		RequestedPolicy: map[string]any{
			"policyHash": "req-policy-idempotent",
		},
	}
	first, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartAsync: %v", err)
	}
	second, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("second StartAsync: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("replay sessionId = %q, want %q", second.SessionID, first.SessionID)
	}
	conflict := req
	conflict.Args["task"] = "different"
	_, err = service.StartAsync(context.Background(), conflict)
	if !errors.Is(err, ErrExecutionRequestIDConflict) {
		t.Fatalf("error = %v, want ErrExecutionRequestIDConflict", err)
	}
}

func TestFakeService_StartSync_TerminalAndTimeoutFixtures(t *testing.T) {
	service := newContractFakeService(t)

	terminal, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-petri-success-001",
		Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartSync terminal: %v", err)
	}
	if terminal.SyncOutcome != SyncOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", terminal.SyncOutcome)
	}
	if terminal.Status != string(LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", terminal.Status)
	}

	timedOut, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-js-timeout-001",
		Source:    Source{Kind: workflowsource.KindWorkflowName, WorkflowName: "long-running-audit"},
		Wait:      &WaitOptions{TimeoutMillis: int64Ptr(30000)},
	})
	if err != nil {
		t.Fatalf("StartSync timeout: %v", err)
	}
	if timedOut.SyncOutcome != SyncOutcomeTimedOut || !timedOut.TimedOut {
		t.Fatalf("timeout response = %#v", timedOut)
	}
}

func TestFakeService_LifecycleControl_IdempotentReplayAndConflict(t *testing.T) {
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

func TestFakeService_ReadProjections_MatchFixtureDispatchesArtifactsEvents(t *testing.T) {
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

func TestFakeService_ReadEvents_ReturnsCanonicalFixtureEventsAndHonorsCursor(t *testing.T) {
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

func TestFakeService_ListAndDetail_ScopedSummariesAndConsistency(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		requestID       string
		sessionID       string
		expectLive      bool
		expectPersisted bool
		expectTerminal  bool
		expectRecoverable bool
	}{
		{"req-petri-run-001", "dur-sess-petri-run-001", true, false, false, false},
		{"req-petri-success-001", "dur-sess-petri-success-001", true, true, true, false},
		{"req-js-failed-partial-001", "dur-sess-js-failed-partial-001", true, true, true, false},
		{"req-js-interrupted-001", "dur-sess-js-interrupted-001", true, true, true, true},
	}
	for _, tc := range cases {
		startAsyncByRequestID(t, service, tc.requestID)
	}

	for _, scope := range []SessionListScope{
		SessionListScopeLive,
		SessionListScopePersisted,
		SessionListScopeAll,
	} {
		first, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: scope})
		if err != nil {
			t.Fatalf("ListSessions(%s) first: %v", scope, err)
		}
		second, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: scope})
		if err != nil {
			t.Fatalf("ListSessions(%s) second: %v", scope, err)
		}
		if len(first.LiveSessions) != len(second.LiveSessions) || len(first.DurableSessions) != len(second.DurableSessions) {
			t.Fatalf("scope %s listing changed between reads: first=%#v second=%#v", scope, first, second)
		}
	}

	live, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeLive})
	if err != nil {
		t.Fatalf("ListSessions live: %v", err)
	}
	persisted, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopePersisted})
	if err != nil {
		t.Fatalf("ListSessions persisted: %v", err)
	}
	all, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeAll})
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}

	liveIDs := liveSessionIDs(live.LiveSessions)
	allLiveIDs := liveSessionIDs(all.LiveSessions)

	for _, tc := range cases {
		t.Run(tc.sessionID, func(t *testing.T) {
			detail, err := service.GetSession(context.Background(), tc.sessionID)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if tc.expectTerminal != IsTerminalLifecycleStatus(detail.Status) {
				t.Fatalf("terminal = %t, want %t for status %q", IsTerminalLifecycleStatus(detail.Status), tc.expectTerminal, detail.Status)
			}
			if detail.Progress == nil || detail.Progress.TotalDispatches == 0 {
				t.Fatalf("detail progress missing dispatch count: %#v", detail.Progress)
			}
			dispatches, err := service.ListDispatches(context.Background(), tc.sessionID)
			if err != nil {
				t.Fatalf("ListDispatches: %v", err)
			}
			if err := ValidateDispatchListMatchesSessionProgress(detail, dispatches.Dispatches); err != nil {
				t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
			}

			inLive := containsString(liveIDs, tc.sessionID)
			inPersisted := containsPersistedSummary(persisted.DurableSessions, tc.sessionID)
			if inLive != tc.expectLive {
				t.Fatalf("live scope membership = %t, want %t", inLive, tc.expectLive)
			}
			if inPersisted != tc.expectPersisted {
				t.Fatalf("persisted scope membership = %t, want %t", inPersisted, tc.expectPersisted)
			}

			var listSummary *DurableSessionListSummary
			if inPersisted {
				for index := range persisted.DurableSessions {
					if persisted.DurableSessions[index].SessionID == tc.sessionID {
						listSummary = &persisted.DurableSessions[index]
						break
					}
				}
			}
			if listSummary != nil {
				if listSummary.Recoverable != tc.expectRecoverable {
					t.Fatalf("recoverable = %t, want %t", listSummary.Recoverable, tc.expectRecoverable)
				}
				if err := ValidateSessionDetailMatchesListSummary(detail, *listSummary); err != nil {
					t.Fatalf("ValidateSessionDetailMatchesListSummary: %v", err)
				}
			}

			inAllLive := containsString(allLiveIDs, tc.sessionID)
			inAllDurable := containsPersistedSummary(all.DurableSessions, tc.sessionID)
			if tc.expectTerminal {
				if inAllLive {
					t.Fatalf("scope=all still contains terminal session %q in live rows", tc.sessionID)
				}
				if !inAllDurable {
					t.Fatal("scope=all missing terminal durable row")
				}
			} else if !inAllLive || inAllDurable {
				t.Fatalf("scope=all live=%t durable=%t, want live-only for running session", inAllLive, inAllDurable)
			}
		})
	}

	for index := 1; index < len(persisted.DurableSessions); index++ {
		if strings.Compare(persisted.DurableSessions[index-1].SessionID, persisted.DurableSessions[index].SessionID) >= 0 {
			t.Fatalf("persisted ordering not stable: %#v", persisted.DurableSessions)
		}
	}
}

func TestFakeService_GetSession_NotFoundDoesNotMutateState(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-run-001")

	before, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeAll})
	if err != nil {
		t.Fatalf("ListSessions before: %v", err)
	}

	_, err = service.GetSession(context.Background(), "dur-sess-missing-001")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("GetSession error = %v, want ErrSessionNotFound", err)
	}

	after, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeAll})
	if err != nil {
		t.Fatalf("ListSessions after: %v", err)
	}
	if len(before.LiveSessions) != len(after.LiveSessions) || len(before.DurableSessions) != len(after.DurableSessions) {
		t.Fatalf("listing changed after not-found read: before=%#v after=%#v", before, after)
	}
	detail, err := service.GetSession(context.Background(), "dur-sess-petri-run-001")
	if err != nil {
		t.Fatalf("GetSession existing: %v", err)
	}
	if detail.Status != LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", detail.Status)
	}
}

func liveSessionIDs(sessions []LiveSessionSummary) []string {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	return ids
}

func containsPersistedSummary(summaries []DurableSessionListSummary, sessionID string) bool {
	for _, summary := range summaries {
		if summary.SessionID == sessionID {
			return true
		}
	}
	return false
}

func TestFakeService_ListSessions_ScopedPersistedAndLive(t *testing.T) {
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

func TestFakeService_StartAsync_ConcurrentIdempotentStarts(t *testing.T) {
	service := newContractFakeService(t)
	const workers = 12
	var wg sync.WaitGroup
	results := make([]AsyncStartResult, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = service.StartAsync(context.Background(), StartRequest{
				RequestID: "req-petri-run-001",
				Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
			})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("StartAsync worker %d: %v", i, err)
		}
	}
	for i := 1; i < workers; i++ {
		if results[i].SessionID != results[0].SessionID {
			t.Fatalf("sessionId[%d] = %q, want %q", i, results[i].SessionID, results[0].SessionID)
		}
	}

	sessionID := results[0].SessionID
	all, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeAll})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	liveCount := 0
	for _, session := range all.LiveSessions {
		if session.ID == sessionID {
			liveCount++
		}
	}
	if liveCount != 1 {
		t.Fatalf("live session rows for %q = %d, want 1", sessionID, liveCount)
	}

	dispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].ID != "disp-petri-001" {
		t.Fatalf("dispatches = %#v, want one stable dispatch", dispatches.Dispatches)
	}

	artifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want none for running scenario", artifacts.Artifacts)
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) != 2 {
		t.Fatalf("events = %d, want 2 for running scenario", len(events.Events))
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
func TestProjectResultRead_ModePartialAndFinal(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")

	partial, err := service.GetResult(context.Background(), "dur-sess-js-run-n-001", ResultRequest{Mode: ResultModePartial})
	if err != nil {
		t.Fatalf("GetResult partial: %v", err)
	}
	if partial.ResultStatus != ResultStatusPartial {
		t.Fatalf("partial status = %q, want PARTIAL", partial.ResultStatus)
	}
	if len(partial.PrimaryResult) == 0 {
		t.Fatal("partial primaryResult missing")
	}
	if partial.Mode != ResultModePartial {
		t.Fatalf("mode = %q, want partial", partial.Mode)
	}

	final, err := service.GetResult(context.Background(), "dur-sess-js-run-n-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult final: %v", err)
	}
	if final.ResultStatus != ResultStatusNotReady {
		t.Fatalf("final status = %q, want NOT_READY", final.ResultStatus)
	}
	if len(final.PrimaryResult) != 0 {
		t.Fatal("final primaryResult should be omitted for running session")
	}
	if final.Availability == nil || final.Availability.Reason != "RESULT_NOT_READY" {
		t.Fatalf("availability = %#v, want RESULT_NOT_READY", final.Availability)
	}
}

func TestProjectResultRead_TerminalFinalAndUnavailable(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	final, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult terminal final: %v", err)
	}
	if final.ResultStatus != ResultStatusFinal {
		t.Fatalf("status = %q, want FINAL", final.ResultStatus)
	}
	if len(final.PrimaryResult) == 0 {
		t.Fatal("final primaryResult missing")
	}

	startAsyncByRequestID(t, service, "req-petri-cancel-001")
	unavailable, err := service.GetResult(context.Background(), "dur-sess-petri-cancel-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult unavailable: %v", err)
	}
	if unavailable.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("status = %q, want UNAVAILABLE", unavailable.ResultStatus)
	}
	if unavailable.Availability == nil || unavailable.Availability.Reason != "SESSION_CANCELED" {
		t.Fatalf("availability = %#v", unavailable.Availability)
	}
}

func TestProjectResultRead_FailedWithPartialHonorsPartialMode(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")

	result, err := service.GetResult(context.Background(), "dur-sess-js-failed-partial-001", ResultRequest{Mode: ResultModePartial})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != ResultStatusFailedWithPartial {
		t.Fatalf("status = %q, want FAILED_WITH_PARTIAL", result.ResultStatus)
	}
	if len(result.PrimaryResult) == 0 {
		t.Fatal("partial primaryResult missing")
	}
	if result.Failure == nil || !result.Failure.PartialResultAvailable {
		t.Fatal("failure detail missing")
	}
}

func TestProjectResultRead_IncludeArtifactsShaping(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	excluded, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{
		Mode:             ResultModeFinal,
		IncludeArtifacts: false,
	})
	if err != nil {
		t.Fatalf("GetResult excluded: %v", err)
	}
	if excluded.IncludeArtifacts {
		t.Fatal("includeArtifacts = true, want false")
	}
	if len(excluded.ArtifactRefs) != 0 {
		t.Fatalf("artifactRefs = %#v, want omitted", excluded.ArtifactRefs)
	}
	if len(excluded.ArtifactIDs) != 1 || excluded.ArtifactIDs[0] != "art-petri-final-001" {
		t.Fatalf("artifactIds = %#v", excluded.ArtifactIDs)
	}

	included, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{
		Mode:             ResultModeFinal,
		IncludeArtifacts: true,
	})
	if err != nil {
		t.Fatalf("GetResult included: %v", err)
	}
	if !included.IncludeArtifacts {
		t.Fatal("includeArtifacts = false, want true")
	}
	if len(included.ArtifactRefs) != 1 || included.ArtifactRefs[0].ID != "art-petri-final-001" {
		t.Fatalf("artifactRefs = %#v", included.ArtifactRefs)
	}
	if len(included.ArtifactIDs) != 0 {
		t.Fatalf("artifactIds = %#v, want omitted when refs included", included.ArtifactIDs)
	}
}

func TestProjectResultRead_NotReadyRunningSession(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-run-001")

	result, err := service.GetResult(context.Background(), "dur-sess-petri-run-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != ResultStatusNotReady {
		t.Fatalf("status = %q, want NOT_READY", result.ResultStatus)
	}
	if result.Availability == nil || result.Availability.Message == "" {
		t.Fatal("availability missing")
	}
}

func TestProjectResultRead_DefaultsToFinalMode(t *testing.T) {
	canonical := ResultReadResult{
		SessionID:     "dur-sess-001",
		ResultStatus:  ResultStatusFinal,
		SessionStatus: LifecycleStatusSucceeded,
		PrimaryResult: json.RawMessage(`[{"type":"text","text":"done"}]`),
	}
	session := SessionReadResult{
		SessionID: "dur-sess-001",
		Status:    LifecycleStatusSucceeded,
		ResultSummary: &ResultSummary{
			ResultStatus: string(ResultStatusFinal),
		},
	}

	projected, err := ProjectResultRead(canonical, session, nil, ResultRequest{})
	if err != nil {
		t.Fatalf("ProjectResultRead: %v", err)
	}
	if projected.Mode != ResultModeFinal {
		t.Fatalf("mode = %q, want final", projected.Mode)
	}
	if projected.ResultStatus != ResultStatusFinal {
		t.Fatalf("status = %q, want FINAL", projected.ResultStatus)
	}
}
