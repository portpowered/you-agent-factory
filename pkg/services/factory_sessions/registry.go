package factorysessions

import (
	"path/filepath"
	"strings"
)

// Registry is the live Factory Session directory consumed by session runtime
// roles. Its mutable implementation lives below the service root.
type Registry interface {
	Upsert(*LiveSession, bool)
	Select(string) bool
	Current() *LiveSession
	Get(string) *LiveSession
	Remove(string)
	Count() int
	IDs() []string
	DefaultSession() *LiveSession
	FindByLogicalSessionKeyID(string) *LiveSession
}

// IsDefaultSessionSelector reports whether sessionID is the compatibility default alias.
func IsDefaultSessionSelector(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	return id == "" || id == DefaultSessionID
}

// LogicalSessionKeyID returns the stable logical-session key for one live session target.
func LogicalSessionKeyID(session *LiveSession) string {
	if session == nil {
		return ""
	}
	folderPath := filepath.Clean(strings.TrimSpace(session.FolderPath))
	if folderPath == "." {
		folderPath = ""
	}
	folderPath = filepath.ToSlash(folderPath)
	targetKind := strings.TrimSpace(string(session.Target.Kind))
	targetName := strings.TrimSpace(session.Target.Name)
	if targetKind == "" {
		targetKind = string(TargetKindDefault)
	}
	return strings.Join([]string{folderPath, targetKind, targetName}, "::")
}
