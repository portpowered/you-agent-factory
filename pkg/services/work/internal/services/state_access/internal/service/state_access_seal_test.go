package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
)

// TestStateAccessSealSubmitAndMovePipeline seals IMP-WORK-04 story 001 focused
// proof: detached submit/move success and already-applied move typed failure
// through one state_access Service instance and a private Session adapter fake.
func TestStateAccessSealSubmitAndMovePipeline(t *testing.T) {
	t.Parallel()

	adapter := &sealSessionAdapter{}
	svc := New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()

	submitted, err := svc.SubmitWorkRequestForSession(ctx, "session-seal", work.WorkRequest{
		RequestID: "request-seal-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "story-1",
			WorkTypeID: "story",
		}},
	})
	if err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if !submitted.Accepted || submitted.RequestID != "request-seal-1" {
		t.Fatalf("submit result = %#v, want accepted request-seal-1", submitted)
	}
	if submitted.Works[0].WorkID != "work-seal-1" {
		t.Fatalf("submit works = %#v, want detached work identity", submitted.Works)
	}

	moved, err := svc.MoveWorkForSession(ctx, "session-seal", "work-seal-1", "review", "move-seal-1")
	if err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	if moved.WorkID != "work-seal-1" || moved.FromState != "draft" || moved.ToState != "review" {
		t.Fatalf("move result = %#v, want detached draft->review", moved)
	}
	if moved.FromPlaceID != "" || moved.TokenID != "" {
		t.Fatalf("move result leaked Petri fields: %#v", moved)
	}

	_, err = svc.MoveWorkForSession(ctx, "session-seal", "work-seal-1", "done", "dup-move")
	if err != nil {
		t.Fatalf("first dup-move: %v", err)
	}
	_, err = svc.MoveWorkForSession(ctx, "session-seal", "work-seal-1", "done", "dup-move")
	if !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
		t.Fatalf("duplicate move error = %v, want ErrMoveWorkRequestAlreadyApplied", err)
	}
}

type sealSessionAdapter struct {
	appliedMoves map[string]bool
}

func (a *sealSessionAdapter) SubmitWorkRequest(
	_ context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{
		RequestID: request.RequestID,
		Accepted:  true,
		Works: []work.WorkRequestSubmittedWork{{
			Name:         "story-1",
			WorkTypeName: "story",
			WorkID:       "work-seal-1",
		}},
	}, nil
}

func (a *sealSessionAdapter) MoveWork(
	_ context.Context,
	workID string,
	stateName string,
	_ work.WorkStateChangeSource,
	requestID string,
) (work.OperatorMoveResult, error) {
	if a.appliedMoves == nil {
		a.appliedMoves = make(map[string]bool)
	}
	if a.appliedMoves[requestID] {
		return work.OperatorMoveResult{}, work.ErrMoveWorkRequestAlreadyApplied
	}
	a.appliedMoves[requestID] = true
	fromState := "draft"
	if stateName == "done" {
		fromState = "review"
	}
	return work.OperatorMoveResult{
		WorkID:      workID,
		WorkTypeID:  "story",
		FromState:   fromState,
		ToState:     stateName,
		FromPlaceID: "story:" + fromState,
		ToPlaceID:   "story:" + stateName,
		TokenID:     "tok-seal",
	}, nil
}

type stubSessionResolver struct {
	adapter stateaccess.SessionAdapter
}

func (r stubSessionResolver) ResolveSessionAdapter(string) (stateaccess.SessionAdapter, error) {
	return r.adapter, nil
}
