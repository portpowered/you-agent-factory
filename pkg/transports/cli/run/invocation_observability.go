package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/runconfig"
	"go.uber.org/zap"
)

const (
	cleanInvocationLogMessageCompleted = "run.invocation.completed"
	cleanInvocationLogMessageRejected  = "run.invocation.rejected"
	cleanInvocationModeLabel           = "clean"
	cleanInvocationRejectReason        = "ambiguous_input"
	cleanInvocationOutcomeSuccess      = "success"
	cleanInvocationOutcomeFailure      = "failure"
	cleanInvocationOutcomeCancelled    = "cancelled"
	cleanInvocationOutcomeTimeout      = "timeout"
	cleanInvocationErrorSummaryLimit   = 160
)

type runtimeLogDiagnosticsProvider interface {
	RuntimeLogDiagnostics() runtimeartifact.Diagnostics
}

func runtimeLogDiagnosticsForRunner(runner factoryServiceRunner) runtimeartifact.Diagnostics {
	if provider, ok := runner.(runtimeLogDiagnosticsProvider); ok {
		return provider.RuntimeLogDiagnostics()
	}
	return runtimeartifact.Diagnostics{}
}

type cleanInvocationCounterSet struct {
	attempts          atomic.Int64
	successes         atomic.Int64
	failures          atomic.Int64
	ambiguityRejected atomic.Int64
	cancellations     atomic.Int64
}

type CleanInvocationMetricsSnapshot struct {
	Attempts          int64
	Successes         int64
	Failures          int64
	AmbiguityRejected int64
	Cancellations     int64
}

type cleanInvocationCompletionLogInput struct {
	Duration time.Duration
	Snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net]
	Target   *cleanInvocationWorkTarget
	Success  *cleanInvocationSuccess
	Err      error
}

var cleanInvocationMetrics cleanInvocationCounterSet

func recordCleanInvocationAttempt() {
	cleanInvocationMetrics.attempts.Add(1)
}

func ObserveInvocationRejection(logger *zap.Logger, err error) {
	var ambiguousErr *AmbiguousInvocationInputError
	if !errors.As(err, &ambiguousErr) {
		return
	}
	recordCleanInvocationAttempt()
	cleanInvocationMetrics.ambiguityRejected.Add(1)
	cleanInvocationLogger(logger).Info(
		cleanInvocationLogMessageRejected,
		zap.String("mode", cleanInvocationModeLabel),
		zap.String("reason", cleanInvocationRejectReason),
		zap.Strings("conflictingSources", invocationInputSourceLogLabels(ambiguousErr.Sources)),
	)
}

func recordCleanInvocationCompletion(logger *zap.Logger, cfg RunConfig, input cleanInvocationCompletionLogInput) {
	logger = cleanInvocationLogger(logger)
	fields := []zap.Field{
		zap.String("mode", cleanInvocationModeLabel),
		zap.String("inputSource", invocationInputSourceLogLabel(cfg.CleanInvocationInputSource)),
		zap.Int64("durationMs", input.Duration.Milliseconds()),
	}

	if input.Success != nil {
		cleanInvocationMetrics.successes.Add(1)
		fields = append(fields,
			zap.String("outcome", cleanInvocationOutcomeSuccess),
			zap.String("workId", input.Success.WorkID),
			zap.String("workTypeName", input.Success.WorkTypeName),
		)
		if strings.TrimSpace(input.Success.TraceID) != "" {
			fields = append(fields, zap.String("traceId", input.Success.TraceID))
		}
		if strings.TrimSpace(input.Success.SessionID) != "" {
			fields = append(fields, zap.String("sessionId", input.Success.SessionID))
		}
		logger.Info(cleanInvocationLogMessageCompleted, fields...)
		return
	}

	if input.Target != nil {
		fields = append(fields,
			zap.String("workId", input.Target.WorkID),
			zap.String("workTypeName", input.Target.WorkTypeName),
		)
	}

	outcome, code, summary := cleanInvocationFailureLogFields(input.Err)
	switch outcome {
	case cleanInvocationOutcomeCancelled:
		cleanInvocationMetrics.cancellations.Add(1)
	case cleanInvocationOutcomeFailure, cleanInvocationOutcomeTimeout:
		cleanInvocationMetrics.failures.Add(1)
	}
	fields = append(fields,
		zap.String("outcome", outcome),
		zap.String("errorCode", code),
	)
	if summary != "" {
		fields = append(fields, zap.String("errorSummary", summary))
	}
	logger.Info(cleanInvocationLogMessageCompleted, fields...)
}

