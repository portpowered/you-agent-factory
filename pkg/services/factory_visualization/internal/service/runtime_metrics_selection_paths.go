package service

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

var runtimeMetricsBackupSuffixPattern = regexp.MustCompile(`-[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}-[0-9]{2}-[0-9]{2}\.[0-9]{3}(?:-[0-9]+)?\.log(?:\.gz)?$`)

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

func runtimeMetricsPathSelector(
	root string,
	sessionID string,
	sessionIDs []string,
	runtimeID string,
	startTimeUTC time.Time,
	endTimeUTC time.Time,
) func(string, bool) bool {
	sessionComponents := runtimeMetricsSessionPathComponents(sessionID, sessionIDs)
	runtimeComponent := platformruntimeartifact.RuntimeArtifactPathComponents(runtimeID)
	return func(path string, isDirectory bool) bool {
		if !runtimeMetricsDatePathInWindow(root, path, isDirectory, startTimeUTC, endTimeUTC) {
			return false
		}
		if isDirectory || (len(sessionComponents) == 0 && runtimeID == "") {
			return true
		}
		base := filepath.Base(path)
		marker := "-runtime-metrics-"
		if len(sessionComponents) > 0 &&
			!runtimeMetricsArtifactMatchesSession(base, marker, sessionComponents) &&
			runtimeMetricsArtifactHasEncodedComponents(base, marker) {
			return false
		}
		if runtimeID != "" {
			if len(sessionComponents) > 0 {
				if !runtimeMetricsArtifactMatchesRuntime(base, marker, sessionComponents, runtimeComponent) &&
					runtimeMetricsArtifactHasEncodedComponents(base, marker) {
					return false
				}
			} else if !runtimeMetricsArtifactContains(base, "-"+runtimeComponent) &&
				runtimeMetricsArtifactHasEncodedComponents(base, marker) {
				return false
			}
		}
		return true
	}
}

func runtimeMetricsSessionPathComponents(sessionID string, sessionIDs []string) []string {
	if len(sessionIDs) == 0 {
		if sessionID == "" {
			return nil
		}
		return []string{platformruntimeartifact.RuntimeArtifactPathComponents(sessionID)}
	}
	components := make([]string, 0, len(sessionIDs))
	for _, candidate := range sessionIDs {
		component := platformruntimeartifact.RuntimeArtifactPathComponents(candidate)
		if component != "" {
			components = append(components, component)
		}
	}
	return components
}

func runtimeMetricsArtifactMatchesSession(base, marker string, components []string) bool {
	for _, component := range components {
		if strings.Contains(base, marker+component+"-") {
			return true
		}
	}
	return false
}

func runtimeMetricsArtifactMatchesRuntime(base, marker string, sessionComponents []string, runtimeComponent string) bool {
	for _, sessionComponent := range sessionComponents {
		if runtimeMetricsArtifactContains(base, marker+sessionComponent+"-"+runtimeComponent) {
			return true
		}
	}
	return false
}

func runtimeMetricsArtifactContains(base, component string) bool {
	return strings.Contains(base, component+"-") ||
		strings.Contains(base, component+".log")
}

func runtimeMetricsArtifactHasEncodedComponents(base, marker string) bool {
	_, suffix, found := strings.Cut(base, marker)
	if !found {
		return false
	}
	suffix = runtimeMetricsBackupSuffixPattern.ReplaceAllString(suffix, "")
	suffix = strings.TrimSuffix(suffix, ".log")
	suffix = strings.TrimSuffix(suffix, ".gz")
	return strings.Contains(suffix, "-")
}

func runtimeMetricsDatePathInWindow(
	root string,
	path string,
	isDirectory bool,
	startTimeUTC time.Time,
	endTimeUTC time.Time,
) bool {
	if startTimeUTC.IsZero() && endTimeUTC.IsZero() {
		return true
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) < 3 {
		return true
	}
	day, ok := parseRuntimeMetricsDate(parts[:3])
	if !ok {
		return true
	}
	if !isDirectory && len(parts) == 3 {
		return true
	}
	dayEnd := day.AddDate(0, 0, 1)
	if !endTimeUTC.IsZero() && !day.Before(endTimeUTC) {
		return false
	}
	if !startTimeUTC.IsZero() && !dayEnd.After(startTimeUTC) {
		return false
	}
	return true
}

func parseRuntimeMetricsDate(parts []string) (time.Time, bool) {
	if len(parts) != 3 {
		return time.Time{}, false
	}
	value, err := time.ParseInLocation("2006/01/02", strings.Join(parts, "/"), time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return value, true
}
