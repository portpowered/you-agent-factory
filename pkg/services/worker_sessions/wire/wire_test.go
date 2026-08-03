package wire_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type stubExecution struct{}

func (stubExecution) StartWorkstationPool(
	context.Context,
	workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{}, nil
}

func (stubExecution) StopWorkstationPool(context.Context) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{}, nil
}

func (stubExecution) DispatchWorkstation(
	context.Context,
	workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	return workers.WorkstationDispatchResult{Result: workers.WorkResult{Outcome: workers.OutcomeAccepted}}, nil
}

func (stubExecution) CancelWorkstationDispatch(
	context.Context,
	workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	return workers.WorkstationDispatchCancelResult{}, nil
}

func TestNewService_ConstructsAWorkingServiceFromInjectedExecution(t *testing.T) {
	service, err := wire.NewService(stubExecution{}, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewService() error = %v, want nil", err)
	}

	session, err := service.Reserve(context.Background(), workersessions.ReserveRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	if session.ID != "worker-1" || session.State != workersessions.StateReserved {
		t.Fatalf("Reserve() = %+v, want ID=worker-1 State=RESERVED", session)
	}
}

func TestNewService_RejectsNilExecution(t *testing.T) {
	if _, err := wire.NewService(nil, logging.NoopLogger{}); err == nil {
		t.Fatalf("NewService(nil, ...) unexpectedly succeeded")
	}
}
