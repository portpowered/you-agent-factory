package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (r *registry) transitionToRunning(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, exists := r.sessions[id]
	if !exists || session.State != workersessions.StateStarting {
		return false
	}
	session.State = workersessions.StateRunning
	r.sessions[id] = session
	return true
}

func (r *registry) logReconciliation(
	id, attemptID string,
	result workers.WorkstationDispatchResult,
	priorState, resultingState workersessions.State,
	startedAt time.Time,
	deadlineAt time.Time,
) {
	elapsedMS := int64(0)
	if !startedAt.IsZero() {
		elapsedMS = r.clock.Now().Sub(startedAt).Milliseconds()
		if elapsedMS < 0 {
			elapsedMS = 0
		}
	}
	deadline := "not_applicable"
	configuredTimeoutMS := int64(0)
	if !deadlineAt.IsZero() {
		deadline = deadlineAt.UTC().Format(time.RFC3339Nano)
		configuredTimeoutMS = deadlineAt.Sub(startedAt).Milliseconds()
	}
	fields := []any{
		"sessionID", id,
		"attemptID", attemptID,
		"dispatchID", result.DispatchID,
		"reason", string(result.ReconciliationReason),
		"prior_state", string(priorState),
		"resulting_state", string(resultingState),
		"result", string(result.TerminalOutcome),
		"elapsed_ms", elapsedMS,
		"deadline", deadline,
	}
	if !deadlineAt.IsZero() {
		fields = append(fields, "configured_timeout_ms", configuredTimeoutMS)
	}
	r.logger.Info("worker session reconciliation", fields...)
}

// publishExecution hands one resolved request to the directly injected
// request-scoped Workers service and returns at the exact admission barrier.
// Workers Execute is synchronous and has no queue or lifecycle API: the
// supervision owns the attempt context, admits immediately before Execute,
// and controls the attempt by canceling that context.
func (r *registry) publishExecution(
	ctx context.Context,
	sessionID string,
	request workers.WorkstationDispatchRequest,
	supervision *supervision,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// The invocation driver already detaches caller cancellation before it
	// reaches this handoff. Keep that detached context here so a process
	// lifecycle shutdown only joins server-owned attempts through Worker
	// Sessions' explicit supervision controls; a direct InvokeSession must not
	// be canceled merely because an unrelated runtime lifecycle closes.
	attemptContext, cancel := context.WithCancel(ctx)
	supervision.installCancel(cancel)
	admitted := make(chan struct{})
	finished := make(chan struct{})
	dispatchDone := make(chan error, 1)
	execution := r.execution
	go func() {
		defer close(finished)
		result, dispatchErr := executeWithService(attemptContext, execution, request, supervision, func() {
			r.acceptSupervision(sessionID, supervision)
			close(admitted)
		})
		if errors.Is(dispatchErr, workers.ErrWorkstationDispatchCanceled) && supervision.pendingTerminalControlBeforeAdmission() != "" {
			// Let a terminal control that claimed this exact unadmitted
			// supervision commit first. Publishing the canceled handoff must not
			// win the terminal race and turn an applied Cancel into an unknown
			// dispatch or a generic publication failure.
			r.finishSupervisionPublication(supervision)
			<-supervision.done
		}
		r.completeSupervision(sessionID, supervision, result, dispatchErr)
		dispatchDone <- dispatchErr
	}()
	select {
	case <-admitted:
	case <-finished:
		dispatchErr := <-dispatchDone
		select {
		case <-admitted:
		default:
			if dispatchErr != nil {
				return dispatchErr
			}
			return workersessions.ErrStartAdmissionFailed
		}
	}
	return nil
}

