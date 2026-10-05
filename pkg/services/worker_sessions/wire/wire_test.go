package wire_test

import (
	"context"
	"encoding/json"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type unusedEventsAppender struct {
	t *testing.T
}

func (appender unusedEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	appender.t.Fatal("reservation must not publish an event")
	return events.AppendResult{}, nil
}

type stubExecution struct{}

type unavailableProviderSessions struct {
	providersessions.Service
}

func (unavailableProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
}

func (stubExecution) Execute(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
}

func (stubExecution) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, workers.ErrExecuteUnavailable
}

func TestNewService_ConstructsAWorkingServiceFromInjectedExecution(t *testing.T) {
	t.Parallel()
	clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Second)
	service, err := wire.NewService(stubExecution{}, unusedEventsAppender{t: t}, logging.NoopLogger{}, clock, clock, unavailableProviderSessions{}, nil, nil, unavailableWorkerControlStore{})
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

// unavailableWorkerControlStore is a controlled persistence outage for tests
// that do not own durable control behavior. Every operation fails explicitly;
// it must never be used as evidence that an intent was committed or replayed.
type unavailableWorkerControlStore struct{}

func (unavailableWorkerControlStore) BeginWorkerControlOperation(context.Context, recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	return recordings.WorkerControlOperationRecord{}, false, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) AdvanceWorkerControlOperation(context.Context, recordings.WorkerControlOperationRecord, uint64) (recordings.WorkerControlOperationRecord, error) {
	return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) LoadWorkerControlOperation(context.Context, recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ListWorkerControlOperations(context.Context, recordings.WorkerControlTarget) ([]recordings.WorkerControlOperationRecord, error) {
	return nil, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) PersistWorkerControlInput(context.Context, recordings.WorkerControlOperationKey, json.RawMessage) (string, error) {
	return "", recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ReadWorkerControlInput(context.Context, recordings.WorkerControlOperationKey, string) (json.RawMessage, error) {
	return nil, recordings.ErrWorkerRecordingPersistence
}
