package factorysessions_test

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type recordingInvocationWorkService struct {
	work.Service
	prepareCalls int
	resolveCalls int
	lastPrepare  work.InvocationInputPreparationRequest
	lastResolve  work.PrimaryResultSelectionInput
}

func (f *recordingInvocationWorkService) PrepareInvocationInput(
	_ context.Context,
	request work.InvocationInputPreparationRequest,
) (work.PreparedInvocationInput, error) {
	f.prepareCalls++
	f.lastPrepare = request
	return f.Service.PrepareInvocationInput(context.Background(), request)
}

func (f *recordingInvocationWorkService) ResolvePrimaryResult(
	_ context.Context,
	input work.PrimaryResultSelectionInput,
) (work.PrimaryResultSelection, error) {
	f.resolveCalls++
	f.lastResolve = input
	return f.Service.ResolvePrimaryResult(context.Background(), input)
}

func newRecordingInvocationWorkService() *recordingInvocationWorkService {
	return &recordingInvocationWorkService{Service: work.NewInvocationPolicyService()}
}

// TestWorkInvocationBoundary_PreparesInputThroughWorkService proves Factory
// Sessions invocation input normalization reaches Work only through the
// published work.Service.PrepareInvocationInput contract.
func TestWorkInvocationBoundary_PreparesInputThroughWorkService(t *testing.T) {
	t.Parallel()

	recording := newRecordingInvocationWorkService()
	sourceKind := factorysessions.InvocationInputSourceKindText
	owner := legacyinvocation.NewSessionOwner(
		func(string) (*factorydefinitions.FactoryConfig, error) {
			return &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
				Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
			}}}, nil
		},
		func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1"}, nil
		},
		func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
			return legacyinvocation.SessionInvocationObservation{}, nil
		},
		nil,
		nil,
		nil,
		definitionResolverForTest("task"),
		func(string) ([]byte, error) { return nil, nil },
		recording,
	)

	resolved, err := owner.ResolveInvocationInput(&factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
		}},
	}, factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("ResolveInvocationInput: %v", err)
	}
	if recording.prepareCalls != 1 {
		t.Fatalf("PrepareInvocationInput calls = %d, want 1", recording.prepareCalls)
	}
	if len(recording.lastPrepare.CompatibilityContent) != 1 || recording.lastPrepare.CompatibilityContent[0].Text != "hello" {
		t.Fatalf("last prepare request = %#v, want compatibility hello content", recording.lastPrepare)
	}
	if len(resolved.Content) != 1 || resolved.Content[0].Text != "hello" {
		t.Fatalf("resolved content = %#v, want hello", resolved.Content)
	}
}

// TestWorkInvocationBoundary_ResolvesPrimaryResultThroughWorkService proves
// Factory Sessions wait/completion paths select primary results only through
// work.Service.ResolvePrimaryResult.
func TestWorkInvocationBoundary_ResolvesPrimaryResultThroughWorkService(t *testing.T) {
	t.Parallel()

	recording := newRecordingInvocationWorkService()
	observation := completedSessionInvocationObservation("request-1", "trace-1", "done")
	owner := legacyinvocation.NewSessionOwner(
		func(string) (*factorydefinitions.FactoryConfig, error) { return sessionOwnerFactoryConfig(), nil },
		func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1"}, nil
		},
		func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
			return observation, nil
		},
		nil,
		nil,
		nil,
		definitionResolverForTest("task"),
		func(string) ([]byte, error) { return nil, nil },
		recording,
	)

	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := owner.InvokeFactorySession(context.Background(), "session-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	if recording.prepareCalls != 1 {
		t.Fatalf("PrepareInvocationInput calls = %d, want 1", recording.prepareCalls)
	}
	if recording.resolveCalls != 1 {
		t.Fatalf("ResolvePrimaryResult calls = %d, want 1", recording.resolveCalls)
	}
	if recording.lastResolve.RequestID != "request-1" {
		t.Fatalf("last resolve request ID = %q, want request-1", recording.lastResolve.RequestID)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "done" {
		t.Fatalf("primary result = %#v, want done", result.PrimaryResult)
	}
}

func sessionOwnerFactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
		Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
	}}}
}

func completedSessionInvocationObservation(requestID, traceID, text string) legacyinvocation.SessionInvocationObservation {
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
