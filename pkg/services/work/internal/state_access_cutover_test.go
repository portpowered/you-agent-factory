package internal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
)

type completedStateAccess struct {
	stateaccess.Service
	submit func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	move   func(context.Context, string, string, string, string) (work.OperatorMoveResult, error)
}

func (s completedStateAccess) SubmitWorkRequestForSession(ctx context.Context, id string, input work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return s.submit(ctx, id, input)
}
func (s completedStateAccess) MoveWorkForSession(ctx context.Context, id, workID, state, requestID string) (work.OperatorMoveResult, error) {
	return s.move(ctx, id, workID, state, requestID)
}
func TestNewServiceDelegatesToCompletedStateAccess(t *testing.T) {
	t.Parallel()
	failure := errors.New("completed state failure")
	for _, wantErr := range []error{nil, failure} {
		state := completedStateAccess{
			submit: func(ctx context.Context, id string, input work.WorkRequest) (work.WorkRequestSubmitResult, error) {
				if ctx != t.Context() || id != "session" || input.RequestID != "request" {
					t.Fatal("admission inputs changed")
				}
				return work.WorkRequestSubmitResult{RequestID: "completed-request", Accepted: true}, wantErr
			},
			move: func(ctx context.Context, id, workID, state, requestID string) (work.OperatorMoveResult, error) {
				if ctx != t.Context() || id != "session" || workID != "work" || state != "done" || requestID != "move" {
					t.Fatal("move inputs changed")
				}
				return work.OperatorMoveResult{WorkID: "completed-work", TokenID: "opaque"}, wantErr
			},
		}
		service := internalservice.NewService(nil, nil, nil, nil, nil, state, nil, nil)
		submitted, err := service.SubmitWorkRequestForSession(t.Context(), "session", work.WorkRequest{RequestID: "request"})
		if err != wantErr || submitted.RequestID != "completed-request" || !submitted.Accepted {
			t.Fatalf("submission=%#v,error=%v", submitted, err)
		}
		moved, err := service.MoveWorkForSession(t.Context(), "session", "work", "done", "move")
		if err != wantErr || moved.WorkID != "completed-work" || moved.TokenID != "opaque" {
			t.Fatalf("move=%#v,error=%v", moved, err)
		}
	}
}
