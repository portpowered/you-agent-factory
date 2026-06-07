package run

import (
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

func recordCLIInvocationResolved(cfg RunConfig, source invocations.InputSourceLabel) {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	logger.Info("factory invocation input resolved", zap.String("input_source", string(source)))
}

func recordCLIInvocationFailure(cfg RunConfig, err error) {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	inputErr, ok := err.(*invocations.InputError)
	if !ok {
		return
	}
	if inputErr.Code == invocations.InputErrorCodeSourceConflict {
		recordInvocationMetric(cfg.InvocationMetricsRecorder, service.InvocationMetric{
			Name: "invocation.source_conflict",
			Labels: map[string]string{
				"input_source": "conflict",
			},
		})
		recordInvocationMetric(cfg.InvocationMetricsRecorder, service.InvocationMetric{
			Name: "invocation.failure",
			Labels: map[string]string{
				"input_source": "conflict",
			},
		})
		logger.Warn(
			"factory invocation input resolution failed",
			zap.String("failure_class", "source_conflict"),
			zap.Strings("conflicting_sources", invocationSourceLabels(inputErr.ConflictingSources)),
			zap.String("error_code", string(inputErr.Code)),
		)
		return
	}
	logger.Warn(
		"factory invocation input resolution failed",
		zap.String("failure_class", "input_invalid"),
		zap.String("error_code", string(inputErr.Code)),
	)
}

func recordInvocationMetric(recorder service.InvocationMetricsRecorder, metric service.InvocationMetric) {
	if recorder == nil {
		return
	}
	recorder.RecordInvocationMetric(metric)
}

func invocationSourceLabels(labels []invocations.InputSourceLabel) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		out = append(out, string(label))
	}
	return out
}
