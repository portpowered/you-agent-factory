package testdeps_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/testdeps"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/metrics"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap/zapcore"
)

func TestDefaultObservability_UsesNoopLoggerAndMetricsEmitter(t *testing.T) {
	t.Parallel()

	obs := testdeps.Default()
	if _, ok := obs.Logger.(logging.NoopLogger); !ok {
		t.Fatalf("Logger = %T, want logging.NoopLogger", obs.Logger)
	}
	if _, ok := obs.MetricsEmitter.(metrics.NoopEmitter); !ok {
		t.Fatalf("MetricsEmitter = %T, want metrics.NoopEmitter", obs.MetricsEmitter)
	}
	if obs.ZapLogger == nil {
		t.Fatal("ZapLogger = nil, want non-nil nop logger")
	}
}

func TestDefaultObservability_DiscardsRoutineLogAndMetricCalls(t *testing.T) {
	t.Chdir(t.TempDir())

	obs := testdeps.Default()
	obs.Logger.Info("routine test log", "key", "value")
	obs.Logger.Warn("routine warning")
	obs.Logger.Error("routine error")
	obs.Logger.Debug("routine debug")
	obs.Logger.Verbose("routine verbose", "detail", true)

	fields := metrics.Fields{DispatchID: "dispatch-1", Workstation: "review"}
	if err := obs.MetricsEmitter.Counter(context.Background(), "dispatch.started", 1, fields); err != nil {
		t.Fatalf("Counter: %v", err)
	}
	if err := obs.MetricsEmitter.Gauge(context.Background(), "queue.depth", 2, fields); err != nil {
		t.Fatalf("Gauge: %v", err)
	}
	if err := obs.MetricsEmitter.Sample(context.Background(), "dispatch.duration", 12.5, "ms", fields); err != nil {
		t.Fatalf("Sample: %v", err)
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("default observability wrote files: %v", entries)
	}
}

func TestDefaultObservability_ZapLoggerIsNoop(t *testing.T) {
	t.Parallel()

	quiet := testdeps.Default()
	if quiet.ZapLogger == nil {
		t.Fatal("ZapLogger = nil")
	}
	quiet.ZapLogger.Info("discarded by nop logger")
}

func TestApplyFactoryServiceConfig_SetsQuietDefaultsWithoutOverridingExplicitValues(t *testing.T) {
	t.Parallel()

	explicitLogger := testdeps.Default().ZapLogger
	cfg := &service.FactoryServiceConfig{
		Logger:                   explicitLogger,
		RuntimeFileLoggingPolicy: service.RuntimeFileLoggingPolicyEnabled,
		RuntimeMetricsPolicy:     service.RuntimeMetricsPolicyEnabled,
	}
	testdeps.Default().ApplyFactoryServiceConfig(cfg)

	if cfg.Logger != explicitLogger {
		t.Fatal("ApplyFactoryServiceConfig replaced explicit logger")
	}
	if cfg.RuntimeFileLoggingPolicy != service.RuntimeFileLoggingPolicyEnabled {
		t.Fatalf("RuntimeFileLoggingPolicy = %q, want enabled", cfg.RuntimeFileLoggingPolicy)
	}
	if cfg.RuntimeMetricsPolicy != service.RuntimeMetricsPolicyEnabled {
		t.Fatalf("RuntimeMetricsPolicy = %q, want enabled", cfg.RuntimeMetricsPolicy)
	}

	emptyCfg := &service.FactoryServiceConfig{}
	testdeps.Default().ApplyFactoryServiceConfig(emptyCfg)
	if emptyCfg.Logger == nil {
		t.Fatal("expected default zap logger to be assigned")
	}
	if emptyCfg.RuntimeFileLoggingPolicy != service.RuntimeFileLoggingPolicyDisabled {
		t.Fatalf("RuntimeFileLoggingPolicy = %q, want disabled", emptyCfg.RuntimeFileLoggingPolicy)
	}
	if emptyCfg.RuntimeMetricsPolicy != service.RuntimeMetricsPolicyDisabled {
		t.Fatalf("RuntimeMetricsPolicy = %q, want disabled", emptyCfg.RuntimeMetricsPolicy)
	}
}

