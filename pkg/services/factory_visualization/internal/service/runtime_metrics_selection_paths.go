package service

import (
	"path/filepath"
	"strings"
	"time"

	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

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
		if len(sessionComponents) > 0 && !runtimeMetricsArtifactMatchesSession(base, marker, sessionComponents) {
			return false
		}
		if runtimeID != "" {
			if len(sessionComponents) > 0 {
				if !runtimeMetricsArtifactMatchesRuntime(base, marker, sessionComponents, runtimeComponent) {
					return false
				}
			} else if !runtimeMetricsArtifactContains(base, "-"+runtimeComponent) {
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
