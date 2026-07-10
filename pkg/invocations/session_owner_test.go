package invocations

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestSessionOwner_SubmitsOneNormalizedWorkAndWaitsWithSubmissionIdentity(t *testing.T) {
	cfg := sessionOwnerFactoryConfig()
	requestID := "caller-request"
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
		Wait: func(gotCtx context.Context, sessionID string, input SessionInvocationWaitInput) (FactoryInvocationResult, error) {
			assertSessionOwnerEqual(t, "wait context", gotCtx, ctx)
			waitInput = input
			return wantResult, nil
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
	assertSessionOwnerEqual(t, "wait input source", waitInput.InputSource, InputSourceLabel(ArgumentSourceKindCompatibilityContent))
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
	if len(argument.Sources) != 1 || argument.Sources[0].Kind != string(ArgumentSourceKindStructured) {
		t.Fatalf("argument sources = %#v, want STRUCTURED", argument.Sources)
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
				Wait: func(context.Context, string, SessionInvocationWaitInput) (FactoryInvocationResult, error) {
					return FactoryInvocationResult{}, nil
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
		Wait: func(context.Context, string, SessionInvocationWaitInput) (FactoryInvocationResult, error) {
			return FactoryInvocationResult{}, nil
		},
	})

	_, err := owner.InvokeFactorySession(context.Background(), "session-1", factoryapi.InvocationRequest{Args: &map[string]any{"input": "hello"}})
	var argumentErr *ArgumentError
	if !errors.As(err, &argumentErr) || argumentErr.Code != ArgumentErrorCodeInvalidInterpolation {
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
		Wait: func(context.Context, string, SessionInvocationWaitInput) (FactoryInvocationResult, error) {
			waitCalls++
			return FactoryInvocationResult{}, nil
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
		Wait: func(context.Context, string, SessionInvocationWaitInput) (FactoryInvocationResult, error) {
			return FactoryInvocationResult{Status: factoryapi.InvocationTerminalStatusCompleted}, nil
		},
	})
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
