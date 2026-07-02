package testdeps_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/internal/metrics"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/testutil/testdeps"
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
