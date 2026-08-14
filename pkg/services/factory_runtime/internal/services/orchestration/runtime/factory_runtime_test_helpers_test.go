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
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type unavailableProviderSessions struct {
	providersessions.Service
}

func (unavailableProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
}

type testFactoryOption func(*testFactoryConfig)

type testFactoryConfig struct {
	net                       *state.Net
	scheduler                 scheduler.Scheduler
	workerExecutors           map[string]workers.WorkerExecutor
	workerService             workers.WorkstationExecutionService
	providerInvocation        workers.WorkstationRequestExecutor
	workerSessions            workersessions.Service
	runtimeConfig             interfaces.RuntimeDefinitionLookup
	workflowContext           *factory_context.FactoryContext
	runtimeMode               interfaces.RuntimeMode
	logger                    logging.Logger
	clock                     factory.Clock
	inlineDispatch            bool
	eventHistory              recordings.RuntimeEventLedger
	submissionHooks           []factory.SubmissionHook
	dispatchRecorder          recordings.DispatchRecorder
	completionRecorder        factory.CompletionRecorder
	petriMutationRecorder     factory.PetriMutationRecorder
	completionDeliveryPlanner factory.CompletionDeliveryPlanner
}

func newTestFactory(opts ...testFactoryOption) (factory.Factory, error) {
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
		workerService = &testWorkstationBoundary{}
	}
	workerSessionsService := cfg.workerSessions
	if workerSessionsService == nil {
		workerSessionsService = &fakeWorkerSessionsService{execution: workerService}
	}
	runtime, err := New(
		cfg.net, cfg.scheduler, &testStatelessExecutionService{
			service:   workerService,
			executors: cfg.workerExecutors,
		}, workerSessionsService, cfg.runtimeConfig, nil, nil,
		cfg.workflowContext, cfg.runtimeMode, cfg.logger, cfg.clock,
		cfg.inlineDispatch, cfg.eventHistory, "runtime-test-recording-id", "runtime-test-id", nil, unavailableProviderSessions{},
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
		workwire.NewRuntimeService(nil, nil, nil, nil, nil),
		func() string { return fmt.Sprintf("work-request-test-id-%d", identity.Add(1)) },
		func() string { return fmt.Sprintf("runtime-test-id-%d", identity.Add(1)) },
		platformfilesystem.Local{},
	)
	if err != nil {
		return nil, err
	}
	return runtime, nil
}

// testStatelessExecutionService keeps legacy test effects behind the detached
// Execute seam. Production Runtime never constructs this adapter.
type testStatelessExecutionService struct {
	service   workers.WorkstationExecutionService
	executors map[string]workers.WorkerExecutor
	startOnce sync.Once
	startErr  error
}

func (service *testStatelessExecutionService) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	service.start(ctx, request)
	if service.startErr != nil {
		return workers.ExecuteResult{}, service.startErr
	}
	if service.service == nil {
		return workers.ExecuteResult{}, fmt.Errorf("test stateless execution service is not configured")
	}
	legacy := testLegacyRequestFromExecute(request)
	result, err := service.service.DispatchWorkstation(ctx, legacy)
	if ctx.Err() != nil {
		_, _ = service.service.CancelWorkstationDispatch(
			context.WithoutCancel(ctx),
			workers.WorkstationDispatchCancelRequest{
				DispatchID: legacy.Execution.Dispatch.DispatchID,
			},
		)
	}
	if err != nil {
		return workers.ExecuteResult{}, err
	}
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     executeOutcomeFromWorkstationResult(result),
		Failure:     executeFailureFromWorkResult(result.Result),
		Output:      workers.ProposedOutputFromLegacyWorkResult(result.Result),
	}, nil
}