func executeWithService(
	ctx context.Context,
	execution workers.Service,
	request workers.WorkstationDispatchRequest,
	supervision *supervision,
	admit func(),
) (result workers.WorkstationDispatchResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := &workers.WorkerExecutorPanicError{Cause: recovered}
			result, err = failedDispatchResult(request, panicErr)
		}
	}()

	if execution == nil {
		return failedDispatchResult(request, workers.ErrExecuteUnavailable)
	}
	executeRequest, conversionErr := executeRequestFromSessionDispatch(request)
	if conversionErr != nil {
		return failedDispatchResult(request, conversionErr)
	}
	if !supervision.admissionAllowed() {
		return canceledDispatchResult(request)
	}
	admit()
	if !supervision.isAccepted() {
		return canceledDispatchResult(request)
	}
	if executeRequest.Input.ProcessLifecycleObserver == nil {
		executeRequest.Input.ProcessLifecycleObserver = processLifecycleObserver{supervision: supervision}
	}
	executeResult, executeErr := execution.Execute(ctx, executeRequest)
	if supervision.processGoneObserved() {
		executeResult = processGoneExecuteResult(executeRequest, executeResult)
		executeErr = workers.ErrWorkstationDispatchProcessGone
	} else if ctx.Err() == context.Canceled && executeErr != nil {
		executeResult.Outcome = workers.ExecutionOutcomeCanceled
		executeResult.Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}
		executeErr = errors.Join(workers.ErrWorkstationDispatchCanceled, executeErr)
	}
	return dispatchResultFromExecute(request, executeResult, executeErr)
}

type processLifecycleObserver struct {
	supervision *supervision
}

func (s *supervision) cancellationRequested() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preAdmissionAction != "" || s.requestedAction != "" ||
		s.controlAction != "" || s.controlActive
}

func (observer processLifecycleObserver) ProcessStarted(platformprocess.ProcessInfo) {}

func (observer processLifecycleObserver) ProcessExited(platformprocess.ProcessInfo) {
	if observer.supervision == nil {
		return
	}
	if observer.supervision.cancellationRequested() {
		return
	}
	observer.supervision.markProcessGone()
	_, _ = observer.supervision.cancelExecution()
}

func processGoneExecuteResult(
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	result.Correlation = request.Correlation
	result.Outcome = workers.ExecutionOutcomeFailed
	result.Output = workers.ProposedOutput{}
	result.StructuredResult = nil
	result.StructuredResultPresent = false
	result.Continuation = nil
	result.Failure = &workers.ExecutionFailure{
		Type:    workers.WorkFailureTypeUnknown,
		Family:  workers.WorkFailureFamilyRetryable,
		Message: workers.ErrWorkstationDispatchProcessGone.Error(),
	}
	return result
}

func executeRequestFromSessionDispatch(
	request workers.WorkstationDispatchRequest,
) (workers.ExecuteRequest, error) {
	execution := workers.CloneWorkstationExecutionRequest(request.Execution)
	dispatch := execution.Dispatch
	dispatchID := strings.TrimSpace(dispatch.DispatchID)
	if dispatchID == "" {
		return workers.ExecuteRequest{}, fmt.Errorf("%w: dispatch ID is required", workers.ErrInvalidExecuteRequest)
	}
	factorySessionID := firstSessionValue(execution.FactorySessionID, execution.ProjectID, dispatch.Execution.RequestID, "worker-session")
	runtimeID := firstSessionValue(execution.RuntimeID, execution.RecordingID, factorySessionID)
	generationID := firstSessionValue(execution.GenerationID, runtimeID)
	requestID := firstSessionValue(dispatch.Execution.RequestID, execution.ProjectID, dispatchID)
	workstationName := firstSessionValue(request.WorkstationName, execution.WorkstationType, dispatch.WorkstationName)
	workerType := firstSessionValue(execution.WorkerType, execution.WorkerName, dispatch.WorkerType)
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: factorySessionID,
			RuntimeID:        runtimeID,
			GenerationID:     generationID,
			DispatchID:       dispatchID,
			AttemptID:        dispatchID,
			RequestID:        requestID,
			TraceID:          strings.TrimSpace(dispatch.Execution.TraceID),
		},
		Target: workers.ExecutionTarget{
			WorkerName:       strings.TrimSpace(execution.WorkerName),
			WorkerType:       workerType,
			WorkstationName:  workstationName,
			RunnerID:         strings.TrimSpace(execution.RunnerID),
			ExecutorProvider: strings.TrimSpace(execution.ExecutorProvider),
			Capabilities:     cloneSessionCapabilities(execution.Capabilities),
			Command:          execution.Command,
			Args:             append([]string(nil), execution.Args...),
			FactoryDirectory: execution.FactoryDirectory,
			Provider: workers.ProviderReference{
				ID:    strings.TrimSpace(execution.ModelProvider),
				Alias: strings.TrimSpace(execution.ModelProvider),
			},
			Model: workers.ModelReference{
				Name:            strings.TrimSpace(execution.Model),
				Provider:        strings.TrimSpace(execution.ModelProvider),
				ReasoningEffort: strings.TrimSpace(execution.ReasoningEffort),
			},
			Prompt: workers.PromptPolicy{
				SystemPrompt: execution.SystemPrompt,
				UserMessage:  execution.UserMessage,
				OutputSchema: execution.OutputSchema,
			},
			Output: workers.OutputPolicy{
				Contract:                    execution.OutputContract,
				Format:                      execution.OutputFormat,
				StopToken:                   execution.StopToken,
				DecisionEnvelope:            execution.DecisionEnvelope,
				GoalRoutingDecisionEnvelope: execution.GoalRoutingDecisionEnvelope,
			},
			Environment: workers.EnvironmentPolicy{
				Vars:                   cloneSessionStringMap(execution.EnvVars),
				ProcessEnvironment:     append([]string(nil), execution.ProcessEnvironment...),
				WorkingDirectory:       execution.WorkingDirectory,
				WorkingDirectorySet:    execution.WorkingDirectoryAuthored,
				SkipProcessInheritance: len(execution.ProcessEnvironment) > 0,
			},
			Workspace: workers.WorkspacePolicy{
				Worktree:         execution.Worktree,
				WorkingDirectory: execution.WorkingDirectory,
			},
			Permissions: workers.PermissionPolicy{SkipPermissions: execution.SkipPermissions},
			Timeout:     execution.Timeout,
		},
		Input: workers.ExecutionInput{
			Dispatch:                 dispatch,
			RecordingID:              execution.RecordingID,
			ModelBindings:            workers.CloneResolvedModelOperationBindings(execution.ModelBindings),
			ModelOperation:           execution.ModelOperation,
			Resume:                   cloneSessionContinuation(execution.Continuation),
			WorkflowContext:          execution.WorkflowContext.Clone(),
			ProcessLifecycleObserver: execution.ProcessLifecycleObserver,
		},
	}, nil
}

