// backendsizecheck:ignore-file consolidated JavaScript runtime execution tests remain together until dedicated execution test seams split.
// pkgmaintcheck:ignore-file-lines consolidated JavaScript runtime execution tests remain together until dedicated execution test seams split.
package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestIsTerminalLifecycleStatus(t *testing.T) {
	t.Parallel()
	terminal := []LifecycleStatus{
		LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated,
	}
	for _, status := range terminal {
		if !IsTerminalLifecycleStatus(status) {
			t.Fatalf("status %q should be terminal", status)
		}
		if status != LifecycleStatusFailed && AllowsRetryDispatchOnTerminal(status) {
			t.Fatalf("retry-dispatch should be rejected on terminal status %q", status)
		}
	}
	if !AllowsRetryDispatchOnTerminal(LifecycleStatusFailed) {
		t.Fatal("retry-dispatch should remain allowed on FAILED terminal sessions")
	}
	active := []LifecycleStatus{
		LifecycleStatusRunning,
		LifecycleStatusPaused,
		LifecycleStatusCanceling,
	}
	for _, status := range active {
		if IsTerminalLifecycleStatus(status) {
			t.Fatalf("status %q should be active", status)
		}
	}
}

func TestEvaluateLifecycleControl_ValidTransitions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		operation LifecycleControlKind
		status    LifecycleStatus
		want      LifecycleControlOutcome
	}{
		{LifecycleControlPause, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlPause, LifecycleStatusPaused, LifecycleControlOutcomeNoOp},
		{LifecycleControlResume, LifecycleStatusPaused, LifecycleControlOutcomeAccepted},
		{LifecycleControlResume, LifecycleStatusInterrupted, LifecycleControlOutcomeAccepted},
		{LifecycleControlResume, LifecycleStatusRunning, LifecycleControlOutcomeNoOp},
		{LifecycleControlCancel, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlCancel, LifecycleStatusCanceling, LifecycleControlOutcomeNoOp},
		{LifecycleControlTerminate, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlApprove, LifecycleStatusAwaitingApproval, LifecycleControlOutcomeAccepted},
		{LifecycleControlRetryDispatch, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlRetryDispatch, LifecycleStatusFailed, LifecycleControlOutcomeAccepted},
	}
	for _, tc := range cases {
		got := EvaluateLifecycleControl(tc.operation, tc.status)
		if got != tc.want {
			t.Fatalf("%s on %s = %q, want %q", tc.operation, tc.status, got, tc.want)
		}
	}
}

func TestEvaluateLifecycleControl_InvalidAndTerminal(t *testing.T) {
	t.Parallel()
	if got := EvaluateLifecycleControl(LifecycleControlPause, LifecycleStatusAwaitingApproval); got != LifecycleControlOutcomeInvalidState {
		t.Fatalf("pause on awaiting approval = %q, want INVALID_STATE", got)
	}
	if got := EvaluateLifecycleControl(LifecycleControlRetryDispatch, LifecycleStatusSucceeded); got != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on succeeded = %q, want TERMINAL_SESSION", got)
	}
	if got := EvaluateLifecycleControl(LifecycleControlCancel, LifecycleStatusCanceled); got != LifecycleControlOutcomeNoOp {
		t.Fatalf("cancel on canceled = %q, want NO_OP", got)
	}
}

func TestNormalizeRetryDispatchRequest_RequiresDispatchID(t *testing.T) {
	t.Parallel()
	_, err := NormalizeRetryDispatchRequest(RetryDispatchRequest{})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want ValidationError", err)
	}
}

func TestControlIdempotencyTupleHash_IsStable(t *testing.T) {
	t.Parallel()
	retry := RetryDispatchRequest{
		ControlRequest: ControlRequest{RequestID: "req-retry-001"},
		DispatchID:     "disp-js-success-002",
	}
	first, err := ControlIdempotencyTupleHash(LifecycleControlRetryDispatch, "dur-sess-js-success-002", ApproveRequest{}, retry, InterruptDispatchRequest{})
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	second, err := ControlIdempotencyTupleHash(LifecycleControlRetryDispatch, "dur-sess-js-success-002", ApproveRequest{}, retry, InterruptDispatchRequest{})
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if first != second {
		t.Fatalf("hash mismatch: %q vs %q", first, second)
	}
}

func TestCheckControlRequestIDReplay_Conflict(t *testing.T) {
	t.Parallel()
	err := CheckControlRequestIDReplay("req-1", "sha256:abc", "sha256:def")
	if !errors.Is(err, ErrControlRequestIDConflict) {
		t.Fatalf("error = %v, want ErrControlRequestIDConflict", err)
	}
}

func TestJavaScriptRuntimeService_ControlWrappersAndDetailReaders(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	service.sessions["dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"] = newJavaScriptRuntimeRunningControlState(now)

	t.Run("detail readers and running controls", func(t *testing.T) {
		testJavaScriptRuntimeServiceRunningControlWrappers(t, service)
	})
	t.Run("approve awaiting session", func(t *testing.T) {
		testJavaScriptRuntimeServiceApproveAwaitingSession(t, service)
	})
	t.Run("retry failed dispatch", func(t *testing.T) {
		testJavaScriptRuntimeServiceRetryFailedDispatch(t, service)
	})
}

func newJavaScriptRuntimeRunningControlState(now time.Time) *runtimeSessionState {
	return &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Status:           LifecycleStatusRunning,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
			ResolvedSource: ResolvedSource{
				SourceRef: "inline",
			},
			Links: InspectionLinksForSession("dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true),
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SessionStatus: LifecycleStatusRunning,
			ResultStatus:  ResultStatusNotReady,
		},
		dispatches: []DispatchSummary{
			{ID: "disp-1", Status: DispatchStatusFailed, Attempt: 1},
		},
		dispatchStatusTransitions: map[string][]DispatchStatus{
			"disp-1": {DispatchStatusQueued, DispatchStatusFailed},
		},
		dispatchJavaScript: map[string]DispatchJavaScriptProjection{
			"disp-1": {TaskLabel: "child"},
		},
		artifacts: []ArtifactSummary{
			{ID: "art-1"},
		},
		events: BuildCanonicalRuntimeSessionEvents(
			SessionReadResult{
				SessionID:        "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Status:           LifecycleStatusRunning,
				OrchestratorKind: interfaces.OrchestratorKindJavaScript,
				Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
			},
			ResultReadResult{
				SessionID:     "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SessionStatus: LifecycleStatusRunning,
				ResultStatus:  ResultStatusNotReady,
				Availability: &ResultAvailabilityDetail{
					Reason:    "RESULT_NOT_READY",
					Message:   "Session is still running.",
					Retryable: true,
				},
			},
		),
	}
}

func testJavaScriptRuntimeServiceRunningControlWrappers(t *testing.T, service *JavaScriptRuntimeService) {
	t.Helper()

	if _, err := service.GetDispatch(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "disp-1"); err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if _, err := service.ListArtifacts(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if _, err := service.GetArtifact(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "art-1"); err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	listed, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeAll})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(listed.LiveSessions) != 1 {
		t.Fatalf("live sessions = %#v, want one session", listed.LiveSessions)
	}

	if _, err := service.Pause(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ControlRequest{}); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if _, err := service.Resume(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ControlRequest{}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if _, err := service.Terminate(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ControlRequest{}); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
}

