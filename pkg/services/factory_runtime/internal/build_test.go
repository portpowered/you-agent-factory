package internal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	factoryruntimeorchestrationowner "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
	ledgerFactory := factory.RuntimeLedgerFactory(func(
		source recordings.InitialStructureSource,
		_ func() time.Time,
		_ interfaces.RuntimeDefinitionLookup,
	) recordings.RuntimeEventLedger {
		capturedSource = source
		return ledger
	})
	recorder := &runtimeRecordingsRecorderStub{}

	bundle, err := testRuntimeFactory().Build(
		context.Background(), dir, dir, "~default",
		"", interfaces.RuntimeModeBatch, false, nil, nil, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, zap.NewNop(), "runtime-recordings-root", "", clockwork.NewFakeClock(),
		"/recordings/session.json", recorder, nil, nil, nil, nil, nil,
		ledgerFactory,
		func(recordings.WorkerEventRecorder, *zap.Logger) (map[string]workers.WorkerExecutor, error) {
			return nil, nil
		},
		testRuntimeWorkers{},
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
		context.Background(), dir, dir, "~default",
		"", interfaces.RuntimeModeBatch, false, nil, nil, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, zap.NewNop(), "runtime-test", "", clockwork.NewFakeClock(), "", nil, nil, nil, nil, nil,
		nil,
		newTestRuntimeLedger,
		func(recordings.WorkerEventRecorder, *zap.Logger) (map[string]workers.WorkerExecutor, error) {
			return nil, nil
		},
		testRuntimeWorkers{},
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
		context.Background(), dir, dir, "~default",
		"", interfaces.RuntimeModeBatch, false, nil, nil, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		"", "", metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, zap.NewNop(), "runtime-observability", "", clockwork.NewFakeClock(), "", nil, nil, nil, nil, nil,
		nil,
		newTestRuntimeLedger,
		func(recordings.WorkerEventRecorder, *zap.Logger) (map[string]workers.WorkerExecutor, error) {
			return nil, nil
		},
		testRuntimeWorkers{},
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
		context.Background(), dir, dir, "~default",
		"", interfaces.RuntimeModeBatch, false, nil, nil, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled,
		metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, zap.NewNop(), "runtime-disabled", "", clockwork.NewFakeClock(), "", nil, nil, nil, nil, nil,
		nil,
		newTestRuntimeLedger,
		func(recordings.WorkerEventRecorder, *zap.Logger) (map[string]workers.WorkerExecutor, error) {
			return nil, nil
		},
		testRuntimeWorkers{},
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

type testRuntimeWorkers struct{}

func (testRuntimeWorkers) StartWorkstationPool(
	context.Context,
	workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{}, nil
}

func (testRuntimeWorkers) StopWorkstationPool(
	context.Context,
) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{}, nil
}

func (testRuntimeWorkers) DispatchWorkstation(
	context.Context,
	workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	return workers.WorkstationDispatchResult{}, nil
}

func (testRuntimeWorkers) CancelWorkstationDispatch(
	context.Context,
	workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	return workers.WorkstationDispatchCancelResult{}, nil
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
		nil, testRuntimeLoggerFactory, nil, nil,
		testRuntimeID, testRuntimeID, localRuntimeFiles{}, localRuntimeFiles{}, filepath.WalkDir,
		testOrchestrationCompilation(),
	)
}

func testRuntimeFactoryWithSinks(logDir, metricsDir string) *factoryinternal.RuntimeFactory {
	return factoryinternal.NewRuntimeFactory(
		nil, testRuntimeLoggerFactory,
		testRuntimeLogFactory(logDir), testRuntimeMetricsFactory(metricsDir),
		testRuntimeID, testRuntimeID, localRuntimeFiles{}, localRuntimeFiles{}, filepath.WalkDir,
		testOrchestrationCompilation(),
	)
}

func newTestRuntimeLedger(
	recordings.InitialStructureSource,
	func() time.Time,
	interfaces.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return &recordingfixtures.ScriptedRuntimeLedger{}
}

func testRuntimeLoggerFactory(*zap.Logger, bool) factory.Logger { return factory.NoopLogger{} }

type testRuntimeLogSink struct {
	logger   *zap.Logger
	artifact factory.RuntimeLogArtifact
}

func (sink *testRuntimeLogSink) Logger() *zap.Logger                  { return sink.logger }
func (sink *testRuntimeLogSink) Artifact() factory.RuntimeLogArtifact { return sink.artifact }
func (sink *testRuntimeLogSink) Close() error                         { return nil }

func testRuntimeLogFactory(root string) factory.RuntimeLogSinkFactory {
	return func(base *zap.Logger, _ string, _ string, config factory.RuntimeLogStorageConfig) (factory.RuntimeLogSink, error) {
		return &testRuntimeLogSink{logger: base, artifact: factory.RuntimeLogArtifact{
			Path: filepath.Join(root, "runtime.log"), RootDir: root,
			StartTimeUTC: time.Now().UTC(), Config: config,
		}}, nil
	}
}

type testRuntimeMetricsSink struct {
	artifact factory.RuntimeMetricsArtifact
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
func (s *testRuntimeMetricsSink) Close() error { return nil }
func (s *testRuntimeMetricsSink) Path() string { return s.artifact.Path }
func (s *testRuntimeMetricsSink) Artifact() factory.RuntimeMetricsArtifact {
	return s.artifact
}

func testRuntimeMetricsFactory(root string) factory.RuntimeMetricsSinkFactory {
	return func(
		factory.RuntimeMetricsScope,
		string,
		factory.RuntimeMetricsStorageConfig,
	) (factory.RuntimeMetricsSink, error) {
		return &testRuntimeMetricsSink{artifact: factory.RuntimeMetricsArtifact{
			Path: filepath.Join(root, "runtime-metrics.log"), RootDir: root,
			StartTimeUTC: time.Now().UTC(),
		}}, nil
	}
}

type runtimeRecordingsRecorderStub struct{}

func (*runtimeRecordingsRecorderStub) BindRecordingService(
	recordings.Service,
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
func (*runtimeRecordingsRecorderStub) Finalize(time.Time) error            { return nil }

var _ recordings.RuntimeRecorder = (*runtimeRecordingsRecorderStub)(nil)
