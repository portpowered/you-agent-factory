package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type unavailableProviderSessions struct {
	providersessions.Service
}

// testRuntimeWorkService is an inert owner-local Work role for Runtime tests
// that do not exercise Work output materialization. Tests that cover that
// behavior inject their own focused Work service fake.
type testRuntimeWorkService struct {
	work.Service
}

func (testRuntimeWorkService) MaterializeWorkerOutput(ctx context.Context, request work.MaterializeWorkerOutputRequest) (work.MaterializeWorkerOutputResult, error) {
	if err := ctx.Err(); err != nil {
		return work.MaterializeWorkerOutputResult{}, err
	}
	result := work.MaterializeWorkerOutputResult{
		PrimaryOutput:  testMaterializationPrimaryOutput(request.Primary),
		Feedback:       strings.TrimSpace(request.Feedback),
		Classification: strings.TrimSpace(request.Classification),
	}
	for index, proposal := range request.ProposedWork {
		materialized, err := materializeTestProposal(request, proposal, index)
		if err != nil {
			return work.MaterializeWorkerOutputResult{}, err
		}
		result.MaterializedWork = append(result.MaterializedWork, materialized)
	}
	return result, nil
}

func materializeTestProposal(
	request work.MaterializeWorkerOutputRequest,
	proposal work.ProposedWorkItem,
	index int,
) (work.FactoryWorkItem, error) {
	name := strings.TrimSpace(proposal.Name)
	if name == "" {
		name = fmt.Sprintf("proposed-%d", index+1)
	}
	workTypeID := strings.TrimSpace(proposal.WorkTypeID)
	if workTypeID == "" {
		workTypeID = strings.TrimSpace(request.DefaultWorkTypeID)
	}
	if workTypeID == "" {
		return work.FactoryWorkItem{}, fmt.Errorf("%w: proposed work %q is missing work type", work.ErrInvalidProposedWork, name)
	}
	if request.ValidWorkTypes != nil && !request.ValidWorkTypes[workTypeID] {
		return work.FactoryWorkItem{}, fmt.Errorf("%w: proposed work %q references unknown work type %q", work.ErrUnknownProposedWorkType, name, workTypeID)
	}
	state := strings.TrimSpace(proposal.State)
	if state != "" && request.ValidStatesByType != nil && !request.ValidStatesByType[workTypeID][state] {
		return work.FactoryWorkItem{}, fmt.Errorf("%w: proposed work %q references unknown state %q", work.ErrInvalidProposedWork, name, state)
	}
	identity := fmt.Sprintf("batch-%s-%s", request.Lineage.RequestID, name)
	if request.IDGenerator != nil {
		generated := strings.TrimSpace(request.IDGenerator())
		if generated == "" {
			return work.FactoryWorkItem{}, fmt.Errorf("%w: ID generator returned an empty identity", work.ErrInvalidProposedWork)
		}
		identity = "work-" + generated
	}
	parentID := strings.TrimSpace(request.Lineage.ParentWorkID)
	if parentID == "" && len(request.Lineage.SourceWorkIDs) > 0 {
		parentID = strings.TrimSpace(request.Lineage.SourceWorkIDs[0])
	}
	tags := work.CloneTags(proposal.Tags)
	if tags == nil {
		tags = map[string]string{}
	}
	tags["_work_name"] = name
	tags["_work_type"] = workTypeID
	return work.FactoryWorkItem{
		ID:                       identity,
		WorkTypeID:               workTypeID,
		State:                    state,
		DisplayName:              name,
		ChainingTraceDepth:       request.Lineage.ChainingTraceDepth,
		CurrentChainingTraceID:   request.Lineage.CurrentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), request.Lineage.PreviousChainingTraceIDs...),
		TraceID:                  request.Lineage.TraceID,
		Content:                  work.CloneWorkContentParts(proposal.Content),
		ParentID:                 parentID,
		Tags:                     tags,
	}, nil
}

func testMaterializationPrimaryOutput(parts []work.WorkContentPart) string {
	var output strings.Builder
	for _, part := range parts {
		if part.Type != work.WorkContentPartTypeText {
			continue
		}
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		if output.Len() > 0 {
			output.WriteByte('\n')
		}
		output.WriteString(text)
	}
	return output.String()
}

