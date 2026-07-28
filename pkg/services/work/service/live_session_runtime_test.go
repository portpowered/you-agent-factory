package service

import (
	"context"
	"fmt"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type legacyMoveFactory struct {
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

func (f *legacyMoveFactory) MoveWork(
	_ context.Context,
	workID string,
	_ string,
	_ work.WorkStateChangeSource,
	_ string,
) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{
		WorkID:      workID,
		WorkTypeID:  "story",
		FromState:   "draft",
		ToState:     "review",
		FromPlaceID: "story.draft",
		ToPlaceID:   "story.review",
		TokenID:     "token-1",
	}, nil
}

func TestLiveSessionRuntimeAdapterFulfillsWorkRuntimePort(t *testing.T) {
	factory := &legacyMoveFactory{}
	adapter := liveSessionRuntimeAdapter{factory: factory}
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
	if result.FromPlaceID == "" || result.ToPlaceID == "" || result.TokenID == "" {
		t.Fatalf("adapter move = %#v, want Petri fields from legacy runtime", result)
	}
}
