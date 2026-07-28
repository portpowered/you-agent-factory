package service_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workservice "github.com/portpowered/infinite-you/pkg/services/work/service"
)

type shimAdmissionRuntime struct {
	submitted work.WorkRequest
}

func (r *shimAdmissionRuntime) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	r.submitted = request
	return work.WorkRequestSubmitResult{RequestID: request.RequestID}, nil
}

func (r *shimAdmissionRuntime) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}

func (r *shimAdmissionRuntime) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	return work.ReadSnapshot{}, nil
}

type shimRuntimeResolver struct {
	runtime work.Runtime
}

func (r *shimRuntimeResolver) ResolveWorkRuntime(string) (work.Runtime, error) {
	return r.runtime, nil
}

func TestShimNewServiceDelegatesToInternalImplementation(t *testing.T) {
	t.Parallel()

	runtime := &shimAdmissionRuntime{}
	service := workservice.NewService(&shimRuntimeResolver{runtime: runtime}, nil, nil, nil)
	var root work.Service = service

	request := work.WorkRequest{RequestID: "shim-admission"}
	if _, err := root.SubmitWorkRequestForSession(context.Background(), "session-1", request); err != nil {
		t.Fatalf("SubmitWorkRequestForSession() error = %v", err)
	}
	if runtime.submitted.RequestID != request.RequestID {
		t.Fatalf("submitted request = %q, want %q", runtime.submitted.RequestID, request.RequestID)
	}
}
