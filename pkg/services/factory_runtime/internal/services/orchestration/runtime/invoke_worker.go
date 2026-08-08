package runtime

import (
	"context"
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// InvokeWorker runs one orchestrator-resolved Worker through the same Worker
// Sessions supervision a Petri dispatch gets.
//
// The body is deliberately the same three steps as startThroughWorkerSessions:
// reserve the Worker Session, commit the dispatch/Worker Session association to
// this runtime's canonical Factory Events, then invoke. Committing the
// association here -- on the runtime that owns the ledger -- is the whole
// reason this is an operation rather than a collaborator handed to callers: the
// transport opens a tool call from that event and from nothing else, so an
// association recorded anywhere but this ledger is invisible.
//
// Unlike the Petri path it names workers.ProviderInvocationRoute rather than an
// authored workstation, and it passes the caller's attempt budget through
// rather than defaulting to one. Those two differences are the entire
// difference between a JavaScript workflow child and a Petri Worker.
func (f *factoryImpl) InvokeWorker(
	ctx context.Context,
	req factory.InvokeWorkerRequest,
) (factory.InvokeWorkerResult, error) {
	if err := req.Validate(); err != nil {
		return factory.InvokeWorkerResult{}, err
	}
	if f == nil || f.cfg == nil || f.cfg.workerSessions == nil || f.eventHistory == nil {
		return factory.InvokeWorkerResult{}, factory.ErrNotRunning
	}

	dispatchID := strings.TrimSpace(req.DispatchID)
	sessionID := dispatchID
	execution := providerInvocationExecutionRequest(f, req, dispatchID)

	if _, err := f.cfg.workerSessions.Reserve(
		context.WithoutCancel(ctx),
		workersessions.ReserveRequest{ID: sessionID},
	); err != nil {
		return factory.InvokeWorkerResult{}, err
	}
	f.eventHistory.RecordDispatchWorkerSessionAssociation(
		f.currentTick(),
		dispatchID,
		sessionID,
		execution.Execution.Dispatch.Execution.RequestID,
		f.cfg.clock.Now(),
	)

	result, err := f.cfg.workerSessions.InvokeSession(
		context.WithoutCancel(ctx),
		workersessions.InvokeSessionRequest{
			ID:        sessionID,
			Execution: execution,
			Retry:     workersessions.RetryPolicy{MaxAttempts: req.MaxAttempts},
		},
	)
	if err != nil {
		return factory.InvokeWorkerResult{}, err
	}
	return invokeWorkerResultFrom(dispatchID, result), nil
}

func (f *factoryImpl) currentTick() int {
	if f == nil || f.engine == nil {
		return 0
	}
	return f.engine.GetRuntimeStateSnapshot().TickCount
}

// providerInvocationExecutionRequest builds the resolved Workers execution
// request for one caller-resolved Worker. Every selection is copied through
// unchanged: this route exists precisely because the caller, not a workstation
// definition, already decided them.
func providerInvocationExecutionRequest(
	f *factoryImpl,
	req factory.InvokeWorkerRequest,
	dispatchID string,
) workers.WorkstationDispatchRequest {
	requestID := ""
	if f.cfg != nil && f.cfg.workflowContext != nil {
		requestID = strings.TrimSpace(f.cfg.workflowContext.SessionID)
	}
	dispatch := work.WorkDispatch{
		DispatchID:      dispatchID,
		WorkstationName: workers.ProviderInvocationRoute,
		WorkerType:      strings.TrimSpace(req.Label),
		Execution: work.ExecutionMetadata{
			RequestID: requestID,
		},
	}
	return workers.WorkstationDispatchRequest{
		WorkstationName: workers.ProviderInvocationRoute,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:         dispatch,
			WorkerType:       strings.TrimSpace(req.Label),
			RunnerID:         strings.TrimSpace(req.RunnerID),
			ExecutorProvider: strings.TrimSpace(req.ExecutorProvider),
			FactorySessionID: requestID,
			SystemPrompt:     req.SystemPrompt,
			UserMessage:      req.Prompt,
			OutputSchema:     req.OutputSchema,
			Model:            strings.TrimSpace(req.Model),
			ModelProvider:    strings.TrimSpace(req.ModelProvider),
			ReasoningEffort:  strings.TrimSpace(req.ReasoningEffort),
			WorkingDirectory: strings.TrimSpace(req.WorkingDirectory),
		},
	}
}

// invokeWorkerResultFrom narrows one Worker Sessions outcome onto the caller
// contract. Only the bounded classification and the provider's own output
// cross: InvocationResult-style diagnostics stay inside Workers, because they
// can carry command lines and credentials.
func invokeWorkerResultFrom(
	dispatchID string,
	result workersessions.InvokeSessionResult,
) factory.InvokeWorkerResult {
	outcome := factory.InvokeWorkerOutcomeFailed
	switch result.Session.State {
	case workersessions.StateCompleted:
		outcome = factory.InvokeWorkerOutcomeCompleted
	case workersessions.StateCanceled, workersessions.StateTerminated:
		outcome = factory.InvokeWorkerOutcomeCanceled
	}

	invoked := factory.InvokeWorkerResult{
		DispatchID:      dispatchID,
		WorkerSessionID: dispatchID,
		Outcome:         outcome,
		Output:          result.Dispatch.Result.Output,
		Attempts:        result.Attempts,
	}
	if session := result.Dispatch.Result.ProviderSession; session != nil {
		invoked.Provider = workers.CanonicalProviderSessionProvider(session.Provider)
		if invoked.Provider == "" {
			invoked.Provider = strings.TrimSpace(session.Provider)
		}
		invoked.ProviderSessionRef = strings.TrimSpace(session.ID)
	}
	if outcome != factory.InvokeWorkerOutcomeCompleted {
		invoked.Diagnostic = invokeWorkerDiagnostic(result)
	}
	return invoked
}

func invokeWorkerDiagnostic(result workersessions.InvokeSessionResult) string {
	if result.Session.Result != nil && result.Session.Result.Cause != nil {
		if detail := strings.TrimSpace(result.Session.Result.Cause.Detail); detail != "" {
			return detail
		}
		return string(result.Session.Result.Cause.Kind)
	}
	if detail := strings.TrimSpace(result.Dispatch.Result.Error); detail != "" {
		return detail
	}
	return "Provider execution failed."
}
