package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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
	workstationName := firstRuntimeValue(request.WorkstationName, dispatch.WorkstationName)
	inputs, invocation, attemptNumber := workInputsFromDispatch(dispatch)
	correlation, err := executionCorrelationFromDispatch(cfg, execution, dispatch)
	if err != nil {
		return workers.ExecuteRequest{}, err
	}
	selection := resolveRuntimeExecutionSelection(cfg, request, inputs)
	return workers.ExecuteRequest{
		Correlation: correlation,
		Target: executionTargetFromSelection(
			selection, workstationName, execution.ProcessEnvironment,
		),
		Input: workers.ExecutionInput{
			Work:           inputs,
			Dispatch:       work.CloneWorkDispatch(dispatch),
			Invocation:     invocation,
			ModelBindings:  workers.CloneResolvedModelOperationBindings(execution.ModelBindings),
			ModelOperation: execution.ModelOperation,
			Resume:         continuationFromLegacySession(execution.ResumeSession),
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
	if sessionID == "" && cfg != nil {
		sessionID = sessionIDFromFactoryConfig(cfg)
	}
	correlation := workers.ExecutionCorrelation{
		FactorySessionID: sessionID,
		RuntimeID:        strings.TrimSpace(execution.RecordingID),
		DispatchID:       strings.TrimSpace(dispatch.DispatchID),
		RequestID:        firstRuntimeValue(dispatch.Execution.RequestID, execution.ProjectID),
		TraceID:          strings.TrimSpace(dispatch.Execution.TraceID),
	}
	if correlation.DispatchID == "" {
		return workers.ExecutionCorrelation{}, fmt.Errorf("build worker attempt: dispatch ID is required")
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

func executionTargetFromSelection(
	selection runtimeExecutionSelection,
	workstationName string,
	processEnvironment []string,
) workers.ExecutionTarget {
	return workers.ExecutionTarget{
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
	selection := initialRuntimeExecutionSelection(request.Execution)
	if lookup, ok := runtimeDefinitionLookup(cfg); ok {
		applyRuntimeDefinitionSelection(cfg, lookup, request, &selection)
	}
	finalizeRuntimeExecutionSelection(&selection, inputs)
	return selection
}

func initialRuntimeExecutionSelection(
	execution workers.WorkstationExecutionRequest,
) runtimeExecutionSelection {
	return runtimeExecutionSelection{
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
}

func applyRuntimeDefinitionSelection(
	cfg *runtimeConfig,
	lookup interfaces.RuntimeDefinitionLookup,
	request workers.WorkstationDispatchRequest,
	selection *runtimeExecutionSelection,
) {
	if selection.workerName == "" {
		selection.workerName = selection.workerType
	}
	worker, workerFound := lookup.Worker(selection.workerName)
	workstation, workstationFound := lookup.Workstation(strings.TrimSpace(request.WorkstationName))
	if !workerFound && workstationFound {
		worker, workerFound = lookup.Worker(workstation.WorkerTypeName)
	}
	if workerFound && worker != nil {
		applyRuntimeWorkerSelection(selection, request.Execution, worker)
	}
	if workstationFound && workstation != nil {
		applyRuntimeWorkstationSelection(selection, workstation)
	}
	applyRuntimeConfigSelection(cfg, selection)
}

func applyRuntimeWorkerSelection(
	selection *runtimeExecutionSelection,
	execution workers.WorkstationExecutionRequest,
	worker *interfaces.FactoryWorkerConfig,
) {
	selection.workerName = firstRuntimeValue(strings.TrimSpace(execution.WorkerName), worker.Name)
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
	if worker.AgentTools != nil && strings.TrimSpace(worker.AgentTools.Policy) != "" &&
		!strings.EqualFold(worker.AgentTools.Policy, "DISABLED") {
		selection.toolExecutionMode = workers.RunnerToolExecutionModeRequired
	}
}

func applyRuntimeWorkstationSelection(
	selection *runtimeExecutionSelection,
	workstation *interfaces.FactoryWorkstationConfig,
) {
	selection.runnerID = firstRuntimeValue(selection.runnerID, workstation.Runner)
	selection.systemPrompt = firstRuntimeValue(selection.systemPrompt, workstation.Body)
	selection.outputSchema = firstRuntimeValue(selection.outputSchema, workstation.OutputSchema)
	selection.outputContract = firstRuntimeValue(selection.outputContract, workstation.OutputContract)
	selection.outputFormat = firstRuntimeValue(selection.outputFormat, workstation.OutcomeFormat)
	selection.workingDirectory = firstRuntimeValue(selection.workingDirectory, workstation.WorkingDirectory)
	selection.workingDirectoryAuthored = selection.workingDirectoryAuthored ||
		strings.TrimSpace(workstation.WorkingDirectory) != ""
	selection.worktree = firstRuntimeValue(selection.worktree, workstation.Worktree)
	selection.environment = mergeRuntimeStringMaps(workstation.Env, selection.environment)
	if selection.timeout <= 0 {
		selection.timeout = parseRuntimeDuration(workstation.Timeout)
	}
	selection.decisionEnvelope = selection.decisionEnvelope ||
		workstation.OutputContract == "decision"
}

func applyRuntimeConfigSelection(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
) {
	configLookup, ok := cfg.runtimeConfig.(interfaces.RuntimeConfigLookup)
	if !ok || selection.factoryDirectory != "" {
		return
	}
	selection.factoryDirectory = strings.TrimSpace(configLookup.FactoryDir())
	if selection.runnerID == "" {
		if factoryConfig := configLookup.FactoryConfig(); factoryConfig != nil {
			selection.runnerID = strings.TrimSpace(factoryConfig.Runner)
		}
	}
}

func finalizeRuntimeExecutionSelection(
	selection *runtimeExecutionSelection,
	inputs []workers.WorkInput,
) {
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
		var dispatchErr error
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
			func(_ context.Context, _ workers.WorkstationDispatchRequest, result workers.WorkstationDispatchResult, err error) {
				dispatchResult = result
				dispatchErr = err
			},
		); err != nil {
			return factory.InvokeWorkerResult{}, err
		}
		if dispatchErr != nil {
			return factory.InvokeWorkerResult{}, dispatchErr
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
	requestID := ""
	if f.cfg != nil && f.cfg.workflowContext != nil {
		requestID = strings.TrimSpace(f.cfg.workflowContext.SessionID)
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
	return workers.WorkstationDispatchRequest{
		WorkstationName: workers.ProviderInvocationRoute,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:         dispatch,
			WorkerType:       workerName,
			SkipPermissions:  req.SkipPermissions,
			RunnerID:         strings.TrimSpace(req.RunnerID),
			ExecutorProvider: strings.TrimSpace(req.ExecutorProvider),
			FactorySessionID: requestID,
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
