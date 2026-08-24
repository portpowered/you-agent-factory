package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// BeginWorkerAttempt prepares a direct-child Worker Session terminal callback.
func (f *factoryImpl) BeginWorkerAttempt(
	ctx context.Context,
	executeRequest workers.ExecuteRequest,
) (func(context.Context, workers.ExecuteResult, error) error, error) {
	if f == nil || f.cfg == nil || f.cfg.workerSessions == nil || f.eventHistory == nil {
		return nil, factory.ErrNotRunning
	}
	if err := executeRequest.Validate(); err != nil {
		return nil, err
	}
	request := workstationDispatchRequestFromExecute(executeRequest)
	dispatchID := strings.TrimSpace(executeRequest.Correlation.DispatchID)
	initialSessionID := runtimeWorkerSessionID(f.cfg, request, executeRequest, false)
	allowRetry := terminalWorkerSessionRequiresRetry(ctx, f.cfg.workerSessions, initialSessionID)
	sessionID := runtimeWorkerSessionID(f.cfg, request, executeRequest, allowRetry)
	prepare := runtimeAttemptPreparation(f.cfg, request, executeRequest, allowRetry)
	if prepare == nil {
		return nil, factory.ErrNotRunning
	}
	if ctx == nil {
		ctx = context.Background()
	}
	terminal, err := prepare(context.WithoutCancel(ctx), &executeRequest)
	if err != nil {
		return nil, err
	}
	// Publish the association only after setup returns a terminal callback;
	// failed setup must not strand a response bridge on a Worker topic.
	recordDispatchWorkerSessionAssociation(
		f.eventHistory,
		f.currentTick(),
		dispatchID,
		sessionID,
		executeRequest.Correlation.RequestID,
		recordings.DispatchWorkerSessionExecutionFacts{
			Model:           executeRequest.Target.Model.Name,
			ReasoningEffort: executeRequest.Target.Model.ReasoningEffort,
		},
		f.cfg.clock.Now(),
	)
	var completeOnce sync.Once
	return func(
		callbackCtx context.Context,
		result workers.ExecuteResult,
		executeErr error,
	) error {
		completeOnce.Do(func() {
			if callbackCtx == nil {
				callbackCtx = context.Background()
			}
			if terminal != nil {
				terminal(callbackCtx, executeRequest, result, executeErr)
			}
		})
		return nil
	}, nil
}

// terminalWorkerSessionRequiresRetry distinguishes a first direct child from
// a resumed child whose prior Worker Session already committed a terminal
// observation. The logical dispatch remains stable, but the reopened attempt
// must use its physical attempt identity as the new Worker Session identity;
// Worker Sessions intentionally does not transition an absorbing terminal
// session back to RESERVED.
func terminalWorkerSessionRequiresRetry(
	ctx context.Context,
	service workersessions.Service,
	sessionID string,
) bool {
	if service == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := service.Get(context.WithoutCancel(ctx), workersessions.GetRequest{ID: sessionID})
	return err == nil && session.Terminal()
}