func TestQuietFactoryServiceConfig_RepresentativeOrdinaryBuildPatternsStayQuiet(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	metricsDir := t.TempDir()
	logDir := t.TempDir()

	cfg := testdeps.QuietFactoryServiceConfig(&service.FactoryServiceConfig{
		Dir:                                     dir,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		RuntimeMetricsDir:                       metricsDir,
		RuntimeLogDir:                           logDir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if cfg.Logger == nil {
		t.Fatal("expected quiet factory service config to assign noop logger")
	}
	if cfg.RuntimeFileLoggingPolicy != service.RuntimeFileLoggingPolicyDisabled {
		t.Fatalf("RuntimeFileLoggingPolicy = %q, want disabled", cfg.RuntimeFileLoggingPolicy)
	}
	if cfg.RuntimeMetricsPolicy != service.RuntimeMetricsPolicyDisabled {
		t.Fatalf("RuntimeMetricsPolicy = %q, want disabled", cfg.RuntimeMetricsPolicy)
	}

	svc, err := service.BuildFactoryService(context.Background(), cfg)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	bundle := svc.CurrentRuntimeBundle()
	if bundle == nil {
		t.Fatal("expected startup runtime bundle")
	}
	if bundle.MetricsSink != nil {
		t.Fatal("expected disabled runtime metrics policy to skip metrics sink creation")
	}

	var metricFiles []string
	err = filepath.WalkDir(metricsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		metricFiles = append(metricFiles, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", metricsDir, err)
	}
	if len(metricFiles) != 0 {
		t.Fatalf("metrics files = %v, want none", metricFiles)
	}

	var logFiles []string
	err = filepath.WalkDir(logDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		logFiles = append(logFiles, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", logDir, err)
	}
	if len(logFiles) != 0 {
		t.Fatalf("runtime log files = %v, want none", logFiles)
	}
}

func TestApplyFactoryServiceConfig_BuildFactoryServiceDoesNotCreateMetricsFiles(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	metricsDir := t.TempDir()

	cfg := &service.FactoryServiceConfig{
		Dir:                                     dir,
		RuntimeMetricsDir:                       metricsDir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	}
	testdeps.Default().ApplyFactoryServiceConfig(cfg)

	svc, err := service.BuildFactoryService(context.Background(), cfg)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc == nil {
		t.Fatal("BuildFactoryService returned nil service")
	}
	bundle := svc.CurrentRuntimeBundle()
	if bundle == nil {
		t.Fatal("expected startup runtime bundle")
	}
	if bundle.MetricsSink != nil {
		t.Fatal("expected disabled runtime metrics policy to skip metrics sink creation")
	}

	var metricFiles []string
	err = filepath.WalkDir(metricsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		metricFiles = append(metricFiles, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", metricsDir, err)
	}
	if len(metricFiles) != 0 {
		t.Fatalf("metrics files = %v, want none", metricFiles)
	}
}

func TestCapturingObservability_RecordsLogAndMetricEmissions(t *testing.T) {
	t.Parallel()

	obs, capture := testdeps.Capturing(zapcore.InfoLevel)
	obs.Logger.Info("observability under test", "detail", "value")

	fields := metrics.Fields{DispatchID: "dispatch-1", Workstation: "review"}
	if err := obs.MetricsEmitter.Counter(context.Background(), "dispatch.started", 1, fields); err != nil {
		t.Fatalf("Counter: %v", err)
	}
	if err := obs.MetricsEmitter.Sample(context.Background(), "dispatch.duration", 12.5, "ms", fields); err != nil {
		t.Fatalf("Sample: %v", err)
	}

	logs := capture.ObservedLogs.FilterMessage("observability under test").All()
	if len(logs) != 1 {
		t.Fatalf("observed logs = %d, want 1", len(logs))
	}
	if !capture.Metrics.ContainsCounter("dispatch.started", 1, fields) {
		t.Fatal("expected captured counter emission")
	}
	if !capture.Metrics.ContainsSample("dispatch.duration", 12.5, "ms", fields) {
		t.Fatal("expected captured sample emission")
	}
}

func TestCapturingObservability_DoesNotAffectDefaultQuietObservability(t *testing.T) {
	t.Chdir(t.TempDir())

	obs, capture := testdeps.Capturing(zapcore.InfoLevel)
	obs.Logger.Info("isolated capture log")
	if len(capture.ObservedLogs.All()) != 1 {
		t.Fatalf("capturing logs = %d, want 1", len(capture.ObservedLogs.All()))
	}

	quiet := testdeps.Default()
	quiet.Logger.Info("quiet default log")
	if err := quiet.MetricsEmitter.Counter(context.Background(), "dispatch.started", 1, metrics.Fields{}); err != nil {
		t.Fatalf("Counter: %v", err)
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("default observability wrote files after isolated capture: %v", entries)
	}
}

func TestRecordingInvocationMetrics_CapturesFactoryServiceMetrics(t *testing.T) {
	t.Parallel()

	recorder := testdeps.NewRecordingInvocationMetrics()
	recorder.RecordInvocationMetric(service.InvocationMetric{
		Name:   "runtime.loaded",
		Labels: map[string]string{"identity": "model-a"},
	})

	if !recorder.Contains("runtime.loaded", map[string]string{"identity": "model-a"}) {
		t.Fatal("expected captured invocation metric")
	}
}

func TestProductionFactoryServiceConfig_PreservesEnabledObservabilityPoliciesWithoutTestDeps(t *testing.T) {
	t.Parallel()

	productionCfg := &service.FactoryServiceConfig{}
	if productionCfg.RuntimeFileLoggingPolicy != "" {
		t.Fatalf("RuntimeFileLoggingPolicy = %q, want empty production default", productionCfg.RuntimeFileLoggingPolicy)
	}
	if productionCfg.RuntimeMetricsPolicy != "" {
		t.Fatalf("RuntimeMetricsPolicy = %q, want empty production default", productionCfg.RuntimeMetricsPolicy)
	}
	if productionCfg.Logger != nil {
		t.Fatal("expected unset logger before production wiring")
	}

	quietCfg := testdeps.QuietFactoryServiceConfig(&service.FactoryServiceConfig{})
	if quietCfg.RuntimeFileLoggingPolicy != service.RuntimeFileLoggingPolicyDisabled {
		t.Fatalf("RuntimeFileLoggingPolicy = %q, want disabled after testdeps", quietCfg.RuntimeFileLoggingPolicy)
	}
	if quietCfg.RuntimeMetricsPolicy != service.RuntimeMetricsPolicyDisabled {
		t.Fatalf("RuntimeMetricsPolicy = %q, want disabled after testdeps", quietCfg.RuntimeMetricsPolicy)
	}
	if quietCfg.Logger == nil {
		t.Fatal("expected quiet logger after testdeps")
	}
}

func TestApplyFactoryServiceConfig_PreservesCapturingLoggerAndMetricsRecorder(t *testing.T) {
	t.Parallel()

	obs, capture := testdeps.Capturing(zapcore.WarnLevel)
	recorder := testdeps.NewRecordingInvocationMetrics()
	cfg := &service.FactoryServiceConfig{
		Logger:                    obs.ZapLogger,
		InvocationMetricsRecorder: recorder,
	}
	testdeps.Default().ApplyFactoryServiceConfig(cfg)

	if cfg.Logger != obs.ZapLogger {
		t.Fatal("ApplyFactoryServiceConfig replaced explicit capturing logger")
	}
	if cfg.InvocationMetricsRecorder != recorder {
		t.Fatal("ApplyFactoryServiceConfig replaced explicit metrics recorder")
	}

	obs.ZapLogger.Warn("preserved capturing logger")
	if len(capture.ObservedLogs.FilterMessage("preserved capturing logger").All()) != 1 {
		t.Fatal("expected capturing logger to remain observable after quiet defaults")
	}
}
