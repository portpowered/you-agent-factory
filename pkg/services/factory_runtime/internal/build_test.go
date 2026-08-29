package internal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	factoryruntimeorchestrationowner "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"go.uber.org/zap"
)

func TestBuild_ConstructsRecordingsRootLedgerAndHostingCapabilities(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	ledger := &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "runtime-recordings-root"}
	var capturedSource recordings.InitialStructureSource
	recorder := &runtimeRecordingsRecorderStub{}
	runtimeOpening := &testRuntimeOpeningStub{
		ledger:         ledger,
		recorder:       recorder,
		capturedSource: &capturedSource,
	}

	bundle, err := testRuntimeFactory().Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-recordings-root", "", clockwork.NewFakeClock(),
		"/recordings/session.json", nil, nil, false, nil, nil, nil, nil,
		runtimeOpening,
		testRuntimeWorkers{},
		testRuntimeWorkerSessionsFactory(t),
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if capturedSource == nil {
		t.Fatal("ledger factory InitialStructureSource = nil")
	}
	if bundle.RecordingLedger() != ledger {
		t.Fatalf("RecordingLedger = %T, want injected root ledger", bundle.RecordingLedger())
	}
	if bundle.Recording != recorder {
		t.Fatalf("Recording = %T, want injected RuntimeRecorder", bundle.Recording)
	}
	if bundle.StreamGeneration() != "runtime-recordings-root" {
		t.Fatalf("stream generation = %q, want runtime-recordings-root", bundle.StreamGeneration())
	}
}

func TestBuild_ConstructsRunnableBundleWithoutRootService(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	bundle, err := testRuntimeFactory().Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-test", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeOpening(newTestRuntimeLedger),
		testRuntimeWorkers{},
		testRuntimeWorkerSessionsFactory(t),
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle = nil")
	}
	if bundle.Factory == nil {
		t.Fatal("bundle.Factory = nil, want runnable factory runtime")
	}
	if bundle.EventHistory == nil {
		t.Fatal("bundle.EventHistory = nil")
	}
	if bundle.Net == nil {
		t.Fatal("bundle.Net = nil")
	}
}

func TestBuild_FinalizesRecordingBeforeClosingRuntimeSinksOnPartialFailure(t *testing.T) {
	dir := t.TempDir()
	logDir := t.TempDir()
	metricsDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	events := make([]string, 0, 3)
	recorder := &runtimeRecordingsRecorderStub{onFinalize: func() {
		events = append(events, "recording.finalize")
	}}
	runtimeOpening := &testRuntimeOpeningStub{
		ledger:   &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "partial-runtime"},
		recorder: recorder,
	}
	_, err = testRuntimeFactoryWithSinkCallbacks(logDir, metricsDir,
		func() { events = append(events, "log.close") },
		func() { events = append(events, "metrics.close") },
	).Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		"", "", metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "partial-runtime", "", clockwork.NewFakeClock(), "recording.json", nil, nil, false, nil, nil, nil, nil,
		runtimeOpening,
		testRuntimeWorkers{}, failingRuntimeWorkerSessionsFactory(), nil,
	)
	if err == nil {
		t.Fatal("Build succeeded, want partial-opening failure")
	}
	if got, want := events, []string{"recording.finalize", "log.close", "metrics.close"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("partial-opening cleanup order = %#v, want %#v", got, want)
	}
}

