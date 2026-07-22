package work_test

import (
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestResolvePrimaryResult_SubmittedWorkTerminalFallbackReturnsSubmittedTerminalContent(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "task", "draft", "root", "task:init")
	rootTerminal := invocationWorkItem("work-root", "task", "complete", "root", "task:complete")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	recordInvocationDispatchOutput(&state, 2, "dispatch-root", []work.FactoryWorkItem{rootInitial}, rootTerminal)
	state.TerminalWorkByID[rootTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: rootTerminal, Status: "TERMINAL"}

	got, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if err != nil {
		t.Fatalf("work.ResolvePrimaryResult: %v", err)
	}

	assertPrimaryResultSelection(t, got, work.ReturnPolicySubmittedWorkTerminal, rootTerminal)
}

func TestResolvePrimaryResult_SubmittedWorkTerminalReturnsAcceptedResponseContent(t *testing.T) {
	state := invocationWorldStateFixture()
	requestContent := []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "submitted request text",
	}}
	responseContent := []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "accepted workstation response",
	}}
	rootInitial := invocationWorkItem("work-root", "task", "draft", "root", "task:init")
	rootInitial.Content = requestContent
	rootTerminal := invocationWorkItem("work-root", "task", "complete", "root", "task:complete")
	rootTerminal.Content = responseContent
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	recordInvocationDispatchOutput(&state, 2, "dispatch-root", []work.FactoryWorkItem{rootInitial}, rootTerminal)
	state.TerminalWorkByID[rootTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: rootTerminal, Status: "TERMINAL"}

	got, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if err != nil {
		t.Fatalf("work.ResolvePrimaryResult: %v", err)
	}
	if len(got.PrimaryResult) != 1 || got.PrimaryResult[0].Text != "accepted workstation response" {
		t.Fatalf("primary result = %#v, want accepted response content", got.PrimaryResult)
	}
	if got.PrimaryResult[0].Text == requestContent[0].Text {
		t.Fatalf("primary result echoed submitted request text")
	}
}

func TestResolvePrimaryResult_ExplicitPolicyReturnsConfiguredTerminalContentInInvocationScope(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "task", "draft", "root", "task:init")
	rootTerminal := invocationWorkItem("work-root", "task", "complete", "root", "task:complete")
	summaryTerminal := invocationWorkItem("work-summary", "summary", "complete", "summary", "summary:complete")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	recordInvocationDispatchOutput(&state, 2, "dispatch-root", []work.FactoryWorkItem{rootInitial}, rootTerminal, summaryTerminal)
	state.TerminalWorkByID[rootTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: rootTerminal, Status: "TERMINAL"}
	state.TerminalWorkByID[summaryTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: summaryTerminal, Status: "TERMINAL"}

	got, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID: "request-1",
		InvocationReturn: &interfaces.InvocationReturnConfig{
			Policy:        work.ReturnPolicyExplicit,
			WorkTypeName:  "summary",
			TerminalState: "complete",
			WorkName:      "summary",
		},
		WorldState: state,
	})
	if err != nil {
		t.Fatalf("work.ResolvePrimaryResult: %v", err)
	}

	assertPrimaryResultSelection(t, got, work.ReturnPolicyExplicit, summaryTerminal)
}

func TestResolvePrimaryResult_ExplicitPolicyFallsBackToInvocationTraceWhenScopeWorkIDChanges(t *testing.T) {
	state := invocationWorldStateFixture()
	traceID := "trace-goal-invocation"
	rootInitial := invocationWorkItem("work-root", "goal", "init", "root", "goal:init")
	rootInitial.TraceID = traceID
	goalComplete := invocationWorkItem("work-derived-complete", "goal", "complete", "root", "goal:complete")
	goalComplete.TraceID = traceID
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.WorkRequestsByID["request-1"] = interfaces.WorkRequestPayload{
		RequestID: "request-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		TraceID:   traceID,
		WorkItems: []work.FactoryWorkItem{rootInitial},
	}
	state.TerminalWorkByID[goalComplete.ID] = interfaces.FactoryTerminalWork{WorkItem: goalComplete, Status: "TERMINAL"}

	got, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID: "request-1",
		InvocationReturn: &interfaces.InvocationReturnConfig{
			Policy:        work.ReturnPolicyExplicit,
			WorkTypeName:  "goal",
			TerminalState: "complete",
		},
		WorldState: state,
	})
	if err != nil {
		t.Fatalf("work.ResolvePrimaryResult: %v", err)
	}

	assertPrimaryResultSelection(t, got, work.ReturnPolicyExplicit, goalComplete)
}