func workstationDispatchRequestFromExecute(
	request workers.ExecuteRequest,
) workers.WorkstationDispatchRequest {
	target := request.Target.Clone()
	dispatch := work.CloneWorkDispatch(request.Input.Dispatch)
	workstationName := strings.TrimSpace(target.WorkstationName)
	if workstationName == "" {
		workstationName = strings.TrimSpace(dispatch.WorkstationName)
	}
	if dispatch.DispatchID == "" {
		dispatch.DispatchID = strings.TrimSpace(request.Correlation.DispatchID)
	}
	if dispatch.WorkstationName == "" {
		dispatch.WorkstationName = workstationName
	}
	if dispatch.Execution.RequestID == "" {
		dispatch.Execution.RequestID = strings.TrimSpace(request.Correlation.RequestID)
	}
	if dispatch.Execution.TraceID == "" {
		dispatch.Execution.TraceID = strings.TrimSpace(request.Correlation.TraceID)
	}
	modelProvider := firstRuntimeValue(
		target.Model.Provider,
		target.Provider.ID,
		target.Provider.Alias,
	)
	workingDirectory := firstRuntimeValue(
		target.Workspace.WorkingDirectory,
		target.Environment.WorkingDirectory,
	)
	projectID := ""
	if request.Input.WorkflowContext != nil {
		projectID = request.Input.WorkflowContext.ProjectID
	}
	return workers.WorkstationDispatchRequest{
		WorkstationName: workstationName,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:                    dispatch,
			WorkerName:                  target.WorkerName,
			WorkerType:                  target.WorkerType,
			RunnerID:                    target.RunnerID,
			ExecutorProvider:            target.ExecutorProvider,
			ProjectID:                   projectID,
			FactorySessionID:            request.Correlation.FactorySessionID,
			RuntimeID:                   request.Correlation.RuntimeID,
			RecordingID:                 request.Input.RecordingID,
			GenerationID:                request.Correlation.GenerationID,
			WorkflowContext:             request.Input.WorkflowContext.Clone(),
			Capabilities:                target.Capabilities,
			ModelOperation:              request.Input.ModelOperation,
			ModelBindings:               workers.CloneResolvedModelOperationBindings(request.Input.ModelBindings),
			Model:                       target.Model.Name,
			ModelProvider:               modelProvider,
			ReasoningEffort:             target.Model.ReasoningEffort,
			Command:                     target.Command,
			Args:                        append([]string(nil), target.Args...),
			FactoryDirectory:            target.FactoryDirectory,
			OutputFormat:                target.Output.Format,
			StopToken:                   target.Output.StopToken,
			DecisionEnvelope:            target.Output.DecisionEnvelope,
			GoalRoutingDecisionEnvelope: target.Output.GoalRoutingDecisionEnvelope,
			SystemPrompt:                target.Prompt.SystemPrompt,
			UserMessage:                 target.Prompt.UserMessage,
			OutputSchema:                target.Prompt.OutputSchema,
			OutputContract:              target.Output.Contract,
			Timeout:                     target.Timeout,
			EnvVars:                     target.Environment.Vars,
			ProcessEnvironment:          append([]string(nil), target.Environment.ProcessEnvironment...),
			ProcessLifecycleObserver:    request.Input.ProcessLifecycleObserver,
			Worktree:                    target.Workspace.Worktree,
			WorkingDirectory:            workingDirectory,
			WorkingDirectoryAuthored:    target.Environment.WorkingDirectorySet,
			Continuation:                cloneRuntimeContinuation(request.Input.Resume),
			SkipPermissions:             target.Permissions.SkipPermissions,
		},
	}
}

// WorkstationRequestExecutorConfig supplies the immutable Runtime facts needed
// to resolve a direct Worker Session request before handing it to the shared
// Workers Execute boundary. It is deliberately separate from the Runtime's
// active attempt lifecycle: direct Worker Sessions own their own supervision.
type WorkstationRequestExecutorConfig struct {
	Service                    workers.Service
	RuntimeDefinitions         interfaces.RuntimeDefinitionLookup
	InvocationInterpolation    interfaces.InvocationInterpolationService
	InvocationFileReader       interfaces.FileReader
	WorkflowContext            *workers.Context
	FactorySessionID           string
	RuntimeID                  string
	RecordingID                string
	EventHistory               recordings.RuntimeLedger
	NewID                      factory.IDGenerator
	PromptRenderer             runtimePromptRenderer
	TemplateFieldResolver      runtimeTemplateFieldResolver
	PromptSourceReader         interfaces.FileReader
	MockWorkers                *workers.MockWorkersConfig
	ProgressPublisher          workers.ProgressPublisher
	Net                        *state.Net
	ExpectedArtifactFileSystem any
}

type attemptProcessObserver struct {
	lifecycle *attemptLifecycle
	attempt   *activeAttempt
}

func (observer attemptProcessObserver) ProcessStarted(_ platformprocess.ProcessInfo) {}

func (observer attemptProcessObserver) ProcessExited(_ platformprocess.ProcessInfo) {
	observer.lifecycle.reconcileProcessGone(observer.attempt)
}

func processGoneAttemptResult(
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	result.Correlation = request.Correlation
	result.Outcome = workers.ExecutionOutcomeFailed
	result.Output = workers.ProposedOutput{}
	result.StructuredResult = nil
	result.StructuredResultPresent = false
	result.Continuation = nil
	message := workers.ErrWorkstationDispatchProcessGone.Error()
	if strings.EqualFold(strings.TrimSpace(request.Target.RunnerID), "script") && result.Failure != nil {
		if failureMessage := strings.TrimSpace(result.Failure.Message); failureMessage != "" &&
			!strings.EqualFold(failureMessage, "execution canceled") {
			message = failureMessage
		} else if result.Diagnostics != nil && result.Diagnostics.Command != nil {
			if stderr := strings.TrimSpace(result.Diagnostics.Command.Stderr); stderr != "" {
				message = stderr
			}
		}
	}
	result.Failure = &workers.ExecutionFailure{
		Type:    workers.WorkFailureTypeUnknown,
		Family:  workers.WorkFailureFamilyRetryable,
		Message: message,
	}
	return result
}

