package content_materialization

import (
	"github.com/portpowered/infinite-you/pkg/services/work/internal/contenturl"
)

// ValidateContentURL reports whether rawURL is a non-empty supported Work
// content URL.
func ValidateContentURL(rawURL string) error {
	return contenturl.Validate(rawURL)
}

// ContentURLAndFileConflictError reports use of both canonical URL and legacy
// file fields on one content part.
func ContentURLAndFileConflictError() error {
	return contenturl.URLAndFileConflictError()
}

// FilesystemPathToContentURL maps a host filesystem path to a canonical file
// content URL.
func FilesystemPathToContentURL(path string) (string, error) {
	return contenturl.FilesystemPathToURL(path)
}

// ResolveDispatchContentURL resolves relative file content URLs against a
// dispatch working directory.
func ResolveDispatchContentURL(workingDirectory, rawURL string) (string, error) {
	return contenturl.ResolveDispatchURL(workingDirectory, rawURL)
}