func cleanInvocationFailureLogFields(err error) (string, string, string) {
	var invocationErr *InvocationError
	if errors.As(err, &invocationErr) {
		switch invocationErr.Code {
		case InvocationErrorCodeCancelled:
			return cleanInvocationOutcomeCancelled, invocationErr.Code, boundedInvocationErrorSummary(invocationErr.Message)
		case InvocationErrorCodeTimeout:
			return cleanInvocationOutcomeTimeout, invocationErr.Code, boundedInvocationErrorSummary(invocationErr.Message)
		default:
			return cleanInvocationOutcomeFailure, invocationErr.Code, boundedInvocationErrorSummary(invocationErr.Message)
		}
	}
	summary := boundedInvocationErrorSummary(errString(err))
	if summary == "" {
		return cleanInvocationOutcomeFailure, InvocationErrorCodeFailed, ""
	}
	return cleanInvocationOutcomeFailure, InvocationErrorCodeFailed, summary
}

func cleanInvocationLogger(logger *zap.Logger) *zap.Logger {
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}

func invocationInputSourceLogLabels(sources []InvocationInputSource) []string {
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		labels = append(labels, invocationInputSourceLogLabel(source))
	}
	return labels
}

func invocationInputSourceLogLabel(source InvocationInputSource) string {
	switch source {
	case InvocationInputSourcePositional:
		return "positional_prompt"
	case InvocationInputSourceStdin:
		return "stdin"
	case InvocationInputSourceWorkFile:
		return "work_file"
	default:
		return "unknown"
	}
}

func boundedInvocationErrorSummary(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if len(message) <= cleanInvocationErrorSummaryLimit {
		return message
	}
	return message[:cleanInvocationErrorSummaryLimit] + "..."
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func snapshotCleanInvocationMetrics() CleanInvocationMetricsSnapshot {
	return CleanInvocationMetricsSnapshot{
		Attempts:          cleanInvocationMetrics.attempts.Load(),
		Successes:         cleanInvocationMetrics.successes.Load(),
		Failures:          cleanInvocationMetrics.failures.Load(),
		AmbiguityRejected: cleanInvocationMetrics.ambiguityRejected.Load(),
		Cancellations:     cleanInvocationMetrics.cancellations.Load(),
	}
}

func resetCleanInvocationMetricsForTest() {
	cleanInvocationMetrics.attempts.Store(0)
	cleanInvocationMetrics.successes.Store(0)
	cleanInvocationMetrics.failures.Store(0)
	cleanInvocationMetrics.ambiguityRejected.Store(0)
	cleanInvocationMetrics.cancellations.Store(0)
}

func recordCLIInvocationResolved(cfg RunConfig, source work.InputSourceLabel) {
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
	inputErr, ok := err.(*work.InputError)
	if !ok {
		return
	}
	if inputErr.Code == work.InputErrorCodeSourceConflict {
		recordInvocationMetric(cfg.InvocationMetricsRecorder, factorysessions.InvocationMetric{
			Name: "invocation.source_conflict",
			Labels: map[string]string{
				"input_source": "conflict",
			},
		})
		recordInvocationMetric(cfg.InvocationMetricsRecorder, factorysessions.InvocationMetric{
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

func recordInvocationMetric(
	recorder runconfig.InvocationMetricsRecorder,
	metric factorysessions.InvocationMetric,
) {
	if recorder == nil {
		return
	}
	recorder.RecordInvocationMetric(metric)
}

func invocationSourceLabels(labels []work.InputSourceLabel) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		out = append(out, string(label))
	}
	return out
}

type responseEventSink = factoryvisualization.ResponseEventSink

// onResponseEvents consumes validated, session-ordered FactoryResponseEvent
// values. Rendering policy is selected only from canonical kind, phase, and
// typed payload fields; provider-native names and raw payload fallbacks are not
// presentation inputs.
func (r *humanResponseStreamRenderer) PresentResponseEvents(events []factorysessions.FactoryResponseEvent) {
	if r == nil {
		return
	}
	r.stream.PresentResponseEvents(events)
}

func formatHumanResponseEvent(
	validate factorysessions.ResponseEventValidator,
	event factorysessions.FactoryResponseEvent,
) (string, bool) {
	if err := validate.Validate(event); err != nil {
		return "", false
	}

	var line string
	var ok bool
	switch event.Kind {
	case factorysessions.ResponseEventKindReasoning:
		line, ok = formatHumanReasoningEvent(event)
	case factorysessions.ResponseEventKindTool:
		line, ok = formatHumanToolEvent(event)
	case factorysessions.ResponseEventKindError:
		line, ok = formatHumanRetryEvent(event)
	case factorysessions.ResponseEventKindProgress:
		line, ok = formatHumanProgressEvent(event)
	case factorysessions.ResponseEventKindStreamGap:
		line, ok = formatHumanStreamGapEvent(event)
	default:
		return "", false
	}
	if !ok {
		return "", false
	}
	line = boundedHumanProgressPayload(line)
	return line, line != ""
}

func formatHumanToolEvent(event factorysessions.FactoryResponseEvent) (string, bool) {
	status, ok := map[factorysessions.ResponseEventPhase]string{
		factorysessions.ResponseEventPhaseStarted:   "started",
		factorysessions.ResponseEventPhaseCompleted: "completed",
		factorysessions.ResponseEventPhaseFailed:    "failed",
		factorysessions.ResponseEventPhaseCanceled:  "canceled",
	}[event.Phase]
	if !ok {
		return "", false
	}
	var payload factorysessions.ResponseEventTool
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false
	}
	name := normalizeHumanProgressField(payload.ToolName)
	callID := normalizeHumanProgressField(payload.ToolCallID)
	if name == "" || callID == "" {
		return "", false
	}
	return "tool: name=" + name + " call=" + callID + " status=" + status, true
}

func formatHumanReasoningEvent(event factorysessions.FactoryResponseEvent) (string, bool) {
	var payload factorysessions.ResponseEventReasoning
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false
	}
	switch event.Phase {
	case factorysessions.ResponseEventPhaseStarted:
		return "reasoning: started", true
	case factorysessions.ResponseEventPhaseDelta:
		if summary := normalizeHumanProgressField(payload.SummaryDelta); summary != "" {
			return "reasoning: " + summary, true
		}
	case factorysessions.ResponseEventPhaseCompleted:
		if summary := normalizeHumanProgressField(payload.Summary); summary != "" {
			return "reasoning: " + summary, true
		}
		return "reasoning: completed", true
	}
	return "", false
}

func formatHumanRetryEvent(event factorysessions.FactoryResponseEvent) (string, bool) {
	if event.Phase != factorysessions.ResponseEventPhaseUpdated {
		return "", false
	}
	var payload factorysessions.ResponseEventErrorPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || !humanRetryStatus(payload) {
		return "", false
	}
	code := normalizeHumanProgressField(payload.Code)
	if code == "" {
		return "", false
	}
	parts := []string{"retry: code=" + code}
	if payload.RetryAttempt != nil {
		parts = append(parts, "attempt="+strconv.Itoa(*payload.RetryAttempt))
	}
	if payload.RetryAfterSeconds != nil {
		parts = append(parts, "retry-in="+strconv.FormatInt(*payload.RetryAfterSeconds, 10)+"s")
	}
	return strings.Join(parts, " "), true
}

func humanRetryStatus(payload factorysessions.ResponseEventErrorPayload) bool {
	if payload.Retryable || payload.RetryAfterSeconds != nil || payload.RetryAttempt != nil {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(payload.Code)) {
	case "rate_limited", "throttled", "too_many_requests":
		return true
	default:
		return false
	}
}

func formatHumanProgressEvent(event factorysessions.FactoryResponseEvent) (string, bool) {
	if event.Phase != factorysessions.ResponseEventPhaseUpdated {
		return "", false
	}
	var payload factorysessions.ResponseEventProgress
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false
	}
	label := normalizeHumanProgressField(payload.Label)
	if label == "" {
		return "", false
	}
	line := "progress: " + label
	if message := normalizeHumanProgressField(payload.Message); message != "" {
		line += " — " + message
	}
	if payload.PercentComplete != nil && !math.IsNaN(*payload.PercentComplete) &&
		!math.IsInf(*payload.PercentComplete, 0) && *payload.PercentComplete >= 0 && *payload.PercentComplete <= 100 {
		line += " (" + strconv.FormatFloat(*payload.PercentComplete, 'f', -1, 64) + "%)"
	}
	return line, true
}

