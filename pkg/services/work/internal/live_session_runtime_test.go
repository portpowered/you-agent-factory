package internal

import (
	"context"
	"fmt"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type legacyMoveFactory struct {
	factory.Service
	submitted work.WorkRequest
}

func (f *legacyMoveFactory) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	f.submitted = request
	return work.WorkRequestSubmitResult{RequestID: request.RequestID}, nil
}

func (legacyMoveFactory) SubscribeFactoryEvents(
	context.Context,
	*interfaces.FactoryEventReconnectCursor,
	interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	return nil, fmt.Errorf("not implemented")
}

func (legacyMoveFactory) ControlMoveWork(_ context.Context, request factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	return factory.MoveWorkResult{
		WorkID: request.WorkID, WorkTypeID: "story",
		FromState: "draft", ToState: request.StateName,
	}, nil
}

func TestLiveSessionRuntimeAdapterFulfillsWorkRuntimePort(t *testing.T) {
	factory := &legacyMoveFactory{}
	adapter := liveSessionRuntimeAdapter{runtime: factory, ingress: factory}
	ctx := context.Background()

	request := work.WorkRequest{RequestID: "request-legacy-adapter"}
	if _, err := adapter.SubmitWorkRequest(ctx, request); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if factory.submitted.RequestID != request.RequestID {
		t.Fatalf("submitted = %q, want %q", factory.submitted.RequestID, request.RequestID)
	}

	result, err := adapter.MoveWork(ctx, "work-1", "review", work.WorkStateChangeSourceAPI, "move-legacy")
	if err != nil {
		t.Fatalf("MoveWork: %v", err)
	}
	if result.FromState != "draft" || result.ToState != "review" {
		t.Fatalf("adapter move = %#v, want detached state facts from Runtime", result)
	}
}

type liveRuntimeStub struct {
	runtime *factorysessions.LiveRuntime
}

func (r liveRuntimeStub) Resolve(string) *factorysessions.LiveRuntime { return r.runtime }

// TestLiveSessionRuntimeResolverUsesDeclaredWorkAndEventIngress proves Work
// submits through the ingress Factory Sessions declared on the live runtime.
func TestLiveSessionRuntimeResolverUsesDeclaredWorkAndEventIngress(t *testing.T) {
	runtimeValue := &legacyMoveFactory{}
	resolver := liveSessionRuntimeResolver{
		sessions: liveRuntimeStub{runtime: &factorysessions.LiveRuntime{
			Factory:             runtimeValue,
			WorkAndEventIngress: runtimeValue,
		}},
	}

	runtime, err := resolver.ResolveWorkRuntime("session-1")
	if err != nil {
		t.Fatalf("ResolveWorkRuntime: %v", err)
	}
	if _, err := runtime.SubmitWorkRequest(
		context.Background(), work.WorkRequest{RequestID: "request-declared"},
	); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if runtimeValue.submitted.RequestID != "request-declared" {
		t.Fatalf("submitted = %q, want request-declared", runtimeValue.submitted.RequestID)
	}
}

// TestLiveSessionRuntimeResolverDoesNotRecoverSubmitterFromRuntimeValue is the
// guard that the retired Work projection fallback stays retired. The bound
// runtime value here does serve SubmitWorkRequest, so the legacy type
// assertion would have succeeded; Work must instead fail closed because
// Factory Sessions declared no Work and event ingress for it.
func TestLiveSessionRuntimeResolverDoesNotRecoverSubmitterFromRuntimeValue(t *testing.T) {
	runtimeValue := &legacyMoveFactory{}
	resolver := liveSessionRuntimeResolver{
		sessions: liveRuntimeStub{runtime: &factorysessions.LiveRuntime{
			Factory: runtimeValue,
		}},
	}

	runtime, err := resolver.ResolveWorkRuntime("session-1")
	if err != nil {
		t.Fatalf("ResolveWorkRuntime: %v", err)
	}
	_, err = runtime.SubmitWorkRequest(context.Background(), work.WorkRequest{RequestID: "request-1"})
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime work submission is required") {
		t.Fatalf("SubmitWorkRequest error = %v, want submission-required error", err)
	}
	if runtimeValue.submitted.RequestID != "" {
		t.Fatalf(
			"submitted = %q, want the runtime value untouched without a declared ingress",
			runtimeValue.submitted.RequestID,
		)
	}
}
