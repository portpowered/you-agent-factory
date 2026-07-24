package work_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// rootServiceFake is a peer-shaped Work root Service that uses only Work-owned
// request, result, value, and typed-error contracts.
type rootServiceFake struct {
	submitResult work.WorkRequestSubmitResult
	submitErr    error
	moveResult   work.OperatorMoveResult
	moveErr      error
	listResult   work.ListResult
	listErr      error
	getResult    work.ReadModel
	getErr       error
	movedRead    work.ReadModel
	movedReadErr error

	lastSessionID string
	lastRequest   work.WorkRequest
	lastListOpts  work.ListOptions
	lastWorkID    string
	lastStateName string
	lastRequestID string
}

func (f *rootServiceFake) SubmitWorkRequestForSession(
	_ context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	f.lastSessionID = sessionID
	f.lastRequest = request
	return f.submitResult, f.submitErr
}

func (f *rootServiceFake) MoveWorkForSession(
	_ context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.OperatorMoveResult, error) {
	f.lastSessionID = sessionID
	f.lastWorkID = workID
	f.lastStateName = stateName
	f.lastRequestID = requestID
	return f.moveResult, f.moveErr
}

func (f *rootServiceFake) ListWork(
	_ context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	f.lastSessionID = sessionID
	f.lastListOpts = options
	return f.listResult, f.listErr
}

func (f *rootServiceFake) GetWork(
	_ context.Context,
	sessionID string,
	id string,
) (work.ReadModel, error) {
	f.lastSessionID = sessionID
	f.lastWorkID = id
	return f.getResult, f.getErr
}

func (f *rootServiceFake) MoveWorkAndRead(
	_ context.Context,
	sessionID string,
	id string,
	stateName string,
	requestID string,
) (work.ReadModel, error) {
	f.lastSessionID = sessionID
	f.lastWorkID = id
	f.lastStateName = stateName
	f.lastRequestID = requestID
	return f.movedRead, f.movedReadErr
}

var _ work.Service = (*rootServiceFake)(nil)

func TestServiceRootContract_FakeImplementsAndExercisesSeam(t *testing.T) {
	fake := &rootServiceFake{
		submitResult: work.WorkRequestSubmitResult{
			RequestID: "request-1",
			TraceID:   "trace-1",
			Accepted:  true,
			Works: []work.WorkRequestSubmittedWork{{
				Name:         "story-1",
				WorkTypeName: "story",
				WorkID:       "work-1",
			}},
		},
		moveResult: work.OperatorMoveResult{
			WorkID:     "work-1",
			WorkTypeID: "story",
			FromState:  "draft",
			ToState:    "review",
		},
		listResult: work.ListResult{
			Results: []work.ReadModel{{
				CursorID:     "work-1",
				Name:         "story-1",
				WorkID:       "work-1",
				WorkTypeName: "story",
				State:        &work.State{Name: "review", Type: work.StateTypeProcessing},
			}},
			MaxResults: work.DefaultListMaxResults,
		},
		getResult: work.ReadModel{
			CursorID:     "work-1",
			Name:         "story-1",
			WorkID:       "work-1",
			WorkTypeName: "story",
			State:        &work.State{Name: "review", Type: work.StateTypeProcessing},
		},
		movedRead: work.ReadModel{
			CursorID:     "work-1",
			Name:         "story-1",
			WorkID:       "work-1",
			WorkTypeName: "story",
			State:        &work.State{Name: "done", Type: work.StateTypeTerminal},
		},
	}

	// Peers consume only the singular root Service seam.
	var service work.Service = fake
	ctx := context.Background()

	submit, err := service.SubmitWorkRequestForSession(ctx, "session-1", work.WorkRequest{
		RequestID: "request-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "story-1",
			WorkTypeID: "story",
			State:      "draft",
		}},
	})
	if err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if !submit.Accepted || submit.RequestID != "request-1" || len(submit.Works) != 1 {
		t.Fatalf("submit result = %#v, want accepted request-1 with one work", submit)
	}
	if fake.lastSessionID != "session-1" || fake.lastRequest.RequestID != "request-1" {
		t.Fatalf("submit routed = (%q, %q)", fake.lastSessionID, fake.lastRequest.RequestID)
	}

	move, err := service.MoveWorkForSession(ctx, "session-1", "work-1", "review", "move-1")
	if err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	if move.WorkID != "work-1" || move.ToState != "review" {
		t.Fatalf("move result = %#v, want work-1 -> review", move)
	}

	listed, err := service.ListWork(ctx, "session-1", work.ListOptions{WorkTypeName: "story"})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(listed.Results) != 1 || listed.Results[0].WorkID != "work-1" {
		t.Fatalf("list result = %#v, want one work-1 entry", listed)
	}
	if fake.lastListOpts.WorkTypeName != "story" {
		t.Fatalf("list options = %#v, want workTypeName=story", fake.lastListOpts)
	}

	got, err := service.GetWork(ctx, "session-1", "work-1")
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if got.WorkID != "work-1" || got.State == nil || got.State.Name != "review" {
		t.Fatalf("get result = %#v, want work-1 in review", got)
	}

	moved, err := service.MoveWorkAndRead(ctx, "session-1", "work-1", "done", "move-2")
	if err != nil {
		t.Fatalf("MoveWorkAndRead: %v", err)
	}
	if moved.WorkID != "work-1" || moved.State == nil || moved.State.Type != work.StateTypeTerminal {
		t.Fatalf("move-and-read result = %#v, want terminal work-1", moved)
	}
}

func TestServiceRootContract_TypedFailuresRemainDistinguishable(t *testing.T) {
	fake := &rootServiceFake{
		getErr:  work.ErrWorkNotFound,
		moveErr: work.ErrMoveWorkRequestAlreadyApplied,
	}
	var service work.Service = fake
	ctx := context.Background()

	_, err := service.GetWork(ctx, "session-1", "missing")
	if !errors.Is(err, work.ErrWorkNotFound) {
		t.Fatalf("GetWork error = %v, want ErrWorkNotFound", err)
	}

	_, err = service.MoveWorkForSession(ctx, "session-1", "work-1", "done", "dup-move")
	if !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
		t.Fatalf("MoveWorkForSession error = %v, want ErrMoveWorkRequestAlreadyApplied", err)
	}
}
