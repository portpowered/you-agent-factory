package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
	executeRequest, err := executeRequestFromWorkstationRequest(cfg, request)
	if err != nil {
		return err
	}
	// Worker Session association is an optional historical observation edge.
	// It does not own admission, cancellation, execution, or terminal
	// authority; the attempt lifecycle below remains the only execution owner.
	if cfg.workerSessions != nil {
		sessionID := executeRequest.Correlation.DispatchID
		if _, reserveErr := cfg.workerSessions.Reserve(
			context.WithoutCancel(ctx),
			workersessions.ReserveRequest{ID: sessionID},
		); reserveErr != nil {
			return reserveErr
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
	}
	return startStatelessAttemptWithRequest(
		ctx, cfg, request, executeRequest,
		!cfg.inlineDispatch && cfg.completionDeliveryPlanner == nil, accept,
	)
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
	start := cfg.attempts.start
	if allowRetry {
		start = cfg.attempts.startRetry
	}
	return start(
		ctx,
		executeRequest,
		async,
		func(callbackCtx context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, executeErr error) {
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
	workstationName := strings.TrimSpace(request.WorkstationName)
	if workstationName == "" {
		workstationName = strings.TrimSpace(dispatch.WorkstationName)
	}
	workerType := strings.TrimSpace(request.Execution.WorkerType)
	if workerType == "" {
		workerType = strings.TrimSpace(dispatch.WorkerType)
	}
	inputs, invocation, attemptNumber := workInputsFromDispatch(dispatch)
	sessionID := strings.TrimSpace(execution.FactorySessionID)
	if sessionID == "" && cfg != nil {
		sessionID = sessionIDFromFactoryConfig(cfg)
	}
	runtimeID := strings.TrimSpace(execution.RecordingID)
	requestID := strings.TrimSpace(dispatch.Execution.RequestID)
	if requestID == "" {
		requestID = strings.TrimSpace(execution.ProjectID)
	}
	correlation := workers.ExecutionCorrelation{
		FactorySessionID: sessionID,
		RuntimeID:        runtimeID,
		DispatchID:       strings.TrimSpace(dispatch.DispatchID),
		RequestID:        requestID,
		TraceID:          strings.TrimSpace(dispatch.Execution.TraceID),
	}
	if correlation.DispatchID == "" {
		return workers.ExecuteRequest{}, fmt.Errorf("build worker attempt: dispatch ID is required")
	}
	if cfg == nil || cfg.newID == nil {
		return workers.ExecuteRequest{}, fmt.Errorf("build worker attempt: Attempt ID generator is required")
	}
	correlation.AttemptID = strings.TrimSpace(cfg.newID())
	if correlation.AttemptID == "" {
		return workers.ExecuteRequest{}, fmt.Errorf("build worker attempt: Attempt ID generator returned an empty ID")
	}

	selection := resolveRuntimeExecutionSelection(cfg, request, inputs)
	resume := continuationFromLegacySession(execution.ResumeSession)
	return workers.ExecuteRequest{
		Correlation: correlation,
		Target: workers.ExecutionTarget{
			WorkerName:       selection.workerName,
			WorkerType:       selection.workerType,
			WorkstationName:  workstationName,
			RunnerID:         selection.runnerID,
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
			},
			Timeout: selection.timeout,
			Environment: workers.EnvironmentPolicy{
				Vars:                selection.environment,
				ProcessEnvironment:  append([]string(nil), execution.ProcessEnvironment...),
				WorkingDirectory:    selection.workingDirectory,
				WorkingDirectorySet: selection.workingDirectoryAuthored,
			},
			Workspace: workers.WorkspacePolicy{
				Worktree:           selection.worktree,
				WorkingDirectory:   selection.workingDirectory,
				FactoryDirectory:   selection.factoryDirectory,
				PrepareWorktree:    false,
				CheckoutIdentifier: "",
			},
			Permissions: workers.PermissionPolicy{SkipPermissions: selection.skipPermissions},
		},
		Input: workers.ExecutionInput{
			Work:           inputs,
			Dispatch:       work.CloneWorkDispatch(dispatch),
			Invocation:     invocation,
			ModelBindings:  workers.CloneResolvedModelOperationBindings(execution.ModelBindings),
			ModelOperation: execution.ModelOperation,
			Resume:         resume,
		},
		Attempt: workers.AttemptContext{Number: attemptNumber},
	}, nil
}

type runtimeExecutionSelection struct {
	workerName                  string
	workerType                  string
	runnerID                    string
	providerID                  string
	model                       string
	modelProvider               string
	modelLocality               string
	reasoningEffort             string
	capabilities                *workers.Capabilities
	command                     string
	args                        []string
	factoryDirectory            string
	systemPrompt                string
	userMessage                 string
	outputSchema                string
	outputContract              string
	outputFormat                string
	stopToken                   string
	decisionEnvelope            bool
	goalRoutingDecisionEnvelope bool
	toolExecutionMode           workers.RunnerToolExecutionMode
	environment                 map[string]string
	workingDirectory            string
	workingDirectoryAuthored    bool
	worktree                    string
	skipPermissions             bool
	timeout                     time.Duration
}

func resolveRuntimeExecutionSelection(
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	inputs []workers.WorkInput,
) runtimeExecutionSelection {
	execution := request.Execution
	selection := runtimeExecutionSelection{
		workerName:                  strings.TrimSpace(execution.WorkerName),
		workerType:                  firstRuntimeValue(execution.WorkerType, execution.Dispatch.WorkerType),
		runnerID:                    strings.TrimSpace(execution.RunnerID),
		providerID:                  strings.TrimSpace(execution.ExecutorProvider),
		model:                       strings.TrimSpace(execution.Model),
		modelProvider:               strings.TrimSpace(execution.ModelProvider),
		reasoningEffort:             strings.TrimSpace(execution.ReasoningEffort),
		capabilities:                cloneRuntimeCapabilities(execution.Capabilities),
		command:                     execution.Command,
		args:                        append([]string(nil), execution.Args...),
		factoryDirectory:            strings.TrimSpace(execution.FactoryDirectory),
		systemPrompt:                execution.SystemPrompt,
		userMessage:                 execution.UserMessage,
		outputSchema:                execution.OutputSchema,
		outputContract:              execution.OutputContract,
		outputFormat:                execution.OutputFormat,
		stopToken:                   execution.StopToken,
		decisionEnvelope:            execution.DecisionEnvelope,
		goalRoutingDecisionEnvelope: execution.GoalRoutingDecisionEnvelope,
		environment:                 cloneRuntimeStringMap(execution.EnvVars),
		workingDirectory:            strings.TrimSpace(execution.WorkingDirectory),
		workingDirectoryAuthored:    execution.WorkingDirectoryAuthored,
		worktree:                    strings.TrimSpace(execution.Worktree),
		skipPermissions:             execution.SkipPermissions,
		timeout:                     execution.Timeout,
	}
	if selection.workerName == "" {
		selection.workerName = selection.workerType
	}
	if lookup, ok := runtimeDefinitionLookup(cfg); ok {
		worker, workerFound := lookup.Worker(selection.workerName)
		workstation, workstationFound := lookup.Workstation(strings.TrimSpace(request.WorkstationName))
		if !workerFound && workstationFound {
			worker, workerFound = lookup.Worker(workstation.WorkerTypeName)
		}
		if workerFound && worker != nil {
			selection.workerName = firstRuntimeValue(selection.workerName, worker.Name)
			selection.workerType = firstRuntimeValue(worker.Type, selection.workerType)
			selection.providerID = firstRuntimeValue(selection.providerID, worker.ExecutorProvider, worker.Provider)
			selection.model = firstRuntimeValue(selection.model, worker.Model)
			selection.modelProvider = firstRuntimeValue(selection.modelProvider, worker.ModelProvider)
			selection.reasoningEffort = firstRuntimeValue(selection.reasoningEffort, worker.ReasoningEffort)
			selection.modelLocality = strings.TrimSpace(worker.ModelLocality)
			selection.command = firstRuntimeValue(selection.command, worker.Command)
			if len(selection.args) == 0 {
				selection.args = append([]string(nil), worker.Args...)
			}
			selection.stopToken = firstRuntimeValue(selection.stopToken, worker.StopToken)
			selection.skipPermissions = selection.skipPermissions || worker.SkipPermissions
			if selection.timeout <= 0 {
				selection.timeout = worker.TimeoutDuration()
			}
			if worker.AgentTools != nil && strings.TrimSpace(worker.AgentTools.Policy) != "" && strings.EqualFold(worker.AgentTools.Policy, "DISABLED") == false {
				selection.toolExecutionMode = workers.RunnerToolExecutionModeRequired
			}
		}
		if workstationFound && workstation != nil {
			selection.runnerID = firstRuntimeValue(selection.runnerID, workstation.Runner)
			selection.systemPrompt = firstRuntimeValue(selection.systemPrompt, workstation.Body)
			selection.outputSchema = firstRuntimeValue(selection.outputSchema, workstation.OutputSchema)
			selection.outputContract = firstRuntimeValue(selection.outputContract, workstation.OutputContract)
			selection.outputFormat = firstRuntimeValue(selection.outputFormat, workstation.OutcomeFormat)
			selection.workingDirectory = firstRuntimeValue(selection.workingDirectory, workstation.WorkingDirectory)
			selection.workingDirectoryAuthored = selection.workingDirectoryAuthored || strings.TrimSpace(workstation.WorkingDirectory) != ""
			selection.worktree = firstRuntimeValue(selection.worktree, workstation.Worktree)
			selection.environment = mergeRuntimeStringMaps(workstation.Env, selection.environment)
			if selection.timeout <= 0 {
				selection.timeout = parseRuntimeDuration(workstation.Timeout)
			}
			selection.decisionEnvelope = selection.decisionEnvelope || workstation.OutputContract != "" && workstation.OutputContract == "decision"
		}
		if configLookup, ok := cfg.runtimeConfig.(interfaces.RuntimeConfigLookup); ok && selection.factoryDirectory == "" {
			selection.factoryDirectory = strings.TrimSpace(configLookup.FactoryDir())
			if selection.runnerID == "" {
				if factoryConfig := configLookup.FactoryConfig(); factoryConfig != nil {
					selection.runnerID = strings.TrimSpace(factoryConfig.Runner)
				}
			}
		}
	}
	if selection.providerID == "" {
		selection.providerID = selection.modelProvider
	}
	if selection.modelProvider == "" {
		selection.modelProvider = selection.providerID
	}
	if selection.runnerID == "" && selection.providerID == "" && selection.model == "" {
		selection.runnerID = workers.RunnerIDCodex
	}
	if selection.userMessage == "" {
		selection.userMessage = workInputMessage(inputs)
	}
	if selection.toolExecutionMode == "" {
		selection.toolExecutionMode = workers.RunnerToolExecutionModeDisabled
	}
	selection.environment = mergeRuntimeStringMaps(nil, selection.environment)
	return selection
}

func runtimeDefinitionLookup(cfg *runtimeConfig) (interfaces.RuntimeDefinitionLookup, bool) {
	if cfg == nil || cfg.runtimeConfig == nil {
		return nil, false
	}
	lookup, ok := cfg.runtimeConfig.(interfaces.RuntimeDefinitionLookup)
	return lookup, ok && lookup != nil
}

func firstRuntimeValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mergeRuntimeStringMaps(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := cloneRuntimeStringMap(base)
	if merged == nil {
		merged = make(map[string]string, len(override))
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func parseRuntimeDuration(value string) time.Duration {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || duration < 0 {
		return 0
	}
	return duration
}

func workInputMessage(inputs []workers.WorkInput) string {
	for _, input := range inputs {
		for _, part := range input.Content {
			if text := strings.TrimSpace(part.Text); text != "" {
				return text
			}
		}
	}
	return ""
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
		inputs = append(inputs, workers.WorkInput{
			WorkID:     token.Color.WorkID,
			WorkTypeID: token.Color.WorkTypeID,
			RequestID:  token.Color.RequestID,
			Content:    work.CloneWorkContentParts(token.Color.Content),
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

func workstationDispatchResultFromExecute(
	request workers.WorkstationDispatchRequest,
	result workers.ExecuteResult,
	executeErr error,
) (workers.WorkstationDispatchResult, error) {
	dispatch := request.Execution.Dispatch
	proposedOutput := result.Output.Clone()
	workResult := workers.WorkResult{
		DispatchID:                  dispatch.DispatchID,
		TransitionID:                dispatch.TransitionID,
		Outcome:                     workers.OutcomeAccepted,
		Output:                      primaryOutputText(result.Output.Primary),
		Feedback:                    result.Output.Feedback,
		SelectedClassificationLabel: result.Output.Classification,
		Metrics: workers.WorkMetrics{
			Duration:   result.Metrics.Duration,
			Cost:       result.Metrics.Cost,
			RetryCount: result.Metrics.RetryCount,
		},
		ProviderSession: providerSessionFromContinuation(result.Continuation),
	}
	terminal := workers.WorkstationDispatchTerminalOutcomeCompleted
	switch result.Outcome {
	case workers.ExecutionOutcomeContinue:
		workResult.Outcome = workers.OutcomeContinue
	case workers.ExecutionOutcomeRejected:
		workResult.Outcome = workers.OutcomeRejected
	case workers.ExecutionOutcomeFailed:
		workResult.Outcome = workers.OutcomeFailed
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
	case workers.ExecutionOutcomeCanceled:
		workResult.Outcome = workers.OutcomeFailed
		terminal = workers.WorkstationDispatchTerminalOutcomeCanceled
	default:
		if result.Outcome != workers.ExecutionOutcomeAccepted {
			workResult.Outcome = workers.OutcomeFailed
			terminal = workers.WorkstationDispatchTerminalOutcomeFailed
		}
	}
	if result.Failure != nil {
		workResult.Error = strings.TrimSpace(result.Failure.Message)
		workResult.FailureMetadata = &workers.WorkFailureMetadata{
			Family: result.Failure.Family,
			Type:   result.Failure.Type,
		}
	}
	if executeErr != nil && terminal != workers.WorkstationDispatchTerminalOutcomeCanceled {
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
		workResult.Outcome = workers.OutcomeFailed
		if strings.TrimSpace(workResult.Error) == "" {
			workResult.Error = executeErr.Error()
		}
	}
	if terminal == workers.WorkstationDispatchTerminalOutcomeCanceled && strings.TrimSpace(workResult.Error) == "" {
		workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
	}
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: terminal,
		Result:          workResult,
		ProposedOutput:  &proposedOutput,
	}, executeErr
}

func primaryOutputText(parts []work.WorkContentPart) string {
	for _, part := range parts {
		switch part.Type.Normalized() {
		case work.WorkContentPartTypeText:
			if strings.TrimSpace(part.Text) != "" {
				return part.Text
			}
		case work.WorkContentPartTypeJSON:
			if len(part.JSON) > 0 {
				return string(part.JSON)
			}
		case work.WorkContentPartTypeImage, work.WorkContentPartTypeAudio, work.WorkContentPartTypeBinary:
			if strings.TrimSpace(part.URL) != "" {
				return part.URL
			}
			if strings.TrimSpace(part.File) != "" {
				return part.File
			}
		}
	}
	return ""
}

func providerSessionFromContinuation(
	continuation *workers.ProviderContinuationRef,
) *workers.ProviderSessionMetadata {
	if continuation == nil {
		return nil
	}
	id := strings.TrimSpace(continuation.ProviderSessionID)
	if id == "" {
		id = strings.TrimSpace(continuation.ExternalRef)
	}
	if id == "" && strings.TrimSpace(continuation.Provider) == "" {
		return nil
	}
	return &workers.ProviderSessionMetadata{
		Provider: continuation.Provider,
		Kind:     "session",
		ID:       id,
	}
}

func cloneRuntimeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneRuntimeCapabilities(value *workers.Capabilities) *workers.Capabilities {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