func testJavaScriptRuntimeServiceApproveAwaitingSession(t *testing.T, service *JavaScriptRuntimeService) {
	t.Helper()

	service.sessions["dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] = &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Status:           LifecycleStatusAwaitingApproval,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Lifecycle:        &LifecycleTimestamps{},
			Links:            InspectionLinksForSession("dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", true),
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			SessionStatus: LifecycleStatusAwaitingApproval,
			ResultStatus:  ResultStatusNotReady,
		},
	}
	if _, err := service.Approve(context.Background(), "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ApproveRequest{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
}

func testJavaScriptRuntimeServiceRetryFailedDispatch(t *testing.T, service *JavaScriptRuntimeService) {
	t.Helper()

	service.sessions["dur-sess-cccccccccccccccccccccccccccccccc"] = &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-cccccccccccccccccccccccccccccccc",
			Status:           LifecycleStatusFailed,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Lifecycle:        &LifecycleTimestamps{},
			Links:            InspectionLinksForSession("dur-sess-cccccccccccccccccccccccccccccccc", true),
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-cccccccccccccccccccccccccccccccc",
			SessionStatus: LifecycleStatusFailed,
			ResultStatus:  ResultStatusUnavailable,
		},
		dispatches: []DispatchSummary{
			{ID: "disp-retry", Status: DispatchStatusFailed, Attempt: 2},
		},
	}
	if _, err := service.RetryDispatch(context.Background(), "dur-sess-cccccccccccccccccccccccccccccccc", RetryDispatchRequest{DispatchID: "disp-retry"}); err != nil {
		t.Fatalf("RetryDispatch: %v", err)
	}
}

func TestProjectedLifecycleControlStatus_PrefersCanonicalBracketStatus(t *testing.T) {
	t.Parallel()
	status := ProjectedLifecycleControlStatus("PAUSED", "RUNNING")
	if status != LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", status)
	}
}

func TestProjectedLifecycleControlStatus_FallsBackToFactoryRuntimeState(t *testing.T) {
	t.Parallel()
	if got := ProjectedLifecycleControlStatus("", "PAUSED"); got != LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", got)
	}
	if got := ProjectedLifecycleControlStatus("", "RUNNING"); got != LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", got)
	}
}

func TestFactoryStateToLifecycleStatus_MapsLiveFactoryStates(t *testing.T) {
	t.Parallel()

	for state, want := range map[string]LifecycleStatus{
		"IDLE": LifecycleStatusRunning, "RUNNING": LifecycleStatusRunning,
		"PAUSED": LifecycleStatusPaused, "COMPLETED": LifecycleStatusSucceeded,
		"FAILED": LifecycleStatusFailed,
	} {
		if got := LifecycleStatusFromFactoryRuntimeState(state); got != want {
			t.Fatalf("state %q = %q, want %q", state, got, want)
		}
	}
}

func TestLiveLifecycleControlResponse_BuildsTypedPauseOutcome(t *testing.T) {
	t.Parallel()

	result := LifecycleControlResult{
		SessionID: "~default", Operation: LifecycleControlPause,
		Outcome: LifecycleControlOutcomeAccepted, Status: LifecycleStatusPaused,
		Links: LiveLifecycleControlLinksForSession("~default"),
	}
	if result.SessionID != "~default" || result.Operation != LifecycleControlPause ||
		result.Outcome != LifecycleControlOutcomeAccepted || result.Status != LifecycleStatusPaused {
		t.Fatalf("result = %#v, want accepted live pause outcome", result)
	}
	if result.Links.Session != "/factory-sessions/~default" {
		t.Fatalf("links = %#v, want /factory-sessions/~default", result.Links)
	}
}

func TestJavaScriptRuntimeService_InterruptAcceptedBeforeChildCompletion_RecordsObservedCancellation(t *testing.T) {
	t.Parallel()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-race-runtime-001"
	dispatchID := "dispatch-1"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "summarize-findings"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	interruptResult, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "operator stop before provider completion"},
		DispatchID:     dispatchID,
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}
	if interruptResult.DispatchID != dispatchID {
		t.Fatalf("dispatchId = %q, want %q", interruptResult.DispatchID, dispatchID)
	}

	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Message != "operator stop before provider completion" {
		t.Fatalf("failureDetail = %#v, want operator stop before provider completion", dispatch.FailureDetail)
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, dispatchID)
	if payload.Reason != "operator stop before provider completion" {
		t.Fatalf("event reason = %q, want operator stop before provider completion", payload.Reason)
	}
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}

	replayed, err := ReplayDispatchProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplayDispatchProjection: %v", err)
	}
	if len(replayed) != 1 || replayed[0].Status != DispatchStatusInterrupted {
		t.Fatalf("replayed dispatches = %#v, want one interrupted dispatch", replayed)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime regression keeps late-result suppression assertions together on one scenario.
func TestJavaScriptRuntimeService_LateChildResultAfterInterrupt_SuppressesNormalRouting(t *testing.T) {
	t.Parallel()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-late-runtime-001"
	dispatchID := "dispatch-1"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "summarize-findings"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	_, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "operator stop"},
		DispatchID:     dispatchID,
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}

	lateRecords := []factory.JavaScriptRuntimeRecord{{
		Kind: factory.JavaScriptRecordKindChildDispatch,
		ChildDispatch: &factory.JavaScriptChildDispatchRecord{
			DispatchID:         dispatchID,
			Status:             factory.JavaScriptChildDispatchStatusCompleted,
			Label:              "summarize-findings",
			ArtifactRef:        factory.FormatArtifactURI(sessionID, "child-artifact-late"),
			ProviderSessionRef: "provider-session-late",
			Provider:           "mock",
		},
	}}
	if err := applyRuntimeTerminalOutcome(service, sessionID, factory.JavaScriptRuntimeOutcome{
		OK:      true,
		Records: lateRecords,
		Value:   factory.TypedValue{JSON: json.RawMessage(`{"label":"agent-run-fake-child"}`)},
	}); err != nil {
		t.Fatalf("ApplyRuntimeTerminalOutcomeForTests: %v", err)
	}

	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch after late completion: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED after late completion", dispatch.Status)
	}
	if len(dispatch.OutputArtifactIDs) != 0 {
		t.Fatalf("outputArtifactIds = %#v, want suppressed late child output", dispatch.OutputArtifactIDs)
	}
	if len(dispatch.ProviderSessionRefs) != 1 || dispatch.ProviderSessionRefs[0].ID != "provider-session-late" {
		t.Fatalf("providerSessionRefs = %#v, want late diagnostic preserved", dispatch.ProviderSessionRefs)
	}

	session, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession after late completion: %v", err)
	}
	if session.Status != LifecycleStatusInterrupted {
		t.Fatalf("session status = %q, want INTERRUPTED after late completion", session.Status)
	}
	if session.Progress != nil && session.Progress.CompletedDispatches != 0 {
		t.Fatalf("completedDispatches = %d, want 0 after suppression", session.Progress.CompletedDispatches)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(ResultStatusUnavailable) {
		t.Fatalf("resultSummary = %#v, want UNAVAILABLE after late completion suppression", session.ResultSummary)
	}

	result, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult after late completion: %v", err)
	}
	if result.SessionStatus != LifecycleStatusInterrupted || result.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("result = status %q session %q, want UNAVAILABLE/INTERRUPTED", result.ResultStatus, result.SessionStatus)
	}
	if result.Availability == nil || result.Availability.Reason != "SESSION_INTERRUPTED" {
		t.Fatalf("availability = %#v, want SESSION_INTERRUPTED", result.Availability)
	}

	artifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	for _, artifact := range artifacts.Artifacts {
		if artifact.DispatchID == dispatchID && artifact.Kind == "CHILD_RESULT" {
			t.Fatalf("artifact = %#v, want late child output suppressed", artifact)
		}
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if !containsEventType(events.Events, "DISPATCH_INTERRUPTED") {
		t.Fatal("DISPATCH_INTERRUPTED event missing after late completion merge")
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this fake-service race regression keeps interrupt and replay assertions together on one scenario.
func TestFakeService_InterruptAcceptedBeforeCompletion_ObservableDispatchAndEventOutcomes(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	started := startAsyncByRequestID(t, service, "req-js-run-n-001")

	dispatches, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches before interrupt: %v", err)
	}
	if len(dispatches.Dispatches) < 2 {
		t.Fatalf("dispatches = %#v, want at least two fixture dispatches", dispatches.Dispatches)
	}
	runningBefore := findDispatchByID(dispatches.Dispatches, "disp-js-002")
	if runningBefore == nil || runningBefore.Status != DispatchStatusRunning {
		t.Fatalf("dispatch disp-js-002 = %#v, want RUNNING before interrupt", runningBefore)
	}

	interruptResult, err := service.InterruptDispatch(context.Background(), started.SessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "stop before provider completion"},
		DispatchID:     "disp-js-002",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}

	dispatch, err := service.GetDispatch(context.Background(), started.SessionID, "disp-js-002")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}

	events, err := service.ReadEvents(context.Background(), started.SessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, "disp-js-002")
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}
	if payload.Reason != "stop before provider completion" {
		t.Fatalf("reason = %q, want stop before provider completion", payload.Reason)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	updated, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches after interrupt: %v", err)
	}
	interrupted := findDispatchByID(updated.Dispatches, "disp-js-002")
	if interrupted == nil || interrupted.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch disp-js-002 after interrupt = %#v, want INTERRUPTED", interrupted)
	}
	if err := ValidateDispatchListMatchesSessionProgress(session, updated.Dispatches); err != nil {
		t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
	}
}

