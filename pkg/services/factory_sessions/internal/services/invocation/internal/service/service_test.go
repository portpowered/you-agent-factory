package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestNewRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		drop func(*invocationservice.Dependencies)
		want string
	}{
		{name: "Factory config", drop: func(deps *invocationservice.Dependencies) { deps.FactoryConfig = nil }, want: "Factory config reader"},
		{name: "Work submitter", drop: func(deps *invocationservice.Dependencies) { deps.SubmitWork = nil }, want: "Work submitter"},
		{name: "result observer", drop: func(deps *invocationservice.Dependencies) { deps.Observe = nil }, want: "result observer"},
		{name: "interpolation", drop: func(deps *invocationservice.Dependencies) { deps.Interpolation = nil }, want: "interpolation service"},
		{name: "Work Type", drop: func(deps *invocationservice.Dependencies) { deps.WorkTypes = nil }, want: "Work Type service"},
		{name: "input reader", drop: func(deps *invocationservice.Dependencies) { deps.InputFiles = nil }, want: "input file reader"},
		{name: "Work service", drop: func(deps *invocationservice.Dependencies) { deps.Work = nil }, want: "Work service"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			deps := validDependencies(nil)
			test.drop(&deps)
			got, err := New(deps)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() = %#v, %v; want error containing %q", got, err, test.want)
			}
			if got != nil {
				t.Fatalf("New() service = %#v, want nil", got)
			}
		})
	}
}

func TestNewIsInert(t *testing.T) {
	t.Parallel()

	calls := 0
	deps := validDependencies(&calls)
	got, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if got == nil {
		t.Fatal("New() service = nil")
	}
	if calls != 0 {
		t.Fatalf("runtime dependency calls during construction = %d, want 0", calls)
	}
}

func TestServiceCoordinatesSubmissionAndTerminalResult(t *testing.T) {
	t.Parallel()

	deps := validDependencies(nil)
	var submitted work.SubmitRequest
	deps.SubmitWork = func(_ context.Context, sessionID string, request work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
		if sessionID != "session-1" {
			t.Fatalf("session ID = %q, want session-1", sessionID)
		}
		submitted = request
		return work.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1"}, nil
	}
	deps.Observe = func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
		return completedObservation("request-1", "trace-1", "done"), nil
	}
	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := service.InvokeFactorySession(context.Background(), "session-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession(): %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusCompleted || result.RequestID != "request-1" || result.TraceID != "trace-1" {
		t.Fatalf("result = %#v, want completed request-1/trace-1", result)
	}
	if submitted.WorkTypeID != "task" || len(submitted.Content) != 1 || submitted.Content[0].Text != "hello" {
		t.Fatalf("submitted Work = %#v, want normalized task content", submitted)
	}
}

func TestServiceCompletesCancellationExactlyOnce(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	deps := validDependencies(nil)
	deps.CancelOnTimeout = func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
		t.Fatal("cancel-on-timeout callback invoked for a canceled wait")
		return factorysessions.LifecycleControlResult{}, nil
	}
	observeCalls := 0
	deps.Observe = func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
		observeCalls++
		cancel()
		return legacyinvocation.SessionInvocationObservation{}, nil
	}
	deps.WaitNext = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := service.InvokeFactorySession(ctx, "session-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession(): %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusCanceled || result.ErrorCode != string(factorydefinitions.InvocationErrorCodeCanceled) {
		t.Fatalf("result = %#v, want canceled", result)
	}
	if observeCalls != 1 {
		t.Fatalf("observation calls = %d, want 1", observeCalls)
	}
}

