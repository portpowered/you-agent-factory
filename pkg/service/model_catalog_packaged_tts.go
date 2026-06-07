package service

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
	"go.uber.org/zap"
)

func (fs *FactoryService) recordPackagedTTSInvocationMetric(
	name string,
	source invocations.InputSourceLabel,
	extra map[string]string,
) {
	labels := mergeMetricLabels(
		inputMetricLabels(source),
		map[string]string{"packaged_factory": tts.PackagedFactoryName},
		extra,
	)
	fs.recordInvocationMetric(name, labels)
}

func (fs *FactoryService) handlePackagedTTSInvocationFailure(
	sessionID string,
	input sessionInvocationWaitInput,
	failure *tts.InvocationFailure,
) apisurface.FactoryInvocationResult {
	result := apisurface.FactoryInvocationResult{
		RequestID: input.RequestID,
		TraceID:   input.TraceID,
		Status:    factoryapi.InvocationTerminalStatusFailed,
		ErrorCode: failure.ErrorCode,
		Message:   failure.Message,
	}
	fs.recordInvocationMetric(invocationMetricFailure, inputMetricLabels(input.InputSource))
	fs.recordPackagedTTSInvocationMetric(tts.MetricPackagedFactoryFailure, input.InputSource, map[string]string{
		"failure_class": failure.FailureClass,
	})
	if failure.FailureClass == tts.FailureClassModelNotReady {
		fs.recordPackagedTTSInvocationMetric(tts.MetricPackagedFactoryNotReady, input.InputSource, nil)
	}
	fs.logger.Warn(
		"packaged tts invocation failed",
		packagedTTSInvocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
			zap.String("request_id", input.RequestID),
			zap.String("trace_id", input.TraceID),
			zap.String("status", string(result.Status)),
			zap.String("error_code", result.ErrorCode),
			zap.String("failure_class", failure.FailureClass),
			zap.String("readiness_outcome", failure.FailureClass),
		)...,
	)
	return result
}

func (fs *FactoryService) logPackagedTTSInvocationLoading(
	sessionID string,
	input sessionInvocationWaitInput,
) {
	fs.logger.Info(
		"packaged tts invocation loading",
		packagedTTSInvocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
			zap.String("request_id", input.RequestID),
			zap.String("trace_id", input.TraceID),
			zap.String("readiness_outcome", tts.FailureClassLoading),
		)...,
	)
}

func (fs *FactoryService) logPackagedTTSInvocationCompleted(
	sessionID string,
	input sessionInvocationWaitInput,
	selection invocations.PrimaryResultSelection,
) {
	fs.recordPackagedTTSInvocationMetric(tts.MetricPackagedFactorySuccess, input.InputSource, map[string]string{
		"readiness_outcome": tts.FailureClassSuccess,
	})
	fs.logger.Info(
		"packaged tts invocation completed",
		packagedTTSInvocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
			zap.String("request_id", input.RequestID),
			zap.String("trace_id", input.TraceID),
			zap.String("status", string(factoryapi.InvocationTerminalStatusCompleted)),
			zap.String("resolved_work_id", selection.WorkID),
			zap.String("resolved_work_type", selection.WorkTypeName),
			zap.String("readiness_outcome", tts.FailureClassSuccess),
		)...,
	)
}

func packagedTTSInvocationLogFields(
	sessionID string,
	source invocations.InputSourceLabel,
	cfg *interfaces.InvocationReturnConfig,
	extra ...zap.Field,
) []zap.Field {
	fields := invocationLogFields(sessionID, source, cfg,
		zap.String("packaged_factory_name", tts.PackagedFactoryName),
		zap.String("tts_backend", tts.BackendRuntimeLabel()),
	)
	return append(fields, extra...)
}
