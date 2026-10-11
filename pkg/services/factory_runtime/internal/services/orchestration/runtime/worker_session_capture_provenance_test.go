package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRestoredDefaultCapturePinsPhysicalOwner(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name, scope, dispatch, openingOwner string
		resolves                            bool
	}{
		{"default history", "~default", "dispatch", "original", true},
		{"foreign dispatch", "~default", "foreign", "original", false},
		{"foreign opening owner", "~default", "dispatch", "foreign", false},
		{"explicit foreign scope", "foreign", "dispatch", "original", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			scope, dispatch := scenario.scope, "dispatch"
			event := interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
				Context: interfaces.FactoryEventContext{SessionID: &scope, DispatchID: &dispatch},
				Payload: []byte(`{"workerSessionId":"worker"}`)}
			reader := &selectedCapturedIdentityReader{summary: selectedCapturedUsageSummary("worker", 4)}
			reader.summary.Capture.Catalog.FactorySessionID = "original"
			reader.summary.Capture.Opening.Payload, _ = json.Marshal(map[string]any{
				"kind": "SESSION", "phase": "STARTED", "payload": map[string]string{
					"workerSessionId": "worker", "dispatchId": scenario.dispatch, "factorySessionId": scenario.openingOwner,
				},
			})
			history := prepareRecordedObservationHistory([]interfaces.FactoryEvent{event}, nil, reader)
			service := &recordedWorkerSessionObservation{restoredWorkerScopes: history.restoredWorkerScopes}
			if got := service.selectedCaptureMatches("worker", reader.summary.Capture.Catalog); got != scenario.resolves {
				t.Fatalf("capture membership = %v, want %v; scopes=%v", got, scenario.resolves, history.restoredWorkerScopes)
			}
			if scenario.resolves {
				foreign := reader.summary.Capture.Catalog
				foreign.FactorySessionID = "foreign"
				if service.selectedCaptureMatches("worker", foreign) {
					t.Fatal("prepared owner accepted a foreign capture")
				}
				foreign = reader.summary.Capture.Catalog
				foreign.WorkerSessionID = "other"
				if service.selectedCaptureMatches("worker", foreign) {
					t.Fatal("prepared owner accepted another physical identity")
				}
				// The prepared owner stays detached from subsequent catalog reads.
				reader.summary.Capture.Catalog.FactorySessionID = "foreign"
				if history.restoredWorkerScopes["worker"] != "original" {
					t.Fatal("catalog mutation poisoned prepared owner")
				}
			}
		})
	}
}

type fakeWorkerSessionsService struct {
	execution workers.Service
}

// This execution-only double has no durable captured activity.
func (*fakeWorkerSessionsService) GetCapturedObservation(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
}

func (*fakeWorkerSessionsService) ReadLogs(context.Context, workersessions.ReadLogsRequest) (workersessions.LogPage, error) {
	return workersessions.LogPage{}, workersessions.ErrSessionNotFound
}

func (*fakeWorkerSessionsService) ReadLogsArtifact(context.Context, string, string) (io.ReadCloser, error) {
	return nil, workersessions.ErrSessionNotFound
}

func (s *fakeWorkerSessionsService) Reserve(context.Context, workersessions.ReserveRequest) (workersessions.Session, error) {
	return workersessions.Session{}, nil
}

func (s *fakeWorkerSessionsService) Get(context.Context, workersessions.GetRequest) (workersessions.Session, error) {
	return workersessions.Session{}, nil
}

func (s *fakeWorkerSessionsService) List(context.Context, workersessions.ListRequest) (workersessions.ListResult, error) {
	return workersessions.ListResult{}, nil
}

func (s *fakeWorkerSessionsService) ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	return workersessions.ListObservationsResult{}, nil
}

func (s *fakeWorkerSessionsService) GetObservation(context.Context, workersessions.GetObservationRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
}

func (s *fakeWorkerSessionsService) GetObservationByWorkerSessionID(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
}

func (s *fakeWorkerSessionsService) ListWorkerSessionObservations(context.Context, workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
	return workersessions.ListWorkerSessionObservationsResult{}, nil
}

func (s *fakeWorkerSessionsService) StreamObservations(context.Context, workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error) {
	return workersessions.ObservationSubscription{}, nil
}

func (s *fakeWorkerSessionsService) StreamObservationsByWorkerSessionID(context.Context, workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	return workersessions.ObservationSubscription{}, nil
}

func (s *fakeWorkerSessionsService) ReadTranscript(context.Context, workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	return workersessions.ReadTranscriptResult{}, nil
}

