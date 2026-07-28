package internal

import (
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"go.uber.org/zap"
)

func TestRuntimeSinkOpeningFailsClosedWithoutInjectedFactories(t *testing.T) {
	t.Parallel()
	if _, _, err := buildRuntimeLogSink(nil, RuntimeFileLoggingPolicyEnabled, "/logs", factory.RuntimeLogStorageConfig{}, zap.NewNop(), "runtime-1"); err == nil || !strings.Contains(err.Error(), "factory is required") {
		t.Fatalf("log opening error = %v, want required factory", err)
	}
	if _, err := buildRuntimeMetricsSink(nil, RuntimeMetricsPolicyEnabled, "/metrics", factory.RuntimeMetricsStorageConfig{}, "session", "runtime-1", "/folder", "/factory"); err == nil || !strings.Contains(err.Error(), "factory is required") {
		t.Fatalf("metrics opening error = %v, want required factory", err)
	}
}

func TestRuntimeSinkOpeningDisabledPolicyNeedsNoFactory(t *testing.T) {
	t.Parallel()
	logSink, runtimeID, err := buildRuntimeLogSink(nil, RuntimeFileLoggingPolicyDisabled, "", factory.RuntimeLogStorageConfig{}, zap.NewNop(), "runtime-1")
	if err != nil || logSink != nil || runtimeID != "runtime-1" {
		t.Fatalf("disabled log opening = (%v, %q, %v)", logSink, runtimeID, err)
	}
	metricsSink, err := buildRuntimeMetricsSink(nil, RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{}, "session", "runtime-1", "", "")
	if err != nil || metricsSink != nil {
		t.Fatalf("disabled metrics opening = (%v, %v)", metricsSink, err)
	}
}

func TestRuntimeLogOpeningRejectsAmbientIdentityAndLoggerFallbacks(t *testing.T) {
	t.Parallel()
	factoryFn := func(*zap.Logger, string, string, factory.RuntimeLogStorageConfig) (factory.RuntimeLogSink, error) {
		return nil, nil
	}
	if _, _, err := buildRuntimeLogSink(factoryFn, RuntimeFileLoggingPolicyEnabled, "/logs", factory.RuntimeLogStorageConfig{}, nil, "runtime-1"); err == nil || !strings.Contains(err.Error(), "base logger is required") {
		t.Fatalf("nil logger error = %v", err)
	}
	if _, _, err := buildRuntimeLogSink(factoryFn, RuntimeFileLoggingPolicyEnabled, "/logs", factory.RuntimeLogStorageConfig{}, zap.NewNop(), ""); err == nil || !strings.Contains(err.Error(), "runtime instance ID is required") {
		t.Fatalf("empty runtime ID error = %v", err)
	}
}