func processGoneDispatchResult(
	result *workers.WorkResult,
	terminal *workers.WorkstationDispatchTerminalOutcome,
	executeErr error,
) workers.WorkstationDispatchReconciliationReason {
	if result == nil || terminal == nil || !errors.Is(executeErr, workers.ErrWorkstationDispatchProcessGone) {
		return ""
	}
	*terminal = workers.WorkstationDispatchTerminalOutcomeFailed
	result.Outcome = workers.OutcomeFailed
	if processGoneResultError(result.Error) == "" {
		result.Error = workers.ErrWorkstationDispatchProcessGone.Error()
	}
	result.FailureMetadata = &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyRetryable,
		Type:   workers.WorkFailureTypeUnknown,
	}
	result.Diagnostics = &workers.WorkDiagnostics{Metadata: map[string]string{
		workers.ProviderResponseMetadataFailureOperation:      "worker_session_reconciliation",
		workers.ProviderResponseMetadataFailureClassification: "process_gone",
		workers.ProviderResponseMetadataFailureStage:          "process",
	}}
	return workers.WorkstationDispatchReconciliationReasonProcessGone
}

func processGoneResultError(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || strings.Contains(normalized, "cancel") || strings.Contains(normalized, "context canceled") {
		return ""
	}
	return strings.TrimSpace(value)
}

type PromptRenderer = runtimePromptRenderer
type TemplateFieldResolver = runtimeTemplateFieldResolver

// WorkstationExecutionResolver resolves a minimal direct Worker Session
// request into the detached Workers execution contract. Factory Runtime owns
// this definition lookup; Workers remains unaware of Factory configuration.
type WorkstationExecutionResolver interface {
	ResolveExecutionRequest(workers.WorkstationExecutionRequest) (workers.ExecuteRequest, error)
}

// SetReplayEvents binds the immutable canonical history loaded for a legacy
// replay. Runtime execution may regenerate derived events, while observation
// streams continue to expose the persisted event positions.
func (f *factoryImpl) SetReplayEvents(events []interfaces.FactoryEvent) {
	if f == nil || f.cfg == nil {
		return
	}
	f.cfg.replayEvents = cloneAndSortFactoryEvents(events)
}

// canonicalWorkerSessionControlEvents applies the same replay precedence used
// by recorded Worker Session observations to control-target capture. A resumed
// Factory must identify completed children from the replayed association
// history even when the live Runtime ledger is empty or only contains events
// emitted after recovery.
func (f *factoryImpl) canonicalWorkerSessionControlEvents() []interfaces.FactoryEvent {
	if f == nil || f.cfg == nil {
		return nil
	}
	if len(f.cfg.replayEvents) > 0 {
		return cloneAndSortFactoryEvents(f.cfg.replayEvents)
	}
	if f.eventHistory == nil {
		return nil
	}
	return f.eventHistory.CanonicalEvents()
}

// NewWorkstationRequestExecutor creates the Runtime-owned compatibility
// adapter used by top-level Worker Session routes. It resolves minimal legacy
// direct requests from the immutable Factory definition and then invokes the
// process-scoped Workers service with a complete detached ExecuteRequest.
func NewWorkstationRequestExecutor(
	config WorkstationRequestExecutorConfig,
) workers.WorkstationRequestExecutor {
	if config.Service == nil {
		return nil
	}
	return &workstationRequestExecutor{
		service: config.Service,
		cfg: &runtimeConfig{
			executeService:             config.Service,
			runtimeConfig:              config.RuntimeDefinitions,
			invocationInterpolation:    config.InvocationInterpolation,
			invocationFileReader:       config.InvocationFileReader,
			workflowContext:            config.WorkflowContext.Clone(),
			recordingID:                strings.TrimSpace(config.RecordingID),
			runtimeID:                  strings.TrimSpace(config.RuntimeID),
			eventHistory:               config.EventHistory,
			newID:                      config.NewID,
			promptRenderer:             config.PromptRenderer,
			templateFieldResolver:      config.TemplateFieldResolver,
			promptSourceReader:         config.PromptSourceReader,
			mockWorkersConfig:          config.MockWorkers.Clone(),
			progressPublisher:          config.ProgressPublisher,
			net:                        config.Net,
			expectedArtifactFileSystem: expectedArtifactFileSystemFrom(config.ExpectedArtifactFileSystem),
		},
		sessionID: strings.TrimSpace(config.FactorySessionID),
	}
}

type workstationRequestExecutor struct {
	service   executeCapability
	cfg       *runtimeConfig
	sessionID string
}

