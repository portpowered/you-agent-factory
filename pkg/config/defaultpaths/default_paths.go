// Package defaultpaths owns the canonical default-path policy for operator
// config and runtime artifacts rooted under the shared home directory.
package defaultpaths

import (
	"path/filepath"
	"time"
)

const (
	sharedHomeDirName           = ".you-agent-factory"
	namedFactoriesDirName       = "factories"
	legacyNamedFactoriesDirName = "you-agent-factories"
	operatorConfigFile          = "config.json"
	recordingsDirName           = "recordings"
	runtimeLogsDirName          = "logs"
	runtimeMetricsName          = "metrics"
	recordingsMonthFmt          = "2006-01"
	recordingsDateFmt           = "2006-01-02"
	runtimeMetricsYear          = "2006"
	runtimeMetricsMonth         = "01"
	runtimeMetricsDay           = "02"
)

// SharedRoot returns the canonical shared root below the supplied home
// directory.
func SharedRoot(homeDir string) string {
	return filepath.Join(homeDir, sharedHomeDirName)
}

// NamedFactoriesRoot returns the canonical customer-owned named-factories root
// below homeDir.
func NamedFactoriesRoot(homeDir string) string {
	return filepath.Join(SharedRoot(homeDir), namedFactoriesDirName)
}

// LegacyNamedFactoriesRoot returns the retired global named-factory root used
// only to migrate existing customer-owned factories during initialization.
func LegacyNamedFactoriesRoot(homeDir string) string {
	return filepath.Join(SharedRoot(homeDir), legacyNamedFactoriesDirName)
}

// OperatorConfigPath returns the default operator config path below homeDir.
func OperatorConfigPath(homeDir string) string {
	return filepath.Join(SharedRoot(homeDir), operatorConfigFile)
}

// RecordingsRoot returns the default recordings root below homeDir.
func RecordingsRoot(homeDir string) string {
	return filepath.Join(SharedRoot(homeDir), recordingsDirName)
}

// RuntimeLogsRoot returns the default runtime log root below homeDir.
func RuntimeLogsRoot(homeDir string) string {
	return filepath.Join(SharedRoot(homeDir), runtimeLogsDirName)
}

// RuntimeMetricsRoot returns the default runtime metrics root below homeDir.
func RuntimeMetricsRoot(homeDir string) string {
	return filepath.Join(SharedRoot(homeDir), runtimeMetricsName)
}

// RecordingsDatedDir returns the dated subdirectory used for default live
// replay recordings.
func RecordingsDatedDir(rootDir string, at time.Time) string {
	return filepath.Join(rootDir, at.Format(recordingsMonthFmt), at.Format(recordingsDateFmt))
}

// RuntimeLogsDatedDir returns the dated subdirectory used for runtime logs.
func RuntimeLogsDatedDir(rootDir string, at time.Time) string {
	return calendarDatedDir(rootDir, at)
}

// RuntimeMetricsDatedDir returns the dated subdirectory used for runtime
// metrics.
func RuntimeMetricsDatedDir(rootDir string, at time.Time) string {
	return calendarDatedDir(rootDir, at)
}

func calendarDatedDir(rootDir string, at time.Time) string {
	at = at.UTC()
	return filepath.Join(rootDir, at.Format(runtimeMetricsYear), at.Format(runtimeMetricsMonth), at.Format(runtimeMetricsDay))
}