func TestServiceCompletesTimeoutExactlyOnce(t *testing.T) {
	t.Parallel()

	deps := validDependencies(nil)
	cancelCalls := 0
	var cancelRequest factorysessions.ControlRequest
	deps.CancelOnTimeout = func(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
		cancelCalls++
		if ctx.Err() != nil {
			t.Fatalf("cancel-on-timeout context error = %v, want detached context", ctx.Err())
		}
		if sessionID != "session-1" {
			t.Fatalf("cancel-on-timeout session ID = %q, want session-1", sessionID)
		}
		cancelRequest = request
		return factorysessions.LifecycleControlResult{Status: factorysessions.LifecycleStatusCanceling}, nil
	}
	observeCalls := 0
	deps.Observe = func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
		observeCalls++
		return legacyinvocation.SessionInvocationObservation{}, nil
	}
	deps.WaitNext = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	sourceKind := factorysessions.InvocationInputSourceKindText
	timeoutMillis := int64(1)
	result, err := service.InvokeFactorySession(context.Background(), "session-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		TimeoutMillis:   &timeoutMillis,
		CancelOnTimeout: true,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession(): %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusTimedOut || result.ErrorCode != string(factorydefinitions.InvocationErrorCodeTimedOut) {
		t.Fatalf("result = %#v, want timed out", result)
	}
	if observeCalls != 1 {
		t.Fatalf("observation calls = %d, want 1", observeCalls)
	}
	if cancelCalls != 1 || cancelRequest.RequestID != "request-1" || cancelRequest.Reason != "invocation wait timed out" {
		t.Fatalf("cancel-on-timeout = calls:%d request:%#v, want one request-scoped cancellation", cancelCalls, cancelRequest)
	}
}

func TestServiceResolveInvocationInputNormalizesCompatibilityText(t *testing.T) {
	t.Parallel()

	deps := validDependencies(nil)
	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	cfg := &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
		Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
	}}}
	sourceKind := factorysessions.InvocationInputSourceKindText
	resolved, err := service.ResolveInvocationInput(cfg, factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("ResolveInvocationInput(): %v", err)
	}
	if len(resolved.Content) != 1 || resolved.Content[0].Text != "hello" {
		t.Fatalf("resolved content = %#v, want hello compatibility text", resolved.Content)
	}
}

func TestServicePreservesDependencyFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("runtime unavailable")
	deps := validDependencies(nil)
	deps.FactoryConfig = func(string) (*factorydefinitions.FactoryConfig, error) { return nil, want }
	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	_, err = service.InvokeFactorySession(context.Background(), "session-1", factorysessions.InvocationRequest{})
	if !errors.Is(err, want) {
		t.Fatalf("InvokeFactorySession() error = %v, want %v", err, want)
	}
}

func validDependencies(calls *int) invocationservice.Dependencies {
	count := func() {
		if calls != nil {
			*calls++
		}
	}
	return invocationservice.Dependencies{
		FactoryConfig: func(string) (*factorydefinitions.FactoryConfig, error) {
			count()
			return &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
				Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
			}}}, nil
		},
		SubmitWork: func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
			count()
			return work.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1"}, nil
		},
		Observe: func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
			count()
			return completedObservation("request-1", "trace-1", "done"), nil
		},
		Interpolation: factorydefinitionfixtures.InvocationInterpolation{},
		WorkTypes:     staticWorkType("task"),
		InputFiles: func(string) ([]byte, error) {
			count()
			return nil, nil
		},
		Work: work.NewInvocationPolicyService(),
	}
}

type staticWorkType string

func (workType staticWorkType) DefaultWorkType(*factorydefinitions.FactoryConfig) (string, error) {
	return string(workType), nil
}

func completedObservation(requestID, traceID, text string) legacyinvocation.SessionInvocationObservation {
	item := work.FactoryWorkItem{
		ID: "work-1", WorkTypeID: "task", State: "done", TraceID: traceID,
		Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: text}},
	}
	return legacyinvocation.SessionInvocationObservation{WorldState: factorydefinitions.FactoryWorldState{
		WorkRequestsByID: map[string]factorydefinitions.WorkRequestPayload{requestID: {
			RequestID: requestID, TraceID: traceID, WorkItems: []work.FactoryWorkItem{item},
		}},
		TerminalWorkByID: map[string]factorydefinitions.FactoryTerminalWork{item.ID: {WorkItem: item, Status: "done"}},
	}}
}