func findDispatchInterruptedEventPayload(t *testing.T, events []json.RawMessage, dispatchID string) dispatchInterruptedEventPayload {
	t.Helper()
	for _, raw := range events {
		var envelope canonicalFactoryEvent
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if envelope.Type != "DISPATCH_INTERRUPTED" {
			continue
		}
		if envelope.Context.DispatchID == nil || *envelope.Context.DispatchID != dispatchID {
			continue
		}
		var payload dispatchInterruptedEventPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal DISPATCH_INTERRUPTED payload: %v", err)
		}
		return payload
	}
	t.Fatalf("DISPATCH_INTERRUPTED event for %s not found in %#v", dispatchID, events)
	return dispatchInterruptedEventPayload{}
}

func containsEventType(events []json.RawMessage, eventType string) bool {
	for _, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type == eventType {
			return true
		}
	}
	return false
}

func findDispatchByID(dispatches []DispatchSummary, dispatchID string) *DispatchSummary {
	for index := range dispatches {
		if dispatches[index].ID == dispatchID {
			return &dispatches[index]
		}
	}
	return nil
}

func TestJavaScriptRuntimeService_InterruptRunningDispatch_PreservesObservedCancellationAtRecordTime(t *testing.T) {
	t.Parallel()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-observed-status-001"
	now := time.Date(2026, 6, 20, 18, 0, 0, 0, time.UTC)
	service.sessions[sessionID] = &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Phase:            "execute",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
			Links:            InspectionLinksForSession(sessionID, true),
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusRunning,
			ResultStatus:  ResultStatusNotReady,
		},
		dispatches: []DispatchSummary{{
			ID:     "dispatch-1",
			Status: DispatchStatusRunning,
			Phase:  "execute",
			Label:  "summarize-findings",
		}},
		dispatchStatusTransitions: map[string][]DispatchStatus{
			"dispatch-1": {DispatchStatusQueued, DispatchStatusRunning},
		},
		events: BuildCanonicalRuntimeSessionEvents(
			SessionReadResult{SessionID: sessionID, Status: LifecycleStatusRunning, OrchestratorKind: interfaces.OrchestratorKindJavaScript},
			ResultReadResult{SessionID: sessionID, SessionStatus: LifecycleStatusRunning, ResultStatus: ResultStatusNotReady},
		),
	}

	_, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "cancellation observed while running"},
		DispatchID:     "dispatch-1",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, "dispatch-1")
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}
	if payload.Reason != "cancellation observed while running" {
		t.Fatalf("reason = %q, want cancellation observed while running", payload.Reason)
	}
}

func TestValidateCheckpointSummaryForResume_RejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	sessionID := "dur-sess-checkpoint-validation-001"
	valid := &factory.JavaScriptCheckpointSummary{
		Kind:           factory.JavaScriptCheckpointSummaryKind,
		SchemaVersion:  factory.JavaScriptCheckpointSummarySchemaVersion,
		CheckpointID:   "checkpoint-1",
		SessionID:      sessionID,
		ResumeStrategy: factory.JavaScriptResumeStrategy,
	}
	if err := validateCheckpointSummaryForResume(valid, sessionID); err != nil {
		t.Fatalf("validateCheckpointSummaryForResume(valid): %v", err)
	}

	cases := []struct {
		name    string
		summary *factory.JavaScriptCheckpointSummary
		field   string
	}{
		{
			name:    "missing checkpoint",
			summary: nil,
			field:   "checkpointSummary",
		},
		{
			name: "invalid kind",
			summary: &factory.JavaScriptCheckpointSummary{
				Kind:         "invalid-kind",
				CheckpointID: "checkpoint-1",
			},
			field: "checkpointSummary.kind",
		},
		{
			name: "unsupported schema version",
			summary: &factory.JavaScriptCheckpointSummary{
				SchemaVersion: 99,
				CheckpointID:  "checkpoint-1",
			},
			field: "checkpointSummary.schemaVersion",
		},
		{
			name: "missing checkpoint id",
			summary: &factory.JavaScriptCheckpointSummary{
				Kind: factory.JavaScriptCheckpointSummaryKind,
			},
			field: "checkpointSummary.checkpointId",
		},
		{
			name: "session id mismatch",
			summary: &factory.JavaScriptCheckpointSummary{
				CheckpointID:   "checkpoint-1",
				SessionID:      "dur-sess-other",
				ResumeStrategy: factory.JavaScriptResumeStrategy,
			},
			field: "checkpointSummary.sessionId",
		},
		{
			name: "checkpoint not approved for resume",
			summary: &factory.JavaScriptCheckpointSummary{
				CheckpointID: "checkpoint-1",
			},
			field: "checkpointSummary.resumeStrategy",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCheckpointSummaryForResume(tc.summary, sessionID)
			resumeErr, ok := err.(*ResumeError)
			if !ok {
				t.Fatalf("error = %T (%v), want *ResumeError", err, err)
			}
			if resumeErr.Outcome != ResumeOutcomeInvalidState && resumeErr.Outcome != ResumeOutcomeMissingCheckpoint {
				t.Fatalf("outcome = %q, want typed resume failure", resumeErr.Outcome)
			}
			if tc.field != "" && resumeErr.Field != tc.field {
				t.Fatalf("field = %q, want %q", resumeErr.Field, tc.field)
			}
		})
	}
}

