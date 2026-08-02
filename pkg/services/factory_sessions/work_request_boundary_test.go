package factorysessions_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// TestWorkRequestBoundary_InvocationSubmitConstructsDetachedWorkRequest proves
// session-owned invocation submit constructs a canonical Work Request with
// observable acceptance facts when admission succeeds through the injected
// submit edge.
func TestWorkRequestBoundary_InvocationSubmitConstructsDetachedWorkRequest(t *testing.T) {
	t.Parallel()

	var submitted work.WorkRequest
	sourceKind := factorysessions.InvocationInputSourceKindText
	owner := legacyinvocation.NewSessionOwner(
		func(string) (*factorydefinitions.FactoryConfig, error) {
			return &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
				Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
			}}}, nil
		},
		func(_ context.Context, sessionID string, request work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
			submitted = work.WorkRequestFromSubmitRequests([]work.SubmitRequest{request})
			if sessionID != "session-1" {
				return work.WorkRequestSubmitResult{}, fmt.Errorf("session = %q, want session-1", sessionID)
			}
			if len(submitted.Works) != 1 || submitted.Works[0].WorkTypeID != "task" {
				return work.WorkRequestSubmitResult{}, fmt.Errorf("submitted request = %#v, want one task work", submitted)
			}
			return work.WorkRequestSubmitResult{
				RequestID: "request-1",
				TraceID:   "trace-1",
				Accepted:  true,
				WorkID:    "work-1",
				Name:      "task",
			}, nil
		},
		func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
			return completedSessionInvocationObservation("request-1", "trace-1", "done"), nil
		},
		nil,
		nil,
		nil,
		definitionResolverForTest("task"),
		func(string) ([]byte, error) { return nil, nil },
		newRecordingInvocationWorkService(),
	)

	result, err := owner.InvokeFactorySession(context.Background(), "session-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	if len(submitted.Works) != 1 || submitted.Works[0].Content[0].Text != "hello" {
		t.Fatalf("constructed request = %#v, want hello content", submitted)
	}
	if result.RequestID != "request-1" || result.TraceID != "trace-1" {
		t.Fatalf("result = %#v, want request-1/trace-1", result)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
}

// TestWorkRequestBoundary_InvocationSubmitRejectsTypedAdmissionFailure proves
// session-owned invocation submit surfaces typed Work admission failures as
// observable invocation errors instead of silently accepting invalid requests.
func TestWorkRequestBoundary_InvocationSubmitRejectsTypedAdmissionFailure(t *testing.T) {
	t.Parallel()

	sourceKind := factorysessions.InvocationInputSourceKindText
	owner := legacyinvocation.NewSessionOwner(
		func(string) (*factorydefinitions.FactoryConfig, error) { return sessionOwnerFactoryConfig(), nil },
		func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{}, fmt.Errorf("work_request: invalid Work Request: %w", work.ErrInvalidWorkRequest)
		},
		func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
			return legacyinvocation.SessionInvocationObservation{}, nil
		},
		nil,
		nil,
		nil,
		definitionResolverForTest("task"),
		func(string) ([]byte, error) { return nil, nil },
		newRecordingInvocationWorkService(),
	)

	_, err := owner.InvokeFactorySession(context.Background(), "session-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err == nil {
		t.Fatal("InvokeFactorySession error = nil, want typed admission failure")
	}
	if !errors.Is(err, work.ErrInvalidWorkRequest) {
		t.Fatalf("InvokeFactorySession error = %v, want ErrInvalidWorkRequest", err)
	}
}

func definitionResolverForTest(defaultWorkType string) legacyinvocation.DefinitionResolver {
	return func(
		_ context.Context,
		_ string,
		cfg *factorydefinitions.FactoryConfig,
		_ *work.InvocationArguments,
		_ map[string][]byte,
	) (factorydefinitions.ResolveInvocationDefinitionResult, error) {
		result := factorydefinitions.ResolveInvocationDefinitionResult{DefaultWorkType: defaultWorkType}
		if cfg != nil {
			result.Factory = *cfg
		}
		return result, nil
	}
}
