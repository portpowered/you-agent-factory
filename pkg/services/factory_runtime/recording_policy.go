package factory

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateRecordReplayPaths enforces mutually exclusive runtime recording
// modes before concrete runtime services are constructed.
func ValidateRecordReplayPaths(recordPath, replayPath string) error {
	if recordPath != "" && replayPath != "" {
		return fmt.Errorf("--record and --replay cannot be used together")
	}
	return nil
}

// RecordingPath applies the recording path policy shared by the runtime writer
// and current-board restore. Tokenized paths retain their existing layout
// while resolving to the concrete Factory Session; explicit non-default paths
// receive the writer's established session suffix.
type RecordingPath string

// ForSession resolves a recording path to the concrete Factory Session.
func (path RecordingPath) ForSession(sessionID string) string {
	basePath := string(path)
	if strings.TrimSpace(basePath) == "" {
		return basePath
	}
	if strings.Contains(basePath, "__factory_session_id__") {
		return strings.ReplaceAll(basePath, "__factory_session_id__", sessionID)
	}
	if sessionID == "~default" {
		return basePath
	}
	ext := filepath.Ext(basePath)
	base := strings.TrimSuffix(basePath, ext)
	return base + "." + sessionID + ext
}