func TestResolvePrimaryResult_FallbackDoesNotCrossTalkAcrossInvocationScopes(t *testing.T) {
	state := invocationWorldStateFixture()
	requestOneRoot := invocationWorkItem("work-root-1", "task", "draft", "root-1", "task:init")
	requestOneTerminal := invocationWorkItem("work-root-1", "task", "complete", "root-1", "task:complete")
	requestTwoRoot := invocationWorkItem("work-root-2", "task", "draft", "root-2", "task:init")
	requestTwoTerminal := invocationWorkItem("work-root-2", "task", "complete", "root-2", "task:complete")
	recordInvocationSubmittedWork(&state, 1, "request-1", requestOneRoot)
	recordInvocationSubmittedWork(&state, 2, "request-2", requestTwoRoot)
	recordInvocationDispatchOutput(&state, 3, "dispatch-root-1", []work.FactoryWorkItem{requestOneRoot}, requestOneTerminal)
	recordInvocationDispatchOutput(&state, 4, "dispatch-root-2", []work.FactoryWorkItem{requestTwoRoot}, requestTwoTerminal)
	state.TerminalWorkByID[requestOneTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: requestOneTerminal, Status: "TERMINAL"}
	state.TerminalWorkByID[requestTwoTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: requestTwoTerminal, Status: "TERMINAL"}

	got, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if err != nil {
		t.Fatalf("work.ResolvePrimaryResult: %v", err)
	}

	assertPrimaryResultSelection(t, got, work.ReturnPolicySubmittedWorkTerminal, requestOneTerminal)
}

func TestResolvePrimaryResult_FallbackPrefersSubmittedLogicalWorkOverFanoutSibling(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "task", "draft", "root", "task:init")
	rootTerminal := invocationWorkItem("work-root", "task", "complete", "root", "task:complete")
	fanoutTerminal := invocationWorkItem("work-fanout", "summary", "complete", "fanout", "summary:complete")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	recordInvocationDispatchOutput(&state, 2, "dispatch-root", []work.FactoryWorkItem{rootInitial}, rootTerminal, fanoutTerminal)
	state.TerminalWorkByID[rootTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: rootTerminal, Status: "TERMINAL"}
	state.TerminalWorkByID[fanoutTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: fanoutTerminal, Status: "TERMINAL"}

	got, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if err != nil {
		t.Fatalf("work.ResolvePrimaryResult: %v", err)
	}

	assertPrimaryResultSelection(t, got, work.ReturnPolicySubmittedWorkTerminal, rootTerminal)
}

func TestResolvePrimaryResult_UnresolvedWhenNoPrimaryOutputExists(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "task", "draft", "root", "task:init")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)

	_, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})

	var selectionErr *work.PrimaryResultError
	if !errors.As(err, &selectionErr) {
		t.Fatalf("error = %v, want work.PrimaryResultError", err)
	}
	if selectionErr.Code != work.PrimaryResultErrorCodeUnresolved {
		t.Fatalf("code = %q, want %q", selectionErr.Code, work.PrimaryResultErrorCodeUnresolved)
	}
	if selectionErr.Policy != work.ReturnPolicySubmittedWorkTerminal {
		t.Fatalf("policy = %q, want %q", selectionErr.Policy, work.ReturnPolicySubmittedWorkTerminal)
	}
}