func formatHumanStreamGapEvent(event factorysessions.FactoryResponseEvent) (string, bool) {
	if event.Phase != factorysessions.ResponseEventPhaseUpdated {
		return "", false
	}
	var payload factorysessions.ResponseEventStreamGap
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false
	}
	if itemID := normalizeHumanProgressField(payload.AffectedItemID); itemID != "" {
		line := "stream gap: item " + itemID + " lifecycle is incomplete"
		if reason := normalizeHumanProgressField(payload.Reason); reason != "" {
			line += " (reason=" + reason + ")"
		}
		return line, true
	}
	line := fmt.Sprintf(
		"stream gap: sequences %d-%d unavailable",
		payload.FromSequence,
		payload.ToSequence,
	)
	if reason := normalizeHumanProgressField(payload.Reason); reason != "" {
		line += " (reason=" + reason + ")"
	}
	return line, true
}

func isPackagedTTSRun(cfg RunConfig) bool {
	return strings.TrimSpace(cfg.NamedFactoryName) == interfaces.PackagedTTSFactoryName
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
		zap.String("packaged_factory_name", interfaces.PackagedTTSFactoryName),
		zap.String("tts_backend", interfaces.DefaultTTSModelName+"/"+interfaces.DefaultTTSBackendName),
		zap.String("readiness_outcome", interfaces.TTSFailureClassLoading),
	}
	if resolution := cfg.NamedFactoryResolution; resolution != nil {
		fields = append(fields,
			zap.String("named_factory_resolution_source", string(resolution.Source)),
			zap.String("named_factory_dir", resolution.FactoryDir),
		)
	}
	logger.Info("packaged tts invocation started", fields...)
}
