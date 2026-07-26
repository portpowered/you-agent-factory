package invocation

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

func TestSessionOwner_SubmitsOneNormalizedWorkAndWaitsWithSubmissionIdentity(t *testing.T) {
	cfg := sessionOwnerFactoryConfig()
	requestID := " caller-request "
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := sessionOwnerTextContent(t, "hello")
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	var submitted []workdomain.SubmitRequest
	var waitInput SessionInvocationWaitInput
	wantResult := FactoryInvocationResult{RequestID: "runtime-request", TraceID: "trace-1", Status: interfaces.InvocationTerminalStatusCompleted}
	owner := newTestSessionOwner(sessionOwnerFixture{
		FactoryConfig: func(sessionID string) (*interfaces.FactoryConfig, error) {
			assertSessionOwnerEqual(t, "sessionID", sessionID, "session-1")
			return cfg, nil
		},
		SubmitWork: func(gotCtx context.Context, sessionID string, request workdomain.SubmitRequest) (workdomain.WorkRequestSubmitResult, error) {
			gotDeadline, ok := gotCtx.Deadline()
			if !ok {
				t.Fatalf("submit deadline = %v, %v; want %v", gotDeadline, ok, deadline)
			}
			assertSessionOwnerEqual(t, "submit deadline", gotDeadline, deadline)
			submitted = append(submitted, request)
			return workdomain.WorkRequestSubmitResult{RequestID: "runtime-request", TraceID: "trace-1"}, nil
		},
		Observe: func(gotCtx context.Context, sessionID string, input SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			assertSessionOwnerEqual(t, "wait context", gotCtx, ctx)
			waitInput = input
			return completedSessionInvocationObservation("runtime-request", "trace-1", "done"), nil
		},
	})

	got, err := owner.InvokeFactorySession(ctx, "session-1", sessionOwnerInvocationRequest(factoryapi.InvocationRequest{
		RequestId:  &requestID,
		SourceKind: &sourceKind,
		Content:    &content,
	}))
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
	assertSessionOwnerEqual(t, "wait input source", waitInput.InputSource, work.InputSourceLabel(work.ArgumentSourceKindCompatibilityContent))
}

func TestSessionOwner_StructuredArgumentsPreserveCanonicalNamesAndSources(t *testing.T) {
	cfg := sessionOwnerFactoryConfig()
	cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{{
		Name: "input", Required: true,
		Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 1}},
	}}}
	var submitted workdomain.SubmitRequest
	owner := successfulSessionOwner(cfg, func(request workdomain.SubmitRequest) { submitted = request })

	_, err := owner.InvokeFactorySession(context.Background(), "session-1", sessionOwnerInvocationRequest(factoryapi.InvocationRequest{Args: &map[string]any{"input": "hello"}}))
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	argument := submitted.InvocationArguments.Arguments["input"]
	if len(argument.Values) != 1 || argument.Values[0] != "hello" {
		t.Fatalf("argument values = %#v, want [hello]", argument.Values)
	}
	if len(argument.Sources) != 1 || argument.Sources[0].Kind != string(work.ArgumentSourceKindStructured) {
		t.Fatalf("argument sources = %#v, want STRUCTURED", argument.Sources)
	}
	if len(submitted.Content) != 1 || submitted.Content[0].Text != "hello" {
		t.Fatalf("submitted content = %#v, want primary structured input", submitted.Content)
	}
}