func TestClassifyMissingPrimaryResult_ReturnsBlockedForScopedWorkItem(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "goal", "init", "Blocked goal", "goal:init")
	rootBlocked := invocationWorkItem("work-root", "goal", "blocked", "Blocked goal", "goal:blocked")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.WorkItemsByID[rootBlocked.ID] = rootBlocked

	got, ok := work.ClassifyMissingPrimaryResult(work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected blocked classification")
	}
	if got.Code != work.PrimaryResultErrorCodeBlocked {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodeBlocked)
	}
	if got.Message != `invocation blocked: work "Blocked goal" is waiting in state "goal:blocked"` {
		t.Fatalf("message = %q", got.Message)
	}
	assertInvocationFailureContext(t, got.Context, work.InvocationFailureContext{
		WorkID:    "work-root",
		WorkName:  "Blocked goal",
		WorkState: "goal:blocked",
	})
}

func TestClassifyMissingPrimaryResult_ReturnsNeedsHumanForScopedWorkItem(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "goal", "init", "Needs operator input", "goal:init")
	rootNeedsHuman := invocationWorkItem("work-root", "goal", "needs-human", "Needs operator input", "goal:needs-human")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.WorkItemsByID[rootNeedsHuman.ID] = rootNeedsHuman

	got, ok := work.ClassifyMissingPrimaryResult(work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected needs-human classification")
	}
	if got.Code != work.PrimaryResultErrorCodeNeedsHuman {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodeNeedsHuman)
	}
	if got.Message != `invocation needs human input: work "Needs operator input" is waiting in state "goal:needs-human"` {
		t.Fatalf("message = %q", got.Message)
	}
	assertInvocationFailureContext(t, got.Context, work.InvocationFailureContext{
		WorkID:    "work-root",
		WorkName:  "Needs operator input",
		WorkState: "goal:needs-human",
	})
}

func TestResolvePrimaryResult_PartialTimeoutLikeFailedWorkDoesNotResolvePrimaryResult(t *testing.T) {
	state := invocationWorldStateFixture()
	partialText := "partial answer before timeout"
	rootInitial := invocationWorkItem("work-root", "task", "draft", "root", "task:init")
	rootFailed := invocationWorkItem("work-root", "task", "failed", "root", "task:failed")
	rootFailed.Content = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: partialText,
	}}
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.TerminalWorkByID[rootFailed.ID] = interfaces.FactoryTerminalWork{WorkItem: rootFailed, Status: "FAILED"}
	state.FailedWorkItemsByID[rootFailed.ID] = rootFailed

	_, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if err == nil {
		t.Fatal("work.ResolvePrimaryResult() error = nil, want unresolved primary result for timeout-like failed work")
	}
	primaryErr, ok := err.(*work.PrimaryResultError)
	if !ok {
		t.Fatalf("error = %T, want *work.PrimaryResultError", err)
	}
	if primaryErr.Code != work.PrimaryResultErrorCodeUnresolved {
		t.Fatalf("code = %q, want %q", primaryErr.Code, work.PrimaryResultErrorCodeUnresolved)
	}

	got, ok := work.ClassifyFailedInvocation("session-1", work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected failed classification for timeout-like partial capture")
	}
	if got.Code != work.PrimaryResultErrorCodeFailed {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodeFailed)
	}
}