// ResolveExecutionRequest resolves a direct Worker Session request without
// executing it. The Factory Runtime wrapper uses this narrow capability to
// preserve the public direct Worker Session route after Worker Sessions hands
// execution to the request-scoped Workers service.
func (executor workstationRequestExecutor) ResolveExecutionRequest(
	request workers.WorkstationExecutionRequest,
) (workers.ExecuteRequest, error) {
	if executor.service == nil || executor.cfg == nil {
		return workers.ExecuteRequest{}, workers.ErrExecuteUnavailable
	}
	request = executor.normalizeExecutionRequest(request)
	return executeRequestFromWorkstationRequest(executor.cfg, workers.WorkstationDispatchRequest{
		WorkstationName: firstRuntimeValue(request.Dispatch.WorkstationName, request.WorkstationType),
		Execution:       request,
	})
}

func (executor workstationRequestExecutor) normalizeExecutionRequest(
	request workers.WorkstationExecutionRequest,
) workers.WorkstationExecutionRequest {
	request = workers.CloneWorkstationExecutionRequest(request)
	if strings.TrimSpace(request.FactorySessionID) == "" {
		request.FactorySessionID = executor.sessionID
	}
	request.WorkerType = firstRuntimeValue(request.WorkerType, request.Dispatch.WorkerType)
	if strings.TrimSpace(request.WorkstationType) == "" {
		request.WorkstationType = firstRuntimeValue(request.WorkstationType, request.Dispatch.WorkstationName)
	}
	return runtimeRecordingExecutionRequest(executor.cfg, request)
}

// SetRuntimeLogger binds the opened Runtime's log sink to this compatibility
// adapter. The adapter itself remains detached; only the per-attempt logger
// capability crosses into the process-scoped Workers request.
func (executor *workstationRequestExecutor) SetRuntimeLogger(logger factory.Logger) {
	if executor == nil || executor.cfg == nil {
		return
	}
	executor.cfg.logger = logger
}

func isRuntimePromptRenderError(err error) bool {
	var promptErr *runtimePromptRenderError
	return errors.As(err, &promptErr)
}

type runtimePromptRenderError struct {
	cause error
}

func (err *runtimePromptRenderError) Error() string {
	if err == nil || err.cause == nil {
		return "runtime prompt rendering failed"
	}
	return err.cause.Error()
}

func (err *runtimePromptRenderError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func wrapRuntimePromptRenderError(err error) error {
	if err == nil {
		return nil
	}
	var promptErr *runtimePromptRenderError
	if errors.As(err, &promptErr) {
		return err
	}
	return &runtimePromptRenderError{cause: err}
}

func workstationDispatchResultFromExecute(
	request workers.WorkstationDispatchRequest,
	result workers.ExecuteResult,
	executeErr error,
) (workers.WorkstationDispatchResult, error) {
	dispatch := request.Execution.Dispatch
	workResult := workstationWorkResultFromExecute(request, result)
	terminal, reconciliationReason := classifyWorkstationDispatchResult(result, executeErr, &workResult)
	var proposedOutput *workers.ProposedOutput
	if result.ProposedOutputPresent {
		output := result.Output.Clone()
		proposedOutput = &output
	}
	if terminal == workers.WorkstationDispatchTerminalOutcomeCanceled {
		if proposedOutput != nil {
			clearWorkstationCanceledResult(&workResult, proposedOutput)
		} else {
			clearWorkstationCanceledResult(&workResult, &workers.ProposedOutput{})
		}
	}
	reconciliationReason = processGoneDispatchResult(&workResult, &terminal, executeErr)
	return workers.WorkstationDispatchResult{
		DispatchID:           dispatch.DispatchID,
		WorkstationName:      request.WorkstationName,
		TerminalOutcome:      terminal,
		ReconciliationReason: reconciliationReason,
		Cancellation:         workResult.Cancellation.Clone(),
		Result:               workResult,
		ProposedOutput:       proposedOutput,
	}, executeErr
}

func workstationWorkResultFromExecute(
	request workers.WorkstationDispatchRequest,
	result workers.ExecuteResult,
) workers.WorkResult {
	dispatch := request.Execution.Dispatch
	workResult := workers.WorkResult{
		DispatchID:                  dispatch.DispatchID,
		TransitionID:                dispatch.TransitionID,
		Outcome:                     workers.OutcomeAccepted,
		Cancellation:                result.Cancellation.Clone(),
		Output:                      primaryOutputText(result.Output.Primary),
		StructuredResult:            jsonvalue.Clone(result.StructuredResult),
		StructuredResultPresent:     jsonvalue.Present(result.StructuredResult, result.StructuredResultPresent),
		ArtifactVerification:        result.ArtifactVerification.Clone(),
		Feedback:                    result.Output.Feedback,
		SelectedClassificationLabel: result.Output.Classification,
		Metrics: workers.WorkMetrics{
			Duration:   result.Metrics.Duration,
			Cost:       result.Metrics.Cost,
			RetryCount: result.Metrics.RetryCount,
		},
		Continuation: result.Continuation,
		Diagnostics:  result.Diagnostics.ToWorkDiagnostics(),
	}
	switch result.Outcome {
	case workers.ExecutionOutcomeContinue:
		workResult.Outcome = workers.OutcomeContinue
	case workers.ExecutionOutcomeRejected:
		workResult.Outcome = workers.OutcomeRejected
	case workers.ExecutionOutcomeFailed:
		workResult.Outcome = workers.OutcomeFailed
	case workers.ExecutionOutcomeCanceled:
		workResult.Outcome = workers.OutcomeCanceled
		if workResult.Cancellation == nil {
			workResult.Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}
		}
	default:
		if result.Outcome != workers.ExecutionOutcomeAccepted {
			workResult.Outcome = workers.OutcomeFailed
		}
	}
	if result.Failure == nil {
		return workResult
	}
	workResult.Error = strings.TrimSpace(result.Failure.Message)
	workResult.FailureDetail = workFailureDetail(result.Failure, workResult.Error)
	if shouldPropagateFailureMetadata(request, result.Failure) {
		workResult.FailureMetadata = &workers.WorkFailureMetadata{
			Family: result.Failure.Family,
			Type:   result.Failure.Type,
		}
	}
	return workResult
}

