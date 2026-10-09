package internal_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
)

type completedStateAccess struct {
	submit   func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	move     func(context.Context, string, string, string, string) (work.OperatorMoveResult, error)
	list     func(context.Context, string, work.ListOptions) (work.ListResult, error)
	get      func(context.Context, string, string) (work.ReadModel, error)
	moveRead func(context.Context, string, string, string, string) (work.ReadModel, error)
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
		if err != wantErr || !reflect.DeepEqual(moved, work.OperatorMoveResult{WorkID: "completed-work", TokenID: "opaque"}) {
			t.Fatalf("move=%#v,error=%v", moved, err)
		}
	}
}

func (s completedStateAccess) ListWork(ctx context.Context, session string, options work.ListOptions) (work.ListResult, error) {
	return s.list(ctx, session, options)
}
func (s completedStateAccess) GetWork(ctx context.Context, session, id string) (work.ReadModel, error) {
	return s.get(ctx, session, id)
}
func (s completedStateAccess) MoveWorkAndRead(ctx context.Context, session, id, state, request string) (work.ReadModel, error) {
	return s.moveRead(ctx, session, id, state, request)
}
func TestNewServiceDelegatesReadsToCompletedStateAccess(t *testing.T) {
	t.Parallel()
	options := work.ListOptions{Name: "caller filter", SortBy: "state.type", NextToken: "opaque cursor", MaxResults: 3}
	listed := work.ListResult{Results: []work.ReadModel{{WorkID: "completed"}}, NextToken: "completed cursor"}
	read := work.ReadModel{WorkID: "completed", State: &work.State{Name: "done"}}
	for _, wantErr := range []error{nil, work.ErrWorkNotFound, &work.ValidationError{Message: "invalid query"}, context.Canceled, context.DeadlineExceeded} {
		state := completedStateAccess{
			list: func(ctx context.Context, session string, got work.ListOptions) (work.ListResult, error) {
				if ctx != t.Context() || session != "session" || !reflect.DeepEqual(got, options) {
					t.Fatal("list inputs changed")
				}
				return listed, wantErr
			},
			get: func(ctx context.Context, session, id string) (work.ReadModel, error) {
				if ctx != t.Context() || [2]string{session, id} != [2]string{"session", "work"} {
					t.Fatal("get inputs changed")
				}
				return read, wantErr
			},
			moveRead: func(ctx context.Context, session, id, state, request string) (work.ReadModel, error) {
				if ctx != t.Context() || [4]string{session, id, state, request} != [4]string{"session", "work", "done", "request"} {
					t.Fatal("move/read inputs changed")
				}
				return read, wantErr
			},
		}
		service := internalservice.NewService(nil, nil, nil, nil, nil, state, nil, nil)
		gotList, err := service.ListWork(t.Context(), "session", options)
		if err != wantErr || !reflect.DeepEqual(gotList, listed) {
			t.Fatalf("list = %#v, error = %v", gotList, err)
		}
		gotRead, err := service.GetWork(t.Context(), "session", "work")
		if err != wantErr || !reflect.DeepEqual(gotRead, read) {
			t.Fatalf("get = %#v, error = %v", gotRead, err)
		}
		gotMove, err := service.MoveWorkAndRead(t.Context(), "session", "work", "done", "request")
		if err != wantErr || !reflect.DeepEqual(gotMove, read) {
			t.Fatalf("move/read = %#v, error = %v", gotMove, err)
		}
	}
}

func (s completedStateAccess) ResolveWorkerSessionWork(ctx context.Context, session, id string) (work.WorkerSessionWork, error) {
	item, err := s.get(ctx, session, id)
	return work.WorkerSessionWork{WorkID: item.WorkID, Name: item.Name}, err
}
