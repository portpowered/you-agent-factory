package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// startThroughStatelessWorkers is the Runtime-owned WSE-B dispatch edge. The
// legacy workstation request is used only as an internal compatibility input
// while Runtime projects it into the detached Execute contract. No executor,
// runner, pool, or binding is passed to Workers.
func startThroughStatelessWorkers(
	ctx context.Context,
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	if cfg == nil || cfg.attempts == nil {
		return ErrAttemptLifecycleUnavailable
	}
	request = runtimeRecordingRequest(cfg, request)
	executeRequest, err := executeRequestFromWorkstationRequest(cfg, request)
	if err != nil {
		return err
	}
	// Petri dispatches are single-attempt at this boundary. Keep their
	// physical workstation identity equal to the logical dispatch identity so
	// model request/response events remain joined to the canonical dispatch.
	// InvokeWorker assigns distinct AttemptIDs itself when it explicitly
	// retries a detached execution.
	executeRequest.Correlation.AttemptID = executeRequest.Correlation.DispatchID
	// Worker Session association does not own admission, cancellation,
	// execution, or terminal authority -- the attempt lifecycle below remains
	// the only execution owner -- but the association Factory Event is part of
	// the canonical dispatch event order and must be recorded on every
	// dispatch, not only when a Worker Sessions dependency happens to be wired.
	sessionID := executeRequest.Correlation.DispatchID
	// Replay must reuse the originally recorded Worker Session ID so live and
	// replay correlation stay stable across resume.
	if resolver, ok := cfg.completionDeliveryPlanner.(factory.ReplayWorkerSessionIDResolver); ok {
		if recordedSessionID, found := resolver.WorkerSessionIDForDispatch(request.Execution.Dispatch); found {
			sessionID = recordedSessionID
		}
	}
	if cfg.eventHistory != nil {
		cfg.eventHistory.RecordDispatchWorkerSessionAssociation(
			request.Execution.Dispatch.Execution.DispatchCreatedTick,
			request.Execution.Dispatch.DispatchID,
			sessionID,
			request.Execution.Dispatch.Execution.RequestID,
			cfg.clock.Now(),
		)
	}
	if isLogicalWorkstationDispatch(cfg, request) {
		if accept != nil {
			accept(
				context.Background(),
				request,
				workers.WorkstationDispatchResult{
					DispatchID:      request.Execution.Dispatch.DispatchID,
					WorkstationName: request.WorkstationName,
					TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
					Result: workers.WorkResult{
						DispatchID:   request.Execution.Dispatch.DispatchID,
						TransitionID: request.Execution.Dispatch.TransitionID,
						Outcome:      workers.OutcomeAccepted,
					},
				},
				nil,
			)
		}
		return nil
	}
	startErr := startStatelessAttemptWithRequest(
		ctx, cfg, request, executeRequest,
		!cfg.inlineDispatch && cfg.completionDeliveryPlanner == nil, accept,
	)
	if startErr != nil && errors.Is(startErr, workersessions.ErrStartOpeningPublication) {
		result, dispatchErr := failedWorkstationDispatchResult(request, startErr)
		if accept != nil {
			accept(context.Background(), request, result, dispatchErr)
		}
		return nil
	}
	return startErr
}

func isLogicalWorkstationDispatch(
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
) bool {
	lookup, ok := runtimeDefinitionLookup(cfg)
	if !ok {
		return false
	}
	name := firstRuntimeValue(request.WorkstationName, request.Execution.Dispatch.WorkstationName)
	workstation, found := lookup.Workstation(name)
	return found && workstation != nil && workstation.Type == interfaces.WorkstationTypeLogical
}

func failedWorkstationDispatchResult(
	request workers.WorkstationDispatchRequest,
	dispatchErr error,
) (workers.WorkstationDispatchResult, error) {
	dispatchID := request.Execution.Dispatch.DispatchID
	transitionID := request.Execution.Dispatch.TransitionID
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID:   dispatchID,
			TransitionID: transitionID,
			Outcome:      workers.OutcomeFailed,
			Error:        dispatchErr.Error(),
		},
	}, dispatchErr
}

