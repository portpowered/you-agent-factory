package factorysessionexecution_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestRuntimeService_StartAsync_IdempotentReplay(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")
	req := factorysessionexecution.StartRequest{
		RequestID: "req-runtime-idempotent-async",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
	}

	first, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartAsync: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		read, err := service.GetSession(context.Background(), first.SessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if read.Status == factorysessionexecution.LifecycleStatusSucceeded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session still %q, want eventual SUCCEEDED before replay assertion", read.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}

	second, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("second StartAsync: %v", err)
	}
	assertAsyncStartReplayEqual(t, first, second)

	beforeCount := runtimeSessionCount(t, service)
	conflict := req
	conflict.Args["subject"] = "different"
	_, err = service.StartAsync(context.Background(), conflict)
	if !errors.Is(err, factorysessionexecution.ErrExecutionRequestIDConflict) {
		t.Fatalf("conflict error = %v, want ErrExecutionRequestIDConflict", err)
	}
	if after := runtimeSessionCount(t, service); after != beforeCount {
		t.Fatalf("session count = %d, want %d after conflict", after, beforeCount)
	}
}

func TestRuntimeService_StartSync_IdempotentReplayCompleted(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")
	req := factorysessionexecution.StartRequest{
		RequestID: "req-runtime-idempotent-sync-complete",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
	}

	first, err := service.StartSync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartSync: %v", err)
	}
	if first.SyncOutcome != factorysessionexecution.SyncOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", first.SyncOutcome)
	}

	second, err := service.StartSync(context.Background(), req)
	if err != nil {
		t.Fatalf("second StartSync: %v", err)
	}
	assertSyncStartReplayEqual(t, first, second)

	beforeCount := runtimeSessionCount(t, service)
	conflict := req
	conflict.Args["prefix"] = "conflict"
	_, err = service.StartSync(context.Background(), conflict)
	if !errors.Is(err, factorysessionexecution.ErrExecutionRequestIDConflict) {
		t.Fatalf("conflict error = %v, want ErrExecutionRequestIDConflict", err)
	}
	if after := runtimeSessionCount(t, service); after != beforeCount {
		t.Fatalf("session count = %d, want %d after conflict", after, beforeCount)
	}
}

func TestRuntimeService_StartSync_IdempotentReplayTimeout(t *testing.T) {
	t.Run("busy-loop immediate replay", func(t *testing.T) {
		_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")
		timeoutMillis := int64(25)
		req := factorysessionexecution.StartRequest{
			RequestID: "req-runtime-idempotent-sync-timeout-busy",
			Source: factorysessionexecution.Source{
				Kind:         workflowsource.KindWorkflowName,
				WorkflowName: "busy-loop",
			},
			Wait: &factorysessionexecution.WaitOptions{
				TimeoutMillis:   &timeoutMillis,
				CancelOnTimeout: false,
			},
		}

		first, err := service.StartSync(context.Background(), req)
		if err != nil {
			t.Fatalf("first StartSync: %v", err)
		}
		if first.SyncOutcome != factorysessionexecution.SyncOutcomeTimedOut || !first.TimedOut {
			t.Fatalf("timeout response = %#v", first)
		}
		if first.Status != string(factorysessionexecution.LifecycleStatusRunning) {
			t.Fatalf("status = %q, want RUNNING", first.Status)
		}

		second, err := service.StartSync(context.Background(), req)
		if err != nil {
			t.Fatalf("second StartSync: %v", err)
		}
		assertSyncStartReplayEqual(t, first, second)
	})

	t.Run("slow-final replay stays timeout-shaped after completion", func(t *testing.T) {
		projectRoot := setupSlowFinalWorkflowFixture(t)
		service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
			StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
		})
		timeoutMillis := int64(20)
		req := factorysessionexecution.StartRequest{
			RequestID: "req-runtime-idempotent-sync-timeout-slow",
			Source: factorysessionexecution.Source{
				Kind:         workflowsource.KindWorkflowName,
				WorkflowName: "slow-final",
			},
			Wait: &factorysessionexecution.WaitOptions{
				TimeoutMillis: &timeoutMillis,
			},
		}

		first, err := service.StartSync(context.Background(), req)
		if err != nil {
			t.Fatalf("first StartSync: %v", err)
		}
		if first.SyncOutcome != factorysessionexecution.SyncOutcomeTimedOut || !first.TimedOut {
			t.Fatalf("timeout response = %#v", first)
		}

		deadline := time.Now().Add(15 * time.Second)
		for {
			read, err := service.GetSession(context.Background(), first.SessionID)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if read.Status == factorysessionexecution.LifecycleStatusSucceeded {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("session still %q, want eventual SUCCEEDED", read.Status)
			}
			time.Sleep(10 * time.Millisecond)
		}

		replay, err := service.StartSync(context.Background(), req)
		if err != nil {
			t.Fatalf("replay StartSync after completion: %v", err)
		}
		assertSyncStartReplayEqual(t, first, replay)

		beforeCount := runtimeSessionCount(t, service)
		conflict := req
		conflict.Args = map[string]any{"marker": "conflict"}
		_, err = service.StartSync(context.Background(), conflict)
		if !errors.Is(err, factorysessionexecution.ErrExecutionRequestIDConflict) {
			t.Fatalf("conflict error = %v, want ErrExecutionRequestIDConflict", err)
		}
		if after := runtimeSessionCount(t, service); after != beforeCount {
			t.Fatalf("session count = %d, want %d after conflict", after, beforeCount)
		}
	})
}

