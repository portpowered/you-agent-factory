package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	platformprocessmemory "github.com/portpowered/infinite-you/pkg/platform/processmemory"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/metrics"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

const (
	runtimeMetricLifecycleStarted           = "runtime.lifecycle.started"
	runtimeMetricLifecycleStopped           = "runtime.lifecycle.stopped"
	runtimeMetricStateActive                = "runtime.state.active"
	runtimeMetricStateIdle                  = "runtime.state.idle"
	runtimeMetricStatePaused                = "runtime.state.paused"
	runtimeMetricStateFailed                = "runtime.state.failed"
	runtimeMetricQueueInFlight              = "runtime.queue.in_flight"
	runtimeMetricQueueSubmissionCount       = "queue.submission_count"
	runtimeMetricDispatchStarted            = "dispatch.started"
	runtimeMetricDispatchComplete           = "dispatch.completed"
	runtimeMetricDispatchDuration           = "dispatch.duration"
	runtimeMetricDispatchRetries            = "dispatch.retry_count"
	runtimeMetricProviderRequest            = "provider.requested"
	runtimeMetricProviderComplete           = "provider.completed"
	runtimeMetricProviderFailed             = "provider.failed"
	runtimeMetricProviderDuration           = "provider.duration"
	runtimeMetricProviderInputTok           = "provider.input_tokens"
	runtimeMetricProviderOutputTok          = "provider.output_tokens"
	runtimeMetricProviderCachedInputTok     = "provider.cached_input_tokens"
	runtimeMetricProviderReasoningOutputTok = "provider.reasoning_output_tokens"
	runtimeMetricScriptStarted              = "script.started"
	runtimeMetricScriptComplete             = "script.completed"
	runtimeMetricScriptDuration             = "script.duration"
	runtimeMetricScriptTimedOut             = "script.timed_out"
	runtimeMetricScriptFailed               = "script.failed"
	runtimeMemoryUnitBytes                  = "bytes"
	runtimeMemoryUnitCount                  = "count"
	runtimeMemoryUnitBoolean                = "boolean"
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
	metricFields.WorkerSessionID = r.workerSessionIDForDispatch(record.DispatchID)
	metricFields.Outcome = string(record.Result.Outcome)
	workerDef, _ := r.runtimeWorkerDefinition(metricFields.WorkerType)
	resolvedProvider := providerMetricProvider(record.Result, workerDef)
	if resolvedProvider == "" {
		resolvedProvider = normalizedRuntimeMetricProvider(metricFields.Provider)
	}
	metricFields.Provider = resolvedProvider
	r.emitMetricCounter(runtimeMetricDispatchComplete, 1, metricFields)
	r.emitMetricSample(runtimeMetricDispatchDuration, float64(record.Result.Metrics.Duration.Milliseconds()), "ms", metricFields)
	r.emitMetricSample(runtimeMetricDispatchRetries, float64(record.Result.Metrics.RetryCount), "", metricFields)
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
	providerFields.Provider = providerMetricProvider(result, workerDef)
	providerFields.Model = providerMetricModel(result.Diagnostics, workerDef)
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
	if cachedInputTokens, ok := providerMetricMetadataFloat(result.Diagnostics, workerexecution.ProviderResponseMetadataCachedInputTokens); ok {
		r.emitMetricSample(runtimeMetricProviderCachedInputTok, cachedInputTokens, "tokens", providerFields)
	}
	if reasoningOutputTokens, ok := providerMetricMetadataFloat(result.Diagnostics, workerexecution.ProviderResponseMetadataReasoningOutputTokens); ok {
		r.emitMetricSample(runtimeMetricProviderReasoningOutputTok, reasoningOutputTokens, "tokens", providerFields)
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
	provider = strings.TrimSpace(provider)
	if provider == "" || strings.Contains(provider, "${") {
		return ""
	}
	return providers.ID(provider).CanonicalSessionProvider()
}

func providerMetricProvider(result workerexecution.WorkResult, workerDef *interfaces.FactoryWorkerConfig) string {
	providers := make([]string, 0, 4)
	if result.Diagnostics != nil && result.Diagnostics.Provider != nil {
		providers = append(providers, result.Diagnostics.Provider.Provider)
	}
	if result.Continuation != nil {
		providers = append(providers, result.Continuation.Provider)
	}
	if result.ProviderSession != nil {
		providers = append(providers, result.ProviderSession.Provider)
	}
	if workerDef != nil {
		providers = append(providers, workerDef.ModelProvider)
	}
	for _, provider := range providers {
		if normalized := normalizedRuntimeMetricProvider(provider); normalized != "" {
			return normalized
		}
	}
	return ""
}

func providerMetricModel(diagnostics *workerexecution.WorkDiagnostics, workerDef *interfaces.FactoryWorkerConfig) string {
	if diagnostics != nil && diagnostics.Provider != nil && strings.TrimSpace(diagnostics.Provider.Model) != "" {
		return strings.TrimSpace(diagnostics.Provider.Model)
	}
	if workerDef != nil {
		return strings.TrimSpace(workerDef.Model)
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

func (r *Bundle) workerSessionIDForDispatch(dispatchID string) string {
	if r == nil || r.EventHistory == nil {
		return ""
	}
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return ""
	}
	events := r.EventHistory.CanonicalEvents()
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Type != interfaces.FactoryEventTypeDispatchWorkerSessionAssoc ||
			event.Context.DispatchID == nil || strings.TrimSpace(*event.Context.DispatchID) != dispatchID {
			continue
		}
		var payload interfaces.DispatchWorkerSessionAssociationEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		if workerSessionID := strings.TrimSpace(payload.WorkerSessionID); workerSessionID != "" {
			return workerSessionID
		}
	}
	return ""
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

type synchronizedMetricsEmitter struct {
	bundle *Bundle
}

func (emitter synchronizedMetricsEmitter) emit(
	kind string,
	name string,
	emit func(metrics.MetricsEmitter) error,
) error {
	if emitter.bundle == nil {
		return nil
	}
	emitter.bundle.metricsMu.Lock()
	defer emitter.bundle.metricsMu.Unlock()
	err := emit(emitter.bundle.rawMetricsEmitter())
	if err != nil {
		emitter.bundle.reportMetricFailure(kind, name, err)
	}
	return err
}

func (emitter synchronizedMetricsEmitter) Counter(
	ctx context.Context, name string, value float64, fields metrics.Fields,
) error {
	return emitter.emit("counter", name, func(sink metrics.MetricsEmitter) error {
		return sink.Counter(ctx, name, value, fields)
	})
}

func (emitter synchronizedMetricsEmitter) Gauge(
	ctx context.Context, name string, value float64, fields metrics.Fields,
) error {
	return emitter.emit("gauge", name, func(sink metrics.MetricsEmitter) error {
		return sink.Gauge(ctx, name, value, fields)
	})
}

func (emitter synchronizedMetricsEmitter) Sample(
	ctx context.Context, name string, value float64, unit string, fields metrics.Fields,
) error {
	return emitter.emit("sample", name, func(sink metrics.MetricsEmitter) error {
		return sink.Sample(ctx, name, value, unit, fields)
	})
}

func (r *Bundle) rawMetricsEmitter() metrics.MetricsEmitter {
	if r == nil {
		return metrics.NoopEmitter{}
	}
	return metrics.EnsureEmitter(r.MetricsSink)
}

// MetricsEmitter returns the metrics sink emitter for the hosted bundle.
func (r *Bundle) MetricsEmitter() metrics.MetricsEmitter {
	if r == nil || r.MetricsSink == nil {
		return metrics.NoopEmitter{}
	}
	return synchronizedMetricsEmitter{bundle: r}
}

// EmitMetricCounter records one runtime counter sample on the hosted bundle.
func (r *Bundle) EmitMetricCounter(name string, value float64, fields metrics.Fields) {
	r.emitMetricCounter(name, value, fields)
}

func (r *Bundle) emitMetricCounter(name string, value float64, fields metrics.Fields) {
	if r == nil {
		return
	}
	_ = r.metricsEmitter().Counter(context.Background(), name, value, fields)
}

func (r *Bundle) emitMetricGauge(name string, value float64, fields metrics.Fields) {
	if r == nil {
		return
	}
	_ = r.metricsEmitter().Gauge(context.Background(), name, value, fields)
}

func (r *Bundle) emitMetricSample(name string, value float64, unit string, fields metrics.Fields) {
	if r == nil {
		return
	}
	_ = r.metricsEmitter().Sample(context.Background(), name, value, unit, fields)
}

func (r *Bundle) reportMetricFailure(kind, name string, err error) {
	if r == nil || err == nil {
		return
	}
	failure := fmt.Errorf("runtime metrics %s emission failed for %q: %w", kind, name, err)
	r.metricsErrMu.Lock()
	if r.metricsErr == nil {
		r.metricsErr = failure
	}
	r.metricsErrMu.Unlock()
	r.RuntimeLogger().Warn(
		"runtime metrics "+kind+" emission failed",
		zap.String("metric_name", name),
		zap.Error(err),
	)
}

func (r *Bundle) metricsError() error {
	if r == nil {
		return nil
	}
	r.metricsErrMu.Lock()
	defer r.metricsErrMu.Unlock()
	return r.metricsErr
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

// EmitRuntimeMemoryMetrics records one point-in-time runtime observation on
// the bundle's existing runtime metrics sink. The Go memory fields and the
// process-commit result are collected before any record is emitted, so all
// fields describe one observer pass.
func (r *Bundle) EmitRuntimeMemoryMetrics() error {
	if r == nil {
		return nil
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	commitBytes, commitErr := platformprocessmemory.CurrentCommit()
	return r.emitRuntimeMemoryStats(runtimeMemoryStats{
		heapAlloc:              stats.HeapAlloc,
		heapInuse:              stats.HeapInuse,
		sys:                    stats.Sys,
		numGC:                  stats.NumGC,
		goroutines:             runtime.NumGoroutine(),
		processCommit:          commitBytes,
		processCommitAvailable: commitErr == nil && commitBytes > 0,
	})
}

type runtimeMemoryStats struct {
	heapAlloc              uint64
	heapInuse              uint64
	sys                    uint64
	numGC                  uint32
	goroutines             int
	processCommit          uint64
	processCommitAvailable bool
}

func (r *Bundle) emitRuntimeMemoryStats(stats runtimeMemoryStats) error {
	if r == nil {
		return nil
	}
	r.metricsMu.Lock()
	defer r.metricsMu.Unlock()

	fields := metrics.Fields{}
	var errs []error
	errAppend := func(name string, value float64, unit string) {
		if err := r.emitRuntimeMemorySampleLocked(name, value, unit, fields); err != nil {
			errs = append(errs, err)
		}
	}
	errAppend(factoryruntime.RuntimeMemoryHeapAlloc, float64(stats.heapAlloc), runtimeMemoryUnitBytes)
	errAppend(factoryruntime.RuntimeMemoryHeapInuse, float64(stats.heapInuse), runtimeMemoryUnitBytes)
	errAppend(factoryruntime.RuntimeMemorySys, float64(stats.sys), runtimeMemoryUnitBytes)
	errAppend(factoryruntime.RuntimeMemoryNumGC, float64(stats.numGC), runtimeMemoryUnitCount)
	errAppend(factoryruntime.RuntimeMemoryGoroutines, float64(stats.goroutines), runtimeMemoryUnitCount)
	errAppend(factoryruntime.RuntimeMemoryProcessCommit, float64(stats.processCommit), runtimeMemoryUnitBytes)
	commitAvailable := 0.0
	if stats.processCommitAvailable {
		commitAvailable = 1
	}
	errAppend(factoryruntime.RuntimeMemoryProcessCommitAvailable, commitAvailable, runtimeMemoryUnitBoolean)
	return errors.Join(errs...)
}

func (r *Bundle) emitRuntimeMemorySampleLocked(
	name string,
	value float64,
	unit string,
	fields metrics.Fields,
) error {
	err := r.rawMetricsEmitter().Sample(context.Background(), name, value, unit, fields)
	if err == nil {
		return nil
	}
	r.reportMetricFailure("sample", name, err)
	return fmt.Errorf("runtime metrics sample %q: %w", name, err)
}
