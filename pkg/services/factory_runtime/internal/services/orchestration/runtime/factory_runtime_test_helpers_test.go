// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
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
	cfg := &testFactoryConfig{runtimeMode: interfaces.RuntimeModeBatch, clock: platformclock.Real{}}
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
	runtime, err := New(
		cfg.net, cfg.scheduler, workerService, workerSessionsService, cfg.runtimeConfig, nil, nil,
		cfg.workflowContext, cfg.runtimeMode, cfg.logger, cfg.clock,
		cfg.inlineDispatch, cfg.eventHistory, "runtime-test-recording-id", "runtime-test-id", nil,
		cfg.restoredWorldState, false, unavailableProviderSessions{},
		nil, nil, cfg.submissionHooks,
		cfg.dispatchRecorder, cfg.completionRecorder, cfg.petriMutationRecorder,
		cfg.completionDeliveryPlanner,
		nil,
		nil,
		interfaces.WorkPropagationPolicyFunc(func(
			*interfaces.FactoryWorkstationConfig,
		) interfaces.WorkPropagationMode {
			return interfaces.WorkPropagationModeOutputAsPayload
		}),
		testRuntimeWorkService{},
		func() string { return fmt.Sprintf("work-request-test-id-%d", identity.Add(1)) },
		func() string { return fmt.Sprintf("runtime-test-id-%d", identity.Add(1)) },
		platformfilesystem.Local{},
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
type fakeWorkerSessionsService struct {
	execution workers.Service
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
	return workersessions.Observation{}, nil
}

func (s *fakeWorkerSessionsService) GetObservationByWorkerSessionID(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, nil
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
				Outcome:      workerexecution.OutcomeFailed,
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

func buildSimpleNet() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "done", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}

	places := make(map[string]*petri.Place)
	for _, p := range wt.GeneratePlaces() {
		places[p.ID] = p
	}

	transition := &petri.Transition{
		ID:         "t-process",
		Name:       "Process",
		Type:       petri.TransitionNormal,
		WorkerType: "mock",
		InputArcs: []petri.Arc{{
			ID:          "a-in",
			Name:        "input",
			PlaceID:     "task:init",
			Direction:   petri.ArcInput,
			Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
		OutputArcs: []petri.Arc{{
			ID:          "a-out",
			Name:        "output",
			PlaceID:     "task:done",
			Direction:   petri.ArcOutput,
			Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
	}

	return &state.Net{
		ID:          "test-net",
		Places:      places,
		Transitions: map[string]*petri.Transition{"t-process": transition},
		WorkTypes:   map[string]*state.WorkType{"task": wt},
		Resources:   make(map[string]*state.ResourceDef),
	}
}

func buildSimpleNetWithFailureArc() *state.Net {
	n := buildSimpleNet()
	n.Transitions["t-process"].FailureArcs = []petri.Arc{{
		ID:        "a-failed",
		Name:      "failed",
		PlaceID:   "task:failed",
		Direction: petri.ArcOutput,
	}}
	return n
}

func newPassingInlineRuntime(t *testing.T) factoryhost.Engine {
	t.Helper()
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f
}

func newPassingInlineRuntimeWithLedger(
	t *testing.T,
) (factoryhost.Engine, *recordingfixtures.ScriptedRuntimeLedger) {
	t.Helper()
	f, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f, ledger
}

func tickableFactory(t *testing.T, f factoryhost.Engine) TickableFactory {
	t.Helper()
	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}
	return tickable
}

func runtimeGeneratedEvents(t *testing.T, f factoryhost.Engine) []factoryapi.FactoryEvent {
	t.Helper()
	events, err := f.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	mapped := make([]factoryapi.FactoryEvent, len(events))
	for index, event := range events {
		mapped[index] = runtimeGeneratedFactoryEvent(t, event)
	}
	return mapped
}

func runtimeGeneratedFactoryEvent(t testing.TB, event interfaces.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()
	var mapped factoryapi.FactoryEvent
	if err := event.Decode(&mapped); err != nil {
		t.Fatalf("map Factory event %q: %v", event.Id, err)
	}
	return mapped
}

func markingContainsWorkAtPlace(marking *petri.MarkingSnapshot, workID string, placeID string) bool {
	if marking == nil {
		return false
	}
	for _, tokenID := range marking.PlaceTokens[placeID] {
		token := marking.Tokens[tokenID]
		if token != nil && token.Color.WorkID == workID {
			return true
		}
	}
	return false
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

func safeBoundaryWorkID(dispatch work.WorkDispatch) string {
	for _, token := range workers.WorkDispatchInputTokens(dispatch) {
		if token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		if token.Color.WorkID != "" {
			return token.Color.WorkID
		}
		if token.ID != "" {
			return token.ID
		}
	}
	for _, workID := range dispatch.Execution.WorkIDs {
		if workID != "" {
			return workID
		}
	}
	return ""
}

func safeBoundaryResult(
	dispatch work.WorkDispatch,
	workID string,
	outcome workerexecution.WorkOutcome,
	errText string,
	providerFailure *workerexecution.WorkFailureMetadata,
	providerSession *providers.SessionMetadata,
	retryCount string,
) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         outcome,
		Output:          "safe boundary output for " + workID,
		Error:           errText,
		FailureMetadata: providerFailure,
		Continuation:    (providerSession).ContinuationRef(),
		Diagnostics: &workerexecution.WorkDiagnostics{
			RenderedPrompt: &workerexecution.RenderedPromptDiagnostic{
				SystemPromptHash: "system-hash-" + workID,
				UserMessageHash:  "user-hash-" + workID,
				Variables: map[string]string{
					"prompt_source":  "factory-renderer",
					"work_type_name": "task",
					"system_prompt":  "raw rendered system prompt must stay private",
					"user_message":   "raw rendered user message must stay private",
					"stdin":          "raw rendered stdin must stay private",
					"env":            "raw rendered environment must stay private",
				},
			},
			Provider: &workerexecution.ProviderDiagnostic{
				Provider: "codex",
				Model:    "gpt-5.4",
				RequestMetadata: map[string]string{
					"prompt_source":      "provider-renderer",
					"worker_type":        "mock",
					"working_directory":  "/workspace/" + workID,
					"worktree":           "/workspace/" + workID + "/.worktree",
					"system_prompt_body": "raw prompt body must stay private",
					"stdin_payload":      "raw stdin payload must stay private",
					"env_secret":         "raw env secret must stay private",
				},
				ResponseMetadata: map[string]string{
					"provider_session_id": providerSession.ID,
					"retry_count":         retryCount,
					"system_prompt_body":  "raw response prompt body must stay private",
					"stdin_payload":       "raw response stdin payload must stay private",
					"env_secret":          "raw response env secret must stay private",
				},
			},
			Command: &workerexecution.CommandDiagnostic{
				Command: "echo",
				Stdin:   "raw command stdin must stay private",
				Env: map[string]string{
					"AGENT_FACTORY_AUTH_TOKEN": "raw environment value must stay private",
				},
			},
			Panic: &workerexecution.PanicDiagnostic{Stack: "panic stack should not be stored"},
		},
	}
}

type serviceModeRunHarness struct {
	t       *testing.T
	Factory factoryhost.Engine
	cancel  context.CancelFunc
	errCh   chan error
}

func startServiceModeRunHarness(t *testing.T, opts ...testFactoryOption) *serviceModeRunHarness {
	t.Helper()

	f, err := newTestFactory(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(ctx)
	}()

	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)
	return &serviceModeRunHarness{t: t, Factory: f, cancel: cancel, errCh: errCh}
}

func (h *serviceModeRunHarness) pauseAndWait() {
	h.t.Helper()
	if err := h.Factory.Pause(context.Background()); err != nil {
		h.t.Fatalf("Pause: %v", err)
	}
	waitForFactoryState(h.t, h.Factory, interfaces.FactoryStatePaused, time.Second)
}

func (h *serviceModeRunHarness) resumeAndWait() {
	h.t.Helper()
	if err := h.Factory.Resume(context.Background()); err != nil {
		h.t.Fatalf("Resume: %v", err)
	}
	waitForFactoryState(h.t, h.Factory, interfaces.FactoryStateRunning, time.Second)
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

func submitPausedBufferTask(t *testing.T, f factoryhost.Engine, requestID, traceID string) {
	t.Helper()
	result, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		RequestID:  requestID,
		WorkTypeID: "task",
		TraceID:    traceID,
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("submit accepted = false, want true")
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

func assertPausedSubmissionNotApplied(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	pollPausedSnapshot(t, f, 300*time.Millisecond, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
		if snap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("factory state = %q, want PAUSED", snap.FactoryState)
		}
		if hasWorkAtPlace(snap, "task:init") || hasWorkAtPlace(snap, "task:done") {
			t.Fatalf("paused submission applied to marking = %#v", snap.Marking.Tokens)
		}
		if snap.InFlightCount > 0 || len(snap.Dispatches) > 0 {
			t.Fatalf("running dispatches while paused inFlight=%d dispatches=%d", snap.InFlightCount, len(snap.Dispatches))
		}
	})
}

func observeNextBufferedResult(t *testing.T, f factoryhost.Engine) <-chan struct{} {
	t.Helper()
	impl, ok := f.(*factoryImpl)
	if !ok || impl.dispatchFlow == nil {
		t.Fatal("test factory does not expose a canonical dispatch result hook")
	}
	written := make(chan struct{})
	var once sync.Once
	notify := func() { once.Do(func() { close(written) }) }
	impl.dispatchFlow.SetOnBufferedResult(notify)
	return written
}

func waitForBufferedResult(t *testing.T, written <-chan struct{}) {
	t.Helper()
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker result to enter runtime buffer")
	}
}