func TestApplyRuntimeCheckpointPartialProjection_SurfacesPartialResultWhileRunning(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-checkpoint-partial-001"
	state := &runtimeSessionState{
		session: SessionReadResult{
			SessionID: sessionID,
			Status:    LifecycleStatusRunning,
		},
		artifacts: []ArtifactSummary{{ID: "artifact-checkpoint-1", Kind: "text"}},
	}
	checkpoint := &factory.JavaScriptCheckpointRecord{
		ID:    "checkpoint-1",
		Label: "after-step-one",
		State: map[string]any{"text": "checkpoint partial output"},
	}

	applyRuntimeCheckpointPartialProjection(state, checkpoint)

	if state.session.ResultSummary == nil || state.session.ResultSummary.ResultStatus != string(ResultStatusPartial) {
		t.Fatalf("result summary = %#v, want PARTIAL", state.session.ResultSummary)
	}
	if state.result.ResultStatus != ResultStatusPartial {
		t.Fatalf("result status = %q, want PARTIAL", state.result.ResultStatus)
	}
	if state.result.Mode != ResultModePartial {
		t.Fatalf("result mode = %q, want partial", state.result.Mode)
	}
	if state.result.SessionStatus != LifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING", state.result.SessionStatus)
	}
	if len(state.result.PrimaryResult) == 0 {
		t.Fatal("primary result missing")
	}
	if len(state.result.ArtifactIDs) != 1 || state.result.ArtifactIDs[0] != "artifact-checkpoint-1" {
		t.Fatalf("artifact IDs = %#v, want checkpoint artifact", state.result.ArtifactIDs)
	}
}

func TestApplyRuntimeCheckpointPartialProjection_NoopsForTerminalOrEmptyCheckpoint(t *testing.T) {
	t.Parallel()
	checkpoint := &factory.JavaScriptCheckpointRecord{
		ID:    "checkpoint-1",
		Label: "after-step-one",
		State: map[string]any{"text": "checkpoint partial output"},
	}
	running := &runtimeSessionState{
		session: SessionReadResult{SessionID: "dur-sess-checkpoint-partial-002", Status: LifecycleStatusRunning},
		result:  ResultReadResult{SessionID: "dur-sess-checkpoint-partial-002", ResultStatus: ResultStatusNotReady},
	}
	terminal := &runtimeSessionState{
		session: SessionReadResult{SessionID: "dur-sess-checkpoint-partial-003", Status: LifecycleStatusSucceeded},
		result:  ResultReadResult{SessionID: "dur-sess-checkpoint-partial-003", ResultStatus: ResultStatusFinal},
	}

	applyRuntimeCheckpointPartialProjection(nil, checkpoint)
	applyRuntimeCheckpointPartialProjection(running, nil)
	applyRuntimeCheckpointPartialProjection(terminal, checkpoint)
	applyRuntimeCheckpointPartialProjection(running, &factory.JavaScriptCheckpointRecord{ID: "checkpoint-empty"})

	if running.result.ResultStatus != ResultStatusNotReady {
		t.Fatalf("running result status = %q, want NOT_READY", running.result.ResultStatus)
	}
	if terminal.result.ResultStatus != ResultStatusFinal {
		t.Fatalf("terminal result status = %q, want FINAL", terminal.result.ResultStatus)
	}
}

func TestJavaScriptRuntimeService_ApplyRunningRuntimeRecord_CheckpointProjectsPartialResult(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-checkpoint-running-record-001"
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, "dispatch-1", "step-one"); err != nil {
		t.Fatalf("seedRuntimeSessionWithRunningDispatch: %v", err)
	}

	service.applyRunningRuntimeRecord(sessionID, factory.JavaScriptRuntimeRecord{
		Sequence: 2,
		Kind:     factory.JavaScriptRecordKindCheckpoint,
		Checkpoint: &factory.JavaScriptCheckpointRecord{
			ID:    "checkpoint-1",
			Label: "after-step-one",
			State: map[string]any{"text": "checkpoint partial output"},
		},
	})

	result, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModePartial})
	if err != nil {
		t.Fatalf("GetResult partial: %v", err)
	}
	if result.ResultStatus != ResultStatusPartial {
		t.Fatalf("partial status = %q, want PARTIAL", result.ResultStatus)
	}
	if len(result.PrimaryResult) == 0 {
		t.Fatal("partial primaryResult missing")
	}
	if result.SessionStatus != LifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING", result.SessionStatus)
	}
}

func TestFinalizeInterruptedTerminalSession_PreservesPartialAndUnavailableResults(t *testing.T) {
	t.Parallel()
	sessionID := "dur-sess-interrupted-finalize-001"
	interruptedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	t.Run("partial result", func(t *testing.T) {
		state := &runtimeSessionState{
			session: SessionReadResult{
				SessionID: sessionID,
				Status:    LifecycleStatusRunning,
				Lifecycle: &LifecycleTimestamps{InterruptedAt: &interruptedAt},
			},
		}
		priorSession := SessionReadResult{
			SessionID:     sessionID,
			ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusPartial), Summary: "partial output"},
		}
		priorResult := ResultReadResult{
			SessionID:     sessionID,
			ResultStatus:  ResultStatusPartial,
			PrimaryResult: json.RawMessage(`{"text":"partial output"}`),
		}
		finalizeInterruptedTerminalSession(state, priorSession, priorResult)
		if state.session.Status != LifecycleStatusInterrupted {
			t.Fatalf("status = %q, want INTERRUPTED", state.session.Status)
		}
		if state.result.ResultStatus != ResultStatusPartial {
			t.Fatalf("result status = %q, want PARTIAL", state.result.ResultStatus)
		}
		if state.session.ResultSummary == nil || state.session.ResultSummary.ResultStatus != string(ResultStatusPartial) {
			t.Fatalf("result summary = %#v, want PARTIAL", state.session.ResultSummary)
		}
	})

	t.Run("unavailable result", func(t *testing.T) {
		state := &runtimeSessionState{
			session: SessionReadResult{
				SessionID: sessionID,
				Status:    LifecycleStatusRunning,
				Lifecycle: &LifecycleTimestamps{FinishedAt: &interruptedAt},
			},
		}
		finalizeInterruptedTerminalSession(state, SessionReadResult{SessionID: sessionID}, ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
		})
		if state.result.ResultStatus != ResultStatusUnavailable {
			t.Fatalf("result status = %q, want UNAVAILABLE", state.result.ResultStatus)
		}
		if state.result.Availability == nil || state.result.Availability.Reason != "SESSION_INTERRUPTED" {
			t.Fatalf("availability = %#v, want SESSION_INTERRUPTED", state.result.Availability)
		}
	})
}