func dispatchResultFromExecute(
	request workers.WorkstationDispatchRequest,
	result workers.ExecuteResult,
	executeErr error,
) (workers.WorkstationDispatchResult, error) {
	workResult := workResultFromExecute(request, result)
	terminal, reconciliationReason, dispatchErr := classifyExecuteResult(result, executeErr, &workResult)
	if terminal == workers.WorkstationDispatchTerminalOutcomeCanceled {
		clearCanceledDispatchResult(&workResult)
	}
	var output *workers.ProposedOutput
	if result.ProposedOutputPresent {
		proposed := result.Output.Clone()
		output = &proposed
	}
	return workers.WorkstationDispatchResult{
		DispatchID:           request.Execution.Dispatch.DispatchID,
		WorkstationName:      request.WorkstationName,
		TerminalOutcome:      terminal,
		ReconciliationReason: reconciliationReason,
		Cancellation:         workResult.Cancellation.Clone(),
		Result:               workResult,
		ProposedOutput:       output,
	}, dispatchErr
}

func workResultFromExecute(
	request workers.WorkstationDispatchRequest,
	result workers.ExecuteResult,
) workers.WorkResult {
	workResult := workers.WorkResult{
		DispatchID:                  request.Execution.Dispatch.DispatchID,
		TransitionID:                request.Execution.Dispatch.TransitionID,
		Outcome:                     workers.OutcomeAccepted,
		Cancellation:                result.Cancellation.Clone(),
		Output:                      primaryText(result.Output.Primary),
		StructuredResult:            result.StructuredResult,
		StructuredResultPresent:     result.StructuredResultPresent,
		Feedback:                    result.Output.Feedback,
		SelectedClassificationLabel: result.Output.Classification,
		ArtifactVerification:        result.ArtifactVerification.Clone(),
		Continuation:                cloneSessionContinuation(result.Continuation),
		Metrics: workers.WorkMetrics{
			Duration:   result.Metrics.Duration,
			Cost:       result.Metrics.Cost,
			RetryCount: result.Metrics.RetryCount,
		},
		Diagnostics: result.Diagnostics.ToWorkDiagnostics(),
	}
	if result.Failure == nil {
		return workResult
	}
	workResult.Error = result.Failure.Message
	workResult.FailureMetadata = &workers.WorkFailureMetadata{
		Family: result.Failure.Family,
		Type:   result.Failure.Type,
	}
	workResult.ProviderFailureKind = result.Failure.ProviderFailureKind
	workResult.ProviderContinuationFailureKind = result.Failure.ProviderContinuationFailureKind
	workResult.ProviderContinuationOutcome = result.Failure.ProviderContinuationOutcome
	return workResult
}