func startStatelessAttempt(
	ctx context.Context,
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	async bool,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	if cfg == nil || cfg.attempts == nil {
		return ErrAttemptLifecycleUnavailable
	}
	request = runtimeRecordingRequest(cfg, request)
	executeRequest, err := executeRequestFromWorkstationRequest(cfg, request)
	if err != nil {
		return err
	}
	return startStatelessAttemptWithRequest(ctx, cfg, request, executeRequest, async, accept)
}

func startStatelessAttemptWithRequest(
	ctx context.Context,
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	executeRequest workers.ExecuteRequest,
	async bool,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	return startStatelessAttemptWithRequestMode(
		ctx, cfg, request, executeRequest, async, accept, false,
	)
}

func startStatelessAttemptWithRequestRetry(
	ctx context.Context,
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	executeRequest workers.ExecuteRequest,
	async bool,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	return startStatelessAttemptWithRequestMode(
		ctx, cfg, request, executeRequest, async, accept, true,
	)
}

func startStatelessAttemptWithRequestMode(
	ctx context.Context,
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	executeRequest workers.ExecuteRequest,
	async bool,
	accept workers.WorkstationDispatchAcceptFunc,
	allowRetry bool,
) error {
	request = runtimeRecordingRequest(cfg, request)
	if strings.TrimSpace(executeRequest.Correlation.RuntimeID) == "" {
		executeRequest.Correlation.RuntimeID = strings.TrimSpace(request.Execution.RecordingID)
	}
	start := cfg.attempts.start
	if allowRetry {
		start = cfg.attempts.startRetry
	}
	prepare := runtimeAttemptPreparation(cfg, request, executeRequest, allowRetry)
	prepare = prepareDetachedModelRecording(cfg, prepare)
	if prepare == nil {
		return start(
			ctx,
			executeRequest,
			async,
			func(callbackCtx context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, executeErr error) {
				result = normalizeDetachedExecutionResult(cfg, executeRequest, result)
				dispatchResult, dispatchErr := workstationDispatchResultFromExecute(
					request,
					result,
					executeErr,
				)
				if accept != nil {
					accept(callbackCtx, request, dispatchResult, dispatchErr)
				}
			},
		)
	}
	return cfg.attempts.startWithPreparation(
		ctx,
		executeRequest,
		async,
		func(callbackCtx context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, executeErr error) {
			result = normalizeDetachedExecutionResult(cfg, executeRequest, result)
			dispatchResult, dispatchErr := workstationDispatchResultFromExecute(
				request,
				result,
				executeErr,
			)
			if accept != nil {
				accept(callbackCtx, request, dispatchResult, dispatchErr)
			}
		},
		allowRetry,
		prepare,
	)
}

func normalizeScriptClassifierResult(
	target workers.ExecutionTarget,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	if !target.Output.ScriptClassifier || strings.TrimSpace(target.RunnerID) != "script" ||
		result.Outcome != workers.ExecutionOutcomeAccepted {
		return result
	}
	output := strings.TrimSpace(primaryOutputText(result.Output.Primary))
	if output == "" {
		return result
	}
	lines := strings.Split(output, "\n")
	label := strings.TrimSpace(lines[len(lines)-1])
	result.Output.Primary = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: label,
	}}
	result.Output.Classification = label
	return result
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

