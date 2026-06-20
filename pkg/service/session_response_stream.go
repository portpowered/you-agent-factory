package service

import (
	"context"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/internal/metrics"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

const (
	runtimeMetricSessionResponseStreamPublished = "session_response_stream.published"
	runtimeMetricSessionResponseStreamCompacted = "session_response_stream.compacted"
)

type inferenceProgressPublisherFactory func(sessionID string) workerprovider.InferenceProgressPublisher

func sessionResponseStreamForLiveSession(session *factorysessions.LiveSession) *factorysessions.SessionResponseStream {
	if session == nil {
		return nil
	}
	state, ok := session.Handle.(*liveSessionState)
	if !ok || state == nil {
		return nil
	}
	if state.responseStream == nil {
		state.responseStream = factorysessions.NewSessionResponseStream()
	}
	return state.responseStream
}

func mapInferenceProgressFragment(fragment workerprovider.InferenceProgressFragment) responsestream.Event {
	kind := responsestream.EventKindProgressFragment
	if fragment.Kind == workerprovider.ResponseFragmentKind {
		kind = responsestream.EventKindResponseFragment
	}
	return responsestream.Event{
		Kind:               kind,
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ProviderSessionRef: fragment.ProviderSessionRef,
		Payload:            fragment.Payload,
	}
}

func newInferenceProgressPublisherFactory(
	sessions *factorysessions.Registry,
	logger *zap.Logger,
) inferenceProgressPublisherFactory {
	if sessions == nil {
		return nil
	}
	return func(sessionID string) workerprovider.InferenceProgressPublisher {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			sessionID = defaultFactorySessionID
		}
		return func(fragment workerprovider.InferenceProgressFragment) {
			session := sessions.Get(sessionID)
			if session == nil && sessionID == defaultFactorySessionID {
				session = sessions.Get(defaultFactorySessionID)
			}
			stream := sessionResponseStreamForLiveSession(session)
			if stream == nil {
				if logger != nil {
					logger.Warn("session response stream unavailable; dropping internal provider progress",
						zap.String("session_id", sessionID),
						zap.String("dispatch_id", fragment.DispatchID),
						zap.String("stream_kind", fragment.Kind),
					)
				}
				return
			}
			event := mapInferenceProgressFragment(fragment)
			stream.Append(event)
			emitSessionResponseStreamPublished(session, sessionID, event)
		}
	}
}

func emitSessionResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event) {
	fields := metrics.Fields{
		DispatchID: strings.TrimSpace(event.DispatchID),
		Reason:     string(event.Kind),
	}
	emitSessionResponseStreamMetric(session, sessionID, runtimeMetricSessionResponseStreamPublished, fields)
}

func emitSessionResponseStreamCompaction(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	summary responsestream.CompactionSummary,
) {
	fields := metrics.Fields{
		DispatchID: strings.TrimSpace(dispatchID),
		Reason:     string(summary.Reason),
	}
	emitSessionResponseStreamMetric(session, sessionID, runtimeMetricSessionResponseStreamCompacted, fields)
	if handle := liveSessionHandle(session); handle != nil && handle.runtime != nil && handle.runtime.logger != nil {
		handle.runtime.logger.Warn("session response stream compacted internal provider progress",
			zap.String("session_id", sessionID),
			zap.String("dispatch_id", dispatchID),
			zap.String("compaction_reason", string(summary.Reason)),
			zap.Int("dropped_sequence_count", summary.DroppedSequenceCount),
			zap.Int64("first_retained_sequence", summary.FirstRetainedSequence),
			zap.Int64("last_dropped_sequence", summary.LastDroppedSequence),
		)
	}
}

func emitSessionResponseStreamMetric(
	session *factorysessions.LiveSession,
	sessionID string,
	name string,
	fields metrics.Fields,
) {
	handle := liveSessionHandle(session)
	if handle == nil || handle.runtime == nil {
		return
	}
	if fields.DispatchID == "" {
		fields.DispatchID = sessionID
	}
	if err := handle.runtime.metricsEmitter().Counter(context.Background(), name, 1, fields); err != nil {
		handle.runtime.runtimeLogger().Warn("session response stream metric emission failed",
			zap.String("metric_name", name),
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}
}
