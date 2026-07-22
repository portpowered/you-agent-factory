package testdeps_test

import (
	"context"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/testdeps"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"go.uber.org/zap/zapcore"
)

func TestDefaultObservability_UsesNoopLoggerAndMetricsEmitter(t *testing.T) {
	t.Parallel()

	obs := testdeps.Default()
	if _, ok := obs.Logger.(logging.NoopLogger); !ok {
		t.Fatalf("Logger = %T, want logging.NoopLogger", obs.Logger)
	}
	if _, ok := obs.MetricsEmitter.(factoryruntime.NoopEmitter); !ok {
		t.Fatalf("MetricsEmitter = %T, want factoryruntime.NoopEmitter", obs.MetricsEmitter)
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

	fields := factoryruntime.Fields{DispatchID: "dispatch-1", Workstation: "review"}
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
	cfg := &testdeps.FactoryServiceObservabilityConfig{
		Logger:                   explicitLogger,
		RuntimeFileLoggingPolicy: factoryruntime.RuntimeFileLoggingPolicyEnabled,
		RuntimeMetricsPolicy:     factoryruntime.RuntimeMetricsPolicyEnabled,
	}
	testdeps.Default().ApplyFactoryServiceConfig(cfg)

	if cfg.Logger != explicitLogger {
		t.Fatal("ApplyFactoryServiceConfig replaced explicit logger")
	}
	if cfg.RuntimeFileLoggingPolicy != factoryruntime.RuntimeFileLoggingPolicyEnabled {
		t.Fatalf("RuntimeFileLoggingPolicy = %q, want enabled", cfg.RuntimeFileLoggingPolicy)
	}
	if cfg.RuntimeMetricsPolicy != factoryruntime.RuntimeMetricsPolicyEnabled {
		t.Fatalf("RuntimeMetricsPolicy = %q, want enabled", cfg.RuntimeMetricsPolicy)
	}

	emptyCfg := &testdeps.FactoryServiceObservabilityConfig{}
	testdeps.Default().ApplyFactoryServiceConfig(emptyCfg)
	if emptyCfg.Logger == nil {
		t.Fatal("expected default zap logger to be assigned")
	}
	if emptyCfg.RuntimeFileLoggingPolicy != factoryruntime.RuntimeFileLoggingPolicyDisabled {
		t.Fatalf("RuntimeFileLoggingPolicy = %q, want disabled", emptyCfg.RuntimeFileLoggingPolicy)
	}
	if emptyCfg.RuntimeMetricsPolicy != factoryruntime.RuntimeMetricsPolicyDisabled {
		t.Fatalf("RuntimeMetricsPolicy = %q, want disabled", emptyCfg.RuntimeMetricsPolicy)
	}
}

func TestQuietFactoryServiceConfig_RepresentativeOrdinaryBuildPatternsStayQuiet(t *testing.T) {
	metricsDir := t.TempDir()
	logDir := t.TempDir()

	_ = metricsDir
	_ = logDir
	cfg := testdeps.QuietFactoryServiceConfig(&testdeps.FactoryServiceObservabilityConfig{})
	if cfg.Logger == nil {
		t.Fatal("expected quiet factory service config to assign noop logger")
	}
	if cfg.RuntimeFileLoggingPolicy != factoryruntime.RuntimeFileLoggingPolicyDisabled {
		t.Fatalf("RuntimeFileLoggingPolicy = %q, want disabled", cfg.RuntimeFileLoggingPolicy)
	}
	if cfg.RuntimeMetricsPolicy != factoryruntime.RuntimeMetricsPolicyDisabled {
		t.Fatalf("RuntimeMetricsPolicy = %q, want disabled", cfg.RuntimeMetricsPolicy)
	}
}

func TestCapturingObservability_RecordsLogAndMetricEmissions(t *testing.T) {
	t.Parallel()

	obs, capture := testdeps.Capturing(zapcore.InfoLevel)
	obs.Logger.Info("observability under test", "detail", "value")

	fields := factoryruntime.Fields{DispatchID: "dispatch-1", Workstation: "review"}
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
	if err := quiet.MetricsEmitter.Counter(context.Background(), "dispatch.started", 1, factoryruntime.Fields{}); err != nil {
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
	recorder.RecordInvocationMetric(factorysessions.InvocationMetric{
		Name:   "runtime.loaded",
		Labels: map[string]string{"identity": "model-a"},
	})

	if !recorder.Contains("runtime.loaded", map[string]string{"identity": "model-a"}) {
		t.Fatal("expected captured invocation metric")
	}
}

func TestProductionFactoryServiceConfig_PreservesEnabledObservabilityPoliciesWithoutTestDeps(t *testing.T) {
	t.Parallel()

	productionCfg := &testdeps.FactoryServiceObservabilityConfig{}
	if productionCfg.RuntimeFileLoggingPolicy != "" {
		t.Fatalf("RuntimeFileLoggingPolicy = %q, want empty production default", productionCfg.RuntimeFileLoggingPolicy)
	}
	if productionCfg.RuntimeMetricsPolicy != "" {
		t.Fatalf("RuntimeMetricsPolicy = %q, want empty production default", productionCfg.RuntimeMetricsPolicy)
	}
	if productionCfg.Logger != nil {
		t.Fatal("expected unset logger before production wiring")
	}

	quietCfg := testdeps.QuietFactoryServiceConfig(&testdeps.FactoryServiceObservabilityConfig{})
	if quietCfg.RuntimeFileLoggingPolicy != factoryruntime.RuntimeFileLoggingPolicyDisabled {
		t.Fatalf("RuntimeFileLoggingPolicy = %q, want disabled after testdeps", quietCfg.RuntimeFileLoggingPolicy)
	}
	if quietCfg.RuntimeMetricsPolicy != factoryruntime.RuntimeMetricsPolicyDisabled {
		t.Fatalf("RuntimeMetricsPolicy = %q, want disabled after testdeps", quietCfg.RuntimeMetricsPolicy)
	}
	if quietCfg.Logger == nil {
		t.Fatal("expected quiet logger after testdeps")
	}
}

func TestApplyFactoryServiceConfig_PreservesCapturingLogger(t *testing.T) {
	t.Parallel()

	obs, capture := testdeps.Capturing(zapcore.WarnLevel)
	cfg := &testdeps.FactoryServiceObservabilityConfig{
		Logger: obs.ZapLogger,
	}
	testdeps.Default().ApplyFactoryServiceConfig(cfg)

	if cfg.Logger != obs.ZapLogger {
		t.Fatal("ApplyFactoryServiceConfig replaced explicit capturing logger")
	}
	obs.ZapLogger.Warn("preserved capturing logger")
	if len(capture.ObservedLogs.FilterMessage("preserved capturing logger").All()) != 1 {
		t.Fatal("expected capturing logger to remain observable after quiet defaults")
	}
}