func TestResolvePrimaryResult_FailedTerminalDoesNotReturnResponseShapedContent(t *testing.T) {
	state := invocationWorldStateFixture()
	requestContent := []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "submitted request text",
	}}
	responseContent := []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "worker response that must not become primary result",
	}}
	rootInitial := invocationWorkItem("work-root", "task", "draft", "root", "task:init")
	rootInitial.Content = requestContent
	rootFailed := invocationWorkItem("work-root", "task", "failed", "root", "task:failed")
	rootFailed.Content = responseContent
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.TerminalWorkByID[rootFailed.ID] = interfaces.FactoryTerminalWork{WorkItem: rootFailed, Status: "FAILED"}
	state.FailedWorkItemsByID[rootFailed.ID] = rootFailed

	_, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if err == nil {
		t.Fatal("work.ResolvePrimaryResult() error = nil, want unresolved primary result for failed terminal work")
	}
	primaryErr, ok := err.(*work.PrimaryResultError)
	if !ok {
		t.Fatalf("error = %T, want *work.PrimaryResultError", err)
	}
	if primaryErr.Code != work.PrimaryResultErrorCodeUnresolved {
		t.Fatalf("code = %q, want %q", primaryErr.Code, work.PrimaryResultErrorCodeUnresolved)
	}

	got, ok := work.ClassifyFailedInvocation("session-1", work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected failed classification")
	}
	if got.Code != work.PrimaryResultErrorCodeFailed {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodeFailed)
	}
}

func TestClassifyFailedInvocation_ReturnsFailedForScopedFailedWorkItem(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "goal", "init", "Failed goal", "goal:init")
	rootFailed := invocationWorkItem("work-root", "goal", "failed", "Failed goal", "goal:failed")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.FailedWorkItemsByID[rootFailed.ID] = rootFailed
	state.TerminalWorkByID[rootFailed.ID] = interfaces.FactoryTerminalWork{WorkItem: rootFailed, Status: "FAILED"}

	got, ok := work.ClassifyFailedInvocation("session-1", work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected failed classification")
	}
	if got.Code != work.PrimaryResultErrorCodeFailed {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodeFailed)
	}
	if got.Message != `invocation failed: work "Failed goal" reached failed state "goal:failed" before a primary result was available` {
		t.Fatalf("message = %q", got.Message)
	}
	assertInvocationFailureContext(t, got.Context, work.InvocationFailureContext{
		WorkID:    "work-root",
		WorkName:  "Failed goal",
		WorkState: "goal:failed",
	})
}

func TestClassifyFailedInvocation_MatchesFailedWorkBySubmittedTrace(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "goal", "init", "Failed goal", "goal:init")
	rootInitial.TraceID = "trace-shared"
	failedChild := invocationWorkItem("work-failed-child", "goal", "failed", "Failed goal child", "goal:failed")
	failedChild.TraceID = "trace-shared"
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.FailedWorkItemsByID[failedChild.ID] = failedChild

	got, ok := work.ClassifyFailedInvocation("session-1", work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected failed classification")
	}
	if got.Code != work.PrimaryResultErrorCodeFailed {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodeFailed)
	}
	if got.Message != `invocation failed: work "Failed goal child" reached failed state "goal:failed" before a primary result was available` {
		t.Fatalf("message = %q", got.Message)
	}
	assertInvocationFailureContext(t, got.Context, work.InvocationFailureContext{
		WorkID:    "work-failed-child",
		WorkName:  "Failed goal child",
		WorkState: "goal:failed",
	})
}

func TestClassifyFailedInvocation_MatchesFailedWorkByRequestStateChange(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "goal", "init", "Failed goal", "goal:init")
	failedChild := invocationWorkItem("work-failed-child", "goal", "failed", "Failed goal child", "goal:failed")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.WorkStateChangesByWorkID[failedChild.ID] = []interfaces.FactoryWorldWorkStateChangeRecord{{
		WorkID:       failedChild.ID,
		WorkTypeName: failedChild.WorkTypeID,
		ToState:      "failed",
		ToPlaceID:    failedChild.PlaceID,
		RequestID:    "request-1",
	}}
	state.FailedWorkItemsByID[failedChild.ID] = failedChild

	got, ok := work.ClassifyFailedInvocation("session-1", work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected failed classification")
	}
	if got.Code != work.PrimaryResultErrorCodeFailed {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodeFailed)
	}
	if got.Message != `invocation failed: work "Failed goal child" reached failed state "goal:failed" before a primary result was available` {
		t.Fatalf("message = %q", got.Message)
	}
	assertInvocationFailureContext(t, got.Context, work.InvocationFailureContext{
		WorkID:    "work-failed-child",
		WorkName:  "Failed goal child",
		WorkState: "goal:failed",
	})
}