func runtimeAttemptPreparation(
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	executeRequest workers.ExecuteRequest,
	allowRetry bool,
) attemptPreparation {
	if cfg == nil || cfg.workerSessions == nil {
		return nil
	}
	recorder, ok := cfg.workerSessions.(interface {
		BeginRuntimeAttempt(context.Context, workersessions.RuntimeAttemptRequest) (workersessions.RuntimeAttempt, error)
	})
	if !ok || recorder == nil {
		return nil
	}
	return func(ctx context.Context, _ workers.ExecuteRequest) (attemptTerminalFunc, error) {
		sessionID := runtimeWorkerSessionID(cfg, request, executeRequest, allowRetry)
		attempt, err := recorder.BeginRuntimeAttempt(
			context.WithoutCancel(ctx),
			workersessions.RuntimeAttemptRequest{
				ID:        sessionID,
				AttemptID: executeRequest.Correlation.AttemptID,
				Execution: request,
			},
		)
		if err != nil {
			return nil, err
		}
		return func(callbackCtx context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, executeErr error) {
			result = normalizeDetachedExecutionResult(cfg, executeRequest, result)
			dispatchResult, dispatchErr := workstationDispatchResultFromExecute(
				request,
				result,
				executeErr,
			)
			_ = attempt.Complete(callbackCtx, dispatchResult, dispatchErr)
		}, nil
	}
}

func runtimeWorkerSessionID(
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	executeRequest workers.ExecuteRequest,
	allowRetry bool,
) string {
	if allowRetry && strings.TrimSpace(executeRequest.Correlation.AttemptID) != "" {
		return strings.TrimSpace(executeRequest.Correlation.AttemptID)
	}
	sessionID := strings.TrimSpace(executeRequest.Correlation.DispatchID)
	if resolver, ok := cfg.completionDeliveryPlanner.(factory.ReplayWorkerSessionIDResolver); ok {
		if recordedSessionID, found := resolver.WorkerSessionIDForDispatch(request.Execution.Dispatch); found {
			sessionID = recordedSessionID
		}
	}
	return sessionID
}

func cancelStatelessAttempt(
	ctx context.Context,
	cfg *runtimeConfig,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	if cfg == nil || cfg.attempts == nil {
		return workers.WorkstationDispatchCancelResult{}, ErrAttemptLifecycleUnavailable
	}
	outcome, err := cfg.attempts.cancel(ctx, request.DispatchID)
	if err != nil {
		return workers.WorkstationDispatchCancelResult{}, err
	}
	return workers.WorkstationDispatchCancelResult{
		DispatchID: request.DispatchID,
		Outcome:    outcome,
	}, nil
}

func executeRequestFromWorkstationRequest(
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
) (workers.ExecuteRequest, error) {
	execution := request.Execution
	dispatch := execution.Dispatch
	workstationName := firstRuntimeValue(request.WorkstationName, dispatch.WorkstationName)
	request.WorkstationName = workstationName
	inputs, invocation, attemptNumber := workInputsFromDispatch(dispatch)
	correlation, err := executionCorrelationFromDispatch(cfg, execution, dispatch)
	if err != nil {
		return workers.ExecuteRequest{}, err
	}
	selection := resolveRuntimeExecutionSelection(cfg, request, inputs, &invocation)
	workflowContext := runtimeWorkflowContext(cfg, correlation.FactorySessionID, execution.WorkflowContext)
	if err := renderRuntimePrompt(
		cfg,
		&selection,
		workers.WorkDispatchInputTokens(dispatch),
		workflowContext,
		inputs,
		&invocation,
	); err != nil {
		return workers.ExecuteRequest{}, err
	}
	return workers.ExecuteRequest{
		Correlation: correlation,
		Target: executionTargetFromSelection(
			selection, workstationName, execution.ProcessEnvironment,
		),
		Input: workers.ExecutionInput{
			Work:              inputs,
			Dispatch:          work.CloneWorkDispatch(dispatch),
			RecordingID:       execution.RecordingID,
			Invocation:        invocation,
			ModelBindings:     workers.CloneResolvedModelOperationBindings(execution.ModelBindings),
			ModelOperation:    firstRuntimeValue(selection.modelOperation, execution.ModelOperation),
			Resume:            continuationFromLegacySession(execution.ResumeSession),
			WorkflowContext:   workflowContext,
			MockWorkers:       cfg.mockWorkersConfig.Clone(),
			ProgressPublisher: cfg.progressPublisher,
		},
		Attempt: workers.AttemptContext{Number: attemptNumber},
	}, nil
}