func assertPausedWorkerResultBuffered(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
	}
	if snap.FactoryState != string(interfaces.FactoryStatePaused) {
		t.Fatalf("factory state = %q, want PAUSED", snap.FactoryState)
	}
	if hasWorkAtPlace(snap, "task:done") {
		t.Fatal("worker result applied while paused")
	}
	if snap.InFlightCount == 0 {
		t.Fatalf("dispatch completed while paused inFlight=%d", snap.InFlightCount)
	}
}

func assertPausedSubmissionNotDone(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
	}
	if hasWorkAtPlace(snap, "task:done") {
		t.Fatal("buffered submission applied while paused")
	}
}

func assertPausedWorkerResultNotDone(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	assertPausedWorkerResultBuffered(t, f)
}

func assertTaskDoneOnce(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if count := countTokensAtPlace(snap, "task:done"); count != 1 {
		t.Fatalf("task:done token count = %d, want 1", count)
	}
}

func assertNoInFlightDispatches(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if snap.InFlightCount != 0 {
		t.Fatalf("inFlightCount = %d, want 0 after resume", snap.InFlightCount)
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

func hasWorkAtPlace(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], placeID string) bool {
	for _, token := range snap.Marking.Tokens {
		if token.PlaceID == placeID {
			return true
		}
	}
	return false
}

func countTokensAtPlace(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], placeID string) int {
	count := 0
	for _, token := range snap.Marking.Tokens {
		if token.PlaceID == placeID {
			count++
		}
	}
	return count
}

func submitTaskWithWorkID(t *testing.T, f factoryhost.Engine, workID, traceID string) {
	t.Helper()
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest for %q: %v", workID, err)
	}
}

func assertWorkNotAtDonePlace(t *testing.T, f factoryhost.Engine, workID string) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if markingContainsWorkAtPlace(&snap.Marking, workID, "task:done") {
		t.Fatalf("marking = %#v, want work %q to remain unprocessed before resume", snap.Marking.Tokens, workID)
	}
}

func assertWorksNotAtDonePlace(t *testing.T, f factoryhost.Engine, workIDs []string) {
	t.Helper()
	for _, workID := range workIDs {
		assertWorkNotAtDonePlace(t, f, workID)
	}
}

func waitForWorkDoneAfterResume(t *testing.T, f factoryhost.Engine, workID string) {
	t.Helper()
	waitForAggregateSnapshotWithTimeout(t, f, 2*time.Second, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return markingContainsWorkAtPlace(&snap.Marking, workID, "task:done")
	})
}

func waitForQuiescentWorksAtDone(t *testing.T, f factoryhost.Engine, workIDs []string) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	return waitForAggregateSnapshotWithTimeout(t, f, 5*time.Second, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return allWorksAtDonePlace(&snap.Marking, workIDs) && snap.InFlightCount == 0
	})
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

func assertDispatchOrder(t *testing.T, history []interfaces.CompletedDispatch, wantWorkIDs []string) {
	t.Helper()
	gotOrder := workIDsFromDispatchHistory(history)
	for i, wantWorkID := range wantWorkIDs {
		if gotOrder[i] != wantWorkID {
			t.Fatalf("dispatch history order = %v, want %v", gotOrder, wantWorkIDs)
		}
	}
}

func resumeFactory(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	if err := f.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
}
func TestRuntimeModelRecordingEnabledRequiresRuntimeOwnerAndRecorder(t *testing.T) {
	t.Parallel()

	ledger := &modelRecordingLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	cases := []struct {
		name string
		cfg  *runtimeConfig
		want bool
	}{
		{name: "nil config", cfg: nil, want: false},
		{name: "missing ledger", cfg: &runtimeConfig{executeService: modelRecordingExecuteService{owns: true}}, want: false},
		{name: "missing runtime owner", cfg: &runtimeConfig{eventHistory: ledger, executeService: modelRecordingExecuteService{}}, want: false},
		{name: "runtime owner disabled", cfg: &runtimeConfig{eventHistory: ledger, executeService: modelRecordingExecuteService{}}, want: false},
		{
			name: "ledger does not expose worker recorder",
			cfg: &runtimeConfig{
				eventHistory:   runtimeLedgerWithoutWorkerRecorder{RuntimeLedger: ledger.ScriptedRuntimeLedger},
				executeService: modelRecordingExecuteService{owns: true},
			},
			want: false,
		},
		{name: "runtime owner and recorder", cfg: &runtimeConfig{eventHistory: ledger, executeService: modelRecordingExecuteService{owns: true}}, want: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeModelRecordingEnabled(test.cfg); got != test.want {
				t.Fatalf("runtimeModelRecordingEnabled() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestPrepareDetachedModelRecordingRecordsDetachedRequestAndResponse(t *testing.T) {
	t.Parallel()

	ledger := &modelRecordingLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	previousCalled := false
	previousTerminalCalled := false
	cfg := &runtimeConfig{
		executeService: modelRecordingExecuteService{owns: true},
		eventHistory:   ledger,
		clock:          testRuntimeClock{},
	}
	request := modelRecordingRequest()
	prepared := prepareDetachedModelRecording(cfg, func(context.Context, *workers.ExecuteRequest) (attemptTerminalFunc, error) {
		previousCalled = true
		return func(context.Context, workers.ExecuteRequest, workers.ExecuteResult, error) {
			previousTerminalCalled = true
		}, nil
	})
	terminal, err := prepared(context.Background(), &request)
	if err != nil {
		t.Fatalf("prepared() error = %v", err)
	}
	if !previousCalled {
		t.Fatal("previous preparation was not called")
	}
	request.Input.PreparedRequestObserver(request)
	assertDetachedModelRequestEvent(t, ledger.events)
	terminal(context.Background(), request, workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "model output",
		}}},
		Continuation: &workers.ProviderContinuationRef{Provider: "provider-1", ProviderSessionID: "session-1"},
		Diagnostics: &workers.SafeDiagnostics{Provider: &workers.SafeProviderDiagnostic{
			Provider:         "agy",
			ResponseMetadata: map[string]string{"input_tokens": "3"},
		}},
		Metrics: workers.ExecutionMetrics{Duration: 1500 * time.Millisecond},
	}, nil)
	if !previousTerminalCalled {
		t.Fatal("previous terminal hook was not called")
	}
	assertDetachedModelResponseEvent(t, ledger.events)
	request.Input.ModelBindings[0].Content[0].Text = "mutated"
	if (*ledger.events[1].Response.Bindings)[0].Content[0].Text != "binding" {
		t.Fatal("recorded response bindings alias the Execute request")
	}
}

