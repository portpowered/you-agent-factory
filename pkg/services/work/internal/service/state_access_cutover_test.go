package service_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/service"
)

type petriMoveRuntime struct {
	recordingFactory
}

func (f *petriMoveRuntime) MoveWork(
	_ context.Context,
	workID string,
	_ string,
	source work.WorkStateChangeSource,
	_ string,
) (work.OperatorMoveResult, error) {
	f.movedID = workID
	f.source = source
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

func TestNewServiceRoutesStateAccessSubmitMoveAndReadThroughDetachedResults(t *testing.T) {
	runtime := &petriMoveRuntime{}
	service := internalservice.NewService(workRuntimeResolver{runtime: runtime}, nil, nil, nil)
	ctx := context.Background()

	request := work.WorkRequest{RequestID: "request-cutover"}
	if _, err := service.SubmitWorkRequestForSession(ctx, "session-1", request); err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if runtime.submitted.RequestID != request.RequestID {
		t.Fatalf("submitted request = %q, want %q", runtime.submitted.RequestID, request.RequestID)
	}

	moved, err := service.MoveWorkForSession(ctx, "session-1", "work-1", "review", "move-cutover")
	if err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	if moved.FromPlaceID != "" || moved.ToPlaceID != "" || moved.TokenID != "" {
		t.Fatalf("MoveWorkForSession = %#v, want detached move result", moved)
	}
	if moved.WorkID != "work-1" || moved.FromState != "draft" || moved.ToState != "review" {
		t.Fatalf("MoveWorkForSession = %#v, want detached work-1 draft->review", moved)
	}
}