func (unavailableProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
}

type testFactoryOption func(*testFactoryConfig)

type testFactoryConfig struct {
	engineOpening             *EngineOpening
	net                       *state.Net
	scheduler                 scheduler.Scheduler
	workerExecutors           map[string]workers.WorkerExecutor
	workerService             workers.Service
	workerSessions            workersessions.Service
	runtimeConfig             interfaces.RuntimeDefinitionLookup
	workflowContext           *factory_context.FactoryContext
	runtimeMode               interfaces.RuntimeMode
	logger                    logging.Logger
	clock                     factory.Clock
	restoredWorldState        *interfaces.FactoryWorldState
	inlineDispatch            bool
	eventHistory              recordings.RuntimeEventLedger
	submissionHooks           []factory.SubmissionHook
	dispatchRecorder          recordings.DispatchRecorder
	completionRecorder        factory.CompletionRecorder
	petriMutationRecorder     factory.PetriMutationRecorder
	completionDeliveryPlanner factory.CompletionDeliveryPlanner
}

func newTestFactory(opts ...testFactoryOption) (factoryhost.Engine, error) {
	cfg := &testFactoryConfig{runtimeMode: interfaces.RuntimeModeBatch, clock: platformclock.Real{}, logger: logging.NoopLogger{}}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.eventHistory == nil && cfg.net != nil {
		cfg.eventHistory = &recordingfixtures.ScriptedRuntimeLedger{}
	}
	var identity atomic.Int64
	workerService := cfg.workerService
	if workerService == nil {
		workerService = &testWorkstationBoundary{executors: cfg.workerExecutors}
	} else if boundary, ok := workerService.(*testWorkstationBoundary); ok && boundary.executors == nil {
		boundary.executors = cfg.workerExecutors
	}
	workerSessionsService := cfg.workerSessions
	if workerSessionsService == nil {
		workerSessionsService = &fakeWorkerSessionsService{execution: workerService}
	}
	workerAttempts, _ := workerSessionsService.(factory.WorkerAttemptOpener)
	opening := cfg.engineOpening
	if opening == nil {
		opening = NewEngineOpening(nil, unavailableProviderSessions{}, nil, nil, interfaces.WorkPropagationPolicyFunc(func(*interfaces.FactoryWorkstationConfig) interfaces.WorkPropagationMode {
			return interfaces.WorkPropagationModeOutputAsPayload
		}), testRuntimeWorkService{}, func() string { return fmt.Sprintf("work-request-test-id-%d", identity.Add(1)) }, func() string { return fmt.Sprintf("runtime-test-id-%d", identity.Add(1)) }, platformfilesystem.Local{}, nil, dispatchplanningwire.NewOpening(), scheduler.NewEnablementEvaluator(), platformfilesystem.Local{}.ReadFile, testPromptRenderer(workerService), testTemplateFields(workerService), testPrepareSchedules)
	}
	runtime, err := opening.Open(
		context.Background(),
		cfg.net, cfg.scheduler, workerService, workerSessionsService, workerAttempts, cfg.runtimeConfig, nil,
		cfg.workflowContext, "", cfg.runtimeMode, cfg.logger, cfg.clock, platformclock.Real{},
		cfg.inlineDispatch, cfg.eventHistory, "runtime-test-recording-id", "runtime-test-id", nil,
		cfg.restoredWorldState, false,
		nil, nil, cfg.submissionHooks,
		cfg.dispatchRecorder, cfg.completionRecorder, cfg.petriMutationRecorder,
		cfg.completionDeliveryPlanner,
	)
	if err != nil {
		return nil, err
	}
	return runtime, nil
}