func TestBuild_ProductionObservabilityPoliciesEnableRuntimeSinksByDefault(t *testing.T) {
	dir := t.TempDir()
	logDir := t.TempDir()
	metricsDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	bundle, err := testRuntimeFactoryWithSinks(logDir, metricsDir).Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		"", "", metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-observability", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeOpening(newTestRuntimeLedger),
		testRuntimeWorkers{},
		testRuntimeWorkerSessionsFactory(t),
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle = nil")
	}
	if bundle.LogSink == nil {
		t.Fatal("LogSink = nil, want runtime log sink when production policy is unset")
	}
	if bundle.MetricsSink == nil {
		t.Fatal("MetricsSink = nil, want runtime metrics sink when production policy is unset")
	}
	if bundle.LogSink.Artifact().RootDir != logDir {
		t.Fatalf("LogSink root = %q, want %q", bundle.LogSink.Artifact().RootDir, logDir)
	}
	if bundle.MetricsSink.Artifact().RootDir != metricsDir {
		t.Fatalf("MetricsSink artifact root = %q, want %q", bundle.MetricsSink.Artifact().RootDir, metricsDir)
	}
	if filepath.Base(bundle.LogSink.Artifact().Path) == "" {
		t.Fatal("LogSink.Path() = empty")
	}
	if filepath.Base(bundle.MetricsSink.Path()) == "" {
		t.Fatal("MetricsSink.Path() = empty")
	}

	disabledBundle, err := testRuntimeFactory().Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled,
		metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-disabled", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeOpening(newTestRuntimeLedger),
		testRuntimeWorkers{},
		testRuntimeWorkerSessionsFactory(t),
		nil,
	)
	if err != nil {
		t.Fatalf("Build disabled policy: %v", err)
	}
	if disabledBundle == nil {
		t.Fatal("disabled policy bundle = nil")
	}
	if disabledBundle.LogSink != nil {
		t.Fatal("LogSink = non-nil, want nil when runtime file logging is explicitly disabled")
	}
	if disabledBundle.MetricsSink != nil {
		t.Fatal("MetricsSink = non-nil, want nil when runtime metrics policy is explicitly disabled")
	}
}

type testRuntimeWorkers struct{ workers.Service }

func testRuntimeWorkerSessionsFactory(t *testing.T) factory.WorkerSessionsFactory {
	t.Helper()
	return func(execution workers.Service, _ platformclock.Source) (workersessions.Service, error) {
		return &stubWorkerSessionsService{execution: execution}, nil
	}
}

func failingRuntimeWorkerSessionsFactory() factory.WorkerSessionsFactory {
	return func(workers.Service, platformclock.Source) (workersessions.Service, error) {
		return nil, errors.New("worker sessions construction failed")
	}
}

// stubWorkerSessionsService is a minimal workersessions.Service double for
// build-composition tests: Start hands the request straight to the resolved
// Workers execution boundary, mirroring the real cutover seam's shape
// without pulling in the peer worker_sessions implementation package.
type stubWorkerSessionsService struct {
	execution workers.Service
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
	return factoryruntimeorchestrationowner.NewCompilation(testRuntimeID, nil, nil)
}

func testRuntimeFactory() *factoryinternal.RuntimeFactory {
	return factoryinternal.NewRuntimeFactory(
		nil, nil, outputAsPayloadPolicy(), nil, nil, nil, zap.NewNop(), testRuntimeLoggerFactory, nil, nil,
		testRuntimeID, testRuntimeID, localRuntimeFiles{}, localRuntimeFiles{}, filepath.WalkDir,
		testOrchestrationCompilation(),
		nil,
	)
}

func testRuntimeFactoryWithSinks(logDir, metricsDir string) *factoryinternal.RuntimeFactory {
	return testRuntimeFactoryWithSinkCallbacks(logDir, metricsDir, nil, nil)
}

func testRuntimeFactoryWithSinkCallbacks(
	logDir string,
	metricsDir string,
	onLogClose func(),
	onMetricsClose func(),
) *factoryinternal.RuntimeFactory {
	return factoryinternal.NewRuntimeFactory(
		nil, nil, outputAsPayloadPolicy(), nil, nil, nil, zap.NewNop(), testRuntimeLoggerFactory,
		testRuntimeLogOwner{root: logDir, onClose: onLogClose},
		testRuntimeMetricsOwner{root: metricsDir, onClose: onMetricsClose},
		testRuntimeID, testRuntimeID, localRuntimeFiles{}, localRuntimeFiles{}, filepath.WalkDir,
		testOrchestrationCompilation(),
		nil,
	)
}

func outputAsPayloadPolicy() interfaces.WorkPropagationPolicyService {
	return interfaces.WorkPropagationPolicyFunc(func(
		*interfaces.FactoryWorkstationConfig,
	) interfaces.WorkPropagationMode {
		return interfaces.WorkPropagationModeOutputAsPayload
	})
}

func newTestRuntimeLedger(
	recordings.InitialStructureSource,
	func() time.Time,
	interfaces.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return &recordingfixtures.ScriptedRuntimeLedger{}
}

func testRuntimeOpening(
	ledgerFactory func(
		recordings.InitialStructureSource,
		func() time.Time,
		interfaces.RuntimeDefinitionLookup,
	) recordings.RuntimeEventLedger,
) recordings.RuntimeOpening {
	return &testRuntimeOpeningStub{ledgerFactory: ledgerFactory}
}