func (service *testStatelessExecutionService) start(
	ctx context.Context,
	request workers.ExecuteRequest,
) {
	service.startOnce.Do(func() {
		requestExecutor := testWorkstationRequestExecutor{executors: service.executors}
		bindings := make([]workers.AssembledRuntimeBinding, 0, len(service.executors)+1)
		for workerType := range service.executors {
			bindings = append(bindings, workers.AssembledRuntimeBinding{
				RoleName: workerType, RoleKind: workers.RuntimeBuildRoleKindWorkstation,
				Executor: requestExecutor,
			})
		}
		bindings = append(bindings, workers.AssembledRuntimeBinding{
			RoleName: request.Target.WorkstationName,
			RoleKind: workers.RuntimeBuildRoleKindWorkstation,
			Executor: requestExecutor,
		})
		if service.service != nil {
			_, service.startErr = service.service.StartWorkstationPool(
				ctx, workers.WorkstationPoolStartRequest{Bindings: bindings},
			)
		}
	})
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
			ResumeSession:               providerSessionRefFromContinuation(request.Input.Resume),
		},
	}
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
	return failure
}

func executeOutcomeFromWorkstationResult(result workers.WorkstationDispatchResult) workers.ExecutionOutcome {
	if result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCanceled {
		return workers.ExecutionOutcomeCanceled
	}
	return executeOutcomeFromWorkResult(result.Result)
}

func executeOutcomeFromWorkResult(result workers.WorkResult) workers.ExecutionOutcome {
	switch result.Outcome {
	case workers.OutcomeContinue:
		return workers.ExecutionOutcomeContinue
	case workers.OutcomeRejected:
		return workers.ExecutionOutcomeRejected
	case workers.OutcomeFailed:
		return workers.ExecutionOutcomeFailed
	default:
		return workers.ExecutionOutcomeAccepted
	}
}

func providerSessionRefFromContinuation(
	continuation *workers.ProviderContinuationRef,
) *providers.SessionRef {
	if continuation == nil {
		return nil
	}
	id := strings.TrimSpace(continuation.ProviderSessionID)
	if id == "" {
		id = strings.TrimSpace(continuation.ExternalRef)
	}
	if id == "" {
		return nil
	}
	return &providers.SessionRef{
		Provider: providers.ID(strings.TrimSpace(continuation.Provider)),
		Kind:     providers.SessionIDKind,
		ID:       id,
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
) (factory.Factory, *recordingfixtures.ScriptedRuntimeLedger, error) {
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

func withWorkerService(service workers.WorkstationExecutionService) testFactoryOption {
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
	execution workers.WorkstationExecutionService
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
	dispatchResult, dispatchErr := s.execution.DispatchWorkstation(ctx, handoff)
	return workersessions.InvokeSessionResult{
		Session:     workersessions.Session{ID: req.ID, State: workersessions.StateCompleted},
		Dispatch:    dispatchResult,
		DispatchErr: dispatchErr,
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
	routes map[string]workers.WorkstationRequestExecutor
}

func testWorkstationPoolBoundaryFactory(cfg workers.WorkstationPoolBoundaryConfig) workers.WorkstationPoolBoundary {
	bindings := make([]workers.AssembledRuntimeBinding, 0, len(cfg.RouteNames)+1)
	requestExecutor := testWorkstationRequestExecutor{executors: cfg.Executors}
	for _, routeName := range cfg.RouteNames {
		bindings = append(bindings, workers.AssembledRuntimeBinding{
			RoleName:      routeName,
			RoleKind:      workers.RuntimeBuildRoleKindWorkstation,
			Executor:      requestExecutor,
			Capacity:      cfg.Capacity,
			QueueCapacity: cfg.QueueCapacity,
		})
	}
	if cfg.ProviderInvocation != nil {
		bindings = append(bindings, workers.AssembledRuntimeBinding{
			RoleName:      workers.ProviderInvocationRoute,
			RoleKind:      workers.RuntimeBuildRoleKindWorkstation,
			Executor:      cfg.ProviderInvocation,
			Capacity:      cfg.Capacity,
			QueueCapacity: cfg.QueueCapacity,
		})
	}
	return &testWorkstationPoolBoundary{service: cfg.Service, bindings: bindings}
}

type testWorkstationPoolBoundary struct {
	service  workers.WorkstationExecutionService
	bindings []workers.AssembledRuntimeBinding
	started  bool
	mu       sync.Mutex
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

func (boundary *testWorkstationPoolBoundary) Start(ctx context.Context) error {
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	if boundary.started {
		return nil
	}
	if boundary.service == nil {
		return nil
	}
	if _, err := boundary.service.StartWorkstationPool(ctx, workers.WorkstationPoolStartRequest{
		Bindings: append([]workers.AssembledRuntimeBinding(nil), boundary.bindings...),
	}); err != nil {
		return err
	}
	boundary.started = true
	return nil
}

func (boundary *testWorkstationPoolBoundary) Publish(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	return boundary.PublishWithAdmission(ctx, request, nil, accept)
}

func (boundary *testWorkstationPoolBoundary) PublishWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admission workers.WorkstationDispatchAdmissionFunc,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	if err := boundary.Start(ctx); err != nil {
		return err
	}
	result, err := boundary.service.DispatchWorkstationWithAdmission(ctx, request, admission)
	if accept != nil {
		accept(context.Background(), request, result, err)
	}
	return nil
}

func (boundary *testWorkstationPoolBoundary) Cancel(
	ctx context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	return boundary.service.CancelWorkstationDispatch(ctx, request)
}

func (boundary *testWorkstationPoolBoundary) Stop(ctx context.Context) error {
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	if !boundary.started || boundary.service == nil {
		return nil
	}
	_, err := boundary.service.StopWorkstationPool(ctx)
	return err
}

func (b *testWorkstationBoundary) StartWorkstationPool(
	_ context.Context,
	request workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	b.routes = make(map[string]workers.WorkstationRequestExecutor, len(request.Bindings))
	for _, binding := range request.Bindings {
		b.routes[binding.RoleName] = binding.Executor
	}
	return workers.WorkstationPoolStartResult{
		Outcome: workers.WorkstationPoolLifecycleOutcomeStarted,
	}, nil
}

func (*testWorkstationBoundary) StopWorkstationPool(
	context.Context,
) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{
		Outcome: workers.WorkstationPoolLifecycleOutcomeStopped,
	}, nil
}

func (b *testWorkstationBoundary) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	executor := b.routes[request.WorkstationName]
	if executor == nil {
		result := workerexecution.WorkResult{
			DispatchID:   request.Execution.Dispatch.DispatchID,
			TransitionID: request.Execution.Dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        fmt.Sprintf("no executor registered for worker type %q", request.Execution.WorkerType),
		}
		return workers.WorkstationDispatchResult{
			DispatchID:      result.DispatchID,
			WorkstationName: request.WorkstationName,
			TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
			Result:          result,
		}, nil
	}
	result, err := executor.Execute(ctx, request.Execution)
	terminal := workers.WorkstationDispatchTerminalOutcomeCompleted
	if err != nil || result.Outcome == workerexecution.OutcomeFailed {
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
	}
	return workers.WorkstationDispatchResult{
		DispatchID:      request.Execution.Dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: terminal,
		Result:          result,
	}, err
}

func (b *testWorkstationBoundary) DispatchWorkstationWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	if admitted != nil {
		admitted()
	}
	return b.DispatchWorkstation(ctx, request)
}

