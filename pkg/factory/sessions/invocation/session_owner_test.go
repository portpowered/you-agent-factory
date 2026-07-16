package invocation

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workinvocation "github.com/portpowered/infinite-you/pkg/work/invocation"
)

func TestSessionOwner_SubmitsOneNormalizedWorkAndWaitsWithSubmissionIdentity(t *testing.T) {
	cfg := sessionOwnerFactoryConfig()
	requestID := " caller-request "
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := sessionOwnerTextContent(t, "hello")
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	var submitted []interfaces.SubmitRequest
	var waitInput SessionInvocationWaitInput
	wantResult := FactoryInvocationResult{RequestID: "runtime-request", TraceID: "trace-1", Status: factoryapi.InvocationTerminalStatusCompleted}
	owner := NewSessionOwner(SessionOwnerDependencies{
		FactoryConfig: func(sessionID string) (*interfaces.FactoryConfig, error) {
			assertSessionOwnerEqual(t, "sessionID", sessionID, "session-1")
			return cfg, nil
		},
		SubmitWork: func(gotCtx context.Context, sessionID string, request interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
			gotDeadline, ok := gotCtx.Deadline()
			if !ok {
				t.Fatalf("submit deadline = %v, %v; want %v", gotDeadline, ok, deadline)
			}
			assertSessionOwnerEqual(t, "submit deadline", gotDeadline, deadline)
			submitted = append(submitted, request)
			return interfaces.WorkRequestSubmitResult{RequestID: "runtime-request", TraceID: "trace-1"}, nil
		},
		Observe: func(gotCtx context.Context, sessionID string, input SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			assertSessionOwnerEqual(t, "wait context", gotCtx, ctx)
			waitInput = input
			return completedSessionInvocationObservation("runtime-request", "trace-1", "done"), nil
		},
	})

	got, err := owner.InvokeFactorySession(ctx, "session-1", factoryapi.InvocationRequest{
		RequestId:  &requestID,
		SourceKind: &sourceKind,
		Content:    &content,
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	assertSessionOwnerEqual(t, "result request ID", got.RequestID, wantResult.RequestID)
	assertSessionOwnerEqual(t, "result trace ID", got.TraceID, wantResult.TraceID)
	assertSessionOwnerEqual(t, "result status", got.Status, wantResult.Status)
	if len(submitted) != 1 {
		t.Fatalf("submitted Work count = %d, want 1", len(submitted))
	}
	assertSessionOwnerEqual(t, "submitted request ID", submitted[0].RequestID, "caller-request")
	assertSessionOwnerEqual(t, "submitted Work type", submitted[0].WorkTypeID, "task")
	assertSessionOwnerEqual(t, "submitted content count", len(submitted[0].Content), 1)
	assertSessionOwnerEqual(t, "submitted content", submitted[0].Content[0].Text, "hello")
	assertSessionOwnerEqual(t, "wait request ID", waitInput.RequestID, "runtime-request")
	assertSessionOwnerEqual(t, "wait trace ID", waitInput.TraceID, "trace-1")
	assertSessionOwnerEqual(t, "wait input source", waitInput.InputSource, workinvocation.InputSourceLabel(workinvocation.ArgumentSourceKindCompatibilityContent))
}

func TestSessionOwner_StructuredArgumentsPreserveCanonicalNamesAndSources(t *testing.T) {
	cfg := sessionOwnerFactoryConfig()
	cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{{
		Name: "input", Required: true,
		Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 1}},
	}}}
	var submitted interfaces.SubmitRequest
	owner := successfulSessionOwner(cfg, func(request interfaces.SubmitRequest) { submitted = request })

	_, err := owner.InvokeFactorySession(context.Background(), "session-1", factoryapi.InvocationRequest{Args: &map[string]any{"input": "hello"}})
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	argument := submitted.InvocationArguments.Arguments["input"]
	if len(argument.Values) != 1 || argument.Values[0] != "hello" {
		t.Fatalf("argument values = %#v, want [hello]", argument.Values)
	}
	if len(argument.Sources) != 1 || argument.Sources[0].Kind != string(workinvocation.ArgumentSourceKindStructured) {
		t.Fatalf("argument sources = %#v, want STRUCTURED", argument.Sources)
	}
}

