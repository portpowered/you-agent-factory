package service

import (
	"strings"
	"time"

	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

const (
	metricLoadSuccess      = "model_host.load.success"
	metricLoadFailure      = "model_host.load.failure"
	metricReadinessTimeout = "model_host.readiness.timeout"
	metricProcessCrash     = "model_host.process.crash"
	metricUnload           = "model_host.unload"
)

type hostDiagnostics struct {
	logger   modelsHostDiagnosticLogger
	metrics  modelsHostMetricsRecorder
	evidence modelseffects.RuntimeEvidenceRecorder
}

type modelsHostDiagnosticLogger interface {
	Info(string, map[string]string)
	Warn(string, map[string]string)
}

type modelsHostMetricsRecorder interface {
	RecordMetric(string, map[string]string)
}

func (d hostDiagnostics) info(msg string, fields map[string]string) {
	if d.logger == nil {
		return
	}
	d.logger.Info(msg, cloneDiagnosticLabels(fields))
}

func (d hostDiagnostics) warn(msg string, fields map[string]string) {
	if d.logger == nil {
		return
	}
	d.logger.Warn(msg, cloneDiagnosticLabels(fields))
}

func (d hostDiagnostics) record(name string, fields map[string]string) {
	if d.metrics == nil || strings.TrimSpace(name) == "" {
		return
	}
	d.metrics.RecordMetric(name, cloneDiagnosticLabels(fields))
}

func identityDiagnosticFields(identity supervisedIdentity) map[string]string {
	return map[string]string{
		"managed_runtime_identity": strings.TrimSpace(identity.Name),
		"backend":                  strings.TrimSpace(identity.Backend),
	}
}

func (d hostDiagnostics) logLoadStarted(identity supervisedIdentity) {
	fields := identityDiagnosticFields(identity)
	fields["readiness_state"] = "LOADING"
	fields["lifecycle_state"] = "LOADING"
	d.info("model host load started", fields)
}

func (d hostDiagnostics) logLoadReady(identity supervisedIdentity) {
	fields := identityDiagnosticFields(identity)
	fields["readiness_state"] = "READY"
	fields["lifecycle_state"] = "LOADED"
	d.info("model host load ready", fields)
	d.record(metricLoadSuccess, fields)
}

func (d hostDiagnostics) logLoadFailed(
	identity supervisedIdentity,
	class hostFailureClass,
	err error,
	elapsed time.Duration,
) {
	runtimeErr := modelseffects.WrapRuntimeFailure(
		runtimeStageForHostFailure(class), err,
	)
	fields := safeRuntimeDiagnosticFields(identity, runtimeErr, elapsed)
	d.warn("model host load failed", fields)
	d.recordRuntimeStage(class, runtimeErr, elapsed)
	metricFields := identityDiagnosticFields(identity)
	metricFields["failure_class"] = string(class)
	d.record(metricLoadFailure, metricFields)
	switch class {
	case hostFailureClassLoadingTimeout:
		d.record(metricReadinessTimeout, metricFields)
	case hostFailureClassProcessCrash:
		d.record(metricProcessCrash, metricFields)
	}
}

func (d hostDiagnostics) logProcessCrash(
	identity supervisedIdentity,
	err error,
	elapsed time.Duration,
) {
	runtimeErr := modelseffects.WrapRuntimeFailure(
		runtimeStageForHostFailure(hostFailureClassProcessCrash), err,
	)
	fields := safeRuntimeDiagnosticFields(identity, runtimeErr, elapsed)
	d.warn("model host process crashed", fields)
	d.recordRuntimeStage(hostFailureClassProcessCrash, runtimeErr, elapsed)
	metricFields := identityDiagnosticFields(identity)
	metricFields["failure_class"] = string(hostFailureClassProcessCrash)
	d.record(metricProcessCrash, metricFields)
}

func (d hostDiagnostics) recordRuntimeStage(
	class hostFailureClass,
	err error,
	elapsed time.Duration,
) {
	if err == nil {
		return
	}
	modelseffects.RecordRuntimeEvidenceStage(
		d.evidence,
		runtimeStageForHostFailure(class),
		err,
		elapsed,
	)
}

func safeRuntimeDiagnosticFields(
	identity supervisedIdentity,
	err error,
	elapsed time.Duration,
) map[string]string {
	fields := identityDiagnosticFields(identity)
	for key, value := range modelseffects.ProjectRuntimeFailure(err, elapsed).DiagnosticFields() {
		fields[key] = value
	}
	return fields
}

func (d hostDiagnostics) logUnload(identity supervisedIdentity, reason string) {
	fields := identityDiagnosticFields(identity)
	fields["unload_reason"] = strings.TrimSpace(reason)
	d.info("model host unload", fields)
	d.record(metricUnload, fields)
}

func cloneDiagnosticLabels(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
}
