package modelhost

import (
	"strings"

	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

const (
	metricLoadSuccess      = "model_host.load.success"
	metricLoadFailure      = "model_host.load.failure"
	metricReadinessTimeout = "model_host.readiness.timeout"
	metricProcessCrash     = "model_host.process.crash"
	metricLeaseAcquire     = "model_host.lease.acquire"
	metricLeaseRelease     = "model_host.lease.release"
	metricLeaseExhausted   = "model_host.lease.exhausted"
	metricUnload           = "model_host.unload"
)

// Logger emits structured operator diagnostics for model host activity.
type Logger interface {
	Info(msg string, fields map[string]string)
	Warn(msg string, fields map[string]string)
}

// MetricsRecorder receives model host counter emissions.
type MetricsRecorder interface {
	RecordMetric(name string, labels map[string]string)
}

// Diagnostics configures model host operator logging and metrics.
type Diagnostics struct {
	Logger  Logger
	Metrics MetricsRecorder
}

func (d Diagnostics) info(msg string, fields map[string]string) {
	if d.Logger == nil {
		return
	}
	d.Logger.Info(msg, cloneDiagnosticLabels(fields))
}

func (d Diagnostics) warn(msg string, fields map[string]string) {
	if d.Logger == nil {
		return
	}
	d.Logger.Warn(msg, cloneDiagnosticLabels(fields))
}

func (d Diagnostics) record(name string, fields map[string]string) {
	if d.Metrics == nil || strings.TrimSpace(name) == "" {
		return
	}
	d.Metrics.RecordMetric(name, cloneDiagnosticLabels(fields))
}

func identityDiagnosticFields(identity Identity) map[string]string {
	return map[string]string{
		"managed_runtime_identity": strings.TrimSpace(identity.Name),
		"backend":                  strings.TrimSpace(identity.Backend),
	}
}

func (d Diagnostics) logLoadStarted(identity Identity) {
	fields := identityDiagnosticFields(identity)
	fields["readiness_state"] = string(managedruntime.ReadinessStateLoading)
	fields["lifecycle_state"] = string(managedruntime.LifecycleStateLoading)
	d.info("model host load started", fields)
}

func (d Diagnostics) logLoadReady(identity Identity) {
	fields := identityDiagnosticFields(identity)
	fields["readiness_state"] = string(managedruntime.ReadinessStateReady)
	fields["lifecycle_state"] = string(managedruntime.LifecycleStateLoaded)
	d.info("model host load ready", fields)
	d.record(metricLoadSuccess, fields)
}

func (d Diagnostics) logLoadFailed(identity Identity, class FailureClass, err error) {
	fields := identityDiagnosticFields(identity)
	fields["failure_class"] = string(class)
	fields["readiness_state"] = string(ReadinessStateForFailureClass(class))
	if err != nil {
		fields["error"] = err.Error()
	}
	d.warn("model host load failed", fields)
	d.record(metricLoadFailure, fields)
	switch class {
	case FailureClassLoadingTimeout:
		d.record(metricReadinessTimeout, fields)
	case FailureClassProcessCrash:
		d.record(metricProcessCrash, fields)
	}
}

func (d Diagnostics) logProcessCrash(identity Identity, err error) {
	fields := identityDiagnosticFields(identity)
	fields["failure_class"] = string(FailureClassProcessCrash)
	fields["readiness_state"] = string(managedruntime.ReadinessStateFailed)
	if err != nil {
		fields["error"] = err.Error()
	}
	d.warn("model host process crashed", fields)
	d.record(metricProcessCrash, fields)
}

func (d Diagnostics) logLeaseAcquired(identity Identity, leaseID string) {
	fields := identityDiagnosticFields(identity)
	fields["lease_id"] = strings.TrimSpace(leaseID)
	d.info("model host lease acquired", fields)
	d.record(metricLeaseAcquire, fields)
}

func (d Diagnostics) logLeaseReleased(identity Identity, leaseID string) {
	fields := identityDiagnosticFields(identity)
	fields["lease_id"] = strings.TrimSpace(leaseID)
	d.info("model host lease released", fields)
	d.record(metricLeaseRelease, fields)
}

func (d Diagnostics) logLeaseExhausted(identity Identity) {
	fields := identityDiagnosticFields(identity)
	fields["failure_class"] = string(FailureClassCapacityExhausted)
	d.warn("model host lease capacity exhausted", fields)
	d.record(metricLeaseExhausted, fields)
}

func (d Diagnostics) logUnload(identity Identity, reason string) {
	fields := identityDiagnosticFields(identity)
	fields["unload_reason"] = strings.TrimSpace(reason)
	d.info("model host unload", fields)
	d.record(metricUnload, fields)
}

func cloneDiagnosticLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}