func assertDetachedModelRequestEvent(t *testing.T, events []workers.ModelEvent) {
	t.Helper()
	if len(events) != 1 {
		t.Fatalf("request events = %#v, want one model request", events)
	}
	recorded := events[0]
	if recorded.Kind != workers.ModelEventKindRequest {
		t.Fatalf("request event kind = %q, want request", recorded.Kind)
	}
	assertDetachedModelRequestIdentity(t, recorded)
	assertDetachedModelRequestPayload(t, recorded.Request)
}

func assertDetachedModelRequestIdentity(t *testing.T, recorded workers.ModelEvent) {
	t.Helper()
	if recorded.ID != "factory-event/model-request/dispatch-1/model-request/1" || recorded.Tick != 7 {
		t.Fatalf("recorded request identity = %#v, want detached correlation", recorded)
	}
	if recorded.RequestID != "request-1" || len(recorded.TraceIDs) != 1 || recorded.TraceIDs[0] != "trace-1" {
		t.Fatalf("recorded request correlation = %#v, want detached correlation", recorded)
	}
}

func assertDetachedModelRequestPayload(t *testing.T, request *workers.ModelRequestEventPayload) {
	t.Helper()
	if request == nil {
		t.Fatal("recorded request payload is nil")
	}
	if request.Operation != "summarize" || request.Worker != "worker-1" || request.Model != "model-1" || request.ProviderLocality != "remote" {
		t.Fatalf("recorded request payload = %#v, want resolved model fields", request)
	}
	if request.WorkingDirectory == nil || *request.WorkingDirectory != "/workspace" {
		t.Fatalf("recorded request working directory = %#v, want /workspace", request.WorkingDirectory)
	}
	if request.Worktree == nil || *request.Worktree != "feature-1" {
		t.Fatalf("recorded request worktree = %#v, want feature-1", request.Worktree)
	}
}

func assertDetachedModelResponseEvent(t *testing.T, events []workers.ModelEvent) {
	t.Helper()
	if len(events) != 2 {
		t.Fatalf("response events = %#v, want one model response", events)
	}
	if events[1].Kind != workers.ModelEventKindResponse {
		t.Fatalf("response event kind = %q, want response", events[1].Kind)
	}
	response := events[1].Response
	if response == nil {
		t.Fatal("recorded response is nil")
	}
	assertDetachedModelResponseMetadata(t, response)
	assertDetachedModelResponseContent(t, response)
}

func assertDetachedModelResponseMetadata(t *testing.T, response *workers.ModelResponseEventPayload) {
	t.Helper()
	if response.Outcome != workers.InferenceOutcomeSucceeded || response.ModelRequestID != "dispatch-1/model-request/1" || response.DurationMillis != 1500 {
		t.Fatalf("recorded response = %#v, want successful detached model response", response)
	}
	providerSession := (response.Continuation).SessionMetadata()
	if providerSession == nil || providerSession.ID != "session-1" {
		t.Fatalf("recorded provider session = %#v, want session-1", providerSession)
	}
}

func assertDetachedModelResponseContent(t *testing.T, response *workers.ModelResponseEventPayload) {
	t.Helper()
	if response.OutputContent == nil || len(*response.OutputContent) != 1 || (*response.OutputContent)[0].Text != "model output" {
		t.Fatalf("recorded response output = %#v, want detached content", response.OutputContent)
	}
	if response.Bindings == nil || len(*response.Bindings) != 1 || (*response.Bindings)[0].Slot != "summary" {
		t.Fatalf("recorded response bindings = %#v, want detached bindings", response.Bindings)
	}
	diagnostics, err := workers.SafeWorkDiagnosticsFromEventPayload(response.Diagnostics)
	if err != nil {
		t.Fatalf("decode recorded response diagnostics: %v", err)
	}
	if diagnostics == nil || diagnostics.Provider == nil || diagnostics.Provider.ResponseMetadata["input_tokens"] != "3" {
		t.Fatalf("recorded response diagnostics = %#v, want public provider metadata", diagnostics)
	}
}

func TestDetachedModelResponseContentDecodesStructuredWorkerText(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        "C:/tmp/factory-work.wav",
		ContentType: "audio/wav",
	}})
	if err != nil {
		t.Fatalf("marshal structured worker output: %v", err)
	}
	got := detachedModelResponseContent([]work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: string(raw),
	}})
	if len(got) != 1 || got[0].Type != work.WorkContentPartTypeAudio || got[0].File != "C:/tmp/factory-work.wav" || got[0].ContentType != "audio/wav" {
		t.Fatalf("decoded detached response content = %#v, want one audio part", got)
	}

	plain := detachedModelResponseContent([]work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "plain provider response",
	}})
	if len(plain) != 1 || plain[0].Type != work.WorkContentPartTypeText || plain[0].Text != "plain provider response" {
		t.Fatalf("plain detached response content = %#v, want unchanged text", plain)
	}
}

func TestRuntimeModelResponseRecordingPreservesProviderSuccessForOutputContractFailure(t *testing.T) {
	t.Parallel()

	ledger := &modelRecordingLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	cfg := &runtimeConfig{
		executeService: modelRecordingExecuteService{owns: true},
		eventHistory:   ledger,
		clock:          testRuntimeClock{},
	}
	recordDetachedModelResponse(cfg, modelRecordingRequest(), workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypePermanentBadRequest,
			Message: "output contract failed",
		},
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "provider response",
		}}},
	}, nil)

	if len(ledger.events) != 1 || ledger.events[0].Response == nil {
		t.Fatalf("recorded events = %#v, want one model response", ledger.events)
	}
	response := ledger.events[0].Response
	if response.Outcome != workers.InferenceOutcomeSucceeded {
		t.Fatalf("provider response outcome = %q, want SUCCEEDED", response.Outcome)
	}
	if response.OutputContent == nil || len(*response.OutputContent) != 1 || (*response.OutputContent)[0].Text != "provider response" {
		t.Fatalf("provider response output = %#v, want raw provider response", response.OutputContent)
	}
}