func testLegacyRequestFromExecute(
	request workers.ExecuteRequest,
) workers.WorkstationDispatchRequest {
	dispatchID := request.Correlation.DispatchID
	if request.Target.WorkstationName == workers.ProviderInvocationRoute {
		dispatchID = request.Correlation.AttemptID
	}
	dispatch := work.CloneWorkDispatch(request.Input.Dispatch)
	if dispatch.DispatchID == "" {
		dispatch.TransitionID = request.Target.WorkstationName
	}
	dispatch.WorkstationName = firstRuntimeValue(
		dispatch.WorkstationName, request.Target.WorkstationName,
	)
	dispatch.WorkerType = firstRuntimeValue(
		dispatch.WorkerType, request.Target.WorkerType,
	)
	dispatch.DispatchID = dispatchID
	dispatch.Execution.RequestID = request.Correlation.RequestID
	dispatch.Execution.TraceID = request.Correlation.TraceID
	dispatch.Execution.WorkIDs = workInputIDs(request.Input.Work)
	dispatch.InputTokens = workers.InputTokens(workTokens(request.Input.Work)...)
	return workers.WorkstationDispatchRequest{
		WorkstationName: request.Target.WorkstationName,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:                    dispatch,
			WorkerName:                  request.Target.WorkerName,
			WorkerType:                  request.Target.WorkerType,
			WorkstationType:             request.Target.WorkstationName,
			RunnerID:                    request.Target.RunnerID,
			ExecutorProvider:            request.Target.Provider.ID,
			ProjectID:                   request.Correlation.RuntimeID,
			FactorySessionID:            request.Correlation.FactorySessionID,
			RuntimeID:                   request.Correlation.RuntimeID,
			RecordingID:                 request.Input.RecordingID,
			GenerationID:                request.Correlation.GenerationID,
			WorkflowContext:             request.Input.WorkflowContext.Clone(),
			Capabilities:                cloneRuntimeCapabilities(request.Target.Capabilities),
			InputTokens:                 workers.InputTokens(workTokens(request.Input.Work)...),
			ModelOperation:              request.Input.ModelOperation,
			ModelBindings:               workers.CloneResolvedModelOperationBindings(request.Input.ModelBindings),
			Model:                       request.Target.Model.Name,
			ModelProvider:               request.Target.Model.Provider,
			ReasoningEffort:             request.Target.Model.ReasoningEffort,
			Command:                     request.Target.Command,
			Args:                        append([]string(nil), request.Target.Args...),
			FactoryDirectory:            request.Target.FactoryDirectory,
			OutputFormat:                request.Target.Output.Format,
			StopToken:                   request.Target.Output.StopToken,
			DecisionEnvelope:            request.Target.Output.DecisionEnvelope,
			GoalRoutingDecisionEnvelope: request.Target.Output.GoalRoutingDecisionEnvelope,
			SystemPrompt:                request.Target.Prompt.SystemPrompt,
			UserMessage:                 request.Target.Prompt.UserMessage,
			OutputSchema:                request.Target.Prompt.OutputSchema,
			OutputContract:              request.Target.Output.Contract,
			Timeout:                     request.Target.Timeout,
			EnvVars:                     cloneRuntimeStringMap(request.Target.Environment.Vars),
			ProcessEnvironment:          append([]string(nil), request.Target.Environment.ProcessEnvironment...),
			Worktree:                    request.Target.Workspace.Worktree,
			WorkingDirectory:            request.Target.Environment.WorkingDirectory,
			WorkingDirectoryAuthored:    request.Target.Environment.WorkingDirectorySet,
			SkipPermissions:             request.Target.Permissions.SkipPermissions,
			Continuation:                cloneRuntimeContinuation(request.Input.Resume),
		},
	}
}