func TestSessionOwner_BeforeSubmitRunsAfterNormalizationAndBeforeWorkSubmission(t *testing.T) {
	cfg := sessionOwnerSignatureFactoryConfig()
	order := make([]string, 0, 2)
	owner := NewSessionOwner(SessionOwnerDependencies{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		BeforeSubmit: func(_ context.Context, _ string, _ *interfaces.FactoryConfig, arguments *workinvocation.NormalizedArguments) error {
			order = append(order, "before")
			if got := arguments.Arguments["input"].Values; len(got) != 1 || got[0] != "hello" {
				t.Fatalf("normalized input = %#v, want [hello]", got)
			}
			return nil
		},
		SubmitWork: func(context.Context, string, interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
			order = append(order, "submit")
			return interfaces.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1"}, nil
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return completedSessionInvocationObservation("request-1", "trace-1", "done"), nil
		},
	})
	if _, err := owner.InvokeFactorySession(context.Background(), "session-1", factoryapi.InvocationRequest{Args: &map[string]any{"input": "hello"}}); err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	if len(order) != 2 || order[0] != "before" || order[1] != "submit" {
		t.Fatalf("call order = %#v, want [before submit]", order)
	}
}

func TestSessionOwner_RejectsInvalidInputsBeforeSubmittingWork(t *testing.T) {
	textKind := factoryapi.InvocationInputSourceKindText
	fileKind := factoryapi.InvocationInputSourceKindFileRef
	content := sessionOwnerTextContent(t, "hello")
	tests := []struct {
		name    string
		cfg     *interfaces.FactoryConfig
		request factoryapi.InvocationRequest
	}{
		{name: "missing content", cfg: sessionOwnerFactoryConfig(), request: factoryapi.InvocationRequest{}},
		{name: "unsupported source", cfg: sessionOwnerFactoryConfig(), request: factoryapi.InvocationRequest{SourceKind: &fileKind}},
		{name: "structured without signature", cfg: sessionOwnerFactoryConfig(), request: factoryapi.InvocationRequest{Args: &map[string]any{"input": "hello"}}},
		{name: "invalid structured value", cfg: sessionOwnerSignatureFactoryConfig(), request: factoryapi.InvocationRequest{Args: &map[string]any{"input": map[string]any{"nested": true}}}},
		{name: "conflicting sources", cfg: sessionOwnerSignatureFactoryConfig(), request: factoryapi.InvocationRequest{SourceKind: &textKind, Content: &content, Args: &map[string]any{"input": "hello"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			submitCalls := 0
			owner := NewSessionOwner(SessionOwnerDependencies{
				FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return tt.cfg, nil },
				SubmitWork: func(context.Context, string, interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
					submitCalls++
					return interfaces.WorkRequestSubmitResult{}, nil
				},
				Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
					return SessionInvocationObservation{}, nil
				},
			})
			if _, err := owner.InvokeFactorySession(context.Background(), "session-1", tt.request); err == nil {
				t.Fatal("InvokeFactorySession error = nil, want validation failure")
			}
			if submitCalls != 0 {
				t.Fatalf("submit calls = %d, want 0", submitCalls)
			}
		})
	}
}

func TestSessionOwner_RejectsInterpolationFailureBeforeSubmittingWork(t *testing.T) {
	cfg := sessionOwnerFactoryConfig()
	cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{{
		Name: "input", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)}},
	}}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{PromptTemplate: "Use ${missing} now"}}
	submitCalls := 0
	owner := NewSessionOwner(SessionOwnerDependencies{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		SubmitWork: func(context.Context, string, interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
			submitCalls++
			return interfaces.WorkRequestSubmitResult{}, nil
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return SessionInvocationObservation{}, nil
		},
	})

	_, err := owner.InvokeFactorySession(context.Background(), "session-1", factoryapi.InvocationRequest{Args: &map[string]any{"input": "hello"}})
	var argumentErr *workinvocation.ArgumentError
	if !errors.As(err, &argumentErr) || argumentErr.Code != workinvocation.ArgumentErrorCodeInvalidInterpolation {
		t.Fatalf("error = %v, want INVALID_INTERPOLATION", err)
	}
	if submitCalls != 0 {
		t.Fatalf("submit calls = %d, want 0", submitCalls)
	}
}

