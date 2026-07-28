package host

import (
	"context"
	"strconv"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/metrics"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

const (
	runtimeMetricLifecycleStarted     = "runtime.lifecycle.started"
	runtimeMetricLifecycleStopped     = "runtime.lifecycle.stopped"
	runtimeMetricStateActive          = "runtime.state.active"
	runtimeMetricStateIdle            = "runtime.state.idle"
	runtimeMetricStatePaused          = "runtime.state.paused"
	runtimeMetricStateFailed          = "runtime.state.failed"
	runtimeMetricQueueInFlight        = "runtime.queue.in_flight"
	runtimeMetricQueueSubmissionCount = "queue.submission_count"
	runtimeMetricDispatchStarted      = "dispatch.started"
	runtimeMetricDispatchComplete     = "dispatch.completed"
	runtimeMetricDispatchDuration     = "dispatch.duration"
	runtimeMetricDispatchRetries      = "dispatch.retry_count"
	runtimeMetricDispatchCost         = "dispatch.cost"
	runtimeMetricProviderRequest      = "provider.requested"
	runtimeMetricProviderComplete     = "provider.completed"
	runtimeMetricProviderFailed       = "provider.failed"
	runtimeMetricProviderDuration     = "provider.duration"
	runtimeMetricProviderInputTok     = "provider.input_tokens"
	runtimeMetricProviderOutputTok    = "provider.output_tokens"
	runtimeMetricProviderCost         = "provider.cost"
	runtimeMetricScriptStarted        = "script.started"
	runtimeMetricScriptComplete       = "script.completed"
	runtimeMetricScriptDuration       = "script.duration"
	runtimeMetricScriptTimedOut       = "script.timed_out"
	runtimeMetricScriptFailed         = "script.failed"
)

func (r *Bundle) RecordSubmissionMetric(record work.FactorySubmissionRecord) {
	fields := metrics.Fields{
		WorkID:  strings.TrimSpace(record.Request.WorkID),
		TraceID: strings.TrimSpace(record.Request.TraceID),
	}
	r.emitMetricCounter(runtimeMetricQueueSubmissionCount, 1, fields)
}

func (r *Bundle) RecordDispatchMetric(record interfaces.FactoryDispatchRecord) {
	fields := runtimeDispatchMetricFields(record.Dispatch)
	r.dispatchMetricFields.Store(record.DispatchID, fields)
	r.emitMetricCounter(runtimeMetricDispatchStarted, 1, fields)
	r.emitWorkerBoundaryStartMetrics(fields)
}

func (r *Bundle) RecordCompletionMetrics(record interfaces.FactoryCompletionRecord) {
	fields, _ := r.dispatchMetricFields.LoadAndDelete(record.DispatchID)
	metricFields, _ := fields.(metrics.Fields)
	if metricFields.DispatchID == "" {
		metricFields.DispatchID = record.DispatchID
	}
	metricFields.Outcome = string(record.Result.Outcome)
	r.emitMetricCounter(runtimeMetricDispatchComplete, 1, metricFields)
	r.emitMetricSample(runtimeMetricDispatchDuration, float64(record.Result.Metrics.Duration.Milliseconds()), "ms", metricFields)
	r.emitMetricSample(runtimeMetricDispatchRetries, float64(record.Result.Metrics.RetryCount), "", metricFields)
	if record.Result.Metrics.Cost > 0 {
		r.emitMetricSample(runtimeMetricDispatchCost, record.Result.Metrics.Cost, "usd", metricFields)
	}
	r.emitWorkerBoundaryCompletionMetrics(record.Result, metricFields)
	if r.dispatchCompleted != nil {
		r.dispatchCompleted(record.DispatchID)
	}
}

func runtimeDispatchMetricFields(dispatch work.WorkDispatch) metrics.Fields {
	fields := metrics.Fields{
		DispatchID:  dispatch.DispatchID,
		TraceID:     strings.TrimSpace(dispatch.Execution.TraceID),
		Workstation: strings.TrimSpace(dispatch.WorkstationName),
		WorkerType:  strings.TrimSpace(dispatch.WorkerType),
	}
	if len(dispatch.Execution.WorkIDs) > 0 {
		fields.WorkID = strings.TrimSpace(dispatch.Execution.WorkIDs[0])
	}
	return fields
}

func (r *Bundle) emitWorkerBoundaryStartMetrics(fields metrics.Fields) {
	workerDef, ok := r.runtimeWorkerDefinition(fields.WorkerType)
	if !ok || workerDef == nil {
		return
	}
	switch workerDef.Type {
	case interfaces.WorkerTypeModel, interfaces.WorkerTypeAgent, interfaces.WorkerTypeInference:
		providerFields := fields
		providerFields.Provider = normalizedRuntimeMetricProvider(workerDef.ModelProvider)
		r.emitMetricCounter(runtimeMetricProviderRequest, 1, providerFields)
	case interfaces.WorkerTypeScript:
		r.emitMetricCounter(runtimeMetricScriptStarted, 1, fields)
	}
}

func (r *Bundle) emitWorkerBoundaryCompletionMetrics(result workerexecution.WorkResult, fields metrics.Fields) {
	workerDef, ok := r.runtimeWorkerDefinition(fields.WorkerType)
	if !ok || workerDef == nil {
		return
	}
	switch workerDef.Type {
	case interfaces.WorkerTypeModel, interfaces.WorkerTypeAgent, interfaces.WorkerTypeInference:
		r.emitProviderCompletionMetrics(result, fields, workerDef)
	case interfaces.WorkerTypeScript:
		r.emitScriptCompletionMetrics(result, fields)
	}
}

func (r *Bundle) emitProviderCompletionMetrics(
	result workerexecution.WorkResult,
	fields metrics.Fields,
	workerDef *interfaces.FactoryWorkerConfig,
) {
	providerFields := fields
	providerFields.Provider = normalizedRuntimeMetricProvider(providerMetricProvider(result.Diagnostics, workerDef))
	r.emitMetricCounter(runtimeMetricProviderComplete, 1, providerFields)
	if result.Outcome == workerexecution.OutcomeFailed {
		providerFields.Reason = providerMetricFailureReason(result)
		r.emitMetricCounter(runtimeMetricProviderFailed, 1, providerFields)
	}
	if durationMS, ok := providerMetricDurationMilliseconds(result.Diagnostics); ok {
		r.emitMetricSample(runtimeMetricProviderDuration, durationMS, "ms", providerFields)
	}
	if inputTokens, ok := providerMetricMetadataFloat(result.Diagnostics, workerexecution.ProviderResponseMetadataInputTokens); ok {
		r.emitMetricSample(runtimeMetricProviderInputTok, inputTokens, "tokens", providerFields)
	}
	if outputTokens, ok := providerMetricMetadataFloat(result.Diagnostics, workerexecution.ProviderResponseMetadataOutputTokens); ok {
		r.emitMetricSample(runtimeMetricProviderOutputTok, outputTokens, "tokens", providerFields)
	}
	if result.Metrics.Cost > 0 {
		r.emitMetricSample(runtimeMetricProviderCost, result.Metrics.Cost, "usd", providerFields)
	}
}

func (r *Bundle) emitScriptCompletionMetrics(result workerexecution.WorkResult, fields metrics.Fields) {
	scriptFields := fields
	if timedOut := scriptMetricTimedOut(result); timedOut {
		scriptFields.Reason = "timeout"
	}
	if result.Outcome == workerexecution.OutcomeFailed && scriptFields.Reason == "" {
		scriptFields.Reason = scriptMetricFailureReason(result)
	}
	r.emitMetricCounter(runtimeMetricScriptComplete, 1, scriptFields)
	if durationMS, ok := scriptMetricDurationMilliseconds(result); ok {
		r.emitMetricSample(runtimeMetricScriptDuration, durationMS, "ms", scriptFields)
	}
	if scriptMetricTimedOut(result) {
		r.emitMetricCounter(runtimeMetricScriptTimedOut, 1, scriptFields)
		return
	}
	if result.Outcome == workerexecution.OutcomeFailed {
		r.emitMetricCounter(runtimeMetricScriptFailed, 1, scriptFields)
	}
}

func (r *Bundle) runtimeWorkerDefinition(workerName string) (*interfaces.FactoryWorkerConfig, bool) {
	if r == nil || r.RuntimeCfg == nil || strings.TrimSpace(workerName) == "" {
		return nil, false
	}
	return r.RuntimeCfg.Worker(strings.TrimSpace(workerName))
}

func normalizedRuntimeMetricProvider(provider string) string {
	return workerexecution.CanonicalProviderSessionProvider(strings.TrimSpace(provider))
}

func providerMetricProvider(diagnostics *workerexecution.WorkDiagnostics, workerDef *interfaces.FactoryWorkerConfig) string {
	if diagnostics != nil && diagnostics.Provider != nil && strings.TrimSpace(diagnostics.Provider.Provider) != "" {
		return diagnostics.Provider.Provider
	}
	if workerDef != nil {
		return workerDef.ModelProvider
	}
	return ""
}

func providerMetricFailureReason(result workerexecution.WorkResult) string {
	if result.FailureMetadata != nil && result.FailureMetadata.Type != "" {
		return string(result.FailureMetadata.Type)
	}
	return strings.TrimSpace(string(result.Outcome))
}

func providerMetricDurationMilliseconds(diagnostics *workerexecution.WorkDiagnostics) (float64, bool) {
	if durationMS, ok := providerMetricMetadataFloat(diagnostics, workerexecution.ProviderResponseMetadataDurationAPIMS); ok {
		return durationMS, true
	}
	return providerMetricMetadataFloat(diagnostics, workerexecution.ProviderResponseMetadataDurationMS)
}

func providerMetricMetadataFloat(diagnostics *workerexecution.WorkDiagnostics, key string) (float64, bool) {
	if diagnostics == nil || diagnostics.Provider == nil || diagnostics.Provider.ResponseMetadata == nil {
		return 0, false
	}
	value := strings.TrimSpace(diagnostics.Provider.ResponseMetadata[key])
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func scriptMetricTimedOut(result workerexecution.WorkResult) bool {
	if result.FailureMetadata != nil && result.FailureMetadata.Type == workerexecution.WorkFailureTypeTimeout {
		return true
	}
	if result.Diagnostics == nil || result.Diagnostics.Command == nil {
		return false
	}
	return result.Diagnostics.Command.TimedOut
}

func scriptMetricFailureReason(result workerexecution.WorkResult) string {
	if result.FailureMetadata != nil && result.FailureMetadata.Type != "" {
		return string(result.FailureMetadata.Type)
	}
	if result.Diagnostics != nil && result.Diagnostics.Command != nil && result.Diagnostics.Command.ExitCode != 0 {
		return "exit_code"
	}
	return strings.TrimSpace(string(result.Outcome))
}

func scriptMetricDurationMilliseconds(result workerexecution.WorkResult) (float64, bool) {
	if result.Diagnostics != nil && result.Diagnostics.Command != nil && result.Diagnostics.Command.Duration > 0 {
		return float64(result.Diagnostics.Command.Duration.Milliseconds()), true
	}
	if result.Metrics.Duration <= 0 {
		return 0, false
	}
	return float64(result.Metrics.Duration.Milliseconds()), true
}

// ScriptMetricTimedOut reports whether script execution timed out.
func ScriptMetricTimedOut(result workerexecution.WorkResult) bool {
	return scriptMetricTimedOut(result)
}

// ScriptMetricFailureReason returns the script failure reason label for metrics.
func ScriptMetricFailureReason(result workerexecution.WorkResult) string {
	return scriptMetricFailureReason(result)
}

// ScriptMetricDurationMilliseconds returns script duration in milliseconds when known.
func ScriptMetricDurationMilliseconds(result workerexecution.WorkResult) (float64, bool) {
	return scriptMetricDurationMilliseconds(result)
}

func (r *Bundle) metricsEmitter() metrics.MetricsEmitter {
	return r.MetricsEmitter()
}

// MetricsEmitter returns the metrics sink emitter for the hosted bundle.
func (r *Bundle) MetricsEmitter() metrics.MetricsEmitter {
	if r == nil {
		return metrics.NoopEmitter{}
	}
	return metrics.EnsureEmitter(r.MetricsSink)
}

// EmitMetricCounter records one runtime counter sample on the hosted bundle.
func (r *Bundle) EmitMetricCounter(name string, value float64, fields metrics.Fields) {
	r.emitMetricCounter(name, value, fields)
}

func (r *Bundle) emitMetricCounter(name string, value float64, fields metrics.Fields) {
	if r == nil {
		return
	}
	if err := r.metricsEmitter().Counter(context.Background(), name, value, fields); err != nil {
		r.RuntimeLogger().Warn("runtime metrics counter emission failed", zap.String("metric_name", name), zap.Error(err))
	}
}

func (r *Bundle) emitMetricGauge(name string, value float64, fields metrics.Fields) {
	if r == nil {
		return
	}
	if err := r.metricsEmitter().Gauge(context.Background(), name, value, fields); err != nil {
		r.RuntimeLogger().Warn("runtime metrics gauge emission failed", zap.String("metric_name", name), zap.Error(err))
	}
}

func (r *Bundle) emitMetricSample(name string, value float64, unit string, fields metrics.Fields) {
	if r == nil {
		return
	}
	if err := r.metricsEmitter().Sample(context.Background(), name, value, unit, fields); err != nil {
		r.RuntimeLogger().Warn("runtime metrics sample emission failed", zap.String("metric_name", name), zap.Error(err))
	}
}

// EmitRuntimeLifecycleStart records the runtime lifecycle started counter.
func (r *Bundle) EmitRuntimeLifecycleStart() {
	if r == nil {
		return
	}
	r.emitMetricCounter(runtimeMetricLifecycleStarted, 1, metrics.Fields{})
}

// EmitRuntimeLifecycleStop records the runtime lifecycle stopped counter.
func (r *Bundle) EmitRuntimeLifecycleStop(outcome string, reason string) {
	if r == nil {
		return
	}
	r.emitMetricCounter(runtimeMetricLifecycleStopped, 1, metrics.Fields{
		Outcome: outcome,
		Reason:  strings.TrimSpace(reason),
	})
}

func boolMetricValue(active bool) float64 {
	if active {
		return 1
	}
	return 0
}

// EmitRuntimeStateMetrics records runtime state gauges from an engine snapshot.
func (r *Bundle) EmitRuntimeStateMetrics(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
	if r == nil || snapshot == nil {
		return
	}
	r.emitMetricGauge(runtimeMetricStateActive, boolMetricValue(snapshot.RuntimeStatus == interfaces.RuntimeStatusActive), metrics.Fields{})
	r.emitMetricGauge(runtimeMetricStateIdle, boolMetricValue(snapshot.RuntimeStatus == interfaces.RuntimeStatusIdle), metrics.Fields{})
	r.emitMetricGauge(runtimeMetricStatePaused, boolMetricValue(snapshot.FactoryState == string(interfaces.FactoryStatePaused)), metrics.Fields{})
	r.emitMetricGauge(runtimeMetricStateFailed, boolMetricValue(snapshot.FactoryState == string(interfaces.FactoryStateFailed)), metrics.Fields{})
	r.emitMetricGauge(runtimeMetricQueueInFlight, float64(snapshot.InFlightCount), metrics.Fields{})
}
