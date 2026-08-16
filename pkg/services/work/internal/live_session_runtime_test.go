package internal

import (
	"context"
	"fmt"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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
	adapter := liveSessionRuntimeAdapter{runtime: factory}
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