func classifyWorkstationDispatchResult(
	result workers.ExecuteResult,
	executeErr error,
	workResult *workers.WorkResult,
) (workers.WorkstationDispatchTerminalOutcome, workers.WorkstationDispatchReconciliationReason) {
	terminal := workers.WorkstationDispatchTerminalOutcomeCompleted
	if result.Outcome == workers.ExecutionOutcomeFailed {
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
	}
	if result.Outcome != "" && !isKnownExecutionOutcome(result.Outcome) {
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
	}
	if workResult.Cancellation != nil {
		workResult.Outcome = workers.OutcomeCanceled
		terminal = workers.WorkstationDispatchTerminalOutcomeCanceled
	}
	if errors.Is(executeErr, workers.ErrWorkstationDispatchCanceled) {
		workResult.Outcome = workers.OutcomeCanceled
		terminal = workers.WorkstationDispatchTerminalOutcomeCanceled
		if workResult.Cancellation == nil {
			workResult.Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}
		}
	}
	if executeErr != nil && terminal != workers.WorkstationDispatchTerminalOutcomeCanceled {
		workResult.Outcome = workers.OutcomeFailed
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
		if strings.TrimSpace(workResult.Error) == "" {
			workResult.Error = executeErr.Error()
		}
	}
	if terminal == workers.WorkstationDispatchTerminalOutcomeCanceled &&
		strings.TrimSpace(workResult.Error) == "" {
		workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
	}
	return terminal, ""
}

func isKnownExecutionOutcome(outcome workers.ExecutionOutcome) bool {
	switch outcome {
	case workers.ExecutionOutcomeAccepted, workers.ExecutionOutcomeContinue,
		workers.ExecutionOutcomeRejected, workers.ExecutionOutcomeFailed,
		workers.ExecutionOutcomeCanceled:
		return true
	default:
		return false
	}
}

func clearWorkstationCanceledResult(
	workResult *workers.WorkResult,
	proposedOutput *workers.ProposedOutput,
) {
	workResult.Outcome = workers.OutcomeCanceled
	workResult.Output = ""
	workResult.StructuredResult = nil
	workResult.StructuredResultPresent = false
	workResult.Continuation = nil
	workResult.FailureDetail = nil
	workResult.FailureMetadata = nil
	*proposedOutput = workers.ProposedOutput{}
}

