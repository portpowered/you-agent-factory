package internal_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/legacysnapshot"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"go.uber.org/zap"
)

type stubWorkerSessionsService struct {
	factory.WorkerAttemptOpener
	execution workers.Service
}

// This execution-only double has no durable captured activity.
func (*stubWorkerSessionsService) GetCapturedObservation(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
}

func (*stubWorkerSessionsService) ReadLogs(context.Context, workersessions.ReadLogsRequest) (workersessions.LogPage, error) {
	return workersessions.LogPage{}, workersessions.ErrSessionNotFound
}

func (*stubWorkerSessionsService) ReadLogsArtifact(context.Context, string, string) (io.ReadCloser, error) {
	return nil, workersessions.ErrSessionNotFound
}

func (s *stubWorkerSessionsService) Reserve(context.Context, workersessions.ReserveRequest) (workersessions.Session, error) {
	return workersessions.Session{}, nil
}

func (s *stubWorkerSessionsService) Get(context.Context, workersessions.GetRequest) (workersessions.Session, error) {
	return workersessions.Session{}, nil
}

func (s *stubWorkerSessionsService) List(context.Context, workersessions.ListRequest) (workersessions.ListResult, error) {
	return workersessions.ListResult{}, nil
}

func (s *stubWorkerSessionsService) ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	return workersessions.ListObservationsResult{}, nil
}

func (s *stubWorkerSessionsService) GetObservation(context.Context, workersessions.GetObservationRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, nil
}

func (s *stubWorkerSessionsService) GetObservationByWorkerSessionID(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, nil
}

func (s *stubWorkerSessionsService) ListWorkerSessionObservations(context.Context, workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
	return workersessions.ListWorkerSessionObservationsResult{}, nil
}

func (s *stubWorkerSessionsService) StreamObservations(context.Context, workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error) {
	return workersessions.ObservationSubscription{}, nil
}

func (s *stubWorkerSessionsService) StreamObservationsByWorkerSessionID(context.Context, workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	return workersessions.ObservationSubscription{}, nil
}

func (s *stubWorkerSessionsService) ReadTranscript(context.Context, workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	return workersessions.ReadTranscriptResult{}, nil
}

func (s *stubWorkerSessionsService) ReadTranscriptByWorkerSessionID(context.Context, workersessions.ReadTranscriptByWorkerSessionIDRequest) (workersessions.ReadTranscriptResult, error) {
	return workersessions.ReadTranscriptResult{}, nil
}

func (s *stubWorkerSessionsService) InvokeSession(ctx context.Context, req workersessions.InvokeSessionRequest) (workersessions.InvokeSessionResult, error) {
	return workersessions.InvokeSessionResult{
		Session: workersessions.Session{ID: req.ID, State: workersessions.StateCompleted},
	}, nil
}

func (s *stubWorkerSessionsService) Start(ctx context.Context, req workersessions.StartRequest) (workersessions.StartResult, error) {
	result, err := s.InvokeSession(ctx, workersessions.InvokeSessionRequest{
		ID:        req.ID,
		Execution: req.Execution,
		Retry:     req.Retry,
	})
	return workersessions.StartResult{Session: result.Session}, err
}

func (s *stubWorkerSessionsService) Continue(context.Context, workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
	return workersessions.ContinueResult{}, nil
}

func (s *stubWorkerSessionsService) Interrupt(context.Context, workersessions.InterruptRequest) (workersessions.InterruptResult, error) {
	return workersessions.InterruptResult{}, nil
}

func (s *stubWorkerSessionsService) PublishRecord(context.Context, workersessions.PublishRecordRequest) (workersessions.PublishRecordResult, error) {
	return workersessions.PublishRecordResult{}, nil
}

func (s *stubWorkerSessionsService) AssociateProviderSession(context.Context, workersessions.ProviderSessionAssociationRequest) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *stubWorkerSessionsService) ObserveProviderSession(context.Context, workersessions.ProviderSessionObservationRequest) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *stubWorkerSessionsService) EnsureProviderBinding(context.Context, workersessions.ProviderBindingRequest) (workersessions.ProviderBindingResult, error) {
	return workersessions.ProviderBindingResult{}, nil
}

