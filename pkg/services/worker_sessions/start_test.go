package workersessions_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func validStartRequestForContractTest() workersessions.StartRequest {
	return workersessions.StartRequest{
		RequestID: "request-1",
		ID:        "worker-1",
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

func TestStartRequest_Validate_RejectsMissingRequestID(t *testing.T) {
	req := validStartRequestForContractTest()
	req.RequestID = " \t"
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidStartRequestID) {
		t.Fatalf("Validate() = %v, want ErrInvalidStartRequestID", err)
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

// TestRetryPolicy_AttemptsCollapsesUnsetAndSingleAttempt pins the property
// every caller of InvokeSession leans on: the zero value is one attempt, so a
// Petri dispatch that never asked for retry keeps the single-attempt behaviour
// it has always had.
func TestRetryPolicy_AttemptsCollapsesUnsetAndSingleAttempt(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		policy workersessions.RetryPolicy
		want   int
	}{
		{name: "unset", policy: workersessions.RetryPolicy{}, want: 1},
		{name: "explicit single attempt", policy: workersessions.RetryPolicy{MaxAttempts: 1}, want: 1},
		{name: "budgeted", policy: workersessions.RetryPolicy{MaxAttempts: 3}, want: 3},
	} {
		if got := testCase.policy.Attempts(); got != testCase.want {
			t.Fatalf("%s: Attempts() = %d, want %d", testCase.name, got, testCase.want)
		}
	}
}

// TestRetryPolicy_RejectsANegativeBudget keeps a caller bug a rejection rather
// than a silent clamp: a negative attempt budget cannot be what anyone meant.
func TestRetryPolicy_RejectsANegativeBudget(t *testing.T) {
	err := workersessions.RetryPolicy{MaxAttempts: -1}.Validate()
	if !errors.Is(err, workersessions.ErrInvalidExecutionRequest) {
		t.Fatalf("Validate() error = %v, want ErrInvalidExecutionRequest", err)
	}
	if err := (workersessions.RetryPolicy{MaxAttempts: 0}).Validate(); err != nil {
		t.Fatalf("Validate() on the zero value = %v, want nil", err)
	}
}

// TestInvokeSessionRequest_RejectsANegativeRetryBudget proves the request
// carries that rejection outward, so no registry mutation or Workers call can
// happen for a malformed budget.
func TestInvokeSessionRequest_RejectsANegativeRetryBudget(t *testing.T) {
	req := workersessions.InvokeSessionRequest{
		ID: "worker-1",
		Execution: workers.WorkstationDispatchRequest{
			WorkstationName: "review",
			Execution: workers.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-1", WorkstationName: "review"},
			},
		},
		Retry: workersessions.RetryPolicy{MaxAttempts: -1},
	}
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidExecutionRequest) {
		t.Fatalf("Validate() error = %v, want ErrInvalidExecutionRequest", err)
	}
}