func testExecuteRequestFromDispatch(
	request workers.WorkstationDispatchRequest,
) workers.ExecuteRequest {
	execution := workers.CloneWorkstationExecutionRequest(request.Execution)
	dispatch := execution.Dispatch
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: execution.FactorySessionID,
			RuntimeID:        execution.RuntimeID,
			GenerationID:     execution.GenerationID,
			DispatchID:       dispatch.DispatchID,
			AttemptID:        dispatch.DispatchID,
			RequestID:        dispatch.Execution.RequestID,
			TraceID:          dispatch.Execution.TraceID,
		},
		Target: workers.ExecutionTarget{
			WorkerName:       execution.WorkerName,
			WorkerType:       execution.WorkerType,
			WorkstationName:  request.WorkstationName,
			RunnerID:         execution.RunnerID,
			ExecutorProvider: execution.ExecutorProvider,
			Capabilities:     cloneRuntimeCapabilities(execution.Capabilities),
			Command:          execution.Command,
			Args:             append([]string(nil), execution.Args...),
			FactoryDirectory: execution.FactoryDirectory,
			Provider: workers.ProviderReference{
				ID:    execution.ModelProvider,
				Alias: execution.ModelProvider,
			},
			Model: workers.ModelReference{
				Name:            execution.Model,
				Provider:        execution.ModelProvider,
				ReasoningEffort: execution.ReasoningEffort,
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
				Vars:                   cloneRuntimeStringMap(execution.EnvVars),
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
			Dispatch:        dispatch,
			RecordingID:     execution.RecordingID,
			ModelBindings:   workers.CloneResolvedModelOperationBindings(execution.ModelBindings),
			ModelOperation:  execution.ModelOperation,
			Resume:          cloneRuntimeContinuation(execution.Continuation),
			WorkflowContext: execution.WorkflowContext.Clone(),
		},
		Attempt: workers.AttemptContext{Number: 1},
	}
}

func testDispatchResultFromExecute(
	request workers.WorkstationDispatchRequest,
	result workers.ExecuteResult,
	executeErr error,
) workers.WorkstationDispatchResult {
	workResult := workers.WorkResult{
		DispatchID:                  request.Execution.Dispatch.DispatchID,
		TransitionID:                request.Execution.Dispatch.TransitionID,
		Outcome:                     workers.OutcomeAccepted,
		Cancellation:                result.Cancellation.Clone(),
		Output:                      testMaterializationPrimaryOutput(result.Output.Primary),
		Feedback:                    result.Output.Feedback,
		SelectedClassificationLabel: result.Output.Classification,
		Continuation:                cloneRuntimeContinuation(result.Continuation),
		Metrics: workers.WorkMetrics{
			Duration:   result.Metrics.Duration,
			Cost:       result.Metrics.Cost,
			RetryCount: result.Metrics.RetryCount,
		},
	}
	terminal := workers.WorkstationDispatchTerminalOutcomeCompleted
	if result.Failure != nil {
		workResult.Error = result.Failure.Message
		workResult.FailureMetadata = &workers.WorkFailureMetadata{
			Family: result.Failure.Family,
			Type:   result.Failure.Type,
		}
	}
	if executeErr != nil {
		if errors.Is(executeErr, workers.ErrWorkstationDispatchCanceled) || result.Outcome == workers.ExecutionOutcomeCanceled {
			terminal = workers.WorkstationDispatchTerminalOutcomeCanceled
			workResult.Outcome = workers.OutcomeCanceled
			if workResult.Cancellation == nil {
				workResult.Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}
			}
			workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
		} else {
			terminal = workers.WorkstationDispatchTerminalOutcomeFailed
			if workResult.Error == "" {
				workResult.Error = executeErr.Error()
			}
		}
	}
	switch result.Outcome {
	case workers.ExecutionOutcomeContinue:
		workResult.Outcome = workers.OutcomeContinue
	case workers.ExecutionOutcomeRejected:
		workResult.Outcome = workers.OutcomeRejected
	case workers.ExecutionOutcomeFailed:
		if terminal != workers.WorkstationDispatchTerminalOutcomeCanceled {
			workResult.Outcome = workers.OutcomeFailed
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
	if terminal == workers.WorkstationDispatchTerminalOutcomeCanceled {
		workResult.Outcome = workers.OutcomeCanceled
	}
	output := result.Output.Clone()
	return workers.WorkstationDispatchResult{
		DispatchID:      request.Execution.Dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: terminal,
		Result:          workResult,
		ProposedOutput:  &output,
	}
}

func testExecuteResultFromDispatchResult(
	request workers.ExecuteRequest,
	result workers.WorkstationDispatchResult,
) workers.ExecuteResult {
	executeResult := workers.ExecuteResult{
		Correlation:  request.Correlation,
		Outcome:      executeOutcomeFromWorkResult(result.Result),
		Cancellation: result.Result.Cancellation.Clone(),
		Failure:      executeFailureFromWorkResult(result.Result),
		Output:       workers.ProposedOutputFromLegacyWorkResult(result.Result),
		Continuation: cloneRuntimeContinuation(result.Result.Continuation),
	}
	if result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCanceled {
		executeResult.Outcome = workers.ExecutionOutcomeCanceled
	}
	return executeResult
}

func executeFailureFromWorkResult(result workers.WorkResult) *workers.ExecutionFailure {
	if result.FailureMetadata == nil && strings.TrimSpace(result.Error) == "" {
		return nil
	}
	failure := &workers.ExecutionFailure{Message: result.Error}
	if result.FailureMetadata != nil {
		failure.Family = result.FailureMetadata.Family
		failure.Type = result.FailureMetadata.Type
		failure.RetryHint = workers.FailureDecisionFromMetadata(result.FailureMetadata).Retryable
	}
	failure.ProviderFailureKind = result.ProviderFailureKind
	failure.ProviderContinuationFailureKind = result.ProviderContinuationFailureKind
	failure.ProviderContinuationOutcome = result.ProviderContinuationOutcome
	return failure
}

func executeOutcomeFromWorkResult(result workers.WorkResult) workers.ExecutionOutcome {
	switch result.Outcome {
	case workers.OutcomeContinue:
		return workers.ExecutionOutcomeContinue
	case workers.OutcomeRejected:
		return workers.ExecutionOutcomeRejected
	case workers.OutcomeFailed:
		return workers.ExecutionOutcomeFailed
	case workers.OutcomeCanceled:
		return workers.ExecutionOutcomeCanceled
	default:
		return workers.ExecutionOutcomeAccepted
	}
}

func workInputIDs(inputs []workers.WorkInput) []string {
	ids := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if input.WorkID != "" {
			ids = append(ids, input.WorkID)
		}
	}
	return ids
}