func TestSessionOwner_PreservesCallerCancellationAtSubmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	waitCalls := 0
	owner := NewSessionOwner(SessionOwnerDependencies{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return sessionOwnerFactoryConfig(), nil },
		SubmitWork: func(ctx context.Context, _ string, _ interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
			return interfaces.WorkRequestSubmitResult{}, ctx.Err()
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			waitCalls++
			return SessionInvocationObservation{}, nil
		},
	})
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := sessionOwnerTextContent(t, "hello")
	_, err := owner.InvokeFactorySession(ctx, "session-1", factoryapi.InvocationRequest{SourceKind: &sourceKind, Content: &content})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if waitCalls != 0 {
		t.Fatalf("wait calls = %d, want 0", waitCalls)
	}
}

func successfulSessionOwner(cfg *interfaces.FactoryConfig, capture func(interfaces.SubmitRequest)) *SessionOwner {
	return NewSessionOwner(SessionOwnerDependencies{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		SubmitWork: func(_ context.Context, _ string, request interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
			capture(request)
			return interfaces.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1"}, nil
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return completedSessionInvocationObservation("request-1", "trace-1", "done"), nil
		},
	})
}

func completedSessionInvocationObservation(requestID, traceID, text string) SessionInvocationObservation {
	work := interfaces.FactoryWorkItem{
		ID: "work-1", WorkTypeID: "task", State: "done", TraceID: traceID,
		Content: []interfaces.WorkContentPart{{Type: interfaces.WorkContentPartTypeText, Text: text}},
	}
	return SessionInvocationObservation{WorldState: interfaces.FactoryWorldState{
		WorkRequestsByID: map[string]interfaces.WorkRequestPayload{requestID: {
			RequestID: requestID, TraceID: traceID, WorkItems: []interfaces.FactoryWorkItem{work},
		}},
		TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{work.ID: {WorkItem: work, Status: "done"}},
	}}
}

func sessionOwnerFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{WorkTypes: []interfaces.WorkTypeConfig{{
		Name: "task", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault},
	}}}
}

func sessionOwnerSignatureFactoryConfig() *interfaces.FactoryConfig {
	cfg := sessionOwnerFactoryConfig()
	cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{{
		Name: "input", Required: true,
		Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)}},
	}}}
	return cfg
}

func sessionOwnerTextContent(t *testing.T, text string) factoryapi.WorkContent {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{Type: factoryapi.WorkContentPartTypeText, Text: text}); err != nil {
		t.Fatalf("build text content: %v", err)
	}
	return factoryapi.WorkContent{part}
}

