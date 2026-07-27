package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/internal/service"
)

type recordingSessionAdapter struct {
	submitted work.WorkRequest
	movedID   string
	source    work.WorkStateChangeSource
	requestID string
	submitErr error
	moveErr   error
}

func (a *recordingSessionAdapter) SubmitWorkRequest(
	_ context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	a.submitted = request
	if a.submitErr != nil {
		return work.WorkRequestSubmitResult{}, a.submitErr
	}
	return work.WorkRequestSubmitResult{
		RequestID: request.RequestID,
		Accepted:  true,
		Works: []work.WorkRequestSubmittedWork{{
			Name:         "story-1",
			WorkTypeName: "story",
			WorkID:       "work-1",
		}},
	}, nil
}

func (a *recordingSessionAdapter) MoveWork(
	_ context.Context,
	workID string,
	_ string,
	source work.WorkStateChangeSource,
	requestID string,
) (work.OperatorMoveResult, error) {
	a.movedID, a.source, a.requestID = workID, source, requestID
	if a.moveErr != nil {
		return work.OperatorMoveResult{}, a.moveErr
	}
	return work.OperatorMoveResult{
		WorkID:     workID,
		WorkTypeID: "story",
		FromState:  "draft",
		ToState:    "review",
		FromPlaceID: "story:draft",
		ToPlaceID:   "story:review",
		TokenID:     "tok-1",
	}, nil
}

type stubSessionResolver struct {
	adapter stateaccess.SessionAdapter
	err     error
}

func (r stubSessionResolver) ResolveSessionAdapter(string) (stateaccess.SessionAdapter, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.adapter, nil
}

func TestSubmitWorkRequestForSessionReturnsDetachedResult(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{}
	svc := internalservice.New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()
	request := work.WorkRequest{RequestID: "request-1"}

	result, err := svc.SubmitWorkRequestForSession(ctx, "session-1", request)
	if err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if !result.Accepted || result.RequestID != "request-1" || len(result.Works) != 1 {
		t.Fatalf("result = %#v, want detached accepted submit facts", result)
	}
	if adapter.submitted.RequestID != "request-1" {
		t.Fatalf("submitted request = %#v", adapter.submitted)
	}
}

func TestMoveWorkForSessionReturnsDetachedResult(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{}
	svc := internalservice.New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()

	result, err := svc.MoveWorkForSession(ctx, "session-1", "work-1", "review", "move-1")
	if err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	if result.WorkID != "work-1" || result.FromState != "draft" || result.ToState != "review" {
		t.Fatalf("result = %#v, want detached draft->review move facts", result)
	}
	if result.FromPlaceID != "" || result.ToPlaceID != "" || result.TokenID != "" {
		t.Fatalf("result leaked Petri fields: %#v", result)
	}
	if adapter.movedID != "work-1" || adapter.source != work.WorkStateChangeSourceAPI || adapter.requestID != "move-1" {
		t.Fatalf("adapter move = (%q, %q, %q)", adapter.movedID, adapter.source, adapter.requestID)
	}
}

func TestMoveWorkForSessionPropagatesAlreadyAppliedFailure(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{moveErr: work.ErrMoveWorkRequestAlreadyApplied}
	svc := internalservice.New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()

	_, err := svc.MoveWorkForSession(ctx, "session-1", "work-1", "done", "dup-move")
	if !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
		t.Fatalf("error = %v, want ErrMoveWorkRequestAlreadyApplied", err)
	}
}

func TestResolveSessionResolverError(t *testing.T) {
	t.Parallel()

	resolverErr := errors.New("session missing")
	svc := internalservice.New(stubSessionResolver{err: resolverErr})
	ctx := context.Background()

	_, err := svc.SubmitWorkRequestForSession(ctx, "missing", work.WorkRequest{})
	if !errors.Is(err, resolverErr) {
		t.Fatalf("error = %v, want resolver error", err)
	}
}