func executionCorrelationFromDispatch(
	cfg *runtimeConfig,
	execution workers.WorkstationExecutionRequest,
	dispatch work.WorkDispatch,
) (workers.ExecutionCorrelation, error) {
	sessionID := strings.TrimSpace(execution.FactorySessionID)
	runtimeID := strings.TrimSpace(execution.RuntimeID)
	if runtimeID == "" {
		runtimeID = strings.TrimSpace(execution.RecordingID)
	}
	if runtimeID == "" && cfg != nil {
		runtimeID = strings.TrimSpace(cfg.runtimeID)
		if runtimeID == "" {
			runtimeID = strings.TrimSpace(cfg.recordingID)
		}
	}
	generationID := strings.TrimSpace(execution.GenerationID)
	if generationID == "" && cfg != nil && cfg.eventHistory != nil {
		generationID = strings.TrimSpace(cfg.eventHistory.StreamGenerationID())
	}
	correlation := workers.ExecutionCorrelation{
		FactorySessionID: sessionID,
		RuntimeID:        runtimeID,
		GenerationID:     generationID,
		DispatchID:       strings.TrimSpace(dispatch.DispatchID),
		RequestID:        firstRuntimeValue(dispatch.Execution.RequestID, execution.ProjectID),
		TraceID:          strings.TrimSpace(dispatch.Execution.TraceID),
	}
	if correlation.DispatchID == "" {
		return workers.ExecutionCorrelation{}, fmt.Errorf("build worker attempt: dispatch ID is required")
	}
	if correlation.FactorySessionID == "" {
		return workers.ExecutionCorrelation{}, fmt.Errorf("build worker attempt: Factory Session ID is required")
	}
	if correlation.RuntimeID == "" {
		return workers.ExecutionCorrelation{}, fmt.Errorf("build worker attempt: Runtime ID is required")
	}
	if correlation.GenerationID == "" {
		return workers.ExecutionCorrelation{}, fmt.Errorf("build worker attempt: generation ID is required")
	}
	if cfg == nil || cfg.newID == nil {
		return workers.ExecutionCorrelation{}, fmt.Errorf("build worker attempt: Attempt ID generator is required")
	}
	correlation.AttemptID = strings.TrimSpace(cfg.newID())
	if correlation.AttemptID == "" {
		return workers.ExecutionCorrelation{}, fmt.Errorf("build worker attempt: Attempt ID generator returned an empty ID")
	}
	return correlation, nil
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

func executionTargetFromSelection(
	selection runtimeExecutionSelection,
	workstationName string,
	processEnvironment []string,
) workers.ExecutionTarget {
	return workers.ExecutionTarget{
		WorkerName: selection.workerName,
		// WorkerType is the authored worker identity carried into command
		// requests. The definition taxonomy remains available in the runtime
		// selection for route decisions, while mock and provider adapters match
		// the customer-facing worker name.
		WorkerType:       firstRuntimeValue(selection.workerName, selection.workerType),
		WorkstationName:  workstationName,
		RunnerID:         selection.runnerID,
		Noop:             selection.noop,
		Capabilities:     cloneRuntimeCapabilities(selection.capabilities),
		Command:          selection.command,
		Args:             append([]string(nil), selection.args...),
		FactoryDirectory: selection.factoryDirectory,
		Provider: workers.ProviderReference{
			ID:    selection.providerID,
			Alias: selection.modelProvider,
		},
		Model: workers.ModelReference{
			Name:            selection.model,
			Provider:        selection.modelProvider,
			ReasoningEffort: selection.reasoningEffort,
			Locality:        selection.modelLocality,
		},
		Prompt: workers.PromptPolicy{
			SystemPrompt: selection.systemPrompt,
			UserMessage:  selection.userMessage,
			OutputSchema: selection.outputSchema,
		},
		Tools: workers.ToolPolicy{ExecutionMode: selection.toolExecutionMode},
		Output: workers.OutputPolicy{
			Format:                      selection.outputFormat,
			StopToken:                   selection.stopToken,
			Contract:                    selection.outputContract,
			DecisionEnvelope:            selection.decisionEnvelope,
			GoalRoutingDecisionEnvelope: selection.goalRoutingDecisionEnvelope,
			ScriptClassifier:            selection.scriptClassifier,
		},
		Timeout: selection.timeout,
		Environment: workers.EnvironmentPolicy{
			Vars:                selection.environment,
			ProcessEnvironment:  append([]string(nil), processEnvironment...),
			WorkingDirectory:    selection.workingDirectory,
			WorkingDirectorySet: selection.workingDirectoryAuthored,
		},
		Workspace: workers.WorkspacePolicy{
			Worktree:         selection.worktree,
			WorkingDirectory: selection.workingDirectory,
			FactoryDirectory: selection.factoryDirectory,
		},
		Permissions: workers.PermissionPolicy{SkipPermissions: selection.skipPermissions},
	}
}

func workInputsFromDispatch(dispatch work.WorkDispatch) ([]workers.WorkInput, work.InvocationArguments, int) {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	inputs := make([]workers.WorkInput, 0, len(tokens))
	invocation := work.InvocationArguments{}
	attemptNumber := 1
	for _, token := range tokens {
		if token.Color.DataType == workers.DataTypeResource {
			continue
		}
		if token.Color.InvocationArguments != nil && len(invocation.Arguments) == 0 {
			if cloned := work.CloneInvocationArguments(token.Color.InvocationArguments); cloned != nil {
				invocation = *cloned
			}
		}
		candidateAttempt := token.History.TotalVisits[dispatch.TransitionID] + 1
		if candidateAttempt > attemptNumber {
			attemptNumber = candidateAttempt
		}
		lastFailure := ""
		if len(token.History.FailureLog) > 0 {
			lastFailure = token.History.FailureLog[len(token.History.FailureLog)-1].Error
		}
		content := work.CloneWorkContentParts(token.Color.Content)
		if len(content) == 0 && len(token.Color.Payload) > 0 {
			// Older admitted Work tokens carry their canonical text in Payload.
			// Preserve that input when detached execution crosses into the newer
			// content-shaped Worker contract.
			content = []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: string(token.Color.Payload),
			}}
		}
		inputs = append(inputs, workers.WorkInput{
			WorkID:     token.Color.WorkID,
			WorkTypeID: token.Color.WorkTypeID,
			RequestID:  token.Color.RequestID,
			Content:    content,
			Tags:       cloneRuntimeStringMap(token.Color.Tags),
			Relations:  append([]work.Relation(nil), token.Color.Relations...),
			Lineage: workers.WorkLineage{
				ParentWorkID: token.Color.ParentID,
				TraceID:      token.Color.TraceID,
				OriginRef:    token.Color.Name,
			},
			AttemptFacts: workers.AttemptFacts{
				AttemptNumber: candidateAttempt,
				LastFailure:   lastFailure,
			},
		})
	}
	return inputs, invocation, attemptNumber
}

