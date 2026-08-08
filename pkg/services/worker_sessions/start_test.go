package workersessions_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func validStartRequestForContractTest() workersessions.InvokeSessionRequest {
	return workersessions.InvokeSessionRequest{
		ID: "worker-1",
		Execution: workers.WorkstationDispatchRequest{
			WorkstationName: "review",
			Execution: workers.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-1", WorkstationName: "review"},
			},
		},
	}
}

func TestStartRequest_Validate_AcceptsWellFormedRequest(t *testing.T) {
	if err := validStartRequestForContractTest().Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestStartRequest_Validate_RejectsEmptySessionID(t *testing.T) {
	req := validStartRequestForContractTest()
	req.ID = "   "
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Errorf("Validate() = %v, want ErrInvalidSessionID", err)
	}
}

func TestStartRequest_Validate_RejectsBlankWorkstationName(t *testing.T) {
	req := validStartRequestForContractTest()
	req.Execution.WorkstationName = ""
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidExecutionRequest) {
		t.Errorf("Validate() = %v, want ErrInvalidExecutionRequest", err)
	}
}

func TestStartRequest_Validate_RejectsBlankAttemptDispatchID(t *testing.T) {
	req := validStartRequestForContractTest()
	req.Execution.Execution.Dispatch.DispatchID = ""
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidExecutionRequest) {
		t.Errorf("Validate() = %v, want ErrInvalidExecutionRequest", err)
	}
}

func TestStartRequest_Validate_RejectsBlankNestedDispatchWorkstationName(t *testing.T) {
	req := validStartRequestForContractTest()
	req.Execution.Execution.Dispatch.WorkstationName = "   "
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidExecutionRequest) {
		t.Errorf("Validate() = %v, want ErrInvalidExecutionRequest", err)
	}
}

func TestStartRequest_Validate_RejectsMismatchedNestedDispatchWorkstationName(t *testing.T) {
	req := validStartRequestForContractTest()
	req.Execution.Execution.Dispatch.WorkstationName = "other-route"
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidExecutionRequest) {
		t.Errorf("Validate() = %v, want ErrInvalidExecutionRequest", err)
	}
}

func TestStartRequest_Validate_IsDeterministicAndDoesNotMutate(t *testing.T) {
	req := validStartRequestForContractTest()
	original := req

	firstErr := req.Validate()
	secondErr := req.Validate()

	if firstErr != nil || secondErr != nil {
		t.Fatalf("Validate() = (%v, %v), want (nil, nil)", firstErr, secondErr)
	}
	if req.ID != original.ID || req.Execution.WorkstationName != original.Execution.WorkstationName {
		t.Fatalf("Validate() mutated req: got %+v, want %+v", req, original)
	}
}
