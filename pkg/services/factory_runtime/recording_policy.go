package factory

import "fmt"

// ValidateRecordReplayPaths enforces mutually exclusive runtime recording
// modes before concrete runtime services are constructed.
func ValidateRecordReplayPaths(recordPath, replayPath string) error {
	if recordPath != "" && replayPath != "" {
		return fmt.Errorf("--record and --replay cannot be used together")
	}
	return nil
}