func (*testWorkstationBoundary) CancelWorkstationDispatch(
	_ context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	return workers.WorkstationDispatchCancelResult{
		DispatchID: request.DispatchID,
		Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
	}, nil
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
		return safeBoundaryResult(dispatch, workID, workerexecution.OutcomeAccepted, "", nil, &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "response_id",
			ID:       "resp-safe-success",
		}, "1"), nil
	case "work-safe-failure":
		return safeBoundaryResult(dispatch, workID, workerexecution.OutcomeFailed, "provider timed out", &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeTimeout,
		}, &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-safe-failure",
		}, "2"), nil
	case "work-safe-windows-process-failure":
		return safeBoundaryResult(dispatch, workID, workerexecution.OutcomeFailed, "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)", &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeInternalServerError,
		}, &workerexecution.ProviderSessionMetadata{
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

func submitWorkRequests(ctx context.Context, f factory.Factory, reqs []work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
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

func newPassingInlineRuntime(t *testing.T) factory.Factory {
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
) (factory.Factory, *recordingfixtures.ScriptedRuntimeLedger) {
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

func tickableFactory(t *testing.T, f factory.Factory) TickableFactory {
	t.Helper()
	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}
	return tickable
}

func runtimeGeneratedEvents(t *testing.T, f factory.Factory) []factoryapi.FactoryEvent {
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

func factoryEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	return types
}

func countFactoryEventsByType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func hasFactoryEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

// runtimePreWorkEventCount is the number of canonical startup events emitted
// before the first work lifecycle event (RUN_REQUEST, INITIAL_STRUCTURE_REQUEST,
// SESSION_STARTED).
const runtimePreWorkEventCount = 3

func runtimeStartupEventTypes() []factoryapi.FactoryEventType {
	return []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeSessionStarted,
	}
}

func runtimeEventIndex(afterStartup int) int {
	return runtimePreWorkEventCount + afterStartup
}

func countFactoryEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
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
	f factory.Factory,
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

func requestViewForWork(t *testing.T, state interfaces.FactoryWorldState, workID string) recordings.WorkstationFactoryWorldWorkstationRequestView {
	t.Helper()
	slice := recordings.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
	if slice.WorkstationRequestsByDispatchId == nil {
		t.Fatalf("workstation request slice = %#v, want work %q", slice, workID)
	}
	for _, request := range *slice.WorkstationRequestsByDispatchId {
		if request.Request.InputWorkItems == nil {
			continue
		}
		for _, item := range *request.Request.InputWorkItems {
			if item.WorkId == workID {
				return request
			}
		}
	}
	t.Fatalf("missing workstation request for work %q: %#v", workID, slice.WorkstationRequestsByDispatchId)
	return recordings.WorkstationFactoryWorldWorkstationRequestView{}
}

func inferenceAttemptForWork(
	t *testing.T,
	state interfaces.FactoryWorldState,
	workID string,
) (interfaces.FactoryWorldInferenceAttempt, bool) {
	t.Helper()
	request := requestViewForWork(t, state, workID)
	attempts := state.InferenceAttemptsByDispatchID[request.DispatchId]
	if len(attempts) == 0 {
		return interfaces.FactoryWorldInferenceAttempt{}, false
	}
	if len(attempts) != 1 {
		t.Fatalf("inference attempts for dispatch %q = %#v, want one attempt for work %q", request.DispatchId, attempts, workID)
	}
	for _, attempt := range attempts {
		return attempt, true
	}
	t.Fatalf("missing inference attempt for dispatch %q", request.DispatchId)
	return interfaces.FactoryWorldInferenceAttempt{}, false
}

func assertThinDispatchResponsesOmitRetiredProviderAttemptFields(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal dispatch response %s: %v", event.Id, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(encoded, &raw); err != nil {
			t.Fatalf("unmarshal dispatch response %s: %v", event.Id, err)
		}
		payload, ok := raw["payload"].(map[string]any)
		if !ok {
			t.Fatalf("dispatch response payload = %#v, want object", raw["payload"])
		}
		for _, retired := range []string{"inputs", "providerSession", "diagnostics"} {
			if _, ok := payload[retired]; ok {
				t.Fatalf("dispatch response payload unexpectedly carried %q: %#v", retired, payload)
			}
		}
	}
}

