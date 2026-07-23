package runtime

import (
	"context"
	"strings"

	metrics "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"go.uber.org/zap"
)

const (
	metricResponseStreamPublished = "session_response_stream.published"
	metricResponseStreamCompacted = "session_response_stream.compacted"
	metricResponseStreamDegraded  = "session_response_stream.degraded"
)

// ResponseStreamRuntimeResolver interprets the session owner's opaque handle
// without coupling session state to a concrete runtime bundle package.
type ResponseStreamRuntimeResolver func(any) (metrics.MetricsEmitter, *zap.Logger)

// ResponseStreamObserver owns runtime metrics and logging for provider response streams.
type ResponseStreamObserver struct{ resolve ResponseStreamRuntimeResolver }

func NewResponseStreamObserver(resolve ResponseStreamRuntimeResolver) ResponseStreamObserver {
	return ResponseStreamObserver{resolve: resolve}
}

func (o ResponseStreamObserver) ObserveResponseStreamPublished(session *livesession.LiveSession, sessionID string, event responsestream.Event) {
	o.emitResponseStreamMetric(session, sessionID, metricResponseStreamPublished, metrics.Fields{DispatchID: strings.TrimSpace(event.DispatchID), Reason: string(event.Kind)})
}

func (o ResponseStreamObserver) ObserveResponseStreamCompaction(session *livesession.LiveSession, sessionID, dispatchID string, summary responsestream.CompactionSummary) {
	o.emitResponseStreamMetric(session, sessionID, metricResponseStreamCompacted, metrics.Fields{DispatchID: strings.TrimSpace(dispatchID), Reason: string(summary.Reason)})
	if _, logger := o.responseStreamTelemetry(session); logger != nil {
		logger.Warn("session response stream compacted internal provider progress",
			zap.String("dispatch_id", dispatchID), zap.String("compaction_reason", string(summary.Reason)),
			zap.Int("dropped_sequence_count", summary.DroppedSequenceCount),
			zap.Int64("first_retained_sequence", summary.FirstRetainedSequence), zap.Int64("last_dropped_sequence", summary.LastDroppedSequence))
	}
}

func (o ResponseStreamObserver) ObserveResponseStreamDegraded(session *livesession.LiveSession, sessionID, dispatchID, reason string, fallback *zap.Logger, err error) {
	o.emitResponseStreamMetric(session, sessionID, metricResponseStreamDegraded, metrics.Fields{DispatchID: strings.TrimSpace(dispatchID), Reason: strings.TrimSpace(reason)})
	logger := fallback
	if _, runtimeLogger := o.responseStreamTelemetry(session); runtimeLogger != nil {
		logger = runtimeLogger
	}
	if logger == nil {
		return
	}
	fields := []zap.Field{zap.String("session_id", sessionID), zap.String("dispatch_id", strings.TrimSpace(dispatchID)), zap.String("reason", strings.TrimSpace(reason))}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	logger.Warn("internal provider progress publication degraded", fields...)
}

func (o ResponseStreamObserver) responseStreamTelemetry(session *livesession.LiveSession) (metrics.MetricsEmitter, *zap.Logger) {
	if session == nil || o.resolve == nil {
		return nil, nil
	}
	return o.resolve(session.Handle)
}

func (o ResponseStreamObserver) emitResponseStreamMetric(session *livesession.LiveSession, sessionID, name string, fields metrics.Fields) {
	emitter, logger := o.responseStreamTelemetry(session)
	if emitter == nil {
		return
	}
	if fields.DispatchID == "" {
		fields.DispatchID = sessionID
	}
	if err := emitter.Counter(context.Background(), name, 1, fields); err != nil && logger != nil {
		logger.Warn("session response stream metric emission failed", zap.String("metric_name", name), zap.String("session_id", sessionID), zap.Error(err))
	}
}