func TestClassifyFailedInvocation_FallsBackToFailedSessionState(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "goal", "init", "Failed goal", "goal:init")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.FactoryState = string(interfaces.FactoryStateFailed)

	got, ok := work.ClassifyFailedInvocation("session-1", work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected failed classification")
	}
	if got.Code != work.PrimaryResultErrorCodeFailed {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodeFailed)
	}
	if got.Message != `invocation failed: session "session-1" reached a failed state before a primary result was available` {
		t.Fatalf("message = %q", got.Message)
	}
	assertInvocationFailureContext(t, got.Context, work.InvocationFailureContext{
		SessionID: "session-1",
		WorkID:    "work-root",
		WorkName:  "Failed goal",
		WorkState: "goal:init",
	})
}

func TestClassifyInvocationControlState_ReturnsPausedForPausedSession(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "goal", "init", "Paused goal", "goal:init")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.FactoryState = string(interfaces.FactoryStatePaused)

	got, ok := work.ClassifyInvocationControlState("session-live-1", "", work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected paused classification")
	}
	if got.Code != work.PrimaryResultErrorCodePaused {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodePaused)
	}
	if got.Message != `invocation paused: session "session-live-1" is paused; resume the session to continue waiting for primary result` {
		t.Fatalf("message = %q", got.Message)
	}
	assertInvocationFailureContext(t, got.Context, work.InvocationFailureContext{
		SessionID: "session-live-1",
		WorkID:    "work-root",
		WorkName:  "Paused goal",
		WorkState: "goal:init",
	})
}

func TestClassifyInvocationControlState_ReturnsInterruptedForScopedDispatch(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "goal", "init", "Interrupted goal", "goal:init")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.WorkItemsByID[rootInitial.ID] = rootInitial
	state.JavaScriptRuntime = &interfaces.FactorySessionJavaScriptRuntimeState{
		Dispatches: []interfaces.FactorySessionDispatchState{{
			ID:             "dispatch-1",
			Status:         "INTERRUPTED",
			RelatedWorkIDs: []string{rootInitial.ID},
		}},
	}

	got, ok := work.ClassifyInvocationControlState("session-js-1", "", work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected interrupted classification")
	}
	if got.Code != work.PrimaryResultErrorCodeInterrupted {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodeInterrupted)
	}
	if got.Message != `invocation interrupted: session "session-js-1" dispatch "dispatch-1" for work "Interrupted goal" was interrupted before a primary result was available` {
		t.Fatalf("message = %q", got.Message)
	}
	assertInvocationFailureContext(t, got.Context, work.InvocationFailureContext{
		SessionID: "session-js-1",
		WorkID:    "work-root",
		WorkName:  "Interrupted goal",
		WorkState: "goal:init",
	})
}

func TestClassifyInvocationControlState_PrefersPausedOverInterrupted(t *testing.T) {
	state := invocationWorldStateFixture()
	rootInitial := invocationWorkItem("work-root", "goal", "init", "Paused goal", "goal:init")
	recordInvocationSubmittedWork(&state, 1, "request-1", rootInitial)
	state.FactoryState = string(interfaces.FactoryStatePaused)
	state.JavaScriptRuntime = &interfaces.FactorySessionJavaScriptRuntimeState{
		Dispatches: []interfaces.FactorySessionDispatchState{{
			ID:             "dispatch-1",
			Status:         "INTERRUPTED",
			RelatedWorkIDs: []string{rootInitial.ID},
		}},
	}

	got, ok := work.ClassifyInvocationControlState("session-live-1", "", work.PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !ok {
		t.Fatal("expected paused classification")
	}
	if got.Code != work.PrimaryResultErrorCodePaused {
		t.Fatalf("code = %q, want %q", got.Code, work.PrimaryResultErrorCodePaused)
	}
}

func invocationWorldStateFixture() interfaces.FactoryWorldState {
	return interfaces.FactoryWorldState{
		PayloadLineage:           work.WorkPayloadLineageProjection{},
		WorkItemsByID:            make(map[string]work.FactoryWorkItem),
		WorkRequestsByID:         make(map[string]interfaces.WorkRequestPayload),
		TerminalWorkByID:         make(map[string]interfaces.FactoryTerminalWork),
		FailedWorkItemsByID:      make(map[string]work.FactoryWorkItem),
		WorkStateChangesByWorkID: make(map[string][]interfaces.FactoryWorldWorkStateChangeRecord),
	}
}

func invocationWorkItem(workID, workTypeName, stateName, name, placeID string) work.FactoryWorkItem {
	return work.FactoryWorkItem{
		ID:          workID,
		WorkTypeID:  workTypeName,
		State:       stateName,
		DisplayName: name,
		TraceID:     workID + "-trace",
		PlaceID:     placeID,
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: workID + "-content",
		}},
	}
}