func classifyExecuteResult(
	result workers.ExecuteResult,
	executeErr error,
	workResult *workers.WorkResult,
) (
	workers.WorkstationDispatchTerminalOutcome,
	workers.WorkstationDispatchReconciliationReason,
	error,
) {
	terminal, reconciliationReason, dispatchErr := classifyExecuteError(executeErr, result, workResult)
	if workResult.Cancellation != nil {
		workResult.Outcome = workers.OutcomeCanceled
		terminal = workers.WorkstationDispatchTerminalOutcomeCanceled
		if workResult.Error == "" {
			workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
		}
	}
	terminal = applyExecuteOutcome(result, workResult, terminal)
	return terminal, reconciliationReason, dispatchErr
}

func classifyExecuteError(
	executeErr error,
	result workers.ExecuteResult,
	workResult *workers.WorkResult,
) (
	workers.WorkstationDispatchTerminalOutcome,
	workers.WorkstationDispatchReconciliationReason,
	error,
) {
	terminal := workers.WorkstationDispatchTerminalOutcomeCompleted
	reconciliationReason := workers.WorkstationDispatchReconciliationReason("")
	if executeErr == nil {
		return terminal, reconciliationReason, nil
	}
	if errors.Is(executeErr, workers.ErrWorkstationDispatchCanceled) ||
		result.Outcome == workers.ExecutionOutcomeCanceled {
		terminal = workers.WorkstationDispatchTerminalOutcomeCanceled
		workResult.Outcome = workers.OutcomeCanceled
		if workResult.Cancellation == nil {
			workResult.Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}
		}
		workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
		return terminal, reconciliationReason, errors.Join(workers.ErrWorkstationDispatchCanceled, executeErr)
	}
	terminal = workers.WorkstationDispatchTerminalOutcomeFailed
	if workResult.Error == "" {
		workResult.Error = executeErr.Error()
	}
	if errors.Is(executeErr, workers.ErrWorkstationDispatchProcessGone) {
		reconciliationReason = workers.WorkstationDispatchReconciliationReasonProcessGone
	} else {
		workResult.Outcome = workers.OutcomeFailed
	}
	return terminal, reconciliationReason, executeErr
}

func applyExecuteOutcome(
	result workers.ExecuteResult,
	workResult *workers.WorkResult,
	terminal workers.WorkstationDispatchTerminalOutcome,
) workers.WorkstationDispatchTerminalOutcome {
	switch result.Outcome {
	case workers.ExecutionOutcomeContinue:
		workResult.Outcome = workers.OutcomeContinue
	case workers.ExecutionOutcomeRejected:
		workResult.Outcome = workers.OutcomeRejected
	case workers.ExecutionOutcomeFailed:
		workResult.Outcome = workers.OutcomeFailed
		if terminal == workers.WorkstationDispatchTerminalOutcomeCompleted {
			terminal = workers.WorkstationDispatchTerminalOutcomeFailed
		}
	case workers.ExecutionOutcomeCanceled:
		workResult.Outcome = workers.OutcomeCanceled
		terminal = workers.WorkstationDispatchTerminalOutcomeCanceled
		if workResult.Cancellation == nil {
			workResult.Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}
		}
		if workResult.Error == "" {
			workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
		}
	}
	return terminal
}

func clearCanceledDispatchResult(result *workers.WorkResult) {
	result.Outcome = workers.OutcomeCanceled
	result.Output = ""
	result.StructuredResult = nil
	result.StructuredResultPresent = false
	result.Continuation = nil
	result.FailureDetail = nil
	result.FailureMetadata = nil
}

func failedDispatchResult(request workers.WorkstationDispatchRequest, cause error) (workers.WorkstationDispatchResult, error) {
	if cause == nil {
		cause = workers.ErrExecuteUnavailable
	}
	return workers.WorkstationDispatchResult{
		DispatchID:      request.Execution.Dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID:   request.Execution.Dispatch.DispatchID,
			TransitionID: request.Execution.Dispatch.TransitionID,
			Outcome:      workers.OutcomeFailed,
			Error:        cause.Error(),
		},
	}, cause
}