func workTokens(inputs []workers.WorkInput) []workers.Token {
	tokens := make([]workers.Token, 0, len(inputs))
	for _, input := range inputs {
		tokens = append(tokens, workers.Token{Color: workers.Color{
			WorkID: input.WorkID, WorkTypeID: input.WorkTypeID, RequestID: input.RequestID,
			DataType: workers.DataTypeWork, TraceID: input.Lineage.TraceID, ParentID: input.Lineage.ParentWorkID,
			Content: work.CloneWorkContentParts(input.Content), Tags: cloneRuntimeStringMap(input.Tags),
			Relations: append([]work.Relation(nil), input.Relations...),
		}})
	}
	return tokens
}

func newTestFactoryWithScriptedLedger(
	opts ...testFactoryOption,
) (factoryhost.Engine, *recordingfixtures.ScriptedRuntimeLedger, error) {
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	opts = append(opts, withFactoryEventHistory(ledger))
	runtime, err := newTestFactory(opts...)
	return runtime, ledger, err
}

func withNet(net *state.Net) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.net = net }
}

func withScheduler(value scheduler.Scheduler) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.scheduler = value }
}

func withWorkerExecutor(workerType string, executor workers.WorkerExecutor) testFactoryOption {
	return func(cfg *testFactoryConfig) {
		if cfg.workerExecutors == nil {
			cfg.workerExecutors = make(map[string]workers.WorkerExecutor)
		}
		cfg.workerExecutors[workerType] = executor
	}
}

func withWorkerService(service workers.Service) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.workerService = service }
}

func withWorkerSessions(service workersessions.Service) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.workerSessions = service }
}

// fakeWorkerSessionsService is the default test double for the W4 Runtime
// dispatch cutover seam: Start hands the resolved request straight to the
// configured Workers execution boundary and reports the raw result, mirroring
// the shape worker_sessions.Service.Start returns once it reaches handoff.
// worker_sessions' own state-machine/Events behavior is exercised in its own
// package tests; Runtime's tests only need this seam's integration contract.
func withRuntimeConfig(value interfaces.RuntimeDefinitionLookup) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.runtimeConfig = value }
}