func TestRuntimeModelResponseRecordingClassifiesFailuresAndContinuationFallbacks(t *testing.T) {
	t.Parallel()

	ledger := &modelRecordingLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	cfg := &runtimeConfig{
		executeService: modelRecordingExecuteService{owns: true},
		eventHistory:   ledger,
		clock:          testRuntimeClock{},
	}
	request := modelRecordingRequest()
	request.Target.Provider.ID = "antigravity"
	request.Attempt.Number = 2
	request.Target.WorkerName = ""
	request.Target.WorkerType = "worker-type"
	request.Correlation.TraceID = ""
	request.Input.Dispatch.Execution.CurrentTick = 9

	recordDetachedModelResponse(cfg, request, workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type: workers.WorkFailureTypeTimeout, Message: "timeout",
			Detail: &workers.FailureDetail{Reason: workers.WorkFailureTypeTimeout, Message: "safe timeout"},
		},
	}, nil)
	recordDetachedModelResponse(cfg, request, workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeCanceled,
		Failure: &workers.ExecutionFailure{Type: workers.WorkFailureTypeUnknown, Message: "cancelled"},
	}, nil)
	recordDetachedModelResponse(cfg, request, workers.ExecuteResult{}, errors.New("transport failed"))

	if len(ledger.events) != 3 {
		t.Fatalf("failure events = %#v, want three responses", ledger.events)
	}
	assertModelFailureEvents(t, ledger.events)
	if got := providerSessionFromExecuteResult(request, workers.ExecuteResult{
		Continuation: &workers.ProviderContinuationRef{Provider: "provider-2", ExternalRef: "external-2"},
	}); got == nil || got.ID != "external-2" || got.Provider != "provider-2" {
		t.Fatalf("external continuation session = %#v, want external reference", got)
	}
}

func assertModelFailureEvents(t *testing.T, events []workers.ModelEvent) {
	t.Helper()
	assertModelFailureProviderSessions(t, events)
	if got := events[0].Response.FailureDetail; got == nil || got.Message != "safe timeout" {
		t.Fatalf("detailed failure = %#v, want cloned safe detail", got)
	}
	if got := events[1].Response.FailureDetail; got == nil || got.Reason != workers.WorkFailureTypeUnknown || got.Message != "cancelled" {
		t.Fatalf("fallback failure = %#v, want execution failure fields", got)
	}
	if got := events[2].Response.FailureDetail; got == nil || got.Reason != workers.WorkFailureTypeUnknown {
		t.Fatalf("error failure = %#v, want unknown detail", got)
	}
	for _, event := range events {
		if event.Response == nil || event.Response.Outcome != workers.InferenceOutcomeFailed || event.Tick != 9 {
			t.Fatalf("failure response = %#v, want failed response at current tick", event.Response)
		}
	}
}

func assertModelFailureProviderSessions(t *testing.T, events []workers.ModelEvent) {
	t.Helper()
	for _, event := range events {
		if event.Response == nil {
			t.Fatal("failure response is nil")
		}
		providerSession := event.Response.ProviderSession
		if providerSession == nil || providerSession.Provider != "antigravity" || providerSession.ID != "" {
			t.Fatalf("failure provider session = %#v, want provider-only antigravity metadata", event.Response)
		}
	}
}

func TestRuntimeModelRecordingHelpersNormalizeOptionalAndExecutionValues(t *testing.T) {
	t.Parallel()
	assertModelExecutionClassification(t)
	assertModelCorrelationHelpers(t)
	assertModelOptionalHelpers(t)
}

func assertModelExecutionClassification(t *testing.T) {
	t.Helper()
	if isModelExecution(workers.ExecuteRequest{Target: workers.ExecutionTarget{RunnerID: "script", Model: workers.ModelReference{Name: "model"}}}) {
		t.Fatal("script execution was classified as model execution")
	}
	if isModelExecution(workers.ExecuteRequest{Target: workers.ExecutionTarget{RunnerID: "inference", Model: workers.ModelReference{Name: "model"}}}) {
		t.Fatal("inference execution was classified as model execution")
	}
	if isModelExecution(workers.ExecuteRequest{Target: workers.ExecutionTarget{RunnerID: "codex"}}) {
		t.Fatal("runner without model was classified as model execution")
	}
	if !isModelExecution(modelRecordingRequest()) {
		t.Fatal("model execution was not recognized")
	}
}

func assertModelCorrelationHelpers(t *testing.T) {
	t.Helper()
	if got := detachedModelRequestID(" dispatch ", 3); got != "dispatch/model-request/3" {
		t.Fatalf("detachedModelRequestID() = %q", got)
	}
	if got := modelEventTick(modelRecordingRequest()); got != 7 {
		t.Fatalf("modelEventTick() = %d, want dispatch-created tick", got)
	}
	request := modelRecordingRequest()
	request.Input.Dispatch.Execution.CurrentTick = 8
	if got := modelEventTick(request); got != 8 {
		t.Fatalf("modelEventTick() = %d, want current tick", got)
	}
	if got := executionWorkerName(request); got != "worker-1" {
		t.Fatalf("executionWorkerName() = %q, want worker name", got)
	}
	request.Target.WorkerName = ""
	request.Target.WorkerType = "worker-type"
	if got := executionWorkerName(request); got != "worker-type" {
		t.Fatalf("executionWorkerName() fallback = %q, want worker type", got)
	}
}

func assertModelOptionalHelpers(t *testing.T) {
	t.Helper()
	if optionalString(" ") != nil || nonEmptyStrings(" ") != nil {
		t.Fatal("blank optional values were retained")
	}
	if got := optionalString(" value "); got == nil || *got != "value" {
		t.Fatalf("optionalString() = %#v, want trimmed pointer", got)
	}
	if got := nonEmptyStrings(" trace "); len(got) != 1 || got[0] != " trace " {
		t.Fatalf("nonEmptyStrings() = %#v, want original non-empty value", got)
	}
	if resolvedModelBindings(nil) != nil {
		t.Fatal("empty model bindings returned non-nil pointer")
	}
	bindings := []workers.ResolvedModelOperationBinding{{Slot: "slot", Content: []work.WorkContentPart{{Text: "content"}}}}
	cloned := resolvedModelBindings(bindings)
	if cloned == nil || len(*cloned) != 1 || (*cloned)[0].Slot != "slot" {
		t.Fatalf("resolvedModelBindings() = %#v, want cloned bindings", cloned)
	}
	assertModelProviderSessionFallbacks(t)
}

func assertModelProviderSessionFallbacks(t *testing.T) {
	t.Helper()
	request := modelRecordingRequest()
	if providerSessionFromExecuteResult(request, workers.ExecuteResult{}) != nil {
		t.Fatal("empty continuation returned a provider session")
	}
	if providerSessionFromExecuteResult(request, workers.ExecuteResult{Continuation: &workers.ProviderContinuationRef{}}) != nil {
		t.Fatal("empty provider continuation returned a provider session")
	}
	request.Target.Provider.ID = "agent"
	if got := providerSessionFromExecuteResult(request, workers.ExecuteResult{}); got == nil || got.Provider != "cursor" || got.ID != "" {
		t.Fatalf("provider fallback = %#v, want canonical provider identity without a fabricated session id", got)
	}
}

func TestPrepareDetachedModelRecordingPreservesDisabledAndPreviousErrors(t *testing.T) {
	t.Parallel()

	previousCalled := false
	previous := func(context.Context, *workers.ExecuteRequest) (attemptTerminalFunc, error) {
		previousCalled = true
		return nil, nil
	}
	prepared := prepareDetachedModelRecording(nil, previous)
	disabledRequest := modelRecordingRequest()
	if _, err := prepared(context.Background(), &disabledRequest); err != nil {
		t.Fatalf("disabled preparation error = %v", err)
	}
	if !previousCalled {
		t.Fatal("disabled preparation did not preserve previous preparation")
	}

	cfg := &runtimeConfig{
		executeService: modelRecordingExecuteService{owns: true},
		eventHistory:   &modelRecordingLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}},
		clock:          testRuntimeClock{},
	}
	wantErr := errors.New("previous preparation failed")
	prepared = prepareDetachedModelRecording(cfg, func(context.Context, *workers.ExecuteRequest) (attemptTerminalFunc, error) {
		return nil, wantErr
	})
	failedPreparationRequest := modelRecordingRequest()
	if _, err := prepared(context.Background(), &failedPreparationRequest); !errors.Is(err, wantErr) {
		t.Fatalf("previous preparation error = %v, want %v", err, wantErr)
	}
}