func normalizeAttemptCorrelation(
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
) (workers.ExecuteResult, bool) {
	if result.Correlation.DispatchID == "" {
		result.Correlation = request.Correlation
		return result, false
	}
	if result.Correlation.AttemptID == "" {
		result.Correlation.AttemptID = request.Correlation.AttemptID
	}
	if result.Correlation.FactorySessionID == "" {
		result.Correlation.FactorySessionID = request.Correlation.FactorySessionID
	}
	if result.Correlation.RuntimeID == "" {
		result.Correlation.RuntimeID = request.Correlation.RuntimeID
	}
	if result.Correlation.GenerationID == "" {
		result.Correlation.GenerationID = request.Correlation.GenerationID
	}
	if result.Correlation.RequestID == "" {
		result.Correlation.RequestID = request.Correlation.RequestID
	}
	if result.Correlation.TraceID == "" {
		result.Correlation.TraceID = request.Correlation.TraceID
	}
	return result, result.Correlation.DispatchID != request.Correlation.DispatchID ||
		result.Correlation.AttemptID != request.Correlation.AttemptID ||
		correlationValueConflicts(result.Correlation.FactorySessionID, request.Correlation.FactorySessionID) ||
		correlationValueConflicts(result.Correlation.RuntimeID, request.Correlation.RuntimeID) ||
		correlationValueConflicts(result.Correlation.GenerationID, request.Correlation.GenerationID) ||
		correlationValueConflicts(result.Correlation.RequestID, request.Correlation.RequestID) ||
		correlationValueConflicts(result.Correlation.TraceID, request.Correlation.TraceID)
}

func conflictingAttemptResult(request workers.ExecuteRequest) workers.ExecuteResult {
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeUnknown,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: "worker execution returned conflicting correlation",
		},
	}
}

func dispatchCancellationReasonFromCancelRequest(
	reasons ...workers.WorkstationDispatchCancelReason,
) workers.DispatchCancellationReason {
	if len(reasons) > 0 && reasons[0] == workers.WorkstationDispatchCancelReasonSuperseded {
		return workers.DispatchCancellationReasonSuperseded
	}
	return workers.DispatchCancellationReasonCanceled
}

func dispatchCancellationReasonFromContext(ctx context.Context) workers.DispatchCancellationReason {
	if platformprocess.CancellationReasonFromContext(ctx) == platformprocess.CancellationReasonSuperseded {
		return workers.DispatchCancellationReasonSuperseded
	}
	return workers.DispatchCancellationReasonCanceled
}

func platformCancellationReason(reason workers.DispatchCancellationReason) platformprocess.CancellationReason {
	if reason == workers.DispatchCancellationReasonSuperseded {
		return platformprocess.CancellationReasonSuperseded
	}
	return platformprocess.CancellationReasonCanceled
}

func (executor workstationRequestExecutor) Execute(
	ctx context.Context,
	request workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	if executor.service == nil || executor.cfg == nil {
		return workers.WorkResult{}, workers.ErrExecuteUnavailable
	}
	request = executor.normalizeExecutionRequest(request)
	executeRequest, err := executor.ResolveExecutionRequest(request)
	if err != nil {
		result := workers.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      workers.OutcomeFailed,
			Error:        err.Error(),
		}
		if isRuntimePromptRenderError(err) {
			result.Error = "prompt render failed: " + err.Error()
			return result, nil
		}
		return result, err
	}
	result, executeErr := executor.service.Execute(ctx, executeRequest)
	result = normalizeDetachedExecutionResult(executor.cfg, executeRequest, result)
	dispatchResult, dispatchErr := workstationDispatchResultFromExecute(
		workstationDispatchRequestForResult(workers.WorkstationDispatchRequest{
			WorkstationName: firstRuntimeValue(request.Dispatch.WorkstationName, request.WorkstationType),
			Execution:       request,
		}, executeRequest),
		result,
		executeErr,
	)
	return dispatchResult.Result, dispatchErr
}

func runtimeRecordingExecutionRequest(
	cfg *runtimeConfig,
	request workers.WorkstationExecutionRequest,
) workers.WorkstationExecutionRequest {
	if cfg == nil {
		return request
	}
	if strings.TrimSpace(request.RuntimeID) == "" {
		request.RuntimeID = strings.TrimSpace(cfg.runtimeID)
	}
	if strings.TrimSpace(request.RecordingID) == "" {
		request.RecordingID = strings.TrimSpace(cfg.recordingID)
	}
	if strings.TrimSpace(request.GenerationID) == "" && cfg.eventHistory != nil {
		request.GenerationID = strings.TrimSpace(cfg.eventHistory.StreamGenerationID())
	}
	return request
}

func validateRuntimeExecutionSelection(selection runtimeExecutionSelection) error {
	if strings.EqualFold(strings.TrimSpace(selection.runnerID), "script") &&
		strings.TrimSpace(selection.command) == "" {
		return fmt.Errorf("construct script worker: misconfigured: script command is required")
	}
	return nil
}