// pkgmaintcheck:ignore-function-lines this restart integration test keeps the pre-restart and post-restart observable read assertions together.
// pkgmaintcheck:ignore-cyclomatic-complexity each assertion validates one durable partial-result field across the restart boundary.
func TestJavaScriptRuntimeService_PausePersistsStablePartialTerminalReadState(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	sessionID := "dur-sess-paused-restart-001"
	dispatchID := "dispatch-completed-1"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "completed child"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	service.mu.Lock()
	state := service.sessions[sessionID]
	state.dispatches[0].Status = DispatchStatusCompleted
	state.dispatches[0].OutputArtifactIDs = []string{"artifact-1"}
	state.dispatchStatusTransitions[dispatchID] = []DispatchStatus{
		DispatchStatusQueued,
		DispatchStatusRunning,
		DispatchStatusCompleted,
	}
	state.artifacts = []ArtifactSummary{{
		ID:         "artifact-1",
		Kind:       "CHILD_RESULT",
		Visibility: "session",
		DispatchID: dispatchID,
	}}
	state.session.ArtifactCount = 1
	state.session.ArtifactRefs = artifactRefsFromSummaries(state.artifacts)
	state.events = rebuildRuntimeSessionCanonicalEvents(state)
	service.mu.Unlock()

	paused, err := service.Pause(context.Background(), sessionID, ControlRequest{RequestID: "pause-restart-001"})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Status != LifecycleStatusPaused {
		t.Fatalf("pause status = %q, want PAUSED", paused.Status)
	}

	wantSession, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession before restart: %v", err)
	}
	wantResult, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModePartial, IncludeArtifacts: true})
	if err != nil {
		t.Fatalf("GetResult before restart: %v", err)
	}
	wantDispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches before restart: %v", err)
	}
	wantEvents, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents before restart: %v", err)
	}

	restarted := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	gotSession, err := restarted.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession after restart: %v", err)
	}
	gotResult, err := restarted.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModePartial, IncludeArtifacts: true})
	if err != nil {
		t.Fatalf("GetResult after restart: %v", err)
	}
	gotDispatches, err := restarted.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches after restart: %v", err)
	}
	gotEvents, err := restarted.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after restart: %v", err)
	}

	if !reflect.DeepEqual(gotSession, wantSession) {
		t.Fatalf("session changed across restart:\ngot  %#v\nwant %#v", gotSession, wantSession)
	}
	if !reflect.DeepEqual(gotResult, wantResult) || gotResult.ResultStatus != ResultStatusNotReady || gotResult.Availability == nil {
		t.Fatalf("result changed across restart: got %#v want %#v", gotResult, wantResult)
	}
	if !reflect.DeepEqual(gotDispatches, wantDispatches) {
		t.Fatalf("dispatches changed across restart: got %#v want %#v", gotDispatches, wantDispatches)
	}
	if len(gotEvents.Events) != len(wantEvents.Events) {
		t.Fatalf("event count changed across restart: got %d want %d", len(gotEvents.Events), len(wantEvents.Events))
	}
	for index := range wantEvents.Events {
		var gotEvent, wantEvent any
		if err := json.Unmarshal(gotEvents.Events[index], &gotEvent); err != nil {
			t.Fatalf("decode restarted event %d: %v", index, err)
		}
		if err := json.Unmarshal(wantEvents.Events[index], &wantEvent); err != nil {
			t.Fatalf("decode live event %d: %v", index, err)
		}
		if !reflect.DeepEqual(gotEvent, wantEvent) {
			t.Fatalf("event %d changed across restart: got %#v want %#v", index, gotEvent, wantEvent)
		}
	}
}

func TestJavaScriptRuntimeService_PausePersistenceFailureKeepsRunningProjection(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{saveErr: errors.New("pause persistence unavailable")}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	sessionID := "dur-sess-pause-persist-failure-001"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, "dispatch-1", "running child"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}
	cancelCalls := 0
	service.mu.Lock()
	service.sessions[sessionID].runCancel = func() { cancelCalls++ }
	service.mu.Unlock()

	_, err := service.Pause(context.Background(), sessionID, ControlRequest{})
	if err == nil || !strings.Contains(err.Error(), "persist durable session snapshot") {
		t.Fatalf("Pause error = %v, want persistence failure", err)
	}
	read, readErr := service.GetSession(context.Background(), sessionID)
	if readErr != nil {
		t.Fatalf("GetSession: %v", readErr)
	}
	if read.Status != LifecycleStatusRunning || read.Lifecycle == nil || read.Lifecycle.PausedAt != nil {
		t.Fatalf("session after rejected pause = %#v, want unchanged RUNNING projection", read)
	}
	if _, err := service.Cancel(context.Background(), sessionID, ControlRequest{}); err != nil {
		t.Fatalf("Cancel after rejected pause: %v", err)
	}
	if cancelCalls != 1 {
		t.Fatalf("cancel calls after rejected pause = %d, want 1", cancelCalls)
	}
}

func TestInterruptedTerminalTimestamp_PrefersSessionLifecycle(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	interruptedAt := time.Date(2026, 6, 29, 11, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 29, 12, 30, 0, 0, time.UTC)

	got := interruptedTerminalTimestamp(
		SessionReadResult{Lifecycle: &LifecycleTimestamps{InterruptedAt: &interruptedAt}},
		SessionReadResult{Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt}},
	)
	if got == nil || !got.Equal(interruptedAt) {
		t.Fatalf("timestamp = %v, want interruptedAt", got)
	}

	got = interruptedTerminalTimestamp(
		SessionReadResult{Lifecycle: &LifecycleTimestamps{FinishedAt: &finishedAt}},
		SessionReadResult{},
	)
	if got == nil || !got.Equal(finishedAt) {
		t.Fatalf("timestamp = %v, want finishedAt", got)
	}

	got = interruptedTerminalTimestamp(
		SessionReadResult{Lifecycle: &LifecycleTimestamps{UpdatedAt: &updatedAt}},
		SessionReadResult{},
	)
	if got == nil || !got.Equal(updatedAt) {
		t.Fatalf("timestamp = %v, want updatedAt", got)
	}

	got = interruptedTerminalTimestamp(SessionReadResult{}, SessionReadResult{
		Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt},
	})
	if got == nil || !got.Equal(startedAt) {
		t.Fatalf("timestamp = %v, want prior startedAt", got)
	}
}