func canceledDispatchResult(request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
	return workers.WorkstationDispatchResult{
		DispatchID:      request.Execution.Dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCanceled,
		Result: workers.WorkResult{
			DispatchID:   request.Execution.Dispatch.DispatchID,
			TransitionID: request.Execution.Dispatch.TransitionID,
			Outcome:      workers.OutcomeCanceled,
			Cancellation: &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled},
			Error:        workers.ErrWorkstationDispatchCanceled.Error(),
		},
	}, workers.ErrWorkstationDispatchCanceled
}

func primaryText(parts []work.WorkContentPart) string {
	for _, part := range parts {
		if part.Type.Normalized() == work.WorkContentPartTypeText {
			return part.Text
		}
	}
	return ""
}

func firstSessionValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneSessionCapabilities(value *workers.Capabilities) *workers.Capabilities {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneSessionStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	clone := make(map[string]string, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}

func cloneSessionContinuation(value *workers.ProviderContinuationRef) *workers.ProviderContinuationRef {
	if value == nil {
		return nil
	}
	clone := value.Clone()
	return &clone
}

func (r *registry) completeSupervision(id string, supervision *supervision, result workers.WorkstationDispatchResult, dispatchErr error) {
	snapshot := supervision.completionSnapshot()
	if snapshot.deadlineExceeded {
		result = timeoutDispatchResult(result)
		dispatchErr = workers.ErrWorkstationDispatchTimeout
	}
	priorState := r.sessionState(id)
	result = r.reconcileContinuationResult(id, snapshot, result)
	supervision.recordResult(result, dispatchErr)
	if !snapshot.continuing {
		r.associateProviderSessionFromResult(id, snapshot.dispatchID, result)
	}
	if r.completePausedSupervision(id, supervision, snapshot, result, dispatchErr) {
		return
	}
	if r.completeRetryableSupervision(id, supervision, snapshot, result, dispatchErr, priorState) {
		return
	}
	r.completeTerminalSupervision(id, supervision, snapshot, result, dispatchErr, priorState)
}

type completionSnapshot struct {
	action           workersessions.ControlAction
	continuing       bool
	dispatchID       string
	serverOwned      bool
	startedAt        time.Time
	deadlineAt       time.Time
	deadlineExceeded bool
}

func (s *supervision) completionSnapshot() completionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return completionSnapshot{
		action:           s.requestedAction,
		continuing:       s.continuing,
		dispatchID:       s.dispatchID,
		serverOwned:      s.serverOwned,
		startedAt:        s.startedAt,
		deadlineAt:       s.deadlineAt,
		deadlineExceeded: s.deadlineExceeded,
	}
}

func (s *supervision) recordResult(result workers.WorkstationDispatchResult, dispatchErr error) {
	s.mu.Lock()
	s.result = result
	s.err = dispatchErr
	s.mu.Unlock()
}

func (s *supervision) clearRequestedAction() {
	s.mu.Lock()
	s.requestedAction = ""
	s.mu.Unlock()
}

func (r *registry) reconcileContinuationResult(id string, snapshot completionSnapshot, result workers.WorkstationDispatchResult) workers.WorkstationDispatchResult {
	if !snapshot.continuing || r.continuationResultMatchesAssociation(id, result) {
		return result
	}
	result = invalidContinuationResult(result)
	r.logger.Info("worker session continuation result rejected", "sessionID", id, "attemptID", snapshot.dispatchID, "outcome", "reference_mismatch")
	return result
}

func (r *registry) completePausedSupervision(
	id string,
	supervision *supervision,
	snapshot completionSnapshot,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) bool {
	if snapshot.action != workersessions.ControlActionPause || !dispatchCanceled(result, dispatchErr) || !r.transitionToPaused(id) {
		return false
	}
	r.finishControlHistory(controlReservationFor(supervision), workersessions.ControlOutcomeApplied, snapshot.dispatchID, workersessions.StatePaused)
	supervision.clearRequestedAction()
	r.logger.Info("worker session control", "sessionID", id, "attemptID", snapshot.dispatchID, "action", string(snapshot.action), "outcome", string(workersessions.ControlOutcomeApplied))
	supervision.signalPaused()
	return true
}