func modelRecordingRequest() workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{DispatchID: "dispatch-1", RequestID: "request-1", TraceID: "trace-1"},
		Target: workers.ExecutionTarget{
			WorkerName: "worker-1", WorkerType: "agent", RunnerID: "codex",
			Model:       workers.ModelReference{Name: "model-1", Locality: "remote"},
			Environment: workers.EnvironmentPolicy{WorkingDirectory: " /workspace "},
			Workspace:   workers.WorkspacePolicy{Worktree: " feature-1 "},
		},
		Input: workers.ExecutionInput{
			ModelOperation: " summarize ",
			ModelBindings: []workers.ResolvedModelOperationBinding{{
				Slot: "summary", Source: workers.ModelOperationBindingSourceInput,
				Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "binding"}},
			}},
			Dispatch: work.WorkDispatch{Execution: work.ExecutionMetadata{
				DispatchCreatedTick: 7, RequestID: "request-1", TraceID: "trace-1", WorkIDs: []string{"work-1"},
			}},
		},
	}
}

type modelRecordingExecuteService struct{ owns bool }

func (service modelRecordingExecuteService) Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return workers.ExecuteResult{}, nil
}

func (service modelRecordingExecuteService) RuntimeOwnsModelEventRecording() bool {
	return service.owns
}

type modelRecordingLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
	events []workers.ModelEvent
}

func (ledger *modelRecordingLedger) RecordModelEvent(event workers.ModelEvent) {
	ledger.events = append(ledger.events, event)
}

type runtimeLedgerWithoutWorkerRecorder struct{ recordings.RuntimeLedger }

// runtimeWorkerSessionsService is a small service-root test double. It keeps
// identity reservation and cancellation observable while delegating execution
// to the already injected Workers root, so Runtime tests do not construct peer
// services through their private wire packages.
type runtimeWorkerSessionsService struct {
	*fakeWorkerSessionsService

	mu       sync.Mutex
	sessions map[string]workersessions.Session
}

func newRuntimeWorkerSessionsService(execution workers.Service) *runtimeWorkerSessionsService {
	return &runtimeWorkerSessionsService{
		fakeWorkerSessionsService: &fakeWorkerSessionsService{execution: execution},
		sessions:                  make(map[string]workersessions.Session),
	}
}

func (service *runtimeWorkerSessionsService) Reserve(
	ctx context.Context,
	request workersessions.ReserveRequest,
) (workersessions.Session, error) {
	if err := ctx.Err(); err != nil {
		return workersessions.Session{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if _, exists := service.sessions[request.ID]; exists {
		return workersessions.Session{}, workersessions.ErrSessionAlreadyExists
	}
	session := workersessions.Session{ID: request.ID, State: workersessions.StateReserved}
	service.sessions[request.ID] = session
	return session.Clone(), nil
}

func (service *runtimeWorkerSessionsService) Get(
	ctx context.Context,
	request workersessions.GetRequest,
) (workersessions.Session, error) {
	if err := ctx.Err(); err != nil {
		return workersessions.Session{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	session, ok := service.sessions[request.ID]
	if !ok {
		return workersessions.Session{}, workersessions.ErrSessionNotFound
	}
	return session.Clone(), nil
}

func (service *runtimeWorkerSessionsService) BeginRuntimeAttempt(
	ctx context.Context,
	request workersessions.RuntimeAttemptRequest,
) (workersessions.RuntimeAttempt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	service.mu.Lock()
	service.sessions[request.ID] = workersessions.Session{ID: request.ID, State: workersessions.StateRunning}
	service.mu.Unlock()
	return workersessions.RuntimeAttempt(func(
		_ context.Context,
		result workers.WorkstationDispatchResult,
		_ error,
	) error {
		service.mu.Lock()
		defer service.mu.Unlock()
		session := service.sessions[request.ID]
		switch result.TerminalOutcome {
		case workers.WorkstationDispatchTerminalOutcomeCompleted:
			session.State = workersessions.StateCompleted
			session.Result = &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
		case workers.WorkstationDispatchTerminalOutcomeCanceled:
			session.State = workersessions.StateCanceled
			session.Result = nil
		default:
			session.State = workersessions.StateFailed
			session.Result = &workersessions.TerminalResult{
				Outcome: workersessions.TerminalOutcomeFailed,
				Cause: &workersessions.FailureCause{
					Kind:   workersessions.FailureCauseWorkersExecutionFailure,
					Detail: "runtime Worker Sessions test execution failed",
				},
			}
		}
		service.sessions[request.ID] = session
		return nil
	}), nil
}

func (service *runtimeWorkerSessionsService) Cancel(
	ctx context.Context,
	request workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	if err := ctx.Err(); err != nil {
		return workersessions.ControlResult{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	session, ok := service.sessions[request.ID]
	if !ok {
		return workersessions.ControlResult{}, workersessions.ErrSessionNotFound
	}
	session.State = workersessions.StateCanceled
	service.sessions[request.ID] = session
	return workersessions.ControlResult{
		Session: session.Clone(), Action: workersessions.ControlActionCancel,
		Outcome: workersessions.ControlOutcomeApplied, DispatchID: request.ID,
	}, nil
}

func (service *runtimeWorkerSessionsService) InvokeSession(
	ctx context.Context,
	request workersessions.InvokeSessionRequest,
) (workersessions.InvokeSessionResult, error) {
	service.mu.Lock()
	session, ok := service.sessions[request.ID]
	service.mu.Unlock()
	if !ok {
		return workersessions.InvokeSessionResult{}, workersessions.ErrSessionNotFound
	}
	if session.State == workersessions.StateCanceled {
		return workersessions.InvokeSessionResult{
			Session:     session.Clone(),
			Dispatch:    canceledRuntimeDispatch(request.Execution),
			DispatchErr: workers.ErrWorkstationDispatchCanceled,
		}, nil
	}

	maxAttempts := request.Retry.Attempts()
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		execution := workers.CloneWorkstationExecutionRequest(request.Execution.Execution)
		if attempt > 1 {
			execution.Dispatch.DispatchID = fmt.Sprintf("%s/attempt/%d", request.ID, attempt)
		}
		dispatchRequest := workers.WorkstationDispatchRequest{
			WorkstationName: request.Execution.WorkstationName,
			Execution:       execution,
		}
		executeResult, executeErr := service.execution.Execute(ctx, testExecuteRequestFromDispatch(dispatchRequest))
		dispatchResult := testDispatchResultFromExecute(dispatchRequest, executeResult, executeErr)
		if dispatchResult.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeFailed &&
			dispatchResult.Result.FailureMetadata != nil &&
			dispatchResult.Result.FailureMetadata.Family == workers.WorkFailureFamilyRetryable &&
			attempt < maxAttempts {
			continue
		}
		service.mu.Lock()
		switch dispatchResult.TerminalOutcome {
		case workers.WorkstationDispatchTerminalOutcomeCompleted:
			session.State = workersessions.StateCompleted
		case workers.WorkstationDispatchTerminalOutcomeCanceled:
			session.State = workersessions.StateCanceled
		default:
			session.State = workersessions.StateFailed
		}
		service.sessions[request.ID] = session
		service.mu.Unlock()
		return workersessions.InvokeSessionResult{
			Session: session.Clone(), Dispatch: dispatchResult, DispatchErr: executeErr,
		}, nil
	}
	return workersessions.InvokeSessionResult{}, errors.New("runtime Worker Sessions test service exhausted retry attempts")
}

func canceledRuntimeDispatch(request workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult {
	return workers.WorkstationDispatchResult{
		DispatchID:      request.Execution.Dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCanceled,
		Result: workers.WorkResult{
			DispatchID:   request.Execution.Dispatch.DispatchID,
			TransitionID: request.Execution.Dispatch.TransitionID,
			Outcome:      workers.OutcomeFailed,
			Error:        workers.ErrWorkstationDispatchCanceled.Error(),
		},
	}
}

func directInputPromptRuntimeFixture(
	t *testing.T,
) (*runtimeConfig, workers.WorkstationDispatchRequest, modelinference.RuntimeScopeRef) {
	t.Helper()
	const (
		workstationName = "execute-tts"
		workerName      = "tts-executor"
		promptTemplate  = "For Work {{ (index .Inputs 0).WorkID }}, read the complete bound text input."
		workID          = "work-tts-1"
		boundText       = "The release is ready."
	)
	modelScope, err := (modelinference.RuntimeScopeRef{}).Parse("factory-session:models")
	if err != nil {
		t.Fatalf("parse Models scope: %v", err)
	}
	cfg := &runtimeConfig{
		modelRuntimeScope: modelScope,
		newID:             func() string { return "attempt-1" },
		promptRenderer: runtimePromptRendererFunc(func(
			prompt string,
			tokens []workers.Token,
			_ *workers.Context,
		) (string, error) {
			if prompt != promptTemplate {
				t.Fatalf("prompt template = %q, want authored template", prompt)
			}
			if len(tokens) != 1 || tokens[0].Color.WorkID != workID ||
				len(tokens[0].Color.Content) != 1 || tokens[0].Color.Content[0].Text != boundText {
				t.Fatalf("prompt tokens = %#v, want WorkID %q with complete bound text", tokens, workID)
			}
			return "For Work " + tokens[0].Color.WorkID + ", read: " + tokens[0].Color.Content[0].Text, nil
		}),
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				workstationName: {
					Name: workstationName, Type: interfaces.WorkstationTypeInference,
					WorkerTypeName: workerName, PromptTemplate: promptTemplate,
					Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "ready"}},
				},
			},
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				workerName: {
					Name: workerName, Type: interfaces.WorkerTypeInference,
					Model: "OMNIVOICE_Q4_K_M", ModelProvider: "CODEX",
					ModelLocality: modelinference.RuntimeModelLocalityLocal,
					Resources:     []interfaces.ResourceConfig{{Name: "omnivoice-cache", Capacity: 1}},
				},
			},
			Factory: &interfaces.FactoryConfig{Resources: []interfaces.ResourceConfig{{
				Name: "omnivoice-cache", Type: interfaces.ResourceTypeModel, Capacity: 1,
				Model: "OMNIVOICE_Q4_K_M", Backend: "LLAMACPP", LoadPolicy: "ON_DEMAND",
			}}},
		},
	}
	return cfg, directInputPromptRequest(workstationName, workerName, workID, boundText), modelScope
}

func directInputPromptRequest(
	workstationName, workerName, workID, boundText string,
) workers.WorkstationDispatchRequest {
	return workers.WorkstationDispatchRequest{
		WorkstationName: workstationName,
		Execution: workers.WorkstationExecutionRequest{
			WorkerName: workerName, WorkerType: interfaces.WorkerTypeInference,
			RunnerID: workers.RunnerIDCodex, FactorySessionID: "session-1",
			RuntimeID: "runtime-1", GenerationID: "generation-1",
			InputTokens: workers.InputTokens(workers.Token{ID: "token-1", State: "ready", Color: workers.Color{
				WorkID: workID, WorkTypeID: "task", RequestID: "request-1", DataType: workers.DataTypeWork,
				Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: boundText, Slot: "text"}},
			}}),
			Dispatch: work.WorkDispatch{
				DispatchID: "dispatch-1", WorkstationName: workstationName, WorkerType: workerName,
				Execution: work.ExecutionMetadata{RequestID: "request-1", WorkIDs: []string{workID}},
			},
		},
	}
}

