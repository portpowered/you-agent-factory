// Package timedisplay owns human-readable CLI timestamp and duration formatting.
package timedisplay

import (
	"fmt"
	"time"
)

const (
	unavailableValue = "n/a"
	timestampLayout  = "2006-01-02 15:04:05 UTC"
)

// Timestamp renders an absolute timestamp with an explicit UTC timezone.
func Timestamp(value time.Time) string {
	if value.IsZero() {
		return unavailableValue
	}
	return value.UTC().Format(timestampLayout)
}

// Duration renders elapsed time compactly for operator-facing CLI output.
func Duration(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	if value == 0 {
		return "0s"
	}
	if value < time.Second {
		return fmt.Sprintf("%dms", value.Milliseconds())
	}

	value = value.Truncate(time.Second)
	if value < time.Minute {
		return fmt.Sprintf("%ds", int(value.Seconds()))
	}
	if value < time.Hour {
		minutes := int(value.Minutes())
		seconds := int(value.Seconds()) % 60
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}

	hours := int(value.Hours())
	minutes := int(value.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, minutes)
}

// ElapsedSince renders elapsed time from start to now, with a deliberate
// placeholder for missing time inputs.
func ElapsedSince(start, now time.Time) string {
	if start.IsZero() || now.IsZero() {
		return unavailableValue
	}
	return Duration(now.Sub(start))
}
