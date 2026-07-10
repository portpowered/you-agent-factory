package invocations

import (
	"context"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestSessionOwnerWait_ReturnsSubmittedTerminalContent(t *testing.T) {
	observation := completedSessionInvocationObservation("request-1", "trace-1", "completed output")
	result := waitForSessionOwnerObservation(t, observation, nil)

	assertSessionOwnerEqual(t, "status", result.Status, factoryapi.InvocationTerminalStatusCompleted)
	assertSessionOwnerEqual(t, "request ID", result.RequestID, "request-1")
	assertSessionOwnerEqual(t, "trace ID", result.TraceID, "trace-1")
	assertSessionOwnerEqual(t, "primary result", result.PrimaryResult[0].Text, "completed output")
}

func TestSessionOwnerWait_ExplicitPolicyIgnoresUnrelatedMatchingWork(t *testing.T) {
	state := invocationWorldStateFixture()
	root := invocationWorkItem("work-root", "task", "draft", "root", "task:draft")
	summary := invocationWorkItem("work-summary", "summary", "complete", "wanted", "summary:complete")
	unrelated := invocationWorkItem("work-unrelated", "summary", "complete", "unrelated", "summary:complete")
	recordInvocationSubmittedWork(&state, 1, "request-1", root)
	recordInvocationDispatchOutput(&state, 2, "dispatch-1", []interfaces.FactoryWorkItem{root}, summary)
	state.TerminalWorkByID[summary.ID] = interfaces.FactoryTerminalWork{WorkItem: summary, Status: "TERMINAL"}
	state.TerminalWorkByID[unrelated.ID] = interfaces.FactoryTerminalWork{WorkItem: unrelated, Status: "TERMINAL"}

	result := waitForSessionOwnerObservation(t, SessionInvocationObservation{WorldState: state}, &interfaces.InvocationReturnConfig{
		Policy: invocationReturnPolicyExplicit, WorkTypeName: "summary", TerminalState: "complete",
	})

	assertSessionOwnerEqual(t, "status", result.Status, factoryapi.InvocationTerminalStatusCompleted)
	assertSessionOwnerEqual(t, "primary result", result.PrimaryResult[0].Text, summary.Content[0].Text)
}

func TestSessionOwnerWait_MapsTimeoutAndCancellation(t *testing.T) {
	tests := []struct {
		name       string
		waitErr    error
		wantStatus factoryapi.InvocationTerminalStatus
		wantCode   string
	}{
		{name: "timeout", waitErr: context.DeadlineExceeded, wantStatus: factoryapi.InvocationTerminalStatusTimedOut, wantCode: string(factoryapi.INVOCATIONTIMEDOUT)},
		{name: "cancellation", waitErr: context.Canceled, wantStatus: factoryapi.InvocationTerminalStatusCanceled, wantCode: string(factoryapi.INVOCATIONCANCELED)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := NewSessionOwner(SessionOwnerDependencies{
				Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
					return activeSessionInvocationObservation(), nil
				},
				WaitNext: func(context.Context) error { return tt.waitErr },
			})
			result, err := owner.waitForResult(context.Background(), "session-1", sessionWaitInput(nil))
			if err != nil {
				t.Fatalf("waitForResult: %v", err)
			}
			assertSessionOwnerEqual(t, "status", result.Status, tt.wantStatus)
			assertSessionOwnerEqual(t, "error code", result.ErrorCode, tt.wantCode)
			assertSessionOwnerEqual(t, "request ID", result.RequestID, "request-1")
			assertSessionOwnerEqual(t, "trace ID", result.TraceID, "trace-1")
		})
	}
}

func TestSessionOwnerWait_ConfiguredTimeoutReachesInjectedWaitBoundary(t *testing.T) {
	timeoutMillis := int64(250)
	owner := NewSessionOwner(SessionOwnerDependencies{
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return activeSessionInvocationObservation(), nil
		},
		WaitNext: func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("wait context has no configured deadline")
			}
			return context.DeadlineExceeded
		},
	})
	input := sessionWaitInput(nil)
	input.TimeoutMillis = &timeoutMillis
	result, err := owner.waitForResult(context.Background(), "session-1", input)
	if err != nil {
		t.Fatalf("waitForResult: %v", err)
	}
	assertSessionOwnerEqual(t, "status", result.Status, factoryapi.InvocationTerminalStatusTimedOut)
}