func withWorkflowContext(value *factory_context.FactoryContext) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.workflowContext = value }
}

func withServiceMode() testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.runtimeMode = interfaces.RuntimeModeService }
}

func withLogger(value logging.Logger) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.logger = value }
}

func withClock(value factory.Clock) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.clock = value }
}

func withRestoredWorldState(value *interfaces.FactoryWorldState) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.restoredWorldState = value }
}

func withInlineDispatch() testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.inlineDispatch = true }
}

func withFactoryEventHistory(value recordings.RuntimeEventLedger) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.eventHistory = value }
}

func withSubmissionHook(value factory.SubmissionHook) testFactoryOption {
	return func(cfg *testFactoryConfig) {
		if value != nil {
			cfg.submissionHooks = append(cfg.submissionHooks, value)
		}
	}
}

func withDispatchRecorder(value recordings.DispatchRecorder) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.dispatchRecorder = value }
}

func withCompletionRecorder(value factory.CompletionRecorder) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.completionRecorder = value }
}

func withPetriMutationRecorder(value factory.PetriMutationRecorder) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.petriMutationRecorder = value }
}

func withCompletionDeliveryPlanner(value factory.CompletionDeliveryPlanner) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.completionDeliveryPlanner = value }
}

type passExecutor struct{}

func (e *passExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "done",
	}, nil
}

type panicExecutor struct {
	message string
}

func (e *panicExecutor) Execute(_ context.Context, _ work.WorkDispatch) (workerexecution.WorkResult, error) {
	panic(e.message)
}

type asyncRecordingExecutor struct {
	started chan work.WorkDispatch
	release chan struct{}
}

func (e *asyncRecordingExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.started <- dispatch
	<-e.release
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "async-executor-output",
	}, nil
}

type acceptedNoOutputExecutor struct{}

func (*acceptedNoOutputExecutor) Execute(
	_ context.Context,
	dispatch work.WorkDispatch,
) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}, nil
}

type blockingExecutor struct {
	started   chan struct{}
	release   chan struct{}
	startOnce sync.Once
}

func (e *blockingExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.startOnce.Do(func() { close(e.started) })
	<-e.release
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "done",
	}, nil
}

type safeDiagnosticsBoundaryExecutor struct{}

func (e *safeDiagnosticsBoundaryExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	workID := safeBoundaryWorkID(dispatch)
	switch workID {
	case "work-safe-success":
		return safeBoundaryResult(dispatch, workID, workerexecution.OutcomeAccepted, "", nil, &providers.SessionMetadata{
			Provider: "codex",
			Kind:     "response_id",
			ID:       "resp-safe-success",
		}, "1"), nil
	case "work-safe-failure":
		return safeBoundaryResult(dispatch, workID, workerexecution.OutcomeFailed, "provider timed out", &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeTimeout,
		}, &providers.SessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-safe-failure",
		}, "2"), nil
	case "work-safe-windows-process-failure":
		return safeBoundaryResult(dispatch, workID, workerexecution.OutcomeFailed, "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)", &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeInternalServerError,
		}, &providers.SessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-safe-windows-4294967295",
		}, "2"), nil
	default:
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       "done",
		}, nil
	}
}

type fixedCompletionDeliveryPlanner struct {
	tick          int
	plannedResult workerexecution.WorkResult
}

func (p fixedCompletionDeliveryPlanner) DeliveryTickForDispatch(work.WorkDispatch) (int, bool, error) {
	return p.tick, true, nil
}

func (p fixedCompletionDeliveryPlanner) PlannedResultForDispatch(dispatch work.WorkDispatch) (workerexecution.WorkResult, bool, error) {
	if p.plannedResult.DispatchID == "" && p.plannedResult.TransitionID == "" && p.plannedResult.Output == "" && p.plannedResult.Outcome == "" {
		return workerexecution.WorkResult{}, false, nil
	}
	result := p.plannedResult
	result.DispatchID = dispatch.DispatchID
	result.TransitionID = dispatch.TransitionID
	return result, true, nil
}