func assertRuntimeSafeBoundaryOmittedInferenceFields(t *testing.T, payload any, keys []string) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(%T): %v", payload, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("Unmarshal(%T): %v", payload, err)
	}
	for _, key := range keys {
		if _, ok := raw[key]; ok {
			t.Fatalf("%T unexpectedly carried retired inference-owned field %q: %#v", payload, key, raw[key])
		}
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper intentionally validates the full safe boundary request view contract in one place.
func assertSafeBoundaryRequestView(
	t *testing.T,
	state interfaces.FactoryWorldState,
	request recordings.WorkstationFactoryWorldWorkstationRequestView,
	workID string,
	sessionID string,
	family string,
	providerFailureType string,
	failureMessage string,
) {
	t.Helper()
	if request.Response == nil {
		t.Fatalf("request response = nil, want response for %#v", request)
	}
	assertRuntimeSafeBoundaryOmittedInferenceFields(t, request.Request, []string{"provider", "model", "requestMetadata", "workingDirectory", "worktree"})
	assertRuntimeSafeBoundaryOmittedInferenceFields(t, request.Response, []string{"providerSession", "diagnostics", "responseMetadata"})

	attempt, ok := inferenceAttemptForWork(t, state, workID)
	if ok {
		if attempt.ProviderSession == nil || attempt.ProviderSession.ID != sessionID {
			t.Fatalf("inference attempt provider session = %#v, want %q", attempt.ProviderSession, sessionID)
		}
		if attempt.Diagnostics == nil || attempt.Diagnostics.Provider == nil || attempt.Diagnostics.RenderedPrompt == nil {
			t.Fatalf("inference attempt diagnostics = %#v, want safe diagnostics", attempt.Diagnostics)
		}
		if attempt.Diagnostics.Provider.Provider != "codex" || attempt.Diagnostics.Provider.Model != "gpt-5.4" {
			t.Fatalf("inference attempt provider/model = %#v, want codex/gpt-5.4", attempt.Diagnostics.Provider)
		}
		if attempt.Diagnostics.Provider.RequestMetadata == nil || attempt.Diagnostics.Provider.RequestMetadata["worker_type"] != "mock" {
			t.Fatalf("inference attempt request metadata = %#v, want worker_type=mock", attempt.Diagnostics.Provider.RequestMetadata)
		}
		if attempt.Diagnostics.Provider.ResponseMetadata == nil || attempt.Diagnostics.Provider.ResponseMetadata["provider_session_id"] != sessionID {
			t.Fatalf("inference attempt response metadata = %#v, want provider_session_id=%q", attempt.Diagnostics.Provider.ResponseMetadata, sessionID)
		}
	}
	if family == "" && request.Response.FailureDetail != nil {
		t.Fatalf("failure detail = %#v, want empty for successful request", request.Response.FailureDetail)
	}
	if family == "" {
		return
	}
	if request.Response.FailureDetail == nil || string(request.Response.FailureDetail.Reason) != providerFailureType {
		t.Fatalf("failure detail = %#v, want reason %q", request.Response.FailureDetail, providerFailureType)
	}
	if request.Response.FailureDetail.Message != failureMessage {
		t.Fatalf("failure message = %q, want %q", request.Response.FailureDetail.Message, failureMessage)
	}
	if ok {
		metadata := attempt.Diagnostics.Provider.ResponseMetadata
		if metadata == nil || metadata["retry_count"] != "2" {
			t.Fatalf("response metadata = %#v, want retry_count=2 for failed request", metadata)
		}
	}
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
	providerSession *workerexecution.ProviderSessionMetadata,
	retryCount string,
) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         outcome,
		Output:          "safe boundary output for " + workID,
		Error:           errText,
		FailureMetadata: providerFailure,
		ProviderSession: providerSession,
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

func safeBoundaryGeneratedFactory() factoryapi.Factory {
	workstationID := "t-process"
	return factoryapi.Factory{
		Name: "safe-boundary-factory",
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "task",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
				{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
			},
		}},
		Workers: &[]factoryapi.Worker{{Name: "mock"}},
		Workstations: &[]factoryapi.Workstation{{
			Id:        &workstationID,
			Name:      "Process",
			Worker:    stringPtrForRuntimeTest("mock"),
			Inputs:    []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
			Outputs:   &[]factoryapi.WorkstationIO{{WorkType: "task", State: "done"}},
			OnFailure: &[]factoryapi.WorkstationIO{{WorkType: "task", State: "failed"}},
		}},
	}
}