func assertSessionOwnerEqual[T comparable](t *testing.T, field string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %#v, want %#v", field, got, want)
	}
}

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
		Policy: workinvocation.ReturnPolicyExplicit, WorkTypeName: "summary", TerminalState: "complete",
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
		wantCode    workinvocation.PrimaryResultErrorCode
		wantMessage string
		wantContext workinvocation.InvocationFailureContext
	}{
		{
			name: "blocked", observation: classifiedObservation(workinvocation.PrimaryResultErrorCodeBlocked, "blocked"), wantCode: workinvocation.PrimaryResultErrorCodeBlocked,
			wantMessage: `invocation blocked: work "Goal" is waiting in state "goal:blocked"`,
			wantContext: workinvocation.InvocationFailureContext{SessionID: "session-1", WorkID: "work-root", WorkName: "Goal", WorkState: "goal:blocked"},
		},
		{
			name: "needs human", observation: classifiedObservation(workinvocation.PrimaryResultErrorCodeNeedsHuman, "needs-human"), wantCode: workinvocation.PrimaryResultErrorCodeNeedsHuman,
			wantMessage: `invocation needs human input: work "Goal" is waiting in state "goal:needs-human"`,
			wantContext: workinvocation.InvocationFailureContext{SessionID: "session-1", WorkID: "work-root", WorkName: "Goal", WorkState: "goal:needs-human"},
		},
		{
			name: "paused", observation: pausedSessionInvocationObservation(), wantCode: workinvocation.PrimaryResultErrorCodePaused,
			wantMessage: `invocation paused: session "session-1" is paused; resume the session to continue waiting for primary result`,
			wantContext: workinvocation.InvocationFailureContext{SessionID: "session-1", WorkID: "work-root", WorkName: "Goal", WorkState: "goal:init"},
		},
		{
			name: "interrupted", observation: interruptedSessionInvocationObservation(), wantCode: workinvocation.PrimaryResultErrorCodeInterrupted,
			wantMessage: `invocation interrupted: session "session-1" dispatch "dispatch-1" for work "Goal" was interrupted before a primary result was available`,
			wantContext: workinvocation.InvocationFailureContext{SessionID: "session-1", WorkID: "work-root", WorkName: "Goal", WorkState: "goal:init"},
		},
		{
			name: "failed", observation: failedSessionInvocationObservation(), wantCode: workinvocation.PrimaryResultErrorCodeFailed,
			wantMessage: `invocation failed: work "Goal" reached failed state "goal:failed" before a primary result was available`,
			wantContext: workinvocation.InvocationFailureContext{WorkID: "work-root", WorkName: "Goal", WorkState: "goal:failed"},
		},
		{
			name: "unresolved", observation: stoppedSessionInvocationObservation(), wantCode: workinvocation.PrimaryResultErrorCodeUnresolved,
			wantMessage: "invocation primary result unresolved: submitted work did not resolve to terminal output",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := waitForSessionOwnerObservation(t, tt.observation, nil)
			assertSessionOwnerEqual(t, "status", result.Status, factoryapi.InvocationTerminalStatusFailed)
			assertSessionOwnerEqual(t, "error code", result.ErrorCode, string(tt.wantCode))
			assertSessionOwnerEqual(t, "message", result.Message, tt.wantMessage)
			assertSessionOwnerEqual(t, "session ID", result.SessionID, tt.wantContext.SessionID)
			assertSessionOwnerEqual(t, "work ID", result.WorkID, tt.wantContext.WorkID)
			assertSessionOwnerEqual(t, "work name", result.WorkName, tt.wantContext.WorkName)
			assertSessionOwnerEqual(t, "work state", result.WorkState, tt.wantContext.WorkState)
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

func classifiedObservation(code workinvocation.PrimaryResultErrorCode, state string) SessionInvocationObservation {
	observation := stoppedSessionInvocationObservation()
	observation.MissingPrimaryResult = workinvocation.ClassifyMissingPrimaryResultWorkItem(
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

func invocationWorldStateFixture() interfaces.FactoryWorldState {
	return interfaces.FactoryWorldState{
		PayloadLineage:           interfaces.WorkPayloadLineageProjection{},
		WorkItemsByID:            make(map[string]interfaces.FactoryWorkItem),
		WorkRequestsByID:         make(map[string]interfaces.WorkRequestPayload),
		TerminalWorkByID:         make(map[string]interfaces.FactoryTerminalWork),
		FailedWorkItemsByID:      make(map[string]interfaces.FactoryWorkItem),
		WorkStateChangesByWorkID: make(map[string][]interfaces.FactoryWorldWorkStateChangeRecord),
	}
}

func invocationWorkItem(workID, workTypeName, stateName, name, placeID string) interfaces.FactoryWorkItem {
	return interfaces.FactoryWorkItem{
		ID:          workID,
		WorkTypeID:  workTypeName,
		State:       stateName,
		DisplayName: name,
		TraceID:     workID + "-trace",
		PlaceID:     placeID,
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: workID + "-content",
		}},
	}
}

func recordInvocationSubmittedWork(state *interfaces.FactoryWorldState, tick int, requestID string, items ...interfaces.FactoryWorkItem) {
	request := interfaces.WorkRequestPayload{
		RequestID: requestID,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		WorkItems: append([]interfaces.FactoryWorkItem(nil), items...),
	}
	state.WorkRequestsByID[requestID] = request
	for _, item := range items {
		state.PayloadLineage.RecordWorkRequestSnapshot(tick, requestID, item)
	}
}

func recordInvocationDispatchOutput(state *interfaces.FactoryWorldState, tick int, dispatchID string, consumed []interfaces.FactoryWorkItem, outputs ...interfaces.FactoryWorkItem) {
	for _, item := range consumed {
		state.PayloadLineage.RecordConsumedInputSnapshot(dispatchID, item)
	}
	for i, item := range outputs {
		state.PayloadLineage.RecordDispatchOutputSnapshot(tick, dispatchID, consumed, item, i)
	}
}
