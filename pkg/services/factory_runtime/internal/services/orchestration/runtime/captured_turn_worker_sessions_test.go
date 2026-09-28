package runtime

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// capturedTurnWorkerSessions supplies the Worker Sessions contract consumed by
// Runtime while keeping execution and each session's exact reference observable.
type capturedTurnWorkerSessions struct {
	*fakeWorkerSessionsService
	mu       sync.Mutex
	sessions map[string]*capturedTurnSession
}

type capturedTurnSession struct {
	request workersessions.InvokeSessionRequest
	session workersessions.Session
	resume  chan workers.WorkstationDispatchRequest
}

func newCapturedTurnWorkerSessions(execution workers.Service) *capturedTurnWorkerSessions {
	return &capturedTurnWorkerSessions{
		fakeWorkerSessionsService: &fakeWorkerSessionsService{execution: execution},
		sessions:                  make(map[string]*capturedTurnSession),
	}
}

func (s *capturedTurnWorkerSessions) InvokeSession(ctx context.Context, request workersessions.InvokeSessionRequest) (workersessions.InvokeSessionResult, error) {
	entry := &capturedTurnSession{
		request: request,
		session: workersessions.Session{ID: request.ID, State: workersessions.StateRunning},
		resume:  make(chan workers.WorkstationDispatchRequest, 1),
	}
	s.mu.Lock()
	s.sessions[request.ID] = entry
	s.mu.Unlock()
	dispatch := request.Execution
	for {
		result, err := s.execution.Execute(ctx, testExecuteRequestFromDispatch(dispatch))
		converted := testDispatchResultFromExecute(dispatch, result, err)
		if result.Failure != nil {
			converted.Result.ProviderContinuationFailureKind = result.Failure.ProviderContinuationFailureKind
		}
		s.mu.Lock()
		state := entry.session.State
		if state == workersessions.StatePaused {
			s.mu.Unlock()
			dispatch = <-entry.resume
			continue
		}
		if state != workersessions.StateTerminated {
			if converted.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCompleted {
				entry.session.State = workersessions.StateCompleted
				entry.session.Result = &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
			} else {
				entry.session.State = workersessions.StateFailed
				entry.session.Result = &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeFailed,
					Cause: &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure,
						Detail:                          "controlled Workers execution failed",
						ProviderContinuationFailureKind: converted.Result.ProviderContinuationFailureKind}}
			}
		}
		session := entry.session.Clone()
		s.mu.Unlock()
		return workersessions.InvokeSessionResult{Session: session, Dispatch: converted, DispatchErr: err}, nil
	}
}

func (s *capturedTurnWorkerSessions) Get(_ context.Context, request workersessions.GetRequest) (workersessions.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.sessions[request.ID]
	if entry == nil {
		return workersessions.Session{}, workersessions.ErrSessionNotFound
	}
	return entry.session.Clone(), nil
}

func (s *capturedTurnWorkerSessions) AssociateProviderSession(_ context.Context, request workersessions.ProviderSessionAssociationRequest) (workersessions.ProviderSessionAssociationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.sessions[request.WorkerSessionID]
	association := workersessions.ProviderSessionAssociation{
		WorkerSessionID: request.WorkerSessionID, DispatchID: request.DispatchID,
		AttemptID: request.DispatchID, TurnID: entry.request.Execution.Execution.Dispatch.Execution.RequestID,
		Reference: request.Reference,
	}
	entry.session.ProviderSessionAssociation = &association
	return workersessions.ProviderSessionAssociationResult{Association: association, Outcome: workersessions.ProviderSessionAssociationOutcomeAccepted}, nil
}

func (s *capturedTurnWorkerSessions) Pause(ctx context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	s.mu.Lock()
	entry := s.sessions[request.ID]
	entry.session.State = workersessions.StatePaused
	dispatchID := entry.request.Execution.Execution.Dispatch.DispatchID
	session := entry.session.Clone()
	s.mu.Unlock()
	err := s.cancel(ctx, dispatchID)
	return workersessions.ControlResult{Session: session, Action: workersessions.ControlActionPause, Outcome: workersessions.ControlOutcomeApplied, DispatchID: dispatchID}, err
}

func (s *capturedTurnWorkerSessions) Resume(_ context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	s.mu.Lock()
	entry := s.sessions[request.ID]
	dispatch := entry.request.Execution
	dispatch.Execution.Dispatch.DispatchID += "/resume/1"
	reference := entry.session.ProviderSessionAssociation.Reference.ContinuationRef()
	dispatch.Execution.Continuation = &reference
	entry.session.State = workersessions.StateRunning
	session := entry.session.Clone()
	entry.resume <- dispatch
	s.mu.Unlock()
	return workersessions.ControlResult{Session: session, Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeApplied, DispatchID: dispatch.Execution.Dispatch.DispatchID}, nil
}

func (s *capturedTurnWorkerSessions) Terminate(ctx context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	s.mu.Lock()
	entry := s.sessions[request.ID]
	entry.session.State = workersessions.StateTerminated
	dispatchID := entry.request.Execution.Execution.Dispatch.DispatchID
	session := entry.session.Clone()
	s.mu.Unlock()
	err := s.cancel(ctx, dispatchID)
	return workersessions.ControlResult{Session: session, Action: workersessions.ControlActionTerminate, Outcome: workersessions.ControlOutcomeApplied, DispatchID: dispatchID}, err
}

func (s *capturedTurnWorkerSessions) cancel(ctx context.Context, dispatchID string) error {
	switch execution := s.execution.(type) {
	case *synchronousFanOutExecution:
		return execution.cancel(ctx, dispatchID)
	case *continuationFanOutExecution:
		execution.cancelInitial(dispatchID)
		<-execution.cancelCalls
	}
	return nil
}

var _ workersessions.Service = (*capturedTurnWorkerSessions)(nil)