func assertDirectInputPromptExecuteRequest(
	t *testing.T,
	request workers.ExecuteRequest,
	modelScope modelinference.RuntimeScopeRef,
) {
	t.Helper()
	if got := request.Target.Prompt.UserMessage; got != "For Work work-tts-1, read: The release is ready." {
		t.Fatalf("rendered user message = %q, want WorkID and complete bound text", got)
	}
	if len(request.Input.Work) != 1 || request.Input.Work[0].WorkID != "work-tts-1" ||
		len(request.Input.Work[0].Content) != 1 || request.Input.Work[0].Content[0].Text != "The release is ready." {
		t.Fatalf("detached Work inputs = %#v, want canonical per-input Work", request.Input.Work)
	}
	if len(workers.WorkDispatchInputTokens(request.Input.Dispatch)) != 1 {
		t.Fatalf("detached dispatch inputs = %#v, want direct input token bridged into dispatch", request.Input.Dispatch.InputTokens)
	}
	modelRuntime := request.Input.ModelRuntime
	if modelRuntime == nil || modelRuntime.Scope != modelScope ||
		modelRuntime.Worker.Name != "tts-executor" || modelRuntime.Worker.Model != "OMNIVOICE_Q4_K_M" ||
		len(modelRuntime.Worker.Resources) != 1 || modelRuntime.Worker.Resources[0].Name != "omnivoice-cache" ||
		len(modelRuntime.Resources) != 1 || modelRuntime.Resources[0].Backend != "LLAMACPP" {
		t.Fatalf("managed Models projection = %#v, want opened scope and authored worker", modelRuntime)
	}
}
func TestRecordedWorkerSessionObservation_ProjectsRestartInterruptionAsTerminalFailure(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-restarted"
	dispatchID := "dispatch-restarted"
	reason := "daemon restart interrupted process-bound attempt"
	events := []interfaces.FactoryEvent{
		{
			Context: interfaces.FactoryEventContext{
				Tick: 1, Sequence: 1, EventTime: base,
				DispatchID: stringPointerForRecordedTest(dispatchID),
				WorkIDs:    stringSliceForRecordedTest([]string{workID}),
			},
			Type:    interfaces.FactoryEventTypeDispatchRequest,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchRequestEventPayload{}),
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 1, Sequence: 2, EventTime: base.Add(time.Second),
				DispatchID: stringPointerForRecordedTest(dispatchID),
			},
			Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{
				WorkerSessionID: "worker-restarted",
			}),
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 2, Sequence: 3, EventTime: base.Add(2 * time.Second),
				DispatchID: stringPointerForRecordedTest(dispatchID),
				WorkIDs:    stringSliceForRecordedTest([]string{workID}),
			},
			Type: interfaces.FactoryEventTypeDispatchInterrupted,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchInterruptedEventPayload{
				InterruptedAt: base.Add(2 * time.Second), ObservedStatus: interfaces.FactoryDispatchStatusRunning,
				Reason: reason, RetryPlanned: true,
			}),
		},
	}
	service := newRecordedWorkerSessionObservation(
		nil,
		&recordingfixtures.ScriptedRuntimeLedger{Events: events},
		func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{
				WorkItemsByID: map[string]work.FactoryWorkItem{workID: {ID: workID}},
				ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{dispatchID: {
					DispatchID: dispatchID, StartedAt: base, WorkItemIDs: []string{workID},
				}},
			}, nil
		},
		platformclock.Real{},
		nil,
	)

	result, err := service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil || len(result.Observations) != 1 {
		t.Fatalf("ListObservations() = %#v, %v; want one interrupted observation", result, err)
	}
	observation := result.Observations[0]
	if observation.State != workersessions.StateFailed {
		t.Fatalf("interrupted observation state = %q, want FAILED", observation.State)
	}
	if observation.Failure == nil || observation.Failure.Kind != workersessions.FailureCauseProcessGone || observation.Failure.Detail != reason {
		t.Fatalf("interrupted observation failure = %#v, want process-gone restart reason", observation.Failure)
	}
	if observation.EndedAt == nil || !observation.EndedAt.Equal(base.Add(2*time.Second)) {
		t.Fatalf("interrupted observation endedAt = %#v, want interruption time", observation.EndedAt)
	}
	if err := observation.Validate(); err != nil {
		t.Fatalf("interrupted observation validation = %v", err)
	}
}

