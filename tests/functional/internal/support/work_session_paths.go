package support

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

// DefaultSessionWorkAPIPrefix is the HTTP prefix for work routes on the default factory session.
const DefaultSessionWorkAPIPrefix = "/factory-sessions/" + factorysessions.DefaultSessionID

// DefaultSessionWorkPath scopes legacy work-relative paths to the default factory session.
func DefaultSessionWorkPath(path string) string {
	switch {
	case path == "/work" || strings.HasPrefix(path, "/work?"):
		return DefaultSessionWorkAPIPrefix + path
	case strings.HasPrefix(path, "/work/"):
		return DefaultSessionWorkAPIPrefix + path
	case strings.HasPrefix(path, "/work-requests/"):
		return DefaultSessionWorkAPIPrefix + path
	default:
		return path
	}
}

// DefaultSessionWorkURL joins baseURL with a default-session-scoped work path.
func DefaultSessionWorkURL(baseURL, path string) string {
	return strings.TrimSuffix(baseURL, "/") + DefaultSessionWorkPath(path)
}