func continuationFromLegacySession(session *providers.SessionRef) *workers.ProviderContinuationRef {
	if session == nil {
		return nil
	}
	provider := strings.TrimSpace(string(session.Provider))
	id := strings.TrimSpace(session.ID)
	if provider == "" && id == "" {
		return nil
	}
	return &workers.ProviderContinuationRef{
		Provider:          provider,
		ProviderSessionID: id,
		ExternalRef:       id,
	}
}

// InvokeWorker runs one orchestrator-resolved Worker through the same
// Runtime-owned attempt lifecycle a Petri dispatch gets. When the composed
// Worker Sessions service exposes the optional runtime-attempt bridge, the
// invocation also receives the durable opening/observation window without
// transferring execution or cancellation ownership away from Runtime.
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
	if f != nil && f.cfg != nil && f.cfg.attempts != nil {
		return f.invokeStatelessWorker(ctx, req)
	}
	if f == nil || f.cfg == nil || f.cfg.workerSessions == nil || f.eventHistory == nil {
		return factory.InvokeWorkerResult{}, factory.ErrNotRunning
	}

	dispatchID := strings.TrimSpace(req.DispatchID)
	sessionID, err := f.reserveWorkerSession(ctx, dispatchID)
	if err != nil {
		return factory.InvokeWorkerResult{}, err
	}
	// Workers is given the Worker Session identity, not the caller's. Workers
	// treats a dispatch ID as single-use for its pool's whole life -- an
	// accepted dispatch is never removed from the pool's record map -- so a
	// re-run under the caller's original ID is refused before it reaches an
	// executor. The Worker Session identity is the one already minted uniquely
	// per attempt, and for every Worker but a resumed one it is that same
	// caller ID.
	execution := providerInvocationExecutionRequest(f, req, sessionID)
	f.eventHistory.RecordDispatchWorkerSessionAssociation(
		f.currentTick(),
		dispatchID,
		sessionID,
		execution.Execution.Dispatch.Execution.RequestID,
		f.cfg.clock.Now(),
	)

	// The caller's cancellation reaches the Worker through the Worker Session's
	// own control, not through the invocation context. Workers deliberately
	// detaches the dispatch context -- a dispatch is cancelled by
	// CancelWorkstationDispatch, never by its caller going away -- so passing a
	// cancellable context down would be ignored, and the running provider would
	// keep going after the workflow that asked for it had stopped.
	stopWatching := f.cancelSessionWhenCallerStops(ctx, sessionID)
	defer stopWatching()

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
	return invokeWorkerResultFrom(dispatchID, sessionID, result), nil
}