func TestNew_WithRestoredWorldStateSeedsEachOccupiedWorkExactlyOnce(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	net := buildSimpleNet()
	net.WorkTypes["task"].States = []state.StateDefinition{
		{Value: "init", Category: state.StateCategoryInitial},
		{Value: "processing", Category: state.StateCategoryProcessing},
		{Value: "done", Category: state.StateCategoryTerminal},
		{Value: "failed", Category: state.StateCategoryFailed},
	}
	net.Places = make(map[string]*petri.Place)
	for _, place := range net.WorkTypes["task"].GeneratePlaces() {
		net.Places[place.ID] = place
	}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-init": {
				ID: "work-init", WorkTypeID: "task", State: "init",
				DisplayName: "Initialize", TraceID: "trace-batch",
			},
			"work-processing": {
				ID: "work-processing", WorkTypeID: "task", State: "processing",
				DisplayName: "Process", TraceID: "trace-batch", ParentID: "work-init",
			},
			"work-done": {
				ID: "work-done", WorkTypeID: "task", State: "done",
				DisplayName: "Done", TraceID: "trace-batch",
			},
		},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{
			"work-init":       {ID: "work-init", WorkTypeID: "task", State: "init"},
			"work-processing": {ID: "work-processing", WorkTypeID: "task", State: "processing"},
		},
		TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{
			"work-done": {WorkItem: work.FactoryWorkItem{ID: "work-done", WorkTypeID: "task", State: "done"}, Status: "TERMINAL"},
		},
		WorkRequestsByID: map[string]interfaces.WorkRequestPayload{
			"request-batch": {RequestID: "request-batch", WorkItems: []work.FactoryWorkItem{
				{ID: "work-init"}, {ID: "work-processing"}, {ID: "work-done"},
			}},
		},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{
			"task:init": {PlaceID: "task:init", WorkItemIDs: []string{"work-init"}},
			"task:done": {PlaceID: "task:done", WorkItemIDs: []string{"work-done"}},
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-processing": {
				DispatchID:  "dispatch-processing",
				WorkItemIDs: []string{"work-processing"},
				Inputs: []interfaces.WorkstationInput{{
					TokenID: "work-processing", PlaceID: "task:processing",
					WorkItem: &work.FactoryWorkItem{ID: "work-processing"},
				}},
			},
		},
	}

	f, err := newTestFactory(
		withNet(net),
		withClock(platformclock.NewDeterministic(base, time.Second)),
		withRestoredWorldState(restored),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	wantByPlace := map[string]string{
		"task:init":       "work-init",
		"task:processing": "work-processing",
		"task:done":       "work-done",
	}
	seen := make(map[string]int, len(wantByPlace))
	for placeID, workID := range wantByPlace {
		tokenIDs := snapshot.Marking.PlaceTokens[placeID]
		if len(tokenIDs) != 1 {
			t.Fatalf("restored token IDs at %q = %#v, want one token", placeID, tokenIDs)
		}
		token := snapshot.Marking.Tokens[tokenIDs[0]]
		if token == nil || token.Color.WorkID != workID {
			t.Fatalf("restored token at %q = %#v, want Work %q", placeID, token, workID)
		}
		seen[token.Color.WorkID]++
	}
	if len(seen) != len(wantByPlace) {
		t.Fatalf("restored Work identities = %#v, want one identity per occupied place", seen)
	}
	for workID, count := range seen {
		if count != 1 {
			t.Fatalf("restored Work %q appears %d times, want once", workID, count)
		}
	}
}

type restoredRequestLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
}

func (ledger *restoredRequestLedger) RecordWorkRequest(
	tick int,
	record work.WorkRequestRecord,
	eventTime time.Time,
) {
	ledger.ScriptedRuntimeLedger.RecordWorkRequest(tick, record, eventTime)
	workIDs := make([]string, 0, len(record.WorkItems))
	works := make([]work.WorkRequestEventWork, 0, len(record.WorkItems))
	for _, item := range record.WorkItems {
		workIDs = append(workIDs, item.ID)
		works = append(works, restoredRequestEventWork(record.RequestID, item))
	}
	payload, err := json.Marshal(work.WorkRequestEventPayload{
		ParentLineage: append([]string(nil), record.ParentLineage...),
		Source:        record.Source,
		Type:          record.Type,
		Works:         works,
	})
	if err != nil {
		panic(err)
	}
	requestID := record.RequestID
	source := record.Source
	ledger.AppendRecordedEvent(interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			EventTime: eventTime,
			RequestID: &requestID,
			Source:    &source,
			Tick:      tick,
			WorkIDs:   stringSlicePointer(workIDs),
		},
		Id:            "factory-event/work-request/test/" + requestID,
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeWorkRequest,
	})
}

func restoredRequestEventWork(requestID string, item work.FactoryWorkItem) work.WorkRequestEventWork {
	name := item.DisplayName
	if name == "" {
		name = item.ID
	}
	var state *work.WorkEventState
	if item.State != "" {
		state = &work.WorkEventState{Name: item.State}
	}
	return work.WorkRequestEventWork{
		Name:                     name,
		WorkID:                   item.ID,
		RequestID:                requestID,
		WorkTypeID:               item.WorkTypeID,
		State:                    state,
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   item.CurrentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), item.PreviousChainingTraceIDs...),
		TraceID:                  item.TraceID,
		Content:                  append([]work.WorkContentPart(nil), item.Content...),
		StructuredResult:         item.StructuredResult,
		StructuredResultPresent:  item.StructuredResultPresent,
		Tags:                     item.Tags,
	}
}

func TestRestoreCarriesWorkRequestsBeforeDispatchRecoveryFacts(t *testing.T) {
	ledger := &restoredRequestLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	item := work.FactoryWorkItem{
		ID: "work-restored", WorkTypeID: "task", State: "init", DisplayName: "Restored",
		TraceID: "trace-restored", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "durable content"}},
	}
	restored := &interfaces.FactoryWorldState{
		Tick:                12,
		WorkItemsByID:       map[string]work.FactoryWorkItem{item.ID: item},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{item.ID: item},
		WorkRequestsByID: map[string]interfaces.WorkRequestPayload{
			"request-restored": {RequestID: "request-restored", Type: work.WorkRequestTypeFactoryRequestBatch, TraceID: item.TraceID, WorkItems: []work.FactoryWorkItem{item}},
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-restored": {
				DispatchID: "dispatch-restored", TransitionID: "t-process", StartedTick: 11,
				WorkItemIDs: []string{item.ID}, Inputs: []interfaces.WorkstationInput{{TokenID: item.ID, PlaceID: "task:init", WorkItem: &item}},
			},
		},
	}
	cfg := &runtimeConfig{clock: testRuntimeClock{}, restoredWorldState: restored}
	recordRestoredWorkRequests(cfg, ledger)
	if err := reconcileRestoredDispatches(cfg, ledger); err != nil {
		t.Fatalf("reconcileRestoredDispatches: %v", err)
	}

	events := ledger.CanonicalEvents()
	assertRestoredRecoveryEvents(t, events, item)
	if err := reconcileRestoredDispatches(cfg, ledger); err != nil {
		t.Fatalf("repeat reconcileRestoredDispatches: %v", err)
	}
	if got := len(ledger.CanonicalEvents()); got != len(events) {
		t.Fatalf("recovery events after repeat = %d, want %d", got, len(events))
	}
}