func TestResumeHelperFunctions_CoverMergeCloneAndPolicyPaths(t *testing.T) {
	t.Parallel()
	existing := []factory.JavaScriptRuntimeRecord{{Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "cp-1"}}}
	resumed := []factory.JavaScriptRuntimeRecord{{Kind: factory.JavaScriptRecordKindChildDispatch, ChildDispatch: &factory.JavaScriptChildDispatchRecord{DispatchID: "dispatch-1"}}}
	merged := mergeRuntimeRecords(existing, resumed)
	if len(merged) != 2 {
		t.Fatalf("merged records = %d, want 2", len(merged))
	}
	if len(mergeRuntimeRecords(nil, resumed)) != 1 {
		t.Fatal("mergeRuntimeRecords(nil, resumed) should clone resumed records")
	}
	if len(mergeRuntimeRecords(existing, nil)) != 1 {
		t.Fatal("mergeRuntimeRecords(existing, nil) should clone existing records")
	}

	policy := workflowPolicyFromSessionPolicy(PolicyProjection{})
	defaultPolicy := factory.DefaultJavaScriptPolicy()
	if policy.Mode != defaultPolicy.Mode {
		t.Fatalf("policy mode = %q, want default %q", policy.Mode, defaultPolicy.Mode)
	}
	customPolicy := workflowPolicyFromSessionPolicy(PolicyProjection{
		Effective: map[string]any{"mode": factory.JavaScriptPolicyModeReadOnly},
	})
	if customPolicy.Mode != factory.JavaScriptPolicyModeReadOnly {
		t.Fatalf("policy mode = %q, want %q", customPolicy.Mode, factory.JavaScriptPolicyModeReadOnly)
	}

	summary := &factory.JavaScriptCheckpointSummary{
		CheckpointID:         "checkpoint-1",
		CompletedDispatchIDs: []string{"dispatch-1"},
		PendingDispatchIDs:   []string{"dispatch-2"},
		ArtifactIDs:          []string{"artifact-1"},
		CheckpointState:      map[string]any{"phase": "execute"},
		CreatedAt:            time.Now().UTC(),
	}
	cloned := cloneCheckpointSummary(summary)
	if cloned == nil || cloned.CheckpointID != summary.CheckpointID {
		t.Fatalf("cloneCheckpointSummary = %#v", cloned)
	}
	cloned.CompletedDispatchIDs[0] = "mutated"
	if summary.CompletedDispatchIDs[0] != "dispatch-1" {
		t.Fatal("cloneCheckpointSummary should deep-copy completed dispatch ids")
	}

	if latestCheckpointSummaryFromRuntime(checkpointfixtures.CheckpointSummariesFixture{}, "dur-sess-1", nil, nil) != nil {
		t.Fatal("latestCheckpointSummaryFromRuntime(nil state) = summary, want nil")
	}
}

func TestCheckpointEventProjection_BuildsCanonicalCheckpointEvents(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-checkpoint-events-001"
	state := &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusInterrupted,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1",
			Phase:            "execute",
			SourceHash:       "sha256:fixture",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusInterrupted,
			ResultStatus:  ResultStatusPartial,
		},
		checkpointSummary: &factory.JavaScriptCheckpointSummary{
			CheckpointID: "checkpoint-1",
			CreatedAt:    startedAt.Add(time.Minute),
		},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{{
			Kind: factory.JavaScriptRecordKindCheckpoint,
			Checkpoint: &factory.JavaScriptCheckpointRecord{
				ID:      "checkpoint-1",
				Label:   "after-first-child",
				Summary: "checkpoint after first child",
			},
		}},
	}
	checkpoints := checkpointEventsFromRuntimeState(state)
	if len(checkpoints) != 1 || checkpoints[0].CheckpointID != "checkpoint-1" {
		t.Fatalf("checkpoint events = %#v", checkpoints)
	}
	if checkpoints[0].ResumabilityStatus != "RESUMABLE" {
		t.Fatalf("resumability = %q, want RESUMABLE", checkpoints[0].ResumabilityStatus)
	}

	events := BuildCanonicalRuntimeSessionEvents(state.session, state.result, runtimeDispatchEventInputFromState(state))
	events = appendCanonicalOrchestratorCheckpointEvents(events, state.session, checkpoints, canonicalEventSourceRuntimeService)
	found := false
	for _, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type == "ORCHESTRATOR_CHECKPOINT_WRITTEN" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected ORCHESTRATOR_CHECKPOINT_WRITTEN canonical event")
	}
}

func TestPhaseEventProjection_PreservesOrderedRunningAndTerminalPhases(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	session := SessionReadResult{
		SessionID:        "dur-sess-phase-events-001",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          "you-workflow-v1",
		Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		PhaseSummaries: []PhaseSummary{
			{Phase: "setup"}, {Phase: " "}, {Phase: "execute"},
		},
	}
	events := appendCanonicalOrchestratorPhaseEvents(nil, session, canonicalEventSourceRuntimeService)
	if got, want := phaseEventStatuses(t, events), []string{"setup:COMPLETED", "execute:ACTIVE"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("running phases = %v, want %v", got, want)
	}

	session.Status = LifecycleStatusSucceeded
	events = appendCanonicalOrchestratorPhaseEvents(nil, session, canonicalEventSourceRuntimeService)
	if got, want := phaseEventStatuses(t, events), []string{"setup:COMPLETED", "execute:COMPLETED"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal phases = %v, want %v", got, want)
	}
	if got := appendCanonicalOrchestratorPhaseEvents(events, SessionReadResult{}, canonicalEventSourceRuntimeService); len(got) != len(events) {
		t.Fatalf("empty phase projection changed event count from %d to %d", len(events), len(got))
	}
}

