package run

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

func isPackagedTTSRun(cfg RunConfig) bool {
	return strings.TrimSpace(cfg.NamedFactoryName) == tts.PackagedFactoryName
}

func logPackagedTTSInvocationStart(cfg RunConfig) {
	if !isPackagedTTSRun(cfg) {
		return
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	fields := []zap.Field{
		zap.String("packaged_factory_name", tts.PackagedFactoryName),
		zap.String("tts_backend", tts.BackendRuntimeLabel()),
		zap.String("readiness_outcome", tts.FailureClassLoading),
	}
	if resolution := cfg.NamedFactoryResolution; resolution != nil {
		fields = append(fields,
			zap.String("named_factory_resolution_source", string(resolution.Source)),
			zap.String("named_factory_dir", resolution.FactoryDir),
		)
	}
	logger.Info("packaged tts invocation started", fields...)
	recordPackagedTTSInvocationMetric(cfg, tts.MetricPackagedFactoryAttempts, nil)
}

func observePackagedTTSInvocationResult(cfg RunConfig, result apisurface.FactoryInvocationResult, err error) {
	if !isPackagedTTSRun(cfg) {
		return
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	if err != nil {
		failureClass := packagedTTSFailureClassFromError(err)
		recordPackagedTTSInvocationMetric(cfg, tts.MetricPackagedFactoryFailure, map[string]string{
			"failure_class": failureClass,
		})
		if failureClass == tts.FailureClassModelNotReady {
			recordPackagedTTSInvocationMetric(cfg, tts.MetricPackagedFactoryNotReady, nil)
		}
		logger.Warn(
			"packaged tts invocation failed",
			zap.String("packaged_factory_name", tts.PackagedFactoryName),
			zap.String("tts_backend", tts.BackendRuntimeLabel()),
			zap.String("failure_class", failureClass),
			zap.String("readiness_outcome", failureClass),
			zap.String("error_code", packagedTTSInvocationErrorCode(err)),
		)
		return
	}
	if result.Status != factoryapi.InvocationTerminalStatusCompleted {
		return
	}
	recordPackagedTTSInvocationMetric(cfg, tts.MetricPackagedFactorySuccess, map[string]string{
		"readiness_outcome": tts.FailureClassSuccess,
	})
	logger.Info(
		"packaged tts invocation completed",
		zap.String("packaged_factory_name", tts.PackagedFactoryName),
		zap.String("tts_backend", tts.BackendRuntimeLabel()),
		zap.String("readiness_outcome", tts.FailureClassSuccess),
		zap.String("request_id", result.RequestID),
		zap.String("trace_id", result.TraceID),
	)
}

func packagedTTSFailureClassFromError(err error) string {
	code := packagedTTSInvocationErrorCode(err)
	switch code {
	case tts.InvocationErrorCodeModelNotReady:
		return tts.FailureClassModelNotReady
	case tts.InvocationErrorCodeGenerationFailed:
		return tts.FailureClassGenerationFailed
	default:
		return tts.FailureClassGenerationFailed
	}
}

func packagedTTSInvocationErrorCode(err error) string {
	if cliErr, ok := err.(invocationCLIError); ok {
		return strings.TrimSpace(cliErr.Code)
	}
	return ""
}

func recordPackagedTTSInvocationMetric(cfg RunConfig, name string, extra map[string]string) {
	labels := map[string]string{
		"packaged_factory": tts.PackagedFactoryName,
	}
	for key, value := range extra {
		labels[key] = value
	}
	recordInvocationMetric(cfg.InvocationMetricsRecorder, service.InvocationMetric{
		Name:   name,
		Labels: labels,
	})
}
