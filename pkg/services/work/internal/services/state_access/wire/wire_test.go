package wire

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

type stubRuntime struct {
	submitResult work.WorkRequestSubmitResult
	submitErr    error
	moveResult   work.OperatorMoveResult
	moveErr      error
	snapshot     work.ReadSnapshot
	snapshotErr  error
}

func (r *stubRuntime) SubmitWorkRequest(
	_ context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	if r.submitErr != nil {
		return work.WorkRequestSubmitResult{}, r.submitErr
	}
	if r.submitResult.RequestID == "" {
		return work.WorkRequestSubmitResult{RequestID: request.RequestID, Accepted: true}, nil
	}
	return r.submitResult, nil
}

func (r *stubRuntime) MoveWork(
	_ context.Context,
	workID string,
	_ string,
	_ work.WorkStateChangeSource,
	_ string,
) (work.OperatorMoveResult, error) {
	if r.moveErr != nil {
		return work.OperatorMoveResult{}, r.moveErr
	}
	if r.moveResult.WorkID == "" {
		return work.OperatorMoveResult{WorkID: workID, FromState: "draft", ToState: "review"}, nil
	}
	return r.moveResult, nil
}

func (r *stubRuntime) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	if r.snapshotErr != nil {
		return work.ReadSnapshot{}, r.snapshotErr
	}
	return r.snapshot, nil
}

type stubRuntimeResolver struct {
	runtime work.Runtime
	err     error
}

func (r stubRuntimeResolver) ResolveWorkRuntime(string) (work.Runtime, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.runtime, nil
}

func TestNewServiceConstructsStateAccessSubservice(t *testing.T) {
	t.Parallel()

	if service := NewService(NewRuntimeSessionResolver(stubRuntimeResolver{})); service == nil {
		t.Fatal("NewService() = nil")
	}
}

func TestNewRuntimeSessionResolverNilResolverReturnsNil(t *testing.T) {
	t.Parallel()

	if resolver := NewRuntimeSessionResolver(nil); resolver != nil {
		t.Fatalf("NewRuntimeSessionResolver(nil) = %#v, want nil", resolver)
	}
}

func TestRuntimeSessionResolverResolveSessionAdapter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	resolveErr := errors.New("resolve failed")

	t.Run("resolve error", func(t *testing.T) {
		t.Parallel()
		resolver := NewRuntimeSessionResolver(stubRuntimeResolver{err: resolveErr})
		if _, err := resolver.ResolveSessionAdapter("session-1"); !errors.Is(err, resolveErr) {
			t.Fatalf("ResolveSessionAdapter() error = %v, want %v", err, resolveErr)
		}
	})

	t.Run("nil runtime", func(t *testing.T) {
		t.Parallel()
		resolver := NewRuntimeSessionResolver(stubRuntimeResolver{})
		adapter, err := resolver.ResolveSessionAdapter("session-1")
		if err != nil {
			t.Fatalf("ResolveSessionAdapter() error = %v", err)
		}
		if adapter != nil {
			t.Fatalf("adapter = %#v, want nil", adapter)
		}
	})

	t.Run("adapter fulfills runtime port", func(t *testing.T) {
		t.Parallel()
		runtime := &stubRuntime{
			submitResult: work.WorkRequestSubmitResult{RequestID: "request-1", Accepted: true},
			moveResult:   work.OperatorMoveResult{WorkID: "work-1", FromState: "draft", ToState: "review"},
			snapshot:     work.ReadSnapshot{Items: []work.ReadModel{{WorkID: "work-1"}}},
		}
		resolver := NewRuntimeSessionResolver(stubRuntimeResolver{runtime: runtime})
		adapter, err := resolver.ResolveSessionAdapter("session-1")
		if err != nil {
			t.Fatalf("ResolveSessionAdapter() error = %v", err)
		}
		if adapter == nil {
			t.Fatal("adapter = nil, want runtime adapter")
		}

		submitResult, err := adapter.SubmitWorkRequest(ctx, work.WorkRequest{RequestID: "request-1"})
		if err != nil {
			t.Fatalf("SubmitWorkRequest() error = %v", err)
		}
		if submitResult.RequestID != "request-1" || !submitResult.Accepted {
			t.Fatalf("submitResult = %#v", submitResult)
		}

		moveResult, err := adapter.MoveWork(ctx, "work-1", "review", work.WorkStateChangeSourceAPI, "move-1")
		if err != nil {
			t.Fatalf("MoveWork() error = %v", err)
		}
		if moveResult.WorkID != "work-1" || moveResult.ToState != "review" {
			t.Fatalf("moveResult = %#v", moveResult)
		}

		snapshot, err := adapter.ReadWorkSnapshot(ctx)
		if err != nil {
			t.Fatalf("ReadWorkSnapshot() error = %v", err)
		}
		if len(snapshot.Items) != 1 || snapshot.Items[0].WorkID != "work-1" {
			t.Fatalf("snapshot = %#v", snapshot)
		}
	})
}