func (f *factoryImpl) invokeStatelessWorker(
	ctx context.Context,
	req factory.InvokeWorkerRequest,
) (factory.InvokeWorkerResult, error) {
	dispatchID := strings.TrimSpace(req.DispatchID)
	execution := providerInvocationExecutionRequest(f, req, dispatchID)
	executeRequest, err := executeRequestFromWorkstationRequest(f.cfg, execution)
	if err != nil {
		return factory.InvokeWorkerResult{}, err
	}
	if f.cfg.eventHistory != nil {
		sessionID := runtimeWorkerSessionID(f.cfg, execution, executeRequest, false)
		f.cfg.eventHistory.RecordDispatchWorkerSessionAssociation(
			f.currentTick(),
			dispatchID,
			sessionID,
			executeRequest.Input.Dispatch.Execution.RequestID,
			f.cfg.clock.Now(),
		)
	}
	_, resumed := f.cfg.attempts.terminalAttemptID(dispatchID)
	if !resumed {
		// Preserve the caller-facing identity for the common first invocation.
		executeRequest.Correlation.AttemptID = dispatchID
	}
	budget := effectiveInvokeWorkerAttempts(req.MaxAttempts)
	for attemptNumber := 1; attemptNumber <= budget; attemptNumber++ {
		if attemptNumber > 1 {
			executeRequest.Correlation.AttemptID = fmt.Sprintf("%s/attempt/%d", dispatchID, attemptNumber)
		}
		executeRequest.Attempt.Number = attemptNumber
		var dispatchResult workers.WorkstationDispatchResult
		start := startStatelessAttemptWithRequest
		if resumed || attemptNumber > 1 {
			start = startStatelessAttemptWithRequestRetry
		}
		if err := start(
			ctx,
			f.cfg,
			execution,
			executeRequest,
			false,
			func(_ context.Context, _ workers.WorkstationDispatchRequest, result workers.WorkstationDispatchResult, _ error) {
				dispatchResult = result
			},
		); err != nil {
			return factory.InvokeWorkerResult{}, err
		}
		invoked := invokeWorkerResultFromDispatch(
			dispatchID,
			executeRequest.Correlation.AttemptID,
			dispatchResult,
			attemptNumber,
		)
		if !invokeWorkerShouldRetry(ctx, invoked, attemptNumber, budget) {
			return invoked, nil
		}
	}
	return factory.InvokeWorkerResult{}, fmt.Errorf("InvokeWorker exhausted without a terminal result")
}