func TestSessionOwner_PreparedCLIArgumentsRetainCanonicalValuesOrderAndProvenance(t *testing.T) {
	cfg := sessionOwnerSignatureFactoryConfig()
	cfg.InvocationSignature.Parameters[0].ValueMode = work.InvocationParameterValueModeRepeated
	var submitted workdomain.SubmitRequest
	owner := successfulSessionOwner(cfg, func(request workdomain.SubmitRequest) { submitted = request })
	prepared := &work.PreparedInvocationInput{
		NormalizedArguments: &work.NormalizedArguments{Arguments: map[string]work.NormalizedArgument{
			"input": {
				Values: []string{"first", "second"},
				Sources: []work.ArgumentSource{
					{Kind: work.ArgumentSourceKindPositional, Name: "1"},
					{Kind: work.ArgumentSourceKindDefault, Name: "input"},
				},
			},
		}},
	}

	_, err := owner.InvokeFactorySession(context.Background(), "session-1", InvocationRequest{
		PreparedInvocationInput: prepared,
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	argument := submitted.InvocationArguments.Arguments["input"]
	if !reflect.DeepEqual(argument.Values, []string{"first", "second"}) {
		t.Fatalf("argument values = %#v, want stable canonical order", argument.Values)
	}
	wantSources := []work.InvocationArgumentSource{
		{Kind: string(work.ArgumentSourceKindPositional), Name: "1"},
		{Kind: string(work.ArgumentSourceKindDefault), Name: "input"},
	}
	if !reflect.DeepEqual(argument.Sources, wantSources) {
		t.Fatalf("argument sources = %#v, want preserved provenance %#v", argument.Sources, wantSources)
	}
}

func TestSessionOwnerRequiresInvocationInputReader(t *testing.T) {
	t.Parallel()

	owner := successfulSessionOwner(sessionOwnerFactoryConfig(), nil)
	owner.inputFiles = nil
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := sessionOwnerTextContent(t, "hello")
	_, err := owner.InvokeFactorySession(context.Background(), "session-1", sessionOwnerInvocationRequest(factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	}))
	if err == nil || !strings.Contains(err.Error(), "input file reader is unavailable") {
		t.Fatalf("InvokeFactorySession missing input reader error = %v", err)
	}
}

func TestStructuredInvocationContentRequiresOnePrimaryPositionalValue(t *testing.T) {
	primarySignature := &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{{
		Name:     "input",
		Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 1}},
	}}}
	noPrimarySignature := &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{{
		Name:     "input",
		Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)}},
	}}}
	tests := []struct {
		name        string
		signature   *interfaces.InvocationSignatureConfig
		arguments   work.NormalizedArguments
		wantContent string
	}{
		{name: "nil signature", arguments: work.NormalizedArguments{}},
		{name: "no positional binding", signature: noPrimarySignature, arguments: work.NormalizedArguments{}},
		{name: "missing primary argument", signature: primarySignature, arguments: work.NormalizedArguments{}},
		{name: "multiple primary values", signature: primarySignature, arguments: work.NormalizedArguments{Arguments: map[string]work.NormalizedArgument{"input": {Values: []string{"first", "second"}}}}},
		{name: "one primary value", signature: primarySignature, arguments: work.NormalizedArguments{Arguments: map[string]work.NormalizedArgument{"input": {Values: []string{"hello"}}}}, wantContent: "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := structuredInvocationContent(tt.signature, tt.arguments)
			if tt.wantContent == "" && content != nil {
				t.Fatalf("content = %#v, want nil", content)
			}
			if tt.wantContent != "" && (len(content) != 1 || content[0].Text != tt.wantContent) {
				t.Fatalf("content = %#v, want %q", content, tt.wantContent)
			}
		})
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
			owner := newTestSessionOwner(sessionOwnerFixture{
				FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return tt.cfg, nil },
				SubmitWork: func(context.Context, string, workdomain.SubmitRequest) (workdomain.WorkRequestSubmitResult, error) {
					submitCalls++
					return workdomain.WorkRequestSubmitResult{}, nil
				},
				Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
					return SessionInvocationObservation{}, nil
				},
			})
			if _, err := owner.InvokeFactorySession(context.Background(), "session-1", sessionOwnerInvocationRequest(tt.request)); err == nil {
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
	owner := newTestSessionOwner(sessionOwnerFixture{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		SubmitWork: func(context.Context, string, workdomain.SubmitRequest) (workdomain.WorkRequestSubmitResult, error) {
			submitCalls++
			return workdomain.WorkRequestSubmitResult{}, nil
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return SessionInvocationObservation{}, nil
		},
		Interpolation: rejectingInvocationInterpolation("missing"),
	})

	_, err := owner.InvokeFactorySession(context.Background(), "session-1", sessionOwnerInvocationRequest(factoryapi.InvocationRequest{Args: &map[string]any{"input": "hello"}}))
	var argumentErr *work.ArgumentError
	if !errors.As(err, &argumentErr) || argumentErr.Code != work.ArgumentErrorCodeInvalidInterpolation {
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
	owner := newTestSessionOwner(sessionOwnerFixture{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return sessionOwnerFactoryConfig(), nil },
		SubmitWork: func(ctx context.Context, _ string, _ workdomain.SubmitRequest) (workdomain.WorkRequestSubmitResult, error) {
			return workdomain.WorkRequestSubmitResult{}, ctx.Err()
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			waitCalls++
			return SessionInvocationObservation{}, nil
		},
	})
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := sessionOwnerTextContent(t, "hello")
	_, err := owner.InvokeFactorySession(ctx, "session-1", sessionOwnerInvocationRequest(factoryapi.InvocationRequest{SourceKind: &sourceKind, Content: &content}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if waitCalls != 0 {
		t.Fatalf("wait calls = %d, want 0", waitCalls)
	}
}

func successfulSessionOwner(cfg *interfaces.FactoryConfig, capture func(workdomain.SubmitRequest)) *SessionOwner {
	return newTestSessionOwner(sessionOwnerFixture{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		SubmitWork: func(_ context.Context, _ string, request workdomain.SubmitRequest) (workdomain.WorkRequestSubmitResult, error) {
			capture(request)
			return workdomain.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1"}, nil
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return completedSessionInvocationObservation("request-1", "trace-1", "done"), nil
		},
	})
}

func completedSessionInvocationObservation(requestID, traceID, text string) SessionInvocationObservation {
	work := workdomain.FactoryWorkItem{
		ID: "work-1", WorkTypeID: "task", State: "done", TraceID: traceID,
		Content: []workdomain.WorkContentPart{{Type: workdomain.WorkContentPartTypeText, Text: text}},
	}
	return SessionInvocationObservation{WorldState: interfaces.FactoryWorldState{
		WorkRequestsByID: map[string]interfaces.WorkRequestPayload{requestID: {
			RequestID: requestID, TraceID: traceID, WorkItems: []workdomain.FactoryWorkItem{work},
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

func sessionOwnerInvocationRequest(request factoryapi.InvocationRequest) InvocationRequest {
	result := InvocationRequest{
		Args:            request.Args,
		Content:         contentcontract.PartsFromGenerated(request.Content),
		ContentProvided: request.Content != nil,
		RequestID:       request.RequestId,
		TimeoutMillis:   request.TimeoutMillis,
	}
	if request.SourceKind != nil {
		sourceKind := InvocationInputSourceKind(*request.SourceKind)
		result.SourceKind = &sourceKind
	}
	return result
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

	assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusCompleted)
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
	recordInvocationDispatchOutput(&state, 2, "dispatch-1", []workdomain.FactoryWorkItem{root}, summary)
	state.TerminalWorkByID[summary.ID] = interfaces.FactoryTerminalWork{WorkItem: summary, Status: "TERMINAL"}
	state.TerminalWorkByID[unrelated.ID] = interfaces.FactoryTerminalWork{WorkItem: unrelated, Status: "TERMINAL"}

	result := waitForSessionOwnerObservation(t, SessionInvocationObservation{WorldState: state}, &interfaces.InvocationReturnConfig{
		Policy: work.ReturnPolicyExplicit, WorkTypeName: "summary", TerminalState: "complete",
	})

	assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusCompleted)
	assertSessionOwnerEqual(t, "primary result", result.PrimaryResult[0].Text, summary.Content[0].Text)
}

func TestSessionOwnerWait_MapsTimeoutAndCancellation(t *testing.T) {
	tests := []struct {
		name       string
		waitErr    error
		wantStatus interfaces.InvocationTerminalStatus
		wantCode   string
	}{
		{name: "timeout", waitErr: context.DeadlineExceeded, wantStatus: interfaces.InvocationTerminalStatusTimedOut, wantCode: string(interfaces.InvocationErrorCodeTimedOut)},
		{name: "cancellation", waitErr: context.Canceled, wantStatus: interfaces.InvocationTerminalStatusCanceled, wantCode: string(interfaces.InvocationErrorCodeCanceled)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := newTestSessionOwner(sessionOwnerFixture{
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
	owner := newTestSessionOwner(sessionOwnerFixture{
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
	assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusTimedOut)
}

func TestSessionOwnerWait_PreservesTerminalFailureClassifications(t *testing.T) {
	tests := []struct {
		name        string
		observation SessionInvocationObservation
		wantCode    work.PrimaryResultErrorCode
		wantMessage string
		wantContext work.InvocationFailureContext
	}{
		{
			name: "blocked", observation: classifiedObservation(work.PrimaryResultErrorCodeBlocked, "blocked"), wantCode: work.PrimaryResultErrorCodeBlocked,
			wantMessage: `invocation blocked: work "Goal" is waiting in state "goal:blocked"`,
			wantContext: work.InvocationFailureContext{SessionID: "session-1", WorkID: "work-root", WorkName: "Goal", WorkState: "goal:blocked"},
		},
		{
			name: "needs human", observation: classifiedObservation(work.PrimaryResultErrorCodeNeedsHuman, "needs-human"), wantCode: work.PrimaryResultErrorCodeNeedsHuman,
			wantMessage: `invocation needs human input: work "Goal" is waiting in state "goal:needs-human"`,
			wantContext: work.InvocationFailureContext{SessionID: "session-1", WorkID: "work-root", WorkName: "Goal", WorkState: "goal:needs-human"},
		},
		{
			name: "paused", observation: pausedSessionInvocationObservation(), wantCode: work.PrimaryResultErrorCodePaused,
			wantMessage: `invocation paused: session "session-1" is paused; resume the session to continue waiting for primary result`,
			wantContext: work.InvocationFailureContext{SessionID: "session-1", WorkID: "work-root", WorkName: "Goal", WorkState: "goal:init"},
		},
		{
			name: "interrupted", observation: interruptedSessionInvocationObservation(), wantCode: work.PrimaryResultErrorCodeInterrupted,
			wantMessage: `invocation interrupted: session "session-1" dispatch "dispatch-1" for work "Goal" was interrupted before a primary result was available`,
			wantContext: work.InvocationFailureContext{SessionID: "session-1", WorkID: "work-root", WorkName: "Goal", WorkState: "goal:init"},
		},
		{
			name: "failed", observation: failedSessionInvocationObservation(), wantCode: work.PrimaryResultErrorCodeFailed,
			wantMessage: `invocation failed: work "Goal" reached failed state "goal:failed" before a primary result was available`,
			wantContext: work.InvocationFailureContext{WorkID: "work-root", WorkName: "Goal", WorkState: "goal:failed"},
		},
		{
			name: "unresolved", observation: stoppedSessionInvocationObservation(), wantCode: work.PrimaryResultErrorCodeUnresolved,
			wantMessage: "invocation primary result unresolved: submitted work did not resolve to terminal output",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := waitForSessionOwnerObservation(t, tt.observation, nil)
			assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusFailed)
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
	owner := newTestSessionOwner(sessionOwnerFixture{Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
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

func classifiedObservation(code work.PrimaryResultErrorCode, state string) SessionInvocationObservation {
	observation := stoppedSessionInvocationObservation()
	observation.MissingPrimaryResult = work.ClassifyMissingPrimaryResultWorkItem(
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
		PayloadLineage:           work.WorkPayloadLineageProjection{},
		WorkItemsByID:            make(map[string]workdomain.FactoryWorkItem),
		WorkRequestsByID:         make(map[string]interfaces.WorkRequestPayload),
		TerminalWorkByID:         make(map[string]interfaces.FactoryTerminalWork),
		FailedWorkItemsByID:      make(map[string]workdomain.FactoryWorkItem),
		WorkStateChangesByWorkID: make(map[string][]interfaces.FactoryWorldWorkStateChangeRecord),
	}
}

func invocationWorkItem(workID, workTypeName, stateName, name, placeID string) workdomain.FactoryWorkItem {
	return workdomain.FactoryWorkItem{
		ID:          workID,
		WorkTypeID:  workTypeName,
		State:       stateName,
		DisplayName: name,
		TraceID:     workID + "-trace",
		PlaceID:     placeID,
		Content: []workdomain.WorkContentPart{{
			Type: workdomain.WorkContentPartTypeText,
			Text: workID + "-content",
		}},
	}
}

func recordInvocationSubmittedWork(state *interfaces.FactoryWorldState, tick int, requestID string, items ...workdomain.FactoryWorkItem) {
	request := interfaces.WorkRequestPayload{
		RequestID: requestID,
		Type:      workdomain.WorkRequestTypeFactoryRequestBatch,
		WorkItems: append([]workdomain.FactoryWorkItem(nil), items...),
	}
	state.WorkRequestsByID[requestID] = request
	for _, item := range items {
		state.PayloadLineage.RecordWorkRequestSnapshot(tick, requestID, item)
	}
}

func recordInvocationDispatchOutput(state *interfaces.FactoryWorldState, tick int, dispatchID string, consumed []workdomain.FactoryWorkItem, outputs ...workdomain.FactoryWorkItem) {
	for _, item := range consumed {
		state.PayloadLineage.RecordConsumedInputSnapshot(dispatchID, item)
	}
	for i, item := range outputs {
		state.PayloadLineage.RecordDispatchOutputSnapshot(tick, dispatchID, consumed, item, i)
	}
}