type testRuntimeOpeningStub struct {
	ledger         recordings.RuntimeEventLedger
	ledgerFactory  func(recordings.InitialStructureSource, func() time.Time, interfaces.RuntimeDefinitionLookup) recordings.RuntimeEventLedger
	recorder       recordings.RuntimeRecorder
	capturedSource *recordings.InitialStructureSource
}

func (opening *testRuntimeOpeningStub) OpenRuntime(
	_ context.Context,
	request recordings.RuntimeScopeRequest,
) (recordings.RuntimeScopeResult, error) {
	if opening.capturedSource != nil {
		*opening.capturedSource = request.Topology
	}
	ledger := opening.ledger
	if opening.ledgerFactory != nil {
		ledger = opening.ledgerFactory(request.Topology, request.Now, request.Definitions)
	}
	return recordings.RuntimeScopeResult{Ledger: ledger, Recorder: opening.recorder}, nil
}

func (*testRuntimeOpeningStub) Projection() recordings.ProjectionService { return nil }

func (*testRuntimeOpeningStub) ReconstructCanonicalFactoryWorldState(
	[]interfaces.FactoryEvent,
	int,
) (recordings.FactoryWorldState, error) {
	return recordings.FactoryWorldState{}, nil
}

func (*testRuntimeOpeningStub) ReplayClock(*recordings.ReplayArtifact) recordings.Clock { return nil }

func (*testRuntimeOpeningStub) ReplayExecution(
	*recordings.ReplayArtifact,
) (providers.Service, platformprocess.CommandRunner, []recordings.ReplayHook, recordings.CompletionDeliveryPlanner, error) {
	return nil, nil, nil, nil, nil
}

func (*testRuntimeOpeningStub) LoadReplayInput(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
	return recordings.LoadReplayInputResult{}, nil
}

func (*testRuntimeOpeningStub) LoadResumeInput(recordings.LoadResumeInputRequest) (recordings.LoadResumeInputResult, error) {
	return recordings.LoadResumeInputResult{}, nil
}

var _ recordings.RuntimeOpening = (*testRuntimeOpeningStub)(nil)

func testRuntimeLoggerFactory(*zap.Logger, bool) factory.Logger { return factory.NoopLogger{} }

type testRuntimeLogSink struct {
	logger   *zap.Logger
	artifact factory.RuntimeLogArtifact
	onClose  func()
}

func (sink *testRuntimeLogSink) Logger() *zap.Logger                  { return sink.logger }
func (sink *testRuntimeLogSink) Artifact() factory.RuntimeLogArtifact { return sink.artifact }
func (sink *testRuntimeLogSink) Close() error {
	if sink != nil && sink.onClose != nil {
		sink.onClose()
	}
	return nil
}

type testRuntimeLogOwner struct {
	root    string
	onClose func()
}

func (owner testRuntimeLogOwner) Open(request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
	return &testRuntimeLogSink{logger: zap.NewNop(), onClose: owner.onClose, artifact: factory.RuntimeLogArtifact{
		Path: filepath.Join(owner.root, request.RuntimeInstanceID+".runtime.log"), RootDir: owner.root,
		StartTimeUTC: time.Now().UTC(), Config: request.Config,
	}}, nil
}

type testRuntimeMetricsSink struct {
	artifact factory.RuntimeMetricsArtifact
	onClose  func()
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
	return nil
}
func (s *testRuntimeMetricsSink) Path() string { return s.artifact.Path }
func (s *testRuntimeMetricsSink) Artifact() factory.RuntimeMetricsArtifact {
	return s.artifact
}

type testRuntimeMetricsOwner struct {
	root    string
	onClose func()
}

func (owner testRuntimeMetricsOwner) Open(request factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) {
	return &testRuntimeMetricsSink{onClose: owner.onClose, artifact: factory.RuntimeMetricsArtifact{
		Path: filepath.Join(owner.root, request.Scope.RuntimeInstanceID+".runtime-metrics.log"), RootDir: owner.root,
		StartTimeUTC: time.Now().UTC(),
	}}, nil
}

type runtimeRecordingsRecorderStub struct {
	onFinalize func()
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
	return nil
}

var _ recordings.RuntimeRecorder = (*runtimeRecordingsRecorderStub)(nil)