func effectiveInvokeWorkerAttempts(maxAttempts int) int {
	if maxAttempts < 1 {
		return 1
	}
	return maxAttempts
}

func invokeWorkerShouldRetry(
	ctx context.Context,
	result factory.InvokeWorkerResult,
	attemptNumber int,
	budget int,
) bool {
	if result.Outcome != factory.InvokeWorkerOutcomeFailed ||
		result.Retryable == nil || !*result.Retryable ||
		attemptNumber >= budget || ctx == nil {
		return false
	}
	return ctx.Err() == nil
}

func invokeWorkerResultFromDispatch(
	dispatchID string,
	attemptID string,
	result workers.WorkstationDispatchResult,
	attempts int,
) factory.InvokeWorkerResult {
	outcome := factory.InvokeWorkerOutcomeCompleted
	switch result.TerminalOutcome {
	case workers.WorkstationDispatchTerminalOutcomeCanceled:
		outcome = factory.InvokeWorkerOutcomeCanceled
	case workers.WorkstationDispatchTerminalOutcomeFailed:
		outcome = factory.InvokeWorkerOutcomeFailed
	}
	invoked := factory.InvokeWorkerResult{
		DispatchID:      dispatchID,
		WorkerSessionID: attemptID,
		Outcome:         outcome,
		Output:          result.Result.Output,
		Attempts:        attempts,
	}
	if session := result.Result.ProviderSession; session != nil {
		invoked.Provider = workers.CanonicalProviderSessionProvider(session.Provider)
		if invoked.Provider == "" {
			invoked.Provider = strings.TrimSpace(session.Provider)
		}
		invoked.ProviderSessionRef = strings.TrimSpace(session.ID)
	}
	if outcome != factory.InvokeWorkerOutcomeCompleted {
		invoked.Diagnostic = strings.TrimSpace(result.Result.Error)
		if metadata := result.Result.FailureMetadata; metadata != nil {
			invoked.FailureReason = string(metadata.Type)
			decision := workers.FailureDecisionFromMetadata(metadata)
			invoked.Retryable = &decision.Retryable
		}
		if invoked.Diagnostic == "" {
			invoked.Diagnostic = "Provider execution failed."
		}
	}
	return invoked
}

// reserveWorkerSession claims the Worker Session identity for one dispatch.
//
// A Worker Session identity is normally the dispatch ID, which is what keeps a
// Worker one tool call. A JavaScript workflow resumed after an interruption
// re-runs the child that was cut off under its original dispatch ID, so that
// identity is already taken by the canceled attempt. The resumed run takes
// ".../resume/N" -- the same shape Worker Sessions already mints for its own
// resume -- so the interrupted Worker keeps its terminal record and the resumed
// one is honestly a second Worker rather than a reopened first.
func (f *factoryImpl) reserveWorkerSession(ctx context.Context, dispatchID string) (string, error) {
	reserveCtx := context.WithoutCancel(ctx)
	candidate := dispatchID
	for attempt := 0; attempt <= maxWorkerSessionResumeAttempts; attempt++ {
		if attempt > 0 {
			candidate = fmt.Sprintf("%s/resume/%d", dispatchID, attempt)
		}
		_, err := f.cfg.workerSessions.Reserve(
			reserveCtx,
			workersessions.ReserveRequest{ID: candidate},
		)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, workersessions.ErrSessionAlreadyExists) {
			return "", err
		}
	}
	return "", fmt.Errorf(
		"%w: dispatch %q exhausted Worker Session resume identities",
		factory.ErrInvalidInvokeWorkerRequest,
		dispatchID,
	)
}

