// pkgmaintcheck:ignore-file-lines consolidated fake durable session service tests remain together until dedicated fake-service test seams split.
// backendsizecheck:ignore-file consolidated fake durable session service tests remain together until dedicated fake-service test seams split.
package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// pkgmaintcheck:ignore-cyclomatic-complexity this lifecycle test keeps supported control outcomes and links together on one seam.
// pkgmaintcheck:ignore-function-lines this lifecycle test keeps supported control outcomes and links together on one seam.
// backendsizecheck:ignore-function this lifecycle test keeps supported control outcomes and links together on one seam.
func TestFakeService_LifecycleControls_AllSupportedOutcomes(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")
	startAsyncByRequestID(t, service, "req-petri-success-001")
	startAsyncByRequestID(t, service, "req-js-awaiting-001")
	startAsyncByRequestID(t, service, "req-petri-cancel-001")
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")

	cases := []struct {
		name      string
		sessionID string
		call      func() (LifecycleControlResult, error)
		want      LifecycleControlOutcome
		wantErr   bool
	}{
		{
			name:      "pause running accepted",
			sessionID: "dur-sess-js-run-n-001",
			call: func() (LifecycleControlResult, error) {
				return service.Pause(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
			},
			want: LifecycleControlOutcomeAccepted,
		},
		{
			name:      "pause paused no-op",
			sessionID: "dur-sess-js-run-n-001",
			call: func() (LifecycleControlResult, error) {
				return service.Pause(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
			},
			want: LifecycleControlOutcomeNoOp,
		},
		{
			name:      "pause awaiting approval invalid",
			sessionID: "dur-sess-js-awaiting-001",
			call: func() (LifecycleControlResult, error) {
				return service.Pause(context.Background(), "dur-sess-js-awaiting-001", ControlRequest{})
			},
			wantErr: true,
		},
		{
			name:      "approve awaiting approval accepted",
			sessionID: "dur-sess-js-awaiting-001",
			call: func() (LifecycleControlResult, error) {
				return service.Approve(context.Background(), "dur-sess-js-awaiting-001", ApproveRequest{
					ControlRequest: ControlRequest{},
				})
			},
			want: LifecycleControlOutcomeAccepted,
		},
		{
			name:      "terminate paused accepted",
			sessionID: "dur-sess-js-run-n-001",
			call: func() (LifecycleControlResult, error) {
				return service.Terminate(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
			},
			want: LifecycleControlOutcomeAccepted,
		},
		{
			name:      "cancel canceled no-op",
			sessionID: "dur-sess-petri-cancel-001",
			call: func() (LifecycleControlResult, error) {
				return service.Cancel(context.Background(), "dur-sess-petri-cancel-001", ControlRequest{})
			},
			want: LifecycleControlOutcomeNoOp,
		},
		{
			name:      "pause terminal success rejected",
			sessionID: "dur-sess-petri-success-001",
			call: func() (LifecycleControlResult, error) {
				return service.Pause(context.Background(), "dur-sess-petri-success-001", ControlRequest{})
			},
			wantErr: true,
		},
		{
			name:      "retry terminal success rejected",
			sessionID: "dur-sess-petri-success-001",
			call: func() (LifecycleControlResult, error) {
				return service.RetryDispatch(context.Background(), "dur-sess-petri-success-001", RetryDispatchRequest{
					ControlRequest: ControlRequest{},
					DispatchID:     "disp-petri-success-001",
				})
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if tc.wantErr {
				var controlErr *ControlError
				if !errors.As(err, &controlErr) {
					t.Fatalf("error = %v, want ControlError", err)
				}
				switch tc.name {
				case "pause awaiting approval invalid":
					if controlErr.Outcome != LifecycleControlOutcomeInvalidState {
						t.Fatalf("outcome = %q, want INVALID_STATE", controlErr.Outcome)
					}
				default:
					if controlErr.Outcome != LifecycleControlOutcomeTerminalSession {
						t.Fatalf("outcome = %q, want TERMINAL_SESSION", controlErr.Outcome)
					}
				}
				detail, detailErr := service.GetSession(context.Background(), tc.sessionID)
				if detailErr != nil {
					t.Fatalf("GetSession after rejected control: %v", detailErr)
				}
				if tc.name == "pause terminal success rejected" && detail.Status != LifecycleStatusSucceeded {
					t.Fatalf("status = %q, want SUCCEEDED", detail.Status)
				}
				if tc.name == "pause awaiting approval invalid" && detail.Status != LifecycleStatusAwaitingApproval {
					t.Fatalf("status = %q, want AWAITING_APPROVAL", detail.Status)
				}
				return
			}
			if err != nil {
				t.Fatalf("control: %v", err)
			}
			if result.Outcome != tc.want {
				t.Fatalf("outcome = %q, want %q", result.Outcome, tc.want)
			}
			if err := ValidateLifecycleControlLinks(tc.sessionID, result.Links); err != nil {
				t.Fatalf("ValidateLifecycleControlLinks: %v", err)
			}
		})
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this replay test keeps lifecycle idempotency and projection stability assertions together.
func TestFakeService_LifecycleControl_IdempotentReplay_NoDuplicateEventsOrRetries(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")

	first, err := service.Pause(context.Background(), "dur-sess-js-run-n-001", ControlRequest{
		RequestID: "ctrl-pause-events-001",
	})
	if err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	eventsAfterFirst, err := service.ReadEvents(context.Background(), "dur-sess-js-run-n-001", EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after pause: %v", err)
	}

	second, err := service.Pause(context.Background(), "dur-sess-js-run-n-001", ControlRequest{
		RequestID: "ctrl-pause-events-001",
	})
	if err != nil {
		t.Fatalf("replay Pause: %v", err)
	}
	if second.Outcome != first.Outcome || second.Status != first.Status {
		t.Fatalf("replay result = %#v, want %#v", second, first)
	}
	eventsAfterReplay, err := service.ReadEvents(context.Background(), "dur-sess-js-run-n-001", EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after replay: %v", err)
	}
	if len(eventsAfterReplay.Events) != len(eventsAfterFirst.Events) {
		t.Fatalf("event count after replay = %d, want %d", len(eventsAfterReplay.Events), len(eventsAfterFirst.Events))
	}

	firstRetry, err := service.RetryDispatch(context.Background(), "dur-sess-js-failed-partial-001", RetryDispatchRequest{
		ControlRequest: ControlRequest{RequestID: "ctrl-retry-events-001"},
		DispatchID:     "disp-js-fail-002",
	})
	if err != nil {
		t.Fatalf("first RetryDispatch: %v", err)
	}
	dispatchesAfterFirst, err := service.ListDispatches(context.Background(), "dur-sess-js-failed-partial-001")
	if err != nil {
		t.Fatalf("ListDispatches after retry: %v", err)
	}
	var retried DispatchSummary
	for _, dispatch := range dispatchesAfterFirst.Dispatches {
		if dispatch.ID == "disp-js-fail-002" {
			retried = dispatch
			break
		}
	}
	if retried.Attempt != 2 {
		t.Fatalf("retried attempt = %d, want 2", retried.Attempt)
	}

	secondRetry, err := service.RetryDispatch(context.Background(), "dur-sess-js-failed-partial-001", RetryDispatchRequest{
		ControlRequest: ControlRequest{RequestID: "ctrl-retry-events-001"},
		DispatchID:     "disp-js-fail-002",
	})
	if err != nil {
		t.Fatalf("replay RetryDispatch: %v", err)
	}
	if secondRetry.Outcome != firstRetry.Outcome || secondRetry.Status != firstRetry.Status {
		t.Fatalf("replay retry = %#v, want %#v", secondRetry, firstRetry)
	}
	dispatchesAfterReplay, err := service.ListDispatches(context.Background(), "dur-sess-js-failed-partial-001")
	if err != nil {
		t.Fatalf("ListDispatches after replay: %v", err)
	}
	for _, dispatch := range dispatchesAfterReplay.Dispatches {
		if dispatch.ID == "disp-js-fail-002" && dispatch.Attempt != 2 {
			t.Fatalf("retried attempt after replay = %d, want 2", dispatch.Attempt)
		}
	}
}

func TestFakeService_LifecycleControls_TerminalSessionsDoNotResumeViaControls(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")

	for _, tc := range []struct {
		name      string
		sessionID string
		want      LifecycleStatus
	}{
		{"success", "dur-sess-petri-success-001", LifecycleStatusSucceeded},
		{"failed-with-partial", "dur-sess-js-failed-partial-001", LifecycleStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Pause(context.Background(), tc.sessionID, ControlRequest{})
			var controlErr *ControlError
			if !errors.As(err, &controlErr) || controlErr.Outcome != LifecycleControlOutcomeTerminalSession {
				t.Fatalf("pause error = %v, want TERMINAL_SESSION", err)
			}
			detail, err := service.GetSession(context.Background(), tc.sessionID)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if detail.Status != tc.want {
				t.Fatalf("status = %q, want %q", detail.Status, tc.want)
			}
		})
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this retry-dispatch test keeps targeted mutation assertions together on one seam.
func TestFakeService_RetryDispatch_UpdatesOnlyTargetDispatch(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")
	sessionID := "dur-sess-js-failed-partial-001"

	beforeDispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches before: %v", err)
	}
	beforeArtifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts before: %v", err)
	}

	_, err = service.RetryDispatch(context.Background(), sessionID, RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "disp-js-fail-002",
	})
	if err != nil {
		t.Fatalf("RetryDispatch: %v", err)
	}

	afterDispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches after: %v", err)
	}
	afterArtifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts after: %v", err)
	}
	if len(afterArtifacts.Artifacts) != len(beforeArtifacts.Artifacts) {
		t.Fatalf("artifact count = %d, want %d", len(afterArtifacts.Artifacts), len(beforeArtifacts.Artifacts))
	}

	beforeByID := map[string]DispatchSummary{}
	for _, dispatch := range beforeDispatches.Dispatches {
		beforeByID[dispatch.ID] = dispatch
	}
	for _, dispatch := range afterDispatches.Dispatches {
		before, ok := beforeByID[dispatch.ID]
		if !ok {
			t.Fatalf("unexpected dispatch %q", dispatch.ID)
		}
		switch dispatch.ID {
		case "disp-js-fail-002":
			if dispatch.Status != DispatchStatusQueued {
				t.Fatalf("retried status = %q, want QUEUED", dispatch.Status)
			}
			if dispatch.Attempt != before.Attempt+1 {
				t.Fatalf("retried attempt = %d, want %d", dispatch.Attempt, before.Attempt+1)
			}
		default:
			if dispatch.Status != before.Status || dispatch.Attempt != before.Attempt ||
				dispatch.DispatchKind != before.DispatchKind || dispatch.Label != before.Label {
				t.Fatalf("dispatch %q changed: before=%#v after=%#v", dispatch.ID, before, dispatch)
			}
		}
	}

	detail, err := service.GetDispatch(context.Background(), sessionID, "disp-js-fail-002")
	if err != nil {
		t.Fatalf("GetDispatch retried: %v", err)
	}
	if detail.Status != DispatchStatusQueued {
		t.Fatalf("detail status = %q, want QUEUED", detail.Status)
	}
	var retriedSummary DispatchSummary
	for _, dispatch := range afterDispatches.Dispatches {
		if dispatch.ID == "disp-js-fail-002" {
			retriedSummary = dispatch
			break
		}
	}
	if retriedSummary.ID == "" {
		t.Fatal("retried dispatch summary missing")
	}
	if err := ValidateDispatchDetailMatchesListSummary(detail, retriedSummary); err != nil {
		t.Fatalf("ValidateDispatchDetailMatchesListSummary: %v", err)
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

func TestFakeService_ReadEvents_ReplayMatchesDirectProjections(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		requestID     string
		sessionID     string
		resultRequest ResultRequest
		wantEvents    int
	}{
		{"req-petri-success-001", "dur-sess-petri-success-001", ResultRequest{Mode: ResultModeFinal, IncludeArtifacts: true}, 3},
		{"req-petri-run-001", "dur-sess-petri-run-001", ResultRequest{Mode: ResultModePartial}, 2},
		{"req-js-failed-partial-001", "dur-sess-js-failed-partial-001", ResultRequest{Mode: ResultModePartial}, 3},
		{"req-js-interrupted-001", "dur-sess-js-interrupted-001", ResultRequest{Mode: ResultModePartial}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.sessionID, func(t *testing.T) {
			startAsyncByRequestID(t, service, tc.requestID)

			session, err := service.GetSession(context.Background(), tc.sessionID)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			result, err := service.GetResult(context.Background(), tc.sessionID, tc.resultRequest)
			if err != nil {
				t.Fatalf("GetResult: %v", err)
			}
			dispatches, err := service.ListDispatches(context.Background(), tc.sessionID)
			if err != nil {
				t.Fatalf("ListDispatches: %v", err)
			}
			artifacts, err := service.ListArtifacts(context.Background(), tc.sessionID)
			if err != nil {
				t.Fatalf("ListArtifacts: %v", err)
			}

			first, err := service.ReadEvents(context.Background(), tc.sessionID, EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents first: %v", err)
			}
			second, err := service.ReadEvents(context.Background(), tc.sessionID, EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents second: %v", err)
			}
			if len(first.Events) != tc.wantEvents {
				t.Fatalf("events = %d, want %d", len(first.Events), tc.wantEvents)
			}
			if len(second.Events) != len(first.Events) {
				t.Fatalf("repeated read event count = %d, want %d", len(second.Events), len(first.Events))
			}
			for index := range first.Events {
				if string(first.Events[index]) != string(second.Events[index]) {
					t.Fatalf("event[%d] changed between reads", index)
				}
			}

			if err := ValidateEventReplayMatchesDirectProjections(
				session,
				result,
				dispatches.Dispatches,
				artifacts.Artifacts,
				first.Events,
			); err != nil {
				t.Fatalf("ValidateEventReplayMatchesDirectProjections: %v", err)
			}
		})
	}
}

func TestFakeService_ReadEvents_ReconnectAfterSequenceReturnsLaterEvents(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-success-002")

	all, err := service.ReadEvents(context.Background(), "dur-sess-js-success-002", EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents all: %v", err)
	}
	if len(all.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(all.Events))
	}

	sequence := 1
	afterSequence, err := service.ReadEvents(context.Background(), "dur-sess-js-success-002", EventReconnectRequest{
		AfterSequence: &sequence,
	})
	if err != nil {
		t.Fatalf("ReadEvents after sequence: %v", err)
	}
	if len(afterSequence.Events) != 1 {
		t.Fatalf("after sequence events = %d, want 1", len(afterSequence.Events))
	}
	assertCanonicalEventEnvelope(t, afterSequence.Events[0], "SESSION_COMPLETED", "session-completed/dur-sess-js-success-002")
}

func TestFakeService_ReadEvents_NotFoundDoesNotMutateState(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")

	before, err := service.ReadEvents(context.Background(), "dur-sess-js-run-n-001", EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents before: %v", err)
	}

	_, err = service.ReadEvents(context.Background(), "dur-sess-missing-001", EventReconnectRequest{})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("ReadEvents error = %v, want ErrSessionNotFound", err)
	}

	after, err := service.ReadEvents(context.Background(), "dur-sess-js-run-n-001", EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after: %v", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("event count changed after not-found read: before=%d after=%d", len(before.Events), len(after.Events))
	}
	for index := range before.Events {
		if string(before.Events[index]) != string(after.Events[index]) {
			t.Fatalf("event[%d] changed after not-found read", index)
		}
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this scoped list/detail test keeps lifecycle membership and summary coherence together.
// pkgmaintcheck:ignore-function-lines this scoped list/detail test keeps lifecycle membership and summary coherence together.
// backendsizecheck:ignore-function this scoped list/detail test keeps lifecycle membership and summary coherence together.
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

// pkgmaintcheck:ignore-cyclomatic-complexity this concurrency test keeps idempotent start and projection stability assertions together.
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

// pkgmaintcheck:ignore-cyclomatic-complexity this concurrency test keeps lifecycle control and read replay safety assertions together.
func TestFakeService_LifecycleControl_ConcurrentReadsRemainCoherent(t *testing.T) {
	service := newContractFakeService(t)
	started := startAsyncByRequestID(t, service, "req-js-awaiting-001")
	sessionID := started.SessionID
	ctx := context.Background()

	const readers = 12
	var wg sync.WaitGroup
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := 0; round < 40; round++ {
				if _, err := service.GetSession(ctx, sessionID); err != nil {
					t.Errorf("GetSession: %v", err)
					return
				}
				if _, err := service.GetResult(ctx, sessionID, ResultRequest{Mode: ResultModeFinal}); err != nil {
					t.Errorf("GetResult: %v", err)
					return
				}
				if _, err := service.ListDispatches(ctx, sessionID); err != nil {
					t.Errorf("ListDispatches: %v", err)
					return
				}
				if _, err := service.ListArtifacts(ctx, sessionID); err != nil {
					t.Errorf("ListArtifacts: %v", err)
					return
				}
				if _, err := service.ReadEvents(ctx, sessionID, EventReconnectRequest{}); err != nil {
					t.Errorf("ReadEvents: %v", err)
					return
				}
			}
		}()
	}

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			requestPrefix := fmt.Sprintf("concurrent-control-%d", index)
			if _, err := service.Approve(ctx, sessionID, ApproveRequest{
				ControlRequest: ControlRequest{RequestID: requestPrefix + "-approve"},
			}); err != nil {
				var controlErr *ControlError
				if !errors.As(err, &controlErr) {
					t.Errorf("Approve: %v", err)
				}
			}
			if _, err := service.Pause(ctx, sessionID, ControlRequest{RequestID: requestPrefix + "-pause"}); err != nil {
				var controlErr *ControlError
				if !errors.As(err, &controlErr) {
					t.Errorf("Pause: %v", err)
				}
			}
			if _, err := service.Resume(ctx, sessionID, ControlRequest{RequestID: requestPrefix + "-resume"}); err != nil {
				var controlErr *ControlError
				if !errors.As(err, &controlErr) {
					t.Errorf("Resume: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
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

// pkgmaintcheck:ignore-cyclomatic-complexity this projection test keeps result, dispatch, and artifact coherence assertions together.
// pkgmaintcheck:ignore-function-lines this projection test keeps result, dispatch, and artifact coherence assertions together.
// backendsizecheck:ignore-function this projection test keeps result, dispatch, and artifact coherence assertions together.
func TestFakeService_ResultDispatchArtifact_ReadProjectionsCoherent(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")
	startAsyncByRequestID(t, service, "req-petri-run-001")
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")
	startAsyncByRequestID(t, service, "req-js-interrupted-001")

	t.Run("success final result and artifacts", func(t *testing.T) {
		sessionID := "dur-sess-petri-success-001"
		session, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		result, err := service.GetResult(context.Background(), sessionID, ResultRequest{
			Mode:             ResultModeFinal,
			IncludeArtifacts: true,
		})
		if err != nil {
			t.Fatalf("GetResult: %v", err)
		}
		if result.ResultStatus != ResultStatusFinal {
			t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
		}
		if len(result.PrimaryResult) == 0 {
			t.Fatal("primaryResult missing")
		}
		if len(result.ArtifactRefs) != 1 || result.ArtifactRefs[0].ID != "art-petri-final-001" {
			t.Fatalf("artifactRefs = %#v", result.ArtifactRefs)
		}
		if err := ValidateResultMatchesSessionRead(session, result); err != nil {
			t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
		}

		dispatches, err := service.ListDispatches(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("ListDispatches: %v", err)
		}
		if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].ID != "disp-petri-success-001" {
			t.Fatalf("dispatches = %#v", dispatches.Dispatches)
		}
		if len(dispatches.Dispatches[0].OutputArtifactIDs) != 1 || dispatches.Dispatches[0].OutputArtifactIDs[0] != "art-petri-final-001" {
			t.Fatalf("outputArtifactIds = %#v", dispatches.Dispatches[0].OutputArtifactIDs)
		}

		dispatchDetail, err := service.GetDispatch(context.Background(), sessionID, "disp-petri-success-001")
		if err != nil {
			t.Fatalf("GetDispatch: %v", err)
		}
		if dispatchDetail.Petri == nil || dispatchDetail.Petri.TransitionID == "" {
			t.Fatalf("petri projection missing: %#v", dispatchDetail.Petri)
		}
		if err := ValidateDispatchDetailMatchesListSummary(dispatchDetail, dispatches.Dispatches[0]); err != nil {
			t.Fatalf("ValidateDispatchDetailMatchesListSummary: %v", err)
		}

		artifacts, err := service.ListArtifacts(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("ListArtifacts: %v", err)
		}
		if len(artifacts.Artifacts) != 1 || artifacts.Artifacts[0].ID != "art-petri-final-001" {
			t.Fatalf("artifacts = %#v", artifacts.Artifacts)
		}
		if artifacts.Artifacts[0].RetrievalRef == nil || artifacts.Artifacts[0].RetrievalRef.Href == "" {
			t.Fatalf("retrievalRef missing: %#v", artifacts.Artifacts[0].RetrievalRef)
		}
		if artifacts.Artifacts[0].DispatchID != "disp-petri-success-001" {
			t.Fatalf("dispatchId = %q, want disp-petri-success-001", artifacts.Artifacts[0].DispatchID)
		}

		artifactDetail, err := service.GetArtifact(context.Background(), sessionID, "art-petri-final-001")
		if err != nil {
			t.Fatalf("GetArtifact: %v", err)
		}
		if len(artifactDetail.Content) == 0 {
			t.Fatal("artifact content missing")
		}
		if err := ValidateArtifactDetailMatchesListSummary(artifactDetail, artifacts.Artifacts[0]); err != nil {
			t.Fatalf("ValidateArtifactDetailMatchesListSummary: %v", err)
		}
	})

	t.Run("running session reports not-ready final result", func(t *testing.T) {
		sessionID := "dur-sess-petri-run-001"
		result, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModeFinal})
		if err != nil {
			t.Fatalf("GetResult: %v", err)
		}
		if result.ResultStatus != ResultStatusNotReady {
			t.Fatalf("resultStatus = %q, want NOT_READY", result.ResultStatus)
		}
		if len(result.PrimaryResult) != 0 {
			t.Fatal("primaryResult should be omitted for running session")
		}
		if result.Availability == nil {
			t.Fatal("availability missing")
		}

		artifacts, err := service.ListArtifacts(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("ListArtifacts: %v", err)
		}
		if len(artifacts.Artifacts) != 0 {
			t.Fatalf("artifacts = %#v, want none", artifacts.Artifacts)
		}
	})

	t.Run("failed-with-partial and interrupted partial versus final modes", func(t *testing.T) {
		failedPartial, err := service.GetResult(context.Background(), "dur-sess-js-failed-partial-001", ResultRequest{Mode: ResultModePartial})
		if err != nil {
			t.Fatalf("GetResult failed partial: %v", err)
		}
		if failedPartial.ResultStatus != ResultStatusFailedWithPartial || len(failedPartial.PrimaryResult) == 0 {
			t.Fatalf("failed partial result = %#v", failedPartial)
		}

		failedFinal, err := service.GetResult(context.Background(), "dur-sess-js-failed-partial-001", ResultRequest{Mode: ResultModeFinal})
		if err != nil {
			t.Fatalf("GetResult failed final: %v", err)
		}
		if failedFinal.ResultStatus != ResultStatusFailedWithPartial || len(failedFinal.PrimaryResult) == 0 {
			t.Fatalf("failed final result = %#v", failedFinal)
		}

		interruptedPartial, err := service.GetResult(context.Background(), "dur-sess-js-interrupted-001", ResultRequest{Mode: ResultModePartial})
		if err != nil {
			t.Fatalf("GetResult interrupted partial: %v", err)
		}
		if interruptedPartial.ResultStatus != ResultStatusPartial || len(interruptedPartial.PrimaryResult) == 0 {
			t.Fatalf("interrupted partial result = %#v", interruptedPartial)
		}

		interruptedFinal, err := service.GetResult(context.Background(), "dur-sess-js-interrupted-001", ResultRequest{Mode: ResultModeFinal})
		if err != nil {
			t.Fatalf("GetResult interrupted final: %v", err)
		}
		if interruptedFinal.ResultStatus != ResultStatusNotReady {
			t.Fatalf("interrupted final status = %q, want NOT_READY", interruptedFinal.ResultStatus)
		}
		if len(interruptedFinal.PrimaryResult) != 0 {
			t.Fatal("interrupted final primaryResult should be omitted")
		}
		if interruptedFinal.Availability == nil {
			t.Fatal("interrupted final availability missing")
		}
	})

	t.Run("dispatch failure and interruption metadata", func(t *testing.T) {
		failedDispatches, err := service.ListDispatches(context.Background(), "dur-sess-js-failed-partial-001")
		if err != nil {
			t.Fatalf("ListDispatches failed: %v", err)
		}
		if len(failedDispatches.Dispatches) != 2 {
			t.Fatalf("failed dispatches = %#v", failedDispatches.Dispatches)
		}
		failedDetail, err := service.GetDispatch(context.Background(), "dur-sess-js-failed-partial-001", "disp-js-fail-002")
		if err != nil {
			t.Fatalf("GetDispatch failed: %v", err)
		}
		if failedDetail.Status != DispatchStatusFailed {
			t.Fatalf("status = %q, want FAILED", failedDetail.Status)
		}
		if failedDetail.FailureDetail == nil || failedDetail.FailureDetail.Reason != "VERIFY_ASSERTION_FAILED" {
			t.Fatalf("failureDetail = %#v", failedDetail.FailureDetail)
		}
		if failedDetail.JavaScript == nil || failedDetail.JavaScript.TaskKind != "VERIFY" {
			t.Fatalf("javascript projection = %#v", failedDetail.JavaScript)
		}
		if err := ValidateDispatchDetailMatchesListSummary(failedDetail, failedDispatches.Dispatches[1]); err != nil {
			t.Fatalf("ValidateDispatchDetailMatchesListSummary: %v", err)
		}

		interruptedDispatches, err := service.ListDispatches(context.Background(), "dur-sess-js-interrupted-001")
		if err != nil {
			t.Fatalf("ListDispatches interrupted: %v", err)
		}
		if len(interruptedDispatches.Dispatches) != 2 {
			t.Fatalf("interrupted dispatches = %#v", interruptedDispatches.Dispatches)
		}
		interruptedDetail, err := service.GetDispatch(context.Background(), "dur-sess-js-interrupted-001", "disp-js-interrupted-002")
		if err != nil {
			t.Fatalf("GetDispatch interrupted: %v", err)
		}
		if interruptedDetail.Status != DispatchStatusCanceled {
			t.Fatalf("status = %q, want CANCELED", interruptedDetail.Status)
		}
		if err := ValidateDispatchDetailMatchesListSummary(interruptedDetail, interruptedDispatches.Dispatches[1]); err != nil {
			t.Fatalf("ValidateDispatchDetailMatchesListSummary: %v", err)
		}
	})

	t.Run("missing dispatch and artifact return not-found", func(t *testing.T) {
		_, err := service.GetDispatch(context.Background(), "dur-sess-petri-success-001", "missing-dispatch")
		if !errors.Is(err, ErrDispatchNotFound) {
			t.Fatalf("GetDispatch error = %v, want ErrDispatchNotFound", err)
		}
		_, err = service.GetArtifact(context.Background(), "dur-sess-petri-success-001", "missing-artifact")
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Fatalf("GetArtifact error = %v, want ErrArtifactNotFound", err)
		}
	})
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