func finalizeRuntimeWorkspaceSelection(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	fileSystem expectedArtifactFileSystem,
) {
	baseDirectory := strings.TrimSpace(selection.factoryDirectory)
	if cfg != nil {
		if runtimeLookup, ok := cfg.runtimeConfig.(interfaces.RuntimeConfigLookup); ok && runtimeLookup != nil {
			baseDirectory = firstRuntimeValue(
				strings.TrimSpace(runtimeLookup.RuntimeBaseDir()),
				baseDirectory,
			)
		}
	}
	if selection.workingDirectory == "" {
		// Detached execution must carry the same default workspace that the
		// legacy workstation executor derived from RuntimeConfig.
		selection.workingDirectory = baseDirectory
	}
	selection.workingDirectory = resolveRuntimePath(baseDirectory, selection.workingDirectory, fileSystem)
}

func runtimeWorkflowContext(cfg *runtimeConfig, sessionID string, supplied *workers.Context) *workers.Context {
	if context := supplied.Clone(); context != nil {
		if strings.TrimSpace(context.SessionID) == "" {
			context.SessionID = strings.TrimSpace(sessionID)
		}
		return context
	}
	if cfg != nil {
		if context := cfg.workflowContext.Clone(); context != nil {
			if strings.TrimSpace(context.SessionID) == "" {
				context.SessionID = strings.TrimSpace(sessionID)
			}
			return context
		}
	}
	return &workers.Context{SessionID: strings.TrimSpace(sessionID)}
}

const classifierFailureRawOutputLimit = 160

func normalizeScriptClassifierResult(
	target workers.ExecutionTarget,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	// A script classifier is a classifier; either signal selects this policy.
	if !target.Output.Classifier && !target.Output.ScriptClassifier {
		return result
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		return result
	}
	raw := primaryOutputText(result.Output.Primary)
	output := strings.TrimSpace(raw)
	// A script classifier writes its label on the final stdout line. Every other
	// classifier worker is expected to produce the bare label as its response.
	if target.Output.ScriptClassifier && strings.TrimSpace(target.RunnerID) == "script" {
		if output == "" {
			return result
		}
		lines := strings.Split(output, "\n")
		output = strings.TrimSpace(lines[len(lines)-1])
	}
	label, err := normalizeClassifierLabel(output)
	if err != nil {
		message := classifierOutputErrorDetail(raw, err)
		result.Outcome = workers.ExecutionOutcomeFailed
		result.Output.Classification = ""
		result.Failure = &workers.ExecutionFailure{
			Family:  workers.WorkFailureFamilyTerminal,
			Type:    workers.WorkFailureTypeStructuredOutputSchemaViolation,
			Message: message,
			Detail: &workers.FailureDetail{
				Reason:  workers.WorkFailureTypeStructuredOutputSchemaViolation,
				Message: message,
			},
		}
		return result
	}
	result.Output.Primary = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: label,
	}}
	result.Output.Classification = label
	return result
}

// normalizeClassifierLabel mirrors the classifier contract enforced by the
// Workers workstation executor: a classifier must emit a bare routing label,
// never a structured payload.
func normalizeClassifierLabel(output string) (string, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "", errors.New("empty label")
	}
	if json.Valid([]byte(trimmed)) {
		return "", errors.New("expected plain string label")
	}
	return trimmed, nil
}

func classifierOutputErrorDetail(rawOutput string, err error) string {
	detail := "classifier output invalid: " + err.Error()
	trimmed := strings.TrimSpace(rawOutput)
	if trimmed == "" {
		return detail
	}
	if len(trimmed) > classifierFailureRawOutputLimit {
		trimmed = trimmed[:classifierFailureRawOutputLimit] + "..."
	}
	return detail + " (raw output: " + strconv.Quote(trimmed) + ")"
}

func normalizeDetachedExecutionResult(
	cfg *runtimeConfig,
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	result = normalizeScriptClassifierResult(request.Target, result)
	return verifyExpectedArtifactsForDispatch(cfg, request, result)
}

// runtimeRecordingRequest carries process-owned recording, runtime, and
// generation identities into the detached Runtime path. Callers may provide
// explicit identities, while ordinary Factory dispatches inherit the values
// opened for this runtime.
func runtimeRecordingRequest(cfg *runtimeConfig, request workers.WorkstationDispatchRequest) workers.WorkstationDispatchRequest {
	if cfg == nil {
		return request
	}
	if strings.TrimSpace(request.Execution.RecordingID) == "" {
		request.Execution.RecordingID = strings.TrimSpace(cfg.recordingID)
	}
	if strings.TrimSpace(request.Execution.RuntimeID) == "" {
		request.Execution.RuntimeID = strings.TrimSpace(cfg.runtimeID)
	}
	if strings.TrimSpace(request.Execution.GenerationID) == "" && cfg.eventHistory != nil {
		request.Execution.GenerationID = strings.TrimSpace(cfg.eventHistory.StreamGenerationID())
	}
	return request
}

