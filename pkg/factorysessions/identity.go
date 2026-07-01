package factorysessions

import (
	"path/filepath"
	"strings"
)

// LogicalSessionKeyID returns the stable logical-session key for one live session target.
func LogicalSessionKeyID(session *LiveSession) string {
	if session == nil {
		return ""
	}
	folderPath := filepath.Clean(strings.TrimSpace(session.FolderPath))
	if folderPath == "." {
		folderPath = ""
	}
	targetKind := strings.TrimSpace(string(session.Target.Kind))
	targetName := strings.TrimSpace(session.Target.Name)
	if targetKind == "" {
		targetKind = string(TargetKindDefault)
	}
	return strings.Join([]string{folderPath, targetKind, targetName}, "::")
}