func assertInvocationFailureContext(t *testing.T, got, want work.InvocationFailureContext) {
	t.Helper()

	if got.SessionID != want.SessionID {
		t.Fatalf("context.sessionID = %q, want %q", got.SessionID, want.SessionID)
	}
	if got.WorkID != want.WorkID {
		t.Fatalf("context.workID = %q, want %q", got.WorkID, want.WorkID)
	}
	if got.WorkName != want.WorkName {
		t.Fatalf("context.workName = %q, want %q", got.WorkName, want.WorkName)
	}
	if got.WorkState != want.WorkState {
		t.Fatalf("context.workState = %q, want %q", got.WorkState, want.WorkState)
	}
}

func recordInvocationSubmittedWork(
	state *interfaces.FactoryWorldState,
	tick int,
	requestID string,
	items ...work.FactoryWorkItem,
) {
	if state == nil {
		return
	}
	request := interfaces.WorkRequestPayload{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		WorkItems: append([]work.FactoryWorkItem(nil), items...),
	}
	state.WorkRequestsByID[requestID] = request
	for _, item := range items {
		state.PayloadLineage.RecordWorkRequestSnapshot(tick, requestID, item)
	}
}

func recordInvocationDispatchOutput(
	state *interfaces.FactoryWorldState,
	tick int,
	dispatchID string,
	consumed []work.FactoryWorkItem,
	outputs ...work.FactoryWorkItem,
) {
	if state == nil {
		return
	}
	for _, item := range consumed {
		state.PayloadLineage.RecordConsumedInputSnapshot(dispatchID, item)
	}
	for i, item := range outputs {
		state.PayloadLineage.RecordDispatchOutputSnapshot(tick, dispatchID, consumed, item, i)
	}
}

func assertPrimaryResultSelection(
	t *testing.T,
	got work.PrimaryResultSelection,
	wantPolicy string,
	want work.FactoryWorkItem,
) {
	t.Helper()

	if got.Policy != wantPolicy {
		t.Fatalf("policy = %q, want %q", got.Policy, wantPolicy)
	}
	if got.WorkID != want.ID {
		t.Fatalf("work ID = %q, want %q", got.WorkID, want.ID)
	}
	if got.WorkTypeName != want.WorkTypeID {
		t.Fatalf("work type name = %q, want %q", got.WorkTypeName, want.WorkTypeID)
	}
	if got.WorkName != want.DisplayName {
		t.Fatalf("work name = %q, want %q", got.WorkName, want.DisplayName)
	}
	if got.TerminalState != want.State {
		t.Fatalf("terminal state = %q, want %q", got.TerminalState, want.State)
	}
	if len(got.PrimaryResult) != len(want.Content) {
		t.Fatalf("primary result = %#v, want %#v", got.PrimaryResult, want.Content)
	}
	if got.PrimaryResult[0].Text != want.Content[0].Text {
		t.Fatalf("primary result text = %q, want %q", got.PrimaryResult[0].Text, want.Content[0].Text)
	}
}