func submitWorkRequests(ctx context.Context, f factoryhost.Engine, reqs []work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
	return f.SubmitWorkRequest(ctx, work.WorkRequestFromSubmitRequests(reqs))
}

type runtimeProjectionConfig = runtimefixtures.RuntimeDefinitionLookupFixture
type runtimeSchedulerConfig = *runtimefixtures.RuntimeDefinitionLookupFixture

type runtimeAwareScheduler struct {
	configured interfaces.RuntimeWorkstationLookup
}

func (s *runtimeAwareScheduler) Select([]interfaces.EnabledTransition, *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision {
	return nil
}

func (s *runtimeAwareScheduler) SetRuntimeConfig(runtimeConfig interfaces.RuntimeWorkstationLookup) {
	s.configured = runtimeConfig
}

type generatedBatchHook struct {
	batch   work.GeneratedSubmissionBatch
	emitted bool
}

func (h *generatedBatchHook) Name() string {
	return "generated-batch-test"
}

func (h *generatedBatchHook) Priority() int {
	return 1
}

func (h *generatedBatchHook) OnTick(context.Context, interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
	if h.emitted {
		return interfaces.SubmissionHookResult{}, nil
	}
	h.emitted = true
	return interfaces.SubmissionHookResult{
		GeneratedBatches: []work.GeneratedSubmissionBatch{h.batch},
	}, nil
}

func testPromptRenderer(service workers.Service) runtimePromptRenderer {
	renderer, _ := service.(runtimePromptRenderer)
	return renderer
}
func testTemplateFields(service workers.Service) runtimeTemplateFieldResolver {
	fields, _ := service.(runtimeTemplateFieldResolver)
	return fields
}

func testPrepareSchedules(context.Context, automations.InvocationScheduleRequest) (automations.PreparedInvocationSchedules, error) {
	return automations.PreparedInvocationSchedules{}, nil
}

func waitForAggregateSnapshot(
	t *testing.T,
	f factoryhost.Engine,
	match func(*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	var last *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		last = snap
		if match(snap) {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	if last == nil {
		t.Fatal("timed out waiting for aggregate snapshot; no snapshot captured")
	}
	t.Fatalf("timed out waiting for aggregate snapshot; last status=%q in_flight=%d tick=%d",
		last.RuntimeStatus,
		last.InFlightCount,
		last.TickCount,
	)
	return nil
}

func (h *serviceModeRunHarness) stop() {
	h.t.Helper()
	h.cancel()
	select {
	case err := <-h.errCh:
		if err != nil {
			h.t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		h.t.Fatal("timed out waiting for service-mode runtime to stop after cancellation")
	}
}

func waitForBlockingWorkerStart(t *testing.T, executor *blockingExecutor, errCh <-chan error) {
	t.Helper()
	select {
	case <-executor.started:
	case err := <-errCh:
		t.Fatalf("Run returned before worker started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker to start")
	}
}

func pollPausedSnapshot(
	t *testing.T,
	f factoryhost.Engine,
	duration time.Duration,
	assertFn func(*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]),
) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
		}
		assertFn(snap)
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForBufferedResult(t *testing.T, written <-chan struct{}) {
	t.Helper()
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker result to enter runtime buffer")
	}
}

func waitForFactoryState(t *testing.T, f factoryhost.Engine, want interfaces.FactoryState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if snap.FactoryState == string(want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	t.Fatalf("factory state = %q, want %q before timeout", snap.FactoryState, want)
}

func waitForWorkAtPlace(t *testing.T, f factoryhost.Engine, placeID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if hasWorkAtPlace(snap, placeID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for work at %s", placeID)
}

func waitForAggregateSnapshotWithTimeout(
	t *testing.T,
	f factoryhost.Engine,
	timeout time.Duration,
	match func(*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		last = snap
		if match(snap) {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	if last == nil {
		t.Fatal("timed out waiting for aggregate snapshot; no snapshot captured")
	}
	t.Fatalf("timed out waiting for aggregate snapshot after %s; last status=%q in_flight=%d tick=%d",
		timeout,
		last.RuntimeStatus,
		last.InFlightCount,
		last.TickCount,
	)
	return nil
}