func (s *fakeWorkerSessionsService) ReadTranscriptByWorkerSessionID(context.Context, workersessions.ReadTranscriptByWorkerSessionIDRequest) (workersessions.ReadTranscriptResult, error) {
	return workersessions.ReadTranscriptResult{}, nil
}

func (s *fakeWorkerSessionsService) InvokeSession(ctx context.Context, req workersessions.InvokeSessionRequest) (workersessions.InvokeSessionResult, error) {
	handoff := workers.WorkstationDispatchRequest{
		WorkstationName: req.Execution.WorkstationName,
		Execution:       req.Execution.Execution,
	}
	executeResult, executeErr := s.execution.Execute(ctx, testExecuteRequestFromDispatch(handoff))
	dispatchResult := testDispatchResultFromExecute(handoff, executeResult, executeErr)
	return workersessions.InvokeSessionResult{
		Session:     workersessions.Session{ID: req.ID, State: workersessions.StateCompleted},
		Dispatch:    dispatchResult,
		DispatchErr: executeErr,
	}, nil
}

func (s *fakeWorkerSessionsService) Start(ctx context.Context, req workersessions.StartRequest) (workersessions.StartResult, error) {
	result, err := s.InvokeSession(ctx, workersessions.InvokeSessionRequest{
		ID:        req.ID,
		Execution: req.Execution,
		Retry:     req.Retry,
	})
	return workersessions.StartResult{Session: result.Session}, err
}

func (s *fakeWorkerSessionsService) Continue(context.Context, workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
	return workersessions.ContinueResult{}, nil
}

func (s *fakeWorkerSessionsService) Interrupt(context.Context, workersessions.InterruptRequest) (workersessions.InterruptResult, error) {
	return workersessions.InterruptResult{}, nil
}

func (s *fakeWorkerSessionsService) PublishRecord(context.Context, workersessions.PublishRecordRequest) (workersessions.PublishRecordResult, error) {
	return workersessions.PublishRecordResult{}, nil
}

func (s *fakeWorkerSessionsService) AssociateProviderSession(context.Context, workersessions.ProviderSessionAssociationRequest) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *fakeWorkerSessionsService) ObserveProviderSession(context.Context, workersessions.ProviderSessionObservationRequest) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *fakeWorkerSessionsService) EnsureProviderBinding(context.Context, workersessions.ProviderBindingRequest) (workersessions.ProviderBindingResult, error) {
	return workersessions.ProviderBindingResult{}, nil
}

func (s *fakeWorkerSessionsService) WorkerSessionIDForDispatch(_ context.Context, dispatchID string) (string, error) {
	return dispatchID, nil
}

func (s *fakeWorkerSessionsService) Pause(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *fakeWorkerSessionsService) Resume(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *fakeWorkerSessionsService) Cancel(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *fakeWorkerSessionsService) Terminate(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

type testWorkstationBoundary struct {
	executors map[string]workers.WorkerExecutor
}

type testWorkstationRequestExecutor struct {
	executors map[string]workers.WorkerExecutor
}

func (executor testWorkstationRequestExecutor) Execute(
	ctx context.Context,
	request workers.WorkstationExecutionRequest,
) (result workers.WorkResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := &workers.WorkerExecutorPanicError{Cause: recovered}
			result = workers.WorkResult{
				DispatchID:   request.Dispatch.DispatchID,
				TransitionID: request.Dispatch.TransitionID,
				Outcome:      workers.OutcomeFailed,
				Error:        panicErr.Error(),
			}
			err = panicErr
		}
	}()
	workerType := request.WorkerType
	if workerType == "" {
		workerType = request.Dispatch.WorkerType
	}
	worker := executor.executors[workerType]
	if worker == nil {
		return workers.WorkResult{}, fmt.Errorf("no executor registered for worker type %q", workerType)
	}
	return worker.Execute(ctx, request.Dispatch)
}

func (b *testWorkstationBoundary) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	legacy := testLegacyRequestFromExecute(request)
	result, err := (testWorkstationRequestExecutor{executors: b.executors}).Execute(ctx, legacy.Execution)
	return workers.ExecuteResult{
		Correlation:  request.Correlation,
		Outcome:      executeOutcomeFromWorkResult(result),
		Cancellation: result.Cancellation.Clone(),
		Failure:      executeFailureFromWorkResult(result),
		Output:       workers.ProposedOutputFromLegacyWorkResult(result),
	}, err
}

func (*testWorkstationBoundary) InvokeModel(
	context.Context,
	string,
	modelinference.Request,
) (modelinference.Result, error) {
	return modelinference.Result{}, errors.New("test Workers service does not support direct model invocation")
}