func assertRestoredRecoveryEvents(t *testing.T, events []interfaces.FactoryEvent, item work.FactoryWorkItem) {
	t.Helper()
	if len(events) != 4 {
		t.Fatalf("recovery events = %d, want Work request plus three dispatch facts", len(events))
	}
	wantTypes := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeWorkRequest,
		interfaces.FactoryEventTypeDispatchRequest,
		interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
		interfaces.FactoryEventTypeDispatchInterrupted,
	}
	for index, wantType := range wantTypes {
		if events[index].Type != wantType {
			t.Fatalf("recovery event %d type = %s, want %s", index, events[index].Type, wantType)
		}
	}
	assertRestoredWorkRequestPayload(t, events[0], item)
	assertRestoredDispatchRequestPayload(t, events[1], item)
}

func assertRestoredWorkRequestPayload(t *testing.T, event interfaces.FactoryEvent, item work.FactoryWorkItem) {
	t.Helper()
	var payload work.WorkRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode restored Work request: %v", err)
	}
	if len(payload.Works) != 1 {
		t.Fatalf("restored Work request payload Works = %#v, want one item", payload.Works)
	}
	restored := payload.Works[0]
	if restored.WorkID != item.ID || restored.State == nil || restored.State.Name != item.State || len(restored.Content) != 1 || restored.Content[0].Text != "durable content" {
		t.Fatalf("restored Work request payload = %#v, want durable identity/state/content", payload)
	}
}

func assertRestoredDispatchRequestPayload(t *testing.T, event interfaces.FactoryEvent, item work.FactoryWorkItem) {
	t.Helper()
	var payload interfaces.DispatchRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode restored dispatch request: %v", err)
	}
	if payload.TransitionID != "t-process" || len(payload.Inputs) != 1 || payload.Inputs[0].WorkID != item.ID {
		t.Fatalf("restored dispatch request payload = %#v, want transition and Work input", payload)
	}
}

func testRestoredNoAttempt(t *testing.T, base time.Time) {
	t.Helper()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	item := work.FactoryWorkItem{ID: "work-no-attempt", WorkTypeID: "task", State: "init"}
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withClock(platformclock.NewDeterministic(base, time.Second)),
		withFactoryEventHistory(ledger),
		withRestoredWorldState(&interfaces.FactoryWorldState{
			WorkItemsByID:       map[string]work.FactoryWorkItem{item.ID: item},
			ActiveWorkItemsByID: map[string]work.FactoryWorkItem{item.ID: item},
			PlaceOccupancyByID:  map[string]interfaces.FactoryPlaceOccupancy{"task:init": {PlaceID: "task:init", WorkItemIDs: []string{item.ID}}},
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, item.ID, "task:init") {
		t.Fatalf("restored no-attempt Work marking = %#v, want task:init", snapshot.Marking.PlaceTokens)
	}
	if got := countRestoredInterruptions(ledger); got != 0 {
		t.Fatalf("no-attempt interruption count = %d, want zero", got)
	}
}

func testRestoredTerminalWork(t *testing.T, base time.Time) {
	t.Helper()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	item := work.FactoryWorkItem{ID: "work-terminal", WorkTypeID: "task", State: "done"}
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withClock(platformclock.NewDeterministic(base, time.Second)),
		withFactoryEventHistory(ledger),
		withRestoredWorldState(&interfaces.FactoryWorldState{
			WorkItemsByID: map[string]work.FactoryWorkItem{item.ID: item},
			TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{
				item.ID: {WorkItem: item, Status: "TERMINAL"},
			},
			PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{"task:done": {PlaceID: "task:done", WorkItemIDs: []string{item.ID}}},
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, item.ID, "task:done") {
		t.Fatalf("restored terminal Work marking = %#v, want task:done", snapshot.Marking.PlaceTokens)
	}
	if got := countRestoredInterruptions(ledger); got != 0 {
		t.Fatalf("terminal interruption count = %d, want zero", got)
	}
}

func testRestoredDependencyBlocked(t *testing.T, base time.Time) {
	t.Helper()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	net := buildSimpleNet()
	net.WorkTypes["task"].States = []state.StateDefinition{
		{Value: "init", Category: state.StateCategoryInitial},
		{Value: "processing", Category: state.StateCategoryProcessing},
		{Value: "done", Category: state.StateCategoryTerminal},
		{Value: "failed", Category: state.StateCategoryFailed},
	}
	net.Places = make(map[string]*petri.Place)
	for _, place := range net.WorkTypes["task"].GeneratePlaces() {
		net.Places[place.ID] = place
	}
	net.Transitions["t-process"].InputArcs[0].Guard = &petri.DependencyGuard{}
	dependent := work.FactoryWorkItem{ID: "work-dependent", WorkTypeID: "task", State: "init"}
	dependency := work.FactoryWorkItem{ID: "work-dependency", WorkTypeID: "task", State: "processing"}
	f, err := newTestFactory(
		withNet(net),
		withClock(platformclock.NewDeterministic(base, time.Second)),
		withFactoryEventHistory(ledger),
		withRestoredWorldState(&interfaces.FactoryWorldState{
			WorkItemsByID: map[string]work.FactoryWorkItem{
				dependent.ID: dependent, dependency.ID: dependency,
			},
			ActiveWorkItemsByID: map[string]work.FactoryWorkItem{
				dependent.ID: dependent, dependency.ID: dependency,
			},
			RelationsByWorkID: map[string][]work.FactoryRelation{
				dependent.ID: {{Type: string(work.WorkRelationDependsOn), TargetWorkID: dependency.ID, RequiredState: "done"}},
			},
			PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{
				"task:processing": {PlaceID: "task:processing", WorkItemIDs: []string{dependency.ID}},
			},
			ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
				"dispatch-dependent": {
					DispatchID:  "dispatch-dependent",
					StartedTick: 1,
					WorkItemIDs: []string{dependent.ID},
					Inputs: []interfaces.WorkstationInput{{
						TokenID: dependent.ID, PlaceID: "task:init",
						WorkItem: &dependent,
					}},
				},
			},
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, dependent.ID, "task:init") {
		t.Fatalf("re-armed dependency-blocked Work marking = %#v, want task:init", snapshot.Marking.PlaceTokens)
	}
	if len(snapshot.EnabledTransitions) != 0 {
		t.Fatalf("dependency-blocked enabled transitions = %#v, want none", snapshot.EnabledTransitions)
	}
	if got := countRestoredInterruptions(ledger); got != 1 {
		t.Fatalf("dependency-blocked interruption count = %d, want one", got)
	}
}

func countRestoredInterruptions(ledger *recordingfixtures.ScriptedRuntimeLedger) int {
	count := 0
	for _, event := range ledger.CanonicalEvents() {
		if event.Type == interfaces.FactoryEventTypeDispatchInterrupted {
			count++
		}
	}
	return count
}
