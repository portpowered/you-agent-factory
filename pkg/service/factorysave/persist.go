package factorysave

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

// SessionFactoryPersistRoot resolves the on-disk factory root for session-scoped persistence.
func SessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	if session != nil && !session.IsDefault && strings.TrimSpace(session.FolderPath) != "" {
		return session.FolderPath
	}
	return factorysessions.SessionFactoryRootDir(serviceRootDir, session)
}