// capturedWorkerSessionControlTargets is the immutable set one committed
// Factory-turn control owns. The Factory Session identity is represented by
// the session-scoped canonical ledger captured below; associations from a
// different Factory Session must therefore be read from a different ledger.
//
// The later fan-out stories retain this value with the committed control. They
// must not call captureAssociatedWorkerSessionTargets again on a retry, or a
// retry could include a child associated after the control linearized.
type capturedWorkerSessionControlTargets struct {
	turnID           string
	workerSessionIDs []string
}

// workerSessionIDsSnapshot returns a detached deterministic target order.
func (c capturedWorkerSessionControlTargets) workerSessionIDsSnapshot() []string {
	return append([]string(nil), c.workerSessionIDs...)
}

// captureAssociatedWorkerSessionTargets selects exactly the Worker Sessions
// canonically associated with turnID at one ledger snapshot. It deliberately
// consumes only the W4 association event rather than tracking dispatches or
// retaining a second association registry. RequestID on that event is the
// Factory invocation's immutable turn correlation supplied by ACP.
func captureAssociatedWorkerSessionTargets(
	ledger recordings.RuntimeLedger,
	turnID string,
) capturedWorkerSessionControlTargets {
	turnID = strings.TrimSpace(turnID)
	if ledger == nil || turnID == "" {
		return capturedWorkerSessionControlTargets{turnID: turnID}
	}
	return selectAssociatedWorkerSessionTargets(ledger.CanonicalEvents(), turnID)
}

func selectAssociatedWorkerSessionTargets(
	events []interfaces.FactoryEvent,
	turnID string,
) capturedWorkerSessionControlTargets {
	turnID = strings.TrimSpace(turnID)
	captured := capturedWorkerSessionControlTargets{turnID: turnID}
	if turnID == "" {
		return captured
	}

	type candidate struct {
		sequence        int
		workerSessionID string
		eventID         string
		index           int
	}
	candidates := make([]candidate, 0)
	for index, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchWorkerSessionAssoc ||
			event.Context.DispatchID == nil || strings.TrimSpace(*event.Context.DispatchID) == "" ||
			event.Context.RequestID == nil || strings.TrimSpace(*event.Context.RequestID) != turnID {
			continue
		}
		var association interfaces.DispatchWorkerSessionAssociationEventPayload
		if event.DecodePayload(&association) != nil {
			continue
		}
		workerSessionID := strings.TrimSpace(association.WorkerSessionID)
		if workerSessionID == "" {
			continue
		}
		candidates = append(candidates, candidate{
			sequence:        event.Context.Sequence,
			workerSessionID: workerSessionID,
			eventID:         event.Id,
			index:           index,
		})
	}

	// Canonical sequence is the normal order. The remaining keys make a
	// partially reconstructed or test fixture event stream deterministic too.
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].sequence != candidates[right].sequence {
			return candidates[left].sequence < candidates[right].sequence
		}
		if candidates[left].eventID != candidates[right].eventID {
			return candidates[left].eventID < candidates[right].eventID
		}
		if candidates[left].workerSessionID != candidates[right].workerSessionID {
			return candidates[left].workerSessionID < candidates[right].workerSessionID
		}
		return candidates[left].index < candidates[right].index
	})

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, duplicate := seen[candidate.workerSessionID]; duplicate {
			continue
		}
		seen[candidate.workerSessionID] = struct{}{}
		captured.workerSessionIDs = append(captured.workerSessionIDs, candidate.workerSessionID)
	}
	return captured
}

func recordDispatchWorkerSessionAssociation(
	ledger recordings.RuntimeLedger,
	tick int,
	dispatchID string,
	workerSessionID string,
	requestID string,
	facts recordings.DispatchWorkerSessionExecutionFacts,
	eventTime time.Time,
) {
	if ledger == nil {
		return
	}
	if recorder, ok := ledger.(recordings.DispatchWorkerSessionAssociationRecorder); ok {
		recorder.RecordDispatchWorkerSessionAssociationWithExecution(
			tick,
			dispatchID,
			workerSessionID,
			requestID,
			facts,
			eventTime,
		)
		return
	}
	ledger.RecordDispatchWorkerSessionAssociation(tick, dispatchID, workerSessionID, requestID, eventTime)
}
