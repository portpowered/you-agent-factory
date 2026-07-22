package runtimeartifact

import "time"

// Diagnostics describes immutable runtime log and metrics artifact metadata.
// It carries no writers, service handles, or lifecycle operations, so artifact
// producers can expose it without leaking their implementation boundary.
type Diagnostics struct {
	Path                string
	RootDir             string
	StartTimeUTC        time.Time
	MetricsPath         string
	MetricsRootDir      string
	MetricsStartTimeUTC time.Time
	MaxSizeMB           int
	MaxBackups          int
	MaxAgeDays          int
	Compress            bool
}
