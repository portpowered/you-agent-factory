package wire_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func newTestEventsAppender(t *testing.T) wire.EventsAppender {
	t.Helper()
	svc, err := eventswire.NewService(logging.NoopLogger{})
	if err != nil {
		t.Fatalf("eventswire.NewService() error = %v, want nil", err)
	}
	return svc
}

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

func (stubExecution) DispatchWorkstationWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	if admitted != nil {
		admitted()
	}
	return stubExecution{}.DispatchWorkstation(ctx, request)
}

func (stubExecution) CancelWorkstationDispatch(
	context.Context,
	workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	return workers.WorkstationDispatchCancelResult{}, nil
}

func (stubExecution) Start(context.Context) error { return nil }

func (stubExecution) Publish(ctx context.Context, request workers.WorkstationDispatchRequest, accept workers.WorkstationDispatchAcceptFunc) error {
	return stubExecution{}.PublishWithAdmission(ctx, request, nil, accept)
}

func (stubExecution) PublishWithAdmission(ctx context.Context, request workers.WorkstationDispatchRequest, admitted workers.WorkstationDispatchAdmissionFunc, accept workers.WorkstationDispatchAcceptFunc) error {
	result, err := stubExecution{}.DispatchWorkstationWithAdmission(ctx, request, admitted)
	accept(context.Background(), request, result, err)
	return nil
}

func (stubExecution) Cancel(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	return workers.WorkstationDispatchCancelResult{}, nil
}

func (stubExecution) Stop(context.Context) error { return nil }

func TestNewService_ConstructsAWorkingServiceFromInjectedExecution(t *testing.T) {
	service, err := wire.NewService(stubExecution{}, newTestEventsAppender(t), logging.NoopLogger{})
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
	if _, err := wire.NewService(nil, newTestEventsAppender(t), logging.NoopLogger{}); err == nil {
		t.Fatalf("NewService(nil, ...) unexpectedly succeeded")
	}
}

func TestNewService_RejectsNilEventsAppender(t *testing.T) {
	if _, err := wire.NewService(stubExecution{}, nil, logging.NoopLogger{}); err == nil {
		t.Fatalf("NewService(execution, nil, ...) unexpectedly succeeded")
	}
}