func stringPtrForRuntimeTest(value string) *string {
	return &value
}

func assertNoAuthRemediationText(t *testing.T, body string) {
	t.Helper()
	lowered := strings.ToLower(body)
	for _, forbidden := range []string{"auth_failure", "authentication", "api key", "unauthorized", "forbidden"} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("expected operator-facing text to avoid %q, got %q", forbidden, body)
		}
	}
}

func assertSafeBoundaryDoesNotLeakJSON(t *testing.T, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON boundary: %v", err)
	}
	assertSafeBoundaryDoesNotLeak(t, string(data))
}

func assertSafeBoundaryDoesNotLeak(t *testing.T, body string) {
	t.Helper()
	for _, unsafe := range safeBoundaryUnsafeValues() {
		if strings.Contains(body, unsafe) {
			t.Fatalf("safe boundary leaked unsafe value %q: %s", unsafe, body)
		}
	}
}

func safeBoundaryUnsafeValues() []string {
	return []string{
		"raw prompt body must stay private",
		"raw response prompt body must stay private",
		"raw stdin payload must stay private",
		"raw response stdin payload must stay private",
		"raw env secret must stay private",
		"raw rendered system prompt must stay private",
		"raw rendered user message must stay private",
		"raw rendered stdin must stay private",
		"raw rendered environment must stay private",
		"raw command stdin must stay private",
		"raw environment value must stay private",
		"AGENT_FACTORY_AUTH_TOKEN",
		"panic stack should not be stored",
	}
}