func TestSessionOwnerWait_PreservesTerminalFailureClassifications(t *testing.T) {
	tests := []struct {
		name        string
		observation SessionInvocationObservation
		wantCode    PrimaryResultErrorCode
	}{
		{name: "blocked", observation: classifiedObservation(PrimaryResultErrorCodeBlocked, "blocked"), wantCode: PrimaryResultErrorCodeBlocked},
		{name: "needs human", observation: classifiedObservation(PrimaryResultErrorCodeNeedsHuman, "needs-human"), wantCode: PrimaryResultErrorCodeNeedsHuman},
		{name: "paused", observation: pausedSessionInvocationObservation(), wantCode: PrimaryResultErrorCodePaused},
		{name: "interrupted", observation: interruptedSessionInvocationObservation(), wantCode: PrimaryResultErrorCodeInterrupted},
		{name: "failed", observation: failedSessionInvocationObservation(), wantCode: PrimaryResultErrorCodeFailed},
		{name: "unresolved", observation: stoppedSessionInvocationObservation(), wantCode: PrimaryResultErrorCodeUnresolved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := waitForSessionOwnerObservation(t, tt.observation, nil)
			assertSessionOwnerEqual(t, "status", result.Status, factoryapi.InvocationTerminalStatusFailed)
			assertSessionOwnerEqual(t, "error code", result.ErrorCode, string(tt.wantCode))
			if result.Message == "" {
				t.Fatal("message is empty")
			}
		})
	}
}

func waitForSessionOwnerObservation(t *testing.T, observation SessionInvocationObservation, policy *interfaces.InvocationReturnConfig) FactoryInvocationResult {
	t.Helper()
	owner := NewSessionOwner(SessionOwnerDependencies{Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
		return observation, nil
	}})
	result, err := owner.waitForResult(context.Background(), "session-1", sessionWaitInput(policy))
	if err != nil {
		t.Fatalf("waitForResult: %v", err)
	}
	return result
}

func sessionWaitInput(policy *interfaces.InvocationReturnConfig) SessionInvocationWaitInput {
	return SessionInvocationWaitInput{RequestID: "request-1", TraceID: "trace-1", InvocationReturn: policy}
}

func activeSessionInvocationObservation() SessionInvocationObservation {
	observation := stoppedSessionInvocationObservation()
	observation.ActiveWork = true
	return observation
}

func stoppedSessionInvocationObservation() SessionInvocationObservation {
	state := invocationWorldStateFixture()
	root := invocationWorkItem("work-root", "goal", "init", "Goal", "goal:init")
	recordInvocationSubmittedWork(&state, 1, "request-1", root)
	return SessionInvocationObservation{WorldState: state}
}

func classifiedObservation(code PrimaryResultErrorCode, state string) SessionInvocationObservation {
	observation := stoppedSessionInvocationObservation()
	observation.MissingPrimaryResult = ClassifyMissingPrimaryResultWorkItem(
		"request-1", nil, invocationWorkItem("work-root", "goal", state, "Goal", "goal:"+state), "session-1",
	)
	if observation.MissingPrimaryResult == nil || observation.MissingPrimaryResult.Code != code {
		panic("invalid classified observation fixture")
	}
	return observation
}

func pausedSessionInvocationObservation() SessionInvocationObservation {
	observation := stoppedSessionInvocationObservation()
	observation.FactoryState = string(interfaces.FactoryStatePaused)
	return observation
}

func interruptedSessionInvocationObservation() SessionInvocationObservation {
	observation := stoppedSessionInvocationObservation()
	root := observation.WorldState.WorkRequestsByID["request-1"].WorkItems[0]
	observation.WorldState.WorkItemsByID[root.ID] = root
	observation.WorldState.JavaScriptRuntime = &interfaces.FactorySessionJavaScriptRuntimeState{Dispatches: []interfaces.FactorySessionDispatchState{{
		ID: "dispatch-1", Status: "INTERRUPTED", RelatedWorkIDs: []string{root.ID},
	}}}
	return observation
}

func failedSessionInvocationObservation() SessionInvocationObservation {
	observation := stoppedSessionInvocationObservation()
	failed := invocationWorkItem("work-root", "goal", "failed", "Goal", "goal:failed")
	observation.WorldState.FailedWorkItemsByID[failed.ID] = failed
	return observation
}