func phaseEventStatuses(t *testing.T, events []json.RawMessage) []string {
	t.Helper()
	statuses := make([]string, 0, len(events))
	for _, raw := range events {
		var event struct {
			Context struct {
				PhaseID *string `json:"phaseId"`
			} `json:"context"`
			Payload struct {
				PhaseStatus string `json:"phaseStatus"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode phase event: %v", err)
		}
		if event.Context.PhaseID != nil && event.Payload.PhaseStatus != "" {
			statuses = append(statuses, *event.Context.PhaseID+":"+event.Payload.PhaseStatus)
		}
	}
	return statuses
}

func TestJavaScriptRuntimeService_FactoryEventObserverDeliversOnlyUnseenEvents(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-observer-events-001"
	session := SessionReadResult{
		SessionID: sessionID, Status: LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
	}
	state := &runtimeSessionState{
		session: session,
		result:  ResultReadResult{SessionID: sessionID, SessionStatus: LifecycleStatusRunning},
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	service := &JavaScriptRuntimeService{sessions: map[string]*runtimeSessionState{sessionID: state}}
	var delivered []interfaces.FactoryEvent
	stop := service.observeFactoryEvents(state, func(events []interfaces.FactoryEvent) {
		delivered = append(delivered, events...)
	})
	service.presentCurrentFactoryEvents(sessionID)
	service.presentCurrentFactoryEvents(sessionID)
	if len(delivered) != len(state.events) {
		t.Fatalf("delivered %d events after duplicate presentation, want %d", len(delivered), len(state.events))
	}
	stop()
	service.presentCurrentFactoryEvents(sessionID)
	if len(delivered) != len(state.events) {
		t.Fatalf("delivery continued after observer stopped: got %d, want %d", len(delivered), len(state.events))
	}
	if stopNil := service.observeFactoryEvents(state, nil); stopNil == nil {
		t.Fatal("nil observer cleanup is nil")
	} else {
		stopNil()
	}
	service.unregisterFactoryEventConsumer("missing-session")
	service.presentCurrentFactoryEvents("missing-session")
}

func TestRuntimeRecordEvents_ReconcileAppendOnlyPhaseCheckpointPhaseHistory(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)
	const sessionID = "dur-sess-append-only-events-001"
	records := []factory.JavaScriptRuntimeRecord{
		{Sequence: 1, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "plan"}},
		{Sequence: 2, Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "checkpoint-plan", Label: "plan-ready"}},
		{Sequence: 3, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "execute"}},
	}
	state := &runtimeSessionState{
		session: SessionReadResult{
			SessionID: sessionID, Status: LifecycleStatusRunning,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1", SourceHash: "sha256:append-only",
			Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt},
		},
		result: ResultReadResult{
			SessionID: sessionID, SessionStatus: LifecycleStatusRunning,
			ResultStatus: ResultStatusNotReady,
		},
		checkpointSummary: &factory.JavaScriptCheckpointSummary{
			CheckpointID: "checkpoint-plan", CreatedAt: startedAt.Add(time.Second),
		},
		runtimeRecords: append(append([]factory.JavaScriptRuntimeRecord(nil), records...), records...),
		eventConsumer:  func([]interfaces.FactoryEvent) {},
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	running := rebuildRuntimeSessionCanonicalEvents(state)
	assertStrictCanonicalSequences(t, running)
	if got, want := phaseEventStatuses(t, running), []string{"plan:ACTIVE", "plan:COMPLETED", "execute:ACTIVE"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("running phase transitions = %v, want %v", got, want)
	}

	state.events = running
	state.session.Status = LifecycleStatusSucceeded
	state.result.SessionStatus = LifecycleStatusSucceeded
	state.result.ResultStatus = ResultStatusFinal
	terminal := rebuildRuntimeSessionCanonicalEvents(state)
	assertStrictCanonicalSequences(t, terminal)
	if got, want := phaseEventStatuses(t, terminal), []string{"plan:ACTIVE", "plan:COMPLETED", "execute:ACTIVE", "execute:COMPLETED"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal phase transitions = %v, want %v", got, want)
	}
	if len(terminal) <= len(running) {
		t.Fatalf("terminal events = %d, want append beyond %d running events", len(terminal), len(running))
	}
	for index := range running {
		if string(terminal[index]) != string(running[index]) {
			t.Fatalf("published event %d was mutated:\nrunning=%s\nterminal=%s", index, running[index], terminal[index])
		}
	}
}

func assertStrictCanonicalSequences(t *testing.T, events []json.RawMessage) {
	t.Helper()
	previousSequence := 0
	previousSessionSequence := -1
	for index, raw := range events {
		var event interfaces.FactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode event %d: %v", index, err)
		}
		if event.Context.Sequence <= previousSequence || event.Context.SessionSequence == nil ||
			*event.Context.SessionSequence <= previousSessionSequence {
			t.Fatalf("event %d sequence context is not increasing: %#v", index, event.Context)
		}
		previousSequence = event.Context.Sequence
		previousSessionSequence = *event.Context.SessionSequence
	}
}

func TestFakeService_ResumeInterruptedSession_ReturnsUnsupported(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	_, err := service.ResumeInterruptedSession(context.Background(), "dur-sess-petri-run-001", ResumeSessionRequest{
		RequestID: "req-fake-resume-unsupported-001",
	})
	if !errors.Is(err, ErrUnsupportedControl) {
		t.Fatalf("ResumeInterruptedSession error = %v, want ErrUnsupportedControl", err)
	}
}

// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestJavaScriptRuntimeService_ResumeInterruptedSession_PackageLocalCoverage(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-0123456789abcdef0123456789abcdef"
	projectRoot := t.TempDir()
	store := mustTestRuntimePersistenceStore(t, runtimepersist.DirForProjectRoot(projectRoot))
	startedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	startRequest := StartRequest{
		RequestID: "req-package-resume-start-001",
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "resumable-two-step-fake-children",
		},
		Args: map[string]any{"subject": "workflows"},
	}
	checkpointSummary := checkpointfixtures.ResumableCheckpointSummaryResult()
	state := runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusInterrupted,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1",
			SourceHash:       "sha256:scripted",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt, InterruptedAt: &startedAt},
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusInterrupted,
			ResultStatus:  ResultStatusPartial,
		},
		dispatches: []DispatchSummary{{
			ID: "dispatch-1", Status: DispatchStatusCompleted, Attempt: 1,
		}},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{
			{
				Sequence: 1,
				Kind:     factory.JavaScriptRecordKindChildDispatch,
				ChildDispatch: &factory.JavaScriptChildDispatchRecord{
					DispatchID: "dispatch-1", ChildIndex: 1,
					Status: factory.JavaScriptChildDispatchStatusCompleted,
					Output: map[string]any{"text": "step one"},
				},
			},
			{
				Sequence: 2,
				Kind:     factory.JavaScriptRecordKindCheckpoint,
				Checkpoint: &factory.JavaScriptCheckpointRecord{
					ID: "checkpoint-1", Label: "after-step-one",
				},
			},
		},
		checkpointSummary: checkpointSummary,
		startRequest:      &startRequest,
		resolvedSource: ResolvedSource{
			Kind:       factory.WorkflowSourceKindWorkflowName,
			SourceRef:  "resumable-two-step-fake-children.workflow.js",
			SourceHash: "sha256:scripted",
			Dialect:    "you-workflow-v1",
		},
		sourceContent: "scripted resumable workflow",
	}
	state.events = rebuildRuntimeSessionCanonicalEvents(&state)
	encoded, err := json.Marshal(persistedSnapshotFromRuntimeState(state))
	if err != nil {
		t.Fatalf("marshal interrupted snapshot: %v", err)
	}
	if err := store.Save(sessionID, encoded); err != nil {
		t.Fatalf("persist interrupted snapshot: %v", err)
	}

	var resumeContextCalls int
	workflows := factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		ResumeContextFunc: func(
			summary factory.JavaScriptCompletedCheckpointSummary,
			records []factory.JavaScriptRuntimeRecord,
		) factory.JavaScriptResumeContext {
			resumeContextCalls++
			if len(summary.CompletedDispatchIDs) != 1 || summary.CompletedDispatchIDs[0] != "dispatch-1" || len(records) != 2 {
				t.Fatalf("resume inputs = %#v / %#v", summary, records)
			}
			return factory.JavaScriptResumeContext{
				CompletedDispatchIDs: []string{"dispatch-1"},
			}
		},
		RunFunc: func(
			_ context.Context,
			request factory.JavaScriptRuntimeRequest,
			_ factory.JavaScriptRuntimeHooks,
		) (factory.JavaScriptRuntimeOutcome, error) {
			if request.Resume == nil || len(request.Resume.CompletedDispatchIDs) != 1 {
				t.Fatalf("runtime resume context = %#v", request.Resume)
			}
			value, marshalErr := json.Marshal(map[string]any{"status": "resumed"})
			return factory.JavaScriptRuntimeOutcome{
				OK: true, Value: factory.TypedValue{JSON: value},
			}, marshalErr
		},
	}
	resumedService := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: store,
		Workflows:   workflows,
	})

	resumed, err := resumedService.ResumeInterruptedSession(context.Background(), sessionID, ResumeSessionRequest{
		RequestID: "req-package-resume-resume-001",
	})
	if err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}
	if resumed.Status != string(LifecycleStatusResuming) && resumed.Status != string(LifecycleStatusSucceeded) {
		t.Fatalf("resumed status = %q, want RESUMING or SUCCEEDED", resumed.Status)
	}

	if resumed.Status != string(LifecycleStatusSucceeded) {
		waitForResumeCoverageSessionStatus(t, resumedService, sessionID, LifecycleStatusSucceeded, 5*time.Second)
	}
	if resumeContextCalls != 1 {
		t.Fatalf("resume context calls = %d, want 1", resumeContextCalls)
	}
}

type resumeCoverageBlockingProvider struct {
	mu              sync.Mutex
	callCount       int
	blockedOnce     bool
	contextCanceled int
}

func newResumeCoverageBlockingProvider() *resumeCoverageBlockingProvider {
	return &resumeCoverageBlockingProvider{}
}

func (p *resumeCoverageBlockingProvider) Execute(
	ctx context.Context,
	input workerexecution.InvocationInput,
) (workerexecution.InvocationResult, error) {
	p.mu.Lock()
	p.callCount++
	call := p.callCount
	alreadyBlocked := p.blockedOnce
	p.mu.Unlock()

	if call == 1 {
		response := workerexecution.InferenceResponse{
			Content: `{"text":"live:resumable-two-step-fake-children:step-one:step-one:workflows","label":"step-one"}`,
			ProviderSession: &workerexecution.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}
		return workerexecution.InvocationResult{
			Response: response, Attempt: input.Attempt,
			ProviderSession: workerexecution.CloneProviderSessionMetadata(response.ProviderSession),
		}, nil
	}

	if !alreadyBlocked {
		p.mu.Lock()
		p.blockedOnce = true
		p.mu.Unlock()

		<-ctx.Done()
		p.mu.Lock()
		p.contextCanceled++
		p.mu.Unlock()
		return workerexecution.InvocationResult{Attempt: input.Attempt}, ctx.Err()
	}

	response := workerexecution.InferenceResponse{
		Content: `{"text":"live:resumable-two-step-fake-children:step-two:step-two:workflows","label":"step-two"}`,
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-2",
		},
	}
	return workerexecution.InvocationResult{
		Response: response, Attempt: input.Attempt,
		ProviderSession: workerexecution.CloneProviderSessionMetadata(response.ProviderSession),
	}, nil
}

func (p *resumeCoverageBlockingProvider) resumeCoverageCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callCount
}

var _ workerexecution.InvocationExecutor = (*resumeCoverageBlockingProvider)(nil)

func (p *resumeCoverageBlockingProvider) waitForCanceledResumeCoverageInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		canceled := p.contextCanceled > 0
		p.mu.Unlock()
		if canceled {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for blocked provider infer cancellation")
}

func setupResumeCoverageWorkflowFixture(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	path := filepath.Join("..", "..", "..", "..", "..", "tests", "fixtures", "javascript_runtime", "resumable-two-step-fake-children.workflow.js")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workflow fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "resumable-two-step-fake-children.js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func waitForResumeCoverageSessionStatus(
	t *testing.T,
	service *JavaScriptRuntimeService,
	sessionID string,
	want LifecycleStatus,
	timeout time.Duration,
) SessionReadResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		read, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if read.Status == want {
			return read
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach status %s within %s", sessionID, want, timeout)
	return SessionReadResult{}
}

func waitForResumeCoverageDispatchStatus(
	t *testing.T,
	service Service,
	sessionID, dispatchID string,
	want DispatchStatus,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
		if err == nil && dispatch.Status == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("dispatch %s did not reach status %s within %s", dispatchID, want, timeout)
}

func TestJavaScriptRuntimeServiceWriteRecordingUsesCanonicalSnapshotAndCorrelatesFailure(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-1234567890abcdef1234567890abcdef"
	observedAt := time.Date(2026, 7, 12, 16, 30, 0, 0, time.UTC)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	service.sessions[sessionID] = &runtimeSessionState{
		session:        SessionReadResult{SessionID: sessionID, Status: LifecycleStatusSucceeded, OrchestratorKind: interfaces.OrchestratorKindJavaScript, ResolvedSource: ResolvedSource{SourceRef: "workflow/audit.js"}, SourceHash: "sha256:" + strings.Repeat("1", 64), Policy: PolicyProjection{EffectiveHash: "sha256:" + strings.Repeat("2", 64)}},
		startRequest:   &StartRequest{Args: map[string]any{"customer": "north"}},
		artifacts:      []ArtifactSummary{{ID: "artifact-1", Kind: "RESULT", Visibility: "PUBLIC", ContentHash: "sha256:" + strings.Repeat("3", 64), SizeBytes: 2, CreatedAt: &observedAt}},
		events:         []json.RawMessage{json.RawMessage(`{"id":"event-1","type":"SESSION_COMPLETED","context":{"sequence":0,"eventTime":"2026-07-12T16:30:00Z"},"payload":{"artifactIds":["artifact-1"]}}`)},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{{Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "checkpoint-secret", State: map[string]any{"secret": "raw-state"}}}},
	}
	path := filepath.Join(t.TempDir(), "session.recording.json")
	if err := service.WriteRecording(context.Background(), sessionID, path); err != nil {
		t.Fatalf("WriteRecording: %v", err)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(encoded), "checkpoint-secret") || strings.Contains(string(encoded), "raw-state") {
		t.Fatalf("recording leaked runtime state: %s", encoded)
	}
	badPath := filepath.Join(t.TempDir(), "missing", "\x00invalid")
	err = service.WriteRecording(context.Background(), sessionID, badPath)
	var recordingErr *RecordingError
	if !errors.As(err, &recordingErr) || recordingErr.SessionID != sessionID || recordingErr.Path != badPath {
		t.Fatalf("WriteRecording failure = %#v", err)
	}
	read, readErr := service.GetSession(context.Background(), sessionID)
	if readErr != nil || read.Status != LifecycleStatusSucceeded {
		t.Fatalf("live session changed after recording failure: read=%#v err=%v", read, readErr)
	}
}