func maxEventTick(events []factoryapi.FactoryEvent) int {
	maxTick := 0
	for _, event := range events {
		if event.Context.Tick > maxTick {
			maxTick = event.Context.Tick
		}
	}
	return maxTick
}

func stringValueForRuntimeTest[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func firstRuntimeTestString(values *[]string) string {
	for _, value := range sliceValueForRuntimeTest(values) {
		if value != "" {
			return value
		}
	}
	return ""
}

func sliceValueForRuntimeTest[T any](values *[]T) []T {
	if values == nil {
		return nil
	}
	return *values
}

type serviceModeRunHarness struct {
	t       *testing.T
	Factory factory.Factory
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

func submitPausedBufferTask(t *testing.T, f factory.Factory, requestID, traceID string) {
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
	f factory.Factory,
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

func assertPausedSubmissionNotApplied(t *testing.T, f factory.Factory) {
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

func observeNextBufferedResult(t *testing.T, f factory.Factory) <-chan struct{} {
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

func assertPausedWorkerResultBuffered(t *testing.T, f factory.Factory) {
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

func assertPausedSubmissionNotDone(t *testing.T, f factory.Factory) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
	}
	if hasWorkAtPlace(snap, "task:done") {
		t.Fatal("buffered submission applied while paused")
	}
}

func assertPausedWorkerResultNotDone(t *testing.T, f factory.Factory) {
	t.Helper()
	assertPausedWorkerResultBuffered(t, f)
}

func assertTaskDoneOnce(t *testing.T, f factory.Factory) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if count := countTokensAtPlace(snap, "task:done"); count != 1 {
		t.Fatalf("task:done token count = %d, want 1", count)
	}
}

func assertNoInFlightDispatches(t *testing.T, f factory.Factory) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if snap.InFlightCount != 0 {
		t.Fatalf("inFlightCount = %d, want 0 after resume", snap.InFlightCount)
	}
}

func waitForFactoryState(t *testing.T, f factory.Factory, want interfaces.FactoryState, timeout time.Duration) {
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

func waitForWorkAtPlace(t *testing.T, f factory.Factory, placeID string, timeout time.Duration) {
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

func submitTaskWithWorkID(t *testing.T, f factory.Factory, workID, traceID string) {
	t.Helper()
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest for %q: %v", workID, err)
	}
}

func assertWorkNotAtDonePlace(t *testing.T, f factory.Factory, workID string) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if markingContainsWorkAtPlace(&snap.Marking, workID, "task:done") {
		t.Fatalf("marking = %#v, want work %q to remain unprocessed before resume", snap.Marking.Tokens, workID)
	}
}

func assertWorksNotAtDonePlace(t *testing.T, f factory.Factory, workIDs []string) {
	t.Helper()
	for _, workID := range workIDs {
		assertWorkNotAtDonePlace(t, f, workID)
	}
}

func waitForWorkDoneAfterResume(t *testing.T, f factory.Factory, workID string) {
	t.Helper()
	waitForAggregateSnapshotWithTimeout(t, f, 2*time.Second, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return markingContainsWorkAtPlace(&snap.Marking, workID, "task:done")
	})
}

func waitForQuiescentWorksAtDone(t *testing.T, f factory.Factory, workIDs []string) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	return waitForAggregateSnapshotWithTimeout(t, f, 5*time.Second, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return allWorksAtDonePlace(&snap.Marking, workIDs) && snap.InFlightCount == 0
	})
}

func waitForAggregateSnapshotWithTimeout(
	t *testing.T,
	f factory.Factory,
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

func resumeFactory(t *testing.T, f factory.Factory) {
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
	if response.ProviderSession == nil || response.ProviderSession.ID != "session-1" {
		t.Fatalf("recorded provider session = %#v, want session-1", response.ProviderSession)
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