// A retryable failure with budget left is deliberately not terminal: the
// session stays open, its publication window stays open, and the driver
// publishes the next attempt. Releasing only the attempt -- never done --
// keeps controls waiting for the session's real terminal outcome.
func (r *registry) completeRetryableSupervision(
	id string,
	supervision *supervision,
	snapshot completionSnapshot,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
	priorState workersessions.State,
) bool {
	if !r.claimRetryAttempt(supervision, snapshot.action, result, dispatchErr) {
		return false
	}
	r.logReconciliationIfNeeded(id, snapshot.dispatchID, result, priorState, workersessions.StateRunning, snapshot.startedAt, snapshot.deadlineAt)
	r.logger.Info("worker session attempt", "sessionID", id, "attemptID", snapshot.dispatchID, "outcome", "retryable_failure")
	supervision.finishAttempt()
	return true
}

func (r *registry) completeTerminalSupervision(
	id string,
	supervision *supervision,
	snapshot completionSnapshot,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
	priorState workersessions.State,
) {
	state, terminal := dispatchedTerminal(snapshot.action, result, dispatchErr)
	controlOutcome := controlOutcomeFromDispatch(snapshot.action, result, dispatchErr)
	if snapshot.action == workersessions.ControlActionPause && state != workersessions.StatePaused {
		controlOutcome = workersessions.ControlOutcomeNoop
	}
	r.finishSupervisionControl(supervision, snapshot, controlOutcome, state)
	final, committed := r.commitTerminal(id, state, terminal)
	if committed {
		r.publishCompletedSupervision(id, snapshot, result, priorState, state, final)
	}
	supervision.finishAttempt()
	supervision.signalDone()
}

func (r *registry) finishSupervisionControl(
	supervision *supervision,
	snapshot completionSnapshot,
	controlOutcome workersessions.ControlOutcome,
	state workersessions.State,
) {
	reservation := controlReservationFor(supervision)
	if reservation == nil {
		return
	}
	if reservation.action == workersessions.ControlActionResume && snapshot.action == "" {
		controlOutcome = workersessions.ControlOutcomeFailed
	}
	r.finishControlHistory(reservation, controlOutcome, snapshot.dispatchID, state)
}

func (r *registry) publishCompletedSupervision(
	id string,
	snapshot completionSnapshot,
	result workers.WorkstationDispatchResult,
	priorState, state workersessions.State,
	final workersessions.Session,
) {
	r.logTerminal(id, snapshot.dispatchID, final)
	r.logReconciliationIfNeeded(id, snapshot.dispatchID, result, priorState, state, snapshot.startedAt, snapshot.deadlineAt)
	terminalContext := context.Background()
	if snapshot.serverOwned {
		terminalContext = r.serverOwnedContext()
	}
	r.publishTerminalRecordOrLog(terminalContext, id, snapshot.dispatchID, state, *final.Result)
}

// continuationResultMatchesAssociation keeps every Workers execution path
// accountable to the exact reference that admitted the continuation. Agent
// runners reject a provider mismatch before progress is published, while this
// final check prevents another Workers adapter from committing a plausible
// success under a stale or foreign retained association.
func (r *registry) continuationResultMatchesAssociation(id string, result workers.WorkstationDispatchResult) bool {
	if result.Result.Outcome != workers.OutcomeAccepted && result.Result.Outcome != workers.OutcomeContinue {
		return true
	}
	continuation := result.Result.Continuation
	if continuation == nil {
		return false
	}
	reference, err := continuation.ToSessionRef()
	if err != nil {
		return false
	}
	r.mu.RLock()
	session, exists := r.sessions[id]
	r.mu.RUnlock()
	return exists && session.ProviderSessionAssociation != nil &&
		session.ProviderSessionAssociation.Reference == reference
}

func invalidContinuationResult(result workers.WorkstationDispatchResult) workers.WorkstationDispatchResult {
	result.Result.Outcome = workers.OutcomeFailed
	result.Result.FailureMetadata = &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyTerminal,
		Type:   workers.WorkFailureTypePermanentBadRequest,
	}
	result.Result.ProviderContinuationFailureKind = providers.ContinuationFailureKindInvalid
	result.Result.ProviderContinuationOutcome = ""
	return result
}

func dispatchCanceled(result workers.WorkstationDispatchResult, dispatchErr error) bool {
	return result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCanceled ||
		errors.Is(dispatchErr, workers.ErrWorkstationDispatchCanceled)
}