// maxWorkerSessionResumeAttempts bounds identity minting so a session that can
// never be reserved fails instead of looping.
const maxWorkerSessionResumeAttempts = 64

// cancelSessionWhenCallerStops translates one caller's cancellation into the
// Worker Session control that actually stops a running Worker. The returned
// function releases the watcher and must be called before InvokeWorker returns.
//
// A caller with no cancellation to observe gets no goroutine at all.
func (f *factoryImpl) cancelSessionWhenCallerStops(ctx context.Context, sessionID string) func() {
	if ctx == nil || ctx.Done() == nil {
		return func() {}
	}
	released := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// The control itself must outlive the cancellation that triggered
			// it, or it would be refused for the very reason it was issued.
			_, _ = f.cfg.workerSessions.Cancel(
				context.WithoutCancel(ctx),
				workersessions.ControlRequest{ID: sessionID},
			)
		case <-released:
		}
	}()
	return sync.OnceFunc(func() { close(released) })
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
	requestID := sessionIDFromFactoryConfig(f.cfg)
	if f.cfg != nil && f.cfg.workflowContext != nil {
		if sessionID := strings.TrimSpace(f.cfg.workflowContext.SessionID); sessionID != "" {
			requestID = sessionID
		}
	}
	// The worker name is the Workers-facing worker type: it is what a
	// mock-worker configuration matches on at the subprocess boundary, and an
	// unnamed Worker must leave it empty so an unmatched dispatch stays
	// unmatched rather than colliding with some other worker's name.
	workerName := strings.TrimSpace(req.WorkerName)
	dispatch := work.WorkDispatch{
		DispatchID:      dispatchID,
		WorkstationName: workers.ProviderInvocationRoute,
		WorkerType:      workerName,
		Execution: work.ExecutionMetadata{
			RequestID: requestID,
		},
	}
	recordingID := strings.TrimSpace(req.RecordingID)
	if recordingID == "" && f.cfg != nil {
		recordingID = strings.TrimSpace(f.cfg.recordingID)
	}
	runtimeID := ""
	if f.cfg != nil {
		runtimeID = strings.TrimSpace(f.cfg.runtimeID)
	}
	return workers.WorkstationDispatchRequest{
		WorkstationName: workers.ProviderInvocationRoute,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:         dispatch,
			WorkerType:       workerName,
			SkipPermissions:  req.SkipPermissions,
			RunnerID:         strings.TrimSpace(req.RunnerID),
			ExecutorProvider: strings.TrimSpace(req.ExecutorProvider),
			FactorySessionID: requestID,
			RuntimeID:        runtimeID,
			RecordingID:      recordingID,
			Capabilities:     cloneSessionCapabilities(req.Capabilities),
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

func cloneSessionCapabilities(value *workers.Capabilities) *workers.Capabilities {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// invokeWorkerResultFrom narrows one Worker Sessions outcome onto the caller
// contract. Only the bounded classification and the provider's own output
// cross: InvocationResult-style diagnostics stay inside Workers, because they
// can carry command lines and credentials.
func invokeWorkerResultFrom(
	dispatchID string,
	sessionID string,
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
		WorkerSessionID: sessionID,
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
		if metadata := result.Dispatch.Result.FailureMetadata; metadata != nil {
			invoked.FailureReason = string(metadata.Type)
			decision := workers.FailureDecisionFromMetadata(metadata)
			invoked.Retryable = &decision.Retryable
		}
		if invoked.FailureReason == "" && result.Session.Result != nil && result.Session.Result.Cause != nil {
			invoked.FailureReason = string(result.Session.Result.Cause.Kind)
		}
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