func assertAsyncStartReplayEqual(t *testing.T, first, second factorysessionexecution.AsyncStartResult) {
	t.Helper()
	if second.SessionID != first.SessionID {
		t.Fatalf("replay sessionId = %q, want %q", second.SessionID, first.SessionID)
	}
	if second.Status != first.Status {
		t.Fatalf("replay status = %q, want %q", second.Status, first.Status)
	}
	if second.SourceHash != first.SourceHash {
		t.Fatalf("replay sourceHash = %q, want %q", second.SourceHash, first.SourceHash)
	}
	if second.Policy.EffectiveHash != first.Policy.EffectiveHash {
		t.Fatalf("replay policy hash = %q, want %q", second.Policy.EffectiveHash, first.Policy.EffectiveHash)
	}
	if second.Links != first.Links {
		t.Fatalf("replay links = %#v, want %#v", second.Links, first.Links)
	}
}

func assertSyncStartReplayEqual(t *testing.T, first, second factorysessionexecution.SyncStartResult) {
	t.Helper()
	assertAsyncStartReplayEqual(t, first.AsyncStartResult, second.AsyncStartResult)
	if second.SyncOutcome != first.SyncOutcome {
		t.Fatalf("replay syncOutcome = %q, want %q", second.SyncOutcome, first.SyncOutcome)
	}
	if second.TimedOut != first.TimedOut {
		t.Fatalf("replay timedOut = %v, want %v", second.TimedOut, first.TimedOut)
	}
	if second.SessionCanceledByTimeout != first.SessionCanceledByTimeout {
		t.Fatalf("replay sessionCanceledByTimeout = %v, want %v", second.SessionCanceledByTimeout, first.SessionCanceledByTimeout)
	}
	if !jsonEqualRaw(t, first.Result, second.Result) {
		t.Fatalf("replay result = %s, want %s", string(second.Result), string(first.Result))
	}
}

func jsonEqualRaw(t *testing.T, first, second json.RawMessage) bool {
	t.Helper()
	if len(first) == 0 && len(second) == 0 {
		return true
	}
	var firstValue any
	var secondValue any
	if err := json.Unmarshal(first, &firstValue); err != nil {
		t.Fatalf("unmarshal first result: %v", err)
	}
	if err := json.Unmarshal(second, &secondValue); err != nil {
		t.Fatalf("unmarshal second result: %v", err)
	}
	return reflect.DeepEqual(firstValue, secondValue)
}

func runtimeSessionCount(t *testing.T, service *factorysessionexecution.RuntimeService) int {
	t.Helper()
	listed, err := service.ListSessions(context.Background(), factorysessionexecution.ListSessionsRequest{
		Scope: factorysessionexecution.SessionListScopeAll,
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	return len(listed.LiveSessions)
}

func int64Ptr(value int64) *int64 {
	return &value
}