func (s *stubWorkerSessionsService) WorkerSessionIDForDispatch(_ context.Context, dispatchID string) (string, error) {
	return dispatchID, nil
}

func (s *stubWorkerSessionsService) Pause(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *stubWorkerSessionsService) Resume(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *stubWorkerSessionsService) Cancel(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *stubWorkerSessionsService) Terminate(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func loadedFactoryFixture(dir string) (interfaces.MutableLoadedFactorySource, error) {
	payload, err := os.ReadFile(filepath.Join(dir, interfaces.FactoryConfigFile))
	if err != nil {
		return nil, err
	}
	config, err := factorymapping.NewFactoryConfigMapper().Expand(payload)
	if err != nil {
		return nil, err
	}
	return factorydefinitionfixtures.NewLoadedSource(dir, config, nil, nil)
}

func testOrchestrationCompilation() factory.OrchestrationCompilation {
	return controlledCompilation{}
}

func testDefinitionMapper() *definitionmapping.Mapper {
	mapper, err := definitionmapping.New(testRuntimeID)
	if err != nil {
		panic(err) // The fixed test ID generator is required by this fixture.
	}
	return mapper
}

func testRuntimeFactory(scopes recordings.RuntimeScopeService) *factoryinternal.RuntimeFactory {
	return testRuntimeFactoryWithEffects(nil, nil, nil, nil, nil, scopes)
}

func testRuntimeFactoryWithSinks(logDir, metricsDir string, scopes recordings.RuntimeScopeService) *factoryinternal.RuntimeFactory {
	return testRuntimeFactoryWithSinkCallbacks(logDir, metricsDir, nil, nil, scopes)
}

func testRuntimeFactoryWithSinkCallbacks(
	logDir string,
	metricsDir string,
	onLogClose func(),
	onMetricsClose func(),
	scopes recordings.RuntimeScopeService,
) *factoryinternal.RuntimeFactory {
	return testRuntimeFactoryWithOwners(
		testRuntimeLogOwner{root: logDir, onClose: onLogClose},
		testRuntimeMetricsOwner{root: metricsDir, onClose: onMetricsClose}, scopes,
	)
}

func testRuntimeFactoryWithOwners(logOwner factory.RuntimeLogOwner, metricsOwner factory.RuntimeMetricsOwner, scopes recordings.RuntimeScopeService) *factoryinternal.RuntimeFactory {
	return testRuntimeFactoryWithEffects(logOwner, metricsOwner, nil, nil, nil, scopes)
}

func testRuntimeFactoryWithEffects(logOwner factory.RuntimeLogOwner, metricsOwner factory.RuntimeMetricsOwner,
	submit recordings.SubmissionRecorder, dispatch recordings.DispatchRecorder, project factory.WorldStateProjector,
	scopes recordings.RuntimeScopeService,
) *factoryinternal.RuntimeFactory {
	return testRuntimeFactoryWithEffectsAndOpening(logOwner, metricsOwner, submit, dispatch, project, scopes, &controlledEngineOpening{})
}

func testRuntimeFactoryWithEffectsAndOpening(logOwner factory.RuntimeLogOwner, metricsOwner factory.RuntimeMetricsOwner,
	submit recordings.SubmissionRecorder, dispatch recordings.DispatchRecorder, project factory.WorldStateProjector,
	scopes recordings.RuntimeScopeService, opening *controlledEngineOpening,
) *factoryinternal.RuntimeFactory {
	return factoryinternal.NewRuntimeFactory(
		testRuntimeLoggerFactory, logOwner, metricsOwner,
		testRuntimeID, testRuntimeID, localRuntimeFiles{}, localRuntimeFiles{}, filepath.WalkDir,
		testOrchestrationCompilation(), platformclock.Real{}, nil,
		opening,
		submit, dispatch, project, scopes,
	)
}

func newTestRuntimeLedger(
	recordings.InitialStructureSource,
	func() time.Time,
	interfaces.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return &recordingfixtures.ScriptedRuntimeLedger{}
}

func testRuntimeScopeService(
	ledgerFactory func(
		recordings.InitialStructureSource,
		func() time.Time,
		interfaces.RuntimeDefinitionLookup,
	) recordings.RuntimeEventLedger,
) recordings.RuntimeScopeService {
	return &testRuntimeScopeServiceStub{ledgerFactory: ledgerFactory}
}

type testRuntimeScopeServiceStub struct {
	bySession       map[string]*testRuntimeScopeServiceStub
	openErr         error
	ledger          recordings.RuntimeEventLedger
	ledgerFactory   func(recordings.InitialStructureSource, func() time.Time, interfaces.RuntimeDefinitionLookup) recordings.RuntimeEventLedger
	recorder        recordings.RuntimeRecorder
	capturedSource  *recordings.InitialStructureSource
	capturedRequest *recordings.RuntimeScopeRequest
}

func (runtimeScopes *testRuntimeScopeServiceStub) OpenRuntime(
	ctx context.Context,
	request recordings.RuntimeScopeRequest,
) (recordings.RuntimeScopeResult, error) {
	if selected := runtimeScopes.bySession[request.FactorySessionID]; selected != nil {
		return selected.OpenRuntime(ctx, request)
	}
	if runtimeScopes.capturedSource != nil {
		*runtimeScopes.capturedSource = request.Topology
	}
	if runtimeScopes.capturedRequest != nil {
		*runtimeScopes.capturedRequest = request
	}
	ledger := runtimeScopes.ledger
	if runtimeScopes.ledgerFactory != nil {
		ledger = runtimeScopes.ledgerFactory(request.Topology, request.Now, request.Definitions)
	}
	return recordings.RuntimeScopeResult{Ledger: ledger, Recorder: runtimeScopes.recorder}, runtimeScopes.openErr
}

func (*testRuntimeScopeServiceStub) Projection() recordings.ProjectionService { return nil }

func (*testRuntimeScopeServiceStub) ReconstructCanonicalFactoryWorldState(
	[]interfaces.FactoryEvent,
	int,
) (recordings.FactoryWorldState, error) {
	return recordings.FactoryWorldState{}, nil
}

func (*testRuntimeScopeServiceStub) ReplayClock(*recordings.ReplayArtifact) recordings.Clock {
	return nil
}

func (*testRuntimeScopeServiceStub) ReplayExecution(
	*recordings.ReplayArtifact,
) (providers.Service, platformprocess.CommandRunner, []recordings.ReplayHook, recordings.CompletionDeliveryPlanner, error) {
	return nil, nil, nil, nil, nil
}

func (*testRuntimeScopeServiceStub) LoadReplayInput(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
	return recordings.LoadReplayInputResult{}, nil
}

func (*testRuntimeScopeServiceStub) LoadResumeInput(recordings.LoadResumeInputRequest) (recordings.LoadResumeInputResult, error) {
	return recordings.LoadResumeInputResult{}, nil
}

var _ recordings.RuntimeScopeService = (*testRuntimeScopeServiceStub)(nil)

func testRuntimeLoggerFactory(*zap.Logger, bool) factory.Logger { return factory.NoopLogger{} }

type testRuntimeLogSink struct {
	logger   *zap.Logger
	artifact factory.RuntimeLogArtifact
	onClose  func()
	closeErr error
}

func (sink *testRuntimeLogSink) Logger() *zap.Logger                  { return sink.logger }
func (sink *testRuntimeLogSink) Artifact() factory.RuntimeLogArtifact { return sink.artifact }
func (sink *testRuntimeLogSink) Close() error {
	if sink != nil && sink.onClose != nil {
		sink.onClose()
	}
	return sink.closeErr
}

type testRuntimeLogOwner struct {
	root     string
	onClose  func()
	openErr  error
	closeErr error
}

type testRuntimeLogOwnerFunc func(*zap.Logger, factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error)

func (owner testRuntimeLogOwnerFunc) Open(logger *zap.Logger, request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
	return owner(logger, request)
}

func (owner testRuntimeLogOwner) Open(logger *zap.Logger, request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
	return &testRuntimeLogSink{logger: logger, onClose: owner.onClose, closeErr: owner.closeErr, artifact: factory.RuntimeLogArtifact{
		Path: filepath.Join(owner.root, request.RuntimeInstanceID+".runtime.log"), RootDir: owner.root,
		StartTimeUTC: time.Now().UTC(), Config: request.Config,
	}}, owner.openErr
}

type testRuntimeMetricsSink struct {
	artifact factory.RuntimeMetricsArtifact
	onClose  func()
	closeErr error
}

func (s *testRuntimeMetricsSink) Counter(context.Context, string, float64, factory.Fields) error {
	return nil
}
func (s *testRuntimeMetricsSink) Gauge(context.Context, string, float64, factory.Fields) error {
	return nil
}
func (s *testRuntimeMetricsSink) Sample(context.Context, string, float64, string, factory.Fields) error {
	return nil
}
func (s *testRuntimeMetricsSink) Close() error {
	if s != nil && s.onClose != nil {
		s.onClose()
	}
	return s.closeErr
}
func (s *testRuntimeMetricsSink) Path() string { return s.artifact.Path }
func (s *testRuntimeMetricsSink) Artifact() factory.RuntimeMetricsArtifact {
	return s.artifact
}

type testRuntimeMetricsOwner struct {
	root     string
	onClose  func()
	openErr  error
	closeErr error
}

type testRuntimeMetricsOwnerFunc func(factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error)

func (owner testRuntimeMetricsOwnerFunc) Open(request factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) {
	return owner(request)
}

func (owner testRuntimeMetricsOwner) Open(request factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) {
	return &testRuntimeMetricsSink{onClose: owner.onClose, closeErr: owner.closeErr, artifact: factory.RuntimeMetricsArtifact{
		Path: filepath.Join(owner.root, request.Scope.RuntimeInstanceID+".runtime-metrics.log"), RootDir: owner.root,
		StartTimeUTC: time.Now().UTC(),
	}}, owner.openErr
}

type runtimeRecordingsRecorderStub struct {
	onFinalize  func()
	finalizeErr error
}

func (*runtimeRecordingsRecorderStub) BindRecordingLifecycle(
	recordings.RecordingLifecycle,
	recordings.CanonicalEventScope,
) error {
	return nil
}

func (*runtimeRecordingsRecorderStub) Start(context.Context)               {}
func (*runtimeRecordingsRecorderStub) Stop()                               {}
func (*runtimeRecordingsRecorderStub) RecordEvent(interfaces.FactoryEvent) {}
func (*runtimeRecordingsRecorderStub) RecordError(error)                   {}
func (*runtimeRecordingsRecorderStub) Finish(time.Time)                    {}
func (*runtimeRecordingsRecorderStub) Flush() error                        { return nil }
func (*runtimeRecordingsRecorderStub) Err() error                          { return nil }
func (recorder *runtimeRecordingsRecorderStub) Finalize(time.Time) error {
	if recorder != nil && recorder.onFinalize != nil {
		recorder.onFinalize()
	}
	return recorder.finalizeErr
}

var _ recordings.RuntimeRecorder = (*runtimeRecordingsRecorderStub)(nil)

type controlledCompilation struct{}

func (controlledCompilation) Compile(context.Context, factory.OrchestrationCompileRequest) (factory.OrchestrationCompileResult, error) {
	return factory.OrchestrationCompileResult{Kind: factory.OrchestrationKindPetri}, nil
}
func (controlledCompilation) CompilePetriNet(context.Context, factory.OrchestrationCompileRequest) (*state.Net, error) {
	return &state.Net{}, nil
}

type controlledEngine struct {
	factoryhost.Engine
	factory.Service
	workflow *factory_context.FactoryContext
}

func (e *controlledEngine) WorkflowContext() *factory_context.FactoryContext { return e.workflow }
func (*controlledEngine) GetEngineStateSnapshot(context.Context) (*legacysnapshot.Snapshot, error) {
	return &legacysnapshot.Snapshot{}, nil
}

type controlledEngineCall struct {
	submit   recordings.SubmissionRecorder
	dispatch recordings.DispatchRecorder
	project  factory.WorldStateProjector
	history  recordings.RuntimeLedger
}
type controlledEngineOpening struct{ calls []controlledEngineCall }

func (opening *controlledEngineOpening) Open(
	net *state.Net,
	runtimeScheduler scheduler.Scheduler,
	statelessService workers.Service,
	workerSessionsService workersessions.Service,
	workerAttempts factory.WorkerAttemptOpener,
	runtimeDefinitions interfaces.RuntimeDefinitionLookup,
	invocationFileReader interfaces.FileReader,
	workflowContext *factory_context.FactoryContext,
	publicSessionID string,
	runtimeMode interfaces.RuntimeMode,
	logger factory.Logger,
	clock factory.Clock,
	workerAttemptScheduler platformclock.TimerSource,
	inlineDispatch bool,
	eventHistory recordings.RuntimeLedger,
	recordingID string,
	runtimeID string,
	worldStateProjector factory.WorldStateProjector,
	restoredWorldState *interfaces.FactoryWorldState,
	skipRestoredDispatchReconciliation bool,
	submissionRecorder recordings.SubmissionRecorder,
	factoryEventRecorder factory.FactoryEventRecorder,
	submissionHooks []factory.SubmissionHook,
	dispatchRecorder recordings.DispatchRecorder,
	completionRecorder factory.CompletionRecorder,
	petriMutationRecorder factory.PetriMutationRecorder,
	completionDeliveryPlanner factory.CompletionDeliveryPlanner,
) (factoryhost.Engine, error) {
	opening.calls = append(opening.calls, controlledEngineCall{submissionRecorder, dispatchRecorder, worldStateProjector, eventHistory})
	return &controlledEngine{workflow: workflowContext}, nil
}

type controlledInitialAssembly struct {
	failure    error
	mutations  []factory.PetriMutationRecorder
	progress   []workers.ProgressPublisher
	runtimeIDs []string
}

func (a *controlledInitialAssembly) Assemble(
	ctx context.Context,
	defaultWorkerModelProvider string,
	defaultWorkerModel string,
	applyOperatorDefaults bool,
	recordPath string,
	workflowID string,
	defaultSessionID string,
	metricsSessionID string,
	mockWorkersConfig *workers.MockWorkersConfig,
	runtimeMode interfaces.RuntimeMode,
	runtimeScheduler factory.Scheduler,
	inlineDispatch bool,
	runtimeLogDir string,
	runtimeLogConfig factory.RuntimeLogStorageConfig,
	runtimeFileLoggingPolicy factory.RuntimeFileLoggingPolicy,
	runtimeMetricsPolicy factory.RuntimeMetricsPolicy,
	runtimeMetricsDir string,
	runtimeMetricsConfig factory.RuntimeMetricsStorageConfig,
	recordFlushInterval time.Duration,
	backendScopeID string,
	factoryRunnerID string,
	verbose bool,
	skipBuiltInPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	clock factory.Clock,
	baseLogger *zap.Logger,
	publishRuntimeStreams bool,
	observations factory.SessionObservations,
	dir string,
	factoryRootDir string,
	executionBaseDir string,
	loadedFactory interfaces.MutableLoadedFactorySource,
	runtimeInstanceID string,
	replayArtifact *interfaces.ReplayArtifact,
	resumeInput *recordings.LoadResumeInputResult,
	restoredWorldState *interfaces.FactoryWorldState,
	restoredEventHistory []interfaces.FactoryEvent,
	serviceMode bool,
) (*factory.RuntimeInitialOpening, error) {
	a.mutations = append(a.mutations, observations.RecordPetriTokenMutations)
	a.progress = append(a.progress, observations.PublishWorkerProgress)
	a.runtimeIDs = append(a.runtimeIDs, runtimeInstanceID)
	return &factory.RuntimeInitialOpening{Activation: &factory.RuntimeActivation{}, Spec: factory.SessionBuildSpec{FolderPath: factoryRootDir, LoadedFactoryCfg: loadedFactory}}, a.failure
}
