package internal_test

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
)

type internalAdmissionRuntime struct {
	submitted work.WorkRequest
}

func (r *internalAdmissionRuntime) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	r.submitted = request
	return work.WorkRequestSubmitResult{RequestID: request.RequestID}, nil
}

func (r *internalAdmissionRuntime) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}

func (r *internalAdmissionRuntime) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	return work.ReadSnapshot{}, nil
}

type internalRuntimeResolver struct {
	runtime work.Runtime
}

func (r *internalRuntimeResolver) ResolveWorkRuntime(string) (work.Runtime, error) {
	return r.runtime, nil
}

func TestNewServiceSatisfiesPublishedWorkRoot(t *testing.T) {
	t.Parallel()

	runtime := &internalAdmissionRuntime{}
	service := internalservice.NewService(&internalRuntimeResolver{runtime: runtime}, nil, nil, nil)
	var root work.Service = service

	request := work.WorkRequest{RequestID: "internal-root-admission"}
	if _, err := root.SubmitWorkRequestForSession(context.Background(), "session-1", request); err != nil {
		t.Fatalf("SubmitWorkRequestForSession() error = %v", err)
	}
	if runtime.submitted.RequestID != request.RequestID {
		t.Fatalf("submitted request = %q, want %q", runtime.submitted.RequestID, request.RequestID)
	}
}

type internalRootOnlyRuntime struct{ factoryruntime.Service }

func TestNewReturnsSessionScopedService(t *testing.T) {
	t.Parallel()

	_ = internalRootOnlyRuntime{}
	_ = internalservice.New
}
