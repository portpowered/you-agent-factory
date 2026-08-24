package service

import (
	"time"

	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
)

func runtimeMetricsStreamSelection(
	root string,
	sessionID string,
	runtimeID string,
	startTimeUTC time.Time,
	endTimeUTC time.Time,
	projection metricsProjection,
) (platformmetrics.StreamSelection, error) {
	return runtimeMetricsStreamSelectionForSessions(
		root,
		sessionID,
		nil,
		runtimeID,
		startTimeUTC,
		endTimeUTC,
		projection,
	)
}

func runtimeMetricsStreamSelectionForSessions(
	root string,
	sessionID string,
	sessionIDs []string,
	runtimeID string,
	startTimeUTC time.Time,
	endTimeUTC time.Time,
	projection metricsProjection,
) (platformmetrics.StreamSelection, error) {
	startTimeUTC = startTimeUTC.UTC()
	endTimeUTC = endTimeUTC.UTC()
	if err := validateMetricsTimeWindow(startTimeUTC, endTimeUTC); err != nil {
		return platformmetrics.StreamSelection{}, err
	}

	selection := platformmetrics.StreamSelection{}
	if needsMetricsPathSelection(sessionID, sessionIDs, runtimeID, startTimeUTC, endTimeUTC) {
		selection.Path = runtimeMetricsPathSelector(root, sessionID, sessionIDs, runtimeID, startTimeUTC, endTimeUTC)
	}
	if needsMetricsEnvelopeSelection(sessionID, sessionIDs, runtimeID, projection) {
		selection.EnvelopeFields = []string{"metric_name", "session_id", "runtime_instance_id"}
		selection.IncludeEnvelope = runtimeMetricsEnvelopeSelector(sessionID, sessionIDs, runtimeID, projection)
	}
	return selection, nil
}

func validateMetricsTimeWindow(startTimeUTC, endTimeUTC time.Time) error {
	if startTimeUTC.IsZero() || endTimeUTC.IsZero() || !startTimeUTC.After(endTimeUTC) {
		return nil
	}
	return &RuntimeMetricsQueryError{
		Kind:    RuntimeMetricsQueryInvalidInput,
		Message: "query Factory Runtime metrics: start time must not be after end time",
	}
}

func needsMetricsPathSelection(sessionID string, sessionIDs []string, runtimeID string, startTimeUTC, endTimeUTC time.Time) bool {
	return sessionID != "" || len(sessionIDs) > 0 || runtimeID != "" || !startTimeUTC.IsZero() || !endTimeUTC.IsZero()
}

func needsMetricsEnvelopeSelection(sessionID string, sessionIDs []string, runtimeID string, projection metricsProjection) bool {
	return sessionID != "" || len(sessionIDs) > 0 || runtimeID != "" || !projection.allDimensions
}

func runtimeMetricsEnvelopeSelector(
	sessionID string,
	sessionIDs []string,
	runtimeID string,
	projection metricsProjection,
) func(platformmetrics.RuntimeMetricRecordEnvelope) bool {
	return func(envelope platformmetrics.RuntimeMetricRecordEnvelope) bool {
		return runtimeMetricsEnvelopeMatchesScope(envelope, sessionID, sessionIDs, runtimeID) &&
			runtimeMetricsEnvelopeMatchesProjection(envelope, projection)
	}
}

func runtimeMetricsEnvelopeMatchesScope(
	envelope platformmetrics.RuntimeMetricRecordEnvelope,
	sessionID string,
	sessionIDs []string,
	runtimeID string,
) bool {
	if len(sessionIDs) > 0 {
		if !containsMetricSessionID(sessionIDs, envelope.Fields["session_id"]) {
			return false
		}
	} else if sessionID != "" && envelope.Fields["session_id"] != sessionID {
		return false
	}
	if runtimeID != "" && envelope.Fields["runtime_instance_id"] != runtimeID {
		return false
	}
	return true
}

func runtimeMetricsEnvelopeMatchesProjection(
	envelope platformmetrics.RuntimeMetricRecordEnvelope,
	projection metricsProjection,
) bool {
	if projection.allDimensions {
		return true
	}
	_, supported := projection.metricNames[envelope.Fields["metric_name"]]
	return supported
}
