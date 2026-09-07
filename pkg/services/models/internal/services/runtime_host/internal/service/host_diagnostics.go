package service

import (
	"strconv"
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

const (
	hostLifecycleStageBackendStart = "BACKEND_START"
	hostLifecycleStageHealth       = "HEALTH"
	hostLifecycleStageUnload       = "UNLOAD"
	hostLifecycleStageStop         = "STOP"
	hostLifecycleOutcomeStarted    = "STARTED"
	hostLifecycleOutcomeCompleted  = "COMPLETED"
	hostLifecycleOutcomeFailed     = "FAILED"
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
	fields := map[string]string{
		"managed_runtime_identity": boundedDiagnosticIdentity(identity.Name),
		"backend":                  boundedDiagnosticIdentity(identity.Backend),
	}
	if revision := boundedDiagnosticIdentity(identity.Revision); revision != "" {
		fields["revision"] = revision
	}
	return fields
}

func (d hostDiagnostics) logLoadStarted(identity supervisedIdentity, correlation string) {
	fields := lifecycleDiagnosticFields(
		identity, correlation, hostLifecycleStageBackendStart,
		hostLifecycleOutcomeStarted, 0,
	)
	fields["readiness_state"] = "LOADING"
	fields["lifecycle_state"] = "LOADING"
	d.info("model host load started", fields)
}

func (d hostDiagnostics) logLoadReady(
	identity supervisedIdentity,
	correlation string,
	elapsed time.Duration,
) {
	fields := lifecycleDiagnosticFields(
		identity, correlation, hostLifecycleStageHealth,
		hostLifecycleOutcomeCompleted, elapsed,
	)
	fields["readiness_state"] = "READY"
	fields["lifecycle_state"] = "LOADED"
	d.info("model host load ready", fields)
	metricFields := identityDiagnosticFields(identity)
	metricFields["readiness_state"] = "READY"
	metricFields["lifecycle_state"] = "LOADED"
	d.record(metricLoadSuccess, metricFields)
	d.recordRuntimeStageSuccess(modelseffects.RuntimeStageBackendStart, elapsed)
	if requiresPinnedGRPCBackend(identity.Backend) {
		d.recordRuntimeStageSuccess(modelseffects.RuntimeStageProtocolLoad, elapsed)
	}
}

func (d hostDiagnostics) logLoadFailed(
	identity supervisedIdentity,
	correlation string,
	class hostFailureClass,
	err error,
	elapsed time.Duration,
) {
	runtimeErr := modelseffects.WrapRuntimeFailure(
		runtimeStageForHostFailure(class), err,
	)
	fields := safeRuntimeDiagnosticFields(identity, runtimeErr, elapsed)
	addLifecycleFields(
		fields, correlation, string(runtimeStageForHostFailure(class)),
		hostLifecycleOutcomeFailed, elapsed,
	)
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
	correlation string,
	err error,
	elapsed time.Duration,
) {
	runtimeErr := modelseffects.WrapRuntimeFailure(
		runtimeStageForHostFailure(hostFailureClassProcessCrash), err,
	)
	fields := safeRuntimeDiagnosticFields(identity, runtimeErr, elapsed)
	addLifecycleFields(
		fields, correlation, string(runtimeStageForHostFailure(hostFailureClassProcessCrash)),
		hostLifecycleOutcomeFailed, elapsed,
	)
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

func (d hostDiagnostics) recordRuntimeStageSuccess(
	stage modelseffects.RuntimeStage,
	elapsed time.Duration,
) {
	modelseffects.RecordRuntimeEvidenceStage(d.evidence, stage, nil, elapsed)
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

func (d hostDiagnostics) logUnload(
	identity supervisedIdentity,
	correlation string,
	reason string,
) {
	fields := lifecycleDiagnosticFields(
		identity, correlation, hostLifecycleStageUnload,
		hostLifecycleOutcomeCompleted, 0,
	)
	if reason = boundedDiagnosticIdentity(reason); reason != "" {
		fields["unload_reason"] = reason
	}
	d.info("model host unload", fields)
	metricFields := identityDiagnosticFields(identity)
	if reason != "" {
		metricFields["unload_reason"] = reason
	}
	d.record(metricUnload, metricFields)
}

func (d hostDiagnostics) logStop(
	identity supervisedIdentity,
	correlation string,
	elapsed time.Duration,
	err error,
) {
	outcome := hostLifecycleOutcomeCompleted
	if err != nil {
		outcome = hostLifecycleOutcomeFailed
	}
	fields := lifecycleDiagnosticFields(identity, correlation, hostLifecycleStageStop, outcome, elapsed)
	if err != nil {
		diagnostic := modelseffects.ProjectRuntimeFailure(
			modelseffects.WrapRuntimeFailure(modelseffects.RuntimeStageBackendStart, err),
			elapsed,
		)
		for key, value := range diagnostic.DiagnosticFields() {
			if key == "duration_millis" || key == "runtime_stage" {
				continue
			}
			fields[key] = value
		}
		d.warn("model host stop failed", fields)
		return
	}
	d.info("model host stopped", fields)
}

func lifecycleDiagnosticFields(
	identity supervisedIdentity,
	correlation string,
	stage string,
	outcome string,
	elapsed time.Duration,
) map[string]string {
	fields := identityDiagnosticFields(identity)
	addLifecycleFields(fields, correlation, stage, outcome, elapsed)
	return fields
}

func addLifecycleFields(
	fields map[string]string,
	correlation string,
	stage string,
	outcome string,
	elapsed time.Duration,
) {
	if fields == nil {
		return
	}
	if correlation = boundedDiagnosticIdentity(correlation); correlation != "" {
		fields["correlation_id"] = correlation
	}
	if stage = boundedDiagnosticIdentity(stage); stage != "" {
		fields["stage"] = stage
	}
	if outcome = boundedDiagnosticIdentity(outcome); outcome != "" {
		fields["outcome"] = outcome
	}
	if elapsed < 0 {
		elapsed = 0
	}
	fields["duration_millis"] = strconv.FormatInt(elapsed.Milliseconds(), 10)
}

func boundedDiagnosticIdentity(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '-' || char == '.' {
			continue
		}
		return ""
	}
	return value
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
